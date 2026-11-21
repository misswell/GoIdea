package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mayoi/internal/openidea"
)

func waitWorkspaceTest(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("workspace request did not finish")
	}
}

func workspaceTestModel(root string) *Model {
	return &Model{ctx: context.Background(), root: root}
}

func TestWorkspaceActionsFollowOpenIDEACallContexts(t *testing.T) {
	primary := filepath.Join(t.TempDir(), "primary")
	member := filepath.Join(t.TempDir(), "member")
	containerRoot := filepath.Join(t.TempDir(), "workspace")
	for _, path := range []string{primary, member, containerRoot} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	attachment := workspaceTestModel(primary)
	attachment.workspace = &openidea.Workspace{
		Version: 1, Name: "Attached", Projects: []string{primary, member},
	}
	container := workspaceTestModel(containerRoot)
	container.workspace = &openidea.Workspace{
		Version: 1, Name: "Container", Projects: []string{member},
	}
	ordinary := workspaceTestModel(primary)

	when := func(id string, m *Model) bool {
		t.Helper()
		a := actions.byID[id]
		if a == nil || a.When == nil {
			t.Fatalf("workspace action %q is not registered with a context", id)
		}
		return a.When(m)
	}

	for _, test := range []struct {
		name         string
		attachment   bool
		container    bool
		ordinary     bool
		action       string
		attachmentID string
		containerID  string
	}{
		{"attach", true, true, true, "project.attach", "", ""},
		{"detach", true, false, false, "project.detach", "project.detach", ""},
		{"remove", false, true, false, "workspace.removeProject", "", "workspace.removeProject"},
		{"manage", false, true, false, "workspace.manage", "", "workspace.manage"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := when(test.action, attachment); got != test.attachment {
				t.Fatalf("attachment context = %v, want %v", got, test.attachment)
			}
			if got := when(test.action, container); got != test.container {
				t.Fatalf("container context = %v, want %v", got, test.container)
			}
			if got := when(test.action, ordinary); got != test.ordinary {
				t.Fatalf("ordinary project context = %v, want %v", got, test.ordinary)
			}
			if test.attachmentID != "" && test.attachmentID != test.action {
				t.Fatalf("bad attachment action alias %q", test.attachmentID)
			}
			if test.containerID != "" && test.containerID != test.action {
				t.Fatalf("bad container action alias %q", test.containerID)
			}
		})
	}
}

func TestWorkspaceDialogDefaultsAndManageEligibility(t *testing.T) {
	root := filepath.Join(t.TempDir(), "app")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	m := workspaceTestModel(root)
	m.OpenNewWorkspace()
	dialog := m.workspaceDialog
	if dialog == nil || dialog.mode != "create" || dialog.name != "Workspace" {
		t.Fatalf("new workspace dialog = %+v", dialog)
	}
	if want := filepath.Join(filepath.Dir(root), "workspace"); dialog.path != want {
		t.Fatalf("workspace location = %q, want %q", dialog.path, want)
	}
	if len(dialog.projects) != 1 || !workspacePathEqual(dialog.projects[0], root) {
		t.Fatalf("new workspace projects = %v", dialog.projects)
	}

	m.workspaceDialog = nil
	m.workspace = &openidea.Workspace{Version: 1, Name: "Existing", Projects: []string{root}}
	m.OpenManageWorkspace()
	if m.workspaceDialog != nil {
		t.Fatalf("attachment should not be manageable: %+v", m.workspaceDialog)
	}

	container := filepath.Join(t.TempDir(), "team")
	m.root = container
	m.workspace = &openidea.Workspace{Version: 1, Name: "Existing", Projects: []string{root}}
	m.OpenManageWorkspace()
	if m.workspaceDialog == nil || m.workspaceDialog.mode != "manage" ||
		!workspacePathEqual(m.workspaceDialog.path, container) {
		t.Fatalf("container should be manageable: %+v", m.workspaceDialog)
	}

	m.workspaceDialog = nil
	m.root = ""
	m.OpenManageWorkspace()
	if m.workspaceDialog != nil {
		t.Fatal("a workspace opened without a container root")
	}

	m.root = container
	m.workspace = &openidea.Workspace{Version: 1, Name: "Empty"}
	if !m.workspaceIsContainer() {
		t.Fatal("an empty workspace must still be manageable")
	}
	m.OpenManageWorkspace()
	if m.workspaceDialog == nil || len(m.workspaceDialog.projects) != 0 {
		t.Fatalf("empty workspace dialog = %+v", m.workspaceDialog)
	}
}

func TestNewWorkspaceCallsTheOpenIDEABackendWithoutTrustPrompts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "app")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	location := filepath.Join(t.TempDir(), "team-workspace")
	called := make(chan struct{})
	m := workspaceTestModel(root)
	m.postHook = func(func()) {}
	m.invokeHook = func(_ context.Context, command string, input any) (json.RawMessage, error) {
		if command != "workspace_create" {
			t.Errorf("unexpected command %q", command)
		}
		request, ok := input.(map[string]any)
		if !ok {
			t.Errorf("workspace_create args = %#v", input)
		} else {
			if request["name"] != "Team" || request["path"] != location {
				t.Errorf("workspace_create request = %#v", request)
			}
			projects, _ := request["projects"].([]string)
			if len(projects) != 1 || !workspacePathEqual(projects[0], root) {
				t.Errorf("workspace_create projects = %#v", projects)
			}
		}
		close(called)
		return json.Marshal(location)
	}
	state := &workspaceDialogState{
		open: true, mode: "create", name: " Team ", path: location,
		projects: []string{root}, selected: map[string]bool{},
	}
	m.workspaceDialog = state
	m.saveWorkspaceDialog()
	select {
	case <-called:
	case <-time.After(2 * time.Second):
		t.Fatal("workspace_create was not called")
	}
	if !state.busy {
		t.Fatal("the create request was not marked busy")
	}
}

func TestManageWorkspaceAppliesBackendInfoWithoutReopeningProject(t *testing.T) {
	root := filepath.Join(t.TempDir(), "app")
	added := filepath.Join(t.TempDir(), "library")
	for _, path := range []string{root, added} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	done := make(chan struct{})
	var commands []string
	var update any
	m := workspaceTestModel(root)
	m.postHook = func(fn func()) {
		fn()
		close(done)
	}
	m.invokeHook = func(_ context.Context, command string, input any) (json.RawMessage, error) {
		commands = append(commands, command)
		switch command {
		case "project_trust_status":
			request := input.(map[string]any)
			if !workspacePathEqual(request["path"].(string), added) {
				t.Errorf("trust path = %#v", request["path"])
			}
			return json.Marshal(openidea.TrustStatus{Trusted: true, Decided: true})
		case "workspace_update":
			update = input
			return json.Marshal(openidea.ProjectInfo{
				Trusted: true, Root: root, Name: "Team",
				Workspace: &openidea.Workspace{
					Version: 1, Name: "Team", Projects: []string{root, added},
				},
			})
		default:
			t.Errorf("unexpected command %q", command)
			return nil, nil
		}
	}
	state := &workspaceDialogState{
		open: true, mode: "manage", name: "Team", path: root,
		projects: []string{root, added}, selected: map[string]bool{},
	}
	m.workspace = &openidea.Workspace{
		Version: 1, Name: "Old", Projects: []string{root},
	}
	m.workspaceDialog = state
	m.saveWorkspaceDialog()
	waitWorkspaceTest(t, done)

	if len(commands) != 2 || commands[0] != "project_trust_status" || commands[1] != "workspace_update" {
		t.Fatalf("manage commands = %v", commands)
	}
	request := update.(map[string]any)
	if request["name"] != "Team" {
		t.Fatalf("workspace_update name = %#v", request["name"])
	}
	projects := request["projects"].([]string)
	if len(projects) != 2 || !workspacePathEqual(projects[1], added) {
		t.Fatalf("workspace_update projects = %#v", projects)
	}
	if m.workspaceDialog != nil || m.Root() != root {
		t.Fatalf("manage dialog or root changed: dialog=%+v root=%q", m.workspaceDialog, m.Root())
	}
	if m.workspace == nil || len(m.workspace.Projects) != 2 ||
		!workspacePathEqual(m.workspace.Projects[1], added) {
		t.Fatalf("workspace info not applied: %+v", m.workspace)
	}
	if m.status != "Workspace updated" {
		t.Fatalf("manage status = %q", m.status)
	}
}

func TestAttachProjectUsesTrustAndKeepsTheCurrentEditorRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "app")
	added := filepath.Join(t.TempDir(), "library")
	for _, path := range []string{root, added} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	done := make(chan struct{})
	var commands []string
	var attach any
	m := workspaceTestModel(root)
	tree := NewTree(root, func(fn func()) { fn() }, func() {})
	t.Cleanup(tree.Close)
	m.tree = tree
	m.postHook = func(fn func()) {
		fn()
		close(done)
	}
	m.invokeHook = func(_ context.Context, command string, input any) (json.RawMessage, error) {
		commands = append(commands, command)
		switch command {
		case "project_trust_status":
			return json.Marshal(openidea.TrustStatus{Trusted: true, Decided: true})
		case "workspace_attach":
			attach = input
			return json.Marshal(root)
		case "project_current":
			return json.Marshal(openidea.ProjectInfo{
				Trusted: true, Root: root, Name: "App", DisplayName: "App, library",
				Workspace: &openidea.Workspace{
					Version: 1, Name: "App", Projects: []string{root, added},
				},
			})
		default:
			t.Errorf("unexpected command %q", command)
			return nil, nil
		}
	}
	m.attachWorkspaceProject(added)
	waitWorkspaceTest(t, done)

	if strings.Join(commands, ",") != "project_trust_status,workspace_attach,project_current" {
		t.Fatalf("attach commands = %v", commands)
	}
	request := attach.(map[string]any)
	if !workspacePathEqual(request["path"].(string), added) {
		t.Fatalf("workspace_attach path = %#v", request["path"])
	}
	if m.Root() != root || m.workspace == nil || len(m.workspace.Projects) != 2 {
		t.Fatalf("attach changed the wrong root or workspace: root=%q workspace=%+v", m.Root(), m.workspace)
	}
	if m.projectLabel() != "App, library" {
		t.Fatalf("attach project label = %q", m.projectLabel())
	}
	if tree.Selected() != added || !tree.open[added] {
		t.Fatalf("attached project was not revealed: selected=%q expanded=%v", tree.Selected(), tree.open)
	}
	if m.status != "Attached library" {
		t.Fatalf("attach status = %q", m.status)
	}
}

func TestWorkspaceProjectLabelDrivesTheOpenIDEATitle(t *testing.T) {
	m := workspaceTestModel("")
	if got := m.windowTitle(""); got != "GoIDEA" {
		t.Fatalf("empty title = %q", got)
	}
	if got := m.windowTitle("Standalone.java"); got != "Standalone.java" {
		t.Fatalf("standalone document title = %q", got)
	}

	m.root = "/work/workspace"
	if got := m.windowTitle(""); got != "workspace" {
		t.Fatalf("folder title = %q", got)
	}

	m.projectName = "Team"
	m.projectDisplayName = "Team, frontend"
	if got := m.projectLabel(); got != "Team, frontend" {
		t.Fatalf("project label = %q", got)
	}
	if got := m.windowTitle("Main.java"); got != "Team, frontend – Main.java" {
		t.Fatalf("document title = %q", got)
	}
}

func TestAttachProjectRefusesARestrictedFolder(t *testing.T) {
	root := filepath.Join(t.TempDir(), "app")
	restricted := filepath.Join(t.TempDir(), "restricted")
	for _, path := range []string{root, restricted} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	done := make(chan struct{})
	attached := false
	m := workspaceTestModel(root)
	m.postHook = func(fn func()) {
		fn()
		close(done)
	}
	m.invokeHook = func(_ context.Context, command string, _ any) (json.RawMessage, error) {
		if command == "workspace_attach" {
			attached = true
		}
		if command != "project_trust_status" {
			t.Errorf("unexpected command %q", command)
		}
		return json.Marshal(openidea.TrustStatus{Trusted: false, Decided: true})
	}
	m.attachWorkspaceProject(restricted)
	waitWorkspaceTest(t, done)

	if attached {
		t.Fatal("a restricted folder reached workspace_attach")
	}
	if !strings.Contains(m.banner, "restricted") ||
		!strings.Contains(m.banner, "Open it separately and choose Trust Project") {
		t.Fatalf("restricted attachment banner = %q", m.banner)
	}
	if m.workspace != nil {
		t.Fatalf("restricted folder changed the workspace: %+v", m.workspace)
	}
}

func TestWorkspaceDiscoveryAddsDirectProjectsAndOffersFoundProjects(t *testing.T) {
	root := filepath.Join(t.TempDir(), "app")
	direct := filepath.Join(t.TempDir(), "direct")
	first := filepath.Join(t.TempDir(), "service-a")
	second := filepath.Join(t.TempDir(), "service-b")
	done := make(chan struct{})
	m := workspaceTestModel(root)
	m.postHook = func(fn func()) {
		fn()
		close(done)
	}
	m.invokeHook = func(_ context.Context, command string, input any) (json.RawMessage, error) {
		if command != "workspace_discover_projects" {
			t.Errorf("unexpected command %q", command)
		}
		request := input.(map[string]any)
		if request["request"].(uint32) == 0 {
			t.Error("discovery request id is zero")
		}
		folders := request["folders"].([]string)
		if len(folders) != 1 || !workspacePathEqual(folders[0], direct) {
			t.Errorf("discovery folders = %#v", folders)
		}
		return json.Marshal(openidea.ProjectDiscovery{
			Direct: []string{direct},
			Projects: []openidea.DiscoveredProject{
				{Path: first, Root: direct, RelativePath: "service-a"},
				{Path: second, Root: direct, RelativePath: "service-b"},
			},
			Truncated: true,
		})
	}
	state := &workspaceDialogState{
		open: true, mode: "create", name: "Workspace", path: filepath.Dir(root),
		projects: []string{root}, selected: map[string]bool{},
	}
	m.workspaceDialog = state
	m.discoverWorkspaceProjects(state, []string{direct})
	waitWorkspaceTest(t, done)

	if state.scanning || state.request != 0 || !state.truncated {
		t.Fatalf("discovery state = %+v", state)
	}
	if len(state.projects) != 2 ||
		!workspacePathEqual(state.projects[0], root) ||
		!workspacePathEqual(state.projects[1], direct) {
		t.Fatalf("direct projects = %v", state.projects)
	}
	if state.discovery == nil || len(state.discovery.Projects) != 2 {
		t.Fatalf("found projects = %+v", state.discovery)
	}
	if !state.selected[first] || !state.selected[second] {
		t.Fatalf("found project selection = %v", state.selected)
	}

	m.addSelectedWorkspaceProjects(state)
	if state.discovery != nil || len(state.projects) != 4 {
		t.Fatalf("selected projects were not added: %+v", state)
	}
}

func TestWorkspaceDiscoveryReportsAFailedScan(t *testing.T) {
	root := filepath.Join(t.TempDir(), "app")
	done := make(chan struct{})
	m := workspaceTestModel(root)
	m.postHook = func(fn func()) {
		fn()
		close(done)
	}
	m.invokeHook = func(_ context.Context, command string, _ any) (json.RawMessage, error) {
		if command != "workspace_discover_projects" {
			t.Errorf("unexpected command %q", command)
		}
		return nil, errors.New("scan failed")
	}
	state := &workspaceDialogState{
		open: true, mode: "create", name: "Workspace", path: filepath.Dir(root),
		projects: []string{root}, selected: map[string]bool{},
	}
	m.workspaceDialog = state
	m.discoverWorkspaceProjects(state, []string{root})
	waitWorkspaceTest(t, done)

	if state.scanning || state.request != 0 || state.error != "scan failed" {
		t.Fatalf("failed discovery state = %+v", state)
	}
}

func TestClosingWorkspaceCancelsDiscoveryAndDropsItsReply(t *testing.T) {
	root := filepath.Join(t.TempDir(), "app")
	started := make(chan uint32, 1)
	release := make(chan struct{})
	cancelled := make(chan struct{}, 1)
	finished := make(chan struct{}, 1)
	m := workspaceTestModel(root)
	m.postHook = func(fn func()) {
		fn()
		finished <- struct{}{}
	}
	m.invokeHook = func(_ context.Context, command string, input any) (json.RawMessage, error) {
		switch command {
		case "workspace_discover_projects":
			request := input.(map[string]any)["request"].(uint32)
			started <- request
			<-release
			return json.Marshal(openidea.ProjectDiscovery{
				Direct: []string{root},
			})
		case "cancel_request":
			cancelled <- struct{}{}
			return json.RawMessage(`null`), nil
		default:
			t.Errorf("unexpected command %q", command)
			return nil, nil
		}
	}
	state := &workspaceDialogState{
		open: true, mode: "create", name: "Workspace", path: filepath.Dir(root),
		projects: []string{root}, selected: map[string]bool{},
	}
	m.workspaceDialog = state
	m.discoverWorkspaceProjects(state, []string{root})
	request := <-started
	if state.request != request {
		t.Fatalf("active discovery request = %d, want %d", state.request, request)
	}
	m.closeWorkspaceDialog()
	if m.workspaceDialog != nil {
		t.Fatal("the workspace dialog stayed open")
	}
	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("closing the workspace did not cancel discovery")
	}
	close(release)
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("the late discovery reply was not processed")
	}
	if m.workspaceDialog != nil || state.request != 0 || state.scanning ||
		len(state.projects) != 1 {
		t.Fatalf("late discovery changed closed state: dialog=%+v state=%+v", m.workspaceDialog, state)
	}
}

func TestWorkspaceRootsDriveAndReleaseTheProjectTreeWatchers(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	first := filepath.Join(t.TempDir(), "service-a")
	second := filepath.Join(t.TempDir(), "service-b")
	for _, path := range []string{root, first, second} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	tree := NewTree(root, func(fn func()) { fn() }, func() {})
	t.Cleanup(tree.Close)
	m := workspaceTestModel(root)
	m.tree = tree
	m.setProjectWorkspace(&openidea.Workspace{
		Version: 1, Name: "Workspace", Projects: []string{first, second},
	})
	if !tree.multiRoot || len(tree.projectRoots) != 2 ||
		!workspacePathEqual(tree.projectRoots[0], first) ||
		!workspacePathEqual(tree.projectRoots[1], second) {
		t.Fatalf("workspace roots = %v multi=%v", tree.projectRoots, tree.multiRoot)
	}
	if len(tree.extraTrees) != 2 || tree.extraTrees[first] == nil || tree.extraTrees[second] == nil {
		t.Fatalf("workspace watchers = %v", tree.extraTrees)
	}
	if !tree.open[first] || !tree.open[second] {
		t.Fatalf("workspace roots are not expanded: %v", tree.open)
	}

	m.setProjectWorkspace(nil)
	if tree.multiRoot || len(tree.projectRoots) != 0 || len(tree.extraTrees) != 0 {
		t.Fatalf("workspace tree was not released: roots=%v multi=%v watchers=%v",
			tree.projectRoots, tree.multiRoot, tree.extraTrees)
	}
}

func TestWorkspaceContextMenuUsesContainerAndAttachmentLanguage(t *testing.T) {
	container := treeTarget{
		path: "/workspace/service", dir: true, root: true,
		workspaceMember: true, workspaceCurrentRoot: false,
	}
	items := treeMenuItems(container, false)
	if got := menuItem(t, items, "Remove from Workspace"); !got.Enabled {
		t.Fatal("container member cannot be removed")
	}
	if labels := strings.Join(treeMenuLabels(items), "|"); strings.Contains(labels, "Detach Project") {
		t.Fatalf("container menu says detach: %s", labels)
	}
	if menuItem(t, items, "Delete…").Enabled {
		t.Fatal("a workspace member root can be deleted")
	}

	attachment := treeTarget{
		path: "/workspace/library", dir: true, root: true,
		workspaceMember: true, workspaceCurrentRoot: false, workspaceAttachment: true,
	}
	items = treeMenuItems(attachment, false)
	if got := menuItem(t, items, "Detach Project"); !got.Enabled {
		t.Fatal("attachment member cannot be detached")
	}
	if labels := strings.Join(treeMenuLabels(items), "|"); strings.Contains(labels, "Remove from Workspace") {
		t.Fatalf("attachment menu says remove: %s", labels)
	}

	current := treeTarget{
		path: "/workspace/app", dir: true, root: true,
		workspaceMember: true, workspaceCurrentRoot: true, workspaceAttachment: true,
	}
	labels := strings.Join(treeMenuLabels(treeMenuItems(current, false)), "|")
	if strings.Contains(labels, "Detach Project") || strings.Contains(labels, "Remove from Workspace") {
		t.Fatalf("the primary attachment can be detached: %s", labels)
	}
}
