package app

import (
	"strings"
	"testing"
)

func TestMergeTextChangesClassifiesConflict(t *testing.T) {
	model := newMergeEditorModel("/p/a.txt", "a\nleft\nz\n", "a\nbase\nz\n", "a\nright\nz\n")
	if len(model.changes) != 1 {
		t.Fatalf("changes = %+v, want one", model.changes)
	}
	change := model.changes[0]
	if change.kind != mergeChangeConflict || change.left != "left\n" ||
		change.base != "base\n" || change.right != "right\n" {
		t.Fatalf("conflict = %+v", change)
	}
	if model.result != model.base || model.conflictCount() != 1 {
		t.Fatalf("initial result = %q, unresolved conflicts = %d", model.result, model.conflictCount())
	}
}

func TestMergeTextChangesKeepsIndependentChangesApart(t *testing.T) {
	model := newMergeEditorModel("/p/a.txt", "ONE\nb\nc\n", "one\nb\nc\n", "one\nb\nTHREE\n")
	if len(model.changes) != 2 {
		t.Fatalf("changes = %+v, want two", model.changes)
	}
	if model.changes[0].kind != mergeChangedLeft || model.changes[1].kind != mergeChangedRight {
		t.Fatalf("kinds = %q, %q", model.changes[0].kind, model.changes[1].kind)
	}
	model.applyNonConflicting(mergeLeft, false)
	if len(model.unresolved()) != 1 {
		t.Fatalf("unresolved after left and right non-conflicting = %v", model.unresolved())
	}
	if model.acceptChange(1, mergeRight, true) == false {
		t.Fatal("right change was not accepted")
	}
	if model.result != "ONE\nb\nTHREE\n" {
		t.Fatalf("merged result = %q", model.result)
	}
}

func TestMergeConflictAcceptsBothSidesInOrder(t *testing.T) {
	model := newMergeEditorModel("/p/a.txt", "ours\n", "base\n", "theirs\n")
	if len(model.changes) != 1 || model.changes[0].kind != mergeChangeConflict {
		t.Fatalf("changes = %+v", model.changes)
	}
	if !model.acceptChange(0, mergeLeft, false) {
		t.Fatal("left side was not accepted")
	}
	if model.result != "ours\n" || len(model.unresolved()) != 1 {
		t.Fatalf("after left = %q, unresolved %v", model.result, model.unresolved())
	}
	if !model.acceptChange(0, mergeRight, false) {
		t.Fatal("right side was not accepted")
	}
	if model.result != "ours\ntheirs\n" || len(model.unresolved()) != 0 {
		t.Fatalf("after both = %q, unresolved %v", model.result, model.unresolved())
	}
}

func TestMergeActionsAutoIgnoreAndRevert(t *testing.T) {
	model := newMergeEditorModel("/p/a.txt", "left one\n", "base\n", "left two\n")
	// The two words differ, so the conflict has no automatic resolution.
	if got := model.resolveSimple(); got != 0 {
		t.Fatalf("simple resolution = %d, changes %+v", got, model.changes)
	}
	if len(model.changes) != 1 || model.changes[0].auto != "" {
		t.Fatalf("conflict unexpectedly auto-resolved: %+v", model.changes)
	}
	if !model.ignoreChange(0, mergeLeft) || !model.ignoreChange(0, mergeRight) {
		t.Fatal("one or both conflict sides were not ignored")
	}
	if model.result != model.base || len(model.unresolved()) != 0 {
		t.Fatalf("ignored result = %q, unresolved %v", model.result, model.unresolved())
	}
	if model.revertResolved() != 1 {
		t.Fatal("resolved conflict was not reverted")
	}
	if len(model.unresolved()) != 1 {
		t.Fatalf("revert left unresolved %v", model.unresolved())
	}
}

func TestMergeManualResultEditsStayAndMapLaterChanges(t *testing.T) {
	model := newMergeEditorModel("/p/a.txt", "LEFT\nmiddle\nright\n", "base\nmiddle\nbase2\n", "base\nmiddle\nRIGHT\n")
	if len(model.changes) != 2 {
		t.Fatalf("changes = %+v, want two", model.changes)
	}
	model.setResultText("hand edited\nmiddle\nbase2\n")
	model.acceptChange(1, mergeRight, true)
	if !strings.HasPrefix(model.result, "hand edited\nmiddle\n") ||
		!strings.HasSuffix(model.result, "RIGHT\n") {
		t.Fatalf("result after manual and right = %q", model.result)
	}
	if !model.changes[0].manual {
		t.Fatalf("manual change not recorded: %+v", model.changes[0])
	}
}

func TestMergeOutputKeepsCRLF(t *testing.T) {
	model := newMergeEditorModel("/p/a.txt", "left\r\n", "base\r\n", "right\r\n")
	if model.result != "base\n" || model.eol != "\r\n" {
		t.Fatalf("normalized result = %q, EOL = %q", model.result, model.eol)
	}
	model.acceptChange(0, mergeLeft, true)
	if got := model.outputText(); got != "left\r\n" {
		t.Fatalf("output = %q", got)
	}
}

func TestMergeTextContainsBinary(t *testing.T) {
	if !mergeTextContainsBinary("a\x00b") {
		t.Fatal("NUL was not recognized as binary")
	}
	if mergeTextContainsBinary("plain text\n") {
		t.Fatal("plain text was reported as binary")
	}
}
