package app

// The main toolbar model, ported from OpenIDEA's mainToolbar.ts: which
// widgets and actions the title bar shows on each side, in which order,
// stored in the preferences. Unset = the default toolbar.

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

const (
	toolbarProject      = "project"
	toolbarVCS          = "vcs.branch"
	toolbarRun          = "run"
	toolbarActionPrefix = "action:"
	toolbarSearch       = toolbarActionPrefix + "app.commandPalette"
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

func toolbarDefaultSide(id string) string {
	switch id {
	case toolbarProject, toolbarVCS:
		return "left"
	}
	return "right"
}

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
	switch id {
	case toolbarProject:
		return "Project Widget"
	case toolbarVCS:
		return "VCS Widget"
	case toolbarRun:
		return "Run"
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

// toolbarCustomizeMenu is the gear's one submenu for the same items.
func (m *Model) toolbarCustomizeMenu(menu *ui.Menu) {
	menu.Submenu("Customize Main Toolbar", func(sub *ui.Menu) { m.toolbarCustomizeItems(sub) })
}
