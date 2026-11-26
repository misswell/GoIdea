package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// updateFeedAt serves one release with this Mac's assets; wait, when not nil,
// holds the answer until it is closed.
func updateFeedAt(t *testing.T, version, notes string, wait <-chan struct{}) *httptest.Server {
	t.Helper()
	dmg := fmt.Sprintf("goidea-%s-%s.dmg", version, runtime.GOARCH)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if wait != nil {
			<-wait
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tag_name": "v" + version,
			"html_url": "https://example.invalid/release",
			"body":     notes,
			"assets": []map[string]string{
				{"name": dmg, "browser_download_url": "https://example.invalid/a.dmg"},
				{"name": dmg + ".sha256", "browser_download_url": "https://example.invalid/a.sha256"},
			},
		})
	}))
	t.Cleanup(srv.Close)
	old := updateFeedURL
	updateFeedURL = srv.URL
	t.Cleanup(func() { updateFeedURL = old })
	return srv
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestCheckForUpdatesAnnouncesAsAStickyNotification(t *testing.T) {
	updateFeedAt(t, "99.0.0", "The first paragraph.\n\n- one fix", nil)
	m := &Model{postHook: func(fn func()) { fn() },
		cfg: Config{SessionPath: filepath.Join(t.TempDir(), "session.json")}}
	m.checkForUpdates(false)
	waitFor(t, "the check's notification", func() bool {
		return len(m.notify.items) > 0 && m.notify.items[0].title != "Checking for updates…"
	})
	n := m.notify.items[0]
	if n.title != "GoIDEA 99.0.0 is available" {
		t.Fatalf("notification title = %q", n.title)
	}
	if !n.sticky {
		t.Fatal("the available notification is not sticky")
	}
	if len(n.actions) != 1 || n.actions[0].label != "Install and Restart" {
		t.Fatalf("actions = %+v, want Install and Restart alone", n.actions)
	}
	if n.body != "The first paragraph." {
		t.Fatalf("body = %q, want the notes' first paragraph", n.body)
	}
	if len(m.tasks) != 0 {
		waitFor(t, "the check task to finish", func() bool { return len(m.tasks) == 0 })
	}
	// The announcement is claimed once: the automatic check stays quiet.
	m.notify.items = nil
	m.checkForUpdates(false)
	waitFor(t, "the automatic check to answer", func() bool { return !m.update.checking })
	time.Sleep(50 * time.Millisecond)
	for _, it := range m.notify.items {
		if strings.Contains(it.title, "is available") {
			t.Fatalf("the automatic check announced again: %q", it.title)
		}
	}
}

func TestUpdateSummaryReadsAsPlainText(t *testing.T) {
	cases := map[string]string{
		"The first paragraph.\n\n- one fix":          "The first paragraph.",
		"- **Check for Updates** now runs in a task": "Check for Updates now runs in a task",
		"# Heading\n\n`code` and __bold__":           "Heading",
		"":                                           "",
	}
	for notes, want := range cases {
		if got := updateSummary(notes); got != want {
			t.Errorf("updateSummary(%q) = %q, want %q", notes, got, want)
		}
	}
}

func TestCancellingACheckDropsItsAnswer(t *testing.T) {
	release := make(chan struct{})
	updateFeedAt(t, "99.0.0", "", release)
	m := &Model{postHook: func(fn func()) { fn() },
		cfg: Config{SessionPath: filepath.Join(t.TempDir(), "session.json")}}
	m.CheckForUpdates()
	if len(m.tasks) != 1 || m.tasks[0].ID != updateCheckTaskID {
		t.Fatalf("check task = %+v", m.tasks)
	}
	m.cancelTask(updateCheckTaskID)
	if m.update.checking {
		t.Fatal("a cancelled check is still checking")
	}
	close(release)
	time.Sleep(150 * time.Millisecond)
	if len(m.notify.items) != 0 {
		t.Fatalf("a cancelled check posted %+v", m.notify.items)
	}
}

func TestCancelledTaskKeepsTheMessageItHad(t *testing.T) {
	m := &Model{postHook: func(fn func()) { fn() }}
	m.upsertTask(BackgroundTask{ID: "t", Title: "Downloading", Fraction: -1, Cancel: func() {}})
	m.cancelTask("t")
	m.upsertTask(BackgroundTask{ID: "t", Title: "Downloading", Fraction: 1, Message: "9 MB of 10 MB", Done: true})
	// A cancelled task leaves on its end, without the linger.
	if len(m.tasks) != 0 {
		t.Fatalf("cancelled task stayed: %+v", m.tasks)
	}
}

func TestMarkdownBlocksKeepTheReadingShape(t *testing.T) {
	blocks := markdownBlocks("# Title\n\nFirst **bold** and [a link](https://x).\n\n- one\n- `two`\n\n> quoted\n\n```\ncode\n```")
	var kinds []string
	for _, b := range blocks {
		kinds = append(kinds, b.kind)
	}
	if got := strings.Join(kinds, ","); got != "h1,p,item,item,quote,code" {
		t.Fatalf("blocks = %s", got)
	}
	if blocks[1].text != "First bold and a link." {
		t.Fatalf("inline text = %q", blocks[1].text)
	}
	if blocks[3].text != "two" {
		t.Fatalf("code span text = %q", blocks[3].text)
	}
}

func TestWhatsNewIsAStandalonePage(t *testing.T) {
	m := &Model{postHook: func(fn func()) { fn() }}
	m.OpenWhatsNew("1.2.3", "# What's new\n\n- Faster indexing")
	if tab := m.tab(); tab == nil || tab.kind != tabKindWhatsNew {
		t.Fatalf("active tab = %+v, want What's New", tab)
	}
	tt := ui.NewTester(func(c *ui.Context) { m.View(c) }, 1280, 820)
	tt.SetScale(1)
	tt.Frame()
	for _, text := range []string{
		"What's New", "GoIDEA 1.2.3", "What's new", "Faster indexing",
		"Show this page after every update",
	} {
		if !tt.HasText(text) {
			t.Fatalf("%q is not on the page: %q", text, tt.Texts())
		}
	}
	m.CloseWhatsNew()
	if m.whatsNewTab() != nil {
		t.Fatal("the page did not close")
	}
}
