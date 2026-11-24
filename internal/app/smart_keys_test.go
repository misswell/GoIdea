package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
)

func smartKeyTestModel(path, text string, values map[string]any) (*Model, *Tab) {
	doc := docbridge.Virtual(path, text)
	state := &editor.State{Buffer: doc.Buffer(), TabSize: 4}
	tab := &Tab{doc: doc, state: state}
	m := &Model{tabs: []*Tab{tab}, active: 0}
	if len(values) > 0 {
		m.nativeSettings.pages = map[string]nativeSettingsPageState{
			settingsPageSmartKeys: {
				loaded:  true,
				applied: values,
				draft:   values,
			},
		}
	}
	return m, tab
}

func TestSmartHomeDisableMovesToPhysicalLineStart(t *testing.T) {
	m, tab := smartKeyTestModel(
		"/project/Test.java", "  alpha\n", map[string]any{"editor.smartKeys.smartHome": false},
	)
	tab.state.Select(5, 5)
	if !m.smartKeyInput(tab, ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyHome}) {
		t.Fatal("Home was not handled with smart Home disabled")
	}
	if got := tab.state.Selection(); got.Anchor != 0 || got.Head != 0 {
		t.Fatalf("Home selection = %+v, want physical line start", got)
	}

	tab.state.Select(5, 5)
	if !m.smartKeyInput(tab, ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyHome, Mods: ui.Shift}) {
		t.Fatal("Shift+Home was not handled with smart Home disabled")
	}
	if got := tab.state.Selection(); got.Anchor != 5 || got.Head != 0 {
		t.Fatalf("Shift+Home selection = %+v, want 5..0", got)
	}
}

func TestCamelHumpsSwitchChoosesWholeOrSubwordMovement(t *testing.T) {
	m, tab := smartKeyTestModel(
		"/project/Test.java", "fooBar", map[string]any{"editor.smartKeys.camelHumps": false},
	)
	tab.state.Select(6, 6)
	m.smartKeyInput(tab, ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyLeft, Mods: ui.Alt})
	if got := tab.state.Cursor(); got != 0 {
		t.Fatalf("word movement caret = %d, want 0", got)
	}

	m, tab = smartKeyTestModel(
		"/project/Test.java", "fooBar", map[string]any{"editor.smartKeys.camelHumps": true},
	)
	tab.state.Select(6, 6)
	m.smartKeyInput(tab, ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyLeft, Mods: ui.Alt})
	if got := tab.state.Cursor(); got != 3 {
		t.Fatalf("subword movement caret = %d, want 3", got)
	}
}

func TestSmartPairsSurroundStepOverAndTabOutRespectSettings(t *testing.T) {
	m, tab := smartKeyTestModel("/project/Test.java", "", nil)
	if !m.smartKeyInput(tab, ui.InputEvent{Kind: ui.InputText, Text: "("}) {
		t.Fatal("opening bracket was not paired")
	}
	if got := tab.state.Buffer.String(); got != "()" {
		t.Fatalf("paired buffer = %q, want parentheses", got)
	}
	if got := tab.state.Cursor(); got != 1 {
		t.Fatalf("paired caret = %d, want inside", got)
	}
	if !m.smartKeyInput(tab, ui.InputEvent{Kind: ui.InputText, Text: ")"}) {
		t.Fatal("matching closer did not step over")
	}
	if got := tab.state.Buffer.String(); got != "()" || tab.state.Cursor() != 2 {
		t.Fatalf("step-over buffer/caret = %q/%d", tab.state.Buffer.String(), tab.state.Cursor())
	}

	m, tab = smartKeyTestModel("/project/Test.java", "", nil)
	m.smartKeyInput(tab, ui.InputEvent{Kind: ui.InputText, Text: "["})
	if !m.smartKeyInput(tab, ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyTab}) {
		t.Fatal("Tab did not leave the paired bracket")
	}
	if got := tab.state.Buffer.String(); got != "[]" || tab.state.Cursor() != 2 {
		t.Fatalf("Tab-out buffer/caret = %q/%d", tab.state.Buffer.String(), tab.state.Cursor())
	}

	m, tab = smartKeyTestModel(
		"/project/Test.java", "hello",
		map[string]any{"editor.smartKeys.surroundSelection": true},
	)
	tab.state.Select(0, 5)
	if !m.smartKeyInput(tab, ui.InputEvent{Kind: ui.InputText, Text: "{"}) {
		t.Fatal("selection was not surrounded")
	}
	selection := tab.state.Selection()
	if got := tab.state.Buffer.String(); got != "{hello}" {
		t.Fatalf("surrounded buffer = %q", got)
	}
	if selection.Anchor != 1 || selection.Head != 6 {
		t.Fatalf("surrounded selection = %+v, want original text", selection)
	}

	m, tab = smartKeyTestModel(
		"/project/Test.java", "",
		map[string]any{"editor.smartKeys.pairBrackets": false},
	)
	if m.smartKeyInput(tab, ui.InputEvent{Kind: ui.InputText, Text: "("}) {
		t.Fatal("pairing was handled while disabled")
	}
	if got := tab.state.Buffer.String(); got != "" {
		t.Fatalf("disabled pair changed buffer to %q", got)
	}
}

func TestSmartPairTracksNativeTextInsertion(t *testing.T) {
	m, tab := smartKeyTestModel("/project/Test.java", "", nil)
	m.smartKeyInput(tab, ui.InputEvent{Kind: ui.InputText, Text: "("})
	if m.smartKeyInput(tab, ui.InputEvent{Kind: ui.InputText, Text: "x"}) {
		t.Fatal("ordinary text was consumed before native insertion")
	}
	tab.state.Buffer.Edit(editor.Edit{Start: 1, End: 1, Text: "x"})
	tab.state.Select(2, 2)
	if m.smartKeyInput(tab, ui.InputEvent{Kind: ui.InputText, Text: "y"}) {
		t.Fatal("ordinary text was consumed")
	}
	if len(tab.smart.pairs) != 1 || tab.smart.pairs[0].close != 3 {
		t.Fatalf("tracked pair scope = %+v, want close offset 3", tab.smart.pairs)
	}
}

func TestSmartEnterInsertsDocStubAndHonorsSmartIndent(t *testing.T) {
	m, tab := smartKeyTestModel("/project/Test.java", "/**", nil)
	tab.state.Select(3, 3)
	if !m.smartKeyInput(tab, ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyEnter}) {
		t.Fatal("documentation stub was not inserted")
	}
	if got, want := tab.state.Buffer.String(), "/**\n * \n */"; got != want {
		t.Fatalf("doc stub = %q, want %q", got, want)
	}
	if got := tab.state.Cursor(); got != len("/**\n * ") {
		t.Fatalf("doc stub caret = %d", got)
	}

	m, tab = smartKeyTestModel(
		"/project/Test.java", "    value",
		map[string]any{"editor.smartKeys.smartIndent": false},
	)
	tab.state.Select(len("    value"), len("    value"))
	if !m.smartKeyInput(tab, ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyEnter}) {
		t.Fatal("plain newline was not inserted")
	}
	if got, want := tab.state.Buffer.String(), "    value\n    "; got != want {
		t.Fatalf("plain newline = %q, want %q", got, want)
	}
}

func TestSmartBackspaceNoneAndNearest(t *testing.T) {
	m, tab := smartKeyTestModel(
		"/project/Test.java", "a😀",
		map[string]any{"editor.smartKeys.backspaceUnindent": "none"},
	)
	tab.state.Select(len("a😀"), len("a😀"))
	if !m.smartKeyInput(tab, ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyBackspace}) {
		t.Fatal("ordinary Unicode backspace was not handled")
	}
	if got := tab.state.Buffer.String(); got != "a" {
		t.Fatalf("backspace result = %q, want one rune deleted", got)
	}

	m, tab = smartKeyTestModel(
		"/project/Test.java", "        value",
		map[string]any{"editor.smartKeys.backspaceUnindent": "nearest"},
	)
	tab.state.Select(8, 8)
	if !m.smartKeyInput(tab, ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyBackspace}) {
		t.Fatal("nearest unindent was not handled")
	}
	if got := tab.state.Buffer.String(); got != "    value" || tab.state.Cursor() != 4 {
		t.Fatalf("nearest unindent = %q at %d", got, tab.state.Cursor())
	}

}
