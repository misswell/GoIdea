package app

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

func TestAccentReachesThemeAndIslands(t *testing.T) {
	m := &Model{prefs: Preferences{Theme: "dark"}}
	m.SetAccentPref("green")
	if m.prefs.Accent != "green" {
		t.Fatalf("accent = %q", m.prefs.Accent)
	}
	want, _ := accentColorOf("green", true)
	if m.theme == nil || m.theme.Accent != want {
		t.Fatalf("theme accent = %+v, want %+v", m.theme, want)
	}
	if got := oiPaletteOf(m.theme).accent; got != want {
		t.Fatalf("islands accent = %+v, want %+v", got, want)
	}
	// Blue is the stored default and clears the override.
	m.SetAccentPref("")
	if m.prefs.Accent != "" {
		t.Fatalf("accent = %q", m.prefs.Accent)
	}
	if m.theme.Accent != uiBlueAccent() {
		t.Fatalf("the default accent did not come back")
	}
}

func uiBlueAccent() ui.Color { return openideaColors(true).accent }

func TestAccentRejectsUnknownIDs(t *testing.T) {
	m := &Model{}
	m.SetAccentPref("magenta")
	if m.prefs.Accent != "" {
		t.Fatalf("an unknown accent was stored: %q", m.prefs.Accent)
	}
}

func TestAccentAndStartupPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := &Model{cfg: Config{SettingsPath: path}}
	m.SetAccentPref("purple")
	m.SetStartupPref("reopen")
	got := LoadPreferences(path)
	if got.Accent != "purple" || got.Startup != "reopen" {
		t.Fatalf("stored = %+v", got)
	}
	m.SetStartupPref("welcome")
	got = LoadPreferences(path)
	if got.Startup != "welcome" {
		t.Fatalf("startup = %q", got.Startup)
	}
}

func TestStartupReopensUnlessWelcomeIsChosen(t *testing.T) {
	m := &Model{}
	if !m.startupReopensProject() {
		t.Fatal("an untouched start does not reopen the last project")
	}
	m.SetStartupPref("welcome")
	if m.startupReopensProject() {
		t.Fatal("the Welcome choice still reopens the last project")
	}
	m.SetStartupPref("nonsense")
	if m.prefs.Startup != "welcome" {
		t.Fatalf("an unknown choice was stored: %q", m.prefs.Startup)
	}
	m.SetStartupPref("reopen")
	if !m.startupReopensProject() {
		t.Fatal("the reopen choice is not read")
	}
}

func TestChooseStartupMatrix(t *testing.T) {
	dir := t.TempDir()
	project := filepath.Join(dir, "project")
	other := filepath.Join(dir, "other")
	explicit := filepath.Join(dir, "explicit")
	for _, path := range []string{project, other, explicit} {
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	inside := filepath.Join(project, "src", "Main.java")
	insideOther := filepath.Join(other, "Other.java")
	loose := filepath.Join(dir, "Loose.java")
	explicitInside := filepath.Join(explicit, "App.java")
	explicitOutside := filepath.Join(project, "Outside.java")
	for _, path := range []string{inside, insideOther, loose, explicitInside, explicitOutside} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("fixture\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	session := Session{
		Root:  project,
		Files: []string{inside, insideOther},
	}

	tests := []struct {
		name          string
		explicitRoot  string
		explicitFiles []string
		session       Session
		reopen        bool
		fresh         bool
		want          startupChoice
	}{
		{
			name:         "explicit root wins",
			explicitRoot: explicit, explicitFiles: []string{explicitInside, explicitOutside},
			session: session, reopen: true,
			want: startupChoice{root: realPath(explicit), files: []string{realPath(explicitInside)}},
		},
		{
			name:          "explicit loose files suppress reopen",
			explicitFiles: []string{inside}, session: session, reopen: true,
			want: startupChoice{looseFiles: []string{realPath(inside)}},
		},
		{
			name:    "reopen disabled stays at welcome",
			session: session, reopen: false,
			want: startupChoice{},
		},
		{
			name:    "fresh session stays at welcome",
			session: session, reopen: true, fresh: true,
			want: startupChoice{},
		},
		{
			name:    "reopen filters files outside root",
			session: session, reopen: true,
			want: startupChoice{root: project, files: normalizeFiles([]string{inside})},
		},
		{
			name:         "relative explicit root is ignored",
			explicitRoot: "project", session: session, reopen: true,
			want: startupChoice{root: project, files: normalizeFiles([]string{inside})},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := chooseStartup(tc.explicitRoot, tc.explicitFiles, tc.session, tc.reopen, tc.fresh)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("choice = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestChooseStartupRestoresRemoteOnlyForSessionReopen(t *testing.T) {
	remote := &openidea.RemoteSession{
		Target: openidea.RemoteTarget{Kind: openidea.RemoteTargetSSH, Config: "dev"},
		Root:   "/srv/app",
	}
	session := Session{Root: "/srv/app", Remote: remote, Files: []string{"/srv/app/Main.java"}}
	got := chooseStartup("", nil, session, true, false)
	if got.remote != remote || got.root != session.Root || !reflect.DeepEqual(got.files, session.Files) {
		t.Fatalf("remote choice = %+v", got)
	}
	got = chooseStartup("", []string{filepath.Join(os.TempDir(), "Loose.java")}, session, true, false)
	if got.remote != nil || got.files != nil || len(got.looseFiles) != 1 {
		t.Fatalf("loose file restored remote session: %+v", got)
	}
}

func TestKeymapPresetLabel(t *testing.T) {
	m := &Model{}
	if keymapPreset() != m.keymapPresetTitle() {
		t.Fatalf("preset = %q", m.keymapPresetTitle())
	}
}
