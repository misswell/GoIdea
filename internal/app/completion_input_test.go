package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
)

func completionInputTestModel(
	path, text, prefix string,
	values map[string]any,
) (*Model, *Tab, editor.Completion) {
	doc := docbridge.Virtual(path, text)
	state := &editor.State{Buffer: doc.Buffer()}
	cursor := len(prefix)
	state.Select(cursor, cursor)
	item := editor.Completion{
		Label: prefix, Insert: prefix, FilterText: prefix,
	}
	state.ShowCompletions(0, []editor.Completion{item})
	tab := &Tab{doc: doc, state: state}
	tab.completion = completionTracking{
		version: doc.Buffer().Version(), cursor: cursor, start: 0,
		infos: map[string]completionInfo{}, selected: &item,
		open: true, generation: 1, explicit: true, focused: false,
		itemCount: 1, selectedIndex: 0, lastSelected: 0,
	}
	m := completionSettingsModel(values)
	m.tabs = []*Tab{tab}
	m.active = 0
	return m, tab, item
}

func TestCompletionSpecialKeyReflectsFocusedAndSetting(t *testing.T) {
	m, tab, _ := completionInputTestModel(
		"/project/Test.java", "fo", "fo",
		map[string]any{"editor.completion.insertOnSpecialKeys": false},
	)
	if m.completionInput(tab, ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyPeriod}) {
		t.Fatal("unfocused completion consumed the punctuation key")
	}
	if tab.state.Buffer.String() != "fo" {
		t.Fatalf("unfocused completion changed buffer to %q", tab.state.Buffer.String())
	}

	m, tab, _ = completionInputTestModel(
		"/project/Test.java", "fo", "fo",
		map[string]any{"editor.completion.insertOnSpecialKeys": true},
	)
	if m.completionInput(tab, ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyPeriod}) {
		t.Fatal("ordinary punctuation was consumed before native insertion")
	}
	if got := tab.state.Buffer.String(); got != "fo" {
		t.Fatalf("accepted completion buffer = %q, want selection applied", got)
	}
	if tab.completion.open {
		t.Fatal("completion remained open after a special key")
	}

	m, tab, _ = completionInputTestModel(
		"/project/Test.java", "fo", "fo",
		map[string]any{"editor.completion.insertOnSpecialKeys": false},
	)
	tab.completion.focused = true
	m.completionInput(tab, ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeySemicolon})
	if tab.completion.open {
		t.Fatal("focused lookup did not accept on a special key")
	}
}

func TestCompletionInputTextCommitsCharacterWhenNativeKeyDownIsAbsent(t *testing.T) {
	m, tab, _ := completionInputTestModel(
		"/project/Test.java", "fo", "fo",
		map[string]any{"editor.completion.insertOnSpecialKeys": true},
	)
	if !m.completionInput(tab, ui.InputEvent{Kind: ui.InputText, Text: "."}) {
		t.Fatal("InputText-only punctuation was not consumed")
	}
	if got := tab.state.Buffer.String(); got != "fo." {
		t.Fatalf("InputText completion buffer = %q, want accepted text plus punctuation", got)
	}
	if tab.completion.consumeText != "" {
		t.Fatalf("InputText punctuation left a stale consumed key: %q", tab.completion.consumeText)
	}
}

func TestCompletionParenConsumeTextSwallowsOneTypedParen(t *testing.T) {
	doc := docbridge.Virtual("/project/Test.java", "ru")
	state := &editor.State{Buffer: doc.Buffer()}
	state.Select(2, 2)
	item := editor.Completion{Label: "run", Insert: "run", FilterText: "ru"}
	kind := "method"
	detail := "void run()"
	lsp := &openidea.CompletionItem{Label: "run", Kind: &kind, Detail: &detail}
	state.ShowCompletions(0, []editor.Completion{item})
	tab := &Tab{doc: doc, state: state}
	tab.completion = completionTracking{
		version: doc.Buffer().Version(), cursor: 2, start: 0,
		infos: map[string]completionInfo{
			completionTrackKey(item): {LSP: lsp},
		},
		selected: &item, open: true, generation: 1, focused: true,
		itemCount: 1, selectedIndex: 0,
	}
	m := completionSettingsModel(map[string]any{
		"editor.completion.insertParentheses": true,
	})
	m.tabs = []*Tab{tab}
	m.active = 0

	if !m.completionInput(tab, ui.InputEvent{
		Kind: ui.InputKeyDown, Key: ui.Key9, Mods: ui.Shift, Text: "(",
	}) {
		t.Fatal("typed parenthesis was not consumed after inserting the call")
	}
	if got := tab.state.Buffer.String(); got != "run()" {
		t.Fatalf("completion call = %q, want run()", got)
	}
	if !m.completionInput(tab, ui.InputEvent{Kind: ui.InputText, Text: "("}) {
		t.Fatal("the platform's following InputText was not consumed")
	}
	if got := tab.state.Buffer.String(); got != "run()" {
		t.Fatalf("double parenthesis after completion = %q", got)
	}
}

func TestCompletionCallParensInsideAfterAndRejections(t *testing.T) {
	function := func(detail, text string, cursor int) (*Model, *Tab, editor.Completion, completionInfo) {
		doc := docbridge.Virtual("/project/Test.java", text)
		state := &editor.State{Buffer: doc.Buffer()}
		state.Select(cursor, cursor)
		item := editor.Completion{Label: "run", Insert: "run", FilterText: "ru"}
		kind := "method"
		lsp := &openidea.CompletionItem{Label: "run", Kind: &kind, Detail: &detail}
		tab := &Tab{doc: doc, state: state}
		tab.completion.start = 0
		m := completionSettingsModel(map[string]any{
			"editor.completion.insertParentheses": true,
		})
		return m, tab, item, completionInfo{LSP: lsp}
	}

	m, tab, item, info := function("void run(int value)", "ru", 2)
	if got := m.completionCallParens(tab, item, info); got != "inside" {
		t.Fatalf("parameter call parens = %q, want inside", got)
	}

	m, tab, item, info = function("void run()", "ru", 2)
	if got := m.completionCallParens(tab, item, info); got != "after" {
		t.Fatalf("parameterless call parens = %q, want after", got)
	}

	m, tab, item, info = function("void run(int value)", "ru世", 2)
	if got := m.completionCallParens(tab, item, info); got != "" {
		t.Fatalf("call parens before a Unicode identifier = %q, want none", got)
	}

	m, tab, item, info = function("void run(int value)", "import ru", len("import ru"))
	tab.completion.start = len("import ")
	if got := m.completionCallParens(tab, item, info); got != "" {
		t.Fatalf("call parens on an import line = %q, want none", got)
	}
}

func TestCompletionDocumentationTimerUsesAutomaticPopupSetting(t *testing.T) {
	m, tab, _ := completionInputTestModel(
		"/project/Test.java", "fo", "fo",
		map[string]any{"editor.completion.showDocumentation": false},
	)
	tab.completion.selected.Documentation = "docs"
	m.scheduleCompletionDocumentation(tab)
	if tab.completion.documentation != nil {
		t.Fatal("documentation timer started with the automatic popup disabled")
	}

	m, tab, _ = completionInputTestModel(
		"/project/Test.java", "fo", "fo",
		map[string]any{
			"editor.completion.showDocumentation":  true,
			"editor.completion.documentationDelay": 30,
		},
	)
	tab.completion.selected.Documentation = "docs"
	m.scheduleCompletionDocumentation(tab)
	if tab.completion.documentation == nil {
		t.Fatal("documentation timer did not start with the automatic popup enabled")
	}
	tab.completion.documentation.Stop()
}

func TestCompletionAutoInsertSingleRequiresCurrentExplicitSingleResult(t *testing.T) {
	t.Run("current", func(t *testing.T) {
		m, tab, _ := completionInputTestModel(
			"/project/Test.java", "fo", "fo", nil,
		)
		post := make(chan func(), 2)
		m.postHook = func(fn func()) { post <- fn }
		m.scheduleSingleCompletion(tab)
		if tab.completion.autoTimer == nil {
			t.Fatal("single explicit completion did not schedule auto-insert")
		}
		deadline := time.After(2 * time.Second)
		for tab.completion.open {
			select {
			case <-deadline:
				t.Fatal("auto-insert timed out")
			case fn := <-post:
				fn()
			}
		}
	})

	t.Run("stale version", func(t *testing.T) {
		m, tab, _ := completionInputTestModel(
			"/project/Test.java", "fo", "fo", nil,
		)
		post := make(chan func(), 2)
		m.postHook = func(fn func()) { post <- fn }
		m.scheduleSingleCompletion(tab)
		tab.state.Buffer.Edit(editor.Edit{Start: 3, End: 3, Text: "X"})
		select {
		case fn := <-post:
			fn()
		case <-time.After(2 * time.Second):
			t.Fatal("stale auto-insert timer did not fire")
		}
		if got := tab.state.Buffer.String(); got != "foX" {
			t.Fatalf("stale auto-insert changed buffer to %q", got)
		}
	})

	t.Run("two results", func(t *testing.T) {
		m, tab, _ := completionInputTestModel(
			"/project/Test.java", "fo", "fo", nil,
		)
		tab.completion.itemCount = 2
		m.scheduleSingleCompletion(tab)
		if tab.completion.autoTimer != nil {
			t.Fatal("two completion results scheduled auto-insert")
		}
	})
}
