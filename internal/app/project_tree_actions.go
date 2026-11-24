package app

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo"
)

type treeMoveRecord struct {
	ID          uint32
	Kind        string
	Seq         int64
	From        string
	To          string
	Moves       []treeFileMove
	DeletePaths []string
	DeleteDirs  []string
	OpenPaths   []string
	Active      string
}

var nextTreeFileOperation int64

func treeFileOperationSequence() int64 {
	nextTreeFileOperation++
	return nextTreeFileOperation
}

func (r *treeMoveRecord) moves() []treeFileMove {
	if r == nil {
		return nil
	}
	if len(r.Moves) > 0 {
		return append([]treeFileMove(nil), r.Moves...)
	}
	if r.From != "" && r.To != "" {
		return []treeFileMove{{From: r.From, To: r.To}}
	}
	return nil
}

type treeFileMove struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type treeFileCopy struct {
	From string
	To   string
}

type treeCopyOutcome struct {
	Copied   []treeFileCopy
	Failures []string
}

// topLevelTreePaths removes duplicate and nested project-view entries while
// preserving row order, as OpenIDEA's selection helpers do before a file
// operation.
func (m *Model) topLevelTreePaths(paths []string) []string {
	clean := make([]string, 0, len(paths))
	seen := map[string]bool{}
	for _, path := range paths {
		path = m.projectTreePathClean(path)
		if path == "" || path == "." {
			continue
		}
		key := path
		if m.projectTreeCaseInsensitive() {
			key = strings.ToLower(path)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		clean = append(clean, path)
	}
	out := make([]string, 0, len(clean))
	for index, path := range clean {
		nested := false
		for otherIndex, other := range clean {
			if index == otherIndex || other == path {
				continue
			}
			if m.projectTreeUnderPath(path, other) {
				nested = true
				break
			}
		}
		if !nested {
			out = append(out, path)
		}
	}
	return out
}

func (m *Model) mutableTreePaths(paths []string) []string {
	paths = m.topLevelTreePaths(paths)
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		if path == "" || path == m.Root() || m.workspaceContains(path) {
			continue
		}
		if m.tree != nil && m.tree.ReadOnly(path) {
			continue
		}
		if !m.projectTreeEntryKnown(path) {
			continue
		}
		out = append(out, path)
	}
	return out
}

func (m *Model) selectedTreePaths() []string {
	if m.tree == nil {
		return nil
	}
	return m.topLevelTreePaths(m.tree.SelectedPaths())
}

func (m *Model) selectedMutableTreePaths() []string {
	return m.mutableTreePaths(m.selectedTreePaths())
}

// writeTreeClipboardFiles hands a project-view copy to macOS as file URLs so
// Finder and other applications can paste it. The in-app clipboard remains the
// source of truth when MyGo is not connected, which also keeps offscreen unit
// models from invoking the desktop clipboard subsystem.
func (m *Model) writeTreeClipboardFiles(paths []string) {
	if len(paths) == 0 || (m.ctx == nil && m.mgr == nil && m.invokeHook == nil) {
		return
	}
	defer func() { _ = recover() }()
	_ = mygo.Clipboard.WriteFiles(paths...)
}

func (m *Model) readTreeClipboardFiles() (files []string, err error) {
	if m.ctx == nil && m.mgr == nil && m.invokeHook == nil {
		return nil, nil
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			files, err = nil, fmt.Errorf("cannot access the clipboard")
		}
	}()
	return mygo.Clipboard.ReadFiles()
}

func (m *Model) sameTreeClipboardFiles(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	key := func(files []string) []string {
		keys := make([]string, 0, len(files))
		for _, path := range files {
			path = m.projectTreeNameKey(m.projectTreePathClean(path))
			keys = append(keys, path)
		}
		sort.Strings(keys)
		return keys
	}
	leftKeys, rightKeys := key(left), key(right)
	for index := range leftKeys {
		if leftKeys[index] != rightKeys[index] {
			return false
		}
	}
	return true
}

func (m *Model) treeActionDirectory() string {
	if target, ok := m.selectedTreeEntry(); ok {
		return m.treeTargetFolder(target)
	}
	if tab := m.tab(); tab != nil && tab.kind == "" && tab.doc != nil {
		return m.projectTreePathDir(tab.doc.Path())
	}
	return m.Root()
}

// NewFileAtProjectSelection starts a New File operation beside the selected
// project-view entry, or in the active editor's folder.
func (m *Model) NewFileAtProjectSelection() {
	if dir := m.treeActionDirectory(); dir != "" {
		m.OpenProjectNew(dir)
	}
}

// NewDirectoryAtProjectSelection starts a New Directory operation.
func (m *Model) NewDirectoryAtProjectSelection() {
	if dir := m.treeActionDirectory(); dir != "" {
		m.OpenNewTreeEntry(dir, true)
	}
}

// NewInThisDirectory opens New for the current file's directory.
func (m *Model) NewInThisDirectory() { m.NewFileAtProjectSelection() }

// CopySelectedTreeEntry copies the project-view selection into the project
// view clipboard, as OpenIDEA's Copy action does.
func (m *Model) CopySelectedTreeEntry() {
	paths := m.mutableTreePaths(m.selectedTreePaths())
	if len(paths) > 0 {
		m.SetTreeClipPaths(paths, false)
	}
}

// CutSelectedTreeEntry marks the project-view selection for a move on Paste.
func (m *Model) CutSelectedTreeEntry() {
	paths := m.selectedMutableTreePaths()
	if len(paths) > 0 {
		m.SetTreeClipPaths(paths, true)
	}
}

// PasteIntoTreeSelection pastes into the selected folder, or the selected
// file's parent.
func (m *Model) PasteIntoTreeSelection() {
	if dir := m.treeActionDirectory(); dir != "" {
		m.PasteTreeEntry(dir)
	}
}

// CopySelectedTreePath copies one of OpenIDEA's project-view path forms.
func (m *Model) CopySelectedTreePath(mode string) {
	target, ok := m.selectedTreeEntry()
	if !ok {
		return
	}
	text := target.path
	switch mode {
	case "name":
		text = m.projectTreePathBase(target.path)
	case "relative", "source":
		if rel, err := m.projectTreePathRelative(m.Root(), target.path); err == nil {
			text = rel
		}
	}
	mygo.Clipboard.WriteText(text)
	m.status = "Copied " + text
	m.invalidate()
}

// ShowSelectedFilePath copies the current project or editor path and reports
// it in the status bar.
func (m *Model) ShowSelectedFilePath() {
	path := ""
	if target, ok := m.selectedTreeEntry(); ok {
		path = target.path
	} else if tab := m.tab(); tab != nil && tab.kind == "" && tab.doc != nil {
		path = tab.doc.Path()
	}
	if path == "" {
		return
	}
	mygo.Clipboard.WriteText(path)
	m.status = path
	m.invalidate()
}

// MoveSelectedTreeEntry opens OpenIDEA's Move refactoring dialog.
func (m *Model) MoveSelectedTreeEntry() {
	paths := m.selectedMutableTreePaths()
	if len(paths) == 0 {
		return
	}
	targetFolder := m.projectTreePathDir(paths[0])
	if target, ok := m.selectedTreeEntry(); ok && target.dir {
		targetFolder = target.path
	}
	m.OpenFileMoveDialogFor(paths, targetFolder, false)
}

func (m *Model) moveTreeEntry(target treeTarget) {
	if target.root || target.path == "" {
		return
	}
	paths := m.selectedMutableTreePaths()
	if len(paths) == 0 {
		paths = []string{target.path}
	}
	m.OpenFileMoveDialogFor(paths, m.projectTreePathDir(paths[0]), false)
}

func (m *Model) moveProjectTreeEntry(target treeTarget, destination string) {
	if target.root || destination == "" {
		return
	}
	destination = m.projectTreePathClean(destination)
	if target.dir {
		inside := m.projectTreeUnderPath(destination, target.path)
		if inside {
			m.status = "A folder cannot be moved into itself"
			m.invalidate()
			return
		}
	}
	if !m.projectTreeIsDir(destination) {
		m.status = "Choose an existing destination folder"
		m.invalidate()
		return
	}
	to := m.projectTreePathJoin(destination, m.projectTreePathBase(target.path))
	if to == target.path {
		m.status = "It is already in that directory"
		m.invalidate()
		return
	}
	if m.projectTreeEntryKnown(to) {
		m.status = "The destination already has " + m.projectTreePathBase(to)
		m.invalidate()
		return
	}
	if bad := m.treeDirtyUnder(target.path); bad != "" {
		m.status = "Save or close it in the editor first: " + bad
		m.invalidate()
		return
	}
	reopen := target.path == m.tabPath()
	m.applyTreeMove(target.path, to, "Move "+m.projectTreePathBase(target.path), func(err error) {
		if err != nil {
			m.status = "Move failed: " + err.Error()
			m.invalidate()
			return
		}
		m.closedTreeTabs(target.path)
		m.moveBookmarkPaths(target.path, to)
		m.refreshTree(m.projectTreePathDir(target.path))
		m.refreshTree(m.projectTreePathDir(to))
		if m.tree != nil {
			m.tree.Select(to)
		}
		m.status = "Moved to " + to
		if reopen && !target.dir {
			m.openFile(to)
		}
		m.invalidate()
	})
}

func (m *Model) tabPath() string {
	if tab := m.tab(); tab != nil && tab.kind == "" && tab.doc != nil {
		return tab.doc.Path()
	}
	return ""
}

// runTreeFileCopies copies paths through the OpenIDEA filesystem backend. It
// keeps successful entries when another item fails, matching OpenIDEA's
// transfer loop, then reveals the first copy.
func (m *Model) runTreeFileCopies(
	copies []treeFileCopy,
	target string,
	openInEditor bool,
	done func(treeCopyOutcome),
) {
	if len(copies) == 0 {
		return
	}
	if m.ctx == nil || (m.mgr == nil && m.invokeHook == nil) {
		if done != nil {
			done(treeCopyOutcome{Failures: []string{"The GoIDEA backend is not connected"}})
		}
		return
	}
	m.status = "Copying…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 90*time.Second)
		defer cancel()
		outcome := treeCopyOutcome{}
		for _, copy := range copies {
			_, err := m.invoke(ctx, "fs_copy", map[string]any{
				"from": copy.From, "to": copy.To,
			})
			if err != nil {
				outcome.Failures = append(outcome.Failures,
					m.projectTreePathBase(copy.From)+": "+err.Error())
				continue
			}
			outcome.Copied = append(outcome.Copied, copy)
		}
		m.post(func() {
			if len(outcome.Copied) > 0 {
				m.refreshTree(target)
				created := make([]string, 0, len(outcome.Copied))
				for _, copied := range outcome.Copied {
					created = append(created, copied.To)
				}
				m.notifyVCSFileOperation("created", created)
				if m.tree != nil {
					m.tree.Select(outcome.Copied[0].To)
				}
				if openInEditor && len(outcome.Copied) == 1 {
					if !m.projectTreeIsDir(outcome.Copied[0].To) {
						m.openFile(outcome.Copied[0].To)
					}
				}
				if len(outcome.Copied) == 1 {
					m.status = "Copied " + m.projectTreePathBase(outcome.Copied[0].From) +
						" to " + m.projectTreePathBase(outcome.Copied[0].To)
				} else {
					m.status = fmt.Sprintf("Copied %d items", len(outcome.Copied))
				}
			} else {
				m.status = ""
			}
			if len(outcome.Failures) > 0 {
				m.notifyError("project.copy", "Some items could not be copied",
					strings.Join(outcome.Failures, "\n"))
			}
			if done != nil {
				done(outcome)
			}
			m.invalidate()
		})
	}()
}

// UndoLastTreeMove reverts the backend's last reference-aware move.
func (m *Model) UndoLastTreeMove() {
	record := m.treeLastMove
	if record == nil || (record.Kind != "" && record.Kind != "move") ||
		m.ctx == nil || (m.mgr == nil && m.invokeHook == nil) {
		m.status = "There is no file move to undo"
		m.invalidate()
		return
	}
	originalMoves := record.moves()
	openPaths := append([]string(nil), record.OpenPaths...)
	active := record.Active
	if len(openPaths) == 0 {
		for _, move := range originalMoves {
			if move.To == m.tabPath() {
				openPaths = []string{move.To}
				active = move.To
				break
			}
		}
	}
	m.status = "Undoing move…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 90*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "refactor_undo_file_move", map[string]any{"id": record.ID})
		var outcome struct {
			Moves     []treeFileMove `json:"moves"`
			Conflicts []string       `json:"conflicts"`
		}
		if err == nil {
			err = json.Unmarshal(raw, &outcome)
		}
		m.post(func() {
			if err != nil {
				m.status = "Undo move failed: " + err.Error()
				m.invalidate()
				return
			}
			reverted := outcome.Moves
			if len(reverted) == 0 {
				for index := len(originalMoves) - 1; index >= 0; index-- {
					original := originalMoves[index]
					reverted = append(reverted,
						treeFileMove{From: original.To, To: original.From})
				}
			}
			for _, move := range reverted {
				m.closedTreeTabs(move.From)
				m.moveBookmarkPaths(move.From, move.To)
				m.refreshTree(m.projectTreePathDir(move.From))
				m.refreshTree(m.projectTreePathDir(move.To))
			}
			if len(reverted) > 0 && m.tree != nil {
				m.tree.Select(reverted[0].To)
			}
			m.treeLastMove = nil
			if len(reverted) == 1 {
				m.status = "Undid move of " + m.projectTreePathBase(reverted[0].To)
			} else {
				m.status = fmt.Sprintf("Undid move of %d items", len(reverted))
			}
			opened := map[string]bool{}
			for _, path := range openPaths {
				restored := m.movedFilePath(path, reverted)
				if restored == path {
					continue
				}
				if !opened[restored] {
					opened[restored] = true
					m.openFile(restored)
				}
			}
			if restored := m.movedFilePath(active, reverted); restored != active {
				if index := m.tabForPath(restored); index >= 0 {
					m.activate(index)
				}
			}
			if len(outcome.Conflicts) > 0 {
				m.notifyWarning("project.refactor.undo", "Refactoring undone",
					"Changed since the refactoring, not reverted: "+
						strings.Join(outcome.Conflicts, ", "))
			}
			m.invalidate()
		})
	}()
}

// UndoLastProjectFileOperation is the Project view's Undo: it restores a newer
// Delete first, otherwise it reverts the last Move or Rename.
func (m *Model) UndoLastProjectFileOperation() {
	record := m.treeLastMove
	if record == nil {
		m.status = "Nothing to undo in the Project view"
		m.invalidate()
		return
	}
	if record.Kind == "delete" {
		m.undoLastTreeDelete()
		return
	}
	m.UndoLastTreeMove()
}

func (m *Model) undoLastTreeDelete() {
	record := m.treeLastMove
	if record == nil || record.Kind != "delete" ||
		m.ctx == nil || (m.mgr == nil && m.invokeHook == nil) {
		m.status = "Nothing to undo in the Project view"
		m.invalidate()
		return
	}
	paths := append([]string(nil), record.DeletePaths...)
	dirs := map[string]bool{}
	for _, path := range record.DeleteDirs {
		dirs[path] = true
	}
	m.treeLastMove = nil
	m.status = "Restoring deleted items…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 90*time.Second)
		defer cancel()
		var restored []string
		var failed []string
		parents := map[string]bool{}
		for _, deletedPath := range paths {
			raw, err := m.invoke(ctx, "history_list_dir", map[string]any{
				"path": deletedPath,
			})
			var history directoryHistory
			if err == nil {
				err = json.Unmarshal(raw, &history)
			}
			if err != nil {
				failed = append(failed, m.projectTreePathBase(deletedPath))
				continue
			}
			gone := map[string]bool{}
			for _, path := range history.Deleted {
				gone[path] = true
			}
			seen := map[string]bool{}
			var latest []historyEntry
			for _, entry := range history.Entries {
				if entry.Kind != "snapshot" || entry.ID == "" ||
					!gone[entry.Path] || seen[entry.Path] {
					continue
				}
				seen[entry.Path] = true
				latest = append(latest, entry)
			}
			ok := true
			if dirs[deletedPath] {
				if _, err := m.invoke(ctx, "fs_create_dir", map[string]any{
					"path": deletedPath,
				}); err != nil {
					ok = false
				}
			} else if len(latest) == 0 {
				ok = false
			}
			for _, entry := range latest {
				if _, err := m.invoke(ctx, "history_restore", map[string]any{
					"path": entry.Path, "id": entry.ID,
				}); err != nil {
					ok = false
					break
				}
			}
			if !ok {
				failed = append(failed, m.projectTreePathBase(deletedPath))
				continue
			}
			restored = append(restored, deletedPath)
			parents[m.projectTreePathDir(deletedPath)] = true
		}
		m.post(func() {
			for parent := range parents {
				m.refreshTree(parent)
			}
			if len(restored) > 0 && m.tree != nil {
				m.tree.Select(restored[0])
			}
			if len(restored) == 1 {
				m.status = "Restored " + m.projectTreePathBase(restored[0])
			} else if len(restored) > 1 {
				m.status = fmt.Sprintf("Restored %d items", len(restored))
			} else {
				m.status = ""
			}
			if len(failed) > 0 {
				m.notifyWarning("project.undoDelete",
					"Some items could not be restored",
					"Not in Local History: "+strings.Join(failed, ", ")+
						". They are in the Trash.")
			}
			m.invalidate()
		})
	}()
}

// applyTreeMove performs the same full reference and text search as the Move
// dialog, then applies the one undoable backend operation. done runs on the UI
// thread. The dialog path additionally stops at Refactoring Preview when
// references change in other files.
func (m *Model) applyTreeMove(from, to, label string, done func(error)) {
	if m.ctx == nil || (m.mgr == nil && m.invokeHook == nil) {
		done(fmt.Errorf("the GoIDEA backend is not connected"))
		return
	}
	moves := []treeFileMove{{From: from, To: to}}
	requests := []uint32{m.nextRequest(), m.nextRequest()}
	options := FileRefactorPrefs{
		References: boolPointer(true), TextOccurrences: boolPointer(true),
	}
	openPaths, active := m.openFileRefactorPaths()
	m.status = label + "…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 90*time.Second)
		defer cancel()
		edit, _ := m.collectFileRefactorEdits(ctx, requests, moves, options)
		rawEdit, marshalErr := json.Marshal(edit)
		err := marshalErr
		var moveID uint32
		if err == nil {
			raw, invokeErr := m.invoke(ctx, "refactor_move_files", map[string]any{
				"moves": moves, "edit": json.RawMessage(rawEdit), "label": label,
			})
			err = invokeErr
			if err == nil {
				err = json.Unmarshal(raw, &moveID)
			}
		}
		var record *treeMoveRecord
		if err == nil {
			record = &treeMoveRecord{
				ID: moveID, Kind: "move", Seq: treeFileOperationSequence(),
				From: from, To: to,
				Moves:     []treeFileMove{{From: from, To: to}},
				OpenPaths: openPaths, Active: active,
			}
		}
		m.post(func() {
			if record != nil {
				m.treeLastMove = record
			}
			done(err)
		})
	}()
}
