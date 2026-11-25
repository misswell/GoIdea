package app

import (
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type dbNewMenu struct {
	open bool
	err  string
}

func (m *Model) DBNew(_ any) {
	v := m.dbEnsureOpen()
	v.newMenu = &dbNewMenu{open: true}
	m.invalidate()
}

func (m *Model) closeDBNewMenu() {
	if m.db != nil {
		m.db.newMenu = nil
	}
	m.invalidate()
}

func (m *Model) dbNewMenuPopup(c *ui.Context, p *ui.Theme) {
	if m.db == nil || m.db.newMenu == nil {
		return
	}
	menu := m.db.newMenu
	open := menu.open
	nativeModal(c, &open, func() {
		ui.Column(c).Width(300).Gap(2).Children(func() {
			item := func(label, detail string, run func()) {
				row := ui.Row(c.Key("dbnew:"+label)).Height(30).Padding(8, 6).Gap(8).
					Radius(5).AlignItems(ui.Center)
				if row.Hovered() {
					row.Background(p.SurfaceHover)
				}
				row.Children(func() {
					ui.Text(c, label).FontSize(13).TextColor(p.Text).SingleLine().Grow(1)
					if detail != "" {
						ui.Text(c, detail).FontSize(11).TextColor(p.TextMuted).SingleLine()
					}
				})
				if row.Clicked() {
					run()
				}
			}
			item("Query Console", "SQL", func() {
				m.closeDBNewMenu()
				m.DBOpenConsole(nil)
			})
			ui.Row(c).Height(1).Background(p.Border)
			item("Data Source…", "", func() {
				m.closeDBNewMenu()
				m.DBNewDataSource(nil)
			})
			item("Data Source from File…", "SQLite", func() {
				m.closeDBNewMenu()
				m.AddSQLiteSource()
			})
			item("Data Source from URL…", "", func() {
				m.closeDBNewMenu()
				m.OpenDBDataSourceFormMap(map[string]any{"urlOnly": true})
			})
			item("Import from Clipboard", "", func() {
				m.importDBDataSourceFromClipboard()
			})
			if menu.err != "" {
				ui.Text(c, menu.err).FontSize(12).TextColor(p.Danger).SingleLine().
					Ellipsis("…").Padding(8, 6)
			}
		})
	})
	if !open && m.db != nil && m.db.newMenu == menu {
		m.db.newMenu = nil
		m.invalidate()
	}
}

func (m *Model) importDBDataSourceFromClipboard() {
	if m.db == nil || m.db.newMenu == nil {
		return
	}
	menu := m.db.newMenu
	text := strings.TrimSpace(mygo.Clipboard.ReadText())
	source, password, ok := dbParseSourceURL(text)
	if !ok {
		menu.err = "No data source on the clipboard"
		m.invalidate()
		return
	}
	m.db.newMenu = nil
	m.OpenDBDataSourceFormMap(source)
	if m.db != nil && m.db.sourceForm != nil {
		m.db.sourceForm.password = password
	}
	m.invalidate()
}
