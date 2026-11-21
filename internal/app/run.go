package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

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
	// pendingCoverage selects the coverage executor for the next pending Run.
	pendingCoverage bool
	// pendingContext runs the active file once trust and run configurations
	// are ready. pendingContextCoverage selects the coverage executor.
	pendingContext         bool
	pendingContextCoverage bool
	pendingDebugContext    bool
	coverage               *coverageExecution
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

	// tests folds the run's test output into a results tree (testing
	// feature), and testParser feeds it; set for JVM configurations, whose
	// output Maven Surefire and Gradle report per test.
	tests        *adapter.TestResults
	testParser   *adapter.JvmParser
	testGo       *adapter.GoParser
	testCargo    *adapter.CargoParser
	testPytest   *adapter.PytestParser
	testUnittest *adapter.UnittestParser
	testJs       *adapter.JsParser
	testPHPUnit  *adapter.PHPUnitParser
	testRuby     *adapter.RubyParser
	testDotnet   *adapter.DotnetReports
	testStdout   *adapter.TestStdoutBuffer
	// selTest is the selected node of the results tree view.
	selTest string
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
				m.status += " · untrusted (allow it before running)"
				m.trustAsk = true
				m.trustFolder = false
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
	folder := m.trustFolder
	m.trustAsk = false
	m.trustFolder = false
	m.run.busy = true
	m.run.trustErr = ""
	wasTrusted := m.run.trusted
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		_, err := m.mgr.Invoke(ctx, Window, "project_trust_set", map[string]any{
			"path": root, "trusted": true, "folder": folder,
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
				m.status = "This project is trusted; it may build and run"
				if m.run.pendingTrust {
					m.run.pendingTrust = false
					if !m.run.pendingContext && !m.run.pendingDebugContext {
						m.run.pendingStart = true
					}
				}
				m.loadRunConfigs()
			}
			if reopenErr != nil {
				m.run.trustErr = "Reconnecting the language service failed: " + reopenErr.Error()
			}
			m.invalidate()
		})
	}()
}

// RevokeProjectTrust turns the current project back into Restricted Mode. Run
// configurations are forgetten until the project is trusted again.
func (m *Model) RevokeProjectTrust() {
	root := m.Root()
	if root == "" || m.ctx == nil || m.mgr == nil || m.run.busy {
		return
	}
	m.run.busy = true
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		_, err := m.mgr.Invoke(ctx, Window, "project_trust_set", map[string]any{
			"path": root, "trusted": false, "folder": false,
		})
		m.post(func() {
			m.run.busy = false
			if m.Root() != root {
				return
			}
			if err != nil {
				m.run.trustErr = "Could not revoke project trust: " + err.Error()
				m.invalidate()
				return
			}
			m.run.trusted = false
			m.run.loaded = false
			m.run.configs = nil
			m.run.sel = -1
			m.run.trustErr = ""
			m.status = "Project trust revoked"
			m.invalidate()
		})
	}()
}

// RemoveTrustedLocation removes the selected folder, or the project root,
// from the backend's trusted-location list.
func (m *Model) RemoveTrustedLocation() {
	root := m.Root()
	if root == "" || m.ctx == nil || m.mgr == nil {
		return
	}
	folder := root
	if target, ok := m.selectedTreeEntry(); ok {
		folder = target.folder()
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		_, err := m.mgr.Invoke(ctx, Window, "project_trust_remove_location", map[string]any{"path": folder})
		m.post(func() {
			if m.Root() != root {
				return
			}
			if err != nil {
				m.run.trustErr = "Could not remove the trusted location: " + err.Error()
			} else {
				m.run.trustErr = ""
				m.status = "Removed " + folder + " from trusted locations"
			}
			m.invalidate()
		})
	}()
}

// PreviewProjectRestricted dismisses the trust prompt and keeps the project
// open with builds, runs, debuggers and language services disabled.
func (m *Model) PreviewProjectRestricted() {
	m.clearTrustPrompt()
	m.status = "This project is open in Restricted Mode"
	m.invalidate()
}

// DontOpenProject rejects an untrusted project and returns to the Welcome
// screen, matching OpenIDEA's third trust-dialog choice.
func (m *Model) DontOpenProject() {
	m.clearTrustPrompt()
	m.CloseProject()
}

func (m *Model) clearTrustPrompt() {
	m.trustAsk = false
	m.trustFolder = false
	m.run.pendingTrust = false
	m.run.pendingCoverage = false
	m.run.pendingContext = false
	m.run.pendingContextCoverage = false
	m.run.pendingDebugContext = false
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
				if m.run.pendingCoverage {
					m.run.pendingStart = false
					m.run.pendingCoverage = false
				}
				if m.run.pendingContext {
					m.run.pendingContext = false
					m.run.pendingContextCoverage = false
				}
				m.run.pendingDebugContext = false
				m.run.err = "Could not read the run configurations: " + err.Error()
				m.run.visible = true
				m.occupyBottom("run")
				m.invalidate()
				return
			}
			m.run.configs = configs
			m.run.inline = nil
			m.run.loaded = true
			m.refreshReadiness()
			m.run.sel = pickRunConfig(configs)
			m.invalidate()
			if m.run.pendingContext {
				m.startContextRun()
				return
			}
			if m.run.pendingDebugContext {
				m.startContextDebug()
				return
			}
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
	m.run.pendingCoverage = false
	m.DockShow("run")
	if m.busy {
		m.invalidate()
		return
	}
	if !m.run.trusted {
		m.run.pendingTrust = true
		m.trustAsk = true
		m.trustFolder = false
		m.status = "The project is not trusted: accept the prompt before running"
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
		if m.run.pendingCoverage {
			m.run.pendingCoverage = false
			m.notifyInfo("run.coverage", "Choose a run configuration to collect coverage", "")
			return
		}
		m.loadRunConfigs()
		return
	}
	if m.run.pendingCoverage {
		m.run.pendingCoverage = false
		m.startRunWithCoverage(cfg.Raw(), cfg.Name, &coverageExecution{name: cfg.Name, root: m.Root()})
		return
	}
	m.startRunWith(json.RawMessage(cfg.Raw()), cfg.Name)
}

// startRunWith starts an arbitrary configuration, which is how a typed
// command line (Run Anything) runs beside the detected configurations.
func (m *Model) startRunWith(raw json.RawMessage, name string) bool {
	return m.startRunWithCoverage(raw, name, nil)
}

func (m *Model) startRunWithCoverage(raw json.RawMessage, name string, coverage *coverageExecution) bool {
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
	// withTestReporting (testing/index.ts): a `go test` run reports per test
	// only with -json, which the parser consumes; the console shows the
	// records' output text.
	var dotnetReports *adapter.DotnetReports
	if probeCfg, rerr := openidea.DecodeRunConfig(raw); rerr == nil {
		switch adapter.FrameworkFor(probeCfg) {
		case "go":
			if len(probeCfg.Args) > 0 && probeCfg.Args[0] == "test" {
				hasJSON := false
				for _, a := range probeCfg.Args {
					if a == "-json" || strings.HasPrefix(a, "-json=") {
						hasJSON = true
						break
					}
				}
				if !hasJSON {
					if patched, ok := patchRunConfigArgs(raw, append([]string{"test", "-json"}, probeCfg.Args[1:]...)); ok {
						raw = patched
					}
				}
			}
		case "pytest":
			// withTestReporting: pytest prints progress dots without -v, so
			// passed tests would never reach the tree.
			chosen := false
			for _, a := range probeCfg.Args {
				if a == "-v" || a == "-q" || a == "--verbose" || a == "--quiet" || strings.HasPrefix(a, "--verbosity") {
					chosen = true
					break
				}
			}
			if !chosen {
				if patched, ok := patchRunConfigArgs(raw, append(append([]string(nil), probeCfg.Args...), "-v")); ok {
					raw = patched
				}
			}
		case "unittest":
			args := adapter.UnittestWithVerbose(probeCfg)
			if patched, ok := patchRunConfigArgs(raw, args); ok {
				raw = patched
			}
		case "phpunit":
			args := adapter.PHPUnitWithTeamcity(probeCfg.Args)
			if patched, ok := patchRunConfigArgs(raw, args); ok {
				raw = patched
			}
		case "ruby":
			if probeCfg.Kind != "rspec" {
				args := adapter.RubyVerboseArgs(probeCfg)
				if patched, ok := patchRunConfigArgs(raw, args); ok {
					raw = patched
				}
			}
		case "dotnet":
			dotnetReports = adapter.NewDotnetReports(probeCfg)
			args := dotnetReports.Args(probeCfg)
			if patched, ok := patchRunConfigArgs(raw, args); ok {
				raw = patched
			}
		}
	}
	m.run.lastID = probe.ID
	m.run.lastRaw = append(json.RawMessage(nil), raw...)
	m.run.lastName = name
	// A JVM or Go configuration's output reports tests per test: fold it
	// into the results tree the rerun-failed action works from. Upstream's
	// withTestReporting turns `go test` into `go test -json` so passed tests
	// reach the tree.
	if probeCfg, err := openidea.DecodeRunConfig(raw); err == nil {
		switch adapter.FrameworkFor(probeCfg) {
		case "jvm":
			m.run.tests = adapter.NewTestResults()
			m.run.testParser = adapter.NewJvmParser()
			m.run.testStdout = adapter.NewTestStdoutBuffer()
		case "go":
			m.run.tests = adapter.NewTestResults()
			m.run.testGo = adapter.NewGoParser()
			m.run.testStdout = adapter.NewTestStdoutBuffer()
		case "cargo":
			m.run.tests = adapter.NewTestResults()
			m.run.testCargo = adapter.NewCargoParser()
			m.run.testStdout = adapter.NewTestStdoutBuffer()
		case "pytest":
			m.run.tests = adapter.NewTestResults()
			m.run.testPytest = adapter.NewPytestParser()
			m.run.testStdout = adapter.NewTestStdoutBuffer()
		case "unittest":
			m.run.tests = adapter.NewTestResults()
			m.run.testUnittest = adapter.NewUnittestParser()
			m.run.testStdout = adapter.NewTestStdoutBuffer()
		case "js":
			m.run.tests = adapter.NewTestResults()
			m.run.testJs = adapter.NewJsParser()
			m.run.testStdout = adapter.NewTestStdoutBuffer()
		case "phpunit":
			m.run.tests = adapter.NewTestResults()
			m.run.testPHPUnit = adapter.NewPHPUnitParser()
			m.run.testStdout = adapter.NewTestStdoutBuffer()
		case "ruby":
			m.run.tests = adapter.NewTestResults()
			m.run.testRuby = adapter.NewRubyParser()
			m.run.testStdout = adapter.NewTestStdoutBuffer()
		case "dotnet":
			m.run.tests = adapter.NewTestResults()
			m.run.testDotnet = dotnetReports
			m.run.testStdout = adapter.NewTestStdoutBuffer()
		}
	}
	if m.run.tests == nil {
		m.run.testParser = nil
		m.run.testGo = nil
		m.run.testCargo = nil
		m.run.testPytest = nil
		m.run.testUnittest = nil
		m.run.testJs = nil
		m.run.testPHPUnit = nil
		m.run.testRuby = nil
		m.run.testDotnet = nil
		m.run.testStdout = nil
	}
	tabs := append([]*Tab(nil), m.tabs...)
	root := m.Root()
	ch := m.nextChannel()
	if m.stackTrace != nil {
		m.stackTrace.active = -1
	}
	m.run.visible = true
	m.occupyBottom("run")
	m.run.busy = true
	m.run.awaiting = true
	m.run.running = false
	m.run.runID = 0
	m.run.coverage = coverage
	m.run.channel = ch
	m.run.pid = nil
	m.run.exitCode = nil
	m.run.err = ""
	m.run.output = nil
	m.run.truncated = false
	m.status = "Starting " + name + "…"
	m.invalidate()

	args := map[string]any{
		"config":   raw,
		"onOutput": map[string]any{"$channel": ch},
		"request":  nil,
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 5*time.Minute)
		defer cancel()
		fail := func(message string, plan *coveragePlan, err error) {
			m.post(func() {
				if m.Root() != root || m.run.channel != ch {
					return
				}
				m.run.busy = false
				m.run.awaiting = false
				m.run.coverage = nil
				m.run.err = message + err.Error()
				if plan != nil {
					m.reportCoveragePreparationError(name, *plan, err)
				}
				m.invalidate()
			})
		}
		// The backend builds from disk, so unsaved text must be written first.
		if err := m.saveOpenTabs(ctx, tabs); err != nil {
			fail("Saving before running failed: ", nil, err)
			return
		}
		launch := raw
		var plan coveragePlan
		if coverage != nil {
			var err error
			plan, err = m.prepareCoverage(ctx, raw)
			if err != nil {
				fail("Preparing coverage failed: ", &plan, err)
				return
			}
			launch = plan.Launch
		}
		resolved, err := m.resolveJVMConfig(ctx, launch, root, m.reportRunConsole(root, ch))
		if err != nil {
			fail("Starting failed: ", nil, err)
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
				m.run.coverage = nil
				m.run.err = "Starting failed: " + err.Error()
				m.invalidate()
				return
			}
			m.run.runID = info.ID
			m.run.pid = info.PID
			m.run.running = info.Running
			if coverage != nil {
				coverage.report = plan.Report
				for _, exit := range coverage.early {
					if exit.Run == info.ID {
						m.acceptExit(exit)
						return
					}
				}
				coverage.early = nil
			}
			if !info.Running {
				m.run.awaiting = false
				m.run.coverage = nil
			}
			m.status = "Running: " + info.Name
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
		m.run.err = "The process has not started"
		m.invalidate()
		return
	}
	m.status = "Stopping…"
	coverage := m.run.coverage
	if coverage != nil {
		coverage.stopped = true
	}
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
		defer cancel()
		if _, err := m.invoke(ctx, "run_stop", map[string]any{"id": id}); err != nil {
			m.post(func() {
				if coverage != nil && m.run.coverage == coverage {
					coverage.stopped = false
				}
				m.run.err = "Stopping failed: " + err.Error()
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
				case "languageServer":
					var e openidea.LanguageServerEvent
					if json.Unmarshal(env.Event, &e) != nil {
						continue
					}
					m.post(func() { m.acceptLanguageServerEvent(e) })
				case "languageServerWarning":
					var e openidea.LanguageServerWarningEvent
					if json.Unmarshal(env.Event, &e) != nil {
						continue
					}
					m.post(func() { m.acceptLanguageServerWarning(e) })
				case "languageServerRemoved":
					var e openidea.LanguageServerRemovedEvent
					if json.Unmarshal(env.Event, &e) != nil {
						continue
					}
					m.post(func() { m.acceptLanguageServerRemoved(e) })
				}
			}
		}
	}
}

func (m *Model) acceptChunk(id uint32, data json.RawMessage) {
	if m.acceptJDKEvent(id, data) {
		return
	}
	if m.acceptToolEvent(id, data) {
		return
	}
	switch {
	case m.acceptVCSOperationOutput(id, data):
	case m.acceptVCSMoreOutput(id, data):
	case id != 0 && id == m.agents.registry.InstallChannel:
		m.acceptRegistryInstallChunk(id, data)
	case id != 0 && id == m.agents.channel:
		m.acceptAgentChunk(id, data)
	case id != 0 && id == m.run.channel:
		var chunk openidea.OutputChunk
		if json.Unmarshal(data, &chunk) != nil {
			return
		}
		if display := m.feedRunTests(chunk.Text); display != chunk.Text {
			chunk.Text = display
		}
		m.run.append(chunk)
		m.invalidate()
	case id != 0 && id == m.debug.heap.channel:
		m.acceptDebugHeapProgress(id, data)
	case id != 0 && id == m.build.channel:
		m.acceptBuildEvent(data)
	case id != 0 && id == m.maven.channel && m.maven.syncing:
		m.acceptMavenEvent(data)
	case id != 0 && m.term.byChannel(id) != nil:
		m.acceptTermChunk(id, data)
	case id != 0 && m.se != nil && id == m.se.textCh:
		m.acceptSearchEverywhereEvent(data)
	case id != 0 && id == m.search.channel:
		m.acceptSearchEvent(data)
	case id != 0 && id == m.search.replaceChannel:
		m.acceptReplaceEvent(data)
	case id != 0 && id == m.impl.request:
		m.acceptImplementationChunk(data)
	case id != 0 && id == m.usages.channel:
		m.acceptUsages(id, data)
	case id != 0 && m.usagePopup != nil && id == m.usagePopup.channel:
		m.acceptUsagePopup(id, data)
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

// testTreeLine is one row of the results tree view: its node id, the status
// mark, the display text and the node's source position when it has one.
type testTreeLine struct {
	id   string
	mark string
	text string
	loc  *adapter.TestLocation
}

// treeLines flattens the results tree for the view: one row per node, roots
// first, children indented, with the aggregated status mark and the duration.
func (p *RunPane) treeLines() []testTreeLine {
	if p.tests == nil || p.tests.IsEmpty() {
		return nil
	}
	mark := func(n *adapter.TestNode) string {
		switch adapter.TestNodeStatusOf(n) {
		case adapter.TestPassed:
			return "✓"
		case adapter.TestFailed:
			return "✗"
		case adapter.TestSkipped:
			return "⊘"
		case adapter.TestRunning:
			return "⟳"
		}
		return "·"
	}
	var out []testTreeLine
	var walk func(n *adapter.TestNode, depth int)
	walk = func(n *adapter.TestNode, depth int) {
		text := strings.Repeat("    ", depth) + n.Name
		if n.DurationMs != nil {
			text += fmt.Sprintf(" (%.1f ms)", *n.DurationMs)
		}
		out = append(out, testTreeLine{id: n.ID, mark: mark(n), text: text, loc: n.Location})
		for _, ch := range n.Children {
			walk(ch, depth+1)
		}
	}
	for _, root := range p.tests.Roots {
		walk(root, 0)
	}
	return out
}

// patchRunConfigArgs writes new arguments into a configuration's raw
// object, keeping the fields GoIDEA does not model.
func patchRunConfigArgs(raw json.RawMessage, args []string) (json.RawMessage, bool) {
	obj := map[string]any{}
	if json.Unmarshal(raw, &obj) != nil {
		return raw, false
	}
	obj["args"] = args
	out, err := json.Marshal(obj)
	if err != nil {
		return raw, false
	}
	return out, true
}

// openTestLocation opens the file of a test node's location at its line,
// resolving a bare file name against the project the way the parsers record
// it (`name` resolutions) and an absolute path directly.
func (m *Model) openTestLocation(loc *adapter.TestLocation) {
	if loc == nil || loc.Path == "" {
		return
	}
	path := loc.Path
	if loc.Resolve == "name" || !filepath.IsAbs(path) {
		// The parsers record a source file's name relative to the project
		// root; the backend's index resolves it.
		raw, err := m.invoke(m.ctx, "find_files", map[string]any{"query": filepath.Base(path), "limit": 20})
		if err == nil {
			var files []openidea.FileMatch
			if json.Unmarshal(raw, &files) == nil {
				for _, f := range files {
					if strings.HasSuffix(filepath.ToSlash(f.RelPath), filepath.ToSlash(path)) ||
						strings.HasSuffix(filepath.ToSlash(f.RelPath), "/"+filepath.Base(path)) {
						path = f.Path
						break
					}
				}
			}
		}
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(m.Root(), path)
	}
	if line := loc.Line - 1; line > 0 {
		// The jump must survive the document load: record it first, so the
		// open's completion lands on the failure's line, then apply it too
		// when the tab already exists.
		if m.pendingJump == nil {
			m.pendingJump = map[string]int{}
		}
		m.pendingJump[realPath(path)] = line
	}
	m.openFile(path)
	if line := loc.Line - 1; line > 0 {
		if t := m.tab(); t != nil && t.doc.Path() == realPath(path) {
			delete(m.pendingJump, realPath(path))
			t.state.GoTo(editor.Pos{Line: line, Col: max(loc.Col-1, 0)})
		}
	}
	m.invalidate()
}

// feedRunTests feeds the run's output lines to the test parser (types.ts
// TestStdout: complete unsplit lines), folding the events into the tree. The
// parser matches plain text, so ANSI is stripped first, exactly as the
// console strips it. `go test -json` records render as their Output text.
func (m *Model) feedRunTests(text string) string {
	if m.run.testStdout == nil || m.run.tests == nil {
		return text
	}
	return m.foldRunTestLines(m.run.testStdout.Feed(stripANSI(text)), text)
}

func (m *Model) flushRunTests() string {
	if m.run.testStdout == nil || m.run.tests == nil {
		return ""
	}
	lines := m.run.testStdout.End()
	return m.foldRunTestLines(lines, strings.Join(lines, ""))
}

func (m *Model) foldRunTestLines(lines []string, fallback string) string {
	var display []string
	for _, line := range lines {
		var events []adapter.TestEvent
		if m.run.testGo != nil {
			events = m.run.testGo.Feed(line)
			display = append(display, adapter.GoConsoleText(line))
		} else if m.run.testCargo != nil {
			events = m.run.testCargo.Feed(line)
		} else if m.run.testPytest != nil {
			events = m.run.testPytest.Feed(line)
		} else if m.run.testUnittest != nil {
			events = m.run.testUnittest.Feed(line)
		} else if m.run.testJs != nil {
			events = m.run.testJs.Feed(line)
		} else if m.run.testPHPUnit != nil {
			events = m.run.testPHPUnit.Feed(line)
			if adapter.ServiceMessageOf(line) == nil {
				display = append(display, line+"\n")
			}
		} else if m.run.testRuby != nil {
			events = m.run.testRuby.Feed(line)
			display = append(display, adapter.RubyConsoleText(line))
		} else if m.run.testDotnet != nil {
			m.run.testDotnet.Feed(line)
		} else if m.run.testParser != nil {
			events = m.run.testParser.Feed(line)
		}
		m.run.tests.Apply(events)
	}
	if m.run.testGo != nil || m.run.testPHPUnit != nil {
		return strings.Join(display, "")
	}
	if m.run.testRuby != nil {
		return strings.Join(display, "")
	}
	return fallback
}

func (m *Model) acceptExit(e openidea.RunExitedEvent) {
	if !m.run.awaiting {
		return
	}
	if m.run.runID != 0 && e.Run != m.run.runID {
		return
	}
	if coverage := m.run.coverage; coverage != nil {
		// A short-lived process can exit before run_start returns its ID.
		if m.run.runID == 0 {
			coverage.saveEarlyExit(e)
			return
		}
		m.run.coverage = nil
		m.collectCoverage(coverage, e)
	}
	m.run.awaiting = false
	m.run.running = false
	m.run.busy = false
	m.run.exitCode = e.Code
	// The test results tree finishes with the process (results.ts finish).
	if m.run.tests != nil {
		if display := m.flushRunTests(); display != "" {
			m.run.append(openidea.OutputChunk{Kind: "stdout", Text: display})
		}
		if m.run.testUnittest != nil {
			m.run.tests.Apply(m.run.testUnittest.End())
		}
		code := 0
		if e.Code != nil {
			code = int(*e.Code)
		}
		m.run.tests.Finish(code)
	}
	if reports := m.run.testDotnet; reports != nil && m.run.tests != nil {
		tests := m.run.tests
		channel := m.run.channel
		root := m.Root()
		go func() {
			events, err := reports.ReadReports(func(path string) (string, error) {
				data, readErr := os.ReadFile(path)
				return string(data), readErr
			})
			m.post(func() {
				if m.Root() != root || m.run.channel != channel || m.run.tests != tests {
					return
				}
				tests.Apply(events)
				if err != nil {
					m.notifyWarning("run.dotnet-report", "Could not read .NET test results", notifySuffix(err.Error()))
				}
				m.invalidate()
			})
		}()
	}
	switch {
	case e.Code == nil:
		m.status = "The run ended"
	case *e.Code == 0:
		m.status = "Run succeeded (exit code 0)"
	default:
		m.status = fmt.Sprintf("Run failed (exit code %d)", *e.Code)
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
		return "", nil, "", errors.New("No project is open")
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
		return "", nil, "", errors.New("The project has no runnable configuration")
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
			return "", nil, "", fmt.Errorf("There is no run configuration named %s", want)
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
		return out, errors.New("No project is open")
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
			return out, fmt.Errorf("No spring-boot run configuration appeared within 6 minutes; detected: %s",
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
		return out, errors.New("A multi-module project must resolve to a submodule, not the project root")
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
		return out, errors.New("The resolved configuration still has classpath placeholders: " + resolvedText)
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
		return out, errors.New("The resolved classpath is empty")
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
		return out, fmt.Errorf("The Spring Boot startup marker %q did not appear within 4 minutes; output tail: %s", marker, tailOf(text, 1200))
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
		return out, errors.New("The Spring Boot startup marker appeared, but the process is no longer running")
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
			return out, errors.New("The process did not exit after run_stop")
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
			return out, fmt.Errorf("%d orphaned JVM processes remained after run_stop: %s", len(orphans), strings.Join(orphans, ", "))
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
		return out, errors.New("No project is open")
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
		return out, errors.New("The project has no runnable configuration")
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
		return out, fmt.Errorf("After saving, run_configs did not return the user configuration %s", duplicate.ID)
	}
	if saved.Detected || saved.Name != editedName {
		return out, fmt.Errorf("The saved configuration did not come back as edited: name=%s detected=%v", saved.Name, saved.Detected)
	}
	out["saved"] = true
	out["savedName"] = saved.Name

	// The build policy rewrites a Maven command line at launch: rebuild
	// prepends clean (upstream applyBuildPolicyToCommand).
	resolved, err := m.resolveJVMConfig(ctx, saved.Raw(), root, nil)
	if err != nil {
		return out, fmt.Errorf("Could not resolve the build policy: %w", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(resolved, &obj); err != nil {
		return out, err
	}
	args := toStrings(obj["args"])
	out["policyArgs"] = args
	if len(args) == 0 || args[0] != "clean" {
		return out, fmt.Errorf("The rebuild policy did not rewrite the Maven command line before launch: %v", args)
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
			return out, errors.New("run_configs still returns the configuration after deleting it")
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

// trustDialog asks whether a project may build and run. It mirrors
// OpenIDEA's 560 DIP Trust and Open Project dialog: the project path, the
// Restricted Mode explanation, an optional trusted-parent-location switch,
// and Preview, Don't Open and Trust buttons in the upstream order.
func (m *Model) trustDialog(c *ui.Context, p *ui.Theme) {
	open := m.trustAsk
	oi := oiPaletteOf(p)
	path := m.Root()
	if path == "" {
		path = "No project selected"
	}
	dialog := ui.Modal(c.Key("trust-dialog"), &open, func() {
		ui.Column(c).Width(560).Gap(12).Children(func() {
			ui.Row(c).Gap(10).AlignItems(ui.Start).Children(func() {
				if logo := welcomeLogoImage(); logo != nil {
					ui.Image(c, logo).Size(32, 32)
				}
				ui.Column(c).Grow(1).MinWidth(0).Children(func() {
					ui.Text(c, "Trust and Open Project?").FontSize(13).Bold().
						TextColor(oi.text).SingleLine()
					ui.Text(c, shortenHome(path)).FontSize(12).TextColor(oi.textSubtle).
						SingleLine().Ellipsis("…")
				})
			})
			ui.Text(c, "If you don't trust the source, preview the project in Restricted Mode, where project commands and tools can't run.").
				FontSize(12).TextColor(oi.textMuted)
			ui.Text(c, "Loading, running or building a project may execute potentially malicious code from its build scripts.").
				FontSize(12).TextColor(oi.textMuted)
			if m.run.trustErr != "" {
				ui.Text(c, m.run.trustErr).FontSize(12).TextColor(oi.danger)
			}
			ui.Checkbox(c, &m.trustFolder, "Trust projects in "+shortenHome(filepath.Dir(path)))
			ui.Row(c).Gap(8).Children(func() {
				ui.Spacer(c)
				ui.Button(c, "Preview in Restricted Mode").OnClick(func() {
					m.PreviewProjectRestricted()
				})
				ui.Button(c, "Don't Open").OnClick(func() {
					m.DontOpenProject()
				})
				ui.Button(c, "Trust Project").OnClick(func() {
					m.TrustProject()
					m.invalidate()
				})
			})
		})
	})
	dialog.Padding(0)
	if !open {
		if m.trustAsk {
			m.DontOpenProject()
		}
	}
}
