package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
	"mayoi/internal/position"
)

type safeDeleteDocBackend struct {
	info  openidea.DocInfo
	edits int
}

func (b *safeDeleteDocBackend) DocOpen(context.Context, string, string) (openidea.DocInfo, error) {
	return b.info, nil
}

func (b *safeDeleteDocBackend) DocEdit(
	_ context.Context,
	_ string,
	_ uint32,
	base uint32,
	_ position.ChangeSet,
) (uint32, error) {
	b.edits++
	return base + uint32(b.edits), nil
}

func (b *safeDeleteDocBackend) DocSnapshot(context.Context, string, uint32) (openidea.DocInfo, error) {
	return b.info, nil
}

func (b *safeDeleteDocBackend) DocSave(context.Context, string, uint32) error {
	return nil
}

func (b *safeDeleteDocBackend) DocClose(context.Context, string, uint32) error {
	return nil
}

func newSafeDeleteTestModel(
	t *testing.T,
	text string,
	caret int,
	hook func(context.Context, string, any) (json.RawMessage, error),
) (*Model, *Tab, chan func()) {
	t.Helper()
	backend := &safeDeleteDocBackend{info: openidea.DocInfo{
		ID: 7, Path: "/project/src/Greeter.java", Text: text,
		Version: 1, Language: "java",
	}}
	doc, err := docbridge.Open(
		context.Background(), backend, "w1", backend.info.Path,
	)
	if err != nil {
		t.Fatal(err)
	}
	state := &editor.State{Buffer: doc.Buffer()}
	state.Select(caret, caret)
	tab := &Tab{doc: doc, state: state}
	posts := make(chan func(), 8)
	m := &Model{
		ctx:        context.Background(),
		tabs:       []*Tab{tab},
		active:     0,
		invokeHook: hook,
		postHook: func(fn func()) {
			posts <- fn
		},
	}
	return m, tab, posts
}

func runSafeDeletePost(t *testing.T, posts <-chan func()) {
	t.Helper()
	select {
	case fn := <-posts:
		fn()
	case <-time.After(3 * time.Second):
		t.Fatal("Safe Delete did not finish its background step")
	}
}

func TestSafeDeleteDeletionRangeIncludesDocsAnnotationsAndBlankLine(t *testing.T) {
	text := "package p;\n\n// Greeter docs\n@Deprecated\npublic class Greeter {\n}\n\nclass User {}\n"
	start := strings.Index(text, "public class")
	end := strings.Index(text[start:], "}\n\nclass User") + start + 1
	got := safeDeleteDeletionRange(
		text, start, end,
		safeDeleteCommentSyntax{line: "//", blockStart: "/*", blockEnd: "*/"},
	)
	if got == nil {
		t.Fatal("deletion range was refused")
	}
	after := text[:got.start] + text[got.end:]
	if after != "package p;\n\nclass User {}\n" {
		t.Fatalf("after deletion = %q", after)
	}
}

func TestSafeDeleteDeletionRangeRefusesSharedLine(t *testing.T) {
	text := "void f() {\n    int first, second;\n}\n"
	start := strings.Index(text, "first")
	if got := safeDeleteDeletionRange(
		text, start, start+len("first"),
		safeDeleteCommentSyntax{line: "//"},
	); got != nil {
		t.Fatalf("shared-line declaration produced %+v", got)
	}
}

func TestSafeDeleteTextOccurrencesIgnoreCodeAndPartialWords(t *testing.T) {
	text := "class A {\n  // A and Apple\n  String s = \"A\";\n}\n"
	hits := safeDeleteTextOccurrences(
		text, "A",
		[]openidea.OutlineRange{{
			Start: openidea.OutlinePos{Line: 0, Col: 6},
			End:   openidea.OutlinePos{Line: 0, Col: 7},
		}},
		0, len("class A"),
	)
	if len(hits) != 2 {
		t.Fatalf("text hits = %+v", hits)
	}
	if hits[0].line != 1 || hits[1].line != 2 {
		t.Fatalf("text hit lines = %+v", hits)
	}
}

func TestSafeDeleteOutlineUsesInnermostContainingItem(t *testing.T) {
	outline := []openidea.OutlineItem{{
		Name: "Outer", Kind: "class",
		Range:          openidea.OutlineRange{Start: openidea.OutlinePos{}, End: openidea.OutlinePos{Line: 8, Col: 1}},
		SelectionRange: openidea.OutlineRange{Start: openidea.OutlinePos{Line: 0, Col: 6}, End: openidea.OutlinePos{Line: 0, Col: 11}},
		Children: []openidea.OutlineItem{{
			Name: "run", Kind: "method",
			Range:          openidea.OutlineRange{Start: openidea.OutlinePos{Line: 2}, End: openidea.OutlinePos{Line: 4, Col: 2}},
			SelectionRange: openidea.OutlineRange{Start: openidea.OutlinePos{Line: 2, Col: 8}, End: openidea.OutlinePos{Line: 2, Col: 11}},
		}},
	}}
	item := safeDeleteOutlineItemAt(
		outline, openidea.OutlinePos{Line: 2, Col: 9},
	)
	if item == nil || item.Name != "run" {
		t.Fatalf("outline item = %+v", item)
	}
}

func TestSafeDeleteLocalDeclarationRejectsParametersAndMultipleNames(t *testing.T) {
	backend := &safeDeleteDocBackend{info: openidea.DocInfo{
		ID: 1, Path: "/p/A.java", Text: "void f() {\n    int first, second;\n}\n",
		Version: 1, Language: "java",
	}}
	doc, err := docbridge.Open(
		context.Background(), backend, "w1", backend.info.Path,
	)
	if err != nil {
		t.Fatal(err)
	}
	statement := &safeDeleteSyntaxNode{
		Kind: "local_variable_declaration",
		Range: openidea.OutlineRange{
			Start: openidea.OutlinePos{Line: 1, Col: 4},
			End:   openidea.OutlinePos{Line: 1, Col: 23},
		},
	}
	symbol := safeDeleteLocalSymbol{
		Name: "first", Kind: "var", Certain: true,
		Definition: openidea.OutlineRange{
			Start: openidea.OutlinePos{Line: 1, Col: 8},
			End:   openidea.OutlinePos{Line: 1, Col: 13},
		},
	}
	if _, reason := safeDeleteLocalDeclaration(
		doc, doc.Text(), symbol, statement,
	); !strings.Contains(reason, "other names") {
		t.Fatalf("multiple-name reason = %q", reason)
	}
	symbol.Kind = "parameter"
	if _, reason := safeDeleteLocalDeclaration(
		doc, doc.Text(), symbol, statement,
	); !strings.Contains(reason, "Change Signature") {
		t.Fatalf("parameter reason = %q", reason)
	}
}

func TestEditorSafeDeleteShowsUsagesAndSendsThemToFindUsages(t *testing.T) {
	const text = "package p;\n\nclass Greeter {\n}\n\nclass User { Greeter greeter; }\n"
	m, _, posts := newSafeDeleteTestModel(
		t, text, strings.Index(text, "Greeter")+2,
		func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			switch command {
			case "languages":
				return json.RawMessage(`[{"id":"java","name":"Java","lineComment":"//","blockComment":["/*","*/"]}]`), nil
			case "lsp_goto":
				return json.RawMessage(`[{"path":"/project/src/Greeter.java","range":{"start":{"line":2,"col":6},"end":{"line":2,"col":13}}}]`), nil
			case "doc_outline":
				return json.RawMessage(`[{"name":"Greeter","kind":"class","range":{"start":{"line":2,"col":0},"end":{"line":3,"col":1}},"selection_range":{"start":{"line":2,"col":6},"end":{"line":2,"col":13}},"children":[]}]`), nil
			case "lsp_references":
				return json.RawMessage(`{"locations":[{"path":"/project/src/User.java","range":{"start":{"line":5,"col":13},"end":{"line":5,"col":20}}}]}`), nil
			case "syntax_identifiers":
				return json.RawMessage(`[]`), nil
			}
			return nil, errors.New("unexpected RPC " + command)
		},
	)
	m.SafeDeleteActive()
	runSafeDeletePost(t, posts)
	if m.safeDelete == nil || m.safeDelete.phase != editorSafeDeleteConfirm ||
		m.safeDelete.target.name != "Greeter" {
		t.Fatalf("confirmation = %+v", m.safeDelete)
	}
	m.safeDelete.searchText = false
	m.confirmEditorSafeDelete()
	runSafeDeletePost(t, posts)
	if m.safeDelete == nil || m.safeDelete.phase != editorSafeDeleteUsages ||
		len(m.safeDelete.usages) != 1 {
		t.Fatalf("usage result = %+v", m.safeDelete)
	}
	m.SafeDeleteViewUsages()
	if m.safeDelete != nil || m.usages.symbol != "Greeter" ||
		len(m.usages.locs) != 1 || !m.usages.done {
		t.Fatalf("Find Usages = %+v, state %+v", m.usages, m.safeDelete)
	}
}

func TestEditorSafeDeleteRemovesTheDeclarationAndOneBlankLine(t *testing.T) {
	const text = "package p;\n\nclass Greeter {\n}\n\nclass User {}\n"
	m, tab, posts := newSafeDeleteTestModel(
		t, text, strings.Index(text, "Greeter")+2,
		func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			switch command {
			case "languages":
				return json.RawMessage(`[{"id":"java","name":"Java","lineComment":"//","blockComment":["/*","*/"]}]`), nil
			case "lsp_goto":
				return json.RawMessage(`[{"path":"/project/src/Greeter.java","range":{"start":{"line":2,"col":6},"end":{"line":2,"col":13}}}]`), nil
			case "doc_outline":
				return json.RawMessage(`[{"name":"Greeter","kind":"class","range":{"start":{"line":2,"col":0},"end":{"line":3,"col":1}},"selection_range":{"start":{"line":2,"col":6},"end":{"line":2,"col":13}},"children":[]}]`), nil
			case "lsp_references":
				return json.RawMessage(`{"locations":[]}`), nil
			case "syntax_identifiers":
				return json.RawMessage(`[]`), nil
			}
			return nil, errors.New("unexpected RPC " + command)
		},
	)
	m.safeDeleteSearchText = false
	m.SafeDeleteActive()
	runSafeDeletePost(t, posts)
	m.confirmEditorSafeDelete()
	runSafeDeletePost(t, posts)
	if m.safeDelete == nil || m.safeDelete.phase != editorSafeDeleteApplying {
		t.Fatalf("Safe Delete did not start applying: %+v", m.safeDelete)
	}
	m.CancelSafeDelete()
	if m.safeDelete == nil || m.safeDelete.phase != editorSafeDeleteApplying {
		t.Fatalf("cancel dismissed the applying sheet: %+v", m.safeDelete)
	}
	runSafeDeletePost(t, posts)
	if got := tab.doc.Text(); got != "package p;\n\nclass User {}\n" {
		t.Fatalf("text after Safe Delete = %q", got)
	}
	if m.safeDelete != nil || !strings.Contains(m.status, "Deleted class 'Greeter'") {
		t.Fatalf("state = %+v, status = %q", m.safeDelete, m.status)
	}
	if selection := tab.state.Selection(); selection.Anchor != 12 || selection.Head != 12 {
		t.Fatalf("caret after deletion = %+v", selection)
	}
}

func TestSafeDeleteUsageMessageNamesCountsAndFiles(t *testing.T) {
	message, detail := safeDeleteUsagesMessage("Class", "Greeter", []openidea.Location{
		{Path: "/p/A.java"},
		{Path: "/p/A.java"},
		{Path: "/p/B.java"},
	})
	if message != "3 usages found" ||
		!strings.Contains(detail, "Class 'Greeter' has 3 usages") ||
		!strings.Contains(detail, "in 2 files") {
		t.Fatalf("message = %q, detail = %q", message, detail)
	}
}

func TestSafeDeleteRefusesADeclarationInAnotherFile(t *testing.T) {
	text := "class Caller { Missing value; }\n"
	m, _, posts := newSafeDeleteTestModel(
		t, text, strings.Index(text, "Missing"),
		func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			switch command {
			case "languages":
				return json.RawMessage(`[{"id":"java","name":"Java","lineComment":"//"}]`), nil
			case "lsp_goto":
				return json.RawMessage(`[{"path":"/project/src/Missing.java","range":{"start":{"line":0,"col":0},"end":{"line":0,"col":7}}}]`), nil
			}
			return json.RawMessage(`null`), nil
		},
	)
	m.SafeDeleteActive()
	runSafeDeletePost(t, posts)
	if m.safeDelete != nil ||
		m.status != "Cannot perform refactoring. 'Missing' is declared in Missing.java; Safe Delete it there" {
		t.Fatalf("state = %+v, status = %q", m.safeDelete, m.status)
	}
}

func TestSafeDeleteTypesDecodeServerRanges(t *testing.T) {
	raw := json.RawMessage(`{"status":"local","symbol":{"name":"x","kind":"var","definition":{"start":{"line":2,"col":4},"end":{"line":2,"col":5}},"scope":{"start":{"line":1,"col":0},"end":{"line":4,"col":1}},"references":[],"certain":true,"conflicts":[]}}`)
	var locals safeDeleteLocals
	if err := json.Unmarshal(raw, &locals); err != nil {
		t.Fatal(err)
	}
	if locals.Status != "local" || locals.Symbol == nil ||
		locals.Symbol.Name != "x" || locals.Symbol.Definition.Start.Line != 2 {
		t.Fatalf("locals = %+v", locals)
	}
}
