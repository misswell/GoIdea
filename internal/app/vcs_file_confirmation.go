package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

type vcsFileConfirmation struct {
	kind     string
	root     string
	paths    []string
	names    []string
	remember bool
	busy     bool
	err      string
}

func (m *Model) notifyVCSFileOperation(kind string, paths []string) {
	root := m.Root()
	if root == "" || len(paths) == 0 || (m.mgr == nil && m.invokeHook == nil) {
		return
	}
	key := "vcs.confirmation.onCreate"
	if kind == "deleted" {
		key = "vcs.confirmation.onDelete"
	}
	mode := m.nativeRuntimeString(settingsPageVCSConfirmation, key)
	if mode == "" {
		mode = "ask"
	}
	if mode == "never" {
		return
	}
	clean := make([]string, 0, len(paths))
	seen := map[string]bool{}
	for _, path := range paths {
		path = filepath.Clean(path)
		if path == "" || seen[path] || !underTreePath(path, root) {
			continue
		}
		seen[path] = true
		clean = append(clean, path)
	}
	if len(clean) == 0 {
		return
	}
	unity := m.projectUnityVersion != ""
	go m.resolveVCSFileOperation(root, kind, clean, mode, unity)
}

func (m *Model) resolveVCSFileOperation(
	root string,
	kind string,
	paths []string,
	mode string,
	unity bool,
) {
	ctx, cancel := context.WithTimeout(modelContext(m), 25*time.Second)
	defer cancel()
	files, err := m.vcsStatusForFileOperation(ctx)
	if err != nil {
		return
	}
	candidates := vcsMatchingFileOperationPaths(files, paths, kind, unity)
	if len(candidates) == 0 && unity && kind == "created" {
		timer := time.NewTimer(450 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		files, err = m.vcsStatusForFileOperation(ctx)
		if err == nil {
			candidates = vcsMatchingFileOperationPaths(files, paths, kind, unity)
		}
	}
	if len(candidates) == 0 {
		return
	}
	m.post(func() {
		if m.Root() != root {
			return
		}
		if mode == "silently" {
			m.runVCSFileConfirmation(root, kind, candidates, false)
			return
		}
		names := make([]string, 0, len(candidates))
		for _, path := range candidates {
			names = append(names, filepath.ToSlash(relPathUnder(root, path)))
		}
		m.vcsFileConfirm = &vcsFileConfirmation{
			kind:  kind,
			root:  root,
			paths: append([]string(nil), paths...),
			names: names,
		}
		m.invalidate()
	})
}

func (m *Model) vcsStatusForFileOperation(ctx context.Context) ([]openidea.FileStatus, error) {
	raw, err := m.invoke(ctx, "vcs_status", nil)
	if err != nil {
		return nil, err
	}
	var files []openidea.FileStatus
	if err := json.Unmarshal(raw, &files); err != nil {
		return nil, err
	}
	return files, nil
}

func vcsMatchingFileOperationPaths(
	files []openidea.FileStatus,
	paths []string,
	kind string,
	unity bool,
) []string {
	statusKind := "untracked"
	if kind == "deleted" {
		statusKind = "deleted"
	}
	available := make(map[string]bool, len(files))
	order := make([]string, 0, len(files))
	for _, file := range files {
		if effectiveVCSKind(file) != statusKind || file.Staged != nil {
			continue
		}
		path := filepath.Clean(file.Path)
		if available[path] {
			continue
		}
		available[path] = true
		order = append(order, path)
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(paths))
	add := func(path string) {
		path = filepath.Clean(path)
		if path == "" || seen[path] {
			return
		}
		if !available[path] {
			return
		}
		seen[path] = true
		out = append(out, path)
	}
	for _, requested := range paths {
		requested = filepath.Clean(requested)
		for _, status := range order {
			if status == requested || underTreePath(status, requested) ||
				underTreePath(requested, status) {
				add(status)
			}
		}
		if kind == "created" && unity &&
			strings.Contains(filepath.ToSlash(requested), "/Assets/") {
			add(requested + ".meta")
		}
	}
	return out
}

func (m *Model) cancelVCSFileConfirmation() {
	state := m.vcsFileConfirm
	if state == nil || state.busy {
		return
	}
	if state.remember {
		key := "vcs.confirmation.onCreate"
		if state.kind == "deleted" {
			key = "vcs.confirmation.onDelete"
		}
		m.rememberVCSFileConfirmation(key, "never")
	}
	m.vcsFileConfirm = nil
	m.invalidate()
}

func (m *Model) confirmVCSFileConfirmation() {
	state := m.vcsFileConfirm
	if state == nil || state.busy {
		return
	}
	if state.remember {
		key := "vcs.confirmation.onCreate"
		if state.kind == "deleted" {
			key = "vcs.confirmation.onDelete"
		}
		m.rememberVCSFileConfirmation(key, "silently")
	}
	state.busy = true
	state.err = ""
	root, kind := state.root, state.kind
	paths := append([]string(nil), state.paths...)
	unity := m.projectUnityVersion != ""
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 25*time.Second)
		defer cancel()
		files, err := m.vcsStatusForFileOperation(ctx)
		if err != nil {
			m.post(func() {
				if state == m.vcsFileConfirm {
					state.busy = false
					state.err = "Could not refresh Git status: " + err.Error()
					m.invalidate()
				}
			})
			return
		}
		candidates := vcsMatchingFileOperationPaths(files, paths, kind, unity)
		m.post(func() {
			if state != m.vcsFileConfirm || m.Root() != root {
				return
			}
			if len(candidates) == 0 {
				m.vcsFileConfirm = nil
				m.invalidate()
				return
			}
			m.runVCSFileConfirmation(root, kind, candidates, true)
		})
	}()
}

func (m *Model) runVCSFileConfirmation(
	root string,
	kind string,
	candidates []string,
	fromDialog bool,
) {
	if fromDialog && m.vcsFileConfirm != nil && m.vcsFileConfirm.root == root {
		m.vcsFileConfirm.busy = true
		m.vcsFileConfirm.err = ""
	}
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 45*time.Second)
		defer cancel()
		_, err := m.invoke(ctx, "vcs_stage", map[string]any{
			"paths": candidates,
		})
		m.post(func() {
			if m.Root() != root {
				return
			}
			if fromDialog && m.vcsFileConfirm != nil {
				m.vcsFileConfirm = nil
			}
			if err != nil {
				m.notifyError(
					"vcs.file-confirmation",
					"Could not update Git",
					err.Error(),
				)
				m.invalidate()
				return
			}
			if kind == "created" {
				m.status = fmt.Sprintf(
					"Added %s to Git",
					count(len(candidates), "file", "files"),
				)
			} else {
				m.status = fmt.Sprintf(
					"Removed %s from Git",
					count(len(candidates), "file", "files"),
				)
			}
			m.RefreshAfterVCSCommand()
		})
	}()
}

func (m *Model) rememberVCSFileConfirmation(key, value string) {
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 8*time.Second)
		defer cancel()
		if err := m.invokeSettingsSet(ctx, "global", key, value); err != nil {
			m.post(func() {
				m.status = "Could not save " + key + ": " + err.Error()
				m.invalidate()
			})
		}
	}()
}

func (m *Model) vcsFileConfirmationDialog(c *ui.Context, p *ui.Theme) {
	state := m.vcsFileConfirm
	if state == nil {
		return
	}
	open := true
	add := state.kind == "created"
	title := "Remove File from Git"
	action := "Remove"
	question := "Do you want to remove the following file from Git?"
	if add {
		title = "Add File to Git"
		action = "Add"
		question = "Do you want to add the following file to Git?"
	}
	if len(state.names) != 1 {
		question = strings.Replace(
			question,
			"file",
			fmt.Sprintf("%d files", len(state.names)),
			1,
		)
	}
	dialog := ui.Modal(c.Key("vcs-file-confirmation"), &open, func() {
		ui.Column(c).Width(480).Gap(10).Children(func() {
			ui.Text(c, title).FontSize(15).Bold().TextColor(p.Text)
			ui.Text(c, question).FontSize(12).TextColor(p.Text)
			lines := state.names
			if len(lines) > 10 {
				lines = append(append([]string(nil), lines[:10]...), "…")
			}
			ui.Text(c, strings.Join(lines, "\n")).Font(oiFontMono).FontSize(12).TextColor(p.TextMuted)
			ui.Checkbox(c, &state.remember, "Don't ask again")
			if state.busy {
				ui.Text(c, "Updating Git…").FontSize(12).TextColor(p.TextMuted)
			}
			if state.err != "" {
				ui.Text(c, state.err).FontSize(12).TextColor(p.Danger)
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				ui.Button(c, action).Disabled(state.busy).OnClick(m.confirmVCSFileConfirmation)
				ui.Button(c, "Cancel").Disabled(state.busy).OnClick(m.cancelVCSFileConfirmation)
			})
		})
	})
	if (dialog.Dismissed() || !open) && !state.busy {
		m.cancelVCSFileConfirmation()
	}
}
