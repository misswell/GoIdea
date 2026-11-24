package openidea

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProfilePreparedKeepsRawLaunchAndSpec(t *testing.T) {
	raw := json.RawMessage(`{
		"status":"ready",
		"profiler":"node --cpu-prof",
		"launch":{
			"id":"app","name":"App","kind":"node","command":"node","args":["app.js"],
			"includeSystemEnv":true,"customField":{"kept":true}
		},
		"spec":{"kind":"cpuProfiles","dir":"/tmp/profile","root":"/tmp/project"}
	}`)
	var prepared ProfilePrepared
	if err := json.Unmarshal(raw, &prepared); err != nil {
		t.Fatal(err)
	}
	if prepared.Status != "ready" || prepared.Profiler != "node --cpu-prof" ||
		prepared.Launch == nil || prepared.Launch.Command != "node" {
		t.Fatalf("prepared = %+v", prepared)
	}
	if !strings.Contains(string(prepared.Launch.Raw()), `"customField":{"kept":true}`) {
		t.Fatalf("launch raw fields were lost: %s", prepared.Launch.Raw())
	}
	if !strings.Contains(string(prepared.Spec), `"cpuProfiles"`) {
		t.Fatalf("profile spec = %s", prepared.Spec)
	}
}

func TestProfilePreparedDecodesFailureActions(t *testing.T) {
	var prepared ProfilePrepared
	if err := json.Unmarshal([]byte(`{
		"status":"missingTool","tool":"py-spy","message":"Install py-spy.",
		"install":{"command":"python3","args":["-m","pip","install","py-spy"],"cwd":null},
		"homepage":"https://github.com/benfred/py-spy"
	}`), &prepared); err != nil {
		t.Fatal(err)
	}
	if prepared.Launch != nil || prepared.Install == nil || prepared.Install.Command != "python3" ||
		prepared.Homepage == nil || *prepared.Homepage != "https://github.com/benfred/py-spy" {
		t.Fatalf("prepared = %+v", prepared)
	}
}
