package app

// File › New Project… after OpenIDEA's NewProject dialog: a name, a location
// and "Create Git repository"; Create makes the folder (reusing an empty one),
// runs git init when asked and opens the project. Creation is a backend call,
// the same fs_create_dir/vcs_init every other file action uses.

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

// newProjectState is the dialog while it is up.
type newProjectState struct {
	open     bool
	name     string
	location string
	gitInit  bool
	busy     bool
	err      string
	// seq drops a reply whose dialog was closed.
	seq int
}

// projectNameError is why name cannot be a project folder, or "".
func projectNameError(name string) string {
	n := strings.TrimSpace(name)
	if n == "" {
		return "Enter a project name"
	}
	if strings.ContainsAny(n, `/\:*?"<>|`) || n == "." || n == ".." {
		return "The name contains characters a folder name cannot have"
	}
	return ""
}

// joinProjectPath puts name under dir, matching the separator dir uses.
func joinProjectPath(dir, name string) string {
	return filepath.Join(dir, strings.TrimSpace(name))
}

// NewProject opens the dialog, as File › New Project… does.
func (m *Model) NewProject() {
	location := ""
	if root := m.Root(); root != "" {
		location = filepath.Dir(root)
	} else if home, err := os.UserHomeDir(); err == nil {
		location = home
	}
	m.newProject = newProjectState{open: true, name: "untitled", location: location, gitInit: true}
	m.invalidate()
}

// NewProjectPath is where Create will make the folder, "" while the form is
// invalid.
func (m *Model) NewProjectPath() string {
	st := &m.newProject
	if projectNameError(st.name) != "" || strings.TrimSpace(st.location) == "" {
		return ""
	}
	return joinProjectPath(st.location, st.name)
}

// BrowseNewProjectLocation asks for the parent folder.
func (m *Model) BrowseNewProjectLocation() {
	parent := m.win
	location := m.newProject.location
	seq := m.newProject.seq
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: parent, Title: "Project Location", ButtonLabel: "Choose",
			Directory: true, DefaultPath: location,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		m.post(func() {
			if m.newProject.seq != seq || !m.newProject.open {
				return
			}
			m.newProject.location = paths[0]
			m.invalidate()
		})
	}()
}

// CloseNewProject dismisses the dialog; a creation in flight is dropped
// rather than opening over the dialog the user closed.
func (m *Model) CloseNewProject() {
	m.newProject.open = false
	m.newProject.busy = false
	m.newProject.seq++
	m.invalidate()
}

// CreateNewProject makes the folder and opens it. An empty existing folder is
// reused; a non-empty one is refused, as OpenIDEA's createProjectFolder does.
func (m *Model) CreateNewProject() {
	st := &m.newProject
	if st.busy {
		return
	}
	path := m.NewProjectPath()
	if path == "" {
		return
	}
	st.busy = true
	st.err = ""
	st.seq++
	seq := st.seq
	gitInit := st.gitInit
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 2*60*1e9)
		defer cancel()
		err := m.createProjectFolder(ctx, path, gitInit)
		m.post(func() {
			if m.newProject.seq != seq {
				return
			}
			m.newProject.busy = false
			if err != nil {
				m.newProject.err = err.Error()
				m.invalidate()
				return
			}
			m.newProject.open = false
			m.rememberProject(path)
			m.confirmSwitch(path)
			m.invalidate()
		})
	}()
}

// createProjectFolder creates path, or reuses it when the folder is empty, and
// initializes Git when asked. With a backend the work is the same
// fs_create_dir/vcs_init the file actions use; tests without one fall back to
// the local file system.
func (m *Model) createProjectFolder(ctx context.Context, path string, gitInit bool) error {
	entries, exists := m.readProjectDir(ctx, path)
	if exists && entries > 0 {
		return errors.New(path + " already exists and is not empty")
	}
	if !exists {
		if err := m.createProjectDir(ctx, path); err != nil {
			return err
		}
	}
	if gitInit {
		if err := m.initProjectGit(ctx, path); err != nil {
			return err
		}
	}
	return nil
}

// readProjectDir reports whether the folder exists and how many entries it
// holds.
func (m *Model) readProjectDir(ctx context.Context, path string) (int, bool) {
	if m.mgr != nil {
		raw, err := m.mgr.Invoke(ctx, Window, "fs_read_dir", map[string]any{"path": path})
		if err == nil {
			var entries []json.RawMessage
			if json.Unmarshal(raw, &entries) == nil {
				return len(entries), true
			}
		}
	}
	f, err := os.Open(path)
	if err != nil {
		return 0, false
	}
	defer f.Close()
	list, err := f.Readdirnames(1)
	if err != nil {
		return 0, true
	}
	return len(list), true
}

// createProjectDir makes the folder through the backend when there is one.
func (m *Model) createProjectDir(ctx context.Context, path string) error {
	if m.mgr != nil {
		if _, err := m.mgr.Invoke(ctx, Window, "fs_create_dir", map[string]any{"path": path}); err == nil {
			return nil
		} else if openidea.ErrorKind(err) != "" {
			return err
		}
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return errors.New("could not create " + path + ": " + err.Error())
	}
	return nil
}

// initProjectGit runs git init through the backend, or the git binary when
// there is no backend.
func (m *Model) initProjectGit(ctx context.Context, path string) error {
	if m.mgr != nil {
		if _, err := m.mgr.Invoke(ctx, Window, "vcs_init", map[string]any{"dir": path}); err == nil {
			return nil
		} else if openidea.ErrorKind(err) != "" {
			return err
		}
	}
	if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
		return nil
	}
	cmd := gitCommand(path, "init")
	if out, err := cmd.CombinedOutput(); err != nil {
		return errors.New("git init failed: " + strings.TrimSpace(string(out)))
	}
	return nil
}

// newProjectDialog draws the sheet OpenIDEA's NewProjectOverlay shows.
func (m *Model) newProjectDialog(c *ui.Context, p *ui.Theme) {
	st := &m.newProject
	if !st.open {
		return
	}
	open := st.open
	nameErr := projectNameError(st.name)
	path := m.NewProjectPath()
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(520).Gap(12).Children(func() {
			ui.Text(c, "New Project").FontSize(15).Bold().TextColor(p.Text).SingleLine()
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Name:").FontSize(12).TextColor(p.Text).Width(64).SingleLine()
				in := ui.TextInput(c, &st.name).AutoFocus()
				in.OnChange(func() { m.invalidate() })
				if in.Submitted() {
					m.CreateNewProject()
				}
			})
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Location:").FontSize(12).TextColor(p.Text).Width(64).SingleLine()
				in := ui.TextInput(c, &st.location)
				in.OnChange(func() { m.invalidate() })
				b := ui.Button(c, "Browse…")
				if b.Clicked() {
					m.BrowseNewProjectLocation()
				}
			})
			hint := projHint(nameErr, path)
			tone := p.TextMuted
			if nameErr != "" {
				tone = p.Danger
			}
			ui.Text(c, hint).FontSize(10.5).TextColor(tone).SingleLine().Ellipsis("…")
			git := st.gitInit
			if ui.Checkbox(c, &git, "Create Git repository").Changed() {
				st.gitInit = git
			}
			if st.err != "" {
				ui.Text(c, st.err).FontSize(11).TextColor(p.Danger).SingleLine().Ellipsis("…")
			}
			ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
				if ui.Button(c, "Cancel").Clicked() {
					m.CloseNewProject()
				}
				b := ui.PrimaryButton(c, "Create")
				if nameErr != "" || path == "" || st.busy {
					b.Tooltip(nameErr)
				}
				if b.Clicked() && nameErr == "" && path != "" && !st.busy {
					m.CreateNewProject()
				}
				if st.busy {
					ui.Text(c, "正在创建…").FontSize(10.5).TextColor(p.TextMuted).SingleLine()
				}
			})
		})
	})
	if !open {
		m.CloseNewProject()
	}
}

// gitCommand runs git in dir, for the case without a backend.
func gitCommand(dir string, args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return cmd
}

func projHint(nameErr, path string) string {
	if nameErr != "" {
		return nameErr
	}
	if path == "" {
		return ""
	}
	return "Project will be created in: " + path
}

// SmokeNewProject drives File › New Project… against the real backend: a new
// folder with Git, an empty folder reused, a non-empty one refused. The
// folder it makes lives in the run's temp directory and is removed.
func (m *Model) SmokeNewProject(ctx context.Context) map[string]any {
	out := map[string]any{}
	root, err := os.MkdirTemp("", "goidea-newproject-")
	if err != nil {
		out["error"] = err.Error()
		return out
	}
	defer os.RemoveAll(root)

	created := filepath.Join(root, "shop")
	if err := m.createProjectFolder(ctx, created, true); err != nil {
		out["createError"] = err.Error()
	} else {
		out["created"] = true
		if st, err := os.Stat(created); err == nil && st.IsDir() {
			out["inPlace"] = true
		}
		if _, err := os.Stat(filepath.Join(created, ".git")); err == nil {
			out["git"] = true
		}
	}

	empty := filepath.Join(root, "empty")
	if err := os.Mkdir(empty, 0o755); err != nil {
		out["error"] = err.Error()
		return out
	}
	if err := m.createProjectFolder(ctx, empty, false); err == nil {
		out["reused"] = true
	}

	full := filepath.Join(root, "full")
	if err := os.Mkdir(full, 0o755); err != nil {
		out["error"] = err.Error()
		return out
	}
	if err := os.WriteFile(filepath.Join(full, "keep.txt"), []byte("x"), 0o644); err != nil {
		out["error"] = err.Error()
		return out
	}
	if err := m.createProjectFolder(ctx, full, false); err != nil {
		out["refused"] = true
	}
	return out
}
