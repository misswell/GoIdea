package app

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// ProjectWindowRecord is one native project window restored at startup. The
// model's session file restores its tabs and unsaved-save decisions, while
// this record preserves window order and group membership.
type ProjectWindowRecord struct {
	ID    string `json:"id"`
	Root  string `json:"root,omitempty"`
	Group int    `json:"group,omitempty"`
}

// ProjectWindowWorkspace is the application-level list of native project
// windows. Active names the focused window within its group.
type ProjectWindowWorkspace struct {
	Version int                   `json:"version,omitempty"`
	Active  string                `json:"active,omitempty"`
	Windows []ProjectWindowRecord `json:"windows,omitempty"`
}

// LoadProjectWindowWorkspace reads the application window list. A missing or
// damaged file restores no extra windows.
func LoadProjectWindowWorkspace(path string) ProjectWindowWorkspace {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ProjectWindowWorkspace{}
	}
	var workspace ProjectWindowWorkspace
	if json.Unmarshal(raw, &workspace) != nil {
		return ProjectWindowWorkspace{}
	}
	return normalizeProjectWindowWorkspace(workspace)
}

// Save writes the window list atomically.
func (w ProjectWindowWorkspace) Save(path string) error {
	w = normalizeProjectWindowWorkspace(w)
	w.Version = 1
	raw, err := json.MarshalIndent(w, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, raw)
}

// ProjectWindowsRestoreEnabled applies the Open Project policy used by the
// main session to the application-level window list.
func ProjectWindowsRestoreEnabled(settingsPath string) bool {
	return LoadPreferences(settingsPath).reopenLastProject()
}

// ProjectWindowWorkspacePath is next to the primary session and settings.
func ProjectWindowWorkspacePath(sessionPath string) string {
	if sessionPath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(sessionPath), "windows.json")
}

func normalizeProjectWindowWorkspace(w ProjectWindowWorkspace) ProjectWindowWorkspace {
	seen := make(map[string]bool, len(w.Windows))
	windows := make([]ProjectWindowRecord, 0, len(w.Windows))
	for _, record := range w.Windows {
		if record.ID == "" || seen[record.ID] {
			continue
		}
		seen[record.ID] = true
		record.Root = filepath.Clean(record.Root)
		if record.Root == "." {
			record.Root = ""
		}
		windows = append(windows, record)
	}
	for i, record := range windows {
		if record.ID != "main" || i == 0 {
			continue
		}
		copy(windows[1:i+1], windows[0:i])
		windows[0] = record
		break
	}
	w.Windows = windows
	if len(windows) == 0 {
		w.Active = ""
		return w
	}
	found := false
	for _, record := range windows {
		if record.ID == w.Active {
			found = true
			break
		}
	}
	if !found {
		w.Active = windows[0].ID
	}
	return w
}
