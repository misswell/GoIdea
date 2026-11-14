package app

// The shared states of the native component system: empty, loading and error
// fill a pane with one centered line. Features use these instead of each
// drawing its own hint, so the three states look the same everywhere and take
// their size and colours from the tokens.

import "github.com/egoist/mygo/ui"

// stateView is a pane's one-line state: text centered in height DIPs.
func stateView(c *ui.Context, p *ui.Theme, text string, tone ui.Color, height float32) {
	ui.Row(c).Height(height).Shrink(0).Grow(1).Justify(ui.Center).
		AlignItems(ui.Center).Children(func() {
		ui.Text(c, text).FontSize(oiFontSizeM).TextColor(tone).SingleLine().Ellipsis("…")
	})
}

// emptyState fills a pane that has nothing to show yet.
func emptyState(c *ui.Context, p *ui.Theme, text string) {
	stateView(c, p, text, p.TextMuted, 120)
}

// loadingState fills a pane whose data is still arriving.
func loadingState(c *ui.Context, p *ui.Theme, text string) {
	stateView(c, p, text, p.TextMuted, 120)
}

// errorState fills a pane whose data failed, in the palette's danger colour.
func errorState(c *ui.Context, p *ui.Theme, text string) {
	stateView(c, p, text, oiPaletteOf(p).danger, 120)
}
