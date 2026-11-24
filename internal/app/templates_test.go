package app

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
	"mayoi/internal/position"
)

func TestExpandTemplateFieldsAndCaret(t *testing.T) {
	got := expandTemplate("if $COND$ {\n\t$COND$\n}\n$END$", liveTemplateContext{})
	if !strings.Contains(got.Text, "if Cond {") {
		t.Fatalf("expanded text = %q", got.Text)
	}
	if got.Caret != len(got.Text) {
		t.Fatalf("caret = %d, len = %d", got.Caret, len(got.Text))
	}
	if len(got.Fields) != 2 || got.Fields[0].Name != "COND" {
		t.Fatalf("fields = %+v", got.Fields)
	}
}

func TestExpandEmmetNestedMultiplication(t *testing.T) {
	got, ok := expandEmmet("ul>li.item*2")
	if !ok {
		t.Fatal("expandEmmet rejected abbreviation")
	}
	want := "<ul>\n  <li class=\"item\"></li>\n  <li class=\"item\"></li>\n</ul>"
	if got != want {
		t.Fatalf("expanded = %q, want %q", got, want)
	}
}

func TestUpdateEmmetTagMergesAttributes(t *testing.T) {
	got, ok := emmetUpdateTag(`<section class="a" id="x" data-z="q">text</section>`, `div.b#y[data-k=v]`)
	if !ok {
		t.Fatal("emmetUpdateTag rejected tag")
	}
	for _, value := range []string{`<div`, `class="a b"`, `id="y"`, `data-k="v"`, `data-z="q"`, `</div>`} {
		if !strings.Contains(got, value) {
			t.Fatalf("updated tag %q lacks %q", got, value)
		}
	}
}

func TestPostfixCandidatesOfferOperandAndExpression(t *testing.T) {
	prefs := defaultTemplatePrefs().Postfix
	got := postfixExpansionCandidates("x > y.not", len("x > y.not"), "go", prefs, liveTemplateContext{})
	if len(got) != 2 {
		t.Fatalf("candidates = %+v", got)
	}
	if got[0].Expression != "y" || got[1].Expression != "x > y" {
		t.Fatalf("expressions = %q, %q", got[0].Expression, got[1].Expression)
	}
	if got[1].Text.Text != "!((x > y))" && got[1].Text.Text != "!(x > y)" {
		t.Fatalf("wide not expansion = %q", got[1].Text.Text)
	}
}

func TestTemplatePrefsRoundTripAndProjectSeed(t *testing.T) {
	dir := t.TempDir()
	prefs := defaultTemplatePrefs()
	prefs.Live.Custom = append(prefs.Live.Custom, LiveTemplate{
		ID: "live.custom", Abbreviation: "hello", Name: "Hello", Body: `println("$END$")`,
	})
	root := filepath.Join(dir, "project")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	project := prefs.Files.ensureProjectScheme(root)
	project.Templates[0].name = "Project File"
	prefs.Files.setProjectScheme(root, project)
	m := &Model{cfg: Config{SettingsPath: filepath.Join(dir, "settings.json")}}
	if err := m.saveTemplates(prefs); err != nil {
		t.Fatal(err)
	}
	loaded := loadTemplatePrefs(m.templateRestorePath())
	if len(loaded.Live.Custom) != 1 || loaded.Live.Custom[0].Abbreviation != "hello" {
		t.Fatalf("live custom = %+v", loaded.Live.Custom)
	}
	got := loaded.Files.effectiveScheme(root)
	if got.Templates[0].name != "Project File" {
		t.Fatalf("project seed = %+v", got.Templates[0])
	}
	if !reflect.DeepEqual(loaded.Files.Schemes["Default"].Includes, defaultFileTemplateScheme().Includes) {
		t.Fatal("default includes changed while editing project scheme")
	}
}

func TestTemplateAbbreviationsHonorCaseSensitivity(t *testing.T) {
	prefs := defaultTemplatePrefs()
	prefs.Live.Custom = append(prefs.Live.Custom, LiveTemplate{
		ID: "live.upper", Abbreviation: "UP", Name: "Upper", Body: "upper()",
	})
	doc := docbridge.Virtual("/example.java", "")
	m := &Model{
		templatePrefs: &prefs,
		tabs:          []*Tab{{doc: doc, state: &editor.State{Buffer: doc.Buffer()}}},
	}
	req := editor.CompletionRequest{Offset: 1, Prefix: "u", Version: doc.Buffer().Version()}
	_, items, _ := m.liveTemplateCompletionItems(m.tab(), req)
	if len(items) != 1 || items[0].Label != "UP" {
		t.Fatalf("case-insensitive completions = %+v", items)
	}
	prefs.Live.CaseSensitive = true
	_, items, _ = m.liveTemplateCompletionItems(m.tab(), req)
	if len(items) != 0 {
		t.Fatalf("case-sensitive completions = %+v", items)
	}
	state := &templateChooserState{
		query: "u", caseSensitive: false,
		templates: []LiveTemplate{{Abbreviation: "UP"}},
	}
	if got := templateChooserMatches(state); len(got) != 1 {
		t.Fatalf("case-insensitive chooser = %+v", got)
	}
	state.caseSensitive = true
	if got := templateChooserMatches(state); len(got) != 0 {
		t.Fatalf("case-sensitive chooser = %+v", got)
	}
}

func TestTemplateGlobalAndIndividualSwitches(t *testing.T) {
	prefs := defaultTemplatePrefs()
	prefs.Live.Custom = append(prefs.Live.Custom, LiveTemplate{
		ID: "live.hello", Abbreviation: "hello", Name: "Hello", Body: "hello()",
	})
	doc := docbridge.Virtual("/example.java", "h")
	m := &Model{
		templatePrefs: &prefs,
		tabs:          []*Tab{{doc: doc, state: &editor.State{Buffer: doc.Buffer()}}},
	}
	req := editor.CompletionRequest{Offset: 1, Prefix: "h", Version: doc.Buffer().Version()}
	if _, items, _ := m.liveTemplateCompletionItems(m.tab(), req); len(items) != 1 {
		t.Fatalf("enabled live templates = %+v", items)
	}
	prefs.Live.Disabled = []string{"live.hello"}
	if _, items, _ := m.liveTemplateCompletionItems(m.tab(), req); len(items) != 0 {
		t.Fatalf("individually disabled live template = %+v", items)
	}
	prefs.Live.Enabled = false
	if _, items, _ := m.liveTemplateCompletionItems(m.tab(), req); len(items) != 0 {
		t.Fatalf("globally disabled live templates = %+v", items)
	}

	prefs.Postfix.Enabled = false
	postfixDoc := docbridge.Virtual("/example.java", "value.not")
	postfixTab := &Tab{doc: postfixDoc, state: &editor.State{Buffer: postfixDoc.Buffer()}}
	postfixReq := editor.CompletionRequest{
		Offset: len("value.not"), Prefix: "not", Version: postfixDoc.Buffer().Version(),
	}
	if m.requestPostfixCompletion(postfixTab, postfixReq) {
		t.Fatal("globally disabled postfix completion was requested")
	}

	prefs.Postfix.Enabled = true
	prefs.Postfix.Disabled = []string{"postfix.not"}
	if candidates := postfixExpansionCandidates(
		postfixDoc.Text(), postfixReq.Offset, "java", prefs.Postfix,
		m.templateContext(postfixTab, ""),
	); len(candidates) != 0 {
		t.Fatalf("individually disabled postfix template = %+v", candidates)
	}
}

func TestTabAcceptanceAppliesTrackedTemplate(t *testing.T) {
	doc := docbridge.Virtual("/example.java", "if")
	state := &editor.State{Buffer: doc.Buffer()}
	state.Select(2, 2)
	expansion := templateExpansion{Text: "if (condition) {\n\t\n}", Caret: len("if (condition) {")}
	item := editor.Completion{
		Label: "if", Insert: expansion.Text, FilterText: "if",
		Kind: editor.CompletionSnippet,
	}
	state.ShowCompletions(0, []editor.Completion{item})
	tab := &Tab{doc: doc, state: state}
	tab.completion = completionTracking{
		version: doc.Buffer().Version(), cursor: 2, start: 0,
		infos: map[string]completionInfo{
			completionTrackKey(item): {Template: &templateCompletionAction{
				TemplateID: "live.if", From: 0, To: 2, Expansion: expansion,
			}},
		},
		selected: &item, open: true, generation: 1, focused: true,
		itemCount: 1, selectedIndex: 0,
	}
	m := &Model{tabs: []*Tab{tab}, active: 0}

	if !m.tabTemplateInput(tab, ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyTab}) {
		t.Fatal("Tab did not accept the tracked template")
	}
	if got := tab.state.Buffer.String(); got != expansion.Text {
		t.Fatalf("template expansion = %q, want %q", got, expansion.Text)
	}
	if got := tab.state.Cursor(); got != expansion.Caret {
		t.Fatalf("template caret = %d, want %d", got, expansion.Caret)
	}
}

func TestBuiltinLiveTemplateSelectionStaysCleanUntilEdited(t *testing.T) {
	m := &Model{settingsDraft: settingsDraft{templates: defaultTemplatePrefs()}}
	m.settingsTemplates.liveID = "live.sout"
	template, ok := m.settingsLiveTemplate(m.settingsTemplates.liveID)
	if !ok {
		t.Fatal("built-in template is not selectable")
	}
	m.commitSettingsLiveTemplate(template)
	if len(m.settingsDraft.templates.Live.Custom) != 0 {
		t.Fatalf("selection created an override: %+v", m.settingsDraft.templates.Live.Custom)
	}
	template.Body += "\n// edited"
	m.commitSettingsLiveTemplate(template)
	if len(m.settingsDraft.templates.Live.Custom) != 1 {
		t.Fatalf("edit did not create an override: %+v", m.settingsDraft.templates.Live.Custom)
	}
	template.Body = defaultLiveTemplates[0].Body
	m.commitSettingsLiveTemplate(template)
	if len(m.settingsDraft.templates.Live.Custom) != 0 {
		t.Fatalf("reverting the built-in kept an override: %+v", m.settingsDraft.templates.Live.Custom)
	}
}

func TestFileTemplateSettingsSeedProjectOnlyWhenOpened(t *testing.T) {
	prefs := defaultTemplatePrefs()
	m := &Model{templatePrefs: &prefs}
	m.setRoot(t.TempDir())
	m.settingsDraft.templates = cloneTemplatePrefs(prefs)
	m.loadTemplateSettingsPage(settingsPageFileTemplates)
	if len(m.settingsDraft.templates.Files.Projects) != 0 {
		t.Fatalf("opening file templates created a project scheme: %+v", m.settingsDraft.templates.Files.Projects)
	}
	m.settingsTemplates.projectFileView = true
	_ = m.settingsFileTemplateScheme()
	if len(m.settingsDraft.templates.Files.Projects) != 1 {
		t.Fatalf("opening Project scheme did not seed it: %+v", m.settingsDraft.templates.Files.Projects)
	}
}

func TestTemplateSettingsStageDirtyAndReset(t *testing.T) {
	dir := t.TempDir()
	m := &Model{cfg: Config{SettingsPath: filepath.Join(dir, "settings.json")}}
	m.setRoot(dir)

	m.OpenSettingsPage(settingsPageLiveTemplates)
	if m.settingsDirtyPage(settingsPageLiveTemplates) {
		t.Fatal("Live Templates page opened dirty")
	}
	m.settingsDraft.templates.Live.CaseSensitive = true
	if !m.settingsDirtyPage(settingsPageLiveTemplates) {
		t.Fatal("Live Templates change was not staged")
	}
	m.settingsResetPage()
	if m.settingsDirtyPage(settingsPageLiveTemplates) || m.settingsDraft.templates.Live.CaseSensitive {
		t.Fatal("Live Templates reset kept staged changes")
	}

	m.OpenSettingsPage(settingsPagePostfix)
	m.settingsDraft.templates.Postfix.Enabled = false
	if !m.settingsDirtyPage(settingsPagePostfix) {
		t.Fatal("Postfix Completion change was not staged")
	}
	m.settingsResetPage()
	if m.settingsDirtyPage(settingsPagePostfix) || !m.settingsDraft.templates.Postfix.Enabled {
		t.Fatal("Postfix Completion reset kept staged changes")
	}

	m.OpenSettingsPage(settingsPageFileTemplates)
	if len(m.settingsDraft.templates.Files.Projects) != 0 {
		t.Fatal("File and Code Templates page opened with a project scheme")
	}
	m.mutateSettingsFileTemplateScheme(func(scheme *FileTemplateScheme) {
		scheme.Templates[0].name = "Changed Template"
	})
	if !m.settingsDirtyPage(settingsPageFileTemplates) {
		t.Fatal("File and Code Templates change was not staged")
	}
	m.settingsResetPage()
	if m.settingsDirtyPage(settingsPageFileTemplates) {
		t.Fatal("File and Code Templates reset kept staged changes")
	}
}

func TestFileTemplateResetKeepsUnrelatedScopes(t *testing.T) {
	root := t.TempDir()
	other := t.TempDir()
	prefs := defaultTemplatePrefs()
	global := prefs.Files.Schemes[prefs.Files.Default]
	global.Includes["Global"] = "global"
	prefs.Files.Schemes[prefs.Files.Default] = global
	prefs.Files.Projects[canonicalTemplateProject(root)] = FileTemplateScheme{
		Templates: []projectNewTemplate{{
			id: "current", name: "Current", kinds: []projectNewKind{{id: "file", body: "$END$"}},
		}},
		Includes: map[string]string{"Current": "current"},
	}
	prefs.Files.Projects[canonicalTemplateProject(other)] = FileTemplateScheme{
		Templates: []projectNewTemplate{{
			id: "other", name: "Other", kinds: []projectNewKind{{id: "file", body: "$END$"}},
		}},
		Includes: map[string]string{"Other": "other"},
	}
	m := &Model{templatePrefs: &prefs}
	m.setRoot(root)
	m.settingsDraft.templates = cloneTemplatePrefs(prefs)

	m.resetTemplateSettingsPage(settingsPageFileTemplates)
	files := m.settingsDraft.templates.Files
	if !reflect.DeepEqual(files.Schemes[files.Default], defaultFileTemplateScheme()) {
		t.Fatal("Default Reset did not restore the global scheme")
	}
	if len(files.Projects) != 2 ||
		files.Projects[canonicalTemplateProject(root)].Includes["Current"] != "current" ||
		files.Projects[canonicalTemplateProject(other)].Includes["Other"] != "other" {
		t.Fatalf("Default Reset changed project overrides: %+v", files.Projects)
	}

	m.settingsTemplates.projectFileView = true
	m.resetTemplateSettingsPage(settingsPageFileTemplates)
	files = m.settingsDraft.templates.Files
	if _, ok := files.Projects[canonicalTemplateProject(root)]; ok {
		t.Fatal("Project Reset kept the current project override")
	}
	if files.Projects[canonicalTemplateProject(other)].Includes["Other"] != "other" {
		t.Fatal("Project Reset removed another project's override")
	}
	if !m.settingsTemplates.projectFileView {
		t.Fatal("Project Reset switched away from the active project scope")
	}
}

func TestCustomFileTemplateFlowsIntoNewFile(t *testing.T) {
	root := t.TempDir()
	custom := projectNewTemplate{
		id: "custom", name: "Custom Text", extension: "txt", defaultName: "notes",
		kinds: []projectNewKind{{
			id: "file", label: "File", body: "header\n$END$\n",
		}},
	}
	prefs := defaultTemplatePrefs()
	prefs.Files.Schemes[prefs.Files.Default] = FileTemplateScheme{
		Templates: []projectNewTemplate{custom},
		Includes:  map[string]string{"Header": "// $NAME$\n"},
	}
	m := &Model{templatePrefs: &prefs}
	m.setRoot(root)
	m.OpenProjectNew(root)
	if m.projectNew == nil || len(m.projectNew.matches) == 0 {
		t.Fatalf("custom template is missing from New: %+v", m.projectNew)
	}
	var customIndex int
	found := false
	for _, match := range m.projectNew.matches {
		if match.entry.templateID == "custom" {
			customIndex = match.entry.templateIndex
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("custom template is missing from New: %+v", m.projectNew)
	}
	m.SelectProjectNewTemplate(customIndex)
	if m.treeOp == nil || m.treeOp.projectTemplate == nil ||
		m.treeOp.projectTemplate.id != "custom" ||
		m.treeOp.projectIncludes["Header"] != "// $NAME$\n" {
		t.Fatalf("New File state = %+v", m.treeOp)
	}
	text, caret := projectNewRenderedTextWithIncludes(
		m.treeOp.projectTemplate, 0, projectNewPlan{directory: root, name: "notes"},
		"", "project", root, time.Now(), m.treeOp.projectIncludes,
	)
	if text != "header\n\n" || caret != len("header\n") {
		t.Fatalf("custom template output = %q, caret %d", text, caret)
	}
}

func TestDefaultLiveTemplatesUseRealNewlines(t *testing.T) {
	for _, template := range defaultLiveTemplates {
		if strings.Contains(template.Body, `\n`) {
			t.Errorf("%s body contains a literal backslash-n: %q", template.ID, template.Body)
		}
	}
}

func TestTemplateCommandsAreInNativeMenus(t *testing.T) {
	found := map[string]bool{}
	walkMenu(mainMenuGroups(), func(entry menuEntry) {
		if entry.command != "" {
			found[entry.command] = true
		}
	})
	for _, command := range []string{
		"code.insertLiveTemplate", "code.surroundWith", "code.surroundWithLiveTemplate",
		"emmet.updateTag", "templates.newFromTemplate",
	} {
		if !found[command] {
			t.Errorf("main menu has no %s", command)
		}
	}
}

func TestTemplateActionsOpenTheirNativeChoosers(t *testing.T) {
	prefs := defaultTemplatePrefs()
	doc, err := docbridge.Open(context.Background(), templateActionBackend{}, "", "/example.html")
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close(context.Background())
	state := &editor.State{Buffer: doc.Buffer()}
	m := &Model{
		templatePrefs: &prefs,
		tabs:          []*Tab{{doc: doc, state: state}},
		active:        0,
	}

	if !m.RunCommand("code.insertLiveTemplate") ||
		m.templateChooser == nil || m.templateChooser.kind != "live" {
		t.Fatalf("Insert Live Template chooser = %+v", m.templateChooser)
	}
	m.closeTemplateChooser()
	state.Buffer.Edit(editor.Edit{Start: 0, End: 0, Text: "hello"})
	state.Select(0, len("hello"))
	if !m.ActionEnabled("code.surroundWith") {
		t.Fatalf("Surround With is disabled: tab=%+v docReadOnly=%v",
			m.tab(), m.tab() != nil && m.tab().doc != nil && m.tab().doc.ReadOnly())
	}
	if !m.RunCommand("code.surroundWith") {
		t.Fatal("Surround With action did not run")
	}
	if m.templateChooser == nil || m.templateChooser.kind != "surround" {
		t.Fatalf("Surround With chooser = %+v selection=%+v buffer=%q",
			m.templateChooser, state.Selection(), state.Buffer.Slice(0, state.Buffer.Len()))
	}
	m.closeTemplateChooser()
	if !m.RunCommand("code.surroundWithLiveTemplate") ||
		m.templateChooser == nil || m.templateChooser.kind != "surround" ||
		len(m.templateChooser.templates) == 0 {
		t.Fatalf("Surround with Live Template chooser = %+v", m.templateChooser)
	}
	m.closeTemplateChooser()
	state.Buffer.Edit(editor.Edit{
		Start: 0, End: state.Buffer.Len(), Text: `<section>text</section>`,
	})
	state.Select(len("<section>"), len("<section>"))
	if !m.RunCommand("emmet.updateTag") ||
		m.templateChooser == nil || m.templateChooser.kind != "updateTag" {
		t.Fatalf("Emmet Update Tag chooser = %+v", m.templateChooser)
	}
	m.closeTemplateChooser()
	for _, id := range []string{"templates.chooser", "templates.emmetUpdateTag"} {
		if _, ok := overlays.byID[id]; !ok {
			t.Errorf("overlay registry lacks %s", id)
		}
	}
}

type templateActionBackend struct{}

func (templateActionBackend) DocOpen(
	_ context.Context, _, path string,
) (openidea.DocInfo, error) {
	return openidea.DocInfo{ID: 1, Path: path, Version: 1}, nil
}

func (templateActionBackend) DocEdit(
	_ context.Context, _ string, _ uint32, _ uint32, _ position.ChangeSet,
) (uint32, error) {
	return 2, nil
}

func (templateActionBackend) DocSnapshot(
	_ context.Context, _ string, _ uint32,
) (openidea.DocInfo, error) {
	return openidea.DocInfo{ID: 1, Path: "/example.html", Version: 1}, nil
}

func (templateActionBackend) DocSave(context.Context, string, uint32) error  { return nil }
func (templateActionBackend) DocClose(context.Context, string, uint32) error { return nil }
