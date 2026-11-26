package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

// VCSPane is the sidebar's Git view: the changes `git status` reports for every
// repository of the project, loaded through openidea-server.
type VCSPane struct {
	loading bool
	loaded  bool
	// files is the complete status used by project-tree decorations and
	// counts. Each repository's own slice is filtered from it for the Git
	// tool window.
	files        []openidea.FileStatus
	repositories []vcsRepository
	current      string
	// statusKinds and dirtyDirectories are the project-tree decorations,
	// rebuilt whenever status changes so drawing a row stays O(1).
	statusKinds      map[string]string
	dirtyDirectories map[string]bool
	// selectedFiles keeps the changed-file selection of each repository tab.
	selectedFiles map[string]string
	// changeQueries keeps each repository's Local Changes filter.
	changeQueries map[string]string
	// branches and logs keep the tool-window state of every repository. The
	// active m.branch / m.log panes are live copies; switching tabs saves and
	// restores them here so one repository cannot erase another's view.
	branches map[string]BranchPane
	logs     map[string]LogPane
	// branchPages, shelf and stashes keep the dedicated Git content pages.
	// Branch pages are per repository; shelves and stashes are project-wide,
	// with each stash carrying the repository that owns it.
	branchPages map[string]*vcsBranchesPane
	shelf       *vcsShelfPane
	stashes     *vcsStashPane
	// content is the Local Changes / Log / Worktrees tab selected in each
	// repository. OpenIDEA keeps repository selection and content selection
	// separately, so returning to a repository restores the tab it had.
	content map[string]vcsContentTab
	// worktrees is loaded independently for each repository and shared by the
	// inline Worktrees tab and its full native dialog.
	worktrees map[string]*vcsWorktreePane
	// busy/opErr mirror the selected repository for existing UI code. The
	// maps keep staging feedback attached to the repository that started it.
	busyByRepo  map[string]bool
	opErrByRepo map[string]string
	// selectionGeneration invalidates branch reads started for a repository
	// that is no longer selected.
	selectionGeneration uint64
	// refreshGeneration invalidates status refreshes after a project reset or
	// a newer refresh starts.
	refreshGeneration uint64
	refreshCancel     context.CancelFunc
	incomingCancel    context.CancelFunc
	incomingRoot      string
	incomingFailed    bool
	// logGeneration invalidates commit-list and commit-detail responses that
	// belong to a repository the user has already left.
	logGeneration         uint64
	branchInfoGenerations map[string]uint64
	branchListGenerations map[string]uint64
	repoScroll            ui.ScrollState
	err                   string
	busy                  bool
	opErr                 string
	// stale asks for another read once the one in flight lands, so a change
	// made while the list is loading is not lost.
	stale bool
}

// rollbackState is the confirmation a rollback asks for. A selected folder is
// expanded to its changed files so a nested repository or submodule root is
// never deleted as if it were an ordinary untracked folder.
type rollbackState struct {
	files       []openidea.FileStatus
	name        string
	deleteAdded bool
	open        bool
	busy        bool
	err         string
}

func (m *Model) resetVCS() {
	if m.vcs.refreshCancel != nil {
		m.vcs.refreshCancel()
	}
	if m.vcs.incomingCancel != nil {
		m.vcs.incomingCancel()
	}
	cancelVCSBranchPane(&m.branch)
	for _, branch := range m.vcs.branches {
		branch := branch
		cancelVCSBranchPane(&branch)
	}
	for _, page := range m.vcs.branchPages {
		if page != nil && page.cancel != nil {
			page.cancel()
		}
	}
	if m.vcs.shelf != nil && m.vcs.shelf.cancel != nil {
		m.vcs.shelf.cancel()
	}
	if m.vcs.stashes != nil && m.vcs.stashes.cancel != nil {
		m.vcs.stashes.cancel()
	}
	for _, state := range m.vcs.worktrees {
		if state != nil && state.cancel != nil {
			state.cancel()
		}
	}
	m.vcs = VCSPane{}
	m.vcsMore = vcsMoreState{}
	m.recentHistory = recentChangesState{}
}

// RefreshVCS discovers all repositories in the project and reloads their
// changes in the background. The current repository tab stays selected while
// it still exists.
func (m *Model) RefreshVCS() {
	root := m.Root()
	if root == "" || m.mgr == nil && m.invokeHook == nil {
		return
	}
	if m.vcs.loading {
		m.vcs.stale = true
		return
	}
	m.vcs.refreshGeneration++
	generation := m.vcs.refreshGeneration
	ctx, cancel := context.WithTimeout(m.mergeContext(), 30*time.Second)
	m.vcs.refreshCancel = cancel
	m.vcs.loading = true
	m.invalidate()
	go func() {
		defer cancel()
		rootsRaw, rootsErr := m.invoke(ctx, "vcs_roots", nil)
		var repositories []vcsRepository
		if rootsErr == nil {
			rootsErr = json.Unmarshal(rootsRaw, &repositories)
		}
		statusRaw, statusErr := m.invoke(ctx, "vcs_status", nil)
		var files []openidea.FileStatus
		if statusErr == nil {
			statusErr = json.Unmarshal(statusRaw, &files)
		}
		m.post(func() {
			if generation != m.vcs.refreshGeneration {
				return
			}
			m.vcs.loading = false
			m.vcs.refreshCancel = nil
			if m.Root() != root {
				return
			}
			m.vcs.loaded = true
			previous := m.vcs.current
			previousRepositories := append([]vcsRepository(nil), m.vcs.repositories...)
			if rootsErr != nil && !vcsRepositoryNotFound(rootsErr) {
				m.vcs.repositories = nil
				m.vcs.files = nil
				m.vcs.current = ""
				m.vcs.err = "Could not discover Git repositories: " + rootsErr.Error()
			} else {
				m.vcs.repositories = repositories
				if statusErr != nil && !vcsRepositoryNotFound(statusErr) {
					m.vcs.err = vcsErrorText(statusErr)
					m.vcs.files = nil
				} else {
					m.vcs.err = ""
					sort.SliceStable(files, func(i, j int) bool { return files[i].RelPath < files[j].RelPath })
					m.vcs.files = files
					m.rebuildVCSStatusDecorations()
					m.vcs.current = vcsCurrentRepository(m.vcs.current, repositories)
					m.vcs.keepVCSSelections(files)
				}
				m.vcs.current = vcsCurrentRepository(m.vcs.current, repositories)
			}
			if m.vcs.current != previous {
				m.vcs.selectionGeneration++
				m.restoreVCSRepositoryView(previous, m.vcs.current)
				if m.vcs.current != "" {
					m.RefreshBranch()
				}
				if m.log.visible || m.currentVCSContent() == vcsContentLog {
					m.enableVCSAggregateLog(m.vcs.current)
				}
			}
			m.syncProjectTreeState()
			if (m.log.visible || m.currentVCSContent() == vcsContentLog) &&
				m.log.aggregate &&
				!sameVCSRepositoryList(
					vcsRepositoryRoots(previousRepositories),
					m.vcs.repositories,
				) {
				m.log.stale = true
				m.RefreshLog()
			}
			m.refreshLoadedVCSContentPages()
			m.ensureVCSIncomingCheck()
			m.invalidate()
			if m.vcs.stale {
				m.vcs.stale = false
				m.RefreshVCS()
			}
		})
	}()
}

func (m *Model) rebuildVCSStatusDecorations() {
	kinds := make(map[string]string, len(m.vcs.files))
	dirty := map[string]bool{}
	root := filepath.Clean(m.Root())
	for _, file := range m.vcs.files {
		path := filepath.Clean(file.Path)
		kinds[path] = effectiveVCSKind(file)
		for parent := filepath.Dir(path); parent != "." && parent != string(filepath.Separator); parent = filepath.Dir(parent) {
			dirty[parent] = true
			if root != "" && filepath.Clean(parent) == root {
				break
			}
		}
	}
	m.vcs.statusKinds = kinds
	m.vcs.dirtyDirectories = dirty
}

func (m *Model) projectTreeVCSKind(path string, directory bool) string {
	if directory {
		if m.vcsShowDirtyDirectories() && m.vcs.dirtyDirectories[filepath.Clean(path)] {
			return "modified"
		}
		return ""
	}
	return m.vcs.statusKinds[filepath.Clean(path)]
}

func vcsTreeStatusColor(p *ui.Theme, oi oiColors, kind string) ui.Color {
	switch kind {
	case "added", "untracked":
		if kind == "untracked" {
			return oi.vcsUntracked
		}
		return oi.vcsAdded
	case "deleted":
		return oi.vcsDeleted
	case "ignored":
		return oi.vcsIgnored
	case "conflicted":
		return oi.vcsConflict
	case "modified", "renamed", "copied":
		return oi.vcsModified
	default:
		return p.Text
	}
}

// ShowVCS switches the sidebar to the changes view (and reloads it).
func (m *Model) ShowVCS() {
	m.sideMode = "vcs"
	m.leftHidden = false
	if m.mgr != nil || m.invokeHook != nil {
		m.RefreshVCS()
		m.RefreshBranch()
	}
	m.rememberLayout()
	m.invalidate()
}

// SmokeVCS reports the project's Git changes through the backend: the
// offscreen check of the changes view.
func (m *Model) SmokeVCS(ctx context.Context) (int, string, error) {
	raw, err := m.mgr.Invoke(ctx, Window, "vcs_status", nil)
	if err != nil {
		return 0, "", err
	}
	var files []openidea.FileStatus
	if err := json.Unmarshal(raw, &files); err != nil {
		return 0, "", err
	}
	sample := ""
	if len(files) > 0 {
		letter, word, _ := vcsBadge(files[0])
		sample = letter + " " + word + " " + files[0].RelPath
	}
	return len(files), sample, nil
}

// vcsErrorText turns the backend's refusal for a project without Git into the
// sentence the sidebar shows; other failures keep their own message.
func vcsErrorText(err error) string {
	var oe *openidea.Error
	if errors.As(err, &oe) && oe.Kind == "unavailable" && strings.Contains(oe.Message, "not a git repository") {
		return "The current project is not a Git repository"
	}
	return "Could not read the Git status: " + err.Error()
}

// vcsBadge is the label of a status row: a letter as IDEA shows it, the
// change in words, and a tone for the colour. The working tree kind wins when
// both sides changed, since that is the state the file is in now.
func vcsBadge(f openidea.FileStatus) (letter, word, tone string) {
	kind := ""
	if f.Unstaged != nil {
		kind = *f.Unstaged
	} else if f.Staged != nil {
		kind = *f.Staged
	}
	switch kind {
	case "added":
		return "A", "Added", "add"
	case "modified":
		return "M", "Modified", "mod"
	case "deleted":
		return "D", "Deleted", "del"
	case "renamed":
		return "R", "Renamed", "add"
	case "conflicted":
		return "!", "Conflict", "del"
	case "untracked":
		return "U", "Untracked", "mod"
	case "ignored":
		return "I", "Ignored", "muted"
	}
	return "?", "Changed", "mod"
}

// vcsKindColor is the colour of a change's name, as OpenIDEA's KIND_COLOR
// paints it.
func vcsKindColor(p *ui.Theme, f openidea.FileStatus) ui.Color {
	oi := oiPaletteOf(p)
	switch effectiveVCSKind(f) {
	case "added":
		return oi.vcsAdded
	case "deleted":
		return oi.vcsDeleted
	case "untracked":
		return oi.vcsUntracked
	case "ignored":
		return oi.vcsIgnored
	case "conflicted":
		return oi.vcsConflict
	}
	return oi.vcsModified
}

// vcsStaged and vcsUnstaged split the changes the way OpenIDEA's changes view
// groups them; a file staged and then edited again is in both.
func vcsStaged(files []openidea.FileStatus) []openidea.FileStatus {
	out := make([]openidea.FileStatus, 0, len(files))
	for _, f := range files {
		if f.Staged != nil {
			out = append(out, f)
		}
	}
	return out
}

func vcsUnstaged(files []openidea.FileStatus) []openidea.FileStatus {
	out := make([]openidea.FileStatus, 0, len(files))
	for _, f := range files {
		if f.Unstaged != nil {
			out = append(out, f)
		}
	}
	return out
}

// StageChange stages one file from the changes view.
func (m *Model) StageChange(f openidea.FileStatus) { m.changeOp("vcs_stage", f, "Staged ") }

// AddToIgnore adds path to the shared .gitignore or, with destination
// "localExclude", to .git/info/exclude, as the project view's Git › Add to
// .gitignore does.
func (m *Model) AddToIgnore(path, destination string) {
	root := m.vcsRepositoryArgForPath(path)
	repoRoot, _ := root.(string)
	if m.Root() == "" || path == "" || m.vcsRepositoryBusy(repoRoot) {
		return
	}
	name := filepath.Base(path)
	where := ".gitignore"
	if destination == "localExclude" {
		where = ".git/info/exclude"
	}
	m.setVCSRepositoryBusy(repoRoot, true)
	m.setVCSRepositoryOpErr(repoRoot, "")
	m.status = "Adding " + name + " to " + where + "…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, time.Minute)
		defer cancel()
		_, err := m.invoke(ctx, "vcs_add_to_ignore", map[string]any{
			"paths": []string{path}, "destination": destination,
		})
		m.post(func() {
			m.setVCSRepositoryBusy(repoRoot, false)
			if m.Root() == "" {
				return
			}
			if err != nil {
				m.setVCSRepositoryOpErr(repoRoot,
					"Could not add it to the ignore list: "+err.Error())
				m.invalidate()
				return
			}
			m.status = "Added " + name + " to " + where
			m.RefreshVCS()
			m.invalidate()
		})
	}()
}

// statusFor is the change `git status` reported for path, if the loaded list
// has one; the tab menu's Git actions read it to enable themselves.
func (m *Model) statusFor(path string) (openidea.FileStatus, bool) {
	for _, f := range m.vcs.files {
		if f.Path == path {
			return f, true
		}
	}
	return openidea.FileStatus{}, false
}

// UnstageChange takes one file back out of the index.
func (m *Model) UnstageChange(f openidea.FileStatus) {
	m.changeOp("vcs_unstage", f, "Unstaged ")
}

func (m *Model) changeOp(command string, f openidea.FileStatus, done string) {
	root := filepath.Clean(vcsRepositoryContainingPath(realPath(f.Path), m.vcs.repositories))
	if root == "" {
		root = m.vcsRepositoryRoot()
	}
	if m.Root() == "" || m.vcsRepositoryBusy(root) {
		return
	}
	m.setVCSRepositoryBusy(root, true)
	m.setVCSRepositoryOpErr(root, "")
	m.status = "Updating " + filepath.Base(f.RelPath) + "…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, time.Minute)
		defer cancel()
		_, err := m.invoke(ctx, command, map[string]any{"paths": []string{f.Path}})
		m.post(func() {
			m.setVCSRepositoryBusy(root, false)
			if m.Root() == "" {
				return
			}
			if err != nil {
				m.setVCSRepositoryOpErr(root,
					"The Git operation failed: "+err.Error())
				m.invalidate()
				return
			}
			m.status = done + filepath.Base(f.RelPath)
			m.RefreshVCS()
			m.invalidate()
		})
	}()
}

type rollbackPlan struct {
	unstage []string
	discard []string
}

// planRollback mirrors OpenIDEA: staged changes leave the index first, renames
// discard both sides, and a staged addition keeps its local copy unless the
// user explicitly chooses to delete it.
func planRollback(files []openidea.FileStatus, deleteAdded bool) rollbackPlan {
	unstage := make([]string, 0, len(files)*2)
	discard := make([]string, 0, len(files)*2)
	seenUnstage := map[string]bool{}
	seenDiscard := map[string]bool{}
	add := func(out *[]string, seen map[string]bool, path string) {
		path = filepath.Clean(path)
		if path == "" || path == "." || seen[path] {
			return
		}
		seen[path] = true
		*out = append(*out, path)
	}
	for _, file := range files {
		from := ""
		if file.Staged != nil && *file.Staged == "renamed" {
			from = renamedFromPath(file)
		}
		if file.Staged != nil {
			add(&unstage, seenUnstage, file.Path)
			if from != "" {
				add(&unstage, seenUnstage, from)
			}
		}
		if from != "" {
			add(&discard, seenDiscard, file.Path)
			add(&discard, seenDiscard, from)
			continue
		}
		if file.Staged != nil && *file.Staged == "added" && !deleteAdded {
			continue
		}
		add(&discard, seenDiscard, file.Path)
	}
	return rollbackPlan{unstage: unstage, discard: discard}
}

func rollbackHasAddedFiles(files []openidea.FileStatus) bool {
	for _, file := range files {
		if file.Staged != nil && *file.Staged == "added" {
			return true
		}
	}
	return false
}

// rollbackFilesForSelection expands a folder selection and leaves a nested
// repository or submodule root to its own repository tab.
func (m *Model) rollbackFilesForSelection(file openidea.FileStatus) []openidea.FileStatus {
	path := filepath.Clean(realPath(file.Path))
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return []openidea.FileStatus{file}
	}
	out := make([]openidea.FileStatus, 0)
	seen := map[string]bool{}
	for _, changed := range m.vcs.files {
		changedPath := filepath.Clean(changed.Path)
		if !underTreePath(changedPath, path) && changedPath != path {
			continue
		}
		if m.rollbackNestedRepositoryEntry(changedPath) {
			continue
		}
		if seen[changedPath] {
			continue
		}
		seen[changedPath] = true
		out = append(out, changed)
	}
	return out
}

func (m *Model) rollbackNestedRepositoryEntry(path string) bool {
	path = filepath.Clean(path)
	for _, repository := range m.vcs.repositories {
		if path == filepath.Clean(repository.Root) {
			return true
		}
	}
	return false
}

// AskRollback opens the confirmation that discards the selected working-tree
// changes.
func (m *Model) AskRollback(f openidea.FileStatus) {
	files := m.rollbackFilesForSelection(f)
	if len(files) == 0 {
		m.status = "No file changes to roll back in " + f.RelPath
		m.invalidate()
		return
	}
	name := f.RelPath
	if len(files) > 1 {
		name = fmt.Sprintf("%s (%d changes)", f.RelPath, len(files))
	}
	m.rollback = &rollbackState{files: files, name: name, open: true}
	m.invalidate()
}

func (m *Model) closeRollback() {
	m.rollback = nil
	m.invalidate()
}

// ConfirmRollback unstages first, then discards the planned paths.
func (m *Model) ConfirmRollback() {
	r := m.rollback
	if r == nil || r.busy || m.Root() == "" {
		return
	}
	plan := planRollback(r.files, r.deleteAdded)
	if len(plan.unstage) == 0 && len(plan.discard) == 0 {
		r.err = "These changes have nothing local to remove"
		m.invalidate()
		return
	}
	r.busy = true
	r.err = ""
	m.invalidate()
	tabs := m.allTabs()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, time.Minute)
		defer cancel()
		if err := m.saveOpenTabs(ctx, tabs); err != nil {
			m.post(func() {
				r.busy = false
				r.err = "Rollback failed while saving files: " + err.Error()
				m.invalidate()
			})
			return
		}
		var err error
		if len(plan.unstage) > 0 {
			_, err = m.invoke(ctx, "vcs_unstage", map[string]any{"paths": plan.unstage})
		}
		if err == nil && len(plan.discard) > 0 {
			_, err = m.invoke(ctx, "vcs_discard", map[string]any{"paths": plan.discard})
		}
		m.post(func() {
			r.busy = false
			if m.Root() == "" {
				return
			}
			if err != nil {
				r.err = "Rollback failed: " + err.Error()
				m.invalidate()
				return
			}
			if m.rollback == r {
				m.rollback = nil
			}
			m.status = fmt.Sprintf("Rolled back %s", count(len(r.files), "file", "files"))
			m.RefreshVCS()
			m.invalidate()
		})
	}()
}

// rollbackDialog asks before the discard.
func (m *Model) rollbackDialog(c *ui.Context, p *ui.Theme) {
	r := m.rollback
	if r == nil {
		return
	}
	open := r.open
	d := nativeModal(c, &open, func() {
		ui.Column(c).Width(420).Gap(10).Children(func() {
			ui.Text(c, "Roll Back Changes").FontSize(13).Bold().TextColor(p.Text).SingleLine()
			ui.Text(c, "Roll back "+r.name+"? Unstaged changes are lost.").FontSize(12).
				TextColor(p.TextMuted)
			if rollbackHasAddedFiles(r.files) {
				if ui.Checkbox(c, &r.deleteAdded, "Delete local copies of added files").Changed() {
					r.err = ""
					m.invalidate()
				}
			}
			if r.err != "" {
				ui.Text(c, r.err).FontSize(12).TextColor(p.Danger).SingleLine().Ellipsis("…")
			}
			ui.Row(c).Gap(8).Children(func() {
				label := "Roll Back"
				if r.busy {
					label = "Rolling back…"
				}
				ui.Button(c, label).Disabled(r.busy).OnClick(m.ConfirmRollback)
				ui.Button(c, "Cancel").OnClick(m.closeRollback)
			})
		})
	})
	if d.Dismissed() || !open {
		m.closeRollback()
	}
}

// SmokeStage proves the changes view's actions on a scratch file that is
// staged, unstaged and then discarded.
func (m *Model) SmokeStage(ctx context.Context) (map[string]any, error) {
	root := m.Root()
	if root == "" {
		return nil, errors.New("No project is open")
	}
	tmp := filepath.Join(root, "goidea-stage-smoke.txt")
	if err := os.WriteFile(tmp, []byte("stage smoke\n"), 0o644); err != nil {
		return nil, err
	}
	out := map[string]any{}
	paths := map[string]any{"paths": []string{tmp}}
	if _, err := m.mgr.Invoke(ctx, Window, "vcs_stage", paths); err != nil {
		return out, err
	}
	out["staged"] = m.smokeStaged(ctx, tmp)
	if _, err := m.mgr.Invoke(ctx, Window, "vcs_unstage", paths); err != nil {
		return out, err
	}
	out["unstaged"] = !m.smokeStaged(ctx, tmp)
	if _, err := m.mgr.Invoke(ctx, Window, "vcs_discard", paths); err != nil {
		return out, err
	}
	_, statErr := os.Stat(tmp)
	out["discarded"] = os.IsNotExist(statErr)
	return out, nil
}

func (m *Model) smokeStaged(ctx context.Context, path string) bool {
	raw, err := m.mgr.Invoke(ctx, Window, "vcs_status", nil)
	if err != nil {
		return false
	}
	var files []openidea.FileStatus
	if json.Unmarshal(raw, &files) != nil {
		return false
	}
	for _, f := range files {
		if f.Path == path {
			return f.Staged != nil
		}
	}
	return false
}
