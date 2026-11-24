package app

import (
	"testing"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
)

func TestDockerGutterActionsMatchOpenIDEA(t *testing.T) {
	m := &Model{root: "/project"}
	m.services.docker.settings.Connections = []openidea.DockerConnection{{
		ID: "remote", Name: "Remote",
	}}

	staged := m.dockerGutterActions(dockerTarget{
		kind: "build", line: 0, stage: "builder",
	}, "/project/docker/api/Dockerfile")
	if len(staged) != 1 || staged[0].title != "Build Stage 'builder'" {
		t.Fatalf("staged actions = %+v", staged)
	}
	cfg := staged[0].config
	if cfg.Kind != "docker-dockerfile" ||
		cfg.Params["dockerfile"] != "docker/api/Dockerfile" ||
		cfg.Params["context"] != "docker/api" ||
		cfg.Params["target"] != "builder" ||
		cfg.Params["connection"] != "remote" {
		t.Fatalf("staged configuration = %+v", cfg)
	}
	if cfg.Command == "" || cfg.StopCommand != nil {
		t.Fatalf("staged command = %q, stop=%v", cfg.Command, cfg.StopCommand)
	}

	final := m.dockerGutterActions(dockerTarget{
		kind: "build", line: 4, last: true,
	}, "/project/docker/api/Dockerfile")
	if len(final) != 2 ||
		final[0].title != "Run 'Dockerfile'" ||
		final[1].title != "Build Image 'api:latest'" {
		t.Fatalf("final actions = %+v", final)
	}
	if final[0].config.Params["runImage"] != "true" ||
		final[1].config.Params["runImage"] != nil {
		t.Fatalf("final configs = %+v", final)
	}

	compose := m.dockerGutterActions(dockerTarget{
		kind: "composeService", line: 8, service: "api",
	}, "/project/compose.yaml")
	if len(compose) != 1 || compose[0].title != "Compose Up 'api'" ||
		compose[0].config.Params["files"] != "compose.yaml" ||
		compose[0].config.Params["services"] != "api" {
		t.Fatalf("compose actions = %+v", compose)
	}
}

func TestRunConfigurationActionArgAndTemporaryAdoption(t *testing.T) {
	cfg := openidea.RunConfig{
		ID: "docker:compose:compose.yaml:api", Name: "compose.yaml: api",
		Kind: "docker-compose", Command: "docker compose up api",
		Args: []string{}, Env: map[string]string{},
		Params: map[string]any{"files": "compose.yaml", "services": "api"},
	}
	got, temporary, ok := runConfigFromActionArg(map[string]any{
		"config": cfg, "temporary": true,
	})
	if !ok || !temporary || got.ID != cfg.ID {
		t.Fatalf("wrapped argument = %+v, temporary=%v, ok=%v", got, temporary, ok)
	}
	if !hiddenActionIDs["run.runConfig"] {
		t.Fatal("run.runConfig is not hidden from the command palette")
	}

	m := &Model{root: "/project"}
	m.run.loaded = true
	m.RunConfiguration(map[string]any{"config": cfg, "temporary": true})
	if !m.trustAsk || !m.run.pendingTrust || m.run.inline == nil ||
		m.run.pendingInline == nil || m.run.inline.ID != cfg.ID {
		t.Fatalf("temporary adoption = run=%+v ask=%v", m.run, m.trustAsk)
	}
}

func TestDockerGutterRendersNativeRunMenu(t *testing.T) {
	const file = "/project/Dockerfile"
	tab := &Tab{doc: docbridge.Virtual(file, "FROM alpine AS builder\nRUN true\nFROM scratch\n")}
	tab.state = &editor.State{
		Buffer: tab.doc.Buffer(), LineHeight: editorLineHeight(13),
	}
	tab.state.Font.Size = 13
	m := &Model{tabs: []*Tab{tab}, active: 0}
	m.setRoot("/project")
	tt := workbenchTester(t, m)

	if _, ok := tt.Find("Build Stage 'builder'"); !ok {
		t.Fatalf("Dockerfile gutter marker missing: %q", tt.Texts())
	}
	if err := tt.Click("Build Stage 'builder'"); err != nil {
		t.Fatal(err)
	}
	menu := tt.Menu()
	want := map[string]bool{
		"Build Stage 'builder'":     false,
		"-":                         false,
		"Modify Run Configuration…": false,
	}
	for _, item := range menu {
		if _, ok := want[item]; ok {
			want[item] = true
		}
	}
	for item, found := range want {
		if !found {
			t.Fatalf("Docker gutter menu %v lacks %q", menu, item)
		}
	}
}
