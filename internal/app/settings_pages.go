package app

// The pages the settings dialog shows. Each one draws only the options this
// build really implements, with OpenIDEA's labels and groups: the theme and
// its Sync with OS switch, the editor font size, the distraction free text
// width, the keymap this build carries, and the Java toolchain.

import (
	"fmt"
	"math"
	"os"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

// settingsPageBody draws the selected node: a group folder's description and
// links, or the page's section.
func (m *Model) settingsPageBody(c *ui.Context, p *ui.Theme, oi oiColors, node settingsNode) {
	if node.page == nil {
		m.settingsGroupPage(c, oi, node)
		return
	}
	switch node.page.section {
	case "appearance":
		m.settingsAppearanceBody(c, oi)
	case "editorAppearance":
		m.settingsEditorAppearanceBody(c, oi)
	case "general":
		m.settingsGeneralBody(c, oi)
	case "nativeSettings":
		m.settingsNativeBody(c, oi, node.page.id)
	case "pythonOverview":
		m.settingsNativeBody(c, oi, node.page.id)
	case "gutterIcons":
		m.nativeGutterBody(c, oi)
	case "breadcrumbs":
		m.settingsBreadcrumbsBody(c, oi)
	case "editorTabs":
		m.settingsEditorTabsBody(c, oi)
	case "codeFolding":
		m.settingsCodeFoldingBody(c, oi)
	case "vim":
		m.settingsVimBody(c, oi)
	case "liveTemplates":
		m.settingsLiveTemplatesBody(c, oi)
	case "postfixCompletion":
		m.settingsPostfixBody(c, oi)
	case "fileTemplates":
		m.settingsFileTemplatesBody(c, oi)
	case "inlayHints":
		m.settingsInlayHintsBody(c, oi)
	case "editorFont":
		m.settingsFontBody(c, oi)
	case "fileTypes":
		m.settingsFileTypesBody(c, oi)
	case "inspections":
		m.settingsInspectionsBody(c, oi)
	case "naturalLanguages":
		m.settingsNaturalLanguagesBody(c, oi)
	case "spelling":
		m.settingsSpellingBody(c, oi)
	case "dateFormats":
		m.settingsDateFormatsBody(c, oi)
	case "system":
		m.settingsSystemBody(c, oi)
	case "keymap":
		m.settingsKeymapBody(c, oi)
	case "todo":
		m.settingsTodoBody(c, oi)
	case "notifications":
		m.settingsNotificationsBody(c, oi)
	case "actionsOnSave":
		m.settingsNativeBody(c, oi, node.page.id)
	case "agents":
		m.settingsAgentsBody(c, oi)
	case "agentPolicy":
		m.settingsAgentPolicyBody(c, oi)
	case "agentMCP":
		m.settingsAgentMCPBody(c, oi)
	case "agentRegistry":
		m.settingsAgentRegistryBody(c, oi)
	case "java":
		m.settingsJavaBody(c, p, oi)
	case "prettier", "oxfmt", "ruff":
		m.settingsFormatterBody(c, oi, node.page.id)
	case "languageServers":
		m.settingsLanguageServersBody(c, p, oi)
	case "issueNavigation":
		m.settingsIssueNavigationBody(c, oi)
	case "vcsOverview":
		m.settingsVCSOverviewBody(c, oi)
	case "vcsMappings":
		m.settingsVCSMappingsBody(c, oi)
	case "vcsLog":
		m.settingsVCSLogBody(c, oi)
	case "vcsGitHub":
		m.settingsVCSGitHubBody(c, oi)
	case "updates":
		m.settingsUpdatesBody(c, oi)
	case "projectTrust":
		m.settingsProjectTrustBody(c, oi)
	case "externalTools":
		m.settingsExternalToolsBody(c, oi)
	case "runTargets":
		m.settingsRunTargetsBody(c, oi)
	case "deployment":
		m.settingsDeploymentBody(c, oi)
	case "ssh":
		m.settingsSSHBody(c, oi)
	}
}

// settingsSystemBody is Settings › Appearance & Behavior › System Settings.
func (m *Model) settingsSystemBody(c *ui.Context, oi oiColors) {
	settingsSectionTitle(c, oi, "Project")
	reopen := m.settingsDraft.reopenLast
	if ui.Checkbox(c, &reopen, "Reopen projects on startup").Changed() {
		m.settingsDraft.reopenLast = reopen
		m.invalidate()
	}
	ui.Row(c).MinHeight(32).Gap(12).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Open project in", 220)
		ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() {
			for _, choice := range []struct {
				id    string
				label string
			}{
				{"new", "New window"},
				{"same", "Current window"},
				{"ask", "Ask"},
			} {
				if ui.Radio(c, &m.settingsDraft.openProjectIn, choice.id, choice.label).Changed() {
					m.invalidate()
				}
			}
		})
	})
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Default directory", 220)
		input := ui.TextInput(c, &m.settingsDraft.defaultProjectDirectory).Grow(1).MaxWidth(420)
		if input.Changed() {
			m.invalidate()
		}
		ui.Button(c, "Browse…").OnClick(func() { m.chooseDefaultProjectDirectory() })
	})
	ui.Text(c, `This directory is preselected in "Open…" and "New Window" dialogs.`).
		FontSize(11).TextColor(oi.textMuted).SingleLine().Ellipsis("…")

	settingsSectionTitle(c, oi, "Caches")
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Current location", 220)
		path := m.goideaDataDir()
		ui.Text(c, path).FontSize(12).Font("monospace").TextColor(oi.text).
			SingleLine().Ellipsis("…").Grow(1)
		ui.Button(c, "Show in Finder").OnClick(func() {
			if path != "" {
				mygo.Shell.ShowItemInFolder(path)
			}
		})
	})
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Local data directory", 220)
		in := ui.TextInput(c, &m.settingsDraft.localDataDir).Grow(1).MaxWidth(420).
			Disabled(m.localDataDirLocked())
		if in.Changed() {
			m.invalidate()
		}
		ui.Button(c, "Browse…").Disabled(m.localDataDirLocked()).
			OnClick(m.chooseLocalDataDir)
		ui.Button(c, "Use default").Disabled(m.localDataDirLocked()).OnClick(func() {
			m.settingsDraft.localDataDir = ""
			m.invalidate()
		})
	})
	move := m.settingsDraft.localDataMove
	if ui.Checkbox(c, &move, "Copy existing data to the new location").
		Disabled(m.localDataDirLocked()).Changed() {
		m.settingsDraft.localDataMove = move
		m.invalidate()
	}
	ui.Text(c, "Tool downloads, language-server data, logs and settings are kept here; Apply restarts GoIDEA.").
		FontSize(11).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
	if os.Getenv("GOIDEA_DATA_DIR") != "" {
		ui.Text(c, "GOIDEA_DATA_DIR is set and takes precedence.").
			FontSize(11).TextColor(oi.warning).SingleLine()
	} else if m.cfg.LocalDataDirConfigPath == "" {
		ui.Text(c, "This launch cannot persist a local data directory change.").
			FontSize(11).TextColor(oi.warning).SingleLine()
	}

	settingsSectionTitle(c, oi, "Autosave")
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		saveOnIdle := m.settingsDraft.saveOnIdle
		if ui.Checkbox(c, &saveOnIdle, "Save files if the IDE is idle for").Changed() {
			m.settingsDraft.saveOnIdle = saveOnIdle
			m.invalidate()
		}
		seconds := float64(m.settingsDraft.idleSeconds)
		field := ui.NumberInput(c, &seconds, 1, 3600, 1).Width(92)
		field.Disabled(!m.settingsDraft.saveOnIdle)
		if field.Changed() {
			m.settingsDraft.idleSeconds = int(seconds)
			m.invalidate()
		}
		ui.Text(c, "seconds").FontSize(12).TextColor(oi.textMuted)
	})
	autoSave := m.settingsDraft.autoSave
	if ui.Checkbox(c, &autoSave,
		"Save files when switching to a different application or a built-in terminal").Changed() {
		m.settingsDraft.autoSave = autoSave
		m.invalidate()
	}
	syncOnFocus := m.settingsDraft.syncOnFocus
	if ui.Checkbox(c, &syncOnFocus,
		"Synchronize external changes when switching to the IDE window").Changed() {
		m.settingsDraft.syncOnFocus = syncOnFocus
		m.invalidate()
	}

	settingsSectionTitle(c, oi, "Local History")
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Keep local history for", 220)
		days := float64(m.settingsDraft.localHistoryDays)
		field := ui.NumberInput(c, &days, 1, 365, 1).Width(92)
		if field.Changed() {
			m.settingsDraft.localHistoryDays = int(days)
			m.invalidate()
		}
		ui.Text(c, "working days").FontSize(12).TextColor(oi.textMuted)
	})
	ui.Text(c, "Days on which files changed; older revisions are removed when a project opens.").
		FontSize(11).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Maximum local history size per project", 220)
		size := float64(m.settingsDraft.localHistoryMaxMB)
		field := ui.NumberInput(c, &size, 16, 16384, 64).Width(100)
		if field.Changed() {
			m.settingsDraft.localHistoryMaxMB = int(size)
			m.invalidate()
		}
		ui.Text(c, "MB").FontSize(12).TextColor(oi.textMuted)
	})

	settingsSectionTitle(c, oi, "Exit")
	confirmExit := m.settingsDraft.confirmExit
	if ui.Checkbox(c, &confirmExit, "Confirm before exiting the IDE").Changed() {
		m.settingsDraft.confirmExit = confirmExit
		m.invalidate()
	}
	closeWithoutWelcome := m.settingsDraft.closeWithoutWelcome
	if ui.Checkbox(c, &closeWithoutWelcome,
		"Close the window instead of showing the Welcome screen").Changed() {
		m.settingsDraft.closeWithoutWelcome = closeWithoutWelcome
		m.invalidate()
	}
	ui.Text(c, "When this is the only window, closing a project or file closes the window. On macOS, the app stays in the Dock.").
		FontSize(11).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
	closeTabsWithWindow := m.settingsDraft.closeTabsWithWindow
	if ui.Checkbox(c, &closeTabsWithWindow,
		"Close all editor tabs when the window closes").Changed() {
		m.settingsDraft.closeTabsWithWindow = closeTabsWithWindow
		m.invalidate()
	}
	ui.Text(c, "Files left open are not restored the next time the project opens.").
		FontSize(11).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
}

// settingsDateFormatsBody is Settings › System Settings › Date Formats.
func (m *Model) settingsDateFormatsBody(c *ui.Context, oi oiColors) {
	f := &m.settingsDraft.dateFormat
	if ui.Checkbox(c, &f.OverrideSystem, "Override system date and time format").Changed() {
		m.invalidate()
	}
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Date format", 220)
		input := ui.TextInput(c, &f.Pattern).Grow(1).MaxWidth(280)
		if !f.OverrideSystem {
			input.Disabled(true)
		}
		if input.Changed() && strings.TrimSpace(f.Pattern) != "" {
			f.Pattern = strings.TrimSpace(f.Pattern)
			m.invalidate()
		}
	})
	ui.Text(c, "Placeholders: yyyy, yy, MMMM, MMM, MM, M, dd, d").
		FontSize(11).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
	use24 := f.Use24Hour
	use24Element := ui.Checkbox(c, &use24, "Use 24-hour time format")
	if !f.OverrideSystem {
		use24Element.Disabled(true)
	}
	if use24Element.Changed() {
		f.Use24Hour = use24
		m.invalidate()
	}
	pretty := f.Pretty
	if ui.Checkbox(c, &pretty, "Use pretty formatting").Changed() {
		f.Pretty = pretty
		m.invalidate()
	}
	ui.Text(c, `Show "Today", "Yesterday" and "N min ago" for recent dates`).
		FontSize(11).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
	now := time.Now()
	region := c.Region()
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Preview", 220)
		ui.Text(c, formatAppTimestamp(*f, region, float64(now.Add(-2*time.Hour).UnixMilli()), now)+
			" · "+formatAppTimestamp(*f, region, float64(now.Add(-3*24*time.Hour).UnixMilli()), now)).
			FontSize(12).TextColor(oi.textStrong).SingleLine().Ellipsis("…")
	})
}

// settingsAgentsBody is Settings › Agents › Chat. It mirrors tools.tsx's
// sections and the keys the Composer, timeline and task header consume.
func (m *Model) settingsAgentsBody(c *ui.Context, oi oiColors) {
	a := &m.settingsDraft.agents

	settingsSectionTitle(c, oi, "New Tasks")
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "New tasks start", 220)
		choices := []string{
			"Without asking for approval",
			"Asking for approval",
		}
		shown := choices[0]
		if a.newTaskApprovals() == "ask" {
			shown = choices[1]
		}
		if ui.Select(c, &shown, choices).Width(280).Changed() {
			if shown == choices[1] {
				a.NewTaskApprovals = "ask"
			} else {
				a.NewTaskApprovals = "bypass"
			}
			m.invalidate()
		}
	})
	ui.Text(c, "Without asking means the agent's bypass or full-access mode, where command permissions don't apply").
		FontSize(11).TextColor(oi.textMuted).SingleLine().Ellipsis("…")

	settingsSectionTitle(c, oi, "Sending")
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Send messages with", 220)
		labels := []string{"Enter", "⌘Enter"}
		shown := "Enter"
		if a.sendWith() == "modEnter" {
			shown = "⌘Enter"
		}
		if ui.Select(c, &shown, labels).Width(160).Changed() {
			if shown == "⌘Enter" {
				a.SendWith = "modEnter"
			} else {
				a.SendWith = "enter"
			}
			m.invalidate()
		}
	})
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "While the agent works, sending", 220)
		labels := []string{
			"Queues the message for after the turn",
			"Sends it into the turn",
		}
		shown := labels[0]
		if a.whileRunning() == "steer" {
			shown = labels[1]
		}
		if ui.Select(c, &shown, labels).Width(280).Changed() {
			if shown == labels[1] {
				a.WhileRunning = "steer"
			} else {
				a.WhileRunning = "queue"
			}
			m.invalidate()
		}
	})
	ui.Text(c, "The other way is a modifier away: ⌘↵ with Enter, ⌘⇧↵ with ⌘Enter").
		FontSize(11).TextColor(oi.textMuted).SingleLine().Ellipsis("…")

	settingsSectionTitle(c, oi, "Conversation")
	fold := !a.NoFoldTurns
	if ui.Checkbox(c, &fold, "Fold a finished turn's work").Changed() {
		a.NoFoldTurns = !fold
		m.invalidate()
	}
	ui.Text(c, "Reasoning and tool calls before the reply fold into one line under its message").
		FontSize(11).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Tool calls", 220)
		labels := []string{"Grouped by what they did", "One per line"}
		shown := labels[0]
		if a.toolCalls() == "each" {
			shown = labels[1]
		}
		if ui.Select(c, &shown, labels).Width(250).Changed() {
			if shown == labels[1] {
				a.ToolCalls = "each"
			} else {
				a.ToolCalls = "grouped"
			}
			m.invalidate()
		}
	})
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Reasoning", 220)
		labels := []string{"Open while it streams", "Always open", "Folded"}
		values := []string{"auto", "expanded", "collapsed"}
		shown := labels[0]
		for i, value := range values {
			if a.thinking() == value {
				shown = labels[i]
				break
			}
		}
		if ui.Select(c, &shown, labels).Width(220).Changed() {
			for i, label := range labels {
				if shown == label {
					a.Thinking = values[i]
					break
				}
			}
			m.invalidate()
		}
	})
	if ui.Checkbox(c, &a.ExpandToolOutput, "Show tool output without opening it").Changed() {
		m.invalidate()
	}
	smooth := !a.NoSmoothText
	if ui.Checkbox(c, &smooth, "Reveal streaming text smoothly").Changed() {
		a.NoSmoothText = !smooth
		m.invalidate()
	}
	ui.Text(c, "Text appears at a steady pace instead of in bursts.").
		FontSize(11).TextColor(oi.textMuted).SingleLine().Ellipsis("…")

	settingsSectionTitle(c, oi, "Shown in the Task Header")
	context := !a.NoContextUsage
	if ui.Checkbox(c, &context, "Context window usage").Changed() {
		a.NoContextUsage = !context
		m.invalidate()
	}
	work := !a.NoWorkTime
	if ui.Checkbox(c, &work, "Time spent on the task").Changed() {
		a.NoWorkTime = !work
		m.invalidate()
	}
	limits := !a.NoUsageLimits
	if ui.Checkbox(c, &limits, "Usage limits").Changed() {
		a.NoUsageLimits = !limits
		m.invalidate()
	}
	ui.Text(c, "The account's 5-hour and weekly limits, when the agent reports them (Claude Code).").
		FontSize(11).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
	if ui.Checkbox(c, &a.ShowCost, "Session cost").Changed() {
		m.invalidate()
	}
	ui.Text(c, "What the session has cost so far, as the agent reports it.").
		FontSize(11).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
}

// settingsUpdatesBody is Settings › Updates, as upstream's updates.tsx: the
// version and the last check with Check Now, then the two switches the update
// flow keeps, then where the build updates from.
func (m *Model) settingsUpdatesBody(c *ui.Context, oi oiColors) {
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Current version", 220)
		ui.Text(c, mygo.App.Version()).FontSize(12).TextColor(oi.text).SingleLine()
	})
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Last checked", 220)
		when := "Never"
		if t := m.lastUpdateCheck(); !t.IsZero() {
			when = t.Format("Jan 2, 2006, 15:04")
		}
		ui.Text(c, when).FontSize(12).TextColor(oi.textMuted).SingleLine()
		ui.Spacer(c)
		label := "Check Now"
		if m.update.checking {
			label = "Checking for updates…"
		}
		ui.Button(c, label).Disabled(m.update.checking).
			OnClick(func() { m.CheckForUpdates() })
	})
	ui.Row(c).Height(10)
	auto := m.settingsDraft.updateAutoCheck
	if ui.Checkbox(c, &auto, "Check for updates automatically").Changed() {
		m.settingsDraft.updateAutoCheck = auto
		m.invalidate()
	}
	ui.Text(c, "At startup and every 4 hours while GoIDEA runs. Each new version is announced once.").
		FontSize(12).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
	ui.Row(c).Height(10)
	whats := m.settingsDraft.updateWhatsNew
	if ui.Checkbox(c, &whats, "Show What's New after an update").Changed() {
		m.settingsDraft.updateWhatsNew = whats
		m.invalidate()
	}
	ui.Text(c, "Opens a page with the release notes when a new version starts. Help › What's New opens it any time.").
		FontSize(12).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
	ui.Row(c).Height(10)
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Update source", 220)
		shown := updateSourceLabel(m.settingsDraft.updateSource)
		sel := ui.Select(c, &shown, []string{"Automatic", "GitHub", "CNB (China mirror)"}).Width(220)
		if sel.Changed() {
			m.settingsDraft.updateSource = updateSourceValue(shown)
			m.invalidate()
		}
	})
	ui.Text(c, "Automatic uses GitHub in this build; the CNB mirror is used when its GoIDEA feed is configured.").
		FontSize(12).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
}

func updateSourceLabel(source string) string {
	switch source {
	case "github":
		return "GitHub"
	case "cnb":
		return "CNB (China mirror)"
	default:
		return "Automatic"
	}
}

func updateSourceValue(label string) string {
	switch label {
	case "GitHub":
		return "github"
	case "CNB (China mirror)":
		return "cnb"
	default:
		return "auto"
	}
}

// settingsGroupPage is a group folder: the description above the links to the
// pages in it, indented as GROUP_DESCRIPTIONS and groupLinks are.
func (m *Model) settingsGroupPage(c *ui.Context, oi oiColors, node settingsNode) {
	if description := settingsGroupDescriptions[node.title]; description != "" {
		ui.Row(c).MaxWidth(720).Padding(0, 0, 14, 0).Children(func() {
			ui.Text(c, description).FontSize(13).TextColor(oi.text)
		})
	}
	ui.Column(c).Gap(2).PaddingStart(32).Children(func() {
		for _, child := range node.children {
			key := child.key
			link := ui.Row(c.Key("settings:link:" + key)).Height(20).AlignItems(ui.Center)
			link.Children(func() {
				ui.Text(c, child.title).FontSize(12).TextColor(oi.link).SingleLine()
			})
			if link.Clicked() {
				m.settingsSelectPage(key)
			}
		}
	})
}

// settingsFormLabel is a settings row's label, in the column the rows share.
func settingsFormLabel(c *ui.Context, oi oiColors, label string, width float32) {
	ui.Text(c, label).FontSize(12).TextColor(oi.text).Width(width).
		SingleLine().Ellipsis("…")
}

// settingsFormLabelDim is a label whose control this state disables, as
// OpenIDEA dims Show tabs in when Tab placement is left, right or none.
func settingsFormLabelDim(c *ui.Context, oi oiColors, label string, width float32) {
	ui.Text(c, label).FontSize(12).TextColor(oi.textMuted).Width(width).
		SingleLine().Ellipsis("…")
}

// settingsAppearanceBody is Appearance's theme, accessibility, UI and tool
// window choices. It stages the same switches View › Appearance changes.
func (m *Model) settingsAppearanceBody(c *ui.Context, oi oiColors) {
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Theme:", 220)
		shown := "Light"
		if m.settingsDraft.theme == "dark" {
			shown = "Dark"
		}
		sync := m.settingsDraft.theme == ""
		sel := ui.Select(c, &shown, []string{"Light", "Dark"}).Disabled(sync)
		if sel.Changed() {
			m.settingsSetTheme(strings.ToLower(shown))
		}
		switchOn := sync
		chk := ui.Checkbox(c, &switchOn, "Sync with OS")
		if chk.Changed() {
			if switchOn {
				m.settingsSetTheme("")
			} else {
				m.settingsSetTheme("light")
			}
		}
		gear := ui.ButtonBase(c.Key("settings:appearance:preferred-themes")).
			Size(24, 24).Radius(6).AlignItems(ui.Center).Justify(ui.Center).
			Cursor(ui.CursorPointer).
			Tooltip("Preferred themes for light and dark OS mode").
			Disabled(!sync)
		if gear.Hovered() && sync {
			gear.Background(oi.hoverTint)
		}
		gear.Children(func() {
			oiIconEl(c, "general/settings", 16, oi, oi.icon)
		})
		if !sync {
			m.settingsThemePrefs = false
			return
		}
		ui.PopoverBase(c, gear, &m.settingsThemePrefs, func(panel ui.Element) {
			panel.Width(470).Padding(10).Gap(6).Background(oi.popup).
				BorderWidth(1).BorderColor(oi.borderStrong).Radius(8).
				Shadow(0, 10, 28, 0, ui.RGBA(0, 0, 0, 0.3))
			panel.Children(func() {
				m.settingsPreferredThemeRow(
					c, oi, false, "Preferred theme for light OS mode",
				)
				m.settingsPreferredThemeRow(
					c, oi, true, "Preferred theme for dark OS mode",
				)
			})
		})
	})
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Accent color:", 220)
		label := accentLabelOf(m.settingsDraft.accent)
		labels := make([]string, 0, len(accentChoices))
		for _, choice := range accentChoices {
			labels = append(labels, choice.label)
		}
		if ui.Select(c, &label, labels).Changed() {
			m.settingsDraft.accent = accentIDOf(label)
			m.settingsPreview()
		}
	})
	ui.Row(c).Height(8)

	settingsSectionTitle(c, oi, "Accessibility")
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Zoom:", 220)
		labels := make([]string, 0, len(ideZoomSteps))
		for _, zoom := range ideZoomSteps {
			labels = append(labels, fmt.Sprintf("%d%%", zoom))
		}
		shown := fmt.Sprintf("%d%%", m.settingsDraft.ideZoom)
		if ui.Select(c, &shown, labels).Changed() {
			var zoom int
			if _, err := fmt.Sscanf(shown, "%d%%", &zoom); err == nil {
				m.settingsDraft.ideZoom = zoom
				m.invalidate()
			}
		}
	})
	customFont := m.settingsDraft.useCustomFont
	if ui.Checkbox(c, &customFont, "Use custom font").Changed() {
		m.settingsDraft.useCustomFont = customFont
		m.settingsPreview()
	}
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabelDim(c, oi, "UI font:", 220)
		shown := uiFontFamilyOr(m.settingsDraft.uiFontFamily)
		choices := settingsUIFontChoices(shown)
		if ui.Select(c, &shown, choices).Grow(1).Disabled(!customFont).Changed() {
			m.settingsDraft.uiFontFamily = shown
			m.settingsPreview()
		}
	})
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabelDim(c, oi, "Fallback font:", 220)
		shown := m.settingsDraft.uiFontFallback
		if shown == "" {
			shown = "(None)"
		}
		choices := append([]string{"(None)"}, settingsUIFontChoices(shown)...)
		if ui.Select(c, &shown, choices).Grow(1).Disabled(!customFont).Changed() {
			if shown == "(None)" {
				shown = ""
			}
			m.settingsDraft.uiFontFallback = shown
			m.settingsPreview()
		}
	})
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabelDim(c, oi, "Size:", 220)
		size := float64(m.settingsDraft.uiFontSize)
		if ui.NumberInput(c, &size, 10, 18, 1).Width(100).Disabled(!customFont).Changed() {
			m.settingsDraft.uiFontSize = int(math.Round(size))
			m.settingsPreview()
		}
		ui.Text(c, "px").FontSize(12).TextColor(oi.textMuted).SingleLine()
	})
	contrast := m.settingsDraft.contrastScrollbars
	if ui.Checkbox(c, &contrast, "Use contrast scrollbars").Changed() {
		m.settingsDraft.contrastScrollbars = contrast
		m.settingsPreview()
	}
	redGreen := m.settingsDraft.redGreenVision
	if ui.Checkbox(c, &redGreen, "Adjust colors for red-green vision deficiency").Changed() {
		m.settingsDraft.redGreenVision = redGreen
		m.settingsPreview()
	}
	ui.Row(c).Height(8)

	settingsSectionTitle(c, oi, "UI Options")
	showToolbar := m.settingsDraft.showMainToolbar
	if ui.Checkbox(c, &showToolbar, "Show main toolbar").Changed() {
		m.settingsDraft.showMainToolbar = showToolbar
		m.invalidate()
	}
	showStatus := m.settingsDraft.showStatusBar
	if ui.Checkbox(c, &showStatus, "Show status bar").Changed() {
		m.settingsDraft.showStatusBar = showStatus
		m.invalidate()
	}
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Navigation bar:", 220)
		shown := navigationBarLabel(m.settingsDraft.navigationBar)
		if ui.Select(c, &shown, []string{"Status Bar", "Top", "Don't Show"}).Changed() {
			m.settingsDraft.navigationBar = navigationBarValue(shown)
			m.invalidate()
		}
	})
	compact := m.settingsDraft.compactMode
	if ui.Checkbox(c, &compact, "Compact Mode").Changed() {
		m.settingsDraft.compactMode = compact
		m.invalidate()
	}
	fullPath := m.settingsDraft.fullPathInTitle
	if ui.Checkbox(c, &fullPath, "Always show full path in window header").Changed() {
		m.settingsDraft.fullPathInTitle = fullPath
	}
	projectColors := m.settingsDraft.projectColorsInToolbar
	if ui.Checkbox(c, &projectColors, "Use project colors in main toolbar").Changed() {
		m.settingsDraft.projectColorsInToolbar = projectColors
	}
	differentToolWindowBackground := m.settingsDraft.differentToolWindowBackground
	if ui.Checkbox(c, &differentToolWindowBackground, "Different tool window background").Changed() {
		m.settingsDraft.differentToolWindowBackground = differentToolWindowBackground
		m.invalidate()
	}
	altDnd := m.settingsDraft.dndWithAltOnly
	if ui.Checkbox(c, &altDnd, "Drag-and-drop with Alt pressed only").Changed() {
		m.settingsDraft.dndWithAltOnly = altDnd
	}
	smallIndents := m.settingsDraft.smallTreeIndents
	if ui.Checkbox(c, &smallIndents, "Use smaller indents in trees").Changed() {
		m.settingsDraft.smallTreeIndents = smallIndents
	}
	menuIcons := m.settingsDraft.menuIcons
	if ui.Checkbox(c, &menuIcons, "Display icons in menu items").Changed() {
		m.settingsDraft.menuIcons = menuIcons
	}
	mainMenuAlt := m.settingsDraft.mainMenuAlt
	if ui.Checkbox(c, &mainMenuAlt, "Open main menu with a lone Alt press").Changed() {
		m.settingsDraft.mainMenuAlt = mainMenuAlt
	}
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Main menu:", 220)
		shown := mainMenuChoiceLabel(m.settingsDraft.mainMenuStyle)
		if ui.Select(c, &shown, []string{
			"Hamburger button", "Merged with main toolbar", "Separate toolbar",
		}).Grow(1).Changed() {
			m.settingsDraft.mainMenuStyle = mainMenuStyleValue(shown)
		}
	})
	ui.Row(c).Height(8)

	settingsSectionTitle(c, oi, "Antialiasing")
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "IDE:", 220)
		shown := antialiasingLabel(m.settingsDraft.ideAntialiasing)
		if ui.Select(c, &shown, antialiasingLabels).Width(180).Changed() {
			m.settingsDraft.ideAntialiasing = antialiasingValue(shown)
		}
	})
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Editor:", 220)
		shown := antialiasingLabel(m.settingsDraft.editorAntialiasing)
		if ui.Select(c, &shown, antialiasingLabels).Width(180).Changed() {
			m.settingsDraft.editorAntialiasing = antialiasingValue(shown)
		}
	})
	ui.Row(c).Height(8)

	settingsSectionTitle(c, oi, "Tool Windows")
	showBars := m.settingsDraft.showToolWindowBars
	if ui.Checkbox(c, &showBars, "Show tool window bars").Changed() {
		m.settingsDraft.showToolWindowBars = showBars
		m.invalidate()
	}
	showNames := m.settingsDraft.showToolWindowNames
	if ui.Checkbox(c, &showNames, "Show tool window names").Changed() {
		m.settingsDraft.showToolWindowNames = showNames
		m.invalidate()
	}
	showBadges := m.settingsDraft.showToolWindowBadges
	if ui.Checkbox(c, &showBadges, "Show tool window badges").Changed() {
		m.settingsDraft.showToolWindowBadges = showBadges
		m.invalidate()
	}
	widescreen := m.settingsDraft.widescreenToolWindows
	if ui.Checkbox(c, &widescreen, "Widescreen tool window layout").Changed() {
		m.settingsDraft.widescreenToolWindows = widescreen
		m.invalidate()
	}
}

var antialiasingLabels = []string{"Subpixel", "Greyscale", "No antialiasing"}

func antialiasingLabel(mode string) string {
	switch antialiasingOrDefault(mode) {
	case "subpixel":
		return "Subpixel"
	case "none":
		return "No antialiasing"
	default:
		return "Greyscale"
	}
}

func antialiasingValue(label string) string {
	switch label {
	case "Subpixel":
		return "subpixel"
	case "No antialiasing":
		return "none"
	default:
		return "greyscale"
	}
}

func mainMenuChoiceLabel(style string) string {
	switch mainMenuStyleOrDefault(style) {
	case "merged":
		return "Merged with main toolbar"
	case "toolbar":
		return "Separate toolbar"
	default:
		return "Hamburger button"
	}
}

func mainMenuStyleValue(label string) string {
	switch label {
	case "Merged with main toolbar":
		return "merged"
	case "Separate toolbar":
		return "toolbar"
	default:
		return "hamburger"
	}
}

func settingsUIFontChoices(current string) []string {
	seen := map[string]bool{oiFontUI: true}
	if current = strings.TrimSpace(current); current != "" && current != "(None)" {
		seen[current] = true
	}
	for _, family := range ui.FontFamilies() {
		if name := strings.TrimSpace(family.Name); name != "" {
			seen[name] = true
		}
	}
	choices := make([]string, 0, len(seen))
	for family := range seen {
		choices = append(choices, family)
	}
	sort.Strings(choices)
	return choices
}

func navigationBarLabel(position string) string {
	switch position {
	case "top":
		return "Top"
	case "hidden":
		return "Don't Show"
	default:
		return "Status Bar"
	}
}

func navigationBarValue(label string) string {
	switch label {
	case "Top":
		return "top"
	case "Don't Show":
		return "hidden"
	default:
		return "statusBar"
	}
}

// settingsPreferredThemeRow is one concrete-theme chooser in the preferred
// themes popover opened from Sync with OS.
func (m *Model) settingsPreferredThemeRow(
	c *ui.Context, oi oiColors, dark bool, label string,
) {
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, label+":", 300)
		name := m.settingsDraft.preferredLightTheme
		if dark {
			name = m.settingsDraft.preferredDarkTheme
		}
		shown := "Light"
		if name == "dark" {
			shown = "Dark"
		}
		if ui.Select(c, &shown, []string{"Light", "Dark"}).Width(120).Changed() {
			m.settingsSetPreferredTheme(dark, strings.ToLower(shown))
		}
	})
}

// settingsFontBody is Editor › Font: family, size and line height, ligatures,
// main weight, fallback and an editable native preview.
func (m *Model) settingsFontBody(c *ui.Context, oi oiColors) {
	if m.settingsFontPreview == "" {
		m.settingsFontPreview = "The quick brown fox jumps over the lazy dog.\n0O 1lI {} () [] -> =>"
	}
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Font", 220)
		family := m.settingsDraft.font.Family
		if family == "" {
			family = oiFontMono
		}
		choices := settingsFontFamilyChoices(m.settingsFontMonoOnly, family)
		shown := family
		if ui.Select(c, &shown, choices).Grow(1).Changed() {
			m.settingsSetFontFamily(shown)
		}
	})
	monoOnly := m.settingsFontMonoOnly
	if ui.Checkbox(c, &monoOnly, "Show only monospaced fonts").Changed() {
		m.settingsFontMonoOnly = monoOnly
		m.invalidate()
	}
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Size:", 220)
		size := float64(m.settingsDraft.fontSize)
		in := ui.NumberInput(c, &size, minFontSize, maxFontSize, 1).Width(100)
		if in.Changed() {
			m.settingsSetFont(float32(math.Round(size)))
		}
		ui.Text(c, "px").FontSize(12).TextColor(oi.textMuted).SingleLine()
		lineHeight := float64(m.settingsDraft.font.lineHeightPixels(m.settingsDraft.fontSize))
		lineHeightInput := ui.NumberInput(c, &lineHeight, minFontLineHeight, maxFontLineHeight, 1).Width(92)
		if lineHeightInput.Changed() {
			m.settingsSetFontLineHeight(float32(math.Round(lineHeight)))
		}
		ui.Text(c, "line height").FontSize(12).TextColor(oi.textMuted).SingleLine()
	})
	ligatures := m.settingsDraft.font.Ligatures
	if ui.Checkbox(c, &ligatures, "Enable ligatures").Changed() {
		m.settingsSetFontLigatures(ligatures)
	}

	ui.Row(c).Height(10)
	settingsSectionTitle(c, oi, "Typography Settings")
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Main font weight:", 220)
		shown := fontWeightLabel(m.settingsDraft.font.weight())
		if ui.Select(c, &shown, fontWeightLabels).Width(150).Changed() {
			m.settingsSetFontWeight(fontWeightValue(shown))
		}
	})
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Fallback font:", 220)
		fallback := m.settingsDraft.font.Fallback
		shown := fallback
		if shown == "" {
			shown = "(None)"
		}
		choices := append([]string{"(None)"}, settingsFontFamilyChoices(false, fallback)...)
		if ui.Select(c, &shown, choices).Grow(1).Changed() {
			if shown == "(None)" {
				shown = ""
			}
			m.settingsSetFontFallback(shown)
		}
	})

	ui.Row(c).Height(10)
	settingsSectionTitle(c, oi, "Preview")
	previewHeight := max(float32(116), m.settingsDraft.font.lineHeightPixels(m.settingsDraft.fontSize)*6)
	ui.TextArea(c, &m.settingsFontPreview).Height(previewHeight).Grow(1).
		Font(m.settingsDraft.font.fontFamily()).
		FontSize(m.settingsDraft.fontSize).
		FontWeight(m.settingsDraft.font.weight()).
		FontFeatures(m.settingsDraft.font.features()).
		LineHeight(m.settingsDraft.font.lineHeightPixels(m.settingsDraft.fontSize) / m.settingsDraft.fontSize)
}

// settingsFontFamilyChoices lists the registered and installed families the
// Font page can use. The family already stored stays selectable even if the
// current machine no longer has it.
func settingsFontFamilyChoices(monoOnly bool, current string) []string {
	seen := map[string]bool{oiFontMono: true}
	if current = strings.TrimSpace(current); current != "" {
		seen[current] = true
	}
	for _, family := range ui.FontFamilies() {
		if monoOnly && !family.Monospaced {
			continue
		}
		if name := strings.TrimSpace(family.Name); name != "" {
			seen[name] = true
		}
	}
	choices := make([]string, 0, len(seen))
	for family := range seen {
		choices = append(choices, family)
	}
	sort.Strings(choices)
	return choices
}

var fontWeightLabels = []string{
	"Thin", "ExtraLight", "Light", "Regular", "Medium",
	"SemiBold", "Bold", "ExtraBold", "Black",
}

var fontWeightValues = []int{100, 200, 300, 400, 500, 600, 700, 800, 900}

func fontWeightLabel(weight int) string {
	weight = max(100, min((weight+25)/50*50, 900))
	return fontWeightLabels[(weight-100)/100]
}

func fontWeightValue(label string) int {
	for i, name := range fontWeightLabels {
		if name == label {
			return fontWeightValues[i]
		}
	}
	return 400
}

// settingsGeneralBody is Editor › General: Markdown Images, Mouse Control,
// Scrolling, Soft Wraps, Virtual Space and Save Files in general.tsx's order.
func (m *Model) settingsGeneralBody(c *ui.Context, oi oiColors) {
	settingsSectionTitle(c, oi, "Markdown Images")
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Image folder:", 220)
		in := ui.TextInput(c, &m.settingsDraft.markdownImageFolder).Grow(1)
		if in.Changed() {
			m.settingsSetMarkdownImageFolder(m.settingsDraft.markdownImageFolder)
		}
	})
	ui.Text(c, "Relative to the Markdown file.").
		FontSize(12).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Image name pattern:", 220)
		in := ui.TextInput(c, &m.settingsDraft.markdownImageNamePattern).Grow(1)
		if in.Changed() {
			m.settingsSetMarkdownImageNamePattern(m.settingsDraft.markdownImageNamePattern)
		}
	})
	ui.Text(c, "{timestamp} becomes YYYYMMDD-HHmmss for clipboard images.").
		FontSize(12).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
	ui.Row(c).Height(10)

	settingsSectionTitle(c, oi, "Mouse Control")
	wheel := m.settingsDraft.wheelZoom
	if ui.Checkbox(c, &wheel, "Change font size with Ctrl/Command+Mouse Wheel").Changed() {
		m.settingsSetWheelZoom(wheel)
	}
	drop := m.settingsDraft.dragAndDrop
	if ui.Checkbox(c, &drop, "Enable drag-and-drop functionality in editor").Changed() {
		m.settingsSetDragAndDrop(drop)
	}
	ui.Row(c).Height(10)

	settingsSectionTitle(c, oi, "Scrolling")
	caretScroll := m.settingsDraft.caretScroll
	ui.RadioGroup(c, func() {
		ui.Radio(c, &caretScroll, "center", "Keep caret line centered")
		ui.Radio(c, &caretScroll, "minimize", "Move caret line to minimize scrolling")
	})
	if caretScroll != m.settingsDraft.caretScroll {
		m.settingsSetCaretScroll(caretScroll)
	}
	ui.Row(c).Height(10)

	settingsSectionTitle(c, oi, "Soft Wraps")
	wrap := m.settingsDraft.softWrap
	if ui.Checkbox(c, &wrap, "Soft-wrap files").Changed() {
		m.settingsSetSoftWrap(wrap)
	}
	ui.Row(c).MinHeight(28).Gap(8).AlignItems(ui.Center).PaddingStart(20).Children(func() {
		settingsFormLabel(c, oi, "File patterns", 110)
		in := ui.TextInput(c, &m.settingsDraft.softWrapFiles).Grow(1).
			Disabled(!m.settingsDraft.softWrap)
		if in.Changed() {
			m.settingsPreview()
		}
	})
	ui.Text(c, "Use ; to separate patterns. * matches every file.").
		FontSize(12).TextColor(oi.textMuted).PaddingStart(20).SingleLine().Ellipsis("…")
	wrapIndent := m.settingsDraft.softWrapIndent
	ui.Checkbox(c, &wrapIndent, "Use original line's indent for wrapped parts").
		PaddingStart(20).Disabled(true)
	settingsUnavailableNote(c, oi, "MyGo always preserves the original line's indent when it wraps.")
	ui.Row(c).Height(10)
	settingsSectionTitle(c, oi, "Virtual Space")
	end := m.settingsDraft.scrollPastEnd
	if ui.Checkbox(c, &end, "Show virtual space at file bottom").Changed() {
		m.settingsSetScrollPastEnd(end)
	}
	ui.Row(c).Height(10)
	settingsSectionTitle(c, oi, "Save Files")
	trim := m.settingsDraft.trimTrailing
	if ui.Checkbox(c, &trim, "Remove trailing spaces on:").Changed() {
		m.settingsSetTrimTrailing(trim)
	}
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).PaddingStart(20).Children(func() {
		shown := saveTrimLabel(m.settingsDraft.trimAllLines)
		sel := ui.Select(c, &shown, []string{"Modified lines", "All lines"}).
			Disabled(!m.settingsDraft.trimTrailing)
		if sel.Changed() {
			m.settingsSetTrimAllLines(shown == "All lines")
		}
	})
	keep := m.settingsDraft.keepTrailingOnCaretLine
	check := ui.Checkbox(c, &keep, "Keep trailing spaces on caret line").
		PaddingStart(20).Disabled(!m.settingsDraft.trimTrailing)
	if check.Changed() {
		m.settingsSetKeepTrailingOnCaretLine(keep)
	}
	blank := m.settingsDraft.dropTrailingBlankLines
	if ui.Checkbox(c, &blank, "Remove trailing blank lines at the end of saved files").Changed() {
		m.settingsSetDropTrailingBlankLines(blank)
	}
	final := m.settingsDraft.ensureFinalNewline
	if ui.Checkbox(c, &final, "Ensure every saved file ends with a line break").Changed() {
		m.settingsSetEnsureFinalNewline(final)
	}
	reformat := m.settingsDraft.reformatOnSave
	if ui.Checkbox(c, &reformat, "Reformat code on save").Changed() {
		m.settingsSetReformatOnSave(reformat)
	}
	ui.Text(c, "Runs the language server's formatter before an explicit save.").
		FontSize(12).TextColor(oi.textMuted).PaddingStart(20).SingleLine().Ellipsis("…")
}

// saveTrimLabel names the Remove trailing spaces choice as its select shows
// it.
func saveTrimLabel(all bool) string {
	if all {
		return "All lines"
	}
	return "Modified lines"
}

// settingsSectionTitle is a page section's heading, as the dialog draws the
// titles above groups of rows.
func settingsSectionTitle(c *ui.Context, oi oiColors, title string) {
	ui.Text(c, title).FontSize(12).TextColor(oi.textStrong).Padding(0, 0, 6, 0).SingleLine()
}

func settingsUnavailableNote(c *ui.Context, oi oiColors, text string) {
	ui.Text(c, text).FontSize(11).TextColor(oi.textMuted).PaddingStart(20).
		SingleLine().Ellipsis("…")
}

// settingsEditorAppearanceBody is Editor › General › Appearance: the caret,
// gutter and whitespace switches, then the distraction free text width, in the
// order general.tsx lists them.
func (m *Model) settingsEditorAppearanceBody(c *ui.Context, oi oiColors) {
	caretBlinking := m.settingsDraft.caretBlinking
	caretPeriod := float64(m.settingsDraft.caretBlinkPeriod)
	ui.Row(c).MinHeight(28).Gap(8).AlignItems(ui.Center).Children(func() {
		check := ui.Checkbox(c, &caretBlinking, "Caret blinking (ms):").Disabled(true)
		input := ui.NumberInput(c, &caretPeriod, minCaretBlinkPeriod, maxCaretBlinkPeriod, 50).
			Width(92).
			Disabled(true)
		if check.Changed() {
			m.settingsSetCaretBlinking(caretBlinking)
		}
		if input.Changed() {
			m.settingsSetCaretBlinkPeriod(int(math.Round(caretPeriod)))
		}
	})
	smoothBlink := m.settingsDraft.smoothCaretBlinking
	if ui.Checkbox(c, &smoothBlink, "Use smooth caret blinking").Disabled(true).Changed() {
		m.settingsSetSmoothCaretBlinking(smoothBlink)
	}
	blockCaret := m.settingsDraft.blockCaret
	if ui.Checkbox(c, &blockCaret, "Use block caret").Changed() {
		m.settingsSetBlockCaret(blockCaret)
	}
	smoothMovement := m.settingsDraft.smoothCaretMovement
	movementStyle := "Snappy"
	if m.settingsDraft.caretMovementStyle == "gliding" {
		movementStyle = "Gliding"
	}
	ui.Row(c).MinHeight(28).Gap(8).AlignItems(ui.Center).Children(func() {
		check := ui.Checkbox(c, &smoothMovement, "Use smooth caret movement").Disabled(true)
		sel := ui.Select(c, &movementStyle, []string{"Snappy", "Gliding"}).
			Width(112).
			Disabled(true)
		if check.Changed() {
			m.settingsSetSmoothCaretMovement(smoothMovement)
		}
		if sel.Changed() {
			m.settingsSetCaretMovementStyle(strings.ToLower(movementStyle))
		}
	})
	settingsUnavailableNote(c, oi, "MyGo renders caret timing and motion from its native editor profile.")
	hardWrapGuides := m.settingsDraft.hardWrapGuides
	if ui.Checkbox(c, &hardWrapGuides, "Show hard wrap and visual guides (configured in Code Style options)").Changed() {
		m.settingsSetHardWrapGuides(hardWrapGuides)
	}
	lineNumbers := m.settingsDraft.lineNumbers
	lineNumberMode := "Absolute"
	switch m.settingsDraft.lineNumberMode {
	case "relative":
		lineNumberMode = "Relative"
	case "hybrid":
		lineNumberMode = "Hybrid"
	}
	ui.Row(c).MinHeight(28).Gap(8).AlignItems(ui.Center).Children(func() {
		check := ui.Checkbox(c, &lineNumbers, "Show line numbers")
		shown := "Absolute"
		shown = lineNumberMode
		sel := ui.Select(c, &shown, []string{"Absolute", "Relative", "Hybrid"}).
			Width(112).
			Disabled(!lineNumbers)
		if check.Changed() {
			m.settingsSetLineNumbers(lineNumbers)
		}
		if sel.Changed() {
			m.settingsSetLineNumberMode(strings.ToLower(shown))
		}
	})
	methodSeparators := m.settingsDraft.methodSeparators
	if ui.Checkbox(c, &methodSeparators, "Show method separators").Disabled(true).Changed() {
		m.settingsSetMethodSeparators(methodSeparators)
	}
	settingsUnavailableNote(c, oi, "MyGo does not expose method-separator decoration ranges.")
	showWhitespace := m.settingsDraft.showWhitespace
	if ui.Checkbox(c, &showWhitespace, "Show whitespaces").Changed() {
		m.settingsSetShowWhitespace(showWhitespace)
	}
	ui.Row(c).Gap(14).PaddingStart(20).Children(func() {
		leading := m.settingsDraft.whitespaceLeading
		inner := m.settingsDraft.whitespaceInner
		trailing := m.settingsDraft.whitespaceTrailing
		leadingCheck := ui.Checkbox(c, &leading, "Leading").Disabled(true)
		innerCheck := ui.Checkbox(c, &inner, "Inner").Disabled(true)
		trailingCheck := ui.Checkbox(c, &trailing, "Trailing").Disabled(!showWhitespace)
		if leadingCheck.Changed() {
			m.settingsSetWhitespaceParts(leading, inner, trailing)
		}
		if innerCheck.Changed() {
			m.settingsSetWhitespaceParts(leading, inner, trailing)
		}
		if trailingCheck.Changed() {
			m.settingsSetWhitespaceParts(leading, inner, trailing)
		}
	})
	settingsUnavailableNote(c, oi, "MyGo exposes all, trailing or selection whitespace; leading and inner need finer spans.")
	indentGuides := m.settingsDraft.indentGuides
	if ui.Checkbox(c, &indentGuides, "Show indent guides").Changed() {
		m.settingsSetIndentGuides(indentGuides)
	}
	rainbow := m.settingsDraft.rainbowIndentGuides
	if ui.Checkbox(c, &rainbow, "Rainbow indent guides").
		PaddingStart(20).
		Disabled(true).
		Changed() {
		m.settingsSetRainbowIndentGuides(rainbow)
	}
	intentionBulb := m.settingsDraft.intentionBulb
	if ui.Checkbox(c, &intentionBulb, "Show intention bulb").Disabled(true).Changed() {
		m.settingsSetIntentionBulb(intentionBulb)
	}
	settingsUnavailableNote(c, oi, "Rainbow guide colors and the intention bulb need native editor decoration hooks.")
	renderDocs := m.settingsDraft.renderDocComments
	if ui.Checkbox(c, &renderDocs, "Render documentation comments").Changed() {
		m.settingsSetRenderDocComments(renderDocs)
	}
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Distraction free mode text width:", 260)
		width := float64(m.settingsDraft.zenWidth)
		in := ui.NumberInput(c, &width, minZenWidth, maxZenWidth, 1).Width(100)
		if in.Changed() {
			m.settingsSetZenWidth(int(math.Round(width)))
		}
		ui.Text(c, "chars").FontSize(12).TextColor(oi.textMuted).SingleLine()
	})
}

// settingsBreadcrumbsBody is Editor › General › Breadcrumbs: the Show
// breadcrumbs switch, Top / Bottom placement and MyGo's language chooser.
func (m *Model) settingsBreadcrumbsBody(c *ui.Context, oi oiColors) {
	show := m.settingsDraft.showBreadcrumbs
	if ui.Checkbox(c, &show, "Show breadcrumbs:").Changed() {
		m.settingsSetShowBreadcrumbs(show)
	}
	place := m.settingsDraft.breadcrumbPlacement
	ui.RadioGroup(c, func() {
		ui.Radio(c, &place, "top", "Top")
		ui.Radio(c, &place, "bottom", "Bottom")
	}).PaddingStart(20).Disabled(!m.settingsDraft.showBreadcrumbs)
	if place != m.settingsDraft.breadcrumbPlacement {
		m.settingsSetBreadcrumbPlacement(place)
	}
	ui.Row(c).MinHeight(28).Gap(8).AlignItems(ui.Center).PaddingStart(20).Children(func() {
		settingsFormLabel(c, oi, "Show breadcrumbs for:", 180)
	})
	languages := m.settingsBreadcrumbLanguages()
	if len(languages) == 0 {
		ui.Text(c, "The project's languages appear here after a file is open.").
			FontSize(12).TextColor(oi.textMuted).PaddingStart(20).SingleLine().Ellipsis("…")
		return
	}
	for i := 0; i < len(languages); i += 2 {
		end := min(i+2, len(languages))
		ui.Row(c).Gap(18).PaddingStart(20).Children(func() {
			for _, language := range languages[i:end] {
				language := language
				on := !slices.Contains(m.settingsDraft.breadcrumbDisabled, language)
				check := ui.Checkbox(c, &on, languageDisplayName(language)).
					Disabled(!m.settingsDraft.showBreadcrumbs)
				if check.Changed() {
					m.settingsToggleBreadcrumbLanguage(language)
				}
			}
		})
	}
}

// settingsBreadcrumbLanguages lists the project's open languages plus any
// stored choice, so a settings file loaded before the project opened remains
// editable.
func (m *Model) settingsBreadcrumbLanguages() []string {
	var values []string
	for _, language := range editor.Languages() {
		id := strings.ToLower(strings.TrimSpace(language.Name))
		switch id {
		case "", "plaintext", "log", "diff":
			continue
		}
		values = append(values, id)
	}
	for _, t := range m.allTabs() {
		if t == nil || t.doc == nil {
			continue
		}
		if language := strings.ToLower(strings.TrimSpace(t.doc.Info().Language)); language != "" {
			values = append(values, language)
		}
	}
	values = append(values, m.settingsDraft.breadcrumbDisabled...)
	return uniqueSortedStrings(values)
}

func languageDisplayName(language string) string {
	if language == "" {
		return ""
	}
	for _, known := range editor.Languages() {
		if strings.EqualFold(known.Name, language) {
			return known.Name
		}
	}
	return strings.ToUpper(language[:1]) + language[1:]
}

// settingsEditorTabsBody is Editor › General › Editor Tabs: the strip's
// placement, its overflow and what a tab shows, then the tab order, the
// preview tab, the closing limit and the tab a close activates, in
// general.tsx's order.
func (m *Model) settingsEditorTabsBody(c *ui.Context, oi oiColors) {
	settingsSectionTitle(c, oi, "Appearance")
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).PaddingStart(20).Children(func() {
		settingsFormLabel(c, oi, "Tab placement", 220)
		shown := tabPlacementLabel(m.settingsDraft.tabPlacement)
		sel := ui.Select(c, &shown, []string{"Top", "Bottom", "Left", "Right", "None"})
		if sel.Changed() {
			m.settingsSetTabPlacement(tabPlacementValue(shown))
		}
	})
	// A strip on the left or right is one scrolling column, as it is in
	// OpenIDEA, so Show tabs in does not apply to it.
	overflowOff := m.settingsDraft.tabPlacement == "left" ||
		m.settingsDraft.tabPlacement == "right" || m.settingsDraft.tabPlacement == "none"
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).PaddingStart(20).Children(func() {
		if overflowOff {
			settingsFormLabelDim(c, oi, "Show tabs in:", 220)
		} else {
			settingsFormLabel(c, oi, "Show tabs in:", 220)
		}
	})
	oneRow := !m.settingsDraft.multipleRows
	ui.RadioGroup(c, func() {
		ui.Radio(c, &oneRow, true, "One row, and if tabs don't fit:")
		ui.Radio(c, &oneRow, false, "Multiple rows")
	}).PaddingStart(20).Disabled(overflowOff)
	if oneRow != !m.settingsDraft.multipleRows {
		m.settingsSetMultipleRows(!oneRow)
	}
	squeeze := m.settingsDraft.squeezeTabs
	ui.RadioGroup(c, func() {
		ui.Radio(c, &squeeze, false, "Scroll the tabs panel")
		ui.Radio(c, &squeeze, true, "Squeeze tabs")
	}).PaddingStart(48).Disabled(overflowOff || !oneRow)
	if squeeze != m.settingsDraft.squeezeTabs {
		m.settingsSetSqueezeTabs(squeeze)
	}
	pinnedRow := m.settingsDraft.pinnedSeparateRow
	if ui.Checkbox(c, &pinnedRow, "Show pinned tabs in a separate row").Changed() {
		m.settingsSetPinnedSeparateRow(pinnedRow)
	}
	fileIcon := m.settingsDraft.showFileIcon
	if ui.Checkbox(c, &fileIcon, "Show file icon").Changed() {
		m.settingsSetShowFileIcon(fileIcon)
	}
	extension := m.settingsDraft.showExtension
	if ui.Checkbox(c, &extension, "Show file extension").Changed() {
		m.settingsSetShowExtension(extension)
	}
	directory := m.settingsDraft.showDirectoryForNonUnique
	if ui.Checkbox(c, &directory, "Show directory for non-unique file names").Changed() {
		m.settingsSetShowDirectoryForNonUnique(directory)
	}
	modified := m.settingsDraft.markModified
	if ui.Checkbox(c, &modified, "Mark modified (*)").Changed() {
		m.settingsSetMarkModified(modified)
	}
	fullPath := m.settingsDraft.showFullPathOnHover
	if ui.Checkbox(c, &fullPath, "Show full path on mouse hover").Changed() {
		m.settingsSetShowFullPathOnHover(fullPath)
	}
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).PaddingStart(20).Children(func() {
		settingsFormLabel(c, oi, "Close button position:", 220)
		shown := closePositionLabel(m.settingsDraft.closePosition)
		sel := ui.Select(c, &shown, []string{"Right", "Left", "None"})
		if sel.Changed() {
			m.settingsSetClosePosition(closePositionValue(shown))
		}
	})
	ui.Row(c).Height(10)
	settingsSectionTitle(c, oi, "Tab Order")
	sortTabs := m.settingsDraft.sortAlphabetically
	if ui.Checkbox(c, &sortTabs, "Sort tabs alphabetically").Changed() {
		m.settingsSetSortTabs(sortTabs)
	}
	atEnd := m.settingsDraft.openAtEnd
	if ui.Checkbox(c, &atEnd, "Open new tabs at the end").Changed() {
		m.settingsSetOpenAtEnd(atEnd)
	}
	ui.Row(c).Height(10)
	settingsSectionTitle(c, oi, "Opening Policy")
	previewTab := !m.settingsDraft.noPreviewTab
	if ui.Checkbox(c, &previewTab, "Enable preview tab").Changed() {
		m.settingsSetNoPreviewTab(!previewTab)
	}
	ui.Row(c).Height(10)
	settingsSectionTitle(c, oi, "Closing Policy")
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).PaddingStart(20).Children(func() {
		settingsFormLabel(c, oi, "When the limit is reached", 220)
		shown := "Close least recently used tabs"
		if m.settingsDraft.tabClosingPolicy == "unchanged" {
			shown = "Close unchanged tabs first"
		}
		sel := ui.Select(c, &shown, []string{
			"Close unchanged tabs first",
			"Close least recently used tabs",
		})
		if sel.Changed() {
			if shown == "Close unchanged tabs first" {
				m.settingsSetTabClosingPolicy("unchanged")
			} else {
				m.settingsSetTabClosingPolicy("lru")
			}
		}
	})
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).PaddingStart(20).Children(func() {
		settingsFormLabel(c, oi, "Tab limit", 220)
		limit := float64(m.settingsDraft.tabLimit)
		in := ui.NumberInput(c, &limit, 1, maxTabLimit, 1).Width(100)
		if in.Changed() {
			m.settingsSetTabLimit(int(math.Round(limit)))
		}
	})
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).PaddingStart(20).Children(func() {
		settingsFormLabel(c, oi, "When the current tab is closed, activate", 220)
	})
	activate := m.settingsDraft.activateOnClose
	ui.RadioGroup(c, func() {
		ui.Radio(c, &activate, "left", "The tab on the left")
		ui.Radio(c, &activate, "right", "The tab on the right")
		ui.Radio(c, &activate, "mru", "Most recently opened tab")
	}).PaddingStart(20)
	if activate != m.settingsDraft.activateOnClose {
		m.settingsSetActivateOnClose(activate)
	}
	ui.Text(c, "Tabs with unsaved changes are never closed automatically.").
		FontSize(12).TextColor(oi.textMuted).PaddingStart(20).SingleLine().Ellipsis("…")
}

// tabPlacementLabel is the select's label for a stored placement.
func tabPlacementLabel(place string) string {
	switch place {
	case "bottom":
		return "Bottom"
	case "left":
		return "Left"
	case "right":
		return "Right"
	case "none":
		return "None"
	}
	return "Top"
}

// tabPlacementValue is the staged placement for a select's label.
func tabPlacementValue(label string) string {
	switch label {
	case "Bottom":
		return "bottom"
	case "Left":
		return "left"
	case "Right":
		return "right"
	case "None":
		return "none"
	}
	return "top"
}

// closePositionLabel is the select's label for a stored close position.
func closePositionLabel(place string) string {
	switch place {
	case "left":
		return "Left"
	case "none":
		return "None"
	}
	return "Right"
}

// closePositionValue is the staged close position for a select's label.
func closePositionValue(label string) string {
	switch label {
	case "Left":
		return "left"
	case "None":
		return "none"
	}
	return "right"
}

// settingsCodeFoldingBody is Code Folding's outline and default-fold controls.
// MyGo exposes a language's first import run for opening folds, but no public
// syntax-tree range for file headers or documentation comments, so those two
// choices remain visible and disabled with the actual renderer limitation.
func (m *Model) settingsCodeFoldingBody(c *ui.Context, oi oiColors) {
	folding := m.settingsDraft.showFolding
	if ui.Checkbox(c, &folding, "Show code folding outline").Changed() {
		m.settingsSetShowFolding(folding)
	}
	ui.Row(c).Height(10)
	settingsSectionTitle(c, oi, "Fold by default")
	fileHeader := true
	ui.Checkbox(c, &fileHeader, "File header").Disabled(true).PaddingStart(20)
	foldImports := m.settingsDraft.foldImports
	if ui.Checkbox(c, &foldImports, "Imports").Disabled(!folding).PaddingStart(20).Changed() {
		m.settingsSetFoldImports(foldImports)
	}
	docComments := false
	ui.Checkbox(c, &docComments, "Documentation comments").Disabled(true).PaddingStart(20)
	ui.Text(c, "File headers and documentation comments need syntax-tree fold ranges from MyGo.").
		FontSize(11).TextColor(oi.textMuted).PaddingStart(20).SingleLine().Ellipsis("…")
}

// settingsInlayHintsBody is Editor › Inlay Hints › Code Vision. It mirrors
// OpenIDEA's two metric switches, each metric's Top / Right position, and the
// "no usages" choice. The global hint switches for types, parameters and other
// hints are not drawn yet because the server returns them as one list.
func (m *Model) settingsInlayHintsBody(c *ui.Context, oi oiColors) {
	settingsSectionTitle(c, oi, "Code Vision")
	usages := m.settingsDraft.codeVisionUsages
	if ui.Checkbox(c, &usages, "Usages").Changed() {
		m.settingsDraft.codeVisionUsages = usages
		m.invalidate()
	}
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).PaddingStart(20).Children(func() {
		if m.settingsDraft.codeVisionUsages {
			settingsFormLabel(c, oi, "Position", 180)
		} else {
			settingsFormLabelDim(c, oi, "Position", 180)
		}
		position := codeVisionPositionLabel(m.settingsDraft.codeVisionUsagesPosition)
		sel := ui.Select(c, &position, []string{"Top", "Right"}).
			Disabled(!m.settingsDraft.codeVisionUsages)
		if sel.Changed() {
			m.settingsDraft.codeVisionUsagesPosition = codeVisionPositionValue(position)
			m.invalidate()
		}
	})
	noUsages := m.settingsDraft.codeVisionNoUsages
	check := ui.Checkbox(c, &noUsages, "Show “no usages”").
		Disabled(!m.settingsDraft.codeVisionUsages)
	if check.Changed() {
		m.settingsDraft.codeVisionNoUsages = noUsages
		m.invalidate()
	}
	author := m.settingsDraft.codeVisionAuthor
	if ui.Checkbox(c, &author, "Code author").Changed() {
		m.settingsDraft.codeVisionAuthor = author
		m.invalidate()
	}
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).PaddingStart(20).Children(func() {
		if m.settingsDraft.codeVisionAuthor {
			settingsFormLabel(c, oi, "Position", 180)
		} else {
			settingsFormLabelDim(c, oi, "Position", 180)
		}
		position := codeVisionPositionLabel(m.settingsDraft.codeVisionAuthorPosition)
		sel := ui.Select(c, &position, []string{"Top", "Right"}).
			Disabled(!m.settingsDraft.codeVisionAuthor)
		if sel.Changed() {
			m.settingsDraft.codeVisionAuthorPosition = codeVisionPositionValue(position)
			m.invalidate()
		}
	})
}

func codeVisionPositionLabel(position string) string {
	if position == "right" {
		return "Right"
	}
	return "Top"
}

func codeVisionPositionValue(label string) string {
	if label == "Right" {
		return "right"
	}
	return "top"
}

// settingsKeymapBody is the Keymap page: the map in use and the actions this
// build carries with their staged keys.
func (m *Model) settingsKeymapBody(c *ui.Context, oi oiColors) {
	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "Keymap:").FontSize(12).TextColor(oi.text).SingleLine()
		labels := make([]string, 0, len(keymapPresetOrder))
		for _, preset := range keymapPresetOrder {
			labels = append(labels, preset.Title)
		}
		selected := keymapPresetForID(m.settingsDraft.keymapPreset).Title
		sel := ui.Select(c, &selected, labels)
		if sel.Changed() {
			for _, preset := range keymapPresetOrder {
				if preset.Title == selected {
					m.settingsDraft.keymapPreset = preset.ID
					break
				}
			}
			m.invalidate()
		}
	})
	ui.Row(c).Padding(0, 0, 6, 0).Children(func() {
		ui.Text(c, "Choose Edit on an action and press its new key combination.").
			FontSize(12).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
	})
	conflicts := keymapConflictsIn(m.settingsDraft.keymapPreset, m.settingsDraft.keymap)
	conflicted := map[string]bool{}
	for _, c := range conflicts {
		for _, id := range c.Actions {
			conflicted[id] = true
		}
	}
	if len(conflicts) > 0 {
		ui.Row(c).Padding(0, 0, 6, 0).Children(func() {
			ui.Text(c, fmt.Sprintf("%s; one key is bound to two actions.", count(len(conflicts), "shortcut conflict", "shortcut conflicts"))).
				FontSize(12).TextColor(oi.danger).SingleLine().Ellipsis("…")
		})
	}
	reset := ui.Button(c, "Restore Default Keymap").OnClick(func() {
		m.settingsDraft.keymapPreset = keymapPresetDefinitionsByOS()
		m.settingsDraft.keymap = nil
		m.invalidate()
	})
	_ = reset
	for _, cmd := range commands() {
		bindings := keymapBindingsIn(
			m.settingsDraft.keymapPreset, m.settingsDraft.keymap, cmd.id,
		)
		if len(bindings) == 0 && cmd.shortcut == "" {
			continue
		}
		ui.Row(c.Key("keymap:"+cmd.id)).Height(24).Padding(0, 6).Gap(8).
			AlignItems(ui.Center).Children(func() {
			tone := oi.text
			if conflicted[cmd.id] {
				tone = oi.danger
			}
			ui.Text(c, cmd.title).FontSize(12).TextColor(tone).
				SingleLine().Ellipsis("…").Grow(1)
			label := strings.Join(bindings, " / ")
			if label == "" {
				label = "Unbound"
			}
			ui.Text(c, label).FontSize(12).TextColor(oi.textMuted).SingleLine()
			if _, custom := m.settingsDraft.keymap[cmd.id]; custom {
				x := ui.ButtonBase(c.Key("keymap:clear:"+cmd.id)).Size(18, 18).Radius(4).
					Justify(ui.Center).AlignItems(ui.Center).Cursor(ui.CursorPointer)
				if x.Hovered() {
					x.Background(oi.stripeHover)
				}
				x.Tooltip("Remove shortcut")
				x.Children(func() {
					ui.Text(c, "×").FontSize(12).TextColor(oi.textMuted).SingleLine()
				})
				x.OnClick(func() {
					overrides := cloneKeymap(m.settingsDraft.keymap)
					if overrides == nil {
						overrides = map[string][]string{}
					}
					overrides[cmd.id] = []string{}
					m.settingsDraft.keymap = overrides
					m.invalidate()
				})
			}
			ui.Button(c, "Edit").OnClick(func() { m.settingsRecordKeymap(cmd.id) })
		})
	}
}

// todoColorLabels are the colour names the TODO page's select offers, in the
// order OpenIDEA's TodoSettingsPage lists them.
var todoColorLabels = []string{"Blue", "Red", "Yellow", "Green", "Violet", "Grey"}

// todoColorLabel is the select's label for a stored tone.
func todoColorLabel(tone string) string {
	switch tone {
	case "danger":
		return "Red"
	case "warning":
		return "Yellow"
	case "success":
		return "Green"
	case "violet":
		return "Violet"
	case "muted":
		return "Grey"
	}
	return "Blue"
}

// todoColorValue is the stored tone for a select's label.
func todoColorValue(label string) string {
	switch label {
	case "Red":
		return "danger"
	case "Yellow":
		return "warning"
	case "Green":
		return "success"
	case "Violet":
		return "violet"
	case "Grey":
		return "muted"
	}
	return "info"
}

// todoToneOf is a pattern tone in the palette the page draws with.
func todoToneOf(tone string, oi oiColors) ui.Color {
	switch tone {
	case "danger":
		return oi.danger
	case "warning":
		return oi.warning
	case "success":
		return oi.success
	case "violet":
		return todoViolet
	case "muted":
		return oi.textMuted
	}
	return oi.accent
}

// settingsTodoBody is Editor › TODO: the matching switches and the keyword
// patterns, with their enable switches, colours and actions, as
// TodoSettingsPage.tsx draws them.
func (m *Model) settingsTodoBody(c *ui.Context, oi oiColors) {
	caseOn := m.settingsDraft.todoCaseSensitive
	if ui.Checkbox(c, &caseOn, "Case sensitive").Changed() {
		m.settingsSetTodoCase(caseOn)
	}
	comments := m.settingsDraft.todoCommentsOnly
	if ui.Checkbox(c, &comments, "Only in comments").Changed() {
		m.settingsSetTodoComments(comments)
	}
	ui.Row(c).Height(10)
	ui.Row(c).MinHeight(28).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "Patterns").FontSize(12).TextColor(oi.textStrong).SingleLine().Grow(1)
		ui.Button(c, "Add Pattern").OnClick(m.settingsAddTodoPattern)
	})
	ui.Row(c).MinHeight(24).Gap(8).AlignItems(ui.Center).PaddingStart(20).Children(func() {
		settingsFormLabelDim(c, oi, "", 18)
		settingsFormLabelDim(c, oi, "Keyword", 180)
		settingsFormLabelDim(c, oi, "", 14)
		settingsFormLabelDim(c, oi, "Color", 96)
	})
	for i := range m.settingsDraft.todoPatterns {
		ui.Row(c.Key(fmt.Sprintf("settings:todo:%d", i))).MinHeight(28).Gap(8).
			AlignItems(ui.Center).PaddingStart(20).Children(func() {
			enabled := m.settingsDraft.todoPatterns[i].Enabled
			if ui.Checkbox(c, &enabled, "").Changed() {
				m.settingsSetTodoEnabled(i, enabled)
			}
			keyword := m.settingsDraft.todoPatterns[i].Keyword
			in := ui.TextInput(c, &keyword).Width(180)
			if in.Changed() {
				m.settingsSetTodoKeyword(i, keyword)
			}
			ui.Row(c).Size(14, 14).Radius(3).
				Background(todoToneOf(m.settingsDraft.todoPatterns[i].Color, oi))
			shown := todoColorLabel(m.settingsDraft.todoPatterns[i].Color)
			sel := ui.Select(c, &shown, todoColorLabels)
			if sel.Changed() {
				m.settingsSetTodoColor(i, todoColorValue(shown))
			}
			ui.Button(c, "Remove").OnClick(func() { m.settingsRemoveTodoPattern(i) })
		})
	}
	ui.Row(c).Height(10)
	ui.Button(c, "Restore Defaults").OnClick(m.settingsRestoreTodoPatterns)
}

// settingsPageSummary is the settings tree as text, for the offscreen smoke
// run: each visible row, indented, with the selected and staged marks.
func (m *Model) settingsPageSummary() []string {
	var out []string
	for _, row := range m.settingsVisibleRows() {
		mark := " "
		if row.node.key == m.settingsPage {
			mark = "*"
		}
		staged := " "
		if m.settingsDirtyPage(row.node.key) {
			staged = "!"
		}
		out = append(out, fmt.Sprintf("%s%s%s %s", mark, staged,
			strings.Repeat("  ", row.depth), row.node.title))
	}
	return out
}

// settingsNotificationsBody is Appearance & Behavior › Notifications: the
// global balloon switch and each group's popup choice and log switch, as
// notificationPrefs() reads them.
func (m *Model) settingsNotificationsBody(c *ui.Context, oi oiColors) {
	balloons := m.settingsDraft.notifyBalloons
	if ui.Checkbox(c, &balloons, "Display balloon notifications").Changed() {
		m.settingsDraft.notifyBalloons = balloons
	}
	sound := m.settingsDraft.notifySound
	if ui.Checkbox(c, &sound, "Play sound").Changed() {
		m.settingsDraft.notifySound = sound
	}
	choices := []struct{ id, label string }{
		{"balloon", "Balloon"},
		{"sticky", "Sticky"},
		{"none", "No popup"},
	}
	for _, g := range notificationGroupTitles {
		g := g
		prefs := m.settingsDraft.notifyGroups[g.id]
		ui.Row(c).MaxWidth(720).Height(28).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, g.title).FontSize(12).TextColor(oi.text).SingleLine()
			ui.Spacer(c)
			for _, opt := range choices {
				opt := opt
				on := prefs.Display == opt.id || (opt.id == "balloon" && prefs.Display == "")
				b := ui.ButtonBase(c.Key("settings:notify:"+g.id+":"+opt.id)).
					Height(20).Padding(0, 7).Radius(4).AlignItems(ui.Center).
					Cursor(ui.CursorPointer)
				if on {
					b.Background(oi.stripeSelected)
				} else if b.Hovered() {
					b.Background(oi.stripeHover)
				}
				b.Children(func() {
					tone := oi.textMuted
					if on {
						tone = oi.text
					}
					ui.Text(c, opt.label).FontSize(12).TextColor(tone).SingleLine()
				})
				b.OnClick(func() {
					p := m.settingsDraft.notifyGroups[g.id]
					p.Display = opt.id
					if p.Display == "balloon" {
						p.Display = ""
					}
					m.settingsDraft.notifyGroups[g.id] = p
					m.invalidate()
				})
			}
			log := !prefs.NoLog
			if ui.Checkbox(c, &log, "Log").Changed() {
				p := m.settingsDraft.notifyGroups[g.id]
				p.NoLog = !log
				m.settingsDraft.notifyGroups[g.id] = p
			}
		})
	}
}
