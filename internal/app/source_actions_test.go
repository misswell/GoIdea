package app

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
)

func writableSourceDocument(path, text string) *docbridge.Document {
	doc := docbridge.Virtual(path, text)
	doc.SetReadOnly(false)
	return doc
}

func sourceActionModel(path, text string) (*Model, *Tab) {
	doc := writableSourceDocument(path, text)
	tab := &Tab{doc: doc, state: &editor.State{Buffer: doc.Buffer()}}
	return &Model{tabs: []*Tab{tab}, active: 0}, tab
}

func TestFixESLintProblemsAppliesPreferredWholeFileAction(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	m, tab := sourceActionModel("/project/web/src/😀.ts", "const answer=1")
	m.ctx = ctx
	post := make(chan func(), 8)
	m.postHook = func(fn func()) { post <- fn }

	preferred := json.RawMessage(`{"title":"Fix all ESLint issues","kind":"source.fixAll.eslint","isPreferred":true}`)
	actions := []json.RawMessage{
		json.RawMessage(`{"title":"Fix all TypeScript issues","kind":"source.fixAll","isPreferred":true}`),
		json.RawMessage(`{"title":"Fix ESLint issues","kind":"source.fixAll.eslint"}`),
		json.RawMessage(`{"title":"Unavailable ESLint fix","kind":"source.fixAll.eslint","isPreferred":true,"disabledReason":"Lint is busy"}`),
		preferred,
	}
	queryReply, err := json.Marshal(openidea.CodeActionQuery{Actions: actions})
	if err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var calls []string
	var queryArgs, applyArgs map[string]any
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, command)
		switch command {
		case "lsp_code_action_query":
			queryArgs = args.(map[string]any)
			return queryReply, nil
		case "lsp_apply_code_action":
			applyArgs = args.(map[string]any)
			return json.RawMessage(`{}`), nil
		default:
			t.Errorf("unexpected RPC %q", command)
			return nil, nil
		}
	}

	m.FixESLintProblems()
	for tab.formatting || m.status != "Fix ESLint Problems done" {
		select {
		case fn := <-post:
			fn()
		case <-ctx.Done():
			t.Fatalf("ESLint action did not finish: status=%q", m.status)
		}
	}

	mu.Lock()
	gotCalls := append([]string(nil), calls...)
	gotQuery := queryArgs
	gotApply := applyArgs
	mu.Unlock()
	if !reflect.DeepEqual(gotCalls, []string{"lsp_code_action_query", "lsp_apply_code_action"}) {
		t.Fatalf("calls = %v", gotCalls)
	}
	if only := gotQuery["only"]; !reflect.DeepEqual(only, []string{"source.fixAll.eslint"}) {
		t.Fatalf("code action kinds = %#v", only)
	}
	wantEnd := lspPos(tab.doc.UTF16Pos(len(tab.doc.Text())))
	if got := gotQuery["range"].(map[string]any)["end"]; !reflect.DeepEqual(got, wantEnd) {
		t.Fatalf("action range end = %#v, want %#v", got, wantEnd)
	}
	if raw, ok := gotApply["action"].(json.RawMessage); !ok || string(raw) != string(preferred) {
		t.Fatalf("applied action = %s, want preferred %s", raw, preferred)
	}
}

func TestFixESLintProblemsReportsNoProblems(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	m, tab := sourceActionModel("/project/web/src/app.ts", "const answer = 1")
	m.ctx = ctx
	post := make(chan func(), 4)
	m.postHook = func(fn func()) { post <- fn }
	m.invokeHook = func(_ context.Context, command string, _ any) (json.RawMessage, error) {
		if command != "lsp_code_action_query" {
			t.Fatalf("unexpected RPC %q", command)
		}
		return json.RawMessage(`{"actions":[]}`), nil
	}

	m.FixESLintProblems()
	for tab.formatting || m.status != "No ESLint problems to fix" {
		select {
		case fn := <-post:
			fn()
		case <-ctx.Done():
			t.Fatalf("empty ESLint action did not finish: status=%q", m.status)
		}
	}
}

func TestFixESLintProblemsMenuGateMatchesOpenIDEA(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		language []string
		status   string
		root     string
		readOnly bool
		want     bool
	}{
		{
			name: "covered TypeScript", path: "/p/web/src/a.ts",
			language: []string{"javascript", "typescript"}, status: "ready", root: "/p/web", want: true,
		},
		{
			name: "busy server still covers", path: "/p/web/src/a.ts",
			language: []string{"TypeScript"}, status: "busy", root: "/p/web", want: true,
		},
		{
			name: "project-wide server", path: "/p/web/src/a.ts",
			language: []string{"typescript"}, status: "ready", want: true,
		},
		{
			name: "Windows path", path: `C:\p\web\a.js`,
			language: []string{"javascript"}, status: "ready", root: `C:\p\web`, want: true,
		},
		{
			name: "wrong language", path: "/p/web/src/a.rs",
			language: []string{"typescript"}, status: "ready", root: "/p/web", want: false,
		},
		{
			name: "outside root", path: "/p/api/a.ts",
			language: []string{"typescript"}, status: "ready", root: "/p/web", want: false,
		},
		{
			name: "root prefix is not containment", path: "/p/webapp/a.ts",
			language: []string{"typescript"}, status: "ready", root: "/p/web", want: false,
		},
		{
			name: "starting does not cover", path: "/p/web/src/a.ts",
			language: []string{"typescript"}, status: "starting", root: "/p/web", want: false,
		},
		{
			name: "read-only editor", path: "/p/web/src/a.ts",
			language: []string{"typescript"}, status: "ready", root: "/p/web", readOnly: true, want: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m, tab := sourceActionModel(test.path, "const value = 1")
			tab.doc.SetReadOnly(test.readOnly)
			m.languageServers = map[uint32]languageServerInfo{
				1: {ID: 1, Name: "eslint", Status: test.status, Languages: test.language, Root: test.root},
			}
			if got := m.fixESLintProblemsAvailable(tab); got != test.want {
				t.Fatalf("ESLint menu available = %v, want %v", got, test.want)
			}
		})
	}
	if _, ok := Command("code.fixEslintProblems"); !ok {
		t.Fatal("code.fixEslintProblems is not registered")
	}
}
