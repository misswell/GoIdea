package app

import (
	"path/filepath"
	"strings"
)

func commitDraftKey(root string) string {
	if strings.TrimSpace(root) == "" {
		return ""
	}
	return filepath.Clean(root)
}

func (m *Model) loadCommitDraft(root string) string {
	key := commitDraftKey(root)
	if key == "" || m.prefs.VCS.CommitDrafts == nil {
		return ""
	}
	return m.prefs.VCS.CommitDrafts[key]
}

func (m *Model) keepCommitDraft(root, message string) {
	key := commitDraftKey(root)
	message = strings.TrimSpace(message)
	if key == "" {
		m.commitInitialMessage = message
		return
	}
	if m.prefs.VCS.CommitDrafts == nil {
		m.prefs.VCS.CommitDrafts = map[string]string{}
	}
	if message == "" {
		delete(m.prefs.VCS.CommitDrafts, key)
	} else {
		m.prefs.VCS.CommitDrafts[key] = message
	}
	m.commitInitialMessage = message
	m.savePreferences()
}
