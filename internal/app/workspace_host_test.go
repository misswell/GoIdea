package app

import (
	"reflect"
	"testing"
	"time"
)

type fakeWorkspaceHost struct {
	windows        int
	openProject    []string
	openProjectOK  bool
	openEmpty      int
	focusPrevious  []bool
	merge          int
	quit           int
	relaunch       int
	immediate      int
	menuChanges    int
	openProjects   []WorkspaceWindow
	focusedProject string
}

func (h *fakeWorkspaceHost) OpenProjectInNewWindow(path string, files []string) bool {
	h.openProject = []string{path}
	h.openProject = append(h.openProject, files...)
	if h.openProjectOK || h.windows == 0 {
		h.windows++
		return true
	}
	return false
}

func (h *fakeWorkspaceHost) OpenNewWindow() bool {
	h.openEmpty++
	h.windows++
	return true
}

func (h *fakeWorkspaceHost) FocusProjectWindow(previous bool) bool {
	h.focusPrevious = append(h.focusPrevious, previous)
	return h.windows > 1
}

func (h *fakeWorkspaceHost) DetachTabToNewWindow(_ TabSnapshot, done func(bool) bool) bool {
	return done(false)
}

func (h *fakeWorkspaceHost) MergeProjectWindows(*Model) bool {
	h.merge++
	return h.windows > 1
}

func (h *fakeWorkspaceHost) WorkspaceWindows(*Model) []WorkspaceWindow { return nil }

func (h *fakeWorkspaceHost) OpenProjects(*Model) []WorkspaceWindow {
	return append([]WorkspaceWindow(nil), h.openProjects...)
}

func (h *fakeWorkspaceHost) FocusOpenProject(_ *Model, path string) bool {
	h.focusedProject = path
	return true
}

func (h *fakeWorkspaceHost) FocusWorkspaceWindow(string) bool { return false }

func (h *fakeWorkspaceHost) WindowStateChanged(*Model) {}

func (h *fakeWorkspaceHost) WindowCount() int { return h.windows }

func (h *fakeWorkspaceHost) WindowFocused(*Model) {}

func (h *fakeWorkspaceHost) RequestApplicationQuit() { h.quit++ }

func (h *fakeWorkspaceHost) RequestRelaunch() { h.relaunch++ }

func (h *fakeWorkspaceHost) QuitImmediately() { h.immediate++ }

func (h *fakeWorkspaceHost) WindowMenuChanged(*Model) { h.menuChanges++ }

func TestNativeProjectWindowActionsUseTheWorkspaceHost(t *testing.T) {
	host := &fakeWorkspaceHost{windows: 1}
	m := &Model{workspaceHost: host}

	m.OpenProjectInNewWindow("/projects/backend")
	if got, want := host.openProject, []string{"/projects/backend"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("project window argument = %q, want %q", got, want)
	}
	if !m.RunAction("window.new") || host.openEmpty != 1 {
		t.Fatalf("New Window did not create an empty native window: %+v", host)
	}
	if !m.RunAction("window.next") || len(host.focusPrevious) != 1 || host.focusPrevious[0] {
		t.Fatalf("Next Window did not focus the next native window: %+v", host)
	}
	if !m.RunAction("window.previous") || len(host.focusPrevious) != 2 || !host.focusPrevious[1] {
		t.Fatalf("Previous Window did not focus the previous native window: %+v", host)
	}
	m.root = "/projects/backend"
	if !m.RunAction("window.mergeAllWindows") || host.merge != 1 {
		t.Fatalf("Merge All Project Windows did not reach the native workspace host: %+v", host)
	}
}

func TestNewProjectPolicyCreatesANativeWindowAndCurrentPolicyDoesNot(t *testing.T) {
	root := t.TempDir()
	next := t.TempDir()
	host := &fakeWorkspaceHost{windows: 1, openProjectOK: true}
	m := &Model{
		root:          root,
		workspaceHost: host,
		postHook:      func(fn func()) { fn() },
	}
	m.prefs.System.OpenProjectIn = "new"
	m.confirmSwitchWithFiles(next, []string{next + "/Main.java"})
	if got, want := host.openProject, []string{next, next + "/Main.java"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("New policy opened %q, want %q", got, want)
	}

	host.openProject = nil
	m.prefs.System.OpenProjectIn = "same"
	m.busy = true
	m.confirmSwitchWithFiles(next, nil)
	if len(host.openProject) != 0 {
		t.Fatalf("Current policy created a native window: %q", host.openProject)
	}
	if m.Root() != root {
		t.Fatalf("Current policy moved the project while busy: %q", m.Root())
	}
}

func TestClosePolicyUsesLiveWindowCount(t *testing.T) {
	root := t.TempDir()
	host := &fakeWorkspaceHost{windows: 1}
	m := &Model{workspaceHost: host, postHook: func(fn func()) { fn() }}
	m.setRoot(root)
	if !m.returnLastSessionToWelcome() {
		t.Fatal("one live window did not follow Close Project")
	}
	if m.Root() != "" || !m.showingWelcome {
		t.Fatalf("one live window = root %q welcome %v", m.Root(), m.showingWelcome)
	}
	m.setRoot(root)
	host.windows = 2
	if m.returnLastSessionToWelcome() {
		t.Fatal("two live windows returned to Welcome")
	}
}

func TestClosePolicyLeavesOneModelOfManyOpen(t *testing.T) {
	root := t.TempDir()
	first := &Model{postHook: func(fn func()) { fn() }}
	second := &Model{postHook: func(fn func()) { fn() }}
	host := &workspaceHostPair{first: first, second: second}
	host.windows = 2
	first.workspaceHost, second.workspaceHost = host, host
	first.setRoot(root)
	second.setRoot(t.TempDir())
	if first.returnLastSessionToWelcome() {
		t.Fatal("one model treated a two-window session as last")
	}
	host.windows = 1
	if !second.returnLastSessionToWelcome() || second.Root() != "" || !second.showingWelcome {
		t.Fatalf("last model = root %q welcome %v", second.Root(), second.showingWelcome)
	}
}

type workspaceHostPair struct {
	fakeWorkspaceHost
	first, second *Model
	windows       int
}

func (h *workspaceHostPair) WindowCount() int { return h.windows }

func TestMainMenuExposesNativeProjectWindowCommands(t *testing.T) {
	menu := (&Model{}).MainMenu()
	for _, id := range []string{
		"menu.file.openInNewWindow",
		"menu.window.new",
		"menu.window.next",
		"menu.window.previous",
	} {
		if menu.ItemByID(id) == nil {
			t.Fatalf("main menu lacks %s", id)
		}
	}
}

func TestModelQuitMethodsDelegateToTheWindowHost(t *testing.T) {
	host := &fakeWorkspaceHost{windows: 2}
	m := &Model{workspaceHost: host}
	m.requestApplicationQuit()
	m.requestApplicationRelaunch()
	m.QuitImmediately()
	if host.quit != 1 || host.relaunch != 1 || host.immediate != 1 {
		t.Fatalf("host lifecycle calls = quit %d relaunch %d immediate %d",
			host.quit, host.relaunch, host.immediate)
	}
}

func TestKeymapPresetChangeRefreshesTheSharedNativeMenu(t *testing.T) {
	host := &fakeWorkspaceHost{windows: 1}
	m := &Model{workspaceHost: host}
	m.SetKeymapPresetPref("vscode-macos")
	if host.menuChanges != 1 {
		t.Fatalf("native menu refreshes = %d, want 1", host.menuChanges)
	}
}

func TestApplicationQuitPreparationReportsAProcessPromptCancellation(t *testing.T) {
	m := &Model{}
	m.run.running = true
	completed := make(chan [2]bool, 1)

	m.PrepareApplicationQuit(func(save, ok bool) {
		completed <- [2]bool{save, ok}
	})

	if m.run.terminateAsk == nil {
		t.Fatal("application quit did not guard the running process")
	}
	m.CancelRunTerminate()

	select {
	case result := <-completed:
		if result != [2]bool{false, false} {
			t.Fatalf("cancelled quit result = save:%v ok:%v", result[0], result[1])
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled process prompt did not complete the application quit")
	}
	if m.quitting {
		t.Fatal("cancelled process prompt left application quit preparation active")
	}
}
