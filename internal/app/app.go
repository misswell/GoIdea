// Package app is GoIDEA's window: the project tree, tabs of documents that
// share their buffers with openidea-server, and the MyGo editor over those
// buffers.
//
// The model lives on the main thread. Background work (backend calls, disk
// reads, dialogs) hands its results back with mygo.RunOnMain, so the view and
// every field here are touched from one goroutine.
package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
)

// Window is the backend window string GoIDEA opens its documents in.
const Window = "goidea"

// Config wires the model to the backend and to the user's data directory.
type Config struct {
	ServerPath   string
	Home         string
	SessionPath  string
	SettingsPath string
	Log          *os.File
}

// Tab is one open file: its backend document, the editor state editing the
// document's buffer, and how much of that buffer the backend has.
type Tab struct {
	doc     *docbridge.Document
	state   *editor.State
	synced  uint64 // buffer version the backend was last given
	seen    uint64 // buffer version last looked at, for the sync debounce
	edited  time.Time
	syncing bool
	saving  bool

	// formatting marks a reformat request in flight, so a second one does not
	// apply the first one's edits twice.
	formatting bool

	// diskTime and diskSize are the file when the buffer last matched it, so
	// a write by another program is noticed; reloading marks one in flight.
	diskTime  time.Time
	diskSize  int64
	reloading bool

	// gitMarks are the gutter marks of the buffer version gitVersion, read
	// against HEAD; gitOff stays set when Git cannot answer for this file.
	gitMarks   []editor.GutterMark
	gitVersion uint64
	gitPath    string
	gitLoading bool
	gitOff     bool

	// outline is the document's symbols for breadcrumbs; outlineGen drops
	// replies that a newer request has superseded.
	outline        []openidea.OutlineItem
	outlineDoc     uint32
	outlineVersion uint64
	outlineGen     uint64
	outlineLoading bool

	// compReq is the completion request in flight, compGen and hoverGen drop
	// replies a newer request superseded, and compOff is when completion asks
	// again after the language server was missing.
	compReq  uint32
	compGen  uint64
	compOff  time.Time
	hoverGen uint64
	defGen   uint64
}

// Model is the app's state.
type Model struct {
	cfg Config
	win *mygo.Window
	mgr *openidea.Manager
	reg *docbridge.Registry
	ctx context.Context
	off context.CancelFunc

	rootMu sync.RWMutex
	root   string

	tabs   []*Tab
	active int
	tree   *Tree

	// diags is the newest diagnostics per file path and producer, which the
	// tabs of that file and the Problems tool window show.
	diags map[string]map[string][]openidea.Diagnostic

	// rename is the open rename prompt, or nil.
	rename *renameState

	// symbols is the open Go to Symbol picker, or nil.
	symbols *symbolPicker

	// files is the open Quick Open picker, or nil.
	files *filePicker

	// palette is the open command palette, or nil.
	palette *commandPicker

	// diff is the open compare-with-HEAD view, or nil.
	diff *diffView

	// db is the open database sheet, or nil.
	db *dbView

	// intents is the open intentions prompt, or nil.
	intents *intentPane

	// prefs are the user's stored settings, and theme is the theme they pick,
	// nil to follow the system; settingsOpen shows the settings sheet.
	prefs        Preferences
	theme        *ui.Theme
	settingsOpen bool

	// javaTools is the Java language server / debugger state the settings
	// sheet shows; toolsBusy names the install in progress, whose events
	// arrive on toolsChannel.
	javaTools     []javaTool
	toolsLoaded   bool
	toolsLoading  bool
	toolsBusy     string
	toolsChannel  uint32
	toolsRequest  uint32
	toolsProgress string
	toolsLog      []string
	toolsErr      string
	toolsAbort    atomic.Bool

	// refactor is the newest backend workspace edit, which UndoRefactor rolls
	// back; redoRefactor is what the last undo can reapply.
	refactor     *refactorEdit
	redoRefactor *refactorEdit

	// sideMode picks what the sidebar shows: "tree" ("" also means tree, the
	// default), "vcs", "search", "usages", "structure" or "bookmarks".
	sideMode string

	// commit is the open Commit dialog, or nil.
	commit *commitState

	// rollback is the open confirmation for discarding a change, or nil.
	rollback *rollbackState

	// bookmarks are the marked lines of the open project, pathBookmarks its
	// bookmarked files, and bookmarkLists the named groups they show in.
	// bookmarksView is the Bookmarks popup, mnemonics the Alt+F3 chooser,
	// newList the new-or-rename list dialog, deleteList the confirmation
	// that drops a list, move the Move to List chooser, and editDesc the
	// description dialog, or nil.
	bookmarks     []Bookmark
	pathBookmarks []PathBookmark
	bookmarkLists []BookmarkList
	defaultList   string
	bookmarksView *bookmarksPane
	mnemonics     *mnemonicPicker
	newList       *newListPane
	deleteList    *deleteListPane
	move          *movePane
	editDesc      *descPane

	run       RunPane
	debug     DebugPane
	build     BuildPane
	term      TermPane
	vcs       VCSPane
	branch    BranchPane
	log       LogPane
	problems  ProblemsPane
	search    SearchPane
	usages    UsagesPane
	structure StructurePane

	// tool is the bottom tool window the shortcuts last opened, "" once it
	// closed; toolStash is what Hide All hid, restored by the next press.
	// bottomHeight is the slot's one size, as OpenIDEA keeps a size per
	// anchor; maximized lets the slot take the workbench and hides the editor.
	tool         string
	toolStash    map[string]bool
	toolStashMax bool
	// splitNames are the windows the bottom split area holds; the others sit in
	// the primary area, as OpenIDEA places a window in one of a side's two areas.
	splitNames   map[string]bool
	bottomHeight int
	maximized    bool
	// frameHeight is the last drawn window height, which caps a resize.
	frameHeight float32
	// problemStep is the row Cmd+Alt+Down last revealed.
	problemStep int
	// reveal is the caret position a search hit asked for; it applies once the
	// file is the active tab.
	reveal  *revealPos
	chanSeq atomic.Uint32
	reqSeq  atomic.Uint32

	busy       bool
	status     string
	banner     string
	bannerPath string

	// watched paces the disk check of open files in syncPump.
	watched time.Time

	quitting  bool
	forceQuit bool

	// startupRoot opens this project instead of the session's, for `goidea
	// <dir>` and the offscreen smoke run.
	startupRoot string
	// startupFiles opens these files beside startupRoot, so an offscreen run
	// has a tab without touching the user's session.
	startupFiles []string
}

// New returns a model; Start opens the last session.
func New(cfg Config) *Model {
	ctx, off := context.WithCancel(context.Background())
	m := &Model{cfg: cfg, ctx: ctx, off: off, bottomHeight: bottomDefaultHeight}
	m.prefs = LoadPreferences(cfg.SettingsPath)
	m.applyPreferences()
	m.mgr = openidea.NewManager(openidea.LaunchConfig{
		ServerPath: cfg.ServerPath,
		Home:       cfg.Home,
		Log:        cfg.Log,
	})
	m.reg = docbridge.NewRegistry(openidea.Docs{M: m.mgr, Window: Window}, Window)
	m.reg.SetErrorHandler(func(path string, err error) {
		m.post(func() { m.fail(path, err) })
	})
	m.reg.SetDiagnosticsHandler(func(ev openidea.DiagnosticsEvent) {
		m.post(func() { m.applyDiagnostics(ev) })
	})
	// Backend edits (formatting, refactors, reloads) reach the shared
	// buffers through the registry, so the editor shows them without a
	// resync.
	go m.reg.Run(ctx, m.mgr.Notifications())
	go m.watchBackend(ctx)
	m.mgr.OnRestart(func(ctx context.Context) error {
		root := m.Root()
		if root == "" {
			return nil
		}
		if err := m.openBackendProject(ctx, root); err != nil {
			return err
		}
		if err := m.reg.ReopenAll(ctx); err != nil {
			return err
		}
		m.post(func() {
			m.run.running = false
			m.run.awaiting = false
			m.run.busy = false
			m.run.exitCode = nil
			if m.run.visible {
				m.run.err = "后端已重启，运行已中断"
			}
			m.debug.starting = false
			m.debug.paused = false
			m.debug.session = 0
			if m.debug.visible {
				m.debug.err = "后端已重启，调试已中断"
			}
			for i := range m.term.sessions {
				m.term.sessions[i].id = 0
				m.term.sessions[i].pid = nil
				m.term.sessions[i].starting = false
				m.term.sessions[i].ended = true
			}
			if m.term.visible {
				m.term.err = "后端已重启，终端已关闭"
			}
			if m.run.trusted {
				m.loadRunConfigs()
			}
			m.invalidate()
		})
		return nil
	})
	go m.watchState()
	return m
}

// Attach gives the model its window and registers the close prompt.
func (m *Model) Attach(win *mygo.Window) {
	m.win = win
	win.OnClose(func(e *mygo.CloseEvent) { m.requestQuit(e.PreventDefault) })
}

// OpenAtStartup opens dir instead of the session's project; call before Start.
func (m *Model) OpenAtStartup(dir string) { m.startupRoot = dir }

// OpenFilesAtStartup opens these files in the startup project; call before
// Start. Paths outside the project are ignored, as session files are.
func (m *Model) OpenFilesAtStartup(paths []string) { m.startupFiles = paths }

// Snapshot reports what the smoke run checks: the project, its tabs, and
// whether the backend is up.
func (m *Model) Snapshot() (root string, tabs int, backend bool) {
	return m.Root(), len(m.tabs), m.BackendState().Running
}

// Start restores the last session in the background.
func (m *Model) Start() {
	m.post(func() { m.busy = true; m.status = "正在连接 openidea-server…" })
	go m.start()
	go m.syncLoop()
}

// syncLoop pushes buffer changes on its own clock, so it works while the
// window is hidden or minimized and the view is not building frames.
func (m *Model) syncLoop() {
	tick := time.NewTicker(120 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case now := <-tick.C:
			m.post(func() { m.syncPump(now) })
		}
	}
}

func (m *Model) start() {
	root := m.startupRoot
	var files []string
	if root == "" {
		s := m.session()
		root = s.Root
		if root != "" {
			for _, p := range s.Files {
				if inProject(root, p) {
					files = append(files, p)
				}
			}
		}
	} else {
		for _, p := range m.startupFiles {
			if inProject(root, p) {
				files = append(files, p)
			}
		}
	}
	if root == "" {
		m.post(func() { m.busy = false; m.status = "请打开一个 Java 项目" })
		return
	}
	m.setRoot(root)
	m.openProject(root, files)
}

func (m *Model) session() Session {
	return LoadSession(m.cfg.SessionPath)
}

// inProject reports whether p lies inside root, tolerating a path that differs
// only by trailing separators or symlinks (/var vs /private/var).
func inProject(root, p string) bool {
	root = filepath.Clean(root)
	p = filepath.Clean(p)
	if root == "" || root == "." || p == "" || p == "." {
		return false
	}
	roots := []string{root}
	if r := realPath(root); r != root {
		roots = append(roots, r)
	}
	paths := []string{p}
	if q := realPath(p); q != p {
		paths = append(paths, q)
	}
	for _, r := range roots {
		for _, q := range paths {
			if q != r && strings.HasPrefix(q, r+string(filepath.Separator)) {
				return true
			}
		}
	}
	return false
}

// Root is the open project folder, safe to read from any goroutine.
func (m *Model) Root() string {
	m.rootMu.RLock()
	defer m.rootMu.RUnlock()
	return m.root
}

func (m *Model) setRoot(root string) {
	m.rootMu.Lock()
	m.root = root
	m.rootMu.Unlock()
}

// post runs fn on the main thread and waits for it.
func (m *Model) post(fn func()) { mygo.RunOnMain(fn) }

// watchState follows the backend so the status bar says when it is down.
func (m *Model) watchState() {
	for {
		select {
		case <-m.ctx.Done():
			return
		case state := <-m.mgr.StatusChanges():
			m.post(func() {
				switch {
				case state.Err != nil:
					m.status = "后端不可用：" + state.Err.Error()
					m.run.running = false
					m.run.awaiting = false
					m.run.busy = false
					m.debug.starting = false
					m.debug.paused = false
					m.debug.session = 0
				case state.Running:
					m.status = "已连接 openidea-server"
				}
				m.invalidate()
			})
		}
	}
}

func (m *Model) invalidate() {
	if m.win != nil {
		m.win.Invalidate()
	}
}

func (m *Model) fail(path string, err error) {
	if err == nil {
		return
	}
	m.banner = err.Error()
	m.bannerPath = path
	m.invalidate()
}

// ChooseProject asks for a folder and opens it.
func (m *Model) ChooseProject() { m.chooseProject() }

// Notify shows a message in the banner, as a missing backend binary.
func (m *Model) Notify(msg string) {
	m.post(func() {
		m.banner = msg
		m.bannerPath = ""
		m.invalidate()
	})
}

// chooseProject asks for a folder and opens it.
func (m *Model) chooseProject() {
	parent := m.win
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: parent, Title: "打开项目", ButtonLabel: "打开",
			Directory: true, CreateDirectories: true,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		dir := paths[0]
		m.post(func() { m.confirmSwitch(dir) })
	}()
}

// confirmSwitch asks about unsaved changes before replacing the project.
func (m *Model) confirmSwitch(dir string) {
	if m.busy {
		return
	}
	if dir == m.Root() {
		return
	}
	if !m.hasDirty() {
		m.openProject(dir, nil)
		return
	}
	detail := strings.Join(m.dirtyPaths(), "\n")
	go func() {
		res, err := mygo.Dialog.Message(mygo.MessageOptions{
			Parent: m.win, Type: mygo.MessageWarning, Title: "切换项目",
			Message: "有未保存的更改", Detail: detail,
			Buttons: []string{"保存并切换", "放弃更改", "取消"}, DefaultButton: 0, CancelButton: 2,
		})
		if err != nil {
			return
		}
		m.post(func() {
			switch res.Button {
			case 0:
				m.openProjectSaving(dir)
			case 1:
				m.openProject(dir, nil)
			}
		})
	}()
}

// OpenProject opens dir now, as the menu's recent entries do.
func (m *Model) OpenProject(dir string) { m.confirmSwitch(dir) }

func (m *Model) openProjectSaving(dir string) {
	tabs := append([]*Tab(nil), m.tabs...)
	m.busy = true
	m.status = "正在保存…"
	m.invalidate()
	go func() {
		if err := saveAll(m.ctx, tabs); err != nil {
			m.post(func() { m.fail("", err); m.busy = false })
			return
		}
		m.post(func() { m.openProject(dir, nil) })
	}()
}

// openProject switches the backend project and restores files, all in the
// background.
func (m *Model) openProject(dir string, files []string) {
	// Language servers resolve symlinks, so a project opened through one
	// (/tmp on macOS) would give the backend paths that miss the diagnostics
	// published under the real path — and code actions lose their context.
	if resolved := realPath(dir); resolved != dir {
		for i, p := range files {
			if rel, err := filepath.Rel(dir, p); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				files[i] = filepath.Join(resolved, rel)
			} else {
				files[i] = realPath(p)
			}
		}
		dir = resolved
	}
	old := append([]*Tab(nil), m.tabs...)
	oldRun := m.run.runID
	oldDebug := m.debug.session
	var oldTerms []uint32
	for i := range m.term.sessions {
		if id := m.term.sessions[i].id; id != 0 {
			oldTerms = append(oldTerms, id)
		}
	}
	m.tabs, m.active = nil, 0
	m.banner, m.bannerPath = "", ""
	m.resetRun()
	m.resetDebug()
	m.resetBuild()
	m.resetTerm()
	m.resetVCS()
	m.resetBranch()
	m.rollback = nil
	m.bookmarksView = nil
	m.mnemonics = nil
	m.bookmarks = bookmarksInProject(m.session().Bookmarks, dir)
	m.pathBookmarks = pathBookmarksInProject(m.session().PathBookmarks, dir)
	m.bookmarkLists = normalizeBookmarkLists(m.session().BookmarkLists)
	m.defaultList = m.session().DefaultList
	m.defaultList = m.defaultBookmarkList()
	m.newList = nil
	m.deleteList = nil
	m.move = nil
	m.editDesc = nil
	m.resetLog()
	m.resetProblems()
	m.structure = StructurePane{}
	m.tool = ""
	m.toolStash = nil
	m.toolStashMax = false
	m.splitNames = nil
	m.maximized = false
	m.commit = nil
	m.cancelSearch()
	m.resetSearch()
	m.resetUsages()
	m.closeSymbols()
	m.closeIntents()
	m.closeQuickFiles()
	m.closeCommands()
	m.diff = nil
	m.db = nil
	m.refactor, m.redoRefactor = nil, nil
	m.busy = true
	m.status = "正在打开 " + filepath.Base(dir) + "…"
	m.setRoot(dir)
	m.invalidate()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		if oldRun != 0 {
			_, _ = m.mgr.Invoke(ctx, Window, "run_stop", map[string]any{"id": oldRun})
		}
		if oldDebug != 0 {
			_, _ = m.mgr.Invoke(ctx, Window, "debug_stop", map[string]any{"session": oldDebug})
		}
		for _, id := range oldTerms {
			_, _ = m.mgr.Invoke(ctx, Window, "term_kill", map[string]any{"id": id})
		}
		for _, t := range old {
			_ = m.reg.Close(ctx, t.doc.Path())
		}
		if err := m.openBackendProject(ctx, dir); err != nil {
			m.post(func() { m.busy = false; m.fail("", fmt.Errorf("打开项目失败：%w", err)) })
			return
		}
		var docs []*docbridge.Document
		var failed []string
		for _, p := range files {
			doc, err := m.reg.Open(ctx, p)
			if err != nil {
				failed = append(failed, filepath.Base(p))
				continue
			}
			docs = append(docs, doc)
		}
		focus := m.session().Focus
		if focus != "" {
			focus = realPath(focus)
		}
		m.post(func() {
			m.clearDiagnostics()
			m.rename = nil
			m.tabs = m.tabs[:0]
			for _, doc := range docs {
				m.tabs = append(m.tabs, m.newTab(doc))
			}
			m.active = 0
			for i, t := range m.tabs {
				if t.doc.Path() == focus {
					m.active = i
				}
			}
			if m.tree != nil {
				m.tree.Close()
			}
			m.tree = NewTree(dir, m.post, m.invalidate)
			m.busy = false
			m.status = fmt.Sprintf("%s · %d 个文件已打开", filepath.Base(dir), len(m.tabs))
			if len(failed) > 0 {
				m.banner = "无法打开：" + strings.Join(failed, "、")
			}
			m.applyActive()
			m.saveSession()
			m.checkTrust(dir)
			m.RefreshVCS()
			m.RefreshBranch()
			m.invalidate()
		})
	}()
}

func (m *Model) openBackendProject(ctx context.Context, dir string) error {
	if _, err := m.mgr.Invoke(ctx, Window, "open_project", map[string]any{"path": dir}); err != nil {
		return err
	}
	deadline := time.Now().Add(60 * time.Second)
	for {
		raw, err := m.mgr.Invoke(ctx, Window, "project_current", nil)
		if err == nil && strings.TrimSpace(string(raw)) != "null" {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("openidea-server 未在 60 秒内就绪")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(60 * time.Millisecond):
		}
	}
}

func (m *Model) newTab(doc *docbridge.Document) *Tab {
	v := doc.Buffer().Version()
	path := doc.Path()
	t := &Tab{
		doc:    doc,
		state:  &editor.State{Buffer: doc.Buffer(), LineHeight: 1.45},
		synced: v,
		seen:   v,
	}
	t.state.Font = ui.Font{Size: m.prefs.fontSize()}
	t.state.OnGutterClick = func(line int) { m.toggleBreakpointAt(path, line) }
	t.state.OnCompletion = func(req editor.CompletionRequest) { m.requestCompletion(t, req) }
	t.state.OnHover = func(req editor.HoverRequest) { m.requestHover(t, req) }
	t.state.OnDefinition = func(off int) { m.gotoDefinition(t, off) }
	m.showDiagnostics(t)
	t.noteDisk()
	m.markRecent(path)
	return t
}

// openFile opens path in a tab, or shows the tab that has it.
func (m *Model) openFile(path string) {
	path = realPath(path)
	for i, t := range m.tabs {
		if t.doc.Path() == path {
			m.activate(i)
			return
		}
	}
	if m.busy || m.Root() == "" {
		return
	}
	m.busy = true
	m.status = "正在打开 " + filepath.Base(path) + "…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		doc, err := m.reg.Open(ctx, path)
		m.post(func() {
			m.busy = false
			if err != nil {
				m.fail(path, err)
				return
			}
			m.tabs = append(m.tabs, m.newTab(doc))
			m.active = len(m.tabs) - 1
			m.revealEditor()
			if m.tree != nil {
				m.tree.reveal(path)
			}
			m.applyActive()
			m.applyReveal(m.tabs[m.active])
			m.status = "已打开 " + filepath.Base(path)
			m.applyActive()
			m.saveSession()
			m.invalidate()
		})
	}()
}

func (m *Model) tab() *Tab {
	if m.active < 0 || m.active >= len(m.tabs) {
		return nil
	}
	return m.tabs[m.active]
}

func (m *Model) activate(i int) {
	if i < 0 || i >= len(m.tabs) || i == m.active {
		return
	}
	m.revealEditor()
	m.active = i
	m.applyActive()
	m.applyReveal(m.tabs[i])
	m.markRecent(m.tabs[i].doc.Path())
	m.saveSession()
	m.invalidate()
}

// applyActive tells the window which file it shows.
func (m *Model) applyActive() {
	if m.win == nil {
		return
	}
	if t := m.tab(); t != nil {
		name := filepath.Base(t.doc.Path())
		m.win.SetTitle(name + " — GoIDEA")
		m.win.SetRepresentedFile(t.doc.Path())
		m.win.SetDocumentEdited(t.dirty())
		return
	}
	m.win.SetTitle("GoIDEA")
	m.win.SetRepresentedFile("")
	m.win.SetDocumentEdited(false)
}

// closeTab closes a tab, asking about unsaved text first.
func (m *Model) closeTab(i int) {
	if i < 0 || i >= len(m.tabs) {
		return
	}
	t := m.tabs[i]
	if !t.unsaved() {
		m.dropTab(i)
		return
	}
	name := filepath.Base(t.doc.Path())
	go func() {
		res, err := mygo.Dialog.Message(mygo.MessageOptions{
			Parent: m.win, Type: mygo.MessageWarning, Title: "关闭文件",
			Message: "是否保存对 " + name + " 的更改？",
			Buttons: []string{"保存", "不保存", "取消"}, DefaultButton: 0, CancelButton: 2,
		})
		if err != nil {
			return
		}
		m.post(func() {
			switch res.Button {
			case 0:
				m.saveTabAsync(t, func() { m.removeTab(t) })
			case 1:
				m.removeTab(t)
			}
		})
	}()
}

func (m *Model) dropTab(i int) { m.removeTab(m.tabs[i]) }

func (m *Model) removeTab(t *Tab) {
	idx := -1
	for i, x := range m.tabs {
		if x == t {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}
	m.tabs = append(m.tabs[:idx], m.tabs[idx+1:]...)
	if m.rename != nil && m.rename.tab == t {
		m.rename = nil
	}
	if m.active >= len(m.tabs) {
		m.active = len(m.tabs) - 1
	}
	if m.active < 0 {
		m.active = 0
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = m.reg.Close(ctx, t.doc.Path())
	}()
	m.applyActive()
	m.saveSession()
	m.invalidate()
}

// SaveActive saves the active document.
func (m *Model) SaveActive() {
	if t := m.tab(); t != nil {
		m.saveTabAsync(t, nil)
	}
}

func (m *Model) saveTabAsync(t *Tab, done func()) {
	if t.saving {
		return
	}
	t.saving = true
	m.status = "正在保存 " + filepath.Base(t.doc.Path()) + "…"
	m.invalidate()
	go func() {
		// The version before the sync bounds what the save writes; edits
		// typed meanwhile stay unsaved and sync on the next tick.
		v := t.doc.Buffer().Version()
		err := t.doc.Sync(m.ctx)
		if err == nil {
			err = t.doc.Save(m.ctx)
		}
		m.post(func() {
			t.saving = false
			if err == nil {
				t.synced = v
				t.noteDisk()
				m.status = "已保存 " + filepath.Base(t.doc.Path())
			} else {
				m.fail(t.doc.Path(), err)
			}
			if done != nil && err == nil {
				done()
			}
			m.applyActive()
			m.invalidate()
			if m.vcs.loaded {
				m.RefreshVCS()
			}
		})
	}()
}

func saveAll(ctx context.Context, tabs []*Tab) error {
	var first error
	for _, t := range tabs {
		if !t.unsaved() {
			continue
		}
		if err := t.doc.Sync(ctx); err != nil {
			if first == nil {
				first = err
			}
			continue
		}
		if err := t.doc.Save(ctx); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// syncPump pushes editor buffer changes to the backend a moment after typing
// stops. It runs once per frame; the backend calls go to other goroutines.
func (m *Model) syncPump(now time.Time) {
	m.watchFiles(now)
	const idle = 220 * time.Millisecond
	for _, t := range m.tabs {
		v := t.doc.Buffer().Version()
		if v != t.seen {
			t.seen = v
			t.edited = now
		}
		m.kickCodeIntel(t, now)
		if t.syncing || t.saving || v == t.synced {
			continue
		}
		if now.Sub(t.edited) < idle {
			continue
		}
		t.syncing = true
		go func(t *Tab, v uint64) {
			err := t.doc.Sync(m.ctx)
			m.post(func() {
				t.syncing = false
				if err != nil {
					m.fail(t.doc.Path(), err)
					return
				}
				t.synced = v
				if t == m.tab() && m.win != nil {
					m.win.SetDocumentEdited(t.dirty())
				}
			})
		}(t, v)
	}
}

// dirty reports whether the buffer holds text the backend has not been given
// or has not saved.
func (t *Tab) dirty() bool {
	return t.syncing || t.saving || t.doc.Buffer().Version() != t.synced || t.doc.Dirty()
}

// unsaved reports whether the buffer's text differs from the file on disk, so
// undoing back to the saved text stops the quit prompt.
func (t *Tab) unsaved() bool { return t.doc.ModifiedFromSaved() }

func (m *Model) hasDirty() bool {
	for _, t := range m.tabs {
		if t.unsaved() {
			return true
		}
	}
	return false
}

func (m *Model) dirtyPaths() []string {
	var out []string
	for _, t := range m.tabs {
		if t.unsaved() {
			out = append(out, t.doc.Path())
		}
	}
	return out
}

// resolveActive settles a conflict for the active document.
func (m *Model) resolveActive(ours bool) {
	t := m.tab()
	if t == nil || m.bannerPath == "" {
		return
	}
	path := m.bannerPath
	m.banner, m.bannerPath = "", ""
	m.invalidate()
	go func() {
		doc := m.reg.Get(path)
		if doc == nil {
			return
		}
		var err error
		if ours {
			err = doc.Resolve(m.ctx, true)
		} else {
			err = doc.Resync(m.ctx)
		}
		m.post(func() {
			if err != nil {
				m.fail(path, err)
				return
			}
			t.synced = doc.Buffer().Version()
			m.status = "已解决冲突：" + filepath.Base(path)
			m.invalidate()
		})
	}()
}

// SmokeRoundTrip edits the active buffer the way the MyGo editor does, waits
// for the backend to see the text, then undoes it: the offscreen check that
// the editor buffer, the sync pump and openidea-server agree, UTF-16 aside
// and emoji included. It is used by the GOIDEA_SMOKE_EDIT run only.
func (m *Model) SmokeRoundTrip(ctx context.Context) (edited, undone bool) {
	var doc *docbridge.Document
	m.post(func() {
		if t := m.tab(); t != nil {
			doc = t.doc
		}
	})
	if doc == nil {
		return false, false
	}
	const marker = "// smoke 世界 😀\n"
	m.post(func() {
		doc.Buffer().Edit(editor.Edit{Start: 0, End: 0, Text: marker})
		m.invalidate()
	})
	docs := openidea.Docs{M: m.mgr, Window: Window}
	seen := func(want bool) bool {
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			snap, err := docs.DocSnapshot(ctx, Window, doc.ID())
			if err == nil && strings.Contains(snap.Text, marker) == want {
				return true
			}
			select {
			case <-ctx.Done():
				return false
			case <-time.After(100 * time.Millisecond):
			}
		}
		return false
	}
	edited = seen(true)
	m.post(func() {
		doc.Buffer().Undo()
		m.invalidate()
	})
	undone = seen(false)
	return edited, undone
}

// saveSession records the project and its tabs for the next start.
func (m *Model) saveSession() {
	root := m.Root()
	if root == "" && len(m.tabs) == 0 {
		// Nothing to remember; keep the previous session rather than
		// writing an empty one after a failed start.
		return
	}
	var files []string
	focus := ""
	for _, t := range m.tabs {
		files = append(files, t.doc.Path())
	}
	if t := m.tab(); t != nil {
		focus = t.doc.Path()
	}
	s := Session{Root: root, Files: normalizeFiles(files), Focus: focus,
		Bookmarks: m.bookmarks, PathBookmarks: m.pathBookmarks,
		BookmarkLists: m.bookmarkLists, DefaultList: m.defaultList}
	path := m.cfg.SessionPath
	go func() { _ = s.Save(path) }()
}

// RequestQuit asks about unsaved tabs and then closes the backend.
func (m *Model) RequestQuit(e *mygo.QuitEvent) { m.requestQuit(e.PreventDefault) }

func (m *Model) requestQuit(prevent func()) {
	if m.forceQuit {
		return
	}
	prevent()
	if m.quitting {
		return
	}
	m.quitting = true
	if !m.hasDirty() {
		m.finishQuit(false)
		return
	}
	detail := strings.Join(m.dirtyPaths(), "\n")
	go func() {
		res, err := mygo.Dialog.Message(mygo.MessageOptions{
			Parent: m.win, Type: mygo.MessageWarning, Title: "退出 GoIDEA",
			Message: "有未保存的更改，退出前是否保存？", Detail: detail,
			Buttons: []string{"保存并退出", "不保存", "取消"}, DefaultButton: 0, CancelButton: 2,
		})
		if err != nil {
			m.post(func() { m.quitting = false })
			return
		}
		m.post(func() {
			m.quitting = false
			switch res.Button {
			case 0:
				m.finishQuit(true)
			case 1:
				m.finishQuit(false)
			}
		})
	}()
}

func (m *Model) finishQuit(save bool) {
	m.saveSession()
	if m.tree != nil {
		m.tree.Close()
		m.tree = nil
	}
	tabs := append([]*Tab(nil), m.tabs...)
	m.forceQuit = true
	go func() {
		if save {
			_ = saveAll(m.ctx, tabs)
		}
		m.off()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		m.reg.CloseAll(ctx)
		_ = m.mgr.Close()
		m.post(func() { mygo.App.Quit() })
	}()
}

// Quit is the menu's action.
func (m *Model) Quit() { mygo.App.Quit() }

// ShowProject puts the sidebar back on the project tree.
func (m *Model) ShowProject() {
	m.sideMode = ""
	m.invalidate()
}

// overlayOpen reports whether a sheet owns the keyboard, so a shortcut that
// would also act in the background leaves it alone.
func (m *Model) overlayOpen() bool {
	return m.rename != nil || m.symbols != nil || m.intents != nil || m.files != nil ||
		m.diff != nil || m.palette != nil || m.commit != nil || m.branch.open ||
		m.rollback != nil || m.bookmarksView != nil || m.mnemonics != nil || m.newList != nil ||
		m.deleteList != nil || m.move != nil || m.editDesc != nil
}

// QuitImmediately closes the backend and quits without asking about changes,
// for the offscreen smoke run. Call it on the main thread.
func (m *Model) QuitImmediately() { m.finishQuit(false) }

// BackendState reports the server for the status bar.
func (m *Model) BackendState() openidea.State { return m.mgr.State() }

// View draws the window.
func (m *Model) View(c *ui.Context) {
	if m.theme != nil {
		c.SetTheme(m.theme)
	}
	if m.maximized && m.visibleTool() == "" {
		m.maximized = false
	}
	_, m.frameHeight = c.Size()
	m.shortcuts(c)
	p := c.Theme()
	ui.Column(c).Fill().Background(p.Background).Children(func() {
		m.toolbar(c, p)
		ui.Row(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			m.sidebar(c, p)
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				if m.maximized {
					// A maximized tool window takes the workbench, as
					// OpenIDEA hides the editor island over it.
					m.bottomSlot(c, p)
					return
				}
				m.tabStrip(c, p)
				if m.banner != "" {
					m.bannerRow(c, p)
				}
				m.editorArea(c, p)
				m.bottomSlot(c, p)
			})
		})
		m.statusBar(c, p)
	})
	m.renameDialog(c, p)
	m.symbolsDialog(c, p)
	m.intentionsDialog(c, p)
	m.filesDialog(c, p)
	m.branchesDialog(c, p)
	m.rollbackDialog(c, p)
	m.bookmarksDialog(c, p)
	m.mnemonicDialog(c, p)
	m.newListDialog(c, p)
	m.deleteListDialog(c, p)
	m.moveBookmarkDialog(c, p)
	m.bookmarkDescriptionDialog(c, p)
	m.commandsDialog(c, p)
	m.diffDialog(c, p)
	m.dbDialog(c, p)
	m.commitDialog(c, p)
	m.settingsDialog(c, p)
}

func (m *Model) shortcuts(c *ui.Context) {
	// A modal sheet owns the keyboard; only its own dismiss keys apply.
	if m.settingsOpen || (m.db != nil && m.db.open) {
		return
	}
	// The Bookmarks popup steps its rows with the arrow keys.
	if m.bookmarksView != nil {
		if c.Shortcut(ui.Modifiers(0), ui.KeyDown) {
			m.stepBookmarkSel(1)
		}
		if c.Shortcut(ui.Modifiers(0), ui.KeyUp) {
			m.stepBookmarkSel(-1)
		}
		if c.Shortcut(ui.Modifiers(0), ui.KeyEnter) {
			m.openSelectedBookmark()
		}
		if c.Shortcut(ui.Modifiers(0), ui.KeyF2) {
			m.OpenBookmarkDescription()
		}
		return
	}
	// The mnemonic chooser and the list dialogs take clicks and Escape only.
	if m.mnemonics != nil || m.newList != nil || m.deleteList != nil || m.move != nil || m.editDesc != nil {
		return
	}
	if c.Shortcut(ui.Cmd, ui.KeyComma) {
		m.OpenSettings()
	}
	if c.Shortcut(ui.Alt, ui.KeyEnter) {
		// An open prompt keeps the keyboard; the caret's menu waits.
		if m.rename == nil && m.symbols == nil && m.intents == nil && m.files == nil {
			m.ShowIntentions()
		}
	}
	if c.Shortcut(ui.Cmd, ui.KeyP) {
		if m.rename == nil && m.symbols == nil && m.intents == nil && m.files == nil {
			m.OpenQuickFiles()
		}
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyP) {
		if m.rename == nil && m.symbols == nil && m.intents == nil && m.files == nil &&
			m.diff == nil && m.palette == nil {
			m.OpenCommands()
		}
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyD) {
		if m.rename == nil && m.symbols == nil && m.intents == nil && m.files == nil && m.diff == nil {
			if t := m.tab(); t != nil {
				m.ShowDiff(t.doc.Path())
			}
		}
	}
	if c.Shortcut(ui.Cmd, ui.KeyS) {
		m.SaveActive()
	}
	if c.Shortcut(ui.Cmd, ui.KeyK) {
		if m.rename == nil && m.symbols == nil && m.intents == nil && m.files == nil &&
			m.diff == nil && m.palette == nil {
			m.OpenCommit()
		}
	}
	if c.Shortcut(ui.Cmd, ui.KeyW) {
		m.closeTab(m.active)
	}
	if c.Shortcut(ui.Cmd, ui.KeyO) {
		m.chooseProject()
	}
	// OpenIDEA's IntelliJ keymap keeps Cmd+1..9 for the tool windows and
	// switches tabs with Ctrl+Right / Ctrl+Left (intellij-macos.ts
	// toolwindow.*, editor.nextTab).
	if c.Shortcut(ui.Cmd, ui.Key1) {
		m.ShowProject()
	}
	if c.Shortcut(ui.Cmd, ui.Key2) {
		if !m.overlayOpen() {
			m.ToggleBookmarksWindow()
		}
	}
	if c.Shortcut(ui.Cmd, ui.Key3) {
		m.ShowSearch()
	}
	if c.Shortcut(ui.Cmd, ui.Key4) {
		m.ShowRun()
	}
	if c.Shortcut(ui.Cmd, ui.Key5) {
		m.ShowDebug()
	}
	if c.Shortcut(ui.Cmd, ui.Key6) {
		if !m.overlayOpen() {
			m.ToggleProblems()
		}
	}
	if c.Shortcut(ui.Cmd, ui.Key7) {
		if !m.overlayOpen() {
			m.ToggleStructure()
		}
	}
	if c.Shortcut(ui.Cmd, ui.Key9) {
		m.ShowVCS()
	}
	if c.Shortcut(ui.Cmd, ui.Key0) {
		if !m.overlayOpen() {
			m.OpenCommit()
		}
	}
	if c.Shortcut(ui.Alt, ui.KeyF12) {
		m.ShowTerm()
	}
	// OpenIDEA's terminal.new; ⌘T is otherwise unbound here, so it opens a tab
	// from anywhere rather than only while the terminal has focus.
	if c.Shortcut(ui.Cmd, ui.KeyT) && !m.overlayOpen() {
		m.OpenTermTab()
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyF12) {
		m.HideAllToolWindows()
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyQuote) && !m.overlayOpen() {
		m.ToggleMaximizeToolWindow()
	}
	if c.Shortcut(ui.Ctrl|ui.Alt, ui.KeyUp) {
		m.ResizeBottomToolWindow("up")
	}
	if c.Shortcut(ui.Ctrl|ui.Alt, ui.KeyDown) {
		m.ResizeBottomToolWindow("down")
	}
	if c.Shortcut(ui.Ctrl|ui.Alt, ui.KeyLeft) {
		m.ResizeBottomToolWindow("left")
	}
	if c.Shortcut(ui.Ctrl|ui.Alt, ui.KeyRight) {
		m.ResizeBottomToolWindow("right")
	}
	if c.Shortcut(ui.Shift, ui.KeyEscape) && !m.overlayOpen() {
		m.HideActiveToolWindow()
	}
	if c.Shortcut(ui.Modifiers(0), ui.KeyF12) {
		m.JumpToLastToolWindow()
	}
	if c.Shortcut(ui.Cmd|ui.Alt, ui.KeyDown) {
		m.StepProblem(1)
	}
	if c.Shortcut(ui.Cmd|ui.Alt, ui.KeyUp) {
		m.StepProblem(-1)
	}
	if n := len(m.tabs); n > 0 {
		if c.Shortcut(ui.Ctrl, ui.KeyRight) || c.Shortcut(ui.Cmd|ui.Shift, ui.KeyBracketRight) {
			m.activate((m.active + 1) % n)
		}
		if c.Shortcut(ui.Ctrl, ui.KeyLeft) || c.Shortcut(ui.Cmd|ui.Shift, ui.KeyBracketLeft) {
			m.activate((m.active - 1 + n) % n)
		}
	}
	if c.Shortcut(ui.Modifiers(0), ui.KeyF5) {
		m.Run()
	}
	if c.Shortcut(ui.Shift, ui.KeyF5) {
		m.StopRun()
		m.StopDebug()
	}
	if c.Shortcut(ui.Modifiers(0), ui.KeyF9) {
		m.Debug()
	}
	if c.Shortcut(ui.Cmd, ui.KeyF9) {
		m.BuildProject()
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyF9) {
		m.BuildModule()
	}
	// OpenIDEA's IntelliJ keymap marks the caret line with F3 and opens the
	// Bookmarks popup with Cmd+F3.
	if c.Shortcut(ui.Modifiers(0), ui.KeyF3) && !m.overlayOpen() {
		m.ToggleBookmark()
	}
	if c.Shortcut(ui.Cmd, ui.KeyF3) && !m.overlayOpen() {
		m.ShowBookmarks()
	}
	if c.Shortcut(ui.Alt, ui.KeyF3) && !m.overlayOpen() {
		m.OpenBookmarkMnemonics()
	}
	m.bookmarkDigitShortcuts(c)
	if c.Shortcut(ui.Cmd, ui.KeyF8) {
		m.ToggleBreakpoint()
	}
	if c.Shortcut(ui.Modifiers(0), ui.KeyF8) {
		m.StepOver()
	}
	if c.Shortcut(ui.Modifiers(0), ui.KeyF7) {
		m.StepInto()
	}
	if c.Shortcut(ui.Shift, ui.KeyF8) {
		m.StepOut()
	}
}
