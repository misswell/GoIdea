package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
	"mayoi/internal/textdiff"
)

const (
	quickDefinitionContextBefore = 1
	quickDefinitionMaxLines      = 30
	quickDefinitionMaxTargets    = 20
	quickDefinitionSourceTab     = "quickDefinitionSource"
)

// quickDefinitionState is OpenIDEA's Quick Definition popup. The declaration
// is shown in place while the editor keeps the keyboard, and several language
// server targets can be switched with the left and right arrow keys.
type quickDefinitionState struct {
	tab           *Tab
	kind          string
	sourceVersion uint64
	loading       bool
	index         int
	targets       []quickDefinitionTarget
	focus         ui.Handle
}

// quickDefinitionTarget is one declaration source the popup can show.
type quickDefinitionTarget struct {
	location   openidea.Location
	name       string
	title      string
	uri        string
	language   string
	firstLine  int
	text       string
	sourceText string
	virtual    bool
}

// QuickDefinition opens Definition or Type Definition as a source preview
// without leaving the current editor.
func (m *Model) QuickDefinition(kind string) {
	if kind != "definition" && kind != "typeDefinition" {
		return
	}
	t := m.tab()
	if t == nil || t.doc == nil || t.state == nil {
		return
	}
	if m.mgr == nil && m.invokeHook == nil {
		m.status = "Language server unavailable"
		m.invalidate()
		return
	}
	if m.ctx == nil {
		m.status = "Language server unavailable"
		m.invalidate()
		return
	}

	m.quickDefinitionGen++
	gen := m.quickDefinitionGen
	q := &quickDefinitionState{
		tab:           t,
		kind:          kind,
		sourceVersion: t.doc.Buffer().Version(),
		loading:       true,
	}
	m.quickDefinition = q
	m.invalidate()

	docID := t.doc.ID()
	pos := t.doc.UTF16Pos(t.state.Cursor())
	fallbackLanguage := t.doc.Info().Language
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
		defer cancel()
		raw, err := m.invoke(ctx, "lsp_goto", map[string]any{
			"doc": docID, "pos": lspPos(pos), "kind": kind,
		})
		var locations []openidea.Location
		if err == nil {
			err = json.Unmarshal(raw, &locations)
		}
		locations = uniqueQuickDefinitionLocations(locations)
		if len(locations) > quickDefinitionMaxTargets {
			locations = locations[:quickDefinitionMaxTargets]
		}
		targets := make([]quickDefinitionTarget, len(locations))
		found := make([]bool, len(locations))
		done := make(chan struct{}, len(locations))
		for i, location := range locations {
			i, location := i, location
			go func() {
				targets[i], found[i] = m.quickDefinitionTarget(ctx, location, fallbackLanguage)
				done <- struct{}{}
			}()
		}
		for range locations {
			<-done
		}

		m.post(func() {
			if m.quickDefinitionGen != gen || !m.tabOpen(t) || m.tab() != t ||
				m.quickDefinition != q || t.doc.Buffer().Version() != q.sourceVersion {
				return
			}
			switch {
			case err != nil:
				m.closeQuickDefinition()
				m.status = "Navigation failed: " + err.Error()
				m.invalidate()
				return
			case len(locations) == 0:
				m.closeQuickDefinition()
				if kind == "typeDefinition" {
					m.status = "No type definition found"
				} else {
					m.status = "No definition found"
				}
				m.invalidate()
				return
			}

			q.targets = targets[:0]
			for i := range targets {
				if found[i] {
					q.targets = append(q.targets, targets[i])
				}
			}
			if len(q.targets) == 0 {
				m.closeQuickDefinition()
				m.status = "Cannot read the definition's file"
				m.invalidate()
				return
			}
			q.loading = false
			m.invalidate()
		})
	}()
}

func uniqueQuickDefinitionLocations(locations []openidea.Location) []openidea.Location {
	seen := make(map[string]bool, len(locations))
	out := make([]openidea.Location, 0, len(locations))
	for _, location := range locations {
		key := fmt.Sprintf("%s\x00%d\x00%d", location.Path,
			location.Range.Start.Line, location.Range.Start.Col)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, location)
	}
	return out
}

// quickDefinitionTarget reads one declaration and cuts the source span the
// popup shows. Local files are read directly. A dependency source asks the
// backend for its virtual document.
func (m *Model) quickDefinitionTarget(
	ctx context.Context,
	location openidea.Location,
	fallbackLanguage string,
) (quickDefinitionTarget, bool) {
	source, err := m.readQuickDefinitionSource(ctx, location)
	if err != nil {
		return quickDefinitionTarget{}, false
	}
	return quickDefinitionTargetFromSource(location, source, fallbackLanguage), true
}

func quickDefinitionTargetFromSource(
	location openidea.Location,
	source quickDefinitionSource,
	fallbackLanguage string,
) quickDefinitionTarget {
	source.text = strings.ReplaceAll(source.text, "\r\n", "\n")
	lines := textdiff.SplitLines(source.text)
	start := int(location.Range.Start.Line)
	if len(lines) == 0 {
		start = 0
	} else {
		start = min(max(0, start), len(lines)-1)
	}
	first := max(0, start-quickDefinitionContextBefore)
	spanEnd := max(int(location.Range.End.Line), start+12)
	last := len(lines) - 1
	if last >= 0 {
		last = min(last, min(spanEnd, first+quickDefinitionMaxLines-1))
	}
	snippet := ""
	if len(lines) > 0 {
		snippet = strings.Join(lines[first:last+1], "\n")
	}
	name := ""
	if start < len(lines) {
		name = quickDefinitionTargetLabelAtUTF16(lines[start], location.Range.Start.Col)
	}
	if name == "" {
		name = quickDefinitionBase(source.title)
	}
	language := source.language
	if language == "" {
		language = fallbackLanguage
	}
	return quickDefinitionTarget{
		location:   location,
		name:       name,
		title:      source.title,
		uri:        source.uri,
		language:   language,
		firstLine:  first,
		text:       snippet,
		sourceText: source.text,
		virtual:    source.virtual,
	}
}

type quickDefinitionSource struct {
	title    string
	language string
	uri      string
	text     string
	virtual  bool
}

func (m *Model) readQuickDefinitionSource(
	ctx context.Context,
	location openidea.Location,
) (quickDefinitionSource, error) {
	uri, localPath, title, virtual := quickDefinitionLocationSource(location)
	if !virtual {
		if localPath == "" {
			return quickDefinitionSource{}, fmt.Errorf("definition has no local source")
		}
		data, err := os.ReadFile(filepath.Clean(localPath))
		if err != nil {
			return quickDefinitionSource{}, err
		}
		return quickDefinitionSource{
			title: title, language: editor.LanguageFor(localPath).Name,
			uri: uri, text: string(data),
		}, nil
	}
	raw, err := m.invoke(ctx, "lsp_virtual_document", map[string]any{"uri": uri})
	if err != nil {
		return quickDefinitionSource{}, err
	}
	var document struct {
		Title      string `json:"title"`
		Text       string `json:"text"`
		LanguageID string `json:"languageId"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		return quickDefinitionSource{}, err
	}
	if document.Title != "" {
		title = document.Title
	}
	return quickDefinitionSource{
		title: title, language: document.LanguageID, uri: uri,
		text: document.Text, virtual: true,
	}, nil
}

func quickDefinitionLocationSource(location openidea.Location) (
	uri, localPath, title string,
	virtual bool,
) {
	candidates := []string{location.Path}
	if location.URI != nil && *location.URI != "" && *location.URI != location.Path {
		candidates = append(candidates, *location.URI)
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if path, ok := quickDefinitionLocalPath(candidate); ok {
			return candidate, path, quickDefinitionBase(candidate), false
		}
		if quickDefinitionVirtualURI(candidate) {
			return candidate, "", quickDefinitionBase(candidate), true
		}
	}
	for _, candidate := range candidates {
		if filepath.IsAbs(candidate) {
			path := realPath(candidate)
			return candidate, path, quickDefinitionBase(path), false
		}
	}
	if location.Path != "" && !quickDefinitionVirtualURI(location.Path) {
		localPath = realPath(location.Path)
		return location.Path, localPath, quickDefinitionBase(localPath), false
	}
	return "", "", "", false
}

func quickDefinitionLocalPath(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	if u.Scheme == "" {
		if filepath.IsAbs(raw) {
			return realPath(raw), true
		}
		return "", false
	}
	if !strings.EqualFold(u.Scheme, "file") {
		return "", false
	}
	if (u.Host != "" && u.Host != "localhost") || !filepath.IsAbs(u.Path) ||
		strings.HasPrefix(u.Path, "//") {
		return "", false
	}
	return realPath(u.Path), true
}

func quickDefinitionVirtualURI(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "jdt", "jar", "zip", "jrt", "kls", "revision":
		return true
	default:
		return false
	}
}

func quickDefinitionBase(raw string) string {
	u, err := url.Parse(raw)
	if err == nil && u.Path != "" {
		raw = u.Path
	}
	raw, _, _ = strings.Cut(raw, "?")
	raw, _, _ = strings.Cut(raw, "#")
	if name := filepath.Base(filepath.Clean(raw)); name != "." && name != string(filepath.Separator) {
		return name
	}
	return raw
}

var quickDefinitionIdentifier = regexp.MustCompile(`[\p{L}\p{N}_$]+`)

func quickDefinitionTargetLabelAtUTF16(line string, column uint32) string {
	byteColumn := 0
	var units uint32
	for byteColumn < len(line) && units < column {
		r, size := utf8.DecodeRuneInString(line[byteColumn:])
		if r == utf8.RuneError && size == 0 {
			break
		}
		if r > 0xffff {
			units += 2
		} else {
			units++
		}
		byteColumn += size
	}
	if byteColumn < len(line) && strings.ContainsRune("\"'`", rune(line[byteColumn])) {
		quote := line[byteColumn]
		end := byteColumn + 1
		for end < len(line) && line[end] != quote {
			if line[end] == '\\' && end+1 < len(line) {
				end++
			}
			end++
		}
		if end < len(line) {
			end++
		}
		literal := line[byteColumn:end]
		if utf16Len(literal) > 2 {
			if utf16Len(literal) > 60 {
				runes := []rune(literal)
				literal = string(runes[:min(59, len(runes))]) + "…"
			}
			return literal
		}
	}
	for _, match := range quickDefinitionIdentifier.FindAllStringIndex(line, -1) {
		startUTF16 := uint32(utf16Len(line[:match[0]]))
		endUTF16 := startUTF16 + uint32(utf16Len(line[match[0]:match[1]]))
		if endUTF16 < column {
			continue
		}
		if startUTF16 > column+1 {
			return ""
		}
		label := line[match[0]:match[1]]
		if r, _ := utf8.DecodeRuneInString(label); r >= '0' && r <= '9' {
			return ""
		}
		return label
	}
	return ""
}

func quickDefinitionDisplayPath(root, path string) string {
	if root != "" {
		if local, ok := quickDefinitionLocalPath(path); ok {
			if rel, err := filepath.Rel(root, local); err == nil && rel != ".." &&
				!strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return rel
			}
		}
	}
	return quickDefinitionBase(path)
}

// quickDefinitionPopup renders the registered Quick Definition overlay.
func (m *Model) quickDefinitionPopup(c *ui.Context, p *ui.Theme) {
	q := m.quickDefinition
	if q == nil {
		return
	}
	if !m.tabOpen(q.tab) || m.tab() != q.tab ||
		q.tab.doc == nil || q.tab.doc.Buffer().Version() != q.sourceVersion {
		m.closeQuickDefinition()
		return
	}

	oi := oiPaletteOf(p)
	width, height := c.Size()
	w := max(float32(1), min(float32(760), width-24))
	h := max(float32(1), min(float32(420), height-32))
	x := max(float32(8), (width-w)/2)
	y := max(float32(16), min(float32(84), (height-h)/2))
	title := "Definition"
	if q.kind == "typeDefinition" {
		title = "Type Definition"
	}
	ui.Overlay(c, func() {
		box := ui.Column(c.Key("quick-definition:popup")).Role(ui.RolePopup).Label(title).
			Focusable().AutoFocus().Bind(&q.focus).Absolute().Left(x).Top(y).Width(w).Height(h).
			MinWidth(0).Clip().Background(oi.popup).Border(1, oi.borderStrong).Radius(7).
			Shadow(0, 10, 28, 0, ui.RGBA(0, 0, 0, 0.3))
		box.Children(func() {
			m.quickDefinitionHeader(c, p, oi, q, title)
			ui.Row(c).Height(1).Shrink(0).Background(oi.border)
			if q.loading {
				ui.Spacer(c)
				ui.Text(c, "Finding definition…").FontSize(12).TextColor(oi.textMuted).
					AlignSelf(ui.Center)
				ui.Spacer(c)
				return
			}
			target := q.targets[q.index]
			codeTheme := editor.DefaultTheme(p)
			ui.Scroll(c.Key(fmt.Sprintf("quick-definition:%d", q.index))).Grow(1).MinHeight(0).
				MinWidth(0).Children(func() {
				ui.Column(c).MinWidth(0).Padding(12, 14).Gap(8).Selectable().Children(func() {
					m.drawDocumentationBlock(c, p, codeTheme, oi.link, &documentationView{},
						documentationBlock{code: true, runs: documentationCode(target.text, target.language)})
				})
			})
			ui.Row(c).Shrink(0).Height(26).Padding(0, 10).Gap(8).AlignItems(ui.Center).
				BorderWidth(1, 0, 0, 0).BorderColor(oi.border).Children(func() {
				if len(q.targets) > 1 {
					left := ui.Button(c, "←").FontSize(12).Tooltip("Previous Definition")
					if left.Clicked() {
						m.stepQuickDefinition(-1)
					}
					right := ui.Button(c, "→").FontSize(12).Tooltip("Next Definition")
					if right.Clicked() {
						m.stepQuickDefinition(1)
					}
					ui.Text(c, fmt.Sprintf("%d/%d", q.index+1, len(q.targets))).
						FontSize(11).TextColor(oi.textMuted).SingleLine()
				}
				ui.Spacer(c)
				ui.Text(c, "←/→ switch · F4 opens · Esc closes").
					FontSize(11).TextColor(oi.textMuted).SingleLine()
			})
		})
		switch {
		case box.PressedOutside() || box.OverlayShortcut(0, ui.KeyEscape):
			m.closeQuickDefinition()
		case len(q.targets) > 1 && box.OverlayShortcut(0, ui.KeyLeft):
			m.stepQuickDefinition(-1)
		case len(q.targets) > 1 && box.OverlayShortcut(0, ui.KeyRight):
			m.stepQuickDefinition(1)
		case !q.loading && box.OverlayShortcut(0, ui.KeyF4):
			m.openQuickDefinitionTarget(q.targets[q.index])
		}
	})
}

func (m *Model) quickDefinitionHeader(
	c *ui.Context,
	p *ui.Theme,
	oi oiColors,
	q *quickDefinitionState,
	title string,
) {
	ui.Row(c).Height(34).Shrink(0).Padding(0, 10).Gap(8).AlignItems(ui.Center).Children(func() {
		oiIconEl(c, "nodes/class", 15, oi, oi.textMuted)
		ui.Text(c, title).FontSize(12).Bold().TextColor(oi.textStrong).SingleLine()
		if q.loading || len(q.targets) == 0 {
			return
		}
		target := q.targets[q.index]
		name := target.name
		if len(q.targets) > 1 {
			name = fmt.Sprintf("%s (%d/%d)", target.name, q.index+1, len(q.targets))
		}
		ui.Text(c, name).FontSize(12).TextColor(oi.text).SingleLine().Ellipsis("…")
		ui.Text(c, quickDefinitionDisplayPath(m.Root(), target.uri)+
			fmt.Sprintf(":%d", int(target.location.Range.Start.Line)+1)).
			FontSize(11).TextColor(oi.textMuted).SingleLine().Ellipsis("…").Grow(1)
		edit := ui.Button(c, "Edit Source").FontSize(11).Tooltip("Edit Source (F4)")
		if edit.Clicked() {
			m.openQuickDefinitionTarget(target)
		}
		close := ui.ButtonBase(c.Key("quick-definition:close")).Size(22, 22).
			Radius(4).Justify(ui.Center).AlignItems(ui.Center).Cursor(ui.CursorPointer)
		if close.Hovered() {
			close.Background(oi.toolbarHover)
		}
		close.Tooltip("Close")
		close.Children(func() { oiIconEl(c, "general/close", 14, oi, oi.textMuted) })
		if close.Clicked() {
			m.closeQuickDefinition()
		}
	})
}

func (m *Model) stepQuickDefinition(delta int) {
	q := m.quickDefinition
	if q == nil || len(q.targets) == 0 {
		return
	}
	q.index = (q.index + delta + len(q.targets)) % len(q.targets)
	m.invalidate()
}

func (m *Model) closeQuickDefinition() {
	m.quickDefinitionGen++
	m.quickDefinition = nil
	m.invalidate()
}

func (m *Model) openQuickDefinitionTarget(target quickDefinitionTarget) {
	m.closeQuickDefinition()
	line := target.location.Range.Start.Line
	if !target.virtual {
		if local, ok := quickDefinitionLocalPath(target.uri); ok {
			m.revealAt(local, int(line), int(target.location.Range.Start.Col))
			return
		}
		if filepath.IsAbs(target.location.Path) {
			m.revealAt(target.location.Path, int(line), int(target.location.Range.Start.Col))
			return
		}
		m.status = "The definition is in an external dependency and cannot open as a file"
		m.invalidate()
		return
	}

	title := target.title
	if title == "" {
		title = quickDefinitionBase(target.uri)
	}
	doc := docbridge.Virtual(title, target.sourceText)
	if language := editor.LanguageNamed(target.language); language != nil {
		doc.Buffer().SetLanguage(language)
	}
	version := doc.Buffer().Version()
	tab := &Tab{
		doc: doc, kind: quickDefinitionSourceTab, gitOff: true,
		seen: version, synced: version, outlineVersion: version,
		state: &editor.State{
			Buffer: doc.Buffer(), ReadOnly: true,
			Font:       ui.Font{Family: oiFontMono, Size: m.prefs.fontSize()},
			LineHeight: editorLineHeight(m.prefs.fontSize()),
			Bindings:   intellijEditorBindings(),
		},
	}
	m.noteTabUse(tab)
	m.insertTab(tab)
	m.applyEditorFontPrefs(m.prefs.Font, m.prefs.fontSize())
	m.applyTabAppearance(tab, m.prefs.Editor)
	m.activate(m.tabIndex(tab))
	byteLine, col := tab.doc.BytePos(int(line), int(target.location.Range.Start.Col))
	tab.state.GoTo(editor.Pos{Line: byteLine, Col: col})
	tab.state.ScrollTo(byteLine)
	m.invalidate()
}
