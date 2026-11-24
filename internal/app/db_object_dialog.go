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

// dbColumnDraft is one row of OpenIDEA's Create Table and Modify Table forms.
// original is nil for an added column and identifies the catalog column for an
// existing one.
type dbColumnDraft struct {
	key          int
	original     *openidea.DBColumnInfo
	name         string
	dataType     string
	defaultValue string
	notNull      bool
	primaryKey   bool
	dropped      bool
}

type dbTableForm struct {
	open      bool
	creating  bool
	addColumn bool
	loading   bool
	busy      bool
	source    string
	schema    string
	table     string
	name      string
	columns   []dbColumnDraft
	selected  int
	err       string
	nextKey   int
}

type dbImportForm struct {
	open        bool
	loading     bool
	busy        bool
	source      string
	schema      string
	table       string
	file        string
	text        string
	separator   string
	header      bool
	emptyAsNull bool
	columns     []string
	rows        [][]string
	edits       []openidea.DBRowEdit
	mapped      []string
	err         string
}

func dbBlankColumn(driver string, key, number int) dbColumnDraft {
	types := dbTypeSuggestions(driver)
	dataType := "text"
	if len(types) > 0 {
		dataType = types[0]
	}
	column := dbColumnDraft{
		key:      key,
		name:     fmt.Sprintf("column_%d", number),
		dataType: dataType,
	}
	if number == 1 {
		column.name = "id"
		column.notNull = true
		column.primaryKey = driver != "clickhouse"
	}
	return column
}

func dbDraftOf(column openidea.DBColumnInfo, key int) dbColumnDraft {
	original := column
	defaultValue := ""
	if column.Default != nil {
		defaultValue = *column.Default
	}
	return dbColumnDraft{
		key: key, original: &original, name: column.Name, dataType: column.DataType,
		defaultValue: defaultValue, notNull: !column.Nullable, primaryKey: column.PrimaryKey,
	}
}

func (m *Model) openDBTableForm(arg any, creating, addColumn bool) {
	t := dbTargetFrom(m, arg)
	if t.Source == "" {
		return
	}
	v := m.dbEnsureOpen()
	driver := m.dbDriver(t.Source)
	form := &dbTableForm{
		open: true, creating: creating, addColumn: addColumn,
		source: t.Source, schema: t.Schema, table: t.Table,
		name: t.Table, nextKey: 1,
	}
	if creating {
		form.name = "table_name"
		form.columns = append(form.columns, dbBlankColumn(driver, form.nextKey, 1))
		form.selected = form.nextKey
		form.nextKey++
		v.tableForm = form
		m.invalidate()
		return
	}
	if t.Table == "" {
		return
	}
	form.loading = true
	v.tableForm = form
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		details, err := m.mgr.DBTableDetailsData(ctx, Window, t.Source, t.Schema, t.Table)
		m.post(func() {
			if m.db == nil || m.db.tableForm != form {
				return
			}
			form.loading = false
			if err != nil {
				form.err = "Could not read the table columns: " + err.Error()
				m.invalidate()
				return
			}
			for _, column := range details.Columns {
				form.columns = append(form.columns, dbDraftOf(column, form.nextKey))
				form.nextKey++
			}
			if addColumn {
				column := dbBlankColumn(driver, form.nextKey, len(form.columns)+1)
				form.nextKey++
				form.columns = append(form.columns, column)
				form.selected = column.key
			} else if len(form.columns) > 0 {
				form.selected = form.columns[0].key
			}
			m.invalidate()
		})
	}()
}

func dbTypeSuggestions(driver string) []string {
	switch driver {
	case "snowflake":
		return []string{
			"NUMBER(38,0)", "NUMBER(10,2)", "FLOAT", "VARCHAR", "BOOLEAN",
			"DATE", "TIME", "TIMESTAMP_NTZ", "TIMESTAMP_LTZ", "TIMESTAMP_TZ",
			"VARIANT", "OBJECT", "ARRAY", "BINARY",
		}
	case "sqlite":
		return []string{"INTEGER", "TEXT", "REAL", "NUMERIC", "BLOB", "BOOLEAN", "DATE", "DATETIME"}
	case "postgres":
		return []string{
			"integer", "bigint", "smallint", "serial", "bigserial", "numeric",
			"real", "double precision", "boolean", "text", "varchar(255)", "uuid",
			"date", "timestamp", "timestamptz", "jsonb", "bytea",
		}
	case "mysql":
		return []string{
			"int", "bigint", "smallint", "tinyint(1)", "decimal(10,2)", "double",
			"varchar(255)", "text", "date", "datetime", "timestamp", "json", "blob",
		}
	case "sqlserver":
		return []string{
			"int", "bigint", "smallint", "tinyint", "decimal(10,2)", "float",
			"real", "varchar(255)", "nvarchar(255)", "text", "ntext", "date",
			"datetime", "datetime2", "uniqueidentifier", "bit", "varbinary(max)",
		}
	case "clickhouse":
		return []string{"UInt64", "Int64", "Float64", "String", "Bool", "Date", "DateTime", "UUID", "Nullable(String)"}
	case "oracle":
		return []string{
			"NUMBER", "NUMBER(10)", "NUMBER(19, 4)", "INTEGER", "BINARY_FLOAT",
			"BINARY_DOUBLE", "VARCHAR2(255)", "VARCHAR2(4000)", "NVARCHAR2(255)",
			"CHAR(1)", "CLOB", "BLOB", "RAW(16)", "DATE", "TIMESTAMP",
			"TIMESTAMP WITH TIME ZONE", "INTERVAL DAY TO SECOND", "XMLTYPE",
		}
	}
	return []string{"text"}
}

func dbColumnDefinition(driver string, column dbColumnDraft) string {
	fallback := "text"
	switch driver {
	case "clickhouse":
		fallback = "String"
	case "sqlite":
		fallback = "TEXT"
	case "oracle":
		fallback = "VARCHAR2(255)"
	case "snowflake":
		fallback = "VARCHAR"
	}
	dataType := strings.TrimSpace(column.dataType)
	if dataType == "" {
		dataType = fallback
	}
	definition := dbQuoteIdent(driver, strings.TrimSpace(column.name)) + " " + dataType
	if value := strings.TrimSpace(column.defaultValue); value != "" {
		definition += " default " + value
	}
	if column.notNull && driver != "clickhouse" {
		definition += " not null"
	}
	return definition
}

func (m *Model) dbCreateTableSQL(source, schema, name string, columns []dbColumnDraft) string {
	driver := m.dbDriver(source)
	live := make([]dbColumnDraft, 0, len(columns))
	keys := make([]string, 0)
	for _, column := range columns {
		if column.dropped || strings.TrimSpace(column.name) == "" {
			continue
		}
		live = append(live, column)
		if column.primaryKey {
			keys = append(keys, dbQuoteIdent(driver, strings.TrimSpace(column.name)))
		}
	}
	items := make([]string, 0, len(live)+1)
	for _, column := range live {
		items = append(items, "    "+dbColumnDefinition(driver, column))
	}
	if len(keys) > 0 && driver != "clickhouse" {
		items = append(items, "    primary key ("+strings.Join(keys, ", ")+")")
	}
	suffix := ""
	if driver == "clickhouse" {
		suffix = "\nengine = MergeTree\norder by tuple()"
	}
	return fmt.Sprintf("create table %s\n(\n%s\n)%s;\n",
		m.dbQualifiedName(source, schema, name), strings.Join(items, ",\n"), suffix)
}

func (m *Model) dbAlterTableSQL(source, schema, table, name string, columns []dbColumnDraft) []string {
	driver := m.dbDriver(source)
	if driver == "clickhouse" {
		return nil
	}
	q := m.dbQualifiedName(source, schema, table)
	statements := []string{}
	for _, column := range columns {
		original := column.original
		if original == nil {
			if !column.dropped && strings.TrimSpace(column.name) != "" {
				if driver == "oracle" {
					statements = append(statements,
						fmt.Sprintf("alter table %s add (%s);", q, dbColumnDefinition(driver, column)))
				} else {
					statements = append(statements,
						fmt.Sprintf("alter table %s add column %s;", q, dbColumnDefinition(driver, column)))
				}
			}
			continue
		}
		if column.dropped {
			statements = append(statements,
				fmt.Sprintf("alter table %s drop column %s;", q, dbQuoteIdent(driver, original.Name)))
			continue
		}
		newName := strings.TrimSpace(column.name)
		renamed := newName != "" && newName != original.Name
		originalDefault := ""
		if original.Default != nil {
			originalDefault = strings.TrimSpace(*original.Default)
		}
		changed := strings.TrimSpace(column.dataType) != original.DataType ||
			column.notNull != !original.Nullable ||
			strings.TrimSpace(column.defaultValue) != originalDefault
		if driver == "mysql" && (renamed || changed) {
			statements = append(statements, fmt.Sprintf("alter table %s change column %s %s;",
				q, dbQuoteIdent(driver, original.Name), dbColumnDefinition(driver, column)))
			continue
		}
		if driver == "oracle" {
			oldName := dbQuoteIdent(driver, original.Name)
			dataType := strings.TrimSpace(column.dataType)
			if dataType != "" && dataType != original.DataType {
				nullability := ""
				if column.notNull {
					nullability = " not null"
				}
				statements = append(statements, fmt.Sprintf("alter table %s modify (%s %s%s);",
					q, oldName, dataType, nullability))
			}
			if column.notNull != !original.Nullable {
				nullability := " null"
				if column.notNull {
					nullability = " not null"
				}
				statements = append(statements, fmt.Sprintf("alter table %s modify (%s%s);",
					q, oldName, nullability))
			}
			if strings.TrimSpace(column.defaultValue) != originalDefault {
				value := strings.TrimSpace(column.defaultValue)
				if value == "" {
					value = "null"
				}
				statements = append(statements, fmt.Sprintf("alter table %s modify (%s default %s);",
					q, oldName, value))
			}
			if renamed {
				statements = append(statements, fmt.Sprintf("alter table %s rename column %s to %s;",
					q, oldName, dbQuoteIdent(driver, newName)))
			}
			continue
		}
		if renamed {
			statements = append(statements, fmt.Sprintf("alter table %s rename column %s to %s;",
				q, dbQuoteIdent(driver, original.Name), dbQuoteIdent(driver, newName)))
		}
		if driver == "snowflake" {
			currentName := newName
			if currentName == "" {
				currentName = original.Name
			}
			columnName := dbQuoteIdent(driver, currentName)
			if strings.TrimSpace(column.dataType) != "" &&
				strings.TrimSpace(column.dataType) != original.DataType {
				statements = append(statements, fmt.Sprintf("alter table %s alter column %s set data type %s;",
					q, columnName, strings.TrimSpace(column.dataType)))
			}
			if column.notNull != !original.Nullable {
				action := "drop"
				if column.notNull {
					action = "set"
				}
				statements = append(statements,
					fmt.Sprintf("alter table %s alter column %s %s not null;", q, columnName, action))
			}
			if strings.TrimSpace(column.defaultValue) == "" && originalDefault != "" {
				statements = append(statements,
					fmt.Sprintf("alter table %s alter column %s drop default;", q, columnName))
			}
			continue
		}
		if driver == "postgres" {
			currentName := original.Name
			if renamed {
				currentName = newName
			}
			columnName := dbQuoteIdent(driver, currentName)
			if strings.TrimSpace(column.dataType) != "" &&
				strings.TrimSpace(column.dataType) != original.DataType {
				statements = append(statements, fmt.Sprintf("alter table %s alter column %s type %s;",
					q, columnName, strings.TrimSpace(column.dataType)))
			}
			if column.notNull != !original.Nullable {
				action := "drop"
				if column.notNull {
					action = "set"
				}
				statements = append(statements,
					fmt.Sprintf("alter table %s alter column %s %s not null;", q, columnName, action))
			}
			if strings.TrimSpace(column.defaultValue) != originalDefault {
				value := strings.TrimSpace(column.defaultValue)
				if value == "" {
					statements = append(statements,
						fmt.Sprintf("alter table %s alter column %s drop default;", q, columnName))
				} else {
					statements = append(statements,
						fmt.Sprintf("alter table %s alter column %s set default %s;", q, columnName, value))
				}
			}
		}
	}
	if newName := strings.TrimSpace(name); newName != "" && newName != table {
		rename := m.dbRenameSQL(dbActionTarget{
			Source: source, Schema: schema, Table: table, ObjectKind: "table",
		}, newName)
		rename = strings.TrimSuffix(strings.TrimSpace(rename), ";")
		statements = append(statements, rename+";")
	}
	return statements
}

func (m *Model) dbTableFormStatements(form *dbTableForm) []string {
	if form == nil {
		return nil
	}
	if form.creating {
		return []string{m.dbCreateTableSQL(form.source, form.schema, strings.TrimSpace(form.name), form.columns)}
	}
	return m.dbAlterTableSQL(form.source, form.schema, form.table, strings.TrimSpace(form.name), form.columns)
}

func (m *Model) ApplyDBTableForm() {
	if m.db == nil || m.db.tableForm == nil {
		return
	}
	form := m.db.tableForm
	if form.busy || form.loading {
		return
	}
	statements := m.dbTableFormStatements(form)
	if len(statements) == 0 {
		return
	}
	dbStatements := make([]openidea.DBStatement, 0, len(statements))
	for _, text := range statements {
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		dbStatements = append(dbStatements, openidea.DBStatement{From: 0, To: utf16Len(text), Text: text})
	}
	form.busy = true
	form.err = ""
	request := m.nextRequest()
	title := "Create Table " + form.name
	if !form.creating {
		title = "Modify Table " + form.table
	}
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 120*time.Second)
		defer cancel()
		outcomes, err := m.mgr.DBExecute(ctx, Window, request, form.source, dbStatements, dbMaxRows, form.schema)
		m.post(func() {
			if m.db == nil || m.db.tableForm != form {
				return
			}
			form.busy = false
			if err != nil {
				form.err = err.Error()
				m.invalidate()
				return
			}
			for _, outcome := range outcomes {
				if outcome.Error != "" {
					form.err = outcome.Error
					m.invalidate()
					return
				}
			}
			m.db.addDBResult(title, form.source, outcomes, "")
			m.db.tableForm = nil
			m.loadDBTables(form.source, form.schema)
			m.invalidate()
		})
	}()
}

func (m *Model) dbTableFormDialog(c *ui.Context, p *ui.Theme) {
	if m.db == nil || m.db.tableForm == nil {
		return
	}
	form := m.db.tableForm
	open := form.open
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(780).Height(640).Gap(10).Children(func() {
			title := "Create Table"
			if !form.creating {
				title = "Modify Table " + form.table
			}
			ui.Text(c, title).FontSize(13).Bold().TextColor(p.Text).SingleLine()
			if form.loading {
				ui.Text(c, "Loading…").FontSize(12).TextColor(p.TextMuted)
			} else {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "Table:").FontSize(12).TextColor(p.TextMuted).Width(48)
					ui.TextArea(c, &form.name).Height(28).SingleLine().Grow(1)
					ui.Text(c, form.schema).FontSize(12).TextColor(p.TextMuted).Width(100).
						SingleLine().Ellipsis("…")
				})
				ui.Row(c).Grow(1).MinHeight(0).Gap(12).AlignItems(ui.Stretch).Children(func() {
					m.dbTableFormColumns(c, p, form)
					m.dbTableFormProperties(c, p, form)
				})
				preview := strings.Join(m.dbTableFormStatements(form), "\n")
				if preview == "" {
					preview = "-- no changes"
				}
				ui.Text(c, "Preview").FontSize(12).TextColor(p.TextMuted).SingleLine()
				ui.TextArea(c, &preview).ReadOnly(true).Height(150).Font("monospace").
					FontSize(12).TextColor(p.TextMuted)
			}
			if form.err != "" {
				ui.Text(c, form.err).FontSize(12).TextColor(p.Danger).SingleLine().Ellipsis("…")
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Spacer(c)
				ui.Button(c, "Cancel").OnClick(func() {
					m.db.tableForm = nil
					m.invalidate()
				})
				statementCount := len(m.dbTableFormStatements(form))
				ui.Button(c, "OK").Disabled(form.loading || form.busy || strings.TrimSpace(form.name) == "" ||
					statementCount == 0).OnClick(func() { m.ApplyDBTableForm() })
			})
		})
	})
	if !open && m.db != nil && m.db.tableForm == form {
		m.db.tableForm = nil
		m.invalidate()
	}
}

func (m *Model) dbTableFormColumns(c *ui.Context, p *ui.Theme, form *dbTableForm) {
	driver := m.dbDriver(form.source)
	ui.Column(c).Width(280).Gap(6).Children(func() {
		ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
			ui.Button(c, "+").OnClick(func() {
				column := dbBlankColumn(driver, form.nextKey, len(form.columns)+1)
				form.nextKey++
				form.columns = append(form.columns, column)
				form.selected = column.key
				m.invalidate()
			})
			current := dbFormSelectedColumn(form)
			removeLabel := "−"
			if current != nil && current.dropped {
				removeLabel = "Restore"
			}
			ui.Button(c, removeLabel).Disabled(current == nil).OnClick(func() {
				current := dbFormSelectedColumn(form)
				if current == nil {
					return
				}
				if current.original != nil {
					current.dropped = !current.dropped
				} else {
					key := current.key
					columns := form.columns[:0]
					for _, column := range form.columns {
						if column.key != key {
							columns = append(columns, column)
						}
					}
					form.columns = columns
					if len(form.columns) > 0 {
						form.selected = form.columns[len(form.columns)-1].key
					} else {
						form.selected = 0
					}
				}
				m.invalidate()
			})
			ui.Spacer(c)
			ui.Text(c, fmt.Sprintf("%d columns", len(form.columns))).FontSize(12).TextColor(p.TextMuted)
		})
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).Padding(0, 6).Children(func() {
				for i := range form.columns {
					column := &form.columns[i]
					row := ui.Row(c.Key(fmt.Sprintf("dbtableform:column:%d", column.key))).Height(26).
						Padding(0, 6).Gap(6).Radius(5).AlignItems(ui.Center)
					if form.selected == column.key || row.Hovered() {
						row.Background(p.SurfaceHover)
					}
					row.Children(func() {
						name := column.name
						if name == "" {
							name = "<unnamed>"
						}
						icon := "openidea/database/column"
						if column.primaryKey {
							icon = "openidea/database/columnKey"
						}
						oiIconEl(c, icon, 16, oiPaletteOf(p), p.TextMuted).MarginEnd(2)
						color := p.Text
						if column.dropped {
							color = p.TextMuted
						}
						ui.Text(c, name).FontSize(13).TextColor(color).SingleLine().Ellipsis("…").Grow(1)
						ui.Text(c, column.dataType).FontSize(11).TextColor(p.TextMuted).
							SingleLine().Ellipsis("…")
					})
					if row.Clicked() {
						form.selected = column.key
						m.invalidate()
					}
				}
			})
		})
	})
}

func dbFormSelectedColumn(form *dbTableForm) *dbColumnDraft {
	if form == nil {
		return nil
	}
	for i := range form.columns {
		if form.columns[i].key == form.selected {
			return &form.columns[i]
		}
	}
	return nil
}

func (m *Model) dbTableFormProperties(c *ui.Context, p *ui.Theme, form *dbTableForm) {
	driver := m.dbDriver(form.source)
	column := dbFormSelectedColumn(form)
	ui.Column(c).Grow(1).MinWidth(0).Gap(6).Children(func() {
		if column == nil {
			ui.Text(c, "Select a column").FontSize(12).TextColor(p.TextMuted)
			return
		}
		ui.Text(c, "Name").FontSize(12).TextColor(p.TextMuted)
		ui.TextArea(c.Key(fmt.Sprintf("dbtableform:name:%d", column.key)), &column.name).
			Height(28).SingleLine()
		ui.Text(c, "Data type").FontSize(12).TextColor(p.TextMuted)
		ui.TextArea(c.Key(fmt.Sprintf("dbtableform:type:%d", column.key)), &column.dataType).
			Height(28).SingleLine().Font("monospace")
		types := dbTypeSuggestions(driver)
		preset := column.dataType
		if choice := ui.Select(c, &preset, types).Width(260); choice.Changed() {
			column.dataType = preset
			m.invalidate()
		}
		if driver != "clickhouse" {
			ui.Checkbox(c, &column.notNull, "Not null")
		}
		if form.creating && driver != "clickhouse" {
			ui.Checkbox(c, &column.primaryKey, "Primary key")
		}
		ui.Text(c, "Default").FontSize(12).TextColor(p.TextMuted)
		ui.TextArea(c.Key(fmt.Sprintf("dbtableform:default:%d", column.key)), &column.defaultValue).
			Height(28).SingleLine().Font("monospace")
		if column.dropped {
			ui.Text(c, "This existing column will be dropped.").FontSize(12).TextColor(p.Danger)
		}
		if driver == "sqlite" && column.original != nil && !column.dropped {
			ui.Text(c, "SQLite can rename or drop an existing column, not change its type, NOT NULL or default.").
				FontSize(12).TextColor(p.TextMuted)
		}
		if driver == "clickhouse" && !form.creating {
			ui.Text(c, "ClickHouse table columns cannot be modified from this form.").
				FontSize(12).TextColor(p.TextMuted)
		}
	})
}

func (m *Model) DBModifyTable(arg any) {
	m.openDBTableForm(arg, false, false)
}

func (m *Model) DBNewTable(arg any) {
	m.openDBTableForm(arg, true, false)
}

func (m *Model) DBNewColumn(arg any) {
	m.openDBTableForm(arg, false, true)
}

func (m *Model) DBImportData(arg any) {
	t := dbTargetFrom(m, arg)
	if t.Source == "" || t.Table == "" {
		return
	}
	v := m.dbEnsureOpen()
	form := &dbImportForm{
		open: true, loading: true, source: t.Source, schema: t.Schema, table: t.Table,
		separator: "Comma", header: true, emptyAsNull: true,
	}
	v.importForm = form
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		details, err := m.mgr.DBTableDetailsData(ctx, Window, t.Source, t.Schema, t.Table)
		m.post(func() {
			if m.db == nil || m.db.importForm != form {
				return
			}
			form.loading = false
			if err != nil {
				form.err = "Could not inspect the import table: " + err.Error()
				m.invalidate()
				return
			}
			for _, column := range details.Columns {
				form.columns = append(form.columns, column.Name)
			}
			m.refreshDBImportForm()
		})
	}()
}

func dbSeparatorValue(label string) rune {
	switch label {
	case "Tab":
		return '\t'
	case "Semicolon":
		return ';'
	case "Pipe":
		return '|'
	default:
		return ','
	}
}

func dbGuessSeparator(text string) string {
	first := strings.SplitN(strings.TrimPrefix(text, "\ufeff"), "\n", 2)[0]
	switch {
	case strings.Contains(first, "\t"):
		return "Tab"
	case strings.Contains(first, ";"):
		return "Semicolon"
	case strings.Contains(first, "|"):
		return "Pipe"
	default:
		return "Comma"
	}
}

func (m *Model) refreshDBImportForm() {
	if m.db == nil || m.db.importForm == nil {
		return
	}
	form := m.db.importForm
	form.err = ""
	form.rows = nil
	form.edits = nil
	form.mapped = nil
	if strings.TrimSpace(form.text) == "" || len(form.columns) == 0 {
		m.invalidate()
		return
	}
	rows, err := dbParseDelimitedSeparator(form.text, dbSeparatorValue(form.separator))
	if err != nil {
		form.err = err.Error()
		m.invalidate()
		return
	}
	form.rows = rows
	form.edits, form.mapped = dbImportEdits(rows, form.columns, form.header, form.emptyAsNull)
	m.invalidate()
}

func (m *Model) BrowseDBImportFile() {
	if m.db == nil || m.db.importForm == nil {
		return
	}
	form := m.db.importForm
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Title: "Import Data from File",
			Filters: []mygo.FileFilter{
				{Name: "Delimited Data", Extensions: []string{"csv", "tsv", "txt"}},
			},
		})
		if err != nil || len(paths) == 0 {
			return
		}
		raw, err := os.ReadFile(paths[0])
		m.post(func() {
			if m.db == nil || m.db.importForm != form {
				return
			}
			if err != nil {
				form.err = "Could not read the import file: " + err.Error()
				m.invalidate()
				return
			}
			form.file = paths[0]
			form.text = string(raw)
			form.separator = dbGuessSeparator(form.text)
			if strings.EqualFold(filepath.Ext(paths[0]), ".tsv") {
				form.separator = "Tab"
			}
			m.refreshDBImportForm()
		})
	}()
}

func (m *Model) ApplyDBImport() {
	if m.db == nil || m.db.importForm == nil {
		return
	}
	form := m.db.importForm
	if form.busy || len(form.edits) == 0 || len(form.mapped) == 0 {
		return
	}
	form.busy = true
	form.err = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 120*time.Second)
		defer cancel()
		changed, err := m.mgr.DBApplyEdits(ctx, Window, form.source, form.schema, form.table, form.edits)
		m.post(func() {
			if m.db == nil || m.db.importForm != form {
				return
			}
			form.busy = false
			if err != nil {
				form.err = "Nothing was imported: " + err.Error()
				m.invalidate()
				return
			}
			if changed != nil {
				m.db.outLog = append(m.db.outLog,
					fmt.Sprintf("%s: %s imported", form.table, count(int(*changed), "row", "rows")))
			}
			m.db.importForm = nil
			m.loadDBTables(form.source, form.schema)
			m.invalidate()
		})
	}()
}

func (m *Model) dbImportFormDialog(c *ui.Context, p *ui.Theme) {
	if m.db == nil || m.db.importForm == nil {
		return
	}
	form := m.db.importForm
	open := form.open
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(760).Height(620).Gap(10).Children(func() {
			ui.Text(c, fmt.Sprintf("Import Data into %s.%s", form.schema, form.table)).
				FontSize(13).Bold().TextColor(p.Text).SingleLine()
			if form.loading {
				ui.Text(c, "Loading…").FontSize(12).TextColor(p.TextMuted)
			} else {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "File:").FontSize(12).TextColor(p.TextMuted).Width(72)
					ui.TextArea(c, &form.file).Height(28).SingleLine().ReadOnly(true).Grow(1)
					ui.Button(c, "Choose File…").OnClick(func() { m.BrowseDBImportFile() })
				})
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "Separator:").FontSize(12).TextColor(p.TextMuted).Width(72)
					separator := form.separator
					if choice := ui.Select(c, &separator,
						[]string{"Comma", "Tab", "Semicolon", "Pipe"}).Width(150); choice.Changed() {
						form.separator = separator
						m.refreshDBImportForm()
					}
				})
				if ui.Checkbox(c, &form.header, "First row is header (columns matched by name)").Changed() {
					m.refreshDBImportForm()
				}
				if ui.Checkbox(c, &form.emptyAsNull, "Treat empty values as NULL").Changed() {
					m.refreshDBImportForm()
				}
				mapped := "No column of the file matches the table"
				if len(form.mapped) > 0 {
					mapped = strings.Join(form.mapped, ", ")
				}
				ui.Text(c, "Columns: "+mapped).FontSize(12).TextColor(p.TextMuted).
					SingleLine().Ellipsis("…")
				ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
					ui.Column(c).Gap(2).Children(func() {
						preview := form.rows
						if len(preview) > 8 {
							preview = preview[:8]
						}
						for rowIndex, row := range preview {
							rowIndex, row := rowIndex, row
							r := ui.Row(c.Key(fmt.Sprintf("dbimport:row:%d", rowIndex))).Height(26).
								Padding(0, 6).Gap(4).AlignItems(ui.Center)
							if rowIndex%2 == 1 || (form.header && rowIndex == 0) {
								r.Background(p.SurfaceHover)
							}
							r.Children(func() {
								for cellIndex, cell := range row {
									text := cell
									if form.header && rowIndex == 0 {
										text = strings.ToUpper(text)
									}
									ui.Text(c, text).FontSize(12).TextColor(p.Text).
										Width(120).SingleLine().Ellipsis("…")
									if cellIndex >= 5 {
										break
									}
								}
							})
						}
					})
				})
			}
			if form.err != "" {
				ui.Text(c, form.err).FontSize(12).TextColor(p.Danger).SingleLine().Ellipsis("…")
			}
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Spacer(c)
				ui.Button(c, "Cancel").OnClick(func() {
					m.db.importForm = nil
					m.invalidate()
				})
				label := fmt.Sprintf("Import %d Rows", len(form.edits))
				ui.Button(c, label).Disabled(form.loading || form.busy || len(form.edits) == 0 ||
					len(form.mapped) == 0).OnClick(func() { m.ApplyDBImport() })
			})
		})
	})
	if !open && m.db != nil && m.db.importForm == form {
		m.db.importForm = nil
		m.invalidate()
	}
}
