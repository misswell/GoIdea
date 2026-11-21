package app

import (
	"testing"

	"mayoi/internal/openidea"
)

func TestReferenceOffsetAndDeclarationChain(t *testing.T) {
	text := "var name = other;"
	if offset, ok := referenceOffsetInText(text, 4, 8); !ok || offset != 4 {
		t.Fatalf("selected identifier = %d, %v", offset, ok)
	}
	if _, ok := referenceOffsetInText(text, 3, 4); ok {
		t.Fatal("selected whitespace was accepted as an identifier")
	}
	start := len("a😀")
	if offset, ok := referenceOffsetInText("a😀b", start, start); !ok || offset != start {
		t.Fatalf("caret identifier = %d, %v", offset, ok)
	}

	outline := []openidea.OutlineItem{{
		Name: "Outer", Kind: "class",
		Range:          openidea.OutlineRange{Start: openidea.OutlinePos{Line: 0}, End: openidea.OutlinePos{Line: 20}},
		SelectionRange: openidea.OutlineRange{Start: openidea.OutlinePos{Line: 0}, End: openidea.OutlinePos{Line: 0, Col: 5}},
		Children: []openidea.OutlineItem{{
			Name: "run(int)", Kind: "method",
			Range:          openidea.OutlineRange{Start: openidea.OutlinePos{Line: 4}, End: openidea.OutlinePos{Line: 10}},
			SelectionRange: openidea.OutlineRange{Start: openidea.OutlinePos{Line: 4, Col: 7}, End: openidea.OutlinePos{Line: 4, Col: 10}},
		}},
	}}
	chain := referenceChainNames(referenceDeclarationChain(outline, 4, 8))
	if len(chain) != 2 || chain[0] != "Outer" || chain[1] != "run" {
		t.Fatalf("declaration chain = %v", chain)
	}
}

func TestQualifiedReferenceMatchesOpenIDEALanguages(t *testing.T) {
	cases := []struct {
		name  string
		input referenceInput
		want  string
	}{
		{
			name: "java method",
			input: referenceInput{
				language: "java", path: "/p/src/demo/Outer.java", root: "/p",
				text: "package demo;\nclass Outer { void run() {} }", line: 1,
				chain: []string{"Outer", "Inner", "run"},
			},
			want: "demo.Outer.Inner#run",
		},
		{
			name: "go method",
			input: referenceInput{
				language: "go", path: "/p/main.go", root: "/p",
				text: "package sample\n", chain: []string{"Service", "Run"},
			},
			want: "sample.Service.Run",
		},
		{
			name: "cpp member",
			input: referenceInput{
				language: "cpp", path: "/p/main.cpp", root: "/p",
				chain: []string{"Widget", "run"},
			},
			want: "Widget::run",
		},
		{
			name: "fallback line",
			input: referenceInput{
				language: "java", path: "/p/src/Main.java", root: "/p",
				line: 11,
			},
			want: "src/Main.java:12",
		},
	}
	for _, test := range cases {
		if got := qualifiedReference(test.input); got != test.want {
			t.Errorf("%s = %q, want %q", test.name, got, test.want)
		}
	}
}
