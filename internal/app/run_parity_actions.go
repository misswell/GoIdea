package app

// Run configuration parity actions after OpenIDEA's features/run: create a
// configuration from another feature, and import VS Code or JetBrains
// configurations into the native Run/Debug Configurations draft.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"mayoi/internal/adapter"
	"mayoi/internal/openidea"
)

func parityRunCommands() []appCommand {
	commands := []appCommand{
		{
			"run.importVscodeLaunch",
			"Import Run Configurations from .vscode/launch.json",
			"Run", "",
			"launch.json vscode visual studio code debug configuration import",
			func(m *Model) { m.ImportRunConfigurations(openidea.RunImportVSCode) },
		},
		{
			"run.importJetbrains",
			"Import Run Configurations from JetBrains (.run, .idea)",
			"Run", "",
			"pycharm webstorm intellij run.xml workspace.xml run configuration import",
			func(m *Model) { m.ImportRunConfigurations(openidea.RunImportJetBrains) },
		},
		{
			"run.createConfig",
			"Create Run Configuration",
			"Run", "",
			"create save run configuration launch",
			func(m *Model) {},
		},
		{
			"run.runConfig",
			"Run Configuration",
			"Run", "",
			"run configuration launch temporary",
			func(m *Model) {},
		},
	}
	return append(commands, runDynamicCommands()...)
}

func parityRunActionState(a *Action) {
	switch a.ID {
	case "run.importVscodeLaunch", "run.importJetbrains":
		a.When = func(m *Model) bool { return m.Root() != "" }
	case "run.createConfig":
		a.RunWithArg = func(m *Model, arg any) {
			m.CreateRunConfiguration(arg)
		}
		a.When = func(m *Model) bool { return m.Root() != "" }
	case "run.runConfig":
		a.RunWithArg = func(m *Model, arg any) {
			m.RunConfiguration(arg)
		}
		a.When = func(m *Model) bool { return m.Root() != "" }
	case "run.runContext", "run.debugContext":
		debug := a.ID == "run.debugContext"
		a.RunWithArg = func(m *Model, arg any) {
			m.RunContextConfigurationArg(arg, debug)
		}
		a.When = func(m *Model) bool { return m.Root() != "" }
	case "debug.attach":
		hiddenActionIDs["debug.attach"] = true
		a.When = func(m *Model) bool { return m.Root() != "" }
		a.RunWithArg = func(m *Model, arg any) {
			m.AttachDebugger(arg)
		}
	case "run.stop":
		a.RunWithArg = func(m *Model, arg any) {
			m.StopRunConfiguration(arg)
		}
	case "run.rerun":
		a.RunWithArg = func(m *Model, arg any) {
			m.RerunActiveRun(arg)
		}
	case "run.rerunFailed":
		a.RunWithArg = func(m *Model, arg any) {
			m.RerunFailedTestsFor(arg)
		}
	case "go.generate.file", "go.generate.package":
		scope := strings.TrimPrefix(a.ID, "go.generate.")
		a.RunWithArg = func(m *Model, arg any) {
			m.RunGoGenerate(scope, arg)
		}
		a.When = func(m *Model) bool { return m.Root() != "" }
	case "php.composerInstall":
		a.RunWithArg = func(m *Model, arg any) {
			m.ComposerInstall(arg)
		}
		a.When = func(m *Model) bool { return m.Root() != "" }
	case "run.modifyContext":
		a.RunWithArg = func(m *Model, arg any) {
			m.ModifyRunContext(arg)
		}
		a.When = func(m *Model) bool { return m.Root() != "" }
	case "run.sampleOutput":
		a.RunWithArg = func(m *Model, arg any) {
			m.StreamSampleOutput(arg)
		}
	}
}

type runContextActionArg struct {
	Path    string
	Line    int
	HasLine bool
	Content string
	Dir     bool
}

func decodeRunContextActionArg(arg any) (runContextActionArg, bool) {
	if arg == nil {
		return runContextActionArg{}, false
	}
	raw, err := json.Marshal(arg)
	if err != nil {
		return runContextActionArg{}, false
	}
	var value struct {
		Path    string `json:"path"`
		Line    *int   `json:"line"`
		Content string `json:"content"`
		Dir     bool   `json:"dir"`
	}
	if json.Unmarshal(raw, &value) != nil || strings.TrimSpace(value.Path) == "" {
		return runContextActionArg{}, false
	}
	line := 0
	hasLine := value.Line != nil
	if hasLine {
		line = max(0, *value.Line)
	}
	return runContextActionArg{
		Path: value.Path, Line: line, HasLine: hasLine, Content: value.Content, Dir: value.Dir,
	}, true
}

func (m *Model) RunContextConfigurationArg(arg any, debug bool) {
	if target, ok := decodeRunContextActionArg(arg); ok {
		m.runContextConfigurationAt(target, debug)
		return
	}
	m.RunContextConfigurationFor(arg, debug)
}

func (m *Model) StopRunConfiguration(arg any) {
	if id := runProcessID(arg); id != 0 {
		m.StopProcess(id)
		return
	}
	mode, _ := arg.(string)
	if mode == "widget" {
		if session := selectedRunSurfaceSession(m); session != nil && session.live {
			m.stopRunSession(session)
			return
		}
		if cfg := m.run.current(); cfg != nil {
			if session := m.chooserConfigSession(cfg.ID); session.running {
				m.activateChooserConfigSession(cfg.ID, session, false)
				return
			}
			if cfg.Kind == "compound" {
				ids := make(map[string]bool)
				for _, member := range adapter.CompoundChildren(cfg, runConfigPointers(m.run.configs)) {
					ids[member.ID] = true
				}
				stopped := false
				for _, child := range append([]*runChild(nil), m.run.children...) {
					if ids[child.config.ID] && !child.finished {
						m.stopRunChild(child)
						stopped = true
					}
				}
				if stopped {
					return
				}
			}
		}
	}
	if m.dockWindowShown("debug") && m.activeToolWindow() == "debug" {
		m.StopDebug()
		return
	}
	if m.dockWindowShown("run") && m.activeToolWindow() == "run" {
		if session := m.runWindowSession(); session != nil && session.live {
			m.stopRunSession(session)
			return
		}
	}
	if session := selectedRunSurfaceSession(m); session != nil && session.live {
		m.stopRunSession(session)
		return
	}
	m.StopProcess(nil)
}

func (m *Model) RerunActiveRun(arg any) {
	mode, _ := arg.(string)
	if mode == "widget" {
		if session := selectedRunSurfaceSession(m); session != nil && session.finished {
			m.rerunRunSession(session)
			return
		}
		if cfg := m.run.current(); cfg != nil {
			if session := m.chooserConfigSession(cfg.ID); session.running {
				m.activateChooserConfigSession(cfg.ID, session, true)
				return
			}
		}
	}
	if m.dockWindowShown("run") && m.activeToolWindow() == "run" {
		if session := m.runWindowSession(); session != nil {
			m.rerunRunSession(session)
			return
		}
	}
	if session := selectedRunSurfaceSession(m); session != nil && session.finished {
		m.rerunRunSession(session)
		return
	}
	if cfg := m.run.current(); cfg != nil {
		if session := m.chooserConfigSession(cfg.ID); session.running {
			m.activateChooserConfigSession(cfg.ID, session, true)
			return
		}
	}
	m.Rerun()
}

func (m *Model) runWindowSession() *runSessionView {
	if m.run.viewChild != nil {
		return m.serviceRunSession(m.run.viewChild)
	}
	return m.activeRunSession()
}

// selectedRunSurfaceSession follows the selected tab in a Run surface. Some
// callers create that surface without asking DockShow to open it, so the
// pane's active child is still the source of truth even when its visible
// flag has not been reconciled with the dock layout yet.
func selectedRunSurfaceSession(m *Model) *runSessionView {
	if m == nil || (m.tool != "run" && !m.run.visible) {
		return nil
	}
	return m.activeRunSession()
}

func (m *Model) runContextConfigurationAt(target runContextActionArg, debug bool) {
	root := m.Root()
	if root == "" {
		return
	}
	path := target.Path
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	path = filepath.Clean(path)
	if path != root && !inProject(root, path) {
		m.run.err = "The context is outside the open project"
		m.debug.err = m.run.err
		m.status = m.run.err
		m.invalidate()
		return
	}
	content := target.Content
	if !target.Dir && content == "" {
		if tab := m.tab(); tab != nil && tab.doc != nil && realPath(tab.doc.Path()) == realPath(path) {
			content = tab.doc.Text()
		} else if data, err := os.ReadFile(path); err == nil {
			content = limitRunContextContent(string(data))
		}
	}
	var cfg *openidea.RunConfig
	var err error
	switch {
	case target.Dir:
		cfg, err = m.contextRunConfigForDirectory(path, content)
	case target.HasLine:
		cfg, err = m.contextRunConfigAtLine(path, target.Line, content)
	default:
		cfg, err = contextRunConfigFor(root, path, content, m.run.configs)
	}
	if err == nil && cfg != nil && cfg.Kind == "python" {
		err = m.resolveContextRunTool(cfg, path)
	}
	if err != nil {
		m.run.err = err.Error()
		m.debug.err = err.Error()
		m.notifyInfo("run-context", "Nothing to run here", err.Error())
		m.invalidate()
		return
	}
	cfg = m.adoptRunTemporary(*cfg)
	if cfg == nil {
		return
	}
	m.run.err = ""
	if debug {
		m.DebugRunConfig(cfg)
		return
	}
	m.askOrStart(cfg.Raw(), cfg.Name)
}

func (m *Model) contextRunConfigForDirectory(path string, content string) (*openidea.RunConfig, error) {
	root := m.Root()
	rel := relPathUnder(root, path)
	if rel == "" || rel == "." || !inProject(root, path) {
		return nil, fmt.Errorf("The directory is outside the open project")
	}
	only := func(kinds ...string) []openidea.RunConfig {
		allowed := make(map[string]bool, len(kinds))
		for _, kind := range kinds {
			allowed[kind] = true
		}
		var out []openidea.RunConfig
		for _, cfg := range m.run.configs {
			if cfg.IsTest && allowed[cfg.Kind] && !strings.HasPrefix(cfg.ID, "context:") {
				out = append(out, cfg)
			}
		}
		return out
	}
	closest := func(kinds ...string) *openidea.RunConfig {
		candidates := closestContextConfigs(only(kinds...), rel)
		if len(candidates) == 0 {
			return nil
		}
		cfg := candidates[0]
		return &cfg
	}
	name := filepath.Base(path)
	makeConfig := func(base openidea.RunConfig, args []string) *openidea.RunConfig {
		cfg := deriveContextConfig(base, rel+"/.")
		cfg.ID = "context:dir:" + base.ID + ":" + rel
		cfg.Name = "Tests in '" + name + "'"
		cfg.Args = args
		cfg.IsTest = true
		return &cfg
	}
	jvmTestDir := strings.Contains("/"+rel+"/", "/src/test/") ||
		strings.HasSuffix(rel, "src/test") ||
		strings.HasSuffix(rel, "src/test/java") ||
		strings.HasSuffix(rel, "src/test/kotlin") ||
		strings.HasSuffix(rel, "src/test/scala") ||
		strings.HasSuffix(rel, "src/test/groovy")
	if jvmTestDir {
		if base := closest("gradle", "maven"); base != nil {
			args := append([]string(nil), base.Args...)
			if i := strings.Index(rel, "/src/test/"); i >= 0 {
				pkg := filepath.ToSlash(strings.TrimPrefix(rel[i+len("/src/test/"):], "java/"))
				pkg = strings.TrimPrefix(pkg, "kotlin/")
				pkg = strings.TrimPrefix(pkg, "scala/")
				pkg = strings.TrimPrefix(pkg, "groovy/")
				pkg = strings.ReplaceAll(pkg, "/", ".")
				if pkg != "" && pkg != "." {
					if base.Kind == "gradle" {
						args = append(args, "--tests", pkg+".*")
					} else {
						args = append(args, "-Dtest="+pkg+".**.*", "-Dsurefire.failIfNoSpecifiedTests=false")
					}
				}
			}
			return makeConfig(*base, args), nil
		}
	}
	testDir := regexp.MustCompile(`(?i)(^|/)(tests?|__tests__|specs?|test_.+)$`).MatchString(rel)
	if !testDir && !jvmTestDir {
		return nil, fmt.Errorf("The selected directory has no test configuration")
	}
	if base := closest("cargo"); base != nil && strings.HasSuffix(rel, "tests") {
		return makeConfig(*base, append(append([]string(nil), base.Args...), "--tests")), nil
	}
	if base := closest("npm", "bun", "deno"); base != nil {
		arg := contextRelToCwd(rel, *base)
		args := append([]string(nil), base.Args...)
		if base.Kind == "deno" || slices.Contains(args, "--") {
			args = append(args, arg)
		} else {
			args = append(args, "--", arg)
		}
		return makeConfig(*base, args), nil
	}
	if base := closest("pytest"); base != nil {
		return makeConfig(*base, append(append([]string(nil), base.Args...), contextRelToCwd(rel, *base))), nil
	}
	if base := closest("go"); base != nil {
		dir := contextRelToCwd(rel, *base)
		if dir == "" {
			dir = "."
		} else {
			dir = "./" + strings.TrimPrefix(filepath.ToSlash(dir), "./")
		}
		args := make([]string, len(base.Args))
		for i, arg := range base.Args {
			if arg == "./..." {
				args[i] = dir + "/..."
			} else {
				args[i] = arg
			}
		}
		return makeConfig(*base, args), nil
	}
	return nil, fmt.Errorf("The project has no test configuration for this directory")
}

var (
	goRunTargetPattern = regexp.MustCompile(`(?m)^\s*func\s+((?:Test|Benchmark|Example)\w*)\s*\(`)
	jsRunTargetPattern = regexp.MustCompile(`(?m)^\s*(?:describe|suite|context|test|it)(?:\.\w+)*\s*\(\s*['"` + "`" + `]([^'"` + "`" + `]+)['"` + "`" + `]`)
	pyRunTargetPattern = regexp.MustCompile(`(?m)^\s*(?:async\s+)?def\s+(test\w*)\s*\(`)
	rsRunTargetPattern = regexp.MustCompile(`(?m)^\s*(?:pub\s+)?(?:async\s+)?fn\s+(\w+)\s*\(`)
	jvmTestPattern     = regexp.MustCompile(`(?m)@(?:Test|ParameterizedTest|RepeatedTest|TestFactory)\b`)
)

func (m *Model) contextRunConfigAtLine(path string, line int, content string) (*openidea.RunConfig, error) {
	cfg, err := contextRunConfigFor(m.Root(), path, content, m.run.configs)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(content, "\n")
	if line < 0 || line >= len(lines) {
		return cfg, nil
	}
	rel := relPathUnder(m.Root(), path)
	lower := strings.ToLower(rel)
	switch {
	case strings.HasSuffix(lower, ".go") && cfg.IsTest:
		if match := goRunTargetAtLine(lines, line); len(match) == 2 {
			args := withoutGoRunSelection(cfg.Args)
			args = append(args, "-run", "^"+regexp.QuoteMeta(match[1])+"$")
			cfg.Args = args
			cfg.Name = match[1]
			cfg.ID += ":line:" + match[1]
		}
	case (strings.HasSuffix(lower, ".js") || strings.HasSuffix(lower, ".jsx") ||
		strings.HasSuffix(lower, ".ts") || strings.HasSuffix(lower, ".tsx")) && cfg.IsTest:
		if match := jsRunTargetAtLine(lines, line); len(match) == 2 {
			cfg.Args = append(cfg.Args, "-t", "^"+regexp.QuoteMeta(match[1])+"$")
			cfg.Name = match[1]
			cfg.ID += ":line:" + match[1]
		}
	case strings.HasSuffix(lower, ".py") && cfg.IsTest:
		if match := pyRunTargetAtLine(lines, line); len(match) == 2 {
			cfg.Args = append(cfg.Args, "::"+match[1])
			cfg.Name = match[1]
			cfg.ID += ":line:" + match[1]
		}
	case strings.HasSuffix(lower, ".rs") && cfg.IsTest:
		if match := rsRunTargetAtLine(lines, line); len(match) == 2 {
			cfg.Args = append(cfg.Args, "--", match[1], "--exact")
			cfg.Name = match[1]
			cfg.ID += ":line:" + match[1]
		}
	case jvmSourceFile.MatchString(lower) && cfg.IsTest:
		if cfg.Kind != "gradle" && cfg.Kind != "maven" {
			return cfg, nil
		}
		method := jvmMethodAtLine(lines, line)
		if method != "" {
			class := jvmClassName(rel, content)
			if class == "" {
				return cfg, nil
			}
			switch cfg.Kind {
			case "gradle":
				cfg.Args = replaceJVMTarget(cfg.Args, "gradle", class+"."+method)
			case "maven":
				cfg.Args = replaceJVMTarget(cfg.Args, "maven", class+"#"+method)
			}
			cfg.Name += "." + method
			cfg.ID += ":line:" + method
		}
	}
	return cfg, nil
}

func adjacentRunTarget(lines []string, line int, pattern *regexp.Regexp, annotation *regexp.Regexp) []string {
	if line < 0 || line >= len(lines) {
		return nil
	}
	match := pattern.FindStringSubmatch(lines[line])
	if len(match) > 1 {
		return match
	}
	if annotation == nil {
		return nil
	}
	for i := line - 1; i >= 0 && line-i <= 8; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if annotation.MatchString(trimmed) {
			for j := line; j <= min(len(lines)-1, line+3); j++ {
				if match := pattern.FindStringSubmatch(lines[j]); len(match) > 1 {
					return match
				}
			}
			return nil
		}
		if trimmed != "" && !strings.HasPrefix(trimmed, "@") {
			return nil
		}
	}
	return nil
}

func goRunTargetAtLine(lines []string, line int) []string {
	if line < 0 || line >= len(lines) {
		return nil
	}
	match := goRunTargetPattern.FindStringSubmatch(lines[line])
	if len(match) != 2 || strings.HasPrefix(match[1], "Benchmark") {
		return nil
	}
	return match
}

func jsRunTargetAtLine(lines []string, line int) []string {
	return adjacentRunTarget(lines, line, jsRunTargetPattern, nil)
}

func pyRunTargetAtLine(lines []string, line int) []string {
	for i := line; i >= 0 && line-i <= 10; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if i != line && trimmed != "" && !strings.HasPrefix(trimmed, "@") &&
			!strings.HasPrefix(trimmed, "def ") && !strings.HasPrefix(trimmed, "async def ") {
			break
		}
		if match := pyRunTargetPattern.FindStringSubmatch(lines[i]); len(match) == 2 {
			return match
		}
	}
	return nil
}

func rsRunTargetAtLine(lines []string, line int) []string {
	return adjacentRunTarget(lines, line, rsRunTargetPattern, regexp.MustCompile(`^\s*#\[.*test`))
}

func withoutGoRunSelection(args []string) []string {
	out := append([]string(nil), args...)
	for i := 0; i < len(out); i++ {
		switch {
		case out[i] == "-run":
			if i+1 < len(out) {
				out = append(out[:i], out[i+2:]...)
			} else {
				out = append(out[:i], out[i+1:]...)
			}
			i--
		case strings.HasPrefix(out[i], "-run="):
			out = append(out[:i], out[i+1:]...)
			i--
		}
	}
	return out
}

func replaceJVMTarget(args []string, kind string, target string) []string {
	out := append([]string(nil), args...)
	for i := len(out) - 1; i >= 0; i-- {
		switch {
		case kind == "gradle" && out[i] == "--tests" && i+1 < len(out):
			out[i+1] = target
			return out
		case kind == "maven" && strings.HasPrefix(out[i], "-Dtest="):
			out[i] = "-Dtest=" + target
			return out
		}
	}
	if kind == "gradle" {
		return append(out, "--tests", target)
	}
	return append(out, "-Dtest="+target, "-Dsurefire.failIfNoSpecifiedTests=false")
}

func jvmMethodAtLine(lines []string, line int) string {
	pattern := regexp.MustCompile(`(?:\bfun\s+` + "`?" + `([A-Za-z_$][\w$]*)` + "`?" + `|\b([A-Za-z_$][\w$]*)\s*\()\s*`)
	start := max(0, line-5)
	for i := line; i >= start; i-- {
		if i != line && jvmTestPattern.MatchString(lines[i]) && i < line {
			return ""
		}
		if match := pattern.FindStringSubmatch(lines[i]); len(match) == 3 {
			if match[1] != "" {
				return match[1]
			}
			if match[2] != "" && match[2] != "if" && match[2] != "for" &&
				match[2] != "while" && match[2] != "switch" && match[2] != "catch" {
				return match[2]
			}
		}
	}
	return ""
}

// RunConfiguration is OpenIDEA's hidden run.runConfig integration action. It
// accepts a saved configuration id, a RunConfig argument, or
// {"config": RunConfig, "temporary": true}. Temporary configurations live in
// the Run widget for this project session and are never persisted.
func (m *Model) RunConfiguration(arg any) {
	if m.Root() == "" {
		return
	}
	if id, ok := arg.(string); ok {
		m.run.pendingInline = nil
		m.run.inline = nil
		if m.run.loaded {
			if m.runChooserByID(id) != nil {
				m.selectRunConfig(id)
				m.run.pendingConfigID = ""
				m.Run()
				return
			}
			m.notifyInfo("run-config", "Run configuration not found", id)
			return
		}
		m.run.pendingConfigID = id
		m.Run()
		return
	}

	cfg, temporary, ok := runConfigFromActionArg(arg)
	if !ok {
		return
	}
	m.run.pendingConfigID = ""
	if temporary {
		m.runTemporaryConfiguration(cfg)
		return
	}
	if onExit := runExitCallback(arg); onExit != nil {
		m.beginRunPlugin(cfg.ID, onExit)
		if !m.startRunWith(cfg.Raw(), cfg.Name) {
			m.cancelRunPlugin()
			onExit(nil, false)
		}
		return
	}
	if m.startRunWith(cfg.Raw(), cfg.Name) {
		m.adoptRunTemporary(cfg)
		m.invalidate()
	}
}

func runExitCallback(arg any) func(*int32, bool) {
	switch value := arg.(type) {
	case map[string]any:
		callback, _ := value["onExit"].(func(*int32, bool))
		return callback
	default:
		return nil
	}
}

func runConfigFromActionArg(arg any) (openidea.RunConfig, bool, bool) {
	if arg == nil {
		return openidea.RunConfig{}, false, false
	}
	if cfg, ok := arg.(openidea.RunConfig); ok {
		return cfg, false, true
	}
	if cfg, ok := arg.(*openidea.RunConfig); ok && cfg != nil {
		return *cfg, false, true
	}
	if values, ok := arg.(map[string]any); ok {
		temporary, _ := values["temporary"].(bool)
		switch cfg := values["config"].(type) {
		case openidea.RunConfig:
			return cfg, temporary, true
		case *openidea.RunConfig:
			if cfg != nil {
				return *cfg, temporary, true
			}
		}
		if rawConfig := values["config"]; rawConfig != nil {
			raw, err := json.Marshal(rawConfig)
			if err == nil {
				cfg, decodeErr := openidea.DecodeRunConfig(raw)
				if decodeErr == nil {
					return *cfg, temporary, true
				}
			}
		}
	}

	var wrapped struct {
		Config    json.RawMessage `json:"config"`
		Temporary bool            `json:"temporary"`
	}
	raw, err := json.Marshal(arg)
	if err != nil {
		return openidea.RunConfig{}, false, false
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil || len(wrapped.Config) == 0 {
		cfg, err := openidea.DecodeRunConfig(raw)
		if err != nil || cfg == nil {
			return openidea.RunConfig{}, false, false
		}
		return *cfg, false, true
	}
	cfg, err := openidea.DecodeRunConfig(wrapped.Config)
	if err != nil || cfg == nil {
		return openidea.RunConfig{}, false, false
	}
	return *cfg, wrapped.Temporary, true
}

func (m *Model) runTemporaryConfiguration(cfg openidea.RunConfig) {
	selected := m.adoptRunTemporary(cfg)
	if selected == nil {
		return
	}
	m.run.visible = true
	m.occupyBottom("run")
	if m.run.busy {
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
	m.run.pendingInline = nil
	m.startRunWith(selected.Raw(), selected.Name)
}

// ImportRunConfigurations opens Run/Debug Configurations with the file's
// translations added to its draft. The configurations are saved only by the
// dialog's Save action, as upstream does.
func (m *Model) ImportRunConfigurations(source openidea.RunImportSource) {
	if m.Root() == "" || m.importingRunConfiguration() {
		return
	}
	if !m.run.trusted {
		m.status = "Trust the project before importing run configurations"
		m.invalidate()
		return
	}

	dialog := m.runCfgDlg
	var draft []*openidea.RunConfig
	var baseConfigs []openidea.RunConfig
	if dialog != nil {
		dialog.applyForm()
		draft = append([]*openidea.RunConfig(nil), dialog.items...)
		dialog.importing = true
		dialog.importErr = ""
	} else {
		m.run.loading = true
	}
	m.invalidate()

	root := m.Root()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
		defer cancel()

		sourcesRaw, err := m.invoke(ctx, "run_import_sources", nil)
		if err != nil {
			m.finishRunImport(dialog, root, nil, openidea.RunImport{}, err, false)
			return
		}
		var sources []openidea.RunImportSource
		if err := json.Unmarshal(sourcesRaw, &sources); err != nil {
			m.finishRunImport(dialog, root, nil, openidea.RunImport{}, err, false)
			return
		}
		if !slices.Contains(sources, source) {
			m.finishRunImport(dialog, root, nil, openidea.RunImport{Source: source}, nil, true)
			return
		}

		if dialog == nil {
			raw, err := m.invoke(ctx, "run_configs", nil)
			if err != nil {
				m.finishRunImport(nil, root, nil, openidea.RunImport{}, err, false)
				return
			}
			configs, err := openidea.DecodeRunConfigs(raw)
			if err != nil {
				m.finishRunImport(nil, root, nil, openidea.RunImport{}, err, false)
				return
			}
			baseConfigs = configs
			draft = runConfigPointers(configs)
		}

		existing, err := runConfigPayload(draft)
		if err != nil {
			m.finishRunImport(dialog, root, baseConfigs, openidea.RunImport{}, err, false)
			return
		}
		raw, err := m.invoke(ctx, "run_import_configs", map[string]any{
			"source": source, "existing": existing,
		})
		if err != nil {
			m.finishRunImport(dialog, root, baseConfigs, openidea.RunImport{}, err, false)
			return
		}
		var result openidea.RunImport
		if err := json.Unmarshal(raw, &result); err != nil {
			m.finishRunImport(dialog, root, baseConfigs, openidea.RunImport{}, err, false)
			return
		}
		m.finishRunImport(dialog, root, baseConfigs, result, nil, false)
	}()
}

func (m *Model) importingRunConfiguration() bool {
	return m.runCfgDlg != nil && m.runCfgDlg.importing
}

func (m *Model) finishRunImport(
	dialog *runConfigDlg,
	root string,
	baseConfigs []openidea.RunConfig,
	result openidea.RunImport,
	err error,
	missing bool,
) {
	m.post(func() {
		if m.Root() != root {
			return
		}
		if dialog != nil && m.runCfgDlg == dialog {
			dialog.importing = false
		} else {
			m.run.loading = false
		}
		if missing {
			m.notifyInfo("run-import", runImportMissingText(result.Source), "")
			m.status = runImportMissingText(result.Source)
			m.invalidate()
			return
		}
		if err != nil {
			if dialog != nil && m.runCfgDlg == dialog {
				dialog.importErr = "Cannot import run configurations: " + err.Error()
			} else {
				m.notifyError("run-import", "Cannot import run configurations", err.Error())
				m.status = "Cannot import run configurations: " + err.Error()
			}
			m.invalidate()
			return
		}

		imported, decodeErr := decodeImportedRunConfigs(result.Imported)
		if decodeErr != nil {
			m.notifyError("run-import", "Cannot read imported run configurations", decodeErr.Error())
			m.status = "Cannot read imported run configurations: " + decodeErr.Error()
			m.invalidate()
			return
		}
		notice := runImportNotice(result)
		if dialog != nil && m.runCfgDlg == dialog {
			appendRunImportToDialog(dialog, imported, result, notice)
		} else {
			m.run.configs = append(append([]openidea.RunConfig(nil), baseConfigs...), imported...)
			m.run.loaded = true
			m.OpenRunConfigs()
			if m.runCfgDlg != nil {
				appendRunImportToDialog(m.runCfgDlg, imported, result, notice)
			}
		}
		level := "info"
		if len(result.Skipped) > 0 || len(imported) == 0 {
			level = "warning"
		}
		m.notifyUser("run-import", level, runImportTitle(result, len(imported)), strings.Join(notice, "\n"))
		m.status = runImportTitle(result, len(imported))
		m.invalidate()
	})
}

func decodeImportedRunConfigs(imported []openidea.ImportedRunConfig) ([]openidea.RunConfig, error) {
	out := make([]openidea.RunConfig, 0, len(imported))
	for _, item := range imported {
		cfg, err := openidea.DecodeRunConfig(item.Config)
		if err != nil {
			return nil, err
		}
		cfg.Detected = false
		out = append(out, *cfg)
	}
	return out, nil
}

func appendRunImportToDialog(
	d *runConfigDlg,
	imported []openidea.RunConfig,
	result openidea.RunImport,
	notice []string,
) {
	first := len(d.items)
	for i := range imported {
		cfg := imported[i]
		cfg.Detected = false
		d.items = adapter.UpsertConfig(d.items, &cfg)
	}
	if len(imported) > 0 {
		d.sel = min(first, len(d.items)-1)
		d.loadForm()
	}
	d.importFile = result.File
	d.importNotice = append([]string(nil), notice...)
	d.importErr = ""
}

func runConfigPointers(configs []openidea.RunConfig) []*openidea.RunConfig {
	out := make([]*openidea.RunConfig, len(configs))
	for i := range configs {
		out[i] = &configs[i]
	}
	return out
}

func runConfigPayload(configs []*openidea.RunConfig) ([]json.RawMessage, error) {
	out := make([]json.RawMessage, 0, len(configs))
	for _, cfg := range configs {
		if cfg == nil {
			continue
		}
		raw := cfg.Raw()
		if len(raw) == 0 {
			var err error
			raw, err = json.Marshal(cfg)
			if err != nil {
				return nil, err
			}
		}
		out = append(out, append(json.RawMessage(nil), raw...))
	}
	return out, nil
}

func runImportMissingText(source openidea.RunImportSource) string {
	switch source {
	case openidea.RunImportVSCode:
		return "This project has no .vscode/launch.json"
	case openidea.RunImportJetBrains:
		return "This project has no JetBrains run configurations (.run, .idea)"
	}
	return "This project has no run configurations to import"
}

func runImportTitle(result openidea.RunImport, imported int) string {
	if imported == 0 {
		return "No run configurations imported from " + result.File
	}
	if imported == 1 {
		return "Imported 1 run configuration from " + result.File
	}
	return fmt.Sprintf("Imported %d run configurations from %s", imported, result.File)
}

func runImportNotice(result openidea.RunImport) []string {
	var out []string
	for _, skipped := range result.Skipped {
		name := skipped.Name
		if name == "" {
			name = "Unnamed"
		}
		out = append(out, fmt.Sprintf(
			"Skipped '%s': %s", name, runImportSkipReason(skipped.Reason)))
	}
	for _, imported := range result.Imported {
		if len(imported.Ignored) > 0 {
			var name string
			var cfg struct {
				Name string `json:"name"`
			}
			_ = json.Unmarshal(imported.Config, &cfg)
			name = cfg.Name
			out = append(out, fmt.Sprintf(
				"Left out of '%s': %s", name, strings.Join(imported.Ignored, ", ")))
		}
	}
	if len(result.Imported) > 0 {
		out = append(out, "Click Save to keep the imported configurations.")
	}
	return out
}

func runImportSkipReason(reason map[string]any) string {
	kind, _ := reason["kind"].(string)
	value := func(key string) string {
		text, _ := reason[key].(string)
		return text
	}
	switch kind {
	case "nameTaken":
		return "A configuration with this name already exists"
	case "unsupportedType":
		return fmt.Sprintf("Type '%s' has no equivalent here", value("type"))
	case "unsupportedRequest":
		return fmt.Sprintf("Request '%s' is not supported", value("request"))
	case "unsupportedAttach":
		return "Only attaching to a host and port is supported"
	case "variable":
		return fmt.Sprintf("Uses %s, which is known only when VS Code starts it", value("variable"))
	case "nothingToRun":
		return "No program, module or command to run"
	case "pathVariable":
		return fmt.Sprintf("Uses %s, a path variable with no value here", value("variable"))
	case "severalModules":
		return fmt.Sprintf("Uses %s, and the project has several modules", value("variable"))
	case "missingFile":
		return fmt.Sprintf("%s is not in the project", value("file"))
	case "invalidFile":
		return "The file cannot be read: " + value("detail")
	case "missingMember":
		return fmt.Sprintf("Its configuration '%s' is not imported", value("member"))
	case "invalid":
		return "Not a configuration with a name"
	}
	return "The source entry has no equivalent here"
}

// CreateRunConfiguration saves the supplied configuration as a user
// configuration and opens it in the native editor, matching
// run.createConfig's hidden integration action.
func (m *Model) CreateRunConfiguration(arg any) {
	if m.Root() == "" || arg == nil {
		return
	}
	raw, err := json.Marshal(arg)
	if err != nil {
		m.status = "Cannot create the run configuration: " + err.Error()
		m.invalidate()
		return
	}
	cfg, err := openidea.DecodeRunConfig(raw)
	if err != nil {
		m.status = "Cannot create the run configuration: " + err.Error()
		m.invalidate()
		return
	}
	cfg.Detected = false
	root := m.Root()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
		defer cancel()
		configRaw, err := m.invoke(ctx, "run_configs", nil)
		if err != nil {
			m.post(func() {
				m.notifyError("run-create", "Cannot create the run configuration", err.Error())
				m.status = "Cannot create the run configuration: " + err.Error()
				m.invalidate()
			})
			return
		}
		configs, err := openidea.DecodeRunConfigs(configRaw)
		if err != nil {
			m.post(func() {
				m.notifyError("run-create", "Cannot create the run configuration", err.Error())
				m.invalidate()
			})
			return
		}
		items := adapter.UpsertConfig(runConfigPointers(configs), cfg)
		err = adapter.SaveUserConfigs(ctx, javaBackend{m}, adapter.UserConfigs(items), nil)
		m.post(func() {
			if m.Root() != root {
				return
			}
			if err != nil {
				m.notifyError("run-create", "Cannot create the run configuration", err.Error())
				m.status = "Cannot create the run configuration: " + err.Error()
				m.invalidate()
				return
			}
			m.run.configs = m.run.configs[:0]
			for _, item := range items {
				m.run.configs = append(m.run.configs, *item)
			}
			m.run.loaded = true
			m.OpenRunConfigs()
			d := m.runCfgDlg
			if d == nil {
				return
			}
			for i, item := range d.items {
				if item.ID == cfg.ID {
					d.sel = i
					d.loadForm()
					break
				}
			}
			m.status = "Created run configuration " + cfg.Name
			m.invalidate()
		})
	}()
	m.invalidate()
}
