package app

import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

// Session is what GoIDEA restores at the next start: the project folder, the
// files that had tabs, the one that was showing, and its bookmarks.
type Session struct {
	Root      string                  `json:"root,omitempty"`
	Remote    *openidea.RemoteSession `json:"remote,omitempty"`
	Files     []string                `json:"files,omitempty"`
	Focus     string                  `json:"focus,omitempty"`
	Bookmarks []Bookmark              `json:"bookmarks,omitempty"`
	// Pinned are the open files whose tabs were pinned, which OpenIDEA keeps
	// at the left of the strip across starts.
	Pinned []string `json:"pinned,omitempty"`
	// PreviewFiles are the editor tabs that were still previews. OpenIDEA
	// restores that mode, so the next result click still replaces the same tab.
	PreviewFiles []string `json:"previewFiles,omitempty"`
	// AgentTasks and AgentHome restore the virtual Agent pages that OpenIDEA
	// keeps as ordinary editor tabs. Missing tasks are removed after the
	// backend reports the project's task list.
	AgentTasks    []string `json:"agentTasks,omitempty"`
	AgentHome     bool     `json:"agentHome,omitempty"`
	FocusAgent    string   `json:"focusAgent,omitempty"`
	FocusAgentNew bool     `json:"focusAgentNew,omitempty"`
	// SplitFiles, SplitActive, SplitDown and SplitFirst are the second editor
	// window group: its open files, the one it showed, whether it was stacked
	// under the first group and whether it sat left of or above it.
	SplitFiles  []string `json:"splitFiles,omitempty"`
	SplitActive int      `json:"splitActive,omitempty"`
	SplitDown   bool     `json:"splitDown,omitempty"`
	SplitFirst  bool     `json:"splitFirst,omitempty"`
	// SplitPreviewFiles keep the second group's preview mode independently.
	SplitPreviewFiles []string `json:"splitPreviewFiles,omitempty"`
	// PathBookmarks are the files marked with Toggle File Bookmark.
	PathBookmarks []PathBookmark `json:"pathBookmarks,omitempty"`
	// BookmarkLists are the named groups the tool window shows, and DefaultList
	// is the one new bookmarks go to.
	BookmarkLists []BookmarkList `json:"bookmarkLists,omitempty"`
	DefaultList   string         `json:"defaultList,omitempty"`
	// RecentProjects are the folders the Welcome screen lists, most recent
	// first, kept for sessions written before Recents carried their model.
	RecentProjects []string `json:"recentProjects,omitempty"`
	// Recents are the Welcome screen's projects with what the row shows:
	// name, when it was opened, its branch and remote, and its group.
	Recents []RecentProject `json:"recents,omitempty"`
	// RecentGroups are the user's Welcome groups, in their order, with the
	// collapsed ones remembered.
	RecentGroups []RecentGroup `json:"recentGroups,omitempty"`
	// Layouts are each project's workbench layout, by project folder, so a
	// project comes back the way it was left, as OpenIDEA stores a tool
	// window layout per project root.
	Layouts map[string]Layout `json:"layouts,omitempty"`
	// NamedLayouts are Window › Tool Window Layouts. OpenIDEA keeps these
	// snapshots in local storage, shared by every project, and remembers the
	// last saved or restored one with a check mark.
	NamedLayouts      []SavedToolWindowLayout `json:"namedLayouts,omitempty"`
	ActiveNamedLayout string                  `json:"activeNamedLayout,omitempty"`
	// ProjectViews are each project's expanded Project View folders. The
	// options and pane mode are global, as OpenIDEA stores them, while the
	// folders a project had open belong to that project.
	ProjectViews map[string][]string `json:"projectViews,omitempty"`
	// RunHistory is each project's Run Anything history, by project folder,
	// most recent first, as OpenIDEA keeps it per project root.
	RunHistory map[string][]string `json:"runHistory,omitempty"`
	// SearchHistory is Search Everywhere's recent queries, most recent first,
	// as OpenIDEA keeps the search history across starts.
	SearchHistory []string `json:"searchHistory,omitempty"`
	// SearchHistories keeps each Search Everywhere tab's history separately,
	// as OpenIDEA does. Legacy SearchHistory remains the All tab's list.
	SearchHistories map[string][]string `json:"searchHistories,omitempty"`
	// LogcatHistory is the Logcat filter menu's recent expressions, newest
	// first, shared across projects like OpenIDEA's local-storage history.
	LogcatHistory []string `json:"logcatHistory,omitempty"`
	// SearchScopes are the named file-pattern scopes Find in Files offers.
	SearchScopes []SearchScope `json:"searchScopes,omitempty"`
	// Notifications is the notification center's logged history. Runtime
	// timers and action callbacks stay in memory.
	Notifications []StoredNotification `json:"notifications,omitempty"`
}

// StoredNotification is one notification-center history row.
type StoredNotification struct {
	Key      string `json:"key,omitempty"`
	Level    string `json:"level"`
	Title    string `json:"title"`
	Body     string `json:"body,omitempty"`
	At       int64  `json:"at"`
	Read     bool   `json:"read,omitempty"`
	Group    string `json:"group,omitempty"`
	Logged   bool   `json:"logged,omitempty"`
	Verbatim bool   `json:"verbatim,omitempty"`
}

// RecentProject is one project of the Welcome screen.
type RecentProject struct {
	Path     string `json:"path"`
	Name     string `json:"name,omitempty"`
	OpenedAt int64  `json:"openedAt,omitempty"`
	Branch   string `json:"branch,omitempty"`
	Remote   string `json:"remote,omitempty"`
	Group    string `json:"group,omitempty"`
}

// RecentGroup is one Welcome group: the heading the recent list shows and
// whether it is collapsed.
type RecentGroup struct {
	Name      string `json:"name"`
	Collapsed bool   `json:"collapsed,omitempty"`
}

// SearchScope is a named custom scope for Find in Files.
type SearchScope struct {
	Name    string `json:"name"`
	Pattern string `json:"pattern"`
}

// normalizeRecentProjects drops missing, duplicate and unnamed projects,
// caps only ungrouped projects, and fills the name when a session left it
// out. Grouped projects stay until their group is removed.
func normalizeRecentProjects(list []RecentProject) []RecentProject {
	return normalizeRecentProjectsOnDisk(list, true)
}

// normalizeRecentRows normalizes state already held by the model. Current
// rows may belong to a temporarily unavailable path and are kept; the load
// path uses normalizeRecentProjects so stale session entries are dropped.
func normalizeRecentRows(list []RecentProject) []RecentProject {
	return normalizeRecentProjectsOnDisk(list, false)
}

func normalizeRecentProjectsOnDisk(list []RecentProject, checkDisk bool) []RecentProject {
	seen := map[string]bool{}
	out := make([]RecentProject, 0, len(list))
	for _, p := range list {
		if p.Path == "" || seen[p.Path] {
			continue
		}
		remote := strings.HasPrefix(p.Path, "ssh://") ||
			strings.HasPrefix(p.Path, "wsl://") ||
			strings.HasPrefix(p.Path, "devcontainer://")
		if checkDisk && !remote {
			if st, err := os.Stat(p.Path); err != nil || !st.IsDir() {
				continue
			}
		}
		if remote && p.Name == "" {
			name := path.Base(strings.TrimRight(p.Path, "/"))
			if name == "." || name == "/" || name == "" {
				continue
			}
			p.Name = name
		}
		if p.Name == "" {
			p.Name = filepath.Base(p.Path)
		}
		if p.Name == "" || p.Name == "." || p.Name == "/" {
			continue
		}
		seen[p.Path] = true
		if p.Group == "" {
			ungrouped := 0
			for _, kept := range out {
				if kept.Group == "" {
					ungrouped++
				}
			}
			if ungrouped >= maxRecentProjects {
				continue
			}
		}
		out = append(out, p)
	}
	return out
}

// Layout is how one project had its workbench arranged, as OpenIDEA keeps a
// tool window layout per project: the window the left area showed, the bottom
// windows that were up, where the split area's windows dock and how tall the
// bottom area was. A maximized window is a moment, not a layout, so it is not
// kept.
type Layout struct {
	// Side is the window the left primary area showed: "" for Project or
	// "vcs" for the Commit window.
	Side string `json:"side,omitempty"`
	// LeftTool is the window in the left side's split area below the primary
	// one: "structure", "bookmarks", or "" for none.
	LeftTool string `json:"leftTool,omitempty"`
	// LeftHidden is the left area closed, which IDEA keeps as a null slot.
	LeftHidden bool `json:"leftHidden,omitempty"`
	// LeftRatio is the share of the left side's room the primary area takes
	// when both areas show, as OpenIDEA's splits.left.
	LeftRatio float64 `json:"leftRatio,omitempty"`
	// LeftWidth is the left side's width in DIPs, as OpenIDEA's sizes.left.
	LeftWidth int `json:"leftWidth,omitempty"`
	// Bottom are the bottom windows that were showing, and Tool the one the
	// shortcuts acted on last: F12, Shift+Escape and Move to Split target it.
	Bottom []string `json:"bottom,omitempty"`
	Tool   string   `json:"tool,omitempty"`
	// Split are the bottom windows that dock in the split area.
	Split map[string]bool `json:"split,omitempty"`
	// BottomHeight is the bottom area's height in DIPs.
	BottomHeight int `json:"bottomHeight,omitempty"`
	// Right is the right area's window ("hierarchy"), RightHidden whether it
	// was closed, and RightWidth its width in DIPs.
	Right       string `json:"right,omitempty"`
	RightHidden bool   `json:"rightHidden,omitempty"`
	RightWidth  int    `json:"rightWidth,omitempty"`
	// RightSplit is the window in the right side's split slot and RightRatio
	// the share of the side its primary slot takes.
	RightSplit string  `json:"rightSplit,omitempty"`
	RightRatio float64 `json:"rightRatio,omitempty"`
	// Dock is the versioned six-slot record; sessions written before it have
	// only the fields above and are migrated when read.
	Dock *DockLayout `json:"dock,omitempty"`
}

// SavedToolWindowLayout is one named, project-independent tool window
// snapshot in Window › Tool Window Layouts.
type SavedToolWindowLayout struct {
	Name   string `json:"name"`
	Layout Layout `json:"layout"`
}

// clone copies a layout, so a saved snapshot never shares maps with the one
// the main thread keeps changing.
func (l Layout) clone() Layout {
	out := l
	out.Bottom = append([]string(nil), l.Bottom...)
	if l.Dock != nil {
		d := *l.Dock
		d.Last = append([]string(nil), l.Dock.Last...)
		d.Slots = map[string]string{}
		for k, v := range l.Dock.Slots {
			d.Slots[k] = v
		}
		if l.Dock.Hidden != nil {
			d.Hidden = map[string]string{}
			for k, v := range l.Dock.Hidden {
				d.Hidden[k] = v
			}
		}
		if l.Dock.StripeButtons != nil {
			d.StripeButtons = make(map[string]bool, len(l.Dock.StripeButtons))
			for id, visible := range l.Dock.StripeButtons {
				d.StripeButtons[id] = visible
			}
		}
		out.Dock = &d
	}
	if l.Split != nil {
		out.Split = make(map[string]bool, len(l.Split))
		for name, split := range l.Split {
			out.Split[name] = split
		}
	}
	return out
}

// normalizeLayout drops what a stored layout cannot mean: an unknown left
// window, bottom windows that are not tool windows, a height no resize
// reaches. The zero Layout is the default arrangement.
func normalizeLayout(l Layout) Layout {
	// Sessions from before the left split stored Structure and Bookmarks in
	// Side; they belong to the split area now.
	switch l.Side {
	case "structure", "bookmarks":
		if l.LeftTool == "" {
			l.LeftTool = l.Side
		}
		l.Side = ""
	case "", "vcs":
	default:
		l.Side = ""
	}
	switch l.LeftTool {
	case "", "structure", "bookmarks":
	default:
		l.LeftTool = ""
	}
	out := Layout{Side: l.Side, LeftTool: l.LeftTool, LeftHidden: l.LeftHidden}
	// A ratio out of the range no drag reaches keeps the default divide.
	if l.LeftRatio >= leftRatioMin && l.LeftRatio <= leftRatioMax {
		out.LeftRatio = l.LeftRatio
	}
	// A width no drag or resize reaches is dropped the way OpenIDEA's
	// sanitizeSizes does.
	if l.LeftWidth >= leftWidthStoredMin && l.LeftWidth <= leftWidthStoredMax {
		out.LeftWidth = l.LeftWidth
	}
	if isToolWindow(l.Tool) {
		out.Tool = l.Tool
	}
	for _, name := range l.Bottom {
		if !isToolWindow(name) || slices.Contains(out.Bottom, name) {
			continue
		}
		out.Bottom = append(out.Bottom, name)
	}
	for name, split := range l.Split {
		if !split || !isToolWindow(name) {
			continue
		}
		if out.Split == nil {
			out.Split = map[string]bool{}
		}
		out.Split[name] = true
	}
	if l.BottomHeight >= bottomMin && l.BottomHeight <= maxBottomHeight {
		out.BottomHeight = l.BottomHeight
	}
	switch l.Right {
	case "hierarchy", "notifications", "problems", "todo":
		out.Right = l.Right
		out.RightHidden = l.RightHidden
	}
	if l.RightWidth >= rightWidthStoredMin && l.RightWidth <= rightWidthStoredMax {
		out.RightWidth = l.RightWidth
	}
	switch l.RightSplit {
	case "hierarchy", "notifications", "problems", "todo", "services":
		out.RightSplit = l.RightSplit
	}
	if l.RightRatio >= leftRatioMin && l.RightRatio <= leftRatioMax {
		out.RightRatio = l.RightRatio
	}
	if l.Dock != nil && l.Dock.Version >= dockLayoutVersion {
		out.Dock = l.clone().Dock
		if out.Dock.Slots == nil {
			out.Dock.Slots = map[string]string{}
		}
	}
	return out
}

// maxRecentProjects is how many folders the Welcome screen keeps, as
// OpenIDEA's Projects page does.
const maxRecentProjects = 12

// maxLayouts bounds the remembered layouts, as maxRecentProjects bounds the
// folders; the oldest ones go first.
const maxLayouts = 48

// pruneLayouts drops layouts for folders that are no longer recent until at
// most maxLayouts remain, keeping the ones in keep.
func pruneLayouts(layouts map[string]Layout, keep []string) map[string]Layout {
	if len(layouts) <= maxLayouts {
		return layouts
	}
	kept := make(map[string]Layout, maxLayouts)
	for _, root := range keep {
		key := layoutKey(root)
		if l, ok := layouts[key]; ok {
			kept[key] = l
		}
	}
	var rest []string
	for key := range layouts {
		if _, ok := kept[key]; !ok {
			rest = append(rest, key)
		}
	}
	sort.Strings(rest)
	for _, key := range rest {
		if len(kept) >= maxLayouts {
			break
		}
		kept[key] = layouts[key]
	}
	return kept
}

// layoutKey is the key a project's layout is stored under: its folder without
// a trailing separator.
func layoutKey(root string) string {
	return filepath.Clean(root)
}

// layoutFor is the layout the folder had, normalized; the zero Layout when
// none is stored.
func (s Session) layoutFor(root string) Layout {
	return normalizeLayout(s.Layouts[layoutKey(root)])
}

// maxRunHistory is how many commands Run Anything remembers per project, as
// OpenIDEA's MAX_HISTORY.
const maxRunHistory = 30

// runHistoryFor returns the folder's remembered commands, most recent first.
func (s Session) runHistoryFor(root string) []string {
	if root == "" {
		return nil
	}
	return append([]string(nil), s.RunHistory[layoutKey(root)]...)
}

// normalizeRunHistory trims each project's list: blank commands and repeats
// go, and it ends at maxRunHistory.
func normalizeRunHistory(history map[string][]string) map[string][]string {
	out := make(map[string][]string, len(history))
	for key, lines := range history {
		seen := make(map[string]bool, len(lines))
		var kept []string
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || seen[line] {
				continue
			}
			seen[line] = true
			kept = append(kept, line)
			if len(kept) == maxRunHistory {
				break
			}
		}
		if len(kept) > 0 {
			out[layoutKey(key)] = kept
		}
	}
	return out
}

// pushRunHistory puts line first in the folder's history, once, as
// OpenIDEA's pushHistory does.
func pushRunHistory(history map[string][]string, root, line string) []string {
	line = strings.TrimSpace(line)
	if root == "" || line == "" {
		return nil
	}
	key := layoutKey(root)
	next := []string{line}
	for _, h := range history[key] {
		if h != line && len(next) < maxRunHistory {
			next = append(next, h)
		}
	}
	if history == nil {
		history = map[string][]string{}
	}
	history[key] = next
	return next
}

// pruneRunHistory keeps the histories of the remembered folders, so the map
// does not grow with projects that left the Welcome screen.
func pruneRunHistory(history map[string][]string, keep []string) map[string][]string {
	if len(history) == 0 {
		return nil
	}
	kept := make(map[string][]string, len(keep))
	for _, root := range keep {
		key := layoutKey(root)
		if lines, ok := history[key]; ok {
			kept[key] = lines
		}
	}
	if len(kept) == 0 {
		return nil
	}
	return kept
}

// normalizeRecents keeps the folders that still exist, drops duplicates and
// ends at the cap, keeping the order it is given.
func normalizeRecents(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	var out []string
	for _, p := range paths {
		if p == "" || seen[p] {
			continue
		}
		if st, err := os.Stat(p); err != nil || !st.IsDir() {
			continue
		}
		seen[p] = true
		out = append(out, p)
		if len(out) == maxRecentProjects {
			break
		}
	}
	return out
}

// LoadSession reads the session at path; a missing or unreadable file is an
// empty session, so a broken file never keeps the app from starting.
func LoadSession(path string) Session {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Session{}
	}
	var s Session
	if json.Unmarshal(raw, &s) != nil {
		return Session{}
	}
	if len(s.Recents) > 0 || len(s.RecentGroups) > 0 {
		s.RecentGroups, s.Recents = normalizeRecentGroups(s.RecentGroups, s.Recents)
		s.Recents = normalizeRecentProjects(s.Recents)
		s.RecentGroups, s.Recents = normalizeRecentGroups(s.RecentGroups, s.Recents)
		s.RecentProjects = recentPaths(s.Recents)
	}
	return s
}

// Save writes the session atomically: a crash mid-write leaves the previous
// session intact instead of a truncated file.
func (s Session) Save(path string) error {
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, raw)
}

// Preferences are the settings the user changes in the settings dialog. The
// zero value is the default: follow the system's appearance and the editor's
// own font size.
type Preferences struct {
	Theme               string  `json:"theme,omitempty"`
	PreferredLightTheme string  `json:"preferredLightTheme,omitempty"`
	PreferredDarkTheme  string  `json:"preferredDarkTheme,omitempty"`
	FontSize            float32 `json:"fontSize,omitempty"`
	// DateFormat is Settings › System Settings › Date Formats. Nil keeps
	// OpenIDEA's system-format, pretty-formatting defaults.
	DateFormat *DateFormatPrefs `json:"dateFormat,omitempty"`
	// Font carries Settings › Editor › Font: the installed family, explicit
	// line height in pixels, OpenType ligatures, main weight and fallback.
	// Zero values keep the bundled JetBrains Mono at the size-derived height.
	Font FontPrefs `json:"font,omitempty"`
	// Appearance carries the runtime View toggles and IDE zoom introduced by
	// OpenIDEA's appearanceRuntime.ts. The inverse hide flags keep the zero
	// value equal to OpenIDEA's visible defaults.
	Appearance AppearancePrefs `json:"appearance,omitempty"`
	// Zen is Distraction Free Mode, which survives a restart as IDEA's does,
	// and ZenWidth is its text column in columns; zero is the default.
	Zen      bool `json:"distractionFree,omitempty"`
	ZenWidth int  `json:"distractionFreeWidth,omitempty"`
	// Editor are Settings › Editor › Appearance, whose toggles are on by
	// default; the inverse names keep the zero value meaning that.
	Editor EditorAppearance `json:"editor,omitempty"`
	// EditorScheme keeps the color-scheme choices used outside the Settings
	// dialog. Backend editor.* values and GoIDEA's local preferences are
	// updated together when one of the scheme actions runs.
	EditorScheme EditorSchemePrefs `json:"editorScheme,omitempty"`
	// Inlay are Settings › Editor › Inlay Hints: Code Vision's usages,
	// author and "no usages" choices. OpenIDEA shows all three by default.
	Inlay InlayPrefs `json:"inlay,omitempty"`
	// Debugger are the native editor and data-view choices from Settings ›
	// Build, Execution, Deployment › Debugger. Inverse names keep the zero
	// value equal to OpenIDEA's defaults.
	Debugger DebuggerPrefs `json:"debugger,omitempty"`
	// Usages are Find Usages' local view choices. Both read and write access
	// filters start on, the popup preview starts off and the tool-window
	// source preview starts on, as OpenIDEA stores them.
	Usages UsagesPrefs `json:"usages,omitempty"`
	// Problems are the Problems tool window's local view choices. The zero
	// value matches OpenIDEA: severity order on, source preview and
	// autoscroll off.
	Problems ProblemsPrefs `json:"problems,omitempty"`
	// Analysis keeps the Analysis tool windows' local options and templates.
	// Pointer-free inverse booleans preserve OpenIDEA's default-on choices in
	// settings files written before these windows existed.
	Analysis AnalysisPrefs `json:"analysis,omitempty"`
	// Hierarchy is the Hierarchy tool window's AutoScroll to Source choice.
	Hierarchy HierarchyPrefs `json:"hierarchy,omitempty"`
	// Breadcrumbs are Settings › Editor › Breadcrumbs: shown above the editor
	// unless hidden, as editor.breadcrumbs.show and .placement store.
	Breadcrumbs BreadcrumbPrefs `json:"breadcrumbs,omitempty"`
	// General are Settings › Editor › General behavior choices that the native
	// editor can honor.
	General GeneralPrefs `json:"general,omitempty"`
	// Vim is Settings › Editor › Vim. Enabled applies to every native editor,
	// and Handlers records only conflict overrides from IdeaVim's defaults.
	Vim VimPrefs `json:"vim,omitempty"`
	// System are Settings › Appearance & Behavior › System Settings. Pointer
	// defaults preserve OpenIDEA's defaults for settings files created before
	// these options existed.
	System SystemPrefs `json:"system,omitempty"`
	// Tabs are Settings › Editor › Editor Tabs: how the strip shows, orders
	// and limits tabs, as editor.tabs.* stores them.
	Tabs TabPrefs `json:"tabs,omitempty"`
	// ProjectTree are the Project View pane, appearance and behavior
	// choices. Pointer-valued defaults keep old settings files and a zero
	// Preferences at OpenIDEA's defaults.
	ProjectTree ProjectTreePrefs `json:"projectTree,omitempty"`
	// SaveFiles are Settings › Editor › General › Save Files, applied to the
	// document on every save; off by default, as OpenIDEA has them.
	SaveFiles SavePrefs `json:"saveFiles,omitempty"`
	// Todo are Settings › Editor › TODO, nil until the page is applied, which
	// keeps OpenIDEA's built-in patterns.
	Todo *TodoPrefs `json:"todo,omitempty"`
	// Notify are Settings › Appearance & Behavior › Notifications: the global
	// balloon switch and each group's display and log choice.
	Notify *NotifyPrefs `json:"notifications,omitempty"`
	// Run are the run flow's stored choices; nil keeps OpenIDEA's defaults.
	Run *RunPrefs `json:"run,omitempty"`
	// Services keeps the run configuration types, hidden configurations and
	// display order chosen in the Services tool window, per project.
	Services ServicesPrefs `json:"services,omitempty"`
	// ExternalTools is Settings › Tools › External Tools. Each enabled tool is
	// also an action and can be a before-launch task.
	ExternalTools []ExternalToolPrefs `json:"externalTools,omitempty"`
	// Agents are Settings › Agents › Chat, shared by the New Agent Task page
	// and agent conversations.
	Agents *AgentPrefs `json:"agents,omitempty"`
	// AgentWorkspace is where conversations open and how each project's task
	// list is pinned or grouped. It is local window state, as OpenIDEA keeps
	// these choices outside the backend's Agents settings.
	AgentWorkspace AgentWorkspacePrefs `json:"agentWorkspace,omitempty"`
	// AgentSecurity is Settings › Agents › Command Permissions.
	AgentSecurity *AgentSecurityPrefs `json:"agentSecurity,omitempty"`
	// Updates are Help › Check for Updates' settings, nil until changed: the
	// automatic check and the What's New page stay on, as app.update.* does.
	Updates *UpdatePrefs `json:"updates,omitempty"`
	// HTTP keeps the HTTP Client environment selected for each request file,
	// as OpenIDEA stores openidea.httpClient.environments.
	HTTP HTTPPrefs `json:"http,omitempty"`
	// GraphQL keeps the environment values entered for each graphql-config,
	// as OpenIDEA stores openidea.graphql.environment outside project files.
	GraphQL GraphQLPrefs `json:"graphql,omitempty"`
	// ScratchRecent are the languages New Scratch File lists first, most
	// recent first.
	ScratchRecent []string `json:"scratchRecent,omitempty"`
	// Accent is the appearance's accent colour id ("" for OpenIDEA blue), and
	// Startup is "reopen" to open the last project at start; "" and "welcome"
	// show the Welcome screen.
	Accent  string `json:"accent,omitempty"`
	Startup string `json:"startup,omitempty"`
	// KeymapPreset is one of OpenIDEA's six preset ids. Empty or unknown uses
	// IntelliJ macOS on macOS and IntelliJ Windows on other systems.
	KeymapPreset string `json:"keymapPreset,omitempty"`
	// Keymap maps an action id to the user's bindings, replacing the selected
	// preset's defaults; an empty list removes the shortcut.
	Keymap map[string][]string `json:"keymap,omitempty"`
	// Macros keeps Edit › Macros' last recording and every named macro.
	Macros MacroPrefs `json:"macros,omitempty"`
	// MainToolbar is the title bar's saved layout, nil for the default one.
	MainToolbar *toolbarLayout `json:"mainToolbar,omitempty"`
	// SearchFilters is Search Everywhere's funnel, kept across starts.
	SearchFilters *seFilters `json:"searchFilters,omitempty"`
	// SearchPreview remembers whether Search Everywhere opens its source
	// preview beside the result list.
	SearchPreview bool `json:"searchPreview,omitempty"`
	// SearchFieldHistory is the Find window's recent multi-line queries, newest
	// first, as OpenIDEA keeps the search-field history.
	SearchFieldHistory []string `json:"searchFieldHistory,omitempty"`
	// Plugins are each plugin's settings, by name, under the same "plugins"
	// key Mayoi uses; a plugin's "enabled" lives there.
	Plugins map[string]json.RawMessage `json:"plugins,omitempty"`
	// VCS keeps the Git tool windows' local display choices and the user's
	// branch/commit bookmarks. Repository-scoped map keys contain the root and
	// the branch or commit id.
	VCS VCSPrefs `json:"vcs,omitempty"`
}

// HTTPPrefs is the HTTP Client's local view state.
type HTTPPrefs struct {
	Environments map[string]string `json:"environments,omitempty"`
}

// GraphQLPrefs is the GraphQL Environment Variables dialog's local storage.
type GraphQLPrefs struct {
	Environment map[string]map[string]string `json:"environment,omitempty"`
}

// SystemPrefs is Settings › Appearance & Behavior › System Settings.
type SystemPrefs struct {
	ReopenLastProject       *bool  `json:"reopenLastProject,omitempty"`
	OpenProjectIn           string `json:"openProjectIn,omitempty"`
	DefaultProjectDirectory string `json:"defaultProjectDirectory,omitempty"`
	AutoSave                *bool  `json:"autoSave,omitempty"`
	SyncOnFocus             *bool  `json:"syncOnFocus,omitempty"`
	SaveOnIdle              bool   `json:"saveOnIdle,omitempty"`
	IdleSeconds             int    `json:"idleSeconds,omitempty"`
	LocalHistoryDays        int    `json:"localHistoryDays,omitempty"`
	LocalHistoryMaxMB       int    `json:"localHistoryMaxMB,omitempty"`
	ConfirmExit             *bool  `json:"confirmExit,omitempty"`
	CloseWithoutWelcome     bool   `json:"closeWithoutWelcome,omitempty"`
	CloseTabsWithWindow     bool   `json:"closeTabsWithWindow,omitempty"`
}

// reopenLastProject resolves the legacy Startup preference and OpenIDEA's
// default. An explicit System Settings choice wins.
func (p Preferences) reopenLastProject() bool {
	if p.System.ReopenLastProject != nil {
		return *p.System.ReopenLastProject
	}
	switch p.Startup {
	case "reopen":
		return true
	case "welcome":
		return false
	default:
		return true
	}
}

func (p SystemPrefs) openProjectIn() string {
	switch p.OpenProjectIn {
	case "new", "same":
		return p.OpenProjectIn
	default:
		return "ask"
	}
}

func (p SystemPrefs) autoSave() bool {
	return prefBool(p.AutoSave, true)
}

func (p SystemPrefs) syncOnFocus() bool {
	return prefBool(p.SyncOnFocus, true)
}

func (p SystemPrefs) idleSeconds() int {
	if p.IdleSeconds == 0 {
		return 15
	}
	return max(1, min(p.IdleSeconds, 3600))
}

func (p SystemPrefs) localHistoryDays() int {
	if p.LocalHistoryDays == 0 {
		return 5
	}
	return max(1, min(p.LocalHistoryDays, 365))
}

func (p SystemPrefs) localHistoryMaxMB() int {
	if p.LocalHistoryMaxMB == 0 {
		return 512
	}
	return max(16, min(p.LocalHistoryMaxMB, 16384))
}

func (p SystemPrefs) confirmExit() bool {
	return prefBool(p.ConfirmExit, true)
}

func (p SystemPrefs) defaultProjectDirectory() string {
	dir := strings.TrimSpace(p.DefaultProjectDirectory)
	if dir == "" {
		return ""
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return ""
	}
	return dir
}

// ServicesPrefs mirrors OpenIDEA's per-project Services stores. The project
// key is the normalized root, so changing project cannot carry one project's
// types, hidden configurations or order into another.
type ServicesPrefs struct {
	Projects map[string]ServiceProjectPrefs `json:"projects,omitempty"`
	// Docker keeps the Docker provider's global View Options. Nil switches
	// retain OpenIDEA's defaults, where stopped containers and untagged images
	// are visible.
	Docker DockerFilterPrefs `json:"docker,omitempty"`
}

// DockerFilterPrefs is OpenIDEA's Docker Services View Options. The pointers
// preserve the default-on behavior for settings files written before these
// filters existed.
type DockerFilterPrefs struct {
	ShowStoppedContainers *bool `json:"showStoppedContainers,omitempty"`
	ShowUntaggedImages    *bool `json:"showUntaggedImages,omitempty"`
}

func (p DockerFilterPrefs) showStoppedContainers() bool {
	return prefBool(p.ShowStoppedContainers, true)
}

func (p DockerFilterPrefs) showUntaggedImages() bool {
	return prefBool(p.ShowUntaggedImages, true)
}

// ServiceProjectPrefs is one project's persisted Services state.
type ServiceProjectPrefs struct {
	Types            []string            `json:"types,omitempty"`
	Hidden           map[string][]string `json:"hidden,omitempty"`
	Order            map[string][]string `json:"order,omitempty"`
	ComposeOverrides map[string]string   `json:"composeFileOverrides,omitempty"`
}

// UsagesPrefs mirrors OpenIDEA's persistent usage-preview and access-filter
// switches. The inverse names keep the zero value at both access filters on
// and the popup preview off while the tool-window preview stays on.
type UsagesPrefs struct {
	PopupPreview  bool `json:"popupPreview,omitempty"`
	NoFindPreview bool `json:"noFindPreview,omitempty"`
	HideRead      bool `json:"hideReadAccess,omitempty"`
	HideWrite     bool `json:"hideWriteAccess,omitempty"`
}

// ShowReadAccess reports whether read and unknown access rows are shown.
func (p UsagesPrefs) ShowReadAccess() bool { return !p.HideRead }

// ShowWriteAccess reports whether write and unknown access rows are shown.
func (p UsagesPrefs) ShowWriteAccess() bool { return !p.HideWrite }

// ShowFindPreview reports whether the Find Usages tool window has its source
// preview pane. Its zero value is on.
func (p UsagesPrefs) ShowFindPreview() bool { return !p.NoFindPreview }

// ProblemsPrefs mirrors OpenIDEA's persistent Problems view options.
type ProblemsPrefs struct {
	NoSeveritySort bool `json:"noSeveritySort,omitempty"`
	SortByName     bool `json:"sortByName,omitempty"`
	ShowPreview    bool `json:"showPreview,omitempty"`
	Autoscroll     bool `json:"autoscroll,omitempty"`
}

// SortBySeverity reports whether errors precede warnings and information.
func (p ProblemsPrefs) SortBySeverity() bool { return !p.NoSeveritySort }

// AnalysisPrefs keeps local state for Inspection Results, Locate Duplicates,
// Search / Replace Structurally and Code Cleanup.
type AnalysisPrefs struct {
	Inspection InspectionViewPrefs  `json:"inspection,omitempty"`
	Duplicates DuplicateSearchPrefs `json:"duplicates,omitempty"`
	Structural StructuralFormPrefs  `json:"structural,omitempty"`
	Cleanup    CodeCleanupPrefs     `json:"cleanup,omitempty"`
}

// InspectionViewPrefs are the Inspection Results view options. Grouping by
// inspection and no autoscroll are OpenIDEA's defaults.
type InspectionViewPrefs struct {
	GroupBySeverity bool `json:"groupBySeverity,omitempty"`
	Autoscroll      bool `json:"autoscroll,omitempty"`
}

// DuplicateSearchPrefs persists Locate Duplicates' options. Anonymization is
// on by default, so the stored flags name the explicit opt-outs.
type DuplicateSearchPrefs struct {
	MinTokens              int  `json:"minTokens,omitempty"`
	NoAnonymizeIdentifiers bool `json:"noAnonymizeIdentifiers,omitempty"`
	NoAnonymizeLiterals    bool `json:"noAnonymizeLiterals,omitempty"`
}

func (p DuplicateSearchPrefs) minTokens() int {
	if p.MinTokens == 0 {
		return 60
	}
	return max(10, min(p.MinTokens, 10_000))
}

func (p DuplicateSearchPrefs) anonymizeIdentifiers() bool {
	return !p.NoAnonymizeIdentifiers
}

func (p DuplicateSearchPrefs) anonymizeLiterals() bool {
	return !p.NoAnonymizeLiterals
}

// StructuralFormPrefs is the last validated Search / Replace Structurally
// form. Scope is intentionally local to one run.
type StructuralFormPrefs struct {
	Pattern     string `json:"pattern,omitempty"`
	Replacement string `json:"replacement,omitempty"`
	Language    string `json:"language,omitempty"`
}

// CodeCleanupPrefs persists the cleanup steps. Before the first change all
// steps are on; Configured distinguishes that default from three explicit
// opt-outs.
type CodeCleanupPrefs struct {
	Configured        bool `json:"configured,omitempty"`
	NoFixAll          bool `json:"noFixAll,omitempty"`
	NoOptimizeImports bool `json:"noOptimizeImports,omitempty"`
	NoReformat        bool `json:"noReformat,omitempty"`
}

func (p CodeCleanupPrefs) options() analysisCleanupOptions {
	if !p.Configured {
		return analysisCleanupOptions{fixAll: true, optimizeImports: true, reformat: true}
	}
	return analysisCleanupOptions{
		fixAll:          !p.NoFixAll,
		optimizeImports: !p.NoOptimizeImports,
		reformat:        !p.NoReformat,
	}
}

func (p *AnalysisPrefs) setCleanupOptions(options analysisCleanupOptions) {
	p.Cleanup = CodeCleanupPrefs{
		Configured:        true,
		NoFixAll:          !options.fixAll,
		NoOptimizeImports: !options.optimizeImports,
		NoReformat:        !options.reformat,
	}
}

// HierarchyPrefs mirrors OpenIDEA's persistent hierarchy options.
type HierarchyPrefs struct {
	Autoscroll bool `json:"autoscroll,omitempty"`
}

// AppearancePrefs is the part of Settings › Appearance that View actions
// change without opening the dialog.
type AppearancePrefs struct {
	IDEZoom            int  `json:"ideZoom,omitempty"`
	CompactMode        bool `json:"compactMode,omitempty"`
	HideMainToolbar    bool `json:"hideMainToolbar,omitempty"`
	HideToolWindowBars bool `json:"hideToolWindowBars,omitempty"`
	HideStatusBar      bool `json:"hideStatusBar,omitempty"`
	// UseCustomFont and the UI font fields mirror OpenIDEA's Accessibility
	// controls. Their zero values keep the bundled UI font and 13 DIP size.
	UseCustomFont                 bool   `json:"useCustomFont,omitempty"`
	UIFontFamily                  string `json:"uiFontFamily,omitempty"`
	UIFontFallback                string `json:"uiFontFallback,omitempty"`
	UIFontSize                    int    `json:"uiFontSize,omitempty"`
	ContrastScrollbars            bool   `json:"contrastScrollbars,omitempty"`
	RedGreenVision                bool   `json:"redGreenVision,omitempty"`
	DifferentToolWindowBackground bool   `json:"differentToolWindowBackground,omitempty"`
	SmallTreeIndents              bool   `json:"smallTreeIndents,omitempty"`
	FullPathInTitle               bool   `json:"fullPathInTitle,omitempty"`
	DndWithAltOnly                bool   `json:"dndWithAltOnly,omitempty"`
	// ProjectColorsInToolbar and MenuIcons default on, so pointer values keep
	// settings files written before these controls existed compatible.
	ProjectColorsInToolbar *bool  `json:"projectColorsInToolbar,omitempty"`
	MenuIcons              *bool  `json:"menuIcons,omitempty"`
	MainMenuStyle          string `json:"mainMenuStyle,omitempty"`
	MainMenuAlt            *bool  `json:"mainMenuAlt,omitempty"`
	IDEAntialiasing        string `json:"ideAntialiasing,omitempty"`
	EditorAntialiasing     string `json:"editorAntialiasing,omitempty"`
	// ShowToolWindowNames puts IDEA's labels under the stripe icons.
	ShowToolWindowNames bool `json:"showToolWindowNames,omitempty"`
	// ShowToolWindowBadges is OpenIDEA's Appearance › Tool Windows option.
	// Nil keeps the upstream default, on, for settings written before it.
	ShowToolWindowBadges  *bool `json:"showToolWindowBadges,omitempty"`
	WidescreenToolWindows bool  `json:"widescreenToolWindows,omitempty"`
	// RenderDocComments draws documentation comments as rendered prose. A
	// tab's View › Active Editor override replaces this setting.
	RenderDocComments bool `json:"renderDocComments,omitempty"`
	// NavigationBar is "statusBar" by default, or "top"/"hidden" from View ›
	// Appearance › Navigation Bar.
	NavigationBar string `json:"navigationBar,omitempty"`
	// StatusBarWidgets keeps per-widget choices as one map because widget ids
	// contain dots, matching OpenIDEA's appearance.statusBarWidgets object.
	StatusBarWidgets map[string]bool `json:"statusBarWidgets,omitempty"`
}

func (p Preferences) toolWindowBadgesVisible() bool {
	return p.Appearance.ShowToolWindowBadges == nil || *p.Appearance.ShowToolWindowBadges
}

func (p Preferences) projectColorsInToolbar() bool {
	return prefBool(p.Appearance.ProjectColorsInToolbar, true)
}

func (p Preferences) menuIconsVisible() bool {
	return prefBool(p.Appearance.MenuIcons, true)
}

func (p Preferences) mainMenuAltFocuses() bool {
	return prefBool(p.Appearance.MainMenuAlt, true)
}

// uiFont is the family list used by every MyGo widget when custom UI font is
// enabled. OpenIDEA falls back to Inter and a 13 DIP base size.
func (p Preferences) uiFont() string {
	if !p.Appearance.UseCustomFont {
		return oiFontUI
	}
	primary := strings.TrimSpace(p.Appearance.UIFontFamily)
	if primary == "" {
		primary = oiFontUI
	}
	if fallback := strings.TrimSpace(p.Appearance.UIFontFallback); fallback != "" && fallback != primary {
		return primary + ", " + fallback
	}
	return primary
}

func (p Preferences) uiFontSize() float32 {
	if !p.Appearance.UseCustomFont {
		return 13
	}
	size := p.Appearance.UIFontSize
	if size == 0 {
		return 13
	}
	return float32(max(10, min(size, 18)))
}

// VCSPrefs is state that OpenIDEA keeps in its VCS stores and client settings.
type VCSPrefs struct {
	Diff             DiffPrefs         `json:"diff,omitempty"`
	Merge            MergePrefs        `json:"merge,omitempty"`
	GitHub           GitHubPrefs       `json:"github,omitempty"`
	FavoriteBranches map[string]bool   `json:"favoriteBranches,omitempty"`
	CommitBookmarks  map[string]string `json:"commitBookmarks,omitempty"`
	// CommitDrafts keeps the message left in the Commit field per project,
	// as OpenIDEA's browser storage keys a draft by the repository root.
	CommitDrafts map[string]string `json:"commitDrafts,omitempty"`
	// CommitMessages keeps the twenty most recent messages per repository.
	CommitMessages map[string][]string `json:"commitMessages,omitempty"`
	Log            LogPrefs            `json:"log,omitempty"`
	// IssueNavigation turns issue ids in commit messages into links in Git
	// Log, commit details and the blame popup.
	IssueNavigation []IssueLinkPrefs `json:"issueNavigation,omitempty"`
}

// GitHubPrefs mirrors the Version Control › GitHub client options.
type GitHubPrefs struct {
	AutoMarkViewed bool `json:"autoMarkViewed,omitempty"`
}

// IssueLinkPrefs is one Settings › Version Control › Issue Navigation row.
// Pattern is a Go regular expression and Link may use $0…$9 substitutions.
type IssueLinkPrefs struct {
	Pattern string `json:"pattern"`
	Link    string `json:"link"`
}

// MergePrefs mirrors the merge editor's local choices. Automatically applying
// non-conflicting changes is off by default, as IntelliJ and OpenIDEA have it.
type MergePrefs struct {
	AutoApply bool `json:"autoApply,omitempty"`
}

// DiffPrefs mirrors OpenIDEA's Diff Settings: the toolbar choices are shared by
// every diff viewer and survive a restart.
type DiffPrefs struct {
	Mode         string `json:"mode,omitempty"`
	Ignore       string `json:"ignore,omitempty"`
	Highlight    string `json:"highlight,omitempty"`
	Collapse     *bool  `json:"collapse,omitempty"`
	SyncScroll   *bool  `json:"syncScroll,omitempty"`
	AlignChanges bool   `json:"alignChanges,omitempty"`
	SoftWrap     string `json:"softWrap,omitempty"`
}

// LogPrefs persists OpenIDEA's Git Log presentation options. Pointer booleans
// distinguish an explicit off choice from a settings file written before that
// option existed, because several OpenIDEA defaults are on.
type LogPrefs struct {
	Sort               string             `json:"sort,omitempty"`
	FirstParent        *bool              `json:"firstParent,omitempty"`
	NoMerges           *bool              `json:"noMerges,omitempty"`
	CollapseLinear     *bool              `json:"collapseLinear,omitempty"`
	CompactRefs        *bool              `json:"compactRefs,omitempty"`
	TagNames           *bool              `json:"tagNames,omitempty"`
	LongEdges          *bool              `json:"longEdges,omitempty"`
	CommitTimestamp    *bool              `json:"commitTimestamp,omitempty"`
	RefsOnLeft         *bool              `json:"refsOnLeft,omitempty"`
	ShowAuthor         *bool              `json:"showAuthor,omitempty"`
	ShowDate           *bool              `json:"showDate,omitempty"`
	ShowHash           *bool              `json:"showHash,omitempty"`
	ShowSignature      *bool              `json:"showSignature,omitempty"`
	ShowGitHubChecks   *bool              `json:"showGitHubChecks,omitempty"`
	ShowColumnHeaders  *bool              `json:"showColumnHeaders,omitempty"`
	HighlightMine      *bool              `json:"highlightMine,omitempty"`
	HighlightMerges    *bool              `json:"highlightMerges,omitempty"`
	HighlightCurrent   *bool              `json:"highlightCurrentBranch,omitempty"`
	ShowBranches       *bool              `json:"showBranches,omitempty"`
	GroupByDirectory   *bool              `json:"groupByDirectory,omitempty"`
	DiffPreview        *bool              `json:"diffPreview,omitempty"`
	ShowLineStats      *bool              `json:"showLineStats,omitempty"`
	ShowDetails        *bool              `json:"showDetails,omitempty"`
	ShowRepository     *bool              `json:"showRepository,omitempty"`
	ShowOnlyAffected   *bool              `json:"showOnlyAffected,omitempty"`
	ChangesFromParents *bool              `json:"changesFromParents,omitempty"`
	DiffLocation       string             `json:"diffLocation,omitempty"`
	ColumnOrder        []string           `json:"columnOrder,omitempty"`
	ColumnWidths       map[string]float64 `json:"columnWidths,omitempty"`
	LinearizeMerges    *bool              `json:"linearizeMerges,omitempty"`
}

func (p LogPrefs) showBranches() bool {
	return prefBool(p.ShowBranches, true)
}

func (p LogPrefs) collapseLinear() bool {
	return prefBool(p.CollapseLinear, false)
}

func (p LogPrefs) groupByDirectory() bool {
	return prefBool(p.GroupByDirectory, true)
}

func (p LogPrefs) diffPreview() bool {
	return prefBool(p.DiffPreview, true)
}

func (p LogPrefs) normalizedSort() string {
	switch p.Sort {
	case "topo", "standard":
		return p.Sort
	default:
		return "date"
	}
}

func (p LogPrefs) normalizedDiffLocation() string {
	switch p.DiffLocation {
	case "right", "bottom":
		return p.DiffLocation
	default:
		return "editor"
	}
}

func prefBool(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func boolPref(value bool) *bool {
	return &value
}

// normalized fills the defaults OpenIDEA uses when a settings file is absent
// or predates a field.
func (p DiffPrefs) normalized() DiffPrefs {
	switch p.Mode {
	case "split", "unified":
	default:
		p.Mode = "split"
	}
	switch p.Ignore {
	case "none", "trim", "whitespace", "whitespaceAndLines":
	default:
		p.Ignore = "none"
	}
	switch p.Highlight {
	case "words", "lines", "none":
	default:
		p.Highlight = "words"
	}
	switch p.SoftWrap {
	case "follow", "on", "off":
	default:
		p.SoftWrap = "follow"
	}
	return p
}

// ProjectTreePrefs is Project View's global pane, appearance and behavior
// state. A nil switch keeps the OpenIDEA default, so adding a default-on
// option does not change settings files written before it existed.
type ProjectTreePrefs struct {
	Mode   string `json:"mode,omitempty"`
	SortBy string `json:"sortBy,omitempty"`

	MoveRefactor   FileRefactorPrefs `json:"moveRefactor,omitempty"`
	RenameRefactor FileRefactorPrefs `json:"renameRefactor,omitempty"`
	CopyOpenEditor bool              `json:"copyOpenInEditor,omitempty"`
	SafeDelete     *bool             `json:"safeDelete,omitempty"`

	ShowHidden           bool  `json:"showHidden,omitempty"`
	ShowExcluded         *bool `json:"showExcluded,omitempty"`
	ShowMembers          bool  `json:"showMembers,omitempty"`
	ShowRootPaths        *bool `json:"showRootPaths,omitempty"`
	Compact              *bool `json:"compact,omitempty"`
	FoldersFirst         *bool `json:"foldersFirst,omitempty"`
	AutoscrollFromSource bool  `json:"autoscrollFromSource,omitempty"`
	AutoscrollToSource   bool  `json:"autoscrollToSource,omitempty"`
	SingleClickOpen      bool  `json:"singleClickOpen,omitempty"`
	SingleClickExpand    bool  `json:"singleClickExpand,omitempty"`
	PreviewTab           bool  `json:"previewTab,omitempty"`
	ShowUnityMeta        bool  `json:"showUnityMeta,omitempty"`
	ShowUnityGenerated   bool  `json:"showUnityGenerated,omitempty"`
}

// FileRefactorPrefs remembers the choices of a project Move or Rename
// dialog. Nil booleans keep OpenIDEA's defaults: search references and text
// occurrences are on, while moved files are not opened without an existing
// editor.
type FileRefactorPrefs struct {
	References      *bool `json:"references,omitempty"`
	TextOccurrences *bool `json:"textOccurrences,omitempty"`
	OpenInEditor    *bool `json:"openInEditor,omitempty"`
}

func (p FileRefactorPrefs) referenceSearch() bool {
	return boolPreference(p.References, true)
}

func (p FileRefactorPrefs) textSearch() bool {
	return boolPreference(p.TextOccurrences, true)
}

func (p FileRefactorPrefs) opensInEditor() bool {
	return boolPreference(p.OpenInEditor, false)
}

func boolPreference(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func boolPointer(value bool) *bool { return &value }

// normalizedMode returns an OpenIDEA project pane id, or Project for an old
// or unknown value.
func (p ProjectTreePrefs) normalizedMode() string {
	switch p.Mode {
	case "", "project":
		return "project"
	case "projectFiles", "openFiles", "changedFiles", "scratches", "unity":
		return p.Mode
	default:
		return "project"
	}
}

// normalizedSort returns one of the three Project View orderings.
func (p ProjectTreePrefs) normalizedSort() string {
	switch p.SortBy {
	case "type", "modified":
		return p.SortBy
	default:
		return "name"
	}
}

func (p ProjectTreePrefs) showsExcluded() bool  { return boolPreference(p.ShowExcluded, true) }
func (p ProjectTreePrefs) compacts() bool       { return boolPreference(p.Compact, true) }
func (p ProjectTreePrefs) foldersFirst() bool   { return boolPreference(p.FoldersFirst, true) }
func (p ProjectTreePrefs) showsRootPaths() bool { return boolPreference(p.ShowRootPaths, true) }
func (p ProjectTreePrefs) searchesUsagesOnDelete() bool {
	return boolPreference(p.SafeDelete, true)
}

// RunPrefs carry legacy choices and the project-scoped chooser state. The four
// runtime fields remain as a migration fallback when the backend's current
// project settings have not loaded yet.
type RunPrefs struct {
	ConfirmRerun       *bool `json:"confirmRerun,omitempty"`
	ConfirmTerminate   *bool `json:"confirmTerminate,omitempty"`
	ShowOnStart        *bool `json:"showOnStart,omitempty"`
	TerminationTimeout *int  `json:"terminationTimeoutMs,omitempty"`
	// Pinned and Recent are the pre-project-scoped layout. They are read once
	// for migration into Projects and are no longer written.
	Pinned   []string                    `json:"pinned,omitempty"`
	Recent   []string                    `json:"recent,omitempty"`
	Projects map[string]*RunProjectPrefs `json:"projects,omitempty"`
}

// RunProjectPrefs is OpenIDEA's per-project chooser state. OpenIDEA stores
// these keys in localStorage under the project root.
type RunProjectPrefs struct {
	Pinned    []string             `json:"pinned,omitempty"`
	Recent    []string             `json:"recent,omitempty"`
	Temporary []openidea.RunConfig `json:"temporary,omitempty"`
	Selected  string               `json:"selected,omitempty"`
}

// ExternalToolPrefs mirrors OpenIDEA's Tools › External Tools entry.
type ExternalToolPrefs struct {
	ID               string `json:"id,omitempty"`
	Name             string `json:"name,omitempty"`
	Description      string `json:"description,omitempty"`
	Group            string `json:"group,omitempty"`
	Enabled          bool   `json:"enabled,omitempty"`
	Program          string `json:"program,omitempty"`
	Arguments        string `json:"arguments,omitempty"`
	WorkingDirectory string `json:"workingDirectory,omitempty"`
	SynchronizeAfter bool   `json:"synchronizeAfter,omitempty"`
	OpenConsole      bool   `json:"openConsole,omitempty"`
}

// AgentPrefs is Settings › Agents › Chat. The inverted switches keep the zero
// value at OpenIDEA's defaults, where finished work folds, streaming text is
// revealed smoothly, and the header shows context, time and usage limits.
type AgentPrefs struct {
	NewTaskApprovals string `json:"newTaskApprovals,omitempty"`
	SendWith         string `json:"sendWith,omitempty"`
	WhileRunning     string `json:"whileRunning,omitempty"`
	ToolCalls        string `json:"toolCalls,omitempty"`
	Thinking         string `json:"thinking,omitempty"`
	NoFoldTurns      bool   `json:"noFoldTurns,omitempty"`
	NoSmoothText     bool   `json:"noSmoothText,omitempty"`
	ExpandToolOutput bool   `json:"expandToolOutput,omitempty"`
	NoContextUsage   bool   `json:"noContextUsage,omitempty"`
	NoWorkTime       bool   `json:"noWorkTime,omitempty"`
	NoUsageLimits    bool   `json:"noUsageLimits,omitempty"`
	ShowCost         bool   `json:"showCost,omitempty"`
	// BackendDefaults and BackendMCPServers preserve the two maps the same
	// upstream setting carries outside the Chat page. They remain JSON text so
	// a Chat edit cannot erase the Registry's defaults or MCP configurations.
	BackendDefaults   string `json:"backendDefaults,omitempty"`
	BackendMCPServers string `json:"backendMCPServers,omitempty"`
}

func (p AgentPrefs) normalized() AgentPrefs {
	p.NewTaskApprovals = p.newTaskApprovals()
	p.SendWith = p.sendWith()
	p.WhileRunning = p.whileRunning()
	p.ToolCalls = p.toolCalls()
	p.Thinking = p.thinking()
	return p
}

// agentPrefs returns OpenIDEA's defaults when the page was never applied.
func (m *Model) agentPrefs() AgentPrefs {
	if m.prefs.Agents == nil {
		return (AgentPrefs{}).normalized()
	}
	return m.prefs.Agents.normalized()
}

// AgentSecurityPrefs are Settings › Agents › Command Permissions. Allowed
// commands run without a prompt and blocked commands are always rejected.
type AgentSecurityPrefs struct {
	AllowedCommands []string `json:"allowedCommands,omitempty"`
	BlockedCommands []string `json:"blockedCommands,omitempty"`
}

var defaultBlockedAgentCommands = []string{
	"rm -rf *",
	"rm -rf /",
	"mkfs*",
	"dd *",
	":(){ :|:& };:",
	"shutdown*",
	"reboot*",
}

func (p AgentSecurityPrefs) normalized() AgentSecurityPrefs {
	p.AllowedCommands = uniqueAgentCommandPatterns(p.AllowedCommands)
	if p.BlockedCommands == nil {
		p.BlockedCommands = append([]string(nil), defaultBlockedAgentCommands...)
	} else {
		p.BlockedCommands = uniqueAgentCommandPatterns(p.BlockedCommands)
	}
	return p
}

func uniqueAgentCommandPatterns(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func (m *Model) agentSecurityPrefs() AgentSecurityPrefs {
	if m.prefs.AgentSecurity == nil {
		return (AgentSecurityPrefs{}).normalized()
	}
	return m.prefs.AgentSecurity.normalized()
}

func (p AgentPrefs) newTaskApprovals() string {
	if p.NewTaskApprovals == "ask" {
		return "ask"
	}
	return "bypass"
}

func (p AgentPrefs) sendWith() string {
	if p.SendWith == "modEnter" {
		return "modEnter"
	}
	return "enter"
}

func (p AgentPrefs) whileRunning() string {
	if p.WhileRunning == "steer" {
		return "steer"
	}
	return "queue"
}

func (p AgentPrefs) toolCalls() string {
	if p.ToolCalls == "each" {
		return "each"
	}
	return "grouped"
}

func (p AgentPrefs) thinking() string {
	switch p.Thinking {
	case "expanded", "collapsed":
		return p.Thinking
	default:
		return "auto"
	}
}

// confirmRerunEnabled reports whether starting a configuration that already
// runs asks before stopping it. The current project's applied backend value
// wins over the legacy preferences fallback.
func (m *Model) confirmRerunEnabled() bool {
	if settings := m.run.runSettings; settings.loaded && settings.root == m.Root() {
		return settings.confirmRerun
	}
	if p := m.prefs.Run; p != nil && p.ConfirmRerun != nil {
		return *p.ConfirmRerun
	}
	return true
}

func (m *Model) confirmTerminateEnabled() bool {
	if settings := m.run.runSettings; settings.loaded && settings.root == m.Root() {
		return settings.confirmTerminate
	}
	if p := m.prefs.Run; p != nil && p.ConfirmTerminate != nil {
		return *p.ConfirmTerminate
	}
	return true
}

func (m *Model) showRunOnStart() bool {
	if settings := m.run.runSettings; settings.loaded && settings.root == m.Root() {
		return settings.showOnStart
	}
	if p := m.prefs.Run; p != nil && p.ShowOnStart != nil {
		return *p.ShowOnStart
	}
	return true
}

func (m *Model) runTerminationTimeout() time.Duration {
	if settings := m.run.runSettings; settings.loaded && settings.root == m.Root() {
		return settings.terminationTimeout
	}
	if p := m.prefs.Run; p != nil && p.TerminationTimeout != nil {
		return time.Duration(*p.TerminationTimeout) * time.Millisecond
	}
	return 3 * time.Second
}

// showRunWindow follows OpenIDEA's start order: an explicit false wins,
// then the configuration's focus/open choices, then the global setting.
func (m *Model) showRunWindow(config *openidea.RunConfig, requested bool) bool {
	if !requested {
		return false
	}
	if config != nil {
		if config.FocusToolWindow {
			return true
		}
		if config.OpenToolWindow != nil {
			return *config.OpenToolWindow
		}
	}
	return m.showRunOnStart()
}

// EditorAppearance is what Settings › Editor › Appearance, Breadcrumbs,
// Code Folding and General show, as editor.appearance.*, editor.softWrap and
// editor.general.scrollPastEnd are stored. The fields are the off switches,
// so the zero value is IDEA's default look.
type EditorAppearance struct {
	NoLineNumbers bool `json:"noLineNumbers,omitempty"`
	// LineNumberMode is "absolute", "relative" or "hybrid"; empty is absolute.
	LineNumberMode string `json:"lineNumberMode,omitempty"`
	ShowWhitespace bool   `json:"showWhitespace,omitempty"`
	NoIndentGuides bool   `json:"noIndentGuides,omitempty"`
	NoFolding      bool   `json:"noFolding,omitempty"`
	// FoldImports folds the first import run when an editor opens, as
	// editor.folding.imports does. The default is on, so the stored field is
	// the off switch.
	NoFoldImports bool `json:"noFoldImports,omitempty"`
	// BlockCaret draws the carets as blocks over the next character rather
	// than bars before it, as editor.appearance.blockCaret does.
	BlockCaret bool `json:"blockCaret,omitempty"`
	// CaretBlinking is on by default. These inverse fields preserve older
	// preference files while exposing the complete editor.appearance.* shape.
	NoCaretBlinking     bool `json:"noCaretBlinking,omitempty"`
	CaretBlinkPeriod    int  `json:"caretBlinkPeriod,omitempty"`
	SmoothCaretBlink    bool `json:"smoothCaretBlinking,omitempty"`
	SmoothCaretMovement bool `json:"smoothCaretMovement,omitempty"`
	// CaretMovementStyle is "snappy" or "gliding"; empty is snappy.
	CaretMovementStyle string `json:"caretMovementStyle,omitempty"`
	// HardWrap guides and an intention bulb are on by default.
	NoHardWrapGuides    bool `json:"noHardWrapGuides,omitempty"`
	MethodSeparators    bool `json:"methodSeparators,omitempty"`
	NoIntentionBulb     bool `json:"noIntentionBulb,omitempty"`
	RainbowIndentGuides bool `json:"rainbowIndentGuides,omitempty"`
	// The whitespace children are on by default when whitespace is shown.
	NoWhitespaceLeading  bool `json:"noWhitespaceLeading,omitempty"`
	NoWhitespaceInner    bool `json:"noWhitespaceInner,omitempty"`
	NoWhitespaceTrailing bool `json:"noWhitespaceTrailing,omitempty"`
	// SoftWrap wraps the files SoftWrapFiles matches, and ScrollPastEnd
	// scrolls past the last line; OpenIDEA defaults both off.
	SoftWrap      bool   `json:"softWrap,omitempty"`
	SoftWrapFiles string `json:"softWrapFiles,omitempty"`
	ScrollPastEnd bool   `json:"scrollPastEnd,omitempty"`
	// TabSize and UseTabCharacter are Other File Types' code-style fallback.
	// Language entries in editor.codeStyle.languages override them.
	TabSize         int  `json:"tabSize,omitempty"`
	UseTabCharacter bool `json:"useTabCharacter,omitempty"`
}

const (
	defaultCaretBlinkPeriod = 500
	minCaretBlinkPeriod     = 100
	maxCaretBlinkPeriod     = 2000
	defaultHardWrapColumn   = 120
)

// caretBlinkMillis returns the visible blink interval, normalizing preference
// files that predate the field or were edited by hand.
func (a EditorAppearance) caretBlinkMillis() int {
	if a.CaretBlinkPeriod < minCaretBlinkPeriod || a.CaretBlinkPeriod > maxCaretBlinkPeriod {
		return defaultCaretBlinkPeriod
	}
	return a.CaretBlinkPeriod
}

// caretMovement returns the validated movement style used by the smooth
// caret option.
func (a EditorAppearance) caretMovement() string {
	if a.CaretMovementStyle == "gliding" {
		return "gliding"
	}
	return "snappy"
}

// wrapGuides returns the hard-wrap guide and any future visual guides. MyGo
// currently takes guide columns directly on its editor state.
func (a EditorAppearance) wrapGuides() []int {
	if a.NoHardWrapGuides {
		return nil
	}
	return []int{defaultHardWrapColumn}
}

// FontPrefs is Settings › Editor › Font. Family empty is the bundled
// JetBrains Mono, LineHeight zero uses the size-derived default, and Weight
// zero is regular.
type FontPrefs struct {
	Family     string  `json:"family,omitempty"`
	LineHeight float32 `json:"lineHeight,omitempty"`
	Ligatures  bool    `json:"ligatures,omitempty"`
	Weight     int     `json:"weight,omitempty"`
	Fallback   string  `json:"fallback,omitempty"`
}

const (
	defaultFontLineHeight = 22
	minFontLineHeight     = 14
	maxFontLineHeight     = 40
)

// fontFamily is the primary family, the bundled JetBrains Mono when unset,
// followed by the fallback.
func (p FontPrefs) fontFamily() string {
	family := strings.TrimSpace(p.Family)
	if family == "" {
		family = oiFontMono
	}
	fallback := strings.TrimSpace(p.Fallback)
	if fallback != "" && fallback != family {
		family += ", " + fallback
	}
	return family
}

// weight is the main font's CSS weight.
func (p FontPrefs) weight() int {
	if p.Weight == 0 {
		return 400
	}
	return max(100, min((p.Weight+25)/50*50, 900))
}

// features turn ligatures on or off for the shaped editor text.
func (p FontPrefs) features() string {
	if p.Ligatures {
		return ""
	}
	return "liga=0,clig=0"
}

// lineHeightPixels is the configured line height, or OpenIDEA's
// size-dependent default when none is stored.
func (p FontPrefs) lineHeightPixels(size float32) float32 {
	if p.LineHeight >= minFontLineHeight && p.LineHeight <= maxFontLineHeight {
		return p.LineHeight
	}
	if size <= 0 {
		size = defaultFontSize
	}
	return defaultFontLineHeight + (size-defaultFontSize)*1.6
}

// GeneralPrefs is Settings › Editor › General. Inverse names keep the zero
// value at OpenIDEA's defaults where an option starts enabled.
type GeneralPrefs struct {
	WheelZoom                 bool   `json:"wheelZoom,omitempty"`
	NoDragAndDrop             bool   `json:"noDragAndDrop,omitempty"`
	CaretScroll               string `json:"caretScroll,omitempty"`
	PowerSaveMode             bool   `json:"powerSaveMode,omitempty"`
	NoSoftWrapIndent          bool   `json:"noSoftWrapIndent,omitempty"`
	NoKeepTrailingOnCaretLine bool   `json:"noKeepTrailingOnCaretLine,omitempty"`
	ReformatOnSave            bool   `json:"reformatOnSave,omitempty"`
	MarkdownMode              string `json:"markdownMode,omitempty"`
	MarkdownImageFolder       string `json:"markdownImageFolder,omitempty"`
	MarkdownImageNamePattern  string `json:"markdownImageNamePattern,omitempty"`
}

func (p GeneralPrefs) dragAndDrop() bool    { return !p.NoDragAndDrop }
func (p GeneralPrefs) softWrapIndent() bool { return !p.NoSoftWrapIndent }
func (p GeneralPrefs) keepTrailingOnCaretLine() bool {
	return !p.NoKeepTrailingOnCaretLine
}

func (p GeneralPrefs) markdownImageFolder() string {
	if strings.TrimSpace(p.MarkdownImageFolder) == "" {
		return "assets"
	}
	return strings.TrimSpace(p.MarkdownImageFolder)
}

func (p GeneralPrefs) markdownImageNamePattern() string {
	if strings.TrimSpace(p.MarkdownImageNamePattern) == "" {
		return "image-{timestamp}"
	}
	return strings.TrimSpace(p.MarkdownImageNamePattern)
}

func (p GeneralPrefs) markdownMode() markdownMode {
	switch markdownMode(strings.TrimSpace(p.MarkdownMode)) {
	case markdownEditorOnly, markdownEditorPreview, markdownPreviewOnly:
		return markdownMode(strings.TrimSpace(p.MarkdownMode))
	default:
		return markdownEditorPreview
	}
}

// InlayPrefs is Settings › Editor › Inlay Hints. The inverse names keep the
// zero value at OpenIDEA's defaults: usage counts, their "no usages" result
// and the code author are shown, and each metric sits at the declaration.
type InlayPrefs struct {
	NoCodeVisionUsages   bool   `json:"noCodeVisionUsages,omitempty"`
	NoCodeVisionAuthor   bool   `json:"noCodeVisionAuthor,omitempty"`
	HideCodeVisionNoUses bool   `json:"hideCodeVisionNoUsages,omitempty"`
	UsagesPosition       string `json:"codeVisionUsagesPosition,omitempty"`
	AuthorPosition       string `json:"codeVisionAuthorPosition,omitempty"`
}

// DebuggerPrefs are the debugger choices the native frontend consumes.
type DebuggerPrefs struct {
	NoShowOnBreakpoint        bool `json:"noShowOnBreakpoint,omitempty"`
	FocusAppOnBreakpoint      bool `json:"focusAppOnBreakpoint,omitempty"`
	HideOnTermination         bool `json:"hideOnTermination,omitempty"`
	BreakpointsAnyFile        bool `json:"breakpointsAnyFile,omitempty"`
	NoRunToCursorInlay        bool `json:"noRunToCursorInlay,omitempty"`
	NoInlineValues            bool `json:"noInlineValues,omitempty"`
	SortValues                bool `json:"sortValues,omitempty"`
	NoValueTooltip            bool `json:"noValueTooltip,omitempty"`
	NoValueTooltipOnSelection bool `json:"noValueTooltipOnSelection,omitempty"`
	ValueTooltipDelay         *int `json:"valueTooltipDelay,omitempty"`
	NoSkipLibraryCode         bool `json:"noSkipLibraryCode,omitempty"`
	NoPythonAttachSubprocess  bool `json:"noPythonAttachSubprocess,omitempty"`
	PythonGevent              bool `json:"pythonGevent,omitempty"`
}

func (p DebuggerPrefs) ShowOnBreakpoint() bool { return !p.NoShowOnBreakpoint }

func (p DebuggerPrefs) RunToCursorInlay() bool { return !p.NoRunToCursorInlay }

func (p DebuggerPrefs) InlineValues() bool { return !p.NoInlineValues }

func (p DebuggerPrefs) ValueTooltip() bool { return !p.NoValueTooltip }

func (p DebuggerPrefs) ValueTooltipOnSelection() bool {
	return !p.NoValueTooltipOnSelection
}

func (p DebuggerPrefs) TooltipDelay() int {
	if p.ValueTooltipDelay == nil {
		return 700
	}
	return *p.ValueTooltipDelay
}

func debuggerPrefsEqual(a, b DebuggerPrefs) bool {
	return a.NoShowOnBreakpoint == b.NoShowOnBreakpoint &&
		a.FocusAppOnBreakpoint == b.FocusAppOnBreakpoint &&
		a.HideOnTermination == b.HideOnTermination &&
		a.BreakpointsAnyFile == b.BreakpointsAnyFile &&
		a.NoRunToCursorInlay == b.NoRunToCursorInlay &&
		a.NoInlineValues == b.NoInlineValues &&
		a.SortValues == b.SortValues &&
		a.NoValueTooltip == b.NoValueTooltip &&
		a.NoValueTooltipOnSelection == b.NoValueTooltipOnSelection &&
		a.TooltipDelay() == b.TooltipDelay() &&
		a.NoSkipLibraryCode == b.NoSkipLibraryCode &&
		a.NoPythonAttachSubprocess == b.NoPythonAttachSubprocess &&
		a.PythonGevent == b.PythonGevent
}

func (p DebuggerPrefs) SkipLibraryCode() bool { return !p.NoSkipLibraryCode }

func (p DebuggerPrefs) PythonAttachSubprocess() bool {
	return !p.NoPythonAttachSubprocess
}

// CodeVisionUsages reports whether usage counts are drawn.
func (p InlayPrefs) CodeVisionUsages() bool { return !p.NoCodeVisionUsages }

// CodeVisionAuthor reports whether the latest author is drawn.
func (p InlayPrefs) CodeVisionAuthor() bool { return !p.NoCodeVisionAuthor }

// CodeVisionNoUsages reports whether a declaration with no uses says so.
func (p InlayPrefs) CodeVisionNoUsages() bool { return !p.HideCodeVisionNoUses }

// UsagesAtRight reports whether usage counts sit at the end of the line
// rather than at the declaration; "top" is the default in OpenIDEA.
func (p InlayPrefs) UsagesAtRight() bool { return p.UsagesPosition == "right" }

// AuthorAtRight reports whether the author sits at the end of the line.
func (p InlayPrefs) AuthorAtRight() bool { return p.AuthorPosition == "right" }

// defaultSoftWrapFiles is what editor.general.softWrapFiles defaults to.
const defaultSoftWrapFiles = "*.md; *.txt; *.rst; *.adoc"

// SavePrefs is Settings › Editor › General › Save Files: what a save does to
// the document first, as files.trimTrailingWhitespace,
// editor.general.removeTrailingBlankLines and files.ensureFinalNewline store
// it. Every switch is off in the zero value, where a save writes the text as
// it stands.
type SavePrefs struct {
	// TrimTrailing removes the spaces and tabs ending a line; TrimAllLines
	// cleans every line rather than only the ones changed since the last save.
	TrimTrailing bool `json:"trimTrailing,omitempty"`
	TrimAllLines bool `json:"trimAllLines,omitempty"`
	// KeepTrailingOnCaretLine preserves the caret's line while the other
	// modified lines are trimmed, as editor.general does.
	KeepTrailingOnCaretLine bool `json:"keepTrailingOnCaretLine,omitempty"`
	// DropTrailingBlankLines drops the empty lines at the end of the file.
	DropTrailingBlankLines bool `json:"dropTrailingBlankLines,omitempty"`
	// EnsureFinalNewline ends the file with a line break.
	EnsureFinalNewline bool `json:"ensureFinalNewline,omitempty"`
}

// empty reports a save that writes the text as it stands.
func (p SavePrefs) empty() bool { return p == SavePrefs{} }

// WrapsFile reports whether Soft-wrap files wraps file: a "*" pattern wraps
// every file, otherwise the base name must match one of the semicolon
// separated patterns, and no pattern wraps none.
func (a EditorAppearance) WrapsFile(file string) bool {
	if !a.SoftWrap {
		return false
	}
	base := filepath.Base(file)
	for _, p := range strings.Split(a.softWrapFiles(), ";") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if p == "*" {
			return true
		}
		if ok, _ := path.Match(p, base); ok {
			return true
		}
	}
	return false
}

// softWrapFiles is the pattern list the page shows: the stored one, or
// OpenIDEA's default list when none is stored.
func (a EditorAppearance) softWrapFiles() string {
	if strings.TrimSpace(a.SoftWrapFiles) == "" {
		return defaultSoftWrapFiles
	}
	return a.SoftWrapFiles
}

// BreadcrumbPrefs is Settings › Editor › Breadcrumbs. The zero value shows the
// chain at the top, as "top" is the placement editor.breadcrumbs.placement
// defaults to.
type BreadcrumbPrefs struct {
	Hidden            bool     `json:"hidden,omitempty"`
	Placement         string   `json:"placement,omitempty"`
	DisabledLanguages []string `json:"disabledLanguages,omitempty"`
}

// TabPrefs is Settings › Editor › Editor Tabs, as editor.tabs.* stores it. The
// inverted names keep the zero value OpenIDEA's default strip: tabs on top,
// extension and directory shown, the full path on hover and a close button on
// the right.
type TabPrefs struct {
	// Placement is "" for the top, "bottom", "left", "right" or "none" to hide
	// the strip.
	Placement         string `json:"placement,omitempty"`
	PinnedSeparateRow bool   `json:"pinnedSeparateRow,omitempty"`
	HideExtension     bool   `json:"hideExtension,omitempty"`
	// NoDirectoryForNonUnique hides the folder a tab appends when another tab
	// shows the same file name.
	NoDirectoryForNonUnique bool `json:"noDirectoryForNonUnique,omitempty"`
	MarkModified            bool `json:"markModified,omitempty"`
	HideFullPathOnHover     bool `json:"hideFullPathOnHover,omitempty"`
	NoCloseButton           bool `json:"noCloseButton,omitempty"`
	CloseButtonLeft         bool `json:"closeButtonLeft,omitempty"`
	SortAlphabetically      bool `json:"sortAlphabetically,omitempty"`
	OpenAtEnd               bool `json:"openAtEnd,omitempty"`
	// TabLimit is editor.tabs.tabLimit; zero is the default of 30.
	TabLimit int `json:"tabLimit,omitempty"`
	// ActivateOnClose is "left", "right" or "mru"; empty is OpenIDEA's left.
	ActivateOnClose string `json:"activateOnClose,omitempty"`
	// ClosingPolicy is "unchanged" or "lru"; empty is least recently used.
	ClosingPolicy string `json:"closingPolicy,omitempty"`
	// NoPreviewTab turns off OpenIDEA's preview tab, where a click in a results
	// list reuses one italic tab instead of adding a tab.
	NoPreviewTab bool `json:"noPreviewTab,omitempty"`
	// MultipleRows wraps tabs that don't fit into more rows instead of keeping
	// one row, as editor.tabs.multipleRows does.
	MultipleRows bool `json:"multipleRows,omitempty"`
	// SqueezeTabs narrows tabs that don't fit instead of scrolling one row, as
	// editor.tabs.oneRowOverflow "squeeze" does.
	SqueezeTabs bool `json:"squeezeTabs,omitempty"`
	// HideFileIcon leaves the file icon off each tab.
	HideFileIcon bool `json:"hideFileIcon,omitempty"`
}

// defaultTabLimit is where editor.tabs.tabLimit starts.
const defaultTabLimit = 30

// maxTabLimit is the top of the Tab limit field's range.
const maxTabLimit = 100

// limit is the tab limit the strip enforces: the stored one, or OpenIDEA's
// default 30.
func (t TabPrefs) limit() int {
	if t.TabLimit <= 0 {
		return defaultTabLimit
	}
	return t.TabLimit
}

// AtBottom reports whether the strip sits under the editor.
func (t TabPrefs) AtBottom() bool { return t.Placement == "bottom" }

// activation is which tab opens when the current one closes: the stored
// policy, or OpenIDEA's default of the tab on the left.
func (t TabPrefs) activation() string {
	switch t.ActivateOnClose {
	case "right", "mru":
		return t.ActivateOnClose
	}
	return "left"
}

// closingPolicy is how a tab beyond the limit is chosen. GoIDEA never closes
// unsaved text automatically, so unchanged prefers an untouched file and lru
// takes the oldest closeable tab.
func (t TabPrefs) closingPolicy() string {
	if t.ClosingPolicy == "unchanged" {
		return "unchanged"
	}
	return "lru"
}

// previews reports whether a navigation open lands in the preview tab.
func (t TabPrefs) previews() bool { return !t.NoPreviewTab }

// Hidden reports whether Tab placement is None.
func (t TabPrefs) Hidden() bool { return t.Placement == "none" }

// Vertical reports whether the strip sits on the editor's left or right, as a
// single scrolling column where Show tabs in does not apply.
func (t TabPrefs) Vertical() bool { return t.Placement == "left" || t.Placement == "right" }

// AtBottom reports whether the chain sits under the editor rather than above
// it.
func (b BreadcrumbPrefs) AtBottom() bool { return b.Placement == "bottom" }

const (
	defaultFontSize = 13
	minFontSize     = 10
	maxFontSize     = 24
)

// LoadPreferences reads the settings at path; a missing or unreadable file is
// the default, so a broken file never keeps the app from starting.
func LoadPreferences(path string) Preferences {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Preferences{}
	}
	var p Preferences
	if json.Unmarshal(raw, &p) != nil {
		return Preferences{}
	}
	return p
}

// Save writes the preferences atomically, like the session.
func (p Preferences) Save(path string) error {
	raw, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, raw)
}

// themePreference maps a stored value to the theme GoIDEA draws, or nil to
// follow the system's appearance.
func (p Preferences) themePreference() *ui.Theme {
	return themeForName(p.Theme)
}

// preferredTheme resolves the concrete theme used for an operating-system
// appearance. OpenIDEA stores a separate preferred theme for light and dark
// mode, with light and dark as their defaults.
func (p Preferences) preferredTheme(dark bool) *ui.Theme {
	return themeForName(p.preferredThemeName(dark))
}

// preferredThemeName resolves one operating-system appearance to a concrete
// built-in theme. Light and dark are OpenIDEA's defaults.
func (p Preferences) preferredThemeName(dark bool) string {
	name := p.PreferredLightTheme
	if dark {
		name = p.PreferredDarkTheme
	}
	return normalizedPreferredThemeName(name, dark)
}

// normalizedPreferredThemeName accepts only concrete themes and falls back to
// the matching OpenIDEA default for an absent or obsolete value.
func normalizedPreferredThemeName(name string, dark bool) string {
	if name == "light" || name == "dark" {
		return name
	}
	if dark {
		return "dark"
	}
	return "light"
}

// storedPreferredThemeName omits the default from JSON while preserving a
// deliberate cross-appearance choice.
func storedPreferredThemeName(name string, dark bool) string {
	name = normalizedPreferredThemeName(name, dark)
	if dark && name == "dark" || !dark && name == "light" {
		return ""
	}
	return name
}

// themeWithAccent is the chosen appearance with the accent the user picked;
// nil still follows the system's appearance, with the accent applied by the
// same override when the window draws.
func (p Preferences) themeWithAccent() *ui.Theme {
	t := p.themePreference()
	if t == nil {
		return nil
	}
	return p.accentedTheme(t)
}

// systemThemeWithAccent is themeWithAccent for Sync with OS, using the
// preferred theme for the operating system's current mode.
func (p Preferences) systemThemeWithAccent(dark bool) *ui.Theme {
	return p.accentedTheme(p.preferredTheme(dark))
}

func (p Preferences) accentedTheme(t *ui.Theme) *ui.Theme {
	t.Font = p.uiFont()
	t.FontSize = p.uiFontSize()
	if c, ok := accentColorOf(p.Accent, t.Dark); ok {
		t.Accent = c
		t.AccentHover = c.Mix(t.Text, 0.14)
		t.AccentPressed = c.Mix(ui.Hex("#000000"), 0.2)
		t.Focus = c
	}
	if p.Appearance.ContrastScrollbars {
		t.Scrollbar = t.Scrollbar.Mix(t.Text, 0.55)
		t.ScrollbarWidth = 12
	}
	if p.Appearance.RedGreenVision {
		// Keep errors distinguishable from successful changes for common
		// protanopia and deuteranopia without changing the rest of the theme.
		t.Danger = ui.Hex("#d98500")
		t.Success = ui.Hex("#3977d4")
		t.Warning = ui.Hex("#b96a00")
	}
	return t
}

// fontSize returns the editor font size to use, with the default standing in
// for the unset value.
func (p Preferences) fontSize() float32 {
	if p.FontSize == 0 {
		return defaultFontSize
	}
	return p.FontSize
}

// zenWidth is the distraction free text column in columns, clamped like IDEA's
// Settings › Appearance field.
func (p Preferences) zenWidth() int {
	if p.ZenWidth == 0 {
		return defaultZenWidth
	}
	return max(minZenWidth, min(p.ZenWidth, maxZenWidth))
}

// todoSettings is Settings › Editor › TODO with OpenIDEA's defaults standing
// in for the unset value: the four built-in patterns, matching case
// sensitively and comments only.
func (p Preferences) todoSettings() (patterns []TodoPattern, caseSensitive, commentsOnly bool) {
	patterns = append([]TodoPattern(nil), defaultTodoPatterns...)
	if p.Todo == nil {
		return patterns, true, true
	}
	if p.Todo.Patterns != nil {
		patterns = append([]TodoPattern{}, p.Todo.Patterns...)
	}
	return patterns, !p.Todo.IgnoreCase, !p.Todo.AllText
}

// writeFileAtomic replaces path in one rename, so a crash mid-write leaves
// the previous content intact.
func writeFileAtomic(path string, raw []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".settings-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

// normalizeFiles sorts and deduplicates the paths of a session, keeping only
// the files that still exist.
func normalizeFiles(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	var out []string
	for _, p := range paths {
		if p == "" || seen[p] {
			continue
		}
		if st, err := os.Stat(p); err != nil || st.IsDir() {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// NotifyPrefs is Settings › Appearance & Behavior › Notifications. The zero
// value is OpenIDEA's default: balloons on, every group balloons and logs.
type NotifyPrefs struct {
	// NoBalloons turns balloons off everywhere; the answer to a command the
	// user ran still pops up, as userInitiated does upstream.
	NoBalloons bool `json:"noBalloons,omitempty"`
	// Sound plays the system alert for a balloon, as "Play sound" does.
	Sound bool `json:"sound,omitempty"`
	// Groups holds each group's choice; a group with no entry balloons and
	// logs.
	Groups map[string]NotifyGroupPrefs `json:"groups,omitempty"`
}

// NotifyGroupPrefs is one notification group's display and log choice.
type NotifyGroupPrefs struct {
	// Display is "balloon", "sticky" or "none"; empty is "balloon".
	Display string `json:"display,omitempty"`
	// NoLog keeps the group out of the Notifications tool window.
	NoLog bool `json:"noLog,omitempty"`
}

// notificationGroupTitles is OpenIDEA's Settings list, in its order.
var notificationGroupTitles = []struct {
	id    string
	title string
}{
	{"system", "System Messages"},
	{"lsp", "Language Servers"},
	{"vcs", "Version Control"},
	{"run", "Run and Debug"},
	{"tools", "External Tools"},
	{"search", "Find in Files"},
	{"plugins", "Plugins"},
	{"updates", "IDE Updates"},
	{"distractionFree", "Distraction Free Mode"},
}

// notifyGroups copies the stored notification groups, never nil so the
// settings page can stage into it.
func (p Preferences) notifyGroups() map[string]NotifyGroupPrefs {
	out := map[string]NotifyGroupPrefs{}
	if p.Notify != nil {
		for k, v := range p.Notify.Groups {
			out[k] = v
		}
	}
	return out
}

// sameNotifyGroups reports the staged groups equal to the stored ones,
// ignoring empty entries.
func sameNotifyGroups(a, b map[string]NotifyGroupPrefs) bool {
	norm := func(m map[string]NotifyGroupPrefs) map[string]NotifyGroupPrefs {
		out := map[string]NotifyGroupPrefs{}
		for k, v := range m {
			if v != (NotifyGroupPrefs{}) {
				out[k] = v
			}
		}
		return out
	}
	a, b = norm(a), norm(b)
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
