package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

func newVimTestModel(t *testing.T, text string) (*Model, *Tab, *vimTabState) {
	t.Helper()
	previousRead, previousWrite := vimClipboardRead, vimClipboardWrite
	vimClipboardRead = func() string { return "" }
	vimClipboardWrite = func(string) {}
	t.Cleanup(func() {
		vimClipboardRead = previousRead
		vimClipboardWrite = previousWrite
	})
	state := &editor.State{
		Buffer:   editor.NewBuffer(text),
		Bindings: intellijEditorBindings(),
	}
	tab := &Tab{state: state}
	m := &Model{
		tabs:   []*Tab{tab},
		active: 0,
		prefs: Preferences{
			Vim: VimPrefs{Enabled: true},
		},
	}
	m.applyVimToTab(tab)
	return m, tab, m.vimStateFor(tab)
}

func feedVim(t *testing.T, m *Model, tab *Tab, state *vimTabState, tokens ...string) {
	t.Helper()
	for _, token := range tokens {
		m.vimFeedToken(tab, state, token, 0)
	}
}

func TestVimNormalOperatorsVisualModesAndCounts(t *testing.T) {
	m, tab, state := newVimTestModel(t, "one two\nthree\n")
	feedVim(t, m, tab, state, "d", "w")
	if got := tab.state.Buffer.String(); got != "two\nthree\n" {
		t.Fatalf("dw produced %q", got)
	}
	feedVim(t, m, tab, state, "2", "x")
	if got := tab.state.Buffer.String(); got != "o\nthree\n" {
		t.Fatalf("2x produced %q", got)
	}

	m, tab, state = newVimTestModel(t, "abc\ndef\n")
	feedVim(t, m, tab, state, "v", "l", "d")
	if got := tab.state.Buffer.String(); got != "c\ndef\n" {
		t.Fatalf("visual delete produced %q", got)
	}
	if state.mode != vimNormal {
		t.Fatalf("mode after visual delete = %v", state.mode)
	}

	m, tab, state = newVimTestModel(t, "one two\nthree\n")
	feedVim(t, m, tab, state, "d", "e")
	if got := tab.state.Buffer.String(); got != " two\nthree\n" {
		t.Fatalf("de produced %q", got)
	}
}

func TestVimInsertModeLeavesTextToMyGo(t *testing.T) {
	m, tab, state := newVimTestModel(t, "abc\n")
	feedVim(t, m, tab, state, "i")
	if state.mode != vimInsert {
		t.Fatalf("i left mode %v", state.mode)
	}
	if m.vimInput(tab, ui.InputEvent{Kind: ui.InputText, Text: "x"}) {
		t.Fatal("insert-mode text was consumed by Vim instead of MyGo")
	}
	if got := tab.state.Buffer.String(); got != "abc\n" {
		t.Fatalf("insert-mode key changed the buffer twice: %q", got)
	}
	if !m.vimInput(tab, ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyEscape}) {
		t.Fatal("Escape did not return to Normal mode")
	}
	if state.mode != vimNormal {
		t.Fatalf("Escape left mode %v", state.mode)
	}
}

func TestVimRegisterSurvivesWithoutWindowClipboard(t *testing.T) {
	m, tab, state := newVimTestModel(t, "first\nsecond\n")
	feedVim(t, m, tab, state, "d", "d", "p")
	if got := tab.state.Buffer.String(); got != "second\nfirst\n" {
		t.Fatalf("ddp produced %q", got)
	}
	if got := state.register.text; got != "first\n" || !state.register.linewise {
		t.Fatalf("line register = %+v", state.register)
	}
}

func TestVimCommandLineRunsExCommandsAndKeepsErrors(t *testing.T) {
	m, tab, state := newVimTestModel(t, "text\n")
	feedVim(t, m, tab, state, ":")
	if m.vim.command == nil || m.vim.command.tab != tab {
		t.Fatalf("command line = %+v", m.vim.command)
	}
	tester := ui.NewTester(func(c *ui.Context) { m.renderOverlays(c, c.Theme()) }, 900, 600)
	texts := strings.Join(tester.Texts(), "\n")
	if !strings.Contains(texts, "Run") || !strings.Contains(texts, "Cancel") {
		t.Fatalf("Vim command overlay = %q", texts)
	}
	m.vim.command.input = "unknown"
	m.submitVimCommand()
	if m.vim.command == nil || !strings.Contains(m.vim.command.err, "unsupported command") {
		t.Fatalf("command error was not kept: %+v", m.vim.command)
	}
	m.vim.command.input = ""
	m.submitVimCommand()
	if m.vim.command != nil {
		t.Fatalf("empty command did not close: %+v", m.vim.command)
	}
}

func TestVimTextObjectsForWordsBracketsAndQuotes(t *testing.T) {
	m, tab, state := newVimTestModel(t, "one two\n")
	feedVim(t, m, tab, state, "c", "i", "w")
	if got := tab.state.Buffer.String(); got != " two\n" {
		t.Fatalf("ciw produced %q", got)
	}
	if state.mode != vimInsert || state.register.text != "one" {
		t.Fatalf("ciw state = %+v, register = %+v", state, state.register)
	}

	m, tab, state = newVimTestModel(t, "a(b(c)d)e")
	tab.state.Select(4, 4)
	feedVim(t, m, tab, state, "d", "i")
	if !state.operatorText || !state.operatorInner {
		t.Fatalf("di did not wait for an inner text object: %+v", state)
	}
	feedVim(t, m, tab, state, "(")
	if got := tab.state.Buffer.String(); got != "a(b()d)e" {
		t.Fatalf("di( produced %q", got)
	}

	m, tab, state = newVimTestModel(t, `say "hello" now`)
	tab.state.Select(5, 5)
	feedVim(t, m, tab, state, "d", "a", `"`)
	if got := tab.state.Buffer.String(); got != "say  now" {
		t.Fatalf("da\" produced %q", got)
	}
}

func TestVimVisualTextObjectSelectsThenDeletes(t *testing.T) {
	m, tab, state := newVimTestModel(t, "one two\n")
	feedVim(t, m, tab, state, "v", "i", "w")
	if got := tab.state.SelectedText(); got != "one" {
		t.Fatalf("viw selected %q", got)
	}
	feedVim(t, m, tab, state, "d")
	if got := tab.state.Buffer.String(); got != " two\n" {
		t.Fatalf("viwd produced %q", got)
	}
	if state.mode != vimNormal || state.register.text != "one" {
		t.Fatalf("visual object state = %+v, register = %+v", state, state.register)
	}
}

func TestVimFunctionTextObjectsMatchOpenIDEA(t *testing.T) {
	code := "class A {\n    int one() {\n        return 1;\n    }\n\n    void two() {}\n}\n"
	at := strings.Index(code, "return 1")
	m, tab, state := newVimTestModel(t, code)
	tab.state.Select(at, at)
	feedVim(t, m, tab, state, "d", "a", "f")
	if got := tab.state.Buffer.String(); strings.Contains(got, "int one()") ||
		!strings.Contains(got, "void two()") {
		t.Fatalf("daf produced %q", got)
	}

	m, tab, state = newVimTestModel(t, code)
	tab.state.Select(at, at)
	feedVim(t, m, tab, state, "d", "i", "f")
	got := tab.state.Buffer.String()
	if !strings.Contains(got, "int one() {\n    }") ||
		strings.Contains(got, "return 1") {
		t.Fatalf("dif produced %q", got)
	}

	m, tab, state = newVimTestModel(t, code)
	tab.state.Select(at, at)
	feedVim(t, m, tab, state, "v", "a", "f")
	if got := tab.state.SelectedText(); !strings.Contains(got, "int one()") ||
		!strings.Contains(got, "return 1") || strings.Contains(got, "void two()") {
		t.Fatalf("vaf selected %q", got)
	}
}

func TestVimRcMappingsPreferencesAndSettingsPersistence(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	rc := "let mapleader = ','\nnnoremap <leader>x dd\n"
	if err := os.WriteFile(filepath.Join(home, ".ideavimrc"), []byte(rc), 0o644); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(root, "settings.json")
	m, tab, state := newVimTestModel(t, "first\nsecond\n")
	m.cfg = Config{Home: home, SettingsPath: settingsPath}
	m.vim.rcLoaded = false
	m.loadVimSettingsPage()
	if m.vim.rc.Path != filepath.Join(home, ".ideavimrc") || m.vim.rc.Mappings != 1 {
		t.Fatalf("loaded rc = %+v", m.vim.rc)
	}
	feedVim(t, m, tab, state, ",", "x")
	if got := tab.state.Buffer.String(); got != "second\n" {
		t.Fatalf("leader mapping produced %q", got)
	}

	m.prefs.Vim = VimPrefs{}
	m.settingsDraft = m.settingsDraftFromPrefs()
	m.settingsDraft.vimEnabled = true
	m.setVimHandler("Ctrl+R", "ide")
	if !m.applyPreferenceSettings() {
		t.Fatalf("apply Vim settings: %s", m.settingsErr)
	}
	if !m.prefs.Vim.Enabled || m.prefs.Vim.Handlers["Ctrl+R"] != "ide" {
		t.Fatalf("applied Vim preferences = %+v", m.prefs.Vim)
	}
	loaded := LoadPreferences(settingsPath)
	if !loaded.Vim.Enabled || loaded.Vim.Handlers["Ctrl+R"] != "ide" {
		t.Fatalf("persisted Vim preferences = %+v", loaded.Vim)
	}
	if !tab.state.NoInputMethod {
		t.Fatal("Normal mode should disable the input method in a Vim editor")
	}
}

func TestVimSettingsPageRegistersAndDescribesRc(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".openideavimrc"),
		[]byte("set ignorecase\nunknown-command\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := &Model{
		cfg:   Config{Home: home, SettingsPath: filepath.Join(root, "settings.json")},
		prefs: Preferences{},
	}
	m.OpenSettingsPage(settingsPageVim)
	if !m.settingsOpen || m.settingsPage != settingsPageVim {
		t.Fatalf("settings page = %q, open=%v", m.settingsPage, m.settingsOpen)
	}
	all := strings.Join(m.settingsPageSummary(), "\n")
	page, ok := settingsPageByID(settingsPageVim)
	if !ok {
		t.Fatal("Vim page is not registered")
	}
	all += "\n" + strings.Join(page.labels, "\n")
	for _, want := range []string{"Vim", "Enable Vim emulation", "Shortcut conflicts"} {
		if !strings.Contains(all, want) {
			t.Fatalf("settings summary lacks %q:\n%s", want, all)
		}
	}
	if len(m.vim.rc.Ignored) == 0 || m.vim.rc.Path == "" {
		t.Fatalf("rc state = %+v", m.vim.rc)
	}
}

func TestVimActionsToggleReloadAndOpenSettings(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	rcPath := filepath.Join(home, ".ideavimrc")
	if err := os.WriteFile(rcPath, []byte("set ignorecase\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := &Model{
		cfg: Config{
			Home:         home,
			SettingsPath: filepath.Join(root, "settings.json"),
		},
		prefs: Preferences{},
	}

	if !m.RunCommand("vim.toggle") || !m.prefs.Vim.Enabled {
		t.Fatal("vim.toggle did not enable Vim")
	}
	if !m.ActionEnabled("vim.reloadRc") {
		t.Fatal("vim.reloadRc is disabled while Vim is enabled")
	}
	m.vim.rc.Path = ""
	if !m.RunCommand("vim.reloadRc") || m.vim.rc.Path != rcPath {
		t.Fatalf("vim.reloadRc did not load %s: %+v", rcPath, m.vim.rc)
	}
	if !m.RunCommand("vim.settings") ||
		!m.settingsOpen || m.settingsPage != settingsPageVim {
		t.Fatalf("vim.settings did not open the Vim page: open=%v page=%q",
			m.settingsOpen, m.settingsPage)
	}
	if !m.RunCommand("vim.toggle") || m.prefs.Vim.Enabled {
		t.Fatal("vim.toggle did not disable Vim")
	}
	if m.ActionEnabled("vim.reloadRc") {
		t.Fatal("vim.reloadRc remained enabled after Vim was disabled")
	}
}

func TestVimRcNormalizesSpecialKeysAndCaseInsensitiveLeader(t *testing.T) {
	m := &Model{}
	parsed := m.parseVimRc(`
let mapleader="$&"
inoremap jk <esc>
map <c-c> <C-C>
imap <CR> <cr>
nmap <s-x> X
nmap <s-tab> <s-cr>
nmap <f1> <Action>(GotoDeclaration)
nmap <LeAdEr>a <LEADER>b
`, "")
	if len(parsed.Ignored) != 0 {
		t.Fatalf("unexpected ignored rc lines: %+v", parsed.Ignored)
	}
	want := []vimRcMapping{
		{Mode: "insert", LHS: []string{"j", "k"}, RHS: []string{"<Esc>"}, Noremap: true},
		{Mode: "all", LHS: []string{"<Ctrl+C>"}, RHS: []string{"<Ctrl+C>"}},
		{Mode: "insert", LHS: []string{"<Enter>"}, RHS: []string{"<Enter>"}},
		{Mode: "normal", LHS: []string{"X"}, RHS: []string{"X"}},
		{Mode: "normal", LHS: []string{"<Shift+Tab>"}, RHS: []string{"<Shift+Enter>"}},
		{Mode: "normal", LHS: []string{"<F1>"}, Ex: "action GotoDeclaration"},
		{Mode: "normal", LHS: []string{"$", "&", "a"}, RHS: []string{"$", "&", "b"}},
	}
	if len(parsed.Mappings) != len(want) {
		t.Fatalf("mappings = %+v, want %d entries", parsed.Mappings, len(want))
	}
	for i := range want {
		if !sameVimTokens(parsed.Mappings[i].LHS, want[i].LHS) ||
			!sameVimTokens(parsed.Mappings[i].RHS, want[i].RHS) ||
			parsed.Mappings[i].Mode != want[i].Mode ||
			parsed.Mappings[i].Ex != want[i].Ex ||
			parsed.Mappings[i].Noremap != want[i].Noremap {
			t.Fatalf("mapping %d = %+v, want %+v", i, parsed.Mappings[i], want[i])
		}
	}
	if got := tokenizeVimKeys("<<esc>"); !sameVimTokens(got, []string{"<", "<Esc>"}) {
		t.Fatalf("nested escaped key tokens = %v", got)
	}
	if got := tokenizeVimKeys("<c-Frob><z-esc><constructor>"); !sameVimTokens(
		got, []string{"<c-Frob>", "<z-esc>", "<constructor>"},
	) {
		t.Fatalf("unknown key tokens = %v", got)
	}
}

func TestVimRcExCommandsAndInputTokensMatchMappings(t *testing.T) {
	m := &Model{}
	parsed := m.parseVimRc(`
nmap <f1> :echo <cr>
nmap <f2> :<C-u>action Back<cr>
nmap <f3> <Action>(GotoDeclaration)
nmap <f4> :action A<CR>j
`, "")
	if len(parsed.Mappings) != 4 ||
		parsed.Mappings[0].Ex != "echo" ||
		parsed.Mappings[1].Ex != "action Back" ||
		parsed.Mappings[2].Ex != "action GotoDeclaration" ||
		len(parsed.Mappings[3].RHS) == 0 {
		t.Fatalf("Ex mappings = %+v", parsed.Mappings)
	}
	cases := []struct {
		mods  ui.Modifiers
		key   ui.Key
		token string
	}{
		{ui.Ctrl, ui.KeyF, "<Ctrl+F>"},
		{ui.Ctrl | ui.Shift, ui.KeyX, "<Ctrl+Shift+X>"},
		{ui.Alt, ui.KeyX, "<Alt+X>"},
		{ui.Cmd, ui.KeyX, "<Cmd+X>"},
		{ui.Shift, ui.KeyTab, "<Shift+Tab>"},
		{ui.Ctrl, ui.KeyBracketLeft, "<Ctrl-[>"},
	}
	for _, tc := range cases {
		got, printable := vimInputToken(ui.InputEvent{
			Kind: ui.InputKeyDown,
			Mods: tc.mods,
			Key:  tc.key,
		})
		if got != tc.token || printable {
			t.Fatalf("input token for %v+%v = %q printable=%v, want %q",
				tc.mods, tc.key, got, printable, tc.token)
		}
	}
}

func TestVimRcSourcesFilesOnceAndReportsCycles(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a.vim")
	b := filepath.Join(root, "b.vim")
	if err := os.WriteFile(a, []byte("source b.vim\nnmap a x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("source a.vim\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := &Model{cfg: Config{Home: root}}
	parsed := m.parseVimRc(
		"source a.vim\nsource a.vim\n",
		filepath.Join(root, "main.vim"),
	)
	if len(parsed.Mappings) != 1 ||
		!sameVimTokens(parsed.Mappings[0].LHS, []string{"a"}) {
		t.Fatalf("duplicate source was applied more than once: %+v", parsed.Mappings)
	}
	if len(parsed.Ignored) != 1 || parsed.Ignored[0].Reason != "file sources itself" {
		t.Fatalf("cycle diagnostics = %+v", parsed.Ignored)
	}

	tab := &Tab{}
	m.vim.tabs = map[*Tab]*vimTabState{
		tab: {mappingKeys: []string{"stale"}},
	}
	m.applyVimRc(vimRcParse{})
	if m.vim.tabs[tab].mappingKeys != nil {
		t.Fatalf("rc reload kept a live mapping prefix: %v", m.vim.tabs[tab].mappingKeys)
	}
}
