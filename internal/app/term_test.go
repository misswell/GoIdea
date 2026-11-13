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

func TestTermTabsRouteByChannelAndKeepActiveInRange(t *testing.T) {
	var p TermPane
	p.add()
	p.sessions[0].channel = 7
	p.add()
	p.sessions[1].channel = 9
	p.sessions[1].id = 42
	if s := p.current(); s == nil || s.channel != 9 {
		t.Fatalf("current after add = %+v", s)
	}
	if p.byChannel(7) == nil || p.byChannel(8) != nil {
		t.Fatalf("byChannel did not pick the tab")
	}
	if s := p.byID(42); s == nil || s.channel != 9 {
		t.Fatalf("byID did not pick the tab: %+v", s)
	}
	p.remove(1)
	if len(p.sessions) != 1 || p.current().channel != 7 || p.current().id != 0 {
		t.Fatalf("remove left %+v active=%d", p.sessions, p.active)
	}
	p.remove(0)
	if p.current() != nil || len(p.sessions) != 0 {
		t.Fatalf("empty pane still has tabs: %+v", p.sessions)
	}
}

func TestTermChunksLandInTheirOwnTab(t *testing.T) {
	var m Model
	m.term.sessions = []termSession{{channel: 3}, {channel: 4}}
	m.acceptTermChunk(4, json.RawMessage(`"hi"`))
	if got := m.term.sessions[1].text(); got != "hi" {
		t.Fatalf("second tab = %q", got)
	}
	if got := m.term.sessions[0].text(); got != "" {
		t.Fatalf("first tab took the chunk: %q", got)
	}
	m.acceptTermChunk(5, json.RawMessage(`"lost"`))
	code := int32(3)
	m.term.sessions[1].id = 12
	m.acceptTermExit(12, &code)
	s := &m.term.sessions[1]
	if s.id != 0 || s.pid != nil || !s.ended || s.exitCode == nil || *s.exitCode != 3 {
		t.Fatalf("exit did not land: %+v", s)
	}
}
