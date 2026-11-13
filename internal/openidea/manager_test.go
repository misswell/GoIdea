package openidea

import (
	"encoding/json"
	"testing"
)

func TestListenCopiesNotificationsWithoutStealing(t *testing.T) {
	m := NewManager(LaunchConfig{})
	second, cancel := m.Listen(4)
	n := Notification{Method: "channel", Params: json.RawMessage(`{"id":3,"data":{"kind":"stdout","text":"hi"}}`)}

	m.deliver(n)

	select {
	case got := <-m.Notifications():
		if got.Method != "channel" {
			t.Fatalf("main stream got %q, want channel", got.Method)
		}
	default:
		t.Fatal("main stream missed the notification")
	}
	select {
	case got := <-second:
		if got.Method != "channel" {
			t.Fatalf("listener got %q, want channel", got.Method)
		}
	default:
		t.Fatal("listener missed the notification")
	}

	cancel()
	m.deliver(n)
	select {
	case <-second:
		t.Fatal("canceled listener still received a notification")
	default:
	}
}

func TestDecodeRunConfigsKeepsRawForRunStart(t *testing.T) {
	raw := json.RawMessage(`[{"id":"maven:package","name":"mvn package","kind":"maven",
		"command":"mvn","args":["package"],"cwd":"/tmp/p","env":{},"detected":true,
		"isTest":false,"debugAdapter":null,"module":null,"params":{"goal":"package"}}]`)
	configs, err := DecodeRunConfigs(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(configs) != 1 || configs[0].Name != "mvn package" || configs[0].IsTest {
		t.Fatalf("decoded %+v", configs)
	}
	var back map[string]any
	if err := json.Unmarshal(configs[0].Raw(), &back); err != nil {
		t.Fatal(err)
	}
	if back["command"] != "mvn" || back["params"].(map[string]any)["goal"] != "package" {
		t.Fatalf("raw lost fields: %s", configs[0].Raw())
	}
}
