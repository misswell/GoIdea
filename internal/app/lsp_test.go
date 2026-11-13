package app

import (
	"testing"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

func TestStripSnippet(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"System.out.println($0);", "System.out.println();"},
		{"for (${1:i} = ${2:0}; $1 < $3; $1++) {\n\t$0\n}", "for (i = 0; i < ; i++) {\n\t\n}"},
		{"${1|public,private|}", "public"},
		{"${1|a\\,b,c|}", "a,b"},
		{"$TM_FILENAME", "$TM_FILENAME"},
		{`\$1`, "$1"},
		{"plain text", "plain text"},
		{"nested ${1:List<${2:String}>}", "nested List<String>"},
	} {
		if got := stripSnippet(tc.in); got != tc.want {
			t.Errorf("stripSnippet(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestLSPCompletionKind(t *testing.T) {
	for _, tc := range []struct {
		kind *string
		want editor.CompletionKind
	}{
		{strp("method"), editor.CompletionMethod},
		{strp("constructor"), editor.CompletionFunction},
		{strp("enumMember"), editor.CompletionField},
		{strp("class"), editor.CompletionType},
		{strp("typeParameter"), editor.CompletionType},
		{strp("module"), editor.CompletionModule},
		{strp("constant"), editor.CompletionConstant},
		{strp("keyword"), editor.CompletionKeyword},
		{strp("snippet"), editor.CompletionSnippet},
		{strp("somethingNew"), editor.CompletionText},
		{nil, editor.CompletionText},
	} {
		if got := lspCompletionKind(tc.kind); got != tc.want {
			t.Errorf("kind %v = %v, want %v", tc.kind, got, tc.want)
		}
	}
}

func TestLSPCompletions(t *testing.T) {
	detail, filter := "()void", "sout"
	items := lspCompletions([]openidea.CompletionItem{{
		Label:       "println",
		LabelDetail: &detail,
		Kind:        strp("method"),
		InsertText:  "println(${1:x})$0",
		IsSnippet:   true,
		FilterText:  &filter,
		Documentation: &openidea.Markup{
			Kind:  "markdown",
			Value: "Prints a line\n\n```java\nvoid println(String x)\n```",
		},
	}, {
		Label:      "name",
		InsertText: "name",
	}})
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
	got := items[0]
	if got.Label != "println" || got.Detail != "()void" || got.Insert != "println(x)" {
		t.Fatalf("first item = %+v", got)
	}
	if got.Kind != editor.CompletionMethod || got.FilterText != "sout" {
		t.Fatalf("first item kind/filter = %v %q", got.Kind, got.FilterText)
	}
	if got.Documentation != "Prints a line\n\nvoid println(String x)" {
		t.Fatalf("documentation = %q", got.Documentation)
	}
	if items[1].Insert != "name" || items[1].Kind != editor.CompletionText {
		t.Fatalf("second item = %+v", items[1])
	}
}

func TestHoverText(t *testing.T) {
	for _, tc := range []struct {
		markup openidea.Markup
		want   string
	}{
		{openidea.Markup{Kind: "plaintext", Value: "int x"}, "int x"},
		{openidea.Markup{Kind: "markdown", Value: "### `main(String[])`\n\n**static** void"}, "main(String[])\n\nstatic void"},
		{openidea.Markup{Kind: "markdown", Value: "```java\nclass A {}\n```"}, "class A {}"},
		{openidea.Markup{Kind: "markdown", Value: "  \n\n"}, ""},
	} {
		if got := hoverText(tc.markup); got != tc.want {
			t.Errorf("hoverText(%+v) = %q, want %q", tc.markup, got, tc.want)
		}
	}
}

func strp(s string) *string { return &s }

func TestLSPJavaServerStates(t *testing.T) {
	state := func(name, status string, langs ...string) lspServerState {
		return lspServerState{Name: name, Status: status, Languages: langs}
	}
	if lspHasJavaServer([]lspServerState{state("yaml", "ready", "yaml")}) {
		t.Error("a server without the java language must not count")
	}
	if !lspHasJavaServer([]lspServerState{state("jdtls", "stopped", "java")}) {
		t.Error("a configured Java server must count")
	}
	if lspJavaStalled([]lspServerState{state("jdtls", "starting", "java")}) {
		t.Error("a starting server is not stalled")
	}
	if !lspJavaStalled([]lspServerState{state("jdtls", "missing", "java"), state("kmp-lsp", "missing", "java")}) {
		t.Error("missing Java servers are stalled")
	}
	if lspJavaStalled([]lspServerState{state("jdtls", "ready", "java"), state("kmp-lsp", "missing", "java")}) {
		t.Error("one running server is enough")
	}
	got := lspJavaStates([]lspServerState{state("jdtls", "missing", "java"), state("yaml", "ready", "yaml")})
	if got != "jdtls=missing" {
		t.Errorf("lspJavaStates = %q, want %q", got, "jdtls=missing")
	}
}
