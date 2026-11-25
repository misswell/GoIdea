package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"mayoi/internal/openidea"
)

// BuildPane is the Build tool window: the console of the build started from
// the Build actions, its result, and the request that Stop Build cancels.
// Compiler messages reach the Problems view and the editors as `build`
// diagnostics, published by the backend; the console shows commands and output.
type BuildPane struct {
	visible bool

	kind    string
	busy    bool
	request uint32
	channel uint32
	taskID  string

	title    string
	system   string
	progress string
	success  *bool
	errors   uint32
	warnings uint32
	duration float64
	// files are the files that carried build diagnostics, cleared by the next
	// build so stale problems do not outlive their sources.
	files []string
	err   string

	output    []outputLine
	truncated bool
	// messages is OpenIDEA's compiler-message tree beside the console.
	messages        []openidea.BuildMessage
	selectedMessage int
}

// maxBuildLines bounds the console; Maven and Gradle can print more than
// anyone reads, and an unbounded log would grow with the build.
const maxBuildLines = 20000
const maxBuildMessages = 4000

// resetBuild drops the previous project's build state.
func (m *Model) resetBuild() { m.build = BuildPane{} }

// ShowBuild toggles the Build tool window.
func (m *Model) ShowBuild() { m.dockActivate("build") }

// BuildProject compiles the project's sources and tests.
func (m *Model) BuildProject() { m.startBuild("build", nil) }

// BuildModule compiles the module that owns the active file.
func (m *Model) BuildModule() { m.startBuild("build", m.activeFileOrNil()) }

// CompileFile compiles the active Java source without building its module.
func (m *Model) CompileFile() {
	t := m.tab()
	if t == nil || t.kind != "" || t.doc == nil {
		return
	}
	m.CompileFilePath(t.doc.Path())
}

// CompileFilePath compiles the Java source selected in a context menu.
func (m *Model) CompileFilePath(path string) {
	if !m.canCompileFile(path) {
		return
	}
	m.startBuild("compileFile", &path)
}

// RebuildProject cleans the outputs, then builds the project.
func (m *Model) RebuildProject() { m.startBuild("rebuild", nil) }

// BuildArtifacts packages the active file's module, or the whole project.
func (m *Model) BuildArtifacts() { m.startBuild("artifacts", m.activeFileOrNil()) }

func buildActionAvailable(m *Model) bool {
	return m.Root() != "" && !m.showingWelcome && !m.build.busy
}

func (m *Model) activeFileOrNil() *string {
	t := m.tab()
	if t == nil || t.doc == nil {
		return nil
	}
	path := t.doc.Path()
	return &path
}

// buildActionOptions is OpenIDEA's action argument for the Build menu. A
// project-view string is a module target, while the before-launch callers pass
// {target, activate, wait}; CMake passes its detected build id too.
type buildActionOptions struct {
	Target   string
	Activate bool
	Wait     bool
	CMake    *string
}

func buildActionOptionsFromArg(arg any) buildActionOptions {
	options := buildActionOptions{Activate: true}
	if target, ok := arg.(string); ok {
		options.Target = strings.TrimSpace(target)
		return options
	}
	if arg == nil {
		return options
	}

	var object map[string]any
	switch value := arg.(type) {
	case map[string]any:
		object = value
	case map[string]string:
		object = make(map[string]any, len(value))
		for key, item := range value {
			object[key] = item
		}
	case []byte:
		if err := json.Unmarshal(value, &object); err != nil {
			return options
		}
	case json.RawMessage:
		if err := json.Unmarshal(value, &object); err != nil {
			return options
		}
	default:
		raw, err := json.Marshal(arg)
		if err != nil || json.Unmarshal(raw, &object) != nil {
			return options
		}
	}
	if target, ok := object["target"].(string); ok {
		options.Target = strings.TrimSpace(target)
	}
	if activate, ok := object["activate"].(bool); ok {
		options.Activate = activate
	}
	if wait, ok := object["wait"].(bool); ok {
		options.Wait = wait
	}
	if cmake, ok := object["cmake"].(string); ok && strings.TrimSpace(cmake) != "" {
		cmake = strings.TrimSpace(cmake)
		options.CMake = &cmake
	}
	return options
}

func (m *Model) buildTargetForAction(options buildActionOptions) *string {
	if options.Target != "" {
		target := options.Target
		return &target
	}
	return m.activeFileOrNil()
}

func (m *Model) runBuildProjectAction(arg any) {
	options := buildActionOptionsFromArg(arg)
	m.startBuildOperation("build", nil, options.CMake, options.Activate)
}

func (m *Model) runBuildModuleAction(arg any) {
	options := buildActionOptionsFromArg(arg)
	m.startBuildOperation("build", m.buildTargetForAction(options), nil, options.Activate)
}

func (m *Model) runCompileFileAction(arg any) {
	options := buildActionOptionsFromArg(arg)
	target := m.buildTargetForAction(options)
	if target == nil || !m.canCompileFile(*target) {
		return
	}
	m.startBuildOperation("compileFile", target, nil, options.Activate)
}

func (m *Model) runRebuildProjectAction(arg any) {
	options := buildActionOptionsFromArg(arg)
	// Rebuild Project always activates the Build window upstream.
	m.startBuildOperation("rebuild", nil, options.CMake, true)
}

func (m *Model) runBuildArtifactsAction(arg any) {
	options := buildActionOptionsFromArg(arg)
	m.startBuildOperation("artifacts", m.buildTargetForAction(options), nil, options.Activate)
}

// beforeLaunchBuildAction maps the persisted Before Launch task back to the
// OpenIDEA action that owns it. All of these calls are awaitable, so the main
// configuration waits for a successful result before it starts.
func beforeLaunchBuildAction(
	kind string,
	file *string,
	cmake *string,
	activate bool,
) (string, map[string]any, error) {
	arg := map[string]any{"activate": activate, "wait": true}
	switch kind {
	case "build":
		if cmake != nil {
			arg["cmake"] = *cmake
		}
		if file != nil {
			arg["target"] = *file
			return "build.buildModule", arg, nil
		}
		return "build.buildProject", arg, nil
	case "artifacts":
		if file != nil {
			arg["target"] = *file
		}
		return "build.buildArtifacts", arg, nil
	default:
		return "", nil, fmt.Errorf("Unknown Before launch build kind %q", kind)
	}
}

type buildActionRun struct {
	channel uint32
	request uint32
	taskID  string
}

// runBuildActionWait is the awaitable form of a build action. The regular
// RunWithArg path stays asynchronous, while Before Launch uses this path only
// when the action requested wait=true.
func (m *Model) runBuildActionWait(ctx context.Context, id string, arg any) error {
	options := buildActionOptionsFromArg(arg)
	if !options.Wait {
		return errors.New("Build action did not request a wait")
	}

	type startResult struct {
		run buildActionRun
		err error
	}
	started := make(chan startResult, 1)
	m.post(func() {
		select {
		case <-ctx.Done():
			started <- startResult{err: ctx.Err()}
			return
		default:
		}
		a, ok := actionGet(id)
		if !ok {
			started <- startResult{err: fmt.Errorf("Build action %q is not registered", id)}
			return
		}
		if !a.usable(m) {
			err := errors.New("Build did not start")
			if m.build.busy {
				err = errors.New("Another build is already running")
			}
			started <- startResult{err: err}
			return
		}
		if !m.RunActionWithArg(id, arg) {
			started <- startResult{err: errors.New("Build did not start")}
			return
		}
		if !m.build.busy {
			err := errors.New("Build did not start")
			if m.build.err != "" {
				err = errors.New(m.build.err)
			}
			started <- startResult{err: err}
			return
		}
		started <- startResult{run: buildActionRun{
			channel: m.build.channel,
			request: m.build.request,
			taskID:  m.build.taskID,
		}}
	})

	var start startResult
	select {
	case start = <-started:
	case <-ctx.Done():
		return ctx.Err()
	}
	if start.err != nil {
		return start.err
	}
	return m.waitBuildAction(ctx, start.run)
}

func (m *Model) waitBuildAction(ctx context.Context, run buildActionRun) error {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		var busy bool
		var success *bool
		var errText string
		var current uint32
		var errorCount uint32
		m.post(func() {
			busy = m.build.busy
			success = m.build.success
			errText = m.build.err
			current = m.build.channel
			errorCount = m.build.errors
		})
		if current != run.channel {
			return errors.New("Build was replaced before it finished")
		}
		if !busy {
			if errText != "" {
				return errors.New(errText)
			}
			if success == nil {
				return errors.New("Build did not return a result")
			}
			if !*success {
				return fmt.Errorf("Build failed with %d error(s)", errorCount)
			}
			return nil
		}
		select {
		case <-ctx.Done():
			m.post(func() { m.cancelBuildRequest(run.request, run.channel, run.taskID) })
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// startBuild streams one build into the Build pane. The backend builds from
// disk, so unsaved text is written first; a second build while one runs is
// ignored, as the actions are disabled while building.
func (m *Model) startBuild(kind string, file *string) {
	m.startBuildOperation(kind, file, nil, true)
}

// startBuildOperation is the shared build entry point. Before-launch builds
// pass activate=false and, for a CMake target, the detected build id.
func (m *Model) startBuildOperation(kind string, file *string, cmake *string, activate bool) {
	if activate {
		m.build.visible = true
		m.occupyBottom("build")
	}
	if m.build.busy {
		m.invalidate()
		return
	}
	if !m.run.trusted {
		m.build.err = "An untrusted project does not build"
		m.trustAsk = true
		m.invalidate()
		return
	}
	tabs := m.allTabs()
	root := m.Root()
	req := m.nextRequest()
	ch := m.nextChannel()
	taskID := fmt.Sprintf("build:%d", req)
	clear := append([]string{}, m.build.files...)
	m.build.kind = kind
	m.build.busy = true
	m.build.request = req
	m.build.channel = ch
	m.build.taskID = taskID
	m.build.title = kindLabel(kind)
	m.build.system = ""
	m.build.progress = ""
	m.build.success = nil
	m.build.errors = 0
	m.build.warnings = 0
	m.build.duration = 0
	m.build.err = ""
	m.build.output = nil
	m.build.truncated = false
	m.build.messages = nil
	m.build.selectedMessage = 0
	m.status = "Building…"
	m.upsertTask(BackgroundTask{
		ID: taskID, Title: kindLabel(kind), Message: "Saving files…",
		Fraction: -1,
		Cancel: func() {
			m.cancelBuildRequest(req, ch, taskID)
		},
	})
	m.invalidate()

	command, args := buildInvocation(kind, file, req, ch, clear)
	if cmake != nil {
		args["cmake"] = *cmake
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Minute)
		defer cancel()
		if err := m.saveOpenTabs(ctx, tabs); err != nil {
			m.post(func() {
				if m.build.channel != ch || m.Root() != root {
					m.removeTask(taskID)
					return
				}
				m.build.busy = false
				m.build.err = "Saving before building failed: " + err.Error()
				m.finishBuildTask(taskID, "Saving before building failed")
				m.invalidate()
			})
			return
		}
		raw, err := m.invoke(ctx, command, args)
		var result openidea.BuildResult
		if err == nil {
			err = json.Unmarshal(raw, &result)
		}
		m.post(func() {
			if m.build.channel != ch || m.Root() != root {
				// A stale request must never finish the newer build's task.
				m.removeTask(taskID)
				return
			}
			m.build.busy = false
			if err != nil {
				m.build.err = "Build failed: " + err.Error()
				m.status = "The build request failed"
				m.DockShow("build")
				m.notifyError("build", "Build failed", notifySuffix(err.Error()))
				m.finishBuildTask(taskID, "Build request failed")
				m.invalidate()
				return
			}
			ok := result.Success
			m.build.success = &ok
			m.build.errors = result.Errors
			m.build.warnings = result.Warnings
			m.build.duration = result.DurationMS
			m.build.files = buildMessageFiles(result.Messages)
			m.build.messages = append([]openidea.BuildMessage(nil), result.Messages...)
			if over := len(m.build.messages) - maxBuildMessages; over > 0 {
				m.build.messages = append(m.build.messages[:0], m.build.messages[over:]...)
			}
			if len(m.build.messages) > 0 {
				m.build.selectedMessage = 0
			}
			if result.Title != "" {
				m.build.title = result.Title
			}
			if result.System != "" {
				m.build.system = result.System
			}
			m.status = buildSummary(result)
			m.finishBuildTask(taskID, buildSummary(result))
			show := notifyAction{label: "Show Build Output", run: func(m *Model) { m.DockShow("build") }}
			switch {
			case !ok:
				// A failed build reveals Output without taking the keyboard,
				// including before-launch builds that passed activate=false.
				m.DockShow("build")
				m.notifyError("build", "Build failed", buildSummary(result), show)
			case result.Warnings > 0:
				m.notifyWarning("build", "Build finished with warnings", buildSummary(result), show)
			default:
				m.notifyInfo("build", "Build finished", buildSummary(result), show)
			}
			m.invalidate()
		})
	}()
}

func (m *Model) finishBuildTask(id, message string) {
	title := m.build.title
	if title == "" {
		title = "Build"
	}
	m.upsertTask(BackgroundTask{
		ID: id, Title: title, Message: message, Fraction: 1, Done: true,
	})
}

func (m *Model) updateBuildTask(title, message string) {
	for i := range m.tasks {
		task := &m.tasks[i]
		if task.ID != m.build.taskID || task.Done {
			continue
		}
		if title != "" {
			task.Title = title
		}
		if message != "" && !task.Cancelling {
			task.Message = message
		}
		m.invalidate()
		return
	}
}

func (m *Model) cancelBuildRequest(request, channel uint32, taskID string) {
	if !m.build.busy || m.build.request != request || m.build.channel != channel {
		return
	}
	if m.mgr == nil && m.invokeHook == nil {
		m.build.err = "Stopping the build failed: the backend is unavailable"
		m.finishBuildTask(taskID, "Stopping the build failed")
		m.invalidate()
		return
	}
	m.status = "Stopping the build…"
	m.invalidate()
	base := m.ctx
	if base == nil {
		base = context.Background()
	}
	go func() {
		ctx, cancel := context.WithTimeout(base, 15*time.Second)
		defer cancel()
		if _, err := m.invoke(ctx, "cancel_request", map[string]any{"id": request}); err != nil {
			m.post(func() {
				if m.build.channel != channel {
					return
				}
				m.build.err = "Stopping the build failed: " + err.Error()
				m.status = "Stopping the build failed"
				m.finishBuildTask(taskID, "Stopping the build failed")
				m.invalidate()
			})
		}
	}()
}

// buildInvocation keeps the single-file RPC separate from module builds.
func buildInvocation(kind string, file *string, req, ch uint32, clear []string) (string, map[string]any) {
	args := map[string]any{
		"file": file, "request": req, "clear": clear,
		"onEvent": map[string]any{"$channel": ch},
	}
	if kind == "compileFile" {
		return "build_compile_file", args
	}
	args["kind"] = kind
	args["cmake"] = nil
	return "build_run", args
}

// StopBuild cancels the running build.
func (m *Model) StopBuild() {
	if !m.build.busy || m.build.request == 0 {
		return
	}
	for _, task := range m.tasks {
		if task.ID == m.build.taskID && !task.Done && task.Cancel != nil {
			m.cancelTask(task.ID)
			return
		}
	}
	m.cancelBuildRequest(m.build.request, m.build.channel, m.build.taskID)
}

// ClearBuild empties the console.
func (m *Model) ClearBuild() {
	m.build.output = nil
	m.build.truncated = false
	m.build.err = ""
	m.invalidate()
}

// acceptBuildEvent applies one streamed build event; it runs on the UI thread.
func (m *Model) acceptBuildEvent(data json.RawMessage) {
	var ev openidea.BuildEvent
	if json.Unmarshal(data, &ev) != nil {
		return
	}
	switch ev.Kind {
	case "started":
		m.build.title, m.build.system = ev.Title, ev.System
		m.updateBuildTask(ev.Title, "")
	case "step":
		m.build.append("system", ev.Command)
		m.updateBuildTask("", ev.Command)
	case "output":
		kind := "stdout"
		if ev.Stderr {
			kind = "stderr"
		}
		m.build.append(kind, ev.Text)
	case "message":
		// Compiler messages also arrive as build diagnostics for the editors;
		// the console keeps the command output, so they are counted, not echoed.
		var msg openidea.BuildMessage
		if json.Unmarshal(ev.Message, &msg) != nil {
			return
		}
		switch strings.ToLower(msg.Severity) {
		case "error":
			m.build.errors++
		case "warning":
			m.build.warnings++
		}
		m.build.messages = append(m.build.messages, msg)
		if over := len(m.build.messages) - maxBuildMessages; over > 0 {
			m.build.messages = append(m.build.messages[:0], m.build.messages[over:]...)
			if m.build.selectedMessage >= over {
				m.build.selectedMessage -= over
			} else {
				m.build.selectedMessage = 0
			}
		}
	case "progress":
		var text string
		if json.Unmarshal(ev.Message, &text) != nil || text == "" {
			return
		}
		m.build.progress = text
		m.status = text
		m.updateBuildTask("", text)
	}
	m.invalidate()
}

// append adds one or more newline-terminated lines to the console.
func (p *BuildPane) append(kind, text string) {
	text = strings.TrimSuffix(stripANSI(text), "\n")
	if text == "" {
		return
	}
	for _, line := range strings.Split(text, "\n") {
		p.output = append(p.output, outputLine{kind: kind, text: strings.TrimSuffix(line, "\r")})
	}
	if over := len(p.output) - maxBuildLines; over > 0 {
		p.output = append(p.output[:0], p.output[over:]...)
		p.truncated = true
	}
}

// renderText is the console as one string; empty lines keep their height.
func (p *BuildPane) renderText() string {
	var b strings.Builder
	for i, line := range p.output {
		if i > 0 {
			b.WriteByte('\n')
		}
		if line.text == "" {
			b.WriteByte(' ')
		} else {
			b.WriteString(line.text)
		}
	}
	return b.String()
}

func kindLabel(kind string) string {
	switch kind {
	case "build":
		return "Build"
	case "compileFile":
		return "Compile File"
	case "rebuild":
		return "Rebuild"
	case "artifacts":
		return "Build Artifacts"
	}
	return kind
}

// buildMessageFiles is the set of files with compiler messages.
func buildMessageFiles(messages []openidea.BuildMessage) []string {
	seen := make(map[string]bool, len(messages))
	files := make([]string, 0, len(messages))
	for _, msg := range messages {
		if msg.Path == nil || seen[*msg.Path] {
			continue
		}
		seen[*msg.Path] = true
		files = append(files, *msg.Path)
	}
	return files
}

// buildSummary is IDEA's finish line: success with the duration, or the
// failure with its error and warning counts.
func buildSummary(result openidea.BuildResult) string {
	duration := formatBuildDuration(result.DurationMS)
	if result.Success {
		if result.Warnings > 0 {
			return fmt.Sprintf("Build succeeded with %d warnings in %s", result.Warnings, duration)
		}
		return "Build succeeded in " + duration
	}
	if result.Errors == 0 {
		return "Build Failed"
	}
	if result.Warnings > 0 {
		return fmt.Sprintf("Build failed: %d errors, %d warnings", result.Errors, result.Warnings)
	}
	return fmt.Sprintf("Build failed: %d errors", result.Errors)
}

// formatBuildDuration renders `450 ms`, `3 s 120 ms` or `1 m 5 s`.
func formatBuildDuration(ms float64) string {
	total := int(ms + 0.5)
	if total < 1000 {
		return fmt.Sprintf("%d ms", total)
	}
	m := total / 60000
	s := (total % 60000) / 1000
	rest := total % 1000
	if m > 0 {
		if s > 0 {
			return fmt.Sprintf("%d m %d s", m, s)
		}
		return fmt.Sprintf("%d m", m)
	}
	if rest > 0 {
		return fmt.Sprintf("%d s %d ms", s, rest)
	}
	return fmt.Sprintf("%d s", s)
}

// SmokeBuild trusts the project and runs one incremental build offscreen: the
// check against a real Maven/Gradle build through the OpenIDEA engine.
func (m *Model) SmokeBuild(ctx context.Context) (map[string]any, error) {
	root := m.Root()
	if root == "" {
		return nil, errors.New("No project is open")
	}
	if _, err := m.mgr.Invoke(ctx, Window, "project_trust_set", map[string]any{
		"path": root, "trusted": true, "folder": true,
	}); err != nil {
		return nil, err
	}
	request := m.nextRequest()
	channel := m.nextChannel()
	m.post(func() {
		m.run.trusted = true
		m.build.visible = true
		m.occupyBottom("build")
		m.build.kind = "build"
		m.build.busy = true
		m.build.request = request
		m.build.channel = channel
		m.build.title = kindLabel("build")
		m.build.success = nil
		m.build.output = nil
		m.build.truncated = false
		m.build.err = ""
		m.invalidate()
	})
	args := map[string]any{
		"kind":    "build",
		"file":    nil,
		"cmake":   nil,
		"request": request,
		"clear":   []string{},
		"onEvent": map[string]any{"$channel": channel},
	}
	raw, err := m.mgr.Invoke(ctx, Window, "build_run", args)
	m.post(func() { m.build.busy = false })
	if err != nil {
		return nil, err
	}
	var result openidea.BuildResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	var console string
	var lines int
	m.post(func() {
		lines = len(m.build.output)
		start := lines - 400
		if start < 0 {
			start = 0
		}
		var b strings.Builder
		for i := start; i < lines; i++ {
			if i > start {
				b.WriteByte('\n')
			}
			b.WriteString(m.build.output[i].text)
		}
		console = b.String()
	})
	return map[string]any{
		"system":     result.System,
		"title":      result.Title,
		"success":    result.Success,
		"errors":     result.Errors,
		"warnings":   result.Warnings,
		"durationMs": result.DurationMS,
		"lines":      lines,
		"console":    console,
	}, nil
}

// SmokeCompileFile compiles the active Java editor through the real backend.
func (m *Model) SmokeCompileFile(ctx context.Context) (map[string]any, error) {
	var started bool
	m.post(func() {
		started = m.RunCommand("build.compileFile") && m.build.busy && m.build.kind == "compileFile"
	})
	if !started {
		return nil, errors.New("Compile File did not start")
	}
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()
	for {
		var done, success bool
		var lines int
		var detail string
		m.post(func() {
			done = !m.build.busy
			success = m.build.success != nil && *m.build.success
			lines, detail = len(m.build.output), m.build.err
		})
		if done {
			result := map[string]any{"success": success, "lines": lines}
			if detail != "" {
				return result, errors.New(detail)
			}
			return result, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-tick.C:
		}
	}
}
