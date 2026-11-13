package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/egoist/mygo/ui/editor"
)

// The gear menu is OpenIDEA's gearMenuItems, in its order.
func TestGearMenuFollowsOpenIDEAOrder(t *testing.T) {
	g := gearGroup(&Model{})
	want := []struct {
		label string
		sep   bool
	}{
		{"Run Anything…", false},
		{"", true},
		{"Settings…", false},
		{"Plugins…", false},
		{"", true},
		{"Theme", false},
		{"Keymap", false},
		{"View Mode", false},
		{"", true},
		{"Customize Main Toolbar", false},
		{"", true},
		{"Check for Updates…", false},
	}
	if len(g) != len(want) {
		t.Fatalf("gear has %d items, want %d: %+v", len(g), len(want), g)
	}
	for i, w := range want {
		if g[i].sep != w.sep || (!w.sep && g[i].label != w.label) {
			t.Errorf("gear item %d = %q (sep %v), want %q (sep %v)", i, g[i].label, g[i].sep, w.label, w.sep)
		}
	}
	for _, e := range g {
		if e.label == "Search Everywhere" {
			t.Fatalf("the gear carries Search Everywhere, which OpenIDEA keeps out of it")
		}
	}
}

func TestGearMenuActionsResolve(t *testing.T) {
	for _, id := range []string{
		"editor.toggleZen",
		"window.toggleFullScreen",
		"settings.appearance",
		"settings.keymap",
		"toolbar.customize",
		"toolbar.reset",
		"app.checkForUpdates",
	} {
		if _, ok := Command(id); !ok {
			t.Errorf("registry lacks %q", id)
		}
	}
}

func TestViewModeTitleFollowsTheMode(t *testing.T) {
	m := &Model{}
	m.tabs = []*Tab{{state: &editor.State{}}}
	if got := zenTitle(m); got != "Enter Distraction Free Mode" {
		t.Fatalf("title before entering = %q", got)
	}
	m.ToggleZen()
	if !m.zen || !m.prefs.Zen {
		t.Fatalf("ToggleZen = zen %v pref %v, want both on", m.zen, m.prefs.Zen)
	}
	if got := zenTitle(m); got != "Exit Distraction Free Mode" {
		t.Fatalf("title in the mode = %q", got)
	}
	if !m.zenHint {
		t.Fatalf("entering did not arm the way-out hint")
	}
	if st := m.tabs[0].state; st.LineNumbers != editor.LineNumbersOff || !st.NoFolding {
		t.Fatalf("gutter in the mode = %v / %v, want hidden", st.LineNumbers, st.NoFolding)
	}
	m.ToggleZen()
	if m.zen || m.prefs.Zen {
		t.Fatalf("leaving kept the mode on")
	}
	if st := m.tabs[0].state; st.LineNumbers != editor.LineNumbersOn || st.NoFolding {
		t.Fatalf("gutter after leaving = %v / %v, want restored", st.LineNumbers, st.NoFolding)
	}
}

func TestZenWidthClamps(t *testing.T) {
	m := &Model{}
	m.StepZenWidth(-1000)
	if got := m.prefs.zenWidth(); got != minZenWidth {
		t.Fatalf("width below the floor = %d, want %d", got, minZenWidth)
	}
	m.StepZenWidth(10000)
	if got := m.prefs.zenWidth(); got != maxZenWidth {
		t.Fatalf("width above the ceiling = %d, want %d", got, maxZenWidth)
	}
}

func TestOpenSettingsPageSelectsThePage(t *testing.T) {
	m := &Model{}
	m.OpenSettingsPage("keymap")
	if !m.settingsOpen || m.settingsPage != "keymap" {
		t.Fatalf("keymap page = open %v page %q", m.settingsOpen, m.settingsPage)
	}
	m.OpenSettingsPage("languages.java")
	if m.settingsPage != "languages.java" {
		t.Fatalf("java page = %q", m.settingsPage)
	}
	m.OpenSettingsPage("no.such.page")
	if m.settingsPage != "appearance" {
		t.Fatalf("unknown page fell back to %q", m.settingsPage)
	}
}

func TestToolbarDraftEdits(t *testing.T) {
	layout := defaultToolbarLayout()
	if len(layout.Left) != 2 {
		t.Fatalf("default left = %v", layout.Left)
	}
	layout = toolbarDraftMove(layout, "left", 0, 1)
	if layout.Left[0] != toolbarVCS || layout.Left[1] != toolbarProject {
		t.Fatalf("move down = %v", layout.Left)
	}
	layout, at := toolbarDraftMoveSide(layout, "left", 0)
	if at != len(layout.Right)-1 || layout.Left[0] != toolbarProject {
		t.Fatalf("move to right = left %v right %v at %d", layout.Left, layout.Right, at)
	}
	layout = toolbarDraftRemove(layout, "right", at)
	if toolbarHas(layout.Right, toolbarVCS) || !toolbarHas(layout.Hidden, toolbarVCS) {
		t.Fatalf("remove = right %v hidden %v", layout.Right, layout.Hidden)
	}
	layout, at = toolbarDraftAdd(layout, "left", 0, toolbarProject)
	if !toolbarHas(layout.Left, toolbarProject) || toolbarHas(layout.Hidden, toolbarProject) || at != 0 {
		t.Fatalf("add = left %v hidden %v at %d", layout.Left, layout.Hidden, at)
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.2.0", "1.1.9", 1},
		{"v1.2.0", "1.2.0", 0},
		{"1.2.0-nightly.3", "1.2.0", 0},
		{"1.2", "1.2.1", -1},
		{"2.0.0", "10.0.0", -1},
	}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestFetchLatestRelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Errorf("release request lacks a User-Agent")
		}
		_, _ = w.Write([]byte(`{"tag_name":"v9.9.9","html_url":"https://example.invalid/releases/9.9.9"}`))
	}))
	defer srv.Close()
	version, page, err := fetchLatestRelease(srv.URL)
	if err != nil {
		t.Fatalf("fetchLatestRelease: %v", err)
	}
	if version != "9.9.9" || page != "https://example.invalid/releases/9.9.9" {
		t.Fatalf("release = %q %q", version, page)
	}
}

func TestFetchLatestReleaseReportsHTTPFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	if _, _, err := fetchLatestRelease(srv.URL); err == nil {
		t.Fatalf("403 reported as a release")
	}
}
