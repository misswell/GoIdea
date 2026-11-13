package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"mayoi/internal/openidea"
)

// TermPane is the integrated terminal: one tab per PTY session that
// openidea-server owns, whose output arrives on a streaming channel. A pane may
// split into a second tab group, as OpenIDEA splits its terminal; each group
// keeps its own tabs and active one. It renders the text stream (ANSI colours
// removed); full-screen TUI programs are beyond a text pane, and say so rather
// than showing garbage.
type TermPane struct {
	visible bool
	groups  []termGroup
	// active is the group that takes typing; split is "right" or "down" while
	// a second group shows, "" with one group.
	active int
	split  string
	input  string
	err    string
}

// termGroup is one pane's tabs: the primary group is 0, the split group 1.
type termGroup struct {
	tabs   []termSession
	active int
}

// termSession is one terminal tab: its PTY, shell and scrollback.
type termSession struct {
	// starting covers the spawn round trip; id is the PTY, 0 before it exists.
	starting bool
	id       uint32
	pid      *int32
	channel  uint32
	shell    string

	lines     []string
	truncated bool
	ended     bool
	exitCode  *int32
}

// group returns group g, creating it when it does not exist.
func (p *TermPane) group(g int) *termGroup {
	for len(p.groups) <= g {
		p.groups = append(p.groups, termGroup{})
	}
	return &p.groups[g]
}

// activeGroup is the group that takes typing, or nil before the first tab.
func (p *TermPane) activeGroup() *termGroup {
	if len(p.groups) == 0 {
		return nil
	}
	if p.active < 0 || p.active >= len(p.groups) {
		p.active = len(p.groups) - 1
	}
	return &p.groups[p.active]
}

// tab is the group's active tab, nil when the group has none.
func (g *termGroup) tab() *termSession {
	if g.active < 0 || g.active >= len(g.tabs) {
		g.active = len(g.tabs) - 1
	}
	if g.active < 0 {
		return nil
	}
	return &g.tabs[g.active]
}

// current is the active tab of the active group, or nil when none is open.
func (p *TermPane) current() *termSession {
	if g := p.activeGroup(); g != nil {
		return g.tab()
	}
	return nil
}

// byChannel finds the tab a streaming channel belongs to.
func (p *TermPane) byChannel(ch uint32) *termSession {
	for gi := range p.groups {
		tabs := p.groups[gi].tabs
		for ti := range tabs {
			if tabs[ti].channel == ch {
				return &tabs[ti]
			}
		}
	}
	return nil
}

// byID finds the tab a PTY belongs to.
func (p *TermPane) byID(id uint32) *termSession {
	for gi := range p.groups {
		tabs := p.groups[gi].tabs
		for ti := range tabs {
			if tabs[ti].id == id {
				return &tabs[ti]
			}
		}
	}
	return nil
}

// add opens a tab in group g and makes it that group's active tab.
func (p *TermPane) add(g int) *termSession {
	gr := p.group(g)
	gr.tabs = append(gr.tabs, termSession{})
	gr.active = len(gr.tabs) - 1
	p.active = g
	return &gr.tabs[gr.active]
}

// remove drops a tab from group g. A split group left empty merges away, and an
// emptied pane is only hidden; the caller ends the shell.
func (p *TermPane) remove(g, ti int) {
	if g < 0 || g >= len(p.groups) {
		return
	}
	gr := &p.groups[g]
	if ti < 0 || ti >= len(gr.tabs) {
		return
	}
	gr.tabs = append(gr.tabs[:ti], gr.tabs[ti+1:]...)
	if gr.active >= len(gr.tabs) {
		gr.active = len(gr.tabs) - 1
	}
	if gr.active < 0 {
		gr.active = 0
	}
	if g == 1 && len(gr.tabs) == 0 {
		p.groups = p.groups[:1]
		p.split = ""
		p.active = 0
		return
	}
	if p.active >= len(p.groups) {
		p.active = len(p.groups) - 1
	}
	if p.active < 0 {
		p.active = 0
	}
}

// total counts the open tabs across groups.
func (p *TermPane) total() int {
	n := 0
	for gi := range p.groups {
		n += len(p.groups[gi].tabs)
	}
	return n
}

// maxTermLines bounds the scrollback each tab keeps.
const maxTermLines = 4000

func (m *Model) resetTerm() { m.term = TermPane{} }

// ShowTerm toggles the terminal; opening it starts a shell on first use.
func (m *Model) ShowTerm() {
	if m.term.visible {
		m.hideTool("term")
		m.invalidate()
		return
	}
	m.term.visible = true
	m.occupyBottom("term")
	if s := m.term.current(); s == nil || (s.id == 0 && !s.starting && !s.ended) {
		m.SpawnTerm()
	}
	m.invalidate()
}

// SpawnTerm starts the user's login shell in a tab, when the active one has no
// shell left to use.
func (m *Model) SpawnTerm() {
	root := m.Root()
	if root == "" {
		m.term.err = "先打开一个项目，再启动终端"
		m.invalidate()
		return
	}
	if s := m.term.current(); s != nil && (s.starting || s.id != 0) {
		return
	}
	m.spawnTermSession(root, m.term.active)
}

// OpenTermTab opens one more shell tab in the active group, as OpenIDEA's New
// Terminal Tab does.
func (m *Model) OpenTermTab() {
	m.OpenTermTabIn(m.term.active)
}

// OpenTermTabIn opens a tab in group g, as a split pane's own + does.
func (m *Model) OpenTermTabIn(g int) {
	root := m.Root()
	if root == "" {
		m.term.err = "先打开一个项目，再启动终端"
		m.invalidate()
		return
	}
	if !m.term.visible {
		m.term.visible = true
		m.occupyBottom("term")
	}
	m.spawnTermSession(root, g)
}

// SplitTermRight splits the window and opens a shell beside the active group.
func (m *Model) SplitTermRight() { m.SplitTerm("right") }

// SplitTermDown splits the window and opens a shell below the active group.
func (m *Model) SplitTermDown() { m.SplitTerm("down") }

// SplitTerm shows a second tab group in the given direction, as OpenIDEA's
// Split Right and Split Down do; splitting twice only changes the direction.
func (m *Model) SplitTerm(axis string) {
	root := m.Root()
	if root == "" {
		m.term.err = "先打开一个项目，再启动终端"
		m.invalidate()
		return
	}
	if !m.term.visible {
		m.term.visible = true
		m.occupyBottom("term")
	}
	if m.term.split != "" && len(m.term.groups) > 1 {
		m.term.split = axis
		m.invalidate()
		return
	}
	m.term.split = axis
	m.spawnTermSession(root, 1)
}

// UnsplitTerm merges the split group's tabs into the primary group, keeping
// their shells, as OpenIDEA's Unsplit does.
func (m *Model) UnsplitTerm() {
	if len(m.term.groups) < 2 {
		m.term.split = ""
		m.invalidate()
		return
	}
	primary := &m.term.groups[0]
	split := &m.term.groups[1]
	primary.tabs = append(primary.tabs, split.tabs...)
	primary.active = len(primary.tabs) - 1
	m.term.groups = m.term.groups[:1]
	m.term.split = ""
	m.term.active = 0
	m.invalidate()
}

// spawnTermSession adds a tab to group g and starts its login shell in the
// project root.
func (m *Model) spawnTermSession(root string, g int) {
	ch := m.nextChannel()
	s := m.term.add(g)
	s.starting = true
	s.channel = ch
	m.term.err = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.mgr.Invoke(ctx, Window, "term_spawn", map[string]any{
			"options": map[string]any{
				"cwd": root, "args": []string{}, "env": map[string]string{},
				"cols": 100, "rows": 30, "shellIntegration": true,
			},
			"onOutput": map[string]any{"$channel": ch},
		})
		var info openidea.TerminalInfo
		if err == nil {
			err = json.Unmarshal(raw, &info)
		}
		m.post(func() {
			s := m.term.byChannel(ch)
			if s != nil {
				s.starting = false
			}
			if m.Root() != root || s == nil {
				if err == nil {
					// The tab or the project went away while the shell started:
					// do not leak it.
					go func() {
						ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
						defer cancel()
						_, _ = m.mgr.Invoke(ctx, Window, "term_kill", map[string]any{"id": info.ID})
					}()
				}
				if s == nil {
					m.invalidate()
				}
				return
			}
			if err != nil {
				m.term.err = "启动终端失败：" + err.Error()
				m.invalidate()
				return
			}
			s.id = info.ID
			s.pid = info.PID
			if info.Title != "" {
				s.shell = info.Title
			}
			m.invalidate()
		})
	}()
}

// SubmitTermInput sends the input line to the active shell, as Enter would.
func (m *Model) SubmitTermInput() {
	line := m.term.input
	s := m.term.current()
	if s == nil || s.id == 0 {
		return
	}
	m.term.input = ""
	id := s.id
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
		defer cancel()
		if _, err := m.mgr.Invoke(ctx, Window, "term_write", map[string]any{
			"id": id, "data": line + "\r",
		}); err != nil {
			m.post(func() {
				m.term.err = "写入终端失败：" + err.Error()
				m.invalidate()
			})
		}
	}()
}

// KillTerm ends the active shell; the tab keeps its output as a record.
func (m *Model) KillTerm() {
	s := m.term.current()
	if s == nil || s.id == 0 {
		return
	}
	id := s.id
	s.id = 0
	s.pid = nil
	s.ended = true
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
		defer cancel()
		if _, err := m.mgr.Invoke(ctx, Window, "term_kill", map[string]any{"id": id}); err != nil {
			m.post(func() {
				m.term.err = "结束终端失败：" + err.Error()
				m.invalidate()
			})
		}
	}()
}

// CloseTermTab ends a tab's shell and drops the tab; closing a split group's
// last tab merges the split away, and closing every tab hides the window, as
// OpenIDEA's Close Terminal Session does.
func (m *Model) CloseTermTab(g, ti int) {
	if g < 0 || g >= len(m.term.groups) {
		return
	}
	tabs := m.term.groups[g].tabs
	if ti < 0 || ti >= len(tabs) {
		return
	}
	id := tabs[ti].id
	if id != 0 {
		go func() {
			ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
			defer cancel()
			_, _ = m.mgr.Invoke(ctx, Window, "term_kill", map[string]any{"id": id})
		}()
	}
	m.term.remove(g, ti)
	if m.term.total() == 0 {
		m.hideTool("term")
	}
	m.invalidate()
}

// CloseActiveTermTab closes the tab the user is looking at.
func (m *Model) CloseActiveTermTab() {
	gr := m.term.activeGroup()
	if gr == nil || gr.active < 0 || gr.active >= len(gr.tabs) {
		return
	}
	m.CloseTermTab(m.term.active, gr.active)
}

// ClearTerm empties the active tab's scrollback.
func (m *Model) ClearTerm() {
	if s := m.term.current(); s != nil {
		s.lines = nil
		s.truncated = false
	}
	m.invalidate()
}

// write appends terminal output, honoring the line discipline a text pane can:
// newlines start lines, a carriage return rewrites the current one (progress
// bars), and everything else appends. ANSI colour codes are dropped.
func (p *termSession) write(text string) {
	text = strings.ReplaceAll(stripANSI(text), "\r\n", "\n")
	if len(p.lines) == 0 {
		p.lines = []string{""}
	}
	for _, r := range text {
		switch r {
		case '\n':
			p.lines = append(p.lines, "")
		case '\r':
			p.lines[len(p.lines)-1] = ""
		default:
			p.lines[len(p.lines)-1] += string(r)
		}
	}
	if over := len(p.lines) - maxTermLines; over > 0 {
		p.lines = append(p.lines[:0], p.lines[over:]...)
		p.truncated = true
	}
}

func (m *Model) acceptTermChunk(ch uint32, data json.RawMessage) {
	var text string
	if json.Unmarshal(data, &text) != nil {
		return
	}
	if text == "" {
		return
	}
	s := m.term.byChannel(ch)
	if s == nil {
		return
	}
	s.write(text)
	m.invalidate()
}

func (m *Model) acceptTermExit(id uint32, code *int32) {
	s := m.term.byID(id)
	if s == nil {
		return
	}
	s.id = 0
	s.pid = nil
	s.ended = true
	s.exitCode = code
	m.invalidate()
}

// SmokeTerm starts a real shell, runs `echo` in it, opens and closes a second
// tab, splits the pane into a second shell and merges it again, then kills
// every shell: the offscreen check of the terminal path.
func (m *Model) SmokeTerm(ctx context.Context) (map[string]any, error) {
	out := map[string]any{"shell": ""}
	root := m.Root()
	if root == "" {
		return out, errors.New("没有打开项目")
	}
	if rawShell, err := m.mgr.Invoke(ctx, Window, "term_default_shell", nil); err != nil {
		out["defaultShellError"] = err.Error()
	} else {
		var shell string
		if json.Unmarshal(rawShell, &shell) == nil {
			out["shell"] = shell
		}
	}
	ch := m.nextChannel()
	m.post(func() {
		m.term = TermPane{visible: true}
		m.occupyBottom("term")
		s := m.term.add(0)
		s.starting = true
		s.channel = ch
		m.invalidate()
	})
	raw, err := m.mgr.Invoke(ctx, Window, "term_spawn", map[string]any{
		"options": map[string]any{
			"cwd": root, "args": []string{}, "env": map[string]string{},
			"cols": 100, "rows": 30, "shellIntegration": false,
		},
		"onOutput": map[string]any{"$channel": ch},
	})
	if err != nil {
		return out, err
	}
	var info openidea.TerminalInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		return out, err
	}
	out["id"] = info.ID
	out["title"] = info.Title
	out["pid"] = info.PID
	m.post(func() {
		s := m.term.byChannel(ch)
		if s == nil {
			return
		}
		s.starting = false
		s.id = info.ID
		s.pid = info.PID
		s.shell = info.Title
		m.invalidate()
	})
	if _, err := m.mgr.Invoke(ctx, Window, "term_write", map[string]any{
		"id": info.ID, "data": "echo GOIDEA_TERM_OK\r",
	}); err != nil {
		return out, err
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		var text string
		m.post(func() {
			if s := m.term.byChannel(ch); s != nil {
				text = s.text()
			}
		})
		if strings.Contains(text, "GOIDEA_TERM_OK") {
			out["output"] = text
			break
		}
		if time.Now().After(deadline) {
			out["output"] = text
			out["timeout"] = true
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
	// A second tab gets its own PTY, and closing it leaves the first one.
	m.post(func() { m.OpenTermTab() })
	tabs, tabsReady := m.waitTermTabs(ctx, ch, 2)
	out["tabs"] = tabs
	out["tabsReady"] = tabsReady
	m.post(func() { m.CloseTermTab(0, 1) })
	var left int
	closeDeadline := time.Now().Add(20 * time.Second)
	for {
		m.post(func() { left = m.term.total() })
		if left == 1 || time.Now().After(closeDeadline) {
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	out["tabsAfterClose"] = left
	// Split Right puts a new shell in a second group, as OpenIDEA does.
	var splitCh uint32
	m.post(func() {
		m.SplitTermRight()
		if len(m.term.groups) > 1 && len(m.term.groups[1].tabs) > 0 {
			splitCh = m.term.groups[1].tabs[0].channel
		}
	})
	groups, splitReady := m.waitTermSplit(ctx, splitCh)
	var axis string
	m.post(func() { axis = m.term.split })
	out["split"] = axis
	out["splitGroups"] = groups
	out["splitReady"] = splitReady
	// Unsplit keeps both shells in one group.
	m.post(func() { m.UnsplitTerm() })
	var unsplit string
	var unsplitGroups, unsplitTabs int
	m.post(func() {
		unsplit = m.term.split
		unsplitGroups = len(m.term.groups)
		unsplitTabs = m.term.total()
	})
	out["unsplitAxis"] = unsplit
	out["unsplitGroups"] = unsplitGroups
	out["unsplitTabs"] = unsplitTabs
	var ids []uint32
	m.post(func() {
		for gi := range m.term.groups {
			for ti := range m.term.groups[gi].tabs {
				if id := m.term.groups[gi].tabs[ti].id; id != 0 {
					ids = append(ids, id)
				}
			}
		}
	})
	for _, id := range ids {
		if _, err := m.mgr.Invoke(ctx, Window, "term_kill", map[string]any{"id": id}); err != nil {
			out["killError"] = err.Error()
		}
	}
	m.post(func() {
		m.term = TermPane{}
		m.hideTool("term")
		m.invalidate()
	})
	return out, nil
}

// waitTermTabs waits until the pane has want tabs and every tab besides the one
// on skipCh has its PTY; a timeout reports what it saw.
func (m *Model) waitTermTabs(ctx context.Context, skipCh uint32, want int) (int, bool) {
	deadline := time.Now().Add(30 * time.Second)
	var tabs int
	var ready bool
	for {
		m.post(func() {
			tabs = m.term.total()
			ready = tabs >= want
			for gi := range m.term.groups {
				for ti := range m.term.groups[gi].tabs {
					s := &m.term.groups[gi].tabs[ti]
					if s.channel != skipCh && s.id == 0 {
						ready = false
					}
				}
			}
		})
		if ready || time.Now().After(deadline) {
			return tabs, ready
		}
		select {
		case <-ctx.Done():
			return tabs, ready
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// waitTermSplit waits for the group holding ch to exist with its PTY.
func (m *Model) waitTermSplit(ctx context.Context, ch uint32) (int, bool) {
	deadline := time.Now().Add(30 * time.Second)
	var groups int
	var ready bool
	for {
		m.post(func() {
			groups = len(m.term.groups)
			ready = ch != 0 && m.term.split != "" && m.term.byChannel(ch) != nil
			if s := m.term.byChannel(ch); s == nil || s.id == 0 {
				ready = false
			}
		})
		if ready || time.Now().After(deadline) {
			return groups, ready
		}
		select {
		case <-ctx.Done():
			return groups, ready
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// text is the scrollback, one line per entry.
func (p *termSession) text() string {
	return strings.Join(p.lines, "\n")
}
