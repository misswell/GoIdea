package app

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"
)

// The Problems tool window is OpenIDEA's list of what language servers and
// inspections reported: the current file or the whole project, grouped by
// file, with severity filters and a row that jumps to the problem's position.

// Problem is one row of the Problems tool window: a diagnostic's position, its
// severity and what it says.
type Problem struct {
	Path      string
	Line, Col int
	Severity  string
	Message   string
	Source    string
	Code      string
}

// ProblemsPane is the tool window's state: which scope and severities it shows.
type ProblemsPane struct {
	visible  bool
	project  bool
	showErr  bool
	showWarn bool
	showInfo bool
}

func (m *Model) resetProblems() {
	m.problems = ProblemsPane{showErr: true, showWarn: true, showInfo: true}
	// -1 is "no row visited yet", so the first Cmd+Alt+Down lands on the first.
	m.problemStep = -1
}

// ShowProblems opens the Problems tool window on the scope it last showed,
// the current file the first time, as OpenIDEA's window keeps its tab.
func (m *Model) ShowProblems() {
	m.problems.visible = true
	m.occupyBottom("problems")
	m.invalidate()
}

// ToggleProblems is Cmd+6: the window opens, and a second press from the
// editor closes it, as OpenIDEA's activateToolWindow does.
func (m *Model) ToggleProblems() {
	if m.problems.visible {
		m.CloseProblems()
		return
	}
	m.ShowProblems()
}

// CloseProblems hides the tool window.
func (m *Model) CloseProblems() {
	m.problems.visible = false
	m.invalidate()
}

// problemRows flattens the window's rows in the order it draws them.
func (m *Model) problemRows() []Problem {
	groups := m.problemList()
	rows := make([]Problem, 0, 8)
	for _, items := range groups {
		rows = append(rows, items...)
	}
	return rows
}

// nextProblem advances the step and returns the row, wrapping at both ends.
func (m *Model) nextProblem(dir int) (Problem, bool) {
	rows := m.problemRows()
	if len(rows) == 0 {
		return Problem{}, false
	}
	m.problemStep = (m.problemStep + dir + len(rows)) % len(rows)
	return rows[m.problemStep], true
}

// StepProblem is OpenIDEA's navigate.nextOccurrence / prevOccurrence for the
// Problems window: Cmd+Alt+Down and Up walk its rows and put the caret there.
func (m *Model) StepProblem(dir int) {
	if r, ok := m.nextProblem(dir); ok {
		m.revealAt(r.Path, r.Line, r.Col)
	}
}

// problemSeverity normalizes a protocol severity; an unknown one is an error,
// as the editor's mapping does.
func problemSeverity(s string) string {
	switch strings.ToLower(s) {
	case "warning":
		return "warning"
	case "info":
		return "info"
	case "hint":
		return "hint"
	}
	return "error"
}

func severityRank(s string) int {
	switch s {
	case "error":
		return 0
	case "warning":
		return 1
	case "info":
		return 2
	}
	return 3
}

// severityGroup is the filter a severity belongs to; hints are information.
func severityGroup(s string) string {
	if s == "hint" {
		return "info"
	}
	return s
}

// problemsOf returns a file's problems across producers, sorted the way
// OpenIDEA's default order is: severity, then position.
func (m *Model) problemsOf(path string) []Problem {
	per := m.diags[path]
	if len(per) == 0 {
		return nil
	}
	sources := make([]string, 0, len(per))
	for src := range per {
		sources = append(sources, src)
	}
	sort.Strings(sources)
	var out []Problem
	for _, src := range sources {
		for _, it := range per[src] {
			p := Problem{
				Path:     path,
				Line:     int(it.Range.Start.Line),
				Col:      int(it.Range.Start.Col),
				Severity: problemSeverity(it.Severity),
				Message:  it.Message,
				Source:   diagSource(src, it),
			}
			if it.Code != nil {
				p.Code = *it.Code
			}
			out = append(out, p)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if a, b := severityRank(out[i].Severity), severityRank(out[j].Severity); a != b {
			return a < b
		}
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		return out[i].Col < out[j].Col
	})
	return out
}

// problemPaths lists the files the tool window covers: every file with
// problems for the project, the active file otherwise.
func (m *Model) problemPaths() []string {
	if m.problems.project {
		paths := make([]string, 0, len(m.diags))
		for p := range m.diags {
			paths = append(paths, p)
		}
		sort.Strings(paths)
		return paths
	}
	t := m.tab()
	if t == nil {
		return nil
	}
	return []string{realPath(t.doc.Path())}
}

// problemList is what the tool window shows: the covered files' problems that
// pass the severity filters.
func (m *Model) problemList() [][]Problem {
	paths := m.problemPaths()
	out := make([][]Problem, 0, len(paths))
	for _, path := range paths {
		var items []Problem
		for _, p := range m.problemsOf(path) {
			switch severityGroup(p.Severity) {
			case "error":
				if !m.problems.showErr {
					continue
				}
			case "warning":
				if !m.problems.showWarn {
					continue
				}
			default:
				if !m.problems.showInfo {
					continue
				}
			}
			items = append(items, p)
		}
		if len(items) > 0 {
			out = append(out, items)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if a, b := hasError(out[i]), hasError(out[j]); a != b {
			return a
		}
		return out[i][0].Path < out[j][0].Path
	})
	return out
}

func hasError(items []Problem) bool {
	for _, p := range items {
		if p.Severity == "error" {
			return true
		}
	}
	return false
}

// problemCounts counts every stored problem by severity, for the tool window's
// header and the status bar.
func (m *Model) problemCounts() (errors, warnings, infos int) {
	for _, per := range m.diags {
		for _, items := range per {
			for _, it := range items {
				switch severityGroup(problemSeverity(it.Severity)) {
				case "error":
					errors++
				case "warning":
					warnings++
				default:
					infos++
				}
			}
		}
	}
	return errors, warnings, infos
}

// SmokeProblems opens the Problems tool window and reports what it would show:
// the counts, the rows the panel builds and the first message they carry.
func (m *Model) SmokeProblems() map[string]any {
	out := map[string]any{}
	run := func() {
		was, prev := m.problems.visible, m.visibleTool()
		m.ShowProblems()
		errors, warnings, infos := m.problemCounts()
		groups := m.problemList()
		rows := 0
		first := ""
		for _, items := range groups {
			rows += len(items)
			if first == "" {
				first = items[0].Message
			}
		}
		out["errors"] = errors
		out["warnings"] = warnings
		out["infos"] = infos
		out["files"] = len(groups)
		out["rows"] = rows
		out["first"] = first
		out["visible"] = m.problems.visible
		if !was {
			m.CloseProblems()
			if prev != "" && prev != "problems" {
				m.activateTool(prev)
			}
		}
	}
	m.post(run)
	return out
}

// problemsStatus is the status bar's problem counts; a click opens the tool
// window, as OpenIDEA's status bar does.
func (m *Model) problemsStatus(c *ui.Context, p *ui.Theme) {
	errors, warnings, infos := m.problemCounts()
	if errors == 0 && warnings == 0 && infos == 0 {
		return
	}
	r := ui.Row(c.Key("status:problems")).Height(18).Padding(0, 4).Gap(8).
		AlignItems(ui.Center)
	if r.Hovered() {
		r.Background(p.SurfaceHover)
	}
	r.Children(func() {
		if errors > 0 {
			ui.Text(c, fmt.Sprintf("%d 个错误", errors)).FontSize(11.5).TextColor(p.Danger)
		}
		if warnings > 0 {
			ui.Text(c, fmt.Sprintf("%d 个警告", warnings)).FontSize(11.5).TextColor(p.Warning)
		}
		if infos > 0 {
			ui.Text(c, fmt.Sprintf("%d 个信息", infos)).FontSize(11.5).TextColor(p.Accent)
		}
	})
	if r.Clicked() {
		m.ShowProblems()
	}
}

// problemsPanel draws the Problems tool window as OpenIDEA's bottom panel does:
// a header with the counts and severity filters, then the problems by file.
func (m *Model) problemsPanel(c *ui.Context, p *ui.Theme) {
	if !m.problems.visible {
		return
	}
	errors, warnings, infos := m.problemCounts()
	groups := m.problemList()
	col := ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Background(p.Surface).
		BorderWidth(1, 0, 0, 0).BorderColor(p.Border)
	col.Children(func() {
		ui.Row(c).Height(30).Padding(0, 8).Gap(10).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
			ui.Text(c, "问题").FontSize(12).Bold().TextColor(p.Text)
			m.problemFilter(c, p, "错误", errors, p.Danger, &m.problems.showErr)
			m.problemFilter(c, p, "警告", warnings, p.Warning, &m.problems.showWarn)
			m.problemFilter(c, p, "信息", infos, p.TextMuted, &m.problems.showInfo)
			ui.Spacer(c)
			m.problemScope(c, p, "当前文件", false)
			m.problemScope(c, p, "项目", true)
			ui.Button(c, "关闭").OnClick(func() { m.CloseProblems() })
		})
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(4, 0).Children(func() {
				if len(groups) == 0 {
					hint := "当前文件没有问题"
					if m.problems.project {
						hint = "项目中没有问题"
					}
					ui.Text(c, hint).FontSize(11.5).TextColor(p.TextMuted)
					return
				}
				for gi, items := range groups {
					gi, items := gi, items
					m.problemFileRow(c, p, gi, items)
				}
			})
		})
	})
}

// problemFilter is one severity count in the header; a click toggles that
// severity in the list, and the count dims while it is off.
func (m *Model) problemFilter(c *ui.Context, p *ui.Theme, label string, n int, color ui.Color, on *bool) {
	r := ui.Row(c.Key("problemfilter:"+label)).Height(22).Padding(0, 4).Gap(4).
		AlignItems(ui.Center)
	if r.Hovered() {
		r.Background(p.SurfaceHover)
	}
	text := fmt.Sprintf("%s %d", label, n)
	if !*on {
		color = p.TextMuted
	}
	r.Children(func() {
		ui.Text(c, text).FontSize(11).TextColor(color).SingleLine()
	})
	if r.Clicked() {
		*on = !*on
		m.invalidate()
	}
}

// problemScope is one of the header's File / Project buttons.
func (m *Model) problemScope(c *ui.Context, p *ui.Theme, label string, project bool) {
	active := m.problems.project == project
	r := ui.Row(c.Key("problemscope:"+label)).Height(22).Padding(0, 8).
		AlignItems(ui.Center)
	if active {
		r.Background(p.Background)
	} else if r.Hovered() {
		r.Background(p.SurfaceHover)
	}
	color := p.TextMuted
	if active {
		color = p.Text
	}
	r.Children(func() {
		ui.Text(c, label).FontSize(11.5).TextColor(color).SingleLine()
	})
	if r.Clicked() && !active {
		m.problems.project = project
		m.invalidate()
	}
}

// problemFileRow is a file's header row and its problems under it.
func (m *Model) problemFileRow(c *ui.Context, p *ui.Theme, gi int, items []Problem) {
	first := items[0]
	dir := filepath.Dir(first.Path)
	dir = strings.TrimPrefix(dir, m.Root())
	dir = strings.TrimPrefix(dir, string(filepath.Separator))
	head := ui.Row(c.Key(fmt.Sprintf("problemfile:%d", gi))).Height(22).Gap(6).
		Padding(0, 6).AlignItems(ui.Center)
	if head.Hovered() {
		head.Background(p.SurfaceHover)
	}
	head.Children(func() {
		ui.Text(c, filepath.Base(first.Path)).FontSize(12).Bold().TextColor(p.Text).SingleLine()
		if dir != "" && dir != "." {
			ui.Text(c, dir).FontSize(10.5).TextColor(p.TextMuted).SingleLine().Ellipsis("…")
		}
		ui.Spacer(c)
		ui.Text(c, fmt.Sprintf("%d", len(items))).FontSize(10.5).TextColor(p.TextMuted)
	})
	if head.Clicked() {
		m.revealAt(first.Path, first.Line, first.Col)
	}
	for i, item := range items {
		i, item := i, item
		letter, color := problemBadge(item.Severity, p)
		r := ui.Row(c.Key(fmt.Sprintf("problem:%d:%d", gi, i))).Height(22).Padding(0, 6).
			Gap(6).AlignItems(ui.Center)
		if r.Hovered() {
			r.Background(p.SurfaceHover)
		}
		r.Children(func() {
			ui.Spacer(c)
			ui.Text(c, letter).FontSize(11).Bold().TextColor(color)
			ui.Text(c, item.Message).FontSize(11.5).TextColor(p.Text).
				SingleLine().Ellipsis("…")
			if item.Code != "" {
				ui.Text(c, "("+item.Code+")").FontSize(10.5).TextColor(p.TextMuted).SingleLine()
			}
			ui.Spacer(c)
			ui.Text(c, fmt.Sprintf("行 %d", item.Line+1)).FontSize(10.5).
				TextColor(p.TextMuted).SingleLine()
		})
		if r.Clicked() {
			m.revealAt(item.Path, item.Line, item.Col)
		}
	}
}

// problemBadge is the letter and colour of a problem's severity.
func problemBadge(severity string, p *ui.Theme) (string, ui.Color) {
	switch severity {
	case "error":
		return "E", p.Danger
	case "warning":
		return "W", p.Warning
	case "info":
		return "I", p.Accent
	}
	return "H", p.TextMuted
}
