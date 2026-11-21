package app

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

var interactiveRebaseActions = []string{"Pick", "Reword", "Stop to Edit", "Squash", "Fixup", "Drop"}

func rebaseActionValue(label string) string {
	switch label {
	case "Reword":
		return "reword"
	case "Stop to Edit":
		return "edit"
	case "Squash":
		return "squash"
	case "Fixup":
		return "fixup"
	case "Drop":
		return "drop"
	default:
		return "pick"
	}
}

func rebaseActionLabel(value string) string {
	switch value {
	case "reword":
		return "Reword"
	case "edit":
		return "Stop to Edit"
	case "squash":
		return "Squash"
	case "fixup":
		return "Fixup"
	case "drop":
		return "Drop"
	default:
		return "Pick"
	}
}

// OpenInteractiveRebase builds IDEA's target..HEAD plan, oldest first.
func (m *Model) OpenInteractiveRebase(arg any) {
	target := m.selectedLogCommitArg(arg)
	if target == nil {
		m.status = "Select a commit in the Git Log"
		m.invalidate()
		return
	}
	index := -1
	for i := range m.log.commits {
		if m.log.commits[i].ID == target.ID {
			index = i
			break
		}
	}
	if index < 0 {
		m.status = "The commit is outside the loaded Git Log"
		m.invalidate()
		return
	}
	rangeCommits := append([]openidea.Commit(nil), m.log.commits[:index+1]...)
	base := ""
	for _, commit := range rangeCommits {
		if len(commit.Parents) > 1 {
			m.status = "Interactive rebase does not support a range with merge commits"
			m.invalidate()
			return
		}
	}
	oldest := rangeCommits[len(rangeCommits)-1]
	if len(oldest.Parents) > 0 {
		base = oldest.Parents[0]
	}
	m.openVCSMore(vcsMoreRebase, "interactive.rebase",
		fmt.Sprintf("Rebase %s", count(len(rangeCommits), "commit", "commits")),
		"Commits are applied from oldest to newest.")
	m.vcsMore.rebaseBase = base
	for i := len(rangeCommits) - 1; i >= 0; i-- {
		commit := rangeCommits[i]
		m.vcsMore.rebaseCommits = append(m.vcsMore.rebaseCommits, commit)
		m.vcsMore.rebaseActions = append(m.vcsMore.rebaseActions, "pick")
		m.vcsMore.rebaseMessages = append(m.vcsMore.rebaseMessages, commit.Message)
	}
	m.autosquashInteractiveRebase()
}

func (m *Model) autosquashInteractiveRebase() {
	// Git's autosquash keeps fixup!/squash! commits directly after their target.
	for i := 1; i < len(m.vcsMore.rebaseCommits); i++ {
		summary := m.vcsMore.rebaseCommits[i].Summary
		kind := ""
		subject := ""
		switch {
		case strings.HasPrefix(summary, "fixup! "):
			kind, subject = "fixup", strings.TrimSpace(strings.TrimPrefix(summary, "fixup! "))
		case strings.HasPrefix(summary, "squash! "):
			kind, subject = "squash", strings.TrimSpace(strings.TrimPrefix(summary, "squash! "))
		default:
			continue
		}
		target := -1
		for j := i - 1; j >= 0; j-- {
			if m.vcsMore.rebaseCommits[j].Summary == subject ||
				len(subject) >= 4 && strings.HasPrefix(m.vcsMore.rebaseCommits[j].ID, subject) {
				target = j
				break
			}
		}
		if target < 0 || i == target+1 {
			continue
		}
		m.moveInteractiveRebaseRow(i, target+1-i)
		m.vcsMore.rebaseActions[target+1] = kind
	}
}

func (m *Model) moveInteractiveRebaseRow(index, delta int) {
	to := index + delta
	if index < 0 || index >= len(m.vcsMore.rebaseCommits) || to < 0 || to >= len(m.vcsMore.rebaseCommits) {
		return
	}
	m.vcsMore.rebaseCommits[index], m.vcsMore.rebaseCommits[to] =
		m.vcsMore.rebaseCommits[to], m.vcsMore.rebaseCommits[index]
	m.vcsMore.rebaseActions[index], m.vcsMore.rebaseActions[to] =
		m.vcsMore.rebaseActions[to], m.vcsMore.rebaseActions[index]
	m.vcsMore.rebaseMessages[index], m.vcsMore.rebaseMessages[to] =
		m.vcsMore.rebaseMessages[to], m.vcsMore.rebaseMessages[index]
	m.invalidate()
}

func (m *Model) interactiveRebaseProblem() string {
	if len(m.vcsMore.rebaseCommits) == 0 {
		return "There are no commits to rebase"
	}
	firstKept := -1
	changed := false
	for i, action := range m.vcsMore.rebaseActions {
		if action != "drop" && firstKept < 0 {
			firstKept = i
		}
		if action == "reword" && strings.TrimSpace(m.vcsMore.rebaseMessages[i]) == "" {
			return "A reworded commit needs a message"
		}
		if action != "pick" && (action != "reword" ||
			strings.TrimSpace(m.vcsMore.rebaseMessages[i]) != strings.TrimSpace(m.vcsMore.rebaseCommits[i].Message)) {
			changed = true
		}
	}
	if firstKept < 0 {
		return "At least one commit must be kept"
	}
	if action := m.vcsMore.rebaseActions[firstKept]; action == "squash" || action == "fixup" {
		label := "squashed"
		if action == "fixup" {
			label = "fixed up"
		}
		return "The first commit cannot be " + label + " into a previous commit"
	}
	if !changed {
		return "Change at least one action or message"
	}
	return ""
}

func (m *Model) submitInteractiveRebase() {
	if problem := m.interactiveRebaseProblem(); problem != "" {
		m.vcsMore.err = problem
		m.invalidate()
		return
	}
	entries := make([]map[string]any, 0, len(m.vcsMore.rebaseCommits))
	for i, commit := range m.vcsMore.rebaseCommits {
		action := m.vcsMore.rebaseActions[i]
		entry := map[string]any{"commit": commit.ID, "action": action}
		if action == "reword" {
			entry["message"] = strings.TrimSpace(m.vcsMore.rebaseMessages[i])
		}
		entries = append(entries, entry)
	}
	var base any
	if m.vcsMore.rebaseBase != "" {
		base = m.vcsMore.rebaseBase
	}
	title := "Interactive Rebase"
	m.closeVCSMore()
	m.vcsOps = VCSOperationPane{kind: "operation", open: true, title: title}
	m.runVCSGitOperation(title, map[string]any{
		"op": "interactiveRebase", "base": base, "entries": entries,
	})
}

func (m *Model) drawInteractiveRebase(c *ui.Context, p *ui.Theme) {
	ui.Text(c, "Commits are applied from top to bottom.").FontSize(12).TextColor(p.TextMuted)
	ui.Scroll(c).Height(360).Children(func() {
		ui.Column(c).Children(func() {
			for i, commit := range m.vcsMore.rebaseCommits {
				i, commit := i, commit
				row := ui.Column(c.Key("interactive-rebase:"+commit.ID)).Gap(4).Padding(6).
					BorderWidth(0, 0, 1, 0).BorderColor(p.Border)
				row.Children(func() {
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						action := rebaseActionLabel(m.vcsMore.rebaseActions[i])
						if choice := ui.Select(c, &action, interactiveRebaseActions).Width(145); choice.Changed() {
							m.vcsMore.rebaseActions[i] = rebaseActionValue(action)
							m.vcsMore.err = ""
							m.invalidate()
						}
						ui.Text(c, commit.ShortID).Font(oiFontMono).FontSize(12).TextColor(p.TextMuted)
						if m.vcsMore.rebaseActions[i] != "reword" {
							ui.Text(c, commit.Summary).FontSize(13).TextColor(p.Text).
								SingleLine().Ellipsis("…").Grow(1)
							if m.vcsMore.rebaseActions[i] == "drop" {
								ui.Text(c, "will be dropped").FontSize(11).TextColor(p.Danger)
							}
						}
						ui.Button(c, "↑").Size(24, 24).Disabled(i == 0).
							OnClick(func() { m.moveInteractiveRebaseRow(i, -1) })
						ui.Button(c, "↓").Size(24, 24).Disabled(i == len(m.vcsMore.rebaseCommits)-1).
							OnClick(func() { m.moveInteractiveRebaseRow(i, 1) })
					})
					if m.vcsMore.rebaseActions[i] == "reword" {
						message := m.vcsMore.rebaseMessages[i]
						ui.TextArea(c, &message).Height(58).FontSize(12)
						if message != m.vcsMore.rebaseMessages[i] {
							m.vcsMore.rebaseMessages[i] = message
							m.invalidate()
						}
					}
				})
			}
		})
	})
	if problem := m.interactiveRebaseProblem(); problem != "" {
		ui.Text(c, problem).FontSize(12).TextColor(p.TextMuted)
	}
}
