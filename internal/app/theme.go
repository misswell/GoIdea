package app

import "github.com/egoist/mygo/ui"

// openideaTheme is the palette OpenIDEA draws with, its theme ported to the
// widgets GoIDEA builds its window from: the dark workbench of a New UI
// window, and its light counterpart.
func openideaTheme(dark bool) *ui.Theme {
	if !dark {
		return &ui.Theme{
			Background:     ui.Hex("#ffffff"),
			Surface:        ui.Hex("#f7f8fa"),
			SurfaceHover:   ui.Hex("#eef1f5"),
			SurfacePressed: ui.Hex("#e2e5ea"),
			Border:         ui.Hex("#dfe1e5"),
			Text:           ui.Hex("#1e1f22"),
			TextMuted:      ui.Hex("#6c707e"),
			Accent:         ui.Hex("#3574f0"),
			AccentHover:    ui.Hex("#4a80f5"),
			AccentPressed:  ui.Hex("#2f65d9"),
			AccentText:     ui.Hex("#ffffff"),
			Danger:         ui.Hex("#c93c3c"),
			Warning:        ui.Hex("#b87b11"),
			Success:        ui.Hex("#2f8f46"),
			Selection:      ui.RGBA(53, 116, 240, 0.22),
			Focus:          ui.RGBA(53, 116, 240, 0.55),
			Scrollbar:      ui.RGBA(0, 0, 0, 0.26),
			ScrollbarWidth: 8,
			Radius:         4,
			Spacing:        4,
			FontSize:       13,
		}
	}
	return &ui.Theme{
		Dark:           true,
		Background:     ui.Hex("#1e1f22"),
		Surface:        ui.Hex("#2b2d30"),
		SurfaceHover:   ui.Hex("#35373b"),
		SurfacePressed: ui.Hex("#393b40"),
		Border:         ui.Hex("#393b40"),
		Text:           ui.Hex("#dfe1e5"),
		TextMuted:      ui.Hex("#868a91"),
		Accent:         ui.Hex("#3574f0"),
		AccentHover:    ui.Hex("#4a80f5"),
		AccentPressed:  ui.Hex("#2f65d9"),
		AccentText:     ui.Hex("#ffffff"),
		Danger:         ui.Hex("#db5c5c"),
		Warning:        ui.Hex("#e0a33e"),
		Success:        ui.Hex("#5fad65"),
		Selection:      ui.Hex("#2e436e"),
		Focus:          ui.RGBA(53, 116, 240, 0.6),
		Scrollbar:      ui.RGBA(255, 255, 255, 0.26),
		ScrollbarWidth: 8,
		Radius:         4,
		Spacing:        4,
		FontSize:       13,
	}
}
