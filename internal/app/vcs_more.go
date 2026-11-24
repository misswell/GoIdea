package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

const (
	vcsMorePrompt       = "prompt"
	vcsMoreConfirm      = "confirm"
	vcsMoreClone        = "clone"
	vcsMoreShelf        = "shelf"
	vcsMoreWorktrees    = "worktrees"
	vcsMoreChangelists  = "changelists"
	vcsMorePatchPreview = "patch"
	vcsMoreChanges      = "changes"
	vcsMoreConflicts    = "conflicts"
	vcsMoreMessage      = "message"
	vcsMoreRebase       = "rebase"
)

const (
	vcsMoreActionShelve         = "shelve"
	vcsMoreActionShelfRename    = "shelf.rename"
	vcsMoreActionWorktreeAdd    = "worktree.add"
	vcsMoreActionWorktreeMove   = "worktree.move"
	vcsMoreActionChangelistNew  = "changelist.new"
	vcsMoreActionChangelistEdit = "changelist.edit"
	vcsMoreActionReword         = "reword"
	vcsMoreActionResetToCommit  = "reset.commit"
	vcsMoreActionCheckout       = "checkout"
	vcsMoreActionCreatePatch    = "createPatch"
)

type vcsMoreItem struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Detail   string `json:"detail"`
	Path     string `json:"path"`
	Revision string `json:"revision"`
	Kind     string `json:"kind"`
	Before   string `json:"before"`
	After    string `json:"after"`
}

type shelvedChangeList struct {
	ID     string                    `json:"id"`
	Name   string                    `json:"name"`
	TimeMs *float64                  `json:"timeMs"`
	Branch *string                   `json:"branch"`
	Repo   *string                   `json:"repo"`
	Files  []openidea.RevisionChange `json:"files"`
}

type shelfDiffFile struct {
	Path   string  `json:"path"`
	Patch  string  `json:"patch"`
	Before *string `json:"before"`
	After  *string `json:"after"`
}

type changelistEntry struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Comment string `json:"comment"`
}

type changelistState struct {
	Lists  []changelistEntry `json:"lists"`
	Active string            `json:"active"`
	Files  map[string]string `json:"files"`
}

type patchPreviewFile struct {
	Path    string  `json:"path"`
	OldPath *string `json:"oldPath"`
	Kind    string  `json:"kind"`
	Added   int     `json:"added"`
	Removed int     `json:"removed"`
	Binary  bool    `json:"binary"`
	Applies bool    `json:"applies"`
}

type patchApplyOutcome struct {
	Applied   []string `json:"applied"`
	Conflicts []string `json:"conflicts"`
}

type mergeSides struct {
	Operation   string  `json:"operation"`
	Branch      *string `json:"branch"`
	Commit      *string `json:"commit"`
	OtherBranch *string `json:"otherBranch"`
	Reversed    bool    `json:"reversed"`
}

type mergeVersions struct {
	Base   *string    `json:"base"`
	Ours   *string    `json:"ours"`
	Theirs *string    `json:"theirs"`
	Sides  mergeSides `json:"sides"`
	Merged *string    `json:"merged"`
}

// vcsMoreState owns the native sheets behind Git's less-frequent commands:
// clone, shelf, worktrees, change lists, patches and conflict resolution.
type vcsMoreState struct {
	open    bool
	kind    string
	action  string
	title   string
	hint    string
	err     string
	status  string
	loading bool
	busy    bool

	input  string
	input2 string
	input3 string

	choice    int
	flag      bool
	items     []vcsMoreItem
	output    []string
	patch     string
	shelves   []shelvedChangeList
	lists     changelistState
	worktrees []openidea.Worktree
	channel   uint32
	request   uint32
	loadID    uint32

	changelistsLoaded bool

	rebaseCommits  []openidea.Commit
	rebaseActions  []string
	rebaseMessages []string
	rebaseBase     string
}

func init() {
	add := func(id, title, category, keywords string, whenProject bool, run func(*Model, any)) {
		when := func(m *Model) bool { return !whenProject || m.Root() != "" }
		_ = registerAction(&Action{
			ID: id, Title: title, Category: category, Keywords: keywords, Builtin: true,
			When: when,
			Run:  func(m *Model) { run(m, nil) },
			RunWithArg: func(m *Model, arg any) {
				run(m, arg)
			},
		})
	}

	add("vcs.refresh", "Refresh VCS Status", "Git", "reload git status", true,
		func(m *Model, _ any) { m.RefreshAfterVCSCommand() })
	add("vcs.rollback", "Rollback…", "Git", "revert discard changes", true,
		func(m *Model, arg any) { m.AskRollbackAction(arg) })
	add("vcs.showDiff", "Show Diff", "Git", "compare revision working tree", true,
		func(m *Model, arg any) { m.ShowDiffAction(arg) })
	add("vcs.annotate", "Annotate with Git Blame", "Git", "blame annotations", true,
		func(m *Model, arg any) { m.ShowAnnotateAction(arg) })
	add("vcs.localHistory", "Show Local History", "Git", "history revision", true,
		func(m *Model, arg any) { m.ShowLocalHistoryAction(arg) })
	add("localHistory.putLabel", "Local History: Put Label…", "Git", "history label checkpoint", true,
		func(m *Model, _ any) { m.OpenHistoryLabel() })
	add("vcs.addToVcs", "Add to VCS", "Git", "git add stage", true,
		func(m *Model, arg any) { m.AddPathsToVCS(arg) })
	add("vcs.ignoreShared", "Add to .gitignore", "Git", "ignore exclude", true,
		func(m *Model, arg any) { m.IgnorePaths(arg, "gitignore") })
	add("vcs.ignoreLocal", "Add to .git/info/exclude", "Git", "ignore local exclude", true,
		func(m *Model, arg any) { m.IgnorePaths(arg, "localExclude") })
	add("vcs.clone", "Get from Version Control…", "Git", "clone checkout git", false,
		func(m *Model, arg any) { m.OpenCloneDialog(arg) })
	add("vcs.commitAndPush", "Commit and Push…", "Git", "commit push", true,
		func(m *Model, _ any) { m.CommitAndPush() })
	add("vcs.toggleAmend", "Amend Commit", "Git", "amend previous", true,
		func(m *Model, _ any) { m.ToggleCommitAmend() })
	add("vcs.showMessageHistory", "Show Commit Message History", "Git", "message history", true,
		func(m *Model, _ any) { m.OpenCommitMessageHistory() })
	add("vcs.newWorktree", "New Worktree…", "Git", "worktree", true,
		func(m *Model, _ any) { m.OpenNewVCSWorktree() })
	add("vcs.worktrees", "Worktrees…", "Git", "worktree", true,
		func(m *Model, _ any) { m.OpenVCSWorktrees() })
	add("vcs.checkoutRevision", "Checkout Tag or Revision…", "Git", "checkout tag revision", true,
		func(m *Model, arg any) { m.OpenCheckoutRevision(arg) })
	add("vcs.compareWithBranch", "Compare with Branch or Revision…", "Git", "diff branch revision", true,
		func(m *Model, arg any) { m.OpenCompareWithRevision(arg) })
	add("vcs.compareWithRevision", "Compare with Revision…", "Git", "file revision history", true,
		func(m *Model, arg any) { m.OpenCompareWithRevision(arg) })
	add("vcs.showLog", "Show Git Log", "Git", "history commits", true,
		func(m *Model, arg any) { m.ShowLogWithArg(arg) })
	add("vcs.showHistory", "Show History for File", "Git", "history file commits", true,
		func(m *Model, arg any) { m.ShowHistoryForFile(arg) })
	add("vcs.showCurrentRevision", "Show Current Revision", "Git", "current revision", true,
		func(m *Model, arg any) { m.ShowCurrentRevision(arg) })
	add("vcs.showHistoryForSelection", "Show History for Selection", "Git", "line history block", true,
		func(m *Model, _ any) { m.ShowHistoryForSelection() })
	add("vcs.shelve", "Shelve Changes…", "Git", "shelf stash", true,
		func(m *Model, arg any) { m.OpenShelveChanges(arg) })
	add("vcs.showShelf", "Show Shelf", "Git", "shelf", true,
		func(m *Model, _ any) { m.OpenVCSShelf() })
	add("vcs.unshelve", "Unshelve…", "Git", "shelf apply", true,
		func(m *Model, arg any) { m.UnshelveArg(arg) })
	add("vcs.shelf.delete", "Delete Shelved Changes", "Git", "shelf remove", true,
		func(m *Model, arg any) { m.ShelfDeleteArg(arg) })
	add("vcs.shelf.rename", "Rename Shelved Changes…", "Git", "shelf name", true,
		func(m *Model, arg any) { m.ShelfRenameArg(arg) })
	add("vcs.shelf.refresh", "Refresh Shelf", "Git", "shelf reload", true,
		func(m *Model, _ any) { m.LoadVCSShelf() })
	add("vcs.shelf.showDiff", "Show Shelved Diff", "Git", "shelf diff", true,
		func(m *Model, arg any) { m.ShowShelfDiffArg(arg, false) })
	add("vcs.shelf.showDiffNewTab", "Show Diff in a New Tab", "Git", "shelf diff", true,
		func(m *Model, arg any) { m.ShowShelfDiffArg(arg, false) })
	add("vcs.shelf.compareLocal", "Compare Shelved Changes with Local", "Git", "shelf local diff", true,
		func(m *Model, arg any) { m.ShowShelfDiffArg(arg, true) })
	add("vcs.shelf.createPatch", "Create Patch from Shelved Changes…", "Git", "shelf patch save", true,
		func(m *Model, arg any) { m.CreateShelfPatchArg(arg) })
	add("vcs.shelf.copyPatch", "Copy as Patch to Clipboard", "Git", "shelf patch clipboard", true,
		func(m *Model, arg any) { m.CopyShelfPatchArg(arg) })
	add("vcs.shelf.importPatches", "Import Patches…", "Git", "patch shelf import", true,
		func(m *Model, arg any) { m.ImportShelfPatches(arg) })
	add("vcs.createPatch", "Create Patch…", "Git", "patch save", true,
		func(m *Model, arg any) { m.CreatePatchForPaths(arg) })
	add("vcs.applyPatch", "Apply Patch…", "Git", "patch apply", true,
		func(m *Model, arg any) { m.OpenApplyPatch(arg) })
	add("vcs.applyPatchFromClipboard", "Apply Patch from Clipboard…", "Git", "patch clipboard", true,
		func(m *Model, _ any) { m.OpenApplyPatchFromClipboard() })
	add("vcs.changelist.new", "New Changelist…", "Git", "change list", true,
		func(m *Model, _ any) { m.OpenNewChangelist() })
	add("vcs.changelist.edit", "Edit Changelist…", "Git", "change list rename", true,
		func(m *Model, arg any) { m.OpenEditChangelist(arg) })
	add("vcs.changelist.delete", "Delete Changelist", "Git", "change list remove", true,
		func(m *Model, arg any) { m.DeleteChangelistArg(arg) })
	add("vcs.changelist.setActive", "Set Active Changelist", "Git", "change list active", true,
		func(m *Model, arg any) { m.SetActiveChangelistArg(arg) })
	add("vcs.changelist.move", "Move to Another Changelist…", "Git", "change list move", true,
		func(m *Model, arg any) { m.MoveToChangelist(arg) })
	add("vcs.changelist.commit", "Commit Changelist…", "Git", "change list commit", true,
		func(m *Model, arg any) { m.CommitChangelistArg(arg) })
	add("vcs.resolveConflicts", "Resolve Conflicts…", "Git", "merge conflict", true,
		func(m *Model, _ any) { m.OpenResolveConflicts() })
	add("vcs.acceptYours", "Accept Yours (Conflict at Caret)", "Git", "merge conflict ours", true,
		func(m *Model, _ any) { m.AcceptConflictAtCaret("ours") })
	add("vcs.acceptTheirs", "Accept Theirs (Conflict at Caret)", "Git", "merge conflict theirs", true,
		func(m *Model, _ any) { m.AcceptConflictAtCaret("theirs") })
	add("vcs.acceptBoth", "Accept Both (Conflict at Caret)", "Git", "merge conflict both", true,
		func(m *Model, _ any) { m.AcceptConflictAtCaret("both") })
	add("vcs.nextConflict", "Next Conflict Marker", "Git", "merge conflict next", true,
		func(m *Model, _ any) { m.StepConflict(1) })
	add("vcs.prevConflict", "Previous Conflict Marker", "Git", "merge conflict previous", true,
		func(m *Model, _ any) { m.StepConflict(-1) })
	add("vcs.log.copyRevision", "Copy Revision Number", "Git Log", "copy commit hash", true,
		func(m *Model, arg any) { m.CopyRevisionArg(arg) })
	add("vcs.log.createPatch", "Create Patch…", "Git Log", "commit patch", true,
		func(m *Model, arg any) { m.CreatePatchForCommits(arg) })
	add("vcs.log.checkoutRevision", "Checkout Revision", "Git Log", "detached head", true,
		func(m *Model, arg any) { m.CheckoutCommitArg(arg) })
	add("vcs.log.compareWithLocal", "Compare with Local", "Git Log", "commit local diff", true,
		func(m *Model, arg any) { m.CompareCommitWithLocal(arg) })
	add("vcs.log.compareVersions", "Compare Versions", "Git Log", "two commits diff", true,
		func(m *Model, arg any) { m.CompareRevisionsArg(arg) })
	add("vcs.log.resetToHere", "Reset Current Branch to Here…", "Git Log", "reset commit", true,
		func(m *Model, arg any) { m.ResetToCommitArg(arg) })
	add("vcs.log.undoCommit", "Undo Commit…", "Git Log", "undo last commit", true,
		func(m *Model, arg any) { m.UndoCommitArg(arg) })
	add("vcs.log.editMessage", "Edit Commit Message…", "Git Log", "reword commit", true,
		func(m *Model, arg any) { m.EditCommitMessageArg(arg) })
	add("vcs.log.dropCommit", "Drop Commit", "Git Log", "remove history", true,
		func(m *Model, arg any) { m.DropCommitArg(arg) })
	add("vcs.log.squashCommits", "Squash Commits…", "Git Log", "combine history", true,
		func(m *Model, arg any) { m.SquashCommitArg(arg) })
	add("vcs.log.fixup", "Fixup…", "Git Log", "fixup local changes", true,
		func(m *Model, arg any) { m.PrepareFixupArg(arg, "fixup") })
	add("vcs.log.squashInto", "Squash Into…", "Git Log", "squash local changes", true,
		func(m *Model, arg any) { m.PrepareFixupArg(arg, "squash") })
	add("vcs.log.pushUpToHere", "Push All up to Here…", "Git Log", "push history", true,
		func(m *Model, _ any) { m.OpenGitPushAtSelectedCommit() })
	add("vcs.log.rebaseOntoCommit", "Rebase Current Branch onto Selected Commit", "Git Log", "rebase", true,
		func(m *Model, arg any) { m.RebaseOntoCommitArg(arg) })
	add("vcs.log.interactiveRebase", "Interactively Rebase from Here…", "Git Log", "rebase squash fixup reword", true,
		func(m *Model, arg any) { m.OpenInteractiveRebase(arg) })
	add("vcs.log.goToParent", "Go to Parent Commit", "Git Log", "history parent", true,
		func(m *Model, _ any) { m.GoToLogParent() })
	add("vcs.log.goToChild", "Go to Child Commit", "Git Log", "history child", true,
		func(m *Model, _ any) { m.GoToLogChild() })
	add("vcs.log.newBranch", "New Branch…", "Git Log", "branch commit", true,
		func(m *Model, arg any) { m.NewBranchAtCommit(arg) })
	add("vcs.log.newTag", "New Tag…", "Git Log", "tag commit", true,
		func(m *Model, arg any) { m.NewTagAtCommit(arg) })
	add("vcs.log.openInBrowser", "View in Browser", "Git Log", "web commit", true,
		func(m *Model, arg any) { m.OpenCommitInBrowser(arg) })
}

func (m *Model) closeVCSMore() {
	m.vcsMore = vcsMoreState{}
	m.invalidate()
}

func (m *Model) openVCSMore(kind, action, title, hint string) {
	m.vcsMore = vcsMoreState{
		open: true, kind: kind, action: action, title: title, hint: hint,
	}
	m.invalidate()
}

func (m *Model) RefreshAfterVCSCommand() {
	m.RefreshVCS()
	m.RefreshBranch()
	m.refreshOpenBlame()
	if m.log.loaded || m.log.visible || m.currentVCSContent() == vcsContentLog {
		m.RefreshLog()
	}
	if m.currentVCSContent() == vcsContentWorktrees {
		m.refreshVCSWorktrees(m.vcs.current)
	}
	m.refreshLoadedVCSContentPages()
	m.status = "Refreshing VCS status"
	m.invalidate()
}

func (m *Model) AddPathsToVCS(arg any) {
	paths := m.vcsActionPaths(arg, true)
	if len(paths) == 0 {
		m.status = "Select a file to add to VCS"
		m.invalidate()
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
		defer cancel()
		_, err := m.mgr.Invoke(ctx, Window, "vcs_stage", map[string]any{"paths": paths})
		m.post(func() {
			if err != nil {
				m.status = "Could not add the file: " + err.Error()
			} else {
				m.status = "Added " + count(len(paths), "file", "files") + " to VCS"
				m.RefreshAfterVCSCommand()
			}
			m.invalidate()
		})
	}()
}

func (m *Model) IgnorePaths(arg any, destination string) {
	paths := m.vcsActionPaths(arg, true)
	if len(paths) == 0 {
		m.status = "Select a file to ignore"
		m.invalidate()
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
		defer cancel()
		_, err := m.mgr.Invoke(ctx, Window, "vcs_add_to_ignore", map[string]any{
			"paths": paths, "destination": destination,
		})
		m.post(func() {
			if err != nil {
				m.status = "Could not ignore the file: " + err.Error()
			} else {
				m.status = "Updated the ignore rules"
				m.RefreshAfterVCSCommand()
			}
			m.invalidate()
		})
	}()
}

func (m *Model) vcsActionPaths(arg any, changedOnly bool) []string {
	paths := actionPaths(arg)
	if len(paths) == 0 {
		if target, ok := m.selectedTreeEntry(); ok {
			paths = []string{target.path}
		} else if t := m.tab(); t != nil && t.doc != nil {
			paths = []string{t.doc.Path()}
		}
	}
	if !changedOnly {
		return paths
	}
	changed := make(map[string]bool, len(m.vcs.files))
	for _, file := range m.vcs.files {
		changed[realPath(file.Path)] = true
	}
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		if changed[realPath(path)] {
			out = append(out, path)
		}
	}
	return out
}

func (m *Model) allChangedPaths() []string {
	out := make([]string, 0, len(m.vcs.files))
	for _, file := range m.vcs.files {
		if file.Unstaged != nil && *file.Unstaged != "ignored" || file.Staged != nil {
			out = append(out, file.Path)
		}
	}
	return out
}

func (m *Model) CommitAndPush() {
	if m.commit == nil {
		m.OpenCommit()
		if m.commit != nil {
			m.commit.pushAfter = true
			m.status = "Enter a commit message, then Commit and Push"
		}
		return
	}
	if strings.TrimSpace(m.commit.message) == "" && !m.commit.amend {
		m.commit.err = "Enter a commit message first"
		m.invalidate()
		return
	}
	m.commit.pushAfter = true
	m.submitCommit()
}

func (m *Model) ToggleCommitAmend() {
	if m.commit == nil {
		m.OpenCommit()
	}
	if m.commit != nil {
		m.setCommitAmend(!m.commit.amend)
	}
}

func (m *Model) OpenCommitMessageHistory() {
	if m.commit == nil {
		m.OpenCommit()
	}
	if m.commit == nil {
		return
	}
	history := m.commitMessageHistory(m.commit.repo)
	items := make([]vcsMoreItem, 0, len(history))
	for i, message := range history {
		items = append(items, vcsMoreItem{
			ID: fmt.Sprintf("message-%d", i), Label: message,
			Detail: firstLine(message),
		})
	}
	m.openVCSMore(vcsMoreMessage, "commit.message", "Commit Message History",
		"Choose a previous message for the Commit dialog.")
	m.vcsMore.items = items
}

func (m *Model) OpenCloneDialog(arg any) {
	url, _ := arg.(string)
	m.openVCSMore(vcsMoreClone, "clone", "Get from Version Control",
		"Clone a Git repository into a new project.")
	m.vcsMore.input = url
	if root := m.Root(); root != "" {
		m.vcsMore.input2 = filepath.Join(filepath.Dir(root), filepath.Base(root)+"-clone")
	}
}

func (m *Model) submitClone() {
	state := &m.vcsMore
	url := strings.TrimSpace(state.input)
	dest := strings.TrimSpace(state.input2)
	if url == "" || dest == "" {
		state.err = "Enter the repository URL and destination first"
		m.invalidate()
		return
	}
	if !filepath.IsAbs(dest) && m.Root() != "" {
		dest = filepath.Join(filepath.Dir(m.Root()), dest)
	}
	state.busy = true
	state.err = ""
	state.output = nil
	request, channel := m.nextRequest(), m.nextChannel()
	state.input2 = dest
	state.channel = channel
	state.request = request
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Minute)
		defer cancel()
		_, err := m.mgr.Invoke(ctx, Window, "vcs_clone", map[string]any{
			"request": request, "url": url, "dest": dest, "depth": nil,
			"onOutput": map[string]any{"$channel": channel},
		})
		m.post(func() {
			if m.vcsMore.action != "clone" || m.vcsMore.kind != vcsMoreClone {
				return
			}
			m.vcsMore.busy = false
			if err != nil {
				m.vcsMore.err = "Clone failed: " + err.Error()
				m.invalidate()
				return
			}
			m.status = "Cloned repository to " + dest
			m.closeVCSMore()
			m.OpenProject(dest)
		})
	}()
}

func (m *Model) OpenVCSWorktrees() {
	m.openVCSMore(vcsMoreWorktrees, "worktrees", "Git Worktrees",
		"Worktrees share the repository while keeping separate branches.")
	m.loadVCSWorktrees()
}

func (m *Model) loadVCSWorktrees() {
	state := &m.vcsMore
	state.kind, state.loading, state.err = vcsMoreWorktrees, true, ""
	root := m.Root()
	repoArg := m.vcsRepositoryArg()
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, time.Minute)
		defer cancel()
		raw, err := m.invoke(ctx, "vcs_worktrees", map[string]any{"repo": repoArg})
		var worktrees []openidea.Worktree
		if err == nil {
			err = json.Unmarshal(raw, &worktrees)
		}
		m.post(func() {
			if m.Root() != root || m.vcsMore.kind != vcsMoreWorktrees {
				return
			}
			m.vcsMore.loading = false
			if err != nil {
				m.vcsMore.err = "Could not read worktrees: " + err.Error()
				m.invalidate()
				return
			}
			m.vcsMore.worktrees = worktrees
			m.vcsMore.choice = clampSymbolSel(m.vcsMore.choice, len(worktrees))
			m.invalidate()
		})
	}()
}

func (m *Model) OpenNewVCSWorktree() {
	m.openVCSMore(vcsMorePrompt, vcsMoreActionWorktreeAdd, "New Worktree",
		"Enter a directory, a starting revision and optionally a new branch.")
	m.vcsMore.input = filepath.Join(filepath.Dir(m.Root()), filepath.Base(m.Root())+"-worktree")
	m.vcsMore.input2 = "HEAD"
}

func (m *Model) submitNewWorktree() {
	state := &m.vcsMore
	path := strings.TrimSpace(state.input)
	commitish := strings.TrimSpace(state.input2)
	branch := strings.TrimSpace(state.input3)
	if path == "" || commitish == "" {
		state.err = "Enter the worktree path and starting revision"
		m.invalidate()
		return
	}
	var newBranch any
	if branch != "" {
		newBranch = branch
	}
	repoArg := m.vcsRepositoryArg()
	repoRoot := m.vcs.current
	state.busy, state.err = true, ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 3*time.Minute)
		defer cancel()
		_, err := m.invoke(ctx, "vcs_add_worktree", map[string]any{
			"repo": repoArg, "path": path, "commitish": commitish, "newBranch": newBranch,
		})
		m.post(func() {
			m.vcsMore.busy = false
			if err != nil {
				m.vcsMore.err = "Could not create the worktree: " + err.Error()
				m.invalidate()
				return
			}
			m.status = "Created worktree " + path
			m.refreshVCSWorktrees(repoRoot)
			m.OpenVCSWorktrees()
		})
	}()
}

func (m *Model) selectedVCSWorktree() (openidea.Worktree, bool) {
	if m.vcsMore.choice < 0 || m.vcsMore.choice >= len(m.vcsMore.worktrees) {
		return openidea.Worktree{}, false
	}
	return m.vcsMore.worktrees[m.vcsMore.choice], true
}

func (m *Model) OpenSelectedVCSWorktree() {
	worktree, ok := m.selectedVCSWorktree()
	if !ok {
		return
	}
	m.closeVCSMore()
	m.OpenProject(worktree.Path)
}

func worktreePathKey(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	value = strings.TrimRight(value, "/")
	if len(value) >= 2 && value[1] == ':' {
		value = strings.ToLower(value)
	}
	return value
}

func worktreePathEqual(left, right string) bool {
	return worktreePathKey(left) == worktreePathKey(right)
}

func worktreeContainsProject(worktree, projectRoot string) bool {
	worktree, projectRoot = worktreePathKey(worktree), worktreePathKey(projectRoot)
	if worktree == "" || projectRoot == "" {
		return false
	}
	return projectRoot == worktree || strings.HasPrefix(projectRoot, worktree+"/")
}

func worktreeMoveTarget(worktree, folder string) string {
	key := worktreePathKey(worktree)
	name := path.Base(key)
	target := strings.TrimRight(folder, `/\`)
	if target == "" {
		target = folder
	}
	if path.Base(worktreePathKey(target)) == name {
		return target
	}
	separator := "/"
	if strings.Contains(target, `\`) && !strings.Contains(target, "/") {
		separator = `\`
	}
	return strings.TrimRight(target, `/\`) + separator + name
}

func (m *Model) worktreeCanChange(worktree openidea.Worktree) bool {
	return !worktree.Main && !worktreeContainsProject(worktree.Path, m.Root())
}

func (m *Model) AskRemoveSelectedVCSWorktree() {
	worktree, ok := m.selectedVCSWorktree()
	if !ok || !m.worktreeCanChange(worktree) {
		return
	}
	m.vcsMore.kind = vcsMoreConfirm
	m.vcsMore.action = "worktree.remove"
	m.vcsMore.title = "Remove Worktree"
	m.vcsMore.hint = "Remove " + worktree.Path + "? Its uncommitted changes can be lost."
	m.vcsMore.input = worktree.Path
	m.invalidate()
}

func (m *Model) removeVCSWorktree(path string) {
	repoArg := m.vcsRepositoryArg()
	repoRoot := m.vcs.current
	m.vcsMore.busy = true
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
		defer cancel()
		_, err := m.invoke(ctx, "vcs_remove_worktree", map[string]any{
			"repo": repoArg, "path": path, "force": false,
		})
		m.post(func() {
			if err != nil {
				m.vcsMore.busy = false
				m.vcsMore.kind = vcsMoreWorktrees
				m.vcsMore.err = "Could not remove the worktree: " + err.Error()
				m.invalidate()
				return
			}
			m.status = "Removed worktree " + path
			m.refreshVCSWorktrees(repoRoot)
			m.OpenVCSWorktrees()
		})
	}()
}

func (m *Model) ChooseMoveVCSWorktree() {
	worktree, ok := m.selectedVCSWorktree()
	if !ok || !m.worktreeCanChange(worktree) {
		return
	}
	parent := m.win
	start := filepath.Dir(worktree.Path)
	go func() {
		picked, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:            parent,
			Title:             "Move Worktree",
			ButtonLabel:       "Choose",
			DefaultPath:       start,
			Directory:         true,
			CreateDirectories: true,
		})
		if err != nil || len(picked) == 0 {
			return
		}
		m.post(func() {
			current, exists := m.selectedVCSWorktree()
			if !m.vcsMore.open || !exists || !worktreePathEqual(current.Path, worktree.Path) ||
				!m.worktreeCanChange(current) {
				return
			}
			target := worktreeMoveTarget(current.Path, picked[0])
			if worktreePathEqual(target, current.Path) {
				return
			}
			m.vcsMore.kind = vcsMoreConfirm
			m.vcsMore.action = vcsMoreActionWorktreeMove
			m.vcsMore.title = "Move Worktree"
			m.vcsMore.hint = "Move the worktree at " + current.Path + " to " + target + "?"
			m.vcsMore.input2, m.vcsMore.input3 = current.Path, target
			m.vcsMore.err = ""
			m.invalidate()
		})
	}()
}

func (m *Model) moveVCSWorktree(from, to string) {
	repoArg := m.vcsRepositoryArg()
	repoRoot := m.vcs.current
	m.vcsMore.busy, m.vcsMore.err = true, ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
		defer cancel()
		_, err := m.invoke(ctx, "vcs_move_worktree", map[string]any{
			"repo": repoArg, "path": from, "to": to,
		})
		m.post(func() {
			m.vcsMore.busy = false
			if err != nil {
				m.vcsMore.kind = vcsMoreWorktrees
				m.vcsMore.err = "Could not move the worktree: " + err.Error()
				m.invalidate()
				return
			}
			m.status = "Moved worktree to " + to
			m.refreshVCSWorktrees(repoRoot)
			m.OpenVCSWorktrees()
		})
	}()
}

func (m *Model) OpenShelveChanges(arg any) {
	paths := m.vcsActionPaths(arg, true)
	if len(paths) == 0 {
		paths = m.allChangedPaths()
	}
	m.openVCSMore(vcsMorePrompt, vcsMoreActionShelve, "Shelve Changes",
		"Save the selected changes on the shelf and revert them.")
	m.vcsMore.input = fmt.Sprintf("Changes on %s", time.Now().Format("Jan 2, 15:04"))
	m.vcsMore.items = []vcsMoreItem{{Label: "Files", Detail: count(len(paths), "file", "files")}}
	m.vcsMore.input2 = strings.Join(paths, "\x00")
}

func (m *Model) submitShelve() {
	state := &m.vcsMore
	name := strings.TrimSpace(state.input)
	paths := strings.Split(state.input2, "\x00")
	if name == "" || len(paths) == 0 || paths[0] == "" {
		state.err = "Enter a name and select at least one changed file"
		m.invalidate()
		return
	}
	state.busy, state.err = true, ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 5*time.Minute)
		defer cancel()
		_, err := m.mgr.Invoke(ctx, Window, "vcs_shelve", map[string]any{
			"name": name, "paths": paths,
		})
		m.post(func() {
			m.vcsMore.busy = false
			if err != nil {
				m.vcsMore.err = "Could not shelve the changes: " + err.Error()
				m.invalidate()
				return
			}
			m.status = "Shelved the selected changes"
			m.RefreshAfterVCSCommand()
			m.OpenVCSShelf()
		})
	}()
}

func (m *Model) OpenVCSShelf() {
	m.openVCSMore(vcsMoreShelf, "shelf", "Shelf",
		"Shelved changes can be inspected, applied or saved as patches.")
	m.LoadVCSShelf()
}

func (m *Model) LoadVCSShelf() {
	if !m.vcsMore.open {
		m.vcsMore = vcsMoreState{
			open: true, kind: vcsMoreShelf, action: "shelf", title: "Shelf",
			hint: "Shelved changes can be inspected, applied or saved as patches.",
		}
	}
	state := &m.vcsMore
	state.kind, state.loading, state.err = vcsMoreShelf, true, ""
	root := m.Root()
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, time.Minute)
		defer cancel()
		raw, err := m.mgr.Invoke(ctx, Window, "vcs_shelves", nil)
		var shelves []shelvedChangeList
		if err == nil {
			err = json.Unmarshal(raw, &shelves)
		}
		m.post(func() {
			if m.Root() != root || m.vcsMore.kind != vcsMoreShelf {
				return
			}
			m.vcsMore.loading = false
			if err != nil {
				m.vcsMore.err = "Could not read the shelf: " + err.Error()
				m.invalidate()
				return
			}
			m.vcsMore.shelves = shelves
			m.vcsMore.choice = clampSymbolSel(m.vcsMore.choice, len(shelves))
			m.invalidate()
		})
	}()
}

func (m *Model) selectedShelf(arg any) (shelvedChangeList, string, bool) {
	if id := shelfTargetID(arg); id != "" {
		for _, shelf := range m.vcsMore.shelves {
			if shelf.ID == id {
				return shelf, shelfTargetPath(arg), true
			}
		}
		if m.vcs.shelf != nil {
			for _, shelf := range m.vcs.shelf.shelves {
				if shelf.ID == id {
					return shelf, shelfTargetPath(arg), true
				}
			}
		}
		return shelvedChangeList{ID: id}, shelfTargetPath(arg), true
	}
	if m.vcsMore.open && m.vcsMore.choice >= 0 && m.vcsMore.choice < len(m.vcsMore.shelves) {
		return m.vcsMore.shelves[m.vcsMore.choice], "", true
	}
	if m.vcs.shelf != nil {
		if shelf, ok := m.selectedVCSShelf(); ok {
			return shelf, "", true
		}
	}
	return shelvedChangeList{}, "", false
}

func shelfTargetID(arg any) string {
	switch value := arg.(type) {
	case string:
		return value
	case map[string]any:
		if id, ok := value["id"].(string); ok {
			return id
		}
		if entry, ok := value["entry"].(map[string]any); ok {
			if id, ok := entry["id"].(string); ok {
				return id
			}
		}
	}
	return ""
}

func shelfTargetPath(arg any) string {
	if value, ok := arg.(map[string]any); ok {
		if path, ok := value["path"].(string); ok {
			return path
		}
	}
	return ""
}

func (m *Model) UnshelveArg(arg any) {
	shelf, path, ok := m.selectedShelf(arg)
	if !ok {
		m.OpenVCSShelf()
		return
	}
	m.unshelve(shelf.ID, path, m.vcsShelfRemoveApplied())
}

func (m *Model) unshelve(id, path string, remove bool) {
	if id == "" {
		return
	}
	m.vcsMore.busy, m.vcsMore.err = true, ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 5*time.Minute)
		defer cancel()
		var pathArg any
		if path != "" {
			pathArg = path
		}
		raw, err := m.mgr.Invoke(ctx, Window, "vcs_unshelve", map[string]any{
			"id": id, "remove": remove, "path": pathArg,
		})
		var outcome patchApplyOutcome
		if err == nil {
			err = json.Unmarshal(raw, &outcome)
		}
		m.post(func() {
			m.vcsMore.busy = false
			if err != nil {
				m.vcsMore.err = "Unshelve failed: " + err.Error()
				m.invalidate()
				return
			}
			m.status = fmt.Sprintf("Unshelved %d files", len(outcome.Applied))
			if len(outcome.Conflicts) > 0 {
				m.status = fmt.Sprintf("Unshelved with %d conflicts", len(outcome.Conflicts))
			}
			m.RefreshAfterVCSCommand()
			m.finishVCSShelfMutation()
		})
	}()
}

func (m *Model) ShelfDeleteArg(arg any) {
	shelf, path, ok := m.selectedShelf(arg)
	if !ok {
		return
	}
	m.vcsMore.kind = vcsMoreConfirm
	m.vcsMore.action = "shelf.delete"
	m.vcsMore.title = "Delete Shelved Changes"
	m.vcsMore.hint = "Delete “" + shelf.Name + "” from the shelf?"
	m.vcsMore.input = shelf.ID
	m.vcsMore.input2 = path
	m.invalidate()
}

func (m *Model) deleteShelf(id, path string) {
	m.vcsMore.busy = true
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, time.Minute)
		defer cancel()
		var pathArg any
		if path != "" {
			pathArg = path
		}
		_, err := m.mgr.Invoke(ctx, Window, "vcs_shelf_delete", map[string]any{
			"id": id, "path": pathArg,
		})
		m.post(func() {
			if err != nil {
				m.vcsMore.busy = false
				m.vcsMore.err = "Could not delete the shelf: " + err.Error()
				m.invalidate()
				return
			}
			m.status = "Deleted shelved changes"
			m.finishVCSShelfMutation()
		})
	}()
}

func (m *Model) ShelfRenameArg(arg any) {
	shelf, path, ok := m.selectedShelf(arg)
	if !ok || path != "" {
		return
	}
	m.vcsMore.kind = vcsMorePrompt
	m.vcsMore.action = vcsMoreActionShelfRename
	m.vcsMore.title = "Rename Shelved Changes"
	m.vcsMore.hint = "Enter a new shelf name."
	m.vcsMore.input = shelf.Name
	m.vcsMore.input2 = shelf.ID
	m.invalidate()
}

func (m *Model) submitShelfRename() {
	name := strings.TrimSpace(m.vcsMore.input)
	id := m.vcsMore.input2
	if name == "" || id == "" {
		m.vcsMore.err = "Enter a shelf name"
		m.invalidate()
		return
	}
	m.vcsMore.busy = true
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, time.Minute)
		defer cancel()
		_, err := m.mgr.Invoke(ctx, Window, "vcs_shelf_rename", map[string]any{
			"id": id, "name": name,
		})
		m.post(func() {
			if err != nil {
				m.vcsMore.busy = false
				m.vcsMore.err = "Could not rename the shelf: " + err.Error()
				m.invalidate()
				return
			}
			m.status = "Renamed shelved changes"
			m.finishVCSShelfMutation()
		})
	}()
}

func (m *Model) ShowShelfDiffArg(arg any, compareLocal bool) {
	shelf, path, ok := m.selectedShelf(arg)
	if !ok {
		m.OpenVCSShelf()
		return
	}
	m.vcsMore.loading, m.vcsMore.err = true, ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, time.Minute)
		defer cancel()
		raw, err := m.mgr.Invoke(ctx, Window, "vcs_shelf_diff", map[string]any{"id": shelf.ID})
		var files []shelfDiffFile
		if err == nil {
			err = json.Unmarshal(raw, &files)
		}
		m.post(func() {
			m.vcsMore.loading = false
			if err != nil {
				m.vcsMore.err = "Could not read the shelved diff: " + err.Error()
				m.vcsMore.kind = vcsMoreShelf
				m.invalidate()
				return
			}
			if path != "" {
				for _, file := range files {
					if file.Path == path || filepath.Base(file.Path) == filepath.Base(path) {
						m.showShelfFileDiff(file, compareLocal)
						return
					}
				}
			}
			items := make([]vcsMoreItem, 0, len(files))
			for _, file := range files {
				before, after := "", ""
				if file.Before != nil {
					before = *file.Before
				}
				if file.After != nil {
					after = *file.After
				}
				items = append(items, vcsMoreItem{
					Path: file.Path, Label: filepath.Base(file.Path), Detail: shelf.Name,
					Before: before, After: after, Kind: "shelf",
				})
			}
			m.vcsMore.kind = vcsMoreChanges
			m.vcsMore.title = "Shelved Changes · " + shelf.Name
			m.vcsMore.hint = "Choose a file to compare its shelved and current versions."
			m.vcsMore.items = items
			m.vcsMore.choice = 0
			m.invalidate()
		})
	}()
}

func (m *Model) showShelfFileDiff(file shelfDiffFile, compareLocal bool) {
	left := ""
	if file.Before != nil {
		left = *file.Before
	}
	right := ""
	if compareLocal {
		if t := m.tab(); t != nil && t.doc != nil && realPath(t.doc.Path()) == realPath(file.Path) {
			right = t.doc.Text()
		} else if data, err := os.ReadFile(file.Path); err == nil {
			right = string(data)
		}
	} else if file.After != nil {
		right = *file.After
	}
	m.closeVCSMore()
	m.CompareTextsArg(map[string]any{
		"path":  file.Path,
		"left":  map[string]any{"text": left, "label": "Shelved"},
		"right": map[string]any{"text": right, "label": map[bool]string{true: "Local", false: "After Unshelve"}[compareLocal]},
	})
}

func (m *Model) CreateShelfPatchArg(arg any) {
	shelf, path, ok := m.selectedShelf(arg)
	if !ok {
		return
	}
	m.vcsMore.busy = true
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, time.Minute)
		defer cancel()
		var pathArg any
		if path != "" {
			pathArg = path
		}
		raw, err := m.mgr.Invoke(ctx, Window, "vcs_shelf_patch", map[string]any{
			"id": shelf.ID, "path": pathArg,
		})
		var patch string
		if err == nil {
			err = json.Unmarshal(raw, &patch)
		}
		m.post(func() {
			m.vcsMore.busy = false
			if err != nil {
				m.vcsMore.err = "Could not create the patch: " + err.Error()
				m.invalidate()
				return
			}
			m.savePatch(patch, shelf.Name)
		})
	}()
}

func (m *Model) CopyShelfPatchArg(arg any) {
	shelf, path, ok := m.selectedShelf(arg)
	if !ok {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, time.Minute)
		defer cancel()
		var pathArg any
		if path != "" {
			pathArg = path
		}
		raw, err := m.mgr.Invoke(ctx, Window, "vcs_shelf_patch", map[string]any{
			"id": shelf.ID, "path": pathArg,
		})
		var patch string
		if err == nil {
			err = json.Unmarshal(raw, &patch)
		}
		m.post(func() {
			if err != nil {
				m.status = "Could not copy the patch: " + err.Error()
			} else {
				mygo.Clipboard.WriteText(patch)
				m.status = "Copied the shelved patch"
			}
			m.invalidate()
		})
	}()
}

func (m *Model) CreatePatchForPaths(arg any) {
	paths := m.vcsActionPaths(arg, true)
	if len(paths) == 0 {
		paths = m.allChangedPaths()
	}
	if len(paths) == 0 {
		m.status = "There are no changes to include in a patch"
		m.invalidate()
		return
	}
	repoArg := m.vcsRepositoryArgForPath(paths[0])
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, time.Minute)
		defer cancel()
		raw, err := m.invoke(ctx, "vcs_create_patch", map[string]any{
			"repo": repoArg,
			"source": map[string]any{
				"from": "changes", "paths": paths, "reverse": false,
			},
		})
		var patch string
		if err == nil {
			err = json.Unmarshal(raw, &patch)
		}
		m.post(func() {
			if err != nil {
				m.status = "Could not create the patch: " + err.Error()
				m.invalidate()
				return
			}
			m.savePatch(patch, filepath.Base(m.Root())+".patch")
		})
	}()
}

func (m *Model) savePatch(patch, suggested string) {
	name := strings.TrimSpace(suggested)
	name = strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			return '-'
		}
		return r
	}, name)
	if name == "" {
		name = "changes"
	}
	if !strings.HasSuffix(strings.ToLower(name), ".patch") {
		name += ".patch"
	}
	parent := m.win
	start := filepath.Join(m.Root(), name)
	go func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent: parent, Title: "Save Patch", ButtonLabel: "Save",
			DefaultPath: start, CreateDirectories: true,
		})
		if err != nil || path == "" {
			return
		}
		err = os.WriteFile(path, []byte(patch), 0o644)
		m.post(func() {
			if err != nil {
				m.status = "Could not save the patch: " + err.Error()
			} else {
				m.status = "Saved patch " + path
				m.afterPatchSaved(path)
			}
			m.invalidate()
		})
	}()
}

func (m *Model) OpenApplyPatch(arg any) {
	path, _ := arg.(string)
	if path == "" {
		parent := m.win
		start := m.Root()
		go func() {
			paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
				Parent: parent, Title: "Apply Patch", ButtonLabel: "Open",
				DefaultPath: start,
				Filters: []mygo.FileFilter{
					{Name: "Patch files", Extensions: []string{"patch", "diff"}},
					{Name: "All files", Extensions: []string{"*"}},
				},
			})
			if err != nil || len(paths) == 0 {
				return
			}
			m.post(func() { m.OpenApplyPatch(paths[0]) })
		}()
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		m.status = "Could not read the patch: " + err.Error()
		m.invalidate()
		return
	}
	m.previewPatch(string(data), filepath.Base(path))
}

func (m *Model) OpenApplyPatchFromClipboard() {
	data, err := mygo.Clipboard.Read(transfer.Text)
	var raw []byte
	if err == nil {
		raw, err = data.Read(transfer.Text)
	}
	if err != nil {
		m.status = "The clipboard has no patch text"
		m.invalidate()
		return
	}
	m.previewPatch(string(raw), "Clipboard")
}

func (m *Model) previewPatch(patch, name string) {
	m.openVCSMore(vcsMorePatchPreview, "apply.patch", "Apply Patch",
		"Review the files before applying "+name+".")
	m.vcsMore.loading = true
	m.vcsMore.patch = patch
	repoArg := m.vcsRepositoryArg()
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, time.Minute)
		defer cancel()
		raw, err := m.invoke(ctx, "vcs_patch_preview", map[string]any{
			"repo": repoArg, "patch": patch,
		})
		var files []patchPreviewFile
		if err == nil {
			err = json.Unmarshal(raw, &files)
		}
		m.post(func() {
			m.vcsMore.loading = false
			if err != nil {
				m.vcsMore.err = "Could not preview the patch: " + err.Error()
				m.invalidate()
				return
			}
			items := make([]vcsMoreItem, 0, len(files))
			for _, file := range files {
				status := "clean"
				if !file.Applies {
					status = "3-way merge"
				}
				if file.Binary {
					status += ", binary"
				}
				items = append(items, vcsMoreItem{
					Path: file.Path, Label: filepath.Base(file.Path),
					Detail: fmt.Sprintf("+%d −%d · %s", file.Added, file.Removed, status),
					Kind:   file.Kind,
				})
			}
			m.vcsMore.items = items
			m.invalidate()
		})
	}()
}

func (m *Model) applyPatch() {
	patch := m.vcsMore.patch
	if patch == "" {
		return
	}
	m.vcsMore.busy, m.vcsMore.err = true, ""
	repoArg := m.vcsRepositoryArg()
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 5*time.Minute)
		defer cancel()
		raw, err := m.invoke(ctx, "vcs_apply_patch", map[string]any{
			"repo": repoArg, "patch": patch,
		})
		var outcome patchApplyOutcome
		if err == nil {
			err = json.Unmarshal(raw, &outcome)
		}
		m.post(func() {
			m.vcsMore.busy = false
			if err != nil {
				m.vcsMore.err = "Could not apply the patch: " + err.Error()
				m.invalidate()
				return
			}
			m.status = fmt.Sprintf("Applied %d files", len(outcome.Applied))
			if len(outcome.Conflicts) > 0 {
				m.status = fmt.Sprintf("Applied with %d conflicts", len(outcome.Conflicts))
			}
			m.closeVCSMore()
			m.RefreshAfterVCSCommand()
		})
	}()
}

func (m *Model) ImportShelfPatches(arg any) {
	paths := actionPaths(arg)
	if len(paths) == 0 {
		parent := m.win
		go func() {
			picked, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
				Parent: parent, Title: "Import Patches", ButtonLabel: "Import",
				DefaultPath: m.Root(), Multiple: true,
				Filters: []mygo.FileFilter{
					{Name: "Patch files", Extensions: []string{"patch", "diff"}},
					{Name: "All files", Extensions: []string{"*"}},
				},
			})
			if err != nil || len(picked) == 0 {
				return
			}
			m.post(func() { m.ImportShelfPatches(picked) })
		}()
		return
	}
	repoArg := m.vcsRepositoryArg()
	m.vcsMore.busy = true
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 5*time.Minute)
		defer cancel()
		var err error
		for _, path := range paths {
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				err = readErr
				break
			}
			_, err = m.invoke(ctx, "vcs_shelf_import", map[string]any{
				"repo": repoArg, "name": strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
				"patch": string(data),
			})
			if err != nil {
				break
			}
		}
		m.post(func() {
			if err != nil {
				m.vcsMore.busy = false
				m.vcsMore.err = "Could not import the patches: " + err.Error()
				m.invalidate()
				return
			}
			m.status = "Imported " + count(len(paths), "patch", "patches")
			m.finishVCSShelfMutation()
		})
	}()
}

func (m *Model) loadChangelists(open bool) {
	m.loadChangelistsThen(open, nil)
}

func (m *Model) loadChangelistsThen(open bool, loaded func()) {
	if open {
		m.openVCSMore(vcsMoreChangelists, "changelists", "Changelists",
			"Group changed files and commit one list at a time.")
	}
	state := &m.vcsMore
	state.kind, state.loading, state.err = vcsMoreChangelists, true, ""
	request := m.nextRequest()
	state.loadID = request
	root := m.Root()
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, time.Minute)
		defer cancel()
		raw, err := m.mgr.Invoke(ctx, Window, "vcs_changelists_load", nil)
		var lists changelistState
		if err == nil {
			err = json.Unmarshal(raw, &lists)
		}
		m.post(func() {
			if m.Root() != root || m.vcsMore.loadID != request {
				return
			}
			m.vcsMore.loading = false
			if err != nil {
				m.vcsMore.err = "Could not read changelists: " + err.Error()
				m.invalidate()
				return
			}
			m.vcsMore.lists = lists
			m.vcsMore.changelistsLoaded = true
			m.vcsMore.choice = clampSymbolSel(m.vcsMore.choice, len(lists.Lists))
			if loaded != nil {
				loaded()
			}
			m.invalidate()
		})
	}()
}

func (m *Model) OpenNewChangelist() {
	openNew := func() {
		m.vcsMore.kind = vcsMorePrompt
		m.vcsMore.action = vcsMoreActionChangelistNew
		m.vcsMore.title = "New Changelist"
		m.vcsMore.hint = "Name a list that can receive changed files."
		m.vcsMore.input = ""
		m.vcsMore.input2 = ""
		m.vcsMore.input3 = ""
		m.vcsMore.err = ""
		m.invalidate()
	}
	if m.vcsMore.changelistsLoaded {
		openNew()
		return
	}
	m.openVCSMore(vcsMoreChangelists, "changelists", "Changelists",
		"Group changed files and commit one list at a time.")
	m.loadChangelistsThen(false, openNew)
}

func (m *Model) selectedChangelist(arg any) (changelistEntry, bool) {
	if id, ok := arg.(string); ok && id != "" {
		for _, list := range m.vcsMore.lists.Lists {
			if list.ID == id {
				return list, true
			}
		}
		return changelistEntry{ID: id, Name: id}, true
	}
	if m.vcsMore.choice >= 0 && m.vcsMore.choice < len(m.vcsMore.lists.Lists) {
		return m.vcsMore.lists.Lists[m.vcsMore.choice], true
	}
	for _, list := range m.vcsMore.lists.Lists {
		if list.ID == m.vcsMore.lists.Active {
			return list, true
		}
	}
	return changelistEntry{}, false
}

func (m *Model) OpenEditChangelist(arg any) {
	openEdit := func() {
		list, ok := m.selectedChangelist(arg)
		if !ok {
			m.vcsMore.err = "Choose a changelist to edit"
			m.invalidate()
			return
		}
		m.vcsMore.kind = vcsMorePrompt
		m.vcsMore.action = vcsMoreActionChangelistEdit
		m.vcsMore.title = "Edit Changelist"
		m.vcsMore.hint = "Change the list name and comment."
		m.vcsMore.input = list.Name
		m.vcsMore.input2 = list.Comment
		m.vcsMore.input3 = list.ID
		m.invalidate()
	}
	if m.vcsMore.changelistsLoaded {
		openEdit()
		return
	}
	m.openVCSMore(vcsMoreChangelists, "changelists", "Changelists",
		"Group changed files and commit one list at a time.")
	m.loadChangelistsThen(false, openEdit)
}

func (m *Model) saveChangelists(next changelistState, close bool) {
	state := &m.vcsMore
	state.busy, state.err = true, ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, time.Minute)
		defer cancel()
		_, err := m.mgr.Invoke(ctx, Window, "vcs_changelists_save", map[string]any{
			"changelists": next,
		})
		m.post(func() {
			state.busy = false
			if err != nil {
				state.err = "Could not save changelists: " + err.Error()
				m.invalidate()
				return
			}
			state.lists = next
			state.changelistsLoaded = true
			state.choice = clampSymbolSel(state.choice, len(next.Lists))
			if close {
				m.closeVCSMore()
			} else {
				state.kind = vcsMoreChangelists
				state.loading = false
				m.invalidate()
			}
		})
	}()
}

func (m *Model) submitNewChangelist() {
	name := strings.TrimSpace(m.vcsMore.input)
	if name == "" {
		m.vcsMore.err = "Enter a changelist name"
		m.invalidate()
		return
	}
	next := m.vcsMore.lists
	id := "list-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	next.Lists = append(next.Lists, changelistEntry{ID: id, Name: name})
	if next.Active == "" {
		next.Active = id
	}
	if next.Files == nil {
		next.Files = map[string]string{}
	}
	m.saveChangelists(next, false)
}

func (m *Model) submitEditChangelist() {
	id := m.vcsMore.input3
	name := strings.TrimSpace(m.vcsMore.input)
	if id == "" || name == "" {
		m.vcsMore.err = "Enter a changelist name"
		m.invalidate()
		return
	}
	next := m.vcsMore.lists
	for i := range next.Lists {
		if next.Lists[i].ID == id {
			next.Lists[i].Name = name
			next.Lists[i].Comment = strings.TrimSpace(m.vcsMore.input2)
			break
		}
	}
	m.saveChangelists(next, false)
}

func (m *Model) DeleteChangelistArg(arg any) {
	ask := func() {
		list, ok := m.selectedChangelist(arg)
		if !ok || list.ID == "" || list.ID == m.vcsMore.lists.Active {
			m.status = "Select a non-active changelist to delete"
			m.invalidate()
			return
		}
		m.vcsMore.kind = vcsMoreConfirm
		m.vcsMore.action = "changelist.delete"
		m.vcsMore.title = "Delete Changelist"
		m.vcsMore.hint = "Delete “" + list.Name + "”? Its files move to the active list."
		m.vcsMore.input = list.ID
		m.invalidate()
	}
	if m.vcsMore.changelistsLoaded {
		ask()
		return
	}
	m.openVCSMore(vcsMoreChangelists, "changelists", "Changelists",
		"Group changed files and commit one list at a time.")
	m.loadChangelistsThen(false, ask)
}

func (m *Model) deleteChangelist(id string) {
	next := m.vcsMore.lists
	filtered := next.Lists[:0]
	for _, list := range next.Lists {
		if list.ID != id {
			filtered = append(filtered, list)
		}
	}
	next.Lists = filtered
	for path, list := range next.Files {
		if list == id {
			delete(next.Files, path)
		}
	}
	m.saveChangelists(next, false)
}

func (m *Model) SetActiveChangelistArg(arg any) {
	setActive := func() {
		list, ok := m.selectedChangelist(arg)
		if !ok {
			return
		}
		next := m.vcsMore.lists
		next.Active = list.ID
		m.saveChangelists(next, false)
	}
	if m.vcsMore.changelistsLoaded {
		setActive()
		return
	}
	m.openVCSMore(vcsMoreChangelists, "changelists", "Changelists",
		"Group changed files and commit one list at a time.")
	m.loadChangelistsThen(false, setActive)
}

func (m *Model) MoveToChangelist(arg any) {
	paths := m.vcsActionPaths(arg, true)
	if len(paths) == 0 {
		m.status = "Select changed files to move"
		m.invalidate()
		return
	}
	openMove := func() {
		m.vcsMore.kind = vcsMoreChangelists
		m.vcsMore.action = "changelist.move"
		m.vcsMore.hint = "Choose the target changelist for the selected files."
		m.vcsMore.input = strings.Join(paths, "\x00")
		m.invalidate()
	}
	if m.vcsMore.changelistsLoaded {
		openMove()
		return
	}
	m.openVCSMore(vcsMoreChangelists, "changelists", "Changelists",
		"Group changed files and commit one list at a time.")
	m.loadChangelistsThen(false, openMove)
}

func (m *Model) movePathsToChangelist(listID string) {
	paths := strings.Split(m.vcsMore.input, "\x00")
	next := m.vcsMore.lists
	if next.Files == nil {
		next.Files = map[string]string{}
	}
	for _, path := range paths {
		if path == "" {
			continue
		}
		if listID == next.Active {
			delete(next.Files, path)
		} else {
			next.Files[path] = listID
		}
	}
	m.saveChangelists(next, false)
}

func (m *Model) CommitChangelistArg(arg any) {
	openCommit := func() {
		list, ok := m.selectedChangelist(arg)
		if !ok {
			return
		}
		paths := map[string]bool{}
		for _, file := range m.vcs.files {
			listID := m.vcsMore.lists.Files[file.Path]
			if listID == "" {
				listID = m.vcsMore.lists.Active
			}
			if listID == list.ID {
				paths[file.Path] = true
			}
		}
		m.closeVCSMore()
		m.OpenCommit()
		if m.commit != nil {
			m.commit.onlyPaths = paths
			m.status = "Committing changelist " + list.Name
			m.invalidate()
		}
	}
	if m.vcsMore.changelistsLoaded {
		openCommit()
		return
	}
	m.openVCSMore(vcsMoreChangelists, "changelists", "Changelists",
		"Group changed files and commit one list at a time.")
	m.loadChangelistsThen(false, openCommit)
}

type conflictBlock struct {
	from        int
	to          int
	line        int
	raw         string
	ours        string
	base        string
	theirs      string
	oursLabel   string
	theirsLabel string
	hasBase     bool
}

func parseConflictMarkers(text string) []conflictBlock {
	if !strings.Contains(text, "<<<<<<<") {
		return nil
	}
	type line struct {
		from int
		to   int
		text string
	}
	var lines []line
	for pos := 0; pos < len(text); {
		next := strings.IndexByte(text[pos:], '\n')
		end := len(text)
		if next >= 0 {
			end = pos + next + 1
		}
		lines = append(lines, line{from: pos, to: end, text: text[pos:end]})
		pos = end
	}
	isMarker := func(value, marker string) bool {
		value = strings.TrimSuffix(strings.TrimSuffix(value, "\n"), "\r")
		return value == marker || strings.HasPrefix(value, marker+" ")
	}
	var blocks []conflictBlock
	for i := 0; i < len(lines); i++ {
		if !isMarker(lines[i].text, "<<<<<<<") {
			continue
		}
		baseAt, sepAt, endAt := -1, -1, -1
		for j := i + 1; j < len(lines); j++ {
			value := lines[j].text
			switch {
			case isMarker(value, "<<<<<<<"):
				j = len(lines)
			case sepAt < 0 && baseAt < 0 && isMarker(value, "|||||||"):
				baseAt = j
			case sepAt < 0 && isMarker(value, "======="):
				sepAt = j
			case sepAt >= 0 && isMarker(value, ">>>>>>>"):
				endAt = j
				j = len(lines)
			}
		}
		if sepAt < 0 || endAt < 0 {
			continue
		}
		slice := func(start, end int) string {
			if start >= end {
				return ""
			}
			return text[lines[start].from:lines[end-1].to]
		}
		oursEnd := sepAt
		if baseAt >= 0 {
			oursEnd = baseAt
		}
		blocks = append(blocks, conflictBlock{
			from: lines[i].from, to: lines[endAt].to, line: i,
			raw:         text[lines[i].from:lines[endAt].to],
			ours:        slice(i+1, oursEnd),
			base:        slice(baseAt+1, sepAt),
			theirs:      slice(sepAt+1, endAt),
			oursLabel:   markerLabel(lines[i].text, "<<<<<<<"),
			theirsLabel: markerLabel(lines[endAt].text, ">>>>>>>"),
			hasBase:     baseAt >= 0,
		})
		i = endAt
	}
	return blocks
}

func markerLabel(line, marker string) string {
	line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
	return strings.TrimSpace(strings.TrimPrefix(line, marker))
}

func (m *Model) OpenResolveConflicts() {
	m.RefreshVCS()
	m.openVCSMore(vcsMoreConflicts, "conflicts", "Resolve Conflicts",
		"Choose a conflicted file, then accept one side for every marker.")
	for _, file := range m.vcs.files {
		if effectiveVCSKind(file) == "conflicted" {
			m.vcsMore.items = append(m.vcsMore.items, vcsMoreItem{
				Path: file.Path, Label: filepath.Base(file.Path), Detail: file.RelPath,
			})
		}
	}
	m.vcsMore.choice = 0
}

func (m *Model) selectedConflictPath() string {
	if m.vcsMore.choice < 0 || m.vcsMore.choice >= len(m.vcsMore.items) {
		if t := m.tab(); t != nil && t.doc != nil {
			return t.doc.Path()
		}
		return ""
	}
	return m.vcsMore.items[m.vcsMore.choice].Path
}

func (m *Model) AcceptConflictAtCaret(how string) {
	t := m.tab()
	if t == nil || t.doc == nil {
		m.status = "Open a conflicted file first"
		m.invalidate()
		return
	}
	blocks := parseConflictMarkers(t.doc.Text())
	if len(blocks) == 0 {
		m.status = "There is no conflict marker at the caret"
		m.invalidate()
		return
	}
	caret := t.state.Cursor()
	block := blocks[0]
	for _, candidate := range blocks {
		if candidate.from <= caret && caret < max(candidate.to, candidate.from+1) {
			block = candidate
			break
		}
	}
	m.resolveConflictBlock(t, block, how)
}

func (m *Model) resolveConflictBlock(t *Tab, block conflictBlock, how string) {
	path := t.doc.Path()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.mgr.Invoke(ctx, Window, "vcs_conflict_versions", map[string]any{"path": path})
		var versions mergeVersions
		if err == nil {
			err = json.Unmarshal(raw, &versions)
		}
		m.post(func() {
			if err != nil {
				m.status = "Could not read the conflict versions: " + err.Error()
				m.invalidate()
				return
			}
			ours, theirs := block.ours, block.theirs
			if versions.Sides.Reversed {
				ours, theirs = theirs, ours
			}
			replacement := ""
			switch how {
			case "yours", "ours":
				replacement = ours
			case "theirs":
				replacement = theirs
			case "base":
				replacement = block.base
			case "both":
				replacement = ours
				if replacement != "" && !strings.HasSuffix(replacement, "\n") {
					replacement += "\n"
				}
				replacement += theirs
			}
			current := t.doc.Text()
			if block.to < block.from || block.to > len(current) ||
				block.raw != "" && current[block.from:block.to] != block.raw {
				m.status = "The conflict changed; choose it again"
				m.invalidate()
				return
			}
			editCtx, editCancel := context.WithTimeout(m.ctx, 30*time.Second)
			defer editCancel()
			if err := t.doc.ApplyEdits(editCtx, editor.Edit{
				Start: block.from, End: block.to, Text: replacement,
			}); err != nil {
				m.status = "Could not resolve the conflict: " + err.Error()
				m.invalidate()
				return
			}
			m.status = "Accepted " + how + " conflict side"
			m.invalidate()
		})
	}()
}

func (m *Model) resolveAllConflicts(path, how string) {
	if path == "" {
		path = m.selectedConflictPath()
	}
	if path == "" {
		m.status = "Choose a conflicted file"
		m.invalidate()
		return
	}
	m.closeVCSMore()
	m.openFileThen(path, func(t *Tab) {
		blocks := parseConflictMarkers(t.doc.Text())
		if len(blocks) == 0 {
			m.status = "This file has no conflict markers"
			m.invalidate()
			return
		}
		m.resolveAllConflictBlocks(t, blocks, how)
	})
}

func (m *Model) resolveAllConflictBlocks(t *Tab, blocks []conflictBlock, how string) {
	path := t.doc.Path()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, time.Minute)
		defer cancel()
		raw, err := m.mgr.Invoke(ctx, Window, "vcs_conflict_versions", map[string]any{"path": path})
		var versions mergeVersions
		if err == nil {
			err = json.Unmarshal(raw, &versions)
		}
		m.post(func() {
			if err != nil {
				m.status = "Could not read the conflict versions: " + err.Error()
				m.invalidate()
				return
			}
			text := t.doc.Text()
			for _, block := range blocks {
				if block.to < block.from || block.to > len(text) ||
					block.raw != "" && text[block.from:block.to] != block.raw {
					m.status = "The conflicts changed; refresh them first"
					m.invalidate()
					return
				}
			}
			var out strings.Builder
			pos := 0
			for _, block := range blocks {
				out.WriteString(text[pos:block.from])
				ours, theirs := block.ours, block.theirs
				if versions.Sides.Reversed {
					ours, theirs = theirs, ours
				}
				switch how {
				case "yours", "ours":
					out.WriteString(ours)
				case "theirs":
					out.WriteString(theirs)
				case "both":
					out.WriteString(ours)
					if ours != "" && !strings.HasSuffix(ours, "\n") {
						out.WriteByte('\n')
					}
					out.WriteString(theirs)
				}
				pos = block.to
			}
			out.WriteString(text[pos:])
			editCtx, editCancel := context.WithTimeout(m.ctx, time.Minute)
			defer editCancel()
			if err := t.doc.ApplyEdits(editCtx, editor.Edit{
				Start: 0, End: len(text), Text: out.String(),
			}); err != nil {
				m.status = "Could not resolve the conflicts: " + err.Error()
			} else {
				m.status = fmt.Sprintf("Resolved %d conflicts", len(blocks))
			}
			m.invalidate()
		})
	}()
}

func (m *Model) StepConflict(delta int) {
	t := m.tab()
	if t == nil || t.doc == nil {
		return
	}
	blocks := parseConflictMarkers(t.doc.Text())
	if len(blocks) == 0 {
		m.status = "This file has no conflict markers"
		m.invalidate()
		return
	}
	caret := t.state.Cursor()
	next := blocks[0]
	if delta > 0 {
		for _, block := range blocks {
			if block.from > caret {
				next = block
				break
			}
		}
	} else {
		next = blocks[len(blocks)-1]
		for i := len(blocks) - 1; i >= 0; i-- {
			if blocks[i].from < caret {
				next = blocks[i]
				break
			}
		}
	}
	line := strings.Count(t.doc.Text()[:next.from], "\n")
	t.state.GoTo(editor.Pos{Line: line, Col: 0})
	m.status = fmt.Sprintf("Conflict at line %d", line+1)
	m.invalidate()
}

func (m *Model) ShowLogWithArg(arg any) {
	path, branch, repoRoot := "", "", ""
	switch value := arg.(type) {
	case string:
		path = value
	case map[string]any:
		path, _ = value["path"].(string)
		branch, _ = value["branch"].(string)
		repoRoot, _ = value["repo"].(string)
	}
	if repoRoot == "" {
		if path != "" {
			repoRoot = vcsRepositoryContainingPath(realPath(path), m.vcs.repositories)
		} else {
			repoRoot = m.vcs.current
		}
	}
	if repoRoot != "" && vcsRepositoryKey(m.log.repo) != vcsRepositoryKey(repoRoot) {
		if oldRoot := vcsRepositoryKey(m.log.repo); oldRoot != "" {
			m.rememberVCSLog(oldRoot)
		}
		visible := m.log.visible
		m.log = LogPane{}
		if parked, ok := m.vcs.logs[vcsRepositoryKey(repoRoot)]; ok {
			m.log = parked
		}
		m.log.repo = repoRoot
		m.log.visible = visible
	}
	m.vcs.logGeneration++
	m.log.loading = false
	m.log.detailing = false
	m.log.aggregate = false
	m.log.path, m.log.branch = path, branch
	if branch != "" {
		m.log.path = ""
	}
	if path == "" && branch == "" {
		m.enableVCSAggregateLog(repoRoot)
	}
	m.DockShow("log")
	if repoRoot != "" {
		m.ensureVCSLogForRepository(repoRoot)
	}
}

func (m *Model) ShowHistoryForFile(arg any) {
	path := firstActionPath(arg)
	if path == "" {
		path = actionPathOrDefault(m, nil)
	}
	if path == "" {
		m.status = "Select a file to show its history"
		m.invalidate()
		return
	}
	m.ShowLogWithArg(map[string]any{"path": path})
}

func (m *Model) ShowHistoryForSelection() {
	t := m.tab()
	if t == nil || t.doc == nil {
		m.status = "Open a file first"
		m.invalidate()
		return
	}
	m.ShowHistoryForFile(map[string]any{"path": t.doc.Path()})
}

// AskRollbackAction bridges OpenIDEA's vcs.rollback action to the native
// changes confirmation. An explicit path wins, then the project-tree
// selection and the active editor.
func (m *Model) AskRollbackAction(arg any) {
	path := firstActionPath(arg)
	if path == "" {
		if entry, ok := m.selectedTreeEntry(); ok {
			path = entry.path
		} else if tab := m.tab(); tab != nil && tab.doc != nil {
			path = tab.doc.Path()
		}
	}
	if path == "" || m.Root() == "" {
		m.status = "Select a changed file to roll back"
		m.invalidate()
		return
	}
	rel, err := filepath.Rel(m.Root(), path)
	if err != nil {
		rel = filepath.Base(path)
	}
	m.AskRollback(openidea.FileStatus{
		Path: path, RelPath: filepath.ToSlash(rel),
	})
}

func (m *Model) ShowDiffAction(arg any) {
	path := firstActionPath(arg)
	if path == "" {
		if entry, ok := m.selectedTreeEntry(); ok && !entry.dir {
			path = entry.path
		} else if tab := m.tab(); tab != nil && tab.doc != nil {
			path = tab.doc.Path()
		}
	}
	if path == "" {
		if m.log.visible && len(m.log.changes) > 0 {
			m.ShowSelectedLogChangeDiff()
			return
		}
		m.status = "Open or select a file to compare"
		m.invalidate()
		return
	}
	m.ShowDiff(path)
}

func (m *Model) ShowAnnotateAction(arg any) {
	path := firstActionPath(arg)
	if path == "" {
		if entry, ok := m.selectedTreeEntry(); ok && !entry.dir {
			path = entry.path
		} else if tab := m.tab(); tab != nil && tab.doc != nil {
			path = tab.doc.Path()
		}
	}
	m.ShowAnnotate(path)
}

func (m *Model) ShowLocalHistoryAction(arg any) {
	path := firstActionPath(arg)
	if path == "" {
		if entry, ok := m.selectedTreeEntry(); ok {
			path = entry.path
		} else if tab := m.tab(); tab != nil && tab.doc != nil {
			path = tab.doc.Path()
		} else {
			path = m.Root()
		}
	}
	m.ShowHistory(path)
}

func (m *Model) ShowCurrentRevision(arg any) {
	path := firstActionPath(arg)
	if path == "" {
		path = actionPathOrDefault(m, nil)
	}
	if path == "" {
		return
	}
	m.openVCSMore(vcsMoreMessage, "current.revision", "Current Revision",
		"Recent commits that changed "+filepath.Base(path)+".")
	m.vcsMore.loading = true
	repoArg := m.vcsRepositoryArgForPath(path)
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, time.Minute)
		defer cancel()
		raw, err := m.invoke(ctx, "vcs_log", map[string]any{
			"repo": repoArg,
			"query": map[string]any{
				"branch": nil, "path": path, "author": nil, "text": nil,
				"skip": 0, "limit": 20,
			},
		})
		var commits []openidea.Commit
		if err == nil {
			err = json.Unmarshal(raw, &commits)
		}
		m.post(func() {
			m.vcsMore.loading = false
			if err != nil {
				m.vcsMore.err = "Could not read the current revision: " + err.Error()
				m.invalidate()
				return
			}
			for _, commit := range commits {
				m.vcsMore.items = append(m.vcsMore.items, vcsMoreItem{
					ID: commit.ID, Label: commit.Summary,
					Detail: commit.ShortID + " · " + commit.Author + " · " +
						logTime(m.prefs.dateFormatPrefs(), m.systemRegion, commit.TimeMs),
				})
			}
			m.invalidate()
		})
	}()
}

func (m *Model) OpenCheckoutRevision(arg any) {
	revision, _ := arg.(string)
	m.openVCSMore(vcsMorePrompt, vcsMoreActionCheckout, "Checkout Revision",
		"Enter a tag, branch or commit. A commit gives a detached HEAD.")
	m.vcsMore.input = revision
}

func (m *Model) submitCheckout() {
	revision := strings.TrimSpace(m.vcsMore.input)
	if revision == "" {
		m.vcsMore.err = "Enter a tag, branch or revision"
		m.invalidate()
		return
	}
	m.vcsMore.busy, m.vcsMore.err = true, ""
	repoArg := m.vcsRepositoryArg()
	if m.vcsMore.input3 != "" {
		repoArg = vcsRepositoryArgFor(m.vcsMore.input3)
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 5*time.Minute)
		defer cancel()
		_, err := m.invoke(ctx, "vcs_checkout", map[string]any{
			"repo": repoArg, "branch": revision, "create": false,
			"options": map[string]any{"mode": "normal"},
		})
		m.post(func() {
			m.vcsMore.busy = false
			if err != nil {
				m.vcsMore.err = "Checkout failed: " + err.Error()
				m.invalidate()
				return
			}
			m.status = "Checked out " + revision
			m.closeVCSMore()
			m.RefreshAfterVCSCommand()
		})
	}()
}

func (m *Model) OpenCompareWithRevision(arg any) {
	paths := m.vcsActionPaths(arg, false)
	if len(paths) == 0 {
		m.status = "Open or select a file or folder to compare"
		m.invalidate()
		return
	}
	m.openVCSMore(vcsMorePrompt, "compare.revision", "Compare with Branch or Revision",
		"Enter the branch or revision used as the left side.")
	m.vcsMore.input2 = strings.Join(paths, "\x00")
}

func (m *Model) submitCompareWithRevision() {
	revision := strings.TrimSpace(m.vcsMore.input)
	paths := strings.Split(m.vcsMore.input2, "\x00")
	if revision == "" || len(paths) == 0 {
		m.vcsMore.err = "Enter a revision"
		m.invalidate()
		return
	}
	m.vcsMore.busy, m.vcsMore.err = true, ""
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 5*time.Minute)
		defer cancel()
		request := m.nextRequest()
		raw, err := m.mgr.Invoke(ctx, Window, "vcs_compare_paths", map[string]any{
			"paths": paths, "revision": revision, "repositories": nil, "request": request,
		})
		var result struct {
			Repositories []struct {
				Root     string  `json:"root"`
				Revision *string `json:"revision"`
				Files    []struct {
					Path    string  `json:"path"`
					Kind    string  `json:"kind"`
					OldPath *string `json:"oldPath"`
					Added   int     `json:"added"`
					Removed int     `json:"removed"`
				} `json:"files"`
				Error           string `json:"error"`
				MissingRevision bool   `json:"missingRevision"`
			} `json:"repositories"`
			SkippedPaths []string `json:"skippedPaths"`
		}
		if err == nil {
			err = json.Unmarshal(raw, &result)
		}
		m.post(func() {
			m.vcsMore.busy = false
			if err != nil {
				m.vcsMore.err = "Compare failed: " + err.Error()
				m.invalidate()
				return
			}
			var items []vcsMoreItem
			for _, repo := range result.Repositories {
				if repo.MissingRevision {
					m.vcsMore.err = "Revision " + revision + " does not exist"
					continue
				}
				if repo.Error != "" {
					m.vcsMore.err = repo.Error
					continue
				}
				left := revision
				if repo.Revision != nil && *repo.Revision != "" {
					left = *repo.Revision
				}
				for _, file := range repo.Files {
					items = append(items, vcsMoreItem{
						Path: filepath.Join(repo.Root, file.Path), Revision: left,
						Label: filepath.Base(file.Path), Kind: file.Kind,
						Detail: fmt.Sprintf("%s · +%d −%d", strings.ToUpper(file.Kind), file.Added, file.Removed),
					})
				}
			}
			m.vcsMore.kind = vcsMoreChanges
			m.vcsMore.title = "Compare with " + revision
			m.vcsMore.hint = "Choose a file to open its two revisions."
			m.vcsMore.items = items
			m.vcsMore.choice = 0
			m.invalidate()
		})
	}()
}

func (m *Model) openRevisionFileDiff(item vcsMoreItem) {
	m.closeVCSMore()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, time.Minute)
		defer cancel()
		raw, err := m.mgr.Invoke(ctx, Window, "vcs_file_at", map[string]any{
			"path": item.Path, "rev": item.Revision,
		})
		var left *string
		if err == nil {
			err = json.Unmarshal(raw, &left)
		}
		right := ""
		if err == nil && item.After != "" {
			rawRight, rightErr := m.mgr.Invoke(ctx, Window, "vcs_file_at", map[string]any{
				"path": item.Path, "rev": item.After,
			})
			err = rightErr
			if err == nil {
				var value *string
				err = json.Unmarshal(rawRight, &value)
				if value != nil {
					right = *value
				}
			}
		} else if err == nil {
			if data, readErr := os.ReadFile(item.Path); readErr == nil {
				right = string(data)
			} else if !os.IsNotExist(readErr) {
				err = readErr
			}
		}
		m.post(func() {
			if err != nil {
				m.status = "Could not compare the file: " + err.Error()
				m.invalidate()
				return
			}
			leftText := ""
			if left != nil {
				leftText = *left
			}
			label := item.Revision
			if label == "" {
				label = "Revision"
			}
			rightLabel := "Local"
			if item.After != "" {
				rightLabel = item.After
			}
			m.CompareTextsArg(map[string]any{
				"path":  item.Path,
				"left":  map[string]any{"text": leftText, "label": label},
				"right": map[string]any{"text": right, "label": rightLabel},
			})
		})
	}()
}

func (m *Model) selectedLogCommitArg(arg any) *openidea.Commit {
	id, repository := "", ""
	switch value := arg.(type) {
	case string:
		id = value
	case map[string]any:
		if valueID, ok := value["id"].(string); ok {
			id = valueID
		} else if valueID, ok := value["revision"].(string); ok {
			id = valueID
		}
		repository, _ = value["repo"].(string)
	}
	if id == "" && m.log.selected >= 0 && m.log.selected < len(m.log.commits) {
		return &m.log.commits[m.log.selected]
	}
	for i := range m.log.commits {
		if (m.log.commits[i].ID == id || m.log.commits[i].ShortID == id) &&
			(repository == "" || i >= len(m.log.commitRepos) ||
				vcsRepositoryKey(m.log.commitRepos[i]) == vcsRepositoryKey(repository)) {
			return &m.log.commits[i]
		}
	}
	if id != "" {
		return &openidea.Commit{ID: id, ShortID: shortID(id)}
	}
	return nil
}

func logCommitArgParts(arg any) (id, repository string) {
	switch value := arg.(type) {
	case string:
		return value, ""
	case map[string]any:
		id, _ = value["id"].(string)
		if id == "" {
			id, _ = value["revision"].(string)
		}
		repository, _ = value["repo"].(string)
	}
	return id, vcsRepositoryKey(repository)
}

func (m *Model) logRepositoryForCommitArg(arg any) string {
	_, repository := logCommitArgParts(arg)
	if repository != "" {
		return repository
	}
	if m.log.detailRepo != "" {
		return vcsRepositoryKey(m.log.detailRepo)
	}
	return m.gitLogRepositoryRoot()
}

func (m *Model) CopyRevisionArg(arg any) {
	commit := m.selectedLogCommitArg(arg)
	if commit == nil {
		m.status = "Select a commit in the Git Log"
		m.invalidate()
		return
	}
	mygo.Clipboard.WriteText(commit.ID)
	m.status = "Copied revision " + commit.ShortID
	m.invalidate()
}

func (m *Model) CreatePatchForCommits(arg any) {
	commit := m.selectedLogCommitArg(arg)
	if commit == nil {
		m.status = "Select a commit in the Git Log"
		m.invalidate()
		return
	}
	repoArg := vcsRepositoryArgFor(m.logRepositoryForCommitArg(arg))
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, time.Minute)
		defer cancel()
		raw, err := m.invoke(ctx, "vcs_create_patch", map[string]any{
			"repo": repoArg,
			"source": map[string]any{
				"from": "commits", "commits": []string{commit.ID}, "reverse": false,
			},
		})
		var patch string
		if err == nil {
			err = json.Unmarshal(raw, &patch)
		}
		m.post(func() {
			if err != nil {
				m.status = "Could not create the patch: " + err.Error()
				m.invalidate()
				return
			}
			m.savePatch(patch, commit.ShortID)
		})
	}()
}

func (m *Model) CheckoutCommitArg(arg any) {
	commit := m.selectedLogCommitArg(arg)
	if commit == nil {
		return
	}
	m.vcsMore = vcsMoreState{
		open: true, kind: vcsMorePrompt, action: vcsMoreActionCheckout,
		title: "Checkout Revision", hint: "Check out " + commit.ShortID + " with a detached HEAD.",
		input: commit.ID, input3: m.logRepositoryForCommitArg(arg),
	}
	m.invalidate()
}

func (m *Model) CompareCommitWithLocal(arg any) {
	commit := m.selectedLogCommitArg(arg)
	if commit == nil {
		m.status = "Select a commit in the Git Log"
		m.invalidate()
		return
	}
	repo := m.logRepositoryForCommitArg(arg)
	repoArg := vcsRepositoryArgFor(repo)
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
		defer cancel()
		raw, err := m.invoke(ctx, "vcs_diff_with_worktree", map[string]any{
			"repo": repoArg, "rev": commit.ID,
		})
		var files []openidea.RevisionChange
		if err == nil {
			err = json.Unmarshal(raw, &files)
		}
		m.post(func() {
			if err != nil {
				m.status = "Compare with local failed: " + err.Error()
				m.invalidate()
				return
			}
			m.vcsMore = vcsMoreState{
				open: true, kind: vcsMoreChanges, action: "commit.local",
				title: "Compare with Local · " + commit.ShortID,
				hint:  "Choose a file to compare the commit with the working tree.",
			}
			for _, file := range files {
				path := file.Path
				if !filepath.IsAbs(path) {
					path = filepath.Join(repo, path)
				}
				m.vcsMore.items = append(m.vcsMore.items, vcsMoreItem{
					Path: path, Revision: commit.ID, Label: filepath.Base(path),
					Detail: strings.ToUpper(file.Kind), Kind: file.Kind,
				})
			}
			m.invalidate()
		})
	}()
}

func (m *Model) CompareRevisionsArg(arg any) {
	var revisions []string
	switch value := arg.(type) {
	case []string:
		revisions = value
	case []any:
		for _, item := range value {
			if id, ok := item.(string); ok {
				revisions = append(revisions, id)
			}
		}
	}
	if len(revisions) != 2 {
		m.vcsMore = vcsMoreState{
			open: true, kind: vcsMorePrompt, action: "compare.revisions",
			title: "Compare Versions", hint: "Enter the left revision.",
		}
		m.invalidate()
		return
	}
	m.compareRevisions(revisions[0], revisions[1])
}

func (m *Model) compareRevisions(from, to string) {
	repo := m.vcsRepositoryRoot()
	if repo == "" {
		repo = m.gitLogRepositoryRoot()
	}
	repoArg := vcsRepositoryArgFor(repo)
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
		defer cancel()
		raw, err := m.invoke(ctx, "vcs_diff_revisions", map[string]any{
			"repo": repoArg, "from": from, "to": to,
		})
		var files []openidea.RevisionChange
		if err == nil {
			err = json.Unmarshal(raw, &files)
		}
		m.post(func() {
			if err != nil {
				m.status = "Compare versions failed: " + err.Error()
				m.invalidate()
				return
			}
			m.vcsMore = vcsMoreState{
				open: true, kind: vcsMoreChanges, action: "commit.compare",
				title: "Compare " + shortID(from) + " ↔ " + shortID(to),
				hint:  "Choose a file to compare the two revisions.",
			}
			for _, file := range files {
				path := file.Path
				if !filepath.IsAbs(path) {
					path = filepath.Join(repo, path)
				}
				m.vcsMore.items = append(m.vcsMore.items, vcsMoreItem{
					Path: path, Revision: from, After: to, Label: filepath.Base(path),
					Detail: strings.ToUpper(file.Kind), Kind: file.Kind,
				})
			}
			m.invalidate()
		})
	}()
}

func (m *Model) ResetToCommitArg(arg any) {
	commit := m.selectedLogCommitArg(arg)
	if commit == nil {
		return
	}
	m.selectLogCommitTarget(arg)
	m.openVCSOperationAt(vcsOpReset, m.logRepositoryForCommitArg(arg))
}

func (m *Model) UndoCommitArg(arg any) {
	commit := m.selectedLogCommitArg(arg)
	if commit == nil {
		return
	}
	selected := *commit
	repository := m.logRepositoryForCommitArg(arg)
	root := m.Root()
	m.status = "Checking whether commit " + shortID(selected.ID) + " can be undone…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.mergeContext(), time.Minute)
		defer cancel()
		info, err := m.undoCommitBranchInfo(ctx, repository)
		m.post(func() {
			if m.Root() != root {
				return
			}
			if err != nil {
				problem := "The branch state could not be read: " + err.Error()
				m.status = "Cannot undo commit: " + problem
				m.notifyError("vcs.undo."+selected.ID, "Cannot undo commit", problem)
				m.invalidate()
				return
			}
			problem := undoCommitProblem(selected, info)
			if problem == "" {
				problem = undoCommitPushedProtectedProblem(info, m.vcsProtectedBranch)
			}
			if problem != "" {
				m.status = "Cannot undo commit: " + problem
				m.notifyError("vcs.undo."+selected.ID, "Cannot undo commit", problem)
				m.invalidate()
				return
			}
			hint := "Move HEAD to the parent and keep this commit's changes staged?"
			if undoCommitHeadPushed(info) {
				hint = "This commit is already on " + strings.TrimSpace(valueOrEmpty(info.Upstream)) +
					". Undo rewrites local history and may require a force push.\n" + hint
			}
			m.vcsMore = vcsMoreState{
				open: true, kind: vcsMoreConfirm, action: "commit.undo",
				title: "Undo Commit", hint: hint,
				input: selected.ID, input3: repository,
			}
			m.status = ""
			m.invalidate()
		})
	}()
}

func (m *Model) undoCommit(id, repository string) {
	if repository == "" {
		repository = m.gitLogRepositoryRoot()
	}
	repoArg := vcsRepositoryArgFor(repository)
	m.vcsMore.busy = true
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
		defer cancel()
		raw, err := m.invoke(ctx, "vcs_undo_commit", map[string]any{
			"repo": repoArg, "commit": id,
		})
		var message string
		if err == nil {
			err = json.Unmarshal(raw, &message)
		}
		m.post(func() {
			m.vcsMore.busy = false
			if err != nil {
				m.vcsMore.err = "Undo Commit failed: " + err.Error()
				m.invalidate()
				return
			}
			if m.commit != nil && message != "" {
				m.commit.message = message
			}
			if message != "" && repository != "" {
				m.keepCommitDraft(repository, message)
			}
			m.status = "Undid commit " + shortID(id)
			m.closeVCSMore()
			m.RefreshAfterVCSCommand()
		})
	}()
}

func (m *Model) EditCommitMessageArg(arg any) {
	commit := m.selectedLogCommitArg(arg)
	if commit == nil {
		return
	}
	m.vcsMore = vcsMoreState{
		open: true, kind: vcsMorePrompt, action: vcsMoreActionReword,
		title: "Edit Commit Message", hint: "Reword " + commit.ShortID + " and its descendants.",
		input: commit.Message, input2: commit.ID, input3: m.logRepositoryForCommitArg(arg),
	}
	if m.vcsMore.input == "" {
		m.vcsMore.input = commit.Summary
	}
	m.invalidate()
}

func (m *Model) rewordCommit(id, message, repository string) {
	if repository == "" {
		repository = m.gitLogRepositoryRoot()
	}
	repoArg := vcsRepositoryArgFor(repository)
	m.vcsMore.busy = true
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
		defer cancel()
		_, err := m.invoke(ctx, "vcs_reword", map[string]any{
			"repo": repoArg, "commit": id, "message": message,
		})
		m.post(func() {
			m.vcsMore.busy = false
			if err != nil {
				m.vcsMore.err = "Edit Commit Message failed: " + err.Error()
				m.invalidate()
				return
			}
			m.status = "Updated commit message"
			m.closeVCSMore()
			m.RefreshAfterVCSCommand()
		})
	}()
}

func (m *Model) DropCommitArg(arg any) {
	commit := m.selectedLogCommitArg(arg)
	if commit == nil {
		return
	}
	m.vcsMore = vcsMoreState{
		open: true, kind: vcsMoreConfirm, action: "commit.drop",
		title: "Drop Commit",
		hint:  "Remove " + commit.ShortID + " “" + commit.Summary + "” from this branch?",
		input: commit.ID, input3: m.logRepositoryForCommitArg(arg),
	}
	m.invalidate()
}

func (m *Model) SquashCommitArg(arg any) {
	commit := m.selectedLogCommitArg(arg)
	if commit == nil {
		return
	}
	m.vcsMore = vcsMoreState{
		open: true, kind: vcsMoreConfirm, action: "commit.squash",
		title: "Squash Commits",
		hint:  "Meld " + commit.ShortID + " into the preceding commit?",
		input: commit.ID, input3: m.logRepositoryForCommitArg(arg),
	}
	m.invalidate()
}

func (m *Model) PrepareFixupArg(arg any, kind string) {
	commit := m.selectedLogCommitArg(arg)
	if commit == nil {
		return
	}
	m.selectLogCommitTarget(arg)
	m.OpenCommit()
	if m.commit == nil {
		return
	}
	m.commit.allRepositories = false
	m.commit.repo = m.logRepositoryForCommitArg(arg)
	m.commit.message = kind + "! " + commit.Summary
	for _, file := range m.vcs.files {
		fileKind := effectiveVCSKind(file)
		m.commit.checks[file.Path] = fileKind != "" && fileKind != "ignored" && fileKind != "conflicted"
	}
	m.status = "Prepared a " + kind + " commit for " + commit.ShortID
	m.invalidate()
}

func (m *Model) runRebasePlan(targetID, targetAction, title, repository string) {
	root := m.Root()
	if repository == "" {
		repository = m.gitLogRepositoryRoot()
	}
	repoArg := vcsRepositoryArgFor(repository)
	m.vcsMore.busy, m.vcsMore.err = true, ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
		defer cancel()
		raw, err := m.invoke(ctx, "vcs_log", map[string]any{
			"repo": repoArg,
			"query": map[string]any{
				"branch": "HEAD", "path": nil, "author": nil, "text": nil,
				"range": targetID + "^..HEAD", "skip": 0, "limit": 1000,
			},
		})
		var commits []openidea.Commit
		if err == nil {
			err = json.Unmarshal(raw, &commits)
		}
		m.post(func() {
			m.vcsMore.busy = false
			if m.Root() != root {
				return
			}
			if err != nil || len(commits) == 0 {
				m.vcsMore.err = "Could not build the history plan"
				m.invalidate()
				return
			}
			var base any
			if len(commits[len(commits)-1].Parents) > 0 {
				base = commits[len(commits)-1].Parents[0]
			}
			entries := make([]map[string]any, 0, len(commits))
			for i := len(commits) - 1; i >= 0; i-- {
				action := "pick"
				if commits[i].ID == targetID {
					action = targetAction
				}
				entries = append(entries, map[string]any{"commit": commits[i].ID, "action": action})
			}
			m.closeVCSMore()
			m.vcsOps = VCSOperationPane{
				kind: "operation", open: true, title: title,
			}
			m.runVCSGitOperationAt(repoArg, title, map[string]any{
				"op": "interactiveRebase", "base": base, "entries": entries,
			})
		})
	}()
}

func (m *Model) RebaseOntoCommitArg(arg any) {
	commit := m.selectedLogCommitArg(arg)
	if commit == nil {
		return
	}
	repoArg := vcsRepositoryArgFor(m.logRepositoryForCommitArg(arg))
	m.vcsOps = VCSOperationPane{
		kind: "operation", open: true, title: "Rebase onto " + commit.ShortID,
	}
	m.vcsOps.busy = true
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Minute)
		defer cancel()
		_, err := m.invoke(ctx, "vcs_rebase_with", map[string]any{
			"repo": repoArg, "upstream": commit.ID, "options": map[string]any{},
		})
		m.post(func() {
			m.vcsOps.busy = false
			if err != nil {
				m.vcsOps.err = "Rebase failed: " + err.Error()
				m.invalidate()
				return
			}
			m.status = "Rebased onto " + commit.ShortID
			m.resetVCSOperations()
			m.RefreshAfterVCSCommand()
		})
	}()
}

func (m *Model) GoToLogParent() {
	commit := m.selectedLogCommitArg(nil)
	if commit == nil || len(commit.Parents) == 0 {
		return
	}
	repo := m.logCommitRepository(m.log.selected)
	for i := range m.log.commits {
		if vcsRepositoryKey(m.logCommitRepository(i)) == vcsRepositoryKey(repo) &&
			m.log.commits[i].ID == commit.Parents[0] {
			m.selectCommit(i)
			return
		}
	}
	m.status = "The parent commit is outside the loaded log"
	m.invalidate()
}

func (m *Model) GoToLogChild() {
	commit := m.selectedLogCommitArg(nil)
	if commit == nil {
		return
	}
	repo := m.logCommitRepository(m.log.selected)
	for i := range m.log.commits {
		if vcsRepositoryKey(m.logCommitRepository(i)) != vcsRepositoryKey(repo) {
			continue
		}
		for _, parent := range m.log.commits[i].Parents {
			if parent == commit.ID {
				m.selectCommit(i)
				return
			}
		}
	}
	m.status = "The child commit is outside the loaded log"
	m.invalidate()
}

func (m *Model) NewBranchAtCommit(arg any) {
	commit := m.selectedLogCommitArg(arg)
	if commit == nil {
		return
	}
	m.selectLogCommitTarget(arg)
	repo := m.logRepositoryForCommitArg(arg)
	m.openVCSOperationAt(vcsOpNewBranch, repo)
	m.vcsOps.preferredStart = commit.ID
}

func (m *Model) NewTagAtCommit(arg any) {
	commit := m.selectedLogCommitArg(arg)
	if commit == nil {
		return
	}
	m.selectLogCommitTarget(arg)
	repo := m.logRepositoryForCommitArg(arg)
	m.openVCSOperationAt(vcsOpNewTag, repo)
	m.vcsOps.choices = []vcsOperationChoice{{
		label: commit.ShortID + " " + commit.Summary,
		value: commit.ID,
	}}
	m.vcsOps.sel = 0
}

func (m *Model) selectLogCommitTarget(arg any) bool {
	id, repository := logCommitArgParts(arg)
	if id == "" {
		return m.log.selected >= 0 && m.log.selected < len(m.log.commits)
	}
	for i := range m.log.commits {
		if (m.log.commits[i].ID == id || m.log.commits[i].ShortID == id) &&
			(repository == "" || i >= len(m.log.commitRepos) ||
				vcsRepositoryKey(m.log.commitRepos[i]) == repository) {
			m.log.selected = i
			m.log.detailRepo = m.logCommitRepository(i)
			return true
		}
	}
	return false
}

func (m *Model) OpenCommitInBrowser(arg any) {
	commit := m.selectedLogCommitArg(arg)
	if commit == nil {
		return
	}
	repoArg := vcsRepositoryArgFor(m.logRepositoryForCommitArg(arg))
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, time.Minute)
		defer cancel()
		raw, err := m.invoke(ctx, "vcs_config_get", map[string]any{
			"repo": repoArg, "key": "remote.origin.url",
		})
		var remote *string
		if err == nil {
			err = json.Unmarshal(raw, &remote)
		}
		m.post(func() {
			if err != nil || remote == nil || *remote == "" {
				m.status = "No web URL for remote origin"
				m.invalidate()
				return
			}
			url := commitWebURL(*remote, commit.ID)
			if url == "" {
				m.status = "Could not derive a web URL from " + *remote
			} else {
				openReleasePage(url)
				m.status = "Opened " + commit.ShortID + " in the browser"
			}
			m.invalidate()
		})
	}()
}

func commitWebURL(remote, id string) string {
	remote = strings.TrimSpace(remote)
	remote = strings.TrimSuffix(remote, "/")
	remote = strings.TrimSuffix(remote, ".git")
	if strings.HasPrefix(remote, "git@") {
		rest := strings.TrimPrefix(remote, "git@")
		if colon := strings.IndexByte(rest, ':'); colon >= 0 {
			remote = "https://" + rest[:colon] + "/" + rest[colon+1:]
		}
	} else if strings.HasPrefix(remote, "ssh://git@") {
		remote = "https://" + strings.TrimPrefix(remote, "ssh://git@")
	}
	switch {
	case strings.HasPrefix(remote, "http://"), strings.HasPrefix(remote, "https://"):
	default:
		return ""
	}
	if strings.Contains(remote, "bitbucket.org") {
		return remote + "/commits/" + id
	}
	return remote + "/commit/" + id
}
