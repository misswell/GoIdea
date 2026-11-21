package app

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// acceptVCSMoreOutput appends Clone progress without letting a closed or
// replaced dialog consume a channel owned by another operation.
func (m *Model) acceptVCSMoreOutput(id uint32, data []byte) bool {
	if id == 0 || id != m.vcsMore.channel {
		return false
	}
	var line string
	if len(data) > 0 && data[0] == '"' {
		var decoded string
		if err := json.Unmarshal(data, &decoded); err == nil {
			line = decoded
		}
	}
	if line == "" {
		line = string(data)
	}
	line = strings.TrimSpace(line)
	if line != "" {
		m.vcsMore.output = append(m.vcsMore.output, line)
		if over := len(m.vcsMore.output) - maxVCSOutputLines; over > 0 {
			m.vcsMore.output = append(m.vcsMore.output[:0], m.vcsMore.output[over:]...)
		}
		m.invalidate()
	}
	return true
}

func (m *Model) submitVCSMore() {
	if !m.vcsMore.open || m.vcsMore.busy || m.vcsMore.loading {
		return
	}
	switch m.vcsMore.action {
	case "clone":
		m.submitClone()
	case vcsMoreActionShelve:
		m.submitShelve()
	case vcsMoreActionShelfRename:
		m.submitShelfRename()
	case "shelf.delete":
		m.deleteShelf(m.vcsMore.input, m.vcsMore.input2)
	case vcsMoreActionWorktreeAdd:
		m.submitNewWorktree()
	case "worktree.remove":
		m.removeVCSWorktree(m.vcsMore.input)
	case vcsMoreActionWorktreeMove:
		m.moveVCSWorktree(m.vcsMore.input2, m.vcsMore.input3)
	case vcsMoreActionChangelistNew:
		m.submitNewChangelist()
	case vcsMoreActionChangelistEdit:
		m.submitEditChangelist()
	case "changelist.delete":
		m.deleteChangelist(m.vcsMore.input)
	case "changelist.move":
		if list, ok := m.selectedVCSMoreChangelist(); ok {
			m.movePathsToChangelist(list.ID)
		}
	case "branch.delete":
		m.deleteGitLogBranch(strings.TrimSpace(m.vcsMore.input))
	case "changes.getFromRevision":
		parts := strings.Split(m.vcsMore.input, "\x00")
		if len(parts) == 2 {
			m.getLogChangeFromRevision(parts[0], parts[1])
		}
	case "changes.revert":
		parts := strings.Split(m.vcsMore.input, "\x00")
		if len(parts) >= 2 {
			oldPath := ""
			if len(parts) > 2 {
				oldPath = parts[2]
			}
			m.revertSelectedLogChange(parts[0], parts[1], oldPath)
		}
	case "log.goToRef":
		revision := strings.TrimSpace(m.vcsMore.input)
		m.closeVCSMore()
		m.goToGitLogRef(revision)
	case vcsMoreActionReword:
		m.rewordCommit(m.vcsMore.input2, strings.TrimSpace(m.vcsMore.input))
	case vcsMoreActionCheckout:
		m.submitCheckout()
	case "compare.revision":
		m.submitCompareWithRevision()
	case "compare.revisions":
		m.submitCompareRevisions()
	case "apply.patch":
		m.applyPatch()
	case "commit.undo":
		m.undoCommit(m.vcsMore.input)
	case "commit.drop":
		m.runRebasePlan(m.vcsMore.input, "drop", "Drop Commit")
	case "commit.squash":
		m.runRebasePlan(m.vcsMore.input, "squash", "Squash Commits")
	case "interactive.rebase":
		m.submitInteractiveRebase()
	}
}

func (m *Model) submitCompareRevisions() {
	from := strings.TrimSpace(m.vcsMore.input)
	to := strings.TrimSpace(m.vcsMore.input2)
	if from == "" || to == "" {
		m.vcsMore.err = "Enter both revisions"
		m.invalidate()
		return
	}
	m.vcsMore.busy, m.vcsMore.err = true, ""
	m.invalidate()
	m.compareRevisions(from, to)
}

func (m *Model) selectedVCSMoreChangelist() (changelistEntry, bool) {
	if m.vcsMore.choice < 0 || m.vcsMore.choice >= len(m.vcsMore.lists.Lists) {
		return changelistEntry{}, false
	}
	return m.vcsMore.lists.Lists[m.vcsMore.choice], true
}

func (m *Model) chooseVCSCloneDestination() {
	parent := m.win
	start := m.vcsMore.input2
	if start == "" {
		start = defaultCloneParent()
	}
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: parent, Title: "Clone Repository", ButtonLabel: "Choose",
			DefaultPath: start, Directory: true, CreateDirectories: true,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		m.post(func() {
			if m.vcsMore.kind == vcsMoreClone && !m.vcsMore.busy {
				m.vcsMore.input2 = paths[0]
				m.invalidate()
			}
		})
	}()
}

func (m *Model) cancelVCSMore() {
	if m.vcsMore.action == "clone" && m.vcsMore.busy && m.vcsMore.request != 0 {
		m.cancelRequest(m.vcsMore.request)
		m.vcsMore.status = "Cancelling Clone…"
		m.invalidate()
		return
	}
	if !m.vcsMore.busy {
		m.closeVCSMore()
	}
}

func (m *Model) activateVCSMoreItem(index int) {
	if index < 0 || index >= len(m.vcsMore.items) {
		return
	}
	m.vcsMore.choice = index
	item := m.vcsMore.items[index]
	switch {
	case m.vcsMore.action == "commit.message":
		if m.commit == nil {
			m.OpenCommit()
		}
		if m.commit != nil {
			m.commit.message = item.Label
			m.commit.err = ""
		}
		m.closeVCSMore()
	case m.vcsMore.kind == vcsMoreChanges && item.Kind == "shelf":
		m.showShelfFileDiff(shelfDiffFile{
			Path: item.Path, Before: textPointer(item.Before), After: textPointer(item.After),
		}, false)
	case m.vcsMore.kind == vcsMoreChanges && item.Revision != "":
		m.openRevisionFileDiff(item)
	case m.vcsMore.action == "current.revision":
		m.CompareCommitWithLocal(map[string]any{"id": item.ID})
	case m.vcsMore.kind == vcsMoreConflicts:
		path := item.Path
		m.OpenMergeEditor(path)
	}
	m.invalidate()
}

func textPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func (m *Model) drawVCSMoreItems(c *ui.Context, p *ui.Theme) {
	if m.vcsMore.loading {
		ui.Text(c, "Loading…").FontSize(12).TextColor(p.TextMuted).SingleLine()
		return
	}
	if len(m.vcsMore.items) == 0 {
		ui.Text(c, "Nothing to show").FontSize(12).TextColor(p.TextMuted).SingleLine()
		return
	}
	ui.Scroll(c).Height(300).Children(func() {
		ui.Column(c).Children(func() {
			for i, item := range m.vcsMore.items {
				i, item := i, item
				label := firstLine(item.Label)
				if label == "" {
					label = filepath.Base(item.Path)
				}
				row := ui.Row(c.Key(fmt.Sprintf("vcs-more-item:%s:%d:%s", m.vcsMore.action, i, item.Path))).
					Height(32).Padding(0, 8).Gap(10).AlignItems(ui.Center)
				switch {
				case i == m.vcsMore.choice:
					row.Background(p.Selection)
				case row.Hovered():
					row.Background(p.SurfaceHover)
				}
				row.Children(func() {
					ui.Text(c, label).FontSize(13).TextColor(p.Text).SingleLine().Ellipsis("…")
					if item.Detail != "" {
						ui.Spacer(c)
						ui.Text(c, item.Detail).FontSize(12).TextColor(p.TextMuted).
							SingleLine().Ellipsis("…")
					}
				})
				if row.Clicked() {
					m.vcsMore.choice = i
					m.vcsMore.err = ""
					m.invalidate()
				}
				if row.DoubleClicked() {
					m.activateVCSMoreItem(i)
				}
			}
		})
	})
}

func (m *Model) drawVCSMoreError(c *ui.Context, p *ui.Theme) {
	if m.vcsMore.err != "" {
		ui.Text(c, m.vcsMore.err).FontSize(12).TextColor(p.Danger)
	}
	if m.vcsMore.status != "" {
		ui.Text(c, m.vcsMore.status).FontSize(12).TextColor(p.TextMuted)
	}
	if m.vcsMore.busy {
		ui.Text(c, "Working…").FontSize(12).TextColor(p.TextMuted).SingleLine()
	}
}

func (m *Model) vcsMorePrimaryLabel() string {
	switch m.vcsMore.action {
	case "clone":
		return "Clone"
	case vcsMoreActionShelve:
		return "Shelve"
	case vcsMoreActionShelfRename:
		return "Rename"
	case "shelf.delete", "changelist.delete", "worktree.remove":
		return "Delete"
	case vcsMoreActionWorktreeAdd:
		return "Create"
	case vcsMoreActionWorktreeMove:
		return "Move"
	case vcsMoreActionChangelistNew:
		return "Create"
	case vcsMoreActionChangelistEdit:
		return "Save"
	case "changelist.move":
		return "Move"
	case vcsMoreActionReword:
		return "Save Message"
	case vcsMoreActionCheckout:
		return "Checkout"
	case "compare.revision", "compare.revisions":
		return "Compare"
	case "apply.patch":
		return "Apply Patch"
	case "commit.undo":
		return "Undo Commit"
	case "commit.drop":
		return "Drop Commit"
	case "commit.squash":
		return "Squash"
	case "interactive.rebase":
		return "Start Rebasing"
	default:
		return "OK"
	}
}

func (m *Model) drawVCSMorePrompt(c *ui.Context, p *ui.Theme) {
	switch m.vcsMore.action {
	case vcsMoreActionWorktreeAdd:
		ui.Text(c, "Directory").FontSize(12).TextColor(p.TextMuted)
		if in := ui.TextInput(c, &m.vcsMore.input).AutoFocus(); in.Submitted() {
			m.submitVCSMore()
		}
		ui.Text(c, "Starting revision").FontSize(12).TextColor(p.TextMuted)
		if in := ui.TextInput(c, &m.vcsMore.input2).Font(oiFontMono); in.Submitted() {
			m.submitVCSMore()
		}
		ui.Text(c, "New branch (optional)").FontSize(12).TextColor(p.TextMuted)
		if in := ui.TextInput(c, &m.vcsMore.input3); in.Submitted() {
			m.submitVCSMore()
		}
	case vcsMoreActionWorktreeMove:
		ui.Text(c, "Worktree").FontSize(12).TextColor(p.TextMuted)
		ui.Text(c, m.vcsMore.input2).FontSize(12).Font(oiFontMono).TextColor(p.Text).SingleLine().
			Ellipsis("…")
		ui.Text(c, "New directory").FontSize(12).TextColor(p.TextMuted)
		if in := ui.TextInput(c, &m.vcsMore.input3).AutoFocus().Font(oiFontMono); in.Submitted() {
			m.submitVCSMore()
		}
	case vcsMoreActionShelve:
		ui.Text(c, "Shelf name").FontSize(12).TextColor(p.TextMuted)
		if in := ui.TextInput(c, &m.vcsMore.input).AutoFocus(); in.Submitted() {
			m.submitVCSMore()
		}
		if len(m.vcsMore.items) > 0 {
			ui.Text(c, m.vcsMore.items[0].Detail).FontSize(12).TextColor(p.TextMuted).SingleLine()
		}
	case vcsMoreActionChangelistEdit:
		ui.Text(c, "Name").FontSize(12).TextColor(p.TextMuted)
		if in := ui.TextInput(c, &m.vcsMore.input).AutoFocus(); in.Submitted() {
			m.submitVCSMore()
		}
		ui.Text(c, "Comment").FontSize(12).TextColor(p.TextMuted)
		ui.TextArea(c, &m.vcsMore.input2).Height(72)
	case vcsMoreActionReword:
		ui.Text(c, "Commit message").FontSize(12).TextColor(p.TextMuted)
		ui.TextArea(c, &m.vcsMore.input).Height(150).AutoFocus()
	case "compare.revisions":
		ui.Text(c, "Left revision").FontSize(12).TextColor(p.TextMuted)
		if in := ui.TextInput(c, &m.vcsMore.input).AutoFocus().Font(oiFontMono); in.Submitted() {
			m.submitVCSMore()
		}
		ui.Text(c, "Right revision").FontSize(12).TextColor(p.TextMuted)
		if in := ui.TextInput(c, &m.vcsMore.input2).Font(oiFontMono); in.Submitted() {
			m.submitVCSMore()
		}
	case vcsMoreActionCheckout, "compare.revision":
		label := "Revision"
		if m.vcsMore.action == "compare.revision" {
			label = "Compare with branch or revision"
		}
		ui.Text(c, label).FontSize(12).TextColor(p.TextMuted)
		if in := ui.TextInput(c, &m.vcsMore.input).AutoFocus().Font(oiFontMono); in.Submitted() {
			m.submitVCSMore()
		}
	default:
		ui.Text(c, "Value").FontSize(12).TextColor(p.TextMuted)
		if in := ui.TextInput(c, &m.vcsMore.input).AutoFocus(); in.Submitted() {
			m.submitVCSMore()
		}
	}
}

func (m *Model) drawVCSMoreShelf(c *ui.Context, p *ui.Theme) {
	ui.Row(c).Gap(6).Children(func() {
		ui.Button(c, "Unshelve").Disabled(m.vcsMore.loading || m.vcsMore.busy || len(m.vcsMore.shelves) == 0).
			OnClick(func() { m.UnshelveArg(nil) })
		ui.Button(c, "Show Diff").Disabled(m.vcsMore.loading || m.vcsMore.busy || len(m.vcsMore.shelves) == 0).
			OnClick(func() { m.ShowShelfDiffArg(nil, false) })
		ui.Button(c, "Rename…").Disabled(m.vcsMore.loading || m.vcsMore.busy || len(m.vcsMore.shelves) == 0).
			OnClick(func() { m.ShelfRenameArg(nil) })
		ui.Button(c, "Delete").Disabled(m.vcsMore.loading || m.vcsMore.busy || len(m.vcsMore.shelves) == 0).
			OnClick(func() { m.ShelfDeleteArg(nil) })
		ui.Button(c, "Refresh").Disabled(m.vcsMore.loading || m.vcsMore.busy).
			OnClick(m.LoadVCSShelf)
	})
	ui.Row(c).Gap(6).Children(func() {
		ui.Button(c, "Create Patch…").Disabled(m.vcsMore.loading || m.vcsMore.busy || len(m.vcsMore.shelves) == 0).
			OnClick(func() { m.CreateShelfPatchArg(nil) })
		ui.Button(c, "Copy Patch").Disabled(m.vcsMore.loading || m.vcsMore.busy || len(m.vcsMore.shelves) == 0).
			OnClick(func() { m.CopyShelfPatchArg(nil) })
		ui.Button(c, "Import Patches…").Disabled(m.vcsMore.loading || m.vcsMore.busy).
			OnClick(func() { m.ImportShelfPatches(nil) })
	})
	if m.vcsMore.loading {
		ui.Text(c, "Loading the shelf…").FontSize(12).TextColor(p.TextMuted)
		return
	}
	if len(m.vcsMore.shelves) == 0 {
		ui.Text(c, "No shelved changes").FontSize(12).TextColor(p.TextMuted)
		return
	}
	ui.Scroll(c).Height(300).Children(func() {
		ui.Column(c).Children(func() {
			for i, shelf := range m.vcsMore.shelves {
				i, shelf := i, shelf
				detail := count(len(shelf.Files), "file", "files")
				if shelf.Branch != nil && *shelf.Branch != "" {
					detail += " · " + *shelf.Branch
				}
				row := ui.Row(c.Key("vcs-shelf:"+shelf.ID)).Height(32).Padding(0, 8).
					Gap(10).AlignItems(ui.Center)
				if i == m.vcsMore.choice {
					row.Background(p.Selection)
				} else if row.Hovered() {
					row.Background(p.SurfaceHover)
				}
				row.Children(func() {
					ui.Text(c, shelf.Name).FontSize(13).TextColor(p.Text).SingleLine().Ellipsis("…")
					ui.Spacer(c)
					ui.Text(c, detail).FontSize(12).TextColor(p.TextMuted).SingleLine()
				})
				if row.Clicked() {
					m.vcsMore.choice = i
					m.invalidate()
				}
				if row.DoubleClicked() {
					m.ShowShelfDiffArg(map[string]any{"id": shelf.ID}, false)
				}
			}
		})
	})
}

func (m *Model) drawVCSMoreWorktrees(c *ui.Context, p *ui.Theme) {
	selected, hasSelection := m.selectedVCSWorktree()
	canChange := hasSelection && m.worktreeCanChange(selected)
	ui.Row(c).Gap(6).Children(func() {
		ui.Button(c, "Open").Disabled(m.vcsMore.loading || m.vcsMore.busy || len(m.vcsMore.worktrees) == 0).
			OnClick(m.OpenSelectedVCSWorktree)
		ui.Button(c, "New…").Disabled(m.vcsMore.loading || m.vcsMore.busy).
			OnClick(m.OpenNewVCSWorktree)
		ui.Button(c, "Move…").Disabled(m.vcsMore.loading || m.vcsMore.busy || !canChange).
			OnClick(m.ChooseMoveVCSWorktree)
		ui.Button(c, "Remove").Disabled(m.vcsMore.loading || m.vcsMore.busy || !canChange).
			OnClick(m.AskRemoveSelectedVCSWorktree)
		ui.Button(c, "Refresh").Disabled(m.vcsMore.loading || m.vcsMore.busy).
			OnClick(m.loadVCSWorktrees)
	})
	if m.vcsMore.loading {
		ui.Text(c, "Loading worktrees…").FontSize(12).TextColor(p.TextMuted)
		return
	}
	if len(m.vcsMore.worktrees) == 0 {
		ui.Text(c, "No worktrees").FontSize(12).TextColor(p.TextMuted)
		return
	}
	ui.Scroll(c).Height(300).Children(func() {
		ui.Column(c).Children(func() {
			for i, worktree := range m.vcsMore.worktrees {
				i, worktree := i, worktree
				row := ui.Row(c.Key("vcs-worktree:"+worktree.Path)).Height(34).Padding(0, 8).
					Gap(10).AlignItems(ui.Center)
				if i == m.vcsMore.choice {
					row.Background(p.Selection)
				} else if row.Hovered() {
					row.Background(p.SurfaceHover)
				}
				row.Children(func() {
					label := filepath.Base(worktree.Path)
					if worktree.Branch != nil && *worktree.Branch != "" {
						label = *worktree.Branch
					} else if worktree.Head != nil && *worktree.Head != "" {
						label = shortID(*worktree.Head)
					}
					ui.Text(c, label).FontSize(13).TextColor(p.Text).SingleLine()
					ui.Text(c, worktree.Path).FontSize(12).TextColor(p.TextMuted).
						SingleLine().Ellipsis("…").Grow(1)
					if worktree.Main {
						ui.Text(c, "main").FontSize(11).TextColor(p.TextMuted)
					}
				})
				if row.Clicked() {
					m.vcsMore.choice = i
					m.invalidate()
				}
				if row.DoubleClicked() {
					m.OpenSelectedVCSWorktree()
				}
			}
		})
	})
}

func (m *Model) drawVCSMoreChangelists(c *ui.Context, p *ui.Theme) {
	moving := m.vcsMore.action == "changelist.move"
	ui.Row(c).Gap(6).Children(func() {
		if moving {
			ui.Button(c, "Move Here").Disabled(m.vcsMore.loading || m.vcsMore.busy || len(m.vcsMore.lists.Lists) == 0).
				OnClick(m.submitVCSMore)
		} else {
			ui.Button(c, "Set Active").Disabled(m.vcsMore.loading || m.vcsMore.busy || len(m.vcsMore.lists.Lists) == 0).
				OnClick(func() {
					if list, ok := m.selectedVCSMoreChangelist(); ok {
						m.SetActiveChangelistArg(list.ID)
					}
				})
			ui.Button(c, "Edit…").Disabled(m.vcsMore.loading || m.vcsMore.busy || len(m.vcsMore.lists.Lists) == 0).
				OnClick(func() {
					if list, ok := m.selectedVCSMoreChangelist(); ok {
						m.OpenEditChangelist(list.ID)
					}
				})
			ui.Button(c, "Delete").Disabled(m.vcsMore.loading || m.vcsMore.busy || len(m.vcsMore.lists.Lists) == 0).
				OnClick(func() {
					if list, ok := m.selectedVCSMoreChangelist(); ok {
						m.DeleteChangelistArg(list.ID)
					}
				})
		}
		ui.Button(c, "New…").Disabled(m.vcsMore.loading || m.vcsMore.busy).OnClick(m.OpenNewChangelist)
		ui.Button(c, "Refresh").Disabled(m.vcsMore.loading || m.vcsMore.busy).
			OnClick(func() { m.loadChangelists(false) })
	})
	if !moving {
		ui.Button(c, "Commit Selected Changelist…").
			Disabled(m.vcsMore.loading || m.vcsMore.busy || len(m.vcsMore.lists.Lists) == 0).
			OnClick(func() {
				if list, ok := m.selectedVCSMoreChangelist(); ok {
					m.CommitChangelistArg(list.ID)
				}
			})
	}
	if m.vcsMore.loading {
		ui.Text(c, "Loading changelists…").FontSize(12).TextColor(p.TextMuted)
		return
	}
	if len(m.vcsMore.lists.Lists) == 0 {
		ui.Text(c, "No changelists yet").FontSize(12).TextColor(p.TextMuted)
		return
	}
	ui.Scroll(c).Height(300).Children(func() {
		ui.Column(c).Children(func() {
			for i, list := range m.vcsMore.lists.Lists {
				i, list := i, list
				files := 0
				for _, listID := range m.vcsMore.lists.Files {
					if listID == list.ID {
						files++
					}
				}
				row := ui.Row(c.Key("vcs-changelist:"+list.ID)).Height(34).Padding(0, 8).
					Gap(10).AlignItems(ui.Center)
				if i == m.vcsMore.choice {
					row.Background(p.Selection)
				} else if row.Hovered() {
					row.Background(p.SurfaceHover)
				}
				row.Children(func() {
					ui.Text(c, list.Name).FontSize(13).TextColor(p.Text).SingleLine()
					if list.ID == m.vcsMore.lists.Active {
						ui.Text(c, "active").FontSize(11).TextColor(p.TextMuted)
					}
					if list.Comment != "" {
						ui.Text(c, list.Comment).FontSize(12).TextColor(p.TextMuted).SingleLine().
							Ellipsis("…")
					}
					ui.Spacer(c)
					ui.Text(c, count(files, "file", "files")).FontSize(12).TextColor(p.TextMuted)
				})
				if row.Clicked() {
					m.vcsMore.choice = i
					m.invalidate()
				}
				if row.DoubleClicked() {
					if moving {
						m.movePathsToChangelist(list.ID)
					} else {
						m.CommitChangelistArg(list.ID)
					}
				}
			}
		})
	})
}

func (m *Model) drawVCSMoreConflicts(c *ui.Context, p *ui.Theme) {
	m.drawVCSMoreItems(c, p)
	ui.Row(c).Gap(6).Children(func() {
		disabled := m.vcsMore.loading || m.vcsMore.busy || len(m.vcsMore.items) == 0
		ui.Button(c, "Open").Disabled(disabled).OnClick(func() { m.activateVCSMoreItem(m.vcsMore.choice) })
		ui.Button(c, "Accept Yours").Disabled(disabled).OnClick(func() {
			m.resolveAllConflicts(m.selectedConflictPath(), "yours")
		})
		ui.Button(c, "Accept Theirs").Disabled(disabled).OnClick(func() {
			m.resolveAllConflicts(m.selectedConflictPath(), "theirs")
		})
		ui.Button(c, "Accept Both").Disabled(disabled).OnClick(func() {
			m.resolveAllConflicts(m.selectedConflictPath(), "both")
		})
	})
}

func (m *Model) vcsMoreDialog(c *ui.Context, p *ui.Theme) {
	if !m.vcsMore.open {
		return
	}
	open := m.vcsMore.open
	width := float32(640)
	if m.vcsMore.kind == vcsMorePrompt || m.vcsMore.kind == vcsMoreConfirm {
		width = 520
	}
	dialog := ui.Modal(c.Key("vcs-more:"+m.vcsMore.kind+":"+m.vcsMore.action), &open, func() {
		ui.Column(c).Width(width).Gap(10).Children(func() {
			ui.Text(c, m.vcsMore.title).FontSize(15).Bold().TextColor(p.Text).SingleLine()
			if m.vcsMore.hint != "" {
				ui.Text(c, m.vcsMore.hint).FontSize(12).TextColor(p.TextMuted)
			}
			switch m.vcsMore.kind {
			case vcsMorePrompt:
				m.drawVCSMorePrompt(c, p)
			case vcsMoreConfirm:
				if m.vcsMore.input != "" && m.vcsMore.action != "worktree.remove" {
					ui.Text(c, shortID(m.vcsMore.input)).Font(oiFontMono).FontSize(12).TextColor(p.TextMuted)
				}
			case vcsMoreClone:
				ui.Text(c, "Repository URL").FontSize(12).TextColor(p.TextMuted)
				if in := ui.TextInput(c, &m.vcsMore.input).AutoFocus().Font(oiFontMono); in.Submitted() {
					m.submitVCSMore()
				}
				ui.Text(c, "Directory").FontSize(12).TextColor(p.TextMuted)
				ui.Row(c).Gap(6).Children(func() {
					if in := ui.TextInput(c, &m.vcsMore.input2).Font(oiFontMono).Grow(1); in.Submitted() {
						m.submitVCSMore()
					}
					ui.Button(c, "Browse…").Disabled(m.vcsMore.busy).OnClick(m.chooseVCSCloneDestination)
				})
				if len(m.vcsMore.output) > 0 {
					output := strings.Join(m.vcsMore.output, "\n")
					ui.TextArea(c, &output).ReadOnly(true).Height(150).Font(oiFontMono).FontSize(11)
				}
			case vcsMoreShelf:
				m.drawVCSMoreShelf(c, p)
			case vcsMoreWorktrees:
				m.drawVCSMoreWorktrees(c, p)
			case vcsMoreChangelists:
				m.drawVCSMoreChangelists(c, p)
			case vcsMorePatchPreview:
				m.drawVCSMoreItems(c, p)
				ui.Row(c).Gap(6).Children(func() {
					ui.Button(c, "Apply Patch").
						Disabled(m.vcsMore.loading || m.vcsMore.busy || m.vcsMore.patch == "").
						OnClick(m.applyPatch)
				})
			case vcsMoreChanges:
				m.drawVCSMoreItems(c, p)
				ui.Row(c).Gap(6).Children(func() {
					ui.Button(c, "Show Diff").Disabled(len(m.vcsMore.items) == 0).
						OnClick(func() { m.activateVCSMoreItem(m.vcsMore.choice) })
					if m.vcsMore.action == "commit.local" || m.vcsMore.action == "current.revision" {
						ui.Button(c, "Compare with Local").Disabled(len(m.vcsMore.items) == 0).
							OnClick(func() {
								item := m.vcsMore.items[m.vcsMore.choice]
								m.CompareCommitWithLocal(map[string]any{"id": item.Revision})
							})
					}
				})
			case vcsMoreConflicts:
				m.drawVCSMoreConflicts(c, p)
			case vcsMoreMessage:
				m.drawVCSMoreItems(c, p)
				ui.Row(c).Gap(6).Children(func() {
					label := "Use Message"
					if m.vcsMore.action == "current.revision" {
						label = "Compare with Local"
					}
					ui.Button(c, label).Disabled(len(m.vcsMore.items) == 0).OnClick(func() {
						m.activateVCSMoreItem(m.vcsMore.choice)
					})
				})
			case vcsMoreRebase:
				m.drawInteractiveRebase(c, p)
			}
			m.drawVCSMoreError(c, p)
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				primary := m.vcsMorePrimaryLabel()
				if m.vcsMore.kind == vcsMorePrompt || m.vcsMore.kind == vcsMoreConfirm ||
					m.vcsMore.kind == vcsMoreClone || m.vcsMore.kind == vcsMoreRebase {
					disabled := m.vcsMore.busy || m.vcsMore.loading
					if m.vcsMore.kind == vcsMoreRebase {
						disabled = disabled || m.interactiveRebaseProblem() != ""
					}
					ui.Button(c, primary).
						Disabled(disabled).
						OnClick(m.submitVCSMore)
				}
				closeLabel := "Close"
				if m.vcsMore.action == "clone" && m.vcsMore.busy {
					closeLabel = "Cancel Clone"
				}
				ui.Button(c, closeLabel).Disabled(m.vcsMore.busy && m.vcsMore.action != "clone").
					OnClick(m.cancelVCSMore)
			})
		})
	})
	if (dialog.Dismissed() || !open) && !m.vcsMore.busy {
		m.closeVCSMore()
	}
}
