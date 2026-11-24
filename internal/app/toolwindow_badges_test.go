package app

import (
	"path/filepath"
	"testing"

	"mayoi/internal/openidea"
)

func boolPointerForToolWindowTest(value bool) *bool { return &value }

func TestOnUseStripeButtonsFollowOpenAndHide(t *testing.T) {
	m := toolModel()
	if !m.toolWindowStripeVisible("problems") {
		t.Fatal("Problems is a default stripe window")
	}
	if m.toolWindowStripeVisible("services") {
		t.Fatal("Services is an onUse window and starts without a button")
	}

	m.ShowServices()
	if !m.toolWindowStripeVisible("services") {
		t.Fatal("opening Services did not add its stripe button")
	}
	m.DockHide("services")
	if !m.toolWindowStripeVisible("services") {
		t.Fatal("hiding Services removed the button it gained when opened")
	}
	m.HideToolWindowFromStripe("services")
	if m.toolWindowStripeVisible("services") {
		t.Fatal("Hide did not remove the Services stripe button")
	}
}

func TestStripeButtonsRoundTripWithTheDockLayout(t *testing.T) {
	m := toolModel()
	m.ShowBuild()
	m.HideToolWindowFromStripe("build")
	m.dock.stripeButtons["gradle"] = true

	layout := m.currentLayout()
	if layout.Dock == nil || !layout.Dock.StripeButtons["gradle"] ||
		layout.Dock.StripeButtons["build"] {
		t.Fatalf("stripe choices = %+v", layout.Dock)
	}
	restored := toolModel()
	restored.applyLayoutSnapshot(layout)
	if !restored.toolWindowStripeVisible("gradle") {
		t.Fatal("the shown Gradle stripe choice did not restore")
	}
	if restored.dock.stripeButtons["build"] {
		t.Fatal("the hidden Build stripe choice did not restore")
	}
}

func TestToolWindowBadgeSettingDefaultsOnAndHidesCounts(t *testing.T) {
	m := toolModel()
	m.run.children = []*runChild{
		{running: true},
		{running: true, service: true},
		{finished: true},
	}
	m.services.children = []*runChild{
		{service: true, running: true, config: openidea.RunConfig{ID: "a", Kind: "external"}},
		{service: true, running: true, config: openidea.RunConfig{ID: "b", Kind: "external"}},
	}
	m.services.types = []string{"external"}
	m.services.root = m.Root()
	m.run.configs = []openidea.RunConfig{
		{ID: "a", Kind: "external"},
		{ID: "b", Kind: "external"},
	}
	m.build.errors = 3
	failed := false
	m.build.success = &failed
	m.todo.items = map[string][]TodoItem{"/p/A.java": {{Path: "/p/A.java"}}}
	m.agents.unread = map[string]bool{"task": true}
	m.agents.tasks = map[string]agentTask{"task": {ID: "task"}}
	m.vcs.files = nil

	if got := m.toolWindowBadge("run"); got != 1 {
		t.Fatalf("Run badge = %d, want one non-service session", got)
	}
	if got := m.toolWindowBadge("services"); got != 2 {
		t.Fatalf("Services badge = %d", got)
	}
	if got := m.toolWindowBadge("build"); got != 3 {
		t.Fatalf("Build badge = %d", got)
	}
	if got := m.toolWindowBadge("problems"); got != 3 {
		t.Fatalf("Problems badge = %d", got)
	}
	if got := m.toolWindowBadge("todo"); got != 1 {
		t.Fatalf("TODO badge = %d", got)
	}
	if got := m.toolWindowBadge("agents"); got != 1 {
		t.Fatalf("Agents badge = %d", got)
	}

	m.prefs.Appearance.ShowToolWindowBadges = boolPointerForToolWindowTest(false)
	if got := m.toolWindowBadge("services"); got != 0 {
		t.Fatalf("hidden badge still reported %d", got)
	}
}

func TestToolWindowBadgeSettingPersistsExplicitChoice(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if !(Preferences{}).toolWindowBadgesVisible() {
		t.Fatal("missing setting must keep OpenIDEA's default of visible badges")
	}

	prefs := Preferences{}
	prefs.Appearance.ShowToolWindowBadges = boolPointerForToolWindowTest(false)
	if err := prefs.Save(path); err != nil {
		t.Fatal(err)
	}
	stored := LoadPreferences(path)
	if got := stored.Appearance.ShowToolWindowBadges; got == nil || *got {
		t.Fatalf("stored hidden choice = %v", got)
	}
	if stored.toolWindowBadgesVisible() {
		t.Fatal("an explicit false choice became visible after reload")
	}

	prefs.Appearance.ShowToolWindowBadges = boolPointerForToolWindowTest(true)
	if err := prefs.Save(path); err != nil {
		t.Fatal(err)
	}
	stored = LoadPreferences(path)
	if got := stored.Appearance.ShowToolWindowBadges; got == nil || !*got {
		t.Fatalf("stored visible choice = %v", got)
	}
	if !stored.toolWindowBadgesVisible() {
		t.Fatal("an explicit true choice became hidden after reload")
	}
}
