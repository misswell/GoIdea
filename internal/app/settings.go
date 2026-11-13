package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

// Session is what GoIDEA restores at the next start: the project folder, the
// files that had tabs, and the one that was showing.
type Session struct {
	Root  string   `json:"root,omitempty"`
	Files []string `json:"files,omitempty"`
	Focus string   `json:"focus,omitempty"`
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
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".session-*")
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
