package app

import (
	"testing"

	"mayoi/internal/openidea"
)

// The Run widget is OpenIDEA's RunWidget: its chooser names the selected
// configuration and lists them all, and choosing one selects it.
func TestRunWidgetChooserSelectsAConfiguration(t *testing.T) {
	m := &Model{}
	m.setRoot(t.TempDir())
	m.run.trusted = true
	m.run.loaded = true
	// loading keeps the widget's lazy load from asking a backend the test has.
	m.run.loading = true
	m.run.configs = []openidea.RunConfig{
		{ID: "app", Name: "Application"},
		{ID: "tests", Name: "All Tests", IsTest: true},
	}
	tt := workbenchTester(t, m)
	if !tt.HasText("Application") {
		t.Fatalf("chooser does not name the selected configuration: %v", tt.Texts())
	}
	if tt.HasText("Select Configuration") {
		t.Fatalf("chooser shows the placeholder beside a configuration")
	}
	if err := tt.Click("Application"); err != nil {
		t.Fatal(err)
	}
	menu := tt.Menu()
	for _, want := range []string{"All Tests", "Reload Run Configurations"} {
		found := false
		for _, item := range menu {
			if item == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("chooser menu %v lacks %q", menu, want)
		}
	}
	if err := tt.ChooseMenuItem("All Tests"); err != nil {
		t.Fatal(err)
	}
	if m.run.sel != 1 {
		t.Fatalf("selection = %d, want 1", m.run.sel)
	}
}

// Before the backend has answered, or without trust, the chooser shows
// IDEA's placeholder rather than an empty name.
func TestRunWidgetChooserPlaceholder(t *testing.T) {
	m := &Model{}
	m.setRoot(t.TempDir())
	tt := workbenchTester(t, m)
	if !tt.HasText("Select Configuration") {
		t.Fatalf("chooser lacks the placeholder: %v", tt.Texts())
	}
}
