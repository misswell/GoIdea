package app

import "github.com/egoist/mygo/ui"

// oiColors are OpenIDEA's Islands design tokens that ui.Theme has no field
// for. Values are the ones in third_party/openidea/src/styles/tokens.css,
// dark in :root and light in [data-theme="light"].
type oiColors struct {
	dark bool

	island, elev, frame, popup ui.Color
	hover, pressed             ui.Color
	border, borderStrong       ui.Color
	textStrong, text           ui.Color
	textSecondary, textMuted   ui.Color
	textSubtle, textFaint      ui.Color
	accent, link               ui.Color
	danger, warning, success   ui.Color
	selected, selectedInactive ui.Color
	hoverTint                  ui.Color
	buttonBorder, buttonHover  ui.Color
	tooltip, tooltipText       ui.Color
	scrollbar                  ui.Color
}

// openideaColors returns the Islands palette of the dark or light theme.
func openideaColors(dark bool) oiColors {
	if dark {
		return oiColors{
			dark:             true,
			island:           ui.Hex("#191a1c"),
			elev:             ui.Hex("#212326"),
			frame:            ui.Hex("#26282c"),
			popup:            ui.Hex("#26282c"),
			hover:            ui.Hex("#2e2f30"),
			pressed:          ui.Hex("#40434a"),
			border:           ui.Hex("#26282c"),
			borderStrong:     ui.Hex("#40434a"),
			textStrong:       ui.Hex("#dfe1e5"),
			text:             ui.Hex("#d1d3d9"),
			textSecondary:    ui.Hex("#9fa2a8"),
			textMuted:        ui.Hex("#8c8f95"),
			textSubtle:       ui.Hex("#73767c"),
			textFaint:        ui.Hex("#5f6269"),
			accent:           ui.Hex("#3871e1"),
			link:             ui.Hex("#71a1fe"),
			danger:           ui.Hex("#f57e84"),
			warning:          ui.Hex("#d59637"),
			success:          ui.Hex("#6db083"),
			selected:         ui.Hex("#2a4371"),
			selectedInactive: ui.Hex("#33353b"),
			hoverTint:        ui.RGBA(255, 255, 255, 0.062),
			buttonBorder:     ui.Hex("#40434a"),
			buttonHover:      ui.RGBA(255, 255, 255, 0.06),
			tooltip:          ui.Hex("#33353b"),
			tooltipText:      ui.Hex("#d1d3d9"),
			scrollbar:        ui.RGBA(255, 255, 255, 0.2),
		}
	}
	return oiColors{
		island:           ui.Hex("#ffffff"),
		elev:             ui.Hex("#f7f8f9"),
		frame:            ui.Hex("#e9eaee"),
		popup:            ui.Hex("#ffffff"),
		hover:            ui.Hex("#ededed"),
		pressed:          ui.Hex("#e2e5ea"),
		border:           ui.Hex("#e9eaee"),
		borderStrong:     ui.Hex("#d1d3d9"),
		textStrong:       ui.Hex("#000000"),
		text:             ui.Hex("#000000"),
		textSecondary:    ui.Hex("#5f6269"),
		textMuted:        ui.Hex("#6c707e"),
		textSubtle:       ui.Hex("#73767c"),
		textFaint:        ui.Hex("#9fa2a8"),
		accent:           ui.Hex("#3871e1"),
		link:             ui.Hex("#2f5eb9"),
		danger:           ui.Hex("#c54e58"),
		warning:          ui.Hex("#a56906"),
		success:          ui.Hex("#338555"),
		selected:         ui.Hex("#d0dffe"),
		selectedInactive: ui.Hex("#e9eaee"),
		hoverTint:        ui.RGBA(0, 0, 0, 0.031),
		buttonBorder:     ui.Hex("#d1d3d9"),
		buttonHover:      ui.RGBA(0, 0, 0, 0.06),
		tooltip:          ui.Hex("#33353b"),
		tooltipText:      ui.Hex("#d1d3d9"),
		scrollbar:        ui.RGBA(0, 0, 0, 0.2),
	}
}

// oiPaletteOf is the Islands palette of the theme in use.
func oiPaletteOf(p *ui.Theme) oiColors { return openideaColors(p != nil && p.Dark) }

// openideaTheme is the palette OpenIDEA draws with: the dark workbench of a
// New UI window, and its light counterpart, mapped onto ui.Theme's fields.
func openideaTheme(dark bool) *ui.Theme {
	oi := openideaColors(dark)
	accentHover := oi.accent.Mix(oi.textStrong, 0.14)
	accentPressed := oi.accent.Mix(ui.Hex("#000000"), 0.2)
	return &ui.Theme{
		Dark:           dark,
		Background:     oi.island,
		Surface:        oi.elev,
		SurfaceHover:   oi.hover,
		SurfacePressed: oi.pressed,
		Border:         oi.border,
		Text:           oi.text,
		TextMuted:      oi.textMuted,
		Accent:         oi.accent,
		AccentHover:    accentHover,
		AccentPressed:  accentPressed,
		AccentText:     ui.Hex("#ffffff"),
		Danger:         oi.danger,
		Warning:        oi.warning,
		Success:        oi.success,
		Selection:      oi.selected,
		Focus:          ui.RGBA(56, 113, 225, 0.6),
		Inverse:        oi.tooltip,
		InverseText:    oi.tooltipText,
		Scrollbar:      oi.scrollbar,
		ScrollbarWidth: 8,
		Radius:         4,
		Spacing:        4,
		FontSize:       13,
	}
}

// projectColorPairs are RecentProject.Color1–9.Avatar Start/End, identical
// in the light and dark themes.
var projectColorPairs = [9][2]ui.Color{
	{ui.Hex("#e08855"), ui.Hex("#e9806f")},
	{ui.Hex("#b08b14"), ui.Hex("#bb7f19")},
	{ui.Hex("#a1a359"), ui.Hex("#87aa59")},
	{ui.Hex("#3b92b8"), ui.Hex("#6183ec")},
	{ui.Hex("#3574f0"), ui.Hex("#7a64f0")},
	{ui.Hex("#c84d8f"), ui.Hex("#a956cf")},
	{ui.Hex("#955ae0"), ui.Hex("#a84de0")},
	{ui.Hex("#24a394"), ui.Hex("#279ccd")},
	{ui.Hex("#5fad65"), ui.Hex("#3d968b")},
}

// projectColorIndex is OpenIDEA's stable 1-based colour slot for a project
// name, hashed the same way so both apps paint a project alike.
func projectColorIndex(name string) int {
	var h int32
	for _, r := range name {
		h = h*31 + int32(r)
	}
	if h < 0 {
		h = -h
	}
	return int(h%9) + 1
}

// projectColors returns the gradient start and end of a project's avatar.
func projectColors(name string) (start, end ui.Color) {
	pair := projectColorPairs[projectColorIndex(name)-1]
	return pair[0], pair[1]
}
