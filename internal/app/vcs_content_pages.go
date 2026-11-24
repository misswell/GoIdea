package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

// vcsBranchesPane is the Branches content page of one repository. Branches
// and tags load independently so a tag failure does not hide usable branch
// rows.
type vcsBranchesPane struct {
	loading bool
	loaded  bool
	query   string

	branches    []openidea.Branch
	tags        []string
	branchesErr string
	tagsErr     string
	selectedKey string
	generation  uint64
	cancel      context.CancelFunc
}

// vcsStashEntry attaches the repository that owns a stash. The Stash page is
// project-wide, matching OpenIDEA, and every operation must route back to this
// root instead of the repository currently selected above it.
type vcsStashEntry struct {
	Repo  string
	Stash openidea.Stash
}

type vcsStashPane struct {
	loading    bool
	loaded     bool
	busy       bool
	entries    []vcsStashEntry
	selected   int
	query      string
	err        string
	generation uint64
	cancel     context.CancelFunc
}

// vcsShelfPane is the project-wide shelf. Shelves predate repositories in
// OpenIDEA, so a missing Repo belongs to the first repository.
type vcsShelfPane struct {
	loading    bool
	loaded     bool
	busy       bool
	shelves    []shelvedChangeList
	selectedID string
	query      string
	err        string
	generation uint64
	cancel     context.CancelFunc
}

func (m *Model) vcsBranchesPane(root string) *vcsBranchesPane {
	root = vcsRepositoryKey(root)
	if root == "" {
		return &vcsBranchesPane{}
	}
	if m.vcs.branchPages == nil {
		m.vcs.branchPages = map[string]*vcsBranchesPane{}
	}
	page := m.vcs.branchPages[root]
	if page == nil {
		page = &vcsBranchesPane{}
		m.vcs.branchPages[root] = page
	}
	return page
}

func (m *Model) ensureVCSBranchesPage(root string) {
	page := m.vcsBranchesPane(root)
	if page != nil && !page.loaded && !page.loading {
		m.refreshVCSBranchesPage(root)
	}
}

func (m *Model) refreshVCSBranchesPage(root string) {
	root = vcsRepositoryKey(root)
	if root == "" || (m.mgr == nil && m.invokeHook == nil) {
		return
	}
	projectRoot := m.Root()
	page := m.vcsBranchesPane(root)
	if page.loading {
		return
	}
	if page.cancel != nil {
		page.cancel()
	}
	ctx, cancel := context.WithTimeout(m.mergeContext(), time.Minute)
	page.cancel = cancel
	page.generation++
	generation := page.generation
	page.loading = true
	page.branchesErr = ""
	page.tagsErr = ""
	m.invalidate()

	type result struct {
		kind     string
		branches []openidea.Branch
		tags     []string
		err      error
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		raw, err := m.invoke(ctx, "vcs_branches", map[string]any{
			"repo": vcsRepositoryArgFor(root),
		})
		var branches []openidea.Branch
		if err == nil {
			err = json.Unmarshal(raw, &branches)
		}
		results <- result{kind: "branches", branches: branches, err: err}
	}()
	go func() {
		defer wg.Done()
		raw, err := m.invoke(ctx, "vcs_tags", map[string]any{
			"repo": vcsRepositoryArgFor(root),
		})
		var tags []string
		if err == nil {
			err = json.Unmarshal(raw, &tags)
		}
		results <- result{kind: "tags", tags: tags, err: err}
	}()
	go func() {
		wg.Wait()
		close(results)
		defer cancel()
		loaded, branchErr, tagErr := result{}, "", ""
		for value := range results {
			if value.err != nil {
				if value.kind == "tags" {
					tagErr = value.err.Error()
				} else {
					branchErr = value.err.Error()
				}
				continue
			}
			if value.kind == "branches" {
				loaded.branches = value.branches
			}
			if value.kind == "tags" {
				loaded.tags = value.tags
			}
		}
		m.post(func() {
			current := m.vcs.branchPages[root]
			if current != page || generation != page.generation ||
				m.Root() != projectRoot {
				return
			}
			page.loading = false
			page.loaded = true
			page.cancel = nil
			page.branchesErr = branchErr
			page.tagsErr = tagErr
			page.branches = loaded.branches
			sort.SliceStable(page.branches, func(i, j int) bool {
				if page.branches[i].Remote != page.branches[j].Remote {
					return !page.branches[i].Remote
				}
				return page.branches[i].Name < page.branches[j].Name
			})
			page.tags = append(page.tags[:0], loaded.tags...)
			sort.Strings(page.tags)
			page.selectedKey = m.vcsValidBranchSelection(page)
			m.invalidate()
		})
	}()
}

func (m *Model) vcsValidBranchSelection(page *vcsBranchesPane) string {
	if page == nil {
		return ""
	}
	if strings.HasPrefix(page.selectedKey, "branch:") {
		name := strings.TrimPrefix(page.selectedKey, "branch:")
		for _, branch := range page.branches {
			if branch.Name == name {
				return page.selectedKey
			}
		}
	}
	if strings.HasPrefix(page.selectedKey, "tag:") {
		name := strings.TrimPrefix(page.selectedKey, "tag:")
		for _, tag := range page.tags {
			if tag == name {
				return page.selectedKey
			}
		}
	}
	for _, branch := range page.branches {
		if branch.Current {
			return "branch:" + branch.Name
		}
	}
	if len(page.branches) > 0 {
		return "branch:" + page.branches[0].Name
	}
	if len(page.tags) > 0 {
		return "tag:" + page.tags[0]
	}
	return ""
}

func (m *Model) vcsBranchPageRows(page *vcsBranchesPane) (local, remote []openidea.Branch, tags []string) {
	if page == nil {
		return nil, nil, nil
	}
	query := strings.ToLower(strings.TrimSpace(page.query))
	for _, branch := range page.branches {
		if query != "" && !strings.Contains(strings.ToLower(branch.Name), query) {
			continue
		}
		if branch.Remote {
			remote = append(remote, branch)
		} else {
			local = append(local, branch)
		}
	}
	for _, tag := range page.tags {
		if query == "" || strings.Contains(strings.ToLower(tag), query) {
			tags = append(tags, tag)
		}
	}
	return local, remote, tags
}

func (m *Model) selectVCSBranchPageRow(key string) {
	page := m.vcsBranchesPane(m.vcs.current)
	page.selectedKey = key
	m.invalidate()
}

func (m *Model) vcsBranchPageSelected(key string) bool {
	return m.vcsBranchesPane(m.vcs.current).selectedKey == key
}

func (m *Model) vcsBranchesContent(c *ui.Context, p *ui.Theme, oi oiColors) {
	root := m.vcs.current
	page := m.vcsBranchesPane(root)
	ui.Row(c).Height(32).Shrink(0).Padding(0, 6).Gap(3).AlignItems(ui.Center).
		BorderWidth(1, 0, 1, 0).BorderColor(p.Border).Children(func() {
		ui.Button(c, "New Branch…").Size(92, 24).Disabled(page.loading).
			OnClick(m.OpenNewGitBranch)
		ui.Button(c, "New Tag…").Size(76, 24).Disabled(page.loading).
			OnClick(m.OpenNewGitTag)
		ui.Button(c, "Refresh").Size(58, 24).Disabled(page.loading).
			OnClick(func() { m.refreshVCSBranchesPage(root) })
	})
	ui.Row(c).Height(34).Shrink(0).Padding(6, 8).Children(func() {
		input := ui.TextInput(c.Key("vcs:branches:filter"), &page.query).
			Grow(1).Placeholder("Filter branches and tags")
		input.OnChange(m.invalidate)
	})
	if page.branchesErr != "" {
		ui.Text(c, "Could not read branches: "+page.branchesErr).Padding(8).
			FontSize(12).TextColor(p.Danger).SingleLine().Ellipsis("…")
	}
	if page.tagsErr != "" {
		ui.Text(c, "Could not read tags: "+page.tagsErr).Padding(8).
			FontSize(12).TextColor(p.Warning).SingleLine().Ellipsis("…")
	}
	if page.loading && !page.loaded {
		ui.Text(c, "Loading branches and tags…").Padding(8).FontSize(12).
			TextColor(p.TextMuted)
		return
	}
	local, remote, tags := m.vcsBranchPageRows(page)
	if len(local) == 0 && len(remote) == 0 && len(tags) == 0 {
		text := "No branches or tags"
		if strings.TrimSpace(page.query) != "" {
			text = "No branches or tags match the filter"
		}
		ui.Text(c, text).Padding(8).FontSize(12).TextColor(p.TextMuted)
		return
	}
	ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Padding(2, 0).Children(func() {
			if len(local) > 0 {
				m.vcsBranchGroupHeader(c, p, "Local Branches", len(local))
				for _, branch := range local {
					m.vcsBranchPageBranchRow(c, p, oi, branch)
				}
			}
			if len(remote) > 0 {
				m.vcsBranchGroupHeader(c, p, "Remote Branches", len(remote))
				for _, branch := range remote {
					m.vcsBranchPageBranchRow(c, p, oi, branch)
				}
			}
			if len(tags) > 0 {
				m.vcsBranchGroupHeader(c, p, "Tags", len(tags))
				for _, tag := range tags {
					m.vcsBranchPageTagRow(c, p, oi, tag)
				}
			}
		})
	})
}

func (m *Model) vcsBranchGroupHeader(c *ui.Context, p *ui.Theme, label string, count int) {
	ui.Row(c).Height(26).Padding(0, 8).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).FontSize(11).Bold().TextColor(p.TextMuted)
		ui.Text(c, fmt.Sprint(count)).FontSize(10).TextColor(p.TextMuted)
	})
}

func (m *Model) vcsBranchPageBranchRow(
	c *ui.Context,
	p *ui.Theme,
	oi oiColors,
	branch openidea.Branch,
) {
	key := "branch:" + branch.Name
	row := ui.Row(c.Key("vcs:branches:"+m.vcs.current+":"+key)).Height(28).
		Padding(0, 10).Gap(6).Radius(5).AlignItems(ui.Center)
	if m.vcsBranchPageSelected(key) {
		row.Background(p.Selection)
	} else if row.Hovered() {
		row.Background(oi.hoverTint)
	}
	row.Tooltip(branch.Name)
	row.Children(func() {
		icon := "general/vcs"
		if branch.Remote {
			icon = "toolwindows/services"
		}
		oiIconEl(c, icon, 14, oi, oi.textMuted)
		if m.branchFavorite(branch.Name) {
			ui.Text(c, "★").FontSize(11).TextColor(p.Accent).SingleLine()
		}
		ui.Text(c, branch.Name).FontSize(12).
			TextColor(boolColor(branch.Remote, p.TextMuted, oi.textStrong)).
			SingleLine().Ellipsis("…")
		ui.Spacer(c)
		if branch.Current {
			ui.Text(c, "Current").FontSize(10).TextColor(p.Accent).SingleLine()
		}
		if branch.Behind > 0 {
			ui.Text(c, fmt.Sprintf("↓%d", branch.Behind)).FontSize(10).
				TextColor(p.TextMuted).SingleLine()
		}
		if branch.Ahead > 0 {
			ui.Text(c, fmt.Sprintf("↑%d", branch.Ahead)).FontSize(10).
				TextColor(p.TextMuted).SingleLine()
		}
	})
	row.ContextMenu(func(menu *ui.Menu) { m.vcsBranchPageMenu(menu, branch) })
	if row.Clicked() {
		m.selectVCSBranchPageRow(key)
	}
	if row.DoubleClicked() {
		m.selectVCSBranchPageRow(key)
		if !branch.Current {
			m.checkoutBranch(branch.Name)
		}
	}
}

func (m *Model) vcsBranchPageTagRow(
	c *ui.Context,
	p *ui.Theme,
	oi oiColors,
	tag string,
) {
	key := "tag:" + tag
	row := ui.Row(c.Key("vcs:branches:"+m.vcs.current+":"+key)).Height(28).
		Padding(0, 10).Gap(6).Radius(5).AlignItems(ui.Center)
	if m.vcsBranchPageSelected(key) {
		row.Background(p.Selection)
	} else if row.Hovered() {
		row.Background(oi.hoverTint)
	}
	row.Tooltip(tag)
	row.Children(func() {
		oiIconEl(c, "general/vcs", 14, oi, oi.textSubtle)
		ui.Text(c, tag).FontSize(12).TextColor(oi.textStrong).
			SingleLine().Ellipsis("…")
	})
	row.ContextMenu(func(menu *ui.Menu) { m.vcsTagPageMenu(menu, tag) })
	if row.Clicked() {
		m.selectVCSBranchPageRow(key)
	}
	if row.DoubleClicked() {
		m.selectVCSBranchPageRow(key)
		m.OpenCheckoutRevision(tag)
	}
}

func (m *Model) vcsBranchPageMenu(menu *ui.Menu, branch openidea.Branch) {
	if !branch.Current && menu.Item("Checkout").Chosen() {
		m.checkoutBranch(branch.Name)
	}
	if menu.Item("New Branch…").Chosen() {
		m.OpenNewGitBranchAt(branch.Name)
	}
	if menu.Item("Show History").Chosen() {
		m.ShowLogWithArg(map[string]any{"branch": branch.Name, "repo": m.vcs.current})
	}
	if !branch.Current {
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
	favorite := m.branchFavorite(branch.Name)
	if menu.Item("Mark/Unmark As Favorite").Checked(favorite).Chosen() {
		m.toggleBranchFavorite(branch.Name)
	}
}

func (m *Model) vcsTagPageMenu(menu *ui.Menu, tag string) {
	if menu.Item("Checkout Tag").Chosen() {
		m.OpenCheckoutRevision(tag)
	}
	if menu.Item("New Branch from Tag…").Chosen() {
		m.OpenNewGitBranchAt(tag)
	}
	if menu.Item("Show History").Chosen() {
		m.ShowLogWithArg(map[string]any{"branch": tag, "repo": m.vcs.current})
	}
	if menu.Item("Compare with Current").Chosen() {
		m.compareRevisions(tag, "HEAD")
	}
	if menu.Item("Show Diff with Working Tree").Chosen() {
		m.showDiffWithWorkingTreeRevision(tag, tag)
	}
	menu.Separator()
	if menu.Item("Delete Tag…").Chosen() {
		m.askDeleteVCSTag(tag)
	}
}

func (m *Model) vcsShelfPageState() *vcsShelfPane {
	if m.vcs.shelf == nil {
		m.vcs.shelf = &vcsShelfPane{}
	}
	return m.vcs.shelf
}

func (m *Model) ensureVCSShelfPage() {
	page := m.vcsShelfPageState()
	if !page.loaded && !page.loading {
		m.refreshVCSShelfPage()
	}
}

func (m *Model) refreshVCSShelfPage() {
	if m.mgr == nil && m.invokeHook == nil {
		return
	}
	projectRoot := m.Root()
	page := m.vcsShelfPageState()
	if page.loading {
		return
	}
	if page.cancel != nil {
		page.cancel()
	}
	ctx, cancel := context.WithTimeout(m.mergeContext(), time.Minute)
	page.cancel = cancel
	page.generation++
	generation := page.generation
	page.loading = true
	page.err = ""
	m.invalidate()
	go func() {
		defer cancel()
		raw, err := m.invoke(ctx, "vcs_shelves", nil)
		var shelves []shelvedChangeList
		if err == nil {
			err = json.Unmarshal(raw, &shelves)
		}
		m.post(func() {
			if m.vcs.shelf != page || page.generation != generation ||
				m.Root() != projectRoot {
				return
			}
			page.loading = false
			page.loaded = true
			page.cancel = nil
			if err != nil {
				page.err = "Could not read the shelf: " + err.Error()
				page.shelves = nil
				page.selectedID = ""
				m.invalidate()
				return
			}
			page.shelves = shelves
			if _, ok := m.selectedVCSShelf(); !ok {
				page.selectedID = ""
				if len(page.shelves) > 0 {
					page.selectedID = page.shelves[0].ID
				}
			}
			m.invalidate()
		})
	}()
}

func (m *Model) vcsShelfCount() int {
	if m.vcs.shelf == nil {
		return 0
	}
	return len(m.vcs.shelf.shelves)
}

func (m *Model) vcsStashCount() int {
	if m.vcs.stashes == nil {
		return 0
	}
	return len(m.vcs.stashes.entries)
}

func (m *Model) vcsRepositoryByRoot(root string) *vcsRepository {
	root = vcsRepositoryKey(root)
	for i := range m.vcs.repositories {
		if vcsRepositoryKey(m.vcs.repositories[i].Root) == root {
			return &m.vcs.repositories[i]
		}
	}
	return nil
}

func (m *Model) vcsRepositoryLabelForRoot(root string) string {
	if repo := m.vcsRepositoryByRoot(root); repo != nil {
		return m.vcsRepositoryLabel(*repo)
	}
	if name := filepath.Base(vcsRepositoryKey(root)); name != "" && name != "." {
		return name
	}
	return root
}

func (m *Model) vcsShelfRepository(shelf shelvedChangeList) string {
	if shelf.Repo != nil {
		if root := vcsRepositoryKey(*shelf.Repo); root != "" {
			return root
		}
	}
	if len(m.vcs.repositories) > 0 {
		return vcsRepositoryKey(m.vcs.repositories[0].Root)
	}
	if root := m.vcsRepositoryRoot(); root != "" {
		return root
	}
	return vcsRepositoryKey(m.Root())
}

func (m *Model) vcsShelfRepositoryLabel(shelf shelvedChangeList) string {
	return m.vcsRepositoryLabelForRoot(m.vcsShelfRepository(shelf))
}

func (m *Model) vcsFilteredShelves() []shelvedChangeList {
	page := m.vcsShelfPageState()
	query := strings.ToLower(strings.TrimSpace(page.query))
	if query == "" {
		return page.shelves
	}
	out := make([]shelvedChangeList, 0, len(page.shelves))
	for _, shelf := range page.shelves {
		haystack := strings.ToLower(strings.Join([]string{
			shelf.Name,
			m.vcsShelfRepositoryLabel(shelf),
			vcsShelfBranch(shelf),
		}, " "))
		if strings.Contains(haystack, query) {
			out = append(out, shelf)
		}
	}
	return out
}

func vcsShelfBranch(shelf shelvedChangeList) string {
	if shelf.Branch != nil {
		return *shelf.Branch
	}
	return ""
}

func (m *Model) selectedVCSShelf() (shelvedChangeList, bool) {
	page := m.vcsShelfPageState()
	if page.selectedID != "" {
		for _, shelf := range page.shelves {
			if shelf.ID == page.selectedID {
				return shelf, true
			}
		}
	}
	rows := m.vcsFilteredShelves()
	if len(rows) == 0 {
		return shelvedChangeList{}, false
	}
	return rows[0], true
}

func (m *Model) selectVCSShelf(id string) {
	page := m.vcsShelfPageState()
	page.selectedID = id
	m.invalidate()
}

func (m *Model) vcsShelfContent(c *ui.Context, p *ui.Theme, oi oiColors) {
	page := m.vcsShelfPageState()
	selected, hasSelected := m.selectedVCSShelf()
	busy := page.loading || page.busy
	ui.Row(c).Height(32).Shrink(0).Padding(0, 6).Gap(3).AlignItems(ui.Center).
		BorderWidth(1, 0, 1, 0).BorderColor(p.Border).Children(func() {
		ui.Button(c, "Unshelve…").Size(76, 24).Disabled(!hasSelected || busy).
			OnClick(func() { m.UnshelveArg(map[string]any{"id": selected.ID}) })
		ui.Button(c, "Show Diff").Size(68, 24).Disabled(!hasSelected || busy).
			OnClick(func() { m.ShowShelfDiffArg(map[string]any{"id": selected.ID}, false) })
		ui.Button(c, "Rename…").Size(62, 24).Disabled(!hasSelected || busy).
			OnClick(func() { m.ShelfRenameArg(map[string]any{"id": selected.ID}) })
		ui.Button(c, "Delete").Size(52, 24).Disabled(!hasSelected || busy).
			OnClick(func() { m.ShelfDeleteArg(map[string]any{"id": selected.ID}) })
		ui.Button(c, "Refresh").Size(58, 24).Disabled(page.loading).
			OnClick(m.refreshVCSShelfPage)
	})
	ui.Row(c).Height(34).Shrink(0).Padding(6, 8).Gap(4).Children(func() {
		input := ui.TextInput(c.Key("vcs:shelf:filter"), &page.query).
			Grow(1).Placeholder("Filter shelved changes")
		input.OnChange(m.invalidate)
		ui.Button(c, "Import Patches…").Size(108, 24).Disabled(page.loading).
			OnClick(func() { m.ImportShelfPatches(nil) })
	})
	if page.err != "" {
		ui.Text(c, page.err).Padding(8).FontSize(12).TextColor(p.Danger).
			SingleLine().Ellipsis("…")
		return
	}
	if page.loading && !page.loaded {
		ui.Text(c, "Loading shelved changes…").Padding(8).FontSize(12).
			TextColor(p.TextMuted)
		return
	}
	rows := m.vcsFilteredShelves()
	if len(rows) == 0 {
		text := "No shelved changes"
		if strings.TrimSpace(page.query) != "" {
			text = "No shelved changes match the filter"
		}
		ui.Text(c, text).Padding(8).FontSize(12).TextColor(p.TextMuted)
		return
	}
	ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Padding(2, 0).Children(func() {
			for _, shelf := range rows {
				shelf := shelf
				row := ui.Row(c.Key("vcs:shelf:"+shelf.ID)).Height(44).
					Padding(0, 8).Gap(7).AlignItems(ui.Center)
				if shelf.ID == page.selectedID {
					row.Background(p.Selection)
				} else if row.Hovered() {
					row.Background(oi.hoverTint)
				}
				row.Tooltip(shelf.Name)
				row.Children(func() {
					oiIconEl(c, "general/vcs", 14, oi, oi.textMuted)
					ui.Column(c).Gap(1).Grow(1).MinWidth(0).Children(func() {
						ui.Text(c, shelf.Name).FontSize(12).TextColor(oi.textStrong).
							SingleLine().Ellipsis("…")
						detail := m.vcsShelfRepositoryLabel(shelf)
						if branch := vcsShelfBranch(shelf); branch != "" {
							detail += " · " + branch
						}
						if len(shelf.Files) > 0 {
							detail += " · " + count(len(shelf.Files), "file", "files")
						}
						ui.Text(c, detail).FontSize(10).TextColor(oi.textSubtle).
							SingleLine().Ellipsis("…")
					})
				})
				row.ContextMenu(func(menu *ui.Menu) { m.vcsShelfRowMenu(menu, shelf) })
				if row.Clicked() {
					m.selectVCSShelf(shelf.ID)
				}
				if row.DoubleClicked() {
					m.selectVCSShelf(shelf.ID)
					m.UnshelveArg(map[string]any{"id": shelf.ID})
				}
			}
		})
	})
}

func (m *Model) vcsShelfRowMenu(menu *ui.Menu, shelf shelvedChangeList) {
	target := map[string]any{"id": shelf.ID}
	if menu.Item("Unshelve…").Chosen() {
		m.UnshelveArg(target)
	}
	if menu.Item("Show Diff").Chosen() {
		m.ShowShelfDiffArg(target, false)
	}
	if menu.Item("Compare with Local").Chosen() {
		m.ShowShelfDiffArg(target, true)
	}
	menu.Separator()
	if menu.Item("Rename…").Chosen() {
		m.ShelfRenameArg(target)
	}
	if menu.Item("Create Patch…").Chosen() {
		m.CreateShelfPatchArg(target)
	}
	if menu.Item("Copy Patch").Chosen() {
		m.CopyShelfPatchArg(target)
	}
	menu.Separator()
	if menu.Item("Delete").Chosen() {
		m.ShelfDeleteArg(target)
	}
}

func (m *Model) vcsStashPageState() *vcsStashPane {
	if m.vcs.stashes == nil {
		m.vcs.stashes = &vcsStashPane{}
	}
	return m.vcs.stashes
}

func (m *Model) ensureVCSStashPage() {
	page := m.vcsStashPageState()
	if !page.loaded && !page.loading {
		m.refreshVCSStashPage()
	}
}

func (m *Model) refreshVCSStashPage() {
	if m.mgr == nil && m.invokeHook == nil {
		return
	}
	projectRoot := m.Root()
	page := m.vcsStashPageState()
	if page.loading {
		return
	}
	if page.cancel != nil {
		page.cancel()
	}
	roots := make([]string, 0, len(m.vcs.repositories)+1)
	for _, repo := range m.vcs.repositories {
		if root := vcsRepositoryKey(repo.Root); root != "" {
			roots = append(roots, root)
		}
	}
	if len(roots) == 0 {
		if root := m.vcsRepositoryRoot(); root != "" {
			roots = append(roots, root)
		}
	}
	ctx, cancel := context.WithTimeout(m.mergeContext(), time.Minute)
	page.cancel = cancel
	page.generation++
	generation := page.generation
	page.loading = true
	page.err = ""
	if len(roots) == 0 {
		page.loading = false
		page.loaded = true
		page.entries = nil
		page.selected = 0
		cancel()
		m.invalidate()
		return
	}
	m.invalidate()
	type result struct {
		stashes []openidea.Stash
		err     error
	}
	results := make([]result, len(roots))
	var wg sync.WaitGroup
	wg.Add(len(roots))
	for i, root := range roots {
		i, root := i, root
		go func() {
			defer wg.Done()
			raw, err := m.invoke(ctx, "vcs_stashes", map[string]any{
				"repo": vcsRepositoryArgFor(root),
			})
			var stashes []openidea.Stash
			if err == nil {
				err = json.Unmarshal(raw, &stashes)
			}
			results[i] = result{stashes: stashes, err: err}
		}()
	}
	go func() {
		wg.Wait()
		defer cancel()
		m.post(func() {
			if m.vcs.stashes != page || page.generation != generation ||
				m.Root() != projectRoot {
				return
			}
			page.loading = false
			page.loaded = true
			page.cancel = nil
			entries := make([]vcsStashEntry, 0)
			for i, root := range roots {
				if results[i].err != nil {
					page.err = fmt.Sprintf("Could not read stashes for %s: %v",
						m.vcsRepositoryLabelForRoot(root), results[i].err)
					page.entries = nil
					page.selected = 0
					m.invalidate()
					return
				}
				for _, stash := range results[i].stashes {
					entries = append(entries, vcsStashEntry{Repo: root, Stash: stash})
				}
			}
			sort.SliceStable(entries, func(i, j int) bool {
				if entries[i].Repo != entries[j].Repo {
					left, right := -1, -1
					for index, root := range roots {
						if root == entries[i].Repo {
							left = index
						}
						if root == entries[j].Repo {
							right = index
						}
					}
					return left < right
				}
				return entries[i].Stash.Index < entries[j].Stash.Index
			})
			page.entries = entries
			page.selected = clampSymbolSel(page.selected, len(entries))
			m.invalidate()
		})
	}()
}

func (m *Model) vcsFilteredStashes() []vcsStashEntry {
	page := m.vcsStashPageState()
	query := strings.ToLower(strings.TrimSpace(page.query))
	if query == "" {
		return page.entries
	}
	out := make([]vcsStashEntry, 0, len(page.entries))
	for _, entry := range page.entries {
		if strings.Contains(strings.ToLower(entry.Stash.Message+" "+
			m.vcsRepositoryLabelForRoot(entry.Repo)), query) {
			out = append(out, entry)
		}
	}
	return out
}

func (m *Model) selectedVCSStash() (vcsStashEntry, bool) {
	page := m.vcsStashPageState()
	rows := m.vcsFilteredStashes()
	if page.selected < 0 || page.selected >= len(rows) {
		return vcsStashEntry{}, false
	}
	return rows[page.selected], true
}

func (m *Model) selectVCSStash(index int) {
	page := m.vcsStashPageState()
	page.selected = clampSymbolSel(index, len(m.vcsFilteredStashes()))
	m.invalidate()
}

func (m *Model) vcsStashContent(c *ui.Context, p *ui.Theme, oi oiColors) {
	page := m.vcsStashPageState()
	selected, hasSelected := m.selectedVCSStash()
	busy := page.loading || page.busy
	ui.Row(c).Height(32).Shrink(0).Padding(0, 6).Gap(3).AlignItems(ui.Center).
		BorderWidth(1, 0, 1, 0).BorderColor(p.Border).Children(func() {
		ui.Button(c, "Apply").Size(50, 24).Disabled(!hasSelected || busy).
			OnClick(func() { m.applyVCSStash(selected, false) })
		ui.Button(c, "Pop").Size(42, 24).Disabled(!hasSelected || busy).
			OnClick(func() { m.applyVCSStash(selected, true) })
		ui.Button(c, "Unstash…").Size(72, 24).Disabled(page.loading || page.busy).
			OnClick(m.OpenGitUnstash)
		ui.Button(c, "Drop").Size(48, 24).Disabled(!hasSelected || busy).
			OnClick(func() { m.askDropVCSStash(selected) })
		ui.Button(c, "Stash Changes…").Size(104, 24).Disabled(page.loading || page.busy).
			OnClick(m.OpenGitStash)
		ui.Button(c, "Refresh").Size(58, 24).Disabled(page.loading).
			OnClick(m.refreshVCSStashPage)
	})
	ui.Row(c).Height(34).Shrink(0).Padding(6, 8).Children(func() {
		input := ui.TextInput(c.Key("vcs:stash:filter"), &page.query).
			Grow(1).Placeholder("Filter stashes")
		input.OnChange(func() {
			page.selected = 0
			m.invalidate()
		})
	})
	if page.err != "" {
		ui.Text(c, page.err).Padding(8).FontSize(12).TextColor(p.Danger).
			SingleLine().Ellipsis("…")
		return
	}
	if page.loading && !page.loaded {
		ui.Text(c, "Loading stashes…").Padding(8).FontSize(12).TextColor(p.TextMuted)
		return
	}
	rows := m.vcsFilteredStashes()
	if len(rows) == 0 {
		text := "No stashes"
		if strings.TrimSpace(page.query) != "" {
			text = "No stashes match the filter"
		}
		ui.Text(c, text).Padding(8).FontSize(12).TextColor(p.TextMuted)
		return
	}
	ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Padding(2, 0).Children(func() {
			for i, entry := range rows {
				i, entry := i, entry
				row := ui.Row(c.Key(fmt.Sprintf("vcs:stash:%s:%d", entry.Repo, entry.Stash.Index))).
					Height(38).Padding(0, 8).Gap(7).AlignItems(ui.Center)
				if i == page.selected {
					row.Background(p.Selection)
				} else if row.Hovered() {
					row.Background(oi.hoverTint)
				}
				row.Tooltip(fmt.Sprintf("stash@{%d}: %s", entry.Stash.Index, entry.Stash.Message))
				row.Children(func() {
					oiIconEl(c, "general/vcs", 14, oi, oi.textMuted)
					ui.Column(c).Gap(1).Grow(1).MinWidth(0).Children(func() {
						branch, message := vcsStashLabel(entry.Stash.Message)
						ui.Text(c, message).FontSize(12).TextColor(oi.textStrong).
							SingleLine().Ellipsis("…")
						detail := fmt.Sprintf("%s · #%d", m.vcsRepositoryLabelForRoot(entry.Repo),
							entry.Stash.Index)
						if branch != "" {
							detail += " · " + branch
						}
						ui.Text(c, detail).FontSize(10).TextColor(oi.textSubtle).
							SingleLine().Ellipsis("…")
					})
				})
				row.ContextMenu(func(menu *ui.Menu) { m.vcsStashRowMenu(menu, entry) })
				if row.Clicked() {
					m.selectVCSStash(i)
				}
				if row.DoubleClicked() {
					m.selectVCSStash(i)
					m.applyVCSStash(entry, false)
				}
			}
		})
	})
}

func vcsStashLabel(message string) (branch, text string) {
	text = strings.TrimSpace(message)
	if !strings.HasPrefix(text, "On ") {
		return "", text
	}
	if index := strings.Index(text, ": "); index > len("On ") {
		return text[len("On "):index], text[index+2:]
	}
	return "", text
}

func (m *Model) vcsStashRowMenu(menu *ui.Menu, entry vcsStashEntry) {
	if menu.Item("Apply Stash").Chosen() {
		m.applyVCSStash(entry, false)
	}
	if menu.Item("Pop Stash").Chosen() {
		m.applyVCSStash(entry, true)
	}
	if menu.Item("Unstash…").Chosen() {
		m.SelectVCSRepository(entry.Repo)
		m.OpenGitUnstash()
	}
	menu.Separator()
	if menu.Item("Drop Stash…").Chosen() {
		m.askDropVCSStash(entry)
	}
}

func (m *Model) applyVCSStash(entry vcsStashEntry, pop bool) {
	page := m.vcsStashPageState()
	if page.busy || entry.Repo == "" {
		return
	}
	projectRoot := m.Root()
	page.busy = true
	page.err = ""
	action := "Apply"
	if pop {
		action = "Pop"
	}
	m.status = fmt.Sprintf("%s stash #%d…", action, entry.Stash.Index)
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.mergeContext(), 5*time.Minute)
		defer cancel()
		_, err := m.invoke(ctx, "vcs_stash_apply", map[string]any{
			"repo": vcsRepositoryArgFor(entry.Repo), "index": int(entry.Stash.Index),
			"pop": pop, "reinstateIndex": false, "branch": nil,
		})
		m.post(func() {
			if m.vcs.stashes != page || m.Root() != projectRoot {
				return
			}
			page.busy = false
			if err != nil {
				page.err = action + " stash failed: " + err.Error()
				m.invalidate()
				return
			}
			done := "Applied"
			if pop {
				done = "Popped"
			}
			m.status = fmt.Sprintf("%s stash #%d", done, entry.Stash.Index)
			m.RefreshAfterVCSCommand()
			m.refreshVCSStashPage()
		})
	}()
}

func (m *Model) askDropVCSStash(entry vcsStashEntry) {
	_, message := vcsStashLabel(entry.Stash.Message)
	if message == "" {
		message = fmt.Sprintf("Stash #%d", entry.Stash.Index)
	}
	m.vcsMore = vcsMoreState{
		open: true, kind: vcsMoreConfirm, action: "stash.drop",
		title: "Drop Stash",
		hint: "Drop “" + message + "” from " +
			m.vcsRepositoryLabelForRoot(entry.Repo) + "? This cannot be undone.",
		input: entry.Repo, input2: fmt.Sprint(entry.Stash.Index),
	}
	m.invalidate()
}

func (m *Model) dropVCSStash(root, indexPath string) {
	page := m.vcsStashPageState()
	index := 0
	if _, err := fmt.Sscan(indexPath, &index); err != nil || root == "" ||
		page.busy {
		return
	}
	projectRoot := m.Root()
	page.busy = true
	page.err = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, time.Minute)
		defer cancel()
		_, err := m.invoke(ctx, "vcs_stash_drop", map[string]any{
			"repo": vcsRepositoryArgFor(root), "index": index,
		})
		m.post(func() {
			if m.vcs.stashes != page || m.Root() != projectRoot {
				return
			}
			page.busy = false
			if err != nil {
				page.err = "Drop stash failed: " + err.Error()
				m.invalidate()
				return
			}
			m.closeVCSMore()
			m.status = fmt.Sprintf("Dropped stash #%d", index)
			m.refreshVCSStashPage()
		})
	}()
}

func (m *Model) askDeleteVCSTag(tag string) {
	if tag == "" {
		return
	}
	m.vcsMore = vcsMoreState{
		open: true, kind: vcsMoreConfirm, action: "tag.delete",
		title: "Delete Tag",
		hint:  "Delete the local tag “" + tag + "”?",
		input: tag, input2: m.vcs.current,
	}
	m.invalidate()
}

func (m *Model) deleteVCSTag(tag, root string) {
	if tag == "" || root == "" {
		return
	}
	projectRoot := m.Root()
	go func() {
		ctx, cancel := context.WithTimeout(m.mergeContext(), time.Minute)
		defer cancel()
		_, err := m.invoke(ctx, "vcs_delete_tag", map[string]any{
			"repo": vcsRepositoryArgFor(root), "name": tag,
		})
		m.post(func() {
			if m.Root() != projectRoot {
				return
			}
			if err != nil {
				m.vcsMore.err = "Could not delete the tag: " + err.Error()
				m.invalidate()
				return
			}
			m.closeVCSMore()
			m.status = "Deleted tag " + tag
			m.refreshVCSBranchesPage(root)
		})
	}()
}

// refreshLoadedVCSContentPages keeps the Git window's dedicated pages and tab
// counts current after refs, shelves or stashes change. Pages that have never
// been opened stay lazy.
func (m *Model) refreshLoadedVCSContentPages() {
	root := m.vcs.current
	if page := m.vcs.branchPages[root]; page != nil &&
		(page.loaded || m.currentVCSContent() == vcsContentBranches) {
		m.refreshVCSBranchesPage(root)
	}
	if m.vcs.shelf != nil &&
		(m.vcs.shelf.loaded || m.currentVCSContent() == vcsContentShelf) {
		m.refreshVCSShelfPage()
	}
	if m.vcs.stashes != nil &&
		(m.vcs.stashes.loaded || m.currentVCSContent() == vcsContentStash) {
		m.refreshVCSStashPage()
	}
}

// finishVCSShelfMutation refreshes the inline Shelf page when that is where
// the action started. Other entry points keep their existing modal behavior.
func (m *Model) finishVCSShelfMutation() {
	if m.currentVCSContent() == vcsContentShelf {
		m.closeVCSMore()
		m.refreshVCSShelfPage()
		return
	}
	m.LoadVCSShelf()
}
