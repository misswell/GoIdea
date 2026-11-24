package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"mayoi/internal/openidea"
)

func TestProjectViewOpenIDEAActionsAndOptions(t *testing.T) {
	ids := []string{
		"project.option.showHidden",
		"project.option.showExcluded",
		"project.option.showMembers",
		"project.option.showRootPaths",
		"project.option.compact",
		"project.option.foldersFirst",
		"project.option.autoscrollFromSource",
		"project.option.autoscrollToSource",
		"project.option.singleClickOpen",
		"project.option.singleClickExpand",
		"project.option.previewTab",
		"project.option.showUnityMeta",
		"project.option.showUnityGenerated",
		"project.viewMode.project",
		"project.viewMode.projectFiles",
		"project.viewMode.openFiles",
		"project.viewMode.changedFiles",
		"project.viewMode.scratches",
		"project.viewMode.unity",
		"project.sortBy.name",
		"project.sortBy.type",
		"project.sortBy.modified",
	}
	for _, id := range ids {
		if _, ok := Command(id); !ok {
			t.Errorf("missing OpenIDEA Project View action %q", id)
		}
	}

	settings := filepath.Join(t.TempDir(), "settings.json")
	m := &Model{cfg: Config{SettingsPath: settings}}
	m.SetProjectTreeOption("showHidden", true)
	m.SetProjectTreeOption("showExcluded", false)
	m.SetProjectTreeOption("compact", false)
	m.SetProjectTreeSort("modified")
	if !m.ActionToggled("project.option.showHidden") ||
		m.ActionToggled("project.option.showExcluded") ||
		m.ActionToggled("project.option.compact") ||
		!m.ActionToggled("project.sortBy.modified") {
		t.Fatalf("Project View toggle state is wrong: %+v", m.prefs.ProjectTree)
	}
	stored := LoadPreferences(settings).ProjectTree
	if !stored.ShowHidden || stored.showsExcluded() || stored.compacts() || stored.normalizedSort() != "modified" {
		t.Fatalf("stored Project View options = %+v", stored)
	}
}

func TestProjectTreeViewOptionsFilterSortAndCompact(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "src", "main", "java", "com", "example")
	for _, dir := range []string{deep, filepath.Join(root, "target"), filepath.Join(root, ".git"), filepath.Join(root, "zeta")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range []string{
		filepath.Join(root, "alpha.txt"),
		filepath.Join(root, "secret.cache"),
		filepath.Join(root, "generated", "data.bin"),
		filepath.Join(deep, "App.java"),
		filepath.Join(root, "target", "Generated.java"),
	} {
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	q := make(mainQueue, 256)
	tree := NewTree(root, q.post, func() {})
	defer tree.Close()
	q.drain(t, "project roots", func() bool { return hasRow(tree, filepath.Join(root, "src")) })

	if hasRow(tree, filepath.Join(root, ".git")) {
		t.Fatal("ignored VCS metadata showed without Show Hidden Files")
	}
	if !hasRow(tree, filepath.Join(root, "target")) {
		t.Fatal("excluded files were hidden by default")
	}
	tree.SetFileFilters([]string{"*.cache"}, []string{"**/generated"})
	q.drain(t, "File Types filters", func() bool {
		return !hasRow(tree, filepath.Join(root, "secret.cache")) &&
			hasRow(tree, filepath.Join(root, "generated"))
	})
	for _, row := range tree.Rows() {
		if row.path == filepath.Join(root, "generated") && !row.excluded {
			t.Fatal("files.exclude did not dim the generated folder")
		}
	}

	options := tree.options
	options.showHidden = true
	options.showExcluded = false
	options.compact = false
	tree.SetOptions(options)
	q.drain(t, "ignored VCS metadata", func() bool { return hasRow(tree, filepath.Join(root, ".git")) })
	if !hasRow(tree, filepath.Join(root, ".git")) {
		t.Fatal("Show Hidden Files did not reveal ignored VCS metadata")
	}
	if hasRow(tree, filepath.Join(root, "target")) {
		t.Fatal("Show Excluded Files off left target in the tree")
	}
	if hasRow(tree, filepath.Join(root, "generated")) {
		t.Fatal("Show Excluded Files off left a files.exclude match in the tree")
	}
	tree.SetFileFilters([]string{"*.cache"}, []string{"**/generated"})
	tree.SetOptions(treeOptions{showHidden: true, showExcluded: false, compact: false, foldersFirst: true, sortBy: "name", showRootPaths: true})
	q.drain(t, "the hidden File Types match", func() bool { return hasRow(tree, filepath.Join(root, "secret.cache")) })
	if !hasRow(tree, filepath.Join(root, "secret.cache")) {
		t.Fatal("Show Hidden Files did not reveal files.ignoredNames matches")
	}

	tree.reveal(filepath.Join(deep, "App.java"))
	q.drain(t, "the Java package chain", func() bool {
		return hasRow(tree, filepath.Join(root, "src", "main", "java", "com"))
	})
	if hasRow(tree, filepath.Join(root, "src", "main", "java", "com.example")) {
		t.Fatal("Compact Middle Packages off still drew a dotted package row")
	}

	options.compact = true
	options.foldersFirst = false
	options.sortBy = "type"
	tree.SetOptions(options)
	q.drain(t, "the compacted package chain", func() bool {
		return hasRow(tree, filepath.Join(deep, "App.java"))
	})
	var comExample *treeNode
	for _, row := range tree.Rows() {
		if row.name == "com.example" {
			comExample = row
		}
	}
	if comExample == nil || comExample.path != deep {
		t.Fatalf("compact package row = %+v", comExample)
	}
}

func TestProjectTreeOpenChangedScratchAndMembers(t *testing.T) {
	root := t.TempDir()
	openFile := filepath.Join(root, "src", "App.kt")
	changedFile := filepath.Join(root, "README.md")
	if err := os.MkdirAll(filepath.Dir(openFile), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{openFile, changedFile} {
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	scratchRoot := filepath.Join(t.TempDir(), "scratches")
	scratchFile := filepath.Join(scratchRoot, "Scratch.java")
	if err := os.MkdirAll(scratchRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(scratchFile, []byte("class Scratch {}"), 0o644); err != nil {
		t.Fatal(err)
	}

	q := make(mainQueue, 512)
	tree := NewTree(root, q.post, func() {})
	defer tree.Close()
	tree.SetScratchRoot(scratchRoot)
	tree.SetFilteredFiles([]string{openFile, scratchFile}, []string{changedFile})
	tree.SetMode("openFiles")
	q.drain(t, "open-file folders", func() bool {
		return hasRow(tree, openFile) && hasRow(tree, scratchFile)
	})
	if hasRow(tree, changedFile) {
		t.Fatal("Open Files listed an unopened changed file")
	}

	tree.SetMode("changedFiles")
	q.drain(t, "changed files", func() bool { return hasRow(tree, changedFile) })
	if hasRow(tree, openFile) {
		t.Fatal("All Changed Files listed an unchanged open file")
	}

	tree.SetMode("scratches")
	q.drain(t, "the scratches root", func() bool { return hasRow(tree, scratchFile) })
	if hasRow(tree, changedFile) {
		t.Fatal("Scratches pane leaked a project file")
	}

	options := tree.options
	options.showMembers = true
	tree.SetOptions(options)
	tree.SetMode("projectFiles")
	q.drain(t, "the project file", func() bool { return hasRow(tree, openFile) })
	tree.SetOutline(openFile, []openidea.OutlineItem{{
		Name: "App", Kind: "class", SelectionRange: openidea.OutlineRange{},
	}})
	tree.Toggle(openFile)
	foundMember := false
	for _, row := range tree.Rows() {
		if row.member && row.path == openFile && row.name == "App" {
			foundMember = true
		}
	}
	if !foundMember {
		t.Fatal("Show Members did not draw the cached declaration")
	}
}

func TestProjectTreeSortsByModificationTime(t *testing.T) {
	root := t.TempDir()
	older := filepath.Join(root, "older.java")
	newer := filepath.Join(root, "newer.java")
	for _, path := range []string{older, newer} {
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	oldTime := time.Now().Add(-2 * time.Hour)
	newTime := time.Now().Add(-time.Hour)
	if err := os.Chtimes(older, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newer, newTime, newTime); err != nil {
		t.Fatal(err)
	}
	q := make(mainQueue, 128)
	tree := NewTree(root, q.post, func() {})
	defer tree.Close()
	q.drain(t, "the files", func() bool { return hasRow(tree, older) && hasRow(tree, newer) })
	options := tree.options
	options.sortBy = "modified"
	tree.SetOptions(options)
	var order []string
	for _, row := range tree.Rows() {
		if row.depth == 1 && !row.dir {
			order = append(order, row.name)
		}
	}
	if len(order) != 2 || order[0] != "newer.java" {
		t.Fatalf("modified-time order = %v", order)
	}
}
