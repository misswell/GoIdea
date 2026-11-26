package app

import (
	"reflect"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

func TestDatabaseActionsAreRegistered(t *testing.T) {
	ids := []string{
		"db.closeAllResultTabs", "db.closeOtherResultTabs", "db.closeResultTab",
		"db.collapseAll", "db.copyReference", "db.ddlToClipboard", "db.ddlToConsole",
		"db.disconnect", "db.drop", "db.duplicateSource", "db.editData", "db.execute",
		"db.executeScript", "db.explainPlan", "db.exportData", "db.findInValue", "db.goToColumn",
		"db.goToDdl", "db.goToReferencedTable", "db.importData", "db.introspect",
		"db.jumpToConsole", "db.modifyTable", "db.new", "db.newColumn",
		"db.newDataSource", "db.newTable", "db.openConsole", "db.openValueWindow",
		"db.pluginConnect", "db.properties", "db.queryHistory", "db.quickDoc",
		"db.reformatValue", "db.refresh", "db.rename", "db.renameResultTab",
		"db.revertChanges", "db.runSqlScript", "db.separateProceduresAndFunctions",
		"db.setCellDefault", "db.setCellNull", "db.showToolWindow", "db.sqlScript",
		"db.submitChanges", "db.toggleBooleanCell", "db.truncate",
	}
	for _, id := range ids {
		if _, ok := Command(id); !ok {
			t.Errorf("%s is not registered", id)
		}
	}
}

func TestDatabaseBuildRowEditsUsesPrimaryKey(t *testing.T) {
	updated := "Ada"
	inserted := "Linus"
	s := &dbTableSheet{
		page: openidea.DBTablePage{
			Result: openidea.DBQueryResult{
				Columns: []openidea.DBColumn{{Name: "id"}, {Name: "name"}},
				Rows:    [][]*string{{dbTestString("1"), dbTestString("Grace")}},
			},
			Details: openidea.DBTableDetails{Columns: []openidea.DBColumnInfo{
				{Name: "id", PrimaryKey: true}, {Name: "name"},
			}},
		},
		pending: map[string]*string{"0:1": &updated},
		deletes: map[int]bool{},
		inserts: []map[string]*string{{"name": &inserted}},
	}
	edits, err := dbBuildRowEdits(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(edits) != 2 {
		t.Fatalf("edits = %#v", edits)
	}
	var update, insert *openidea.DBRowEdit
	for i := range edits {
		switch edits[i].Kind {
		case "update":
			update = &edits[i]
		case "insert":
			insert = &edits[i]
		}
	}
	if update == nil || len(update.Key) != 1 || update.Key[0].Column != "id" ||
		update.Key[0].Value == nil || *update.Key[0].Value != "1" ||
		len(update.Values) != 1 || update.Values[0].Column != "name" ||
		update.Values[0].Value == nil || *update.Values[0].Value != "Ada" {
		t.Fatalf("update = %#v", update)
	}
	if insert == nil || len(insert.Values) != 1 ||
		insert.Values[0].Value == nil || *insert.Values[0].Value != "Linus" {
		t.Fatalf("insert = %#v", insert)
	}
}

func TestDatabaseBuildRowEditsRejectsTablesWithoutPrimaryKey(t *testing.T) {
	value := "changed"
	s := &dbTableSheet{
		page: openidea.DBTablePage{
			Result: openidea.DBQueryResult{
				Columns: []openidea.DBColumn{{Name: "name"}},
				Rows:    [][]*string{{dbTestString("old")}},
			},
		},
		pending: map[string]*string{"0:0": &value},
	}
	if _, err := dbBuildRowEdits(s); err == nil {
		t.Fatal("a table without a primary key allowed a row update")
	}
}

func TestDatabaseImportMapsHeadersAndNulls(t *testing.T) {
	edits, mapped := dbImportEdits([][]string{
		{"NAME", "unknown"},
		{"Ada", "ignored"},
		{"", "ignored"},
	}, []string{"id", "name"}, true, true)
	if !reflect.DeepEqual(mapped, []string{"name"}) || len(edits) != 2 {
		t.Fatalf("mapped=%v edits=%#v", mapped, edits)
	}
	if edits[0].Values[0].Value == nil || *edits[0].Values[0].Value != "Ada" {
		t.Fatalf("first import row = %#v", edits[0])
	}
	if edits[1].Values[0].Value != nil {
		t.Fatalf("empty import field was not NULL: %#v", edits[1])
	}
}

func TestDatabaseImportMatchesBOMBlankRowsAndRFCFields(t *testing.T) {
	rows, err := dbParseDelimitedSeparator(
		"\ufeffname,note\r\n Ada ,\"a,b\"\r\n\r\nBob,\"kept\"\n", ',',
	)
	if err != nil {
		t.Fatal(err)
	}
	wantRows := [][]string{
		{"name", "note"},
		{" Ada ", "a,b"},
		{"Bob", "kept"},
	}
	if !reflect.DeepEqual(rows, wantRows) {
		t.Fatalf("RFC rows = %#v, want %#v", rows, wantRows)
	}
	edits, mapped := dbImportEdits(
		[][]string{{"name", "note"}, {"Ada", ""}, {"", ""}, {"Bob"}},
		[]string{"name", "note"}, true, true,
	)
	if !reflect.DeepEqual(mapped, []string{"name", "note"}) || len(edits) != 2 {
		t.Fatalf("mapped=%v edits=%#v", mapped, edits)
	}
	if edits[0].Values[1].Value != nil {
		t.Fatalf("empty field was not NULL: %#v", edits[0])
	}
	if edits[1].Values[1].Value != nil {
		t.Fatalf("missing trailing field was not NULL: %#v", edits[1])
	}
}

func TestDatabaseSourceFormPreservesUnknownFields(t *testing.T) {
	f := &dbSourceForm{
		raw: map[string]any{
			"id": "source", "name": "Shop", "driver": "postgres",
			"vendorSpecific": "kept",
			"options":        map[string]any{"transaction": "manual"},
		},
		id: "source", name: "Shop", driver: "postgres",
		host: "localhost", sslMode: "prefer", transaction: "manual",
	}
	f.apply()
	if f.raw["vendorSpecific"] != "kept" {
		t.Fatalf("unknown source field was lost: %#v", f.raw)
	}
	options, ok := f.raw["options"].(map[string]any)
	if !ok || options["transaction"] != "manual" {
		t.Fatalf("options = %#v", f.raw["options"])
	}
}

func TestDatabaseSQLScripts(t *testing.T) {
	m := &Model{db: &dbView{sources: []openidea.DataSource{{
		ID: "pg", Driver: "postgres",
	}}}}
	got := m.dbScriptSQL("pg", "update", "public", "people", []openidea.DBColumnInfo{
		{Name: "id", PrimaryKey: true},
		{Name: "name"},
	})
	want := "update public.people\nset name = ?\nwhere id = ?;\n"
	if got != want {
		t.Fatalf("script = %q, want %q", got, want)
	}
	if target, current := dbBaseTarget("pg:shop"), "pg"; target != current {
		t.Fatalf("database target = %q, want %q", target, current)
	}
	if issue := dbScriptIssue("clickhouse", "update", []openidea.DBColumnInfo{{Name: "id", PrimaryKey: true}}); issue == "" {
		t.Fatal("ClickHouse sorting-key-only update did not fail")
	}
	if issue := dbScriptIssue(
		"clickhouse", "update",
		[]openidea.DBColumnInfo{{Name: "id", PrimaryKey: true}, {Name: "name"}},
	); issue != "" {
		t.Fatalf("writable ClickHouse update was rejected: %s", issue)
	}
}

func TestDatabaseCreateAndAlterTableSQL(t *testing.T) {
	m := &Model{db: &dbView{sources: []openidea.DataSource{
		{ID: "lite", Driver: "sqlite"},
		{ID: "pg", Driver: "postgres"},
	}}}
	created := m.dbCreateTableSQL("lite", "main", "people", []dbColumnDraft{
		{name: "id", dataType: "INTEGER", notNull: true, primaryKey: true},
		{name: "name", dataType: "TEXT"},
	})
	wantCreated := "create table people\n(\n    id INTEGER not null,\n    name TEXT,\n    primary key (id)\n);\n"
	if created != wantCreated {
		t.Fatalf("create table = %q, want %q", created, wantCreated)
	}

	email := openidea.DBColumnInfo{Name: "email", DataType: "TEXT", Nullable: false}
	name := openidea.DBColumnInfo{Name: "name", DataType: "TEXT", Nullable: true}
	columns := []dbColumnDraft{
		dbDraftOf(email, 1),
		dbDraftOf(name, 2),
	}
	columns[0].name = "mail"
	columns[1].dropped = true
	columns = append(columns, dbColumnDraft{key: 3, name: "age", dataType: "INTEGER", defaultValue: "0"})
	got := m.dbAlterTableSQL("lite", "main", "users", "people", columns)
	want := []string{
		"alter table users rename column email to mail;",
		"alter table users drop column name;",
		"alter table users add column age INTEGER default 0;",
		"alter table users rename to people;",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("alter table = %#v, want %#v", got, want)
	}
}

func TestDatabaseIdentifierFoldingAndDelimitedParsing(t *testing.T) {
	if got := dbQuoteIdent("postgres", "Users"); got != `"Users"` {
		t.Fatalf("postgres identifier = %q", got)
	}
	if got := dbQuoteIdent("sqlite", "select"); got != `"select"` {
		t.Fatalf("reserved identifier = %q", got)
	}
	if got := dbQuoteIdent("sqlite", "Users"); got != "Users" {
		t.Fatalf("sqlite mixed-case identifier = %q", got)
	}
	if got := dbQuoteIdent("clickhouse", "price$usd"); got != `"price$usd"` {
		t.Fatalf("clickhouse dollar identifier = %q", got)
	}
	rows, err := dbParseDelimitedSeparator("name|age\nAda|37\n", '|')
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rows, [][]string{{"name", "age"}, {"Ada", "37"}}) {
		t.Fatalf("rows = %#v", rows)
	}
}

func TestDatabaseRenameDropAndTruncateSQL(t *testing.T) {
	m := &Model{db: &dbView{sources: []openidea.DataSource{
		{ID: "pg", Driver: "postgres"},
		{ID: "my", Driver: "mysql"},
		{ID: "lite", Driver: "sqlite"},
	}}}
	if got := m.dbRenameSQL(dbActionTarget{
		Source: "my", Schema: "shop", Table: "users", Kind: "table",
	}, "people"); got != "rename table shop.users to shop.people;" {
		t.Fatalf("mysql rename = %q", got)
	}
	if got := m.dbRenameSQL(dbActionTarget{
		Source: "pg", Schema: "public", ObjectKind: "index", ObjectName: "ix_users",
	}, "ix_people"); got != "alter index public.ix_users rename to ix_people;" {
		t.Fatalf("index rename = %q", got)
	}
	if got := m.dbDropSQL(dbActionTarget{
		Source: "pg", Schema: "public", ObjectKind: "trigger", ObjectName: "audit", Table: "people",
	}, dbDropOptions{IfExists: true, Cascade: true}); got !=
		"drop trigger if exists audit on public.people cascade;" {
		t.Fatalf("trigger drop = %q", got)
	}
	if got := m.dbDropSQL(dbActionTarget{
		Source: "my", Schema: "shop", ObjectKind: "index", ObjectName: "ix_users", Table: "users",
	}, dbDropOptions{IfExists: true}); got != "drop index ix_users on shop.users;" {
		t.Fatalf("mysql index drop = %q", got)
	}
	if got := m.dbTruncateSQL("lite", "main", "people", true, true); got != "delete from people;" {
		t.Fatalf("sqlite truncate = %q", got)
	}
	if got := m.dbTruncateSQL("pg", "public", "people", true, true); got !=
		"truncate table public.people restart identity cascade;" {
		t.Fatalf("postgres truncate = %q", got)
	}
}

func TestDatabaseActionTargetsAndDialectGates(t *testing.T) {
	pg := dbActionTarget{Source: "pg", Schema: "public", Table: "users", Kind: "table"}
	mysqlIndex := dbActionTarget{
		Source: "my", Schema: "shop", Table: "users",
		ObjectKind: "index", ObjectName: "ix_users",
	}
	clickhouse := dbActionTarget{Source: "ch", Schema: "default", Table: "events", Kind: "table"}
	snowflake := dbActionTarget{
		Source: "sf", Schema: "HR", Table: "EMP",
		ObjectKind: "trigger", ObjectName: "AUDIT",
	}
	cases := []struct {
		name string
		t    dbActionTarget
		ren  bool
		drop bool
	}{
		{"postgres table", pg, true, true},
		{"mysql index", mysqlIndex, false, true},
		{"clickhouse table", clickhouse, true, true},
		{"snowflake trigger", snowflake, false, false},
	}
	for _, tc := range cases {
		driver := map[string]string{
			"pg": "postgres", "my": "mysql", "ch": "clickhouse", "sf": "snowflake",
		}[tc.t.Source]
		if got := dbCanRename(driver, dbObjectKind(tc.t)); got != tc.ren {
			t.Errorf("%s rename = %v", tc.name, got)
		}
		if got := dbCanDrop(driver, dbObjectKind(tc.t)); got != tc.drop {
			t.Errorf("%s drop = %v", tc.name, got)
		}
	}
	if dbCanModifyTable("clickhouse") || !dbCanModifyTable("postgres") {
		t.Fatal("modify-table gate does not follow OpenIDEA")
	}

	if !dbRawSourceTarget(map[string]any{"source": "pg"}) ||
		dbRawSourceTarget(map[string]any{"source": "pg", "schema": "public"}) {
		t.Fatal("source-only target was confused with a schema object")
	}
	if !dbRawDatabaseTarget(map[string]any{"source": "pg", "database": "shop"}) ||
		dbRawDatabaseTarget(map[string]any{
			"source": "pg", "database": "shop", "table": "users",
		}) {
		t.Fatal("database-only target was confused with a table")
	}
	if got := dbQuoteIdent("snowflake", "QUALIFY"); got != `"QUALIFY"` {
		t.Fatalf("snowflake reserved identifier = %q", got)
	}
}

func TestDatabaseSourceActionsDoNotInheritTheSelectedTable(t *testing.T) {
	m := &Model{db: &dbView{
		open: true, source: "pg", schema: "public",
		selectedTable: openidea.DBTable{Name: "users", Kind: "table"},
		sources:       []openidea.DataSource{{ID: "pg", Driver: "postgres"}},
	}}
	m.DBDrop(map[string]any{"source": "pg"})
	if m.db.actionPrompt == nil || m.db.actionPrompt.title != "Remove Data Source" ||
		m.db.actionPrompt.table != "" || m.db.actionPrompt.value != "pg" {
		t.Fatalf("source drop prompt = %#v", m.db.actionPrompt)
	}
	m.db.actionPrompt = nil
	m.DBRename(map[string]any{"source": "pg"})
	if m.db.actionPrompt == nil || m.db.actionPrompt.title != "Rename Data Source" ||
		m.db.actionPrompt.table != "" || m.db.actionPrompt.value != "pg" {
		t.Fatalf("source rename prompt = %#v", m.db.actionPrompt)
	}
}

func TestDatabaseViewAndReadOnlyActionGates(t *testing.T) {
	m := &Model{db: &dbView{
		open: true, source: "lite", schema: "main",
		sources: []openidea.DataSource{{ID: "lite", Driver: "sqlite"}},
	}}
	view := map[string]any{"source": "lite", "schema": "main", "table": "summary", "kind": "view"}
	m.DBTruncate(view)
	if m.db.actionPrompt != nil || !strings.Contains(m.db.err, "Only tables") {
		t.Fatalf("view truncate = prompt %#v err %q", m.db.actionPrompt, m.db.err)
	}
	m.db.err = ""
	m.DBModifyTable(view)
	if m.db.tableForm != nil || !strings.Contains(m.db.err, "Only tables") {
		t.Fatalf("view modify = form %#v err %q", m.db.tableForm, m.db.err)
	}
	m.db.err = ""
	m.DBImportData(view)
	if m.db.importForm != nil || !strings.Contains(m.db.err, "available for tables") {
		t.Fatalf("view import = form %#v err %q", m.db.importForm, m.db.err)
	}
}

func TestDatabaseIntrospectionFilteringAndDDLOrder(t *testing.T) {
	all := false
	got := dbShownNames(
		[]string{"app", "app_test", "billing", "postgres"},
		&all, []string{"app*"}, "postgres",
	)
	if !reflect.DeepEqual(got, []string{"app", "app_test", "postgres"}) {
		t.Fatalf("shown databases = %#v", got)
	}
	if !dbMatchesPattern("app_test", "app*") || dbMatchesPattern("app_test", "app?") {
		t.Fatal("database wildcard matching differs from OpenIDEA")
	}
	tables := []openidea.DBTable{
		{Name: "active_people", Kind: "view"},
		{Name: "people", Kind: "table"},
		{Name: "archived_people", Kind: "table"},
	}
	sorted := dbSortDDLTables(tables, "")
	if sorted[0].Name != "people" || sorted[1].Name != "archived_people" || sorted[2].Name != "active_people" {
		t.Fatalf("DDL order = %#v", sorted)
	}
	views := dbSortDDLTables(tables, "views")
	if len(views) != 1 || views[0].Name != "active_people" {
		t.Fatalf("view DDL filter = %#v", views)
	}
}

func TestDatabaseSQLParametersMatchOpenIDEA(t *testing.T) {
	type found struct {
		text string
		drv  string
		n    int
		want []string
	}
	cases := []found{
		{
			"select * from t where a = :id and b = ? and c = ?3 and d = @x and e = '?' -- ?",
			"sqlite", 0, []string{":id", "?1", "?3", "@x"},
		},
		{"select @x := 1, @@version, ? from t", "mysql", 0, []string{"?1"}},
		{
			"select j ? 'k', $1, a::int from t where b = :name",
			"postgres", 0, []string{"$1", ":name"},
		},
		{
			"SELECT src:customer.name::string, $1 FROM @my_stage/path WHERE x = :id",
			"snowflake", 0, []string{":id"},
		},
		{
			"select {value:UInt64}, {label: Nullable(String)}, '{skip:String}'",
			"clickhouse", 0, []string{"{value:UInt64}", "{label: Nullable(String)}"},
		},
	}
	for _, tc := range cases {
		params, next := dbFindSQLParams(tc.text, tc.drv, tc.n)
		names := make([]string, len(params))
		for i, param := range params {
			names[i] = param.Name
		}
		if !reflect.DeepEqual(names, tc.want) {
			t.Errorf("%s names = %#v, want %#v", tc.drv, names, tc.want)
		}
		if tc.drv == "sqlite" && next != 1 {
			t.Errorf("bare positional counter = %d", next)
		}
	}

	sql := "update t set a = :v where id = :id and b = :v"
	params, _ := dbFindSQLParams(sql, "sqlite", 0)
	got := dbBindSQLParams(sql, params, map[string]string{":v": "'x'", ":id": " 3 "})
	if got != "update t set a = 'x' where id = 3 and b = 'x'" {
		t.Fatalf("bound SQL = %q", got)
	}
	missing, _ := dbFindSQLParams("select ?", "sqlite", 0)
	if got := dbBindSQLParams("select ?", missing, nil); got != "select NULL" {
		t.Fatalf("missing parameter = %q", got)
	}
}

func TestDatabaseParameterPromptRendersAndCancels(t *testing.T) {
	v := &dbView{
		open: true, source: "pg", sources: []openidea.DataSource{{ID: "pg", Driver: "postgres"}},
	}
	pending := &dbPendingRun{
		view: v, mode: "execute", source: "pg",
		inputs: []dbParamInput{{name: ":id"}, {name: "$1"}},
	}
	v.pendingRun = pending
	v.actionPrompt = &dbActionPrompt{kind: "params", title: "Parameters"}
	v.running = true
	m := &Model{db: v}
	tester := ui.NewTester(func(c *ui.Context) { m.dbActionPromptDialog(c, c.Theme()) }, 760, 560)
	tester.Frame()
	for _, want := range []string{"Parameters", ":id", "$1", "Execute", "Cancel"} {
		if !tester.HasText(want) {
			t.Fatalf("parameter prompt is missing %q: %v", want, tester.Texts())
		}
	}
	if err := tester.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	if v.actionPrompt != nil || v.pendingRun != nil || v.running {
		t.Fatalf("cancel did not clear the pending run: %#v", v)
	}
}

func TestDatabasePluginConnectRequiresTrustedProject(t *testing.T) {
	m := &Model{db: &dbView{
		open: true, source: "pg", sources: []openidea.DataSource{{ID: "pg", Driver: "postgres"}},
	}}
	m.DBConnectAction(map[string]any{"source": "pg"})
	if !strings.Contains(m.db.err, "Trust this project") {
		t.Fatalf("untrusted connect error = %q", m.db.err)
	}
}

func TestDatabaseRunGenerationSupersedesOlderReplies(t *testing.T) {
	v := &dbView{open: true, runGen: 7}
	m := &Model{db: v}
	gen := m.dbBeginRun(v)
	if gen != 8 || v.runGen != 8 || !v.running {
		t.Fatalf("new run generation = %d, view = %#v", gen, v)
	}
	v.pendingRun = &dbPendingRun{view: v}
	v.actionPrompt = &dbActionPrompt{kind: "params"}
	gen = m.dbBeginRun(v)
	if gen != 9 || v.pendingRun != nil || v.actionPrompt != nil {
		t.Fatalf("superseding run did not clear old state: %#v", v)
	}
}

func dbTestString(value string) *string { return &value }
