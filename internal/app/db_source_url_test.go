package app

import (
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestDBParseSourceURL(t *testing.T) {
	testPassword := "test-" + "password"
	rawURL := "postgresql://ada:" + testPassword +
		"@db.example:5433/shop?sslmode=verify-full&application_name=idea"
	source, password, ok := dbParseSourceURL(rawURL)
	if !ok {
		t.Fatal("PostgreSQL URL was rejected")
	}
	if password != testPassword {
		t.Fatalf("password = %q", password)
	}
	for key, want := range map[string]any{
		"driver": "postgres", "host": "db.example", "port": "5433",
		"database": "shop", "user": "ada", "ssl": "verifyFull",
	} {
		if got := source[key]; got != want {
			t.Fatalf("%s = %#v, want %#v", key, got, want)
		}
	}
	wantProperties := []map[string]any{{"name": "application_name", "value": "idea"}}
	if !reflect.DeepEqual(source["properties"], wantProperties) {
		t.Fatalf("properties = %#v", source["properties"])
	}

	source, password, ok = dbParseSourceURL("jdbc:sqlite:shop.db")
	if !ok || password != "" || source["driver"] != "sqlite" ||
		source["file"] != "shop.db" || source["name"] != "shop" {
		t.Fatalf("SQLite URL = %#v password=%q ok=%v", source, password, ok)
	}
	if _, _, ok := dbParseSourceURL("not a connection URL"); ok {
		t.Fatal("invalid URL was accepted")
	}
}

func TestDBNewMenuShowsAllEntryPoints(t *testing.T) {
	m := &Model{db: &dbView{open: true, newMenu: &dbNewMenu{open: true}}}
	tester := ui.NewTester(func(c *ui.Context) { m.dbNewMenuPopup(c, c.Theme()) }, 500, 400)
	tester.Frame()
	for _, label := range []string{
		"Query Console", "Data Source…", "Data Source from File…",
		"Data Source from URL…", "Import from Clipboard",
	} {
		if !tester.HasText(label) {
			t.Fatalf("missing %q from New menu: %v", label, tester.Texts())
		}
	}
}
