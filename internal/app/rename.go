package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"goidea/internal/openidea"
)

// renameTimeout bounds one rename step; JDT may validate the whole workspace
// for a rename, which a cold index makes slow.
const renameTimeout = 60 * time.Second

// renameState is the rename prompt: the symbol the caret was on, the name the
// user is typing, and the edit computed for it when it needs confirming.
type renameState struct {
	tab  *Tab
	pos  openidea.OutlinePos
	old  string
	name string
	busy bool
	err  string

	// pending is the edit lsp_rename computed, as JSON for
	// apply_workspace_edit, kept until the user confirms it.
	pending json.RawMessage
	files   int
	moves   int
	warning string
}

// RenameActive opens the rename prompt for the symbol at the caret.
func (m *Model) RenameActive() {
	t := m.tab()
	if t == nil || m.rename != nil || !m.tabOpen(t) {
		return
	}
	docID := t.doc.ID()
	pos := t.doc.UTF16Pos(t.state.Cursor())
	m.status = "正在查找可重命名的符号…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, renameTimeout)
		defer cancel()
		raw, err := m.mgr.Invoke(ctx, Window, "lsp_prepare_rename", map[string]any{
			"doc": docID, "pos": lspPos(pos),
		})
		var rng *openidea.OutlineRange
		if err == nil {
			err = json.Unmarshal(raw, &rng)
		}
		var old string
		if err == nil && rng != nil {
			start, end := t.doc.ByteRange(*rng)
			old = t.doc.Buffer().Snapshot().Slice(start, end)
		}
		m.post(func() {
			switch {
			case err != nil:
				m.fail(t.doc.Path(), fmt.Errorf("重命名失败：%w", err))
			case old == "":
				m.status = "光标处没有可重命名的符号"
			default:
				m.rename = &renameState{tab: t, pos: pos, old: old, name: old}
			}
			m.invalidate()
		})
	}()
}

// submitRename computes the edit for the typed name; an edit upstream marks for
// preview waits for applyRename before it touches any file.
func (m *Model) submitRename() {
	r := m.rename
	if r == nil || r.busy {
		return
	}
	name := strings.TrimSpace(r.name)
	if name == "" || name == r.old || !m.tabOpen(r.tab) {
		m.rename = nil
		m.invalidate()
		return
	}
	r.busy = true
	r.err = ""
	t := r.tab
	docID := t.doc.ID()
	pos := r.pos
	m.status = "正在计算重命名…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, renameTimeout)
		defer cancel()
		if serr := t.doc.Sync(ctx); serr != nil {
			m.finishRename(r, serr, nil)
			return
		}
		raw, err := m.mgr.Invoke(ctx, Window, "lsp_rename", map[string]any{
			"request": m.nextRequest(), "doc": docID, "pos": lspPos(pos), "newName": name,
		})
		var renamed openidea.Renamed
		if err == nil {
			err = json.Unmarshal(raw, &renamed)
		}
		m.finishRename(r, err, &renamed)
	}()
}

func (m *Model) finishRename(r *renameState, err error, renamed *openidea.Renamed) {
	m.post(func() {
		if r != m.rename {
			return
		}
		r.busy = false
		switch {
		case err != nil:
			r.err = renameErrorText(err)
			m.status = "重命名失败"
		case renamed == nil:
			r.err = "重命名没有返回结果"
		default:
			info := renameEditInfo(renamed.Edit)
			if info.Files == 0 {
				m.rename = nil
				m.status = "没有需要修改的地方"
				break
			}
			r.files = info.Files
			r.moves = info.Moves
			r.warning = renameIncompleteText(renamed.Incomplete)
			if renamed.Preview {
				r.pending = renamed.Edit
				break
			}
			m.rename = nil
			m.status = fmt.Sprintf("正在重命名 %s → %s…", r.old, strings.TrimSpace(r.name))
			m.applyRenameEdit(r, renamed.Edit, info)
		}
		m.invalidate()
	})
}

// applyRename applies a previewed edit the user confirmed.
func (m *Model) applyRename() {
	r := m.rename
	if r == nil || r.busy || len(r.pending) == 0 {
		return
	}
	r.busy = true
	r.err = ""
	edit := r.pending
	r.pending = nil
	info := renameEditInfo(edit)
	m.invalidate()
	m.applyRenameEdit(r, edit, info)
}

func (m *Model) applyRenameEdit(r *renameState, edit json.RawMessage, info renameInfo) {
	old, name := r.old, strings.TrimSpace(r.name)
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, renameTimeout)
		defer cancel()
		_, err := m.mgr.Invoke(ctx, Window, "apply_workspace_edit", map[string]any{"edit": edit})
		m.post(func() {
			if err != nil {
				if r == m.rename {
					r.busy = false
					r.err = renameErrorText(err)
					m.invalidate()
				} else {
					m.fail("", fmt.Errorf("重命名 %s → %s：%w", old, name, err))
				}
				return
			}
			if r == m.rename {
				m.rename = nil
			}
			m.status = fmt.Sprintf("已重命名 %s → %s：%d 个文件", old, name, info.Files)
			m.invalidate()
		})
	}()
}

// renameInfo is what the UI says about an edit before applying it.
type renameInfo struct {
	Files int
	Moves int
}

func renameEditInfo(edit json.RawMessage) renameInfo {
	var e openidea.WorkspaceEditInfo
	if len(edit) == 0 || json.Unmarshal(edit, &e) != nil {
		return renameInfo{}
	}
	info := renameInfo{Files: len(e.Files)}
	for _, f := range e.Files {
		if f.Kind != "edit" {
			info.Moves++
		}
	}
	return info
}

// renameErrorText turns the errors a rename can hit into what the dialog says.
func renameErrorText(err error) string {
	switch {
	case openidea.IsOutOfSync(err):
		return "文档已改动，请重新发起重命名"
	case openidea.ErrorKind(err) == "notFound":
		return "找不到该符号所属的语言服务器"
	}
	return err.Error()
}

// renameIncompleteText names what a rename may have missed.
func renameIncompleteText(incomplete *string) string {
	if incomplete == nil {
		return ""
	}
	switch *incomplete {
	case "indexing":
		return "语言服务器仍在索引，重命名可能遗漏引用"
	case "tooManyFiles":
		return "提及该名称的文件过多，只检查了一部分"
	case "filesSkipped":
		return "部分文件过大或无法读取，未检查"
	case "serverFailed":
		return "服务端查询失败，重命名可能遗漏引用"
	}
	return "重命名可能遗漏引用（" + *incomplete + "）"
}

// renameDialog shows the prompt over the window while a rename is open.
func (m *Model) renameDialog(c *ui.Context, p *ui.Theme) {
	r := m.rename
	if r == nil {
		return
	}
	open := true
	d := ui.Modal(c, &open, func() {
		ui.Column(c).Width(420).Gap(8).Children(func() {
			title := "重命名符号"
			if r.old != "" {
				title += "：" + r.old
			}
			ui.Text(c, title).FontSize(13).Bold().TextColor(p.Text).SingleLine().Ellipsis("…")
			in := ui.TextInput(c, &r.name).AutoFocus()
			if in.Submitted() {
				m.submitRename()
			}
			if r.warning != "" {
				ui.Text(c, r.warning).FontSize(11).TextColor(p.Warning)
			}
			if r.err != "" {
				ui.Text(c, r.err).FontSize(11).TextColor(p.Danger)
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if r.busy {
					ui.Text(c, "处理中…").FontSize(11.5).TextColor(p.TextMuted)
				}
				if len(r.pending) > 0 {
					label := fmt.Sprintf("应用到 %d 个文件", r.files)
					if r.moves > 0 {
						label = fmt.Sprintf("应用到 %d 个文件（含 %d 项重命名）", r.files, r.moves)
					}
					ui.Button(c, label).Disabled(r.busy).OnClick(func() { m.applyRename() })
				} else {
					ui.Button(c, "重命名").Disabled(r.busy).OnClick(func() { m.submitRename() })
				}
				ui.Button(c, "取消").OnClick(func() {
					m.rename = nil
					m.invalidate()
				})
			})
		})
	})
	if d.Dismissed() || !open {
		m.rename = nil
		m.invalidate()
	}
}
