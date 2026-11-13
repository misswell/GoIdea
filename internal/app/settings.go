package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"

	"github.com/egoist/mygo/ui"
)

// Session is what GoIDEA restores at the next start: the project folder, the
// files that had tabs, the one that was showing, and its bookmarks.
type Session struct {
	Root      string     `json:"root,omitempty"`
	Files     []string   `json:"files,omitempty"`
	Focus     string     `json:"focus,omitempty"`
	Bookmarks []Bookmark `json:"bookmarks,omitempty"`
}

// LoadSession reads the session at path; a missing or unreadable file is an
// empty session, so a broken file never keeps the app from starting.
func LoadSession(path string) Session {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Session{}
	}
	var s Session
	if json.Unmarshal(raw, &s) != nil {
		return Session{}
	}
	return s
}

// Save writes the session atomically: a crash mid-write leaves the previous
// session intact instead of a truncated file.
func (s Session) Save(path string) error {
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, raw)
}

// Preferences are the settings the user changes in the settings dialog. The
// zero value is the default: follow the system's appearance and the editor's
// own font size.
type Preferences struct {
	Theme    string  `json:"theme,omitempty"`
	FontSize float32 `json:"fontSize,omitempty"`
}

const (
	defaultFontSize = 13
	minFontSize     = 10
	maxFontSize     = 24
)

// LoadPreferences reads the settings at path; a missing or unreadable file is
// the default, so a broken file never keeps the app from starting.
func LoadPreferences(path string) Preferences {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Preferences{}
	}
	var p Preferences
	if json.Unmarshal(raw, &p) != nil {
		return Preferences{}
	}
	return p
}

// Save writes the preferences atomically, like the session.
func (p Preferences) Save(path string) error {
	raw, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, raw)
}

// themePreference maps a stored value to the theme GoIDEA draws, or nil to
// follow the system's appearance.
func (p Preferences) themePreference() *ui.Theme {
	switch p.Theme {
	case "light":
		return ui.LightTheme()
	case "dark":
		return ui.DarkTheme()
	}
	return nil
}

// fontSize returns the editor font size to use, with the default standing in
// for the unset value.
func (p Preferences) fontSize() float32 {
	if p.FontSize == 0 {
		return defaultFontSize
	}
	return p.FontSize
}

// writeFileAtomic replaces path in one rename, so a crash mid-write leaves
// the previous content intact.
func writeFileAtomic(path string, raw []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".settings-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

// normalizeFiles sorts and deduplicates the paths of a session, keeping only
// the files that still exist.
func normalizeFiles(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	var out []string
	for _, p := range paths {
		if p == "" || seen[p] {
			continue
		}
		if st, err := os.Stat(p); err != nil || st.IsDir() {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}
