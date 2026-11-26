package app

import (
	"testing"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
)

func gutterSettingsModel(values map[string]any) *Model {
	m := &Model{}
	m.nativeSettings.pages = map[string]nativeSettingsPageState{
		settingsPageGutterIcons: {
			loaded:  true,
			applied: values,
			draft:   values,
		},
	}
	return m
}

func editorCodeEditingModel(values map[string]any) *Model {
	page := nativeSettingsPageState{
		loaded: true, scope: "global", applied: values, draft: map[string]any{},
	}
	return &Model{nativeSettings: nativeSettingsCatalogState{
		pages: map[string]nativeSettingsPageState{settingsPageCodeEditing: page},
	}}
}

func TestGutterMasterAndGroupSwitches(t *testing.T) {
	m := gutterSettingsModel(map[string]any{
		"editor.showGutterIcons":     true,
		"editor.gutterIconsDisabled": []any{"run", "bookmarks"},
	})
	if !m.gutterIconsShown() {
		t.Fatal("show gutter icons was ignored")
	}
	for group, want := range map[string]bool{
		"run": false, "bookmarks": false, "colors": true,
	} {
		if got := m.gutterGroupEnabled(group); got != want {
			t.Fatalf("gutter group %q enabled = %v, want %v", group, got, want)
		}
	}

	m = gutterSettingsModel(map[string]any{"editor.showGutterIcons": false})
	if m.gutterGroupEnabled("colors") {
		t.Fatal("group remained enabled with the gutter master switch off")
	}
}

func TestCodeEditingHighlightsBracesWordsAndSelectionOccurrences(t *testing.T) {
	doc := docbridge.Virtual("/Demo.go", "func f(value int) {\n\tvalue := value\n}")
	state := &editor.State{Buffer: doc.Buffer()}
	state.Select(7, 12)
	tab := &Tab{doc: doc, state: state}
	m := editorCodeEditingModel(map[string]any{
		"editor.codeEditing.highlightUsages":    true,
		"editor.codeEditing.highlightBraces":    true,
		"editor.codeEditing.highlightSelection": true,
	})
	backgrounds := m.editorTextBackgrounds(tab)
	if len(backgrounds) < 2 {
		t.Fatalf("highlights = %+v", backgrounds)
	}
	m.setNativeSettingsValue(settingsPageCodeEditing, "editor.codeEditing.highlightBraces", false)
	m.setNativeSettingsValue(settingsPageCodeEditing, "editor.codeEditing.highlightSelection", false)
	m.nativeSettingsMu.Lock()
	page := m.nativeSettings.pages[settingsPageCodeEditing]
	page.applied["editor.codeEditing.highlightBraces"] = false
	page.applied["editor.codeEditing.highlightSelection"] = false
	m.nativeSettings.pages[settingsPageCodeEditing] = page
	m.nativeSettingsMu.Unlock()
	if got := m.selectionOccurrenceBackgrounds(tab); len(got) != 0 {
		t.Fatalf("selection occurrences with switch off = %+v", got)
	}
}

func TestColorPreviewGutterMarksRespectGutterGroups(t *testing.T) {
	doc := docbridge.Virtual(
		"/project/style.css",
		"#fff\nrgb(1, 2, 3)\n#12345678\nnot-a-color\n",
	)
	tab := &Tab{doc: doc}
	m := &Model{}
	marks := m.colorPreviewGutterMarks(tab, nil)
	if len(marks) != 3 || marks[0].Line != 0 || marks[1].Line != 1 || marks[2].Line != 2 {
		t.Fatalf("color preview marks = %+v, want three color lines", marks)
	}

	taken := map[int]bool{0: true}
	marks = m.colorPreviewGutterMarks(tab, taken)
	if len(marks) != 2 || marks[0].Line != 1 {
		t.Fatalf("color preview marks with a taken line = %+v", marks)
	}

	m = gutterSettingsModel(map[string]any{
		"editor.gutterIconsDisabled": []any{"colors"},
	})
	if marks := m.colorPreviewGutterMarks(tab, nil); len(marks) != 0 {
		t.Fatalf("disabled color marks = %+v", marks)
	}
}

func TestBookmarkGutterDotsRespectGutterGroups(t *testing.T) {
	m := &Model{
		bookmarks: []Bookmark{{Path: "/project/Test.java", Line: 2}},
	}
	marks := m.bookmarkMarks("/project/Test.java", nil)
	if len(marks) != 1 || marks[0].Line != 2 {
		t.Fatalf("bookmark gutter marks = %+v", marks)
	}
	if marks := m.bookmarkMarks("/project/Test.java", map[int]bool{2: true}); len(marks) != 0 {
		t.Fatalf("taken bookmark line kept a duplicate: %+v", marks)
	}
	m = gutterSettingsModel(map[string]any{
		"editor.gutterIconsDisabled": []any{"bookmarks"},
	})
	m.bookmarks = []Bookmark{{Path: "/project/Test.java", Line: 2}}
	if marks := m.bookmarkMarks("/project/Test.java", nil); len(marks) != 0 {
		t.Fatalf("disabled bookmark marks = %+v", marks)
	}
}

func TestPluginStructureMarkersRespectGutterGroupsIndependently(t *testing.T) {
	m, tab := newPluginVisualTestModel(t, "implemented\noverriding\ngeneric")
	m.nativeSettings.pages = map[string]nativeSettingsPageState{
		settingsPageGutterIcons: {
			loaded: true,
			applied: map[string]any{
				"editor.gutterIconsDisabled": []any{"inheritors"},
			},
		},
	}
	version := tab.doc.Buffer().Version()
	backendVersion := tab.doc.Version()
	implemented, overriding := "implemented", "overriding"
	m.plugins.visual.lineResults[tab.doc.ID()] = pluginLineMarkerResult{
		LocalVersion:   version,
		BackendVersion: backendVersion,
		Markers: []openidea.PluginLineMarker{
			{Line: 0, Icon: &implemented, Tooltip: "Implemented method"},
			{Line: 1, Icon: &overriding, Tooltip: "Overriding method"},
			{Line: 2, Tooltip: "Plugin marker"},
		},
	}

	marks := m.pluginGutterMarks(tab)
	lines := map[int]bool{}
	for _, mark := range marks {
		lines[mark.Line] = true
	}
	if len(marks) != 2 || !lines[1] || !lines[2] {
		t.Fatalf("gutter marks with inheritors disabled = %+v, want overriding and generic", marks)
	}
	setNativeAppliedTestValue(
		m,
		settingsPageGutterIcons,
		"editor.gutterIconsDisabled",
		[]string{"supers"},
	)
	marks = m.pluginGutterMarks(tab)
	lines = map[int]bool{}
	for _, mark := range marks {
		lines[mark.Line] = true
	}
	if len(marks) != 2 || !lines[0] || !lines[2] {
		t.Fatalf("gutter marks with supers disabled = %+v, want implemented and generic", marks)
	}
	setNativeAppliedTestValue(m, settingsPageGutterIcons, "editor.showGutterIcons", false)
	if marks := m.pluginGutterMarks(tab); len(marks) != 1 || marks[0].Line != 2 {
		t.Fatalf("master gutter switch changed the independent plugin marker: %+v", marks)
	}
}

func smartPasteTestModel(values map[string]any) (*Model, *Tab) {
	doc := docbridge.Virtual("/project/Test.java", "class Test {\n    \n}\n")
	doc.SetReadOnly(false)
	tab := &Tab{doc: doc, state: &editor.State{Buffer: doc.Buffer()}}
	m := &Model{}
	m.nativeSettings.pages = map[string]nativeSettingsPageState{
		settingsPageSmartKeys: {
			loaded: true, applied: values, draft: values,
		},
	}
	return m, tab
}

func TestEditorPasteReformatsWithBothSmartKeyModes(t *testing.T) {
	const caret = 17
	for _, tc := range []struct {
		mode string
		text string
		want string
	}{
		{
			mode: "indentBlock",
			text: "  call();\n      next();",
			want: "class Test {\n    call();\n        next();\n}\n",
		},
		{
			mode: "indentEachLine",
			text: "call();\n      next();\n  last();",
			want: "class Test {\n    call();\n    next();\n    last();\n}\n",
		},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			m, tab := smartPasteTestModel(map[string]any{
				"editor.smartKeys.reformatOnPaste": tc.mode,
				"editor.smartKeys.escapeOnPaste":   false,
			})
			tab.state.Select(caret, caret)
			if !m.editorPaste(tab, tc.text) {
				t.Fatal("reformat-on-paste did not consume the paste")
			}
			if got := tab.doc.Text(); got != tc.want {
				t.Fatalf("paste result:\n%s\nwant:\n%s", got, tc.want)
			}
		})
	}
}

func TestEditorPasteEscapesOnlyInsidePlainStringLiterals(t *testing.T) {
	doc := docbridge.Virtual("/project/Test.java", `var value = "";`)
	doc.SetReadOnly(false)
	tab := &Tab{doc: doc, state: &editor.State{Buffer: doc.Buffer()}}
	m := &Model{}
	m.nativeSettings.pages = map[string]nativeSettingsPageState{
		settingsPageSmartKeys: {
			loaded: true,
			applied: map[string]any{
				"editor.smartKeys.reformatOnPaste": "none",
				"editor.smartKeys.escapeOnPaste":   true,
			},
		},
	}
	tab.state.Select(13, 13)
	if !m.editorPaste(tab, "a\"b\n\t\\") {
		t.Fatal("string escaping did not consume the paste")
	}
	if got, want := tab.doc.Text(), `var value = "a\"b\n\t\\";`; got != want {
		t.Fatalf("escaped paste = %q, want %q", got, want)
	}

	doc = docbridge.Virtual("/project/Test.java", `value();`)
	doc.SetReadOnly(false)
	tab = &Tab{doc: doc, state: &editor.State{Buffer: doc.Buffer()}}
	tab.state.Select(6, 6)
	if m.editorPaste(tab, `a"b`) {
		t.Fatal("outside a string the paste should keep the editor's default")
	}
	if got := tab.doc.Text(); got != `value();` {
		t.Fatalf("a default paste changed the document directly: %q", got)
	}
}
