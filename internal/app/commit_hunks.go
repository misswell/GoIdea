package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

// commitHunksState is one file's loaded line changes and the stable identities
// the user selected. Loading a picker makes that file explicitly hunk-scoped;
// selecting all of it again falls back to a whole-file stage.
type commitHunksState struct {
	loading  bool
	err      string
	hunks    []openidea.LineHunk
	keys     []string
	selected map[string]bool
}

func (s *commitHunksState) selectedCount() int {
	if s == nil {
		return 0
	}
	n := 0
	for _, key := range s.keys {
		if s.selected[key] {
			n++
		}
	}
	return n
}

func (s *commitHunksState) selectedKeys() []string {
	if s == nil {
		return nil
	}
	out := make([]string, 0, len(s.keys))
	for _, key := range s.keys {
		if s.selected[key] {
			out = append(out, key)
		}
	}
	return out
}

func (s *commitHunksState) selectAll(selected bool) {
	if s == nil {
		return
	}
	for _, key := range s.keys {
		s.selected[key] = selected
	}
}

func (m *Model) commitHunkChoice(c *commitState, path string) (selected, total int, ok bool) {
	if c == nil || c.hunks == nil {
		return 0, 0, false
	}
	state := c.hunks[path]
	if state == nil || state.loading || state.err != "" || len(state.hunks) == 0 {
		return 0, 0, false
	}
	return state.selectedCount(), len(state.hunks), true
}

func commitFileCanChooseHunks(f openidea.FileStatus) bool {
	if f.Unstaged == nil || *f.Unstaged != "modified" {
		return false
	}
	if f.Staged == nil {
		return true
	}
	switch *f.Staged {
	case "added", "renamed", "conflicted":
		return false
	default:
		return true
	}
}

func (m *Model) toggleCommitFileHunks(path string, include bool) {
	c := m.commit
	if c == nil {
		return
	}
	c.checks[path] = include
	if state := c.hunks[path]; state != nil {
		state.selectAll(include)
	}
	c.problems, c.problemsOverride = nil, false
	m.invalidate()
}

func (m *Model) openCommitHunks(path string) {
	c := m.commit
	if c == nil || path == "" || !m.commitFileCanBePartiallyCommitted(path) {
		return
	}
	if c.hunkPath == path {
		c.hunkPath = ""
		m.invalidate()
		return
	}
	c.hunkPath = path
	c.checks[path] = true
	c.problems, c.problemsOverride = nil, false
	if state := c.hunks[path]; state != nil && !state.loading {
		m.invalidate()
		return
	}
	state := &commitHunksState{loading: true, selected: map[string]bool{}}
	c.hunks[path] = state
	m.invalidate()
	tabs := m.allTabs()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, time.Minute)
		defer cancel()
		if err := m.saveOpenTabs(ctx, tabs); err != nil {
			m.post(func() {
				if c != m.commit || c.hunks[path] != state {
					return
				}
				state.loading = false
				state.err = "The file could not be saved: " + err.Error()
				m.invalidate()
			})
			return
		}
		_, disk, hunks, err := m.commitHunks(ctx, path)
		m.post(func() {
			if c != m.commit || c.hunks[path] != state {
				return
			}
			state.loading = false
			if err != nil {
				state.err = "Could not read line changes: " + err.Error()
				m.invalidate()
				return
			}
			if len(hunks) == 0 {
				state.err = "This file has no partial line changes"
				m.invalidate()
				return
			}
			keys := commitHunkKeys(disk, hunks)
			selected := make(map[string]bool, len(keys))
			for _, key := range keys {
				selected[key] = true
			}
			state.hunks, state.keys, state.selected = hunks, keys, selected
			c.checks[path] = true
			m.invalidate()
		})
	}()
}

func (m *Model) commitFileCanBePartiallyCommitted(path string) bool {
	for _, file := range m.vcs.files {
		if file.Path == path {
			return commitFileCanChooseHunks(file)
		}
	}
	return false
}

// commitHunkKeys gives each changed block an identity from its old and new
// text. Edits elsewhere in the file may move its line numbers without changing
// the chosen block.
func commitHunkKeys(text string, hunks []openidea.LineHunk) []string {
	seen := make(map[string]int, len(hunks))
	keys := make([]string, len(hunks))
	for i, hunk := range hunks {
		base := fmt.Sprintf("%q\x00%q", hunk.OldText, commitHunkNewText(text, hunk))
		occurrence := seen[base]
		seen[base] = occurrence + 1
		keys[i] = fmt.Sprintf("%s:%d", base, occurrence)
	}
	return keys
}

func commitHunkNewText(text string, hunk openidea.LineHunk) string {
	if hunk.Lines == 0 {
		return ""
	}
	lines := strings.SplitAfter(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	start := int(hunk.Start)
	if start < 0 || start >= len(lines) {
		return ""
	}
	end := start + int(hunk.Lines)
	if end > len(lines) {
		end = len(lines)
	}
	return strings.Join(lines[start:end], "")
}

func commitHunkLabel(index int, hunk openidea.LineHunk) string {
	line := int(hunk.Start) + 1
	switch hunk.Kind {
	case "added":
		if hunk.Lines == 1 {
			return fmt.Sprintf("%d. Added line %d", index+1, line)
		}
		return fmt.Sprintf("%d. Added lines %d-%d", index+1, line, line+int(hunk.Lines)-1)
	case "deleted":
		return fmt.Sprintf("%d. Deleted %s at line %d",
			index+1, count(int(hunk.OldLines), "line", "lines"), line)
	default:
		if hunk.Lines == 1 {
			return fmt.Sprintf("%d. Changed line %d", index+1, line)
		}
		return fmt.Sprintf("%d. Changed lines %d-%d", index+1, line, line+int(hunk.Lines)-1)
	}
}

func commitHunkDetail(hunk openidea.LineHunk) string {
	text := strings.TrimSpace(strings.SplitN(hunk.OldText, "\n", 2)[0])
	if text == "" {
		return "No previous text"
	}
	if len([]rune(text)) > 90 {
		text = string([]rune(text)[:90]) + "…"
	}
	return "Before: " + text
}

func (m *Model) commitHunksDialog(c *ui.Context, p *ui.Theme) {
	s := m.commit
	if s == nil || s.hunkPath == "" {
		return
	}
	state := s.hunks[s.hunkPath]
	open := true
	d := ui.Modal(c.Key("commit-hunks:"+s.hunkPath), &open, func() {
		ui.Column(c).Width(520).Gap(8).Children(func() {
			ui.Text(c, "Include Hunks").FontSize(14).Bold().TextColor(p.Text).SingleLine()
			ui.Text(c, filepath.ToSlash(s.hunkPath)).Font(oiFontMono).FontSize(11).
				TextColor(p.TextMuted).SingleLine().Ellipsis("…")
			if state == nil || state.loading {
				ui.Text(c, "Reading line changes…").FontSize(12).TextColor(p.TextMuted)
			} else if state.err != "" {
				ui.Text(c, state.err).FontSize(12).TextColor(p.Danger)
			} else {
				ui.Row(c).Gap(8).Children(func() {
					if ui.Button(c, "Include All").Clicked() {
						state.selectAll(true)
						s.checks[s.hunkPath] = true
					}
					if ui.Button(c, "Include None").Clicked() {
						state.selectAll(false)
						s.checks[s.hunkPath] = false
					}
					ui.Text(c, fmt.Sprintf("%d of %d selected", state.selectedCount(), len(state.hunks))).
						FontSize(12).TextColor(p.TextMuted)
				})
				ui.Scroll(c).Height(280).Children(func() {
					for i, hunk := range state.hunks {
						key := state.keys[i]
						selected := state.selected[key]
						row := ui.Row(c.Key(fmt.Sprintf("commit-hunk:%d", i))).Gap(6).AlignItems(ui.Start)
						row.Children(func() {
							if ui.Checkbox(c, &selected, commitHunkLabel(i, hunk)).Changed() {
								state.selected[key] = selected
								s.checks[s.hunkPath] = state.selectedCount() > 0
								s.problems, s.problemsOverride = nil, false
								m.invalidate()
							}
							ui.Text(c, commitHunkDetail(hunk)).FontSize(11).
								TextColor(p.TextMuted).SingleLine().Ellipsis("…")
						})
					}
				})
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "Done").Clicked() {
					s.hunkPath = ""
					s.problems, s.problemsOverride = nil, false
					m.invalidate()
				}
			})
		})
	})
	if d.Dismissed() || !open {
		s.hunkPath = ""
		m.invalidate()
	}
}
