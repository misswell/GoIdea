package app

import (
	"encoding/json"
	"testing"
)

func TestTermWriteKeepsTheLineDiscipline(t *testing.T) {
	var s termSession
	s.write("one\r\ntwo\rthree\n")
	if got := s.text(); got != "one\nthree\n" {
		t.Fatalf("scrollback = %q", got)
	}
	s.write("four")
	if got := s.text(); got != "one\nthree\nfour" {
		t.Fatalf("scrollback after append = %q", got)
	}
}

func TestTermHitsIgnoreCaseInTheScrollback(t *testing.T) {
	lines := []string{"one", "", "Match ME", "two"}
	hits := termHits(lines, "match")
	if len(hits) != 1 || hits[0] != 2 {
		t.Fatalf("hits = %v", hits)
	}
	if hits := termHits(lines, ""); hits != nil {
		t.Fatalf("empty query matched %v", hits)
	}
	if hits := termHits(lines, "absent"); len(hits) != 0 {
		t.Fatalf("absent query matched %v", hits)
	}
}

func TestTermTabsRouteByChannelAndKeepActiveInRange(t *testing.T) {
	var p TermPane
	p.add(0).channel = 7
	p.add(0).channel = 9
	p.groups[0].tabs[1].id = 42
	if s := p.current(); s == nil || s.channel != 9 {
		t.Fatalf("current after add = %+v", s)
	}
	if p.byChannel(7) == nil || p.byChannel(8) != nil {
		t.Fatalf("byChannel did not pick the tab")
	}
	if s := p.byID(42); s == nil || s.channel != 9 {
		t.Fatalf("byID did not pick the tab: %+v", s)
	}
	p.remove(0, 1)
	if p.total() != 1 || p.current().channel != 7 || p.current().id != 0 {
		t.Fatalf("remove left %v active=%d", p.groups, p.active)
	}
	p.remove(0, 0)
	if p.current() != nil || p.total() != 0 {
		t.Fatalf("empty pane still has tabs: %+v", p.groups)
	}
}

func TestTermSplitKeepsBothGroupsAndMergesBack(t *testing.T) {
	var p TermPane
	p.add(0).channel = 7
	p.group(1)
	p.split = "right"
	p.add(1).channel = 9
	if p.active != 1 || p.split != "right" || p.total() != 2 {
		t.Fatalf("split = %q groups=%d active=%d", p.split, len(p.groups), p.active)
	}
	if s := p.byChannel(9); s == nil || s.channel != 9 {
		t.Fatalf("split tab not found by channel")
	}
	// An emptied split group merges away.
	p.remove(1, 0)
	if p.split != "" || len(p.groups) != 1 || p.total() != 1 || p.active != 0 {
		t.Fatalf("empty split did not merge: split=%q groups=%d", p.split, len(p.groups))
	}
}

func TestTermChunksLandInTheirOwnTab(t *testing.T) {
	var m Model
	m.term.add(0).channel = 3
	m.term.add(0).channel = 4
	m.acceptTermChunk(4, json.RawMessage(`"hi"`))
	if got := m.term.groups[0].tabs[1].text(); got != "hi" {
		t.Fatalf("second tab = %q", got)
	}
	if got := m.term.groups[0].tabs[0].text(); got != "" {
		t.Fatalf("first tab took the chunk: %q", got)
	}
	m.acceptTermChunk(5, json.RawMessage(`"lost"`))
	code := int32(3)
	m.term.groups[0].tabs[1].id = 12
	m.acceptTermExit(12, &code)
	s := &m.term.groups[0].tabs[1]
	if s.id != 0 || s.pid != nil || !s.ended || s.exitCode == nil || *s.exitCode != 3 {
		t.Fatalf("exit did not land: %+v", s)
	}
}

func TestTermShellLabelsDisambiguate(t *testing.T) {
	shells := []string{"/bin/zsh", "/usr/local/bin/zsh", "/bin/bash"}
	if got := termShellLabel(shells, 0); got != "zsh (/bin/zsh)" {
		t.Fatalf("duplicate label = %q", got)
	}
	if got := termShellLabel(shells, 2); got != "bash" {
		t.Fatalf("unique label = %q", got)
	}
}

func TestTermShellSelectionWraps(t *testing.T) {
	m := &Model{termShell: &termShellPicker{open: true, shells: []string{"a", "b"}}}
	m.StepTermShell(1)
	if m.termShell.sel != 1 {
		t.Fatalf("down = %d", m.termShell.sel)
	}
	m.StepTermShell(1)
	if m.termShell.sel != 0 {
		t.Fatalf("wrap = %d", m.termShell.sel)
	}
	m.StepTermShell(-1)
	if m.termShell.sel != 1 {
		t.Fatalf("up wrap = %d", m.termShell.sel)
	}
}
