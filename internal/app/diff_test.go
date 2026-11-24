package app

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
	"mayoi/internal/position"
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
		leftEditable: true, rightEditable: true,
	}
	d.resetDiffEditors()
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
		leftEditable: true, rightEditable: true,
	}
	d.resetDiffEditors()
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
		"diff.applyRightSide", "diff.acceptLeftAll", "diff.acceptRightAll",
		"diff.applyResult", "diff.saveResult", "diff.jumpToSource", "diff.compareFiles",
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
		leftEditable: true, rightEditable: true,
	}}
	m.diff.resetDiffEditors()
	m.diff.setDiffRows(m.diff.leftText, m.diff.rightText)
	tester := ui.NewTester(func(c *ui.Context) {
		m.diffDialog(c, c.Theme())
	}, 1200, 760)
	tester.Frame()
	for _, label := range []string{
		"HEAD", "Working tree", "Accept Left", "Accept Right", "Jump to Source",
		"Use Left for All", "Use Right for All", "Apply Result", "Save Result",
		"Reject Change", "Reject Entire Side", "Discard Result",
		"Side by side", "Diff Settings…",
	} {
		if !tester.HasText(label) {
			t.Errorf("diff dialog does not contain %q", label)
		}
	}
	if _, ok := tester.Find("Compare left"); !ok {
		t.Error("diff dialog did not render the left native editor")
	}
	if _, ok := tester.Find("Compare right"); !ok {
		t.Error("diff dialog did not render the right native editor")
	}
}

func TestDiffDialogOffscreenRejectAndDiscard(t *testing.T) {
	m := &Model{diff: &diffView{
		open: true, path: "/project/Main.java", rightPath: "/project/Main.java",
		leftLabel: "HEAD", rightLabel: "Working tree",
		leftText: "old\n", rightText: "new\n",
		leftEditable: true, rightEditable: true,
		activeSide: editor.DiffNew,
	}}
	d := m.diff
	d.resetDiffEditors()
	d.setDiffRows(d.leftText, d.rightText)
	tester := ui.NewTester(func(c *ui.Context) {
		m.diffDialog(c, c.Theme())
	}, 1200, 760)
	tester.Frame()

	if err := tester.Click("Reject Change"); err != nil {
		t.Fatal(err)
	}
	if d.rightText != "old\n" || !d.dirty {
		t.Fatalf("offscreen reject = text %q dirty %v", d.rightText, d.dirty)
	}
	tester.Frame()
	if !tester.HasText("Unsaved result") {
		t.Fatal("offscreen diff did not show its dirty result")
	}
	if err := tester.Click("Discard Result"); err != nil {
		t.Fatal(err)
	}
	if d.rightText != "new\n" || d.dirty {
		t.Fatalf("offscreen discard = text %q dirty %v", d.rightText, d.dirty)
	}

	if err := tester.Click("Reject Entire Side"); err != nil {
		t.Fatal(err)
	}
	if d.rightText != "old\n" || !d.dirty {
		t.Fatalf("offscreen whole-side reject = text %q dirty %v", d.rightText, d.dirty)
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
		open:         true,
		leftText:     "same\n \nold\n",
		rightText:    "same\nnew\n",
		leftEditable: true, rightEditable: true,
		options: options,
	}
	d.resetDiffEditors()
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
		open:         true,
		leftText:     "same\n \nold\n",
		rightText:    "same\nnew\n",
		leftEditable: true, rightEditable: true,
		options: options,
	}
	d.resetDiffEditors()
	d.setDiffRows(d.leftText, d.rightText)
	m.diff = d
	m.ApplyDiffSide(false)
	if d.leftText != "same\n \nnew\n" {
		t.Fatalf("apply right = %q, want blank line preserved", d.leftText)
	}
}

func TestEditableDiffUsesNativeEditorStates(t *testing.T) {
	m := &Model{}
	d := &diffView{
		open:         true,
		path:         "/project/Main.java",
		leftPath:     "/project/Main.java",
		rightPath:    "/project/Main.java",
		leftText:     "one\ntwo\n",
		rightText:    "one\nTWO\n",
		leftEditable: true, rightEditable: true,
	}
	d.resetDiffEditors()
	d.setDiffRows(d.leftText, d.rightText)
	m.ensureDiffEditors(d)
	if d.leftEditor == nil || d.rightEditor == nil {
		t.Fatal("editable compare did not create both native editor states")
	}
	if d.leftEditor.ReadOnly || d.rightEditor.ReadOnly {
		t.Fatal("file-backed compare panes should be editable")
	}
	d.leftEditor.Buffer.Edit(editor.Edit{
		Start: len("one\n"), End: len("one\ntwo"), Text: "changed",
	})
	if !d.refreshDiffEditorText() {
		t.Fatal("native editor edit was not observed")
	}
	if !d.dirty || d.leftText != "one\nchanged\n" || len(d.changes) == 0 {
		t.Fatalf("edited compare = dirty %v, left %q, changes %+v", d.dirty, d.leftText, d.changes)
	}
}

func TestDiffAcceptUsesTheChangeAtTheActiveCaret(t *testing.T) {
	m := &Model{}
	d := &diffView{
		open: true, leftText: "same\nleft\nend\n", rightText: "same\nright\nend\n",
		leftEditable: true, rightEditable: true,
	}
	d.resetDiffEditors()
	d.setDiffRows(d.leftText, d.rightText)
	m.ensureDiffEditors(d)
	line := d.rightEditor.Buffer.Snapshot().LineStart(1)
	d.rightEditor.Select(line, line)
	d.activeSide = editor.DiffNew
	m.diff = d

	m.ApplyDiffSide(true)

	if got, want := d.rightEditor.Buffer.String(), d.leftEditor.Buffer.String(); got != want {
		t.Fatalf("accepted right side = %q, want %q", got, want)
	}
	if !d.dirty {
		t.Fatal("accepted change was not marked as an unsaved compare result")
	}
}

func TestDiffWholeSideAcceptAndReadOnlyTarget(t *testing.T) {
	m := &Model{}
	d := &diffView{
		open:     true,
		leftPath: "/project/A.java", rightPath: "/project/B.java",
		leftText: "left one\nleft two\n", rightText: "right\n",
		leftEditable: true, rightEditable: true,
	}
	d.resetDiffEditors()
	m.diff = d
	m.ApplyWholeDiffSide(true)
	if d.rightText != d.leftText || d.rightOriginal == d.rightText {
		t.Fatalf("whole-side result = %q, original %q", d.rightText, d.rightOriginal)
	}

	readOnly := &diffView{
		open: true, leftText: "old\n", rightText: "new\n",
		rightEditable: true,
	}
	readOnly.setDiffRows(readOnly.leftText, readOnly.rightText)
	m.diff = readOnly
	m.ApplyDiffSide(false)
	if readOnly.leftText != "old\n" {
		t.Fatalf("read-only left side changed to %q", readOnly.leftText)
	}
}

func TestDiffResultTracksOnlyChangedFileBackedEditableSides(t *testing.T) {
	d := &diffView{
		leftPath: "/project/A.java", rightPath: "/project/B.java",
		leftText: "left\n", rightText: "right\n",
		leftOriginal: "left\n", rightOriginal: "right\n",
		leftEditable: true, rightEditable: true,
	}
	d.resetDiffEditors()
	d.leftText, d.rightText = "changed left\n", "right\n"
	files := d.pendingResultFiles()
	if len(files) != 1 || !files[0].left || files[0].path != d.leftPath {
		t.Fatalf("pending result files = %+v", files)
	}

	d.leftEditable = false
	if files := d.pendingResultFiles(); len(files) != 0 {
		t.Fatalf("read-only side became saveable: %+v", files)
	}
}

func TestMapDiffScrollLineUsesChanges(t *testing.T) {
	changes := []diffChange{
		{oldStart: 2, oldEnd: 4, newStart: 2, newEnd: 3},
		{oldStart: 7, oldEnd: 7, newStart: 6, newEnd: 8},
	}
	cases := []struct {
		line int
		ltr  bool
		want int
	}{
		{1, true, 1},
		{2, true, 2},
		{4, true, 3},
		{8, true, 9},
		{5, false, 6},
		{7, false, 7},
	}
	for _, tc := range cases {
		if got := mapDiffScrollLine(tc.line, tc.ltr, changes, 20); got != tc.want {
			t.Errorf("mapDiffScrollLine(%d, %v) = %d, want %d", tc.line, tc.ltr, got, tc.want)
		}
	}
}

func TestDiffWordBackgroundsHighlightOnlyChangedWords(t *testing.T) {
	d := &diffView{
		open:         true,
		leftText:     "same old value\n",
		rightText:    "same new value\n",
		leftEditable: true, rightEditable: true,
		options: DiffPrefs{Highlight: "words"},
	}
	d.resetDiffEditors()
	d.setDiffRows(d.leftText, d.rightText)
	m := &Model{}
	m.ensureDiffEditors(d)
	theme := ui.LightTheme()
	d.applyEditorOptions(false, theme, d.options)

	textAt := func(state *editor.State, backgrounds []editor.TextBackground) []string {
		out := make([]string, 0, len(backgrounds))
		text := state.Buffer.String()
		for _, background := range backgrounds {
			out = append(out, text[background.Start:background.End])
		}
		return out
	}
	if got := textAt(d.leftEditor, d.leftEditor.TextBackgrounds); !reflect.DeepEqual(got, []string{"old"}) {
		t.Fatalf("left inline highlights = %v, want old", got)
	}
	if got := textAt(d.rightEditor, d.rightEditor.TextBackgrounds); !reflect.DeepEqual(got, []string{"new"}) {
		t.Fatalf("right inline highlights = %v, want new", got)
	}

	d.options.Highlight = "lines"
	d.applyEditorOptions(false, theme, d.options)
	if len(d.leftEditor.TextBackgrounds) != 0 || len(d.rightEditor.TextBackgrounds) != 0 {
		t.Fatal("line highlighting still drew word backgrounds")
	}
	if len(d.leftEditor.LineBackgrounds) == 0 || len(d.rightEditor.LineBackgrounds) == 0 {
		t.Fatal("line highlighting did not draw both change blocks")
	}
}

func TestDiffSyncScrollMapsBothDirections(t *testing.T) {
	var left, right strings.Builder
	for i := 0; i < 90; i++ {
		left.WriteString("line ")
		left.WriteString(string(rune('A' + i%26)))
		left.WriteString("\n")
		right.WriteString("line ")
		right.WriteString(string(rune('A' + i%26)))
		right.WriteString("\n")
		if i == 29 {
			right.WriteString("inserted\n")
		}
	}
	m := &Model{}
	d := &diffView{
		open:         true,
		leftText:     left.String(),
		rightText:    right.String(),
		leftEditable: true, rightEditable: true,
		options: DiffPrefs{Mode: "split", SyncScroll: boolPref(true)},
	}
	d.resetDiffEditors()
	d.setDiffRows(d.leftText, d.rightText)
	m.ensureDiffEditors(d)
	tester := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Height(220).Children(func() {
			editor.View(c, d.leftEditor).Width(420).Height(220)
			editor.View(c, d.rightEditor).Width(420).Height(220)
		})
	}, 840, 220)
	tester.Frame()
	d.syncEditorScroll(d.options)

	d.leftEditor.SetScrollY(d.leftEditor.LineTop(50))
	d.syncEditorScroll(d.options)
	if got, want := diffEditorAnchorLine(d.rightEditor, d.rightEditor.ScrollY()), 51; got != want {
		t.Fatalf("left to right anchor = %d, want %d", got, want)
	}

	tester.Frame()
	d.syncEditorScroll(d.options)
	d.rightEditor.SetScrollY(d.rightEditor.LineTop(70))
	d.syncEditorScroll(d.options)
	if got, want := diffEditorAnchorLine(d.leftEditor, d.leftEditor.ScrollY()), 69; got != want {
		t.Fatalf("right to left anchor = %d, want %d", got, want)
	}
}

func TestDiffRejectAndDiscardKeepUnsavedDocumentsUntouched(t *testing.T) {
	m := &Model{}
	d := &diffView{
		open:         true,
		leftText:     "old one\nold two\n",
		rightText:    "new one\nnew two\n",
		leftEditable: true, rightEditable: true,
		activeSide: editor.DiffNew,
	}
	d.resetDiffEditors()
	d.setDiffRows(d.leftText, d.rightText)
	m.diff = d

	m.RejectDiffChange()
	if got, want := d.rightText, "old one\nold two\n"; got != want {
		t.Fatalf("rejected change = %q, want %q", got, want)
	}
	if !d.dirty {
		t.Fatal("rejecting a change did not stage a result")
	}

	d = &diffView{
		open:         true,
		leftText:     "left\n",
		rightText:    "right\n",
		leftEditable: true, rightEditable: true,
	}
	d.resetDiffEditors()
	d.setDiffRows(d.leftText, d.rightText)
	m.diff = d
	m.ensureDiffEditors(d)
	d.replaceDiffSide(false, "typed result\n")
	d.setDiffRows(d.leftText, d.rightText)
	if !d.dirty {
		t.Fatal("typed result was not dirty")
	}
	m.DiscardDiffResult()
	if d.rightText != "right\n" || d.rightEditor.Buffer.String() != "right\n" || d.dirty {
		t.Fatalf("discard = text %q editor %q dirty %v",
			d.rightText, d.rightEditor.Buffer.String(), d.dirty)
	}
}

func TestDiffPathLabelUsesTheContentRoot(t *testing.T) {
	root := t.TempDir()
	m := &Model{}
	m.setRoot(root)
	inside := filepath.Join(root, "src", "Main.java")
	if got, want := m.diffPathLabel(inside), "src/Main.java"; got != want {
		t.Fatalf("inside label = %q, want %q", got, want)
	}
	outside := filepath.Join(t.TempDir(), "Library.java")
	if got := m.diffPathLabel(outside); got != filepath.ToSlash(outside) {
		t.Fatalf("outside label = %q, want absolute %q", got, filepath.ToSlash(outside))
	}
}

func TestCompareTabWithClipboardKeepsFileSideEditable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Main.java")
	doc := docbridge.Virtual(path, "one\ntwo\n")
	tab := &Tab{doc: doc, state: &editor.State{Buffer: doc.Buffer()}}
	done := make(chan struct{}, 1)
	m := &Model{
		tabs: []*Tab{tab}, active: 0,
		postHook: func(fn func()) {
			fn()
			done <- struct{}{}
		},
	}
	m.compareTabWithClipboardText(tab, "clipboard\n")
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("clipboard comparison did not finish")
	}
	d := m.diff
	if d == nil || d.leftText != "clipboard\n" || d.rightText != "one\ntwo\n" {
		t.Fatalf("clipboard comparison = %+v", d)
	}
	if d.leftEditable || !d.rightEditable {
		t.Fatalf("clipboard editability = left %v right %v", d.leftEditable, d.rightEditable)
	}
	if d.rightDocumentVersion != doc.Buffer().Version() || !d.rightVersionKnown ||
		d.leftLabel != "Clipboard" {
		t.Fatalf("clipboard metadata = version %d known %v label %q",
			d.rightDocumentVersion, d.rightVersionKnown, d.leftLabel)
	}
}

func TestCompareTabWithClipboardLabelsTheSelectionFromTheContentRoot(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "src", "Main.java")
	doc := docbridge.Virtual(path, "one\ntwo\n")
	state := &editor.State{Buffer: doc.Buffer()}
	start := state.Buffer.Snapshot().LineStart(1)
	state.Select(start, start+len("two"))
	tab := &Tab{doc: doc, state: state}
	done := make(chan struct{}, 1)
	m := &Model{
		tabs: []*Tab{tab}, active: 0,
		postHook: func(fn func()) {
			fn()
			done <- struct{}{}
		},
	}
	m.setRoot(root)
	m.compareTabWithClipboardText(tab, "clipboard\n")
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("selection comparison did not finish")
	}
	d := m.diff
	if d == nil || d.rightText != "two" || d.rightLabel != "Selection" {
		t.Fatalf("selection comparison = %+v", d)
	}
	if got, want := d.label, "Clipboard ↔ src/Main.java (selection)"; got != want {
		t.Fatalf("selection title = %q, want %q", got, want)
	}
}

func TestDiffStepFileProtectsADirtyResult(t *testing.T) {
	d := &diffView{
		open: true,
		fileChain: []diffFileTarget{
			{left: "/p/left/A.java", right: "/p/right/A.java"},
			{left: "/p/left/B.java", right: "/p/right/B.java"},
		},
		dirty: true,
	}
	m := &Model{diff: d}
	m.StepDiffFile(1)
	if d.fileIndex != 0 {
		t.Fatalf("dirty result switched to file %d", d.fileIndex)
	}
	if !strings.Contains(m.status, "before changing files") {
		t.Fatalf("dirty-file status = %q", m.status)
	}
}

type diffApplyBackend struct {
	info  openidea.DocInfo
	edits chan position.ChangeSet
}

func (b *diffApplyBackend) DocOpen(context.Context, string, string) (openidea.DocInfo, error) {
	return b.info, nil
}

func (b *diffApplyBackend) DocEdit(
	_ context.Context,
	_ string,
	_ uint32,
	baseVersion uint32,
	changes position.ChangeSet,
) (uint32, error) {
	if b.edits != nil {
		b.edits <- changes
	}
	return baseVersion + 1, nil
}

func (b *diffApplyBackend) DocSnapshot(context.Context, string, uint32) (openidea.DocInfo, error) {
	return b.info, nil
}

func (b *diffApplyBackend) DocSave(context.Context, string, uint32) error {
	return nil
}

func (b *diffApplyBackend) DocClose(context.Context, string, uint32) error {
	return nil
}

func TestApplyDiffResultUsesTheDocumentRPCAndClearsDirty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Main.java")
	backend := &diffApplyBackend{
		info: openidea.DocInfo{
			ID: 41, Path: path, Text: "old\n", Version: 7,
			Encoding: "UTF-8", LineEnding: "LF", URI: "file://" + path,
		},
		edits: make(chan position.ChangeSet, 1),
	}
	doc, err := docbridge.Open(context.Background(), backend, Window, path)
	if err != nil {
		t.Fatal(err)
	}
	tab := &Tab{doc: doc, state: &editor.State{Buffer: doc.Buffer()}}
	posted := make(chan struct{}, 1)
	m := &Model{
		ctx: context.Background(), tabs: []*Tab{tab}, active: 0,
		postHook: func(fn func()) {
			fn()
			posted <- struct{}{}
		},
	}
	d := &diffView{
		open: true, path: path, rightPath: path,
		rightText: "old\n", rightEditable: true,
		rightDocumentVersion: doc.Buffer().Version(), rightVersionKnown: true,
	}
	d.resetDiffEditors()
	m.ensureDiffEditors(d)
	d.replaceDiffSide(false, "new\n")
	d.setDiffRows(d.leftText, d.rightText)
	m.diff = d
	m.ApplyDiffResult()

	select {
	case changes := <-backend.edits:
		if len(changes.Changes) != 1 || changes.Changes[0].Insert != "new\n" {
			t.Fatalf("doc_edit changes = %+v", changes)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Apply Result did not send a doc_edit")
	}
	select {
	case <-posted:
	case <-time.After(2 * time.Second):
		t.Fatal("Apply Result did not finish")
	}
	if doc.Text() != "new\n" || !doc.Dirty() || d.rightOriginal != "new\n" || d.dirty {
		t.Fatalf("applied result = doc %q doc dirty %v original %q diff dirty %v",
			doc.Text(), doc.Dirty(), d.rightOriginal, d.dirty)
	}
}

func TestApplyDiffResultRefusesAChangedDocument(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Main.java")
	backend := &diffApplyBackend{info: openidea.DocInfo{
		ID: 42, Path: path, Text: "old\n", Version: 2,
		Encoding: "UTF-8", LineEnding: "LF", URI: "file://" + path,
	}}
	doc, err := docbridge.Open(context.Background(), backend, Window, path)
	if err != nil {
		t.Fatal(err)
	}
	tab := &Tab{doc: doc, state: &editor.State{Buffer: doc.Buffer()}}
	m := &Model{
		tabs: []*Tab{tab}, active: 0,
		postHook: func(fn func()) { fn() },
	}
	d := &diffView{
		open: true, path: path, rightPath: path,
		rightText: "old\n", rightEditable: true,
		rightDocumentVersion: doc.Buffer().Version(), rightVersionKnown: true,
	}
	d.resetDiffEditors()
	m.ensureDiffEditors(d)
	d.replaceDiffSide(false, "diff result\n")
	d.setDiffRows(d.leftText, d.rightText)
	m.diff = d
	doc.Buffer().Edit(editor.Edit{Start: 0, End: 0, Text: "user edit\n"})

	m.ApplyDiffResult()
	if got := doc.Text(); got != "user edit\nold\n" {
		t.Fatalf("conflicting document was overwritten: %q", got)
	}
	if !strings.Contains(d.err, "changed while it was being compared") {
		t.Fatalf("conflict error = %q", d.err)
	}

	doc.Buffer().Edit(editor.Edit{
		Start: 0, End: doc.Buffer().Len(), Text: "old\n",
	})
	d.err = ""
	m.ApplyDiffResult()
	if got := doc.Text(); got != "old\n" {
		t.Fatalf("same-text version conflict was overwritten: %q", got)
	}
	if !strings.Contains(d.err, "changed while it was being compared") {
		t.Fatalf("version-only conflict error = %q", d.err)
	}
}
