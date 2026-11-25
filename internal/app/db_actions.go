package app

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

// Database actions mirror OpenIDEA's database feature. A context menu or
// automation can pass a target map with source, schema, table, database,
// column, objectKind and objectName fields. Without one, the selected source,
// schema and table are used.
func init() {
	type spec struct {
		id, title, keywords string
		run                 func(*Model, any)
		when                func(*Model) bool
		enabled             func(*Model) bool
		toggled             func(*Model) bool
	}
	projectOpen := func(m *Model) bool { return m.Root() != "" }
	databaseOpen := func(m *Model) bool { return m.db != nil && m.db.open }
	tableEditorOpen := func(m *Model) bool { return m.dbTable != nil }
	tableEditorEditable := func(m *Model) bool {
		editable, _ := dbTableEditability(m.dbTable)
		return editable
	}
	tableEditorDirty := func(m *Model) bool {
		return dbTableHasChanges(m.dbTable) && !m.dbTable.submitting
	}
	selectedEditableCell := func(m *Model) bool {
		s := m.dbTable
		return s != nil && dbTableEditableCell(s, s.selectedRow, s.selectedCol)
	}
	valueEditorOpen := func(m *Model) bool {
		return m.dbTable != nil && m.dbTable.valueOpen
	}
	specs := []spec{
		{"db.pluginConnect", "Connect Plugin Data Source", "database plugin source connect", func(m *Model, a any) { m.DBConnectAction(a) }, projectOpen, nil, nil},
		{"db.showToolWindow", "Database", "data sources sql tables", func(m *Model, _ any) { m.OpenDatabase() }, projectOpen, nil, nil},
		{"db.newDataSource", "New Data Source…", "database connection postgres mysql sqlite", func(m *Model, _ any) { m.DBNewDataSource(nil) }, projectOpen, nil, nil},
		{"db.openConsole", "Query Console", "database sql console", func(m *Model, a any) { m.DBOpenConsole(a) }, projectOpen, nil, nil},
		{"db.execute", "Execute SQL Statement", "run query statement", func(m *Model, _ any) { m.DBExecuteAction() }, projectOpen, nil, nil},
		{"db.executeScript", "Execute SQL Script", "run all whole file statements", func(m *Model, _ any) { m.DBExecuteScriptAction() }, projectOpen, nil, nil},
		{"db.goToColumn", "Go to Column", "table editor file structure columns", func(m *Model, _ any) { m.DBGoToColumn() }, tableEditorOpen, nil, nil},
		{"db.submitChanges", "Submit", "table editor apply changes commit", func(m *Model, _ any) { m.DBSubmitChanges() }, tableEditorOpen, func(m *Model) bool {
			return tableEditorEditable(m) && tableEditorDirty(m)
		}, nil},
		{"db.revertChanges", "Revert", "table editor discard changes undo", func(m *Model, _ any) { m.DBRevertChanges() }, tableEditorOpen, tableEditorDirty, nil},
		{"db.explainPlan", "Explain Plan", "query plan explain sql", func(m *Model, _ any) { m.DBExplainAction() }, projectOpen, nil, nil},
		{"db.queryHistory", "Browse Query History", "database sql history", func(m *Model, _ any) { m.DBHistoryAction() }, projectOpen, nil, nil},
		{"db.properties", "Data Source Properties…", "connection settings", func(m *Model, a any) { m.DBProperties(a) }, projectOpen, nil, nil},
		{"db.refresh", "Refresh", "synchronize introspect", func(m *Model, a any) { m.DBRefreshAction(a) }, projectOpen, nil, nil},
		{"db.introspect", "Introspect All Databases", "snowflake completion catalog schemas", func(m *Model, a any) { m.DBIntrospect(a) }, projectOpen, nil, nil},
		{"db.disconnect", "Deactivate", "disconnect close connection", func(m *Model, a any) { m.DBDisconnectAction(a) }, projectOpen, nil, nil},
		{"db.jumpToConsole", "Jump to Query Console…", "database console", func(m *Model, a any) { m.DBOpenConsole(a) }, projectOpen, nil, nil},
		{"db.editData", "Edit Data", "open table table editor", func(m *Model, a any) { m.DBEditData(a) }, projectOpen, nil, nil},
		{"db.goToDdl", "Go to DDL", "definition create table", func(m *Model, a any) { m.DBGoToDDL(a) }, projectOpen, nil, nil},
		{"db.quickDoc", "Quick Documentation", "info schema", func(m *Model, a any) { m.DBQuickDoc(a) }, databaseOpen, nil, nil},
		{"db.rename", "Rename…", "edit object", func(m *Model, a any) { m.DBRename(a) }, databaseOpen, nil, nil},
		{"db.drop", "Drop…", "delete remove object data source", func(m *Model, a any) { m.DBDrop(a) }, databaseOpen, nil, nil},
		{"db.truncate", "Truncate…", "delete all rows empty table", func(m *Model, a any) { m.DBTruncate(a) }, databaseOpen, nil, nil},
		{"db.modifyTable", "Modify Table…", "alter table add modify column", func(m *Model, a any) { m.DBModifyTable(a) }, databaseOpen, nil, nil},
		{"db.newTable", "Table", "create table new", func(m *Model, a any) { m.DBNewTable(a) }, databaseOpen, nil, nil},
		{"db.newColumn", "Column", "add column", func(m *Model, a any) { m.DBNewColumn(a) }, databaseOpen, nil, nil},
		{"db.importData", "Import Data from File…", "csv tsv load data", func(m *Model, a any) { m.DBImportData(a) }, databaseOpen, nil, nil},
		{"db.copyReference", "Copy Reference", "qualified name clipboard", func(m *Model, a any) { m.DBCopyReference(a) }, projectOpen, nil, nil},
		{"db.duplicateSource", "Duplicate", "copy data source", func(m *Model, a any) { m.DBDuplicateSource(a) }, projectOpen, nil, nil},
		{"db.ddlToClipboard", "Generate DDL to Clipboard", "create table sql script copy", func(m *Model, a any) { m.DBDDLToClipboard(a) }, projectOpen, nil, nil},
		{"db.ddlToConsole", "Generate DDL to Query Console", "create table sql script", func(m *Model, a any) { m.DBDDLToConsole(a) }, projectOpen, nil, nil},
		{"db.runSqlScript", "Run SQL Script…", "execute file import sql", func(m *Model, a any) { m.DBRunSQLScript(a) }, projectOpen, nil, nil},
		{"db.sqlScript", "SQL Script Template", "select insert update delete", func(m *Model, a any) { m.DBSQLScript(a) }, databaseOpen, nil, nil},
		{"db.goToReferencedTable", "Go to Referenced Table", "foreign key", func(m *Model, a any) { m.DBGoToReferencedTable(a) }, databaseOpen, nil, nil},
		{"db.new", "New…", "query console data source table", func(m *Model, a any) { m.DBNew(a) }, projectOpen, nil, nil},
		{"db.separateProceduresAndFunctions", "Separate Procedures and Functions", "schema routines", func(m *Model, _ any) { m.DBSeparateObjects() }, databaseOpen, nil, func(m *Model) bool {
			return m.db != nil && m.db.separateProcedures
		}},
		{"db.collapseAll", "Collapse All", "tree folders", func(m *Model, _ any) { m.DBCollapseAll() }, databaseOpen, nil, nil},
		{"db.closeResultTab", "Close Result Tab", "close", func(m *Model, a any) { m.DBCloseResultAction(a) }, databaseOpen, nil, nil},
		{"db.closeOtherResultTabs", "Close Other Result Tabs", "close others", func(m *Model, a any) { m.DBCloseOtherResultsAction(a) }, databaseOpen, nil, nil},
		{"db.closeAllResultTabs", "Close All Result Tabs", "close all", func(m *Model, _ any) { m.DBCloseAllResultsAction() }, databaseOpen, nil, nil},
		{"db.renameResultTab", "Rename Result Tab…", "rename result", func(m *Model, a any) { m.DBRenameResultAction(a) }, databaseOpen, nil, nil},
		{"db.exportData", "Export Data…", "csv tsv json sql markdown clipboard file", func(m *Model, a any) { m.DBExportAction(a) }, databaseOpen, dbExportResultEnabled, nil},
		{"db.nextResultTab", "Select Next Result Tab", "next result", func(m *Model, _ any) { m.StepDBResult(1) }, databaseOpen, nil, nil},
		{"db.previousResultTab", "Select Previous Result Tab", "previous result", func(m *Model, _ any) { m.StepDBResult(-1) }, databaseOpen, nil, nil},
		{"db.toggleBooleanCell", "Toggle Boolean Value", "table editor boolean", func(m *Model, _ any) { m.DBToggleBooleanCell() }, tableEditorOpen, func(m *Model) bool {
			return selectedEditableCell(m) &&
				dbIsBooleanType(dbTableColumnType(m.dbTable, m.dbTable.selectedCol), m.dbTable.driver)
		}, nil},
		{"db.setCellNull", "Set NULL", "table editor null", func(m *Model, _ any) { m.DBSetCellNull() }, tableEditorOpen, func(m *Model) bool {
			return selectedEditableCell(m) && dbTableColumnNullable(m.dbTable, m.dbTable.selectedCol)
		}, nil},
		{"db.setCellDefault", "Set DEFAULT", "table editor default", func(m *Model, _ any) { m.DBSetCellDefault() }, tableEditorOpen, func(m *Model) bool {
			if !selectedEditableCell(m) {
				return false
			}
			if _, inserted := dbTableInserted(m.dbTable, m.dbTable.selectedRow); inserted {
				return true
			}
			_, ok := dbTableDefaultValue(m.dbTable, m.dbTable.selectedCol)
			return ok
		}, nil},
		{"db.findInValue", "Find in Value…", "value editor text search", func(m *Model, _ any) { m.DBFindInValue() }, valueEditorOpen, nil, nil},
		{"db.reformatValue", "Reformat Value", "json format pretty", func(m *Model, _ any) { m.DBReformatValue() }, valueEditorOpen, func(m *Model) bool {
			return m.dbTable != nil && !m.dbTable.valueRO
		}, nil},
		{"db.openValueWindow", "Open Value in Editor", "value editor window", func(m *Model, _ any) { m.DBOpenValueWindow() }, tableEditorOpen, nil, nil},
	}
	for _, s := range specs {
		s := s
		_ = registerAction(&Action{
			ID: s.id, Title: s.title, Category: "Database", Keywords: s.keywords,
			When: s.when, Enabled: s.enabled, Toggled: s.toggled,
			RunWithArg: func(m *Model, arg any) { s.run(m, arg) },
		})
	}
}

type dbActionTarget struct {
	Source     string
	Schema     string
	Table      string
	Kind       string
	Database   string
	Column     string
	ObjectKind string
	ObjectName string
	Arguments  string
	Filter     string
	Script     string
}

func dbTargetFrom(m *Model, arg any) dbActionTarget {
	var t dbActionTarget
	if values, ok := arg.(map[string]any); ok {
		get := func(names ...string) string {
			for _, name := range names {
				if value, ok := values[name]; ok {
					switch v := value.(type) {
					case string:
						return v
					case fmt.Stringer:
						return v.String()
					}
				}
			}
			return ""
		}
		t.Source = get("source", "sourceId", "id")
		t.Schema = get("schema")
		t.Table = get("table")
		t.Kind = get("kind")
		t.Database = get("database")
		t.Column = get("column")
		t.ObjectKind = get("objectKind")
		t.ObjectName = get("objectName", "name")
		t.Arguments = get("arguments")
		t.Filter = get("filter")
		t.Script = get("script")
	}
	if m.db != nil {
		if t.Source == "" {
			t.Source = m.db.source
		}
		if t.Schema == "" {
			t.Schema = m.db.schema
		}
		if t.Table == "" {
			t.Table = m.db.selectedTable.Name
		}
		if t.Kind == "" {
			t.Kind = m.db.selectedTable.Kind
		}
	}
	if m.dbTable != nil {
		if t.Source == "" {
			t.Source = m.dbTable.source
		}
		if t.Schema == "" {
			t.Schema = m.dbTable.schema
		}
		if t.Table == "" {
			t.Table = m.dbTable.table
		}
	}
	return t
}

func (m *Model) dbSourceByID(id string) (openidea.DataSource, bool) {
	if m.db == nil {
		return openidea.DataSource{}, false
	}
	for _, source := range m.db.sources {
		if source.ID == id {
			return source, true
		}
	}
	return openidea.DataSource{}, false
}

func (m *Model) dbEnsureOpen() *dbView {
	if m.db == nil {
		m.db = &dbView{open: true, manualCommit: map[string]bool{}, pendingTx: map[string]bool{}}
	}
	if m.db.connected == nil {
		m.db.connected = map[string]bool{}
	}
	if m.db.manualCommit == nil {
		m.db.manualCommit = map[string]bool{}
	}
	if m.db.pendingTx == nil {
		m.db.pendingTx = map[string]bool{}
	}
	if m.db.objects == nil {
		m.db.objects = map[string]openidea.DBSchemaObjects{}
	}
	if m.db.consoleSources == nil {
		m.db.consoleSources = map[string]string{}
	}
	m.db.open = true
	if len(m.db.sources) == 0 {
		m.RefreshDBSources()
	}
	m.invalidate()
	return m.db
}

func (v *dbView) activeTarget() string {
	if v != nil && v.databaseTarget != "" {
		return v.databaseTarget
	}
	if v != nil {
		return v.source
	}
	return ""
}

func (m *Model) DBConnectAction(arg any) {
	t := dbTargetFrom(m, arg)
	m.dbEnsureOpen()
	if t.Source == "" {
		return
	}
	m.SelectDBSource(t.Source)
	m.ConnectDBSource(t.Source)
}

func (m *Model) DBProperties(arg any) {
	t := dbTargetFrom(m, arg)
	m.dbEnsureOpen()
	m.OpenDBDataSourceForm(t.Source)
}

func (m *Model) DBNewDataSource(arg any) {
	m.dbEnsureOpen()
	if values, ok := arg.(map[string]any); ok && len(values) != 0 {
		m.OpenDBDataSourceFormMap(values)
		return
	}
	m.OpenDBDataSourceForm("")
}

func (m *Model) DBRefreshAction(arg any) {
	t := dbTargetFrom(m, arg)
	v := m.dbEnsureOpen()
	if t.Table != "" {
		m.OpenDBTable(t.Schema, t.Table)
		return
	}
	if t.Schema != "" && t.Source != "" {
		m.loadDBTables(t.Source, t.Schema)
		return
	}
	m.RefreshDBSources()
	if v.source != "" && v.connected[v.source] && v.schema != "" {
		m.loadDBTables(v.source, v.schema)
	}
}

func (m *Model) DBIntrospect(arg any) {
	t := dbTargetFrom(m, arg)
	v := m.dbEnsureOpen()
	if t.Source == "" {
		t.Source = v.source
	}
	if t.Source == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
		defer cancel()
		dbNames, err := m.mgr.DBDatabases(ctx, Window, t.Source)
		var catalog *openidea.DBSavedCatalog
		if err == nil {
			catalog, err = m.mgr.DBSavedCatalog(ctx, Window, t.Source)
		}
		m.post(func() {
			if m.db != v {
				return
			}
			if err != nil {
				v.err = "Could not introspect the data source: " + err.Error()
			} else {
				v.err = ""
				v.actionPrompt = &dbActionPrompt{
					kind: "introspection", title: "Database Catalog",
					source: t.Source, value: strings.Join(dbNames.Names, "\n"),
				}
				if catalog != nil {
					v.actionPrompt.sql = fmt.Sprintf("%s retained", count(len(catalog.Databases), "database", "databases"))
				}
			}
			m.invalidate()
		})
	}()
}

func (m *Model) DBDisconnectAction(arg any) {
	t := dbTargetFrom(m, arg)
	v := m.dbEnsureOpen()
	if t.Source == "" {
		for id := range v.connected {
			v := id
			m.DisconnectDBSource(v)
		}
		return
	}
	m.DisconnectDBSource(t.Source)
	if t.Database != "" {
		m.DisconnectDBSource(t.Source + ":" + t.Database)
	}
}

func (m *Model) DBOpenConsole(arg any) {
	t := dbTargetFrom(m, arg)
	v := m.dbEnsureOpen()
	if t.Source == "" {
		if len(v.sources) == 1 {
			t.Source = v.sources[0].ID
		}
	}
	if t.Source == "" {
		return
	}
	m.SelectDBSource(t.Source)
	target := v.activeTarget()
	if t.Database != "" && !strings.Contains(target, ":") {
		target = t.Source + ":" + t.Database
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		path, err := m.mgr.DBConsolePath(ctx, Window, target)
		m.post(func() {
			if err != nil {
				if m.db != nil {
					m.db.err = "Could not open the query console: " + err.Error()
					m.invalidate()
				}
				return
			}
			if m.db != nil {
				if m.db.consoleSources == nil {
					m.db.consoleSources = map[string]string{}
				}
				m.db.consoleSources[path] = target
			}
			m.openFile(path)
		})
	}()
}

func (m *Model) DBExecuteAction() {
	if tab := m.tab(); tab != nil && dbSQLPath(tab.doc.Path()) {
		m.ExecuteDBSQLTab(tab, false)
		return
	}
	m.dbEnsureOpen()
	m.RunDB()
}

func (m *Model) DBExecuteScriptAction() {
	if tab := m.tab(); tab != nil && dbSQLPath(tab.doc.Path()) {
		m.ExecuteDBSQLTab(tab, true)
		return
	}
	m.dbEnsureOpen()
	m.RunDB()
}

func dbSQLPath(path string) bool {
	extension := strings.ToLower(filepath.Ext(path))
	return extension == ".sql" || extension == ".ddl"
}

// ExecuteDBSQLTab runs the selection or statement at the caret of a SQL
// editor, or its whole script, on the console's data source.
func (m *Model) ExecuteDBSQLTab(tab *Tab, whole bool) {
	if tab == nil || tab.doc == nil || tab.state == nil {
		return
	}
	v := m.dbEnsureOpen()
	path := tab.doc.Path()
	target := v.consoleSources[path]
	if target == "" {
		if len(v.sources) == 1 {
			target = v.sources[0].ID
		} else if v.activeTarget() != "" {
			target = v.activeTarget()
		}
	}
	if target == "" {
		v.err = "Choose a data source for this console"
		m.invalidate()
		return
	}
	text := tab.doc.Text()
	from, to := uint32(0), utf16Len(text)
	if !whole {
		selection := tab.state.Selection()
		start := tab.doc.UTF16Pos(selection.Start())
		end := tab.doc.UTF16Pos(selection.End())
		from = start.Col + dbLineOffset(tab, int(start.Line))
		to = end.Col + dbLineOffset(tab, int(end.Line))
		if selection.Start() == selection.End() {
			to = 0
		}
	}
	if v.running {
		return
	}
	v.open = true
	v.source = dbBaseTarget(target)
	v.databaseTarget = target
	v.running = true
	v.err = ""
	v.outLog = append(v.outLog, "Run: "+oneLine(text))
	request := m.nextRequest()
	schema := v.schema
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 120*time.Second)
		defer cancel()
		statements, err := m.mgr.DBStatements(ctx, Window, target, text, from, to)
		var outcomes []openidea.DBOutcome
		if err == nil && len(statements) > 0 {
			outcomes, err = m.mgr.DBExecute(ctx, Window, request, target, statements, dbMaxRows, schema)
		}
		m.post(func() {
			if m.db != v {
				return
			}
			v.running = false
			title := filepath.Base(path)
			switch {
			case err != nil:
				v.addDBResult(title, target, nil, "Execution failed: "+err.Error())
				v.outLog = append(v.outLog, "Execution failed: "+err.Error())
			case len(statements) == 0:
				v.addDBResult(title, target, nil, "There is no statement to execute")
			default:
				v.addDBResult(title, target, outcomes, "")
				for _, outcome := range outcomes {
					v.outLog = append(v.outLog, oneLine(outcome.Statement.Text)+" · "+dbOutcomeSummary(outcome))
					if outcome.Error != "" {
						v.outLog = append(v.outLog, outcome.Error)
					}
				}
				if v.manualCommit[target] {
					for _, outcome := range outcomes {
						if outcome.Error == "" && !dbReadOnlyStatement(outcome.Statement.Text) {
							v.pendingTx[target] = true
							break
						}
					}
				}
			}
			if len(v.outLog) > 200 {
				v.outLog = v.outLog[len(v.outLog)-200:]
			}
			m.invalidate()
		})
	}()
}

func dbLineOffset(tab *Tab, line int) uint32 {
	if tab == nil || line <= 0 {
		return 0
	}
	text := tab.doc.Text()
	offset := uint32(0)
	current := 0
	for _, r := range text {
		if current >= line {
			break
		}
		if r == '\n' {
			current++
		}
		if r > 0xffff {
			offset += 2
		} else {
			offset++
		}
	}
	return offset
}

func (m *Model) DBExplainAction() {
	m.dbEnsureOpen()
	m.ExplainDB()
}

func (m *Model) DBHistoryAction() {
	m.dbEnsureOpen()
	m.OpenDBHistory()
}

// ToggleDBManualCommit switches the selected connection between auto-commit
// and manual transactions.
func (m *Model) ToggleDBManualCommit() {
	v := m.dbEnsureOpen()
	target := v.activeTarget()
	if target == "" {
		return
	}
	manual := !v.manualCommit[target]
	m.SetDBManualCommit(manual)
}

// SetDBManualCommit changes the transaction mode and reports a backend error
// in the database sheet.
func (m *Model) SetDBManualCommit(manual bool) {
	v := m.dbEnsureOpen()
	source := v.activeTarget()
	if source == "" || v.manualCommit[source] == manual {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		err := m.mgr.DBSetManualCommit(ctx, Window, source, manual)
		m.post(func() {
			if m.db != v {
				return
			}
			if err != nil {
				v.err = "Could not change transaction mode: " + err.Error()
			} else {
				v.manualCommit[source] = manual
				if !manual {
					delete(v.pendingTx, source)
				}
				v.outLog = append(v.outLog, fmt.Sprintf("Transaction control: %s", map[bool]string{true: "Manual", false: "Auto"}[manual]))
			}
			m.invalidate()
		})
	}()
}

// FinishDBTransaction commits or rolls back the selected source's open manual
// transaction.
func (m *Model) FinishDBTransaction(commit bool) {
	v := m.dbEnsureOpen()
	source := v.activeTarget()
	if source == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
		defer cancel()
		err := m.mgr.DBFinishTransaction(ctx, Window, source, commit)
		m.post(func() {
			if m.db != v {
				return
			}
			if err != nil {
				v.err = "Could not finish the transaction: " + err.Error()
			} else {
				delete(v.pendingTx, source)
				word := "rolled back"
				if commit {
					word = "committed"
				}
				v.outLog = append(v.outLog, "Transaction "+word)
			}
			m.invalidate()
		})
	}()
}

func (m *Model) DBEditData(arg any) {
	t := dbTargetFrom(m, arg)
	v := m.dbEnsureOpen()
	if t.Table == "" {
		return
	}
	v.selectedTable = openidea.DBTable{Name: t.Table, Kind: t.Kind}
	m.OpenDBTable(t.Schema, t.Table)
}

func (m *Model) dbLoadDefinition(t dbActionTarget) (string, error) {
	ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
	defer cancel()
	if t.ObjectKind != "" && t.ObjectName != "" {
		return m.mgr.DBObjectDDL(ctx, Window, t.Source, t.Schema, t.ObjectKind, t.ObjectName)
	}
	if t.Table == "" {
		return "", fmt.Errorf("choose a table, view or schema object")
	}
	return m.mgr.DBTableDDL(ctx, Window, t.Source, t.Schema, t.Table)
}

func (m *Model) DBGoToDDL(arg any) {
	t := dbTargetFrom(m, arg)
	v := m.dbEnsureOpen()
	go func() {
		ddl, err := m.dbLoadDefinition(t)
		m.post(func() {
			if m.db != v {
				return
			}
			if err != nil {
				v.err = "Could not read the DDL: " + err.Error()
				m.invalidate()
				return
			}
			title := t.ObjectName
			if title == "" {
				title = t.Table
			}
			v.actionPrompt = &dbActionPrompt{
				kind: "ddl", title: "DDL: " + title, source: t.Source,
				schema: t.Schema, table: t.Table, objectKind: t.ObjectKind,
				objectName: t.ObjectName, sql: ddl,
			}
			m.invalidate()
		})
	}()
}

func (m *Model) DBQuickDoc(arg any) {
	t := dbTargetFrom(m, arg)
	v := m.dbEnsureOpen()
	lines := []string{}
	if t.Source != "" {
		if source, ok := m.dbSourceByID(t.Source); ok {
			lines = append(lines, "Data source: "+source.Name, "Driver: "+source.Driver)
			if source.Host != "" {
				lines = append(lines, "Host: "+source.Host)
			}
			if source.Database != "" {
				lines = append(lines, "Database: "+source.Database)
			}
		}
	}
	if t.Database != "" {
		lines = append(lines, "Database: "+t.Database)
	}
	if t.Schema != "" {
		lines = append(lines, "Schema: "+t.Schema)
	}
	if t.Table != "" {
		lines = append(lines, "Table: "+t.Table, "Kind: "+t.Kind)
		if m.dbTable != nil && m.dbTable.table == t.Table {
			lines = append(lines, dbTableDetailLines(m.dbTable.page.Details)...)
		}
	}
	if len(lines) == 0 {
		lines = append(lines, "Select a database object")
	}
	v.actionPrompt = &dbActionPrompt{kind: "quickDoc", title: "Quick Documentation", value: strings.Join(lines, "\n")}
	m.invalidate()
}

func (m *Model) DBRename(arg any) {
	t := dbTargetFrom(m, arg)
	m.dbEnsureOpen()
	title := "Rename"
	if t.Table != "" {
		title = "Rename Table"
	} else if t.ObjectName != "" {
		title = "Rename " + strings.Title(t.ObjectKind)
	} else if t.Source != "" {
		title = "Rename Data Source"
	}
	m.db.actionPrompt = &dbActionPrompt{
		kind: "rename", title: title, value: dbDisplayTargetName(t),
		source: t.Source, schema: t.Schema, table: t.Table,
		objectKind: t.ObjectKind, objectName: t.ObjectName, arguments: t.Arguments,
	}
	m.invalidate()
}

func dbDisplayTargetName(t dbActionTarget) string {
	if t.Source != "" && t.Table == "" && t.ObjectName == "" {
		return t.Source
	}
	if t.ObjectName != "" {
		return t.ObjectName
	}
	return t.Table
}

func (m *Model) DBDrop(arg any) {
	t := dbTargetFrom(m, arg)
	m.dbEnsureOpen()
	title := "Confirm Drop"
	if t.Source != "" && t.Table == "" && t.ObjectName == "" {
		title = "Remove Data Source"
	}
	m.db.actionPrompt = &dbActionPrompt{
		kind: "drop", title: title, value: dbDisplayTargetName(t),
		source: t.Source, schema: t.Schema, table: t.Table,
		objectKind: t.ObjectKind, objectName: t.ObjectName, arguments: t.Arguments,
	}
	m.invalidate()
}

func (m *Model) DBTruncate(arg any) {
	t := dbTargetFrom(m, arg)
	m.dbEnsureOpen()
	m.db.actionPrompt = &dbActionPrompt{
		kind: "truncate", title: "Truncate", value: t.Table,
		source: t.Source, schema: t.Schema, table: t.Table,
	}
	m.invalidate()
}

func (m *Model) DBCopyReference(arg any) {
	t := dbTargetFrom(m, arg)
	text := m.dbCopyReferenceTarget(t)
	if text != "" {
		mygo.Clipboard.WriteText(text)
	}
}

func (m *Model) DBDuplicateSource(arg any) {
	t := dbTargetFrom(m, arg)
	v := m.dbEnsureOpen()
	if t.Source == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		sources, err := m.mgr.DBSourcesRaw(ctx, Window)
		if err != nil {
			m.post(func() {
				if m.db == v {
					m.db.err = "Could not duplicate the data source: " + err.Error()
					m.invalidate()
				}
			})
			return
		}
		index := -1
		for i, source := range sources {
			if fmt.Sprint(source["id"]) == t.Source {
				index = i
				break
			}
		}
		if index < 0 {
			return
		}
		copySource := dbCloneMap(sources[index])
		copySource["id"] = dbNewSourceID()
		base := fmt.Sprint(copySource["name"])
		if base == "<nil>" || strings.TrimSpace(base) == "" {
			base = "Data Source"
		}
		names := map[string]bool{}
		for _, source := range sources {
			names[fmt.Sprint(source["name"])] = true
		}
		n := 2
		name := ""
		for {
			name = fmt.Sprintf("%s [%d]", base, n)
			if !names[name] {
				break
			}
			n++
		}
		copySource["name"] = name
		delete(copySource, "pluginOwner")
		delete(copySource, "pluginSourceId")
		err = m.mgr.DBSaveSourcesRaw(ctx, Window, append(sources, copySource))
		m.post(func() {
			if m.db != v {
				return
			}
			if err != nil {
				v.err = "Could not duplicate the data source: " + err.Error()
			}
			m.RefreshDBSources()
			m.invalidate()
		})
	}()
}

func (m *Model) DBDDLToClipboard(arg any) {
	t := dbTargetFrom(m, arg)
	go func() {
		ddl, err := m.dbLoadDefinition(t)
		m.post(func() {
			if err != nil {
				if m.db != nil {
					m.db.err = "Could not generate the DDL: " + err.Error()
					m.invalidate()
				}
				return
			}
			mygo.Clipboard.WriteText(ddl)
		})
	}()
}

func (m *Model) DBDDLToConsole(arg any) {
	t := dbTargetFrom(m, arg)
	go func() {
		ddl, err := m.dbLoadDefinition(t)
		m.post(func() {
			if err != nil {
				if m.db != nil {
					m.db.err = "Could not generate the DDL: " + err.Error()
					m.invalidate()
				}
				return
			}
			m.DBPutSQLTemplate(t, ddl)
		})
	}()
}

func (m *Model) DBPutSQLTemplate(t dbActionTarget, sql string) {
	v := m.dbEnsureOpen()
	if t.Source != "" {
		v.source = t.Source
	}
	if t.Schema != "" {
		v.schema = t.Schema
	}
	v.sql = sql
	m.DBOpenConsole(map[string]any{"source": t.Source})
	m.invalidate()
}

func (m *Model) DBRunSQLScript(arg any) {
	t := dbTargetFrom(m, arg)
	v := m.dbEnsureOpen()
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Title: "Run SQL Script",
			Filters: []mygo.FileFilter{
				{Name: "SQL", Extensions: []string{"sql", "ddl"}},
			},
		})
		if err != nil || len(paths) == 0 {
			return
		}
		raw, err := os.ReadFile(paths[0])
		if err == nil {
			m.post(func() {
				if m.db != v {
					return
				}
				if t.Source != "" {
					v.source = t.Source
				}
				if t.Schema != "" {
					v.schema = t.Schema
				}
				v.sql = string(raw)
				m.RunDB()
			})
		}
	}()
}

func (m *Model) DBSQLScript(arg any) {
	t := dbTargetFrom(m, arg)
	kind := "selectAll"
	if values, ok := arg.(map[string]any); ok {
		if value, ok := values["script"].(string); ok && value != "" {
			kind = value
		}
	}
	columns := []openidea.DBColumnInfo{}
	if m.dbTable != nil {
		columns = append(columns, m.dbTable.page.Details.Columns...)
	}
	m.DBPutSQLTemplate(t, m.dbScriptSQL(t.Source, kind, t.Schema, t.Table, columns))
}

func (m *Model) DBGoToReferencedTable(arg any) {
	t := dbTargetFrom(m, arg)
	if t.Schema == "" || t.Table == "" {
		return
	}
	m.dbEnsureOpen()
	m.loadDBTables(t.Source, t.Schema)
	m.db.selectedTable = openidea.DBTable{Name: t.Table, Kind: "table"}
	m.invalidate()
}

func (m *Model) DBSeparateObjects() {
	v := m.dbEnsureOpen()
	v.separateProcedures = !v.separateProcedures
	m.invalidate()
}

func (m *Model) DBCollapseAll() {
	v := m.dbEnsureOpen()
	v.collapsed = true
	v.selectedTable = openidea.DBTable{}
	m.invalidate()
}

func dbResultIndex(arg any, fallback int) int {
	if values, ok := arg.(map[string]any); ok {
		for _, key := range []string{"index", "result", "id"} {
			switch value := values[key].(type) {
			case int:
				return value
			case float64:
				return int(value)
			case string:
				if n, err := strconv.Atoi(value); err == nil {
					return n
				}
			}
		}
	}
	switch value := arg.(type) {
	case int:
		return value
	case float64:
		return int(value)
	}
	return fallback
}

func (m *Model) DBCloseResultAction(arg any) {
	if m.db == nil {
		return
	}
	m.CloseDBResult(dbResultIndex(arg, m.db.active))
}

func (m *Model) DBCloseOtherResultsAction(arg any) {
	if m.db == nil {
		return
	}
	m.CloseOtherDBResults(dbResultIndex(arg, m.db.active))
}

func (m *Model) DBCloseAllResultsAction() {
	if m.db == nil {
		return
	}
	m.db.results = nil
	m.selectDBResult(-1)
}

func (m *Model) DBRenameResultAction(arg any) {
	v := m.dbEnsureOpen()
	index := dbResultIndex(arg, v.active)
	if index < 0 || index >= len(v.results) {
		return
	}
	v.actionPrompt = &dbActionPrompt{
		kind: "resultName", title: "Rename Result Tab", value: v.results[index].title, resultIndex: index,
	}
	m.invalidate()
}

func (m *Model) dbTableActionTarget() (dbTableSheet, dbActionTarget, bool) {
	if m.dbTable == nil {
		return dbTableSheet{}, dbActionTarget{}, false
	}
	t := dbActionTarget{
		Source: m.dbTable.source, Schema: m.dbTable.schema, Table: m.dbTable.table,
	}
	if len(m.dbTable.page.Result.Columns) > 0 {
		col := m.dbTable.selectedCol
		if col < 0 {
			col = 0
		}
		if col >= len(m.dbTable.page.Result.Columns) {
			col = len(m.dbTable.page.Result.Columns) - 1
		}
		t.Column = m.dbTable.page.Result.Columns[col].Name
	}
	return *m.dbTable, t, true
}

func (m *Model) dbSetSelectedCell(value *string) {
	s := m.dbTable
	if s == nil || !dbTableEditableCell(s, s.selectedRow, s.selectedCol) {
		return
	}
	dbTableSetCellValue(s, s.selectedRow, s.selectedCol, value)
	m.invalidate()
}

func (m *Model) DBSetCellNull() {
	if s := m.dbTable; s != nil {
		dbTableSetCellNull(s)
		m.invalidate()
	}
}

func (m *Model) DBSetCellDefault() {
	if s := m.dbTable; s != nil {
		dbTableSetCellDefault(s)
		m.invalidate()
	}
}

func (m *Model) DBToggleBooleanCell() {
	if s := m.dbTable; s != nil {
		dbTableToggleBoolean(s)
		m.invalidate()
	}
}

func (m *Model) dbSelectedValue() (string, bool) {
	return dbTableSelectedValue(m.dbTable)
}

func (m *Model) DBOpenValueWindow() {
	s := m.dbTable
	if s == nil {
		return
	}
	text, _ := m.dbSelectedValue()
	s.valueText = text
	s.valueOpen = true
	s.valueFind = ""
	editable, _ := dbTableEditability(s)
	s.valueRO = !editable || !dbTableEditableCell(s, s.selectedRow, s.selectedCol)
	if s.editing {
		dbTableCancelCellEdit(s)
	}
	m.invalidate()
}

func (m *Model) DBFindInValue() {
	s := m.dbTable
	if s == nil || !s.valueOpen {
		return
	}
	s.valueFind = ""
	m.invalidate()
}

func (m *Model) DBReformatValue() {
	s := m.dbTable
	if s == nil {
		return
	}
	text, ok := m.dbSelectedValue()
	if !ok {
		return
	}
	var value any
	if json.Unmarshal([]byte(text), &value) == nil {
		if formatted, err := json.MarshalIndent(value, "", "  "); err == nil {
			text = string(formatted)
		}
	}
	if s.valueOpen {
		s.valueText = text
		m.invalidate()
		return
	}
	if !s.valueRO && dbTableEditableCell(s, s.selectedRow, s.selectedCol) {
		dbTableSetCellValue(s, s.selectedRow, s.selectedCol, &text)
	}
}

func (m *Model) DBGoToColumn() {
	s := m.dbTable
	if s == nil {
		return
	}
	s.showDetails = true
	s.showDDL = false
	m.invalidate()
}

func (m *Model) DBSubmitChanges() {
	s := m.dbTable
	if s == nil || s.submitting {
		return
	}
	editable, reason := dbTableEditability(s)
	if !editable {
		s.err = reason
		m.invalidate()
		return
	}
	if s.editing {
		dbTableCommitCellEdit(s)
	}
	edits, err := dbBuildRowEdits(s)
	if err != nil {
		s.err = err.Error()
		m.invalidate()
		return
	}
	if len(edits) == 0 {
		return
	}
	s.submitting = true
	source, schema, table := s.source, s.schema, s.table
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
		defer cancel()
		changed, err := m.mgr.DBApplyEdits(ctx, Window, source, schema, table, edits)
		m.post(func() {
			if m.dbTable != s {
				return
			}
			s.submitting = false
			if err != nil {
				s.err = "Could not submit changes: " + err.Error()
			} else {
				dbTableDiscardChanges(s)
				if changed != nil {
					if m.db != nil {
						m.db.outLog = append(m.db.outLog, fmt.Sprintf("%s changed", count(int(*changed), "row", "rows")))
					}
				}
				m.loadDBTablePage()
			}
			m.invalidate()
		})
	}()
}

func (m *Model) DBRevertChanges() {
	s := m.dbTable
	if s == nil {
		return
	}
	dbTableDiscardChanges(s)
	m.invalidate()
}

func dbBuildRowEdits(s *dbTableSheet) ([]openidea.DBRowEdit, error) {
	if s == nil || len(s.page.Result.Columns) == 0 {
		return nil, nil
	}
	if editable, reason := dbTableEditability(s); !editable && dbTableHasChanges(s) {
		return nil, fmt.Errorf("%s", reason)
	}
	columns := make([]string, len(s.page.Result.Columns))
	for i, column := range s.page.Result.Columns {
		columns[i] = column.Name
	}
	pk := s.page.Details.Columns
	keyColumns := []string{}
	for _, column := range pk {
		if column.PrimaryKey {
			keyColumns = append(keyColumns, column.Name)
		}
	}
	keyOf := func(row int) ([]openidea.DBCellValue, error) {
		if len(keyColumns) == 0 {
			return nil, fmt.Errorf("this table has no primary key; editing its rows is unsafe")
		}
		values := make([]openidea.DBCellValue, 0, len(keyColumns))
		for _, name := range keyColumns {
			index := -1
			for i, column := range columns {
				if column == name {
					index = i
					break
				}
			}
			if index < 0 || row < 0 || row >= len(s.page.Result.Rows) {
				return nil, fmt.Errorf("primary key %s is not on this result page", name)
			}
			var value *string
			if index < len(s.page.Result.Rows[row]) {
				value = s.page.Result.Rows[row][index]
			}
			values = append(values, openidea.DBCellValue{Column: name, Value: value})
		}
		return values, nil
	}
	byRow := map[int]map[string]*string{}
	for key, value := range s.pending {
		row, col, ok := dbTableParseCellKey(key)
		if !ok || col < 0 || col >= len(columns) {
			continue
		}
		if byRow[row] == nil {
			byRow[row] = map[string]*string{}
		}
		byRow[row][columns[col]] = value
	}
	rows := make([]int, 0, len(byRow))
	for row := range byRow {
		rows = append(rows, row)
	}
	sort.Ints(rows)
	out := []openidea.DBRowEdit{}
	for _, row := range rows {
		values := byRow[row]
		if s.deletes[row] || len(values) == 0 {
			continue
		}
		key, err := keyOf(row)
		if err != nil {
			return nil, err
		}
		editValues := make([]openidea.DBCellValue, 0, len(values))
		for _, column := range columns {
			if value, ok := values[column]; ok {
				editValues = append(editValues, openidea.DBCellValue{Column: column, Value: value})
			}
		}
		out = append(out, openidea.DBRowEdit{Kind: "update", Key: key, Values: editValues})
	}
	deletes := make([]int, 0, len(s.deletes))
	for row := range s.deletes {
		deletes = append(deletes, row)
	}
	sort.Ints(deletes)
	for _, row := range deletes {
		key, err := keyOf(row)
		if err != nil {
			return nil, err
		}
		out = append(out, openidea.DBRowEdit{Kind: "delete", Key: key})
	}
	for _, values := range s.inserts {
		editValues := make([]openidea.DBCellValue, 0, len(values))
		for _, column := range columns {
			if value, ok := values[column]; ok {
				editValues = append(editValues, openidea.DBCellValue{Column: column, Value: value})
			}
		}
		out = append(out, openidea.DBRowEdit{Kind: "insert", Values: editValues})
	}
	return out, nil
}

func dbParseDelimited(text, extension string) ([][]string, error) {
	comma := ','
	if strings.EqualFold(extension, ".tsv") {
		comma = '\t'
	} else if !strings.Contains(text, ",") {
		first := strings.SplitN(text, "\n", 2)[0]
		if strings.Contains(first, "\t") {
			comma = '\t'
		} else if strings.Contains(first, ";") {
			comma = ';'
		}
	}
	return dbParseDelimitedSeparator(text, comma)
}

func dbParseDelimitedSeparator(text string, comma rune) ([][]string, error) {
	reader := csv.NewReader(strings.NewReader(text))
	reader.Comma = comma
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("Could not parse the import file: %w", err)
	}
	return rows, nil
}

func dbImportEdits(rows [][]string, columns []string, header, emptyAsNull bool) ([]openidea.DBRowEdit, []string) {
	if len(rows) == 0 {
		return nil, nil
	}
	target := func(name string) string {
		for _, column := range columns {
			if strings.EqualFold(strings.TrimSpace(name), column) {
				return column
			}
		}
		return ""
	}
	mapping := []string{}
	if header {
		for _, name := range rows[0] {
			mapping = append(mapping, target(name))
		}
		rows = rows[1:]
	} else {
		for i := 0; i < len(columns); i++ {
			width := 0
			for _, row := range rows {
				if len(row) > width {
					width = len(row)
				}
			}
			if i < width {
				mapping = append(mapping, columns[i])
			}
		}
	}
	mapped := []string{}
	for _, name := range mapping {
		if name != "" {
			mapped = append(mapped, name)
		}
	}
	edits := []openidea.DBRowEdit{}
	for _, row := range rows {
		if len(row) == 0 {
			continue
		}
		values := []openidea.DBCellValue{}
		for i, column := range mapping {
			if column == "" || i >= len(row) {
				continue
			}
			var value *string
			text := row[i]
			if !emptyAsNull || text != "" {
				value = &text
			}
			values = append(values, openidea.DBCellValue{Column: column, Value: value})
		}
		if len(values) > 0 {
			edits = append(edits, openidea.DBRowEdit{Kind: "insert", Values: values})
		}
	}
	return edits, mapped
}

func dbCloneMap(source map[string]any) map[string]any {
	raw, _ := json.Marshal(source)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}

func dbNewSourceID() string {
	return fmt.Sprintf("db-%d-%d", time.Now().UnixNano(), os.Getpid())
}

func (m *Model) dbDriver(source string) string {
	if m != nil && m.db != nil {
		for _, s := range m.db.sources {
			if s.ID == source || strings.HasPrefix(source, s.ID+":") {
				return strings.ToLower(s.Driver)
			}
		}
	}
	return ""
}

func dbQuoteIdent(driver, value string) string {
	if value == "" {
		return ""
	}
	plain := false
	switch driver {
	case "postgres":
		plain = dbPlainIdent(value, false)
	case "oracle", "snowflake":
		plain = dbPlainIdent(value, true)
	default:
		plain = dbPlainIdent(value, false)
	}
	if plain && !dbReservedIdent(value) {
		return value
	}
	switch driver {
	case "clickhouse":
		return `"` + strings.ReplaceAll(strings.ReplaceAll(value, `\`, `\\`), `"`, `\"`) + `"`
	case "mysql":
		return "`" + strings.ReplaceAll(value, "`", "``") + "`"
	case "sqlserver":
		return "[" + strings.ReplaceAll(value, "]", "]]") + "]"
	default:
		return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
	}
}

func dbPlainIdent(value string, upper bool) bool {
	if value == "" {
		return false
	}
	for i, r := range value {
		firstLetter := r >= 'a' && r <= 'z'
		if upper {
			firstLetter = r >= 'A' && r <= 'Z'
		} else if r >= 'A' && r <= 'Z' && i == 0 {
			return false
		}
		if i == 0 {
			if !(r == '_' || firstLetter) {
				return false
			}
			continue
		}
		if !(r == '_' || r == '$' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			return false
		}
	}
	return true
}

func dbReservedIdent(value string) bool {
	switch strings.ToUpper(value) {
	case "ALL", "AND", "AS", "ASC", "BETWEEN", "BY", "CASE", "CHECK",
		"COLUMN", "CONSTRAINT", "CREATE", "CROSS", "DEFAULT", "DELETE",
		"DESC", "DISTINCT", "DROP", "ELSE", "END", "EXISTS", "FOREIGN",
		"FROM", "FULL", "GROUP", "HAVING", "IN", "INDEX", "INNER",
		"INSERT", "INTO", "IS", "JOIN", "KEY", "LEFT", "LIKE", "LIMIT",
		"NOT", "NULL", "OFFSET", "ON", "OR", "ORDER", "OUTER", "PRIMARY",
		"REFERENCES", "RIGHT", "SELECT", "SET", "TABLE", "THEN", "TO",
		"UNION", "UNIQUE", "UPDATE", "USER", "USING", "VALUES", "VIEW",
		"WHEN", "WHERE", "WITH":
		return true
	}
	return false
}

func (m *Model) dbQualifiedName(source, schema, table string) string {
	driver := m.dbDriver(source)
	parts := []string{}
	if schema != "" && !(driver == "sqlite" && schema == "main") {
		parts = append(parts, dbQuoteIdent(driver, schema))
	}
	if table != "" {
		parts = append(parts, dbQuoteIdent(driver, table))
	}
	return strings.Join(parts, ".")
}

func (m *Model) dbCopyReference(source, schema, table, object string) string {
	name := table
	if object != "" {
		name = object
	}
	if name == "" {
		return ""
	}
	return m.dbQualifiedName(source, schema, name)
}

func (m *Model) dbCopyReferenceTarget(target dbActionTarget) string {
	driver := m.dbDriver(target.Source)
	kind := strings.ToLower(target.ObjectKind)
	if kind == "schema" {
		name := target.ObjectName
		if name == "" {
			name = target.Schema
		}
		return dbQuoteIdent(driver, name)
	}
	if kind == "column" {
		column := target.Column
		if column == "" {
			column = target.ObjectName
		}
		return m.dbQualifiedName(target.Source, target.Schema, target.Table) + "." +
			dbQuoteIdent(driver, column)
	}
	return m.dbCopyReference(target.Source, target.Schema, target.Table, target.ObjectName)
}

func (m *Model) dbScriptSQL(source, kind, schema, table string, columns []openidea.DBColumnInfo) string {
	driver := m.dbDriver(source)
	q := m.dbQualifiedName(source, schema, table)
	quoted := make([]string, 0, len(columns))
	keys := make([]string, 0, len(columns))
	for _, column := range columns {
		name := dbQuoteIdent(driver, column.Name)
		quoted = append(quoted, name)
		if column.PrimaryKey {
			keys = append(keys, name)
		}
	}
	where := keys
	if len(where) == 0 && len(quoted) > 0 {
		where = quoted[:1]
	}
	switch kind {
	case "selectFirst":
		if driver == "oracle" {
			return fmt.Sprintf("select *\nfrom %s\nfetch first 10 rows only;\n", q)
		}
		return fmt.Sprintf("select *\nfrom %s\nlimit 10;\n", q)
	case "insert":
		placeholders := make([]string, len(quoted))
		for i := range placeholders {
			placeholders[i] = "?"
		}
		return fmt.Sprintf("insert into %s (%s)\nvalues (%s);\n",
			q, strings.Join(quoted, ", "), strings.Join(placeholders, ", "))
	case "update":
		if driver == "clickhouse" {
			set := make([]string, 0, len(columns))
			for i, column := range columns {
				if !column.PrimaryKey {
					set = append(set, quoted[i]+" = ?")
				}
			}
			if len(set) == 0 {
				return "ClickHouse sorting key columns cannot be updated."
			}
			return fmt.Sprintf("alter table %s\nupdate %s\nwhere 1 = 0;\n",
				q, strings.Join(set, ",\n       "))
		}
		set := make([]string, 0, len(quoted))
		for i, column := range columns {
			if !column.PrimaryKey {
				set = append(set, quoted[i]+" = ?")
			}
		}
		if len(set) == 0 {
			for _, name := range quoted {
				set = append(set, name+" = ?")
			}
		}
		return fmt.Sprintf("update %s\nset %s\nwhere %s;\n",
			q, strings.Join(set, ",\n    "), dbScriptWhere(where))
	case "delete":
		if driver == "clickhouse" {
			return fmt.Sprintf("alter table %s\ndelete\nwhere 1 = 0;\n", q)
		}
		return fmt.Sprintf("delete\nfrom %s\nwhere %s;\n", q, dbScriptWhere(where))
	default:
		return fmt.Sprintf("select *\nfrom %s;\n", q)
	}
}

func dbScriptWhere(columns []string) string {
	if len(columns) == 0 {
		return "1 = 0"
	}
	parts := make([]string, len(columns))
	for i, column := range columns {
		parts[i] = column + " = ?"
	}
	return strings.Join(parts, " and ")
}

// ConfirmDBActionPrompt applies the current rename, drop, truncate or tab-name
// prompt. Destructive statements run only after this explicit action.
func (m *Model) ConfirmDBActionPrompt() {
	if m.db == nil || m.db.actionPrompt == nil {
		return
	}
	prompt := m.db.actionPrompt
	if prompt.busy {
		return
	}
	switch prompt.kind {
	case "resultName":
		if prompt.resultIndex >= 0 && prompt.resultIndex < len(m.db.results) {
			title := strings.TrimSpace(prompt.value)
			if title != "" {
				m.db.results[prompt.resultIndex].title = title
			}
		}
		m.db.actionPrompt = nil
		m.invalidate()
		return
	case "rename":
		name := strings.TrimSpace(prompt.value)
		if name == "" {
			prompt.err = "Enter a name"
			m.invalidate()
			return
		}
		if prompt.table == "" && prompt.objectName == "" {
			m.renameDBDataSource(prompt, name)
			return
		}
		t := dbActionTarget{
			Source: prompt.source, Schema: prompt.schema, Table: prompt.table,
			ObjectKind: prompt.objectKind, ObjectName: prompt.objectName, Arguments: prompt.arguments,
		}
		sql := m.dbRenameSQL(t, name)
		m.dbExecuteActionSQL(prompt, []string{sql})
		return
	case "drop":
		if prompt.table == "" && prompt.objectName == "" {
			m.dropDBDataSource(prompt)
			return
		}
		t := dbActionTarget{
			Source: prompt.source, Schema: prompt.schema, Table: prompt.table,
			ObjectKind: prompt.objectKind, ObjectName: prompt.objectName, Arguments: prompt.arguments,
		}
		m.dbExecuteActionSQL(prompt, []string{m.dbDropSQL(t, dbDropOptions{
			IfExists: prompt.ifExists, Cascade: prompt.cascade,
		})})
		return
	case "truncate":
		t := dbActionTarget{Source: prompt.source, Schema: prompt.schema, Table: prompt.table}
		m.dbExecuteActionSQL(prompt, []string{m.dbTruncateSQL(
			t.Source, t.Schema, t.Table, prompt.restartID, prompt.cascade,
		)})
	default:
		m.db.actionPrompt = nil
		m.invalidate()
	}
}

func (m *Model) renameDBDataSource(prompt *dbActionPrompt, name string) {
	prompt.busy = true
	prompt.err = ""
	sourceID := prompt.source
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		sources, err := m.mgr.DBSourcesRaw(ctx, Window)
		if err == nil {
			found := false
			for i := range sources {
				if fmt.Sprint(sources[i]["id"]) == sourceID {
					sources[i]["name"] = name
					found = true
					break
				}
			}
			if !found {
				err = fmt.Errorf("the data source no longer exists")
			} else {
				err = m.mgr.DBSaveSourcesRaw(ctx, Window, sources)
			}
		}
		m.post(func() {
			if m.db == nil || m.db.actionPrompt != prompt {
				return
			}
			prompt.busy = false
			if err != nil {
				prompt.err = "Could not rename the data source: " + err.Error()
			} else {
				m.db.actionPrompt = nil
				m.RefreshDBSources()
			}
			m.invalidate()
		})
	}()
}

func (m *Model) dropDBDataSource(prompt *dbActionPrompt) {
	prompt.busy = true
	prompt.err = ""
	sourceID := prompt.source
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		_ = m.mgr.DBDisconnect(ctx, Window, sourceID)
		sources, err := m.mgr.DBSourcesRaw(ctx, Window)
		if err == nil {
			next := make([]map[string]any, 0, len(sources))
			for _, source := range sources {
				if fmt.Sprint(source["id"]) != sourceID {
					next = append(next, source)
				}
			}
			err = m.mgr.DBSaveSourcesRaw(ctx, Window, next)
		}
		m.post(func() {
			if m.db == nil || m.db.actionPrompt != prompt {
				return
			}
			prompt.busy = false
			if err != nil {
				prompt.err = "Could not remove the data source: " + err.Error()
			} else {
				delete(m.db.connected, sourceID)
				if m.db.source == sourceID {
					m.db.source = ""
					m.db.schema = ""
					m.db.tables = nil
					m.db.selectedTable = openidea.DBTable{}
				}
				m.db.actionPrompt = nil
				m.RefreshDBSources()
			}
			m.invalidate()
		})
	}()
}

func (m *Model) dbRenameSQL(t dbActionTarget, name string) string {
	driver := m.dbDriver(t.Source)
	newName := dbQuoteIdent(driver, name)
	kind := strings.ToLower(t.ObjectKind)
	if kind == "" && t.Table != "" {
		kind = strings.ToLower(t.Kind)
		if kind == "" {
			kind = "table"
		}
	}
	if kind == "column" {
		column := t.Column
		if column == "" {
			column = t.ObjectName
		}
		return fmt.Sprintf("alter table %s rename column %s to %s;",
			m.dbQualifiedName(t.Source, t.Schema, t.Table), dbQuoteIdent(driver, column), newName)
	}
	if t.ObjectName != "" {
		oldName := m.dbQualifiedName(t.Source, t.Schema, t.ObjectName)
		switch kind {
		case "view":
			switch driver {
			case "snowflake":
				return fmt.Sprintf("alter view %s rename to %s;", oldName,
					m.dbQualifiedName(t.Source, t.Schema, name))
			case "postgres":
				return fmt.Sprintf("alter view %s rename to %s;", oldName, newName)
			case "oracle":
				return fmt.Sprintf("rename %s to %s;", oldName, newName)
			default:
				return fmt.Sprintf("rename table %s to %s;", oldName,
					m.dbQualifiedName(t.Source, t.Schema, name))
			}
		case "index":
			return fmt.Sprintf("alter index %s rename to %s;", oldName, newName)
		case "function", "procedure", "trigger", "sequence", "package":
			return fmt.Sprintf("alter %s %s rename to %s;", kind, oldName, newName)
		default:
			if driver == "mysql" || driver == "clickhouse" {
				return fmt.Sprintf("rename table %s to %s;", oldName,
					m.dbQualifiedName(t.Source, t.Schema, name))
			}
			if driver == "snowflake" {
				return fmt.Sprintf("alter table %s rename to %s;", oldName,
					m.dbQualifiedName(t.Source, t.Schema, name))
			}
			return fmt.Sprintf("alter table %s rename to %s;", oldName, newName)
		}
	}
	oldName := m.dbQualifiedName(t.Source, t.Schema, t.Table)
	if driver == "mysql" || driver == "clickhouse" {
		return fmt.Sprintf("rename table %s to %s;", oldName, m.dbQualifiedName(t.Source, t.Schema, name))
	}
	if driver == "snowflake" {
		return fmt.Sprintf("alter table %s rename to %s;", oldName, m.dbQualifiedName(t.Source, t.Schema, name))
	}
	return fmt.Sprintf("alter table %s rename to %s;", oldName, newName)
}

type dbDropOptions struct {
	IfExists bool
	Cascade  bool
}

func (m *Model) dbDropSQL(t dbActionTarget, opts dbDropOptions) string {
	driver := m.dbDriver(t.Source)
	kind := t.ObjectKind
	name := t.ObjectName
	if name == "" {
		name = t.Table
		switch strings.ToLower(t.Kind) {
		case "view":
			kind = "view"
		case "materializedview":
			kind = "materialized view"
		default:
			kind = "table"
		}
	}
	if kind == "" {
		kind = "table"
	}
	kind = strings.ToLower(kind)
	ifExists := ""
	if opts.IfExists {
		ifExists = "if exists "
	}
	cascade := ""
	if opts.Cascade && driver == "postgres" {
		cascade = " cascade"
	}
	switch kind {
	case "column":
		if driver != "postgres" {
			ifExists = ""
		}
		column := t.Column
		if column == "" {
			column = t.ObjectName
		}
		return fmt.Sprintf("alter table %s drop column %s%s%s;",
			m.dbQualifiedName(t.Source, t.Schema, t.Table), ifExists,
			dbQuoteIdent(driver, column), cascade)
	case "index":
		if driver == "mysql" {
			return fmt.Sprintf("drop index %s on %s;", dbQuoteIdent(driver, name),
				m.dbQualifiedName(t.Source, t.Schema, t.Table))
		}
		if driver == "oracle" {
			return fmt.Sprintf("drop index %s;", m.dbQualifiedName(t.Source, t.Schema, name))
		}
		return fmt.Sprintf("drop index %s%s%s;", ifExists,
			m.dbQualifiedName(t.Source, t.Schema, name), cascade)
	case "schema":
		if driver == "oracle" {
			return fmt.Sprintf("drop user %s cascade;", dbQuoteIdent(driver, name))
		}
		namespace := "schema"
		if driver == "mysql" || driver == "clickhouse" {
			namespace = "database"
		}
		return fmt.Sprintf("drop %s %s%s%s;", namespace, ifExists,
			dbQuoteIdent(driver, name), cascade)
	case "materializedview":
		if driver == "clickhouse" {
			return fmt.Sprintf("drop table %s%s;", ifExists,
				m.dbQualifiedName(t.Source, t.Schema, name))
		}
		return fmt.Sprintf("drop materialized view %s%s%s;", ifExists,
			m.dbQualifiedName(t.Source, t.Schema, name), cascade)
	case "function", "procedure":
		args := ""
		if driver == "postgres" || driver == "snowflake" {
			args = "(" + t.Arguments + ")"
		}
		return fmt.Sprintf("drop %s %s%s%s%s;", kind, ifExists,
			m.dbQualifiedName(t.Source, t.Schema, name), args, cascade)
	case "package":
		return fmt.Sprintf("drop package %s;", m.dbQualifiedName(t.Source, t.Schema, name))
	case "trigger":
		if driver == "postgres" {
			return fmt.Sprintf("drop trigger %s%s on %s%s;", ifExists,
				dbQuoteIdent(driver, name), m.dbQualifiedName(t.Source, t.Schema, t.Table), cascade)
		}
		return fmt.Sprintf("drop trigger %s%s%s;", ifExists,
			m.dbQualifiedName(t.Source, t.Schema, name), cascade)
	default:
		return fmt.Sprintf("drop %s %s%s%s;", kind, ifExists,
			m.dbQualifiedName(t.Source, t.Schema, name), cascade)
	}
}

func (m *Model) dbTruncateSQL(source, schema, table string, restartIdentity, cascade bool) string {
	driver := m.dbDriver(source)
	q := m.dbQualifiedName(source, schema, table)
	switch driver {
	case "sqlite":
		return fmt.Sprintf("delete from %s;", q)
	case "mysql", "sqlserver", "clickhouse", "oracle", "snowflake":
		return fmt.Sprintf("truncate table %s;", q)
	default:
		suffix := ""
		if restartIdentity {
			suffix += " restart identity"
		}
		if cascade {
			suffix += " cascade"
		}
		return fmt.Sprintf("truncate table %s%s;", q, suffix)
	}
}

func (m *Model) dbExecuteActionSQL(prompt *dbActionPrompt, texts []string) {
	if len(texts) == 0 {
		m.db.actionPrompt = nil
		m.invalidate()
		return
	}
	statements := make([]openidea.DBStatement, 0, len(texts))
	for _, text := range texts {
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		statements = append(statements, openidea.DBStatement{
			From: 0, To: utf16Len(text), Text: text,
		})
	}
	prompt.busy = true
	prompt.err = ""
	prompt.sql = strings.Join(texts, ";\n")
	source := prompt.source
	schema := prompt.schema
	request := m.nextRequest()
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 120*time.Second)
		defer cancel()
		outcomes, err := m.mgr.DBExecute(ctx, Window, request, source, statements, dbMaxRows, schema)
		m.post(func() {
			if m.db == nil || m.db.actionPrompt != prompt {
				return
			}
			prompt.busy = false
			if err != nil {
				prompt.err = err.Error()
				m.invalidate()
				return
			}
			for _, outcome := range outcomes {
				if outcome.Error != "" {
					prompt.err = outcome.Error
					m.invalidate()
					return
				}
			}
			m.db.addDBResult(prompt.title, source, outcomes, "")
			m.db.actionPrompt = nil
			m.RefreshDBSources()
			if schema != "" {
				m.loadDBTables(source, schema)
			}
			m.invalidate()
		})
	}()
}

func (m *Model) dbActionPromptDialog(c *ui.Context, p *ui.Theme) {
	if m.db == nil || m.db.actionPrompt == nil {
		return
	}
	prompt := m.db.actionPrompt
	open := true
	nativeModal(c, &open, func() {
		ui.Column(c).Width(680).Height(520).Gap(10).Children(func() {
			ui.Text(c, prompt.title).FontSize(13).Bold().TextColor(p.Text).SingleLine()
			if prompt.err != "" {
				ui.Text(c, prompt.err).FontSize(12).TextColor(p.Danger).SingleLine().Ellipsis("…")
			}
			switch prompt.kind {
			case "rename", "resultName":
				ui.Text(c, prompt.value).FontSize(12).TextColor(p.TextMuted).SingleLine()
				ui.TextArea(c, &prompt.value).Height(32).SingleLine().AutoFocus()
				if prompt.kind == "rename" && prompt.table != "" || prompt.kind == "rename" && prompt.objectName != "" {
					t := dbActionTarget{
						Source: prompt.source, Schema: prompt.schema, Table: prompt.table,
						ObjectKind: prompt.objectKind, ObjectName: prompt.objectName,
						Arguments: prompt.arguments,
					}
					preview := m.dbRenameSQL(t, strings.TrimSpace(prompt.value))
					ui.Text(c, "Preview").FontSize(12).TextColor(p.TextMuted).SingleLine()
					ui.TextArea(c, &preview).ReadOnly(true).Height(90).Font("monospace").FontSize(12)
				}
			case "drop":
				ui.Text(c, "This cannot be undone.").FontSize(12).TextColor(p.Danger).SingleLine()
				ui.Text(c, prompt.value).FontSize(13).TextColor(p.Text).SingleLine()
				if prompt.table != "" || prompt.objectName != "" {
					driver := m.dbDriver(prompt.source)
					if ui.Checkbox(c, &prompt.ifExists, "Use IF EXISTS syntax").
						Disabled(prompt.objectKind == "package" ||
							prompt.objectKind == "index" && (driver == "mysql" || driver == "oracle") ||
							prompt.objectKind == "column" && driver != "postgres").Changed() {
					}
					ui.Checkbox(c, &prompt.cascade, "Use DROP CASCADE syntax").Disabled(driver != "postgres")
					t := dbActionTarget{
						Source: prompt.source, Schema: prompt.schema, Table: prompt.table,
						ObjectKind: prompt.objectKind, ObjectName: prompt.objectName,
						Arguments: prompt.arguments,
					}
					preview := m.dbDropSQL(t, dbDropOptions{IfExists: prompt.ifExists, Cascade: prompt.cascade})
					ui.Text(c, "Preview").FontSize(12).TextColor(p.TextMuted).SingleLine()
					ui.TextArea(c, &preview).ReadOnly(true).Height(100).Font("monospace").FontSize(12)
				}
			case "truncate":
				ui.Text(c, "This removes every row and cannot be undone.").FontSize(12).TextColor(p.Danger).SingleLine()
				ui.Text(c, prompt.value).FontSize(13).TextColor(p.Text).SingleLine()
				if m.dbDriver(prompt.source) == "postgres" {
					ui.Checkbox(c, &prompt.restartID, "Restart identity")
					ui.Checkbox(c, &prompt.cascade, "Cascade")
				}
				preview := m.dbTruncateSQL(prompt.source, prompt.schema, prompt.table, prompt.restartID, prompt.cascade)
				ui.Text(c, "Preview").FontSize(12).TextColor(p.TextMuted).SingleLine()
				ui.TextArea(c, &preview).ReadOnly(true).Height(90).Font("monospace").FontSize(12)
			default:
				ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
					text := prompt.sql
					if prompt.kind == "introspection" {
						text = prompt.value
						if prompt.sql != "" {
							text += "\n\n" + prompt.sql
						}
					}
					for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
						ui.Text(c, line).FontSize(12).Font("monospace").TextColor(p.Text).SingleLine()
					}
				})
			}
			ui.Row(c).Gap(8).Children(func() {
				if prompt.kind == "ddl" {
					ui.Button(c, "Copy").OnClick(func() { mygo.Clipboard.WriteText(prompt.sql) })
					ui.Button(c, "Use in Console").OnClick(func() {
						t := dbActionTarget{
							Source: prompt.source, Schema: prompt.schema, Table: prompt.table,
							ObjectKind: prompt.objectKind, ObjectName: prompt.objectName,
						}
						m.db.actionPrompt = nil
						m.DBPutSQLTemplate(t, prompt.sql)
					})
				}
				ui.Spacer(c)
				if prompt.kind == "rename" || prompt.kind == "resultName" || prompt.kind == "drop" || prompt.kind == "truncate" {
					ui.Button(c, "Cancel").OnClick(func() { m.db.actionPrompt = nil; m.invalidate() })
					label := "OK"
					if prompt.kind == "drop" {
						label = "OK"
					} else if prompt.kind == "truncate" {
						label = "OK"
					} else if prompt.kind == "resultName" {
						label = "Rename"
					} else if prompt.kind == "rename" {
						label = "Refactor"
					}
					ui.Button(c, label).Disabled(prompt.busy).OnClick(func() { m.ConfirmDBActionPrompt() })
				} else {
					ui.Button(c, "Close").OnClick(func() { m.db.actionPrompt = nil; m.invalidate() })
				}
			})
		})
	})
	if !open && m.db != nil && m.db.actionPrompt == prompt {
		m.db.actionPrompt = nil
		m.invalidate()
	}
}

func (m *Model) dbValueEditorDialog(c *ui.Context, p *ui.Theme) {
	s := m.dbTable
	if s == nil || !s.valueOpen {
		return
	}
	open := s.valueOpen
	nativeModal(c, &open, func() {
		ui.Column(c).Width(720).Height(520).Gap(8).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Value").FontSize(13).Bold().TextColor(p.Text).SingleLine()
				column := ""
				if s.selectedCol >= 0 && s.selectedCol < len(s.page.Result.Columns) {
					column = s.page.Result.Columns[s.selectedCol].Name
				}
				if column != "" {
					ui.Text(c, column).FontSize(12).TextColor(p.TextMuted).SingleLine()
				}
				if s.valueRO {
					ui.Text(c, "Read-only").FontSize(12).TextColor(p.TextMuted).SingleLine()
				}
				ui.Spacer(c)
				ui.Button(c, "Reformat").Disabled(s.valueRO).OnClick(func() { m.DBReformatValue() })
				ui.Button(c, "Copy").OnClick(func() { mygo.Clipboard.WriteText(s.valueText) })
			})
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Find").FontSize(12).TextColor(p.TextMuted).SingleLine()
				ui.TextArea(c, &s.valueFind).Height(28).SingleLine().Grow(1)
				if s.valueFind != "" {
					count := strings.Count(s.valueText, s.valueFind)
					ui.Text(c, fmt.Sprintf("%d matches", count)).FontSize(12).TextColor(p.TextMuted).SingleLine()
				}
			})
			ui.TextArea(c, &s.valueText).Grow(1).MinHeight(0).Font("monospace").FontSize(12).
				ReadOnly(s.valueRO)
			ui.Row(c).Gap(8).Children(func() {
				ui.Spacer(c)
				ui.Button(c, "Cancel").OnClick(func() { s.valueOpen = false; m.invalidate() })
				if s.valueRO {
					ui.Button(c, "Close").OnClick(func() { s.valueOpen = false; m.invalidate() })
					return
				}
				ui.Button(c, "Apply").OnClick(func() {
					value := s.valueText
					m.dbSetSelectedCell(&value)
					s.valueOpen = false
					m.invalidate()
				})
			})
		})
	})
	if !open && m.dbTable == s {
		s.valueOpen = false
		m.invalidate()
	}
}
