package app

import (
	"reflect"
	"testing"

	"mayoi/internal/openidea"
)

func TestSectionsForMatchesOpenIDEA(t *testing.T) {
	cases := []struct {
		tab   seTab
		empty bool
		want  []seSection
	}{
		{seTabAll, true, []seSection{seRecentFiles, seRecentActions}},
		{seTabFiles, true, []seSection{seRecentFiles}},
		{seTabActions, true, []seSection{seRecentActions}},
		{seTabClasses, true, nil},
		{seTabSymbols, true, nil},
		{seTabText, true, nil},
		{seTabAll, false, []seSection{seClasses, seFiles, seSymbols, seActions, seText}},
		{seTabClasses, false, []seSection{seClasses}},
		{seTabText, false, []seSection{seText}},
	}
	for _, c := range cases {
		got := sectionsFor(c.tab, c.empty)
		if !reflect.DeepEqual(got, c.want) {
			t.Fatalf("sectionsFor(%q, %v) = %v, want %v", c.tab, c.empty, got, c.want)
		}
	}
}

func TestSearchEverywhereTabCyclesAndWraps(t *testing.T) {
	m := &Model{}
	m.OpenSearchEverywhere()
	if m.se == nil || m.se.tab != seTabAll {
		t.Fatalf("the overlay did not open on All: %+v", m.se)
	}
	m.CycleSearchEverywhereTab(1)
	if m.se.tab != seTabClasses {
		t.Fatalf("Tab went to %q, want Classes", m.se.tab)
	}
	m.CycleSearchEverywhereTab(-1)
	if m.se.tab != seTabAll {
		t.Fatalf("Shift+Tab came back to %q, want All", m.se.tab)
	}
	m.CycleSearchEverywhereTab(-1)
	if m.se.tab != seTabText {
		t.Fatalf("wrapping back went to %q, want Text", m.se.tab)
	}
	m.SetSearchEverywhereTab(seTabActions)
	if m.se.tab != seTabActions {
		t.Fatalf("SetSearchEverywhereTab left %q", m.se.tab)
	}
	m.CloseSearchEverywhere()
	if m.se != nil {
		t.Fatalf("the overlay stayed open")
	}
}

func TestRecentActionsKeepNewestFirstAndDedupe(t *testing.T) {
	m := &Model{}
	m.RunCommand("file.reloadAllFromDisk")
	m.RunCommand("file.save")
	m.RunCommand("file.reloadAllFromDisk")
	want := []string{"file.reloadAllFromDisk", "file.save"}
	if !reflect.DeepEqual(m.recentActions, want) {
		t.Fatalf("recent actions = %v, want %v", m.recentActions, want)
	}
	for i := 0; i < seRecentActionCap+5; i++ {
		m.rememberRecentAction("test.action" + string(rune('a'+i)))
	}
	if len(m.recentActions) != seRecentActionCap {
		t.Fatalf("recent actions grew to %d, want %d", len(m.recentActions), seRecentActionCap)
	}
	if m.recentActions[0] != "test.action"+string(rune('a'+seRecentActionCap+4)) {
		t.Fatalf("the newest action is not first: %v", m.recentActions)
	}
}

func TestSearchEverywhereRowsCarrySectionHeadersAndLimits(t *testing.T) {
	m := &Model{se: &sePane{
		open:  true,
		tab:   seTabAll,
		extra: map[seSection]int{},
		files: make([]openidea.FileMatch, 40),
	}}
	for i := range m.se.files {
		m.se.files[i].Path = "/p/File" + string(rune('A'+i)) + ".java"
		m.se.files[i].RelPath = "src/File" + string(rune('A'+i)) + ".java"
	}
	rows := m.seRows()
	if len(rows) == 0 || !rows[0].header || rows[0].section != seRecentFiles {
		t.Fatalf("the first row is not the Recent Files header: %+v", rows)
	}
	items, more := 0, false
	for _, r := range rows {
		if r.more {
			more = true
		} else if seSelectable(r) {
			items++
		}
	}
	if items != seRecentLimit {
		t.Fatalf("the empty All query listed %d files, want %d", items, seRecentLimit)
	}
	if !more {
		t.Fatalf("a longer list has no More row")
	}
	// More widens the section by the All step.
	m.se.extra[seRecentFiles] = seMoreStepAll
	rows = m.seRows()
	items = 0
	for _, r := range rows {
		if seSelectable(r) && !r.more {
			items++
		}
	}
	if items != seRecentLimit+seMoreStepAll {
		t.Fatalf("More listed %d files, want %d", items, seRecentLimit+seMoreStepAll)
	}
}

func TestSearchEverywhereActionsFilter(t *testing.T) {
	m := &Model{se: &sePane{
		open:  true,
		tab:   seTabActions,
		query: "reload",
		extra: map[seSection]int{},
	}}
	rows := m.seRows()
	found := false
	for _, r := range rows {
		if r.item.kind == "action" && r.item.action == "file.reloadAllFromDisk" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the Actions tab did not list Reload All from Disk: %+v", rows)
	}
	for _, r := range rows {
		if r.item.kind == "action" && r.item.action == "project.new" {
			t.Fatalf("an unmatched action was listed")
		}
	}
}

func TestClassesTabKeepsOnlyTypeKinds(t *testing.T) {
	m := &Model{se: &sePane{
		open:  true,
		tab:   seTabClasses,
		query: "App",
		extra: map[seSection]int{},
		symbols: []openidea.WorkspaceSymbol{
			{Name: "App", Kind: "class"},
			{Name: "run", Kind: "method"},
			{Name: "Greeter", Kind: "interface"},
		},
	}}
	names := map[string]bool{}
	for _, r := range m.seRows() {
		if seSelectable(r) {
			names[r.item.title] = true
		}
	}
	if !names["App"] || !names["Greeter"] || names["run"] {
		t.Fatalf("the Classes tab listed %v", names)
	}
	if isTypeKind("method") || !isTypeKind("enum") {
		t.Fatalf("isTypeKind disagrees with OpenIDEA's class kinds")
	}
}

func TestSearchEverywhereSelectionSkipsHeaders(t *testing.T) {
	m := &Model{se: &sePane{
		open:    true,
		tab:     seTabAll,
		sel:     0,
		extra:   map[seSection]int{},
		files:   []openidea.FileMatch{{Path: "/p/A.java", RelPath: "A.java"}},
		symbols: []openidea.WorkspaceSymbol{{Name: "App", Kind: "class", Location: openidea.Location{Path: "/p/A.java"}}},
	}}
	m.recentActions = []string{"file.save"}
	rows := m.seRows()
	// The first selectable row follows the first header.
	m.stepSearchEverywhere(1)
	if m.se.sel < 0 || m.se.sel >= len(rows) || rows[m.se.sel].header {
		t.Fatalf("the selection landed on a header: %d", m.se.sel)
	}
	m.stepSearchEverywhere(1)
	if m.se.sel >= len(rows) || rows[m.se.sel].header {
		t.Fatalf("stepping past a header failed: %d", m.se.sel)
	}
	m.stepSearchEverywhere(-1)
	if m.se.sel <= 0 || rows[m.se.sel].header {
		t.Fatalf("stepping back landed on a header: %d", m.se.sel)
	}
}
