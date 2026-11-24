package app

import (
	"fmt"
	"path/filepath"
)

// DetachWindowTab is OpenIDEA's window.detachTab. The desktop host opens a new
// native project window and restores the file's unsaved text, selection and
// scroll before removing the source tab. A standalone model keeps the older
// editor-group fallback used by tests and alternate front ends.
func (m *Model) DetachWindowTab(arg any) {
	t := m.tab()
	if arg != nil {
		if path, ok := arg.(string); ok && path != "" {
			t, _ = m.tabByPathInAnyGroup(path)
		}
	}
	if t == nil {
		return
	}
	if t.doc == nil || t.kind != "" {
		m.status = "Only file tabs can move to another project window"
		m.invalidate()
		return
	}
	snapshot, snapshotOK := m.CaptureWindowTab(t.doc.Path())
	if !snapshotOK {
		m.status = "This tab's editor state cannot move to another project window"
		m.invalidate()
		return
	}
	if m.detachTabToNativeWindow(snapshot, func(targetReady bool) bool {
		if !targetReady {
			m.status = "Could not move " + filepath.Base(snapshot.Path) + "; the tab stayed open"
			m.invalidate()
			return false
		}
		return m.RemoveTransferredWindowTab(snapshot)
	}) {
		m.status = "Moving " + filepath.Base(snapshot.Path) + " to a new window…"
		m.invalidate()
		return
	}
	if m.workspaceHost != nil {
		m.status = "Could not create another project window; the tab stayed open"
		m.invalidate()
		return
	}
	if m.split == nil {
		if len(m.tabs) < 2 {
			m.status = "Move another tab here first; the last project tab cannot leave its window group"
			m.invalidate()
			return
		}
		m.split = &splitGroup{}
		m.splitFirst = false
	}
	from, ok := m.groupOf(t)
	if !ok || from == groupSplit {
		if from == groupSplit {
			m.moveToGroup(t, groupFocused)
		}
		return
	}
	m.moveToGroup(t, groupSplit)
	m.status = "Tab moved to a new window group"
	m.invalidate()
}

// MergeAllWindowGroups groups every native project window while every model
// and backend stays alive. With no native host, it keeps the editor-group
// fallback used by tests and alternate front ends.
func (m *Model) MergeAllWindowGroups() {
	if m.mergeNativeProjectWindows() {
		m.status = "Merged all project windows"
		m.invalidate()
		return
	}
	if m.workspaceHost != nil {
		m.status = "Open at least two project windows before merging them"
		m.invalidate()
		return
	}
	if m.split == nil {
		m.status = "There are no separate window groups to merge"
		m.invalidate()
		return
	}
	active := m.tab()
	m.tabs = append(m.tabs, m.split.tabs...)
	m.active = tabIndexIn(m.tabs, active)
	if m.active < 0 {
		m.active = max(0, min(m.active, len(m.tabs)-1))
	}
	m.split = nil
	m.splitFirst = false
	m.applyActive()
	m.saveSession()
	m.status = fmt.Sprintf("Merged all window groups (%d tabs)", len(m.tabs))
	m.invalidate()
}

// CycleWindowGroup focuses the next or previous native project window. Models
// without a multi-window host retain the editor split-group cycle.
func (m *Model) CycleWindowGroup(previous bool) {
	if m.focusNativeProjectWindow(previous) {
		m.status = "Focused another project window"
		m.invalidate()
		return
	}
	if m.split == nil {
		m.status = "There is no other window group"
		m.invalidate()
		return
	}
	if previous {
		m.GoToPreviousSplitter()
	} else {
		m.GoToNextSplitter()
	}
	m.status = "Focused the other window group"
	m.invalidate()
}
