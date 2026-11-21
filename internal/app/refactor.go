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
	root  string
}

// acceptWorkspaceEdit remembers the backend's newest workspace edit, which is
// what UndoRefactor undoes; a new edit invalidates the redo it replaced and
// replaces OpenIDEA's refactoring notification.
func (m *Model) acceptWorkspaceEdit(e openidea.WorkspaceEditAppliedEvent) {
	if e.ID == 0 {
		return
	}
	m.refactor = &refactorEdit{id: e.ID, paths: e.Paths, root: m.Root()}
	m.redoRefactor = nil
	m.showRefactorHistoryNotification(m.refactor, false)
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
		_, err := m.invoke(ctx, cmd, map[string]any{"id": r.id})
		m.post(func() {
			if err != nil {
				text := refactorErrorText(err, action)
				m.status = text
				m.notifyError(
					"refactor.workspaceEdit",
					"Cannot "+action+" refactoring",
					text,
				)
				m.invalidate()
				return
			}
			if undo {
				m.refactor, m.redoRefactor = nil, r
			} else {
				m.refactor, m.redoRefactor = r, nil
			}
			m.status = fmt.Sprintf("%s the refactoring: %d files", done, len(r.paths))
			m.showRefactorHistoryNotification(r, undo)
			m.invalidate()
		})
	}()
}

// showRefactorHistoryNotification mirrors OpenIDEA's workspace history
// notification. The action replaces itself as Undo and Redo alternate.
func (m *Model) showRefactorHistoryNotification(r *refactorEdit, undone bool) {
	title, action := "Refactoring applied", "Undo"
	if undone {
		title, action = "Refactoring undone", "Redo"
	}
	m.notifyInfo("refactor.workspaceEdit", title, "", notifyAction{
		label: action,
		run: func(model *Model) {
			if model.Root() != r.root {
				model.notifyError(
					"refactor.workspaceEdit",
					"Cannot "+strings.ToLower(action)+" refactoring",
					"The project for this refactoring is no longer open",
				)
				return
			}
			if undone {
				if model.redoRefactor != r {
					return
				}
				model.RedoRefactor()
				return
			}
			if model.refactor != r {
				return
			}
			model.UndoRefactor()
		},
	})
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
