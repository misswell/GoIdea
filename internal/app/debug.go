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

// DebugPane is the Debug tool window: independent java-debug content tabs
// through jdtls, the selected session's paused frames and variables, adapter
// output, and the line breakpoints the user set. Breakpoints live here, not in
// a session: they are given to every session that starts.
type DebugPane struct {
	visible  bool
	starting bool
	// active is the selected Debug content tab. session is the active tab's
	// live backend id and becomes zero when that tab terminates; sessions keeps
	// the other tabs' independent frames, console, status and lifecycle.
	active          uint32
	sessions        map[uint32]*debugSessionState
	sessionOrder    []uint32
	closeAfterStop  map[uint32]bool
	retiredSessions map[uint32]bool
	retiredOrder    []uint32
	// heapChannels maps a bounded backend progress stream to the content tab
	// that requested it. It is cleared when that request finishes, fails or is
	// cancelled, so late chunk notifications cannot revive an old request.
	heapChannels map[uint32]uint32
	// startCancel stops a launch that is still waiting or preparing (Stop
	// while the Java project imports), so no session starts after the user
	// gave up.
	startCancel    context.CancelFunc
	session        uint32 // active backend DebugSessionId; 0 when none is running
	parent         uint32 // root session for an adapter-started child; 0 for a root
	adapter        string
	stopping       bool // debug_stop is in flight for session
	caps           openidea.DebugCapabilities
	capsKnown      bool
	capsRequest    uint64
	jvmBackend     openidea.JvmBackendState
	stopGeneration uint64
	threadRequest  uint64
	frameRequest   uint64
	// retiredSession is the last session the user stopped. Events can arrive
	// after the stop reply, so they must not attach the old session to a new
	// launch.
	retiredSession uint32
	config         string // main class being debugged
	tabName        string
	view           debugView
	console        debugConsoleView
	memory         debugMemoryView

	// lastID/lastRaw/lastKind remember the configuration this pane debugged,
	// so the widget's rerun restarts it as saved now.
	lastID   string
	lastRaw  json.RawMessage
	lastKind string
	// pluginExit is the completion callback passed to debug.pluginStart. It is
	// consumed on the first terminated/exited event, as OpenIDEA's debug
	// session subscription does.
	pluginExit    func(code *int32, stopped bool)
	pluginStopped bool
	pluginRequest uint64
	// launchCwd is the session's working directory (its module dir); the
	// HotSwap compile covers it.
	launchCwd string
	// reloading guards HotSwapReload against concurrent compiles; hotSwapBuild
	// remembers the last HotSwap compile's status for the acceptance run.
	reloading    bool
	hotSwapBuild string
	hotSwapStage string
	// hotSwapRequest is the cancellable java_build_workspace request behind
	// Reload Changed Classes.
	hotSwapRequest      uint32
	hotSwapCancel       chan struct{}
	hotSwapNeedsRestart bool
	hotSwapQuietUntil   time.Time
	buildToolDebug      *debugBuildToolState

	paused     bool
	pausing    bool
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
	// varContainer is the variablesReference of the scope shown in d.vars.
	varContainer int64
	selVar       int

	output    []outputLine
	truncated bool
	err       string
	status    string

	// bps maps an absolute path to its 0-based breakpoint lines.
	bps map[string][]int
	// muted keeps the breakpoints visible while the debugger ignores them.
	muted bool
	// temporaryBps records which of bps are removed after their first stop.
	temporaryBps map[string][]int
	// breakpointOptions keeps conditions, hit counts, log messages and the
	// per-line enabled state beside the line list used by the gutter.
	breakpointOptions map[string]map[int]debugBreakpointOptions
	// methodBps and fieldBps are the special breakpoints the adapter owns.
	methodBps []openidea.FunctionBreakpoint
	fieldBps  []openidea.DataBreakpoint
	// breakpointEdit is the modal editor opened from View Breakpoints.
	breakpointEdit debugBreakpointEditState

	// watches are expressions the user pinned; they evaluate again in each
	// stopped frame. evalText is the input being typed for a new one.
	watches  []debugWatch
	evalText string
	evalBusy bool
	evaluate debugEvaluateState

	setValue        debugSetValueState
	smartStep       debugSmartStepState
	breakpointsOpen bool
	heap            debugHeapState
}

type debugBreakpointOptions struct {
	Enabled      bool
	Condition    string
	HitCondition string
	LogMessage   string
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

const (
	debugJavaPreparationRetryWindow = 2 * time.Minute
	debugJavaPreparationRetryMax    = 2 * time.Second
)

// resetDebug drops the previous project's debug state, breakpoints included:
// a line number means nothing in another project.
func (m *Model) resetDebug() {
	if m.debug.startCancel != nil {
		m.debug.startCancel()
	}
	if m.debug.hotSwapRequest != 0 {
		request := m.debug.hotSwapRequest
		go func() {
			ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
			defer cancel()
			_, _ = m.invoke(ctx, "cancel_request", map[string]any{"id": request})
		}()
	}
	sessions := map[uint32]bool{}
	if m.debug.session != 0 {
		sessions[m.debug.session] = true
	}
	for _, state := range m.debug.sessions {
		if state != nil && state.session != 0 && state.parent == 0 {
			sessions[state.session] = true
		}
	}
	m.releaseDebugHeapChannels(0)
	m.debug = DebugPane{stopLine: -1}
	for session := range sessions {
		go func(id uint32) {
			ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
			defer cancel()
			_, _ = m.invoke(ctx, "debug_stop", map[string]any{"session": id})
		}(session)
	}
}

// breakpointLines returns a copy of path's lines, sorted.
func (p *DebugPane) breakpointLines(path string) []int {
	lines := append([]int(nil), p.bps[path]...)
	sort.Ints(lines)
	return lines
}

func (p *DebugPane) breakpointOptionsAt(path string, line int) debugBreakpointOptions {
	options := debugBreakpointOptions{Enabled: true}
	if byLine := p.breakpointOptions[path]; byLine != nil {
		if saved, ok := byLine[line]; ok {
			return saved
		}
	}
	return options
}

func (p *DebugPane) setBreakpointOptions(path string, line int, options debugBreakpointOptions) {
	defaults := debugBreakpointOptions{Enabled: true}
	if options == defaults {
		p.removeBreakpointOptions(path, line)
		return
	}
	if p.breakpointOptions == nil {
		p.breakpointOptions = map[string]map[int]debugBreakpointOptions{}
	}
	if p.breakpointOptions[path] == nil {
		p.breakpointOptions[path] = map[int]debugBreakpointOptions{}
	}
	p.breakpointOptions[path][line] = options
}

func (p *DebugPane) removeBreakpointOptions(path string, line int) {
	byLine := p.breakpointOptions[path]
	if byLine == nil {
		return
	}
	delete(byLine, line)
	if len(byLine) == 0 {
		delete(p.breakpointOptions, path)
	}
}

// toggleBreakpointAt adds or removes a breakpoint on a 0-based line, then
// gives the file's whole list to the backend (which keeps it for sessions).
func (m *Model) toggleBreakpointAt(path string, line int) {
	if line < 0 || (!isJavaSource(path) && !m.prefs.Debugger.BreakpointsAnyFile) {
		m.status = "Enable Allow placing breakpoints in any file to use this file"
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
		m.debug.removeBreakpointOptions(path, line)
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
		options := m.debug.breakpointOptionsAt(path, line)
		bp := openidea.Breakpoint{
			Line:         line,
			Condition:    breakpointString(options.Condition),
			HitCondition: breakpointString(options.HitCondition),
			LogMessage:   breakpointString(options.LogMessage),
			Enabled:      options.Enabled && !m.debug.muted,
		}
		list = append(list, bp)
	}
	if list == nil {
		list = []openidea.Breakpoint{}
	}
	_, err := m.invoke(ctx, "debug_set_breakpoints", map[string]any{
		"path": path, "breakpoints": list,
	})
	return err
}

func breakpointString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

// ToggleBreakpointsMuted is OpenIDEA's Mute Breakpoints: the markers stay in
// place, but the adapter receives them disabled until the command runs again.
func (m *Model) ToggleBreakpointsMuted() {
	m.debug.muted = !m.debug.muted
	m.refreshDecorations()
	m.invalidate()
	paths := make([]string, 0, len(m.debug.bps))
	for path := range m.debug.bps {
		paths = append(paths, path)
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		for _, path := range paths {
			_ = m.pushFileBreakpoints(ctx, path)
		}
	}()
}

// refreshDecorations paints breakpoints and Git changes in the gutter, and the
// paused line behind the code of every open tab.
func (m *Model) refreshDecorations() {
	for _, t := range m.allTabs() {
		m.refreshTabDecorations(t)
	}
}

func (m *Model) refreshTabDecorations(t *Tab) {
	if t == nil || t.doc == nil || t.state == nil {
		return
	}
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
	for _, mark := range m.coverageGutterMarksForTab(t) {
		if !seen[mark.Line] {
			marks = append(marks, mark)
			seen[mark.Line] = true
		}
	}
	for _, mark := range t.gitMarks {
		if !seen[mark.Line] {
			marks = append(marks, mark)
			seen[mark.Line] = true
		}
	}
	marks = append(marks, m.bookmarkMarks(path, seen)...)
	for _, mark := range m.pluginGutterMarks(t) {
		if seen[mark.Line] {
			continue
		}
		marks = append(marks, mark)
		seen[mark.Line] = true
	}
	for _, mark := range m.colorPreviewGutterMarks(t, seen) {
		marks = append(marks, mark)
		seen[mark.Line] = true
	}
	t.state.GutterMarks = marks
	var bgs []editor.LineBackground
	if m.debug.paused && m.debug.stopFile == path && m.debug.stopLine >= 0 {
		bgs = append(bgs, editor.LineBackground{
			First: m.debug.stopLine, Last: m.debug.stopLine, Color: stopLineColor,
		})
	}
	pluginLines, _ := m.pluginDecorationBackgrounds(t)
	bgs = append(bgs, pluginLines...)
	t.state.LineBackgrounds = bgs
	m.refreshDebugInlays(t)
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
	vmArgs = append(debugAttachListenerArgs(rc, strings.Join(vmArgs, " ")), vmArgs...)
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
func (m *Model) reportDebugConsole(ctx context.Context, root string) func(text, kind string) {
	return func(text, kind string) {
		m.post(func() {
			if ctx.Err() != nil || m.Root() != root {
				return
			}
			if kind == "" {
				kind = "system"
			}
			if text = strings.TrimSuffix(text, "\n"); text != "" {
				m.debug.appendOutput(text, kind)
			}
			m.invalidate()
		})
	}
}

func (d *DebugPane) appendOutput(text, kind string) {
	appendDebugOutput(&d.output, &d.truncated, text, kind)
	if !d.console.noFollow && !d.console.searching {
		d.console.list.ScrollToEnd()
	}
}

// nextRequest allocates an id for a cancellable backend request.
func (m *Model) nextRequest() uint32 { return m.reqSeq.Add(1) }

// ShowDebug toggles the Debug tool window.
func (m *Model) ShowDebug() { m.dockActivate("debug") }

// Debug is run.debug: debug the selected run configuration (upstream
// debugSelected). The active file's own configuration is run.debugContext.
func (m *Model) Debug() {
	if m.busy {
		m.invalidate()
		return
	}
	cfg := m.run.current()
	if m.run.currentFile {
		m.DebugContext()
		return
	}
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

// DebugConfiguration is OpenIDEA's run.debugConfig/debug.startConfig action:
// an explicit configuration starts without changing the user's selection.
func (m *Model) DebugConfiguration(arg any) {
	if arg == nil {
		m.Debug()
		return
	}
	if id, ok := arg.(string); ok && strings.TrimSpace(id) != "" {
		if m.run.loaded && m.runChooserByID(id) == nil {
			m.notifyInfo("run-config", "Run configuration not found", id)
			return
		}
		m.selectRunConfig(id)
		m.Debug()
		return
	}
	if cfg, _, ok := runConfigFromActionArg(arg); ok {
		m.DebugRunConfig(&cfg)
	}
}

// DebugRunConfig starts a java-debug session for an arbitrary run
// configuration, the port of upstream javaLaunchConfig: JVM applications go
// through the shared PrepareJavaLaunch, a kind GoIDEA cannot debug is
// reported instead of silently starting.
func (m *Model) DebugRunConfig(rc *openidea.RunConfig) {
	_ = m.debugRunConfig(rc, true)
}

// debugRunConfig keeps the debug console available to Services without
// bringing the Debug tool window to the front.
func (m *Model) debugRunConfig(rc *openidea.RunConfig, show bool) bool {
	root := m.Root()
	show = m.showRunWindow(rc, show)
	if isRemoteDebugKind(rc.Kind) {
		m.startRemoteDebug(rc, root, show)
		return true
	}
	if debugBuildToolKind(rc) != "" {
		m.startBuildToolDebug(rc, root, show)
		return true
	}
	if m.canStartDebugWithWindow(root, show) {
		m.startJavaDebug(rc, root)
		return true
	}
	return false
}

func (m *Model) canStartDebug(root string) bool {
	return m.canStartDebugWithWindow(root, true)
}

func (m *Model) canStartDebugWithWindow(root string, show bool) bool {
	if show {
		m.DockShow("debug")
	}
	if root == "" || m.busy {
		m.invalidate()
		return false
	}
	if !m.run.trusted {
		m.debug.err = "The project is not trusted: allow building and debugging first"
		m.invalidate()
		return false
	}
	if m.debug.starting {
		return false
	}
	return true
}

// waitForJavaDebugReady closes the window between classpath preparation and
// java-debug's startDebugSession command. jdtls may still report Busy while
// indexing immediately after it has resolved the module, and asking it to
// start a debug server in that state makes OpenIDEA refuse a launch that
// would succeed a moment later. Only Busy and Starting are wait states: other
// verdicts go to the backend so Missing, Crashed and Misconfigured keep their
// precise errors.
func (m *Model) waitForJavaDebugReady(ctx context.Context) error {
	for {
		callCtx, cancel := context.WithTimeout(ctx, lspTimeout)
		raw, err := m.invoke(callCtx, "lsp_servers", nil)
		cancel()
		if err != nil {
			return fmt.Errorf("could not check the Java project import: %w", err)
		}
		var servers []lspServerState
		if err := json.Unmarshal(raw, &servers); err != nil {
			return fmt.Errorf("could not read the Java project import: %w", err)
		}
		if !javaDebugProjectBusy(servers) {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("the Java project did not finish importing before Debug started: %w", ctx.Err())
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// javaDebugProjectBusy is true while every Java server is in an import state.
// A ready or failed server is left for the backend call to handle immediately.
func javaDebugProjectBusy(servers []lspServerState) bool {
	waiting := false
	for _, server := range servers {
		if !hasLanguage(server.Languages, "java") {
			continue
		}
		if server.Status == "busy" || server.Status == "starting" {
			waiting = true
			continue
		}
		return false
	}
	return waiting
}

// prepareJavaDebugLaunch retries the launch preparation when jdtls is still
// starting and OpenIDEA reports a transient cancelled request. Bind this retry
// to the caller's context and a short window so a real stop remains immediate
// and an unsupported project keeps its precise backend error.
func (m *Model) prepareJavaDebugLaunch(ctx context.Context, rc *openidea.RunConfig, root string) (*adapter.PreparedJavaLaunch, error) {
	deadline := time.Now().Add(debugJavaPreparationRetryWindow)
	delay := 200 * time.Millisecond
	report := m.reportDebugConsole(ctx, root)
	for {
		prepared, err := adapter.PrepareJavaLaunch(ctx, javaBackend{m}, rc, &adapter.Preparation{
			Request: m.nextRequest(),
			Report:  report,
		})
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		if err == nil || openidea.ErrorKind(err) != "cancelled" ||
			time.Now().After(deadline) {
			return prepared, err
		}
		// A Java server that is visible as busy or starting can be awaited
		// directly. Before it appears, the bounded delay below leaves the
		// import a chance to register the server.
		_ = m.waitForJavaDebugReady(ctx)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
		if delay < debugJavaPreparationRetryMax {
			delay *= 2
			if delay > debugJavaPreparationRetryMax {
				delay = debugJavaPreparationRetryMax
			}
		}
	}
}

func (m *Model) startJavaDebug(rc *openidea.RunConfig, root string) {
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
	m.debug.rememberDebugSession()
	m.debug.active = 0
	m.debug.retiredSession = 0
	m.debug.resetActiveDebugFields()
	m.debug.lastID = rc.ID
	m.debug.lastRaw = append(json.RawMessage(nil), rc.Raw()...)
	m.debug.lastKind = rc.Kind
	m.debug.config = adapter.ParseJavaArgs(rc.Args).MainClass
	m.debug.tabName = rc.Name
	m.debug.view = debugViewConsole
	m.debug.console = debugConsoleView{}
	m.debug.memory = debugMemoryView{}
	m.debug.starting = true
	m.debug.output = []outputLine{{kind: "system", text: "Starting debugger for " + rc.Name + "…"}}
	m.debug.status = "Saving and building…"
	m.refreshDecorations()
	m.invalidate()

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
				if ctx.Err() == context.Canceled || m.Root() != root || !m.debug.starting {
					if m.Root() == root && !m.debug.starting {
						m.finishPluginDebug(nil)
					}
					return
				}
				m.debug.starting = false
				m.debug.startCancel = nil
				m.debug.err = text
				m.debug.status = ""
				m.finishPluginDebug(nil)
				m.invalidate()
			})
		}
		// The build reads the files from disk.
		if err := m.saveOpenTabs(ctx, tabs); err != nil {
			fail("Saving before debugging failed", err)
			return
		}
		_ = m.pushBreakpoints(ctx, "")
		// The launch RPC normally waits for the target module's import. A
		// cancelled verdict during jdtls startup is retried within a bound.
		prepared, err := m.prepareJavaDebugLaunch(ctx, rc, root)
		if err != nil {
			fail(err.Error(), nil)
			return
		}
		if err := m.waitForJavaDebugReady(ctx); err != nil {
			fail("Java project import failed", err)
			return
		}
		if prepared.Info.ErrorCount > 0 {
			m.post(func() {
				m.debug.status = "There are compilation errors; debugging the last compiled classes"
				m.invalidate()
			})
		}
		launch, err := m.javaDebugLaunch(ctx, rc, root, prepared, m.reportDebugConsole(ctx, root))
		if err != nil {
			fail(err.Error(), nil)
			return
		}
		if err := ctx.Err(); err != nil {
			fail("Starting the debug session failed", err)
			return
		}
		rawSession, err := m.invoke(ctx, "debug_start", map[string]any{"config": launch})
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
			if ctx.Err() != nil || m.Root() != root {
				// The project changed, or the user stopped while it started:
				// stop the backend session at once.
				go func(id uint32) {
					ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
					defer cancel()
					_, _ = m.invoke(ctx, "debug_stop", map[string]any{"session": id})
				}(session)
				return
			}
			if m.debug.retiredSession == session || (m.debug.ended && m.debug.session == 0) {
				// A short program can terminate before the debug_start reply
				// reaches the UI. The terminated/exited event already retired
				// this session, so do not resurrect it or block the next launch.
				m.debug.starting = false
				m.debug.startCancel = nil
				m.invalidate()
				return
			}
			if !m.debug.starting {
				go func(id uint32) {
					ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
					defer cancel()
					_, _ = m.invoke(ctx, "debug_stop", map[string]any{"session": id})
				}(session)
				return
			}
			m.debug.starting = false
			m.debug.startCancel = nil
			m.debug.retiredSession = 0
			if m.debug.active == 0 {
				m.debug.adoptDebugSession(session)
				m.debug.session = session
			} else if m.debug.active == session {
				m.debug.session = session
			}
			m.debug.status = "Debugging started: " + m.debug.config
			m.recordRecentRunConfig(rc.ID)
			m.debug.rememberDebugSession()
			m.pushSpecialBreakpoints()
			m.refreshDebugCapabilities(session)
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
		ctx, cancel := context.WithTimeout(modelContext(m), 30*time.Second)
		defer cancel()
		if !m.waitForDebugSessionStop(ctx, session) {
			m.post(func() {
				if m.debug.session == session && !m.debug.ended {
					if m.debug.err == "" {
						m.debug.err = "The previous debug session did not stop, so it was not restarted"
					}
					m.invalidate()
				}
			})
			return
		}
		m.post(func() {
			if m.debug.session == 0 || m.debug.ended {
				m.DebugRunConfig(cfg)
			}
		})
	}()
}

func (m *Model) waitForDebugSessionStop(ctx context.Context, session uint32) bool {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		state := make(chan struct {
			stopped bool
			failed  bool
		}, 1)
		m.post(func() {
			stopped := m.debug.ended || m.debug.session != session && m.debug.session == 0
			failed := !stopped && m.debug.session == session && !m.debug.stopping
			state <- struct {
				stopped bool
				failed  bool
			}{stopped: stopped, failed: failed}
		})
		select {
		case current := <-state:
			if current.stopped {
				return true
			}
			if current.failed {
				return false
			}
		case <-ctx.Done():
			return false
		case <-ticker.C:
		}
	}
}

// Continue resumes the paused thread.
func (m *Model) DebugContinue() {
	if m.debug.session == 0 || !m.debug.paused {
		return
	}
	session, thread := m.debug.session, m.debug.stopThread
	m.debug.stopGeneration++
	m.debug.threadRequest++
	m.debug.frameRequest++
	m.debug.status = "Resuming…"
	m.debug.rememberDebugSession()
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		if _, err := m.invoke(ctx, "debug_resume", map[string]any{"session": session, "threadId": thread}); err != nil {
			m.post(func() {
				m.updateDebugSession(session, func(state *debugSessionState) {
					if state.session == session && !state.ended {
						state.err = "Resume failed: " + err.Error()
					}
				})
				m.invalidate()
			})
		}
	}()
}

// DebugPause suspends the program.
func (m *Model) DebugPause() {
	d := &m.debug
	if d.session == 0 || d.paused || d.ended || d.pausing {
		return
	}
	session, thread := d.session, d.stopThread
	d.pausing = true
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		var err error
		if thread == 0 {
			var raw json.RawMessage
			raw, err = m.invoke(ctx, "debug_threads", map[string]any{"session": session})
			if err == nil {
				var threads []openidea.ThreadInfo
				err = json.Unmarshal(raw, &threads)
				if err == nil && len(threads) == 0 {
					err = errors.New("There is no thread to pause")
				}
				if err == nil {
					thread = threads[0].ID
				}
			}
		}
		if err == nil {
			_, err = m.invoke(ctx, "debug_pause", map[string]any{"session": session, "threadId": thread})
		}
		m.post(func() {
			m.updateDebugSession(session, func(state *debugSessionState) {
				if state.session != session {
					return
				}
				state.pausing = false
				if err != nil && !state.ended {
					state.err = "Pause failed: " + err.Error()
				}
			})
			m.invalidate()
		})
	}()
}

// DebugStep runs one step of kind ("over", "into" or "out") in the paused
// thread.
func (m *Model) DebugStep(kind string) {
	if m.debug.session == 0 || !m.debug.paused || m.debug.stopThread == 0 {
		return
	}
	session, thread := m.debug.session, m.debug.stopThread
	m.debug.stopGeneration++
	m.debug.threadRequest++
	m.debug.frameRequest++
	m.debug.status = "Stepping…"
	m.debug.paused = false
	m.debug.rememberDebugSession()
	m.refreshDecorations()
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		if _, err := m.invoke(ctx, "debug_step", map[string]any{
			"session": session, "threadId": thread, "kind": kind,
		}); err != nil {
			m.post(func() {
				m.updateDebugSession(session, func(state *debugSessionState) {
					if state.session == session && !state.ended {
						state.paused = true
						state.err = "Step failed: " + err.Error()
					}
				})
				if m.debug.active == session {
					m.refreshDecorations()
				}
				m.invalidate()
			})
		}
	}()
}

// StepOver, StepInto and StepOut are the menu's and keyboard's steps.
func (m *Model) StepOver() { m.DebugStep("over") }
func (m *Model) StepInto() { m.DebugStep("into") }
func (m *Model) StepOut()  { m.DebugStep("out") }

// ForceStepOver uses DAP's ignore-breakpoints step when the adapter supports
// it. OpenIDEA's command is distinct from the normal Step Over for this reason.
func (m *Model) ForceStepOver() {
	if m.debug.session == 0 || !m.debug.paused || m.debug.stopThread == 0 {
		return
	}
	session, thread := m.debug.session, m.debug.stopThread
	m.debug.stopGeneration++
	m.debug.threadRequest++
	m.debug.frameRequest++
	m.debug.status = "Force-stepping over…"
	m.debug.paused = false
	m.debug.rememberDebugSession()
	m.refreshDecorations()
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		if _, err := m.invoke(ctx, "debug_force_step_over", map[string]any{
			"session": session, "threadId": thread,
		}); err != nil {
			m.post(func() {
				m.updateDebugSession(session, func(state *debugSessionState) {
					if state.session == session && !state.ended {
						state.paused = true
						state.err = "Force Step Over failed: " + err.Error()
					}
				})
				if m.debug.active == session {
					m.refreshDecorations()
				}
				m.invalidate()
			})
		}
	}()
}

// DAP has no generic ignore-filters flag for Step Into. OpenIDEA's action uses
// the adapter's own filters, so the normal request is the portable equivalent.
func (m *Model) ForceStepInto() { m.StepInto() }

// RemoveAllBreakpoints clears every stored breakpoint and pushes the empty set
// to each file the backend knows about.
func (m *Model) RemoveAllBreakpoints() {
	paths := make([]string, 0, len(m.debug.bps))
	for path := range m.debug.bps {
		paths = append(paths, path)
	}
	m.debug.bps = map[string][]int{}
	m.debug.temporaryBps = map[string][]int{}
	m.debug.breakpointOptions = nil
	m.debug.methodBps = nil
	m.debug.fieldBps = nil
	m.refreshDecorations()
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		for _, path := range paths {
			_ = m.pushFileBreakpoints(ctx, path)
		}
		_, _ = m.invoke(ctx, "debug_set_function_breakpoints", map[string]any{
			"breakpoints": []openidea.FunctionBreakpoint{},
		})
		_, _ = m.invoke(ctx, "debug_set_data_breakpoints", map[string]any{
			"breakpoints": []openidea.DataBreakpoint{},
		})
	}()
}

// RunToCursor continues to the caret's line (Alt+F9); force ignores
// breakpoints on the way (Cmd+Alt+F9).
func (m *Model) RunToCursor(force bool) {
	t := m.tab()
	if t == nil || t.doc == nil || t.state == nil {
		if m.debug.session != 0 && m.debug.paused {
			m.debug.err = "No file is open, so Run to Cursor has no place to go"
			m.invalidate()
		}
		return
	}
	m.runToCursorAtLine(t.doc.Path(), t.state.CursorPos().Line, force)
}

// runToCursorAtLine is shared by Alt+F9 and the native Run to Cursor hover.
func (m *Model) runToCursorAtLine(path string, line int, force bool) {
	if m.debug.session == 0 || !m.debug.paused || m.debug.stopThread == 0 {
		return
	}
	if path == "" || line < 0 {
		m.debug.err = "Run to Cursor needs a source line"
		m.invalidate()
		return
	}
	session, thread := m.debug.session, m.debug.stopThread
	m.debug.stopGeneration++
	m.debug.threadRequest++
	m.debug.frameRequest++
	m.debug.status = "Running to the cursor…"
	m.debug.paused = false
	m.debug.rememberDebugSession()
	m.refreshDecorations()
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
		defer cancel()
		if _, err := m.invoke(ctx, "debug_run_to", map[string]any{
			"session": session, "threadId": thread, "path": path, "line": line, "force": force,
		}); err != nil {
			m.post(func() {
				m.updateDebugSession(session, func(state *debugSessionState) {
					if state.session == session && !state.ended {
						state.paused = true
						state.err = "Run to Cursor failed: " + err.Error()
					}
				})
				if m.debug.active == session {
					m.refreshDecorations()
				}
				m.invalidate()
			})
		}
	}()
}

// ForceRunToCursor runs to the caret ignoring breakpoints.
func (m *Model) ForceRunToCursor() { m.RunToCursor(true) }

// StopDebug stops the session or cancels pending Java launch preparation.
func (m *Model) StopDebug() {
	session := m.debug.session
	if m.debug.active == 0 && session != 0 {
		m.debug.active = session
		m.debug.ensureDebugSessionOrder(session)
	}
	if m.debug.starting {
		if m.debug.startCancel != nil {
			m.debug.startCancel()
			m.debug.startCancel = nil
		}
		if m.debug.pluginExit != nil {
			m.debug.pluginStopped = true
		}
		m.debug.starting = false
		m.debug.status = ""
		m.debug.err = ""
		m.finishPluginDebug(nil)
		m.invalidate()
	}
	if session == 0 || m.debug.ended {
		return
	}
	if m.debug.pluginExit != nil {
		m.debug.pluginStopped = true
	}
	if m.mgr == nil && m.invokeHook == nil {
		m.debug.stopping = false
		m.invalidate()
		return
	}
	m.debug.stopping = true
	m.debug.status = "Stopping the debug session…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 30*time.Second)
		defer cancel()
		if _, err := m.invoke(ctx, "debug_stop", map[string]any{"session": session}); err != nil {
			m.post(func() {
				m.updateDebugSession(session, func(state *debugSessionState) {
					if state.session != session {
						return
					}
					state.stopping = false
					delete(m.debug.closeAfterStop, session)
					state.err = "Stopping the debug session failed: " + err.Error()
				})
				if m.debug.active == session {
					m.debug.retiredSession = 0
				}
				m.invalidate()
			})
			return
		}
		m.post(func() {
			m.markDebugSessionRetired(session)
			m.updateDebugSession(session, func(state *debugSessionState) {
				if state.session != session {
					return
				}
				state.session = 0
				state.ended = true
				state.stopping = false
				state.paused = false
				state.status = "Debugging stopped"
				state.stopGeneration++
				state.threadRequest++
				state.frameRequest++
				state.memory.invalidateInspection()
			})
			if m.debug.active == session {
				m.debug.retiredSession = session
			}
			m.endDebugHeapOperation(session)
			if m.debug.closeAfterStop[session] {
				delete(m.debug.closeAfterStop, session)
				m.closeDebugSessionTab(session)
				return
			}
			if m.debug.active == session {
				m.refreshDecorations()
			}
			m.invalidate()
		})
	}()
}

// acceptDebugEvent applies one adapter event of the GoIDEA window's session.
func (m *Model) acceptDebugEvent(session uint32, raw json.RawMessage) {
	if session == 0 {
		return
	}
	var e openidea.DebugEvent
	if json.Unmarshal(raw, &e) != nil {
		return
	}
	if m.debug.retiredSessions[session] {
		if session == m.debug.retiredSession && e.Kind == "output" {
			text := strings.TrimSuffix(stripANSI(e.Output), "\n")
			if text != "" {
				kind := e.Category
				if kind == "" {
					kind = "console"
				}
				m.debug.appendOutput(text, kind)
				m.invalidate()
			}
		}
		return
	}
	d := &m.debug
	if d.sessions != nil && d.sessions[session] != nil {
		if d.active != session && m.acceptBackgroundDebugEvent(session, e) {
			return
		}
	}
	if d.sessions == nil {
		d.sessions = map[uint32]*debugSessionState{}
	}
	if d.sessions[session] == nil && e.Kind == "started" && e.Parent != nil {
		if !m.debugSessionOwns(*e.Parent) {
			return
		}
		state := &debugSessionState{
			session:  session,
			parent:   *e.Parent,
			adapter:  e.Adapter,
			config:   e.Name,
			tabName:  e.Name,
			view:     debugViewVariables,
			status:   "Debugging started",
			stopLine: -1,
		}
		d.sessions[session] = state
		d.ensureDebugSessionOrder(session)
		if d.active == 0 && !d.starting {
			d.restoreDebugSession(*state)
			d.active = session
		}
		m.refreshDebugCapabilities(session)
		m.invalidate()
		return
	}
	if d.sessions[session] == nil && d.starting {
		d.adoptDebugSession(session)
		d.active = session
		d.session = session
		d.parent = 0
		d.adapter = e.Adapter
	}
	if d.active == 0 && d.session == session {
		d.active = session
		d.ensureDebugSessionOrder(session)
	}
	if d.active != 0 && d.active != session {
		if m.acceptBackgroundDebugEvent(session, e) {
			return
		}
		return
	}
	if session == d.retiredSession {
		if e.Kind == "output" {
			text := strings.TrimSuffix(stripANSI(e.Output), "\n")
			if text != "" {
				kind := e.Category
				if kind == "" {
					kind = "console"
				}
				d.appendOutput(text, kind)
				m.invalidate()
			}
		}
		return
	}
	if d.active == 0 {
		if !d.starting {
			return
		}
		d.adoptDebugSession(session)
		d.session = session
	}
	if session != d.active {
		return
	}
	d.rememberDebugSession()
	if d.stopping && e.Kind != "terminated" && e.Kind != "exited" {
		return
	}
	switch e.Kind {
	case "started":
		d.visible = true
		m.occupyBottom("debug")
		d.ended = false
		d.adapter = e.Adapter
		if e.Parent != nil {
			d.parent = *e.Parent
		}
		if e.Name != "" {
			d.config = e.Name
			d.tabName = e.Name
		}
		d.status = "Debugging started"
		m.refreshDebugCapabilities(session)
	case "initialized":
		if d.status == "" || strings.HasPrefix(d.status, "Starting") ||
			strings.HasPrefix(d.status, "Debugging started") {
			d.status = "Running…"
		}
	case "stopped":
		d.stopGeneration++
		d.threadRequest++
		d.frameRequest++
		d.memory.invalidateInspection()
		if e.Reason != "step" && d.view != debugViewMemory {
			d.view = debugViewVariables
		}
		d.paused = true
		d.ended = false
		d.stopThread = e.ThreadID
		d.reason = e.Reason
		d.frames = nil
		d.scopes = nil
		d.vars = nil
		d.frameID = 0
		d.varContainer = 0
		d.stopLine = -1
		d.stopFile = ""
		d.status = "Paused"
		if e.Description != "" {
			d.status = "Paused: " + e.Description
		}
		if m.prefs.Debugger.ShowOnBreakpoint() {
			m.DockShow("debug")
		}
		if m.prefs.Debugger.FocusAppOnBreakpoint && m.win != nil {
			m.win.Focus()
		}
		d.rememberDebugSession()
		m.fetchStop(session, e.ThreadID, d.stopGeneration)
	case "continued":
		d.stopGeneration++
		d.threadRequest++
		d.frameRequest++
		d.memory.invalidateInspection()
		d.paused = false
		d.setValue.open = false
		d.smartStep.open = false
		d.frames = nil
		d.scopes = nil
		d.vars = nil
		d.stopLine = -1
		d.stopFile = ""
		d.frameID = 0
		d.varContainer = 0
		m.clearWatchValues()
		d.status = "Running…"
	case "output":
		text := strings.TrimSuffix(stripANSI(e.Output), "\n")
		if text != "" {
			kind := e.Category
			if kind == "" {
				kind = "console"
			}
			d.appendOutput(text, kind)
		}
	case "thread":
		if d.paused {
			m.fetchDebugThreads(session, d.threadRequest)
		}
	case "terminated":
		m.markDebugSessionRetired(session)
		m.endDebugHeapOperation(session)
		d.stopGeneration++
		d.threadRequest++
		d.frameRequest++
		d.memory.invalidateInspection()
		d.starting = false
		d.startCancel = nil
		d.session = 0
		d.retiredSession = session
		d.stopping = false
		d.ended = true
		d.paused = false
		d.setValue.open = false
		d.smartStep.open = false
		d.frames = nil
		d.scopes = nil
		d.vars = nil
		d.stopLine = -1
		d.frameID = 0
		d.varContainer = 0
		m.clearWatchValues()
		d.status = "Debugging ended"
		m.finishPluginDebug(nil)
		d.rememberDebugSession()
	case "exited":
		m.markDebugSessionRetired(session)
		m.endDebugHeapOperation(session)
		d.stopGeneration++
		d.threadRequest++
		d.frameRequest++
		d.memory.invalidateInspection()
		d.starting = false
		d.startCancel = nil
		d.session = 0
		d.retiredSession = session
		d.stopping = false
		d.ended = true
		d.paused = false
		d.frames = nil
		d.scopes = nil
		d.vars = nil
		d.stopLine = -1
		d.stopFile = ""
		d.frameID = 0
		d.varContainer = 0
		m.clearWatchValues()
		d.status = "The program exited"
		if e.Code != nil {
			d.status = fmt.Sprintf("The program exited (code %d)", *e.Code)
		}
		m.finishPluginDebug(e.Code)
		d.rememberDebugSession()
	case "breakpointsChanged":
		// Verification is refetched by the file's next push; nothing to do
		// in this panel yet.
	case "hotCodeReplace":
		// java-debug reports after redefineClasses and after jdtls's own
		// build (BUILD_COMPLETE): offer the reload, as IDEA's default
		// "Reload classes after compilation: Ask" does.
		if e.ChangeType == "BUILD_COMPLETE" {
			m.offerHotSwapAfterBuild(session)
		} else if e.Message != "" {
			d.status = "Hot swap: " + e.Message
		}
	case "capabilities":
		m.refreshDebugCapabilities(session)
	case "invalidated":
		if d.paused && debugInvalidatedVariables(e.Areas) {
			d.frameRequest++
			if frameID := d.frameID; frameID != 0 {
				m.loadDebugScopes(session, frameID, d.frameRequest)
			}
		}
	case "jvmBackend":
		if e.JVMBackend != nil {
			m.acceptJVMBackend(session, *e.JVMBackend)
		}
	}
	d = &m.debug
	d.rememberDebugSession()
	if (e.Kind == "terminated" || e.Kind == "exited") &&
		m.prefs.Debugger.HideOnTermination && !m.debugHasLiveSessions() {
		m.DockHide("debug")
	}
	if d.closeAfterStop[session] {
		delete(d.closeAfterStop, session)
		m.closeDebugSessionTab(session)
		return
	}
	m.refreshDecorations()
	m.invalidate()
}

func (m *Model) finishPluginDebug(code *int32) {
	done := m.debug.pluginExit
	if done == nil {
		return
	}
	stopped := m.debug.pluginStopped
	m.debug.pluginExit = nil
	m.debug.pluginStopped = false
	done(code, stopped)
}

// fetchStop loads the thread list, the stopped thread's frames, and the first
// scope's variables for the session and stop that produced the event. Replies
// from an earlier stop are discarded even if that session is active again.
func (m *Model) fetchStop(session uint32, threadID int64, generation uint64) {
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		var threads []openidea.ThreadInfo
		if raw, err := m.invoke(ctx, "debug_threads", map[string]any{"session": session}); err == nil {
			_ = json.Unmarshal(raw, &threads)
		}
		frames, scopes, vars := m.loadFrames(ctx, session, threadID, 0)
		m.post(func() {
			current := m.debugSessionState(session)
			if current == nil || current.session != session || current.ended ||
				!current.paused || current.stopGeneration != generation ||
				current.stopThread != threadID {
				return
			}
			m.updateDebugSession(session, func(state *debugSessionState) {
				state.threads = cloneThreads(threads)
				state.frames = cloneFrames(frames)
				state.selFrame = 0
				state.scopes = append([]openidea.Scope(nil), scopes...)
				state.vars = append([]openidea.Variable(nil), vars...)
				state.varContainer = debugScopeContainer(scopes)
				state.selVar = 0
				state.frameID = 0
				if len(frames) > 0 {
					state.frameID = frames[0].ID
				}
			})
			if m.debug.active == session {
				m.applyStopFrame()
				m.evaluateWatches()
			}
			m.invalidate()
		})
	}()
}

// loadFrames fetches a thread's frames and one frame's scopes and variables.
func (m *Model) loadFrames(ctx context.Context, session uint32, threadID int64, index int) ([]openidea.StackFrame, []openidea.Scope, []openidea.Variable) {
	raw, err := m.invoke(ctx, "debug_stack_trace", map[string]any{
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
	raw, err := m.invoke(ctx, "debug_scopes", map[string]any{
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
		rawVars, err := m.invoke(ctx, "debug_variables", map[string]any{
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

func (m *Model) fetchDebugThreads(session uint32, request uint64) {
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "debug_threads", map[string]any{"session": session})
		var threads []openidea.ThreadInfo
		if err == nil {
			err = json.Unmarshal(raw, &threads)
		}
		m.post(func() {
			if err != nil {
				return
			}
			m.updateDebugSession(session, func(state *debugSessionState) {
				if state.threadRequest == request && state.paused && !state.ended {
					state.threads = cloneThreads(threads)
				}
			})
			m.invalidate()
		})
	}()
}

func (m *Model) loadDebugScopes(session uint32, frameID int64, request uint64) {
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		scopes, vars := m.loadScopes(ctx, session, frameID)
		m.post(func() {
			m.updateDebugSession(session, func(state *debugSessionState) {
				if state.frameRequest != request || state.frameID != frameID ||
					!state.paused || state.ended {
					return
				}
				state.scopes = append([]openidea.Scope(nil), scopes...)
				state.vars = append([]openidea.Variable(nil), vars...)
				state.varContainer = debugScopeContainer(scopes)
				state.selVar = 0
			})
			if m.debug.active == session {
				m.evaluateWatches()
			}
			m.invalidate()
		})
	}()
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

// QuickEvaluate evaluates the selection, or the identifier at the caret, as a
// temporary watch in the current paused frame.
func (m *Model) QuickEvaluate() {
	if m.debug.session == 0 || !m.debug.paused {
		return
	}
	t := m.tab()
	if t == nil {
		return
	}
	expr := strings.TrimSpace(t.state.SelectedText())
	if expr == "" {
		expr = wordAt(t.doc.Text(), t.state.Cursor())
	}
	if expr == "" {
		return
	}
	m.DockShow("debug")
	m.addWatch(expr)
}

// ShowExecutionPoint opens the paused frame's source and places the caret on
// the execution line.
func (m *Model) ShowExecutionPoint() {
	if !m.debug.paused || m.debug.stopFile == "" || m.debug.stopLine < 0 {
		return
	}
	m.openFile(m.debug.stopFile)
	m.revealAt(m.debug.stopFile, m.debug.stopLine, 0)
	m.invalidate()
}

// evaluateWatches evaluates every pinned expression in the stopped frame. A
// reply for a frame the user has stepped past is dropped.
func (m *Model) evaluateWatches() {
	d := &m.debug
	if (m.mgr == nil && m.invokeHook == nil) ||
		d.session == 0 || !d.paused || d.frameID == 0 || len(d.watches) == 0 {
		return
	}
	session, frame, generation := d.session, d.frameID, d.stopGeneration
	exprs := make([]string, len(d.watches))
	for i, w := range d.watches {
		exprs[i] = w.Expr
	}
	m.updateDebugSession(session, func(state *debugSessionState) {
		state.evalBusy = true
	})
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		values := make(map[string]debugWatch, len(exprs))
		for _, expr := range exprs {
			w := debugWatch{Expr: expr}
			raw, err := m.invoke(ctx, "debug_evaluate", map[string]any{
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
			m.updateDebugSession(session, func(state *debugSessionState) {
				if state.session != session || state.stopGeneration != generation ||
					state.frameID != frame || !state.paused || state.ended {
					return
				}
				state.evalBusy = false
				for i := range state.watches {
					if w, ok := values[state.watches[i].Expr]; ok {
						state.watches[i].Value = w.Value
						state.watches[i].Err = w.Err
					}
				}
			})
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
	m.removeTemporaryBreakpoint(*f.Path, f.Line)
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
	session, frame := m.debug.session, m.debug.frames[i]
	m.debug.frameRequest++
	request := m.debug.frameRequest
	m.debug.selFrame = i
	m.debug.frameID = frame.ID
	m.applyStopFrame()
	m.refreshDecorations()
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		scopes, vars := m.loadScopes(ctx, session, frame.ID)
		m.post(func() {
			m.updateDebugSession(session, func(state *debugSessionState) {
				if state.frameRequest != request || state.frameID != frame.ID ||
					!state.paused || state.ended {
					return
				}
				state.scopes = append([]openidea.Scope(nil), scopes...)
				state.vars = append([]openidea.Variable(nil), vars...)
				state.varContainer = debugScopeContainer(scopes)
				state.selVar = 0
			})
			if m.debug.active == session {
				m.evaluateWatches()
			}
			m.invalidate()
		})
	}()
}

func debugScopeContainer(scopes []openidea.Scope) int64 {
	for _, scope := range scopes {
		if !scope.Expensive {
			return scope.VariablesReference
		}
	}
	return 0
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
	if _, err := m.invoke(ctx, "debug_set_breakpoints", map[string]any{
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
	prepared, err := m.prepareJavaDebugLaunch(ctx, rc, root)
	if err != nil {
		m.post(func() { m.debug.starting = false })
		return out, err
	}
	if jdk := strings.TrimSpace(os.Getenv("GOIDEA_SMOKE_DEBUG_JDK")); jdk != "" {
		prepared.Command = filepath.Join(jdk, "bin", "java")
		out["jdk"] = jdk
	}
	if prepared.Info.Build != nil {
		out["build"] = *prepared.Info.Build
	}
	out["classPaths"] = len(prepared.Info.ClassPaths)
	out["errorCount"] = prepared.Info.ErrorCount
	launch, err := m.javaDebugLaunch(ctx, rc, root, prepared, m.reportDebugConsole(ctx, root))
	if err != nil {
		m.post(func() { m.debug.starting = false })
		return out, err
	}
	out["debugServer"] = true
	out["extraJson"] = *launch.ExtraJSON
	rawSession, err := m.invoke(ctx, "debug_start", map[string]any{"config": launch})
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
	rawEval, evalErr := m.invoke(ctx, "debug_evaluate", map[string]any{
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

	m.post(func() {
		m.debug.console.expr = "4 + 5"
		m.evaluateDebugConsole()
		m.refreshDebugMemory()
	})
	m.waitDebug(ctx, func(d *DebugPane) bool {
		return !d.console.busy && !d.memory.loading
	}, 35*time.Second)
	m.post(func() {
		out["consoleEval"] = false
		for i, line := range m.debug.output {
			if line.text == "› 4 + 5" && i+1 < len(m.debug.output) {
				out["consoleEval"] = m.debug.output[i+1].text == "9"
				break
			}
		}
		out["memoryRows"] = len(m.debug.memory.snapshot.Rows)
		out["memoryError"] = m.debug.memory.err
	})

	// Exercise the Memory pane against java-debug's suspended JVM, using the
	// same instance, field and referrer requests as the native controls.
	m.post(func() { m.showDebugMemoryInstances("java.lang.String") })
	if !m.waitDebug(ctx, func(d *DebugPane) bool {
		return d.memory.inspection != nil && !d.memory.inspection.loading
	}, 35*time.Second) {
		out["memoryInspectionError"] = "Timed out reading instances"
	} else {
		var node *debugMemoryNode
		var inspection *debugMemoryInspection
		m.post(func() {
			inspection = m.debug.memory.inspection
			out["memoryInstances"] = len(inspection.instances)
			if inspection.err != "" {
				out["memoryInspectionError"] = inspection.err
			}
			if len(inspection.instances) > 0 {
				ref, ok := debugMemoryValueRef(inspection.generation, inspection.instances[0])
				if ok {
					node = &debugMemoryNode{ref: ref}
					inspection.nodes[ref] = node
					m.debugMemoryLoadNode(inspection, node, false)
					m.debugMemoryLoadNode(inspection, node, true)
				}
			}
		})
		if node != nil {
			m.waitDebug(ctx, func(d *DebugPane) bool {
				return d.memory.inspection != inspection || (!node.fieldsLoading && !node.referrersLoading)
			}, 35*time.Second)
			m.post(func() {
				out["memoryFieldsLoaded"] = node.fieldsLoaded
				out["memoryReferrersLoaded"] = node.referrersLoaded
				out["memoryFieldsError"] = node.fieldsError
				out["memoryReferrersError"] = node.referrersError
			})
		}
	}

	// Capture and query a real heap snapshot over the same bounded progress
	// channel and pinned RPCs as the Heap tab. Closing it here also proves the
	// snapshot cleanup that runs when the UI closes a session-owned snapshot.
	heapRequest := m.nextRequest()
	heapChannel := m.nextChannel()
	m.registerDebugHeapChannel(session, heapChannel)
	rawHeap, heapErr := m.invoke(ctx, "debug_heap_capture", map[string]any{
		"session":    session,
		"request":    heapRequest,
		"onProgress": map[string]any{"$channel": heapChannel},
	})
	m.releaseDebugHeapChannel(heapChannel)
	var heapResult debugHeapCaptureResult
	if heapErr == nil {
		heapErr = json.Unmarshal(rawHeap, &heapResult)
	}
	out["heapStatus"] = heapResult.Status
	if heapErr != nil {
		out["heapError"] = heapErr.Error()
	} else if heapResult.Status != "ready" || heapResult.Snapshot == nil {
		out["heapError"] = debugHeapUnavailable(heapResult.Reason)
	} else {
		snapshot := heapResult.Snapshot
		out["heapSnapshot"] = snapshot.ID
		out["heapObjects"] = snapshot.Summary.Objects
		rawClasses, classErr := m.invoke(ctx, "debug_heap_classes", map[string]any{
			"snapshot": snapshot.ID,
		})
		var classes []debugHeapClass
		if classErr == nil {
			classErr = json.Unmarshal(rawClasses, &classes)
		}
		if classErr != nil {
			out["heapClassesError"] = classErr.Error()
		} else {
			out["heapClasses"] = len(classes)
		}
		if _, closeErr := m.invoke(ctx, "debug_heap_close", map[string]any{
			"snapshot": snapshot.ID,
		}); closeErr != nil {
			out["heapCloseError"] = closeErr.Error()
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
		if _, err := m.invoke(ctx, "debug_step", map[string]any{
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
	if _, err := m.invoke(ctx, "debug_resume", map[string]any{
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

// SmokeDebugImport checks that Debug starts Java preparation during import and
// that Stop prevents a pending launch from creating a session.
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
	out["importObserved"] = true
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
	})
	// Stay on the busy edge instead of stopping immediately. A correct launch
	// waits for jdtls to leave Busy, then either starts or reports a real
	// error. Returning at the first error is what made the shipped app show
	// "stopped importing before it was ready (busy)" while the project was
	// still importing normally.
	debugDeadline := time.Now().Add(6 * time.Minute)
	for {
		r := m.readinessNow(ctx)
		var starting, ended bool
		var session uint32
		var debugErr string
		m.post(func() {
			starting, ended = m.debug.starting, m.debug.ended
			session, debugErr = m.debug.session, m.debug.err
		})
		out["javaReady"] = r.Java
		out["javaDetail"] = r.JavaDetail
		out["debugStarting"] = starting
		out["debugEnded"] = ended
		out["debugSession"] = session
		out["debugErr"] = debugErr
		if debugErr != "" {
			lower := strings.ToLower(debugErr)
			if strings.Contains(lower, "stopped importing") ||
				strings.Contains(lower, "before it was ready") ||
				strings.Contains(lower, "busy") {
				return out, fmt.Errorf("Debug treated a normal Java import as a failure: %s", debugErr)
			}
			break
		}
		if r.Java || !starting || session != 0 || ended {
			break
		}
		if time.Now().After(debugDeadline) {
			return out, errors.New("Debug stayed in the import wait state for more than 6 minutes")
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	m.post(func() {
		m.StopDebug()
	})
	if !m.waitDebug(ctx, func(d *DebugPane) bool {
		return !d.starting && d.session == 0 && d.err == ""
	}, 15*time.Second) {
		return out, errors.New("Stop did not stop the Debug session")
	}
	m.post(func() {
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
