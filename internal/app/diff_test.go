package app

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
	"mayoi/internal/textdiff"
)

func TestShowDiffNeedsAProject(t *testing.T) {
	m := &Model{}
	m.ShowDiff("/tmp/x")
	if m.diff != nil {
		t.Fatalf("compare view opened without a project")
	}
}

func TestCloseDiffDropsView(t *testing.T) {
	m := &Model{diff: &diffView{open: true}}
	m.closeDiff()
	if m.diff != nil {
		t.Fatalf("compare view stayed open")
	}
}

func TestDiffErrorTextNamesMissingGit(t *testing.T) {
	err := &openidea.Error{Kind: "unavailable", Message: "not a git repository"}
	if got := diffErrorText(err); got != "The current project is not a Git repository" {
		t.Fatalf("diffErrorText = %q", got)
	}
	if got := diffErrorText(textdiffError{}); !strings.HasPrefix(got, "Could not read the diff: ") {
		t.Fatalf("diffErrorText = %q", got)
	}
}

type textdiffError struct{}

func (textdiffError) Error() string { return "boom" }

func TestDiffRowText(t *testing.T) {
	row := textdiff.Row{Kind: textdiff.Add, Old: 4, New: 5, Text: "x"}
	if got := diffRowText(row); got != "+ 4:5 x" {
		t.Fatalf("diffRowText = %q", got)
	}
}

func TestDiffRowsCountsBothSides(t *testing.T) {
	rows, added, gone := diffRows("a\nb\n", "a\nc\n")
	if len(rows) == 0 || added != 1 || gone != 1 {
		t.Fatalf("diffRows = %d rows, +%d -%d", len(rows), added, gone)
	}
	rows, added, gone = diffRows("same\n", "same\n")
	if len(rows) != 0 || added != 0 || gone != 0 {
		t.Fatalf("equal texts = %d rows, +%d -%d", len(rows), added, gone)
	}
}

func TestDiffTitleNamesEachCompare(t *testing.T) {
	cases := []struct {
		d    diffView
		want string
	}{
		{diffView{path: "/p/Main.java"}, "Main.java · Compared with HEAD"},
		{diffView{path: "/p/Main.java", other: "/p/Util.java"}, "Main.java ↔ Util.java"},
		{diffView{path: "/p/Main.java", inline: true, label: "Clipboard"}, "Main.java ↔ Clipboard"},
		{diffView{path: "/p/Main.java", loading: true}, "Main.java · Comparing…"},
	}
	for _, c := range cases {
		if got := diffTitle(&c.d); got != c.want {
			t.Fatalf("diffTitle(%+v) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestCompareWithoutATabOpensNothing(t *testing.T) {
	m := &Model{}
	m.CompareTabWithClipboard(nil)
	m.PickCompareFile(nil)
	m.ShowTextCompare("", "a", "Clipboard", "b")
	if m.diff != nil {
		t.Fatalf("compare view opened without a tab")
	}
}

func TestDiffChangesAndApplyBothSides(t *testing.T) {
	m := &Model{}
	d := &diffView{
		open: true, leftText: "same\nold\nend\n", rightText: "same\nnew\nend\n",
	}
	d.setDiffRows(d.leftText, d.rightText)
	if len(d.changes) != 1 {
		t.Fatalf("changes = %+v", d.changes)
	}
	change := d.changes[0]
	if change.oldStart != 1 || change.oldEnd != 2 || change.newStart != 1 || change.newEnd != 2 {
		t.Fatalf("change = %+v", change)
	}
	m.diff = d
	m.ApplyDiffSide(true)
	if d.rightText != d.leftText {
		t.Fatalf("apply left = %q, want %q", d.rightText, d.leftText)
	}
	d = &diffView{
		open: true, leftText: "same\nold\nend\n", rightText: "same\nnew\nend\n",
	}
	d.setDiffRows(d.leftText, d.rightText)
	m.diff = d
	m.ApplyDiffSide(false)
	if d.leftText != d.rightText {
		t.Fatalf("apply right = %q, want %q", d.leftText, d.rightText)
	}
}

func TestDiffNavigationWraps(t *testing.T) {
	m := &Model{diff: &diffView{
		changes: []diffChange{{oldStart: 0, oldEnd: 1, newStart: 0, newEnd: 1},
			{oldStart: 4, oldEnd: 5, newStart: 4, newEnd: 5}},
	}}
	m.StepDiffChange(1)
	if m.diff.selected != 1 {
		t.Fatalf("next selection = %d", m.diff.selected)
	}
	m.StepDiffChange(1)
	if m.diff.selected != 0 {
		t.Fatalf("wrapped selection = %d", m.diff.selected)
	}
	m.StepDiffChange(-1)
	if m.diff.selected != 1 {
		t.Fatalf("previous wrapped selection = %d", m.diff.selected)
	}
}

func TestCompareTextsArgAndActions(t *testing.T) {
	m := &Model{}
	m.CompareTextsArg(map[string]any{
		"path":  "/project/Main.java",
		"left":  map[string]any{"text": "old\n", "label": "Clipboard"},
		"right": map[string]any{"text": "new\n", "label": "Main.java"},
	})
	if m.diff == nil || len(m.diff.changes) != 1 || m.diff.leftLabel != "Clipboard" {
		t.Fatalf("compare texts = %+v", m.diff)
	}
	for _, id := range []string{
		"diff.nextDifference", "diff.prevDifference", "diff.applyLeftSide",
		"diff.applyRightSide", "diff.jumpToSource", "diff.compareFiles",
		"diff.compareTexts", "vcs.openDiff", "vcs.showDiffInNewWindow",
		"vcs.nextChange", "vcs.prevChange",
	} {
		if _, ok := actionGet(id); !ok {
			t.Errorf("missing action %s", id)
		}
	}
}

func TestDiffDialogRendersSideBySideControls(t *testing.T) {
	m := &Model{diff: &diffView{
		open: true, path: "/project/Main.java", rightPath: "/project/Main.java",
		leftLabel: "HEAD", rightLabel: "Working tree",
		leftText: "old\n", rightText: "new\n",
	}}
	m.diff.setDiffRows(m.diff.leftText, m.diff.rightText)
	tester := ui.NewTester(func(c *ui.Context) {
		m.diffDialog(c, c.Theme())
	}, 1200, 760)
	tester.Frame()
	for _, label := range []string{
		"HEAD", "Working tree", "Accept Left", "Accept Right", "Jump to Source",
		"Side by side", "Diff Settings…",
	} {
		if !tester.HasText(label) {
			t.Errorf("diff dialog does not contain %q", label)
		}
	}
}

func TestDiffRowsForAlignedAndUnalignedPanes(t *testing.T) {
	rows := []textdiff.Row{
		{Kind: textdiff.Hunk, Text: "@@ -1 +1 @@"},
		{Kind: textdiff.Delete, Old: 1, New: 1, Text: "old"},
		{Kind: textdiff.Add, Old: 1, New: 1, Text: "new"},
		{Kind: textdiff.Context, Old: 2, New: 2, Text: "same"},
	}
	left := diffRowsForPane(rows, true, true)
	right := diffRowsForPane(rows, false, true)
	if len(left) != len(rows) || len(right) != len(rows) {
		t.Fatalf("aligned pane rows = left %d, right %d, want %d", len(left), len(right), len(rows))
	}
	if !left[1].present || left[2].present || right[1].present || !right[2].present {
		t.Fatalf("aligned side presence = left %+v, right %+v", left, right)
	}
	left = diffRowsForPane(rows, true, false)
	right = diffRowsForPane(rows, false, false)
	if len(left) != 3 || len(right) != 3 {
		t.Fatalf("unaligned pane rows = left %d, right %d, want 3 each", len(left), len(right))
	}
	if left[1].source != 1 || right[1].source != 2 {
		t.Fatalf("unaligned source indexes = left %+v, right %+v", left, right)
	}
}

func TestDiffTransferOffsetKeepsRelativePlace(t *testing.T) {
	cases := []struct {
		value, fromMax, toMax, want float32
	}{
		{50, 100, 200, 100},
		{120, 100, 40, 40},
		{-10, 100, 200, 0},
		{20, 0, 30, 20},
		{20, 0, 0, 0},
	}
	for _, c := range cases {
		if got := transferDiffOffset(c.value, c.fromMax, c.toMax); got != c.want {
			t.Errorf("transferDiffOffset(%v, %v, %v) = %v, want %v",
				c.value, c.fromMax, c.toMax, got, c.want)
		}
	}
}

func TestDiffSoftWrapFollowsEditorSetting(t *testing.T) {
	m := &Model{prefs: Preferences{Editor: EditorAppearance{
		SoftWrap: true, SoftWrapFiles: "*.java",
	}}}
	d := &diffView{path: "/project/Main.java"}
	if !m.diffUsesSoftWrap(d, DiffPrefs{SoftWrap: "follow"}) {
		t.Fatal("follow did not use the editor's matching soft-wrap pattern")
	}
	if m.diffUsesSoftWrap(&diffView{path: "/project/README.md"}, DiffPrefs{SoftWrap: "follow"}) {
		t.Fatal("follow wrapped a file the editor pattern does not match")
	}
	if !m.diffUsesSoftWrap(&diffView{path: "/project/README.md"}, DiffPrefs{SoftWrap: "on"}) {
		t.Fatal("diff-specific soft wrap did not override the editor setting")
	}
	if m.diffUsesSoftWrap(d, DiffPrefs{SoftWrap: "off"}) {
		t.Fatal("diff-specific soft wrap off did not override the editor setting")
	}
}

func TestDiffSettingsPersistAndRecompute(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	d := &diffView{
		open: true, path: "/project/Main.java", leftText: "old\n", rightText: "new\n",
	}
	d.setDiffRows(d.leftText, d.rightText)
	m := &Model{cfg: Config{SettingsPath: path}, diff: d}
	collapse, syncScroll := false, false
	want := DiffPrefs{
		Mode: "unified", Ignore: "whitespace", Highlight: "lines",
		Collapse: &collapse, SyncScroll: &syncScroll, AlignChanges: true,
		SoftWrap: "on",
	}
	m.setDiffOptions(want)
	if !reflect.DeepEqual(d.options, want) {
		t.Fatalf("view options = %+v, want %+v", d.options, want)
	}
	if got := LoadPreferences(path).VCS.Diff; !reflect.DeepEqual(got, want) {
		t.Fatalf("stored diff options = %+v, want %+v", got, want)
	}
	if len(d.rows) < 2 {
		t.Fatalf("collapsed-off unified comparison has %d rows, want full source", len(d.rows))
	}
}

func TestDiffIgnoreBlankLinesAppliesOriginalCoordinates(t *testing.T) {
	options := DiffPrefs{Ignore: "whitespaceAndLines"}
	d := &diffView{
		open:      true,
		leftText:  "same\n \nold\n",
		rightText: "same\nnew\n",
		options:   options,
	}
	d.setDiffRows(d.leftText, d.rightText)
	if len(d.changes) != 1 {
		t.Fatalf("changes = %+v, want one content change", d.changes)
	}
	change := d.changes[0]
	if change.oldStart != 2 || change.newStart != 1 {
		t.Fatalf("change = %+v, want old line 3 and new line 2", change)
	}
	m := &Model{diff: d}
	m.ApplyDiffSide(true)
	if d.rightText != "same\nold\n" {
		t.Fatalf("apply left = %q, want %q", d.rightText, "same\nold\n")
	}

	d = &diffView{
		open:      true,
		leftText:  "same\n \nold\n",
		rightText: "same\nnew\n",
		options:   options,
	}
	d.setDiffRows(d.leftText, d.rightText)
	m.diff = d
	m.ApplyDiffSide(false)
	if d.leftText != "same\n \nnew\n" {
		t.Fatalf("apply right = %q, want blank line preserved", d.leftText)
	}
}
