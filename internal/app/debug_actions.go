package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

func init() {
	_ = registerAction(&Action{
		ID:       "debug.pluginStart",
		Title:    "Debug Plugin Configuration",
		Category: "Run",
		Keywords: "plugin debug configuration external run",
		When:     func(m *Model) bool { return m.Root() != "" },
		RunWithArg: func(m *Model, arg any) {
			m.DebugPluginStart(arg)
		},
	})
}

// DebugPluginStart revalidates a plugin-owned configuration through
// OpenIDEA's plugin host before launching it. The plugin bridge calls the
// hidden action with {"config": ...}; in-process callers may also supply
// onExit to receive the same completion callback as OpenIDEA's action.
func (m *Model) DebugPluginStart(arg any) {
	request := debugPluginArgObject(arg)
	if request == nil {
		m.status = "The plugin debug configuration is missing"
		m.invalidate()
		return
	}
	config, wrapped := request["config"]
	if !wrapped {
		config = request
	}
	raw, err := json.Marshal(config)
	if err != nil || len(raw) == 0 || string(raw) == "null" {
		m.status = "Could not read the plugin debug configuration"
		m.invalidate()
		return
	}
	var onExit func(code *int32, stopped bool)
	if callback, ok := request["onExit"].(func(*int32, bool)); ok {
		onExit = callback
	}
	if m.ctx == nil {
		m.status = "The plugin debugger is not connected"
		m.invalidate()
		return
	}
	if m.mgr == nil && m.invokeHook == nil {
		m.status = "The plugin debugger is not connected"
		m.invalidate()
		return
	}
	ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
	m.status = "Preparing plugin debug configuration…"
	m.invalidate()
	go func() {
		defer cancel()
		prepared, err := m.invoke(ctx, "plugin_prepare_debug_run", map[string]any{
			"config": json.RawMessage(raw),
		})
		m.post(func() {
			if err != nil {
				m.status = "Could not prepare the plugin debug configuration: " + err.Error()
				m.invalidate()
				return
			}
			cfg, decodeErr := openidea.DecodeRunConfig(prepared)
			if decodeErr != nil {
				m.status = "Could not read the prepared plugin debug configuration: " + decodeErr.Error()
				m.invalidate()
				return
			}
			m.debug.pluginExit = onExit
			m.debug.pluginStopped = false
			m.status = ""
			m.DebugRunConfig(cfg)
		})
	}()
}

func debugPluginArgObject(arg any) map[string]any {
	switch value := arg.(type) {
	case map[string]any:
		return value
	case json.RawMessage:
		var object map[string]any
		if json.Unmarshal(value, &object) == nil {
			return object
		}
	case []byte:
		var object map[string]any
		if json.Unmarshal(value, &object) == nil {
			return object
		}
	case openidea.RunConfig:
		return map[string]any{"config": value}
	case *openidea.RunConfig:
		if value != nil {
			return map[string]any{"config": value}
		}
	}
	return nil
}

type debugSetValueState struct {
	open      bool
	busy      bool
	value     string
	err       string
	variable  openidea.Variable
	container int64
	frameID   int64
	session   uint32
}

type debugSmartStepState struct {
	open    bool
	loading bool
	err     string
	targets []openidea.StepInTarget
	session uint32
	thread  int64
}

// OpenDebugSetValue opens the native Set Value editor for the selected
// variable, or the first variable when the panel has no selection.
func (m *Model) OpenDebugSetValue() {
	d := &m.debug
	if !d.paused || d.session == 0 || d.frameID == 0 || len(d.vars) == 0 {
		d.err = "Pause in a frame with variables before using Set Value"
		m.invalidate()
		return
	}
	i := d.selVar
	if i < 0 || i >= len(d.vars) {
		i = 0
	}
	v := d.vars[i]
	d.setValue = debugSetValueState{
		open:      true,
		value:     v.Value,
		variable:  v,
		container: d.varContainer,
		frameID:   d.frameID,
		session:   d.session,
	}
	m.invalidate()
}

func (m *Model) commitDebugSetValue() {
	v := &m.debug.setValue
	if !v.open || v.busy || v.session == 0 || v.frameID == 0 {
		return
	}
	value := strings.TrimSpace(v.value)
	if value == "" {
		v.err = "Enter a value"
		m.invalidate()
		return
	}
	args := map[string]any{
		"session":      v.session,
		"container":    v.container,
		"name":         v.variable.Name,
		"evaluateName": v.variable.EvaluateName,
		"frameId":      v.frameID,
		"value":        value,
	}
	v.busy, v.err = true, ""
	m.invalidate()
	name, session := v.variable.Name, v.session
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "debug_set_variable", args)
		var updated openidea.Variable
		if err == nil {
			err = json.Unmarshal(raw, &updated)
		}
		m.post(func() {
			if m.debug.setValue.session != session {
				return
			}
			m.debug.setValue.busy = false
			if err != nil {
				m.debug.setValue.err = err.Error()
				m.invalidate()
				return
			}
			for i := range m.debug.vars {
				if m.debug.vars[i].Name == name {
					m.debug.vars[i] = updated
					break
				}
			}
			m.debug.setValue.open = false
			m.evaluateWatches()
			m.invalidate()
		})
	}()
}

func (m *Model) debugSetValueDialog(c *ui.Context, p *ui.Theme) {
	v := &m.debug.setValue
	open := v.open
	d := ui.Modal(c.Key("debug-set-value"), &open, func() {
		ui.Column(c).Width(460).Gap(9).Children(func() {
			ui.Text(c, "Set Value").FontSize(13).Bold().TextColor(p.Text)
			ui.Text(c, v.variable.Name).FontSize(12).TextColor(p.TextMuted).SingleLine()
			in := ui.TextInput(c.Key("debug-set-value-input"), &v.value).AutoFocus()
			if in.Submitted() {
				m.commitDebugSetValue()
			}
			if v.err != "" {
				ui.Text(c, v.err).FontSize(12).TextColor(p.Danger).SingleLine().Ellipsis("…")
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Button(c, "Cancel").OnClick(func() {
					m.debug.setValue.open = false
					m.invalidate()
				})
				ui.Button(c, "Set Value").Disabled(v.busy).OnClick(m.commitDebugSetValue)
			})
		})
	})
	if d.Dismissed() || !open {
		v.open = false
	}
}

// DebugResetFrame returns the selected frame to the start of its method.
func (m *Model) DebugResetFrame() {
	d := &m.debug
	if !d.paused || d.session == 0 || len(d.frames) == 0 {
		return
	}
	if d.selFrame < 0 || d.selFrame >= len(d.frames) {
		return
	}
	session, frame := d.session, d.frames[d.selFrame]
	d.status = "Resetting frame…"
	d.paused = false
	m.refreshDecorations()
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		_, err := m.invoke(ctx, "debug_restart_frame", map[string]any{
			"session": session, "frameId": frame.ID,
		})
		m.post(func() {
			if m.debug.session != session {
				return
			}
			if err != nil {
				m.debug.paused = true
				m.debug.err = "Reset Frame failed: " + err.Error()
			} else {
				m.debug.status = "Frame reset"
			}
			m.invalidate()
		})
	}()
}

// DebugSmartStepInto asks the adapter which calls are on the current line and
// steps directly when there is at most one target.
func (m *Model) DebugSmartStepInto() {
	d := &m.debug
	if !d.paused || d.session == 0 || d.stopThread == 0 || len(d.frames) == 0 {
		return
	}
	if d.selFrame < 0 || d.selFrame >= len(d.frames) {
		return
	}
	session, thread, frame := d.session, d.stopThread, d.frames[d.selFrame]
	d.smartStep = debugSmartStepState{open: true, loading: true, session: session, thread: thread}
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "debug_step_in_targets", map[string]any{
			"session": session, "frameId": frame.ID,
		})
		var targets []openidea.StepInTarget
		if err == nil {
			err = json.Unmarshal(raw, &targets)
		}
		m.post(func() {
			if m.debug.session != session || !m.debug.paused {
				return
			}
			if err != nil {
				m.debug.err = "Smart Step Into failed: " + err.Error()
				m.debug.smartStep.open = false
				m.invalidate()
				return
			}
			switch len(targets) {
			case 0:
				m.debug.smartStep.open = false
				m.StepInto()
			case 1:
				m.debug.smartStep.open = false
				m.stepIntoDebugTarget(targets[0].ID)
			default:
				m.debug.smartStep.loading = false
				m.debug.smartStep.targets = targets
				m.invalidate()
			}
		})
	}()
}

func (m *Model) stepIntoDebugTarget(targetID int64) {
	d := &m.debug
	if d.smartStep.session == 0 || d.smartStep.thread == 0 || targetID == 0 {
		return
	}
	session, thread := d.smartStep.session, d.smartStep.thread
	d.smartStep.open = false
	d.status = "Smart stepping…"
	d.paused = false
	m.refreshDecorations()
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		_, err := m.invoke(ctx, "debug_step_in_target", map[string]any{
			"session": session, "threadId": thread, "targetId": targetID,
		})
		m.post(func() {
			if m.debug.session != session {
				return
			}
			if err != nil {
				m.debug.paused = true
				m.debug.err = "Smart Step Into failed: " + err.Error()
			}
			m.invalidate()
		})
	}()
}

func (m *Model) debugSmartStepDialog(c *ui.Context, p *ui.Theme) {
	v := &m.debug.smartStep
	open := v.open
	d := ui.Modal(c.Key("debug-smart-step"), &open, func() {
		ui.Column(c).Width(420).Gap(7).Children(func() {
			ui.Text(c, "Smart Step Into").FontSize(13).Bold().TextColor(p.Text)
			if v.loading {
				ui.Text(c, "Finding calls on the current line…").FontSize(12).TextColor(p.TextMuted)
			}
			if v.err != "" {
				ui.Text(c, v.err).FontSize(12).TextColor(p.Danger)
			}
			for _, target := range v.targets {
				target := target
				row := ui.Button(c, target.Label).Disabled(v.loading)
				if row.Clicked() {
					m.stepIntoDebugTarget(target.ID)
				}
			}
			ui.Button(c, "Cancel").OnClick(func() {
				v.open = false
				m.invalidate()
			})
		})
	})
	if d.Dismissed() || !open {
		v.open = false
	}
}

// DebugJumpToSource reveals the selected frame or the current execution point.
func (m *Model) DebugJumpToSource() {
	d := &m.debug
	if !d.paused {
		return
	}
	if len(d.frames) > 0 && d.selFrame >= 0 && d.selFrame < len(d.frames) {
		f := d.frames[d.selFrame]
		if f.Path != nil {
			m.openFile(*f.Path)
			m.revealAt(*f.Path, f.Line, f.Column)
			return
		}
	}
	m.ShowExecutionPoint()
}

// DebugHotRestart asks adapters that support it (Flutter) to restart the app
// while keeping the debug session.
func (m *Model) DebugHotRestart() {
	d := &m.debug
	if d.session == 0 || d.ended || d.starting {
		return
	}
	session := d.session
	d.status = "Restarting the application…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
		defer cancel()
		_, err := m.invoke(ctx, "debug_hot_restart", map[string]any{"session": session})
		m.post(func() {
			if m.debug.session != session {
				return
			}
			if err != nil {
				m.debug.err = "Hot Restart failed: " + err.Error()
			} else {
				m.debug.status = "Application restarted"
			}
			m.invalidate()
		})
	}()
}

func (m *Model) OpenDebugBreakpoints() {
	m.debug.breakpointsOpen = true
	m.invalidate()
}

// DebugBrowseConsoleHistory shows the expressions entered in this session.
func (m *Model) DebugBrowseConsoleHistory() {
	v := &m.debug.console
	if len(v.history) == 0 {
		m.debug.status = "No debug console history yet"
		m.invalidate()
		return
	}
	v.historyOpen = true
	m.invalidate()
}

func (m *Model) debugConsoleHistoryDialog(c *ui.Context, p *ui.Theme) {
	v := &m.debug.console
	open := v.historyOpen
	d := ui.Modal(c.Key("debug-console-history"), &open, func() {
		ui.Column(c).Width(520).Gap(6).Children(func() {
			ui.Text(c, "Debug Console History").FontSize(13).Bold().TextColor(p.Text)
			for i := len(v.history) - 1; i >= 0; i-- {
				text := v.history[i]
				if strings.TrimSpace(text) == "" {
					continue
				}
				row := ui.Button(c, text).AlignItems(ui.Start)
				if row.Clicked() {
					v.expr = text
					v.historyOpen = false
					m.debug.view = debugViewConsole
					m.invalidate()
				}
			}
			ui.Button(c, "Cancel").OnClick(func() {
				v.historyOpen = false
				m.invalidate()
			})
		})
	})
	if d.Dismissed() || !open {
		v.historyOpen = false
	}
}

// ToggleTemporaryBreakpoint adds a line breakpoint that the adapter removes
// again after its first stop.
func (m *Model) ToggleTemporaryBreakpoint() {
	t := m.tab()
	if t == nil {
		return
	}
	path, line := t.doc.Path(), t.state.CursorPos().Line
	if !isJavaSource(path) || line < 0 {
		m.debug.err = "Temporary breakpoints require a Java source line"
		m.invalidate()
		return
	}
	if m.debug.bps == nil {
		m.debug.bps = map[string][]int{}
	}
	if m.debug.temporaryBps == nil {
		m.debug.temporaryBps = map[string][]int{}
	}
	lines := m.debug.bps[path]
	if containsInt(lines, line) {
		m.debug.bps[path] = removeInt(lines, line)
		m.debug.temporaryBps[path] = removeInt(m.debug.temporaryBps[path], line)
		m.debug.status = fmt.Sprintf("Temporary breakpoint removed at line %d", line+1)
	} else {
		m.debug.bps[path] = append(lines, line)
		sort.Ints(m.debug.bps[path])
		m.debug.temporaryBps[path] = append(m.debug.temporaryBps[path], line)
		sort.Ints(m.debug.temporaryBps[path])
		m.debug.status = fmt.Sprintf("Temporary breakpoint set at line %d", line+1)
	}
	if len(m.debug.bps[path]) == 0 {
		delete(m.debug.bps, path)
	}
	if len(m.debug.temporaryBps[path]) == 0 {
		delete(m.debug.temporaryBps, path)
	}
	m.refreshDecorations()
	m.invalidate()
	m.pushBreakpointsAsync(path)
}

func (m *Model) removeTemporaryBreakpoint(path string, line int) {
	if !containsInt(m.debug.temporaryBps[path], line) {
		return
	}
	m.debug.bps[path] = removeInt(m.debug.bps[path], line)
	m.debug.temporaryBps[path] = removeInt(m.debug.temporaryBps[path], line)
	if len(m.debug.bps[path]) == 0 {
		delete(m.debug.bps, path)
	}
	if len(m.debug.temporaryBps[path]) == 0 {
		delete(m.debug.temporaryBps, path)
	}
	m.pushBreakpointsAsync(path)
}

var (
	javaPackagePattern = regexp.MustCompile(`(?m)^\s*package\s+([A-Za-z_$][\w$]*(?:\.[A-Za-z_$][\w$]*)*)\s*;`)
	javaTypePattern    = regexp.MustCompile(`\b(?:class|interface|enum|record)\s+([A-Za-z_$][\w$]*)`)
	javaMethodPattern  = regexp.MustCompile(`(?:\b[A-Za-z_$][\w$<>\[\],.? ]+\s+)?([A-Za-z_$][\w$]*)\s*\([^;{}]*\)\s*(?:throws\s+[^{]+)?\{`)
	javaFieldPattern   = regexp.MustCompile(`(?:\b(?:public|protected|private|static|final|transient|volatile)\s+)+[A-Za-z_$][\w$<>\[\],.?]*\s+([A-Za-z_$][\w$]*)\s*(?:=|;)`)
)

func javaDebugTypeAt(text string, line int) string {
	lines := strings.Split(text, "\n")
	if line < 0 || line >= len(lines) {
		return ""
	}
	before := strings.Join(lines[:line+1], "\n")
	pkg := ""
	if match := javaPackagePattern.FindStringSubmatch(before); len(match) == 2 {
		pkg = match[1]
	}
	allTypes := javaTypePattern.FindAllStringSubmatch(before, -1)
	if len(allTypes) == 0 {
		return ""
	}
	typ := allTypes[len(allTypes)-1][1]
	if pkg == "" {
		return typ
	}
	return pkg + "." + typ
}

func javaDebugMethodAt(text string, line int) string {
	lines := strings.Split(text, "\n")
	if line < 0 || line >= len(lines) {
		return ""
	}
	end := min(len(lines), line+6)
	snippet := stripJavaComments(strings.Join(lines[line:end], " "))
	match := javaMethodPattern.FindStringSubmatch(snippet)
	typ := javaDebugTypeAt(text, line)
	if len(match) != 2 || typ == "" || match[1] == "if" || match[1] == "for" ||
		match[1] == "while" || match[1] == "switch" || match[1] == "catch" {
		return ""
	}
	return typ + "#" + match[1]
}

func javaDebugFieldAt(text string, line int) string {
	lines := strings.Split(text, "\n")
	if line < 0 || line >= len(lines) {
		return ""
	}
	match := javaFieldPattern.FindStringSubmatch(stripJavaComments(lines[line]))
	typ := javaDebugTypeAt(text, line)
	if len(match) != 2 || typ == "" {
		return ""
	}
	return typ + "#" + match[1]
}

func stripJavaComments(text string) string {
	text = regexp.MustCompile(`//.*`).ReplaceAllString(text, "")
	text = regexp.MustCompile(`/\*.*?\*/`).ReplaceAllString(text, " ")
	return strings.Join(strings.Fields(text), " ")
}

func containsInt(list []int, value int) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

func removeInt(list []int, value int) []int {
	out := list[:0]
	for _, item := range list {
		if item != value {
			out = append(out, item)
		}
	}
	return out
}

func containsFunctionBreakpoint(list []openidea.FunctionBreakpoint, name string) int {
	for i := range list {
		if list[i].Name == name {
			return i
		}
	}
	return -1
}

func containsDataBreakpoint(list []openidea.DataBreakpoint, id string) int {
	for i := range list {
		if list[i].DataID == id {
			return i
		}
	}
	return -1
}

// ToggleMethodBreakpoint sets a DAP function breakpoint at the Java method
// declaration under the caret.
func (m *Model) ToggleMethodBreakpoint() {
	t := m.tab()
	if t == nil {
		return
	}
	name := javaDebugMethodAt(t.doc.Text(), t.state.CursorPos().Line)
	if name == "" {
		m.debug.err = "No Java method declaration on this line"
		m.invalidate()
		return
	}
	if i := containsFunctionBreakpoint(m.debug.methodBps, name); i >= 0 {
		m.debug.methodBps = append(m.debug.methodBps[:i], m.debug.methodBps[i+1:]...)
		m.debug.status = "Method breakpoint removed: " + name
	} else {
		m.debug.methodBps = append(m.debug.methodBps, openidea.FunctionBreakpoint{Name: name, Enabled: true})
		m.debug.status = "Method breakpoint set: " + name
	}
	m.invalidate()
	m.pushSpecialBreakpoints()
}

// ToggleFieldWatchpoint sets a DAP data breakpoint at the Java field under
// the caret. java-debug accepts Class#field data ids across sessions.
func (m *Model) ToggleFieldWatchpoint() {
	t := m.tab()
	if t == nil {
		return
	}
	id := javaDebugFieldAt(t.doc.Text(), t.state.CursorPos().Line)
	if id == "" {
		m.debug.err = "No Java field declaration on this line"
		m.invalidate()
		return
	}
	if i := containsDataBreakpoint(m.debug.fieldBps, id); i >= 0 {
		m.debug.fieldBps = append(m.debug.fieldBps[:i], m.debug.fieldBps[i+1:]...)
		m.debug.status = "Field watchpoint removed: " + id
	} else {
		m.debug.fieldBps = append(m.debug.fieldBps, openidea.DataBreakpoint{DataID: id, Enabled: true})
		m.debug.status = "Field watchpoint set: " + id
	}
	m.invalidate()
	m.pushSpecialBreakpoints()
}

func (m *Model) pushSpecialBreakpoints() {
	methods := append([]openidea.FunctionBreakpoint(nil), m.debug.methodBps...)
	fields := append([]openidea.DataBreakpoint(nil), m.debug.fieldBps...)
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		_, _ = m.invoke(ctx, "debug_set_function_breakpoints", map[string]any{"breakpoints": methods})
		_, _ = m.invoke(ctx, "debug_set_data_breakpoints", map[string]any{"breakpoints": fields})
	}()
}

func (m *Model) debugBreakpointsDialog(c *ui.Context, p *ui.Theme) {
	open := m.debug.breakpointsOpen
	d := ui.Modal(c.Key("debug-breakpoints"), &open, func() {
		ui.Column(c).Width(620).Height(460).Gap(7).Children(func() {
			ui.Text(c, "Breakpoints").FontSize(13).Bold().TextColor(p.Text)
			ui.Scroll(c).Grow(1).Children(func() {
				paths := make([]string, 0, len(m.debug.bps))
				for path := range m.debug.bps {
					paths = append(paths, path)
				}
				sort.Strings(paths)
				for _, path := range paths {
					for _, line := range m.debug.breakpointLines(path) {
						path, line := path, line
						label := path
						if rel, err := filepath.Rel(m.Root(), path); err == nil {
							label = rel
						}
						ui.Row(c).Height(26).Gap(6).AlignItems(ui.Center).Children(func() {
							if containsInt(m.debug.temporaryBps[path], line) {
								label = "Temporary · " + label
							}
							ui.Text(c, fmt.Sprintf("%s:%d", label, line+1)).Grow(1).FontSize(12).
								TextColor(p.Text).SingleLine().Ellipsis("…")
							if ui.Button(c, "Remove").Clicked() {
								m.debug.bps[path] = removeInt(m.debug.bps[path], line)
								m.debug.temporaryBps[path] = removeInt(m.debug.temporaryBps[path], line)
								m.pushBreakpointsAsync(path)
								m.invalidate()
							}
						})
					}
				}
				for i, bp := range append([]openidea.FunctionBreakpoint(nil), m.debug.methodBps...) {
					i, bp := i, bp
					ui.Row(c).Height(26).Gap(6).AlignItems(ui.Center).Children(func() {
						ui.Text(c, "Method · "+bp.Name).Grow(1).FontSize(12).TextColor(p.Text)
						if ui.Button(c, "Remove").Clicked() {
							m.debug.methodBps = append(m.debug.methodBps[:i], m.debug.methodBps[i+1:]...)
							m.pushSpecialBreakpoints()
							m.invalidate()
						}
					})
				}
				for i, bp := range append([]openidea.DataBreakpoint(nil), m.debug.fieldBps...) {
					i, bp := i, bp
					ui.Row(c).Height(26).Gap(6).AlignItems(ui.Center).Children(func() {
						ui.Text(c, "Field · "+bp.DataID).Grow(1).FontSize(12).TextColor(p.Text)
						if ui.Button(c, "Remove").Clicked() {
							m.debug.fieldBps = append(m.debug.fieldBps[:i], m.debug.fieldBps[i+1:]...)
							m.pushSpecialBreakpoints()
							m.invalidate()
						}
					})
				}
			})
			ui.Row(c).Gap(8).Children(func() {
				ui.Button(c, "Remove All").OnClick(func() {
					m.RemoveAllBreakpoints()
					m.invalidate()
				})
				ui.Spacer(c)
				ui.Button(c, "Close").OnClick(func() {
					m.debug.breakpointsOpen = false
					m.invalidate()
				})
			})
		})
	})
	if d.Dismissed() || !open {
		m.debug.breakpointsOpen = false
	}
}
