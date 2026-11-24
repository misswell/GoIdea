package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"mayoi/internal/openidea"
)

// workspaceDialogState is New Workspace or Manage Workspace. Project discovery
// replaces the dialog body while the user chooses projects, as OpenIDEA does.
type workspaceDialogState struct {
	open      bool
	mode      string
	name      string
	path      string
	projects  []string
	adding    string
	busy      bool
	error     string
	discovery *openidea.ProjectDiscovery
	selected  map[string]bool
	truncated bool
	scanning  bool
	request   uint32
	// parity carries project-level prompts introduced for OpenIDEA File and
	// Find parity. They use the registered workspace modal while they are open.
	parity parityDialogState
}

// setProjectWorkspace applies project_current's manifest to the Project and
// Project Files trees. It retains a copy so later workspace updates can decide
// whether a selected root is a member or the primary attachment.
func (m *Model) setProjectWorkspace(workspace *openidea.Workspace) {
	if workspace == nil {
		m.workspace = nil
		if m.tree != nil {
			m.tree.SetProjectRoots(nil, false)
		}
		return
	}
	copyOf := *workspace
	copyOf.Projects = append([]string(nil), workspace.Projects...)
	m.workspace = &copyOf
	if m.tree != nil {
		m.tree.SetProjectRoots(copyOf.Projects, true)
	}
}

// setProjectInfo applies the names OpenIDEA derives from the project settings
// and workspace manifest. The title bar and project widget use displayName,
// while name remains the editable project name.
func (m *Model) setProjectInfo(info openidea.ProjectInfo) {
	m.projectName = info.Name
	m.projectDisplayName = info.DisplayName
	m.setProjectWorkspace(info.Workspace)
	m.pluginVisualProjectChanged()
	m.applyActive()
}

// projectLabel is the label OpenIDEA shows in the title bar and project widget.
func (m *Model) projectLabel() string {
	if name := strings.TrimSpace(m.projectDisplayName); name != "" {
		return name
	}
	if name := strings.TrimSpace(m.projectName); name != "" {
		return name
	}
	if root := m.Root(); root != "" {
		return filepath.Base(root)
	}
	return "GoIDEA"
}

func (m *Model) workspaceContext() context.Context {
	if m.ctx != nil {
		return m.ctx
	}
	return context.Background()
}

func workspacePathEqual(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return layoutKey(filepath.Clean(a)) == layoutKey(filepath.Clean(b))
}

func (m *Model) workspaceContains(path string) bool {
	if m.workspace == nil {
		return false
	}
	for _, project := range m.workspace.Projects {
		if workspacePathEqual(project, path) {
			return true
		}
	}
	return false
}

func (m *Model) workspaceIsContainer() bool {
	return m.workspace != nil &&
		(len(m.workspace.Projects) == 0 || !workspacePathEqual(m.workspace.Projects[0], m.Root()))
}

func (m *Model) workspaceIsAttachment() bool {
	return m.workspace != nil && len(m.workspace.Projects) > 0 &&
		workspacePathEqual(m.workspace.Projects[0], m.Root()) &&
		len(m.workspace.Projects) > 1
}

// OpenNewWorkspace opens File › New Workspace. With a single project open, its
// root is suggested as the first linked project, as OpenIDEA's dialog does.
func (m *Model) OpenNewWorkspace() {
	if m.busy || m.workspaceDialog != nil {
		return
	}
	state := &workspaceDialogState{
		open:     true,
		mode:     "create",
		name:     "Workspace",
		selected: map[string]bool{},
	}
	if root := m.Root(); root != "" {
		state.path = filepath.Join(filepath.Dir(root), "workspace")
		if m.workspace == nil {
			state.projects = []string{root}
		}
	}
	m.workspaceDialog = state
	m.invalidate()
}

// OpenManageWorkspace opens File › Manage Workspace for the current container
// or attachment.
func (m *Model) OpenManageWorkspace() {
	if m.busy || m.Root() == "" || !m.workspaceIsContainer() || m.workspaceDialog != nil {
		return
	}
	m.workspaceDialog = &workspaceDialogState{
		open:     true,
		mode:     "manage",
		name:     m.workspace.Name,
		path:     m.Root(),
		projects: append([]string(nil), m.workspace.Projects...),
		selected: map[string]bool{},
	}
	m.invalidate()
}

// OpenWorkspace asks for a folder containing .openidea/workspace.json and
// opens it in this window.
func (m *Model) OpenWorkspace() {
	if m.busy {
		return
	}
	parent := m.win
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: parent, Title: "Open Workspace", ButtonLabel: "Open",
			Directory: true, CreateDirectories: false,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		m.post(func() { m.confirmSwitch(paths[0]) })
	}()
}

// AttachProject asks for one trusted project and adds it to the current
// project's workspace without replacing the window's editor session.
func (m *Model) AttachProject() {
	if m.busy || m.Root() == "" {
		return
	}
	parent := m.win
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: parent, Title: "Attach Project", ButtonLabel: "Attach",
			Directory: true, CreateDirectories: false,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		m.attachWorkspaceProject(paths[0])
	}()
}

func (m *Model) attachWorkspaceProject(path string) {
	root := m.Root()
	if root == "" || workspacePathEqual(root, path) {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.workspaceContext(), 60*time.Second)
		defer cancel()
		if err := m.ensureWorkspaceTrust(ctx, []string{path}); err != nil {
			m.post(func() { m.setWorkspaceResult(err) })
			return
		}
		raw, err := m.invoke(ctx, "workspace_attach", map[string]any{"path": path})
		if err == nil {
			raw, err = m.invoke(ctx, "project_current", nil)
		}
		var info openidea.ProjectInfo
		if err == nil {
			info, err = openidea.DecodeProjectInfo(raw)
		}
		m.post(func() {
			if err != nil {
				m.setWorkspaceResult(err)
				return
			}
			if !workspacePathEqual(info.Root, root) {
				m.setWorkspaceResult(errors.New("The backend opened a different project"))
				return
			}
			m.applyWorkspaceProjectInfo(info)
			// OpenIDEA reveals an attached project without moving keyboard focus.
			if m.tree != nil {
				m.tree.reveal(path)
				m.tree.Select(path)
			}
			m.status = "Attached " + filepath.Base(path)
			m.invalidate()
		})
	}()
}

// RemoveWorkspaceProject removes path from the workspace and never changes
// files on disk. In an attachment workspace the primary project cannot be
// detached, and removing another project asks for confirmation.
func (m *Model) RemoveWorkspaceProject(path string) {
	workspace := m.workspace
	if workspace == nil || !m.workspaceContains(path) {
		return
	}
	if m.workspaceIsAttachment() {
		if workspacePathEqual(path, m.Root()) {
			return
		}
		name := filepath.Base(path)
		go func() {
			result, err := mygo.Dialog.Message(mygo.MessageOptions{
				Parent: m.win, Type: mygo.MessageQuestion, Title: "Detach Project",
				Message: fmt.Sprintf("Detach '%s' from the project?", name),
				Detail:  "Its files stay in place.",
				Buttons: []string{"Cancel", "Detach"}, DefaultButton: 1, CancelButton: 0,
			})
			if err != nil || result.Button != 1 {
				return
			}
			m.updateWorkspaceProjects(workspace.Name, workspace.Projects, path)
		}()
		return
	}
	m.updateWorkspaceProjects(workspace.Name, workspace.Projects, path)
}

func (m *Model) updateWorkspaceProjects(name string, projects []string, remove string) {
	included := make([]string, 0, len(projects))
	for _, project := range projects {
		if !workspacePathEqual(project, remove) {
			included = append(included, project)
		}
	}
	if len(included) == len(projects) {
		return
	}
	root := m.Root()
	go func() {
		ctx, cancel := context.WithTimeout(m.workspaceContext(), 60*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "workspace_update", map[string]any{
			"name": name, "projects": included,
		})
		var info openidea.ProjectInfo
		if err == nil {
			info, err = openidea.DecodeProjectInfo(raw)
		}
		m.post(func() {
			if err != nil {
				m.setWorkspaceResult(err)
				return
			}
			if !workspacePathEqual(info.Root, root) {
				m.setWorkspaceResult(errors.New("The backend changed the project while updating the workspace"))
				return
			}
			m.applyWorkspaceProjectInfo(info)
			m.status = "Removed " + filepath.Base(remove) + " from the workspace"
			m.invalidate()
		})
	}()
}

func (m *Model) setWorkspaceResult(err error) {
	if err == nil || errors.Is(err, context.Canceled) {
		return
	}
	m.status = "Workspace: " + err.Error()
	m.banner = err.Error()
	m.invalidate()
}

func (m *Model) applyWorkspaceProjectInfo(info openidea.ProjectInfo) {
	m.setProjectInfo(info)
	if m.tree != nil {
		m.tree.SetExpanded(m.projectViews[layoutKey(info.Root)])
	}
	m.syncProjectTreeState()
	m.saveSession()
}

func (m *Model) closeWorkspaceDialog() {
	state := m.workspaceDialog
	if state == nil {
		return
	}
	if state.request != 0 {
		m.cancelRequest(state.request)
	}
	state.request = 0
	state.scanning = false
	m.workspaceDialog = nil
	m.invalidate()
}

func (m *Model) workspaceDialogCanSave(state *workspaceDialogState) bool {
	return state != nil && !state.busy && !state.scanning && state.discovery == nil &&
		strings.TrimSpace(state.name) != "" && strings.TrimSpace(state.path) != ""
}

func (m *Model) addWorkspaceProjectPath(state *workspaceDialogState, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	for _, project := range state.projects {
		if workspacePathEqual(project, value) {
			state.adding = ""
			return
		}
	}
	state.projects = append(state.projects, value)
	state.adding = ""
}

func (m *Model) saveWorkspaceDialog() {
	state := m.workspaceDialog
	if !m.workspaceDialogCanSave(state) {
		return
	}
	state.adding = strings.TrimSpace(state.adding)
	if state.adding != "" {
		m.addWorkspaceProjectPath(state, state.adding)
	}
	state.busy = true
	state.error = ""
	name := strings.TrimSpace(state.name)
	path := strings.TrimSpace(state.path)
	projects := append([]string(nil), state.projects...)
	mode := state.mode
	previous := []string(nil)
	if m.workspace != nil {
		previous = append(previous, m.workspace.Projects...)
	}
	m.invalidate()

	go func() {
		ctx, cancel := context.WithTimeout(m.workspaceContext(), 2*time.Minute)
		defer cancel()
		var added []string
		if mode == "manage" {
			for _, project := range projects {
				if !stringSliceContainsPath(previous, project) {
					added = append(added, project)
				}
			}
			if err := m.ensureWorkspaceTrust(ctx, added); err != nil {
				m.post(func() { m.finishWorkspaceDialogSave(state, "", openidea.ProjectInfo{}, err) })
				return
			}
		}
		command := "workspace_create"
		if mode == "manage" {
			command = "workspace_update"
		}
		raw, err := m.invoke(ctx, command, map[string]any{
			"path": path, "name": name, "projects": projects,
		})
		openPath := ""
		var info openidea.ProjectInfo
		if err == nil && mode == "create" {
			err = json.Unmarshal(raw, &openPath)
		}
		if err == nil && mode == "manage" {
			info, err = openidea.DecodeProjectInfo(raw)
		}
		m.post(func() { m.finishWorkspaceDialogSave(state, openPath, info, err) })
	}()
}

func stringSliceContainsPath(paths []string, path string) bool {
	for _, candidate := range paths {
		if workspacePathEqual(candidate, path) {
			return true
		}
	}
	return false
}

func (m *Model) finishWorkspaceDialogSave(
	state *workspaceDialogState,
	openPath string,
	info openidea.ProjectInfo,
	err error,
) {
	if m.workspaceDialog != state {
		return
	}
	if err != nil {
		state.busy = false
		state.error = err.Error()
		m.invalidate()
		return
	}
	m.workspaceDialog = nil
	if state.mode == "create" {
		m.invalidate()
		m.confirmSwitch(openPath)
		return
	}
	if !workspacePathEqual(info.Root, m.Root()) {
		m.setWorkspaceResult(errors.New("The backend changed the project while updating the workspace"))
		return
	}
	m.applyWorkspaceProjectInfo(info)
	m.status = "Workspace updated"
	m.invalidate()
}

// ensureWorkspaceTrust mirrors OpenIDEA's attach flow: an undecided project is
// asked about once, and a restricted project is never linked.
func (m *Model) ensureWorkspaceTrust(ctx context.Context, paths []string) error {
	for _, path := range paths {
		raw, err := m.invoke(ctx, "project_trust_status", map[string]any{"path": path})
		if err != nil {
			return err
		}
		var status openidea.TrustStatus
		if err := json.Unmarshal(raw, &status); err != nil {
			return err
		}
		if !status.Trusted && !status.Decided {
			result, err := mygo.Dialog.Message(mygo.MessageOptions{
				Parent: m.win, Type: mygo.MessageQuestion, Title: "Trust Project",
				Message: "Trust this project?",
				Detail:  path,
				Buttons: []string{
					"Preview in Restricted Mode",
					"Don't Open",
					"Trust Project",
				},
				DefaultButton: 2, CancelButton: 1,
			})
			if err != nil {
				return err
			}
			if result.Button != 2 {
				return context.Canceled
			}
			if _, err := m.invoke(ctx, "project_trust_set", map[string]any{
				"path": path, "trusted": true, "folder": false,
			}); err != nil {
				return err
			}
			status.Trusted, status.Decided = true, true
		}
		if !status.Trusted {
			return fmt.Errorf(
				"Cannot attach %s because it is restricted. Open it separately and choose Trust Project, then attach it again.",
				path,
			)
		}
	}
	return nil
}

func (m *Model) browseWorkspaceLocation(state *workspaceDialogState) {
	parent := m.win
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: parent, Title: "Workspace Location", ButtonLabel: "Choose",
			Directory: true, CreateDirectories: true,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		m.post(func() {
			if m.workspaceDialog == state {
				state.path = paths[0]
				m.invalidate()
			}
		})
	}()
}

func (m *Model) browseWorkspaceProjects(state *workspaceDialogState) {
	parent := m.win
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: parent, Title: "Add Projects", ButtonLabel: "Add",
			Directory: true, Multiple: true, CreateDirectories: false,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		m.discoverWorkspaceProjects(state, paths)
	}()
}

func (m *Model) discoverWorkspaceProjects(state *workspaceDialogState, folders []string) {
	if state == nil || state.busy || state.scanning {
		return
	}
	request := m.nextRequest()
	state.request = request
	state.scanning = true
	state.error = ""
	state.truncated = false
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.workspaceContext(), 2*time.Minute)
		defer cancel()
		raw, err := m.invoke(ctx, "workspace_discover_projects", map[string]any{
			"request": request, "folders": folders,
		})
		var discovery openidea.ProjectDiscovery
		if err == nil {
			err = json.Unmarshal(raw, &discovery)
		}
		m.post(func() {
			if m.workspaceDialog != state || state.request != request {
				return
			}
			state.request = 0
			state.scanning = false
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					state.error = err.Error()
					m.invalidate()
				}
				return
			}
			for _, project := range discovery.Direct {
				m.addWorkspaceProjectPath(state, project)
			}
			state.truncated = discovery.Truncated
			if len(discovery.Projects) > 0 {
				copyOf := discovery
				state.discovery = &copyOf
				state.selected = map[string]bool{}
				for _, project := range discovery.Projects {
					if !stringSliceContainsPath(state.projects, project.Path) &&
						!stringSliceContainsPath(discovery.Direct, project.Path) {
						state.selected[project.Path] = true
					}
				}
			}
			m.invalidate()
		})
	}()
}

func (m *Model) addSelectedWorkspaceProjects(state *workspaceDialogState) {
	if state == nil || state.discovery == nil {
		return
	}
	for _, project := range state.discovery.Projects {
		if state.selected[project.Path] {
			m.addWorkspaceProjectPath(state, project.Path)
		}
	}
	state.discovery = nil
	state.selected = map[string]bool{}
	m.invalidate()
}

// workspaceActionState follows OpenIDEA's project.* and workspace.* context
// keys. The two removal actions accept the selected project path as their
// argument and fall back to the Project View selection from the palette.
func workspaceActionState(a *Action) {
	selected := func(m *Model) string {
		if target, ok := m.selectedTreeEntry(); ok {
			return target.path
		}
		return ""
	}
	switch a.ID {
	case "project.detach", "workspace.removeProject":
		a.RunWithArg = func(m *Model, arg any) {
			path, _ := arg.(string)
			if strings.TrimSpace(path) == "" {
				path = selected(m)
			}
			m.RemoveWorkspaceProject(path)
		}
	}
	switch a.ID {
	case "project.attach":
		a.When = func(m *Model) bool { return m.Root() != "" && !m.busy }
	case "project.detach":
		a.When = func(m *Model) bool { return m.workspaceIsAttachment() }
	case "workspace.removeProject":
		a.When = func(m *Model) bool { return m.workspaceIsContainer() }
	case "workspace.manage":
		a.When = func(m *Model) bool { return m.Root() != "" && m.workspaceIsContainer() }
	}
}

// drawWorkspaceDialog draws File › New Workspace and File › Manage Workspace.
func (m *Model) drawWorkspaceDialog(c *ui.Context, p *ui.Theme) {
	state := m.workspaceDialog
	if state == nil || !state.open {
		return
	}
	open := state.open
	dialog := ui.Modal(c, &open, func() {
		if state.parity.Kind != "" {
			m.drawParityWorkspaceDialog(c, p, state)
		} else if state.discovery != nil {
			m.workspaceDiscoverySheet(c, p, state)
		} else {
			m.workspaceEditorSheet(c, p, state)
		}
	})
	if dialog.Dismissed() || !open {
		if !state.busy {
			m.closeWorkspaceDialog()
		}
	}
}

func (m *Model) workspaceEditorSheet(c *ui.Context, p *ui.Theme, state *workspaceDialogState) {
	oi := oiPaletteOf(p)
	title := "New Workspace"
	action := "Create"
	if state.mode == "manage" {
		title, action = "Manage Workspace", "Apply"
	}
	disabled := state.busy || state.scanning
	ui.Column(c).Width(640).Height(560).Padding(18, 20, 16, 20).Gap(12).
		Background(oi.elev).Border(1, oi.border).Radius(6).Children(func() {
		ui.Text(c, title).FontSize(14).Bold().TextColor(oi.textStrong).SingleLine()
		ui.Text(c, "Name").FontSize(11).TextColor(oi.textMuted).SingleLine()
		if input := ui.TextInput(c, &state.name).Disabled(disabled); input.Submitted() {
			m.saveWorkspaceDialog()
		}
		ui.Text(c, "Location").FontSize(11).TextColor(oi.textMuted).SingleLine()
		ui.Row(c).Gap(8).Children(func() {
			ui.TextInput(c, &state.path).Grow(1).Disabled(disabled || state.mode == "manage")
			if state.mode == "create" {
				ui.Button(c, "Choose…").Disabled(disabled).OnClick(func() {
					m.browseWorkspaceLocation(state)
				})
			}
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Linked projects").FontSize(12).Bold().TextColor(oi.textStrong).SingleLine()
			ui.Spacer(c)
			ui.Button(c, "Add Projects…").Disabled(disabled).OnClick(func() {
				m.browseWorkspaceProjects(state)
			})
		})
		ui.Scroll(c).Height(250).MinHeight(0).Children(func() {
			ui.Column(c).Gap(3).Children(func() {
				if len(state.projects) == 0 {
					ui.Text(c, "Add the projects you want to work on together.").
						FontSize(12).TextColor(oi.textMuted)
					return
				}
				for i, project := range state.projects {
					i, project := i, project
					ui.Row(c).Height(34).Padding(0, 8).Gap(8).AlignItems(ui.Center).
						Background(oi.island).Border(1, oi.border).Radius(4).Children(func() {
						ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
							ui.Text(c, filepath.Base(project)).FontSize(12).
								TextColor(oi.textStrong).SingleLine().Ellipsis("…")
							ui.Text(c, project).FontSize(10).TextColor(oi.textMuted).
								SingleLine().Ellipsis("…")
						})
						remove := ui.Button(c, "Remove").Disabled(disabled)
						remove.OnClick(func() {
							state.projects = append(state.projects[:i], state.projects[i+1:]...)
							m.invalidate()
						})
					})
				}
			})
		})
		ui.Row(c).Gap(8).Children(func() {
			input := ui.TextInput(c, &state.adding).Grow(1).Disabled(disabled).
				Placeholder("Project folder path")
			if input.Submitted() {
				m.addWorkspaceProjectPath(state, state.adding)
				m.invalidate()
			}
			if ui.Button(c, "Add").Disabled(disabled || strings.TrimSpace(state.adding) == "").Clicked() {
				m.addWorkspaceProjectPath(state, state.adding)
				m.invalidate()
			}
		})
		ui.Text(c, "Project folders stay in place. Removing a project here keeps its files.").
			FontSize(11).TextColor(oi.textMuted)
		if state.scanning {
			ui.Text(c, "Searching for projects…").FontSize(11).TextColor(oi.textMuted).SingleLine()
		}
		if state.truncated {
			ui.Text(c, "Search stopped at the entry limit. More projects may exist; choose a smaller folder to search again.").
				FontSize(11).TextColor(oi.textMuted)
		}
		if state.error != "" {
			ui.Text(c, state.error).FontSize(11).TextColor(oi.danger)
		}
		ui.Spacer(c)
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, "Cancel").Disabled(state.busy).Clicked() {
				m.closeWorkspaceDialog()
			}
			if ui.PrimaryButton(c, action).Disabled(!m.workspaceDialogCanSave(state)).Clicked() {
				m.saveWorkspaceDialog()
			}
		})
	})
}

func (m *Model) workspaceDiscoverySheet(c *ui.Context, p *ui.Theme, state *workspaceDialogState) {
	oi := oiPaletteOf(p)
	discovery := state.discovery
	if discovery == nil {
		return
	}
	count := 0
	for _, project := range discovery.Projects {
		if state.selected[project.Path] && !stringSliceContainsPath(state.projects, project.Path) {
			count++
		}
	}
	ui.Column(c).Width(640).Height(500).Padding(18, 20, 16, 20).Gap(10).
		Background(oi.elev).Border(1, oi.border).Radius(6).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Choose Projects").FontSize(14).Bold().TextColor(oi.textStrong).SingleLine()
			ui.Spacer(c)
			if ui.Button(c, "Select All").Clicked() {
				for _, project := range discovery.Projects {
					if !stringSliceContainsPath(state.projects, project.Path) {
						state.selected[project.Path] = true
					}
				}
				m.invalidate()
			}
			if ui.Button(c, "Select None").Clicked() {
				clear(state.selected)
				m.invalidate()
			}
		})
		ui.Text(c, "Choose the projects to link to this workspace.").
			FontSize(11).TextColor(oi.textMuted)
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).Gap(3).Children(func() {
				for _, project := range discovery.Projects {
					project := project
					linked := stringSliceContainsPath(state.projects, project.Path)
					checked := state.selected[project.Path] && !linked
					ui.Row(c).Height(40).Padding(0, 8).Gap(8).AlignItems(ui.Center).
						Background(oi.island).Border(1, oi.border).Radius(4).Children(func() {
						if ui.Checkbox(c, &checked, "").Disabled(linked).Changed() {
							state.selected[project.Path] = checked
							m.invalidate()
						}
						ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
							label := project.RelativePath
							if label == "" {
								label = filepath.Base(project.Path)
							}
							ui.Text(c, label).FontSize(12).TextColor(oi.textStrong).
								SingleLine().Ellipsis("…")
							detail := filepath.Base(project.Root)
							if linked {
								detail = "Already linked"
							}
							ui.Text(c, detail).FontSize(10).TextColor(oi.textMuted).
								SingleLine().Ellipsis("…")
						})
					})
				}
			})
		})
		if state.truncated {
			ui.Text(c, "Search stopped at the entry limit. More projects may exist; choose a smaller folder to search again.").
				FontSize(11).TextColor(oi.textMuted)
		}
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, "Back").Clicked() {
				state.discovery = nil
				state.selected = map[string]bool{}
				m.invalidate()
			}
			if ui.PrimaryButton(c, "Add Selected").Disabled(count == 0).Clicked() {
				m.addSelectedWorkspaceProjects(state)
			}
		})
	})
}
