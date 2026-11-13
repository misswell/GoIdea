package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScratchFileName(t *testing.T) {
	if got := scratchFileName(nil, "md"); got != "scratch.md" {
		t.Fatalf("first name = %q, want scratch.md", got)
	}
	got := scratchFileName([]string{"scratch.md", "SCRATCH_1.md"}, "md")
	if got != "scratch_2.md" {
		t.Fatalf("next name = %q, want scratch_2.md", got)
	}
	if got := scratchFileName([]string{"scratch"}, ""); got != "scratch_1" {
		t.Fatalf("no extension name = %q, want scratch_1", got)
	}
}

func TestScratchContent(t *testing.T) {
	text, line, col := scratchContent("class X {\n  $END$\n}\n")
	if text != "class X {\n  \n}\n" || line != 1 || col != 2 {
		t.Fatalf("content = %q at %d:%d", text, line, col)
	}
	if text, line, col := scratchContent("abc"); text != "abc" || line != 0 || col != 0 {
		t.Fatalf("no caret = %q at %d:%d", text, line, col)
	}
}

func TestScratchLanguagesLocal(t *testing.T) {
	langs := scratchLanguagesLocal()
	if len(langs) < 10 {
		t.Fatalf("languages = %d, want the editor's lexer languages", len(langs))
	}
	byExt := make(map[string]bool, len(langs))
	foundJava := false
	for i, l := range langs {
		if l.ext == "" || l.name == "" {
			t.Fatalf("language %+v is missing a name or extension", l)
		}
		if byExt[l.ext] {
			t.Fatalf("extension %q listed twice", l.ext)
		}
		byExt[l.ext] = true
		if l.ext == "java" {
			foundJava = true
		}
		if i > 0 && strings.ToLower(langs[i-1].name) > strings.ToLower(l.name) {
			t.Fatalf("languages are not sorted: %q before %q", langs[i-1].name, l.name)
		}
	}
	if !foundJava {
		t.Fatalf("no Java in the scratch languages")
	}
}

func TestScratchRowsRecentAndFilter(t *testing.T) {
	m := &Model{prefs: Preferences{ScratchRecent: []string{"md"}}}
	m.scratch = &scratchChooser{languages: []scratchLanguage{
		{id: "java", name: "Java", ext: "java"},
		{id: "md", name: "Markdown", ext: "md"},
	}}
	rows := m.scratchRows()
	if len(rows) != 2 || rows[0].id != "md" || rows[1].id != "java" {
		t.Fatalf("rows = %+v, want Markdown first", rows)
	}
	m.SetScratchFilter("jav")
	if rows := m.scratchRows(); len(rows) != 1 || rows[0].id != "java" {
		t.Fatalf("filtered rows = %+v, want Java", rows)
	}
	m.SetScratchFilter("mkn")
	if rows := m.scratchRows(); len(rows) != 1 || rows[0].id != "md" {
		t.Fatalf("fuzzy rows = %+v, want Markdown", rows)
	}
	m.SetScratchFilter("zzz")
	if rows := m.scratchRows(); len(rows) != 0 {
		t.Fatalf("unmatched rows = %+v, want none", rows)
	}
	if m.scratch.active != 0 {
		t.Fatalf("active = %d, want the top row", m.scratch.active)
	}
}

func TestCreateScratchFile(t *testing.T) {
	dir := t.TempDir()
	m := &Model{cfg: Config{SettingsPath: filepath.Join(dir, "settings.json")}}
	lang := scratchLanguage{id: "md", name: "Markdown", ext: "md", template: "# Title\n$END$\n"}
	path, line, col, err := m.createScratchFile(lang)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if filepath.Base(path) != "scratch.md" || filepath.Dir(path) != filepath.Join(dir, "scratches") {
		t.Fatalf("path = %q, want <data>/scratches/scratch.md", path)
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != "# Title\n\n" {
		t.Fatalf("content = %q (%v), want the template without $END$", raw, err)
	}
	if line != 1 || col != 0 {
		t.Fatalf("caret = %d:%d, want 1:0", line, col)
	}
	second, _, _, err := m.createScratchFile(lang)
	if err != nil || filepath.Base(second) != "scratch_1.md" {
		t.Fatalf("second = %q (%v), want scratch_1.md", second, err)
	}
}

func TestRememberScratchLanguage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	m := &Model{cfg: Config{SettingsPath: path}, prefs: Preferences{ScratchRecent: []string{"java", "md"}}}
	m.rememberScratchLanguage("md")
	if got := m.prefs.ScratchRecent; len(got) != 2 || got[0] != "md" || got[1] != "java" {
		t.Fatalf("recent = %v, want md then java", got)
	}
	if stored := LoadPreferences(path); len(stored.ScratchRecent) != 2 || stored.ScratchRecent[0] != "md" {
		t.Fatalf("stored recent = %v", stored.ScratchRecent)
	}
	for _, id := range []string{"a", "b", "c", "d"} {
		m.rememberScratchLanguage(id)
	}
	if got := m.prefs.ScratchRecent; len(got) != maxScratchRecent || got[0] != "d" {
		t.Fatalf("recent = %v, want at most %d entries", got, maxScratchRecent)
	}
}
