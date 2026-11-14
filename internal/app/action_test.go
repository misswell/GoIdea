package app

import (
	"strings"
	"testing"
)

func TestBuiltinCommandsAreRegisteredOnce(t *testing.T) {
	builtins := builtinCommands()
	if len(actions.byID) < len(builtins) {
		t.Fatalf("registry has %d actions, built-ins are %d", len(actions.byID), len(builtins))
	}
	seen := map[string]bool{}
	for _, a := range actionList() {
		if seen[a.ID] {
			t.Fatalf("%s is registered twice", a.ID)
		}
		seen[a.ID] = true
		if a.Builtin && (a.Title == "" || a.Run == nil) {
			t.Fatalf("%s has no title or run", a.ID)
		}
	}
	if !seen["app.commandPalette"] || !seen["view.services"] {
		t.Fatalf("shipped actions are missing: %v", len(seen))
	}
}

func TestActionRegistrationLifecycle(t *testing.T) {
	ran := 0
	err := registerAction(&Action{ID: "test.action", Title: "Test Action", Category: "Test",
		Run: func(m *Model) { ran++ }})
	if err != nil {
		t.Fatal(err)
	}
	if err := registerAction(&Action{ID: "test.action", Title: "Again", Run: func(m *Model) {}}); err == nil {
		t.Fatal("a duplicate id was accepted")
	}
	if _, ok := Command("test.action"); !ok {
		t.Fatal("the new action is not in the palette's list")
	}
	m := &Model{}
	if !m.RunCommand("test.action") || ran != 1 {
		t.Fatalf("the action did not run: %v %d", m.RunCommand("test.action"), ran)
	}
	if !unregisterAction("test.action") {
		t.Fatal("unregister reported nothing removed")
	}
	if _, ok := Command("test.action"); ok {
		t.Fatal("the action is still listed")
	}
	if m.RunCommand("test.action") {
		t.Fatal("a removed action ran")
	}
}

func TestActionWhenAndEnabledGateRunning(t *testing.T) {
	allowed := false
	ran := 0
	if err := registerAction(&Action{ID: "test.gated", Title: "Gated",
		When:    func(m *Model) bool { return allowed },
		Enabled: func(m *Model) bool { return allowed },
		Run:     func(m *Model) { ran++ }}); err != nil {
		t.Fatal(err)
	}
	defer unregisterAction("test.gated")
	m := &Model{}
	if m.RunAction("test.gated") || m.ActionEnabled("test.gated") {
		t.Fatal("a closed context ran the action")
	}
	allowed = true
	if !m.ActionEnabled("test.gated") || !m.RunAction("test.gated") || ran != 1 {
		t.Fatalf("the open context did not run it: %d", ran)
	}
}

func TestActionToggledAndRunning(t *testing.T) {
	on, busy := false, false
	if err := registerAction(&Action{ID: "test.state", Title: "State",
		Toggled: func(m *Model) bool { return on },
		Running: func(m *Model) bool { return busy },
		Run:     func(m *Model) {}}); err != nil {
		t.Fatal(err)
	}
	defer unregisterAction("test.state")
	m := &Model{}
	if m.ActionToggled("test.state") || m.ActionRunning("test.state") {
		t.Fatal("zero state is on")
	}
	on, busy = true, true
	if !m.ActionToggled("test.state") || !m.ActionRunning("test.state") {
		t.Fatal("the state did not reach the registry")
	}
	if m.ActionToggled("test.missing") || m.ActionRunning("test.missing") {
		t.Fatal("a missing action reported state")
	}
}

func TestKeymapDefaultsAndOverrides(t *testing.T) {
	if got := keymapShortcutLabel("app.commandPalette"); got == "" {
		t.Fatal("the palette has no default shortcut")
	}
	if keymapNormalize("cmd+shift+a") != keymapNormalize("Cmd+Shift+A") {
		t.Fatal("normalization is case sensitive")
	}
	m := &Model{}
	before := m.KeymapBindings("app.commandPalette")
	if len(before) == 0 {
		t.Fatal("no default binding")
	}
	m.KeymapSetOverrides(map[string][]string{"app.commandPalette": {"Alt+P"}})
	if got := m.KeymapBindings("app.commandPalette"); len(got) != 1 || got[0] != "Alt+P" {
		t.Fatalf("the override is %v", got)
	}
	if got := keymapShortcutLabel("app.commandPalette"); got == "Alt+P" {
		t.Fatal("the global label followed a per-model override")
	}
	m.KeymapSetOverrides(map[string][]string{"app.commandPalette": nil})
	if got := m.KeymapBindings("app.commandPalette"); len(got) != 0 {
		t.Fatalf("an empty override kept %v", got)
	}
	if m.prefs.Keymap == nil || len(m.prefs.Keymap["app.commandPalette"]) != 0 {
		t.Fatalf("the removal was not stored: %v", m.prefs.Keymap)
	}
}

func TestKeymapConflicts(t *testing.T) {
	m := &Model{}
	m.KeymapSetOverrides(map[string][]string{
		"app.commandPalette": {"Alt+Q"},
		"view.services":      {"alt+q"},
	})
	conflicts := m.KeymapConflicts()
	found := false
	for _, c := range conflicts {
		if strings.Contains(strings.Join(c.Actions, ","), "app.commandPalette") &&
			strings.Contains(strings.Join(c.Actions, ","), "view.services") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the shared Alt+Q was not reported: %+v", conflicts)
	}
	m.KeymapSetOverrides(nil)
}
