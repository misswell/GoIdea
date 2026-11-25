package app

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

func (m *Model) searchScopesDialog(c *ui.Context, p *ui.Theme) {
	s := &m.search
	if !s.scopesOpen {
		return
	}
	open := s.scopesOpen
	dialog := nativeModal(c.Key("search-scopes"), &open, func() {
		ui.Column(c).Width(680).Gap(10).Children(func() {
			ui.Text(c, "Scopes").FontSize(14).Bold().TextColor(p.Text).SingleLine()
			ui.Text(c, "A scope is a file pattern such as file:src//*.java&&!file:*test*.").
				FontSize(12).TextColor(p.TextMuted).SingleLine().Ellipsis("…")
			if len(s.scopesDraft) == 0 {
				ui.Row(c).Height(180).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
					ui.Text(c, "No custom scopes").FontSize(12).TextColor(p.TextMuted)
				})
			} else {
				labels := make([]string, len(s.scopesDraft))
				for index, scope := range s.scopesDraft {
					labels[index] = scope.Name
					if labels[index] == "" {
						labels[index] = fmt.Sprintf("Unnamed (%d)", index+1)
					}
				}
				if choice := ui.Select(c, &s.scopesChoice, labels).Width(240); choice.Changed() {
					for index, label := range labels {
						if label == s.scopesChoice {
							s.scopesSelected = index
							break
						}
					}
				}
				if s.scopesSelected < 0 || s.scopesSelected >= len(s.scopesDraft) {
					s.scopesSelected = 0
				}
				scope := &s.scopesDraft[s.scopesSelected]
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "Name").Width(60).FontSize(12).TextColor(p.TextMuted)
					ui.TextInput(c, &scope.Name).Grow(1)
				})
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "Pattern").Width(60).FontSize(12).TextColor(p.TextMuted)
					ui.TextInput(c, &scope.Pattern).Grow(1).
						Placeholder("file:src//*.java&&!file:*test*")
				})
			}
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Button(c, "Add").OnClick(m.addSearchScope)
				if len(s.scopesDraft) > 0 {
					ui.Button(c, "Remove").OnClick(m.removeSearchScope)
				}
				ui.Spacer(c)
				ui.Button(c, "Cancel").OnClick(func() {
					s.scopesOpen = false
					s.scopesErr = ""
					m.invalidate()
				})
				ui.Button(c, "Save").OnClick(m.saveSearchScopes)
			})
			if s.scopesErr != "" {
				ui.Text(c, s.scopesErr).FontSize(12).TextColor(p.Danger).SingleLine().Ellipsis("…")
			}
		})
	})
	if dialog.Dismissed() || !open {
		s.scopesOpen = false
		s.scopesErr = ""
		m.invalidate()
	}
}
