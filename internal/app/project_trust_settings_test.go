package app

import (
	"encoding/json"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestProjectTrustSettingsPageMatchesOpenIDEA(t *testing.T) {
	page, ok := settingsPageByID(settingsPageProjectTrust)
	if !ok {
		t.Fatal("Project Trust page is not registered")
	}
	if page.title != "Project Trust" || page.group != "Security" {
		t.Fatalf("Project Trust page identity = %q in %q", page.title, page.group)
	}

	m := &Model{}
	m.projectTrust = projectTrustSettingsState{
		loaded: true,
		decisions: projectTrustDecisions{
			Projects:  []string{"/work/project"},
			Locations: []string{"/work"},
		},
	}
	tester := ui.NewTester(func(c *ui.Context) {
		m.settingsProjectTrustBody(c, oiPaletteOf(c.Theme()))
	}, 900, 600)
	tester.Frame()
	for _, text := range []string{
		"Trust decisions are saved in your user data and take effect immediately.",
		"Trusted projects", "/work/project", "Revoke Trust",
		"Trusted locations", "Add folder…",
		"Every project inside a trusted location is trusted without asking.",
		"/work", "Remove",
	} {
		if !tester.HasText(text) {
			t.Fatalf("Project Trust page is missing %q: %v", text, tester.Texts())
		}
	}
}

func TestProjectTrustDecisionsDecodeBackendShape(t *testing.T) {
	var decisions projectTrustDecisions
	if err := json.Unmarshal([]byte(`{
		"projects":["/trusted/app"],
		"locations":["/trusted"],
		"restricted":["/untrusted/app"]
	}`), &decisions); err != nil {
		t.Fatal(err)
	}
	if len(decisions.Projects) != 1 || decisions.Projects[0] != "/trusted/app" ||
		len(decisions.Locations) != 1 || decisions.Locations[0] != "/trusted" ||
		len(decisions.Restricted) != 1 || decisions.Restricted[0] != "/untrusted/app" {
		t.Fatalf("decoded decisions = %+v", decisions)
	}
}
