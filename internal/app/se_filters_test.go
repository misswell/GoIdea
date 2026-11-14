package app

import (
	"path/filepath"
	"testing"

	"mayoi/internal/openidea"
)

func TestSEFilterKindsToggleAndBucket(t *testing.T) {
	f := seFilters{}
	if f.active(seTabSymbols) || f.kindExcluded(seTabSymbols, "function") {
		t.Fatal("an empty filter drops something")
	}
	f = f.toggleKind(seTabSymbols, "function")
	if !f.active(seTabSymbols) || !f.kindExcluded(seTabSymbols, "function") {
		t.Fatalf("the kind was not excluded: %+v", f)
	}
	if f.kindExcluded(seTabClasses, "function") {
		t.Fatal("the Symbols filter reached Classes")
	}
	f = f.toggleKind(seTabSymbols, "function")
	if f.kindExcluded(seTabSymbols, "function") {
		t.Fatalf("the kind was not included again: %+v", f)
	}

	g := seFilters{}.toggleKind(seTabClasses, "module")
	if !g.kindExcluded(seTabClasses, "package") {
		t.Fatal("package is not bucketed under module")
	}
	if !(seFilters{}).toggleKind(seTabClasses, "typeAlias").kindExcluded(seTabClasses, "typeParameter") {
		t.Fatal("typeParameter is not bucketed under typeAlias")
	}
	g = g.clearTab(seTabClasses)
	if g.active(seTabClasses) {
		t.Fatalf("Reset Filter left %+v", g)
	}
}

func TestSETextMaskMatching(t *testing.T) {
	cases := []struct {
		mask, path string
		want       bool
	}{
		{"", "/p/A.java", true},
		{"*.java", "/p/A.java", true},
		{"*.java", "/p/A.kt", false},
		{"!*.test.*", "/p/A.java", true},
		{"!*.test.*", "/p/A.test.java", false},
		{"*.java, *.kt", "/p/A.kt", true},
		{"*.java, !A.java", "/p/A.java", false},
		{"*.java, !A.java", "/p/B.java", true},
		{"src/*.java", "/p/src/A.java", false},
	}
	for _, c := range cases {
		if got := seTextMaskMatches(c.mask, c.path); got != c.want {
			t.Fatalf("mask %q on %q = %v, want %v", c.mask, c.path, got, c.want)
		}
	}
}

func TestSERowsApplyTheFunnel(t *testing.T) {
	m := &Model{}
	m.se = &sePane{open: true, tab: seTabClasses, query: "A", extra: map[seSection]int{}}
	m.se.symbols = []openidea.WorkspaceSymbol{
		{Name: "App", Kind: "class", Location: openidea.Location{Path: "/p/App.java"}},
		{Name: "Api", Kind: "interface", Location: openidea.Location{Path: "/p/Api.java"}},
	}
	m.se.filters.Classes = []string{"interface"}
	if n := m.seItemCount(m.seRows()); n != 1 {
		t.Fatalf("the excluded interface still counts: %d", n)
	}
	if rows := m.seRows(); len(rows) < 2 || rows[1].item.title != "App" {
		t.Fatalf("the wrong class row is listed: %+v", rows)
	}

	m.se.tab = seTabText
	m.se.text = []openidea.FileMatches{
		{Path: "/p/App.java", RelPath: "App.java", Matches: []openidea.LineMatch{{Line: 0, Text: "App"}}},
		{Path: "/p/App.kt", RelPath: "App.kt", Matches: []openidea.LineMatch{{Line: 0, Text: "App"}}},
	}
	m.se.filters.Text = "*.java"
	if n := m.seItemCount(m.seRows()); n != 1 {
		t.Fatalf("the text mask did not drop the .kt hit: %d", n)
	}
}

func TestSearchFiltersPersist(t *testing.T) {
	m := &Model{}
	m.cfg.SettingsPath = filepath.Join(t.TempDir(), "settings.json")
	m.seFilters = seFilters{Classes: []string{"enum"}, Text: "*.java"}
	m.saveSearchFilters()
	p := LoadPreferences(m.cfg.SettingsPath)
	if p.SearchFilters == nil || len(p.SearchFilters.Classes) != 1 || p.SearchFilters.Text != "*.java" {
		t.Fatalf("the funnel did not persist: %+v", p.SearchFilters)
	}
}
