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
		Category: "Debug",
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
	var onExit func(code *int32, stopped bool)
	if callback, ok := request["onExit"].(func(*int32, bool)); ok {
		onExit = callback
	}
	config, wrapped := request["config"]
	if !wrapped {
		config = request
	}
	raw, err := json.Marshal(config)
	if err != nil || len(raw) == 0 || string(raw) == "null" {
		m.finishDebugPluginCallback(onExit, nil, false)
		m.status = "Could not read the plugin debug configuration"
		m.invalidate()
		return
	}
	if m.ctx == nil {
		m.finishDebugPluginCallback(onExit, nil, false)
		m.status = "The plugin debugger is not connected"
		m.invalidate()
		return
	}
	if m.mgr == nil && m.invokeHook == nil {
		m.finishDebugPluginCallback(onExit, nil, false)
		m.status = "The plugin debugger is not connected"
		m.invalidate()
		return
	}
	root := m.Root()
	m.debug.pluginRequest++
	generation := m.debug.pluginRequest
	ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
	m.status = "Preparing plugin debug configuration…"
	m.invalidate()
	go func() {
		defer cancel()
		prepared, err := m.invoke(ctx, "plugin_prepare_debug_run", map[string]any{
			"config": json.RawMessage(raw),
		})
		m.post(func() {
			if !m.debugPluginRequestCurrent(generation, root) {
				m.finishDebugPluginCallback(onExit, nil, true)
				return
			}
			if err != nil {
				m.finishDebugPluginCallback(onExit, nil, false)
				m.status = "Could not prepare the plugin debug configuration: " + err.Error()
				m.invalidate()
				return
			}
			cfg, decodeErr := openidea.DecodeRunConfig(prepared)
			if decodeErr != nil {
				m.finishDebugPluginCallback(onExit, nil, false)
				m.status = "Could not read the prepared plugin debug configuration: " + decodeErr.Error()
				m.invalidate()
				return
			}
			if !m.canStartDebugWithWindow(root, false) {
				m.finishDebugPluginCallback(onExit, nil, false)
				return
			}
			m.debug.pluginExit = onExit
			m.debug.pluginStopped = false
			m.status = ""
			m.DebugRunConfig(cfg)
			if m.debug.pluginExit != nil &&
				!m.debug.starting &&
				m.debug.session == 0 &&
				m.debug.err != "" {
				m.finishPluginDebug(nil)
			}
		})
	}()
}

func (m *Model) debugPluginRequestCurrent(generation uint64, root string) bool {
	return generation == m.debug.pluginRequest && m.Root() == root
}

func (m *Model) finishDebugPluginCallback(
	callback func(code *int32, stopped bool),
	code *int32,
	stopped bool,
) {
	if callback != nil {
		callback(code, stopped)
	}
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
	open       bool
	busy       bool
	value      string
	err        string
	variable   openidea.Variable
	container  int64
	frameID    int64
	session    uint32
	generation uint64
}

type debugSmartStepState struct {
	open       bool
	loading    bool
	err        string
	targets    []openidea.StepInTarget
	session    uint32
	thread     int64
	frameID    int64
	generation uint64
}

type debugEvaluateState struct {
	open       bool
	busy       bool
	expression string
	err        string
	result     openidea.Variable
	hasResult  bool
	session    uint32
	frameID    int64
	generation uint64
}

// OpenDebugEvaluate opens OpenIDEA's Evaluate Expression dialog with the
// editor selection seeded as the expression.
func (m *Model) OpenDebugEvaluate() {
	d := &m.debug
	if d.session == 0 {
		d.err = "Start a debug session before evaluating an expression"
		m.invalidate()
		return
	}
	expression := ""
	if t := m.tab(); t != nil {
		expression = strings.TrimSpace(t.state.SelectedText())
	}
	d.evaluate = debugEvaluateState{
		open:       true,
		expression: expression,
		session:    d.session,
		frameID:    d.frameID,
		generation: d.stopGeneration,
	}
	m.invalidate()
}

func (m *Model) commitDebugEvaluate() {
	v := &m.debug.evaluate
	if !v.open || v.busy {
		return
	}
	expression := strings.TrimSpace(v.expression)
	if expression == "" {
		v.err = "Enter an expression"
		m.invalidate()
		return
	}
	if m.debug.session == 0 || !m.debug.paused {
		v.err = "Pause the program before evaluating an expression"
		m.invalidate()
		return
	}
	session, frameID := m.debug.session, m.debug.frameID
	generation := m.debug.stopGeneration
	v.session, v.frameID, v.generation = session, frameID, generation
	v.busy, v.err, v.hasResult = true, "", false
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		args := map[string]any{
			"session": session, "expression": expression,
		}
		if frameID != 0 {
			args["frameId"] = frameID
		}
		raw, err := m.invoke(ctx, "debug_evaluate", args)
		var result openidea.Variable
		if err == nil {
			err = json.Unmarshal(raw, &result)
		}
		m.post(func() {
			m.updateDebugSession(session, func(state *debugSessionState) {
				if state.evaluate.session != session ||
					state.evaluate.generation != generation {
					return
				}
				state.evaluate.busy = false
				if state.stopGeneration != generation || state.frameID != frameID {
					state.evaluate.err = "The selected frame changed before evaluation finished"
					return
				}
				if err != nil {
					state.evaluate.err = "Evaluation failed: " + err.Error()
					return
				}
				state.evaluate.result = result
				state.evaluate.hasResult = true
			})
			m.invalidate()
		})
	}()
}

func (m *Model) debugEvaluateDialog(c *ui.Context, p *ui.Theme) {
	v := &m.debug.evaluate
	open := v.open
	d := nativeModal(c.Key("debug-evaluate"), &open, func() {
		ui.Column(c).Width(620).Gap(9).Children(func() {
			ui.Text(c, "Evaluate").FontSize(13).Bold().TextColor(p.Text)
			ui.Text(c, "Expression").FontSize(12).TextColor(p.TextMuted)
			ui.TextArea(c.Key("debug-evaluate-expression"), &v.expression).
				Height(84).Font("monospace").FontSize(12).AutoFocus()
			switch {
			case v.err != "":
				ui.Text(c, v.err).FontSize(12).TextColor(p.Danger)
			case v.hasResult:
				ui.Text(c, v.result.Name+" = "+v.result.Value).
					Font("monospace").FontSize(12).TextColor(p.Text)
			default:
				ui.Text(c, "Evaluation is available while the program is suspended.").
					FontSize(12).TextColor(p.TextMuted)
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Button(c, "Close").OnClick(func() {
					v.open = false
					m.invalidate()
				})
				ui.Button(c, "Add to Watches").Disabled(strings.TrimSpace(v.expression) == "").
					OnClick(func() {
						if m.addWatch(v.expression) {
							v.open = false
						}
					})
				ui.Spacer(c)
				ui.Button(c, "Evaluate").
					Disabled(v.busy || m.debug.session == 0 || !m.debug.paused ||
						strings.TrimSpace(v.expression) == "").
					OnClick(m.commitDebugEvaluate)
			})
		})
	})
	if d.Dismissed() || !open {
		v.open = false
	}
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
	if !m.debugSessionSupports(d.session, func(caps openidea.DebugCapabilities) bool {
		return caps.SetValue
	}) {
		d.err = "This debugger does not support changing values"
		m.invalidate()
		return
	}
	i := d.selVar
	if i < 0 || i >= len(d.vars) {
		i = 0
	}
	v := d.vars[i]
	d.setValue = debugSetValueState{
		open:       true,
		value:      v.Value,
		variable:   v,
		container:  d.varContainer,
		frameID:    d.frameID,
		session:    d.session,
		generation: d.stopGeneration,
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
	generation, frameID := v.generation, v.frameID
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "debug_set_variable", args)
		var updated openidea.Variable
		if err == nil {
			err = json.Unmarshal(raw, &updated)
		}
		m.post(func() {
			m.updateDebugSession(session, func(state *debugSessionState) {
				if state.session != session || state.stopGeneration != generation ||
					state.frameID != frameID {
					return
				}
				state.setValue.busy = false
				if err != nil {
					state.setValue.err = err.Error()
					return
				}
				for i := range state.vars {
					if state.vars[i].Name == name {
						state.vars[i] = updated
						break
					}
				}
				state.setValue.open = false
			})
			if m.debug.active == session {
				m.evaluateWatches()
			}
			m.invalidate()
		})
	}()
}

func (m *Model) debugSetValueDialog(c *ui.Context, p *ui.Theme) {
	v := &m.debug.setValue
	open := v.open
	d := nativeModal(c.Key("debug-set-value"), &open, func() {
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

func (m *Model) debugResetFrameDisabledReason() string {
	return m.debugResetFrameDisabledReasonAt(m.debug.selFrame)
}

func (m *Model) debugResetFrameDisabledReasonAt(frame int) string {
	d := &m.debug
	switch {
	case d.session == 0 || d.ended:
		return "No live debug session can reset a frame"
	case !d.paused:
		return "Pause before resetting a frame"
	case !d.capsKnown:
		return "Reading the debugger capabilities"
	case !d.caps.RestartFrame:
		return "This debugger does not support Reset Frame"
	case len(d.frames) == 0:
		return "No stack frame is available"
	case frame < 0 || frame >= len(d.frames):
		return "Select a stack frame first"
	}
	if frame == len(d.frames)-1 {
		return "The bottom stack frame cannot be reset"
	}
	if strings.Contains(d.frames[frame].Name, "[native method]") ||
		frame+1 < len(d.frames) && strings.Contains(d.frames[frame+1].Name, "[native method]") {
		return "A frame involving a native caller cannot be reset"
	}
	return ""
}

// DebugResetFrame returns the selected frame to the start of its method.
func (m *Model) DebugResetFrame() {
	d := &m.debug
	if reason := m.debugResetFrameDisabledReason(); reason != "" {
		d.err = reason
		m.invalidate()
		return
	}
	session, frame := d.session, d.frames[d.selFrame]
	generation := d.stopGeneration
	d.stopGeneration++
	d.threadRequest++
	d.frameRequest++
	d.status = "Resetting frame…"
	d.paused = false
	d.setValue.open = false
	d.smartStep.open = false
	d.rememberDebugSession()
	m.refreshDecorations()
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		_, err := m.invoke(ctx, "debug_restart_frame", map[string]any{
			"session": session, "frameId": frame.ID,
		})
		m.post(func() {
			m.updateDebugSession(session, func(state *debugSessionState) {
				if state.session != session || state.stopGeneration != generation+1 {
					return
				}
				if err != nil {
					state.paused = true
					state.err = "Reset Frame failed: " + err.Error()
					return
				}
				state.status = "Frame reset"
			})
			if m.debug.active == session {
				m.refreshDecorations()
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
	generation := d.stopGeneration
	d.smartStep = debugSmartStepState{
		open: true, loading: true, session: session, thread: thread,
		frameID: frame.ID, generation: generation,
	}
	d.rememberDebugSession()
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
			accepted := false
			m.updateDebugSession(session, func(state *debugSessionState) {
				if state.session != session || !state.paused ||
					state.stopGeneration != generation || state.stopThread != thread ||
					state.frameID != frame.ID {
					return
				}
				accepted = true
				state.smartStep.loading = false
				if err != nil {
					state.err = "Smart Step Into failed: " + err.Error()
					state.smartStep.open = false
					return
				}
				state.smartStep.targets = targets
			})
			if !accepted {
				return
			}
			if err != nil {
				m.invalidate()
				return
			}
			switch len(targets) {
			case 0:
				m.debugStepInSession(session, thread, frame.ID, generation)
			case 1:
				m.stepIntoDebugTargetForSession(session, thread, frame.ID, generation, targets[0].ID)
			default:
				m.invalidate()
			}
		})
	}()
}

func (m *Model) stepIntoDebugTarget(targetID int64) {
	smart := m.debug.smartStep
	if smart.session == 0 || smart.thread == 0 || smart.frameID == 0 || targetID == 0 {
		return
	}
	m.stepIntoDebugTargetForSession(
		smart.session, smart.thread, smart.frameID, smart.generation, targetID,
	)
}

func (m *Model) stepIntoDebugTargetForSession(
	session uint32,
	thread int64,
	frameID int64,
	generation uint64,
	targetID int64,
) {
	var expectedGeneration uint64
	accepted := m.updateDebugSession(session, func(state *debugSessionState) {
		if state.session != session || !state.paused || state.ended ||
			state.stopThread != thread || state.frameID != frameID ||
			state.stopGeneration != generation {
			return
		}
		state.stopGeneration++
		state.threadRequest++
		state.frameRequest++
		state.smartStep = debugSmartStepState{}
		state.status = "Smart stepping…"
		state.paused = false
		expectedGeneration = state.stopGeneration
	})
	if !accepted || expectedGeneration == 0 {
		return
	}
	if m.debug.active == session {
		m.refreshDecorations()
	}
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		_, err := m.invoke(ctx, "debug_step_in_target", map[string]any{
			"session": session, "threadId": thread, "targetId": targetID,
		})
		m.post(func() {
			if err != nil {
				m.updateDebugSession(session, func(state *debugSessionState) {
					if state.session != session || state.ended ||
						state.stopGeneration != expectedGeneration {
						return
					}
					state.paused = true
					state.err = "Smart Step Into failed: " + err.Error()
				})
			}
			if m.debug.active == session {
				m.refreshDecorations()
			}
			m.invalidate()
		})
	}()
}

func (m *Model) debugStepInSession(
	session uint32,
	thread int64,
	frameID int64,
	generation uint64,
) {
	var expectedGeneration uint64
	accepted := m.updateDebugSession(session, func(state *debugSessionState) {
		if state.session != session || !state.paused || state.ended ||
			state.stopThread != thread || state.frameID != frameID ||
			state.stopGeneration != generation {
			return
		}
		state.stopGeneration++
		state.threadRequest++
		state.frameRequest++
		state.smartStep = debugSmartStepState{}
		state.status = "Stepping…"
		state.paused = false
		expectedGeneration = state.stopGeneration
	})
	if !accepted || expectedGeneration == 0 {
		return
	}
	if m.debug.active == session {
		m.refreshDecorations()
	}
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		_, err := m.invoke(ctx, "debug_step", map[string]any{
			"session": session, "threadId": thread, "kind": "into",
		})
		m.post(func() {
			if err != nil {
				m.updateDebugSession(session, func(state *debugSessionState) {
					if state.session != session || state.ended ||
						state.stopGeneration != expectedGeneration {
						return
					}
					state.paused = true
					state.err = "Step failed: " + err.Error()
				})
			}
			if m.debug.active == session {
				m.refreshDecorations()
			}
			m.invalidate()
		})
	}()
}

func (m *Model) debugSmartStepDialog(c *ui.Context, p *ui.Theme) {
	v := &m.debug.smartStep
	open := v.open
	d := nativeModal(c.Key("debug-smart-step"), &open, func() {
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

func (m *Model) debugDartSessionRunning() bool {
	d := &m.debug
	return d.session != 0 && !d.ended && !d.starting && d.adapter == "dart"
}

func (m *Model) debugHotApplicationAction(command, started, completed, failed string) {
	d := &m.debug
	if !m.debugDartSessionRunning() {
		d.err = "Hot Reload and Hot Restart are available for a running Dart or Flutter debug session"
		m.invalidate()
		return
	}
	session := d.session
	d.err = ""
	d.status = started
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
		defer cancel()
		_, err := m.invoke(ctx, command, map[string]any{"session": session})
		m.post(func() {
			if m.debug.session != session {
				return
			}
			if err != nil {
				m.debug.err = failed + ": " + err.Error()
			} else {
				m.debug.status = completed
			}
			m.invalidate()
		})
	}()
}

// DebugHotReload injects changed Dart or Flutter sources into the running app.
func (m *Model) DebugHotReload() {
	m.debugHotApplicationAction(
		"debug_hot_reload",
		"Reloading the application…",
		"Application reloaded",
		"Hot Reload failed",
	)
}

// DebugHotRestart restarts a Dart or Flutter app while keeping its debugger.
func (m *Model) DebugHotRestart() {
	m.debugHotApplicationAction(
		"debug_hot_restart",
		"Restarting the application…",
		"Application restarted",
		"Hot Restart failed",
	)
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
	d := nativeModal(c.Key("debug-console-history"), &open, func() {
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
		m.debug.removeBreakpointOptions(path, line)
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
	m.debug.removeBreakpointOptions(path, line)
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

type debugBreakpointEditState struct {
	open         bool
	kind         string
	path         string
	line         int
	name         string
	enabled      bool
	condition    string
	hitCondition string
	logMessage   string
	err          string
}

func (m *Model) editDebugLineBreakpoint(path string, line int) {
	options := m.debug.breakpointOptionsAt(path, line)
	m.debug.breakpointsOpen = false
	m.debug.breakpointEdit = debugBreakpointEditState{
		open:         true,
		kind:         "line",
		path:         path,
		line:         line,
		enabled:      options.Enabled,
		condition:    options.Condition,
		hitCondition: options.HitCondition,
		logMessage:   options.LogMessage,
	}
	m.invalidate()
}

func (m *Model) editDebugMethodBreakpoint(name string) {
	for _, bp := range m.debug.methodBps {
		if bp.Name != name {
			continue
		}
		m.debug.breakpointsOpen = false
		m.debug.breakpointEdit = debugBreakpointEditState{
			open:         true,
			kind:         "method",
			name:         name,
			enabled:      bp.Enabled,
			condition:    debugStringValue(bp.Condition),
			hitCondition: debugStringValue(bp.HitCondition),
		}
		m.invalidate()
		return
	}
}

func (m *Model) editDebugFieldBreakpoint(dataID string) {
	for _, bp := range m.debug.fieldBps {
		if bp.DataID != dataID {
			continue
		}
		m.debug.breakpointsOpen = false
		m.debug.breakpointEdit = debugBreakpointEditState{
			open:         true,
			kind:         "data",
			name:         dataID,
			enabled:      bp.Enabled,
			condition:    debugStringValue(bp.Condition),
			hitCondition: debugStringValue(bp.HitCondition),
		}
		m.invalidate()
		return
	}
}

func debugStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (m *Model) saveDebugBreakpointEditor() {
	v := &m.debug.breakpointEdit
	if !v.open {
		return
	}
	switch v.kind {
	case "line":
		if !containsInt(m.debug.bps[v.path], v.line) {
			v.err = "This breakpoint no longer exists"
			m.invalidate()
			return
		}
		m.debug.setBreakpointOptions(v.path, v.line, debugBreakpointOptions{
			Enabled:      v.enabled,
			Condition:    strings.TrimSpace(v.condition),
			HitCondition: strings.TrimSpace(v.hitCondition),
			LogMessage:   strings.TrimSpace(v.logMessage),
		})
		m.pushBreakpointsAsync(v.path)
	case "method":
		found := false
		for i := range m.debug.methodBps {
			bp := &m.debug.methodBps[i]
			if bp.Name != v.name {
				continue
			}
			bp.Enabled = v.enabled
			bp.Condition = breakpointString(v.condition)
			bp.HitCondition = breakpointString(v.hitCondition)
			found = true
			break
		}
		if !found {
			v.err = "This method breakpoint no longer exists"
			m.invalidate()
			return
		}
		m.pushSpecialBreakpoints()
	case "data":
		found := false
		for i := range m.debug.fieldBps {
			bp := &m.debug.fieldBps[i]
			if bp.DataID != v.name {
				continue
			}
			bp.Enabled = v.enabled
			bp.Condition = breakpointString(v.condition)
			bp.HitCondition = breakpointString(v.hitCondition)
			found = true
			break
		}
		if !found {
			v.err = "This field watchpoint no longer exists"
			m.invalidate()
			return
		}
		m.pushSpecialBreakpoints()
	default:
		v.err = "This breakpoint cannot be edited"
		m.invalidate()
		return
	}
	v.open = false
	m.debug.breakpointsOpen = true
	m.invalidate()
}

func (m *Model) debugBreakpointEditorDialog(c *ui.Context, p *ui.Theme) {
	v := &m.debug.breakpointEdit
	open := v.open
	d := nativeModal(c.Key("debug-breakpoint-editor"), &open, func() {
		ui.Column(c).Width(560).Gap(9).Children(func() {
			title := "Breakpoint"
			switch v.kind {
			case "method":
				title = "Method Breakpoint"
			case "data":
				title = "Field Watchpoint"
			}
			ui.Text(c, title).FontSize(13).Bold().TextColor(p.Text)
			if v.name != "" {
				ui.Text(c, v.name).FontSize(12).TextColor(p.TextMuted).SingleLine().Ellipsis("…")
			}
			ui.Checkbox(c, &v.enabled, "Enabled")
			ui.Text(c, "Condition").FontSize(12).TextColor(p.TextMuted)
			ui.TextInput(c.Key("debug-breakpoint-condition"), &v.condition).
				Placeholder("Evaluate to true before stopping").
				Font("monospace").FontSize(12)
			ui.Text(c, "Hit count").FontSize(12).TextColor(p.TextMuted)
			ui.TextInput(c.Key("debug-breakpoint-hit-condition"), &v.hitCondition).
				Placeholder("For example: 3, >= 5").
				Font("monospace").FontSize(12)
			if v.kind == "line" {
				ui.Text(c, "Log message").FontSize(12).TextColor(p.TextMuted)
				ui.TextInput(c.Key("debug-breakpoint-log-message"), &v.logMessage).
					Placeholder("Log text with {expression} values and continue").
					Font("monospace").FontSize(12)
			}
			if v.err != "" {
				ui.Text(c, v.err).FontSize(12).TextColor(p.Danger)
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Button(c, "Cancel").OnClick(func() {
					v.open = false
					m.debug.breakpointsOpen = true
					m.invalidate()
				})
				ui.Spacer(c)
				ui.Button(c, "Apply").OnClick(m.saveDebugBreakpointEditor)
			})
		})
	})
	if d.Dismissed() || !open {
		v.open = false
		m.debug.breakpointsOpen = true
	}
}

func debugLineBreakpointState(options debugBreakpointOptions) string {
	state := make([]string, 0, 3)
	if !options.Enabled {
		state = append(state, "Disabled")
	}
	if strings.TrimSpace(options.Condition) != "" {
		state = append(state, "Conditional")
	}
	if strings.TrimSpace(options.HitCondition) != "" {
		state = append(state, "Hit count")
	}
	if strings.TrimSpace(options.LogMessage) != "" {
		state = append(state, "Log")
	}
	return strings.Join(state, " · ")
}

func debugSpecialBreakpointState(bp openidea.FunctionBreakpoint) string {
	state := make([]string, 0, 3)
	if !bp.Enabled {
		state = append(state, "Disabled")
	}
	if bp.Condition != nil && strings.TrimSpace(*bp.Condition) != "" {
		state = append(state, "Conditional")
	}
	if bp.HitCondition != nil && strings.TrimSpace(*bp.HitCondition) != "" {
		state = append(state, "Hit count")
	}
	return strings.Join(state, " · ")
}

func debugDataBreakpointState(bp openidea.DataBreakpoint) string {
	state := make([]string, 0, 3)
	if !bp.Enabled {
		state = append(state, "Disabled")
	}
	if bp.Condition != nil && strings.TrimSpace(*bp.Condition) != "" {
		state = append(state, "Conditional")
	}
	if bp.HitCondition != nil && strings.TrimSpace(*bp.HitCondition) != "" {
		state = append(state, "Hit count")
	}
	return strings.Join(state, " · ")
}

func (m *Model) debugBreakpointsDialog(c *ui.Context, p *ui.Theme) {
	open := m.debug.breakpointsOpen
	d := nativeModal(c.Key("debug-breakpoints"), &open, func() {
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
							options := m.debug.breakpointOptionsAt(path, line)
							if containsInt(m.debug.temporaryBps[path], line) {
								label = "Temporary · " + label
							}
							ui.Text(c, fmt.Sprintf("%s:%d", label, line+1)).Grow(1).FontSize(12).
								TextColor(p.Text).SingleLine().Ellipsis("…")
							if state := debugLineBreakpointState(options); state != "" {
								ui.Text(c, state).Shrink(0).FontSize(11).TextColor(p.TextMuted)
							}
							if ui.Button(c, "Edit").Clicked() {
								m.editDebugLineBreakpoint(path, line)
							}
							if ui.Button(c, "Remove").Clicked() {
								m.debug.bps[path] = removeInt(m.debug.bps[path], line)
								m.debug.temporaryBps[path] = removeInt(m.debug.temporaryBps[path], line)
								m.debug.removeBreakpointOptions(path, line)
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
						if state := debugSpecialBreakpointState(bp); state != "" {
							ui.Text(c, state).Shrink(0).FontSize(11).TextColor(p.TextMuted)
						}
						if ui.Button(c, "Edit").Clicked() {
							m.editDebugMethodBreakpoint(bp.Name)
						}
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
						if state := debugDataBreakpointState(bp); state != "" {
							ui.Text(c, state).Shrink(0).FontSize(11).TextColor(p.TextMuted)
						}
						if ui.Button(c, "Edit").Clicked() {
							m.editDebugFieldBreakpoint(bp.DataID)
						}
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
