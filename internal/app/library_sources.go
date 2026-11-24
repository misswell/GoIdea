package app

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

// librarySourceState is one class file's source lookup. Both Download Sources
// and Choose Sources use the same entry so a second request cannot race the
// first, and the banner can show OpenIDEA's Loading and Cancel states.
type librarySourceState struct {
	checking    bool
	checked     bool
	canDownload bool
	checkErr    string
	busy        bool
	message     string
	request     uint32
	cancel      context.CancelFunc
	root        string
	generation  uint64
}

func init() {
	register := func(id, title, keywords string, run func(*Model, any)) {
		_ = registerAction(&Action{
			ID:         id,
			Title:      title,
			Category:   "Code",
			Keywords:   keywords,
			RunWithArg: run,
			When:       func(m *Model) bool { return m.librarySourcePath("") != "" },
		})
	}
	register(
		"editor.downloadSources",
		"Download Sources",
		"library class dependency source maven decompiled download",
		func(m *Model, arg any) { m.retrieveLibrarySources(arg, false) },
	)
	register(
		"editor.chooseSources",
		"Choose Sources…",
		"library class dependency source archive jar zip decompiled choose",
		func(m *Model, arg any) { m.retrieveLibrarySources(arg, true) },
	)
	register(
		"editor.cancelSourceDownload",
		"Cancel",
		"cancel library class dependency source download",
		func(m *Model, arg any) { m.cancelLibrarySource(arg) },
	)
}

func (m *Model) ensureLibrarySources() {
	if m.librarySources == nil {
		m.librarySources = map[string]*librarySourceState{}
	}
}

func (m *Model) librarySourceState(path string) *librarySourceState {
	m.ensureLibrarySources()
	state := m.librarySources[path]
	if state == nil {
		state = &librarySourceState{}
		m.librarySources[path] = state
	}
	return state
}

// librarySourcePath resolves OpenIDEA's action argument: the explicit path, or
// the active editor's path. Only decompiled library classes have these actions.
func (m *Model) librarySourcePath(arg any) string {
	path, _ := arg.(string)
	if strings.TrimSpace(path) == "" {
		if t := m.tab(); t != nil && t.doc != nil {
			path = t.doc.Path()
		}
	}
	path = strings.TrimSpace(path)
	if !isLibraryClassPath(path) {
		return ""
	}
	return path
}

func isLibraryClassPath(path string) bool {
	lower := strings.ToLower(strings.TrimSpace(path))
	if !strings.HasSuffix(strings.SplitN(lower, "?", 2)[0], ".class") {
		return false
	}
	return strings.HasPrefix(lower, "jdt:") ||
		strings.HasPrefix(lower, "jar:") ||
		strings.Contains(lower, "!/") && strings.HasSuffix(strings.SplitN(lower, "?", 2)[0], ".class")
}

// ensureLibrarySourceSupport asks the backend whether this class belongs to a
// Maven artifact whose sources can be downloaded. It runs once per path and
// project root; a project switch drops the old answer.
func (m *Model) ensureLibrarySourceSupport(path string) *librarySourceState {
	state := m.librarySourceState(path)
	root := m.Root()
	if state.checked || state.checking {
		if state.root == root {
			return state
		}
		state.checked = false
		state.checking = false
		state.canDownload = false
		state.checkErr = ""
		state.message = ""
	}
	state.root = root
	if root == "" || !m.run.trusted {
		state.checked = true
		return state
	}
	state.checking = true
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "lsp_can_download_sources", map[string]any{"uri": path})
		var supported bool
		if err == nil {
			err = json.Unmarshal(raw, &supported)
		}
		m.post(func() {
			if state.root != root {
				return
			}
			state.checking = false
			state.checked = true
			state.canDownload = err == nil && supported
			if err != nil && !isCancelledError(err) {
				state.checkErr = err.Error()
			}
			m.invalidate()
		})
	}()
	return state
}

// retrieveLibrarySources ports OpenIDEA's retrieve(path, choose). The source
// archive picker runs only after the user chooses sources; otherwise the Maven
// backend resolves and downloads the matching sources artifact.
func (m *Model) retrieveLibrarySources(arg any, choose bool) {
	path := m.librarySourcePath(arg)
	if path == "" {
		return
	}
	state := m.librarySourceState(path)
	if state.busy {
		return
	}
	if m.Root() == "" || !m.run.trusted {
		m.notifyWarning("editor.downloadSources", "Project is not trusted", "Trust this project before loading library sources.")
		return
	}
	root := m.Root()
	pos := openidea.OutlinePos{}
	if t := m.tab(); t != nil && t.doc != nil && t.doc.Path() == path {
		pos = t.doc.UTF16Pos(t.state.Cursor())
	}
	if choose {
		go m.chooseLibrarySources(path, root, pos)
		return
	}
	m.startLibrarySourceRequest(path, root, pos, nil)
}

func (m *Model) chooseLibrarySources(path, root string, pos openidea.OutlinePos) {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent: m.win,
		Title:  "Choose Sources…",
		Filters: []mygo.FileFilter{{
			Name: "Source archives", Extensions: []string{"jar", "zip"},
		}},
	})
	if err != nil || len(paths) == 0 {
		return
	}
	m.post(func() {
		if m.Root() != root {
			return
		}
		sourceArchive := paths[0]
		m.startLibrarySourceRequest(path, root, pos, &sourceArchive)
	})
}

func (m *Model) startLibrarySourceRequest(
	path, root string,
	pos openidea.OutlinePos,
	sources *string,
) {
	state := m.librarySourceState(path)
	if state.busy {
		return
	}
	request := m.nextRequest()
	ctx, cancel := context.WithTimeout(m.ctx, 5*time.Minute)
	state.busy = true
	state.message = ""
	state.request = request
	state.cancel = cancel
	state.root = root
	state.generation++
	generation := state.generation
	m.invalidate()
	go func() {
		defer cancel()
		raw, err := m.invoke(ctx, "lsp_library_sources", map[string]any{
			"request": request,
			"uri":     path,
			"pos":     lspPos(pos),
			"sources": sources,
		})
		var location *openidea.Location
		if err == nil {
			if strings.TrimSpace(string(raw)) != "" && strings.TrimSpace(string(raw)) != "null" {
				err = json.Unmarshal(raw, &location)
			}
		}
		m.post(func() {
			if state.generation != generation {
				return
			}
			state.busy = false
			state.request = 0
			state.cancel = nil
			if m.Root() != root {
				return
			}
			if err != nil {
				if !isCancelledError(err) {
					message := "Cannot load library sources"
					state.message = message
					m.notifyError("editor.downloadSources", message, err.Error())
				}
				m.invalidate()
				return
			}
			if location == nil || strings.TrimSpace(location.Path) == "" {
				state.message = "Sources not found"
				m.invalidate()
				return
			}
			m.finishLibrarySourceDownload(path, *location)
		})
	}()
}

func (m *Model) finishLibrarySourceDownload(original string, location openidea.Location) {
	m.openFileThen(location.Path, func(t *Tab) {
		line, col := t.doc.BytePos(int(location.Range.Start.Line), int(location.Range.Start.Col))
		t.state.GoTo(editorPos(line, col))
		t.state.ScrollTo(line)
		for _, originalTab := range m.allTabs() {
			if originalTab != t && originalTab.doc != nil && originalTab.doc.Path() == original {
				if idx := m.tabIndex(originalTab); idx >= 0 {
					m.closeTab(idx)
				}
				break
			}
		}
		m.invalidate()
	})
}

func (m *Model) cancelLibrarySource(arg any) {
	path := m.librarySourcePath(arg)
	if path == "" {
		return
	}
	state := m.librarySources[path]
	if state == nil || !state.busy {
		return
	}
	if state.request != 0 {
		request := state.request
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, _ = m.invoke(ctx, "cancel_request", map[string]any{"request": request})
		}()
	}
	if state.cancel != nil {
		state.cancel()
	}
	state.generation++
	state.busy = false
	state.request = 0
	state.cancel = nil
	state.message = ""
	m.invalidate()
}

// librarySourcesBanner is OpenIDEA's decompiled-class bar above the editor.
// It shows the read-only reason, download support, the active request's Cancel
// affordance, and the backend's "Sources not found" result.
func (m *Model) librarySourcesBanner(c *ui.Context, p *ui.Theme, t *Tab) {
	if t == nil || t.doc == nil || !isLibraryClassPath(t.doc.Path()) {
		return
	}
	path := t.doc.Path()
	state := m.ensureLibrarySourceSupport(path)
	ui.Row(c.Key("library-source-banner:"+path)).Shrink(0).
		Padding(7, 10).Gap(8).AlignItems(ui.Center).
		Background(p.Surface).BorderWidth(0, 0, 1, 0).BorderColor(p.Border).
		Children(func() {
			ui.Text(c, "◈").FontSize(12).TextColor(p.Accent)
			ui.Text(c, "Decompiled .class file: sources are not available.").
				FontSize(12).TextColor(p.Text).SingleLine().Ellipsis("…").Grow(1)
			if state.message != "" {
				ui.Text(c, state.message).FontSize(12).
					TextColor(p.Warning).SingleLine().Ellipsis("…")
			}
			if state.busy {
				ui.Text(c, "Loading library sources…").FontSize(12).TextColor(p.TextMuted)
				ui.Button(c, "Cancel").OnClick(func() {
					m.RunActionWithArg("editor.cancelSourceDownload", path)
				})
				return
			}
			if state.canDownload {
				ui.Button(c, "Download Sources").OnClick(func() {
					m.RunActionWithArg("editor.downloadSources", path)
				})
			}
			ui.Button(c, "Choose Sources…").OnClick(func() {
				m.RunActionWithArg("editor.chooseSources", path)
			})
			if state.checkErr != "" {
				ui.Text(c, state.checkErr).FontSize(11).
					TextColor(p.TextMuted).SingleLine().Ellipsis("…")
			}
		})
}

// editorPos keeps the import of the editor package out of the rest of this
// file's payload code.
func editorPos(line, col int) editor.Pos {
	return editor.Pos{Line: line, Col: col}
}
