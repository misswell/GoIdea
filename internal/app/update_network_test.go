package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestMacSystemProxyUsesHTTPSAndBypassesExceptions(t *testing.T) {
	snapshot := parseMacProxySnapshot(`<dictionary> {
  ExceptionsList : <array> {
    0 : 127.0.0.1
    1 : 192.168.0.0/16
    2 : *.example.com
    3 : <local>
  }
  HTTPEnable : 1
  HTTPPort : 7890
  HTTPProxy : 127.0.0.1
  HTTPSEnable : 1
  HTTPSPort : 7890
  HTTPSProxy : 127.0.0.1
  SOCKSEnable : 1
  SOCKSPort : 7890
  SOCKSProxy : 127.0.0.1
}`)
	for _, raw := range []string{
		"https://api.github.com/releases/latest",
		"HTTP://example.org/file",
	} {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		got := snapshot.proxyFor(u)
		if got == nil || got.String() != "http://127.0.0.1:7890" {
			t.Fatalf("proxy for %s = %v", raw, got)
		}
	}
	for _, raw := range []string{
		"https://127.0.0.1/file",
		"https://localhost/file",
		"https://build.example.com/file",
		"https://192.168.1.2/file",
		"https://intranet/file",
	} {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if got := snapshot.proxyFor(u); got != nil {
			t.Fatalf("proxy for excluded %s = %v", raw, got)
		}
	}
}

func TestMacSystemProxyFallsBackToSOCKS(t *testing.T) {
	snapshot := parseMacProxySnapshot(`<dictionary> {
  HTTPEnable : 0
  HTTPSEnable : 0
  SOCKSEnable : 1
  SOCKSPort : 1080
  SOCKSProxy : 127.0.0.1
}`)
	u, _ := url.Parse("https://api.github.com/")
	got := snapshot.proxyFor(u)
	if got == nil || got.String() != "socks5://127.0.0.1:1080" {
		t.Fatalf("SOCKS proxy = %v", got)
	}
}

func TestHTTPProxySettingsRouteManualSOCKSAndNoProxy(t *testing.T) {
	m := &Model{}
	setProxyRuntime(m)
	t.Cleanup(func() { setProxyRuntime(nil) })
	m.nativeSettings.pages = map[string]nativeSettingsPageState{
		settingsPageHTTPProxy: {
			loaded: true,
			applied: map[string]any{
				"proxy.mode":         "manual",
				"proxy.kind":         "socks",
				"proxy.host":         "proxy.example",
				"proxy.noProxy":      "*.internal.example, 192.168.*",
				"proxy.port":         float64(1080),
				"proxy.authenticate": true,
				"proxy.login":        "me",
				"proxy.password":     "secret",
			},
			draft: map[string]any{
				"proxy.mode":         "manual",
				"proxy.kind":         "socks",
				"proxy.host":         "proxy.example",
				"proxy.noProxy":      "*.internal.example, 192.168.*",
				"proxy.port":         float64(1080),
				"proxy.authenticate": true,
				"proxy.login":        "me",
				"proxy.password":     "secret",
			},
		},
	}
	for raw, want := range map[string]string{
		"https://api.example.com/": "socks5h://me:secret@proxy.example:1080",
	} {
		req, err := http.NewRequest(http.MethodGet, raw, nil)
		if err != nil {
			t.Fatal(err)
		}
		got, err := updateRequestProxy(req)
		if err != nil {
			t.Fatal(err)
		}
		if got == nil || got.String() != want {
			t.Fatalf("proxy for %s = %v, want %s", raw, got, want)
		}
	}
	for _, raw := range []string{
		"https://service.internal.example/",
		"https://192.168.4.9/",
		"http://localhost:8080/",
	} {
		req, err := http.NewRequest(http.MethodGet, raw, nil)
		if err != nil {
			t.Fatal(err)
		}
		got, err := updateRequestProxy(req)
		if err != nil {
			t.Fatal(err)
		}
		if got != nil {
			t.Fatalf("proxy for excluded %s = %v", raw, got)
		}
	}
}

func TestHTTPProxyCheckConnectionUsesDraftAndReturnsStatus(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.String() != "http://target.example/check" {
			t.Errorf("proxied URL = %q", r.URL.String())
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer proxy.Close()
	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(proxyURL.Port())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	status, err := proxyCheckConnection(ctx, nativeProxySettings{
		mode: "manual", kind: "http", host: proxyURL.Hostname(), port: port,
	}, "target.example/check")
	if err != nil {
		t.Fatal(err)
	}
	if status != "201 Created" {
		t.Fatalf("proxy check status = %q", status)
	}
}

func TestHTTPProxyPageRendersPasswordAndCheckConnection(t *testing.T) {
	m := &Model{}
	m.nativeSettings.pages = map[string]nativeSettingsPageState{
		settingsPageHTTPProxy: {
			loaded: true,
			applied: map[string]any{
				"proxy.mode":         "manual",
				"proxy.kind":         "http",
				"proxy.host":         "proxy.example",
				"proxy.noProxy":      "",
				"proxy.port":         float64(3128),
				"proxy.authenticate": true,
				"proxy.login":        "me",
				"proxy.password":     "secret",
			},
			draft: map[string]any{
				"proxy.mode":         "manual",
				"proxy.kind":         "http",
				"proxy.host":         "proxy.example",
				"proxy.noProxy":      "",
				"proxy.port":         float64(3128),
				"proxy.authenticate": true,
				"proxy.login":        "me",
				"proxy.password":     "secret",
			},
		},
	}
	tester := ui.NewTester(func(c *ui.Context) {
		m.settingsNativeBody(c, oiPaletteOf(c.Theme()), settingsPageHTTPProxy)
	}, 1000, 800)
	tester.Frame()
	for _, text := range []string{"Password", "Check connection", "Check"} {
		if !tester.HasText(text) {
			t.Fatalf("HTTP Proxy page lacks %q: %q", text, tester.Texts())
		}
	}
	if got := nativeSettingsString(m, settingsPageHTTPProxy, "proxy.password"); got != "secret" {
		t.Fatalf("proxy password draft = %q", got)
	}
}

func TestPendingUpdateFailureIsReportedAfterRelaunch(t *testing.T) {
	current := mygo.App.Version()
	if current == "" {
		t.Skip("test binary has no app version")
	}
	path := filepath.Join(t.TempDir(), "session.json")
	prefsPath := updatePrefsPath(path)
	saveUpdatePrefs(prefsPath, updatePrefs{
		LastRunVersion: current,
		PendingVersion: "99.99.99",
		PendingNotes:   "The failed release notes.",
	})
	m := &Model{cfg: Config{SessionPath: path}, postHook: func(fn func()) { fn() }}
	m.finishPendingUpdate()
	prefs := loadUpdatePrefs(prefsPath)
	if prefs.PendingVersion != "" {
		t.Fatalf("failed update stayed pending: %+v", prefs)
	}
	if len(m.notify.items) != 1 {
		t.Fatalf("notifications = %+v", m.notify.items)
	}
	if got := m.notify.items[0].title; got != "GoIDEA 99.99.99 did not finish installing" {
		t.Fatalf("notification title = %q", got)
	}
	if len(m.notify.items[0].actions) != 1 ||
		m.notify.items[0].actions[0].label != "Check Again" {
		t.Fatalf("actions = %+v", m.notify.items[0].actions)
	}
}

func TestPendingUpdateSuccessOpensWhatsNewAndClearsState(t *testing.T) {
	current := mygo.App.Version()
	if current == "" {
		t.Skip("test binary has no app version")
	}
	path := filepath.Join(t.TempDir(), "session.json")
	prefsPath := updatePrefsPath(path)
	saveUpdatePrefs(prefsPath, updatePrefs{
		LastRunVersion: "0.0.1",
		PendingVersion: current,
		PendingNotes:   "# Updated successfully\n\n- Online update applied.",
	})
	m := &Model{cfg: Config{SessionPath: path}, postHook: func(fn func()) { fn() }}
	m.finishPendingUpdate()
	prefs := loadUpdatePrefs(prefsPath)
	if prefs.PendingVersion != "" || prefs.LastRunVersion != current {
		t.Fatalf("update preferences = %+v", prefs)
	}
	tab := m.whatsNewTab()
	if tab == nil || tab.pageVersion != current ||
		!strings.Contains(tab.pageNotes, "Online update applied") {
		t.Fatalf("What's New tab = %+v", tab)
	}
}
