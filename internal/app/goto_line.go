package app

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

// gotoLineState keeps the editor and position present when the dialog opened.
// Editing its input never changes the editor until the user submits it.
type gotoLineState struct {
	open    bool
	query   string
	tab     *Tab
	current string
	input   ui.Handle
}

var gotoLinePattern = regexp.MustCompile(`^\s*([0-9]+)\s*(?:[:,]\s*([0-9]+))?\s*$`)

// parseGotoLine accepts OpenIDEA's one-based line[:column] and line,column.
// Huge positive values saturate so the editor can clamp them to the document.
func parseGotoLine(text string) (line, col int, ok bool) {
	match := gotoLinePattern.FindStringSubmatch(text)
	if match == nil {
		return 0, 0, false
	}
	parse := func(s string) (int, bool) {
		value, err := strconv.ParseUint(s, 10, 32)
		if err != nil {
			if n, e := strconv.ParseUint(s, 10, 64); e == nil && n == 0 {
				return 0, false
			}
			value = uint64(^uint32(0))
		}
		if value == 0 {
			return 0, false
		}
		return int(value - 1), true
	}
	line, ok = parse(match[1])
	if !ok {
		return 0, 0, false
	}
	if match[2] == "" {
		return line, 0, true
	}
	col, ok = parse(match[2])
	return line, col, ok
}

func (m *Model) OpenGotoLine() {
	if m.gotoLine != nil {
		return
	}
	v := &gotoLineState{open: true}
	if t := m.tab(); t != nil && t.kind == "" && t.doc != nil && t.state != nil {
		pos := t.doc.UTF16Pos(t.state.Cursor())
		v.tab = t
		v.current = fmt.Sprintf("%d:%d", uint64(pos.Line)+1, uint64(pos.Col)+1)
		v.query = v.current
	}
	v.input.SelectAll()
	m.gotoLine = v
	m.invalidate()
}

func (m *Model) closeGotoLine() {
	if m.gotoLine == nil {
		return
	}
	m.gotoLine = nil
	m.invalidate()
}

func (m *Model) submitGotoLine() {
	v := m.gotoLine
	if v == nil || v.tab == nil || !m.tabOpen(v.tab) || m.tab() != v.tab {
		return
	}
	line, col, ok := parseGotoLine(v.query)
	if !ok {
		return
	}
	line, col = v.tab.doc.BytePos(line, col)
	m.recordNavigation(true)
	v.tab.state.GoTo(editor.Pos{Line: line, Col: col})
	v.tab.state.ScrollTo(line)
	m.recordNavigation(true)
	m.closeGotoLine()
}

func (m *Model) gotoLineDialog(c *ui.Context, p *ui.Theme) {
	v := m.gotoLine
	if v == nil {
		return
	}
	open := v.open
	d := ui.Modal(c, &open, func() {
		ui.Column(c).Width(340).Gap(8).Children(func() {
			ui.Text(c, "Go to Line:Column").FontSize(13).Bold().TextColor(p.Text).SingleLine()
			in := ui.TextInput(c, &v.query).Bind(&v.input).Font(oiFontMono).Placeholder("line[:column]")
			if in.Submitted() {
				m.submitGotoLine()
			}
			hint := "No active editor"
			if v.tab != nil {
				hint = "Current position " + v.current
			}
			ui.Text(c, hint).FontSize(12).TextColor(p.TextMuted).SingleLine()
			_, _, valid := parseGotoLine(v.query)
			ui.Row(c).Gap(8).Children(func() {
				ui.Button(c, "Cancel").OnClick(m.closeGotoLine)
				ui.Button(c, "OK").Disabled(!valid || v.tab == nil).OnClick(m.submitGotoLine)
			})
		})
	})
	if d.Dismissed() || !open {
		m.closeGotoLine()
	}
}
