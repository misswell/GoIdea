package app

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestOpenPathsQueuesEveryFileWhileTheFirstIsOpening(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.go")
	second := filepath.Join(dir, "second.go")
	if err := os.WriteFile(first, []byte("package first\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("package second\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := New(Config{
		SettingsPath: filepath.Join(dir, "settings.json"),
	})
	t.Cleanup(m.off)
	ready := make(chan struct{})
	var once sync.Once
	m.postHook = func(fn func()) {
		fn()
		if len(m.tabs) == 2 {
			once.Do(func() { close(ready) })
		}
	}
	if !m.RunActionWithArg("file.openPaths", []any{first, second}) {
		t.Fatal("file.openPaths did not run")
	}
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatalf("batch did not finish: busy=%v queued=%d tabs=%d", m.busy, len(m.openQueue), len(m.tabs))
	}
	if len(m.tabs) != 2 || m.active < 0 || m.active >= len(m.tabs) {
		t.Fatalf("tabs=%d active=%d", len(m.tabs), m.active)
	}
	if m.openQueue != nil || m.busy {
		t.Fatalf("after the batch: busy=%v queue=%v", m.busy, m.openQueue)
	}
	got := map[string]bool{}
	for _, tab := range m.tabs {
		got[tab.doc.Path()] = true
	}
	if !got[realPath(first)] || !got[realPath(second)] {
		t.Fatalf("opened paths = %v", got)
	}
}

func TestOpenQueueContinuesPastAnAlreadyOpenFile(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.go")
	second := filepath.Join(dir, "second.go")
	third := filepath.Join(dir, "third.go")
	for path, text := range map[string]string{
		first:  "package first\n",
		second: "package second\n",
		third:  "package third\n",
	} {
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := New(Config{
		SettingsPath: filepath.Join(dir, "settings.json"),
	})
	t.Cleanup(m.off)

	opened := make(chan struct{})
	var openedOnce sync.Once
	m.postHook = func(fn func()) {
		fn()
		if m.tabForPath(realPath(first)) >= 0 {
			openedOnce.Do(func() { close(opened) })
		}
	}
	m.openFile(first)
	select {
	case <-opened:
	case <-time.After(5 * time.Second):
		t.Fatal("the initial file did not open")
	}

	done := make(chan struct{})
	var doneOnce sync.Once
	m.postHook = func(fn func()) {
		fn()
		if m.tabForPath(realPath(third)) >= 0 {
			doneOnce.Do(func() { close(done) })
		}
	}
	if !m.RunActionWithArg("file.openPaths", []any{second, first, third}) {
		t.Fatal("file.openPaths did not run")
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("batch stopped at the open file: busy=%v queued=%v tabs=%d",
			m.busy, len(m.openQueue), len(m.tabs))
	}
	deadline := time.Now().Add(5 * time.Second)
	for m.busy || len(m.openQueue) != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("batch did not finish: busy=%v queue=%v tabs=%d",
				m.busy, m.openQueue, len(m.tabs))
		}
		time.Sleep(10 * time.Millisecond)
	}
	for _, path := range []string{first, second, third} {
		if m.tabForPath(realPath(path)) < 0 {
			t.Fatalf("%s did not open", path)
		}
	}
}

func TestOpenFileActionsFollowOpenIDEAEntrances(t *testing.T) {
	m := &Model{}
	for _, id := range []string{
		"file.openFile",
		"file.openPaths",
		"file.openProjectPath",
		"file.openRecentProject",
	} {
		if !m.ActionEnabled(id) {
			t.Errorf("%s is not registered and enabled", id)
		}
	}
	for _, id := range []string{
		"file.openPaths",
		"file.openProjectPath",
		"file.openRecentProject",
	} {
		if !hiddenActionIDs[id] {
			t.Errorf("%s is not hidden from the command palette", id)
		}
	}
}

func TestOpenProjectPathStringFollowsPolicy(t *testing.T) {
	current, next := t.TempDir(), t.TempDir()
	host := &fakeWorkspaceHost{windows: 1, openProjectOK: true}
	m := &Model{workspaceHost: host, postHook: func(fn func()) { fn() }}
	m.setRoot(current)
	m.prefs.System.OpenProjectIn = "new"
	m.openProjectPath(next)
	if len(host.openProject) != 1 || host.openProject[0] != next || m.Root() != current {
		t.Fatalf("policy new window = host %v current %q", host.openProject, m.Root())
	}
}

func TestOpenProjectPathNewWindowIsExplicit(t *testing.T) {
	current, next := t.TempDir(), t.TempDir()
	host := &fakeWorkspaceHost{windows: 1, openProjectOK: true}
	m := &Model{workspaceHost: host, postHook: func(fn func()) { fn() }}
	m.setRoot(current)
	m.projectPathRequest(openProjectPathRequest{
		path: next, newWindow: boolPointer(true),
	})
	if len(host.openProject) != 1 || host.openProject[0] != next || m.Root() != current {
		t.Fatalf("explicit new window = host %v current %q", host.openProject, m.Root())
	}
}

func TestOpenProjectPathCurrentWindowUsesOrdinaryGuard(t *testing.T) {
	current, next := t.TempDir(), t.TempDir()
	on := true
	m := &Model{
		prefs:    Preferences{Run: &RunPrefs{ConfirmTerminate: &on}},
		postHook: func(fn func()) { fn() },
	}
	m.setRoot(current)
	m.run.running = true
	if !m.RunActionWithArg("file.openProjectPath", map[string]any{
		"path": next, "newWindow": false,
	}) {
		t.Fatal("object action did not run")
	}
	if m.run.terminateAsk == nil {
		t.Fatal("explicit current window bypassed the running-work guard")
	}
	if m.Root() != current || m.showingWelcome {
		t.Fatalf("guard moved project: root %q welcome %v", m.Root(), m.showingWelcome)
	}
	m.CancelRunTerminate()
}

func TestOpenProjectPathRequestRequiresBooleanNewWindow(t *testing.T) {
	request := openProjectPathRequestArg(map[string]any{
		"path": "/p", "newWindow": "true",
	})
	if request.path != "/p" || request.newWindow != nil {
		t.Fatalf("malformed object = %+v", request)
	}
	if got := projectPathArg(map[string]any{"path": "/p", "newWindow": true}); got != "/p" {
		t.Fatalf("legacy path helper = %q", got)
	}
}

func TestHelpMenuFollowsOpenIDEA(t *testing.T) {
	groups := mainMenuGroups()
	var help []string
	for _, group := range groups {
		if group.label != "Help" {
			continue
		}
		for _, item := range group.sub {
			help = append(help, item.label)
		}
	}
	want := []string{
		"Find Action…",
		"Keymap Reference",
		"What's New",
		"Show Log in Finder",
		"",
		"Clear Caches",
		"Show Memory Indicator Details",
		"Toggle Performance Overlay (Dev)",
		"",
		"Check for Updates…",
		"About GoIDEA",
	}
	if len(help) != len(want) {
		t.Fatalf("Help entries = %v, want %v", help, want)
	}
	for i := range want {
		if help[i] != want[i] {
			t.Fatalf("Help[%d] = %q, want %q (all: %v)", i, help[i], want[i], help)
		}
	}
}
