package app

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestRunningAnUntrustedProjectAsksToTrust(t *testing.T) {
	m := &Model{}
	m.setRoot(t.TempDir())
	m.run.trusted = false
	m.Run()
	if !m.trustAsk || !m.run.pendingTrust {
		t.Fatalf("Run on an untrusted project did not ask: ask=%v pending=%v", m.trustAsk, m.run.pendingTrust)
	}
	if m.status == "" {
		t.Fatal("the refusal was silent")
	}
	o := overlays.byID["project.trustDialog"]
	if o == nil || !o.Open(m) || !o.Modal {
		t.Fatal("the Trust Project prompt is not a modal overlay")
	}
	// Cancel drops the queued run.
	m.trustAsk = false
	m.run.pendingTrust = false
	if o.Open(m) {
		t.Fatal("the prompt stayed after Cancel")
	}
}

func TestOpeningAnUntrustedProjectAsksOnce(t *testing.T) {
	m := &Model{}
	m.setRoot(t.TempDir())
	m.run.trusted = false
	// checkTrust's main-thread half: an untrusted project opens the prompt.
	m.trustAsk = true
	if !overlays.byID["project.trustDialog"].Open(m) {
		t.Fatal("an untrusted project does not ask to be trusted")
	}
}

func TestProjectTrustCompatibilityActions(t *testing.T) {
	for _, id := range []string{
		"project.trust",
		"project.previewRestricted",
		"project.dontOpen",
		"project.revokeTrust",
		"project.removeTrustedLocation",
	} {
		if _, ok := Command(id); !ok {
			t.Errorf("missing OpenIDEA project-trust action %q", id)
		}
	}
	if got := canonicalActionID("run.trust"); got != "project.trust" {
		t.Fatalf("run.trust = %q", got)
	}

	m := &Model{}
	m.trustAsk = true
	m.trustFolder = true
	m.run.pendingTrust = true
	m.run.pendingCoverage = true
	m.PreviewProjectRestricted()
	if m.trustAsk || m.trustFolder || m.run.pendingTrust || m.run.pendingCoverage {
		t.Fatal("Restricted Mode kept the trust prompt or queued execution")
	}
}

func TestProjectTrustPromptMatchesOpenIDEA(t *testing.T) {
	root := filepath.Join(t.TempDir(), "untrusted project")
	m := &Model{}
	m.setRoot(root)
	m.trustAsk = true
	tester := ui.NewTester(func(c *ui.Context) {
		m.trustDialog(c, c.Theme())
	}, 900, 600)
	tester.Frame()
	for _, text := range []string{
		"Trust and Open Project?",
		root,
		"If you don't trust the source, preview the project in Restricted Mode, where project commands and tools can't run.",
		"Loading, running or building a project may execute potentially malicious code from its build scripts.",
		"Trust projects in " + filepath.Dir(root),
		"Preview in Restricted Mode",
		"Don't Open",
		"Trust Project",
	} {
		if !tester.HasText(text) {
			t.Fatalf("trust dialog is missing %q: %v", text, tester.Texts())
		}
	}
}

func TestProjectTrustPreviewClearsFolderScope(t *testing.T) {
	m := &Model{}
	m.trustAsk = true
	m.trustFolder = true
	m.run.pendingTrust = true
	m.run.pendingCoverage = true
	m.PreviewProjectRestricted()
	if m.trustAsk || m.trustFolder || m.run.pendingTrust || m.run.pendingCoverage {
		t.Fatal("Restricted Mode kept the prompt, folder scope or queued execution")
	}
}

func TestProjectTrustDialogCancelKeepsTheProjectClosed(t *testing.T) {
	m := &Model{}
	m.setRoot(filepath.Join(t.TempDir(), "rejected"))
	m.trustAsk = true
	m.trustFolder = true
	m.DontOpenProject()
	if m.trustAsk || m.trustFolder {
		t.Fatal("Don't Open kept the trust prompt")
	}
	if m.Root() != "" || !m.showingWelcome {
		t.Fatalf("Don't Open left root=%q welcome=%v", m.Root(), m.showingWelcome)
	}
}

func TestStartupPhasesReport(t *testing.T) {
	m := &Model{}
	m.setStartPhase("server")
	if m.Phase() != "server" {
		t.Fatalf("phase = %q", m.Phase())
	}
	ids := statusIDs(m.statusSide("left"))
	if indexOf(ids, "status.startup") == -1 {
		t.Fatal("a starting app does not show its phase")
	}
	m.setStartPhase("ready")
	if indexOf(statusIDs(m.statusSide("left")), "status.startup") != -1 {
		t.Fatal("a ready app still shows a startup phase")
	}
}
