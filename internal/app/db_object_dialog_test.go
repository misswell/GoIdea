package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

func TestDBTableFormRendersAndAddsColumn(t *testing.T) {
	form := &dbTableForm{
		open: true, creating: true, source: "lite", schema: "main", name: "people",
		nextKey: 2,
		columns: []dbColumnDraft{{
			key: 1, name: "id", dataType: "INTEGER", notNull: true, primaryKey: true,
		}},
		selected: 1,
	}
	m := &Model{db: &dbView{
		open: true, sources: []openidea.DataSource{{ID: "lite", Driver: "sqlite"}},
		tableForm: form,
	}}
	tester := ui.NewTester(func(c *ui.Context) { m.dbTableFormDialog(c, c.Theme()) }, 900, 700)
	tester.Frame()
	for _, label := range []string{
		"Create Table", "Table:", "Preview", "id", "Primary key", "OK", "Cancel",
	} {
		if !tester.HasText(label) {
			t.Fatalf("missing %q from table form: %v", label, tester.Texts())
		}
	}
	if err := tester.Click("+"); err != nil {
		t.Fatal(err)
	}
	if len(form.columns) != 2 || form.columns[1].name != "column_2" || form.selected != form.columns[1].key {
		t.Fatalf("added column = %#v", form)
	}
}

func TestDBImportFormRendersOptionsAndPreview(t *testing.T) {
	form := &dbImportForm{
		open: true, source: "lite", schema: "main", table: "people",
		file: "people.csv", separator: "Comma", header: true, emptyAsNull: true,
		columns: []string{"id", "name"},
		text:    "name\nAda\n",
	}
	m := &Model{db: &dbView{
		open: true, sources: []openidea.DataSource{{ID: "lite", Driver: "sqlite"}},
		importForm: form,
	}}
	m.refreshDBImportForm()
	tester := ui.NewTester(func(c *ui.Context) { m.dbImportFormDialog(c, c.Theme()) }, 900, 700)
	tester.Frame()
	for _, label := range []string{
		"Import Data into main.people", "Separator:", "Comma",
		"First row is header (columns matched by name)", "Treat empty values as NULL",
		"Columns: name", "Import 1 Rows", "Cancel",
	} {
		if !tester.HasText(label) {
			t.Fatalf("missing %q from import form: %v", label, tester.Texts())
		}
	}
}
