package app

// What's New, after OpenIDEA's features/whats-new: a page with the release
// notes of one version, opened from Help at any time and once on the first
// start of a new version. The page is a tab in the editor area when a project
// is open and owns the window on its own while the Welcome screen would
// otherwise show, since upstream's tab is standalone.

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
)

const (
	// tabKindWhatsNew marks the page's tab: its body is the page, not the
	// document's text, which stays empty.
	tabKindWhatsNew = "whatsNew"
	// whatsNewReleases is where the page links a version without notes.
	whatsNewReleases = "https://github.com/misswell/GoIdea/releases"
	// whatsNewColumn is the reading column's width, as upstream's 820px.
	whatsNewColumn = 820
)

func (m *Model) whatsNewTab() *Tab {
	for _, t := range m.tabs {
		if t.kind == tabKindWhatsNew {
			return t
		}
	}
	return nil
}

// whatsNewShown reports whether the page is the tab on show.
func (m *Model) whatsNewShown() bool {
	t := m.tab()
	return t != nil && t.kind == tabKindWhatsNew
}

// openWhatsNewPage shows version's page, reusing its tab when the version's
// page is already open.
func (m *Model) openWhatsNewPage(version, notes string) {
	if t := m.whatsNewTab(); t != nil {
		t.pageVersion = version
		t.pageNotes = notes
		t.pageLoading = false
		for i, x := range m.tabs {
			if x == t {
				m.activate(i)
				break
			}
		}
		m.showingWelcome = false
		m.applyActive()
		m.invalidate()
		return
	}
	doc := docbridge.Virtual("What's New", "")
	t := &Tab{
		doc:         doc,
		state:       &editor.State{Buffer: doc.Buffer(), LineHeight: 1.65, Bindings: intellijEditorBindings(), ReadOnly: true},
		kind:        tabKindWhatsNew,
		pageVersion: version,
		pageNotes:   notes,
	}
	t.state.Font = ui.Font{Family: oiFontUI, Size: m.prefs.fontSize()}
	t.synced = doc.Buffer().Version()
	t.seen = t.synced
	m.noteTabUse(t)
	if m.split != nil {
		m.focusGroup(groupFocused)
	}
	m.insertTab(t)
	m.active = m.tabIndex(t)
	m.showingWelcome = false
	m.revealEditor()
	m.applyActive()
	m.invalidate()
}

// CloseWhatsNew closes the page, which the standalone page's close button
// does; with a project the tab strip closes it the usual way.
func (m *Model) CloseWhatsNew() {
	if t := m.whatsNewTab(); t != nil {
		m.removeTab(t)
	}
}

// whatsNewPage draws What's New in the editor area.
func (m *Model) whatsNewPage(c *ui.Context, p *ui.Theme, t *Tab) {
	ui.Column(c.Key("whatsNew:page")).Grow(1).MinHeight(0).MinWidth(0).
		Background(p.Background).Children(func() {
		ui.Row(c).Shrink(0).Justify(ui.Center).BorderWidth(0, 0, 1, 0).
			BorderColor(p.Border).Children(func() {
			ui.Column(c).Width(whatsNewColumn).MaxWidthPercent(100).
				Padding(22, 32, 14, 32).Gap(3).Children(func() {
				ui.Text(c, "What's New").FontSize(20).Bold().TextColor(p.Text).SingleLine()
				ui.Text(c, "GoIDEA "+t.pageVersion).FontSize(12).
					TextColor(p.TextMuted).SingleLine()
			})
		})
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			ui.Row(c).Justify(ui.Center).Children(func() {
				ui.Column(c).Width(whatsNewColumn).MaxWidthPercent(100).
					Padding(20, 32, 32, 32).Children(func() {
					m.whatsNewNotes(c, p, t)
				})
			})
		})
		ui.Row(c).Shrink(0).Justify(ui.Center).BorderWidth(1, 0, 0, 0).
			BorderColor(p.Border).Padding(10, 0).Children(func() {
			ui.Column(c).Width(whatsNewColumn).MaxWidthPercent(100).
				Padding(0, 32).Children(func() {
				m.whatsNewFooter(c)
			})
		})
	})
}

// whatsNewStandalone draws the page without a project: a slim bar names the
// tab and closes it, since there is no tab strip to close it otherwise.
func (m *Model) whatsNewStandalone(c *ui.Context, p *ui.Theme) {
	t := m.tab()
	if t == nil || t.kind != tabKindWhatsNew {
		return
	}
	ui.Column(c).Fill().Background(p.Background).Children(func() {
		ui.Row(c).Height(oiTabBarHeight).Shrink(0).Padding(0, 6).
			AlignItems(ui.Center).Gap(6).Background(p.Surface).
			BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
			ui.Text(c, "What's New").FontSize(13).TextColor(p.Text).SingleLine()
			ui.Spacer(c)
			close := ui.ButtonBase(c.Key("whatsNew:close")).Size(22, 22).Radius(4).
				Justify(ui.Center).AlignItems(ui.Center).Cursor(ui.CursorPointer).
				Tooltip("Close")
			if close.Hovered() {
				close.Background(p.SurfaceHover)
			}
			close.Children(func() {
				oiIconEl(c, "general/close", 16, oiPaletteOf(p), p.TextMuted)
			})
			close.OnClick(func() { m.CloseWhatsNew() })
		})
		m.whatsNewPage(c, p, t)
	})
}

// whatsNewNotes draws the notes, with OpenIDEA's reading styles for the
// markdown the release bodies carry.
func (m *Model) whatsNewNotes(c *ui.Context, p *ui.Theme, t *Tab) {
	notes := strings.TrimSpace(t.pageNotes)
	if notes == "" {
		if t.pageLoading {
			ui.Text(c, "Fetching release notes…").FontSize(13).
				TextColor(p.TextMuted).SingleLine()
			return
		}
		ui.Text(c, "There are no release notes for this version.").FontSize(13).
			TextColor(p.TextMuted).SingleLine()
		link := ui.ButtonBase(c.Key("whatsNew:releases")).Padding(0, 0).
			Cursor(ui.CursorPointer)
		link.Children(func() {
			ui.Text(c, "Open Release Page").FontSize(13).TextColor(p.Accent).SingleLine()
		})
		if link.Clicked() {
			url := whatsNewReleases
			if t.pageVersion != "" {
				url += "/tag/v" + t.pageVersion
			}
			openReleasePage(url)
		}
		return
	}
	for _, block := range markdownBlocks(notes) {
		block := block
		switch block.kind {
		case "h1", "h2", "h3":
			size := float32(13)
			if block.kind == "h1" {
				size = 18
			} else if block.kind == "h2" {
				size = 15
			}
			ui.Text(c, block.text).FontSize(size).
				Bold().TextColor(p.Text).SingleLine().Ellipsis("…").Padding(12, 0, 4, 0)
		case "rule":
			ui.Row(c).Height(1).Shrink(0).Background(p.Border).Padding(9, 0, 9, 0)
		case "item":
			ui.Row(c).Gap(6).Padding(2, 0).Children(func() {
				ui.Text(c, "•").FontSize(13).TextColor(p.TextMuted).SingleLine()
				ui.Text(c, block.text).FontSize(13).TextColor(p.Text).Grow(1)
			})
		case "quote":
			ui.Row(c).Padding(2, 0).Gap(10).Children(func() {
				ui.Row(c).Width(2).Background(p.Border)
				ui.Text(c, block.text).FontSize(13).TextColor(p.TextMuted).Grow(1)
			})
		case "code":
			ui.Column(c).Padding(8, 10).Radius(4).Background(p.SurfaceHover).
				Children(func() {
					ui.Text(c, block.text).FontSize(12).TextColor(p.Text).SingleLine().Ellipsis("…")
				})
		default:
			ui.Text(c, block.text).FontSize(13).TextColor(p.Text).Padding(0, 0, 6, 0)
		}
	}
}

// whatsNewFooter is the page's one setting, shared with Settings › Updates.
func (m *Model) whatsNewFooter(c *ui.Context) {
	on := m.showWhatsNewAfterUpdate()
	if ui.Checkbox(c, &on, "Show this page after every update").Changed() {
		m.SetShowWhatsNewAfterUpdate(on)
		m.invalidate()
	}
}

// markdownBlock is one paragraph of the notes after the several markdown
// shapes the release bodies use; inline emphasis is dropped since the text
// widgets carry no runs.
type markdownBlock struct {
	kind string
	text string
}

// markdownBlocks groups release notes into headings, items, quotes, code and
// paragraphs, dropping the markdown that has no plain-text reading.
func markdownBlocks(notes string) []markdownBlock {
	var out []markdownBlock
	var para []string
	var code []string
	inCode := false
	flush := func() {
		if len(para) > 0 {
			out = append(out, markdownBlock{kind: "p", text: strings.Join(para, " ")})
			para = nil
		}
	}
	for _, raw := range strings.Split(notes, "\n") {
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			if inCode {
				out = append(out, markdownBlock{kind: "code", text: strings.Join(code, "\n")})
				code = nil
				inCode = false
			} else {
				flush()
				inCode = true
			}
			continue
		}
		if inCode {
			code = append(code, line)
			continue
		}
		if trimmed == "" {
			flush()
			continue
		}
		if level, text, ok := markdownHeading(trimmed); ok {
			flush()
			out = append(out, markdownBlock{kind: level, text: inlineText(text)})
			continue
		}
		switch {
		case trimmed == "---" || trimmed == "***" || trimmed == "___":
			flush()
			out = append(out, markdownBlock{kind: "rule"})
		case strings.HasPrefix(trimmed, "> "):
			flush()
			out = append(out, markdownBlock{kind: "quote", text: inlineText(strings.TrimPrefix(trimmed, "> "))})
		case strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") ||
			strings.HasPrefix(trimmed, "+ "):
			flush()
			out = append(out, markdownBlock{kind: "item", text: inlineText(trimmed[2:])})
		default:
			para = append(para, inlineText(trimmed))
		}
	}
	if inCode && len(code) > 0 {
		out = append(out, markdownBlock{kind: "code", text: strings.Join(code, "\n")})
	}
	flush()
	return out
}

// markdownHeading recognises "#" to "######" headings.
func markdownHeading(line string) (kind, text string, ok bool) {
	level := 0
	for level < len(line) && line[level] == '#' {
		level++
	}
	if level == 0 || level > 6 || level >= len(line) || line[level] != ' ' {
		return "", "", false
	}
	switch {
	case level == 1:
		kind = "h1"
	case level == 2:
		kind = "h2"
	default:
		kind = "h3"
	}
	return kind, strings.TrimSpace(line[level+1:]), true
}

// inlineText drops emphasis markers, inline code ticks and link targets,
// leaving the words a plain text widget shows.
func inlineText(s string) string {
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "__", "")
	s = strings.ReplaceAll(s, "`", "")
	s = strings.ReplaceAll(s, "~~", "")
	for {
		open := strings.Index(s, "[")
		if open < 0 {
			break
		}
		mid := strings.Index(s[open:], "](")
		end := strings.Index(s[open:], ")")
		if mid < 0 || end < 0 || mid > end {
			break
		}
		label := s[open+1 : open+mid]
		s = s[:open] + label + s[open+end+1:]
	}
	return strings.TrimSpace(s)
}
