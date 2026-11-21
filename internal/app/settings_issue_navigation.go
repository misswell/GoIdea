package app

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
)

// settingsIssueNavigationBody is Settings › Version Control › Issue
// Navigation. Its table follows OpenIDEA's add/remove/edit flow and stores the
// same regular expressions and link templates.
func (m *Model) settingsIssueNavigationBody(c *ui.Context, oi oiColors) {
	d := &m.settingsDraft
	ui.Text(c, "Matches of an Issue ID regular expression in commit messages become links.").
		FontSize(12).TextColor(oi.textSecondary).Wrap()
	ui.Text(c, "$0 is the whole match and $1… the capture groups.").
		FontSize(11).TextColor(oi.textMuted).SingleLine().Ellipsis("…")

	ui.Row(c).Height(28).Gap(4).AlignItems(ui.Center).Children(func() {
		ui.Button(c, "Add").OnClick(func() {
			d.issueEditing = len(d.issueLinks)
			d.issueLinkDraft = IssueLinkPrefs{}
			m.invalidate()
		})
		remove := ui.Button(c, "Remove").
			Disabled(d.issueSelected < 0 || d.issueSelected >= len(d.issueLinks) || d.issueEditing >= 0)
		if remove.Clicked() {
			m.settingsRemoveIssueLink(d.issueSelected)
		}
		edit := ui.Button(c, "Edit").
			Disabled(d.issueSelected < 0 || d.issueSelected >= len(d.issueLinks) || d.issueEditing >= 0)
		if edit.Clicked() {
			m.settingsEditIssueLink(d.issueSelected)
		}
	})

	ui.Row(c).Height(24).Padding(0, 6).Gap(8).AlignItems(ui.Center).
		Background(oi.frame).Children(func() {
		ui.Text(c, "Issue ID").Width(220).FontSize(11).TextColor(oi.textMuted)
		ui.Text(c, "Issue Link").Grow(1).FontSize(11).TextColor(oi.textMuted)
	})

	shownRows := len(d.issueLinks)
	if d.issueEditing == len(d.issueLinks) {
		shownRows++
	}
	if shownRows == 0 {
		ui.Row(c).Height(30).Padding(0, 8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "No issue navigation links").FontSize(12).TextColor(oi.textMuted)
		})
	}
	for i := 0; i < shownRows; i++ {
		i := i
		if d.issueEditing == i {
			m.settingsIssueEditorRow(c, oi, i)
			continue
		}
		row := d.issueLinks[i]
		rowElement := ui.Row(c.Key(fmt.Sprintf("settings-issue:%d", i))).Height(26).
			Padding(0, 6).Gap(8).AlignItems(ui.Center)
		if d.issueSelected == i {
			rowElement.Background(oi.selected)
		} else if rowElement.Hovered() {
			rowElement.Background(oi.hover)
		}
		rowElement.Children(func() {
			ui.Text(c, row.Pattern).Width(220).Font("monospace").FontSize(11).
				TextColor(issuePatternColor(row, oi)).SingleLine().Ellipsis("…")
			ui.Text(c, row.Link).Grow(1).MinWidth(0).Font("monospace").FontSize(11).
				TextColor(oi.textSecondary).SingleLine().Ellipsis("…")
		})
		if rowElement.Clicked() {
			d.issueSelected = i
			m.invalidate()
		}
		if rowElement.DoubleClicked() {
			m.settingsEditIssueLink(i)
		}
	}

	invalid := make([]string, 0)
	for _, link := range d.issueLinks {
		if issueLinkPatternInvalid(link) {
			invalid = append(invalid, link.Pattern)
		}
	}
	if len(invalid) > 0 {
		ui.Text(c, "Invalid pattern: "+strings.Join(invalid, ", ")).
			FontSize(11).TextColor(oi.danger).Wrap()
	}

	ui.Row(c).Height(28).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Button(c, "Add JIRA Pattern").OnClick(func() {
			m.settingsAddIssuePreset(IssueLinkPrefs{
				Pattern: `[A-Z][A-Z0-9]+-\d+`,
				Link:    "https://jira.example.com/browse/$0",
			})
		})
		ui.Button(c, "Add YouTrack Pattern").OnClick(func() {
			m.settingsAddIssuePreset(IssueLinkPrefs{
				Pattern: `[A-Z][A-Z0-9]+-\d+`,
				Link:    "https://youtrack.example.com/issue/$0",
			})
		})
		ui.Button(c, "Add GitHub Pattern").OnClick(func() {
			m.settingsAddIssuePreset(IssueLinkPrefs{
				Pattern: `#(\d+)`,
				Link:    "https://github.com/owner/repo/issues/$1",
			})
		})
	})
}

func (m *Model) settingsIssueEditorRow(c *ui.Context, oi oiColors, index int) {
	d := &m.settingsDraft
	row := ui.Row(c.Key(fmt.Sprintf("settings-issue-editor:%d", index))).Height(34).
		Padding(0, 6).Gap(6).AlignItems(ui.Center).Background(oi.elev)
	row.Children(func() {
		pattern := ui.TextInput(c, &d.issueLinkDraft.Pattern).Width(220).
			Font("monospace").FontSize(11).Placeholder(`[A-Z]+-\d+`).AutoFocus()
		pattern.OnSubmit(m.settingsCommitIssueLink)
		pattern.OnShortcut(0, ui.KeyEscape, m.settingsCancelIssueLink)
		link := ui.TextInput(c, &d.issueLinkDraft.Link).Grow(1).
			Font("monospace").FontSize(11).
			Placeholder("https://tracker.example.com/issue/$0")
		link.OnSubmit(m.settingsCommitIssueLink)
		link.OnShortcut(0, ui.KeyEscape, m.settingsCancelIssueLink)
		ui.Button(c, "Save").OnClick(m.settingsCommitIssueLink)
		ui.Button(c, "Cancel").OnClick(m.settingsCancelIssueLink)
	})
}

func issuePatternColor(link IssueLinkPrefs, oi oiColors) ui.Color {
	if issueLinkPatternInvalid(link) {
		return oi.danger
	}
	return oi.textSecondary
}

func (m *Model) settingsEditIssueLink(index int) {
	d := &m.settingsDraft
	if index < 0 || index >= len(d.issueLinks) {
		return
	}
	d.issueSelected = index
	d.issueEditing = index
	d.issueLinkDraft = d.issueLinks[index]
	m.invalidate()
}

func (m *Model) settingsCommitIssueLink() {
	d := &m.settingsDraft
	if d.issueEditing < 0 {
		return
	}
	row := IssueLinkPrefs{
		Pattern: strings.TrimSpace(d.issueLinkDraft.Pattern),
		Link:    strings.TrimSpace(d.issueLinkDraft.Link),
	}
	index := d.issueEditing
	switch {
	case row.Pattern == "" && index < len(d.issueLinks):
		d.issueLinks = append(d.issueLinks[:index:index], d.issueLinks[index+1:]...)
		d.issueSelected = -1
	case row.Pattern == "":
		d.issueSelected = -1
	case index >= len(d.issueLinks):
		d.issueLinks = append(d.issueLinks, row)
		d.issueSelected = len(d.issueLinks) - 1
	default:
		d.issueLinks[index] = row
		d.issueSelected = index
	}
	d.issueEditing = -1
	d.issueLinkDraft = IssueLinkPrefs{}
	m.invalidate()
}

func (m *Model) settingsCancelIssueLink() {
	m.settingsDraft.issueEditing = -1
	m.settingsDraft.issueLinkDraft = IssueLinkPrefs{}
	m.invalidate()
}

func (m *Model) settingsRemoveIssueLink(index int) {
	d := &m.settingsDraft
	if index < 0 || index >= len(d.issueLinks) {
		return
	}
	d.issueLinks = append(d.issueLinks[:index:index], d.issueLinks[index+1:]...)
	d.issueSelected = -1
	d.issueEditing = -1
	m.invalidate()
}

func (m *Model) settingsAddIssuePreset(link IssueLinkPrefs) {
	d := &m.settingsDraft
	d.issueLinks = append(d.issueLinks, link)
	d.issueSelected = len(d.issueLinks) - 1
	d.issueEditing = -1
	d.issueLinkDraft = IssueLinkPrefs{}
	m.invalidate()
}
