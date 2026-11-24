package app

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestDiffOverviewMarksUseExactDocumentRanges(t *testing.T) {
	d := &diffView{
		leftText:  "a\nb\n",
		rightText: "a\nx\nb\n",
	}
	d.setDiffRows(d.leftText, d.rightText)
	right := diffOverviewMarks(d, false, false)
	if len(right) != 1 {
		t.Fatalf("right overview marks = %+v, want one insertion", right)
	}
	if mark := right[0]; mark.kind != overviewInserted ||
		mark.startByte != len("a\n") || mark.endByte != len("a\nx\n") ||
		mark.startLine != 1 || mark.endLine != 2 {
		t.Fatalf("right insertion mark = %+v", mark)
	}
	left := diffOverviewMarks(d, true, false)
	if len(left) != 1 || left[0].kind != overviewInserted ||
		left[0].startByte != len("a\n") || left[0].endByte != len("a\n") ||
		left[0].startLine != 1 || left[0].endLine != 1 {
		t.Fatalf("left insertion point = %+v", left)
	}

	d = &diffView{
		leftText:  "a\nold\nb\n",
		rightText: "a\nnew\nb\n",
	}
	d.setDiffRows(d.leftText, d.rightText)
	if got := diffOverviewMarks(d, false, false); len(got) != 1 ||
		got[0].kind != overviewModified {
		t.Fatalf("modified overview marks = %+v", got)
	}
}

func TestDiffOverviewOmitsAnEmptySideAndUnifiedKeepsBothKinds(t *testing.T) {
	d := &diffView{
		leftText:  "",
		rightText: "one\ntwo\n",
	}
	d.setDiffRows(d.leftText, d.rightText)
	if marks := diffOverviewMarks(d, true, false); len(marks) != 0 {
		t.Fatalf("empty left pane has overview marks: %+v", marks)
	}
	right := diffOverviewMarks(d, false, false)
	if len(right) != 1 || right[0].kind != overviewInserted ||
		right[0].startLine != 0 || right[0].endLine != 2 {
		t.Fatalf("inserted side marks = %+v", right)
	}

	unified := diffOverviewMarks(d, false, true)
	if len(unified) != 1 || unified[0].kind != overviewInserted {
		t.Fatalf("unified insertion marks = %+v", unified)
	}

	d = &diffView{
		leftText:  "gone\n",
		rightText: "",
	}
	d.setDiffRows(d.leftText, d.rightText)
	if marks := diffOverviewMarks(d, false, false); len(marks) != 0 {
		t.Fatalf("empty right pane has overview marks: %+v", marks)
	}
	unified = diffOverviewMarks(d, false, true)
	if len(unified) != 1 || unified[0].kind != overviewDeleted ||
		unified[0].startByte != 0 || unified[0].endByte != 0 {
		t.Fatalf("unified deletion point = %+v", unified)
	}
}

func TestOverviewLanePaintsPointMarksOffscreen(t *testing.T) {
	const text = "one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine\nten\n"
	mark := overviewMarkForLines(text, 5, 5, overviewModified)
	if mark.startLine != mark.endLine {
		t.Fatalf("point mark spans lines %d..%d", mark.startLine, mark.endLine)
	}
	theme := ui.LightTheme()
	tester := ui.NewTester(func(c *ui.Context) {
		overviewLane(c, theme, "overview-test", "Overview test", nil, text,
			[]overviewMark{mark}, 200)
	}, 9, 200)
	tester.Frame()

	image := tester.Image()
	painted := 0
	for y := 0; y < 200; y++ {
		for x := 0; x < 9; x++ {
			pixel := image.RGBAAt(x, y)
			if pixel.R == theme.Accent.R &&
				pixel.G == theme.Accent.G &&
				pixel.B == theme.Accent.B &&
				pixel.A == theme.Accent.A {
				painted++
			}
		}
	}
	if painted < 4 {
		t.Fatalf("point mark painted %d pixels, want a visible marker", painted)
	}
	if painted > 36 {
		t.Fatalf("point mark painted %d pixels, like an adjacent whole line", painted)
	}
}

func TestAlignedSplitUsesOnlyTheRightOverviewLane(t *testing.T) {
	aligned := true
	d := &diffView{
		open: true, path: "/project/Main.java", rightPath: "/project/Main.java",
		leftLabel: "HEAD", rightLabel: "Working tree",
		leftText: "old\n", rightText: "new\n",
		options: DiffPrefs{Mode: "split", AlignChanges: aligned},
	}
	d.setDiffRows(d.leftText, d.rightText)
	m := &Model{diff: d}
	tester := ui.NewTester(func(c *ui.Context) {
		m.diffDialog(c, c.Theme())
	}, 1200, 760)
	tester.Frame()
	if _, ok := tester.Find("Diff overview left"); ok {
		t.Fatal("aligned split rendered a second overview lane")
	}
	if _, ok := tester.Find("Diff overview right"); !ok {
		t.Fatal("aligned split did not render the shared right overview lane")
	}

	d.options.AlignChanges = false
	tester = ui.NewTester(func(c *ui.Context) {
		m.diffDialog(c, c.Theme())
	}, 1200, 760)
	tester.Frame()
	if _, ok := tester.Find("Diff overview left"); !ok {
		t.Fatal("unaligned split did not render the left overview lane")
	}
	if _, ok := tester.Find("Diff overview right"); !ok {
		t.Fatal("unaligned split did not render the right overview lane")
	}
}

func TestMergeOverviewMarksTrackDecisionsAndManualEdits(t *testing.T) {
	model := newMergeEditorModel(
		"/repo/conflict.go",
		"a\nleft\nz\n",
		"a\nbase\nz\n",
		"a\nright\nz\n",
	)
	result := mergeOverviewMarks(model, mergePaneResult)
	if len(result) != 1 || result[0].kind != overviewConflict ||
		result[0].priority <= overviewMarkForLines(model.result, 0, 0, overviewInserted).priority {
		t.Fatalf("conflict marker = %+v", result)
	}
	if marks := mergeOverviewMarks(model, mergePaneYours); len(marks) != 1 {
		t.Fatalf("yours markers before decision = %+v", marks)
	}
	if marks := mergeOverviewMarks(model, mergePaneTheirs); len(marks) != 1 {
		t.Fatalf("theirs markers before decision = %+v", marks)
	}

	if !model.acceptChange(0, mergeLeft, false) {
		t.Fatal("left side was not accepted")
	}
	if marks := mergeOverviewMarks(model, mergePaneYours); len(marks) != 0 {
		t.Fatalf("accepted yours marker remains: %+v", marks)
	}
	if marks := mergeOverviewMarks(model, mergePaneResult); len(marks) != 1 {
		t.Fatalf("unresolved result marker disappeared after one side: %+v", marks)
	}
	if !model.acceptChange(0, mergeRight, false) {
		t.Fatal("right side was not accepted")
	}
	for _, pane := range []mergePane{mergePaneYours, mergePaneResult, mergePaneTheirs} {
		if marks := mergeOverviewMarks(model, pane); len(marks) != 0 {
			t.Fatalf("%s markers remain after resolution: %+v", pane, marks)
		}
	}

	model = newMergeEditorModel(
		"/repo/conflict.go",
		"a\nleft\nz\n",
		"a\nbase\nz\n",
		"a\nright\nz\n",
	)
	model.setResultText("a\nhand edited\nz\n")
	if marks := mergeOverviewMarks(model, mergePaneResult); len(marks) != 0 {
		t.Fatalf("manually resolved range remains: %+v", marks)
	}
}

func TestMergeOverviewConflictHasHighestPaintPriority(t *testing.T) {
	marks := []overviewMark{
		{kind: overviewModified, priority: overviewPriority(overviewModified)},
		{kind: overviewConflict, priority: overviewPriority(overviewConflict)},
		{kind: overviewInserted, priority: overviewPriority(overviewInserted)},
		{kind: overviewDeleted, priority: overviewPriority(overviewDeleted)},
	}
	sorted := sortedOverviewMarks(marks)
	if got := sorted[len(sorted)-1].kind; got != overviewConflict {
		t.Fatalf("last painted mark = %v, want conflict", got)
	}
	for i := 1; i < len(sorted); i++ {
		if sorted[i-1].priority > sorted[i].priority {
			t.Fatalf("marks are not priority ordered: %+v", sorted)
		}
	}
}

func TestOverviewLineOffsetsHandleFinalNewline(t *testing.T) {
	text := strings.Join([]string{"a", "b", "c"}, "\n") + "\n"
	offsets := overviewLineOffsets(text)
	want := []int{0, 2, 4, 6}
	if len(offsets) != len(want) {
		t.Fatalf("offsets = %v, want %v", offsets, want)
	}
	for i := range want {
		if offsets[i] != want[i] {
			t.Fatalf("offsets = %v, want %v", offsets, want)
		}
	}
}
