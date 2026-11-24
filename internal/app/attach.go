package app

// Attach to Process and remote debug configurations port
// src/features/debugger/attach.ts, java.ts's javaAttachConfig and remote.ts.
// The backend lists the processes and starts adapters. This file builds the
// same adapter payloads and keeps each launch in its own debug content tab.

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

var attachKindOrder = []string{"java", "node", "python", "go", "native"}

var attachKindLabels = map[string]string{
	"java":   "Java (JDWP)",
	"node":   "Node.js",
	"python": "Python (debugpy)",
	"go":     "Go (Delve)",
	"native": "Native",
}

// attachDlg is the Attach to Process popup while it is open.
type attachDlg struct {
	open  bool
	query string
	sel   int
	list  []openidea.AttachableProcess
	err   string
	busy  bool
}

type attachGroup struct {
	kind  string
	label string
	rows  []openidea.AttachableProcess
}

// OpenAttach opens Attach to Process and loads the process list.
func (m *Model) OpenAttach() {
	m.attach = &attachDlg{open: true}
	m.invalidate()
	m.refreshAttachList()
}

func (m *Model) closeAttach() {
	m.attach = nil
	m.invalidate()
}

func attachKindRank(kind string) int {
	for i, candidate := range attachKindOrder {
		if candidate == kind {
			return i
		}
	}
	return len(attachKindOrder)
}

func attachKindLabel(kind string) string {
	if label := attachKindLabels[kind]; label != "" {
		return label
	}
	return kind
}

// refreshAttachList asks the backend for every process a pinned OpenIDEA
// debugger can attach to. Adapter availability stays a backend decision; a
// missing adapter returns the backend's real error instead of disappearing.
func (m *Model) refreshAttachList() {
	if m.attach == nil {
		return
	}
	m.attach.busy = true
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "debug_attachable_processes", nil)
		var list []openidea.AttachableProcess
		if err == nil {
			err = json.Unmarshal(raw, &list)
		}
		sort.SliceStable(list, func(i, j int) bool {
			left, right := list[i], list[j]
			li, ri := attachKindRank(left.Kind), attachKindRank(right.Kind)
			if li != ri {
				return li < ri
			}
			if (left.Port != nil) != (right.Port != nil) {
				return left.Port != nil
			}
			if left.Name != right.Name {
				return left.Name < right.Name
			}
			return left.PID < right.PID
		})
		m.post(func() {
			if m.attach == nil {
				return
			}
			m.attach.busy = false
			if err != nil {
				m.attach.err = err.Error()
			} else {
				m.attach.err = ""
				m.attach.list = list
				if m.attach.sel >= len(m.attach.filterRows()) {
					m.attach.sel = 0
				}
			}
			m.invalidate()
		})
	}()
}

// filterRows applies attach.ts's case-insensitive, every-word filter.
func (d *attachDlg) filterRows() []openidea.AttachableProcess {
	words := strings.Fields(strings.ToLower(d.query))
	if len(words) == 0 {
		return d.list
	}
	var out []openidea.AttachableProcess
	for _, p := range d.list {
		hay := strings.ToLower(fmt.Sprintf("%d %s %s", p.PID, p.Name, p.CommandLine))
		ok := true
		for _, word := range words {
			if !strings.Contains(hay, word) {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, p)
		}
	}
	return out
}

func attachGroupsOf(rows []openidea.AttachableProcess) []attachGroup {
	var groups []attachGroup
	for _, kind := range attachKindOrder {
		group := attachGroup{kind: kind, label: attachKindLabel(kind)}
		for _, p := range rows {
			if p.Kind == kind {
				group.rows = append(group.rows, p)
			}
		}
		if len(group.rows) != 0 {
			groups = append(groups, group)
		}
	}
	return groups
}

func attachDetail(p openidea.AttachableProcess) string {
	if p.Port != nil {
		host := "localhost"
		if p.Host != nil && *p.Host != "" {
			host = *p.Host
		}
		return fmt.Sprintf("%s:%d", host, *p.Port)
	}
	return fmt.Sprintf("pid %d", p.PID)
}

type debugStartPlan struct {
	name     string
	config   string
	lastID   string
	lastKind string
	lastRaw  json.RawMessage
	status   string
	prepare  func(context.Context) (openidea.DebugLaunchConfig, error)
}

// startDebugPlan owns one pending content tab. Existing live tabs stay in the
// session map while preparation and debug_start are in flight.
func (m *Model) startDebugPlan(root string, plan debugStartPlan) {
	if root == "" || plan.prepare == nil {
		return
	}
	if m.debug.starting {
		return
	}
	m.debug.rememberDebugSession()
	m.debug.active = 0
	m.debug.retiredSession = 0
	m.debug.resetActiveDebugFields()
	m.debug.lastID = plan.lastID
	m.debug.lastRaw = append(json.RawMessage(nil), plan.lastRaw...)
	m.debug.lastKind = plan.lastKind
	m.debug.config = plan.config
	m.debug.tabName = plan.name
	m.debug.view = debugViewConsole
	m.debug.console = debugConsoleView{}
	m.debug.memory = debugMemoryView{}
	m.debug.starting = true
	m.debug.output = []outputLine{{kind: "system", text: plan.status}}
	m.debug.status = plan.status
	m.invalidate()

	ctx, cancel := context.WithTimeout(m.ctx, 10*time.Minute)
	m.debug.startCancel = cancel
	go func() {
		defer cancel()
		fail := func(message string, err error) {
			if err != nil {
				message += ": " + err.Error()
			}
			m.post(func() {
				if ctx.Err() == context.Canceled || m.Root() != root || !m.debug.starting {
					return
				}
				m.debug.starting = false
				m.debug.startCancel = nil
				m.debug.err = message
				m.debug.status = ""
				m.invalidate()
			})
		}
		if err := m.pushBreakpoints(ctx, ""); err != nil {
			fail("Sending breakpoints failed", err)
			return
		}
		launch, err := plan.prepare(ctx)
		if err != nil {
			fail("Preparing the debugger failed", err)
			return
		}
		rawSession, err := m.invoke(ctx, "debug_start", map[string]any{"config": launch})
		if err != nil {
			fail("Starting the debugger failed", err)
			return
		}
		var session uint32
		if err := json.Unmarshal(rawSession, &session); err != nil {
			fail("Reading the debug session failed", err)
			return
		}
		m.post(func() {
			if ctx.Err() != nil || m.Root() != root || !m.debug.starting {
				if !m.debug.retiredSessions[session] {
					go func(id uint32) {
						stopCtx, stopCancel := context.WithTimeout(m.ctx, 15*time.Second)
						defer stopCancel()
						_, _ = m.invoke(stopCtx, "debug_stop", map[string]any{"session": id})
					}(session)
				}
				return
			}
			if m.debug.retiredSessions[session] {
				m.debug.starting = false
				m.debug.startCancel = nil
				m.debug.rememberDebugSession()
				m.invalidate()
				return
			}
			if state := m.debug.sessions[session]; state != nil {
				if m.debug.active != session {
					m.debug.restoreDebugSession(*state)
					m.debug.active = session
				}
			} else {
				m.debug.adoptDebugSession(session)
				m.debug.session = session
			}
			m.debug.starting = false
			m.debug.startCancel = nil
			m.debug.retiredSession = 0
			m.debug.session = session
			m.debug.status = plan.status
			m.recordRecentRunConfig(plan.lastID)
			m.debug.rememberDebugSession()
			m.pushSpecialBreakpoints()
			m.refreshDebugCapabilities(session)
			m.refreshDecorations()
			m.invalidate()
		})
	}()
}

func newDebugLaunch(
	name string,
	adapter string,
	request string,
	cwd string,
	extra map[string]any,
) (openidea.DebugLaunchConfig, error) {
	if adapter == "" {
		return openidea.DebugLaunchConfig{}, fmt.Errorf("the backend did not name a debug adapter")
	}
	rawExtra, err := json.Marshal(extra)
	if err != nil {
		return openidea.DebugLaunchConfig{}, err
	}
	text := string(rawExtra)
	return openidea.DebugLaunchConfig{
		Name: name, Adapter: adapter, Request: request,
		Program: nil, Args: []string{}, Cwd: cwd,
		Env: map[string]string{}, ExtraJSON: &text,
	}, nil
}

func attachRoot(module string, root string) string {
	if module == "" {
		return filepath.Clean(root)
	}
	if filepath.IsAbs(module) {
		return filepath.Clean(module)
	}
	return filepath.Clean(filepath.Join(root, module))
}

// javaAttachLaunch ports java.ts's javaAttachConfig for a JDWP target.
func (m *Model) javaAttachLaunch(
	ctx context.Context,
	name string,
	module string,
	host string,
	port int,
	root string,
	adapter string,
) (openidea.DebugLaunchConfig, error) {
	if port <= 0 || port > 65535 {
		return openidea.DebugLaunchConfig{}, fmt.Errorf("JDWP port %d is invalid", port)
	}
	if host == "" {
		host = "localhost"
	}
	cwd := attachRoot(module, root)
	if adapter == "kotlin-debug-adapter" {
		return newDebugLaunch(name, adapter, "attach", cwd, map[string]any{
			"projectRoot": cwd, "hostName": host, "port": port, "timeout": 30000,
		})
	}
	rawPort, err := m.invoke(ctx, "java_debug_server", map[string]any{"module": module})
	if err != nil {
		return openidea.DebugLaunchConfig{}, err
	}
	var debugServer uint16
	if err := json.Unmarshal(rawPort, &debugServer); err != nil {
		return openidea.DebugLaunchConfig{}, fmt.Errorf("could not read the java-debug port: %w", err)
	}
	return newDebugLaunch(name, "java-debug", "attach", cwd, map[string]any{
		"debugServer": int(debugServer),
		"hostName":    host,
		"port":        port,
		"timeout":     30000,
		"projectName": javaProjectName(cwd),
	})
}

func attachAdapter(p openidea.AttachableProcess) string {
	if p.Adapter != "" {
		return p.Adapter
	}
	switch p.Kind {
	case "java":
		return "java-debug"
	case "node":
		return "js-debug"
	case "python":
		return "debugpy"
	case "go":
		return "delve"
	}
	return ""
}

func (m *Model) attachSkipFiles() []string {
	if !m.prefs.Debugger.SkipLibraryCode() {
		return []string{}
	}
	return []string{"<node_internals>/**", "**/node_modules/**"}
}

// addPythonDebuggerOptions consumes OpenIDEA's Python Debugger page in the
// debugpy launch payload. debugpy uses subProcess for child processes and
// gevent for monkey-patched cooperative schedulers.
func (m *Model) addPythonDebuggerOptions(extra map[string]any) {
	extra["subProcess"] = m.prefs.Debugger.PythonAttachSubprocess()
	if m.prefs.Debugger.PythonGevent {
		extra["gevent"] = true
	}
}

// attachLaunchConfig ports attach.ts's attachConfig payloads for every kind.
func (m *Model) attachLaunchConfig(
	ctx context.Context,
	p openidea.AttachableProcess,
	root string,
) (openidea.DebugLaunchConfig, error) {
	name := fmt.Sprintf("%s (%d)", p.Name, p.PID)
	adapter := attachAdapter(p)
	host := "localhost"
	if p.Host != nil && *p.Host != "" {
		host = *p.Host
	}
	switch p.Kind {
	case "java":
		if p.Port == nil {
			return openidea.DebugLaunchConfig{}, fmt.Errorf("process %d has no JDWP debug port", p.PID)
		}
		return m.javaAttachLaunch(ctx, name, "", host, *p.Port, root, adapter)
	case "node":
		extra := map[string]any{
			"type": "pwa-node", "skipFiles": m.attachSkipFiles(), "continueOnAttach": true,
		}
		if p.Port != nil {
			extra["address"], extra["port"] = host, *p.Port
		} else {
			extra["processId"] = strconv.Itoa(p.PID)
		}
		return newDebugLaunch(name, adapter, "attach", root, extra)
	case "python":
		extra := map[string]any{"justMyCode": m.prefs.Debugger.SkipLibraryCode()}
		m.addPythonDebuggerOptions(extra)
		if p.Port != nil {
			extra["debugServer"] = *p.Port
		} else {
			extra["processId"] = p.PID
		}
		return newDebugLaunch(name, adapter, "attach", root, extra)
	case "go":
		return newDebugLaunch(name, adapter, "attach", root, map[string]any{
			"mode": "local", "processId": p.PID,
		})
	case "native":
		return newDebugLaunch(name, adapter, "attach", root, map[string]any{"pid": p.PID})
	default:
		return openidea.DebugLaunchConfig{}, fmt.Errorf("the backend returned an unknown attach kind %q", p.Kind)
	}
}

// AttachToProcess attaches through the adapter named by the pinned backend.
func (m *Model) AttachToProcess(p openidea.AttachableProcess) {
	root := m.Root()
	if root == "" {
		m.debug.err = "Open a project first: the sources and the debuggee share a workspace"
		m.invalidate()
		return
	}
	if !m.run.trusted {
		m.debug.err = "The project is not trusted: allow building and debugging first"
		m.invalidate()
		return
	}
	if m.busy || m.debug.starting {
		m.debug.err = "Another operation is starting; wait for it to finish or stop it"
		m.invalidate()
		return
	}
	if p.Adapter == "" && p.Kind == "native" {
		m.debug.err = "The backend did not provide a native debug adapter"
		m.invalidate()
		return
	}
	if m.showRunOnStart() {
		m.DockShow("debug")
	}
	m.attach = nil
	m.startDebugPlan(root, debugStartPlan{
		name:     p.Name,
		config:   fmt.Sprintf("%s (%d)", p.Name, p.PID),
		lastKind: p.Kind,
		status:   "Connecting to " + p.Name + "…",
		prepare: func(ctx context.Context) (openidea.DebugLaunchConfig, error) {
			return m.attachLaunchConfig(ctx, p, root)
		},
	})
}

func debugParamString(params map[string]any, key string, fallback string) string {
	if params == nil {
		return fallback
	}
	switch value := params[key].(type) {
	case string:
		if text := strings.TrimSpace(value); text != "" {
			return text
		}
	case fmt.Stringer:
		if text := strings.TrimSpace(value.String()); text != "" {
			return text
		}
	case float64:
		if value > 0 {
			return strconv.FormatInt(int64(value), 10)
		}
	case int:
		if value > 0 {
			return strconv.Itoa(value)
		}
	}
	return fallback
}

func debugParamPort(params map[string]any, fallback int) int {
	raw := debugParamString(params, "port", "")
	if raw == "" {
		return fallback
	}
	port, err := strconv.Atoi(raw)
	if err != nil || port < 1 || port > 65535 {
		return fallback
	}
	return port
}

func debugServerAddress(host string, port int) string {
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		host = "[" + host + "]"
	}
	return fmt.Sprintf("%s:%d", host, port)
}

type debugRootMapping struct {
	local  string
	remote string
}

func remoteRootMapping(params map[string]any, root string) *debugRootMapping {
	remote := debugParamString(params, "remoteRoot", "")
	if remote == "" {
		return nil
	}
	local := root
	if relative := debugParamString(params, "localRoot", ""); relative != "" {
		local = attachRoot(relative, root)
	}
	return &debugRootMapping{local: local, remote: remote}
}

func remoteName(config *openidea.RunConfig) string {
	if config.Name != "" {
		return config.Name
	}
	return config.Kind
}

func (m *Model) remoteAttachLaunch(
	config *openidea.RunConfig,
	root string,
) (openidea.DebugLaunchConfig, error) {
	name := remoteName(config)
	params := config.Params
	host := debugParamString(params, "host", "localhost")
	mapping := remoteRootMapping(params, root)
	switch config.Kind {
	case "go-remote":
		extra := map[string]any{
			"debugServer": debugServerAddress(host, debugParamPort(params, 2345)),
			"mode":        "remote",
		}
		if mapping != nil {
			extra["substitutePath"] = []map[string]string{{"from": mapping.local, "to": mapping.remote}}
		}
		return newDebugLaunch(name, "delve", "attach", root, extra)
	case "python-remote":
		extra := map[string]any{
			"debugServer": debugServerAddress(host, debugParamPort(params, 5678)),
			"justMyCode":  m.prefs.Debugger.SkipLibraryCode(),
		}
		m.addPythonDebuggerOptions(extra)
		if mapping != nil {
			extra["pathMappings"] = []map[string]string{{
				"localRoot": mapping.local, "remoteRoot": mapping.remote,
			}}
		}
		return newDebugLaunch(name, "debugpy", "attach", root, extra)
	case "node-remote":
		extra := map[string]any{
			"type": "pwa-node", "address": host,
			"port": debugParamPort(params, 9229), "skipFiles": m.attachSkipFiles(),
		}
		if mapping != nil {
			extra["localRoot"], extra["remoteRoot"] = mapping.local, mapping.remote
		}
		return newDebugLaunch(name, "js-debug", "attach", root, extra)
	case "php-remote":
		extra := map[string]any{"port": debugParamPort(params, 9003)}
		if mapping != nil {
			extra["pathMappings"] = map[string]string{mapping.remote: mapping.local}
		}
		return newDebugLaunch(name, "php-debug", "launch", root, extra)
	case "godot-attach":
		return newDebugLaunch(name, "godot", "attach", root, map[string]any{
			"debugServer": debugServerAddress(host, debugParamPort(params, 6006)),
			"project":     root,
		})
	default:
		return openidea.DebugLaunchConfig{}, fmt.Errorf("unsupported remote debug kind %q", config.Kind)
	}
}

func (m *Model) remoteJvmLaunch(
	ctx context.Context,
	config *openidea.RunConfig,
	root string,
) (openidea.DebugLaunchConfig, error) {
	params := config.Params
	host := debugParamString(params, "host", "localhost")
	port := debugParamPort(params, 5005)
	module := debugParamString(params, "module", "")
	if debugParamString(params, "mode", "attach") == "listen" {
		request := m.nextRequest()
		done := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				cancelCtx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
				defer cancel()
				_, _ = m.invoke(cancelCtx, "cancel_request", map[string]any{"id": request})
			case <-done:
			}
		}()
		m.post(func() {
			if m.debug.starting {
				m.debug.status = fmt.Sprintf("Listening for the remote JVM on %s:%d", host, port)
				m.debug.output = []outputLine{{
					kind: "system",
					text: fmt.Sprintf("Listening for the remote JVM on %s:%d…", host, port),
				}}
				m.invalidate()
			}
		})
		raw, err := m.invoke(ctx, "debug_listen_for_jvm", map[string]any{
			"request": request, "host": host, "port": port,
		})
		close(done)
		if err != nil {
			return openidea.DebugLaunchConfig{}, err
		}
		var relay uint16
		if err := json.Unmarshal(raw, &relay); err != nil {
			return openidea.DebugLaunchConfig{}, fmt.Errorf("could not read the JVM relay port: %w", err)
		}
		host, port = "127.0.0.1", int(relay)
	}
	return m.javaAttachLaunch(ctx, remoteName(config), module, host, port, root, "java-debug")
}

func isRemoteDebugKind(kind string) bool {
	switch kind {
	case "remote-jvm", "go-remote", "python-remote", "node-remote", "godot-attach", "php-remote":
		return true
	}
	return false
}

// startRemoteDebug ports the prepare-and-start behavior of remote.ts without
// launching a process: the target is already running on the other side.
func (m *Model) startRemoteDebug(config *openidea.RunConfig, root string, show bool) {
	if config == nil || !isRemoteDebugKind(config.Kind) {
		return
	}
	if !m.canStartDebugWithWindow(root, show) {
		return
	}
	kind := config.Kind
	if kind == "remote-jvm" &&
		debugParamString(config.Params, "mode", "attach") == "listen" {
		kind = "remote-jvm-listen"
	}
	m.startDebugPlan(root, debugStartPlan{
		name:     remoteName(config),
		config:   remoteName(config),
		lastID:   config.ID,
		lastKind: kind,
		lastRaw:  append(json.RawMessage(nil), config.Raw()...),
		status:   "Starting debugger for " + remoteName(config) + "…",
		prepare: func(ctx context.Context) (openidea.DebugLaunchConfig, error) {
			if config.Kind == "remote-jvm" {
				return m.remoteJvmLaunch(ctx, config, root)
			}
			return m.remoteAttachLaunch(config, root)
		},
	})
}

// attachDialog draws Attach to Process with the pinned adapter groups.
func (m *Model) attachDialog(c *ui.Context, p *ui.Theme) {
	d := m.attach
	if d == nil {
		return
	}
	open := d.open
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(620).Gap(8).Children(func() {
			ui.Text(c, "Attach to Process").FontSize(13).Bold().TextColor(p.Text).SingleLine()
			in := ui.TextInput(c, &d.query).AutoFocus()
			in.OnShortcut(0, ui.KeyDown, func() { m.stepAttach(1) })
			in.OnShortcut(0, ui.KeyUp, func() { m.stepAttach(-1) })
			rows := d.filterRows()
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				switch {
				case d.busy:
					ui.Text(c, "Reading the processes…").FontSize(12).TextColor(p.TextMuted).SingleLine()
				case d.err != "":
					ui.Text(c, d.err).FontSize(12).TextColor(p.Danger).SingleLine().Ellipsis("…")
				case len(rows) == 0:
					ui.Text(c, "No attachable processes matched this filter.").
						FontSize(12).TextColor(p.TextMuted)
				default:
					ui.Text(c, fmt.Sprintf("%d processes, Enter attaches", len(rows))).
						FontSize(12).TextColor(p.TextMuted).SingleLine()
				}
			})
			if len(rows) > 0 {
				ui.Scroll(c).Height(310).Children(func() {
					ui.Column(c).Children(func() {
						index := 0
						for _, group := range attachGroupsOf(rows) {
							ui.Row(c.Key("attach-group:"+group.kind)).Height(26).Padding(0, 8).
								AlignItems(ui.Center).Children(func() {
								ui.Text(c, group.label).FontSize(12).Bold().TextColor(p.Text)
								ui.Text(c, fmt.Sprintf("· %d", len(group.rows))).
									FontSize(12).TextColor(p.TextMuted)
							})
							for _, proc := range group.rows {
								proc, itemIndex := proc, index
								index++
								r := ui.Row(c.Key(fmt.Sprintf("attach:%s:%d", proc.Kind, proc.PID))).
									Height(26).Padding(18, 8).Gap(8).AlignItems(ui.Center)
								if itemIndex == d.sel {
									r.Background(p.SurfaceHover)
								} else if r.Hovered() {
									r.Background(p.SurfaceHover)
								}
								r.OnClick(func() {
									d.sel = itemIndex
									m.AttachToProcess(proc)
								})
								r.Children(func() {
									ui.Text(c, proc.Name).FontSize(12).TextColor(p.Text).
										SingleLine().Ellipsis("…")
									ui.Spacer(c)
									ui.Text(c, proc.Adapter).Width(100).FontSize(12).
										TextColor(p.TextMuted).SingleLine()
									ui.Text(c, attachDetail(proc)).Width(150).FontSize(12).
										TextColor(p.TextMuted).SingleLine()
								})
							}
						}
					})
				})
			}
			ui.Row(c).Gap(8).Children(func() {
				attach := ui.Button(c, "Attach")
				attach.OnClick(func() { m.attachSelected(rows) })
				ui.Button(c, "Refresh").OnClick(m.refreshAttachList)
				ui.Button(c, "Cancel").OnClick(m.closeAttach)
			})
		})
	})
	if !open {
		m.closeAttach()
	}
}

func (m *Model) stepAttach(delta int) {
	d := m.attach
	if d == nil {
		return
	}
	n := len(d.filterRows())
	if n == 0 {
		return
	}
	d.sel = ((d.sel+delta)%n + n) % n
	m.invalidate()
}

func (m *Model) attachSelected(rows []openidea.AttachableProcess) {
	d := m.attach
	if d == nil || d.sel < 0 || d.sel >= len(rows) {
		return
	}
	m.AttachToProcess(rows[d.sel])
}
