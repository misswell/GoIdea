package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"

	"mayoi/internal/openidea"
)

func TestCloseProjectReturnsToWelcome(t *testing.T) {
	m := &Model{postHook: func(fn func()) { fn() }}
	m.setRoot(filepath.Join(t.TempDir(), "proj"))
	m.showingWelcome = false
	m.CloseProject()
	if m.Root() != "" {
		t.Fatalf("the project stayed open as %q", m.Root())
	}
	if !m.showingWelcome || m.status != "Open a Java project" {
		t.Fatalf("welcome %v status %q", m.showingWelcome, m.status)
	}
	if m.busy {
		t.Fatal("the window is still busy after closing the project")
	}
}

func TestCloseProjectWithOpenIDEAConfirmDefaults(t *testing.T) {
	m := &Model{
		postHook: func(fn func()) { fn() },
		invokeHook: func(context.Context, string, any) (json.RawMessage, error) {
			return json.RawMessage(`null`), nil
		},
	}
	m.setRoot(filepath.Join(t.TempDir(), "proj"))
	m.CloseProject()
	deadline := time.Now().Add(time.Second)
	for m.busy && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if m.Root() != "" || !m.showingWelcome || m.busy {
		t.Fatalf("defaults: root %q welcome %v busy %v", m.Root(), m.showingWelcome, m.busy)
	}
}

func TestCloseProjectFromWelcomeStaysThere(t *testing.T) {
	m := &Model{postHook: func(fn func()) { fn() }}
	m.showingWelcome = true
	m.CloseProject()
	if m.Root() != "" || !m.showingWelcome {
		t.Fatalf("root %q welcome %v", m.Root(), m.showingWelcome)
	}
	found := false
	for _, cmd := range builtinCommands() {
		if cmd.id == "project.close" {
			found = true
		}
	}
	if !found {
		t.Fatal("the command registry lacks project.close")
	}
}

func TestCloseProjectCommitAfterBackendSuccess(t *testing.T) {
	project := filepath.Join(t.TempDir(), "proj")
	closed := make(chan struct{}, 1)
	m := &Model{
		invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			if command == "close_project" {
				select {
				case closed <- struct{}{}:
				default:
				}
			}
			return json.RawMessage(`null`), nil
		},
		postHook: func(fn func()) { fn() },
	}
	m.setRoot(project)
	m.CloseProject()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("CloseProject did not call the backend")
	}
	deadline := time.Now().Add(time.Second)
	for (m.Root() != "" || m.busy) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if m.Root() != "" || !m.showingWelcome || m.busy {
		t.Fatalf("successful close = root %q welcome %v busy %v", m.Root(), m.showingWelcome, m.busy)
	}
	if m.projectName != "" || m.projectDisplayName != "" || m.workspace != nil ||
		m.windowTitle("") != "GoIDEA" {
		t.Fatalf("project identity survived close: name=%q display=%q workspace=%+v title=%q",
			m.projectName, m.projectDisplayName, m.workspace, m.windowTitle(""))
	}
}

func TestResetProjectStateClearsProjectIdentity(t *testing.T) {
	m := &Model{postHook: func(fn func()) { fn() }}
	m.projectName = "internal"
	m.projectDisplayName = "Display"
	m.workspace = &openidea.Workspace{Name: "Workspace", Projects: []string{"/p"}}
	m.resetProjectState("")
	if m.projectName != "" || m.projectDisplayName != "" || m.workspace != nil ||
		m.projectLabel() != "GoIDEA" {
		t.Fatalf("reset identity = name %q display %q workspace %+v label %q",
			m.projectName, m.projectDisplayName, m.workspace, m.projectLabel())
	}
}

func TestCloseProjectSuccessDoesNotPersistRemovedEditorState(t *testing.T) {
	project := t.TempDir()
	sessionPath := filepath.Join(t.TempDir(), "session.json")
	recent := filepath.Join(t.TempDir(), "recent")
	m := &Model{
		cfg:      Config{SessionPath: sessionPath},
		postHook: func(fn func()) { fn() },
		invokeHook: func(context.Context, string, any) (json.RawMessage, error) {
			return json.RawMessage(`null`), nil
		},
	}
	doc := fileActionDocument(t, false)
	tab := m.newTab(doc)
	m.tabs = []*Tab{tab}
	m.split = &splitGroup{tabs: []*Tab{{doc: doc}}, active: 0}
	m.active = 0
	if err := os.Mkdir(recent, 0o755); err != nil {
		t.Fatal(err)
	}
	m.recents = []RecentProject{{Path: recent, Name: filepath.Base(recent)}}
	m.setRoot(project)
	m.closeProjectCommitted(project, nil)
	if len(m.tabs) != 0 || m.split != nil || len(m.recents) != 1 {
		t.Fatalf("close state = tabs %d split %+v recents %+v", len(m.tabs), m.split, m.recents)
	}
	m.saveSessionNow()

	session := LoadSession(sessionPath)
	if session.Root != "" || len(session.Files) != 0 || len(session.SplitFiles) != 0 ||
		len(session.Recents) != 1 || session.Recents[0].Path != recent {
		t.Fatalf("persisted close state = root %q files %v split %v recents %+v",
			session.Root, session.Files, session.SplitFiles, session.Recents)
	}
}

func TestCloseProjectFailurePreservesProjectAndTabsAndOffersRetry(t *testing.T) {
	project := filepath.Join(t.TempDir(), "proj")
	backendCalls := 0
	m := &Model{
		invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			if command != "close_project" {
				return json.RawMessage(`null`), nil
			}
			backendCalls++
			if backendCalls == 1 {
				return nil, errors.New("backend unavailable")
			}
			return json.RawMessage(`null`), nil
		},
		postHook: func(fn func()) { fn() },
	}
	doc := fileActionDocument(t, false)
	tab := m.newTab(doc)
	m.tabs = []*Tab{tab}
	m.active = 0
	m.setRoot(project)
	m.showingWelcome = false
	m.closeProjectNow()
	deadline := time.Now().Add(time.Second)
	for backendCalls == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	for m.busy && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if m.Root() != project || m.showingWelcome || len(m.tabs) != 1 || m.tabs[0] != tab || m.busy {
		t.Fatalf("failed close mutated the window: root=%q welcome=%v tabs=%d busy=%v",
			m.Root(), m.showingWelcome, len(m.tabs), m.busy)
	}
	if m.banner == "" {
		t.Fatal("failed close did not show the backend error")
	}
	var retry *ideNotification
	for i := range m.notify.items {
		if m.notify.items[i].key == "project.close" {
			retry = &m.notify.items[i]
			break
		}
	}
	if retry == nil || len(retry.actions) != 1 || retry.actions[0].label != "Retry" {
		t.Fatalf("retry notification = %+v", retry)
	}
	retry.actions[0].run(m)
	deadline = time.Now().Add(time.Second)
	for backendCalls < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	for m.busy && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if backendCalls != 2 || m.Root() != "" || !m.showingWelcome || len(m.tabs) != 0 {
		t.Fatalf("retry = calls %d root %q welcome %v tabs %d",
			backendCalls, m.Root(), m.showingWelcome, len(m.tabs))
	}
}

func TestBackendFailureReleasesCloseGuard(t *testing.T) {
	project := filepath.Join(t.TempDir(), "proj")
	m := &Model{
		invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			if command == "close_project" {
				return nil, errors.New("cannot close")
			}
			return json.RawMessage(`null`), nil
		},
		postHook: func(fn func()) { fn() },
	}
	m.setRoot(project)
	m.closeProjectNow()
	deadline := time.Now().Add(time.Second)
	for m.busy && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if m.busy || m.Root() != project {
		t.Fatalf("failed close guard = root %q busy %v", m.Root(), m.busy)
	}
}

func TestLastWindowCloseReturnsToWelcome(t *testing.T) {
	project := filepath.Join(t.TempDir(), "proj")
	host := &fakeWorkspaceHost{windows: 1}
	m := &Model{workspaceHost: host, postHook: func(fn func()) { fn() }}
	m.setRoot(project)
	m.windowCloseAfterProcessGuard()
	if m.Root() != "" || !m.showingWelcome {
		t.Fatalf("last window = root %q welcome %v", m.Root(), m.showingWelcome)
	}
	if m.quitting {
		t.Fatal("returning to Welcome left the close guard active")
	}
	if host.immediate != 0 {
		t.Fatal("last window was destroyed instead of returning to Welcome")
	}
}

func TestWelcomeWindowCloseProceedsInsteadOfReopeningWelcome(t *testing.T) {
	host := &fakeWorkspaceHost{windows: 1}
	confirmExit := false
	m := &Model{
		workspaceHost: host,
		postHook:      func(fn func()) { fn() },
		confirmExitHook: func(done func(bool, bool)) {
			confirmExit = true
			done(true, false)
		},
	}
	m.showingWelcome = true
	event := &mygo.CloseEvent{}
	m.registerWindowCloseHandler(func(callback func(*mygo.CloseEvent)) func() {
		callback(event)
		return func() {}
	})
	if !event.DefaultPrevented() {
		t.Fatal("the close request did not prevent the native window from closing first")
	}
	if !m.forceQuit {
		t.Fatal("the Welcome window did not proceed through shutdown")
	}
	if m.applicationQuit {
		t.Fatal("closing a Welcome window incorrectly became an application quit")
	}
	if !confirmExit {
		t.Fatal("the last Welcome window skipped general.confirmExit")
	}
}

func TestWelcomeWindowCloseCancelKeepsTheWindowOpen(t *testing.T) {
	host := &fakeWorkspaceHost{windows: 1}
	posted := make(chan func(), 1)
	m := &Model{
		workspaceHost: host,
		postHook:      func(fn func()) { posted <- fn },
		confirmExitHook: func(done func(bool, bool)) {
			done(false, false)
		},
	}
	m.showingWelcome = true
	event := &mygo.CloseEvent{}
	m.handleWindowClose(event)
	if !event.DefaultPrevented() {
		t.Fatal("the close request did not prevent the native window from closing first")
	}
	if m.forceQuit || m.quitting || m.applicationQuit {
		t.Fatalf("cancelled Welcome close changed lifecycle state: force=%v quitting=%v appQuit=%v",
			m.forceQuit, m.quitting, m.applicationQuit)
	}
	select {
	case <-posted:
		t.Fatal("cancelled Welcome close still called the main-thread shutdown")
	default:
	}
}

func TestClosingOneOfSeveralWelcomeWindowsDoesNotAskToExit(t *testing.T) {
	host := &fakeWorkspaceHost{windows: 2}
	confirmExit := false
	m := &Model{
		workspaceHost: host,
		postHook:      func(fn func()) { fn() },
		confirmExitHook: func(done func(bool, bool)) {
			confirmExit = true
			done(false, false)
		},
	}
	m.showingWelcome = true
	event := &mygo.CloseEvent{}
	m.handleWindowClose(event)
	if !event.DefaultPrevented() {
		t.Fatal("the close request did not prevent the native window from closing first")
	}
	if confirmExit {
		t.Fatal("closing one of several windows asked to quit the application")
	}
	if !m.forceQuit || m.applicationQuit {
		t.Fatalf("multi-window Welcome close = force=%v appQuit=%v", m.forceQuit, m.applicationQuit)
	}
}

func TestWelcomeWindowCloseCanPersistDontAskAgain(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	host := &fakeWorkspaceHost{windows: 1}
	m := &Model{
		cfg:           Config{SettingsPath: settingsPath},
		workspaceHost: host,
		postHook:      func(fn func()) { fn() },
		confirmExitHook: func(done func(bool, bool)) {
			done(true, true)
		},
	}
	m.showingWelcome = true
	event := &mygo.CloseEvent{}
	m.handleWindowClose(event)
	if !m.forceQuit {
		t.Fatal("the confirmed Welcome close did not finish")
	}
	if m.prefs.System.confirmExit() || LoadPreferences(settingsPath).System.confirmExit() {
		t.Fatal("Don't ask again did not persist general.confirmExit=false")
	}
}

func TestLastProjectWindowReturnsToWelcomeWithoutExitConfirmation(t *testing.T) {
	project := filepath.Join(t.TempDir(), "project")
	confirmExit := false
	m := &Model{
		workspaceHost: &fakeWorkspaceHost{windows: 1},
		postHook:      func(fn func()) { fn() },
		confirmExitHook: func(done func(bool, bool)) {
			confirmExit = true
			done(true, false)
		},
	}
	m.setRoot(project)
	m.handleWindowClose(&mygo.CloseEvent{})
	if confirmExit {
		t.Fatal("returning the last project session to Welcome asked to exit")
	}
	if m.Root() != "" || !m.showingWelcome || m.forceQuit || m.quitting {
		t.Fatalf("last project close = root %q welcome %v force %v quitting %v",
			m.Root(), m.showingWelcome, m.forceQuit, m.quitting)
	}
}

func TestLastProjectWindowWithCloseWithoutWelcomeConfirmsExit(t *testing.T) {
	project := filepath.Join(t.TempDir(), "project")
	confirmExit := false
	m := &Model{
		workspaceHost: &fakeWorkspaceHost{windows: 1},
		postHook:      func(fn func()) { fn() },
		confirmExitHook: func(done func(bool, bool)) {
			confirmExit = true
			done(false, false)
		},
	}
	m.setRoot(project)
	m.prefs.System.CloseWithoutWelcome = true
	m.handleWindowClose(&mygo.CloseEvent{})
	if !confirmExit {
		t.Fatal("closeWithoutWelcome skipped the last-window exit confirmation")
	}
	if m.forceQuit || m.quitting || m.Root() != project || m.showingWelcome {
		t.Fatalf("cancelled exit changed the project: root %q welcome %v force %v quitting %v",
			m.Root(), m.showingWelcome, m.forceQuit, m.quitting)
	}
}

func TestApplicationExitUsesTheSameDontAskConfirmation(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	m := &Model{
		cfg:      Config{SettingsPath: settingsPath},
		postHook: func(fn func()) { fn() },
		confirmExitHook: func(done func(bool, bool)) {
			done(true, true)
		},
	}
	confirmed := false
	m.ConfirmApplicationExit(func(ok bool) {
		confirmed = ok
	})
	if !confirmed {
		t.Fatal("application exit confirmation was not accepted")
	}
	if m.prefs.System.confirmExit() || LoadPreferences(settingsPath).System.confirmExit() {
		t.Fatal("application exit did not persist Don't ask again")
	}
}

func TestWelcomeWindowCloseStopsTheBackend(t *testing.T) {
	manager := openidea.NewManager(openidea.LaunchConfig{})
	m := &Model{
		ctx:           context.Background(),
		mgr:           manager,
		workspaceHost: &fakeWorkspaceHost{windows: 1},
		postHook:      func(fn func()) { fn() },
		confirmExitHook: func(done func(bool, bool)) {
			done(true, false)
		},
		cancel: func() {},
	}
	m.showingWelcome = true
	m.handleWindowClose(&mygo.CloseEvent{})

	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := manager.Ensure(context.Background()); errors.Is(err, openidea.ErrClosed) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("Welcome close did not close the backend manager: %+v", manager.State())
		}
		time.Sleep(time.Millisecond)
	}
	if manager.Server() != nil {
		t.Fatal("Welcome close left a live backend server")
	}
}

func TestNonLastWindowCloseKeepsClosePath(t *testing.T) {
	host := &fakeWorkspaceHost{windows: 2}
	m := &Model{workspaceHost: host, postHook: func(fn func()) { fn() }}
	m.setRoot(filepath.Join(t.TempDir(), "proj"))
	m.quitting = true
	if m.returnLastSessionToWelcome() {
		t.Fatal("a non-last window returned to Welcome")
	}
}

func TestCloseWithoutWelcomeBypassesWelcomeOnLastWindow(t *testing.T) {
	host := &fakeWorkspaceHost{windows: 1}
	m := &Model{workspaceHost: host, postHook: func(fn func()) { fn() }}
	m.setRoot(filepath.Join(t.TempDir(), "proj"))
	m.prefs.System.CloseWithoutWelcome = true
	m.quitting = true
	if m.returnLastSessionToWelcome() {
		t.Fatal("closeWithoutWelcome returned to Welcome")
	}
}

func TestLastEditorTabReturnsToWelcome(t *testing.T) {
	host := &fakeWorkspaceHost{windows: 1}
	m := &Model{workspaceHost: host, postHook: func(fn func()) { fn() }}
	tab := &Tab{}
	m.tabs = []*Tab{tab}
	m.removeTabFromWindow(tab, false)
	if !m.showingWelcome || m.status != "Open a file" {
		t.Fatalf("last tab = welcome %v status %q", m.showingWelcome, m.status)
	}
}

func TestCloseProjectSavingCanBeObservedWithoutWindows(t *testing.T) {
	m := &Model{
		invokeHook: func(context.Context, string, any) (json.RawMessage, error) {
			return json.RawMessage(`null`), nil
		},
		postHook: func(fn func()) { fn() },
	}
	m.setRoot(filepath.Join(t.TempDir(), "proj"))
	m.busy = true
	m.tabs = []*Tab{{}}
	m.CloseProject()
	if m.Root() == "" {
		t.Fatal("CloseProject bypassed the busy guard")
	}
}

func TestRetryCloseProjectWaitsForAReply(t *testing.T) {
	reply := make(chan error, 1)
	m := &Model{
		invokeHook: func(context.Context, string, any) (json.RawMessage, error) {
			return nil, <-reply
		},
		postHook: func(fn func()) { fn() },
	}
	m.setRoot(filepath.Join(t.TempDir(), "proj"))
	m.closeProjectNow()
	if m.Root() == "" {
		t.Fatal("close committed before the backend replied")
	}
	reply <- errors.New("failed")
	deadline := time.Now().Add(time.Second)
	for m.Root() == "" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if m.Root() == "" {
		t.Fatal("failed close committed the reset")
	}
}

func TestDropFolderUsesNativeProjectWindow(t *testing.T) {
	root := t.TempDir()
	folder := t.TempDir()
	files := make([]string, 0)
	folders := make([]string, 0)
	m := &Model{postHook: func(fn func()) { fn() }}
	m.setRoot(root)
	m.invokeHook = func(context.Context, string, any) (json.RawMessage, error) {
		return json.RawMessage(`null`), nil
	}
	m.openDroppedFolderHook = func(path string) { folders = append(folders, path) }
	m.openDroppedPathsWithFiles([]string{folder}, func(paths []string) {
		files = append(files, paths...)
	})
	if len(folders) != 1 || folders[0] != realPath(folder) || len(files) != 0 {
		t.Fatalf("folder drop = folders %v files %v", folders, files)
	}
}

func TestDropFilesUseEditorForTheReceivingWindow(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "Loose.txt")
	if err := os.WriteFile(file, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	var got []string
	m := &Model{postHook: func(fn func()) { fn() }}
	m.openDroppedPathsWithFiles([]string{file}, func(paths []string) {
		got = append(got, paths...)
	})
	if len(got) != 1 || got[0] != realPath(file) {
		t.Fatalf("file drop = %v, want %s", got, realPath(file))
	}
}

func TestOpenCreatedProjectFromExistingProjectUsesNewWindow(t *testing.T) {
	current, created := t.TempDir(), t.TempDir()
	host := &fakeWorkspaceHost{windows: 1, openProjectOK: true}
	m := &Model{workspaceHost: host, postHook: func(fn func()) { fn() }}
	m.setRoot(current)
	m.openCreatedProject(created)
	if len(host.openProject) != 1 || host.openProject[0] != created || m.Root() != current {
		t.Fatalf("created project = window %v root %q", host.openProject, m.Root())
	}
}

func TestOpenCreatedProjectFallbackRunsLeaveGuard(t *testing.T) {
	current, created := t.TempDir(), t.TempDir()
	on := true
	m := &Model{
		prefs:    Preferences{Run: &RunPrefs{ConfirmTerminate: &on}},
		postHook: func(fn func()) { fn() },
		invokeHook: func(context.Context, string, any) (json.RawMessage, error) {
			return json.RawMessage(`null`), nil
		},
	}
	m.setRoot(current)
	m.run.running = true
	m.openCreatedProject(created)
	if m.run.terminateAsk == nil {
		t.Fatal("creating a project in the current window bypassed the running-work guard")
	}
	m.CancelRunTerminate()
}

func TestMissingRecentProjectOffersRemovalAndKeepsCurrentProject(t *testing.T) {
	current := t.TempDir()
	missing := filepath.Join(t.TempDir(), "gone")
	prompted := make(chan string, 1)
	m := &Model{
		recents:  []RecentProject{{Path: missing, Name: "gone"}},
		postHook: func(fn func()) { fn() },
		openRecentDialog: func(title string, _ error) (bool, error) {
			prompted <- title
			return true, nil
		},
	}
	m.setRoot(current)
	m.OpenRecentProject(missing)
	select {
	case title := <-prompted:
		if !strings.Contains(title, "not found") {
			t.Fatalf("prompt title = %q", title)
		}
	case <-time.After(time.Second):
		t.Fatal("missing recent project did not offer removal")
	}
	deadline := time.Now().Add(time.Second)
	for len(m.recents) != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(m.recents) != 0 || m.Root() != current || m.showingWelcome {
		t.Fatalf("missing recent = recents %+v root %q welcome %v", m.recents, m.Root(), m.showingWelcome)
	}
}

func TestRecentProjectOpenFailureHasRemovalAction(t *testing.T) {
	current := t.TempDir()
	missing := filepath.Join(t.TempDir(), "gone")
	var prompt bool
	m := &Model{
		recents:  []RecentProject{{Path: missing, Name: "gone"}},
		postHook: func(fn func()) { fn() },
		openRecentHook: func(_ string, remove func()) bool {
			prompt = true
			remove()
			return false
		},
	}
	m.setRoot(current)
	m.OpenRecentProject(missing)
	if !prompt || len(m.recents) != 0 || m.Root() != current {
		t.Fatalf("recent failure = prompt %v recents %+v root %q", prompt, m.recents, m.Root())
	}
}
