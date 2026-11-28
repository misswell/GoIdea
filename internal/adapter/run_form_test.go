package adapter

import (
	"reflect"
	"testing"

	"mayoi/internal/openidea"
)

func TestGoRunFormUsesBackendContract(t *testing.T) {
	cfg := &openidea.RunConfig{ID: "new", Kind: "go-test"}
	got := ApplyRunTypeForm(cfg, map[string]string{"goKind": "test", "scope": "package", "target": "./...", "programArgs": `-name "two words"`})
	if got.Kind != "go" || !got.IsTest || got.Params["programArgv"] != `["-name","two words"]` {
		t.Fatalf("incorrect backend configuration: %#v", got)
	}
	if !reflect.DeepEqual(got.Args, []string{"test", "./...", "-args", "-name", "two words"}) {
		t.Fatalf("test args = %v", got.Args)
	}
	template := ApplyRunTypeForm(&openidea.RunConfig{ID: "template:go-test", Kind: "go-test"}, map[string]string{"goKind": "test"})
	if template.Kind != "go-test" {
		t.Fatalf("template kind = %s", template.Kind)
	}
}

func TestRunTypeFormPreservesLegacyCommand(t *testing.T) {
	cases := []openidea.RunConfig{
		{Kind: "cargo", Command: "cargo", Args: []string{"run"}, Params: map[string]any{"subcommand": "run"}},
		{Kind: "program", Command: "echo hello"},
		{Kind: "godot", Command: "godot", Args: []string{"--path", "game"}},
		{Kind: "npm", Command: "npm", Args: []string{"run"}},
	}
	for _, cfg := range cases {
		if form := RunTypeForm(&cfg); form != nil {
			t.Errorf("%s: unreproducible command has form %v", cfg.Kind, form)
		}
	}
}

func TestGoBuildFormPreservesCwdAndProgramArgs(t *testing.T) {
	cwd := "old-dir"
	cfg := &openidea.RunConfig{Kind: "go", Cwd: &cwd}
	got := ApplyRunTypeForm(cfg, map[string]string{"goKind": "build", "target": ".", "module": "module", "runAfterBuild": "true", "programArgs": `"hello world"`})
	if got.CwdOr("") != cwd || got.Params["programArgv"] != `["hello world"]` {
		t.Fatalf("unexpected result: %#v", got)
	}
}
