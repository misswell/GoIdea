package app

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"mayoi/internal/openidea"
)

func TestFindInPathCarriesQueryAndDirectory(t *testing.T) {
	m := &Model{}
	m.setRoot(t.TempDir())
	m.RunActionWithArg("find.replaceInPath", map[string]any{
		"query": "Order", "directory": m.Root(),
	})
	if !m.search.visible || !m.search.replaceMode || m.search.query != "Order" ||
		m.search.scopePath != m.Root() {
		t.Fatalf("find action state = %+v", m.search)
	}
}

func TestRunSearchSendsFullOpenIDEAQuery(t *testing.T) {
	requests := make(chan map[string]any, 2)
	applied := make(chan struct{}, 1)
	m := &Model{
		ctx: context.Background(),
		postHook: func(fn func()) {
			fn()
			applied <- struct{}{}
		},
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			switch command {
			case "search_validate":
				return nil, nil
			case "search_start":
				requests <- input.(map[string]any)
				return nil, nil
			default:
				return nil, fmt.Errorf("unexpected command %s", command)
			}
		},
	}
	m.setRoot(t.TempDir())
	m.searchScopes = []SearchScope{{
		Name: "Production", Pattern: "file:src//*.java&&!file:*Test*",
	}}
	m.search.query = "order"
	m.search.isRegex = true
	m.search.caseSensitive = true
	m.search.wholeWord = true
	m.search.include = "*.java, *.kt"
	m.search.exclude = "build/*"
	m.search.scopeName = "Production"
	m.RunSearch()
	select {
	case args := <-requests:
		query := args["query"].(map[string]any)
		scope := query["scope"].(map[string]any)
		if query["pattern"] != "order" || query["isRegex"] != true ||
			query["caseSensitive"] != true || query["wholeWord"] != true ||
			scope["kind"] != "pattern" ||
			scope["pattern"] != "file:src//*.java&&!file:*Test*" {
			t.Fatalf("search query = %#v", query)
		}
		include := query["include"].([]string)
		exclude := query["exclude"].([]string)
		if len(include) != 2 || include[0] != "*.java" || exclude[0] != "build/*" {
			t.Fatalf("search masks = include %v, exclude %v", include, exclude)
		}
	case <-time.After(time.Second):
		t.Fatal("Find did not start")
	}
	select {
	case <-applied:
	case <-time.After(time.Second):
		t.Fatal("Find did not finish its request")
	}
}

func TestEmptySearchMasksEncodeAsArrays(t *testing.T) {
	if searchMaskList("") == nil {
		t.Fatal("empty include/exclude masks encode as null instead of []")
	}
}

func TestSearchReplyWaitsForTheStreamedDoneEvent(t *testing.T) {
	started := make(chan struct{}, 1)
	replied := make(chan struct{}, 1)
	m := &Model{
		ctx: context.Background(),
		postHook: func(fn func()) {
			fn()
			replied <- struct{}{}
		},
		invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			if command == "search_start" {
				started <- struct{}{}
			}
			return nil, nil
		},
	}
	m.setRoot(t.TempDir())
	m.search.query = "class "
	m.RunSearch()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("search did not start")
	}
	select {
	case <-replied:
	case <-time.After(time.Second):
		t.Fatal("search request did not reply")
	}
	if !m.search.running || m.search.done {
		t.Fatalf("reply ended the stream early: %+v", m.search)
	}

	m.acceptSearchEvent(json.RawMessage(`{"type":"done","summary":{"files":0,"matches":0,"elapsedMs":1}}`))
	if m.search.running || !m.search.done {
		t.Fatalf("Done did not finish the stream: %+v", m.search)
	}
}

func TestReplaceInFilesUsesExactSelectedPosition(t *testing.T) {
	requests := make(chan map[string]any, 1)
	applied := make(chan struct{}, 1)
	m := &Model{
		ctx: context.Background(),
		postHook: func(fn func()) {
			fn()
			applied <- struct{}{}
		},
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command != "replace_in_files" {
				return nil, fmt.Errorf("unexpected command %s", command)
			}
			requests <- input.(map[string]any)
			return nil, nil
		},
	}
	m.setRoot(t.TempDir())
	m.search.replaceMode = true
	m.search.query = "old"
	m.search.replacement = "new"
	m.search.preserveCase = true
	m.search.files = []openidea.FileMatches{
		{
			Path: "/project/src/App.java",
			Matches: []openidea.LineMatch{{
				Line: 7, Ranges: [][2]uint32{{9, 12}},
			}},
		},
	}
	m.search.selectedFile, m.search.selectedMatch = 0, 0
	m.ReplaceSearchSelected()
	select {
	case args := <-requests:
		targets := args["targets"].([]openidea.ReplaceTarget)
		if args["replacement"] != "new" || args["preserveCase"] != true ||
			len(targets) != 1 || targets[0].Path != "/project/src/App.java" ||
			len(targets[0].Only) != 1 || targets[0].Only[0].Line != 7 ||
			targets[0].Only[0].Col != 9 {
			t.Fatalf("replace request = %#v", args)
		}
	case <-time.After(time.Second):
		t.Fatal("Replace did not start")
	}
	select {
	case <-applied:
	case <-time.After(time.Second):
		t.Fatal("Replace request did not finish")
	}
	m.acceptReplaceEvent(json.RawMessage(`{"type":"done","summary":{"files":1,"replacements":1,"failed":0,"cancelled":false,"elapsedMs":4}}`))
	if !m.search.replaceDone || m.search.replaceSummary != "Replaced 1 occurrences in 1 files" {
		t.Fatalf("replace summary = %+v", m.search)
	}
}

func TestSearchScopesValidateAndPersist(t *testing.T) {
	m := &Model{}
	m.OpenSearchScopes()
	m.addSearchScope()
	m.search.scopesDraft[0] = SearchScope{Name: "Sources", Pattern: "file:src//*"}
	m.saveSearchScopes()
	if m.search.scopesOpen || len(m.searchScopes) != 1 ||
		m.searchScopes[0] != (SearchScope{Name: "Sources", Pattern: "file:src//*"}) {
		t.Fatalf("saved scopes = %+v", m.searchScopes)
	}
	m.OpenSearchScopes()
	m.search.scopesDraft = append(m.search.scopesDraft,
		SearchScope{Name: "Sources", Pattern: "file:test//*"})
	m.saveSearchScopes()
	if !m.search.scopesOpen || m.search.scopesErr != "Duplicate scope name 'Sources'" {
		t.Fatalf("duplicate validation = %+v", m.search)
	}
}

func TestSearchActionsAreRegisteredAndPaletteVisible(t *testing.T) {
	for _, id := range []string{
		"find.inPath", "find.replaceInPath", "find.rerun",
		"navigate.switcher", "navigate.fileStructure", "search.editScopes",
	} {
		if _, ok := Command(id); !ok {
			t.Errorf("%s is not registered", id)
		}
	}
	for _, command := range commands() {
		if command.id == "navigate.switcherBack" {
			t.Fatal("backward switcher leaked into the command palette")
		}
	}
}
