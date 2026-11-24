package app

import (
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui/editor"
)

// WorkspaceWindow describes one native project window for the project-tab
// strip. Group is zero for a standalone window. Selected is true for the
// visible member of a group.
type WorkspaceWindow struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Group    int    `json:"group,omitempty"`
	Selected bool   `json:"selected,omitempty"`
	Dirty    bool   `json:"dirty,omitempty"`
}

// TabSnapshot is the complete editor state needed to move a file tab between
// native project windows with independent backends.
type TabSnapshot struct {
	Root       string             `json:"root"`
	Path       string             `json:"path"`
	Text       string             `json:"text"`
	SavedText  string             `json:"savedText"`
	Selections []editor.Selection `json:"selections,omitempty"`
	ScrollY    float64            `json:"scrollY,omitempty"`
	Pinned     bool               `json:"pinned,omitempty"`
	Preview    bool               `json:"preview,omitempty"`
	ReadOnly   bool               `json:"readOnly,omitempty"`
}

// WorkspaceHost owns the native project windows of one GoIDEA process. The
// application package keeps the project-window actions and lifecycle here so
// the command can provide the native MyGo windows while tests can use a small
// in-memory host.
type WorkspaceHost interface {
	// OpenProjectInNewWindow creates an independent model, backend server and
	// native window for the project. It reports whether the window was made.
	OpenProjectInNewWindow(path string, files []string) bool
	// OpenNewWindow creates a native window at Welcome.
	OpenNewWindow() bool
	// FocusProjectWindow focuses the next or previous native project window.
	FocusProjectWindow(previous bool) bool
	// DetachTabToNewWindow creates a native project window and restores
	// snapshot in it. done receives whether the target restored successfully
	// and returns whether the source tab was removed. A false result closes
	// the target, so a failed or cancelled move leaves the source intact.
	DetachTabToNewWindow(snapshot TabSnapshot, done func(bool) bool) bool
	// MergeProjectWindows groups every project window while retaining each
	// model and backend. The model passed here becomes the visible member.
	MergeProjectWindows(anchor *Model) bool
	// WorkspaceWindows lists the windows for the project-tab strip.
	WorkspaceWindows(*Model) []WorkspaceWindow
	// FocusWorkspaceWindow shows and focuses one window in the current group.
	FocusWorkspaceWindow(id string) bool
	// WindowStateChanged persists a project root, close or title change.
	WindowStateChanged(*Model)
	// WindowCount is the number of live native project windows.
	WindowCount() int
	// WindowFocused tells the host which window owns the shared application
	// menu and native window commands.
	WindowFocused(m *Model)
	// RequestApplicationQuit runs the multi-window quit preflight.
	RequestApplicationQuit()
	// RequestRelaunch runs the same preflight and relaunches after every
	// window has closed its backend and saved its state.
	RequestRelaunch()
	// QuitImmediately closes every window without prompts, for headless smoke
	// runs and fatal test cleanup.
	QuitImmediately()
}

// projectTabHost controls the native windows represented by a grouped project's strip.
type projectTabHost interface {
	CloseWorkspaceWindows(caller *Model, keep string, ids []string)
	ReorderWorkspaceWindows(caller *Model, ids []string) bool
	DetachWorkspaceWindow(caller *Model) bool
}

// nativeMenuHost is implemented by the desktop host when the shared menu bar
// needs rebuilding after a window changes the active keymap.
type nativeMenuHost interface {
	WindowMenuChanged(*Model)
}

func (m *Model) setWorkspaceHost(host WorkspaceHost) {
	m.workspaceHost = host
}

// SetWorkspaceHost attaches the native-window controller after the model is
// constructed and before its window starts.
func (m *Model) SetWorkspaceHost(host WorkspaceHost) {
	m.setWorkspaceHost(host)
}

func (m *Model) windowMenuChanged() {
	if m == nil {
		return
	}
	if host, ok := m.workspaceHost.(nativeMenuHost); ok {
		host.WindowMenuChanged(m)
	}
}

// ProjectWindowCount reports the number of windows the host owns, or one for a
// standalone model in tests.
func (m *Model) ProjectWindowCount() int {
	if m == nil || m.workspaceHost == nil {
		return 1
	}
	return max(1, m.workspaceHost.WindowCount())
}

func (m *Model) openProjectInNativeWindow(path string, files []string) bool {
	if m == nil || m.workspaceHost == nil {
		return false
	}
	return m.workspaceHost.OpenProjectInNewWindow(path, append([]string(nil), files...))
}

func (m *Model) openEmptyNativeWindow() bool {
	if m == nil || m.workspaceHost == nil {
		return false
	}
	return m.workspaceHost.OpenNewWindow()
}

func (m *Model) focusNativeProjectWindow(previous bool) bool {
	if m == nil || m.workspaceHost == nil || m.workspaceHost.WindowCount() < 2 {
		return false
	}
	return m.workspaceHost.FocusProjectWindow(previous)
}

func (m *Model) detachTabToNativeWindow(snapshot TabSnapshot, done func(bool) bool) bool {
	if m == nil || m.workspaceHost == nil {
		return false
	}
	return m.workspaceHost.DetachTabToNewWindow(snapshot, done)
}

func (m *Model) mergeNativeProjectWindows() bool {
	if m == nil || m.workspaceHost == nil {
		return false
	}
	return m.workspaceHost.MergeProjectWindows(m)
}

func (m *Model) workspaceWindows() []WorkspaceWindow {
	if m == nil || m.workspaceHost == nil {
		return nil
	}
	return m.workspaceHost.WorkspaceWindows(m)
}

func (m *Model) focusWorkspaceWindow(id string) bool {
	if m == nil || m.workspaceHost == nil {
		return false
	}
	return m.workspaceHost.FocusWorkspaceWindow(id)
}

func (m *Model) workspaceStateChanged() {
	if m == nil || m.workspaceHost == nil {
		return
	}
	m.workspaceHost.WindowStateChanged(m)
}

func (m *Model) requestApplicationQuit() {
	if m == nil {
		return
	}
	if m.workspaceHost != nil {
		m.workspaceHost.RequestApplicationQuit()
		return
	}
	m.requestQuit(func() {})
}

func (m *Model) requestApplicationRelaunch() {
	if m == nil {
		return
	}
	if m.workspaceHost != nil {
		m.workspaceHost.RequestRelaunch()
		return
	}
	m.restartPending = true
	m.requestQuit(func() {})
}

// CloseWorkspaceTab uses the running-work guard and waits for unsaved files
// in both editor panes before destroying just this window and its services.
func (m *Model) CloseWorkspaceTab(done func(bool)) {
	if m.quitting || m.forceQuit || m.win == nil {
		done(false)
		return
	}
	m.quitting = true
	cancel := func() {
		m.cancelQuitPreparation()
		done(false)
	}
	closeWindow := func() {
		m.shutdown(false, func() {
			m.win.Destroy()
			done(true)
		})
	}
	m.requestRunningWorkTermination(func() {
		tabs := m.allTabs()
		var dirty []string
		for _, t := range tabs {
			if t.unsaved() {
				dirty = append(dirty, tabPath(t))
			}
		}
		if len(dirty) == 0 {
			closeWindow()
			return
		}
		go func() {
			choice, err := mygo.Dialog.Message(mygo.MessageOptions{
				Parent: m.win, Type: mygo.MessageWarning, Title: "Close Project Tab",
				Message: "There are unsaved changes. Save before closing this project?", Detail: strings.Join(dirty, "\n"),
				Buttons: []string{"Save and Close", "Discard Changes", "Cancel"}, DefaultButton: 0, CancelButton: 2,
			})
			if err != nil {
				m.post(cancel)
				return
			}
			switch choice.Button {
			case 0:
				err := m.saveOpenTabs(m.ctx, tabs)
				m.post(func() {
					if err != nil {
						m.fail("", err)
						cancel()
						return
					}
					if m.WorkspaceDirty() {
						cancel()
						return
					}
					closeWindow()
				})
			case 1:
				m.post(closeWindow)
			default:
				m.post(cancel)
			}
		}()
	}, cancel)
}
