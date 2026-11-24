package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

const springGeneratedRequestsFile = "generated-requests.http"

type springRequestTarget struct {
	endpoint springEndpoint
	path     string
}

type springGutterMark struct {
	line        int
	key         string
	title       string
	controller  springController
	endpoint    springEndpoint
	hasEndpoint bool
}

func springDocumentPath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".java", ".kt":
		return true
	default:
		return false
	}
}

func (m *Model) springControllersForTab(t *Tab) []springController {
	if t == nil || t.doc == nil || t.state == nil || t.state.Buffer == nil ||
		!springDocumentPath(t.doc.Path()) {
		return nil
	}
	version := t.state.Buffer.Version()
	if t.springParsed && t.springVersion == version {
		return t.springControllers
	}
	t.springVersion = version
	t.springParsed = true
	t.springControllers = springControllersInText(t.state.Buffer.String())
	return t.springControllers
}

func (m *Model) activeSpringEditor() bool {
	t := m.tab()
	return t != nil && t.doc != nil && t.state != nil && springDocumentPath(t.doc.Path())
}

func (m *Model) springAtCaret(t *Tab) (*springController, *springEndpoint) {
	if t == nil || t.state == nil {
		return nil, nil
	}
	return springAt(m.springControllersForTab(t), t.state.Selection().Head)
}

func (m *Model) GenerateSpringRequest() {
	m.generateSpringAtCaret(false)
}

func (m *Model) GenerateSpringControllerRequests() {
	m.generateSpringAtCaret(true)
}

func (m *Model) generateSpringAtCaret(controllerScope bool) {
	t := m.tab()
	if t == nil || t.doc == nil || !m.activeSpringEditor() {
		return
	}
	controller, endpoint := m.springAtCaret(t)
	if controllerScope {
		if controller == nil || len(controller.endpoints) == 0 {
			m.notifyInfo("http-generate", "No Spring controller at the caret", "")
			m.invalidate()
			return
		}
		targets := make([]springRequestTarget, 0, len(controller.endpoints))
		for _, item := range controller.endpoints {
			targets = append(targets, springRequestTarget{endpoint: item})
		}
		m.generateSpringRequests(t, targets)
		return
	}
	if endpoint == nil {
		m.notifyInfo("http-generate", "No Spring request mapping at the caret", "")
		m.invalidate()
		return
	}
	m.generateSpringRequests(t, []springRequestTarget{{endpoint: *endpoint}})
}

func (m *Model) generateSpringPath(t *Tab, endpoint springEndpoint, path string) {
	if t == nil {
		return
	}
	m.generateSpringRequests(t, []springRequestTarget{{endpoint: endpoint, path: path}})
}

func (m *Model) generateSpringController(t *Tab, controller springController) {
	if t == nil || len(controller.endpoints) == 0 {
		return
	}
	targets := make([]springRequestTarget, 0, len(controller.endpoints))
	for _, endpoint := range controller.endpoints {
		targets = append(targets, springRequestTarget{endpoint: endpoint})
	}
	m.generateSpringRequests(t, targets)
}

// generateSpringRequests mimics OpenIDEA's request generator: it reads the
// current unsaved source, resolves server.* and body types, appends the request
// blocks to the generated-requests scratch, and puts the caret on the first one.
func (m *Model) generateSpringRequests(sourceTab *Tab, targets []springRequestTarget) {
	if sourceTab == nil || sourceTab.doc == nil || sourceTab.state == nil || len(targets) == 0 {
		return
	}
	sourcePath := sourceTab.doc.Path()
	sourceText := sourceTab.state.Buffer.String()
	projectRoot := m.Root()
	go func() {
		host := springFindHost(sourcePath, projectRoot)
		endpoints := make([]springEndpoint, 0, len(targets))
		for _, target := range targets {
			endpoints = append(endpoints, target.endpoint)
		}
		lookup := springBodyLookup(sourcePath, projectRoot, sourceText, endpoints)
		blocks := make([]string, 0, len(targets))
		for _, target := range targets {
			var body any
			if target.endpoint.bodyType != nil {
				body = springSkeleton(*target.endpoint.bodyType, lookup, 0)
			}
			blocks = append(blocks, springRenderRequest(target.endpoint, host, target.path, body))
		}
		m.post(func() { m.appendGeneratedHTTPRequests(blocks) })
	}()
}

func (m *Model) appendGeneratedHTTPRequests(blocks []string) {
	if len(blocks) == 0 {
		return
	}
	directory := filepath.Join(m.goideaDataDir(), "scratches")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		m.notifyError("http-generate", "Cannot generate the HTTP request", err.Error())
		m.invalidate()
		return
	}
	path := filepath.Join(directory, springGeneratedRequestsFile)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err == nil {
		err = file.Close()
	} else if os.IsExist(err) {
		err = nil
	}
	if err != nil {
		m.notifyError("http-generate", "Cannot generate the HTTP request", err.Error())
		m.invalidate()
		return
	}
	m.openWith(path, openSpec{
		at: -1,
		done: func(tab *Tab) {
			m.appendSpringRequests(tab, blocks)
		},
		failed: func(_ string, openErr error) {
			m.notifyError("http-generate", "Cannot open the HTTP request file", openErr.Error())
			m.invalidate()
		},
	})
}

func (m *Model) appendSpringRequests(t *Tab, blocks []string) {
	if t == nil || t.state == nil || t.state.Buffer == nil || t.state.ReadOnly {
		return
	}
	existing := t.state.Buffer.String()
	insert, from, caret := springAppendRequests(existing, blocks)
	if insert == "" {
		return
	}
	t.state.SetSelections(editor.Selection{Anchor: from, Head: from})
	t.state.Insert(insert)
	t.state.SetSelections(editor.Selection{Anchor: caret, Head: caret})
	m.status = fmt.Sprintf("Generated %d HTTP request", len(blocks))
	if len(blocks) != 1 {
		m.status += "s"
	}
	m.invalidate()
}

func (m *Model) springEndpointMenu(menu *ui.Menu, t *Tab, endpoint springEndpoint) {
	if len(endpoint.paths) <= 1 {
		if menu.Item("Generate Request in HTTP Client").Chosen() {
			m.generateSpringPath(t, endpoint, "")
		}
		return
	}
	menu.Submenu("Generate Request in HTTP Client", func(submenu *ui.Menu) {
		for _, path := range endpoint.paths {
			path := path
			if submenu.Item(endpoint.httpMethod + " " + path).Chosen() {
				m.generateSpringPath(t, endpoint, path)
			}
		}
	})
}

func (m *Model) springMenuAvailable(t *Tab) bool {
	if t == nil || t.doc == nil || !springDocumentPath(t.doc.Path()) {
		return false
	}
	controller, endpoint := m.springAtCaret(t)
	return controller != nil && (endpoint != nil || len(controller.endpoints) > 0)
}

// springEditorMenu adds the Spring request actions at the caret.
func (m *Model) springEditorMenu(menu *ui.Menu, t *Tab) {
	if t == nil || t.doc == nil || !springDocumentPath(t.doc.Path()) {
		return
	}
	controller, endpoint := m.springAtCaret(t)
	if controller == nil {
		return
	}
	if endpoint != nil {
		m.springEndpointMenu(menu, t, *endpoint)
	}
	if len(controller.endpoints) > 0 &&
		menu.Item("Generate Requests for Controller").Chosen() {
		m.generateSpringController(t, *controller)
	}
}

func (m *Model) springGutterMenu(menu *ui.Menu, t *Tab, mark springGutterMark) {
	if mark.hasEndpoint {
		m.springEndpointMenu(menu, t, mark.endpoint)
		return
	}
	if menu.Item("Generate Requests for Controller").Chosen() {
		m.generateSpringController(t, mark.controller)
	}
}

func springLineAt(text string, offset int) int {
	offset = max(0, min(offset, len(text)))
	return strings.Count(text[:offset], "\n")
}

func springMarksForText(controllers []springController, text string) []springGutterMark {
	var marks []springGutterMark
	usedLines := map[int]bool{}
	add := func(mark springGutterMark) {
		if usedLines[mark.line] {
			return
		}
		usedLines[mark.line] = true
		marks = append(marks, mark)
	}
	for _, controller := range controllers {
		if len(controller.endpoints) > 0 {
			add(springGutterMark{
				line:       springLineAt(text, controller.offset),
				key:        "class:" + controller.name,
				title:      "Generate Requests for Controller",
				controller: controller,
			})
		}
		for _, endpoint := range controller.endpoints {
			title := endpoint.httpMethod + " /"
			if len(endpoint.paths) > 0 {
				title = endpoint.httpMethod + " " + endpoint.paths[0]
			}
			add(springGutterMark{
				line:        springLineAt(text, endpoint.offset),
				key:         endpoint.httpMethod + " " + strings.Join(endpoint.paths, ","),
				title:       title,
				controller:  controller,
				endpoint:    endpoint,
				hasEndpoint: true,
			})
		}
	}
	return marks
}

// springGutter paints OpenIDEA's file-http action icon on controller and
// request-mapping declaration lines. MyGo exposes line geometry but not custom
// gutter drawings, so the native button is anchored to that geometry.
func (m *Model) springGutter(
	c *ui.Context,
	p *ui.Theme,
	t *Tab,
	editorRow, view ui.Element,
) {
	if t == nil || t.doc == nil || t.state == nil || t.state.Buffer == nil ||
		!springDocumentPath(t.doc.Path()) {
		return
	}
	text := t.state.Buffer.String()
	marks := springMarksForText(m.springControllersForTab(t), text)
	if len(marks) == 0 {
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
	for _, mark := range marks {
		if mark.line < first || mark.line > last {
			continue
		}
		top := float32(t.state.LineTop(mark.line) - t.state.ScrollY())
		height := float32(t.state.LineTop(mark.line+1) - t.state.LineTop(mark.line))
		if height <= 0 {
			height = t.state.Font.Size * t.state.LineHeight
		}
		if top+height <= 0 || top >= viewBounds.H {
			continue
		}
		localX := viewBounds.X - rowBounds.X + 1
		localY := viewBounds.Y - rowBounds.Y + top + max(0, (height-14)/2)
		mark := mark
		button := ui.Row(c.Key("spring-gutter:"+t.doc.Path()+":"+mark.key)).
			Absolute().Left(localX).Top(localY).Size(14, 14).
			AlignItems(ui.Center).Justify(ui.Center).Radius(3).
			Label(mark.title).Tooltip(mark.title).Cursor(ui.CursorPointer)
		if button.Hovered() {
			button.Background(oi.hoverTint)
		}
		button.Menu(func(menu *ui.Menu) { m.springGutterMenu(menu, t, mark) })
		button.Children(func() {
			oiIconEl(c, "fileTypes/http", 14, oi, oi.textMuted)
		})
	}
}
