package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

// VCSPane is the sidebar's Git view: the changes `git status` reports for every
// repository of the project, loaded through openidea-server.
type VCSPane struct {
	loading bool
	loaded  bool
	files   []openidea.FileStatus
	err     string
	busy    bool
	opErr   string
	// stale asks for another read once the one in flight lands, so a change
	// made while the list is loading is not lost.
	stale bool
}

// rollbackState is the confirmation a rollback of one change asks for, since
// the working-tree edit is lost.
type rollbackState struct {
	path string
	name string
	open bool
	busy bool
	err  string
}

func (m *Model) resetVCS() { m.vcs = VCSPane{} }

// RefreshVCS reloads the project's changes in the background.
func (m *Model) RefreshVCS() {
	root := m.Root()
	if root == "" {
		return
	}
	if m.vcs.loading {
		m.vcs.stale = true
		return
	}
	m.vcs.loading = true
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.mgr.Invoke(ctx, Window, "vcs_status", nil)
		var files []openidea.FileStatus
		if err == nil {
			err = json.Unmarshal(raw, &files)
		}
		m.post(func() {
			m.vcs.loading = false
			if m.Root() != root {
				return
			}
			m.vcs.loaded = true
			if err != nil {
				m.vcs.err = vcsErrorText(err)
				m.vcs.files = nil
			} else {
				m.vcs.err = ""
				sort.SliceStable(files, func(i, j int) bool { return files[i].RelPath < files[j].RelPath })
				m.vcs.files = files
			}
			m.invalidate()
			if m.vcs.stale {
				m.vcs.stale = false
				m.RefreshVCS()
			}
		})
	}()
}

// ShowVCS switches the sidebar to the changes view (and reloads it).
func (m *Model) ShowVCS() {
	m.sideMode = "vcs"
	m.leftHidden = false
	m.RefreshVCS()
	m.RefreshBranch()
	m.rememberLayout()
	m.invalidate()
}

// SmokeVCS reports the project's Git changes through the backend: the
// offscreen check of the changes view.
func (m *Model) SmokeVCS(ctx context.Context) (int, string, error) {
	raw, err := m.mgr.Invoke(ctx, Window, "vcs_status", nil)
	if err != nil {
		return 0, "", err
	}
	var files []openidea.FileStatus
	if err := json.Unmarshal(raw, &files); err != nil {
		return 0, "", err
	}
	sample := ""
	if len(files) > 0 {
		letter, word, _ := vcsBadge(files[0])
		sample = letter + " " + word + " " + files[0].RelPath
	}
	return len(files), sample, nil
}

// vcsErrorText turns the backend's refusal for a project without Git into the
// sentence the sidebar shows; other failures keep their own message.
func vcsErrorText(err error) string {
	var oe *openidea.Error
	if errors.As(err, &oe) && oe.Kind == "unavailable" && strings.Contains(oe.Message, "not a git repository") {
		return "The current project is not a Git repository"
	}
	return "Could not read the Git status: " + err.Error()
}

// vcsBadge is the label of a status row: a letter as IDEA shows it, the
// change in words, and a tone for the colour. The working tree kind wins when
// both sides changed, since that is the state the file is in now.
func vcsBadge(f openidea.FileStatus) (letter, word, tone string) {
	kind := ""
	if f.Unstaged != nil {
		kind = *f.Unstaged
	} else if f.Staged != nil {
		kind = *f.Staged
	}
	switch kind {
	case "added":
		return "A", "Added", "add"
	case "modified":
		return "M", "Modified", "mod"
	case "deleted":
		return "D", "Deleted", "del"
	case "renamed":
		return "R", "Renamed", "add"
	case "conflicted":
		return "!", "Conflict", "del"
	case "untracked":
		return "U", "Untracked", "mod"
	case "ignored":
		return "I", "Ignored", "muted"
	}
	return "?", "Changed", "mod"
}

// vcsKindColor is the colour of a change's name, as OpenIDEA's KIND_COLOR
// paints it.
func vcsKindColor(p *ui.Theme, f openidea.FileStatus) ui.Color {
	oi := oiPaletteOf(p)
	switch effectiveVCSKind(f) {
	case "added":
		return oi.vcsAdded
	case "deleted":
		return oi.vcsDeleted
	case "untracked":
		return oi.vcsUntracked
	case "ignored":
		return oi.vcsIgnored
	case "conflicted":
		return oi.vcsConflict
	}
	return oi.vcsModified
}

// vcsStaged and vcsUnstaged split the changes the way OpenIDEA's changes view
// groups them; a file staged and then edited again is in both.
func vcsStaged(files []openidea.FileStatus) []openidea.FileStatus {
	out := make([]openidea.FileStatus, 0, len(files))
	for _, f := range files {
		if f.Staged != nil {
			out = append(out, f)
		}
	}
	return out
}

func vcsUnstaged(files []openidea.FileStatus) []openidea.FileStatus {
	out := make([]openidea.FileStatus, 0, len(files))
	for _, f := range files {
		if f.Unstaged != nil {
			out = append(out, f)
		}
	}
	return out
}

// StageChange stages one file from the changes view.
func (m *Model) StageChange(f openidea.FileStatus) { m.changeOp("vcs_stage", f, "Staged ") }

// AddToIgnore adds path to the shared .gitignore or, with destination
// "localExclude", to .git/info/exclude, as the project view's Git › Add to
// .gitignore does.
func (m *Model) AddToIgnore(path, destination string) {
	if m.Root() == "" || path == "" || m.vcs.busy {
		return
	}
	name := filepath.Base(path)
	where := ".gitignore"
	if destination == "localExclude" {
		where = ".git/info/exclude"
	}
	m.vcs.busy = true
	m.vcs.opErr = ""
	m.status = "Adding " + name + " to " + where + "…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, time.Minute)
		defer cancel()
		_, err := m.mgr.Invoke(ctx, Window, "vcs_add_to_ignore", map[string]any{
			"paths": []string{path}, "destination": destination,
		})
		m.post(func() {
			m.vcs.busy = false
			if m.Root() == "" {
				return
			}
			if err != nil {
				m.vcs.opErr = "Could not add it to the ignore list: " + err.Error()
				m.invalidate()
				return
			}
			m.status = "Added " + name + " to " + where
			m.RefreshVCS()
			m.invalidate()
		})
	}()
}

// statusFor is the change `git status` reported for path, if the loaded list
// has one; the tab menu's Git actions read it to enable themselves.
func (m *Model) statusFor(path string) (openidea.FileStatus, bool) {
	for _, f := range m.vcs.files {
		if f.Path == path {
			return f, true
		}
	}
	return openidea.FileStatus{}, false
}

// UnstageChange takes one file back out of the index.
func (m *Model) UnstageChange(f openidea.FileStatus) {
	m.changeOp("vcs_unstage", f, "Unstaged ")
}

func (m *Model) changeOp(command string, f openidea.FileStatus, done string) {
	if m.Root() == "" || m.vcs.busy {
		return
	}
	m.vcs.busy = true
	m.vcs.opErr = ""
	m.status = "Updating " + filepath.Base(f.RelPath) + "…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, time.Minute)
		defer cancel()
		_, err := m.mgr.Invoke(ctx, Window, command, map[string]any{"paths": []string{f.Path}})
		m.post(func() {
			m.vcs.busy = false
			if m.Root() == "" {
				return
			}
			if err != nil {
				m.vcs.opErr = "The Git operation failed: " + err.Error()
				m.invalidate()
				return
			}
			m.status = done + filepath.Base(f.RelPath)
			m.RefreshVCS()
			m.invalidate()
		})
	}()
}

// AskRollback opens the confirmation that discards one working-tree change.
func (m *Model) AskRollback(f openidea.FileStatus) {
	m.rollback = &rollbackState{path: f.Path, name: f.RelPath, open: true}
	m.invalidate()
}

func (m *Model) closeRollback() {
	m.rollback = nil
	m.invalidate()
}

// ConfirmRollback discards the change the confirmation names.
func (m *Model) ConfirmRollback() {
	r := m.rollback
	if r == nil || r.busy || m.Root() == "" {
		return
	}
	r.busy = true
	r.err = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, time.Minute)
		defer cancel()
		_, err := m.mgr.Invoke(ctx, Window, "vcs_discard", map[string]any{"paths": []string{r.path}})
		m.post(func() {
			r.busy = false
			if m.Root() == "" {
				return
			}
			if err != nil {
				r.err = "Rollback failed: " + err.Error()
				m.invalidate()
				return
			}
			if m.rollback == r {
				m.rollback = nil
			}
			m.status = "Rolled back " + r.name
			m.RefreshVCS()
			m.invalidate()
		})
	}()
}

// rollbackDialog asks before the discard.
func (m *Model) rollbackDialog(c *ui.Context, p *ui.Theme) {
	r := m.rollback
	if r == nil {
		return
	}
	open := r.open
	d := ui.Modal(c, &open, func() {
		ui.Column(c).Width(420).Gap(10).Children(func() {
			ui.Text(c, "Roll Back Changes").FontSize(13).Bold().TextColor(p.Text).SingleLine()
			ui.Text(c, "Roll back "+r.name+"? Unstaged changes are lost.").FontSize(12).
				TextColor(p.TextMuted)
			if r.err != "" {
				ui.Text(c, r.err).FontSize(12).TextColor(p.Danger).SingleLine().Ellipsis("…")
			}
			ui.Row(c).Gap(8).Children(func() {
				label := "Roll Back"
				if r.busy {
					label = "Rolling back…"
				}
				ui.Button(c, label).Disabled(r.busy).OnClick(m.ConfirmRollback)
				ui.Button(c, "Cancel").OnClick(m.closeRollback)
			})
		})
	})
	if d.Dismissed() || !open {
		m.closeRollback()
	}
}

// SmokeStage proves the changes view's actions on a scratch file that is
// staged, unstaged and then discarded.
func (m *Model) SmokeStage(ctx context.Context) (map[string]any, error) {
	root := m.Root()
	if root == "" {
		return nil, errors.New("No project is open")
	}
	tmp := filepath.Join(root, "goidea-stage-smoke.txt")
	if err := os.WriteFile(tmp, []byte("stage smoke\n"), 0o644); err != nil {
		return nil, err
	}
	out := map[string]any{}
	paths := map[string]any{"paths": []string{tmp}}
	if _, err := m.mgr.Invoke(ctx, Window, "vcs_stage", paths); err != nil {
		return out, err
	}
	out["staged"] = m.smokeStaged(ctx, tmp)
	if _, err := m.mgr.Invoke(ctx, Window, "vcs_unstage", paths); err != nil {
		return out, err
	}
	out["unstaged"] = !m.smokeStaged(ctx, tmp)
	if _, err := m.mgr.Invoke(ctx, Window, "vcs_discard", paths); err != nil {
		return out, err
	}
	_, statErr := os.Stat(tmp)
	out["discarded"] = os.IsNotExist(statErr)
	return out, nil
}

func (m *Model) smokeStaged(ctx context.Context, path string) bool {
	raw, err := m.mgr.Invoke(ctx, Window, "vcs_status", nil)
	if err != nil {
		return false
	}
	var files []openidea.FileStatus
	if json.Unmarshal(raw, &files) != nil {
		return false
	}
	for _, f := range files {
		if f.Path == path {
			return f.Staged != nil
		}
	}
	return false
}
