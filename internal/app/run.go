package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/adapter"
	"mayoi/internal/openidea"
)

// RunPane is the Run tool window: the project's configurations, the output of
// the process run_start started, and whether the project is trusted to build
// and run at all.
type RunPane struct {
	visible bool
	trusted bool
	// trustErr is why trust could not be read or granted.
	trustErr string

	configs []openidea.RunConfig
	sel     int
	loaded  bool
	loading bool
	// pendingStart runs the selected configuration once run_configs lands.
	pendingStart bool
	// pendingTrust is a Run the user asked for before trusting the project;
	// it starts once the grant lands.
	pendingTrust bool
	// inline is a configuration that is not in the detected list, such as the
	// command line Run Anything starts; current() reports it while it is set.
	inline *openidea.RunConfig

	busy     bool
	running  bool
	awaiting bool
	runID    uint32
	channel  uint32
	pid      *int32
	exitCode *int32
	err      string

	// lastID/lastRaw/lastName remember the configuration this pane started,
	// so Rerun launches it as saved now (upstream rerunConfig).
	lastID   string
	lastRaw  json.RawMessage
	lastName string

	output    []outputLine
	truncated bool
}

type outputLine struct {
	kind string
	text string
}

// maxRunLines bounds what the panel keeps; Maven can print more than anyone
// reads, and an unbounded log would grow with the process's lifetime.
const maxRunLines = 3000

// resetRun drops the previous project's run state.
func (m *Model) resetRun() {
	m.run = RunPane{}
}

// nextChannel allocates a streaming channel id for run output.
func (m *Model) nextChannel() uint32 { return m.chanSeq.Add(1) }

// checkTrust reads the project's execution trust, then loads its run
// configurations when the user has allowed running this folder.
func (m *Model) checkTrust(dir string) {
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.mgr.Invoke(ctx, Window, "project_trust_status", map[string]any{"path": dir})
		var status openidea.TrustStatus
		if err == nil {
			err = json.Unmarshal(raw, &status)
		}
		m.post(func() {
			if m.Root() != dir {
				return
			}
			m.run.trustErr = ""
			m.run.trusted = err == nil && status.Trusted
			if err != nil {
				m.run.trustErr = err.Error()
			}
			if m.run.trusted {
				m.loadRunConfigs()
			} else {
				m.status += " · 未信任（运行前需授权）"
				m.trustAsk = true
			}
			m.invalidate()
		})
	}()
}

// TrustProject grants this folder permission to build and run. Project
// settings never grant it; only this explicit action does.
func (m *Model) TrustProject() {
	root := m.Root()
	if root == "" || m.run.busy {
		return
	}
	m.run.busy = true
	m.run.trustErr = ""
	wasTrusted := m.run.trusted
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		_, err := m.mgr.Invoke(ctx, Window, "project_trust_set", map[string]any{
			"path": root, "trusted": true, "folder": true,
		})
		var status openidea.TrustStatus
		if err == nil {
			var raw []byte
			raw, err = m.mgr.Invoke(ctx, Window, "project_trust_status", map[string]any{"path": root})
			if err == nil {
				err = json.Unmarshal(raw, &status)
			}
		}
		// Documents opened while the project was untrusted carry no language
		// server: attaching them happened before the grant existed. Re-opening
		// them attaches the servers the grant started.
		var reopenErr error
		if err == nil && status.Trusted && !wasTrusted {
			reopenErr = m.reg.ReopenAll(ctx)
		}
		m.post(func() {
			m.run.busy = false
			if m.Root() != root {
				return
			}
			if err != nil {
				m.run.trustErr = err.Error()
				m.invalidate()
				return
			}
			m.run.trusted = status.Trusted
			if m.run.trusted {
				m.status = "已信任此项目，可以构建和运行"
				if m.run.pendingTrust {
					m.run.pendingTrust = false
					m.run.pendingStart = true
				}
				m.loadRunConfigs()
			}
			if reopenErr != nil {
				m.run.trustErr = "语言服务重连失败：" + reopenErr.Error()
			}
			m.invalidate()
		})
	}()
}

// loadRunConfigs asks the backend for the project's configurations. Detection
// needs a trusted project, so this only runs after trust is granted.
func (m *Model) loadRunConfigs() {
	root := m.Root()
	if root == "" || m.run.loading {
		return
	}
	m.run.loading = true
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "run_configs", nil)
		var configs []openidea.RunConfig
		if err == nil {
			configs, err = openidea.DecodeRunConfigs(raw)
		}
		m.post(func() {
			m.run.loading = false
			if m.Root() != root {
				return
			}
			if err != nil {
				m.run.err = "读取运行配置失败：" + err.Error()
				m.run.visible = true
				m.occupyBottom("run")
				m.invalidate()
				return
			}
			m.run.configs = configs
			m.run.inline = nil
			m.run.loaded = true
			m.run.sel = pickRunConfig(configs)
			m.invalidate()
			if m.run.pendingStart {
				m.run.pendingStart = false
				m.startRun()
			}
		})
	}()
}

// ReloadRunConfigs drops what detection found and asks the backend again.
func (m *Model) ReloadRunConfigs() {
	if m.Root() == "" || m.run.loading {
		return
	}
	m.run.loaded = false
	m.run.inline = nil
	m.loadRunConfigs()
	m.invalidate()
}

// pickRunConfig prefers the program over the build tool: a detected Java
// application, then any non-test configuration. `mvn package` is a build, so
// it should not be what the Run button starts when an application is known.
func pickRunConfig(configs []openidea.RunConfig) int {
	for i, c := range configs {
		if c.IsTest {
			continue
		}
		if c.DebugAdapter != nil && *c.DebugAdapter != "" {
			return i
		}
		switch c.Kind {
		case "java", "application", "spring-boot":
			return i
		}
	}
	for i, c := range configs {
		if !c.IsTest {
			return i
		}
	}
	return 0
}

func (p *RunPane) current() *openidea.RunConfig {
	if p.inline != nil {
		return p.inline
	}
	if p.sel < 0 || p.sel >= len(p.configs) {
		return nil
	}
	return &p.configs[p.sel]
}

// ShowRun toggles the Run tool window.
func (m *Model) ShowRun() { m.dockActivate("run") }

// Run starts the selected configuration, after saving what the build must see.
func (m *Model) Run() {
	m.DockShow("run")
	if m.busy {
		m.invalidate()
		return
	}
	if !m.run.trusted {
		m.run.pendingTrust = true
		m.trustAsk = true
		m.status = "项目未信任：请在弹窗中信任后再运行"
		m.invalidate()
		return
	}
	if !m.run.loaded {
		m.run.pendingStart = true
		m.loadRunConfigs()
		m.invalidate()
		return
	}
	m.startRun()
}

func (m *Model) startRun() {
	m.run.inline = nil
	cfg := m.run.current()
	if cfg == nil {
		m.loadRunConfigs()
		return
	}
	m.startRunWith(json.RawMessage(cfg.Raw()), cfg.Name)
}

// startRunWith starts an arbitrary configuration, which is how a typed
// command line (Run Anything) runs beside the detected configurations.
func (m *Model) startRunWith(raw json.RawMessage, name string) bool {
	if !m.run.trusted {
		m.invalidate()
		return false
	}
	if m.run.busy || m.run.running {
		return false
	}
	// Rerun launches this configuration as saved now (upstream rerunConfig).
	var probe struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(raw, &probe)
	m.run.lastID = probe.ID
	m.run.lastRaw = append(json.RawMessage(nil), raw...)
	m.run.lastName = name
	tabs := append([]*Tab(nil), m.tabs...)
	root := m.Root()
	ch := m.nextChannel()
	m.run.visible = true
	m.occupyBottom("run")
	m.run.busy = true
	m.run.awaiting = true
	m.run.running = false
	m.run.runID = 0
	m.run.channel = ch
	m.run.pid = nil
	m.run.exitCode = nil
	m.run.err = ""
	m.run.output = nil
	m.run.truncated = false
	m.status = "正在启动 " + name + "…"
	m.invalidate()

	args := map[string]any{
		"config":   raw,
		"onOutput": map[string]any{"$channel": ch},
		"request":  nil,
	}
	p := m.prefs.SaveFiles
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 5*time.Minute)
		defer cancel()
		// The backend builds from disk, so unsaved text must be written first.
		if err := saveAll(ctx, tabs, p); err != nil {
			m.post(func() {
				m.run.busy = false
				m.run.awaiting = false
				m.run.err = "保存后再运行失败：" + err.Error()
				m.invalidate()
			})
			return
		}
		resolved, err := m.resolveJVMConfig(ctx, raw, root, m.reportRunConsole(root, ch))
		if err != nil {
			m.post(func() {
				m.run.busy = false
				m.run.awaiting = false
				m.run.err = "启动失败：" + err.Error()
				m.invalidate()
			})
			return
		}
		args["config"] = resolved
		rawInfo, err := m.invoke(ctx, "run_start", args)
		var info openidea.RunInfo
		if err == nil {
			err = json.Unmarshal(rawInfo, &info)
		}
		m.post(func() {
			if m.Root() != root || m.run.channel != ch {
				// The project changed while run_start was in flight: the
				// process belongs to the old one, so stop it and keep the
				// new project's run state untouched.
				if err == nil && info.ID != 0 {
					go func(id uint32) {
						ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
						defer cancel()
						_, _ = m.mgr.Invoke(ctx, Window, "run_stop", map[string]any{"id": id})
					}(info.ID)
				}
				return
			}
			m.run.busy = false
			if err != nil {
				m.run.awaiting = false
				m.run.running = false
				m.run.err = "启动失败：" + err.Error()
				m.invalidate()
				return
			}
			m.run.runID = info.ID
			m.run.pid = info.PID
			m.run.running = info.Running
			if !info.Running {
				m.run.awaiting = false
			}
			m.status = "运行中：" + info.Name
			m.invalidate()
		})
	}()
	return true
}

// StopRun terminates the running configuration.
func (m *Model) StopRun() {
	if !m.run.running && !m.run.awaiting {
		return
	}
	id := m.run.runID
	if id == 0 {
		m.run.err = "进程尚未启动"
		m.invalidate()
		return
	}
	m.status = "正在停止…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
		defer cancel()
		if _, err := m.invoke(ctx, "run_stop", map[string]any{"id": id}); err != nil {
			m.post(func() {
				m.run.err = "停止失败：" + err.Error()
				m.invalidate()
			})
		}
	}()
}

// CycleRunConfig selects the next (delta 1) or previous (delta -1)
// configuration.
func (m *Model) CycleRunConfig(delta int) {
	n := len(m.run.configs)
	if n == 0 {
		return
	}
	m.run.sel = (m.run.sel + delta + n) % n
	m.run.inline = nil
	m.invalidate()
}

// ClearRun empties the output pane.
func (m *Model) ClearRun() {
	m.run.output = nil
	m.run.truncated = false
	m.run.exitCode = nil
	m.run.err = ""
	m.invalidate()
}

// watchBackend listens on its own copy of the notification stream: run output
// channels and runExited events, which the document registry does not use.
func (m *Model) watchBackend(ctx context.Context) {
	ch, cancel := m.mgr.Listen(8192)
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return
		case n, ok := <-ch:
			if !ok {
				return
			}
			switch n.Method {
			case "channel":
				var msg openidea.ChannelMessage
				if json.Unmarshal(n.Params, &msg) != nil {
					continue
				}
				id, data := msg.ID, msg.Data
				m.post(func() { m.acceptChunk(id, data) })
			case "event":
				var env openidea.WindowEvent
				if json.Unmarshal(n.Params, &env) != nil {
					continue
				}
				if env.Window != nil && *env.Window != Window {
					continue
				}
				switch openidea.EventType(env.Event) {
				case "runExited":
					var e openidea.RunExitedEvent
					if json.Unmarshal(env.Event, &e) != nil {
						continue
					}
					m.post(func() { m.acceptExit(e) })
				case "terminalExited":
					var e openidea.TerminalExitedEvent
					if json.Unmarshal(env.Event, &e) != nil {
						continue
					}
					term, code := e.Terminal, e.Code
					m.post(func() { m.acceptTermExit(term, code) })
				case "debug":
					var d openidea.DebugEnvelope
					if json.Unmarshal(env.Event, &d) != nil {
						continue
					}
					session, raw := d.Session, d.Event
					m.post(func() { m.acceptDebugEvent(session, raw) })
				case "workspaceEditApplied":
					var e openidea.WorkspaceEditAppliedEvent
					if json.Unmarshal(env.Event, &e) != nil {
						continue
					}
					m.post(func() { m.acceptWorkspaceEdit(e) })
				}
			}
		}
	}
}

func (m *Model) acceptChunk(id uint32, data json.RawMessage) {
	if m.acceptToolEvent(id, data) {
		return
	}
	switch {
	case id != 0 && id == m.run.channel:
		var chunk openidea.OutputChunk
		if json.Unmarshal(data, &chunk) != nil {
			return
		}
		m.run.append(chunk)
		m.invalidate()
	case id != 0 && id == m.build.channel:
		m.acceptBuildEvent(data)
	case id != 0 && m.term.byChannel(id) != nil:
		m.acceptTermChunk(id, data)
	case id != 0 && m.se != nil && id == m.se.textCh:
		m.acceptSearchEverywhereEvent(data)
	case id != 0 && id == m.search.channel:
		m.acceptSearchEvent(data)
	case id != 0 && id == m.impl.request:
		m.acceptImplementationChunk(data)
	case id != 0 && id == m.usages.channel:
		m.acceptUsages(id, data)
	case id != 0 && id == m.todo.channel:
		m.acceptTodoEvent(data)
	case id != 0 && id == m.newProject.channel:
		m.acceptScaffoldEvent(data)
	case id != 0 && id == m.todo.scopeChannel:
		m.acceptTodoScopeEvent(data)
	}
}

func (p *RunPane) append(c openidea.OutputChunk) {
	text := strings.TrimSuffix(stripANSI(c.Text), "\n")
	if text == "" {
		return
	}
	kind := c.Kind
	if kind == "" {
		kind = "stdout"
	}
	for _, line := range strings.Split(text, "\n") {
		p.output = append(p.output, outputLine{kind: kind, text: line})
	}
	if over := len(p.output) - maxRunLines; over > 0 {
		p.output = append(p.output[:0], p.output[over:]...)
		p.truncated = true
	}
}

func (m *Model) acceptExit(e openidea.RunExitedEvent) {
	if !m.run.awaiting {
		return
	}
	if m.run.runID != 0 && e.Run != m.run.runID {
		return
	}
	m.run.awaiting = false
	m.run.running = false
	m.run.busy = false
	m.run.exitCode = e.Code
	switch {
	case e.Code == nil:
		m.status = "运行已结束"
	case *e.Code == 0:
		m.status = "运行成功（退出码 0）"
	default:
		m.status = fmt.Sprintf("运行失败（退出码 %d）", *e.Code)
	}
	m.invalidate()
}

// stripANSI removes CSI escape sequences (Maven colors its console output)
// so the pane shows text, not control codes.
func stripANSI(s string) string {
	if !strings.ContainsRune(s, 0x1b) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != 0x1b {
			b.WriteByte(s[i])
			continue
		}
		i++
		if i < len(s) && s[i] == '[' {
			i++
			for i < len(s) && (s[i] < 0x40 || s[i] > 0x7e) {
				i++
			}
		}
	}
	return b.String()
}

// text is the pane's output, one line per entry.
func (p *RunPane) text() string {
	var b strings.Builder
	for i, line := range p.output {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(line.text)
	}
	return b.String()
}

// SmokeRun trusts the project, runs its first build configuration and waits
// for the process to exit: the offscreen check against a real Java build.
func (m *Model) SmokeRun(ctx context.Context) (config string, code *int32, output string, err error) {
	root := m.Root()
	if root == "" {
		return "", nil, "", errors.New("没有打开项目")
	}
	if _, err = m.mgr.Invoke(ctx, Window, "project_trust_set", map[string]any{
		"path": root, "trusted": true, "folder": true,
	}); err != nil {
		return "", nil, "", err
	}
	raw, err := m.mgr.Invoke(ctx, Window, "run_configs", nil)
	if err != nil {
		return "", nil, "", err
	}
	configs, err := openidea.DecodeRunConfigs(raw)
	if err != nil {
		return "", nil, "", err
	}
	if len(configs) == 0 {
		return "", nil, "", errors.New("项目没有可运行的配置")
	}
	sel := pickRunConfig(configs)
	if want := strings.TrimSpace(os.Getenv("GOIDEA_SMOKE_RUN_CONFIG")); want != "" {
		sel = -1
		for i, c := range configs {
			if c.Name == want {
				sel = i
				break
			}
		}
		if sel < 0 {
			return "", nil, "", fmt.Errorf("没有名为 %s 的运行配置", want)
		}
	}
	cfg := configs[sel]
	channel := m.nextChannel()
	m.post(func() {
		m.run.visible = true
		m.occupyBottom("run")
		m.run.trusted = true
		m.run.loaded = true
		m.run.configs = configs
		m.run.sel = sel
		m.run.busy = true
		m.run.awaiting = true
		m.run.running = false
		m.run.runID = 0
		m.run.channel = channel
		m.run.pid = nil
		m.run.exitCode = nil
		m.run.err = ""
		m.run.output = nil
		m.run.truncated = false
		m.invalidate()
	})
	resolved, err := m.resolveJVMConfig(ctx, json.RawMessage(cfg.Raw()), root, m.reportRunConsole(root, channel))
	if err != nil {
		m.post(func() { m.run.busy = false; m.run.awaiting = false })
		return cfg.Name, nil, "", err
	}
	args := map[string]any{
		"config":   resolved,
		"onOutput": map[string]any{"$channel": channel},
		"request":  nil,
	}
	rawInfo, err := m.mgr.Invoke(ctx, Window, "run_start", args)
	if err != nil {
		m.post(func() { m.run.busy = false; m.run.awaiting = false })
		return cfg.Name, nil, "", err
	}
	var info openidea.RunInfo
	if err = json.Unmarshal(rawInfo, &info); err != nil {
		m.post(func() { m.run.busy = false; m.run.awaiting = false })
		return cfg.Name, nil, "", err
	}
	m.post(func() {
		m.run.busy = false
		m.run.runID = info.ID
		m.run.pid = info.PID
		m.run.running = info.Running
		m.invalidate()
	})

	if ms, err := strconv.Atoi(os.Getenv("GOIDEA_SMOKE_RUN_STOP_MS")); err == nil && ms > 0 {
		select {
		case <-ctx.Done():
		case <-time.After(time.Duration(ms) * time.Millisecond):
			m.StopRun()
		}
	}

	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	for {
		var exit *int32
		var txt, runErr string
		m.post(func() {
			exit = m.run.exitCode
			txt = m.run.text()
			runErr = m.run.err
		})
		if exit != nil {
			return cfg.Name, exit, txt, nil
		}
		if runErr != "" {
			return cfg.Name, nil, txt, errors.New(runErr)
		}
		select {
		case <-ctx.Done():
			return cfg.Name, nil, txt, ctx.Err()
		case <-tick.C:
		}
	}
}

// SmokeSpringRun runs the detected Spring Boot configuration of a real
// multi-module Maven project end to end: Maven model and Java import finish,
// the nested service module's configuration is discovered, java_resolve_launch
// resolves a classpath with no placeholder left, run_start starts a real JVM,
// the Spring Boot startup marker appears, the process is stopped and no
// orphan JVM is left behind.
func (m *Model) SmokeSpringRun(ctx context.Context) (map[string]any, error) {
	out := map[string]any{"root": m.Root()}
	root := m.Root()
	if root == "" {
		return out, errors.New("没有打开项目")
	}
	if _, err := m.mgr.Invoke(ctx, Window, "project_trust_set", map[string]any{
		"path": root, "trusted": true, "folder": true,
	}); err != nil {
		return out, err
	}

	// The configuration appears once the Maven model is loaded and detected;
	// a fresh project's Java import can take minutes, so poll for it.
	var cfg *openidea.RunConfig
	var configs []openidea.RunConfig
	deadline := time.Now().Add(6 * time.Minute)
	for {
		raw, err := m.mgr.Invoke(ctx, Window, "run_configs", nil)
		if err == nil {
			configs, err = openidea.DecodeRunConfigs(raw)
		}
		if err == nil {
			for i := range configs {
				if configs[i].Kind == "spring-boot" {
					cfg = &configs[i]
					break
				}
			}
		}
		if cfg != nil {
			break
		}
		if time.Now().After(deadline) {
			names := make([]string, 0, len(configs))
			for _, c := range configs {
				names = append(names, c.Kind+":"+c.Name)
			}
			return out, fmt.Errorf("6 分钟内没有发现 spring-boot 运行配置；检测到：%s",
				strings.Join(names, ", "))
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	out["configKind"] = cfg.Kind
	out["configName"] = cfg.Name
	var rawCfg map[string]any
	if err := json.Unmarshal(cfg.Raw(), &rawCfg); err != nil {
		return out, err
	}
	module := ""
	if cfg.Module != nil {
		module = *cfg.Module
	}
	out["configModule"] = module
	if module == "" {
		return out, errors.New("多模块项目必须解析出子模块，而不是项目根目录")
	}
	out["configArgs"] = rawCfg["args"]
	mainClass := adapter.ParseJavaArgs(cfg.Args).MainClass
	out["mainClass"] = mainClass

	// The run console shows the whole preparation; the resolution must leave
	// no placeholder and an executable java launcher.
	channel := m.nextChannel()
	m.post(func() {
		m.run.visible = true
		m.occupyBottom("run")
		m.run.trusted = true
		m.run.loaded = true
		m.run.configs = configs
		m.run.sel = -1
		m.run.busy = true
		m.run.awaiting = true
		m.run.running = false
		m.run.runID = 0
		m.run.channel = channel
		m.run.pid = nil
		m.run.exitCode = nil
		m.run.err = ""
		m.run.output = nil
		m.run.truncated = false
		m.invalidate()
	})
	resolved, err := m.resolveJVMConfig(ctx, json.RawMessage(cfg.Raw()), root, m.reportRunConsole(root, channel))
	if err != nil {
		m.post(func() { m.run.busy = false; m.run.awaiting = false })
		out["resolveError"] = err.Error()
		return out, err
	}
	resolvedText := string(resolved)
	if strings.Contains(resolvedText, "${classpath}") || strings.Contains(resolvedText, "${testClasspath}") {
		m.post(func() { m.run.busy = false; m.run.awaiting = false })
		return out, errors.New("解析后的配置仍带有 classpath 占位符：" + resolvedText)
	}
	var resolvedCfg map[string]any
	if err := json.Unmarshal(resolved, &resolvedCfg); err != nil {
		return out, err
	}
	resolvedArgs := toStrings(resolvedCfg["args"])
	out["resolvedCommand"] = resolvedCfg["command"]
	entries := 0
	for i, a := range resolvedArgs {
		if a == "-cp" || a == "-classpath" || a == "--class-path" {
			if i+1 < len(resolvedArgs) {
				sep := string(os.PathListSeparator)
				entries = len(strings.Split(resolvedArgs[i+1], sep))
			}
		}
	}
	out["classPathEntries"] = entries
	if entries == 0 {
		return out, errors.New("解析后的 classpath 为空")
	}

	args := map[string]any{
		"config":   resolved,
		"onOutput": map[string]any{"$channel": channel},
		"request":  nil,
	}
	rawInfo, err := m.mgr.Invoke(ctx, Window, "run_start", args)
	if err != nil {
		m.post(func() { m.run.busy = false; m.run.awaiting = false })
		return out, err
	}
	var info openidea.RunInfo
	if err := json.Unmarshal(rawInfo, &info); err != nil {
		return out, err
	}
	m.post(func() {
		m.run.busy = false
		m.run.runID = info.ID
		m.run.pid = info.PID
		m.run.running = info.Running
		m.invalidate()
	})
	if info.PID != nil {
		out["pid"] = *info.PID
	}

	// Wait for the real Spring Boot startup: the marker proves Spring came up
	// and the sibling lib module was on the resolved classpath.
	marker := strings.TrimSpace(os.Getenv("GOIDEA_SMOKE_SPRING_MARKER"))
	if marker == "" {
		marker = "GREETING:Hello, GoIDEA!"
	}
	waitDeadline := time.Now().Add(4 * time.Minute)
	started := false
	var text string
	for {
		m.post(func() { text = m.run.text() })
		if strings.Contains(text, marker) {
			started = true
			break
		}
		if time.Now().After(waitDeadline) {
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	out["started"] = started
	if !started {
		return out, fmt.Errorf("4 分钟内没有等到 Spring Boot 启动标志 %q；输出尾部：%s", marker, tailOf(text, 1200))
	}

	// The process must really be running, not just reported.
	rawList, err := m.mgr.Invoke(ctx, Window, "run_list", nil)
	if err != nil {
		return out, err
	}
	var list []openidea.RunInfo
	if err := json.Unmarshal(rawList, &list); err != nil {
		return out, err
	}
	running := false
	for _, r := range list {
		if r.ID == info.ID && r.Running {
			running = true
		}
	}
	out["processRunning"] = running
	if !running {
		return out, errors.New("Spring Boot 启动标志出现，但进程已不在运行")
	}

	m.StopRun()
	for {
		var exit *int32
		m.post(func() { exit = m.run.exitCode })
		if exit != nil {
			out["exitCode"] = *exit
			break
		}
		if time.Now().After(waitDeadline) {
			return out, errors.New("run_stop 后进程没有退出")
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	// Killing the run must kill the JVM: no orphan may outlive the stop.
	stopDeadline := time.Now().Add(15 * time.Second)
	for {
		orphans := probeOrphanJVMs(mainClass)
		if len(orphans) == 0 {
			out["orphans"] = 0
			break
		}
		if time.Now().After(stopDeadline) {
			out["orphans"] = len(orphans)
			return out, fmt.Errorf("run_stop 后仍有 %d 个孤立的 JVM 进程：%s", len(orphans), strings.Join(orphans, ", "))
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return out, nil
}

// probeOrphanJVMs lists the processes whose command line still names the main
// class; an empty list means run_stop left no orphan JVM.
func probeOrphanJVMs(mainClass string) []string {
	if mainClass == "" {
		return nil
	}
	out, err := exec.Command("pgrep", "-f", mainClass).Output()
	if err != nil {
		// pgrep exits 1 when nothing matches: no orphan.
		return nil
	}
	return strings.Fields(string(out))
}

func toStrings(v any) []string {
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// tailOf keeps the last n characters of a run console dump.
func tailOf(text string, n int) string {
	if len(text) <= n {
		return text
	}
	return text[len(text)-n:]
}

// SmokeRunConfigEdit exercises the Edit Configurations flow against the real
// backend: duplicate the first configuration into a user copy, save it,
// verify run_configs serves it back with the edit, verify the build policy
// rewrites a Maven command line at launch, and delete it again.
func (m *Model) SmokeRunConfigEdit(ctx context.Context) (map[string]any, error) {
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
	fetch := func() ([]*openidea.RunConfig, error) {
		raw, err := m.invoke(ctx, "run_configs", nil)
		if err != nil {
			return nil, err
		}
		configs, err := openidea.DecodeRunConfigs(raw)
		if err != nil {
			return nil, err
		}
		out := make([]*openidea.RunConfig, len(configs))
		for i := range configs {
			out[i] = &configs[i]
		}
		return out, nil
	}
	configs, err := fetch()
	if err != nil {
		return out, err
	}
	if len(configs) == 0 {
		return out, errors.New("项目没有可运行的配置")
	}
	// Editing the first detected configuration produces a user copy with its
	// id, a new name, and a Build Before Launch policy (upstream upsertConfig).
	source := configs[0]
	duplicate, err := adapter.DuplicateConfig(source, configs)
	if err != nil {
		return out, err
	}
	editedName := "GoIDEA-E2E-Edited"
	duplicate.Name = editedName
	policy := "rebuild"
	duplicate.BuildPolicy = &policy
	items := adapter.UpsertConfig(configs, duplicate)
	if err := adapter.SaveUserConfigs(ctx, javaBackend{m}, adapter.UserConfigs(items), nil); err != nil {
		out["saveError"] = err.Error()
		return out, err
	}
	merged, err := fetch()
	if err != nil {
		return out, err
	}
	var saved *openidea.RunConfig
	for _, c := range merged {
		if c.ID == duplicate.ID {
			saved = c
		}
	}
	if saved == nil {
		return out, fmt.Errorf("保存后 run_configs 没有返回用户配置 %s", duplicate.ID)
	}
	if saved.Detected || saved.Name != editedName {
		return out, fmt.Errorf("保存的配置未按编辑内容返回：name=%s detected=%v", saved.Name, saved.Detected)
	}
	out["saved"] = true
	out["savedName"] = saved.Name

	// The build policy rewrites a Maven command line at launch: rebuild
	// prepends clean (upstream applyBuildPolicyToCommand).
	resolved, err := m.resolveJVMConfig(ctx, saved.Raw(), root, nil)
	if err != nil {
		return out, fmt.Errorf("解析构建策略失败：%w", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(resolved, &obj); err != nil {
		return out, err
	}
	args := toStrings(obj["args"])
	out["policyArgs"] = args
	if len(args) == 0 || args[0] != "clean" {
		return out, fmt.Errorf("rebuild 策略没有在启动前重写 Maven 命令行：%v", args)
	}
	out["policyApplied"] = true

	// The user copy runs like any other configuration, then is deleted again;
	// its id does not come back.
	if code, err := m.runByName(ctx, saved); err != nil {
		return out, err
	} else {
		out["exitCode"] = *code
	}
	if err := adapter.SaveConfigs(ctx, javaBackend{m}, adapter.UserConfigs(adapter.RemoveConfig(merged, duplicate.ID)), nil); err != nil {
		return out, err
	}
	merged, err = fetch()
	if err != nil {
		return out, err
	}
	for _, c := range merged {
		if c.ID == duplicate.ID {
			return out, errors.New("删除后 run_configs 仍返回该配置")
		}
	}
	out["removed"] = true
	return out, nil
}

// runByName runs a configuration object through the real run path and waits
// for its exit, the way SmokeRun does for the selected one.
func (m *Model) runByName(ctx context.Context, cfg *openidea.RunConfig) (*int32, error) {
	channel := m.nextChannel()
	m.post(func() {
		m.run.visible = true
		m.occupyBottom("run")
		m.run.trusted = true
		m.run.loaded = true
		m.run.busy = true
		m.run.awaiting = true
		m.run.running = false
		m.run.runID = 0
		m.run.channel = channel
		m.run.pid = nil
		m.run.exitCode = nil
		m.run.err = ""
		m.run.output = nil
		m.run.truncated = false
		m.invalidate()
	})
	resolved, err := m.resolveJVMConfig(ctx, cfg.Raw(), m.Root(), m.reportRunConsole(m.Root(), channel))
	if err != nil {
		m.post(func() { m.run.busy = false; m.run.awaiting = false })
		return nil, err
	}
	args := map[string]any{
		"config":   resolved,
		"onOutput": map[string]any{"$channel": channel},
		"request":  nil,
	}
	rawInfo, err := m.mgr.Invoke(ctx, Window, "run_start", args)
	if err != nil {
		m.post(func() { m.run.busy = false; m.run.awaiting = false })
		return nil, err
	}
	var info openidea.RunInfo
	if err := json.Unmarshal(rawInfo, &info); err != nil {
		return nil, err
	}
	m.post(func() {
		m.run.busy = false
		m.run.runID = info.ID
		m.run.pid = info.PID
		m.run.running = info.Running
		m.invalidate()
	})
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	for {
		var exit *int32
		var runErr string
		m.post(func() { exit = m.run.exitCode; runErr = m.run.err })
		if exit != nil {
			return exit, nil
		}
		if runErr != "" {
			return nil, errors.New(runErr)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-tick.C:
		}
	}
}

// trustDialog asks whether a project may build and run. It opens when an
// untrusted project is opened and when Run, Build or Debug is asked for
// before the grant.
func (m *Model) trustDialog(c *ui.Context, p *ui.Theme) {
	open := m.trustAsk
	oi := oiPaletteOf(p)
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(460).Gap(10).Children(func() {
			ui.Text(c, "Trust Project").FontSize(14).Bold().TextColor(oi.text).SingleLine()
			ui.Text(c, "此项目来自外部来源。构建、运行、调试和语言服务会执行项目中的代码。").
				FontSize(12).TextColor(oi.textMuted)
			if m.run.trustErr != "" {
				ui.Text(c, m.run.trustErr).FontSize(11).TextColor(oi.danger)
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Spacer(c)
				ui.Button(c, "Cancel").OnClick(func() {
					m.trustAsk = false
					m.run.pendingTrust = false
					m.invalidate()
				})
				ui.Button(c, "Trust Project").OnClick(func() {
					m.trustAsk = false
					m.TrustProject()
					m.invalidate()
				})
			})
		})
	})
	if !open {
		m.trustAsk = false
		m.run.pendingTrust = false
		m.invalidate()
	}
}
