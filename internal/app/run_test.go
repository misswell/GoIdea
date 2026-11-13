package app

import (
	"encoding/json"
	"strings"
	"testing"

	"mayoi/internal/openidea"
)

func TestRunPaneAppendKeepsOnlyTheTail(t *testing.T) {
	p := RunPane{}
	var b strings.Builder
	for i := 0; i < maxRunLines+50; i++ {
		b.WriteString("line\n")
	}
	p.append(openidea.OutputChunk{Kind: "stdout", Text: b.String()})
	if len(p.output) != maxRunLines || !p.truncated {
		t.Fatalf("kept %d lines, truncated %v", len(p.output), p.truncated)
	}
	if p.output[len(p.output)-1].text != "line" {
		t.Fatalf("last line %q", p.output[len(p.output)-1].text)
	}
}

func TestRunPaneAppendSplitsChunksAndKinds(t *testing.T) {
	p := RunPane{}
	p.append(openidea.OutputChunk{Kind: "stderr", Text: "boom\nwarn\n"})
	if len(p.output) != 2 || p.output[0].kind != "stderr" || p.output[1].text != "warn" {
		t.Fatalf("output %+v", p.output)
	}
	if p.renderText() != "boom\nwarn" {
		t.Fatalf("render %q", p.renderText())
	}
}

func TestStripANSIColors(t *testing.T) {
	got := stripANSI("\x1b[1;34mINFO\x1b[m --- \x1b[0;32mbuild\x1b[m")
	if got != "INFO --- build" {
		t.Fatalf("stripped %q", got)
	}
	if stripANSI("plain") != "plain" {
		t.Fatal("plain text changed")
	}
}

func TestAcceptExitMatchesTheStartedRun(t *testing.T) {
	m := &Model{run: RunPane{awaiting: true, runID: 7, running: true}}
	code := int32(0)
	other := int32(1)
	m.acceptExit(openidea.RunExitedEvent{Run: 8, Code: &other})
	if !m.run.awaiting || !m.run.running {
		t.Fatal("an exit for another run stopped this one")
	}
	m.acceptExit(openidea.RunExitedEvent{Run: 7, Code: &code})
	if m.run.awaiting || m.run.running || m.run.exitCode == nil || *m.run.exitCode != 0 {
		t.Fatalf("exit not applied: %+v", m.run)
	}
}

func TestAcceptChunkIgnoresOtherChannels(t *testing.T) {
	m := &Model{run: RunPane{channel: 3}}
	m.acceptChunk(4, json.RawMessage(`{"kind":"stdout","text":"stolen"}`))
	if len(m.run.output) != 0 {
		t.Fatal("output from another channel was kept")
	}
	m.acceptChunk(3, json.RawMessage(`{"kind":"stdout","text":"mine"}`))
	if len(m.run.output) != 1 || m.run.output[0].text != "mine" {
		t.Fatalf("output %+v", m.run.output)
	}
}

func TestPickRunConfigSkipsTests(t *testing.T) {
	configs := []openidea.RunConfig{{Name: "mvn test", IsTest: true}, {Name: "mvn package"}}
	if got := pickRunConfig(configs); got != 1 {
		t.Fatalf("picked %d", got)
	}
	if got := pickRunConfig(nil); got != 0 {
		t.Fatalf("empty picked %d", got)
	}
}

func TestPickRunConfigPrefersTheApplication(t *testing.T) {
	java := "java"
	configs := []openidea.RunConfig{
		{Name: "mvn package", Kind: "maven"},
		{Name: "App", Kind: "java", DebugAdapter: &java},
		{Name: "CalculatorTest", Kind: "junit", IsTest: true},
	}
	if got := pickRunConfig(configs); got != 1 {
		t.Fatalf("picked %d, want the Java application", got)
	}
	gradle := []openidea.RunConfig{
		{Name: "gradle build", Kind: "gradle"},
		{Name: "App", Kind: "application"},
	}
	if got := pickRunConfig(gradle); got != 1 {
		t.Fatalf("picked %d, want the application", got)
	}
}
