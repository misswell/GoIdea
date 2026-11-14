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
	doc    *docbridge.Document
	state  *editor.State
	synced uint64 // buffer version the backend was last given
	seen   uint64 // buffer version last looked at, for the sync debounce
	// todoSeen and todoDark are the buffer version and appearance the editor's
	// TODO tints were computed for, so a frame only rescans when one changed.
	todoSeen uint64
	todoDark bool
	edited   time.Time
	syncing  bool
	saving   bool

	// formatting marks a reformat request in flight, so a second one does not
	// apply the first one's edits twice.
	formatting bool

	// pinned keeps the tab at the left of the strip, as OpenIDEA pins it; the
	// bulk closes leave it, except Close All Tabs.
	pinned bool

	// preview marks the single tab a results-list click opens: italic in the
	// strip, reused in place by the next such click, promoted by an edit or a
	// pin, and closed when a regular tab opens.
	preview bool

	// used is when the tab was last activated, so the tab limit closes the
	// least recently used one.
	used uint64

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

	// split is the second editor window group, or nil while the editor is one
	// group. The focused group stays in tabs and active; the other lives in
	// split. splitDown stacks the two, splitFirst draws the split group left
	// or above, and focusQueued moves the keyboard to the split group on the
	// next frame after its editor took it. focusApply gives the keyboard to
	// the editor of the group that just became focused.
	split       *splitGroup
	splitDown   bool
	splitFirst  bool
	focusQueued bool
	focusApply  bool

	// closedTabs is the tab history Reopen Closed Tab pops, most recently
	// closed last as OpenIDEA's tabs store keeps it.
	closedTabs []closedTab

	// tabPrefs is Settings › Editor › Editor Tabs as the strip draws it, which
	// the settings preview stages; it follows prefs.Tabs on Apply. tabUse
	// counts activations for the tab limit's least-recently-used order.
	tabPrefs TabPrefs
	tabUse   uint64

	// tabsScroll keeps how far the strip scrolls sideways, tabsPinnedScroll the
	// pinned row of a two-row strip, tabsSideScroll how far a strip on the
	// editor's left or right scrolls down, and tabsShown is the active tab the
	// last strip drew, so a change reveals the new one.
	tabsScroll       ui.ScrollState
	tabsPinnedScroll ui.ScrollState
	tabsSideScroll   ui.ScrollState
	tabsShown        int

	// tabsMenuOpen is OpenIDEA's Show Hidden Tabs menu, opened by the chevron
	// that a strip shows only while its tabs run past its width.
	tabsMenuOpen bool

	// tabDrag is the tab the pointer has lifted out of the strip: the strip
	// leaves a slot where it would drop and a ghost of it follows the pointer.
	tabDrag tabDragState

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

	// se is Search Everywhere while it is open; recentActions is the commands
	// it lists on an empty query, newest first.
	se            *sePane
	recentActions []string

	// notify is the notification center: the balloons and their history.
	notify notifier

	// runAnything is the open Run Anything popup, or nil.
	runAnything *runAnythingState

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
	// settingsPage is the page the settings sheet shows, by its OpenIDEA id,
	// or "group:<name>" while a group folder is selected.
	settingsPage string
	// settingsDraft stages the dialog's edits, as OpenIDEA's settings store
	// does: Apply and OK write them to prefs, Cancel drops them. settingsQuery
	// filters the page tree, and the history stack takes Back and Forward.
	settingsDraft     settingsDraft
	settingsQuery     string
	settingsHistory   []string
	settingsHistoryAt int
	// settingsCollapsed are the tree nodes closed with their caret, and
	// settingsSide is the page tree's width after a drag of the split handle.
	settingsCollapsed map[string]bool
	settingsSide      float32
	// settingsErr is a failed write of the staged settings, shown in the
	// dialog's footer as OpenIDEA shows its save error.
	settingsErr string

	// zen is Distraction Free Mode, and zenHint the way-out hint entering it
	// puts above the editor, as OpenIDEA's balloon does.
	zen     bool
	zenHint bool

	// crumbs are Settings › Editor › Breadcrumbs as the editor draws them:
	// whether the chain shows and whether it sits below the text, so the
	// dialog can preview a staged choice.
	crumbs BreadcrumbPrefs

	// toolbarCustomize is the open Customize Main Toolbar dialog, and update
	// the Check for Updates dialog.
	toolbarCustomize bool
	// toolbarDraft is the dialog's working layout, saved on OK only; side and
	// sel are the chosen row, adding and addQuery the Add… picker.
	toolbarDraft    toolbarLayout
	toolbarSide     string
	toolbarSel      int
	toolbarAdding   bool
	toolbarAddQuery string
	update          updateState

	// invalidateDlg is File › Invalidate Caches… while it is open, and
	// restartPending relaunches the app once the quit flow finishes.
	invalidateDlg  *invalidatePane
	restartPending bool
	// scratch is the New Scratch File chooser while it is open.
	scratch *scratchChooser

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

	// sideMode picks what the left primary area shows: the project tree (""),
	// or "vcs" for the changes view.
	sideMode string
	// leftSplit is the window in the left side's split area, which OpenIDEA
	// shows below the primary one: "structure", "bookmarks", or "" for none.
	// One window at a time, as each area shows.
	leftSplit string
	// leftHidden hides the left primary area; its split area and the stripe
	// stay, as OpenIDEA hides one area without the other.
	leftHidden bool
	// leftRatio is the share of the left side's room the primary area takes
	// when both areas show, as OpenIDEA's splits.left; zero is the default.
	leftRatio float32
	// leftWidth is the left side's width in DIPs, as OpenIDEA's sizes.left;
	// zero is the default.
	leftWidth float32
	// leftWidthDragging, leftDragWidth and leftDragMax carry a drag of the
	// gap beside the side: where the pointer put the edge, and the widest it
	// may go with the editor's minimum kept. leftDragCollapse hides the side
	// when the pointer went past half its minimum.
	leftWidthDragging bool
	leftDragWidth     float32
	leftDragMax       float32
	leftDragCollapse  bool
	// leftDragging and leftDragTotal carry a drag of the split handle: that it
	// is going on, and the room the areas shared when it began.
	leftDragging  bool
	leftDragTotal float32
	// rightTool is the right area's window: "" for none or "hierarchy", as
	// OpenIDEA anchors the Hierarchy window on the right.
	rightTool string
	// rightHidden hides the right area without forgetting its window.
	rightHidden bool
	// rightStash is what Hide All left the right area showing, so a second
	// press brings it back as the bottom windows do.
	rightStash bool
	// rightWidth is the right area's width in DIPs, as OpenIDEA's sizes.right.
	rightWidth float32
	// rightWidthDragging and rightDragWidth carry a drag of the gap beside it.
	rightWidthDragging bool
	rightDragWidth     float32
	// hierarchy is the call/type hierarchy the right window shows.
	hierarchy hierarchyPane
	// showingWelcome puts the Welcome screen up while a project stays loaded,
	// as OpenIDEA's project menu leaves it.
	showingWelcome bool

	// plugins is Welcome › Plugins, made on first use, nil before.
	plugins *pluginPage

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

	// treeClip is what the project view's Copy or Cut holds, and treeOp its
	// open New, Rename or Delete prompt, or nil.
	treeClip *treeClipState
	treeOp   *treeOpState
	// history is the open Local History sheet, or nil.
	history *historyState
	// dirCompare is the open Compare Directories sheet, or nil.
	dirCompare *dirCompareState
	// annotate is the open Annotate sheet, or nil.
	annotate *annotateState

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
	todo      TodoPane
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
	// editorWidth is the width the editor island drew at last frame, which
	// caps a drag of the side beside it, and editorHeight the height it drew,
	// which caps a drag of the bottom area below it.
	editorWidth  float32
	editorHeight float32
	// bottomDragging, bottomDragSize and bottomDragMax carry a drag of the gap
	// above the bottom area: the height the pointer asks for, and the tallest
	// it may go with the editor's minimum height kept. bottomDragCollapse
	// hides every bottom window when the pointer went past half its minimum.
	bottomDragging     bool
	bottomDragSize     float32
	bottomDragMax      float32
	bottomDragCollapse bool
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

	// recents are the folders the Welcome screen lists, most recent first,
	// welcomeQuery filters them, and welcomePage is the rail page shown:
	// "" for projects, or "customize" or "plugins".
	recents      []string
	welcomeQuery string
	welcomePage  string

	// layouts are each project's saved workbench layout, by folder, as
	// OpenIDEA keeps one per project root; sessionSeq orders the session
	// writes so a slow one never overwrites a newer snapshot.
	layouts    map[string]Layout
	layoutHold bool
	// runHistory is each project's Run Anything history, by folder, as
	// OpenIDEA keeps it per project root.
	runHistory   map[string][]string
	sessionSeq   int
	sessionWrote int
	sessionMu    sync.Mutex
	sessionWrite sync.Mutex

	// The Welcome screen's per-folder and per-dialog state: the branch of
	// each recent project once read, the gear popup, and the clone and
	// remove dialogs.
	welcomeBranches      map[string]string
	welcomeBranchLoading map[string]bool
	welcomeGearOpen      bool
	welcomeCloneOpen     bool
	welcomeCloneURL      string
	welcomeCloneDir      string
	welcomeCloneErr      string
	welcomeCloneBusy     bool
	welcomeCloneSeq      int
	welcomeRemoveOpen    bool
	welcomeRemovePath    string
}

// New returns a model; Start opens the last session.
func New(cfg Config) *Model {
	ctx, off := context.WithCancel(context.Background())
	m := &Model{cfg: cfg, ctx: ctx, off: off, bottomHeight: bottomDefaultHeight}
	m.prefs = LoadPreferences(cfg.SettingsPath)
	m.applyPreferences()
	session := LoadSession(cfg.SessionPath)
	m.recents = normalizeRecents(session.RecentProjects)
	m.layouts = map[string]Layout{}
	for root, l := range session.Layouts {
		m.layouts[layoutKey(root)] = normalizeLayout(l)
	}
	m.runHistory = normalizeRunHistory(session.RunHistory)
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
			for gi := range m.term.groups {
				tabs := m.term.groups[gi].tabs
				for ti := range tabs {
					tabs[ti].id = 0
					tabs[ti].pid = nil
					tabs[ti].starting = false
					tabs[ti].ended = true
				}
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
	m.notifyError("fail:"+path, "Error", notifySuffix(err.Error()))
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
	p := m.prefs.SaveFiles
	go func() {
		if err := saveAll(m.ctx, tabs, p); err != nil {
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
	if prev := m.Root(); prev != "" && layoutKey(prev) != layoutKey(dir) {
		if m.layouts == nil {
			m.layouts = map[string]Layout{}
		}
		m.layouts[layoutKey(prev)] = m.currentLayout()
	}
	old := m.allTabs()
	pinned := map[string]bool{}
	for _, p := range m.session().Pinned {
		pinned[realPath(p)] = true
	}
	// The second window group comes back with its files, its active tab and
	// its side, as the first group's files come back.
	sess := m.session()
	var splitFiles []string
	for _, p := range sess.SplitFiles {
		p = realPath(p)
		if inProject(dir, p) {
			splitFiles = append(splitFiles, p)
		}
	}
	splitActive, splitDown, splitFirst := sess.SplitActive, sess.SplitDown, sess.SplitFirst
	oldRun := m.run.runID
	oldDebug := m.debug.session
	var oldTerms []uint32
	for gi := range m.term.groups {
		tabs := m.term.groups[gi].tabs
		for ti := range tabs {
			if id := tabs[ti].id; id != 0 {
				oldTerms = append(oldTerms, id)
			}
		}
	}
	m.tabs, m.active = nil, 0
	m.split, m.splitDown, m.splitFirst, m.focusQueued = nil, false, false, false
	m.banner, m.bannerPath = "", ""
	m.showingWelcome = false
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
	m.resetTodo()
	m.resetHierarchy()
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
		var splitDocs []*docbridge.Document
		for _, p := range splitFiles {
			doc, err := m.reg.Open(ctx, p)
			if err != nil {
				failed = append(failed, filepath.Base(p))
				continue
			}
			splitDocs = append(splitDocs, doc)
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
			// Pinned tabs come back left of the strip, as the saved order and
			// OpenIDEA's pinned group put them.
			if len(pinned) > 0 {
				ordered := make([]*Tab, 0, len(m.tabs))
				for _, t := range m.tabs {
					if t.pinned = pinned[t.doc.Path()]; t.pinned {
						ordered = append(ordered, t)
					}
				}
				for _, t := range m.tabs {
					if !t.pinned {
						ordered = append(ordered, t)
					}
				}
				m.tabs = ordered
			}
			m.active = 0
			for i, t := range m.tabs {
				if t.doc.Path() == focus {
					m.active = i
				}
			}
			// The second window group comes back with its tabs, side and
			// stacked or side-by-side orientation.
			if len(splitDocs) > 0 {
				s := &splitGroup{}
				for _, doc := range splitDocs {
					s.tabs = append(s.tabs, m.newTab(doc))
				}
				s.active = min(max(splitActive, 0), len(s.tabs)-1)
				m.split = s
				m.splitDown, m.splitFirst = splitDown, splitFirst
				if focus != "" && m.tabForPath(focus) < 0 {
					for i, t := range s.tabs {
						if t.doc.Path() == focus {
							m.focusGroup(groupSplit)
							m.active = i
							break
						}
					}
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
			m.applyLayout(dir)
			m.rememberProject(dir)
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
		doc: doc,
		state: &editor.State{
			Buffer:     doc.Buffer(),
			LineHeight: 1.45,
			Bindings:   intellijEditorBindings(),
		},
		synced: v,
		seen:   v,
	}
	t.state.Font = ui.Font{Size: m.prefs.fontSize()}
	m.applyTabZenGutter(t)
	m.noteTabUse(t)
	t.state.OnGutterClick = func(line int) { m.toggleBreakpointAt(path, line) }
	t.state.OnCompletion = func(req editor.CompletionRequest) { m.requestCompletion(t, req) }
	t.state.OnHover = func(req editor.HoverRequest) { m.requestHover(t, req) }
	t.state.OnDefinition = func(off int) { m.gotoDefinition(t, off) }
	t.state.ContextMenu = func(menu *ui.Menu) { m.editorMenu(menu, t) }
	m.showDiagnostics(t)
	t.noteDisk()
	m.markRecent(path)
	return t
}

// openFile opens path in a regular tab, or shows the tab that has it.
func (m *Model) openFile(path string) { m.openFileWith(path, false) }

// openPreview opens path in the preview tab, or shows the tab that has it:
// the preview turns regular, as an explicit open promotes it.
func (m *Model) openPreview(path string) { m.openFileWith(path, true) }

// openFileWith is openFile with the preview flag. A preview open reuses the
// strip's one preview tab, and a regular open closes it, as openTab does.
func (m *Model) openFileWith(path string, preview bool) {
	m.openWith(path, openSpec{preview: preview, at: -1})
}

// openSpec says where an opened document goes: preview marks the strip's one
// preview tab, at (other than -1) is where a reopened tab had its place, and
// pinned joins the pinned group.
// pinned joins the pinned group.
type openSpec struct {
	preview bool
	at      int
	pinned  bool
}

// openWith is openFileWith with the place the document goes in the strip. A
// regular open closes the preview tab, as openTab does, and an open that
// replaces it keeps the preview slot.
func (m *Model) openWith(path string, spec openSpec) {
	preview := spec.preview && m.tabPrefs.previews()
	path = realPath(path)
	if i := m.tabForPath(path); i >= 0 {
		if !preview {
			m.promoteTab(m.tabs[i])
		}
		m.activate(i)
		return
	}
	if m.split != nil {
		for i, t := range m.split.tabs {
			if t.doc.Path() == path {
				if !preview {
					m.promoteTab(t)
				}
				m.focusGroup(groupSplit)
				m.activateIn(groupFocused, i)
				return
			}
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
			tab := m.newTab(doc)
			tab.preview = preview
			tab.pinned = spec.pinned
			switch {
			case spec.at >= 0:
				m.dropPreviewTabs()
				m.insertTabAt(tab, spec.at)
			case preview && m.previewIndex() >= 0:
				m.replacePreviewTab(m.previewIndex(), tab)
			default:
				if !preview {
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
	m.activateIn(groupFocused, i)
}

// activateIn shows the tab at i in the window group g; g is the focused group
// by the time the replacement tab is applied.
func (m *Model) activateIn(g groupRef, i int) {
	if g == groupSplit {
		m.focusGroup(groupSplit)
	}
	if i < 0 || i >= len(m.tabs) || i == m.active {
		return
	}
	m.revealEditor()
	m.active = i
	m.noteTabUse(m.tabs[i])
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

func (m *Model) dropTab(i int) { m.removeTab(m.tabs[i]) }

// removeTab closes t and puts it in the closed history Reopen Closed Tab pops.
func (m *Model) removeTab(t *Tab) {
	if g, ok := m.groupOf(t); ok && t.doc != nil {
		m.rememberClosed(closedTab{path: t.doc.Path(), index: tabIndexIn(m.groupTabsAt(g), t), pinned: t.pinned})
	}
	m.removeTabNow(t)
}

// removeTabNow takes t out of its window group without the closed history, as
// the preview trim and the preview slot's reuse do.
func (m *Model) removeTabNow(t *Tab) {
	g, ok := m.groupOf(t)
	if !ok {
		return
	}
	m.removeFromGroup(g, t)
	if m.rename != nil && m.rename.tab == t {
		m.rename = nil
	}
	m.settleGroups()
	if t.doc != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_ = m.reg.Close(ctx, t.doc.Path())
		}()
	}
	if len(m.tabs) == 0 {
		m.active = 0
	}
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
	p := m.prefs.SaveFiles
	go func() {
		// Save Files cleans the document first, as the editor saves it.
		err := cleanForSave(m.ctx, t.doc, p)
		// The version before the sync bounds what the save writes; edits
		// typed meanwhile stay unsaved and sync on the next tick.
		v := t.doc.Buffer().Version()
		if err == nil {
			err = t.doc.Sync(m.ctx)
		}
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

func saveAll(ctx context.Context, tabs []*Tab, p SavePrefs) error {
	var first error
	for _, t := range tabs {
		if !t.unsaved() {
			continue
		}
		if err := cleanForSave(ctx, t.doc, p); err != nil {
			if first == nil {
				first = err
			}
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
			// An edit promotes the preview tab, as the controller does.
			m.promoteTab(t)
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

// sessionSnapshot copies the session on the main thread, so the goroutine that
// writes it never reads state the user is still changing.
func (m *Model) sessionSnapshot() (Session, bool) {
	root := m.Root()
	if root == "" && len(m.tabs) == 0 {
		// Nothing to remember; keep the previous session rather than
		// writing an empty one after a failed start.
		return Session{}, false
	}
	var files, pinned []string
	focus := ""
	for _, t := range m.tabs {
		files = append(files, t.doc.Path())
		if t.pinned {
			pinned = append(pinned, t.doc.Path())
		}
	}
	if t := m.tab(); t != nil {
		focus = t.doc.Path()
	}
	var splitFiles []string
	splitActive := 0
	if m.split != nil {
		for _, t := range m.split.tabs {
			splitFiles = append(splitFiles, t.doc.Path())
		}
		splitActive = m.split.active
	}
	layouts := make(map[string]Layout, len(m.layouts))
	for key, l := range m.layouts {
		layouts[key] = l.clone()
	}
	// The open project's layout is what the workbench shows right now, so a
	// change no action recorded still reaches the next start.
	if root != "" && !m.layoutHold {
		layouts[layoutKey(root)] = m.currentLayout()
	}
	keep := append([]string{root}, m.recents...)
	return Session{Root: root, Files: normalizeFiles(files), Focus: focus,
		Pinned:         pinned,
		SplitFiles:     normalizeFiles(splitFiles),
		SplitActive:    splitActive,
		SplitDown:      m.splitDown,
		SplitFirst:     m.splitFirst,
		Bookmarks:      append([]Bookmark(nil), m.bookmarks...),
		PathBookmarks:  append([]PathBookmark(nil), m.pathBookmarks...),
		BookmarkLists:  append([]BookmarkList(nil), m.bookmarkLists...),
		DefaultList:    m.defaultList,
		RecentProjects: append([]string(nil), m.recents...),
		Layouts:        pruneLayouts(layouts, keep),
		RunHistory:     pruneRunHistory(cloneRunHistory(m.runHistory), keep)}, true
}

// cloneRunHistory copies the histories, so a snapshot being written never
// shares slices with the lists Run Anything keeps changing.
func cloneRunHistory(history map[string][]string) map[string][]string {
	out := make(map[string][]string, len(history))
	for key, lines := range history {
		out[key] = append([]string(nil), lines...)
	}
	return out
}

// saveSession writes the session in the background. The sequence numbers keep
// a slow write from replacing a newer snapshot on disk.
func (m *Model) saveSession() {
	if m.cfg.SessionPath == "" {
		return
	}
	s, ok := m.sessionSnapshot()
	if !ok {
		return
	}
	m.sessionMu.Lock()
	m.sessionSeq++
	seq := m.sessionSeq
	m.sessionMu.Unlock()
	path := m.cfg.SessionPath
	go m.writeSession(seq, path, s)
}

// writeSession writes one snapshot unless a newer one already reached the
// disk, and the write lock serializes them so the newest seq writes last.
func (m *Model) writeSession(seq int, path string, s Session) {
	m.sessionWrite.Lock()
	defer m.sessionWrite.Unlock()
	m.sessionMu.Lock()
	superseded := seq <= m.sessionWrote
	m.sessionMu.Unlock()
	if superseded {
		return
	}
	if s.Save(path) != nil {
		return
	}
	m.sessionMu.Lock()
	if seq > m.sessionWrote {
		m.sessionWrote = seq
	}
	m.sessionMu.Unlock()
}

// saveSessionNow writes the session before the process exits, so the last
// layout change is not lost with a write still in flight.
func (m *Model) saveSessionNow() {
	if m.cfg.SessionPath == "" {
		return
	}
	s, ok := m.sessionSnapshot()
	if !ok {
		return
	}
	m.sessionMu.Lock()
	m.sessionSeq++
	seq := m.sessionSeq
	m.sessionMu.Unlock()
	m.writeSession(seq, m.cfg.SessionPath, s)
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
			m.post(func() {
				m.quitting = false
				m.restartPending = false
			})
			return
		}
		m.post(func() {
			m.quitting = false
			switch res.Button {
			case 0:
				m.finishQuit(true)
			case 1:
				m.finishQuit(false)
			default:
				m.restartPending = false
			}
		})
	}()
}

func (m *Model) finishQuit(save bool) {
	m.saveSessionNow()
	if m.tree != nil {
		m.tree.Close()
		m.tree = nil
	}
	tabs := append([]*Tab(nil), m.tabs...)
	p := m.prefs.SaveFiles
	m.forceQuit = true
	go func() {
		if save {
			_ = saveAll(m.ctx, tabs, p)
		}
		m.off()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		m.reg.CloseAll(ctx)
		_ = m.mgr.Close()
		m.post(func() {
			if m.restartPending {
				mygo.App.Relaunch()
				return
			}
			mygo.App.Quit()
		})
	}()
}

// Quit is the menu's action.
func (m *Model) Quit() { mygo.App.Quit() }

// ShowProject puts the sidebar back on the project tree.
func (m *Model) ShowProject() {
	m.sideMode = ""
	m.leftHidden = false
	m.rememberLayout()
	m.invalidate()
}

// overlayOpen reports whether a sheet owns the keyboard, so a shortcut that
// would also act in the background leaves it alone.
func (m *Model) overlayOpen() bool {
	return m.rename != nil || m.symbols != nil || m.intents != nil || m.files != nil ||
		m.diff != nil || m.palette != nil || m.runAnything != nil || m.commit != nil || m.branch.open ||
		m.rollback != nil || m.bookmarksView != nil || m.mnemonics != nil || m.newList != nil ||
		m.deleteList != nil || m.move != nil || m.editDesc != nil || m.treeOp != nil ||
		m.history != nil || m.dirCompare != nil || m.invalidateDlg != nil || m.scratch != nil ||
		m.se != nil
}

// QuitImmediately closes the backend and quits without asking about changes,
// for the offscreen smoke run. Call it on the main thread.
func (m *Model) QuitImmediately() { m.finishQuit(false) }

// BackendState reports the server for the status bar.
func (m *Model) BackendState() openidea.State { return m.mgr.State() }

// View draws the window.
func (m *Model) View(c *ui.Context) {
	// OpenIDEA's own palette, day or night, whatever the appearance: the
	// stored choice picks one, the system's appearance the shade.
	if m.theme != nil {
		c.SetTheme(m.theme)
	} else if base := c.Theme(); base != nil {
		c.SetTheme(openideaTheme(base.Dark))
	}
	if m.maximized && m.visibleTool() == "" {
		m.maximized = false
	}
	// A click in the split pane's editor asked for the keyboard: the groups
	// swap here, between frames, and the editor of the group that is now
	// focused takes it back while this frame builds.
	if m.focusQueued && m.split != nil {
		m.focusQueued = false
		m.focusGroup(groupSplit)
		m.focusApply = true
	}
	_, m.frameHeight = c.Size()
	m.shortcuts(c)
	p := c.Theme()
	m.refreshTodoHighlights(time.Now(), p)
	ui.Column(c).Fill().Background(p.Background).Children(func() {
		if m.Root() == "" || m.showingWelcome {
			// With no project open the Welcome screen owns the window, as
			// OpenIDEA's does.
			if m.banner != "" {
				m.bannerRow(c, p)
			}
			m.welcome(c, p)
			return
		}
		m.workbench(c, p)
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
	m.treeOpDialog(c, p)
	m.historyDialog(c, p)
	m.dirCompareDialog(c, p)
	m.commandsDialog(c, p)
	m.searchEverywhereDialog(c, p)
	m.runAnythingDialog(c, p)
	m.diffDialog(c, p)
	m.annotateDialog(c, p)
	m.dbDialog(c, p)
	m.commitDialog(c, p)
	m.settingsDialog(c, p)
	m.toolbarCustomizeDialog(c, p)
	m.updateDialog(c, p)
	m.invalidateCachesDialog(c, p)
	m.scratchChooserDialog(c, p)
}

func (m *Model) shortcuts(c *ui.Context) {
	// The scratch chooser steps its rows with the arrow keys.
	if m.scratch != nil {
		if c.Shortcut(ui.Modifiers(0), ui.KeyDown) {
			m.stepScratch(1)
		}
		if c.Shortcut(ui.Modifiers(0), ui.KeyUp) {
			m.stepScratch(-1)
		}
		if c.Shortcut(ui.Modifiers(0), ui.KeyEnter) {
			m.CreateScratchSelected()
		}
		return
	}
	// A modal sheet owns the keyboard; only its own dismiss keys apply.
	if m.settingsOpen || m.toolbarCustomize || m.update.Open || m.invalidateDlg != nil || (m.db != nil && m.db.open) {
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
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyN) {
		if !m.overlayOpen() {
			m.OpenScratchChooser()
		}
	}
	if c.Shortcut(ui.Ctrl|ui.Alt, ui.KeyH) {
		if !m.overlayOpen() {
			m.OpenCallHierarchy()
		}
	}
	if c.Shortcut(ui.Ctrl, ui.KeyH) {
		if !m.overlayOpen() {
			m.OpenTypeHierarchy()
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
	// OpenIDEA's IntelliJ keymap expands the project tree with Cmd+=, collapses
	// it with Cmd+-, reloads it from disk with Cmd+Alt+Y and selects the active
	// file in it with Alt+F1.
	if c.Shortcut(ui.Cmd, ui.KeyEqual) && !m.overlayOpen() {
		m.ExpandProjectAll()
	}
	if c.Shortcut(ui.Cmd, ui.KeyMinus) && !m.overlayOpen() {
		m.CollapseProjectAll()
	}
	if c.Shortcut(ui.Cmd|ui.Alt, ui.KeyY) && !m.overlayOpen() {
		m.ReloadSelectedFromDisk()
	}
	if c.Shortcut(ui.Alt, ui.KeyF1) && !m.overlayOpen() {
		m.SelectInProjectView()
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
	// OpenIDEA's keymap splits the editor right with Cmd+\ and moves between
	// the window groups with Alt+Tab / Alt+Shift+Tab (editor.splitRight,
	// editor.nextSplitter, editor.prevSplitter).
	if c.Shortcut(ui.Cmd, ui.KeyBackslash) && !m.overlayOpen() {
		m.SplitEditorRight()
	}
	if c.Shortcut(ui.Alt, ui.KeyTab) && !m.overlayOpen() {
		m.GoToNextSplitter()
	}
	if c.Shortcut(ui.Alt|ui.Shift, ui.KeyTab) && !m.overlayOpen() {
		m.GoToPreviousSplitter()
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
