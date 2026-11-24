package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

type httpBlockCache struct {
	version    uint64
	generation uint64
	loading    bool
	blocks     []openidea.HTTPRequestBlock
}

func (m *Model) activeHTTPRequestEditor() bool {
	t := m.tab()
	return t != nil && t.doc != nil && t.state != nil &&
		isHTTPRequestFile(t.doc.Path())
}

// NewHTTPRequest starts OpenIDEA's template name prompt for an .http file.
func (m *Model) NewHTTPRequest(directory string) {
	directory = strings.TrimSpace(directory)
	if directory != "" {
		for index := range projectNewTemplates {
			if projectNewTemplates[index].id != "http-request" {
				continue
			}
			m.OpenProjectNew(directory)
			m.SelectProjectNewTemplate(index)
			return
		}
		return
	}
	directory = filepath.Join(m.goideaDataDir(), "scratches")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		m.notifyError("http.new", "Cannot create the HTTP request file", err.Error())
		return
	}
	path := ""
	for index := 0; ; index++ {
		name := "requests.http"
		if index > 0 {
			name = fmt.Sprintf("requests_%d.http", index)
		}
		candidate := filepath.Join(directory, name)
		file, err := os.OpenFile(candidate, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			m.notifyError("http.new", "Cannot create the HTTP request file", err.Error())
			return
		}
		_, writeErr := file.WriteString(httpNewRequestTemplate)
		closeErr := file.Close()
		if writeErr != nil {
			_ = os.Remove(candidate)
			m.notifyError("http.new", "Cannot create the HTTP request file", writeErr.Error())
			return
		}
		if closeErr != nil {
			_ = os.Remove(candidate)
			m.notifyError("http.new", "Cannot create the HTTP request file", closeErr.Error())
			return
		}
		path = candidate
		break
	}
	m.openFile(path)
}

const httpNewRequestTemplate = `### Example request
# Variables come from http-client.env.json / http-client.private.env.json
# next to this file or in the project root: {{host}}
GET https://example.com/
Accept: application/json

###
`

// httpRequestAtLine is OpenIDEA's requestAtCaret: a line inside a request
// selects that request, a line between requests selects the nearest one above,
// and a line before the first request selects the first.
func httpRequestAtLine(blocks []openidea.HTTPRequestBlock, line int) (openidea.HTTPRequestBlock, bool) {
	if len(blocks) == 0 {
		return openidea.HTTPRequestBlock{}, false
	}
	var above *openidea.HTTPRequestBlock
	for i := range blocks {
		block := &blocks[i]
		end := block.EndLine
		if block.Line > end {
			end = block.Line
		}
		if int(block.StartLine) <= line && line <= int(end) {
			return *block, true
		}
		if int(block.Line) <= line {
			above = block
		}
	}
	if above != nil {
		return *above, true
	}
	return blocks[0], true
}

func httpRequestLabel(block openidea.HTTPRequestBlock) string {
	if block.Name != nil && strings.TrimSpace(*block.Name) != "" {
		return strings.TrimSpace(*block.Name)
	}
	return strings.TrimSpace(block.Method + " " + block.URL)
}

func (m *Model) withHTTPRequestAtCaret(
	t *Tab,
	run func(block openidea.HTTPRequestBlock, text string),
) {
	if t == nil || t.doc == nil || t.state == nil {
		return
	}
	text := t.doc.Text()
	line := t.doc.UTF16Pos(t.state.Selection().Head).Line
	go func() {
		blocks, err := m.parseHTTPRequests(text)
		if err != nil {
			m.post(func() {
				m.notifyError("http-run", "Cannot read HTTP requests", err.Error())
				m.invalidate()
			})
			return
		}
		block, ok := httpRequestAtLine(blocks, int(line))
		m.post(func() {
			if !ok {
				m.notifyInfo("http-run", "No HTTP request at the caret", "")
				m.invalidate()
				return
			}
			if !m.hasTab(t) {
				return
			}
			run(block, t.doc.Text())
		})
	}()
}

// ensureHTTPBlocks parses a request file at most once for each buffer version.
// The gutter cannot call RPC while laying out, so a previous frame's parse is
// painted now and a changed buffer schedules the next parse.
func (m *Model) ensureHTTPBlocks(t *Tab) []openidea.HTTPRequestBlock {
	if t == nil || t.doc == nil || t.state == nil || t.state.Buffer == nil ||
		!isHTTPRequestFile(t.doc.Path()) || m.invokeHook == nil && m.mgr == nil {
		return nil
	}
	m.ensureHTTPClient()
	path := cleanHTTPPath(t.doc.Path())
	version := t.state.Buffer.Version()
	entry, ok := m.http.blocks[path]
	if ok && entry.version == version {
		return entry.blocks
	}
	if ok && entry.loading && entry.version != version {
		// The old parse may finish, but its generation is no longer current.
	}
	m.http.blockGeneration++
	generation := m.http.blockGeneration
	text := t.doc.Text()
	m.http.blocks[path] = httpBlockCache{
		version: version, generation: generation, loading: true,
	}
	root := m.Root()
	go func() {
		blocks, err := m.parseHTTPRequests(text)
		if err != nil {
			blocks = nil
		}
		m.post(func() {
			if m.http.root != root {
				return
			}
			current, exists := m.http.blocks[path]
			if !exists || current.generation != generation {
				return
			}
			current.loading = false
			current.blocks = blocks
			m.http.blocks[path] = current
			m.invalidate()
		})
	}()
	return nil
}

func (m *Model) httpEditorToolbar(c *ui.Context, p *ui.Theme, t *Tab) {
	if t == nil || t.doc == nil || !isHTTPRequestFile(t.doc.Path()) {
		return
	}
	m.ensureHTTPClient()
	oi := oiPaletteOf(p)
	path := cleanHTTPPath(t.doc.Path())
	m.loadHTTPEnvironments(path)
	selected := m.http.envByFile[path]
	label := "No Environment"
	if selected != "" {
		label = selected
	}
	ui.Row(c.Key("http-editor:toolbar:"+t.doc.Path())).Height(32).Shrink(0).
		Padding(0, 8).Gap(4).AlignItems(ui.Center).Background(p.Background).
		BorderWidth(0, 0, 1, 0).BorderColor(oi.border).Children(func() {
		m.httpToolbarButton(c, p, "actions/runAll", "Run All Requests in File",
			false, m.RunAllHTTPRequests)
		m.httpToolbarButton(c, p, "run/stop", "Stop",
			m.http.running == nil, func() { m.cancelHTTPRequest(true) })
		ui.Text(c, "Run with:").FontSize(12).TextColor(oi.textSecondary)
		ui.MenuButton(c, label, func(menu *ui.Menu) {
			if menu.Item("No Environment").Chosen() {
				m.setHTTPEnvironment(path, "")
			}
			for _, environment := range m.http.environments[path] {
				environment := environment
				item := menu.Item(environment)
				item.Checked(environment == selected)
				if item.Chosen() {
					m.setHTTPEnvironment(path, environment)
				}
			}
		})
		m.httpToolbarButton(c, p, "general/refresh", "Reload Environments",
			false, func() {
				delete(m.http.environments, path)
				m.loadHTTPEnvironments(path)
				m.invalidate()
			})
		m.httpToolbarMenuButton(c, p, "general/add", "Add Environment File",
			func(menu *ui.Menu) {
				if menu.Item("Add Environment to Public File…").Chosen() {
					m.OpenHTTPEnvironmentFile(false)
				}
				if menu.Item("Add Environment to Private File…").Chosen() {
					m.OpenHTTPEnvironmentFile(true)
				}
			})
		m.httpToolbarButton(c, p, "general/copy", "Convert to cURL and Copy",
			false, m.CopyHTTPRequestAsCurl)
	})
}

func (m *Model) httpToolbarButton(
	c *ui.Context,
	p *ui.Theme,
	icon, title string,
	disabled bool,
	onClick func(),
) {
	oi := oiPaletteOf(p)
	button := ui.Row(c).Size(24, 24).AlignItems(ui.Center).Justify(ui.Center).
		Radius(4).Label(title).Tooltip(title).Disabled(disabled).
		Cursor(ui.CursorPointer)
	if button.Hovered() && !disabled {
		button.Background(oi.hoverTint)
	}
	button.OnClick(onClick)
	button.Children(func() {
		oiIconEl(c, icon, 16, oi, oi.textMuted)
	})
}

func (m *Model) httpToolbarMenuButton(
	c *ui.Context,
	p *ui.Theme,
	icon, title string,
	build func(menu *ui.Menu),
) {
	oi := oiPaletteOf(p)
	button := ui.Row(c).Size(24, 24).AlignItems(ui.Center).Justify(ui.Center).
		Radius(4).Label(title).Tooltip(title).Cursor(ui.CursorPointer)
	if button.Hovered() {
		button.Background(oi.hoverTint)
	}
	button.Menu(build)
	button.Children(func() {
		oiIconEl(c, icon, 16, oi, oi.textMuted)
	})
}

func (m *Model) httpGutter(
	c *ui.Context,
	p *ui.Theme,
	t *Tab,
	editorRow, view ui.Element,
) {
	if t == nil || t.doc == nil || t.state == nil || t.state.Buffer == nil ||
		!isHTTPRequestFile(t.doc.Path()) {
		return
	}
	blocks := m.ensureHTTPBlocks(t)
	if len(blocks) == 0 {
		return
	}
	viewBounds, rowBounds := view.Bounds(), editorRow.Bounds()
	if viewBounds.W <= 0 || viewBounds.H <= 0 {
		return
	}
	first, last := t.state.VisibleLines()
	first = max(0, first)
	last = min(last, t.state.Buffer.LineCount()-1)
	oi := oiPaletteOf(p)
	path, text := cleanHTTPPath(t.doc.Path()), t.doc.Text()
	for _, block := range blocks {
		line := int(block.Line)
		if line < first || line > last {
			continue
		}
		top := float32(t.state.LineTop(line) - t.state.ScrollY())
		height := float32(t.state.LineTop(line+1) - t.state.LineTop(line))
		if height <= 0 {
			height = t.state.Font.Size * t.state.LineHeight
		}
		if top+height <= 0 || top >= viewBounds.H {
			continue
		}
		title := "Run '" + httpRequestLabel(block) + "'"
		localX := viewBounds.X - rowBounds.X + 1
		localY := viewBounds.Y - rowBounds.Y + top + max(0, (height-14)/2)
		block := block
		button := ui.Row(c.Key(fmt.Sprintf("http-gutter:%s:%d", path, block.Index))).
			Absolute().Left(localX).Top(localY).Size(14, 14).
			AlignItems(ui.Center).Justify(ui.Center).Radius(3).
			Label(title).Tooltip(title).Cursor(ui.CursorPointer)
		if button.Hovered() {
			button.Background(oi.hoverTint)
		}
		button.OnClick(func() { m.RunHTTPRequest(path, text, block.Index) })
		button.Children(func() {
			oiIconEl(c, "gutter/run", 14, oi, oi.textMuted)
		})
	}
}

func isCurlCommand(command string) bool {
	command = strings.TrimSpace(command)
	return strings.HasPrefix(strings.ToLower(command), "curl ")
}

// pasteHTTPRequest mirrors OpenIDEA's editor paste handler: a cURL command is
// converted first, and the original text is inserted when conversion cannot
// understand it.
func (m *Model) pasteHTTPRequest(t *Tab) bool {
	if t == nil || t.doc == nil || t.state == nil || t.state.ReadOnly ||
		!isHTTPRequestFile(t.doc.Path()) {
		return false
	}
	command := strings.TrimSpace(mygo.Clipboard.ReadText())
	if !isCurlCommand(command) {
		return false
	}
	m.insertHTTPFromCurl(t, command)
	return true
}

func (m *Model) insertHTTPFromCurl(t *Tab, command string) {
	doc := t.doc
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "http_from_curl", map[string]any{"command": command})
		var request *string
		if err == nil {
			err = json.Unmarshal(raw, &request)
		}
		insert := command
		if err == nil && request != nil {
			insert = *request
		}
		m.post(func() {
			if !m.hasTab(t) || t.doc != doc || t.state.ReadOnly {
				return
			}
			t.state.Insert(insert)
			if err != nil || request == nil {
				m.status = "The pasted cURL command was kept because it could not be converted"
			} else {
				m.status = "The cURL command was converted to an HTTP request"
			}
			m.invalidate()
		})
	}()
}
