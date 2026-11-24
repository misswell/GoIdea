package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

// commitState is the Commit dialog: the message, the inclusion of the changed
// files and the commit options. The changed files themselves stay in m.vcs, so
// a refresh while the dialog is open keeps the list current.
type commitState struct {
	message  string
	amend    bool
	signOff  bool
	runHooks bool
	// allRepositories makes the ordinary Commit action cover every repository
	// with an included change. File, directory and changelist commits keep
	// their explicit scope.
	allRepositories bool
	// amendBefore is the draft replaced by Amend, and amendMessage is the last
	// commit's message put in its place. Amend restores the draft only while
	// the amended message is still untouched.
	amendBefore     string
	amendMessage    string
	amendGeneration uint64
	// repo is the repository selected when the dialog opened. The shared
	// Commit action keeps that context even if another content tab gets focus.
	repo string
	// checks overrides the inclusion of a file (absolute path → included);
	// without an entry a file uses its initial check, as OpenIDEA does.
	checks map[string]bool
	// hunks holds the line changes shown by a partially selected file. An entry
	// replaces the ordinary checked/staged-only decision for that path.
	hunks map[string]*commitHunksState
	// hunkPath is the file whose hunk picker is open.
	hunkPath string
	// only keeps a single file in the commit, as the tab menu's Commit File…
	// asks for.
	only string
	// scope keeps only the changed files under a directory in the commit, as
	// the project view's Git › Commit… asks for; empty means every change.
	scope string
	// onlyPaths limits the commit to an explicit set, as Commit Changelist
	// does. A non-nil empty map means no changelist file is present.
	onlyPaths map[string]bool
	// pushAfter opens Push once the commit succeeds.
	pushAfter bool
	busy      bool
	err       string
	problems  []string
	// problemsOverride is IDEA's Commit Anyway: checks have already been shown
	// and the next Commit click proceeds without running them again.
	problemsOverride bool
}

// commitPlan is what vcs_commit gets: files committed with their working-tree
// content, staged files kept out, and the files that end up in the commit.
type commitPlan struct {
	stage    []string
	exclude  []string
	included []string
	conflict []string
	partial  []commitPartialSelection
}

// commitPartialSelection keeps stable hunk identities chosen in the dialog.
// The ranges are resolved again after before-commit edits, immediately before
// vcs_commit, so stale line numbers never reach the backend.
type commitPartialSelection struct {
	Path string
	Keys []string
}

// OpenCommit shows the Commit dialog for the project's changes, saving the
// documents first so the commit is made from what is on disk.
func (m *Model) OpenCommit() {
	if m.Root() == "" || m.commit != nil {
		return
	}
	m.commit = m.newCommitState(m.vcsRepositoryRoot())
	m.commit.allRepositories = true
	m.loadPreparedCommitMessage(m.commit)
	if m.vcsCommitNonModal() {
		m.ShowVCS()
		m.SelectVCSTab(vcsContentChanges)
	}
	m.RefreshVCS()
	m.invalidate()
}

// OpenCommitFile shows the Commit dialog for one file, as the tab menu's
// Commit File… does: every other change stays out of the commit.
func (m *Model) OpenCommitFile(path string) {
	if m.Root() == "" || m.commit != nil || path == "" {
		return
	}
	path = realPath(path)
	repo := m.vcsRepositoryRoot()
	if selected := vcsRepositoryContainingPath(path, m.vcs.repositories); selected != "" {
		repo = selected
	}
	m.commit = m.newCommitState(repo)
	m.commit.only = path
	m.loadPreparedCommitMessage(m.commit)
	m.RefreshVCS()
	m.invalidate()
}

// OpenCommitDir shows the Commit dialog for the changes under a directory, as
// the project view's Git › Commit… does: every change outside it stays out of
// the commit, and the project root means all of them.
func (m *Model) OpenCommitDir(dir string) {
	if m.Root() == "" || m.commit != nil || dir == "" {
		return
	}
	dir = realPath(dir)
	repo := m.vcsRepositoryRoot()
	if selected := vcsRepositoryContainingPath(dir, m.vcs.repositories); selected != "" {
		repo = selected
	}
	c := m.newCommitState(repo)
	if dir != realPath(m.Root()) {
		c.scope = dir
	}
	m.commit = c
	m.loadPreparedCommitMessage(c)
	m.RefreshVCS()
	m.invalidate()
}

func (m *Model) newCommitState(repo string) *commitState {
	message := m.commitInitialMessage
	if repo != "" {
		switch {
		case message == "", filepath.Clean(repo) != filepath.Clean(m.Root()):
			message = m.loadCommitDraft(repo)
		}
	}
	return &commitState{
		message:  message,
		signOff:  m.vcsGitBool("vcs.git.signOff"),
		runHooks: true,
		checks:   map[string]bool{},
		hunks:    map[string]*commitHunksState{},
		repo:     repo,
	}
}

// commitVisible reports whether the dialog lists a file: the scope of a
// directory commit keeps every change outside it out of both the list and the
// plan, and ignored files never show.
func commitVisible(c *commitState, f openidea.FileStatus) bool {
	if effectiveVCSKind(f) == "ignored" {
		return false
	}
	return c.scope == "" || underTreePath(f.Path, c.scope)
}

// commitFiles lists the changes the dialog shows, in the order of the changes
// view.
func (m *Model) commitFiles(c *commitState) []openidea.FileStatus {
	out := make([]openidea.FileStatus, 0, len(m.vcs.files))
	for _, f := range m.vcs.files {
		if !m.commitFileInRepositoryScope(c, f.Path) {
			continue
		}
		if commitVisible(c, f) {
			out = append(out, f)
		}
	}
	return out
}

func (m *Model) commitFileInRepositoryScope(c *commitState, path string) bool {
	if c.allRepositories || c.repo == "" {
		return true
	}
	return vcsRepositoryForPath(path, m.vcs.repositories) == filepath.Clean(c.repo)
}

// commitIncluded is a file's inclusion: the user's override, or the initial
// check OpenIDEA uses — changes are included, unversioned files and conflicts
// are opt-in, and a partially staged file commits only what is staged. A
// single-file commit includes only that file, even when it is new.
func (m *Model) commitIncluded(c *commitState, f openidea.FileStatus) bool {
	if !commitVisible(c, f) {
		return false
	}
	if c.onlyPaths != nil && !c.onlyPaths[f.Path] {
		return false
	}
	if c.only != "" {
		if f.Path != c.only {
			return false
		}
		kind := effectiveVCSKind(f)
		return kind != "ignored" && kind != "" && kind != "conflicted"
	}
	if state := c.hunks[f.Path]; state != nil {
		selected, total, ok := m.commitHunkChoice(c, f.Path)
		return ok && total > 0 && selected > 0
	}
	if on, ok := c.checks[f.Path]; ok {
		return on
	}
	kind := effectiveVCSKind(f)
	if kind == "ignored" || kind == "" {
		return false
	}
	if kind == "untracked" || kind == "conflicted" {
		return false
	}
	return true
}

// partiallyStaged reports whether a file has both an index entry and further
// working-tree changes, whose initial inclusion covers only the index side.
func partiallyStaged(f openidea.FileStatus) bool {
	if f.Staged == nil || f.Unstaged == nil {
		return false
	}
	switch *f.Unstaged {
	case "untracked", "conflicted":
		return false
	}
	return *f.Staged != "conflicted"
}

// stagedOnly reports whether an included file is committed as it is staged.
func stagedOnly(c *commitState, f openidea.FileStatus) bool {
	if _, ok := c.checks[f.Path]; ok {
		return false
	}
	return partiallyStaged(f)
}

// planCommit derives the vcs_commit options from the inclusion, mirroring
// OpenIDEA's planCommit: included unstaged files are staged, excluded staged
// files stay staged, and a staged rename's source is left out with it.
func (m *Model) planCommit(c *commitState) commitPlan {
	var plan commitPlan
	for _, f := range m.vcs.files {
		if !m.commitFileInRepositoryScope(c, f.Path) {
			continue
		}
		kind := effectiveVCSKind(f)
		if kind == "ignored" || kind == "" {
			continue
		}
		if !m.commitIncluded(c, f) {
			if f.Staged != nil && *f.Staged != "conflicted" {
				plan.exclude = append(plan.exclude, f.Path)
				if from := renamedFromPath(f); from != "" {
					plan.exclude = append(plan.exclude, from)
				}
			}
			continue
		}
		if kind == "conflicted" {
			plan.conflict = append(plan.conflict, f.RelPath)
			continue
		}
		if selected, total, ok := m.commitHunkChoice(c, f.Path); ok && total > 0 && selected < total {
			state := c.hunks[f.Path]
			keys := state.selectedKeys()
			plan.partial = append(plan.partial, commitPartialSelection{
				Path: f.Path,
				Keys: keys,
			})
			plan.included = append(plan.included, f.Path)
			continue
		}
		if f.Unstaged != nil && !stagedOnly(c, f) {
			plan.stage = append(plan.stage, f.Path)
		}
		plan.included = append(plan.included, f.Path)
	}
	return plan
}

// submitCommit saves the documents and commits the planned files with
// vcs_commit; the backend copies the index, stages the included files and
// keeps the excluded ones staged.
func (m *Model) submitCommit() {
	c := m.commit
	if c == nil || c.busy {
		return
	}
	message := strings.TrimSpace(c.message)
	switch {
	case message == "" && !c.amend:
		c.err = "Enter a commit message"
		m.invalidate()
		return
	case len(m.commitFiles(c)) == 0 && !c.amend:
		c.err = "There are no changes to commit"
		m.invalidate()
		return
	}
	for path, state := range c.hunks {
		if state == nil {
			continue
		}
		if state.loading {
			c.err = "Wait for the line changes for " + filepath.Base(path) + " to finish loading"
			m.invalidate()
			return
		}
		if state.err != "" {
			c.err = "Line changes for " + filepath.Base(path) + " could not be loaded: " + state.err
			m.invalidate()
			return
		}
	}
	plan := m.planCommit(c)
	if len(plan.conflict) > 0 {
		c.err = "Resolve the conflicts first: " + strings.Join(plan.conflict, "、")
		m.invalidate()
		return
	}
	if len(plan.included) == 0 && !c.amend {
		c.err = "No changes are selected for the commit"
		m.invalidate()
		return
	}
	if !c.problemsOverride {
		if problems := m.vcsCommitCheckProblems(c, plan); len(problems) > 0 {
			c.problems = problems
			c.problemsOverride = true
			c.err = ""
			m.invalidate()
			return
		}
	}
	c.busy = true
	c.err = ""
	c.problems = nil
	m.status = "Committing…"
	m.invalidate()
	tabs := m.allTabs()
	modifications := m.vcsCommitModifications()
	existing := commitExistingPaths(commitFilesForPlan(m.commitFiles(c), plan))
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 3*time.Minute)
		defer cancel()
		m.modifyCommitFiles(ctx, tabs, existing, modifications)
		if err := m.saveOpenTabs(ctx, tabs); err != nil {
			m.finishCommitResult(c, commitRunResult{err: err})
			return
		}
		resolved, err := m.resolvePartialCommitPlan(ctx, plan)
		if err != nil {
			m.finishCommitResult(c, commitRunResult{err: err})
			return
		}
		repositories := m.commitRepositories(c, resolved)
		result := commitRunResult{totalRepositories: len(repositories)}
		for _, repository := range repositories {
			options := m.commitOptionsForRepository(c, resolved, repository)
			raw, commitErr := m.invoke(ctx, "vcs_commit", map[string]any{
				"repo": vcsRepositoryArgFor(repository), "message": message, "options": options,
			})
			var id string
			if commitErr == nil {
				commitErr = json.Unmarshal(raw, &id)
			}
			if commitErr != nil {
				result.err = commitErr
				break
			}
			result.id = id
			result.repositories = append(result.repositories, repository)
		}
		result.committedRepositories = len(result.repositories)
		m.finishCommitResult(c, result)
	}()
}

func (m *Model) finishCommit(c *commitState, id string, err error) {
	m.finishCommitResult(c, commitRunResult{
		id: id, err: err, totalRepositories: 1,
	})
}

// commitRunResult records what a sequential multi-repository commit completed
// before a later repository failed.
type commitRunResult struct {
	id                    string
	err                   error
	repositories          []string
	committedRepositories int
	totalRepositories     int
}

func (m *Model) finishCommitResult(c *commitState, result commitRunResult) {
	m.post(func() {
		if c != m.commit {
			return
		}
		c.busy = false
		if result.err != nil {
			if result.committedRepositories > 0 {
				c.err = fmt.Sprintf("Committed %d of %d repositories before the next commit failed: %s",
					result.committedRepositories, result.totalRepositories, commitErrorText(result.err))
				m.status = fmt.Sprintf("Committed %d repositories; the remaining commit failed",
					result.committedRepositories)
				m.RefreshVCS()
			} else {
				c.err = commitErrorText(result.err)
				m.status = "Commit failed"
			}
			m.invalidate()
			return
		}
		m.commit = nil
		repositories := append([]string(nil), result.repositories...)
		if len(repositories) == 0 {
			repositories = []string{c.repo}
		}
		for _, repository := range repositories {
			m.rememberCommitMessage(repository, c.message)
			if m.vcsCommitClearInitialMessage() {
				m.keepCommitDraft(repository, "")
			} else {
				m.keepCommitDraft(repository, c.message)
			}
		}
		summary := firstLine(c.message)
		if summary == "" {
			summary = "Amend the last commit"
		}
		if len(repositories) > 1 {
			m.status = fmt.Sprintf("Committed in %d repositories: %s", len(repositories), summary)
		} else {
			m.status = fmt.Sprintf("Committed %s: %s", shortID(result.id), summary)
		}
		m.RefreshVCS()
		if c.pushAfter {
			if m.vcsPushDialogNeeded(m.vcsCurrentBranch()) {
				m.OpenGitPush()
			} else {
				m.pushAfterCommitWithoutDialog(repositories[len(repositories)-1])
			}
		}
		m.invalidate()
	})
}

// SmokeCommit commits the project's changed files with a message and verifies
// the status afterwards; it is the offscreen check of the commit path.
func (m *Model) SmokeCommit(ctx context.Context) (map[string]any, error) {
	status := func() ([]openidea.FileStatus, error) {
		raw, err := m.mgr.Invoke(ctx, Window, "vcs_status", nil)
		if err != nil {
			return nil, err
		}
		var files []openidea.FileStatus
		if err := json.Unmarshal(raw, &files); err != nil {
			return nil, err
		}
		return files, nil
	}
	before, err := status()
	if err != nil {
		return nil, err
	}
	m.vcs.files = before
	c := &commitState{message: "e2e: commit the change", runHooks: true, checks: map[string]bool{}}
	plan := m.planCommit(c)
	if len(plan.included) == 0 {
		return map[string]any{"included": 0}, nil
	}
	options := openidea.CommitOptions{
		RunHooks: c.runHooks, Stage: plan.stage, Exclude: plan.exclude,
	}
	raw, err := m.mgr.Invoke(ctx, Window, "vcs_commit", map[string]any{
		"repo": nil, "message": c.message, "options": options,
	})
	if err != nil {
		return map[string]any{"included": len(plan.included)}, err
	}
	var id string
	if err := json.Unmarshal(raw, &id); err != nil {
		return nil, err
	}
	after, err := status()
	if err != nil {
		return nil, err
	}
	remaining := make([]string, 0, len(after))
	for _, f := range after {
		remaining = append(remaining, f.RelPath)
	}
	cut := len(id)
	if cut > 12 {
		cut = 12
	}
	return map[string]any{
		"id": id, "shortId": id[:cut], "message": c.message,
		"staged": len(plan.stage), "excluded": len(plan.exclude),
		"before": len(before), "after": len(after), "remaining": remaining,
	}, nil
}

// effectiveVCSKind is the change kind a status row shows, working-tree side
// first, as OpenIDEA's effectiveKind derives it.
func effectiveVCSKind(f openidea.FileStatus) string {
	if f.Staged != nil && *f.Staged == "conflicted" || f.Unstaged != nil && *f.Unstaged == "conflicted" {
		return "conflicted"
	}
	if f.Unstaged != nil {
		if *f.Unstaged == "untracked" {
			if f.Staged != nil {
				return *f.Staged
			}
			return "untracked"
		}
		if *f.Unstaged == "ignored" {
			return "ignored"
		}
	}
	if f.Staged != nil && *f.Staged == "added" {
		if f.Unstaged != nil && *f.Unstaged == "deleted" {
			return ""
		}
		return "added"
	}
	if f.Unstaged != nil && *f.Unstaged == "deleted" || f.Staged != nil && *f.Staged == "deleted" {
		return "deleted"
	}
	if f.Staged != nil && *f.Staged == "renamed" {
		return "renamed"
	}
	if f.Staged != nil || f.Unstaged != nil {
		return "modified"
	}
	return ""
}

// renamedFromPath is the absolute path a staged rename came from, which the
// commit leaves out together with the new path.
func renamedFromPath(f openidea.FileStatus) string {
	if f.RenamedFrom == nil || *f.RenamedFrom == "" {
		return ""
	}
	path := strings.TrimSuffix(f.Path, "/")
	rel := strings.TrimSuffix(f.RelPath, "/")
	if !strings.HasSuffix(path, rel) {
		return ""
	}
	return strings.TrimSuffix(path, rel) + *f.RenamedFrom
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func firstLine(text string) string {
	text = strings.TrimSpace(text)
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		text = strings.TrimSpace(text[:i])
	}
	return text
}

// commitErrorText turns the refusals a commit can hit into the dialog's words.
func commitErrorText(err error) string {
	if openidea.IsOutOfSync(err) {
		return "The file changed on disk; refresh and try again"
	}
	return "Commit failed: " + err.Error()
}

// commitDialog draws the Commit dialog over the window.
func (m *Model) commitDialog(c *ui.Context, p *ui.Theme) {
	s := m.commit
	if s == nil {
		return
	}
	open := true
	d := ui.Modal(c, &open, func() {
		files := m.commitFiles(s)
		ui.Column(c).Width(560).Gap(8).Children(func() {
			ui.Text(c, "Commit Changes").FontSize(13).Bold().TextColor(p.Text)
			m.commitMessageEditor(c, p, s, false)
			ui.Text(c, fmt.Sprintf("%s selected", count(len(m.planCommit(s).included), "file", "files"))).
				FontSize(12).TextColor(p.TextMuted)
			if len(files) == 0 {
				ui.Text(c, "There are no changes to commit").FontSize(12).TextColor(p.TextMuted)
			} else {
				ui.Scroll(c).Height(220).Children(func() {
					for i, f := range files {
						i, f := i, f
						on := m.commitIncluded(s, f)
						selected, total, hasHunks := m.commitHunkChoice(s, f.Path)
						partlySelected := hasHunks && selected > 0 && selected < total
						row := ui.Row(c.Key(fmt.Sprintf("commit:%d", i))).Gap(4).AlignItems(ui.Center)
						_, word, _ := vcsBadge(f)
						row.Tooltip(word)
						row.Children(func() {
							if partlySelected {
								if ui.Button(c, "◩").Tooltip("Partially selected; include all hunks").Clicked() {
									m.toggleCommitFileHunks(f.Path, true)
								}
							} else if ui.Checkbox(c, &on, "").Changed() {
								m.toggleCommitFileHunks(f.Path, on)
							}
							fileIconEl(c, oiPaletteOf(p), f.Path, 16, p.TextMuted)
							ui.Text(c, filepath.Base(f.RelPath)).FontSize(13).
								TextColor(vcsKindColor(p, f)).SingleLine().Ellipsis("…")
							if commitFileCanChooseHunks(f) {
								label := "Hunks…"
								if state := s.hunks[f.Path]; state != nil && state.loading {
									label = "Reading…"
								}
								if ui.Button(c, label).Clicked() {
									m.openCommitHunks(f.Path)
								}
							}
						})
						if !partlySelected && on != m.commitIncluded(s, f) {
							s.checks[f.Path] = on
							s.problems, s.problemsOverride = nil, false
							m.invalidate()
						}
					}
				})
			}
			ui.Row(c).Gap(12).Children(func() {
				m.commitOptions(c, s)
			})
			m.commitFooter(c, p, s, len(files) == 0)
		})
	})
	if d.Dismissed() || !open {
		if !s.busy {
			m.commit = nil
			m.invalidate()
		}
	}
	m.commitHunksDialog(c, p)
}

func (m *Model) commitMessageEditor(c *ui.Context, p *ui.Theme, s *commitState, inline bool) {
	height := float32(84)
	if inline {
		height = 72
	}
	input := ui.TextArea(c, &s.message).Height(height)
	if input.Changed() {
		s.problems, s.problemsOverride = nil, false
		settings := m.vcsCommitMessageSettings()
		if m.nativeRuntimeBool(settingsPageVCSCommit, "vcs.commit.wrapAtMargin") &&
			settings.limitBody {
			s.message = wrapVCSCommitMessageAt(s.message, settings.bodyLimit)
		}
	}
	settings := m.vcsCommitMessageSettings()
	subject := s.message
	if newline := strings.IndexByte(subject, '\n'); newline >= 0 {
		subject = subject[:newline]
	}
	hint := "Commit message, for example: Fix the login timeout"
	if settings.limitSubject {
		hint = fmt.Sprintf("Subject line %d/%d", utf8.RuneCountInString(subject), settings.subjectLimit)
	}
	if m.nativeRuntimeBool(settingsPageVCSCommit, "vcs.commit.showRightMargin") {
		margin := settings.subjectLimit
		if settings.limitBody {
			margin = settings.bodyLimit
		}
		hint += fmt.Sprintf(" · margin %d", margin)
	}
	ui.Text(c, hint).FontSize(12).TextColor(p.TextMuted).SingleLine()
	for _, problem := range m.vcsCommitMessageProblems(s.message) {
		ui.Text(c, problem).FontSize(11).TextColor(p.Warning).SingleLine()
	}
	for _, problem := range s.problems {
		ui.Text(c, problem).FontSize(11).TextColor(p.Warning)
	}
}

func (m *Model) commitOptions(c *ui.Context, s *commitState) {
	amend := s.amend
	if ui.Checkbox(c, &amend, "Amend the last commit").Changed() {
		m.setCommitAmend(amend)
	}
	m.commitMessageHistoryButton(c, s)
	ui.Checkbox(c, &s.signOff, "Sign-off")
	ui.Checkbox(c, &s.runHooks, "Run Git hooks")
}

func (m *Model) commitFooter(c *ui.Context, p *ui.Theme, s *commitState, noFiles bool) {
	if s.err != "" {
		ui.Text(c, s.err).FontSize(12).TextColor(p.Danger)
	}
	ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
		if s.busy {
			ui.Text(c, "Committing…").FontSize(12).TextColor(p.TextMuted)
		}
		label := "Commit"
		switch {
		case s.problemsOverride:
			label = "Commit Anyway"
		case s.amend:
			label = "Amend Commit"
		case s.pushAfter:
			label = "Commit and Push"
		}
		ui.Button(c, label).Disabled(s.busy || noFiles && !s.amend).OnClick(m.submitCommit)
		ui.Button(c, "Cancel").OnClick(func() {
			m.keepCommitDraft(s.repo, s.message)
			m.commit = nil
			m.invalidate()
		})
	})
}

func (m *Model) pushAfterCommitWithoutDialog(repo string) {
	if m.Root() == "" {
		return
	}
	op := map[string]any{
		"op": "push", "force_with_lease": false,
		"set_upstream": false, "tags": nil,
		"force": false, "no_verify": false,
	}
	m.runVCSRemoteAt(vcsRepositoryArgFor(repo), vcsOpPush, "Push", op)
}
