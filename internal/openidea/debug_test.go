package openidea

import (
	"encoding/json"
	"testing"
)

func TestDecodeDebugEnvelope(t *testing.T) {
	raw := json.RawMessage(`{"session":7,"event":{"kind":"stopped","threadId":12,"reason":"breakpoint","description":null,"allThreadsStopped":true}}`)
	var env DebugEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	if env.Session != 7 {
		t.Fatalf("session = %d", env.Session)
	}
	var e DebugEvent
	if err := json.Unmarshal(env.Event, &e); err != nil {
		t.Fatal(err)
	}
	if e.Kind != "stopped" || e.ThreadID != 12 || e.Reason != "breakpoint" || !e.AllThreadsStopped {
		t.Fatalf("event = %+v", e)
	}
}

func TestDecodeDebugLaunchConfigRoundTrip(t *testing.T) {
	extra := `{"debugServer":5005}`
	cfg := DebugLaunchConfig{
		Name: "demo.Hello", Adapter: "java-debug", Request: "launch",
		Args: []string{}, Cwd: "/p", Env: map[string]string{}, ExtraJSON: &extra,
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var back DebugLaunchConfig
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if back.Adapter != "java-debug" || back.Request != "launch" || back.ExtraJSON == nil || *back.ExtraJSON != extra {
		t.Fatalf("back = %+v", back)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if string(m["args"]) != "[]" {
		t.Fatalf("args = %s, want [] so Rust decodes a Vec", m["args"])
	}
	if string(m["program"]) != "null" {
		t.Fatalf("program = %s, want null", m["program"])
	}
}

func TestDecodeStackFrameZeroBased(t *testing.T) {
	var f StackFrame
	line := 4
	if err := json.Unmarshal([]byte(`{"id":3,"name":"demo.Hello.main","path":"/p/Hello.java","line":4,"column":8}`), &f); err != nil {
		t.Fatal(err)
	}
	if f.Line != line || f.Path == nil || *f.Path != "/p/Hello.java" {
		t.Fatalf("frame = %+v", f)
	}
}
