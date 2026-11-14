package app

// The visual regression: the shell, tool windows and overlays OpenIDEA's
// screenshots cover, rendered offscreen from a live model into one PNG per
// state. The e2e run drives these with a real project and backend, so the
// shots are of what the app actually draws, not of a mock.

import (
	"context"
	"image/png"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo/ui"
)

// smokeShotSize is the window the shots are taken at.
const (
	smokeShotWidth  = 1440
	smokeShotHeight = 900
)

// SmokeShots renders each visual-regression state into dir and reports the
// bytes written per state.
func (m *Model) SmokeShots(ctx context.Context, dir string) (map[string]any, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	out := map[string]any{}
	sizes := map[string]int{}
	var errs []string
	shot := func(name string) {
		var size int
		var errText string
		m.post(func() {
			tt := ui.NewTester(func(c *ui.Context) { m.View(c) }, smokeShotWidth, smokeShotHeight)
			tt.SetDark(true)
			tt.SetScale(1)
			tt.Frame()
			path := filepath.Join(dir, name+".png")
			f, err := os.Create(path)
			if err != nil {
				errText = err.Error()
				return
			}
			err = png.Encode(f, tt.Image())
			f.Close()
			if err != nil {
				errText = err.Error()
				return
			}
			if fi, err := os.Stat(path); err == nil {
				size = int(fi.Size())
			}
		})
		if errText != "" {
			errs = append(errs, name+": "+errText)
			return
		}
		sizes[name] = size
	}
	wait := func(done func() bool, limit time.Duration) bool {
		deadline := time.Now().Add(limit)
		for {
			var ok bool
			m.post(func() { ok = done() })
			if ok {
				return true
			}
			if time.Now().After(deadline) {
				return false
			}
			select {
			case <-ctx.Done():
				return false
			case <-time.After(150 * time.Millisecond):
			}
		}
	}

	// Workbench with the project tree and the file the harness opened.
	m.post(func() {
		m.showingWelcome = false
		m.welcomePage = "projects"
		m.sideMode = ""
		m.leftHidden = false
		if m.tab() == nil {
			if p := findSmokeFile(m.Root()); p != "" {
				m.openFile(p)
			}
		}
	})
	wait(func() bool { return m.tab() != nil }, 60*time.Second)
	shot("workbench")

	// The editor alone, as Distraction Free Mode shows it.
	m.post(func() { m.zen = true })
	shot("editor")
	m.post(func() { m.zen = false })

	// Bottom tool window: the TODO window over its scanned project.
	m.post(func() { m.ShowTodo() })
	wait(func() bool { return m.todo.done && !m.todo.running }, 90*time.Second)
	shot("bottom-todo")
	m.post(func() { m.CloseTodo() })

	// Two editor groups side by side.
	m.post(func() { m.SplitEditorRight() })
	wait(func() bool { return m.split != nil }, 30*time.Second)
	shot("split")
	m.post(func() { m.Unsplit() })

	// The right area, when a hierarchy was run in this phase.
	var hasHierarchy bool
	m.post(func() { hasHierarchy = m.hierarchy.active() })
	if hasHierarchy {
		m.post(func() {
			m.rightTool = "hierarchy"
			m.rightHidden = false
		})
		shot("right-hierarchy")
		m.post(func() { m.rightHidden = true })
	}

	// Settings as a dialog.
	m.post(func() { m.OpenSettingsPage(settingsPageAppearance) })
	wait(func() bool { return m.settingsOpen }, 30*time.Second)
	shot("settings")
	m.post(func() { m.closeSettings() })

	// Command palette and the quick-file popup.
	m.post(func() { m.OpenCommands() })
	wait(func() bool { return m.palette != nil }, 30*time.Second)
	shot("palette")
	m.post(func() { m.closeCommands() })
	m.post(func() { m.OpenQuickFiles() })
	wait(func() bool { return m.files != nil }, 30*time.Second)
	shot("popup-quick-files")
	m.post(func() { m.closeQuickFiles() })

	// Search Everywhere over the real index.
	m.post(func() {
		m.OpenSearchEverywhere()
		if m.se != nil {
			m.se.query = "App"
			m.refreshSearchEverywhere()
			m.clampSESelection()
		}
	})
	wait(func() bool { return m.se != nil && len(m.se.files) > 0 }, 60*time.Second)
	shot("search-everywhere")
	m.post(func() { m.CloseSearchEverywhere() })

	// The Services tool window over the detected build system.
	m.post(func() { m.ShowServices() })
	shot("services")
	m.post(func() { m.CloseServices() })

	// The left tool window: Structure in the split area below Project.
	m.post(func() { m.ShowStructure() })
	wait(func() bool { return m.leftSplit == "structure" && m.structure.visible }, 30*time.Second)
	shot("left-structure")
	m.post(func() { m.CloseStructure() })

	// File › New Project… as a real dialog.
	m.post(func() { m.NewProject() })
	shot("new-project")
	m.post(func() { m.CloseNewProject() })

	// A dialog: File › Invalidate Caches….
	m.post(func() { m.OpenInvalidateCaches() })
	wait(func() bool { return m.invalidateDlg != nil }, 30*time.Second)
	shot("dialog")
	m.post(func() { m.closeInvalidateCaches() })

	// Notifications: the bottom-right balloons and the tool window's history.
	m.post(func() { m.notifyGroupSample() })
	shot("balloons")
	m.post(func() { m.ShowNotifications() })
	shot("notifications")
	m.post(func() {
		m.CloseNotifications()
		m.ClearNotifications()
	})

	// Welcome: empty, then with recents, Customize and Plugins.
	m.post(func() { m.showWelcome() })
	wait(func() bool { return m.showingWelcome }, 30*time.Second)
	var recents []RecentProject
	m.post(func() { recents = m.recents; m.recents = nil })
	shot("welcome-empty")
	m.post(func() { m.recents = recents })
	shot("welcome")
	m.post(func() { m.welcomePage = "customize" })
	shot("welcome-customize")
	m.post(func() { m.openPlugins() })
	shot("welcome-plugins")

	m.post(func() {
		m.showingWelcome = false
		m.welcomePage = "projects"
		m.invalidate()
	})
	out["shots"] = sizes
	if len(errs) > 0 {
		out["shotErrors"] = errs
	}
	return out, nil
}
