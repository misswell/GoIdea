package app

// Call and type hierarchy after OpenIDEA's code-insight feature: a right-anchored
// tool window whose levels load from the language server as they expand. A call
// node's children are its callers or callees; a type node's are its supertypes
// or subtypes. A node that repeats an ancestor is kept a leaf, as upstream's
// recursion mark does.

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

const hierarchyRoot = "root"

const (
	hierarchyToolbarHeight     float32 = 32
	hierarchyRowHeight         float32 = 24
	hierarchyTabHeight         float32 = 24
	hierarchyToolButton        float32 = 20
	hierarchyToolbarGap        float32 = 2
	hierarchyHeaderSidePadding float32 = 6
	hierarchyBodyInset         float32 = 24
)

// hierarchyItem is one HierarchyItem of the language server's answer.
type hierarchyItem struct {
	Name           string                  `json:"name"`
	Kind           string                  `json:"kind"`
	Detail         *string                 `json:"detail"`
	Path           string                  `json:"path"`
	URI            *string                 `json:"uri"`
	Range          openidea.OutlineRange   `json:"range"`
	SelectionRange openidea.OutlineRange   `json:"selectionRange"`
	FromRanges     []openidea.OutlineRange `json:"fromRanges"`
}

// hierarchyNode is one row of the tree with its parent and depth.
type hierarchyNode struct {
	id     string
	parent string
	item   hierarchyItem
	depth  int
	// recursion marks a node repeating an ancestor: it stays a leaf.
	recursion bool
}

// hierarchyPane is the Hierarchy window's state.
type hierarchyPane struct {
	// mode is "call" or "type"; direction "incoming" or "outgoing".
	mode      string
	direction string
	rootName  string
	rootPath  string
	rootLine  int
	rootCol   int
	// children maps a parent id ("root" for the top level) to its rows.
	children map[string][]hierarchyNode
	expanded map[string]bool
	loading  map[string]bool
	failed   map[string]string
	notes    map[string]string
	gen      uint64
	err      string
	selected string
	// loadedRoot marks that a top level was asked for.
	loadedRoot bool
}

// resetHierarchy drops the tree, as switching project does.
func (m *Model) resetHierarchy() {
	m.hierarchy = hierarchyPane{}
}

func (h *hierarchyPane) active() bool {
	return h.rootPath != "" || len(h.children) > 0
}

// OpenCallHierarchy is Navigate › Call Hierarchy: the caret's callers or
// callees in the right window.
func (m *Model) OpenCallHierarchy() { m.openHierarchy("call") }

// OpenTypeHierarchy is Navigate › Type Hierarchy.
func (m *Model) OpenTypeHierarchy() { m.openHierarchy("type") }

// openHierarchy starts a query at the caret and shows the right window.
func (m *Model) openHierarchy(mode string) {
	t := m.tab()
	if t == nil {
		m.status = "Hierarchy needs an open file"
		m.invalidate()
		return
	}
	offset := t.state.Cursor()
	pos := t.doc.UTF16Pos(offset)
	direction := "incoming"
	if mode == "type" {
		direction = "outgoing"
	}
	if m.hierarchy.active() && m.hierarchy.mode == mode &&
		(m.hierarchy.direction == "incoming" || m.hierarchy.direction == "outgoing") {
		direction = m.hierarchy.direction
	}
	m.hierarchy = hierarchyPane{
		mode:      mode,
		direction: direction,
		rootName:  wordAtCaret(t.doc.Text(), offset),
		rootPath:  realPath(t.doc.Path()),
		rootLine:  int(pos.Line),
		rootCol:   int(pos.Col),
		children:  map[string][]hierarchyNode{},
		expanded:  map[string]bool{hierarchyRoot: true},
		loading:   map[string]bool{},
		failed:    map[string]string{},
		notes:     map[string]string{},
		gen:       1,
	}
	m.DockShow("hierarchy")
	m.loadHierarchy(hierarchyRoot, t, openidea.OutlinePos{Line: uint32(m.hierarchy.rootLine), Col: uint32(m.hierarchy.rootCol)})
	m.invalidate()
}

// wordAtCaret is the identifier the caret is in or beside, for the title.
func wordAtCaret(text string, offset int) string {
	if offset < 0 || offset > len(text) {
		offset = len(text)
	}
	isWord := func(b byte) bool {
		return b == '_' || b == '$' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
	}
	start := offset
	for start > 0 && isWord(text[start-1]) {
		start--
	}
	end := offset
	for end < len(text) && isWord(text[end]) {
		end++
	}
	return text[start:end]
}

// loadHierarchy asks the language server for one level and stores it.
func (m *Model) loadHierarchy(parent string, t *Tab, pos openidea.OutlinePos) {
	if m.mgr == nil || t == nil {
		return
	}
	gen := m.hierarchy.gen
	mode := m.hierarchy.mode
	direction := m.hierarchy.direction
	delete(m.hierarchy.failed, parent)
	delete(m.hierarchy.notes, parent)
	m.hierarchy.loading[parent] = true
	m.invalidate()
	docID := t.doc.ID()
	path := t.doc.Path()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
		defer cancel()
		items, incomplete, err := m.requestHierarchy(ctx, mode, docID, pos, direction)
		m.post(func() {
			if m.hierarchy.gen != gen {
				return
			}
			delete(m.hierarchy.loading, parent)
			if err != nil {
				m.hierarchy.failed[parent] = err.Error()
				m.invalidate()
				return
			}
			m.hierarchy.children[parent] = m.hierarchyNodes(parent, path, items)
			if parent == hierarchyRoot {
				m.hierarchy.loadedRoot = true
			}
			if incomplete != "" {
				if m.hierarchy.notes == nil {
					m.hierarchy.notes = map[string]string{}
				}
				m.hierarchy.notes[parent] = incomplete
				m.status = incomplete
			}
			m.invalidate()
		})
	}()
}

// requestHierarchy invokes the RPC of the mode and returns its items.
func (m *Model) requestHierarchy(ctx context.Context, mode string, docID uint32, pos openidea.OutlinePos, direction string) ([]hierarchyItem, string, error) {
	if mode == "type" {
		raw, err := m.mgr.Invoke(ctx, Window, "lsp_type_hierarchy", map[string]any{
			"doc": docID, "pos": lspPos(pos), "direction": direction,
		})
		if err != nil {
			return nil, "", err
		}
		var items []hierarchyItem
		if err := json.Unmarshal(raw, &items); err != nil {
			return nil, "", err
		}
		return items, "", nil
	}
	raw, err := m.mgr.Invoke(ctx, Window, "lsp_call_hierarchy", map[string]any{
		"request": m.nextRequest(), "doc": docID, "pos": lspPos(pos), "direction": direction,
	})
	if err != nil {
		return nil, "", err
	}
	var answer struct {
		Items      []hierarchyItem `json:"items"`
		Incomplete *struct {
			Message string `json:"message"`
		} `json:"incomplete"`
	}
	if err := json.Unmarshal(raw, &answer); err != nil {
		return nil, "", err
	}
	if answer.Incomplete != nil && answer.Incomplete.Message != "" {
		return answer.Items, answer.Incomplete.Message, nil
	}
	return answer.Items, "", nil
}

// hierarchyNodes turns one level's items into rows, marking a row that repeats
// an ancestor of its parent as a recursion leaf.
func (m *Model) hierarchyNodes(parent, path string, items []hierarchyItem) []hierarchyNode {
	ancestors := map[string]bool{}
	if parent != hierarchyRoot {
		var walk func(id string)
		walk = func(id string) {
			if id == hierarchyRoot || id == "" || ancestors[id] {
				return
			}
			ancestors[id] = true
			walk(hierarchyParent(id))
		}
		walk(parent)
	}
	ancestors[hierarchyKey(path, m.hierarchy.rootLine, m.hierarchy.rootCol, m.hierarchy.rootName)] = true
	depth := 0
	if parent != hierarchyRoot {
		depth = m.hierarchyDepth(parent) + 1
	} else {
		depth = 1
	}
	seen := map[string]bool{}
	out := make([]hierarchyNode, 0, len(items))
	for _, item := range items {
		key := hierarchyItemKey(item)
		id := parent + "/" + key
		if seen[id] {
			continue
		}
		seen[id] = true
		rec := ancestors[key]
		out = append(out, hierarchyNode{
			id: id, parent: parent, item: item, depth: depth, recursion: rec,
		})
		if rec {
			if m.hierarchy.children == nil {
				m.hierarchy.children = map[string][]hierarchyNode{}
			}
			m.hierarchy.children[id] = []hierarchyNode{}
		}
	}
	return out
}

// hierarchyParent is the parent id encoded in a node id, "" for a top level.
func hierarchyParent(id string) string {
	i := strings.LastIndex(id, "/")
	if i <= 0 {
		return ""
	}
	return id[:i]
}

// hierarchyKey is a symbol's position key, so the same symbol always has one.
func hierarchyKey(path string, line, col int, name string) string {
	return fmt.Sprintf("%s:%d:%d:%s", path, line, col, name)
}

// hierarchyItemKey is hierarchyKey of an item.
func hierarchyItemKey(item hierarchyItem) string {
	start := item.SelectionRange.Start
	return hierarchyKey(item.Path, int(start.Line), int(start.Col), item.Name)
}

// hierarchyDepth is a node's depth, read from the stored rows.
func (m *Model) hierarchyDepth(id string) int {
	parent := hierarchyParent(id)
	for _, node := range m.hierarchy.children[parent] {
		if node.id == id {
			return node.depth
		}
	}
	return 0
}

// ToggleHierarchyNode expands a row, loading its level once, or collapses it.
// A recursion leaf has nothing to show.
func (m *Model) ToggleHierarchyNode(id string) {
	if m.hierarchy.expanded[id] {
		delete(m.hierarchy.expanded, id)
		m.invalidate()
		return
	}
	m.hierarchy.expanded[id] = true
	if _, loaded := m.hierarchy.children[id]; loaded {
		m.invalidate()
		return
	}
	if node, ok := m.hierarchyNode(id); ok && !node.recursion {
		m.loadHierarchyNode(node)
	}
	m.invalidate()
}

func (m *Model) loadHierarchyNode(node hierarchyNode) {
	if node.item.Path == "" {
		return
	}
	gen := m.hierarchy.gen
	if t := m.tabForHierarchy(node.item.Path); t != nil {
		m.loadHierarchy(node.id, t, node.item.SelectionRange.Start)
		return
	}
	m.openFileThen(node.item.Path, func(t *Tab) {
		if t == nil || m.hierarchy.gen != gen ||
			!m.hierarchy.expanded[node.id] {
			return
		}
		m.loadHierarchy(node.id, t, node.item.SelectionRange.Start)
	})
}

// hierarchyNode finds a row by id.
func (m *Model) hierarchyNode(id string) (hierarchyNode, bool) {
	for _, nodes := range m.hierarchy.children {
		for _, node := range nodes {
			if node.id == id {
				return node, true
			}
		}
	}
	return hierarchyNode{}, false
}

// tabForHierarchy is the open tab of an item's file, opened first when it has
// none: a request needs the server's document id.
func (m *Model) tabForHierarchy(path string) *Tab {
	want := realPath(path)
	for _, t := range m.tabs {
		if realPath(t.doc.Path()) == want {
			return t
		}
	}
	return nil
}

// SetHierarchyDirection switches Callers/Callees or Supertypes/Subtypes and
// queries again from the same root.
func (m *Model) SetHierarchyDirection(direction string) {
	if direction != "outgoing" {
		direction = "incoming"
	}
	if m.hierarchy.direction == direction {
		return
	}
	m.hierarchy.direction = direction
	m.hierarchy.gen++
	m.hierarchy.children = map[string][]hierarchyNode{}
	m.hierarchy.expanded = map[string]bool{}
	m.hierarchy.loading = map[string]bool{}
	m.hierarchy.failed = map[string]string{}
	m.hierarchy.notes = map[string]string{}
	m.hierarchy.err = ""
	m.hierarchy.expanded[hierarchyRoot] = true
	m.hierarchy.selected = hierarchyRoot
	m.loadHierarchyRoot()
	m.invalidate()
}

// RefreshHierarchy reruns the current hierarchy from the same root and keeps
// the selected direction, as OpenIDEA's Refresh button does.
func (m *Model) RefreshHierarchy() {
	if !m.hierarchy.active() {
		return
	}
	m.hierarchy.gen++
	m.hierarchy.children = map[string][]hierarchyNode{}
	m.hierarchy.expanded = map[string]bool{hierarchyRoot: true}
	m.hierarchy.loading = map[string]bool{}
	m.hierarchy.failed = map[string]string{}
	m.hierarchy.notes = map[string]string{}
	m.hierarchy.err = ""
	m.hierarchy.loadedRoot = false
	m.hierarchy.selected = hierarchyRoot
	m.loadHierarchyRoot()
	m.invalidate()
}

func (m *Model) loadHierarchyRoot() {
	if m.hierarchy.rootPath == "" {
		return
	}
	pos := openidea.OutlinePos{
		Line: uint32(m.hierarchy.rootLine), Col: uint32(m.hierarchy.rootCol),
	}
	if t := m.tabForHierarchy(m.hierarchy.rootPath); t != nil {
		m.loadHierarchy(hierarchyRoot, t, pos)
		return
	}
	m.openFileThen(m.hierarchy.rootPath, func(t *Tab) {
		if t != nil && m.hierarchy.active() {
			m.loadHierarchy(hierarchyRoot, t, pos)
		}
	})
}

// OpenHierarchyItem navigates to the row's declaration.
func (m *Model) OpenHierarchyItem(id string) {
	if id == hierarchyRoot {
		if m.hierarchy.rootPath != "" {
			m.revealAt(m.hierarchy.rootPath, m.hierarchy.rootLine, m.hierarchy.rootCol)
		}
		return
	}
	node, ok := m.hierarchyNode(id)
	if !ok || node.item.Path == "" {
		return
	}
	line, col := m.hierarchyTarget(node.item)
	m.revealAt(node.item.Path, line, col)
}

func (m *Model) hierarchyTarget(item hierarchyItem) (line, col int) {
	position := item.SelectionRange.Start
	if m.hierarchy.mode == "call" && m.hierarchy.direction == "incoming" &&
		len(item.FromRanges) > 0 {
		position = item.FromRanges[0].Start
	}
	return int(position.Line), int(position.Col)
}

// CloseHierarchyWindow hides the right window; its tree stays for the stripe.
func (m *Model) CloseHierarchyWindow() { m.DockHide("hierarchy") }

// CloseHierarchyContent closes the hierarchy tab and drops its tree, as
// OpenIDEA's closable hierarchy content tab does.
func (m *Model) CloseHierarchyContent() {
	m.resetHierarchy()
	m.CloseHierarchyWindow()
	m.invalidate()
}

// ShowHierarchyWindow shows the right window again.
func (m *Model) ShowHierarchyWindow() {
	if !m.hierarchy.active() {
		m.OpenCallHierarchy()
		return
	}
	m.DockShow("hierarchy")
}

// ToggleHierarchyWindow is the stripe button: it closes the window when it
// shows, and shows it otherwise.
func (m *Model) ToggleHierarchyWindow() {
	if m.dockWindowShown("hierarchy") {
		m.CloseHierarchyWindow()
		return
	}
	m.ShowHierarchyWindow()
}

// rightShown reports whether the right side shows anything: either of its
// slots takes a track, or a legacy window still recorded in the old fields.
func (m *Model) rightShown() bool {
	return m.dockTrackShown(dockRight) || m.dockTrackShown(dockRightSplit) ||
		m.rightTool != "" && !m.rightHidden && !m.undockedWindow(m.rightTool)
}

// hierarchyDirectionLabel is the header's direction tab.
func (m *Model) hierarchyDirectionLabel(direction string) string {
	if m.hierarchy.mode == "type" {
		if direction == "outgoing" {
			return "Subtypes"
		}
		return "Supertypes"
	}
	if direction == "outgoing" {
		return "Callees"
	}
	return "Callers"
}

func (m *Model) hierarchyTitle() string {
	name := m.hierarchy.rootName
	if name == "" {
		name = filepath.Base(m.hierarchy.rootPath)
	}
	prefix := "Callers"
	switch {
	case m.hierarchy.mode == "type" && m.hierarchy.direction == "incoming":
		prefix = "Supertypes"
	case m.hierarchy.mode == "type":
		prefix = "Subtypes"
	case m.hierarchy.direction == "outgoing":
		prefix = "Callees"
	}
	return prefix + " of " + name
}

// hierarchyEmptyLabel is the tree's empty line for the current mode.
func (m *Model) hierarchyEmptyLabel() string {
	if m.hierarchy.mode == "type" {
		if m.hierarchy.direction == "outgoing" {
			return "— no subtypes found"
		}
		return "— no supertypes found"
	}
	if m.hierarchy.direction == "outgoing" {
		return "— no callees found"
	}
	return "— no callers found"
}

// hierarchyRows flattens the expanded tree for drawing.
func (m *Model) hierarchyRows() []hierarchyNode {
	if !m.hierarchy.expanded[hierarchyRoot] {
		return nil
	}
	var out []hierarchyNode
	var walk func(parent string)
	walk = func(parent string) {
		for _, node := range m.hierarchy.children[parent] {
			out = append(out, node)
			if m.hierarchy.expanded[node.id] {
				walk(node.id)
			}
		}
	}
	walk(hierarchyRoot)
	return out
}

// StepHierarchy walks every visible hierarchy occurrence, including the root,
// and opens the target. Callers navigate to their call site, as IDEA does.
func (m *Model) StepHierarchy(dir int) {
	rows := m.hierarchyRows()
	ids := make([]string, 0, len(rows)+1)
	ids = append(ids, hierarchyRoot)
	for _, row := range rows {
		ids = append(ids, row.id)
	}
	if dir == 0 || len(ids) == 0 {
		return
	}
	selected := -1
	for i, id := range ids {
		if id == m.hierarchy.selected {
			selected = i
			break
		}
	}
	if dir < 0 && selected < 0 {
		selected = 0
	}
	next := (selected + dir + len(ids)) % len(ids)
	m.selectHierarchy(ids[next])
	m.OpenHierarchyItem(ids[next])
}

// hierarchyRightView is OpenIDEA's tool-window frame. The title and compact
// content tab live in the same 32 DIP header; the controls are one 32 DIP
// toolbar below it, split into compact icon groups.
func (m *Model) hierarchyRightView(c *ui.Context, p *ui.Theme, oi oiColors) {
	ui.Column(c).Grow(1).MinHeight(0).Background(oi.island).
		Radius(oiIslandRadius).Focusable().AutoFocus().Children(func() {
		m.hierarchyHeader(c, p, oi)
		m.hierarchyToolbar(c, p, oi)
		if note := m.hierarchy.notes[hierarchyRoot]; note != "" {
			ui.Row(c).Shrink(0).Padding(0, 8, 4, 8).Children(func() {
				ui.Text(c, note).FontSize(oiFontSizeS).Italic().TextColor(oi.textSubtle).
					SingleLine().Ellipsis("…")
			})
		}
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			if m.hierarchy.err != "" {
				ui.Text(c, m.hierarchy.err).FontSize(12).TextColor(p.Danger).
					Padding(8, 10).Ellipsis("…")
				return
			}
			if !m.hierarchy.active() {
				ui.Column(c).Fill().Padding(hierarchyBodyInset).Gap(oiSpaceL).
					Justify(ui.Center).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "Use Call Hierarchy (Ctrl+Alt+H) or Type Hierarchy (Ctrl+H) on a symbol to see its hierarchy here").
						FontSize(oiFontSizeM).TextColor(oi.textMuted).MaxWidth(320).Wrap()
				})
				return
			}
			m.hierarchyRootRow(c, p, oi)
			rows := m.hierarchyRows()
			for _, node := range rows {
				m.hierarchyRow(c, p, oi, node)
			}
		})
	})
}

// hierarchyHeader is HeaderRow in OpenIDEA: the plain title, then the compact
// closable content tab, a flexible spacer, and the faded frame actions.
func (m *Model) hierarchyHeader(c *ui.Context, p *ui.Theme, oi oiColors) {
	header := ui.Row(c).Key("hierarchy:header").Height(oiTokensFor(m, p).HeaderHeight).
		Shrink(0).Padding(0, hierarchyHeaderSidePadding).
		Gap(oiSpaceM).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(oi.border)
	if header.DoubleClicked() {
		m.toggleToolWindowMaximized()
	}
	header.Children(func() {
		ui.Text(c, "Hierarchy").FontSize(oiFontSizeM).TextColor(oi.textStrong).
			SingleLine().Ellipsis("…")
		if m.hierarchy.active() {
			m.hierarchyContentTab(c, oi)
		}
		ui.Spacer(c)
		actions := ui.Row(c).Key("hierarchy:header-actions").Shrink(0).
			Gap(hierarchyToolbarGap).AlignItems(ui.Center)
		if header.Hovered() || header.FocusWithin() {
			actions.Opacity(1)
		} else {
			actions.Opacity(0.52)
		}
		actions.Children(func() {
			more := hierarchyIconButton(c, "hierarchy:options", "general/moreVertical",
				"Options", oi, oi.icon, func() {})
			more.Menu(func(menu *ui.Menu) { m.toolWindowFrameOptionsMenu(menu, "hierarchy") })
			hideTip := "Hide Hierarchy Tool Window"
			hide := hierarchyIconButton(c, "hierarchy:hide", "general/hide",
				hideTip, oi, oi.icon, m.CloseHierarchyWindow)
			hide.Label(hideTip)
		})
	})
}

// hierarchyContentTab is the compact Tabs primitive: 28 DIP strip, 22–24 DIP
// selected pill, 10 DIP label inset and a 20 DIP close slot.
func (m *Model) hierarchyContentTab(c *ui.Context, oi oiColors) {
	ui.Row(c).Key("hierarchy:tabs").Height(28).Shrink(1).MinWidth(0).
		AlignItems(ui.Center).Children(func() {
		tab := ui.Row(c).Key("hierarchy:content-tab").Height(hierarchyTabHeight).
			Radius(oiRadiusM).AlignItems(ui.Center).Background(oi.selected).
			BorderWidth(1, 1, 1, 1).BorderColor(oi.borderStrong).
			Focusable().Role(ui.RoleTab).Label(m.hierarchyTitle())
		tab.HandleInput(func(ev ui.InputEvent) bool {
			if ev.Kind != ui.InputKeyDown {
				return false
			}
			switch ev.Key {
			case ui.KeyEnter, ui.KeySpace:
				m.OpenHierarchyItem(hierarchyRoot)
			case ui.KeyBackspace, ui.KeyDelete, ui.KeyEscape:
				m.CloseHierarchyContent()
			default:
				return false
			}
			return true
		})
		if tab.Clicked() {
			m.selectHierarchy(hierarchyRoot)
		}
		tab.Children(func() {
			ui.Text(c, m.hierarchyTitle()).FontSize(12).TextColor(oi.textStrong).
				PaddingStart(10).SingleLine().Ellipsis("…")
			closeButton := ui.ButtonBase(c.Key("hierarchy:content-tab:close")).
				Size(20, 20).Radius(oiRadiusS).Justify(ui.Center).AlignItems(ui.Center).
				Cursor(ui.CursorPointer).Tooltip("Close Hierarchy").Label("Close Hierarchy")
			if closeButton.Hovered() {
				closeButton.Background(oi.toolbarHover)
			}
			closeButton.Children(func() {
				oiIconEl(c, "general/remove", 16, oi, oi.textMuted)
			})
			if closeButton.Clicked() {
				m.CloseHierarchyContent()
			}
		})
	})
}

// hierarchyToolbar is HierarchyToolbar: two direction buttons, a separator,
// Refresh, another separator, previous/next occurrences and Autoscroll. Every
// control is the same 20x20 icon button as OpenIDEA's small IconButton.
func (m *Model) hierarchyToolbar(c *ui.Context, p *ui.Theme, oi oiColors) {
	ui.Row(c).Key("hierarchy:toolbar").Role(ui.RoleToolbar).
		Label("Hierarchy actions").Height(hierarchyToolbarHeight).Shrink(0).
		Padding(0, hierarchyHeaderSidePadding).Gap(hierarchyToolbarGap).
		AlignItems(ui.Center).Children(func() {
		for _, direction := range []string{"incoming", "outgoing"} {
			direction := direction
			icon := "general/up"
			if direction == "outgoing" {
				icon = "general/down"
			}
			button := hierarchyIconButton(c, "hierarchy:direction:"+direction, icon,
				m.hierarchyDirectionLabel(direction)+" Hierarchy", oi, oi.icon, func() {
					m.SetHierarchyDirection(direction)
				})
			if m.hierarchy.direction == direction {
				button.Background(oi.stripeSelected).TextColor(oi.textStrong)
				button.Opacity(1)
			}
		}
		hierarchyToolbarSeparator(c, oi, "directions")
		hierarchyIconButton(c, "hierarchy:refresh", "general/refresh", "Refresh",
			oi, oi.icon, func() {
				if m.hierarchy.active() {
					m.RefreshHierarchy()
				}
			}).Disabled(!m.hierarchy.active())
		hierarchyToolbarSeparator(c, oi, "refresh")
		hierarchyIconButton(c, "hierarchy:prev", "general/up", "Previous Hierarchy Occurrence",
			oi, oi.icon, func() {
				if m.hierarchy.active() {
					m.StepHierarchy(-1)
				}
			}).Disabled(!m.hierarchy.active())
		hierarchyIconButton(c, "hierarchy:next", "general/down", "Next Hierarchy Occurrence",
			oi, oi.icon, func() {
				if m.hierarchy.active() {
					m.StepHierarchy(1)
				}
			}).Disabled(!m.hierarchy.active())
		autoscroll := hierarchyIconButton(c, "hierarchy:autoscroll", "general/autoscrollToSource",
			"Autoscroll to Source", oi, oi.icon, func() {
				m.prefs.Hierarchy.Autoscroll = !m.prefs.Hierarchy.Autoscroll
				m.savePreferences()
				m.invalidate()
			})
		if m.prefs.Hierarchy.Autoscroll {
			autoscroll.Background(oi.stripeSelected).TextColor(oi.textStrong)
			autoscroll.Opacity(1)
		}
	})
}

func hierarchyToolbarSeparator(c *ui.Context, oi oiColors, key string) {
	ui.Row(c).Key("hierarchy:toolbar:separator:" + key).
		Width(1).Height(16).MarginX(4).Background(oi.border)
}

func hierarchyIconButton(
	c *ui.Context,
	key, icon, label string,
	oi oiColors,
	tint ui.Color,
	onClick func(),
) ui.Element {
	button := ui.ButtonBase(c.Key(key)).Size(hierarchyToolButton, hierarchyToolButton).
		Radius(oiRadiusM).Justify(ui.Center).AlignItems(ui.Center).
		Cursor(ui.CursorPointer).Tooltip(label).Label(label)
	if button.Hovered() || button.Focused() {
		button.Background(oi.toolbarHover)
	}
	button.Children(func() { oiIconEl(c, icon, 16, oi, tint) })
	if onClick != nil && button.Clicked() {
		onClick()
	}
	return button
}

func (m *Model) hierarchyRootRow(c *ui.Context, p *ui.Theme, oi oiColors) {
	rootName := m.hierarchy.rootName
	if rootName == "" {
		rootName = filepath.Base(m.hierarchy.rootPath)
	}
	row := ui.Row(c.Key("hierarchy:root")).Height(hierarchyRowHeight).Gap(oiSpaceS).
		PaddingStart(4).PaddingEnd(8).Radius(5).AlignItems(ui.Center).
		Cursor(ui.CursorPointer)
	if m.hierarchy.selected == hierarchyRoot {
		row.Background(oi.selected)
	} else if row.Hovered() {
		row.Background(oi.hoverTint)
	}
	row.Children(func() {
		chevron := ui.Row(c.Key("hierarchy:caret:root")).Size(16, 16).
			Shrink(0).Justify(ui.Center).AlignItems(ui.Center).Cursor(ui.CursorPointer)
		glyph := "general/chevronRight"
		if m.hierarchy.expanded[hierarchyRoot] {
			glyph = "general/chevronDown"
		}
		chevron.Children(func() { oiIconEl(c, glyph, 16, oi, oi.textMuted) })
		if chevron.Clicked() {
			m.hierarchy.expanded[hierarchyRoot] = !m.hierarchy.expanded[hierarchyRoot]
			m.invalidate()
		}
		kind := "method"
		if m.hierarchy.mode == "type" {
			kind = "class"
		}
		if asset := symbolIcon(kind); asset != "" {
			oiIconEl(c, asset, 16, oi, oi.textSubtle).MarginEnd(2)
		}
		ui.Text(c, rootName).FontSize(oiFontSizeM).TextColor(oi.textStrong).
			SingleLine().Ellipsis("…")
		ui.Spacer(c)
		if m.hierarchy.rootPath != "" {
			ui.Text(c, fmt.Sprintf("%s:%d", filepath.Base(m.hierarchy.rootPath), m.hierarchy.rootLine+1)).
				FontSize(oiFontSizeM).TextColor(oi.textSubtle).SingleLine().Ellipsis("…")
		}
		switch {
		case m.hierarchy.loading[hierarchyRoot]:
			ui.Text(c, "Loading…").FontSize(12).TextColor(oi.textMuted).SingleLine()
		case m.hierarchy.failed[hierarchyRoot] != "":
			ui.Text(c, "failed to load").FontSize(12).TextColor(oi.danger).SingleLine()
		case m.hierarchy.loadedRoot || len(m.hierarchy.children[hierarchyRoot]) > 0:
			if len(m.hierarchy.children[hierarchyRoot]) == 0 {
				ui.Text(c, m.hierarchyEmptyLabel()).FontSize(12).TextColor(oi.textMuted).SingleLine()
			}
		}
	})
	if row.Clicked() {
		m.selectHierarchy(hierarchyRoot)
	}
	if row.DoubleClicked() {
		m.OpenHierarchyItem(hierarchyRoot)
	}
}

// hierarchyRow draws one row: the expand caret, the symbol badge, the name,
// its detail and the call-site count.
func (m *Model) hierarchyRow(c *ui.Context, p *ui.Theme, oi oiColors, node hierarchyNode) {
	row := ui.Row(c.Key("hierarchy:" + node.id)).Height(hierarchyRowHeight).Gap(oiSpaceM).
		PaddingStart(4 + float32(node.depth)*m.treeIndentWidth()).PaddingEnd(8).Radius(5).
		AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if node.item.Path != "" {
		path := node.item.Path
		if root := m.Root(); root != "" {
			if rel, err := filepath.Rel(root, path); err == nil {
				path = rel
			}
		}
		row.Tooltip(path)
	}
	if m.hierarchy.selected == node.id {
		row.Background(oi.selected)
	} else if row.Hovered() {
		row.Background(oi.hoverTint)
	}
	row.Children(func() {
		expandable := !node.recursion
		chevron := ui.Row(c.Key("hierarchy:caret:"+node.id)).Size(16, 16).
			Shrink(0).Justify(ui.Center).AlignItems(ui.Center).Cursor(ui.CursorPointer)
		if expandable {
			glyph := "general/chevronRight"
			if m.hierarchy.expanded[node.id] {
				glyph = "general/chevronDown"
			}
			chevron.Children(func() { oiIconEl(c, glyph, 16, oi, oi.textMuted) })
			if chevron.Clicked() {
				m.ToggleHierarchyNode(node.id)
				return
			}
		}
		kind := strings.ToLower(node.item.Kind)
		if m.hierarchy.mode == "type" {
			kind = "class"
		}
		if asset := symbolIcon(kind); asset != "" {
			oiIconEl(c, asset, 16, oi, oi.textSubtle).MarginEnd(2)
		} else {
			ui.Row(c).Size(16, 16).Shrink(0).MarginEnd(2)
		}
		ui.Text(c, node.item.Name).FontSize(oiFontSizeM).TextColor(oi.text).
			SingleLine().Ellipsis("…")
		if node.item.Detail != nil && *node.item.Detail != "" {
			ui.Text(c, *node.item.Detail).FontSize(oiFontSizeM).
				TextColor(oi.textSubtle).MinWidth(0).SingleLine().Ellipsis("…")
		}
		ui.Spacer(c)
		if node.item.Path != "" {
			ui.Text(c, fmt.Sprintf("%s:%d", filepath.Base(node.item.Path), node.item.SelectionRange.Start.Line+1)).
				FontSize(oiFontSizeM).TextColor(oi.textSubtle).SingleLine().Ellipsis("…")
		}
		if node.recursion {
			ui.Text(c, "recursive").FontSize(oiFontSizeM).TextColor(oi.textSubtle).SingleLine()
		}
		if n := len(node.item.FromRanges); n > 1 {
			ui.Text(c, fmt.Sprintf("(%d calls)", n)).FontSize(oiFontSizeM).
				TextColor(oi.textSubtle).SingleLine()
		}
		if note := m.hierarchy.notes[node.id]; note != "" {
			ui.Text(c, note).FontSize(12).TextColor(oi.textMuted).
				SingleLine().Ellipsis("…")
		}
		if m.hierarchy.failed[node.id] != "" {
			ui.Text(c, "failed to load").FontSize(12).TextColor(oi.danger).SingleLine()
		}
	})
	if row.Clicked() {
		m.selectHierarchy(node.id)
	}
	if row.DoubleClicked() {
		m.OpenHierarchyItem(node.id)
	}
}

func (m *Model) selectHierarchy(id string) {
	m.hierarchy.selected = id
	if !m.prefs.Hierarchy.Autoscroll {
		m.invalidate()
		return
	}
	if id == hierarchyRoot {
		if m.hierarchy.rootPath != "" {
			m.previewAt(m.hierarchy.rootPath, m.hierarchy.rootLine, m.hierarchy.rootCol)
		}
		m.invalidate()
		return
	}
	if node, ok := m.hierarchyNode(id); ok && node.item.Path != "" {
		line, col := m.hierarchyTarget(node.item)
		m.previewAt(node.item.Path, line, col)
	}
	m.invalidate()
}

// SmokeHierarchy opens the call hierarchy at the caret and reports the levels
// it loaded, for the language-server phase of the e2e run.
func (m *Model) SmokeHierarchy(ctx context.Context) (map[string]any, error) {
	out := map[string]any{}
	t := m.tab()
	if t == nil {
		return out, fmt.Errorf("no open file")
	}
	// The fixture's main method, so callers exist.
	var pos openidea.OutlinePos
	m.post(func() {
		text := t.doc.Text()
		off := strings.Index(text, "main(")
		if off < 0 {
			off = strings.Index(text, "void main")
		}
		if off < 0 {
			off = 0
		}
		pos = t.doc.UTF16Pos(off)
	})
	m.post(func() {
		m.hierarchy = hierarchyPane{
			mode: "call", direction: "outgoing",
			rootName: "main", rootPath: realPath(t.doc.Path()),
			rootLine: int(pos.Line), rootCol: int(pos.Col),
			children: map[string][]hierarchyNode{}, expanded: map[string]bool{},
			loading: map[string]bool{}, failed: map[string]string{}, gen: 1,
		}
		m.rightTool = "hierarchy"
		m.rightHidden = false
		m.loadHierarchy(hierarchyRoot, t, openidea.OutlinePos{Line: pos.Line, Col: pos.Col})
	})
	deadline := time.Now().Add(90 * time.Second)
	for {
		done := false
		var rows int
		var errText string
		m.post(func() {
			rows = len(m.hierarchy.children[hierarchyRoot])
			errText = m.hierarchy.failed[hierarchyRoot]
			done = rows > 0 || errText != "" || !m.hierarchy.loading[hierarchyRoot] && len(m.hierarchy.children) > 0
		})
		if done || time.Now().After(deadline) {
			out["rows"] = rows
			out["visible"] = m.rightShown()
			out["mode"] = m.hierarchy.mode
			out["direction"] = m.hierarchy.direction
			if errText != "" {
				out["error"] = errText
			}
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
	return out, nil
}
