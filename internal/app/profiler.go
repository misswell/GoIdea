package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

const (
	maxProfileSnapshots   = 5
	liveProfileNotStopped = "The program ended by itself, so no garbage collection ran before the profile " +
		"was written: objects that were already unreachable may show as live."
	liveProfileNotCollected = "No garbage collection finished before the program stopped (the JVM's attach " +
		"listener did not answer in time or was not reachable): objects that were already unreachable may " +
		"show as live."
)

type profileSnapshot struct {
	id        string
	name      string
	createdAt int64
	profile   openidea.Profile
	notice    string

	tab        string
	query      string
	focus      int
	expanded   map[int]bool
	methodSort string
	treeSel    int
	methodSel  int
	treeList   ui.ListState
	methodList ui.ListState
}

type profiledRun struct {
	name        string
	mode        openidea.ProfileMode
	spec        json.RawMessage
	runID       uint32
	gcCollected uint32
	stopped     bool
	stopPending bool
	exited      bool
}

type profilerState struct {
	snapshots []*profileSnapshot
	activeID  string
	tab       string
	query     string
	mode      openidea.ProfileMode

	preparing  bool
	collecting bool
	stopping   bool
	err        string
	notice     string
	plan       *openidea.ProfilePrepared
	activeRun  *profiledRun
	pending    openidea.ProfileMode
	nextID     uint64
}

func (m *Model) ensureProfiler() {
	if m.profiler.tab == "" {
		m.profiler.tab = "flame"
	}
	if m.profiler.mode == "" {
		m.profiler.mode = openidea.ProfileCPU
	}
}

// RunWithProfiler runs the selected configuration under the CPU, allocation
// or live-object profiler. Preparation and collection stay in the backend.
func (m *Model) RunWithProfiler(mode openidea.ProfileMode) {
	m.ensureProfiler()
	if mode == "" {
		mode = openidea.ProfileCPU
	}
	m.profiler.mode = mode
	if m.Root() == "" || m.showingWelcome {
		return
	}
	if m.run.busy || m.run.running || m.debug.starting || (m.debug.session != 0 && !m.debug.ended) {
		m.notifyError("run.profile", "Cannot profile while another run is active", "")
		return
	}
	if !m.run.trusted {
		m.profiler.pending = mode
		m.trustAsk = true
		m.trustFolder = false
		m.status = "The project is not trusted: accept the prompt before profiling"
		m.invalidate()
		return
	}
	if !m.run.loaded {
		m.profiler.pending = mode
		m.loadRunConfigs()
		return
	}
	m.prepareProfiler(mode)
}

func (m *Model) startPendingProfiler() bool {
	m.ensureProfiler()
	if m.profiler.pending == "" {
		return false
	}
	mode := m.profiler.pending
	m.profiler.pending = ""
	m.prepareProfiler(mode)
	return true
}

func (m *Model) prepareProfiler(mode openidea.ProfileMode) {
	cfg := m.run.current()
	if cfg == nil {
		m.notifyInfo("run.profile", "Choose a run configuration to profile", "")
		return
	}
	m.prepareProfilerConfig(cfg, mode)
}

func (m *Model) prepareProfilerConfig(cfg *openidea.RunConfig, mode openidea.ProfileMode) {
	m.ensureProfiler()
	if cfg == nil {
		m.notifyInfo("run.profile", "Choose a run configuration to profile", "")
		return
	}
	if m.profiler.preparing || m.profiler.activeRun != nil || m.run.busy || m.run.running {
		return
	}
	root := m.Root()
	raw := append(json.RawMessage(nil), cfg.Raw()...)
	name := cfg.Name
	m.profiler.preparing = true
	m.profiler.err = ""
	m.profiler.notice = "Preparing " + profileModeName(mode) + " profiler for " + name + "…"
	m.status = m.profiler.notice
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.backgroundContext(), 60*time.Second)
		defer cancel()
		reply, err := m.invoke(ctx, "profiler_prepare", map[string]any{
			"config": raw,
			"mode":   string(mode),
		})
		var plan openidea.ProfilePrepared
		if err == nil {
			err = json.Unmarshal(reply, &plan)
		}
		m.post(func() {
			if m.Root() != root {
				return
			}
			m.profiler.preparing = false
			if err != nil {
				m.profiler.err = "Cannot prepare the profiler: " + err.Error()
				m.profiler.notice = ""
				m.status = m.profiler.err
				m.invalidate()
				return
			}
			m.profiler.plan = &plan
			switch plan.Status {
			case "ready":
				m.profiler.notice = ""
				if plan.Launch == nil {
					m.profiler.err = "The profiler returned no launch configuration"
					m.invalidate()
					return
				}
				active := &profiledRun{
					name: name, mode: mode,
					spec: append(json.RawMessage(nil), plan.Spec...),
				}
				m.profiler.activeRun = active
				if !m.startRunWith(plan.Launch.Raw(), "Profile '"+name+"'") {
					m.profiler.activeRun = nil
					m.profiler.err = "The profiled run could not be started"
				}
			case "missingTool":
				m.profiler.err = plan.Message
				m.profiler.notice = plan.Tool + " is required to profile " + name
				m.notifySticky("profiler-missing-"+plan.Tool, "warning", plan.Tool+" is not installed",
					profilerMissingText(plan.Message, &plan), true, m.profilerInstallActions(&plan)...)
			case "unsupported":
				m.profiler.err = plan.Reason
				m.profiler.notice = "Cannot profile " + name + ": " + plan.Reason
				m.notifyInfo("profiler", "Cannot profile '"+name+"'", plan.Reason)
			default:
				m.profiler.err = "Unknown profiler preparation status: " + plan.Status
			}
			m.invalidate()
		})
	}()
}

// InstallProfiler runs the install command returned by profiler_prepare.
func (m *Model) InstallProfiler() bool {
	plan := m.profiler.plan
	if plan == nil || plan.Status != "missingTool" || plan.Install == nil {
		return false
	}
	install := plan.Install
	config := map[string]any{
		"id":               "install:" + plan.Tool,
		"name":             "Install " + plan.Tool,
		"kind":             "shell",
		"command":          install.Command,
		"args":             install.Args,
		"cwd":              install.Cwd,
		"env":              map[string]string{},
		"detected":         true,
		"isTest":           false,
		"debugAdapter":     nil,
		"buildPolicy":      nil,
		"module":           nil,
		"folder":           nil,
		"beforeLaunch":     []any{},
		"compound":         []string{},
		"includeSystemEnv": true,
	}
	raw, err := json.Marshal(config)
	if err != nil {
		m.profiler.err = "Cannot prepare the profiler installation: " + err.Error()
		m.invalidate()
		return false
	}
	return m.startRunWith(raw, "Install "+plan.Tool)
}

func (m *Model) profilerInstallActions(plan *openidea.ProfilePrepared) []notifyAction {
	if plan == nil || plan.Status != "missingTool" {
		return nil
	}
	var actions []notifyAction
	if plan.Install != nil {
		actions = append(actions, notifyAction{
			label: "Install",
			run:   func(*Model) { m.InstallProfiler() },
		})
	}
	if plan.Homepage != nil && strings.TrimSpace(*plan.Homepage) != "" {
		actions = append(actions, notifyAction{
			label: "Download…",
			run:   func(*Model) { m.OpenProfilerHomepage() },
		})
	}
	return actions
}

func profilerMissingText(message string, plan *openidea.ProfilePrepared) string {
	if plan != nil && plan.Install != nil {
		line := strings.TrimSpace(strings.Join(
			append([]string{plan.Install.Command}, plan.Install.Args...), " "))
		if line != "" {
			return message + " Install it with: " + line
		}
	}
	if plan != nil && plan.Homepage != nil && strings.TrimSpace(*plan.Homepage) != "" {
		return message + " " + strings.TrimSpace(*plan.Homepage)
	}
	return message
}

// OpenProfilerHomepage opens the missing tool's documented download page.
func (m *Model) OpenProfilerHomepage() bool {
	plan := m.profiler.plan
	if plan == nil || plan.Homepage == nil || strings.TrimSpace(*plan.Homepage) == "" {
		return false
	}
	openReleasePage(*plan.Homepage)
	return true
}

func (m *Model) finishProfiledRun() {
	active := m.profiler.activeRun
	if active == nil {
		return
	}
	if active.stopPending {
		active.exited = true
		return
	}
	m.profiler.activeRun = nil
	m.profiler.stopping = false
	notice := ""
	if active.mode == openidea.ProfileLiveObjects {
		if !active.stopped {
			notice = liveProfileNotStopped
		} else if active.gcCollected == 0 {
			notice = liveProfileNotCollected
		}
	}
	m.collectProfile(active, notice)
}

func (m *Model) stopProfiledRun(runID uint32) bool {
	active := m.profiler.activeRun
	if active == nil {
		return false
	}
	active.runID = runID
	active.stopPending = true
	m.profiler.stopping = true
	m.profiler.err = ""
	m.status = "Collecting live objects before stopping…"
	m.invalidate()
	go func() {
		var collected uint32
		var gcErr error
		if active.mode == openidea.ProfileLiveObjects {
			ctx, cancel := context.WithTimeout(m.backgroundContext(), 30*time.Second)
			raw, err := m.invoke(ctx, "profiler_gc_before_stop", map[string]any{"run": runID})
			cancel()
			if err == nil {
				err = json.Unmarshal(raw, &collected)
			}
			gcErr = err
		}
		ctx, cancel := context.WithTimeout(m.backgroundContext(), 15*time.Second)
		_, stopErr := m.invoke(ctx, "run_stop", map[string]any{"id": runID})
		cancel()
		m.post(func() {
			if m.profiler.activeRun != active {
				return
			}
			active.gcCollected = collected
			active.stopPending = false
			if stopErr != nil {
				m.profiler.stopping = false
				m.profiler.err = "Stopping the profiled run failed: " + stopErr.Error()
				m.run.err = m.profiler.err
			} else {
				active.stopped = true
				if gcErr != nil {
					m.profiler.err = "Live-object collection failed: " + gcErr.Error()
				}
			}
			m.invalidate()
			if active.exited {
				m.finishProfiledRun()
			}
		})
	}()
	return true
}

func (m *Model) collectProfile(active *profiledRun, notice string) {
	m.ensureProfiler()
	if active == nil || len(active.spec) == 0 {
		m.profiler.err = "The profiler returned no profile specification"
		m.invalidate()
		return
	}
	m.profiler.collecting = true
	m.profiler.err = ""
	m.profiler.notice = "Collecting the " + profileModeName(active.mode) + " profile…"
	m.status = m.profiler.notice
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.backgroundContext(), 5*time.Minute)
		defer cancel()
		raw, err := m.invoke(ctx, "profiler_collect", map[string]any{"spec": active.spec})
		var profile openidea.Profile
		if err == nil {
			err = json.Unmarshal(raw, &profile)
		}
		m.post(func() {
			m.profiler.collecting = false
			if err != nil {
				m.profiler.err = "Cannot collect the profile: " + err.Error()
				m.profiler.notice = ""
				m.notifyError("profiler-done", "No profile for '"+active.name+"'", err.Error())
				m.invalidate()
				return
			}
			m.addProfileSnapshot(active.name, profile, notice)
			m.profiler.notice = ""
			m.DockShow("profiler")
			m.invalidate()
		})
	}()
}

func (m *Model) addProfileSnapshot(name string, profile openidea.Profile, notice string) *profileSnapshot {
	m.ensureProfiler()
	m.profiler.nextID++
	snapshot := &profileSnapshot{
		id:         fmt.Sprintf("profile%d", m.profiler.nextID),
		name:       name,
		createdAt:  time.Now().UnixMilli(),
		profile:    profile,
		notice:     notice,
		tab:        "flame",
		focus:      0,
		expanded:   hottestProfilePath(profile, 30),
		methodSort: "self",
		treeSel:    -1,
		methodSel:  -1,
	}
	m.profiler.snapshots = append([]*profileSnapshot{snapshot}, m.profiler.snapshots...)
	if len(m.profiler.snapshots) > maxProfileSnapshots {
		m.profiler.snapshots = m.profiler.snapshots[:maxProfileSnapshots]
	}
	m.profiler.activeID = snapshot.id
	return snapshot
}

func (m *Model) activeProfileSnapshot() *profileSnapshot {
	m.ensureProfiler()
	for _, snapshot := range m.profiler.snapshots {
		if snapshot.id == m.profiler.activeID {
			return snapshot
		}
	}
	if len(m.profiler.snapshots) > 0 {
		m.profiler.activeID = m.profiler.snapshots[0].id
		return m.profiler.snapshots[0]
	}
	return nil
}

func (m *Model) SelectProfileSnapshot(id string) {
	for _, snapshot := range m.profiler.snapshots {
		if snapshot.id == id {
			m.profiler.activeID = id
			m.invalidate()
			return
		}
	}
}

func (m *Model) CloseProfileSnapshot(id string) {
	for i, snapshot := range m.profiler.snapshots {
		if snapshot.id != id {
			continue
		}
		m.profiler.snapshots = append(m.profiler.snapshots[:i], m.profiler.snapshots[i+1:]...)
		if m.profiler.activeID == id {
			m.profiler.activeID = ""
			if len(m.profiler.snapshots) > 0 {
				m.profiler.activeID = m.profiler.snapshots[0].id
			}
		}
		m.invalidate()
		return
	}
}

func (m *Model) resetProfiler() {
	m.profiler = profilerState{tab: "flame", mode: openidea.ProfileCPU}
}

func (m *Model) openProfileFrame(snapshot *profileSnapshot, frame int) {
	if snapshot == nil || frame < 0 || frame >= len(snapshot.profile.Frames) {
		return
	}
	item := snapshot.profile.Frames[frame]
	if item.File == nil || strings.TrimSpace(*item.File) == "" {
		return
	}
	line := 0
	if item.Line != nil {
		line = *item.Line
	}
	m.revealAt(*item.File, max(0, line), 0)
}

func profileModeName(mode openidea.ProfileMode) string {
	switch mode {
	case openidea.ProfileAllocations:
		return "allocation"
	case openidea.ProfileLiveObjects:
		return "live-object"
	default:
		return "CPU"
	}
}

func profilerSupports(config openidea.RunConfig) bool {
	program := profileProgramName(config.Command)
	switch program {
	case "node", "npx", "tsx", "ts-node", "npm", "pnpm", "yarn", "java":
		return true
	}
	if program == "cargo" {
		for _, arg := range config.Args {
			if !strings.HasPrefix(arg, "+") {
				return runtime.GOOS == "linux" && arg == "run"
			}
		}
	}
	if config.Kind == "native" {
		return runtime.GOOS == "linux"
	}
	if config.Kind == "python" || config.Kind == "pytest" {
		return true
	}
	if strings.HasPrefix(program, "python") || program == "py" || program == "pytest" || program == "py.test" {
		return true
	}
	return (program == "uv" || program == "poetry") && len(config.Args) > 0 && config.Args[0] == "run"
}

func allocationProfilerSupports(config openidea.RunConfig) bool {
	return profileProgramName(config.Command) == "java"
}

func profileProgramName(command string) string {
	name := filepath.Base(strings.ReplaceAll(command, `\`, "/"))
	lower := strings.ToLower(name)
	for _, ext := range []string{".exe", ".cmd", ".bat"} {
		lower = strings.TrimSuffix(lower, ext)
	}
	return lower
}

func hottestProfilePath(profile openidea.Profile, limit int) map[int]bool {
	expanded := make(map[int]bool)
	if len(profile.Nodes) == 0 {
		return expanded
	}
	roots := profile.Nodes[0].Children
	if len(roots) == 0 {
		return expanded
	}
	current := roots[0]
	for depth := 0; depth < limit && current >= 0 && current < len(profile.Nodes); depth++ {
		expanded[current] = true
		children := profile.Nodes[current].Children
		if len(children) == 0 {
			break
		}
		current = children[0]
	}
	return expanded
}
