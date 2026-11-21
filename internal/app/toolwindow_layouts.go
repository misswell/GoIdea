package app

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
)

const (
	layoutDialogSave    = "save"
	layoutDialogRestore = "restore"
	layoutDialogRename  = "rename"
	layoutDialogDelete  = "delete"
)

// toolWindowLayoutDialog is the native prompt or chooser behind Window ›
// Tool Window Layouts. Rename first chooses a layout, then switches to its
// name field; delete chooses a layout and asks before removing it.
type toolWindowLayoutDialog struct {
	open       bool
	mode       string
	selected   int
	name       string
	renameFrom string
	confirm    bool
	err        string
}

// normalizeSavedToolWindowLayouts trims names, drops unnamed malformed rows,
// keeps OpenIDEA's save order and sanitizes every nested project layout.
func normalizeSavedToolWindowLayouts(list []SavedToolWindowLayout) []SavedToolWindowLayout {
	out := make([]SavedToolWindowLayout, 0, len(list))
	for _, entry := range list {
		entry.Name = strings.TrimSpace(entry.Name)
		if entry.Name == "" {
			continue
		}
		entry.Layout = normalizeLayout(entry.Layout)
		out = append(out, entry)
	}
	return out
}

func cloneSavedToolWindowLayouts(list []SavedToolWindowLayout) []SavedToolWindowLayout {
	if len(list) == 0 {
		return nil
	}
	out := make([]SavedToolWindowLayout, 0, len(list))
	for _, entry := range list {
		entry.Layout = entry.Layout.clone()
		out = append(out, entry)
	}
	return out
}

func normalizeActiveNamedLayout(active string, list []SavedToolWindowLayout) string {
	active = strings.TrimSpace(active)
	for _, entry := range list {
		if entry.Name == active {
			return active
		}
	}
	return ""
}

func findSavedToolWindowLayout(list []SavedToolWindowLayout, name string) (SavedToolWindowLayout, bool) {
	name = strings.TrimSpace(name)
	for _, entry := range list {
		if entry.Name == name {
			return entry, true
		}
	}
	return SavedToolWindowLayout{}, false
}

func saveToolWindowLayoutInList(
	list []SavedToolWindowLayout, name string, layout Layout,
) []SavedToolWindowLayout {
	name = strings.TrimSpace(name)
	if name == "" {
		return cloneSavedToolWindowLayouts(list)
	}
	out := cloneSavedToolWindowLayouts(list)
	entry := SavedToolWindowLayout{Name: name, Layout: layout.clone()}
	for i := range out {
		if out[i].Name == name {
			out[i] = entry
			return out
		}
	}
	return append(out, entry)
}

func renameToolWindowLayoutInList(
	list []SavedToolWindowLayout, from, to string,
) ([]SavedToolWindowLayout, bool) {
	from = strings.TrimSpace(from)
	to = strings.TrimSpace(to)
	if to == "" || (to != from && hasSavedToolWindowLayout(list, to)) {
		return cloneSavedToolWindowLayouts(list), false
	}
	out := cloneSavedToolWindowLayouts(list)
	for i := range out {
		if out[i].Name == from {
			out[i].Name = to
			return out, true
		}
	}
	return out, false
}

func hasSavedToolWindowLayout(list []SavedToolWindowLayout, name string) bool {
	_, ok := findSavedToolWindowLayout(list, name)
	return ok
}

func deleteToolWindowLayoutFromList(
	list []SavedToolWindowLayout, name string,
) []SavedToolWindowLayout {
	name = strings.TrimSpace(name)
	out := make([]SavedToolWindowLayout, 0, len(list))
	for _, entry := range list {
		if entry.Name != name {
			out = append(out, entry)
		}
	}
	return out
}

// exitDistractionFree mirrors the upstream layout actions: restoring a saved
// or default layout first leaves Distraction Free Mode.
func (m *Model) exitDistractionFree() {
	if !m.zen {
		return
	}
	m.prefs.Zen = false
	m.applyPreferences()
	m.savePreferences()
}

// OpenSaveCurrentLayout opens Save Current Layout as… with the active name as
// its initial value, as OpenIDEA's promptInput does.
func (m *Model) OpenSaveCurrentLayout() {
	if m.Root() == "" {
		m.status = "Open a project before saving a tool window layout"
		m.invalidate()
		return
	}
	m.layoutDlg = &toolWindowLayoutDialog{
		open: true,
		mode: layoutDialogSave,
		name: m.activeNamedLayout,
	}
	m.invalidate()
}

// OpenRestoreToolWindowLayout restores arg by name when the menu or automation
// supplies one; otherwise it opens OpenIDEA's chooser.
func (m *Model) OpenRestoreToolWindowLayout(arg any) {
	if m.Root() == "" {
		m.status = "Open a project before restoring a tool window layout"
		m.invalidate()
		return
	}
	if name, ok := arg.(string); ok && strings.TrimSpace(name) != "" {
		if !m.RestoreToolWindowLayout(name) {
			m.status = fmt.Sprintf("Tool window layout “%s” was not found", strings.TrimSpace(name))
			m.invalidate()
		}
		return
	}
	if len(m.namedLayouts) == 0 {
		m.status = "No saved layouts. Use Save Current Layout as… first."
		m.invalidate()
		return
	}
	m.layoutDlg = &toolWindowLayoutDialog{
		open: true,
		mode: layoutDialogRestore,
	}
	m.selectActiveLayout()
	m.invalidate()
}

// OpenRenameToolWindowLayout opens the saved-layout chooser used by Rename
// Layout…, which is enabled only while at least one layout exists.
func (m *Model) OpenRenameToolWindowLayout() {
	m.openLayoutChooser(layoutDialogRename)
}

// OpenDeleteToolWindowLayout opens the chooser and confirmation behind Delete
// Layout….
func (m *Model) OpenDeleteToolWindowLayout() {
	m.openLayoutChooser(layoutDialogDelete)
}

func (m *Model) openLayoutChooser(mode string) {
	if len(m.namedLayouts) == 0 {
		return
	}
	m.layoutDlg = &toolWindowLayoutDialog{open: true, mode: mode}
	m.selectActiveLayout()
	m.invalidate()
}

func (m *Model) selectActiveLayout() {
	if m.layoutDlg == nil {
		return
	}
	m.layoutDlg.selected = 0
	for i, entry := range m.namedLayouts {
		if entry.Name == m.activeNamedLayout {
			m.layoutDlg.selected = i
			break
		}
	}
}

// CloseToolWindowLayoutDialog cancels the open prompt or chooser.
func (m *Model) CloseToolWindowLayoutDialog() {
	m.layoutDlg = nil
	m.invalidate()
}

// SaveCurrentToolWindowLayout commits the Save Current Layout as… prompt.
// Saving an existing name replaces that entry and keeps its position.
func (m *Model) SaveCurrentToolWindowLayout() {
	d := m.layoutDlg
	if d == nil || d.mode != layoutDialogSave {
		return
	}
	name := strings.TrimSpace(d.name)
	if name == "" {
		d.err = "Enter a layout name"
		m.invalidate()
		return
	}
	m.namedLayouts = saveToolWindowLayoutInList(m.namedLayouts, name, m.currentLayout())
	m.activeNamedLayout = name
	m.layoutDlg = nil
	m.rememberLayout()
	m.saveSession()
	m.status = fmt.Sprintf("Saved the tool window layout “%s”", name)
	m.invalidate()
}

// RestoreToolWindowLayout applies a named snapshot to the current project.
func (m *Model) RestoreToolWindowLayout(name string) bool {
	entry, ok := findSavedToolWindowLayout(m.namedLayouts, name)
	if !ok {
		return false
	}
	m.exitDistractionFree()
	m.applyLayoutSnapshot(entry.Layout)
	m.activeNamedLayout = entry.Name
	m.rememberLayout()
	m.saveSession()
	m.layoutDlg = nil
	m.status = fmt.Sprintf("Restored the tool window layout “%s”", entry.Name)
	m.invalidate()
	return true
}

// ChooseToolWindowLayout handles a row in the restore, rename or delete
// chooser. Restore applies immediately; delete asks before removing.
func (m *Model) ChooseToolWindowLayout(index int) {
	d := m.layoutDlg
	if d == nil || index < 0 || index >= len(m.namedLayouts) {
		return
	}
	d.selected = index
	entry := m.namedLayouts[index]
	switch d.mode {
	case layoutDialogRestore:
		m.RestoreToolWindowLayout(entry.Name)
	case layoutDialogRename:
		d.renameFrom = entry.Name
		d.name = entry.Name
		d.mode = layoutDialogRename + "-name"
		d.err = ""
		m.invalidate()
	case layoutDialogDelete:
		d.name = entry.Name
		d.confirm = true
		d.mode = layoutDialogDelete + "-confirm"
		m.invalidate()
	}
}

// RenameToolWindowLayout commits the Rename Layout name prompt. A collision
// with another layout keeps the prompt open and reports the same validation
// as OpenIDEA.
func (m *Model) RenameToolWindowLayout() {
	d := m.layoutDlg
	if d == nil || d.mode != layoutDialogRename+"-name" {
		return
	}
	next, ok := renameToolWindowLayoutInList(m.namedLayouts, d.renameFrom, d.name)
	if !ok {
		to := strings.TrimSpace(d.name)
		if to == "" {
			d.err = "Enter a layout name"
		} else {
			d.err = "A layout with this name exists"
		}
		m.invalidate()
		return
	}
	old := d.renameFrom
	name := strings.TrimSpace(d.name)
	m.namedLayouts = next
	if m.activeNamedLayout == old {
		m.activeNamedLayout = name
	}
	m.layoutDlg = nil
	m.saveSession()
	m.status = fmt.Sprintf("Renamed the tool window layout “%s” to “%s”", old, name)
	m.invalidate()
}

// DeleteToolWindowLayout confirms removal of the chosen saved snapshot.
func (m *Model) DeleteToolWindowLayout() {
	d := m.layoutDlg
	if d == nil || d.mode != layoutDialogDelete+"-confirm" {
		return
	}
	name := d.name
	m.namedLayouts = deleteToolWindowLayoutFromList(m.namedLayouts, name)
	if m.activeNamedLayout == name {
		m.activeNamedLayout = ""
	}
	m.layoutDlg = nil
	m.saveSession()
	m.status = fmt.Sprintf("Deleted the tool window layout “%s”", name)
	m.invalidate()
}

// StepToolWindowLayoutDialog moves the chooser selection with the arrow keys.
func (m *Model) StepToolWindowLayoutDialog(delta int) {
	d := m.layoutDlg
	if d == nil || len(m.namedLayouts) == 0 {
		return
	}
	switch d.mode {
	case layoutDialogRestore, layoutDialogRename, layoutDialogDelete:
		d.selected = (d.selected + delta + len(m.namedLayouts)) % len(m.namedLayouts)
		m.invalidate()
	}
}

func (m *Model) toolWindowLayoutDialog(c *ui.Context, p *ui.Theme) {
	d := m.layoutDlg
	if d == nil || !d.open {
		return
	}
	open := d.open
	dialog := ui.Modal(c.Key("tool-window-layouts"), &open, func() {
		m.drawToolWindowLayoutDialog(c, p, d)
	})
	if dialog.Dismissed() || !open {
		m.layoutDlg = nil
	}
}

func (m *Model) drawToolWindowLayoutDialog(
	c *ui.Context, p *ui.Theme, d *toolWindowLayoutDialog,
) {
	width := float32(460)
	title := "Tool Window Layouts"
	switch d.mode {
	case layoutDialogSave:
		title = "Save Current Layout as"
	case layoutDialogRestore:
		title = "Restore Layout"
	case layoutDialogRename:
		title = "Rename Layout"
	case layoutDialogRename + "-name":
		title = "Rename Layout"
	case layoutDialogDelete:
		title = "Delete Layout"
	case layoutDialogDelete + "-confirm":
		title = "Delete Layout"
	}
	ui.Column(c).Width(width).Gap(9).Children(func() {
		ui.Text(c, title).FontSize(14).Bold().TextColor(p.Text)
		switch d.mode {
		case layoutDialogSave, layoutDialogRename + "-name":
			ui.Text(c, "Layout name:").FontSize(12).TextColor(p.TextMuted)
			in := ui.TextInput(c.Key("tool-window-layout-name"), &d.name).AutoFocus()
			if in.Submitted() {
				if d.mode == layoutDialogSave {
					m.SaveCurrentToolWindowLayout()
				} else {
					m.RenameToolWindowLayout()
				}
			}
			if d.err != "" {
				ui.Text(c, d.err).FontSize(12).TextColor(p.Danger)
			}
			ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
				label := "Save"
				if d.mode == layoutDialogRename+"-name" {
					label = "Rename"
				}
				if ui.PrimaryButton(c, label).Clicked() {
					if d.mode == layoutDialogSave {
						m.SaveCurrentToolWindowLayout()
					} else {
						m.RenameToolWindowLayout()
					}
				}
				if ui.Button(c, "Cancel").Clicked() {
					m.CloseToolWindowLayoutDialog()
				}
			})
		case layoutDialogDelete + "-confirm":
			ui.Text(c, fmt.Sprintf("Delete the layout '%s'?", d.name)).
				FontSize(12).TextColor(p.Text)
			ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
				ui.Button(c, "Delete").OnClick(m.DeleteToolWindowLayout)
				if ui.Button(c, "Cancel").Clicked() {
					m.CloseToolWindowLayoutDialog()
				}
			})
		default:
			ui.Text(c, "Choose a saved layout:").FontSize(12).TextColor(p.TextMuted)
			if len(m.namedLayouts) == 0 {
				ui.Text(c, "No saved layouts. Use Save Current Layout as… first.").
					FontSize(12).TextColor(p.TextMuted)
			}
			ui.Scroll(c).MaxHeight(300).Children(func() {
				ui.Column(c).Gap(2).Children(func() {
					for i, entry := range m.namedLayouts {
						i, entry := i, entry
						row := ui.Row(c).Height(28).Padding(0, 8).Gap(8).
							Radius(4).AlignItems(ui.Center)
						if i == d.selected {
							row.Background(p.Selection)
						}
						row.OnClick(func() { m.ChooseToolWindowLayout(i) }).Children(func() {
							ui.Text(c, entry.Name).FontSize(12).TextColor(p.Text).
								Grow(1).SingleLine().Ellipsis("…")
							if entry.Name == m.activeNamedLayout {
								ui.Text(c, "✓").FontSize(12).TextColor(p.TextMuted)
							}
						})
					}
				})
			})
			ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
				label := "Restore"
				if d.mode == layoutDialogRename {
					label = "Rename…"
				} else if d.mode == layoutDialogDelete {
					label = "Delete…"
				}
				if ui.PrimaryButton(c, label).Clicked() {
					m.ChooseToolWindowLayout(d.selected)
				}
				if ui.Button(c, "Cancel").Clicked() {
					m.CloseToolWindowLayoutDialog()
				}
			})
		}
	})
}
