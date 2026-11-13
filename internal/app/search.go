package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

// SearchPane is the Find window's project search: the query, the files the
// backend streamed back, and whether the search is still running.
type SearchPane struct {
	visible bool
	query   string
	channel uint32
	request uint32
	running bool
	// done is set by the summary; cancelled says the user stopped it early.
	done      bool
	cancelled bool
	// scopePath limits the search to one folder; empty means the whole project.
	scopePath string
	summary   string
	files     []openidea.FileMatches
	matches   int
	truncated bool
	err       string
}

const (
	maxSearchFiles   = 200
	maxSearchMatches = 2000
)

func (m *Model) resetSearch() { m.search = SearchPane{} }

// ShowSearch opens OpenIDEA's Find window in the bottom side: Enter in the
// query field runs it.
func (m *Model) ShowSearch() {
	m.search.scopePath = ""
	m.search.visible = true
	m.occupyBottom("find")
	m.invalidate()
}

// ShowSearchInFolder opens the Find window limited to dir, as Find/Replace in
// Folder does.
func (m *Model) ShowSearchInFolder(dir string) {
	if dir == "" {
		return
	}
	m.search.scopePath = dir
	m.search.visible = true
	m.occupyBottom("find")
	m.invalidate()
}

// FindInFolderSelected scopes Find to the folder selected in the project view,
// or to the project when the selection is not a folder.
func (m *Model) FindInFolderSelected() {
	dir := m.Root()
	if m.tree != nil {
		if sel := m.tree.Selected(); m.tree.IsDir(sel) {
			dir = sel
		}
	}
	if dir == m.Root() {
		m.ShowSearch()
		return
	}
	m.ShowSearchInFolder(dir)
}

// RunSearch starts a project-wide search, replacing any running one.
func (m *Model) RunSearch() {
	pattern := strings.TrimSpace(m.search.query)
	m.cancelSearch()
	if pattern == "" || m.Root() == "" {
		m.search.files = nil
		m.search.matches = 0
		m.search.summary = ""
		m.search.err = ""
		m.search.done = false
		m.invalidate()
		return
	}
	ch := m.nextChannel()
	m.search.channel = ch
	m.search.request = ch
	m.search.running = true
	m.search.done = false
	m.search.cancelled = false
	m.search.files = nil
	m.search.matches = 0
	m.search.truncated = false
	m.search.summary = ""
	m.search.err = ""
	m.invalidate()
	root := m.Root()
	scope := map[string]any{"kind": "project"}
	if m.search.scopePath != "" {
		scope = map[string]any{"kind": "directory", "path": m.search.scopePath}
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 5*time.Minute)
		defer cancel()
		_, err := m.mgr.Invoke(ctx, Window, "search_start", map[string]any{
			"request": ch,
			"query": map[string]any{
				"pattern": pattern, "isRegex": false, "caseSensitive": false,
				"wholeWord": false, "include": []string{}, "exclude": []string{},
				"scope": scope, "maxMatches": 0,
			},
			"onEvent": map[string]any{"$channel": ch},
		})
		m.post(func() {
			if m.search.request != ch {
				return
			}
			m.search.running = false
			if err != nil && m.search.err == "" {
				m.search.err = "搜索失败：" + err.Error()
			}
			if m.Root() != root {
				m.search.files = nil
			}
			m.invalidate()
		})
	}()
}

// cancelSearch stops a running search; its summary then reports cancelled.
func (m *Model) cancelSearch() {
	if !m.search.running || m.search.request == 0 {
		return
	}
	id := m.search.request
	m.search.running = false
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
		defer cancel()
		_, _ = m.mgr.Invoke(ctx, Window, "cancel_request", map[string]any{"id": id})
	}()
}

// acceptSearchEvent consumes one streamed search message.
func (m *Model) acceptSearchEvent(data json.RawMessage) {
	var ev openidea.SearchEvent
	if json.Unmarshal(data, &ev) != nil {
		return
	}
	switch ev.Type {
	case "file":
		if ev.File == nil || len(ev.File.Matches) == 0 {
			return
		}
		if len(m.search.files) >= maxSearchFiles || m.search.matches >= maxSearchMatches {
			m.search.truncated = true
			return
		}
		m.search.files = append(m.search.files, *ev.File)
		m.search.matches += len(ev.File.Matches)
	case "done":
		m.search.done = true
		if ev.Summary != nil {
			m.search.cancelled = ev.Summary.Cancelled
			m.search.summary = fmt.Sprintf("%d 个文件，%d 处匹配，%d ms",
				ev.Summary.Files, ev.Summary.Matches, int(ev.Summary.ElapsedMs))
		}
	}
	m.invalidate()
}

// revealPos is a caret position to apply when its file becomes active. The
// column is UTF-16, as search hits and language servers report it; it turns
// into the editor's byte column once the tab has the document open.
type revealPos struct {
	path      string
	line, col int
}

// revealAt opens a file in a regular tab and moves the caret to line and col
// (0-based).
func (m *Model) revealAt(path string, line, col int) {
	m.revealTo(path, line, col, false)
}

// previewAt opens a file in the preview tab and moves the caret to line and
// col, as a single click in a results list does.
func (m *Model) previewAt(path string, line, col int) {
	m.revealTo(path, line, col, true)
}

// revealTo opens path in a tab, preview or regular, and puts the caret where
// the navigation points.
func (m *Model) revealTo(path string, line, col int, preview bool) {
	m.reveal = &revealPos{path: path, line: line, col: col}
	if preview {
		m.openPreview(path)
	} else {
		m.openFile(path)
	}
	if t := m.tab(); t != nil && t.doc.Path() == path {
		m.applyReveal(t)
	}
}

// applyReveal moves the caret once the file a search hit points at is active.
func (m *Model) applyReveal(t *Tab) {
	if m.reveal == nil || t.doc.Path() != m.reveal.path {
		return
	}
	line, col := m.reveal.line, m.reveal.col
	m.reveal = nil
	line, col = t.doc.BytePos(line, col)
	t.state.GoTo(editor.Pos{Line: line, Col: col})
	t.state.ScrollTo(line)
	m.invalidate()
}

// SmokeSearch runs one real project search and reports what streamed back.
func (m *Model) SmokeSearch(ctx context.Context) (map[string]any, error) {
	out := map[string]any{}
	if m.Root() == "" {
		return out, errors.New("没有打开项目")
	}
	m.post(func() {
		m.search.query = "class "
		m.ShowSearch()
		m.RunSearch()
	})
	deadline := time.Now().Add(90 * time.Second)
	for {
		var done, running bool
		var files, matches int
		var summary, firstFile, firstText string
		var firstLine int
		m.post(func() {
			done, running = m.search.done, m.search.running
			files, matches, summary = len(m.search.files), m.search.matches, m.search.summary
			firstFile = m.search.firstRel()
			if f := m.search.firstFile(); f != nil && len(f.Matches) > 0 {
				firstLine = int(f.Matches[0].Line) + 1
				firstText = strings.TrimSpace(f.Matches[0].Text)
			}
		})
		if done || (!running && files == 0) || time.Now().After(deadline) {
			out["files"] = files
			out["matches"] = matches
			out["summary"] = summary
			out["done"] = done
			if running {
				out["timeout"] = true
			}
			out["firstFile"] = firstFile
			out["firstLine"] = firstLine
			out["firstText"] = firstText
			return out, nil
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
}

// firstFile is the first file with hits, or nil.
func (p *SearchPane) firstFile() *openidea.FileMatches {
	if len(p.files) == 0 {
		return nil
	}
	return &p.files[0]
}

// firstRel is the first hit file's project-relative path.
func (p *SearchPane) firstRel() string {
	if f := p.firstFile(); f != nil {
		return f.RelPath
	}
	return ""
}
