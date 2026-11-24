package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"mayoi/internal/openidea"
)

func TestPrepareGradleJVMLaunchAddsTheBackendInitScript(t *testing.T) {
	cfg, err := openidea.DecodeRunConfig(mustJSON(t, map[string]any{
		"id": "gradle:bootRun", "name": "bootRun", "kind": "gradle",
		"command": "./gradlew", "args": []string{"bootRun"},
		"params": map[string]any{
			"tasks": "bootRun", "wrapper": "true",
			"vmOptions": "-Xmx512m", "activeProfiles": "dev",
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	invoked := false
	m := &Model{
		ctx: context.Background(),
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command != "run_gradle_jvm_args_script" {
				t.Fatalf("unexpected RPC %q", command)
			}
			invoked = true
			args := input.(map[string]any)["jvmArgs"].([]string)
			if strings.Join(args, " ") != "-Xmx512m -Dspring.profiles.active=dev" {
				t.Fatalf("jvmArgs = %#v", args)
			}
			return json.RawMessage(`"/tmp/goidea-gradle-jvm.gradle"`), nil
		},
	}
	raw, err := m.prepareGradleJVMLaunch(context.Background(), cfg.Raw(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !invoked {
		t.Fatal("the Gradle JVM init script was not requested")
	}
	prepared, err := openidea.DecodeRunConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--init-script", "/tmp/goidea-gradle-jvm.gradle", "bootRun"}
	if strings.Join(prepared.Args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("prepared args = %#v, want %#v", prepared.Args, want)
	}
}

func TestPrepareGradleJVMLaunchRejectsRunTargets(t *testing.T) {
	cfg, err := openidea.DecodeRunConfig(mustJSON(t, map[string]any{
		"id": "gradle:test", "name": "test", "kind": "gradle",
		"command": "./gradlew", "args": []string{"test"}, "target": "ssh:dev",
		"params": map[string]any{"tasks": "test", "vmOptions": "-Xmx256m"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	m := &Model{ctx: context.Background()}
	if _, err := m.prepareGradleJVMLaunch(context.Background(), cfg.Raw(), cfg); err == nil {
		t.Fatal("Gradle VM options on a run target must be refused")
	}
}
