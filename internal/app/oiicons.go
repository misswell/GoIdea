package app

// OpenIDEA's icon set: the JetBrains expui SVGs it vendors, embedded as-is
// and drawn the way its React Icon does. Monochrome icons are recolored to
// the text color and take the caller's tone; colored ones (file types, run
// and debug controls, the vcs chevron) keep the palette of the variant of
// the theme in use. `size >= 18` picks the @20x20 drawings the stripes use,
// as upstream does.

import (
	"embed"
	"io/fs"
	"regexp"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"
)

//go:embed assets/icons
var oiIconAssets embed.FS

// oiMonoLight and oiMonoDark are the greys upstream's icon build treats as
// monochrome; an icon with any other colour keeps its own palette.
const (
	oiMonoLight = "#6c707e"
	oiMonoDark  = "#ced0d6"
)

var (
	oiColorRE   = regexp.MustCompile(`#[0-9a-fA-F]{3,8}\b`)
	oiCommentRE = regexp.MustCompile(`(?s)<!--.*?-->`)
	oiSpaceRE   = regexp.MustCompile(`>\s+<`)

	oiIconMu    sync.Mutex
	oiIconCache = map[oiIconKey]*oiIcon{}
)

type oiIconKey struct {
	name  string
	large bool
	dark  bool
}

type oiIcon struct {
	svg  *ui.SVG
	mono bool
}

// oiIconOf returns the icon upstream names `name` ("toolwindows/project",
// "fileTypes/java", "openidea/nodes/testResourcesRoot"), parsed for the
// theme, or nil when the build has no such drawing.
func oiIconOf(name string, size float32, dark bool) *oiIcon {
	large := size >= 18
	key := oiIconKey{name: name, large: large, dark: dark}
	oiIconMu.Lock()
	defer oiIconMu.Unlock()
	if ic, ok := oiIconCache[key]; ok {
		return ic
	}
	ic := oiLoadIcon(key)
	oiIconCache[key] = ic
	return ic
}

func oiLoadIcon(key oiIconKey) *oiIcon {
	base := "assets/icons/expui/" + key.name
	if strings.HasPrefix(key.name, "openidea/") {
		base = "assets/icons/" + key.name
	}
	file := base + ".svg"
	if key.large {
		if _, err := fs.Stat(oiIconAssets, base+"@20x20.svg"); err == nil {
			file = base + "@20x20.svg"
		}
	}
	darkFile := strings.TrimSuffix(file, ".svg") + "_dark.svg"
	lightSrc, err := oiIconAssets.ReadFile(file)
	if err != nil {
		return nil
	}
	darkSrc, darkErr := oiIconAssets.ReadFile(darkFile)
	mono := oiIsMono(lightSrc) && (darkErr != nil || oiIsMonoDark(darkSrc))
	src := lightSrc
	if key.dark && darkErr == nil {
		src = darkSrc
	}
	svg, err := oiParseIcon(src, mono)
	if err != nil {
		return nil
	}
	return &oiIcon{svg: svg, mono: mono}
}

// oiSVGColors is the set of colour literals an SVG body paints with.
func oiSVGColors(src []byte) map[string]bool {
	out := map[string]bool{}
	for _, m := range oiColorRE.FindAll(src, -1) {
		out[strings.ToLower(string(m))] = true
	}
	return out
}

func oiIsMono(src []byte) bool {
	colors := oiSVGColors(src)
	if len(colors) == 0 {
		return false
	}
	for c := range colors {
		if c != oiMonoLight {
			return false
		}
	}
	return true
}

func oiIsMonoDark(src []byte) bool {
	colors := oiSVGColors(src)
	if len(colors) == 0 {
		return false
	}
	for c := range colors {
		if c != oiMonoDark {
			return false
		}
	}
	return true
}

// oiParseIcon cleans the drawing the way upstream's build does and parses it;
// a monochrome icon is rewritten to currentColor so the caller's tone paints
// it.
func oiParseIcon(src []byte, mono bool) (*ui.SVG, error) {
	body := oiCommentRE.ReplaceAll(src, nil)
	body = oiSpaceRE.ReplaceAll(body, []byte("><"))
	if mono {
		body = oiColorRE.ReplaceAll(body, []byte("currentColor"))
	}
	return ui.ParseSVG(body)
}

// oiIconEl is an icon element: monochrome icons take tint, colored ones draw
// their palette.
func oiIconEl(c *ui.Context, name string, size float32, oi oiColors, tint ui.Color) ui.Element {
	ic := oiIconOf(name, size, oi.dark)
	if ic == nil {
		return ui.Element{}
	}
	if ic.mono {
		return ui.Icon(c, ic.svg).Size(size, size).TextColor(tint)
	}
	return ui.Image(c, ic.svg).Size(size, size)
}

// oiIconExists reports whether the build carries the drawing, which the icon
// parity test checks for every name the chrome uses.
func oiIconExists(name string, size float32) bool {
	return oiIconOf(name, size, false) != nil
}
