package app

import (
	"reflect"
	"testing"

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

func dbTestString(value string) *string { return &value }
