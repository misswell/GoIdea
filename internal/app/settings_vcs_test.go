package app

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestVCSSettingsPagesMatchOpenIDEATreeAndSearch(t *testing.T) {
	tree := settingsTree()
	want := []string{
		settingsPageVCSCommit,
		settingsPageVCSConfirmation,
		settingsPageVCSMappings,
		settingsPageIssueNavigation,
		settingsPageVCSLog,
		settingsPageVCSShelf,
		settingsPageVCSGit,
		settingsPageVCSGitHub,
	}
	node, ok := settingsFindNode(tree, settingsPageVCS)
	if !ok {
		t.Fatal("Version Control page is not registered")
	}
	got := make([]string, len(node.children))
	for i := range node.children {
		got[i] = node.children[i].key
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Version Control children = %v, want %v", got, want)
	}
	for _, page := range want {
		breadcrumb := settingsBreadcrumb(tree, page)
		wantLength := 2
		if page == settingsPageVCS {
			wantLength = 1
		}
		if len(breadcrumb) != wantLength || breadcrumb[0] != "Version Control" {
			t.Fatalf("%s breadcrumb = %v", page, breadcrumb)
		}
	}
	for query, page := range map[string]string{
		"modal commit":       settingsPageVCSCommit,
		"patch reveal":       settingsPageVCSConfirmation,
		"vcs root":           settingsPageVCSMappings,
		"commit checks":      settingsPageVCSCommit,
		"references columns": settingsPageVCSLog,
		"mark viewed":        settingsPageVCSGitHub,
	} {
		rows := settingsFilterTree(tree, query, nil)
		found := false
		for _, row := range settingsFlattenAll(rows) {
			if row.key == page {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("search %q did not find %s", query, page)
		}
	}
}

func TestVCSSettingsCustomPagesRenderAndPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := &Model{ctx: context.Background(), cfg: Config{SettingsPath: path}}
	m.applyPreferences()

	overview := ui.NewTester(func(c *ui.Context) {
		m.settingsVCSOverviewBody(c, oiPaletteOf(c.Theme()))
	}, 900, 700)
	overview.Frame()
	for _, text := range []string{
		"Version Control", "Commit", "Confirmation", "Directory Mappings",
		"Issue Navigation", "Log", "Shelf", "Git", "GitHub",
	} {
		if !overview.HasText(text) {
			t.Fatalf("Version Control overview lacks %q: %q", text, overview.Texts())
		}
	}

	logPage := ui.NewTester(func(c *ui.Context) {
		m.settingsVCSLogBody(c, oiPaletteOf(c.Theme()))
	}, 900, 700)
	logPage.Frame()
	for _, text := range []string{
		"Show only the first reference", "Show tag names",
		"Show commit time rather than author time", "Show references on the left",
		"Show lines added and deleted", "Visible columns", "Author", "Hash", "Date",
		"GitHub Commit Checks",
	} {
		if !logPage.HasText(text) {
			t.Fatalf("Log settings page lacks %q: %q", text, logPage.Texts())
		}
	}
	if err := logPage.Click("Hash"); err != nil {
		t.Fatal(err)
	}
	if !prefBool(LoadPreferences(path).VCS.Log.ShowHash, false) {
		t.Fatal("Hash column choice was not saved")
	}

	githubPage := ui.NewTester(func(c *ui.Context) {
		m.settingsVCSGitHubBody(c, oiPaletteOf(c.Theme()))
	}, 900, 300)
	githubPage.Frame()
	if !githubPage.HasText("Automatically mark opened files as viewed") {
		t.Fatalf("GitHub settings page lacks its option: %q", githubPage.Texts())
	}
	if err := githubPage.Click("Automatically mark opened files as viewed"); err != nil {
		t.Fatal(err)
	}
	if !LoadPreferences(path).VCS.GitHub.AutoMarkViewed {
		t.Fatal("GitHub AutoMarkViewed choice was not saved")
	}
}

func TestVCSSettingsMappingsRenderNoProjectNonGitAndGit(t *testing.T) {
	m := &Model{}
	render := func() *ui.Tester {
		tester := ui.NewTester(func(c *ui.Context) {
			m.settingsVCSMappingsBody(c, oiPaletteOf(c.Theme()))
		}, 900, 400)
		tester.Frame()
		return tester
	}
	if tester := render(); !tester.HasText("No project is open") {
		t.Fatalf("empty mappings = %q", tester.Texts())
	}

	root := t.TempDir()
	m.setRoot(root)
	if tester := render(); !tester.HasText(root) || !tester.HasText("<none>") {
		t.Fatalf("non-Git mappings = %q", tester.Texts())
	}
	m.vcs.loaded = true
	if tester := render(); !tester.HasText(root) || !tester.HasText("Git") {
		t.Fatalf("Git mappings = %q", tester.Texts())
	}
}

func TestVCSNativePagesHaveOpenIDEADefaults(t *testing.T) {
	for _, page := range []string{
		settingsPageVCSCommit,
		settingsPageVCSConfirmation,
		settingsPageVCSGit,
		settingsPageVCSShelf,
	} {
		def, ok := nativeSettingsDefinition(page)
		if !ok {
			t.Fatalf("%s is not a native settings page", page)
		}
		if def.page.title == "" || !def.projectOnly || def.defaultScope != "project" {
			t.Fatalf("%s definition = %+v", page, def)
		}
		if len(def.sections) == 0 || len(def.sections[0].fields) == 0 {
			t.Fatalf("%s has no native controls", page)
		}
	}
	defaults := map[string]any{}
	for _, page := range []string{settingsPageVCSCommit, settingsPageVCSConfirmation, settingsPageVCSGit, settingsPageVCSShelf} {
		def, _ := nativeSettingsDefinition(page)
		for key, value := range nativeTestDefaults(def) {
			defaults[key] = value
		}
	}
	for key, want := range map[string]any{
		"vcs.commit.nonModal":             true,
		"vcs.commit.subjectLimit":         float64(72),
		"vcs.confirmation.onCreate":       "ask",
		"vcs.confirmation.onPatchCreated": "ask",
		"vcs.git.updateMethod":            "merge",
		"vcs.git.protectedBranches":       "master; main",
		"vcs.git.showPushDialog":          true,
		"vcs.shelf.removeApplied":         false,
	} {
		if got := defaults[key]; got != want {
			t.Errorf("%s default = %#v, want %#v", key, got, want)
		}
	}
}
