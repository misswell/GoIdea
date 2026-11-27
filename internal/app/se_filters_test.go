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

func TestSELanguageFilterUsesLongestExtensionSuffix(t *testing.T) {
	languages := []openidea.LanguageInfo{
		{ID: "typescript", Name: "TypeScript", Extensions: []string{"ts", "tsx"}},
		{ID: "typescript-definition", Name: "TypeScript Definition", Extensions: []string{"d.ts"}},
		{ID: "java", Name: "Java", Extensions: []string{".java"}},
	}
	cases := map[string]string{
		"/p/src/main.ts":            "typescript",
		"/p/src/view.tsx":           "typescript",
		"/p/types/index.d.ts":       "typescript-definition",
		"/p/src/App.java":           "java",
		"/p/README.md":              "",
		"/p/types/archive.d.ts.bak": "",
	}
	for path, want := range cases {
		if got := seLanguageOf(path, languages); got != want {
			t.Fatalf("seLanguageOf(%q) = %q, want %q", path, got, want)
		}
	}

	f := seFilters{}.toggleLanguage(seTabFiles, "java")
	if !f.active(seTabFiles) || !f.languageExcluded(seTabFiles, "/p/App.java", languages) {
		t.Fatalf("Java was not excluded from Files: %+v", f)
	}
	if f.languageExcluded(seTabFiles, "/p/main.ts", languages) ||
		f.languageExcluded(seTabClasses, "/p/App.java", languages) {
		t.Fatal("a language exclusion reached the wrong tab")
	}
	if got := f.clearTab(seTabFiles); got.active(seTabFiles) {
		t.Fatalf("Reset Filter left %+v", got)
	}
}

func TestSERowsApplyTheFunnel(t *testing.T) {
	m := &Model{}
	m.se = &sePane{open: true, tab: seTabClasses, query: "A", extra: map[seSection]int{}}
	m.se.languages = []openidea.LanguageInfo{
		{ID: "java", Name: "Java", Extensions: []string{"java"}},
		{ID: "kotlin", Name: "Kotlin", Extensions: []string{"kt"}},
	}
	m.se.symbols = []openidea.WorkspaceSymbol{
		{Name: "App", Kind: "class", Location: openidea.Location{Path: "/p/App.java"}},
		{Name: "Api", Kind: "interface", Location: openidea.Location{Path: "/p/Api.java"}},
	}
	m.se.filters.Classes = []string{"interface"}
	if n := m.seItemCount(m.seRows()); n != 1 {
		t.Fatalf("the excluded interface still counts: %d", n)
	}
	if rows := m.seRows(); len(rows) != 1 || rows[0].item.title != "App" {
		t.Fatalf("the wrong class row is listed: %+v", rows)
	}
	m.se.filters.Classes = nil
	m.se.filters.ClassesLanguages = []string{"java"}
	if n := m.seItemCount(m.seRows()); n != 0 {
		t.Fatalf("the excluded Java classes still count: %d", n)
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
	m.seFilters = seFilters{
		FilesLanguages: []string{"kotlin"}, Classes: []string{"enum"}, Text: "*.java",
	}
	m.saveSearchFilters()
	p := LoadPreferences(m.cfg.SettingsPath)
	if p.SearchFilters == nil || len(p.SearchFilters.FilesLanguages) != 1 ||
		len(p.SearchFilters.Classes) != 1 || p.SearchFilters.Text != "*.java" {
		t.Fatalf("the funnel did not persist: %+v", p.SearchFilters)
	}
}
