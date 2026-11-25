package app

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/highlight"
)

const recentLocationRowHeight float32 = 140

func (m *Model) recentNavigationShortcuts(c *ui.Context) {
	if c.Shortcut(ui.Cmd, ui.KeyE) {
		m.RunCommand("navigate.recentFiles")
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyE) {
		m.RunCommand("navigate.recentLocations")
	}
}

func (m *Model) moveRecentNavigation(step int, page, extend bool) {
	p := m.recentNav.popup
	if p == nil {
		return
	}
	if p.switcher && !page {
		m.moveSwitcherNavigation(step)
		return
	}
	count := p.count()
	if count == 0 {
		return
	}
	delta := step
	if page {
		delta = 10
		if step < 0 {
			delta = -10
		}
		if p.locations {
			delta = 4
			if step < 0 {
				delta = -4
			}
		}
	}
	next := clampSymbolSel(p.index+delta, count)
	if !page {
		wrap := p.locations || p.onTools || p.switcher || !extend
		if delta < 0 && p.index+delta < 0 && wrap {
			next = count - 1
		}
		if delta > 0 && p.index+delta >= count && wrap {
			next = 0
		}
	}
	if next == p.index {
		return
	}
	if p.locations || p.onTools {
		p.index = next
	} else {
		mode := recentSelectionSingle
		if extend {
			mode = recentSelectionRange
		}
		p.selectRecentFile(next, mode)
	}
	if !p.onTools {
		height := float32(24)
		if p.locations {
			height = recentLocationRowHeight
		}
		top := float32(p.index) * height
		p.scroll.Y = min(max(p.scroll.Y, top+height-336), top)
	}
	m.invalidate()
}

// moveSwitcherNavigation follows OpenIDEA's switcherSelection: Ctrl+Tab walks
// files first and then tool windows, wrapping across both columns.
func (m *Model) moveSwitcherNavigation(step int) {
	p := m.recentNav.popup
	if p == nil || step == 0 {
		return
	}
	files, tools := len(p.switcherItems), len(p.tools)
	total := files + tools
	if total == 0 {
		return
	}
	at := p.index
	if p.onTools {
		at += files
	}
	at = ((at+step)%total + total) % total
	if at < files {
		p.onTools, p.index = false, at
		height := float32(24)
		top := float32(p.index) * height
		p.scroll.Y = min(max(p.scroll.Y, top+height-336), top)
	} else {
		p.onTools, p.index = true, at-files
	}
	m.invalidate()
}

func (m *Model) recentNavigationDialog(c *ui.Context, theme *ui.Theme) {
	p := m.recentNav.popup
	if p == nil {
		return
	}
	releaseSwitcher := p.switcher && p.switcherSaw && c.Modifiers()&ui.Ctrl == 0
	if p.switcher && c.Modifiers()&ui.Ctrl != 0 {
		p.switcherSaw = true
	}
	oi := oiPaletteOf(theme)
	title, filter, toggle := "Recent Files", "Filter recent files", "Show edited only"
	if p.switcher {
		title, filter, toggle = "Switcher", "", ""
	} else if p.locations {
		title, filter, toggle = "Recent Locations", "Type to filter by file name or code", "Show changed only"
		if p.onlyChanged {
			title = "Recently Changed Locations"
		}
	} else if p.onlyChanged {
		title = "Recently Edited Files"
	}
	open := true
	dialog := nativeModal(c.Key("recent-navigation"), &open, func() {
		body := ui.Column(c).Width(720)
		if p.switcher {
			body = body.Focusable().AutoFocus().Label("Switcher")
			body = m.bindRecentNavigationKeys(body, p).
				OnShortcut(0, ui.KeyEnter, func() { m.submitRecentNavigation(false) }).
				OnShortcut(ui.Shift, ui.KeyEnter, func() { m.submitRecentNavigation(true) })
		}
		body.Children(func() {
			ui.Row(c).Padding(8, 12).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Text(c, title).FontSize(13).Bold().SingleLine()
				if p.switcher {
					ui.Spacer(c)
				} else {
					in := ui.TextInput(c, &p.query).Grow(1).AutoFocus().Placeholder(filter)
					in = m.bindRecentNavigationKeys(in, p)
					in.OnShortcut(ui.Cmd, ui.KeyE, func() { m.RunCommand("navigate.recentFiles") })
					in.OnShortcut(ui.Cmd|ui.Shift, ui.KeyE, func() { m.RunCommand("navigate.recentLocations") })
					in.OnChange(func() { m.refreshRecentNavigation(true) })
					if in.Submitted() {
						m.submitRecentNavigation(in.SubmittedMods()&ui.Shift != 0)
					}
				}
				if toggle != "" && ui.Checkbox(c, &p.onlyChanged, toggle).Changed() {
					m.refreshRecentNavigation(true)
				}
			})
			if p.locations {
				m.recentLocationRows(c, p, oi)
			} else {
				m.recentFileRows(c, p, oi)
			}
		})
	})
	dialog.Padding(0)
	if !open {
		m.closeRecentNavigation()
		return
	}
	if releaseSwitcher {
		m.chooseRecentNavigation(false)
	}
}

func (m *Model) bindRecentNavigationKeys(e ui.Element, p *recentNavPopup) ui.Element {
	e = e.
		OnShortcut(0, ui.KeyUp, func() { m.moveRecentNavigation(-1, false, false) }).
		OnShortcut(ui.Shift, ui.KeyUp, func() { m.moveRecentNavigation(-1, false, true) }).
		OnShortcut(0, ui.KeyDown, func() { m.moveRecentNavigation(1, false, false) }).
		OnShortcut(ui.Shift, ui.KeyDown, func() { m.moveRecentNavigation(1, false, true) }).
		OnShortcut(0, ui.KeyPageUp, func() { m.moveRecentNavigation(-1, true, false) }).
		OnShortcut(ui.Shift, ui.KeyPageUp, func() { m.moveRecentNavigation(-1, true, true) }).
		OnShortcut(0, ui.KeyPageDown, func() { m.moveRecentNavigation(1, true, false) }).
		OnShortcut(ui.Shift, ui.KeyPageDown, func() { m.moveRecentNavigation(1, true, true) })
	if p.locations {
		e = e.OnShortcut(0, ui.KeyDelete, m.removeRecentPlace)
		if p.query == "" {
			e = e.OnShortcut(0, ui.KeyBackspace, m.removeRecentPlace)
		}
		return e
	}
	e = e.OnShortcut(0, ui.KeyDelete, m.removeSelectedRecentNavigation)
	if p.query == "" {
		e = e.OnShortcut(0, ui.KeyBackspace, m.removeSelectedRecentNavigation)
	}
	e = e.OnShortcut(0, ui.KeyTab, func() {
		if (p.onTools && len(p.files) > 0) || (!p.onTools && len(p.tools) > 0) {
			p.onTools, p.index = !p.onTools, 0
			p.scroll.Y = 0
			p.fileSelection = recentFileSelection{anchor: 0}
			m.invalidate()
		}
	})
	e = e.OnShortcut(0, ui.KeyLeft, func() {
		if p.query == "" && len(p.tools) > 0 {
			p.onTools, p.index = true, 0
			p.fileSelection = recentFileSelection{anchor: 0}
			m.invalidate()
		}
	})
	return e.OnShortcut(0, ui.KeyRight, func() {
		if p.query == "" && len(p.files) > 0 {
			p.onTools, p.index = false, 0
			p.fileSelection = recentFileSelection{anchor: 0}
			m.invalidate()
		}
	})
}

func (m *Model) submitRecentNavigation(side bool) {
	p := m.recentNav.popup
	if p == nil {
		return
	}
	if !p.locations && !p.switcher && !p.onTools && len(p.files) == 0 && strings.TrimSpace(p.query) != "" {
		query := strings.TrimSpace(p.query)
		m.closeRecentNavigation()
		m.openSearchEverywhereAt(seTabFiles)
		if m.se != nil {
			m.se.query = query
			m.se.sel = 0
			m.se.extra = map[seSection]int{}
			m.refreshSearchEverywhere()
		}
		return
	}
	m.chooseRecentNavigation(side)
}

func recentNavigationRow(c *ui.Context, id string, height float32, oi oiColors, selected bool) ui.Element {
	row := ui.Row(c.Key(id)).Height(height).Padding(0, 10).Gap(8).AlignItems(ui.Center)
	if selected {
		row.Background(oi.selected)
	} else if row.Hovered() {
		row.Background(oi.hover)
	}
	return row
}

func (m *Model) recentFileRows(c *ui.Context, p *recentNavPopup, oi oiColors) {
	if p.switcher {
		m.recentSwitcherRows(c, p, oi)
		return
	}
	ui.Row(c).Height(336).AlignItems(ui.Stretch).Children(func() {
		ui.Column(c).Width(196).BorderWidth(0, 1, 0, 0).BorderColor(oi.border).Children(func() {
			ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
				for i, spec := range p.tools {
					row := recentNavigationRow(c, spec.id, 24, oi, p.onTools && p.index == i)
					row.OnClick(func() {
						p.onTools, p.index = true, i
						p.fileSelection = recentFileSelection{anchor: 0}
						m.chooseRecentNavigation(false)
					})
					row.Children(func() {
						oiIconEl(c, spec.icon, 14, oi, oi.textMuted)
						ui.Text(c, spec.title).FontSize(13).SingleLine()
					})
				}
			})
			ui.Button(c, "Recent Locations").OnClick(func() { m.openRecentNavigation(true) })
		})
		ui.Scroll(c).TrackScroll(&p.scroll).Grow(1).MinWidth(0).Children(func() {
			if len(p.files) == 0 {
				label := "No recent files"
				switch {
				case p.loading:
					label = "Loading recent files…"
				case p.onlyChanged:
					label = "No edited files"
				case p.query != "":
					label = "No matching files"
				}
				ui.Text(c, label).Padding(10).FontSize(13).TextColor(oi.textMuted)
			}
			for i, path := range p.files {
				row := recentNavigationRow(c, path, 24, oi, !p.onTools && p.isRecentFileSelected(i))
				mods := row.ClickModifiers()
				row.OnClick(func() {
					p.onTools = false
					mode := recentSelectionSingle
					switch {
					case mods&ui.Shift != 0:
						mode = recentSelectionRange
					case mods&ui.Cmd != 0:
						mode = recentSelectionToggle
					}
					p.selectRecentFile(i, mode)
					if mode == recentSelectionSingle {
						m.submitRecentNavigation(false)
					} else {
						m.invalidate()
					}
				})
				row.Children(func() {
					name := filepath.Base(path)
					fileIconEl(c, oi, name, 14, oi.textMuted)
					ui.Text(c, name).FontSize(13).SingleLine()
					if dir := filepath.Dir(projectRelative(m.Root(), path)); dir != "." {
						ui.Text(c, dir).FontSize(12).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
					}
				})
			}
		})
	})
	footer := m.Root()
	if !p.onTools && p.index < len(p.files) {
		footer = p.files[p.index]
	}
	if p.err != "" {
		footer = p.err
	}
	ui.Text(c, footer).Padding(6, 12).FontSize(12).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
}

func (m *Model) recentSwitcherRows(c *ui.Context, p *recentNavPopup, oi oiColors) {
	ui.Row(c).Height(336).AlignItems(ui.Stretch).Children(func() {
		ui.Column(c).Width(196).BorderWidth(0, 1, 0, 0).BorderColor(oi.border).Children(func() {
			ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
				for i, spec := range p.tools {
					row := recentNavigationRow(c, "switcher-tool:"+spec.id, 24, oi, p.onTools && p.index == i)
					row.OnClick(func() {
						p.onTools, p.index = true, i
						m.chooseRecentNavigation(false)
					})
					row.Children(func() {
						oiIconEl(c, spec.icon, 14, oi, oi.textMuted)
						ui.Text(c, spec.title).FontSize(13).SingleLine()
					})
				}
			})
		})
		ui.Scroll(c).TrackScroll(&p.scroll).Grow(1).MinWidth(0).Children(func() {
			if len(p.switcherItems) == 0 {
				ui.Text(c, "No open files").Padding(10).FontSize(13).TextColor(oi.textMuted)
			}
			for i, item := range p.switcherItems {
				row := recentNavigationRow(c, "switcher-file:"+item.path, 24, oi, !p.onTools && p.index == i)
				row.OnClick(func() {
					p.onTools, p.index = false, i
					m.chooseRecentNavigation(false)
				})
				row.Children(func() {
					if item.nonFile {
						oiIconEl(c, "toolwindows/agents", 14, oi, oi.textMuted)
						ui.Text(c, item.title).FontSize(13).SingleLine()
						return
					}
					name := filepath.Base(item.path)
					fileIconEl(c, oi, name, 14, oi.textMuted)
					ui.Text(c, name).FontSize(13).SingleLine()
					if dir := filepath.Dir(projectRelative(m.Root(), item.path)); dir != "." {
						ui.Text(c, dir).FontSize(12).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
					}
				})
			}
		})
	})
	footer := m.Root()
	if !p.onTools && p.index >= 0 && p.index < len(p.switcherItems) {
		footer = p.switcherItems[p.index].path
	}
	if p.err != "" {
		footer = p.err
	}
	ui.Text(c, footer).Padding(6, 12).FontSize(12).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
}

func (m *Model) recentLocationRows(c *ui.Context, p *recentNavPopup, oi oiColors) {
	ui.Scroll(c).TrackScroll(&p.scroll).Height(336).Children(func() {
		if len(p.places) == 0 {
			text := "No recent locations"
			if p.onlyChanged {
				text = "No changed locations"
			}
			ui.Text(c, text).Padding(10).FontSize(13).TextColor(oi.textMuted)
		}
		for i, place := range p.places {
			key := fmt.Sprintf("%s:%d", place.path, place.pos.Line)
			row := recentNavigationRow(c, key, recentLocationRowHeight, oi, p.index == i)
			row.OnClick(func() {
				p.index = i
				m.invalidate()
			})
			if row.DoubleClicked() {
				p.index = i
				m.chooseRecentNavigation(row.ClickModifiers()&ui.Shift != 0)
			}
			row.Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
					ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
						name := filepath.Base(place.path)
						fileIconEl(c, oi, name, 14, oi.textMuted)
						ui.Text(c, name).FontSize(13).SingleLine()
						where := m.recentLocationBreadcrumb(place.path, place.pos.Line)
						if where == "" {
							where = fmt.Sprintf("%s:%d", projectRelative(m.Root(), place.path), place.pos.Line+1)
						}
						ui.Text(c, where).Grow(1).FontSize(12).TextColor(oi.textMuted).
							SingleLine()
						if place.changed {
							ui.Text(c, "changed").FontSize(12).TextColor(oi.textMuted)
						}
						ui.Text(c, recentPlaceTime(place.at, p.now)).FontSize(12).TextColor(oi.textMuted)
					})
					lines := recentDedent(place.lines)
					tokens := m.recentSnippetLines(p, place.path, lines)
					codeTheme := editor.DefaultTheme(c.Theme())
					spans := make([]ui.Span, 0, len(lines)*5)
					for j, text := range lines {
						line := place.firstLine + j
						var background ui.Color
						if line == place.pos.Line {
							background = oi.hoverTint
						}
						spans = append(spans, ui.Span{
							Text: fmt.Sprintf("%4d  ", line+1), Color: oi.textFaint, Background: background,
						})
						var lineTokens []highlight.Token
						if j < len(tokens) {
							lineTokens = tokens[j].Tokens
						}
						lineSpans := recentSnippetSpans(text, lineTokens, codeTheme, oi.textMuted)
						for i := range lineSpans {
							lineSpans[i].Background = background
						}
						spans = append(spans, lineSpans...)
						if j+1 < len(lines) {
							spans = append(spans, ui.Span{Text: "\n", Background: background})
						}
					}
					ui.RichText(c, spans...).Grow(1).MinWidth(0).
						Font(oiFontMono).FontSize(12)
				})
			})
		}
	})
}

func (m *Model) recentSnippetLines(p *recentNavPopup, path string, lines []string) []highlight.Line {
	if p.snippets == nil {
		p.snippets = make(map[string][]highlight.Line)
	}
	text := strings.Join(lines, "\n")
	key := realPath(path) + "\x00" + text
	if cached, ok := p.snippets[key]; ok {
		return cached
	}
	lexed := highlight.Lex(highlight.Lexer(path), text)
	if len(p.snippets) >= 200 {
		clear(p.snippets)
	}
	p.snippets[key] = lexed
	return lexed
}

func recentDedent(lines []string) []string {
	minimum := -1
	for _, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed == "" {
			continue
		}
		indent := len(line) - len(trimmed)
		if minimum < 0 || indent < minimum {
			minimum = indent
		}
	}
	if minimum <= 0 {
		return lines
	}
	out := make([]string, len(lines))
	for i, line := range lines {
		indent := min(minimum, len(line))
		out[i] = line[indent:]
	}
	return out
}

func recentSnippetSpans(
	text string,
	tokens []highlight.Token,
	theme *editor.Theme,
	plain ui.Color,
) []ui.Span {
	runes := []rune(strings.TrimRight(text, "\r\n"))
	if len(runes) == 0 {
		return []ui.Span{{Text: " ", Color: plain}}
	}
	spans := make([]ui.Span, 0, len(tokens)*2+1)
	cursor := 0
	for _, token := range tokens {
		start := max(0, min(token.Start, len(runes)))
		end := max(start, min(token.End, len(runes)))
		if start > cursor {
			spans = append(spans, ui.Span{Text: string(runes[cursor:start]), Color: plain})
		}
		if end > start {
			spans = append(spans, ui.Span{
				Text:  string(runes[start:end]),
				Color: theme.TokenColor(recentSnippetTokenKind(token.Kind)),
			})
		}
		cursor = max(cursor, end)
	}
	if cursor < len(runes) {
		spans = append(spans, ui.Span{Text: string(runes[cursor:]), Color: plain})
	}
	if len(spans) == 0 {
		spans = append(spans, ui.Span{Text: string(runes), Color: plain})
	}
	return spans
}

func recentSnippetTokenKind(kind highlight.Kind) editor.TokenKind {
	switch kind {
	case highlight.Keyword:
		return editor.TokenKeyword
	case highlight.Type:
		return editor.TokenType
	case highlight.Function, highlight.Builtin:
		return editor.TokenFunction
	case highlight.String:
		return editor.TokenString
	case highlight.Number:
		return editor.TokenNumber
	case highlight.Comment:
		return editor.TokenComment
	case highlight.Operator:
		return editor.TokenOperator
	case highlight.Punctuation:
		return editor.TokenPunctuation
	case highlight.Constant:
		return editor.TokenConstant
	case highlight.Tag:
		return editor.TokenTag
	case highlight.Attribute:
		return editor.TokenAttribute
	case highlight.Variable:
		return editor.TokenVariable
	case highlight.Heading:
		return editor.TokenHeading
	case highlight.Deleted:
		return editor.TokenDeleted
	case highlight.Inserted:
		return editor.TokenInserted
	case highlight.Strong:
		return editor.TokenStrong
	case highlight.Emph:
		return editor.TokenEmphasis
	case highlight.Mark:
		return editor.TokenMeta
	default:
		return editor.TokenText
	}
}

func recentPlaceTime(at, now time.Time) string {
	age := now.Sub(at)
	switch {
	case age < time.Minute:
		return "Just now"
	case age < time.Hour:
		minutes := int(age.Minutes())
		if minutes == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", minutes)
	default:
		return at.Format("Jan 2 15:04")
	}
}
