package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

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
	if len(m.notify.items) != 1 || m.notify.items[0].key != "refactor.workspaceEdit" ||
		m.notify.items[0].title != "Refactoring applied" ||
		len(m.notify.items[0].actions) != 1 || m.notify.items[0].actions[0].label != "Undo" {
		t.Fatalf("applied notification = %+v", m.notify.items)
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

func TestWorkspaceEditNotificationAlternatesUndoAndRedo(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	root := t.TempDir()
	posts := make(chan func(), 4)
	m := &Model{
		ctx:      ctx,
		postHook: func(fn func()) { posts <- fn },
	}
	m.setRoot(root)
	m.invokeHook = func(_ context.Context, command string, input any) (json.RawMessage, error) {
		if command != "undo_workspace_edit" && command != "redo_workspace_edit" {
			t.Fatalf("unexpected RPC %q", command)
		}
		if id := input.(map[string]any)["id"]; id != uint32(17) {
			t.Fatalf("%s id = %#v", command, id)
		}
		return json.RawMessage(`{}`), nil
	}

	m.acceptWorkspaceEdit(openidea.WorkspaceEditAppliedEvent{
		ID: 17, Paths: []string{"/p/A.java", "/p/B.java"},
	})
	applied := m.notify.items[0]
	applied.actions[0].run(m)
	waitForRefactorNotification(t, ctx, posts, m, "Refactoring undone", "Redo")
	if m.refactor != nil || m.redoRefactor == nil {
		t.Fatalf("undo history = undo %+v redo %+v", m.refactor, m.redoRefactor)
	}

	undone := m.notify.items[0]
	undone.actions[0].run(m)
	waitForRefactorNotification(t, ctx, posts, m, "Refactoring applied", "Undo")
	if m.refactor == nil || m.redoRefactor != nil {
		t.Fatalf("redo history = undo %+v redo %+v", m.refactor, m.redoRefactor)
	}
}

func waitForRefactorNotification(
	t *testing.T,
	ctx context.Context,
	posts <-chan func(),
	m *Model,
	title, action string,
) {
	t.Helper()
	for {
		select {
		case fn := <-posts:
			fn()
			if len(m.notify.items) > 0 &&
				m.notify.items[0].title == title &&
				len(m.notify.items[0].actions) == 1 &&
				m.notify.items[0].actions[0].label == action {
				return
			}
		case <-ctx.Done():
			t.Fatalf("refactoring notification did not become %q with %s", title, action)
		}
	}
}

func TestRefactorErrorText(t *testing.T) {
	if got := refactorErrorText(&openidea.Error{Kind: "notFound", Message: "gone"}, "undo"); !strings.Contains(got, "history") {
		t.Fatalf("not found says %q", got)
	}
	if got := refactorErrorText(&openidea.Error{Kind: "outOfSync", Message: "changed"}, "redo"); !strings.Contains(got, "by hand") {
		t.Fatalf("out of sync says %q", got)
	}
	if got := refactorErrorText(errors.New("boom"), "undo"); got != "Undo refactoring failed: boom" {
		t.Fatalf("plain error says %q", got)
	}
}
