package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

func usageTestLocation(path string, line, col, endLine, endCol uint32) openidea.Location {
	return openidea.Location{
		Path: path,
		Range: openidea.OutlineRange{
			Start: openidea.OutlinePos{Line: line, Col: col},
			End:   openidea.OutlinePos{Line: endLine, Col: endCol},
		},
	}
}

func usageTestClassification(
	location openidea.Location, kind, access, text string,
) openidea.ClassifiedReference {
	return openidea.ClassifiedReference{
		Location: location, Kind: kind, Access: access, Text: usageStringPointer(text),
	}
}

func TestBuildUsagePopupRowsSortsDeduplicatesAndKeepsSemantics(t *testing.T) {
	first := usageTestLocation("/p/B.java", 4, 2, 4, 8)
	second := usageTestLocation("/p/A.java", 1, 3, 2, 7)
	text := "b.setName(\"x\")"
	rows := buildUsagePopupRows(
		[]openidea.Location{first, second, first},
		[]openidea.ClassifiedReference{
			usageTestClassification(first, "setter", "write", text),
			usageTestClassification(second, "getter", "read", "multi\nline"),
		},
		nil,
	)
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if rows[0].source.Path != "/p/A.java" || rows[0].kind != "getter" ||
		rows[0].access != "read" || rows[0].endCol != utf16Length("multi\nline") {
		t.Fatalf("first row = %+v", rows[0])
	}
	if rows[1].source.Path != "/p/B.java" || rows[1].kind != "setter" ||
		rows[1].access != "write" || rows[1].text != text {
		t.Fatalf("second row = %+v", rows[1])
	}
}

func TestUsageAccessFiltersKeepUnknownAndReadWrite(t *testing.T) {
	prefs := UsagesPrefs{HideRead: true}
	if usageAccessVisible("read", prefs) {
		t.Fatal("read row survived the read filter")
	}
	for _, access := range []string{"write", "readWrite", "unknown", ""} {
		if !usageAccessVisible(access, prefs) {
			t.Fatalf("%q row was hidden", access)
		}
	}
	prefs = UsagesPrefs{HideWrite: true}
	if usageAccessVisible("write", prefs) {
		t.Fatal("write row survived the write filter")
	}
	if !usageAccessVisible("read", prefs) {
		t.Fatal("read row was hidden by the write filter")
	}
}

func TestToggleUsageAccessNeverHidesBoth(t *testing.T) {
	m := &Model{}
	m.toggleUsageAccess("read")
	if m.prefs.Usages.ShowReadAccess() || !m.prefs.Usages.ShowWriteAccess() {
		t.Fatalf("first toggle = %+v", m.prefs.Usages)
	}
	m.toggleUsageAccess("write")
	if m.prefs.Usages.HideRead || !m.prefs.Usages.HideWrite {
		t.Fatalf("turning off the last mode did not restore the other: %+v", m.prefs.Usages)
	}
}

func TestUsagePrefsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	want := UsagesPrefs{PopupPreview: true, NoFindPreview: true, HideWrite: true}
	if err := (Preferences{Usages: want}).Save(path); err != nil {
		t.Fatal(err)
	}
	got := LoadPreferences(path).Usages
	if got != want {
		t.Fatalf("usages = %+v, want %+v", got, want)
	}
}

func TestUsagePopupFilterNavigationAndEscape(t *testing.T) {
	m := &Model{}
	m.usagePopup = &usagePopupState{
		open: true, active: 0,
		rows: []usagePopupRow{
			{source: usageTestLocation("/p/A.java", 0, 0, 0, 3), text: "foo(order)"},
			{source: usageTestLocation("/p/A.java", 1, 0, 1, 3), text: "foo(cart)"},
			{source: usageTestLocation("/p/B.java", 0, 0, 0, 3), text: "foo(order, 2)"},
		},
	}
	if got := len(m.usagePopupVisibleRows()); got != 3 {
		t.Fatalf("unfiltered rows = %d", got)
	}
	m.usagePopup.filter = "ORDER"
	if got := len(m.usagePopupVisibleRows()); got != 2 {
		t.Fatalf("line-text rows = %d, want 2", got)
	}
	m.usagePopup.filter = "b.java"
	if got := len(m.usagePopupVisibleRows()); got != 1 {
		t.Fatalf("file-name rows = %d, want 1", got)
	}
	m.usagePopup.filter = ""
	m.usagePopup.listScroll.Y = 40
	m.stepUsagePopup(ui.KeyDown)
	if m.usagePopup.active != 1 {
		t.Fatalf("down = %d", m.usagePopup.active)
	}
	m.stepUsagePopup(ui.KeyUp)
	m.stepUsagePopup(ui.KeyUp)
	if m.usagePopup.active != len(m.usagePopupVisibleRows())-1 {
		t.Fatalf("wrapped up = %d", m.usagePopup.active)
	}
	m.usagePopup.filter = "cart"
	m.escapeUsagePopup()
	if m.usagePopup == nil || m.usagePopup.filter != "" || !m.usagePopup.open {
		t.Fatalf("first Escape = %+v", m.usagePopup)
	}
	m.escapeUsagePopup()
	if m.usagePopup != nil {
		t.Fatal("second Escape did not close Show Usages")
	}
}

func TestEditorCaretAnchorUsesScrolledLineTop(t *testing.T) {
	var text strings.Builder
	for i := 0; i < 300; i++ {
		text.WriteString("value++\n")
	}
	state := &editor.State{
		Buffer:     editor.NewBuffer(text.String()),
		Font:       ui.Font{Size: 13},
		LineHeight: editorLineHeight(13),
	}
	var bounds ui.Rect
	tester := ui.NewTester(func(c *ui.Context) {
		bounds = editor.View(c, state).Fill().Bounds()
	}, 600, 320)
	tester.Frame()
	state.GoTo(editor.Pos{Line: 240, Col: 3})
	tester.Frame()
	if state.ScrollY() == 0 {
		t.Fatal("the test editor did not scroll")
	}
	anchor := editorCaretAnchor(bounds, state, 13)
	wantY := bounds.Y + float32(state.LineTop(240)-state.ScrollY())
	if diff := anchor.Y - wantY; diff < -0.5 || diff > 0.5 {
		t.Fatalf("caret anchor y = %.2f, want %.2f (LineTop minus scroll)", anchor.Y, wantY)
	}
}

func TestUsagePopupTransfersExactResultsToFindWindow(t *testing.T) {
	location := openidea.Location{
		Path: "/Bean.java",
		Range: openidea.OutlineRange{
			Start: openidea.OutlinePos{Line: 1, Col: 0},
			End:   openidea.OutlinePos{Line: 2, Col: 10},
		},
	}
	m := &Model{}
	m.usagePopup = &usagePopupState{
		open: true, symbol: "value", note: "Results may be incomplete",
		rows: []usagePopupRow{{
			source: location, kind: "field", access: "readWrite", text: "value", endCol: 10,
		}},
	}
	m.OpenUsageResultsInFindWindow()
	if m.usagePopup != nil {
		t.Fatal("popup stayed open after the transfer")
	}
	if !m.usages.done || m.usages.symbol != "value" || len(m.usages.locs) != 1 ||
		m.usages.locs[0] != location {
		t.Fatalf("Find Usages = %+v", m.usages)
	}
	if len(m.usages.classified) != 1 || m.usages.classified[0].Access != "readWrite" ||
		m.usages.classified[0].Kind != "field" ||
		m.usages.classified[0].Text == nil || *m.usages.classified[0].Text != "value" {
		t.Fatalf("classified transfer = %+v", m.usages.classified)
	}
	if !m.toolVisible("usages") {
		t.Fatal("the Find Usages tool window did not open")
	}
}

func TestShowUsagesPopupLoadsStreamedResults(t *testing.T) {
	m := gotoLineTestModel("class A { void run() { used(); used(); } }", 10)
	m.ctx = context.Background()
	m.postHook = func(fn func()) { fn() }
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		if command == "cancel_request" {
			return json.RawMessage(`null`), nil
		}
		if command != "lsp_references" {
			t.Fatalf("unexpected command %q", command)
		}
		input := args.(map[string]any)
		if input["libraries"] != false {
			t.Fatalf("initial libraries = %v", input["libraries"])
		}
		refs := openidea.References{
			Locations: []openidea.Location{
				usageTestLocation("/test.java", 0, 24, 0, 30),
				usageTestLocation("/test.java", 0, 32, 0, 38),
			},
			LibrariesSkipped: true,
		}
		raw, _ := json.Marshal(refs)
		return raw, nil
	}
	m.ShowUsagesPopup()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if state := m.usagePopup; state != nil && state.done && state.open {
			if len(state.rows) != 2 || state.scope != usageScopeProject {
				t.Fatalf("popup state = %+v", state)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("popup did not load: %+v", m.usagePopup)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestUsagePopupKeepsScopeChooserAbsentWithoutScopedSearch(t *testing.T) {
	m := gotoLineTestModel("used();", 1)
	m.ctx = context.Background()
	m.postHook = func(fn func()) { fn() }
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		if command == "cancel_request" {
			return json.RawMessage(`null`), nil
		}
		refs := openidea.References{Locations: []openidea.Location{
			usageTestLocation("/test.java", 0, 0, 0, 4),
			usageTestLocation("/test.java", 0, 7, 0, 11),
		}}
		raw, _ := json.Marshal(refs)
		return raw, nil
	}
	m.ShowUsagesPopup()
	waitUsagePopup(t, m, func(state *usagePopupState) bool {
		return state.done && state.open
	})
	if m.usagePopup.scope != "" {
		t.Fatalf("unscoped search exposed a scope chooser: %q", m.usagePopup.scope)
	}
}

func TestShowUsagesPopupWidensTheSameTarget(t *testing.T) {
	m := gotoLineTestModel("used();", 1)
	m.ctx = context.Background()
	m.postHook = func(fn func()) { fn() }
	var searches atomic.Int32
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		switch command {
		case "cancel_request":
			return json.RawMessage(`null`), nil
		case "lsp_references":
			n := searches.Add(1)
			refs := openidea.References{
				Locations: []openidea.Location{
					usageTestLocation("/test.java", 0, 0, 0, 4),
					usageTestLocation("/test.java", 0, 7, 0, 11),
				},
				LibrariesSkipped: n == 1,
			}
			raw, _ := json.Marshal(refs)
			return raw, nil
		default:
			t.Fatalf("unexpected command %q", command)
			return nil, nil
		}
	}
	m.ShowUsagesPopup()
	waitUsagePopup(t, m, func(state *usagePopupState) bool {
		return state.done && state.open && state.scope == usageScopeProject
	})
	m.ShowUsagesPopup()
	waitUsagePopup(t, m, func(state *usagePopupState) bool {
		return state.done && state.scope == usageScopeLibraries
	})
	if searches.Load() != 2 {
		t.Fatalf("searches = %d, want 2", searches.Load())
	}
}

func TestUsagePopupStreamsPartialBeforeFinal(t *testing.T) {
	m := &Model{ctx: context.Background()}
	m.postHook = func(fn func()) { fn() }
	state := &usagePopupState{
		open: false, loading: true, active: 0, channel: 9, generation: 1,
	}
	m.usagePopup = state
	raw := json.RawMessage(`{
		"locations":[{"path":"/p/A.java","range":{"start":{"line":0,"col":0},"end":{"line":0,"col":3}}}],
		"classified":[{"location":{"path":"/p/A.java","range":{"start":{"line":0,"col":0},"end":{"line":0,"col":3}}},"kind":"unknown","text":"foo()"}],
		"incomplete":null
	}`)
	m.acceptUsagePopup(9, raw)
	if state.open {
		t.Fatal("the first partial opened the popup without the 300ms grace period")
	}
	deadline := time.Now().Add(time.Second)
	for !state.open && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !state.open || !state.loading || !state.early || len(state.rows) != 1 {
		t.Fatalf("streamed popup = %+v", state)
	}
}

func TestUsagePopupRendersAndSpeedSearches(t *testing.T) {
	m := &Model{}
	m.usagePopup = &usagePopupState{
		open: true, symbol: "foo", scope: usageScopeProject,
		rows: []usagePopupRow{
			{
				source: usageTestLocation("/p/A.java", 2, 0, 2, 3),
				kind:   "unknown", text: "foo(order)",
			},
			{
				source: usageTestLocation("/p/A.java", 5, 0, 5, 3),
				kind:   "unknown", text: "foo(cart)",
			},
			{
				source: usageTestLocation("/p/B.java", 1, 0, 1, 3),
				kind:   "unknown", text: "foo(order, 2)",
			},
		},
	}
	tester := ui.NewTester(func(c *ui.Context) {
		m.usagesPopup(c, c.Theme())
	}, 1100, 700)
	tester.Frame()
	for _, text := range []string{"Usages of foo", "3 found", "Open in Find Window", "Project Files"} {
		if !tester.HasText(text) {
			t.Fatalf("%q is absent: %q", text, tester.Texts())
		}
	}
	if !tester.HasText("Open in Find Window Alt+F7") {
		t.Fatalf("Open in Find Window has the wrong shortcut: %q", tester.Texts())
	}
	tester.Type(" ")
	if m.usagePopup.filter != "" {
		t.Fatalf("an initial space started speed search: %q", m.usagePopup.filter)
	}
	tester.Type("cart")
	if m.usagePopup.filter != "cart" {
		t.Fatalf("typed filter = %q", m.usagePopup.filter)
	}
	tester.Frame()
	if !tester.HasText("1 of 3 found") || !tester.HasText("foo(cart)") {
		t.Fatalf("filtered popup = %q", tester.Texts())
	}
	tester.Key(0, ui.KeyEscape)
	tester.Frame()
	if m.usagePopup == nil || m.usagePopup.filter != "" {
		t.Fatalf("first Escape = %+v", m.usagePopup)
	}
	tester.Key(0, ui.KeyEscape)
	tester.Frame()
	if m.usagePopup != nil {
		t.Fatal("second Escape did not close the popup")
	}
}

func TestUsagePopupPreviewReadsSurroundingSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Example.java")
	var lines []string
	for i := 0; i < 50; i++ {
		lines = append(lines, "line")
	}
	lines[25] = "    used();"
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}
	start, got, err := readUsagePreview(path, 25, 20, 20)
	if err != nil {
		t.Fatal(err)
	}
	if start != 5 || len(got) != 41 || got[20] != "    used();" {
		t.Fatalf("preview start=%d lines=%d active=%q", start, len(got), got[min(20, len(got)-1)])
	}
}

func TestUsagePopupPreviewLoadIsDebounced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Example.java")
	if err := os.WriteFile(path, []byte("class Example {\n    used();\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := &Model{postHook: func(fn func()) { fn() }}
	m.prefs.Usages.PopupPreview = true
	m.usagePopup = &usagePopupState{
		open: true,
		rows: []usagePopupRow{{
			source: usageTestLocation(path, 1, 4, 1, 8),
			text:   "    used();",
		}},
	}
	defer stopUsagePopupPreview(m.usagePopup)
	m.refreshUsagePopupPreview()
	time.Sleep(20 * time.Millisecond)
	if !m.usagePopup.preview.loading || len(m.usagePopup.preview.lines) != 0 {
		t.Fatalf("preview loaded before its debounce: %+v", m.usagePopup.preview)
	}
	deadline := time.Now().Add(2 * time.Second)
	for m.usagePopup.preview.loading && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if m.usagePopup.preview.loading || m.usagePopup.preview.err != "" {
		t.Fatalf("preview did not load: %+v", m.usagePopup.preview)
	}
	active := m.usagePopup.preview.line - m.usagePopup.preview.start
	if active < 0 || active >= len(m.usagePopup.preview.lines) ||
		m.usagePopup.preview.lines[active] != "    used();" {
		t.Fatalf("preview lines = %+v", m.usagePopup.preview)
	}
}

func waitUsagePopup(t *testing.T, m *Model, done func(*usagePopupState) bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if state := m.usagePopup; state != nil && done(state) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("usage popup did not reach its state: %+v", m.usagePopup)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
