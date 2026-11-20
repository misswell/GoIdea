package app

import (
	"path/filepath"
	"testing"

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
		{"shell", "scripts/build.zsh", "true\n", "shell", "zsh", []string{"scripts/build.zsh"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg, err := contextRunConfigFor(root, filepath.Join(root, test.rel), test.text, []openidea.RunConfig{python})
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Kind != test.kind || cfg.Command != test.command || !sameStrings(cfg.Args, test.args) {
				t.Fatalf("config = %s %s %q, want %s %s %q", cfg.Kind, cfg.Command, cfg.Args, test.kind, test.command, test.args)
			}
		})
	}
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
			if !sameStrings(cfg.Args, test.want) {
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
	if cfg.Kind != "go" || !sameStrings(cfg.Args, []string{"run", "./cmd/tool"}) || cfg.IsTest {
		t.Fatalf("go main config = %+v", cfg)
	}

	java, err := contextRunConfigFor(root, filepath.Join(root, "app/src/main/java/demo/App.java"),
		"package demo; class App { public static void main(String[] args) {} }", nil)
	if err != nil {
		t.Fatal(err)
	}
	if java.Kind != "application" || java.ID != "application:app:demo.App" ||
		!sameStrings(java.Args, []string{"-cp", "${classpath}", "demo.App"}) {
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
	if cfg.Kind != "pytest" || !cfg.IsTest || !sameStrings(cfg.Args, want) {
		t.Fatalf("pytest config = %+v", cfg)
	}
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
