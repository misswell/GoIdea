package app

// OpenIDEA's typography: Inter for the interface and JetBrains Mono for code,
// the families tokens.css names. They are embedded rather than taken from the
// system so both appearances look the same on every machine, as the bundled
// @fontsource packages do in the browser.

import (
	_ "embed"
	"log"

	"github.com/egoist/mygo/ui"
)

const (
	oiFontUI   = "Inter"
	oiFontMono = "JetBrains Mono"
)

//go:embed assets/fonts/Inter-Regular.ttf
var fontInterRegular []byte

//go:embed assets/fonts/Inter-Medium.ttf
var fontInterMedium []byte

//go:embed assets/fonts/Inter-SemiBold.ttf
var fontInterSemiBold []byte

//go:embed assets/fonts/Inter-Bold.ttf
var fontInterBold []byte

//go:embed assets/fonts/JetBrainsMono-Regular.ttf
var fontJetBrainsRegular []byte

//go:embed assets/fonts/JetBrainsMono-Medium.ttf
var fontJetBrainsMedium []byte

//go:embed assets/fonts/JetBrainsMono-Bold.ttf
var fontJetBrainsBold []byte

// oiFontFaces registers under one family per typeface; the engine picks the
// face whose weight the style asks for, so a bold label is Inter Bold rather
// than a thickened regular.
var oiFontFaces = []struct {
	data   []byte
	family string
}{
	{fontInterRegular, oiFontUI},
	{fontInterMedium, oiFontUI},
	{fontInterSemiBold, oiFontUI},
	{fontInterBold, oiFontUI},
	{fontJetBrainsRegular, oiFontMono},
	{fontJetBrainsMedium, oiFontMono},
	{fontJetBrainsBold, oiFontMono},
}

func init() {
	for _, f := range oiFontFaces {
		if err := ui.RegisterFont(f.data, f.family); err != nil {
			log.Printf("goidea: registering %s: %v", f.family, err)
		}
	}
}

// editorLineHeight is OpenIDEA's spacing for an editor font size: --lh-code
// (22px at the 13px default) grows by 1.6px a point, as the editor's zoom
// steps do, kept as the multiple of the size MyGo's editor takes.
func editorLineHeight(size float32) float32 {
	if size <= 0 {
		size = 13
	}
	return (22 + (size-13)*1.6) / size
}
