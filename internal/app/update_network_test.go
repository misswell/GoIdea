package app

import (
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo"
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
