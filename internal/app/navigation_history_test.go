package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui/editor"
)

func TestNavigationHistoryMergesCaretMovesAndTruncatesForward(t *testing.T) {
	now := time.Now()
	place := func(path string, line int) navigationLocation {
		return navigationLocation{path: path, pos: editor.Pos{Line: line}, at: now}
	}
	var history navigationHistory
	history.update(place("/p/A.java", 0), false)
	history.update(place("/p/A.java", 2), false)
	if len(history.entries) != 1 || history.entries[0].pos.Line != 2 {
		t.Fatalf("caret move did not replace the current place: %+v", history.entries)
	}
	history.update(place("/p/A.java", 10), true)
	history.update(place("/p/B.java", 4), true)
	if len(history.entries) != 3 || !history.canGoBack() {
		t.Fatalf("navigation entries = %+v index %d", history.entries, history.index)
	}
	if got, ok := history.back(); !ok || got.pos.Line != 10 {
		t.Fatalf("back = %+v, %v", got, ok)
	}
	if got, ok := history.forward(); !ok || got.path != "/p/B.java" {
		t.Fatalf("forward = %+v, %v", got, ok)
	}
	history.update(place("/p/C.java", 1), true)
	if history.canGoForward() || history.entries[len(history.entries)-1].path != "/p/C.java" {
		t.Fatalf("new navigation did not truncate forward history: %+v", history.entries)
	}
}

func TestNavigationHistoryDropsDeletedFilesAndDirectories(t *testing.T) {
	now := time.Now()
	var history navigationHistory
	history.update(navigationLocation{path: "/p/keep.java", pos: editor.Pos{Line: 0}, at: now}, false)
	history.update(navigationLocation{path: "/p/src/old/A.java", pos: editor.Pos{Line: 3}, at: now}, true)
	history.update(navigationLocation{path: "/p/src/B.java", pos: editor.Pos{Line: 5}, at: now}, true)
	history.update(navigationLocation{path: "/p/keep.java", pos: editor.Pos{Line: 8}, at: now}, true)
	if len(history.entries) != 4 {
		t.Fatalf("history entries = %+v", history.entries)
	}
	history.removeTree("/p/src", true)
	if len(history.entries) != 2 || history.entries[0].path != "/p/keep.java" ||
		history.entries[1].path != "/p/keep.java" {
		t.Fatalf("history after deleting /p/src = %+v", history.entries)
	}
	if !history.canGoBack() || history.canGoForward() {
		t.Fatalf("history after deleting /p/src has index %d of %d", history.index, len(history.entries))
	}

	var edits editHistory
	edits.record(navigationLocation{path: "/p/src/A.java", pos: editor.Pos{Line: 0}})
	edits.record(navigationLocation{path: "/p/A.java", pos: editor.Pos{Line: 10}})
	edits.record(navigationLocation{path: "/p/src/B.java", pos: editor.Pos{Line: 20}})
	edits.removeTree("/p/src", true)
	if len(edits.entries) != 1 || edits.entries[0].path != "/p/A.java" || edits.index != 1 {
		t.Fatalf("edit history after deleting /p/src = %+v index %d", edits.entries, edits.index)
	}
}

func TestEditHistoryWalksBackAndForward(t *testing.T) {
	var history editHistory
	history.record(navigationLocation{path: "/p/A.java", pos: editor.Pos{Line: 1}})
	history.record(navigationLocation{path: "/p/A.java", pos: editor.Pos{Line: 3}})
	history.record(navigationLocation{path: "/p/A.java", pos: editor.Pos{Line: 12}})
	if len(history.entries) != 2 || history.entries[0].pos.Line != 3 {
		t.Fatalf("merged edits = %+v", history.entries)
	}
	caret := history.entries[1]
	first, ok := history.back(&caret)
	if !ok || first.pos.Line != 3 {
		t.Fatalf("first back = %+v, %v", first, ok)
	}
	if _, ok := history.back(&first); ok {
		t.Fatal("walked before the oldest edit")
	}
	next, ok := history.forward()
	if !ok || next.pos.Line != 12 {
		t.Fatalf("next edit = %+v, %v", next, ok)
	}
	history.record(navigationLocation{path: "/p/B.java", pos: editor.Pos{Line: 8}})
	if history.index != len(history.entries) {
		t.Fatal("a new edit did not reset the walk")
	}
}

func TestNavigateBackAndForwardThroughTheActionRegistry(t *testing.T) {
	m := gotoLineTestModel("first\nsecond\nthird", 0)
	m.recordNavigation(true)
	m.tab().state.GoTo(editor.Pos{Line: 2, Col: 0})
	m.recordNavigation(true)
	if !m.ActionEnabled("navigate.back") {
		t.Fatal("Back is disabled with a previous location")
	}
	if !m.RunCommand("navigate.back") {
		t.Fatal("navigate.back is unavailable")
	}
	if got := m.tab().state.CursorPos().Line; got != 0 {
		t.Fatalf("Back moved to line %d", got)
	}
	if !m.ActionEnabled("navigate.forward") || !m.RunCommand("navigate.forward") {
		t.Fatal("navigate.forward is unavailable")
	}
	if got := m.tab().state.CursorPos().Line; got != 2 {
		t.Fatalf("Forward moved to line %d", got)
	}
}

func TestLastAndNextEditLocationUseTheEditHistory(t *testing.T) {
	m := gotoLineTestModel("first\nsecond\nthird\nfourth\nfifth\nsixth", 0)
	m.setRoot(t.TempDir())
	m.tab().state.GoTo(editor.Pos{Line: 1, Col: 0})
	place, _ := m.currentNavigationLocation(time.Now())
	m.recentNav.edits.record(place)
	m.tab().state.GoTo(editor.Pos{Line: 5, Col: 0})
	place, _ = m.currentNavigationLocation(time.Now())
	m.recentNav.edits.record(place)
	m.tab().state.GoTo(editor.Pos{Line: 0, Col: 0})
	if !m.ActionEnabled("navigate.lastEditLocation") || !m.RunCommand("navigate.lastEditLocation") {
		t.Fatal("navigate.lastEditLocation is unavailable")
	}
	if got := m.tab().state.CursorPos().Line; got != 5 {
		t.Fatalf("first Last Edit Location moved to line %d", got)
	}
	if !m.RunCommand("navigate.lastEditLocation") {
		t.Fatal("second navigate.lastEditLocation is unavailable")
	}
	if got := m.tab().state.CursorPos().Line; got != 1 {
		t.Fatalf("second Last Edit Location moved to line %d", got)
	}
	if !m.ActionEnabled("navigate.nextEditLocation") || !m.RunCommand("navigate.nextEditLocation") {
		t.Fatal("navigate.nextEditLocation is unavailable")
	}
	if got := m.tab().state.CursorPos().Line; got != 5 {
		t.Fatalf("Next Edit Location moved to line %d", got)
	}
}

func TestNavigationActionsHaveOpenIDEAIdentityAndBindings(t *testing.T) {
	m := &Model{}
	want := map[string]struct {
		title    string
		bindings []string
	}{
		"navigate.back":             {"Back", []string{"Cmd+[", "Alt+Cmd+Left", "Button4", "SwipeRight"}},
		"navigate.forward":          {"Forward", []string{"Cmd+]", "Alt+Cmd+Right", "Button5", "SwipeLeft"}},
		"navigate.lastEditLocation": {"Last Edit Location", []string{"Shift+Cmd+Backspace"}},
		"navigate.nextEditLocation": {"Next Edit Location", nil},
		"editor.copyReference":      {"Copy Reference", []string{"Alt+Shift+Cmd+C"}},
		"editor.addOrRemoveCaret":   {"Add or Remove Caret", []string{"Alt+Shift+Button1"}},
		"editor.copyPath":           {"Absolute Path", nil},
	}
	for id, check := range want {
		action, ok := actionGet(id)
		if !ok {
			t.Errorf("registry has no %s", id)
			continue
		}
		if action.Title != check.title {
			t.Errorf("%s title = %q, want %q", id, action.Title, check.title)
		}
		got := m.KeymapBindings(id)
		if len(got) != len(check.bindings) {
			t.Errorf("%s bindings = %v, want %v", id, got, check.bindings)
			continue
		}
		for i := range got {
			if got[i] != check.bindings[i] {
				t.Errorf("%s bindings = %v, want %v", id, got, check.bindings)
				break
			}
		}
	}
	if !hiddenActionIDs["editor.addOrRemoveCaret"] {
		t.Fatal("the pointer-only caret action appears in the command palette")
	}
}
