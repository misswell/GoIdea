package app

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

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
	tt := ui.NewTester(func(c *ui.Context) { m.titleBar(c, c.Theme()) }, 1440, 40)
	tt.Frame()
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
	tt := ui.NewTester(func(c *ui.Context) { m.titleBar(c, c.Theme()) }, 1440, 40)
	tt.Frame()
	if !tt.HasText("Select Configuration") {
		t.Fatalf("chooser lacks the placeholder: %v", tt.Texts())
	}
}

func TestRunWidgetDisablesRunForRemoteDebugConfigurations(t *testing.T) {
	m := &Model{}
	m.setRoot(t.TempDir())
	m.run.trusted = true
	m.run.loaded = true
	m.run.loading = true
	m.run.configs = []openidea.RunConfig{{
		ID: "remote", Name: "Remote JVM", Kind: "remote-jvm",
		Params: map[string]any{"host": "localhost", "port": "5005"},
	}}
	tt := ui.NewTester(func(c *ui.Context) { m.titleBar(c, c.Theme()) }, 1440, 40)
	tt.Frame()
	if !tt.HasText("Remote JVM") {
		t.Fatalf("chooser does not name the remote configuration: %v", tt.Texts())
	}
	if tt.HasText("Run 'Remote JVM'") || !tt.HasText("'Remote JVM' can only be debugged") {
		t.Fatalf("remote debug tooltip is wrong: %v", tt.Texts())
	}
	if err := tt.Click("'Remote JVM' can only be debugged"); err == nil {
		// A disabled button may be found but must not launch anything.
		if m.run.pendingStart || m.run.running || m.run.awaiting {
			t.Fatal("a click started a remote configuration with Run")
		}
	}
}

func TestRunWidgetShowsStartingStateAndKeepsStopAvailable(t *testing.T) {
	m := &Model{}
	m.setRoot(t.TempDir())
	m.run.trusted = true
	m.run.loaded = true
	m.run.loading = true
	m.run.configs = []openidea.RunConfig{{ID: "app", Name: "Application", Kind: "application"}}
	m.run.lastID = "app"
	m.run.busy = true
	m.run.awaiting = true
	tt := ui.NewTester(func(c *ui.Context) { m.titleBar(c, c.Theme()) }, 1440, 40)
	tt.Frame()
	if !tt.HasText("Application") {
		t.Fatalf("Run widget did not render in the production title bar: %v", tt.Texts())
	}
	if !tt.HasText("Starting Run") {
		t.Fatalf("Run startup has no spinner label: %v", tt.Texts())
	}
	if !tt.HasText("Stop") {
		t.Fatalf("Stop disappeared during startup: %v", tt.Texts())
	}
}

func TestRunWidgetDoesNotApplyAProcessToADifferentSelection(t *testing.T) {
	m := &Model{}
	m.setRoot(t.TempDir())
	m.run.trusted = true
	m.run.loaded = true
	m.run.loading = true
	m.run.configs = []openidea.RunConfig{
		{ID: "app", Name: "Application", Kind: "application"},
		{ID: "tests", Name: "Tests", Kind: "maven", IsTest: true},
	}
	m.run.lastID = "app"
	m.run.running = true
	m.run.sel = 1
	tt := ui.NewTester(func(c *ui.Context) { m.titleBar(c, c.Theme()) }, 1440, 40)
	tt.Frame()
	texts := strings.Join(tt.Texts(), "\n")
	if !tt.HasText("Tests") {
		t.Fatalf("selected configuration did not render in the title bar: %s", texts)
	}
	if strings.Contains(texts, "Rerun 'Tests'") || strings.Contains(texts, "Stop 'Tests'") {
		t.Fatalf("another configuration's process was attributed to Tests: %s", texts)
	}
	if !tt.HasText("Run 'Tests'") || !tt.HasText("Debug 'Tests'") {
		t.Fatalf("the idle selected configuration lost its controls: %s", texts)
	}
}
