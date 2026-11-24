package app

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

func TestDocumentationMarkup(t *testing.T) {
	markup := openidea.Markup{Kind: "markdown", Value: `~~~java
public int sum(int x)
~~~

**Returns** the *sum* of [values](file:///workspace/My%20Class.java#L3,5).

- First item
- Second item

<kbd>F4</kbd>

<script>alert("secret")</script>

<a href="javascript:alert(1)">Unsafe link</a>
`}
	blocks := documentationBlocks(markup, "Java")
	if len(blocks) < 5 || !blocks[0].code || len(blocks[0].runs) == 0 {
		t.Fatalf("signature block lost: %+v", blocks)
	}
	var visible strings.Builder
	styles := map[string]documentationRun{}
	for _, block := range blocks {
		for _, run := range block.runs {
			visible.WriteString(run.text)
			styles[strings.TrimSpace(run.text)] = run
		}
		visible.WriteByte('\n')
	}
	for _, text := range []string{"public int sum(int x)", "Returns the sum of values.", "• First item", "F4", "Unsafe link"} {
		if !strings.Contains(visible.String(), text) {
			t.Errorf("missing rendered text %q in %q", text, visible.String())
		}
	}
	if strings.Contains(visible.String(), "secret") || styles["Unsafe link"].href != "" {
		t.Errorf("unexpected untrusted markup in %q", visible.String())
	}
	if !styles["Returns"].bold || !styles["sum"].italic || styles["values"].href == "" || !styles["F4"].code {
		t.Errorf("inline formatting was lost: %+v", styles)
	}
}

func TestDocumentationSourceLinks(t *testing.T) {
	tests := []struct {
		href, path string
		line, col  uint32
		virtual    bool
	}{
		{"file:///workspace/My%20Class.java#L3,5", "/workspace/My Class.java", 2, 4, false},
		{"file:///workspace/Example.class#L3", "file:///workspace/Example.class", 2, 0, true},
		{"jdt://contents/rt.jar/java.lang/String.class?=project#L42", "jdt://contents/rt.jar/java.lang/String.class?=project", 41, 0, true},
	}
	for _, tt := range tests {
		got, ok := documentationLink(tt.href)
		if !ok || got.path != tt.path || got.pos.Line != tt.line || got.pos.Col != tt.col || got.virtual != tt.virtual {
			t.Errorf("documentationLink(%q) = %+v, %v", tt.href, got, ok)
		}
	}
	for _, href := range []string{"javascript:alert(1)", "https://www.example.org", "file://remote/share/Example.java", "file:relative.java"} {
		if _, ok := documentationLink(href); ok {
			t.Errorf("unexpected source link %q", href)
		}
	}
	if cols := documentationHoverColumns("/*😀*/ public class Ω {", 0); !slices.Equal(cols, []uint32{0, 20}) {
		t.Errorf("documentation hover columns = %v, want insertion and symbol column", cols)
	}
}

func TestDocumentationHistoryAndDock(t *testing.T) {
	m := gotoLineTestModel("symbol", 1)
	m.setRoot(t.TempDir())
	first := documentationEntry{blocks: documentationBlocks(openidea.Markup{Kind: "plaintext", Value: "First result"})}
	second := documentationEntry{blocks: documentationBlocks(openidea.Markup{Kind: "plaintext", Value: "Second result"})}
	replacement := documentationEntry{blocks: documentationBlocks(openidea.Markup{Kind: "plaintext", Value: "New result"})}
	view := &documentationView{}
	view.push(first)
	view.push(second)
	m.stepDocumentation(view, -1)
	view.push(replacement)
	m.documentation.popup = view
	m.documentation.tab = m.tab()
	m.documentation.version = m.tab().doc.Buffer().Version()
	tt := ui.NewTester(func(c *ui.Context) {
		if m.documentation.popup != nil {
			m.documentationPopup(c, c.Theme())
		}
		if m.toolVisible(documentationWindow) {
			m.documentationPanel(c, c.Theme())
		}
	}, 800, 500)
	tt.Frame()
	if !tt.HasText("New result") || tt.HasText("Second result") {
		t.Fatalf("popup content = %q", tt.Texts())
	}
	if err := tt.Click("Show in Tool Window"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if m.documentation.popup != nil || !m.toolVisible(documentationWindow) || !tt.HasText("New result") {
		t.Fatalf("docking lost the documentation: %q", tt.Texts())
	}
	m.stepDocumentation(&m.documentation.window, -1)
	tt.Frame()
	if !tt.HasText("First result") {
		t.Fatalf("previous entry missing in tool window: %q", tt.Texts())
	}
}

func TestDocumentationFocusAndKeyboardDock(t *testing.T) {
	m := gotoLineTestModel("symbol", 1)
	entry := documentationEntry{blocks: documentationBlocks(openidea.Markup{Kind: "plaintext", Value: "Documented symbol"})}
	view := &documentationView{}
	view.push(entry)
	m.documentation.popup, m.documentation.tab = view, m.tab()
	m.documentation.version = m.tab().doc.Buffer().Version()
	tt := ui.NewTester(func(c *ui.Context) {
		if c.Shortcut(0, ui.KeyF1) {
			m.QuickDocumentation()
		}
		if m.documentation.popup != nil {
			m.documentationPopup(c, c.Theme())
		}
	}, 800, 500)
	tt.Frame()
	if m.documentation.popupFocused {
		t.Fatal("popup took focus on opening")
	}
	tt.Key(0, ui.KeyF1)
	tt.Frame()
	if m.documentation.popup != view || !m.documentation.popupFocused {
		t.Fatal("the first repeated F1 did not focus the popup")
	}
	tt.Key(0, ui.KeyF1)
	tt.Frame()
	if m.documentation.popup != nil || !m.toolVisible(documentationWindow) {
		t.Fatal("F1 on the focused popup did not open the tool window")
	}
}

func TestDocumentationModifiedLinkOpensSource(t *testing.T) {
	m := gotoLineTestModel("alpha\nbeta", 0)
	m.ctx = context.Background()
	m.mgr = openidea.NewManager(openidea.LaunchConfig{})
	view := &documentationView{}
	view.push(documentationEntry{blocks: documentationBlocks(openidea.Markup{
		Kind: "markdown", Value: "See [this declaration](file:///test.java#L2,2).",
	})})
	m.documentation.popup, m.documentation.tab = view, m.tab()
	m.documentation.version = m.tab().doc.Buffer().Version()
	tt := ui.NewTester(func(c *ui.Context) {
		if m.documentation.popup != nil {
			m.documentationPopup(c, c.Theme())
		}
	}, 800, 500)
	tt.Frame()
	if err := tt.ClickWith(ui.Cmd, "this declaration"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if m.documentation.popup != nil || m.tab().state.Cursor() != len("alpha\nb") {
		t.Fatalf("modifier-click did not reveal the declaration; cursor = %d", m.tab().state.Cursor())
	}
}

func TestQuickDocumentationForSelectedCompletionUsesInlineDocs(t *testing.T) {
	item := editor.Completion{
		Label: "sum", Detail: "(int a, int b)", Insert: "sum",
		Documentation: "Returns the sum.",
	}
	m := selectedCompletionDocumentationModel(t, item, openidea.CompletionItem{
		Label: "sum", InsertText: "sum", Server: 4,
	})
	m.invokeHook = func(context.Context, string, any) (json.RawMessage, error) {
		t.Error("inline completion documentation should not resolve")
		return nil, context.Canceled
	}
	m.QuickDocumentation()
	entry := m.documentation.popup.current()
	if entry == nil || !strings.Contains(documentationEntryText(entry), "Returns the sum.") {
		t.Fatalf("completion documentation = %+v", entry)
	}
}

func TestQuickDocumentationResolvesSelectedCompletion(t *testing.T) {
	item := editor.Completion{Label: "sum", Detail: "(int a, int b)", Insert: "sum"}
	source := openidea.CompletionItem{
		Label: "sum", InsertText: "sum", Data: json.RawMessage(`{"resolve":"sum"}`), Server: 8,
	}
	m := selectedCompletionDocumentationModel(t, item, source)
	posts := make(chan struct{}, 1)
	m.postHook = func(fn func()) {
		fn()
		posts <- struct{}{}
	}
	m.invokeHook = func(_ context.Context, command string, input any) (json.RawMessage, error) {
		if command != "lsp_resolve_completion" {
			t.Errorf("command = %q", command)
		}
		args := input.(map[string]any)
		tracked, ok := args["item"].(openidea.CompletionItem)
		if !ok || tracked.Server != 8 || string(tracked.Data) != `{"resolve":"sum"}` {
			t.Errorf("resolve item = %#v", args["item"])
		}
		documentation := openidea.Markup{Kind: "markdown", Value: "Resolved **sum** documentation."}
		return json.Marshal(openidea.CompletionItem{
			Label: "sum", InsertText: "sum", Documentation: &documentation,
			Data: source.Data, Server: source.Server,
		})
	}
	m.QuickDocumentation()
	select {
	case <-posts:
	case <-time.After(2 * time.Second):
		t.Fatal("completion documentation did not resolve")
	}
	entry := m.documentation.popup.current()
	if entry == nil || !strings.Contains(documentationEntryText(entry), "Resolved sum documentation.") {
		t.Fatalf("resolved completion documentation = %+v", entry)
	}
}

func TestQuickDocumentationKeepsHoverWithoutSelectedCompletion(t *testing.T) {
	item := editor.Completion{Label: "sum", Insert: "sum"}
	m := selectedCompletionDocumentationModel(t, item, openidea.CompletionItem{
		Label: "sum", InsertText: "sum", Server: 1,
	})
	m.tab().completion.open = false
	posts := make(chan struct{}, 1)
	m.postHook = func(fn func()) {
		fn()
		posts <- struct{}{}
	}
	m.invokeHook = func(_ context.Context, command string, _ any) (json.RawMessage, error) {
		if command != "lsp_hover" {
			t.Errorf("command = %q, want lsp_hover", command)
		}
		return json.Marshal(openidea.Hover{
			Contents: openidea.Markup{Kind: "plaintext", Value: "Hover documentation"},
		})
	}
	m.QuickDocumentation()
	select {
	case <-posts:
	case <-time.After(2 * time.Second):
		t.Fatal("hover documentation did not finish")
	}
	entry := m.documentation.popup.current()
	if entry == nil || !strings.Contains(documentationEntryText(entry), "Hover documentation") {
		t.Fatalf("hover documentation = %+v", entry)
	}
}

func TestQuickDocumentationDropsStaleCompletionReply(t *testing.T) {
	item := editor.Completion{Label: "sum", Insert: "sum"}
	m := selectedCompletionDocumentationModel(t, item, openidea.CompletionItem{
		Label: "sum", InsertText: "sum", Data: json.RawMessage(`{"resolve":"sum"}`), Server: 3,
	})
	started, release, posted := make(chan struct{}), make(chan struct{}), make(chan struct{}, 1)
	m.postHook = func(fn func()) {
		fn()
		posted <- struct{}{}
	}
	m.invokeHook = func(context.Context, string, any) (json.RawMessage, error) {
		close(started)
		<-release
		documentation := openidea.Markup{Kind: "plaintext", Value: "Stale documentation"}
		return json.Marshal(openidea.CompletionItem{
			Label: "sum", InsertText: "sum", Documentation: &documentation,
		})
	}
	m.QuickDocumentation()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("completion documentation did not start")
	}
	other := editor.Completion{Label: "other", Insert: "other"}
	m.tab().completion.selected = &other
	close(release)
	select {
	case <-posted:
	case <-time.After(2 * time.Second):
		t.Fatal("stale completion documentation did not finish")
	}
	if m.documentation.popup != nil || m.documentation.request != 0 {
		t.Fatalf("stale completion documentation was accepted: %+v", m.documentation)
	}
}

func selectedCompletionDocumentationModel(
	t *testing.T,
	item editor.Completion,
	source openidea.CompletionItem,
) *Model {
	t.Helper()
	doc := fileActionDocument(t, true)
	state := &editor.State{Buffer: doc.Buffer()}
	state.Select(0, 0)
	m := &Model{
		ctx:  context.Background(),
		tabs: []*Tab{{doc: doc, state: state}},
	}
	tab := m.tab()
	tab.completion = completionTracking{
		version:  doc.Buffer().Version(),
		cursor:   state.Cursor(),
		selected: &item,
		infos: map[string]completionInfo{
			completionTrackKey(item): {LSP: &source},
		},
		open: true,
	}
	return m
}

func documentationEntryText(entry *documentationEntry) string {
	if entry == nil {
		return ""
	}
	var text strings.Builder
	for _, block := range entry.blocks {
		for _, run := range block.runs {
			text.WriteString(run.text)
		}
		text.WriteByte('\n')
	}
	return text.String()
}
