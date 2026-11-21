package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestAppHelpActionsAreRegistered(t *testing.T) {
	for _, id := range []string{
		"app.about",
		"app.quit",
		"help.keymapReference",
		"help.showLog",
	} {
		if _, ok := actionGet(id); !ok {
			t.Errorf("missing action %s", id)
		}
	}
}

func TestAboutSheetUsesGoIDEABrandingAndCloses(t *testing.T) {
	m := &Model{}
	m.OpenAbout()
	tester := ui.NewTester(func(c *ui.Context) {
		m.aboutDialog(c, c.Theme())
	}, 800, 700)
	tester.Frame()
	for _, text := range []string{
		"About GoIDEA",
		"GoIDEA Development",
		"A fast, native IDE for Java and the languages around it.",
		"Keyboard Reference",
		"Show Log",
		"Report Issue",
		"Copy Info",
		"Close",
	} {
		if !tester.HasText(text) {
			t.Errorf("About is missing %q", text)
		}
	}
	m.closeAbout()
	if m.aboutOpen {
		t.Fatal("About stayed open")
	}
}

func TestKeymapReferenceGroupsFiltersAndRenders(t *testing.T) {
	m := &Model{}
	all := m.keymapReferenceGroups("")
	if len(all) == 0 {
		t.Fatal("Keyboard Reference has no groups")
	}
	seen := map[string]bool{}
	count := 0
	for _, group := range all {
		for _, entry := range group.entries {
			if seen[entry.id] {
				t.Fatalf("action %s appears twice", entry.id)
			}
			seen[entry.id] = true
			count++
		}
	}
	expected := 0
	for _, action := range actionList() {
		if !hiddenActionIDs[action.ID] && len(m.KeymapBindings(action.ID)) > 0 {
			expected++
		}
	}
	if count != expected {
		t.Fatalf("Keyboard Reference has %d shortcuts, want %d", count, expected)
	}

	groups := m.keymapReferenceGroups("navigate.back")
	if len(groups) != 1 || groups[0].category != "Navigate" ||
		len(groups[0].entries) != 1 || groups[0].entries[0].id != "navigate.back" {
		t.Fatalf("filtered groups = %+v", groups)
	}

	m.OpenKeymapReference()
	m.keymapReference.query = "navigate.back"
	tester := ui.NewTester(func(c *ui.Context) {
		m.keymapReferenceDialog(c, c.Theme())
	}, 900, 760)
	tester.Frame()
	for _, text := range []string{"Keyboard Reference", "Navigate", "Back", "Cmd+[", "Cancel"} {
		if text == "Cancel" {
			text = "Close"
		}
		if !tester.HasText(text) {
			t.Errorf("Keyboard Reference is missing %q", text)
		}
	}
	m.closeKeymapReference()
	if m.keymapReference.open || m.keymapReference.query != "" {
		t.Fatalf("Keyboard Reference state = %+v", m.keymapReference)
	}
}

func TestAppLogPathUsesTheConfiguredLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "goidea.log")
	log, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	m := &Model{cfg: Config{Log: log}}
	if got := m.appLogPath(); got != path {
		t.Fatalf("log path = %q, want %q", got, path)
	}
}
