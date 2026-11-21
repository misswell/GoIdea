package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
)

const (
	copyPathPopupRowHeight = 32
	copyPathPopupHeader    = 36
	copyPathPopupFooter    = 25
)

type copyPathPopupRow struct {
	id        string
	label     string
	value     string
	reference bool
}

type copyReferencePreview struct {
	value   string
	err     string
	loading bool
}

type copyPathPopupState struct {
	tab    *Tab
	anchor ui.Rect
	rows   []copyPathPopupRow
	active int

	reference    copyReferencePreview
	referenceGen uint64
	focus        ui.Handle
}

// OpenCopyPathReferencePopup is Edit › Copy Path/Reference…. The list is a
// snapshot anchored at the caret, including an asynchronously resolved
// Reference row.
func (m *Model) OpenCopyPathReferencePopup() {
	t := m.tab()
	if t == nil || t.doc == nil || t.state == nil || t.doc.Path() == "" || !m.tabOpen(t) {
		return
	}
	line := t.state.CursorPos().Line + 1
	state := &copyPathPopupState{
		tab: t, anchor: m.usagePopupAnchor,
		rows:      copyPathPopupRows(m.Root(), t.doc.Path(), line),
		reference: copyReferencePreview{loading: true},
	}
	state.referenceGen++
	m.copyPathPopup = state

	result := m.resolveEditorReference(t)
	generation := state.referenceGen
	go func() {
		resolved := <-result
		m.post(func() {
			m.finishCopyPathReference(state, generation, resolved)
		})
	}()
	m.invalidate()
}

func copyPathPopupRows(root, path string, line int) []copyPathPopupRow {
	flavours := editorPathFlavours(root, path, line)
	rows := []copyPathPopupRow{
		{id: "editor.copyPath", label: flavours[0].label, value: flavours[0].value},
		{id: "project.copyFileName", label: flavours[1].label, value: flavours[1].value},
		{id: "project.copyRelativePath", label: flavours[2].label, value: flavours[2].value},
	}
	if source, ok := editorSourceRootPath(root, path); ok {
		rows = append(rows, copyPathPopupRow{
			id: "project.copySourceRootPath", label: "Path From Source Root", value: source,
		})
	}
	rows = append(rows,
		copyPathPopupRow{
			id:    "editor.copyPathWithLineNumber",
			label: "Path with line number", value: flavours[3].value,
		},
		copyPathPopupRow{
			id: "editor.copyReference", label: "Reference", reference: true,
		},
	)
	return rows
}

// editorSourceRootPath finds the conventional Maven source root containing a
// file and returns the path below it. Unknown layouts omit the chooser row.
func editorSourceRootPath(root, path string) (string, bool) {
	if root == "" {
		return "", false
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for i := 0; i+3 < len(parts); i++ {
		if parts[i] != "src" {
			continue
		}
		candidate := filepath.Join(append([]string{root}, parts[:i+3]...)...)
		if !isSourceRoot(candidate) {
			continue
		}
		return strings.Join(parts[i+3:], "/"), true
	}
	return "", false
}

func (m *Model) finishCopyPathReference(
	state *copyPathPopupState,
	generation uint64,
	result editorReferenceResult,
) {
	if m.copyPathPopup != state || state.referenceGen != generation {
		return
	}
	state.reference.loading = false
	if result.err != nil {
		state.reference.err = "Cannot copy reference"
	} else {
		state.reference.value = result.text
	}
	m.invalidate()
}

func (m *Model) dismissCopyPathReferencePopup() {
	m.copyPathPopup = nil
	m.invalidate()
}

func (m *Model) stepCopyPathReferencePopup(delta int) {
	state := m.copyPathPopup
	if state == nil || len(state.rows) == 0 {
		return
	}
	state.active = (state.active + delta + len(state.rows)) % len(state.rows)
	m.invalidate()
}

func (m *Model) chooseCopyPathReferencePopup() {
	state := m.copyPathPopup
	if state == nil || state.active < 0 || state.active >= len(state.rows) {
		return
	}
	m.chooseCopyPathPopupRow(state, state.rows[state.active])
}

func (m *Model) chooseCopyPathPopupRow(state *copyPathPopupState, row copyPathPopupRow) {
	if m.copyPathPopup != state {
		return
	}
	value := row.value
	if row.reference {
		if state.reference.loading || state.reference.err != "" || state.reference.value == "" {
			return
		}
		value = state.reference.value
	}
	if value == "" {
		return
	}
	m.copyPathPopup = nil
	if row.reference {
		m.copyReferenceText(value)
		return
	}
	m.copyEditorText(value)
}

func (m *Model) copyPathReferencePopup(c *ui.Context, p *ui.Theme) {
	state := m.copyPathPopup
	if state == nil {
		return
	}
	if !m.tabOpen(state.tab) || m.tab() != state.tab ||
		state.tab.doc == nil || state.tab.doc.Path() == "" {
		m.copyPathPopup = nil
		return
	}

	oi := oiPaletteOf(p)
	width, height := c.Size()
	w := max(float32(280), min(float32(480), width-16))
	h := min(float32(copyPathPopupHeader+len(state.rows)*copyPathPopupRowHeight+copyPathPopupFooter),
		height-16)
	anchor := state.anchor
	if anchor.W <= 0 || anchor.H <= 0 {
		anchor = ui.Rect{X: 24, Y: 24, W: 1, H: 18}
	}
	x := anchor.X
	y := anchor.Y + anchor.H + 4
	if y+h > height-8 {
		y = anchor.Y - h - 4
	}
	x = max(float32(8), min(x, width-w-8))
	y = max(float32(8), min(y, height-h-8))

	ui.Overlay(c, func() {
		box := ui.Column(c.Key("copy-path-reference:popup")).Role(ui.RolePopup).
			Label("Copy Path/Reference…").Focusable().AutoFocus().Bind(&state.focus).
			Absolute().Left(x).Top(y).Width(w).Height(h).MinWidth(0).Clip().
			Background(oi.popup).Border(1, oi.borderStrong).Radius(7).
			Shadow(0, 10, 28, 0, ui.RGBA(0, 0, 0, 0.3))
		box.Children(func() {
			ui.Row(c).Height(copyPathPopupHeader).Shrink(0).Padding(0, 12).Gap(8).
				AlignItems(ui.Center).Children(func() {
				oiIconEl(c, "general/copy", 15, oi, oi.textMuted)
				ui.Text(c, "Copy Path/Reference…").FontSize(12).Bold().
					TextColor(oi.textStrong).SingleLine()
			})
			ui.Row(c).Height(1).Shrink(0).Background(oi.border)
			ui.Scroll(c.Key("copy-path-reference:rows")).Grow(1).MinHeight(0).
				MinWidth(0).Children(func() {
				ui.Column(c).Padding(4, 0).Children(func() {
					for i, row := range state.rows {
						m.copyPathPopupRow(c, oi, state, i, row)
					}
				})
			})
			ui.Row(c).Height(copyPathPopupFooter).Shrink(0).Padding(0, 10).
				AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).
				BorderColor(oi.border).Children(func() {
				ui.Text(c, "↑/↓ select · Enter copy · Esc close").
					FontSize(11).TextColor(oi.textMuted).SingleLine()
			})
		})
		switch {
		case box.PressedOutside() || box.OverlayShortcut(0, ui.KeyEscape):
			m.dismissCopyPathReferencePopup()
		case box.OverlayShortcut(0, ui.KeyUp):
			m.stepCopyPathReferencePopup(-1)
		case box.OverlayShortcut(0, ui.KeyDown):
			m.stepCopyPathReferencePopup(1)
		case box.OverlayShortcut(0, ui.KeyEnter):
			m.chooseCopyPathReferencePopup()
		}
	})
}

func (m *Model) copyPathPopupRow(
	c *ui.Context,
	oi oiColors,
	state *copyPathPopupState,
	index int,
	row copyPathPopupRow,
) {
	value := row.value
	disabled := false
	if row.reference {
		switch {
		case state.reference.loading:
			value = "Resolving reference…"
			disabled = true
		case state.reference.err != "":
			value = state.reference.err
			disabled = true
		default:
			value = state.reference.value
		}
	}
	button := ui.ButtonBase(c.Key(fmt.Sprintf("copy-path-reference:%d", index))).
		Height(copyPathPopupRowHeight).Padding(0, 12).Radius(4).
		AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if index == state.active {
		button.Background(oi.selected)
	} else if button.Hovered() {
		button.Background(oi.toolbarHover)
	}
	button.Disabled(disabled)
	button.Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, row.label).Width(160).FontSize(12).
				TextColor(oi.textStrong).SingleLine()
			text := ui.Text(c, value).Grow(1).FontSize(11).
				TextColor(oi.textMuted).SingleLine().Ellipsis("…")
			if row.reference && index == state.active {
				text.TextColor(oi.link)
			}
			if row.reference && !disabled {
				ui.Text(c, "Cmd+Alt+Shift+C").FontSize(10).
					TextColor(oi.textFaint).SingleLine()
			}
		})
	})
	button.OnClick(func() {
		state.active = index
		m.chooseCopyPathPopupRow(state, row)
	})
}
