package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

func dbExportTestData() dbExportData {
	return dbExportData{
		Columns: []openidea.DBColumn{
			{Name: "id", TypeName: "INTEGER"},
			{Name: "city", TypeName: "TEXT"},
		},
		Rows: [][]*string{
			{dbExportStringPointer("1"), dbExportStringPointer("Shanghai")},
			{dbExportStringPointer("2"), nil},
		},
		Table:  "customers",
		Driver: "sqlite",
	}
}

func TestDBExportDelimitedLayout(t *testing.T) {
	data := dbExportTestData()
	cases := []struct {
		name    string
		format  string
		options dbExportOptions
		want    string
	}{
		{
			name:    "csv default",
			format:  dbExportCSV,
			options: dbExportDefaultOptions,
			want:    "id,city\r\n1,Shanghai\r\n2,",
		},
		{
			name:    "csv without header",
			format:  dbExportCSV,
			options: dbExportOptions{},
			want:    "1,Shanghai\r\n2,",
		},
		{
			name:    "csv row header",
			format:  dbExportCSV,
			options: dbExportOptions{ColumnHeader: true, RowHeader: true},
			want:    "#,id,city\r\n1,1,Shanghai\r\n2,2,",
		},
		{
			name:    "tsv transpose",
			format:  dbExportTSV,
			options: dbExportOptions{ColumnHeader: true, Transpose: true},
			want:    "id\t1\t2\ncity\tShanghai\t",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := dbExportText(test.format, data, test.options); got != test.want {
				t.Fatalf("export = %q, want %q", got, test.want)
			}
		})
	}

	quoted := dbExportData{
		Columns: []openidea.DBColumn{{Name: "id"}, {Name: "note"}},
		Rows: [][]*string{{
			dbExportStringPointer("1"),
			dbExportStringPointer("say \"hi\", ok\nnext"),
		}},
	}
	if got, want := dbExportText(dbExportCSV, quoted, dbExportDefaultOptions),
		"id,note\r\n1,\"say \"\"hi\"\", ok\nnext\""; got != want {
		t.Fatalf("quoted CSV = %q, want %q", got, want)
	}

	flattened := dbExportData{
		Columns: []openidea.DBColumn{{Name: "a"}, {Name: "b"}},
		Rows: [][]*string{{
			dbExportStringPointer("x\ty"),
			dbExportStringPointer("line\nbreak"),
		}},
	}
	if got, want := dbExportText(dbExportTSV, flattened, dbExportDefaultOptions),
		"a\tb\nx y\tline break"; got != want {
		t.Fatalf("flattened TSV = %q, want %q", got, want)
	}
}

func TestDBExportJSONSQLAndMarkdown(t *testing.T) {
	data := dbExportTestData()
	data.Rows[0][1] = dbExportStringPointer("O'Hara <team>")
	numeric := dbExportNumericColumns([]openidea.DBColumn{
		{Name: "amount", TypeName: "NUMBER(38,0)"},
		{Name: "fixed_text", TypeName: "FixedString(10)"},
		{Name: "sample", TypeName: "unknown"},
	}, [][]*string{{
		dbExportStringPointer("1"),
		dbExportStringPointer("12"),
		dbExportStringPointer("2.5e+2"),
	}})
	if want := []bool{true, false, true}; !reflect.DeepEqual(numeric, want) {
		t.Fatalf("numeric columns = %v, want %v", numeric, want)
	}

	rawJSON := dbExportText(dbExportJSON, data, dbExportDefaultOptions)
	if strings.Contains(rawJSON, `\u003c`) {
		t.Fatalf("JSON escaped an HTML-looking value: %s", rawJSON)
	}
	var decoded []map[string]any
	if err := json.Unmarshal([]byte(rawJSON), &decoded); err != nil {
		t.Fatalf("JSON did not parse: %v\n%s", err, rawJSON)
	}
	wantJSON := []map[string]any{
		{"id": float64(1), "city": "O'Hara <team>"},
		{"id": float64(2), "city": nil},
	}
	if !reflect.DeepEqual(decoded, wantJSON) {
		t.Fatalf("JSON = %#v, want %#v", decoded, wantJSON)
	}

	data.Table = "order items"
	data.Driver = "postgres"
	wantSQL := "INSERT INTO \"order items\" (id, city) VALUES (1, 'O''Hara <team>');\n" +
		"INSERT INTO \"order items\" (id, city) VALUES (2, NULL);"
	if got := dbExportText(dbExportSQL, data, dbExportDefaultOptions); got != wantSQL {
		t.Fatalf("SQL = %q, want %q", got, wantSQL)
	}

	clickhouse := dbExportData{
		Columns: []openidea.DBColumn{{Name: "id", TypeName: "UInt64"}, {Name: "name"}},
		Rows:    [][]*string{{dbExportStringPointer("1"), dbExportStringPointer(`C:\new\file's`)}},
		Table:   "order items",
		Driver:  "clickhouse",
	}
	wantClickHouse := "INSERT INTO \"order items\" (id, name) VALUES (1, 'C:\\\\new\\\\file''s');"
	if got := dbExportText(dbExportSQL, clickhouse, dbExportDefaultOptions); got != wantClickHouse {
		t.Fatalf("ClickHouse SQL = %q, want %q", got, wantClickHouse)
	}

	snowflake := dbExportData{
		Columns: []openidea.DBColumn{
			{Name: "ID", TypeName: "NUMBER(38,0)"},
			{Name: "name", TypeName: "VARCHAR"},
		},
		Rows:   [][]*string{{dbExportStringPointer("1"), dbExportStringPointer(`C:\new\file's`)}},
		Table:  "ORDERS",
		Driver: "snowflake",
	}
	wantSnowflake := `INSERT INTO ORDERS (ID, "name") VALUES (1, 'C:\\new\\file''s');`
	if got := dbExportText(dbExportSQL, snowflake, dbExportDefaultOptions); got != wantSnowflake {
		t.Fatalf("Snowflake SQL = %q, want %q", got, wantSnowflake)
	}

	markdown := dbExportData{
		Columns: []openidea.DBColumn{
			{Name: "id", TypeName: "INTEGER"},
			{Name: "name", TypeName: "TEXT"},
		},
		Rows: [][]*string{{
			dbExportStringPointer("1"),
			dbExportStringPointer("a|b\nnext"),
		}},
	}
	wantMarkdown := "| id | name |\n| --: | --- |\n| 1 | a\\|b next |"
	if got := dbExportText(dbExportMarkdown, markdown, dbExportDefaultOptions); got != wantMarkdown {
		t.Fatalf("Markdown = %q, want %q", got, wantMarkdown)
	}
}

func TestDBExportOneRowPreviewAndQueryTable(t *testing.T) {
	data := dbExportTestData()
	if got, want := dbExportText(dbExportOneRow, data, dbExportDefaultOptions),
		"1, 'Shanghai', 2, NULL"; got != want {
		t.Fatalf("One-Row = %q, want %q", got, want)
	}

	large := dbExportData{
		Columns: []openidea.DBColumn{{Name: "payload", TypeName: "TEXT"}},
		Rows:    [][]*string{{dbExportStringPointer(strings.Repeat("😀", 501))}},
	}
	if got, want := dbExportCellPreview(*large.Rows[0][0]),
		strings.Repeat("😀", 500)+"…"; got != want {
		t.Fatalf("preview clipped a surrogate pair: %q", got)
	}

	rows := make([][]*string, dbExportPreviewAt+1)
	for rowIndex := range rows {
		rows[rowIndex] = []*string{dbExportStringPointer("1")}
	}
	preview := dbExportPreview(dbExportCSV, dbExportData{
		Columns: []openidea.DBColumn{{Name: "id", TypeName: "INTEGER"}},
		Rows:    rows,
	}, dbExportDefaultOptions)
	if !strings.HasSuffix(preview, "\n… 1 more rows") {
		t.Fatalf("preview did not report omitted rows: %q", preview)
	}

	for statement, want := range map[string]string{
		"SELECT * FROM people":                             "people",
		`SELECT * FROM "order items" AS orders`:            "order items",
		"SELECT * FROM main.people p":                      "main.people",
		"SELECT extract(year FROM created_at) FROM logs":   "logs",
		"WITH x AS (SELECT * FROM hidden) SELECT * FROM x": "x",
		"SELECT 1": "",
	} {
		if got := dbExportQueryTable(statement); got != want {
			t.Fatalf("table of %q = %q, want %q", statement, got, want)
		}
	}
}

func TestDBExportWritesFilesAndKeepsErrors(t *testing.T) {
	data := dbExportTestData()
	path := filepath.Join(t.TempDir(), "people.csv")
	m := &Model{db: &dbView{open: true, exportDialog: &dbExportDialog{
		open: true, data: data, format: dbExportCSV, options: dbExportDefaultOptions, file: path,
	}}}
	m.DBExportToFile()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(raw), "id,city\r\n1,Shanghai\r\n2,"; got != want {
		t.Fatalf("file = %q, want %q", got, want)
	}
	if m.db.exportDialog != nil {
		t.Fatal("the export dialog stayed open after writing")
	}

	parentFile := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(parentFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	dialog := &dbExportDialog{
		open: true, data: data, format: dbExportCSV, options: dbExportDefaultOptions,
		file: filepath.Join(parentFile, "out.csv"),
	}
	m.db.exportDialog = dialog
	m.DBExportToFile()
	if m.db.exportDialog != dialog {
		t.Fatal("a failed export closed its dialog")
	}
	if !strings.HasPrefix(dialog.err, "Cannot export data: ") {
		t.Fatalf("export error = %q", dialog.err)
	}
}

func TestDBExportActionClipboardAndDialog(t *testing.T) {
	m := &Model{db: &dbView{
		open:   true,
		source: "warehouse",
		sources: []openidea.DataSource{
			{ID: "lite", Name: "SQLite", Driver: "sqlite"},
			{ID: "warehouse", Name: "Warehouse", Driver: "snowflake"},
		},
		active: 0,
		results: []dbResult{{
			id:     1,
			title:  "SELECT * FROM customers",
			source: "lite",
			outcomes: []openidea.DBOutcome{{
				Statement: openidea.DBStatement{Text: "SELECT * FROM customers"},
				Result: &openidea.DBQueryResult{
					Columns: dbExportTestData().Columns,
					Rows:    dbExportTestData().Rows,
				},
			}},
		}},
	}}
	if !m.ActionEnabled("db.exportData") {
		t.Fatal("Export Data is disabled for a tabular result")
	}
	data, ok := m.dbExportDataForResult(0)
	if !ok || data.Driver != "sqlite" {
		t.Fatalf("the result lost its query-time driver: %+v, %v", data, ok)
	}
	m.DBExportAction(map[string]any{"format": dbExportJSON})
	if m.db.exportDialog == nil || m.db.exportDialog.format != dbExportJSON {
		t.Fatalf("Export Data dialog = %+v", m.db.exportDialog)
	}
	if m.db.exportDialog.source != "SQLite" ||
		!strings.HasSuffix(m.db.exportDialog.file, "customers.json") {
		t.Fatalf("dialog source/file = %q / %q", m.db.exportDialog.source, m.db.exportDialog.file)
	}
	overlay := overlays.byID["database.exportData"]
	if overlay == nil || !overlay.Modal || !overlay.Open(m) {
		t.Fatalf("database.exportData overlay = %+v", overlay)
	}
	m.dbExportSetFormat(m.db.exportDialog, dbExportTSV)
	if !strings.HasSuffix(m.db.exportDialog.file, "customers.tsv") {
		t.Fatalf("file did not follow the extractor: %q", m.db.exportDialog.file)
	}

	previousClipboard := dbExportClipboardWrite
	t.Cleanup(func() { dbExportClipboardWrite = previousClipboard })
	var copied string
	dbExportClipboardWrite = func(text string) { copied = text }
	m.db.exportDialog = nil
	m.DBExportCopy(map[string]any{"index": 0, "format": dbExportCSV})
	if copied != "id,city\r\n1,Shanghai\r\n2," {
		t.Fatalf("clipboard = %q", copied)
	}

	m.db.exportDialog = &dbExportDialog{
		open: true, data: dbExportTestData(), source: "SQLite", format: dbExportCSV,
		options: dbExportDefaultOptions, file: "/tmp/customers.csv",
		err: "Cannot export data: permission denied",
	}
	tester := ui.NewTester(func(c *ui.Context) { m.dbExportDialog(c, c.Theme()) }, 980, 720)
	tester.Frame()
	for _, label := range []string{
		"Export Data", "Source:", "Extractor:", "CSV",
		"Transpose", "Add column header", "Add row header", "Output file:",
		"Browse…", "Export preview:", "2 rows · CSV",
		"Cannot export data: permission denied", "Cancel", "Copy to Clipboard",
		"Export to File",
	} {
		if !tester.HasText(label) {
			t.Fatalf("missing %q from export dialog: %v", label, tester.Texts())
		}
	}
	if err := tester.Click("Add row header"); err != nil {
		t.Fatal(err)
	}
	if !m.db.exportDialog.options.RowHeader {
		t.Fatal("Add row header did not update the export options")
	}
	if err := tester.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	if m.db.exportDialog != nil {
		t.Fatal("Cancel kept the export dialog open")
	}
}
