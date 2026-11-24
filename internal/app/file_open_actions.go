package app

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
)

// OpenFile asks for one or more files and opens them in the editor. Files
// outside the open project use LightEdit's local document backend.
func (m *Model) OpenFile() {
	go func() {
		opts := mygo.OpenDialogOptions{
			Parent:      m.win,
			Title:       "Open File",
			ButtonLabel: "Open",
			Multiple:    true,
		}
		if root := m.Root(); root != "" {
			opts.DefaultPath = root
		} else if t := m.tab(); t != nil && t.doc != nil {
			opts.DefaultPath = filepath.Dir(t.doc.Path())
		}
		paths, err := mygo.Dialog.Open(opts)
		if err != nil {
			m.post(func() { m.fail("", err) })
			return
		}
		if len(paths) > 0 {
			m.post(func() { m.openPaths(paths) })
		}
	}()
}

// openPaths handles files and dropped folders like OpenIDEA: files open as
// documents, and the first folder opens as the current project.
func (m *Model) openPaths(paths []string) {
	var files []string
	var dir string
	seen := make(map[string]bool)
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		clean := realPath(path)
		if seen[clean] {
			continue
		}
		seen[clean] = true
		info, err := os.Stat(clean)
		switch {
		case err == nil && info.IsDir():
			if dir == "" {
				dir = clean
			}
		case err == nil:
			files = append(files, clean)
		default:
			m.fail(clean, err)
		}
	}
	files, associated := partitionAssociatedPaths(files, m.associatedApplicationPatterns())
	for _, path := range associated {
		if err := mygo.Shell.OpenPath(path); err != nil {
			m.fail(path, err)
		}
	}
	if len(files) > 0 {
		m.openBatch(files, func() {
			if dir != "" {
				m.confirmSwitch(dir)
			}
		})
		return
	}
	if dir != "" {
		m.confirmSwitch(dir)
	}
}

func (m *Model) openBatch(paths []string, done func()) {
	var next func(int)
	next = func(i int) {
		if i >= len(paths) {
			if done != nil {
				done()
			}
			return
		}
		advanced := false
		advance := func() {
			if advanced {
				return
			}
			advanced = true
			next(i + 1)
		}
		m.openWith(paths[i], openSpec{
			at:     -1,
			done:   func(*Tab) { advance() },
			failed: func(string, error) { advance() },
		})
	}
	next(0)
}

func (m *Model) openProjectPath(path string) {
	if strings.TrimSpace(path) != "" {
		m.confirmSwitch(realPath(path))
	}
}

func stringPaths(arg any) []string {
	switch value := arg.(type) {
	case string:
		if value != "" {
			return []string{value}
		}
	case []string:
		return value
	case []any:
		paths := make([]string, 0, len(value))
		for _, item := range value {
			if path, ok := item.(string); ok && path != "" {
				paths = append(paths, path)
			}
		}
		return paths
	}
	return nil
}

func projectPathArg(arg any) string {
	switch value := arg.(type) {
	case string:
		return value
	case map[string]any:
		path, _ := value["path"].(string)
		return path
	default:
		return ""
	}
}
