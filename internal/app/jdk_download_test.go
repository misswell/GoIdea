package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

type recordedBackendCall struct {
	command string
	args    any
}

func TestDownloadJDKActionParsesVersionAndOpensSettingsWithoutIt(t *testing.T) {
	m := &Model{cfg: Config{SettingsPath: filepath.Join(t.TempDir(), "settings.json")}}
	if !m.RunAction("java.downloadJdk") {
		t.Fatal("java.downloadJdk is not registered")
	}
	if !m.settingsOpen || m.settingsPage != settingsPageJava {
		t.Fatalf("no-argument action opened settings page %q", m.settingsPage)
	}

	args, ok := parseDownloadJDKArgs(map[string]any{
		"version": float64(21), "languageServer": true,
	})
	if !ok || args.Version != 21 || !args.LanguageServer {
		t.Fatalf("parsed args = %+v, ok=%v", args, ok)
	}
	for _, arg := range []any{
		nil,
		map[string]any{},
		map[string]any{"version": "21"},
		map[string]any{"version": float64(0)},
		21,
	} {
		if _, ok := parseDownloadJDKArgs(arg); ok {
			t.Fatalf("accepted invalid Download JDK argument %#v", arg)
		}
	}
}

func TestRequiredJDKReadsLanguageServerAndProjectErrors(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    jdkRequirement
		ok      bool
	}{
		{
			name:    "jdtls",
			message: "jdtls needs JDK 25 or newer to run (found JDK 8, 17, 21).",
			want:    jdkRequirement{version: 25, languageServer: true},
			ok:      true,
		},
		{
			name:    "language server JDK",
			message: "The language server JDK (JDK 21) is too old: jdtls needs JDK 25 or newer.",
			want:    jdkRequirement{version: 25, languageServer: true},
			ok:      true,
		},
		{
			name:    "project",
			message: "This project targets Java 21: it needs JDK 21 or newer (found JDK 17).",
			want:    jdkRequirement{version: 21},
			ok:      true,
		},
		{
			name:    "not a version requirement",
			message: "The language server JDK /x is not a JDK home.",
		},
		{
			name:    "missing binary",
			message: "`rust-analyzer` was not found on PATH",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := requiredJDK(test.message)
			if ok != test.ok || got != test.want {
				t.Fatalf("requiredJDK(%q) = %+v, %v; want %+v, %v",
					test.message, got, ok, test.want, test.ok)
			}
		})
	}
}

func TestMisconfiguredLanguageServerPromptsOnceForEachRequiredJDK(t *testing.T) {
	m := &Model{cfg: Config{SettingsPath: filepath.Join(t.TempDir(), "settings.json")}}
	message := "jdtls needs JDK 25 or newer to run (found JDK 8, 17, 21)."
	event := openidea.LanguageServerEvent{
		Server: 4, Name: "jdtls", Status: "misconfigured", Message: &message,
	}
	m.acceptLanguageServerEvent(event)
	if len(m.notify.items) != 1 {
		t.Fatalf("JDK prompt count = %d, want 1", len(m.notify.items))
	}
	prompt := m.notify.items[0]
	if prompt.level != "warning" || !prompt.permanent ||
		prompt.title != "jdtls needs JDK 25 to run" ||
		len(prompt.actions) != 2 ||
		prompt.actions[0].label != "Download JDK 25" ||
		prompt.actions[1].label != "Java Settings…" {
		t.Fatalf("JDK prompt = %+v", prompt)
	}

	m.acceptLanguageServerEvent(event)
	if len(m.notify.items) != 1 {
		t.Fatalf("duplicate JDK prompt = %d entries", len(m.notify.items))
	}

	newer := "jdtls needs JDK 26 or newer to run."
	m.acceptLanguageServerEvent(openidea.LanguageServerEvent{
		Server: 4, Name: "jdtls", Status: "misconfigured", Message: &newer,
	})
	if len(m.notify.items) != 1 ||
		m.notify.items[0].actions[0].label != "Download JDK 26" {
		t.Fatalf("new JDK requirement was not replaced: %+v", m.notify.items)
	}

	m.acceptLanguageServerEvent(openidea.LanguageServerEvent{
		Server: 5, Name: "other", Status: "crashed", Message: &newer,
	})
	if len(m.notify.items) != 1 {
		t.Fatal("a non-misconfigured server raised a JDK prompt")
	}
}

func TestDownloadJDKActionStartsABackgroundDownload(t *testing.T) {
	done := make(chan struct{})
	var once sync.Once
	var callsMu sync.Mutex
	var calls []recordedBackendCall
	m := &Model{ctx: context.Background()}
	m.postHook = func(fn func()) {
		fn()
		once.Do(func() { close(done) })
	}
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		callsMu.Lock()
		calls = append(calls, recordedBackendCall{command: command, args: args})
		callsMu.Unlock()
		switch command {
		case "jdk_download":
			return json.Marshal(openidea.Sdk{
				Path: "/tools/jdks/temurin-21", Name: "Temurin 21",
			})
		default:
			return nil, fmt.Errorf("unexpected command %s", command)
		}
	}

	if !m.RunActionWithArg("java.downloadJdk", map[string]any{"version": 21}) {
		t.Fatal("java.downloadJdk did not run")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("background JDK download did not finish")
	}
	if m.jdkDownloadBusy || m.jdkDownloadReq != 0 || m.jdkDownloadCh != 0 {
		t.Fatalf("download state did not clear: %+v", m)
	}
	if !strings.Contains(m.status, "Temurin 21 downloaded") {
		t.Fatalf("status = %q", m.status)
	}
	callsMu.Lock()
	defer callsMu.Unlock()
	if len(calls) != 1 || calls[0].command != "jdk_download" {
		t.Fatalf("backend calls = %+v", calls)
	}
	args := calls[0].args.(map[string]any)
	if args["vendor"] != defaultJDKVendor || args["version"] != uint32(21) ||
		args["location"] != nil {
		t.Fatalf("jdk_download args = %#v", args)
	}
}

func TestDownloadJDKLanguageServerMovesAnExplicitJDK(t *testing.T) {
	done := make(chan struct{})
	var once sync.Once
	var mu sync.Mutex
	var selected string
	m := &Model{ctx: context.Background(), root: "/project"}
	m.postHook = func(fn func()) {
		fn()
		once.Do(func() { close(done) })
	}
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		switch command {
		case "jdk_download":
			return json.Marshal(openidea.Sdk{
				Path: "/tools/jdks/temurin-25", Name: "Temurin 25",
			})
		case "settings_get":
			return json.RawMessage(`{"languages":{"java":{"jdtlsJdk":"/old/jdk-17"}}}`), nil
		case "settings_set":
			request := args.(map[string]any)
			mu.Lock()
			selected = request["value"].(string)
			mu.Unlock()
			if request["scope"] != "project" ||
				request["key"] != "languages.java.jdtlsJdk" {
				t.Fatalf("settings_set args = %#v", request)
			}
			return json.RawMessage(`{}`), nil
		default:
			return nil, fmt.Errorf("unexpected command %s", command)
		}
	}

	m.StartJDKDownload(defaultJDKVendor, 25, "", true)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("language-server JDK download did not finish")
	}
	mu.Lock()
	defer mu.Unlock()
	if selected != `"/tools/jdks/temurin-25"` {
		t.Fatalf("selected language-server JDK = %q", selected)
	}
}

func TestJDKDownloadDialogLoadsAndRendersNativeChoices(t *testing.T) {
	posted := make(chan struct{}, 2)
	m := &Model{ctx: context.Background()}
	m.postHook = func(fn func()) {
		fn()
		posted <- struct{}{}
	}
	m.invokeHook = func(_ context.Context, command string, _ any) (json.RawMessage, error) {
		switch command {
		case "jdk_download_vendors":
			return json.RawMessage(`[{"id":"temurin","name":"Eclipse Temurin"}]`), nil
		case "jdk_download_location":
			return json.RawMessage(`"/tools/jdks"`), nil
		case "jdk_download_releases":
			return json.RawMessage(`{"versions":[25,21,17],"lts":[21,17],"recommended":21}`), nil
		default:
			return nil, fmt.Errorf("unexpected command %s", command)
		}
	}
	m.OpenJDKDownloadDialog(0)
	for range 2 {
		select {
		case <-posted:
		case <-time.After(2 * time.Second):
			t.Fatal("JDK dialog choices did not load")
		}
	}
	if m.jdkDialog.vendor != defaultJDKVendor || m.jdkDialog.version != 21 ||
		m.jdkDialog.location != "/tools/jdks" {
		t.Fatalf("JDK dialog state = %+v", m.jdkDialog)
	}

	theme := themeForName("dark")
	tester := ui.NewTester(func(c *ui.Context) {
		m.jdkDownloadDialog(c, theme)
	}, 760, 600)
	tester.Frame()
	for _, text := range []string{
		"Download JDK", "Eclipse Temurin", "21 (LTS)", "/tools/jdks", "Download", "Cancel",
	} {
		if !tester.HasText(text) {
			t.Fatalf("Download JDK dialog is missing %q: %v", text, tester.Texts())
		}
	}
}

func TestJDKDownloadEventsUpdateOnlyThatTask(t *testing.T) {
	m := &Model{
		jdkDownloadCh:  9,
		jdkDownloadVer: 21,
		tasks: []BackgroundTask{
			{ID: jdkDownloadTaskID(21), Title: "Downloading JDK 21"},
			{ID: jdkDownloadTaskID(17), Title: "Downloading JDK 17"},
		},
	}
	m.acceptChunk(8, json.RawMessage(`{"type":"line","text":"stolen"}`))
	if m.jdkDialog.message != "" {
		t.Fatal("another channel was accepted")
	}
	m.acceptChunk(9, json.RawMessage(
		`{"type":"progress","fraction":0.5,"message":"Downloaded 10 of 20 MB"}`))
	if m.jdkDialog.progress == nil || *m.jdkDialog.progress != 0.5 ||
		m.jdkDialog.message != "Downloaded 10 of 20 MB" {
		t.Fatalf("dialog progress = %+v", m.jdkDialog)
	}
	if m.tasks[0].Fraction != 0.5 || m.tasks[1].Fraction != 0 {
		t.Fatalf("task progress = %+v", m.tasks)
	}
}

func TestCancelJDKDownloadCancelsTheBackendRequest(t *testing.T) {
	cancelled := make(chan uint32, 1)
	m := &Model{
		ctx:             context.Background(),
		jdkDownloadBusy: true,
		jdkDownloadReq:  44,
		jdkDownloadCh:   7,
		jdkDialog:       jdkDialogState{open: true},
	}
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		if command != "cancel_request" {
			return nil, fmt.Errorf("unexpected command %s", command)
		}
		cancelled <- args.(map[string]any)["id"].(uint32)
		return json.RawMessage(`{}`), nil
	}
	m.CancelJDKDownload()
	select {
	case request := <-cancelled:
		if request != 44 {
			t.Fatalf("cancelled request = %d", request)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancel_request was not sent")
	}
	if !m.jdkDownloadAbort.Load() || m.jdkDialog.open {
		t.Fatalf("JDK download did not enter cancelled state")
	}
	if !hiddenActionIDs["java.downloadJdk"] {
		t.Fatal("java.downloadJdk must stay hidden like upstream")
	}
}
