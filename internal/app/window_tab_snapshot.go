package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/egoist/mygo/ui/editor"
)

// CaptureWindowTab captures a file tab's unsaved text and editor state. It
// returns false for generated tabs, which have backend-owned state that cannot
// be reconstructed in another project window yet.
func (m *Model) CaptureWindowTab(path string) (TabSnapshot, bool) {
	if m == nil {
		return TabSnapshot{}, false
	}
	t, ok := m.tabByPathInAnyGroup(path)
	if !ok || t == nil || t.doc == nil || t.kind != "" || t.state == nil {
		return TabSnapshot{}, false
	}
	snapshot := TabSnapshot{
		Root:      m.Root(),
		Path:      t.doc.Path(),
		Text:      t.doc.Text(),
		SavedText: t.doc.SavedText(),
		ScrollY:   t.state.ScrollY(),
		Pinned:    t.pinned,
		Preview:   t.preview,
		ReadOnly:  t.state.ReadOnly || t.doc.ReadOnly(),
	}
	if selections := t.state.Selections(); len(selections) > 0 {
		snapshot.Selections = append([]editor.Selection(nil), selections...)
	}
	return snapshot, true
}

// RestoreWindowTab applies a moved tab's unsaved text and selection to the
// already-open file in this window. The caller opens the path first so a
// missing project or document can cancel the move before anything changes.
func (m *Model) RestoreWindowTab(snapshot TabSnapshot, done func(error)) {
	if done == nil {
		done = func(error) {}
	}
	t, ok := m.tabByPathInAnyGroup(snapshot.Path)
	if !ok || t == nil || t.doc == nil || t.kind != "" || t.state == nil {
		done(fmt.Errorf("%s is not open in the target window", snapshot.Path))
		return
	}
	go func() {
		var err error
		if t.doc.Text() != snapshot.Text {
			ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
			err = t.doc.ApplyEdits(ctx, editor.Edit{
				Start: 0,
				End:   len(t.doc.Text()),
				Text:  snapshot.Text,
			})
			cancel()
		}
		m.post(func() {
			if err != nil {
				done(err)
				return
			}
			current, exists := m.tabByPathInAnyGroup(snapshot.Path)
			if !exists || current == nil || current.doc == nil || current.state == nil {
				done(errors.New("the moved tab closed before its state was restored"))
				return
			}
			current.doc.SetReadOnly(snapshot.ReadOnly)
			current.state.ReadOnly = snapshot.ReadOnly
			if len(snapshot.Selections) > 0 {
				current.state.SetSelections(snapshot.Selections...)
			}
			current.state.SetScrollY(snapshot.ScrollY)
			current.pinned = snapshot.Pinned
			current.preview = snapshot.Preview
			m.sortTabsAlphabetically()
			if i := m.tabIndex(current); i >= 0 {
				m.active = i
			}
			m.showingWelcome = false
			m.applyActive()
			m.syncProjectTreeState()
			m.saveSession()
			m.invalidate()
			done(nil)
		})
	}()
}

// RemoveTransferredWindowTab removes the source tab after its target loaded
// successfully. A text change while the target was opening cancels the move,
// keeping the newer source buffer intact.
func (m *Model) RemoveTransferredWindowTab(snapshot TabSnapshot) bool {
	t, ok := m.tabByPathInAnyGroup(snapshot.Path)
	if !ok || t == nil || t.doc == nil || t.kind != "" || t.doc.Text() != snapshot.Text {
		if ok && t != nil {
			m.status = "The tab changed while it was moving, so it stayed in this window"
			m.invalidate()
		}
		return false
	}
	m.removeTabFromWindow(t, true)
	m.status = "Moved " + t.doc.Path() + " to a new window"
	m.invalidate()
	return true
}
