package app

// SmokeWorkspace exercises OpenIDEA's workspace lifecycle through GoIDEA's
// native UI state against the real backend.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func workspaceSmokeProject(root string) error {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	return os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.test/"+filepath.Base(root)+"\n\ngo 1.23\n"),
		0o644,
	)
}

// SmokeWorkspace proves the workspace actions against real project folders:
// attach and reveal, detach, discovery, create, manage-rename and remove.
func (m *Model) SmokeWorkspace(ctx context.Context) (map[string]any, error) {
	out := map[string]any{}
	var root string
	m.post(func() { root = m.Root() })
	if root == "" {
		return out, fmt.Errorf("workspace smoke needs an open project")
	}
	var tabs []*Tab
	m.post(func() {
		tabs = append([]*Tab(nil), m.tabs...)
		if m.split != nil {
			tabs = append(tabs, m.split.tabs...)
		}
	})
	if err := m.saveOpenTabs(ctx, tabs); err != nil {
		return out, fmt.Errorf("save before workspace smoke: %w", err)
	}
	// Earlier smoke phases may leave a warning related to their own files. It
	// is not a workspace failure, and a real workspace error sets a fresh one.
	m.post(func() {
		m.banner = ""
		m.bannerPath = ""
	})
	var dirty []string
	m.post(func() { dirty = m.dirtyPaths() })
	if len(dirty) > 0 {
		return out, fmt.Errorf("workspace smoke still has unsaved files: %v", dirty)
	}
	base, err := os.MkdirTemp(filepath.Dir(root), ".goidea-workspace-smoke-")
	if err != nil {
		return out, err
	}
	defer os.RemoveAll(base)
	linked := filepath.Join(base, "linked")
	container := filepath.Join(base, "workspace")
	scan := filepath.Join(base, "scan")
	first := filepath.Join(scan, "service")
	second := filepath.Join(scan, "frontend")
	third := filepath.Join(base, "reporting")
	for _, project := range []string{linked, first, second, third} {
		if err := workspaceSmokeProject(project); err != nil {
			return out, err
		}
	}
	for _, project := range []string{linked, third} {
		if _, err := m.invoke(ctx, "project_trust_set", map[string]any{
			"path": project, "trusted": true, "folder": false,
		}); err != nil {
			return out, fmt.Errorf("trust %s: %w", project, err)
		}
	}

	m.post(func() { m.attachWorkspaceProject(linked) })
	attachDeadline := time.Now().Add(60 * time.Second)
	for {
		var attached, revealed bool
		var banner string
		m.post(func() {
			attached = m.workspaceContains(linked)
			revealed = m.tree != nil && workspacePathEqual(m.tree.Selected(), linked)
			banner = m.banner
		})
		if attached && revealed {
			break
		}
		if banner != "" {
			return out, fmt.Errorf("attach workspace project: %s", banner)
		}
		if time.Now().After(attachDeadline) {
			return out, fmt.Errorf("attach workspace project timed out")
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	out["attached"] = true
	out["revealed"] = true

	m.post(func() {
		workspace := *m.workspace
		m.updateWorkspaceProjects(workspace.Name, workspace.Projects, linked)
	})
	detachDeadline := time.Now().Add(60 * time.Second)
	for {
		var attached bool
		var banner string
		m.post(func() {
			attached = m.workspaceContains(linked)
			banner = m.banner
		})
		if !attached {
			break
		}
		if banner != "" {
			return out, fmt.Errorf("detach workspace project: %s", banner)
		}
		if time.Now().After(detachDeadline) {
			return out, fmt.Errorf("detach workspace project timed out")
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	out["detached"] = true

	discovery := &workspaceDialogState{
		open: true, mode: "create", name: "Discovery", path: container,
		projects: []string{root, linked}, selected: map[string]bool{},
	}
	m.post(func() {
		m.workspaceDialog = discovery
		m.discoverWorkspaceProjects(discovery, []string{scan})
	})
	discoveryDeadline := time.Now().Add(60 * time.Second)
	for {
		var scanning bool
		var errorText string
		var projects int
		m.post(func() {
			scanning = discovery.scanning
			errorText = discovery.error
			if discovery.discovery != nil {
				projects = len(discovery.discovery.Projects)
			}
		})
		if !scanning && projects > 0 {
			out["discovered"] = projects
			break
		}
		if errorText != "" {
			m.post(func() { m.workspaceDialog = nil })
			return out, fmt.Errorf("discover workspace projects: %s", errorText)
		}
		if time.Now().After(discoveryDeadline) {
			m.post(func() { m.workspaceDialog = nil })
			return out, fmt.Errorf("discover workspace projects timed out")
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}

	created := &workspaceDialogState{
		open: true, mode: "create", name: "Smoke Workspace", path: container,
		projects: []string{root, linked}, selected: map[string]bool{},
	}
	m.post(func() {
		// A smoke window cannot answer OpenIDEA's Ask new-window prompt. Use
		// Current Window for the created workspace so the action path stays
		// native and headless.
		m.prefs.System.OpenProjectIn = "same"
		m.workspaceDialog = created
		m.saveWorkspaceDialog()
	})
	createDeadline := time.Now().Add(90 * time.Second)
	for {
		var current string
		var busy bool
		var count int
		var phase string
		var dialogError string
		var dirty int
		var dialogOpen bool
		var banner, statusText string
		m.post(func() {
			current, busy, phase = m.Root(), m.busy, m.Phase()
			dialogError = created.error
			dirty = len(m.dirtyPaths())
			dialogOpen = m.workspaceDialog != nil
			banner, statusText = m.banner, m.status
			if m.workspace != nil {
				count = len(m.workspace.Projects)
			}
		})
		if workspacePathEqual(current, container) && !busy && count == 2 {
			break
		}
		if dialogError != "" {
			return out, fmt.Errorf("create workspace: %s", dialogError)
		}
		if phase == "failed" || time.Now().After(createDeadline) {
			return out, fmt.Errorf(
				"create workspace timed out in phase %s (root %q, busy %v, projects %d, dirty %d, dialog %v, status %q, banner %q)",
				phase, current, busy, count, dirty, dialogOpen, statusText, banner,
			)
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
	var createdLabel string
	m.post(func() { createdLabel = m.projectLabel() })
	out["created"] = true
	out["createdLabel"] = createdLabel

	managed := &workspaceDialogState{
		open: true, mode: "manage", name: "Smoke Workspace Managed", path: container,
		projects: []string{root, linked, third}, selected: map[string]bool{},
	}
	m.post(func() {
		m.workspaceDialog = managed
		m.saveWorkspaceDialog()
	})
	manageDeadline := time.Now().Add(60 * time.Second)
	for {
		var open bool
		var count int
		var name, dialogError, banner string
		m.post(func() {
			open = m.workspaceDialog != nil
			dialogError = managed.error
			banner = m.banner
			if m.workspace != nil {
				count, name = len(m.workspace.Projects), m.workspace.Name
			}
		})
		if !open && count == 3 && name == "Smoke Workspace Managed" {
			out["managed"] = true
			out["renamed"] = name
			break
		}
		if dialogError != "" {
			return out, fmt.Errorf("manage workspace: %s", dialogError)
		}
		if banner != "" {
			return out, fmt.Errorf("manage workspace: %s", banner)
		}
		if time.Now().After(manageDeadline) {
			return out, fmt.Errorf("manage workspace timed out")
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}

	m.post(func() {
		workspace := *m.workspace
		m.updateWorkspaceProjects(workspace.Name, workspace.Projects, third)
	})
	removeDeadline := time.Now().Add(60 * time.Second)
	for {
		var count int
		var banner string
		m.post(func() {
			banner = m.banner
			if m.workspace != nil {
				count = len(m.workspace.Projects)
			}
		})
		if count == 2 {
			out["removed"] = true
			break
		}
		if banner != "" {
			return out, fmt.Errorf("remove workspace project: %s", banner)
		}
		if time.Now().After(removeDeadline) {
			return out, fmt.Errorf("remove workspace project timed out")
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return out, nil
}
