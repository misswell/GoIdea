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
// openidea-server owns, whose output arrives on a streaming channel. It renders
// the text stream (ANSI colours removed); full-screen TUI programs are beyond a
// text pane, and say so rather than showing garbage.
type TermPane struct {
	visible  bool
	sessions []termSession
	active   int
	input    string
	err      string
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

// current is the active tab, or nil when no tab is open.
func (p *TermPane) current() *termSession {
	if len(p.sessions) == 0 {
		return nil
	}
	if p.active < 0 || p.active >= len(p.sessions) {
		p.active = len(p.sessions) - 1
	}
	return &p.sessions[p.active]
}

// byChannel finds the tab a streaming channel belongs to.
func (p *TermPane) byChannel(ch uint32) *termSession {
	for i := range p.sessions {
		if p.sessions[i].channel == ch {
			return &p.sessions[i]
		}
	}
	return nil
}

// byID finds the tab a PTY belongs to.
func (p *TermPane) byID(id uint32) *termSession {
	for i := range p.sessions {
		if p.sessions[i].id == id {
			return &p.sessions[i]
		}
	}
	return nil
}

// add opens a tab and makes it active.
func (p *TermPane) add() *termSession {
	p.sessions = append(p.sessions, termSession{})
	p.active = len(p.sessions) - 1
	return &p.sessions[len(p.sessions)-1]
}

// remove drops a tab, keeping the active index in range.
func (p *TermPane) remove(i int) {
	if i < 0 || i >= len(p.sessions) {
		return
	}
	p.sessions = append(p.sessions[:i], p.sessions[i+1:]...)
	if p.active >= len(p.sessions) {
		p.active = len(p.sessions) - 1
	}
	if p.active < 0 {
		p.active = 0
	}
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
	m.spawnTermSession(root)
}

// OpenTermTab opens one more shell tab, as OpenIDEA's New Terminal Tab does.
func (m *Model) OpenTermTab() {
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
	m.spawnTermSession(root)
}

// spawnTermSession adds a tab and starts its login shell in the project root.
func (m *Model) spawnTermSession(root string) {
	ch := m.nextChannel()
	s := m.term.add()
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

// CloseTermTab ends a tab's shell and drops the tab; closing the last one hides
// the window, as OpenIDEA's Close Terminal Session does.
func (m *Model) CloseTermTab(i int) {
	if i < 0 || i >= len(m.term.sessions) {
		return
	}
	s := &m.term.sessions[i]
	id := s.id
	if id != 0 {
		go func() {
			ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
			defer cancel()
			_, _ = m.mgr.Invoke(ctx, Window, "term_kill", map[string]any{"id": id})
		}()
	}
	m.term.remove(i)
	if len(m.term.sessions) == 0 {
		m.hideTool("term")
	}
	m.invalidate()
}

// CloseActiveTermTab closes the tab the user is looking at.
func (m *Model) CloseActiveTermTab() {
	if len(m.term.sessions) == 0 {
		return
	}
	m.CloseTermTab(m.term.active)
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

// SmokeTerm starts a real shell, runs `echo` in it, opens a second tab and
// closes it, then kills both shells: the offscreen check of the terminal path.
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
		m.term.visible = true
		m.occupyBottom("term")
		m.term.sessions = nil
		m.term.active = 0
		s := m.term.add()
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
	second := m.waitTermTab(ctx, ch)
	out["tabs"] = second
	m.post(func() { m.CloseTermTab(1) })
	var left int
	closeDeadline := time.Now().Add(20 * time.Second)
	for {
		m.post(func() { left = len(m.term.sessions) })
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
	if _, err := m.mgr.Invoke(ctx, Window, "term_kill", map[string]any{"id": info.ID}); err != nil {
		out["killError"] = err.Error()
	}
	m.post(func() {
		m.term.sessions = nil
		m.term.active = 0
		m.hideTool("term")
		m.invalidate()
	})
	return out, nil
}

// waitTermTab waits for the tab after the one on ch to get its PTY and returns
// the tab count; a timeout records the count as it is.
func (m *Model) waitTermTab(ctx context.Context, ch uint32) int {
	deadline := time.Now().Add(30 * time.Second)
	var tabs int
	for {
		var ready bool
		m.post(func() {
			tabs = len(m.term.sessions)
			for i := range m.term.sessions {
				s := &m.term.sessions[i]
				if s.channel != ch && s.id != 0 {
					ready = true
				}
			}
		})
		if tabs > 1 && ready {
			return tabs
		}
		if time.Now().After(deadline) {
			return tabs
		}
		select {
		case <-ctx.Done():
			return tabs
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// text is the scrollback, one line per entry.
func (p *termSession) text() string {
	return strings.Join(p.lines, "\n")
}
