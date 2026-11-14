package app

// The main toolbar model, ported from OpenIDEA's mainToolbar.ts: which
// widgets and actions the title bar shows on each side, in which order,
// stored in the preferences. Unset = the default toolbar.

import (
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"
)

const (
	toolbarProject      = "project"
	toolbarVCS          = "vcs.branch"
	toolbarRun          = "run"
	toolbarActionPrefix = "action:"
	toolbarSearch       = toolbarActionPrefix + "navigate.searchEverywhere"
	toolbarTerminal     = toolbarActionPrefix + "view.terminal"
)

// toolbarLayout is the saved toolbar, in OpenIDEA's shape so a settings file
// written there would read back.
type toolbarLayout struct {
	Left   []string `json:"left,omitempty"`
	Right  []string `json:"right,omitempty"`
	Hidden []string `json:"hidden,omitempty"`
}

func defaultToolbarLayout() toolbarLayout {
	return toolbarLayout{
		Left:  []string{toolbarProject, toolbarVCS},
		Right: []string{toolbarRun, toolbarSearch, toolbarTerminal},
	}
}

func isToolbarAction(id string) bool { return strings.HasPrefix(id, toolbarActionPrefix) }

func toolbarAction(id string) string { return strings.TrimPrefix(id, toolbarActionPrefix) }

func toolbarDefaultSide(id string) string { return titleWidgetSide(id) }

// resolveToolbar is the entries to show per side: the saved order, plus the
// widgets registered since (not listed, not removed) at their default side's
// end. The gear button is not part of the layout, as in OpenIDEA.
func resolveToolbar(saved *toolbarLayout) (left, right []string) {
	d := defaultToolbarLayout()
	if saved == nil {
		return d.Left, d.Right
	}
	left = append(left, saved.Left...)
	right = append(right, saved.Right...)
	known := make(map[string]bool, len(left)+len(right)+len(saved.Hidden))
	for _, id := range append(append(append([]string{}, left...), right...), saved.Hidden...) {
		known[id] = true
	}
	for _, id := range d.Left {
		if !known[id] {
			left = append(left, id)
		}
	}
	for _, id := range d.Right {
		if !known[id] {
			right = append(right, id)
		}
	}
	return left, right
}

// editableToolbar is the whole layout the customization menu lists: what each
// side shows and what is hidden.
func editableToolbar(saved *toolbarLayout) toolbarLayout {
	left, right := resolveToolbar(saved)
	layout := toolbarLayout{Left: left, Right: right}
	if saved != nil {
		layout.Hidden = append(layout.Hidden, saved.Hidden...)
	}
	return layout
}

// toggleToolbarEntry shows or removes one entry, keeping the rest of the
// layout, as widget toggling does in OpenIDEA. Actions are not remembered as
// hidden; widgets are, so a widget stays off after being removed.
func toggleToolbarEntry(layout toolbarLayout, id string) toolbarLayout {
	remove := func(list []string) []string {
		out := list[:0]
		for _, x := range list {
			if x != id {
				out = append(out, x)
			}
		}
		return out
	}
	shown := false
	for _, x := range layout.Left {
		shown = shown || x == id
	}
	for _, x := range layout.Right {
		shown = shown || x == id
	}
	layout.Left, layout.Right = remove(layout.Left), remove(layout.Right)
	if shown {
		if !isToolbarAction(id) {
			layout.Hidden = append(layout.Hidden, id)
		}
		return layout
	}
	if toolbarDefaultSide(id) == "left" {
		layout.Left = append(layout.Left, id)
	} else {
		layout.Right = append(layout.Right, id)
	}
	layout.Hidden = remove(layout.Hidden)
	return layout
}

// toolbarTitle names an entry the way Customize Main Toolbar lists it.
func toolbarTitle(id string) string {
	if w := titleWidgetFor(id); w != nil {
		return w.Title
	}
	if isToolbarAction(id) {
		return commandTitle(toolbarAction(id))
	}
	return id
}

// commandTitle is a registry action's title, falling back to its id.
func commandTitle(id string) string {
	for _, c := range commands() {
		if c.id == id {
			return c.title
		}
	}
	return id
}

// toolbarEntries lists every entry the customization menu shows, shown first
// in side order, then the hidden ones.
func (m *Model) toolbarEntries() []string {
	layout := editableToolbar(m.prefs.MainToolbar)
	return append(append(append([]string{}, layout.Left...), layout.Right...), layout.Hidden...)
}

func (m *Model) toolbarShows(id string) bool {
	left, right := resolveToolbar(m.prefs.MainToolbar)
	for _, x := range append(left, right...) {
		if x == id {
			return true
		}
	}
	return false
}

func (m *Model) toggleToolbarWidget(id string) {
	layout := toggleToolbarEntry(editableToolbar(m.prefs.MainToolbar), id)
	m.saveToolbarLayout(&layout)
}

// RestoreDefaultToolbar puts every widget back in its default place.
func (m *Model) RestoreDefaultToolbar() { m.saveToolbarLayout(nil) }

func (m *Model) saveToolbarLayout(layout *toolbarLayout) {
	m.prefs.MainToolbar = layout
	if m.cfg.SettingsPath != "" {
		_ = m.prefs.Save(m.cfg.SettingsPath)
	}
	m.invalidate()
}

// toolbarCustomizeItems is Customize Main Toolbar's body: a checkbox per
// entry, shown and hidden alike, then Restore Default Toolbar. The title bar's
// context menu shows it flat, as OpenIDEA's does.
func (m *Model) toolbarCustomizeItems(menu *ui.Menu) {
	for _, id := range m.toolbarEntries() {
		id := id
		it := menu.Item(toolbarTitle(id))
		it.Checked(m.toolbarShows(id))
		if it.Chosen() {
			m.toggleToolbarWidget(id)
		}
	}
	menu.Separator()
	if menu.Item("Restore Default Toolbar").Chosen() {
		m.RestoreDefaultToolbar()
	}
}

// OpenToolbarCustomize opens Customize Main Toolbar on a draft of the layout:
// the dialog saves only on OK, as OpenIDEA's does, and Cancel discards.
func (m *Model) OpenToolbarCustomize() {
	if m.toolbarCustomize {
		return
	}
	m.toolbarDraft = editableToolbar(m.prefs.MainToolbar)
	m.toolbarSide = "left"
	m.toolbarSel = 0
	m.toolbarAdding = false
	m.toolbarAddQuery = ""
	m.toolbarCustomize = true
	m.invalidate()
}

func (m *Model) closeToolbarCustomize() {
	m.toolbarCustomize = false
	m.toolbarAdding = false
	m.invalidate()
}

func toolbarSideList(layout toolbarLayout, side string) []string {
	if side == "left" {
		return layout.Left
	}
	return layout.Right
}

func toolbarSetSide(layout toolbarLayout, side string, list []string) toolbarLayout {
	if side == "left" {
		layout.Left = list
	} else {
		layout.Right = list
	}
	return layout
}

func toolbarHas(list []string, id string) bool {
	for _, x := range list {
		if x == id {
			return true
		}
	}
	return false
}

// toolbarDraftMove swaps the entry at index with its neighbour on the same side.
func toolbarDraftMove(layout toolbarLayout, side string, index, delta int) toolbarLayout {
	list := append([]string{}, toolbarSideList(layout, side)...)
	j := index + delta
	if index < 0 || index >= len(list) || j < 0 || j >= len(list) {
		return layout
	}
	list[index], list[j] = list[j], list[index]
	return toolbarSetSide(layout, side, list)
}

// toolbarDraftRemove takes the entry off its side; a widget is remembered as
// hidden so the Add… picker can bring it back, as OpenIDEA's Remove does.
func toolbarDraftRemove(layout toolbarLayout, side string, index int) toolbarLayout {
	list := toolbarSideList(layout, side)
	if index < 0 || index >= len(list) {
		return layout
	}
	id := list[index]
	out := make([]string, 0, len(list)-1)
	for i, x := range list {
		if i != index {
			out = append(out, x)
		}
	}
	layout = toolbarSetSide(layout, side, out)
	if !isToolbarAction(id) && !toolbarHas(layout.Hidden, id) {
		layout.Hidden = append(append([]string{}, layout.Hidden...), id)
	}
	return layout
}

// toolbarDraftMoveSide moves the entry at index to the end of the other side,
// returning where it landed.
func toolbarDraftMoveSide(layout toolbarLayout, side string, index int) (toolbarLayout, int) {
	list := toolbarSideList(layout, side)
	if index < 0 || index >= len(list) {
		return layout, 0
	}
	id := list[index]
	out := append([]string{}, list[:index]...)
	out = append(out, list[index+1:]...)
	other := "right"
	if side == "right" {
		other = "left"
	}
	olist := append(append([]string{}, toolbarSideList(layout, other)...), id)
	layout = toolbarSetSide(layout, side, out)
	layout = toolbarSetSide(layout, other, olist)
	return layout, len(olist) - 1
}

// toolbarDraftAdd puts id on side after index, taking it off the other side
// and the hidden list, and returns where it landed.
func toolbarDraftAdd(layout toolbarLayout, side string, index int, id string) (toolbarLayout, int) {
	list := make([]string, 0, len(toolbarSideList(layout, side))+1)
	for _, x := range toolbarSideList(layout, side) {
		if x != id {
			list = append(list, x)
		}
	}
	at := len(list)
	if index >= 0 && index < len(list) {
		at = index + 1
	}
	list = append(list, "")
	copy(list[at+1:], list[at:])
	list[at] = id
	other := "right"
	if side == "right" {
		other = "left"
	}
	olist := make([]string, 0, len(toolbarSideList(layout, other)))
	for _, x := range toolbarSideList(layout, other) {
		if x != id {
			olist = append(olist, x)
		}
	}
	layout = toolbarSetSide(layout, side, list)
	layout = toolbarSetSide(layout, other, olist)
	hidden := make([]string, 0, len(layout.Hidden))
	for _, x := range layout.Hidden {
		if x != id {
			hidden = append(hidden, x)
		}
	}
	layout.Hidden = hidden
	return layout, at
}

// toolbarAddCandidates is the Add… picker: removed widgets first, then the
// registry actions the toolbar does not show yet, filtered by the query.
func (m *Model) toolbarAddCandidates() []string {
	seen := make(map[string]bool)
	var out []string
	for _, id := range m.toolbarDraft.Hidden {
		if !seen[id] {
			out = append(out, id)
			seen[id] = true
		}
	}
	var acts []string
	for _, cmd := range commands() {
		id := toolbarActionPrefix + cmd.id
		if !seen[id] && !m.toolbarHasDraft(id) {
			acts = append(acts, id)
			seen[id] = true
		}
	}
	sort.Slice(acts, func(i, j int) bool { return toolbarTitle(acts[i]) < toolbarTitle(acts[j]) })
	out = append(out, acts...)
	query := strings.ToLower(strings.TrimSpace(m.toolbarAddQuery))
	if query == "" {
		return out
	}
	filtered := out[:0]
	for _, id := range out {
		if strings.Contains(strings.ToLower(toolbarTitle(id)), query) {
			filtered = append(filtered, id)
		}
	}
	return filtered
}

func (m *Model) toolbarHasDraft(id string) bool {
	return toolbarHas(m.toolbarDraft.Left, id) || toolbarHas(m.toolbarDraft.Right, id)
}

// toolbarCustomizeDialog draws Customize Main Toolbar: the entries of each
// side, reorderable and movable, the Add… picker, Restore Defaults, OK and
// Cancel, as OpenIDEA's MainToolbarDialog does.
func (m *Model) toolbarCustomizeDialog(c *ui.Context, p *ui.Theme) {
	if !m.toolbarCustomize {
		return
	}
	open := m.toolbarCustomize
	dlg := ui.Modal(c, &open, func() {
		ui.Column(c).Width(620).Gap(10).Children(func() {
			ui.Text(c, "Customize Main Toolbar").FontSize(14).Bold().TextColor(p.Text).SingleLine()
			ui.Row(c).Height(240).Gap(12).Children(func() {
				m.toolbarSideView(c, p, "left")
				m.toolbarSideView(c, p, "right")
			})
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Button(c, "Add…").OnClick(func() {
					m.toolbarAdding = !m.toolbarAdding
					m.invalidate()
				})
				list := toolbarSideList(m.toolbarDraft, m.toolbarSide)
				has := m.toolbarSel >= 0 && m.toolbarSel < len(list)
				ui.Button(c, "Remove").Disabled(!has).OnClick(func() {
					m.toolbarDraft = toolbarDraftRemove(m.toolbarDraft, m.toolbarSide, m.toolbarSel)
					if m.toolbarSel > 0 {
						m.toolbarSel--
					}
					m.invalidate()
				})
				ui.Button(c, "Move Up").Disabled(!has || m.toolbarSel <= 0).OnClick(func() {
					m.toolbarDraft = toolbarDraftMove(m.toolbarDraft, m.toolbarSide, m.toolbarSel, -1)
					if m.toolbarSel > 0 {
						m.toolbarSel--
					}
					m.invalidate()
				})
				ui.Button(c, "Move Down").Disabled(!has || m.toolbarSel >= len(list)-1).OnClick(func() {
					m.toolbarDraft = toolbarDraftMove(m.toolbarDraft, m.toolbarSide, m.toolbarSel, 1)
					if m.toolbarSel < len(list)-1 {
						m.toolbarSel++
					}
					m.invalidate()
				})
				label := "Move to Right"
				if m.toolbarSide == "right" {
					label = "Move to Left"
				}
				ui.Button(c, label).Disabled(!has).OnClick(func() {
					var at int
					m.toolbarDraft, at = toolbarDraftMoveSide(m.toolbarDraft, m.toolbarSide, m.toolbarSel)
					if m.toolbarSide == "left" {
						m.toolbarSide = "right"
					} else {
						m.toolbarSide = "left"
					}
					m.toolbarSel = at
					m.invalidate()
				})
			})
			if m.toolbarAdding {
				m.toolbarAddView(c, p)
			}
			ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Center).Children(func() {
				ui.Button(c, "Restore Defaults").OnClick(func() {
					m.toolbarDraft = defaultToolbarLayout()
					m.toolbarSide = "left"
					m.toolbarSel = 0
					m.invalidate()
				})
				ui.Row(c).Gap(6).Children(func() {
					ui.Button(c, "OK").OnClick(func() {
						m.saveToolbarLayout(&m.toolbarDraft)
						m.closeToolbarCustomize()
					})
					ui.Button(c, "Cancel").OnClick(m.closeToolbarCustomize)
				})
			})
		})
	})
	if dlg.Dismissed() || !open {
		m.closeToolbarCustomize()
	}
}

func (m *Model) toolbarSideView(c *ui.Context, p *ui.Theme, side string) {
	title := "Left"
	if side == "right" {
		title = "Right"
	}
	ui.Column(c).Width(286).Gap(4).Children(func() {
		ui.Text(c, title).FontSize(11).TextColor(p.TextMuted).SingleLine()
		list := toolbarSideList(m.toolbarDraft, side)
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).Children(func() {
				if len(list) == 0 {
					ui.Text(c, "Empty").FontSize(12).TextColor(p.TextMuted).SingleLine()
				}
				for i, id := range list {
					i, id := i, id
					r := ui.Row(c.Key("tb:"+side+":"+id)).Height(24).Padding(0, 8).Gap(8).AlignItems(ui.Center)
					if m.toolbarSide == side && m.toolbarSel == i {
						r.Background(p.SurfaceHover)
					} else if r.Hovered() {
						r.Background(p.SurfaceHover)
					}
					r.Children(func() {
						ui.Text(c, toolbarTitle(id)).FontSize(12).TextColor(p.Text).SingleLine().Ellipsis("…")
					})
					if r.Clicked() {
						m.toolbarSide = side
						m.toolbarSel = i
						m.invalidate()
					}
				}
			})
		})
	})
}

func (m *Model) toolbarAddView(c *ui.Context, p *ui.Theme) {
	ui.Column(c).Gap(4).Children(func() {
		in := ui.TextInput(c, &m.toolbarAddQuery)
		in.OnChange(func() { m.invalidate() })
		cands := m.toolbarAddCandidates()
		ui.Scroll(c).Height(160).Children(func() {
			ui.Column(c).Children(func() {
				if len(cands) == 0 {
					ui.Text(c, "No actions to add").FontSize(12).TextColor(p.TextMuted).SingleLine()
				}
				for _, id := range cands {
					id := id
					r := ui.Row(c.Key("tbadd:"+id)).Height(24).Padding(0, 8).Gap(8).AlignItems(ui.Center)
					if r.Hovered() {
						r.Background(p.SurfaceHover)
					}
					r.Children(func() {
						ui.Text(c, toolbarTitle(id)).FontSize(12).TextColor(p.Text).SingleLine().Ellipsis("…")
					})
					if r.Clicked() {
						var at int
						m.toolbarDraft, at = toolbarDraftAdd(m.toolbarDraft, m.toolbarSide, m.toolbarSel, id)
						m.toolbarSel = at
						m.toolbarAdding = false
						m.invalidate()
					}
				}
			})
		})
	})
}
