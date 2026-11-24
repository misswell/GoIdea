package app

// Maven and Gradle debugging follows
// src/features/run/javaDebug.ts. These build tools launch the debuggee in a
// forked JVM, so GoIDEA starts the real run through run_start, watches its
// bounded output for the JDWP banner, then attaches java-debug over RPC.

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"mayoi/internal/adapter"
	"mayoi/internal/openidea"
)

const (
	debugJDWPAgent          = "-agentlib:jdwp=transport=dt_socket,server=y,suspend=y,address=127.0.0.1:0"
	debugAttachListener     = "-XX:+StartAttachListener"
	debugGradleRunFlag      = "-Dopenidea.run.debug=true"
	debugGradleListenerFlag = "-Dopenidea.run.attachListener=true"
	debugGradleBootScriptID = "goidea-gradle-debug-boot"
)

//go:embed debug_gradle_boot.gradle
var debugGradleBootScript string

var (
	debugJDWPListen = regexp.MustCompile(
		`Listening for transport dt_socket at address:\s*(?:[\w.\[\]:-]*:)?(\d+)\s*$`,
	)
	debugMavenInProcessGoal = regexp.MustCompile(
		`^[\w.-]+(?::[\w.-]+)*:(?:run|run-war|run-exploded|java)$`,
	)
	debugMavenTestGoal = regexp.MustCompile(
		`^(?:test|verify|integration-test|surefire:test|failsafe:integration-test)$`,
	)
)

type debugBuildToolState struct {
	root       string
	name       string
	module     string
	channel    uint32
	pinnedPort int
	queued     map[int]bool
}

func debugIsMaven(config *openidea.RunConfig) bool {
	if config == nil {
		return false
	}
	name := strings.ToLower(filepath.Base(config.Command))
	return config.Kind == "maven" || name == "mvn" || name == "mvnw" || name == "mvnw.cmd"
}

func debugIsGradle(config *openidea.RunConfig) bool {
	if config == nil {
		return false
	}
	name := strings.ToLower(filepath.Base(config.Command))
	return config.Kind == "gradle" || name == "gradle" || name == "gradlew" || name == "gradlew.bat"
}

func debugBuildToolKind(config *openidea.RunConfig) string {
	switch {
	case debugIsMaven(config):
		return "maven"
	case debugIsGradle(config):
		return "gradle"
	default:
		return ""
	}
}

func debugHasOption(args []string, key string) bool {
	prefix := "-D" + key + "="
	for _, arg := range args {
		if strings.HasPrefix(arg, prefix) {
			return true
		}
	}
	return false
}

func debugPropertyValue(args []string, key string) string {
	prefix := "-D" + key + "="
	for _, arg := range args {
		if strings.HasPrefix(arg, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(arg, prefix))
		}
	}
	return ""
}

func debugWithProperty(args []string, key string, value string) []string {
	prefix := "-D" + key + "="
	out := append([]string(nil), args...)
	for i, arg := range out {
		if strings.HasPrefix(arg, prefix) {
			current := strings.TrimSpace(strings.TrimPrefix(arg, prefix))
			if current != "" {
				value = current + " " + value
			}
			out[i] = prefix + value
			return out
		}
	}
	return append(out, prefix+value)
}

func debugWithEnvOption(env map[string]string, key string, value string) map[string]string {
	out := make(map[string]string, len(env)+1)
	for name, old := range env {
		out[name] = old
	}
	if current := strings.TrimSpace(out[key]); current != "" {
		value = current + " " + value
	}
	out[key] = value
	return out
}

func debugOptsOutOfAttach(text string) bool {
	return strings.Contains(text, "StartAttachListener") ||
		strings.Contains(text, "DisableAttachMechanism")
}

// debugAttachListenerArgs opens the debuggee's attach socket so the Memory and
// Heap views can reach the JVM. A VM option in the configuration or launch
// decides the attach mechanism, so it is left alone.
func debugAttachListenerArgs(config *openidea.RunConfig, existing ...string) []string {
	options := append([]string(nil), existing...)
	if config != nil && config.Params != nil {
		if vmOptions := debugParamString(config.Params, "vmOptions", ""); vmOptions != "" {
			options = append(options, vmOptions)
		}
	}
	if runtime.GOOS == "windows" || debugOptsOutOfAttach(strings.Join(options, " ")) {
		return nil
	}
	return []string{debugAttachListener}
}

func debugAgentOptions(config *openidea.RunConfig, existing ...string) string {
	options := make([]string, 0, len(existing)+2)
	if config.Params != nil {
		if vmOptions := debugParamString(config.Params, "vmOptions", ""); vmOptions != "" {
			options = append(options, vmOptions)
		}
	}
	for _, value := range existing {
		if text := strings.TrimSpace(value); text != "" {
			options = append(options, text)
		}
	}
	joined := strings.Join(options, " ")
	if runtime.GOOS == "windows" || debugOptsOutOfAttach(joined) {
		return debugJDWPAgent
	}
	return debugAttachListener + " " + debugJDWPAgent
}

func debugPinJDWPPort(agent string, port int) string {
	if port < 1 || port > 65535 {
		return agent
	}
	return strings.ReplaceAll(agent, "address=127.0.0.1:0", fmt.Sprintf("address=127.0.0.1:%d", port))
}

func debugMavenGoals(args []string) []string {
	var goals []string
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		goals = append(goals, arg)
	}
	return goals
}

func debugMavenIsTest(config *openidea.RunConfig) bool {
	if config.IsTest {
		return true
	}
	for _, goal := range debugMavenGoals(config.Args) {
		if debugMavenTestGoal.MatchString(goal) {
			return true
		}
	}
	return false
}

func debugMavenIsSpringBootRun(args []string) bool {
	for _, goal := range args {
		if goal == "spring-boot:run" ||
			strings.HasSuffix(goal, ":spring-boot-maven-plugin:run") {
			return true
		}
	}
	return false
}

func debugMavenIsInProcess(config *openidea.RunConfig) bool {
	if config.DebugAdapter != nil && (*config.DebugAdapter == "java-debug" ||
		*config.DebugAdapter == "kotlin-debug-adapter") {
		return true
	}
	for _, goal := range debugMavenGoals(config.Args) {
		if debugMavenInProcessGoal.MatchString(goal) {
			return true
		}
	}
	return false
}

func (m *Model) debugMavenLaunch(
	config openidea.RunConfig,
	pinnedPort int,
) (openidea.RunConfig, error) {
	rewritten := adapter.ApplyBuildPolicyToCommand(&config)
	if rewritten != nil {
		config = *rewritten
	}
	if debugMavenIsSpringBootRun(config.Args) {
		existing := debugPropertyValue(config.Args, "spring-boot.run.jvmArguments")
		agent := debugPinJDWPPort(debugAgentOptions(&config, existing), pinnedPort)
		config.Args = debugWithProperty(config.Args, "spring-boot.run.jvmArguments", agent)
		return config, nil
	}
	if debugMavenIsTest(&config) {
		agent := debugPinJDWPPort(debugAgentOptions(&config, strings.Join(config.Args, " ")), pinnedPort)
		config.Args = debugWithProperty(config.Args, "maven.surefire.debug", agent)
		failsafe := false
		for _, arg := range config.Args {
			if strings.Contains(arg, "integration-test") || strings.Contains(arg, "verify") ||
				strings.Contains(arg, "failsafe") {
				failsafe = true
				break
			}
		}
		if failsafe {
			config.Args = debugWithProperty(config.Args, "maven.failsafe.debug", agent)
		}
		if !debugHasOption(config.Args, "maven.surefire.debug") &&
			!debugHasOption(config.Args, "maven.failsafe.debug") {
			return config, fmt.Errorf("the Maven test configuration has no Surefire or Failsafe debug hook")
		}
		return config, nil
	}
	if debugMavenIsInProcess(&config) {
		agent := debugPinJDWPPort(debugAgentOptions(&config), pinnedPort)
		config.Env = debugWithEnvOption(config.Env, "MAVEN_OPTS", agent)
		return config, nil
	}
	return config, fmt.Errorf("this Maven configuration does not start a debuggable JVM")
}

func debugGradleShortTask(arg string) string {
	if strings.HasPrefix(arg, "-") {
		return ""
	}
	if at := strings.LastIndex(arg, ":"); at >= 0 {
		return arg[at+1:]
	}
	return arg
}

func debugGradleHasTask(args []string, names ...string) bool {
	wanted := map[string]bool{}
	for _, name := range names {
		wanted[name] = true
	}
	for _, arg := range args {
		if wanted[debugGradleShortTask(arg)] {
			return true
		}
	}
	return false
}

func debugWithInitScript(args []string, script string) []string {
	if script == "" || hasInitScript(args, script) {
		return args
	}
	return append([]string{"--init-script", script}, args...)
}

func (m *Model) debugGradleInitScript(
	ctx context.Context,
	name string,
	text string,
) (string, error) {
	raw, err := m.invoke(ctx, "run_gradle_init_script", map[string]any{
		"name": name, "text": text,
	})
	if err != nil {
		return "", err
	}
	var path string
	if err := json.Unmarshal(raw, &path); err != nil {
		return "", err
	}
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("the backend returned an empty Gradle init-script path")
	}
	return path, nil
}

func (m *Model) debugGradleLaunch(
	ctx context.Context,
	config openidea.RunConfig,
) (openidea.RunConfig, error) {
	rewritten := adapter.ApplyBuildPolicyToCommand(&config)
	if rewritten != nil {
		config = *rewritten
	}
	if debugGradleHasTask(config.Args, "bootRun") {
		script, err := m.debugGradleInitScript(ctx, debugGradleBootScriptID, debugGradleBootScript)
		if err != nil {
			return config, err
		}
		config.Args = debugWithInitScript(config.Args, script)
		config.Args = append(config.Args, debugGradleRunFlag)
		if !debugOptsOutOfAttach(strings.Join(config.Args, " ")) {
			config.Args = append(config.Args, debugGradleListenerFlag)
		}
		return config, nil
	}
	if debugGradleHasTask(config.Args, "test", "integrationTest") {
		config.IsTest = true
		config.Args = append(config.Args, debugGradleRunFlag)
		if !debugOptsOutOfAttach(strings.Join(config.Args, " ")) {
			raw, err := m.invoke(ctx, "run_gradle_jvm_args_script", map[string]any{
				"jvmArgs": []string{debugAttachListener},
			})
			if err == nil {
				var script string
				if json.Unmarshal(raw, &script) != nil {
					script = ""
				}
				config.Args = debugWithInitScript(config.Args, script)
			}
		}
		return config, nil
	}
	if debugGradleHasTask(config.Args, "run", "check") {
		if !containsString(config.Args, "--debug-jvm") {
			config.Args = append(config.Args, "--debug-jvm")
		}
		raw, err := m.invoke(ctx, "run_gradle_jvm_args_script", map[string]any{
			"jvmArgs": []string{debugAttachListener},
		})
		if err == nil {
			var script string
			if json.Unmarshal(raw, &script) == nil {
				config.Args = debugWithInitScript(config.Args, script)
			}
		}
		return config, nil
	}
	return config, fmt.Errorf("this Gradle task does not start a debuggable JVM")
}

func (m *Model) startBuildToolDebug(config *openidea.RunConfig, root string, show bool) {
	kind := debugBuildToolKind(config)
	if kind == "" {
		return
	}
	if !m.canStartDebugWithWindow(root, show) {
		return
	}
	if m.run.busy || m.run.running {
		m.debug.err = "A Run session is already active"
		m.invalidate()
		return
	}
	module := ""
	if config.Module != nil {
		module = *config.Module
	} else if config.Cwd != nil {
		module = *config.Cwd
	}
	name := config.Name
	if name == "" {
		name = config.ID
	}
	original := *config
	m.status = "Preparing the " + kind + " debug launch…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
		defer cancel()
		prepared := original
		pinnedPort := 0
		var err error
		if kind == "maven" && debugMavenIsTest(&original) {
			rawPort, portErr := m.invoke(ctx, "debug_free_port", nil)
			if portErr == nil {
				var port uint16
				if json.Unmarshal(rawPort, &port) == nil {
					pinnedPort = int(port)
				}
			}
		}
		switch kind {
		case "maven":
			prepared, err = m.debugMavenLaunch(original, pinnedPort)
		case "gradle":
			prepared, err = m.debugGradleLaunch(ctx, original)
		}
		var raw json.RawMessage
		if err == nil {
			raw, err = json.Marshal(prepared)
		}
		m.post(func() {
			if ctx.Err() != nil || m.Root() != root {
				return
			}
			if err != nil {
				m.debug.err = "Cannot debug " + name + ": " + err.Error()
				m.status = ""
				m.invalidate()
				return
			}
			if m.debug.starting || m.run.busy || m.run.running {
				m.debug.err = "The debugger or Run session changed while the build was being prepared"
				m.invalidate()
				return
			}
			m.debug.buildToolDebug = &debugBuildToolState{
				root: root, name: name, module: module,
				pinnedPort: pinnedPort, queued: map[int]bool{},
			}
			m.debug.err = ""
			if !m.startRunWith(raw, name) {
				m.debug.buildToolDebug = nil
				if m.debug.err == "" {
					m.debug.err = "The build-tool debug launch did not start"
				}
			} else {
				m.debug.buildToolDebug.channel = m.run.channel
				m.debug.status = "Waiting for the " + kind + " JVM to expose JDWP…"
			}
			m.invalidate()
		})
	}()
}

func (m *Model) acceptBuildToolDebugOutput(text string) {
	state := m.debug.buildToolDebug
	if state == nil || state.channel == 0 || state.channel != m.run.channel {
		return
	}
	clean := stripANSI(text)
	port := 0
	if match := debugJDWPListen.FindStringSubmatch(clean); len(match) > 1 {
		port, _ = strconv.Atoi(match[1])
	}
	if port == 0 && state.pinnedPort != 0 {
		line := strings.TrimSpace(clean)
		line = strings.TrimSpace(strings.TrimPrefix(line, "[INFO]"))
		if line == "T E S T S" {
			port = state.pinnedPort
		}
	}
	if port == 0 || state.queued[port] || m.debug.starting {
		return
	}
	state.queued[port] = true
	m.debug.status = fmt.Sprintf("Attaching to %s on JDWP port %d…", state.name, port)
	m.startDebugPlan(state.root, debugStartPlan{
		name:     state.name,
		config:   state.name,
		lastKind: "java-attach",
		status:   fmt.Sprintf("Attaching to %s on JDWP port %d…", state.name, port),
		prepare: func(ctx context.Context) (openidea.DebugLaunchConfig, error) {
			return m.javaAttachLaunch(
				ctx, state.name, state.module, "127.0.0.1", port, state.root, "java-debug",
			)
		},
	})
	m.invalidate()
}
