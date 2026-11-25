package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
)

const (
	tabKindNotebook        = "notebook"
	notebookTypingDelay    = 250 * time.Millisecond
	notebookPreviewLimit   = 200_000
	notebookInternalRadius = 6
)

type notebookState struct {
	sessions map[string]*notebookSession
}

type notebookSession struct {
	path string

	snapshot openidea.NotebookSnapshot
	loaded   bool
	loading  bool
	busy     bool
	running  bool
	error    string
	kernel   string
	python   string
	envs     []openidea.Sdk

	request uint32
	channel uint32
	loadSeq uint64
	runSeq  uint64

	pending     map[string]string
	editSeq     map[string]uint64
	editBusy    bool
	closeAfter  bool
	closeTab    *Tab
	streaming   map[string]string
	markdown    map[string]bool
	selected    string
	closeAsk    bool
	outputCache map[string]*ui.Bitmap
	editors     map[string]*notebookCellEditor
	cellDocs    map[string]openidea.NotebookCellDoc
	docsLoading bool
	docsLoaded  bool
	docsSeq     uint64
	docsRetry   time.Time
	cellDiags   map[string]map[string][]openidea.Diagnostic
}

type notebookCellEditor struct {
	state    *editor.State
	seen     uint64
	comp     uint32
	compGen  uint64
	hoverGen uint64
	defGen   uint64
}

type notebookTabSnapshot struct {
	session *notebookSession
	tab     *Tab
}

func isNotebookPath(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".ipynb")
}

func partitionNotebookPaths(paths []string) (files, notebooks []string) {
	files = make([]string, 0, len(paths))
	for _, path := range paths {
		if isNotebookPath(path) {
			notebooks = append(notebooks, realPath(path))
			continue
		}
		files = append(files, path)
	}
	return files, notebooks
}

func (m *Model) notebook(path string) *notebookSession {
	path = realPath(path)
	if m.notebooks.sessions == nil {
		m.notebooks.sessions = make(map[string]*notebookSession)
	}
	if session := m.notebooks.sessions[path]; session != nil {
		return session
	}
	session := &notebookSession{
		path:        path,
		pending:     make(map[string]string),
		editSeq:     make(map[string]uint64),
		streaming:   make(map[string]string),
		markdown:    make(map[string]bool),
		outputCache: make(map[string]*ui.Bitmap),
		editors:     make(map[string]*notebookCellEditor),
		cellDocs:    make(map[string]openidea.NotebookCellDoc),
		cellDiags:   make(map[string]map[string][]openidea.Diagnostic),
	}
	m.notebooks.sessions[path] = session
	return session
}

func (m *Model) newNotebookTab(path, title string, preview bool) *Tab {
	path = realPath(path)
	if title == "" {
		title = filepath.Base(path)
	}
	doc := docbridge.Virtual(path, "")
	size := m.prefs.fontSize()
	font := m.prefs.Font
	tab := &Tab{
		doc: doc,
		state: &editor.State{
			Buffer:     doc.Buffer(),
			LineHeight: font.lineHeightPixels(size) / size,
			Bindings:   m.editorBindings(),
			ReadOnly:   true,
		},
		kind:      tabKindNotebook,
		pageTitle: title,
		pagePath:  path,
		preview:   preview,
	}
	tab.state.Font = ui.Font{
		Family: font.fontFamily(), Size: size,
		Weight: font.weight(), Features: font.features(),
	}
	tab.synced = doc.Buffer().Version()
	tab.seen = tab.synced
	return tab
}

func (m *Model) openNotebook(path string, spec openSpec) {
	path = realPath(path)
	session := m.notebook(path)
	m.showingWelcome = false
	tab := m.newNotebookTab(path, filepath.Base(path), spec.preview && m.tabPrefs.previews())
	tab.pinned = spec.pinned
	switch {
	case spec.at >= 0:
		m.dropPreviewTabs()
		m.insertTabAt(tab, spec.at)
	case tab.preview && m.previewIndex() >= 0:
		m.replacePreviewTab(m.previewIndex(), tab)
	default:
		if !tab.preview {
			m.dropPreviewTabs()
		}
		m.insertTab(tab)
	}
	m.active = m.tabIndex(tab)
	m.sortTabsAlphabetically()
	m.active = m.tabIndex(tab)
	m.enforceTabLimit()
	m.active = m.tabIndex(tab)
	m.revealEditor()
	m.applyActive()
	m.syncProjectTreeState()
	m.markRecent(path)
	m.saveSession()
	m.finishOpen(spec, tab)
	m.pumpOpenQueue()
	m.invalidate()
	if !session.loaded && !session.loading {
		m.loadNotebook(session)
	}
}

func (m *Model) loadNotebook(session *notebookSession) {
	if session == nil || session.loading {
		return
	}
	if m.ctx == nil || (m.mgr == nil && m.invokeHook == nil) {
		session.error = "Notebook editing and execution require the GoIDEA backend"
		m.invalidate()
		return
	}
	session.loading = true
	session.error = ""
	session.loadSeq++
	seq := session.loadSeq
	root := m.Root()
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "notebook_open", map[string]any{"path": session.path})
		var snapshot openidea.NotebookSnapshot
		if err == nil {
			err = json.Unmarshal(raw, &snapshot)
		}
		var envs []openidea.Sdk
		var python string
		if err == nil {
			if rawEnvs, envErr := m.invoke(ctx, "sdk_detect", map[string]any{"kind": "python"}); envErr == nil {
				_ = json.Unmarshal(rawEnvs, &envs)
			}
			if rawPython, envErr := m.invoke(ctx, "sdk_python_effective", map[string]any{"near": session.path}); envErr == nil {
				_ = json.Unmarshal(rawPython, &python)
			}
		}
		m.post(func() {
			if session.loadSeq != seq || m.Root() != root || m.notebooks.sessions[session.path] != session {
				return
			}
			session.loading = false
			if err != nil {
				session.error = "Could not open notebook: " + err.Error()
				m.invalidate()
				return
			}
			session.loaded = true
			session.envs = envs
			if session.python == "" {
				session.python = python
			}
			m.acceptNotebookSnapshot(session, snapshot)
		})
	}()
}

func (m *Model) acceptNotebookSnapshot(session *notebookSession, snapshot openidea.NotebookSnapshot) {
	if session == nil || snapshot.ID == "" {
		return
	}
	if session.snapshot.ID != "" && snapshot.ID != session.snapshot.ID {
		return
	}
	if session.snapshot.Version > snapshot.Version {
		return
	}
	session.snapshot = snapshot
	session.loaded = true
	session.error = ""
	for _, tab := range m.allTabs() {
		if tab != nil && tab.kind == tabKindNotebook && samePath(tab.pagePath, session.path) {
			tab.pageDirty = snapshot.Dirty || len(session.pending) > 0
			tab.pageTitle = filepath.Base(session.path)
		}
	}
	for cell, source := range session.pending {
		for i := range session.snapshot.Cells {
			if session.snapshot.Cells[i].ID == cell {
				session.snapshot.Cells[i].Source = source
				break
			}
		}
	}
	for i := range session.snapshot.Cells {
		if session.snapshot.Cells[i].CellType != "markdown" {
			delete(session.markdown, session.snapshot.Cells[i].ID)
		} else if _, exists := session.markdown[session.snapshot.Cells[i].ID]; !exists {
			session.markdown[session.snapshot.Cells[i].ID] = false
		}
	}
	if session.selected == "" && len(session.snapshot.Cells) > 0 {
		for _, cell := range session.snapshot.Cells {
			if cell.CellType == "code" {
				session.selected = cell.ID
				break
			}
		}
		if session.selected == "" {
			session.selected = session.snapshot.Cells[0].ID
		}
	}
	m.syncNotebookCellEditors(session)
	m.pruneNotebookCellDocs(session)
	if notebookCellDocsMissing(session) {
		session.docsLoaded = false
	}
	m.refreshNotebookCellDocs(session)
	m.showNotebookCellDiagnostics(session)
	m.invalidate()
}

func (m *Model) syncNotebookCellEditors(session *notebookSession) {
	live := make(map[string]bool, len(session.snapshot.Cells))
	for _, cell := range session.snapshot.Cells {
		live[cell.ID] = true
		view := session.editors[cell.ID]
		if view == nil {
			continue
		}
		if _, pending := session.pending[cell.ID]; pending {
			continue
		}
		if view.state.Buffer.String() != cell.Source {
			view.state.Buffer.Reset(cell.Source)
			view.seen = view.state.Buffer.Version()
		}
	}
	for cell := range session.editors {
		if !live[cell] {
			delete(session.editors, cell)
		}
	}
}

func (m *Model) pruneNotebookCellDocs(session *notebookSession) {
	codeCells := make(map[string]bool)
	for _, cell := range session.snapshot.Cells {
		if cell.CellType == "code" {
			codeCells[cell.ID] = true
		}
	}
	for cell := range session.cellDocs {
		if !codeCells[cell] {
			delete(session.cellDocs, cell)
			delete(session.cellDiags, cell)
		}
	}
}

func (m *Model) applyNotebookDiagnostics(ev openidea.DiagnosticsEvent) bool {
	matched := false
	for _, session := range m.notebooks.sessions {
		if session == nil {
			continue
		}
		for cell, doc := range session.cellDocs {
			if doc.URI != ev.Path {
				continue
			}
			matched = true
			per := session.cellDiags[cell]
			if len(ev.Items) == 0 {
				if per == nil {
					continue
				}
				delete(per, ev.Source)
			} else {
				if per == nil {
					per = make(map[string][]openidea.Diagnostic)
					session.cellDiags[cell] = per
				}
				per[ev.Source] = ev.Items
			}
			if len(per) == 0 {
				delete(session.cellDiags, cell)
			}
			m.showNotebookCellDiagnostics(session)
		}
	}
	return matched
}

func (m *Model) showNotebookCellDiagnostics(session *notebookSession) {
	for cell, view := range session.editors {
		per := session.cellDiags[cell]
		version := view.state.Buffer.Version()
		if len(per) == 0 {
			view.state.SetDiagnostics(version, nil)
			continue
		}
		text := view.state.Buffer.String()
		sources := make([]string, 0, len(per))
		for source := range per {
			sources = append(sources, source)
		}
		sort.Strings(sources)
		var diagnostics []editor.Diagnostic
		for _, source := range sources {
			for _, item := range per[source] {
				start := notebookByteOffset(text, item.Range.Start)
				end := notebookByteOffset(text, item.Range.End)
				if end < start {
					end = start
				}
				diagnostics = append(diagnostics, editor.Diagnostic{
					Start:    start,
					End:      end,
					Severity: diagSeverity(item.Severity),
					Message:  item.Message,
					Source:   diagSource(source, item),
				})
			}
		}
		view.state.SetDiagnostics(version, diagnostics)
	}
}

func notebookCellDocsMissing(session *notebookSession) bool {
	if session == nil || session.snapshot.ID == "" {
		return false
	}
	for _, cell := range session.snapshot.Cells {
		if cell.CellType == "code" && session.cellDocs[cell.ID].Doc == 0 {
			return true
		}
	}
	return false
}

func (m *Model) refreshNotebookCellDocs(session *notebookSession) {
	if session == nil || session.docsLoading || session.docsLoaded ||
		session.snapshot.ID == "" || time.Now().Before(session.docsRetry) ||
		!notebookCellDocsMissing(session) {
		return
	}
	if m.ctx == nil || m.mgr == nil && m.invokeHook == nil {
		return
	}
	session.docsLoading = true
	session.docsSeq++
	seq := session.docsSeq
	id := session.snapshot.ID
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "notebook_cell_docs", map[string]any{"id": id})
		var docs []openidea.NotebookCellDoc
		if err == nil {
			err = json.Unmarshal(raw, &docs)
		}
		m.post(func() {
			if m.notebooks.sessions[session.path] != session || session.docsSeq != seq {
				return
			}
			session.docsLoading = false
			if err != nil {
				session.docsRetry = time.Now().Add(lspBackoff)
				return
			}
			session.docsRetry = time.Time{}
			session.cellDocs = make(map[string]openidea.NotebookCellDoc, len(docs))
			for _, doc := range docs {
				if doc.Cell != "" && doc.Doc != 0 && doc.URI != "" {
					session.cellDocs[doc.Cell] = doc
				}
			}
			session.docsLoaded = true
			m.showNotebookCellDiagnostics(session)
			m.invalidate()
		})
	}()
}

func (m *Model) saveNotebookSync(ctx context.Context, tab *Tab) error {
	if tab == nil || tab.kind != tabKindNotebook {
		return nil
	}
	session := m.notebooks.sessions[realPath(tab.pagePath)]
	if session == nil || session.snapshot.ID == "" {
		if tab.pageDirty {
			return fmt.Errorf("notebook %s is not loaded", filepath.Base(tab.pagePath))
		}
		return nil
	}
	for {
		if !session.editBusy {
			cell, source, ok := nextNotebookPending(session)
			if !ok {
				break
			}
			delete(session.pending, cell)
			raw, err := m.invoke(ctx, "notebook_edit", map[string]any{
				"id": session.snapshot.ID, "version": session.snapshot.Version,
				"edit": openidea.NotebookEdit{Type: "setSource", Cell: cell, Source: source},
			})
			if err != nil {
				session.pending[cell] = source
				return err
			}
			var edited openidea.NotebookSnapshot
			if err := json.Unmarshal(raw, &edited); err != nil {
				session.pending[cell] = source
				return err
			}
			session.snapshot = edited
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
	if !session.snapshot.Dirty {
		return nil
	}
	raw, err := m.invoke(ctx, "notebook_save", map[string]any{"id": session.snapshot.ID})
	if err != nil {
		return err
	}
	var snapshot openidea.NotebookSnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return err
	}
	m.post(func() { m.acceptNotebookSnapshot(session, snapshot) })
	return nil
}

func nextNotebookPending(session *notebookSession) (string, string, bool) {
	for cell, source := range session.pending {
		return cell, source, true
	}
	return "", "", false
}

func (m *Model) notebookCellChanged(session *notebookSession, cell, source string) {
	if session == nil || session.running || session.snapshot.ID == "" {
		return
	}
	session.pending[cell] = source
	for _, tab := range m.allTabs() {
		if tab != nil && tab.kind == tabKindNotebook && samePath(tab.pagePath, session.path) {
			tab.pageDirty = true
		}
	}
	session.editSeq[cell]++
	seq := session.editSeq[cell]
	go func() {
		timer := time.NewTimer(notebookTypingDelay)
		defer timer.Stop()
		select {
		case <-m.ctx.Done():
			return
		case <-timer.C:
		}
		m.post(func() {
			if session.editSeq[cell] == seq {
				m.flushNotebookCell(session, cell)
			}
		})
	}()
}

func (m *Model) flushNotebookCell(session *notebookSession, cell string) {
	if session == nil || session.editBusy || session.snapshot.ID == "" {
		return
	}
	source, ok := session.pending[cell]
	if !ok {
		return
	}
	delete(session.pending, cell)
	session.editBusy = true
	version := session.snapshot.Version
	id := session.snapshot.ID
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "notebook_edit", map[string]any{
			"id": id, "version": version,
			"edit": openidea.NotebookEdit{Type: "setSource", Cell: cell, Source: source},
		})
		var snapshot openidea.NotebookSnapshot
		if err == nil {
			err = json.Unmarshal(raw, &snapshot)
		}
		m.post(func() {
			if m.notebooks.sessions[session.path] != session {
				return
			}
			session.editBusy = false
			if err != nil {
				if _, newer := session.pending[cell]; !newer {
					session.pending[cell] = source
				}
				session.error = "Could not update notebook cell: " + err.Error()
			} else {
				m.acceptNotebookSnapshot(session, snapshot)
				m.flushNextNotebookEdit(session)
				m.advanceNotebookSave(session)
			}
			m.invalidate()
		})
	}()
}

func (m *Model) flushNextNotebookEdit(session *notebookSession) {
	if session == nil || session.editBusy || session.running {
		return
	}
	for cell := range session.pending {
		m.flushNotebookCell(session, cell)
		return
	}
}

func (m *Model) editNotebook(session *notebookSession, edit openidea.NotebookEdit) {
	if session == nil || session.snapshot.ID == "" || session.busy || session.running {
		return
	}
	if edit.Type != "setSource" {
		session.docsLoaded = false
	}
	session.busy = true
	session.error = ""
	version := session.snapshot.Version
	id := session.snapshot.ID
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "notebook_edit", map[string]any{
			"id": id, "version": version, "edit": edit,
		})
		var snapshot openidea.NotebookSnapshot
		if err == nil {
			err = json.Unmarshal(raw, &snapshot)
		}
		m.post(func() {
			if m.notebooks.sessions[session.path] != session {
				return
			}
			session.busy = false
			if err != nil {
				session.error = "Could not edit notebook: " + err.Error()
			} else {
				m.acceptNotebookSnapshot(session, snapshot)
			}
			m.advanceNotebookSave(session)
			m.invalidate()
		})
	}()
}

func (m *Model) notebookHistory(session *notebookSession, redo bool) {
	if session == nil || session.snapshot.ID == "" || session.running || session.busy {
		return
	}
	session.docsLoaded = false
	session.busy = true
	session.error = ""
	id := session.snapshot.ID
	command := "notebook_undo"
	if redo {
		command = "notebook_redo"
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, command, map[string]any{"id": id})
		var snapshot openidea.NotebookSnapshot
		if err == nil {
			err = json.Unmarshal(raw, &snapshot)
		}
		m.post(func() {
			if m.notebooks.sessions[session.path] != session {
				return
			}
			session.busy = false
			if err != nil {
				session.error = "Notebook history failed: " + err.Error()
			} else {
				m.acceptNotebookSnapshot(session, snapshot)
			}
			m.advanceNotebookSave(session)
			m.invalidate()
		})
	}()
}

func (m *Model) activeNotebook() bool {
	tab := m.tab()
	return tab != nil && tab.kind == tabKindNotebook
}

func (m *Model) activeNotebookSession() *notebookSession {
	tab := m.tab()
	if tab == nil || tab.kind != tabKindNotebook {
		return nil
	}
	return m.notebook(tab.pagePath)
}

func (m *Model) activeNotebookCanUndo() bool {
	session := m.activeNotebookSession()
	return session != nil && session.loaded && !session.running && !session.busy &&
		session.snapshot.CanUndo
}

func (m *Model) activeNotebookCanRedo() bool {
	session := m.activeNotebookSession()
	return session != nil && session.loaded && !session.running && !session.busy &&
		session.snapshot.CanRedo
}

func (m *Model) undoNotebook() bool {
	session := m.activeNotebookSession()
	if session == nil {
		return false
	}
	if !m.activeNotebookCanUndo() {
		return true
	}
	m.notebookHistory(session, false)
	return true
}

func (m *Model) redoNotebook() bool {
	session := m.activeNotebookSession()
	if session == nil {
		return false
	}
	if !m.activeNotebookCanRedo() {
		return true
	}
	m.notebookHistory(session, true)
	return true
}

func (m *Model) saveNotebook(tab *Tab) {
	if tab == nil || tab.kind != tabKindNotebook {
		return
	}
	session := m.notebook(tab.pagePath)
	if session == nil || session.snapshot.ID == "" {
		return
	}
	session.closeAfter = false
	session.closeTab = nil
	m.advanceNotebookSave(session)
}

func (m *Model) advanceNotebookSave(session *notebookSession) {
	if session == nil || session.snapshot.ID == "" || session.busy || session.editBusy || session.running {
		return
	}
	if len(session.pending) > 0 {
		m.flushNextNotebookEdit(session)
		return
	}
	if !session.snapshot.Dirty {
		if session.closeAfter && session.closeTab != nil {
			tab := session.closeTab
			m.finishCloseNotebook(tab, session)
		}
		return
	}
	session.busy = true
	session.error = ""
	id := session.snapshot.ID
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "notebook_save", map[string]any{"id": id})
		var snapshot openidea.NotebookSnapshot
		if err == nil {
			err = json.Unmarshal(raw, &snapshot)
		}
		m.post(func() {
			if m.notebooks.sessions[session.path] != session {
				return
			}
			session.busy = false
			if err != nil {
				session.error = "Could not save notebook: " + err.Error()
				if session.closeAfter {
					session.closeAfter = false
					session.closeTab = nil
					session.closeAsk = true
				}
			} else {
				m.acceptNotebookSnapshot(session, snapshot)
				m.status = "Saved " + filepath.Base(session.path)
				m.advanceNotebookSave(session)
			}
			m.invalidate()
		})
	}()
}

func (m *Model) reloadNotebook(session *notebookSession) {
	if session == nil || session.snapshot.ID == "" || session.busy || session.running {
		return
	}
	session.docsLoaded = false
	session.busy = true
	session.error = ""
	id := session.snapshot.ID
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "notebook_reload", map[string]any{"id": id})
		var snapshot openidea.NotebookSnapshot
		if err == nil {
			err = json.Unmarshal(raw, &snapshot)
		}
		m.post(func() {
			if m.notebooks.sessions[session.path] != session {
				return
			}
			session.busy = false
			if err != nil {
				session.error = "Could not reload notebook: " + err.Error()
			} else {
				session.pending = make(map[string]string)
				session.streaming = make(map[string]string)
				m.acceptNotebookSnapshot(session, snapshot)
			}
			m.advanceNotebookSave(session)
			m.invalidate()
		})
	}()
}

func (m *Model) executeNotebook(session *notebookSession, cells []string) {
	if session == nil || session.snapshot.ID == "" || session.running {
		return
	}
	codeCells := make([]string, 0, len(cells))
	allowed := make(map[string]bool, len(cells))
	for _, id := range cells {
		allowed[id] = true
	}
	for _, cell := range session.snapshot.Cells {
		if cell.CellType == "code" && (len(allowed) == 0 || allowed[cell.ID]) {
			codeCells = append(codeCells, cell.ID)
		}
	}
	if len(codeCells) == 0 {
		return
	}
	for _, cell := range codeCells {
		delete(session.streaming, cell)
	}
	session.running = true
	session.busy = true
	session.error = ""
	session.runSeq++
	runSeq := session.runSeq
	session.channel = m.nextChannel()
	session.request = m.nextRequest()
	channel, request := session.channel, session.request
	id := session.snapshot.ID
	python := session.python
	if strings.TrimSpace(python) == "" {
		python = ""
	}
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 24*time.Hour)
		defer cancel()
		args := map[string]any{
			"id": id, "cells": codeCells, "request": request,
			"onOutput": map[string]any{"$channel": channel},
		}
		if python != "" {
			args["python"] = python
		} else {
			args["python"] = nil
		}
		raw, err := m.invoke(ctx, "notebook_execute", args)
		var snapshot openidea.NotebookSnapshot
		if err == nil {
			err = json.Unmarshal(raw, &snapshot)
		}
		m.post(func() {
			if m.notebooks.sessions[session.path] != session || session.runSeq != runSeq {
				return
			}
			session.running = false
			session.busy = false
			session.request = 0
			session.channel = 0
			if err != nil && !strings.Contains(strings.ToLower(err.Error()), "cancel") {
				session.error = "Notebook execution failed: " + err.Error()
			} else if err == nil {
				m.acceptNotebookSnapshot(session, snapshot)
			}
			m.advanceNotebookSave(session)
			m.invalidate()
		})
	}()
}

func (m *Model) interruptNotebook(session *notebookSession) {
	if session == nil || session.snapshot.ID == "" {
		return
	}
	id := session.snapshot.ID
	if session.request != 0 {
		m.cancelRequest(session.request)
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
		defer cancel()
		_, err := m.invoke(ctx, "notebook_interrupt", map[string]any{"id": id})
		m.post(func() {
			if err != nil {
				session.error = "Could not interrupt the notebook kernel: " + err.Error()
			} else {
				session.kernel = "idle"
			}
			m.invalidate()
		})
	}()
}

func (m *Model) restartNotebook(session *notebookSession) {
	if session == nil || session.snapshot.ID == "" {
		return
	}
	session.docsLoaded = false
	if session.request != 0 {
		m.cancelRequest(session.request)
	}
	session.busy = true
	session.error = ""
	id := session.snapshot.ID
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "notebook_restart", map[string]any{"id": id})
		var snapshot openidea.NotebookSnapshot
		if err == nil {
			err = json.Unmarshal(raw, &snapshot)
		}
		m.post(func() {
			if m.notebooks.sessions[session.path] != session {
				return
			}
			session.busy = false
			session.running = false
			if err != nil {
				session.error = "Could not restart notebook kernel: " + err.Error()
			} else {
				m.acceptNotebookSnapshot(session, snapshot)
			}
			m.advanceNotebookSave(session)
			m.invalidate()
		})
	}()
}

func (m *Model) detectNotebookInterpreters(session *notebookSession) {
	if session == nil || session.running {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "sdk_detect", map[string]any{"kind": "python"})
		var envs []openidea.Sdk
		if err == nil {
			err = json.Unmarshal(raw, &envs)
		}
		m.post(func() {
			if m.notebooks.sessions[session.path] != session {
				return
			}
			if err != nil {
				session.error = "Could not refresh Python interpreters: " + err.Error()
			} else {
				session.envs = envs
				if session.python == "" && len(envs) > 0 {
					session.python = envs[0].Path
				}
			}
			m.invalidate()
		})
	}()
}

func (m *Model) acceptNotebookChunk(channel uint32, data json.RawMessage) bool {
	for _, session := range m.notebooks.sessions {
		if session == nil || session.channel != channel {
			continue
		}
		var event openidea.NotebookOutputEvent
		if json.Unmarshal(data, &event) != nil || event.ID != session.snapshot.ID {
			return true
		}
		if text := notebookKernelOutputText(event.Content); text != "" {
			session.streaming[event.Cell] += text
			if len(session.streaming[event.Cell]) > notebookPreviewLimit {
				session.streaming[event.Cell] = session.streaming[event.Cell][len(session.streaming[event.Cell])-notebookPreviewLimit:]
			}
		}
		m.invalidate()
		return true
	}
	return false
}

func (m *Model) acceptNotebookKernelEvent(event openidea.NotebookKernelEvent) {
	for _, session := range m.notebooks.sessions {
		if session == nil || event.ID != "" && session.snapshot.ID != event.ID ||
			event.Path != "" && !samePath(event.Path, session.path) {
			continue
		}
		if event.State != "" {
			session.kernel = event.State
		}
		if event.Message != "" {
			session.error = event.Message
		}
		m.invalidate()
		return
	}
}

func (m *Model) requestCloseNotebook(tab *Tab) {
	if tab == nil || tab.kind != tabKindNotebook {
		return
	}
	session := m.notebook(tab.pagePath)
	if session == nil {
		m.removeTabNow(tab)
		return
	}
	others := 0
	for _, candidate := range m.allTabs() {
		if candidate != tab && candidate.kind == tabKindNotebook &&
			samePath(candidate.pagePath, session.path) {
			others++
		}
	}
	if others > 0 || !tab.pageDirty && !session.running && !session.busy &&
		!session.snapshot.Dirty && len(session.pending) == 0 {
		m.finishCloseNotebook(tab, session)
		return
	}
	if session.closeAfter && session.closeTab != nil && session.closeTab != tab {
		return
	}
	session.closeAsk = true
	m.invalidate()
}

func (m *Model) confirmCloseNotebook(save bool) {
	tab := m.tab()
	if tab == nil || tab.kind != tabKindNotebook {
		return
	}
	session := m.notebook(tab.pagePath)
	if session == nil {
		m.removeTabNow(tab)
		return
	}
	session.closeAsk = false
	if save {
		session.closeAfter = true
		session.closeTab = tab
		m.advanceNotebookSave(session)
		return
	}
	session.closeAfter = false
	session.closeTab = nil
	m.finishCloseNotebook(tab, session)
}

func (m *Model) finishCloseNotebook(tab *Tab, session *notebookSession) {
	if tab == nil {
		return
	}
	hasAnotherView := false
	for _, candidate := range m.allTabs() {
		if candidate != tab && candidate.kind == tabKindNotebook &&
			samePath(candidate.pagePath, session.path) {
			hasAnotherView = true
			break
		}
	}
	m.removeTabNow(tab)
	if hasAnotherView || session == nil {
		return
	}
	session.closeAfter = false
	session.closeTab = nil
	delete(m.notebooks.sessions, session.path)
	if session.request != 0 {
		m.cancelRequest(session.request)
	}
	if session.snapshot.ID == "" || m.mgr == nil && m.invokeHook == nil {
		return
	}
	id := session.snapshot.ID
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
		defer cancel()
		_, _ = m.invoke(ctx, "notebook_close", map[string]any{"id": id})
	}()
}

func (m *Model) closeAllNotebooks() {
	for _, session := range m.notebooks.sessions {
		if session == nil {
			continue
		}
		if session.request != 0 {
			m.cancelRequest(session.request)
		}
		if session.snapshot.ID == "" || m.mgr == nil && m.invokeHook == nil {
			continue
		}
		id := session.snapshot.ID
		go func() {
			ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
			defer cancel()
			_, _ = m.invoke(ctx, "notebook_close", map[string]any{"id": id})
		}()
	}
	m.notebooks = notebookState{}
}

func (m *Model) notebookPage(c *ui.Context, p *ui.Theme, tab *Tab) {
	session := m.notebook(tab.pagePath)
	area := ui.Column(c.Key("notebook:" + tab.pagePath)).Grow(1).MinHeight(0).
		MinWidth(0).Background(p.Background)
	area.OnShortcut(ui.Cmd, ui.KeyS, func() { m.saveNotebook(tab) })
	area.OnShortcut(ui.Cmd|ui.Shift, ui.KeyZ, func() { m.notebookHistory(session, true) })
	area.Children(func() {
		m.notebookToolbar(c, p, session)
		if session.error != "" {
			ui.Row(c.Key("notebook:error:"+session.path)).Shrink(0).Padding(6, 10).
				Gap(8).AlignItems(ui.Center).Background(p.Surface).
				BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
				ui.Text(c, session.error).FontSize(12).TextColor(p.Warning).
					Grow(1).SingleLine().Ellipsis("…")
				if !session.loaded && !session.loading {
					ui.Button(c, "Retry").OnClick(func() { m.loadNotebook(session) })
				}
				ui.Button(c, "Close").OnClick(func() {
					session.error = ""
					m.invalidate()
				})
			})
		}
		switch {
		case session.loading && !session.loaded:
			ui.Row(c).Grow(1).Justify(ui.Center).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Loading notebook…").FontSize(13).TextColor(p.TextMuted)
			})
		case !session.loaded:
			ui.Row(c).Grow(1).Justify(ui.Center).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Notebook unavailable").FontSize(13).TextColor(p.TextMuted)
			})
		default:
			m.notebookCells(c, p, session)
		}
	})
	if session != nil && session.closeAsk {
		open := true
		nativeModal(c.Key("notebook:close-confirm:"+session.path), &open, func() {
			ui.Text(c, "Save changes to this notebook before closing?").
				FontSize(13).TextColor(p.Text).Padding(8, 0)
			ui.Row(c).Gap(8).Justify(ui.End).Padding(8, 0).Children(func() {
				ui.Button(c, "Cancel").OnClick(func() {
					session.closeAsk = false
					m.invalidate()
				})
				ui.Button(c, "Discard").OnClick(func() {
					session.pending = make(map[string]string)
					m.confirmCloseNotebook(false)
				})
				ui.Button(c, "Save and Close").OnClick(func() { m.confirmCloseNotebook(true) })
			})
		})
		if !open && session.closeAsk {
			session.closeAsk = false
		}
	}
}

func (m *Model) notebookToolbar(c *ui.Context, p *ui.Theme, session *notebookSession) {
	ui.Row(c.Key("notebook:toolbar:"+session.path)).Shrink(0).Height(38).
		Padding(0, 8).Gap(6).AlignItems(ui.Center).Background(p.Surface).
		BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
		if session == nil {
			return
		}
		m.notebookToolbarButton(c, p, "Run Cell", session.loaded && !session.running &&
			notebookSelectedIsCode(session), func() {
			if session.selected != "" {
				m.executeNotebook(session, []string{session.selected})
			}
		})
		m.notebookToolbarButton(c, p, "Run All", session.loaded && !session.running &&
			notebookHasCodeCells(session), func() { m.executeNotebook(session, nil) })
		m.notebookToolbarButton(c, p, "Stop", session.running, func() { m.interruptNotebook(session) })
		m.notebookToolbarButton(c, p, "Restart", session.loaded && !session.busy, func() {
			m.restartNotebook(session)
		})
		ui.Spacer(c)
		label := "Select Python interpreter"
		if session.python != "" {
			label = filepath.Base(session.python)
			for _, env := range session.envs {
				if samePath(env.Path, session.python) && env.Name != "" {
					label = env.Name
					break
				}
			}
		}
		menu := ui.MenuButton(c.Key("notebook:python:"+session.path), label, func(menu *ui.Menu) {
			for _, env := range session.envs {
				env := env
				item := menu.Item(env.Name + "  " + env.Path)
				item.Checked(samePath(env.Path, session.python))
				if item.Chosen() {
					session.python = env.Path
					session.error = ""
					m.invalidate()
				}
			}
			if len(session.envs) == 0 {
				menu.Item("No Python environments found").Disabled(true)
			}
		}).Height(26).MaxWidth(320)
		menu.Disabled(session.running || !session.loaded)
		m.notebookToolbarButton(c, p, "Refresh", session.loaded && !session.running, func() {
			m.detectNotebookInterpreters(session)
		})
		ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
			label := session.kernel
			if label == "" {
				label = "idle"
			}
			ui.Text(c, label).FontSize(11).TextColor(p.TextMuted).SingleLine()
		})
		ui.Spacer(c)
		m.notebookToolbarButton(c, p, "Undo", session.loaded && !session.running &&
			!session.busy && session.snapshot.CanUndo, func() { m.notebookHistory(session, false) })
		m.notebookToolbarButton(c, p, "Redo", session.loaded && !session.running &&
			!session.busy && session.snapshot.CanRedo, func() { m.notebookHistory(session, true) })
		m.notebookToolbarButton(c, p, "Save", session.loaded && !session.busy, func() {
			tab := m.tab()
			m.saveNotebook(tab)
		})
		m.notebookToolbarButton(c, p, "Reload", session.loaded && !session.running && !session.busy, func() {
			m.reloadNotebook(session)
		})
	})
}

func (m *Model) notebookToolbarButton(c *ui.Context, _ *ui.Theme, label string, enabled bool, click func()) {
	b := ui.Button(c, label).Height(26).Disabled(!enabled)
	b.OnClick(click)
}

func (m *Model) notebookCells(c *ui.Context, p *ui.Theme, session *notebookSession) {
	ui.Scroll(c.Key("notebook:scroll:" + session.path)).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Padding(18, 24).Gap(12).Children(func() {
			if len(session.snapshot.Cells) == 0 {
				ui.Column(c).Padding(28).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "This notebook has no cells").FontSize(14).TextColor(p.TextMuted)
					ui.Row(c).Gap(8).Children(func() {
						ui.Button(c, "Add Code Cell").OnClick(func() {
							m.editNotebook(session, openidea.NotebookEdit{Type: "insertCell", CellType: "code"})
						})
						ui.Button(c, "Add Markdown Cell").OnClick(func() {
							m.editNotebook(session, openidea.NotebookEdit{Type: "insertCell", CellType: "markdown"})
						})
					})
				})
			}
			for index := range session.snapshot.Cells {
				m.notebookCell(c, p, session, index)
			}
			ui.Row(c).Gap(8).Padding(2, 0, 12, 0).Children(func() {
				last := (*string)(nil)
				if len(session.snapshot.Cells) > 0 {
					id := session.snapshot.Cells[len(session.snapshot.Cells)-1].ID
					last = &id
				}
				ui.Button(c, "Add Code Cell").Disabled(!session.loaded || session.running || session.busy).
					OnClick(func() {
						m.editNotebook(session, openidea.NotebookEdit{
							Type: "insertCell", After: last, CellType: "code",
						})
					})
				ui.Button(c, "Add Markdown Cell").Disabled(!session.loaded || session.running || session.busy).
					OnClick(func() {
						m.editNotebook(session, openidea.NotebookEdit{
							Type: "insertCell", After: last, CellType: "markdown",
						})
					})
			})
		})
	})
}

func (m *Model) notebookCell(c *ui.Context, p *ui.Theme, session *notebookSession, index int) {
	cell := &session.snapshot.Cells[index]
	selected := session.selected == cell.ID
	card := ui.Column(c.Key("notebook:cell:"+cell.ID)).Padding(8).Gap(6).
		Radius(float32(notebookInternalRadius)).Background(p.Surface).
		BorderWidth(1, 1, 1, 1).BorderColor(p.Border)
	if selected {
		card.BorderColor(p.Accent)
	}
	card.Children(func() {
		ui.Row(c).Height(26).Gap(5).AlignItems(ui.Center).Children(func() {
			count := " "
			if cell.CellType == "code" {
				count = fmt.Sprintf("[%s]", notebookExecutionCount(cell.ExecutionCount))
			} else {
				count = fmt.Sprintf("%d", index+1)
			}
			ui.Text(c, count).FontSize(11).TextColor(p.TextMuted).Width(38).SingleLine()
			cellType := ui.MenuButton(c.Key("notebook:type:"+cell.ID), cellTypeLabel(cell.CellType),
				func(menu *ui.Menu) {
					for _, kind := range []string{"code", "markdown", "raw"} {
						kind := kind
						item := menu.Item(cellTypeLabel(kind))
						item.Checked(cell.CellType == kind)
						if item.Chosen() && cell.CellType != kind {
							m.editNotebook(session, openidea.NotebookEdit{
								Type: "setCellType", Cell: cell.ID, CellType: kind,
							})
						}
					}
				}).Height(24).MaxWidth(130)
			cellType.Disabled(session.running || session.busy)
			if cell.CellType == "code" {
				run := ui.Button(c, "Run").Height(24).
					Disabled(!session.loaded || session.running)
				run.OnClick(func() { m.executeNotebook(session, []string{cell.ID}) })
			}
			if cell.CellType == "markdown" {
				editing := session.markdown[cell.ID]
				label := "Edit"
				if editing {
					label = "Preview"
				}
				ui.Button(c, label).Height(24).Disabled(session.running).OnClick(func() {
					session.markdown[cell.ID] = !editing
					m.invalidate()
				})
			}
			ui.Spacer(c)
			up := ui.Button(c, "Up").Height(24).Disabled(session.running || session.busy || index == 0)
			up.OnClick(func() {
				target := session.snapshot.Cells[index-1].ID
				m.editNotebook(session, openidea.NotebookEdit{
					Type: "moveCell", Cell: cell.ID, Before: &target,
				})
			})
			down := ui.Button(c, "Down").Height(24).
				Disabled(session.running || session.busy || index == len(session.snapshot.Cells)-1)
			down.OnClick(func() {
				var before *string
				if index+2 < len(session.snapshot.Cells) {
					id := session.snapshot.Cells[index+2].ID
					before = &id
				}
				m.editNotebook(session, openidea.NotebookEdit{
					Type: "moveCell", Cell: cell.ID, Before: before,
				})
			})
			insert := ui.Button(c, "+").Height(24).Width(28).
				Disabled(session.running || session.busy)
			insert.OnClick(func() {
				id := cell.ID
				m.editNotebook(session, openidea.NotebookEdit{
					Type: "insertCell", After: &id, CellType: "code",
				})
			})
			remove := ui.Button(c, "Delete").Height(24).
				Disabled(session.running || session.busy)
			remove.OnClick(func() {
				m.editNotebook(session, openidea.NotebookEdit{Type: "deleteCell", Cell: cell.ID})
			})
		})
		cellCard := ui.Column(c).Gap(4)
		cellCard.OnClick(func() {
			session.selected = cell.ID
			m.invalidate()
		})
		cellCard.Children(func() {
			if cell.CellType == "markdown" && !session.markdown[cell.ID] {
				m.notebookMarkdown(c, p, cell.Source)
			} else {
				view := m.notebookCellEditorView(session, cell)
				area := editor.View(c, view.state).MinHeight(72).MaxHeight(360)
				area.OnClick(func() {
					session.selected = cell.ID
					m.invalidate()
				})
				if cell.CellType == "code" {
					area.OnShortcut(ui.Shift, ui.KeyEnter, func() {
						m.executeNotebook(session, []string{cell.ID})
					})
				}
			}
			if cell.CellType == "code" {
				if streaming := session.streaming[cell.ID]; streaming != "" {
					m.notebookOutputText(c, p, streaming, false)
				}
				for outputIndex, output := range cell.Outputs {
					m.notebookOutput(c, p, session, cell.ID, outputIndex, output)
				}
			}
		})
	})
}

func (m *Model) notebookCellEditorView(session *notebookSession, cell *openidea.NotebookCell) *notebookCellEditor {
	view := session.editors[cell.ID]
	if view == nil {
		buffer := editor.NewBuffer(cell.Source)
		switch cell.CellType {
		case "code":
			buffer.SetLanguage(editor.LanguageFor("cell.py"))
		case "markdown":
			buffer.SetLanguage(editor.LanguageFor("cell.md"))
		}
		size := m.prefs.fontSize()
		font := m.prefs.Font
		view = &notebookCellEditor{
			state: &editor.State{
				Buffer:         buffer,
				Font:           ui.Font{Family: font.fontFamily(), Size: size, Weight: font.weight(), Features: font.features()},
				LineHeight:     font.lineHeightPixels(size) / size,
				TabSize:        4,
				SoftWrap:       true,
				LineNumbers:    editor.LineNumbersOn,
				NoIndentGuides: true,
				FitContent:     true,
				Bindings:       m.editorBindings(),
				Label:          "Notebook cell source",
			},
			seen: buffer.Version(),
		}
		session.editors[cell.ID] = view
	}
	view.state.ReadOnly = session.running
	view.state.Label = "Notebook cell source"
	doc := session.cellDocs[cell.ID]
	cellID := cell.ID
	view.state.OnCompletion = nil
	view.state.OnHover = nil
	view.state.OnDefinition = nil
	if cell.CellType == "code" && doc.Doc != 0 {
		view.state.OnCompletion = func(req editor.CompletionRequest) {
			m.requestNotebookCompletion(session, view, doc, req)
		}
		view.state.OnHover = func(req editor.HoverRequest) {
			m.requestNotebookHover(session, view, doc, req)
		}
		view.state.OnDefinition = func(offset int) {
			m.notebookDefinition(session, view, doc, offset)
		}
	}
	if version := view.state.Buffer.Version(); version != view.seen {
		view.seen = version
		if !session.running {
			cell.Source = view.state.Buffer.String()
			session.selected = cellID
			m.notebookCellChanged(session, cellID, cell.Source)
		}
	}
	return view
}

func (m *Model) requestNotebookCompletion(session *notebookSession, view *notebookCellEditor, doc openidea.NotebookCellDoc, req editor.CompletionRequest) {
	if session == nil || view == nil || doc.Doc == 0 || session.running {
		return
	}
	source := view.state.Buffer.String()
	start := max(0, req.Offset-len(req.Prefix))
	var trigger any
	if req.Offset > 0 {
		if last := source[req.Offset-1 : req.Offset]; strings.ContainsAny(last, ".:>") {
			trigger = last
		}
	}
	if view.comp != 0 {
		m.cancelRequest(view.comp)
	}
	id := m.nextRequest()
	view.comp = id
	view.compGen++
	gen := view.compGen
	pos := notebookUTF16Position(source, req.Offset)
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, lspTimeout)
		defer cancel()
		raw, err := m.invoke(ctx, "lsp_completion", map[string]any{
			"request": id, "doc": doc.Doc, "pos": lspPos(pos), "trigger": trigger,
		})
		var list openidea.CompletionList
		if err == nil {
			err = json.Unmarshal(raw, &list)
		}
		items := lspCompletions(list.Items)
		m.post(func() {
			if view.comp == id {
				view.comp = 0
			}
			if err != nil || view.compGen != gen || m.notebooks.sessions[session.path] != session ||
				session.cellDocs[doc.Cell].Doc != doc.Doc || len(items) == 0 {
				return
			}
			view.state.ShowCompletions(start, items)
			m.invalidate()
		})
	}()
}

func (m *Model) requestNotebookHover(session *notebookSession, view *notebookCellEditor, doc openidea.NotebookCellDoc, req editor.HoverRequest) {
	if session == nil || view == nil || doc.Doc == 0 || session.running {
		return
	}
	source := view.state.Buffer.String()
	pos := notebookUTF16Position(source, req.Offset)
	view.hoverGen++
	gen := view.hoverGen
	request := m.nextRequest()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, lspTimeout)
		defer cancel()
		raw, err := m.invoke(ctx, "lsp_hover", map[string]any{
			"request": request, "doc": doc.Doc, "pos": lspPos(pos),
		})
		if err != nil {
			return
		}
		var hover *openidea.Hover
		if json.Unmarshal(raw, &hover) != nil || hover == nil {
			return
		}
		text := hoverText(hover.Contents)
		if text == "" {
			return
		}
		m.post(func() {
			if view.hoverGen != gen || m.notebooks.sessions[session.path] != session ||
				session.cellDocs[doc.Cell].Doc != doc.Doc ||
				view.state.Buffer.Version() != req.Version || view.state.Cursor() != req.Offset {
				return
			}
			start, end := req.Offset, req.Offset
			if hover.Range != nil {
				start = notebookByteOffset(view.state.Buffer.String(), hover.Range.Start)
				end = notebookByteOffset(view.state.Buffer.String(), hover.Range.End)
			}
			view.state.ShowHover(start, end, text, nil)
			m.invalidate()
		})
	}()
}

func (m *Model) notebookDefinition(session *notebookSession, view *notebookCellEditor, doc openidea.NotebookCellDoc, offset int) {
	if session == nil || view == nil || doc.Doc == 0 || session.running {
		return
	}
	pos := notebookUTF16Position(view.state.Buffer.String(), offset)
	view.defGen++
	gen := view.defGen
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, lspTimeout)
		defer cancel()
		raw, err := m.invoke(ctx, "lsp_goto", map[string]any{
			"doc": doc.Doc, "pos": lspPos(pos), "kind": "definition",
		})
		var locations []openidea.Location
		if err == nil {
			err = json.Unmarshal(raw, &locations)
		}
		m.post(func() {
			if view.defGen != gen || m.notebooks.sessions[session.path] != session {
				return
			}
			switch {
			case err != nil:
				m.status = "Navigation failed: " + err.Error()
			case len(locations) == 0:
				m.status = "No definition found"
			default:
				location := locations[0]
				if targetSession, cell, ok := m.notebookCellLocation(location.Path); ok {
					m.selectNotebookCell(targetSession, cell, location.Range.Start)
					return
				}
				if strings.Contains(location.Path, "://") {
					m.status = "The definition is in an external dependency and cannot open as a file"
				} else {
					m.revealAt(location.Path, int(location.Range.Start.Line), int(location.Range.Start.Col))
					return
				}
			}
			m.invalidate()
		})
	}()
}

func (m *Model) notebookCellLocation(uri string) (*notebookSession, string, bool) {
	for _, session := range m.notebooks.sessions {
		for cell, doc := range session.cellDocs {
			if doc.URI == uri {
				return session, cell, true
			}
		}
	}
	return nil, "", false
}

func (m *Model) selectNotebookCell(session *notebookSession, cell string, pos openidea.OutlinePos) {
	if session == nil {
		return
	}
	session.selected = cell
	if view := session.editors[cell]; view != nil {
		offset := notebookByteOffset(view.state.Buffer.String(), pos)
		view.state.SetSelections(editor.Selection{Anchor: offset, Head: offset})
	}
	for i, tab := range m.tabs {
		if tab != nil && tab.kind == tabKindNotebook && samePath(tab.pagePath, session.path) {
			m.active = i
			break
		}
	}
	m.revealEditor()
	m.invalidate()
}

func notebookUTF16Position(source string, offset int) openidea.OutlinePos {
	offset = max(0, min(offset, len(source)))
	lineStart := strings.LastIndex(source[:offset], "\n") + 1
	line := strings.Count(source[:lineStart], "\n")
	col := len(utf16.Encode([]rune(source[lineStart:offset])))
	return openidea.OutlinePos{Line: uint32(line), Col: uint32(col)}
}

func notebookByteOffset(source string, pos openidea.OutlinePos) int {
	line := int(pos.Line)
	target := int(pos.Col)
	lineStart := 0
	for n := 0; n < line; n++ {
		next := strings.IndexByte(source[lineStart:], '\n')
		if next < 0 {
			return len(source)
		}
		lineStart += next + 1
	}
	lineEnd := len(source)
	if next := strings.IndexByte(source[lineStart:], '\n'); next >= 0 {
		lineEnd = lineStart + next
	}
	units := 0
	for at, r := range source[lineStart:lineEnd] {
		width := 1
		if r > 0xffff {
			width = 2
		}
		if units+width > target {
			return lineStart + at
		}
		units += width
		if units == target {
			return lineStart + at + len(string(r))
		}
	}
	return lineEnd
}

func (m *Model) notebookMarkdown(c *ui.Context, p *ui.Theme, source string) {
	if strings.TrimSpace(source) == "" {
		ui.Text(c, "Double-click Edit to write this Markdown cell.").
			FontSize(12).TextColor(p.TextMuted).Padding(8)
		return
	}
	col := ui.Column(c).Padding(8).Gap(5)
	col.Children(func() {
		for _, raw := range strings.Split(source, "\n") {
			line := strings.TrimSpace(raw)
			switch {
			case strings.HasPrefix(line, "### "):
				ui.Text(c, strings.TrimPrefix(line, "### ")).FontSize(14).Bold().TextColor(p.Text)
			case strings.HasPrefix(line, "## "):
				ui.Text(c, strings.TrimPrefix(line, "## ")).FontSize(16).Bold().TextColor(p.Text)
			case strings.HasPrefix(line, "# "):
				ui.Text(c, strings.TrimPrefix(line, "# ")).FontSize(19).Bold().TextColor(p.Text)
			case strings.HasPrefix(line, "- "), strings.HasPrefix(line, "* "):
				ui.Text(c, "• "+line[2:]).FontSize(13).TextColor(p.Text)
			case strings.HasPrefix(line, "```"):
				ui.Text(c, strings.TrimPrefix(line, "```")).Font("monospace").
					FontSize(12).TextColor(p.TextMuted)
			case line == "":
				ui.Spacer(c).Height(5)
			default:
				ui.Text(c, line).FontSize(13).TextColor(p.Text)
			}
		}
	})
}

func (m *Model) notebookOutput(c *ui.Context, p *ui.Theme, session *notebookSession, cellID string, index int, raw json.RawMessage) {
	if bitmap := m.notebookOutputBitmap(session, raw); bitmap != nil {
		ui.Image(c, bitmap).Width(560).MaxWidthPercent(100).Height(320).
			Fit(ui.Contain)
		return
	}
	text, isError := notebookPersistedOutput(raw)
	if strings.TrimSpace(text) == "" {
		var value map[string]any
		if json.Unmarshal(raw, &value) == nil {
			kind, _ := value["output_type"].(string)
			if kind != "" {
				ui.Text(c, "Unsupported output: "+kind).FontSize(11).
					TextColor(p.TextMuted).Padding(6)
			}
		}
		return
	}
	m.notebookOutputText(c, p, text, isError)
	_ = cellID
	_ = index
}

func (m *Model) notebookOutputText(c *ui.Context, p *ui.Theme, text string, isError bool) {
	if len(text) > notebookPreviewLimit {
		text = text[:notebookPreviewLimit] + "\nOutput preview truncated; full output is retained in the notebook."
	}
	color := p.Text
	if isError {
		color = p.Danger
	}
	ui.Row(c).Padding(8).Radius(4).Background(p.Background).Children(func() {
		ui.Text(c, text).Font("monospace").FontSize(12).TextColor(color)
	})
}

func (m *Model) notebookOutputBitmap(session *notebookSession, raw json.RawMessage) *ui.Bitmap {
	var output struct {
		Data map[string]any `json:"data"`
	}
	if json.Unmarshal(raw, &output) != nil || len(output.Data) == 0 {
		return nil
	}
	mime, encoded := "", ""
	for _, candidate := range []string{"image/png", "image/jpeg", "image/gif", "image/webp"} {
		if value, ok := output.Data[candidate]; ok {
			mime, encoded = candidate, notebookTextValue(value, false)
			break
		}
	}
	if mime == "" || encoded == "" {
		return nil
	}
	key := mime + ":" + encoded
	if bitmap := session.outputCache[key]; bitmap != nil {
		return bitmap
	}
	data, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(encoded), ""))
	if err != nil {
		return nil
	}
	bitmap, err := ui.DecodeBitmap(data)
	if err != nil {
		return nil
	}
	if len(session.outputCache) >= 64 {
		session.outputCache = make(map[string]*ui.Bitmap)
	}
	session.outputCache[key] = bitmap
	return bitmap
}

func notebookPersistedOutput(raw json.RawMessage) (string, bool) {
	var output map[string]any
	if json.Unmarshal(raw, &output) != nil {
		return "", false
	}
	kind, _ := output["output_type"].(string)
	switch kind {
	case "stream":
		return notebookTextValue(output["text"], false), output["name"] == "stderr"
	case "error":
		if traceback, ok := output["traceback"].([]any); ok && len(traceback) > 0 {
			parts := make([]string, 0, len(traceback))
			for _, value := range traceback {
				parts = append(parts, notebookTextValue(value, false))
			}
			return strings.Join(parts, "\n"), true
		}
		return notebookTextValue(output["ename"], false) + ": " +
			notebookTextValue(output["evalue"], false), true
	case "display_data", "execute_result":
		data, _ := output["data"].(map[string]any)
		for _, mime := range []string{"text/plain", "text/html", "application/json"} {
			if value, ok := data[mime]; ok {
				return notebookTextValue(value, true), false
			}
		}
	}
	return "", false
}

func notebookKernelOutputText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return ""
	}
	object, _ := value.(map[string]any)
	if object == nil {
		return notebookTextValue(value, true)
	}
	if text, ok := object["text"]; ok {
		return notebookTextValue(text, false)
	}
	if traceback, ok := object["traceback"].([]any); ok {
		parts := make([]string, 0, len(traceback))
		for _, item := range traceback {
			parts = append(parts, notebookTextValue(item, false))
		}
		return strings.Join(parts, "\n")
	}
	if name, ok := object["ename"]; ok {
		return notebookTextValue(name, false) + ": " + notebookTextValue(object["evalue"], false)
	}
	if data, ok := object["data"].(map[string]any); ok {
		for _, mime := range []string{"text/plain", "text/html", "application/json"} {
			if item, found := data[mime]; found {
				return notebookTextValue(item, true)
			}
		}
	}
	return ""
}

func notebookTextValue(value any, jsonScalars bool) string {
	switch item := value.(type) {
	case nil:
		return ""
	case string:
		return item
	case []any:
		parts := make([]string, 0, len(item))
		for _, entry := range item {
			parts = append(parts, notebookTextValue(entry, jsonScalars))
		}
		return strings.Join(parts, "")
	case []string:
		return strings.Join(item, "")
	default:
		if jsonScalars {
			data, _ := json.Marshal(item)
			return string(data)
		}
		return fmt.Sprint(item)
	}
}

func notebookExecutionCount(count *uint32) string {
	if count == nil {
		return " "
	}
	return fmt.Sprint(*count)
}

func notebookSelectedIsCode(session *notebookSession) bool {
	if session == nil {
		return false
	}
	for _, cell := range session.snapshot.Cells {
		if cell.ID == session.selected {
			return cell.CellType == "code"
		}
	}
	return false
}

func notebookHasCodeCells(session *notebookSession) bool {
	if session == nil {
		return false
	}
	for _, cell := range session.snapshot.Cells {
		if cell.CellType == "code" {
			return true
		}
	}
	return false
}

func cellTypeLabel(kind string) string {
	switch kind {
	case "code":
		return "Code"
	case "markdown":
		return "Markdown"
	case "raw":
		return "Raw"
	default:
		if kind == "" {
			return "Unknown"
		}
		return kind
	}
}
