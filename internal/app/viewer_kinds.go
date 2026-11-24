package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
	"mayoi/internal/vfs"
)

const (
	tabKindImageViewer = "imageViewer"
	tabKindHexViewer   = "hexViewer"

	maxImageViewerBytes = 32 << 20
	maxHexViewerBytes   = 16 << 20
	maxTextViewerBytes  = 8 << 20
)

type fileViewerState struct {
	source     ui.ImageSource
	width      int
	height     int
	size       int64
	zoom       float32
	chessboard bool
	hex        []byte
	truncated  bool
	hexList    ui.ListState
	loadErr    string
}

func imageViewerPath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".bmp":
		return true
	default:
		return false
	}
}

func imageViewerSource(path string, data []byte) (ui.ImageSource, int, int, error) {
	if strings.EqualFold(filepath.Ext(path), ".svg") {
		svg, err := ui.ParseSVG(data)
		if err != nil {
			return nil, 0, 0, err
		}
		w, h := svg.Size()
		return svg, int(w), int(h), nil
	}
	bitmap, err := ui.DecodeBitmap(data)
	if err != nil {
		return nil, 0, 0, err
	}
	w, h := bitmap.Size()
	return bitmap, w, h, nil
}

func isBinaryViewerData(data []byte) bool {
	return looksLikeBinaryText(data)
}

func wantsHexViewer(path string) (bool, int64) {
	info, err := vfs.Stat(path)
	if err != nil || info.IsDir() {
		return false, 0
	}
	if info.Size() > maxTextViewerBytes {
		return true, info.Size()
	}
	data, err := vfs.ReadFileLimit(path, 64<<10)
	if err != nil {
		return false, info.Size()
	}
	return isBinaryViewerData(data), info.Size()
}

func (m *Model) openImageViewer(path string, spec openSpec) {
	info, data, err := vfs.ReadFileInfo(path, maxImageViewerBytes)
	if err == nil && info.Size() > maxImageViewerBytes {
		err = fmt.Errorf("the image is larger than 32 MB")
	}
	viewer := &fileViewerState{zoom: 0}
	if err == nil {
		viewer.size = info.Size()
		viewer.source, viewer.width, viewer.height, err = imageViewerSource(path, data)
	}
	if err != nil {
		viewer.loadErr = err.Error()
	}
	m.openFileViewerTab(tabKindImageViewer, path, viewer, spec)
}

func (m *Model) openHexViewer(path string, size int64, spec openSpec) {
	data, err := vfs.ReadFileLimit(path, maxHexViewerBytes)
	viewer := &fileViewerState{size: size, hex: data}
	if err != nil {
		viewer.loadErr = err.Error()
	}
	viewer.truncated = size > int64(len(data))
	m.openFileViewerTab(tabKindHexViewer, path, viewer, spec)
}

func (m *Model) openFileViewerTab(kind, path string, viewer *fileViewerState, spec openSpec) {
	doc := docbridge.Virtual(path, "")
	t := &Tab{
		doc:      doc,
		state:    m.newReadOnlyEditorState(doc),
		kind:     kind,
		pagePath: path,
		viewer:   viewer,
	}
	t.state.Font = ui.Font{Family: oiFontUI, Size: m.prefs.fontSize()}
	t.synced = doc.Buffer().Version()
	t.seen = t.synced
	m.noteTabUse(t)
	m.insertTab(t)
	m.active = m.tabIndex(t)
	m.showingWelcome = false
	m.revealEditor()
	m.applyActive()
	m.saveSession()
	m.finishOpen(spec, t)
	m.pumpOpenQueue()
	m.invalidate()
}

func (m *Model) newReadOnlyEditorState(doc *docbridge.Document) *editor.State {
	return &editor.State{
		Buffer: doc.Buffer(), LineHeight: 1.65,
		Bindings: m.editorBindings(), ReadOnly: true,
	}
}

func activeImageViewer(m *Model) *Tab {
	if m == nil {
		return nil
	}
	t := m.tab()
	if t == nil || t.kind != tabKindImageViewer || t.viewer == nil {
		return nil
	}
	return t
}

func (m *Model) imageViewerForArg(arg any) *Tab {
	if m == nil {
		return nil
	}
	path := pluginPathArg(arg)
	if path != "" {
		if index := m.tabForPath(realPath(path)); index >= 0 {
			t := m.tabs[index]
			if t != nil && t.kind == tabKindImageViewer {
				return t
			}
		}
	}
	return activeImageViewer(m)
}

func (m *Model) imageViewerPage(c *ui.Context, p *ui.Theme, oi oiColors, t *Tab) {
	v := t.viewer
	if v == nil {
		return
	}
	ui.Column(c.Key("image:" + t.doc.Path())).Grow(1).MinHeight(0).MinWidth(0).
		Background(p.Background).Children(func() {
		ui.Row(c).Height(38).Shrink(0).Padding(0, 10).Gap(8).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
			ui.Text(c, filepath.Base(t.doc.Path())).FontSize(12).TextColor(p.Text).SingleLine()
			ui.Text(c, fmt.Sprintf("%d × %d", v.width, v.height)).
				FontSize(12).TextColor(p.TextMuted).SingleLine()
			ui.Spacer(c)
			m.viewerButton(c, oi, "−", func() { m.zoomImage(t, -1) })
			ui.Text(c, viewerZoomLabel(v)).Width(54).FontSize(12).
				TextColor(p.TextMuted).SingleLine()
			m.viewerButton(c, oi, "+", func() { m.zoomImage(t, 1) })
			m.viewerButton(c, oi, "Actual Size", func() { m.actualSizeImage(t) })
			m.viewerButton(c, oi, "Fit", func() { m.fitImage(t) })
			m.viewerButton(c, oi, "Chessboard", func() {
				v.chessboard = !v.chessboard
				m.invalidate()
			})
		})
		canvas := ui.Scroll(c.Key("image:scroll:" + t.doc.Path())).Grow(1).
			MinHeight(0).MinWidth(0)
		if v.chessboard {
			canvas.Background(p.Surface).Draw(func(painter *ui.Painter, r ui.Rect) {
				paintImageViewerChessboard(painter, r, p.Surface, p.SurfaceHover)
			})
		} else {
			canvas.Background(p.Background)
		}
		canvas.Children(func() {
			ui.Column(c).MinWidth(0).MinHeight(0).Padding(24).
				AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				switch {
				case v.loadErr != "":
					ui.Text(c, "Unable to open this image: "+v.loadErr).
						FontSize(13).TextColor(oi.danger).Wrap()
				case v.source != nil && v.zoom <= 0:
					ui.Image(c, v.source).Fit(ui.Contain).Label(filepath.Base(t.doc.Path()))
				case v.source != nil:
					w := float32(v.width) * v.zoom
					h := float32(v.height) * v.zoom
					ui.Image(c, v.source).Size(max(1, w), max(1, h)).
						Fit(ui.FillBox).Label(filepath.Base(t.doc.Path()))
				}
			})
		})
	})
}

func paintImageViewerChessboard(p *ui.Painter, r ui.Rect, light, dark ui.Color) {
	p.Fill(r, light, 0)
	const square = float32(12)
	for y, row := r.Y, 0; y < r.Y+r.H; y, row = y+square, row+1 {
		for x, col := r.X, 0; x < r.X+r.W; x, col = x+square, col+1 {
			if (row+col)%2 == 0 {
				p.Fill(ui.Rect{
					X: x,
					Y: y,
					W: min(square, r.X+r.W-x),
					H: min(square, r.Y+r.H-y),
				}, dark, 0)
			}
		}
	}
}

func (m *Model) viewerButton(c *ui.Context, oi oiColors, label string, run func()) {
	ui.Button(c, label).OnClick(run)
}

func viewerZoomLabel(v *fileViewerState) string {
	if v == nil || v.zoom <= 0 {
		return "Fit"
	}
	return fmt.Sprintf("%.0f%%", v.zoom*100)
}

func (m *Model) zoomImage(t *Tab, direction int) {
	if t == nil || t.viewer == nil {
		return
	}
	zoom := t.viewer.zoom
	if zoom <= 0 {
		zoom = 1
	}
	if direction > 0 {
		zoom *= 1.25
	} else {
		zoom /= 1.25
	}
	t.viewer.zoom = min(8, max(0.05, zoom))
	m.status = "Image zoom " + viewerZoomLabel(t.viewer)
	m.invalidate()
}

func (m *Model) actualSizeImage(t *Tab) {
	if t == nil || t.viewer == nil {
		return
	}
	t.viewer.zoom = 1
	m.status = "Image zoom 100%"
	m.invalidate()
}

func (m *Model) fitImage(t *Tab) {
	if t == nil || t.viewer == nil {
		return
	}
	t.viewer.zoom = 0
	m.status = "Image fitted to the window"
	m.invalidate()
}

func hexViewerRow(data []byte, row int) (offset, hex, ascii string) {
	start := row * 16
	if start < 0 || start >= len(data) {
		return "", "", ""
	}
	end := min(start+16, len(data))
	var hexText, asciiText strings.Builder
	for i := 0; i < 16; i++ {
		if start+i < end {
			b := data[start+i]
			fmt.Fprintf(&hexText, "%02X ", b)
			if b >= 0x20 && b <= 0x7e {
				asciiText.WriteByte(b)
			} else {
				asciiText.WriteByte('.')
			}
		} else {
			hexText.WriteString("   ")
		}
		if i == 7 {
			hexText.WriteByte(' ')
		}
	}
	return fmt.Sprintf("%08X", start), strings.TrimRight(hexText.String(), " "), asciiText.String()
}

func (m *Model) hexViewerPage(c *ui.Context, p *ui.Theme, oi oiColors, t *Tab) {
	v := t.viewer
	if v == nil {
		return
	}
	rows := (len(v.hex) + 15) / 16
	ui.Column(c.Key("hex:" + t.doc.Path())).Grow(1).MinHeight(0).MinWidth(0).
		Background(p.Background).Children(func() {
		ui.Row(c).Height(36).Shrink(0).Padding(0, 10).Gap(12).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
			ui.Text(c, filepath.Base(t.doc.Path())).FontSize(12).TextColor(p.Text).SingleLine()
			ui.Text(c, viewerSizeLabel(v.size)).FontSize(12).
				TextColor(p.TextMuted).SingleLine()
			ui.Text(c, "Read-only hex").FontSize(12).TextColor(p.TextMuted).SingleLine()
			if v.truncated {
				ui.Text(c, "Showing the first 16 MB").FontSize(12).
					TextColor(oi.warning).SingleLine()
			}
		})
		switch {
		case v.loadErr != "":
			ui.Text(c, "Unable to read this file: "+v.loadErr).Padding(16).
				FontSize(13).TextColor(oi.danger).Wrap()
		case rows == 0:
			ui.Text(c, "This file is empty").Padding(16).FontSize(13).
				TextColor(p.TextMuted).SingleLine()
		default:
			ui.List(c.Key("hex:rows:"+t.doc.Path()), &v.hexList, rows, func(row int) {
				offset, hexText, ascii := hexViewerRow(v.hex, row)
				ui.Row(c).Height(22).Padding(0, 12).Gap(16).AlignItems(ui.Center).
					Children(func() {
						ui.Text(c, offset).Width(72).Font(oiFontMono).FontSize(12).
							TextColor(p.TextMuted).SingleLine()
						ui.Text(c, hexText).Width(392).Font(oiFontMono).FontSize(12).
							TextColor(p.Text).SingleLine()
						ui.Text(c, ascii).Font(oiFontMono).FontSize(12).
							TextColor(p.TextMuted).SingleLine()
					})
			})
		}
	})
}

func viewerSizeLabel(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	value := float64(size)
	for _, suffix := range []string{"KB", "MB", "GB", "TB"} {
		value /= unit
		if value < unit || suffix == "TB" {
			if value >= 100 {
				return fmt.Sprintf("%.0f %s", value, suffix)
			}
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%d B", size)
}
