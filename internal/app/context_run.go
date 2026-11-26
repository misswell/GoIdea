package app

import (
	"context"
	"encoding/json"
	"errors"
	"path"
	"path/filepath"
	"regexp"
	goruntime "runtime"
	"sort"
	"strings"
	"time"

	"mayoi/internal/adapter"
	"mayoi/internal/openidea"
)

var (
	javaMainDeclaration = regexp.MustCompile(
		`(?m)\bstatic\b[^;{}\n]{0,80}\bvoid\s+main\s*\(\s*(?:final\s+)?(?:java\.lang\.)?String\s*(?:\[\s*\]|\.\.\.)`,
	)
	kotlinMainDeclaration = regexp.MustCompile(`(?m)\bfun\s+main\s*\(`)
	pythonRelativeImport  = regexp.MustCompile(`(?m)^[ \t]*from[ \t]+\.+[\w.]*[ \t]+import\b`)
	pythonIdentifier      = regexp.MustCompile(`^[A-Za-z_]\w*$`)
	pythonCommand         = regexp.MustCompile(`^python[\d.]*$`)
	csharpTestAttribute   = regexp.MustCompile(`\[(Fact|Theory|Test|TestMethod|TestCase)\b`)
	goMainPackage         = regexp.MustCompile(`(?m)^\s*package\s+main\b`)
	goTestFile            = regexp.MustCompile(`_test\.go$`)
	rustTestFile          = regexp.MustCompile(`(?:^|/)tests/([^/]+)\.rs$`)
	cargoBinFile          = regexp.MustCompile(`(?:^|/)src/bin/([^/]+)\.rs$`)
	genericTestFile       = regexp.MustCompile(`(Test|Tests|Spec|IT)\.(kt|java|scala|groovy)$`)
	webTestFile           = regexp.MustCompile(`(?i)\.(test|spec)\.[cm]?[jt]sx?$`)
	webSourceFile         = regexp.MustCompile(`\.[cm]?[jt]sx?$`)
	jvmSourceFile         = regexp.MustCompile(`\.(kt|java|scala|groovy)$`)
)

// RunContextConfiguration runs the active file through the temporary
// configuration OpenIDEA derives for tests, programs and scripts.
func (m *Model) RunContextConfiguration() {
	m.requestContextRun(false, false)
}

func (m *Model) RunContextConfigurationFor(arg any, debug bool) {
	if path := buildToolArgString(arg, "path"); path != "" {
		if tab, _ := m.tabByPathInAnyGroup(path); tab != nil {
			group, ok := m.groupOf(tab)
			if ok {
				m.focusGroup(group)
				m.activateIn(group, tabIndexIn(m.groupTabsAt(group), tab))
			}
		} else {
			m.openFile(path)
		}
	}
	m.requestContextRun(false, debug)
}

// RunContextConfigurationWithCoverage runs run.coverageContext: the same
// context configuration through the coverage executor.
func (m *Model) RunContextConfigurationWithCoverage() {
	m.requestContextExecution(true, "")
}

// RunContextConfigurationWithProfiler runs run.profileContext: the active
// file's temporary configuration through the CPU profiler.
func (m *Model) RunContextConfigurationWithProfiler() {
	m.requestContextExecution(false, openidea.ProfileCPU)
}

func (m *Model) requestContextRun(coverage, debug bool) {
	m.requestContextExecution(coverage, "")
	if debug {
		m.run.pendingContext = false
		m.run.pendingDebugContext = true
		m.DockShow("debug")
	}
}

func (m *Model) requestContextExecution(coverage bool, profile openidea.ProfileMode) {
	if m.Root() == "" || m.ctx == nil {
		return
	}
	m.run.pendingContext = true
	m.run.pendingContextCoverage = coverage
	m.run.pendingContextProfile = profile
	m.run.pendingDebugContext = false
	m.DockShow("run")
	if !m.run.trusted {
		m.run.pendingTrust = true
		m.trustAsk = true
		m.status = "Trust this project to run the active file"
		m.invalidate()
		return
	}
	if !m.run.loaded {
		m.loadRunConfigs()
		m.invalidate()
		return
	}
	m.startContextRun()
}

func (m *Model) startContextRun() {
	coverage := m.run.pendingContextCoverage
	profile := m.run.pendingContextProfile
	m.run.pendingContext = false
	m.run.pendingContextCoverage = false
	m.run.pendingContextProfile = ""
	cfg, err := m.contextRunConfig()
	if err != nil {
		m.run.err = err.Error()
		m.notifyInfo("run-context", "Nothing to run here", err.Error())
		m.invalidate()
		return
	}
	cfg = m.adoptRunTemporary(*cfg)
	if cfg == nil {
		return
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		m.run.err = "Could not prepare the context configuration: " + err.Error()
		m.notifyError("run-context", "Cannot run the active file", err.Error())
		m.invalidate()
		return
	}
	if coverage {
		m.startRunWithCoverage(raw, cfg.Name, &coverageExecution{name: cfg.Name, root: m.Root()})
		return
	}
	if profile != "" {
		m.prepareProfilerConfig(cfg, profile)
		return
	}
	m.startRunWith(raw, cfg.Name)
}

func (m *Model) startContextDebug() {
	m.run.pendingDebugContext = false
	cfg, err := m.contextRunConfig()
	if err != nil {
		m.debug.err = err.Error()
		m.notifyInfo("debug-context", "Nothing to debug here", err.Error())
		m.invalidate()
		return
	}
	cfg = m.adoptRunTemporary(*cfg)
	if cfg == nil {
		return
	}
	m.DebugRunConfig(cfg)
}

// contextRunConfig ports OpenIDEA's context producer. A test file narrows the
// nearest matching test configuration, a main file reuses its language's run
// target, and a script starts through the detected interpreter.
func (m *Model) contextRunConfig() (*openidea.RunConfig, error) {
	tab := m.tab()
	if tab == nil || tab.doc == nil {
		return nil, errors.New("Open a source file first")
	}
	cfg, err := contextRunConfigFor(m.Root(), tab.doc.Path(), tab.doc.Text(), m.run.configs)
	if err != nil {
		return nil, err
	}
	if err := m.resolveContextRunTool(cfg, tab.doc.Path()); err != nil {
		// Tool lookup only improves a context command. A backend tooling
		// catalogue that is unavailable must not prevent an otherwise valid
		// run from starting; preserve the already resolved run configuration.
		m.status = "Could not resolve the context tool; using " + cfg.Command
	}
	m.applyContextLanguageSettings(cfg)
	return cfg, nil
}

// resolveContextRunTool replaces a bare context command with the executable
// OpenIDEA would use. PHP comes from the configured interpreter when set;
// node, npx and the other script runtimes come from the project-aware tool
// lookup, which prefers node_modules/.bin before the search path.
func (m *Model) resolveContextRunTool(cfg *openidea.RunConfig, near string) error {
	if cfg == nil || strings.TrimSpace(cfg.Command) == "" {
		return nil
	}
	name := strings.TrimSpace(cfg.Command)
	if strings.ContainsAny(name, `/\`) {
		return nil
	}
	if m.mgr == nil && m.invokeHook == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(modelContext(m), 15*time.Second)
	defer cancel()
	if strings.EqualFold(name, "php") {
		raw, err := m.invoke(ctx, "sdk_php_effective", nil)
		var effective *string
		if err == nil {
			err = json.Unmarshal(raw, &effective)
		}
		if err == nil && effective != nil && strings.TrimSpace(*effective) != "" {
			cfg.Command = strings.TrimSpace(*effective)
			return nil
		}
	}
	if near != "" {
		near = filepath.Dir(near)
	}
	raw, err := m.invoke(ctx, "sdk_locate_tools", map[string]any{
		"names": []string{name},
		"near":  near,
	})
	if err != nil {
		return err
	}
	var located openidea.LocatedTools
	if err := json.Unmarshal(raw, &located); err != nil {
		return err
	}
	for _, tool := range located.Tools {
		if !strings.EqualFold(tool.Name, name) || tool.Path == nil {
			continue
		}
		if path := strings.TrimSpace(*tool.Path); path != "" {
			cfg.Command = path
		}
		break
	}
	return nil
}

func (m *Model) applyContextLanguageSettings(cfg *openidea.RunConfig) {
	if cfg == nil {
		return
	}
	if cfg.Env == nil {
		cfg.Env = map[string]string{}
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Kind)) {
	case "python", "pytest", "django-test":
		if python := strings.TrimSpace(m.nativeRuntimeString(
			settingsPagePythonDeps, "languages.python.interpreter",
		)); python != "" {
			if !filepath.IsAbs(python) && m.Root() != "" {
				python = filepath.Join(m.Root(), python)
			}
			cfg.Command = filepath.Clean(python)
		}
	case "node", "npm", "pnpm", "yarn", "bun":
		if node := strings.TrimSpace(m.nativeRuntimeString(
			settingsPageNode, "languages.node.interpreter",
		)); node != "" && strings.EqualFold(cfg.Command, "node") {
			if !filepath.IsAbs(node) && m.Root() != "" {
				node = filepath.Join(m.Root(), node)
			}
			cfg.Command = filepath.Clean(node)
		}
	case "php":
		if php := strings.TrimSpace(m.nativeRuntimeString(
			settingsPagePHP, "languages.php.interpreter",
		)); php != "" {
			if !filepath.IsAbs(php) && m.Root() != "" {
				php = filepath.Join(m.Root(), php)
			}
			cfg.Command = filepath.Clean(php)
		}
	case "ruby":
		if ruby := strings.TrimSpace(m.nativeRuntimeString(
			settingsPageRuby, "languages.ruby.interpreter",
		)); ruby != "" {
			if !filepath.IsAbs(ruby) && m.Root() != "" {
				ruby = filepath.Join(m.Root(), ruby)
			}
			cfg.Command = filepath.Clean(ruby)
		}
	case "go":
		if goRoot := strings.TrimSpace(m.nativeRuntimeString(
			settingsPageGo, "languages.go.goroot",
		)); goRoot != "" {
			cfg.Env["GOROOT"] = goRoot
		}
		if goPath := strings.TrimSpace(m.nativeRuntimeString(
			settingsPageGo, "languages.go.gopath",
		)); goPath != "" {
			cfg.Env["GOPATH"] = goPath
		}
	}
}

type contextFileRole string

const (
	contextRoleTest   contextFileRole = "test"
	contextRoleMain   contextFileRole = "main"
	contextRoleScript contextFileRole = "script"
)

func contextRunConfigFor(root, absolute, text string, configs []openidea.RunConfig) (*openidea.RunConfig, error) {
	rel := relPathUnder(root, absolute)
	role := contextRoleFor(rel, text, runtimeIsWindows(), hasDjangoTestConfig(configs))
	switch role {
	case contextRoleTest:
		if cfg := contextTestConfig(rel, configs, text); cfg != nil {
			return cfg, nil
		}
		return nil, errors.New("The project has no run configuration for this test")
	case contextRoleMain:
		if cfg, ok := contextMainConfig(rel, text, configs); ok {
			return cfg, nil
		}
		if cfg := contextScriptConfig(rel, configs, text); cfg != nil {
			return cfg, nil
		}
	case contextRoleScript:
		if cfg := contextScriptConfig(rel, configs, text); cfg != nil {
			return cfg, nil
		}
	}
	return nil, errors.New("The active file has no runnable element, test or run configuration")
}

// contextRoleFor is OpenIDEA's fileRole: test names win over main and script
// entries because the closest test setup, not the raw interpreter, owns them.
func contextRoleFor(rel, text string, windows, django bool) contextFileRole {
	name := path.Base(rel)
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".cs") && csharpTestAttribute.MatchString(text) {
		return contextRoleTest
	}
	if webTestFile.MatchString(name) ||
		strings.Contains("/"+rel+"/", "/__tests__/") ||
		regexp.MustCompile(`(?i)^test_.*\.py$|_test\.py$`).MatchString(name) ||
		goTestFile.MatchString(name) ||
		strings.HasSuffix(name, "Test.php") ||
		regexp.MustCompile(`(?i)_(spec|test)\.rb$|^test_.*\.rb$`).MatchString(name) ||
		genericTestFile.MatchString(name) ||
		(strings.Contains("/"+rel+"/", "/tests/") && strings.HasSuffix(name, ".rs") && name != "mod.rs") ||
		(django && name == "tests.py") {
		return contextRoleTest
	}
	switch {
	case regexp.MustCompile(`(^|/)src/main\.rs$|(^|/)src/bin/[^/]+\.rs$`).MatchString(rel):
		return contextRoleMain
	case strings.HasSuffix(lower, ".go") && goMainPackage.MatchString(text):
		return contextRoleMain
	case strings.HasSuffix(lower, ".java") && javaMainDeclaration.MatchString(text):
		return contextRoleMain
	case strings.HasSuffix(lower, ".kt") && kotlinMainDeclaration.MatchString(text):
		return contextRoleMain
	case strings.HasSuffix(lower, ".py"), strings.HasSuffix(lower, ".rb"),
		strings.HasSuffix(lower, ".sh"), strings.HasSuffix(lower, ".bash"),
		strings.HasSuffix(lower, ".zsh"), strings.HasSuffix(lower, ".js"),
		strings.HasSuffix(lower, ".mjs"), strings.HasSuffix(lower, ".cjs"),
		strings.HasSuffix(lower, ".ts"), strings.HasSuffix(lower, ".php"):
		return contextRoleScript
	case windows && (strings.HasSuffix(lower, ".bat") || strings.HasSuffix(lower, ".cmd")):
		return contextRoleScript
	}
	return ""
}

func contextTestKinds(rel string) []string {
	lower := strings.ToLower(rel)
	switch {
	case strings.HasSuffix(lower, ".cs"):
		return []string{"dotnet-test"}
	case webSourceFile.MatchString(lower):
		return []string{"npm", "deno", "bun"}
	case strings.HasSuffix(lower, ".py"):
		return []string{"pytest", "python", "django-test"}
	case strings.HasSuffix(lower, ".php"):
		return []string{"phpunit"}
	case strings.HasSuffix(lower, "_spec.rb"):
		return []string{"rspec", "rake", "ruby"}
	case strings.HasSuffix(lower, "test.rb"):
		return []string{"rake", "rspec", "ruby"}
	case strings.HasSuffix(lower, ".go"):
		return []string{"go"}
	case strings.HasSuffix(lower, ".rs"):
		return []string{"cargo"}
	case jvmSourceFile.MatchString(lower):
		return []string{"gradle", "maven"}
	}
	return nil
}

func contextScope(cfg openidea.RunConfig) string {
	scope := ""
	switch cfg.Kind {
	case "dotnet", "dotnet-test":
		if project, ok := cfg.Params["project"].(string); ok {
			scope = path.Dir(strings.ReplaceAll(project, `\`, "/"))
		}
	case "go":
		if module, ok := cfg.Params["module"].(string); ok {
			scope = module
		} else if cfg.Module != nil {
			scope = *cfg.Module
		} else {
			scope = cfg.CwdOr("")
		}
	default:
		if cfg.Module != nil {
			scope = *cfg.Module
		} else {
			scope = cfg.CwdOr("")
		}
	}
	scope = strings.ReplaceAll(scope, `\`, "/")
	scope = strings.TrimPrefix(scope, "./")
	return strings.TrimRight(scope, "/")
}

func closestContextConfigs(configs []openidea.RunConfig, rel string) []openidea.RunConfig {
	best := -1
	scored := make([]struct {
		cfg   openidea.RunConfig
		score int
	}, 0, len(configs))
	for _, cfg := range configs {
		if strings.HasPrefix(cfg.ID, "context:") {
			continue
		}
		scope := contextScope(cfg)
		score := 0
		if scope != "" {
			switch {
			case rel == scope || strings.HasPrefix(rel, scope+"/"):
				score = len(scope)
			default:
				score = -1
			}
		}
		if score >= 0 {
			scored = append(scored, struct {
				cfg   openidea.RunConfig
				score int
			}{cfg: cfg, score: score})
			if score > best {
				best = score
			}
		}
	}
	out := make([]openidea.RunConfig, 0, len(scored))
	for _, item := range scored {
		if item.score == best {
			out = append(out, item.cfg)
		}
	}
	return out
}

func contextRelToCwd(rel string, cfg openidea.RunConfig) string {
	scope := strings.TrimRight(strings.TrimPrefix(cfg.CwdOr(""), "./"), "/")
	if cfg.Kind == "go" {
		scope = contextScope(cfg)
	}
	switch {
	case rel == scope:
		return ""
	case scope != "" && strings.HasPrefix(rel, scope+"/"):
		return strings.TrimPrefix(rel, scope+"/")
	default:
		return rel
	}
}

func deriveContextConfig(base openidea.RunConfig, rel string) openidea.RunConfig {
	out := base
	out.ID = "context:" + base.ID + ":" + rel
	out.Name = path.Base(rel)
	out.Detected = true
	out.Env = cloneStringMap(base.Env)
	out.EnvFiles = append([]string(nil), base.EnvFiles...)
	out.Params = cloneAnyMap(base.Params)
	return out
}

func contextTestBases(rel string, configs []openidea.RunConfig) []openidea.RunConfig {
	kinds := contextTestKinds(rel)
	order := make(map[string]int, len(kinds))
	for i, kind := range kinds {
		order[kind] = i
	}
	candidates := make([]openidea.RunConfig, 0)
	for _, cfg := range configs {
		if !cfg.IsTest || strings.HasPrefix(cfg.ID, "context:") {
			continue
		}
		if _, ok := order[cfg.Kind]; ok {
			candidates = append(candidates, cfg)
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return order[candidates[i].Kind] < order[candidates[j].Kind]
	})
	if closest := closestContextConfigs(candidates, rel); len(closest) > 0 {
		return closest
	}
	if cfg := implicitPytestConfig(rel, configs); cfg != nil {
		return []openidea.RunConfig{*cfg}
	}
	return nil
}

func contextTestConfig(rel string, configs []openidea.RunConfig, text string) *openidea.RunConfig {
	bases := contextTestBases(rel, configs)
	if len(bases) == 0 {
		return nil
	}
	base := bases[0]
	file := contextRelToCwd(rel, base)
	switch base.Kind {
	case "dotnet-test":
		if !csharpTestAttribute.MatchString(text) {
			return nil
		}
	case "npm", "bun":
		base.Args = adapter.AppendScriptArgs(base.Command, base.Args, []string{file})
	case "deno", "pytest", "python", "rspec", "django-test":
		base.Args = append(append([]string(nil), base.Args...), file)
	case "phpunit":
		base.Args = append(append([]string(nil), base.Args...), file)
	case "rake":
		task := ""
		if len(base.Args) > 0 {
			task = base.Args[len(base.Args)-1]
		}
		switch {
		case regexp.MustCompile(`(^|:)spec$`).MatchString(task):
			base.Args = append(append([]string(nil), base.Args...), "SPEC="+file)
		case regexp.MustCompile(`(^|:)test$`).MatchString(task):
			base.Args = append(append([]string(nil), base.Args...), "TEST="+file)
		default:
			return &base
		}
	case "ruby":
		if len(base.Args) > 0 {
			base.Args = append(append([]string(nil), base.Args[:len(base.Args)-1]...), file)
		}
	case "go":
		dir := path.Dir(file)
		target := "."
		if dir != "." {
			target = "./" + dir
		}
		parts := adapter.GoTestParts(base.Args)
		base.Args = append([]string{"test"}, parts.Flags...)
		base.Args = append(base.Args, target)
		base.Args = append(base.Args, parts.Program...)
		base.Name = "go test " + target
		if base.Params != nil {
			if _, ok := base.Params["goKind"]; ok {
				base.Params["scope"] = "directory"
				base.Params["target"] = target
			}
		}
	case "cargo":
		match := rustTestFile.FindStringSubmatch(rel)
		if len(match) == 0 {
			return &base
		}
		base.Args = append(append([]string(nil), base.Args...), "--test", match[1])
	case "gradle":
		filter := "*" + strings.TrimSuffix(path.Base(rel), path.Ext(rel))
		if class := jvmClassName(rel, text); class != "" {
			filter = class
		}
		base.Args = append(append([]string(nil), base.Args...), "--tests", filter)
	case "maven":
		filter := strings.TrimSuffix(path.Base(rel), path.Ext(rel))
		if class := jvmClassName(rel, text); class != "" {
			filter = class
		}
		if !hasArgPrefix(base.Args, "-Dtest=") {
			base.Args = append(append([]string(nil), base.Args...),
				"-Dtest="+filter, "-Dsurefire.failIfNoSpecifiedTests=false")
		}
	default:
		return &base
	}
	out := deriveContextConfig(base, rel)
	out.IsTest = true
	return &out
}

func contextMainConfig(rel, text string, configs []openidea.RunConfig) (*openidea.RunConfig, bool) {
	lower := strings.ToLower(rel)
	switch {
	case strings.HasSuffix(lower, ".java"), strings.HasSuffix(lower, ".kt"):
		target, ok := jvmContextTarget(rel, text)
		if !ok {
			return nil, false
		}
		cfg := applicationConfig(target, adapter.JVMModuleDir(rel, configs, nil))
		cfg.Cwd = nil
		cfg.Env = map[string]string{}
		cfg.Detected = true
		return cfg, true
	case strings.HasSuffix(lower, ".rs"):
		return rustContextMain(rel, configs)
	case strings.HasSuffix(lower, ".go"):
		return goContextMain(rel, configs)
	}
	return nil, false
}

func goContextMain(rel string, configs []openidea.RunConfig) (*openidea.RunConfig, bool) {
	var gos []openidea.RunConfig
	for _, cfg := range configs {
		if cfg.Kind == "go" && !strings.HasPrefix(cfg.ID, "context:") {
			gos = append(gos, cfg)
		}
	}
	closest := closestContextConfigs(gos, rel)
	if len(closest) == 0 {
		return nil, false
	}
	base := closest[0]
	for _, cfg := range closest {
		if len(cfg.Args) > 0 && cfg.Args[0] == "run" {
			base = cfg
			break
		}
	}
	dir := path.Dir(contextRelToCwd(rel, base))
	target := "."
	if dir != "." {
		target = "./" + dir
	}
	if base.Params != nil && base.Params["goKind"] == "build" {
		base.Params["scope"] = "package"
		base.Params["target"] = target
		base.Params["runAfterBuild"] = "true"
		base.Args = []string{"run", target}
		base.Name = "go run " + target
		base.IsTest = false
		return &base, true
	}
	for _, cfg := range closest {
		if contextScope(cfg) == contextScope(base) && len(cfg.Args) > 1 && cfg.Args[1] == target {
			return &cfg, true
		}
	}
	base.Args = []string{"run", target}
	base.Name = "go run " + target
	base.IsTest = false
	if base.Params != nil && base.Params["goKind"] == "test" {
		base.Params = nil
	}
	return &base, true
}

func rustContextMain(rel string, configs []openidea.RunConfig) (*openidea.RunConfig, bool) {
	var runs []openidea.RunConfig
	for _, cfg := range configs {
		if cfg.Kind == "cargo" && len(cfg.Args) > 0 && cfg.Args[0] == "run" {
			runs = append(runs, cfg)
		}
	}
	closest := closestContextConfigs(runs, rel)
	if len(closest) == 0 {
		return nil, false
	}
	if match := cargoBinFile.FindStringSubmatch(rel); len(match) > 0 {
		for _, cfg := range closest {
			if cfg.ID == "cargo:run:"+match[1] || containsArg(cfg.Args, match[1]) {
				return &cfg, true
			}
		}
	}
	return &closest[0], true
}

func contextScriptConfig(rel string, configs []openidea.RunConfig, text string) *openidea.RunConfig {
	base := openidea.RunConfig{
		ID:       "context:file:" + rel,
		Name:     path.Base(rel),
		Kind:     "shell",
		Env:      map[string]string{},
		Detected: true,
	}
	lower := strings.ToLower(rel)
	if strings.HasSuffix(lower, ".py") {
		command, prefix := projectPython(configs)
		if module := pythonModuleRun(rel, text, configs); module != nil {
			base.Kind = "python"
			base.Command = commandFromSubfolder(command, module.cwd)
			base.Args = append(prefix, "-m", module.module)
			if module.cwd != "" {
				cwd := module.cwd
				base.Cwd = &cwd
			}
			base.DebugAdapter = stringPtr("debugpy")
			return &base
		}
		base.Kind = "python"
		base.Command = command
		base.Args = append(prefix, rel)
		base.DebugAdapter = stringPtr("debugpy")
		return &base
	}
	switch {
	case strings.HasSuffix(lower, ".js"), strings.HasSuffix(lower, ".mjs"), strings.HasSuffix(lower, ".cjs"):
		base.Command = "node"
		base.Args = []string{rel}
		base.DebugAdapter = stringPtr("js-debug")
	case strings.HasSuffix(lower, ".ts"):
		base.Command = "npx"
		base.Args = []string{"tsx", rel}
		base.DebugAdapter = stringPtr("js-debug")
	case strings.HasSuffix(lower, ".php"):
		base.Kind = "php"
		base.Command = "php"
		base.Args = []string{rel}
		base.DebugAdapter = stringPtr("php-debug")
	case strings.HasSuffix(lower, ".zsh"):
		base.Command = "zsh"
		base.Args = []string{rel}
	case strings.HasSuffix(lower, ".bash"):
		base.Command = "bash"
		base.Args = []string{rel}
	case strings.HasSuffix(lower, ".sh"):
		base.Command = "sh"
		base.Args = []string{rel}
	case runtimeIsWindows() && (strings.HasSuffix(lower, ".bat") || strings.HasSuffix(lower, ".cmd")):
		base.Command = "cmd.exe"
		base.Args = []string{"/c", path.Base(rel)}
		if dir := path.Dir(rel); dir != "." {
			cwd := dir
			base.Cwd = &cwd
		}
	default:
		return nil
	}
	return &base
}

func projectPython(configs []openidea.RunConfig) (string, []string) {
	for _, cfg := range configs {
		if cfg.Kind != "python" {
			continue
		}
		end := len(cfg.Args)
		for i, arg := range cfg.Args {
			if arg == "-m" || strings.HasSuffix(arg, ".py") {
				end = i
				break
			}
		}
		if end == len(cfg.Args) {
			for i, arg := range cfg.Args {
				if pythonCommand.MatchString(arg) {
					end = i + 1
					break
				}
			}
		}
		return cfg.Command, append([]string(nil), cfg.Args[:end]...)
	}
	for _, cfg := range configs {
		if cfg.Kind != "pytest" {
			continue
		}
		venv := regexp.MustCompile(`^(.*[/\\](?:bin|Scripts)[/\\])pytest(\.exe)?$`)
		if match := venv.FindStringSubmatch(cfg.Command); len(match) > 0 {
			return match[1] + "python" + match[2], nil
		}
		for i := 0; i+1 < len(cfg.Args); i++ {
			if cfg.Args[i] == "-m" && cfg.Args[i+1] == "pytest" {
				return cfg.Command, append([]string(nil), cfg.Args[:i]...)
			}
		}
		if len(cfg.Args) >= 2 && cfg.Args[0] == "run" && cfg.Args[1] == "pytest" {
			return cfg.Command, []string{"run", "python"}
		}
	}
	if runtimeIsWindows() {
		return "python", nil
	}
	return "python3", nil
}

func implicitPytestConfig(rel string, configs []openidea.RunConfig) *openidea.RunConfig {
	if !strings.HasSuffix(strings.ToLower(rel), ".py") {
		return nil
	}
	var owners []openidea.RunConfig
	for _, cfg := range configs {
		switch cfg.Kind {
		case "python", "pytest", "django-test":
			owners = append(owners, cfg)
		}
	}
	owners = closestContextConfigs(owners, rel)
	command, prefix := projectPython(owners)
	cfg := &openidea.RunConfig{
		ID:           "pytest:implicit",
		Name:         "pytest",
		Kind:         "pytest",
		Command:      command,
		Args:         append(prefix, "-m", "pytest"),
		Env:          map[string]string{},
		Detected:     true,
		IsTest:       true,
		DebugAdapter: stringPtr("debugpy"),
	}
	if len(owners) > 0 {
		cfg.Cwd = owners[0].Cwd
		cfg.Module = owners[0].Module
		cfg.Env = cloneStringMap(owners[0].Env)
		cfg.EnvFiles = append([]string(nil), owners[0].EnvFiles...)
	}
	return cfg
}

type pythonModule struct {
	cwd    string
	module string
}

func pythonModuleRun(rel, text string, configs []openidea.RunConfig) *pythonModule {
	if !pythonRelativeImport.MatchString(text) {
		return nil
	}
	var owners []openidea.RunConfig
	for _, cfg := range configs {
		if cfg.Kind == "python" || cfg.Kind == "pytest" {
			owners = append(owners, cfg)
		}
	}
	project := ""
	if closest := closestContextConfigs(owners, rel); len(closest) > 0 {
		project = strings.TrimRight(strings.TrimPrefix(closest[0].CwdOr(""), "./"), "/")
	}
	segs := strings.Split(strings.TrimSuffix(rel, ".py"), "/")
	first := 0
	if project != "" && strings.HasPrefix(rel, project+"/") {
		first = len(strings.Split(project, "/"))
	}
	start := first
	for i := len(segs) - 2; i >= 0; i-- {
		if segs[i] == "src" && i >= first {
			start = i + 1
			break
		}
	}
	parts := append([]string(nil), segs[start:]...)
	if len(parts) < 2 {
		return nil
	}
	for _, part := range parts {
		if !pythonIdentifier.MatchString(part) {
			return nil
		}
	}
	if parts[len(parts)-1] == "__main__" || parts[len(parts)-1] == "__init__" {
		parts = parts[:len(parts)-1]
	}
	return &pythonModule{
		cwd:    strings.Join(segs[:start], "/"),
		module: strings.Join(parts, "."),
	}
}

func commandFromSubfolder(command, cwd string) string {
	if cwd == "" || !strings.Contains(command, "/") || strings.HasPrefix(command, "/") ||
		regexp.MustCompile(`^[A-Za-z]:`).MatchString(command) {
		return command
	}
	return strings.Repeat("../", len(strings.Split(cwd, "/"))) + command
}

func hasDjangoTestConfig(configs []openidea.RunConfig) bool {
	for _, cfg := range configs {
		if cfg.Kind == "django-test" {
			return true
		}
	}
	return false
}

func jvmContextTarget(rel, text string) (javaTarget, bool) {
	if !jvmSourceFile.MatchString(rel) {
		return javaTarget{}, false
	}
	target := javaTarget{test: strings.Contains("/"+rel+"/", "/src/test/")}
	switch {
	case strings.Contains("/"+rel+"/", "/src/main/"):
		target.module = strings.SplitN(rel, "/src/main/", 2)[0]
		if target.module == rel {
			target.module = ""
		}
	case strings.Contains("/"+rel+"/", "/src/test/"):
		target.module = strings.SplitN(rel, "/src/test/", 2)[0]
		if target.module == rel {
			target.module = ""
		}
	}
	name := strings.TrimSuffix(path.Base(rel), path.Ext(rel))
	if pkg := javaPackage(text); pkg != "" {
		name = pkg + "." + name
	}
	target.mainClass = name
	return target, name != ""
}

func jvmClassName(rel, text string) string {
	target, ok := jvmContextTarget(rel, text)
	if !ok {
		return ""
	}
	return target.mainClass
}

func runtimeIsWindows() bool {
	return goruntime.GOOS == "windows"
}

func stringPtr(value string) *string {
	return &value
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func hasArgPrefix(args []string, prefix string) bool {
	for _, arg := range args {
		if arg == prefix || strings.HasPrefix(arg, prefix) {
			return true
		}
	}
	return false
}

func cloneStringMap(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}
	out := make(map[string]string, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}

func cloneAnyMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	out := make(map[string]any, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}

// DebugContext is run.debugContext: derive the active file's configuration
// and send it through the same debugger path as a saved configuration.
func (m *Model) DebugContext() {
	if m.tab() == nil {
		m.debug.err = "Open a source file first"
		m.invalidate()
		return
	}
	if m.Root() == "" || m.ctx == nil {
		return
	}
	m.run.pendingDebugContext = true
	m.run.pendingContext = false
	m.run.pendingContextProfile = ""
	m.DockShow("debug")
	if !m.run.trusted {
		m.run.pendingTrust = true
		m.trustAsk = true
		m.status = "Trust this project to debug the active file"
		m.invalidate()
		return
	}
	if !m.run.loaded {
		m.loadRunConfigs()
		m.invalidate()
		return
	}
	m.startContextDebug()
}
