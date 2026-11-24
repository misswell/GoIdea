package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"

	"mayoi/internal/adapter"
	"mayoi/internal/openidea"
)

func init() {
	_ = registerAction(&Action{
		ID:       "run.updateApplication",
		Title:    "Update Application",
		Category: "Run",
		Keywords: "update resources classes restart redeploy spring boot tomcat",
		When: func(m *Model) bool {
			session, ok := m.currentApplicationUpdateSession()
			return ok && session.config.Kind != ""
		},
		Enabled: func(m *Model) bool {
			session, ok := m.currentApplicationUpdateSession()
			return ok && !m.applicationUpdateBusy(session.config.ID)
		},
		Running: func(m *Model) bool {
			session, ok := m.currentApplicationUpdateSession()
			return ok && m.applicationUpdateBusy(session.config.ID)
		},
		RunWithArg: func(m *Model, arg any) {
			m.UpdateApplication(arg)
		},
	})
}

type applicationUpdateSession struct {
	config         openidea.RunConfig
	debugging      bool
	runID          uint32
	runChild       bool
	debugSessionID uint32
}

type applicationUpdateKey struct {
	model    *Model
	configID string
}

var applicationUpdateState = struct {
	sync.Mutex
	busy map[applicationUpdateKey]bool
}{busy: map[applicationUpdateKey]bool{}}

// applicationUpdateSlots bounds background work when a window is deactivated.
// The backend's build and update commands are independent, but a burst of
// project windows must not start an unbounded number of builds at once.
var applicationUpdateSlots = make(chan struct{}, 2)

func (m *Model) applicationUpdateBusy(configID string) bool {
	applicationUpdateState.Lock()
	defer applicationUpdateState.Unlock()
	return applicationUpdateState.busy[applicationUpdateKey{model: m, configID: configID}]
}

func (m *Model) beginApplicationUpdate(configID string) bool {
	applicationUpdateState.Lock()
	defer applicationUpdateState.Unlock()
	key := applicationUpdateKey{model: m, configID: configID}
	if applicationUpdateState.busy[key] {
		return false
	}
	applicationUpdateState.busy[key] = true
	return true
}

func (m *Model) endApplicationUpdate(configID string) {
	applicationUpdateState.Lock()
	defer applicationUpdateState.Unlock()
	delete(applicationUpdateState.busy, applicationUpdateKey{model: m, configID: configID})
}

func (m *Model) applicationUpdateSessions() []applicationUpdateSession {
	if m == nil {
		return nil
	}
	// The selected debug tab is the source of truth for its mutable fields.
	m.debug.rememberDebugSession()

	sessions := make([]applicationUpdateSession, 0, 3)
	seen := map[string]bool{}
	addRun := func(raw json.RawMessage, runID uint32, child bool, fallback openidea.RunConfig) {
		cfg, ok := m.applicationUpdateConfig(raw, fallback)
		if !ok || seen[cfg.ID] {
			return
		}
		seen[cfg.ID] = true
		sessions = append(sessions, applicationUpdateSession{
			config: cfg, runID: runID, runChild: child,
		})
	}

	if m.run.running {
		addRun(m.run.lastRaw, m.run.runID, false, openidea.RunConfig{})
	}
	for _, child := range m.run.children {
		if child != nil && child.running {
			addRun(child.config.Raw(), child.runID, true, child.config)
		}
	}
	for _, child := range m.services.children {
		if child != nil && child.running {
			addRun(child.config.Raw(), child.runID, true, child.config)
		}
	}

	debugged := map[string]uint32{}
	addDebug := func(id uint32) {
		state := m.debug.sessions[id]
		if state == nil || state.session == 0 || state.ended {
			return
		}
		cfg, ok := m.applicationUpdateConfig(state.lastRaw, openidea.RunConfig{})
		if !ok {
			return
		}
		debugged[cfg.ID] = id
		if seen[cfg.ID] {
			return
		}
		seen[cfg.ID] = true
		sessions = append(sessions, applicationUpdateSession{
			config: cfg, debugSessionID: id,
		})
	}
	for _, id := range m.debug.sessionOrder {
		addDebug(id)
	}
	// During the short launch window the active pane is not in sessionOrder
	// yet. It is still a valid Debug candidate once the adapter reported a
	// session id.
	if m.debug.active != 0 {
		addDebug(m.debug.active)
	}

	for i := range sessions {
		if id := debugged[sessions[i].config.ID]; id != 0 {
			sessions[i].debugging = true
			sessions[i].debugSessionID = id
		}
	}
	return sessions
}

func (m *Model) applicationUpdateConfig(
	raw json.RawMessage,
	fallback openidea.RunConfig,
) (openidea.RunConfig, bool) {
	if len(raw) > 0 {
		if cfg, ok := m.lastRunConfiguration(raw); ok && applicationConfigUpdatable(&cfg) {
			return cfg, true
		}
	}
	if applicationConfigUpdatable(&fallback) {
		return fallback, true
	}
	return openidea.RunConfig{}, false
}

func (m *Model) currentApplicationUpdateSession() (applicationUpdateSession, bool) {
	sessions := m.applicationUpdateSessions()
	if len(sessions) == 0 {
		return applicationUpdateSession{}, false
	}
	// Follow the active Run or Debug configuration, then fall back to the
	// first live application. Upstream does the same when no session id was
	// passed to showApplicationUpdate.
	for _, session := range sessions {
		if m.run.lastID != "" && session.config.ID == m.run.lastID {
			return session, true
		}
	}
	if m.debug.lastID != "" {
		for _, session := range sessions {
			if session.config.ID == m.debug.lastID {
				return session, true
			}
		}
	}
	return sessions[0], true
}

func (m *Model) lastRunConfiguration(raw json.RawMessage) (openidea.RunConfig, bool) {
	if len(raw) == 0 {
		return openidea.RunConfig{}, false
	}
	cfg, err := openidea.DecodeRunConfig(raw)
	if err != nil || cfg == nil {
		return openidea.RunConfig{}, false
	}
	for _, saved := range m.run.configs {
		if saved.ID == cfg.ID {
			return saved, true
		}
	}
	return *cfg, true
}

func applicationConfigUpdatable(cfg *openidea.RunConfig) bool {
	if cfg == nil || (cfg.Kind != "spring-boot" && cfg.Kind != "tomcat") {
		return false
	}
	return cfg.Target == nil || strings.TrimSpace(*cfg.Target) == ""
}

func applicationUpdateAllowed(cfg *openidea.RunConfig, mode string) bool {
	if !applicationConfigUpdatable(cfg) {
		return false
	}
	switch mode {
	case "resources", "classes", "restart":
	case "redeploy":
		if cfg.Kind != "tomcat" {
			return false
		}
	default:
		return false
	}
	if (mode == "resources" || mode == "classes") && cfg.Kind == "tomcat" {
		deployment, _ := cfg.Params["deployment"].(string)
		return !strings.EqualFold(deployment, "war")
	}
	return true
}

// UpdateApplication is run.updateApplication. With no explicit mode it uses
// the current configuration's onUpdate policy and otherwise offers the same
// choices as OpenIDEA's Update Application menu.
func (m *Model) UpdateApplication(arg any) bool {
	session, ok := m.applicationUpdateSessionArg(arg)
	if !ok {
		m.status = "No updatable application is running"
		m.invalidate()
		return false
	}
	mode := applicationUpdateModeArg(arg)
	if mode == "" {
		policy, _ := session.config.Params["onUpdate"].(string)
		if policy == "" {
			policy = "ask"
		}
		if policy != "ask" && applicationUpdateAllowed(&session.config, policy) {
			mode = policy
		} else {
			m.showApplicationUpdateChooser(session)
			return true
		}
	}
	if !applicationUpdateAllowed(&session.config, mode) {
		m.notifyInfo(
			"application.update",
			"Cannot update this application",
			fmt.Sprintf("%s is not available for %s.", mode, session.config.Kind),
		)
		return false
	}
	return m.startApplicationUpdate(session, mode)
}

func (m *Model) applicationUpdateSessionArg(arg any) (applicationUpdateSession, bool) {
	requested := ""
	if values, ok := arg.(map[string]any); ok {
		for _, key := range []string{"configId", "configID", "sessionId", "sessionID"} {
			if value, ok := values[key].(string); ok && strings.TrimSpace(value) != "" {
				requested = strings.TrimSpace(value)
				break
			}
		}
	}
	sessions := m.applicationUpdateSessions()
	if requested != "" {
		for _, session := range sessions {
			if session.config.ID == requested ||
				(requested == "debug:"+fmt.Sprint(session.debugSessionID) && session.debugSessionID != 0) ||
				(requested == "run:"+fmt.Sprint(session.runID) && session.runID != 0) {
				return session, true
			}
		}
	}
	return m.currentApplicationUpdateSession()
}

func applicationUpdateModeArg(arg any) string {
	switch value := arg.(type) {
	case string:
		return strings.TrimSpace(value)
	case map[string]any:
		mode, _ := value["mode"].(string)
		return strings.TrimSpace(mode)
	}
	return ""
}

func (m *Model) showApplicationUpdateChooser(session applicationUpdateSession) {
	choices := applicationUpdateChoices(session)
	actions := make([]notifyAction, 0, len(choices))
	for _, choice := range choices {
		choice := choice
		if !applicationUpdateAllowed(&session.config, choice.mode) {
			continue
		}
		actions = append(actions, notifyAction{
			label: choice.label,
			run:   func(m *Model) { m.startApplicationUpdate(session, choice.mode) },
		})
	}

	// MyGo's native menu is the same context-menu surface OpenIDEA uses for
	// the Update Application header. Tests and headless callers have no window;
	// keep the notification fallback for them so the action remains usable.
	if m.win != nil {
		mygo.NewMenu(applicationUpdateMenuItems(m, session, choices)).Popup(m.win)
		return
	}
	m.notifySticky(
		"application.update",
		"info",
		"Update Application",
		"Choose how to update "+session.config.Name+".",
		true,
		actions...,
	)
}

func applicationUpdateMenuItems(
	m *Model,
	session applicationUpdateSession,
	choices []struct {
		mode  string
		label string
	},
) []*mygo.MenuItem {
	items := make([]*mygo.MenuItem, 0, len(choices)+2)
	items = append(items,
		&mygo.MenuItem{Label: "Update Application", Disabled: true},
		mygo.Separator(),
	)
	for _, choice := range choices {
		choice := choice
		items = append(items, &mygo.MenuItem{
			Label: choice.label,
			Disabled: !applicationUpdateAllowed(&session.config, choice.mode) ||
				m.applicationUpdateBusy(session.config.ID),
			Click: func(_ *mygo.MenuItem, _ *mygo.Window) {
				m.startApplicationUpdate(session, choice.mode)
			},
		})
	}
	return items
}

func applicationUpdateChoices(session applicationUpdateSession) []struct {
	mode  string
	label string
} {
	choices := []struct {
		mode  string
		label string
	}{
		{"resources", "Update resources"},
		{"classes", "Update classes and resources"},
	}
	if session.config.Kind == "tomcat" {
		choices = append(choices, struct {
			mode  string
			label string
		}{"redeploy", "Redeploy"})
	}
	restart := "Restart"
	if session.config.Kind == "tomcat" {
		restart = "Restart server"
	}
	choices = append(choices, struct {
		mode  string
		label string
	}{"restart", restart})
	return choices
}

func (m *Model) startApplicationUpdate(session applicationUpdateSession, mode string) bool {
	if !m.beginApplicationUpdate(session.config.ID) {
		return false
	}
	root := m.Root()
	raw := append(json.RawMessage(nil), session.config.Raw()...)
	if len(raw) == 0 {
		var err error
		raw, err = json.Marshal(session.config)
		if err != nil {
			m.endApplicationUpdate(session.config.ID)
			return false
		}
	}
	go func() {
		defer m.endApplicationUpdate(session.config.ID)
		applicationUpdateSlots <- struct{}{}
		defer func() { <-applicationUpdateSlots }()
		hotSwapped := false
		var err error
		if mode == "restart" {
			err = m.restartApplication(root, session)
		} else {
			hotSwapped, err = m.updateApplicationFiles(raw, session.debugging, mode)
		}
		m.post(func() {
			if m.Root() != root {
				return
			}
			if err != nil {
				m.run.err = "Application update failed: " + err.Error()
				m.notifyError("application.update", "Application update failed", err.Error())
				m.invalidate()
				return
			}
			if session.config.Kind == "spring-boot" {
				if hotSwapped {
					m.invalidate()
					return
				}
				body := ""
				if mode == "classes" {
					body = "Classes were compiled. Restart the application to load them, or launch with the debugger to use HotSwap."
				}
				m.notifyInfo(
					"application.update",
					"Resources updated",
					body,
				)
			} else {
				m.notifyInfo("application.update", "Application updated", "")
			}
			m.invalidate()
		})
	}()
	return true
}

func (m *Model) updateApplicationFiles(
	raw json.RawMessage,
	debugging bool,
	mode string,
) (bool, error) {
	cfg, err := openidea.DecodeRunConfig(raw)
	if err != nil {
		return false, err
	}
	ctx, cancel := context.WithTimeout(m.backgroundContext(), 10*time.Minute)
	defer cancel()
	if err := m.saveOpenTabs(ctx, append([]*Tab(nil), m.tabs...)); err != nil {
		return false, fmt.Errorf("save before update: %w", err)
	}
	switch cfg.Kind {
	case "tomcat":
		return false, m.updateTomcatApplication(ctx, cfg, raw, debugging, mode)
	case "spring-boot":
		return m.updateSpringBootApplication(ctx, cfg, raw, debugging, mode)
	default:
		return false, fmt.Errorf("%s is not an updatable Spring Boot configuration", cfg.Kind)
	}
}

func (m *Model) updateSpringBootApplication(
	ctx context.Context,
	cfg *openidea.RunConfig,
	raw json.RawMessage,
	debugging bool,
	mode string,
) (bool, error) {
	parsed := adapter.ParseJavaArgs(cfg.Args)
	if parsed.MainClass == "" {
		return false, errors.New("The Spring Boot configuration has no main class")
	}
	module, err := adapter.ResolveJavaModule(ctx, javaBackend{m}, cfg)
	if err != nil {
		return false, err
	}
	policy := "noBuild"
	if mode == "classes" {
		policy = "make"
	}
	rawInfo, err := m.invoke(ctx, "java_resolve_launch", map[string]any{
		"module": module, "test": parsed.Classpath == "test",
		"policy": policy, "request": m.nextRequest(),
	})
	if err != nil {
		return false, err
	}
	var info openidea.JavaLaunchInfo
	if err := json.Unmarshal(rawInfo, &info); err != nil {
		return false, fmt.Errorf("read java_resolve_launch: %w", err)
	}
	if info.Build != nil &&
		(*info.Build == "failed" || *info.Build == "withErrors" || *info.Build == "cancelled") {
		return false, errors.New("Application update build failed")
	}
	rawBuild, err := m.invoke(ctx, "run_application_build", map[string]any{
		"config": json.RawMessage(raw), "module": module,
	})
	if err != nil {
		return false, err
	}
	build, err := openidea.DecodeRunConfig(rawBuild)
	if err != nil || build == nil {
		if err == nil {
			err = errors.New("the update build returned no configuration")
		}
		return false, fmt.Errorf("read update build: %w", err)
	}
	if err := m.runApplicationUpdateBuild(ctx, cfg.ID, build); err != nil {
		return false, errors.New("Application update build failed")
	}
	classPaths := info.ClassPaths
	if parsed.ExplicitClasspath != "" {
		separator := info.PathListSeparator
		if separator == "" {
			separator = string(os.PathListSeparator)
		}
		classPaths = strings.Split(parsed.ExplicitClasspath, separator)
	}
	if _, err := m.invoke(ctx, "run_application_publish", map[string]any{
		"config": json.RawMessage(raw), "classPaths": classPaths, "mainClass": parsed.MainClass,
	}); err != nil {
		return false, err
	}
	if mode == "classes" && debugging {
		// The HotSwap action is scoped to this configuration and completes
		// before the update worker reports success. A switch while the build is
		// running cannot turn into a reload of another debug session.
		return m.HotSwapReloadForConfig(cfg.ID), nil
	}
	return false, nil
}

type tomcatWebArtifact struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Module string `json:"module"`
	Tool   string `json:"tool"`
}

func (m *Model) updateTomcatApplication(
	ctx context.Context,
	cfg *openidea.RunConfig,
	raw json.RawMessage,
	debugging bool,
	mode string,
) error {
	if mode != "redeploy" {
		synced, err := m.syncTomcatOutputs(ctx, cfg, mode == "resources")
		if err != nil {
			return err
		}
		if synced {
			if mode == "classes" && debugging && !m.HotSwapReloadForConfig(cfg.ID) {
				if err := m.runTomcatUpdate(ctx, raw, "redeploy"); err != nil {
					return err
				}
			}
			return nil
		}
	}

	buildRaw, err := m.invoke(ctx, "run_tomcat_build", map[string]any{
		"config": json.RawMessage(raw),
	})
	if err != nil {
		return err
	}
	var build *openidea.RunConfig
	if strings.TrimSpace(string(buildRaw)) != "" && string(buildRaw) != "null" {
		build, err = openidea.DecodeRunConfig(buildRaw)
		if err != nil {
			return fmt.Errorf("read Tomcat build: %w", err)
		}
	}
	if build != nil {
		if err := m.runApplicationUpdateBuild(ctx, cfg.ID, build); err != nil {
			return err
		}
	}
	if err := m.runTomcatUpdate(ctx, raw, mode); err != nil {
		return err
	}
	if mode == "classes" && debugging && !m.HotSwapReloadForConfig(cfg.ID) {
		return m.runTomcatUpdate(ctx, raw, "redeploy")
	}
	return nil
}

func (m *Model) syncTomcatOutputs(
	ctx context.Context,
	cfg *openidea.RunConfig,
	resourcesOnly bool,
) (bool, error) {
	if cfg == nil {
		return false, errors.New("the Tomcat configuration is missing")
	}
	if deployment, _ := cfg.Params["deployment"].(string); deployment != "" && deployment != "artifact" {
		return false, nil
	}
	rawArtifacts, err := m.invoke(ctx, "run_tomcat_artifacts", nil)
	if err != nil {
		return false, err
	}
	var artifacts []tomcatWebArtifact
	if err := json.Unmarshal(rawArtifacts, &artifacts); err != nil {
		return false, fmt.Errorf("read Tomcat artifacts: %w", err)
	}
	if len(artifacts) == 0 {
		return false, nil
	}
	artifactID, _ := cfg.Params["artifact"].(string)
	artifact := artifacts[0]
	if artifactID != "" {
		found := false
		for _, candidate := range artifacts {
			if candidate.ID == artifactID {
				artifact = candidate
				found = true
				break
			}
		}
		if !found {
			return false, nil
		}
	}

	rawInfo, err := m.invoke(ctx, "java_resolve_launch", map[string]any{
		"module": artifact.Module, "test": false, "policy": "make",
		"request": m.nextRequest(),
	})
	if err != nil {
		// The upstream Tomcat sync treats a failed Java resolution as
		// unavailable and falls back to rebuilding the artifact.
		return false, nil
	}
	var info openidea.JavaLaunchInfo
	if err := json.Unmarshal(rawInfo, &info); err != nil {
		return false, fmt.Errorf("read Tomcat Java resolution: %w", err)
	}
	if info.Build != nil &&
		(*info.Build == "failed" || *info.Build == "withErrors" || *info.Build == "cancelled") {
		return false, errors.New("Application update build failed")
	}
	rawSynced, err := m.invoke(ctx, "run_tomcat_sync", map[string]any{
		"config": json.RawMessage(cfg.Raw()), "module": artifact.Module,
		"classPaths": info.ClassPaths, "resourcesOnly": resourcesOnly,
	})
	if err != nil {
		return false, err
	}
	var synced bool
	if err := json.Unmarshal(rawSynced, &synced); err != nil {
		return false, fmt.Errorf("read Tomcat sync: %w", err)
	}
	return synced, nil
}

func (m *Model) runTomcatUpdate(ctx context.Context, raw json.RawMessage, mode string) error {
	_, err := m.invoke(ctx, "run_tomcat_update", map[string]any{
		"config": json.RawMessage(raw), "mode": mode,
	})
	return err
}

// updateApplicationsOnDeactivation applies each live application's
// onDeactivation policy. It is intentionally separate from UpdateApplication:
// policies never show the chooser and only resources/classes are valid.
func (m *Model) updateApplicationsOnDeactivation() {
	for _, session := range m.applicationUpdateSessions() {
		mode, _ := session.config.Params["onDeactivation"].(string)
		if mode != "resources" && mode != "classes" {
			continue
		}
		if !applicationUpdateAllowed(&session.config, mode) {
			continue
		}
		m.startApplicationUpdate(session, mode)
	}
}

// runApplicationUpdateBuild uses an independent Services-owned child so the
// build cannot replace the application's primary RunPane lifecycle.
func (m *Model) runApplicationUpdateBuild(
	ctx context.Context,
	configID string,
	build *openidea.RunConfig,
) error {
	child, err := m.startServiceChildRun(ctx, build, "application-update:"+configID)
	if err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		m.stopRunChild(child)
		return ctx.Err()
	case err := <-child.done:
		if err != nil {
			return err
		}
		if child.exitCode == nil || *child.exitCode != 0 {
			return errors.New("Application update build failed")
		}
		return nil
	}
}

func (m *Model) restartApplication(root string, session applicationUpdateSession) error {
	latest := session.config
	raw := append(json.RawMessage(nil), latest.Raw()...)
	if len(raw) == 0 {
		var err error
		raw, err = json.Marshal(latest)
		if err != nil {
			return err
		}
	}
	if session.runChild {
		var child *runChild
		m.post(func() { child = m.childByRun(session.runID) })
		if child == nil {
			return errors.New("the run session is no longer available")
		}
		if session.debugSessionID != 0 {
			return m.restartApplicationRunAndDebug(root, raw, session, child)
		}
		return m.restartApplicationChild(root, raw, latest.Name, child)
	}
	if session.debugSessionID != 0 && session.runID != 0 {
		return m.restartApplicationRunAndDebug(root, raw, session, nil)
	}
	if session.debugSessionID != 0 {
		return m.restartApplicationDebug(root, raw, session.debugSessionID)
	}
	if session.debugging {
		m.post(func() {
			m.StopRun()
			m.StopDebug()
		})
		if !m.waitApplicationUpdate(root, func() bool {
			debugStopped := !m.debug.starting && (m.debug.session == 0 || m.debug.ended)
			return debugStopped && !m.run.running && !m.run.awaiting && !m.run.busy
		}) {
			return errors.New("the run and debug sessions did not stop before restart")
		}
		var started bool
		m.post(func() {
			if m.Root() == root {
				cfg, err := openidea.DecodeRunConfig(raw)
				if err == nil {
					started = m.debugRunConfig(cfg, true)
				}
			}
		})
		if !started {
			return errors.New("the debug configuration could not restart")
		}
		return nil
	}

	m.post(func() { m.StopRun() })
	if !m.waitApplicationUpdate(root, func() bool {
		return !m.run.running && !m.run.awaiting && !m.run.busy
	}) {
		return errors.New("the run session did not stop before restart")
	}
	var started bool
	m.post(func() {
		if m.Root() == root {
			started = m.startRunWith(raw, latest.Name)
		}
	})
	if !started {
		return errors.New("the run configuration could not restart")
	}
	return nil
}

func (m *Model) restartApplicationRunAndDebug(
	root string,
	raw json.RawMessage,
	session applicationUpdateSession,
	child *runChild,
) error {
	m.post(func() {
		if child != nil {
			m.stopRunChild(child)
		} else if session.runID != 0 {
			m.StopRun()
		}
		m.selectDebugSession(session.debugSessionID)
		if m.debug.active == session.debugSessionID {
			m.StopDebug()
		}
	})
	if !m.waitApplicationUpdate(root, func() bool {
		runStopped := true
		if child != nil {
			runStopped = child.finished
		} else if session.runID != 0 {
			runStopped = !m.run.running && !m.run.awaiting && !m.run.busy
		}
		state := m.debug.sessions[session.debugSessionID]
		debugStopped := !m.debug.starting &&
			(state == nil || state.ended || state.session == 0)
		return runStopped && debugStopped
	}) {
		return errors.New("the run and debug sessions did not stop before restart")
	}
	var started bool
	m.post(func() {
		if m.Root() != root {
			return
		}
		cfg, err := openidea.DecodeRunConfig(raw)
		if err == nil {
			started = m.debugRunConfig(cfg, true)
		}
	})
	if !started {
		return errors.New("the debug configuration could not restart")
	}
	return nil
}

func (m *Model) restartApplicationChild(
	root string,
	raw json.RawMessage,
	name string,
	child *runChild,
) error {
	if child == nil {
		return errors.New("the run session is no longer available")
	}
	service := child.service
	groupID := child.groupID
	if groupID == "" {
		groupID = "application-update:" + child.config.ID
	}
	m.post(func() { m.stopRunChild(child) })
	if !m.waitApplicationUpdate(root, func() bool { return child.finished }) {
		return errors.New("the run session did not stop before restart")
	}
	cfg, err := openidea.DecodeRunConfig(raw)
	if err != nil {
		return err
	}
	var started *runChild
	if service {
		started, err = m.startServiceChildRun(m.ctx, cfg, groupID)
	} else {
		started, err = m.startChildRun(m.ctx, cfg, groupID, false)
	}
	if err != nil || started == nil {
		if err == nil {
			err = errors.New("the run configuration could not restart")
		}
		return err
	}
	if !m.waitApplicationUpdate(root, func() bool {
		return started.running || started.err != ""
	}) {
		return errors.New("the run configuration did not start before the restart timeout")
	}
	if started.err != "" {
		return fmt.Errorf("%s did not start: %s", name, started.err)
	}
	return nil
}

func (m *Model) restartApplicationDebug(
	root string,
	raw json.RawMessage,
	sessionID uint32,
) error {
	var found bool
	m.post(func() { found = m.debug.sessions[sessionID] != nil })
	if !found {
		return errors.New("the debug session is no longer available")
	}
	m.post(func() {
		m.selectDebugSession(sessionID)
		if m.debug.active == sessionID {
			m.StopDebug()
		}
	})
	if !m.waitApplicationUpdate(root, func() bool {
		state := m.debug.sessions[sessionID]
		return state == nil || state.ended || state.session == 0
	}) {
		return errors.New("the debug session did not stop before restart")
	}
	var started bool
	m.post(func() {
		if m.Root() != root {
			return
		}
		cfg, err := openidea.DecodeRunConfig(raw)
		if err == nil {
			started = m.debugRunConfig(cfg, true)
		}
	})
	if !started {
		return errors.New("the debug configuration could not restart")
	}
	return nil
}

func (m *Model) waitApplicationUpdate(root string, done func() bool) bool {
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if err := m.ctx.Err(); err != nil {
			return false
		}
		var finished bool
		var cancelled bool
		m.post(func() {
			if m.Root() != root {
				cancelled = true
				return
			}
			finished = done()
		})
		if finished {
			return true
		}
		if cancelled {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}
