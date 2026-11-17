package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/adapter"
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
	// startCancel stops a launch that is still waiting or preparing (Stop
	// while the Java project imports), so no session starts after the user
	// gave up.
	startCancel context.CancelFunc
	session     uint32 // backend DebugSessionId; 0 before the first start
	config      string // main class being debugged

	// lastID/lastRaw/lastKind remember the configuration this pane debugged,
	// so the widget's rerun restarts it as saved now.
	lastID   string
	lastRaw  json.RawMessage
	lastKind string
	// launchCwd is the session's working directory (its module dir); the
	// HotSwap compile covers it.
	launchCwd string
	// reloading guards HotSwapReload against concurrent compiles; hotSwapBuild
	// remembers the last HotSwap compile's status for the acceptance run.
	reloading    bool
	hotSwapBuild string
	hotSwapStage string

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
	if m.debug.startCancel != nil {
		m.debug.startCancel()
	}
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
		m.status = "Breakpoints only work in Java source files"
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
		m.status = fmt.Sprintf("Breakpoint removed at line %d", line+1)
	} else {
		m.status = fmt.Sprintf("Breakpoint set at line %d", line+1)
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

// relPathUnder is the file's path relative to the project root, slash-form.
func relPathUnder(root, path string) string {
	rel := path
	if r, err := filepath.Rel(root, path); err == nil {
		rel = filepath.ToSlash(r)
	}
	return strings.TrimPrefix(rel, "./")
}

// applicationConfig ports upstream applicationConfig: the temporary
// "Application" configuration that runs a file's main class. The module comes
// from jvmModuleDir (the run configs the build knows, then the source
// layout); the classpath placeholder decides runtime vs test classpath.
func applicationConfig(target javaTarget, module string) *openidea.RunConfig {
	outer := target.mainClass
	if i := strings.Index(outer, "$"); i >= 0 {
		outer = outer[:i]
	}
	simple := outer[strings.LastIndex(outer, ".")+1:]
	placeholder := adapter.Classpath
	if target.test {
		placeholder = adapter.TestClasspath
	}
	return &openidea.RunConfig{
		ID:      "application:" + module + ":" + target.mainClass,
		Name:    simple,
		Kind:    "application",
		Command: "java",
		Args:    []string{"-cp", placeholder, target.mainClass},
		Module:  &module,
	}
}

// javaDebugMissing is the server verdict that java-debug is not loaded into
// jdtls, matched like upstream offerInstall does.
var javaDebugMissing = regexp.MustCompile(`(?i)java-debug|debugger`)

// javaDebugLaunch ports upstream javaLaunchConfig's java-debug branch: the
// DAP launch configuration built from the shared preparation result.
// java-debug starts the JVM itself, bypassing the Run process that echoes its
// command line, so the equivalent launch is echoed for Debug and Run to stay
// comparable.
func (m *Model) javaDebugLaunch(ctx context.Context, rc *openidea.RunConfig, root string, prepared *adapter.PreparedJavaLaunch, report func(text, kind string)) (openidea.DebugLaunchConfig, error) {
	rc, err := m.withEnvFiles(ctx, rc, root)
	if err != nil {
		return openidea.DebugLaunchConfig{}, err
	}
	info := &prepared.Info
	parsed := prepared.Parsed
	if info.ErrorCount > 0 && report == nil {
		m.post(func() {
			m.notifyWarning("debug.java-compile-errors",
				"'"+rc.Name+"' has compilation errors; debugging the last compiled classes",
				strings.TrimRight(adapter.FormatCompileErrors(info), "\n"))
		})
	}
	if report != nil {
		report(adapter.JoinJavaArgs(append([]string{prepared.Command}, prepared.Resolved...))+"\n", "system")
	}
	rawPort, err := m.invoke(ctx, "java_debug_server", map[string]any{"module": prepared.Module})
	if err != nil {
		// java-debug missing from jdtls: offer the one-click install; jdtls
		// restarts with the bundle.
		if openidea.ErrorKind(err) == "unavailable" && javaDebugMissing.MatchString(err.Error()) {
			m.post(func() {
				m.notifyWarning("debug.java-debug-missing", "Java Debugger Not Installed",
					"Debugging Java needs Microsoft java-debug inside jdtls. Install it and debug once jdtls restarts.",
					notifyAction{label: "Install", run: func(m *Model) { m.InstallJavaToolchain() }})
			})
		}
		return openidea.DebugLaunchConfig{}, err
	}
	var port uint16
	if err := json.Unmarshal(rawPort, &port); err != nil {
		return openidea.DebugLaunchConfig{}, fmt.Errorf("Could not read the java-debug port: %w", err)
	}
	// java-debug resolves the Java project for Evaluate on its own from
	// `mainClass` only when the class belongs to a project it knows; name the
	// project it imported so that works for classes compiled outside the
	// language server's build.
	projectRoot := root
	if info.ProjectRoot != nil && *info.ProjectRoot != "" {
		projectRoot = *info.ProjectRoot
	}
	cwd := root
	if rel := rc.CwdOr(""); rel != "" {
		cwd = filepath.Join(root, filepath.FromSlash(rel))
	}
	classPaths := info.ClassPaths
	if parsed.ExplicitClasspath != "" {
		classPaths = strings.Split(parsed.ExplicitClasspath, info.PathListSeparator)
	}
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
	vmArgs := append(adapter.ConsoleVMArgs(rc.Kind, rc.Args), parsed.VMArgs...)
	extra := map[string]any{
		"debugServer": int(port),
		"mainClass":   parsed.MainClass,
		"classPaths":  classPaths,
		"modulePaths": modulePaths,
		"projectName": javaProjectName(projectRoot),
		"vmArgs":      adapter.JoinJavaArgs(vmArgs),
		"args":        adapter.JoinJavaArgs(parsed.ProgramArgs),
		// java-debug spawns the JVM itself: it must shorten a classpath too
		// long for the OS the same way Run does.
		"shortenCommandLine": shorten,
		"console":            "internalConsole",
		"stopOnEntry":        false,
	}
	if prepared.Command != "java" {
		extra["javaExec"] = prepared.Command
	}
	rawExtra, err := json.Marshal(extra)
	if err != nil {
		return openidea.DebugLaunchConfig{}, err
	}
	s := string(rawExtra)
	env := rc.Env
	if env == nil {
		env = map[string]string{}
	}
	m.debug.launchCwd = cwd
	return openidea.DebugLaunchConfig{
		Name:      rc.Name,
		Adapter:   "java-debug",
		Request:   "launch",
		Program:   nil,
		Args:      []string{},
		Cwd:       cwd,
		Env:       env,
		ExtraJSON: &s,
	}, nil
}

// withEnvFiles is upstream launch.ts's withEnvFiles: rc with its .env files
// loaded underneath its explicit variables (mirrors Run). Absolute paths go
// to the backend, which parses with the same loader runs use.
func (m *Model) withEnvFiles(ctx context.Context, rc *openidea.RunConfig, root string) (*openidea.RunConfig, error) {
	var files []string
	for _, f := range rc.EnvFiles {
		if strings.TrimSpace(f) != "" {
			files = append(files, f)
		}
	}
	if len(files) == 0 {
		return rc, nil
	}
	paths := make([]string, 0, len(files))
	for _, f := range files {
		paths = append(paths, filepath.Join(root, filepath.FromSlash(f)))
	}
	raw, err := m.invoke(ctx, "run_env_files", map[string]any{"files": paths})
	if err != nil {
		return nil, err
	}
	var loaded map[string]string
	if err := json.Unmarshal(raw, &loaded); err != nil {
		return nil, err
	}
	env := make(map[string]string, len(loaded)+len(rc.Env))
	for k, v := range loaded {
		env[k] = v
	}
	for k, v := range rc.Env {
		env[k] = v
	}
	merged := *rc
	merged.Env = env
	return &merged, nil
}

// reportDebugConsole appends preparation messages to the Debug tool window's
// adapter output of the project they belong to.
func (m *Model) reportDebugConsole(root string) func(text, kind string) {
	return func(text, kind string) {
		m.post(func() {
			if m.Root() != root {
				return
			}
			if kind == "" {
				kind = "system"
			}
			for _, line := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
				if line == "" {
					continue
				}
				m.debug.output = append(m.debug.output, outputLine{kind: kind, text: line})
			}
			if over := len(m.debug.output) - maxDebugLines; over > 0 {
				m.debug.output = append(m.debug.output[:0], m.debug.output[over:]...)
				m.debug.truncated = true
			}
			m.invalidate()
		})
	}
}

// nextRequest allocates an id for a cancellable backend request.
func (m *Model) nextRequest() uint32 { return m.reqSeq.Add(1) }

// ShowDebug toggles the Debug tool window.
func (m *Model) ShowDebug() { m.dockActivate("debug") }

// Debug is run.debug: debug the selected run configuration (upstream
// debugSelected). The active file's own configuration is run.debugContext.
func (m *Model) Debug() {
	m.DockShow("debug")
	if m.busy {
		m.invalidate()
		return
	}
	cfg := m.run.current()
	if cfg == nil && m.run.inline != nil {
		cfg = m.run.inline
	}
	if cfg == nil {
		m.debug.err = "Choose a configuration in the run configuration chooser first"
		m.invalidate()
		return
	}
	m.DebugRunConfig(cfg)
}

// DebugRunConfig starts a java-debug session for an arbitrary run
// configuration, the port of upstream javaLaunchConfig: JVM applications go
// through the shared PrepareJavaLaunch, a kind GoIDEA cannot debug is
// reported instead of silently starting.
func (m *Model) DebugRunConfig(rc *openidea.RunConfig) {
	root := m.Root()
	m.DockShow("debug")
	if root == "" || m.busy {
		m.invalidate()
		return
	}
	if !m.run.trusted {
		m.debug.err = "The project is not trusted: allow building and debugging first"
		m.invalidate()
		return
	}
	gate, wait := m.debugReadiness()
	if gate != "" {
		m.debug.err = gate
		m.invalidate()
		return
	}
	if m.debug.starting {
		return
	}
	if m.debug.session != 0 && !m.debug.ended {
		m.debug.err = "A debug session is already running: stop it first"
		m.invalidate()
		return
	}
	if !adapter.IsJVMApplication(rc.Kind) {
		m.debug.err = fmt.Sprintf("GoIDEA cannot debug %s run configurations yet", adapter.KindLabel(rc.Kind))
		m.invalidate()
		return
	}
	if rc.DebugAdapter != nil && *rc.DebugAdapter != "java-debug" && *rc.DebugAdapter != "" {
		m.debug.err = fmt.Sprintf("The %s debugger is not available: GoIDEA ships java-debug only", *rc.DebugAdapter)
		m.invalidate()
		return
	}
	if adapter.ParseJavaArgs(rc.Args).MainClass == "" {
		m.debug.err = fmt.Sprintf("Configuration '%s' has no main class", rc.Name)
		m.invalidate()
		return
	}
	m.debug.lastID = rc.ID
	m.debug.lastRaw = append(json.RawMessage(nil), rc.Raw()...)
	m.debug.lastKind = rc.Kind
	m.debug.config = adapter.ParseJavaArgs(rc.Args).MainClass
	m.debug.starting = true
	m.debug.session = 0
	m.debug.ended = false
	m.debug.paused = false
	m.debug.stopLine = -1
	m.debug.stopFile = ""
	m.debug.frames = nil
	m.debug.scopes = nil
	m.debug.vars = nil
	m.debug.threads = nil
	m.debug.output = nil
	m.debug.truncated = false
	m.debug.err = ""
	m.debug.status = "Saving and building…"
	if wait {
		m.debug.status = "Waiting for the Java project to finish importing…"
	}
	m.refreshDecorations()
	m.invalidate()

	p := m.prefs.SaveFiles
	tabs := append([]*Tab(nil), m.tabs...)
	ctx, cancel := context.WithTimeout(m.ctx, 10*time.Minute)
	m.debug.startCancel = cancel
	go func() {
		defer cancel()
		fail := func(msg string, err error) {
			text := msg
			if err != nil {
				text += "：" + err.Error()
			}
			m.post(func() {
				if !m.debug.starting {
					return
				}
				m.debug.starting = false
				m.debug.startCancel = nil
				m.debug.err = text
				m.debug.status = ""
				m.invalidate()
			})
		}
		if wait {
			if err := m.waitJavaReady(ctx); err != nil {
				fail("The Java project did not finish importing", err)
				return
			}
			m.post(func() { m.debug.status = "Saving and building…"; m.invalidate() })
		}
		// The build reads the files from disk.
		if err := saveAll(ctx, tabs, p); err != nil {
			fail("Saving before debugging failed", err)
			return
		}
		_ = m.pushBreakpoints(ctx, "")
		// Run and Debug share one preparation, exactly like OpenIDEA's
		// prepareJavaLaunch. A failure stops the launch here.
		prepared, err := adapter.PrepareJavaLaunch(ctx, javaBackend{m}, rc, &adapter.Preparation{
			Request: m.nextRequest(),
			Report:  m.reportDebugConsole(root),
		})
		if err != nil {
			fail(err.Error(), nil)
			return
		}
		if prepared.Info.ErrorCount > 0 {
			m.post(func() {
				m.debug.status = "There are compilation errors; debugging the last compiled classes"
				m.invalidate()
			})
		}
		launch, err := m.javaDebugLaunch(ctx, rc, root, prepared, m.reportDebugConsole(root))
		if err != nil {
			fail(err.Error(), nil)
			return
		}
		rawSession, err := m.mgr.Invoke(ctx, Window, "debug_start", map[string]any{"config": launch})
		if err != nil {
			fail("Starting the debug session failed", err)
			return
		}
		var session uint32
		if err := json.Unmarshal(rawSession, &session); err != nil {
			fail("Reading the debug session failed", err)
			return
		}
		m.post(func() {
			if m.Root() != root || !m.debug.starting {
				// The project changed, or the user stopped while it started:
				// stop the backend session at once.
				go func(id uint32) {
					ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
					defer cancel()
					_, _ = m.mgr.Invoke(ctx, Window, "debug_stop", map[string]any{"session": id})
				}(session)
				return
			}
			m.debug.starting = false
			m.debug.startCancel = nil
			if m.debug.session == 0 {
				m.debug.session = session
			}
			m.debug.status = "Debugging started: " + m.debug.config
			m.refreshDecorations()
			m.invalidate()
		})
	}()
}

// DebugRerun restarts the configuration the debug pane ran, as upstream's
// rerun of a debug session does: the configuration as saved now, its session
// stopped first, and — when no session was started yet — the same call just
// starts it.
func (m *Model) DebugRerun() {
	if m.debug.lastRaw == nil {
		return
	}
	cfg, err := openidea.DecodeRunConfig(m.debug.lastRaw)
	if err != nil {
		m.debug.err = "Could not read the last debug configuration: " + err.Error()
		m.invalidate()
		return
	}
	if m.debug.session == 0 || m.debug.ended {
		m.DebugRunConfig(cfg)
		return
	}
	m.StopDebug()
	session := m.debug.session
	go func() {
		deadline := time.Now().Add(30 * time.Second)
		for {
			var ended bool
			var next uint32
			m.post(func() {
				ended = m.debug.ended || m.debug.session != session && m.debug.session == 0
				next = m.debug.session
			})
			_ = next
			if ended || time.Now().After(deadline) {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		m.post(func() { m.DebugRunConfig(cfg) })
	}()
}

// DebugContext is run.debugContext: debug the active file's own
// configuration (upstream runContext(true, arg)).
func (m *Model) DebugContext() {
	root := m.Root()
	m.DockShow("debug")
	if root == "" || m.busy {
		m.invalidate()
		return
	}
	if !m.run.trusted {
		m.debug.err = "The project is not trusted: allow building and debugging first"
		m.invalidate()
		return
	}
	gateCtx, waitImport := m.debugReadiness()
	if gateCtx != "" {
		m.debug.err = gateCtx
		m.invalidate()
		return
	}
	if m.debug.starting {
		return
	}
	if m.debug.session != 0 && !m.debug.ended {
		m.debug.err = "A debug session is already running: stop it first"
		m.invalidate()
		return
	}
	t := m.tab()
	if t == nil {
		m.debug.err = "Open a Java source file first"
		m.invalidate()
		return
	}
	target, ok := javaTargetFor(root, t.doc.Path(), t.doc.Text())
	if !ok {
		m.debug.err = "The current file is not a Java main class"
		m.invalidate()
		return
	}
	rel := relPathUnder(root, t.doc.Path())
	rc := applicationConfig(target, adapter.JVMModuleDir(rel, m.run.configs, nil))
	m.debug.lastID = rc.ID
	m.debug.lastRaw = append(json.RawMessage(nil), rc.Raw()...)
	m.debug.lastKind = rc.Kind
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
	m.debug.status = "Saving and building…"
	if waitImport {
		m.debug.status = "Waiting for the Java project to finish importing…"
	}
	m.refreshDecorations()
	m.invalidate()

	p := m.prefs.SaveFiles
	ctx, cancel := context.WithTimeout(m.ctx, 10*time.Minute)
	m.debug.startCancel = cancel
	go func() {
		defer cancel()
		fail := func(msg string, err error) {
			text := msg
			if err != nil {
				text += "：" + err.Error()
			}
			m.post(func() {
				if !m.debug.starting {
					return
				}
				m.debug.starting = false
				m.debug.startCancel = nil
				m.debug.err = text
				m.debug.status = ""
				m.invalidate()
			})
		}
		if waitImport {
			if err := m.waitJavaReady(ctx); err != nil {
				fail("The Java project did not finish importing", err)
				return
			}
			m.post(func() { m.debug.status = "Saving and building…"; m.invalidate() })
		}
		// The build reads the files from disk.
		if err := saveAll(ctx, tabs, p); err != nil {
			fail("Saving before debugging failed", err)
			return
		}
		_ = m.pushBreakpoints(ctx, "")
		// Run and Debug share one preparation: module, build and classpath
		// through java_resolve_launch, exactly like OpenIDEA's
		// prepareJavaLaunch. A failure stops the launch here.
		prepared, err := adapter.PrepareJavaLaunch(ctx, javaBackend{m}, rc, &adapter.Preparation{
			Request: m.nextRequest(),
			Report:  m.reportDebugConsole(root),
		})
		if err != nil {
			fail(err.Error(), nil)
			return
		}
		if prepared.Info.ErrorCount > 0 {
			m.post(func() {
				m.debug.status = "There are compilation errors; debugging the last compiled classes"
				m.invalidate()
			})
		}
		launch, err := m.javaDebugLaunch(ctx, rc, root, prepared, m.reportDebugConsole(root))
		if err != nil {
			fail(err.Error(), nil)
			return
		}
		rawSession, err := m.mgr.Invoke(ctx, Window, "debug_start", map[string]any{"config": launch})
		if err != nil {
			fail("Starting the debug session failed", err)
			return
		}
		var session uint32
		if err := json.Unmarshal(rawSession, &session); err != nil {
			fail("Reading the debug session failed", err)
			return
		}
		m.post(func() {
			if m.Root() != root || !m.debug.starting {
				// The project changed, or the user stopped while it started:
				// stop the backend session at once.
				go func(id uint32) {
					ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
					defer cancel()
					_, _ = m.mgr.Invoke(ctx, Window, "debug_stop", map[string]any{"session": id})
				}(session)
				return
			}
			m.debug.starting = false
			m.debug.startCancel = nil
			if m.debug.session == 0 {
				m.debug.session = session
			}
			m.debug.status = "Debugging started: " + target.mainClass
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
	m.debug.status = "Resuming…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		if _, err := m.mgr.Invoke(ctx, Window, "debug_resume", map[string]any{"session": session, "threadId": thread}); err != nil {
			m.post(func() { m.debug.err = "Resume failed: " + err.Error(); m.invalidate() })
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
		m.debug.err = "There is no thread to pause"
		m.invalidate()
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		if _, err := m.mgr.Invoke(ctx, Window, "debug_pause", map[string]any{"session": session, "threadId": thread}); err != nil {
			m.post(func() { m.debug.err = "Pause failed: " + err.Error(); m.invalidate() })
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
	m.debug.status = "Stepping…"
	m.debug.paused = false
	m.refreshDecorations()
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		if _, err := m.mgr.Invoke(ctx, Window, "debug_step", map[string]any{
			"session": session, "threadId": thread, "kind": kind,
		}); err != nil {
			m.post(func() { m.debug.err = "Step failed: " + err.Error(); m.invalidate() })
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
		m.debug.err = "No file is open, so Run to Cursor has no place to go"
		m.invalidate()
		return
	}
	path := t.doc.Path()
	line := t.state.CursorPos().Line
	session, thread := m.debug.session, m.debug.stopThread
	m.debug.status = "Running to the cursor…"
	m.debug.paused = false
	m.refreshDecorations()
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
		defer cancel()
		if _, err := m.mgr.Invoke(ctx, Window, "debug_run_to", map[string]any{
			"session": session, "threadId": thread, "path": path, "line": line, "force": force,
		}); err != nil {
			m.post(func() { m.debug.err = "Run to Cursor failed: " + err.Error(); m.invalidate() })
		}
	}()
}

// ForceRunToCursor runs to the caret ignoring breakpoints.
func (m *Model) ForceRunToCursor() { m.RunToCursor(true) }

// StopDebug ends the session.
// StopDebug stops the session, or a launch still waiting for the project's
// import.
func (m *Model) StopDebug() {
	if m.debug.starting {
		if m.debug.startCancel != nil {
			m.debug.startCancel()
			m.debug.startCancel = nil
		}
		m.debug.starting = false
		m.debug.status = ""
		m.debug.err = ""
		m.invalidate()
	}
	if m.debug.session == 0 || m.debug.ended {
		return
	}
	session := m.debug.session
	m.debug.status = "Stopping the debug session…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		if _, err := m.mgr.Invoke(ctx, Window, "debug_stop", map[string]any{"session": session}); err != nil {
			m.post(func() { m.debug.err = "Stopping the debug session failed: " + err.Error(); m.invalidate() })
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
		d.status = "Debugging started"
	case "stopped":
		d.paused = true
		d.ended = false
		d.stopThread = e.ThreadID
		d.reason = e.Reason
		d.status = "Paused"
		if e.Description != "" {
			d.status = "Paused: " + e.Description
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
		d.status = "Running…"
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
		d.status = "Debugging ended"
	case "exited":
		d.ended = true
		d.paused = false
		m.clearWatchValues()
		d.status = "The program exited"
		if e.Code != nil {
			d.status = fmt.Sprintf("The program exited (code %d)", *e.Code)
		}
	case "breakpointsChanged":
		// Verification is refetched by the file's next push; nothing to do
		// in this panel yet.
	case "hotCodeReplace":
		// java-debug reports after redefineClasses and after jdtls's own
		// build (BUILD_COMPLETE): offer the reload, as IDEA's default
		// "Reload classes after compilation: Ask" does.
		if e.ChangeType == "BUILD_COMPLETE" {
			m.offerHotSwapAfterBuild()
		} else if e.Message != "" {
			d.status = "Hot swap: " + e.Message
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
		m.debug.err = "Pause before evaluating"
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
		m.debug.err = fmt.Sprintf("At most %d expressions can be watched", maxWatches)
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
		return out, errors.New("No project is open")
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
			return out, errors.New("The project has no Java file with a main method")
		}
		m.post(func() { m.openFile(candidate) })
		if !m.waitDebug(ctx, func(*DebugPane) bool { return m.tab() != nil }, 90*time.Second) {
			return out, errors.New("Opening " + candidate + " timed out")
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
		return out, errors.New("The current file is not a Java main class: " + path)
	}
	out["mainClass"] = target.mainClass
	rc := applicationConfig(target, adapter.JVMModuleDir(relPathUnder(root, path), nil, nil))
	out["module"] = *rc.Module
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
		m.debug.status = "smoke: building…"
		if m.debug.bps == nil {
			m.debug.bps = map[string][]int{}
		}
		m.debug.bps[path] = []int{line}
		m.invalidate()
	})

	// The same shared preparation Run and Debug use: no smoke-only shortcut.
	prepared, err := adapter.PrepareJavaLaunch(ctx, javaBackend{m}, rc, &adapter.Preparation{
		Request: m.nextRequest(),
		Report:  m.reportDebugConsole(root),
	})
	if err != nil {
		m.post(func() { m.debug.starting = false })
		return out, err
	}
	if prepared.Info.Build != nil {
		out["build"] = *prepared.Info.Build
	}
	out["classPaths"] = len(prepared.Info.ClassPaths)
	out["errorCount"] = prepared.Info.ErrorCount
	launch, err := m.javaDebugLaunch(ctx, rc, root, prepared, m.reportDebugConsole(root))
	if err != nil {
		m.post(func() { m.debug.starting = false })
		return out, err
	}
	out["debugServer"] = true
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
		return out, errors.New("The breakpoint was not hit within 4 minutes: " + state)
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

// SmokeDebugImport starts a debug while the Java project is still importing
// and reports that it waits instead of refusing, for the real-server acceptance
// run. It stops the pending launch before any build starts.
func (m *Model) SmokeDebugImport(ctx context.Context) (map[string]any, error) {
	out := map[string]any{}
	root := m.Root()
	if root == "" {
		return out, errors.New("没有打开项目")
	}
	if _, err := m.mgr.Invoke(ctx, Window, "project_trust_set", map[string]any{
		"path": root, "trusted": true, "folder": true,
	}); err != nil {
		return out, err
	}
	// The grant above is the backend's; the model's run pane learns it through
	// the same check the UI uses when a project opens.
	m.checkTrust(root)
	trusted := false
	for !trusted {
		m.post(func() { trusted = m.run.trusted })
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	if err := m.reg.ReopenAll(ctx); err != nil {
		out["reopenError"] = err.Error()
	}
	candidate := findJavaMain(root)
	if candidate == "" {
		return out, errors.New("项目里没有带 main 方法的 Java 文件")
	}
	m.post(func() { m.openFile(candidate) })
	if !m.waitDebug(ctx, func(*DebugPane) bool { return m.tab() != nil }, 60*time.Second) {
		return out, errors.New("打开 " + candidate + " 超时")
	}
	out["file"] = candidate
	// The import window: jdtls reports starting or its analysis running.
	deadline := time.Now().Add(90 * time.Second)
	caught := false
	for time.Now().Before(deadline) {
		r := m.readinessNow(ctx)
		if r.Importing || (!r.Java && r.JavaDetail == "starting") {
			out["readiness"] = r.JavaDetail
			caught = true
			break
		}
		if r.Java {
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	if !caught {
		out["skipped"] = "the Java project was already imported"
		return out, nil
	}
	m.post(func() { m.DebugContext() })
	if !m.waitDebug(ctx, func(d *DebugPane) bool {
		return d.starting || d.err != "" || d.session != 0
	}, 20*time.Second) {
		return out, errors.New("Debug did not react")
	}
	m.post(func() {
		out["starting"] = m.debug.starting
		out["status"] = m.debug.status
		out["err"] = m.debug.err
		m.StopDebug()
		out["stopped"] = !m.debug.starting && m.debug.session == 0 && m.debug.err == ""
	})
	// A stopped waiting launch must not start a session once the import ends.
	select {
	case <-ctx.Done():
		return out, ctx.Err()
	case <-time.After(2 * time.Second):
	}
	m.post(func() { out["sessionAfterStop"] = m.debug.session })
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
