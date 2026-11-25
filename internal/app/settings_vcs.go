package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

func vcsSettingsPages() []settingsPageSpec {
	return []settingsPageSpec{
		{
			id: settingsPageVCS, title: "Version Control",
			order:    5,
			keywords: []string{"git", "vcs", "history", "commit", "confirmation", "shelf"},
			section:  "vcsOverview",
		},
		{
			id: settingsPageVCSLog, title: "Log",
			group: "Version Control", order: 6,
			keywords: []string{"git log", "references", "tags", "columns", "commit time"},
			section:  "vcsLog",
		},
		{
			id: settingsPageVCSMappings, title: "Directory Mappings",
			group: "Version Control", order: 3,
			keywords: []string{"root", "repository", "vcs root", "directory", "git"},
			labels:   []string{"Directory mappings", "Directory", "VCS", "Git"},
			section:  "vcsMappings",
		},
		{
			id: settingsPageVCSGitHub, title: "GitHub",
			group: "Version Control", order: 9,
			keywords: []string{"pull request", "viewed", "review", "mark viewed"},
			section:  "vcsGitHub",
		},
	}
}

func vcsNativeSettingsPages() []nativeSettingsPageSpec {
	toggle := nativeToggle
	number := nativeNumber
	selectField := nativeSelect
	return []nativeSettingsPageSpec{
		{
			page: settingsPageSpec{
				id: settingsPageVCSCommit, title: "Commit",
				group: "Version Control", order: 1,
				keywords: []string{
					"commit checks", "commit message", "modal commit", "commit dialog",
				},
				section: "nativeSettings",
			},
			defaultScope: "project",
			projectOnly:  true,
			sections: []nativeSettingsSection{
				{fields: []nativeSettingsField{
					toggle("vcs.commit.nonModal", "Use non-modal commit interface", true),
					toggle("vcs.commit.clearInitialMessage", "Clear initial commit message", false),
				}},
				{
					title: "Commit Message Inspections",
					fields: []nativeSettingsField{
						toggle("vcs.commit.blankLineAfterSubject", "Blank line between subject and body", true),
						toggle("vcs.commit.limitSubject", "Limit subject line", true),
						number("vcs.commit.subjectLimit", "Limit subject line", 72, 20, 500, 1),
						toggle("vcs.commit.limitBody", "Limit body line", false),
						number("vcs.commit.bodyLimit", "Limit body line", 72, 20, 500, 1),
						toggle("vcs.commit.showRightMargin", "Show right margin", true),
						toggle("vcs.commit.wrapAtMargin", "Wrap when typing reaches right margin", false),
					},
				},
				{
					title: "Commit Checks",
					fields: []nativeSettingsField{
						toggle("vcs.commit.analyzeCode", "Analyze code", true),
						toggle("vcs.commit.checkTodo", "Check TODO", true),
						toggle("vcs.commit.cleanup", "Cleanup", false),
						toggle("vcs.commit.reformat", "Reformat code", false),
						toggle("vcs.commit.optimizeImports", "Optimize imports", false),
					},
				},
			},
			note: "Commit checks run before the native commit action when the backend supports the selected language.",
		},
		{
			page: settingsPageSpec{
				id: settingsPageVCSConfirmation, title: "Confirmation",
				group: "Version Control", order: 2,
				keywords: []string{"add to git", "remove from git", "ask", "patch", "patch reveal", "history"},
				section:  "nativeSettings",
			},
			defaultScope: "project",
			projectOnly:  true,
			sections: []nativeSettingsSection{
				{
					title: "Confirmation",
					fields: []nativeSettingsField{
						selectField("vcs.confirmation.onCreate", "When files are created", "ask",
							nativeChoice("ask", "Ask"),
							nativeChoice("silently", "Add silently"),
							nativeChoice("never", "Do not add")),
						selectField("vcs.confirmation.onDelete", "When files are deleted", "ask",
							nativeChoice("ask", "Ask"),
							nativeChoice("silently", "Remove silently"),
							nativeChoice("never", "Do not remove")),
						toggle("vcs.confirmation.showUpdateOptions", "Show options before Update", true),
						toggle("vcs.confirmation.confirmDropCommits", "Ask for confirmation to drop commits", true),
					},
				},
				{
					title: "Changes",
					fields: []nativeSettingsField{
						toggle("vcs.showDirtyDirectories", "Highlight directories that contain modified files in the Project tree", false),
						selectField("vcs.confirmation.onPatchCreated", "When patch is created", "ask",
							nativeChoice("ask", "Ask"),
							nativeChoice("no", "No"),
							nativeChoice("reveal", revealLabel())),
						toggle("vcs.limitHistory", "Limit history", true),
						number("vcs.historyLimit", "Limit history to", 1000, 10, 100000, 10),
					},
				},
			},
		},
		{
			page: settingsPageSpec{
				id: settingsPageVCSGit, title: "Git",
				group: "Version Control", order: 8,
				keywords: []string{"git", "executable", "push", "pull", "update", "credentials"},
				section:  "nativeSettings",
			},
			defaultScope: "project",
			projectOnly:  true,
			sections: []nativeSettingsSection{
				{
					fields: []nativeSettingsField{
						nativePath("vcs.git.executable", "Path to Git executable", "Auto-detected", false),
						toggle("vcs.git.inlineBlame", "Show inline blame for the current line", false),
						toggle("vcs.git.showBranchInProjectView", "Show branch and incoming/outgoing commits in the Project view", false),
					},
				},
				{
					title: "Commit",
					fields: []nativeSettingsField{
						toggle("vcs.git.warnCrlf", "Warn if CRLF line separators are about to be committed", true),
						toggle("vcs.git.warnDetachedHead", "Warn when committing in detached HEAD or during rebase", true),
						toggle("vcs.git.signOff", "Sign-off commit", false),
						toggle("vcs.git.warnLargeFiles", "Warn when committing files larger than", true),
						number("vcs.git.largeFileMb", "Warn when committing files larger than", 50, 1, 100000, 1),
						toggle("vcs.git.warnBadFileNames", "Warn when committing files with names that might cause issues on other systems", true),
					},
				},
				{
					title: "Push",
					fields: []nativeSettingsField{
						toggle("vcs.git.autoUpdateOnPushRejected", "Auto-update if push of the current branch was rejected", false),
						toggle("vcs.git.showPushDialog", "Show Push dialog for Commit and Push", true),
						toggle("vcs.git.pushDialogOnlyProtected", "Show Push dialog only when committing to protected branches", false),
						nativeText("vcs.git.protectedBranches", "Protected branches", "master; main", "master; main"),
					},
				},
				{
					title: "Update",
					fields: []nativeSettingsField{
						selectField("vcs.git.updateMethod", "Update method", "merge",
							nativeChoice("merge", "Merge"), nativeChoice("rebase", "Rebase")),
						selectField("vcs.git.incomingCheck", "Explicitly check for incoming commits on remotes", "auto",
							nativeChoice("auto", "Auto"), nativeChoice("always", "Always"), nativeChoice("never", "Never")),
					},
				},
				{
					title: "Stash",
					fields: []nativeSettingsField{
						toggle("vcs.git.combineStashAndShelf", "Combine stashes and shelves in one tab", false),
					},
				},
			},
			note: "Saved HTTPS credentials remain in the operating system's credential store.",
		},
		{
			page: settingsPageSpec{
				id: settingsPageVCSShelf, title: "Shelf",
				group: "Version Control", order: 7,
				keywords: []string{"shelve", "unshelve", "applied"},
				section:  "nativeSettings",
			},
			defaultScope: "project",
			projectOnly:  true,
			sections: []nativeSettingsSection{{
				fields: []nativeSettingsField{
					toggle("vcs.shelf.removeApplied", "Remove successfully applied files from the shelf", false),
				},
			}},
		},
	}
}

func (m *Model) settingsVCSOverviewBody(c *ui.Context, oi oiColors) {
	settingsSectionTitle(c, oi, "Version Control")
	ui.Text(c, "Configure commit, confirmation, mappings, Git, diff, log and shelf behavior.").
		FontSize(12).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
	children := []struct {
		page, title, detail string
	}{
		{settingsPageVCSCommit, "Commit", "Commit dialog, message checks and commit inspections"},
		{settingsPageVCSConfirmation, "Confirmation", "When files are added or removed, and when patches are created"},
		{settingsPageVCSMappings, "Directory Mappings", "The project's VCS roots"},
		{settingsPageIssueNavigation, "Issue Navigation", "Turn issue IDs into tracker links"},
		{settingsPageVCSLog, "Log", "References, dates and visible columns"},
		{settingsPageVCSShelf, "Shelf", "Applied shelved changes"},
		{settingsPageVCSGit, "Git", "Executable, commit warnings, Push and Update"},
		{settingsPageVCSGitHub, "GitHub", "Pull request review behavior"},
	}
	for _, child := range children {
		child := child
		ui.Row(c).MinHeight(50).Padding(8, 10).Gap(12).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(oi.border).Children(func() {
			ui.Column(c).Grow(1).Children(func() {
				ui.Text(c, child.title).FontSize(13).Bold().TextColor(oi.text).SingleLine()
				ui.Text(c, child.detail).FontSize(11).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
			})
			ui.Button(c, "Open").Width(72).OnClick(func() { m.settingsSelectPage(child.page) })
		})
	}
}

func (m *Model) settingsVCSMappingsBody(c *ui.Context, oi oiColors) {
	settingsSectionTitle(c, oi, "Directory mappings")
	ui.Row(c).MinHeight(28).Padding(8, 10).Children(func() {
		ui.Text(c, "Directory").FontSize(11).Bold().TextColor(oi.textMuted).Grow(1)
		ui.Text(c, "VCS").FontSize(11).Bold().TextColor(oi.textMuted).Width(120)
	})
	root := m.Root()
	if root == "" {
		ui.Text(c, "No project is open").FontSize(12).TextColor(oi.textMuted).
			Padding(8, 10).SingleLine()
		return
	}
	vcs := "<none>"
	if m.vcs.loaded && m.vcs.err == "" {
		vcs = "Git"
	}
	ui.Row(c).MinHeight(40).Padding(8, 10).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "<Project> "+root).FontSize(12).TextColor(oi.text).Grow(1).SingleLine().Ellipsis("…")
		ui.Text(c, vcs).FontSize(12).TextColor(oi.textMuted).Width(120).SingleLine()
	})
}

func (m *Model) settingsVCSLogBody(c *ui.Context, oi oiColors) {
	options := m.prefs.VCS.Log
	changed := false
	check := func(value **bool, fallback bool, label string) {
		on := prefBool(*value, fallback)
		if ui.Checkbox(c, &on, label).Changed() {
			*value = &on
			changed = true
		}
	}
	check(&options.CompactRefs, true, "Show only the first reference")
	check(&options.TagNames, false, "Show tag names")
	check(&options.CommitTimestamp, false, "Show commit time rather than author time")
	check(&options.RefsOnLeft, false, "Show references on the left")
	check(&options.ShowLineStats, false, "Show lines added and deleted")
	settingsSectionTitle(c, oi, "Visible columns")
	check(&options.ShowAuthor, true, "Author")
	check(&options.ShowHash, false, "Hash")
	check(&options.ShowDate, true, "Date")
	check(&options.ShowGitHubChecks, false, "GitHub Commit Checks")
	if changed {
		m.prefs.VCS.Log = options
		m.savePreferences()
		m.invalidate()
	}
}

func (m *Model) settingsVCSGitHubBody(c *ui.Context, oi oiColors) {
	value := m.prefs.VCS.GitHub.AutoMarkViewed
	if ui.Checkbox(c, &value, "Automatically mark opened files as viewed").Changed() {
		m.prefs.VCS.GitHub.AutoMarkViewed = value
		m.savePreferences()
		m.invalidate()
	}
}

// settingsVCSNativeBody renders the main Version Control pages with the same
// inline toggle-and-number rows, indent and dependent controls as OpenIDEA.
// Their fields stay in nativeSettingsDefinitions so loading, staging, reset,
// backend scope and dotted-key writes use the shared settings machinery.
func (m *Model) settingsVCSNativeBody(c *ui.Context, oi oiColors, page string) bool {
	switch page {
	case settingsPageVCSCommit:
		m.settingsVCSCommitNativeBody(c, oi)
	case settingsPageVCSConfirmation:
		m.settingsVCSConfirmationNativeBody(c, oi)
	case settingsPageVCSGit:
		m.settingsVCSGitNativeBody(c, oi)
	default:
		return false
	}
	return true
}

func (m *Model) settingsVCSCommitNativeBody(c *ui.Context, oi oiColors) {
	page := settingsPageVCSCommit
	m.settingsVCSToggle(c, oi, page, "vcs.commit.nonModal",
		"Use non-modal commit interface", 0,
		"When off, Local Changes and Stash are tabs of the Git tool window and Commit opens a dialog.")
	m.settingsVCSToggle(c, oi, page, "vcs.commit.clearInitialMessage",
		"Clear initial commit message", 0, "")

	settingsSectionTitle(c, oi, "Commit Message Inspections")
	m.settingsVCSToggle(c, oi, page, "vcs.commit.blankLineAfterSubject",
		"Blank line between subject and body", 0, "")
	m.settingsVCSToggleNumber(c, oi, page,
		"vcs.commit.limitSubject", "vcs.commit.subjectLimit",
		"Limit subject line", "", 20, 500, 1)
	m.settingsVCSToggleNumber(c, oi, page,
		"vcs.commit.limitBody", "vcs.commit.bodyLimit",
		"Limit body line", "", 20, 500, 1)
	anyLimit := nativeSettingsBool(m, page, "vcs.commit.limitSubject") ||
		nativeSettingsBool(m, page, "vcs.commit.limitBody")
	m.settingsVCSToggle(c, oi, page, "vcs.commit.showRightMargin",
		"Show right margin", 20, "", !anyLimit)
	m.settingsVCSToggle(c, oi, page, "vcs.commit.wrapAtMargin",
		"Wrap when typing reaches right margin", 20, "",
		!nativeSettingsBool(m, page, "vcs.commit.limitBody"))

	settingsSectionTitle(c, oi, "Commit Checks")
	ui.Row(c).AlignItems(ui.Start).Gap(24).Children(func() {
		ui.Column(c).Grow(1).Children(func() {
			m.settingsVCSToggle(c, oi, page, "vcs.commit.analyzeCode",
				"Analyze code", 0, "Reports errors and warnings from language servers.")
			m.settingsVCSToggle(c, oi, page, "vcs.commit.checkTodo",
				"Check TODO", 0, "Looks for TODO and FIXME in added lines.")
			m.settingsVCSToggle(c, oi, page, "vcs.commit.cleanup",
				"Cleanup", 0, "Applies fix-all code actions.")
		})
		ui.Column(c).Grow(1).Children(func() {
			m.settingsVCSToggle(c, oi, page, "vcs.commit.reformat",
				"Reformat code", 0, "")
			m.settingsVCSToggle(c, oi, page, "vcs.commit.optimizeImports",
				"Optimize imports", 0, "")
		})
	})
}

func (m *Model) settingsVCSConfirmationNativeBody(c *ui.Context, oi oiColors) {
	page := settingsPageVCSConfirmation
	settingsSectionTitle(c, oi, "Confirmation")
	m.settingsVCSRadioRow(c, oi, page, "vcs.confirmation.onCreate",
		"When files are created",
		nativeChoice("ask", "Ask"),
		nativeChoice("silently", "Add silently"),
		nativeChoice("never", "Do not add"))
	m.settingsVCSRadioRow(c, oi, page, "vcs.confirmation.onDelete",
		"When files are deleted",
		nativeChoice("ask", "Ask"),
		nativeChoice("silently", "Remove silently"),
		nativeChoice("never", "Do not remove"))
	m.settingsVCSToggle(c, oi, page, "vcs.confirmation.showUpdateOptions",
		"Show options before Update", 0, "")
	m.settingsVCSToggle(c, oi, page, "vcs.confirmation.confirmDropCommits",
		"Ask for confirmation to drop commits", 0, "Applicable to Git.")

	settingsSectionTitle(c, oi, "Changes")
	m.settingsVCSToggle(c, oi, page, "vcs.showDirtyDirectories",
		"Highlight directories that contain modified files in the Project tree", 0, "")
	m.settingsVCSSelectRow(c, oi, page, "vcs.confirmation.onPatchCreated",
		"When patch is created",
		nativeChoice("ask", "Ask"),
		nativeChoice("no", "No"),
		nativeChoice("reveal", revealLabel()))
	m.settingsVCSToggleNumber(c, oi, page,
		"vcs.limitHistory", "vcs.historyLimit",
		"Limit history to", "rows", 10, 100000, 10)
}

func (m *Model) settingsVCSGitNativeBody(c *ui.Context, oi oiColors) {
	page := settingsPageVCSGit
	m.settingsVCSGitExecutableRow(c, oi)
	m.settingsVCSToggle(c, oi, page, "vcs.git.inlineBlame",
		"Show inline blame for the current line", 0, "")
	m.settingsVCSToggle(c, oi, page, "vcs.git.showBranchInProjectView",
		"Show branch and incoming/outgoing commits in the Project view", 0, "")

	settingsSectionTitle(c, oi, "Commit")
	m.settingsVCSToggle(c, oi, page, "vcs.git.warnCrlf",
		"Warn if CRLF line separators are about to be committed", 0, "")
	m.settingsVCSToggle(c, oi, page, "vcs.git.warnDetachedHead",
		"Warn when committing in detached HEAD or during rebase", 0, "")
	m.settingsVCSToggle(c, oi, page, "vcs.git.signOff",
		"Sign-off commit", 0,
		"Adds a Signed-off-by trailer by default; it can still be switched per commit in Commit Options.")
	m.settingsVCSToggleNumber(c, oi, page,
		"vcs.git.warnLargeFiles", "vcs.git.largeFileMb",
		"Warn when committing files larger than", "MB", 1, 100000, 1)
	m.settingsVCSToggle(c, oi, page, "vcs.git.warnBadFileNames",
		"Warn when committing files with names that might cause issues on other systems", 0, "")

	settingsSectionTitle(c, oi, "Push")
	m.settingsVCSToggle(c, oi, page, "vcs.git.autoUpdateOnPushRejected",
		"Auto-update if push of the current branch was rejected", 0, "")
	showPush := nativeSettingsBool(m, page, "vcs.git.showPushDialog")
	m.settingsVCSToggle(c, oi, page, "vcs.git.showPushDialog",
		"Show Push dialog for Commit and Push", 0, "")
	m.settingsVCSToggle(c, oi, page, "vcs.git.pushDialogOnlyProtected",
		"Show Push dialog only when committing to protected branches", 20, "", !showPush)
	m.settingsVCSTextRow(c, oi, page, "vcs.git.protectedBranches",
		"Protected branches", "master; main",
		"Force push is not allowed to these branches. Separate patterns with ; (wildcards * and ? are supported).")

	settingsSectionTitle(c, oi, "Update")
	m.settingsVCSRadioRow(c, oi, page, "vcs.git.updateMethod",
		"Update method",
		nativeChoice("merge", "Merge"),
		nativeChoice("rebase", "Rebase"))
	m.settingsVCSSelectRow(c, oi, page, "vcs.git.incomingCheck",
		"Explicitly check for incoming commits on remotes",
		nativeChoice("auto", "Auto"),
		nativeChoice("always", "Always"),
		nativeChoice("never", "Never"))
	settingsVCSHint(c, oi, 20,
		"Fetches remotes in the background so incoming commits show in the Branches popup. Auto stops after an authentication failure.")

	settingsSectionTitle(c, oi, "Passwords")
	m.settingsVCSGitCredentialsRow(c, oi)

	settingsSectionTitle(c, oi, "Stash")
	m.settingsVCSToggle(c, oi, page, "vcs.git.combineStashAndShelf",
		"Combine stashes and shelves in one tab", 0, "")
}

func settingsVCSHint(c *ui.Context, oi oiColors, padding float32, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	ui.Text(c, text).FontSize(11).TextColor(oi.textMuted).PaddingStart(padding).
		SingleLine().Ellipsis("…")
}

func (m *Model) settingsVCSToggle(
	c *ui.Context,
	oi oiColors,
	page, key, label string,
	padding float32,
	hint string,
	disabled ...bool,
) {
	value := nativeSettingsBool(m, page, key)
	check := ui.Checkbox(c, &value, label).PaddingStart(padding)
	if len(disabled) > 0 {
		check.Disabled(disabled[0])
	}
	if check.Changed() && (len(disabled) == 0 || !disabled[0]) {
		m.setNativeSettingsValue(page, key, value)
	}
	settingsVCSHint(c, oi, padding+20, hint)
}

func (m *Model) settingsVCSToggleNumber(
	c *ui.Context,
	oi oiColors,
	page, toggleKey, numberKey, label, unit string,
	minValue, maxValue, step float64,
) {
	enabled := nativeSettingsBool(m, page, toggleKey)
	number := nativeSettingsNumberValue(m, page, numberKey)
	ui.Row(c).MinHeight(30).Gap(8).AlignItems(ui.Center).Children(func() {
		check := ui.Checkbox(c, &enabled, label+":")
		if check.Changed() {
			m.setNativeSettingsValue(page, toggleKey, enabled)
		}
		input := ui.NumberInput(c, &number, minValue, maxValue, step).Width(112)
		if input.Changed() {
			m.setNativeSettingsValue(page, numberKey, number)
		}
		if unit != "" {
			ui.Text(c, unit).FontSize(12).
				TextColor(boolColor(enabled, oi.text, oi.textFaint)).SingleLine()
		}
	})
}

func (m *Model) settingsVCSRadioRow(
	c *ui.Context,
	oi oiColors,
	page, key, label string,
	choices ...nativeSettingsChoice,
) {
	value := nativeSettingsString(m, page, key)
	ui.Row(c).MinHeight(32).Gap(10).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, label+":", 230)
		ui.RadioGroup(c, func() {
			for _, choice := range choices {
				choice := choice
				if ui.Radio(c, &value, choice.value, choice.label).Changed() {
					m.setNativeSettingsValue(page, key, value)
					_ = choice
				}
			}
		})
	})
}

func (m *Model) settingsVCSSelectRow(
	c *ui.Context,
	oi oiColors,
	page, key, label string,
	choices ...nativeSettingsChoice,
) {
	value := nativeSettingsString(m, page, key)
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, label+":", 230)
		labels := make([]string, len(choices))
		selected := value
		for i, choice := range choices {
			labels[i] = choice.label
			if choice.value == value {
				selected = choice.label
			}
		}
		selector := ui.Select(c, &selected, labels).Grow(1)
		if selector.Changed() {
			for _, choice := range choices {
				if choice.label == selected {
					m.setNativeSettingsValue(page, key, choice.value)
					break
				}
			}
		}
	})
}

func (m *Model) settingsVCSTextRow(
	c *ui.Context,
	oi oiColors,
	page, key, label, placeholder, hint string,
) {
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, label+":", 230)
		value := nativeSettingsString(m, page, key)
		input := ui.TextInput(c, &value).Grow(1).Placeholder(placeholder)
		if input.Changed() {
			m.setNativeSettingsValue(page, key, value)
		}
	})
	settingsVCSHint(c, oi, 0, hint)
}

func (m *Model) settingsVCSGitCredentialsRow(c *ui.Context, oi oiColors) {
	m.ensureVCSGitCredentials()
	state := m.nativeSettings.pages[settingsPageVCSGit]
	hosts := []string{}
	if encoded := state.aux["git.credentials"]; encoded != "" {
		_ = json.Unmarshal([]byte(encoded), &hosts)
	}
	if state.aux["git.credentialsState"] == "loading" {
		ui.Text(c, "Loading saved credentials…").FontSize(11).TextColor(oi.textMuted).
			PaddingStart(20).SingleLine()
		return
	}
	if errText := state.aux["git.credentialsError"]; errText != "" {
		ui.Text(c, errText).FontSize(11).TextColor(oi.danger).PaddingStart(20).
			SingleLine().Ellipsis("…")
	}
	if len(hosts) == 0 {
		ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
			settingsFormLabel(c, oi, "Passwords:", 230)
			ui.Text(c, "No saved credentials").FontSize(12).TextColor(oi.textMuted).
				Grow(1).SingleLine()
		})
		settingsVCSHint(c, oi, 20,
			"Credentials the IDE answers git's HTTPS prompts with. Saved after a successful fetch / pull / push when \"Remember for this host\" was checked.")
		return
	}
	settingsVCSHint(c, oi, 20,
		"Credentials the IDE answers git's HTTPS prompts with, stored in the app's own secret file (not git's credential.helper). A rejected credential is forgotten automatically.")
	for _, host := range hosts {
		host := host
		ui.Row(c).MinHeight(34).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, host).FontSize(12).TextColor(oi.text).Grow(1).
				SingleLine().Ellipsis("…")
			forgetting := state.aux["git.credentialsForgetting:"+host] == "1"
			label := "Forget"
			if forgetting {
				label = "Forgetting…"
			}
			ui.Button(c, label).Width(92).Disabled(forgetting).
				OnClick(func() { m.forgetVCSGitCredential(host) })
		})
	}
}

func (m *Model) ensureVCSGitCredentials() {
	if m.mgr == nil && m.invokeHook == nil {
		return
	}
	if !m.beginVCSGitAux("git.credentialsState") {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 8*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "git_credentials", nil)
		hosts := []string{}
		if err == nil && string(raw) != "null" {
			err = json.Unmarshal(raw, &hosts)
		}
		m.post(func() {
			m.mutateVCSGitAux(func(aux map[string]string) {
				aux["git.credentialsState"] = "ready"
				if err != nil {
					aux["git.credentialsError"] = err.Error()
				} else {
					encoded, _ := json.Marshal(hosts)
					aux["git.credentials"] = string(encoded)
					delete(aux, "git.credentialsError")
				}
			})
		})
	}()
}

func (m *Model) forgetVCSGitCredential(host string) {
	m.mutateVCSGitAux(func(aux map[string]string) {
		aux["git.credentialsForgetting:"+host] = "1"
		delete(aux, "git.credentialsError")
	})
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 8*time.Second)
		defer cancel()
		_, err := m.invoke(ctx, "git_credential_forget", map[string]any{"key": host})
		m.post(func() {
			m.mutateVCSGitAux(func(aux map[string]string) {
				delete(aux, "git.credentialsForgetting:"+host)
				if err != nil {
					aux["git.credentialsError"] = err.Error()
					return
				}
				hosts := []string{}
				if encoded := aux["git.credentials"]; encoded != "" {
					_ = json.Unmarshal([]byte(encoded), &hosts)
				}
				filtered := hosts[:0]
				for _, existing := range hosts {
					if existing != host {
						filtered = append(filtered, existing)
					}
				}
				encoded, _ := json.Marshal(filtered)
				aux["git.credentials"] = string(encoded)
				delete(aux, "git.credentialsError")
			})
		})
	}()
}

func (m *Model) vcsGitAuxSnapshot() map[string]string {
	m.nativeSettingsMu.Lock()
	defer m.nativeSettingsMu.Unlock()
	state := m.nativeSettings.pages[settingsPageVCSGit]
	aux := make(map[string]string, len(state.aux))
	for key, value := range state.aux {
		aux[key] = value
	}
	return aux
}

func (m *Model) mutateVCSGitAux(change func(map[string]string)) {
	m.nativeSettingsMu.Lock()
	state := m.nativeSettings.pages[settingsPageVCSGit]
	aux := make(map[string]string, len(state.aux)+1)
	for key, value := range state.aux {
		aux[key] = value
	}
	change(aux)
	state.aux = aux
	m.nativeSettings.pages[settingsPageVCSGit] = state
	m.nativeSettingsMu.Unlock()
	m.invalidate()
}

func (m *Model) beginVCSGitAux(key string) bool {
	m.nativeSettingsMu.Lock()
	defer m.nativeSettingsMu.Unlock()
	state := m.nativeSettings.pages[settingsPageVCSGit]
	aux := make(map[string]string, len(state.aux)+1)
	for name, value := range state.aux {
		aux[name] = value
	}
	if aux[key] != "" {
		return false
	}
	aux[key] = "loading"
	state.aux = aux
	m.nativeSettings.pages[settingsPageVCSGit] = state
	return true
}

type vcsGitExecutableInfo struct {
	Version string `json:"version"`
	Path    string `json:"path"`
}

func (m *Model) settingsVCSGitExecutableRow(c *ui.Context, oi oiColors) {
	page := settingsPageVCSGit
	m.ensureVCSGitDetected()
	aux := m.vcsGitAuxSnapshot()
	detected, testing := aux["git.detected"], aux["git.testing"] == "1"
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Path to Git executable:", 230)
		value := nativeSettingsString(m, page, "vcs.git.executable")
		placeholder := "Auto-detected"
		if detected != "" {
			placeholder = "Auto-detected: " + detected
		}
		input := ui.TextInput(c, &value).Grow(1).Placeholder(placeholder)
		if input.Changed() {
			m.setNativeSettingsValue(page, "vcs.git.executable", value)
			m.clearVCSGitTestResult()
		}
		label := "Test"
		if testing {
			label = "Testing…"
		}
		ui.Button(c, label).Width(82).Disabled(testing || (m.mgr == nil && m.invokeHook == nil)).
			OnClick(func() { m.testVCSGitExecutable(value) })
	})
	result := aux["git.result"]
	if result != "" {
		tone := oi.success
		if aux["git.resultOK"] != "1" {
			tone = oi.danger
		}
		ui.Text(c, result).FontSize(11).TextColor(tone).PaddingStart(20).
			SingleLine().Ellipsis("…")
	}
}

func (m *Model) ensureVCSGitDetected() {
	if m.mgr == nil && m.invokeHook == nil {
		return
	}
	if !m.beginVCSGitAux("git.detectedState") {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 8*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "vcs_detected_git", nil)
		var detected string
		if err == nil && string(raw) != "null" {
			err = json.Unmarshal(raw, &detected)
		}
		m.post(func() {
			m.mutateVCSGitAux(func(aux map[string]string) {
				if err == nil {
					aux["git.detected"] = detected
				}
				aux["git.detectedState"] = "ready"
			})
		})
	}()
}

func (m *Model) clearVCSGitTestResult() {
	if m.vcsGitAuxSnapshot() == nil {
		return
	}
	m.mutateVCSGitAux(func(aux map[string]string) {
		delete(aux, "git.result")
		delete(aux, "git.resultOK")
	})
}

func (m *Model) testVCSGitExecutable(path string) {
	m.mutateVCSGitAux(func(aux map[string]string) {
		aux["git.testing"] = "1"
		delete(aux, "git.result")
		delete(aux, "git.resultOK")
	})
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 15*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "vcs_git_version", map[string]any{"path": path})
		var info vcsGitExecutableInfo
		if err == nil {
			err = json.Unmarshal(raw, &info)
		}
		m.post(func() {
			m.mutateVCSGitAux(func(aux map[string]string) {
				delete(aux, "git.testing")
				if err != nil {
					aux["git.result"] = err.Error()
					aux["git.resultOK"] = "0"
				} else {
					version := strings.TrimSpace(info.Version)
					version = strings.Replace(version, "git version", "Git version is", 1)
					aux["git.result"] = fmt.Sprintf("%s (%s)", version, info.Path)
					aux["git.resultOK"] = "1"
				}
			})
		})
	}()
}
