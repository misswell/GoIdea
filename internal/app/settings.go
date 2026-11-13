package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"
)

// Session is what GoIDEA restores at the next start: the project folder, the
// files that had tabs, the one that was showing, and its bookmarks.
type Session struct {
	Root      string     `json:"root,omitempty"`
	Files     []string   `json:"files,omitempty"`
	Focus     string     `json:"focus,omitempty"`
	Bookmarks []Bookmark `json:"bookmarks,omitempty"`
	// Pinned are the open files whose tabs were pinned, which OpenIDEA keeps
	// at the left of the strip across starts.
	Pinned []string `json:"pinned,omitempty"`
	// PathBookmarks are the files marked with Toggle File Bookmark.
	PathBookmarks []PathBookmark `json:"pathBookmarks,omitempty"`
	// BookmarkLists are the named groups the tool window shows, and DefaultList
	// is the one new bookmarks go to.
	BookmarkLists []BookmarkList `json:"bookmarkLists,omitempty"`
	DefaultList   string         `json:"defaultList,omitempty"`
	// RecentProjects are the folders the Welcome screen lists, most recent
	// first.
	RecentProjects []string `json:"recentProjects,omitempty"`
	// Layouts are each project's workbench layout, by project folder, so a
	// project comes back the way it was left, as OpenIDEA stores a tool
	// window layout per project root.
	Layouts map[string]Layout `json:"layouts,omitempty"`
	// RunHistory is each project's Run Anything history, by project folder,
	// most recent first, as OpenIDEA keeps it per project root.
	RunHistory map[string][]string `json:"runHistory,omitempty"`
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
}

// clone copies a layout, so a saved snapshot never shares maps with the one
// the main thread keeps changing.
func (l Layout) clone() Layout {
	out := l
	out.Bottom = append([]string(nil), l.Bottom...)
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
	Theme    string  `json:"theme,omitempty"`
	FontSize float32 `json:"fontSize,omitempty"`
	// Zen is Distraction Free Mode, which survives a restart as IDEA's does,
	// and ZenWidth is its text column in columns; zero is the default.
	Zen      bool `json:"distractionFree,omitempty"`
	ZenWidth int  `json:"distractionFreeWidth,omitempty"`
	// Editor are Settings › Editor › Appearance, whose toggles are on by
	// default; the inverse names keep the zero value meaning that.
	Editor EditorAppearance `json:"editor,omitempty"`
	// Breadcrumbs are Settings › Editor › Breadcrumbs: shown above the editor
	// unless hidden, as editor.breadcrumbs.show and .placement store.
	Breadcrumbs BreadcrumbPrefs `json:"breadcrumbs,omitempty"`
	// MainToolbar is the title bar's saved layout, nil for the default one.
	MainToolbar *toolbarLayout `json:"mainToolbar,omitempty"`
	// Plugins are each plugin's settings, by name, under the same "plugins"
	// key Mayoi uses; a plugin's "enabled" lives there.
	Plugins map[string]json.RawMessage `json:"plugins,omitempty"`
}

// EditorAppearance is what Settings › Editor › Appearance and Code Folding
// show, as editor.appearance.* and editor.folding.showOutline are stored. The
// fields are the off switches, so the zero value is IDEA's default look.
type EditorAppearance struct {
	NoLineNumbers  bool `json:"noLineNumbers,omitempty"`
	ShowWhitespace bool `json:"showWhitespace,omitempty"`
	NoIndentGuides bool `json:"noIndentGuides,omitempty"`
	NoFolding      bool `json:"noFolding,omitempty"`
}

// BreadcrumbPrefs is Settings › Editor › Breadcrumbs. The zero value shows the
// chain at the top, as "top" is the placement editor.breadcrumbs.placement
// defaults to.
type BreadcrumbPrefs struct {
	Hidden    bool   `json:"hidden,omitempty"`
	Placement string `json:"placement,omitempty"`
}

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
