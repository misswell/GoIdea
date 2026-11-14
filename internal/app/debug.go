package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

// breakpointColor marks breakpoints in the gutter; stopLineColor fills the
// paused frame's line.
var (
	breakpointColor = ui.RGB(220, 80, 80)
	stopLineColor   = ui.RGBA(255, 200, 60, 0.16)
)

// DebugPane is the Debug tool window: one java-debug session (through jdtls),
// the paused thread's frames and variables, adapter output, and the line
// breakpoints the user set. Breakpoints live here, not in the session: they
// are given to every session that starts.
type DebugPane struct {
	visible  bool
	starting bool
	session  uint32 // backend DebugSessionId; 0 before the first start
	config   string // main class being debugged

	paused     bool
	ended      bool
	stopFile   string
	stopLine   int // 0-based; -1 when not paused
	stopThread int64
	reason     string

	threads  []openidea.ThreadInfo
	frames   []openidea.StackFrame
	selFrame int
	scopes   []openidea.Scope
	vars     []openidea.Variable
	frameID  int64

	output    []outputLine
	truncated bool
	err       string
	status    string

	// bps maps an absolute path to its 0-based breakpoint lines.
	bps map[string][]int

	// watches are expressions the user pinned; they evaluate again in each
	// stopped frame. evalText is the input being typed for a new one.
	watches  []debugWatch
	evalText string
	evalBusy bool
}

// debugWatch is one pinned expression and what it evaluated to in the current
// stopped frame.
type debugWatch struct {
	Expr  string
	Value string
	Err   string
}

// maxWatches bounds the panel; a frame with more expressions to inspect is
// better served by adding them one at a time.
const maxWatches = 20

const maxDebugLines = 2000

// resetDebug drops the previous project's debug state, breakpoints included:
// a line number means nothing in another project.
func (m *Model) resetDebug() {
	m.debug = DebugPane{stopLine: -1}
}

// breakpointLines returns a copy of path's lines, sorted.
func (p *DebugPane) breakpointLines(path string) []int {
	lines := append([]int(nil), p.bps[path]...)
	sort.Ints(lines)
	return lines
}

// toggleBreakpointAt adds or removes a breakpoint on a 0-based line, then
// gives the file's whole list to the backend (which keeps it for sessions).
func (m *Model) toggleBreakpointAt(path string, line int) {
	if line < 0 || !isJavaSource(path) {
		m.status = "断点只支持 Java 源文件"
		m.invalidate()
		return
	}
	if m.debug.bps == nil {
		m.debug.bps = map[string][]int{}
	}
	lines := m.debug.bps[path]
	next, found := toggledLines(lines, line)
	if len(next) == 0 {
		delete(m.debug.bps, path)
	} else {
		m.debug.bps[path] = next
	}
	if found {
		m.status = fmt.Sprintf("已移除断点：第 %d 行", line+1)
	} else {
		m.status = fmt.Sprintf("已设置断点：第 %d 行", line+1)
	}
	m.refreshDecorations()
	m.invalidate()
	m.pushBreakpointsAsync(path)
}

// ToggleBreakpoint toggles a breakpoint on the active editor's caret line.
func (m *Model) ToggleBreakpoint() {
	t := m.tab()
	if t == nil {
		return
	}
	m.toggleBreakpointAt(t.doc.Path(), t.state.CursorPos().Line)
}

// toggledLines adds line to a sorted list, or removes it; found reports the
// removal.
func toggledLines(lines []int, line int) ([]int, bool) {
	out := make([]int, 0, len(lines)+1)
	found := false
	for _, l := range lines {
		if l == line {
			found = true
			continue
		}
		out = append(out, l)
	}
	if !found {
		out = append(out, line)
	}
	sort.Ints(out)
	return out, found
}

// isJavaSource reports whether path is a Java source the debugger can break in.
func isJavaSource(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ".java")
}

// pushBreakpointsAsync sends one file's breakpoints to the debugger.
func (m *Model) pushBreakpointsAsync(path string) {
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		_ = m.pushBreakpoints(ctx, path)
	}()
}

// pushBreakpoints sends a file's breakpoints, so a session that starts later
// has them too. An empty path sends every file that has some.
func (m *Model) pushBreakpoints(ctx context.Context, path string) error {
	if path != "" {
		return m.pushFileBreakpoints(ctx, path)
	}
	paths := make([]string, 0, len(m.debug.bps))
	for p := range m.debug.bps {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var first error
	for _, p := range paths {
		if err := m.pushFileBreakpoints(ctx, p); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (m *Model) pushFileBreakpoints(ctx context.Context, path string) error {
	var list []openidea.Breakpoint
	for _, line := range m.debug.breakpointLines(path) {
		list = append(list, openidea.Breakpoint{Line: line, Enabled: true})
	}
	if list == nil {
		list = []openidea.Breakpoint{}
	}
	_, err := m.mgr.Invoke(ctx, Window, "debug_set_breakpoints", map[string]any{
		"path": path, "breakpoints": list,
	})
	return err
}

// refreshDecorations paints breakpoints and Git changes in the gutter, and the
// paused line behind the code of every open tab.
func (m *Model) refreshDecorations() {
	for _, t := range m.tabs {
		path := t.doc.Path()
		lines := m.debug.breakpointLines(path)
		bps := make(map[int]bool, len(lines))
		marks := make([]editor.GutterMark, 0, len(lines)+len(t.gitMarks))
		for _, l := range lines {
			bps[l] = true
			marks = append(marks, editor.GutterMark{Line: l, Color: breakpointColor})
		}
		seen := make(map[int]bool, len(marks)+len(t.gitMarks))
		for _, mark := range marks {
			seen[mark.Line] = true
		}
		for _, mark := range t.gitMarks {
			if !seen[mark.Line] {
				marks = append(marks, mark)
				seen[mark.Line] = true
			}
		}
		marks = append(marks, m.bookmarkMarks(path, seen)...)
		t.state.GutterMarks = marks
		var bgs []editor.LineBackground
		if m.debug.paused && m.debug.stopFile == path && m.debug.stopLine >= 0 {
			bgs = append(bgs, editor.LineBackground{
				First: m.debug.stopLine, Last: m.debug.stopLine, Color: stopLineColor,
			})
		}
		t.state.LineBackgrounds = bgs
	}
}

// javaTarget is what Debug starts: the module whose classpath jdtls resolves
// and the main class of the active file.
type javaTarget struct {
	path      string
	module    string
	mainClass string
	test      bool
}

// javaTargetFor derives the target from a Java file: its module from the
// path below the project root, its class from the package declaration and the
// file name.
func javaTargetFor(root, path, text string) (javaTarget, bool) {
	if !isJavaSource(path) {
		return javaTarget{}, false
	}
	rel := path
	if r, err := filepath.Rel(root, path); err == nil {
		rel = filepath.ToSlash(r)
	}
	rel = strings.TrimPrefix(rel, "./")
	t := javaTarget{path: path}
	switch {
	case strings.Contains("/"+rel+"/", "/src/main/java/"):
		i := strings.Index(rel, "/src/main/java/")
		if i < 0 {
			i = strings.Index(rel, "src/main/java/")
			t.module = ""
		} else {
			t.module = rel[:i]
		}
	case strings.Contains("/"+rel+"/", "/src/test/java/"):
		t.test = true
		if i := strings.Index(rel, "/src/test/java/"); i >= 0 {
			t.module = rel[:i]
		}
	}
	base := filepath.Base(path)
	t.mainClass = strings.TrimSuffix(base, filepath.Ext(base))
	if pkg := javaPackage(text); pkg != "" {
		t.mainClass = pkg + "." + t.mainClass
	}
	return t, t.mainClass != ""
}

// javaPackage returns the file's package declaration, or "".
func javaPackage(text string) string {
	for _, line := range strings.Split(text, "\n") {
		s := strings.TrimSpace(line)
		if !strings.HasPrefix(s, "package") {
			continue
		}
		rest := strings.TrimLeft(strings.TrimPrefix(s, "package"), " \t")
		if rest == s || rest == "" {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		return strings.TrimSuffix(fields[0], ";")
	}
	return ""
}

// javaProjectName is the name jdtls imports a module under: Eclipse's
// descriptor, else the pom's own artifactId (not the parent's), else the
// folder name.
func javaProjectName(dir string) string {
	if b, err := os.ReadFile(filepath.Join(dir, ".project")); err == nil {
		s := string(b)
		if i := strings.Index(s, "<name>"); i >= 0 {
			if j := strings.Index(s[i:], "</name>"); j > 0 {
				name := strings.TrimSpace(s[i+len("<name>") : i+j])
				if name != "" {
					return name
				}
			}
		}
	}
	if b, err := os.ReadFile(filepath.Join(dir, "pom.xml")); err == nil {
		s := string(b)
		if i := strings.Index(s, "<parent>"); i >= 0 {
			if j := strings.Index(s[i:], "</parent>"); j > 0 {
				s = s[:i] + s[i+j+len("</parent>"):]
			}
		}
		if i := strings.Index(s, "<artifactId>"); i >= 0 {
			if j := strings.Index(s[i:], "</artifactId>"); j > 0 {
				id := strings.TrimSpace(s[i+len("<artifactId>") : i+j])
				if id != "" && !strings.Contains(id, "${") {
					return id
				}
			}
		}
	}
	return filepath.Base(dir)
}

// joinJavaArgs quotes program / VM arguments the way java-debug expects them:
// one string, empty and spaced values quoted.
func joinJavaArgs(args []string) string {
	parts := make([]string, 0, len(args))
	for _, a := range args {
		switch {
		case a == "":
			parts = append(parts, `""`)
		case strings.ContainsAny(a, " \t\""):
			parts = append(parts, `"`+strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(a)+`"`)
		default:
			parts = append(parts, a)
		}
	}
	return strings.Join(parts, " ")
}

// buildJavaLaunch is the java-debug LaunchConfig: it connects to the DAP
// server inside jdtls (debugServer) and starts the JVM itself.
func buildJavaLaunch(name string, target javaTarget, root string, info openidea.JavaLaunchInfo, port uint16) (openidea.DebugLaunchConfig, error) {
	classPaths := info.ClassPaths
	if classPaths == nil {
		classPaths = []string{}
	}
	modulePaths := info.ModulePaths
	if modulePaths == nil {
		modulePaths = []string{}
	}
	shorten := "none"
	if info.ShortenCommandLine != nil && *info.ShortenCommandLine != "" {
		shorten = *info.ShortenCommandLine
	}
	cwd := root
	if target.module != "" {
		cwd = filepath.Join(root, filepath.FromSlash(target.module))
	}
	extra := map[string]any{
		"debugServer":        int(port),
		"mainClass":          target.mainClass,
		"classPaths":         classPaths,
		"modulePaths":        modulePaths,
		"projectName":        javaProjectName(cwd),
		"vmArgs":             "",
		"args":               "",
		"shortenCommandLine": shorten,
		"console":            "internalConsole",
		"stopOnEntry":        false,
	}
	if info.JavaExec != nil && *info.JavaExec != "" {
		extra["javaExec"] = *info.JavaExec
	} else if home := os.Getenv("JAVA_HOME"); home != "" {
		java := filepath.Join(home, "bin", "java")
		if st, err := os.Stat(java); err == nil && !st.IsDir() {
			extra["javaExec"] = java
		}
	}
	rawExtra, err := json.Marshal(extra)
	if err != nil {
		return openidea.DebugLaunchConfig{}, err
	}
	s := string(rawExtra)
	return openidea.DebugLaunchConfig{
		Name:      name,
		Adapter:   "java-debug",
		Request:   "launch",
		Args:      []string{},
		Cwd:       cwd,
		Env:       map[string]string{},
		ExtraJSON: &s,
	}, nil
}

// nextRequest allocates an id for a cancellable backend request.
func (m *Model) nextRequest() uint32 { return m.reqSeq.Add(1) }

// ShowDebug toggles the Debug tool window.
func (m *Model) ShowDebug() { m.dockActivate("debug") }

// Debug starts a java-debug session for the active Java file.
func (m *Model) Debug() {
	root := m.Root()
	m.DockShow("debug")
	if root == "" || m.busy {
		m.invalidate()
		return
	}
	if !m.run.trusted {
		m.debug.err = "项目未受信任：先授权构建和调试"
		m.invalidate()
		return
	}
	if m.debug.starting {
		return
	}
	if m.debug.session != 0 && !m.debug.ended {
		m.debug.err = "已有调试会话在运行：先停止"
		m.invalidate()
		return
	}
	t := m.tab()
	if t == nil {
		m.debug.err = "先打开一个 Java 源文件"
		m.invalidate()
		return
	}
	target, ok := javaTargetFor(root, t.doc.Path(), t.doc.Text())
	if !ok {
		m.debug.err = "当前文件不是 Java 主类"
		m.invalidate()
		return
	}
	tabs := append([]*Tab(nil), m.tabs...)
	m.debug.starting = true
	m.debug.session = 0
	m.debug.ended = false
	m.debug.paused = false
	m.debug.stopLine = -1
	m.debug.stopFile = ""
	m.debug.config = target.mainClass
	m.debug.frames = nil
	m.debug.scopes = nil
	m.debug.vars = nil
	m.debug.threads = nil
	m.debug.output = nil
	m.debug.truncated = false
	m.debug.err = ""
	m.debug.status = "正在保存并构建…"
	m.refreshDecorations()
	m.invalidate()

	p := m.prefs.SaveFiles
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Minute)
		defer cancel()
		fail := func(msg string, err error) {
			text := msg
			if err != nil {
				text += "：" + err.Error()
			}
			m.post(func() {
				m.debug.starting = false
				m.debug.err = text
				m.debug.status = ""
				m.invalidate()
			})
		}
		// The build reads the files from disk.
		if err := saveAll(ctx, tabs, p); err != nil {
			fail("保存后再调试失败", err)
			return
		}
		_ = m.pushBreakpoints(ctx, "")
		rawInfo, err := m.mgr.Invoke(ctx, Window, "java_resolve_launch", map[string]any{
			"module": target.module, "test": target.test, "policy": "automatic", "request": m.nextRequest(),
		})
		if err != nil {
			fail("构建项目失败", err)
			return
		}
		var info openidea.JavaLaunchInfo
		if err := json.Unmarshal(rawInfo, &info); err != nil {
			fail("读取构建结果失败", err)
			return
		}
		if info.Build != nil && *info.Build == "failed" {
			fail("构建失败，未启动调试", nil)
			return
		}
		if info.Build != nil && *info.Build == "withErrors" {
			m.post(func() { m.debug.status = "有编译错误，调试最后一次编译的类" })
		}
		rawPort, err := m.mgr.Invoke(ctx, Window, "java_debug_server", map[string]any{"module": target.module})
		if err != nil {
			fail("启动 java-debug 失败", err)
			return
		}
		var port uint16
		if err := json.Unmarshal(rawPort, &port); err != nil {
			fail("读取 java-debug 端口失败", err)
			return
		}
		launch, err := buildJavaLaunch(target.mainClass, target, root, info, port)
		if err != nil {
			fail("构造调试配置失败", err)
			return
		}
		rawSession, err := m.mgr.Invoke(ctx, Window, "debug_start", map[string]any{"config": launch})
		if err != nil {
			fail("启动调试失败", err)
			return
		}
		var session uint32
		if err := json.Unmarshal(rawSession, &session); err != nil {
			fail("读取调试会话失败", err)
			return
		}
		m.post(func() {
			if m.Root() != root {
				// The project changed while the session started: stop the
				// old project's debugger and leave the new state alone.
				go func(id uint32) {
					ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
					defer cancel()
					_, _ = m.mgr.Invoke(ctx, Window, "debug_stop", map[string]any{"session": id})
				}(session)
				return
			}
			m.debug.starting = false
			if m.debug.session == 0 {
				m.debug.session = session
			}
			m.debug.status = "调试已启动：" + target.mainClass
			m.refreshDecorations()
			m.invalidate()
		})
	}()
}

// Continue resumes the paused thread.
func (m *Model) DebugContinue() {
	if m.debug.session == 0 || !m.debug.paused {
		return
	}
	session, thread := m.debug.session, m.debug.stopThread
	m.debug.status = "继续运行…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		if _, err := m.mgr.Invoke(ctx, Window, "debug_resume", map[string]any{"session": session, "threadId": thread}); err != nil {
			m.post(func() { m.debug.err = "继续失败：" + err.Error(); m.invalidate() })
		}
	}()
}

// DebugPause suspends the program.
func (m *Model) DebugPause() {
	if m.debug.session == 0 || m.debug.paused {
		return
	}
	session := m.debug.session
	thread := m.debug.stopThread
	if thread == 0 && len(m.debug.threads) > 0 {
		thread = m.debug.threads[0].ID
	}
	if thread == 0 {
		m.debug.err = "没有可暂停的线程"
		m.invalidate()
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		if _, err := m.mgr.Invoke(ctx, Window, "debug_pause", map[string]any{"session": session, "threadId": thread}); err != nil {
			m.post(func() { m.debug.err = "暂停失败：" + err.Error(); m.invalidate() })
		}
	}()
}

// DebugStep runs one step of kind ("over", "into" or "out") in the paused
// thread.
func (m *Model) DebugStep(kind string) {
	if m.debug.session == 0 || !m.debug.paused || m.debug.stopThread == 0 {
		return
	}
	session, thread := m.debug.session, m.debug.stopThread
	m.debug.status = "单步执行…"
	m.debug.paused = false
	m.refreshDecorations()
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		if _, err := m.mgr.Invoke(ctx, Window, "debug_step", map[string]any{
			"session": session, "threadId": thread, "kind": kind,
		}); err != nil {
			m.post(func() { m.debug.err = "单步失败：" + err.Error(); m.invalidate() })
		}
	}()
}

// StepOver, StepInto and StepOut are the menu's and keyboard's steps.
func (m *Model) StepOver() { m.DebugStep("over") }
func (m *Model) StepInto() { m.DebugStep("into") }
func (m *Model) StepOut()  { m.DebugStep("out") }

// RunToCursor continues to the caret's line (Alt+F9); force ignores
// breakpoints on the way (Cmd+Alt+F9).
func (m *Model) RunToCursor(force bool) {
	if m.debug.session == 0 || !m.debug.paused || m.debug.stopThread == 0 {
		return
	}
	t := m.tab()
	if t == nil {
		m.debug.err = "没有打开的文件，无法运行到光标"
		m.invalidate()
		return
	}
	path := t.doc.Path()
	line := t.state.CursorPos().Line
	session, thread := m.debug.session, m.debug.stopThread
	m.debug.status = "运行到光标…"
	m.debug.paused = false
	m.refreshDecorations()
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
		defer cancel()
		if _, err := m.mgr.Invoke(ctx, Window, "debug_run_to", map[string]any{
			"session": session, "threadId": thread, "path": path, "line": line, "force": force,
		}); err != nil {
			m.post(func() { m.debug.err = "运行到光标失败：" + err.Error(); m.invalidate() })
		}
	}()
}

// ForceRunToCursor runs to the caret ignoring breakpoints.
func (m *Model) ForceRunToCursor() { m.RunToCursor(true) }

// StopDebug ends the session.
func (m *Model) StopDebug() {
	if m.debug.session == 0 || m.debug.ended {
		return
	}
	session := m.debug.session
	m.debug.status = "正在停止调试…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		if _, err := m.mgr.Invoke(ctx, Window, "debug_stop", map[string]any{"session": session}); err != nil {
			m.post(func() { m.debug.err = "停止调试失败：" + err.Error(); m.invalidate() })
		}
	}()
}

// acceptDebugEvent applies one adapter event of the GoIDEA window's session.
func (m *Model) acceptDebugEvent(session uint32, raw json.RawMessage) {
	var e openidea.DebugEvent
	if json.Unmarshal(raw, &e) != nil {
		return
	}
	d := &m.debug
	if d.session == 0 {
		d.session = session
	}
	if session != 0 && d.session != 0 && session != d.session {
		return
	}
	switch e.Kind {
	case "started":
		d.visible = true
		m.occupyBottom("debug")
		d.ended = false
		if e.Name != "" {
			d.config = e.Name
		}
		d.status = "调试已启动"
	case "stopped":
		d.paused = true
		d.ended = false
		d.stopThread = e.ThreadID
		d.reason = e.Reason
		d.status = "已暂停"
		if e.Description != "" {
			d.status = "已暂停：" + e.Description
		}
		m.fetchStop(e.ThreadID)
	case "continued":
		d.paused = false
		d.frames = nil
		d.scopes = nil
		d.vars = nil
		d.stopLine = -1
		d.stopFile = ""
		m.clearWatchValues()
		d.status = "运行中…"
	case "output":
		text := strings.TrimSuffix(stripANSI(e.Output), "\n")
		if text != "" {
			kind := e.Category
			if kind == "" {
				kind = "console"
			}
			for _, line := range strings.Split(text, "\n") {
				d.output = append(d.output, outputLine{kind: kind, text: line})
			}
			if over := len(d.output) - maxDebugLines; over > 0 {
				d.output = append(d.output[:0], d.output[over:]...)
				d.truncated = true
			}
		}
	case "terminated":
		d.ended = true
		d.paused = false
		d.frames = nil
		d.scopes = nil
		d.vars = nil
		d.stopLine = -1
		m.clearWatchValues()
		d.status = "调试已结束"
	case "exited":
		d.ended = true
		d.paused = false
		m.clearWatchValues()
		d.status = "程序已退出"
		if e.Code != nil {
			d.status = fmt.Sprintf("程序已退出（退出码 %d）", *e.Code)
		}
	case "breakpointsChanged":
		// Verification is refetched by the file's next push; nothing to do
		// in this panel yet.
	case "hotCodeReplace":
		if e.Message != "" {
			d.status = "热替换：" + e.Message
		}
	}
	m.refreshDecorations()
	m.invalidate()
}

// fetchStop loads the thread list, the stopped thread's frames, and the first
// scope's variables: what the panel shows for a pause.
func (m *Model) fetchStop(threadID int64) {
	session := m.debug.session
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		var threads []openidea.ThreadInfo
		if raw, err := m.mgr.Invoke(ctx, Window, "debug_threads", map[string]any{"session": session}); err == nil {
			_ = json.Unmarshal(raw, &threads)
		}
		frames, scopes, vars := m.loadFrames(ctx, session, threadID, 0)
		m.post(func() {
			if session != m.debug.session {
				return
			}
			m.debug.threads = threads
			m.debug.frames = frames
			m.debug.selFrame = 0
			m.debug.scopes = scopes
			m.debug.vars = vars
			m.applyStopFrame()
			m.evaluateWatches()
			m.invalidate()
		})
	}()
}

// loadFrames fetches a thread's frames and one frame's scopes and variables.
func (m *Model) loadFrames(ctx context.Context, session uint32, threadID int64, index int) ([]openidea.StackFrame, []openidea.Scope, []openidea.Variable) {
	raw, err := m.mgr.Invoke(ctx, Window, "debug_stack_trace", map[string]any{
		"session": session, "threadId": threadID,
	})
	if err != nil {
		return nil, nil, nil
	}
	var frames []openidea.StackFrame
	if json.Unmarshal(raw, &frames) != nil || len(frames) == 0 {
		return frames, nil, nil
	}
	if index >= len(frames) {
		index = 0
	}
	scopes, vars := m.loadScopes(ctx, session, frames[index].ID)
	return frames, scopes, vars
}

// loadScopes fetches a frame's scopes and the first scope's variables.
func (m *Model) loadScopes(ctx context.Context, session uint32, frameID int64) ([]openidea.Scope, []openidea.Variable) {
	raw, err := m.mgr.Invoke(ctx, Window, "debug_scopes", map[string]any{
		"session": session, "frameId": frameID,
	})
	if err != nil {
		return nil, nil
	}
	var scopes []openidea.Scope
	if json.Unmarshal(raw, &scopes) != nil {
		return nil, nil
	}
	for _, s := range scopes {
		if s.Expensive {
			continue
		}
		rawVars, err := m.mgr.Invoke(ctx, Window, "debug_variables", map[string]any{
			"session": session, "reference": s.VariablesReference,
		})
		if err != nil {
			return scopes, nil
		}
		var vars []openidea.Variable
		if json.Unmarshal(rawVars, &vars) == nil {
			return scopes, vars
		}
	}
	return scopes, nil
}

// SubmitWatch evaluates the typed expression and pins it to the panel.
func (m *Model) SubmitWatch() {
	if !m.debug.paused {
		m.debug.err = "暂停后才能求值"
		m.invalidate()
		return
	}
	if !m.addWatch(m.debug.evalText) {
		return
	}
	m.debug.evalText = ""
	m.invalidate()
}

// addWatch pins expr, replacing nothing: a repeat selects the existing one.
// It reports whether the panel accepted it.
func (m *Model) addWatch(expr string) bool {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return false
	}
	for _, w := range m.debug.watches {
		if w.Expr == expr {
			m.evaluateWatches()
			return true
		}
	}
	if len(m.debug.watches) >= maxWatches {
		m.debug.err = fmt.Sprintf("最多监视 %d 个表达式", maxWatches)
		m.invalidate()
		return false
	}
	m.debug.watches = append(m.debug.watches, debugWatch{Expr: expr})
	m.evaluateWatches()
	return true
}

// RemoveWatch drops the i-th pinned expression.
func (m *Model) RemoveWatch(i int) {
	if i < 0 || i >= len(m.debug.watches) {
		return
	}
	m.debug.watches = append(m.debug.watches[:i], m.debug.watches[i+1:]...)
	m.invalidate()
}

// evaluateWatches evaluates every pinned expression in the stopped frame. A
// reply for a frame the user has stepped past is dropped.
func (m *Model) evaluateWatches() {
	d := &m.debug
	if m.mgr == nil || d.session == 0 || !d.paused || d.frameID == 0 || len(d.watches) == 0 {
		return
	}
	session, frame := d.session, d.frameID
	exprs := make([]string, len(d.watches))
	for i, w := range d.watches {
		exprs[i] = w.Expr
	}
	d.evalBusy = true
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		values := make(map[string]debugWatch, len(exprs))
		for _, expr := range exprs {
			w := debugWatch{Expr: expr}
			raw, err := m.mgr.Invoke(ctx, Window, "debug_evaluate", map[string]any{
				"session": session, "frameId": frame, "expression": expr,
			})
			if err != nil {
				w.Err = err.Error()
			} else {
				var v openidea.Variable
				if uerr := json.Unmarshal(raw, &v); uerr != nil {
					w.Err = uerr.Error()
				} else {
					w.Value = v.Value
				}
			}
			values[expr] = w
		}
		m.post(func() {
			if m.debug.session != session || m.debug.frameID != frame || !m.debug.paused {
				return
			}
			m.debug.evalBusy = false
			for i := range m.debug.watches {
				if w, ok := values[m.debug.watches[i].Expr]; ok {
					m.debug.watches[i].Value = w.Value
					m.debug.watches[i].Err = w.Err
				}
			}
			m.invalidate()
		})
	}()
}

// clearWatchValues drops what the watches showed for a frame that is gone.
func (m *Model) clearWatchValues() {
	for i := range m.debug.watches {
		m.debug.watches[i].Value = ""
		m.debug.watches[i].Err = ""
	}
}

// applyStopFrame highlights and reveals the selected frame's line.
func (m *Model) applyStopFrame() {
	if len(m.debug.frames) == 0 || m.debug.selFrame >= len(m.debug.frames) {
		return
	}
	f := m.debug.frames[m.debug.selFrame]
	m.debug.frameID = f.ID
	if f.Path == nil {
		return
	}
	m.debug.stopFile = *f.Path
	m.debug.stopLine = f.Line
	for i, t := range m.tabs {
		if t.doc.Path() != *f.Path {
			continue
		}
		m.activate(i)
		t.state.GoTo(editor.Pos{Line: f.Line, Col: f.Column})
		break
	}
}

// SelectFrame shows a frame's variables and moves the editor to it.
func (m *Model) SelectFrame(i int) {
	if i < 0 || i >= len(m.debug.frames) {
		return
	}
	m.debug.selFrame = i
	session, frame := m.debug.session, m.debug.frames[i]
	m.applyStopFrame()
	m.refreshDecorations()
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		scopes, vars := m.loadScopes(ctx, session, frame.ID)
		m.post(func() {
			if m.debug.session != session {
				return
			}
			m.debug.scopes = scopes
			m.debug.vars = vars
			m.evaluateWatches()
			m.invalidate()
		})
	}()
}

// SmokeDebug debugs the active Java file with a real java-debug session: set
// a breakpoint on the println line, wait for the pause, evaluate an
// expression, step over, resume, and wait for the end. No mocks.
func (m *Model) SmokeDebug(ctx context.Context) (map[string]any, error) {
	root := m.Root()
	out := map[string]any{"root": root}
	if root == "" {
		return out, errors.New("没有打开项目")
	}
	if _, err := m.mgr.Invoke(ctx, Window, "project_trust_set", map[string]any{
		"path": root, "trusted": true, "folder": true,
	}); err != nil {
		return out, err
	}
	var path, text string
	var line int
	m.post(func() {
		if t := m.tab(); t != nil {
			path, text = t.doc.Path(), t.doc.Text()
		}
	})
	if path == "" {
		// Nothing is open yet: open the project's entry file through the same
		// path the tree uses, so the smoke exercises the real document bridge.
		candidate := findJavaMain(root)
		if candidate == "" {
			return out, errors.New("项目里没有带 main 方法的 Java 文件")
		}
		m.post(func() { m.openFile(candidate) })
		if !m.waitDebug(ctx, func(*DebugPane) bool { return m.tab() != nil }, 90*time.Second) {
			return out, errors.New("打开 " + candidate + " 超时")
		}
		m.post(func() {
			if t := m.tab(); t != nil {
				path, text = t.doc.Path(), t.doc.Text()
			}
		})
		out["opened"] = path
	}
	target, ok := javaTargetFor(root, path, text)
	if !ok {
		return out, errors.New("当前文件不是 Java 主类：" + path)
	}
	out["mainClass"] = target.mainClass
	out["module"] = target.module
	for i, l := range strings.Split(text, "\n") {
		if strings.Contains(l, "System.out.println") {
			line = i
			break
		}
	}
	out["breakpoint"] = line + 1
	if _, err := m.mgr.Invoke(ctx, Window, "debug_set_breakpoints", map[string]any{
		"path": path, "breakpoints": []openidea.Breakpoint{{Line: line, Enabled: true}},
	}); err != nil {
		return out, err
	}
	m.post(func() {
		m.debug.visible = true
		m.occupyBottom("debug")
		m.debug.starting = true
		m.debug.status = "smoke：正在构建…"
		if m.debug.bps == nil {
			m.debug.bps = map[string][]int{}
		}
		m.debug.bps[path] = []int{line}
		m.invalidate()
	})

	rawInfo, err := m.mgr.Invoke(ctx, Window, "java_resolve_launch", map[string]any{
		"module": target.module, "test": target.test, "policy": "automatic", "request": m.nextRequest(),
	})
	if err != nil {
		m.post(func() { m.debug.starting = false })
		return out, fmt.Errorf("java_resolve_launch: %w", err)
	}
	var info openidea.JavaLaunchInfo
	if err := json.Unmarshal(rawInfo, &info); err != nil {
		m.post(func() { m.debug.starting = false })
		return out, err
	}
	if info.Build != nil {
		out["build"] = *info.Build
	}
	out["classPaths"] = len(info.ClassPaths)
	out["errorCount"] = info.ErrorCount
	rawPort, err := m.mgr.Invoke(ctx, Window, "java_debug_server", map[string]any{"module": target.module})
	if err != nil {
		m.post(func() { m.debug.starting = false })
		return out, fmt.Errorf("java_debug_server: %w", err)
	}
	var port uint16
	if err := json.Unmarshal(rawPort, &port); err != nil {
		m.post(func() { m.debug.starting = false })
		return out, err
	}
	out["debugServer"] = port
	launch, err := buildJavaLaunch(target.mainClass, target, root, info, port)
	if err != nil {
		m.post(func() { m.debug.starting = false })
		return out, err
	}
	out["extraJson"] = *launch.ExtraJSON
	rawSession, err := m.mgr.Invoke(ctx, Window, "debug_start", map[string]any{"config": launch})
	if err != nil {
		m.post(func() { m.debug.starting = false })
		return out, fmt.Errorf("debug_start: %w", err)
	}
	var session uint32
	if err := json.Unmarshal(rawSession, &session); err != nil {
		m.post(func() { m.debug.starting = false })
		return out, err
	}
	out["session"] = session
	m.post(func() {
		m.debug.starting = false
		m.debug.session = session
		m.debug.ended = false
		m.debug.err = ""
		m.invalidate()
	})

	if !m.waitDebug(ctx, func(d *DebugPane) bool { return d.paused }, 4*time.Minute) {
		var state string
		m.post(func() { state = m.debug.status + " " + m.debug.err })
		out["paused"] = false
		out["state"] = state
		return out, errors.New("断点未在 4 分钟内命中：" + state)
	}
	out["paused"] = true
	frameID := int64(0)
	m.post(func() {
		out["status"] = m.debug.status
		out["frames"] = len(m.debug.frames)
		if len(m.debug.frames) > 0 {
			out["frame"] = m.debug.frames[0].Name
			out["frameLine"] = m.debug.frames[0].Line + 1
			frameID = m.debug.frames[0].ID
		}
		out["vars"] = len(m.debug.vars)
	})

	// Evaluate in the stopped frame: java-debug rejects a frame-less evaluate
	// while the thread is not marked suspended yet, and the UI evaluates in the
	// selected frame the same way.
	var evalFrame any
	if frameID != 0 {
		evalFrame = frameID
	}
	rawEval, evalErr := m.mgr.Invoke(ctx, Window, "debug_evaluate", map[string]any{
		"session": session, "frameId": evalFrame, "expression": `System.getProperty("java.version")`,
	})
	if evalErr == nil {
		var v openidea.Variable
		if err := json.Unmarshal(rawEval, &v); err != nil {
			out["evaluateError"] = err.Error()
		} else {
			out["evaluated"] = v.Value
		}
	} else {
		out["evaluateError"] = evalErr.Error()
	}

	// A pinned watch goes through the panel's own submit path and evaluates in
	// the stopped frame, without the smoke talking to the backend directly.
	m.post(func() {
		m.debug.evalText = "2 + 2"
		m.SubmitWatch()
	})
	watchDeadline := time.Now().Add(20 * time.Second)
	for {
		var watch, watchErr string
		var busy bool
		m.post(func() {
			busy = m.debug.evalBusy
			for _, w := range m.debug.watches {
				if w.Expr == "2 + 2" {
					watch, watchErr = w.Value, w.Err
				}
			}
		})
		if !busy || time.Now().After(watchDeadline) {
			out["watch"] = watch
			out["watchErr"] = watchErr
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}

	// Run to Cursor to the loop body: the same pause must arrive at that line
	// through debug_run_to, as Alt+F9 does in the UI.
	runTo := -1
	m.post(func() {
		if t := m.tab(); t != nil {
			for i, l := range strings.Split(t.doc.Text(), "\n") {
				if i > line && strings.Contains(l, "total += i") {
					runTo = i
					t.state.GoTo(editor.Pos{Line: i, Col: 0})
					break
				}
			}
		}
	})
	if runTo >= 0 {
		m.post(func() { m.RunToCursor(false) })
		m.waitDebug(ctx, func(d *DebugPane) bool {
			return d.ended || (d.paused && d.stopLine == runTo)
		}, 60*time.Second)
		m.post(func() {
			out["runToLine"] = runTo + 1
			out["runToPaused"] = m.debug.paused && m.debug.stopLine == runTo
			out["runToStatus"] = m.debug.status
		})
	}

	// Step over the println: the frame moves and the session pauses again.
	stopThread := int64(0)
	beforeStep := -1
	m.post(func() { stopThread, beforeStep = m.debug.stopThread, m.debug.stopLine })
	if stopThread != 0 {
		if _, err := m.mgr.Invoke(ctx, Window, "debug_step", map[string]any{
			"session": session, "threadId": stopThread, "kind": "over",
		}); err == nil {
			// The breakpoint still marks paused, so wait for the step's own
			// stop: a different line, or the session ending.
			m.waitDebug(ctx, func(d *DebugPane) bool {
				return d.ended || (d.paused && d.stopLine != beforeStep)
			}, 60*time.Second)
			m.post(func() {
				out["afterStepPaused"] = m.debug.paused
				out["afterStepLine"] = m.debug.stopLine + 1
			})
		}
	}
	m.post(func() { m.debug.paused = true })
	if _, err := m.mgr.Invoke(ctx, Window, "debug_resume", map[string]any{
		"session": session, "threadId": stopThread,
	}); err != nil {
		out["resumeError"] = err.Error()
	}
	m.waitDebug(ctx, func(d *DebugPane) bool { return d.ended }, 60*time.Second)
	m.post(func() {
		out["ended"] = m.debug.ended
		out["statusAfter"] = m.debug.status
		out["output"] = m.debug.text()
		out["outputLines"] = len(m.debug.output)
	})
	return out, nil
}

// waitDebug polls the model's debug pane until cond holds or timeout; it is
// for the offscreen smoke run, which drives the model itself.
// findJavaMain picks the smoke's entry file: the first main class under the
// project, preferring one that prints (the breakpoint smoke needs a line to
// stop on). Build outputs are skipped, as they hold copies of the sources.
func findJavaMain(root string) string {
	var mains []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != root {
				switch d.Name() {
				case ".git", ".idea", "target", "build", "out", "node_modules":
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(p, ".java") {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil || !strings.Contains(string(data), "static void main") {
			return nil
		}
		mains = append(mains, p)
		if strings.Contains(string(data), "System.out.println") {
			mains = []string{p}
			return filepath.SkipAll
		}
		return nil
	})
	if len(mains) == 0 {
		return ""
	}
	return mains[0]
}

// waitDebug polls cond on the UI loop until it holds or the timeout passes.
func (m *Model) waitDebug(ctx context.Context, cond func(*DebugPane) bool, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		ok := false
		m.post(func() { ok = cond(&m.debug) })
		if ok {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(150 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			return false
		}
	}
}

// text is the adapter console, one line per entry.
func (p *DebugPane) text() string {
	var b strings.Builder
	for i, line := range p.output {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(line.text)
	}
	return b.String()
}
