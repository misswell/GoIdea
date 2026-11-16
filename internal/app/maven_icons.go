package app

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// The Maven window's toolbar and tree icons, drawn as monochrome strokes in
// the colour the row asks for, like the rest of GoIDEA's chrome.
var (
	iconMavenStop       = welcomeIcon(`<rect width="12" height="12" x="6" y="6" rx="1"/>`)
	iconMavenRefresh    = welcomeIcon(`<path d="M21 12a9 9 0 1 1-2.64-6.36"/><path d="M21 3v6h-6"/>`)
	iconMavenSources    = welcomeIcon(`<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8Z"/><path d="M14 2v6h6"/><path d="M12 13v5"/><path d="m9 16 3 3 3-3"/>`)
	iconMavenPlus       = welcomeIcon(`<path d="M5 12h14"/><path d="M12 5v14"/>`)
	iconMavenRun        = welcomeIcon(`<path d="m7 4 12 8-12 8Z"/>`)
	iconMavenGoal       = welcomeIcon(`<circle cx="12" cy="12" r="8"/><path d="m10 8 6 4-6 4Z"/>`)
	iconMavenOffline    = welcomeIcon(`<path d="M12 3v10"/><path d="m8 9 4 4 4-4"/><path d="M4 17v2a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-2"/>`)
	iconMavenAnalyze    = welcomeIcon(`<circle cx="6" cy="6" r="2"/><circle cx="18" cy="6" r="2"/><circle cx="12" cy="18" r="2"/><path d="M6 8v2a2 2 0 0 0 2 2h8a2 2 0 0 0 2-2V8"/><path d="M12 12v4"/>`)
	iconMavenSkip       = welcomeIcon(`<path d="m5 5 10 7-10 7Z"/><path d="M19 5v14"/>`)
	iconMavenExpand     = welcomeIcon(`<path d="m7 6 5 5 5-5"/><path d="m7 13 5 5 5-5"/>`)
	iconMavenCollapse   = welcomeIcon(`<path d="m7 11 5-5 5 5"/><path d="m7 18 5-5 5 5"/>`)
	iconMavenProject    = welcomeIcon(`<path d="M4 7h16v12a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2Z"/><path d="M4 7V5a2 2 0 0 1 2-2h4l2 4"/>`)
	iconMavenDependency = welcomeIcon(`<path d="M21 8 12 3 3 8l9 5Z"/><path d="m3 8v8l9 5 9-5V8"/><path d="M12 13v8"/>`)
	iconMavenLoad       = welcomeIcon(`<path d="M12 3v10"/><path d="m8 9 4 4 4-4"/><path d="M4 17v2a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-2"/>`)
)

// iconPackage is a package row of the project tree: OpenIDEA draws packages
// with their own icon, not a folder's.
var iconPackage = welcomeIcon(`<path d="M21 8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16Z"/><path d="m3.3 7 8.7 5 8.7-5"/><path d="M12 22V12"/>`)

// The project tree's file-type marks: OpenIDEA gives a file the icon of what
// it is, not one generic page.
var (
	iconCode   = welcomeIcon(`<path d="m16 18 6-6-6-6"/><path d="m8 6-6 6 6 6"/>`)
	iconBraces = welcomeIcon(`<path d="M8 3H7a2 2 0 0 0-2 2v4a2 2 0 0 1-2 2 2 2 0 0 1 2 2v4a2 2 0 0 0 2 2h1"/><path d="M16 3h1a2 2 0 0 1 2 2v4a2 2 0 0 0 2 2 2 2 0 0 0-2 2v4a2 2 0 0 1-2 2h-1"/>`)
)

// treeFileIcon is the icon and tone of a file row, by name.
func treeFileIcon(name string, p *ui.Theme) (*ui.SVG, ui.Color) {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".java"):
		return iconCode, p.Accent
	case strings.HasSuffix(lower, ".class"):
		return iconPackage, p.Warning
	case strings.HasSuffix(lower, ".jar"):
		return iconPackage, p.Warning
	case strings.HasSuffix(lower, ".xml"):
		return iconCode, p.Danger
	case strings.HasSuffix(lower, ".yml"), strings.HasSuffix(lower, ".yaml"):
		return iconFile, p.Danger
	case strings.HasSuffix(lower, ".json"):
		return iconBraces, p.Warning
	case strings.HasSuffix(lower, ".sh"), strings.HasSuffix(lower, ".zsh"), strings.HasSuffix(lower, ".bash"),
		lower == "dockerfile", strings.HasPrefix(lower, "dockerfile."):
		return iconTerminal, p.Accent
	case strings.HasSuffix(lower, ".md"), strings.HasSuffix(lower, ".txt"), strings.HasSuffix(lower, ".log"):
		return iconFile, p.TextMuted
	}
	return iconFile, p.TextMuted
}
