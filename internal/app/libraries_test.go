package app

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"mayoi/internal/openidea"
)

func TestExternalLibrariesMergeSortAndLabelsMatchOpenIDEA(t *testing.T) {
	groups := mergeLibraryGroups([]openidea.LibraryGroup{
		{
			Name: "Maven: org.example:app:1.0", Kind: "maven",
			Roots: []openidea.LibraryRoot{{Name: "app sources", Path: "/m2/app-sources.jar!"}},
		},
		{
			Name: "Python 3.12 (.venv)", Kind: "python",
			Roots: []openidea.LibraryRoot{{Name: "python3.12", Path: "/venv/lib/python3.12"}},
		},
		{
			Name: "Python 3.12 (.venv)", Kind: "python",
			Roots: []openidea.LibraryRoot{
				{Name: "python3.12", Path: "/venv/lib/python3.12"},
				{Name: "site-packages", Path: "/venv/lib/python3.12/site-packages"},
			},
		},
	})
	groups = sortLibraryGroups(groups)
	if len(groups) != 2 {
		t.Fatalf("merged groups = %+v", groups)
	}
	if groups[0].Kind != "python" || groups[1].Kind != "maven" {
		t.Fatalf("SDK ordering = %s, %s", groups[0].Kind, groups[1].Kind)
	}
	if got := len(groups[0].Roots); got != 2 {
		t.Fatalf("deduplicated roots = %d, want 2", got)
	}
	if got := externalLibraryGroupLabel(groups[0]); got != "< Python 3.12 (.venv) >" {
		t.Fatalf("SDK label = %q", got)
	}
	if got := externalLibraryGroupLabel(groups[1]); got != "Maven: org.example:app:1.0" {
		t.Fatalf("library label = %q", got)
	}
	if got := externalLibraryRootIcon(groups[1].Roots[0]); got != "fileTypes/archive" {
		t.Fatalf("archive icon = %q", got)
	}
}

func TestExternalLibrariesTreeUsesBackendVFSCallback(t *testing.T) {
	rootDir := t.TempDir()
	posts := make(chan func(), 8)
	tree := NewTree(rootDir, func(fn func()) { posts <- fn }, func() {})
	t.Cleanup(tree.Close)
	root := openidea.LibraryRoot{Name: "src.zip", Path: "/jdk/lib/src.zip!"}
	var loaded []string
	tree.SetExternalLibraries([]openidea.LibraryGroup{{
		Name: "21", Kind: "jdk", Roots: []openidea.LibraryRoot{root},
	}}, func(dir string, done func([]openidea.RemoteDirectoryEntry, error)) {
		loaded = append(loaded, dir)
		done([]openidea.RemoteDirectoryEntry{{
			Name: "java.base", Path: root.Path + "/java.base", IsDir: true,
		}, {
			Name: "module-info.java", Path: root.Path + "/module-info.java",
		}}, nil)
	})

	groupID := externalLibraryGroupID(openidea.LibraryGroup{Name: "21", Kind: "jdk"})
	tree.Toggle(externalLibrariesRootID)
	tree.Toggle(groupID)
	tree.Toggle(root.Path)
	deadline := time.After(3 * time.Second)
	for !tree.ws.Loaded(rootDir) {
		select {
		case fn := <-posts:
			fn()
		case <-deadline:
			t.Fatal("timed out loading the workspace tree")
		}
	}
	if !reflect.DeepEqual(loaded, []string{root.Path}) {
		t.Fatalf("VFS loads = %v, want %s", loaded, root.Path)
	}
	rows := tree.Rows()
	labels := map[string]*treeNode{}
	for _, row := range rows {
		labels[row.name] = row
	}
	if labels["External Libraries"] == nil || labels["< 21 >"] == nil ||
		labels["src.zip"] == nil || labels["java.base"] == nil ||
		labels["module-info.java"] == nil {
		t.Fatalf("External Libraries rows = %v", treeLabels(rows))
	}
	if !labels["module-info.java"].readOnly || !tree.ReadOnly(root.Path+"/module-info.java") {
		t.Fatal("archive entry is editable")
	}
	if labels["src.zip"].detail != "library root" {
		t.Fatalf("root detail = %q", labels["src.zip"].detail)
	}
}

func TestRefreshExternalLibrariesUsesStreamingRPCAndReadOnlyRoots(t *testing.T) {
	var args map[string]any
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
		run:      RunPane{trusted: true},
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command != "sdk_project_libraries" {
				t.Fatalf("command = %q", command)
			}
			args = input.(map[string]any)
			return json.RawMessage(`[
				{"name":"17","kind":"jdk","roots":[
					{"name":"src.zip","path":"/jdk/lib/src.zip!"}
				]}
			]`), nil
		},
	}
	m.setRoot("/project")
	m.RefreshExternalLibraries()
	waitFor(t, "External Libraries reply", func() bool {
		return !m.externalLibraries.loading && len(m.externalLibraries.groups) == 1
	})
	if args["request"] == nil || args["onGroups"] == nil {
		t.Fatalf("stream arguments = %#v", args)
	}
	if !m.isExternalLibraryPath("/jdk/lib/src.zip!/java.base/java/lang/Object.java") {
		t.Fatal("archive library path was not recognized")
	}
	if m.isExternalLibraryPath("/other/File.java") {
		t.Fatal("unrelated path was treated as a library")
	}
	if !m.ActionEnabled("libraries.reload") {
		t.Fatal("Reload External Libraries action is unavailable")
	}
}

func TestExternalLibrariesChunksMergeWithoutLosingModuleRoots(t *testing.T) {
	m := &Model{}
	state := m.ensureExternalLibraries()
	state.channel = 7
	if !m.acceptExternalLibrariesChunk(7, json.RawMessage(`[
		{"name":"python","kind":"python","roots":[{"name":"stdlib","path":"/py/stdlib"}]}
	]`)) {
		t.Fatal("first chunk was not routed")
	}
	m.acceptExternalLibrariesChunk(7, json.RawMessage(`[
		{"name":"python","kind":"python","roots":[
			{"name":"stdlib","path":"/py/stdlib"},
			{"name":"site-packages","path":"/py/site-packages"}
		]}
	]`))
	if len(state.groups) != 1 || len(state.groups[0].Roots) != 2 {
		t.Fatalf("merged chunk groups = %+v", state.groups)
	}
}

func TestExternalLibraryManifestDetection(t *testing.T) {
	if !touchesExternalLibraryManifest([]string{"/project/pom.xml"}) {
		t.Fatal("pom.xml did not schedule a library refresh")
	}
	if !touchesExternalLibraryManifest([]string{`C:\project\gradle.properties`}) {
		t.Fatal("gradle.properties did not schedule a library refresh")
	}
	if touchesExternalLibraryManifest([]string{"/project/node_modules/pkg/package.json"}) {
		t.Fatal("a nested node_modules package refreshed External Libraries")
	}
	if touchesExternalLibraryManifest([]string{"/project/src/Main.java"}) {
		t.Fatal("an unrelated source file refreshed External Libraries")
	}
}

func treeLabels(rows []*treeNode) []string {
	labels := make([]string, 0, len(rows))
	for _, row := range rows {
		labels = append(labels, row.name)
	}
	return labels
}
