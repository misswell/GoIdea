package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	markdownhtml "github.com/yuin/goldmark/renderer/html"
	"golang.org/x/net/html"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
	"mayoi/internal/position"
)

const documentationWindow = "documentation"

type documentationState struct {
	popup        *documentationView
	window       documentationView
	tab          *Tab
	offset       int
	version      uint64
	anchor       ui.Rect
	virtual      map[string]*Tab
	request      uint32
	pending      *documentationView
	cancel       context.CancelFunc
	popupFocused bool
	focus        ui.Handle
	dockFocus    ui.Handle
}

type documentationView struct {
	entries []documentationEntry
	index   int
	loading bool
	message string
}

type documentationEntry struct {
	blocks []documentationBlock
	source *documentationTarget
}

type documentationTarget struct {
	path    string
	pos     openidea.OutlinePos
	doc     *docbridge.Document
	virtual bool
}

func (v *documentationView) current() *documentationEntry {
	if len(v.entries) == 0 {
		return nil
	}
	return &v.entries[v.index]
}

func (v *documentationView) push(entry documentationEntry) {
	if len(v.entries) > 0 {
		clear(v.entries[v.index+1:])
		v.entries = v.entries[:v.index+1]
	}
	v.entries = append(v.entries, entry)
	// Documentation can be browsed for a whole editing session. Hold at most
	// the last 50 results and discard forward entries after following a link.
	if len(v.entries) > 50 {
		v.entries = append([]documentationEntry(nil), v.entries[len(v.entries)-50:]...)
	}
	v.index = len(v.entries) - 1
	v.message = ""
}

func (m *Model) finishDocumentationRequest() {
	d := &m.documentation
	if d.request == 0 {
		return
	}
	id := d.request
	m.completeDocumentationRequest()
	if m.mgr != nil {
		m.cancelRequest(id)
	}
}

func (m *Model) completeDocumentationRequest() {
	d := &m.documentation
	d.pending.loading = false
	d.cancel()
	d.request, d.pending, d.cancel = 0, nil, nil
}

func (m *Model) closeDocumentationPopup() {
	d := &m.documentation
	if d.popup == nil {
		return
	}
	m.finishDocumentationRequest()
	d.popup, d.tab, d.anchor = nil, nil, ui.Rect{}
	d.popupFocused = false
	d.focus.CancelFocus()
	m.invalidate()
}

func (m *Model) resetDocumentation() {
	m.finishDocumentationRequest()
	m.documentation = documentationState{}
}

func (m *Model) dockDocumentation() {
	d := &m.documentation
	if d.popup == nil {
		return
	}
	m.DockShow(documentationWindow)
	if !m.toolVisible(documentationWindow) {
		return
	}
	d.window = *d.popup
	d.popup, d.tab, d.anchor = nil, nil, ui.Rect{}
	d.popupFocused = false
	d.focus.CancelFocus()
	d.dockFocus.Focus()
	m.invalidate()
}

// QuickDocumentation shows documentation at the caret. Repeating the action
// gives the popup the keyboard, then moves its history into the tool window.
func (m *Model) QuickDocumentation() {
	d := &m.documentation
	if m.PowerSaveMode() {
		m.status = "Power Save Mode is on: documentation popups are off"
		m.invalidate()
		return
	}
	if d.popup != nil {
		if d.popup.loading {
			return
		}
		if d.popupFocused {
			m.dockDocumentation()
			return
		}
		d.focus.Focus()
		m.invalidate()
		return
	}
	t := m.tab()
	if t == nil || t.doc == nil || t.doc.ID() == 0 || m.ctx == nil || (m.mgr == nil && m.invokeHook == nil) {
		return
	}
	if item, info, ok := m.selectedCompletion(t); ok &&
		(info.LSP != nil || strings.TrimSpace(item.Documentation) != "") {
		m.quickCompletionDocumentation(t, item, info)
		return
	}
	view := &d.window
	popup := !m.toolVisible(documentationWindow)
	off := t.state.Cursor()
	version := t.doc.Buffer().Version()
	if popup {
		view = &documentationView{}
		d.popup, d.tab, d.offset, d.version = view, t, off, version
	}
	source := documentationTarget{doc: t.doc, pos: t.doc.UTF16Pos(off)}
	id, ctx := m.beginDocumentationRequest(view)
	root := m.Root()
	go func() {
		err := source.doc.Sync(ctx)
		var hover *openidea.Hover
		if err == nil {
			hover, err = m.documentationHover(ctx, id, source.doc.ID(), source.pos)
		}
		var entry documentationEntry
		if hover != nil {
			entry = documentationEntry{blocks: documentationBlocks(hover.Contents, source.doc.Info().Language), source: &source}
		}
		m.post(func() {
			if d.request != id || m.Root() != root {
				return
			}
			m.completeDocumentationRequest()
			if m.tab() != t || t.doc.Buffer().Version() != version || t.state.Cursor() != off {
				if popup {
					m.closeDocumentationPopup()
				}
				m.invalidate()
				return
			}
			if err != nil {
				view.message = "Could not load documentation: " + err.Error()
			} else if len(entry.blocks) == 0 {
				view.message = "No documentation found"
			} else {
				if popup && hover.Range != nil {
					d.offset, _ = t.doc.ByteRange(*hover.Range)
				}
				view.push(entry)
			}
			m.invalidate()
		})
	}()
}

func (m *Model) quickCompletionDocumentation(t *Tab, item editor.Completion, info completionInfo) {
	d := &m.documentation
	view := &d.window
	popup := !m.toolVisible(documentationWindow)
	offset := t.state.Cursor()
	version := t.doc.Buffer().Version()
	if popup {
		view = &documentationView{}
		d.popup, d.tab, d.offset, d.version = view, t, offset, version
	}
	if info.LSP == nil || strings.TrimSpace(item.Documentation) != "" ||
		info.LSP.Documentation != nil {
		entry := documentationEntry{blocks: completionDocumentationBlocks(item, info.LSP, t.doc.Info().Language)}
		if len(entry.blocks) == 0 {
			view.message = "No documentation found"
		} else {
			view.push(entry)
		}
		m.invalidate()
		return
	}
	source := *info.LSP
	resolveData := strings.TrimSpace(string(source.Data))
	if resolveData == "" || resolveData == "null" || source.Server == 0 {
		entry := documentationEntry{blocks: completionDocumentationBlocks(item, &source, t.doc.Info().Language)}
		if len(entry.blocks) == 0 {
			view.message = "No documentation found"
		} else {
			view.push(entry)
		}
		m.invalidate()
		return
	}

	docID := t.doc.ID()
	root := m.Root()
	selected := completionTrackKey(item)
	resolveKey := completionResolveKey(&source)
	language := t.doc.Info().Language
	id, ctx := m.beginDocumentationRequest(view)
	go func() {
		resolved, err := m.documentationResolveCompletion(ctx, docID, source)
		entry := documentationEntry{
			blocks: completionDocumentationBlocks(item, &source, language),
		}
		if err == nil {
			entry.blocks = completionDocumentationBlocks(item, &resolved, language)
		}
		m.post(func() {
			if d.request != id {
				return
			}
			m.completeDocumentationRequest()
			current, currentInfo, selectedNow := m.selectedCompletion(t)
			stale := m.Root() != root || m.tab() != t ||
				t.doc.Buffer().Version() != version || t.state.Cursor() != offset ||
				!selectedNow || currentInfo.LSP == nil ||
				completionTrackKey(current) != selected ||
				completionResolveKey(currentInfo.LSP) != resolveKey
			if stale {
				if popup {
					m.closeDocumentationPopup()
				}
				m.invalidate()
				return
			}
			if err != nil {
				if len(entry.blocks) > 0 {
					view.push(entry)
				}
				view.message = "Could not load documentation: " + err.Error()
			} else if len(entry.blocks) == 0 {
				view.message = "No documentation found"
			} else {
				view.push(entry)
			}
			m.invalidate()
		})
	}()
}

func completionResolveKey(item *openidea.CompletionItem) string {
	if item == nil {
		return ""
	}
	return item.Label + "\x00" + item.InsertText + "\x00" +
		strconv.FormatUint(uint64(item.Server), 10) + "\x00" + string(item.Data)
}

func (m *Model) documentationResolveCompletion(
	ctx context.Context,
	doc uint32,
	item openidea.CompletionItem,
) (openidea.CompletionItem, error) {
	reply, err := m.invoke(ctx, "lsp_resolve_completion", map[string]any{"doc": doc, "item": item})
	if err != nil {
		return openidea.CompletionItem{}, err
	}
	var resolved openidea.CompletionItem
	if err := json.Unmarshal(reply, &resolved); err != nil {
		return openidea.CompletionItem{}, err
	}
	return resolved, nil
}

func completionDocumentationBlocks(
	item editor.Completion,
	source *openidea.CompletionItem,
	language string,
) []documentationBlock {
	var out []documentationBlock
	detail := strings.TrimSpace(item.Detail)
	labelDetail := ""
	if source != nil {
		if source.LabelDetail != nil {
			labelDetail = strings.TrimSpace(*source.LabelDetail)
		}
		if source.Detail != nil && detail == "" {
			detail = strings.TrimSpace(*source.Detail)
		}
	}
	detail = strings.TrimSpace(detail)
	if detail != "" {
		code := detail
		if labelDetail != "" && strings.HasPrefix(detail, "(") {
			code = item.Label + detail
		}
		markup := openidea.Markup{Kind: "markdown", Value: "```" + strings.ToLower(language) + "\n" + code + "\n```"}
		out = append(out, documentationBlocks(markup, language)...)
	}
	if source != nil && source.Documentation != nil && strings.TrimSpace(source.Documentation.Value) != "" {
		out = append(out, documentationBlocks(*source.Documentation, language)...)
	} else if strings.TrimSpace(item.Documentation) != "" {
		out = append(out, documentationBlocks(
			openidea.Markup{Kind: "plaintext", Value: item.Documentation}, language,
		)...)
	}
	return out
}

func (m *Model) beginDocumentationRequest(view *documentationView) (uint32, context.Context) {
	m.finishDocumentationRequest()
	ctx, cancel := context.WithTimeout(m.ctx, lspTimeout)
	id := m.nextRequest()
	m.documentation.request, m.documentation.pending, m.documentation.cancel = id, view, cancel
	view.loading, view.message = true, ""
	m.invalidate()
	return id, ctx
}

func (m *Model) documentationHover(ctx context.Context, req, doc uint32, pos openidea.OutlinePos) (*openidea.Hover, error) {
	reply, err := m.invoke(ctx, "lsp_hover", map[string]any{"request": req, "doc": doc, "pos": lspPos(pos)})
	var hover *openidea.Hover
	if err == nil {
		err = json.Unmarshal(reply, &hover)
	}
	return hover, err
}

// smokeDocumentation drives the command over an indexed Java symbol and
// confirms that docking preserves the language server's content.
func (m *Model) smokeDocumentation(ctx context.Context, t *Tab, offset int) map[string]any {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	defer m.post(func() {
		m.closeDocumentationPopup()
		if m.toolVisible(documentationWindow) {
			m.DockToggle(documentationWindow)
		}
	})
	m.post(func() {
		t.state.Select(offset, offset)
		m.RunCommand("code.quickDocumentation")
	})
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		var out map[string]any
		m.post(func() {
			popup := m.documentation.popup
			if popup == nil || popup.loading {
				return
			}
			entry := popup.current()
			if entry == nil || len(entry.blocks) == 0 {
				out = map[string]any{"error": popup.message}
				return
			}
			m.dockDocumentation()
			docked := m.documentation.window.current()
			out = map[string]any{"blocks": len(entry.blocks), "docked": m.toolVisible(documentationWindow) && docked != nil && len(docked.blocks) == len(entry.blocks)}
		})
		if out != nil {
			return out
		}
		select {
		case <-ctx.Done():
			return map[string]any{"error": ctx.Err().Error()}
		case <-ticker.C:
		}
	}
}

// positionDocumentation anchors the popup below the symbol's line in the
// active editor, including when that editor is in a split.
func (m *Model) positionDocumentation(t *Tab, view ui.Element) {
	d := &m.documentation
	if d.popup == nil || d.tab != t || m.tab() != t {
		return
	}
	r := view.Bounds()
	if r.W == 0 || r.H == 0 {
		return
	}
	// The editor exposes the line layout and its scroll origin. Use them so
	// folded content above the symbol does not shift the documentation anchor.
	pos := t.doc.UTF16Pos(d.offset)
	line := int(pos.Line)
	top := float32(t.state.LineTop(line) - t.state.ScrollY())
	lineHeight := float32(t.state.LineTop(line+1) - t.state.LineTop(line))
	if lineHeight <= 0 || lineHeight > t.state.Font.Size*4 {
		lineHeight = t.state.Font.Size * t.state.LineHeight
	}
	d.anchor = ui.Rect{X: r.X + 44, Y: r.Y + min(max(0, top), r.H), H: max(18, lineHeight)}
}

func (m *Model) documentationPopup(c *ui.Context, p *ui.Theme) {
	d := &m.documentation
	v := d.popup
	if d.tab == nil || d.tab != m.tab() || d.tab.doc.Buffer().Version() != d.version {
		m.closeDocumentationPopup()
		return
	}
	width, height := c.Size()
	w, h := max(1, min(520, width-16)), max(1, min(330, height-16))
	x, y := d.anchor.X, d.anchor.Y+d.anchor.H+4
	if d.anchor.H == 0 {
		x, y = width/2-w/2, height/2-h/2
	} else if y+h > height-8 && d.anchor.Y-4-h > 8 {
		y = d.anchor.Y - 4 - h
	}
	oi := oiPaletteOf(p)
	ui.Overlay(c, func() {
		box := ui.Column(c.Key("documentation:popup")).Role(ui.RolePopup).Label("Documentation").Focusable().Bind(&d.focus).
			Absolute().Left(max(8, min(x, width-w-8))).Top(max(8, min(y, height-h-8))).
			Width(w).Height(h).MinWidth(0).Clip().Background(oi.popup).Border(1, oi.borderStrong).
			Radius(6).Shadow(0, 8, 24, 0, ui.RGBA(0, 0, 0, 0.28))
		box.Children(func() { m.drawDocumentation(c, p, v) })
		if box.PressedOutside() || box.OverlayShortcut(0, ui.KeyEscape) {
			m.closeDocumentationPopup()
			return
		}
		d.popupFocused = box.FocusWithin()
		m.documentationKeys(box, v)
	})
}

func (m *Model) documentationPanel(c *ui.Context, p *ui.Theme) {
	d := &m.documentation
	oi := oiPaletteOf(p)
	background := oi.island
	if m.prefs.Appearance.DifferentToolWindowBackground {
		background = oi.elev
	}
	box := ui.Column(c.Key("documentation:tool")).Grow(1).MinHeight(0).MinWidth(0).
		Background(background).Radius(oiIslandRadius).Focusable().Bind(&d.dockFocus)
	box.Children(func() {
		ui.Row(c).Height(oiTokensFor(m, p).HeaderHeight).Shrink(0).Padding(0, 6, 0, 8).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Documentation").FontSize(13).TextColor(oi.textStrong).SingleLine().Ellipsis("…")
			ui.Spacer(c)
			m.headerIcon(c, p, oi, "documentation-close", "Hide", "general/remove", func() { m.DockHide(documentationWindow) })
		})
		m.drawDocumentation(c, p, &d.window)
	})
	m.documentationKeys(box, &d.window)
}

func (m *Model) documentationKeys(box ui.Element, v *documentationView) {
	switch {
	case box.Shortcut(0, ui.KeyLeft):
		m.stepDocumentation(v, -1)
	case box.Shortcut(0, ui.KeyRight):
		m.stepDocumentation(v, 1)
	case box.Shortcut(0, ui.KeyF4):
		m.editDocumentationSource(v)
	}
}

func (m *Model) stepDocumentation(v *documentationView, delta int) {
	if v.loading || len(v.entries) == 0 {
		return
	}
	v.index = min(max(0, v.index+delta), len(v.entries)-1)
	v.message = ""
	m.invalidate()
}

func (m *Model) drawDocumentation(c *ui.Context, p *ui.Theme, v *documentationView) {
	entry := v.current()
	message := v.message
	if v.loading {
		message = "Loading documentation…"
	}
	if entry == nil && message == "" {
		message = "No documentation"
	}
	codeTheme := editor.DefaultTheme(p)
	linkColor := oiPaletteOf(p).link
	ui.Scroll(c.Key(strconv.Itoa(len(v.entries)) + ":" + strconv.Itoa(v.index))).
		Grow(1).MinHeight(0).MinWidth(0).Children(func() {
		ui.Column(c).MinWidth(0).Padding(12).Gap(8).Selectable().Children(func() {
			if message != "" {
				ui.Text(c, message).TextColor(p.TextMuted).Wrap()
			}
			if entry != nil {
				for _, block := range entry.blocks {
					m.drawDocumentationBlock(c, p, codeTheme, linkColor, v, block)
				}
			}
		})
	})
	ui.Row(c).Shrink(0).Padding(4, 8).Gap(3).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(p.Border).Children(func() {
		documentationButton(c, "←", "Back", v.loading || v.index == 0, func() { m.stepDocumentation(v, -1) })
		documentationButton(c, "→", "Forward", v.loading || v.index+1 >= len(v.entries), func() { m.stepDocumentation(v, 1) })
		ui.Spacer(c)
		if entry != nil && entry.source != nil {
			documentationButton(c, "Edit Source", "Edit Source (F4)", v.loading, func() { m.editDocumentationSource(v) })
		}
		if v == m.documentation.popup {
			documentationButton(c, "Show in Tool Window", "Show in Documentation Tool Window", v.loading, m.dockDocumentation)
		}
	})
}

func documentationButton(c *ui.Context, label, description string, disabled bool, run func()) {
	b := ui.Button(c, label).FontSize(12).Tooltip(description).Disabled(disabled)
	if b.Clicked() {
		run()
	}
}

func (m *Model) editDocumentationSource(v *documentationView) {
	entry := v.current()
	if entry == nil || entry.source == nil || v.loading {
		return
	}
	source := *entry.source
	if source.doc == nil {
		m.openDocumentationSource(v, source)
		return
	}
	id, ctx := m.beginDocumentationRequest(v)
	root := m.Root()
	go func() {
		reply, err := m.invoke(ctx, "lsp_goto", map[string]any{
			"doc": source.doc.ID(), "pos": lspPos(source.pos), "kind": "definition",
		})
		var targets []openidea.Location
		if err == nil {
			err = json.Unmarshal(reply, &targets)
		}
		m.post(func() {
			if m.documentation.request != id || m.Root() != root {
				return
			}
			m.completeDocumentationRequest()
			if err != nil || len(targets) == 0 {
				v.message = "No declaration found"
				m.invalidate()
				return
			}
			loc := targets[0]
			target := documentationTarget{path: loc.Path, pos: loc.Range.Start}
			uri := loc.Path
			if loc.URI != nil {
				uri = *loc.URI
			}
			if virtual, ok := documentationLink(uri); ok && virtual.virtual {
				target.path, target.virtual = virtual.path, true
			}
			m.openDocumentationSource(v, target)
		})
	}()
}

var documentationFragment = regexp.MustCompile(`^#?L?(\d+)(?:[,:](\d+))?`)
var documentationIdentifier = regexp.MustCompile(`[\p{L}_$][\p{L}\p{N}_$]*`)
var documentationKeyword = regexp.MustCompile(`^(?:export|default|declare|abstract|public|private|protected|internal|static|final|async|const|let|var|val|function|fun|fn|func|def|class|interface|struct|enum|trait|type|impl|pub|mod|object|record|override|readonly|open|data|sealed|extends|implements)$`)

func documentationLink(href string) (documentationTarget, bool) {
	u, err := url.Parse(href)
	if err != nil {
		return documentationTarget{}, false
	}
	var t documentationTarget
	switch strings.ToLower(u.Scheme) {
	case "file":
		// A documented link is untrusted. Local files and dependency sources
		// are navigable, but following a link must never touch a remote share.
		if (u.Host != "" && u.Host != "localhost") || !filepath.IsAbs(u.Path) || strings.HasPrefix(u.Path, "//") {
			return t, false
		}
		t.path = u.Path
		if strings.EqualFold(filepath.Ext(u.Path), ".class") {
			t.virtual = true
		}
	case "jdt", "jar", "zip", "jrt", "kls", "revision":
		t.virtual = true
	default:
		return t, false
	}
	if t.virtual {
		t.path, _, _ = strings.Cut(href, "#")
	}
	if matches := documentationFragment.FindStringSubmatch(u.Fragment); len(matches) != 0 {
		line, _ := strconv.ParseUint(matches[1], 10, 32)
		col, _ := strconv.ParseUint(matches[2], 10, 32)
		t.pos = openidea.OutlinePos{
			Line: uint32(max(1, line) - 1), Col: uint32(max(1, col) - 1),
		}
	}
	return t, true
}

func (m *Model) followDocumentationLink(v *documentationView, href string, mods ui.Modifiers) {
	t, hasSource := documentationLink(href)
	if !hasSource {
		if web, err := url.Parse(href); err == nil && web.Hostname() != "" && (web.Scheme == "http" || web.Scheme == "https") {
			openReleasePage(href)
		}
		return
	}
	if v.loading || m.ctx == nil || (m.mgr == nil && m.invokeHook == nil) {
		return
	}
	if mods&(ui.Cmd|ui.Ctrl) != 0 {
		m.openDocumentationSource(v, t)
		return
	}
	id, ctx := m.beginDocumentationRequest(v)
	root := m.Root()
	go func() {
		entry := m.documentationAt(ctx, id, t)
		m.post(func() {
			if m.documentation.request != id || m.Root() != root {
				return
			}
			m.completeDocumentationRequest()
			if entry == nil {
				m.openDocumentationSource(v, t)
				return
			}
			v.push(*entry)
			m.invalidate()
		})
	}()
}

func (m *Model) documentationAt(ctx context.Context, request uint32, target documentationTarget) *documentationEntry {
	if !target.virtual && m.reg != nil {
		if doc := m.reg.Get(target.path); doc != nil && doc.Sync(ctx) == nil {
			return m.documentationAtText(ctx, request, target, doc.ID(), doc.Buffer().Line(int(target.pos.Line)))
		}
	}
	var info openidea.DocInfo
	command, args := "doc_open", map[string]any{"path": target.path}
	if target.virtual {
		command, args = "doc_open_virtual", map[string]any{"uri": target.path}
	}
	raw, err := m.invoke(ctx, command, args)
	if err != nil || json.Unmarshal(raw, &info) != nil || info.ID == 0 {
		return nil
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		_, _ = m.invoke(ctx, "doc_close", map[string]any{"doc": info.ID})
	}()
	var line string
	for i, text := range strings.Split(strings.ReplaceAll(info.Text, "\r\n", "\n"), "\n") {
		if uint32(i) == target.pos.Line {
			line = text
			break
		}
	}
	return m.documentationAtText(ctx, request, target, info.ID, line)
}

func documentationHoverColumns(line string, col uint32) []uint32 {
	cols := []uint32{col}
	for _, match := range documentationIdentifier.FindAllStringIndex(line, -1) {
		at := uint32(position.UTF16Length(line[:match[0]]))
		if at > col && !documentationKeyword.MatchString(line[match[0]:match[1]]) {
			cols = append(cols, at)
			if len(cols) == 5 {
				break
			}
		}
	}
	return cols
}

func (m *Model) documentationAtText(ctx context.Context, request uint32, target documentationTarget, doc uint32, line string) *documentationEntry {
	originalCol := target.pos.Col
	language := ""
	if lang := editor.LanguageFor(target.path); lang != nil {
		language = lang.Name
	}
	for _, col := range documentationHoverColumns(line, originalCol) {
		if ctx.Err() != nil {
			break
		}
		target.pos.Col = col
		hover, err := m.documentationHover(ctx, request, doc, target.pos)
		if err == nil && hover != nil {
			if blocks := documentationBlocks(hover.Contents, language); len(blocks) > 0 {
				return &documentationEntry{blocks: blocks, source: &target}
			}
		}
	}
	return nil
}

const documentationSourceTab = "documentationSource"

func (m *Model) openDocumentationSource(v *documentationView, target documentationTarget) {
	if target.path == "" {
		v.message = "No declaration found"
		m.invalidate()
		return
	}
	if !target.virtual {
		if v == m.documentation.popup {
			m.closeDocumentationPopup()
		}
		if path := realPath(target.path); filepath.IsAbs(path) {
			m.revealAt(path, int(target.pos.Line), int(target.pos.Col))
		}
		return
	}
	d := &m.documentation
	if t := d.virtual[target.path]; t != nil && m.hasTab(t) {
		m.focusDocumentationSource(v, t, target.pos)
		return
	}
	id, ctx := m.beginDocumentationRequest(v)
	root := m.Root()
	go func() {
		raw, err := m.invoke(ctx, "lsp_virtual_document", map[string]any{"uri": target.path})
		var source struct {
			Title      string `json:"title"`
			Text       string `json:"text"`
			LanguageID string `json:"languageId"`
		}
		if err == nil {
			err = json.Unmarshal(raw, &source)
		}
		m.post(func() {
			if d.request != id || m.Root() != root {
				return
			}
			m.completeDocumentationRequest()
			if err != nil {
				v.message = "Could not open dependency source: " + err.Error()
				m.invalidate()
				return
			}
			if source.Title == "" {
				source.Title, _, _ = strings.Cut(filepath.Base(target.path), "?")
			}
			doc := docbridge.Virtual(source.Title, source.Text)
			if lang := editor.LanguageNamed(source.LanguageID); lang != nil {
				doc.Buffer().SetLanguage(lang)
			}
			version := doc.Buffer().Version()
			t := &Tab{doc: doc, kind: documentationSourceTab, gitOff: true,
				seen: version, synced: version, outlineVersion: version, state: &editor.State{
					Buffer: doc.Buffer(), ReadOnly: true, Font: ui.Font{Family: oiFontMono, Size: m.prefs.fontSize()},
					LineHeight: editorLineHeight(m.prefs.fontSize()), Bindings: m.editorBindings(),
				}}
			m.noteTabUse(t)
			m.insertTab(t)
			m.applyEditorFontPrefs(m.prefs.Font, m.prefs.fontSize())
			m.applyTabAppearance(t, m.prefs.Editor)
			if d.virtual == nil {
				d.virtual = make(map[string]*Tab)
			}
			for uri, old := range d.virtual {
				if !m.hasTab(old) {
					delete(d.virtual, uri)
				}
			}
			d.virtual[target.path] = t
			m.focusDocumentationSource(v, t, target.pos)
		})
	}()
}

func (m *Model) focusDocumentationSource(v *documentationView, t *Tab, pos openidea.OutlinePos) {
	if v == m.documentation.popup {
		m.closeDocumentationPopup()
	}
	if g, ok := m.groupOf(t); ok {
		m.focusGroup(g)
	}
	m.activate(m.tabIndex(t))
	line, col := t.doc.BytePos(int(pos.Line), int(pos.Col))
	t.state.GoTo(editor.Pos{Line: line, Col: col})
	t.state.ScrollTo(line)
	m.invalidate()
}

type documentationRun struct {
	text, href                 string
	bold, italic, code, strike bool
	token                      editor.TokenKind
}

type documentationBlock struct {
	runs                []documentationRun
	indent              int
	heading, code, rule bool
}

var documentationMarkdown = goldmark.New(goldmark.WithExtensions(extension.GFM),
	goldmark.WithRendererOptions(markdownhtml.WithUnsafe()))
var documentationWhitespace = regexp.MustCompile(`[\t\r\n ]+`)

// Translate markup to native blocks and text runs. The intermediate markup is
// parsed as data. The workbench draws text and accepts only known link schemes.
func documentationBlocks(markup openidea.Markup, language ...string) []documentationBlock {
	plain := func() []documentationBlock {
		if text := strings.TrimSpace(markup.Value); text != "" {
			return []documentationBlock{{runs: []documentationRun{{text: text}}}}
		}
		return nil
	}
	if markup.Kind != "markdown" {
		return plain()
	}
	var stream bytes.Buffer
	if err := documentationMarkdown.Convert([]byte(markup.Value), &stream); err != nil {
		return plain()
	}
	tree, err := html.Parse(&stream)
	if err != nil {
		return plain()
	}
	p := documentationParser{}
	if len(language) > 0 {
		p.language = language[0]
	}
	p.walk(tree, documentationRun{})
	p.flush()
	return p.blocks
}

type documentationParser struct {
	blocks   []documentationBlock
	block    documentationBlock
	language string
}

func (p *documentationParser) flush() {
	runs := p.block.runs
	p.block.runs = nil
	for len(runs) > 0 {
		runs[0].text = strings.TrimLeft(runs[0].text, " \t\n")
		if runs[0].text != "" {
			break
		}
		runs = runs[1:]
	}
	for len(runs) > 0 {
		last := &runs[len(runs)-1]
		last.text = strings.TrimRight(last.text, " \t\n")
		if last.text != "" {
			break
		}
		runs = runs[:len(runs)-1]
	}
	if len(runs) > 0 {
		block := p.block
		block.runs = runs
		p.blocks = append(p.blocks, block)
	}
}

func (p *documentationParser) walk(n *html.Node, style documentationRun) {
	if n.Type == html.TextNode {
		style.text = documentationWhitespace.ReplaceAllString(n.Data, " ")
		if strings.HasPrefix(style.text, " ") && len(p.block.runs) > 0 {
			prev := p.block.runs[len(p.block.runs)-1].text
			if strings.HasSuffix(prev, " ") || strings.HasSuffix(prev, "\n") {
				style.text = strings.TrimPrefix(style.text, " ")
			}
		}
		p.block.runs = append(p.block.runs, style)
		return
	}
	if n.Type == html.CommentNode {
		return
	}
	switch n.Data {
	case "head", "script", "style", "iframe", "object", "svg", "template", "textarea":
		return
	case "pre":
		p.flush()
		var text strings.Builder
		docNodeText(&text, n)
		language := p.language
		if child := n.FirstChild; child != nil && child.Data == "code" {
			if name, ok := strings.CutPrefix(docAttribute(child, "class"), "language-"); ok {
				language = name
			}
		}
		runs := documentationCode(strings.TrimSuffix(text.String(), "\n"), language)
		p.blocks = append(p.blocks, documentationBlock{runs: runs, code: true, indent: p.block.indent})
		return
	case "hr":
		p.flush()
		p.blocks = append(p.blocks, documentationBlock{rule: true})
		return
	case "br":
		p.block.runs = append(p.block.runs, documentationRun{text: "\n"})
		return
	case "p", "div", "tr", "dt", "dd":
		p.flush()
		defer p.flush()
	case "ul", "ol":
		p.flush()
		p.walkList(n, style)
		return
	case "blockquote":
		p.flush()
		p.block.indent++
		defer func() {
			p.flush()
			p.block.indent--
		}()
	case "h1", "h2", "h3", "h4", "h5", "h6":
		p.flush()
		p.block.heading = true
		defer func() {
			p.flush()
			p.block.heading = false
		}()
	case "strong", "b", "th":
		style.bold = true
	case "em", "i":
		style.italic = true
	case "code", "kbd", "samp":
		style.code = true
	case "s", "del":
		style.strike = true
	case "a":
		href := docAttribute(n, "href")
		if _, ok := documentationLink(href); ok || strings.HasPrefix(href, "https://") || strings.HasPrefix(href, "http://") {
			style.href = href
		}
	case "img":
		if alt := docAttribute(n, "alt"); alt != "" {
			style.text = alt
			p.block.runs = append(p.block.runs, style)
		}
		return
	}
	if (n.Data == "th" || n.Data == "td") && len(p.block.runs) != 0 {
		p.block.runs = append(p.block.runs, documentationRun{text: "  │  "})
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		p.walk(child, style)
	}
}

func (p *documentationParser) walkList(n *html.Node, style documentationRun) {
	p.block.indent++
	defer func() { p.block.indent-- }()
	num, _ := strconv.Atoi(docAttribute(n, "start"))
	num = max(1, num)
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Data != "li" {
			continue
		}
		prefix := "• "
		if n.Data == "ol" {
			prefix = strconv.Itoa(num) + ". "
			num++
		}
		p.block.runs = append(p.block.runs, documentationRun{text: prefix})
		for item := child.FirstChild; item != nil; item = item.NextSibling {
			if item.Data == "p" && len(p.block.runs) == 1 && p.block.runs[0].text == prefix {
				// A loose Markdown list wraps its first line in a paragraph.
				for sub := item.FirstChild; sub != nil; sub = sub.NextSibling {
					p.walk(sub, style)
				}
			} else {
				p.walk(item, style)
			}
		}
		p.flush()
	}
}

func docAttribute(n *html.Node, name string) string {
	for _, attr := range n.Attr {
		if attr.Key == name {
			return attr.Val
		}
	}
	return ""
}

func docNodeText(out *strings.Builder, n *html.Node) {
	if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style") {
		return
	}
	if n.Type == html.TextNode {
		out.WriteString(n.Data)
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		docNodeText(out, child)
	}
}

func documentationCode(text, name string) []documentationRun {
	lang := editor.LanguageNamed(name)
	if lang == nil || lang.Tokenizer == nil {
		return []documentationRun{{text: text, code: true}}
	}
	parts := strings.Split(text, "\n")
	var runs []documentationRun
	var state uint64
	for i, line := range parts {
		tokens, nextState := lang.Tokenizer.Tokenize(line, state, nil)
		state = nextState
		at := 0
		for _, tok := range tokens {
			runs = append(runs, documentationRun{text: line[at:tok.End], code: true, token: tok.Kind})
			at = tok.End
		}
		runs = append(runs, documentationRun{text: line[at:], code: true})
		if i+1 < len(parts) {
			runs = append(runs, documentationRun{text: "\n", code: true})
		}
	}
	return runs
}

func (m *Model) drawDocumentationBlock(c *ui.Context, p *ui.Theme, codeTheme *editor.Theme, linkColor ui.Color, v *documentationView, block documentationBlock) {
	if block.rule {
		ui.Row(c).Height(1).Background(p.Border)
		return
	}
	para := ui.RichText(c).MinWidth(0).MarginStart(float32(block.indent) * 12).FontSize(13).Wrap()
	if block.heading {
		para.Bold().FontSize(15)
	}
	if block.code {
		para.Padding(6, 8).Background(p.Surface).Radius(4)
	}
	para.Children(func() {
		for _, run := range block.runs {
			if run.text == "" {
				continue
			}
			var span ui.Element
			if run.href != "" {
				span = ui.Link(c, run.text, "").TextColor(linkColor)
				span.OnClick(func() { m.followDocumentationLink(v, run.href, span.ClickModifiers()) })
			} else {
				span = ui.Text(c, run.text)
			}
			if run.code {
				span.Font(oiFontMono)
				if run.token != editor.TokenText && run.href == "" {
					span.TextColor(codeTheme.TokenColor(run.token))
				}
			}
			if run.bold {
				span.Bold()
			}
			if run.italic {
				span.Italic()
			}
			if run.strike {
				span.Strikethrough()
			}
		}
	})
}
