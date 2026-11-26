package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

func TestContextRunProducesScriptConfigurations(t *testing.T) {
	root := t.TempDir()
	pythonDir := filepath.Join(root, "worker")
	python := openidea.RunConfig{
		ID:      "python:worker",
		Kind:    "python",
		Command: ".venv/bin/python",
		Args:    []string{"python", "-m", "manage"},
		Cwd:     &pythonDir,
	}
	tests := []struct {
		name    string
		rel     string
		text    string
		kind    string
		command string
		args    []string
	}{
		{"python", "tools/gen.py", "print('ok')", "python", ".venv/bin/python", []string{"python", "tools/gen.py"}},
		{"python module", "worker/src/shop/__main__.py", "from .models import Item\n", "python", "../../.venv/bin/python", []string{"python", "-m", "shop"}},
		{"node", "tools/run.js", "console.log('ok')", "shell", "node", []string{"tools/run.js"}},
		{"tsx", "tools/run.ts", "console.log('ok')", "shell", "npx", []string{"tsx", "tools/run.ts"}},
		{"php", "tools/run.php", "<?php echo 'ok';", "php", "php", []string{"tools/run.php"}},
		{"shell", "scripts/build.zsh", "true\n", "shell", "zsh", []string{"scripts/build.zsh"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg, err := contextRunConfigFor(root, filepath.Join(root, test.rel), test.text, []openidea.RunConfig{python})
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Kind != test.kind || cfg.Command != test.command || !sameStringSlice(cfg.Args, test.args) {
				t.Fatalf("config = %s %s %q, want %s %s %q", cfg.Kind, cfg.Command, cfg.Args, test.kind, test.command, test.args)
			}
		})
	}
}

func TestContextRunResolvesBareToolsThroughBackend(t *testing.T) {
	root := t.TempDir()
	t.Run("configured PHP", func(t *testing.T) {
		m := &Model{ctx: context.Background()}
		var commands []string
		m.invokeHook = func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			commands = append(commands, command)
			if command != "sdk_php_effective" {
				t.Fatalf("unexpected RPC %q", command)
			}
			return json.RawMessage(`"/opt/php8.3/bin/php"`), nil
		}
		cfg := openidea.RunConfig{Kind: "php", Command: "php"}
		if err := m.resolveContextRunTool(&cfg, filepath.Join(root, "tools/run.php")); err != nil {
			t.Fatal(err)
		}
		if cfg.Command != "/opt/php8.3/bin/php" {
			t.Fatalf("PHP command = %q", cfg.Command)
		}
		if len(commands) != 1 {
			t.Fatalf("commands = %q, want only sdk_php_effective", commands)
		}
	})

	t.Run("located npx", func(t *testing.T) {
		m := &Model{ctx: context.Background()}
		m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
			if command != "sdk_locate_tools" {
				t.Fatalf("unexpected RPC %q", command)
			}
			request := args.(map[string]any)
			if request["near"] != filepath.Join(root, "tools") {
				t.Fatalf("locate request = %#v", request)
			}
			if names := request["names"].([]string); len(names) != 1 || names[0] != "npx" {
				t.Fatalf("locate names = %#v", names)
			}
			return json.RawMessage(`{
				"tools":[{"name":"npx","path":"/repo/node_modules/.bin/npx","version":"10.8.0"}],
				"compileDir":"/tmp"
			}`), nil
		}
		cfg := openidea.RunConfig{Kind: "shell", Command: "npx"}
		if err := m.resolveContextRunTool(&cfg, filepath.Join(root, "tools/run.ts")); err != nil {
			t.Fatal(err)
		}
		if cfg.Command != "/repo/node_modules/.bin/npx" {
			t.Fatalf("npx command = %q", cfg.Command)
		}
	})

	t.Run("PHP on PATH", func(t *testing.T) {
		m := &Model{ctx: context.Background()}
		m.invokeHook = func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			switch command {
			case "sdk_php_effective":
				return json.RawMessage(`null`), nil
			case "sdk_locate_tools":
				return json.RawMessage(`{
					"tools":[{"name":"php","path":"/usr/bin/php","version":"8.3.0"}],
					"compileDir":"/tmp"
				}`), nil
			default:
				t.Fatalf("unexpected RPC %q", command)
				return nil, nil
			}
		}
		cfg := openidea.RunConfig{Kind: "php", Command: "php"}
		if err := m.resolveContextRunTool(&cfg, filepath.Join(root, "run.php")); err != nil {
			t.Fatal(err)
		}
		if cfg.Command != "/usr/bin/php" {
			t.Fatalf("fallback PHP command = %q", cfg.Command)
		}
	})
}

func TestContextRunNarrowsEachTestRunner(t *testing.T) {
	root := t.TempDir()
	tests := []struct {
		name  string
		rel   string
		text  string
		base  openidea.RunConfig
		want  []string
		debug string
	}{
		{
			name: "go", rel: "internal/calc/calc_test.go", text: "package calc",
			base: openidea.RunConfig{ID: "go:root", Kind: "go", Command: "go", Args: []string{"test", "./..."}, IsTest: true},
			want: []string{"test", "./internal/calc"},
		},
		{
			name: "cargo", rel: "tests/api.rs", text: "#[test] fn api() {}",
			base: openidea.RunConfig{ID: "cargo:root", Kind: "cargo", Command: "cargo", Args: []string{"test"}, IsTest: true},
			want: []string{"test", "--test", "api"},
		},
		{
			name: "npm", rel: "web/src/cart.test.ts", text: "test('cart', () => {})",
			base: openidea.RunConfig{ID: "npm:web", Kind: "npm", Command: "npm", Args: []string{"run", "test"}, IsTest: true},
			want: []string{"run", "test", "--", "web/src/cart.test.ts"},
		},
		{
			name: "maven", rel: "service/src/test/java/demo/CartTest.java", text: "package demo; class CartTest {}",
			base: openidea.RunConfig{ID: "maven:service", Kind: "maven", Command: "mvn", Args: []string{"test"}, IsTest: true},
			want: []string{"test", "-Dtest=demo.CartTest", "-Dsurefire.failIfNoSpecifiedTests=false"},
		},
		{
			name: "gradle", rel: "service/src/test/java/demo/CartTest.java", text: "package demo; class CartTest {}",
			base: openidea.RunConfig{ID: "gradle:service", Kind: "gradle", Command: "gradle", Args: []string{"test"}, IsTest: true},
			want: []string{"test", "--tests", "demo.CartTest"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg, err := contextRunConfigFor(root, filepath.Join(root, test.rel), test.text, []openidea.RunConfig{test.base})
			if err != nil {
				t.Fatal(err)
			}
			if !sameStringSlice(cfg.Args, test.want) {
				t.Fatalf("args = %q, want %q", cfg.Args, test.want)
			}
			if !cfg.IsTest || !cfg.Detected {
				t.Fatalf("context test was not marked as a detected test: %+v", cfg)
			}
		})
	}
}

func TestContextRunReusesMainTargets(t *testing.T) {
	root := t.TempDir()
	module := "api"
	goCfg := openidea.RunConfig{
		ID: "go:api", Kind: "go", Command: "go", Args: []string{"test", "./..."},
		Module: &module, IsTest: true,
	}
	cfg, err := contextRunConfigFor(root, filepath.Join(root, "api/cmd/tool/main.go"),
		"package main\nfunc main() {}\n", []openidea.RunConfig{goCfg})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Kind != "go" || !sameStringSlice(cfg.Args, []string{"run", "./cmd/tool"}) || cfg.IsTest {
		t.Fatalf("go main config = %+v", cfg)
	}

	java, err := contextRunConfigFor(root, filepath.Join(root, "app/src/main/java/demo/App.java"),
		"package demo; class App { public static void main(String[] args) {} }", nil)
	if err != nil {
		t.Fatal(err)
	}
	if java.Kind != "application" || java.ID != "application:app:demo.App" ||
		!sameStringSlice(java.Args, []string{"-cp", "${classpath}", "demo.App"}) {
		t.Fatalf("java main config = %+v", java)
	}
}

func TestContextRunFallsBackToImplicitPytest(t *testing.T) {
	root := t.TempDir()
	cfg, err := contextRunConfigFor(root, filepath.Join(root, "tests/test_math.py"), "def test_add(): pass\n", nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-m", "pytest", "tests/test_math.py"}
	if cfg.Kind != "pytest" || !cfg.IsTest || !sameStringSlice(cfg.Args, want) {
		t.Fatalf("pytest config = %+v", cfg)
	}
}

func TestContextRunUsesSelectedLanguageInterpretersAndGoEnvironment(t *testing.T) {
	root := t.TempDir()
	m := &Model{}
	m.setRoot(root)
	m.nativeSettings.pages = map[string]nativeSettingsPageState{
		settingsPagePythonDeps: {
			loaded: true, applied: map[string]any{
				"languages.python.interpreter": filepath.Join(root, ".venv", "bin", "python"),
			},
		},
		settingsPageNode: {
			loaded: true, applied: map[string]any{
				"languages.node.interpreter": filepath.Join(root, ".tools", "node"),
			},
		},
		settingsPagePHP: {
			loaded: true, applied: map[string]any{
				"languages.php.interpreter": "/opt/php/bin/php",
			},
		},
		settingsPageRuby: {
			loaded: true, applied: map[string]any{
				"languages.ruby.interpreter": "/opt/ruby/bin/ruby",
			},
		},
		settingsPageGo: {
			loaded: true, applied: map[string]any{
				"languages.go.goroot": "/opt/go",
				"languages.go.gopath": filepath.Join(root, ".gopath"),
			},
		},
	}
	for _, test := range []struct {
		kind, command, want string
	}{
		{"python", "python3", filepath.Join(root, ".venv", "bin", "python")},
		{"node", "node", filepath.Join(root, ".tools", "node")},
		{"php", "php", "/opt/php/bin/php"},
		{"ruby", "ruby", "/opt/ruby/bin/ruby"},
	} {
		cfg := &openidea.RunConfig{Kind: test.kind, Command: test.command}
		m.applyContextLanguageSettings(cfg)
		if cfg.Command != test.want {
			t.Fatalf("%s context command = %q, want %q", test.kind, cfg.Command, test.want)
		}
	}
	goCfg := &openidea.RunConfig{Kind: "go", Command: "go"}
	m.applyContextLanguageSettings(goCfg)
	if goCfg.Env["GOROOT"] != "/opt/go" || goCfg.Env["GOPATH"] != filepath.Join(root, ".gopath") {
		t.Fatalf("Go context environment = %#v", goCfg.Env)
	}
}

func TestContextRunWithCoveragePreparesLaunchesAndCollects(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	root := t.TempDir()
	path := filepath.Join(root, "internal/calc/calc_test.go")
	doc := writableSourceDocument(path, "package calc\n")
	posts := make(chan func(), 16)
	m := &Model{
		ctx:    ctx,
		tabs:   []*Tab{{doc: doc, state: &editor.State{Buffer: doc.Buffer()}}},
		active: 0,
		postHook: func(fn func()) {
			posts <- fn
		},
	}
	m.setRoot(root)
	m.run.trusted, m.run.loaded = true, true
	m.run.configs = []openidea.RunConfig{{
		ID: "go:root", Name: "Go Tests", Kind: "go", Command: "go",
		Args: []string{"test", "./..."}, IsTest: true,
	}}

	const report = `{"kind":"goProfile","path":"/tmp/context-cover.out","module":"example.com/context","moduleDir":"/tmp"}`
	var prepared openidea.RunConfig
	var launched openidea.RunConfig
	m.invokeHook = func(_ context.Context, command string, input any) (json.RawMessage, error) {
		args, _ := input.(map[string]any)
		switch command {
		case "coverage_prepare":
			config, ok := args["config"].(json.RawMessage)
			if !ok {
				return nil, fmt.Errorf("coverage config payload is %T", args["config"])
			}
			if err := json.Unmarshal(config, &prepared); err != nil {
				return nil, err
			}
			launch, err := json.Marshal(prepared)
			if err != nil {
				return nil, err
			}
			return json.RawMessage(fmt.Sprintf(
				`{"status":"ready","runner":"go test -cover","launch":%s,"report":%s}`,
				launch, report,
			)), nil
		case "run_start":
			config, ok := args["config"].(json.RawMessage)
			if !ok {
				return nil, fmt.Errorf("run config payload is %T", args["config"])
			}
			if err := json.Unmarshal(config, &launched); err != nil {
				return nil, err
			}
			return json.RawMessage(`{"id":42,"running":true,"name":"go test ./internal/calc"}`), nil
		case "coverage_collect":
			if raw := args["report"].(json.RawMessage); string(raw) != report {
				return nil, fmt.Errorf("coverage report payload changed: %s", raw)
			}
			return json.RawMessage(`{"runner":"go test -cover","hasBranches":false,"files":[{"path":"internal/calc/calc.go","linesTotal":5,"linesCovered":4}]}`), nil
		case "settings_get":
			return json.RawMessage(`{"runTargets":[],"run":{}}`), nil
		default:
			return nil, fmt.Errorf("unexpected RPC %q", command)
		}
	}

	m.RunContextConfigurationWithCoverage()
	exitSent := false
	for m.coverage.activeSuite() == nil {
		select {
		case fn := <-posts:
			fn()
			if !exitSent && m.run.runID == 42 && m.run.awaiting {
				code := int32(0)
				m.acceptExit(openidea.RunExitedEvent{Run: 42, Code: &code})
				exitSent = true
			}
		case <-ctx.Done():
			t.Fatalf("context coverage did not finish: run=%s coverage=%s", m.run.err, m.coverage.err)
		}
	}

	if prepared.Kind != "go" || prepared.Name != "calc_test.go" ||
		!prepared.IsTest || !sameStringSlice(prepared.Args, []string{"test", "-json", "./internal/calc"}) {
		t.Fatalf("prepared context config = %+v", prepared)
	}
	if launched.ID != prepared.ID || !sameStringSlice(launched.Args, prepared.Args) {
		t.Fatalf("coverage launch = %+v, want prepared %+v", launched, prepared)
	}
	suite := m.coverage.activeSuite()
	if suite.name != "calc_test.go" || suite.totals.LinesCovered != 4 ||
		suite.totals.LinesTotal != 5 || m.run.running {
		t.Fatalf("context coverage suite = %+v, running = %v", suite, m.run.running)
	}
}
