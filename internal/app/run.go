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

	busy     bool
	running  bool
	awaiting bool
	runID    uint32
	channel  uint32
	pid      *int32
	exitCode *int32
	err      string

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
		raw, err := m.mgr.Invoke(ctx, Window, "run_configs", nil)
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

// pickRunConfig prefers a non-test configuration, so Run starts the build
// rather than the test suite.
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
	if p.sel < 0 || p.sel >= len(p.configs) {
		return nil
	}
	return &p.configs[p.sel]
}

// ShowRun toggles the Run tool window.
func (m *Model) ShowRun() {
	if m.run.visible {
		m.hideTool("run")
		m.invalidate()
		return
	}
	m.run.visible = true
	m.occupyBottom("run")
	if m.run.trusted && !m.run.loaded {
		m.loadRunConfigs()
	}
	m.invalidate()
}

// Run starts the selected configuration, after saving what the build must see.
func (m *Model) Run() {
	m.run.visible = true
	m.occupyBottom("run")
	if m.busy {
		m.invalidate()
		return
	}
	if !m.run.trusted {
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
	cfg := m.run.current()
	if cfg == nil {
		m.loadRunConfigs()
		return
	}
	if m.run.busy || m.run.running {
		return
	}
	tabs := append([]*Tab(nil), m.tabs...)
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
	m.status = "正在启动 " + cfg.Name + "…"
	m.invalidate()

	args := map[string]any{
		"config":   json.RawMessage(cfg.Raw()),
		"onOutput": map[string]any{"$channel": ch},
		"request":  nil,
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
		defer cancel()
		// The backend builds from disk, so unsaved text must be written first.
		if err := saveAll(ctx, tabs); err != nil {
			m.post(func() {
				m.run.busy = false
				m.run.awaiting = false
				m.run.err = "保存后再运行失败：" + err.Error()
				m.invalidate()
			})
			return
		}
		raw, err := m.mgr.Invoke(ctx, Window, "run_start", args)
		var info openidea.RunInfo
		if err == nil {
			err = json.Unmarshal(raw, &info)
		}
		m.post(func() {
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
		if _, err := m.mgr.Invoke(ctx, Window, "run_stop", map[string]any{"id": id}); err != nil {
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
	case id != 0 && id == m.term.channel:
		m.acceptTermChunk(data)
	case id != 0 && id == m.search.channel:
		m.acceptSearchEvent(data)
	case id != 0 && id == m.usages.channel:
		m.acceptUsages(id, data)
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
	args := map[string]any{
		"config":   json.RawMessage(cfg.Raw()),
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
