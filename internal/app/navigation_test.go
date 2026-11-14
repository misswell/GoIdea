package app

import (
	"testing"

	"mayoi/internal/openidea"
)

func TestIdentifierAt(t *testing.T) {
	text := "class App implements Greeter { void run() {} }"
	at := len("class App implements Gre") // inside Greeter
	if got := identifierAt(text, at); got != "Greeter" {
		t.Fatalf("identifier = %q", got)
	}
	if got := identifierAt(text, 0); got != "class" {
		t.Fatalf("start = %q", got)
	}
	if got := identifierAt("a.b(c)", 1); got != "a" {
		t.Fatalf("punctuation = %q", got)
	}
	if got := identifierAt("", 0); got != "" {
		t.Fatalf("empty = %q", got)
	}
}

func TestImplTargetsDeduplicate(t *testing.T) {
	loc := func(line uint32) openidea.Location {
		return openidea.Location{
			Path: "/p/A.java",
			Range: openidea.OutlineRange{
				Start: openidea.OutlinePos{Line: line},
				End:   openidea.OutlinePos{Line: line, Col: 5},
			},
		}
	}
	m := &Model{}
	m.addImplTargets([]openidea.Location{loc(1), loc(2), loc(1)})
	if len(m.impl.items) != 2 {
		t.Fatalf("items = %d", len(m.impl.items))
	}
	m.addImplTargets([]openidea.Location{loc(2)})
	if len(m.impl.items) != 2 {
		t.Fatalf("re-add = %d", len(m.impl.items))
	}
}

func TestImplementationSelectionWraps(t *testing.T) {
	m := &Model{}
	m.impl.items = []implTarget{{path: "/p/A.java"}, {path: "/p/B.java"}}
	m.impl.sel = 0
	m.stepImplementation(1)
	if m.impl.sel != 1 {
		t.Fatalf("down = %d", m.impl.sel)
	}
	m.stepImplementation(1)
	if m.impl.sel != 0 {
		t.Fatalf("wrap = %d", m.impl.sel)
	}
	m.stepImplementation(-1)
	if m.impl.sel != 1 {
		t.Fatalf("up wrap = %d", m.impl.sel)
	}
}

func TestImplTargetLabel(t *testing.T) {
	it := implTarget{path: "/p/app/A.java", rng: openidea.OutlineRange{
		Start: openidea.OutlinePos{Line: 4},
	}}
	if got := implTargetLabel("/p", it); got != "app/A.java:5" {
		t.Fatalf("label = %q", got)
	}
	if got := implTargetLabel("", it); got != "/p/app/A.java:5" {
		t.Fatalf("outside label = %q", got)
	}
}

func TestImplementationTitle(t *testing.T) {
	m := &Model{}
	m.impl.items = []implTarget{{path: "/p/A.java"}}
	m.impl.name = "greet"
	if got := m.implementationTitle(); got != "选择实现：greet（1）" {
		t.Fatalf("title = %q", got)
	}
	m.impl.name = ""
	if got := m.implementationTitle(); got != "选择实现（1）" {
		t.Fatalf("plain title = %q", got)
	}
}
