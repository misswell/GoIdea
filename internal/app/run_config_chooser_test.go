package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"mayoi/internal/openidea"
)

func chooserFixture(t *testing.T) *Model {
	t.Helper()
	raw := mustJSON(t, []map[string]any{
		{"id": "maven:test", "name": "test", "kind": "maven", "command": "mvn", "args": []string{"test"}, "detected": true},
		{"id": "user:app", "name": "App", "kind": "application", "command": "java",
			"args": []string{"-cp", "${classpath}", "com.example.App"}, "module": "", "detected": true},
	})
	configs, err := openidea.DecodeRunConfigs(raw)
	if err != nil {
		t.Fatal(err)
	}
	m := &Model{postHook: func(fn func()) { fn() }, ctx: context.Background()}
	m.root = "/p"
	m.run.configs = configs
	m.run.loaded = true
	m.run.trusted = true
	return m
}

func TestRunChooserFiltersAndSelects(t *testing.T) {
	m := chooserFixture(t)
	m.OpenRunChooser("select")
	if m.chooser == nil || m.chooser.mode != "select" {
		t.Fatalf("the chooser opened in select mode")
	}
	if len(m.chooserRows()) != 2 {
		t.Fatalf("two rows without a query, got %d", len(m.chooserRows()))
	}
	m.chooser.query = "app"
	if len(m.chooserRows()) != 1 || m.chooserRows()[0].ID != "user:app" {
		t.Fatalf("the query filters by name and kind: %d", len(m.chooserRows()))
	}
	m.chooseActivate("select")
	if m.chooser != nil {
		t.Fatalf("selecting closes the chooser")
	}
	if m.run.sel < 0 || m.run.configs[m.run.sel].ID != "user:app" {
		t.Fatalf("the selection moved to the chosen configuration")
	}
}

func TestRunChooserRunsTheChosenConfiguration(t *testing.T) {
	m := chooserFixture(t)
	started := 0
	m.invokeHook = func(ctx context.Context, cmd string, args any) (json.RawMessage, error) {
		if cmd == "run_start" {
			started++
			return json.Marshal(openidea.RunInfo{ID: 1, Name: "test", Running: true})
		}
		if cmd == "java_resolve_launch" {
			return json.Marshal(openidea.JavaLaunchInfo{Build: strPtr("succeeded"), PathListSeparator: ":"})
		}
		return json.Marshal(nil)
	}
	m.OpenRunChooser("run")
	m.chooser.query = "test"
	m.chooseActivate("run")
	if m.chooser != nil {
		t.Fatalf("running closes the chooser")
	}
	for i := 0; i < 100 && started == 0; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if started != 1 {
		t.Fatalf("the chosen configuration starts, got %d starts", started)
	}
}

func TestDebugSelectedRefusesUndebuggableKinds(t *testing.T) {
	m := chooserFixture(t)
	m.run.sel = 0 // the maven configuration
	m.Debug()
	if m.debug.err == "" || !strings.Contains(m.debug.err, "Maven") {
		t.Fatalf("a non-JVM configuration is reported, got %q", m.debug.err)
	}
	if m.debug.starting {
		t.Fatalf("nothing starts for an undebuggable kind")
	}

	// A Kotlin adapter is not shipped: refused, not silently launched.
	raw := mustJSON(t, map[string]any{
		"id": "user:kt", "name": "Kt", "kind": "application", "command": "java",
		"args": []string{"-cp", "${classpath}", "a.MainKt"}, "debugAdapter": "kotlin-debug-adapter",
	})
	kt, err := openidea.DecodeRunConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	m.DebugRunConfig(kt)
	if m.debug.err == "" || !strings.Contains(m.debug.err, "kotlin-debug-adapter") {
		t.Fatalf("an unshipped adapter is reported, got %q", m.debug.err)
	}
}

func TestRunChooserDebugsTheCurrentFileRow(t *testing.T) {
	m := chooserFixture(t)
	m.OpenRunChooser("debug")
	if m.chooser.mode != "debug" {
		t.Fatalf("debug mode")
	}
	// The refusal path proves the current-file row reaches DebugContext.
	m.tab() // nothing open: DebugContext reports it
	m.chooseCurrentFile()
	if m.chooser != nil {
		t.Fatalf("the chooser closed")
	}
	if m.debug.err != "先打开一个 Java 源文件" {
		t.Fatalf("the current-file row debugs the context, got %q", m.debug.err)
	}
}
