package app

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/egoist/mygo/ui/editor"
	"mayoi/internal/openidea"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func TestPasteImportFiltersAndMerge(t *testing.T) {
	pos := func(line, col uint32) openidea.OutlinePos { return openidea.OutlinePos{Line: line, Col: col} }
	rng := func(a, b openidea.OutlinePos) openidea.OutlineRange { return openidea.OutlineRange{Start: a, End: b} }
	d := openidea.Diagnostic{Message: "Cannot find name Foo", Range: rng(pos(0, 0), pos(0, 3))}
	if !pasteDiagnosticMatches("Foo Bar", []autoImportPasteRange{{0, 3}}, d) {
		t.Fatal("pasted diagnostic not matched")
	}
	if pasteDiagnosticMatches("Foo Bar", []autoImportPasteRange{{4, 7}}, d) {
		t.Fatal("matched outside paste")
	}
	d.Unnecessary = true
	if pasteDiagnosticMatches("Foo Bar", []autoImportPasteRange{{0, 3}}, d) {
		t.Fatal("matched unnecessary diagnostic")
	}
	action := func(title, text string) autoImportAction {
		return autoImportAction{Title: title, Kind: "quickfix", Edit: &autoImportWorkspaceEdit{Files: []autoImportFileEdit{{Kind: "edit", Path: "/a", Edits: []autoImportTextEdit{{Range: rng(pos(0, 0), pos(0, 0)), NewText: text}}}}}}
	}
	a, b := action("Import Foo", "import Foo;\n"), action("Import Bar", "import Bar;\n")
	if _, ok := pastedImport([]autoImportAction{a, b}); ok {
		t.Fatal("ambiguous import accepted")
	}
	if _, ok := pastedImport([]autoImportAction{action("Import missing", "x")}); ok {
		t.Fatal("bulk fix accepted")
	}
	merged := mergePastedImports("/a", []autoImportAction{a, b})
	if merged == nil || merged.Files[0].Edits[0].NewText != "import Foo;\nimport Bar;\n" {
		t.Fatalf("merge: %#v", merged)
	}
	b.Edit.Files[0].Edits[0].NewText = "partial"
	if mergePastedImports("/a", []autoImportAction{a, b}) != nil {
		t.Fatal("partial insert merged")
	}
	if mergePastedImports("/other", []autoImportAction{a}) != nil {
		t.Fatal("foreign file accepted")
	}
}

func TestPasteCommandEditAndSelectionRanges(t *testing.T) {
	action := autoImportAction{Title: "Import Foo", Command: &autoImportCommand{Arguments: []json.RawMessage{json.RawMessage(`{"changes":{"file:///a%20b.java":[{"range":{"start":{"line":0,"character":0},"end":{"line":0,"character":0}},"newText":"import Foo;\n"}]}}`)}}}
	got := pasteCommandEdit(action, "/a b.java")
	if got.Edit == nil || got.Edit.Files[0].Edits[0].NewText != "import Foo;\n" {
		t.Fatalf("command edit = %#v", got.Edit)
	}
	if pasteCommandEdit(action, "/other.java").Edit != nil {
		t.Fatal("accepted another file")
	}
	selections := []editor.Selection{{Anchor: 1, Head: 1}, {Anchor: 3, Head: 3}}
	ranges := pastedSelectionRanges("abcd", "aXbcXd", selections, []editor.Selection{{Anchor: 2, Head: 2}, {Anchor: 5, Head: 5}})
	if !reflect.DeepEqual(ranges, []autoImportPasteRange{{1, 2}, {4, 5}}) {
		t.Fatalf("ranges: %#v", ranges)
	}
	if pastedSelectionRanges("abcd", "aXbcYd", selections, []editor.Selection{{Anchor: 1, Head: 1}, {Anchor: 5, Head: 5}}) != nil {
		t.Fatal("accepted invalid caret positions")
	}
	edits := []editor.Edit{{Start: 3, End: 3, Text: "B"}, {Start: 1, End: 2, Text: "AA"}}
	if got := pasteEditRanges(edits); !reflect.DeepEqual(got, []autoImportPasteRange{{1, 3}, {4, 5}}) {
		t.Fatalf("history ranges: %#v", got)
	}
}

func TestPasteDiagnosticsWaitForSyncAndPrepareGuard(t *testing.T) {
	m, tab := autoImportTestModel("Foo")
	page := m.nativeSettings.pages[settingsPageAutoImport]
	page.applied["editor.autoImport.importsOnPaste"] = "ask"
	m.nativeSettings.pages[settingsPageAutoImport] = page
	posts := make(chan func(), 8)
	m.postHook = func(fn func()) { posts <- fn }
	calls := make(chan string, 8)
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		calls <- command
		switch command {
		case "lsp_code_action_query":
			return autoImportQueryReply(autoImportImportAction(0)), nil
		case "prepare_workspace_edit":
			edit := args.(map[string]any)["edit"].(*autoImportWorkspaceEdit)
			edit.Preconditions = []autoImportWorkspacePrecond{{Path: tab.doc.Path(), Hash: "snapshot"}}
			return json.Marshal(edit)
		case "apply_workspace_edit":
			edit := args.(map[string]any)["edit"].(*autoImportWorkspaceEdit)
			if len(edit.Preconditions) != 1 || edit.Files[0].Version == nil {
				t.Error("apply omitted snapshot guards")
			}
			return json.RawMessage(`null`), nil
		default:
			return nil, fmt.Errorf("unexpected %s", command)
		}
	}
	p := &tab.autoImport.paste
	p.doc = tab.doc
	p.docID = tab.doc.ID()
	p.path = tab.doc.Path()
	p.root = m.root
	p.text = tab.doc.Text()
	p.version = tab.doc.Buffer().Version()
	p.ranges = []autoImportPasteRange{{0, 3}}
	p.diagnostics = map[string][]openidea.Diagnostic{}
	p.abort = func() {}
	d := openidea.Diagnostic{Message: "Cannot find name Foo", Range: openidea.OutlineRange{Start: openidea.OutlinePos{}, End: openidea.OutlinePos{Col: 3}}}
	m.autoImportPastePublished(tab, openidea.DiagnosticsEvent{Source: "java", Items: []openidea.Diagnostic{d}})
	if len(p.diagnostics) != 1 || p.settle != nil {
		t.Fatal("diagnostics were not retained until sync")
	}
	p.synced = true
	p.backendVersion = tab.doc.Version()
	m.collectAutoImportPaste(tab, p.gen)
	select {
	case fn := <-posts:
		fn()
	case <-time.After(3 * time.Second):
		t.Fatal("collection timed out")
	}
	if !p.open || p.edit == nil || len(p.edit.Preconditions) != 1 {
		t.Fatal("confirmation lacks prepared edit")
	}
	m.applyAutoImportPaste(tab, p.actions)
	for _, want := range []string{"lsp_code_action_query", "prepare_workspace_edit", "apply_workspace_edit"} {
		select {
		case got := <-calls:
			if got != want {
				t.Fatalf("got %s, want %s", got, want)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("missing %s", want)
		}
	}
}

func TestApplyPastedImportsActionUsesSelectedIndices(t *testing.T) {
	m, tab := autoImportTestModel("Foo")
	page := m.nativeSettings.pages[settingsPageAutoImport]
	page.applied["editor.autoImport.importsOnPaste"] = "ask"
	m.nativeSettings.pages[settingsPageAutoImport] = page
	applied := make(chan *autoImportWorkspaceEdit, 1)
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		if command != "apply_workspace_edit" {
			return nil, fmt.Errorf("unexpected %s", command)
		}
		applied <- args.(map[string]any)["edit"].(*autoImportWorkspaceEdit)
		return json.RawMessage(`null`), nil
	}
	position := openidea.OutlinePos{Line: 0, Col: 0}
	importAction := func(title, text string) autoImportAction {
		return autoImportAction{
			Title: title,
			Kind:  "quickfix",
			Edit: &autoImportWorkspaceEdit{Files: []autoImportFileEdit{{
				Kind: "edit",
				Path: tab.doc.Path(),
				Edits: []autoImportTextEdit{{
					Range:   openidea.OutlineRange{Start: position, End: position},
					NewText: text,
				}},
			}}},
		}
	}
	p := &tab.autoImport.paste
	p.doc = tab.doc
	p.docID = tab.doc.ID()
	p.path = tab.doc.Path()
	p.root = m.root
	p.text = tab.doc.Text()
	p.version = tab.doc.Buffer().Version()
	p.backendVersion = tab.doc.Version()
	p.synced = true
	p.open = true
	p.abort = func() {}
	p.actions = []autoImportAction{
		importAction("Import Foo", "import Foo;\n"),
		importAction("Import Bar", "import Bar;\n"),
	}
	p.edit = &autoImportWorkspaceEdit{
		Preconditions: []autoImportWorkspacePrecond{{Path: p.path, Hash: "snapshot"}},
		Files:         []autoImportFileEdit{{Kind: "edit", Path: p.path}},
	}
	if !m.RunActionWithArg("code.applyPastedImports", []any{float64(1)}) {
		t.Fatal("code.applyPastedImports is not callable")
	}
	select {
	case edit := <-applied:
		if edit == nil || len(edit.Files) != 1 ||
			len(edit.Files[0].Edits) != 1 ||
			edit.Files[0].Edits[0].NewText != "import Bar;\n" ||
			len(edit.Preconditions) != 1 {
			t.Fatalf("applied edit = %#v", edit)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("apply_workspace_edit was not called")
	}
}

func TestPasteImportDropsResultWhenDocumentChanges(t *testing.T) {
	m, tab := autoImportTestModel("Foo")
	page := m.nativeSettings.pages[settingsPageAutoImport]
	page.applied["editor.autoImport.importsOnPaste"] = "always"
	m.nativeSettings.pages[settingsPageAutoImport] = page
	posts := make(chan func(), 8)
	m.postHook = func(fn func()) { posts <- fn }
	var applied atomic.Bool
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		switch command {
		case "lsp_code_action_query":
			tab.state.Buffer.Edit(editor.Edit{Start: 0, End: 0, Text: "x"})
			return autoImportQueryReply(autoImportImportAction(0)), nil
		case "prepare_workspace_edit":
			edit := args.(map[string]any)["edit"].(*autoImportWorkspaceEdit)
			edit.Preconditions = []autoImportWorkspacePrecond{{Path: tab.doc.Path(), Hash: "snapshot"}}
			return json.Marshal(edit)
		case "apply_workspace_edit":
			applied.Store(true)
			return json.RawMessage(`null`), nil
		}
		return nil, fmt.Errorf("unexpected %s", command)
	}
	p := &tab.autoImport.paste
	p.doc = tab.doc
	p.docID = tab.doc.ID()
	p.path = tab.doc.Path()
	p.root = m.root
	p.text = tab.doc.Text()
	p.version = tab.doc.Buffer().Version()
	p.backendVersion = tab.doc.Version()
	p.synced = true
	p.ranges = []autoImportPasteRange{{0, 3}}
	p.abort = func() {}
	p.diagnostics = map[string][]openidea.Diagnostic{"java": {{Message: "Cannot find name Foo", Range: openidea.OutlineRange{End: openidea.OutlinePos{Col: 3}}}}}
	m.collectAutoImportPaste(tab, p.gen)
	select {
	case fn := <-posts:
		fn()
	case <-time.After(3 * time.Second):
		t.Fatal("collection timed out")
	}
	if applied.Load() || p.open {
		t.Fatal("stale paste result was used")
	}
	m.cancelAutoImportPaste(tab)
}
