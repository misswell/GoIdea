package app

import (
	"strings"
	"testing"

	"mayoi/internal/openidea"
)

func TestWordAt(t *testing.T) {
	text := "class Foo { int count = 1; }"
	cases := []struct {
		name string
		off  int
		want string
	}{
		{"inside", 6, "Foo"},
		{"caret after word", 9, "Foo"},
		{"second identifier", 17, "count"},
		{"punctuation", 22, ""},
		{"end of text", len(text), ""},
		{"negative", -3, "class"},
		{"past end", 999, ""},
	}
	for _, c := range cases {
		if got := wordAt(text, c.off); got != c.want {
			t.Errorf("%s: wordAt(%q, %d) = %q, want %q", c.name, text, c.off, got, c.want)
		}
	}
}

func TestWordAtUnicodeAndDollar(t *testing.T) {
	text := "int 数量 = $total;"
	if got := wordAt(text, 4); got != "数量" {
		t.Errorf("unicode: got %q", got)
	}
	if got := wordAt(text, 15); got != "$total" {
		t.Errorf("dollar: got %q", got)
	}
}

func TestGroupUsages(t *testing.T) {
	locs := []openidea.Location{
		{Path: "/p/A.java"},
		{Path: "/p/B.java"},
		{Path: "/p/A.java"},
	}
	files := groupUsages(locs)
	if len(files) != 2 {
		t.Fatalf("files = %d, want 2", len(files))
	}
	if files[0].path != "/p/A.java" || len(files[0].locs) != 2 {
		t.Errorf("first group = %q with %d locations", files[0].path, len(files[0].locs))
	}
	if files[1].path != "/p/B.java" || len(files[1].locs) != 1 {
		t.Errorf("second group = %q with %d locations", files[1].path, len(files[1].locs))
	}
	if got := groupUsages(nil); len(got) != 0 {
		t.Errorf("nil locations: %v", got)
	}
}

func TestSetUsagesKeepsListBounded(t *testing.T) {
	m := &Model{}
	kind := "indexing"
	refs := &openidea.References{
		Locations:        make([]openidea.Location, maxUsages+5),
		Incomplete:       &kind,
		LibrariesSkipped: true,
	}
	m.setUsages(refs)
	if n := len(m.usages.locs); n != maxUsages {
		t.Errorf("locations = %d, want %d", n, maxUsages)
	}
	if !m.usages.truncated {
		t.Error("truncated = false")
	}
	if !strings.Contains(m.usages.incomplete, "索引") {
		t.Errorf("incomplete = %q", m.usages.incomplete)
	}
	if !m.usages.librariesSkipped {
		t.Error("librariesSkipped = false")
	}

	m.setUsages(&openidea.References{Locations: make([]openidea.Location, 2)})
	if m.usages.truncated || m.usages.incomplete != "" || m.usages.librariesSkipped {
		t.Errorf("small snapshot kept stale flags: %+v", m.usages)
	}
}

func TestIncompleteText(t *testing.T) {
	cases := map[string]string{
		"indexing":     "索引",
		"tooManyFiles": "过多",
		"filesSkipped": "未检查",
		"serverFailed": "失败",
		"somethingNew": "somethingNew",
		"":             "",
	}
	for kind, want := range cases {
		if got := incompleteText(kind); !strings.Contains(got, want) {
			t.Errorf("incompleteText(%q) = %q, want it to contain %q", kind, got, want)
		}
	}
}
