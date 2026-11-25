package app

// The visual regression: the shell, tool windows and overlays OpenIDEA's
// screenshots cover, rendered offscreen from a live model into one PNG per
// state. The e2e run drives these with a real project and backend, so the
// shots are of what the app actually draws, not of a mock.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	// The database shots execute real SQL. A reused E2E data directory may
	// not retain trust for this run's unique project path, so establish it
	// through the same backend setting the user's trust prompt writes.
	if root := m.Root(); root != "" && m.mgr != nil {
		trustCtx, trustCancel := context.WithTimeout(ctx, 30*time.Second)
		defer trustCancel()
		if _, err := m.mgr.Invoke(trustCtx, Window, "project_trust_set", map[string]any{
			"path": root, "trusted": true, "folder": false,
		}); err != nil {
			return nil, fmt.Errorf("trust screenshot project: %w", err)
		}
	}
	// Every state is captured in both appearances, as the Dark and Light
	// golden sets require.
	shot := func(name string) {
		for _, theme := range []struct {
			name string
			dark bool
		}{{"dark", true}, {"light", false}} {
			theme := theme
			key := theme.name + "/" + name
			var size int
			var errText string
			m.post(func() {
				previous := m.theme
				m.theme = openideaTheme(theme.dark)
				tt := ui.NewTester(func(c *ui.Context) { m.View(c) }, smokeShotWidth, smokeShotHeight)
				tt.SetDark(theme.dark)
				tt.SetScale(1)
				// MyGo resolves Grow() and Bounds() one frame after the tree
				// changes, so a state switch is laid out by the first frame
				// and complete on the second: the image is what the screen
				// shows one frame later, not a half-built tree.
				tt.Frame()
				tt.Frame()
				img := tt.Image()
				m.theme = previous
				themeDir := filepath.Join(dir, theme.name)
				if err := os.MkdirAll(themeDir, 0o755); err != nil {
					errText = err.Error()
					return
				}
				path := filepath.Join(themeDir, name+".png")
				f, err := os.Create(path)
				if err != nil {
					errText = err.Error()
					return
				}
				err = png.Encode(f, img)
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
				errs = append(errs, key+": "+errText)
				continue
			}
			sizes[key] = size
		}
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

	// The project tree with the source chain open: OpenIDEA compacts
	// com/example into one package row.
	m.post(func() {
		m.tree.reveal(filepath.Join(m.Root(), "src/main/java/com/example/App.java"))
	})
	wait(func() bool {
		for _, row := range m.tree.Rows() {
			if row.packageDir {
				return true
			}
		}
		return false
	}, 30*time.Second)
	shot("left-packages")
	m.post(func() {
		if m.tree.open[filepath.Join(m.Root(), "src")] {
			m.tree.Toggle(filepath.Join(m.Root(), "src"))
		}
	})

	// The editor alone, as Distraction Free Mode shows it: the action, not the
	// flag, so the gutter and the hint come out as they do for the user.
	m.post(func() { m.ToggleZen() })
	wait(func() bool { return m.zen && m.tab() != nil }, 30*time.Second)
	shot("editor")
	m.post(func() { m.ToggleZen() })

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

	// Every other settings page this build carries, one state each.
	for _, page := range []struct{ id, name string }{
		{settingsPageKeymap, "settings-keymap"},
		{settingsPageGeneral, "settings-editor"},
		{settingsPageEditorAppearance, "settings-editor-appearance"},
		{settingsPageBreadcrumbs, "settings-breadcrumbs"},
		{settingsPageEditorTabs, "settings-tabs"},
		{settingsPageCodeFolding, "settings-folding"},
		{settingsPageEditorFont, "settings-font"},
		{settingsPageTodo, "settings-todo"},
		{settingsPageNotifications, "settings-notifications"},
		{settingsPageJava, "settings-java"},
	} {
		page := page
		m.post(func() { m.OpenSettingsPage(page.id) })
		wait(func() bool { return m.settingsOpen }, 30*time.Second)
		shot(page.name)
		m.post(func() { m.closeSettings() })
		wait(func() bool { return !m.settingsOpen }, 30*time.Second)
	}
	m.post(func() { m.OpenSettingsPage(settingsPageProjectTrust) })
	wait(func() bool {
		return m.settingsOpen && m.projectTrust.loaded && !m.projectTrust.loading
	}, 30*time.Second)
	shot("settings-project-trust")
	m.post(func() { m.closeSettings() })
	wait(func() bool { return !m.settingsOpen }, 30*time.Second)

	// Command palette and the quick-file popup.
	m.post(func() { m.OpenCommands() })
	wait(func() bool { return m.palette != nil }, 30*time.Second)
	shot("palette")
	m.post(func() { m.closeCommands() })
	m.post(func() { m.OpenQuickFiles() })
	wait(func() bool { return m.files != nil && m.files.done }, 60*time.Second)
	shot("popup-quick-files")
	m.post(func() { m.closeQuickFiles() })

	// Run Anything over the same configurations.
	m.post(func() { m.OpenRunAnything() })
	wait(func() bool { return m.runAnything != nil }, 30*time.Second)
	shot("run-anything")
	m.post(func() { m.closeRunAnything() })

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
	wait(func() bool {
		return (!m.run.loading && m.run.loaded) && !m.services.docker.loading
	}, 90*time.Second)
	m.post(func() {
		for _, cfg := range m.run.configs {
			m.addServiceType(cfg.Kind)
		}
		if len(m.run.configs) > 0 {
			m.services.selected = serviceConfigKey(m.run.configs[0].ID)
		}
	})
	shot("services")
	m.post(func() { m.CloseServices() })

	// The Maven tool window: toolbar, banner and the reactor tree.
	m.post(func() { m.ShowMaven() })
	wait(func() bool {
		return m.maven.visible && !m.maven.loading && len(m.maven.model.MavenProjects()) > 0
	}, 60*time.Second)
	shot("maven")
	m.post(func() { m.CloseMaven() })

	// The Run window's native session strip, action rail and colored console.
	// This uses a real detected configuration and synthetic process output so
	// the visual state never needs to stop a backend user's existing process.
	m.post(func() {
		cfg := m.run.current()
		if cfg == nil {
			return
		}
		m.run.visible = true
		m.occupyBottom("run")
		m.run.trusted = true
		m.run.lastID = cfg.ID
		m.run.lastRaw = append(json.RawMessage(nil), cfg.Raw()...)
		m.run.lastName = cfg.Name
		m.run.channel = m.nextChannel()
		m.run.runID = 1
		m.run.running = true
		m.run.stdinClosed = true
		m.run.started = time.Now().Add(-1200 * time.Millisecond)
		m.run.output = []outputLine{
			{kind: "stdout", text: "Starting " + cfg.Name + "…"},
			{kind: "stdout", text: "Connected to https://example.test/application"},
			{kind: "stderr", text: "warning: development profile is active"},
			{kind: "stdout", text: "Application started in 1.2 seconds"},
		}
		m.invalidate()
	})
	wait(func() bool { return m.run.visible && m.run.hasPrimarySession() }, 30*time.Second)
	shot("run-window")
	m.post(func() {
		m.dropPrimaryRunSession()
		m.hideRunWhenEmpty()
	})
	wait(func() bool { return !m.run.visible }, 30*time.Second)

	// The Debug window with no session, and the terminal with a live shell.
	m.post(func() { m.ShowDebug() })
	wait(func() bool { return m.debug.visible }, 30*time.Second)
	shot("debug")
	// DockHide, not the mirror flag: the dock is what draws the bottom area,
	// so every later state would keep showing the window.
	m.post(func() { m.DockHide("debug") })
	wait(func() bool { return !m.debug.visible }, 30*time.Second)
	m.post(func() { m.ShowTerm() })
	wait(func() bool {
		s := m.term.current()
		return s != nil && s.id != 0 && len(s.lines) > 0
	}, 60*time.Second)
	shot("terminal")
	m.post(func() { m.DockHide("term") })
	wait(func() bool { return !m.term.visible }, 30*time.Second)

	// The left tool window: Structure in the split area below Project.
	m.post(func() { m.ShowStructure() })
	wait(func() bool { return m.leftSplit == "structure" && m.structure.visible }, 30*time.Second)
	// The symbols come from the backend: a shot taken while they load shows
	// the window's "reading" placeholder instead of its rows.
	wait(func() bool {
		t := m.tab()
		return t != nil && !t.outlineLoading
	}, 60*time.Second)
	shot("left-structure")
	m.post(func() { m.CloseStructure() })

	// File › New Project… as a real dialog, with the backend's generator
	// catalog loaded so the sheet shows the template list OpenIDEA shows.
	m.post(func() { m.NewProject() })
	wait(func() bool { return m.newProject.open && len(m.newProject.catalog) > 0 }, 60*time.Second)
	shot("new-project")
	m.post(func() { m.CloseNewProject() })
	wait(func() bool { return !m.newProject.open }, 30*time.Second)

	// A dialog: File › Invalidate Caches….
	m.post(func() { m.OpenInvalidateCaches() })
	wait(func() bool { return m.invalidateDlg != nil }, 30*time.Second)
	shot("dialog")
	m.post(func() { m.closeInvalidateCaches() })

	// Run › Edit Configurations… over the configurations the backend detects.
	wait(func() bool { return len(m.run.configs) > 0 }, 60*time.Second)
	m.post(func() { m.OpenRunConfigs() })
	wait(func() bool { return m.runCfgDlg != nil }, 30*time.Second)
	shot("run-configs")
	m.post(func() { m.closeRunConfigs() })

	// Run › Attach to Process… with the backend's process list.
	m.post(func() { m.OpenAttach() })
	wait(func() bool { return m.attach != nil && !m.attach.busy }, 60*time.Second)
	shot("attach")
	m.post(func() { m.closeAttach() })

	// Git › Commit… over the project's uncommitted change.
	m.post(func() { m.OpenCommit() })
	wait(func() bool {
		return m.commit != nil && m.vcs.loaded && !m.vcs.loading && len(m.vcs.files) > 0
	}, 60*time.Second)
	shot("commit")
	m.post(func() { m.commit = nil })

	// The diff of that change, from the open file's buffer.
	m.post(func() {
		if t := m.tab(); t != nil {
			m.ShowDiff(t.doc.Path())
		}
	})
	wait(func() bool { return m.diff != nil && !m.diff.loading }, 60*time.Second)
	shot("diff")
	m.post(func() { m.closeDiff() })

	// Local History from the editor menu, over the file's own snapshots.
	m.post(func() {
		if t := m.tab(); t != nil {
			m.ShowHistory(t.doc.Path())
		}
	})
	wait(func() bool {
		return m.history != nil && !m.history.loading && !m.history.busy
	}, 60*time.Second)
	shot("local-history")
	m.post(func() { m.closeHistory() })

	// View › Tool Windows › Database with the project's data sources.
	m.post(func() { m.OpenDatabase() })
	wait(func() bool { return m.db != nil && m.db.open && !m.db.loading }, 60*time.Second)
	shot("database")
	m.post(func() { m.closeDatabase() })

	// The database window's own sheets over a temporary SQLite source: the
	// table page, the statement history and an execution plan. The source is
	// written into the project's settings, so it is removed again below.
	dbFile, dbErr := os.CreateTemp("", "goidea-shots-*.sqlite")
	if dbErr != nil {
		errs = append(errs, "database source: "+dbErr.Error())
	} else {
		dbPath := dbFile.Name()
		dbFile.Close()
		defer os.Remove(dbPath)
		dbID := sqliteSourceID(dbPath)
		setupDB := func() error {
			if m.mgr == nil {
				return errors.New("no backend")
			}
			dbCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
			defer cancel()
			// The shot run may reuse a project, so drop the sources an earlier
			// run left before adding this one.
			if err := m.mgr.DBSaveSourcesRaw(dbCtx, Window, []map[string]any{}); err != nil {
				return err
			}
			if err := m.mgr.DBSaveSQLiteSource(dbCtx, Window, dbID, filepath.Base(dbPath), dbPath); err != nil {
				return err
			}
			if err := m.mgr.DBConnect(dbCtx, Window, dbID); err != nil {
				return err
			}
			sql := "CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT); " +
				"INSERT INTO items (name) VALUES ('one'), ('two');"
			stmts, err := m.mgr.DBStatements(dbCtx, Window, dbID, sql, 0, utf16Len(sql))
			if err != nil {
				return err
			}
			_, err = m.mgr.DBExecute(dbCtx, Window, 1, dbID, stmts, 50, "")
			return err
		}
		if err := setupDB(); err != nil {
			errs = append(errs, "database source: "+err.Error())
		} else {
			m.post(func() { m.OpenDatabase() })
			wait(func() bool {
				return m.db != nil && m.db.open && !m.db.loading && len(m.db.tables) > 0
			}, 60*time.Second)
			m.post(func() {
				schema := m.db.schema
				if schema == "" {
					schema = "main"
				}
				m.OpenDBTable(schema, "items")
			})
			wait(func() bool {
				return m.dbTable != nil && !m.dbTable.loading && len(m.dbTable.page.Result.Rows) > 0
			}, 60*time.Second)
			shot("db-table")
			m.post(func() { m.closeDBTable() })

			m.post(func() {
				m.db.sql = "SELECT * FROM items"
				m.RunDB()
			})
			wait(func() bool { return !m.db.running && len(m.db.outcomes) > 0 }, 60*time.Second)
			m.post(func() { m.OpenDBHistory() })
			wait(func() bool {
				return m.dbHist != nil && !m.dbHist.loading && len(m.dbHist.items) > 0
			}, 60*time.Second)
			shot("db-history")
			m.post(func() { m.closeDBHistory() })

			m.post(func() {
				m.db.sql = "SELECT * FROM items WHERE id = 1"
				m.ExplainDB()
			})
			wait(func() bool {
				return m.dbPlan != nil && !m.dbPlan.loading && len(m.dbPlan.rows) > 0
			}, 60*time.Second)
			shot("db-plan")
			m.post(func() { m.closeDBPlan() })
			m.post(func() { m.closeDatabase() })

			dbCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			_ = m.mgr.DBDisconnect(dbCtx, Window, dbID)
			_ = m.mgr.DBSaveSourcesRaw(dbCtx, Window, []map[string]any{})
			cancel()
		}
	}

	// The Trust Project prompt a project asks before it may build.
	m.post(func() { m.trustAsk = true })
	shot("trust")
	m.post(func() { m.trustAsk = false })

	// Notifications: the bottom-right balloons and the tool window's history.
	m.post(func() { m.notifyGroupSample() })
	shot("balloons")
	m.post(func() { m.ShowNotifications() })
	shot("notifications")
	m.post(func() {
		m.CloseNotifications()
		m.ClearNotifications()
	})

	// File › Open Recent: the current project is hidden, as upstream hides it.
	var shotRecents []RecentProject
	m.post(func() {
		shotRecents = m.recents
		if len(m.filteredRecentsFor("", m.Root())) == 0 {
			parent := filepath.Dir(m.Root())
			m.recents = []RecentProject{{
				Path: parent, Name: filepath.Base(parent), OpenedAt: time.Now().UnixMilli(),
			}}
		}
		m.showRecentProjectsPopup()
	})
	wait(func() bool { return m.recentProjectsPopup != nil }, 30*time.Second)
	shot("recent-projects")
	m.post(func() {
		m.closeRecentProjectsPopup()
		m.recents = shotRecents
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
