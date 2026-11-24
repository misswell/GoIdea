package app

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"mayoi/internal/openidea"
)

func pythonPackagesTestModel(t *testing.T, root, interpreter string) *Model {
	t.Helper()
	m := &Model{ctx: context.Background()}
	m.setRoot(root)
	m.postHook = func(fn func()) { fn() }
	def, ok := nativeSettingsDefinition(settingsPagePythonDeps)
	if !ok {
		t.Fatal("Python Dependencies settings page is missing")
	}
	values := nativeTestDefaults(def)
	values["languages.python.interpreter"] = interpreter
	m.nativeSettings.pages = map[string]nativeSettingsPageState{
		settingsPagePythonDeps: {
			loaded:  true,
			applied: nativeSettingsCloneValues(values),
			draft:   nativeSettingsCloneValues(values),
		},
	}
	return m
}

func TestPythonPackagesLoadTheEffectiveInterpreter(t *testing.T) {
	root := t.TempDir()
	m := pythonPackagesTestModel(t, root, "/project/.venv/bin/python")
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		request := args.(map[string]any)
		switch command {
		case "sdk_python_effective":
			if request["near"] != root {
				t.Fatalf("effective request = %#v", request)
			}
			return json.RawMessage(`"/project/.venv/bin/python"`), nil
		case "sdk_python_packages":
			if request["python"] != "/project/.venv/bin/python" || request["request"].(uint32) == 0 {
				t.Fatalf("packages request = %#v", request)
			}
			return json.RawMessage(`[
				{"name":"requests","version":"2.32.3"},
				{"name":"urllib3","version":"2.2.3"}
			]`), nil
		default:
			t.Fatalf("unexpected RPC %q", command)
			return nil, nil
		}
	}

	m.ensurePythonPackages(false)
	waitSettingsBackendTest(t, func() bool {
		state := m.languageSDKs.packages
		return state.loaded && !state.loading
	})
	state := m.languageSDKs.packages
	if state.interpreter != "/project/.venv/bin/python" {
		t.Fatalf("interpreter = %q", state.interpreter)
	}
	if !reflect.DeepEqual(state.packages, []openidea.PythonPackage{
		{Name: "requests", Version: "2.32.3"},
		{Name: "urllib3", Version: "2.2.3"},
	}) {
		t.Fatalf("packages = %#v", state.packages)
	}
}

func TestPythonPackagesFilterAndFreezeDeterministically(t *testing.T) {
	packages := []openidea.PythonPackage{
		{Name: "Zulu", Version: "3.0"},
		{Name: "alpha", Version: "2.0"},
		{Name: "Beta", Version: "1.0"},
	}
	if got := filterPythonPackages(packages, "ET"); len(got) != 1 || got[0].Name != "Beta" {
		t.Fatalf("filtered packages = %#v", got)
	}
	want := "alpha==2.0\nBeta==1.0\nZulu==3.0\n"
	if got := freezePythonRequirements(packages); got != want {
		t.Fatalf("freeze output = %q, want %q", got, want)
	}
}

func TestPythonPackageJobUsesOwnedOutputChannelAndRefreshes(t *testing.T) {
	root := t.TempDir()
	m := pythonPackagesTestModel(t, root, "/venv/python")
	release := make(chan struct{})
	request := make(chan map[string]any, 1)
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		switch command {
		case "sdk_python_pip":
			in := args.(map[string]any)
			copied := make(map[string]any, len(in))
			for key, value := range in {
				copied[key] = value
			}
			request <- copied
			<-release
			return json.RawMessage(`null`), nil
		case "sdk_python_effective":
			return json.RawMessage(`"/venv/python"`), nil
		case "sdk_python_packages":
			return json.RawMessage(`[{"name":"requests","version":"2.32.3"}]`), nil
		default:
			t.Fatalf("unexpected RPC %q", command)
			return nil, nil
		}
	}
	m.languageSDKs.packages = pythonPackagesState{
		root: root, interpreter: "/venv/python", loaded: true,
	}
	m.startPythonPackageJob("upgrade", "requests")
	payload := <-request
	if payload["python"] != "/venv/python" || payload["requirement"] != "requests" ||
		payload["upgrade"] != true || payload["uninstall"] != false ||
		payload["request"].(uint32) == 0 {
		t.Fatalf("pip request = %#v", payload)
	}
	output := payload["onOutput"].(map[string]any)
	channel := output["$channel"].(uint32)
	if channel == 0 || !m.acceptLanguageSDKChunk(channel, json.RawMessage(`"Collecting requests"`)) {
		t.Fatalf("owned output channel = %d", channel)
	}
	if m.acceptLanguageSDKChunk(channel+1, json.RawMessage(`"wrong"`)) {
		t.Fatal("output from another channel was accepted")
	}
	if got := m.languageSDKs.packages.log; !reflect.DeepEqual(got, []string{"Collecting requests"}) {
		t.Fatalf("package log = %#v", got)
	}
	close(release)
	waitSettingsBackendTest(t, func() bool {
		state := m.languageSDKs.packages
		return state.job == nil && state.loaded && !state.loading
	})
	if got := m.languageSDKs.packages.packages; !reflect.DeepEqual(got, []openidea.PythonPackage{
		{Name: "requests", Version: "2.32.3"},
	}) {
		t.Fatalf("refreshed packages = %#v", got)
	}
}

func TestPythonPackageJobCanBeCanceled(t *testing.T) {
	root := t.TempDir()
	m := pythonPackagesTestModel(t, root, "/venv/python")
	started := make(chan struct{})
	canceled := make(chan struct{})
	m.invokeHook = func(ctx context.Context, command string, _ any) (json.RawMessage, error) {
		switch command {
		case "sdk_python_pip":
			close(started)
			<-canceled
			return nil, context.Canceled
		case "cancel_request":
			close(canceled)
			return json.RawMessage(`null`), nil
		default:
			t.Fatalf("unexpected RPC %q", command)
			return nil, nil
		}
	}
	m.languageSDKs.packages = pythonPackagesState{
		root: root, interpreter: "/venv/python", loaded: true,
	}
	m.startPythonPackageJob("install", "requests")
	<-started
	m.cancelPythonPackageJob()
	waitSettingsBackendTest(t, func() bool {
		return m.languageSDKs.packages.job == nil
	})
	if got := m.languageSDKs.packages.jobErr; got != "" {
		t.Fatalf("canceled job error = %q", got)
	}
}

func TestPythonPackagesIgnoreAProjectSwitch(t *testing.T) {
	firstRoot := t.TempDir()
	secondRoot := t.TempDir()
	started := make(chan struct{})
	release := make(chan struct{})
	posted := make(chan struct{}, 1)
	m := pythonPackagesTestModel(t, firstRoot, "")
	m.postHook = func(fn func()) {
		fn()
		posted <- struct{}{}
	}
	m.invokeHook = func(_ context.Context, command string, _ any) (json.RawMessage, error) {
		switch command {
		case "sdk_python_effective":
			close(started)
			<-release
			return json.RawMessage(`"/old/python"`), nil
		case "sdk_python_packages":
			return json.RawMessage(`[]`), nil
		default:
			t.Errorf("unexpected RPC %q", command)
			return nil, nil
		}
	}
	m.ensurePythonPackages(false)
	<-started
	m.setRoot(secondRoot)
	close(release)
	<-posted
	if state := m.languageSDKs.packages; state.interpreter != "" || state.loaded {
		t.Fatalf("stale package state was applied: %+v", state)
	}
}

func TestFreezePythonRequirementsKeepsVersionlessNames(t *testing.T) {
	got := freezePythonRequirements([]openidea.PythonPackage{
		{Name: "editable-package"},
		{Name: "zope", Version: "5.0"},
	})
	if want := "editable-package\nzope==5.0\n"; got != want {
		t.Fatalf("freeze output = %q, want %q", got, want)
	}
}
