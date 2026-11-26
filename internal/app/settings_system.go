package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
)

// openProjectWindowPolicy normalizes System Settings' three choices. Unknown
// and empty values follow OpenIDEA's Ask default.
func openProjectWindowPolicy(mode string) string {
	switch mode {
	case "new", "same":
		return mode
	default:
		return "ask"
	}
}

// saveSystemSettingsBackend writes the local-history limits to the backend's
// global settings. These two values govern the backend's pruning and quota,
// so keeping them only in GoIDEA's local preferences would leave the page
// cosmetic.
func (m *Model) saveSystemSettingsBackend(before, after SystemPrefs) {
	if m == nil || (m.mgr == nil && m.invokeHook == nil) {
		return
	}
	type write struct {
		key   string
		value int
	}
	var writes []write
	if before.localHistoryDays() != after.localHistoryDays() {
		writes = append(writes, write{key: "files.localHistoryDays", value: after.localHistoryDays()})
	}
	if before.localHistoryMaxMB() != after.localHistoryMaxMB() {
		writes = append(writes, write{key: "files.localHistoryMaxMb", value: after.localHistoryMaxMB()})
	}
	if len(writes) == 0 {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 30*time.Second)
		defer cancel()
		for _, write := range writes {
			_, err := m.invoke(ctx, "settings_set", map[string]any{
				"scope": "global",
				"key":   write.key,
				"value": jsonNumber(write.value),
			})
			if err != nil {
				key := write.key
				m.post(func() {
					m.settingsErr = "Could not save " + key + ": " + err.Error()
					m.invalidate()
				})
				return
			}
		}
	}()
}

func jsonNumber(value int) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}

// chooseDefaultProjectDirectory is System Settings' folder picker. The chosen
// folder pre-fills Open Project and the clone dialog, as OpenIDEA's
// general.defaultProjectDirectory does.
func (m *Model) chooseDefaultProjectDirectory() {
	start := strings.TrimSpace(m.settingsDraft.defaultProjectDirectory)
	parent := m.win
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: parent, Title: "Default Directory", ButtonLabel: "Choose",
			DefaultPath: start, Directory: true, CreateDirectories: true,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		dir := filepath.Clean(paths[0])
		m.post(func() {
			m.settingsDraft.defaultProjectDirectory = dir
			m.invalidate()
		})
	}()
}

// autoSaveOnSwitch is the autosave choice beside "switching to a different
// application or a built-in terminal". It starts each write independently so
// losing focus never blocks the native event loop.
func (m *Model) autoSaveOnSwitch() {
	if m == nil || !m.prefs.System.autoSave() || m.quitting {
		return
	}
	for _, tab := range append([]*Tab(nil), m.tabs...) {
		if tab != nil && tab.kind == "" && tab.unsaved() {
			m.saveTabAsync(tab, nil)
		}
	}
}

// syncExternalOnFocus refreshes files whose disk stamp changed while the
// window was inactive. Unsaved buffers keep their text and receive the same
// banner as the background watcher.
func (m *Model) syncExternalOnFocus(now time.Time) {
	if m == nil || !m.prefs.System.syncOnFocus() {
		return
	}
	m.watched = time.Time{}
	m.watchFiles(now)
}

// maybeSaveIdle writes dirty buffers after the configured inactive interval.
// It runs from the normal synchronization pump, so autosave follows edits
// without introducing another timer or goroutine.
func (m *Model) maybeSaveIdle(now time.Time) {
	if m == nil {
		return
	}
	p := m.prefs.System
	if !p.SaveOnIdle || m.quitting {
		return
	}
	idle := time.Duration(p.idleSeconds()) * time.Second
	for _, tab := range append([]*Tab(nil), m.tabs...) {
		if tab == nil || tab.kind != "" || !tab.unsaved() || tab.saving {
			continue
		}
		if tab.edited.IsZero() || now.Sub(tab.edited) < idle {
			continue
		}
		m.saveTabAsync(tab, nil)
	}
}

// runningWorkCount is the count OpenIDEA's close guard reports before it asks
// whether to terminate processes. Terminal sessions are included because the
// backend owns their PTYs too.
func (m *Model) runningWorkCount() int {
	if m == nil {
		return 0
	}
	count := 0
	if m.run.running || m.run.awaiting || m.run.busy {
		count++
	}
	for _, child := range m.run.children {
		if child != nil && child.live() {
			count++
		}
	}
	for _, run := range m.run.agentRuns {
		if run != nil && run.live() {
			count++
		}
	}
	count += m.liveDebugProcessCount()
	for _, child := range m.services.children {
		if child != nil && !child.finished && (child.starting || child.running || child.runID != 0) {
			count++
		}
	}
	for _, starting := range m.services.starting {
		if starting {
			count++
		}
	}
	count += m.term.total()
	return count
}

// liveDebugProcessCount follows OpenIDEA's root debug sessions. Adapter child
// sessions belong to their root process, while each independent Debug tab has
// its own process reservation.
func (m *Model) liveDebugProcessCount() int {
	if m == nil {
		return 0
	}
	live := map[uint32]bool{}
	if m.debug.parent == 0 && m.debug.session != 0 && !m.debug.ended {
		live[m.debug.session] = true
	}
	for _, state := range m.debug.sessions {
		if state == nil || state.parent != 0 || state.session == 0 || state.ended {
			continue
		}
		live[state.session] = true
	}
	if m.debug.starting {
		live[0] = true
	}
	return len(live)
}

// confirmQuitWithoutChanges applies general.confirmExit after the shared
// running-process guard has already settled.
func (m *Model) confirmQuitWithoutChanges() {
	if !m.prefs.System.confirmExit() {
		m.finishQuit(false)
		return
	}
	message := "Are you sure you want to quit GoIDEA?"
	go func() {
		res, err := mygo.Dialog.Message(mygo.MessageOptions{
			Parent: m.win, Type: mygo.MessageWarning, Title: "Quit GoIDEA",
			Message: message,
			Buttons: []string{"Quit", "Cancel"}, DefaultButton: 1, CancelButton: 1,
		})
		if err != nil {
			m.post(func() {
				m.quitting = false
				m.applicationQuit = false
				m.restartPending = false
			})
			return
		}
		m.post(func() {
			m.quitting = false
			m.applicationQuit = false
			if res.Button == 0 {
				m.finishQuit(false)
			} else {
				m.restartPending = false
			}
		})
	}()
}

// saveSessionWithoutEditorTabs persists the project, recents and window
// choices while honoring general.closeTabsWithWindow. It does not clear the
// live model, so the unsaved-change pipeline can still save the buffers that
// finishQuit received before the process exits.
func (m *Model) saveSessionWithoutEditorTabs() {
	if m.cfg.SessionPath == "" {
		return
	}
	s, ok := m.sessionSnapshot()
	if !ok {
		return
	}
	s = sessionWithoutEditorTabs(s)
	m.sessionMu.Lock()
	m.sessionSeq++
	seq := m.sessionSeq
	m.sessionMu.Unlock()
	m.writeSession(seq, m.cfg.SessionPath, s)
}

func sessionWithoutEditorTabs(s Session) Session {
	s.Files = nil
	s.Pinned = nil
	s.PreviewFiles = nil
	s.Focus = ""
	s.SplitFiles = nil
	s.SplitActive = 0
	s.SplitFirst = false
	s.SplitPreviewFiles = nil
	return s
}
