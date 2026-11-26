package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

const maxVCSOutputLines = 300

const (
	vcsOpNewBranch = "newBranch"
	vcsOpNewTag    = "newTag"
	vcsOpMerge     = "merge"
	vcsOpRebase    = "rebase"
	vcsOpStash     = "stash"
	vcsOpUnstash   = "unstash"
	vcsOpFetch     = "fetch"
	vcsOpPull      = "pull"
	vcsOpPush      = "push"
	vcsOpReset     = "reset"
)

type vcsOperationChoice struct {
	label  string
	value  string
	detail string
}

type vcsQuickItem struct {
	id     string
	title  string
	detail string
}

type vcsQuickList struct {
	open  bool
	query string
	sel   int
}

var vcsQuickItems = []vcsQuickItem{
	{id: "vcs.newBranch", title: "New Branch…", detail: "Create and check out a branch"},
	{id: "vcs.newTag", title: "New Tag…", detail: "Tag the selected revision"},
	{id: "vcs.commit", title: "Commit…", detail: "Review and commit changes"},
	{id: "vcs.fetch", title: "Fetch", detail: "Download remote refs"},
	{id: "vcs.pull", title: "Pull…", detail: "Update the current branch"},
	{id: "vcs.push", title: "Push…", detail: "Publish the current branch"},
	{id: "vcs.manageRemotes", title: "Manage Remotes…", detail: "Edit remote repositories"},
	{id: "vcs.compareWithBranch", title: "Compare with Branch or Revision…", detail: "Compare paths with a revision"},
	{id: "vcs.merge", title: "Merge…", detail: "Merge a branch or tag"},
	{id: "vcs.rebase", title: "Rebase…", detail: "Rebase onto a branch or tag"},
	{id: "vcs.checkoutRevision", title: "Checkout Tag or Revision…", detail: "Check out a ref"},
	{id: "vcs.worktrees", title: "Worktrees…", detail: "Open and manage worktrees"},
	{id: "vcs.newWorktree", title: "New Worktree…", detail: "Create a linked worktree"},
	{id: "vcs.stash", title: "Stash Changes…", detail: "Store local changes"},
	{id: "vcs.unstash", title: "Unstash Changes…", detail: "Restore stashed changes"},
	{id: "vcs.resetHead", title: "Reset HEAD…", detail: "Move the current branch"},
	{id: "vcs.resolveConflicts", title: "Resolve Conflicts…", detail: "Resolve merge conflicts"},
	{id: "vcs.log", title: "Show Git Log", detail: "Browse commits and changes"},
}

// VCSOperationPane is the native Git operations sheet. It is shared by the
// Git menu, project and editor Git menus, command palette and toolbar.
type VCSOperationPane struct {
	kind  string
	open  bool
	title string
	hint  string
	label string
	name  string
	// allRepositories applies the operation to every repository in the project.
	// Calls that name a repository keep their single-repository behaviour.
	allRepositories bool
	// repo is the repository this operation targets. Git Log actions can
	// operate on a commit from another repository without changing the
	// currently selected Git tool-window tab.
	repo string

	message          string
	includeUntracked bool
	keepIndex        bool
	rebase           bool
	force            bool
	setUpstream      bool
	resetMode        int

	choices []vcsOperationChoice
	sel     int
	loading bool
	busy    bool
	err     string
	output  []string

	request uint32
	channel uint32
	remotes *vcsRemotesPane
	// pushRejected is the native Merge/Rebase decision shown after a
	// non-fast-forward Push, before retrying the same Push.
	pushRejected *vcsPushRejectedState
	// multiRepoRollback records a partially completed Merge or Rebase. Only
	// repositories that completed successfully are eligible for Rollback.
	multiRepoRollback *vcsMultiRepoRollbackState
	// preferredStart is the branch/tag selected by a Git Log or Branches pane
	// action before the shared revision chooser finishes loading.
	preferredStart string
	// pushRevision is the selected Git Log boundary for "Push All up to
	// Here"; it is sent as pushBranch.revision rather than replacing HEAD.
	pushRevision string
	// pushRepository is the repository that owned the selected Git Log
	// commit when Push All up to Here opened. It prevents a stale selection
	// from being retargeted after the repository list is refreshed.
	pushRepository string
}

type vcsPushRejectedState struct {
	repo     string
	op       map[string]any
	detail   string
	errText  string
	branch   string
	merges   bool
	lease    bool
	remember bool
	attempts int
	busy     bool
	err      string
}

type vcsRepoHeadChange struct {
	root   string
	before string
	after  string
}

type vcsRepositoryFailure struct {
	root string
	err  error
}

type vcsMultiRepoHistoryResult struct {
	completed []vcsRepoHeadChange
	failures  []vcsRepositoryFailure
	attempted []string
}

type vcsMultiRepoRollbackState struct {
	kind        string
	target      string
	failureRoot string
	failureText string
	completed   []vcsRepoHeadChange
	busy        bool
	err         string
}

type vcsPushRepositoryPlan struct {
	root        string
	branch      string
	remote      string
	target      string
	setUpstream bool
	op          map[string]any
}

type vcsPushRepositoriesResult struct {
	completed []vcsPushRepositoryPlan
	failures  []vcsRepositoryFailure
	attempted []string
}

type vcsRemoteRepositoriesResult struct {
	completed []string
	skipped   []string
	failures  []vcsRepositoryFailure
	attempted []string
}

func (m *Model) resetVCSOperations() { m.vcsOps = VCSOperationPane{} }

// OpenVCSOperations opens OpenIDEA's VCS Operations quick list. The list
// dispatches to the same native commands as the Git menus.
func (m *Model) OpenVCSOperations() {
	if m.Root() == "" {
		return
	}
	if m.vcsQuick.open {
		m.closeVCSQuickList()
		return
	}
	m.vcsQuick = vcsQuickList{open: true}
	m.invalidate()
}

func (m *Model) closeVCSQuickList() {
	m.vcsQuick = vcsQuickList{}
	m.invalidate()
}

func (m *Model) vcsQuickRows() []vcsQuickItem {
	query := strings.ToLower(strings.TrimSpace(m.vcsQuick.query))
	rows := make([]vcsQuickItem, 0, len(vcsQuickItems))
	for _, item := range vcsQuickItems {
		if !m.ActionEnabled(item.id) {
			continue
		}
		haystack := strings.ToLower(item.title + " " + item.detail)
		if query != "" && !strings.Contains(haystack, query) {
			continue
		}
		rows = append(rows, item)
	}
	return rows
}

func (m *Model) stepVCSQuick(delta int) {
	rows := m.vcsQuickRows()
	if len(rows) == 0 {
		return
	}
	m.vcsQuick.sel = clampSymbolSel(m.vcsQuick.sel+delta, len(rows))
	m.invalidate()
}

func (m *Model) activateVCSQuick() {
	rows := m.vcsQuickRows()
	if len(rows) == 0 {
		return
	}
	item := rows[clampSymbolSel(m.vcsQuick.sel, len(rows))]
	m.closeVCSQuickList()
	m.RunAction(item.id)
}

func (m *Model) vcsQuickListDialog(c *ui.Context, p *ui.Theme) {
	if !m.vcsQuick.open {
		return
	}
	open := m.vcsQuick.open
	rows := m.vcsQuickRows()
	if m.vcsQuick.sel >= len(rows) {
		m.vcsQuick.sel = max(0, len(rows)-1)
	}
	nativeModal(c, &open, func() {
		ui.Column(c).Width(540).Gap(8).Children(func() {
			ui.Text(c, "VCS Operations").FontSize(13).Bold().TextColor(p.Text).SingleLine()
			in := ui.TextInput(c, &m.vcsQuick.query).AutoFocus()
			in.OnShortcut(0, ui.KeyDown, func() { m.stepVCSQuick(1) })
			in.OnShortcut(0, ui.KeyUp, func() { m.stepVCSQuick(-1) })
			if in.Submitted() {
				m.activateVCSQuick()
			}
			ui.Scroll(c).Height(330).Children(func() {
				ui.Column(c).Children(func() {
					if len(rows) == 0 {
						ui.Text(c, "No matching VCS operations").FontSize(12).TextColor(p.TextMuted)
					}
					for i, item := range rows {
						i, item := i, item
						r := ui.Row(c.Key("vcs.quick:"+item.id)).Height(30).Padding(0, 9).
							Gap(12).AlignItems(ui.Center)
						if i == m.vcsQuick.sel {
							r.Background(p.SurfaceHover)
						} else if r.Hovered() {
							r.Background(p.SurfaceHover)
						}
						r.OnClick(func() {
							m.vcsQuick.sel = i
							m.activateVCSQuick()
						})
						r.Children(func() {
							ui.Text(c, item.title).FontSize(13).TextColor(p.Text).SingleLine()
							ui.Spacer(c)
							ui.Text(c, item.detail).FontSize(12).TextColor(p.TextMuted).SingleLine()
						})
					}
				})
			})
			ui.Row(c).Gap(8).Children(func() {
				ui.Button(c, "Cancel").OnClick(m.closeVCSQuickList)
			})
		})
	})
	if !open {
		m.closeVCSQuickList()
	}
}

func (m *Model) OpenNewGitBranch() {
	m.openVCSOperation(vcsOpNewBranch)
}

// OpenNewGitBranchAt starts the branch chooser at a ref selected outside the
// popup, as the Git Log Branches pane does.
func (m *Model) OpenNewGitBranchAt(ref string) {
	m.openVCSOperation(vcsOpNewBranch)
	m.vcsOps.preferredStart = ref
}

func (m *Model) OpenNewGitTag() {
	m.openVCSOperation(vcsOpNewTag)
}

func (m *Model) OpenGitMerge() {
	m.openVCSOperationAllRepositories(vcsOpMerge)
}

// OpenGitMergeAt opens Merge with a branch selected in the Branches popup.
func (m *Model) OpenGitMergeAt(ref string) {
	m.openVCSOperationAllRepositories(vcsOpMerge)
	m.vcsOps.preferredStart = ref
}

func (m *Model) OpenGitRebase() {
	m.openVCSOperationAllRepositories(vcsOpRebase)
}

// OpenGitRebaseAt opens Rebase with a branch selected in the Branches popup.
func (m *Model) OpenGitRebaseAt(ref string) {
	m.openVCSOperationAllRepositories(vcsOpRebase)
	m.vcsOps.preferredStart = ref
}

func (m *Model) OpenGitStash() {
	m.openVCSOperation(vcsOpStash)
}

func (m *Model) OpenGitUnstash() {
	m.openVCSOperation(vcsOpUnstash)
}

func (m *Model) GitFetch() {
	m.runVCSRemoteAll(vcsOpFetch, "Fetch", map[string]any{"op": "fetch"})
}

func (m *Model) OpenGitPull() {
	if !m.nativeRuntimeBool(settingsPageVCSConfirmation, "vcs.confirmation.showUpdateOptions") {
		m.runVCSRemoteAll(vcsOpPull, "Update Project", map[string]any{
			"op": "pull", "rebase": m.vcsGitUpdateMethod() == "rebase",
		})
		return
	}
	m.openVCSOperationAllRepositories(vcsOpPull)
}

func (m *Model) OpenGitPush() {
	m.openVCSOperationAllRepositories(vcsOpPush)
}

func (m *Model) OpenGitPushAtSelectedCommit() {
	repo := m.gitLogRepositoryRoot()
	revision := ""
	if m.log.selected >= 0 && m.log.selected < len(m.log.commits) {
		repo = m.logCommitRepository(m.log.selected)
		revision = m.log.commits[m.log.selected].ID
	}
	if revision == "" {
		m.status = "Select a commit in the Git Log"
		m.invalidate()
		return
	}
	m.openVCSOperationAt(vcsOpPush, repo)
	m.vcsOps.pushRevision = revision
	m.vcsOps.pushRepository = repo
	m.vcsOps.title = "Push All up to Here"
	m.vcsOps.hint = "Push " + repo + " up to " + shortID(revision) + "."
	m.vcsOps.busy = false
	if !m.vcsRepositoryHasRoot(repo) {
		m.vcsOps.err = "The selected commit's repository is no longer open"
	}
	m.invalidate()
}

func (m *Model) OpenGitReset() {
	m.openVCSOperation(vcsOpReset)
}

func (m *Model) OpenGitResetAtSelectedCommit() {
	repo := m.gitLogRepositoryRoot()
	if m.log.selected >= 0 && m.log.selected < len(m.log.commits) {
		repo = m.logCommitRepository(m.log.selected)
	}
	m.openVCSOperationAt(vcsOpReset, repo)
}

func (m *Model) openVCSOperation(kind string) {
	m.openVCSOperationScoped(kind, m.vcsRepositoryRoot(), false)
}

func (m *Model) openVCSOperationAt(kind, repo string) {
	m.openVCSOperationScoped(kind, repo, false)
}

func (m *Model) openVCSOperationAllRepositories(kind string) {
	m.openVCSOperationScoped(kind, m.vcsRepositoryRoot(), true)
}

func (m *Model) openVCSOperationScoped(kind, repo string, allRepositories bool) {
	if m.Root() == "" {
		return
	}
	repo = vcsRepositoryKey(repo)
	if repo == "" {
		repo = m.vcsRepositoryRoot()
	}
	state := VCSOperationPane{
		kind:             kind,
		open:             true,
		allRepositories:  allRepositories,
		repo:             repo,
		sel:              0,
		includeUntracked: true,
		keepIndex:        false,
	}
	switch kind {
	case vcsOpNewBranch:
		state.title = "Create New Branch"
		state.hint = "The new branch starts at the selected revision."
		state.label = "Branch name"
		m.loadVCSStartChoices(&state)
	case vcsOpNewTag:
		state.title = "Create New Tag"
		state.hint = "Create a tag at the current revision."
		state.label = "Tag name"
		state.choices = []vcsOperationChoice{m.selectedCommitChoiceFor(repo)}
	case vcsOpMerge:
		state.title = "Merge"
		state.hint = "Merge the selected branch or tag into every repository that has it."
		m.loadVCSRevisionChoices(&state, "No branches or tags to merge")
	case vcsOpRebase:
		state.title = "Rebase"
		state.hint = "Rebase every repository that has the selected branch or tag."
		m.loadVCSRevisionChoices(&state, "No branches or tags to rebase onto")
	case vcsOpStash:
		state.title = "Stash Changes"
		state.hint = "Store local changes and return the working tree to HEAD."
		state.label = "Stash message"
	case vcsOpUnstash:
		state.title = "Unstash Changes"
		state.hint = "Apply a stash and remove it from the stash list."
		m.loadVCSStashes(&state)
	case vcsOpPull:
		state.title = "Update Project"
		state.hint = "Update the current branch in every repository from its configured upstream."
		state.rebase = m.vcsGitUpdateMethod() == "rebase"
	case vcsOpPush:
		state.title = "Push"
		state.hint = "Push the current branch in every repository to its configured remote."
	case vcsOpReset:
		state.title = "Reset Current Branch"
		state.hint = "Move the current branch to the selected revision."
		state.label = "Mode"
		state.choices = []vcsOperationChoice{m.selectedCommitChoiceFor(repo)}
	}
	m.vcsOps = state
	m.invalidate()
}

func (m *Model) closeVCSOperation() {
	if m.vcsOps.busy || m.vcsOps.remotes != nil && m.vcsOps.remotes.busy {
		return
	}
	m.resetVCSOperations()
	m.invalidate()
}

func (m *Model) selectedCommitChoice() vcsOperationChoice {
	return m.selectedCommitChoiceFor(m.vcsRepositoryRoot())
}

func (m *Model) selectedCommitChoiceFor(repo string) vcsOperationChoice {
	repo = vcsRepositoryKey(repo)
	if m.log.visible && m.log.selected >= 0 && m.log.selected < len(m.log.commits) &&
		(repo == "" || vcsRepositoryKey(m.logCommitRepository(m.log.selected)) == repo) {
		c := m.log.commits[m.log.selected]
		return vcsOperationChoice{label: c.ShortID + " " + c.Summary, value: c.ID}
	}
	return vcsOperationChoice{label: "Current revision", value: "HEAD"}
}

func (m *Model) vcsOperationRepoArg(state *VCSOperationPane) any {
	if state != nil && state.repo != "" {
		return vcsRepositoryArgFor(state.repo)
	}
	return m.vcsRepositoryArg()
}

func (m *Model) loadVCSStartChoices(state *VCSOperationPane) {
	state.loading = true
	m.vcsOps = *state
	root := m.Root()
	repoArg := m.vcsOperationRepoArg(state)
	repo := state.repo
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		var branches []openidea.Branch
		branchRaw, branchErr := m.invoke(ctx, "vcs_branches", map[string]any{"repo": repoArg})
		if branchErr == nil {
			branchErr = json.Unmarshal(branchRaw, &branches)
		}
		var tags []string
		tagRaw, tagErr := m.invoke(ctx, "vcs_tags", map[string]any{"repo": repoArg})
		if tagErr == nil {
			tagErr = json.Unmarshal(tagRaw, &tags)
		}
		m.post(func() {
			if m.Root() != root || !m.vcsOps.open || m.vcsOps.kind != vcsOpNewBranch {
				return
			}
			if repo != "" && vcsRepositoryKey(m.vcsOps.repo) != repo {
				return
			}
			m.vcsOps.loading = false
			if branchErr != nil {
				m.vcsOps.err = "Could not read the starting revisions: " + branchErr.Error()
			} else if tagErr != nil {
				m.vcsOps.err = "Could not read the tags: " + tagErr.Error()
			}
			m.vcsOps.choices = m.vcsRevisionChoices(branches, tags)
			if m.vcsOps.preferredStart != "" {
				for i, choice := range m.vcsOps.choices {
					if choice.value == m.vcsOps.preferredStart {
						m.vcsOps.sel = i
						break
					}
				}
			}
			m.invalidate()
		})
	}()
}

func (m *Model) loadVCSRevisionChoices(state *VCSOperationPane, empty string) {
	state.loading = true
	m.vcsOps = *state
	root := m.Root()
	repoArg := m.vcsOperationRepoArg(state)
	repo := state.repo
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		if state.allRepositories {
			choices, loadErr := m.loadVCSAllRevisionChoices(ctx)
			m.post(func() {
				if m.Root() != root || !m.vcsOps.open || m.vcsOps.kind != state.kind {
					return
				}
				if repo != "" && vcsRepositoryKey(m.vcsOps.repo) != repo {
					return
				}
				m.vcsOps.loading = false
				if loadErr != nil {
					m.vcsOps.err = "Could not read the revisions: " + loadErr.Error()
				}
				m.vcsOps.choices = choices
				if m.vcsOps.preferredStart != "" {
					for i, choice := range m.vcsOps.choices {
						if choice.value == m.vcsOps.preferredStart {
							m.vcsOps.sel = i
							break
						}
					}
				}
				if len(m.vcsOps.choices) == 0 && m.vcsOps.err == "" {
					m.vcsOps.err = empty
				}
				m.invalidate()
			})
			return
		}
		var branches []openidea.Branch
		branchRaw, branchErr := m.invoke(ctx, "vcs_branches", map[string]any{"repo": repoArg})
		if branchErr == nil {
			branchErr = json.Unmarshal(branchRaw, &branches)
		}
		var tags []string
		tagRaw, tagErr := m.invoke(ctx, "vcs_tags", map[string]any{"repo": repoArg})
		if tagErr == nil {
			tagErr = json.Unmarshal(tagRaw, &tags)
		}
		m.post(func() {
			if m.Root() != root || !m.vcsOps.open || m.vcsOps.kind != state.kind {
				return
			}
			if repo != "" && vcsRepositoryKey(m.vcsOps.repo) != repo {
				return
			}
			m.vcsOps.loading = false
			if branchErr != nil {
				m.vcsOps.err = "Could not read the revisions: " + branchErr.Error()
			} else if tagErr != nil {
				m.vcsOps.err = "Could not read the tags: " + tagErr.Error()
			}
			m.vcsOps.choices = m.vcsRevisionChoices(branches, tags)
			if m.vcsOps.preferredStart != "" {
				for i, choice := range m.vcsOps.choices {
					if choice.value == m.vcsOps.preferredStart {
						m.vcsOps.sel = i
						break
					}
				}
			}
			if len(m.vcsOps.choices) == 0 && m.vcsOps.err == "" {
				m.vcsOps.err = empty
			}
			m.invalidate()
		})
	}()
}

func (m *Model) loadVCSAllRevisionChoices(ctx context.Context) ([]vcsOperationChoice, error) {
	repositories, err := m.loadVCSRepositories(ctx)
	if err != nil {
		return nil, err
	}
	choices := make([]vcsOperationChoice, 0)
	seen := make(map[string]struct{})
	for _, repository := range repositories {
		root := vcsRepositoryKey(repository.Root)
		if root == "" {
			continue
		}
		label := m.vcsOperationRepositoryLabel(root)
		repoArg := vcsRepositoryArgFor(root)
		branchRaw, err := m.invoke(ctx, "vcs_branches", map[string]any{"repo": repoArg})
		if err != nil {
			return nil, fmt.Errorf("could not read branches in %s: %w", label, err)
		}
		var branches []openidea.Branch
		if err := json.Unmarshal(branchRaw, &branches); err != nil {
			return nil, fmt.Errorf("could not read branches in %s: %w", label, err)
		}
		tagRaw, err := m.invoke(ctx, "vcs_tags", map[string]any{"repo": repoArg})
		if err != nil {
			return nil, fmt.Errorf("could not read tags in %s: %w", label, err)
		}
		var tags []string
		if err := json.Unmarshal(tagRaw, &tags); err != nil {
			return nil, fmt.Errorf("could not read tags in %s: %w", label, err)
		}
		for _, choice := range m.vcsRevisionChoices(branches, tags) {
			if _, ok := seen[choice.value]; ok {
				continue
			}
			seen[choice.value] = struct{}{}
			choices = append(choices, choice)
		}
	}
	return choices, nil
}

func (m *Model) vcsRevisionChoices(branches []openidea.Branch, tags []string) []vcsOperationChoice {
	out := make([]vcsOperationChoice, 0, len(branches)+len(tags))
	for _, b := range branches {
		if b.Remote || b.Current {
			continue
		}
		detail := "Branch"
		if b.Upstream != nil {
			detail += " · tracks " + *b.Upstream
		}
		out = append(out, vcsOperationChoice{label: b.Name, value: b.Name, detail: detail})
	}
	for _, tag := range tags {
		out = append(out, vcsOperationChoice{label: tag, value: tag, detail: "Tag"})
	}
	return out
}

func (m *Model) loadVCSStashes(state *VCSOperationPane) {
	state.loading = true
	m.vcsOps = *state
	root := m.Root()
	repoArg := m.vcsOperationRepoArg(state)
	repo := state.repo
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "vcs_stashes", map[string]any{"repo": repoArg})
		var stashes []openidea.Stash
		if err == nil {
			err = json.Unmarshal(raw, &stashes)
		}
		m.post(func() {
			if m.Root() != root || !m.vcsOps.open || m.vcsOps.kind != vcsOpUnstash {
				return
			}
			if repo != "" && vcsRepositoryKey(m.vcsOps.repo) != repo {
				return
			}
			m.vcsOps.loading = false
			if err != nil {
				m.vcsOps.err = "Could not read the stash list: " + err.Error()
				m.invalidate()
				return
			}
			for _, s := range stashes {
				label := s.Message
				if label == "" {
					label = fmt.Sprintf("Stash #%d", s.Index)
				}
				detail := logTime(m.prefs.dateFormatPrefs(), m.systemRegion, s.TimeMs)
				if detail != "" {
					detail = fmt.Sprintf("#%d · %s", s.Index, detail)
				} else {
					detail = fmt.Sprintf("#%d", s.Index)
				}
				m.vcsOps.choices = append(m.vcsOps.choices, vcsOperationChoice{
					label: label, value: fmt.Sprint(s.Index), detail: detail,
				})
			}
			if len(m.vcsOps.choices) == 0 {
				m.vcsOps.err = "There are no stashed changes"
			}
			m.invalidate()
		})
	}()
}

func (m *Model) stepVCSOperationChoice(delta int) {
	state := &m.vcsOps
	if len(state.choices) == 0 || state.kind == vcsOpReset {
		return
	}
	state.sel = clampSymbolSel(state.sel+delta, len(state.choices))
	m.invalidate()
}

func (m *Model) submitVCSOperation() {
	state := &m.vcsOps
	if !state.open || state.busy || state.loading {
		return
	}
	repoArg := m.vcsOperationRepoArg(state)
	switch state.kind {
	case vcsOpNewBranch:
		name := strings.TrimSpace(state.name)
		if name == "" {
			state.err = "Enter a branch name"
			m.invalidate()
			return
		}
		start := "HEAD"
		if len(state.choices) > 0 {
			start = state.choices[clampSymbolSel(state.sel, len(state.choices))].value
		}
		m.runVCSOperation("Create Branch", "vcs_create_branch", map[string]any{
			"repo": repoArg, "name": name, "start": start, "checkout": true, "overwrite": nil,
		})
	case vcsOpNewTag:
		name := strings.TrimSpace(state.name)
		if name == "" {
			state.err = "Enter a tag name"
			m.invalidate()
			return
		}
		commit := state.choices[clampSymbolSel(state.sel, len(state.choices))].value
		message := strings.TrimSpace(state.message)
		var annotated any
		if message != "" {
			annotated = message
		}
		m.runVCSOperation("Create Tag", "vcs_create_tag", map[string]any{
			"repo": repoArg, "name": name, "commit": commit, "message": annotated, "force": false,
		})
	case vcsOpMerge:
		if len(state.choices) == 0 {
			state.err = "Select a branch or tag to merge"
			m.invalidate()
			return
		}
		branch := state.choices[clampSymbolSel(state.sel, len(state.choices))].value
		m.runVCSMultiRepoHistoryOperation(state, branch)
	case vcsOpRebase:
		if len(state.choices) == 0 {
			state.err = "Select a branch or tag to rebase onto"
			m.invalidate()
			return
		}
		upstream := state.choices[clampSymbolSel(state.sel, len(state.choices))].value
		m.runVCSMultiRepoHistoryOperation(state, upstream)
	case vcsOpStash:
		message := strings.TrimSpace(state.message)
		var stashMessage any
		if message != "" {
			stashMessage = message
		}
		m.runVCSOperation("Stash", "vcs_stash_push", map[string]any{
			"repo": repoArg, "message": stashMessage,
			"includeUntracked": state.includeUntracked, "keepIndex": state.keepIndex,
		})
	case vcsOpUnstash:
		if len(state.choices) == 0 {
			state.err = "Select a stash to apply"
			m.invalidate()
			return
		}
		index := 0
		_, _ = fmt.Sscan(state.choices[clampSymbolSel(state.sel, len(state.choices))].value, &index)
		m.runVCSOperation("Apply Stash", "vcs_stash_apply", map[string]any{
			"repo": repoArg, "index": index, "pop": true, "reinstateIndex": state.keepIndex, "branch": nil,
		})
	case vcsOpPull:
		op := map[string]any{"op": "pull", "rebase": state.rebase}
		if state.allRepositories {
			m.runVCSRemoteAll(vcsOpPull, "Update Project", op)
		} else {
			m.runVCSRemoteAt(repoArg, vcsOpPull, "Pull", op)
		}
	case vcsOpPush:
		if state.pushRevision != "" {
			selectedRepo, _ := repoArg.(string)
			if state.pushRepository != selectedRepo ||
				!m.vcsRepositoryHasRoot(state.pushRepository) {
				state.err = "The selected commit's repository is no longer open"
				m.invalidate()
				return
			}
			root := m.Root()
			revision := state.pushRevision
			force := state.force
			request := m.nextRequest()
			state.request = request
			state.busy = true
			state.err = ""
			m.invalidate()
			go func() {
				ctx, cancel := context.WithTimeout(modelContext(m), 2*time.Minute)
				defer cancel()
				infoRaw, err := m.invoke(ctx, "vcs_branch_info", map[string]any{
					"repo": repoArg,
				})
				var info openidea.BranchInfo
				if err == nil {
					err = json.Unmarshal(infoRaw, &info)
				}
				if err != nil || info.Head == nil || strings.TrimSpace(*info.Head) == "" {
					if err == nil {
						err = errors.New("the selected repository has no current branch")
					}
					m.post(func() {
						if m.Root() != root || !m.vcsOps.open || m.vcsOps.kind != vcsOpPush ||
							m.vcsOps.request != request || m.vcsOps.pushRevision != revision {
							return
						}
						m.vcsOps.busy = false
						m.vcsOps.err = "Could not resolve the branch for this push: " + err.Error()
						m.invalidate()
					})
					return
				}
				branch := strings.TrimSpace(*info.Head)
				op, err := m.revisionPushOptions(
					selectedRepo, branch, revision, info.Upstream, force,
				)
				m.post(func() {
					if m.Root() != root || !m.vcsOps.open || m.vcsOps.kind != vcsOpPush ||
						m.vcsOps.request != request || m.vcsOps.pushRevision != revision ||
						!m.vcsRepositoryHasRoot(selectedRepo) {
						return
					}
					if err != nil {
						m.vcsOps.busy = false
						m.vcsOps.err = err.Error()
						m.invalidate()
						return
					}
					m.vcsOps.busy = false
					m.runVCSRemoteAt(repoArg, vcsOpPush, "Push All up to Here", op)
				})
			}()
			return
		}
		if state.allRepositories {
			m.runVCSMultiRepoPush("Push", state.force, state.setUpstream)
			return
		}
		if state.force && m.vcsProtectedBranch(m.vcsCurrentBranch()) {
			state.err = "Force push is not allowed to protected branches"
			m.invalidate()
			return
		}
		m.runVCSRemoteAt(repoArg, vcsOpPush, "Push", map[string]any{
			"op": "push", "force_with_lease": state.force,
			"set_upstream": state.setUpstream, "tags": nil,
			"force": false, "no_verify": false,
		})
	case vcsOpReset:
		commit := state.choices[clampSymbolSel(state.sel, len(state.choices))].value
		modes := []string{"soft", "mixed", "hard", "keep"}
		mode := modes[clampSymbolSel(state.resetMode, len(modes))]
		if mode == "hard" && !state.force {
			state.err = "Select the confirmation to discard local changes"
			m.invalidate()
			return
		}
		m.runVCSGitOperationAt(repoArg, "Reset", map[string]any{
			"op": "reset", "commit": commit, "mode": mode,
		})
	}
}

func (m *Model) runVCSOperation(title, command string, args map[string]any) {
	state := &m.vcsOps
	state.busy = true
	state.err = ""
	state.output = nil
	root := m.Root()
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 5*time.Minute)
		defer cancel()
		_, err := m.invoke(ctx, command, args)
		m.post(func() {
			if m.Root() != root || !m.vcsOps.open || m.vcsOps.kind != state.kind {
				return
			}
			m.vcsOps.busy = false
			if err != nil {
				m.vcsOps.err = title + " failed: " + err.Error()
				m.invalidate()
				return
			}
			m.status = title + " completed"
			m.refreshAfterVCSWrite()
			m.resetVCSOperations()
			m.invalidate()
		})
	}()
}

func (m *Model) loadVCSRepositories(ctx context.Context) ([]vcsRepository, error) {
	raw, err := m.invoke(ctx, "vcs_roots", nil)
	if err != nil {
		return nil, err
	}
	var repositories []vcsRepository
	if err := json.Unmarshal(raw, &repositories); err != nil {
		return nil, err
	}
	return repositories, nil
}

func (m *Model) vcsOperationRepositoryLabel(root string) string {
	root = vcsRepositoryKey(root)
	if root == "" {
		return "repository"
	}
	return m.vcsRepositoryLabel(vcsRepository{Root: root})
}

func (m *Model) vcsRepositoryContainsTarget(ctx context.Context, root, target string) (bool, error) {
	branchesRaw, err := m.invoke(ctx, "vcs_branches", map[string]any{
		"repo": vcsRepositoryArgFor(root),
	})
	if err != nil {
		return false, err
	}
	var branches []openidea.Branch
	if err := json.Unmarshal(branchesRaw, &branches); err != nil {
		return false, err
	}
	for _, branch := range branches {
		if branch.Name == target {
			return true, nil
		}
	}
	tagsRaw, err := m.invoke(ctx, "vcs_tags", map[string]any{
		"repo": vcsRepositoryArgFor(root),
	})
	if err != nil {
		return false, err
	}
	var tags []string
	if err := json.Unmarshal(tagsRaw, &tags); err != nil {
		return false, err
	}
	for _, tag := range tags {
		if tag == target {
			return true, nil
		}
	}
	return false, nil
}

func (m *Model) vcsRepositoriesContainingTarget(
	ctx context.Context,
	repositories []vcsRepository,
	target string,
) ([]string, error) {
	roots := make([]string, 0, len(repositories))
	for _, repository := range repositories {
		root := vcsRepositoryKey(repository.Root)
		if root == "" {
			continue
		}
		contains, err := m.vcsRepositoryContainsTarget(ctx, root, target)
		if err != nil {
			return nil, fmt.Errorf("could not inspect %s: %w", m.vcsOperationRepositoryLabel(root), err)
		}
		if contains {
			roots = append(roots, root)
		}
	}
	return roots, nil
}

func (m *Model) executeVCSMultiRepoHistory(
	ctx context.Context,
	kind, target string,
	options map[string]any,
) vcsMultiRepoHistoryResult {
	repositories, err := m.loadVCSRepositories(ctx)
	if err != nil {
		return vcsMultiRepoHistoryResult{failures: []vcsRepositoryFailure{{err: err}}}
	}
	roots, err := m.vcsRepositoriesContainingTarget(ctx, repositories, target)
	if err != nil {
		return vcsMultiRepoHistoryResult{failures: []vcsRepositoryFailure{{err: err}}}
	}
	if len(roots) == 0 {
		return vcsMultiRepoHistoryResult{failures: []vcsRepositoryFailure{{
			err: fmt.Errorf("no repository contains %q", target),
		}}}
	}

	var result vcsMultiRepoHistoryResult
	for _, root := range roots {
		result.attempted = append(result.attempted, root)
		before, err := m.vcsRepositoryHeadCommit(ctx, root)
		if err != nil {
			result.failures = append(result.failures, vcsRepositoryFailure{root: root, err: err})
			break
		}
		args := map[string]any{"repo": vcsRepositoryArgFor(root)}
		command := "vcs_merge_with"
		if kind == vcsOpRebase {
			command = "vcs_rebase_with"
			args["upstream"] = target
		} else {
			args["branches"] = []string{target}
		}
		args["options"] = cloneVCSOperation(options)
		if _, err := m.invoke(ctx, command, args); err != nil {
			result.failures = append(result.failures, vcsRepositoryFailure{root: root, err: err})
			break
		}
		after, err := m.vcsRepositoryHeadCommit(ctx, root)
		if err != nil {
			result.failures = append(result.failures, vcsRepositoryFailure{root: root, err: err})
			break
		}
		result.completed = append(result.completed, vcsRepoHeadChange{
			root: root, before: before, after: after,
		})
	}
	return result
}

func (m *Model) vcsRepositoryHeadCommit(ctx context.Context, root string) (string, error) {
	raw, err := m.invoke(ctx, "vcs_branch_info", map[string]any{
		"repo": vcsRepositoryArgFor(root),
	})
	if err != nil {
		return "", err
	}
	var info openidea.BranchInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		return "", err
	}
	if info.HeadCommit == nil || strings.TrimSpace(*info.HeadCommit) == "" {
		return "", errors.New("the repository has no HEAD commit")
	}
	return strings.TrimSpace(*info.HeadCommit), nil
}

func (m *Model) runVCSMultiRepoHistoryOperation(state *VCSOperationPane, target string) {
	kind := state.kind
	title := "Merge"
	commandOptions := map[string]any{
		"noFf": false, "ffOnly": false, "squash": false,
		"noCommit": false, "message": nil,
	}
	if kind == vcsOpRebase {
		title = "Rebase"
		commandOptions = map[string]any{
			"onto": nil, "rebaseMerges": false, "keepEmpty": false,
		}
	}
	state.busy = true
	state.err = ""
	state.output = nil
	state.request = m.nextRequest()
	root := m.Root()
	request := state.request
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Minute)
		defer cancel()
		result := m.executeVCSMultiRepoHistory(ctx, kind, target, commandOptions)
		m.post(func() {
			if m.Root() != root || !m.vcsOps.open || m.vcsOps.kind != kind ||
				m.vcsOps.request != request {
				return
			}
			m.vcsOps.busy = false
			if len(result.failures) == 0 {
				m.status = fmt.Sprintf("%s completed in %d repositories", title, len(result.completed))
				m.refreshAfterVCSWrite()
				m.resetVCSOperations()
				m.invalidate()
				return
			}
			m.refreshAfterVCSWrite()
			if len(result.completed) == 0 {
				m.vcsOps.err = title + " failed: " + m.vcsRepositoryFailuresText(result.failures)
				m.invalidate()
				return
			}
			failure := result.failures[0]
			m.vcsOps.err = ""
			m.vcsOps.multiRepoRollback = &vcsMultiRepoRollbackState{
				kind: kind, target: target,
				failureRoot: failure.root, failureText: failure.err.Error(),
				completed: append([]vcsRepoHeadChange(nil), result.completed...),
			}
			m.invalidate()
		})
	}()
}

func (m *Model) vcsRepositoryFailuresText(failures []vcsRepositoryFailure) string {
	lines := make([]string, 0, len(failures))
	for _, failure := range failures {
		if failure.err == nil {
			continue
		}
		if failure.root == "" {
			lines = append(lines, failure.err.Error())
			continue
		}
		lines = append(lines, m.vcsOperationRepositoryLabel(failure.root)+": "+failure.err.Error())
	}
	return strings.Join(lines, "\n")
}

func (m *Model) runVCSRemote(kind, title string, op map[string]any) {
	m.runVCSRemoteAt(m.vcsRepositoryArg(), kind, title, op)
}

func (m *Model) runVCSRemoteAt(repoArg any, kind, title string, op map[string]any) {
	if m.Root() == "" {
		return
	}
	if !m.vcsOps.open || m.vcsOps.kind != kind {
		m.vcsOps = VCSOperationPane{kind: kind, open: true, title: title}
	}
	state := &m.vcsOps
	state.busy = true
	state.err = ""
	state.output = nil
	state.request = m.nextRequest()
	state.channel = m.nextChannel()
	root := m.Root()
	request, channel := state.request, state.channel
	repo := ""
	if path, ok := repoArg.(string); ok {
		repo = path
	}
	autoUpdateRejected := m.vcsGitBool("vcs.git.autoUpdateOnPushRejected")
	updateMethod := m.vcsGitUpdateMethod()
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Minute)
		defer cancel()
		err := m.invokeVCSRemoteAttempt(ctx, repoArg, channel, op)
		deferred := false
		if err != nil && kind == vcsOpPush {
			err, deferred = m.handleRejectedPush(
				ctx, root, request, channel, repo, title, op, err,
				autoUpdateRejected, updateMethod,
			)
		}
		if deferred {
			return
		}
		m.post(func() {
			if m.Root() != root || !m.vcsOps.open || m.vcsOps.kind != kind || m.vcsOps.request != request {
				return
			}
			m.vcsOps.busy = false
			if err != nil {
				m.vcsOps.err = title + " failed: " + err.Error()
				m.invalidate()
				return
			}
			m.status = title + " completed"
			m.refreshAfterVCSWrite()
			m.invalidate()
		})
	}()
}

func (m *Model) runVCSRemoteAll(kind, title string, op map[string]any) {
	if m.Root() == "" {
		return
	}
	if !m.vcsOps.open || m.vcsOps.kind != kind {
		m.vcsOps = VCSOperationPane{kind: kind, open: true, title: title}
	}
	state := &m.vcsOps
	state.allRepositories = true
	state.busy = true
	state.err = ""
	state.output = nil
	state.request = m.nextRequest()
	state.channel = m.nextChannel()
	root := m.Root()
	request, channel := state.request, state.channel
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Minute)
		defer cancel()
		repositories, err := m.loadVCSRepositories(ctx)
		if err != nil {
			m.post(func() {
				if m.Root() != root || m.vcsOps.request != request || m.vcsOps.kind != kind {
					return
				}
				m.vcsOps.busy = false
				m.vcsOps.err = title + " failed: " + err.Error()
				m.invalidate()
			})
			return
		}
		roots := vcsRepositoryRoots(repositories)
		result := m.executeVCSRemoteRepositories(ctx, request, channel, kind, op, roots)
		m.post(func() {
			if m.Root() != root || m.vcsOps.request != request || m.vcsOps.kind != kind {
				return
			}
			m.vcsOps.busy = false
			for _, skipped := range result.skipped {
				m.vcsOps.output = append(m.vcsOps.output,
					"Skipped "+m.vcsOperationRepositoryLabel(skipped)+": no current branch or upstream")
			}
			if len(result.failures) > 0 {
				prefix := title + " failed"
				if len(result.completed) > 0 {
					prefix = title + " partially failed"
				}
				m.vcsOps.err = prefix + ":\n" + m.vcsRepositoryFailuresText(result.failures)
				m.refreshAfterVCSWrite()
				m.invalidate()
				return
			}
			switch kind {
			case vcsOpFetch:
				m.status = fmt.Sprintf("Fetched %d repositories", len(result.completed))
			default:
				m.status = fmt.Sprintf("Updated %d repositories", len(result.completed))
			}
			m.refreshAfterVCSWrite()
			m.invalidate()
		})
	}()
}

func (m *Model) executeVCSRemoteRepositories(
	ctx context.Context,
	request, channel uint32,
	kind string,
	op map[string]any,
	repositories []string,
) vcsRemoteRepositoriesResult {
	var result vcsRemoteRepositoriesResult
	for _, root := range repositories {
		root = vcsRepositoryKey(root)
		if root == "" {
			continue
		}
		result.attempted = append(result.attempted, root)
		if kind == vcsOpPull {
			raw, err := m.invoke(ctx, "vcs_branch_info", map[string]any{
				"repo": vcsRepositoryArgFor(root),
			})
			if err != nil {
				result.failures = append(result.failures,
					vcsRepositoryFailure{root: root, err: err})
				continue
			}
			var info openidea.BranchInfo
			if err := json.Unmarshal(raw, &info); err != nil {
				result.failures = append(result.failures,
					vcsRepositoryFailure{root: root, err: err})
				continue
			}
			if info.Head == nil || strings.TrimSpace(*info.Head) == "" ||
				info.Upstream == nil || strings.TrimSpace(*info.Upstream) == "" {
				result.skipped = append(result.skipped, root)
				continue
			}
		}
		if err := m.invokeVCSRemoteAttemptForRequest(
			ctx, request, vcsRepositoryArgFor(root), channel, op,
		); err != nil {
			result.failures = append(result.failures,
				vcsRepositoryFailure{root: root, err: err})
			continue
		}
		result.completed = append(result.completed, root)
	}
	return result
}

func (m *Model) vcsPushPlanForRepository(
	ctx context.Context,
	root string,
	forceWithLease, setUpstream bool,
) (vcsPushRepositoryPlan, error) {
	infoRaw, err := m.invoke(ctx, "vcs_branch_info", map[string]any{
		"repo": vcsRepositoryArgFor(root),
	})
	if err != nil {
		return vcsPushRepositoryPlan{}, err
	}
	var info openidea.BranchInfo
	if err := json.Unmarshal(infoRaw, &info); err != nil {
		return vcsPushRepositoryPlan{}, err
	}
	if info.Head == nil || strings.TrimSpace(*info.Head) == "" {
		return vcsPushRepositoryPlan{}, errors.New("HEAD is detached")
	}
	remotesRaw, err := m.invoke(ctx, "vcs_remotes", map[string]any{
		"repo": vcsRepositoryArgFor(root),
	})
	if err != nil {
		return vcsPushRepositoryPlan{}, err
	}
	var remotes []openidea.Remote
	if err := json.Unmarshal(remotesRaw, &remotes); err != nil {
		return vcsPushRepositoryPlan{}, err
	}
	if len(remotes) == 0 {
		return vcsPushRepositoryPlan{}, errors.New("no Git remote is configured")
	}

	upstream := ""
	if info.Upstream != nil {
		upstream = strings.TrimSpace(*info.Upstream)
		upstream = strings.TrimPrefix(upstream, "refs/remotes/")
	}
	remote, target := "", ""
	if upstream != "" {
		for _, candidate := range remotes {
			prefix := candidate.Name + "/"
			if strings.HasPrefix(upstream, prefix) && len(upstream) > len(prefix) {
				remote = candidate.Name
				target = strings.TrimPrefix(upstream, prefix)
				break
			}
		}
	}
	if remote == "" {
		for _, candidate := range remotes {
			if candidate.Name == "origin" {
				remote = candidate.Name
				break
			}
		}
	}
	if remote == "" {
		remote = remotes[0].Name
	}
	if target == "" {
		target = strings.TrimSpace(*info.Head)
	}
	branch := strings.TrimSpace(*info.Head)
	if branch == "" || remote == "" || target == "" {
		return vcsPushRepositoryPlan{}, errors.New("could not resolve the branch, remote and target")
	}
	if forceWithLease && m.vcsProtectedBranch(target) {
		return vcsPushRepositoryPlan{}, fmt.Errorf(
			"force push to protected branch %q is not allowed", target,
		)
	}
	plan := vcsPushRepositoryPlan{
		root: root, branch: branch, remote: remote, target: target,
		setUpstream: setUpstream,
	}
	plan.op = map[string]any{
		"op": "pushBranch", "branch": branch, "revision": nil,
		"force_with_lease": forceWithLease, "target": target, "remote": remote,
		"force": false, "set_upstream": setUpstream, "no_verify": false,
	}
	return plan, nil
}

func (m *Model) executeVCSMultiRepoPush(
	ctx context.Context,
	request, channel uint32,
	forceWithLease, setUpstream bool,
) vcsPushRepositoriesResult {
	repositories, err := m.loadVCSRepositories(ctx)
	if err != nil {
		return vcsPushRepositoriesResult{failures: []vcsRepositoryFailure{{err: err}}}
	}
	var result vcsPushRepositoriesResult
	for _, repository := range repositories {
		root := vcsRepositoryKey(repository.Root)
		if root == "" {
			continue
		}
		result.attempted = append(result.attempted, root)
		plan, err := m.vcsPushPlanForRepository(ctx, root, forceWithLease, setUpstream)
		if err != nil {
			result.failures = append(result.failures,
				vcsRepositoryFailure{root: root, err: err})
			continue
		}
		if err := m.invokeVCSRemoteAttemptForRequest(
			ctx, request, vcsRepositoryArgFor(root), channel, plan.op,
		); err != nil {
			result.failures = append(result.failures,
				vcsRepositoryFailure{root: root, err: err})
			continue
		}
		result.completed = append(result.completed, plan)
	}
	return result
}

func (m *Model) runVCSMultiRepoPush(title string, forceWithLease, setUpstream bool) {
	if m.Root() == "" {
		return
	}
	state := &m.vcsOps
	state.kind = vcsOpPush
	state.allRepositories = true
	state.busy = true
	state.err = ""
	state.output = nil
	state.request = m.nextRequest()
	state.channel = m.nextChannel()
	root := m.Root()
	request, channel := state.request, state.channel
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Minute)
		defer cancel()
		result := m.executeVCSMultiRepoPush(
			ctx, request, channel, forceWithLease, setUpstream,
		)
		m.post(func() {
			if m.Root() != root || !m.vcsOps.open || m.vcsOps.kind != vcsOpPush ||
				m.vcsOps.request != request {
				return
			}
			m.vcsOps.busy = false
			if len(result.failures) > 0 {
				prefix := title + " failed"
				if len(result.completed) > 0 {
					prefix = title + " partially failed"
				}
				m.vcsOps.err = prefix + ":\n" + m.vcsRepositoryFailuresText(result.failures)
				m.refreshAfterVCSWrite()
				m.invalidate()
				return
			}
			m.status = fmt.Sprintf("Pushed %d repositories", len(result.completed))
			m.refreshAfterVCSWrite()
			m.resetVCSOperations()
			m.invalidate()
		})
	}()
}

func (m *Model) invokeVCSRemoteAttempt(
	ctx context.Context,
	repoArg any,
	channel uint32,
	op map[string]any,
) error {
	return m.invokeVCSRemoteAttemptForRequest(ctx, m.nextRequest(), repoArg, channel, op)
}

func (m *Model) invokeVCSRemoteAttemptForRequest(
	ctx context.Context,
	request uint32,
	repoArg any,
	channel uint32,
	op map[string]any,
) error {
	_, err := m.invoke(ctx, "vcs_remote", map[string]any{
		"repo": repoArg, "request": request, "op": op,
		"onOutput": map[string]any{"$channel": channel},
	})
	return err
}

func (m *Model) vcsPushDestination(
	repo, branch string,
	upstream *string,
) (remote, target string) {
	target = strings.TrimSpace(branch)
	upstreamRemote, upstreamTarget := "", ""
	hasParsedUpstream := false
	if upstream != nil && strings.TrimSpace(*upstream) != "" {
		value := strings.TrimSpace(*upstream)
		if scope, name, ok := strings.Cut(value, "/"); ok && scope != "" && name != "" {
			upstreamRemote, upstreamTarget = scope, name
			target = name
			hasParsedUpstream = true
		}
		if value != "" && !hasParsedUpstream {
			target = value
		}
	}
	ctx, cancel := context.WithTimeout(modelContext(m), 30*time.Second)
	defer cancel()
	raw, err := m.invoke(ctx, "vcs_remotes", map[string]any{
		"repo": vcsRepositoryArgFor(repo),
	})
	if err != nil {
		return "", target
	}
	var remotes []openidea.Remote
	if json.Unmarshal(raw, &remotes) != nil {
		return "", target
	}
	if hasParsedUpstream {
		for _, candidate := range remotes {
			if candidate.Name == upstreamRemote {
				return upstreamRemote, upstreamTarget
			}
		}
		return "", target
	}
	for _, candidate := range remotes {
		if candidate.Name == "origin" {
			return candidate.Name, target
		}
	}
	if len(remotes) > 0 {
		return remotes[0].Name, target
	}
	return "", target
}

func (m *Model) revisionPushOptions(
	repo, branch, revision string,
	upstream *string,
	force bool,
) (map[string]any, error) {
	remote, target := m.vcsPushDestination(repo, branch, upstream)
	if remote == "" {
		return nil, errors.New("no Git remote is configured")
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return nil, errors.New("could not resolve the remote branch")
	}
	if force && (m.vcsProtectedBranch(branch) || m.vcsProtectedBranch(target)) {
		return nil, errors.New("Force push is not allowed to protected branches")
	}
	return map[string]any{
		"op": "pushBranch", "branch": branch, "revision": revision,
		"force_with_lease": force, "target": target, "remote": remote,
		"force": false, "set_upstream": false, "no_verify": false,
	}, nil
}

func (m *Model) handleRejectedPush(
	ctx context.Context,
	root string,
	request uint32,
	channel uint32,
	repo, title string,
	op map[string]any,
	pushErr error,
	autoUpdate bool,
	updateMethod string,
) (error, bool) {
	if vcsPushLeaseRejected(pushErr) && vcsBoolOption(op, "force_with_lease") {
		m.post(func() {
			if m.Root() != root || m.vcsOps.request != request || m.vcsOps.kind != vcsOpPush {
				return
			}
			m.vcsOps.busy = false
			m.vcsOps.err = ""
			m.vcsOps.pushRejected = &vcsPushRejectedState{
				repo: repo, op: cloneVCSOperation(op), detail: title,
				errText: pushErr.Error(), lease: true, attempts: 1,
			}
			m.invalidate()
		})
		return nil, true
	}
	if !vcsPushCanUpdate(op, pushErr) {
		return pushErr, false
	}
	branch, merges, ok := m.rejectedPushBranch(ctx, vcsRepositoryArgFor(repo))
	if !ok {
		return pushErr, false
	}
	if autoUpdate {
		err := m.retryVCSRemotePush(ctx, vcsRepositoryArgFor(repo), channel, op, updateMethod, 1)
		return err, false
	}
	m.post(func() {
		if m.Root() != root || m.vcsOps.request != request || m.vcsOps.kind != vcsOpPush {
			return
		}
		m.vcsOps.busy = false
		m.vcsOps.err = ""
		m.vcsOps.pushRejected = &vcsPushRejectedState{
			repo: repo, op: cloneVCSOperation(op), detail: title,
			errText: pushErr.Error(), branch: branch, merges: merges, attempts: 1,
		}
		m.invalidate()
	})
	return nil, true
}

func (m *Model) rejectedPushBranch(ctx context.Context, repoArg any) (string, bool, bool) {
	raw, err := m.invoke(ctx, "vcs_branch_info", map[string]any{"repo": repoArg})
	var info openidea.BranchInfo
	if err != nil {
		return "", false, false
	}
	if err := json.Unmarshal(raw, &info); err != nil ||
		info.Head == nil || strings.TrimSpace(*info.Head) == "" || info.Upstream == nil {
		return "", false, false
	}
	raw, err = m.invoke(ctx, "vcs_log", map[string]any{
		"repo": repoArg,
		"query": map[string]any{
			"branch": "HEAD", "path": nil, "author": nil, "text": nil,
			"range": "@{u}..HEAD", "skip": 0, "limit": 1000,
		},
	})
	var commits []openidea.Commit
	if err != nil {
		return *info.Head, false, true
	}
	if json.Unmarshal(raw, &commits) != nil {
		return *info.Head, false, true
	}
	merges := false
	for _, commit := range commits {
		if len(commit.Parents) > 1 {
			merges = true
			break
		}
	}
	return *info.Head, merges, true
}

func (m *Model) retryVCSRemotePush(
	ctx context.Context,
	repoArg any,
	channel uint32,
	op map[string]any,
	method string,
	attempts int,
) error {
	var last error
	for attempts < 10 {
		attempts++
		if err := m.invokeVCSRemoteAttempt(ctx, repoArg, channel, map[string]any{
			"op": "pull", "rebase": method == "rebase",
		}); err != nil {
			return fmt.Errorf("push was rejected and the update failed: %w", err)
		}
		last = m.invokeVCSRemoteAttempt(ctx, repoArg, channel, op)
		if last == nil {
			return nil
		}
		if !vcsPushCanUpdate(op, last) {
			return last
		}
	}
	return fmt.Errorf("push was rejected after 10 attempts: %w", last)
}

func (m *Model) resolveVCSRejectedPush(method string) {
	state := &m.vcsOps
	rejected := state.pushRejected
	if rejected == nil || rejected.busy {
		return
	}
	if method == "cancel" {
		state.err = "Push rejected"
		state.pushRejected = nil
		m.invalidate()
		return
	}
	if method == "merge" || method == "rebase" {
		if rejected.remember {
			m.saveVCSGitChoice("vcs.git.autoUpdateOnPushRejected", true)
		}
		if !rejected.merges && method != m.vcsGitUpdateMethod() {
			m.saveVCSGitChoice("vcs.git.updateMethod", method)
		}
	}
	rejected.busy = true
	rejected.err = ""
	root := m.Root()
	request := state.request
	repoArg := vcsRepositoryArgFor(rejected.repo)
	op := cloneVCSOperation(rejected.op)
	channel := state.channel
	attempts := rejected.attempts
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Minute)
		defer cancel()
		var err error
		switch method {
		case "force":
			forceOp := cloneVCSOperation(op)
			forceOp["force"] = true
			forceOp["force_with_lease"] = false
			err = m.invokeVCSRemoteAttempt(ctx, repoArg, channel, forceOp)
		default:
			err = m.retryVCSRemotePush(ctx, repoArg, channel, op, method, attempts)
		}
		m.post(func() {
			if m.Root() != root || m.vcsOps.request != request ||
				m.vcsOps.kind != vcsOpPush || m.vcsOps.pushRejected != rejected {
				return
			}
			m.vcsOps.pushRejected = nil
			m.vcsOps.busy = false
			if err != nil {
				m.vcsOps.err = rejected.detail + " failed: " + err.Error()
				m.invalidate()
				return
			}
			m.status = rejected.detail + " completed"
			m.refreshAfterVCSWrite()
			m.invalidate()
		})
	}()
}

func (m *Model) restoreVCSRepositoryHead(
	ctx context.Context,
	completed vcsRepoHeadChange,
) error {
	if completed.before == completed.after {
		return nil
	}
	_, err := m.invoke(ctx, "vcs_restore_head", map[string]any{
		"repo":   vcsRepositoryArgFor(completed.root),
		"before": completed.before,
		"after":  completed.after,
	})
	return err
}

func (m *Model) resolveVCSMultiRepoRollback(action string) {
	state := &m.vcsOps
	rollback := state.multiRepoRollback
	if rollback == nil || rollback.busy {
		return
	}
	title := "Merge"
	if rollback.kind == vcsOpRebase {
		title = "Rebase"
	}
	if action != "rollback" {
		state.multiRepoRollback = nil
		m.status = fmt.Sprintf("Kept %s in %d repositories", title, len(rollback.completed))
		m.resetVCSOperations()
		m.invalidate()
		return
	}
	rollback.busy = true
	rollback.err = ""
	root := m.Root()
	request := state.request
	completed := append([]vcsRepoHeadChange(nil), rollback.completed...)
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 5*time.Minute)
		defer cancel()
		failures := make([]vcsRepositoryFailure, 0)
		remaining := make([]vcsRepoHeadChange, 0)
		for _, repository := range completed {
			if err := m.restoreVCSRepositoryHead(ctx, repository); err != nil {
				failures = append(failures, vcsRepositoryFailure{root: repository.root, err: err})
				remaining = append(remaining, repository)
			}
		}
		m.post(func() {
			if m.Root() != root || m.vcsOps.request != request ||
				m.vcsOps.multiRepoRollback != rollback {
				return
			}
			rollback.busy = false
			m.refreshAfterVCSWrite()
			if len(failures) > 0 {
				rollback.completed = remaining
				rollback.err = "Rollback failed:\n" + m.vcsRepositoryFailuresText(failures)
				m.invalidate()
				return
			}
			m.status = fmt.Sprintf("Rolled back %s in %d repositories", title, len(completed))
			m.resetVCSOperations()
			m.invalidate()
		})
	}()
}

func (m *Model) saveVCSGitChoice(key string, value any) {
	state := m.nativeSettings.pages[settingsPageVCSGit]
	state.loaded = true
	state.scope = "project"
	state.root = m.Root()
	if state.applied == nil {
		state.applied = map[string]any{}
	}
	if state.draft == nil {
		state.draft = map[string]any{}
	}
	state.applied[key] = value
	state.draft[key] = value
	m.nativeSettings.pages[settingsPageVCSGit] = state
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 8*time.Second)
		defer cancel()
		if err := m.invokeSettingsSet(ctx, "project", key, value); err != nil {
			m.post(func() {
				m.status = "Could not save " + key + ": " + err.Error()
				m.invalidate()
			})
		}
	}()
}

func cloneVCSOperation(op map[string]any) map[string]any {
	out := make(map[string]any, len(op))
	for key, value := range op {
		out[key] = value
	}
	return out
}

func vcsBoolOption(op map[string]any, key string) bool {
	value, _ := op[key].(bool)
	return value
}

func vcsPushCanUpdate(op map[string]any, err error) bool {
	return vcsPushWasRejected(err) &&
		!vcsBoolOption(op, "force") &&
		!vcsBoolOption(op, "force_with_lease")
}

func vcsPushLeaseRejected(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "stale info") ||
		strings.Contains(text, "force-with-lease") && strings.Contains(text, "rejected")
}

func (m *Model) runVCSGitOperation(title string, op map[string]any) {
	m.runVCSGitOperationAt(m.vcsRepositoryArg(), title, op)
}

func (m *Model) runVCSGitOperationAt(repoArg any, title string, op map[string]any) {
	state := &m.vcsOps
	state.busy = true
	state.err = ""
	state.output = nil
	state.request = m.nextRequest()
	state.channel = m.nextChannel()
	root := m.Root()
	request, channel := state.request, state.channel
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Minute)
		defer cancel()
		_, err := m.invoke(ctx, "vcs_operation", map[string]any{
			"repo": repoArg, "request": request, "op": op,
			"onOutput": map[string]any{"$channel": channel},
		})
		m.post(func() {
			if m.Root() != root || !m.vcsOps.open || m.vcsOps.request != request {
				return
			}
			m.vcsOps.busy = false
			if err != nil {
				m.vcsOps.err = title + " failed: " + err.Error()
				m.invalidate()
				return
			}
			m.status = title + " completed"
			m.refreshAfterVCSWrite()
			m.resetVCSOperations()
			m.invalidate()
		})
	}()
}

func (m *Model) acceptVCSOperationOutput(id uint32, data json.RawMessage) bool {
	state := &m.vcsOps
	if id == 0 || id != state.channel {
		return false
	}
	var line string
	if json.Unmarshal(data, &line) != nil {
		return true
	}
	line = strings.TrimSuffix(line, "\n")
	if line != "" {
		state.output = append(state.output, line)
		if over := len(state.output) - maxVCSOutputLines; over > 0 {
			state.output = append(state.output[:0], state.output[over:]...)
		}
	}
	m.invalidate()
	return true
}

func (m *Model) refreshAfterVCSWrite() {
	if m.refreshAfterVCSWriteHook != nil {
		m.refreshAfterVCSWriteHook()
		return
	}
	m.RefreshVCS()
	m.RefreshBranch()
	if m.log.visible {
		m.RefreshLog()
	}
}

// ContinueGitOperation continues a stopped merge, rebase, cherry-pick or
// revert through vcs_operation.
func (m *Model) ContinueGitOperation() {
	m.runStandaloneVCSGitOperation("Continue", map[string]any{"op": "continue"})
}

// AbortGitOperation aborts the operation currently in progress.
func (m *Model) AbortGitOperation() {
	m.runStandaloneVCSGitOperation("Abort", map[string]any{"op": "abort"})
}

// SkipGitOperation skips the stopped rebase or cherry-pick commit.
func (m *Model) SkipGitOperation() {
	m.runStandaloneVCSGitOperation("Skip", map[string]any{"op": "skip"})
}

func (m *Model) runStandaloneVCSGitOperation(title string, op map[string]any) {
	if m.Root() == "" || m.vcsOps.busy {
		return
	}
	m.vcsOps = VCSOperationPane{kind: "operation", open: true, title: title}
	m.runVCSGitOperation(title, op)
}

func (m *Model) selectedGitCommit() string {
	if m.log.selected >= 0 && m.log.selected < len(m.log.commits) {
		return m.log.commits[m.log.selected].ID
	}
	return ""
}

func (m *Model) selectedLogCommitRepositoryArg() any {
	if m.log.selected >= 0 && m.log.selected < len(m.log.commits) {
		return vcsRepositoryArgFor(m.logCommitRepository(m.log.selected))
	}
	return m.vcsRepositoryArg()
}

// CherryPickSelectedCommit applies the commit selected in Git Log.
func (m *Model) CherryPickSelectedCommit() {
	commit := m.selectedGitCommit()
	if commit == "" {
		m.notifyInfo("git-cherry-pick", "Select a commit in the Git log", "")
		return
	}
	m.vcsOps = VCSOperationPane{kind: "operation", open: true, title: "Cherry-Pick"}
	repoArg := m.selectedLogCommitRepositoryArg()
	m.runVCSGitOperationAt(repoArg, "Cherry-Pick", map[string]any{
		"op": "cherryPick", "commits": []string{commit}, "recordOrigin": false,
	})
}

// RevertSelectedCommit applies the inverse of the commit selected in Git Log.
func (m *Model) RevertSelectedCommit() {
	commit := m.selectedGitCommit()
	if commit == "" {
		m.notifyInfo("git-revert", "Select a commit in the Git log", "")
		return
	}
	m.vcsOps = VCSOperationPane{kind: "operation", open: true, title: "Revert Commit"}
	repoArg := m.selectedLogCommitRepositoryArg()
	m.runVCSGitOperationAt(repoArg, "Revert", map[string]any{
		"op": "revert", "commits": []string{commit},
	})
}

func (m *Model) vcsOperationDialog(c *ui.Context, p *ui.Theme) {
	state := &m.vcsOps
	if !state.open {
		return
	}
	if state.remotes != nil {
		m.vcsRemotesDialog(c, p, state.remotes)
		return
	}
	open := state.open
	d := nativeModal(c, &open, func() {
		ui.Column(c).Width(560).Gap(10).Children(func() {
			ui.Text(c, state.title).FontSize(13).Bold().TextColor(p.Text).SingleLine()
			if state.hint != "" {
				ui.Text(c, state.hint).FontSize(12).TextColor(p.TextMuted)
			}
			if state.multiRepoRollback != nil {
				m.drawVCSMultiRepoRollback(c, p, state.multiRepoRollback)
				return
			}
			if state.pushRejected != nil {
				m.drawVCSRejectedPush(c, p, state, state.pushRejected)
				return
			}
			if state.loading {
				ui.Text(c, "Reading revisions…").FontSize(12).TextColor(p.TextMuted)
			}
			if state.label != "" && state.kind != vcsOpReset {
				ui.Text(c, state.label).FontSize(12).TextColor(p.TextMuted)
				in := ui.TextInput(c, &state.name).AutoFocus()
				if in.Submitted() {
					m.submitVCSOperation()
				}
			}
			if state.kind == vcsOpReset {
				modes := []string{"Soft", "Mixed", "Hard", "Keep"}
				ui.Text(c, "Mode").FontSize(12).TextColor(p.TextMuted)
				mode := modes[clampSymbolSel(state.resetMode, len(modes))]
				if choice := ui.Select(c, &mode, modes).Width(180); choice.Changed() {
					for i, value := range modes {
						if value == mode {
							state.resetMode = i
							break
						}
					}
					state.err = ""
				}
			}
			if state.kind == vcsOpNewTag || state.kind == vcsOpStash {
				label := "Tag message (optional)"
				if state.kind == vcsOpStash {
					label = "Stash message (optional)"
				}
				ui.Text(c, label).FontSize(12).TextColor(p.TextMuted)
				in := ui.TextInput(c, &state.message).Grow(1)
				if in.Submitted() {
					m.submitVCSOperation()
				}
			}
			if state.kind == vcsOpStash {
				ui.Checkbox(c, &state.includeUntracked, "Include untracked files")
				ui.Checkbox(c, &state.keepIndex, "Keep index")
			}
			if state.kind == vcsOpUnstash {
				ui.Checkbox(c, &state.keepIndex, "Reinstate index")
			}
			if state.kind == vcsOpPull {
				ui.Checkbox(c, &state.rebase, "Rebase incoming changes")
			}
			if state.kind == vcsOpPush {
				protected := m.vcsProtectedBranch(m.vcsCurrentBranch())
				ui.Checkbox(c, &state.force, "Force push with lease").Disabled(protected)
				if protected {
					ui.Text(c, "Force push is not allowed to protected branches.").
						FontSize(11).TextColor(p.TextMuted).SingleLine()
				}
				ui.Checkbox(c, &state.setUpstream, "Set upstream")
			}
			if state.kind == vcsOpReset {
				ui.Checkbox(c, &state.force, "I understand local changes may be discarded")
			}
			if len(state.choices) > 0 && state.kind != vcsOpReset {
				ui.Scroll(c).Height(210).Children(func() {
					ui.Column(c).Children(func() {
						for i, choice := range state.choices {
							i, choice := i, choice
							r := ui.Row(c.Key(fmt.Sprintf("vcs-choice:%s:%d", state.kind, i))).
								Height(26).Padding(0, 8).Gap(8).AlignItems(ui.Center)
							if i == state.sel {
								r.Background(p.SurfaceHover)
							} else if r.Hovered() {
								r.Background(p.SurfaceHover)
							}
							r.Children(func() {
								ui.Text(c, choice.label).FontSize(13).TextColor(p.Text).
									SingleLine().Ellipsis("…")
								ui.Spacer(c)
								if choice.detail != "" {
									ui.Text(c, choice.detail).FontSize(12).TextColor(p.TextMuted).SingleLine()
								}
							})
							if r.Clicked() {
								state.sel = i
							}
						}
					})
				})
			}
			if len(state.output) > 0 {
				ui.Scroll(c).Height(150).Padding(8).Background(p.Background).Children(func() {
					ui.Text(c, strings.Join(state.output, "\n")).FontSize(12).
						Font(oiFontMono).TextColor(p.TextMuted)
				})
			}
			if state.err != "" {
				ui.Text(c, state.err).FontSize(12).TextColor(p.Danger)
			}
			ui.Row(c).Gap(8).Children(func() {
				label := "OK"
				switch state.kind {
				case vcsOpNewBranch:
					label = "Create Branch"
				case vcsOpNewTag:
					label = "Create Tag"
				case vcsOpMerge:
					label = "Merge"
				case vcsOpRebase:
					label = "Rebase"
				case vcsOpStash:
					label = "Stash"
				case vcsOpUnstash:
					label = "Apply Stash"
				case vcsOpPull:
					label = "Pull"
				case vcsOpPush:
					label = "Push"
				case vcsOpReset:
					label = "Reset"
				}
				if state.busy {
					label = "Running…"
				}
				ui.Button(c, label).Disabled(state.loading || state.busy || len(state.err) > 0 && state.kind == vcsOpUnstash).
					OnClick(m.submitVCSOperation)
				if state.busy {
					ui.Button(c, "Stop").OnClick(func() {
						id := state.request
						if id == 0 {
							return
						}
						go func() {
							ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
							defer cancel()
							_, _ = m.invoke(ctx, "cancel_request", map[string]any{"id": id})
						}()
					})
				}
				ui.Button(c, "Close").Disabled(state.busy).OnClick(m.closeVCSOperation)
			})
		})
	})
	if d.Dismissed() || !open {
		m.closeVCSOperation()
	}
}

func (m *Model) drawVCSMultiRepoRollback(
	c *ui.Context,
	p *ui.Theme,
	rollback *vcsMultiRepoRollbackState,
) {
	title := "Merge Partially Failed"
	if rollback.kind == vcsOpRebase {
		title = "Rebase Partially Failed"
	}
	ui.Text(c, title).FontSize(15).Bold().TextColor(p.Text)
	repositories := "repositories"
	if len(rollback.completed) == 1 {
		repositories = "repository"
	}
	ui.Text(c, fmt.Sprintf(
		"Succeeded in %d %s before %q failed. Rollback restores only the successful %s to its previous HEAD.",
		len(rollback.completed), repositories, m.vcsOperationRepositoryLabel(rollback.failureRoot),
		repositories,
	)).FontSize(12).TextColor(p.Text)
	ui.Text(c, "The failed repository is left as it is, including any merge or rebase conflict.").
		FontSize(12).TextColor(p.Warning)
	if rollback.failureText != "" {
		ui.Text(c, rollback.failureText).FontSize(12).TextColor(p.Danger)
	}
	if rollback.err != "" {
		ui.Text(c, rollback.err).FontSize(12).TextColor(p.Danger)
	}
	ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
		ui.Button(c, "Rollback").Disabled(rollback.busy).
			OnClick(func() { m.resolveVCSMultiRepoRollback("rollback") })
		ui.Button(c, "Keep").Disabled(rollback.busy).
			OnClick(func() { m.resolveVCSMultiRepoRollback("keep") })
	})
	if rollback.busy {
		ui.Text(c, "Rolling back completed repositories…").FontSize(12).TextColor(p.TextMuted)
	}
}

func (m *Model) drawVCSRejectedPush(
	c *ui.Context,
	p *ui.Theme,
	state *VCSOperationPane,
	rejected *vcsPushRejectedState,
) {
	ui.Text(c, "Push Rejected").FontSize(15).Bold().TextColor(p.Text)
	if rejected.lease {
		ui.Text(c, "The force-with-lease Push was rejected because the remote moved. "+
			"Fetch first if those changes must be preserved, or force Push to override them.").
			FontSize(12).TextColor(p.TextMuted)
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			ui.Button(c, "Force push anyway").Disabled(rejected.busy).
				OnClick(func() { m.resolveVCSRejectedPush("force") })
			ui.Button(c, "Cancel").Disabled(rejected.busy).
				OnClick(func() { m.resolveVCSRejectedPush("cancel") })
		})
		if rejected.busy {
			ui.Text(c, "Updating and retrying Push…").FontSize(12).TextColor(p.TextMuted)
		}
		if rejected.err != "" {
			ui.Text(c, rejected.err).FontSize(12).TextColor(p.Danger)
		}
		return
	}
	branch := rejected.branch
	if branch == "" {
		branch = state.title
	}
	ui.Text(c, fmt.Sprintf("Push of the current branch %q was rejected. "+
		"Remote changes need to be updated before pushing.", branch)).
		FontSize(12).TextColor(p.Text)
	if rejected.merges {
		ui.Text(c, "The branch contains unpushed merge commits. Merge is recommended; "+
			"rebasing them can rewrite commits that are already referenced.").
			FontSize(12).TextColor(p.Warning)
	}
	ui.Checkbox(c, &rejected.remember,
		"Remember the update method and update silently in the future. "+
			"Change this later in Settings | Version Control.")
	if rejected.busy {
		ui.Text(c, "Updating and retrying Push…").FontSize(12).TextColor(p.TextMuted)
	}
	if rejected.err != "" {
		ui.Text(c, rejected.err).FontSize(12).TextColor(p.Danger)
	}
	ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
		ui.Button(c, "Merge").Disabled(rejected.busy).
			OnClick(func() { m.resolveVCSRejectedPush("merge") })
		rebase := "Rebase"
		if rejected.merges {
			rebase = "Rebase Anyway"
		}
		ui.Button(c, rebase).Disabled(rejected.busy).
			OnClick(func() { m.resolveVCSRejectedPush("rebase") })
		ui.Button(c, "Cancel").Disabled(rejected.busy).
			OnClick(func() { m.resolveVCSRejectedPush("cancel") })
	})
}

// SmokeVCSOperations proves the Git operations added to the native UI against
// the real backend and a local bare remote. It leaves the repository clean.
func (m *Model) SmokeVCSOperations(ctx context.Context) (map[string]any, error) {
	root := m.Root()
	if root == "" {
		return nil, errors.New("No project is open")
	}
	out := map[string]any{"root": root}
	if _, err := m.invoke(ctx, "project_trust_set", map[string]any{
		"path": root, "trusted": true, "folder": true,
	}); err != nil {
		return out, err
	}
	info, err := m.branchInfo(ctx)
	if err != nil {
		return out, err
	}
	if info.Head == nil || info.HeadCommit == nil {
		return out, errors.New("VCS smoke needs a branch with one commit")
	}
	original, branch := *info.HeadCommit, *info.Head
	out["branch"] = branch
	out["head"] = original

	tag := "goidea-smoke-" + fmt.Sprint(time.Now().UnixNano())
	if _, err := m.invoke(ctx, "vcs_create_tag", map[string]any{
		"repo": nil, "name": tag, "commit": original, "message": "GoIDEA smoke tag", "force": false,
	}); err != nil {
		return out, err
	}
	tagRaw, tagErr := m.invoke(ctx, "vcs_tags", map[string]any{"repo": nil})
	var tags []string
	if tagErr == nil {
		tagErr = json.Unmarshal(tagRaw, &tags)
	}
	out["tagCreated"] = tagErr == nil && containsString(tags, tag)
	if tagErr == nil {
		_, _ = m.invoke(ctx, "vcs_delete_tag", map[string]any{"repo": nil, "name": tag})
	}

	file := filepath.Join(root, "goidea-vcs-smoke.txt")
	if err := osWriteFile(file, []byte("stash smoke\n")); err != nil {
		return out, err
	}
	if _, err := m.invoke(ctx, "vcs_stash_push", map[string]any{
		"repo": nil, "message": "GoIDEA smoke stash", "includeUntracked": true, "keepIndex": false,
	}); err != nil {
		return out, err
	}
	stashRaw, stashErr := m.invoke(ctx, "vcs_stashes", map[string]any{"repo": nil})
	var stashes []openidea.Stash
	if stashErr == nil {
		stashErr = json.Unmarshal(stashRaw, &stashes)
	}
	out["stashCreated"] = stashErr == nil && len(stashes) > 0
	if stashErr == nil && len(stashes) > 0 {
		index := stashes[0].Index
		if _, err := m.invoke(ctx, "vcs_stash_apply", map[string]any{
			"repo": nil, "index": index, "pop": true, "reinstateIndex": false, "branch": nil,
		}); err != nil {
			return out, err
		}
		restored, err := os.ReadFile(file)
		if err != nil {
			return out, fmt.Errorf("read the unstashed working-tree change: %w", err)
		}
		if string(restored) != "stash smoke\n" {
			return out, fmt.Errorf("unstash restored %q, want the original change", restored)
		}
		out["stashRestored"] = true
		_, _ = m.invoke(ctx, "vcs_discard", map[string]any{"paths": []string{file}})
	}

	remoteDir := filepath.Join(filepath.Dir(root), "goidea-vcs-smoke-remote.git")
	remoteName := "goidea-smoke"
	if err := setupVCSRemote(ctx, root, remoteDir, remoteName, branch); err != nil {
		return out, err
	}
	remoteListRaw, remoteListErr := m.invoke(ctx, "vcs_remotes", map[string]any{"repo": nil})
	var remotes []openidea.Remote
	if remoteListErr == nil {
		remoteListErr = json.Unmarshal(remoteListRaw, &remotes)
	}
	if remoteListErr != nil {
		return out, remoteListErr
	}
	out["remoteListed"] = false
	for _, remote := range remotes {
		if remote.Name == remoteName && remote.URL == remoteDir {
			out["remoteListed"] = true
			break
		}
	}
	if out["remoteListed"] != true {
		return out, fmt.Errorf("vcs_remotes did not list %s", remoteName)
	}
	secondaryRemote := remoteName + "-renamed"
	if _, err := m.invoke(ctx, "vcs_edit_remote", map[string]any{
		"repo": nil,
		"edit": map[string]any{"op": "add", "name": secondaryRemote, "url": remoteDir + "-secondary"},
	}); err != nil {
		return out, err
	}
	finalRemote := remoteName + "-updated"
	if _, err := m.invoke(ctx, "vcs_edit_remote", map[string]any{
		"repo": nil,
		"edit": map[string]any{
			"op": "edit", "name": secondaryRemote, "new_name": finalRemote,
			"url": remoteDir + "-updated",
		},
	}); err != nil {
		return out, err
	}
	remoteListRaw, remoteListErr = m.invoke(ctx, "vcs_remotes", map[string]any{"repo": nil})
	if remoteListErr == nil {
		remoteListErr = json.Unmarshal(remoteListRaw, &remotes)
	}
	if remoteListErr != nil {
		return out, remoteListErr
	}
	out["remoteEdited"] = false
	for _, remote := range remotes {
		if remote.Name == finalRemote && remote.URL == remoteDir+"-updated" {
			out["remoteEdited"] = true
			break
		}
	}
	if out["remoteEdited"] != true {
		return out, fmt.Errorf("vcs_edit_remote did not rename and update %s", secondaryRemote)
	}
	if _, err := m.invoke(ctx, "vcs_edit_remote", map[string]any{
		"repo": nil,
		"edit": map[string]any{"op": "remove", "name": finalRemote},
	}); err != nil {
		return out, err
	}

	suffix := fmt.Sprint(time.Now().UnixNano())
	worktreePath := filepath.Join(filepath.Dir(root), "goidea-vcs-smoke-worktree-"+suffix)
	worktreeBranch := "goidea-smoke-worktree-" + suffix
	if _, err := m.invoke(ctx, "vcs_add_worktree", map[string]any{
		"repo": nil, "path": worktreePath, "commitish": branch, "newBranch": worktreeBranch,
	}); err != nil {
		return out, err
	}
	worktreeRaw, worktreeErr := m.invoke(ctx, "vcs_worktrees", map[string]any{"repo": nil})
	var worktrees []openidea.Worktree
	if worktreeErr == nil {
		worktreeErr = json.Unmarshal(worktreeRaw, &worktrees)
	}
	if worktreeErr != nil {
		return out, worktreeErr
	}
	out["worktreeCreated"] = false
	for _, worktree := range worktrees {
		if worktreePathEqual(worktree.Path, worktreePath) {
			out["worktreeCreated"] = true
			break
		}
	}
	if out["worktreeCreated"] != true {
		return out, errors.New("vcs_worktrees did not list the newly created worktree")
	}
	movedWorktreePath := worktreePath + "-moved"
	if _, err := m.invoke(ctx, "vcs_move_worktree", map[string]any{
		"repo": nil, "path": worktreePath, "to": movedWorktreePath,
	}); err != nil {
		return out, err
	}
	worktreeRaw, worktreeErr = m.invoke(ctx, "vcs_worktrees", map[string]any{"repo": nil})
	if worktreeErr == nil {
		worktreeErr = json.Unmarshal(worktreeRaw, &worktrees)
	}
	if worktreeErr != nil {
		return out, worktreeErr
	}
	out["worktreeMoved"] = false
	for _, worktree := range worktrees {
		if worktreePathEqual(worktree.Path, movedWorktreePath) {
			out["worktreeMoved"] = true
			break
		}
	}
	if out["worktreeMoved"] != true {
		return out, errors.New("vcs_move_worktree did not update the listed path")
	}
	if _, err := m.invoke(ctx, "vcs_remove_worktree", map[string]any{
		"repo": nil, "path": movedWorktreePath, "force": false,
	}); err != nil {
		return out, err
	}
	worktreeRaw, worktreeErr = m.invoke(ctx, "vcs_worktrees", map[string]any{"repo": nil})
	if worktreeErr == nil {
		worktreeErr = json.Unmarshal(worktreeRaw, &worktrees)
	}
	if worktreeErr != nil {
		return out, worktreeErr
	}
	out["worktreeRemoved"] = true
	for _, worktree := range worktrees {
		if worktreePathEqual(worktree.Path, movedWorktreePath) {
			out["worktreeRemoved"] = false
			break
		}
	}
	if out["worktreeRemoved"] != true {
		return out, errors.New("vcs_remove_worktree left the worktree in the list")
	}
	remoteCh := m.nextChannel()
	remoteRequest := m.nextRequest()
	if _, err := m.invoke(ctx, "vcs_remote", map[string]any{
		"repo": nil, "request": remoteRequest, "op": map[string]any{"op": "fetch"},
		"onOutput": map[string]any{"$channel": remoteCh},
	}); err != nil {
		return out, err
	}
	pushCh := m.nextChannel()
	pushRequest := m.nextRequest()
	if _, err := m.invoke(ctx, "vcs_remote", map[string]any{
		"repo": nil, "request": pushRequest,
		"op": map[string]any{
			"op": "pushBranch", "branch": branch, "revision": nil,
			"force_with_lease": false, "target": branch, "remote": remoteName,
			"force": false, "set_upstream": true, "no_verify": true,
		},
		"onOutput": map[string]any{"$channel": pushCh},
	}); err != nil {
		return out, err
	}
	out["fetched"] = true
	out["pushed"] = true

	if err := commitVCSSmokeRemote(ctx, remoteDir, branch); err != nil {
		return out, err
	}
	pullCh := m.nextChannel()
	pullRequest := m.nextRequest()
	if _, err := m.invoke(ctx, "vcs_remote", map[string]any{
		"repo": nil, "request": pullRequest,
		"op":       map[string]any{"op": "pull", "rebase": false},
		"onOutput": map[string]any{"$channel": pullCh},
	}); err != nil {
		return out, err
	}
	if _, err := os.Stat(filepath.Join(root, "goidea-vcs-pull-smoke.txt")); err != nil {
		return out, fmt.Errorf("pull did not update the worktree: %w", err)
	}
	out["pulled"] = true

	info, err = m.branchInfo(ctx)
	if err != nil {
		return out, err
	}
	if info.HeadCommit == nil {
		return out, errors.New("VCS smoke lost HEAD after pull")
	}
	rebaseStart := *info.HeadCommit
	feature := "goidea-smoke-" + fmt.Sprint(time.Now().UnixNano())
	if _, err := m.invoke(ctx, "vcs_create_branch", map[string]any{
		"repo": nil, "name": feature, "start": rebaseStart, "checkout": true, "overwrite": nil,
	}); err != nil {
		return out, err
	}
	out["branchCreated"] = true
	if err := osWriteFile(file, []byte("merge smoke\n")); err != nil {
		return out, err
	}
	if _, err := m.invoke(ctx, "vcs_stage", map[string]any{"paths": []string{file}}); err != nil {
		return out, err
	}
	if _, err := m.invoke(ctx, "vcs_commit", map[string]any{
		"repo": nil, "message": "VCS operation smoke",
		"options": map[string]any{
			"amend": false, "signOff": false, "author": nil,
			"runHooks": false, "stage": []string{file},
		},
	}); err != nil {
		return out, err
	}
	if _, err := m.invoke(ctx, "vcs_checkout", map[string]any{
		"repo": nil, "branch": branch, "create": false, "options": nil,
	}); err != nil {
		return out, err
	}
	mainOnly := filepath.Join(root, "goidea-vcs-main-smoke.txt")
	if err := osWriteFile(mainOnly, []byte("main moved after the feature\n")); err != nil {
		return out, err
	}
	if _, err := m.invoke(ctx, "vcs_stage", map[string]any{"paths": []string{mainOnly}}); err != nil {
		return out, err
	}
	if _, err := m.invoke(ctx, "vcs_commit", map[string]any{
		"repo": nil, "message": "VCS rebase smoke on main",
		"options": map[string]any{
			"amend": false, "signOff": false, "author": nil,
			"runHooks": false, "stage": []string{mainOnly},
		},
	}); err != nil {
		return out, err
	}
	if _, err := m.invoke(ctx, "vcs_checkout", map[string]any{
		"repo": nil, "branch": feature, "create": false, "options": nil,
	}); err != nil {
		return out, err
	}
	if _, err := m.invoke(ctx, "vcs_rebase_with", map[string]any{
		"repo": nil, "upstream": branch,
		"options": map[string]any{"onto": nil, "rebaseMerges": false, "keepEmpty": false},
	}); err != nil {
		return out, err
	}
	out["rebased"] = true
	if _, err := m.invoke(ctx, "vcs_checkout", map[string]any{
		"repo": nil, "branch": branch, "create": false, "options": nil,
	}); err != nil {
		return out, err
	}
	if _, err := m.invoke(ctx, "vcs_merge_with", map[string]any{
		"repo": nil, "branches": []string{feature},
		"options": map[string]any{
			"noFf": false, "ffOnly": false, "squash": false,
			"noCommit": false, "message": nil,
		},
	}); err != nil {
		return out, err
	}
	out["merged"] = true
	opCh := m.nextChannel()
	opRequest := m.nextRequest()
	if _, err := m.invoke(ctx, "vcs_operation", map[string]any{
		"repo": nil, "request": opRequest,
		"op":       map[string]any{"op": "reset", "commit": original, "mode": "hard"},
		"onOutput": map[string]any{"$channel": opCh},
	}); err != nil {
		return out, err
	}
	info, err = m.branchInfo(ctx)
	if err != nil {
		return out, err
	}
	if info.HeadCommit == nil || *info.HeadCommit != original {
		return out, fmt.Errorf("hard reset left HEAD at %v, want %s", info.HeadCommit, original)
	}
	out["reset"] = true
	_, _ = m.invoke(ctx, "vcs_delete_branch", map[string]any{"repo": nil, "branch": feature})
	_, _ = m.invoke(ctx, "vcs_remote", map[string]any{
		"repo": nil, "request": m.nextRequest(),
		"op":       map[string]any{"op": "deleteBranch", "branch": remoteName + "/" + branch},
		"onOutput": map[string]any{"$channel": m.nextChannel()},
	})
	_ = exec.CommandContext(ctx, "git", "-C", root, "remote", "remove", remoteName).Run()
	if err := os.RemoveAll(remoteDir); err != nil {
		return out, err
	}
	if err := os.RemoveAll(remoteDir + "-work"); err != nil {
		return out, err
	}
	return out, nil
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func osWriteFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
}

func setupVCSRemote(ctx context.Context, root, remoteDir, remoteName, branch string) error {
	if err := exec.CommandContext(ctx, "git", "init", "--bare", remoteDir).Run(); err != nil {
		return fmt.Errorf("prepare the VCS smoke remote: %w", err)
	}
	command := exec.CommandContext(ctx, "git", "-C", root, "remote", "add", remoteName, remoteDir)
	if err := command.Run(); err != nil {
		command = exec.CommandContext(ctx, "git", "-C", root, "remote", "set-url", remoteName, remoteDir)
		if err := command.Run(); err != nil {
			return fmt.Errorf("configure the VCS smoke remote: %w", err)
		}
	}
	_ = branch
	return nil
}

// commitVCSSmokeRemote advances the local bare remote through a second clone,
// so vcs_remote Pull has a real upstream commit to fetch and merge.
func commitVCSSmokeRemote(ctx context.Context, remoteDir, branch string) error {
	work := remoteDir + "-work"
	if err := os.RemoveAll(work); err != nil {
		return err
	}
	run := func(dir string, args ...string) error {
		command := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
		}
		return nil
	}
	if err := run(filepath.Dir(remoteDir), "clone", "--branch", branch, remoteDir, work); err != nil {
		return fmt.Errorf("prepare the VCS pull fixture: %w", err)
	}
	if err := run(work, "config", "user.name", "GoIDEA smoke"); err != nil {
		return err
	}
	if err := run(work, "config", "user.email", "goidea-smoke@invalid"); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(work, "goidea-vcs-pull-smoke.txt"), []byte("remote update\n"), 0o644); err != nil {
		return err
	}
	if err := run(work, "add", "goidea-vcs-pull-smoke.txt"); err != nil {
		return err
	}
	if err := run(work, "commit", "-m", "VCS pull smoke"); err != nil {
		return err
	}
	if err := run(work, "push", "origin", branch); err != nil {
		return err
	}
	return nil
}
