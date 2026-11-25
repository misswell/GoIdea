package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

// BranchPane is the title bar's branch widget and its Branches popup: the
// current head `vcs_branch_info` reports and the branches `vcs_branches` lists.
type BranchPane struct {
	loaded     bool
	head       *string
	headCommit *string
	upstream   *string
	ahead      int
	behind     int
	operation  *string

	open     bool
	query    string
	branches []openidea.Branch
	sel      int
	loading  bool
	listing  bool
	// branchesLoaded distinguishes an empty repository from a list that has not
	// answered yet. The cancels stop work for a repository after its tab was
	// left; generations still guard responses from transports that cannot cancel.
	branchesLoaded bool
	infoCancel     context.CancelFunc
	listCancel     context.CancelFunc
	busy           bool
	err            string
	// prompt is rename while the Branches popup asks for a new branch name.
	// A remote delete uses it as its required destructive confirmation.
	prompt       string
	promptBranch openidea.Branch
	promptName   string
	promptUnset  bool
	promptErr    string
	// refreshPending coalesces a refresh requested while branch_info is in
	// flight instead of dropping it.
	refreshPending bool
}

func (m *Model) resetBranch() {
	cancelVCSBranchPane(&m.branch)
	m.branch = BranchPane{}
}

func (m *Model) vcsBranchPaneForRepository(root string) BranchPane {
	root = vcsRepositoryKey(root)
	if root == "" || root == vcsRepositoryKey(m.vcs.current) {
		return m.branch
	}
	return m.vcs.branches[root]
}

func (m *Model) updateVCSBranchPane(root string, update func(*BranchPane)) {
	root = vcsRepositoryKey(root)
	if root == "" || root == vcsRepositoryKey(m.vcs.current) {
		update(&m.branch)
		m.invalidate()
		return
	}
	if m.vcs.branches == nil {
		m.vcs.branches = map[string]BranchPane{}
	}
	pane := m.vcs.branches[root]
	update(&pane)
	m.vcs.branches[root] = pane
}

func cancelVCSBranchPane(branch *BranchPane) {
	if branch == nil {
		return
	}
	if branch.infoCancel != nil {
		branch.infoCancel()
		branch.infoCancel = nil
	}
	if branch.listCancel != nil {
		branch.listCancel()
		branch.listCancel = nil
	}
	branch.loading = false
	branch.listing = false
}

func (m *Model) nextVCSBranchInfoGeneration(root string) uint64 {
	root = vcsRepositoryKey(root)
	if m.vcs.branchInfoGenerations == nil {
		m.vcs.branchInfoGenerations = map[string]uint64{}
	}
	m.vcs.branchInfoGenerations[root]++
	return m.vcs.branchInfoGenerations[root]
}

func (m *Model) nextVCSBranchListGeneration(root string) uint64 {
	root = vcsRepositoryKey(root)
	if m.vcs.branchListGenerations == nil {
		m.vcs.branchListGenerations = map[string]uint64{}
	}
	m.vcs.branchListGenerations[root]++
	return m.vcs.branchListGenerations[root]
}

// RefreshBranch reloads the current branch in the background.
func (m *Model) RefreshBranch() {
	root := m.Root()
	repoRoot := m.vcsRepositoryRoot()
	branch := m.vcsBranchPaneForRepository(repoRoot)
	if root == "" {
		return
	}
	if branch.loading {
		m.updateVCSBranchPane(repoRoot, func(branch *BranchPane) {
			branch.refreshPending = true
		})
		return
	}
	repoArg := m.vcsRepositoryArg()
	generation := m.nextVCSBranchInfoGeneration(repoRoot)
	ctx, cancel := context.WithTimeout(modelContext(m), 20*time.Second)
	m.updateVCSBranchPane(repoRoot, func(branch *BranchPane) {
		if branch.infoCancel != nil {
			branch.infoCancel()
		}
		branch.infoCancel = cancel
		branch.loading, branch.refreshPending = true, false
	})
	go func() {
		defer cancel()
		raw, err := m.invoke(ctx, "vcs_branch_info", map[string]any{"repo": repoArg})
		var info openidea.BranchInfo
		if err == nil {
			err = json.Unmarshal(raw, &info)
		}
		m.post(func() {
			if m.Root() != root ||
				m.vcs.branchInfoGenerations[repoRoot] != generation {
				return
			}
			m.updateVCSBranchPane(repoRoot, func(branch *BranchPane) {
				branch.loading = false
				branch.infoCancel = nil
				branch.loaded = true
				if err != nil {
					// A project without Git simply has no widget.
					if branchErrorText(err) == "" {
						branch.head = nil
						branch.headCommit = nil
						branch.upstream = nil
						branch.ahead, branch.behind = 0, 0
						branch.operation = nil
					} else {
						branch.err = branchErrorText(err)
					}
					return
				}
				branch.err = ""
				branch.head = info.Head
				branch.headCommit = info.HeadCommit
				branch.upstream = info.Upstream
				branch.ahead, branch.behind = info.Ahead, info.Behind
				branch.operation = info.Operation
			})
			m.invalidate()
			active := m.vcsBranchPaneForRepository(repoRoot)
			if active.refreshPending {
				m.updateVCSBranchPane(repoRoot, func(branch *BranchPane) {
					branch.refreshPending = false
				})
				if repoRoot != "" &&
					filepath.Clean(m.vcsRepositoryRoot()) == filepath.Clean(repoRoot) {
					m.RefreshBranch()
				}
			}
		})
	}()
}

// branchLabel is the title bar text: the branch or, on a detached head, its
// commit, an operation in progress, and how far it is from its upstream.
func (m *Model) branchLabel() string {
	b := &m.branch
	head := ""
	if b.head != nil {
		head = *b.head
	} else if b.headCommit != nil && len(*b.headCommit) >= 8 {
		head = (*b.headCommit)[:8]
	}
	label := branchOperationTitle(b.operation, head)
	if label == "" {
		label = head
	}
	if b.behind > 0 {
		label += fmt.Sprintf(" ↓%d", b.behind)
	}
	if b.ahead > 0 {
		label += fmt.Sprintf(" ↑%d", b.ahead)
	}
	return label
}

// branchOperationTitle words a stopped merge, rebase or cherry-pick the way
// IDEA's widget does: the operation with the branch it is on.
func branchOperationTitle(op *string, head string) string {
	if op == nil {
		return ""
	}
	verb := ""
	switch *op {
	case "rebase":
		verb = "Rebasing"
	case "merge":
		verb = "Merging"
	case "cherry-pick":
		verb = "Cherry-picking"
	case "revert":
		verb = "Reverting"
	case "am":
		verb = "Applying the patch"
	default:
		return ""
	}
	if head == "" {
		return verb
	}
	return verb + " " + head
}

// ToggleBranches opens the Branches popup, or closes it when it is open.
func (m *Model) ToggleBranches() {
	if m.Root() == "" {
		return
	}
	if m.branch.open {
		m.closeBranches()
		return
	}
	m.branch.open = true
	m.branch.query = ""
	m.branch.err = ""
	m.branch.sel = 0
	m.branch.listing = true
	m.invalidate()
	m.RefreshBranch()
	m.loadBranches()
}

// ShowBranches opens the Branches popup, leaving it open when it already is;
// the tab menu's Git › Branches… uses it.
func (m *Model) ShowBranches() {
	if m.branch.open {
		return
	}
	m.ToggleBranches()
}

// closeBranches drops the popup.
func (m *Model) closeBranches() {
	if !m.branch.open {
		return
	}
	m.branch.open = false
	m.branch.query = ""
	m.branch.prompt = ""
	m.branch.promptErr = ""
	m.invalidate()
}

// loadBranches lists the repository's branches for the popup.
func (m *Model) loadBranches() {
	root := m.Root()
	if root == "" || !m.branch.listing {
		return
	}
	repoRoot := m.vcsRepositoryRoot()
	repoArg := m.vcsRepositoryArg()
	generation := m.nextVCSBranchListGeneration(repoRoot)
	ctx, cancel := context.WithTimeout(modelContext(m), 30*time.Second)
	m.updateVCSBranchPane(repoRoot, func(branch *BranchPane) {
		if branch.listCancel != nil {
			branch.listCancel()
		}
		branch.listCancel = cancel
		branch.listing = true
		branch.branchesLoaded = false
	})
	go func() {
		defer cancel()
		raw, err := m.invoke(ctx, "vcs_branches", map[string]any{"repo": repoArg})
		var branches []openidea.Branch
		if err == nil {
			err = json.Unmarshal(raw, &branches)
		}
		m.post(func() {
			if m.Root() != root ||
				m.vcs.branchListGenerations[repoRoot] != generation {
				return
			}
			m.updateVCSBranchPane(repoRoot, func(branch *BranchPane) {
				branch.listing = false
				branch.listCancel = nil
				if err != nil {
					if branchErrorText(err) != "" {
						branch.err = branchErrorText(err)
					}
					branch.branches = nil
					return
				}
				// Local branches first, as IDEA groups them.
				sort.SliceStable(branches, func(i, j int) bool {
					if branches[i].Remote != branches[j].Remote {
						return !branches[i].Remote
					}
					return branches[i].Name < branches[j].Name
				})
				branch.branches = branches
				branch.branchesLoaded = true
				branch.sel = m.currentBranchRowFor(branch)
			})
			m.invalidate()
		})
	}()
}

func (m *Model) currentBranchRow() int {
	return m.currentBranchRowFor(&m.branch)
}

func (m *Model) currentBranchRowFor(branch *BranchPane) int {
	query := strings.ToLower(strings.TrimSpace(branch.query))
	rows := branch.branches
	if query != "" {
		rows = make([]openidea.Branch, 0, len(branch.branches))
		for _, item := range branch.branches {
			if strings.Contains(strings.ToLower(item.Name), query) {
				rows = append(rows, item)
			}
		}
	}
	for i, b := range rows {
		if b.Current {
			return i
		}
	}
	return 0
}

// branchRows is the list the popup shows: the branches matching the typed
// filter, in their loaded order.
func (m *Model) branchRows() []openidea.Branch {
	query := strings.ToLower(strings.TrimSpace(m.branch.query))
	if query == "" {
		return m.branch.branches
	}
	out := make([]openidea.Branch, 0, len(m.branch.branches))
	for _, b := range m.branch.branches {
		if strings.Contains(strings.ToLower(b.Name), query) {
			out = append(out, b)
		}
	}
	return out
}

// stepBranch moves the chosen row by delta, wrapping as IDEA's popup does.
func (m *Model) stepBranch(delta int) {
	rows := m.branchRows()
	if len(rows) == 0 {
		return
	}
	sel := m.branch.sel + delta
	if sel < 0 {
		sel = len(rows) - 1
	} else if sel >= len(rows) {
		sel = 0
	}
	m.branch.sel = sel
	m.invalidate()
}

// CheckoutSelectedBranch checks out the row Enter picks.
func (m *Model) CheckoutSelectedBranch() {
	rows := m.branchRows()
	if len(rows) == 0 {
		m.invalidate()
		return
	}
	m.checkoutBranch(rows[clampSymbolSel(m.branch.sel, len(rows))].Name)
}

// branchRowMenu is the Branches popup's native context menu. Local deletion
// follows OpenIDEA and proceeds without a confirmation so the deleted payload
// can immediately back a Restore action. Remote deletion, which changes the
// server, requires its confirmation.
func (m *Model) branchRowMenu(menu *ui.Menu, branch openidea.Branch) {
	if !branch.Current {
		if menu.Item("Checkout").Chosen() {
			m.checkoutBranch(branch.Name)
		}
	}
	if menu.Item("New Branch…").Chosen() {
		m.OpenNewGitBranchAt(branch.Name)
	}
	if menu.Item("Show History").Chosen() {
		m.ShowLogWithArg(map[string]any{"branch": branch.Name})
	}
	if !branch.Current {
		if m.branch.head != nil {
			if menu.Item(fmt.Sprintf("Checkout and Rebase onto '%s'", *m.branch.head)).Chosen() {
				m.checkoutAndRebaseBranch(branch.Name, *m.branch.head)
			}
		}
		if menu.Item("Merge into Current").Chosen() {
			m.OpenGitMergeAt(branch.Name)
		}
		if menu.Item("Rebase Current onto Branch").Chosen() {
			m.OpenGitRebaseAt(branch.Name)
		}
		if menu.Item("Compare with Current").Chosen() {
			m.compareRevisions(branch.Name, "HEAD")
		}
		if menu.Item("Show Diff with Working Tree").Chosen() {
			m.showDiffWithWorkingTreeRevision(branch.Name, branch.Name)
		}
	}
	if !branch.Remote {
		if menu.Item("Update").Disabled(!branch.Current && branch.Upstream == nil).Chosen() {
			m.runVCSRemote(vcsOpFetch, "Update "+branch.Name,
				map[string]any{"op": "updateBranch", "branch": branch.Name})
		}
		if menu.Item("Push…").Chosen() {
			m.OpenGitPush()
		}
	} else {
		if menu.Item("Pull into Current Using Rebase").Chosen() {
			m.pullRemoteBranchIntoCurrent(branch.Name, true)
		}
		if menu.Item("Pull into Current Using Merge").Chosen() {
			m.pullRemoteBranchIntoCurrent(branch.Name, false)
		}
	}
	favorite := m.branchFavorite(branch.Name)
	if menu.Item("Mark/Unmark As Favorite").Checked(favorite).Chosen() {
		m.toggleBranchFavorite(branch.Name)
	}
	if !branch.Remote && menu.Item("Rename…").Chosen() {
		m.startBranchRename(branch)
	}
	if !branch.Current {
		if menu.Item("Delete").Chosen() {
			if branch.Remote {
				m.startRemoteBranchDelete(branch)
			} else {
				m.deleteLocalBranch(branch)
			}
		}
	}
}

func (m *Model) checkoutAndRebaseBranch(name, onto string) {
	repoRoot := m.vcsRepositoryRoot()
	if name == "" || onto == "" || m.vcsBranchPaneForRepository(repoRoot).busy {
		return
	}
	root := m.Root()
	repoArg := m.vcsRepositoryArg()
	m.updateVCSBranchPane(repoRoot, func(branch *BranchPane) {
		branch.busy, branch.err = true, ""
	})
	m.status = "Checking out and rebasing " + name + " onto " + onto + "…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.mergeContext(), 5*time.Minute)
		defer cancel()
		_, err := m.invoke(ctx, "vcs_checkout", map[string]any{
			"repo": repoArg, "branch": name, "create": false,
			"options": map[string]any{"mode": "normal", "diverged": nil},
		})
		if err == nil {
			_, err = m.invoke(ctx, "vcs_rebase_with", map[string]any{
				"repo": repoArg, "upstream": onto,
				"options": map[string]any{
					"onto": nil, "rebaseMerges": false, "keepEmpty": false,
				},
			})
		}
		m.post(func() {
			m.updateVCSBranchPane(repoRoot, func(branch *BranchPane) {
				branch.busy = false
			})
			if m.Root() != root {
				return
			}
			if err != nil {
				m.updateVCSBranchPane(repoRoot, func(branch *BranchPane) {
					branch.err = "Checkout and rebase failed: " + err.Error()
				})
				return
			}
			if vcsRepositoryKey(m.vcs.current) == vcsRepositoryKey(repoRoot) {
				m.closeBranches()
			}
			m.status = "Checked out and rebased " + name + " onto " + onto
			m.RefreshAfterVCSCommand()
		})
	}()
}

func (m *Model) pullRemoteBranchIntoCurrent(name string, rebase bool) {
	i := strings.IndexByte(name, '/')
	if i <= 0 || i+1 >= len(name) {
		m.status = "The remote branch has no remote name"
		m.invalidate()
		return
	}
	title := "Pull into Current"
	if rebase {
		title += " Using Rebase"
	} else {
		title += " Using Merge"
	}
	m.runVCSRemote(vcsOpPull, title, map[string]any{
		"op": "pullFrom", "remote": name[:i], "branches": []string{name[i+1:]}, "rebase": rebase,
	})
}

// showDiffWithWorkingTreeRevision reuses the Git Log's per-file compare sheet
// for a branch selected in the Branches popup.
func (m *Model) showDiffWithWorkingTreeRevision(revision, label string) {
	root := m.Root()
	repoRoot := m.vcsRepositoryRoot()
	if repoRoot == "" {
		repoRoot = filepath.Clean(root)
	}
	repoArg := vcsRepositoryArgFor(repoRoot)
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 2*time.Minute)
		defer cancel()
		raw, err := m.invoke(ctx, "vcs_diff_with_worktree", map[string]any{
			"repo": repoArg, "rev": revision,
		})
		var files []openidea.RevisionChange
		if err == nil {
			err = json.Unmarshal(raw, &files)
		}
		m.post(func() {
			if m.Root() != root {
				return
			}
			if current := m.vcsRepositoryRoot(); current != "" &&
				vcsRepositoryKey(current) != vcsRepositoryKey(repoRoot) {
				return
			}
			if err != nil {
				m.status = "Compare with working tree failed: " + err.Error()
				m.invalidate()
				return
			}
			m.vcsMore = vcsMoreState{
				open: true, kind: vcsMoreChanges, action: "commit.local",
				title: "Compare with Working Tree · " + label,
				hint:  "Choose a file to compare the revision with the working tree.",
			}
			for _, file := range files {
				path := file.Path
				if !filepath.IsAbs(path) {
					path = filepath.Join(repoRoot, path)
				}
				m.vcsMore.items = append(m.vcsMore.items, vcsMoreItem{
					Path: path, Revision: revision, Label: filepath.Base(path),
					Detail: strings.ToUpper(file.Kind), Kind: file.Kind,
				})
			}
			m.invalidate()
		})
	}()
}

func (m *Model) branchFavorite(name string) bool {
	root := m.vcsBranchFavoriteRepositoryRoot()
	if name == "" || root == "" {
		return false
	}
	return m.prefs.VCS.FavoriteBranches[gitLogScopedKey(root, name)]
}

func (m *Model) vcsBranchFavoriteRepositoryRoot() string {
	if root := m.vcsRepositoryRoot(); root != "" {
		return root
	}
	if len(m.vcs.repositories) == 0 {
		return vcsRepositoryKey(m.Root())
	}
	return ""
}

func (m *Model) toggleBranchFavorite(name string) {
	root := m.vcsBranchFavoriteRepositoryRoot()
	if name == "" || root == "" {
		return
	}
	if m.prefs.VCS.FavoriteBranches == nil {
		m.prefs.VCS.FavoriteBranches = map[string]bool{}
	}
	key := gitLogScopedKey(root, name)
	if m.prefs.VCS.FavoriteBranches[key] {
		delete(m.prefs.VCS.FavoriteBranches, key)
	} else {
		m.prefs.VCS.FavoriteBranches[key] = true
	}
	m.savePreferences()
	m.invalidate()
}

func (m *Model) startBranchRename(branch openidea.Branch) {
	if branch.Remote || branch.Name == "" {
		return
	}
	m.branch.prompt = "rename"
	m.branch.promptBranch = branch
	m.branch.promptName = branch.Name
	m.branch.promptUnset = false
	m.branch.promptErr = ""
	m.invalidate()
}

func (m *Model) startRemoteBranchDelete(branch openidea.Branch) {
	if !branch.Remote || branch.Name == "" {
		return
	}
	m.branch.prompt = "delete-remote"
	m.branch.promptBranch = branch
	m.branch.promptName = ""
	m.branch.promptUnset = false
	m.branch.promptErr = ""
	m.invalidate()
}

func (m *Model) cancelBranchPrompt() {
	m.branch.prompt = ""
	m.branch.promptBranch = openidea.Branch{}
	m.branch.promptName = ""
	m.branch.promptUnset = false
	m.branch.promptErr = ""
	m.invalidate()
}

// branchNameProblem mirrors the checks promptText applies before
// vcs_rename_branch, so the native prompt can explain the problem inline.
func branchNameProblem(name string, branches []openidea.Branch, oldName string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "Branch name must not be empty"
	}
	if name == oldName {
		return "Enter a new name"
	}
	invalid := " ~^:?*[\\"
	if strings.ContainsAny(name, invalid) || strings.HasPrefix(name, "-") ||
		strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") ||
		strings.HasSuffix(name, ".") || strings.HasSuffix(name, ".lock") ||
		strings.Contains(name, "..") || strings.Contains(name, "//") ||
		strings.Contains(name, "@{") || name == "@" {
		return "Invalid branch name"
	}
	for _, r := range name {
		if r < 32 || r == 127 {
			return "Branch name contains invalid characters"
		}
	}
	for _, branch := range branches {
		if !branch.Remote && branch.Name == name {
			return fmt.Sprintf("Branch '%s' already exists", name)
		}
	}
	return ""
}

func (m *Model) submitBranchPrompt() {
	if m.branch.busy {
		return
	}
	switch m.branch.prompt {
	case "rename":
		m.submitBranchRename()
	case "delete-remote":
		m.submitRemoteBranchDelete()
	case "delete-worktree":
		m.submitDeleteBranchWorktree(false)
	case "force-worktree":
		m.submitDeleteBranchWorktree(true)
	}
}

func (m *Model) submitBranchRename() {
	branch := m.branch.promptBranch
	name := strings.TrimSpace(m.branch.promptName)
	if problem := branchNameProblem(name, m.branch.branches, branch.Name); problem != "" {
		m.branch.promptErr = problem
		m.invalidate()
		return
	}
	root := m.Root()
	repoRoot := m.vcsRepositoryRoot()
	unsetUpstream := m.branch.promptUnset && branch.Upstream != nil
	repoArg := m.vcsRepositoryArg()
	m.updateVCSBranchPane(repoRoot, func(branchPane *BranchPane) {
		branchPane.busy, branchPane.promptErr = true, ""
	})
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.mergeContext(), 2*time.Minute)
		defer cancel()
		_, err := m.invoke(ctx, "vcs_rename_branch", map[string]any{
			"repo": repoArg, "from": branch.Name, "to": name, "unsetUpstream": unsetUpstream,
		})
		m.post(func() {
			m.updateVCSBranchPane(repoRoot, func(branchPane *BranchPane) {
				branchPane.busy = false
			})
			if m.Root() != root {
				return
			}
			if err != nil {
				m.updateVCSBranchPane(repoRoot, func(branchPane *BranchPane) {
					branchPane.promptErr = "Could not rename the branch: " + err.Error()
				})
				return
			}
			active := vcsRepositoryKey(m.vcs.current) == vcsRepositoryKey(repoRoot)
			if active {
				m.cancelBranchPrompt()
				m.closeBranches()
			}
			m.updateVCSBranchPane(repoRoot, func(branchPane *BranchPane) {
				if branchPane.head != nil && *branchPane.head == branch.Name {
					branchPane.head = &name
					if unsetUpstream {
						branchPane.upstream = nil
					}
				}
				for i := range branchPane.branches {
					if !branchPane.branches[i].Remote && branchPane.branches[i].Name == branch.Name {
						branchPane.branches[i].Name = name
						if unsetUpstream {
							branchPane.branches[i].Upstream = nil
						}
					}
				}
			})
			m.status = "Renamed branch " + branch.Name + " to " + name
			m.RefreshAfterVCSCommand()
			if active {
				m.reloadBranchRows()
			}
		})
	}()
}

func (m *Model) submitRemoteBranchDelete() {
	branch := m.branch.promptBranch
	name := branch.Name
	m.branch.prompt = ""
	m.branch.promptErr = ""
	m.closeBranches()
	m.runVCSRemote(vcsOpFetch, "Delete Remote Branch", map[string]any{
		"op": "deleteBranch", "branch": name,
	})
}

func (m *Model) deleteLocalBranch(branch openidea.Branch) {
	repoRoot := m.vcsRepositoryRoot()
	if m.vcsBranchPaneForRepository(repoRoot).busy ||
		branch.Remote || branch.Current || branch.Name == "" {
		return
	}
	root := m.Root()
	repoArg := m.vcsRepositoryArg()
	m.updateVCSBranchPane(repoRoot, func(pane *BranchPane) {
		pane.busy, pane.err = true, ""
	})
	m.status = "Deleting branch " + branch.Name + "…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.mergeContext(), 2*time.Minute)
		defer cancel()
		raw, worktreeErr := m.invoke(ctx, "vcs_worktrees", map[string]any{"repo": repoArg})
		var worktrees []openidea.Worktree
		if worktreeErr == nil {
			worktreeErr = decodeOpenIDEAResponse(raw, &worktrees)
		}
		if worktreeErr == nil {
			if holding, ok := branchHoldingWorktree(branch.Name, worktrees); ok &&
				!worktreeContainsProject(holding.Path, root) {
				m.post(func() {
					m.updateVCSBranchPane(repoRoot, func(pane *BranchPane) {
						pane.busy, pane.err = false, ""
						pane.prompt = "delete-worktree"
						pane.promptBranch = branch
						pane.promptName = holding.Path
						pane.promptErr = ""
					})
				})
				return
			}
		}
		raw, err := m.invoke(ctx, "vcs_delete_branch", map[string]any{
			"repo": repoArg, "branch": branch.Name,
		})
		var deleted openidea.DeletedBranch
		if err == nil {
			err = json.Unmarshal(raw, &deleted)
		}
		m.post(func() {
			m.updateVCSBranchPane(repoRoot, func(pane *BranchPane) {
				pane.busy = false
			})
			if m.Root() != root {
				return
			}
			if err != nil {
				m.updateVCSBranchPane(repoRoot, func(pane *BranchPane) {
					pane.err = "Could not delete the branch: " + err.Error()
				})
				return
			}
			active := vcsRepositoryKey(m.vcs.current) == vcsRepositoryKey(repoRoot)
			if active {
				m.closeBranches()
				m.reloadBranchRows()
			}
			m.status = "Deleted branch " + branch.Name
			m.RefreshAfterVCSCommand()
			m.showDeletedBranchNotification(root, repoRoot, deleted)
		})
	}()
}

// branchHoldingWorktree finds the linked worktree that makes Git refuse to
// delete a local branch. The main worktree is deliberately ignored so Git's
// own refusal remains visible when it holds the branch.
func branchHoldingWorktree(name string, worktrees []openidea.Worktree) (openidea.Worktree, bool) {
	for _, worktree := range worktrees {
		if worktree.Main || worktree.Branch == nil || *worktree.Branch != name {
			continue
		}
		return worktree, true
	}
	return openidea.Worktree{}, false
}

// submitDeleteBranchWorktree removes the confirmed linked worktree without
// force first. A failed removal opens an explicit Force Remove confirmation.
// Branch deletion runs only after the worktree is gone.
func (m *Model) submitDeleteBranchWorktree(force bool) {
	if m.branch.busy {
		return
	}
	branch := m.branch.promptBranch
	path := m.branch.promptName
	if branch.Name == "" || path == "" {
		m.branch.promptErr = "The linked worktree is no longer selected"
		m.invalidate()
		return
	}
	if worktreeContainsProject(path, m.Root()) {
		m.branch.promptErr = "This worktree contains the open project and cannot be removed"
		m.invalidate()
		return
	}
	root := m.Root()
	repoRoot := m.vcsRepositoryRoot()
	repoArg := m.vcsRepositoryArg()
	m.updateVCSBranchPane(repoRoot, func(pane *BranchPane) {
		pane.busy, pane.promptErr = true, ""
	})
	go func() {
		ctx, cancel := context.WithTimeout(m.mergeContext(), 2*time.Minute)
		defer cancel()
		_, err := m.invoke(ctx, "vcs_remove_worktree", map[string]any{
			"repo": repoArg, "path": path, "force": force,
		})
		if err != nil {
			m.post(func() {
				m.updateVCSBranchPane(repoRoot, func(pane *BranchPane) {
					pane.busy = false
					pane.prompt = "force-worktree"
					pane.promptErr = "Could not remove the worktree: " + err.Error()
				})
			})
			return
		}
		raw, err := m.invoke(ctx, "vcs_delete_branch", map[string]any{
			"repo": repoArg, "branch": branch.Name,
		})
		var deleted openidea.DeletedBranch
		if err == nil {
			err = decodeOpenIDEAResponse(raw, &deleted)
		}
		m.post(func() {
			m.updateVCSBranchPane(repoRoot, func(pane *BranchPane) {
				pane.busy = false
			})
			if m.Root() != root {
				return
			}
			if err != nil {
				m.cancelBranchPrompt()
				m.updateVCSBranchPane(repoRoot, func(pane *BranchPane) {
					pane.err = "The worktree was removed, but the branch could not be deleted: " + err.Error()
				})
				return
			}
			m.cancelBranchPrompt()
			active := vcsRepositoryKey(m.vcs.current) == vcsRepositoryKey(repoRoot)
			if active {
				m.closeBranches()
				m.reloadBranchRows()
			}
			m.status = "Deleted worktree and branch " + branch.Name
			m.RefreshAfterVCSCommand()
			m.showDeletedBranchNotification(root, repoRoot, deleted)
		})
	}()
}

func (m *Model) showDeletedBranchNotification(
	root, repoRoot string,
	deleted openidea.DeletedBranch,
) {
	if deleted.Name == "" {
		return
	}
	body := ""
	if deleted.Unmerged {
		body = "Unmerged commits were discarded"
	}
	actions := []notifyAction{{
		label: "Restore",
		run: func(m *Model) {
			m.restoreDeletedBranch(root, repoRoot, deleted)
		},
	}}
	if deleted.Unmerged && deleted.Tip != "" {
		tip := deleted.Tip
		actions = append(actions, notifyAction{
			label: "View Commits",
			run: func(m *Model) {
				if m.Root() != root {
					m.status = "Reopen the branch's project to view its commits"
					m.invalidate()
					return
				}
				m.ShowLogWithArg(map[string]any{"repo": repoRoot})
				m.goToGitLogRef(tip)
			},
		})
	}
	if deleted.Upstream != nil && *deleted.Upstream != "" {
		upstream := *deleted.Upstream
		actions = append(actions, notifyAction{
			label: "Delete Tracked Branch",
			run: func(m *Model) {
				if m.Root() != root {
					m.status = "Reopen the branch's project before deleting its tracked branch"
					m.invalidate()
					return
				}
				m.runVCSRemoteAt(vcsRepositoryArgFor(repoRoot), vcsOpFetch,
					"Delete Remote Branch", map[string]any{
						"op": "deleteBranch", "branch": upstream,
					})
			},
		})
	}
	m.notifyInfo("vcs.deleted."+deleted.Name, "Deleted branch "+deleted.Name, body, actions...)
}

func (m *Model) restoreDeletedBranch(
	root, repoRoot string,
	deleted openidea.DeletedBranch,
) {
	if m.Root() != root || deleted.Name == "" || deleted.Tip == "" {
		m.status = "Reopen the branch's project before restoring it"
		m.invalidate()
		return
	}
	m.status = "Restoring branch " + deleted.Name + "…"
	m.invalidate()
	repoArg := vcsRepositoryArgFor(repoRoot)
	go func() {
		ctx, cancel := context.WithTimeout(m.mergeContext(), 2*time.Minute)
		defer cancel()
		_, err := m.invoke(ctx, "vcs_restore_branch", map[string]any{
			"repo": repoArg, "deleted": deleted,
		})
		m.post(func() {
			if m.Root() != root {
				return
			}
			if err != nil {
				m.notifyError("vcs.deleted."+deleted.Name, "Restore branch "+deleted.Name,
					"Could not restore the branch: "+err.Error())
				return
			}
			m.status = "Restored branch " + deleted.Name
			if vcsRepositoryKey(m.vcs.current) == vcsRepositoryKey(repoRoot) {
				m.reloadBranchRows()
			}
			m.RefreshAfterVCSCommand()
		})
	}()
}

func (m *Model) reloadBranchRows() {
	if !m.branch.open {
		return
	}
	m.branch.listing = true
	m.branch.branchesLoaded = false
	m.loadBranches()
}

// checkoutBranch switches the repository to name and refreshes what shows it.
// The branch already checked out is a no-op that closes the popup.
func (m *Model) checkoutBranch(name string) {
	if name == "" || m.Root() == "" {
		return
	}
	repoRoot := m.vcsRepositoryRoot()
	branch := m.vcsBranchPaneForRepository(repoRoot)
	if branch.head != nil && name == *branch.head {
		m.closeBranches()
		return
	}
	if branch.busy {
		return
	}
	m.updateVCSBranchPane(repoRoot, func(branch *BranchPane) {
		branch.busy, branch.err = true, ""
	})
	m.status = "Switching to " + name + "…"
	m.invalidate()
	repoArg := m.vcsRepositoryArg()
	go func() {
		ctx, cancel := context.WithTimeout(m.mergeContext(), 2*time.Minute)
		defer cancel()
		_, err := m.invoke(ctx, "vcs_checkout", map[string]any{
			"repo": repoArg, "branch": name, "create": false, "options": nil,
		})
		m.post(func() {
			m.updateVCSBranchPane(repoRoot, func(branch *BranchPane) {
				branch.busy = false
			})
			if m.Root() == "" {
				return
			}
			if err != nil {
				m.updateVCSBranchPane(repoRoot, func(branch *BranchPane) {
					branch.err = "Switching branches failed: " + err.Error()
				})
				return
			}
			if vcsRepositoryKey(m.vcs.current) == vcsRepositoryKey(repoRoot) {
				m.closeBranches()
			}
			m.status = "Switched to " + name
			m.RefreshBranch()
			if m.vcs.loaded || m.vcs.loading {
				m.RefreshVCS()
			}
			if m.log.visible {
				m.RefreshLog()
			}
			m.invalidate()
		})
	}()
}

// SmokeBranch exercises the widget's commands against the backend: the current
// head, the branch list, and a checkout that comes back to where it started.
func (m *Model) SmokeBranch(ctx context.Context) (map[string]any, error) {
	out := map[string]any{}
	info, err := m.branchInfo(ctx)
	if err != nil {
		return out, err
	}
	if info.Head != nil {
		out["head"] = *info.Head
	}
	branches, err := m.branchList(ctx)
	if err != nil {
		return out, err
	}
	out["branches"] = len(branches)
	for _, b := range branches {
		if b.Current {
			out["current"] = b.Name
		}
	}
	if info.Head == nil {
		return out, nil
	}
	const temp = "goidea-smoke"
	if _, err := m.mgr.Invoke(ctx, Window, "vcs_create_branch", map[string]any{
		"repo": nil, "name": temp, "start": nil, "checkout": true, "overwrite": nil,
	}); err != nil {
		return out, err
	}
	if now, err := m.branchInfo(ctx); err == nil && now.Head != nil {
		out["checkedOut"] = *now.Head
	}
	if _, err := m.mgr.Invoke(ctx, Window, "vcs_checkout", map[string]any{
		"repo": nil, "branch": *info.Head, "create": false, "options": nil,
	}); err != nil {
		return out, err
	}
	if _, err := m.mgr.Invoke(ctx, Window, "vcs_delete_branch", map[string]any{
		"repo": nil, "branch": temp,
	}); err != nil {
		return out, err
	}
	back, err := m.branchInfo(ctx)
	if err != nil {
		return out, err
	}
	out["restored"] = back.Head != nil && *back.Head == *info.Head
	return out, nil
}

func (m *Model) branchInfo(ctx context.Context) (openidea.BranchInfo, error) {
	var info openidea.BranchInfo
	raw, err := m.invoke(ctx, "vcs_branch_info", map[string]any{"repo": m.vcsRepositoryArg()})
	if err != nil {
		return info, err
	}
	err = json.Unmarshal(raw, &info)
	return info, err
}

func (m *Model) branchList(ctx context.Context) ([]openidea.Branch, error) {
	var branches []openidea.Branch
	raw, err := m.invoke(ctx, "vcs_branches", map[string]any{"repo": m.vcsRepositoryArg()})
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(raw, &branches)
	return branches, err
}

// branchErrorText turns the backend's refusal for a project without Git into
// "" (no widget, no error) and other failures into the sentence the popup
// shows.
func branchErrorText(err error) string {
	var oe *openidea.Error
	if errors.As(err, &oe) && oe.Kind == "unavailable" && strings.Contains(oe.Message, "not a git repository") {
		return ""
	}
	return "Could not read the branches: " + err.Error()
}

// branchesDialog draws the Branches popup: a filter over the branches, the
// current one marked, and Enter or a click checking one out.
func (m *Model) branchesDialog(c *ui.Context, p *ui.Theme) {
	if !m.branch.open {
		return
	}
	if m.branch.prompt != "" {
		m.branchPromptDialog(c, p)
		return
	}
	open := m.branch.open
	d := nativeModal(c, &open, func() {
		ui.Column(c).Width(520).Gap(8).Children(func() {
			ui.Text(c, "Branches").FontSize(13).Bold().TextColor(p.Text).SingleLine()
			in := ui.TextInput(c, &m.branch.query).AutoFocus()
			in.OnChange(func() {
				m.branch.sel = 0
				m.invalidate()
			})
			in.OnShortcut(0, ui.KeyDown, func() { m.stepBranch(1) })
			in.OnShortcut(0, ui.KeyUp, func() { m.stepBranch(-1) })
			if in.Submitted() {
				m.CheckoutSelectedBranch()
			}
			rows := m.branchRows()
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				text := "Type a name to filter; Enter checks the branch out"
				switch {
				case m.branch.busy:
					text = "Switching…"
				case m.branch.err != "":
					text = ""
				case m.branch.listing:
					text = "Reading the branches…"
				case len(rows) == 0:
					text = "No matching branches"
				default:
					text = count(len(rows), "branch", "branches")
				}
				if text != "" {
					ui.Text(c, text).FontSize(12).TextColor(p.TextMuted).SingleLine()
				}
			})
			if m.branch.err != "" {
				ui.Text(c, m.branch.err).FontSize(12).TextColor(p.Danger).SingleLine().Ellipsis("…")
			}
			if len(rows) == 0 {
				return
			}
			ui.Scroll(c).Height(330).Children(func() {
				ui.Column(c).Children(func() {
					for i, b := range rows {
						i, b := i, b
						r := ui.Row(c.Key(fmt.Sprintf("branch:%s", b.Name))).Height(24).Padding(0, 8).
							Gap(8).AlignItems(ui.Center)
						if i == m.branch.sel {
							r.Background(p.SurfaceHover)
						} else if r.Hovered() {
							r.Background(p.SurfaceHover)
						}
						r.Children(func() {
							color := p.Text
							if b.Remote {
								color = p.TextMuted
							}
							if m.branchFavorite(b.Name) {
								ui.Text(c, "★").FontSize(12).TextColor(p.Accent).SingleLine()
							}
							ui.Text(c, b.Name).FontSize(13).TextColor(color).SingleLine().Ellipsis("…")
							if b.Current {
								ui.Text(c, "Current").FontSize(12).TextColor(p.Accent).SingleLine()
							}
							if b.Upstream != nil && !b.Remote {
								ui.Text(c, "Tracking "+*b.Upstream).FontSize(12).TextColor(p.TextMuted).
									SingleLine().Ellipsis("…")
							}
							if b.Behind > 0 {
								ui.Text(c, fmt.Sprintf("↓%d", b.Behind)).FontSize(12).
									TextColor(p.TextMuted).SingleLine()
							}
							if b.Ahead > 0 {
								ui.Text(c, fmt.Sprintf("↑%d", b.Ahead)).FontSize(12).
									TextColor(p.TextMuted).SingleLine()
							}
						})
						if r.Clicked() {
							m.checkoutBranch(b.Name)
						}
						r.ContextMenu(func(menu *ui.Menu) { m.branchRowMenu(menu, b) })
					}
				})
			})
		})
	})
	if d.Dismissed() || !open {
		m.closeBranches()
	}
}

func (m *Model) branchPromptDialog(c *ui.Context, p *ui.Theme) {
	open := true
	d := nativeModal(c.Key("branches:"+m.branch.prompt), &open, func() {
		ui.Column(c).Width(440).Gap(10).Children(func() {
			switch m.branch.prompt {
			case "rename":
				branch := m.branch.promptBranch
				ui.Text(c, "Rename Branch "+branch.Name).FontSize(13).Bold().
					TextColor(p.Text).SingleLine().Ellipsis("…")
				ui.Text(c, "New branch name:").FontSize(12).TextColor(p.TextMuted)
				in := ui.TextInput(c, &m.branch.promptName).AutoFocus()
				if in.Submitted() {
					m.submitBranchPrompt()
				}
				if branch.Upstream != nil {
					ui.Checkbox(c, &m.branch.promptUnset, "Unset upstream branch")
				}
				if m.branch.promptErr != "" {
					ui.Text(c, m.branch.promptErr).FontSize(12).TextColor(p.Danger)
				}
				ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
					if ui.Button(c, "Rename").Disabled(m.branch.busy).Clicked() {
						m.submitBranchPrompt()
					}
					if ui.Button(c, "Cancel").Clicked() {
						m.cancelBranchPrompt()
					}
				})
			case "delete-remote":
				branch := m.branch.promptBranch
				ui.Text(c, "Delete Remote Branch").FontSize(13).Bold().TextColor(p.Text)
				ui.Text(c, "Delete branch '"+shortRemoteBranchName(branch.Name)+
					"' on the remote? The remote-tracking branch '"+branch.Name+"' is removed as well.").
					FontSize(12).TextColor(p.Text)
				if m.branch.promptErr != "" {
					ui.Text(c, m.branch.promptErr).FontSize(12).TextColor(p.Danger)
				}
				ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
					if ui.Button(c, "Delete").Clicked() {
						m.submitBranchPrompt()
					}
					if ui.Button(c, "Cancel").Clicked() {
						m.cancelBranchPrompt()
					}
				})
			case "delete-worktree":
				branch := m.branch.promptBranch
				ui.Text(c, "Delete Branch").FontSize(13).Bold().TextColor(p.Text)
				ui.Text(c, fmt.Sprintf(
					"Branch '%s' is checked out in the worktree at %s.\nDelete the worktree and the branch? Its folder is deleted.",
					branch.Name, m.branch.promptName)).
					FontSize(12).TextColor(p.Text)
				if m.branch.promptErr != "" {
					ui.Text(c, m.branch.promptErr).FontSize(12).TextColor(p.Danger)
				}
				ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
					if ui.Button(c, "Delete").Disabled(m.branch.busy).Clicked() {
						m.submitBranchPrompt()
					}
					if ui.Button(c, "Cancel").Clicked() {
						m.cancelBranchPrompt()
					}
				})
			case "force-worktree":
				branch := m.branch.promptBranch
				ui.Text(c, "Force Remove Worktree").FontSize(13).Bold().TextColor(p.Text)
				ui.Text(c, fmt.Sprintf(
					"Branch '%s' is still checked out at %s. Force removal discards that worktree's changes and deletes its folder.",
					branch.Name, m.branch.promptName)).
					FontSize(12).TextColor(p.Text)
				if m.branch.promptErr != "" {
					ui.Text(c, m.branch.promptErr).FontSize(12).TextColor(p.Danger)
				}
				ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
					if ui.Button(c, "Force Remove").Disabled(m.branch.busy).Clicked() {
						m.submitBranchPrompt()
					}
					if ui.Button(c, "Cancel").Clicked() {
						m.cancelBranchPrompt()
					}
				})
			}
		})
	})
	if d.Dismissed() || !open {
		m.cancelBranchPrompt()
	}
}

func shortRemoteBranchName(name string) string {
	if i := strings.IndexByte(name, '/'); i >= 0 && i+1 < len(name) {
		return name[i+1:]
	}
	return name
}
