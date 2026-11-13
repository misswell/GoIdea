package app

import (
	"errors"
	"strings"
	"testing"

	"mayoi/internal/openidea"
)

func TestAcceptWorkspaceEditReplacesRedo(t *testing.T) {
	m := &Model{redoRefactor: &refactorEdit{id: 7, paths: []string{"/p/Old.java"}}}
	m.acceptWorkspaceEdit(openidea.WorkspaceEditAppliedEvent{ID: 9, Paths: []string{"/p/A.java", "/p/B.java"}})
	if m.refactor == nil || m.refactor.id != 9 || len(m.refactor.paths) != 2 {
		t.Fatalf("refactor = %+v", m.refactor)
	}
	if m.redoRefactor != nil {
		t.Fatalf("new edit kept redo %+v", m.redoRefactor)
	}
	// An event without an id is not a history entry; it must not clear what
	// an undo left for redo.
	m.refactor = nil
	m.redoRefactor = &refactorEdit{id: 7}
	m.acceptWorkspaceEdit(openidea.WorkspaceEditAppliedEvent{})
	if m.refactor != nil || m.redoRefactor == nil {
		t.Fatalf("empty event changed history: undo=%+v redo=%+v", m.refactor, m.redoRefactor)
	}
}

func TestRefactorErrorText(t *testing.T) {
	if got := refactorErrorText(&openidea.Error{Kind: "notFound", Message: "gone"}, "撤销"); !strings.Contains(got, "历史记录") {
		t.Fatalf("not found says %q", got)
	}
	if got := refactorErrorText(&openidea.Error{Kind: "outOfSync", Message: "changed"}, "重做"); !strings.Contains(got, "手动处理") {
		t.Fatalf("out of sync says %q", got)
	}
	if got := refactorErrorText(errors.New("boom"), "撤销"); got != "撤销重构失败：boom" {
		t.Fatalf("plain error says %q", got)
	}
}
