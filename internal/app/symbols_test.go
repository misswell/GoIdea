package app

import (
	"strings"
	"testing"

	"mayoi/internal/openidea"
)

func TestClampSymbolSel(t *testing.T) {
	cases := []struct{ sel, n, want int }{
		{0, 0, 0},
		{-3, 5, 0},
		{2, 5, 2},
		{4, 5, 4},
		{5, 5, 4},
		{99, 5, 4},
	}
	for _, c := range cases {
		if got := clampSymbolSel(c.sel, c.n); got != c.want {
			t.Fatalf("clampSymbolSel(%d, %d) = %d, want %d", c.sel, c.n, got, c.want)
		}
	}
}

func TestSymbolKindLabel(t *testing.T) {
	cases := map[string]string{
		"class":     "类",
		"interface": "接口",
		"method":    "方法",
		"field":     "字段",
		"constant":  "常量",
		"enum":      "枚举",
	}
	for kind, want := range cases {
		if got := symbolKindLabel(kind); !strings.Contains(got, want) {
			t.Fatalf("%s is %q", kind, got)
		}
	}
	if got := symbolKindLabel("somethingNew"); got != "符号" {
		t.Fatalf("unknown kind is %q", got)
	}
}

func TestSymbolWhere(t *testing.T) {
	got := symbolWhere("/p", "/p/src/A.java", 4)
	if got != "src/A.java:5" {
		t.Fatalf("relative path is %q", got)
	}
	// A file outside the root keeps its full path, and a jar-backed location
	// has no relative form.
	if got := symbolWhere("/p", "/other/B.java", 0); got != "/other/B.java:1" {
		t.Fatalf("outside path is %q", got)
	}
	if got := symbolWhere("", "jdt://contents/A.class", 9); got != "jdt://contents/A.class:10" {
		t.Fatalf("uri is %q", got)
	}
}

func TestApplySymbolClosesPicker(t *testing.T) {
	m := &Model{symbols: &symbolPicker{open: true, query: "Smoke"}}
	m.applySymbol(openidea.WorkspaceSymbol{
		Name:     "Smoke",
		Kind:     "class",
		Location: openidea.Location{Path: "jdt://contents/Smoke.class"},
	})
	if m.symbols != nil {
		t.Fatalf("picker stayed open")
	}
	if !strings.Contains(m.status, "外部依赖") {
		t.Fatalf("status is %q", m.status)
	}
}
