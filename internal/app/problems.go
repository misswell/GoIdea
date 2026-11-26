package app

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

// The Problems tool window is OpenIDEA's list of what language servers and
// inspections reported: the current file or the whole project, grouped by
// file, with severity filters and a row that jumps to the problem's position.

// Problem is one row of the Problems tool window: a diagnostic's position, its
// severity and what it says.
type Problem struct {
	Path                       string
	Line, Col, EndLine, EndCol int
	Severity                   string
	Message                    string
	Source                     string
	Code                       string
}

// ProblemsPane is the tool window's state: which scope and severities it shows.
type ProblemsPane struct {
	visible   bool
	project   bool
	showErr   bool
	showWarn  bool
	showInfo  bool
	collapsed map[string]bool
	selected  string
	preview   problemPreview
}

type problemPreview struct {
	path              string
	line, col, endCol int
	start             int
	lines             []string
	err               string
}

func (m *Model) resetProblems() {
	m.problems = ProblemsPane{
		showErr: true, showWarn: true, showInfo: true,
		collapsed: map[string]bool{},
	}
	// -1 is "no row visited yet", so the first Cmd+Alt+Down lands on the first.
	m.problemStep = -1
}

// ShowProblems opens the Problems tool window on the scope it last showed,
// the current file the first time, as OpenIDEA's window keeps its tab.
func (m *Model) ShowProblems() { m.DockShow("problems") }

// ToggleProblems is Cmd+6: the window opens, and a second press from the
// editor closes it, as OpenIDEA's activateToolWindow does.
func (m *Model) ToggleProblems() { m.DockToggle("problems") }

// CloseProblems hides the tool window.
func (m *Model) CloseProblems() { m.DockHide("problems") }

// problemRows flattens the window's rows in the order it draws them.
func (m *Model) problemRows() []Problem {
	groups := m.problemList()
	rows := make([]Problem, 0, 8)
	for _, items := range groups {
		if len(items) > 0 && m.problems.collapsed[items[0].Path] {
			continue
		}
		rows = append(rows, items...)
	}
	return rows
}

func (m *Model) navigationProblemRows() []Problem {
	rows := m.problemRows()
	if !m.nativeRuntimeBool(
		settingsPageCodeEditing,
		"editor.codeEditing.nextErrorErrorsOnly",
	) {
		return rows
	}
	filtered := make([]Problem, 0, len(rows))
	for _, problem := range rows {
		if problem.Severity == "error" || problem.Severity == "warning" {
			filtered = append(filtered, problem)
		}
	}
	return filtered
}

// nextProblem advances the step and returns the row, wrapping at both ends.
func (m *Model) nextProblem(dir int) (Problem, bool) {
	rows := m.navigationProblemRows()
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
		m.selectProblem(r)
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
				EndLine:  int(it.Range.End.Line),
				EndCol:   int(it.Range.End.Col),
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
	m.sortProblems(out)
	return out
}

func (m *Model) sortProblems(items []Problem) {
	prefs := m.prefs.Problems
	position := func(a, b Problem) int {
		if a.Line != b.Line {
			if a.Line < b.Line {
				return -1
			}
			return 1
		}
		if a.Col < b.Col {
			return -1
		}
		if a.Col > b.Col {
			return 1
		}
		return 0
	}
	name := func(a, b Problem) int {
		left, right := strings.ToLower(a.Message), strings.ToLower(b.Message)
		if left < right {
			return -1
		}
		if left > right {
			return 1
		}
		return 0
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if prefs.SortBySeverity() {
			if rank := severityRank(a.Severity) - severityRank(b.Severity); rank != 0 {
				return rank < 0
			}
		}
		if prefs.SortByName {
			if cmp := name(a, b); cmp != 0 {
				return cmp < 0
			}
			if cmp := position(a, b); cmp != 0 {
				return cmp < 0
			}
			return false
		}
		if cmp := position(a, b); cmp != 0 {
			return cmp < 0
		}
		return name(a, b) < 0
	})
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
			ui.Text(c, count(errors, "error", "errors")).FontSize(12).TextColor(p.Danger)
		}
		if warnings > 0 {
			ui.Text(c, count(warnings, "warning", "warnings")).FontSize(12).TextColor(p.Warning)
		}
		if infos > 0 {
			ui.Text(c, fmt.Sprintf("%d info", infos)).FontSize(12).TextColor(p.Accent)
		}
	})
	if r.Clicked() {
		m.ShowProblems()
	}
}

// problemsPanel draws the Problems tool window as OpenIDEA's bottom panel does:
// a header with the counts and severity filters, then the problems by file.
func (m *Model) problemsPanel(c *ui.Context, p *ui.Theme) {
	errors, warnings, infos := m.problemCounts()
	groups := m.problemList()
	col := ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Background(m.toolWindowBackground(p)).
		BorderWidth(1, 0, 0, 0).BorderColor(p.Border).
		Focusable().AutoFocus().HandleInput(m.handleProblemsInput)
	col.Children(func() {
		ui.Row(c).Height(30).Padding(0, 8).Gap(10).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
			ui.Text(c, "Problems").FontSize(12).Bold().TextColor(p.Text)
			ui.Button(c, "↑").Disabled(len(m.problemRows()) == 0).OnClick(func() { m.StepProblem(-1) })
			ui.Button(c, "↓").Disabled(len(m.problemRows()) == 0).OnClick(func() { m.StepProblem(1) })
			m.problemFilter(c, p, "Errors", errors, p.Danger, &m.problems.showErr)
			m.problemFilter(c, p, "Warnings", warnings, p.Warning, &m.problems.showWarn)
			m.problemFilter(c, p, "Info", infos, p.TextMuted, &m.problems.showInfo)
			ui.Spacer(c)
			fileLabel := "File"
			if t := m.tab(); t != nil && t.doc != nil {
				fileLabel = "File: " + filepath.Base(t.doc.Path())
				n := 0
				for _, problem := range m.problemsOf(realPath(t.doc.Path())) {
					if problem.Severity == "error" || problem.Severity == "warning" {
						n++
					}
				}
				if n > 0 {
					fileLabel += fmt.Sprintf("  %d", n)
				}
			}
			projectLabel := "Project Errors"
			if errors > 0 {
				projectLabel += fmt.Sprintf("  %d", errors)
			}
			m.problemScope(c, p, fileLabel, false)
			m.problemScope(c, p, projectLabel, true)
			options := ui.Button(c, "Options…")
			options.Menu(func(menu *ui.Menu) { m.problemsOptionsMenu(menu) })
			ui.Button(c, "Close").OnClick(func() { m.CloseProblems() })
		})
		if m.prefs.Problems.ShowPreview {
			ui.Row(c).Grow(1).MinHeight(0).MinWidth(0).AlignItems(ui.Stretch).
				Children(func() {
					ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
						m.problemListBody(c, p, groups)
					})
					ui.Column(c).Width(1).Shrink(0).Background(p.Border)
					ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
						m.problemSourcePreview(c, p)
					})
				})
		} else {
			m.problemListBody(c, p, groups)
		}
	})
}

func (m *Model) problemListBody(c *ui.Context, p *ui.Theme, groups [][]Problem) {
	ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Padding(4, 0).Children(func() {
			if len(groups) == 0 {
				hint := "No problems found"
				if !m.problems.project && m.tab() == nil {
					hint = "No file is open"
				}
				ui.Row(c).Padding(8, 8).Children(func() {
					ui.Text(c, hint).FontSize(12).TextColor(p.TextMuted)
				})
				return
			}
			for gi, items := range groups {
				gi, items := gi, items
				m.problemFileRow(c, p, gi, items)
			}
		})
	})
}

func (m *Model) problemsOptionsMenu(menu *ui.Menu) {
	prefs := &m.prefs.Problems
	if menu.Item("Severity").Checked(prefs.SortBySeverity()).Chosen() {
		prefs.NoSeveritySort = !prefs.NoSeveritySort
		m.savePreferences()
		m.invalidate()
	}
	if menu.Item("Name").Checked(prefs.SortByName).Chosen() {
		prefs.SortByName = !prefs.SortByName
		m.savePreferences()
		m.invalidate()
	}
	menu.Separator()
	if menu.Item("Show Preview").Checked(prefs.ShowPreview).Chosen() {
		prefs.ShowPreview = !prefs.ShowPreview
		m.savePreferences()
		if prefs.ShowPreview {
			m.ensureProblemPreview()
		}
		m.invalidate()
	}
	if menu.Item("Autoscroll to Source").Checked(prefs.Autoscroll).Chosen() {
		prefs.Autoscroll = !prefs.Autoscroll
		m.savePreferences()
		m.invalidate()
	}
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
		ui.Text(c, text).FontSize(12).TextColor(color).SingleLine()
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
		ui.Text(c, label).FontSize(12).TextColor(color).SingleLine()
	})
	if r.Clicked() && !active {
		m.problems.project = project
		m.invalidate()
	}
}

// problemFileRow is a file's header row and its problems under it.
func (m *Model) problemFileRow(c *ui.Context, p *ui.Theme, gi int, items []Problem) {
	first := items[0]
	collapsed := m.problems.collapsed[first.Path]
	dir := filepath.Dir(first.Path)
	dir = strings.TrimPrefix(dir, m.Root())
	dir = strings.TrimPrefix(dir, string(filepath.Separator))
	oi := oiPaletteOf(p)
	head := ui.Row(c.Key(fmt.Sprintf("problemfile:%d", gi))).Height(24).
		PaddingStart(4).PaddingEnd(8).Gap(6).Radius(5).AlignItems(ui.Center)
	if head.Hovered() {
		head.Background(oi.hoverTint)
	}
	head.Children(func() {
		caret := "▾"
		if collapsed {
			caret = "▸"
		}
		if ui.Button(c, caret).Clicked() {
			m.problems.collapsed[first.Path] = !collapsed
			m.invalidate()
		}
		fileIconEl(c, oi, first.Path, 16, oi.textMuted)
		nameColor := oi.textStrong
		if hasError(items) {
			nameColor = oi.danger
		}
		ui.Text(c, filepath.Base(first.Path)).FontSize(13).TextColor(nameColor).SingleLine()
		if dir != "" && dir != "." {
			ui.Text(c, dir).FontSize(13).TextColor(oi.textSubtle).SingleLine().Ellipsis("…")
		}
		ui.Spacer(c)
		var errors, warnings, infos int
		for _, item := range items {
			switch item.Severity {
			case "error":
				errors++
			case "warning":
				warnings++
			default:
				infos++
			}
		}
		var parts []string
		if errors > 0 {
			parts = append(parts, count(errors, "error", "errors"))
		}
		if warnings > 0 {
			parts = append(parts, count(warnings, "warning", "warnings"))
		}
		if infos > 0 {
			parts = append(parts, fmt.Sprintf("%d info", infos))
		}
		ui.Text(c, strings.Join(parts, ", ")).FontSize(13).TextColor(oi.textSubtle).
			SingleLine().MarginStart(4)
	})
	if head.Clicked() {
		m.selectProblem(first)
	}
	if head.DoubleClicked() {
		m.revealAt(first.Path, first.Line, first.Col)
	}
	head.Menu(func(menu *ui.Menu) {
		if menu.Item("Show Quick Fixes").Chosen() {
			m.showProblemQuickFixes(first)
		}
		if menu.Item("Jump to Source").Chosen() {
			m.revealAt(first.Path, first.Line, first.Col)
		}
		menu.Item("Copy Problem Description").Disabled(true)
	})
	if collapsed {
		return
	}
	for i, item := range items {
		i, item := i, item
		r := ui.Row(c.Key(fmt.Sprintf("problem:%d:%d", gi, i))).Height(24).
			PaddingStart(24).PaddingEnd(8).Gap(6).Radius(5).AlignItems(ui.Center)
		if m.problems.selected == problemKey(item) {
			r.Background(oi.selected)
		} else if r.Hovered() {
			r.Background(oi.hoverTint)
		}
		r.Tooltip(item.Message)
		r.Children(func() {
			oiIconEl(c, problemSeverityIcon(item.Severity), 14, oi, oi.textMuted)
			ui.Text(c, firstLineOf(item.Message)).FontSize(13).TextColor(oi.text).
				SingleLine().Ellipsis("…")
			ui.Text(c, fmt.Sprintf(":%d", item.Line+1)).FontSize(13).
				TextColor(oi.textSubtle).SingleLine()
			ui.Spacer(c)
			if source := problemSourceLabel(item); source != "" {
				ui.Text(c, source).FontSize(13).TextColor(oi.textFaint).SingleLine()
			}
		})
		if r.Clicked() {
			m.selectProblem(item)
		}
		if r.DoubleClicked() {
			m.revealAt(item.Path, item.Line, item.Col)
		}
		r.Menu(func(menu *ui.Menu) {
			if menu.Item("Show Quick Fixes").Chosen() {
				m.showProblemQuickFixes(item)
			}
			if menu.Item("Jump to Source").Chosen() {
				m.revealAt(item.Path, item.Line, item.Col)
			}
			if menu.Item("Copy Problem Description").Chosen() {
				mygo.Clipboard.WriteText(problemDescription(item))
				m.status = "Problem description copied"
				m.invalidate()
			}
		})
	}
}

func problemKey(p Problem) string {
	return fmt.Sprintf("%s\x00%d\x00%d\x00%s", p.Path, p.Line, p.Col, p.Message)
}

func problemDescription(p Problem) string {
	text := p.Message
	if p.Source != "" {
		text += " (" + p.Source
		if p.Code != "" {
			text += "(" + p.Code + ")"
		}
		text += ")"
	}
	return text
}

func problemSourceLabel(p Problem) string {
	if p.Source == "" {
		return ""
	}
	if p.Code == "" {
		return p.Source
	}
	return p.Source + "(" + p.Code + ")"
}

func (m *Model) selectProblem(p Problem) {
	m.problems.selected = problemKey(p)
	if m.prefs.Problems.Autoscroll {
		m.previewAt(p.Path, p.Line, p.Col)
	}
	if m.prefs.Problems.ShowPreview {
		m.loadProblemPreview(p)
	}
	m.invalidate()
}

func (m *Model) selectedProblem() (Problem, bool) {
	for _, p := range m.problemRows() {
		if problemKey(p) == m.problems.selected {
			return p, true
		}
	}
	return Problem{}, false
}

func (m *Model) showProblemQuickFixes(p Problem) {
	m.selectProblem(p)
	m.revealAt(p.Path, p.Line, p.Col)
	m.ShowIntentions()
}

func (m *Model) copySelectedProblem() {
	p, ok := m.selectedProblem()
	if !ok {
		return
	}
	mygo.Clipboard.WriteText(problemDescription(p))
	m.status = "Problem description copied"
	m.invalidate()
}

func (m *Model) handleProblemsInput(ev ui.InputEvent) bool {
	if ev.Kind != ui.InputKeyDown {
		return false
	}
	p, ok := m.selectedProblem()
	if !ok {
		return false
	}
	switch {
	case ev.Key == ui.KeyEnter && ev.Mods == ui.Alt:
		m.showProblemQuickFixes(p)
	case ev.Key == ui.KeyC && ev.Mods&(ui.Cmd|ui.Ctrl) != 0:
		m.copySelectedProblem()
	case ev.Key == ui.KeyF4 && ev.Mods == 0:
		m.revealAt(p.Path, p.Line, p.Col)
	default:
		return false
	}
	return true
}

func (m *Model) ensureProblemPreview() {
	rows := m.problemRows()
	if len(rows) == 0 {
		m.problems.preview = problemPreview{}
		return
	}
	selected := m.problems.selected
	for _, p := range rows {
		if problemKey(p) == selected {
			m.loadProblemPreview(p)
			return
		}
	}
	m.loadProblemPreview(rows[0])
}

func (m *Model) loadProblemPreview(p Problem) {
	m.problems.selected = problemKey(p)
	endCol := p.EndCol
	if p.EndLine != p.Line {
		endCol = int(^uint(0) >> 1)
	}
	start, lines, err := readUsagePreview(p.Path, p.Line, 20, 20)
	preview := problemPreview{
		path: p.Path, line: p.Line, col: p.Col, endCol: endCol,
		start: start, lines: lines,
	}
	if err != nil {
		preview.err = err.Error()
	}
	m.problems.preview = preview
}

func (m *Model) problemSourcePreview(c *ui.Context, p *ui.Theme) {
	preview := m.problems.preview
	oi := oiPaletteOf(p)
	ui.Row(c).Height(26).Shrink(0).Padding(0, 8).Gap(6).AlignItems(ui.Center).
		Background(oi.elev).Children(func() {
		ui.Text(c, "Preview").FontSize(10).Bold().TextColor(oi.textMuted).SingleLine()
		ui.Spacer(c)
		if preview.path != "" {
			ui.Text(c, filepath.Base(preview.path)).FontSize(10).TextColor(oi.textSubtle).
				SingleLine().Ellipsis("…")
		}
	})
	switch {
	case preview.err != "":
		ui.Row(c).Padding(8, 10).Children(func() {
			ui.Text(c, preview.err).FontSize(11).TextColor(oi.warning).SingleLine().Ellipsis("…")
		})
	case len(preview.lines) > 0:
		ui.ScrollBoth(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).Padding(5, 0).Children(func() {
				for i, line := range preview.lines {
					lineNo := preview.start + i
					row := ui.Row(c.Key(fmt.Sprintf("problem-preview:%d", lineNo))).
						Height(18).Padding(0, 5).Gap(8).AlignItems(ui.Center)
					if lineNo == preview.line {
						row.Background(oi.hoverTint)
					}
					row.Children(func() {
						ui.Text(c, fmt.Sprintf("%d", lineNo+1)).Width(42).
							Font("monospace").FontSize(10).TextColor(oi.textSubtle)
						if lineNo == preview.line {
							usageRow := usagePopupRow{
								source: openidea.Location{
									Path: preview.path,
									Range: openidea.OutlineRange{
										Start: openidea.OutlinePos{Line: uint32(lineNo), Col: uint32(preview.col)},
										End:   openidea.OutlinePos{Line: uint32(lineNo), Col: uint32(preview.endCol)},
									},
								},
								text: line, endCol: uint32(preview.endCol),
							}
							text, from, to := usagePreviewText(usageRow)
							ui.RichText(c, usageHighlightedSpans(text, from, to, oi)...).
								Font("monospace").FontSize(11).SingleLine()
						} else {
							ui.Text(c, line).Font("monospace").FontSize(11).
								TextColor(oi.textMuted).SingleLine()
						}
					})
				}
			})
		})
	default:
		ui.Row(c).Padding(8, 10).Children(func() {
			ui.Text(c, "Choose a problem to preview its source").FontSize(11).TextColor(oi.textMuted)
		})
	}
}

// problemSeverityIcon is OpenIDEA's severity drawing, the one it picks from
// "error", "warning" and "info".
func problemSeverityIcon(severity string) string {
	switch severity {
	case "error":
		return "status/error"
	case "warning":
		return "status/warning"
	}
	return "status/info"
}

// firstLineOf is the message's first line, which a problem row shows.
func firstLineOf(text string) string {
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		return text[:i]
	}
	return text
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
