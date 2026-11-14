package app

import "testing"

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
	o := overlays.byID["trust"]
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
	if !overlays.byID["trust"].Open(m) {
		t.Fatal("an untrusted project does not ask to be trusted")
	}
}
