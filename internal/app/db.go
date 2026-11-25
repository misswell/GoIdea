package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
	open            bool
	sources         []openidea.DataSource
	connected       map[string]bool
	source          string
	databaseTarget  string
	currentDatabase string
	databases       []string
	// selectedTable is the table or view the toolbar and context actions act
	// on when no explicit argument is supplied.
	selectedTable  openidea.DBTable
	schemas        []string
	schema         string
	tables         []openidea.DBTable
	objects        map[string]openidea.DBSchemaObjects
	sql            string
	completion     openidea.DBCompletion
	consoleSources map[string]string
	// manualCommit and pendingTx mirror the backend's transaction mode for the
	// selected connection.
	manualCommit       map[string]bool
	pendingTx          map[string]bool
	separateProcedures bool
	collapsed          bool
	renamingResult     int
	actionPrompt       *dbActionPrompt
	sourceForm         *dbSourceForm
	tableForm          *dbTableForm
	importForm         *dbImportForm
	exportDialog       *dbExportDialog
	connectRemedy      *dbConnectRemedy
	newMenu            *dbNewMenu

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

// dbActionPrompt is a small native prompt used by database actions that need
// one value or a destructive confirmation.
type dbActionPrompt struct {
	kind        string
	title       string
	value       string
	source      string
	schema      string
	table       string
	database    string
	column      string
	objectKind  string
	objectName  string
	arguments   string
	sql         string
	err         string
	busy        bool
	resultIndex int
	ifExists    bool
	cascade     bool
	restartID   bool
}

// dbResult is one run's result tab.
type dbResult struct {
	id       int
	title    string
	source   string
	outcomes []openidea.DBOutcome
	err      string
}

// addDBResult puts a new result tab in front, caps the strip and selects it.
func (v *dbView) addDBResult(title, source string, outcomes []openidea.DBOutcome, errText string) {
	v.seq++
	v.results = append([]dbResult{{
		id: v.seq, title: title, source: source, outcomes: outcomes, err: errText,
	}}, v.results...)
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
	m.dbTable = nil
	m.dbHist = nil
	m.dbPlan = nil
	m.db.exportDialog = nil
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
				v.err = "Could not read the data sources: " + err.Error()
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
	v.databaseTarget = id
	v.currentDatabase = ""
	v.databases = nil
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
		manual := false
		var completion openidea.DBCompletion
		var failedSource map[string]any
		if err == nil {
			loadedCompletion, completionErr := m.mgr.DBCompletion(ctx, Window, id)
			if completionErr == nil {
				completion = loadedCompletion
			}
			if sources, sourcesErr := m.mgr.DBSourcesRaw(ctx, Window); sourcesErr == nil {
				for _, source := range sources {
					if fmt.Sprint(source["id"]) != dbBaseTarget(id) {
						continue
					}
					if options, ok := source["options"].(map[string]any); ok &&
						fmt.Sprint(options["transaction"]) == "manual" {
						manual = true
					}
					break
				}
			}
			if manual {
				_ = m.mgr.DBSetManualCommit(ctx, Window, id, true)
			}
		} else {
			failedSource = m.dbSourceRawForID(ctx, id)
		}
		m.post(func() {
			if m.db != v {
				return
			}
			v.loading = false
			if err != nil {
				v.err = "Connect failed: " + err.Error()
				m.openDBConnectRemedy(id, err, failedSource)
				m.invalidate()
				return
			}
			if v.connected == nil {
				v.connected = map[string]bool{}
			}
			v.connected[id] = true
			if manual {
				v.manualCommit[id] = true
			}
			if completion.DefaultSchema != "" || len(completion.Tables) > 0 {
				v.completion = completion
			}
			m.loadDBSchemas(id)
			if !strings.Contains(id, ":") {
				m.loadDBDatabases(id)
			}
			m.invalidate()
		})
	}()
}

func dbBaseTarget(id string) string {
	if i := strings.IndexByte(id, ':'); i >= 0 {
		return id[:i]
	}
	return id
}

func (m *Model) loadDBDatabases(id string) {
	v := m.db
	if v == nil || id == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		databases, err := m.mgr.DBDatabases(ctx, Window, id)
		m.post(func() {
			if m.db != v || v.source != id {
				return
			}
			if err == nil {
				v.databases = databases.Names
				v.currentDatabase = databases.Current
				if v.databaseTarget == "" {
					v.databaseTarget = id
				}
			}
			m.invalidate()
		})
	}()
}

// SelectDBDatabase opens id:database and lists that database's schemas.
func (m *Model) SelectDBDatabase(database string) {
	v := m.db
	if v == nil || v.source == "" || strings.TrimSpace(database) == "" {
		return
	}
	target := v.source + ":" + database
	v.databaseTarget = target
	v.currentDatabase = database
	v.schemas = nil
	v.schema = ""
	v.tables = nil
	v.selectedTable = openidea.DBTable{}
	if v.connected[target] {
		m.loadDBSchemas(target)
		return
	}
	m.ConnectDBSource(target)
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
				v.err = "Disconnect failed: " + err.Error()
			} else {
				delete(v.connected, id)
				v.schemas = nil
				v.schema = ""
				v.tables = nil
				v.selectedTable = openidea.DBTable{}
			}
			m.invalidate()
		})
	}()
}

// sqliteSourceID names a SQLite data source after its file. A connection
// target is <id>[:<database>], so the id itself must not contain a colon.
func sqliteSourceID(path string) string {
	sum := sha256.Sum256([]byte(path))
	return "sqlite-" + hex.EncodeToString(sum[:6])
}

// AddSQLiteSource asks for a SQLite file and saves it as a data source.
func (m *Model) AddSQLiteSource() {
	v := m.db
	if v == nil {
		return
	}
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Title: "Choose a SQLite Database File",
			Filters: []mygo.FileFilter{
				{Name: "SQLite Databases", Extensions: []string{"db", "sqlite", "sqlite3"}},
			},
		})
		if err != nil || len(paths) == 0 {
			return
		}
		path := paths[0]
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		id := sqliteSourceID(path)
		err = m.mgr.DBSaveSQLiteSource(ctx, Window, id, filepath.Base(path), path)
		m.post(func() {
			if m.db != v {
				return
			}
			if err != nil {
				v.err = "Could not add the data source: " + err.Error()
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
				v.err = "Could not read the schemas: " + err.Error()
				m.invalidate()
				return
			}
			v.schemas = schemas
			if len(schemas) > 0 {
				schema := v.completion.DefaultSchema
				if schema == "" {
					schema = schemas[0]
				}
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
		objects, _ := m.mgr.DBSchemaObjects(ctx, Window, id, schema)
		m.post(func() {
			if m.db != v || v.gen != gen || v.source != id || v.schema != schema {
				return
			}
			if err != nil {
				v.err = "Could not read the tables: " + err.Error()
				m.invalidate()
				return
			}
			v.tables = tables
			if v.objects == nil {
				v.objects = map[string]openidea.DBSchemaObjects{}
			}
			key := id + "\x00" + schema
			v.objects[key] = objects
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
		v.err = "Choose a data source first"
		m.invalidate()
		return
	}
	target := v.databaseTarget
	if target == "" {
		target = v.source
	}
	if !v.connected[target] && !v.connected[v.source] {
		v.err = "Connect the data source first"
		m.invalidate()
		return
	}
	sql := strings.TrimSpace(v.sql)
	if sql == "" {
		v.err = "Enter SQL"
		m.invalidate()
		return
	}
	v.running = true
	v.err = ""
	v.outcomes = nil
	v.active = 0
	v.outLog = append(v.outLog, "Run: "+oneLine(sql))
	if len(v.outLog) > 200 {
		v.outLog = v.outLog[len(v.outLog)-200:]
	}
	request := m.chanSeq.Add(1)
	id, schema := target, v.schema
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
				v.addDBResult(title, id, nil, "Execution failed: "+err.Error())
				v.outLog = append(v.outLog, "Execution failed: "+err.Error())
			case len(statements) == 0:
				v.addDBResult(title, id, outcomes, "There is no statement to execute")
				v.outLog = append(v.outLog, "There is no statement to execute")
			default:
				v.addDBResult(title, id, outcomes, "")
				for _, o := range outcomes {
					v.outLog = append(v.outLog, oneLine(o.Statement.Text)+" · "+dbOutcomeSummary(o))
				}
				if v.manualCommit[id] {
					for _, outcome := range outcomes {
						if outcome.Error == "" && !dbReadOnlyStatement(outcome.Statement.Text) {
							v.pendingTx[id] = true
							break
						}
					}
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

func dbReadOnlyStatement(text string) bool {
	text = strings.TrimSpace(strings.ToLower(text))
	for _, prefix := range []string{"select", "show", "pragma", "explain", "describe", "desc", "values", "table", "with"} {
		if strings.HasPrefix(text, prefix+" ") || text == prefix {
			return true
		}
	}
	return false
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
		return "Connecting…"
	case v.running:
		return "Running…"
	case v.source == "":
		return "No data source"
	case v.connected[v.source]:
		return "Connected"
	}
	return "Not connected"
}

// dbOutcomeSummary describes a statement's result in one line.
func dbOutcomeSummary(o openidea.DBOutcome) string {
	if o.Result == nil {
		return ""
	}
	r := o.Result
	if r.Affected != nil {
		return fmt.Sprintf("%s affected · %.1f ms", count(int(*r.Affected), "row", "rows"), r.ElapsedMs)
	}
	s := fmt.Sprintf("%s · %.1f ms", count(len(r.Rows), "row", "rows"), r.ElapsedMs)
	if r.HasMore {
		s += " · more rows"
	}
	return s
}

// dbDriverIcon is the data source icon of its driver, the database glyph when
// the build has no drawing for it.
func dbDriverIcon(driver string) string {
	switch strings.ToLower(driver) {
	case "sqlite", "mysql", "mariadb", "postgres", "postgresql", "oracle", "sqlserver", "clickhouse",
		"cockroach", "redshift", "snowflake", "tidb", "timescale", "yugabyte", "supabase", "singlestore",
		"greenplum", "oceanbase", "neon", "aurora":
		name := strings.ToLower(driver)
		if name == "postgresql" {
			name = "postgres"
		}
		return "openidea/database/" + name
	}
	return "openidea/toolwindows/database"
}

// dbCell renders one cell; NULL is the backend's nil.
func dbCell(c *ui.Context, p *ui.Theme, cell *string) {
	text, color := "NULL", p.TextMuted
	if cell != nil {
		text, color = *cell, p.Text
	}
	ui.Text(c, text).FontSize(12).Font("monospace").TextColor(color).Width(dbCellWidth).
		SingleLine().Ellipsis("…")
}

// dbDialog draws the database sheet.
func (m *Model) dbDialog(c *ui.Context, p *ui.Theme) {
	v := m.db
	if v == nil || !v.open {
		return
	}
	open := v.open
	dlg := nativeModal(c, &open, func() {
		ui.Column(c).Width(980).Height(640).Gap(10).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Database").FontSize(13).Bold().TextColor(p.Text).SingleLine()
				ui.Spacer(c)
				ui.Text(c, dbStatusText(v)).FontSize(12).TextColor(p.TextMuted).SingleLine()
			})
			ui.Row(c).Grow(1).MinHeight(0).Gap(14).AlignItems(ui.Stretch).Children(func() {
				ui.Column(c).Width(250).Gap(6).Children(func() {
					ui.Text(c, "Data Sources").FontSize(12).TextColor(p.TextMuted).SingleLine()
					if len(v.sources) == 0 {
						ui.Text(c, "No data sources yet").FontSize(12).TextColor(p.TextMuted).SingleLine()
					}
					ui.Scroll(c).Height(140).Children(func() {
						ui.Column(c).Padding(0, 6).Children(func() {
							for _, s := range v.sources {
								s := s
								sel := v.source == s.ID
								r := ui.Row(c.Key("dbsrc:" + s.ID)).Height(24).PaddingStart(4).
									PaddingEnd(8).Gap(4).Radius(5).AlignItems(ui.Center)
								if sel || r.Hovered() {
									r.Background(p.SurfaceHover)
								}
								r.Children(func() {
									oiIconEl(c, dbDriverIcon(s.Driver), 16, oiPaletteOf(p), p.TextMuted).
										MarginEnd(2)
									name := oiPaletteOf(p).text
									if v.connected[s.ID] {
										name = oiPaletteOf(p).textStrong
									}
									ui.Text(c, s.Name).FontSize(13).TextColor(name).
										SingleLine().Ellipsis("…")
									ui.Spacer(c)
									ui.Text(c, s.Driver).FontSize(12).TextColor(p.TextMuted).SingleLine()
								})
								if r.Clicked() {
									m.SelectDBSource(s.ID)
								}
								if r.DoubleClicked() {
									m.SelectDBSource(s.ID)
									m.ConnectDBSource(s.ID)
								}
								r.ContextMenu(func(menu *ui.Menu) {
									if menu.Item("Data Source Properties…").Chosen() {
										m.DBProperties(map[string]any{"source": s.ID})
									}
									if menu.Item("Duplicate").Chosen() {
										m.DBDuplicateSource(map[string]any{"source": s.ID})
									}
									if menu.Item("Query Console").Chosen() {
										m.DBOpenConsole(map[string]any{"source": s.ID})
									}
									menu.Separator()
									if menu.Item("Remove Data Source…").Chosen() {
										m.DBDrop(map[string]any{"source": s.ID})
									}
								})
							}
						})
					})
					ui.Row(c).Gap(6).Children(func() {
						ui.Button(c, "Connect").OnClick(func() { m.ConnectDBSource(v.activeTarget()) })
						ui.Button(c, "Disconnect").OnClick(func() { m.DisconnectDBSource(v.activeTarget()) })
						ui.Button(c, "+").OnClick(func() { m.DBNew(nil) })
						ui.Button(c, "Properties").OnClick(func() { m.DBProperties(nil) })
						ui.Button(c, "Console").OnClick(func() { m.DBOpenConsole(nil) })
					})
					if len(v.databases) > 0 {
						ui.Text(c, "Databases").FontSize(12).TextColor(p.TextMuted).SingleLine()
						ui.Scroll(c).Height(88).Children(func() {
							ui.Column(c).Padding(0, 6).Children(func() {
								for _, database := range v.databases {
									database := database
									row := ui.Row(c.Key("dbdatabase:" + database)).Height(24).
										PaddingStart(4).PaddingEnd(8).Gap(4).Radius(5).AlignItems(ui.Center)
									if database == v.currentDatabase || row.Hovered() {
										row.Background(p.SurfaceHover)
									}
									row.Children(func() {
										oiIconEl(c, "openidea/database/schema", 16, oiPaletteOf(p), p.TextMuted).MarginEnd(2)
										ui.Text(c, database).FontSize(13).TextColor(p.Text).SingleLine().Ellipsis("…")
									})
									if row.Clicked() {
										m.SelectDBDatabase(database)
									}
								}
							})
						})
					}
					if len(v.schemas) > 0 {
						ui.Text(c, "Schemas").FontSize(12).TextColor(p.TextMuted).SingleLine()
						ui.Scroll(c).Height(88).Children(func() {
							ui.Column(c).Padding(0, 6).Children(func() {
								for _, schema := range v.schemas {
									schema := schema
									r := ui.Row(c.Key("dbschema:" + schema)).Height(24).PaddingStart(4).
										PaddingEnd(8).Gap(4).Radius(5).AlignItems(ui.Center)
									if v.schema == schema || r.Hovered() {
										r.Background(p.SurfaceHover)
									}
									r.Children(func() {
										oiIconEl(c, "openidea/database/schema", 16, oiPaletteOf(p), p.TextMuted).
											MarginEnd(2)
										ui.Text(c, schema).FontSize(13).TextColor(p.Text).
											SingleLine().Ellipsis("…")
									})
									if r.Clicked() {
										m.loadDBTables(v.source, schema)
									}
								}
							})
						})
					}
					ui.Text(c, "Tables").FontSize(12).TextColor(p.TextMuted).SingleLine()
					ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
						ui.Column(c).Padding(0, 6).Children(func() {
							if v.collapsed {
								return
							}
							for _, t := range v.tables {
								t := t
								icon := "openidea/database/table"
								if t.Kind != "table" {
									icon = "openidea/database/view"
								}
								r := ui.Row(c.Key("dbtable:" + t.Name)).Height(24).PaddingStart(4).
									PaddingEnd(8).Gap(4).Radius(5).AlignItems(ui.Center)
								if v.selectedTable.Name == t.Name || r.Hovered() {
									r.Background(p.SurfaceHover)
								}
								r.Children(func() {
									oiIconEl(c, icon, 16, oiPaletteOf(p), p.TextMuted).MarginEnd(2)
									ui.Text(c, t.Name).FontSize(13).TextColor(p.Text).
										SingleLine().Ellipsis("…")
								})
								if r.Clicked() {
									v.selectedTable = t
									v.sql = "SELECT * FROM " + m.dbQualifiedName(v.source, v.schema, t.Name)
									m.invalidate()
								}
								if r.DoubleClicked() {
									m.OpenDBTable(v.schema, t.Name)
								}
								r.ContextMenu(func(menu *ui.Menu) {
									target := map[string]any{
										"source": v.source, "schema": v.schema,
										"table": t.Name, "kind": t.Kind,
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
							}
							objects := v.objects[v.source+"\x00"+v.schema]
							renderObject := func(kind, name, detail, arguments string) {
								icon := "openidea/database/table"
								switch kind {
								case "function", "procedure":
									icon = "openidea/database/function"
								case "sequence":
									icon = "openidea/database/sequence"
								}
								row := ui.Row(c.Key("dbobject:" + kind + ":" + name)).Height(24).
									PaddingStart(18).PaddingEnd(8).Gap(4).Radius(5).AlignItems(ui.Center)
								if row.Hovered() {
									row.Background(p.SurfaceHover)
								}
								row.Children(func() {
									oiIconEl(c, icon, 16, oiPaletteOf(p), p.TextMuted).MarginEnd(2)
									ui.Text(c, name).FontSize(13).TextColor(p.Text).SingleLine().Ellipsis("…")
									if detail != "" {
										ui.Spacer(c)
										ui.Text(c, oneLine(detail)).FontSize(11).TextColor(p.TextMuted).
											SingleLine().Ellipsis("…")
									}
								})
								target := map[string]any{
									"source": v.source, "schema": v.schema,
									"objectKind": kind, "objectName": name, "arguments": arguments,
								}
								if row.DoubleClicked() {
									m.DBGoToDDL(target)
								}
								row.ContextMenu(func(menu *ui.Menu) {
									if menu.Item("Go to DDL").Chosen() {
										m.DBGoToDDL(target)
									}
									if menu.Item("Quick Documentation").Chosen() {
										m.DBQuickDoc(target)
									}
									if menu.Item("Copy Reference").Chosen() {
										m.DBCopyReference(target)
									}
									menu.Separator()
									if menu.Item("Rename…").Chosen() {
										m.DBRename(target)
									}
									if menu.Item("Drop…").Chosen() {
										m.DBDrop(target)
									}
								})
							}
							if !v.separateProcedures {
								for _, routine := range objects.Routines {
									detail := routine.Arguments
									if routine.Returns != nil {
										detail += " → " + *routine.Returns
									}
									renderObject(routine.Kind, routine.Name, detail, routine.Arguments)
								}
							} else {
								for _, kind := range []string{"procedure", "function"} {
									for _, routine := range objects.Routines {
										if routine.Kind == kind {
											detail := routine.Arguments
											if routine.Returns != nil {
												detail += " → " + *routine.Returns
											}
											renderObject(routine.Kind, routine.Name, detail, routine.Arguments)
										}
									}
								}
							}
							for _, pkg := range objects.Packages {
								renderObject("package", pkg.Name, "", "")
							}
							for _, seq := range objects.Sequences {
								renderObject("sequence", seq.Name, seq.DataType, "")
							}
							for _, other := range objects.Others {
								renderObject(other.Kind, other.Name, other.Detail, "")
							}
						})
					})
				})
				ui.Column(c).Grow(1).MinWidth(0).Gap(8).Children(func() {
					area := ui.TextArea(c, &v.sql).Height(120).Font("monospace").FontSize(12)
					area.OnShortcut(ui.Cmd, ui.KeyEnter, func() { m.RunDB() })
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Button(c, "Run").Disabled(v.running).OnClick(func() { m.RunDB() })
						ui.Button(c, "History").OnClick(func() { m.OpenDBHistory() })
						ui.Button(c, "Explain").OnClick(func() { m.ExplainDB() })
						ui.Button(c, "Query History").OnClick(func() { m.DBHistoryAction() })
						ui.Button(c, "Run Script…").OnClick(func() { m.DBRunSQLScript(nil) })
						activeTarget := v.activeTarget()
						txLabel := "Tx: Auto"
						if v.manualCommit[activeTarget] {
							txLabel = "Tx: Manual"
						}
						ui.Button(c, txLabel).OnClick(func() { m.ToggleDBManualCommit() })
						if v.manualCommit[activeTarget] {
							ui.Button(c, "Commit").OnClick(func() { m.FinishDBTransaction(true) })
							ui.Button(c, "Rollback").OnClick(func() { m.FinishDBTransaction(false) })
						}
						ui.Text(c, "Cmd+Enter runs the statement; results show the first 50 rows").FontSize(12).
							TextColor(p.TextMuted).SingleLine()
					})
					_, exportable := m.dbExportDataForResult(v.active)
					ui.Row(c).Gap(2).AlignItems(ui.Center).Children(func() {
						for i := -1; i < len(v.results); i++ {
							i := i
							title := "Output"
							if i >= 0 {
								title = v.results[i].title
							}
							on := v.active == i
							b := ui.ButtonBase(c.Key(fmt.Sprintf("dbresult:%d", i))).Height(24).
								Padding(0, 8).Radius(4).AlignItems(ui.Center).Cursor(ui.CursorPointer)
							if on {
								b.Background(p.SurfaceHover)
								b.BorderWidth(0, 0, 1, 0).BorderColor(p.Accent)
							} else if b.Hovered() {
								b.Background(p.SurfaceHover)
							}
							b.Children(func() {
								ui.Text(c, title).FontSize(12).TextColor(p.TextMuted).
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
									if menu.Item("Close All").Chosen() {
										m.DBCloseAllResultsAction()
									}
									if menu.Item("Rename Tab…").Chosen() {
										m.DBRenameResultAction(i)
									}
									menu.Separator()
									if menu.Item("Next Result").Chosen() {
										m.StepDBResult(1)
									}
									if menu.Item("Previous Result").Chosen() {
										m.StepDBResult(-1)
									}
									if _, ok := m.dbExportDataForResult(i); ok {
										menu.Separator()
										if menu.Item("Export Data…").Chosen() {
											m.OpenDBExport(i)
										}
									}
								})
							}
						}
						ui.Spacer(c)
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
										m.DBExportCopy(map[string]any{
											"index":  index,
											"format": format.ID,
										})
									}
								}
							})
						}
					})
					if v.err != "" {
						ui.Text(c, v.err).FontSize(12).TextColor(p.Danger).
							SingleLine().Ellipsis("…")
					}
					ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
						if v.active < 0 {
							ui.Column(c).Padding(4, 6).Gap(2).Children(func() {
								for _, line := range v.outLog {
									ui.Text(c, line).FontSize(12).TextColor(p.TextMuted).
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
										ui.Text(c, oneLine(o.Statement.Text)).FontSize(12).
											TextColor(p.TextMuted).SingleLine().Ellipsis("…").Grow(1)
										ui.Text(c, dbOutcomeSummary(o)).FontSize(12).
											TextColor(p.TextMuted).SingleLine()
									})
									if o.Error != "" {
										ui.Text(c, o.Error).FontSize(12).TextColor(p.Danger).SingleLine()
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
											ui.Text(c, col.Name).FontSize(12).Bold().TextColor(p.Text).
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
											Height(24).Padding(0, 6).Gap(4).AlignItems(ui.Center)
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
		m.OpenDBTable(v.schema, "people")
	})
	wait("tablePage", func() bool {
		s := m.dbTable
		return s != nil && !s.loading && (s.err != "" || len(s.page.Result.Rows) > 0)
	})
	mygo.RunOnMain(func() {
		s := m.dbTable
		if s == nil {
			return
		}
		out["tableRows"] = len(s.page.Result.Rows)
		out["tableColumns"] = len(s.page.Result.Columns)
		out["tableIndexes"] = len(s.page.Details.Indexes)
		pk := 0
		for _, c := range s.page.Details.Columns {
			if c.PrimaryKey {
				pk++
			}
		}
		out["tablePrimaryKeys"] = pk
		if s.err != "" {
			out["tableError"] = s.err
		}
		editRow, editCol := -1, -1
		for row := range s.page.Result.Rows {
			for col, column := range s.page.Result.Columns {
				value := dbTableDisplayedCell(s, row, col)
				if column.Name == "name" && value != nil && *value == "Ada" {
					editRow, editCol = row, col
					break
				}
			}
			if editRow >= 0 {
				break
			}
		}
		if editRow < 0 {
			out["tableEditError"] = "the smoke table did not contain people.name = Ada"
			m.ToggleDBTableDDL()
			return
		}
		m.DBStartCellEdit(editRow, editCol, false)
		if !s.editing {
			out["tableEditError"] = "a short text cell did not open the inline editor"
			m.ToggleDBTableDDL()
			return
		}
		s.editText = "Ada Lovelace"
		m.DBCommitCellEdit()
		m.DBSubmitChanges()
	})
	wait("tableEdit", func() bool {
		s := m.dbTable
		if s == nil || s.submitting || s.loading {
			return false
		}
		if s.err != "" {
			return true
		}
		for row := range s.page.Result.Rows {
			for col, column := range s.page.Result.Columns {
				if column.Name != "name" {
					continue
				}
				value := dbTableDisplayedCell(s, row, col)
				if value != nil && *value == "Ada Lovelace" {
					return true
				}
			}
		}
		return false
	})
	mygo.RunOnMain(func() {
		if s := m.dbTable; s != nil {
			out["tableEdited"] = !dbTableHasChanges(s) && s.err == ""
			for row := range s.page.Result.Rows {
				for col, column := range s.page.Result.Columns {
					if column.Name != "name" {
						continue
					}
					if value := dbTableDisplayedCell(s, row, col); value != nil && *value == "Ada Lovelace" {
						out["tableEditedValue"] = *value
					}
				}
			}
			if s.err != "" {
				out["tableEditError"] = s.err
			}
		}
		m.ToggleDBTableDDL()
	})
	wait("ddl", func() bool {
		s := m.dbTable
		return s != nil && !s.ddlLoad && (s.ddl != "" || s.ddlErr != "")
	})
	mygo.RunOnMain(func() {
		if s := m.dbTable; s != nil {
			out["tableDDL"] = strings.Contains(strings.ToUpper(s.ddl), "CREATE")
			if s.ddlErr != "" {
				out["tableDDLError"] = s.ddlErr
			}
			m.closeDBTable()
			out["tableClosed"] = m.dbTable == nil
		}
		m.OpenDBHistory()
	})
	wait("history", func() bool {
		h := m.dbHist
		return h != nil && !h.loading && (h.err != "" || len(h.items) > 0)
	})
	mygo.RunOnMain(func() {
		if h := m.dbHist; h != nil {
			out["history"] = len(h.items)
			if h.err != "" {
				out["historyError"] = h.err
			}
			if len(h.items) > 0 {
				out["historyFirst"] = dbHistoryLabel(h.items[0])
			}
		}
		m.closeDBHistory()
		if m.db != nil {
			m.db.sql = "SELECT * FROM people"
			m.ExplainDB()
		}
	})
	wait("plan", func() bool {
		p := m.dbPlan
		return p != nil && !p.loading && (p.err != "" || len(p.nodes) > 0)
	})
	mygo.RunOnMain(func() {
		if p := m.dbPlan; p != nil {
			out["planNodes"] = len(p.nodes)
			if p.err != "" {
				out["planError"] = p.err
			}
			if len(p.nodes) > 0 {
				out["planFirst"] = p.nodes[0].Operation
			}
			m.closeDBPlan()
		}
		m.closeDatabase()
		out["closed"] = m.db == nil || !m.db.open
	})
	return out, nil
}

// dbTableSheet is the open table data page.
type dbTableSheet struct {
	open        bool
	source      string
	schema      string
	table       string
	driver      string
	readOnly    bool
	offset      int
	limit       int
	page        openidea.DBTablePage
	loading     bool
	err         string
	ddl         string
	ddlErr      string
	showDDL     bool
	showDetails bool
	ddlLoad     bool
	gen         uint64
	filter      string
	sort        []openidea.DBSortKey
	selectedRow int
	selectedCol int
	// pending maps "row:column" to an edited value. deletes and inserts use
	// the same edit representation OpenIDEA submits.
	pending    map[string]*string
	deletes    map[int]bool
	inserts    []map[string]*string
	valueOpen  bool
	valueText  string
	valueFind  string
	valueRO    bool
	editing    bool
	editRow    int
	editCol    int
	editText   string
	closeAsk   bool
	submitting bool
}

// OpenDBTable shows the data page of a table of the selected source.
func (m *Model) OpenDBTable(schema, table string) {
	if m.db == nil || m.db.source == "" || table == "" {
		return
	}
	if schema == "" {
		schema = m.db.schema
	}
	source, _ := m.dbSourceByID(m.db.activeTarget())
	m.dbTable = &dbTableSheet{
		open: true, source: m.db.activeTarget(), schema: schema, table: table,
		driver: source.Driver, readOnly: source.ReadOnly, limit: 100,
		selectedRow: -1, selectedCol: -1, editRow: -1, editCol: -1,
		pending: map[string]*string{}, deletes: map[int]bool{},
	}
	m.loadDBTablePage()
}

// closeDBTable drops the data page.
func (m *Model) closeDBTable() {
	if m.dbTable != nil {
		m.dbTable.gen++
	}
	m.dbTable = nil
	m.invalidate()
}

// loadDBTablePage asks the backend for the page at the sheet's offset.
func (m *Model) loadDBTablePage() {
	s := m.dbTable
	if s == nil || s.loading {
		return
	}
	s.loading = true
	s.err = ""
	s.gen++
	gen := s.gen
	source, schema, table := s.source, s.schema, s.table
	offset, limit := s.offset, s.limit
	var filter *string
	if strings.TrimSpace(s.filter) != "" {
		value := strings.TrimSpace(s.filter)
		filter = &value
	}
	sort := append([]openidea.DBSortKey(nil), s.sort...)
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
		defer cancel()
		page, err := m.mgr.DBTablePageQuery(ctx, Window, m.nextRequest(), source, schema, table,
			filter, sort, uint32(offset), uint32(limit))
		m.post(func() {
			if m.dbTable != s || s.gen != gen {
				return
			}
			s.loading = false
			if err != nil {
				s.err = "Could not read the table: " + err.Error()
			} else {
				s.page = page
			}
			m.invalidate()
		})
	}()
}

// DBTablePage moves the data page by whole pages, -1 previous, 1 next.
func (m *Model) DBTableStepPage(delta int) {
	s := m.dbTable
	if s == nil || s.loading {
		return
	}
	if !m.dbBlockTableNavigation("Changing the page") {
		return
	}
	next := s.offset + delta*s.limit
	if next < 0 || (delta > 0 && !s.page.Result.HasMore) {
		return
	}
	s.offset = next
	m.loadDBTablePage()
}

// ToggleDBTableDDL shows the table's CREATE statement, loading it once.
func (m *Model) ToggleDBTableDDL() {
	s := m.dbTable
	if s == nil {
		return
	}
	s.showDDL = !s.showDDL
	s.showDetails = false
	if !s.showDDL || s.ddl != "" || s.ddlLoad {
		m.invalidate()
		return
	}
	s.ddlLoad = true
	gen := s.gen
	source, schema, table := s.source, s.schema, s.table
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		ddl, err := m.mgr.DBTableDDL(ctx, Window, source, schema, table)
		m.post(func() {
			if m.dbTable != s || s.gen != gen {
				return
			}
			s.ddlLoad = false
			if err != nil {
				s.ddlErr = "Could not read the DDL: " + err.Error()
			} else {
				s.ddl = ddl
			}
			m.invalidate()
		})
	}()
}

// ToggleDBTableDetails shows the table's columns, indexes, foreign keys and
// triggers, which the page already carries.
func (m *Model) ToggleDBTableDetails() {
	s := m.dbTable
	if s == nil {
		return
	}
	s.showDetails = !s.showDetails
	s.showDDL = false
	m.invalidate()
}

// dbTableDetailLines lists a table's schema as the details view shows it.
func dbTableDetailLines(d openidea.DBTableDetails) []string {
	out := []string{fmt.Sprintf("Columns (%d)", len(d.Columns))}
	for _, c := range d.Columns {
		flags := ""
		if c.PrimaryKey {
			flags += " · primary key"
		}
		if !c.Nullable {
			flags += " · not null"
		}
		if c.Default != nil && *c.Default != "" {
			flags += " · default " + *c.Default
		}
		out = append(out, fmt.Sprintf("  %s  %s%s", c.Name, c.DataType, flags))
	}
	if len(d.Indexes) > 0 {
		out = append(out, fmt.Sprintf("Indexes (%d)", len(d.Indexes)))
		for _, ix := range d.Indexes {
			kind := ""
			if ix.Unique {
				kind = " · unique"
			}
			out = append(out, fmt.Sprintf("  %s (%s)%s", ix.Name, strings.Join(ix.Columns, ", "), kind))
		}
	}
	if len(d.ForeignKeys) > 0 {
		out = append(out, fmt.Sprintf("Foreign keys (%d)", len(d.ForeignKeys)))
		for _, fk := range d.ForeignKeys {
			ref := fk.RefSchema + "." + fk.RefTable
			out = append(out, fmt.Sprintf("  %s → %s(%s)",
				strings.Join(fk.Columns, ", "), ref, strings.Join(fk.RefColumns, ", ")))
		}
	}
	if len(d.Triggers) > 0 {
		out = append(out, fmt.Sprintf("Triggers (%d)", len(d.Triggers)))
		for _, tg := range d.Triggers {
			out = append(out, fmt.Sprintf("  %s  %s %s", tg.Name, tg.Timing, tg.Events))
		}
	}
	return out
}

// dbTableRange labels the rows a page shows.
func dbTableRange(s *dbTableSheet) string {
	if s == nil || len(s.page.Result.Rows) == 0 {
		return "No rows"
	}
	from := s.offset + 1
	to := s.offset + len(s.page.Result.Rows)
	return fmt.Sprintf("Rows %d–%d", from, to)
}

// dbTableDialog draws the data page.
func (m *Model) dbTableDialog(c *ui.Context, p *ui.Theme) {
	s := m.dbTable
	if s == nil {
		return
	}
	if s.closeAsk {
		open := s.closeAsk
		nativeModal(c, &open, func() {
			ui.Column(c).Width(430).Gap(10).Children(func() {
				ui.Text(c, "Unsaved table changes").FontSize(13).Bold().TextColor(p.Text).SingleLine()
				ui.Text(c, "Submit the row changes before closing, or discard them.").
					FontSize(12).TextColor(p.TextMuted)
				ui.Row(c).Gap(8).Children(func() {
					ui.Spacer(c)
					ui.Button(c, "Cancel").OnClick(func() { s.closeAsk = false; m.invalidate() })
					ui.Button(c, "Discard").OnClick(func() { m.DBDiscardAndCloseTable() })
					ui.Button(c, "Submit").Disabled(s.submitting || s.err != "").
						OnClick(func() {
							s.closeAsk = false
							m.DBSubmitChanges()
						})
				})
			})
		})
		if !open && m.dbTable == s {
			s.closeAsk = false
			m.invalidate()
		}
		return
	}

	editable, editNotice := dbTableEditability(s)
	open := s.open
	nativeModal(c, &open, func() {
		content := ui.Column(c).Width(940).Height(600).Gap(8)
		content.Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Table data: "+s.table).FontSize(13).Bold().TextColor(p.Text).SingleLine()
				if s.loading {
					ui.Text(c, "Loading…").FontSize(12).TextColor(p.TextMuted).SingleLine()
				} else {
					ui.Text(c, dbTableRange(s)).FontSize(12).TextColor(p.TextMuted).SingleLine()
				}
				ui.Spacer(c)
				ui.Button(c, "Refresh").Disabled(s.loading).
					OnClick(func() { m.DBRefreshTablePage() })
				ui.Button(c, "Previous").Disabled(s.offset == 0 || s.loading).
					OnClick(func() { m.DBTableStepPage(-1) })
				ui.Button(c, "Next").Disabled(!s.page.Result.HasMore || s.loading).
					OnClick(func() { m.DBTableStepPage(1) })
				pageSize := strconv.Itoa(s.limit)
				if choice := ui.Select(c, &pageSize, []string{"50", "100", "200", "500"}).Width(76); choice.Changed() {
					if m.dbBlockTableNavigation("Changing the page size") {
						if size, err := strconv.Atoi(pageSize); err == nil {
							s.limit = size
							s.offset = 0
							m.loadDBTablePage()
						}
					}
				}
				ui.Button(c, "Details").OnClick(func() { m.ToggleDBTableDetails() })
				ui.Button(c, "DDL").OnClick(func() { m.ToggleDBTableDDL() })
				ui.Button(c, "Go to Column").OnClick(func() { m.DBGoToColumn() })
				if editable {
					ui.Button(c, "Add Row").OnClick(func() { m.DBAddTableRow() })
					ui.Button(c, "Submit").Disabled(!dbTableHasChanges(s) || s.submitting).
						OnClick(func() { m.DBSubmitChanges() })
					ui.Button(c, "Revert").Disabled(!dbTableHasChanges(s)).
						OnClick(func() { m.DBRevertChanges() })
				}
				ui.Button(c, "Close").OnClick(func() { m.DBCloseTable() })
			})
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Filter").FontSize(12).TextColor(p.TextMuted).SingleLine()
				ui.TextArea(c, &s.filter).Height(28).SingleLine().Grow(1)
				ui.Button(c, "Apply").OnClick(func() { m.DBApplyTableFilter() })
				ui.Button(c, "Clear").OnClick(func() { m.DBClearTableFilter() })
				deleteLabel := "Delete Row"
				if s.selectedRow >= 0 {
					if inserted, ok := dbTableInserted(s, s.selectedRow); ok {
						_ = inserted
						deleteLabel = "Remove Added Row"
					} else if s.deletes[s.selectedRow] {
						deleteLabel = "Restore Row"
					}
				}
				ui.Button(c, deleteLabel).Disabled(!editable || s.selectedRow < 0).
					OnClick(func() { m.DBToggleDeletedTableRow() })
				if pending := dbTablePendingCount(s); pending > 0 {
					ui.Text(c, fmt.Sprintf("%d pending", pending)).
						FontSize(12).TextColor(p.TextMuted).SingleLine()
				}
			})
			if !editable && s.page.Result.Columns != nil {
				ui.Text(c, editNotice).FontSize(12).TextColor(p.TextMuted).SingleLine().Ellipsis("…")
			}
			if s.err != "" {
				ui.Text(c, s.err).FontSize(12).TextColor(p.Danger).SingleLine().Ellipsis("…")
			}
			if s.showDDL {
				ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
					ui.Column(c).Padding(8, 8).Children(func() {
						if s.ddlErr != "" {
							ui.Text(c, s.ddlErr).FontSize(12).TextColor(p.Danger)
						}
						for _, line := range strings.Split(strings.TrimRight(s.ddl, "\n"), "\n") {
							ui.Text(c, line).FontSize(12).Font("monospace").TextColor(p.Text).SingleLine()
						}
					})
				})
				return
			}
			if s.showDetails {
				ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
					ui.Column(c).Padding(8, 8).Children(func() {
						for _, line := range dbTableDetailLines(s.page.Details) {
							ui.Text(c, line).FontSize(12).Font("monospace").TextColor(p.Text).SingleLine()
						}
					})
				})
				return
			}
			ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
				ui.Column(c).Children(func() {
					ui.Row(c).Height(26).Children(func() {
						ui.Text(c, "#").FontSize(12).Bold().TextColor(p.TextMuted).Width(38).SingleLine()
						for colIndex, column := range s.page.Result.Columns {
							colIndex, column := colIndex, column
							head := ui.ButtonBase(c.Key(fmt.Sprintf("dbtheader:%d", colIndex))).
								Height(26).Width(168).AlignItems(ui.Center)
							if head.Hovered() {
								head.Background(p.SurfaceHover)
							}
							head.Children(func() {
								label := column.Name
								for _, key := range s.sort {
									if key.Column == column.Name {
										if key.Descending {
											label += " ↓"
										} else {
											label += " ↑"
										}
									}
								}
								ui.Text(c, label).FontSize(12).Bold().TextColor(p.TextMuted).
									SingleLine().Ellipsis("…")
							})
							head.OnClick(func() { m.DBSortTableColumn(colIndex) })
						}
					})
					for rowIndex := 0; rowIndex < dbTableRowCount(s); rowIndex++ {
						rowIndex := rowIndex
						_, inserted := dbTableInserted(s, rowIndex)
						deleted := !inserted && s.deletes[rowIndex]
						ui.Row(c).Height(28).Children(func() {
							rowNumberText := strconv.Itoa(s.offset + rowIndex + 1)
							tone := p.TextMuted
							if inserted {
								rowNumberText = "+"
								tone = p.Accent
							} else if deleted {
								tone = p.Danger
							}
							number := ui.ButtonBase(c.Key(fmt.Sprintf("dbrow:%d", rowIndex))).
								Height(28).Width(38).AlignItems(ui.Center)
							if rowIndex == s.selectedRow {
								number.Background(p.SurfaceHover)
							}
							number.Children(func() {
								ui.Text(c, rowNumberText).FontSize(11).TextColor(tone).SingleLine()
							})
							number.OnClick(func() {
								s.selectedRow = rowIndex
								if s.selectedCol < 0 {
									s.selectedCol = 0
								}
								m.invalidate()
							})
							if number.DoubleClicked() {
								s.selectedRow = rowIndex
								if len(s.page.Result.Columns) > 0 {
									m.DBStartCellEdit(rowIndex, 0, false)
								}
							}
							number.ContextMenu(func(menu *ui.Menu) {
								label := "Delete Row"
								if deleted {
									label = "Restore Row"
								}
								if inserted {
									label = "Remove Added Row"
								}
								if menu.Item(label).Disabled(!editable).Chosen() {
									s.selectedRow = rowIndex
									m.DBToggleDeletedTableRow()
								}
							})
							for colIndex := range s.page.Result.Columns {
								colIndex := colIndex
								cell := dbTableDisplayedCell(s, rowIndex, colIndex)
								selected := rowIndex == s.selectedRow && colIndex == s.selectedCol
								editing := s.editing && rowIndex == s.editRow && colIndex == s.editCol
								key := fmt.Sprintf("%d:%d", rowIndex, colIndex)
								modified := false
								if !inserted {
									_, modified = s.pending[key]
								} else if insertedIndex, ok := dbTableInserted(s, rowIndex); ok {
									_, modified = s.inserts[insertedIndex][s.page.Result.Columns[colIndex].Name]
								}
								if editing {
									area := ui.TextArea(c.Key("dbedit:"+key), &s.editText).
										Height(28).Width(168).Font("monospace").FontSize(12).SingleLine()
									area.OnChange(func() { m.invalidate() })
									area.OnSubmit(func() { m.DBCommitCellEdit() })
									continue
								}
								text := "<null>"
								tone := p.TextMuted
								if cell != nil {
									text = *cell
									tone = p.Text
								}
								cellButton := ui.ButtonBase(c.Key("dbcell:" + key)).Height(28).
									Width(168).PaddingStart(4).AlignItems(ui.Center)
								if selected {
									cellButton.Background(p.Selection)
								} else if deleted {
									cellButton.Background(p.SurfaceHover)
								}
								cellButton.Children(func() {
									columnType := dbTableColumnType(s, colIndex)
									if editable && !deleted && dbIsBooleanType(columnType, s.driver) {
										values, _ := dbBooleanLiterals(s.driver)
										checked := cell != nil && strings.EqualFold(*cell, values.trueValue)
										if ui.CheckboxBase(c, &checked).Changed() {
											s.selectedRow, s.selectedCol = rowIndex, colIndex
											m.DBToggleBooleanCell()
										}
										if cell == nil {
											ui.Text(c, "<null>").FontSize(12).Font("monospace").
												TextColor(p.TextMuted).SingleLine()
										}
									} else {
										ui.Text(c, text).FontSize(12).Font("monospace").TextColor(tone).
											SingleLine().Ellipsis("…")
									}
								})
								if modified {
									cellButton.BorderWidth(0, 0, 1, 0).BorderColor(p.Accent)
								}
								cellButton.OnClick(func() { m.DBSelectTableCell(rowIndex, colIndex) })
								if cellButton.DoubleClicked() {
									m.DBStartCellEdit(rowIndex, colIndex, false)
								}
								cellButton.ContextMenu(func(menu *ui.Menu) {
									if menu.Item("Edit Cell").Disabled(!dbTableEditableCell(s, rowIndex, colIndex)).Chosen() {
										m.DBStartCellEdit(rowIndex, colIndex, false)
									}
									if menu.Item("Open Value in Editor").Chosen() {
										s.selectedRow, s.selectedCol = rowIndex, colIndex
										m.DBOpenValueWindow()
									}
									if menu.Item("Set NULL").
										Disabled(!editable || !dbTableColumnNullable(s, colIndex)).Chosen() {
										s.selectedRow, s.selectedCol = rowIndex, colIndex
										m.DBSetCellNull()
									}
									_, defaultOK := dbTableDefaultValue(s, colIndex)
									_, isInserted := dbTableInserted(s, rowIndex)
									if menu.Item("Set DEFAULT").
										Disabled(!editable || (!isInserted && !defaultOK)).Chosen() {
										s.selectedRow, s.selectedCol = rowIndex, colIndex
										m.DBSetCellDefault()
									}
									if menu.Item("Toggle Boolean").
										Disabled(!editable || !dbIsBooleanType(dbTableColumnType(s, colIndex), s.driver)).Chosen() {
										s.selectedRow, s.selectedCol = rowIndex, colIndex
										m.DBToggleBooleanCell()
									}
								})
							}
						})
					}
				})
			})
		})
		content.OnShortcut(0, ui.KeyEscape, func() {
			if s.editing {
				m.DBCancelCellEdit()
				return
			}
			if s.valueOpen {
				s.valueOpen = false
				m.invalidate()
				return
			}
			m.DBCloseTable()
		})
		content.OnShortcut(0, ui.KeyUp, func() {
			if !s.editing {
				m.DBMoveTableCell(-1, 0)
			}
		})
		content.OnShortcut(0, ui.KeyDown, func() {
			if !s.editing {
				m.DBMoveTableCell(1, 0)
			}
		})
		content.OnShortcut(0, ui.KeyLeft, func() {
			if !s.editing {
				m.DBMoveTableCell(0, -1)
			}
		})
		content.OnShortcut(0, ui.KeyRight, func() {
			if !s.editing {
				m.DBMoveTableCell(0, 1)
			}
		})
		content.OnShortcut(0, ui.KeyEnter, func() {
			if !s.editing {
				m.DBStartCellEdit(s.selectedRow, s.selectedCol, false)
			}
		})
		content.OnShortcut(ui.Shift, ui.KeyEnter, func() {
			if !s.editing {
				m.DBStartCellEdit(s.selectedRow, s.selectedCol, true)
			}
		})
		content.OnShortcut(0, ui.KeyF2, func() {
			if !s.editing {
				m.DBStartCellEdit(s.selectedRow, s.selectedCol, false)
			}
		})
		content.OnShortcut(0, ui.KeySpace, func() {
			if !s.editing && editable {
				m.DBToggleBooleanCell()
			}
		})
		content.OnShortcut(ui.Cmd, ui.KeyEnter, func() { m.DBSubmitChanges() })
		content.OnShortcut(ui.Ctrl, ui.KeyEnter, func() { m.DBSubmitChanges() })
		content.OnShortcut(ui.Cmd|ui.Alt, ui.KeyZ, func() { m.DBRevertChanges() })
		content.OnShortcut(ui.Ctrl|ui.Alt, ui.KeyZ, func() { m.DBRevertChanges() })
		content.OnShortcut(ui.Cmd, ui.KeyBackspace, func() { m.DBSetCellNull() })
		content.OnShortcut(ui.Ctrl, ui.KeyBackspace, func() { m.DBSetCellNull() })
		content.OnShortcut(ui.Cmd|ui.Shift, ui.KeyBackspace, func() { m.DBSetCellDefault() })
		content.OnShortcut(ui.Ctrl|ui.Shift, ui.KeyBackspace, func() { m.DBSetCellDefault() })
	})
	if !open && m.dbTable == s {
		m.DBCloseTable()
	}
}

// dbHistoryPane is the open statement history list.
type dbHistoryPane struct {
	open    bool
	loading bool
	err     string
	items   []openidea.DBHistoryEntry
	sel     int
	gen     uint64
}

// dbPlanPane is the open execution plan of one statement.
type dbPlanPane struct {
	open      bool
	loading   bool
	err       string
	statement string
	nodes     []openidea.DBPlanNode
	rows      []dbPlanRow
}

// dbPlanRow is one plan node placed in the tree.
type dbPlanRow struct {
	index uint32
	depth int
}

// OpenDBHistory shows the statements run against the selected source.
func (m *Model) OpenDBHistory() {
	if m.db == nil || m.db.source == "" {
		return
	}
	h := &dbHistoryPane{open: true, loading: true}
	m.dbHist = h
	h.gen++
	gen := h.gen
	source := m.db.source
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		items, err := m.mgr.DBHistory(ctx, Window, source)
		m.post(func() {
			if m.dbHist != h || h.gen != gen {
				return
			}
			h.loading = false
			if err != nil {
				h.err = "Could not read the history: " + err.Error()
			} else {
				h.items = items
			}
			m.invalidate()
		})
	}()
}

// StepDBHistory moves the history selection, wrapping around.
func (m *Model) StepDBHistory(delta int) {
	h := m.dbHist
	if h == nil || len(h.items) == 0 {
		return
	}
	h.sel = (h.sel + delta + len(h.items)) % len(h.items)
	m.invalidate()
}

// ChooseDBHistory puts the selected statement into the console and runs it.
func (m *Model) ChooseDBHistory(i int) {
	h := m.dbHist
	if h == nil || i < 0 || i >= len(h.items) {
		return
	}
	text := h.items[i].Text
	m.closeDBHistory()
	if m.db != nil {
		m.db.sql = text
		m.invalidate()
	}
}

// closeDBHistory drops the history list.
func (m *Model) closeDBHistory() { m.dbHist = nil; m.invalidate() }

// ExplainDB shows the plan of the console's first statement.
func (m *Model) ExplainDB() {
	if m.db == nil || m.db.source == "" || strings.TrimSpace(m.db.sql) == "" {
		return
	}
	p := &dbPlanPane{open: true, loading: true, statement: m.db.sql}
	m.dbPlan = p
	m.invalidate()
	source := m.db.source
	schema := m.db.schema
	text := m.db.sql
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
		defer cancel()
		stmts, err := m.mgr.DBStatements(ctx, Window, source, text, 0, uint32(byteToUTF16Offset(text, len(text))))
		if err == nil && len(stmts) == 0 {
			err = fmt.Errorf("There is no statement to explain")
		}
		var nodes []openidea.DBPlanNode
		if err == nil {
			p.statement = stmts[0].Text
			nodes, err = m.mgr.DBExplain(ctx, Window, m.nextRequest(), source, stmts[0].Text, schema)
		}
		m.post(func() {
			if m.dbPlan != p {
				return
			}
			p.loading = false
			if err != nil {
				p.err = "Could not read the plan: " + err.Error()
			} else {
				p.nodes = nodes
				p.rows = dbPlanRows(nodes)
			}
			m.invalidate()
		})
	}()
}

// closeDBPlan drops the plan view.
func (m *Model) closeDBPlan() { m.dbPlan = nil; m.invalidate() }

// dbPlanRows places each node under its parent, depth first.
func dbPlanRows(nodes []openidea.DBPlanNode) []dbPlanRow {
	children := make(map[int][]uint32)
	var roots []uint32
	for i, n := range nodes {
		if n.Parent != nil && int(*n.Parent) < len(nodes) {
			children[int(*n.Parent)] = append(children[int(*n.Parent)], uint32(i))
		} else {
			roots = append(roots, uint32(i))
		}
	}
	out := make([]dbPlanRow, 0, len(nodes))
	var walk func(i uint32, depth int)
	walk = func(i uint32, depth int) {
		out = append(out, dbPlanRow{index: i, depth: depth})
		for _, c := range children[int(i)] {
			walk(c, depth+1)
		}
	}
	for _, r := range roots {
		walk(r, 0)
	}
	return out
}

// dbHistoryLabel names a history entry in the list.
func dbHistoryLabel(e openidea.DBHistoryEntry) string {
	line := strings.TrimSpace(strings.SplitN(strings.TrimSpace(e.Text), "\n", 2)[0])
	if len(line) > 90 {
		line = line[:90] + "…"
	}
	return line
}

// dbHistoryDialog draws the statement history list.
func (m *Model) dbHistoryDialog(c *ui.Context, p *ui.Theme) {
	h := m.dbHist
	if h == nil {
		return
	}
	open := h.open
	jump := -1
	nativeModal(c, &open, func() {
		ui.Column(c).Width(620).Gap(8).Children(func() {
			ui.Text(c, "Execution History").FontSize(13).Bold().TextColor(p.Text).SingleLine()
			if h.loading {
				ui.Text(c, "Loading…").FontSize(12).TextColor(p.TextMuted).SingleLine()
			} else if h.err != "" {
				ui.Text(c, h.err).FontSize(12).TextColor(p.Danger).SingleLine().Ellipsis("…")
			} else if len(h.items) == 0 {
				ui.Text(c, "No SQL has been executed yet.").FontSize(12).TextColor(p.TextMuted).SingleLine()
			} else {
				ui.Text(c, fmt.Sprintf("%s · Enter puts it in the console · Esc closes", count(len(h.items), "statement", "statements"))).
					FontSize(12).TextColor(p.TextMuted).SingleLine()
				// The list hugs its rows up to the cap; a fixed box left a
				// mostly empty sheet for a handful of statements.
				historyHeight := float32(len(h.items) * 24)
				if historyHeight > 320 {
					historyHeight = 320
				}
				ui.Scroll(c).Height(historyHeight).Children(func() {
					ui.Column(c).Children(func() {
						for i, e := range h.items {
							i, e := i, e
							row := ui.Row(c.Key(fmt.Sprintf("dbhist:%d", i))).Height(24).Padding(0, 8).
								AlignItems(ui.Center)
							if i == h.sel || row.Hovered() {
								row.Background(p.SurfaceHover)
							}
							row.Children(func() {
								ui.Text(c, dbHistoryLabel(e)).FontSize(13).TextColor(p.Text).
									SingleLine().Ellipsis("…")
							})
							if row.Clicked() {
								jump = i
								h.sel = i
							}
						}
					})
				})
			}
		})
	})
	if !open && m.dbHist != nil {
		m.closeDBHistory()
	}
	if jump >= 0 {
		m.ChooseDBHistory(jump)
	}
}

// dbPlanDialog draws an execution plan.
func (m *Model) dbPlanDialog(c *ui.Context, p *ui.Theme) {
	pl := m.dbPlan
	if pl == nil {
		return
	}
	open := pl.open
	nativeModal(c, &open, func() {
		ui.Column(c).Width(720).Gap(8).Children(func() {
			ui.Text(c, "Execution Plan").FontSize(13).Bold().TextColor(p.Text).SingleLine()
			ui.Text(c, strings.TrimSpace(strings.SplitN(pl.statement, "\n", 2)[0])).FontSize(12).
				TextColor(p.TextMuted).SingleLine().Ellipsis("…")
			if pl.loading {
				ui.Text(c, "Explaining…").FontSize(12).TextColor(p.TextMuted).SingleLine()
			} else if pl.err != "" {
				ui.Text(c, pl.err).FontSize(12).TextColor(p.Danger).SingleLine().Ellipsis("…")
			} else {
				planHeight := float32(len(pl.rows) * 24)
				if planHeight > 340 {
					planHeight = 340
				}
				ui.Scroll(c).Height(planHeight).Children(func() {
					ui.Column(c).Padding(0, 6).Children(func() {
						for _, r := range pl.rows {
							n := pl.nodes[r.index]
							ui.Row(c).Height(24).PaddingStart(4 + float32(r.depth)*m.treeIndentWidth()).
								PaddingEnd(8).Gap(4).AlignItems(ui.Center).Children(func() {
								oiIconEl(c, "openidea/database/plan", 16, oiPaletteOf(p), p.TextMuted).
									MarginEnd(2)
								ui.Text(c, n.Operation).FontSize(13).TextColor(p.Text).SingleLine()
								if n.Params != "" {
									ui.Text(c, n.Params).FontSize(12).TextColor(p.TextMuted).
										SingleLine().Ellipsis("…")
								}
							})
						}
					})
				})
			}
			ui.Row(c).Children(func() {
				ui.Spacer(c)
				ui.Button(c, "Close").OnClick(func() { m.closeDBPlan() })
			})
		})
	})
	if !open && m.dbPlan != nil {
		m.closeDBPlan()
	}
}

// dbActiveResult is the selected result tab index, -1 for Output or none.
func (m *Model) dbActiveResult() int {
	if m.db == nil {
		return -1
	}
	return m.db.active
}
