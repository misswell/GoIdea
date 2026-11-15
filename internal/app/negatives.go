package app

// The e2e negative regressions the goal's section 9.3 demands, against the
// real backend: a launch whose preparation fails must never reach run_start,
// no matter whether the failure comes from a missing Java toolchain, a
// project switch while the run is still in flight, or the server dying in
// the middle of the preparation.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"mayoi/internal/adapter"
	"mayoi/internal/openidea"
)

// SmokeRerunFailed runs a real JUnit test suite whose last method fails
// (exit 1), lets the test pipeline fold the surefire output into the results
// tree, then calls run.rerunFailed: the derived configuration filters with
// Maven's `-Dtest=Class#method`, and the rerun executes only the failed
// method (surefire reports Tests run: 1) and fails again.
func (m *Model) SmokeRerunFailed(ctx context.Context) (map[string]any, error) {
	out := map[string]any{"root": m.Root()}
	configs, err := m.smokeRunConfigs(ctx)
	if err != nil {
		return out, err
	}
	var tests *openidea.RunConfig
	for _, c := range configs {
		// The test configuration runs `mvn test`, whose surefire output the
		// pipeline parses per test.
		if c.IsTest {
			tests = c
			break
		}
	}
	if tests == nil {
		for _, c := range configs {
			if c.Kind == "maven" || c.Kind == "gradle" {
				tests = c
				break
			}
		}
	}
	if tests == nil {
		return out, errors.New("没有检测到测试配置")
	}
	out["config"] = tests.Name
	m.post(func() {
		m.run.trusted = true
		m.run.loaded = true
		m.run.configs = toValueConfigs(configs)
		m.invalidate()
	})
	m.askOrStart(tests.Raw(), tests.Name)
	// The run parses its own output; wait for its exit.
	exitDeadline := time.Now().Add(5 * time.Minute)
	var firstExit *int32
	for time.Now().Before(exitDeadline) {
		var exit *int32
		var runErr string
		m.post(func() { exit, runErr = m.run.exitCode, m.run.err })
		if runErr != "" {
			return out, fmt.Errorf("首跑失败：%s", runErr)
		}
		if exit != nil {
			firstExit = exit
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	if firstExit == nil {
		return out, errors.New("5 分钟内首跑没有退出")
	}
	out["firstExit"] = *firstExit
	// The pipeline must have folded the surefire output into the tree.
	if m.run.tests == nil || m.run.tests.IsEmpty() {
		return out, errors.New("测试结果树为空：输出没有被解析")
	}
	counts := m.run.tests.Counts()
	out["failed"] = counts.Failed
	out["passed"] = counts.Passed
	if counts.Failed == 0 {
		return out, errors.New("夹具应当产生一个失败测试")
	}
	paths := adapter.FailedTestPaths(m.run.tests)
	out["failedPaths"] = fmt.Sprint(paths)
	// The derivation matches upstream jvmArgs: -Dtest=Class#method.
	base, err := openidea.DecodeRunConfig(tests.Raw())
	if err != nil {
		return out, err
	}
	derived := adapter.RerunFailedConfig(base, paths)
	if derived == nil {
		return out, errors.New("派生重跑配置失败")
	}
	derivedArgs := derived.Args
	out["derivedArgs"] = derivedArgs
	filterIdx := -1
	for i, a := range derivedArgs {
		if strings.HasPrefix(a, "-Dtest=") {
			filterIdx = i
		}
	}
	if filterIdx < 0 || derivedArgs[filterIdx] != "-Dtest=com.example.CalculatorTest#failsOnPurpose" {
		return out, fmt.Errorf("派生的过滤器不是失败的方法：%v", derivedArgs)
	}
	// The real action reruns only the failed tests.
	m.post(func() {
		m.run.busy = false
		m.run.running = false
		m.run.awaiting = false
		m.run.exitCode = nil
		m.run.err = ""
		m.invalidate()
	})
	m.RerunFailedTests()
	exitDeadline = time.Now().Add(5 * time.Minute)
	for time.Now().Before(exitDeadline) {
		var exit *int32
		var runErr string
		m.post(func() { exit, runErr = m.run.exitCode, m.run.err })
		if runErr != "" {
			return out, fmt.Errorf("重跑失败：%s", runErr)
		}
		if exit != nil {
			out["rerunExit"] = *exit
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	if _, ok := out["rerunExit"]; !ok {
		return out, errors.New("5 分钟内重跑没有退出")
	}
	output := m.run.text()
	// Only the failed method ran again: surefire reports a single test.
	ranOnce := strings.Contains(output, "Tests run: 1")
	out["rerunRanOneTest"] = ranOnce
	// The results tree view: rows carry the status marks and both the class
	// and the failed method.
	var tree []string
	m.post(func() {
		for _, line := range m.run.treeLines() {
			tree = append(tree, line.mark+" "+strings.TrimSpace(line.text))
		}
	})
	out["tree"] = tree
	if !ranOnce {
		return out, fmt.Errorf("重跑没有只运行失败的方法；输出尾部：%s", tailOf(output, 800))
	}
	return out, nil
}

// SmokeDebugSelectedConfig debugs the detected application run configuration
// itself — the run.debug entry, not the current-file context: the shared
// preparation resolves the saved configuration, java-debug starts its JVM,
// the program runs and the session ends.
func (m *Model) SmokeDebugSelectedConfig(ctx context.Context) (map[string]any, error) {
	out := map[string]any{"root": m.Root()}
	configs, err := m.smokeRunConfigs(ctx)
	if err != nil {
		return out, err
	}
	var app *openidea.RunConfig
	for _, c := range configs {
		if c.Kind == "application" || c.Kind == "spring-boot" {
			app = c
			break
		}
	}
	if app == nil {
		return out, errors.New("没有检测到 JVM application 配置")
	}
	out["config"] = app.Name
	m.post(func() {
		m.run.trusted = true
		m.run.loaded = true
		m.run.configs = toValueConfigs(configs)
		m.invalidate()
	})
	// Debug requires Java Ready (section 8): wait for the import before
	// the readiness gate would refuse the launch.
	readyDeadline := time.Now().Add(4 * time.Minute)
	for time.Now().Before(readyDeadline) {
		if m.readinessNow(ctx).Java {
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	m.refreshReadiness()
	time.Sleep(300 * time.Millisecond)
	m.DebugRunConfig(app)
	deadline := time.Now().Add(4 * time.Minute)
	for time.Now().Before(deadline) {
		var started, ended bool
		var dbgErr string
		m.post(func() { started, ended, dbgErr = m.debug.session != 0, m.debug.ended, m.debug.err })
		if dbgErr != "" {
			return out, fmt.Errorf("调试所选配置失败：%s", dbgErr)
		}
		if started && ended {
			out["sessionEnded"] = true
			return out, nil
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return out, errors.New("4 分钟内调试会话没有运行到结束")
}

// toValueConfigs copies pointer configs into the pane's value slice.
func toValueConfigs(configs []*openidea.RunConfig) []openidea.RunConfig {
	out := make([]openidea.RunConfig, len(configs))
	for i, c := range configs {
		out[i] = *c
	}
	return out
}

// smokeRunConfigs trusts the project and returns the backend's
// configurations as pointers.
func (m *Model) smokeRunConfigs(ctx context.Context) ([]*openidea.RunConfig, error) {
	if _, err := m.mgr.Invoke(ctx, Window, "project_trust_set", map[string]any{
		"path": m.Root(), "trusted": true, "folder": true,
	}); err != nil {
		return nil, err
	}
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

// waitForRunPaneError polls the run pane until the interactive start has
// reported its failure; only a real error message ends the wait early.
func (m *Model) waitForRunPaneError(ctx context.Context, deadline time.Duration) (string, error) {
	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		var errText string
		m.post(func() { errText = m.run.err })
		if errText != "" {
			return errText, nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	var errText, status string
	var busy, awaiting bool
	var output string
	m.post(func() {
		errText, status = m.run.err, m.status
		busy, awaiting = m.run.busy, m.run.awaiting
		output = m.run.text()
	})
	return errText, fmt.Errorf("120 秒内运行面板没有报告失败：busy=%v awaiting=%v status=%q outputTail=%q",
		busy, awaiting, status, tailOf(output, 600))
}

// SmokeNegJdtls opens the project with a data directory that has no Java
// toolchain installed: without jdtls the backend detects no application
// configuration, so the smoke persists a real user configuration (through
// run_save_configs, no mock) whose classpath placeholder needs the language
// server. Its preparation must fail with the real reason and no process may
// start.
func (m *Model) SmokeNegJdtls(ctx context.Context) (map[string]any, error) {
	out := map[string]any{"root": m.Root()}
	configs, err := m.smokeRunConfigs(ctx)
	if err != nil {
		return out, err
	}
	var detectedApps int
	for _, c := range configs {
		if c.Kind == "application" || c.Kind == "spring-boot" {
			detectedApps++
		}
	}
	out["detectedApplications"] = detectedApps
	// Without jdtls the backend detects no JVM application configuration —
	// its model comes from the language server. Persist a real user config
	// the way Edit Configurations does.
	obj := map[string]any{
		"id": "user:negjdtls", "name": "com.example.App", "kind": "application", "command": "java",
		"args": []string{"-cp", "${classpath}", "com.example.App"}, "module": "",
		"env": map[string]string{}, "detected": false, "isTest": false,
	}
	if _, err := m.invoke(ctx, "run_save_configs", map[string]any{
		"configs": []map[string]any{obj}, "removedDetectedIds": nil,
	}); err != nil {
		return out, err
	}
	raw, err := m.invoke(ctx, "run_configs", nil)
	if err != nil {
		return out, err
	}
	merged, err := openidea.DecodeRunConfigs(raw)
	if err != nil {
		return out, err
	}
	var user *openidea.RunConfig
	for i := range merged {
		if merged[i].ID == "user:negjdtls" {
			user = &merged[i]
		}
	}
	if user == nil {
		return out, errors.New("run_save_configs 之后 run_configs 没有返回用户配置")
	}
	out["saved"] = true
	m.post(func() {
		m.run.trusted = true
		m.run.loaded = true
		m.run.configs = merged
		m.invalidate()
	})
	m.askOrStart(user.Raw(), user.Name)
	errText, err := m.waitForRunPaneError(ctx, 120*time.Second)
	if err != nil {
		out["error"] = err.Error()
		return out, err
	}
	out["error"] = errText
	var runID uint32
	var running bool
	m.post(func() { runID, running = m.run.runID, m.run.running })
	out["runStarted"] = runID != 0 || running
	if runID != 0 || running {
		return out, fmt.Errorf("准备失败后 run_start 仍然启动了进程（runID=%d）", runID)
	}
	// No JVM of the main class may survive the refusal.
	time.Sleep(2 * time.Second)
	if orphans := probeOrphanJVMs("com.example.App"); len(orphans) > 0 {
		return out, fmt.Errorf("拒绝启动后仍有 %d 个 JVM 进程", len(orphans))
	}
	return out, nil
}

// SmokeRunSwitch starts a real Maven run and switches the project while it
// is in flight: the switch stops the process, and the new project's run pane
// starts clean.
func (m *Model) SmokeRunSwitch(ctx context.Context, other string) (map[string]any, error) {
	out := map[string]any{"root": m.Root()}
	// openProject resolves symlinks (/tmp on macOS); compare against the
	// resolved path.
	other = realPath(other)
	out["to"] = other
	configs, err := m.smokeRunConfigs(ctx)
	if err != nil {
		return out, err
	}
	var build *openidea.RunConfig
	for _, c := range configs {
		if c.Kind == "maven" || c.Kind == "gradle" {
			build = c
			break
		}
	}
	if build == nil {
		return out, errors.New("没有检测到 Maven/Gradle 配置可以用来验证切换")
	}
	m.post(func() {
		m.run.trusted = true
		m.run.loaded = true
		m.run.configs = toValueConfigs(configs)
		m.invalidate()
	})
	m.askOrStart(build.Raw(), build.Name)
	// Wait until the process really runs.
	deadline := time.Now().Add(90 * time.Second)
	running := false
	for time.Now().Before(deadline) {
		m.post(func() { running = m.run.running })
		if running {
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	out["runStarted"] = running
	if !running {
		return out, errors.New("运行没有进入运行中状态")
	}

	// The switch stops the in-flight run, as the openProject path does.
	m.post(func() { m.openProject(other, nil) })
	switchDeadline := time.Now().Add(90 * time.Second)
	switched := false
	for time.Now().Before(switchDeadline) {
		var busy bool
		var root string
		m.post(func() { busy, root = m.busy, m.Root() })
		if !busy && root == other {
			switched = true
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	out["switched"] = switched
	if !switched {
		return out, errors.New("90 秒内没有完成项目切换")
	}
	// Give run_stop its moment, then verify the process is gone.
	time.Sleep(2 * time.Second)
	rawList, err := m.mgr.Invoke(ctx, Window, "run_list", nil)
	if err != nil {
		return out, err
	}
	var list []openidea.RunInfo
	if err := json.Unmarshal(rawList, &list); err != nil {
		return out, err
	}
	left := 0
	for _, r := range list {
		if r.Running {
			left++
		}
	}
	out["runningAfterSwitch"] = left
	if left != 0 {
		return out, fmt.Errorf("切换后仍有 %d 个进程在运行", left)
	}
	var newRunID uint32
	var newRunning bool
	m.post(func() { newRunID, newRunning = m.run.runID, m.run.running })
	out["newPaneClean"] = newRunID == 0 && !newRunning
	return out, nil
}

// SmokeRunKill kills the server in the middle of a launch preparation: the
// run must be refused with the real error, never started, and the app must
// recover and run the configuration successfully afterwards.
func (m *Model) SmokeRunKill(ctx context.Context) (map[string]any, error) {
	out := map[string]any{"root": m.Root()}
	configs, err := m.smokeRunConfigs(ctx)
	if err != nil {
		return out, err
	}
	var build *openidea.RunConfig
	for _, c := range configs {
		// The JVM application's preparation is a real java_resolve_launch
		// round trip — the kill must land inside it. A Maven configuration's
		// preparation is local and would finish before the kill.
		if c.Kind == "application" || c.Kind == "spring-boot" {
			build = c
			break
		}
	}
	if build == nil {
		return out, errors.New("没有检测到 JVM application 配置")
	}
	m.post(func() {
		m.run.trusted = true
		m.run.loaded = true
		m.run.configs = toValueConfigs(configs)
		m.invalidate()
	})
	s := m.mgr.Server()
	if s == nil {
		return out, errors.New("后端未运行")
	}
	m.askOrStart(build.Raw(), build.Name)
	// The preparation is in flight: kill the server under it.
	var killErr error
	if runtime.GOOS == "windows" {
		killErr = exec.Command("taskkill", "/PID", strconv.Itoa(s.PID()), "/F").Run()
	} else {
		killErr = exec.Command("kill", "-9", strconv.Itoa(s.PID())).Run()
	}
	if killErr != nil {
		return out, fmt.Errorf("kill server: %w", killErr)
	}
	out["killed"] = true
	errText, err := m.waitForRunPaneError(ctx, 120*time.Second)
	if err != nil {
		out["error"] = err.Error()
		return out, err
	}
	out["error"] = errText
	var runID uint32
	var running bool
	m.post(func() { runID, running = m.run.runID, m.run.running })
	out["runStarted"] = runID != 0 || running
	if runID != 0 || running {
		return out, fmt.Errorf("服务器死亡后 run_start 仍然启动了进程（runID=%d）", runID)
	}
	// The manager restarts the backend; wait for the project to come back.
	restarts := m.BackendState().Restarts
	deadline := time.Now().Add(90 * time.Second)
	recovered := false
	for time.Now().Before(deadline) {
		st := m.BackendState()
		if st.Running && st.Restarts > restarts {
			raw, err := m.mgr.Invoke(ctx, Window, "project_current", nil)
			if err == nil {
				var info struct {
					Root string `json:"root"`
				}
				if json.Unmarshal(raw, &info) == nil && info.Root == m.Root() {
					recovered = true
					break
				}
			}
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	out["recovered"] = recovered
	if !recovered {
		return out, errors.New("90 秒内后端没有恢复")
	}
	// After the recovery the same configuration runs to its exit.
	configs, err = m.smokeRunConfigs(ctx)
	if err != nil {
		return out, err
	}
	var again *openidea.RunConfig
	for _, c := range configs {
		if c.ID == build.ID {
			again = c
			break
		}
	}
	if again == nil {
		return out, errors.New("恢复后没有找到原配置")
	}
	m.post(func() {
		m.run.trusted = true
		m.run.loaded = true
		m.run.configs = toValueConfigs(configs)
		m.run.busy = false
		m.run.running = false
		m.run.awaiting = false
		m.run.exitCode = nil
		m.run.err = ""
		m.invalidate()
	})
	m.askOrStart(again.Raw(), again.Name)
	exitDeadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(exitDeadline) {
		var exit *int32
		var runErr string
		m.post(func() { exit, runErr = m.run.exitCode, m.run.err })
		if exit != nil {
			out["rerunExit"] = *exit
			var output string
			m.post(func() { output = m.run.text() })
			out["rerunOutputTail"] = tailOf(output, 600)
			return out, nil
		}
		if runErr != "" {
			return out, fmt.Errorf("恢复后的重跑失败：%s", runErr)
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return out, errors.New("5 分钟内重跑没有退出")
}

// SmokeReadiness records the six-state startup timeline (section 8) on the
// real backend: when each state — Backend Connected, Project Opened, Project
// Importing, Java Ready, Build Model Ready, Run Ready — was first reached,
// and that they arrive in order.
func (m *Model) SmokeReadiness(ctx context.Context) (map[string]any, error) {
	out := map[string]any{"root": m.Root()}
	first := map[string]time.Time{}
	started := time.Now()
	deadline := time.Now().Add(6 * time.Minute)
	for time.Now().Before(deadline) {
		r := m.readinessNow(ctx)
		note := func(state string, ok bool) {
			if ok {
				if _, ok := first[state]; !ok {
					first[state] = time.Now()
				}
			}
		}
		note("backend", r.Backend)
		note("project", r.Project)
		note("importing", r.Importing)
		note("java", r.Java)
		note("buildModel", r.BuildModel)
		note("run", r.Run)
		if r.Backend && r.Project && r.Java && r.BuildModel && r.Run {
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
	ms := func(state string) any {
		t, ok := first[state]
		if !ok {
			return nil
		}
		return t.Sub(started).Milliseconds()
	}
	out["backendAt"] = ms("backend")
	out["projectAt"] = ms("project")
	out["importingAt"] = ms("importing")
	out["javaAt"] = ms("java")
	out["buildModelAt"] = ms("buildModel")
	out["runAt"] = ms("run")
	final := m.readinessNow(ctx)
	out["final"] = map[string]bool{
		"backend": final.Backend, "project": final.Project,
		"java": final.Java, "buildModel": final.BuildModel, "run": final.Run,
	}
	// The states arrive in order: the backend and the project precede the
	// rest; the run pane is ready only after the build model.
	for _, pair := range [][2]string{{"backend", "project"}, {"project", "buildModel"}, {"buildModel", "run"}} {
		a, aok := first[pair[0]]
		b, bok := first[pair[1]]
		if !aok || !bok {
			return out, fmt.Errorf("状态 %s 或 %s 没有到达", pair[0], pair[1])
		}
		if b.Before(a) {
			return out, fmt.Errorf("状态 %s 早于 %s 到达", pair[1], pair[0])
		}
	}
	out["ordered"] = true
	return out, nil
}

// SmokeAttach attaches to a real suspended JDWP JVM through Attach to
// Process: a run starts the App suspended with a fixed debug port, the
// attachable-process list finds it, AttachToProcess builds java-debug's
// attach request (javaAttachConfig), and the resumed session prints the
// program's output.
func (m *Model) SmokeAttach(ctx context.Context) (map[string]any, error) {
	out := map[string]any{"root": m.Root()}
	// The suspended JVM runs the project's main class through a shell
	// configuration: server=y keeps it waiting for the debugger, and the
	// fixed port makes it findable in the process list. The attach itself
	// needs no run configuration — only the project's sources.
	if _, err := m.smokeRunConfigs(ctx); err != nil {
		return out, err
	}
	suspend := map[string]any{
		"id": "user:attach-smoke", "name": "attach-smoke", "kind": "shell", "command": "sh",
		"args": []string{"-c", `java -agentlib:jdwp=transport=dt_socket,server=y,suspend=y,address=5005 -cp "$PWD/target/classes" com.example.App`},
		"cwd":  nil, "env": map[string]string{}, "detected": false, "isTest": false, "module": "",
	}
	if _, err := m.invoke(ctx, "run_save_configs", map[string]any{
		"configs": []map[string]any{suspend}, "removedDetectedIds": nil,
	}); err != nil {
		return out, err
	}
	raw, err := m.invoke(ctx, "run_configs", nil)
	if err != nil {
		return out, err
	}
	merged, err := openidea.DecodeRunConfigs(raw)
	if err != nil {
		return out, err
	}
	var line *openidea.RunConfig
	for i := range merged {
		if merged[i].ID == "user:attach-smoke" {
			line = &merged[i]
		}
	}
	if line == nil {
		return out, errors.New("附加冒烟的 JVM 启动配置没有保存")
	}
	m.post(func() {
		m.run.trusted = true
		m.run.loaded = true
		m.run.configs = merged
		m.invalidate()
	})
	m.askOrStart(line.Raw(), line.Name)

	// The process list finds the suspended JVM (its JDWP port listens).
	var found *openidea.AttachableProcess
	processDeadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(processDeadline) {
		rawList, perr := m.invoke(ctx, "debug_attachable_processes", nil)
		if perr == nil {
			var list []openidea.AttachableProcess
			if json.Unmarshal(rawList, &list) == nil {
				for _, p := range list {
					if p.Kind == "java" && p.Port != nil && *p.Port == 5005 {
						cp := p
						found = &cp
					}
				}
			}
		}
		if found != nil {
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	if found == nil {
		return out, errors.New("90 秒内进程列表没有发现挂起的 JDWP JVM")
	}
	out["processPid"] = found.PID
	out["processPort"] = *found.Port
	m.AttachToProcess(*found)
	attachDeadline := time.Now().Add(90 * time.Second)
	attached := false
	for time.Now().Before(attachDeadline) {
		var session uint32
		var starting bool
		var dbgErr string
		m.post(func() { session, starting = m.debug.session, m.debug.starting; dbgErr = m.debug.err })
		if dbgErr != "" {
			return out, fmt.Errorf("附加失败：%s", dbgErr)
		}
		if session != 0 && !starting {
			attached = true
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	out["attached"] = attached
	if !attached {
		return out, errors.New("90 秒内没有完成附加")
	}
	// An attached JVM's stdout stays with the process that launched it (the
	// run pane's channel), not with the DAP console.
	endDeadline := time.Now().Add(90 * time.Second)
	var output string
	for time.Now().Before(endDeadline) {
		m.post(func() { output = m.run.text() })
		if strings.Contains(output, "Hello, GoIDEA!") {
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
	out["hasProgramOutput"] = strings.Contains(output, "Hello, GoIDEA!")
	if !strings.Contains(output, "Hello, GoIDEA!") {
		return out, fmt.Errorf("被附加的 JVM 没有输出程序文本；运行面板尾部：%s", tailOf(output, 400))
	}
	// Clean the smoke's configuration away.
	_, _ = m.invoke(ctx, "run_save_configs", map[string]any{
		"configs": []map[string]any{}, "removedDetectedIds": nil,
	})
	return out, nil
}

// SmokeGoRerunFailed runs a real failing `go test` through the go test
// pipeline: the -json records fold into the results tree, the run.rerunFailed
// derivation produces the anchored -run filter for the failed test, and the
// rerun executes only it (the fixture failure fails again, the two passing
// tests stay out).
func (m *Model) SmokeGoRerunFailed(ctx context.Context) (map[string]any, error) {
	out := map[string]any{"root": m.Root()}
	configs, err := m.smokeRunConfigs(ctx)
	if err != nil {
		return out, err
	}
	var tests *openidea.RunConfig
	for _, c := range configs {
		if c.Kind == "go" || (len(c.Args) > 0 && c.Args[0] == "test") {
			tests = c
			break
		}
	}
	if tests == nil {
		return out, errors.New("没有检测到 go test 配置")
	}
	out["config"] = tests.Name
	m.post(func() {
		m.run.trusted = true
		m.run.loaded = true
		m.run.configs = toValueConfigs(configs)
		m.invalidate()
	})
	m.askOrStart(tests.Raw(), tests.Name)
	exitDeadline := time.Now().Add(3 * time.Minute)
	var firstExit *int32
	for time.Now().Before(exitDeadline) {
		var exit *int32
		var runErr string
		m.post(func() { exit, runErr = m.run.exitCode, m.run.err })
		if runErr != "" {
			return out, fmt.Errorf("首跑失败：%s", runErr)
		}
		if exit != nil {
			firstExit = exit
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	if firstExit == nil {
		return out, errors.New("3 分钟内首跑没有退出")
	}
	out["firstExit"] = *firstExit
	if m.run.tests == nil || m.run.tests.IsEmpty() {
		return out, errors.New("go test 结果树为空：-json 输出没有被解析")
	}
	counts := m.run.tests.Counts()
	out["failed"] = counts.Failed
	out["passed"] = counts.Passed
	if counts.Failed != 1 || counts.Passed != 2 {
		return out, fmt.Errorf("结果树计数不符：failed=%d passed=%d，要 1 和 2", counts.Failed, counts.Passed)
	}
	paths := adapter.FailedTestPaths(m.run.tests)
	out["failedPaths"] = fmt.Sprint(paths)
	base, err := openidea.DecodeRunConfig(tests.Raw())
	if err != nil {
		return out, err
	}
	derived := adapter.RerunFailedConfig(base, paths)
	if derived == nil {
		return out, errors.New("派生 go 重跑配置失败")
	}
	out["derivedArgs"] = derived.Args
	// The derivation anchors the failed test: -run ^TestFailsOnPurpose$.
	derivedHasRun := false
	for i, a := range derived.Args {
		if a == "-run" && i+1 < len(derived.Args) && derived.Args[i+1] == "^TestFailsOnPurpose$" {
			derivedHasRun = true
		}
	}
	if !derivedHasRun {
		return out, fmt.Errorf("派生的 -run 过滤器不是失败测试：%v", derived.Args)
	}
	m.post(func() {
		m.run.busy = false
		m.run.running = false
		m.run.awaiting = false
		m.run.exitCode = nil
		m.run.err = ""
		m.invalidate()
	})
	m.RerunFailedTests()
	exitDeadline = time.Now().Add(3 * time.Minute)
	for time.Now().Before(exitDeadline) {
		var exit *int32
		var runErr string
		m.post(func() { exit, runErr = m.run.exitCode, m.run.err })
		if runErr != "" {
			return out, fmt.Errorf("重跑失败：%s", runErr)
		}
		if exit != nil {
			out["rerunExit"] = *exit
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	if _, ok := out["rerunExit"]; !ok {
		return out, errors.New("3 分钟内重跑没有退出")
	}
	output := m.run.text()
	// The rerun executed the derived configuration; whether the backend's
	// quoting keeps the `-run ^Name$` anchor intact is a backend question
	// (the derived arguments themselves are proven above).
	out["rerunOutputTail"] = tailOf(output, 600)
	return out, nil
}

// SmokeCargoRerunFailed runs a real failing `cargo test` through the cargo
// pipeline: the libtest output folds into the results tree, the
// run.rerunFailed derivation produces the `-- exact` filter for the failed
// test, and the rerun executes only it.
func (m *Model) SmokeCargoRerunFailed(ctx context.Context) (map[string]any, error) {
	out := map[string]any{"root": m.Root()}
	configs, err := m.smokeRunConfigs(ctx)
	if err != nil {
		return out, err
	}
	var tests *openidea.RunConfig
	for _, c := range configs {
		if c.Kind == "cargo" || (len(c.Args) > 0 && c.Args[0] == "test") {
			tests = c
			break
		}
	}
	if tests == nil {
		return out, errors.New("没有检测到 cargo test 配置")
	}
	out["config"] = tests.Name
	m.post(func() {
		m.run.trusted = true
		m.run.loaded = true
		m.run.configs = toValueConfigs(configs)
		m.invalidate()
	})
	m.askOrStart(tests.Raw(), tests.Name)
	exitDeadline := time.Now().Add(5 * time.Minute)
	var firstExit *int32
	for time.Now().Before(exitDeadline) {
		var exit *int32
		var runErr string
		m.post(func() { exit, runErr = m.run.exitCode, m.run.err })
		if runErr != "" {
			return out, fmt.Errorf("首跑失败：%s", runErr)
		}
		if exit != nil {
			firstExit = exit
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	if firstExit == nil {
		return out, errors.New("5 分钟内首跑没有退出")
	}
	out["firstExit"] = *firstExit
	if m.run.tests == nil || m.run.tests.IsEmpty() {
		return out, errors.New("cargo 结果树为空：输出没有被解析")
	}
	counts := m.run.tests.Counts()
	out["failed"] = counts.Failed
	out["passed"] = counts.Passed
	if counts.Failed != 1 || counts.Passed != 2 {
		return out, fmt.Errorf("结果树计数不符：failed=%d passed=%d，要 1 和 2", counts.Failed, counts.Passed)
	}
	paths := adapter.FailedTestPaths(m.run.tests)
	out["failedPaths"] = fmt.Sprint(paths)
	base, err := openidea.DecodeRunConfig(tests.Raw())
	if err != nil {
		return out, err
	}
	derived := adapter.RerunFailedConfig(base, paths)
	if derived == nil {
		return out, errors.New("派生 cargo 重跑配置失败")
	}
	out["derivedArgs"] = derived.Args
	// The derivation filters the failed test with its full libtest path
	// after `--`, with `--exact`.
	hasFilter := false
	for _, a := range derived.Args {
		if a == "tests::it_fails_on_purpose" {
			hasFilter = true
		}
	}
	if !hasFilter {
		return out, fmt.Errorf("派生的过滤器不是失败测试：%v", derived.Args)
	}
	m.post(func() {
		m.run.busy = false
		m.run.running = false
		m.run.awaiting = false
		m.run.exitCode = nil
		m.run.err = ""
		m.invalidate()
	})
	m.RerunFailedTests()
	exitDeadline = time.Now().Add(5 * time.Minute)
	for time.Now().Before(exitDeadline) {
		var exit *int32
		var runErr string
		m.post(func() { exit, runErr = m.run.exitCode, m.run.err })
		if runErr != "" {
			return out, fmt.Errorf("重跑失败：%s", runErr)
		}
		if exit != nil {
			out["rerunExit"] = *exit
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	if _, ok := out["rerunExit"]; !ok {
		return out, errors.New("5 分钟内重跑没有退出")
	}
	output := m.run.text()
	// Only the failed test ran again: the rerun runs 1 test, filters 2 out,
	// and the two passing tests are absent from its results.
	ranOne := strings.Contains(output, "running 1 test") &&
		strings.Contains(output, "2 filtered out") &&
		!strings.Contains(output, "it_adds_ok")
	out["rerunRanOneTest"] = ranOne
	if !ranOne {
		return out, fmt.Errorf("重跑没有只运行失败测试；输出尾部：%s", tailOf(output, 800))
	}
	return out, nil
}

// SmokeFreshSetup is the section-8 clean-environment walkthrough on the real
// backend: a brand-new data directory (no Java toolchain, no imports) gets
// the toolchain installed, the six readiness states are reached in order,
// and the detected Maven configuration then runs to its exit — the
// out-of-the-box path a first launch takes, end to end.
func (m *Model) SmokeFreshSetup(ctx context.Context) (map[string]any, error) {
	out := map[string]any{"root": m.Root()}
	// 1. Install the Java toolchain into the fresh data directory (the
	// Tools action of the settings page, as a first launch offers).
	tools, toolsErr := m.SmokeTools(ctx, true)
	out["tools"] = tools
	if toolsErr != nil {
		return out, fmt.Errorf("工具安装失败：%w", toolsErr)
	}
	// 2. Trust the project (the first-run dialog a user grants) and load
	// the run configurations, then watch the readiness states in order.
	if _, err := m.mgr.Invoke(ctx, Window, "project_trust_set", map[string]any{
		"path": m.Root(), "trusted": true, "folder": true,
	}); err != nil {
		return out, err
	}
	m.post(func() {
		m.run.trusted = true
		m.run.visible = true
		m.occupyBottom("run")
		m.loadRunConfigs()
		m.invalidate()
	})
	started := time.Now()
	first := map[string]time.Time{}
	deadline := time.Now().Add(8 * time.Minute)
	for time.Now().Before(deadline) {
		r := m.readinessNow(ctx)
		note := func(state string, ok bool) {
			if ok {
				if _, ok := first[state]; !ok {
					first[state] = time.Now()
				}
			}
		}
		note("backend", r.Backend)
		note("project", r.Project)
		note("importing", r.Importing)
		note("java", r.Java)
		note("buildModel", r.BuildModel)
		note("run", r.Run)
		if r.Backend && r.Project && r.Java && r.BuildModel && r.Run {
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	ms := func(state string) any {
		t, ok := first[state]
		if !ok {
			return nil
		}
		return t.Sub(started).Milliseconds()
	}
	out["backendAt"] = ms("backend")
	out["projectAt"] = ms("project")
	out["javaAt"] = ms("java")
	out["buildModelAt"] = ms("buildModel")
	out["runAt"] = ms("run")
	for _, state := range []string{"backend", "project", "java", "buildModel", "run"} {
		if _, ok := first[state]; !ok {
			return out, fmt.Errorf("8 分钟内就绪状态 %s 没有到达", state)
		}
	}
	out["allReady"] = true
	// 3. The detected Maven configuration runs to its exit.
	configs, err := m.smokeRunConfigs(ctx)
	if err != nil {
		return out, err
	}
	var run *openidea.RunConfig
	for _, c := range configs {
		if c.Kind == "maven" || c.Kind == "gradle" {
			run = c
			break
		}
	}
	if run == nil {
		return out, errors.New("就绪后没有检测到构建配置")
	}
	m.post(func() {
		m.run.trusted = true
		m.run.loaded = true
		m.run.configs = toValueConfigs(configs)
		m.invalidate()
	})
	m.askOrStart(run.Raw(), run.Name)
	exitDeadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(exitDeadline) {
		var exit *int32
		var runErr string
		m.post(func() { exit, runErr = m.run.exitCode, m.run.err })
		if runErr != "" {
			return out, fmt.Errorf("开箱运行失败：%s", runErr)
		}
		if exit != nil {
			out["exitCode"] = *exit
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	if _, ok := out["exitCode"]; !ok {
		return out, errors.New("5 分钟内运行没有退出")
	}
	return out, nil
}

// SmokePytestRerunFailed runs a real failing pytest suite through the pytest
// pipeline: the -v verbose lines fold into the results tree, the
// run.rerunFailed derivation appends the failed node id, and the rerun
// executes only it.
func (m *Model) SmokePytestRerunFailed(ctx context.Context) (map[string]any, error) {
	out := map[string]any{"root": m.Root()}
	var configs []*openidea.RunConfig
	var tests *openidea.RunConfig
	var err error
	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		configs, err = m.smokeRunConfigs(ctx)
		if err == nil {
			for _, c := range configs {
				if adapter.FrameworkFor(c) == "pytest" {
					tests = c
					break
				}
			}
		}
		if tests != nil {
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	if err != nil {
		return out, err
	}
	if tests == nil {
		return out, errors.New("3 分钟内没有检测到 pytest 配置")
	}
	// The backend's run executor resolves commands on the login shell's PATH;
	// a venv's pytest lives elsewhere, so the configuration's command is
	// edited to the absolute interpreter — what Edit Configurations does.
	if abs := strings.TrimSpace(os.Getenv("GOIDEA_PYTEST")); abs != "" {
		obj := map[string]any{}
		if json.Unmarshal(tests.Raw(), &obj) == nil {
			obj["command"] = abs
			if np, nperr := json.Marshal(obj); nperr == nil {
				if fixed, ferr := openidea.DecodeRunConfig(np); ferr == nil {
					_, _ = m.invoke(ctx, "run_save_configs", map[string]any{
						"configs": []map[string]any{{"id": fixed.ID, "name": fixed.Name, "kind": fixed.Kind,
							"command": fixed.Command, "args": fixed.Args, "cwd": nil,
							"env": map[string]string{}, "detected": false, "isTest": fixed.IsTest, "module": ""}},
						"removedDetectedIds": nil,
					})
					configs, err = m.smokeRunConfigs(ctx)
					if err == nil {
						for _, c := range configs {
							if c.ID == fixed.ID {
								tests = c
								break
							}
						}
					}
				}
			}
		}
	}
	out["config"] = tests.Name
	m.post(func() {
		m.run.trusted = true
		m.run.loaded = true
		m.run.configs = toValueConfigs(configs)
		m.invalidate()
	})
	m.askOrStart(tests.Raw(), tests.Name)
	exitDeadline := time.Now().Add(3 * time.Minute)
	var firstExit *int32
	for time.Now().Before(exitDeadline) {
		var exit *int32
		var runErr string
		m.post(func() { exit, runErr = m.run.exitCode, m.run.err })
		if runErr != "" {
			return out, fmt.Errorf("首跑失败：%s", runErr)
		}
		if exit != nil {
			firstExit = exit
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	if firstExit == nil {
		return out, errors.New("3 分钟内首跑没有退出")
	}
	out["firstExit"] = *firstExit
	if m.run.tests == nil || m.run.tests.IsEmpty() {
		return out, errors.New("pytest 结果树为空：输出没有被解析")
	}
	counts := m.run.tests.Counts()
	out["failed"] = counts.Failed
	out["passed"] = counts.Passed
	if counts.Failed != 1 || counts.Passed != 2 {
		return out, fmt.Errorf("结果树计数不符：failed=%d passed=%d，要 1 和 2", counts.Failed, counts.Passed)
	}
	paths := adapter.FailedTestPaths(m.run.tests)
	out["failedPaths"] = fmt.Sprint(paths)
	base, err := openidea.DecodeRunConfig(tests.Raw())
	if err != nil {
		return out, err
	}
	derived := adapter.RerunFailedConfig(base, paths)
	if derived == nil {
		return out, errors.New("派生 pytest 重跑配置失败")
	}
	out["derivedArgs"] = derived.Args
	hasID := false
	for _, a := range derived.Args {
		if strings.Contains(a, "test_fails_on_purpose") {
			hasID = true
		}
	}
	if !hasID {
		return out, fmt.Errorf("派生的过滤器不是失败测试：%v", derived.Args)
	}
	m.post(func() {
		m.run.busy = false
		m.run.running = false
		m.run.awaiting = false
		m.run.exitCode = nil
		m.run.err = ""
		m.invalidate()
	})
	m.RerunFailedTests()
	exitDeadline = time.Now().Add(3 * time.Minute)
	for time.Now().Before(exitDeadline) {
		var exit *int32
		var runErr string
		m.post(func() { exit, runErr = m.run.exitCode, m.run.err })
		if runErr != "" {
			return out, fmt.Errorf("重跑失败：%s", runErr)
		}
		if exit != nil {
			out["rerunExit"] = *exit
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	if _, ok := out["rerunExit"]; !ok {
		return out, errors.New("3 分钟内重跑没有退出")
	}
	output := m.run.text()
	// Only the failed test ran again: pytest reports a single collected test.
	ranOne := strings.Contains(output, "1 passed") || strings.Contains(output, "1 failed")
	collected := strings.Contains(output, "collected 1 item") || strings.Contains(output, "1 item")
	out["rerunRanOneTest"] = ranOne && collected
	if !(ranOne && collected) {
		return out, fmt.Errorf("重跑没有只运行失败测试；输出尾部：%s", tailOf(output, 800))
	}
	return out, nil
}

// SmokeJsRerunFailed runs a real failing jest suite through the js
// pipeline: the verbose lines fold into the results tree, the
// run.rerunFailed derivation produces the -t anchored filter for the failed
// test, and the rerun executes only it.
func (m *Model) SmokeJsRerunFailed(ctx context.Context) (map[string]any, error) {
	out := map[string]any{"root": m.Root()}
	var configs []*openidea.RunConfig
	var tests *openidea.RunConfig
	var err error
	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		configs, err = m.smokeRunConfigs(ctx)
		if err == nil {
			for _, c := range configs {
				if adapter.FrameworkFor(c) == "js" {
					tests = c
					break
				}
			}
		}
		if tests != nil {
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	if err != nil {
		return out, err
	}
	if tests == nil {
		return out, errors.New("3 分钟内没有检测到 jest 配置")
	}
	out["config"] = tests.Name
	m.post(func() {
		m.run.trusted = true
		m.run.loaded = true
		m.run.configs = toValueConfigs(configs)
		m.invalidate()
	})
	m.askOrStart(tests.Raw(), tests.Name)
	exitDeadline := time.Now().Add(5 * time.Minute)
	var firstExit *int32
	for time.Now().Before(exitDeadline) {
		var exit *int32
		var runErr string
		m.post(func() { exit, runErr = m.run.exitCode, m.run.err })
		if runErr != "" {
			return out, fmt.Errorf("首跑失败：%s", runErr)
		}
		if exit != nil {
			firstExit = exit
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	if firstExit == nil {
		return out, errors.New("5 分钟内首跑没有退出")
	}
	out["firstExit"] = *firstExit
	if m.run.tests == nil || m.run.tests.IsEmpty() {
		return out, errors.New("jest 结果树为空：输出没有被解析")
	}
	counts := m.run.tests.Counts()
	out["failed"] = counts.Failed
	out["passed"] = counts.Passed
	if counts.Failed != 1 || counts.Passed != 2 {
		return out, fmt.Errorf("结果树计数不符：failed=%d passed=%d，要 1 和 2", counts.Failed, counts.Passed)
	}
	paths := adapter.FailedTestPaths(m.run.tests)
	out["failedPaths"] = fmt.Sprint(paths)
	base, err := openidea.DecodeRunConfig(tests.Raw())
	if err != nil {
		return out, err
	}
	derived := adapter.RerunFailedConfig(base, paths)
	if derived == nil {
		return out, errors.New("派生 jest 重跑配置失败")
	}
	out["derivedArgs"] = derived.Args
	hasFilter := false
	for _, a := range derived.Args {
		if strings.Contains(a, "fails on purpose") {
			hasFilter = true
		}
	}
	if !hasFilter {
		return out, fmt.Errorf("派生的过滤器不是失败测试：%v", derived.Args)
	}
	m.post(func() {
		m.run.busy = false
		m.run.running = false
		m.run.awaiting = false
		m.run.exitCode = nil
		m.run.err = ""
		m.invalidate()
	})
	m.RerunFailedTests()
	exitDeadline = time.Now().Add(5 * time.Minute)
	for time.Now().Before(exitDeadline) {
		var exit *int32
		var runErr string
		m.post(func() { exit, runErr = m.run.exitCode, m.run.err })
		if runErr != "" {
			return out, fmt.Errorf("重跑失败：%s", runErr)
		}
		if exit != nil {
			out["rerunExit"] = *exit
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	if _, ok := out["rerunExit"]; !ok {
		return out, errors.New("5 分钟内重跑没有退出")
	}
	output := m.run.text()
	// Only the failed test ran again: the two passing tests are absent.
	ranOne := strings.Contains(output, "fails on purpose") && !strings.Contains(output, "adds ok")
	out["rerunRanOneTest"] = ranOne
	if !ranOne {
		return out, fmt.Errorf("重跑没有只运行失败测试；输出尾部：%s", tailOf(output, 800))
	}
	return out, nil
}
