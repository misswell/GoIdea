package app

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
)

func (m *Model) loadVimSettingsPage() {
	if !m.vim.rcLoaded {
		m.reloadVimRc(true)
	}
}

// settingsVimBody is Settings › Editor › Vim. It stages the global switch and
// IdeaVim's shortcut conflict table, and exposes the rc file's parsed state
// without leaving the settings window.
func (m *Model) settingsVimBody(c *ui.Context, oi oiColors) {
	enabled := m.settingsDraft.vimEnabled
	if settingsCheckbox(c, &enabled, "Enable Vim emulation").Changed() {
		m.settingsDraft.vimEnabled = enabled
		m.invalidate()
	}

	m.settingsSectionTitle(c, oi, "Vim rc file")
	ui.Row(c).MinHeight(settingsFormRow).Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "Configuration file").FontSize(12).Bold().TextColor(oi.textStrong).SingleLine()
		ui.Button(c, "Reload").OnClick(func() {
			m.reloadVimRc(false)
			m.vim.rcLoaded = true
		})
	})
	rc := m.vim.rc
	ui.Text(c, vimRcDescription(rc)).FontSize(12).TextColor(oi.textMuted).
		SingleLine().Ellipsis("…")
	ui.Text(c,
		"Supported: set options, map / nmap / imap / vmap / noremap variants, "+
			"let mapleader, source, and :action / <Action>() mappings. :w saves, :q closes the tab.",
	).FontSize(11).TextColor(oi.textMuted).MaxWidth(760)
	if len(rc.Ignored) > 0 {
		ui.Text(c, "Ignored lines").FontSize(11).Bold().TextColor(oi.textStrong).
			PaddingStart(20).SingleLine()
		for i, ignored := range rc.Ignored {
			if i == 8 {
				ui.Text(c, fmt.Sprintf("…and %d more", len(rc.Ignored)-i)).
					FontSize(11).TextColor(oi.textMuted).PaddingStart(32).SingleLine()
				break
			}
			location := fmt.Sprintf("%d", ignored.Line)
			if ignored.File != "" {
				location = ignored.File + ":" + location
			}
			ui.Text(c, fmt.Sprintf("%s: %s — %s", location, ignored.Text, ignored.Reason)).
				FontSize(11).TextColor(oi.textMuted).PaddingStart(32).
				SingleLine().Ellipsis("…")
		}
	}

	m.settingsSectionTitle(c, oi, "Shortcut conflicts")
	ui.Text(c,
		"With Vim as the handler the key goes to the editor while Vim mode is on. "+
			"With IDE, an enabled IDE action runs first.",
	).FontSize(11).TextColor(oi.textMuted).MaxWidth(760)
	m.settingsVimConflictHeader(c, oi)
	for _, key := range vimKeys {
		m.settingsVimConflictRow(c, oi, key)
	}
	ui.Row(c).Padding(0, 0, 8, 0).Children(func() {
		button := ui.Button(c, "Reset to Defaults").Disabled(len(m.settingsDraft.vimHandlers) == 0)
		button.OnClick(func() {
			m.settingsDraft.vimHandlers = nil
			m.invalidate()
		})
	})
}

func (m *Model) settingsVimConflictHeader(c *ui.Context, oi oiColors) {
	ui.Row(c).Height(28).Gap(10).AlignItems(ui.Center).
		Padding(0, 8).Background(oi.frame).Children(func() {
		settingsVimCell(c, oi, "Shortcut", 112, true)
		settingsVimCell(c, oi, "IDE action", 250, true)
		settingsVimCell(c, oi, "Vim", 250, true)
		settingsVimCell(c, oi, "Handler", 90, true)
	})
}

func (m *Model) settingsVimConflictRow(c *ui.Context, oi oiColors, key vimKey) {
	stroke := keymapNormalize(key.stroke)
	actions := vimConflictActions(m, stroke)
	actionText := "—"
	if len(actions) > 0 {
		actionText = strings.Join(actions, ", ")
	}
	handler := m.settingsDraft.vimHandlers[key.stroke]
	if handler != "vim" && handler != "ide" {
		handler = key.handler
	}
	ui.Row(c.Key("settings:vim:"+stroke)).MinHeight(settingsFormRow).Gap(10).AlignItems(ui.Center).
		Padding(0, 8).Children(func() {
		settingsVimCell(c, oi, key.stroke, 112, false)
		settingsVimCell(c, oi, actionText, 250, false)
		settingsVimCell(c, oi, key.action, 250, false)
		shown := "IDE"
		if handler == "vim" {
			shown = "Vim"
		}
		sel := ui.Select(c, &shown, []string{"IDE", "Vim"}).Width(90)
		if sel.Changed() {
			m.setVimHandler(key.stroke, strings.ToLower(shown))
		}
	})
}

func settingsVimCell(c *ui.Context, oi oiColors, text string, width float32, bold bool) {
	row := ui.Text(c, text).FontSize(11.5).Width(width).
		TextColor(map[bool]ui.Color{true: oi.textStrong, false: oi.text}[bold]).
		SingleLine().Ellipsis("…")
	if bold {
		row.Bold()
	}
}
