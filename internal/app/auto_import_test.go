package app

import (
	"context"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

func autoImportTestModel(text string) (*Model, *Tab) {
	doc := writableSourceDocument("/project/Test.java", text)
	tab := &Tab{
		doc:           doc,
		state:         &editor.State{Buffer: doc.Buffer()},
		editorFocused: true,
	}
	m := &Model{
		ctx:    context.Background(),
		tabs:   []*Tab{tab},
		active: 0,
	}
	m.nativeSettings.pages = map[string]nativeSettingsPageState{
		settingsPageAutoImport: {
			loaded: true,
			applied: map[string]any{
				"editor.autoImport.addUnambiguous":   true,
				"editor.autoImport.optimizeOnTheFly": false,
			},
			draft: map[string]any{},
		},
	}
	return m, tab
}

func autoImportQueryReply(actions ...string) json.RawMessage {
	rawActions := make([]json.RawMessage, 0, len(actions))
	for _, action := range actions {
		rawActions = append(rawActions, json.RawMessage(action))
	}
	reply, _ := json.Marshal(openidea.CodeActionQuery{Actions: rawActions})
	return reply
}

func autoImportTestCandidate() autoImportCandidate {
	diagnostic := openidea.Diagnostic{
		Range: openidea.OutlineRange{
			Start: openidea.OutlinePos{Line: 4, Col: 8},
			End:   openidea.OutlinePos{Line: 4, Col: 11},
		},
		Severity: "error",
		Message:  "Foo cannot be resolved to a type",
	}
	return autoImportCandidate{key: autoImportDiagnosticKey(diagnostic), diagnostic: diagnostic}
}

func autoImportImportAction(rangeLine uint32) string {
	return `{
		"title":"Import 'Foo' (example)",
		"kind":"quickfix",
		"edit":{"files":[{
			"kind":"edit",
			"path":"/project/Test.java",
			"edits":[{
				"range":{"start":{"line":` + strconvUint(rangeLine) + `,"col":0},"end":{"line":` + strconvUint(rangeLine) + `,"col":0}},
				"new_text":"import example.Foo;\n"
			}]
		}]}
	}`
}

func strconvUint(value uint32) string {
	return strconv.FormatUint(uint64(value), 10)
}

func TestAutoImportRequiresExactlyOneEnabledImportAction(t *testing.T) {
	tests := []struct {
		name    string
		actions []string
		want    bool
	}{
		{
			name:    "one import",
			actions: []string{autoImportImportAction(1)},
			want:    true,
		},
		{
			name:    "two imports",
			actions: []string{autoImportImportAction(1), autoImportImportAction(2)},
		},
		{
			name: "one enabled and one disabled import",
			actions: []string{
				autoImportImportAction(1),
				`{"title":"Import 'Foo' (disabled)","kind":"quickfix","disabledReason":"busy"}`,
			},
			want: true,
		},
		{
			name:    "non quickfix",
			actions: []string{`{"title":"Import 'Foo'","kind":"refactor","edit":{"files":[]}}`},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, tab := autoImportTestModel("class Test { Foo value; }")
			reply := autoImportQueryReply(tc.actions...)
			m.invokeHook = func(_ context.Context, command string, _ any) (json.RawMessage, error) {
				if command != "lsp_code_action_query" {
					t.Fatalf("unexpected RPC %q", command)
				}
				return reply, nil
			}
			action, ok := m.findUnambiguousImport(
				m.ctx, tab.doc.ID(), tab.doc.Path(), 0,
				[]autoImportCandidate{autoImportTestCandidate()},
			)
			if ok != tc.want {
				t.Fatalf("action found = %v, want %v", ok, tc.want)
			}
			if ok && len(action) == 0 {
				t.Fatal("a successful import match returned no action")
			}
		})
	}
}

func TestAutoImportRejectsCaretLineAndUnrelatedTaggedFix(t *testing.T) {
	m, tab := autoImportTestModel("class Test { Foo value; }")
	reply := autoImportQueryReply(autoImportImportAction(0))
	m.invokeHook = func(_ context.Context, _ string, _ any) (json.RawMessage, error) {
		return reply, nil
	}
	if _, ok := m.findUnambiguousImport(
		m.ctx, tab.doc.ID(), tab.doc.Path(), 0,
		[]autoImportCandidate{autoImportTestCandidate()},
	); ok {
		t.Fatal("an import edit on the caret line was accepted")
	}

	candidate := autoImportTestCandidate()
	action := autoImportImportAction(1)
	action = strings.Replace(action, `"title"`, `"diagnostics":[{"range":{"start":{"line":9,"col":0},"end":{"line":9,"col":1}},"severity":"error","message":"Other"}],"title"`, 1)
	reply = autoImportQueryReply(action)
	if _, ok := m.findUnambiguousImport(
		m.ctx, tab.doc.ID(), tab.doc.Path(), 0,
		[]autoImportCandidate{candidate},
	); ok {
		t.Fatal("a quickfix tagged for a different diagnostic was accepted")
	}
}

func TestAutoImportOptimizeRejectsDisabledCaretAndNoopEdits(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		action    string
		caretLine int
		want      bool
	}{
		{
			name: "changed import block",
			text: "import a.A;\nimport b.B;\nclass Test {}\n",
			action: `{"title":"Optimize imports","kind":"source.organizeImports","edit":{"files":[{
				"kind":"edit","path":"/project/Test.java","edits":[{
					"range":{"start":{"line":0,"col":0},"end":{"line":2,"col":0}},
					"new_text":"import a.A;\n"
				}]
			}]}}`,
			caretLine: 3,
			want:      true,
		},
		{
			name: "disabled",
			text: "import a.A;\n",
			action: `{"title":"Optimize imports","kind":"source.organizeImports","disabledReason":"busy",
				"edit":{"files":[{"kind":"edit","path":"/project/Test.java","edits":[]}]}}`,
		},
		{
			name: "caret line",
			text: "import a.A;\n",
			action: `{"title":"Optimize imports","kind":"source.organizeImports","edit":{"files":[{
				"kind":"edit","path":"/project/Test.java","edits":[{
					"range":{"start":{"line":0,"col":0},"end":{"line":0,"col":12}},
					"new_text":"import b.B;"
				}]
			}]}}`,
		},
		{
			name: "no op",
			text: "import a.A;\n",
			action: `{"title":"Optimize imports","kind":"source.organizeImports","edit":{"files":[{
				"kind":"edit","path":"/project/Test.java","edits":[{
					"range":{"start":{"line":0,"col":0},"end":{"line":0,"col":12}},
					"new_text":"import a.A;"
				}]
			}]}}`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, tab := autoImportTestModel(tc.text)
			reply := autoImportQueryReply(tc.action)
			m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
				if command != "lsp_code_action_query" {
					t.Fatalf("unexpected RPC %q", command)
				}
				call := args.(map[string]any)
				if only := call["only"]; !reflect.DeepEqual(only, []string{"source.organizeImports"}) {
					t.Fatalf("only = %#v", only)
				}
				return reply, nil
			}
			action, ok := m.findOptimizeImportAction(
				m.ctx, tab.doc.ID(), tab.doc, tc.text, tab.doc.Path(), tc.caretLine,
			)
			if ok != tc.want {
				t.Fatalf("action found = %v, want %v", ok, tc.want)
			}
			if ok && len(action) == 0 {
				t.Fatal("a successful optimize action returned no action")
			}
		})
	}
}

func TestRunAutoImportAppliesOneFixAndRejectsStaleText(t *testing.T) {
	for _, stale := range []bool{false, true} {
		t.Run(map[bool]string{false: "current", true: "stale"}[stale], func(t *testing.T) {
			m, tab := autoImportTestModel("class Test { Foo value; }")
			candidate := autoImportTestCandidate()
			m.diags = map[string]map[string][]openidea.Diagnostic{
				realPath(tab.doc.Path()): {"java": {candidate.diagnostic}},
			}
			post := make(chan func(), 4)
			m.postHook = func(fn func()) { post <- fn }

			actionLine := uint32(1)
			reply := autoImportQueryReply(autoImportImportAction(actionLine))
			var mu sync.Mutex
			var calls []string
			m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
				mu.Lock()
				calls = append(calls, command)
				mu.Unlock()
				switch command {
				case "lsp_code_action_query":
					if stale {
						tab.state.Buffer.Edit(editor.Edit{Start: 0, End: 0, Text: "// edited\n"})
					}
					return reply, nil
				case "lsp_apply_code_action":
					apply := args.(map[string]any)
					if _, ok := apply["action"].(json.RawMessage); !ok {
						t.Errorf("apply action type = %T", apply["action"])
					}
					return json.RawMessage(`null`), nil
				default:
					t.Errorf("unexpected RPC %q", command)
					return nil, nil
				}
			}

			m.runAutoImport(tab)
			deadline := time.After(3 * time.Second)
			for tab.autoImport.running {
				select {
				case fn := <-post:
					fn()
				case <-deadline:
					t.Fatal("auto import did not finish")
				}
			}

			mu.Lock()
			gotCalls := append([]string(nil), calls...)
			mu.Unlock()
			wantCalls := []string{"lsp_code_action_query", "lsp_apply_code_action"}
			if stale {
				wantCalls = wantCalls[:1]
			}
			if strings.Join(gotCalls, ",") != strings.Join(wantCalls, ",") {
				t.Fatalf("RPC calls = %v, want %v", gotCalls, wantCalls)
			}
		})
	}
}
