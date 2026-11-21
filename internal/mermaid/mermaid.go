// Package mermaid renders Mermaid diagrams with Pele and draws them through
// MyGo's native canvas. It has no browser or WebView dependency.
package mermaid

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MoeForge/moejs"
	"github.com/egoist/mygo/ui"

	"mayoi/locales"
)

// peleJS is Pele, which lays Mermaid diagrams out and draws them as SVG,
// bundled by scripts/pele.sh into one script that sets the global Pele.
//
//go:embed pele.js.gz
var peleJS []byte

// Style is what a diagram is drawn in: the colors of the theme
// that Pele's CSS variables take, and the fonts of its labels.
type Style struct {
	BG, FG, Muted, Line, Surface, SurfaceAlt, Border, Accent, Danger ui.Color
	// Sans is the labels' font, "" for the window's; Mono and MonoWeight
	// are those of their code.
	Sans, Mono string
	MonoWeight int
}

// Font is the font of a label's text in Pele's style, of its bits:
// bold, italic, code, and the smaller text of a subscript or superscript.
func (st Style) Font(size float32, style int) ui.Font {
	f := ui.Font{Family: st.Sans, Size: size, Weight: 400}
	if style&pelebold != 0 {
		f.Weight = 700
	}
	f.Italic = style&peleItalic != 0
	if style&peleMono != 0 {
		f.Family, f.Weight = st.Mono, st.MonoWeight
	}
	if style&(peleSub|peleSup) != 0 {
		f.Size *= mermaidSmaller
	}
	return f
}

// The bits of a text's style that Pele measures it in.
const (
	pelebold   = 1
	peleItalic = 2
	peleMono   = 4
	peleSub    = 8
	peleSup    = 16
)

// mermaidSmaller is the size of a subscript or a superscript, of the
// text's.
const mermaidSmaller = 0.8

// Diagram is a diagram drawn: its shapes, which an SVG draws, and
// its labels, which the window's text draws, as MyGo's SVGs have no text.
type Diagram struct {
	// SVG is the whole of its shapes, and Tiles the pieces they are
	// drawn in, as MyGo draws no SVG past 4096 pixels.
	SVG    *ui.SVG
	Tiles  []Tile
	W, H   float32
	Labels []Label
	// Shapes is the SVG SVG draws, and Style what the diagram was drawn
	// in, which its copies as an image and its exports draw it from.
	Shapes []byte
	Style  Style
	// Err is why the diagram could not be drawn, "" when it was.
	Err string
	// Shaped are the labels shaped at scale, the scale the diagram was
	// drawn at last, for the UI thread that draws it.
	Shaped []*ui.TextLayout
	Scale  float32
}

// Label is a run of a label's text in font, with its top left at
// x, y of the diagram, turned by angle degrees around it.
type Label struct {
	Text  string
	Font  ui.Font
	X, Y  float32
	Angle float32
	Color ui.Color
}

// Tile is a piece of a diagram's shapes, the part of it at X, Y,
// w×h of the diagram.
type Tile struct {
	SVG        *ui.SVG
	X, Y, W, H float32
}

// mermaidTileSide is the longest side of a tile, in DIPs: under 4096
// pixels at 4×, as a 2× display shows a diagram zoomed to 2.
const mermaidTileSide = 1024

// Draw draws the diagram in r, its labels shaped at the size they show
// at, which keeps them sharp as the diagram is zoomed.
func (d *Diagram) Draw(pt *ui.Painter, r ui.Rect) {
	if d.SVG == nil || d.W <= 0 || r.W <= 0 {
		return
	}
	s := r.W / d.W
	for _, t := range d.Tiles {
		// Each tile from where the next one starts, so they meet.
		x0, y0 := r.X+t.X*s, r.Y+t.Y*s
		pt.Image(t.SVG, ui.Rect{X: x0, Y: y0, W: r.X + (t.X+t.W)*s - x0, H: r.Y + (t.Y+t.H)*s - y0}, ui.FillBox)
	}
	if size := float32(math.Round(float64(s)*100) / 100); size != d.Scale || len(d.Shaped) != len(d.Labels) {
		d.Scale, d.Shaped = size, make([]*ui.TextLayout, len(d.Labels))
		for i, l := range d.Labels {
			f := l.Font
			f.Size *= max(size, 0.01)
			d.Shaped[i] = ui.ShapeText(l.Text, f, 0)
		}
	}
	for i, l := range d.Labels {
		x, y := r.X+l.X*s, r.Y+l.Y*s
		if l.Angle == 0 {
			pt.TextLayout(d.Shaped[i], x, y, l.Color)
			continue
		}
		pt.Transform(ui.Rotation(l.Angle), x, y, func() { pt.TextLayout(d.Shaped[i], x, y, l.Color) })
	}
}

// mermaidTimeout is how long Pele may take with a diagram.
const mermaidTimeout = 5 * time.Second

// pele is the JavaScript runtime that runs Pele, started as the first
// diagram is drawn, one diagram at a time.
var pele struct {
	sync.Mutex
	js     *jsEngine
	render moejs.Value
	err    error
	// style is the style of the diagram being drawn, whose fonts measure
	// its text, and measured that of the widths Pele keeps.
	style, measured Style
}

// peleStart starts the runtime with Pele, and a render(source, fresh)
// that draws a diagram as SVG, measuring its text in the window's fonts,
// and forgetting the widths it measured before when fresh.
func peleStart() error {
	zr, err := gzip.NewReader(bytes.NewReader(peleJS))
	if err != nil {
		return err
	}
	script, err := io.ReadAll(zr)
	if err != nil {
		return err
	}
	measure := moejs.NativeFunc(func(r *moejs.Realm, this moejs.Value, args []moejs.Value) (moejs.Value, error) {
		text, _ := pele.js.rt.ToGo(moejs.Arg(args, 0))
		s, _ := text.(string)
		size, style := moejs.Arg(args, 1).AsNumber(), int(moejs.Arg(args, 2).AsNumber())
		w, _ := ui.ShapeText(s, pele.style.Font(float32(size), style), 0).Size()
		return pele.js.rt.FromGo(float64(w))
	})
	js, err := newJSEngine("pele.js", string(script), map[string]any{"mayoiMeasure": measure})
	if err != nil {
		return err
	}
	pele.js = js
	fn, err := js.eval("pele-render.js", `(function () {
	var measurer = {width: function (text, size, style) { return mayoiMeasure(String(text), Number(size), Number(style) || 0) }};
	return function (source, fresh) {
		if (fresh) Pele.forgetTextWidths();
		return Pele.render(source, {measurer: measurer}).svg;
	};
})()`)
	if err != nil {
		return err
	}
	if !fn.IsObject() || !fn.AsObject().IsCallable() {
		return errors.New("pele: no render")
	}
	pele.render = fn
	return nil
}

// peleRender draws the diagram of source as SVG, its text measured in
// the fonts of st, or says what is wrong with it.
func peleRender(source string, st Style) (svg string, err error) {
	pele.Lock()
	defer pele.Unlock()
	if pele.js == nil && pele.err == nil {
		pele.err = peleStart()
	}
	if pele.err != nil {
		return "", pele.err
	}
	pele.style = st
	fresh := st.Sans != pele.measured.Sans || st.Mono != pele.measured.Mono || st.MonoWeight != pele.measured.MonoWeight
	pele.measured = st
	out, err := pele.js.callString(pele.render, mermaidTimeout, source, fresh)
	if err != nil {
		return "", peleError(pele.js, err)
	}
	return out, nil
}

// peleError is what Pele threw, as one line: its message without the
// snippet of the source that shows where.
func peleError(js *jsEngine, err error) error {
	if timedOut(err) {
		return errors.New(locales.ErrMermaidTimeout())
	}
	msg, snippet := js.thrown(err)
	if snippet != "" {
		msg = strings.Replace(msg, snippet, " ", 1)
	}
	return errors.New(oneLine(msg))
}

// Render draws the diagram of source in st.
func Render(source string, st Style) *Diagram {
	svg, err := peleRender(source, st)
	if err != nil {
		return &Diagram{Err: err.Error()}
	}
	shapes, texts, err := mermaidSVG(svg, st)
	if err != nil {
		return &Diagram{Err: err.Error()}
	}
	s, err := ui.ParseSVG(shapes)
	if err != nil {
		return &Diagram{Err: err.Error()}
	}
	d := &Diagram{SVG: s, Shapes: shapes, Style: st}
	if d.W, d.H = s.Size(); d.W <= 0 || d.H <= 0 {
		return &Diagram{Err: locales.ErrSVGNoSize()}
	}
	if d.Tiles, err = mermaidTiles(shapes, s, d.W, d.H); err != nil {
		return &Diagram{Err: err.Error()}
	}
	for _, t := range texts {
		d.Labels = append(d.Labels, t.shape(st)...)
	}
	return d
}

// mermaidTiles cuts the shapes of a diagram w×h, which s draws whole, into
// tiles no side of which is longer than mermaidTileSide.
func mermaidTiles(shapes []byte, s *ui.SVG, w, h float32) ([]Tile, error) {
	if w <= mermaidTileSide && h <= mermaidTileSide {
		return []Tile{{SVG: s, W: w, H: h}}, nil
	}
	open := bytes.Index(shapes, []byte("<svg"))
	end := bytes.IndexByte(shapes[max(open, 0):], '>')
	if open < 0 || end < 0 {
		return []Tile{{SVG: s, W: w, H: h}}, nil
	}
	end += open
	root := string(shapes[open:end])
	vx, vy, vw, vh := float32(0), float32(0), w, h
	if m := svgViewBox.FindStringSubmatch(root); m != nil {
		if f := strings.FieldsFunc(m[1], func(r rune) bool { return r == ',' || r == ' ' }); len(f) == 4 {
			var n [4]float32
			ok := true
			for i, v := range f {
				x, err := strconv.ParseFloat(v, 32)
				n[i], ok = float32(x), ok && err == nil
			}
			if ok && n[2] > 0 && n[3] > 0 {
				vx, vy, vw, vh = n[0], n[1], n[2], n[3]
			}
		}
	}
	root = svgSizeAttr.ReplaceAllString(root, "")
	var tiles []Tile
	for y := float32(0); y < h; y += mermaidTileSide {
		for x := float32(0); x < w; x += mermaidTileSide {
			tw, th := min(mermaidTileSide, w-x), min(mermaidTileSide, h-y)
			var b bytes.Buffer
			b.Write(shapes[:open])
			fmt.Fprintf(&b, `%s viewBox="%g %g %g %g" width="%g" height="%g"`, root,
				vx+x*vw/w, vy+y*vh/h, tw*vw/w, th*vh/h, tw, th)
			b.Write(shapes[end:])
			t, err := ui.ParseSVG(b.Bytes())
			if err != nil {
				return nil, err
			}
			tiles = append(tiles, Tile{SVG: t, X: x, Y: y, W: tw, H: th})
		}
	}
	return tiles, nil
}

var (
	svgViewBox  = regexp.MustCompile(`\sviewBox="([^"]*)"`)
	svgSizeAttr = regexp.MustCompile(`\s(viewBox|width|height|preserveAspectRatio)="[^"]*"`)
)

// StripSVGSizeAttributes removes the width and placement attributes so an
// embedding SVG can set them for its export.
func StripSVGSizeAttributes(source string) string {
	return svgSizeAttr.ReplaceAllString(source, "")
}

// XMLEscape escapes text for an SVG text node.
func XMLEscape(source string) string { return xmlEscape(source) }

// XMLEscapeTo appends source escaped for an SVG text node.
func XMLEscapeTo(dst *bytes.Buffer, source string) { xmlEscapeTo(dst, source) }

// mermaidRun is a run of a label's text in one style.
type mermaidRun struct {
	text  string
	size  float32
	style int
	color ui.Color
}

// mermaidText is a chunk of a label's text, the runs that follow each
// other from x, y, its baseline, placed as anchor says: "start",
// "middle" or "end". m places it in the diagram.
type mermaidText struct {
	m      affine
	x, y   float32
	anchor string
	runs   []mermaidRun
}

// shape shapes the runs of the chunk, and places them in the diagram.
func (t mermaidText) shape(st Style) []Label {
	type piece struct {
		text   string
		font   ui.Font
		x, top float32
		color  ui.Color
	}
	var pieces []piece
	pen := float32(0)
	for _, r := range t.runs {
		if r.text == "" {
			continue
		}
		f := st.Font(r.size, r.style)
		l := ui.ShapeText(r.text, f, 0)
		w, h := l.Size()
		m := f.Metrics()
		// The baseline is where the line's room above the ascent ends,
		// that room half of what it has besides the ascent and descent.
		base := (h-m.Ascent-m.Descent)/2 + m.Ascent
		shift := float32(0)
		switch {
		case r.style&peleSub != 0:
			shift = r.size * 0.2
		case r.style&peleSup != 0:
			shift = -r.size * 0.35
		}
		pieces = append(pieces, piece{r.text, f, pen, shift - base, r.color})
		pen += w
	}
	left := t.x
	switch t.anchor {
	case "middle":
		left -= pen / 2
	case "end":
		left -= pen
	}
	angle := float32(math.Atan2(float64(t.m.b), float64(t.m.a)) * 180 / math.Pi)
	if math.Abs(float64(angle)) < 0.01 {
		angle = 0
	}
	labels := make([]Label, 0, len(pieces))
	for _, p := range pieces {
		x, y := t.m.apply(left+p.x, t.y+p.top)
		labels = append(labels, Label{Text: p.text, Font: p.font, X: x, Y: y, Angle: angle, Color: p.color})
	}
	return labels
}

// affine is a 2D transform, as SVG's matrix(a b c d e f).
type affine struct{ a, b, c, d, e, f float32 }

var identity = affine{a: 1, d: 1}

// then is m followed by n.
func (m affine) then(n affine) affine {
	return affine{
		a: n.a*m.a + n.c*m.b, b: n.b*m.a + n.d*m.b,
		c: n.a*m.c + n.c*m.d, d: n.b*m.c + n.d*m.d,
		e: n.a*m.e + n.c*m.f + n.e, f: n.b*m.e + n.d*m.f + n.f,
	}
}

func (m affine) apply(x, y float32) (float32, float32) {
	return m.a*x + m.c*y + m.e, m.b*x + m.d*y + m.f
}

// parseTransform parses an SVG transform, a list of translate, scale,
// rotate, skewX, skewY and matrix, into the transform it makes.
func parseTransform(s string) affine {
	m := identity
	for {
		open := strings.IndexByte(s, '(')
		end := strings.IndexByte(s, ')')
		if open < 0 || end < open {
			return m
		}
		name := strings.TrimSpace(strings.Trim(s[:open], " ,\t\n"))
		var n []float32
		for _, f := range strings.FieldsFunc(s[open+1:end], func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' }) {
			if v, err := strconv.ParseFloat(f, 32); err == nil {
				n = append(n, float32(v))
			}
		}
		arg := func(i int, def float32) float32 {
			if i < len(n) {
				return n[i]
			}
			return def
		}
		var t affine
		switch name {
		case "translate":
			t = affine{a: 1, d: 1, e: arg(0, 0), f: arg(1, 0)}
		case "scale":
			t = affine{a: arg(0, 1), d: arg(1, arg(0, 1))}
		case "rotate":
			sin, cos := math.Sincos(float64(arg(0, 0)) * math.Pi / 180)
			cx, cy := arg(1, 0), arg(2, 0)
			t = affine{a: 1, d: 1, e: -cx, f: -cy}.
				then(affine{a: float32(cos), b: float32(sin), c: float32(-sin), d: float32(cos)}).
				then(affine{a: 1, d: 1, e: cx, f: cy})
		case "skewX":
			t = affine{a: 1, c: float32(math.Tan(float64(arg(0, 0)) * math.Pi / 180)), d: 1}
		case "skewY":
			t = affine{a: 1, b: float32(math.Tan(float64(arg(0, 0)) * math.Pi / 180)), d: 1}
		case "matrix":
			t = affine{arg(0, 1), arg(1, 0), arg(2, 0), arg(3, 1), arg(4, 0), arg(5, 0)}
		default:
			t = identity
		}
		// Each transform of the list applies within those before it.
		m = t.then(m)
		s = s[end+1:]
	}
}

// pelePalette is the value of Pele's CSS variables in a style, those it
// leaves out falling back to Pele's own.
func (st Style) pelePalette() map[string]string {
	return map[string]string{
		"--pele-bg": HexColor(st.BG), "--pele-fg": HexColor(st.FG), "--pele-muted": HexColor(st.Muted),
		"--pele-line": HexColor(st.Line), "--pele-surface": HexColor(st.Surface),
		"--pele-surface-alt": HexColor(st.SurfaceAlt), "--pele-border": HexColor(st.Border),
		"--pele-accent": HexColor(st.Accent), "--pele-critical": HexColor(st.Danger),
		"--pele-error": HexColor(st.Danger),
	}
}

func HexColor(c ui.Color) string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }

// mermaidFrame is what an element of Pele's SVG passes on to those
// within it: where it is, and its fill and text's style.
type mermaidFrame struct {
	m                  affine
	fill               string
	size               float32
	bold, italic, mono bool
	anchor             string
	shift              int
	x, y, dy           string
	preserve           bool
	// under is the fill a node's or a subgraph's style gave its
	// shape, which its labels are drawn on, transparent for none;
	// ownFill says the text's style gave it its fill.
	under   ui.Color
	ownFill bool
}

// mermaidSVG makes what MyGo draws of the SVG Pele drew: its colors, which
// are CSS variables, those of st, and its text taken out of it, as the
// chunks of text that the window's text draws.
func mermaidSVG(svg string, st Style) ([]byte, []mermaidText, error) {
	vars := st.pelePalette()
	resolve := func(v string) string { return cssResolve(v, vars) }
	var (
		out   bytes.Buffer
		texts []mermaidText
		stack = []mermaidFrame{{m: identity, size: 16, anchor: "start"}}
		// text is the chunk being read of the text element read, and penX
		// and penY where its next run goes.
		text       *mermaidText
		penX, penY float32
		inText     int
	)
	dec := xml.NewDecoder(strings.NewReader(svg))
	dec.Strict = false
	for {
		tok, err := dec.RawToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			top := stack[len(stack)-1]
			f := top
			f.x, f.y, f.dy = "", "", ""
			styleAttr := ""
			for _, at := range t.Attr {
				if at.Name.Local == "style" && at.Name.Space == "" {
					styleAttr = at.Value
				}
			}
			decls := cssDecls(styleAttr)
			if len(stack) == 1 {
				// The root's custom properties are Pele's short names of
				// its variables, as --_bg for --pele-bg.
				for _, d := range decls {
					if strings.HasPrefix(d[0], "--") {
						vars[d[0]] = d[1]
					}
				}
			}
			prop := func(name string) (string, bool) {
				for _, d := range decls {
					if d[0] == name {
						return resolve(d[1]), true
					}
				}
				for _, at := range t.Attr {
					if at.Name.Local == name && at.Name.Space == "" {
						return resolve(at.Value), true
					}
				}
				return "", false
			}
			if v, ok := prop("fill"); ok {
				f.fill = v
			}
			// A shape filled by its style, as style and classDef fill
			// it, is what the labels of its group are drawn on, and a
			// text filled by it has the color they gave it.
			for _, d := range decls {
				switch {
				case d[0] != "fill":
				case t.Name.Local == "text" || inText > 0:
					f.ownFill = true
				case len(stack) > 1:
					u := cssParseColor(resolve(d[1]), ui.Color{})
					if v, ok := prop("fill-opacity"); ok {
						if n, err := strconv.ParseFloat(v, 32); err == nil {
							u = u.Alpha(float32(n))
						}
					}
					stack[len(stack)-1].under = u
				}
			}
			// A size "smaller" is a subscript's or a superscript's, which
			// their style makes smaller.
			if v, ok := prop("font-size"); ok {
				if n, ok := cssLength(v); ok {
					f.size = n
				}
			}
			if v, ok := prop("font"); ok {
				for _, w := range strings.Fields(v) {
					if n, ok := cssLength(w); ok {
						f.size = n
					}
				}
			}
			if v, ok := prop("font-weight"); ok {
				// A variable Pele leaves unset, as its --_w, is its bold.
				f.bold = v == "" || cssBold(v)
			}
			if v, ok := prop("font-style"); ok {
				f.italic = v == "italic" || v == "oblique"
			}
			if v, ok := prop("font-family"); ok && inText > 0 {
				f.mono = strings.Contains(v, "mono") || v == st.Mono
			}
			if v, ok := prop("text-anchor"); ok {
				f.anchor = v
			}
			if v, ok := prop("baseline-shift"); ok {
				switch v {
				case "sub":
					f.shift = peleSub
				case "super":
					f.shift = peleSup
				}
			}
			if v, ok := prop("transform"); ok {
				f.m = parseTransform(v).then(top.m)
			}
			for _, at := range t.Attr {
				switch at.Name.Local {
				case "x":
					f.x = at.Value
				case "y":
					f.y = at.Value
				case "dy":
					f.dy = at.Value
				case "space":
					f.preserve = at.Value == "preserve"
				}
			}
			stack = append(stack, f)
			if t.Name.Local == "text" || inText > 0 {
				if t.Name.Local == "text" {
					texts = append(texts, mermaidText{m: f.m, anchor: f.anchor})
					text = &texts[len(texts)-1]
					penX, penY = 0, 0
				}
				inText++
				x, xok := cssLength(f.x)
				y, yok := cssLength(f.y)
				dy, _ := cssLength(f.dy)
				if yok {
					penY = y
				}
				penY += dy
				if xok || yok && t.Name.Local != "text" {
					if !xok {
						x = penX
					}
					if len(text.runs) > 0 {
						texts = append(texts, mermaidText{m: text.m, anchor: f.anchor})
						text = &texts[len(texts)-1]
					}
					text.x, text.y, text.anchor = x, penY, f.anchor
					penX = x
				}
				if len(text.runs) == 0 {
					text.y = penY
				}
				continue
			}
			out.WriteByte('<')
			out.WriteString(xmlName(t.Name))
			var keep []string
			for _, d := range decls {
				switch {
				case strings.HasPrefix(d[0], "--"):
				case d[0] == "rx" || d[0] == "ry":
					// MyGo takes a rect's corners from its attributes.
					if n, ok := cssLength(resolve(d[1])); ok {
						fmt.Fprintf(&out, ` %s="%g"`, d[0], n)
					}
				default:
					keep = append(keep, d[0]+":"+resolve(d[1]))
				}
			}
			for _, at := range t.Attr {
				if at.Name.Local == "style" && at.Name.Space == "" {
					if len(keep) > 0 {
						fmt.Fprintf(&out, ` style="%s"`, xmlEscape(strings.Join(keep, ";")))
					}
					continue
				}
				fmt.Fprintf(&out, ` %s="%s"`, xmlName(at.Name), xmlEscape(resolve(at.Value)))
			}
			out.WriteByte('>')
		case xml.EndElement:
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
			if inText > 0 {
				inText--
				continue
			}
			out.WriteString("</" + xmlName(t.Name) + ">")
		case xml.CharData:
			if inText == 0 {
				xmlEscapeTo(&out, string(t))
				continue
			}
			f := stack[len(stack)-1]
			s := string(t)
			if !f.preserve {
				s = strings.Join(strings.Fields(s), " ")
			}
			if s == "" {
				continue
			}
			style := f.shift
			if f.bold {
				style |= pelebold
			}
			if f.italic {
				style |= peleItalic
			}
			if f.mono {
				style |= peleMono
			}
			text.runs = append(text.runs, mermaidRun{text: s, size: f.size, style: style, color: f.color(st)})
		}
	}
	return out.Bytes(), texts, nil
}

// labelContrast is the least contrast a label keeps with the fill a
// style gave its shape, below which it is drawn in another color.
const labelContrast = 3

// color is the color of a frame's text: its fill, or, when the style of
// its shape gave that a fill the fill of the diagram reads poorly on,
// whichever of the theme's text and background reads better on it.
func (f mermaidFrame) color(st Style) ui.Color {
	c := cssParseColor(f.fill, st.FG)
	if f.ownFill || f.under.A == 0 {
		return c
	}
	u := f.under.Over(st.BG)
	if contrast(c.Over(st.BG), u) >= labelContrast {
		return c
	}
	if contrast(st.FG, u) >= contrast(st.BG, u) {
		return st.FG
	}
	return st.BG
}

// contrast is the WCAG contrast ratio of two opaque colors.
func contrast(x, y ui.Color) float64 {
	lum := func(c ui.Color) float64 {
		ch := func(v uint8) float64 {
			f := float64(v) / 255
			if f <= 0.03928 {
				return f / 12.92
			}
			return math.Pow((f+0.055)/1.055, 2.4)
		}
		return 0.2126*ch(c.R) + 0.7152*ch(c.G) + 0.0722*ch(c.B)
	}
	a, b := lum(x)+0.05, lum(y)+0.05
	return max(a, b) / min(a, b)
}

func xmlName(n xml.Name) string {
	if n.Space != "" {
		return n.Space + ":" + n.Local
	}
	return n.Local
}

func xmlEscape(s string) string {
	var b bytes.Buffer
	xmlEscapeTo(&b, s)
	return b.String()
}

func xmlEscapeTo(b *bytes.Buffer, s string) { xml.EscapeText(b, []byte(s)) }

// cssDecls splits a style attribute into its declarations, name and value.
func cssDecls(s string) [][2]string {
	var decls [][2]string
	for _, d := range strings.Split(s, ";") {
		name, value, ok := strings.Cut(d, ":")
		if name = strings.TrimSpace(name); ok && name != "" {
			decls = append(decls, [2]string{name, strings.TrimSpace(value)})
		}
	}
	return decls
}

// cssResolve replaces the var() of a CSS value with the values of vars,
// or their fallbacks, and mixes the colors of its color-mix().
func cssResolve(v string, vars map[string]string) string {
	return cssResolveDepth(v, vars, 0)
}

func cssResolveDepth(v string, vars map[string]string, depth int) string {
	if depth > 8 || !strings.Contains(v, "(") {
		return v
	}
	for _, fn := range []string{"var(", "color-mix("} {
		for {
			i := strings.Index(v, fn)
			if i < 0 {
				break
			}
			end := cssClose(v, i+len(fn))
			if end < 0 {
				return v
			}
			args := cssArgs(v[i+len(fn) : end])
			var with string
			if fn == "var(" {
				name := strings.TrimSpace(args[0])
				if val, ok := vars[name]; ok {
					with = cssResolveDepth(val, vars, depth+1)
				} else if len(args) > 1 {
					with = cssResolveDepth(strings.Join(args[1:], ","), vars, depth+1)
				}
			} else {
				with = cssMix(args, vars, depth)
			}
			v = v[:i] + strings.TrimSpace(with) + v[end+1:]
		}
	}
	return v
}

// cssClose returns where the parenthesis open before at closes, or -1.
func cssClose(s string, at int) int {
	depth := 1
	for i := at; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			if depth--; depth == 0 {
				return i
			}
		}
	}
	return -1
}

// cssArgs splits the arguments of a CSS function at its commas, not those
// of the functions within it.
func cssArgs(s string) []string {
	var args []string
	depth, from := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				args = append(args, s[from:i])
				from = i + 1
			}
		}
	}
	return append(args, s[from:])
}

// cssMix mixes the two colors of color-mix(in srgb, a p%, b q%).
func cssMix(args []string, vars map[string]string, depth int) string {
	if len(args) != 3 {
		return ""
	}
	color := func(s string) (ui.Color, float32, bool) {
		s = cssResolveDepth(strings.TrimSpace(s), vars, depth+1)
		pct := float32(-1)
		if i := strings.LastIndexByte(s, ' '); i > 0 && strings.HasSuffix(s, "%") {
			if n, err := strconv.ParseFloat(s[i+1:len(s)-1], 32); err == nil {
				pct, s = float32(n)/100, strings.TrimSpace(s[:i])
			}
		}
		c, ok := cssHex(s)
		return c, pct, ok
	}
	a, pa, okA := color(args[1])
	b, pb, okB := color(args[2])
	if !okA || !okB {
		return ""
	}
	switch {
	case pa < 0 && pb < 0:
		pa = 0.5
	case pa < 0:
		pa = 1 - pb
	}
	return HexColor(a.Mix(b, 1-pa))
}

// cssHex parses a color as #rgb, #rgba, #rrggbb or #rrggbbaa.
func cssHex(s string) (ui.Color, bool) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "#") {
		return ui.Color{}, false
	}
	h := s[1:]
	if len(h) == 3 || len(h) == 4 {
		var b strings.Builder
		for _, ch := range h {
			b.WriteRune(ch)
			b.WriteRune(ch)
		}
		h = b.String()
	}
	if len(h) != 6 && len(h) != 8 {
		return ui.Color{}, false
	}
	n, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return ui.Color{}, false
	}
	if len(h) == 6 {
		n = n<<8 | 0xff
	}
	return ui.Color{R: uint8(n >> 24), G: uint8(n >> 16), B: uint8(n >> 8), A: uint8(n)}, true
}

// cssParseColor is the color of a fill, def when it is none it reads.
func cssParseColor(s string, def ui.Color) ui.Color {
	if c, ok := cssHex(s); ok {
		return c
	}
	if c, ok := cssNamed[strings.ToLower(strings.TrimSpace(s))]; ok {
		return c
	}
	if strings.HasPrefix(s, "rgb") {
		if open, end := strings.IndexByte(s, '('), strings.IndexByte(s, ')'); open > 0 && end > open {
			var n []float64
			for _, f := range strings.FieldsFunc(s[open+1:end], func(r rune) bool { return r == ',' || r == ' ' || r == '/' }) {
				v, err := strconv.ParseFloat(strings.TrimSuffix(f, "%"), 64)
				if err != nil {
					return def
				}
				n = append(n, v)
			}
			if len(n) >= 3 {
				alpha := float32(1)
				if len(n) > 3 {
					alpha = float32(n[3])
				}
				return ui.RGBA(uint8(n[0]), uint8(n[1]), uint8(n[2]), alpha)
			}
		}
	}
	return def
}

// cssNamed are the color names a diagram's style is likely to give.
var cssNamed = map[string]ui.Color{
	"black": ui.Hex("#000000"), "white": ui.Hex("#ffffff"), "red": ui.Hex("#ff0000"),
	"green": ui.Hex("#008000"), "blue": ui.Hex("#0000ff"), "yellow": ui.Hex("#ffff00"),
	"orange": ui.Hex("#ffa500"), "purple": ui.Hex("#800080"), "gray": ui.Hex("#808080"),
	"grey": ui.Hex("#808080"), "pink": ui.Hex("#ffc0cb"), "brown": ui.Hex("#a52a2a"),
}

// cssLength parses a length in pixels, as "12", "12px" or "12.5".
func cssLength(s string) (float32, bool) {
	s = strings.TrimSuffix(strings.TrimSpace(s), "px")
	if s == "" {
		return 0, false
	}
	n, err := strconv.ParseFloat(s, 32)
	return float32(n), err == nil
}

// cssBold reports whether a font-weight is bold: bold, bolder, or 600
// and up.
func cssBold(v string) bool {
	switch v {
	case "bold", "bolder":
		return true
	}
	n, err := strconv.Atoi(v)
	return err == nil && n >= 600
}
