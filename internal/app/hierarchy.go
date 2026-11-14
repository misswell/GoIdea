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
	gen      uint64
	err      string
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
		m.status = "Hierarchy 需要打开的文件"
		m.invalidate()
		return
	}
	offset := t.state.Cursor()
	pos := t.doc.UTF16Pos(offset)
	m.hierarchy = hierarchyPane{
		mode:      mode,
		direction: "incoming",
		rootName:  wordAtCaret(t.doc.Text(), offset),
		rootPath:  realPath(t.doc.Path()),
		rootLine:  int(pos.Line),
		rootCol:   int(pos.Col),
		children:  map[string][]hierarchyNode{},
		expanded:  map[string]bool{},
		loading:   map[string]bool{},
		failed:    map[string]string{},
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
			if incomplete != "" {
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
		t := m.tabForHierarchy(node.item.Path)
		if t != nil {
			m.loadHierarchy(id, t, node.item.SelectionRange.Start)
		}
	}
	m.invalidate()
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
	if m.reg != nil {
		go m.post(func() { m.openFile(path) })
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
	m.hierarchy.err = ""
	t := m.tabForHierarchy(m.hierarchy.rootPath)
	if t != nil {
		m.loadHierarchy(hierarchyRoot, t, openidea.OutlinePos{Line: uint32(m.hierarchy.rootLine), Col: uint32(m.hierarchy.rootCol)})
	}
	m.invalidate()
}

// OpenHierarchyItem navigates to the row's declaration.
func (m *Model) OpenHierarchyItem(id string) {
	node, ok := m.hierarchyNode(id)
	if !ok || node.item.Path == "" {
		return
	}
	m.revealAt(node.item.Path, int(node.item.SelectionRange.Start.Line), int(node.item.SelectionRange.Start.Col))
}

// CloseHierarchyWindow hides the right window; its tree stays for the stripe.
func (m *Model) CloseHierarchyWindow() { m.DockHide("hierarchy") }

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
// slots occupied, or a legacy window still recorded in the old fields.
func (m *Model) rightShown() bool {
	if m.dockRightShown() {
		return true
	}
	return m.rightTool != "" && !m.rightHidden
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

// hierarchyRightView is the right area: the window's title, tabs and tree.
func (m *Model) hierarchyRightView(c *ui.Context, p *ui.Theme, oi oiColors) {
	ui.Column(c).Grow(1).MinHeight(0).Background(oi.island).
		Radius(oiIslandRadius).Children(func() {
		ui.Row(c).Height(oiHeaderHeight).Shrink(0).Padding(0, 6, 0, 8).
			Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Hierarchy").FontSize(13).TextColor(oi.textStrong).
				SingleLine().Ellipsis("…")
			ui.Spacer(c)
			m.headerIcon(c, p, oi, "hierarchy-close", "Hide", iconClose, m.CloseHierarchyWindow)
		})
		m.hierarchyTabs(c, p, oi)
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			if m.hierarchy.err != "" {
				ui.Text(c, m.hierarchy.err).FontSize(11).TextColor(p.Danger).
					Padding(8, 10).Ellipsis("…")
				return
			}
			if len(m.hierarchy.loading) > 0 && len(m.hierarchy.children) == 0 {
				ui.Text(c, "Loading…").FontSize(11).TextColor(oi.textMuted).
					Padding(8, 10).SingleLine()
				return
			}
			if !m.hierarchy.active() {
				ui.Text(c, "Ctrl+Alt+H runs a Call Hierarchy here.").FontSize(11).
					TextColor(oi.textMuted).Padding(8, 10).Ellipsis("…")
				return
			}
			rows := m.hierarchyRows()
			if len(rows) == 0 {
				ui.Text(c, m.hierarchyEmptyLabel()).FontSize(11).
					TextColor(oi.textMuted).Padding(8, 10).SingleLine()
				return
			}
			for _, node := range rows {
				m.hierarchyRow(c, p, oi, node)
			}
		})
	})
}

// hierarchyTabs draws the direction tabs the header carries, as
// HierarchyTabs does.
func (m *Model) hierarchyTabs(c *ui.Context, p *ui.Theme, oi oiColors) {
	ui.Row(c).Height(28).Shrink(0).Gap(2).Padding(0, 6, 0, 8).
		AlignItems(ui.Center).Children(func() {
		for _, dir := range []string{"incoming", "outgoing"} {
			dir := dir
			active := m.hierarchy.direction == dir
			tab := ui.Row(c.Key("hierarchy:tab:"+dir)).Height(24).Padding(0, 10).
				Radius(4).AlignItems(ui.Center).Cursor(ui.CursorPointer)
			text := m.hierarchyDirectionLabel(dir)
			switch {
			case active:
				tab = tab.Background(oi.hoverTint)
			default:
				tab = tab.Cursor(ui.CursorPointer)
			}
			tab.Children(func() {
				tone := oi.textMuted
				if active {
					tone = oi.textStrong
				}
				ui.Text(c, text).FontSize(12).TextColor(tone).SingleLine()
			})
			if tab.Clicked() {
				m.SetHierarchyDirection(dir)
			}
		}
	})
}

// hierarchyRow draws one row: the expand caret, the name, its detail and the
// call-site count.
func (m *Model) hierarchyRow(c *ui.Context, p *ui.Theme, oi oiColors, node hierarchyNode) {
	row := ui.Row(c.Key("hierarchy:" + node.id)).Height(24).Gap(6).
		PaddingStart(6 + float32(node.depth)*12).PaddingEnd(8).
		AlignItems(ui.Center).Cursor(ui.CursorPointer)
	row.Children(func() {
		expandable := !node.recursion
		if expandable {
			caret := "▸"
			if m.hierarchy.expanded[node.id] {
				caret = "▾"
			}
			btn := ui.Row(c.Key("hierarchy:caret:"+node.id)).Size(14, 14).
				AlignItems(ui.Center).Cursor(ui.CursorPointer)
			btn.Children(func() {
				ui.Text(c, caret).FontSize(11).TextColor(oi.textMuted)
			})
			if btn.Clicked() {
				m.ToggleHierarchyNode(node.id)
				return
			}
		} else {
			ui.Text(c, "·").FontSize(11).TextColor(oi.textFaint)
		}
		ui.Text(c, node.item.Name).FontSize(12).TextColor(oi.text).
			SingleLine().Ellipsis("…")
		if node.item.Detail != nil && *node.item.Detail != "" {
			ui.Text(c, *node.item.Detail).FontSize(11).TextColor(oi.textMuted).
				SingleLine().Ellipsis("…")
		}
		ui.Spacer(c)
		if n := len(node.item.FromRanges); n > 0 {
			ui.Text(c, fmt.Sprintf("%d", n)).FontSize(10.5).
				TextColor(oi.textFaint).SingleLine()
		} else if node.item.Path != "" {
			ui.Text(c, filepath.Base(node.item.Path)).FontSize(10.5).
				TextColor(oi.textFaint).SingleLine().Ellipsis("…")
		}
	})
	if row.Clicked() {
		m.OpenHierarchyItem(node.id)
	}
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
