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
	m.openPathsThen(paths, nil)
}

// OpenDroppedPaths is the OS drop entry point. A folder opens with the same
// project choice as other open actions. Files open as LightEdit documents;
// project files use the same editor and document registry.
func (m *Model) OpenDroppedPaths(paths []string) {
	m.openDroppedPathsWithFiles(paths, func(files []string) {
		m.openPathsThen(files, nil)
	})
}

func (m *Model) openDroppedPathsWithFiles(paths []string, openFiles func([]string)) {
	var files []string
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		clean := realPath(path)
		if info, err := os.Stat(clean); err == nil && info.IsDir() {
			m.openDroppedFolder(clean)
			return
		}
		files = append(files, clean)
	}
	if len(files) > 0 {
		openFiles(files)
	}
}

func (m *Model) openDroppedFolder(dir string) {
	if m.openDroppedFolderHook != nil {
		m.openDroppedFolderHook(dir)
		return
	}
	m.confirmSwitch(dir)
}

func (m *Model) openPathsThen(paths []string, done func()) {
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
			} else if done != nil {
				done()
			}
		})
		return
	}
	if dir != "" {
		m.confirmSwitch(dir)
		return
	}
	if done != nil {
		done()
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

type openProjectPathRequest struct {
	path      string
	newWindow *bool
}

// openProjectPath preserves the upstream action's two forms. A string follows
// the Open Project In policy; an object with a boolean newWindow is explicit:
// true always creates a window, false replaces this window after the normal
// unsaved and running-work guards.
func (m *Model) projectPathRequest(request openProjectPathRequest) {
	path := strings.TrimSpace(request.path)
	if path == "" {
		return
	}
	clean := strings.TrimSpace(path)
	if abs, err := filepath.Abs(clean); err == nil {
		clean = filepath.Clean(abs)
	}
	if request.newWindow == nil {
		m.confirmSwitch(clean)
		return
	}
	if *request.newWindow {
		if !m.openProjectInNativeWindow(clean, nil) && m.workspaceHost != nil {
			m.status = "Could not open another project window"
			m.invalidate()
		}
		return
	}
	m.confirmSwitchDirect(clean, nil)
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
	return openProjectPathRequestArg(arg).path
}

func openProjectPathRequestArg(arg any) openProjectPathRequest {
	switch value := arg.(type) {
	case string:
		return openProjectPathRequest{path: value}
	case map[string]any:
		path, _ := value["path"].(string)
		newWindow, ok := value["newWindow"].(bool)
		request := openProjectPathRequest{path: path}
		if ok {
			request.newWindow = &newWindow
		}
		return request
	default:
		return openProjectPathRequest{}
	}
}
