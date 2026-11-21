package app

// Project View Move / Rename follows OpenIDEA's file refactoring: save the
// open buffers, collect language-server and textual edits, preview changes in
// other files, apply the move with refactor_move_files, and reopen the moved
// editors at their new paths.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
)

type fileRefactorDialogState struct {
	kind  string // "move" or "rename"
	path  string
	paths []string
	dir   bool

	name   string
	target string

	references      bool
	textOccurrences bool
	openInEditor    bool
	clearClipboard  bool

	busy    bool
	err     string
	request []uint32
	cancel  context.CancelFunc
	moves   []treeFileMove
}

type fileRefactorPreviewFile struct {
	Path     string
	Relative string
	Name     string
	Enabled  bool
	Edits    []fileRefactorPreviewEdit
}

type fileRefactorPreviewEdit struct {
	Line int
	Pre  string
	Old  string
	New  string
	Post string
}

type fileRefactorPreviewState struct {
	title     string
	label     string
	moves     []treeFileMove
	edit      workspaceEdit
	files     []fileRefactorPreviewFile
	pre       string
	count     int
	warnings  []string
	run       fileRefactorRun
	openPaths []string
	active    string
	busy      bool
	err       string
	cancel    context.CancelFunc
}

type fileRefactorRun struct {
	moves          []treeFileMove
	edit           workspaceEdit
	label          string
	title          string
	openInEditor   bool
	clearClipboard bool
	openPaths      []string
	active         string
	warnings       []string
}

type workspacePosition struct {
	Line uint32 `json:"line"`
	Col  uint32 `json:"col"`
}

type workspaceRange struct {
	Start workspacePosition `json:"start"`
	End   workspacePosition `json:"end"`
}

type workspaceTextEdit struct {
	Range   workspaceRange `json:"range"`
	NewText string         `json:"new_text"`
}

type workspaceFileEdit struct {
	Kind      string              `json:"kind"`
	Path      string              `json:"path,omitempty"`
	Edits     []workspaceTextEdit `json:"edits,omitempty"`
	Version   *int32              `json:"version,omitempty"`
	From      string              `json:"from,omitempty"`
	To        string              `json:"to,omitempty"`
	Overwrite bool                `json:"overwrite,omitempty"`
	Recursive bool                `json:"recursive,omitempty"`
}

type workspacePrecondition struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
}

type workspaceEdit struct {
	Files         []workspaceFileEdit     `json:"files"`
	Preconditions []workspacePrecondition `json:"preconditions,omitempty"`
}

type fileRenameEditsResponse struct {
	Edit    *workspaceEdit `json:"edit"`
	Servers []string       `json:"servers"`
	Errors  []string       `json:"errors"`
}

func emptyWorkspaceEdit() workspaceEdit {
	return workspaceEdit{Files: []workspaceFileEdit{}}
}

func defaultFileRefactorPrefs(kind string, prefs ProjectTreePrefs) FileRefactorPrefs {
	if kind == "rename" {
		return prefs.RenameRefactor
	}
	return prefs.MoveRefactor
}

// OpenFileRenameDialog opens OpenIDEA's Rename dialog, which updates
// references rather than issuing a plain fs_rename.
func (m *Model) OpenFileRenameDialog(path string) {
	if path == "" || path == m.Root() {
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		m.status = "Rename failed: " + err.Error()
		m.invalidate()
		return
	}
	stored := defaultFileRefactorPrefs("rename", m.prefs.ProjectTree)
	m.fileRefactorPreview = nil
	m.treeOp = nil
	m.fileRefactor = &fileRefactorDialogState{
		kind:            "rename",
		path:            path,
		dir:             info.IsDir(),
		name:            filepath.Base(path),
		references:      stored.referenceSearch(),
		textOccurrences: stored.textSearch(),
		openInEditor:    stored.opensInEditor(),
	}
	m.invalidate()
}

// OpenFileMoveDialog opens the Move dialog with the destination field.
// targetFolder is the initial folder, as F6 or Cut › Paste supplies.
func (m *Model) OpenFileMoveDialog(path, targetFolder string, clearClipboard bool) {
	m.OpenFileMoveDialogFor([]string{path}, targetFolder, clearClipboard)
}

// OpenFileMoveDialogFor opens the Move dialog for the project selection.
func (m *Model) OpenFileMoveDialogFor(paths []string, targetFolder string, clearClipboard bool) {
	paths = m.mutableTreePaths(paths)
	if len(paths) == 0 {
		return
	}
	path := paths[0]
	if path == "" || path == m.Root() {
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		m.status = "Move failed: " + err.Error()
		m.invalidate()
		return
	}
	if targetFolder == "" {
		targetFolder = filepath.Dir(path)
	}
	stored := defaultFileRefactorPrefs("move", m.prefs.ProjectTree)
	m.fileRefactorPreview = nil
	m.treeOp = nil
	m.fileRefactor = &fileRefactorDialogState{
		kind:            "move",
		path:            path,
		paths:           paths,
		dir:             info.IsDir(),
		target:          m.displayRefactorTarget(targetFolder),
		references:      stored.referenceSearch(),
		textOccurrences: stored.textSearch(),
		openInEditor:    stored.opensInEditor(),
		clearClipboard:  clearClipboard,
	}
	m.invalidate()
}

// OpenFileCopyDialog opens OpenIDEA's Copy dialog for one or more project-view
// entries. A single entry can be renamed in place; several entries receive
// Finder-style non-conflicting names.
func (m *Model) OpenFileCopyDialog(paths []string, targetFolder string) {
	paths = m.mutableTreePaths(paths)
	if len(paths) == 0 {
		return
	}
	if targetFolder == "" {
		targetFolder = filepath.Dir(paths[0])
	}
	dir := false
	if len(paths) == 1 {
		if info, err := os.Stat(paths[0]); err == nil {
			dir = info.IsDir()
		}
	}
	m.fileRefactorPreview = nil
	m.treeOp = nil
	m.fileRefactor = &fileRefactorDialogState{
		kind:         "copy",
		path:         paths[0],
		paths:        append([]string(nil), paths...),
		dir:          dir,
		name:         m.uniqueTreeCopyName(targetFolder, filepath.Base(paths[0]), dir),
		target:       m.displayRefactorTarget(targetFolder),
		openInEditor: m.prefs.ProjectTree.CopyOpenEditor,
	}
	m.invalidate()
}

func (m *Model) displayRefactorTarget(path string) string {
	root := m.Root()
	if root != "" {
		if rel, err := filepath.Rel(root, path); err == nil {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(path)
}

func (m *Model) resolveRefactorTarget(target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return filepath.Clean(m.Root())
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(m.Root(), filepath.FromSlash(target))
	}
	return filepath.Clean(target)
}

func refactorNameError(name string) string {
	switch {
	case name == "":
		return "The name cannot be empty"
	case name == "." || name == "..":
		return "The name is not valid"
	case strings.TrimSpace(name) != name:
		return "The name cannot start or end with a space"
	case strings.HasSuffix(name, "."):
		return "The name cannot end with a period"
	case strings.ContainsAny(name, `/\`):
		return "The name cannot contain a path separator"
	}
	for _, r := range name {
		if r < 32 || (r >= 127 && r <= 159) {
			return "The name cannot contain control characters"
		}
	}
	stem := name
	if dot := strings.IndexByte(stem, '.'); dot >= 0 {
		stem = stem[:dot]
	}
	switch strings.ToUpper(stem) {
	case "CON", "PRN", "AUX", "NUL":
		return "That name is reserved"
	}
	if len(stem) == 4 && (strings.HasPrefix(strings.ToUpper(stem), "COM") ||
		strings.HasPrefix(strings.ToUpper(stem), "LPT")) &&
		stem[3] >= '1' && stem[3] <= '9' {
		return "That name is reserved"
	}
	return ""
}

func caseInsensitiveFileSystem() bool {
	return runtime.GOOS == "darwin" || runtime.GOOS == "windows"
}

func sameFileName(left, right string) bool {
	if caseInsensitiveFileSystem() {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func (m *Model) refactorRenameMove(d *fileRefactorDialogState) (treeFileMove, error) {
	if errText := refactorNameError(d.name); errText != "" {
		return treeFileMove{}, errors.New(errText)
	}
	to := filepath.Join(filepath.Dir(d.path), d.name)
	if to == d.path {
		return treeFileMove{}, errors.New("The name is unchanged")
	}
	entries, err := os.ReadDir(filepath.Dir(d.path))
	if err != nil {
		return treeFileMove{}, err
	}
	for _, entry := range entries {
		if entry.Name() == filepath.Base(d.path) {
			continue
		}
		if sameFileName(entry.Name(), d.name) {
			return treeFileMove{}, errors.New("A file or directory with that name already exists")
		}
	}
	return treeFileMove{From: d.path, To: to}, nil
}

func (m *Model) refactorMoveTarget(d *fileRefactorDialogState) (treeFileMove, error) {
	moves, err := m.refactorMoveTargets(d)
	if err != nil {
		return treeFileMove{}, err
	}
	return moves[0], nil
}

func (m *Model) refactorMoveTargets(d *fileRefactorDialogState) ([]treeFileMove, error) {
	destination := m.resolveRefactorTarget(d.target)
	info, err := os.Stat(destination)
	if err != nil || !info.IsDir() {
		return nil, errors.New("Choose an existing destination directory")
	}
	paths := d.paths
	if len(paths) == 0 {
		paths = []string{d.path}
	}
	moves := make([]treeFileMove, 0, len(paths))
	destinations := map[string]bool{}
	for _, path := range paths {
		if info, err := os.Stat(path); err == nil && info.IsDir() &&
			(destination == path ||
				strings.HasPrefix(destination, path+string(filepath.Separator))) {
			return nil, errors.New("Cannot move a directory into itself")
		}
		to := filepath.Join(destination, filepath.Base(path))
		if to == path {
			continue
		}
		key := keyForCaseInsensitiveFileSystem(to)
		if destinations[key] {
			return nil, errors.New("Two selected items have the same name")
		}
		destinations[key] = true
		if _, err := os.Stat(to); err == nil {
			return nil, errors.New("The destination already has " + filepath.Base(to))
		}
		moves = append(moves, treeFileMove{From: path, To: to})
	}
	if len(moves) == 0 {
		return nil, errors.New("Already in this directory")
	}
	return moves, nil
}

func uniqueTreeName(name string, isDir bool, taken map[string]bool) string {
	key := func(candidate string) string {
		if caseInsensitiveFileSystem() {
			return strings.ToLower(candidate)
		}
		return candidate
	}
	if !taken[key(name)] {
		return name
	}
	stem, extension := name, ""
	if !isDir {
		if dot := strings.LastIndexByte(name, '.'); dot > 0 {
			stem, extension = name[:dot], name[dot:]
		}
	}
	for number := 1; ; number++ {
		suffix := " copy"
		if number > 1 {
			suffix += " " + strconv.Itoa(number)
		}
		candidate := stem + suffix + extension
		if !taken[key(candidate)] {
			return candidate
		}
	}
}

func (m *Model) copyDialogPlan(d *fileRefactorDialogState) ([]treeFileCopy, error) {
	destination := m.resolveRefactorTarget(d.target)
	info, err := os.Stat(destination)
	if err != nil || !info.IsDir() {
		return nil, errors.New("Choose an existing destination directory")
	}
	paths := d.paths
	if len(paths) == 0 {
		paths = []string{d.path}
	}
	entries, err := os.ReadDir(destination)
	if err != nil {
		return nil, err
	}
	key := keyForCaseInsensitiveFileSystem
	taken := make(map[string]bool, len(entries)+len(paths))
	for _, entry := range entries {
		taken[key(entry.Name())] = true
	}
	name := strings.TrimSpace(d.name)
	if len(paths) == 1 {
		if errText := refactorNameError(name); errText != "" {
			return nil, errors.New(errText)
		}
	}
	copies := make([]treeFileCopy, 0, len(paths))
	for _, source := range paths {
		sourceInfo, err := os.Stat(source)
		if err != nil {
			return nil, err
		}
		if sourceInfo.IsDir() && destination != filepath.Dir(source) &&
			underTreePath(destination, source) {
			return nil, errors.New("Cannot copy a directory into itself")
		}
		finalName := name
		if len(paths) > 1 {
			finalName = uniqueTreeName(filepath.Base(source), sourceInfo.IsDir(), taken)
		}
		if len(paths) == 1 && taken[key(finalName)] {
			return nil, errors.New("A file or directory named " + finalName + " already exists")
		}
		to := filepath.Join(destination, finalName)
		if to == source {
			return nil, errors.New("The copy would replace the original")
		}
		copies = append(copies, treeFileCopy{From: source, To: to})
		taken[key(finalName)] = true
	}
	if len(copies) == 0 {
		return nil, errors.New("There is nothing to copy")
	}
	return copies, nil
}

// keyForCaseInsensitiveFileSystem returns a lookup key that is lowercased only
// on file systems where names compare case-insensitively.
func keyForCaseInsensitiveFileSystem(name string) string {
	if caseInsensitiveFileSystem() {
		return strings.ToLower(name)
	}
	return name
}

func (m *Model) refactorDialogMove(d *fileRefactorDialogState) (treeFileMove, error) {
	if d.kind == "rename" {
		return m.refactorRenameMove(d)
	}
	return m.refactorMoveTarget(d)
}

func (m *Model) refactorDialogMoves(d *fileRefactorDialogState) ([]treeFileMove, error) {
	if d.kind == "rename" {
		move, err := m.refactorRenameMove(d)
		if err != nil {
			return nil, err
		}
		return []treeFileMove{move}, nil
	}
	return m.refactorMoveTargets(d)
}

// ConfirmFileRefactor validates the dialog and starts OpenIDEA's reference
// search. The dialog closes after the search and the preview or move follows.
func (m *Model) ConfirmFileRefactor() {
	d := m.fileRefactor
	if d == nil || d.busy {
		return
	}
	if d.kind == "copy" {
		m.confirmFileCopy(d)
		return
	}
	if m.ctx == nil || (m.mgr == nil && m.invokeHook == nil) {
		d.err = "The GoIDEA backend is not connected"
		m.invalidate()
		return
	}
	moves, err := m.refactorDialogMoves(d)
	if err != nil {
		if d.kind == "rename" && d.name == filepath.Base(d.path) {
			m.closeFileRefactorDialog(false)
			return
		}
		d.err = err.Error()
		m.invalidate()
		return
	}

	prefs := FileRefactorPrefs{
		References:      boolPointer(d.references),
		TextOccurrences: boolPointer(d.textOccurrences),
		OpenInEditor:    boolPointer(d.openInEditor),
	}
	if d.kind == "rename" {
		m.prefs.ProjectTree.RenameRefactor = prefs
	} else {
		m.prefs.ProjectTree.MoveRefactor = prefs
	}
	m.savePreferences()

	d.busy = true
	d.err = ""
	d.moves = moves
	openPaths, active := m.openFileRefactorPaths()
	tabs := append([]*Tab(nil), m.allTabs()...)
	base := m.ctx
	if base == nil {
		base = context.Background()
	}
	ctx, cancel := context.WithTimeout(base, 90*time.Second)
	d.cancel = cancel
	if d.references {
		d.request = append(d.request, m.nextRequest())
	}
	if d.textOccurrences {
		d.request = append(d.request, m.nextRequest())
	}
	requests := append([]uint32(nil), d.request...)
	options := FileRefactorPrefs{
		References:      boolPointer(d.references),
		TextOccurrences: boolPointer(d.textOccurrences),
		OpenInEditor:    boolPointer(d.openInEditor),
	}
	label := d.kind
	if label == "move" {
		label = "Move"
	} else {
		label = "Rename"
	}
	title := describeFileRefactor(d.moves, label)
	m.status = "Looking for references…"
	m.invalidate()

	go func() {
		if d.references || d.textOccurrences {
			// OpenIDEA saves quietly before servers and text search read disk.
			_ = m.saveOpenTabs(ctx, tabs)
		}
		edit, warnings := m.collectFileRefactorEdits(ctx, requests, d.moves, options)
		m.post(func() {
			if m.fileRefactor != d {
				return
			}
			d.busy = false
			d.cancel = nil
			d.request = nil
			if ctx.Err() != nil {
				m.closeFileRefactorDialog(false)
				return
			}
			m.fileRefactor = nil
			m.status = ""
			run := fileRefactorRun{
				moves:          append([]treeFileMove(nil), d.moves...),
				edit:           edit,
				label:          label,
				title:          title,
				openInEditor:   d.openInEditor,
				clearClipboard: d.clearClipboard,
				openPaths:      openPaths,
				active:         active,
				warnings:       warnings,
			}
			if fileRefactorTouchesOtherFiles(edit, run.moves) {
				m.showFileRefactorPreview(run)
			} else {
				m.applyPreparedFileRefactor(run, nil)
			}
			m.invalidate()
		})
	}()
}

func (m *Model) confirmFileCopy(d *fileRefactorDialogState) {
	if m.ctx == nil || (m.mgr == nil && m.invokeHook == nil) {
		d.err = "The GoIDEA backend is not connected"
		m.invalidate()
		return
	}
	copies, err := m.copyDialogPlan(d)
	if err != nil {
		d.err = err.Error()
		m.invalidate()
		return
	}
	m.prefs.ProjectTree.CopyOpenEditor = d.openInEditor
	m.savePreferences()
	d.busy = true
	d.err = ""
	target := m.resolveRefactorTarget(d.target)
	m.invalidate()
	m.runTreeFileCopies(copies, target, d.openInEditor && !d.dir, func(outcome treeCopyOutcome) {
		if m.fileRefactor != d {
			return
		}
		d.busy = false
		if len(outcome.Copied) == 0 {
			d.err = "The copy failed"
			if len(outcome.Failures) > 0 {
				d.err = outcome.Failures[0]
			}
			return
		}
		m.fileRefactor = nil
	})
}

func (m *Model) closeFileRefactorDialog(cancel bool) {
	d := m.fileRefactor
	if d == nil {
		return
	}
	if cancel {
		if d.cancel != nil {
			d.cancel()
		}
		for _, id := range d.request {
			m.cancelRequest(id)
		}
	}
	m.fileRefactor = nil
	m.status = ""
	m.invalidate()
}

func (m *Model) openFileRefactorPaths() ([]string, string) {
	tabs := m.allTabs()
	paths := make([]string, 0, len(tabs))
	seen := map[string]bool{}
	for _, tab := range tabs {
		if tab == nil || tab.kind != "" || tab.doc == nil || tab.doc.Path() == "" {
			continue
		}
		path := tab.doc.Path()
		if !seen[path] {
			seen[path] = true
			paths = append(paths, path)
		}
	}
	return paths, m.tabPath()
}

func (m *Model) collectFileRefactorEdits(
	ctx context.Context,
	requests []uint32,
	moves []treeFileMove,
	options FileRefactorPrefs,
) (workspaceEdit, []string) {
	empty := emptyWorkspaceEdit()
	if !options.referenceSearch() && !options.textSearch() {
		return empty, nil
	}
	type answer struct {
		lsp  bool
		edit workspaceEdit
		warn []string
		err  error
	}
	answers := make(chan answer, 2)
	var wg sync.WaitGroup
	nextRequest := 0
	if options.referenceSearch() {
		request := uint32(0)
		if nextRequest < len(requests) {
			request = requests[nextRequest]
		}
		nextRequest++
		wg.Add(1)
		go func() {
			defer wg.Done()
			raw, err := m.invoke(ctx, "refactor_file_move_edits", map[string]any{
				"request": request, "moves": moves,
			})
			if err != nil {
				answers <- answer{lsp: true, edit: emptyWorkspaceEdit(), err: err}
				return
			}
			var response fileRenameEditsResponse
			if err := json.Unmarshal(raw, &response); err != nil {
				answers <- answer{lsp: true, edit: emptyWorkspaceEdit(), err: err}
				return
			}
			edit := emptyWorkspaceEdit()
			if response.Edit != nil {
				edit = *response.Edit
				normalizeWorkspaceEdit(&edit)
			}
			answers <- answer{lsp: true, edit: edit, warn: response.Errors}
		}()
	}
	if options.textSearch() {
		request := uint32(0)
		if nextRequest < len(requests) {
			request = requests[nextRequest]
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			raw, err := m.invoke(ctx, "refactor_file_move_text_edits", map[string]any{
				"request": request, "moves": moves,
			})
			edit := emptyWorkspaceEdit()
			if err == nil {
				err = json.Unmarshal(raw, &edit)
				normalizeWorkspaceEdit(&edit)
			}
			answers <- answer{edit: edit, err: err}
		}()
	}
	go func() {
		wg.Wait()
		close(answers)
	}()

	lsp, text := emptyWorkspaceEdit(), emptyWorkspaceEdit()
	var warnings []string
	for result := range answers {
		if result.err != nil {
			if ctx.Err() == nil {
				warnings = append(warnings, result.err.Error())
			}
			continue
		}
		warnings = append(warnings, result.warn...)
		if result.lsp {
			lsp = result.edit
		} else {
			text = result.edit
		}
	}
	return mergeFileMoveEdits(lsp, text), warnings
}

func normalizeWorkspaceEdit(edit *workspaceEdit) {
	if edit.Files == nil {
		edit.Files = []workspaceFileEdit{}
	}
}

func positionBefore(left, right workspacePosition) bool {
	return left.Line < right.Line || (left.Line == right.Line && left.Col <= right.Col)
}

func workspaceRangeOverlaps(left, right workspaceRange) bool {
	return !positionBefore(left.End, right.Start) && !positionBefore(right.End, left.Start)
}

type editVersion struct {
	set   bool
	value *int32
}

func cloneInt32(value *int32) *int32 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

// mergeFileMoveEdits combines server and textual changes under the same rules
// as OpenIDEA: text hits already covered by the language server disappear,
// incompatible versions stay as guards, and duplicate paths merge in order.
func mergeFileMoveEdits(lsp, text workspaceEdit) workspaceEdit {
	versions := map[string]editVersion{}
	guards := []workspaceFileEdit{}
	byPath := map[string][]workspaceTextEdit{}
	order := []string{}

	for _, file := range lsp.Files {
		if file.Kind != "edit" {
			continue
		}
		previous := versions[file.Path]
		if previous.set && previous.value != nil && file.Version != nil &&
			*previous.value != *file.Version {
			guards = append(guards, workspaceFileEdit{
				Kind: "edit", Path: file.Path,
				Edits: []workspaceTextEdit{}, Version: cloneInt32(previous.value),
			})
		}
		if file.Version != nil {
			versions[file.Path] = editVersion{set: true, value: cloneInt32(file.Version)}
		} else if !previous.set {
			versions[file.Path] = editVersion{set: true}
		}
		if _, exists := byPath[file.Path]; !exists {
			order = append(order, file.Path)
		}
		byPath[file.Path] = append(byPath[file.Path], file.Edits...)
	}

	for _, file := range text.Files {
		if file.Kind != "edit" {
			continue
		}
		existing := byPath[file.Path]
		extra := make([]workspaceTextEdit, 0, len(file.Edits))
		for _, candidate := range file.Edits {
			overlaps := false
			for _, edit := range existing {
				if workspaceRangeOverlaps(edit.Range, candidate.Range) {
					overlaps = true
					break
				}
			}
			if !overlaps {
				extra = append(extra, candidate)
			}
		}
		if len(extra) == 0 {
			continue
		}
		if _, exists := byPath[file.Path]; !exists {
			order = append(order, file.Path)
		}
		byPath[file.Path] = append(existing, extra...)
	}

	files := append([]workspaceFileEdit(nil), guards...)
	for _, path := range order {
		edits := byPath[path]
		if len(edits) == 0 {
			continue
		}
		version := versions[path]
		files = append(files, workspaceFileEdit{
			Kind: "edit", Path: path, Edits: edits, Version: cloneInt32(version.value),
		})
	}
	preconditions := append([]workspacePrecondition(nil), lsp.Preconditions...)
	preconditions = append(preconditions, text.Preconditions...)
	return workspaceEdit{Files: files, Preconditions: preconditions}
}

func fileRefactorTouchesOtherFiles(edit workspaceEdit, moves []treeFileMove) bool {
	for _, file := range edit.Files {
		if file.Kind != "edit" || len(file.Edits) == 0 {
			continue
		}
		insideMoved := false
		for _, move := range moves {
			if underTreePath(file.Path, move.From) {
				insideMoved = true
				break
			}
		}
		if !insideMoved {
			return true
		}
	}
	return false
}

func movedFilePath(path string, moves []treeFileMove) string {
	for _, move := range moves {
		if underTreePath(path, move.From) {
			return move.To + path[len(move.From):]
		}
	}
	return path
}

func describeFileRefactor(moves []treeFileMove, kind string) string {
	if len(moves) != 1 {
		return fmt.Sprintf("%s %d items", kind, len(moves))
	}
	move := moves[0]
	if kind == "rename" || filepath.Dir(move.From) == filepath.Dir(move.To) {
		return fmt.Sprintf("Rename '%s' to '%s'", filepath.Base(move.From), filepath.Base(move.To))
	}
	return fmt.Sprintf("Move '%s' to '%s'", filepath.Base(move.From), filepath.Dir(move.To))
}

func (m *Model) showFileRefactorPreview(run fileRefactorRun) {
	files := make([]fileRefactorPreviewFile, 0)
	count := 0
	for _, file := range run.edit.Files {
		if file.Kind != "edit" || len(file.Edits) == 0 {
			continue
		}
		preview := fileRefactorPreviewFile{
			Path: file.Path, Name: filepath.Base(file.Path), Enabled: true,
		}
		if rel, err := filepath.Rel(m.Root(), file.Path); err == nil {
			preview.Relative = filepath.ToSlash(rel)
		} else {
			preview.Relative = filepath.ToSlash(file.Path)
		}
		preview.Edits = m.fileRefactorPreviewEdits(file.Path, file.Edits)
		count += len(file.Edits)
		files = append(files, preview)
	}
	m.fileRefactorPreview = &fileRefactorPreviewState{
		title: run.title, label: run.label, moves: run.moves, edit: run.edit,
		files: files, count: count, warnings: run.warnings, run: run,
		openPaths: run.openPaths, active: run.active,
	}
	m.invalidate()
}

func utf16ColumnToRunes(text string, column uint32) int {
	if column == 0 {
		return 0
	}
	units := uint32(0)
	for index, r := range text {
		width := uint32(1)
		if r > 0xffff {
			width = 2
		}
		if units+width > column {
			return index
		}
		units += width
		if units == column {
			return index + len(string(r))
		}
	}
	return len(text)
}

func (m *Model) refactorPreviewSource(path string) string {
	for _, tab := range m.allTabs() {
		if tab != nil && tab.doc != nil && tab.doc.Path() == path {
			return tab.doc.Buffer().String()
		}
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() > 8<<20 {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

func (m *Model) fileRefactorPreviewEdits(path string, edits []workspaceTextEdit) []fileRefactorPreviewEdit {
	source := strings.ReplaceAll(m.refactorPreviewSource(path), "\r\n", "\n")
	source = strings.ReplaceAll(source, "\r", "\n")
	lines := strings.Split(source, "\n")
	sorted := append([]workspaceTextEdit(nil), edits...)
	sort.SliceStable(sorted, func(i, j int) bool {
		left, right := sorted[i].Range.Start, sorted[j].Range.Start
		return left.Line < right.Line || (left.Line == right.Line && left.Col < right.Col)
	})
	out := make([]fileRefactorPreviewEdit, 0, min(len(sorted), 50))
	for _, edit := range sorted[:min(len(sorted), 50)] {
		line := int(edit.Range.Start.Line)
		text := ""
		if line >= 0 && line < len(lines) {
			text = strings.TrimSuffix(lines[line], "\r")
		}
		start := utf16ColumnToRunes(text, edit.Range.Start.Col)
		end := len(text)
		if edit.Range.End.Line == edit.Range.Start.Line {
			end = utf16ColumnToRunes(text, edit.Range.End.Col)
		}
		start = max(0, min(start, len(text)))
		end = max(start, min(end, len(text)))
		lead := len(text) - len(strings.TrimLeft(text, " \t"))
		trimmedStart := max(0, start-lead)
		trimmedEnd := max(trimmedStart, end-lead)
		trimmed := text[lead:]
		trimmedEnd = min(trimmedEnd, len(trimmed))
		preview := fileRefactorPreviewEdit{
			Line: line + 1,
			Pre:  trimmed[:trimmedStart],
			Old:  trimmed[trimmedStart:trimmedEnd],
			New:  strings.ReplaceAll(edit.NewText, "\n", " ↵ "),
			Post: trimmed[trimmedEnd:],
		}
		out = append(out, preview)
	}
	return out
}

func (m *Model) closeFileRefactorPreview(cancel bool) {
	p := m.fileRefactorPreview
	if p == nil {
		return
	}
	if cancel && p.cancel != nil {
		p.cancel()
	}
	m.fileRefactorPreview = nil
	m.status = ""
	m.invalidate()
}

func (p *fileRefactorPreviewState) acceptedEdit() workspaceEdit {
	files := make([]workspaceFileEdit, 0)
	enabled := map[string]bool{}
	for _, file := range p.files {
		enabled[file.Path] = file.Enabled
	}
	for _, file := range p.edit.Files {
		if file.Kind == "edit" && !enabled[file.Path] {
			continue
		}
		files = append(files, file)
	}
	return workspaceEdit{Files: files, Preconditions: p.edit.Preconditions}
}

// ConfirmFileRefactorPreview applies the included files as one undoable move.
func (m *Model) ConfirmFileRefactorPreview() {
	p := m.fileRefactorPreview
	if p == nil || p.busy {
		return
	}
	run := p.run
	run.edit = p.acceptedEdit()
	p.busy = true
	p.err = ""
	m.status = "Refactoring…"
	m.invalidate()
	m.applyPreparedFileRefactor(run, func(err error) {
		if m.fileRefactorPreview != p {
			return
		}
		if err != nil {
			p.busy = false
			p.err = err.Error()
			m.status = ""
			m.invalidate()
			return
		}
		m.fileRefactorPreview = nil
		m.status = ""
		m.invalidate()
	})
}

func (m *Model) applyPreparedFileRefactor(run fileRefactorRun, done func(error)) {
	if len(run.moves) == 0 {
		if done != nil {
			done(nil)
		}
		return
	}
	if m.ctx == nil || (m.mgr == nil && m.invokeHook == nil) {
		err := errors.New("the GoIDEA backend is not connected")
		if done != nil {
			done(err)
		} else {
			m.status = "Refactoring failed: " + err.Error()
			m.invalidate()
		}
		return
	}
	edit, err := json.Marshal(run.edit)
	if err != nil {
		if done != nil {
			done(err)
		}
		return
	}
	m.status = "Refactoring…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 90*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "refactor_move_files", map[string]any{
			"moves": run.moves, "edit": json.RawMessage(edit), "label": run.label,
		})
		var id uint32
		if err == nil {
			err = json.Unmarshal(raw, &id)
		}
		m.post(func() {
			if err != nil {
				if done != nil {
					done(err)
				} else {
					m.notifyError("project.refactor.move", "Cannot "+strings.ToLower(run.label),
						err.Error())
					m.status = ""
					m.invalidate()
				}
				return
			}
			m.finishPreparedFileRefactor(run, id)
			if run.clearClipboard {
				m.treeClip = nil
			}
			if done != nil {
				done(nil)
			}
			m.invalidate()
		})
	}()
}

func (m *Model) finishPreparedFileRefactor(run fileRefactorRun, id uint32) {
	oldPaths := append([]string(nil), run.openPaths...)
	for _, move := range run.moves {
		m.closedTreeTabs(move.From)
		m.moveBookmarkPaths(move.From, move.To)
		m.refreshTree(filepath.Dir(move.From))
		m.refreshTree(filepath.Dir(move.To))
	}
	m.treeLastMove = &treeMoveRecord{
		ID: id, Kind: "move", Seq: treeFileOperationSequence(),
		Moves:     append([]treeFileMove(nil), run.moves...),
		OpenPaths: oldPaths, Active: run.active,
	}
	if len(run.moves) == 1 {
		m.treeLastMove.From = run.moves[0].From
		m.treeLastMove.To = run.moves[0].To
	}
	opened := map[string]bool{}
	reopen := func(path string) {
		if path == "" || opened[path] {
			return
		}
		opened[path] = true
		m.openFile(path)
	}
	for _, path := range oldPaths {
		if moved := movedFilePath(path, run.moves); moved != path {
			reopen(moved)
		}
	}
	if run.openInEditor {
		for _, move := range run.moves {
			if info, err := os.Stat(move.To); err == nil && !info.IsDir() {
				reopen(move.To)
			}
		}
	}
	active := movedFilePath(run.active, run.moves)
	if active != "" {
		if index := m.tabForPath(active); index >= 0 {
			m.activate(index)
		}
	}
	if len(run.moves) > 0 && m.tree != nil {
		m.tree.Select(run.moves[0].To)
	}
	m.status = run.title
	edited := len(run.edit.Files)
	body := ""
	if edited == 1 {
		body = "Updated references in 1 file"
	} else if edited > 1 {
		body = fmt.Sprintf("Updated references in %d files", edited)
	}
	m.notifyInfo("project.refactor.done", run.title, body, notifyAction{
		label: "Undo",
		run: func(m *Model) {
			m.UndoLastTreeMove()
		},
	})
	if len(run.warnings) > 0 {
		m.notifyWarning("project.refactor.errors",
			"Some references may not have been updated",
			strings.Join(run.warnings, "\n"))
	}
}

// fileRefactorDialog draws the native Move or Rename sheet.
func (m *Model) fileRefactorDialog(c *ui.Context, p *ui.Theme) {
	d := m.fileRefactor
	if d == nil {
		return
	}
	open := true
	modal := ui.Modal(c.Key("project-file-refactor"), &open, func() {
		title := "Move"
		action := "Refactor"
		subject := ""
		switch d.kind {
		case "rename":
			title = "Rename"
			word := "file"
			if d.dir {
				word = "directory"
			}
			subject = fmt.Sprintf("Rename %s '%s' and its usages to:", word, filepath.Base(d.path))
		case "copy":
			title, action = "Copy", "OK"
			if len(d.paths) > 1 {
				subject = fmt.Sprintf("Copy %d items", len(d.paths))
			} else {
				word := "file"
				if d.dir {
					word = "directory"
				}
				subject = fmt.Sprintf("Copy %s '%s'", word, filepath.Base(d.path))
			}
		default:
			word := "file"
			if d.dir {
				word = "directory"
			}
			if len(d.paths) > 1 {
				subject = fmt.Sprintf("Move %d items", len(d.paths))
			} else {
				subject = fmt.Sprintf("Move %s '%s'", word, filepath.Base(d.path))
			}
		}
		ui.Column(c).Width(520).Gap(8).Children(func() {
			ui.Text(c, title).FontSize(14).Bold().TextColor(p.Text).SingleLine()
			ui.Text(c, subject).FontSize(12).TextColor(p.Text).SingleLine().Ellipsis("…")
			switch d.kind {
			case "rename":
				input := ui.TextInput(c, &d.name).AutoFocus().Disabled(d.busy)
				input.OnChange(func() { d.err = "" })
				if input.Submitted() {
					m.ConfirmFileRefactor()
				}
			case "copy":
				autoFocus := len(d.paths) > 1
				if len(d.paths) == 1 {
					ui.Text(c, "New name:").FontSize(12).TextColor(p.TextMuted).SingleLine()
					input := ui.TextInput(c, &d.name).AutoFocus().Disabled(d.busy)
					input.OnChange(func() { d.err = "" })
					if input.Submitted() {
						m.ConfirmFileRefactor()
					}
				}
				ui.Text(c, "To directory:").FontSize(12).TextColor(p.TextMuted).SingleLine()
				input := ui.TextInput(c, &d.target).Disabled(d.busy)
				if autoFocus {
					input.AutoFocus()
				}
				input.OnChange(func() { d.err = "" })
				target := m.resolveRefactorTarget(d.target)
				hint := filepath.ToSlash(target)
				if rel, err := filepath.Rel(m.Root(), target); err == nil {
					if rel == "." {
						hint = "Project root"
					} else {
						hint = filepath.ToSlash(rel)
					}
				}
				ui.Text(c, hint).FontSize(11).TextColor(p.TextMuted).SingleLine().Ellipsis("…")
				if input.Submitted() {
					m.ConfirmFileRefactor()
				}
				if len(d.paths) == 1 && !d.dir {
					ui.Checkbox(c, &d.openInEditor, "Open copy in editor").Disabled(d.busy)
				}
			default:
				ui.Text(c, "To directory:").FontSize(12).TextColor(p.TextMuted).SingleLine()
				input := ui.TextInput(c, &d.target).AutoFocus().Disabled(d.busy)
				input.OnChange(func() { d.err = "" })
				target := m.resolveRefactorTarget(d.target)
				hint := filepath.ToSlash(target)
				if rel, err := filepath.Rel(m.Root(), target); err == nil {
					if rel == "." {
						hint = "Project root"
					} else {
						hint = filepath.ToSlash(rel)
					}
				}
				ui.Text(c, hint).FontSize(11).TextColor(p.TextMuted).SingleLine().Ellipsis("…")
				if input.Submitted() {
					m.ConfirmFileRefactor()
				}
			}
			if d.kind != "copy" {
				ui.Checkbox(c, &d.references, "Search for references").Disabled(d.busy)
				ui.Checkbox(c, &d.textOccurrences, "Search in comments and strings").Disabled(d.busy)
			}
			if d.kind == "move" {
				ui.Checkbox(c, &d.openInEditor, "Open moved files in editor").Disabled(d.busy)
			}
			if d.err != "" {
				ui.Text(c, d.err).FontSize(12).TextColor(p.Danger).SingleLine().Ellipsis("…")
			}
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				if d.busy {
					busyText := "Looking for references…"
					if d.kind == "copy" {
						busyText = "Copying…"
					}
					ui.Text(c, busyText).FontSize(12).TextColor(p.TextMuted).SingleLine()
				}
				ui.Spacer(c)
				cancel := ui.Button(c, "Cancel")
				cancel.OnClick(func() { m.closeFileRefactorDialog(true) })
				if button := ui.PrimaryButton(c, action); true {
					if d.busy {
						button.Disabled(true)
					}
					button.OnClick(m.ConfirmFileRefactor)
				}
			})
		})
	})
	if modal.Dismissed() || !open {
		m.closeFileRefactorDialog(true)
	}
}

// fileRefactorPreviewDialog draws OpenIDEA's Refactoring Preview with
// per-file inclusion and before/after occurrences.
func (m *Model) fileRefactorPreviewDialog(c *ui.Context, p *ui.Theme) {
	v := m.fileRefactorPreview
	if v == nil {
		return
	}
	open := true
	modal := ui.Modal(c.Key("project-refactoring-preview"), &open, func() {
		ui.Column(c).Width(700).Gap(10).Children(func() {
			ui.Text(c, "Refactoring Preview: "+v.title).FontSize(14).Bold().
				TextColor(p.Text).SingleLine().Ellipsis("…")
			included, occurrences := 0, 0
			for _, file := range v.files {
				if file.Enabled {
					included++
					occurrences += len(file.Edits)
				}
			}
			ui.Text(c, fmt.Sprintf("References to update: %d occurrence%s in %d file%s",
				occurrences, pluralSuffix(occurrences), included, pluralSuffix(included))).
				FontSize(12).TextColor(p.TextMuted).SingleLine()
			ui.Scroll(c).MaxHeight(440).Children(func() {
				ui.Column(c).Gap(8).Children(func() {
					for index := range v.files {
						file := &v.files[index]
						ui.Column(c.Key("refactor-preview:" + file.Path)).Gap(3).Children(func() {
							ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
								ui.Checkbox(c, &file.Enabled, "").Disabled(v.busy)
								ui.Text(c, file.Name).FontSize(12).Bold().TextColor(p.Text).SingleLine()
								ui.Text(c, file.Relative).FontSize(11).TextColor(p.TextMuted).
									SingleLine().Ellipsis("…").Grow(1)
								ui.Text(c, fmt.Sprintf("%d occurrence%s", len(file.Edits),
									pluralSuffix(len(file.Edits)))).FontSize(11).
									TextColor(p.TextMuted).SingleLine()
							})
							for editIndex, edit := range file.Edits {
								editIndex, edit := editIndex, edit
								ui.Row(c.Key(fmt.Sprintf("refactor-occurrence:%s:%d", file.Path, editIndex))).
									Height(24).PaddingStart(24).Gap(6).AlignItems(ui.Center).Children(func() {
									ui.Text(c, fmt.Sprintf("%d", edit.Line)).FontSize(11).
										Font("monospace").TextColor(p.TextMuted)
									ui.Text(c, edit.Pre).FontSize(12).Font("monospace").TextColor(p.Text)
									if edit.Old != "" {
										ui.Text(c, edit.Old).FontSize(12).Font("monospace").
											TextColor(p.Danger).Background(p.Background.Mix(p.Danger, 0.14))
									}
									if edit.New != "" {
										ui.Text(c, edit.New).FontSize(12).Font("monospace").
											TextColor(p.Success).Background(p.Background.Mix(p.Success, 0.14))
									}
									ui.Text(c, edit.Post).FontSize(12).Font("monospace").
										TextColor(p.Text).SingleLine().Ellipsis("…")
								})
							}
						})
					}
				})
			})
			if v.err != "" {
				ui.Text(c, v.err).FontSize(12).TextColor(p.Danger).SingleLine().Ellipsis("…")
			}
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				if v.busy {
					ui.Text(c, "Refactoring…").FontSize(12).TextColor(p.TextMuted).SingleLine()
				}
				ui.Spacer(c)
				ui.Button(c, "Do Refactor").Disabled(v.busy).OnClick(m.ConfirmFileRefactorPreview)
				ui.Button(c, "Cancel").OnClick(func() { m.closeFileRefactorPreview(true) })
			})
		})
	})
	if modal.Dismissed() || !open {
		m.closeFileRefactorPreview(true)
	}
}

func pluralSuffix(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}
