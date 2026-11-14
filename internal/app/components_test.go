package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestStateComponentsRenderTheirLine(t *testing.T) {
	cases := []struct {
		name  string
		state func(c *ui.Context, p *ui.Theme, text string)
		text  string
	}{
		{"empty", emptyState, "Nothing here yet"},
		{"loading", loadingState, "Working"},
		{"error", errorState, "It broke"},
	}
	for _, c := range cases {
		c := c
		tt := ui.NewTester(func(ctx *ui.Context) {
			c.state(ctx, ctx.Theme(), c.text)
		}, 400, 200)
		tt.Frame()
		if !tt.HasText(c.text) {
			t.Fatalf("%s state did not draw %q: %q", c.name, c.text, tt.Texts())
		}
	}
}

func TestSearchEverywhereEmptyState(t *testing.T) {
	m := &Model{}
	m.OpenSearchEverywhere()
	defer m.CloseSearchEverywhere()
	tt := ui.NewTester(func(c *ui.Context) { m.View(c) }, 1440, 900)
	tt.Frame()
	if !tt.HasText("输入以搜索类、文件、符号、操作和文本") {
		t.Fatalf("the empty Search Everywhere has no hint: %q", tt.Texts())
	}
}
