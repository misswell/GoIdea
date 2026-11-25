package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

// The Structure tool window is OpenIDEA's Cmd+7 window: the active file's
// outline in the left slot, filtered as you type, with a row that jumps to the
// symbol's declaration.

// StructurePane is the window's state: the filter and the collapsed symbols.
// Both survive a close, as the tool window keeps its state.
type StructurePane struct {
	visible   bool
	popup     bool
	filter    string
	collapsed map[string]bool
}

// StructureRow is one line of the outline the window shows.
type StructureRow struct {
	Name string
	Kind string
	Path string
	Line int
	Col  int
	// Depth indents a nested symbol under its container.
	Depth int
	// Container marks a filter-only ancestor: it is shown to lead to a match,
	// not because the symbol itself matched.
	Container bool
	HasKids   bool
	Collapsed bool
}

// ShowStructure opens the Structure window in the left side's split area, the
// slot OpenIDEA gives it below the Project window.
func (m *Model) ShowStructure() { m.DockShow("structure") }

// ToggleStructure is Cmd+7: a second press closes the window and the split area
// goes with it, leaving the window above it alone, as OpenIDEA's
// activateToolWindow hides a window that already has the slot.
func (m *Model) ToggleStructure() { m.DockToggle("structure") }

// CloseStructure hides the window; the filter and collapsed symbols stay for
// the next open.
func (m *Model) CloseStructure() { m.DockHide("structure") }

// OpenFileStructure opens the popup form of File Structure. It uses the same
// live outline as the tool window and keeps both entry points in sync.
func (m *Model) OpenFileStructure() {
	m.refreshStructure()
	m.structure.popup = true
	m.invalidate()
}

func (m *Model) fileStructureDialog(c *ui.Context, p *ui.Theme) {
	open := m.structure.popup
	dialog := nativeModal(c.Key("file-structure"), &open, func() {
		ui.Column(c).Width(620).Children(func() {
			ui.Row(c).Padding(8, 12).Children(func() {
				ui.Text(c, "File Structure").FontSize(13).Bold().TextColor(p.Text).SingleLine()
			})
			if t := m.tab(); t != nil && t.outlineLoading && len(t.outline) == 0 {
				ui.Row(c).Padding(8, 12).Children(func() {
					ui.Text(c, "Reading the structure…").FontSize(12).TextColor(p.TextMuted)
				})
			}
			ui.Scroll(c).Height(420).Children(func() {
				m.structureRowsUI(c, p)
			})
		})
	})
	dialog.Padding(0)
	if dialog.Dismissed() || !open {
		m.structure.popup = false
		m.invalidate()
	}
}

// toggleStructure collapses or expands one symbol.
func (m *Model) toggleStructure(path string) {
	if m.structure.collapsed == nil {
		m.structure.collapsed = map[string]bool{}
	}
	if m.structure.collapsed[path] {
		delete(m.structure.collapsed, path)
	} else {
		m.structure.collapsed[path] = true
	}
	m.invalidate()
}

// refreshStructure loads the active file's outline unless the one it has is
// current; the outline also feeds the breadcrumbs, so it may already be there.
func (m *Model) refreshStructure() {
	t := m.tab()
	if t == nil || t.outlineLoading {
		return
	}
	v := t.doc.Buffer().Version()
	if t.outlineDoc == t.doc.ID() && t.outlineVersion == v {
		return
	}
	m.kickOutline(t, v)
}

// structureRows flattens an outline for the window. A filter keeps a symbol
// that matches or has a matching descendant, marks the ancestors it only keeps
// as containers, and expands collapsed symbols so matches stay visible.
func structureRows(items []openidea.OutlineItem, filter string, collapsed map[string]bool, depth int, prefix string) []StructureRow {
	q := strings.ToLower(strings.TrimSpace(filter))
	var rows []StructureRow
	for _, it := range items {
		path := prefix + "/" + it.Name + "@" + fmt.Sprint(it.Range.Start.Line)
		kids := structureRows(it.Children, q, collapsed, depth+1, path)
		match := q == "" || strings.Contains(strings.ToLower(it.Name), q)
		if !match && len(kids) == 0 {
			continue
		}
		row := StructureRow{
			Name:      it.Name,
			Kind:      it.Kind,
			Path:      path,
			Line:      int(it.SelectionRange.Start.Line),
			Col:       int(it.SelectionRange.Start.Col),
			Depth:     depth,
			Container: !match,
			HasKids:   len(it.Children) > 0,
			Collapsed: q == "" && collapsed[path],
		}
		rows = append(rows, row)
		if !row.Collapsed {
			rows = append(rows, kids...)
		}
	}
	return rows
}

// structureOutline is the file the window reads and its outline.
func (m *Model) structureOutline() (string, []openidea.OutlineItem) {
	t := m.tab()
	if t == nil {
		return "", nil
	}
	return t.doc.Path(), t.outline
}

// structureRowsUI draws the filter and the outline rows in the sidebar.
func (m *Model) structureRowsUI(c *ui.Context, p *ui.Theme) {
	s := &m.structure
	ui.Row(c).Padding(6, 8).Children(func() {
		ui.TextInput(c, &s.filter).Grow(1).Placeholder("Filter symbols")
	})
	path, items := m.structureOutline()
	if path == "" {
		ui.Row(c).Padding(8, 8).Children(func() {
			ui.Text(c, "Open a file, then look at its structure").FontSize(12).TextColor(p.TextMuted)
		})
		return
	}
	ui.Row(c).Padding(0, 8).Children(func() {
		ui.Text(c, filepath.Base(path)).FontSize(12).TextColor(p.TextMuted).
			SingleLine().Ellipsis("…")
	})
	if t := m.tab(); t != nil && t.outlineLoading && len(items) == 0 {
		ui.Row(c).Padding(8, 8).Children(func() {
			ui.Text(c, "Reading the structure…").FontSize(12).TextColor(p.TextMuted)
		})
		return
	}
	rows := structureRows(items, s.filter, s.collapsed, 0, "")
	if len(rows) == 0 {
		hint := "This file has no structure to show"
		if strings.TrimSpace(s.filter) != "" {
			hint = "No matching symbols"
		}
		ui.Row(c).Padding(8, 8).Children(func() {
			ui.Text(c, hint).FontSize(12).TextColor(p.TextMuted)
		})
		return
	}
	oi := oiPaletteOf(p)
	ui.Column(c).Padding(0, 6).Children(func() {
		for i, row := range rows {
			i, row := i, row
			r := ui.Row(c.Key(fmt.Sprintf("structure:%d", i))).Height(24).
				PaddingStart(4 + float32(row.Depth)*20).PaddingEnd(8).Gap(4).
				Radius(5).AlignItems(ui.Center)
			if r.Hovered() {
				r.Background(oi.hoverTint)
			}
			r.Children(func() {
				// The chevron slot is reserved on every row, as the tree does.
				chevron := ui.Row(c).Size(16, 16).Shrink(0).
					Justify(ui.Center).AlignItems(ui.Center)
				if row.HasKids {
					glyph := "general/chevronDown"
					if row.Collapsed {
						glyph = "general/chevronRight"
					}
					chevron.Children(func() {
						oiIconEl(c, glyph, 16, oi, oi.textMuted)
					})
					if chevron.Clicked() {
						m.toggleStructure(row.Path)
					}
				}
				if asset := symbolIcon(strings.ToLower(row.Kind)); asset != "" {
					oiIconEl(c, asset, 16, oi, oi.textSubtle).MarginEnd(2)
				} else if row.HasKids {
					ui.Row(c).Size(16, 16).Shrink(0).MarginEnd(2)
				}
				color := oi.textStrong
				if row.Container {
					color = oi.textMuted
				}
				ui.Text(c, row.Name).FontSize(13).TextColor(color).SingleLine().Ellipsis("…")
				ui.Spacer(c)
				ui.Text(c, symbolKindLabel(row.Kind)).FontSize(13).TextColor(oi.textSubtle).SingleLine()
			})
			if r.Clicked() {
				m.structure.popup = false
				m.revealAt(path, row.Line, row.Col)
			}
		}
	})
}

// structureReport opens the Structure window and reports what it would show:
// the file, the rows and the first symbol. Call it on the main thread.
func (m *Model) structureReport() map[string]any {
	was := m.structure.visible
	m.ShowStructure()
	path, items := m.structureOutline()
	rows := structureRows(items, "", nil, 0, "")
	out := map[string]any{
		"path":    path,
		"rows":    len(rows),
		"visible": m.structure.visible,
	}
	if len(rows) > 0 {
		out["first"] = rows[0].Name
		out["firstKind"] = rows[0].Kind
	}
	if !was {
		m.CloseStructure()
	}
	return out
}
