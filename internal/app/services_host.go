package app

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

func (m *Model) servicesHostTabs(c *ui.Context, p *ui.Theme) {
	oi := oiPaletteOf(p)
	if m.services.view == "" {
		m.services.view = serviceHostServices
	}
	ui.Row(c.Key("services:host-tabs")).Height(30).Shrink(0).Padding(2, 6).
		Gap(4).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(oi.border).Children(func() {
		ui.ScrollHorizontal(c).Grow(1).MinWidth(0).AlignItems(ui.Stretch).
			Children(func() {
				ui.Row(c).Gap(2).AlignItems(ui.Stretch).Children(func() {
					for _, view := range []struct {
						id, label, icon string
					}{
						{string(serviceHostServices), "Services", "toolwindows/services"},
						{string(serviceHostDatabase), "Database", "openidea/database/table"},
						{string(serviceHostHTTP), "HTTP Client", "fileTypes/http"},
					} {
						view := view
						id := serviceHostView(view.id)
						active := m.services.view == id
						button := ui.ButtonBase(c.Key("services:host-tab:"+view.id)).
							Height(24).Padding(0, 9).Gap(6).Radius(oiRadiusM).
							AlignItems(ui.Center).Cursor(ui.CursorPointer)
						if active {
							button.Background(oi.selected)
						} else if button.Hovered() {
							button.Background(oi.hoverTint)
						}
						button.OnClick(func() {
							m.services.view = id
							switch id {
							case serviceHostDatabase:
								m.ensureServicesDatabase()
							case serviceHostHTTP:
								m.ensureHTTPClient()
								m.loadHTTPHistory(false)
							}
							m.invalidate()
						})
						button.Children(func() {
							oiIconEl(c, view.icon, 15, oi, oi.textMuted)
							ui.Text(c, view.label).FontSize(12).
								TextColor(oi.textStrong).SingleLine()
						})
					}
				})
			})
	})
}

func (m *Model) ensureServicesDatabase() {
	if m.db == nil {
		m.db = &dbView{}
	}
	if len(m.db.sources) == 0 && !m.db.loading {
		m.RefreshDBSources()
	}
}

func (m *Model) servicesDatabasePanel(c *ui.Context, p *ui.Theme) {
	m.ensureServicesDatabase()
	v := m.db
	oi := oiPaletteOf(p)
	ui.Column(c.Key("services:database")).Grow(1).MinWidth(0).MinHeight(0).
		Children(func() {
			m.servicesDatabaseToolbar(c, p)
			if v.loading && len(v.sources) == 0 {
				ui.Column(c).Grow(1).AlignItems(ui.Center).Justify(ui.Center).
					Children(func() {
						ui.Text(c, "Loading data sources…").FontSize(12).TextColor(oi.textMuted)
					})
				return
			}
			if v.err != "" {
				ui.Row(c).Shrink(0).Padding(5, 9).Gap(8).AlignItems(ui.Center).
					Background(p.Danger.Mix(p.Surface, 0.9)).Children(func() {
					oiIconEl(c, "status/error", 15, oi, p.Danger)
					ui.Text(c, v.err).FontSize(11).TextColor(p.Danger).
						SingleLine().Ellipsis("…").Grow(1)
					ui.Button(c, "Dismiss").OnClick(func() {
						v.err = ""
						m.invalidate()
					})
				})
			}
			ui.Row(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
				ui.Column(c).Width(330).Shrink(0).MinWidth(230).MinHeight(0).
					BorderWidth(0, 0, 0, 1).BorderColor(oi.border).Children(func() {
					m.servicesDatabaseTree(c, p)
				})
				ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
					m.servicesDatabaseConsole(c, p)
				})
			})
		})
}

func (m *Model) servicesDatabaseToolbar(c *ui.Context, p *ui.Theme) {
	v := m.db
	oi := oiPaletteOf(p)
	ui.Row(c.Key("services:database-toolbar")).Height(32).Shrink(0).
		Padding(0, 8).Gap(7).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(oi.border).Children(func() {
		ui.Button(c, "+ New").OnClick(func() { m.DBNew(nil) })
		connect := ui.Button(c, "Connect")
		connect.Disabled(v.activeTarget() == "")
		connect.OnClick(func() { m.ConnectDBSource(v.activeTarget()) })
		disconnect := ui.Button(c, "Disconnect")
		disconnect.Disabled(v.activeTarget() == "" || !v.connected[v.activeTarget()])
		disconnect.OnClick(func() { m.DisconnectDBSource(v.activeTarget()) })
		ui.Button(c, "Query Console").OnClick(func() { m.DBOpenConsole(nil) })
		ui.Spacer(c)
		run := ui.Button(c, "Run")
		run.Disabled(v.running || v.activeTarget() == "")
		run.OnClick(m.RunDB)
		ui.Button(c, "History").OnClick(func() { m.OpenDBHistory() })
		ui.Button(c, "Explain").OnClick(m.ExplainDB)
		ui.Button(c, "Open Database Tool Window").OnClick(m.OpenDatabase)
	})
}

func (m *Model) servicesDatabaseTree(c *ui.Context, p *ui.Theme) {
	v := m.db
	oi := oiPaletteOf(p)
	ui.Scroll(c.Key("services:database-tree")).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Padding(4, 5).Gap(1).Children(func() {
			ui.Text(c, "Data Sources").FontSize(11).Bold().TextColor(oi.textMuted).
				Padding(3, 5)
			if len(v.sources) == 0 {
				ui.Column(c).Padding(10, 8).Gap(5).Children(func() {
					ui.Text(c, "No data sources yet").FontSize(12).TextColor(oi.textMuted)
					ui.Button(c, "New Data Source").OnClick(func() { m.DBNew(nil) })
				})
				return
			}
			for _, source := range v.sources {
				source := source
				selected := v.source == source.ID
				row := ui.Row(c.Key("services:db-source:"+source.ID)).Height(25).
					Padding(4, 6).Gap(6).Radius(5).AlignItems(ui.Center).
					Cursor(ui.CursorPointer)
				if selected {
					row.Background(oi.selected)
				} else if row.Hovered() {
					row.Background(oi.hoverTint)
				}
				if row.Clicked() {
					m.SelectDBSource(source.ID)
				}
				if row.DoubleClicked() {
					m.SelectDBSource(source.ID)
					m.ConnectDBSource(source.ID)
				}
				row.ContextMenu(func(menu *ui.Menu) {
					if menu.Item("Data Source Properties…").Chosen() {
						m.DBProperties(map[string]any{"source": source.ID})
					}
					if menu.Item("Duplicate").Chosen() {
						m.DBDuplicateSource(map[string]any{"source": source.ID})
					}
					if menu.Item("Query Console").Chosen() {
						m.DBOpenConsole(map[string]any{"source": source.ID})
					}
					connectionItem := menu.Item("Connect")
					if v.connected[source.ID] {
						connectionItem = menu.Item("Disconnect")
					}
					if connectionItem.Chosen() {
						if v.connected[source.ID] {
							m.DisconnectDBSource(source.ID)
						} else {
							m.ConnectDBSource(source.ID)
						}
					}
					menu.Separator()
					if menu.Item("Remove Data Source…").Chosen() {
						m.DBDrop(map[string]any{"source": source.ID})
					}
				})
				row.Children(func() {
					oiIconEl(c, dbDriverIcon(source.Driver), 16, oi, oi.textMuted)
					ui.Text(c, source.Name).FontSize(12).TextColor(oi.text).
						SingleLine().Ellipsis("…").Grow(1)
					if v.connected[source.ID] {
						ui.Text(c, "●").FontSize(9).TextColor(oi.success)
					} else {
						ui.Text(c, source.Driver).FontSize(10).TextColor(oi.textMuted)
					}
				})
			}
			if v.currentDatabase != "" {
				m.servicesDatabaseGroup(c, p, "Database", v.currentDatabase)
			}
			if len(v.databases) > 0 && v.currentDatabase == "" {
				for _, database := range v.databases {
					database := database
					row := m.servicesDatabaseRow(c, p, "services:db-database:"+database, database)
					if row.Clicked() {
						m.SelectDBDatabase(database)
					}
				}
			}
			if len(v.schemas) > 0 {
				for _, schema := range v.schemas {
					schema := schema
					row := m.servicesDatabaseRow(c, p, "services:db-schema:"+schema, schema)
					if row.Clicked() {
						m.loadDBTables(v.source, schema)
					}
				}
			}
			if v.schema != "" {
				ui.Text(c, "Tables and Views").FontSize(11).Bold().TextColor(oi.textMuted).
					Padding(10, 5)
				for _, table := range v.tables {
					table := table
					icon := "openidea/database/table"
					if table.Kind != "table" {
						icon = "openidea/database/matview"
					}
					row := ui.Row(c.Key("services:db-table:"+table.Name)).Height(24).
						Padding(16, 6).Gap(6).Radius(4).AlignItems(ui.Center).
						Cursor(ui.CursorPointer)
					if v.selectedTable.Name == table.Name || row.Hovered() {
						row.Background(oi.hoverTint)
					}
					if row.Clicked() {
						v.selectedTable = table
						v.sql = "SELECT * FROM " + m.dbQualifiedName(v.source, v.schema, table.Name)
						m.invalidate()
					}
					if row.DoubleClicked() {
						m.OpenDBTable(v.schema, table.Name)
					}
					row.ContextMenu(func(menu *ui.Menu) {
						target := map[string]any{
							"source": v.source, "schema": v.schema,
							"table": table.Name, "kind": table.Kind,
						}
						if menu.Item("Edit Data").Chosen() {
							m.DBEditData(target)
						}
						if menu.Item("Go to DDL").Chosen() {
							m.DBGoToDDL(target)
						}
						if menu.Item("Copy Reference").Chosen() {
							m.DBCopyReference(target)
						}
						menu.Separator()
						if menu.Item("Rename…").Chosen() {
							m.DBRename(target)
						}
						if menu.Item("Truncate…").Chosen() {
							m.DBTruncate(target)
						}
						if menu.Item("Drop…").Chosen() {
							m.DBDrop(target)
						}
					})
					row.Children(func() {
						oiIconEl(c, icon, 15, oi, oi.textMuted)
						ui.Text(c, table.Name).FontSize(12).TextColor(oi.text).
							SingleLine().Ellipsis("…")
					})
				}
			}
		})
	})
}

func (m *Model) servicesDatabaseRow(c *ui.Context, p *ui.Theme, key, label string) ui.Element {
	oi := oiPaletteOf(p)
	row := ui.Row(c.Key(key)).Height(24).Padding(12, 6).Gap(6).
		Radius(4).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if row.Hovered() {
		row.Background(oi.hoverTint)
	}
	row.Children(func() {
		oiIconEl(c, "openidea/database/schema", 14, oi, oi.textMuted)
		ui.Text(c, label).FontSize(12).TextColor(oi.text).SingleLine().Ellipsis("…")
	})
	return row
}

func (m *Model) servicesDatabaseGroup(c *ui.Context, p *ui.Theme, label, value string) {
	oi := oiPaletteOf(p)
	ui.Row(c).Height(24).Padding(10, 6).Gap(6).AlignItems(ui.Center).Children(func() {
		oiIconEl(c, "openidea/database/schema", 14, oi, oi.textMuted)
		ui.Text(c, label).FontSize(11).TextColor(oi.textMuted)
		ui.Text(c, value).FontSize(12).TextColor(oi.text).SingleLine().Ellipsis("…")
	})
}

func (m *Model) servicesDatabaseConsole(c *ui.Context, p *ui.Theme) {
	v := m.db
	oi := oiPaletteOf(p)
	ui.Column(c.Key("services:database-console")).Grow(1).MinWidth(0).MinHeight(0).
		Padding(8).Gap(7).Children(func() {
		area := ui.TextArea(c, &v.sql).Height(130).Font("monospace").FontSize(12)
		area.OnShortcut(ui.Cmd, ui.KeyEnter, func() { m.RunDB() })
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			run := ui.Button(c, "Run")
			run.Disabled(v.running || v.activeTarget() == "")
			run.OnClick(m.RunDB)
			ui.Button(c, "Explain").OnClick(m.ExplainDB)
			ui.Button(c, "History").OnClick(func() { m.OpenDBHistory() })
			ui.Button(c, "Run Script…").OnClick(func() { m.DBRunSQLScript(nil) })
			target := v.activeTarget()
			label := "Tx: Auto"
			if v.manualCommit[target] {
				label = "Tx: Manual"
			}
			ui.Button(c, label).OnClick(m.ToggleDBManualCommit)
			if v.manualCommit[target] {
				ui.Button(c, "Commit").OnClick(func() { m.FinishDBTransaction(true) })
				ui.Button(c, "Rollback").OnClick(func() { m.FinishDBTransaction(false) })
			}
			ui.Spacer(c)
			if v.running {
				ui.Text(c, "Running…").FontSize(11).TextColor(oi.textMuted)
			}
		})
		m.servicesDatabaseResultTabs(c, p)
		if v.err != "" {
			ui.Text(c, v.err).FontSize(11).TextColor(oi.danger).
				SingleLine().Ellipsis("…")
		}
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			m.servicesDatabaseResults(c, p)
		})
	})
}

func (m *Model) servicesDatabaseResultTabs(c *ui.Context, p *ui.Theme) {
	v := m.db
	oi := oiPaletteOf(p)
	_, exportable := m.dbExportDataForResult(v.active)
	ui.Row(c).Height(26).Shrink(0).Gap(2).AlignItems(ui.Center).Children(func() {
		ui.ScrollHorizontal(c).Grow(1).MinWidth(0).Children(func() {
			ui.Row(c).Gap(2).Children(func() {
				for i := -1; i < len(v.results); i++ {
					i := i
					title := "Output"
					if i >= 0 {
						title = v.results[i].title
					}
					button := ui.ButtonBase(c.Key(fmt.Sprintf("services:db-result:%d", i))).
						Height(24).Padding(0, 8).Radius(4).AlignItems(ui.Center).
						Cursor(ui.CursorPointer)
					if v.active == i {
						button.Background(oi.selected)
					} else if button.Hovered() {
						button.Background(oi.hoverTint)
					}
					button.OnClick(func() { m.selectDBResult(i) })
					button.ContextMenu(func(menu *ui.Menu) {
						if i >= 0 {
							if menu.Item("Close").Chosen() {
								m.CloseDBResult(i)
							}
							if menu.Item("Close Others").Chosen() {
								m.CloseOtherDBResults(i)
							}
							if menu.Item("Close All").Chosen() {
								m.DBCloseAllResultsAction()
							}
							if menu.Item("Rename Tab…").Chosen() {
								m.DBRenameResultAction(i)
							}
						}
					})
					button.Children(func() {
						ui.Text(c, title).FontSize(11).TextColor(oi.textStrong).
							SingleLine().Ellipsis("…")
					})
				}
			})
		})
		if exportable {
			ui.MenuButton(c, "Export", func(menu *ui.Menu) {
				index := v.active
				if menu.Item("Export Data…").Chosen() {
					m.OpenDBExport(index)
				}
				menu.Separator()
				for _, format := range dbExportFormats {
					format := format
					if menu.Item("Copy as " + format.Label).Chosen() {
						m.DBExportCopy(map[string]any{"index": index, "format": format.ID})
					}
				}
			})
		}
	})
}

func (m *Model) servicesDatabaseResults(c *ui.Context, p *ui.Theme) {
	v := m.db
	oi := oiPaletteOf(p)
	if v.active < 0 {
		ui.Column(c).Padding(3, 4).Gap(2).Children(func() {
			if len(v.outLog) == 0 {
				ui.Text(c, "The statement output appears here").FontSize(11).TextColor(oi.textMuted)
			}
			for _, line := range v.outLog {
				ui.Text(c, line).FontSize(11).TextColor(oi.textMuted).
					SingleLine().Ellipsis("…")
			}
		})
		return
	}
	ui.Column(c).Gap(8).Children(func() {
		for resultIndex, outcome := range v.outcomes {
			resultIndex, outcome := resultIndex, outcome
			ui.Column(c).Gap(4).Children(func() {
				ui.Row(c).Gap(8).Children(func() {
					ui.Text(c, oneLine(outcome.Statement.Text)).FontSize(11).
						TextColor(oi.textMuted).SingleLine().Ellipsis("…").Grow(1)
					ui.Text(c, dbOutcomeSummary(outcome)).FontSize(11).TextColor(oi.textMuted)
				})
				if outcome.Error != "" {
					ui.Text(c, outcome.Error).FontSize(11).TextColor(oi.danger)
					return
				}
				result := outcome.Result
				if result == nil || len(result.Columns) == 0 {
					return
				}
				columns := result.Columns
				if len(columns) > dbShowCols {
					columns = columns[:dbShowCols]
				}
				head := ui.Row(c.Key(fmt.Sprintf("services:db-head:%d", resultIndex))).
					Height(24).Padding(0, 5).Gap(4).AlignItems(ui.Center)
				head.Background(oi.elev)
				head.Children(func() {
					for _, column := range columns {
						ui.Text(c, column.Name).FontSize(11).Bold().TextColor(oi.text).
							Width(dbCellWidth).SingleLine().Ellipsis("…")
					}
				})
				rows := result.Rows
				if len(rows) > dbShowRows {
					rows = rows[:dbShowRows]
				}
				for rowIndex, cells := range rows {
					rowIndex, cells := rowIndex, cells
					row := ui.Row(c.Key(fmt.Sprintf("services:db-row:%d:%d", resultIndex, rowIndex))).
						Height(24).Padding(0, 5).Gap(4).AlignItems(ui.Center)
					if rowIndex%2 == 1 || row.Hovered() {
						row.Background(oi.hoverTint)
					}
					row.Children(func() {
						for columnIndex := range columns {
							if columnIndex < len(cells) {
								dbCell(c, p, cells[columnIndex])
							} else {
								dbCell(c, p, nil)
							}
						}
					})
				}
			})
		}
	})
}
