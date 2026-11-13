package app

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

// workbenchTester renders the Islands workbench offscreen at 1440×900, so the
// shell is checked without opening a window.
func workbenchTester(t *testing.T, m *Model) *ui.Tester {
	t.Helper()
	if m.mgr == nil {
		// NewManager only builds its channels; it does not start a server, so
		// the status bar can read the backend state without a process.
		m.mgr = openidea.NewManager(openidea.LaunchConfig{})
	}
	tt := ui.NewTester(func(c *ui.Context) {
		m.workbench(c, c.Theme())
	}, 1440, 900)
	tt.SetDark(true)
	tt.SetScale(1)
	tt.Frame()
	if out := os.Getenv("WORKBENCH_SHOT"); out != "" {
		f, err := os.Create(out)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	return tt
}

func TestWorkbenchShellRenders(t *testing.T) {
	root := t.TempDir()
	m := &Model{}
	m.setRoot(root)
	tt := workbenchTester(t, m)
	for _, text := range []string{
		filepath.Base(root), "Search Everywhere", "Go to File",
		"Drop files here to open them", "openidea-server offline",
	} {
		if !tt.HasText(text) {
			t.Fatalf("%q is not in the workbench: %q", text, tt.Texts())
		}
	}
	// The Project tool window title sits in the 240px island right of the
	// 34px stripe, never under it, and the island fills the work area from the
	// title bar down rather than floating at its content height.
	if r, ok := tt.Find("Project"); !ok || r.X < 40 || r.X+4 > 46+oiLeftWidth || r.Y > 80 {
		t.Fatalf("Project is not at the top of the left island: %+v, %v", r, ok)
	}
	// The status bar occupies the last 34px of the window.
	if r, ok := tt.Find("openidea-server offline"); !ok || r.Y < 900-oiStatusBarHeight {
		t.Fatalf("the backend state is not in the status bar: %+v, %v", r, ok)
	}
}

func TestWorkbenchStripeTogglesTheIsland(t *testing.T) {
	m := &Model{}
	tt := workbenchTester(t, m)
	// The Project button is active by default, so its press hides the island.
	tt.ClickAt(25, 60)
	tt.Frame()
	if !m.leftHidden {
		t.Fatalf("the Project stripe button did not hide the island")
	}
	if tt.HasText("Project") {
		t.Fatalf("the left island kept its header: %q", tt.Texts())
	}
	tt.ClickAt(25, 60)
	tt.Frame()
	if m.leftHidden {
		t.Fatalf("a second press did not bring the island back")
	}
	if !tt.HasText("Project") {
		t.Fatalf("the left island did not return: %q", tt.Texts())
	}
}

func TestWorkbenchToolWindowTitles(t *testing.T) {
	m := &Model{sideMode: "structure"}
	tt := workbenchTester(t, m)
	if r, ok := tt.Find("Structure"); !ok || r.X < 40 || r.X >= 46+oiLeftWidth {
		t.Fatalf("Structure is not the left island title: %+v, %v", r, ok)
	}
}
