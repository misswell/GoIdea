package app

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

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
