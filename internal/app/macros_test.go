package app

import (
	"encoding/json"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

func newMacroTestModel(t *testing.T) *Model {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settings.json")
	m := &Model{
		cfg:      Config{SettingsPath: path},
		postHook: func(fn func()) { fn() },
	}
	m.ensureMacros()
	t.Cleanup(func() {
		for _, action := range actionList() {
			if strings.HasPrefix(action.ID, macroPlayActionPrefix) {
				unregisterAction(action.ID)
			}
		}
	})
	return m
}

func TestMacroRecordingMergesTextKeepsKeysAndPersistsNamedMacros(t *testing.T) {
	m := newMacroTestModel(t)
	var ran string
	if err := registerAction(&Action{
		ID: "test.macro.action", Title: "Macro Action",
		RunWithArg: func(_ *Model, arg any) {
			ran = arg.(map[string]any)["path"].(string)
		},
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { unregisterAction("test.macro.action") })

	m.startMacroRecording()
	m.recordMacroEditorInput(ui.InputEvent{Kind: ui.InputText, Text: "he"})
	m.recordMacroEditorInput(ui.InputEvent{Kind: ui.InputText, Text: "llo"})
	m.recordMacroEditorInput(ui.InputEvent{
		Kind: ui.InputKeyDown, Key: ui.KeyEnter,
	})
	if !m.RunActionWithArg("test.macro.action", map[string]any{"path": "/project/a.go"}) ||
		ran != "/project/a.go" {
		t.Fatalf("recorded action did not run: %q", ran)
	}
	m.recordMacroEditorInput(ui.InputEvent{
		Kind: ui.InputKeyDown, Key: ui.KeyLeft, Mods: ui.Alt | ui.Shift,
	})
	m.stopMacroRecording()

	want := []MacroStep{
		{Kind: "text", Text: "hello"},
		{Kind: "key", Key: "Enter"},
		{Kind: "action", ID: "test.macro.action", Arg: map[string]any{"path": "/project/a.go"}},
		{Kind: "key", Key: "Alt+Shift+ArrowLeft"},
	}
	if !reflectMacroStepsEqual(m.macros.last, want) {
		t.Fatalf("last macro = %#v, want %#v", m.macros.last, want)
	}
	if !m.macros.nameOpen {
		t.Fatal("stopping a non-empty recording did not ask for a name")
	}
	m.macros.nameDraft = "Greeting"
	m.finishMacroNaming(true)

	stored := LoadPreferences(m.cfg.SettingsPath)
	if len(stored.Macros.Saved) != 1 || stored.Macros.Saved[0].Name != "Greeting" ||
		!reflectMacroStepsEqual(stored.Macros.Saved[0].Steps, want) {
		t.Fatalf("stored macros = %#v", stored.Macros)
	}
	action, ok := actionGet("macros.play.Greeting")
	if !ok || action.Title != "Greeting" || !m.ActionEnabled(action.ID) {
		t.Fatalf("saved macro action = %+v, %v", action, ok)
	}
}

func TestMacroRecordingExcludesMacroActionsAndUnstorableArguments(t *testing.T) {
	m := newMacroTestModel(t)
	m.startMacroRecording()
	m.RunAction("macro.stopRecording")
	if m.macros.recordingActive {
		t.Fatal("Stop Macro Recording was recorded instead of stopping")
	}
	if len(m.macros.last) != 0 {
		t.Fatalf("empty recording became %#v", m.macros.last)
	}

	m.startMacroRecording()
	m.recordMacroAction("file.openPaths", []string{"/a", "/b"})
	m.recordMacroAction("file.openPaths", map[string]any{"paths": []string{"/a", "/b"}})
	m.recordMacroAction("file.openPaths", map[string]any{"nested": map[string]any{"bad": true}})
	m.stopMacroRecording()
	if len(m.macros.last) != 2 {
		t.Fatalf("storable action arguments = %#v", m.macros.last)
	}
}

func TestMacroPlaybackRunsInOrderAndStopsAtTheFirstFailure(t *testing.T) {
	m := newMacroTestModel(t)
	buffer := editor.NewBuffer("abc")
	bufferState := &editor.State{Buffer: buffer}
	bufferState.Select(3, 3)
	m.tabs = []*Tab{{state: bufferState}}
	m.active = 0

	var gotArg any
	if err := registerAction(&Action{
		ID: "test.macro.pass", Title: "Pass",
		RunWithArg: func(_ *Model, arg any) { gotArg = arg },
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { unregisterAction("test.macro.pass") })
	if err := registerAction(&Action{
		ID: "test.macro.fail", Title: "Fail",
		When: func(*Model) bool { return false },
		Run:  func(*Model) {},
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { unregisterAction("test.macro.fail") })

	ok := m.playMacroSteps([]MacroStep{
		{Kind: "action", ID: "test.macro.pass", Arg: map[string]any{"path": "/a"}},
		{Kind: "text", Text: "X"},
		{Kind: "key", Key: "Backspace"},
		{Kind: "action", ID: "test.macro.fail"},
	}, "Playback")
	if ok {
		t.Fatal("playback reported success after a step failed")
	}
	if gotArg.(map[string]any)["path"] != "/a" {
		t.Fatalf("playback argument = %#v", gotArg)
	}
	if got := bufferState.Buffer.String(); got != "abc" {
		t.Fatalf("playback text/key result = %q", got)
	}
	if m.macros.playing {
		t.Fatal("playback left the playing guard set")
	}
	if len(m.notify.items) != 1 || m.notify.items[0].title != "Macro 'Playback' stopped" {
		t.Fatalf("playback notification = %+v", m.notify.items)
	}
}

func TestMacroRenameDeleteAndPreferencesReload(t *testing.T) {
	m := newMacroTestModel(t)
	m.saveMacro("Copy", []MacroStep{{Kind: "action", ID: "editor.copyPath"}})
	m.saveMacro("Comment", []MacroStep{{Kind: "action", ID: "code.commentLine"}})
	m.macros.editSelection = 0
	m.openRenameMacro()
	m.macros.renameDraft = "Comment"
	if got := m.macroRenameError(); got != "A macro with this name exists" {
		t.Fatalf("duplicate rename error = %q", got)
	}
	m.macros.renameDraft = "Copy Path"
	m.commitRenameMacro()
	if _, ok := actionGet("macros.play.Copy"); ok {
		t.Fatal("renaming left the old saved-macro action registered")
	}
	if _, ok := actionGet("macros.play.Copy Path"); !ok {
		t.Fatal("renaming did not register the new saved-macro action")
	}

	m.macros.editSelection = 1
	m.deleteSelectedMacro()
	if _, ok := actionGet("macros.play.Comment"); ok {
		t.Fatal("deleting left the saved-macro action registered")
	}
	stored := LoadPreferences(m.cfg.SettingsPath)
	if len(stored.Macros.Saved) != 1 || stored.Macros.Saved[0].Name != "Copy Path" {
		t.Fatalf("stored macros after rename/delete = %#v", stored.Macros.Saved)
	}

	reloaded := &Model{prefs: stored}
	reloaded.ensureMacros()
	if len(reloaded.macros.saved) != 1 || reloaded.macros.saved[0].Name != "Copy Path" {
		t.Fatalf("reloaded macros = %#v", reloaded.macros.saved)
	}
}

func TestMacroArgumentsAndEditorKeysMatchOpenIDEA(t *testing.T) {
	cases := []struct {
		value any
		want  any
		ok    bool
	}{
		{"value", "value", true},
		{true, true, true},
		{2, float64(2), true},
		{[]string{"a", "b"}, []string{"a", "b"}, true},
		{[]any{"a", "b"}, []string{"a", "b"}, true},
		{[]int{1}, nil, false},
		{map[string]any{"path": "/a", "paths": []string{"/a", "/b"}},
			map[string]any{"path": "/a", "paths": []string{"/a", "/b"}}, true},
		{map[string]any{"nested": map[string]any{"a": 1}}, nil, false},
		{math.Inf(1), nil, false},
	}
	for _, test := range cases {
		got, ok := storableMacroArg(test.value)
		if ok != test.ok || !reflectMacroValueEqual(got, test.want) {
			t.Errorf("storableMacroArg(%#v) = %#v, %t, want %#v, %t",
				test.value, got, ok, test.want, test.ok)
		}
	}

	keyCases := []struct {
		event ui.InputEvent
		want  string
	}{
		{ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyEnter}, "Enter"},
		{ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyLeft, Mods: ui.Alt | ui.Shift},
			"Alt+Shift+ArrowLeft"},
		{ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyHome, Mods: ui.Ctrl | ui.Shift},
			"Ctrl+Shift+Home"},
		{ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyA}, ""},
	}
	for _, test := range keyCases {
		got, ok := macroEditorKeyName(test.event)
		if got != test.want || ok != (test.want != "") {
			t.Errorf("macroEditorKeyName(%+v) = %q, %t, want %q", test.event, got, ok, test.want)
		}
	}
}

func TestMacrosNativeStatusDialogsAndMenu(t *testing.T) {
	m := newMacroTestModel(t)
	m.startMacroRecording()
	item := statusItemByID("macros.recording")
	if item == nil || !item.Visible(m) {
		t.Fatal("the recording status item is not visible")
	}
	tt := ui.NewTester(func(c *ui.Context) {
		item.Render(m, c, c.Theme(), oiPaletteOf(c.Theme()))
	}, 500, 80)
	tt.SetDark(true)
	tt.Frame()
	if !tt.HasText("Recording macro…") {
		t.Fatalf("recording status = %v", tt.Texts())
	}
	if err := tt.Click("Recording macro…"); err != nil {
		t.Fatal(err)
	}
	if m.macros.recordingActive {
		t.Fatal("clicking the recording status did not stop the macro")
	}

	m.saveMacro("Copy", []MacroStep{
		{Kind: "action", ID: "editor.copyPath"},
		{Kind: "text", Text: "a\nb"},
		{Kind: "key", Key: "Enter"},
	})
	m.openEditMacros()
	tt = ui.NewTester(func(c *ui.Context) { m.renderOverlays(c, c.Theme()) }, 1000, 700)
	tt.SetDark(true)
	tt.Frame()
	for _, text := range []string{"Edit Macros", "Copy", `Type "a↵b"`, "Press Enter"} {
		if !tt.HasText(text) {
			t.Fatalf("Edit Macros is missing %q: %v", text, tt.Texts())
		}
	}
	if err := tt.Click("Rename…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("Rename Macro") {
		t.Fatalf("Rename Macro did not open: %v", tt.Texts())
	}
	m.macros.renameDraft = "Duplicate"
	m.commitRenameMacro()
	tt.Frame()
	if !tt.HasText("Duplicate") {
		t.Fatalf("renamed macro is not in the editor: %v", tt.Texts())
	}
	m.macros.editOpen = false
	m.openPlaySavedMacros()
	tt.Frame()
	if !tt.HasText("Play Saved Macro") || !tt.HasText("Duplicate") {
		t.Fatalf("Play Saved Macros did not render: %v", tt.Texts())
	}

	entries := macroMenuEntries(m)
	var labels []string
	for _, entry := range entries {
		if entry.label != "" {
			labels = append(labels, entry.label)
		}
	}
	if got := strings.Join(labels, ","); !strings.Contains(got, "Play Saved Macros…") ||
		!strings.Contains(got, "Edit Macros…") {
		t.Fatalf("Macros menu = %v", labels)
	}
}

func statusItemByID(id string) *StatusItem {
	for _, item := range statusItems.items {
		if item.ID == id {
			return item
		}
	}
	return nil
}

func reflectMacroStepsEqual(left, right []MacroStep) bool {
	return reflectMacroValueEqual(left, right)
}

func reflectMacroValueEqual(left, right any) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && string(leftJSON) == string(rightJSON)
}
