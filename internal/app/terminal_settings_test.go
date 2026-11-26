package app

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func terminalSettingsModel(values map[string]any) *Model {
	m := &Model{}
	m.nativeSettings.pages = map[string]nativeSettingsPageState{
		settingsPageTerminal: {loaded: true, applied: values, draft: values},
	}
	return m
}

func TestTerminalActivatesConfiguredPythonVirtualenv(t *testing.T) {
	root := t.TempDir()
	m := terminalSettingsModel(map[string]any{
		"terminal.env": map[string]string{"PATH": "/usr/bin", "KEEP": "1"},
	})
	m.setRoot(root)
	pythonPage := nativeSettingsPageState{
		loaded: true,
		scope:  "project",
		applied: map[string]any{
			"languages.python.interpreter":        filepath.Join(root, ".venv", "bin", "python"),
			"languages.python.activateVirtualenv": true,
		},
		draft: map[string]any{},
	}
	m.nativeSettings.pages[settingsPagePythonDeps] = pythonPage
	options := m.terminalSpawnOptions("/bin/zsh", nil)
	env, ok := options["env"].(map[string]string)
	if !ok {
		t.Fatalf("terminal environment = %#v", options["env"])
	}
	wantBin := filepath.Join(root, ".venv", "bin")
	if env["VIRTUAL_ENV"] != filepath.Join(root, ".venv") ||
		env["KEEP"] != "1" ||
		!strings.HasPrefix(env["PATH"], wantBin+string(os.PathListSeparator)) {
		t.Fatalf("activated Python environment = %#v", env)
	}
	pythonPage.applied["languages.python.activateVirtualenv"] = false
	m.nativeSettings.pages[settingsPagePythonDeps] = pythonPage
	options = m.terminalSpawnOptions("/bin/zsh", nil)
	env = options["env"].(map[string]string)
	if _, exists := env["VIRTUAL_ENV"]; exists || env["PATH"] != "/usr/bin" {
		t.Fatalf("disabled Python activation changed environment: %#v", env)
	}
}

func TestTerminalStartDirectoryResolvesProjectRelativeAndAbsolutePaths(t *testing.T) {
	root := t.TempDir()
	m := terminalSettingsModel(map[string]any{"terminal.startDirectory": "tools/run"})
	if got, want := m.terminalStartDirectory(root), filepath.Join(root, "tools", "run"); got != want {
		t.Fatalf("relative start directory = %q, want %q", got, want)
	}

	absolute := t.TempDir()
	setNativeAppliedTestValue(m, settingsPageTerminal, "terminal.startDirectory", absolute)
	if got := m.terminalStartDirectory(root); got != filepath.Clean(absolute) {
		t.Fatalf("absolute start directory = %q, want %q", got, absolute)
	}

	setNativeAppliedTestValue(m, settingsPageTerminal, "terminal.startDirectory", "")
	if got := m.terminalStartDirectory(root); got != root {
		t.Fatalf("default start directory = %q, want %q", got, root)
	}
}

func TestTerminalShellCommandParsing(t *testing.T) {
	tests := []struct {
		command string
		program string
		args    []string
	}{
		{"", "", nil},
		{"/bin/zsh -l -i", "/bin/zsh", []string{"-l", "-i"}},
		{`"/Applications/My Shell/bin/zsh" -l`, "/Applications/My Shell/bin/zsh", []string{"-l"}},
		{`/bin/bash -c 'echo "hello world"'`, "/bin/bash", []string{"-c", `echo "hello world"`}},
		{`/bin/zsh "two words" escaped\ value`, "/bin/zsh", []string{"two words", "escaped value"}},
	}
	for _, test := range tests {
		program, args := splitTerminalShellCommand(test.command)
		if program != test.program || !reflect.DeepEqual(args, test.args) {
			t.Fatalf("%q = %q %#v, want %q %#v", test.command, program, args, test.program, test.args)
		}
	}
}

func TestTerminalSpawnOptionsApplyShellEnvironmentAndFont(t *testing.T) {
	m := terminalSettingsModel(map[string]any{
		"terminal.shellPath":          `/bin/zsh -l -i`,
		"terminal.env":                map[string]string{"CI": "1", "LANG": "en_US.UTF-8"},
		"terminal.shellIntegration":   false,
		"terminal.tabName":            "Build",
		"terminal.audibleBell":        false,
		"terminal.closeOnExit":        false,
		"terminal.useEditorFont":      false,
		"terminal.fontFamily":         "Example Mono",
		"terminal.fallbackFontFamily": "Menlo",
		"terminal.fontSize":           float64(16),
		"terminal.lineHeight":         float64(1.4),
	})
	options := m.terminalSpawnOptions("", nil)
	if options["shell"] != "/bin/zsh" ||
		!reflect.DeepEqual(options["args"], []string{"-l", "-i"}) ||
		!reflect.DeepEqual(options["env"], map[string]string{"CI": "1", "LANG": "en_US.UTF-8"}) ||
		options["shellIntegration"] != false {
		t.Fatalf("terminal options = %#v", options)
	}
	if got := m.terminalDefaultTabName(); got != "Build" {
		t.Fatalf("terminal tab name = %q", got)
	}
	if m.terminalPlaysBell() || m.terminalClosesOnExit() {
		t.Fatal("terminal bell/close-on-exit settings were ignored")
	}
	family, size, lineHeight := m.terminalFont()
	if family != "Example Mono, Menlo" || size != 16 || lineHeight != 1.4 {
		t.Fatalf("terminal font = %q %v %v", family, size, lineHeight)
	}
}

func TestTerminalCloseOnExitDropsTheFinishedTab(t *testing.T) {
	m := terminalSettingsModel(map[string]any{"terminal.closeOnExit": true})
	m.term.groups = []termGroup{{
		tabs:   []termSession{{id: 42, pid: new(int32)}},
		active: 0,
	}}
	code := int32(0)
	m.acceptTermExit(42, &code)
	if m.term.total() != 0 {
		t.Fatalf("finished terminal tabs = %d, want 0", m.term.total())
	}
}
