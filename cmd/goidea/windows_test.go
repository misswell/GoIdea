package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mayoi/internal/app"
)

func TestSecondaryProjectWindowsShareTheBackendDataRoot(t *testing.T) {
	base := app.Config{
		Home:         filepath.Join(t.TempDir(), "openidea-server"),
		SessionPath:  "/shared/session.json",
		SettingsPath: "/shared/settings.json",
		DataDir:      "/shared",
	}
	firstDir := filepath.Join(base.DataDir, "windows", "window-2")
	secondDir := filepath.Join(base.DataDir, "windows", "window-3")
	first := secondaryProjectWindowConfig(base, firstDir, true)
	second := secondaryProjectWindowConfig(base, secondDir, true)

	if first.Home != base.Home || second.Home != base.Home {
		t.Fatalf("backend homes = %q and %q, want shared %q", first.Home, second.Home, base.Home)
	}
	if first.SettingsPath != base.SettingsPath || second.SettingsPath != base.SettingsPath {
		t.Fatalf("settings paths = %q and %q, want shared %q", first.SettingsPath, second.SettingsPath, base.SettingsPath)
	}
	if first.SessionPath == second.SessionPath || first.SessionPath == base.SessionPath {
		t.Fatalf("session paths are not isolated: %q, %q, base %q", first.SessionPath, second.SessionPath, base.SessionPath)
	}
}

func TestLegacyProjectWindowToolsMoveToTheSharedDataRoot(t *testing.T) {
	base := t.TempDir()
	sharedHome := filepath.Join(base, "openidea-server")
	legacyHome := filepath.Join(base, "windows", "window-2", "openidea-server")
	legacyTool := filepath.Join(legacyHome, "data", "tools", "jdtls", "config_linux")
	legacyJDK := filepath.Join(legacyHome, "data", "jdks", "temurin-21", "bin", "java")
	for path, contents := range map[string]string{
		legacyTool: "jdtls",
		legacyJDK:  "jdk",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := migrateLegacyWindowToolData(base, sharedHome); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		filepath.Join(sharedHome, "data", "tools", "jdtls", "config_linux"):    "jdtls",
		filepath.Join(sharedHome, "data", "jdks", "temurin-21", "bin", "java"): "jdk",
	} {
		got, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read migrated %s: %v", name, err)
		}
		if string(got) != want {
			t.Fatalf("migrated %s = %q, want %q", name, got, want)
		}
	}
	if _, err := os.Stat(filepath.Join(legacyHome, "data", "tools", "jdtls", "config_linux")); !os.IsNotExist(err) {
		t.Fatalf("legacy tool was not moved: %v", err)
	}
}

func TestLegacyToolMigrationPreservesSharedInstallations(t *testing.T) {
	base := t.TempDir()
	sharedHome := filepath.Join(base, "openidea-server")
	legacyHome := filepath.Join(base, "windows", "window-3", "openidea-server")
	sharedTool := filepath.Join(sharedHome, "data", "tools", "jdtls", "current")
	legacyExtra := filepath.Join(legacyHome, "data", "tools", "jdtls", "legacy-extra")
	for path, contents := range map[string]string{
		sharedTool:  "new",
		legacyExtra: "old",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := migrateLegacyWindowToolData(base, sharedHome); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(sharedTool); err != nil || string(got) != "new" {
		t.Fatalf("shared installation = %q, %v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(sharedHome, "data", "tools", "jdtls", "legacy-extra")); err != nil || string(got) != "old" {
		t.Fatalf("legacy extra = %q, %v", got, err)
	}
}

func TestGoideaWindowUsesOpenIDEAChrome(t *testing.T) {
	options := goideaWindowOptions("main", true, nil)
	if options.TitleBarStyle != mygo.TitleBarHidden || options.TitleBarHeight != 40 {
		t.Fatalf("window title bar = %q height %d", options.TitleBarStyle, options.TitleBarHeight)
	}
	if options.TrafficLightPosition == nil ||
		options.TrafficLightPosition.X != 14 || options.TrafficLightPosition.Y != 13 {
		t.Fatalf("traffic light position = %+v", options.TrafficLightPosition)
	}
	if options.Title != "GoIDEA" || options.Width != 1280 || options.Height != 820 {
		t.Fatalf("window defaults = %+v", options)
	}
}

func TestAuxiliaryDiffWindowIsTransientChildSurface(t *testing.T) {
	parent := &mygo.Window{}
	options := auxiliaryDiffWindowOptions(
		"Diff – A.java",
		parent,
		ui.View(func(*ui.Context) {}),
	)
	if options.Parent != parent || options.Title != "Diff – A.java" {
		t.Fatalf("auxiliary diff owner/title = %p %q", options.Parent, options.Title)
	}
	if options.Width != 1080 || options.Height != 720 ||
		options.MinWidth != 520 || options.MinHeight != 320 {
		t.Fatalf("auxiliary diff bounds = %dx%d min %dx%d",
			options.Width, options.Height, options.MinWidth, options.MinHeight)
	}
	if options.TitleBarStyle != mygo.TitleBarHidden || options.TrafficLightPosition == nil ||
		options.Content == nil {
		t.Fatalf("auxiliary diff chrome/content = %+v", options)
	}
	if !strings.Contains(options.Title, filepath.Base("A.java")) {
		t.Fatalf("auxiliary diff title lost the file: %q", options.Title)
	}
}

func TestWorkspaceWindowsOnlyIncludesTheModelsGroup(t *testing.T) {
	first, second, other, standalone := &app.Model{}, &app.Model{}, &app.Model{}, &app.Model{}
	host := &nativeWindowHost{
		windows: []*nativeProjectWindow{
			{id: "first", group: 1, model: first},
			{id: "second", group: 1, model: second},
			{id: "other", group: 2, model: other},
			{id: "standalone", model: standalone},
		},
		selectedWindow: map[int]string{1: "first", 2: "other"},
	}
	got := host.WorkspaceWindows(second)
	if len(got) != 2 || got[0].ID != "first" || !got[0].Selected || got[1].ID != "second" || got[1].Selected {
		t.Fatalf("group windows = %+v", got)
	}
	if got := host.WorkspaceWindows(standalone); len(got) != 0 {
		t.Fatalf("standalone window has group tabs: %+v", got)
	}
}

func TestWindowGroupActionsOnNativeHost(t *testing.T) {
	h := &nativeWindowHost{headless: true, selectedWindow: map[int]string{1: "b"}}
	for _, id := range []string{"a", "outside", "b", "c"} {
		m := &app.Model{}
		m.SetWorkspaceHost(h)
		g := 1
		if id == "outside" {
			g = 0
		}
		h.windows = append(h.windows, &nativeProjectWindow{id: id, group: g, model: m})
	}
	b := h.windowByID("b")
	h.active = b.model
	order := func() []string {
		var ids []string
		for _, w := range h.windows {
			ids = append(ids, w.id)
		}
		return ids
	}
	if !b.model.RunActionWithArg("window.reorderTabs", []any{"c", "a", "b"}) || !slices.Equal(order(), []string{"c", "outside", "a", "b"}) {
		t.Fatalf("group reorder changed the wrong slots: %v", order())
	}
	for _, bad := range []any{[]string{"outside", "a", "b"}, []string{"c", "a", "a"}, []string{"b"}, []any{"a", 7, "b", "c"}} {
		b.model.RunActionWithArg("window.reorderTabs", bad)
		if !slices.Equal(order(), []string{"c", "outside", "a", "b"}) {
			t.Fatalf("accepted invalid order %v", bad)
		}
	}
	b.model.RunActionWithArg("window.selectTab", "outside")
	if h.active != b.model {
		t.Fatal("selected another group")
	}
	b.model.RunAction("window.nextTab")
	if h.active != h.windowByID("c").model {
		t.Fatal("cycle did not wrap in group order")
	}
	h.active.RunAction("window.previousTab")
	if h.active != b.model {
		t.Fatal("reverse cycle failed")
	}
	b.model.RunActionWithArg("window.selectTab", "a")
	a := h.windowByID("a")
	if h.active != a.model {
		t.Fatal("did not select the project tab")
	}
	if !a.model.RunAction("window.detachTab") || a.group != 0 || h.active != a.model || len(h.windows) != 4 || len(h.groupedWindows(1)) != 2 {
		t.Fatal("detach did not preserve the original native windows")
	}
	if a.model.ActionEnabled("window.closeTab") || a.model.ActionEnabled("window.detachTab") {
		t.Fatal("standalone project still has group actions")
	}
}

func TestProjectWindowAtResolvesSymlinkedRoot(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "project")
	link := filepath.Join(dir, "project-link")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(root, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	model := app.New(app.Config{DataDir: t.TempDir(), FreshSession: true})
	t.Cleanup(model.QuitImmediately)
	model.SetRootForTest(root)
	entry := &nativeProjectWindow{id: "main", model: model, win: &mygo.Window{}}
	host := &nativeWindowHost{windows: []*nativeProjectWindow{entry}}
	if got := host.projectWindowAt(link); got != entry {
		t.Fatalf("symlink resolved to %+v, want %+v", got, entry)
	}
	if got := host.projectWindowAt(filepath.Join(dir, "missing")); got != nil {
		t.Fatalf("missing project resolved to %+v", got)
	}
}

func TestCanonicalProjectPathIsAbsoluteAndStable(t *testing.T) {
	root := t.TempDir()
	got := canonicalProjectPath(filepath.Join(root, ".", "child", ".."))
	if got != canonicalProjectPath(root) || !filepath.IsAbs(got) {
		t.Fatalf("canonical path = %q from %q", got, root)
	}
}
