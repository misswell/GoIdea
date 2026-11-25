package app

// The shared states of the native component system: empty, loading and error
// fill a pane with one centered line. Features use these instead of each
// drawing its own hint, so the three states look the same everywhere and take
// their size and colours from the tokens.

import "github.com/egoist/mygo/ui"

// stateView is a pane's centered state. Loading states pair the message with
// OpenIDEA's spinner; empty and error states use a 24 DIP content inset.
func stateView(c *ui.Context, p *ui.Theme, text string, tone ui.Color, loading bool) {
	ui.Column(c).Key("state:" + text).Grow(1).Shrink(0).MinHeight(120).
		Padding(24).Gap(oiSpaceL).Justify(ui.Center).AlignItems(ui.Center).
		Children(func() {
			if loading {
				ui.Spinner(c).Size(18, 18).TextColor(tone).Label(text)
			}
			ui.Text(c, text).FontSize(oiFontSizeM).TextColor(tone).
				MaxWidth(480).Wrap()
		})
}

// emptyState fills a pane that has nothing to show yet.
func emptyState(c *ui.Context, p *ui.Theme, text string) {
	stateView(c, p, text, p.TextMuted, false)
}

// loadingState fills a pane whose data is still arriving.
func loadingState(c *ui.Context, p *ui.Theme, text string) {
	stateView(c, p, text, p.TextMuted, true)
}

// errorState fills a pane whose data failed, in the palette's danger colour.
func errorState(c *ui.Context, p *ui.Theme, text string) {
	stateView(c, p, text, oiPaletteOf(p).danger, false)
}
