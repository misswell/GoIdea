package app

import (
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
)

const (
	agentRevealHorizon = 150 * time.Millisecond
	agentRevealFrame   = 16 * time.Millisecond
	agentMaxFrame      = 250 * time.Millisecond
)

// agentRevealState keeps the whole chunked text and the part already painted.
// Arrival changes the target; the visible prefix catches up over a short
// horizon, so a burst from the agent does not jump onto the screen at once.
type agentRevealState struct {
	Text    string
	Shown   int
	Last    time.Time
	Pending bool
}

func (m *Model) agentRevealedText(key, text string, live bool) string {
	prefs := m.agentPrefs()
	if !live || prefs.NoSmoothText {
		delete(m.agents.revealTexts, key)
		return text
	}
	now := time.Now()
	if m.agents.revealTexts == nil {
		m.agents.revealTexts = make(map[string]*agentRevealState)
	}
	state := m.agents.revealTexts[key]
	if state == nil {
		// Text that was already present when the message first appeared is
		// shown whole. Only later chunks are paced.
		state = &agentRevealState{Text: text, Shown: utf8.RuneCountInString(text), Last: now}
		m.agents.revealTexts[key] = state
		return text
	}

	total := utf8.RuneCountInString(text)
	if state.Text != text {
		painted := agentTextPrefix(state.Text, state.Shown)
		if !strings.HasPrefix(text, painted) {
			state.Shown = total
		}
		if state.Shown > total {
			state.Shown = total
		}
		state.Text = text
	}
	if state.Shown >= total {
		state.Shown = total
		return agentWithoutBrokenTail(text)
	}

	elapsed := now.Sub(state.Last)
	if !state.Last.IsZero() && elapsed >= agentRevealFrame {
		backlog := total - state.Shown
		state.Shown = min(total, state.Shown+agentRevealStep(backlog, elapsed))
		state.Last = now
	}
	if state.Shown < total {
		m.scheduleAgentRevealRefresh()
	}
	return agentTextPrefix(text, state.Shown)
}

// agentRevealStep reports how much of a backlog to paint after elapsed. Its
// pure form keeps the pacing rules testable without a window.
func agentRevealStep(backlog int, elapsed time.Duration) int {
	if backlog <= 0 {
		return 0
	}
	ms := elapsed
	if ms < 0 {
		ms = 0
	}
	if ms > agentMaxFrame {
		ms = agentMaxFrame
	}
	if ms >= agentRevealHorizon {
		return backlog
	}
	step := int(math.Ceil(float64(backlog) * float64(ms) / float64(agentRevealHorizon)))
	return min(backlog, max(1, step))
}

func agentTextPrefix(text string, runes int) string {
	if runes <= 0 {
		return ""
	}
	if runes >= utf8.RuneCountInString(text) {
		return text
	}
	index := 0
	for offset := range text {
		if index == runes {
			return text[:offset]
		}
		index++
	}
	return text
}

// agentWithoutBrokenTail holds back the last half of a grapheme that can
// arrive split between chunks.
func agentWithoutBrokenTail(text string) string {
	runes := []rune(text)
	if len(runes) == 0 {
		return text
	}
	if runes[len(runes)-1] == '\u200d' {
		return string(runes[:len(runes)-1])
	}
	flags := 0
	for index := len(runes) - 1; index >= 0; index-- {
		if runes[index] < 0x1f1e6 || runes[index] > 0x1f1ff {
			break
		}
		flags++
	}
	if flags%2 == 1 {
		return string(runes[:len(runes)-1])
	}
	return text
}

func (m *Model) scheduleAgentRevealRefresh() {
	a := &m.agents
	if m.ctx == nil || a.revealTimer != nil {
		return
	}
	generation := a.generation
	a.revealTimer = time.AfterFunc(agentRevealFrame, func() {
		m.post(func() {
			if a.generation != generation {
				return
			}
			a.revealTimer = nil
			m.invalidate()
		})
	})
}

// scheduleAgentMeterRefresh keeps running time and reset countdowns moving on
// the same one-second cadence as OpenIDEA's Meters component.
func (m *Model) scheduleAgentMeterRefresh(task agentTask) {
	if m.ctx == nil || m.agents.meterTimer != nil {
		return
	}
	needsTick := agentTaskBusy(task.Status)
	if !needsTick {
		for _, limit := range m.agents.limits[task.Provider] {
			needsTick = limit.ResetsMS != nil
			if needsTick {
				break
			}
		}
	}
	if !needsTick {
		return
	}
	generation := m.agents.generation
	m.agents.meterTimer = time.AfterFunc(time.Second, func() {
		m.post(func() {
			if m.agents.generation != generation {
				return
			}
			m.agents.meterTimer = nil
			m.invalidate()
		})
	})
}

func (m *Model) agentTimelineEntryWorking(task agentTask, entries []agentEntry, index int) bool {
	if subagent := m.agents.currentSubagentCard(task.ID); subagent != nil {
		return index == len(entries)-1 && agentSubagentWorking(subagent.Status)
	}
	return index == len(entries)-1 && agentTaskBusy(task.Status)
}

func (m *Model) agentThoughtEntry(c *ui.Context, p *ui.Theme, oi oiColors,
	task agentTask, entries []agentEntry, index int, entry agentEntry, find *regexp.Regexp) {
	working := m.agentTimelineEntryWorking(task, entries, index)
	thinking := m.agentPrefs().thinking()
	defaultOpen := thinking == "expanded" || (thinking == "auto" && working)
	key := agentTimelineFoldKey(task.ID, m.agents.currentSubagent(), "thought", index)
	open, remembered := m.agents.timelineOpen[key]
	if !remembered {
		open = defaultOpen
	}
	if find != nil {
		open = true
	}
	ui.Column(c).Gap(2).Children(func() {
		ui.ButtonBase(c.Key("agents-thought:"+key)).MinHeight(24).Padding(1, 4).
			Radius(4).Gap(5).AlignItems(ui.Center).Cursor(ui.CursorPointer).
			OnClick(func() {
				if m.agents.timelineOpen == nil {
					m.agents.timelineOpen = make(map[string]bool)
				}
				m.agents.timelineOpen[key] = !open
				m.invalidate()
			}).Children(func() {
			chevron := "▸"
			if open {
				chevron = "▾"
			}
			ui.Text(c, chevron).FontSize(10).TextColor(oi.textMuted)
			label := agentThoughtPreview(entry.Text)
			if working {
				label = "Thinking"
			} else if label == "" {
				label = "Reasoning"
			}
			ag := agentFindRichText(c, label, find, p).Grow(1).MinWidth(0).
				FontSize(11).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
			if working {
				ag.TextColor(oi.accent)
			}
		})
		if !open {
			return
		}
		text := m.agentRevealedText(key, entry.Text, working)
		ui.Column(c).PaddingStart(12).Children(func() {
			m.agentMarkdown(c, p, oi, text, task.Cwd, find)
		})
	})
}

func (m *Model) agentToolEntry(c *ui.Context, p *ui.Theme, oi oiColors,
	task agentTask, entries []agentEntry, index int, entry agentEntry, find *regexp.Regexp) {
	live := entry.Status == "running" || entry.Status == "pending"
	hasOutput := entry.Output != nil && strings.TrimSpace(*entry.Output) != ""
	key := agentTimelineFoldKey(task.ID, m.agents.currentSubagent(), "tool", index)
	open := m.agentPrefs().ExpandToolOutput || live || find != nil
	if remembered, ok := m.agents.timelineOpen[key]; ok {
		open = remembered
	}
	if find != nil {
		open = open || hasOutput
	}
	background := oi.hoverTint
	if entry.Status == "failed" {
		background = p.Danger.Alpha(0.08)
	}
	ui.Column(c).Gap(2).Padding(4, 5).Radius(4).Background(background).
		Children(func() {
			ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
				head := ui.ButtonBase(c.Key("agents-tool:"+key)).Grow(1).MinWidth(0).
					Padding(0, 2).Gap(5).AlignItems(ui.Center).Cursor(ui.CursorPointer)
				if hasOutput {
					head.OnClick(func() {
						if m.agents.timelineOpen == nil {
							m.agents.timelineOpen = make(map[string]bool)
						}
						m.agents.timelineOpen[key] = !open
						m.invalidate()
					})
				}
				head.Children(func() {
					if hasOutput {
						chevron := "▸"
						if open {
							chevron = "▾"
						}
						ui.Text(c, chevron).FontSize(10).TextColor(oi.textMuted)
					}
					title := entry.Title
					if title == "" {
						title = entry.Kind
					}
					element := agentFindRichText(c, title, find, p).Grow(1).
						MinWidth(0).FontSize(11).TextColor(oi.text).
						SingleLine().Ellipsis("…").Tooltip(title)
					if live {
						element.TextColor(oi.accent)
					}
					if entry.Status != "" && entry.Status != "completed" {
						label := entry.Status
						color := oi.textMuted
						if entry.Status == "failed" {
							label = "Failed"
							color = p.Danger
						}
						ui.Text(c, label).FontSize(9).TextColor(color).SingleLine()
					}
				})
				for _, diff := range entry.Diffs {
					diff := diff
					label := ""
					if diff.Added > 0 {
						label = fmt.Sprintf("+%d", diff.Added)
					}
					if diff.Removed > 0 {
						if label != "" {
							label += " "
						}
						label += fmt.Sprintf("−%d", diff.Removed)
					}
					if label == "" {
						label = "Diff"
					}
					ui.Button(c, label).Tooltip(diff.Path).OnClick(func() {
						m.openAgentToolDiff(task, diff)
					})
				}
			})
			if hasOutput && open {
				agentSearchTextBlock(c, p, oi, *entry.Output, oi.textMuted, 12, find)
			}
		})
}

func (m *Model) openAgentToolDiff(task agentTask, diff agentToolDiff) {
	path := diff.Path
	if path == "" {
		return
	}
	if !filepath.IsAbs(path) && task.Cwd != "" {
		path = filepath.Join(task.Cwd, path)
	}
	m.revealAt(path, 1, 1)
}

func agentThoughtPreview(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = strings.TrimLeft(line, "#")
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "- ")
		line = strings.TrimPrefix(line, "* ")
		line = strings.NewReplacer("*", "", "_", "", "`", "").Replace(line)
		return strings.TrimSpace(line)
	}
	return ""
}
