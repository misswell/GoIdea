package app

// Distraction Free Mode, ported from OpenIDEA's distractionFree.ts: the same
// action enters and exits, tool windows, the stripe, the toolbar contents, the
// status bar, editor tabs, breadcrumbs and the gutter are hidden, and the text
// column is centred at the width Settings › Appearance configures. The mode is
// a preference, so it survives a restart, and Esc does not leave it. Entering
// shows IDEA's hint on how to leave, since the toolbar and status bar are gone.

import (
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

const (
	defaultZenWidth = 80
	minZenWidth     = 20
	maxZenWidth     = 500
	// oiCompactHeader is OpenIDEA's --h-compact-header, the bar the compact
	// title sits on.
	oiCompactHeader float32 = 30
)

// zenHintText is IDEA's way out, with GoIDEA's Find Action shortcut.
const zenHintText = "To exit Distraction Free Mode, use View | Appearance | Exit Distraction Free Mode or Find Action (⌘⇧A)."

// ToggleZen enters or leaves distraction free mode, as editor.toggleZen does.
func (m *Model) ToggleZen() {
	entering := !m.zen
	m.prefs.Zen = entering
	if entering {
		m.zenHint = true
	}
	m.applyPreferences()
	m.savePreferences()
	m.status = zenTitle(m)
	m.invalidate()
}

// StepZenWidth moves the distraction free text width by delta columns, clamped
// to IDEA's 20..500.
func (m *Model) StepZenWidth(delta int) {
	w := m.prefs.zenWidth() + delta
	if w < minZenWidth {
		w = minZenWidth
	}
	if w > maxZenWidth {
		w = maxZenWidth
	}
	if w == m.prefs.zenWidth() {
		return
	}
	m.prefs.ZenWidth = w
	m.savePreferences()
	m.invalidate()
}

// ToggleFullScreen switches the window between windowed and full screen, as
// window.toggleFullScreen does.
func (m *Model) ToggleFullScreen() {
	if m.win == nil {
		return
	}
	m.win.ToggleFullScreen()
	m.invalidate()
}

// applyZenGutter hides the gutter in distraction free mode, as IDEA hides it
// with the line numbers and fold markers, and restores it on the way out.
func (m *Model) applyZenGutter() {
	for _, t := range m.tabs {
		m.applyTabZenGutter(t)
	}
}

// applyTabZenGutter gives one editor the gutter the mode asks for, including a
// tab opened while the mode is on.
func (m *Model) applyTabZenGutter(t *Tab) {
	if m.zen {
		t.state.LineNumbers = editor.LineNumbersOff
		t.state.NoFolding = true
	} else {
		t.state.LineNumbers = editor.LineNumbersOn
		t.state.NoFolding = false
	}
}

// zenColumnWidth is the width of the centred text column in DIPs. A monospace
// glyph advances about 0.6em, which is what IDEA's column measured in ch is.
func (m *Model) zenColumnWidth() float32 {
	w := float32(m.prefs.zenWidth()) * m.prefs.fontSize() * 0.6
	if w < 200 {
		w = 200
	}
	return w
}

// zenWorkbench is the distraction free frame: the compact header, then the
// editor island alone, centred, with no stripe, tool windows or status bar.
func (m *Model) zenWorkbench(c *ui.Context, p *ui.Theme) {
	oi := oiPaletteOf(p)
	ui.Column(c).Fill().Background(oi.frame).Children(func() {
		m.zenHeader(c, oi)
		ui.Row(c).Grow(1).MinHeight(0).MinWidth(0).
			Padding(0, 6, 0, 6).AlignItems(ui.Stretch).Children(func() {
			m.zenEditor(c, p, oi)
		})
	})
}

// zenHeader is OpenIDEA's compact header: the open file's name, centred, on a
// 30px bar and no widgets at all.
func (m *Model) zenHeader(c *ui.Context, oi oiColors) {
	title := "GoIDEA"
	if t := m.tab(); t != nil {
		title = filepath.Base(t.doc.Path())
	}
	ui.Row(c).Height(oiCompactHeader).Shrink(0).Padding(0, 10).
		AlignItems(ui.Center).Children(func() {
		ui.Row(c).Grow(1).Justify(ui.Center).Children(func() {
			ui.Text(c, title).FontSize(13).TextColor(oi.textSecondary).
				SingleLine().Ellipsis("…")
		})
	})
}

// zenEditor is the editor island of distraction free mode: the hint bar, then
// the text column centred at the configured width.
func (m *Model) zenEditor(c *ui.Context, p *ui.Theme, oi oiColors) {
	island := ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Background(oi.island).
		Radius(oiIslandRadius)
	m.editorWidth = island.Bounds().W
	m.editorHeight = island.Bounds().H
	island.Children(func() {
		if m.zenHint {
			m.zenHintRow(c, p)
		}
		t := m.tab()
		if t == nil {
			m.emptyEditor(c, p)
			return
		}
		ui.Row(c).Grow(1).MinHeight(0).MinWidth(0).Justify(ui.Center).Children(func() {
			editor.View(c, t.state).Width(m.zenColumnWidth()).MinWidth(0).MinHeight(0)
		})
	})
}

// zenHintRow is IDEA's hint balloon on the way in, with the action that
// leaves the mode and a place to put it away.
func (m *Model) zenHintRow(c *ui.Context, p *ui.Theme) {
	oi := oiPaletteOf(p)
	ui.Row(c).Height(26).Shrink(0).Padding(0, 10).Gap(8).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(oi.border).Children(func() {
		ui.Text(c, zenHintText).FontSize(12).TextColor(oi.textMuted).
			SingleLine().Ellipsis("…").Grow(1)
		ui.Button(c, "Exit Distraction Free Mode").OnClick(func() { m.ToggleZen() })
		ui.Button(c, "×").OnClick(func() {
			m.zenHint = false
			m.invalidate()
		})
	})
}

// SmokeZen drives distraction free mode offscreen: the mode, the gutter, the
// persisted choice and the restore on the way out.
func (m *Model) SmokeZen() map[string]any {
	out := map[string]any{}
	mygo.RunOnMain(func() {
		was, wasHint := m.zen, m.zenHint
		m.ToggleZen()
		out["on"] = m.zen
		out["title"] = zenTitle(m)
		out["stored"] = LoadPreferences(m.cfg.SettingsPath).Zen
		if t := m.tab(); t != nil {
			out["gutterOff"] = t.state.LineNumbers == editor.LineNumbersOff && t.state.NoFolding
		}
		out["hint"] = m.zenHint
		out["width"] = m.prefs.zenWidth()
		m.StepZenWidth(1)
		out["widthAfter"] = m.prefs.zenWidth()
		m.ToggleZen()
		out["off"] = !m.zen
		out["hintOff"] = !m.zenHint
		if t := m.tab(); t != nil {
			out["gutterOn"] = t.state.LineNumbers == editor.LineNumbersOn && !t.state.NoFolding
		}
		m.prefs.Zen, m.prefs.ZenWidth = was, 0
		m.zen, m.zenHint = was, wasHint
		m.applyPreferences()
		m.savePreferences()
	})
	return out
}
