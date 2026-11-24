package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

type capturedSettingsWrite struct {
	scope string
	key   string
	value string
}

func waitSettingsBackendTest(t *testing.T, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if done() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("settings backend operation did not settle")
}

func TestFileTypesSettingsLoadStageSaveAndRuntimeMatch(t *testing.T) {
	root := t.TempDir()
	var mu sync.Mutex
	var writes []capturedSettingsWrite
	m := &Model{ctx: context.Background(), cfg: Config{SettingsPath: filepath.Join(root, "settings.json")}}
	m.setRoot(root)
	m.postHook = func(fn func()) { fn() }
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		switch command {
		case "languages":
			return json.RawMessage(`[{"id":"go","name":"Go","extensions":["go"],"filenames":[]},{"id":"markdown","name":"Markdown","extensions":["md"],"filenames":["README"]}]`), nil
		case "settings_get":
			return json.RawMessage(`{"files":{"fileTypes":{"go":{"patterns":["*.gotmpl"],"removedPatterns":[],"hashbangs":["gorun"]}},"fileTypeOverrides":{},"associatedApplications":["*.pdf","*.xlsx"],"ignoredNames":[".DS_Store"],"exclude":["**/.git"]}}`), nil
		case "settings_set":
			request := args.(map[string]any)
			mu.Lock()
			writes = append(writes, capturedSettingsWrite{
				scope: request["scope"].(string), key: request["key"].(string), value: request["value"].(string),
			})
			mu.Unlock()
			return json.RawMessage(`null`), nil
		}
		return json.RawMessage(`null`), nil
	}
	m.loadFileTypesSettings()
	waitSettingsBackendTest(t, func() bool { return m.fileTypesSettings.loaded && !m.fileTypesSettings.loading })
	if m.fileTypesSettings.scope != "project" {
		t.Fatalf("scope = %q, want project", m.fileTypesSettings.scope)
	}
	if len(m.fileTypesSettings.languages) != 2 {
		t.Fatalf("languages = %+v", m.fileTypesSettings.languages)
	}
	if !pathMatchesAssociatedApplication("/tmp/report.PDF", m.associatedApplicationPatterns()) {
		t.Fatal("loaded associated-application list did not match a PDF case-insensitively")
	}

	language := m.fileTypesSettings.languages[0]
	fileTypesAddPattern(&m.fileTypesSettings.draft, m.fileTypesSettings.languages, language.ID, "Jenkinsfile")
	m.fileTypesSettings.draft.AssociatedApplications = append(
		m.fileTypesSettings.draft.AssociatedApplications, "*.pages",
	)
	m.fileTypesSettings.draft.IgnoredNames = append(m.fileTypesSettings.draft.IgnoredNames, "*.cache")
	m.fileTypesSettings.draft.Exclude = append(m.fileTypesSettings.draft.Exclude, "**/generated")
	m.fileTypesSettings.overridePath = filepath.Join(root, "src", "notes.txt")
	m.fileTypesSettings.overrideLang = "markdown"
	m.addFileTypeOverride()
	if !m.settingsDirtyPage(settingsPageFileTypes) || !m.settingsPending() {
		t.Fatal("File Types changes were not staged")
	}
	m.backendFormatOnSaveKnown = true
	m.backendFormatOnSave = false
	m.saveBackendCatalogThenJava(false)
	waitSettingsBackendTest(t, func() bool { return !m.settingsBackendCatalogSaving() && !m.fileTypesDirty() })

	mu.Lock()
	defer mu.Unlock()
	byKey := map[string]capturedSettingsWrite{}
	for _, write := range writes {
		byKey[write.key] = write
	}
	fileTypeWrite, ok := byKey["files.fileTypes"]
	if !ok || fileTypeWrite.scope != "project" {
		t.Fatalf("fileTypes write = %+v; all = %+v", fileTypeWrite, writes)
	}
	var associations map[string]fileTypeAssociation
	if err := json.Unmarshal([]byte(fileTypeWrite.value), &associations); err != nil {
		t.Fatal(err)
	}
	if !containsStringFold(associations["go"].Patterns, "Jenkinsfile") {
		t.Fatalf("stored associations = %+v", associations)
	}
	associated, ok := byKey["files.associatedApplications"]
	if !ok || !containsStringFoldFromJSONString(t, associated.value, "*.pages") {
		t.Fatalf("associated applications write = %+v", associated)
	}
	if !pathMatchesAssociatedApplication("/tmp/portfolio.pages", m.associatedApplicationPatterns()) {
		t.Fatal("saved associated applications were not applied to runtime dispatch")
	}
	var override map[string]string
	overrideWrite, ok := byKey["files.fileTypeOverrides"]
	if !ok || json.Unmarshal([]byte(overrideWrite.value), &override) != nil ||
		override["src/notes.txt"] != "markdown" {
		t.Fatalf("file type override write = %+v", overrideWrite)
	}
	if ignored, ok := byKey["files.ignoredNames"]; !ok ||
		!containsStringFoldFromJSONString(t, ignored.value, "*.cache") {
		t.Fatalf("ignored names write = %+v", ignored)
	}
	if exclude, ok := byKey["files.exclude"]; !ok ||
		!containsStringFoldFromJSONString(t, exclude.value, "**/generated") {
		t.Fatalf("exclude write = %+v", exclude)
	}
}

func containsStringFoldFromJSONString(t *testing.T, raw, want string) bool {
	t.Helper()
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		t.Fatal(err)
	}
	return containsStringFold(values, want)
}

func TestPartitionAssociatedPaths(t *testing.T) {
	paths := []string{"a.go", "report.pdf", "sheet.XLSX", "notes.txt"}
	editorPaths, associated := partitionAssociatedPaths(paths, []string{"*.pdf", "*.xlsx"})
	if len(editorPaths) != 2 || editorPaths[0] != "a.go" || editorPaths[1] != "notes.txt" {
		t.Fatalf("editor paths = %v", editorPaths)
	}
	if len(associated) != 2 || associated[0] != "report.pdf" || associated[1] != "sheet.XLSX" {
		t.Fatalf("associated paths = %v", associated)
	}
}

func TestFileTypesResetRestoresApplicationDefaults(t *testing.T) {
	m := &Model{fileTypesSettings: fileTypesSettingsState{
		session: true, loaded: true,
		applied: fileTypesSettings{
			FileTypes:              map[string]fileTypeAssociation{"go": {Patterns: []string{"*.gotmpl"}}},
			FileTypeOverrides:      map[string]string{"notes.txt": "markdown"},
			AssociatedApplications: []string{"*.pdf"}, IgnoredNames: []string{".DS_Store"},
			Exclude: []string{"**/.git"},
		},
	}}
	m.fileTypesSettings.draft = cloneFileTypesSettings(m.fileTypesSettings.applied)
	m.resetFileTypesDraft()
	if len(m.fileTypesSettings.draft.FileTypes) != 0 ||
		len(m.fileTypesSettings.draft.FileTypeOverrides) != 0 ||
		!sameStrings(m.fileTypesSettings.draft.AssociatedApplications, defaultAssociatedApplications) ||
		!sameStrings(m.fileTypesSettings.draft.IgnoredNames, defaultIgnoredNames) ||
		!sameStrings(m.fileTypesSettings.draft.Exclude, defaultFileExcludes) {
		t.Fatalf("Reset File Types = %+v", m.fileTypesSettings.draft)
	}
}

func TestFileTypeOverrideKeysAndRuntimeLanguage(t *testing.T) {
	root := t.TempDir()
	if got := fileTypeOverrideKey(filepath.Join(root, "src", "notes.txt"), root); got != "src/notes.txt" {
		t.Fatalf("project override key = %q", got)
	}
	outside := filepath.Join(t.TempDir(), "notes.txt")
	if got := fileTypeOverrideKey(outside, root); got != filepath.ToSlash(outside) {
		t.Fatalf("absolute override key = %q", got)
	}

	m := &Model{fileTypesSettings: fileTypesSettingsState{
		runtimeLoaded: true,
		runtime: fileTypesSettings{FileTypeOverrides: map[string]string{
			"src/notes.txt": "markdown",
		}},
	}}
	m.setRoot(root)
	if language := m.fileTypeLanguageFor(filepath.Join(root, "src", "notes.txt")); language == nil ||
		language.Name != "Markdown" {
		t.Fatalf("override language = %+v, want Markdown", language)
	}
}

func TestFileTypeOverrideRevertStagesRemovalAndSavesEmptyMap(t *testing.T) {
	root := t.TempDir()
	var mu sync.Mutex
	var writes []capturedSettingsWrite
	m := &Model{ctx: context.Background(), cfg: Config{SettingsPath: filepath.Join(root, "settings.json")}}
	m.setRoot(root)
	m.postHook = func(fn func()) { fn() }
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		if command == "settings_set" {
			request := args.(map[string]any)
			mu.Lock()
			writes = append(writes, capturedSettingsWrite{
				scope: request["scope"].(string), key: request["key"].(string), value: request["value"].(string),
			})
			mu.Unlock()
		}
		return json.RawMessage(`null`), nil
	}
	base := fileTypesSettings{
		FileTypes:              map[string]fileTypeAssociation{},
		FileTypeOverrides:      map[string]string{"src/notes.txt": "markdown"},
		AssociatedApplications: cloneStrings(defaultAssociatedApplications),
		IgnoredNames:           cloneStrings(defaultIgnoredNames),
		Exclude:                cloneStrings(defaultFileExcludes),
	}
	m.fileTypesSettings = fileTypesSettingsState{
		session: true, loaded: true, scope: "project",
		applied: cloneFileTypesSettings(base), draft: cloneFileTypesSettings(base),
	}

	m.removeFileTypeOverride("src/notes.txt")
	if _, ok := m.fileTypesSettings.draft.FileTypeOverrides["src/notes.txt"]; ok {
		t.Fatal("Revert left the override staged")
	}
	if !m.fileTypesDirty() {
		t.Fatal("Revert did not mark File Types dirty")
	}

	m.backendFormatOnSaveKnown = true
	m.backendFormatOnSave = false
	m.saveBackendCatalogThenJava(false)
	waitSettingsBackendTest(t, func() bool { return !m.settingsBackendCatalogSaving() && !m.fileTypesDirty() })

	mu.Lock()
	gotWrites := append([]capturedSettingsWrite(nil), writes...)
	mu.Unlock()
	var overrideWrite *capturedSettingsWrite
	for i := range gotWrites {
		if gotWrites[i].scope == "project" && gotWrites[i].key == "files.fileTypeOverrides" {
			overrideWrite = &gotWrites[i]
			break
		}
	}
	if overrideWrite == nil {
		t.Fatalf("Revert did not save files.fileTypeOverrides: %+v", gotWrites)
	}
	var overrides map[string]string
	if err := json.Unmarshal([]byte(overrideWrite.value), &overrides); err != nil {
		t.Fatal(err)
	}
	if len(overrides) != 0 {
		t.Fatalf("saved overrides = %+v, want an empty map", overrides)
	}
}

func TestSpellingSettingsSaveGlobalAndProjectLayers(t *testing.T) {
	var mu sync.Mutex
	var writes []capturedSettingsWrite
	m := &Model{ctx: context.Background(), cfg: Config{SettingsPath: filepath.Join(t.TempDir(), "settings.json")}}
	m.setRoot(t.TempDir())
	m.postHook = func(fn func()) { fn() }
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		if command == "settings_set" {
			request := args.(map[string]any)
			mu.Lock()
			writes = append(writes, capturedSettingsWrite{
				scope: request["scope"].(string), key: request["key"].(string), value: request["value"].(string),
			})
			mu.Unlock()
		}
		return json.RawMessage(`null`), nil
	}
	base := spellingSettings{IDEWords: []string{"goidea"}, ProjectWords: []string{"mayoi"}, CustomDictionaries: []string{"team.dic"}}
	m.spellingSettings = spellingSettingsState{
		session: true, loaded: true, globalApplied: base, globalDraft: cloneSpellingSettings(base),
		projectApplied: base, projectDraft: cloneSpellingSettings(base),
	}
	m.spellingSettings.globalDraft.IDEWords = append(m.spellingSettings.globalDraft.IDEWords, "openidea")
	m.spellingSettings.projectDraft.ProjectWords = append(m.spellingSettings.projectDraft.ProjectWords, "qwen")
	m.backendFormatOnSaveKnown = true
	m.backendFormatOnSave = false
	m.saveBackendCatalogThenJava(false)
	waitSettingsBackendTest(t, func() bool { return !m.settingsBackendCatalogSaving() && !m.spellingDirty() })

	mu.Lock()
	defer mu.Unlock()
	got := map[string]string{}
	for _, write := range writes {
		got[write.scope+":"+write.key] = write.value
	}
	if got["global:spelling.ideWords"] == "" || got["project:spelling.projectWords"] == "" {
		t.Fatalf("spelling writes = %+v", got)
	}
}

func TestFormatterSettingsSaveNestedLanguageKeys(t *testing.T) {
	var mu sync.Mutex
	var writes []capturedSettingsWrite
	m := &Model{ctx: context.Background(), cfg: Config{SettingsPath: filepath.Join(t.TempDir(), "settings.json")}}
	m.setRoot(t.TempDir())
	m.postHook = func(fn func()) { fn() }
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		if command == "settings_set" {
			request := args.(map[string]any)
			mu.Lock()
			writes = append(writes, capturedSettingsWrite{
				scope: request["scope"].(string), key: request["key"].(string), value: request["value"].(string),
			})
			mu.Unlock()
		}
		return json.RawMessage(`null`), nil
	}
	base := formatterDefaultSettings("prettier")
	draft := cloneFormatterSettings(base)
	draft.Prefer = true
	draft.RunOnSave = true
	draft.Languages = []string{"markdown", "typescript"}
	m.formatterSettings = formatterSettingsState{
		session: true, loaded: true, page: settingsPagePrettier, tool: "prettier",
		scope: "project", applied: base, draft: draft,
	}
	m.backendFormatOnSaveKnown = true
	m.backendFormatOnSave = false
	m.saveBackendCatalogThenJava(false)
	waitSettingsBackendTest(t, func() bool { return !m.settingsBackendCatalogSaving() && !m.formatterDirty() })

	mu.Lock()
	defer mu.Unlock()
	keys := map[string]string{}
	for _, write := range writes {
		keys[write.key] = write.value
	}
	for _, key := range []string{
		"languages.prettier.languages", "languages.prettier.prefer", "languages.prettier.runOnSave",
	} {
		if keys[key] == "" {
			t.Fatalf("missing %s in %+v", key, writes)
		}
	}
}

func TestFormatterDraftsSurvivePageNavigation(t *testing.T) {
	base := formatterDefaultSettings("prettier")
	draft := cloneFormatterSettings(base)
	draft.RunOnSave = true
	m := &Model{formatterSettings: formatterSettingsState{
		session: true, loaded: true, page: settingsPagePrettier, tool: "prettier", scope: "project",
		applied: base, draft: draft,
	}}
	if !m.formatterDirty() {
		t.Fatal("Prettier draft was not dirty")
	}
	m.loadFormatterSettings(settingsPageOxfmt)
	m.loadFormatterSettings(settingsPagePrettier)
	if m.formatterSettings.page != settingsPagePrettier || !m.formatterSettings.draft.RunOnSave || !m.formatterDirty() {
		t.Fatalf("navigating back lost the staged Prettier draft: %+v", m.formatterSettings)
	}
}

func TestInspectionsSettingsSaveSeverityAndOptions(t *testing.T) {
	var mu sync.Mutex
	var writes []capturedSettingsWrite
	m := &Model{ctx: context.Background(), cfg: Config{SettingsPath: filepath.Join(t.TempDir(), "settings.json")}}
	m.setRoot(t.TempDir())
	m.postHook = func(fn func()) { fn() }
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		if command == "settings_set" {
			request := args.(map[string]any)
			mu.Lock()
			writes = append(writes, capturedSettingsWrite{
				scope: request["scope"].(string), key: request["key"].(string), value: request["value"].(string),
			})
			mu.Unlock()
		}
		return json.RawMessage(`null`), nil
	}
	base := inspectionsSettings{Profile: "default", Default: map[string]inspectionOverrideSettings{},
		Project: map[string]inspectionOverrideSettings{}, IDEProfiles: map[string]customInspectionProfile{},
		ProjectProfiles: map[string]customInspectionProfile{}}
	draft := cloneInspectionSettings(base)
	draft.Profile = "project"
	draft.Project["builtin:spelling"] = inspectionOverrideSettings{
		Severity: "error", Options: map[string]bool{"processCode": false},
	}
	m.inspectionsSettings = inspectionsSettingsState{
		session: true, loaded: true, globalApplied: cloneInspectionSettings(base),
		globalDraft: cloneInspectionSettings(base), projectApplied: cloneInspectionSettings(base), projectDraft: draft,
	}
	m.backendFormatOnSaveKnown = true
	m.backendFormatOnSave = false
	m.saveBackendCatalogThenJava(false)
	waitSettingsBackendTest(t, func() bool { return !m.settingsBackendCatalogSaving() && !m.inspectionsDirty() })

	mu.Lock()
	defer mu.Unlock()
	got := map[string]capturedSettingsWrite{}
	for _, write := range writes {
		got[write.key] = write
	}
	profile, ok := got["inspections.profile"]
	if !ok || profile.scope != "project" {
		t.Fatalf("profile write = %+v in %+v", profile, writes)
	}
	project, ok := got["inspections.project"]
	var overrides map[string]inspectionOverrideSettings
	if ok {
		err := json.Unmarshal([]byte(project.value), &overrides)
		if err != nil {
			t.Fatal(err)
		}
	}
	if !ok || project.scope != "project" || overrides["builtin:spelling"].Severity != "error" ||
		overrides["builtin:spelling"].Options["processCode"] {
		t.Fatalf("project override write = %+v", project)
	}
}

func TestInspectionProfileCopyRenameDelete(t *testing.T) {
	m := &Model{}
	m.setRoot(t.TempDir())
	baseDefault := map[string]inspectionOverrideSettings{
		"builtin:spelling": {Severity: "warning"},
	}
	m.inspectionsSettings = inspectionsSettingsState{
		session: true, loaded: true,
		globalApplied: inspectionsSettings{
			Profile: "default", Default: cloneInspectionOverrides(baseDefault),
			IDEProfiles: map[string]customInspectionProfile{},
		},
		globalDraft: inspectionsSettings{
			Profile: "default", Default: cloneInspectionOverrides(baseDefault),
			IDEProfiles: map[string]customInspectionProfile{},
		},
		projectApplied: inspectionsSettings{
			Profile: "default", Project: map[string]inspectionOverrideSettings{},
			ProjectProfiles: map[string]customInspectionProfile{},
		},
		projectDraft: inspectionsSettings{
			Profile: "default", Project: map[string]inspectionOverrideSettings{},
			ProjectProfiles: map[string]customInspectionProfile{},
		},
	}

	if err := m.copyInspectionProfile("default", "Strict", "ide"); err != nil {
		t.Fatal(err)
	}
	ideProfile := m.activeInspectionProfile()
	if ideProfile != "ide:strict" ||
		m.inspectionsSettings.globalDraft.IDEProfiles["strict"].Overrides["builtin:spelling"].Severity != "warning" {
		t.Fatalf("IDE copy = %+v", m.inspectionsSettings.globalDraft.IDEProfiles)
	}
	if err := m.renameInspectionProfile(ideProfile, "  Team strict  "); err != nil {
		t.Fatal(err)
	}
	if got := m.inspectionsSettings.globalDraft.IDEProfiles["strict"].Name; got != "Team strict" {
		t.Fatalf("renamed profile = %q", got)
	}
	if err := m.copyInspectionProfile(ideProfile, "Team strict", "project"); err != nil {
		t.Fatal(err)
	}
	projectProfile := m.activeInspectionProfile()
	if projectProfile != "project:team-strict" ||
		m.inspectionsSettings.projectDraft.ProjectProfiles["team-strict"].Overrides["builtin:spelling"].Severity != "warning" {
		t.Fatalf("project copy = %+v", m.inspectionsSettings.projectDraft.ProjectProfiles)
	}
	if err := m.copyInspectionProfile(ideProfile, "Team strict", "ide"); err == nil {
		t.Fatal("copy accepted a profile name already used at IDE level")
	}
	if err := m.deleteInspectionProfile(projectProfile); err != nil {
		t.Fatal(err)
	}
	if m.activeInspectionProfile() != "project" {
		t.Fatalf("delete active project profile selected %q, want project default", m.activeInspectionProfile())
	}
	if err := m.deleteInspectionProfile("default"); err == nil {
		t.Fatal("default profile was deletable")
	}
	if err := m.renameInspectionProfile("project", "Renamed default"); err == nil {
		t.Fatal("project default profile was renameable")
	}
}

func TestInspectionProfileEntryOrdering(t *testing.T) {
	settings := inspectionsSettings{
		IDEProfiles: map[string]customInspectionProfile{
			"zeta":  {Name: "zeta"},
			"alpha": {Name: "alpha"},
			"beta":  {Name: "Beta"},
		},
		ProjectProfiles: map[string]customInspectionProfile{
			"zeta":  {Name: "Zeta"},
			"alpha": {Name: "alpha"},
			"beta":  {Name: "Beta"},
		},
	}
	want := []string{"Project Default", "alpha", "Beta", "Zeta", "Default", "alpha", "Beta", "zeta"}
	for attempt := 0; attempt < 20; attempt++ {
		entries := inspectionProfileEntries(settings, true)
		labels := make([]string, len(entries))
		for i, entry := range entries {
			labels[i] = entry.Label
		}
		if !sameStrings(labels, want) {
			t.Fatalf("profile order = %v, want %v", labels, want)
		}
	}
}

func TestInspectionResetClearsProfilePrompt(t *testing.T) {
	m := &Model{inspectionsSettings: inspectionsSettingsState{
		profilePrompt: &inspectionProfilePrompt{Mode: "rename", Level: "ide", Profile: "ide:strict"},
	}}
	m.resetInspectionsDraft()
	if m.inspectionsSettings.profilePrompt != nil {
		t.Fatal("Reset left the inspection profile prompt open")
	}
}

func TestBackendCatalogPagesRenderOpenIDEAControls(t *testing.T) {
	m := &Model{}
	m.fileTypesSettings = fileTypesSettingsState{
		session: true, loaded: true, scope: "project",
		applied:   fileTypesSettings{FileTypes: map[string]fileTypeAssociation{}, FileTypeOverrides: map[string]string{}, AssociatedApplications: []string{"*.pdf"}},
		languages: []openIDEALanguageInfo{{ID: "go", Name: "Go", Extensions: []string{"go"}}},
		selected:  "go",
	}
	m.fileTypesSettings.draft = cloneFileTypesSettings(m.fileTypesSettings.applied)
	m.spellingSettings = spellingSettingsState{
		session: true, loaded: true,
		globalApplied: spellingSettings{}, globalDraft: spellingSettings{},
		projectApplied: spellingSettings{}, projectDraft: spellingSettings{},
	}
	m.inspectionsSettings = inspectionsSettingsState{
		session: true, loaded: true,
		globalApplied:  inspectionsSettings{Profile: "default", Default: map[string]inspectionOverrideSettings{}, IDEProfiles: map[string]customInspectionProfile{}},
		globalDraft:    inspectionsSettings{Profile: "default", Default: map[string]inspectionOverrideSettings{}, IDEProfiles: map[string]customInspectionProfile{}},
		projectApplied: inspectionsSettings{Profile: "default", Project: map[string]inspectionOverrideSettings{}, ProjectProfiles: map[string]customInspectionProfile{}},
		projectDraft:   inspectionsSettings{Profile: "default", Project: map[string]inspectionOverrideSettings{}, ProjectProfiles: map[string]customInspectionProfile{}},
	}
	cases := []struct {
		name string
		body func(*ui.Context)
		want []string
	}{
		{"file types", func(c *ui.Context) { m.settingsFileTypesBody(c, oiPaletteOf(c.Theme())) }, []string{"Recognized File Types", "File name patterns", "Hashbang patterns", "Files opened in associated applications", "Override File Type", "Ignored Files and Folders", "Exclude patterns"}},
		{"inspections", func(c *ui.Context) { m.settingsInspectionsBody(c, oiPaletteOf(c.Theme())) }, []string{"Profile", "Profile Actions", "Search inspections", "Typo", "Severity"}},
		{"spelling", func(c *ui.Context) { m.settingsSpellingBody(c, oiPaletteOf(c.Theme())) }, []string{"Accepted words", "Application dictionary", "Custom dictionaries"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tester := ui.NewTester(tc.body, 900, 620)
			tester.Frame()
			for _, want := range tc.want {
				if !tester.HasText(want) {
					t.Fatalf("%s page lacks %q: %q", tc.name, want, tester.Texts())
				}
			}
		})
	}
}
