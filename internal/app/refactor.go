package app

import (
	"context"
	"fmt"
	"strings"

	"mayoi/internal/openidea"
)

// refactorEdit is a workspace edit the backend keeps in its undo history: the
// id undo_workspace_edit and redo_workspace_edit take, and the files it touched.
type refactorEdit struct {
	id    uint32
	paths []string
}

// acceptWorkspaceEdit remembers the backend's newest workspace edit, which is
// what UndoRefactor undoes; a new edit invalidates the redo it replaced.
func (m *Model) acceptWorkspaceEdit(e openidea.WorkspaceEditAppliedEvent) {
	if e.ID == 0 {
		return
	}
	m.refactor = &refactorEdit{id: e.ID, paths: e.Paths}
	m.redoRefactor = nil
}

// UndoRefactor rolls back the last refactor as one operation.
func (m *Model) UndoRefactor() {
	r := m.refactor
	if r == nil {
		m.status = "There is no refactoring to undo"
		m.invalidate()
		return
	}
	m.refactorHistory("undo_workspace_edit", r, true)
}

// RedoRefactor reapplies the refactor the last undo rolled back.
func (m *Model) RedoRefactor() {
	r := m.redoRefactor
	if r == nil {
		m.status = "There is no refactoring to redo"
		m.invalidate()
		return
	}
	m.refactorHistory("redo_workspace_edit", r, false)
}

func (m *Model) refactorHistory(cmd string, r *refactorEdit, undo bool) {
	done, action := "Undid", "undo"
	if undo {
		m.status = "Undoing the refactoring…"
	} else {
		done, action, m.status = "Redid", "redo", "Redoing the refactoring…"
	}
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, renameTimeout)
		defer cancel()
		_, err := m.mgr.Invoke(ctx, Window, cmd, map[string]any{"id": r.id})
		m.post(func() {
			if err != nil {
				m.status = refactorErrorText(err, action)
				m.invalidate()
				return
			}
			if undo {
				m.refactor, m.redoRefactor = nil, r
			} else {
				m.refactor, m.redoRefactor = r, nil
			}
			m.status = fmt.Sprintf("%s the refactoring: %d files", done, len(r.paths))
			m.invalidate()
		})
	}()
}

// refactorErrorText turns the errors undo and redo can hit into what the status
// line says.
func refactorErrorText(err error, verb string) string {
	switch {
	case openidea.ErrorKind(err) == "notFound":
		return "The refactoring is no longer in the history; cannot " + verb
	case openidea.IsOutOfSync(err):
		return "The file changed after the refactoring; cannot " + verb + "; resolve it by hand"
	}
	title := strings.ToUpper(verb[:1]) + verb[1:]
	return title + " refactoring failed: " + err.Error()
}
