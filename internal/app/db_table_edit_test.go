package app

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

func dbEditableSheet() *dbTableSheet {
	return &dbTableSheet{
		open: true, source: "pg", schema: "public", table: "people",
		driver: "postgres", limit: 100, selectedRow: 0, selectedCol: 1,
		editRow: -1, editCol: -1,
		page: openidea.DBTablePage{
			Result: openidea.DBQueryResult{
				Columns: []openidea.DBColumn{
					{Name: "id", TypeName: "int4"},
					{Name: "active", TypeName: "bool"},
					{Name: "name", TypeName: "varchar"},
				},
				Rows: [][]*string{
					{dbTestString("1"), dbTestString("t"), dbTestString("Ada")},
				},
			},
			Details: openidea.DBTableDetails{Columns: []openidea.DBColumnInfo{
				{Name: "id", DataType: "int4", PrimaryKey: true},
				{Name: "active", DataType: "boolean", Nullable: true},
				{Name: "name", DataType: "varchar", Nullable: false, Default: dbTestString("'unknown'::text")},
			}},
		},
		pending: map[string]*string{},
		deletes: map[int]bool{},
	}
}

func TestDBTableEditabilityMatchesOpenIDEA(t *testing.T) {
	s := dbEditableSheet()
	if editable, reason := dbTableEditability(s); !editable || reason != "" {
		t.Fatalf("editable = %v, reason = %q", editable, reason)
	}

	s.readOnly = true
	if editable, reason := dbTableEditability(s); editable || reason != "Read-only data source." {
		t.Fatalf("read-only source: editable=%v reason=%q", editable, reason)
	}

	s.readOnly = false
	s.driver = "clickhouse"
	if editable, reason := dbTableEditability(s); editable ||
		reason != "Use the query console to change ClickHouse data." {
		t.Fatalf("clickhouse: editable=%v reason=%q", editable, reason)
	}

	s.driver = "postgres"
	s.page.Details.Columns[0].PrimaryKey = false
	if editable, reason := dbTableEditability(s); editable ||
		reason != "Read-only: the table has no primary key to identify rows." {
		t.Fatalf("no key: editable=%v reason=%q", editable, reason)
	}
}

func TestDBTableCellEditsReturnToOriginalAndKeepInsertDefaultsDistinct(t *testing.T) {
	s := dbEditableSheet()
	changed := "Grace"
	dbTableSetCellValue(s, 0, 2, &changed)
	if len(s.pending) != 1 {
		t.Fatalf("pending = %#v", s.pending)
	}
	dbTableSetCellValue(s, 0, 2, dbTestString("Ada"))
	if len(s.pending) != 0 {
		t.Fatalf("returning to the original text left a pending edit: %#v", s.pending)
	}

	dbTableAddRow(s)
	row := len(s.page.Result.Rows)
	s.selectedRow = row
	s.selectedCol = 2
	dbTableSetCellValue(s, row, 2, nil)
	values := s.inserts[0]
	if value, ok := values["name"]; !ok || value != nil {
		t.Fatalf("explicit NULL = %#v, present=%v", value, ok)
	}
	dbTableSetCellDefault(s)
	if _, ok := values["name"]; ok {
		t.Fatalf("Set DEFAULT on an added row kept a value: %#v", values)
	}
}

func TestDBTableBooleanAndDefaultLiteralsFollowTheDriver(t *testing.T) {
	cases := []struct {
		columnType string
		driver     string
		bool       bool
		trueValue  string
		falseValue string
	}{
		{"boolean", "postgres", true, "t", "f"},
		{"tinyint(1)", "mysql", true, "1", "0"},
		{"bit", "postgres", false, "", ""},
		{"BIT", "sqlserver", true, "true", "false"},
	}
	for _, tc := range cases {
		if got := dbIsBooleanType(tc.columnType, tc.driver); got != tc.bool {
			t.Errorf("%s on %s boolean = %v", tc.columnType, tc.driver, got)
		}
		if !tc.bool {
			continue
		}
		values, _ := dbBooleanLiterals(tc.driver)
		if values.trueValue != tc.trueValue || values.falseValue != tc.falseValue {
			t.Errorf("%s on %s literals = %#v", tc.columnType, tc.driver, values)
		}
	}

	for expression, want := range map[string]string{
		"'active'":                    "active",
		"'it''s'::character varying":  "it's",
		"''::text":                    "",
		"((42))":                      "42",
		"FALSE":                       "false",
		"now()":                       "",
		"nextval('people_id_seq')":    "",
		"CURRENT_TIMESTAMP":           "",
		"'broken'::varchar COLLATE C": "",
	} {
		got, ok := dbDefaultLiteral(dbTestString(expression))
		if want == "" && expression != "''::text" {
			if ok {
				t.Errorf("default %q accepted as %q", expression, got)
			}
			continue
		}
		if !ok || got != want {
			t.Errorf("default %q = %q, %v, want %q", expression, got, ok, want)
		}
	}

	s := dbEditableSheet()
	s.selectedRow, s.selectedCol = 0, 1
	dbTableToggleBoolean(s)
	if got := dbTableDisplayedCell(s, 0, 1); got == nil || *got != "f" {
		t.Fatalf("postgres boolean = %#v", got)
	}

	s.page.Details.Columns[1].DataType = "tinyint(1)"
	s.driver = "mysql"
	s.pending = map[string]*string{}
	dbTableToggleBoolean(s)
	if got := dbTableDisplayedCell(s, 0, 1); got == nil || *got != "1" {
		t.Fatalf("mysql boolean = %#v", got)
	}
}

func TestDBBuildRowEditsRejectsUneditableDataAndIsDeterministic(t *testing.T) {
	s := dbEditableSheet()
	one := "one"
	two := "two"
	s.pending = map[string]*string{"0:2": &two, "0:1": &one}
	edits, err := dbBuildRowEdits(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(edits) != 1 || len(edits[0].Values) != 2 ||
		edits[0].Values[0].Column != "active" || edits[0].Values[1].Column != "name" {
		t.Fatalf("edits = %#v", edits)
	}

	s.readOnly = true
	if _, err := dbBuildRowEdits(s); err == nil ||
		!strings.Contains(err.Error(), "Read-only data source") {
		t.Fatalf("read-only build error = %v", err)
	}
}

func TestDBTableEditorActionsUseDirtyAndEditableState(t *testing.T) {
	s := dbEditableSheet()
	m := &Model{dbTable: s}
	if m.ActionEnabled("db.submitChanges") || m.ActionEnabled("db.revertChanges") {
		t.Fatal("dirty actions were enabled without pending edits")
	}
	changed := "Grace"
	dbTableSetCellValue(s, 0, 2, &changed)
	if !m.ActionEnabled("db.submitChanges") || !m.ActionEnabled("db.revertChanges") {
		t.Fatal("dirty actions were disabled with a pending edit")
	}
	if !m.ActionEnabled("db.toggleBooleanCell") || !m.ActionEnabled("db.setCellNull") {
		t.Fatal("editable boolean and nullable cell actions were disabled")
	}
	s.selectedCol = 2
	if m.ActionEnabled("db.toggleBooleanCell") {
		t.Fatal("toggle was enabled for a non-boolean column")
	}
	s.readOnly = true
	if m.ActionEnabled("db.submitChanges") {
		t.Fatal("a read-only source allowed submit")
	}
}

func TestDBTableDialogRendersReadOnlyReasonAndEditableActions(t *testing.T) {
	s := dbEditableSheet()
	m := &Model{dbTable: s}
	tester := ui.NewTester(func(c *ui.Context) {
		m.dbTableDialog(c, c.Theme())
	}, 1100, 760)
	text := strings.Join(tester.Texts(), "\n")
	for _, want := range []string{"Table data: people", "Add Row", "Submit", "Revert"} {
		if !strings.Contains(text, want) {
			t.Errorf("editable dialog is missing %q", want)
		}
	}

	s.readOnly = true
	s.pending = map[string]*string{}
	tester.Frame()
	text = strings.Join(tester.Texts(), "\n")
	if !strings.Contains(text, "Read-only data source.") {
		t.Fatalf("read-only dialog omitted the reason:\n%s", text)
	}
}

func TestDBTableCloseProtectsPendingEdits(t *testing.T) {
	s := dbEditableSheet()
	changed := "Grace"
	dbTableSetCellValue(s, 0, 2, &changed)
	m := &Model{dbTable: s}
	m.DBCloseTable()
	if m.dbTable != s || !s.closeAsk {
		t.Fatal("closing with pending edits did not ask for confirmation")
	}
	m.DBDiscardAndCloseTable()
	if m.dbTable != nil {
		t.Fatal("discard did not close the table")
	}
}
