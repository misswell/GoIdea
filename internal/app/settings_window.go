package app

// Settings (⌘,) after OpenIDEA's settings dialog: a 980 DIP sheet with the
// page tree and its search on the left, the page under a breadcrumb on the
// right, and Cancel / Apply / OK below. The dialog stages its edits the way
// OpenIDEA's settings store does: the theme and the font preview live, Apply
// and OK write the staged choices to the preferences file, and Cancel or
// Escape puts the stored ones back. The page tree is built from registered
// pages exactly as model/pages.ts builds it: a dotted id nests under a
// registered prefix, anything else under a folder for its group.

import (
	"fmt"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"
)

// Metrics of SettingsDialog.module.css and SettingsDialog.tsx.
const (
	settingsDialogWidth     float32 = 980
	settingsDialogMaxHeight float32 = 720
	settingsDialogMinHeight float32 = 420
	settingsSideDefault     float32 = 240
	settingsSideMin         float32 = 160
	settingsTitleHeight     float32 = 40
	settingsHeaderHeight    float32 = 40
	settingsTreeRow         float32 = 24
	settingsFormRow         float32 = 28
	settingsFooterHeight    float32 = 52
)

// Settings page ids, the ones OpenIDEA's menu actions pass.
const (
	settingsPageKeymap           = "keymap"
	settingsPageAppearance       = "appearance"
	settingsPageGeneral          = "editor.general"
	settingsPageEditorAppearance = "editor.general.appearance"
	settingsPageBreadcrumbs      = "editor.general.breadcrumbs"
	settingsPageEditorTabs       = "editor.general.editorTabs"
	settingsPageCodeFolding      = "editor.general.codeFolding"
	settingsPageSystem           = "general"
	settingsPageDateFormats      = "general.dateFormats"
	settingsPageInlayHints       = "editor.inlayHints"
	settingsPageEditorFont       = "editor.font"
	settingsPageTodo             = "editor.todo"
	settingsPageNotifications    = "notifications"
	settingsPageAgentRegistry    = "agents.list"
	settingsPageAgentMCP         = "agents.mcpServers"
	settingsPageAgentPolicy      = "agents.policy"
	settingsPageAgents           = "agents.chat"
	settingsPageJava             = "languages.java"
	settingsPageLanguageServers  = "languages.servers"
	settingsPageIssueNavigation  = "vcs.issueNavigation"
	settingsPageUpdates          = "general.updates"
	settingsPageProjectTrust     = "projectTrust"
)

// settingsDraft is the value of each staged setting. The zero value follows
// the system's appearance; the font size is in points and the distraction
// free text width in columns. The appearance switches are the shown state, the
// way the page reads, while the stored fields are the off switches.
type settingsDraft struct {
	theme          string
	fontSize       float32
	font           FontPrefs
	dateFormat     DateFormatPrefs
	reopenLast     bool
	zenWidth       int
	lineNumbers    bool
	lineNumberMode string
	blockCaret     bool
	showWhitespace bool
	indentGuides   bool
	showFolding    bool
	// showBreadcrumbs and breadcrumbPlacement are Breadcrumbs' switches, the
	// placement "top" or "bottom" as the page's radios read.
	showBreadcrumbs     bool
	breadcrumbPlacement string
	breadcrumbDisabled  []string
	// softWrap, softWrapFiles and scrollPastEnd are General's Soft Wraps and
	// Virtual Space switches.
	softWrap                 bool
	softWrapFiles            string
	softWrapIndent           bool
	scrollPastEnd            bool
	wheelZoom                bool
	dragAndDrop              bool
	caretScroll              string
	keepTrailingOnCaretLine  bool
	reformatOnSave           bool
	markdownImageFolder      string
	markdownImageNamePattern string
	// The Save Files switches clean the document on every save.
	trimTrailing           bool
	trimAllLines           bool
	dropTrailingBlankLines bool
	ensureFinalNewline     bool
	// The tab fields are Editor Tabs' staged strip: where it sits, what a tab
	// shows, where its close button is, how tabs are ordered, the limit and
	// which tab opens when the current one closes.
	tabPlacement              string
	pinnedSeparateRow         bool
	showExtension             bool
	showDirectoryForNonUnique bool
	markModified              bool
	showFullPathOnHover       bool
	closePosition             string
	sortAlphabetically        bool
	openAtEnd                 bool
	tabLimit                  int
	activateOnClose           string
	tabClosingPolicy          string
	// noPreviewTab is Opening Policy's "Enable preview tab" turned off.
	noPreviewTab bool
	// multipleRows, squeezeTabs and showFileIcon are Appearance's "Show tabs in"
	// choice and the file icon switch.
	multipleRows bool
	squeezeTabs  bool
	showFileIcon bool
	// The Inlay Hints page stages Code Vision's metrics and their placement.
	codeVisionUsages         bool
	codeVisionAuthor         bool
	codeVisionNoUsages       bool
	codeVisionUsagesPosition string
	codeVisionAuthorPosition string
	// The TODO page stages its keywords, whether matching is case sensitive
	// and whether only comments count.
	todoPatterns      []TodoPattern
	todoCaseSensitive bool
	todoCommentsOnly  bool
	// The Notifications page stages the global balloon switch and each
	// group's display and log choice.
	notifyBalloons bool
	notifySound    bool
	notifyGroups   map[string]NotifyGroupPrefs
	// The Agents › Chat page stages the composer and conversation choices.
	agents   AgentPrefs
	security AgentSecurityPrefs
	// The Command Permissions page's two add fields.
	securityAllowDraft string
	securityBlockDraft string
	// Issue Navigation's table selection, inline editor and pending row.
	issueLinks     []IssueLinkPrefs
	issueSelected  int
	issueEditing   int
	issueLinkDraft IssueLinkPrefs
	// updateAutoCheck and updateWhatsNew are Settings › Updates' shown
	// switches, updateSource the feed choice, and updateLastChecked preserved
	// from the stored prefs while the page is staged.
	updateAutoCheck   bool
	updateWhatsNew    bool
	updateSource      string
	updateLastChecked int64
	// keymap is the staged override map. A nil map means defaults.
	keymap map[string][]string
}

// notifyPrefs is the staged Notifications page as the preferences store it.
func (d settingsDraft) notifyPrefs() *NotifyPrefs {
	groups := map[string]NotifyGroupPrefs{}
	for k, v := range d.notifyGroups {
		if v != (NotifyGroupPrefs{}) {
			groups[k] = v
		}
	}
	if d.notifyBalloons && !d.notifySound && len(groups) == 0 {
		return nil
	}
	return &NotifyPrefs{NoBalloons: !d.notifyBalloons, Sound: d.notifySound, Groups: groups}
}

func cloneIssueLinks(rows []IssueLinkPrefs) []IssueLinkPrefs {
	return append([]IssueLinkPrefs(nil), rows...)
}

func sameIssueLinks(a, b []IssueLinkPrefs) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// appearance is the staged switches as the editor stores them.
func (d settingsDraft) appearance() EditorAppearance {
	files := d.softWrapFiles
	if files == defaultSoftWrapFiles {
		files = ""
	}
	lineNumberMode := ""
	if d.lineNumberMode == "relative" {
		lineNumberMode = "relative"
	}
	return EditorAppearance{
		NoLineNumbers:  !d.lineNumbers,
		LineNumberMode: lineNumberMode,
		BlockCaret:     d.blockCaret,
		ShowWhitespace: d.showWhitespace,
		NoIndentGuides: !d.indentGuides,
		NoFolding:      !d.showFolding,
		SoftWrap:       d.softWrap,
		SoftWrapFiles:  files,
		ScrollPastEnd:  d.scrollPastEnd,
	}
}

// breadcrumbs is the staged breadcrumb switches as the preferences store
// them: a top placement is the default, so only "bottom" is stored.
func (d settingsDraft) breadcrumbs() BreadcrumbPrefs {
	b := BreadcrumbPrefs{
		Hidden:            !d.showBreadcrumbs,
		DisabledLanguages: uniqueSortedStrings(d.breadcrumbDisabled),
	}
	if d.breadcrumbPlacement == "bottom" {
		b.Placement = "bottom"
	}
	return b
}

// tabPrefs is the staged Editor Tabs strip as the preferences store it, with
// the defaults left out so the zero value stays OpenIDEA's look.
func (d settingsDraft) tabPrefs() TabPrefs {
	t := TabPrefs{
		PinnedSeparateRow:       d.pinnedSeparateRow,
		HideExtension:           !d.showExtension,
		NoDirectoryForNonUnique: !d.showDirectoryForNonUnique,
		MarkModified:            d.markModified,
		HideFullPathOnHover:     !d.showFullPathOnHover,
		SortAlphabetically:      d.sortAlphabetically,
		OpenAtEnd:               d.openAtEnd,
		NoPreviewTab:            d.noPreviewTab,
		MultipleRows:            d.multipleRows,
		SqueezeTabs:             d.squeezeTabs,
		HideFileIcon:            !d.showFileIcon,
	}
	if d.tabPlacement != "top" {
		t.Placement = d.tabPlacement
	}
	switch d.closePosition {
	case "none":
		t.NoCloseButton = true
	case "left":
		t.CloseButtonLeft = true
	}
	if d.tabLimit != defaultTabLimit {
		t.TabLimit = d.tabLimit
	}
	if d.activateOnClose != "left" {
		t.ActivateOnClose = d.activateOnClose
	}
	if d.tabClosingPolicy == "unchanged" {
		t.ClosingPolicy = "unchanged"
	}
	return t
}

// todoPrefs is the staged TODO page as the preferences store it: nil while it
// still holds OpenIDEA's built-in patterns, so the stored file keeps the
// defaults out of sight. An empty list is kept, as removing every pattern is a
// choice.
func (d settingsDraft) todoPrefs() *TodoPrefs {
	if sameTodoPatterns(d.todoPatterns, defaultTodoPatterns) && d.todoCaseSensitive && d.todoCommentsOnly {
		return nil
	}
	return &TodoPrefs{
		Patterns:   append([]TodoPattern{}, d.todoPatterns...),
		IgnoreCase: !d.todoCaseSensitive,
		AllText:    !d.todoCommentsOnly,
	}
}

// savePrefs is the staged Save Files switches as the preferences store them:
// the All lines choice only counts while trailing spaces are trimmed.
func (d settingsDraft) savePrefs() SavePrefs {
	return SavePrefs{
		TrimTrailing:            d.trimTrailing,
		TrimAllLines:            d.trimTrailing && d.trimAllLines,
		KeepTrailingOnCaretLine: d.trimTrailing && d.keepTrailingOnCaretLine,
		DropTrailingBlankLines:  d.dropTrailingBlankLines,
		EnsureFinalNewline:      d.ensureFinalNewline,
	}
}

func (d settingsDraft) generalPrefs() GeneralPrefs {
	caretScroll := ""
	if d.caretScroll == "center" {
		caretScroll = "center"
	}
	markdownImageFolder := strings.TrimSpace(d.markdownImageFolder)
	if markdownImageFolder == "assets" {
		markdownImageFolder = ""
	}
	markdownImageNamePattern := strings.TrimSpace(d.markdownImageNamePattern)
	if markdownImageNamePattern == "image-{timestamp}" {
		markdownImageNamePattern = ""
	}
	return GeneralPrefs{
		WheelZoom:                 d.wheelZoom,
		NoDragAndDrop:             !d.dragAndDrop,
		CaretScroll:               caretScroll,
		NoSoftWrapIndent:          !d.softWrapIndent,
		NoKeepTrailingOnCaretLine: !d.keepTrailingOnCaretLine,
		ReformatOnSave:            d.reformatOnSave,
		MarkdownImageFolder:       markdownImageFolder,
		MarkdownImageNamePattern:  markdownImageNamePattern,
	}
}

func (d settingsDraft) dateFormatPrefs() *DateFormatPrefs {
	return dateFormatPrefsPointer(d.dateFormat)
}

func (d settingsDraft) updatePrefs() *UpdatePrefs {
	source := d.updateSource
	switch source {
	case "github", "cnb":
	default:
		source = "auto"
	}
	if d.updateAutoCheck && d.updateWhatsNew && source == "auto" && d.updateLastChecked == 0 {
		return nil
	}
	return &UpdatePrefs{
		NoAutoCheck: !d.updateAutoCheck,
		NoWhatsNew:  !d.updateWhatsNew,
		Source:      source,
		LastChecked: d.updateLastChecked,
	}
}

// sameUpdatePrefs compares the normalized Updates settings. The page's
// default source is Auto, whether that came from a missing value or "auto".
func sameUpdatePrefs(a, b *UpdatePrefs) bool {
	normalize := func(p *UpdatePrefs) UpdatePrefs {
		if p == nil {
			return UpdatePrefs{Source: "auto"}
		}
		out := *p
		switch out.Source {
		case "github", "cnb":
		default:
			out.Source = "auto"
		}
		return out
	}
	return normalize(a) == normalize(b)
}

func uniqueSortedStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(strings.ToLower(value))
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

func sameStrings(a, b []string) bool {
	a = uniqueSortedStrings(a)
	b = uniqueSortedStrings(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sameBreadcrumbPrefs(a, b BreadcrumbPrefs) bool {
	return a.Hidden == b.Hidden && a.Placement == b.Placement &&
		sameStrings(a.DisabledLanguages, b.DisabledLanguages)
}

func normalizedCaretScroll(mode string) string {
	if mode == "center" {
		return "center"
	}
	return "minimize"
}

// inlayPrefs is the staged Inlay Hints page, with OpenIDEA's defaults left
// out of the stored file.
func (d settingsDraft) inlayPrefs() InlayPrefs {
	p := InlayPrefs{
		NoCodeVisionUsages:   !d.codeVisionUsages,
		NoCodeVisionAuthor:   !d.codeVisionAuthor,
		HideCodeVisionNoUses: !d.codeVisionNoUsages,
	}
	if d.codeVisionUsagesPosition == "right" {
		p.UsagesPosition = "right"
	}
	if d.codeVisionAuthorPosition == "right" {
		p.AuthorPosition = "right"
	}
	return p
}

// settingsPageSpec is a settings page this build carries: what
// pages/builtins.tsx registers for it, plus the section that draws it.
type settingsPageSpec struct {
	id       string
	title    string
	group    string
	order    int
	keywords []string
	labels   []string
	section  string
}

// settingsPages are the registered pages, with the upstream title, group,
// order and keywords. Only pages GoIDEA really implements are registered:
// editor.general keeps its Appearance sub-page, and the group folders are
// synthesized from these.
func settingsPages() []settingsPageSpec {
	return []settingsPageSpec{
		{
			id:    settingsPageAppearance,
			title: "Appearance",
			group: "Appearance & Behavior",
			order: 0,
			keywords: []string{
				"theme", "colour", "color", "dark", "light", "zoom", "font", "compact",
			},
			labels:  []string{"Theme", "Sync with OS", "Light", "Dark"},
			section: "appearance",
		},
		{
			id:    settingsPageSystem,
			title: "System Settings",
			group: "Appearance & Behavior",
			order: 1,
			keywords: []string{
				"general", "startup", "exit", "quit", "autosave", "save",
				"local history", "welcome", "editor tabs",
			},
			labels: []string{
				"Reopen projects on startup", "Local data directory", "Show in Finder",
			},
			section: "system",
		},
		{
			id:    settingsPageDateFormats,
			title: "Date Formats",
			group: "Appearance & Behavior",
			order: 1,
			keywords: []string{
				"date", "time", "pretty", "24-hour",
			},
			labels: []string{
				"Override system date and time format", "Date format",
				"Placeholders: yyyy, yy, MMMM, MMM, MM, M, dd, d",
				"Use 24-hour time format", "Use pretty formatting",
				"Show \"Today\", \"Yesterday\" and \"N min ago\" for recent dates",
				"Preview",
			},
			section: "dateFormats",
		},
		{
			id:    settingsPageNotifications,
			title: "Notifications",
			group: "Appearance & Behavior",
			order: 5,
			keywords: []string{
				"notifications", "balloons", "popup", "sticky", "log", "groups",
			},
			labels:  []string{"Display balloon notifications"},
			section: "notifications",
		},
		{
			id:    settingsPageKeymap,
			title: "Keymap",
			order: 10,
			keywords: []string{
				"shortcut", "keyboard", "hotkey", "binding", "preset", "vs code", "intellij",
			},
			section: "keymap",
		},
		{
			id:       settingsPageGeneral,
			title:    "General",
			group:    "Editor",
			order:    20,
			keywords: []string{"mouse", "scroll", "wrap", "trailing spaces", "save"},
			labels: []string{
				"Markdown Images", "Image folder", "Image name pattern",
				"Mouse Control", "Change font size with Ctrl/Command+Mouse Wheel",
				"Enable drag-and-drop functionality in editor",
				"Scrolling", "When caret is moved",
				"Soft Wraps", "Soft-wrap files", "File patterns",
				"Use original line's indent for wrapped parts",
				"Virtual Space", "Show virtual space at file bottom",
				"Save Files", "Remove trailing spaces on:", "Modified lines", "All lines",
				"Keep trailing spaces on caret line", "Reformat code on save",
				"Remove trailing blank lines at the end of saved files",
				"Ensure every saved file ends with a line break",
			},
			section: "general",
		},
		{
			id:    settingsPageEditorAppearance,
			title: "Appearance",
			group: "Editor",
			order: 1,
			keywords: []string{
				"caret", "line numbers", "whitespace", "indent guides", "distraction free", "zen",
			},
			labels: []string{
				"Use block caret", "Show line numbers", "Absolute", "Relative",
				"Show whitespaces", "Show indent guides",
				"Distraction free mode text width:",
			},
			section: "editorAppearance",
		},
		{
			id:    settingsPageBreadcrumbs,
			title: "Breadcrumbs",
			group: "Editor",
			order: 2,
			labels: []string{
				"Show breadcrumbs:", "Top", "Bottom", "Show breadcrumbs for:",
			},
			section: "breadcrumbs",
		},
		{
			id:    settingsPageEditorTabs,
			title: "Editor Tabs",
			group: "Editor",
			order: 5,
			keywords: []string{
				"tab limit", "preview tab", "close button", "pinned", "squeeze", "rows",
				"activate",
			},
			labels: []string{
				"Tab placement", "Top", "Bottom", "None",
				"Show pinned tabs in a separate row", "Show file extension",
				"Show directory for non-unique file names", "Mark modified (*)",
				"Show full path on mouse hover", "Close button position:", "Right", "Left",
				"Sort tabs alphabetically", "Open new tabs at the end",
				"Enable preview tab", "Tab limit",
				"When the current tab is closed, activate", "The tab on the left",
				"The tab on the right", "Most recently opened tab",
				"When the tab limit is reached", "Close non-modified files first",
				"Close least recently used files",
			},
			section: "editorTabs",
		},
		{
			id:    settingsPageCodeFolding,
			title: "Code Folding",
			group: "Editor",
			order: 4,
			keywords: []string{
				"fold", "collapse",
			},
			labels:  []string{"Show code folding outline", "Fold by default:"},
			section: "codeFolding",
		},
		{
			id:    settingsPageInlayHints,
			title: "Inlay Hints",
			group: "Editor",
			order: 33,
			keywords: []string{
				"hints", "code vision", "usages", "author", "position",
			},
			labels: []string{
				"Code Vision", "Usages", "Position", "Top", "Right",
				"Show “no usages”", "Code author",
			},
			section: "inlayHints",
		},
		{
			id:    settingsPageEditorFont,
			title: "Font",
			group: "Editor",
			order: 22,
			keywords: []string{
				"typeface", "monospace", "ligatures", "font size", "line spacing", "font weight", "bold",
			},
			labels: []string{
				"Font", "Show only monospaced fonts", "Size", "Line height",
				"Enable ligatures", "Typography Settings", "Main font weight",
				"Fallback font", "Preview",
			},
			section: "editorFont",
		},
		{
			id:    settingsPageTodo,
			title: "TODO",
			group: "Editor",
			order: 40,
			keywords: []string{
				"todo", "fixme", "hack", "pattern",
			},
			labels: []string{
				"Case sensitive", "Only in comments", "Patterns",
				"Add Pattern", "Restore Defaults", "Remove Pattern",
			},
			section: "todo",
		},
		{
			id:    settingsPageLanguageServers,
			title: "Language Servers",
			group: "Languages & Frameworks",
			order: 400,
			keywords: []string{
				"lsp", "language server", "debugger", "adapter", "dap", "install",
				"initialization options", "rust-analyzer", "gopls", "pyright",
			},
			labels:  []string{"Language Servers", "Install", "Uninstall", "Reload"},
			section: "languageServers",
		},
		{
			id:    settingsPageIssueNavigation,
			title: "Issue Navigation",
			group: "Version Control",
			order: 5,
			keywords: []string{
				"issue", "tracker", "jira", "youtrack", "link",
			},
			labels: []string{
				"Issue navigation", "Issue ID", "Issue Link", "Add",
				"Remove", "Edit", "Add JIRA Pattern", "Add YouTrack Pattern",
				"Add GitHub Pattern",
			},
			section: "issueNavigation",
		},
		{
			id:    settingsPageJava,
			title: "Java",
			group: "Languages & Frameworks",
			order: 395,
			keywords: []string{
				"jdk", "project sdk", "java home", "jdtls", "language server jdk",
				"jdk for importer", "jvm options", "vmargs", "lombok", "javaagent",
				"eclipse jdt", "shared jdtls", "linked builds", "server process",
			},
			section: "java",
		},
		{
			id:    settingsPageUpdates,
			title: "Updates",
			group: "Appearance & Behavior",
			order: 3,
			keywords: []string{
				"update", "upgrade", "version", "automatic", "GitHub",
			},
			labels: []string{
				"Current version", "Last checked", "Check Now",
				"Check for updates automatically", "Show What's New after an update",
				"Update source",
			},
			section: "updates",
		},
		{
			id:    settingsPageProjectTrust,
			title: "Project Trust",
			group: "Security",
			order: 500,
			keywords: []string{
				"trust", "security", "restricted mode", "trusted projects",
				"trusted locations", "revoke", "folder",
			},
			labels: []string{
				"Trusted projects", "Revoke Trust", "Trusted locations",
				"Add folder…", "Remove", "Every project inside a trusted location is trusted without asking.",
			},
			section: "projectTrust",
		},
		{
			id:    settingsPageAgentRegistry,
			title: "Coding Agents",
			group: "Agents",
			order: 450,
			keywords: []string{
				"agent", "acp", "registry", "install", "update", "coding agent",
			},
			labels: []string{
				"Agents", "Search agents", "Install", "Update", "Remove",
				"Installed", "Available", "Repository", "Website", "License",
			},
			section: "agentRegistry",
		},
		{
			id:    settingsPageAgentMCP,
			title: "External MCP Servers",
			group: "Agents",
			order: 475,
			keywords: []string{
				"mcp", "model context protocol", "servers", "tools", "agent",
			},
			labels: []string{
				"External MCP Servers", "Import from Project", "Browse",
				"Search MCP servers", "User", "Workspace", "Add MCP Server",
				"Configure MCP Server", "Delete Server",
			},
			section: "agentMCP",
		},
		{
			id:    settingsPageAgentPolicy,
			title: "Command Permissions",
			group: "Agents",
			order: 490,
			keywords: []string{
				"agent", "policy", "security", "allow", "block", "permission", "command",
			},
			labels: []string{
				"Allowed Commands", "Blocked Commands", "Add allowed command",
				"Add blocked command", "e.g. cargo test", "e.g. rm -rf *",
			},
			section: "agentPolicy",
		},
		{
			id:    settingsPageAgents,
			title: "Chat",
			group: "Agents",
			order: 460,
			keywords: []string{
				"agent", "claude", "codex", "gemini", "acp", "usage", "quota",
				"rate limit", "cost", "tokens", "meter", "send", "reasoning",
				"approval", "permission", "bypass",
			},
			labels: []string{
				"New tasks start", "Without asking for approval",
				"Asking for approval", "Send messages with",
				"While the agent works, sending",
				"Fold a finished turn's work", "Tool calls", "Reasoning",
				"Show tool output without opening it",
				"Reveal streaming text smoothly",
				"Shown in the Task Header", "Context window usage",
				"Time spent on the task", "Usage limits", "Session cost",
			},
			section: "agents",
		},
	}
}

// settingsPageByID finds a registered page.
func settingsPageByID(id string) (settingsPageSpec, bool) {
	for _, p := range settingsPages() {
		if p.id == id {
			return p, true
		}
	}
	return settingsPageSpec{}, false
}

// settingsGroupKey is the key of the synthesized folder of a group.
func settingsGroupKey(group string) string { return "group:" + group }

// settingsGroupDescriptions are GROUP_DESCRIPTIONS from SettingsDialog.tsx.
var settingsGroupDescriptions = map[string]string{
	"Appearance & Behavior":  "Customize IDE appearance and behavior: change themes and font size, configure system settings and notifications.",
	"Editor":                 "Configure code editing: completion, folding, tabs, fonts, colors, code style and inspections.",
	"Languages & Frameworks": "Configure language servers, interpreters, SDKs and framework support.",
}

// settingsNode is one row of the page tree; page is nil on a group folder.
type settingsNode struct {
	key      string
	title    string
	page     *settingsPageSpec
	children []settingsNode
	order    int
	matched  bool
}

// settingsBuildNode is the tree while it is put together: pointers, so a
// page attached under a parent keeps gaining its own children.
type settingsBuildNode struct {
	key      string
	title    string
	page     *settingsPageSpec
	children []*settingsBuildNode
	order    int
}

// settingsTree builds the page tree as buildPageTree does: a page whose
// dotted id extends a registered page goes under it, otherwise under its
// group's folder, otherwise at the top level, ordered by order and title.
func settingsTree() []settingsNode {
	pages := settingsPages()
	ids := make(map[string]bool, len(pages))
	for _, p := range pages {
		ids[p.id] = true
	}
	nodes := make(map[string]*settingsBuildNode, len(pages))
	for i := range pages {
		p := &pages[i]
		nodes[p.id] = &settingsBuildNode{key: p.id, title: p.title, page: p, order: p.order}
	}
	var roots []*settingsBuildNode
	groups := map[string]*settingsBuildNode{}
	for i := range pages {
		p := &pages[i]
		node := nodes[p.id]
		parent := ""
		for at := strings.LastIndex(p.id, "."); at > 0; at = strings.LastIndex(p.id[:at], ".") {
			if ids[p.id[:at]] {
				parent = p.id[:at]
				break
			}
		}
		switch {
		case parent != "":
			nodes[parent].children = append(nodes[parent].children, node)
		case p.group != "":
			g, ok := groups[p.group]
			if !ok {
				g = &settingsBuildNode{key: settingsGroupKey(p.group), title: p.group, order: node.order}
				groups[p.group] = g
				roots = append(roots, g)
			}
			g.order = min(g.order, node.order)
			g.children = append(g.children, node)
		default:
			roots = append(roots, node)
		}
	}
	out := settingsValueNodes(roots)
	settingsSortNodes(out)
	return out
}

// settingsValueNodes copies the built tree into values, so each frame's tree
// is its own.
func settingsValueNodes(nodes []*settingsBuildNode) []settingsNode {
	out := make([]settingsNode, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, settingsNode{
			key:      n.key,
			title:    n.title,
			page:     n.page,
			children: settingsValueNodes(n.children),
			order:    n.order,
		})
	}
	return out
}

// settingsSortNodes orders each level by order, then title, as byOrder does.
func settingsSortNodes(nodes []settingsNode) {
	sort.SliceStable(nodes, func(i, j int) bool {
		if nodes[i].order != nodes[j].order {
			return nodes[i].order < nodes[j].order
		}
		return nodes[i].title < nodes[j].title
	})
	for i := range nodes {
		settingsSortNodes(nodes[i].children)
	}
}

// settingsFlattenAll lists the nodes depth first.
func settingsFlattenAll(nodes []settingsNode) []settingsNode {
	var out []settingsNode
	for _, n := range nodes {
		out = append(out, n)
		out = append(out, settingsFlattenAll(n.children)...)
	}
	return out
}

// settingsBreadcrumb is the titles from the root to key.
func settingsBreadcrumb(nodes []settingsNode, key string) []string {
	for _, n := range nodes {
		if n.key == key {
			return []string{n.title}
		}
		if sub := settingsBreadcrumb(n.children, key); len(sub) > 0 {
			return append([]string{n.title}, sub...)
		}
	}
	return nil
}

// settingsFindNode finds a node by key anywhere in the tree.
func settingsFindNode(nodes []settingsNode, key string) (settingsNode, bool) {
	for _, n := range nodes {
		if n.key == key {
			return n, true
		}
		if found, ok := settingsFindNode(n.children, key); ok {
			return found, true
		}
	}
	return settingsNode{}, false
}

// settingsQueryTerms lower-cases and splits the query, as queryTerms does.
func settingsQueryTerms(query string) []string {
	return strings.Fields(strings.ToLower(query))
}

// settingsMatches reports whether every term occurs in one of the texts.
func settingsMatches(texts, terms []string) bool {
	if len(terms) == 0 {
		return true
	}
	for _, term := range terms {
		found := false
		for _, text := range texts {
			if strings.Contains(strings.ToLower(text), term) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// settingsPageTexts is the search text of a page: title, group, keywords and
// the labels of its options.
func settingsPageTexts(p settingsPageSpec) []string {
	texts := []string{p.title, p.group}
	texts = append(texts, p.keywords...)
	texts = append(texts, p.labels...)
	return texts
}

// settingsFilterTree keeps the pages matching the query and their ancestors.
// A page also matches on its title path, so "editor font" finds Font.
func settingsFilterTree(nodes []settingsNode, query string, path []string) []settingsNode {
	terms := settingsQueryTerms(query)
	if len(terms) == 0 {
		return nodes
	}
	return settingsFilterTreeMatching(nodes, terms, path, settingsTitlePathMatches(nodes, terms, path))
}

// settingsTitlePathMatches makes a query such as "editor font" select the
// Font page through its breadcrumb rather than every page that mentions
// "font". Lesser queries still search titles, keywords and labels below.
func settingsTitlePathMatches(nodes []settingsNode, terms []string, path []string) bool {
	phrase := strings.Join(terms, " ")
	for _, n := range nodes {
		trail := append(append([]string{}, path...), n.title)
		if strings.Contains(strings.ToLower(strings.Join(trail, " ")), phrase) {
			return true
		}
		if settingsTitlePathMatches(n.children, terms, trail) {
			return true
		}
	}
	return false
}

func settingsFilterTreeMatching(
	nodes []settingsNode,
	terms []string,
	path []string,
	titlePathOnly bool,
) []settingsNode {
	var out []settingsNode
	for _, n := range nodes {
		trail := append(append([]string{}, path...), n.title)
		children := settingsFilterTreeMatching(n.children, terms, trail, titlePathOnly)
		self := false
		if n.page != nil {
			texts := append(append([]string{}, trail...), settingsPageTexts(*n.page)...)
			if titlePathOnly {
				self = strings.Contains(
					strings.ToLower(strings.Join(trail, " ")),
					strings.Join(terms, " "),
				)
			} else {
				self = settingsMatches(texts, terms)
			}
		}
		if self || len(children) > 0 {
			n.children = children
			n.matched = self
			out = append(out, n)
		}
	}
	return out
}

// settingsRow is a visible tree row with its depth.
type settingsRow struct {
	node  settingsNode
	depth int
}

// settingsFlatten lists the visible rows: a node's children show unless its
// caret closed it, and a search opens everything it kept.
func settingsFlatten(nodes []settingsNode, depth int, collapsed map[string]bool, openAll bool) []settingsRow {
	var out []settingsRow
	for _, n := range nodes {
		out = append(out, settingsRow{node: n, depth: depth})
		if len(n.children) == 0 || (collapsed[n.key] && !openAll) {
			continue
		}
		out = append(out, settingsFlatten(n.children, depth+1, collapsed, openAll)...)
	}
	return out
}

// settingsVisibleRows is the tree after the query, with its carets applied.
func (m *Model) settingsVisibleRows() []settingsRow {
	return settingsFlatten(settingsFilterTree(settingsTree(), m.settingsQuery, nil), 0,
		m.settingsCollapsed, m.settingsQuery != "")
}

// settingsCurrent is the selected node; a key that is not in the tree falls
// back to the first page, as SettingsLayout does.
func (m *Model) settingsCurrent() (settingsNode, bool) {
	tree := settingsTree()
	if n, ok := settingsFindNode(tree, m.settingsPage); ok {
		return n, true
	}
	for _, n := range settingsFlattenAll(tree) {
		if n.page != nil {
			return n, true
		}
	}
	return settingsNode{}, false
}

// settingsSelectPage shows a page and records it in the Back / Forward
// history.
func (m *Model) settingsSelectPage(key string) {
	key = m.settingsNormalizeKey(key)
	if key == "" || key == m.settingsPage {
		return
	}
	m.settingsPage = key
	if key == settingsPageJava {
		m.LoadJavaSettings()
	}
	if key == settingsPageLanguageServers {
		m.loadLanguageServerSettings()
	}
	if key == settingsPageProjectTrust {
		m.loadProjectTrust()
	}
	if m.settingsHistoryAt < 0 || m.settingsHistoryAt >= len(m.settingsHistory) {
		m.settingsHistory = []string{key}
		m.settingsHistoryAt = 0
	} else {
		m.settingsHistory = append(m.settingsHistory[:m.settingsHistoryAt+1], key)
		m.settingsHistoryAt = len(m.settingsHistory) - 1
	}
	if settingsPageUsesAgentBackend(key) {
		m.loadAgentSettings()
	}
	m.invalidate()
}

// settingsNormalizeKey maps a page id or group folder to a key the tree has.
func (m *Model) settingsNormalizeKey(key string) string {
	if key == "" {
		return ""
	}
	if _, ok := settingsFindNode(settingsTree(), key); ok {
		return key
	}
	return ""
}

// settingsStepHistory moves Back (−1) or Forward (+1).
func (m *Model) settingsStepHistory(delta int) {
	at := m.settingsHistoryAt + delta
	if at < 0 || at >= len(m.settingsHistory) {
		return
	}
	key := m.settingsNormalizeKey(m.settingsHistory[at])
	if key == "" {
		return
	}
	m.settingsHistoryAt = at
	m.settingsPage = key
	if key == settingsPageJava {
		m.LoadJavaSettings()
	}
	if key == settingsPageLanguageServers {
		m.loadLanguageServerSettings()
	}
	if key == settingsPageProjectTrust {
		m.loadProjectTrust()
	}
	m.invalidate()
}

// settingsStepSelection moves the page selection through the visible pages,
// which the search field's arrows do while the caret stays in the field.
func (m *Model) settingsStepSelection(delta int) {
	var pages []string
	for _, row := range m.settingsVisibleRows() {
		if row.node.page != nil {
			pages = append(pages, row.node.key)
		}
	}
	if len(pages) == 0 {
		return
	}
	at := -1
	for i, key := range pages {
		if key == m.settingsPage {
			at = i
			break
		}
	}
	next := at + delta
	if next < 0 {
		next = 0
	}
	if next >= len(pages) {
		next = len(pages) - 1
	}
	m.settingsSelectPage(pages[next])
}

// settingsQueryChanged selects the first matched page once the query hides
// the current one, as SettingsLayout's onQuery does.
func (m *Model) settingsQueryChanged() {
	if len(settingsQueryTerms(m.settingsQuery)) == 0 {
		m.invalidate()
		return
	}
	rows := m.settingsVisibleRows()
	currentMatched := false
	for _, row := range rows {
		if row.node.key == m.settingsPage && row.node.page != nil && row.node.matched {
			currentMatched = true
			break
		}
	}
	if !currentMatched {
		for _, row := range rows {
			if row.node.page != nil && row.node.matched {
				m.settingsSelectPage(row.node.key)
				return
			}
		}
	}
	m.invalidate()
}

// settingsToggleNode opens or closes a tree node's caret.
func (m *Model) settingsToggleNode(key string) {
	if m.settingsCollapsed == nil {
		m.settingsCollapsed = map[string]bool{}
	}
	m.settingsCollapsed[key] = !m.settingsCollapsed[key]
	m.invalidate()
}

// --- Staging ---------------------------------------------------------------

// settingsDraftFromPrefs is the stored settings as the dialog stages them.
func (m *Model) settingsDraftFromPrefs() settingsDraft {
	placement := "top"
	if m.prefs.Breadcrumbs.AtBottom() {
		placement = "bottom"
	}
	lineNumberMode := "absolute"
	if m.prefs.Editor.LineNumberMode == "relative" {
		lineNumberMode = "relative"
	}
	tabPlacement := "top"
	switch m.prefs.Tabs.Placement {
	case "bottom", "left", "right", "none":
		tabPlacement = m.prefs.Tabs.Placement
	}
	closePosition := "right"
	if m.prefs.Tabs.NoCloseButton {
		closePosition = "none"
	}
	if m.prefs.Tabs.CloseButtonLeft {
		closePosition = "left"
	}
	todoPatterns, todoCase, todoComments := m.prefs.todoSettings()
	usagesPosition, authorPosition := "top", "top"
	if m.prefs.Inlay.UsagesAtRight() {
		usagesPosition = "right"
	}
	if m.prefs.Inlay.AuthorAtRight() {
		authorPosition = "right"
	}
	updateAuto, updateWhats, updateSource, updateLast := true, true, "auto", int64(0)
	if m.prefs.Updates != nil {
		updateAuto = !m.prefs.Updates.NoAutoCheck
		updateWhats = !m.prefs.Updates.NoWhatsNew
		switch m.prefs.Updates.Source {
		case "github", "cnb":
			updateSource = m.prefs.Updates.Source
		}
		updateLast = m.prefs.Updates.LastChecked
	}
	return settingsDraft{
		theme:                     m.prefs.Theme,
		fontSize:                  m.prefs.fontSize(),
		font:                      m.prefs.Font,
		dateFormat:                m.prefs.dateFormatPrefs(),
		reopenLast:                m.prefs.Startup == "reopen",
		zenWidth:                  m.prefs.zenWidth(),
		lineNumbers:               !m.prefs.Editor.NoLineNumbers,
		lineNumberMode:            lineNumberMode,
		blockCaret:                m.prefs.Editor.BlockCaret,
		showWhitespace:            m.prefs.Editor.ShowWhitespace,
		indentGuides:              !m.prefs.Editor.NoIndentGuides,
		showFolding:               !m.prefs.Editor.NoFolding,
		showBreadcrumbs:           !m.prefs.Breadcrumbs.Hidden,
		breadcrumbPlacement:       placement,
		breadcrumbDisabled:        uniqueSortedStrings(m.prefs.Breadcrumbs.DisabledLanguages),
		softWrap:                  m.prefs.Editor.SoftWrap,
		softWrapFiles:             m.prefs.Editor.softWrapFiles(),
		softWrapIndent:            m.prefs.General.softWrapIndent(),
		scrollPastEnd:             m.prefs.Editor.ScrollPastEnd,
		wheelZoom:                 m.prefs.General.WheelZoom,
		dragAndDrop:               m.prefs.General.dragAndDrop(),
		caretScroll:               normalizedCaretScroll(m.prefs.General.CaretScroll),
		keepTrailingOnCaretLine:   m.prefs.General.keepTrailingOnCaretLine(),
		reformatOnSave:            m.prefs.General.ReformatOnSave,
		markdownImageFolder:       m.prefs.General.markdownImageFolder(),
		markdownImageNamePattern:  m.prefs.General.markdownImageNamePattern(),
		trimTrailing:              m.prefs.SaveFiles.TrimTrailing,
		trimAllLines:              m.prefs.SaveFiles.TrimAllLines,
		dropTrailingBlankLines:    m.prefs.SaveFiles.DropTrailingBlankLines,
		ensureFinalNewline:        m.prefs.SaveFiles.EnsureFinalNewline,
		tabPlacement:              tabPlacement,
		pinnedSeparateRow:         m.prefs.Tabs.PinnedSeparateRow,
		showExtension:             !m.prefs.Tabs.HideExtension,
		showDirectoryForNonUnique: !m.prefs.Tabs.NoDirectoryForNonUnique,
		markModified:              m.prefs.Tabs.MarkModified,
		showFullPathOnHover:       !m.prefs.Tabs.HideFullPathOnHover,
		closePosition:             closePosition,
		sortAlphabetically:        m.prefs.Tabs.SortAlphabetically,
		openAtEnd:                 m.prefs.Tabs.OpenAtEnd,
		tabLimit:                  m.prefs.Tabs.limit(),
		activateOnClose:           m.prefs.Tabs.activation(),
		tabClosingPolicy:          m.prefs.Tabs.closingPolicy(),
		noPreviewTab:              m.prefs.Tabs.NoPreviewTab,
		multipleRows:              m.prefs.Tabs.MultipleRows,
		squeezeTabs:               m.prefs.Tabs.SqueezeTabs,
		showFileIcon:              !m.prefs.Tabs.HideFileIcon,
		codeVisionUsages:          m.prefs.Inlay.CodeVisionUsages(),
		codeVisionAuthor:          m.prefs.Inlay.CodeVisionAuthor(),
		codeVisionNoUsages:        m.prefs.Inlay.CodeVisionNoUsages(),
		codeVisionUsagesPosition:  usagesPosition,
		codeVisionAuthorPosition:  authorPosition,
		todoPatterns:              todoPatterns,
		todoCaseSensitive:         todoCase,
		todoCommentsOnly:          todoComments,
		notifyBalloons:            m.prefs.Notify == nil || !m.prefs.Notify.NoBalloons,
		notifySound:               m.prefs.Notify != nil && m.prefs.Notify.Sound,
		notifyGroups:              m.prefs.notifyGroups(),
		agents:                    m.agentPrefs(),
		security:                  m.agentSecurityPrefs(),
		issueLinks:                cloneIssueLinks(m.prefs.VCS.IssueNavigation),
		issueSelected:             -1,
		issueEditing:              -1,
		updateAutoCheck:           updateAuto,
		updateWhatsNew:            updateWhats,
		updateSource:              updateSource,
		updateLastChecked:         updateLast,
		keymap:                    cloneKeymap(m.prefs.Keymap),
	}
}

// settingsSetTheme stages the appearance and previews it in the window.
func (m *Model) settingsSetTheme(theme string) {
	switch theme {
	case "", "light", "dark":
	default:
		return
	}
	if m.settingsDraft.theme == theme {
		return
	}
	m.settingsDraft.theme = theme
	m.settingsPreview()
}

// settingsSetFont stages the editor font size, clamped to the readable range.
func (m *Model) settingsSetFont(size float32) {
	size = max(float32(minFontSize), min(size, float32(maxFontSize)))
	if m.settingsDraft.fontSize == size {
		return
	}
	m.settingsDraft.fontSize = size
	m.settingsPreview()
}

// settingsSetFontFamily stages an installed font family.
func (m *Model) settingsSetFontFamily(family string) {
	family = strings.TrimSpace(family)
	if family == oiFontMono {
		family = ""
	}
	if m.settingsDraft.font.Family == family {
		return
	}
	m.settingsDraft.font.Family = family
	m.settingsPreview()
}

// settingsSetFontLineHeight stages the line height in pixels; zero keeps the
// size-derived default.
func (m *Model) settingsSetFontLineHeight(height float32) {
	if height <= 0 {
		height = 0
	} else {
		height = max(float32(minFontLineHeight), min(height, float32(maxFontLineHeight)))
	}
	if m.settingsDraft.font.LineHeight == height {
		return
	}
	m.settingsDraft.font.LineHeight = height
	m.settingsPreview()
}

// settingsSetFontLigatures stages Enable ligatures.
func (m *Model) settingsSetFontLigatures(on bool) {
	if m.settingsDraft.font.Ligatures == on {
		return
	}
	m.settingsDraft.font.Ligatures = on
	m.settingsPreview()
}

// settingsSetFontWeight stages the main font's CSS weight.
func (m *Model) settingsSetFontWeight(weight int) {
	weight = max(100, min((weight+25)/50*50, 900))
	if m.settingsDraft.font.weight() == weight {
		return
	}
	m.settingsDraft.font.Weight = weight
	m.settingsPreview()
}

// settingsSetFontFallback stages the family used for unsupported glyphs.
func (m *Model) settingsSetFontFallback(family string) {
	family = strings.TrimSpace(family)
	if m.settingsDraft.font.Fallback == family {
		return
	}
	m.settingsDraft.font.Fallback = family
	m.settingsPreview()
}

// settingsSetZenWidth stages the distraction free text width, clamped like
// its field's range.
func (m *Model) settingsSetZenWidth(width int) {
	width = max(minZenWidth, min(width, maxZenWidth))
	if m.settingsDraft.zenWidth == width {
		return
	}
	m.settingsDraft.zenWidth = width
	m.invalidate()
}

// settingsSetLineNumbers stages the Show line numbers switch.
func (m *Model) settingsSetLineNumbers(on bool) {
	if m.settingsDraft.lineNumbers == on {
		return
	}
	m.settingsDraft.lineNumbers = on
	m.settingsPreview()
}

// settingsSetLineNumberMode stages Absolute or Relative line numbers.
func (m *Model) settingsSetLineNumberMode(mode string) {
	if mode != "relative" {
		mode = "absolute"
	}
	if m.settingsDraft.lineNumberMode == mode {
		return
	}
	m.settingsDraft.lineNumberMode = mode
	m.settingsPreview()
}

// settingsSetBlockCaret stages the Use block caret switch.
func (m *Model) settingsSetBlockCaret(on bool) {
	if m.settingsDraft.blockCaret == on {
		return
	}
	m.settingsDraft.blockCaret = on
	m.settingsPreview()
}

// settingsSetShowWhitespace stages the Show whitespaces switch.
func (m *Model) settingsSetShowWhitespace(on bool) {
	if m.settingsDraft.showWhitespace == on {
		return
	}
	m.settingsDraft.showWhitespace = on
	m.settingsPreview()
}

// settingsSetIndentGuides stages the Show indent guides switch.
func (m *Model) settingsSetIndentGuides(on bool) {
	if m.settingsDraft.indentGuides == on {
		return
	}
	m.settingsDraft.indentGuides = on
	m.settingsPreview()
}

// settingsSetShowFolding stages the Show code folding outline switch.
func (m *Model) settingsSetShowFolding(on bool) {
	if m.settingsDraft.showFolding == on {
		return
	}
	m.settingsDraft.showFolding = on
	m.settingsPreview()
}

// settingsSetShowBreadcrumbs stages the Show breadcrumbs switch.
func (m *Model) settingsSetShowBreadcrumbs(on bool) {
	if m.settingsDraft.showBreadcrumbs == on {
		return
	}
	m.settingsDraft.showBreadcrumbs = on
	m.settingsPreview()
}

// settingsSetBreadcrumbPlacement stages the Top / Bottom radios.
func (m *Model) settingsSetBreadcrumbPlacement(place string) {
	switch place {
	case "top", "bottom":
	default:
		return
	}
	if m.settingsDraft.breadcrumbPlacement == place {
		return
	}
	m.settingsDraft.breadcrumbPlacement = place
	m.settingsPreview()
}

// settingsToggleBreadcrumbLanguage stages whether a language keeps
// breadcrumbs; every other listed language remains on.
func (m *Model) settingsToggleBreadcrumbLanguage(language string) {
	language = strings.TrimSpace(strings.ToLower(language))
	if language == "" {
		return
	}
	out := make([]string, 0, len(m.settingsDraft.breadcrumbDisabled)+1)
	disabled := false
	for _, current := range m.settingsDraft.breadcrumbDisabled {
		if current == language {
			disabled = true
			continue
		}
		out = append(out, current)
	}
	if !disabled {
		out = append(out, language)
	}
	m.settingsDraft.breadcrumbDisabled = uniqueSortedStrings(out)
	m.settingsPreview()
}

// settingsSetSoftWrap stages the Soft-wrap files switch.
func (m *Model) settingsSetSoftWrap(on bool) {
	if m.settingsDraft.softWrap == on {
		return
	}
	m.settingsDraft.softWrap = on
	m.settingsPreview()
}

// settingsSetSoftWrapFiles stages General's File patterns field.
func (m *Model) settingsSetSoftWrapFiles(patterns string) {
	m.settingsDraft.softWrapFiles = patterns
	m.settingsPreview()
}

func (m *Model) settingsSetSoftWrapIndent(on bool) {
	if m.settingsDraft.softWrapIndent == on {
		return
	}
	m.settingsDraft.softWrapIndent = on
	m.invalidate()
}

func (m *Model) settingsSetWheelZoom(on bool) {
	if m.settingsDraft.wheelZoom == on {
		return
	}
	m.settingsDraft.wheelZoom = on
	m.invalidate()
}

func (m *Model) settingsSetDragAndDrop(on bool) {
	if m.settingsDraft.dragAndDrop == on {
		return
	}
	m.settingsDraft.dragAndDrop = on
	m.invalidate()
}

func (m *Model) settingsSetCaretScroll(mode string) {
	mode = normalizedCaretScroll(mode)
	if m.settingsDraft.caretScroll == mode {
		return
	}
	m.settingsDraft.caretScroll = mode
	m.invalidate()
}

func (m *Model) settingsSetKeepTrailingOnCaretLine(on bool) {
	if m.settingsDraft.keepTrailingOnCaretLine == on {
		return
	}
	m.settingsDraft.keepTrailingOnCaretLine = on
	m.invalidate()
}

func (m *Model) settingsSetReformatOnSave(on bool) {
	if m.settingsDraft.reformatOnSave == on {
		return
	}
	m.settingsDraft.reformatOnSave = on
	m.invalidate()
}

func (m *Model) settingsSetMarkdownImageFolder(folder string) {
	m.settingsDraft.markdownImageFolder = folder
	m.invalidate()
}

func (m *Model) settingsSetMarkdownImageNamePattern(pattern string) {
	m.settingsDraft.markdownImageNamePattern = pattern
	m.invalidate()
}

// settingsSetScrollPastEnd stages the Show virtual space at file bottom
// switch.
func (m *Model) settingsSetScrollPastEnd(on bool) {
	if m.settingsDraft.scrollPastEnd == on {
		return
	}
	m.settingsDraft.scrollPastEnd = on
	m.settingsPreview()
}

// settingsSetTrimTrailing stages Save Files' Remove trailing spaces switch.
func (m *Model) settingsSetTrimTrailing(on bool) {
	if m.settingsDraft.trimTrailing == on {
		return
	}
	m.settingsDraft.trimTrailing = on
	m.settingsPreview()
}

// settingsSetTrimAllLines stages whether trailing spaces come off every line
// or only the lines modified since the file was saved.
func (m *Model) settingsSetTrimAllLines(all bool) {
	if m.settingsDraft.trimAllLines == all {
		return
	}
	m.settingsDraft.trimAllLines = all
	m.settingsPreview()
}

// settingsSetDropTrailingBlankLines stages Remove trailing blank lines at the
// end of saved files.
func (m *Model) settingsSetDropTrailingBlankLines(on bool) {
	if m.settingsDraft.dropTrailingBlankLines == on {
		return
	}
	m.settingsDraft.dropTrailingBlankLines = on
	m.settingsPreview()
}

// settingsSetEnsureFinalNewline stages Ensure every saved file ends with a
// line break.
func (m *Model) settingsSetEnsureFinalNewline(on bool) {
	if m.settingsDraft.ensureFinalNewline == on {
		return
	}
	m.settingsDraft.ensureFinalNewline = on
	m.settingsPreview()
}

// settingsSetTabPlacement stages the Tab placement select: where the editor
// strip sits, one of the four edges, or none to hide it.
func (m *Model) settingsSetTabPlacement(place string) {
	switch place {
	case "top", "bottom", "left", "right", "none":
	default:
		return
	}
	if m.settingsDraft.tabPlacement == place {
		return
	}
	m.settingsDraft.tabPlacement = place
	m.settingsPreview()
}

// settingsSetMultipleRows stages "Show tabs in": multiple rows wraps tabs that
// don't fit, one row scrolls or squeezes them.
func (m *Model) settingsSetMultipleRows(on bool) {
	if m.settingsDraft.multipleRows == on {
		return
	}
	m.settingsDraft.multipleRows = on
	m.settingsPreview()
}

// settingsSetSqueezeTabs stages the one row's overflow: squeeze tabs that
// don't fit rather than scrolling the strip.
func (m *Model) settingsSetSqueezeTabs(on bool) {
	if m.settingsDraft.squeezeTabs == on {
		return
	}
	m.settingsDraft.squeezeTabs = on
	m.settingsPreview()
}

// settingsSetShowFileIcon stages the Show file icon switch.
func (m *Model) settingsSetShowFileIcon(on bool) {
	if m.settingsDraft.showFileIcon == on {
		return
	}
	m.settingsDraft.showFileIcon = on
	m.settingsPreview()
}

// settingsSetPinnedSeparateRow stages the pinned tabs row switch.
func (m *Model) settingsSetPinnedSeparateRow(on bool) {
	if m.settingsDraft.pinnedSeparateRow == on {
		return
	}
	m.settingsDraft.pinnedSeparateRow = on
	m.settingsPreview()
}

// settingsSetShowExtension stages the Show file extension switch.
func (m *Model) settingsSetShowExtension(on bool) {
	if m.settingsDraft.showExtension == on {
		return
	}
	m.settingsDraft.showExtension = on
	m.settingsPreview()
}

// settingsSetShowDirectoryForNonUnique stages the directory suffix switch.
func (m *Model) settingsSetShowDirectoryForNonUnique(on bool) {
	if m.settingsDraft.showDirectoryForNonUnique == on {
		return
	}
	m.settingsDraft.showDirectoryForNonUnique = on
	m.settingsPreview()
}

// settingsSetMarkModified stages the Mark modified switch.
func (m *Model) settingsSetMarkModified(on bool) {
	if m.settingsDraft.markModified == on {
		return
	}
	m.settingsDraft.markModified = on
	m.settingsPreview()
}

// settingsSetShowFullPathOnHover stages the full-path tooltip switch.
func (m *Model) settingsSetShowFullPathOnHover(on bool) {
	if m.settingsDraft.showFullPathOnHover == on {
		return
	}
	m.settingsDraft.showFullPathOnHover = on
	m.settingsPreview()
}

// settingsSetClosePosition stages the close button position select.
func (m *Model) settingsSetClosePosition(place string) {
	switch place {
	case "right", "left", "none":
	default:
		return
	}
	if m.settingsDraft.closePosition == place {
		return
	}
	m.settingsDraft.closePosition = place
	m.settingsPreview()
}

// settingsSetSortTabs stages the Sort tabs alphabetically switch and the
// strip reorders to show it.
func (m *Model) settingsSetSortTabs(on bool) {
	if m.settingsDraft.sortAlphabetically == on {
		return
	}
	m.settingsDraft.sortAlphabetically = on
	m.settingsPreview()
}

// settingsSetOpenAtEnd stages the Open new tabs at the end switch.
func (m *Model) settingsSetOpenAtEnd(on bool) {
	if m.settingsDraft.openAtEnd == on {
		return
	}
	m.settingsDraft.openAtEnd = on
	m.settingsPreview()
}

// settingsSetTabLimit stages the Tab limit field, clamped to its range. The
// previewed strip keeps the draft's limit, but the limit only closes tabs when
// a file opens, so staging can not drop one.
func (m *Model) settingsSetTabLimit(limit int) {
	limit = max(1, min(limit, maxTabLimit))
	if m.settingsDraft.tabLimit == limit {
		return
	}
	m.settingsDraft.tabLimit = limit
	m.settingsPreview()
}

// settingsSetActivateOnClose stages which tab opens when the current one
// closes, previewed by the next close.
func (m *Model) settingsSetActivateOnClose(policy string) {
	switch policy {
	case "left", "right", "mru":
	default:
		return
	}
	if m.settingsDraft.activateOnClose == policy {
		return
	}
	m.settingsDraft.activateOnClose = policy
	m.settingsPreview()
}

// settingsSetTabClosingPolicy stages unchanged vs least-recently-used.
func (m *Model) settingsSetTabClosingPolicy(policy string) {
	if policy != "unchanged" {
		policy = "lru"
	}
	if m.settingsDraft.tabClosingPolicy == policy {
		return
	}
	m.settingsDraft.tabClosingPolicy = policy
	m.settingsPreview()
}

// settingsSetNoPreviewTab stages Opening Policy's "Enable preview tab", which
// the strip's italic preview tab shows.
func (m *Model) settingsSetNoPreviewTab(noPreview bool) {
	if m.settingsDraft.noPreviewTab == noPreview {
		return
	}
	m.settingsDraft.noPreviewTab = noPreview
	m.settingsPreview()
}

// settingsSetTodoCase stages the TODO page's Case sensitive switch.
func (m *Model) settingsSetTodoCase(on bool) {
	if m.settingsDraft.todoCaseSensitive == on {
		return
	}
	m.settingsDraft.todoCaseSensitive = on
	m.invalidate()
}

// settingsSetTodoComments stages the TODO page's Only in comments switch.
func (m *Model) settingsSetTodoComments(on bool) {
	if m.settingsDraft.todoCommentsOnly == on {
		return
	}
	m.settingsDraft.todoCommentsOnly = on
	m.invalidate()
}

// settingsSetTodoEnabled stages one pattern row's checkbox.
func (m *Model) settingsSetTodoEnabled(index int, on bool) {
	if index < 0 || index >= len(m.settingsDraft.todoPatterns) {
		return
	}
	m.settingsDraft.todoPatterns[index].Enabled = on
	m.invalidate()
}

// settingsSetTodoKeyword stages a row's keyword, with the whitespace stripped
// as the page's input does.
func (m *Model) settingsSetTodoKeyword(index int, keyword string) {
	if index < 0 || index >= len(m.settingsDraft.todoPatterns) {
		return
	}
	cleaned := strings.Join(strings.Fields(keyword), "")
	if m.settingsDraft.todoPatterns[index].Keyword == cleaned {
		return
	}
	m.settingsDraft.todoPatterns[index].Keyword = cleaned
	m.invalidate()
}

// settingsSetTodoColor stages a row's colour tone.
func (m *Model) settingsSetTodoColor(index int, tone string) {
	if index < 0 || index >= len(m.settingsDraft.todoPatterns) {
		return
	}
	m.settingsDraft.todoPatterns[index].Color = tone
	m.invalidate()
}

// settingsAddTodoPattern appends NOTE in the success tone, as Add Pattern does.
func (m *Model) settingsAddTodoPattern() {
	m.settingsDraft.todoPatterns = append(m.settingsDraft.todoPatterns,
		TodoPattern{Keyword: "NOTE", Color: "success", Enabled: true})
	m.invalidate()
}

// settingsRemoveTodoPattern drops one row.
func (m *Model) settingsRemoveTodoPattern(index int) {
	if index < 0 || index >= len(m.settingsDraft.todoPatterns) {
		return
	}
	m.settingsDraft.todoPatterns = append(m.settingsDraft.todoPatterns[:index:index],
		m.settingsDraft.todoPatterns[index+1:]...)
	m.invalidate()
}

// settingsRestoreTodoPatterns puts the built-in keywords back.
func (m *Model) settingsRestoreTodoPatterns() {
	m.settingsDraft.todoPatterns = append([]TodoPattern(nil), defaultTodoPatterns...)
	m.settingsDraft.todoCaseSensitive = true
	m.settingsDraft.todoCommentsOnly = true
	m.invalidate()
}

// settingsPreview draws the staged theme, font, editor appearance,
// breadcrumbs and tab strip in this window.
func (m *Model) settingsPreview() {
	m.theme = themeForName(m.settingsDraft.theme)
	m.applyEditorFontPrefs(m.settingsDraft.font, m.settingsDraft.fontSize)
	m.applyAppearanceToTabs(m.settingsDraft.appearance())
	m.crumbs = m.settingsDraft.breadcrumbs()
	m.tabPrefs = m.settingsDraft.tabPrefs()
	m.sortTabsAlphabetically()
	m.invalidate()
}

// settingsPending reports staged settings that differ from the stored ones,
// which enables Apply.
func (m *Model) settingsPending() bool {
	for _, key := range []string{
		settingsPageAppearance, settingsPageEditorFont, settingsPageEditorAppearance,
		settingsPageBreadcrumbs, settingsPageCodeFolding, settingsPageGeneral,
		settingsPageEditorTabs, settingsPageInlayHints, settingsPageTodo,
		settingsPageSystem, settingsPageDateFormats,
		settingsPageNotifications, settingsPageAgents, settingsPageAgentPolicy,
		settingsPageJava, settingsPageLanguageServers, settingsPageIssueNavigation,
		settingsPageUpdates, settingsPageKeymap,
	} {
		if m.settingsDirtyPage(key) {
			return true
		}
	}
	return false
}

// settingsDirtyPage reports a page whose staged choice differs from the
// stored one; its title takes the accent and it offers Reset.
func (m *Model) settingsDirtyPage(key string) bool {
	switch key {
	case settingsPageAppearance:
		return m.settingsDraft.theme != m.prefs.Theme
	case settingsPageEditorFont:
		return m.settingsDraft.fontSize != m.prefs.fontSize() || m.settingsDraft.font != m.prefs.Font
	case settingsPageDateFormats:
		return !sameDateFormatPrefs(m.settingsDraft.dateFormat, m.prefs.dateFormatPrefs())
	case settingsPageSystem:
		return m.settingsDraft.reopenLast != (m.prefs.Startup == "reopen")
	case settingsPageIssueNavigation:
		return !sameIssueLinks(m.settingsDraft.issueLinks, m.prefs.VCS.IssueNavigation)
	case settingsPageEditorAppearance:
		a := m.settingsDraft.appearance()
		return m.settingsDraft.zenWidth != m.prefs.zenWidth() ||
			a.NoLineNumbers != m.prefs.Editor.NoLineNumbers ||
			a.LineNumberMode != m.prefs.Editor.LineNumberMode ||
			a.BlockCaret != m.prefs.Editor.BlockCaret ||
			a.ShowWhitespace != m.prefs.Editor.ShowWhitespace ||
			a.NoIndentGuides != m.prefs.Editor.NoIndentGuides
	case settingsPageCodeFolding:
		return !m.settingsDraft.showFolding != m.prefs.Editor.NoFolding
	case settingsPageBreadcrumbs:
		return !sameBreadcrumbPrefs(m.settingsDraft.breadcrumbs(), m.prefs.Breadcrumbs)
	case settingsPageGeneral:
		return m.settingsDraft.softWrap != m.prefs.Editor.SoftWrap ||
			m.settingsDraft.softWrapFiles != m.prefs.Editor.softWrapFiles() ||
			m.settingsDraft.scrollPastEnd != m.prefs.Editor.ScrollPastEnd ||
			m.settingsDraft.generalPrefs() != m.prefs.General ||
			m.settingsDraft.savePrefs() != m.prefs.SaveFiles
	case settingsPageEditorTabs:
		return m.settingsDraft.tabPrefs() != m.prefs.Tabs
	case settingsPageInlayHints:
		return m.settingsDraft.inlayPrefs() != m.prefs.Inlay
	case settingsPageNotifications:
		return m.settingsDraft.notifyBalloons != (m.prefs.Notify == nil || !m.prefs.Notify.NoBalloons) ||
			m.settingsDraft.notifySound != (m.prefs.Notify != nil && m.prefs.Notify.Sound) ||
			!sameNotifyGroups(m.settingsDraft.notifyGroups, m.prefs.notifyGroups())
	case settingsPageAgents:
		return !sameAgentPrefs(m.settingsDraft.agents.normalized(), m.agentPrefs())
	case settingsPageJava:
		return m.javaSettingsDirty()
	case settingsPageLanguageServers:
		return languageToolSettingsDirty(&m.languageToolSettings)
	case settingsPageAgentPolicy:
		return !sameAgentSecurity(m.settingsDraft.security.normalized(), m.agentSecurityPrefs())
	case settingsPageTodo:
		patterns, caseSensitive, commentsOnly := m.prefs.todoSettings()
		return !sameTodoPatterns(m.settingsDraft.todoPatterns, patterns) ||
			m.settingsDraft.todoCaseSensitive != caseSensitive ||
			m.settingsDraft.todoCommentsOnly != commentsOnly
	case settingsPageUpdates:
		return !sameUpdatePrefs(m.settingsDraft.updatePrefs(), m.prefs.Updates)
	case settingsPageKeymap:
		return !sameKeymaps(m.settingsDraft.keymap, m.prefs.Keymap)
	}
	return false
}

// settingsResetPage drops the current page's staged choices, as resetPage
// does.
func (m *Model) settingsResetPage() {
	switch m.settingsPage {
	case settingsPageAppearance:
		m.settingsDraft.theme = ""
	case settingsPageEditorFont:
		m.settingsDraft.fontSize = defaultFontSize
		m.settingsDraft.font = FontPrefs{}
	case settingsPageDateFormats:
		m.settingsDraft.dateFormat = defaultDateFormatPrefs()
	case settingsPageSystem:
		m.settingsDraft.reopenLast = false
	case settingsPageIssueNavigation:
		m.settingsDraft.issueLinks = nil
		m.settingsDraft.issueSelected = -1
		m.settingsDraft.issueEditing = -1
		m.settingsDraft.issueLinkDraft = IssueLinkPrefs{}
	case settingsPageEditorAppearance:
		m.settingsDraft.zenWidth = defaultZenWidth
		m.settingsDraft.lineNumbers = true
		m.settingsDraft.lineNumberMode = "absolute"
		m.settingsDraft.blockCaret = false
		m.settingsDraft.showWhitespace = false
		m.settingsDraft.indentGuides = true
	case settingsPageCodeFolding:
		m.settingsDraft.showFolding = true
	case settingsPageBreadcrumbs:
		m.settingsDraft.showBreadcrumbs = true
		m.settingsDraft.breadcrumbPlacement = "top"
		m.settingsDraft.breadcrumbDisabled = nil
	case settingsPageGeneral:
		m.settingsDraft.softWrap = false
		m.settingsDraft.softWrapFiles = defaultSoftWrapFiles
		m.settingsDraft.softWrapIndent = true
		m.settingsDraft.scrollPastEnd = false
		m.settingsDraft.wheelZoom = false
		m.settingsDraft.dragAndDrop = true
		m.settingsDraft.caretScroll = "minimize"
		m.settingsDraft.keepTrailingOnCaretLine = true
		m.settingsDraft.reformatOnSave = false
		m.settingsDraft.markdownImageFolder = "assets"
		m.settingsDraft.markdownImageNamePattern = "image-{timestamp}"
		m.settingsDraft.trimTrailing = false
		m.settingsDraft.trimAllLines = false
		m.settingsDraft.dropTrailingBlankLines = false
		m.settingsDraft.ensureFinalNewline = false
	case settingsPageLanguageServers:
		m.resetLanguageServerDrafts()
		return
	case settingsPageEditorTabs:
		m.settingsDraft.tabPlacement = "top"
		m.settingsDraft.pinnedSeparateRow = false
		m.settingsDraft.showExtension = true
		m.settingsDraft.showDirectoryForNonUnique = true
		m.settingsDraft.markModified = false
		m.settingsDraft.showFullPathOnHover = true
		m.settingsDraft.closePosition = "right"
		m.settingsDraft.sortAlphabetically = false
		m.settingsDraft.openAtEnd = false
		m.settingsDraft.tabLimit = defaultTabLimit
		m.settingsDraft.activateOnClose = "left"
		m.settingsDraft.tabClosingPolicy = "lru"
		m.settingsDraft.noPreviewTab = false
		m.settingsDraft.multipleRows = false
		m.settingsDraft.squeezeTabs = false
		m.settingsDraft.showFileIcon = true
	case settingsPageInlayHints:
		m.settingsDraft.codeVisionUsages = true
		m.settingsDraft.codeVisionAuthor = true
		m.settingsDraft.codeVisionNoUsages = true
		m.settingsDraft.codeVisionUsagesPosition = "top"
		m.settingsDraft.codeVisionAuthorPosition = "top"
	case settingsPageNotifications:
		m.settingsDraft.notifyBalloons = true
		m.settingsDraft.notifySound = false
		m.settingsDraft.notifyGroups = map[string]NotifyGroupPrefs{}
	case settingsPageAgents:
		m.settingsDraft.agents = (AgentPrefs{}).normalized()
	case settingsPageJava:
		m.resetJavaSettingsDraft()
	case settingsPageAgentPolicy:
		m.settingsDraft.security = (AgentSecurityPrefs{}).normalized()
		m.settingsDraft.securityAllowDraft = ""
		m.settingsDraft.securityBlockDraft = ""
	case settingsPageTodo:
		m.settingsDraft.todoPatterns = append([]TodoPattern(nil), defaultTodoPatterns...)
		m.settingsDraft.todoCaseSensitive = true
		m.settingsDraft.todoCommentsOnly = true
	case settingsPageUpdates:
		m.settingsDraft.updateAutoCheck = true
		m.settingsDraft.updateWhatsNew = true
		m.settingsDraft.updateSource = "auto"
		m.settingsDraft.updateLastChecked = 0
	case settingsPageKeymap:
		m.settingsDraft.keymap = nil
	default:
		return
	}
	m.settingsPreview()
}

// applySettings writes the staged choices to the preferences and the disk;
// OK closes afterwards, Apply keeps the dialog open.
func (m *Model) applySettings() {
	if !m.applyPreferenceSettings() {
		return
	}
	m.saveJavaSettingsThenLanguageServers(false)
}

func (m *Model) applySettingsAndClose() {
	if !m.applyPreferenceSettings() {
		return
	}
	m.saveJavaSettingsThenLanguageServers(true)
}

func (m *Model) applyPreferenceSettings() bool {
	m.prefs.Theme = m.settingsDraft.theme
	m.prefs.FontSize = m.settingsDraft.fontSize
	m.prefs.Font = m.settingsDraft.font
	m.prefs.DateFormat = m.settingsDraft.dateFormatPrefs()
	m.prefs.Startup = "welcome"
	if m.settingsDraft.reopenLast {
		m.prefs.Startup = "reopen"
	}
	m.prefs.VCS.IssueNavigation = cloneIssueLinks(m.settingsDraft.issueLinks)
	m.prefs.ZenWidth = m.settingsDraft.zenWidth
	m.prefs.Editor = m.settingsDraft.appearance()
	m.prefs.General = m.settingsDraft.generalPrefs()
	m.prefs.Inlay = m.settingsDraft.inlayPrefs()
	m.prefs.Breadcrumbs = m.settingsDraft.breadcrumbs()
	m.prefs.Tabs = m.settingsDraft.tabPrefs()
	m.prefs.SaveFiles = m.settingsDraft.savePrefs()
	m.prefs.Updates = m.settingsDraft.updatePrefs()
	m.prefs.Keymap = cloneKeymap(m.settingsDraft.keymap)
	m.prefs.Todo = m.settingsDraft.todoPrefs()
	m.prefs.Notify = m.settingsDraft.notifyPrefs()
	agents := m.settingsDraft.agents.normalized()
	m.prefs.Agents = &agents
	security := m.settingsDraft.security.normalized()
	m.prefs.AgentSecurity = &security
	m.settingsErr = ""
	if err := m.prefs.Save(m.settingsWritePath()); err != nil {
		m.settingsErr = err.Error()
	}
	m.saveAgentSettingsBackend()
	m.applyPreferences()
	m.applyTodoSettings()
	m.settingsDraft = m.settingsDraftFromPrefs()
	m.invalidate()
	return m.settingsErr == ""
}

// settingsWritePath is the preferences file, or the one place a smoke run
// points at.
func (m *Model) settingsWritePath() string { return m.cfg.SettingsPath }

// --- Drawing ---------------------------------------------------------------

// settingsDialog draws Settings while it is open; Escape and a click outside
// dismiss it as Cancel.
func (m *Model) settingsDialog(c *ui.Context, p *ui.Theme) {
	if !m.settingsOpen {
		return
	}
	open := m.settingsOpen
	dlg := ui.Modal(c, &open, func() {
		m.settingsSheet(c, p)
	})
	if dlg.Dismissed() || !open {
		m.closeSettings()
		return
	}
	m.settingsKeymapRecorderDialog(c, p)
}

// settingsSheet is the dialog: the title, the tree beside the page, and the
// footer's status and buttons.
func (m *Model) settingsSheet(c *ui.Context, p *ui.Theme) {
	oi := oiPaletteOf(p)
	ui.Column(c).Width(settingsDialogWidth).Height(m.settingsDialogHeight()).
		Background(oi.elev).Radius(6).Children(func() {
		m.settingsTitle(c, oi)
		// Stretch, not the Row default of centering: a short page would
		// otherwise float in the middle instead of filling its side.
		ui.Row(c).Grow(1).MinHeight(0).MinWidth(0).AlignItems(ui.Stretch).Children(func() {
			m.settingsSidebar(c, p, oi)
			m.settingsSplitHandle(c, oi)
			m.settingsPageHost(c, p, oi)
		})
		m.settingsFooter(c, p, oi)
	})
}

// settingsDialogHeight is min(720px, the window less 48), with a floor so
// the page still has room on a small window.
func (m *Model) settingsDialogHeight() float32 {
	h := m.editorHeight - 48
	if h <= 0 || h > settingsDialogMaxHeight {
		h = settingsDialogMaxHeight
	}
	if h < settingsDialogMinHeight {
		h = settingsDialogMinHeight
	}
	return h
}

// settingsSideWidth is the page tree's width, 160..40% of the sheet.
func (m *Model) settingsSideWidth() float32 {
	w := m.settingsSide
	if w <= 0 {
		w = settingsSideDefault
	}
	w = max(w, settingsSideMin)
	return min(w, settingsDialogWidth*0.4)
}

// settingsTitle is the dialog's title row.
func (m *Model) settingsTitle(c *ui.Context, oi oiColors) {
	ui.Row(c).Height(settingsTitleHeight).Shrink(0).Padding(0, 16).
		AlignItems(ui.Center).Children(func() {
		ui.Text(c, "Settings").FontSize(13).Bold().TextColor(oi.textStrong).SingleLine()
	})
}

// settingsSidebar is the search field and the page tree.
func (m *Model) settingsSidebar(c *ui.Context, p *ui.Theme, oi oiColors) {
	ui.Column(c).Width(m.settingsSideWidth()).Shrink(0).MinHeight(0).Children(func() {
		ui.Row(c).Padding(7, 5, 6, 5).Shrink(0).Children(func() {
			in := ui.TextInput(c, &m.settingsQuery).Grow(1).Placeholder("Search settings").AutoFocus()
			in.OnChange(func() { m.settingsQueryChanged() })
			in.OnShortcut(0, ui.KeyDown, func() { m.settingsStepSelection(1) })
			in.OnShortcut(0, ui.KeyUp, func() { m.settingsStepSelection(-1) })
		})
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).Children(func() {
				rows := m.settingsVisibleRows()
				if len(rows) == 0 {
					ui.Row(c).Padding(8, 12).Children(func() {
						ui.Text(c, "No matches").FontSize(12).TextColor(oi.textSubtle).SingleLine()
					})
					return
				}
				for i, row := range rows {
					m.settingsTreeRow(c, oi, i, row)
				}
			})
		})
	})
}

// settingsTreeRow is one 24 DIP row: the caret of a node with children, then
// its title, bold at the top level and accent-colored while the page is
// staged.
func (m *Model) settingsTreeRow(c *ui.Context, oi oiColors, i int, row settingsRow) {
	selected := row.node.key == m.settingsPage
	r := ui.Row(c.Key(fmt.Sprintf("settings:node:%d", i))).Height(settingsTreeRow).
		PaddingStart(6 + float32(row.depth)*16).PaddingEnd(10).Gap(4).AlignItems(ui.Center)
	switch {
	case selected:
		r.Background(oi.selected)
	case r.Hovered():
		r.Background(oi.hover)
	}
	r.Children(func() {
		if len(row.node.children) == 0 {
			ui.Row(c).Width(14)
		} else {
			caret := ui.Row(c).Width(14).Height(settingsTreeRow).AlignItems(ui.Center).Justify(ui.Center)
			caret.Children(func() {
				glyph := "▾"
				if m.settingsCollapsed[row.node.key] && m.settingsQuery == "" {
					glyph = "▸"
				}
				ui.Text(c, glyph).FontSize(9).TextColor(oi.textMuted).SingleLine()
			})
			if caret.Clicked() {
				m.settingsToggleNode(row.node.key)
			}
		}
		color := oi.text
		if selected {
			color = oi.textStrong
		}
		if row.node.page != nil && m.settingsDirtyPage(row.node.key) {
			color = oi.accent
		}
		label := ui.Text(c, row.node.title).FontSize(12).TextColor(color).
			SingleLine().Ellipsis("…").Grow(1)
		if row.depth == 0 {
			label.Bold()
		}
	})
	if r.Clicked() {
		m.settingsSelectPage(row.node.key)
	}
}

// settingsSplitHandle is the hairline between the tree and the page; dragging
// it resizes the tree between 160 DIPs and 40% of the sheet.
func (m *Model) settingsSplitHandle(c *ui.Context, oi oiColors) {
	h := ui.Row(c).Key("settings:split").Width(1).Shrink(0).Background(oi.borderStrong).
		Cursor(ui.CursorResizeEW)
	dx, _, held := h.Dragged()
	if !held || dx == 0 {
		return
	}
	w := max(min(m.settingsSideWidth()+dx, settingsDialogWidth*0.4), settingsSideMin)
	if w != m.settingsSide {
		m.settingsSide = w
		m.invalidate()
	}
}

// settingsPageHost is the breadcrumb header and the selected page's body.
func (m *Model) settingsPageHost(c *ui.Context, p *ui.Theme, oi oiColors) {
	current, ok := m.settingsCurrent()
	ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
		m.settingsPageHeader(c, p, oi, current, ok)
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).Padding(4, 20, 20, 20).MinWidth(0).Children(func() {
				if !ok {
					ui.Text(c, "Nothing found").FontSize(12).TextColor(oi.textSubtle).SingleLine()
					return
				}
				m.settingsPageBody(c, p, oi, current)
			})
		})
	})
}

// settingsPageHeader is the breadcrumb, Reset while the page is staged, and
// the Back / Forward buttons.
func (m *Model) settingsPageHeader(c *ui.Context, p *ui.Theme, oi oiColors, current settingsNode, ok bool) {
	ui.Row(c).Height(settingsHeaderHeight).Shrink(0).Padding(0, 12, 0, 20).
		Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).Gap(4).AlignItems(ui.Center).Children(func() {
			if !ok {
				return
			}
			for i, crumb := range settingsBreadcrumb(settingsTree(), current.key) {
				if i > 0 {
					ui.Text(c, "›").FontSize(11).TextColor(oi.textSubtle).SingleLine()
				}
				ui.Text(c, crumb).FontSize(12).Bold().TextColor(oi.textStrong).
					SingleLine().Ellipsis("…")
			}
		})
		if ok && m.settingsDirtyPage(current.key) {
			ui.Button(c, "Reset").OnClick(func() { m.settingsResetPage() })
		}
		back := ui.Button(c, "‹").Width(26).Disabled(m.settingsHistoryAt <= 0)
		back.OnClick(func() { m.settingsStepHistory(-1) })
		fwd := ui.Button(c, "›").Width(26).Disabled(m.settingsHistoryAt >= len(m.settingsHistory)-1)
		fwd.OnClick(func() { m.settingsStepHistory(1) })
	})
}

// settingsFooter is the save status and Cancel / Apply / OK, in macOS order.
func (m *Model) settingsFooter(c *ui.Context, p *ui.Theme, oi oiColors) {
	ui.Row(c).Height(settingsFooterHeight).Shrink(0).Padding(10, 16).
		Gap(10).AlignItems(ui.Center).Children(func() {
		if m.settingsErr != "" {
			ui.Text(c, "Could not save: "+m.settingsErr).FontSize(12).
				TextColor(oi.danger).SingleLine().Ellipsis("…").Grow(1)
		} else if m.javaSettings.saving {
			ui.Text(c, "Saving Java settings…").FontSize(12).
				TextColor(oi.textSubtle).SingleLine().Grow(1)
		} else if m.languageToolSettings.saving {
			ui.Text(c, "Saving language servers…").FontSize(12).
				TextColor(oi.textSubtle).SingleLine().Grow(1)
		} else {
			ui.Spacer(c)
		}
		saving := m.javaSettings.saving || m.languageToolSettings.saving
		ui.Button(c, "Cancel").Disabled(saving).OnClick(m.closeSettings)
		apply := ui.Button(c, "Apply").Disabled(saving || !m.settingsPending())
		apply.OnClick(m.applySettings)
		ui.Button(c, "OK").Disabled(saving).OnClick(m.applySettingsAndClose)
	})
}
