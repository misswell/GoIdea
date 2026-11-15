package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"mayoi/internal/adapter"
	"mayoi/internal/openidea"
)

// fakeInvoke is the injectable RPC client: resolveJVMConfig must stop the
// launch on every failure, so no test here ever reaches run_start.
type fakeInvoke struct {
	resolves []string
	calls    []string
}

func (f *fakeInvoke) hook(ctx context.Context, cmd string, args any) (json.RawMessage, error) {
	f.calls = append(f.calls, cmd)
	switch cmd {
	case "find_files":
		return json.Marshal([]openidea.FileMatch{})
	case "fs_read_text":
		return json.Marshal("")
	case "run_configs":
		return json.Marshal([]openidea.RunConfig{})
	case "java_resolve_launch":
		f.resolves = append(f.resolves, args.(map[string]any)["module"].(string))
		return json.Marshal(openidea.JavaLaunchInfo{
			Build: ptr("succeeded"), ClassPaths: []string{"/p/app/target/classes"},
			ModulePaths: []string{}, PathListSeparator: ":", JavaExec: ptr("/jdk/bin/java"),
		})
	}
	return nil, errors.New("unexpected invoke: " + cmd)
}

func ptr(s string) *string { return &s }

func runConfigJSON(t *testing.T, cfg map[string]any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestJavaMainClassFromArgs(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"-cp", "${classpath}", "demo.App"}, "demo.App"},
		{[]string{"-Xmx1g", "--add-opens", "java.base/java.lang=ALL-UNNAMED", "a.Main", "--port", "1"}, "a.Main"},
		{[]string{"-jar", "app.jar"}, ""},
		{nil, ""},
	}
	for _, c := range cases {
		if got := adapter.ParseJavaArgs(c.args).MainClass; got != c.want {
			t.Fatalf("ParseJavaArgs(%v).MainClass = %q, want %q", c.args, got, c.want)
		}
	}
}

func TestResolveJVMConfigResolvesTheCommandLine(t *testing.T) {
	fake := &fakeInvoke{}
	m := &Model{invokeHook: fake.hook}
	raw := runConfigJSON(t, map[string]any{
		"id": "user:main", "name": "Main", "kind": "application", "command": "java",
		"args":   []string{"-Xmx1g", "-cp", "${classpath}", "com.example.App", "serve"},
		"module": "app", "env": map[string]string{"FOO": "bar"}, "cwd": "app",
	})
	resolved, err := m.resolveJVMConfig(context.Background(), raw, "/p", func(text, kind string) {})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(resolved, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg["command"] != "/jdk/bin/java" {
		t.Fatalf("command = %v, want the resolved javaExec", cfg["command"])
	}
	args := cfg["args"].([]any)
	joined := make([]string, len(args))
	for i, a := range args {
		joined[i] = a.(string)
	}
	want := []string{"-Xmx1g", "-cp", "/p/app/target/classes", "com.example.App", "serve"}
	if strings.Join(joined, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("args = %v, want %v", joined, want)
	}
	// Every other field survives the round trip.
	if cfg["env"].(map[string]any)["FOO"] != "bar" || cfg["cwd"] != "app" {
		t.Fatalf("fields the launch does not touch must survive: %v", cfg)
	}
}

func TestResolveJVMConfigStopsOnResolveFailure(t *testing.T) {
	calls := 0
	m := &Model{invokeHook: func(ctx context.Context, cmd string, args any) (json.RawMessage, error) {
		if cmd == "java_resolve_launch" {
			calls++
			return nil, errors.New("jdtls: Module app does not belong to any java project")
		}
		return json.Marshal([]openidea.RunConfig{})
	}}
	raw := runConfigJSON(t, map[string]any{
		"id": "user:main", "name": "Main", "kind": "application", "command": "java",
		"args": []string{"-cp", "${classpath}", "com.example.App"}, "module": "app",
	})
	_, err := m.resolveJVMConfig(context.Background(), raw, "/p", func(text, kind string) {})
	if err == nil || !strings.Contains(err.Error(), "尚未导入") {
		t.Fatalf("an unimported module must stop the launch with an explanation, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("java_resolve_launch must be attempted once, got %d", calls)
	}
}

func TestResolveJVMConfigKeepsOtherKinds(t *testing.T) {
	m := &Model{invokeHook: func(ctx context.Context, cmd string, args any) (json.RawMessage, error) {
		return nil, errors.New("nothing must be invoked for another kind")
	}}
	raw := runConfigJSON(t, map[string]any{
		"id": "maven:test", "name": "test", "kind": "maven", "command": "mvn", "args": []string{"test"},
	})
	resolved, err := m.resolveJVMConfig(context.Background(), raw, "/p", nil)
	if err != nil || string(resolved) != string(raw) {
		t.Fatalf("a non-JVM configuration must pass through unchanged, got %v / %v", resolved, err)
	}
}

func TestResolveJVMConfigReportsToTheRunConsole(t *testing.T) {
	fake := &fakeInvoke{}
	m := &Model{invokeHook: fake.hook, postHook: func(fn func()) { fn() }}
	m.root = "/p"
	m.run.channel = 7
	raw := runConfigJSON(t, map[string]any{
		"id": "user:main", "name": "Main", "kind": "application", "command": "java",
		"args": []string{"-cp", "${classpath}", "com.example.App"}, "module": "app",
	})
	if _, err := m.resolveJVMConfig(context.Background(), raw, "/p", m.reportRunConsole("/p", 7)); err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	// RunOnMain executes inline on the test's goroutine, so the report landed.
	if len(m.run.output) == 0 || !strings.Contains(m.run.output[0].text, "com.example.App") {
		t.Fatalf("the preparation progress must reach the run console, got %v", m.run.output)
	}
}
