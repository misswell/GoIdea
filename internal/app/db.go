package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

const (
	dbMaxRows   = 200
	dbShowRows  = 50
	dbShowCols  = 6
	dbCellWidth = 108
)

// dbView is the open database sheet: the sources, the connection and schema it
// has open, the console text and the results of its last run.
type dbView struct {
	open      bool
	sources   []openidea.DataSource
	connected map[string]bool
	source    string
	schemas   []string
	schema    string
	tables    []openidea.DBTable
	sql       string

	loading  bool
	running  bool
	err      string
	outcomes []openidea.DBOutcome
	// results are the run result tabs (v0.1.44's result tabs); active is the
	// selected one, -1 for the Output tab. outcomes mirrors the active result
	// for the smoke and older callers.
	results []dbResult
	active  int
	outLog  []string
	seq     int

	// gen drops replies that a newer request for the same list superseded.
	gen uint64
}

// dbResult is one run's result tab.
type dbResult struct {
	id       int
	title    string
	outcomes []openidea.DBOutcome
	err      string
}

// addDBResult puts a new result tab in front, caps the strip and selects it.
func (v *dbView) addDBResult(title string, outcomes []openidea.DBOutcome, errText string) {
	v.seq++
	v.results = append([]dbResult{{id: v.seq, title: title, outcomes: outcomes, err: errText}}, v.results...)
	if len(v.results) > 10 {
		v.results = v.results[:10]
	}
	v.active = 0
	v.outcomes, v.err = outcomes, errText
}

// selectDBResult activates a result tab, or the Output tab for -1.
func (m *Model) selectDBResult(i int) {
	v := m.db
	if v == nil {
		return
	}
	if i < -1 || i >= len(v.results) {
		return
	}
	v.active = i
	if i == -1 {
		v.outcomes, v.err = nil, ""
	} else {
		v.outcomes, v.err = v.results[i].outcomes, v.results[i].err
	}
	m.invalidate()
}

// CloseDBResult closes one result tab; the Output tab is not closable.
func (m *Model) CloseDBResult(i int) {
	v := m.db
	if v == nil || i < 0 || i >= len(v.results) {
		return
	}
	v.results = append(v.results[:i], v.results[i+1:]...)
	if v.active >= len(v.results) {
		v.active = len(v.results) - 1
	}
	if v.active < 0 {
		v.active = -1
	}
	m.selectDBResult(v.active)
}

// CloseOtherDBResults keeps one result tab.
func (m *Model) CloseOtherDBResults(i int) {
	v := m.db
	if v == nil || i < 0 || i >= len(v.results) {
		return
	}
	kept := v.results[i]
	v.results = []dbResult{kept}
	m.selectDBResult(0)
}

// StepDBResult cycles the Output and result tabs.
func (m *Model) StepDBResult(delta int) {
	v := m.db
	if v == nil || len(v.results) == 0 {
		return
	}
	ids := len(v.results) + 1
	cur := v.active + 1 // 0 = Output
	next := ((cur+delta)%ids + ids) % ids
	m.selectDBResult(next - 1)
}

// OpenDatabase opens the database sheet and lists the data sources.
func (m *Model) OpenDatabase() {
	if m.db == nil {
		m.db = &dbView{open: true}
	}
	m.db.open = true
	m.RefreshDBSources()
	m.invalidate()
}

func (m *Model) closeDatabase() {
	if m.db == nil || !m.db.open {
		return
	}
	m.db.open = false
	m.invalidate()
}

// RefreshDBSources rereads the project's data sources and which are connected.
func (m *Model) RefreshDBSources() {
	v := m.db
	if v == nil {
		return
	}
	v.loading = true
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		sources, err := m.mgr.DBSources(ctx, Window)
		connected, _ := m.mgr.DBConnected(ctx, Window)
		m.post(func() {
			if m.db != v {
				return
			}
			v.loading = false
			if err != nil {
				v.err = "读取数据源失败：" + err.Error()
				m.invalidate()
				return
			}
			v.sources = sources
			v.connected = make(map[string]bool, len(connected))
			for _, id := range connected {
				v.connected[id] = true
			}
			if v.source == "" && len(sources) > 0 {
				v.source = sources[0].ID
			}
			m.invalidate()
		})
	}()
}

// SelectDBSource makes id the sheet's data source and loads its schemas when
// it is connected.
func (m *Model) SelectDBSource(id string) {
	v := m.db
	if v == nil || id == "" {
		return
	}
	if v.source == id {
		return
	}
	v.source = id
	v.schemas = nil
	v.schema = ""
	v.tables = nil
	v.outcomes = nil
	v.err = ""
	m.loadDBSchemas(id)
	m.invalidate()
}

// ConnectDBSource opens the connection of id, then loads its schemas.
func (m *Model) ConnectDBSource(id string) {
	v := m.db
	if v == nil || id == "" || v.connected[id] {
		return
	}
	v.err = ""
	v.loading = true
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := m.mgr.DBConnect(ctx, Window, id)
		m.post(func() {
			if m.db != v {
				return
			}
			v.loading = false
			if err != nil {
				v.err = "连接失败：" + err.Error()
				m.invalidate()
				return
			}
			if v.connected == nil {
				v.connected = map[string]bool{}
			}
			v.connected[id] = true
			m.loadDBSchemas(id)
			m.invalidate()
		})
	}()
}

// DisconnectDBSource closes the connection of id.
func (m *Model) DisconnectDBSource(id string) {
	v := m.db
	if v == nil || id == "" || !v.connected[id] {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		err := m.mgr.DBDisconnect(ctx, Window, id)
		m.post(func() {
			if m.db != v {
				return
			}
			if err != nil {
				v.err = "断开失败：" + err.Error()
			} else {
				delete(v.connected, id)
				v.schemas = nil
				v.schema = ""
				v.tables = nil
			}
			m.invalidate()
		})
	}()
}

// AddSQLiteSource asks for a SQLite file and saves it as a data source.
func (m *Model) AddSQLiteSource() {
	v := m.db
	if v == nil {
		return
	}
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Title: "选择 SQLite 数据库文件",
			Filters: []mygo.FileFilter{
				{Name: "SQLite 数据库", Extensions: []string{"db", "sqlite", "sqlite3"}},
			},
		})
		if err != nil || len(paths) == 0 {
			return
		}
		path := paths[0]
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		id := "sqlite:" + path
		err = m.mgr.DBSaveSQLiteSource(ctx, Window, id, filepath.Base(path), path)
		m.post(func() {
			if m.db != v {
				return
			}
			if err != nil {
				v.err = "添加数据源失败：" + err.Error()
			}
			m.RefreshDBSources()
			m.invalidate()
		})
	}()
}

func (m *Model) loadDBSchemas(id string) {
	v := m.db
	if v == nil || id == "" {
		return
	}
	v.gen++
	gen := v.gen
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		schemas, err := m.mgr.DBSchemas(ctx, Window, id)
		m.post(func() {
			if m.db != v || v.gen != gen || v.source != id {
				return
			}
			if err != nil {
				v.err = "读取架构失败：" + err.Error()
				m.invalidate()
				return
			}
			v.schemas = schemas
			if len(schemas) > 0 {
				schema := schemas[0]
				for _, s := range schemas {
					if s == "main" || s == "public" {
						schema = s
						break
					}
				}
				m.loadDBTables(id, schema)
			}
			m.invalidate()
		})
	}()
}

func (m *Model) loadDBTables(id, schema string) {
	v := m.db
	if v == nil || id == "" || schema == "" || v.source != id {
		return
	}
	v.schema = schema
	v.tables = nil
	v.gen++
	gen := v.gen
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		tables, err := m.mgr.DBTables(ctx, Window, id, schema)
		m.post(func() {
			if m.db != v || v.gen != gen || v.source != id || v.schema != schema {
				return
			}
			if err != nil {
				v.err = "读取表失败：" + err.Error()
				m.invalidate()
				return
			}
			v.tables = tables
			m.invalidate()
		})
	}()
}

// RunDB runs the console's scripts on the connected data source and shows the
// outcomes of each statement.
func (m *Model) RunDB() {
	v := m.db
	if v == nil || v.running {
		return
	}
	if v.source == "" {
		v.err = "请先选择数据源"
		m.invalidate()
		return
	}
	if !v.connected[v.source] {
		v.err = "请先连接数据源"
		m.invalidate()
		return
	}
	sql := strings.TrimSpace(v.sql)
	if sql == "" {
		v.err = "请输入 SQL"
		m.invalidate()
		return
	}
	v.running = true
	v.err = ""
	v.outcomes = nil
	v.active = 0
	v.outLog = append(v.outLog, "运行："+oneLine(sql))
	if len(v.outLog) > 200 {
		v.outLog = v.outLog[len(v.outLog)-200:]
	}
	request := m.chanSeq.Add(1)
	id, schema := v.source, v.schema
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		statements, err := m.mgr.DBStatements(ctx, Window, id, sql, 0, utf16Len(sql))
		var outcomes []openidea.DBOutcome
		if err == nil && len(statements) > 0 {
			outcomes, err = m.mgr.DBExecute(ctx, Window, request, id, statements, dbMaxRows, schema)
		}
		m.post(func() {
			if m.db != v {
				return
			}
			v.running = false
			title := oneLine(sql)
			if i := strings.IndexAny(title, " ;"); i > 0 {
				title = title[:i]
			}
			switch {
			case err != nil:
				v.addDBResult(title, nil, "执行失败："+err.Error())
				v.outLog = append(v.outLog, "执行失败："+err.Error())
			case len(statements) == 0:
				v.addDBResult(title, outcomes, "没有可执行的语句")
				v.outLog = append(v.outLog, "没有可执行的语句")
			default:
				v.addDBResult(title, outcomes, "")
				for _, o := range outcomes {
					v.outLog = append(v.outLog, oneLine(o.Statement.Text)+" · "+dbOutcomeSummary(o))
				}
			}
			if len(v.outLog) > 200 {
				v.outLog = v.outLog[len(v.outLog)-200:]
			}
			m.loadDBTables(id, schema)
			m.invalidate()
		})
	}()
}

// utf16Len is the length of s in UTF-16 code units, the offsets the backend
// uses on its IPC boundary.
func utf16Len(s string) uint32 {
	var n uint32
	for _, r := range s {
		if r > 0xFFFF {
			n += 2
		} else {
			n++
		}
	}
	return n
}

// dbStatusText is the sheet's one-line state.
func dbStatusText(v *dbView) string {
	switch {
	case v.loading:
		return "正在连接…"
	case v.running:
		return "正在执行…"
	case v.source == "":
		return "没有数据源"
	case v.connected[v.source]:
		return "已连接"
	}
	return "未连接"
}

// dbOutcomeSummary describes a statement's result in one line.
func dbOutcomeSummary(o openidea.DBOutcome) string {
	if o.Result == nil {
		return ""
	}
	r := o.Result
	if r.Affected != nil {
		return fmt.Sprintf("影响 %d 行 · %.1f ms", int(*r.Affected), r.ElapsedMs)
	}
	s := fmt.Sprintf("%d 行 · %.1f ms", len(r.Rows), r.ElapsedMs)
	if r.HasMore {
		s += " · 还有更多"
	}
	return s
}

// dbCell renders one cell; NULL is the backend's nil.
func dbCell(c *ui.Context, p *ui.Theme, cell *string) {
	text, color := "NULL", p.TextMuted
	if cell != nil {
		text, color = *cell, p.Text
	}
	ui.Text(c, text).FontSize(11).TextColor(color).Width(dbCellWidth).
		SingleLine().Ellipsis("…")
}

// dbDialog draws the database sheet.
func (m *Model) dbDialog(c *ui.Context, p *ui.Theme) {
	v := m.db
	if v == nil || !v.open {
		return
	}
	open := v.open
	dlg := ui.Modal(c, &open, func() {
		ui.Column(c).Width(980).Height(640).Gap(10).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "数据库").FontSize(14).Bold().TextColor(p.Text).SingleLine()
				ui.Spacer(c)
				ui.Text(c, dbStatusText(v)).FontSize(10.5).TextColor(p.TextMuted).SingleLine()
			})
			ui.Row(c).Grow(1).MinHeight(0).Gap(14).Children(func() {
				ui.Column(c).Width(250).Gap(6).Children(func() {
					ui.Text(c, "数据源").FontSize(11).TextColor(p.TextMuted).SingleLine()
					if len(v.sources) == 0 {
						ui.Text(c, "还没有数据源").FontSize(11).TextColor(p.TextMuted).SingleLine()
					}
					ui.Scroll(c).Height(140).Children(func() {
						ui.Column(c).Children(func() {
							for _, s := range v.sources {
								s := s
								sel := v.source == s.ID
								r := ui.Row(c.Key("dbsrc:"+s.ID)).Height(24).Padding(0, 6).
									Gap(6).AlignItems(ui.Center)
								if sel || r.Hovered() {
									r.Background(p.SurfaceHover)
								}
								r.Children(func() {
									mark := "○"
									if v.connected[s.ID] {
										mark = "●"
									}
									ui.Text(c, mark).FontSize(11).TextColor(p.Accent).Width(12)
									ui.Text(c, s.Name).FontSize(12).TextColor(p.Text).
										SingleLine().Ellipsis("…")
									ui.Spacer(c)
									ui.Text(c, s.Driver).FontSize(10).TextColor(p.TextMuted).SingleLine()
								})
								if r.Clicked() {
									m.SelectDBSource(s.ID)
								}
								if r.DoubleClicked() {
									m.SelectDBSource(s.ID)
									m.ConnectDBSource(s.ID)
								}
							}
						})
					})
					ui.Row(c).Gap(6).Children(func() {
						ui.Button(c, "连接").OnClick(func() { m.ConnectDBSource(v.source) })
						ui.Button(c, "断开").OnClick(func() { m.DisconnectDBSource(v.source) })
						ui.Button(c, "添加 SQLite…").OnClick(func() { m.AddSQLiteSource() })
					})
					if len(v.schemas) > 0 {
						ui.Text(c, "架构").FontSize(11).TextColor(p.TextMuted).SingleLine()
						ui.Scroll(c).Height(88).Children(func() {
							ui.Column(c).Children(func() {
								for _, schema := range v.schemas {
									schema := schema
									r := ui.Row(c.Key("dbschema:"+schema)).Height(22).Padding(0, 6)
									if v.schema == schema || r.Hovered() {
										r.Background(p.SurfaceHover)
									}
									r.Children(func() {
										ui.Text(c, schema).FontSize(12).TextColor(p.Text).
											SingleLine().Ellipsis("…")
									})
									if r.Clicked() {
										m.loadDBTables(v.source, schema)
									}
								}
							})
						})
					}
					ui.Text(c, "表").FontSize(11).TextColor(p.TextMuted).SingleLine()
					ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
						ui.Column(c).Children(func() {
							for _, t := range v.tables {
								t := t
								icon := "▤"
								if t.Kind != "table" {
									icon = "◇"
								}
								r := ui.Row(c.Key("dbtable:"+t.Name)).Height(22).Padding(0, 6).
									Gap(6).AlignItems(ui.Center)
								if r.Hovered() {
									r.Background(p.SurfaceHover)
								}
								r.Children(func() {
									ui.Text(c, icon).FontSize(10).TextColor(p.TextMuted).Width(10)
									ui.Text(c, t.Name).FontSize(12).TextColor(p.Text).
										SingleLine().Ellipsis("…")
								})
								if r.Clicked() || r.DoubleClicked() {
									table := t.Name
									if v.schema != "" && v.schema != "main" && v.schema != "public" {
										table = v.schema + "." + t.Name
									}
									v.sql = "SELECT * FROM " + table
									m.invalidate()
								}
							}
						})
					})
				})
				ui.Column(c).Grow(1).MinWidth(0).Gap(8).Children(func() {
					area := ui.TextArea(c, &v.sql).Height(120).Font("monospace").FontSize(12)
					area.OnShortcut(ui.Cmd, ui.KeyEnter, func() { m.RunDB() })
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Button(c, "运行").Disabled(v.running).OnClick(func() { m.RunDB() })
						ui.Text(c, "Cmd+Enter 运行，结果最多显示前 50 行").FontSize(10.5).
							TextColor(p.TextMuted).SingleLine()
					})
					ui.Row(c).Gap(2).Children(func() {
						for i := -1; i < len(v.results); i++ {
							i := i
							title := "Output"
							if i >= 0 {
								title = v.results[i].title
							}
							on := v.active == i
							b := ui.ButtonBase(c.Key(fmt.Sprintf("dbresult:%d", i))).Height(22).
								Padding(0, 8).Radius(4).AlignItems(ui.Center).Cursor(ui.CursorPointer)
							if on {
								b.Background(p.SurfaceHover)
								b.BorderWidth(0, 0, 1, 0).BorderColor(p.Accent)
							} else if b.Hovered() {
								b.Background(p.SurfaceHover)
							}
							b.Children(func() {
								ui.Text(c, title).FontSize(11).TextColor(p.TextMuted).
									SingleLine().Ellipsis("…")
							})
							b.OnClick(func() { m.selectDBResult(i) })
							if i >= 0 {
								b.ContextMenu(func(menu *ui.Menu) {
									if menu.Item("Close").Chosen() {
										m.CloseDBResult(i)
									}
									if menu.Item("Close Others").Chosen() {
										m.CloseOtherDBResults(i)
									}
									menu.Separator()
									if menu.Item("Next Result").Chosen() {
										m.StepDBResult(1)
									}
									if menu.Item("Previous Result").Chosen() {
										m.StepDBResult(-1)
									}
								})
							}
						}
					})
					if v.err != "" {
						ui.Text(c, v.err).FontSize(11).TextColor(p.Danger).
							SingleLine().Ellipsis("…")
					}
					ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
						if v.active < 0 {
							ui.Column(c).Padding(4, 6).Gap(2).Children(func() {
								for _, line := range v.outLog {
									ui.Text(c, line).FontSize(10.5).TextColor(p.TextMuted).
										SingleLine().Ellipsis("…")
								}
							})
							return
						}
						ui.Column(c).Gap(8).Children(func() {
							for i, o := range v.outcomes {
								i, o := i, o
								ui.Column(c).Gap(4).Children(func() {
									ui.Row(c).Gap(8).Children(func() {
										ui.Text(c, oneLine(o.Statement.Text)).FontSize(11).
											TextColor(p.TextMuted).SingleLine().Ellipsis("…").Grow(1)
										ui.Text(c, dbOutcomeSummary(o)).FontSize(10.5).
											TextColor(p.TextMuted).SingleLine()
									})
									if o.Error != "" {
										ui.Text(c, o.Error).FontSize(11).TextColor(p.Danger).SingleLine()
										return
									}
									r := o.Result
									if r == nil || len(r.Columns) == 0 {
										return
									}
									cols := r.Columns
									if len(cols) > dbShowCols {
										cols = cols[:dbShowCols]
									}
									row := ui.Row(c.Key(fmt.Sprintf("dbhead:%d", i))).Height(24).
										Padding(0, 6).Gap(4).AlignItems(ui.Center)
									row.Background(p.Surface)
									row.Children(func() {
										for _, col := range cols {
											ui.Text(c, col.Name).FontSize(11).Bold().TextColor(p.Text).
												Width(dbCellWidth).SingleLine().Ellipsis("…")
										}
									})
									rows := r.Rows
									if len(rows) > dbShowRows {
										rows = rows[:dbShowRows]
									}
									for ri, cells := range rows {
										ri, cells := ri, cells
										rr := ui.Row(c.Key(fmt.Sprintf("dbrow:%d:%d", i, ri))).
											Height(22).Padding(0, 6).Gap(4).AlignItems(ui.Center)
										if ri%2 == 1 || rr.Hovered() {
											rr.Background(p.SurfaceHover)
										}
										rr.Children(func() {
											for ci := range cols {
												if ci < len(cells) {
													dbCell(c, p, cells[ci])
												} else {
													dbCell(c, p, nil)
												}
											}
										})
									}
								})
							}
						})
					})
				})
			})
		})
	})
	if dlg.Dismissed() || !open {
		m.closeDatabase()
	}
}

// oneLine collapses a statement's whitespace for a one-line caption.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// SmokeDatabase drives the database sheet offscreen against a throwaway SQLite
// file: it saves the source, connects, lists schemas and tables, runs DDL, DML
// and a query, then puts the project's sources back as they were.
func (m *Model) SmokeDatabase(ctx context.Context) (map[string]any, error) {
	// Connecting and running SQL is execution: the project must be trusted
	// (the same grant the 信任此项目 menu item records).
	root := m.Root()
	if _, err := m.mgr.Invoke(ctx, Window, "project_trust_set", map[string]any{
		"path": root, "trusted": true, "folder": true,
	}); err != nil {
		return nil, err
	}
	original, err := m.mgr.DBSourcesRaw(ctx, Window)
	if err != nil {
		return nil, err
	}
	// Restoring an empty list still rewrites the backend's sources file, so a
	// project that had none gets its file back to "absent" below.
	sourcesFile := filepath.Join(root, ".openidea", "data-sources.toml")
	_, statErr := os.Stat(sourcesFile)
	hadSources := statErr == nil
	out := map[string]any{}
	id := "goidea-smoke-sqlite"
	dbPath := filepath.Join(os.TempDir(), fmt.Sprintf("goidea-db-smoke-%d.db", os.Getpid()))
	_ = os.Remove(dbPath)
	// SQLite treats a zero-length file as an empty database, which is what
	// connect expects to find (it never creates one).
	if err := os.WriteFile(dbPath, nil, 0o644); err != nil {
		return nil, err
	}
	defer func() {
		_ = os.Remove(dbPath)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = m.mgr.DBDisconnect(ctx, Window, id)
		_ = m.mgr.DBSaveSourcesRaw(ctx, Window, original)
		if !hadSources {
			_ = os.Remove(sourcesFile)
		}
	}()
	if err := m.mgr.DBSaveSQLiteSource(ctx, Window, id, "smoke.db", dbPath); err != nil {
		return nil, err
	}
	mygo.RunOnMain(func() { m.OpenDatabase() })
	wait := func(what string, fn func() bool) bool {
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			var ok bool
			mygo.RunOnMain(func() { ok = fn() })
			if ok {
				return true
			}
			if ctx.Err() != nil {
				return false
			}
			time.Sleep(150 * time.Millisecond)
		}
		out["timedOut"] = what
		return false
	}
	if !wait("sources", func() bool {
		if m.db == nil {
			return false
		}
		for _, s := range m.db.sources {
			if s.ID == id {
				return true
			}
		}
		return false
	}) {
		return out, nil
	}
	mygo.RunOnMain(func() { m.SelectDBSource(id); m.ConnectDBSource(id) })
	if !wait("connect", func() bool { return m.db != nil && m.db.connected[id] }) {
		return out, nil
	}
	out["connected"] = true
	if !wait("schemas", func() bool { return m.db != nil && len(m.db.schemas) > 0 }) {
		return out, nil
	}
	mygo.RunOnMain(func() {
		out["schemas"] = len(m.db.schemas)
		out["schema"] = m.db.schema
	})

	script := "CREATE TABLE people (id INTEGER PRIMARY KEY, name TEXT);\n" +
		"INSERT INTO people (name) VALUES ('Ada'), ('Bob');\n" +
		"SELECT id, name FROM people ORDER BY id;"
	mygo.RunOnMain(func() {
		m.db.sql = script
		m.RunDB()
	})
	if !wait("execute", func() bool { return m.db != nil && !m.db.running && (m.db.outcomes != nil || m.db.err != "") }) {
		return out, nil
	}
	mygo.RunOnMain(func() {
		v := m.db
		out["statements"] = len(v.outcomes)
		if v.err != "" {
			out["error"] = v.err
		}
		if n := len(v.outcomes); n > 0 {
			if r := v.outcomes[n-1].Result; r != nil {
				out["columns"] = len(r.Columns)
				out["rows"] = len(r.Rows)
				if len(r.Rows) > 0 {
					sample := make([]string, 0, len(r.Rows[0]))
					for _, cell := range r.Rows[0] {
						if cell == nil {
							sample = append(sample, "NULL")
						} else {
							sample = append(sample, *cell)
						}
					}
					out["firstRow"] = strings.Join(sample, ",")
				}
			}
			if e := v.outcomes[0].Error; e != "" {
				out["firstError"] = e
			}
		}
	})
	wait("tables", func() bool {
		for _, t := range m.db.tables {
			if t.Name == "people" {
				return true
			}
		}
		return false
	})
	mygo.RunOnMain(func() {
		v := m.db
		out["tables"] = len(v.tables)
		for _, t := range v.tables {
			if t.Name == "people" {
				out["people"] = true
			}
		}
		m.closeDatabase()
		out["closed"] = !v.open
	})
	return out, nil
}

// dbActiveResult is the selected result tab index, -1 for Output or none.
func (m *Model) dbActiveResult() int {
	if m.db == nil {
		return -1
	}
	return m.db.active
}
