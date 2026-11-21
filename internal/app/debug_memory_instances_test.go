package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestDebugMemoryInspectValueAndReference(t *testing.T) {
	var result debugMemoryInspectResult
	raw := `{"status":"ready","generation":7,"instances":[{"kind":"object","id":4294967297,"typeName":"a.Example"},{"kind":"string","id":12,"text":"hello","length":5},{"kind":"primitive","typeName":"int","text":"42"}]}`
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatal(err)
	}
	want := []string{"Example@4294967297", `"hello"`, "42"}
	for i, value := range result.Instances {
		if got := debugMemoryValueText(value); got != want[i] {
			t.Fatalf("value %d = %q, want %q", i, got, want[i])
		}
		ref, ok := debugMemoryValueRef(result.Generation, value)
		if i < 2 && (!ok || ref.Generation != 7 || ref.ID != value.ID) {
			t.Fatalf("object reference = %+v, %v", ref, ok)
		}
		if i == 2 && ok {
			t.Fatal("primitive became an object reference")
		}
	}
}

func TestDebugMemoryInspectDropsReplyAfterResume(t *testing.T) {
	calls := make(chan map[string]any, 1)
	respond := make(chan struct{})
	posted := make(chan func(), 1)
	m := &Model{
		ctx:      context.Background(),
		postHook: func(f func()) { posted <- f },
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command != "debug_jvm_memory_instances" {
				t.Errorf("unexpected command %s", command)
			}
			calls <- input.(map[string]any)
			<-respond
			return json.RawMessage(`{"status":"ready","generation":8,"instances":[{"kind":"object","id":1,"typeName":"Test"}]}`), nil
		},
	}
	m.debug.session = 42
	m.debug.paused = true
	m.showDebugMemoryInstances("Test")
	args := <-calls
	if args["session"] != uint32(42) || args["className"] != "Test" || args["limit"] != debugMemoryInstanceLimit {
		t.Fatalf("instances arguments = %#v", args)
	}
	inspection := m.debug.memory.inspection
	m.acceptDebugEvent(42, json.RawMessage(`{"kind":"continued"}`))
	close(respond)
	select {
	case callback := <-posted:
		callback()
	case <-time.After(5 * time.Second):
		t.Fatal("request did not finish")
	}
	if !inspection.outdated || len(inspection.instances) != 0 {
		t.Fatalf("stale response was retained: %+v", inspection)
	}
}

func TestDebugMemoryDisplayRowsBoundCyclesAndShowTruncation(t *testing.T) {
	value := debugMemoryValue{Kind: "object", ID: 3, TypeName: "Test"}
	ref, _ := debugMemoryValueRef(9, value)
	node := &debugMemoryNode{ref: ref, expanded: true, fieldsLoaded: true,
		fields:        []debugMemoryField{{Name: "self", Value: value}},
		showReferrers: true, referrersLoaded: true, referrersTruncated: true}
	inspection := debugMemoryInspection{generation: 9, instances: []debugMemoryValue{value},
		nodes: map[debugMemoryObjectRef]*debugMemoryNode{ref: node}}
	rows := inspection.displayRows()
	if len(rows) != 4 || rows[1].name != "self" || rows[2].text != "Referrers" || !strings.Contains(rows[3].text, "first") {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestDebugMemoryFieldsAndReferrersUseGeneration(t *testing.T) {
	type request struct {
		command string
		args    map[string]any
	}
	requests := make(chan request, 2)
	callbacks := make(chan func(), 2)
	m := &Model{
		ctx:      context.Background(),
		postHook: func(f func()) { callbacks <- f },
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			requests <- request{command, input.(map[string]any)}
			switch command {
			case "debug_jvm_memory_fields":
				return json.RawMessage(`{"status":"ready","fields":[{"name":"count","value":{"kind":"primitive","typeName":"int","text":"1"}}]}`), nil
			case "debug_jvm_memory_referrers":
				return json.RawMessage(`{"status":"ready","referrers":[{"value":{"kind":"object","id":2,"typeName":"Holder"},"via":["target"]}],"truncated":true}`), nil
			}
			t.Errorf("unexpected command %s", command)
			return nil, nil
		},
	}
	m.debug.session, m.debug.paused = 42, true
	ref := debugMemoryObjectRef{Generation: 123, ID: 7}
	node := &debugMemoryNode{ref: ref}
	inspection := &debugMemoryInspection{session: 42, generation: 123, nodes: map[debugMemoryObjectRef]*debugMemoryNode{ref: node}}
	m.debug.memory.inspection = inspection

	m.debugMemoryLoadNode(inspection, node, false)
	m.debugMemoryLoadNode(inspection, node, true)
	for range 2 {
		select {
		case call := <-requests:
			if call.args["session"] != uint32(42) || call.args["object"] != ref {
				t.Fatalf("%s arguments = %#v", call.command, call.args)
			}
			if call.command == "debug_jvm_memory_referrers" && call.args["limit"] != debugMemoryReferrerLimit {
				t.Fatalf("referrer limit = %#v", call.args["limit"])
			}
		case <-time.After(5 * time.Second):
			t.Fatal("request was not sent")
		}
	}
	for range 2 {
		select {
		case callback := <-callbacks:
			callback()
		case <-time.After(5 * time.Second):
			t.Fatal("request did not finish")
		}
	}
	if !node.fieldsLoaded || len(node.fields) != 1 || node.fields[0].Name != "count" {
		t.Fatalf("fields = %+v", node)
	}
	if !node.referrersLoaded || len(node.referrers) != 1 || !node.referrersTruncated || node.referrers[0].Via[0] != "target" {
		t.Fatalf("referrers = %+v", node)
	}
}

func TestDebugMemoryInstancePaneRendersAndExpands(t *testing.T) {
	ref := debugMemoryObjectRef{Generation: 5, ID: 11}
	value := debugMemoryValue{Kind: "object", ID: ref.ID, TypeName: "com.example.Holder"}
	text := "12"
	node := &debugMemoryNode{ref: ref, fieldsLoaded: true,
		fields: []debugMemoryField{{Name: "count", Value: debugMemoryValue{Kind: "primitive", Text: &text}}}}
	m := &Model{}
	m.debug.session, m.debug.paused = 4, true
	m.debug.memory.snapshot.Rows = []debugMemoryClass{{Name: "com.example.Holder", Instances: 1}}
	m.debug.memory.inspection = &debugMemoryInspection{session: 4, generation: 5,
		className: "com.example.Holder", instances: []debugMemoryValue{value},
		nodes: map[debugMemoryObjectRef]*debugMemoryNode{ref: node}}
	tester := ui.NewTester(func(c *ui.Context) {
		p := c.Theme()
		m.debugMemory(c, p, oiPaletteOf(p))
	}, 1000, 500)
	tester.Frame()
	if !tester.HasText("Instances of com.example.Holder") || !tester.HasText("Holder@11") {
		t.Fatalf("instance pane is missing its content: %q", tester.Texts())
	}
	if err := tester.Click("Holder@11"); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	if !tester.HasText("count = 12") {
		t.Fatalf("field is missing after expansion: %q", tester.Texts())
	}
}

func TestDebugMemoryClassOpensInstances(t *testing.T) {
	calls := make(chan string, 1)
	m := &Model{ctx: context.Background(), postHook: func(f func()) { f() }}
	m.invokeHook = func(_ context.Context, command string, input any) (json.RawMessage, error) {
		calls <- command
		return json.RawMessage(`{"status":"ready","generation":1,"instances":[]}`), nil
	}
	m.debug.session, m.debug.paused = 4, true
	m.debug.memory.loaded = true
	m.debug.memory.snapshot.Rows = []debugMemoryClass{{Name: "com.example.Widget", Instances: 1}}
	tester := ui.NewTester(func(c *ui.Context) {
		p := c.Theme()
		m.debugMemory(c, p, oiPaletteOf(p))
	}, 1000, 500)
	tester.Frame()
	if err := tester.Click("Show Instances"); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	select {
	case command := <-calls:
		if command != "debug_jvm_memory_instances" {
			t.Fatalf("command = %s", command)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Show Instances did not request instances")
	}
}

func TestDebugMemoryJumpToSourceUsesExactPackageSymbol(t *testing.T) {
	requests := make(chan map[string]any, 1)
	applied := make(chan struct{}, 1)
	m := &Model{
		ctx: context.Background(),
		postHook: func(fn func()) {
			fn()
			applied <- struct{}{}
		},
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command != "lsp_workspace_symbols" {
				return nil, fmt.Errorf("unexpected command %s", command)
			}
			requests <- input.(map[string]any)
			return json.RawMessage(`[
				{"name":"Order","kind":"class","container":"com.a","location":{"path":"/p/com/a/Order.java","range":{"start":{"line":2,"col":6},"end":{"line":2,"col":11}}}},
				{"name":"Order","kind":"class","container":"app.jar com.x","location":{"path":"/p/com/x/Order.java","range":{"start":{"line":9,"col":13},"end":{"line":9,"col":18}}}}
			]`), nil
		},
	}
	m.DebugMemoryJumpToSource("com.x.Order$Line[][]")
	select {
	case args := <-requests:
		if args["query"] != "Order" || args["request"] == uint32(0) {
			t.Fatalf("workspace symbol arguments = %#v", args)
		}
	case <-time.After(time.Second):
		t.Fatal("memory jump did not query workspace symbols")
	}
	select {
	case <-applied:
	case <-time.After(time.Second):
		t.Fatal("memory jump did not apply the symbol")
	}
	if m.reveal == nil || m.reveal.path != "/p/com/x/Order.java" ||
		m.reveal.line != 9 || m.reveal.col != 13 {
		t.Fatalf("memory source reveal = %+v", m.reveal)
	}
}

func TestDebugMemoryPackageMatchDoesNotAcceptSimilarPackages(t *testing.T) {
	appJar := "app.jar com.x"
	similar := "com.ax"
	exact := "com.x"
	if !debugMemoryInPackage(&appJar, "com.x") {
		t.Fatal("a package-qualified dependency container did not match")
	}
	if !debugMemoryInPackage(&exact, "com.x") {
		t.Fatal("an exact package container did not match")
	}
	if debugMemoryInPackage(&similar, "com.x") {
		t.Fatal("a similar package name matched")
	}
	if got := debugMemoryOuterClassName("com.x.Order$Line[][]"); got != "com.x.Order" {
		t.Fatalf("outer class = %q", got)
	}
}
