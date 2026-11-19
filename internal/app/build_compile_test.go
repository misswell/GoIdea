package app

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/egoist/mygo/ui/editor"
	"mayoi/internal/docbridge"
)

func TestCompileFileRPCAndLifecycle(t *testing.T) {
	type call struct {
		command string
		args    map[string]any
	}
	calls := make(chan call, 1)
	finished := make(chan struct{}, 1)
	doc := docbridge.Virtual("/project/src/App.java", "class App {}")
	m := &Model{ctx: context.Background(), tabs: []*Tab{{doc: doc, state: &editor.State{Buffer: doc.Buffer()}}}}
	m.setRoot("/project")
	m.run.trusted = true
	m.build.files = []string{"/project/Old.java"}
	m.postHook = func(fn func()) { fn(); finished <- struct{}{} }
	m.invokeHook = func(_ context.Context, cmd string, args any) (json.RawMessage, error) {
		calls <- call{cmd, args.(map[string]any)}
		return json.RawMessage(`{"success":true,"messages":[]}`), nil
	}
	m.CompileFile()
	var got call
	select {
	case got = <-calls:
	case <-time.After(5 * time.Second):
		t.Fatal("compile RPC not called")
	}
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("build did not finish")
	}
	if got.command != "build_compile_file" {
		t.Fatalf("command = %q", got.command)
	}
	if file, ok := got.args["file"].(*string); !ok || *file != doc.Path() {
		t.Fatalf("file = %v", got.args["file"])
	}
	if _, exists := got.args["kind"]; exists {
		t.Fatal("single-file RPC must not include kind")
	}
	if _, exists := got.args["cmake"]; exists {
		t.Fatal("single-file RPC must not include cmake")
	}
	if clear := got.args["clear"].([]string); len(clear) != 1 || clear[0] != "/project/Old.java" {
		t.Fatalf("clear = %v", clear)
	}
	if got.args["request"] == uint32(0) || got.args["onEvent"].(map[string]any)["$channel"] == uint32(0) {
		t.Fatal("missing request or channel")
	}
	if m.build.busy || m.build.success == nil || !*m.build.success {
		t.Fatal("compile result did not finish successfully")
	}
}

func TestCompileFileGates(t *testing.T) {
	m := &Model{}
	m.setRoot("/project")
	m.CompileFile()
	m.CompileFilePath("/project/readme.md")
	if m.build.visible {
		t.Fatal("missing or non-Java file started a build")
	}
	m.CompileFilePath("/project/App.java")
	if !m.trustAsk || m.build.busy {
		t.Fatal("untrusted project must request trust")
	}
	m.trustAsk = false
	m.build.busy = true
	m.CompileFilePath("/project/App.java")
	if m.trustAsk || !m.build.busy {
		t.Fatal("running build must not be replaced")
	}
}

func TestBuildInvocationKeepsEmptyDiagnosticsClearArray(t *testing.T) {
	_, args := buildInvocation("compileFile", new(string), 1, 2, []string{})
	data, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if string(decoded["clear"]) != "[]" {
		t.Fatalf("clear = %s", decoded["clear"])
	}
}
