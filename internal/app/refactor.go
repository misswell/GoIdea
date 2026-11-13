package app

import (
	"context"
	"fmt"

	"goidea/internal/openidea"
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
		m.status = "没有可撤销的重构"
		m.invalidate()
		return
	}
	m.refactorHistory("undo_workspace_edit", r, true)
}

// RedoRefactor reapplies the refactor the last undo rolled back.
func (m *Model) RedoRefactor() {
	r := m.redoRefactor
	if r == nil {
		m.status = "没有可重做的重构"
		m.invalidate()
		return
	}
	m.refactorHistory("redo_workspace_edit", r, false)
}

func (m *Model) refactorHistory(cmd string, r *refactorEdit, undo bool) {
	verb := "撤销"
	if undo {
		m.status = "正在撤销重构…"
	} else {
		verb, m.status = "重做", "正在重做重构…"
	}
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, renameTimeout)
		defer cancel()
		_, err := m.mgr.Invoke(ctx, Window, cmd, map[string]any{"id": r.id})
		m.post(func() {
			if err != nil {
				m.status = refactorErrorText(err, verb)
				m.invalidate()
				return
			}
			if undo {
				m.refactor, m.redoRefactor = nil, r
			} else {
				m.refactor, m.redoRefactor = r, nil
			}
			m.status = fmt.Sprintf("已%s重构：%d 个文件", verb, len(r.paths))
			m.invalidate()
		})
	}()
}

// refactorErrorText turns the errors undo and redo can hit into what the status
// line says.
func refactorErrorText(err error, verb string) string {
	switch {
	case openidea.ErrorKind(err) == "notFound":
		return "该重构已不在历史记录中，无法" + verb
	case openidea.IsOutOfSync(err):
		return "文件在重构后被修改，无法" + verb + "；请手动处理"
	}
	return verb + "重构失败：" + err.Error()
}
