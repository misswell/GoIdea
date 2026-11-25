package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mayoi/internal/adapter"
	"mayoi/internal/openidea"
)

// runChild is one independent session of a Compound configuration or of a
// "Run Another Configuration" before-launch task. A Compound keeps the sessions
// separate, like OpenIDEA's Run tool-window content tabs, instead of pretending
// their unrelated output belongs to one process.
type runChild struct {
	channel uint32
	runID   uint32
	name    string
	config  openidea.RunConfig
	groupID string
	// service marks a session owned by Services rather than the Run window.
	// root keeps a late event from attaching it after a project switch.
	service bool
	root    string

	pid         *int32
	starting    bool
	running     bool
	stopped     bool
	exitCode    *int32
	err         string
	finished    bool
	started     time.Time
	ended       time.Time
	stdinClosed bool
	done        chan error

	output    []outputLine
	truncated bool
	dropped   int

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
	selTest      string
}

func (r *runChild) append(c openidea.OutputChunk) {
	text := strings.TrimSuffix(stripANSI(c.Text), "\n")
	if text == "" {
		return
	}
	kind := c.Kind
	if kind == "" {
		kind = "stdout"
	}
	for _, line := range strings.Split(text, "\n") {
		r.output = append(r.output, outputLine{kind: kind, text: line})
	}
	if over := len(r.output) - maxRunLines; over > 0 {
		r.output = append(r.output[:0], r.output[over:]...)
		r.truncated = true
		r.dropped += over
	}
}

func (r *runChild) renderText() string {
	var b strings.Builder
	for i, line := range r.output {
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

func (r *runChild) live() bool {
	return !r.finished && (r.starting || r.running)
}

func (m *Model) childByChannel(channel uint32) *runChild {
	for _, child := range m.run.children {
		if child.channel == channel {
			return child
		}
	}
	for _, child := range m.services.children {
		if child.channel == channel {
			return child
		}
	}
	return nil
}

func (m *Model) childByRun(runID uint32) *runChild {
	if runID == 0 {
		return nil
	}
	for _, child := range m.run.children {
		if child.runID == runID {
			return child
		}
	}
	for _, child := range m.services.children {
		if child.runID == runID {
			return child
		}
	}
	return nil
}

func (m *Model) activeChild() *runChild {
	if m.run.activeChild == 0 {
		return nil
	}
	return m.childByChannel(m.run.activeChild)
}

func (m *Model) hasStartingChild() bool {
	for _, child := range m.run.children {
		if child.starting {
			return true
		}
	}
	return false
}

func (m *Model) hasStartingServiceChild() bool {
	for _, child := range m.services.children {
		if child.service && child.starting {
			return true
		}
	}
	return false
}

func (m *Model) addRunChild(child *runChild, show bool) {
	m.run.children = append(m.run.children, child)
	if show || m.run.activeChild == 0 {
		m.run.activeChild = child.channel
	}
	m.syncChildActivity()
	m.invalidate()
}

func (m *Model) syncChildActivity() {
	if len(m.run.children) == 0 {
		return
	}
	running := false
	awaiting := false
	starting := false
	firstErr := ""
	for _, child := range m.run.children {
		if child.err != "" && firstErr == "" {
			firstErr = child.err
		}
		if child.finished {
			continue
		}
		awaiting = true
		starting = starting || child.starting
		running = running || child.running
	}
	m.run.busy = m.run.preparingGroup || starting
	m.run.awaiting = awaiting
	m.run.running = running
	m.run.err = firstErr
	if child := m.activeChild(); child != nil {
		m.run.exitCode = child.exitCode
		if m.run.activeChild != 0 {
			m.run.err = child.err
		}
	}
	switch {
	case starting:
		m.status = "Starting configuration…"
	case running:
		if child := m.activeChild(); child != nil {
			m.status = "Running: " + child.name
		} else {
			m.status = fmt.Sprintf("Running %d configurations", len(m.run.children))
		}
	case awaiting:
		m.status = "Waiting for configurations to exit…"
	case m.run.err != "":
		m.status = m.run.err
	default:
		m.status = "Run finished"
	}
}

// acceptChildChunk routes a streaming channel to its Compound/before-launch
// session. It returns true when the chunk belonged to a child.
func (m *Model) acceptChildChunk(channel uint32, data json.RawMessage) bool {
	child := m.childByChannel(channel)
	if child == nil {
		return m.runChildChannelRetired(channel)
	}
	var chunk openidea.OutputChunk
	if json.Unmarshal(data, &chunk) == nil {
		if display := m.feedChildTests(child, chunk.Text); display != chunk.Text {
			chunk.Text = display
		}
		child.append(chunk)
		m.invalidate()
	}
	return true
}

// acceptChildExit applies a runExited event to a child session. An exit can
// beat run_start's reply, so an id that is not registered yet is kept until
// startChildRun learns that id.
func (m *Model) acceptChildExit(runID uint32, code *int32) bool {
	if m.runChildRunRetired(runID) {
		return true
	}
	if child := m.childByRun(runID); child != nil {
		m.finishRunChild(child, code, "")
		return true
	}
	startingRun := m.hasStartingChild()
	startingService := m.hasStartingServiceChild()
	if !startingRun && !startingService {
		return false
	}
	if startingRun {
		if m.run.pendingChildExits == nil {
			m.run.pendingChildExits = map[uint32]*int32{}
		}
		m.run.pendingChildExits[runID] = code
	}
	if startingService {
		if m.services.pendingChildExits == nil {
			m.services.pendingChildExits = map[uint32]*int32{}
		}
		m.services.pendingChildExits[runID] = code
	}
	return true
}

func (m *Model) finishRunChild(child *runChild, code *int32, errText string) {
	if child == nil || child.finished {
		return
	}
	child.starting = false
	child.running = false
	child.finished = true
	child.ended = time.Now()
	child.exitCode = code
	child.err = errText
	testCode := 0
	if code != nil {
		testCode = int(*code)
	}
	m.finishChildTests(child, testCode)
	if child.done != nil {
		var doneErr error
		if errText != "" {
			doneErr = errors.New(errText)
		}
		select {
		case child.done <- doneErr:
		default:
		}
	}
	if child.service {
		m.syncServiceActivity()
		m.invalidate()
		return
	}
	m.syncChildActivity()
	m.invalidate()
}

func (m *Model) registerRunChild(child *runChild, info openidea.RunInfo, err error) {
	if child == nil {
		return
	}
	if m.runChildChannelRetired(child.channel) || m.runChildRunRetired(info.ID) ||
		!m.runChildOwned(child) {
		m.retireRunChild(child)
		m.retireRunChildRun(info.ID)
		if info.ID != 0 {
			go func(id uint32) {
				ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
				defer cancel()
				_, _ = m.invoke(ctx, "run_stop", map[string]any{"id": id})
			}(info.ID)
		}
		return
	}
	if err != nil {
		m.finishRunChild(child, nil, "Starting failed: "+err.Error())
		return
	}
	if child.stopped {
		if info.ID != 0 {
			go func(id uint32) {
				ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
				defer cancel()
				_, _ = m.invoke(ctx, "run_stop", map[string]any{"id": id})
			}(info.ID)
		}
		return
	}
	child.runID = info.ID
	child.pid = info.PID
	child.starting = false
	child.running = info.Running
	pending := m.run.pendingChildExits
	if child.service {
		pending = m.services.pendingChildExits
	}
	if code, ok := pending[info.ID]; ok {
		delete(pending, info.ID)
		m.finishRunChild(child, code, "")
		return
	}
	if child.service {
		m.syncServiceActivity()
		m.invalidate()
		return
	}
	m.syncChildActivity()
	m.invalidate()
}

func configRaw(cfg *openidea.RunConfig) (json.RawMessage, error) {
	if cfg == nil {
		return nil, errors.New("missing configuration")
	}
	if raw := cfg.Raw(); len(raw) > 0 {
		return raw, nil
	}
	return json.Marshal(cfg)
}

func containsRunID(chain []string, id string) bool {
	for _, item := range chain {
		if item == id {
			return true
		}
	}
	return false
}

func appendRunID(chain []string, id string) []string {
	return append(append([]string(nil), chain...), id)
}

// prepareBeforeLaunch runs the configuration's before-launch list in order.
// The first failing task aborts the launch.
func (m *Model) prepareBeforeLaunch(
	ctx context.Context,
	cfg *openidea.RunConfig,
	chain []string,
	show bool,
	groupID string,
) error {
	if cfg == nil || len(cfg.BeforeLaunch) == 0 {
		return nil
	}
	var tools []ExternalToolPrefs
	var configs []*openidea.RunConfig
	m.post(func() {
		tools = cloneExternalTools(m.prefs.ExternalTools)
		configs = runConfigPointers(m.run.configs)
	})
	taskChain := appendRunID(chain, cfg.ID)
	for _, task := range cfg.BeforeLaunch {
		var err error
		switch task.Kind {
		case "run":
			err = m.runAnotherBeforeLaunch(ctx, task.Target, taskChain, show, groupID, configs)
		case "build":
			if task.Target == "" {
				err = m.buildBeforeLaunch(ctx, "build", nil, nil, false)
			} else {
				buildID := task.Target
				found := false
				for _, config := range configs {
					if config.ID == buildID {
						found = config.Kind == "cmake"
						break
					}
				}
				if !found {
					err = fmt.Errorf("CMake build configuration '%s' no longer exists", buildID)
				} else {
					err = m.buildBeforeLaunch(ctx, "build", nil, &buildID, false)
				}
			}
		case "buildModule":
			if strings.TrimSpace(task.Target) == "" {
				err = errors.New("No module selected")
			} else {
				module := moduleBuildPath(m.Root(), task.Target)
				err = m.buildBeforeLaunch(ctx, "build", &module, nil, false)
			}
		case "buildArtifacts":
			err = m.buildBeforeLaunch(ctx, "artifacts", nil, nil, false)
		case "dotnetBuild":
			built := dotnetBeforeLaunchConfig(cfg, m.Root())
			err = m.runChildBeforeLaunch(ctx, built, taskChain, show, groupID)
		case "command":
			if strings.TrimSpace(task.Target) == "" {
				err = errors.New("No command")
			} else {
				command := shellBeforeLaunchConfig(cfg, task.Target)
				err = m.runChildBeforeLaunch(ctx, command, taskChain, show, groupID)
			}
		case "toolWindow":
			if dockSpec(task.Target) == nil {
				err = fmt.Errorf("Tool window '%s' is not available", task.Target)
			} else {
				m.post(func() {
					m.activateTool(task.Target)
					m.invalidate()
				})
			}
		case "tool":
			var toolConfig *openidea.RunConfig
			toolConfig, err = m.externalToolRunConfig(task.Target, cfg)
			if err == nil {
				err = m.runChildBeforeLaunch(ctx, toolConfig, taskChain, show, groupID)
			}
		default:
			err = fmt.Errorf("Unknown before-launch task '%s'", task.Kind)
		}
		if err != nil {
			return fmt.Errorf("%s %w", beforeLaunchTaskLabel(
				task, configs, tools,
			), err)
		}
	}
	return nil
}

func (m *Model) runAnotherBeforeLaunch(
	ctx context.Context,
	id string,
	chain []string,
	show bool,
	groupID string,
	configs []*openidea.RunConfig,
) error {
	if containsRunID(chain, id) {
		return errors.New("The before-launch tasks run in a cycle")
	}
	var target *openidea.RunConfig
	for _, config := range configs {
		if config.ID == id {
			target = config
			break
		}
	}
	if target == nil {
		return fmt.Errorf("Configuration '%s' no longer exists", id)
	}
	if target.Kind == "compound" {
		return errors.New("A Compound cannot be a before-launch run target")
	}
	return m.runChildBeforeLaunch(ctx, target, chain, show, groupID)
}

func shellBeforeLaunchConfig(parent *openidea.RunConfig, command string) *openidea.RunConfig {
	return &openidea.RunConfig{
		ID:       "before-launch:" + parent.ID + ":" + command,
		Name:     command,
		Kind:     "shell",
		Command:  command,
		Args:     []string{},
		Cwd:      parent.Cwd,
		Env:      map[string]string{},
		Detected: true,
	}
}

func dotnetBeforeLaunchConfig(parent *openidea.RunConfig, root string) *openidea.RunConfig {
	project, framework := "", ""
	if parent.Params != nil {
		project, _ = parent.Params["project"].(string)
		framework, _ = parent.Params["framework"].(string)
	}
	args := []string{"build", project, "--configuration", "Debug"}
	if framework != "" {
		args = append(args, "--framework", framework)
	}
	command := parent.Command
	if command == "" {
		command = "dotnet"
	}
	projectDir := root
	if root == "" {
		projectDir = ""
	}
	name := "dotnet build"
	if project != "" {
		name += " (" + project + ")"
	}
	return &openidea.RunConfig{
		ID:       "dotnet-build:" + parent.ID,
		Name:     name,
		Kind:     "shell",
		Command:  command,
		Args:     args,
		Cwd:      &projectDir,
		Env:      map[string]string{},
		Detected: true,
	}
}

// runChildToEnd starts a before-launch/Compound member and resolves only after
// its backend process exits.
func (m *Model) runChildToEnd(
	ctx context.Context,
	cfg *openidea.RunConfig,
	chain []string,
	show bool,
	groupID string,
) (*int32, error) {
	if cfg == nil {
		return nil, errors.New("missing configuration")
	}
	if containsRunID(chain, cfg.ID) {
		return nil, errors.New("The run configurations form a cycle")
	}
	if err := m.prepareBeforeLaunch(ctx, cfg, chain, show, groupID); err != nil {
		return nil, err
	}
	child, err := m.startChildRun(ctx, cfg, groupID, show)
	if err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		m.stopRunChild(child)
		return nil, ctx.Err()
	case err := <-child.done:
		if err != nil {
			return child.exitCode, err
		}
		return child.exitCode, nil
	}
}

// runChildBeforeLaunch waits for a prerequisite and turns any non-zero exit
// into an error. A prerequisite without a reported exit code is treated as
// incomplete, so the main configuration never starts on an unknown result.
func (m *Model) runChildBeforeLaunch(
	ctx context.Context,
	cfg *openidea.RunConfig,
	chain []string,
	show bool,
	groupID string,
) error {
	code, err := m.runChildToEnd(ctx, cfg, chain, show, groupID)
	if err != nil {
		return fmt.Errorf("could not run: %w", err)
	}
	if code == nil {
		return errors.New("failed")
	}
	if *code != 0 {
		return fmt.Errorf("failed with exit code %d", *code)
	}
	return nil
}

func (m *Model) startChildRun(
	parent context.Context,
	cfg *openidea.RunConfig,
	groupID string,
	show bool,
) (*runChild, error) {
	return m.startOwnedChildRun(parent, cfg, groupID, show, false)
}

func (m *Model) startServiceChildRun(
	parent context.Context,
	cfg *openidea.RunConfig,
	groupID string,
) (*runChild, error) {
	return m.startOwnedChildRun(parent, cfg, groupID, false, true)
}

func (m *Model) startOwnedChildRun(
	parent context.Context,
	cfg *openidea.RunConfig,
	groupID string,
	show bool,
	service bool,
) (*runChild, error) {
	raw, err := configRaw(cfg)
	if err != nil {
		return nil, err
	}
	raw, dotnetReports := prepareTestReporting(raw, cfg)
	if launchCfg, decodeErr := openidea.DecodeRunConfig(raw); decodeErr == nil {
		cfg = launchCfg
	}
	root := m.Root()
	child := &runChild{
		channel:     m.nextChannel(),
		name:        cfg.Name,
		config:      *cfg,
		groupID:     groupID,
		service:     service,
		root:        root,
		starting:    true,
		started:     time.Now(),
		stdinClosed: cfg.StdinFile != nil,
		done:        make(chan error, 1),
	}
	m.initChildTests(child, dotnetReports)
	m.post(func() {
		if service {
			m.addServiceChild(child)
			return
		}
		m.addRunChild(child, show)
	})

	resolved, err := m.resolveJVMConfig(parent, raw, root, func(text, kind string) {
		m.post(func() {
			child.append(openidea.OutputChunk{Kind: kind, Text: text})
			m.invalidate()
		})
	})
	if err != nil {
		m.post(func() {
			m.finishRunChild(child, nil, "Starting failed: "+err.Error())
		})
		return child, err
	}
	args := map[string]any{
		"config":   resolved,
		"onOutput": map[string]any{"$channel": child.channel},
		"request":  nil,
	}
	rawInfo, err := m.invoke(parent, "run_start", args)
	var info openidea.RunInfo
	if err == nil {
		err = json.Unmarshal(rawInfo, &info)
	}
	m.post(func() { m.registerRunChild(child, info, err) })
	if err != nil {
		return child, err
	}
	return child, nil
}

func (m *Model) stopRunChild(child *runChild) {
	if child == nil || child.finished {
		return
	}
	child.stopped = true
	if child.runID == 0 {
		m.finishRunChild(child, nil, "Stopped before the process started")
		return
	}
	go func(id uint32) {
		ctx, cancel := context.WithTimeout(modelContext(m), 15*time.Second)
		defer cancel()
		_, err := m.invoke(ctx, "run_stop", map[string]any{"id": id})
		if err != nil {
			m.post(func() {
				child.err = "Stopping failed: " + err.Error()
				m.syncChildActivity()
				m.invalidate()
			})
		}
	}(child.runID)
}

// startCompoundRun implements OpenIDEA startCompound: the Compound's own
// before-launch tasks run once, then every flattened member starts
// concurrently. The Run window keeps one output tab per member.
func (m *Model) startCompoundRun(cfg *openidea.RunConfig, coverage *coverageExecution) bool {
	if cfg == nil {
		return false
	}
	configs := runConfigPointers(m.run.configs)
	if adapter.CompoundCycle(cfg, configs) {
		m.run.err = "'" + cfg.Name + "' was not started: Compound configurations form a cycle"
		m.run.busy = false
		m.invalidate()
		return false
	}
	children := adapter.CompoundChildren(cfg, configs)
	if len(children) == 0 {
		m.run.err = "'" + cfg.Name + "' was not started: add at least one configuration"
		m.invalidate()
		return false
	}
	if coverage != nil {
		m.run.err = "Coverage is not available for Compound configurations"
		m.invalidate()
		return false
	}

	tabs := append([]*Tab(nil), m.tabs...)
	root := m.Root()
	pluginGeneration := m.run.pluginGeneration
	m.run.viewChild = nil
	m.run.visible = true
	m.occupyBottom("run")
	m.run.busy = true
	m.run.preparingGroup = true
	m.run.groupID = cfg.ID
	m.run.children = nil
	m.run.activeChild = 0
	m.run.pendingChildExits = map[uint32]*int32{}
	m.run.awaiting = true
	m.run.running = false
	m.run.runID = 0
	m.run.channel = 0
	m.run.pid = nil
	m.run.exitCode = nil
	m.run.err = ""
	m.run.output = nil
	m.run.truncated = false
	m.run.coverage = nil
	m.run.tests = nil
	m.run.lastID = cfg.ID
	m.run.lastRaw = append(json.RawMessage(nil), cfg.Raw()...)
	m.run.lastName = cfg.Name
	m.status = "Starting " + cfg.Name + "…"
	m.invalidate()

	go func() {
		ctx, cancel := context.WithCancel(m.ctx)
		defer cancel()
		if err := m.saveOpenTabs(ctx, tabs); err != nil {
			m.post(func() {
				m.failRunPluginGeneration(pluginGeneration)
				m.run.preparingGroup = false
				m.run.busy = false
				m.run.awaiting = false
				m.run.err = "Saving before running failed: " + err.Error()
				m.status = m.run.err
				m.invalidate()
			})
			return
		}
		if err := m.prepareBeforeLaunch(ctx, cfg, nil, true, cfg.ID); err != nil {
			m.post(func() {
				m.failRunPluginGeneration(pluginGeneration)
				m.run.preparingGroup = false
				m.run.busy = false
				m.run.awaiting = false
				if len(m.run.children) == 0 {
					m.run.err = "'" + cfg.Name + "' was not started: Before launch: " + err.Error()
				} else {
					m.run.err = "Before launch: " + err.Error()
				}
				m.status = m.run.err
				m.invalidate()
			})
			return
		}
		m.post(func() {
			if m.Root() != root || m.run.groupID != cfg.ID {
				m.cancelRunPluginGeneration(pluginGeneration)
				return
			}
			m.run.preparingGroup = false
			m.syncChildActivity()
			m.invalidate()
		})

		chain := []string{cfg.ID}
		var running int
		results := make(chan struct{}, len(children))
		for _, member := range children {
			member := member
			running++
			go func() {
				defer func() { results <- struct{}{} }()
				_, err := m.runChildToEnd(ctx, member, chain, true, cfg.ID)
				if err == nil {
					return
				}
				m.post(func() {
					m.run.err = "'" + member.Name + "' failed: " + err.Error()
					m.status = m.run.err
					m.invalidate()
				})
			}()
		}
		for range running {
			<-results
		}
		m.post(func() {
			if m.Root() != root || m.run.groupID != cfg.ID {
				m.cancelRunPluginGeneration(pluginGeneration)
				return
			}
			m.syncChildActivity()
			if m.run.err == "" {
				m.status = "All configurations finished"
			}
			var code *int32
			for _, child := range m.run.children {
				if code == nil {
					zero := int32(0)
					code = &zero
				}
				if child.exitCode == nil {
					code = nil
					break
				}
				if *child.exitCode != 0 {
					value := *child.exitCode
					code = &value
					break
				}
			}
			m.finishRunPluginGeneration(pluginGeneration, code)
			m.invalidate()
		})
	}()
	return true
}

// externalToolRunConfig is completed by the Tools › External Tools settings
// model. It returns the same shell configuration that OpenIDEA registers.
func (m *Model) externalToolRunConfig(id string, parent *openidea.RunConfig) (*openidea.RunConfig, error) {
	tool, ok := m.externalTool(id)
	if !ok {
		return nil, fmt.Errorf("External tool '%s' no longer exists", id)
	}
	cfg, err := m.expandExternalTool(tool)
	if err != nil {
		return nil, err
	}
	if parent != nil && cfg.Cwd == nil {
		cfg.Cwd = parent.Cwd
	}
	return cfg, nil
}

// childVisibleOutput returns the selected child's console, or nil when the
// primary session is selected.
func (m *Model) childVisibleOutput() *runChild {
	return m.activeChild()
}

func (m *Model) stopSelectedChildren() bool {
	selectedID := ""
	if cfg := m.run.current(); cfg != nil {
		selectedID = cfg.ID
	}
	stopped := false
	for _, child := range m.run.children {
		if child.finished {
			continue
		}
		if selectedID != "" && child.groupID != selectedID {
			continue
		}
		stopped = true
		m.stopRunChild(child)
	}
	return stopped
}

func (m *Model) selectRunChild(channel uint32) {
	m.run.agentActive = 0
	if m.stackTrace != nil {
		m.stackTrace.active = -1
	}
	if channel == 0 {
		m.run.activeChild = 0
	} else if m.childByChannel(channel) != nil {
		m.run.activeChild = channel
	}
	m.invalidate()
}

func (m *Model) closeRunChild(channel uint32) {
	child := m.childByChannel(channel)
	if child == nil {
		return
	}
	m.retireRunChild(child)
	if child.live() {
		m.stopRunChild(child)
	}
	out := m.run.children[:0]
	for _, candidate := range m.run.children {
		if candidate != child {
			out = append(out, candidate)
		}
	}
	m.run.children = out
	if m.run.activeChild == channel {
		m.run.activeChild = 0
	}
	m.syncChildActivity()
	m.invalidate()
}

func moduleBuildPath(root, target string) string {
	if target == "" || filepath.IsAbs(target) {
		return target
	}
	return filepath.Join(root, target)
}

func dotnetOutputFile(cfg *openidea.RunConfig) string {
	if cfg == nil || cfg.OutputFile == nil {
		return ""
	}
	return *cfg.OutputFile
}

func removeRunOutput(path string) {
	if path != "" {
		_ = os.Remove(path)
	}
}
