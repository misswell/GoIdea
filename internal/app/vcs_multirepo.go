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

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

type vcsContentTab int

const (
	vcsContentChanges vcsContentTab = iota
	vcsContentBranches
	vcsContentShelf
	vcsContentStash
	vcsContentLog
	vcsContentWorktrees
)

// vcsWorktreePane is the Worktrees content tab of one repository. It is
// separate from the full native worktree dialog so loading or selecting a
// worktree in one repository cannot replace another repository's list.
type vcsWorktreePane struct {
	loading    bool
	loaded     bool
	worktrees  []openidea.Worktree
	selected   int
	err        string
	generation uint64
	cancel     context.CancelFunc
}

// vcsRepository is one repository root reported by OpenIDEA's vcs_roots.
// head is nil on a detached HEAD, in which case headCommit is the revision to
// show.
type vcsRepository struct {
	Root       string  `json:"root"`
	Operation  *string `json:"operation"`
	Head       *string `json:"head"`
	HeadCommit *string `json:"headCommit"`
}

// currentVCSRepository returns the repository selected by the Git tool
// window, or nil when the project has no repository.
func (m *Model) currentVCSRepository() *vcsRepository {
	if m.vcs.current == "" {
		return nil
	}
	for i := range m.vcs.repositories {
		if filepath.Clean(m.vcs.repositories[i].Root) == filepath.Clean(m.vcs.current) {
			return &m.vcs.repositories[i]
		}
	}
	return nil
}

func vcsRepositoryKey(root string) string {
	root = filepath.Clean(root)
	if root == "." {
		return ""
	}
	return root
}

func (m *Model) vcsRepositoryHasRoot(root string) bool {
	root = vcsRepositoryKey(root)
	if root == "" {
		return false
	}
	for _, repository := range m.vcs.repositories {
		if vcsRepositoryKey(repository.Root) == root {
			return true
		}
	}
	return false
}

func (m *Model) vcsContentFor(root string) vcsContentTab {
	if tab, ok := m.vcs.content[vcsRepositoryKey(root)]; ok {
		return tab
	}
	return vcsContentChanges
}

func (m *Model) currentVCSContent() vcsContentTab {
	return m.vcsContentFor(m.vcs.current)
}

// SelectVCSTab changes the content shown under the repository tabs. The
// selection is stored per repository, matching OpenIDEA's separate repository
// and Git-content selections.
func (m *Model) SelectVCSTab(tab vcsContentTab) {
	root := vcsRepositoryKey(m.vcs.current)
	if root == "" {
		return
	}
	if m.vcs.content == nil {
		m.vcs.content = map[string]vcsContentTab{}
	}
	m.vcs.content[root] = tab
	switch tab {
	case vcsContentBranches:
		m.ensureVCSBranchesPage(root)
	case vcsContentShelf:
		m.ensureVCSShelfPage()
	case vcsContentStash:
		m.ensureVCSStashPage()
	case vcsContentLog:
		m.enableVCSAggregateLog(root)
		m.ensureVCSLogForRepository(root)
	case vcsContentWorktrees:
		state := m.vcsWorktreeState(root)
		if !state.loaded && !state.loading {
			m.refreshVCSWorktrees(root)
		}
	}
	m.invalidate()
}

// vcsRepositoryBusy reports staging work for one repository while leaving
// other repository tabs interactive.
func (m *Model) vcsRepositoryBusy(root string) bool {
	root = vcsRepositoryKey(root)
	if root == "" || root == vcsRepositoryKey(m.vcs.current) {
		return m.vcs.busy
	}
	return m.vcs.busyByRepo[root]
}

func (m *Model) setVCSRepositoryBusy(root string, busy bool) {
	root = vcsRepositoryKey(root)
	if m.vcs.busyByRepo == nil {
		m.vcs.busyByRepo = map[string]bool{}
	}
	if root != "" {
		m.vcs.busyByRepo[root] = busy
	}
	if root == "" || root == vcsRepositoryKey(m.vcs.current) {
		m.vcs.busy = busy
	}
}

func (m *Model) vcsRepositoryOpErr(root string) string {
	root = vcsRepositoryKey(root)
	if root == "" || root == vcsRepositoryKey(m.vcs.current) {
		return m.vcs.opErr
	}
	return m.vcs.opErrByRepo[root]
}

func (m *Model) setVCSRepositoryOpErr(root, text string) {
	root = vcsRepositoryKey(root)
	if m.vcs.opErrByRepo == nil {
		m.vcs.opErrByRepo = map[string]string{}
	}
	if root != "" {
		if text == "" {
			delete(m.vcs.opErrByRepo, root)
		} else {
			m.vcs.opErrByRepo[root] = text
		}
	}
	if root == "" || root == vcsRepositoryKey(m.vcs.current) {
		m.vcs.opErr = text
	}
}

// rememberVCSRepositoryView snapshots the active branch and log panes. A
// response already in flight may still arrive, but clearing the loading flags
// keeps the parked view usable if it never does.
func (m *Model) rememberVCSRepositoryView(root string) {
	root = vcsRepositoryKey(root)
	if root == "" {
		return
	}
	if m.vcs.branches == nil {
		m.vcs.branches = map[string]BranchPane{}
	}
	branch := m.branch
	branch.loading = false
	branch.listing = false
	m.vcs.branches[root] = branch

	logRoot := vcsRepositoryKey(m.log.repo)
	if logRoot == "" || logRoot == root {
		m.rememberVCSLog(root)
	}
}

func (m *Model) rememberVCSLog(root string) {
	root = vcsRepositoryKey(root)
	if root == "" {
		return
	}
	if m.log.cancel != nil {
		m.log.cancel()
		m.log.cancel = nil
		m.vcs.logGeneration++
	}
	if m.vcs.logs == nil {
		m.vcs.logs = map[string]LogPane{}
	}
	log := m.log
	log.loading = false
	log.detailing = false
	log.repo = root
	m.vcs.logs[root] = log
}

// restoreVCSRepositoryView parks the old repository before installing the
// newly selected one. It preserves the global Log dock's visibility while
// changing its repository-scoped contents.
func (m *Model) restoreVCSRepositoryView(previous, next string) {
	m.rememberVCSRepositoryView(previous)
	next = vcsRepositoryKey(next)
	logVisible := m.log.visible
	m.branch = BranchPane{}
	if branch, ok := m.vcs.branches[next]; ok {
		m.branch = branch
	}
	m.log = LogPane{}
	if log, ok := m.vcs.logs[next]; ok {
		m.log = log
	}
	m.log.repo = next
	m.log.visible = logVisible
	if next != "" {
		if m.vcs.content == nil {
			m.vcs.content = map[string]vcsContentTab{}
		}
		if _, ok := m.vcs.content[next]; !ok {
			m.vcs.content[next] = vcsContentChanges
		}
	}
	m.vcs.busy = false
	m.vcs.opErr = ""
	if next != "" {
		m.vcs.busy = m.vcs.busyByRepo[next]
		m.vcs.opErr = m.vcs.opErrByRepo[next]
	}
}

// vcsRepositoryRoot is the absolute root used as the repo argument of every
// selected-repository RPC.
func (m *Model) vcsRepositoryRoot() string {
	if repo := m.currentVCSRepository(); repo != nil {
		return filepath.Clean(repo.Root)
	}
	return ""
}

func (m *Model) vcsRepositoryArg() any {
	return vcsRepositoryArgFor(m.vcsRepositoryRoot())
}

func (m *Model) vcsRepositoryArgForPath(path string) any {
	if root := vcsRepositoryContainingPath(realPath(path), m.vcs.repositories); root != "" {
		return vcsRepositoryArgFor(root)
	}
	return m.vcsRepositoryArg()
}

func vcsRepositoryArgFor(root string) any {
	if root = filepath.Clean(root); root != "" && root != "." {
		return root
	}
	return nil
}

func vcsCurrentRepository(current string, repositories []vcsRepository) string {
	if current != "" {
		for _, repo := range repositories {
			if filepath.Clean(repo.Root) == filepath.Clean(current) {
				return filepath.Clean(repo.Root)
			}
		}
	}
	if len(repositories) == 0 {
		return ""
	}
	return filepath.Clean(repositories[0].Root)
}

func vcsRepositoryNotFound(err error) bool {
	if err == nil {
		return false
	}
	var oe *openidea.Error
	if errors.As(err, &oe) && oe.Kind == "unavailable" {
		return strings.Contains(strings.ToLower(oe.Message), "not a git repository")
	}
	return strings.Contains(strings.ToLower(err.Error()), "not a git repository")
}

// vcsRepositoryContainingPath returns the innermost repository containing a
// path. Unlike change-status grouping, an exact repository root counts as that
// repository because commit actions name the folder itself.
func vcsRepositoryContainingPath(path string, repositories []vcsRepository) string {
	path = filepath.Clean(path)
	best := ""
	for _, repo := range repositories {
		root := filepath.Clean(repo.Root)
		if path == root || strings.HasPrefix(path, root+string(filepath.Separator)) {
			if len(root) > len(best) {
				best = root
			}
		}
	}
	if best != "" {
		return best
	}
	for _, repo := range repositories {
		if path == filepath.Clean(repo.Root) {
			return path
		}
	}
	return best
}

// vcsRepositoryForPath attributes a changed-file row to a repository. A nested
// repository root is normally reported by its enclosing repository as a
// submodule or untracked folder, so that exact entry belongs to the outer
// repository while files below the root belong to the inner one.
func vcsRepositoryForPath(path string, repositories []vcsRepository) string {
	path = filepath.Clean(path)
	repo := vcsRepositoryContainingPath(path, repositories)
	if repo == "" || path != repo {
		return repo
	}
	best := ""
	for _, candidate := range repositories {
		root := filepath.Clean(candidate.Root)
		if root != path && strings.HasPrefix(path, root+string(filepath.Separator)) &&
			len(root) > len(best) {
			best = root
		}
	}
	if best != "" {
		return best
	}
	return repo
}

func (m *Model) vcsFilesForRepository(root string) []openidea.FileStatus {
	if root == "" {
		return nil
	}
	out := make([]openidea.FileStatus, 0)
	for _, file := range m.vcs.files {
		if vcsRepositoryForPath(file.Path, m.vcs.repositories) == filepath.Clean(root) {
			out = append(out, file)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].RelPath < out[j].RelPath
	})
	return out
}

func (m *Model) currentVCSFiles() []openidea.FileStatus {
	return m.vcsFilesForRepository(m.vcs.current)
}

func (m *Model) vcsChangeQuery(root string) string {
	return strings.TrimSpace(m.vcs.changeQueries[vcsRepositoryKey(root)])
}

func (m *Model) filteredVCSFilesForRepository(root string) []openidea.FileStatus {
	files := m.vcsFilesForRepository(root)
	query := strings.ToLower(m.vcsChangeQuery(root))
	if query == "" {
		return files
	}
	out := make([]openidea.FileStatus, 0, len(files))
	for _, file := range files {
		_, word, _ := vcsBadge(file)
		haystack := strings.ToLower(strings.Join([]string{
			file.RelPath,
			file.Path,
			filepath.Base(file.RelPath),
			word,
			effectiveVCSKind(file),
		}, " "))
		if strings.Contains(haystack, query) {
			out = append(out, file)
		}
	}
	return out
}

// keepVCSSelections drops remembered rows that no longer exist while leaving
// the selections of repositories the user is not looking at untouched.
func (pane *VCSPane) keepVCSSelections(files []openidea.FileStatus) {
	live := make(map[string]bool, len(files))
	for _, file := range files {
		live[filePathKey(file.Path)] = true
	}
	for root, path := range pane.selectedFiles {
		if !live[filePathKey(path)] {
			delete(pane.selectedFiles, root)
		}
	}
}

func filePathKey(path string) string {
	if path == "" {
		return ""
	}
	return filepath.Clean(path)
}

func (m *Model) selectVCSFile(path string) {
	if m.vcs.current == "" || path == "" {
		return
	}
	if m.vcs.selectedFiles == nil {
		m.vcs.selectedFiles = map[string]string{}
	}
	m.vcs.selectedFiles[m.vcs.current] = filepath.Clean(path)
}

func (m *Model) vcsFileSelected(path string) bool {
	return filepath.Clean(m.vcs.selectedFiles[m.vcs.current]) == filepath.Clean(path)
}

// SelectVCSRepository switches the Git tool window context and refreshes its
// branch and commit target. Repository discovery remains project-wide.
func (m *Model) SelectVCSRepository(root string) {
	root = filepath.Clean(root)
	if root == "." || root == "" {
		return
	}
	known := false
	for _, repo := range m.vcs.repositories {
		if filepath.Clean(repo.Root) == root {
			known = true
			break
		}
	}
	if !known || root == m.vcs.current {
		return
	}
	previous := m.vcs.current
	m.vcs.current = root
	m.vcs.selectionGeneration++
	m.restoreVCSRepositoryView(previous, root)
	m.invalidate()
	if m.mgr != nil || m.invokeHook != nil {
		m.RefreshBranch()
	}
	if m.currentVCSContent() == vcsContentLog {
		m.enableVCSAggregateLog(root)
	}
	if m.log.visible || m.currentVCSContent() == vcsContentLog {
		m.ensureVCSLogForRepository(root)
	}
	if m.currentVCSContent() == vcsContentWorktrees {
		state := m.vcsWorktreeState(root)
		if !state.loaded && !state.loading {
			m.refreshVCSWorktrees(root)
		}
	}
	switch m.currentVCSContent() {
	case vcsContentBranches:
		m.ensureVCSBranchesPage(root)
	case vcsContentShelf:
		m.ensureVCSShelfPage()
	case vcsContentStash:
		m.ensureVCSStashPage()
	}
}

// ensureVCSLogForRepository installs the parked log for root and loads it once.
func (m *Model) ensureVCSLogForRepository(root string) {
	root = vcsRepositoryKey(root)
	if root == "" {
		return
	}
	if m.log.aggregate {
		m.log.repo = root
		if (m.log.stale || !m.log.loaded) && !m.log.loading &&
			(m.mgr != nil || m.invokeHook != nil) {
			m.RefreshLog()
		}
		return
	}
	if vcsRepositoryKey(m.log.repo) != root {
		oldRoot := vcsRepositoryKey(m.log.repo)
		if oldRoot != "" {
			m.rememberVCSLog(oldRoot)
		}
		visible := m.log.visible
		m.log = LogPane{}
		if parked, ok := m.vcs.logs[root]; ok {
			m.log = parked
		}
		m.log.repo = root
		m.log.visible = visible
	}
	if (m.log.stale || !m.log.loaded) && !m.log.loading &&
		(m.mgr != nil || m.invokeHook != nil) {
		m.RefreshLog()
	}
}

// enableVCSAggregateLog makes the tool-window Log project-wide. Path, branch
// and external-history logs remain scoped to the repository they name.
func (m *Model) enableVCSAggregateLog(root string) {
	enabled := len(m.vcs.repositories) > 1 && m.log.path == "" && m.log.branch == ""
	wasAggregate := m.log.aggregate
	if enabled == wasAggregate {
		m.log.repo = vcsRepositoryKey(root)
		return
	}
	if m.log.cancel != nil {
		m.log.cancel()
		m.log.cancel = nil
	}
	m.vcs.logGeneration++
	m.log.aggregate = enabled
	m.log.repo = vcsRepositoryKey(root)
	m.log.loading = false
	m.log.loaded = false
	m.log.stale = false
	m.log.err = ""
}

func (m *Model) vcsWorktreeState(root string) *vcsWorktreePane {
	root = vcsRepositoryKey(root)
	if root == "" {
		return &vcsWorktreePane{}
	}
	if m.vcs.worktrees == nil {
		m.vcs.worktrees = map[string]*vcsWorktreePane{}
	}
	state := m.vcs.worktrees[root]
	if state == nil {
		state = &vcsWorktreePane{}
		m.vcs.worktrees[root] = state
	}
	return state
}

func (m *Model) refreshVCSWorktrees(root string) {
	root = vcsRepositoryKey(root)
	if root == "" || (m.mgr == nil && m.invokeHook == nil) {
		return
	}
	state := m.vcsWorktreeState(root)
	if state.loading {
		return
	}
	if state.cancel != nil {
		state.cancel()
	}
	ctx, cancel := context.WithTimeout(m.mergeContext(), time.Minute)
	state.cancel = cancel
	state.generation++
	generation := state.generation
	state.loading = true
	state.err = ""
	m.invalidate()
	go func() {
		defer cancel()
		raw, err := m.invoke(ctx, "vcs_worktrees", map[string]any{
			"repo": vcsRepositoryArgFor(root),
		})
		var worktrees []openidea.Worktree
		if err == nil {
			err = json.Unmarshal(raw, &worktrees)
		}
		m.post(func() {
			if state.generation != generation {
				return
			}
			state.loading = false
			state.loaded = true
			state.cancel = nil
			if err != nil {
				state.err = "Could not read worktrees: " + err.Error()
				state.worktrees = nil
				m.invalidate()
				return
			}
			state.worktrees = worktrees
			state.selected = clampSymbolSel(state.selected, len(worktrees))
			m.invalidate()
		})
	}()
}

func (m *Model) selectVCSWorktree(index int) {
	state := m.vcsWorktreeState(m.vcs.current)
	state.selected = clampSymbolSel(index, len(state.worktrees))
	m.invalidate()
}

func (m *Model) openSelectedVCSWorktree() {
	state := m.vcsWorktreeState(m.vcs.current)
	if state.selected < 0 || state.selected >= len(state.worktrees) {
		return
	}
	m.OpenProject(state.worktrees[state.selected].Path)
}

func (m *Model) vcsRepositoryLabel(repo vcsRepository) string {
	root := filepath.Clean(repo.Root)
	project := filepath.Clean(m.Root())
	if rel, err := filepath.Rel(project, root); err == nil && rel != "." &&
		rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(rel)
	}
	if name := filepath.Base(root); name != "" && name != "." {
		return name
	}
	return root
}

func vcsRepositoryHead(repo vcsRepository) string {
	if repo.Head != nil && *repo.Head != "" {
		return *repo.Head
	}
	if repo.HeadCommit != nil && *repo.HeadCommit != "" {
		commit := *repo.HeadCommit
		if len(commit) > 8 {
			commit = commit[:8]
		}
		return "Detached at " + commit
	}
	return "No commits"
}

func vcsRepositoryBranch(repo vcsRepository) string {
	head := vcsRepositoryHead(repo)
	if repo.Operation != nil && *repo.Operation != "" && repo.Head != nil {
		if operation := branchOperationTitle(repo.Operation, head); operation != "" {
			return operation
		}
	}
	return head
}

func (m *Model) vcsRepositoryCount(root string) int {
	return len(m.vcsFilesForRepository(root))
}

// vcsRepositoryTabs is the Git tool window's per-repository content strip.
func (m *Model) vcsRepositoryTabs(c *ui.Context, p *ui.Theme, oi oiColors) {
	ui.ScrollBoth(c).Key("vcs:repos").Height(38).Shrink(0).TrackScroll(&m.vcs.repoScroll).
		Children(func() {
			ui.Row(c).Gap(4).Padding(0, 6).Children(func() {
				for _, repo := range m.vcs.repositories {
					repo := repo
					active := filepath.Clean(repo.Root) == filepath.Clean(m.vcs.current)
					tab := ui.Row(c.Key("vcs:repo:"+repo.Root)).Height(32).MinWidth(92).
						Padding(0, 8).Gap(6).Radius(6).AlignItems(ui.Center).Cursor(ui.CursorPointer)
					switch {
					case active:
						tab.Background(oi.stripeSelected)
					case tab.Hovered():
						tab.Background(oi.hoverTint)
					}
					tab.Children(func() {
						ui.Column(c).Gap(0).Children(func() {
							ui.Text(c, m.vcsRepositoryLabel(repo)).FontSize(12).Bold().
								TextColor(oi.textStrong).SingleLine().Ellipsis("…")
							ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
								oiIconEl(c, "general/vcs", 11, oi, oi.textSubtle)
								ui.Text(c, vcsRepositoryBranch(repo)).FontSize(10).
									TextColor(oi.textSubtle).SingleLine().Ellipsis("…")
							})
						})
						ui.Text(c, fmt.Sprint(m.vcsRepositoryCount(repo.Root))).FontSize(10).
							TextColor(oi.textSubtle).SingleLine()
					})
					if tab.Clicked() {
						m.SelectVCSRepository(repo.Root)
					}
				}
			})
		})
}

// vcsRepositorySummary names the branch and repository the shared actions
// below it use.
func (m *Model) vcsRepositorySummary(c *ui.Context, oi oiColors, repo vcsRepository) {
	ui.Row(c).Height(24).Shrink(0).Padding(0, 8).Gap(5).AlignItems(ui.Center).Children(func() {
		oiIconEl(c, "general/vcs", 14, oi, oi.icon)
		ui.Text(c, m.vcsRepositoryLabel(repo)).FontSize(11).TextColor(oi.textSubtle).
			SingleLine().Ellipsis("…")
		ui.Text(c, "· "+vcsRepositoryBranch(repo)).FontSize(11).TextColor(oi.textStrong).
			SingleLine().Ellipsis("…")
		ui.Spacer(c)
		ui.Text(c, fmt.Sprint(m.vcsRepositoryCount(repo.Root))).FontSize(10).
			TextColor(oi.textSubtle).SingleLine()
	})
}

func (m *Model) vcsToolbar(c *ui.Context, p *ui.Theme) {
	hasRepo := m.currentVCSRepository() != nil
	busy := m.vcsRepositoryBusy(m.vcs.current)
	ui.Row(c).Height(32).Shrink(0).Padding(0, 6).Gap(3).AlignItems(ui.Center).
		BorderWidth(1, 0, 1, 0).BorderColor(p.Border).Children(func() {
		refresh := "Refresh"
		if m.vcs.loading {
			refresh = "Loading…"
		}
		ui.Button(c, refresh).Size(58, 24).Disabled(m.vcs.loading).OnClick(m.RefreshVCS)
		ui.Button(c, "Commit").Size(54, 24).Disabled(!hasRepo || busy).OnClick(m.OpenCommit)
		ui.Button(c, "Update").Size(50, 24).Disabled(!hasRepo || busy).OnClick(m.OpenGitPull)
		ui.Button(c, "Push").Size(42, 24).Disabled(!hasRepo || busy).OnClick(m.OpenGitPush)
		ui.Button(c, "More…").Size(50, 24).Disabled(!hasRepo).Menu(m.vcsAdvancedMenu)
	})
}

func (m *Model) vcsAdvancedMenu(menu *ui.Menu) {
	if menu.Item("Fetch").Chosen() {
		m.GitFetch()
	}
	if menu.Item("Merge…").Chosen() {
		m.OpenGitMerge()
	}
	if menu.Item("Rebase…").Chosen() {
		m.OpenGitRebase()
	}
	menu.Separator()
	if menu.Item("New Branch…").Chosen() {
		m.OpenNewGitBranch()
	}
	if menu.Item("New Tag…").Chosen() {
		m.OpenNewGitTag()
	}
	if menu.Item("Checkout Tag or Revision…").Chosen() {
		m.OpenCheckoutRevision("")
	}
	menu.Separator()
	if menu.Item("Compare with Branch or Revision…").Chosen() {
		m.OpenCompareWithRevision(map[string]any{"path": m.vcs.current})
	}
	if menu.Item("Worktrees…").Chosen() {
		m.OpenVCSWorktrees()
	}
	if menu.Item("New Worktree…").Chosen() {
		m.OpenNewVCSWorktree()
	}
	if menu.Item("Manage Remotes…").Chosen() {
		m.OpenGitRemotes()
	}
	menu.Separator()
	if menu.Item("Stash Changes…").Chosen() {
		m.OpenGitStash()
	}
	if menu.Item("Unstash Changes…").Chosen() {
		m.OpenGitUnstash()
	}
	if menu.Item("Reset HEAD…").Chosen() {
		m.OpenGitReset()
	}
	if menu.Item("Resolve Conflicts…").Chosen() {
		m.OpenResolveConflicts()
	}
}

// vcsRepositoryRows renders the selected repository's staging groups. The
// full status remains loaded so switching tabs does not require another read.
func (m *Model) vcsRepositoryRows(c *ui.Context, p *ui.Theme) {
	repo := m.currentVCSRepository()
	if repo == nil {
		return
	}
	files := m.filteredVCSFilesForRepository(repo.Root)
	if text := m.vcsRepositoryOpErr(repo.Root); text != "" {
		ui.Row(c).Padding(4, 8).Children(func() {
			ui.Text(c, text).FontSize(12).TextColor(p.Danger).SingleLine().Ellipsis("…")
		})
	}
	staged, unstaged := vcsStaged(files), vcsUnstaged(files)
	if len(staged) == 0 && len(unstaged) == 0 {
		ui.Row(c).Padding(8, 8).Children(func() {
			text := "No local changes"
			if m.vcsChangeQuery(repo.Root) != "" {
				text = "No local changes match the filter"
			}
			ui.Text(c, text).FontSize(12).TextColor(p.TextMuted)
		})
		return
	}
	ui.Column(c).Padding(0, 6).Children(func() {
		if len(staged) > 0 {
			m.vcsGroup(c, p, "Staged Changes", len(staged))
			for i, file := range staged {
				m.vcsRepositoryRow(c, p, "staged", i, file)
			}
		}
		if len(unstaged) > 0 {
			m.vcsGroup(c, p, "Unstaged Changes", len(unstaged))
			for i, file := range unstaged {
				m.vcsRepositoryRow(c, p, "unstaged", i, file)
			}
		}
	})
}

func (m *Model) vcsRepositoryRow(
	c *ui.Context,
	p *ui.Theme,
	side string,
	index int,
	file openidea.FileStatus,
) {
	_, word, _ := vcsBadge(file)
	oi := oiPaletteOf(p)
	selected := m.vcsFileSelected(file.Path)
	row := ui.Row(c.Key(fmt.Sprintf("vcs:%s:%s:%d", m.vcs.current, side, index))).
		Height(24).PaddingStart(18).PaddingEnd(8).Gap(4).Radius(5).
		AlignItems(ui.Center)
	if selected {
		row.Background(p.Selection)
	} else if row.Hovered() {
		row.Background(oi.hoverTint)
	}
	row.Tooltip(word)
	row.Children(func() {
		fileIconEl(c, oi, file.Path, 16, oi.textMuted)
		ui.Text(c, filepath.Base(file.RelPath)).FontSize(13).
			TextColor(vcsKindColor(p, file)).SingleLine().Ellipsis("…")
		ui.Spacer(c)
		if row.Hovered() {
			busy := m.vcsRepositoryBusy(m.vcs.current)
			if side == "staged" {
				if !busy && m.rowChip(c, p, "vcs:unstage:"+file.Path, "Unstage") {
					m.UnstageChange(file)
				}
			} else {
				if !busy && m.rowChip(c, p, "vcs:stage:"+file.Path, "Stage") {
					m.StageChange(file)
				}
				if !busy && m.rowChip(c, p, "vcs:rollback:"+file.Path, "Rollback") {
					m.AskRollback(file)
				}
			}
		}
	})
	row.ContextMenu(func(menu *ui.Menu) {
		m.vcsChangeRowMenu(menu, side, file)
	})
	if row.Clicked() {
		m.selectVCSFile(file.Path)
		m.ShowDiff(file.Path)
	}
	if row.DoubleClicked() {
		m.closeDiff()
		m.openFile(file.Path)
		m.invalidate()
	}
}

// vcsChangeRowMenu is the native context menu for one changed file. Actions
// carry the absolute path, so the same menu works after switching repository
// tabs and routes path-based backend calls to the innermost repository.
func (m *Model) vcsChangeRowMenu(menu *ui.Menu, side string, file openidea.FileStatus) {
	target := map[string]any{"path": file.Path}
	if menu.Item("Show Diff").Chosen() {
		m.ShowDiff(file.Path)
	}
	if menu.Item("Jump to Source").Chosen() {
		m.closeDiff()
		m.openFile(file.Path)
		m.invalidate()
	}
	if menu.Item("Show History").Chosen() {
		m.ShowHistoryForFile(target)
	}
	if menu.Item("Commit File…").Chosen() {
		m.OpenCommitFile(file.Path)
	}
	menu.Separator()
	if side == "staged" {
		if menu.Item("Unstage").Chosen() {
			m.UnstageChange(file)
		}
	} else {
		if menu.Item("Stage").Chosen() {
			m.StageChange(file)
		}
		if file.Unstaged != nil && *file.Unstaged == "untracked" {
			if menu.Item("Add to VCS").Chosen() {
				m.AddPathsToVCS(target)
			}
		}
		if menu.Item("Rollback…").Chosen() {
			m.AskRollback(file)
		}
	}
	if files := vcsDeletableFiles([]openidea.FileStatus{file}); len(files) > 0 {
		if menu.Item("Delete…").Chosen() {
			m.AskDeleteVCSFiles([]openidea.FileStatus{file})
		}
	}
	menu.Separator()
	menu.Submenu("Add to Ignore", func(ignore *ui.Menu) {
		if ignore.Item("Add to .gitignore").Chosen() {
			m.AddToIgnore(file.Path, "gitignore")
		}
		if ignore.Item("Add to .git/info/exclude").Chosen() {
			m.AddToIgnore(file.Path, "localExclude")
		}
	})
	if menu.Item("Shelve Changes…").Chosen() {
		m.OpenShelveChanges(target)
	}
	if menu.Item("Create Patch…").Chosen() {
		m.CreatePatchForPaths(target)
	}
	menu.Separator()
	if menu.Item("Copy Path").Chosen() {
		mygo.Clipboard.WriteText(file.Path)
		m.status = "Copied the path"
		m.invalidate()
	}
	if menu.Item("Copy File Name").Chosen() {
		mygo.Clipboard.WriteText(filepath.Base(file.RelPath))
		m.status = "Copied the file name"
		m.invalidate()
	}
}

// vcsDeletableFiles mirrors OpenIDEA's Changes view Delete action. A file
// already deleted in the working tree cannot be deleted again, including the
// staged-only deletion state.
func vcsDeletableFiles(files []openidea.FileStatus) []string {
	paths := make([]string, 0, len(files))
	seen := make(map[string]bool, len(files))
	for _, file := range files {
		path := filepath.Clean(strings.TrimSpace(file.Path))
		if path == "" || path == "." {
			return nil
		}
		if seen[path] {
			continue
		}
		if file.Unstaged != nil && *file.Unstaged == "deleted" {
			return nil
		}
		if file.Unstaged == nil && file.Staged != nil && *file.Staged == "deleted" {
			return nil
		}
		seen[path] = true
		paths = append(paths, path)
	}
	return paths
}

func normalizeVCSDeletePaths(paths []string) []string {
	clean := make([]string, 0, len(paths))
	seen := make(map[string]bool, len(paths))
	for _, path := range paths {
		path = filepath.Clean(strings.TrimSpace(path))
		if path == "" || path == "." || seen[path] {
			continue
		}
		seen[path] = true
		clean = append(clean, path)
	}
	return clean
}

func (m *Model) AskDeleteVCSFiles(files []openidea.FileStatus) {
	paths := vcsDeletableFiles(files)
	if len(paths) == 0 {
		m.status = "The selected files are already deleted"
		m.invalidate()
		return
	}
	hint := fmt.Sprintf("Delete %d items?", len(paths))
	if len(paths) == 1 {
		hint = "Delete '" + filepath.Base(paths[0]) + "'?"
	}
	hint += "\n\nItems are moved to the Trash."
	m.openVCSMore(vcsMoreConfirm, "changes.deleteFiles", "Delete", hint)
	m.vcsMore.input = strings.Join(paths, "\x00")
}

// deleteVCSFiles sends each confirmed path through OpenIDEA's fs_delete RPC.
// The backend owns the Trash move and undo data; partial failures remain
// visible without preventing successful paths from refreshing.
func (m *Model) deleteVCSFiles(paths []string) {
	paths = normalizeVCSDeletePaths(paths)
	if len(paths) == 0 {
		m.closeVCSMore()
		return
	}
	if m.ctx == nil || (m.mgr == nil && m.invokeHook == nil) {
		m.vcsMore.err = "The GoIDEA backend is not connected"
		m.invalidate()
		return
	}
	root := m.Root()
	repo := m.vcs.current
	if repo == "" {
		repo = vcsRepositoryForPath(paths[0], m.vcs.repositories)
	}
	m.closeVCSMore()
	m.setVCSRepositoryBusy(repo, true)
	m.setVCSRepositoryOpErr(repo, "")
	m.status = "Deleting…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 90*time.Second)
		defer cancel()
		deleted := make([]string, 0, len(paths))
		failures := make([]string, 0)
		for _, path := range paths {
			_, err := m.invoke(ctx, "fs_delete", map[string]any{"path": path})
			if err != nil {
				failures = append(failures, filepath.Base(path)+": "+err.Error())
				continue
			}
			deleted = append(deleted, path)
		}
		m.post(func() {
			m.setVCSRepositoryBusy(repo, false)
			if m.Root() != root {
				return
			}
			if len(deleted) > 0 {
				m.notifyVCSFileOperation("deleted", deleted)
			}
			if len(failures) > 0 {
				m.setVCSRepositoryOpErr(repo, "Could not delete some files")
				m.notifyWarning(
					"vcs.changes.delete",
					"Could not delete some files",
					strings.Join(failures, "\n"),
				)
			}
			m.RefreshVCS()
			switch {
			case len(deleted) == 1:
				m.status = "Deleted " + filepath.Base(deleted[0])
			case len(deleted) > 1:
				m.status = fmt.Sprintf("Deleted %d items", len(deleted))
			case len(failures) > 0:
				m.status = "Delete failed"
			}
			m.invalidate()
		})
	}()
}

// vcsToolWindow is the Commit tool window with OpenIDEA's multi-repository
// context: repository tabs, repository-scoped Local Changes / Log / Worktrees
// content, and actions routed through the selected repository.
func (m *Model) vcsToolWindow(c *ui.Context, p *ui.Theme, oi oiColors) {
	ui.Column(c).Grow(1).MinHeight(0).Children(func() {
		if len(m.vcs.repositories) > 1 {
			m.vcsRepositoryTabs(c, p, oi)
		}
		repo := m.currentVCSRepository()
		if repo != nil {
			m.vcsRepositorySummary(c, oi, *repo)
			m.vcsContentTabs(c, p, oi)
		}
		if m.vcs.err != "" {
			ui.Row(c).Padding(8, 8).Children(func() {
				ui.Text(c, m.vcs.err).FontSize(12).TextColor(p.Warning)
			})
			return
		}
		if repo == nil {
			ui.Column(c).Padding(12, 10).Gap(4).Children(func() {
				if m.vcs.loading || !m.vcs.loaded {
					ui.Text(c, "Discovering Git repositories…").FontSize(12).TextColor(p.TextMuted)
					return
				}
				ui.Text(c, "Version control is not configured").FontSize(13).
					Bold().TextColor(p.Text)
				ui.Text(c, "Open a Git repository to commit changes.").FontSize(12).
					TextColor(p.TextMuted)
			})
			return
		}
		m.vcsToolWindowContent(c, p, oi)
	})
}
