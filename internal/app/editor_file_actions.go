package app

import (
	"context"
	"encoding/json"
	"path/filepath"

	"github.com/egoist/mygo"
	"mayoi/internal/docbridge"
)

// activeFileCanToggleReadOnly reports whether the active tab is a real file
// document whose filesystem writable bit OpenIDEA can change.
func (m *Model) activeFileCanToggleReadOnly() bool {
	t := m.tab()
	return t != nil && t.kind == "" && t.doc != nil && t.state != nil && t.doc.ID() != 0
}

// ToggleActiveFileReadOnly confirms and then flips the writable bit through
// OpenIDEA's fs_set_writable, matching File Properties and the status-bar lock.
func (m *Model) ToggleActiveFileReadOnly() {
	if !m.activeFileCanToggleReadOnly() {
		return
	}
	t := m.tab()
	doc := t.doc
	readOnly := doc.ReadOnly()
	name := filepath.Base(doc.Path())
	title, message, confirm := "Make File Read-only", "Make '"+name+"' read-only?", "Make Read-only"
	if readOnly {
		title, message, confirm = "Make File Writable", "'"+name+"' is read-only. Make it writable?", "Make Writable"
	}
	go func() {
		result, err := mygo.Dialog.Message(mygo.MessageOptions{
			Parent: m.win, Type: mygo.MessageQuestion, Title: title, Message: message,
			Buttons: []string{confirm, "Cancel"}, DefaultButton: 0, CancelButton: 1,
		})
		if err != nil || result.Button != 0 {
			return
		}
		m.setFileWritable(doc, !readOnly)
	}()
}

// setFileWritable calls fs_set_writable for the document the command started
// on and applies its authoritative result to every tab sharing that document.
func (m *Model) setFileWritable(doc *docbridge.Document, writable bool) {
	if doc == nil || doc.ID() == 0 {
		return
	}
	ctx := m.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	go func() {
		raw, err := m.invoke(ctx, "fs_set_writable", map[string]any{
			"path": doc.Path(), "writable": writable,
		})
		var readOnly bool
		if err == nil {
			err = json.Unmarshal(raw, &readOnly)
		}
		m.post(func() {
			if err != nil {
				action := "read-only"
				if writable {
					action = "writable"
				}
				m.status = "Cannot make " + filepath.Base(doc.Path()) + " " + action + ": " + err.Error()
				m.invalidate()
				return
			}
			doc.SetReadOnly(readOnly)
			for _, tab := range m.allTabs() {
				if tab.doc == doc && tab.state != nil {
					tab.state.ReadOnly = readOnly
				}
			}
			if readOnly {
				m.status = filepath.Base(doc.Path()) + " is read-only"
			} else {
				m.status = filepath.Base(doc.Path()) + " is writable"
			}
			m.invalidate()
		})
	}()
}
