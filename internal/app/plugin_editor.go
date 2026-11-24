package app

import (
	"context"
	"errors"
	"time"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
)

type pluginTextEdit struct {
	Range pluginTextRange `json:"range"`
	Text  string          `json:"text"`
}

type pluginSelection struct {
	Anchor pluginTextPosition `json:"anchor"`
	Active pluginTextPosition `json:"active"`
}

type pluginEditorSnapshot struct {
	ID         uint32            `json:"id"`
	Document   map[string]any    `json:"document"`
	Selections []pluginSelection `json:"selections"`
}

func (h *goideaPluginHost) pluginActiveEditor() *pluginEditorSnapshot {
	var out *pluginEditorSnapshot
	h.m.post(func() {
		out = pluginEditorData(h.m.tab())
	})
	return out
}

func (h *goideaPluginHost) pluginVisibleEditors() []pluginEditorSnapshot {
	var out []pluginEditorSnapshot
	h.m.post(func() {
		seen := map[uint32]bool{}
		for _, tab := range h.m.allTabs() {
			if tab == nil || tab.kind != "" || tab.doc == nil {
				continue
			}
			id := tab.doc.ID()
			if seen[id] {
				continue
			}
			seen[id] = true
			if editor := pluginEditorData(tab); editor != nil {
				out = append(out, *editor)
			}
		}
	})
	return out
}

func pluginEditorData(tab *Tab) *pluginEditorSnapshot {
	if tab == nil || tab.kind != "" || tab.doc == nil || tab.state == nil {
		return nil
	}
	selections := tab.state.Selections()
	out := &pluginEditorSnapshot{
		ID:         tab.doc.ID(),
		Document:   pluginDocumentData(tab.doc),
		Selections: make([]pluginSelection, 0, len(selections)),
	}
	for _, selection := range selections {
		out.Selections = append(out.Selections, pluginSelection{
			Anchor: pluginPosition(tab.doc.UTF16Pos(selection.Anchor)),
			Active: pluginPosition(tab.doc.UTF16Pos(selection.Head)),
		})
	}
	return out
}

func pluginPosition(pos openidea.OutlinePos) pluginTextPosition {
	return pluginTextPosition{Line: int(pos.Line), Character: int(pos.Col)}
}

func (h *goideaPluginHost) pluginEdit(
	id uint32,
	edits []pluginTextEdit,
	version uint64,
	selects []pluginSelection,
) (bool, error) {
	tab := h.pluginEditorByID(id)
	if tab == nil {
		return false, errors.New("the editor is closed")
	}
	if len(edits) == 0 {
		if len(selects) > 0 {
			return true, h.pluginSetSelections(id, selects)
		}
		return true, nil
	}
	doc := tab.doc
	if version == 0 {
		version = doc.Buffer().Snapshot().Version()
	}
	converted := make([]editor.Edit, 0, len(edits))
	for _, edit := range edits {
		start := doc.ByteOffset(openideaPosition(edit.Range.Start))
		end := doc.ByteOffset(openideaPosition(edit.Range.End))
		if start > end {
			start, end = end, start
		}
		converted = append(converted, editor.Edit{
			Start: start,
			End:   end,
			Text:  edit.Text,
		})
	}
	ctx, cancel := context.WithTimeout(h.m.remoteBaseContext(), 30*time.Second)
	defer cancel()
	if err := doc.ApplyEditsIfVersion(ctx, version, converted...); err != nil {
		if errors.Is(err, docbridge.ErrChanged) {
			return false, nil
		}
		return false, err
	}
	if len(selects) > 0 {
		if err := h.pluginSetSelections(id, selects); err != nil {
			return true, err
		}
	}
	return true, nil
}

func (h *goideaPluginHost) pluginInsert(id uint32, text string) (bool, error) {
	tab := h.pluginEditorByID(id)
	if tab == nil {
		return false, errors.New("the editor is closed")
	}
	edits := make([]pluginTextEdit, 0, len(tab.state.Selections()))
	for _, selection := range tab.state.Selections() {
		edits = append(edits, pluginTextEdit{
			Range: pluginTextRange{
				Start: pluginPosition(tab.doc.UTF16Pos(selection.Start())),
				End:   pluginPosition(tab.doc.UTF16Pos(selection.End())),
			},
			Text: text,
		})
	}
	return h.pluginEdit(id, edits, tab.doc.Buffer().Snapshot().Version(), nil)
}

func (h *goideaPluginHost) pluginSetSelections(id uint32, values []pluginSelection) error {
	var err error
	h.m.post(func() {
		tab := h.pluginTabByID(id)
		if tab == nil {
			err = errors.New("the editor is closed")
			return
		}
		selections := make([]editor.Selection, 0, len(values))
		for _, value := range values {
			anchor := tab.doc.ByteOffset(openideaPosition(value.Anchor))
			head := tab.doc.ByteOffset(openideaPosition(value.Active))
			selections = append(selections, editor.Selection{Anchor: anchor, Head: head})
		}
		if len(selections) == 0 {
			caret := tab.state.Cursor()
			selections = append(selections, editor.Selection{Anchor: caret, Head: caret})
		}
		tab.state.SetSelections(selections...)
		h.m.invalidate()
	})
	return err
}

func (h *goideaPluginHost) pluginRevealEditor(id uint32, line int) error {
	var err error
	h.m.post(func() {
		tab := h.pluginTabByID(id)
		if tab == nil {
			err = errors.New("the editor is closed")
			return
		}
		if line < 0 {
			line = 0
		}
		tab.state.GoTo(editor.Pos{Line: line})
		h.m.activateTab(tab)
		h.m.invalidate()
	})
	return err
}

func (h *goideaPluginHost) pluginEditorByID(id uint32) *Tab {
	var tab *Tab
	h.m.post(func() {
		tab = h.pluginTabByID(id)
	})
	return tab
}

func (h *goideaPluginHost) pluginTabByID(id uint32) *Tab {
	if active := h.m.tab(); active != nil && active.kind == "" &&
		active.doc != nil && active.doc.ID() == id {
		return active
	}
	for _, tab := range h.m.allTabs() {
		if tab != nil && tab.kind == "" && tab.doc != nil && tab.doc.ID() == id {
			return tab
		}
	}
	return nil
}
