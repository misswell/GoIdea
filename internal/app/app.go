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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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
	ServerPath             string
	Home                   string
	SessionPath            string
	SettingsPath           string
	DataDir                string
	DefaultDataDir         string
	LocalDataDirConfigPath string
	Log                    *os.File
	// ServerMinVersion and ServerSHA256 pin the backend the client was
	// tested against; the preflight refuses an older or different binary.
	ServerMinVersion string
	ServerSHA256     string
	// FreshSession loads shared Welcome state from SessionPath but does not
	// reopen its project. Secondary native windows use it so a New Window
	// starts at Welcome while recents, layouts and bookmarks remain available.
	FreshSession bool
}

// startupChoice is the pure decision behind Start. It names the project root
// and the files reopened with it; looseFiles are the explicit paths a launch
// must open as LightEdit instead of restoring a project.
type startupChoice struct {
	root       string
	files      []string
	looseFiles []string
	remote     *openidea.RemoteSession
}

func chooseStartup(
	explicitRoot string,
	explicitFiles []string,
	session Session,
	reopen bool,
	freshSession bool,
) startupChoice {
	explicitFiles = explicitLaunchPaths(explicitFiles)
	root := strings.TrimSpace(explicitRoot)
	if root != "" && filepath.IsAbs(root) {
		root = realPath(filepath.Clean(root))
	} else {
		root = ""
	}
	if root == "." {
		root = ""
	}
	if root != "" {
		return startupChoice{root: root, files: filesInsideProject(root, explicitFiles)}
	}
	if len(explicitFiles) > 0 {
		return startupChoice{looseFiles: explicitFiles}
	}
	if freshSession || !reopen || session.Root == "" {
		return startupChoice{}
	}
	if session.Remote != nil {
		return startupChoice{
			root:   session.Root,
			files:  append([]string(nil), session.Files...),
			remote: session.Remote,
		}
	}
	return startupChoice{
		root:  session.Root,
		files: filesInsideProject(session.Root, session.Files),
	}
}

func startupWillOpenProject(session Session, prefs Preferences, freshSession bool) bool {
	choice := chooseStartup("", nil, session, prefs.reopenLastProject(), freshSession)
	return choice.root != "" || len(choice.looseFiles) > 0 || choice.remote != nil
}

// explicitLaunchPaths keeps every path named by the launch command. A missing
// file still suppresses project restore; openPaths reports the later failure
// instead of silently opening the previous session.
func explicitLaunchPaths(paths []string) []string {
	out := make([]string, 0, len(paths))
	seen := make(map[string]bool, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		path = realPath(path)
		if seen[path] {
			continue
		}
		seen[path] = true
		out = append(out, path)
	}
	slices.Sort(out)
	return out
}

func filesInsideProject(root string, paths []string) []string {
	files := make([]string, 0, len(paths))
	for _, path := range paths {
		if inProject(root, path) {
			files = append(files, path)
		}
	}
	return normalizeFiles(files)
}

// Tab is one open file: its backend document, the editor state editing the
// document's buffer, and how much of that buffer the backend has.
type Tab struct {
	// owner is the window that shows this tab. Virtual pages use it to find
	// their model without a shared document registry.
	owner   *Model
	content *pluginContentSession
	doc     *docbridge.Document
	state   *editor.State
	caret   *caretClickState
	synced  uint64 // buffer version the backend was last given
	seen    uint64 // buffer version last looked at, for the sync debounce
	// inline is the backend's latest inline completion at this tab's caret.
	inline inlineCompletionState
	// columnMode makes Shift+arrows extend a rectangular selection one row at a
	// time, matching OpenIDEA's Column Selection Mode.
	columnMode  bool
	columnBlock columnSelectionBlock
	// todoSeen and todoDark are the buffer version and appearance the editor's
	// TODO tints were computed for, so a frame only rescans when one changed.
	todoSeen uint64
	todoDark bool
	// Semantic tokens keep the exact buffer version and document they describe.
	// One request may be in flight per tab; stale replies are discarded.
	semanticVersion    uint64
	semanticGeneration uint64
	semanticLoaded     bool
	semanticLoading    bool
	semanticRetryAt    time.Time
	semanticRanges     []semanticTokenSpan
	edited             time.Time
	syncing            bool
	saving             bool

	// formatting marks a reformat request in flight, so a second one does not
	// apply the first one's edits twice.
	formatting bool
	// centerCaret asks the next frame to center the caret after a keyboard
	// move, when Editor › General prefers that scroll behavior.
	centerCaret bool
	// renderedDocsOverride is View › Active Editor › Render Documentation
	// Comments for this tab. Nil follows the global editor setting.
	renderedDocsOverride *bool

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

	// editorFocused is the editor's last rendered focus state. Vim's shortcut
	// claim uses it so Ctrl conflicts stay with editor keys only while the
	// editor, rather than a tool window or dialog, owns the keyboard.
	editorFocused bool

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
	// blame is the live Git annotation gutter for this tab. It is separate
	// from Model.annotate, the static sheet a merge pane uses for one side.
	blame *tabBlameState

	// springControllers is the parsed Spring Web mapping model for the buffer
	// version springVersion. It feeds both the request gutter and its menus.
	springVersion     uint64
	springParsed      bool
	springControllers []springController

	// outline is the document's symbols for breadcrumbs; outlineGen drops
	// replies that a newer request has superseded.
	outline        []openidea.OutlineItem
	outlineDoc     uint32
	outlineVersion uint64
	outlineGen     uint64
	outlineLoading bool

	// usageHighlights are the sticky in-file usages shown by
	// find.highlightUsages. A reply only applies to the document version and
	// caret it was requested for.
	usageHighlights       []usageHighlight
	usageHighlightVersion uint64
	usageHighlightGen     uint64
	usageHighlightBusy    bool
	usageHighlightSticky  bool

	// quickFixGen drops a quick-fix query or apply after the tab, buffer or
	// caret has moved on to another request.
	quickFixGen uint64

	// insertOverwrite is Editor › Toggle Insert/Overwrite for this tab. Typed
	// characters replace what follows the caret when it is on.
	insertOverwrite bool
	// wordExpand is the current Cyclic Expand Word cycle. It ends when the
	// buffer version, caret or document changes.
	wordExpand editorWordExpansion
	// testNavGen drops a Go to Test reply after a newer request or edit.
	testNavGen uint64
	// hierarchyGen drops hierarchy-member replies when another refactoring
	// starts or the document moves on.
	hierarchyGen uint64

	// signatureGen drops parameter information after an edit or caret move.
	signatureGen uint64

	// typeInfoGen drops Expression Type Info replies after an edit or move.
	typeInfoGen uint64

	// compReq is the completion request in flight, compGen and hoverGen drop
	// replies a newer request superseded, and compOff is when completion asks
	// again after the language server was missing.
	compReq    uint32
	compGen    uint64
	compOff    time.Time
	completion completionTracking
	smart      smartKeyState
	autoImport autoImportState
	hoverGen   uint64
	// debugSelectGen drops delayed selection values after the selection moves.
	debugSelectGen   uint64
	debugSelectTimer *time.Timer
	defGen           uint64
	inlay            inlayState
	// coverageMap moves report lines through edits until another coverage
	// run replaces the active suite.
	coverageMap *tabCoverageMap

	// kind marks a built-in page tab ("whatsNew") whose body the page draws
	// instead of the document's text; pageVersion and pageNotes are What's
	// New's release, and pageLoading is a notes fetch in flight.
	kind        string
	pageVersion string
	pageNotes   string
	pageLoading bool
	pageTitle   string
	agentTask   string
	pagePath    string
	pageDirty   bool

	// markdownMode is the native Markdown layout for this document. Empty
	// means the last chosen Markdown layout.
	markdownMode markdownMode
	// viewer holds decoded image or hex data for a non-text file tab.
	viewer *fileViewerState
}

const tabKindVCSDiff = "vcsDiff"

// Model is the app's state.
type Model struct {
	cfg Config
	win *mygo.Window
	mgr *openidea.Manager
	// workspaceHost owns the other native project windows. Nil keeps the
	// original standalone window behavior for tests and alternate front ends.
	workspaceHost WorkspaceHost
	// invokeHook replaces the manager's invoke surface in tests; nil means
	// the live server.
	invokeHook func(ctx context.Context, cmd string, args any) (json.RawMessage, error)
	// openRecentHook replaces the backend probe and open call in recent-project
	// tests. Production uses the same model RPC surface as other project opens.
	openRecentHook func(path string, remove func()) bool
	// postHook replaces RunOnMain in tests, which run without the event
	// loop; nil means the real main-thread hop.
	postHook func(fn func())
	// confirmExitHook replaces the native exit confirmation in focused window
	// close tests. Production shows the same prompt as application Quit.
	confirmExitHook func(done func(confirmed, dontAsk bool))
	// refreshAfterVCSWriteHook lets focused operation tests observe a
	// completed remote call without starting the live refresh RPCs.
	refreshAfterVCSWriteHook func()
	reg                      *docbridge.Registry
	ctx                      context.Context
	cancel                   context.CancelFunc
	remote                   remoteDevelopmentState
	// fileEncodings caches the global/properties defaults used by standalone
	// LightEdit documents. Project documents receive the same defaults from
	// openidea-server's project settings.
	fileEncodings fileEncodingRuntime

	rootMu              sync.RWMutex
	root                string
	workspaceGeneration uint64
	pluginContentMu     sync.RWMutex
	pluginContent       pluginContentModelState
	projectName         string
	projectDisplayName  string
	// workspace is project_current's manifest for this folder. Nil means the
	// folder is an ordinary single project.
	workspace *openidea.Workspace
	// workspaceDialog is New Workspace or Manage Workspace while it is open.
	workspaceDialog *workspaceDialogState

	tabs   []*Tab
	active int
	tree   *Tree
	// openQueue holds files requested while another document is opening. Each
	// entry keeps its openSpec so a batch can continue after a completed open.
	openQueue []queuedOpen
	// treeScroll keeps how far the Project tree shows its rows from: a row
	// wider than the window scrolls sideways, as OpenIDEA's tree does.
	treeScroll ui.ScrollState
	// projectTreeAnchor is the selected Project View row's last drawn bounds.
	// The New popup opens under it, as OpenIDEA anchors the popup to the lead row.
	projectTreeAnchor ui.Rect

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
	// editorFontZoom is the shared temporary zoom of every code editor in
	// points, as OpenIDEA's Increase/Decrease Font Size actions change. It sits
	// on top of Preferences.FontSize and is reset without changing that setting.
	editorFontZoom int

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
	// editorExtras owns the app-wide clipboard history and the native Paste
	// from History sheet.
	editorExtras editorExtrasState

	// rename is the open rename prompt, or nil.
	rename *renameState
	// safeDelete is Refactor › Safe Delete's confirmation, search or preview.
	// safeDeleteSearchText remembers IDEA's comments/strings checkbox across
	// invocations for this app session.
	safeDelete           *editorSafeDeleteState
	safeDeleteSearchText bool

	// symbols is the open Go to Symbol picker, or nil.
	symbols *symbolPicker
	// These native popups share the editor action-gap implementation.
	customRegions     *customRegionPopup
	testNav           *testNavigationPopup
	unwrap            *unwrapPopupState
	hierarchyRefactor *hierarchyRefactorState

	// files is the open Quick Open picker, or nil.
	files           *filePicker
	recentNav       recentNavigation
	gotoLine        *gotoLineState
	stackTrace      *stackTraceState
	documentation   documentationState
	templateChooser *templateChooserState
	// templatePrefs is loaded from templates.json next to the main settings
	// file. File templates, live templates, postfix templates and Emmet are
	// intentionally outside Preferences because their bodies are documents.
	templatePrefs *TemplatePrefs
	// quickDefinition is OpenIDEA's Quick Definition / Quick Type Definition
	// popup: declaration source from one of the language server's targets.
	quickDefinition    *quickDefinitionState
	quickDefinitionGen uint64
	coverage           coverageState

	// palette is the open command palette, or nil.
	palette *commandPicker

	// se is Search Everywhere while it is open; recentActions is the commands
	// it lists on an empty query, newest first.
	se            *sePane
	recentActions []string
	// doubleShift recognizes Shift released and pressed again within
	// OpenIDEA's 300 ms window.
	doubleShift doubleShiftState
	// caretCloneChord arms OpenIDEA's double-Alt/double-Control carets for
	// the next Up or Down key.
	caretCloneChord caretCloneChordState

	// searchHistory is the All tab's Search Everywhere history, kept for
	// compatibility with older sessions. searchHistories keeps every tab.
	searchHistory   []string
	searchHistories map[seTab][]string
	sePreviewState  bool
	// seFilters is Search Everywhere's funnel, mirrored into the prefs.
	seFilters seFilters

	// overlayIDs is the overlays the last frame drew, so closing the last one
	// hands the keyboard back to the editor.
	overlayIDs []string

	// dockDragID, dockDragSlot and dockDragArmed carry a dragged tool window
	// button: the slot under the pointer and whether letting go moves it.
	dockDragID     string
	dockDragSlot   dockSlot
	dockDragArmed  bool
	dockDragStatus string

	// notify is the notification center: the balloons and their history.
	notify notifier
	// eventRuntime owns backend-event queues, timers and unsupported-consumer
	// markers so each received event stays with this model's project window.
	eventRuntime eventRuntimeState

	// services is the Services bottom window.
	services servicesPane
	// github is the native Pull Requests tool window and its GitHub API state.
	github githubState
	// http is OpenIDEA's HTTP Client content view in Services.
	http    httpClientState
	graphql graphqlPane
	// dotnet owns the native .NET workspace, NuGet and test discovery pane.
	dotnet dotnetPane
	// logcat streams one connected Android device into the native tool window.
	logcat logcatState
	// profiler owns Run with Profiler plans, snapshots and native analysis views.
	profiler profilerState
	perf     perfState
	// externalLibraries is the pinned backend's SDK and dependency root model,
	// shown as a read-only External Libraries contribution in Project View.
	externalLibraries *externalLibrariesState
	// librarySources owns OpenIDEA's Download Sources and Choose Sources
	// requests for decompiled library classes, keyed by the virtual document
	// path the backend understands.
	librarySources map[string]*librarySourceState
	maven          mavenPane
	// versionCatalogs caches Gradle catalog text loaded through the backend.
	versionCatalogs gradleVersionCatalogState
	// notebooks keeps one backend notebook session per path, shared by every
	// editor tab that shows it.
	notebooks notebookState
	// impl is the Go to Implementation(s) search and chooser.
	impl implChooser
	// navDone and navLanded record the last language-server navigation, for
	// the headless acceptance runs.
	navDone   bool
	navLanded string

	// dock is the six-slot dock: the slots are the authority on where each
	// registered tool window shows.
	dock dockState
	// rightSplitTool mirrors the window the right side's split slot shows.
	rightSplitTool string
	// rightRatio is the share of the right side the primary slot takes when
	// both are occupied; rightDragging and rightDragTotal carry a drag of the
	// gap between its two windows.
	rightRatio     float32
	rightDragging  bool
	rightDragTotal float32

	// runAnything is the open Run Anything popup, or nil.
	runAnything *runAnythingState

	// diff is the open compare-with-HEAD view, or nil.
	diff *diffView
	// merge is OpenIDEA's native three-way merge editor, or nil.
	merge *mergeEditorState

	// db is the open database sheet, or nil.
	db *dbView
	// dbTable is the open table data page.
	dbTable *dbTableSheet
	// dbHist is the open statement history and dbPlan the execution plan.
	dbHist *dbHistoryPane
	dbPlan *dbPlanPane

	// intents is the open intentions prompt, or nil.
	intents *intentPane

	// prefs are the user's stored settings, and theme is the theme they pick,
	// nil to follow the system; settingsOpen shows the settings sheet.
	prefs Preferences
	// settingsBaseline is this window's last known settings JSON. Saving diffs
	// it against prefs so another native window's changed leaves survive.
	settingsBaseline json.RawMessage
	// editorSchemeBusy serializes the backend-backed color-scheme actions so
	// repeated clicks cannot queue an unbounded stream of settings writes.
	editorSchemeBusy bool
	// vim is the native editor Vim runtime and its loaded rc-file state.
	vim vimRuntime
	// macros is Edit › Macros' recorder, last playback and named recordings.
	macros macroState
	// systemRegion is the regional formatting snapshot captured at the start
	// of the frame. Main-thread helpers use it after the view, and Code Vision
	// captures a copy before starting background work.
	systemRegion dateRegion
	theme        *ui.Theme
	systemDark   bool
	settingsOpen bool
	// aboutOpen and keymapReference are the Help menu's app-level sheets.
	aboutOpen       bool
	keymapReference keymapReferenceState
	// settingsPage is the page the settings sheet shows, by its OpenIDEA id,
	// or "group:<name>" while a group folder is selected.
	settingsPage string
	// settingsDraft stages the dialog's edits, as OpenIDEA's settings store
	// does: Apply and OK write them to prefs, Cancel drops them. settingsQuery
	// filters the page tree, and the history stack takes Back and Forward.
	settingsDraft     settingsDraft
	settingsTemplates templateSettingsUI
	settingsQuery     string
	settingsHistory   []string
	settingsHistoryAt int
	// settingsFontMonoOnly filters Editor › Font to monospaced families, and
	// settingsFontPreview is the editable sample shown under that page.
	settingsFontMonoOnly bool
	settingsFontPreview  string
	settingsThemePrefs   bool
	// settingsExternalTool is the selected row of Tools › External Tools.
	settingsExternalTool string
	// externalToolPrompt asks for $Prompt$ before an External Tool runs.
	externalToolPrompt *externalToolPromptState
	// settingsCollapsed are the tree nodes closed with their caret, and
	// settingsSide is the page tree's width after a drag of the split handle.
	settingsCollapsed      map[string]bool
	settingsSide           float32
	settingsSideDragging   bool
	settingsWindowResizing bool
	settingsOffsetX        float32
	settingsOffsetY        float32
	settingsSectionStarted bool
	// keymapRecorder stages a shortcut captured for one Keymap action; Apply
	// writes it through the Settings draft rather than the live preferences.
	keymapRecorder keymapRecorderState
	// keymapDispatch caches the selected preset's parsed keys and holds the
	// first stroke while waiting for a two-stroke chord.
	keymapDispatch keymapDispatchState
	// settingsErr is a failed write of the staged settings, shown in the
	// dialog's footer as OpenIDEA shows its save error.
	settingsErr string
	// projectStructure is File › Project Structure: the staged Project,
	// Modules, Dependencies, Libraries, Facets, Artifacts and SDKs model.
	projectStructure projectStructureState

	// zen is Distraction Free Mode, and zenHint the way-out hint entering it
	// puts above the editor, as OpenIDEA's balloon does.
	zen     bool
	zenHint bool

	// crumbs are Settings › Editor › Breadcrumbs as the editor draws them:
	// whether the chain shows and whether it sits below the text, so the
	// dialog can preview a staged choice.
	crumbs BreadcrumbPrefs
	// navigationOpen is Navigate › Jump to Navigation Bar's floating chooser.
	navigationOpen bool
	// navigation keeps that chooser's snapshot while it is open: the
	// segments, the highlighted one, and the child list opened below it.
	navigation navigationBarState

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
	// tasks are the background tasks the status bar shows, and tasksOpen
	// tells whether its Background Tasks popup is up.
	tasks         []BackgroundTask
	tasksOpen     bool
	tasksStopMode bool

	// invalidateDlg is File › Invalidate Caches… while it is open, and
	// restartPending relaunches the app once the quit flow finishes.
	invalidateDlg  *invalidatePane
	restartPending bool
	// runCfgDlg is Run › Edit Configurations… while it is open, rerunAsk is
	// the "Process is running" confirm of a rerun, chooser is the Select
	// Run/Debug Configuration popup, and attach is Attach to Process.
	runCfgDlg *runConfigDlg
	rerunAsk  *rerunAsk
	chooser   *runChooser
	attach    *attachDlg
	// pendingJump is a line an open should land on once its tab exists: a
	// test tree's failing node opened before the document finished loading.
	pendingJump map[string]int
	// ready is the cached six-state readiness snapshot (section 8), refreshed
	// when the project opens, after the run configs and the build model land,
	// and by the readiness smoke.
	ready *Readiness
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
	// javaSettings is Settings › Languages & Frameworks › Java. Its draft is
	// loaded with the project or global settings scope and saved field by
	// field through settings_set.
	javaSettings javaSettingsState
	// languageSDKs caches sdk_detect results for the native language pages and
	// owns the cancellable Python environment creation flow.
	languageSDKs languageSDKSettingsState
	// fileTypesSettings is Settings › Editor › File Types and the runtime
	// associated-application/file-type snapshot. inspectionsSettings,
	// spellingSettings and formatterSettings stage the other OpenIDEA pages
	// that live in the pinned backend's settings model.
	fileTypesSettings   fileTypesSettingsState
	inspectionsSettings inspectionsSettingsState
	spellingSettings    spellingSettingsState
	formatterSettings   formatterSettingsState
	// nativeSettings stages the backend-backed pages whose controls map
	// directly to dotted settings keys. Each page keeps its own applied and
	// draft maps so navigating between pages cannot discard staged edits.
	nativeSettings   nativeSettingsCatalogState
	nativeSettingsMu sync.Mutex
	// backendFormatOnSave mirrors editor.formatOnSave, which the backend's
	// lsp_format checks before running an external formatter on save.
	backendFormatOnSave      bool
	backendFormatOnSaveKnown bool
	// languageTools is the complete backend registry behind Settings ›
	// Languages & Frameworks › Language Servers. javaTools is its Java subset,
	// retained for the Java page and the debugger prompt.
	languageTools      []javaTool
	languageToolPick   string
	languageToolFind   string
	languageToolFilter string
	// languageToolSettings is the staged languageServers/tools configuration
	// for the Language Servers page. It loads from settings_get and applies
	// field diffs through settings_set.
	languageToolSettings languageToolSettingsState
	// These sets keep language-server install prompts quiet after their first
	// offer and suppress a fallback whose preferred alternative was already
	// offered, as OpenIDEA's install missing server prompts do.
	languageToolPrompted map[string]bool
	languageToolOffered  map[string]bool
	languageToolWarned   map[string]bool
	languageToolLoading  map[string]bool
	// languageServers is the live snapshot behind the Language Services
	// status item. Event sequence numbers keep an older lsp_servers reply from
	// overwriting a newer lifecycle event or resurrection after removal.
	languageServers         map[uint32]languageServerInfo
	languageServerOpen      bool
	languageServerSeq       uint64
	languageServerLastEvent map[uint32]uint64
	languageServerRemovedAt map[uint32]uint64
	languageServerRefresh   uint64
	languageServerBusyGen   map[uint32]uint64
	// languageServerJavaReady remembers the project root each Java server last
	// loaded. Background busy progress does not revoke readiness until restart.
	languageServerJavaReady map[uint32]string
	// jdkDialog is Download JDK, and jdkDownloadBusy / jdkDownloadChannel
	// carry the one cancelling and streaming download it starts.
	jdkDialog        jdkDialogState
	jdkDownloadBusy  bool
	jdkDownloadCh    uint32
	jdkDownloadReq   uint32
	jdkDownloadVer   uint32
	jdkDownloadAbort atomic.Bool
	// jdkPrompted prevents the same language-server binary from raising the
	// same JDK download prompt again during this app session.
	jdkPrompted map[string]bool

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
	// recentProjectsPopup is File › Open Recent's compact chooser over the
	// Welcome screen's recent projects, or nil while closed.
	recentProjectsPopup *recentProjectsPopup
	// projectPopup is the title-bar Projects popup, which can switch to any
	// open window or reopen a recent project without a native menu.
	projectPopup *projectPopup

	// plugins is Welcome › Plugins, made on first use, nil before.
	plugins *pluginPage
	// intellijThemes chooses a theme plugin installed by a JetBrains IDE.
	// intellijConsent is the install preview and permission sheet reached
	// through the same conversion flow.
	intellijThemes  *intellijThemeChooser
	intellijConsent *intellijConsentDialog

	// commit is the open Commit dialog, or nil.
	commit               *commitState
	commitInitialMessage string

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

	// treeClip is what the project view's Copy or Cut holds. projectNew is
	// the searchable New file template chooser, and treeOp is its name prompt
	// or an open Rename, Copy or Delete prompt.
	treeClip     *treeClipState
	projectNew   *projectNewState
	treeOp       *treeOpState
	treeLastMove *treeMoveRecord
	// vcsFileConfirm is OpenIDEA's Add/Remove File to Git prompt after a
	// project-view file operation.
	vcsFileConfirm *vcsFileConfirmation
	// treeDrag is the pointer gesture moving project-view files inside the
	// tree; treeDragFrame lets whichever row is under the pointer claim it
	// as the drop target for this frame.
	treeDrag      *treeDragState
	treeDragFrame uint64
	// projectTreeFocus says Cmd+C, Cmd+Z and the other context-sensitive
	// editor/project shortcuts belong to the Project view.
	projectTreeFocus bool
	// fileRefactor is OpenIDEA's Move / Rename dialog, and
	// fileRefactorPreview the confirmation shown when references change in
	// files other than the moved ones.
	fileRefactor        *fileRefactorDialogState
	fileRefactorPreview *fileRefactorPreviewState
	// history is the open Local History sheet, or nil.
	history *historyState
	// dirCompare is the open Compare Directories sheet, or nil.
	dirCompare *dirCompareState
	// annotate is the open Annotate sheet, or nil.
	annotate *annotateState

	// trustAsk is the Trust Project prompt a project opens with until the
	// user allows building and running it.
	trustAsk    bool
	trustFolder bool
	// projectTrust is Settings › Security › Project Trust's live decision
	// list and the RPCs that revoke one.
	projectTrust projectTrustSettingsState
	// startMu guards startPhase, the startup state machine the window shows.
	startMu    sync.RWMutex
	startPhase string
	run        RunPane
	debug      DebugPane
	build      BuildPane
	term       TermPane
	// termShell is the New Terminal Tab shell list.
	termShell     *termShellPicker
	vcs           VCSPane
	vcsOps        VCSOperationPane
	vcsQuick      vcsQuickList
	vcsMore       vcsMoreState
	recentHistory recentChangesState
	branch        BranchPane
	log           LogPane
	problems      ProblemsPane
	search        SearchPane
	searchScopes  []SearchScope
	usages        UsagesPane
	usagePopup    *usagePopupState
	copyPathPopup *copyPathPopupState
	todo          TodoPane
	// analysis owns OpenIDEA's native Inspection Results, Duplicates,
	// Structural Search and Code Cleanup surfaces.
	analysis  analysisState
	structure StructurePane
	agents    AgentsPane
	// mcp owns the confirmation queue for requests from the IDE's built-in
	// MCP server. Run tabs and file opens are dispatched as soon as they
	// arrive; confirmations wait for the user's answer.
	mcp        mcpRequestsState
	mcpReports mcpReportsState
	mcpServer  mcpServerSettingsState

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
	frameWidth  float32
	frameHeight float32
	// editorWidth is the width the editor island drew at last frame, which
	// caps a drag of the side beside it, and editorHeight the height it drew,
	// which caps a drag of the bottom area below it.
	editorWidth  float32
	editorHeight float32
	// editorBodyRects are the two groups' editor surfaces as last drawn. A
	// lifted tab released near one of these edges moves into a split there.
	editorBodyRects [2]ui.Rect
	// usagePopupAnchor is the caret's last drawn position, so Show Usages
	// opens beside the editor where OpenIDEA anchors it.
	usagePopupAnchor ui.Rect
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

	// startupRoot opens this project instead of the Welcome screen, for `goidea
	// <dir>` and the offscreen smoke run.
	startupRoot string
	// startupFiles opens these files beside startupRoot, so an offscreen run
	// has a tab without touching the user's session.
	startupFiles []string
	// startupPending suppresses Welcome while the first frame is waiting for
	// the backend to restore a project. OpenIDEA restores the project window
	// without flashing a Welcome frame first.
	startupPending bool

	// recents are the Welcome screen's projects, most recent first, and
	// recentGroups the headings it lists them under; welcomeQuery filters
	// them, welcomeSel is the row Enter opens, and welcomePage is the rail
	// page shown: "" for projects, "customize" or "plugins".
	recents      []RecentProject
	recentGroups []RecentGroup
	welcomeQuery string
	welcomeSel   int
	welcomePage  string
	// welcomeGroupPath is the project the New Group prompt was opened from.
	welcomeGroupPath string

	// newProject is File › New Project… while it is up.
	newProject newProjectState

	// welcomeGroupDlg is the New Group / Rename Group prompt while it is up:
	// the name being typed, the group being renamed ("" for a new one) and
	// the project the new group's menu was opened for.
	welcomeGroupDlg welcomeGroupDialog

	// layouts are each project's saved workbench layout, by folder, as
	// OpenIDEA keeps one per project root; sessionSeq orders the session
	// writes so a slow one never overwrites a newer snapshot.
	layouts           map[string]Layout
	namedLayouts      []SavedToolWindowLayout
	activeNamedLayout string
	layoutDlg         *toolWindowLayoutDialog
	layoutHold        bool
	// projectViews is each project's expanded Project View folders, which
	// belongs to the project rather than the global pane and options.
	projectViews              map[string][]string
	projectUnityVersion       string
	projectTreeMembersLoading map[string]bool
	// runHistory is each project's Run Anything history, by folder, as
	// OpenIDEA keeps it per project root.
	runHistory   map[string][]string
	sessionSeq   int
	sessionWrote int
	sessionMu    sync.Mutex
	sessionWrite sync.Mutex
	sessionWG    sync.WaitGroup
	closing      bool

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
	// recentOpenInFlight prevents repeated row activation while a failed recent
	// project is waiting for the user to remove it.
	recentOpenInFlight string
	// openRecentDialog lets a test exercise the missing-folder prompt without
	// opening a native window.
	openRecentDialog func(string, error) (bool, error)
	// applicationQuit distinguishes the app-wide close from a single-window
	// close preflight, which shares the quitting guard.
	applicationQuit bool
	// openDroppedFolderHook keeps drop-routing assertions off the live backend.
	openDroppedFolderHook func(string)
}

// New returns a model; Start opens the last session.
func New(cfg Config) *Model {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Model{
		cfg: cfg, ctx: ctx, cancel: cancel, bottomHeight: bottomDefaultHeight,
		safeDeleteSearchText: true,
	}
	m.prefs = LoadPreferences(cfg.SettingsPath)
	if raw, err := json.Marshal(m.prefs); err == nil {
		m.settingsBaseline = raw
	}
	if m.prefs.SearchFilters != nil {
		m.seFilters = m.prefs.SearchFilters.sanitized()
	}
	m.seFilters = m.seFilters.sanitized()
	m.sePreviewState = m.prefs.SearchPreview
	m.ensureMacros()
	m.syncSavedMacroActions()
	m.applyPreferences()
	m.syncExternalToolActions()
	session := LoadSession(cfg.SessionPath)
	m.startupPending = startupWillOpenProject(session, m.prefs, cfg.FreshSession)
	m.recents = sessionRecents(session)
	m.recentGroups = session.RecentGroups
	m.searchHistories = map[seTab][]string{}
	for tab, queries := range session.SearchHistories {
		m.searchHistories[seTab(tab)] = append([]string(nil), queries...)
	}
	if len(m.searchHistories[seTabAll]) == 0 && len(session.SearchHistory) > 0 {
		m.searchHistories[seTabAll] = append([]string(nil), session.SearchHistory...)
	}
	m.searchHistory = append([]string(nil), m.searchHistories[seTabAll]...)
	m.logcat.history = append([]string(nil), session.LogcatHistory...)
	m.logcat.historyAt = -1
	m.searchScopes = append([]SearchScope(nil), session.SearchScopes...)
	m.restoreNotificationHistory(session.Notifications)
	m.layouts = map[string]Layout{}
	for root, l := range session.Layouts {
		m.layouts[layoutKey(root)] = normalizeLayout(l)
	}
	m.namedLayouts = normalizeSavedToolWindowLayouts(session.NamedLayouts)
	m.activeNamedLayout = normalizeActiveNamedLayout(
		session.ActiveNamedLayout, m.namedLayouts,
	)
	m.projectViews = map[string][]string{}
	for root, expanded := range session.ProjectViews {
		m.projectViews[layoutKey(root)] = append([]string(nil), expanded...)
	}
	m.runHistory = normalizeRunHistory(session.RunHistory)
	m.mgr = openidea.NewManager(openidea.LaunchConfig{
		ServerPath:     cfg.ServerPath,
		Home:           cfg.Home,
		Log:            cfg.Log,
		MinVersion:     cfg.ServerMinVersion,
		ExpectedSHA256: cfg.ServerSHA256,
	})
	hybridDocs := newHybridDocs(
		openidea.Docs{M: m.mgr, Window: Window},
		m.Root,
	)
	hybridDocs.SetEncodingRuntime(m.ensureFileEncodingSettings, m.standaloneEncodingFallback)
	hybridDocs.SetRemotePathMatcher(m.isExternalLibraryPath)
	m.reg = docbridge.NewRegistry(hybridDocs, Window)
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
		fail := func(err error) error {
			m.post(func() {
				m.setStartPhase("failed")
				m.status = "Restoring the project after the backend restart failed"
				m.banner = "Restoring the project after the backend restart failed: " + err.Error()
				m.bannerPath = ""
				m.notifyError("backend-restore", "The backend restarted, but restoring the project failed", err.Error())
				m.invalidate()
			})
			return err
		}
		m.setStartPhase("restoring")
		m.post(func() { m.status = "The backend restarted; restoring the project…"; m.invalidate() })
		if err := m.openBackendProject(ctx, root); err != nil {
			return fail(err)
		}
		if err := m.reg.ReopenAll(ctx); err != nil {
			return fail(err)
		}
		_ = m.loadNativeRuntimeSettings(ctx)
		m.post(func() {
			m.run.running = false
			m.run.awaiting = false
			m.run.busy = false
			m.run.exitCode = nil
			if m.run.visible {
				m.run.err = "The backend restarted; the run was interrupted"
			}
			m.debug.starting = false
			m.debug.paused = false
			m.debug.session = 0
			if m.debug.visible {
				m.debug.err = "The backend restarted; the debug session was interrupted"
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
				m.term.err = "The backend restarted; the terminal was closed"
			}
			if m.run.trusted {
				m.loadRunConfigs()
				m.RefreshExternalLibraries()
			}
			m.dockerBackendRestored()
			m.connectAgents(true)
			m.pluginVisualBackendRestarted()
			m.setStartPhase("ready")
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
	m.SetWindowActive(win.IsFocused())
	m.mcpReports.windowFocused = win.IsFocused()
	m.registerWindowCloseHandler(win.OnClose)
	win.OnBlur(func() {
		m.SetWindowActive(false)
		m.mcpSetWindowFocused(false)
		m.autoSaveOnSwitch()
		m.updateApplicationsOnDeactivation()
	})
	win.OnFocus(func() {
		m.SetWindowActive(true)
		if m.workspaceHost != nil {
			m.workspaceHost.WindowFocused(m)
		}
		m.mcpSetWindowFocused(true)
		m.syncExternalOnFocus(time.Now())
	})
	if m.workspaceHost != nil {
		m.workspaceHost.WindowFocused(m)
	}
}

// registerWindowCloseHandler installs the same callback used by every native
// window. The seam lets tests invoke MyGo's CloseEvent path without starting a
// visible window or the native event loop.
func (m *Model) registerWindowCloseHandler(register func(func(*mygo.CloseEvent)) func()) {
	if register == nil {
		return
	}
	register(m.handleWindowClose)
}

// handleWindowClose is the callback Attach registers with MyGo's native close
// button. It first prevents the default close so process and document guards
// can run, then destroys the window only after they succeed.
func (m *Model) handleWindowClose(e *mygo.CloseEvent) {
	if m.forceQuit {
		return
	}
	m.requestWindowClose(e.PreventDefault)
}

// OpenAtStartup opens dir instead of the session's project; call before Start.
func (m *Model) OpenAtStartup(dir string) {
	m.startupRoot = dir
	if strings.TrimSpace(dir) != "" {
		m.startupPending = true
		m.showingWelcome = false
	}
}

// OpenFilesAtStartup opens these files in the startup project; call before
// Start. Paths outside the project are ignored, as session files are.
func (m *Model) OpenFilesAtStartup(paths []string) {
	m.startupFiles = paths
	if len(paths) > 0 {
		m.startupPending = true
		m.showingWelcome = false
	}
}

// Snapshot reports what the smoke run checks: the project, its tabs, and
// whether the backend is up.
func (m *Model) Snapshot() (root string, tabs int, backend bool) {
	return m.Root(), len(m.tabs), m.BackendState().Running
}

// startPhase is where startup is: preflight, server, opening, restoring,
// ready or failed.
func (m *Model) setStartPhase(phase string) {
	m.startMu.Lock()
	m.startPhase = phase
	m.startMu.Unlock()
}

// Phase reports the startup state machine's current phase.
func (m *Model) Phase() string {
	m.startMu.RLock()
	defer m.startMu.RUnlock()
	return m.startPhase
}

// Start restores the last session in the background.
func (m *Model) Start() {
	m.post(func() { m.busy = true; m.status = "Connecting to the GoIDEA backend…" })
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
	// Start the plugin engine once the window loop is up: enabled plugins
	// activate on their own goroutines and their commands join the palette.
	m.pluginsPage()
	m.setStartPhase("server")
	// The backend's preflight and initialize run before a project is opened,
	// so Welcome is served by a live server and a broken binary fails here.
	startCtx, cancelStart := context.WithTimeout(m.ctx, 60*time.Second)
	_, startErr := m.mgr.Ensure(startCtx)
	cancelStart()
	if startErr != nil {
		m.setStartPhase("failed")
		m.post(func() {
			m.busy = false
			m.startupPending = false
			m.status = "The GoIDEA backend failed to start"
			m.banner = "The GoIDEA backend failed to start: " + startErr.Error()
			m.notifyError("backend", "GoIDEA backend failed to start", startErr.Error())
			m.invalidate()
		})
		return
	}
	runtimeCtx, cancelRuntime := context.WithTimeout(m.ctx, 20*time.Second)
	_ = m.loadNativeRuntimeSettings(runtimeCtx)
	cancelRuntime()
	s := m.session()
	choice := chooseStartup(
		m.startupRoot,
		m.startupFiles,
		s,
		m.startupReopensProject(),
		m.cfg.FreshSession,
	)
	m.startUpdateChecks()
	if choice.remote != nil {
		m.remote.restoreFiles = append([]string(nil), choice.files...)
		m.setStartPhase("opening")
		m.post(func() {
			m.openRemoteLocation(choice.remote.Target, choice.remote.Root)
		})
		return
	}
	if choice.root == "" {
		runtimeCtx, cancelRuntime := context.WithTimeout(m.ctx, 20*time.Second)
		m.refreshFileTypeRuntime(runtimeCtx)
		cancelRuntime()
		m.setStartPhase("ready")
		m.post(func() {
			m.startupPending = false
			m.busy = false
			if len(choice.looseFiles) > 0 {
				m.status = "Opening standalone files…"
				m.openPaths(choice.looseFiles)
			} else {
				m.status = "Open a Java project"
			}
		})
		return
	}
	m.setStartPhase("opening")
	m.setRoot(choice.root)
	m.openProject(choice.root, choice.files)
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
	changed := m.root != root
	m.root = root
	m.rootMu.Unlock()
	if changed {
		m.clearLanguageServers()
		m.workspaceStateChanged()
	}
}

// post runs fn on the main thread and waits for it.
func (m *Model) post(fn func()) {
	// Tests run without the mygo event loop; postHook executes inline.
	if m.postHook != nil {
		m.postHook(fn)
		return
	}
	mygo.RunOnMain(fn)
}

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
					m.setStartPhase("failed")
					m.status = "Backend unavailable: " + state.Err.Error()
					m.dockerBackendLost(state.Err)
					m.run.running = false
					m.run.awaiting = false
					m.run.busy = false
					m.debug.starting = false
					m.debug.paused = false
					m.debug.session = 0
				case state.Running:
					m.status = "Connected to the GoIDEA backend"
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
	m.confirmSwitchWithFiles(dir, nil)
}

// OpenRecentProject applies OpenIDEA's recent-project routing before the
// normal switch policy. A folder that no longer exists is offered for removal.
// An ordinary open failure is reported with the same removal action, while the
// project already on screen remains untouched.
func (m *Model) OpenRecentProject(dir string) {
	m.openRecentProject(dir)
}

func (m *Model) openRecentProject(dir string) {
	if m == nil || strings.TrimSpace(dir) == "" || m.recentOpenInFlight != "" {
		return
	}
	clean := realPath(dir)
	m.recentOpenInFlight = clean
	m.status = "Opening " + filepath.Base(clean) + "…"
	m.invalidate()
	if m.openRecentHook != nil {
		ok := m.openRecentHook(clean, func() { m.removeRecentProject(clean) })
		m.recentOpenInFlight = ""
		if !ok {
			m.status = ""
			m.invalidate()
		}
		return
	}
	ctx, cancel := context.WithTimeout(m.workspaceContext(), 8*time.Second)
	go func() {
		defer cancel()
		if m.mgr != nil {
			if raw, err := m.mgr.Invoke(ctx, Window, "fs_read_dir", map[string]any{"path": clean}); err == nil {
				var entries []json.RawMessage
				if json.Unmarshal(raw, &entries) == nil {
					m.post(func() {
						m.recentOpenInFlight = ""
						m.confirmSwitch(clean)
					})
					return
				}
			} else if recentProjectNotFound(err) {
				m.post(func() {
					m.recentOpenInFlight = ""
					m.status = ""
					m.confirmRemoveMissingRecent(clean, err)
				})
				return
			}
		}
		info, err := os.Stat(clean)
		if err != nil || !info.IsDir() {
			if err == nil {
				err = fmt.Errorf("%s is not a folder", clean)
			}
			m.post(func() {
				m.recentOpenInFlight = ""
				m.status = ""
				m.confirmRemoveMissingRecent(clean, err)
			})
			return
		}
		m.post(func() {
			m.recentOpenInFlight = ""
			m.confirmSwitchRecent(clean)
		})
	}()
}

func (m *Model) confirmSwitchRecent(dir string) {
	if m.focusOpenProject(realPath(dir)) {
		return
	}
	m.confirmSwitch(dir)
}

func recentProjectNotFound(err error) bool {
	if err == nil {
		return false
	}
	kind := strings.ToLower(openidea.ErrorKind(err))
	message := strings.ToLower(err.Error())
	return kind == "notfound" || strings.Contains(kind, "not_found") ||
		strings.Contains(message, "not found") || strings.Contains(message, "no such file")
}

func (m *Model) confirmRemoveMissingRecent(dir string, err error) {
	message := "Project folder not found: " + filepath.Base(dir)
	body := dir
	if err != nil {
		body += "\n" + err.Error()
	}
	if m.openRecentDialog != nil {
		remove, dialogErr := m.openRecentDialog(message, errors.New(body))
		if dialogErr != nil {
			m.notifyRecentOpenFailure(dir, dialogErr)
			return
		}
		if remove {
			m.removeRecentProject(dir)
		}
		return
	}
	go func() {
		res, dialogErr := mygo.Dialog.Message(mygo.MessageOptions{
			Parent: m.win, Type: mygo.MessageError, Title: message, Message: body,
			Buttons:       []string{"Remove from Recent Projects", "Cancel"},
			DefaultButton: 0, CancelButton: 1,
		})
		if dialogErr != nil {
			m.post(func() { m.notifyRecentOpenFailure(dir, dialogErr) })
			return
		}
		m.post(func() {
			if res.Button == 0 {
				m.removeRecentProject(dir)
			}
		})
	}()
}

func (m *Model) notifyRecentOpenFailure(dir string, err error) {
	body := ""
	if err != nil {
		body = notifySuffix(err.Error())
	}
	remove := func(model *Model) { model.removeRecentProject(dir) }
	m.notifyError(
		"recent.open:"+dir,
		"Cannot open "+filepath.Base(dir),
		body,
		notifyAction{label: "Remove from Recent Projects", run: remove},
	)
}

func (m *Model) notifyProjectOpenFailure(dir string, err error) {
	if m == nil || dir == "" || err == nil {
		return
	}
	remove := func(model *Model) { model.removeRecentProject(dir) }
	m.notifyError(
		"project.open:"+dir,
		"Cannot open "+filepath.Base(dir),
		notifySuffix(err.Error()),
		notifyAction{label: "Remove from Recent Projects", run: remove},
	)
}

func (m *Model) removeRecentProject(dir string) {
	if dir == "" {
		return
	}
	clean := realPath(dir)
	kept := m.recents[:0]
	for _, project := range m.recents {
		if realPath(project.Path) != clean {
			kept = append(kept, project)
		}
	}
	m.recents = kept
	delete(m.welcomeBranches, dir)
	delete(m.welcomeBranchLoading, dir)
	m.saveRecents()
	m.invalidate()
}

// confirmSwitchWithFiles is confirmSwitch with files to reopen after the
// project switch. LightEdit uses it to turn the current standalone file's
// directory into a project without losing the file from the editor.
func (m *Model) confirmSwitchWithFiles(dir string, files []string) {
	if m.Root() != "" && dir != m.Root() {
		switch openProjectWindowPolicy(m.prefs.System.openProjectIn()) {
		case "new":
			if m.openProjectInNativeWindow(dir, files) {
				return
			}
			if m.workspaceHost != nil {
				m.status = "Could not open another project window"
				m.invalidate()
				return
			}
		case "ask":
			if m.workspaceHost != nil {
				m.askOpenProjectWindow(dir, files)
				return
			}
		}
	}
	m.confirmSwitchDirect(dir, files)
}

func (m *Model) openProjectPath(path string) {
	m.projectPathRequest(openProjectPathRequest{path: path})
}

func (m *Model) openProjectPathArg(arg any) {
	m.projectPathRequest(openProjectPathRequestArg(arg))
}

// askOpenProjectWindow is OpenIDEA's Ask policy: the user chooses when an
// existing project is about to be replaced by another project.
func (m *Model) askOpenProjectWindow(dir string, files []string) {
	if m.win == nil || m.workspaceHost == nil {
		m.confirmSwitchDirect(dir, files)
		return
	}
	go func() {
		res, err := mygo.Dialog.Message(mygo.MessageOptions{
			Parent: m.win, Type: mygo.MessageInfo, Title: "Open Project",
			Message: "Open " + filepath.Base(dir) + " in a new window?",
			Detail:  "The current project can stay open in its own window.",
			Buttons: []string{"New Window", "Current Window", "Cancel"}, DefaultButton: 0, CancelButton: 2,
		})
		if err != nil {
			return
		}
		m.post(func() {
			switch res.Button {
			case 0:
				if !m.openProjectInNativeWindow(dir, files) {
					m.status = "Could not open another project window"
					m.invalidate()
				}
			case 1:
				m.confirmSwitchDirect(dir, files)
			}
		})
	}()
}

// askOpenInSingleWindow preserves a sensible confirmation for callers that
// provide no native-window host, such as a standalone test model.
func (m *Model) askOpenInSingleWindow(dir string, files []string) {
	if m.win == nil {
		m.confirmSwitchDirect(dir, files)
		return
	}
	go func() {
		res, err := mygo.Dialog.Message(mygo.MessageOptions{
			Parent: m.win, Type: mygo.MessageInfo, Title: "Open Project",
			Message: "Open " + filepath.Base(dir) + " in this window?",
			Detail:  "This build uses one native window; opening another project replaces the current one.",
			Buttons: []string{"Open Here", "Cancel"}, DefaultButton: 0, CancelButton: 1,
		})
		if err != nil {
			return
		}
		m.post(func() {
			if res.Button == 0 {
				m.confirmSwitchDirect(dir, files)
			}
		})
	}()
}

func (m *Model) confirmSwitchDirect(dir string, files []string) {
	if m.busy {
		return
	}
	target, remoteRoot, remote := parseRemoteProjectLocation(dir)
	if !remote && dir == m.Root() {
		if len(files) == 0 {
			return
		}
		m.openBatch(files, nil)
		return
	}
	m.requestProjectLeave(func() {
		m.confirmSwitchAfterLeaveGuard(dir, files, target, remoteRoot, remote)
	})
}

func (m *Model) confirmSwitchAfterLeaveGuard(
	dir string,
	files []string,
	target openidea.RemoteTarget,
	remoteRoot string,
	remote bool,
) {
	if remote {
		m.openRemoteLocation(target, remoteRoot)
		return
	}
	if !m.hasDirty() {
		m.openProject(dir, files)
		return
	}
	detail := strings.Join(m.dirtyPaths(), "\n")
	go func() {
		res, err := mygo.Dialog.Message(mygo.MessageOptions{
			Parent: m.win, Type: mygo.MessageWarning, Title: "Switch Project",
			Message: "There are unsaved changes", Detail: detail,
			Buttons: []string{"Save and Switch", "Discard Changes", "Cancel"}, DefaultButton: 0, CancelButton: 2,
		})
		if err != nil {
			return
		}
		m.post(func() {
			switch res.Button {
			case 0:
				m.openProjectSavingWithFiles(dir, files)
			case 1:
				m.openProject(dir, files)
			}
		})
	}()
}

// OpenProject opens dir now, as the menu's recent entries do.
func (m *Model) OpenProject(dir string) { m.confirmSwitch(dir) }

func (m *Model) openProjectSaving(dir string) {
	m.openProjectSavingWithFiles(dir, nil)
}

func (m *Model) openProjectSavingWithFiles(dir string, files []string) {
	tabs := append([]*Tab(nil), m.tabs...)
	m.busy = true
	m.status = "Saving…"
	m.invalidate()
	go func() {
		if err := m.saveOpenTabs(m.ctx, tabs); err != nil {
			m.post(func() { m.fail("", err); m.busy = false })
			return
		}
		m.post(func() { m.openProject(dir, files) })
	}()
}

// resetProjectState drops everything the previous project put on screen —
// tabs, terminals, tool windows, searches and their caches — and reports
// whether Maven was visible, so a new project can load its build model.
func (m *Model) resetProjectState(dir string) (buildToolsVisible bool) {
	m.workspaceGeneration++
	m.pluginContentProjectReset(dir, m.workspaceGeneration)
	m.projectName = ""
	m.projectDisplayName = ""
	m.setProjectWorkspace(nil)
	m.clearProjectNotifications()
	m.resetEventRuntime()
	m.resetDocumentation()
	m.resetAnalysisState()
	m.coverage = coverageState{}
	m.recentNav = recentNavigation{}
	m.gotoLine = nil
	m.stackTrace = nil
	m.reveal = nil
	m.tabs, m.active = nil, 0
	m.openQueue = nil
	m.split, m.splitDown, m.splitFirst, m.focusQueued = nil, false, false, false
	m.banner, m.bannerPath = "", ""
	m.showingWelcome = false
	m.resetServices()
	m.resetGraphQL()
	m.resetDotnet()
	m.resetExternalLibraries()
	m.resetRun()
	m.resetDebug()
	m.resetBuild()
	m.resetTerm()
	m.resetVCS()
	m.resetVCSOperations()
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
	m.commitInitialMessage = m.loadCommitDraft(dir)
	m.vcsFileConfirm = nil
	m.cancelSearch()
	m.resetSearch()
	m.resetUsages()
	m.resetTodo()
	m.resetHierarchy()
	m.resetAgents()
	m.closeAllNotebooks()
	m.closeSymbols()
	m.closeIntents()
	m.closeQuickFiles()
	m.closeCommands()
	m.projectUnityVersion = ""
	m.projectTreeMembersLoading = nil
	m.diff = nil
	m.db = nil
	m.dbTable = nil
	m.dbHist = nil
	m.dbPlan = nil
	mavenVisible := m.maven.visible
	gradleVisible := m.maven.gradleVisible
	m.maven = mavenPane{visible: mavenVisible, gradleVisible: gradleVisible}
	m.refactor, m.redoRefactor = nil, nil
	return mavenVisible || gradleVisible
}

// CloseProject closes the open project and returns to Welcome; the app keeps
// running. Unsaved changes are asked about first, as a project switch does.
func (m *Model) CloseProject() {
	if m.busy {
		return
	}
	if m.Root() == "" {
		m.showWelcome()
		m.invalidate()
		return
	}
	m.requestProjectLeave(m.closeProjectAfterLeaveGuard)
}

func (m *Model) closeProjectAfterLeaveGuard() {
	if !m.hasDirty() {
		m.closeProjectNow()
		return
	}
	detail := strings.Join(m.dirtyPaths(), "\n")
	go func() {
		res, err := mygo.Dialog.Message(mygo.MessageOptions{
			Parent: m.win, Type: mygo.MessageWarning, Title: "Close Project",
			Message: "There are unsaved changes", Detail: detail,
			Buttons: []string{"Save and Close", "Discard Changes", "Cancel"}, DefaultButton: 0, CancelButton: 2,
		})
		if err != nil {
			return
		}
		m.post(func() {
			switch res.Button {
			case 0:
				m.closeProjectSaving()
			case 1:
				m.closeProjectNow()
			}
		})
	}()
}

func (m *Model) closeProjectSaving() {
	tabs := append([]*Tab(nil), m.tabs...)
	m.busy = true
	m.status = "Saving…"
	m.invalidate()
	go func() {
		if err := m.saveOpenTabs(m.ctx, tabs); err != nil {
			m.post(func() { m.fail("", err); m.busy = false })
			return
		}
		m.post(m.closeProjectNow)
	}()
}

// closeProjectNow verifies the backend can release the project before changing
// this window. A failed call leaves every tab, document and layout in place and
// offers an immediate retry.
func (m *Model) closeProjectNow() {
	if remoteConnected(m) {
		m.disconnectRemoteNow()
		return
	}
	prev := m.Root()
	if prev == "" {
		return
	}
	var oldTerms []uint32
	for gi := range m.term.groups {
		for _, tab := range m.term.groups[gi].tabs {
			if tab.id != 0 {
				oldTerms = append(oldTerms, tab.id)
			}
		}
	}
	if m.mgr == nil && m.invokeHook == nil {
		m.closeProjectCommitted(prev, oldTerms)
		return
	}
	m.busy = true
	m.status = "Closing " + filepath.Base(prev) + "…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.workspaceContext(), 30*time.Second)
		defer cancel()
		if _, err := m.invoke(ctx, "close_project", nil); err != nil {
			m.post(func() {
				m.busy = false
				message := "Closing the project in the backend failed: " + err.Error()
				m.banner = message
				m.bannerPath = ""
				retry := func(model *Model) {
					model.banner = ""
					model.bannerPath = ""
					model.invalidate()
					model.CloseProject()
				}
				m.notifyError(
					"project.close",
					"Could not close the project",
					notifySuffix(err.Error()),
					notifyAction{label: "Retry", run: retry},
				)
				m.cancelQuitPreparation()
				m.invalidate()
			})
			return
		}
		m.post(func() {
			m.closeProjectCommitted(prev, oldTerms)
		})
	}()
}

// closeProjectCommitted applies the local reset only after the backend has
// released the project.
func (m *Model) closeProjectCommitted(prev string, oldTerms []uint32) {
	if m.Root() != prev {
		return
	}
	if m.prefs.System.CloseWithoutWelcome && !m.applicationQuit && m.isLastProjectWindow() {
		if m.win != nil {
			m.finishWindowClose(false)
		} else {
			m.finishQuit(false)
		}
		return
	}
	if m.layouts == nil {
		m.layouts = map[string]Layout{}
	}
	m.layouts[layoutKey(prev)] = m.currentLayout()
	m.mcpProjectClosed()
	m.resetProjectState("")
	m.setRoot("")
	m.showingWelcome = true
	m.welcomePage = ""
	m.busy = false
	m.status = "Open a Java project"
	m.applyActive()
	m.saveSession()
	m.invalidate()
	if m.mgr == nil || len(oldTerms) == 0 {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for _, id := range oldTerms {
			_, _ = m.mgr.Invoke(ctx, Window, "term_kill", map[string]any{"id": id})
		}
	}()
}

// openProject switches the backend project and restores files, all in the
// background.
func (m *Model) openProject(dir string, files []string) {
	if remoteConnected(m) {
		m.switchRemoteProjectToLocal(dir, files)
		return
	}
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
	prevRoot := m.Root()
	old := m.allTabs()
	pinned := map[string]bool{}
	for _, p := range m.session().Pinned {
		pinned[realPath(p)] = true
	}
	// The second window group comes back with its files, its active tab and
	// its side, as the first group's files come back.
	sess := m.session()
	previews := map[string]bool{}
	for _, p := range sess.PreviewFiles {
		previews[realPath(p)] = true
	}
	splitPreviews := map[string]bool{}
	for _, p := range sess.SplitPreviewFiles {
		splitPreviews[realPath(p)] = true
	}
	var splitFiles []string
	for _, p := range sess.SplitFiles {
		p = realPath(p)
		if inProject(dir, p) {
			splitFiles = append(splitFiles, p)
		}
	}
	splitActive, splitDown, splitFirst := sess.SplitActive, sess.SplitDown, sess.SplitFirst
	var oldTerms []uint32
	for gi := range m.term.groups {
		tabs := m.term.groups[gi].tabs
		for ti := range tabs {
			if id := tabs[ti].id; id != 0 {
				oldTerms = append(oldTerms, id)
			}
		}
	}
	m.busy = true
	m.status = "Opening " + filepath.Base(dir) + "…"
	m.invalidate()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		projectInfo, err := m.openBackendProjectAt(ctx, dir, prevRoot)
		if err != nil {
			m.post(func() {
				m.busy = false
				name := "Welcome"
				if prevRoot != "" {
					name = filepath.Base(prevRoot)
				}
				m.status = "Opening " + filepath.Base(dir) + " failed; still in " + name
				m.banner = "Opening the project failed: " + err.Error()
				m.bannerPath = ""
				m.notifyProjectOpenFailure(dir, err)
				m.invalidate()
			})
			return
		}
		m.post(func() {
			m.startupPending = false
			if prevRoot != "" && layoutKey(prevRoot) != layoutKey(dir) {
				if m.layouts == nil {
					m.layouts = map[string]Layout{}
				}
				m.layouts[layoutKey(prevRoot)] = m.currentLayout()
			}
			if prevRoot != "" {
				m.mcpProjectClosed()
			}
			buildToolsVisible := m.resetProjectState(dir)
			m.setRoot(dir)
			if buildToolsVisible {
				m.loadBuildModel()
			}
			m.invalidate()
		})
		for _, id := range oldTerms {
			_, _ = m.mgr.Invoke(ctx, Window, "term_kill", map[string]any{"id": id})
		}
		_ = m.loadNativeRuntimeSettings(ctx)
		fileRuntime := m.refreshFileTypeRuntime(ctx)
		restoreFiles := append([]string(nil), files...)
		restoreSplitFiles := append([]string(nil), splitFiles...)
		files, notebookFiles := partitionNotebookPaths(files)
		splitFiles, splitNotebookFiles := partitionNotebookPaths(splitFiles)
		files, associated := partitionAssociatedPaths(files, fileRuntime.AssociatedApplications)
		if len(associated) > 0 {
			m.post(func() {
				for _, path := range associated {
					if err := mygo.Shell.OpenPath(path); err != nil {
						m.fail(path, err)
					}
				}
			})
		}
		for _, t := range old {
			if t.kind == "" {
				_ = m.reg.Close(ctx, t.doc.Path())
			}
		}
		m.post(func() { m.setStartPhase("restoring") })
		var docs []*docbridge.Document
		docByPath := make(map[string]*docbridge.Document, len(files))
		var failed []string
		for _, p := range files {
			doc, err := m.reg.Open(ctx, p)
			if err != nil {
				failed = append(failed, filepath.Base(p))
				continue
			}
			docs = append(docs, doc)
			docByPath[realPath(p)] = doc
		}
		var splitDocs []*docbridge.Document
		splitDocByPath := make(map[string]*docbridge.Document, len(splitFiles))
		for _, p := range splitFiles {
			doc, err := m.reg.Open(ctx, p)
			if err != nil {
				failed = append(failed, filepath.Base(p))
				continue
			}
			splitDocs = append(splitDocs, doc)
			splitDocByPath[realPath(p)] = doc
		}
		focus := m.session().Focus
		if focus != "" {
			focus = realPath(focus)
		}
		m.post(func() {
			m.mcpProjectReady()
			m.clearDiagnostics()
			m.rename = nil
			m.tabs = m.tabs[:0]
			var restoredNotebooks []*notebookSession
			for _, path := range restoreFiles {
				if isNotebookPath(path) {
					if slices.Contains(notebookFiles, realPath(path)) || slices.Contains(notebookFiles, path) {
						m.tabs = append(m.tabs, m.newNotebookTab(path, filepath.Base(path), false))
						restoredNotebooks = append(restoredNotebooks, m.notebook(path))
					}
					continue
				}
				if doc := docByPath[realPath(path)]; doc != nil {
					m.tabs = append(m.tabs, m.newTab(doc))
				}
			}
			for _, taskID := range uniqueAgentIDs(sess.AgentTasks) {
				m.tabs = append(m.tabs, m.newAgentPageTab(
					tabKindAgentSession, agentSessionPath(taskID), "Agent Task", taskID))
			}
			if sess.AgentHome {
				m.tabs = append(m.tabs, m.newAgentPageTab(
					tabKindAgentHome, "agent:new", "New Agent Task", ""))
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
			restorePreviewFlags(m.tabs, pinned, previews)
			m.active = 0
			for i, t := range m.tabs {
				if t.doc.Path() == focus {
					m.active = i
				}
			}
			for i, t := range m.tabs {
				if t.kind == tabKindAgentSession && t.agentTask == sess.FocusAgent {
					m.active = i
					break
				}
				if t.kind == tabKindAgentHome && sess.FocusAgentNew {
					m.active = i
					break
				}
			}
			// The second window group comes back with its tabs, side and
			// stacked or side-by-side orientation.
			if len(splitDocs) > 0 || len(splitNotebookFiles) > 0 {
				s := &splitGroup{}
				for _, path := range restoreSplitFiles {
					if isNotebookPath(path) {
						if slices.Contains(splitNotebookFiles, realPath(path)) || slices.Contains(splitNotebookFiles, path) {
							tab := m.newNotebookTab(path, filepath.Base(path), false)
							s.tabs = append(s.tabs, tab)
							restoredNotebooks = append(restoredNotebooks, m.notebook(path))
						}
						continue
					}
					if doc := splitDocByPath[realPath(path)]; doc != nil {
						s.tabs = append(s.tabs, m.newTab(doc))
					}
				}
				if len(s.tabs) > 0 {
					restorePreviewFlags(s.tabs, nil, splitPreviews)
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
			}
			if m.tree != nil {
				m.tree.Close()
			}
			m.tree = NewTree(dir, m.post, m.invalidate)
			m.tree.SetRemovedHandler(func(paths []string) {
				for _, path := range paths {
					m.removeNavigationTree(path)
				}
			})
			m.setProjectInfo(projectInfo)
			m.configureProjectTreeView()
			m.busy = false
			m.status = fmt.Sprintf("%s · %d files opened", filepath.Base(dir), len(m.tabs))
			for _, session := range restoredNotebooks {
				if session != nil && !session.loaded && !session.loading {
					m.loadNotebook(session)
				}
			}
			if len(failed) > 0 {
				m.banner = "Could not open: " + strings.Join(failed, ", ")
			}
			m.applyActive()
			m.applyLayout(dir)
			m.rememberProject(dir)
			m.saveSession()
			m.checkTrust(dir)
			m.refreshReadiness()
			m.RefreshLanguageServers()
			m.RefreshVCS()
			m.RefreshBranch()
			m.connectAgents(false)
			m.setStartPhase("ready")
			m.invalidate()
		})
	}()
}

func (m *Model) openBackendProject(ctx context.Context, dir string) error {
	info, err := m.openBackendProjectAt(ctx, dir, "")
	if err == nil {
		m.post(func() { m.setProjectInfo(info) })
	}
	return err
}

// openBackendProjectAt opens dir and waits until the backend's project_current
// reports that exact root, so a project that fails to open cannot leave the UI
// on a project the backend does not have. On failure restoreTo, the project
// that was open, is opened again.
func (m *Model) openBackendProjectAt(ctx context.Context, dir, restoreTo string) (openidea.ProjectInfo, error) {
	if _, err := m.mgr.Invoke(ctx, Window, "open_project", map[string]any{"path": dir}); err != nil {
		m.restoreBackendProject(ctx, restoreTo, dir)
		return openidea.ProjectInfo{}, err
	}
	deadline := time.Now().Add(60 * time.Second)
	for {
		raw, err := m.mgr.Invoke(ctx, Window, "project_current", nil)
		if err == nil {
			if info, decodeErr := openidea.DecodeProjectInfo(raw); decodeErr == nil &&
				info.Root != "" && layoutKey(info.Root) == layoutKey(dir) {
				return info, nil
			}
		}
		if time.Now().After(deadline) {
			m.restoreBackendProject(ctx, restoreTo, dir)
			return openidea.ProjectInfo{}, fmt.Errorf(
				"The GoIDEA backend did not make %s the current project within 60 seconds",
				filepath.Base(dir))
		}
		select {
		case <-ctx.Done():
			return openidea.ProjectInfo{}, ctx.Err()
		case <-time.After(60 * time.Millisecond):
		}
	}
}

// projectRootOf reads the root of a project_current reply.
func projectRootOf(raw []byte) (string, bool) {
	var info struct {
		Root string `json:"root"`
	}
	if json.Unmarshal(raw, &info) != nil || info.Root == "" {
		return "", false
	}
	return info.Root, true
}

// restoreBackendProject reopens the project that was current before a failed
// switch, best effort: the UI reports the failure either way.
func (m *Model) restoreBackendProject(ctx context.Context, root, failed string) {
	if root == "" || layoutKey(root) == layoutKey(failed) || m.mgr == nil {
		return
	}
	if _, err := m.mgr.Invoke(ctx, Window, "open_project", map[string]any{"path": root}); err != nil {
		return
	}
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); {
		raw, err := m.mgr.Invoke(ctx, Window, "project_current", nil)
		if err == nil {
			if got, ok := projectRootOf(raw); ok && layoutKey(got) == layoutKey(root) {
				return
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(80 * time.Millisecond):
		}
	}
}

func (m *Model) newTab(doc *docbridge.Document) *Tab {
	v := doc.Buffer().Version()
	path := doc.Path()
	fontSize := m.editorFontSize()
	font := m.prefs.Font
	t := &Tab{
		owner: m,
		doc:   doc,
		state: &editor.State{
			Buffer:     doc.Buffer(),
			LineHeight: font.lineHeightPixels(fontSize) / fontSize,
			Bindings:   m.editorBindings(),
			ReadOnly:   doc.Info().ReadOnly,
			Theme:      m.editorColorSchemeTheme(),
		},
		synced: v,
		seen:   v,
	}
	t.state.Font = ui.Font{
		Family:   font.fontFamily(),
		Size:     fontSize,
		Weight:   font.weight(),
		Features: font.features(),
	}
	t.state.Buffer.SetLanguage(m.fileTypeLanguageFor(path))
	t.state.GutterMarks = m.coverageGutterMarksForTab(t)
	m.applyTabZenGutter(t)
	m.applyTabFoldingDefaults(t, m.prefs.Editor)
	m.noteTabUse(t)
	t.state.OnGutterClick = func(line int) {
		if m.pluginMarkerClick(t, line) {
			return
		}
		m.toggleBreakpointAt(path, line)
	}
	t.state.OnCompletion = func(req editor.CompletionRequest) { m.requestCompletion(t, req) }
	t.state.RenderCompletions = func(c *ui.Context, panel ui.Element, menu *editor.CompletionMenu) {
		m.renderedCompletion(c, panel, menu)
	}
	t.state.OnHover = func(req editor.HoverRequest) {
		if m.pluginDecorationHover(t, req) {
			return
		}
		m.requestEditorHover(t, req)
	}
	t.state.OnDefinition = func(off int) { m.gotoDefinition(t, off) }
	t.state.ContextMenu = func(menu *ui.Menu) { m.editorMenu(menu, t) }
	t.state.OnInput = func(ev ui.InputEvent) bool { return m.editorCaretInput(t, ev) }
	m.applyVimToTab(t)
	t.state.NoAutoClose = true
	m.showDiagnostics(t)
	m.noteTabDisk(t)
	m.markRecent(path)
	return t
}

// openFile opens path in a regular tab, or shows the tab that has it.
func (m *Model) openFile(path string) { m.openFileWith(path, false) }

// openFileThen opens path and invokes done after its tab is active. Project
// actions use this so "open first" variants never act on the previously active
// document while the requested file is still loading.
func (m *Model) openFileThen(path string, done func(*Tab)) {
	m.openWith(path, openSpec{at: -1, done: done})
}

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
	done    func(*Tab)
	failed  func(string, error)
}

type queuedOpen struct {
	path string
	spec openSpec
}

// openWith is openFileWith with the place the document goes in the strip. A
// regular open closes the preview tab, as openTab does, and an open that
// replaces it keeps the preview slot.
func (m *Model) openWith(path string, spec openSpec) {
	m.recordNavigation(true)
	m.observeRecentLocation(time.Now(), true)
	preview := spec.preview && m.tabPrefs.previews()
	path = realPath(path)
	if i := m.tabForPath(path); i >= 0 {
		m.showingWelcome = false
		if !preview {
			m.promoteTab(m.tabs[i])
		}
		m.activate(i)
		m.finishOpen(spec, m.tabs[i])
		m.pumpOpenQueue()
		return
	}
	if m.split != nil {
		for i, t := range m.split.tabs {
			if t.doc.Path() == path {
				m.showingWelcome = false
				if !preview {
					m.promoteTab(t)
				}
				m.focusGroup(groupSplit)
				m.activateIn(groupFocused, i)
				m.finishOpen(spec, t)
				m.pumpOpenQueue()
				return
			}
		}
	}
	if isNotebookPath(path) {
		m.openNotebook(path, spec)
		return
	}
	if imageViewerPath(path) {
		m.openImageViewer(path, spec)
		return
	}
	if hex, size := wantsHexViewer(path); hex {
		m.openHexViewer(path, size, spec)
		return
	}
	if m.busy {
		m.openQueue = append(m.openQueue, queuedOpen{path: path, spec: spec})
		m.status = fmt.Sprintf("Opening %d files…", len(m.openQueue)+1)
		m.invalidate()
		return
	}
	m.busy = true
	m.status = "Opening " + filepath.Base(path) + "…"
	m.invalidate()
	if m.reg == nil {
		m.busy = false
		err := fmt.Errorf("goidea: document registry is unavailable")
		m.fail(path, err)
		if spec.failed != nil {
			spec.failed(path, err)
		}
		m.pumpOpenQueue()
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		doc, err := m.reg.Open(ctx, path)
		m.post(func() {
			m.busy = false
			if err != nil {
				m.fail(path, err)
				if spec.failed != nil {
					spec.failed(path, err)
				}
				m.pumpOpenQueue()
				return
			}
			if m.isExternalLibraryPath(path) {
				doc.SetReadOnly(true)
			}
			m.showingWelcome = false
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
			m.applyActive()
			m.applyReveal(m.tabs[m.active])
			m.syncProjectTreeState()
			if m.prefs.ProjectTree.AutoscrollFromSource {
				m.followActiveInProjectView(true)
			}
			// A jump requested before the document was ready (a test tree's
			// failing node) lands once its tab exists.
			if line, ok := m.pendingJump[path]; ok {
				delete(m.pendingJump, path)
				m.tabs[m.active].state.GoTo(editor.Pos{Line: line, Col: 0})
			}
			m.status = "Opened " + filepath.Base(path)
			m.applyActive()
			m.saveSession()
			m.invalidate()
			m.finishOpen(spec, tab)
			m.pumpOpenQueue()
		})
	}()
}

func (m *Model) pumpOpenQueue() {
	if m.busy || len(m.openQueue) == 0 {
		return
	}
	next := m.openQueue[0]
	copy(m.openQueue, m.openQueue[1:])
	m.openQueue = m.openQueue[:len(m.openQueue)-1]
	m.openWith(next.path, next.spec)
}

func (m *Model) finishOpen(spec openSpec, tab *Tab) {
	if spec.done != nil && tab != nil && m.tabOpen(tab) {
		spec.done(tab)
	}
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
	m.recordNavigation(true)
	m.observeRecentLocation(time.Now(), true)
	if g == groupSplit {
		m.focusGroup(groupSplit)
	}
	if i < 0 || i >= len(m.tabs) || i == m.active {
		return
	}
	next := m.tabs[i]
	if next.kind == tabKindAgentSession {
		if _, ok := m.agents.tasks[next.agentTask]; ok {
			m.switchAgentComposer(next.agentTask)
			m.agents.conversation = true
			m.agents.selectedSubagents = nil
			delete(m.agents.unread, next.agentTask)
			m.loadAgentEntries(next.agentTask, false)
		}
	}
	m.revealEditor()
	m.active = i
	m.noteTabUse(m.tabs[i])
	m.applyActive()
	m.applyReveal(m.tabs[i])
	m.recordNavigation(true)
	m.syncProjectTreeState()
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
		if m.prefs.Appearance.FullPathInTitle {
			name = t.doc.Path()
		}
		if t.kind != "" {
			name = m.agentEditorTitle(t)
			if t.kind == tabKindNotebook && t.pageTitle != "" {
				name = t.pageTitle
			}
			if t.kind == tabKindPluginContent {
				name = m.pluginContentTitle(t)
			}
			if name == "" {
				name = t.doc.Path()
			}
			m.win.SetTitle(m.windowTitle(name))
			m.win.SetRepresentedFile("")
			m.win.SetDocumentEdited(
				(t.kind == tabKindNotebook && t.pageDirty) ||
					(t.kind == tabKindPluginContent && t.unsaved()),
			)
			return
		}
		m.win.SetTitle(m.windowTitle(name))
		m.win.SetRepresentedFile(t.doc.Path())
		m.win.SetDocumentEdited(t.dirty())
		return
	}
	m.win.SetTitle(m.windowTitle(""))
	m.win.SetRepresentedFile("")
	m.win.SetDocumentEdited(false)
}

// windowTitle follows OpenIDEA's `<project> – <file>` document title. Branding
// is only used when there is neither a project nor a document.
func (m *Model) windowTitle(fileName string) string {
	file := strings.TrimSpace(fileName)
	project := m.projectLabel()
	if m.Root() == "" {
		if file != "" {
			return file
		}
		return project
	}
	if file != "" {
		return project + " – " + file
	}
	return project
}

func (m *Model) dropTab(i int) { m.removeTab(m.tabs[i]) }

// removeTab closes t and puts it in the closed history Reopen Closed Tab pops.
func (m *Model) removeTab(t *Tab) {
	if t != nil && t.kind == tabKindNotebook {
		m.requestCloseNotebook(t)
		return
	}
	if t != nil && t.kind == tabKindPluginContent {
		if session := m.pluginContentTabFor(t); session != nil {
			pluginContents.remove(session)
		}
		m.removeTabNow(t)
		return
	}
	if g, ok := m.groupOf(t); ok && t.doc != nil && t.kind == "" {
		entry := closedTab{
			path: t.doc.Path(), index: tabIndexIn(m.groupTabsAt(g), t),
			pinned: t.pinned, doc: t.doc,
		}
		if t.state != nil {
			entry.selections = t.state.Selections()
			entry.scrollY = t.state.ScrollY()
		}
		m.rememberClosed(entry)
		m.removeTabFromWindow(t, false)
		return
	}
	if m.diff != nil && m.diff.tab == t {
		m.diff = nil
	}
	m.removeTabNow(t)
}

// removeTabNow takes t out of its window group without the closed history, as
// the preview trim and the preview slot's reuse do.
func (m *Model) removeTabNow(t *Tab) {
	m.removeTabFromWindow(t, true)
}

// removeTabFromWindow removes t from its strip. A file tab kept for Reopen
// Closed Tab keeps its registry reference, and with it the shared buffer's
// undo history, until the closed entry is reopened or evicted.
func (m *Model) removeTabFromWindow(t *Tab, closeDoc bool) {
	g, ok := m.groupOf(t)
	if !ok {
		return
	}
	m.releaseDockerLogTab(t)
	m.removeFromGroup(g, t)
	m.vimTabClosed(t)
	if m.rename != nil && m.rename.tab == t {
		m.rename = nil
	}
	m.settleGroups()
	if closeDoc && t.doc != nil && t.kind == "" {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_ = m.reg.Close(ctx, t.doc.Path())
		}()
	}
	if len(m.tabs) == 0 {
		m.active = 0
	}
	if m.Root() == "" && len(m.allTabs()) == 0 {
		// The final editor tab follows the window close policy: the last
		// session returns to Welcome, while a non-last window closes and an
		// explicit closeWithoutWelcome keeps its normal close path.
		if !m.returnLastSessionToWelcome() && !m.isLastProjectWindow() {
			if m.win != nil {
				m.finishWindowClose(false)
			}
			return
		}
		m.showingWelcome = true
		m.status = "Open a file"
	}
	m.applyActive()
	m.syncProjectTreeState()
	m.saveSession()
	m.invalidate()
}

// SaveActive saves the active document.
func (m *Model) SaveActive() {
	if t := m.tab(); t != nil && t.kind == tabKindNotebook {
		m.saveNotebook(t)
		return
	}
	if t := m.tab(); t != nil && t.kind == tabKindPluginContent {
		if session := m.pluginContentTabFor(t); session != nil {
			m.savePluginContentThen(session, false, nil)
		}
		return
	}
	if t := m.tab(); t != nil && t.kind == "" {
		m.saveTabAsyncMode(t, true, nil)
	}
}

func (m *Model) saveTabAsync(t *Tab, done func()) {
	m.saveTabAsyncMode(t, false, done)
}

func (m *Model) saveTabAsyncMode(t *Tab, explicit bool, done func()) {
	if t.saving {
		return
	}
	t.saving = true
	m.status = "Saving " + filepath.Base(t.doc.Path()) + "…"
	m.invalidate()
	go func() {
		// Actions on Save run before formatting, then Save Files cleans the
		// document, as OpenIDEA's save participants order them.
		err := m.prepareTabForSave(m.ctx, t)
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
				m.noteTabDisk(t)
				m.status = "Saved " + filepath.Base(t.doc.Path())
				m.deploymentAutoUploadAfterSave(t, explicit)
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
		if t != nil && t.kind == tabKindPluginContent && t.owner != nil && t.unsaved() {
			if err := t.owner.pluginContentSaveForLifecycle(ctx, t); err != nil && first == nil {
				first = err
			}
			continue
		}
		if t == nil || t.kind != "" || t.doc == nil || !t.unsaved() {
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
		if err := t.doc.Save(ctx); err != nil {
			if first == nil {
				first = err
			}
		}
	}
	return first
}

// saveOpenTabs is the user-facing save pipeline: Format on Save runs first,
// then Save Files cleans the document while keeping the caret line when asked,
// and only then is the result synced and written.
func (m *Model) saveOpenTabs(ctx context.Context, tabs []*Tab) error {
	var first error
	for _, t := range tabs {
		if t != nil && t.kind == tabKindNotebook {
			if err := m.saveNotebookSync(ctx, t); err != nil && first == nil {
				first = err
			}
			continue
		}
		if t != nil && t.kind == tabKindPluginContent {
			if err := m.pluginContentSaveForLifecycle(ctx, t); err != nil && first == nil {
				first = err
			}
			continue
		}
		if t == nil || t.kind != "" || t.doc == nil || !t.unsaved() {
			continue
		}
		if err := m.prepareTabForSave(ctx, t); err != nil {
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

// prepareTabForSave runs Actions on Save, formatting and Save Files cleanup
// in OpenIDEA's order, then computes the caret line the Keep trailing spaces
// option protects.
func (m *Model) prepareTabForSave(ctx context.Context, t *Tab) error {
	if err := m.applyActionsOnSave(ctx, t); err != nil {
		return err
	}
	if m.prefs.General.ReformatOnSave &&
		(m.mgr != nil || m.invokeHook != nil) {
		if t.formatting {
			return fmt.Errorf("Cannot save %s while it is already being reformatted", filepath.Base(t.doc.Path()))
		}
		version := t.doc.Buffer().Version()
		_, stale, err := m.applyLanguageFormat(
			ctx, t, t.doc.ID(), version, true, m.formatGuardForDocument(),
		)
		switch {
		case err != nil:
			return fmt.Errorf("Formatting %s failed: %w", filepath.Base(t.doc.Path()), err)
		case stale:
			return fmt.Errorf("%s changed while it was being formatted; save it again",
				filepath.Base(t.doc.Path()))
		}
	}
	p := m.prefs.SaveFiles
	caretLine := -1
	keepCaretLine := p.KeepTrailingOnCaretLine
	if keepCaretLine && t.state != nil {
		caretLine = t.state.CursorPos().Line
	}
	return cleanForSaveAtCaret(ctx, t.doc, p, caretLine, keepCaretLine)
}

// syncPump pushes editor buffer changes to the backend a moment after typing
// stops. It runs once per frame; the backend calls go to other goroutines.
func (m *Model) syncPump(now time.Time) {
	m.watchFiles(now)
	m.maybeSaveIdle(now)
	m.observeRecentLocation(now, false)
	m.recordNavigation(false)
	m.revalidatePendingGraphQLSchemas(now)
	const idle = 220 * time.Millisecond
	for _, t := range m.tabs {
		v := t.doc.Buffer().Version()
		if v != t.seen {
			t.seen = v
			t.edited = now
			// An edit promotes the preview tab, as the controller does.
			m.promoteTab(t)
			m.pluginEditorChanged(t, now)
			m.refreshTabDecorations(t)
			m.noteGraphQLSchemaEdit(t, now)
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
	m.syncPluginVisuals(now)
}

// dirty reports whether the buffer holds text the backend has not been given
// or has not saved.
func (t *Tab) dirty() bool {
	return t.syncing || t.saving || t.doc.Buffer().Version() != t.synced || t.doc.Dirty()
}

// unsaved reports whether the buffer's text differs from the file on disk, so
// undoing back to the saved text stops the quit prompt.
func (t *Tab) unsaved() bool {
	if t == nil {
		return false
	}
	if t.kind == tabKindPluginContent && t.owner != nil {
		return t.owner.pluginContentDirty(t)
	}
	if t.kind == tabKindNotebook {
		return t.pageDirty
	}
	return t.doc.ModifiedFromSaved()
}

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
			name := t.doc.Path()
			if t.kind == tabKindPluginContent {
				name = m.pluginContentTitle(t)
			}
			out = append(out, name)
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
			m.status = "Conflict resolved: " + filepath.Base(path)
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
	const marker = "// smoke world 😀\n"
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
	fileTabs := 0
	for _, t := range m.tabs {
		if t.kind == "" || t.kind == tabKindNotebook {
			fileTabs++
		}
	}
	if root == "" && fileTabs == 0 && len(m.recents) == 0 && len(m.recentGroups) == 0 &&
		len(m.searchHistory) == 0 && len(m.searchHistories) == 0 && len(m.notify.items) == 0 &&
		len(m.namedLayouts) == 0 && m.activeNamedLayout == "" {
		// Nothing to remember; keep the previous session rather than
		// writing an empty one after a failed start.
		return Session{}, false
	}
	var files, pinned, previews, agentTasks []string
	focus := ""
	agentHome := false
	focusAgent := ""
	focusAgentNew := false
	for _, t := range m.tabs {
		if t.kind != "" && t.kind != tabKindNotebook {
			switch t.kind {
			case tabKindAgentSession:
				agentTasks = append(agentTasks, t.agentTask)
			case tabKindAgentHome:
				agentHome = true
			}
			continue
		}
		files = append(files, t.doc.Path())
		if t.pinned {
			pinned = append(pinned, t.doc.Path())
		}
		if t.preview {
			previews = append(previews, t.doc.Path())
		}
	}
	if t := m.tab(); t != nil && (t.kind == "" || t.kind == tabKindNotebook) {
		focus = t.doc.Path()
	} else if t := m.tab(); t != nil {
		if t.kind == tabKindAgentSession {
			focusAgent = t.agentTask
		} else if t.kind == tabKindAgentHome {
			focusAgentNew = true
		}
	}
	var splitFiles, splitPreviews []string
	splitActive := 0
	if m.split != nil {
		for _, t := range m.split.tabs {
			if t.kind != "" && t.kind != tabKindNotebook {
				continue
			}
			splitFiles = append(splitFiles, t.doc.Path())
			if t.preview {
				splitPreviews = append(splitPreviews, t.doc.Path())
			}
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
	keepPaths := make([]string, 0, len(m.recents)+1)
	keepPaths = append(keepPaths, root)
	for _, p := range m.recents {
		keepPaths = append(keepPaths, p.Path)
	}
	keep := keepPaths
	var remoteSession *openidea.RemoteSession
	if m.remote.session != nil {
		copied := *m.remote.session
		copied.Commands = append([]string(nil), copied.Commands...)
		remoteSession = &copied
	}
	return Session{Root: root, Remote: remoteSession, Files: normalizeFiles(files), Focus: focus,
		Pinned:            pinned,
		PreviewFiles:      normalizeFiles(previews),
		AgentTasks:        uniqueAgentIDs(agentTasks),
		AgentHome:         agentHome,
		FocusAgent:        focusAgent,
		FocusAgentNew:     focusAgentNew,
		SplitFiles:        normalizeFiles(splitFiles),
		SplitActive:       splitActive,
		SplitDown:         m.splitDown,
		SplitFirst:        m.splitFirst,
		SplitPreviewFiles: normalizeFiles(splitPreviews),
		Bookmarks:         append([]Bookmark(nil), m.bookmarks...),
		PathBookmarks:     append([]PathBookmark(nil), m.pathBookmarks...),
		BookmarkLists:     append([]BookmarkList(nil), m.bookmarkLists...),
		DefaultList:       m.defaultList,
		RecentProjects:    recentPaths(m.recents),
		Recents:           append([]RecentProject(nil), m.recents...),
		RecentGroups:      append([]RecentGroup(nil), m.recentGroups...),
		SearchHistory:     append([]string(nil), m.searchHistories[seTabAll]...),
		SearchHistories:   cloneSearchHistories(m.searchHistories),
		LogcatHistory:     append([]string(nil), m.logcat.history...),
		SearchScopes:      append([]SearchScope(nil), m.searchScopes...),
		Notifications:     m.notificationHistory(),
		Layouts:           pruneLayouts(layouts, keep),
		NamedLayouts:      cloneSavedToolWindowLayouts(m.namedLayouts),
		ActiveNamedLayout: m.activeNamedLayout,
		ProjectViews:      cloneProjectViews(m.projectViews),
		RunHistory:        pruneRunHistory(cloneRunHistory(m.runHistory), keep)}, true
}

func cloneProjectViews(views map[string][]string) map[string][]string {
	if len(views) == 0 {
		return nil
	}
	out := make(map[string][]string, len(views))
	for root, expanded := range views {
		if len(expanded) > maxProjectTreeExpanded {
			expanded = expanded[:maxProjectTreeExpanded]
		}
		out[root] = append([]string(nil), expanded...)
	}
	return out
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
	if m.closing {
		m.sessionMu.Unlock()
		return
	}
	m.sessionSeq++
	seq := m.sessionSeq
	m.sessionWG.Add(1)
	m.sessionMu.Unlock()
	path := m.cfg.SessionPath
	go func() {
		defer m.sessionWG.Done()
		m.writeSession(seq, path, s)
	}()
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

// off stops background work and waits for queued session writes, so closing
// the model cannot race a temporary data directory or truncate the last save.
func (m *Model) off() {
	if m.cancel != nil {
		m.cancel()
	}
	m.sessionMu.Lock()
	m.closing = true
	m.sessionMu.Unlock()
	m.sessionWG.Wait()
}

// RequestQuit asks about live processes, unsaved tabs, and then closes the
// backend. The process guard runs first so cancelling it preserves every
// document, including a dirty one that would otherwise open a second prompt.
func (m *Model) RequestQuit(e *mygo.QuitEvent) { m.requestQuit(e.PreventDefault) }

func (m *Model) requestQuit(prevent func()) {
	if m.forceQuit {
		return
	}
	prevent()
	if m.workspaceHost != nil {
		m.requestApplicationQuit()
		return
	}
	if m.quitting {
		return
	}
	m.quitting = true
	m.applicationQuit = true
	m.requestRunningWorkTermination(m.quitAfterProcessGuard, func() {
		m.cancelQuitPreparation()
	})
}

func (m *Model) quitAfterProcessGuard() {
	if m.workspaceHost != nil {
		m.requestApplicationQuit()
		return
	}
	m.applicationQuit = false
	if !m.hasDirty() {
		m.confirmQuitWithoutChanges()
		return
	}
	detail := strings.Join(m.dirtyPaths(), "\n")
	go func() {
		res, err := mygo.Dialog.Message(mygo.MessageOptions{
			Parent: m.win, Type: mygo.MessageWarning, Title: "Quit GoIDEA",
			Message: "There are unsaved changes. Save before quitting?", Detail: detail,
			Buttons: []string{"Save and Quit", "Don't Save", "Cancel"}, DefaultButton: 0, CancelButton: 2,
		})
		if err != nil {
			m.post(func() {
				m.quitting = false
				m.applicationQuit = false
				m.restartPending = false
			})
			return
		}
		m.post(func() {
			m.quitting = false
			m.applicationQuit = false
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

// requestWindowClose is the close-button path. It closes only this native
// window after its own process and unsaved-document guards pass.
func (m *Model) requestWindowClose(prevent func()) {
	if m.forceQuit {
		return
	}
	prevent()
	if m.quitting {
		return
	}
	m.quitting = true
	m.applicationQuit = false
	closeWindow := func() {
		m.requestRunningWorkTermination(m.windowCloseAfterProcessGuard, m.cancelQuitPreparation)
	}
	if m.returnsToWelcomeOnWindowClose() {
		closeWindow()
		return
	}
	if !m.isLastProjectWindow() {
		closeWindow()
		return
	}
	m.confirmWindowExit(func(ok bool) {
		if !ok {
			m.cancelQuitPreparation()
			return
		}
		closeWindow()
	})
}

// returnsToWelcomeOnWindowClose is OpenIDEA's returnsToWelcome: a last window
// with a project or LightEdit session goes back to Welcome unless the user
// explicitly chose to close it.
func (m *Model) returnsToWelcomeOnWindowClose() bool {
	return m != nil && !m.applicationQuit && m.isLastProjectWindow() &&
		m.hasWindowSession() && !m.prefs.System.CloseWithoutWelcome
}

func (m *Model) hasWindowSession() bool {
	return m != nil && (m.Root() != "" || len(m.allTabs()) > 0)
}

// confirmWindowExit is decideClose's last-window confirmation. Choosing Don't
// ask again persists general.confirmExit=false, as OpenIDEA does.
func (m *Model) confirmWindowExit(done func(bool)) {
	if done == nil {
		return
	}
	if !m.prefs.System.confirmExit() {
		done(true)
		return
	}
	answer := func(confirmed, dontAsk bool) {
		if confirmed && dontAsk {
			m.prefs.System.ConfirmExit = boolPref(false)
			if err := m.savePreferences(); err != nil {
				m.status = "Saving settings failed: " + err.Error()
			}
		}
		done(confirmed)
	}
	if m.confirmExitHook != nil {
		m.confirmExitHook(answer)
		return
	}
	go func() {
		res, err := mygo.Dialog.Message(mygo.MessageOptions{
			Parent: m.win, Type: mygo.MessageWarning, Title: "Confirm Exit",
			Message: "Are you sure you want to exit GoIDEA?",
			Buttons: []string{"Exit", "Cancel"}, DefaultButton: 0, CancelButton: 1,
			CheckboxLabel:   "Don't ask again",
			CheckboxChecked: false,
		})
		if err != nil {
			m.post(func() { done(false) })
			return
		}
		m.post(func() { answer(res.Button == 0, res.CheckboxChecked) })
	}()
}

func (m *Model) windowCloseAfterProcessGuard() {
	m.applicationQuit = false
	if !m.hasDirty() {
		if !m.returnLastSessionToWelcome() {
			m.finishWindowClose(false)
		} else {
			m.cancelQuitPreparation()
		}
		return
	}
	detail := strings.Join(m.dirtyPaths(), "\n")
	go func() {
		res, err := mygo.Dialog.Message(mygo.MessageOptions{
			Parent: m.win, Type: mygo.MessageWarning, Title: "Close Window",
			Message: "There are unsaved changes. Save before closing this window?", Detail: detail,
			Buttons: []string{"Save and Close", "Don't Save", "Cancel"}, DefaultButton: 0, CancelButton: 2,
		})
		if err != nil {
			m.post(m.cancelQuitPreparation)
			return
		}
		m.post(func() {
			switch res.Button {
			case 0:
				if !m.returnLastSessionToWelcome() {
					m.finishWindowClose(true)
				} else {
					m.cancelQuitPreparation()
				}
			case 1:
				if !m.returnLastSessionToWelcome() {
					m.finishWindowClose(false)
				} else {
					m.cancelQuitPreparation()
				}
			default:
				m.cancelQuitPreparation()
			}
		})
	}()
}

func (m *Model) finishWindowClose(save bool) {
	m.shutdown(save, func() {
		if m.win != nil {
			m.win.Destroy()
		}
	})
}

func (m *Model) cancelQuitPreparation() {
	if m.forceQuit {
		return
	}
	m.quitting = false
	m.applicationQuit = false
	m.restartPending = false
}

// PrepareApplicationQuit performs this window's process and unsaved-document
// checks but leaves its backend running. The window host calls this for every
// window before it commits the application quit.
func (m *Model) PrepareApplicationQuit(done func(save, ok bool)) {
	if done == nil {
		return
	}
	if m.forceQuit {
		done(false, true)
		return
	}
	if m.quitting {
		done(false, false)
		return
	}
	m.quitting = true
	m.applicationQuit = true
	m.requestRunningWorkTermination(func() {
		if !m.hasDirty() {
			done(false, true)
			return
		}
		detail := strings.Join(m.dirtyPaths(), "\n")
		go func() {
			res, err := mygo.Dialog.Message(mygo.MessageOptions{
				Parent: m.win, Type: mygo.MessageWarning, Title: "Quit GoIDEA",
				Message: "There are unsaved changes. Save before quitting?", Detail: detail,
				Buttons: []string{"Save and Quit", "Don't Save", "Cancel"}, DefaultButton: 0, CancelButton: 2,
			})
			if err != nil {
				m.post(func() { done(false, false) })
				return
			}
			m.post(func() {
				if res.Button == 2 || res.Button < 0 {
					done(false, false)
					return
				}
				done(res.Button == 0, true)
			})
		}()
	}, func() {
		m.cancelQuitPreparation()
		done(false, false)
	})
}

// CancelApplicationQuitPreparation releases a window whose quit prompt this
// run did not commit, so cancellation of any later window leaves all windows
// usable.
func (m *Model) CancelApplicationQuitPreparation() {
	m.cancelQuitPreparation()
}

// ConfirmApplicationExit shows the one application-wide confirmation after
// every window has passed its own checks.
func (m *Model) ConfirmApplicationExit(done func(ok bool)) {
	m.confirmWindowExit(done)
}

// ShutdownForApplicationQuit closes only this window's resources. The host
// calls it after every window's prompts succeeded, then quits or relaunches.
func (m *Model) ShutdownForApplicationQuit(save bool, done func()) {
	m.shutdown(save, done)
}

// shutdown tears down one model and backend without deciding whether the
// application quits or whether another native window stays open.
func (m *Model) shutdown(save bool, done func()) {
	if m.forceQuit {
		if done != nil {
			done()
		}
		return
	}
	m.mcpStopReports()
	if m.prefs.System.CloseTabsWithWindow {
		m.saveSessionWithoutEditorTabs()
	} else {
		m.saveSessionNow()
	}
	if m.tree != nil {
		m.tree.Close()
		m.tree = nil
	}
	tabs := append([]*Tab(nil), m.tabs...)
	m.forceQuit = true
	go func() {
		if save {
			_ = m.saveOpenTabs(m.ctx, tabs)
		}
		if m.remote.backend != nil {
			m.remote.backend.closeForwards()
		}
		m.off()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if m.reg != nil {
			m.reg.CloseAll(ctx)
		}
		if m.plugins != nil {
			m.plugins.reg.Close()
		}
		if m.mgr != nil {
			_ = m.mgr.Close()
		}
		m.post(func() {
			if done != nil {
				done()
			}
		})
	}()
}

func (m *Model) finishQuit(save bool) {
	if m.workspaceHost != nil {
		if m.restartPending {
			m.workspaceHost.RequestRelaunch()
		} else {
			m.workspaceHost.RequestApplicationQuit()
		}
		return
	}
	m.shutdown(save, func() {
		if m.restartPending {
			mygo.App.Relaunch()
			return
		}
		mygo.App.Quit()
	})
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

// QuitImmediately closes every backend and quits without asking about
// changes, for offscreen smoke runs. Call it on the main thread.
func (m *Model) QuitImmediately() {
	if m.workspaceHost != nil {
		m.workspaceHost.QuitImmediately()
		return
	}
	m.shutdown(false, mygo.App.Quit)
}

// CloseWindowImmediately closes one native window without prompts. The host
// uses it for deterministic headless checks and cleanup.
func (m *Model) CloseWindowImmediately() {
	m.shutdown(false, func() {
		if m.win != nil {
			m.win.Destroy()
		}
	})
}

// BackendState reports the server for the status bar. A standalone editor has
// no project backend until it becomes part of a project.
func (m *Model) BackendState() openidea.State {
	if m.mgr == nil {
		return openidea.State{Phase: "unavailable"}
	}
	return m.mgr.State()
}

// BackendPID reports the current backend process, or zero when no server is
// running. Native-window smoke checks use it to prove each project has an
// independent server.
func (m *Model) BackendPID() int {
	if m.mgr == nil {
		return 0
	}
	server := m.mgr.Server()
	if server == nil {
		return 0
	}
	return server.PID()
}

// View draws the window.
func (m *Model) View(c *ui.Context) {
	m.systemRegion = c.Region()
	m.applyIDEOZoom(c)
	// OpenIDEA's own palette, day or night, whatever the appearance: the
	// stored choice picks one, the system's appearance the shade.

	// The native base theme is the operating-system appearance. Capture it
	// before an explicit GoIDEA theme replaces this context.
	systemDark := m.systemDark
	if base := c.Theme(); base != nil {
		systemDark = base.Dark
		m.systemDark = systemDark
	}
	if m.theme != nil {
		c.SetTheme(m.theme)
	} else {
		c.SetTheme(m.prefs.systemThemeWithAccent(systemDark))
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
	m.frameWidth, m.frameHeight = c.Size()
	m.shortcuts(c)
	p := c.Theme()
	m.refreshTodoHighlights(time.Now(), p)
	ui.Column(c).Fill().Background(p.Background).Children(func() {
		if m.startupWaitingVisible() {
			m.startupSurface(c, p)
			m.balloons(c, p)
			return
		}
		if m.welcomeVisible() {
			// With no project open the Welcome screen owns the window, as
			// OpenIDEA's does; What's New is standalone, so while it is on
			// show it takes the window instead.
			if m.banner != "" {
				m.bannerRow(c, p)
			}
			if m.Root() == "" && m.whatsNewShown() {
				m.whatsNewStandalone(c, p)
			} else {
				m.welcome(c, p)
			}
			// The Welcome screen has no status bar, so balloons are the only
			// place an update or backend notice can show, as upstream's are.
			m.balloons(c, p)
			return
		}
		m.workbench(c, p)
	})
	m.renderOverlays(c, p)
	m.mcpObserveEditorState()
}

// welcomeVisible reports whether the Welcome screen owns the frame. A loaded
// project can temporarily return to Welcome, while a standalone document
// keeps the normal workbench visible even though no project is open.
func (m *Model) welcomeVisible() bool {
	if m.showingWelcome {
		return true
	}
	return !m.startupPending && m.Root() == "" && len(m.allTabs()) == 0
}

func (m *Model) startupWaitingVisible() bool {
	return m.startupPending && m.Root() == "" && len(m.allTabs()) == 0
}

func (m *Model) startupSurface(c *ui.Context, p *ui.Theme) {
	oi := oiPaletteOf(p)
	ui.Column(c.Key("startup:opening")).Fill().Background(p.Background).
		AlignItems(ui.Center).Justify(ui.Center).Gap(12).Children(func() {
		if logo := welcomeLogoImage(); logo != nil {
			ui.Image(c, logo).Size(64, 64)
		}
		ui.Text(c, "GoIDEA").FontSize(22).Bold().TextColor(oi.textStrong)
		ui.Spinner(c).Size(18, 18).TextColor(oi.textMuted).
			Label("Opening project")
		ui.Text(c, "Opening project…").FontSize(12).TextColor(oi.textMuted)
	})
}

func (m *Model) shortcuts(c *ui.Context) {
	if m.customRegions != nil && m.customRegions.open {
		if m.customRegionShortcut(c) {
			return
		}
	}
	if m.testNav != nil && m.testNav.open {
		if m.testNavigationShortcut(c) {
			return
		}
	}
	if m.unwrap != nil && m.unwrap.open {
		if m.unwrapShortcut(c) {
			return
		}
	}
	if !m.overlayOpen() &&
		m.keymapHasBinding("navigate.searchEverywhere", "Shift Shift") &&
		m.doubleShift.observeDoubleShift(c.Modifiers(), c.Now()) {
		if m.RunCommand("navigate.searchEverywhere") {
			return
		}
	}
	if !m.overlayOpen() && m.observeCaretCloneChord(c) {
		return
	}
	if !m.overlayOpen() && m.caretCloneChordShortcut(c) {
		return
	}
	if !m.overlayOpen() && m.vimClaimsShortcut(c) {
		return
	}
	if m.recentNav.popup != nil {
		return
	}
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
	// The implementations chooser steps its rows with the arrow keys.
	if m.impl.open {
		if c.Shortcut(ui.Modifiers(0), ui.KeyDown) {
			m.stepImplementation(1)
		}
		if c.Shortcut(ui.Modifiers(0), ui.KeyUp) {
			m.stepImplementation(-1)
		}
		if c.Shortcut(ui.Modifiers(0), ui.KeyEnter) {
			m.chooseImplementation(m.impl.sel)
		}
		return
	}
	// The statement history steps the same way.
	if m.dbHist != nil && m.dbHist.open {
		if c.Shortcut(ui.Modifiers(0), ui.KeyDown) {
			m.StepDBHistory(1)
		}
		if c.Shortcut(ui.Modifiers(0), ui.KeyUp) {
			m.StepDBHistory(-1)
		}
		if c.Shortcut(ui.Modifiers(0), ui.KeyEnter) {
			m.ChooseDBHistory(m.dbHist.sel)
		}
		return
	}
	// The shell list steps with the arrow keys too.
	if m.termShell != nil && m.termShell.open {
		if c.Shortcut(ui.Modifiers(0), ui.KeyDown) {
			m.StepTermShell(1)
		}
		if c.Shortcut(ui.Modifiers(0), ui.KeyUp) {
			m.StepTermShell(-1)
		}
		if c.Shortcut(ui.Modifiers(0), ui.KeyEnter) {
			m.ChooseTermShell(m.termShell.sel)
		}
		return
	}
	// A modal sheet owns the keyboard; only its own dismiss keys apply.
	if m.modalOverlayOpen() {
		if c.Shortcut(ui.Modifiers(0), ui.KeyEnter) {
			if action := m.overlayDefaultAction(); action != nil {
				action()
			}
		}
		return
	}
	if m.dispatchEditorActionGapShortcuts(c) {
		return
	}
	// Preset bindings run through the same action registry as menus and the
	// palette. The editor has already had first refusal for its own bindings,
	// and KeyWhen preferences resolve the context-specific collisions.
	if m.dispatchKeymapShortcut(c) {
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
	// Escape is window behavior rather than a configurable action: it clears
	// the selected usage highlights first, then returns focus from a tool
	// window to the editor.
	if c.Shortcut(ui.Modifiers(0), ui.KeyEscape) && !m.overlayOpen() &&
		m.ClearStickyUsageHighlights() {
		return
	}
	if c.Shortcut(ui.Modifiers(0), ui.KeyEscape) && !m.overlayOpen() && m.visibleTool() != "" {
		m.FocusEditor()
	}
}
