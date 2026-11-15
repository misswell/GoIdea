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
