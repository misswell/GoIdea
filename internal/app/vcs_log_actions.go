package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

const emptyTreeID = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

func init() {
	add := func(id, title, category, keywords string, run func(*Model, any)) {
		_ = registerAction(&Action{
			ID: id, Title: title, Category: category, Keywords: keywords, Builtin: true,
			When: func(m *Model) bool { return m.Root() != "" || m.log.repo != "" },
			Run:  func(m *Model) { run(m, nil) },
			RunWithArg: func(m *Model, arg any) {
				run(m, arg)
			},
		})
	}

	add("localHistory.recentChanges", "Recent Changes", "View", "local history", func(m *Model, _ any) {
		m.ShowHistory(m.Root())
	})
	add("vcs.showCommit", "Show Commit in Git Log", "Git", "reveal commit", func(m *Model, arg any) {
		m.ShowCommitInGitLog(arg)
	})
	add("vcs.openOnRemote", "Open on Git Remote", "Git", "github gitlab bitbucket browser", func(m *Model, arg any) {
		m.OpenPathOnGitRemote(arg)
	})
	add("vcs.log.openExternal", "Open External Git Log…", "Git Log", "repository", func(m *Model, _ any) {
		m.OpenExternalGitLog()
	})
	add("vcs.log.goToRef", "Go to Hash/Branch/Tag", "Git Log", "find jump revision", func(m *Model, arg any) {
		m.PromptGoToGitLogRef(arg)
	})
	add("vcs.log.revealBookmark", "Show Commit Bookmark", "Git Log", "bookmark repository commit", func(m *Model, arg any) {
		m.RevealCommitBookmark(arg)
	})
	add("vcs.log.toggleBookmark", "Bookmark Commit", "Git Log", "bookmark favorite commit", func(m *Model, arg any) {
		m.ToggleCommitBookmark(arg)
	})
	add("vcs.log.showBranches", "Show Branches", "Git Log", "branch pane", func(m *Model, _ any) {
		m.ToggleGitLogBranches()
	})
	add("vcs.log.collapseLinear", "Collapse Linear Branches", "Git Log", "flatten graph", func(m *Model, _ any) {
		m.log.collapseLinear = true
		m.saveGitLogPreferences()
		m.invalidate()
	})
	add("vcs.log.expandLinear", "Expand Linear Branches", "Git Log", "show graph", func(m *Model, _ any) {
		m.log.collapseLinear = false
		m.saveGitLogPreferences()
		m.invalidate()
	})
	add("vcs.log.branches.new", "New Branch…", "Git Log", "branch", func(m *Model, arg any) {
		m.NewBranchFromGitLogBranches(arg)
	})
	add("vcs.log.branches.update", "Update Selected", "Git Log", "pull fast-forward branch", func(m *Model, arg any) {
		m.UpdateGitLogBranch(arg)
	})
	add("vcs.log.branches.delete", "Delete Branch", "Git Log", "remove branch", func(m *Model, arg any) {
		m.AskDeleteGitLogBranch(arg)
	})
	add("vcs.log.branches.toggleFavorite", "Mark/Unmark As Favorite", "Git Log", "star branch", func(m *Model, arg any) {
		m.ToggleFavoriteGitLogBranch(arg)
	})
	add("vcs.log.branches.navigate", "Navigate Log to Selected Branch", "Git Log", "locate ref", func(m *Model, arg any) {
		m.NavigateGitLogToBranchArg(arg)
	})
	add("vcs.log.changes.showDiff", "Show Diff", "Git Log", "changed file", func(m *Model, arg any) {
		m.ShowLogChangeDiff(arg)
	})
	add("vcs.log.changes.compareWithLocal", "Compare with Local", "Git Log", "revision working tree", func(m *Model, arg any) {
		m.CompareLogChangeWithLocal(arg, "after")
	})
	add("vcs.log.changes.compareBeforeWithLocal", "Compare Before with Local", "Git Log", "previous revision working tree", func(m *Model, arg any) {
		m.CompareLogChangeWithLocal(arg, "before")
	})
	add("vcs.log.changes.jumpToSource", "Jump to Source", "Git Log", "open file", func(m *Model, arg any) {
		m.JumpToLogChangeSource(arg)
	})
	add("vcs.log.changes.getFromRevision", "Get from Revision", "Git Log", "restore file", func(m *Model, arg any) {
		m.AskGetLogChangeFromRevision(arg)
	})
	add("vcs.log.changes.revert", "Revert Selected Changes", "Git Log", "undo file changes", func(m *Model, arg any) {
		m.AskRevertLogChange(arg)
	})
	add("vcs.log.changes.groupByDirectory", "Group by Directory", "Git Log", "tree folders", func(m *Model, _ any) {
		m.log.groupByDirectory = !m.log.groupByDirectory
		m.saveGitLogPreferences()
		m.invalidate()
	})
	add("vcs.log.changes.diffPreview", "Show Diff Preview", "Git Log", "preview", func(m *Model, _ any) {
		m.log.diffPreview = !m.log.diffPreview
		m.saveGitLogPreferences()
		if m.log.diffPreview {
			m.ShowSelectedLogChangeDiff()
		}
		m.invalidate()
	})
}

// gitLogRepositoryRoot is the repository whose branch favorites and commit
// bookmarks the visible log belongs to.
func (m *Model) gitLogRepositoryRoot() string {
	root := m.log.repo
	if root == "" {
		root = m.Root()
	}
	if root == "" {
		return ""
	}
	return filepath.Clean(root)
}

// gitLogScopedKey keeps branch and commit bookmarks from colliding when
// several repositories are open, as OpenIDEA's multi-root log does.
func gitLogScopedKey(root, id string) string {
	return filepath.Clean(root) + "\x00" + id
}

func (m *Model) applyGitLogPreferences() {
	options := m.prefs.VCS.Log
	m.log.showBranches = options.showBranches()
	m.log.collapseLinear = options.collapseLinear()
	m.log.groupByDirectory = options.groupByDirectory()
	m.log.diffPreview = options.diffPreview()
	m.loadGitLogBranchFavorites()
	m.loadGitLogBookmarks()
}

func (m *Model) saveGitLogPreferences() {
	options := m.prefs.VCS.Log
	options.ShowBranches = boolPref(m.log.showBranches)
	options.CollapseLinear = boolPref(m.log.collapseLinear)
	options.GroupByDirectory = boolPref(m.log.groupByDirectory)
	options.DiffPreview = boolPref(m.log.diffPreview)
	m.prefs.VCS.Log = options
	m.savePreferences()
}

func (m *Model) loadGitLogBranchFavorites() {
	root := m.gitLogRepositoryRoot()
	m.log.favoriteBranches = map[string]bool{}
	prefix := filepath.Clean(root) + "\x00"
	for key, favorite := range m.prefs.VCS.FavoriteBranches {
		if !favorite || !strings.HasPrefix(key, prefix) {
			continue
		}
		if name := strings.TrimPrefix(key, prefix); name != "" {
			m.log.favoriteBranches[name] = true
		}
	}
}

func (m *Model) loadGitLogBookmarks() {
	m.log.bookmarks = map[string]string{}
	for key, summary := range m.prefs.VCS.CommitBookmarks {
		if summary == "" {
			continue
		}
		m.log.bookmarks[key] = summary
	}
}

func (m *Model) selectedCommitBookmarkKey(commit *openidea.Commit) string {
	if commit == nil {
		return ""
	}
	root := m.gitLogRepositoryRoot()
	if root == "" {
		return ""
	}
	return gitLogScopedKey(root, commit.ID)
}

func (m *Model) commitIsBookmarked(commit *openidea.Commit) bool {
	key := m.selectedCommitBookmarkKey(commit)
	return key != "" && m.log.bookmarks[key] != ""
}

func (m *Model) ToggleCommitBookmark(arg any) {
	commit := m.selectedLogCommitArg(arg)
	key := m.selectedCommitBookmarkKey(commit)
	if commit == nil || key == "" {
		return
	}
	bookmarks := m.prefs.VCS.CommitBookmarks
	if bookmarks == nil {
		bookmarks = map[string]string{}
	}
	if _, exists := bookmarks[key]; exists {
		delete(bookmarks, key)
		delete(m.log.bookmarks, key)
		m.status = "Removed the commit bookmark"
	} else {
		bookmarks[key] = commit.Summary
		if m.log.bookmarks == nil {
			m.log.bookmarks = map[string]string{}
		}
		m.log.bookmarks[key] = commit.Summary
		m.status = "Bookmarked " + commit.ShortID
	}
	m.prefs.VCS.CommitBookmarks = bookmarks
	m.savePreferences()
	m.invalidate()
}

type commitBookmarkTarget struct {
	repo string
	id   string
}

func parseCommitBookmark(arg any) (commitBookmarkTarget, bool) {
	switch value := arg.(type) {
	case string:
		value = strings.TrimSpace(value)
		if strings.HasPrefix(value, "git-log:") {
			target := strings.TrimPrefix(value, "git-log:")
			repo, id, ok := strings.Cut(target, "#")
			if !ok || id == "" {
				return commitBookmarkTarget{}, false
			}
			decoded, err := url.PathUnescape(repo)
			if err != nil {
				return commitBookmarkTarget{}, false
			}
			return commitBookmarkTarget{repo: decoded, id: id}, true
		}
		if repo, id, ok := strings.Cut(value, "\x00"); ok && id != "" {
			return commitBookmarkTarget{repo: repo, id: id}, true
		}
		if value != "" {
			return commitBookmarkTarget{id: value}, true
		}
	case map[string]any:
		repo, _ := value["repo"].(string)
		id, _ := value["id"].(string)
		if id != "" {
			return commitBookmarkTarget{repo: repo, id: id}, true
		}
		if key, ok := value["key"].(string); ok {
			return parseCommitBookmark(key)
		}
	}
	return commitBookmarkTarget{}, false
}

func commitBookmarkPath(root, id string) string {
	return "git-log:" + url.PathEscape(filepath.Clean(root)) + "#" + id
}

func (m *Model) RevealCommitBookmark(arg any) {
	target, ok := parseCommitBookmark(arg)
	if !ok {
		m.status = "Select a commit bookmark"
		m.invalidate()
		return
	}
	currentRoot := m.gitLogRepositoryRoot()
	if target.repo == "" || filepath.Clean(target.repo) == filepath.Clean(currentRoot) {
		m.DockShow("log")
		m.goToGitLogRef(target.id)
		return
	}
	m.openExternalGitLogTarget(target.repo, target.id)
}

func (m *Model) openExternalGitLogTarget(path, revision string) {
	path = realPath(path)
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.mgr.Invoke(ctx, Window, "vcs_open_external_log", map[string]any{"path": path})
		var root string
		if err == nil {
			err = json.Unmarshal(raw, &root)
		}
		m.post(func() {
			if err != nil {
				m.status = "Could not open the Git log: " + err.Error()
				m.invalidate()
				return
			}
			m.log = LogPane{repo: root, visible: true}
			m.applyGitLogPreferences()
			m.DockShow("log")
			m.status = "Opened Git log for " + root
			m.invalidate()
			m.goToGitLogRef(revision)
		})
	}()
}

func (m *Model) ToggleGitLogBranches() {
	m.log.showBranches = !m.log.showBranches
	m.saveGitLogPreferences()
	if m.log.showBranches && len(m.log.branches) == 0 && !m.log.branchesLoading {
		m.loadGitLogBranches()
	}
	m.invalidate()
}

func (m *Model) loadGitLogBranches() {
	root, repo := m.Root(), m.log.repo
	m.log.branchesLoading, m.log.branchesErr = true, ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, time.Minute)
		defer cancel()
		var repoArg any
		if repo != "" {
			repoArg = repo
		}
		raw, err := m.mgr.Invoke(ctx, Window, "vcs_branches", map[string]any{"repo": repoArg})
		var branches []openidea.Branch
		if err == nil {
			err = json.Unmarshal(raw, &branches)
		}
		m.post(func() {
			if m.Root() != root || m.log.repo != repo {
				return
			}
			m.log.branchesLoading = false
			if err != nil {
				m.log.branchesErr = "Could not read branches: " + err.Error()
			} else {
				m.log.branches = branches
				m.log.branchSelected = clampSymbolSel(m.log.branchSelected, len(branches))
				m.loadGitLogBranchFavorites()
			}
			m.invalidate()
		})
	}()
}

func (m *Model) selectedGitLogBranch(arg any) (openidea.Branch, bool) {
	if ref := branchRefArg(arg); ref != "" {
		for _, branch := range m.log.branches {
			if branch.Name == ref {
				return branch, true
			}
		}
		return openidea.Branch{Name: ref}, true
	}
	if m.log.branchSelected < 0 || m.log.branchSelected >= len(m.log.branches) {
		return openidea.Branch{}, false
	}
	return m.log.branches[m.log.branchSelected], true
}

func branchRefArg(arg any) string {
	switch value := arg.(type) {
	case string:
		return value
	case map[string]any:
		if ref, ok := value["ref"].(string); ok {
			return ref
		}
		if branch, ok := value["branch"].(map[string]any); ok {
			if name, ok := branch["name"].(string); ok {
				return name
			}
		}
	}
	return ""
}

func (m *Model) gitLogBranchMenu(menu *ui.Menu, index int) {
	if index >= 0 && index < len(m.log.branches) {
		m.log.branchSelected = index
	}
	branch, ok := m.selectedGitLogBranch(nil)
	if menu.Item("New Branch…").Chosen() {
		m.NewBranchFromGitLogBranches(nil)
	}
	if menu.Item("Update Selected").Disabled(!ok || branch.Remote || !branch.Current && branch.Upstream == nil).Chosen() {
		m.UpdateGitLogBranch(nil)
	}
	if menu.Item("Delete").Disabled(!ok || branch.Remote || branch.Current).Chosen() {
		m.AskDeleteGitLogBranch(nil)
	}
	favorite := ok && m.log.favoriteBranches[branch.Name]
	if menu.Item("Mark/Unmark As Favorite").Checked(favorite).Disabled(!ok).Chosen() {
		m.ToggleFavoriteGitLogBranch(nil)
	}
	if menu.Item("Navigate Log to Selected Branch").Disabled(!ok).Chosen() {
		m.NavigateGitLogToBranchArg(nil)
	}
}

func (m *Model) NewBranchFromGitLogBranches(arg any) {
	branch, ok := m.selectedGitLogBranch(arg)
	start := "HEAD"
	if ok && branch.Name != "HEAD" && !branch.Remote && !branch.Current {
		start = branch.Name
	}
	m.OpenNewGitBranchAt(start)
}

func (m *Model) OpenNewGitBranchAtLogSelection() {
	m.NewBranchFromGitLogBranches(nil)
}

func (m *Model) UpdateGitLogBranch(arg any) {
	branch, ok := m.selectedGitLogBranch(arg)
	if !ok || branch.Remote {
		return
	}
	m.runVCSRemote(vcsOpFetch, "Update "+branch.Name,
		map[string]any{"op": "updateBranch", "branch": branch.Name})
}

func (m *Model) AskDeleteGitLogBranch(arg any) {
	branch, ok := m.selectedGitLogBranch(arg)
	if !ok || branch.Remote || branch.Current {
		m.status = "Select a local branch other than the current branch"
		m.invalidate()
		return
	}
	m.openVCSMore(vcsMoreConfirm, "branch.delete", "Delete Branch",
		"Delete local branch “"+branch.Name+"”?")
	m.vcsMore.input = branch.Name
}

func (m *Model) deleteGitLogBranch(name string) {
	if name == "" {
		return
	}
	m.vcsMore.busy, m.vcsMore.err = true, ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
		defer cancel()
		var repoArg any
		if m.log.repo != "" {
			repoArg = m.log.repo
		}
		_, err := m.mgr.Invoke(ctx, Window, "vcs_delete_branch", map[string]any{
			"repo": repoArg, "branch": name,
		})
		m.post(func() {
			m.vcsMore.busy = false
			if err != nil {
				m.vcsMore.err = "Could not delete the branch: " + err.Error()
				m.invalidate()
				return
			}
			m.status = "Deleted branch " + name
			m.closeVCSMore()
			m.loadGitLogBranches()
			m.RefreshAfterVCSCommand()
		})
	}()
}

func (m *Model) ToggleFavoriteGitLogBranch(arg any) {
	branch, ok := m.selectedGitLogBranch(arg)
	if !ok {
		return
	}
	if m.log.favoriteBranches == nil {
		m.log.favoriteBranches = map[string]bool{}
	}
	m.log.favoriteBranches[branch.Name] = !m.log.favoriteBranches[branch.Name]
	root := m.gitLogRepositoryRoot()
	if root != "" {
		key := gitLogScopedKey(root, branch.Name)
		favorites := m.prefs.VCS.FavoriteBranches
		if favorites == nil {
			favorites = map[string]bool{}
		}
		if m.log.favoriteBranches[branch.Name] {
			favorites[key] = true
		} else {
			delete(favorites, key)
		}
		m.prefs.VCS.FavoriteBranches = favorites
		m.savePreferences()
	}
	m.invalidate()
}

func (m *Model) NavigateGitLogToBranchArg(arg any) {
	branch, ok := m.selectedGitLogBranch(arg)
	if !ok {
		return
	}
	m.goToGitLogRef(branch.Name)
}

func (m *Model) NavigateGitLogToBranch(index int) {
	if index < 0 || index >= len(m.log.branches) {
		return
	}
	m.log.branchSelected = index
	m.NavigateGitLogToBranchArg(nil)
}

func (m *Model) gitLogOptionsMenu(menu *ui.Menu) {
	if menu.Item("Branches").Checked(m.log.showBranches).Chosen() {
		m.ToggleGitLogBranches()
	}
	if menu.Item("Collapse Linear Branches").Checked(m.log.collapseLinear).Chosen() {
		m.log.collapseLinear = true
		m.saveGitLogPreferences()
		m.invalidate()
	}
	if menu.Item("Expand Linear Branches").Checked(!m.log.collapseLinear).Chosen() {
		m.log.collapseLinear = false
		m.saveGitLogPreferences()
		m.invalidate()
	}
	menu.Separator()
	if menu.Item("Group Changes by Directory").Checked(m.log.groupByDirectory).Chosen() {
		m.log.groupByDirectory = !m.log.groupByDirectory
		m.saveGitLogPreferences()
		m.invalidate()
	}
	if menu.Item("Show Diff Preview").Checked(m.log.diffPreview).Chosen() {
		m.log.diffPreview = !m.log.diffPreview
		m.saveGitLogPreferences()
		m.invalidate()
	}
}

func (m *Model) gitLogBookmarksMenu(menu *ui.Menu) {
	type entry struct {
		key     string
		summary string
	}
	entries := make([]entry, 0, len(m.log.bookmarks))
	for key, summary := range m.log.bookmarks {
		entries = append(entries, entry{key: key, summary: summary})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].summary == entries[j].summary {
			return entries[i].key < entries[j].key
		}
		return entries[i].summary < entries[j].summary
	})
	for _, item := range entries {
		if menu.Item(item.summary).Chosen() {
			m.RevealCommitBookmark(item.key)
		}
	}
}

func (m *Model) gitLogCommitMenu(menu *ui.Menu, id string) {
	m.selectLogCommitByID(id)
	if menu.Item("Copy Revision Number").Chosen() {
		m.CopyRevisionArg(id)
	}
	bookmarkLabel := "Bookmark Commit"
	selected := m.selectedLogCommitArg(nil)
	if m.commitIsBookmarked(selected) {
		bookmarkLabel = "Remove Commit Bookmark"
	}
	if menu.Item(bookmarkLabel).Chosen() {
		m.ToggleCommitBookmark(nil)
	}
	if menu.Item("Create Patch…").Chosen() {
		m.CreatePatchForCommits(id)
	}
	if menu.Item("Checkout Revision").Chosen() {
		m.CheckoutCommitArg(id)
	}
	if menu.Item("Compare with Local").Chosen() {
		m.CompareCommitWithLocal(id)
	}
	if menu.Item("Revert Commit").Chosen() {
		m.RevertSelectedCommit()
	}
	menu.Separator()
	if menu.Item("Reset Current Branch to Here…").Chosen() {
		m.ResetToCommitArg(id)
	}
	if menu.Item("Undo Commit…").Chosen() {
		m.UndoCommitArg(id)
	}
	if menu.Item("Edit Commit Message…").Chosen() {
		m.EditCommitMessageArg(id)
	}
	if menu.Item("Drop Commit").Chosen() {
		m.DropCommitArg(id)
	}
	if menu.Item("Squash Commits…").Chosen() {
		m.SquashCommitArg(id)
	}
	if menu.Item("Interactively Rebase from Here…").Chosen() {
		m.OpenInteractiveRebase(id)
	}
	menu.Separator()
	if menu.Item("New Branch…").Chosen() {
		m.NewBranchAtCommit(id)
	}
	if menu.Item("New Tag…").Chosen() {
		m.NewTagAtCommit(id)
	}
	if menu.Item("View in Browser").Chosen() {
		m.OpenCommitInBrowser(id)
	}
}

func (m *Model) gitLogChangeMenu(menu *ui.Menu, index int) {
	if index >= 0 && index < len(m.log.changes) {
		m.log.changeSelected = index
	}
	if menu.Item("Show Diff").Chosen() {
		m.ShowSelectedLogChangeDiff()
	}
	if menu.Item("Compare with Local").Chosen() {
		m.CompareLogChangeWithLocal(nil, "after")
	}
	if menu.Item("Compare Before with Local").Chosen() {
		m.CompareLogChangeWithLocal(nil, "before")
	}
	if menu.Item("Jump to Source").Chosen() {
		m.JumpToLogChangeSource(nil)
	}
	menu.Separator()
	if menu.Item("Get from Revision").Chosen() {
		m.AskGetLogChangeFromRevision(nil)
	}
	if menu.Item("Revert Selected Changes").Chosen() {
		m.AskRevertLogChange(nil)
	}
	menu.Separator()
	if menu.Item("Group by Directory").Checked(m.log.groupByDirectory).Chosen() {
		m.log.groupByDirectory = !m.log.groupByDirectory
		m.saveGitLogPreferences()
		m.invalidate()
	}
	if menu.Item("Show Diff Preview").Checked(m.log.diffPreview).Chosen() {
		m.log.diffPreview = !m.log.diffPreview
		m.saveGitLogPreferences()
		m.invalidate()
	}
}

type logChangeTarget struct {
	change   openidea.RevisionChange
	commit   openidea.Commit
	root     string
	path     string
	leftPath string
	before   string
	after    string
}

func (m *Model) selectedLogChangeTarget(arg any) (logChangeTarget, bool) {
	index := m.log.changeSelected
	if path := firstActionPath(arg); path != "" {
		for i, change := range m.log.changes {
			if change.Path == path || filepath.Base(change.Path) == filepath.Base(path) {
				index = i
				break
			}
		}
	}
	if index < 0 || index >= len(m.log.changes) ||
		m.log.selected < 0 || m.log.selected >= len(m.log.commits) {
		return logChangeTarget{}, false
	}
	change := m.log.changes[index]
	commit := m.log.commits[m.log.selected]
	root := m.log.repo
	if root == "" {
		root = m.Root()
	}
	if root == "" {
		return logChangeTarget{}, false
	}
	path := change.Path
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	leftPath := path
	if change.OldPath != nil && *change.OldPath != "" {
		leftPath = filepath.Join(root, *change.OldPath)
	}
	before := emptyTreeID
	if len(commit.Parents) > 0 {
		before = commit.Parents[0]
	}
	after := commit.ID
	if change.Kind == "added" {
		before = ""
	}
	if change.Kind == "deleted" {
		after = ""
	}
	return logChangeTarget{
		change: change, commit: commit, root: root, path: path,
		leftPath: leftPath, before: before, after: after,
	}, true
}

func (m *Model) fileAtRevision(ctx context.Context, path, revision string) (string, error) {
	if path == "" || revision == "" {
		return "", nil
	}
	var revArg any
	if revision != "" {
		revArg = revision
	}
	raw, err := m.mgr.Invoke(ctx, Window, "vcs_file_at", map[string]any{
		"path": path, "rev": revArg,
	})
	if err != nil {
		return "", err
	}
	var value *string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	if value == nil {
		return "", nil
	}
	return *value, nil
}

func (m *Model) ShowSelectedLogChangeDiff() {
	m.ShowLogChangeDiff(nil)
}

func (m *Model) ShowLogChangeDiff(arg any) {
	target, ok := m.selectedLogChangeTarget(arg)
	if !ok {
		m.status = "Select a changed file in the Git Log"
		m.invalidate()
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, time.Minute)
		defer cancel()
		left, err := m.fileAtRevision(ctx, target.leftPath, target.before)
		if err != nil {
			m.post(func() {
				m.status = "Could not read the previous revision: " + err.Error()
				m.invalidate()
			})
			return
		}
		right, err := m.fileAtRevision(ctx, target.path, target.after)
		if err != nil {
			m.post(func() {
				m.status = "Could not read the commit revision: " + err.Error()
				m.invalidate()
			})
			return
		}
		m.post(func() {
			m.CompareTextsArg(map[string]any{
				"path": target.path,
				"left": map[string]any{
					"text": left, "label": map[bool]string{true: "Parent", false: "Empty"}[target.before != ""],
				},
				"right": map[string]any{
					"text": right, "label": map[bool]string{true: target.commit.ShortID, false: "Empty"}[target.after != ""],
				},
			})
		})
	}()
}

func (m *Model) CompareLogChangeWithLocal(arg any, side string) {
	target, ok := m.selectedLogChangeTarget(arg)
	if !ok {
		m.status = "Select a changed file in the Git Log"
		m.invalidate()
		return
	}
	revision, revisionPath := target.after, target.path
	label := target.commit.ShortID
	if side == "before" {
		revision, revisionPath = target.before, target.leftPath
		label = "Before " + target.commit.ShortID
	}
	if revision == "" {
		m.status = "This side of the change does not exist"
		m.invalidate()
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, time.Minute)
		defer cancel()
		left, err := m.fileAtRevision(ctx, revisionPath, revision)
		if err != nil {
			m.post(func() {
				m.status = "Could not read the revision: " + err.Error()
				m.invalidate()
			})
			return
		}
		m.post(func() {
			right := ""
			if t := m.tab(); t != nil && t.doc != nil && realPath(t.doc.Path()) == realPath(target.path) {
				right = t.doc.Text()
			} else if data, readErr := os.ReadFile(target.path); readErr == nil {
				right = string(data)
			}
			m.CompareTextsArg(map[string]any{
				"path":  target.path,
				"left":  map[string]any{"text": left, "label": label},
				"right": map[string]any{"text": right, "label": "Local"},
			})
		})
	}()
}

func (m *Model) JumpToLogChangeSource(arg any) {
	target, ok := m.selectedLogChangeTarget(arg)
	if !ok || target.after == "" {
		m.status = "This file does not exist in the working tree"
		m.invalidate()
		return
	}
	m.openFile(target.path)
}

func (m *Model) AskGetLogChangeFromRevision(arg any) {
	target, ok := m.selectedLogChangeTarget(arg)
	if !ok {
		return
	}
	m.openVCSMore(vcsMoreConfirm, "changes.getFromRevision", "Get from Revision",
		"Replace "+filepath.Base(target.path)+" with its content from "+target.commit.ShortID+"? Local changes are lost.")
	m.vcsMore.input = target.path + "\x00" + target.after
}

func (m *Model) getLogChangeFromRevision(path, revision string) {
	m.vcsMore.busy, m.vcsMore.err = true, ""
	m.invalidate()
	tabs := append([]*Tab(nil), m.tabs...)
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
		defer cancel()
		if err := m.saveOpenTabs(ctx, tabs); err != nil {
			m.post(func() {
				m.vcsMore.busy = false
				m.vcsMore.err = "Could not save the open files: " + err.Error()
				m.invalidate()
			})
			return
		}
		var err error
		if revision == "" {
			_, err = m.mgr.Invoke(ctx, Window, "fs_delete", map[string]any{"path": path})
		} else {
			var text string
			text, err = m.fileAtRevision(ctx, path, revision)
			if err == nil {
				err = os.MkdirAll(filepath.Dir(path), 0o755)
				if err == nil {
					err = os.WriteFile(path, []byte(text), 0o644)
				}
			}
		}
		m.post(func() {
			m.vcsMore.busy = false
			if err != nil {
				m.vcsMore.err = "Could not get the file from the revision: " + err.Error()
				m.invalidate()
				return
			}
			m.status = "Got " + filepath.Base(path) + " from the revision"
			m.closeVCSMore()
			m.ReloadAllFromDisk()
			m.RefreshAfterVCSCommand()
		})
	}()
}

func (m *Model) AskRevertLogChange(arg any) {
	target, ok := m.selectedLogChangeTarget(arg)
	if !ok {
		return
	}
	m.openVCSMore(vcsMoreConfirm, "changes.revert", "Revert Selected Changes",
		"Apply the reverse of "+filepath.Base(target.path)+" from "+target.commit.ShortID+"?")
	m.vcsMore.input = target.commit.ID + "\x00" + target.path
	if target.change.OldPath != nil {
		m.vcsMore.input += "\x00" + *target.change.OldPath
	}
}

func (m *Model) revertSelectedLogChange(commitID, path, oldPath string) {
	if commitID == "" || path == "" {
		return
	}
	m.vcsMore.busy, m.vcsMore.err = true, ""
	m.invalidate()
	tabs := append([]*Tab(nil), m.tabs...)
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
		defer cancel()
		if err := m.saveOpenTabs(ctx, tabs); err != nil {
			m.post(func() {
				m.vcsMore.busy = false
				m.vcsMore.err = "Could not save the open files: " + err.Error()
				m.invalidate()
			})
			return
		}
		var repoArg any
		if m.log.repo != "" {
			repoArg = m.log.repo
		}
		raw, err := m.mgr.Invoke(ctx, Window, "vcs_create_patch", map[string]any{
			"repo": repoArg,
			"source": map[string]any{
				"from": "commits", "commits": []string{commitID}, "reverse": true,
			},
		})
		var patch string
		if err == nil {
			err = json.Unmarshal(raw, &patch)
		}
		paths := map[string]bool{filepath.ToSlash(path): true}
		if oldPath != "" {
			paths[filepath.ToSlash(oldPath)] = true
		}
		if err == nil {
			patch = filterUnifiedPatch(patch, paths)
		}
		if err == nil && strings.TrimSpace(patch) == "" {
			err = fmt.Errorf("nothing to revert")
		}
		if err == nil {
			_, err = m.mgr.Invoke(ctx, Window, "vcs_apply_patch", map[string]any{
				"repo": repoArg, "patch": patch,
			})
		}
		m.post(func() {
			m.vcsMore.busy = false
			if err != nil {
				m.vcsMore.err = "Could not revert the changes: " + err.Error()
				m.invalidate()
				return
			}
			m.status = "Reverted changes to " + filepath.Base(path)
			m.closeVCSMore()
			m.ReloadAllFromDisk()
			m.RefreshAfterVCSCommand()
		})
	}()
}

// filterUnifiedPatch keeps the git diff sections that touch paths.
func filterUnifiedPatch(patch string, paths map[string]bool) string {
	if strings.TrimSpace(patch) == "" {
		return ""
	}
	var kept strings.Builder
	for _, section := range unifiedPatchSections(patch) {
		oldPath, newPath, ok := unifiedPatchHeaderPaths(section)
		if ok && (paths[oldPath] || paths[newPath]) {
			kept.WriteString(section)
		}
	}
	return kept.String()
}

func unifiedPatchSections(patch string) []string {
	const marker = "diff --git "
	first := strings.Index(patch, marker)
	if first < 0 {
		return nil
	}
	nextMarker := "\n" + marker
	sections := make([]string, 0, 4)
	for start := first; start >= 0; {
		relativeEnd := strings.Index(patch[start+len(marker):], nextMarker)
		if relativeEnd < 0 {
			sections = append(sections, patch[start:])
			break
		}
		end := start + len(marker) + relativeEnd
		sections = append(sections, patch[start:end+1])
		start = end + 1
	}
	return sections
}

func unifiedPatchHeaderPaths(section string) (string, string, bool) {
	const marker = "diff --git "
	if !strings.HasPrefix(section, marker) {
		return "", "", false
	}
	line := strings.TrimPrefix(section, marker)
	if end := strings.IndexByte(line, '\n'); end >= 0 {
		line = line[:end]
	}
	oldPath, rest, ok := consumeGitDiffPath(line)
	if !ok {
		return "", "", false
	}
	newPath, rest, ok := consumeGitDiffPath(strings.TrimLeft(rest, " "))
	if !ok || strings.TrimSpace(rest) != "" {
		return "", "", false
	}
	oldPath = strings.TrimPrefix(filepath.ToSlash(oldPath), "a/")
	newPath = strings.TrimPrefix(filepath.ToSlash(newPath), "b/")
	return oldPath, newPath, true
}

func consumeGitDiffPath(value string) (string, string, bool) {
	if value == "" {
		return "", "", false
	}
	if value[0] == '"' {
		escaped := false
		for i := 1; i < len(value); i++ {
			switch {
			case escaped:
				escaped = false
			case value[i] == '\\':
				escaped = true
			case value[i] == '"':
				path, err := strconv.Unquote(value[:i+1])
				if err != nil {
					return "", "", false
				}
				return path, value[i+1:], true
			}
		}
		return "", "", false
	}
	if end := strings.Index(value, " b/"); end >= 0 {
		return value[:end], value[end:], true
	}
	return value, "", true
}

func (m *Model) PromptGoToGitLogRef(arg any) {
	revision, _ := arg.(string)
	m.openVCSMore(vcsMorePrompt, "log.goToRef", "Go to Hash/Branch/Tag",
		"Enter a commit hash, branch or tag name.")
	m.vcsMore.input = revision
}

func (m *Model) goToGitLogRef(revision string) {
	revision = strings.TrimSpace(revision)
	if revision == "" {
		return
	}
	for i, commit := range m.log.commits {
		if commit.ID == revision || strings.HasPrefix(commit.ID, revision) {
			m.selectCommit(i)
			m.status = "Found " + commit.ShortID
			m.invalidate()
			return
		}
		for _, ref := range commit.Refs {
			if ref == revision || strings.TrimPrefix(ref, "HEAD -> ") == revision {
				m.selectCommit(i)
				m.status = "Found " + revision
				m.invalidate()
				return
			}
		}
	}
	m.loadGitLogAtRef(revision)
}

func (m *Model) loadGitLogAtRef(revision string) {
	root, repo := m.Root(), m.log.repo
	var repoArg any
	if repo != "" {
		repoArg = repo
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, time.Minute)
		defer cancel()
		raw, err := m.mgr.Invoke(ctx, Window, "vcs_log", map[string]any{
			"repo": repoArg,
			"query": map[string]any{
				"branch": revision, "path": nil, "author": nil, "text": nil,
				"skip": 0, "limit": logLimit,
			},
		})
		var commits []openidea.Commit
		if err == nil {
			err = json.Unmarshal(raw, &commits)
		}
		m.post(func() {
			if m.Root() != root || m.log.repo != repo {
				return
			}
			if err != nil || len(commits) == 0 {
				m.status = "‘" + revision + "’ was not found in the log"
				m.invalidate()
				return
			}
			m.log.commits = commits
			m.log.loaded = true
			m.log.branch = revision
			m.selectCommit(0)
		})
	}()
}

func (m *Model) OpenExternalGitLog() {
	parent := m.win
	start := m.Root()
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: parent, Title: "Open Git Log", ButtonLabel: "Open",
			DefaultPath: start, Directory: true,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		m.post(func() { m.openExternalGitLogTarget(paths[0], "") })
	}()
}

func (m *Model) ShowCommitInGitLog(arg any) {
	id := ""
	switch value := arg.(type) {
	case string:
		id = value
	case map[string]any:
		id, _ = value["id"].(string)
	}
	if id == "" {
		if commit := m.selectedLogCommitArg(nil); commit != nil {
			id = commit.ID
		}
	}
	if id == "" {
		return
	}
	m.DockShow("log")
	m.goToGitLogRef(id)
}

func (m *Model) OpenPathOnGitRemote(arg any) {
	path, dir, line := actionPathSoft(arg)
	if path == "" {
		path = actionPathOrDefault(m, nil)
	}
	if path == "" || m.Root() == "" {
		m.status = "Select a file or folder inside the project"
		m.invalidate()
		return
	}
	root := m.Root()
	rel, err := filepath.Rel(root, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		m.status = "This file is outside the Git repository"
		m.invalidate()
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, time.Minute)
		defer cancel()
		remoteName := "origin"
		info, infoErr := m.branchInfo(ctx)
		if infoErr == nil && info.Upstream != nil {
			if slash := strings.IndexByte(*info.Upstream, '/'); slash > 0 {
				remoteName = (*info.Upstream)[:slash]
			}
		}
		raw, err := m.mgr.Invoke(ctx, Window, "vcs_config_get", map[string]any{
			"repo": nil, "key": "remote." + remoteName + ".url",
		})
		var remote *string
		if err == nil {
			err = json.Unmarshal(raw, &remote)
		}
		branch := "HEAD"
		if infoErr == nil && info.Head != nil && *info.Head != "" {
			branch = *info.Head
		}
		m.post(func() {
			if err != nil || remote == nil || *remote == "" {
				m.status = "This file has no page on a Git remote"
				m.invalidate()
				return
			}
			url := fileWebURL(*remote, branch, filepath.ToSlash(rel), dir, line)
			if url == "" {
				m.status = "Could not derive a web URL from " + *remote
				m.invalidate()
				return
			}
			openReleasePage(url)
			m.status = "Opened " + filepath.Base(path) + " on Git Remote"
			m.invalidate()
		})
	}()
}

func actionPathSoft(arg any) (path string, dir bool, line int) {
	switch value := arg.(type) {
	case string:
		return value, false, 0
	case map[string]any:
		path, _ = value["path"].(string)
		dir, _ = value["dir"].(bool)
		switch number := value["line"].(type) {
		case int:
			line = number
		case float64:
			line = int(number)
		}
	}
	return path, dir, line
}

func fileWebURL(remote, revision, rel string, dir bool, line int) string {
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
	if !strings.HasPrefix(remote, "https://") && !strings.HasPrefix(remote, "http://") {
		return ""
	}
	rel = strings.TrimPrefix(filepath.ToSlash(rel), "/")
	switch {
	case strings.Contains(remote, "bitbucket.org"):
		if dir {
			return remote + "/src/" + revision + "/" + rel
		}
		url := remote + "/src/" + revision + "/" + rel
		if line > 0 {
			url += fmt.Sprintf("#lines-%d", line)
		}
		return url
	case strings.Contains(remote, "gitlab"):
		url := remote + "/-/blob/" + revision + "/" + rel
		if dir {
			url = remote + "/-/tree/" + revision + "/" + rel
		}
		if line > 0 {
			url += fmt.Sprintf("#L%d", line)
		}
		return url
	case strings.Contains(remote, "github"), strings.Contains(remote, "gitea"):
		kind := "blob"
		if dir {
			kind = "tree"
		}
		url := remote + "/" + kind + "/" + revision + "/" + rel
		if line > 0 {
			url += fmt.Sprintf("#L%d", line)
		}
		return url
	}
	kind := "blob"
	if dir {
		kind = "tree"
	}
	url := remote + "/" + kind + "/" + revision + "/" + rel
	if line > 0 {
		url += fmt.Sprintf("#L%d", line)
	}
	return url
}
