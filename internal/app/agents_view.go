package app

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

const agentSendTimeout = 45 * time.Second
const agentImageCacheLimit = 24

func (m *Model) agentImageBitmap(image agentImage) *ui.Bitmap {
	key := agentImageCacheKey(image.MimeType, image.Data)
	if bitmap := m.agents.imageCache[key]; bitmap != nil {
		m.touchAgentImageCache(key)
		return bitmap
	}
	data, err := base64.StdEncoding.DecodeString(image.Data)
	if err != nil {
		return nil
	}
	bitmap, err := ui.DecodeBitmap(data)
	if err != nil {
		return nil
	}
	if m.agents.imageCache == nil {
		m.agents.imageCache = make(map[string]*ui.Bitmap)
	}
	m.agents.imageCache[key] = bitmap
	m.agents.imageOrder = append(m.agents.imageOrder, key)
	for len(m.agents.imageCache) > agentImageCacheLimit && len(m.agents.imageOrder) > 0 {
		oldest := m.agents.imageOrder[0]
		m.agents.imageOrder = m.agents.imageOrder[1:]
		delete(m.agents.imageCache, oldest)
	}
	return bitmap
}

func agentImageCacheKey(mimeType, data string) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(mimeType))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(data))
	return fmt.Sprintf("%x", hash.Sum(nil))
}

func (m *Model) touchAgentImageCache(key string) {
	for index, cached := range m.agents.imageOrder {
		if cached != key {
			continue
		}
		copy(m.agents.imageOrder[index:], m.agents.imageOrder[index+1:])
		m.agents.imageOrder[len(m.agents.imageOrder)-1] = key
		return
	}
	m.agents.imageOrder = append(m.agents.imageOrder, key)
}

func (m *Model) agentImageStrip(c *ui.Context, oi oiColors, images []agentImage,
	remove func(int)) {
	if len(images) == 0 {
		return
	}
	ui.Row(c).Wrap().Gap(5).Children(func() {
		for index, image := range images {
			index, image := index, image
			column := ui.Column(c.Key(fmt.Sprintf("agent-image:%d:%s", index, image.Data[:min(16, len(image.Data))]))).
				Gap(2).Padding(2).Radius(4).Background(oi.hoverTint)
			column.Children(func() {
				if bitmap := m.agentImageBitmap(image); bitmap != nil {
					ui.Image(c, bitmap).Size(96, 64)
				} else {
					ui.Text(c, "Image").Width(96).Height(64).FontSize(11).
						TextColor(oi.textMuted)
				}
				ui.Text(c, agentImageName(image, index)).Width(96).FontSize(9).
					TextColor(oi.textMuted).SingleLine().Ellipsis("…").
					Tooltip(agentImageName(image, index))
				if remove != nil {
					ui.Button(c, "Remove").OnClick(func() { remove(index) })
				}
			})
		}
	})
}

// agentsRightView is the right-anchored Agents tool window: the same grouped
// task list and task timeline OpenIDEA renders, drawn with MyGo controls.
func (m *Model) agentsRightView(c *ui.Context, p *ui.Theme, oi oiColors) {
	ui.Column(c).Grow(1).MinHeight(0).Background(oi.island).
		Radius(oiIslandRadius).Children(func() {
		if m.agentSessionLocation() == "toolWindow" && m.agents.conversation {
			m.agentsConversation(c, p, oi)
		} else {
			m.agentsTaskList(c, p, oi)
		}
	})
}

func (m *Model) agentsTaskList(c *ui.Context, p *ui.Theme, oi oiColors) {
	a := &m.agents
	ui.Row(c).Height(oiTokensFor(m, p).HeaderHeight).Shrink(0).Padding(0, 6, 0, 8).
		Gap(5).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "Agents").FontSize(13).TextColor(oi.textStrong).
			SingleLine().Ellipsis("…")
		if unread := agentUnreadCount(a); unread > 0 {
			label := fmt.Sprint(unread)
			if unread > 99 {
				label = "99+"
			}
			ui.Text(c, label).FontSize(10).Bold().TextColor(oi.textStrong).
				Padding(1, 5).Radius(8).Background(oi.hoverTint)
		}
		ui.Spacer(c)
		m.agentsOptionsIcon(c, oi)
		m.headerIcon(c, p, oi, "agents-new", "New Agent Task", "general/add", m.NewAgentTaskOpen)
		m.headerIcon(c, p, oi, "agents-sessions", "Agent sessions", "general/mainMenu",
			m.OpenAgentExternalSessions)
		m.headerIcon(c, p, oi, "agents-refresh", "Refresh Agents", "general/refresh", func() {
			m.connectAgents(true)
		})
		m.headerIcon(c, p, oi, "agents-hide", "Hide", "general/remove", func() {
			a.visible = false
			m.DockHide("agents")
		})
	})
	ui.Row(c).Height(34).Shrink(0).Padding(0, 8).Gap(6).AlignItems(ui.Center).Children(func() {
		in := ui.TextInput(c.Key("agents-search"), &a.filter).Grow(1).
			Placeholder("Find a task")
		if in.Changed() {
			m.invalidate()
		}
	})
	if a.actionErr != "" {
		ui.Row(c).Padding(4, 8).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.actionErr).Grow(1).FontSize(12).TextColor(p.Danger).
				SingleLine().Ellipsis("…").Tooltip(a.actionErr)
			ui.Button(c, "Dismiss").OnClick(func() {
				a.actionErr = ""
				m.invalidate()
			})
		})
	}
	if a.loading && !a.loaded {
		ui.Text(c, "Loading agents…").FontSize(12).TextColor(oi.textMuted).
			Padding(8, 10).SingleLine()
		return
	}
	if a.err != "" {
		ui.Column(c).Padding(8, 10).Gap(6).Children(func() {
			ui.Text(c, a.err).FontSize(12).TextColor(p.Danger)
			ui.Button(c, "Retry").OnClick(func() { m.connectAgents(true) })
		})
		return
	}
	groups := m.taskGroups()
	if len(groups) == 0 {
		ui.Column(c).Grow(1).Padding(12, 12).Gap(8).Children(func() {
			if len(a.tasks) == 0 {
				ui.Text(c, "No agent tasks").FontSize(12).TextColor(oi.textMuted).SingleLine()
				ui.Text(c, "Start a task to let a coding agent work in this project.").
					FontSize(12).TextColor(oi.textMuted)
			} else {
				ui.Text(c, "No tasks match the search").FontSize(12).
					TextColor(oi.textMuted).SingleLine()
			}
			ui.Button(c, "New Agent Task…").OnClick(m.NewAgentTaskOpen)
		})
		return
	}
	ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Padding(0, 4, 8, 4).Children(func() {
			for _, group := range groups {
				header := ui.Row(c.Key("agents-group:"+group.ID+":"+group.Status)).
					Height(26).Padding(2, 7).AlignItems(ui.Center).Radius(4)
				target := ""
				if group.Pinned {
					target = "pinned"
				} else if group.Custom {
					target = "group:" + group.ID
				}
				if target != "" {
					m.agentGroupDropTarget(header, target)
					if a.taskDropTarget == target {
						header.Background(oi.hoverTint)
					}
				}
				header.Children(func() {
					ui.Text(c, fmt.Sprintf("%s  %d", group.Label, len(group.Tasks))).
						FontSize(11).Bold().TextColor(oi.textMuted).SingleLine().Ellipsis("…")
				})
				if group.Custom {
					header.Menu(func(menu *ui.Menu) {
						if menu.Item("Rename Group…").Chosen() {
							m.openRenameAgentTaskGroup(group.ID)
						}
						if menu.Item("Remove Group").Chosen() {
							m.openRemoveAgentTaskGroup(group.ID)
						}
					})
				}
				for _, task := range group.Tasks {
					m.agentTaskRow(c, p, oi, task)
				}
			}
		})
	})
}

func agentUnreadCount(a *AgentsPane) int {
	count := 0
	for _, task := range a.tasks {
		if a.unread[task.ID] || task.Status == "needsInput" {
			count++
		}
	}
	return count
}

func (m *Model) agentsOptionsIcon(c *ui.Context, oi oiColors) {
	b := ui.ButtonBase(c.Key("panel:agents-options")).Size(20, 20).Radius(4).
		Justify(ui.Center).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if b.Hovered() {
		b.Background(oi.toolbarHover)
	}
	b.Tooltip("Options")
	b.Children(func() { oiIconEl(c, "general/mainMenu", 16, oi, oi.icon) })
	b.Menu(m.agentTaskHeaderMenu)
}

func (m *Model) agentTaskRow(c *ui.Context, p *ui.Theme, oi oiColors, task agentTask) {
	a := &m.agents
	row := ui.Row(c.Key("agents-task:"+task.ID)).MinHeight(46).Padding(4, 8).
		Gap(7).Radius(5).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	selected := slicesContains(a.selectedTasks, task.ID)
	if row.Hovered() || task.ID == a.selected || selected {
		row.Background(oi.hoverTint)
	}
	row.Children(func() {
		ui.Text(c, "●").FontSize(10).TextColor(agentStatusColor(p, task.Status))
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Row(c).MinWidth(0).Gap(5).AlignItems(ui.Center).Children(func() {
				ui.Text(c, task.Title).Grow(1).MinWidth(0).FontSize(13).
					TextColor(oi.text).SingleLine().Ellipsis("…")
				if a.unread[task.ID] && task.Status != "needsInput" {
					ui.Text(c, "●").FontSize(7).TextColor(oi.accent)
				}
				if when := agentUpdatedLabel(task.UpdatedMS); when != "" {
					ui.Text(c, when).FontSize(11).TextColor(oi.textSubtle).SingleLine()
				}
			})
			detail := agentStatusLabel(task.Status)
			if branch := agentBranchText(task.Branch); branch != "" {
				detail += " · " + branch
			}
			if task.Queue != nil && len(task.Queue) > 0 {
				detail += fmt.Sprintf(" · %d queued", len(task.Queue))
			}
			ui.Text(c, detail).FontSize(11).TextColor(oi.textMuted).
				SingleLine().Ellipsis("…")
		})
	})
	if row.Clicked() {
		mods := row.ClickModifiers()
		m.agentSelectTask(task.ID, mods)
		if mods&(ui.Cmd|ui.Ctrl|ui.Shift) == 0 && len(a.selectedTasks) == 1 {
			m.openAgentTask(task.ID)
		}
	}
	row.Menu(func(menu *ui.Menu) {
		m.prepareAgentTaskMenu(task.ID)
		m.agentTaskMenu(menu, task.ID)
	})
	m.agentTaskDrag(row, task.ID)
}

func agentStatusColor(p *ui.Theme, status string) ui.Color {
	switch status {
	case "needsInput":
		return p.Warning
	case "failed":
		return p.Danger
	case "limited":
		return p.Warning
	case "running", "starting":
		return p.Accent
	case "ready":
		return p.Success
	case "archived":
		return p.TextMuted
	default:
		return p.TextMuted
	}
}

func (m *Model) agentsConversation(c *ui.Context, p *ui.Theme, oi oiColors) {
	m.agentConversationView(c, p, oi, false)
}

func (m *Model) agentConversationView(c *ui.Context, p *ui.Theme, oi oiColors, editor bool) {
	a := &m.agents
	task, ok := a.tasks[a.selected]
	if !ok {
		a.conversation = false
		a.selected = ""
		m.invalidate()
		return
	}
	subagentID := a.currentSubagent()
	subagent := a.currentSubagentCard(task.ID)
	ui.Row(c).Height(oiTokensFor(m, p).HeaderHeight).Shrink(0).Padding(0, 5, 0, 8).
		Gap(4).AlignItems(ui.Center).Children(func() {
		if !editor {
			ui.ButtonBase(c.Key("agents-back")).Height(24).Padding(0, 6).Radius(4).
				Cursor(ui.CursorPointer).OnClick(func() {
				if subagentID != "" {
					path := append([]string(nil), a.selectedSubagents[:len(a.selectedSubagents)-1]...)
					m.navigateAgentSubagents(task.ID, path)
					return
				}
				m.closeAgentConversation()
			}).Children(func() {
				ui.Text(c, "‹").FontSize(18).TextColor(oi.textMuted).SingleLine()
			})
		}
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			title, status := task.Title, task.Status
			if subagent != nil {
				title, status = agentSubagentTitle(*subagent), subagent.Status
			}
			ui.Text(c, title).FontSize(13).TextColor(oi.textStrong).
				SingleLine().Ellipsis("…")
			statusLabel := agentStatusLabel(status)
			if subagent != nil {
				statusLabel = agentSubagentStatusLabel(status)
			}
			ui.Text(c, statusLabel+" · "+
				agentProviderName(a.providers, task.Provider)).FontSize(11).
				TextColor(agentStatusColor(p, status)).SingleLine().Ellipsis("…")
		})
		if subagent != nil && agentSubagentWorking(subagent.Status) && subagent.Cancellable {
			m.headerIcon(c, p, oi, "agents-stop-subagent", "Stop Subagent", "general/stop",
				func() { m.cancelAgentSubagent(task.ID, subagent.ID) })
		} else if subagent == nil && agentTaskBusy(task.Status) {
			m.headerIcon(c, p, oi, "agents-stop", "Stop", "general/stop",
				func() { m.cancelAgentTask(task.ID) })
		}
		m.headerIcon(c, p, oi, "agents-changes", "Review Changes", "general/diff", func() {
			m.loadAgentChanges(task.ID)
		})
		m.headerIcon(c, p, oi, "agents-conversation-sessions", "Agent sessions",
			"general/mainMenu", m.OpenAgentExternalSessions)
		if editor {
			m.headerIcon(c, p, oi, "agents-dock-task", "Open Agent Task in Tool Window",
				"general/move", func() { m.dockAgentTask(task.ID) })
		} else {
			m.headerIcon(c, p, oi, "agents-hide-conversation", "Hide", "general/remove", func() {
				a.visible = false
				m.DockHide("agents")
			})
		}
	})
	if subagentID != "" {
		m.agentSubagentBreadcrumb(c, p, oi, task, a.subagentPathCards(task.ID))
	}
	entries := a.entries[a.currentEntriesKey()]
	busy := agentTaskBusy(task.Status)
	if subagent != nil {
		busy = agentSubagentWorking(subagent.Status)
	}
	findBefore := a.findText
	expression := agentFindExpression(a.findText)
	matches := agentFindMatchCount(expression, entries)
	if ui.FindBar(c, &a.findOpen, &a.findText, matches, &a.findMatch).Changed() {
		if a.findText != findBefore {
			a.findMatch = 0
		}
		m.invalidate()
	}
	if !a.findOpen && a.findText != "" {
		a.findText = ""
		a.findMatch = 0
	}
	if a.conversationScroll.Y < a.conversationScroll.MaxY-24 {
		ui.Row(c).Height(24).Padding(0, 8).AlignItems(ui.Center).Children(func() {
			ui.Spacer(c)
			ui.Button(c, "Jump to Latest").OnClick(m.jumpAgentToLatest)
		})
	}
	ui.Scroll(c).Grow(1).MinHeight(0).TrackScroll(&a.conversationScroll).Children(func() {
		ui.Column(c).Gap(8).Padding(8, 8).Children(func() {
			m.agentTaskSummary(c, p, oi, task, entries)
			entriesKey := a.currentEntriesKey()
			if a.entriesBusy[entriesKey] && !a.entriesLoaded[entriesKey] {
				ui.Text(c, "Loading conversation…").FontSize(12).
					TextColor(oi.textMuted).SingleLine()
				return
			}
			if message := a.entriesErr[entriesKey]; message != "" {
				ui.Column(c).Gap(5).Children(func() {
					ui.Text(c, message).FontSize(12).TextColor(p.Danger)
					ui.Button(c, "Retry").OnClick(func() {
						m.loadAgentConversation(task.ID, a.currentSubagent(), true)
					})
				})
				return
			}
			if a.entriesLoaded[entriesKey] && len(entries) == 0 {
				ui.Text(c, "The conversation is empty").FontSize(12).
					TextColor(oi.textMuted).SingleLine()
			}
			matchOffset := 0
			prefs := m.agentPrefs()
			grouped := prefs.toolCalls() == "grouped" && expression == nil
			var folded map[int]int
			if !prefs.NoFoldTurns && expression == nil {
				folded = agentFoldedTurns(entries, busy)
			}
			for i := 0; i < len(entries); i++ {
				entry := entries[i]
				if grouped && agentProcessEntry(entry) {
					end := i + 1
					for end < len(entries) && agentProcessEntry(entries[end]) {
						end++
					}
					if end-i > 1 {
						m.agentProcessRun(c, p, oi, task, entries, i, end, expression)
						i = end - 1
						continue
					}
				}
				entryMatches := 0
				if expression != nil {
					entryMatches = len(expression.FindAllStringIndex(
						agentEntrySearchText(entry), -1))
				}
				currentEntry := expression != nil && a.findMatch >= matchOffset &&
					a.findMatch < matchOffset+entryMatches
				row := ui.Column(c.Key(fmt.Sprintf("agents-entry:%s:%d", task.ID, i))).
					Gap(2)
				if currentEntry {
					row.Background(oi.hoverTint).Radius(4).Padding(2, 3).
						ScrollIntoView()
				}
				row.Children(func() {
					m.agentTimelineEntry(c, p, oi, task, entries, i, entry, expression)
				})
				matchOffset += entryMatches
				if reply, ok := folded[i]; ok {
					foldKey := agentTimelineFoldKey(task.ID, a.currentSubagent(), "turn", i)
					open := a.timelineOpen[foldKey]
					m.agentTurnFold(c, oi, task, entries, i, reply)
					if !open {
						i = reply - 1
					}
				}
			}
			if a.actionErr != "" {
				ui.Text(c, a.actionErr).FontSize(12).TextColor(p.Danger)
			}
		})
	})
	if task.Status == "archived" {
		ui.Text(c, "This task is archived. Unarchive it from the task menu to continue.").
			Shrink(0).Padding(6, 8).FontSize(11).TextColor(oi.textMuted).
			SingleLine().Ellipsis("…")
		return
	}
	ui.Column(c).Shrink(0).Padding(6, 8).Gap(5).
		BorderWidth(1, 0, 0, 0).BorderColor(oi.border).Children(func() {
		area := ui.TextArea(c.Key("agents-composer"), &a.composer).Height(72).
			FontSize(12).Placeholder("Ask the agent to continue…").
			HandleInput(m.HandleAgentComposerInput)
		if dropped := area.DroppedFiles(); len(dropped) > 0 {
			m.addAgentDroppedPaths(false, dropped)
		}
		if area.FileDragOver() {
			area.BorderWidth(1, 1, 1, 1).BorderColor(oi.accent)
		}
		if a.completionCaretSet {
			caret := a.completionCaret
			area.SetTextSelection(caret, caret)
			a.completionCaretSet = false
		}
		_, caret := area.TextSelection()
		a.composerCaret = caret
		root := task.Cwd
		if root == "" {
			root = m.Root()
		}
		m.refreshTaskAgentCommands(task.ID, false)
		m.SyncAgentCompletion(task.ID, a.composer, caret, root, task.Commands)
		if message := a.commandsErr[task.ID]; message != "" {
			ui.Text(c, "Could not load slash commands: "+message).FontSize(10).
				TextColor(p.Danger).SingleLine().Ellipsis("…")
		}
		m.agentCompletionsView(c, p, oi)
		ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Attachments").FontSize(10).TextColor(oi.textSubtle)
			ui.Spacer(c)
			ui.Button(c, "Paste Image").Disabled(a.composerBusy).OnClick(
				m.AddAgentConversationImageFromClipboard)
			ui.Button(c, "Add Files…").Disabled(a.composerBusy).OnClick(func() {
				m.AddAgentConversationAttachments(false)
			})
			ui.Button(c, "Add Folder…").Disabled(a.composerBusy).OnClick(func() {
				m.AddAgentConversationAttachments(true)
			})
		})
		if len(a.composerFiles) > 0 {
			ui.Scroll(c).Height(70).Children(func() {
				ui.Column(c).Gap(2).Children(func() {
					for _, path := range a.composerFiles {
						path := path
						row := ui.Row(c.Key("agents-composer-file:"+path)).MinHeight(22).
							Padding(0, 4).Gap(5).AlignItems(ui.Center).Radius(4)
						if row.Hovered() {
							row.Background(oi.hoverTint)
						}
						row.Children(func() {
							ui.Text(c, "◌").FontSize(10).TextColor(oi.textMuted)
							ui.Text(c, m.AgentTaskFilePath(path)).Grow(1).MinWidth(0).
								FontSize(10).TextColor(oi.text).SingleLine().
								Ellipsis("…").Tooltip(path)
							ui.Button(c, "Remove").OnClick(func() {
								m.RemoveAgentConversationFile(path)
							})
						})
					}
				})
			})
		}
		m.agentImageStrip(c, oi, a.composerImages, m.RemoveAgentConversationImage)
		ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
			ui.Text(c, m.agentComposerHint(task)).Grow(1).FontSize(10).
				TextColor(oi.textSubtle).SingleLine().Ellipsis("…")
			if agentTaskBusy(task.Status) {
				ui.Button(c, "Stop").Disabled(a.taskBusy[task.ID]).OnClick(func() {
					m.cancelAgentTask(task.ID)
				})
			}
			label := "Send"
			if agentTaskBusy(task.Status) {
				label = "Queue"
			}
			ui.Button(c, label).Disabled((strings.TrimSpace(a.composer) == "" &&
				len(a.composerFiles) == 0 && len(a.composerImages) == 0) ||
				a.taskBusy[task.ID]).OnClick(m.submitAgentPrompt)
		})
	})
}

func (m *Model) submitAgentPromptNow() {
	a := &m.agents
	taskID := a.selected
	task, ok := a.tasks[taskID]
	text := strings.TrimSpace(a.composer)
	root := m.Root()
	promptRoot := root
	if task.Cwd != "" {
		promptRoot = task.Cwd
	}
	files := agentPromptFiles(text, a.composerFiles, promptRoot)
	if !ok || (text == "" && len(files) == 0 && len(a.composerImages) == 0) ||
		a.taskBusy[taskID] || task.Status == "archived" {
		return
	}
	gen := a.generation
	a.taskBusy[taskID] = true
	a.actionErr = ""
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, agentSendTimeout)
		defer cancel()
		_, err := m.invoke(ctx, "agent_prompt", map[string]any{
			"task":   taskID,
			"prompt": map[string]any{"text": text, "files": files, "images": a.composerImages},
			"now":    true,
		})
		m.post(func() {
			if a.generation != gen || m.Root() != root {
				return
			}
			a.taskBusy[taskID] = false
			if err != nil {
				a.actionErr = "Could not steer the agent: " + err.Error()
			} else {
				a.composer = ""
				a.composerFiles = nil
				a.composerImages = nil
				delete(a.composerDrafts, taskID)
			}
			m.invalidate()
		})
	}()
	m.invalidate()
}

func (m *Model) agentSubagentBreadcrumb(c *ui.Context, p *ui.Theme, oi oiColors,
	task agentTask, cards []agentSubagent) {
	ui.Row(c).Height(28).Shrink(0).Padding(0, 9).Gap(3).AlignItems(ui.Center).
		BorderWidth(1, 0, 1, 0).BorderColor(oi.border).Children(func() {
		ui.Button(c, task.Title).Disabled(len(cards) == 0).OnClick(func() {
			m.navigateAgentSubagents(task.ID, nil)
		})
		for index, card := range cards {
			index, card := index, card
			ui.Text(c, "›").FontSize(11).TextColor(oi.textMuted).SingleLine()
			title := agentSubagentTitle(card)
			if index == len(cards)-1 {
				ui.Text(c, title).FontSize(11).TextColor(oi.textStrong).
					SingleLine().Ellipsis("…")
				continue
			}
			ui.Button(c, title).OnClick(func() {
				path := append([]string(nil), m.agents.selectedSubagents[:index+1]...)
				m.navigateAgentSubagents(task.ID, path)
			})
		}
		ui.Spacer(c)
		if len(cards) > 0 {
			card := cards[len(cards)-1]
			ui.Text(c, agentSubagentStatusLabel(card.Status)).FontSize(10).
				TextColor(agentStatusColor(p, card.Status)).SingleLine()
		}
	})
}

func agentSubagentWorking(status string) bool {
	return status == "running" || status == "needsInput"
}

func agentUsageLabel(usage agentUsage) string {
	if usage.Size == 0 {
		return "Context 0%"
	}
	percent := float64(usage.Used) / float64(usage.Size) * 100
	return fmt.Sprintf("Context %s / %s (%.0f%%)",
		agentTokenCount(usage.Used), agentTokenCount(usage.Size), percent)
}

func agentTokenCount(tokens uint32) string {
	value := float64(tokens)
	switch {
	case value >= 1_000_000:
		return strings.TrimSuffix(strings.TrimSuffix(fmt.Sprintf("%.1f", value/1_000_000), "0"), ".") + "M"
	case value >= 1_000:
		precision := 1
		if value >= 100_000 {
			precision = 0
		}
		text := fmt.Sprintf("%.*f", precision, value/1_000)
		return strings.TrimSuffix(strings.TrimSuffix(text, "0"), ".") + "k"
	default:
		return fmt.Sprint(tokens)
	}
}

func agentCostLabel(cost agentCost) string {
	currency := strings.TrimSpace(cost.Currency)
	if currency == "" {
		return fmt.Sprintf("%.2f", cost.Amount)
	}
	switch currency {
	case "USD":
		return fmt.Sprintf("$%.2f", cost.Amount)
	case "EUR":
		return fmt.Sprintf("€%.2f", cost.Amount)
	case "GBP":
		return fmt.Sprintf("£%.2f", cost.Amount)
	case "CNY":
		return fmt.Sprintf("¥%.2f", cost.Amount)
	default:
		return fmt.Sprintf("%.2f %s", cost.Amount, currency)
	}
}

func agentLimitLabel(limit agentLimit, nowMS int64) string {
	window := strings.TrimSpace(limit.Window)
	switch window {
	case "five_hour":
		window = "5h"
	case "seven_day":
		window = "7d"
	case "seven_day_opus":
		window = "Opus 7d"
	case "":
		window = "Limit"
	}
	label := window
	if limit.UsedPercent != nil {
		label += fmt.Sprintf(" %.0f%%", *limit.UsedPercent)
	}
	if limit.ResetsMS != nil && *limit.ResetsMS > float64(nowMS) {
		label += " · resets in " + agentDurationLabel(*limit.ResetsMS-float64(nowMS))
	}
	return label
}

func agentWorkTime(entries []agentEntry, busy bool, nowMS int64) int64 {
	var users []agentEntry
	for _, entry := range entries {
		if entry.Type == "user" && !entry.Steered && entry.SentMS != nil {
			users = append(users, entry)
		}
	}
	var total int64
	for index, entry := range users {
		start := int64(*entry.SentMS)
		end := int64(0)
		if entry.DoneMS != nil {
			end = int64(*entry.DoneMS)
		} else if busy && index == len(users)-1 {
			end = nowMS
		}
		if end > start {
			total += end - start
		}
	}
	return total
}

func agentDurationLabel(ms float64) string {
	if ms < 1000 {
		return fmt.Sprintf("%.0fms", max(0, ms))
	}
	seconds := int64(ms / 1000)
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	minutes := seconds / 60
	if minutes < 60 {
		return fmt.Sprintf("%dm %02ds", minutes, seconds%60)
	}
	return fmt.Sprintf("%dh %02dm", minutes/60, minutes%60)
}

func agentTaskBusy(status string) bool {
	return status == "starting" || status == "running" || status == "needsInput"
}

func (m *Model) agentComposerHint(task agentTask) string {
	prefs := m.agentPrefs()
	send := "Enter"
	alternate := "⌘Enter"
	if prefs.sendWith() == "modEnter" {
		send, alternate = "⌘Enter", "⌘⇧Enter"
	}
	if !agentTaskBusy(task.Status) {
		return send + " sends"
	}
	steer := agentCanSteer(task, m.agents.composerImages)
	if prefs.whileRunning() == "steer" && steer {
		return send + " sends into the running turn · " + alternate + " queues"
	}
	return send + " queues for the next turn · " + alternate + " sends now"
}

func (m *Model) agentTaskSummary(c *ui.Context, p *ui.Theme, oi oiColors,
	task agentTask, entries []agentEntry) {
	prefs := m.agentPrefs()
	m.scheduleAgentMeterRefresh(task)
	ui.Column(c).Gap(4).Padding(2, 3).Children(func() {
		if task.Error != nil && *task.Error != "" {
			ui.Text(c, *task.Error).FontSize(12).TextColor(p.Danger)
		}
		detail := task.Isolation
		if task.Model != nil && *task.Model != "" {
			detail += " · " + *task.Model
		}
		if branch := agentBranchText(task.Branch); branch != "" {
			detail += " · " + branch
		}
		ui.Text(c, detail).FontSize(11).TextColor(oi.textMuted).
			SingleLine().Ellipsis("…")
		meters := make([]string, 0, 4)
		if !prefs.NoContextUsage && task.Usage != nil && task.Usage.Size > 0 {
			meters = append(meters, agentUsageLabel(*task.Usage))
		}
		if !prefs.NoUsageLimits {
			for _, limit := range m.agents.limits[task.Provider] {
				meters = append(meters, agentLimitLabel(limit, time.Now().UnixMilli()))
			}
		}
		if prefs.ShowCost && task.Usage != nil && task.Usage.Cost != nil {
			meters = append(meters, agentCostLabel(*task.Usage.Cost))
		}
		if !prefs.NoWorkTime {
			work := agentWorkTime(entries, agentTaskBusy(task.Status), time.Now().UnixMilli())
			if work >= 1000 {
				meters = append(meters, agentDurationLabel(float64(work)))
			}
		}
		if len(meters) > 0 {
			ui.Text(c, strings.Join(meters, " · ")).FontSize(10).
				TextColor(oi.textSubtle).SingleLine().Ellipsis("…")
		}
		if task.Retry != nil {
			label := fmt.Sprintf("Retrying request %d", task.Retry.Attempt)
			if task.Retry.MaxRetries != nil {
				label += fmt.Sprintf(" of %d", *task.Retry.MaxRetries)
			}
			ui.Text(c, label).FontSize(11).TextColor(p.Warning).SingleLine()
		}
		options := agentMainConfigOptions(task.Config)
		if len(options) > 0 {
			ui.Row(c).Wrap().Gap(5).Children(func() {
				for _, option := range options {
					option := option
					labels := make([]string, 0, len(option.Values))
					labelToValue := make(map[string]string, len(option.Values))
					selected := ""
					for _, value := range option.Values {
						label := value.Name
						if label == "" {
							label = value.Value
						}
						labels = append(labels, label)
						labelToValue[label] = value.Value
						if option.Current != nil && *option.Current == value.Value {
							selected = label
						}
					}
					if len(labels) == 0 {
						continue
					}
					if selected == "" {
						selected = labels[0]
					}
					choice := ui.Select(c, &selected, labels).
						Disabled(m.agents.taskBusy[task.ID] || task.Status == "archived")
					if choice.Changed() {
						m.setAgentConfig(task.ID, option.ID, labelToValue[selected])
					}
				}
			})
		}
		if len(task.Queue) > 0 {
			ui.Text(c, "Queued messages").FontSize(10).Bold().
				TextColor(oi.textMuted).SingleLine()
			for _, queued := range task.Queue {
				queued := queued
				row := ui.Row(c.Key("agents-queued:"+queued.ID)).MinHeight(26).
					Padding(1, 4).Gap(5).AlignItems(ui.Center).Radius(4)
				if row.Hovered() {
					row.Background(oi.hoverTint)
				}
				row.Children(func() {
					text := strings.TrimSpace(queued.Prompt.Text)
					if text == "" {
						text = "Image"
					}
					ui.Text(c, "Ⅱ").FontSize(10).TextColor(oi.textMuted)
					ui.Text(c, text).Grow(1).MinWidth(0).FontSize(11).
						TextColor(oi.text).SingleLine().Ellipsis("…").Tooltip(text)
					disabled := m.agents.taskBusy[task.ID] || task.Status == "archived"
					ui.Button(c, "Send now").Disabled(disabled).OnClick(func() {
						m.sendAgentQueuedNow(task.ID, queued.ID)
					})
					ui.Button(c, "Edit").Disabled(disabled).OnClick(func() {
						m.editAgentQueued(task.ID, queued.ID)
					})
					ui.Button(c, "Remove").Disabled(disabled).OnClick(func() {
						m.removeAgentQueued(task.ID, queued.ID)
					})
				})
			}
		}
		background := make([]agentBackground, 0, len(task.Background))
		for _, item := range task.Background {
			if item.Kind != "local_agent" {
				background = append(background, item)
			}
		}
		if len(background) > 0 {
			ui.Text(c, "Running in the background").FontSize(10).Bold().
				TextColor(oi.textMuted).SingleLine()
			for _, item := range background {
				item := item
				ui.Row(c.Key("agents-background:"+item.ID)).MinHeight(24).
					Padding(1, 4).Gap(5).AlignItems(ui.Center).Children(func() {
					label := item.Description
					if label == "" {
						label = item.Kind
					}
					ui.Text(c, "◷").FontSize(11).TextColor(oi.textMuted)
					ui.Text(c, label).Grow(1).MinWidth(0).FontSize(11).
						TextColor(oi.textSubtle).SingleLine().Ellipsis("…").Tooltip(label)
					if when := agentUpdatedLabel(item.StartedMS); when != "" {
						ui.Text(c, when).FontSize(10).TextColor(oi.textSubtle).SingleLine()
					}
					if task.Capabilities.BackgroundTasks {
						ui.Button(c, "Stop").Disabled(m.agents.taskBusy[task.ID]).
							OnClick(func() { m.stopAgentBackground(task.ID, item.ID) })
					}
				})
			}
		}
	})
}

// agentMainConfigOptions follows OpenIDEA's composer order: model, mode,
// collaboration, permissions, approval, thinking effort, then model options.
func agentMainConfigOptions(config []agentConfigOption) []agentConfigOption {
	order := []string{
		"model", "mode", "collaboration_mode", "permissions",
		"approval", "thought_level", "model_config",
	}
	out := make([]agentConfigOption, 0, len(order))
	for _, category := range order {
		found := false
		for _, option := range config {
			optionCategory := ""
			if option.Category != nil {
				optionCategory = *option.Category
			}
			matches := optionCategory == category ||
				(category == "model_config" && option.ID == "fast")
			if !matches || len(option.Values) == 0 {
				continue
			}
			out = append(out, option)
			if category != "model_config" {
				found = true
				break
			}
		}
		if found {
			continue
		}
	}
	return out
}

func agentTimelineFoldKey(taskID, subagent, kind string, index int) string {
	return fmt.Sprintf("%s\x00%s\x00%s:%d", taskID, subagent, kind, index)
}

func agentProcessEntry(entry agentEntry) bool {
	return entry.Type == "thought" || entry.Type == "tool"
}

type agentWorkSummary struct {
	Thoughts int
	Read     int
	Search   int
	Edited   int
	Run      int
	Fetch    int
	Other    int
	Added    uint32
	Removed  uint32
	Subagent int
}

func agentSummarizeWork(entries []agentEntry) agentWorkSummary {
	var summary agentWorkSummary
	edited := make(map[string]bool)
	for _, entry := range entries {
		switch entry.Type {
		case "thought":
			summary.Thoughts++
		case "subagent":
			summary.Subagent++
		case "tool":
			for _, diff := range entry.Diffs {
				edited[diff.Path] = true
				summary.Added += diff.Added
				summary.Removed += diff.Removed
			}
			switch entry.Kind {
			case "read":
				summary.Read++
			case "search":
				summary.Search++
			case "execute":
				summary.Run++
			case "fetch":
				summary.Fetch++
			case "edit", "delete", "move":
				if len(entry.Diffs) == 0 {
					for _, path := range entry.Locations {
						edited[path] = true
					}
				}
			case "think":
			default:
				summary.Other++
			}
		}
	}
	summary.Edited = len(edited)
	return summary
}

func agentWorkSummaryText(summary agentWorkSummary, toolCalls int) string {
	parts := make([]string, 0, 8)
	if summary.Edited > 0 {
		parts = append(parts, fmt.Sprintf("Edited %d %s", summary.Edited,
			pluralWord(summary.Edited, "file", "files")))
	}
	if summary.Run > 0 {
		parts = append(parts, fmt.Sprintf("Ran %d %s", summary.Run,
			pluralWord(summary.Run, "command", "commands")))
	}
	if summary.Read > 0 {
		parts = append(parts, fmt.Sprintf("Read %d %s", summary.Read,
			pluralWord(summary.Read, "file", "files")))
	}
	if summary.Search > 0 {
		parts = append(parts, fmt.Sprintf("Searched %d %s", summary.Search,
			pluralWord(summary.Search, "time", "times")))
	}
	if summary.Fetch > 0 {
		parts = append(parts, fmt.Sprintf("Fetched %d %s", summary.Fetch,
			pluralWord(summary.Fetch, "page", "pages")))
	}
	if summary.Subagent > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", summary.Subagent,
			pluralWord(summary.Subagent, "subagent", "subagents")))
	}
	if summary.Other > 0 {
		parts = append(parts, fmt.Sprintf("%d other %s", summary.Other,
			pluralWord(summary.Other, "tool call", "tool calls")))
	}
	if len(parts) == 0 && toolCalls > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", toolCalls,
			pluralWord(toolCalls, "tool call", "tool calls")))
	}
	if len(parts) == 0 && summary.Thoughts > 0 {
		parts = append(parts, "Reasoning")
	}
	return strings.Join(parts, " · ")
}

func pluralWord(count int, one, many string) string {
	if count == 1 {
		return one
	}
	return many
}

func agentFoldedTurns(entries []agentEntry, busy bool) map[int]int {
	folded := make(map[int]int)
	starts := make([]int, 0)
	for index, entry := range entries {
		if entry.Type == "user" && !entry.Steered {
			starts = append(starts, index)
		}
	}
	for position, user := range starts {
		end := len(entries)
		if position+1 < len(starts) {
			end = starts[position+1]
		}
		if position == len(starts)-1 && busy {
			continue
		}
		reply := -1
		blocked := false
		for index := end - 1; index > user; index-- {
			entry := entries[index]
			if entry.Type == "agent" && strings.TrimSpace(entry.Text) != "" {
				reply = index
				break
			}
			if entry.Type == "error" || entry.Type == "permission" && entry.Chosen == nil {
				blocked = true
				break
			}
		}
		if blocked || reply < 0 || reply == user+1 {
			continue
		}
		for _, entry := range entries[user+1 : reply] {
			if entry.Type == "user" || entry.Type == "error" {
				blocked = true
				break
			}
		}
		if !blocked {
			folded[user] = reply
		}
	}
	return folded
}

func (m *Model) agentProcessRun(c *ui.Context, p *ui.Theme, oi oiColors,
	task agentTask, entries []agentEntry, start, end int, find *regexp.Regexp) {
	key := agentTimelineFoldKey(task.ID, m.agents.currentSubagent(), "run", start)
	open := m.agents.timelineOpen[key]
	summary := agentSummarizeWork(entries[start:end])
	toolCalls := 0
	live := ""
	for _, entry := range entries[start:end] {
		if entry.Type != "tool" {
			continue
		}
		toolCalls++
		if (entry.Status == "running" || entry.Status == "pending") &&
			(entry.Title != "" || entry.Kind != "") {
			live = entry.Title
			if live == "" {
				live = entry.Kind
			}
		}
	}
	label := agentWorkSummaryText(summary, toolCalls)
	if label == "" {
		label = "Work"
	}
	if live != "" {
		label += " · " + live
	}
	ui.Column(c).Gap(3).Children(func() {
		ui.ButtonBase(c.Key("agents-process:"+key)).MinHeight(26).Padding(2, 5).
			Radius(4).Gap(5).AlignItems(ui.Center).Cursor(ui.CursorPointer).
			OnClick(func() {
				if m.agents.timelineOpen == nil {
					m.agents.timelineOpen = make(map[string]bool)
				}
				m.agents.timelineOpen[key] = !open
				m.invalidate()
			}).Children(func() {
			ui.Text(c, map[bool]string{true: "▾", false: "▸"}[open]).FontSize(10).
				TextColor(oi.textMuted)
			ui.Text(c, label).Grow(1).MinWidth(0).FontSize(11).
				TextColor(oi.textMuted).SingleLine().Ellipsis("…")
			if summary.Added > 0 {
				ui.Text(c, fmt.Sprintf("+%d", summary.Added)).FontSize(10).
					TextColor(p.Success)
			}
			if summary.Removed > 0 {
				ui.Text(c, fmt.Sprintf("−%d", summary.Removed)).FontSize(10).
					TextColor(p.Danger)
			}
		})
		if open {
			ui.Column(c).Gap(3).PaddingStart(8).Children(func() {
				for index := start; index < end; index++ {
					m.agentTimelineEntry(c, p, oi, task, entries, index, entries[index], find)
				}
			})
		}
	})
}

func (m *Model) agentTurnFold(c *ui.Context, oi oiColors, task agentTask,
	entries []agentEntry, user, reply int) {
	key := agentTimelineFoldKey(task.ID, m.agents.currentSubagent(), "turn", user)
	work := agentSummarizeWork(entries[user+1 : reply])
	label := agentWorkSummaryText(work, 0)
	messages := 0
	for _, entry := range entries[user+1 : reply] {
		if entry.Type == "agent" && strings.TrimSpace(entry.Text) != "" {
			messages++
		}
	}
	if messages > 0 {
		if label != "" {
			label += " · "
		}
		label += fmt.Sprintf("%d %s", messages, pluralWord(messages, "message", "messages"))
	}
	if label == "" {
		label = "Work"
	}
	request := entries[user]
	if request.SentMS != nil && request.DoneMS != nil && *request.DoneMS > *request.SentMS {
		label += " · " + agentDurationLabel(*request.DoneMS-*request.SentMS)
	}
	ui.ButtonBase(c.Key("agents-turn-fold:"+key)).MinHeight(26).Padding(2, 5).
		Radius(4).Gap(5).AlignItems(ui.Center).Cursor(ui.CursorPointer).
		Background(oi.hoverTint).OnClick(func() {
		if m.agents.timelineOpen == nil {
			m.agents.timelineOpen = make(map[string]bool)
		}
		m.agents.timelineOpen[key] = !m.agents.timelineOpen[key]
		m.invalidate()
	}).Children(func() {
		chevron := "▸"
		if m.agents.timelineOpen[key] {
			chevron = "▾"
		}
		ui.Text(c, chevron).FontSize(10).TextColor(oi.textMuted)
		ui.Text(c, label).Grow(1).MinWidth(0).FontSize(11).
			TextColor(oi.textMuted).SingleLine().Ellipsis("…")
		if work.Added > 0 {
			ui.Text(c, fmt.Sprintf("+%d", work.Added)).FontSize(10).TextColor(oi.accent)
		}
		if work.Removed > 0 {
			ui.Text(c, fmt.Sprintf("−%d", work.Removed)).FontSize(10).TextColor(oi.textMuted)
		}
	})
}

func (m *Model) agentTimelineEntry(c *ui.Context, p *ui.Theme, oi oiColors,
	task agentTask, entries []agentEntry, index int, entry agentEntry,
	find *regexp.Regexp) {
	if entry.Type == "user" && m.agents.editOpen &&
		m.agents.editTask == task.ID && m.agents.editIndex == index {
		m.agentMessageEditor(c, p, oi, task, index)
		return
	}
	switch entry.Type {
	case "permission":
		m.agentPermissionEntry(c, p, oi, task, entry, find)
	case "tool":
		m.agentToolEntry(c, p, oi, task, entries, index, entry, find)
	case "thought":
		m.agentThoughtEntry(c, p, oi, task, entries, index, entry, find)
	case "subagent":
		m.agentSubagentCard(c, p, oi, task, entry.Subagent, find)
	case "plan":
		ui.Column(c).Gap(2).Children(func() {
			ui.Text(c, "Plan").FontSize(11).Bold().TextColor(oi.textMuted).SingleLine()
			for _, item := range entry.Items {
				mark := "○"
				switch item.Status {
				case "completed":
					mark = "✓"
				case "in_progress":
					mark = "◐"
				}
				agentSearchTextBlock(c, p, oi, mark+" "+item.Content, oi.text, 8, find)
			}
		})
	case "error":
		agentSearchTextBlock(c, p, oi, entry.Message, p.Danger, 0, find)
	case "message":
		label := agentEntryLabel(entry)
		ui.Column(c).Gap(2).Padding(3, 5).Radius(4).
			Background(oi.hoverTint).Children(func() {
			agentFindRichText(c, label, find, p).FontSize(10).Bold().
				TextColor(oi.textMuted).SingleLine()
			agentSearchTextBlock(c, p, oi, entry.Text, oi.text, 0, find)
		})
	default:
		titleColor := oi.textMuted
		textColor := oi.text
		if entry.Type == "user" {
			titleColor = oi.accent
		} else if entry.Type == "thought" {
			textColor = oi.textMuted
		}
		ui.Column(c).Gap(2).Children(func() {
			label := agentEntryLabel(entry)
			if entry.Steered {
				label += " · steered"
			}
			agentFindRichText(c, label, find, p).FontSize(11).Bold().
				TextColor(titleColor).SingleLine().Ellipsis("…")
			if entry.Type == "agent" {
				working := m.agentTimelineEntryWorking(task, entries, index)
				shown := m.agentRevealedText(
					agentTimelineFoldKey(task.ID, m.agents.currentSubagent(), "reply", index),
					entry.Text, working)
				m.agentMarkdown(c, p, oi, shown, task.Cwd, find)
			} else {
				agentSearchTextBlock(c, p, oi, entry.Text, textColor, 0, find)
			}
			if entry.Type == "user" && len(entry.Images) > 0 {
				m.agentImageStrip(c, oi, entry.Images, nil)
			}
			if len(entry.Attachments) > 0 {
				ui.Text(c, strings.Join(entry.Attachments, ", ")).FontSize(10).
					TextColor(oi.textSubtle).SingleLine().Ellipsis("…")
			}
			if entry.Type == "user" && !entry.Steered {
				m.agentUserMessageActions(c, p, oi, task, entries, index, entry)
			}
			if entry.Type == "agent" && strings.TrimSpace(entry.Text) != "" {
				m.agentReplyActions(c, p, oi, task, entries, index, entry)
			}
		})
	}
}

func (m *Model) agentMessageEditor(c *ui.Context, p *ui.Theme, oi oiColors,
	task agentTask, index int) {
	a := &m.agents
	ui.Column(c).Gap(5).Padding(5, 6).Radius(5).
		BorderWidth(1, 1, 1, 1).BorderColor(p.Accent).Children(func() {
		ui.Text(c, "Edit message").FontSize(11).Bold().TextColor(oi.textStrong).
			SingleLine()
		area := ui.TextArea(c.Key(fmt.Sprintf("agents-edit:%s:%d", task.ID, index)),
			&a.editText).Height(104).Disabled(a.editBusy)
		if dropped := area.DroppedFiles(); len(dropped) > 0 {
			m.addAgentEditDroppedPaths(dropped)
		}
		m.agentImageStrip(c, oi, a.editImages, m.RemoveAgentEditImage)
		if a.editErr != "" {
			ui.Text(c, a.editErr).FontSize(11).TextColor(p.Danger)
		}
		ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "The messages after this one are replaced. Files the agent changed stay as they are.").
				Grow(1).MinWidth(0).FontSize(10).TextColor(oi.textMuted).
				SingleLine().Ellipsis("…")
			ui.Button(c, "Cancel").Disabled(a.editBusy).
				OnClick(m.cancelAgentMessageEditor)
			ui.Button(c, "Send").Disabled(a.editBusy ||
				(strings.TrimSpace(a.editText) == "" && len(a.editImages) == 0)).
				OnClick(m.submitAgentMessageEditor)
		})
	})
}

func (m *Model) agentUserMessageActions(c *ui.Context, p *ui.Theme, oi oiColors,
	task agentTask, entries []agentEntry, index int, entry agentEntry) {
	turn, ok := agentTurnAt(entries, index)
	if !ok {
		return
	}
	turns := agentTurnPositions(entries)
	idle := task.Status != "archived" && !agentTaskBusy(task.Status)
	inPlace := idle && task.Capabilities.Rollback
	latest := turn.Number == uint32(len(turns)-1)
	ui.Row(c).Wrap().Gap(4).AlignItems(ui.Center).Children(func() {
		if entry.Text != "" {
			ui.Button(c, "Copy").OnClick(func() { c.WriteClipboard(entry.Text) })
		}
		if inPlace && entry.Text != "" {
			ui.Button(c, "Edit").OnClick(func() {
				m.openAgentMessageEditor(task.ID, entries, index)
			})
		}
		if inPlace {
			ui.Button(c, "Retract").OnClick(func() {
				m.retractAgentTurn(task.ID, entries, index)
			})
		}
		if idle && entry.Text != "" && len(entry.Attachments) == 0 {
			label := "Send Again"
			if inPlace {
				label = "Run Again"
			}
			ui.Button(c, label).OnClick(func() {
				if inPlace {
					m.regenerateAgentTurn(task.ID, entries, index)
				} else {
					m.sendAgentMessageAgain(task.ID, entry)
				}
			})
		}
		if idle && task.Capabilities.Fork && (task.Capabilities.Rollback || latest) {
			ui.Button(c, "Fork").OnClick(func() {
				turnsToKeep := turn.Number + 1
				m.forkAgentTaskFrom(task.ID, &turnsToKeep)
			})
		}
	})
}

func (m *Model) agentReplyActions(c *ui.Context, p *ui.Theme, oi oiColors,
	task agentTask, entries []agentEntry, index int, entry agentEntry) {
	turns := agentTurnPositions(entries)
	turn, ok := agentTurnForReply(entries, index)
	if !ok || len(turns) == 0 {
		return
	}
	idle := task.Status != "archived" && !agentTaskBusy(task.Status)
	latest := turn.Number == uint32(len(turns)-1)
	prompt := entries[turn.PromptIndex]
	ui.Row(c).Wrap().Gap(4).AlignItems(ui.Center).Children(func() {
		ui.Button(c, "Copy Markdown").OnClick(func() {
			c.WriteClipboard(entry.Text)
		})
		if entry.Text != "" {
			ui.Button(c, "Quote").OnClick(func() {
				m.quoteAgentText(entry.Text)
			})
		}
		if idle && latest && task.Capabilities.Rollback &&
			strings.TrimSpace(prompt.Text) != "" {
			ui.Button(c, "Regenerate").OnClick(func() {
				m.regenerateAgentTurn(task.ID, entries, turn.PromptIndex)
			})
		}
		if idle && task.Capabilities.Fork && (task.Capabilities.Rollback || latest) {
			ui.Button(c, "Fork from here").OnClick(func() {
				turnsToKeep := turn.Number + 1
				m.forkAgentTaskFrom(task.ID, &turnsToKeep)
			})
		}
	})
}

func (m *Model) agentSubagentCard(c *ui.Context, p *ui.Theme, oi oiColors,
	task agentTask, subagent agentSubagent, find *regexp.Regexp) {
	ui.Column(c).Gap(4).Padding(6, 7).Radius(5).
		Background(oi.hoverTint).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "◆").FontSize(10).TextColor(agentStatusColor(p, subagent.Status))
			agentFindRichText(c, agentSubagentTitle(subagent), find, p).Grow(1).
				MinWidth(0).FontSize(12).Bold().TextColor(oi.textStrong).
				SingleLine().Ellipsis("…")
			ui.Text(c, agentSubagentStatusLabel(subagent.Status)).FontSize(10).
				TextColor(agentStatusColor(p, subagent.Status)).SingleLine()
			ui.Button(c, "Open").OnClick(func() {
				m.openAgentSubagent(task.ID, subagent.ID)
			})
		})
		if description := subagent.DescriptionValue(); description != "" &&
			description != agentSubagentTitle(subagent) {
			agentFindRichText(c, description, find, p).FontSize(11).
				TextColor(oi.textMuted).SingleLine().Ellipsis("…")
		}
		meta := fmt.Sprintf("%d tool calls", subagent.Tools)
		if activity := subagent.ActivityValue(); activity != "" &&
			agentSubagentWorking(subagent.Status) {
			meta += " · " + activity
		}
		ui.Text(c, meta).FontSize(10).TextColor(oi.textSubtle).
			SingleLine().Ellipsis("…").Tooltip(meta)
		if prompt := subagent.PromptValue(); prompt != "" {
			ui.Text(c, "Asked · "+prompt).FontSize(10).TextColor(oi.textMuted).
				SingleLine().Ellipsis("…").Tooltip(prompt)
		}
		if reply := subagent.ReplyValue(); reply != "" {
			ui.Text(c, "Answered · "+reply).FontSize(10).TextColor(oi.text).
				SingleLine().Ellipsis("…").Tooltip(reply)
		}
		if message := subagent.ErrorValue(); message != "" {
			ui.Text(c, message).FontSize(11).TextColor(p.Danger)
		}
	})
}

func (m *Model) agentPermissionEntry(c *ui.Context, p *ui.Theme, oi oiColors,
	task agentTask, entry agentEntry, find *regexp.Regexp) {
	ui.Column(c).Gap(5).Padding(6, 7).Radius(5).
		BorderWidth(1, 1, 1, 1).BorderColor(p.Warning).Children(func() {
		agentFindRichText(c, entry.Title, find, p).FontSize(12).Bold().
			TextColor(oi.text)
		if entry.Chosen != nil {
			ui.Text(c, "Answered: "+*entry.Chosen).FontSize(11).
				TextColor(oi.textMuted).SingleLine().Ellipsis("…")
			return
		}
		for _, question := range entry.Questions {
			label := question.Question
			if question.Header != "" {
				label = question.Header + " · " + label
			}
			agentSearchTextBlock(c, p, oi, label, oi.text, 0, find)
		}
		permission := agentPermissionNumber(entry)
		ui.Row(c).Wrap().Gap(5).Children(func() {
			for _, option := range entry.Options {
				option := option
				ui.Button(c, option.Name).OnClick(func() {
					m.respondAgentPermission(task.ID, permission, option.ID)
				})
			}
			if len(entry.Options) == 0 {
				ui.Button(c, "Cancel Request").OnClick(func() {
					m.respondAgentPermission(task.ID, permission, "")
				})
			}
		})
	})
}

func agentFindRichText(c *ui.Context, text string, find *regexp.Regexp, p *ui.Theme) ui.Element {
	if find == nil {
		return ui.RichText(c, ui.Span{Text: text})
	}
	return ui.RichText(c, agentFindSpans(text, find, p)...)
}

func agentSearchTextBlock(c *ui.Context, p *ui.Theme, oi oiColors, text string,
	color ui.Color, indent float32, find *regexp.Regexp) {
	if find == nil {
		agentTextBlock(c, oi, text, color, indent)
		return
	}
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return
	}
	for _, line := range strings.Split(text, "\n") {
		ui.RichText(c, agentFindSpans(line, find, p)...).FontSize(12).
			TextColor(color).PaddingStart(indent).SingleLine().Ellipsis("…")
	}
}

func agentFindSpans(text string, find *regexp.Regexp, p *ui.Theme) []ui.Span {
	ranges := find.FindAllStringIndex(text, -1)
	if len(ranges) == 0 {
		return []ui.Span{{Text: text}}
	}
	spans := make([]ui.Span, 0, len(ranges)*2+1)
	start := 0
	for _, match := range ranges {
		if match[0] > start {
			spans = append(spans, ui.Span{Text: text[start:match[0]]})
		}
		spans = append(spans, ui.Span{Text: text[match[0]:match[1]], Background: p.Selection})
		start = match[1]
	}
	if start < len(text) {
		spans = append(spans, ui.Span{Text: text[start:]})
	}
	return spans
}

func agentTextBlock(c *ui.Context, oi oiColors, text string, color ui.Color, indent float32) {
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return
	}
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		ui.Text(c, line).FontSize(12).TextColor(color).
			PaddingStart(indent).SingleLine().Ellipsis("…")
	}
}

func (m *Model) agentsNewTaskDialog(c *ui.Context, p *ui.Theme) {
	a := &m.agents
	if !a.newOpen {
		return
	}
	open := a.newOpen
	dialog := ui.Modal(c.Key("agents-new-dialog"), &open, func() {
		ui.Column(c).Width(560).Gap(10).Children(func() {
			ui.Text(c, "New Agent Task").FontSize(15).Bold().TextColor(p.Text)
			ui.Text(c, "Provider").FontSize(12).TextColor(p.TextMuted)
			if a.providersLoading && len(a.providers) == 0 {
				ui.Text(c, "Loading agents…").FontSize(12).TextColor(p.TextMuted)
			} else if a.providersErr != "" {
				ui.Row(c).Gap(6).Children(func() {
					ui.Text(c, a.providersErr).Grow(1).FontSize(12).TextColor(p.Danger)
					ui.Button(c, "Retry").OnClick(func() { m.refreshAgentProviders(true) })
				})
			} else if len(a.providers) == 0 {
				ui.Text(c, "No agent providers are configured").FontSize(12).
					TextColor(p.TextMuted)
			} else {
				labels := make([]string, 0, len(a.providers))
				labelToID := make(map[string]string, len(a.providers))
				selected := ""
				for _, provider := range a.providers {
					label := provider.Name
					if !provider.Installed {
						label += " (not installed)"
					}
					labels = append(labels, label)
					labelToID[label] = provider.ID
					if provider.ID == a.newProvider {
						selected = label
					}
				}
				if selected == "" {
					selected = labels[0]
				}
				if choice := ui.Select(c, &selected, labels).Grow(1); choice.Changed() {
					a.newProvider = labelToID[selected]
					a.newConfig = nil
					m.prepareNewAgentConfig()
					m.resetNewAgentCommands()
				}
			}
			m.prepareNewAgentConfig()
			for _, option := range m.newAgentConfigOptions() {
				option := option
				if len(option.Values) == 0 {
					continue
				}
				labels := make([]string, 0, len(option.Values))
				values := make(map[string]string, len(option.Values))
				current := ""
				for _, value := range option.Values {
					label := value.Name
					if label == "" {
						label = value.Value
					}
					labels = append(labels, label)
					values[label] = value.Value
					if a.newConfig[option.ID] == value.Value {
						current = label
					}
				}
				if current == "" {
					current = labels[0]
				}
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, option.Name).Width(120).FontSize(12).
						TextColor(p.TextMuted).SingleLine().Ellipsis("…")
					if choice := ui.Select(c, &current, labels).Grow(1); choice.Changed() {
						a.newConfig[option.ID] = values[current]
						m.prepareNewAgentConfig()
					}
				})
			}
			m.RefreshNewAgentCommands(false)
			if a.newCommandsErr != "" {
				ui.Text(c, a.newCommandsErr).FontSize(11).TextColor(p.Danger).
					SingleLine().Ellipsis("…")
			}
			ui.Text(c, "Where to work").FontSize(12).TextColor(p.TextMuted)
			locations := []string{"Local checkout", "New worktree"}
			shown := "Local checkout"
			if a.newIsolation == "worktree" {
				shown = "New worktree"
			}
			if choice := ui.Select(c, &shown, locations).Grow(1); choice.Changed() {
				if shown == "Local checkout" {
					a.newIsolation = "local"
				} else {
					a.newIsolation = "worktree"
				}
			}
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Attachments").FontSize(12).TextColor(p.TextMuted)
				ui.Spacer(c)
				ui.Button(c, "Paste Image").Disabled(a.newFileBusy).OnClick(
					m.AddAgentTaskImageFromClipboard)
				ui.Button(c, "Add Files…").Disabled(a.newFileBusy).OnClick(func() {
					m.AddAgentTaskAttachments(false)
				})
				ui.Button(c, "Add Folder…").Disabled(a.newFileBusy).OnClick(func() {
					m.AddAgentTaskAttachments(true)
				})
			})
			if len(a.newFiles) > 0 {
				ui.Scroll(c).Height(96).Children(func() {
					ui.Column(c).Gap(2).Children(func() {
						for _, path := range a.newFiles {
							path := path
							row := ui.Row(c.Key("agents-new-file:"+path)).MinHeight(24).
								Padding(0, 5).Gap(6).AlignItems(ui.Center).Radius(4)
							if row.Hovered() {
								row.Background(p.SurfaceHover)
							}
							row.Children(func() {
								ui.Text(c, "◌").FontSize(11).TextColor(p.TextMuted)
								ui.Text(c, m.AgentTaskFilePath(path)).Grow(1).MinWidth(0).
									FontSize(11).TextColor(p.Text).SingleLine().
									Ellipsis("…").Tooltip(path)
								ui.Button(c, "Remove").OnClick(func() {
									m.RemoveAgentTaskFile(path)
								})
							})
						}
					})
				})
			}
			m.agentImageStrip(c, oiPaletteOf(p), a.newImages, m.RemoveAgentTaskImage)
			ui.Text(c, "Task").FontSize(12).TextColor(p.TextMuted)
			area := ui.TextArea(c.Key("agents-new-prompt"), &a.newPrompt).
				Height(170).AutoFocus().HandleInput(m.HandleAgentComposerInput)
			if dropped := area.DroppedFiles(); len(dropped) > 0 {
				m.addAgentDroppedPaths(true, dropped)
			}
			if area.FileDragOver() {
				area.BorderWidth(1, 1, 1, 1).BorderColor(p.Accent)
			}
			if a.completionCaretSet {
				caret := a.completionCaret
				area.SetTextSelection(caret, caret)
				a.completionCaretSet = false
			}
			_, caret := area.TextSelection()
			m.SyncAgentCompletion("new", a.newPrompt, caret, m.Root(), a.newCommands)
			m.agentCompletionsView(c, p, oiPaletteOf(p))
			if a.newErr != "" {
				ui.Text(c, a.newErr).FontSize(12).TextColor(p.Danger)
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Button(c, "Create").Disabled(a.newBusy || a.newProvider == "" ||
					(strings.TrimSpace(a.newPrompt) == "" && len(a.newFiles) == 0 &&
						len(a.newImages) == 0)).
					OnClick(m.createAgentTask)
				ui.Button(c, "Cancel").OnClick(func() {
					a.newOpen = false
					a.newErr = ""
				})
			})
		})
	})
	if dialog.Dismissed() || !open {
		a.newOpen = false
	}
}

func (m *Model) agentsExternalSessionsDialog(c *ui.Context, p *ui.Theme) {
	a := &m.agents
	if !a.externalOpen {
		return
	}
	open := a.externalOpen
	dialog := ui.Modal(c.Key("agents-external-dialog"), &open, func() {
		ui.Column(c).Width(660).Height(480).Gap(8).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Children(func() {
					ui.Text(c, "Agent sessions").FontSize(15).Bold().TextColor(p.Text)
					ui.Text(c, "Sessions started in a terminal or another editor").
						FontSize(11).TextColor(p.TextMuted).SingleLine().Ellipsis("…")
				})
				ui.Button(c, "Refresh").Disabled(a.externalLoading).OnClick(func() {
					m.loadAgentExternalSessions()
				})
				ui.Button(c, "Close").OnClick(func() { a.externalOpen = false })
			})
			if a.externalErr != "" {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, a.externalErr).Grow(1).FontSize(12).TextColor(p.Danger)
					ui.Button(c, "Retry").OnClick(func() { m.loadAgentExternalSessions() })
				})
			}
			ui.Column(c).Grow(1).MinHeight(0).Children(func() {
				if a.externalLoading && !a.externalLoaded {
					ui.Text(c, "Loading Agent sessions…").FontSize(12).
						TextColor(p.TextMuted).SingleLine()
					return
				}
				if len(a.external) == 0 {
					ui.Text(c, "No sessions from other apps").FontSize(12).
						TextColor(p.TextMuted).SingleLine()
					return
				}
				ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
					ui.Column(c).Gap(2).Children(func() {
						for _, session := range a.external {
							session := session
							key := session.Provider + "\x00" + session.SessionID
							row := ui.Row(c.Key("agents-external:"+key)).MinHeight(42).
								Padding(3, 6).Gap(8).AlignItems(ui.Center).Radius(4)
							if row.Hovered() {
								row.Background(p.SurfaceHover)
							}
							row.Children(func() {
								ui.Text(c, "●").FontSize(10).TextColor(p.Accent)
								ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
									ui.Text(c, session.Title).FontSize(12).
										TextColor(p.Text).SingleLine().Ellipsis("…").
										Tooltip(session.Title)
									detail := agentProviderName(a.providers, session.Provider)
									if session.Cwd != nil && *session.Cwd != "" {
										detail += " · " + *session.Cwd
									}
									ui.Text(c, detail).FontSize(10).TextColor(p.TextMuted).
										SingleLine().Ellipsis("…").Tooltip(detail)
								})
								if session.UpdatedMS != nil {
									if when := agentUpdatedLabel(*session.UpdatedMS); when != "" {
										ui.Text(c, when).FontSize(10).TextColor(p.TextMuted).
											SingleLine()
									}
								}
								label := "Import"
								if a.externalBusy == key {
									label = "Importing…"
								}
								ui.Button(c, label).Disabled(a.externalBusy != "").
									OnClick(func() { m.importAgentExternalSession(session) })
							})
						}
					})
				})
			})
		})
	})
	if dialog.Dismissed() || !open {
		a.externalOpen = false
	}
}

func (m *Model) agentsRenameDialog(c *ui.Context, p *ui.Theme) {
	a := &m.agents
	if !a.renameOpen {
		return
	}
	open := a.renameOpen
	dialog := ui.Modal(c.Key("agents-rename-dialog"), &open, func() {
		ui.Column(c).Width(420).Gap(9).Children(func() {
			ui.Text(c, "Rename Agent Task").FontSize(14).Bold().TextColor(p.Text)
			in := ui.TextInput(c.Key("agents-rename-title"), &a.renameTitle).AutoFocus()
			if in.Submitted() {
				m.submitAgentRename()
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Button(c, "Rename").Disabled(strings.TrimSpace(a.renameTitle) == "").
					OnClick(m.submitAgentRename)
				ui.Button(c, "Cancel").OnClick(func() {
					a.renameOpen = false
				})
			})
		})
	})
	if dialog.Dismissed() || !open {
		a.renameOpen = false
	}
}

func (m *Model) agentsGroupDialog(c *ui.Context, p *ui.Theme) {
	a := &m.agents
	if !a.groupOpen {
		return
	}
	open := a.groupOpen
	title, action := "New Group", "Create"
	if a.groupMode == "rename" {
		title, action = "Rename Group", "Rename"
	}
	dialog := ui.Modal(c.Key("agents-group-dialog"), &open, func() {
		ui.Column(c).Width(420).Gap(9).Children(func() {
			ui.Text(c, title).FontSize(14).Bold().TextColor(p.Text)
			ui.Text(c, "Group name:").FontSize(12).TextColor(p.TextMuted)
			in := ui.TextInput(c.Key("agents-group-name"), &a.groupName).AutoFocus()
			if in.Submitted() {
				m.submitAgentTaskGroup()
			}
			if a.groupErr != "" {
				ui.Text(c, a.groupErr).FontSize(12).TextColor(p.Danger)
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Button(c, action).Disabled(strings.TrimSpace(a.groupName) == "").
					OnClick(m.submitAgentTaskGroup)
				ui.Button(c, "Cancel").OnClick(func() {
					a.groupOpen = false
					a.groupErr = ""
				})
			})
		})
	})
	if dialog.Dismissed() || !open {
		a.groupOpen = false
		a.groupErr = ""
	}
}

func (m *Model) agentsRemoveGroupDialog(c *ui.Context, p *ui.Theme) {
	a := &m.agents
	if !a.removeGroupOpen {
		return
	}
	groupName := ""
	for _, group := range m.agentOrganize().Groups {
		if group.ID == a.removeGroupID {
			groupName = group.Name
			break
		}
	}
	if groupName == "" {
		a.removeGroupOpen = false
		a.removeGroupID = ""
		return
	}
	open := a.removeGroupOpen
	dialog := ui.Modal(c.Key("agents-remove-group-dialog"), &open, func() {
		ui.Column(c).Width(460).Gap(9).Children(func() {
			ui.Text(c, "Remove Group").FontSize(14).Bold().TextColor(p.Text)
			ui.Text(c, fmt.Sprintf("Remove “%s”? Its tasks move back to their status groups.",
				groupName)).FontSize(12).TextColor(p.Text)
			ui.Row(c).Gap(8).Children(func() {
				ui.Button(c, "Remove").OnClick(m.confirmRemoveAgentTaskGroup)
				ui.Button(c, "Cancel").OnClick(func() {
					a.removeGroupOpen = false
					a.removeGroupID = ""
				})
			})
		})
	})
	if dialog.Dismissed() || !open {
		a.removeGroupOpen = false
		a.removeGroupID = ""
	}
}

func (m *Model) agentsDeleteDialog(c *ui.Context, p *ui.Theme) {
	a := &m.agents
	if !a.deleteOpen {
		return
	}
	taskIDs := uniqueAgentIDs(a.deleteTasks)
	if len(taskIDs) == 0 && a.deleteTask != "" {
		taskIDs = []string{a.deleteTask}
	}
	multiple := len(taskIDs) > 1
	open := a.deleteOpen
	dialog := ui.Modal(c.Key("agents-delete-dialog"), &open, func() {
		ui.Column(c).Width(460).Gap(9).Children(func() {
			title := "Delete Agent Task?"
			if multiple {
				title = "Delete Agent Tasks?"
			}
			ui.Text(c, title).FontSize(14).Bold().TextColor(p.Text)
			message := "The task and its conversation will be deleted. This cannot be undone."
			if multiple {
				message = fmt.Sprintf("%d tasks and their conversations will be deleted. This cannot be undone.",
					len(taskIDs))
			}
			ui.Text(c, message).
				FontSize(12).TextColor(p.Text)
			worktree := false
			for _, taskID := range taskIDs {
				if task, ok := a.tasks[taskID]; ok && task.Isolation == "worktree" {
					worktree = true
					break
				}
			}
			if worktree {
				worktree := a.deleteWorktree
				label := "Delete the task's worktree"
				if multiple {
					label = "Delete their worktree folders"
				}
				if ui.Checkbox(c, &worktree, label).Changed() {
					a.deleteWorktree = worktree
				}
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Button(c, "Delete").OnClick(m.submitAgentDelete)
				ui.Button(c, "Cancel").OnClick(func() {
					a.deleteOpen = false
				})
			})
		})
	})
	if dialog.Dismissed() || !open {
		a.deleteOpen = false
	}
}

func (m *Model) agentsChangesDialog(c *ui.Context, p *ui.Theme) {
	a := &m.agents
	if !a.changesOpen {
		return
	}
	task, ok := a.tasks[a.changesTask]
	if !ok {
		a.changesOpen = false
		return
	}
	uncommitted, commits, grouped, _ := m.agentReview(task)
	added, removed := agentDiffSummary(uncommitted)
	if a.changesFold == nil {
		a.changesFold = make(map[string]bool)
	}
	open := a.changesOpen
	dialog := ui.Modal(c.Key("agents-changes-dialog"), &open, func() {
		ui.Column(c).Width(760).Height(520).Gap(8).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Children(func() {
					ui.Text(c, "Changes · "+task.Title).FontSize(14).Bold().
						TextColor(p.Text).SingleLine().Ellipsis("…")
					summary := agentDiffCountLabel(len(uncommitted), "changed file", "changed files")
					if grouped {
						summary += " vs HEAD"
					} else if task.Base != nil && *task.Base != "" {
						summary += " vs " + *task.Base
					}
					if added > 0 {
						summary += fmt.Sprintf("  +%d", added)
					}
					if removed > 0 {
						summary += fmt.Sprintf("  −%d", removed)
					}
					if len(commits) > 0 {
						summary += " · " + agentDiffCountLabel(len(commits), "commit", "commits")
					}
					ui.Text(c, summary).FontSize(11).TextColor(oiPaletteOf(p).textMuted).
						SingleLine().Ellipsis("…")
				})
				if task.Isolation == "worktree" {
					label := "Flat List"
					if !grouped {
						label = "Group by Commit"
					}
					ui.Button(c, label).Disabled(a.changesBusy).OnClick(func() {
						m.setAgentChangesGrouped(!grouped)
					})
				}
			})
			if a.changesBusy {
				ui.Text(c, "Loading changes…").FontSize(12).TextColor(p.TextMuted)
			} else if a.changesErr != "" {
				ui.Text(c, a.changesErr).FontSize(12).TextColor(p.Danger)
			} else if len(uncommitted) == 0 && len(commits) == 0 {
				ui.Text(c, "No changed files").FontSize(12).TextColor(p.TextMuted)
			}
			ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
				ui.Column(c).Gap(2).Children(func() {
					if grouped {
						key := "uncommitted"
						folded := a.changesFold[key]
						if len(uncommitted) > 0 {
							ui.ButtonBase(c.Key("agents-changes-group:"+key)).MinHeight(28).
								Padding(2, 6).Gap(6).Radius(4).AlignItems(ui.Center).
								Cursor(ui.CursorPointer).OnClick(func() {
								a.changesFold[key] = !folded
								m.invalidate()
							}).Children(func() {
								ui.Text(c, map[bool]string{true: "▸", false: "▾"}[folded]).
									FontSize(10).TextColor(p.TextMuted)
								ui.Text(c, "Uncommitted").Grow(1).MinWidth(0).FontSize(11).
									Bold().TextColor(p.Text).SingleLine()
								ui.Text(c, agentDiffCountLabel(len(uncommitted), "file", "files")).
									FontSize(10).TextColor(p.TextMuted).SingleLine()
								if added > 0 {
									ui.Text(c, fmt.Sprintf("+%d", added)).FontSize(10).
										TextColor(p.Success)
								}
								if removed > 0 {
									ui.Text(c, fmt.Sprintf("−%d", removed)).FontSize(10).
										TextColor(p.Danger)
								}
							})
							if !folded {
								for _, change := range uncommitted {
									m.agentChangeRow(c, p, task, change, nil)
								}
							}
						}
						key = "committed"
						folded = a.changesFold[key]
						if len(commits) > 0 {
							ui.ButtonBase(c.Key("agents-changes-group:"+key)).MinHeight(28).
								Padding(2, 6).Gap(6).Radius(4).AlignItems(ui.Center).
								Cursor(ui.CursorPointer).OnClick(func() {
								a.changesFold[key] = !folded
								m.invalidate()
							}).Children(func() {
								ui.Text(c, map[bool]string{true: "▸", false: "▾"}[folded]).
									FontSize(10).TextColor(p.TextMuted)
								ui.Text(c, "Committed").Grow(1).MinWidth(0).FontSize(11).
									Bold().TextColor(p.Text).SingleLine()
								ui.Text(c, agentDiffCountLabel(len(commits), "commit", "commits")).
									FontSize(10).TextColor(p.TextMuted).SingleLine()
							})
							if !folded {
								for _, commit := range commits {
									commit := commit
									key := "commit:" + commit.ID
									folded := a.changesFold[key]
									commitAdded, commitRemoved := agentDiffCommitStats(commit)
									ui.ButtonBase(c.Key("agents-change:"+key)).MinHeight(30).
										Padding(2, 7).Gap(6).Radius(4).AlignItems(ui.Center).
										Cursor(ui.CursorPointer).OnClick(func() {
										a.changesFold[key] = !folded
										m.invalidate()
									}).Children(func() {
										ui.Text(c, map[bool]string{true: "▸", false: "▾"}[folded]).
											FontSize(10).TextColor(p.TextMuted)
										ui.Text(c, agentDiffCommitLabel(commit)).Grow(1).
											MinWidth(0).FontSize(12).TextColor(p.Text).
											SingleLine().Ellipsis("…").Tooltip(commit.Message)
										if when := agentDiffTimeLabel(commit.TimeMS); when != "" {
											ui.Text(c, when).FontSize(10).TextColor(p.TextMuted).SingleLine()
										}
										if commitAdded > 0 {
											ui.Text(c, fmt.Sprintf("+%d", commitAdded)).FontSize(10).
												TextColor(p.Success)
										}
										if commitRemoved > 0 {
											ui.Text(c, fmt.Sprintf("−%d", commitRemoved)).FontSize(10).
												TextColor(p.Danger)
										}
									})
									if !folded {
										for _, change := range commit.Files {
											m.agentChangeRow(c, p, task, change, &commit)
										}
									}
								}
							}
						}
					} else {
						for _, change := range uncommitted {
							m.agentChangeRow(c, p, task, change, nil)
						}
					}
				})
			})
			if a.actionErr != "" {
				ui.Text(c, a.actionErr).FontSize(12).TextColor(p.Danger)
			}
			ui.Row(c).Gap(6).Children(func() {
				if task.Isolation == "worktree" {
					ui.Button(c, "Accept Changes…").
						Disabled(a.changesBusy || len(uncommitted) == 0 && len(commits) == 0).
						OnClick(m.acceptAgentChanges)
				}
				ui.Button(c, "Roll Back…").Disabled(a.changesBusy || len(uncommitted) == 0).
					OnClick(m.rollbackAgentChanges)
				ui.Button(c, "Refresh").Disabled(a.changesBusy).OnClick(func() {
					m.loadAgentChanges(a.changesTask)
				})
				ui.Spacer(c)
				ui.Button(c, "Close").OnClick(func() { a.changesOpen = false })
			})
		})
	})
	if dialog.Dismissed() || !open {
		a.changesOpen = false
	}
}

func (m *Model) agentChangeRow(c *ui.Context, p *ui.Theme, task agentTask,
	change agentChange, commit *agentCommit) {
	path, _ := agentChangePaths(change)
	displayPath := change.RelPath
	if displayPath == "" {
		displayPath = filepath.ToSlash(path)
		if task.Cwd != "" {
			if relative, err := filepath.Rel(task.Cwd, path); err == nil {
				displayPath = filepath.ToSlash(relative)
			}
		}
	}
	key := path
	if commit != nil {
		key = "commit:" + commit.ID + ":" + key
	}
	row := ui.Row(c.Key("agents-change:"+key)).MinHeight(28).
		Padding(0, 8).Gap(8).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if row.Hovered() {
		row.Background(p.SurfaceHover)
	}
	row.Children(func() {
		ui.Text(c, strings.ToUpper(change.Kind)).Width(76).
			FontSize(10).TextColor(p.TextMuted).SingleLine()
		ui.Text(c, displayPath).Grow(1).MinWidth(0).FontSize(12).
			TextColor(p.Text).SingleLine().Ellipsis("…")
		if change.Added > 0 {
			ui.Text(c, fmt.Sprintf("+%d", change.Added)).FontSize(10).
				TextColor(p.Success)
		}
		if change.Removed > 0 {
			ui.Text(c, fmt.Sprintf("−%d", change.Removed)).FontSize(10).
				TextColor(p.Danger)
		}
	})
	if row.Clicked() {
		m.showAgentChangeDiff(task.ID, change, commit)
	}
}
