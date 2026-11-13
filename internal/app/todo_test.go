package app

import (
	"path/filepath"
	"testing"

	"mayoi/internal/openidea"
)

func todoModel() *Model {
	m := &Model{}
	m.resetTodo()
	return m
}

func TestTodoRegexFollowsPatterns(t *testing.T) {
	m := todoModel()
	if got, want := m.todoRegex(), `\b(TODO|FIXME|HACK|XXX)\b.*`; got != want {
		t.Fatalf("regex = %q, want %q", got, want)
	}
	m.todo.patterns[0].Enabled = false
	if got, want := m.todoRegex(), `\b(FIXME|HACK|XXX)\b.*`; got != want {
		t.Fatalf("regex without TODO = %q, want %q", got, want)
	}
	for i := range m.todo.patterns {
		m.todo.patterns[i].Enabled = false
	}
	if got := m.todoRegex(); got != "" {
		t.Fatalf("regex with every pattern off = %q, want empty", got)
	}
}

func TestLooksLikeComment(t *testing.T) {
	cases := []struct {
		prefix    string
		truncated bool
		want      bool
	}{
		{"", false, false},
		{"// ", false, true},
		{"\t/* ", false, true},
		{"<!-- ", false, true},
		{"# ", false, true},
		{"-- ", false, true},
		{"{- ", false, true},
		{"(* ", false, true},
		{" * ", false, true},
		{`; `, false, true},
		{`x + y `, false, false},
		{`url: https://`, false, false},
		{`x(* `, true, true},
		{`x = 1 `, true, false},
	}
	for _, c := range cases {
		if got := looksLikeComment(c.prefix, c.truncated); got != c.want {
			t.Errorf("looksLikeComment(%q, %v) = %v, want %v", c.prefix, c.truncated, got, c.want)
		}
	}
}

func TestTodoItemsFromBackendMatches(t *testing.T) {
	m := todoModel()
	text := "func x() { // TODO: fix }"
	// The backend range covers the whole regex match, from TODO to the end.
	fm := openidea.FileMatches{
		Path: "/p/a.go",
		Matches: []openidea.LineMatch{{
			Line:       3,
			Text:       text,
			Ranges:     [][2]uint32{{14, 23}},
			TextOffset: 0,
		}},
	}
	items := m.todoItemsFrom(fm)
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	it := items[0]
	if it.Path != "/p/a.go" || it.Line != 3 || it.Col != 14 {
		t.Fatalf("item = %+v, want path /p/a.go line 3 col 14", it)
	}
	if it.Keyword != "TODO" || it.Text != "TODO: fix" {
		t.Fatalf("keyword/text = %q/%q, want TODO/TODO: fix", it.Keyword, it.Text)
	}
}

func TestTodoItemsFromDropsNonComments(t *testing.T) {
	m := todoModel()
	fm := openidea.FileMatches{
		Path: "/p/a.go",
		Matches: []openidea.LineMatch{{
			Line:   0,
			Text:   `s := "TODO in a string"`,
			Ranges: [][2]uint32{{6, 10}},
		}},
	}
	if items := m.todoItemsFrom(fm); len(items) != 0 {
		t.Fatalf("items = %+v, want none", items)
	}
	m.todo.commentsOnly = false
	if items := m.todoItemsFrom(fm); len(items) != 1 {
		t.Fatalf("items without commentsOnly = %d, want 1", len(items))
	}
}

func TestTodoItemsFromCountsUTF16Columns(t *testing.T) {
	m := todoModel()
	text := "// 🎉 TODO: x"
	fm := openidea.FileMatches{
		Path: "/p/a.go",
		Matches: []openidea.LineMatch{{
			// The emoji is two UTF-16 code units, so TODO starts at unit 6.
			Line:   0,
			Text:   text,
			Ranges: [][2]uint32{{6, 11}},
		}},
	}
	items := m.todoItemsFrom(fm)
	if len(items) != 1 || items[0].Col != 6 {
		t.Fatalf("items = %+v, want one at column 6", items)
	}
}

func TestScanTodoText(t *testing.T) {
	m := todoModel()
	text := "package p\n\n// FIXME: broken\nvar s = \"TODO: not this\"\nurl := \"https://TODO.example\"\n\t* HACK: fix later\n"
	items := m.scanTodoText("/p/a.go", text)
	if len(items) != 2 {
		t.Fatalf("items = %+v, want FIXME and HACK", items)
	}
	if items[0].Keyword != "FIXME" || items[0].Line != 2 {
		t.Fatalf("first = %+v, want FIXME on line 2", items[0])
	}
	if items[1].Keyword != "HACK" || items[1].Line != 5 {
		t.Fatalf("second = %+v, want HACK on line 5", items[1])
	}
}

func TestScanTodoTextCaseSensitivity(t *testing.T) {
	m := todoModel()
	if items := m.scanTodoText("/p/a.go", "// todo: lower\n"); len(items) != 0 {
		t.Fatalf("case-sensitive items = %+v, want none", items)
	}
	m.todo.caseSensitive = false
	items := m.scanTodoText("/p/a.go", "// todo: lower\n")
	if len(items) != 1 || items[0].Keyword != "TODO" {
		t.Fatalf("case-insensitive items = %+v, want one TODO", items)
	}
}

func TestTodoGroupsFilterAndOrder(t *testing.T) {
	root := t.TempDir()
	a, b := filepath.Join(root, "a.go"), filepath.Join(root, "b.go")
	m := todoModel()
	m.todo.tab = "project"
	m.todo.items = map[string][]TodoItem{
		a: {{Path: a, Line: 4, Col: 3, Keyword: "TODO", Text: "TODO: two"},
			{Path: a, Line: 1, Col: 0, Keyword: "FIXME", Text: "FIXME: one"}},
		b: {{Path: b, Line: 0, Col: 0, Keyword: "TODO", Text: "TODO: b"}},
	}
	groups := m.todoGroups()
	if len(groups) != 2 || groups[0].path != a || groups[1].path != b {
		t.Fatalf("groups = %+v, want a.go then b.go", groups)
	}
	if groups[0].items[0].Line != 1 || groups[0].items[1].Line != 4 {
		t.Fatalf("a.go items = %+v, want line order", groups[0].items)
	}
	m.todo.hidden["FIXME"] = true
	groups = m.todoGroups()
	if len(groups[0].items) != 1 || groups[0].items[0].Keyword != "TODO" {
		t.Fatalf("filtered items = %+v, want only TODO", groups[0].items)
	}
	if got := m.todoTabCount("project"); got != 2 {
		t.Fatalf("tab count = %d, want 2", got)
	}
}

func TestTodoChangesTabFollowsVCS(t *testing.T) {
	root := t.TempDir()
	changed, unchanged := filepath.Join(root, "changed.go"), filepath.Join(root, "unchanged.go")
	m := todoModel()
	m.todo.tab = "changes"
	m.todo.items = map[string][]TodoItem{
		changed:   {{Path: changed, Line: 0, Keyword: "TODO", Text: "TODO: a"}},
		unchanged: {{Path: unchanged, Line: 0, Keyword: "TODO", Text: "TODO: b"}},
	}
	m.vcs.files = []openidea.FileStatus{statusOf(changed, "", "modified")}
	groups := m.todoGroups()
	if len(groups) != 1 || groups[0].path != realPath(changed) {
		t.Fatalf("groups = %+v, want only the changed file", groups)
	}
}

func TestTodoFileTabNeedsAnEditor(t *testing.T) {
	m := todoModel()
	m.todo.tab = "file"
	if groups := m.todoGroups(); len(groups) != 0 {
		t.Fatalf("groups = %+v, want none without an editor", groups)
	}
	if hint := m.todoEmptyHint(); hint != "没有打开的文件" {
		t.Fatalf("hint = %q, want 没有打开的文件", hint)
	}
}

func TestTodoSettingsDefaults(t *testing.T) {
	patterns, caseSensitive, commentsOnly := Preferences{}.todoSettings()
	if !caseSensitive || !commentsOnly || !sameTodoPatterns(patterns, defaultTodoPatterns) {
		t.Fatalf("defaults = %+v %v %v, want the built-in patterns", patterns, caseSensitive, commentsOnly)
	}
	// A stored Todo with no pattern list is the default set.
	patterns, _, _ = (&Preferences{Todo: &TodoPrefs{}}).todoSettings()
	if !sameTodoPatterns(patterns, defaultTodoPatterns) {
		t.Fatalf("null patterns = %+v, want the defaults", patterns)
	}
	// An empty list means the user removed every pattern, and stays empty.
	patterns, _, _ = (&Preferences{Todo: &TodoPrefs{Patterns: []TodoPattern{}}}).todoSettings()
	if patterns == nil || len(patterns) != 0 {
		t.Fatalf("empty patterns = %+v, want an empty list", patterns)
	}
	custom := []TodoPattern{{Keyword: "NOTE", Color: "success", Enabled: true}}
	prefs := Preferences{Todo: &TodoPrefs{Patterns: custom, IgnoreCase: true, AllText: true}}
	patterns, caseSensitive, commentsOnly = prefs.todoSettings()
	if caseSensitive || commentsOnly || !sameTodoPatterns(patterns, custom) {
		t.Fatalf("stored = %+v %v %v", patterns, caseSensitive, commentsOnly)
	}
}

func TestPreferencesTodoRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := (Preferences{Todo: &TodoPrefs{Patterns: []TodoPattern{}}}).Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}
	stored := LoadPreferences(path)
	if stored.Todo == nil || stored.Todo.Patterns == nil || len(stored.Todo.Patterns) != 0 {
		t.Fatalf("stored todo = %+v, want an empty pattern list", stored.Todo)
	}
	patterns, caseSensitive, commentsOnly := stored.todoSettings()
	if len(patterns) != 0 || !caseSensitive || !commentsOnly {
		t.Fatalf("settings = %+v %v %v", patterns, caseSensitive, commentsOnly)
	}
}

func TestResetTodoLoadsStoredSettings(t *testing.T) {
	m := &Model{prefs: Preferences{Todo: &TodoPrefs{
		Patterns:   []TodoPattern{{Keyword: "NOTE", Color: "success", Enabled: true}},
		IgnoreCase: true,
		AllText:    true,
	}}}
	m.resetTodo()
	if got, want := m.todoRegex(), `\b(NOTE)\b.*`; got != want {
		t.Fatalf("regex = %q, want %q", got, want)
	}
	if m.todo.caseSensitive || m.todo.commentsOnly {
		t.Fatalf("switches = %v %v, want both off", m.todo.caseSensitive, m.todo.commentsOnly)
	}
}

func TestSettingsTodoPageStagesAndApplies(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := &Model{cfg: Config{SettingsPath: path}}
	m.applyPreferences()
	m.resetTodo()
	m.OpenSettingsPage(settingsPageTodo)
	if m.settingsPage != settingsPageTodo {
		t.Fatalf("page = %q, want %q", m.settingsPage, settingsPageTodo)
	}
	if !sameTodoPatterns(m.settingsDraft.todoPatterns, defaultTodoPatterns) {
		t.Fatalf("draft = %+v, want the built-in patterns", m.settingsDraft.todoPatterns)
	}
	if m.settingsDirtyPage(settingsPageTodo) || m.settingsPending() {
		t.Fatalf("fresh page reported staged changes")
	}

	m.settingsSetTodoCase(false)
	m.settingsSetTodoComments(false)
	m.settingsSetTodoKeyword(3, "YY Y")
	if m.prefs.Todo != nil {
		t.Fatalf("staging wrote the prefs: %+v", m.prefs.Todo)
	}
	if !m.settingsDirtyPage(settingsPageTodo) || !m.settingsPending() {
		t.Fatalf("staged TODO did not dirty its page")
	}
	m.applySettings()
	if m.prefs.Todo == nil || !m.prefs.Todo.IgnoreCase || !m.prefs.Todo.AllText {
		t.Fatalf("applied prefs = %+v", m.prefs.Todo)
	}
	if len(m.prefs.Todo.Patterns) != 4 || m.prefs.Todo.Patterns[3].Keyword != "YYY" {
		t.Fatalf("stored patterns = %+v, want the whitespace stripped", m.prefs.Todo.Patterns)
	}
	if m.todo.caseSensitive || m.todo.commentsOnly {
		t.Fatalf("tool window kept %v %v, want both off", m.todo.caseSensitive, m.todo.commentsOnly)
	}
	if stored := LoadPreferences(path); stored.Todo == nil || !stored.Todo.IgnoreCase || !stored.Todo.AllText {
		t.Fatalf("file = %+v, want the applied TODO settings", stored.Todo)
	}

	m.settingsRestoreTodoPatterns()
	m.applySettings()
	if m.prefs.Todo != nil {
		t.Fatalf("restoring the defaults stored %+v", m.prefs.Todo)
	}
	if !m.todo.caseSensitive || !m.todo.commentsOnly {
		t.Fatalf("tool window = %v %v, want the defaults", m.todo.caseSensitive, m.todo.commentsOnly)
	}

	m.OpenSettingsPage(settingsPageTodo)
	for len(m.settingsDraft.todoPatterns) > 0 {
		m.settingsRemoveTodoPattern(0)
	}
	m.applySettings()
	stored := LoadPreferences(path)
	if stored.Todo == nil || stored.Todo.Patterns == nil || len(stored.Todo.Patterns) != 0 {
		t.Fatalf("stored empty patterns = %+v", stored.Todo)
	}
	if len(m.todo.patterns) != 0 || m.todoRegex() != "" {
		t.Fatalf("tool window kept patterns %+v", m.todo.patterns)
	}
}
