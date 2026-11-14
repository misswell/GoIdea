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
}

// maxBuildLines bounds the console; Maven and Gradle can print more than
// anyone reads, and an unbounded log would grow with the build.
const maxBuildLines = 20000

// resetBuild drops the previous project's build state.
func (m *Model) resetBuild() { m.build = BuildPane{} }

// ShowBuild toggles the Build tool window.
func (m *Model) ShowBuild() { m.dockActivate("build") }

// BuildProject compiles the project's sources and tests.
func (m *Model) BuildProject() { m.startBuild("build", nil) }

// BuildModule compiles the module that owns the active file.
func (m *Model) BuildModule() { m.startBuild("build", m.activeFileOrNil()) }

// RebuildProject cleans the outputs, then builds the project.
func (m *Model) RebuildProject() { m.startBuild("rebuild", nil) }

// BuildArtifacts packages the active file's module, or the whole project.
func (m *Model) BuildArtifacts() { m.startBuild("artifacts", m.activeFileOrNil()) }

func (m *Model) activeFileOrNil() *string {
	t := m.tab()
	if t == nil {
		return nil
	}
	path := t.doc.Path()
	return &path
}

// startBuild streams one build into the Build pane. The backend builds from
// disk, so unsaved text is written first; a second build while one runs is
// ignored, as the actions are disabled while building.
func (m *Model) startBuild(kind string, file *string) {
	m.build.visible = true
	m.occupyBottom("build")
	if m.build.busy {
		m.invalidate()
		return
	}
	if !m.run.trusted {
		m.build.err = "未信任的项目不会构建"
		m.trustAsk = true
		m.invalidate()
		return
	}
	tabs := append([]*Tab(nil), m.tabs...)
	root := m.Root()
	req := m.nextRequest()
	ch := m.nextChannel()
	clear := append([]string(nil), m.build.files...)
	m.build.kind = kind
	m.build.busy = true
	m.build.request = req
	m.build.channel = ch
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
	m.status = "正在构建…"
	m.invalidate()

	args := map[string]any{
		"kind":    kind,
		"file":    file,
		"cmake":   nil,
		"request": req,
		"clear":   clear,
		"onEvent": map[string]any{"$channel": ch},
	}
	p := m.prefs.SaveFiles
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Minute)
		defer cancel()
		if err := saveAll(ctx, tabs, p); err != nil {
			m.post(func() {
				if m.build.channel != ch || m.Root() != root {
					return
				}
				m.build.busy = false
				m.build.err = "保存后再构建失败：" + err.Error()
				m.invalidate()
			})
			return
		}
		raw, err := m.mgr.Invoke(ctx, Window, "build_run", args)
		var result openidea.BuildResult
		if err == nil {
			err = json.Unmarshal(raw, &result)
		}
		m.post(func() {
			if m.build.channel != ch || m.Root() != root {
				return
			}
			m.build.busy = false
			if err != nil {
				m.build.err = "构建失败：" + err.Error()
				m.status = "构建请求失败"
				m.notifyError("build", "Build failed", notifySuffix(err.Error()))
				m.invalidate()
				return
			}
			ok := result.Success
			m.build.success = &ok
			m.build.errors = result.Errors
			m.build.warnings = result.Warnings
			m.build.duration = result.DurationMS
			m.build.files = buildMessageFiles(result.Messages)
			if result.Title != "" {
				m.build.title = result.Title
			}
			if result.System != "" {
				m.build.system = result.System
			}
			m.status = buildSummary(result)
			show := notifyAction{label: "Show Build Output", run: func(m *Model) { m.ShowBuild() }}
			switch {
			case !ok:
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

// StopBuild cancels the running build.
func (m *Model) StopBuild() {
	if !m.build.busy || m.build.request == 0 {
		return
	}
	id := m.build.request
	m.status = "正在停止构建…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
		defer cancel()
		if _, err := m.mgr.Invoke(ctx, Window, "cancel_request", map[string]any{"id": id}); err != nil {
			m.post(func() {
				m.build.err = "停止构建失败：" + err.Error()
				m.invalidate()
			})
		}
	}()
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
	case "step":
		m.build.append("system", ev.Command)
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
	case "progress":
		var text string
		if json.Unmarshal(ev.Message, &text) != nil || text == "" {
			return
		}
		m.build.progress = text
		m.status = text
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
		return "构建"
	case "rebuild":
		return "重新构建"
	case "artifacts":
		return "构建产物"
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
			return fmt.Sprintf("构建成功，%d 个警告，用时 %s", result.Warnings, duration)
		}
		return "构建成功，用时 " + duration
	}
	if result.Errors == 0 {
		return "构建失败"
	}
	if result.Warnings > 0 {
		return fmt.Sprintf("构建失败：%d 个错误，%d 个警告", result.Errors, result.Warnings)
	}
	return fmt.Sprintf("构建失败：%d 个错误", result.Errors)
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
		return nil, errors.New("没有打开项目")
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
