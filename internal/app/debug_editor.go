package app

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

const (
	debugInlineValueSpan     = 60
	debugInlineValueMax      = 4
	debugInlineValueTextMax  = 40
	debugHoverValueTextMax   = 400
	debugSelectionExpression = 200
	debugValueHoverBase      = 100
)

type debugExpression struct {
	Start, End int
	Text       string
}

// requestEditorHover reserves a paused expression for the debugger value hint
// and falls back to the language server for every other token.
func (m *Model) requestEditorHover(t *Tab, req editor.HoverRequest) {
	if t != nil && m.debug.paused && m.debug.session != 0 &&
		m.debug.frameID != 0 && m.prefs.Debugger.ValueTooltip() {
		if expr, ok := debugExpressionAt(t, req.Offset); ok {
			m.requestDebugValueHover(t, req, expr)
			return
		}
	}
	if m.showDebugRunToCursorHover(t, req.Offset) {
		return
	}
	if m.gradleVersionCatalogHover(t, req) {
		return
	}
	m.requestHover(t, req)
}

// showDebugRunToCursorHover provides the native equivalent of OpenIDEA's
// hover-only Run to Cursor button. MyGo reports the hovered byte offset, so
// the action is offered at the start of a non-blank source line.
func (m *Model) showDebugRunToCursorHover(t *Tab, off int) bool {
	line, ok := debugRunToCursorLine(t, off)
	if !ok || !m.debug.paused || m.debug.session == 0 || m.debug.stopThread == 0 ||
		!m.prefs.Debugger.RunToCursorInlay() {
		return false
	}
	path := t.doc.Path()
	atExecution := m.debug.stopFile == path && m.debug.stopLine == line
	label := "Run to Cursor"
	action := func() { m.runToCursorAtLine(path, line, false) }
	if atExecution {
		label = "Resume Program"
		action = m.DebugContinue
	}
	start := t.doc.Buffer().Snapshot().LineStart(line)
	end := min(t.doc.Buffer().Len(), start+1)
	t.state.ShowHover(start, end, "", func(c *ui.Context) {
		ui.Button(c, label).Tooltip(label).OnClick(action)
	})
	m.invalidate()
	return true
}

const debugRunToCursorHoverChars = 4

// debugRunToCursorLine reports the line whose leading text area the pointer
// is over. OpenIDEA exposes the action over the first four display columns
// before the first non-space character.
func debugRunToCursorLine(t *Tab, off int) (int, bool) {
	if t == nil || t.doc == nil || off < 0 {
		return 0, false
	}
	snapshot := t.doc.Buffer().Snapshot()
	if off > snapshot.Len() {
		return 0, false
	}
	pos := snapshot.Pos(off)
	text := snapshot.Line(pos.Line)
	indent := len(text) - len(strings.TrimLeft(text, " \t"))
	if strings.TrimSpace(text) == "" || pos.Col < indent || pos.Col > indent+debugRunToCursorHoverChars {
		return 0, false
	}
	return pos.Line, true
}

func (m *Model) requestDebugValueHover(t *Tab, req editor.HoverRequest, expr debugExpression) {
	session, frame := m.debug.session, m.debug.frameID
	generation, docVersion := m.debug.stopGeneration, req.Version
	t.hoverGen++
	hoverGeneration := t.hoverGen
	delay := time.Duration(max(0, m.prefs.Debugger.TooltipDelay()-debugValueHoverBase)) * time.Millisecond
	go func() {
		if delay > 0 {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-m.ctx.Done():
				return
			case <-timer.C:
			}
		}
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "debug_evaluate_in", map[string]any{
			"session": session, "frameId": frame, "expression": expr.Text, "context": "hover",
		})
		if err != nil {
			m.post(func() {
				if m.tabOpen(t) && t.hoverGen == hoverGeneration {
					m.requestHover(t, req)
				}
			})
			return
		}
		var value openidea.Variable
		if err := json.Unmarshal(raw, &value); err != nil {
			m.post(func() {
				if m.tabOpen(t) && t.hoverGen == hoverGeneration {
					m.requestHover(t, req)
				}
			})
			return
		}
		m.post(func() {
			if !m.tabOpen(t) || t.hoverGen != hoverGeneration || t.doc == nil ||
				t.doc.Buffer().Version() != docVersion || m.debug.session != session ||
				!m.debug.paused || m.debug.frameID != frame ||
				m.debug.stopGeneration != generation {
				return
			}
			t.state.ShowHover(expr.Start, expr.End, debugValueHint(expr.Text, value), nil)
			m.invalidate()
		})
	}()
}

func debugValueHint(expression string, value openidea.Variable) string {
	text := expression + " = " + inlineDebugValue(value.Value, debugHoverValueTextMax)
	if value.TypeName != nil && *value.TypeName != "" {
		text += "  {" + *value.TypeName + "}"
	}
	return text
}

func inlineDebugValue(value string, limit int) string {
	value = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(value)
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	if limit <= 1 {
		return "…"
	}
	return string(runes[:limit-1]) + "…"
}

// debugExpressionAt returns the selection under off, or the dotted identifier
// around it. The selected expression must be one non-blank line, as OpenIDEA's
// value tooltip permits.
func debugExpressionAt(t *Tab, off int) (debugExpression, bool) {
	if t == nil || t.state == nil || t.doc == nil {
		return debugExpression{}, false
	}
	text := t.doc.Text()
	if off < 0 || off > len(text) {
		return debugExpression{}, false
	}
	sel := t.state.Selection()
	start, end := min(sel.Anchor, sel.Head), max(sel.Anchor, sel.Head)
	if start != end && off >= start && off <= end {
		selected := strings.TrimSpace(text[start:end])
		if selected != "" && len(selected) <= debugSelectionExpression && !strings.ContainsAny(selected, "\r\n") {
			return debugExpression{Start: start, End: end, Text: selected}, true
		}
		return debugExpression{}, false
	}
	return debugIdentifierAt(text, off)
}

func debugIdentifierAt(text string, off int) (debugExpression, bool) {
	if off > 0 && off == len(text) || (off < len(text) && !debugIdentifierPartAt(text, off)) {
		off--
	}
	if off < 0 || off >= len(text) || !debugIdentifierPartAt(text, off) {
		return debugExpression{}, false
	}
	start, end := off, off+1
	for start > 0 && debugIdentifierPartAt(text, start-1) {
		start--
	}
	for end < len(text) && debugIdentifierPartAt(text, end) {
		end++
	}
	for {
		dot := end
		for dot < len(text) && (text[dot] == '.' || text[dot] == '$') {
			dot++
		}
		if dot == end || dot >= len(text) || !debugIdentifierPartAt(text, dot) {
			break
		}
		end = dot + 1
		for end < len(text) && debugIdentifierPartAt(text, end) {
			end++
		}
	}
	for {
		dot := start
		for dot > 0 && (text[dot-1] == '.' || text[dot-1] == '$') {
			dot--
		}
		if dot == start || dot == 0 || !debugIdentifierPartAt(text, dot-1) {
			break
		}
		start = dot - 1
		for start > 0 && debugIdentifierPartAt(text, start-1) {
			start--
		}
	}
	expr := text[start:end]
	if expr == "" || !debugIdentifierStart(expr[0]) {
		return debugExpression{}, false
	}
	return debugExpression{Start: start, End: end, Text: expr}, true
}

func debugIdentifierPartAt(text string, off int) bool {
	if off < 0 || off >= len(text) {
		return false
	}
	b := text[off]
	return b == '_' || b == '$' || b >= '0' && b <= '9' ||
		b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= utf8.RuneSelf
}

func debugIdentifierStart(b byte) bool {
	r, _ := utf8.DecodeRune([]byte{b})
	return b == '_' || b == '$' || b >= utf8.RuneSelf || unicode.IsLetter(r)
}

var debugInlineIdentifier = regexp.MustCompile(`[A-Za-z_$][A-Za-z0-9_$]*`)

func debugInlineParts(line string, vars []openidea.Variable) []string {
	if strings.TrimSpace(line) == "" {
		return nil
	}
	values := make(map[string]openidea.Variable, len(vars))
	for _, variable := range vars {
		if variable.Name != "" {
			if _, exists := values[variable.Name]; !exists {
				values[variable.Name] = variable
			}
		}
	}
	parts := make([]string, 0, debugInlineValueMax)
	seen := make(map[string]bool, debugInlineValueMax)
	for _, match := range debugInlineIdentifier.FindAllString(line, -1) {
		if seen[match] {
			continue
		}
		variable, ok := values[match]
		if !ok {
			continue
		}
		seen[match] = true
		parts = append(parts, match+": "+inlineDebugValue(variable.Value, debugInlineValueTextMax))
		if len(parts) == debugInlineValueMax {
			break
		}
	}
	return parts
}

// debugInlineInlays builds the local values shown after lines in the active
// frame's source. MyGo stores inlays in byte offsets, so the text is inserted
// at each line's end rather than at a widget in a separate editor lane.
func debugInlineInlays(t *Tab, paused bool, path string, stopLine int, vars []openidea.Variable) []editor.Inlay {
	if t == nil || t.doc == nil || t.state == nil || !paused || path == "" ||
		t.doc.Path() != path || stopLine < 0 || len(vars) == 0 {
		return nil
	}
	buffer := t.doc.Buffer()
	snapshot := buffer.Snapshot()
	first := max(0, stopLine-debugInlineValueSpan)
	last := min(stopLine, snapshot.LineCount()-1)
	out := make([]editor.Inlay, 0, last-first+1)
	for line := first; line <= last; line++ {
		text := snapshot.Line(line)
		parts := debugInlineParts(text, vars)
		if len(parts) == 0 {
			continue
		}
		out = append(out, editor.Inlay{
			Offset: snapshot.LineEnd(line),
			Text:   "    " + strings.Join(parts, ", "),
		})
	}
	return out
}

// refreshDebugInlays combines values for the selected pause frame with the
// language server and Code Vision hints already held by the editor.
func (m *Model) refreshDebugInlays(t *Tab) {
	if t == nil || t.state == nil || t.doc == nil {
		return
	}
	var inlays []editor.Inlay
	if m.prefs.Debugger.InlineValues() {
		inlays = debugInlineInlays(
			t, m.debug.paused, m.debug.stopFile, m.debug.stopLine, m.debug.vars,
		)
	}
	t.inlay.debug = inlays
	applyEditorInlays(t)
}

const debugSelectionSettleDelay = 400 * time.Millisecond

// cancelDebugSelectionHover invalidates a pending or in-flight selection
// value lookup.
func (m *Model) cancelDebugSelectionHover(t *Tab) {
	if t == nil {
		return
	}
	t.debugSelectGen++
	if t.debugSelectTimer != nil {
		t.debugSelectTimer.Stop()
		t.debugSelectTimer = nil
	}
}

// scheduleDebugSelectionHover is MyGo's equivalent of CodeMirror's
// selection-settled hook. The value is requested only after a one-range,
// single-line selection rests.
func (m *Model) scheduleDebugSelectionHover(t *Tab) {
	if t == nil || t.state == nil || t.doc == nil || m.ctx == nil {
		return
	}
	m.cancelDebugSelectionHover(t)
	if !m.debug.paused || m.debug.session == 0 || m.debug.frameID == 0 ||
		!m.prefs.Debugger.ValueTooltipOnSelection() {
		return
	}
	if _, open := m.groupOf(t); !open {
		return
	}
	expr, ok := debugSelectionExpressionAt(t)
	if !ok {
		return
	}
	selection := t.state.Selection()
	version := t.doc.Buffer().Version()
	session, frame := m.debug.session, m.debug.frameID
	generation := m.debug.stopGeneration
	selectGeneration := t.debugSelectGen
	t.debugSelectTimer = time.AfterFunc(debugSelectionSettleDelay, func() {
		m.post(func() {
			t.debugSelectTimer = nil
			if !m.debugSelectionStillCurrent(
				t, selection, version, session, frame, generation, selectGeneration,
			) {
				return
			}
			m.requestDebugSelectionValue(
				t, expr, selection, version, session, frame, generation, selectGeneration,
			)
		})
	})
}

func debugSelectionExpressionAt(t *Tab) (debugExpression, bool) {
	if t == nil || t.state == nil || t.doc == nil || len(t.state.Selections()) != 1 {
		return debugExpression{}, false
	}
	selection := t.state.Selection()
	if selection.Empty() {
		return debugExpression{}, false
	}
	text := t.doc.Text()
	start, end := selection.Start(), selection.End()
	if start < 0 || end > len(text) {
		return debugExpression{}, false
	}
	raw := text[start:end]
	selected := strings.TrimSpace(raw)
	if selected == "" || len(selected) > debugSelectionExpression ||
		strings.ContainsAny(selected, "\r\n") {
		return debugExpression{}, false
	}
	if lead := strings.Index(raw, selected); lead >= 0 {
		start += lead
		end = start + len(selected)
	}
	return debugExpression{Start: start, End: end, Text: selected}, true
}

func (m *Model) debugSelectionStillCurrent(
	t *Tab,
	selection editor.Selection,
	version uint64,
	session uint32,
	frame int64,
	generation uint64,
	selectGeneration uint64,
) bool {
	if t == nil || t.state == nil || t.doc == nil || t.debugSelectGen != selectGeneration ||
		t.doc.Buffer().Version() != version || m.debug.session != session ||
		!m.debug.paused || m.debug.frameID != frame ||
		m.debug.stopGeneration != generation ||
		t.state.Selection() != selection {
		return false
	}
	_, open := m.groupOf(t)
	return open
}

func (m *Model) requestDebugSelectionValue(
	t *Tab,
	expr debugExpression,
	selection editor.Selection,
	version uint64,
	session uint32,
	frame int64,
	generation uint64,
	selectGeneration uint64,
) {
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "debug_evaluate_in", map[string]any{
			"session": session, "frameId": frame,
			"expression": expr.Text, "context": "hover",
		})
		if err != nil {
			return
		}
		var value openidea.Variable
		if err := json.Unmarshal(raw, &value); err != nil {
			return
		}
		m.post(func() {
			if !m.debugSelectionStillCurrent(
				t, selection, version, session, frame, generation, selectGeneration,
			) {
				return
			}
			t.state.ShowHover(expr.Start, expr.End, debugValueHint(expr.Text, value), nil)
			m.invalidate()
		})
	}()
}

func sortedDebugVariables(vars []openidea.Variable, enabled bool) []openidea.Variable {
	out := append([]openidea.Variable(nil), vars...)
	if !enabled || debugVariablesAreIndices(out) {
		return out
	}
	sort.SliceStable(out, func(i, j int) bool {
		return naturalDebugNameLess(out[i].Name, out[j].Name)
	})
	return out
}

func sortedDebugVariableIndexes(vars []openidea.Variable, enabled bool) []int {
	indices := make([]int, len(vars))
	for i := range indices {
		indices[i] = i
	}
	if !enabled || debugVariablesAreIndices(vars) {
		return indices
	}
	sort.SliceStable(indices, func(i, j int) bool {
		return naturalDebugNameLess(vars[indices[i]].Name, vars[indices[j]].Name)
	})
	return indices
}

func debugVariablesAreIndices(vars []openidea.Variable) bool {
	if len(vars) == 0 {
		return false
	}
	for _, variable := range vars {
		name := strings.Trim(variable.Name, "[]")
		if name == "" {
			return false
		}
		for _, r := range name {
			if !unicode.IsDigit(r) {
				return false
			}
		}
	}
	return true
}

func naturalDebugNameLess(a, b string) bool {
	lowerA, lowerB := strings.ToLower(a), strings.ToLower(b)
	for i, j := 0, 0; i < len(lowerA) && j < len(lowerB); {
		if lowerA[i] >= '0' && lowerA[i] <= '9' && lowerB[j] >= '0' && lowerB[j] <= '9' {
			ai, bj := i, j
			for ai < len(lowerA) && lowerA[ai] >= '0' && lowerA[ai] <= '9' {
				ai++
			}
			for bj < len(lowerB) && lowerB[bj] >= '0' && lowerB[bj] <= '9' {
				bj++
			}
			an, _ := strconv.Atoi(lowerA[i:ai])
			bn, _ := strconv.Atoi(lowerB[j:bj])
			if an != bn {
				return an < bn
			}
			i, j = ai, bj
			continue
		}
		if lowerA[i] != lowerB[j] {
			return lowerA[i] < lowerB[j]
		}
		i++
		j++
	}
	if len(lowerA) == len(lowerB) {
		return a < b
	}
	return len(lowerA) < len(lowerB)
}

func (m *Model) debugValueAt(ctx context.Context, expression string) (openidea.Variable, error) {
	raw, err := m.invoke(ctx, "debug_evaluate_in", map[string]any{
		"session": m.debug.session, "frameId": m.debug.frameID,
		"expression": expression, "context": "hover",
	})
	if err != nil {
		return openidea.Variable{}, err
	}
	var value openidea.Variable
	if err := json.Unmarshal(raw, &value); err != nil {
		return openidea.Variable{}, fmt.Errorf("decode debug value: %w", err)
	}
	return value, nil
}
