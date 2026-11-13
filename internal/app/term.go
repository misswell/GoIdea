package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"mayoi/internal/openidea"
)

// TermPane is the integrated terminal: a login shell in a PTY owned by
// openidea-server, whose output arrives on a streaming channel. It renders the
// text stream (ANSI colours removed); full-screen TUI programs are beyond a
// text pane, and say so rather than showing garbage.
type TermPane struct {
	visible bool
	// starting covers the spawn round trip; id is the PTY, 0 before it exists.
	starting bool
	id       uint32
	pid      *int32
	channel  uint32
	shell    string
	input    string

	lines     []string
	truncated bool
	ended     bool
	exitCode  *int32
	err       string
}

// maxTermLines bounds the scrollback the pane keeps.
const maxTermLines = 4000

func (m *Model) resetTerm() { m.term = TermPane{} }

// ShowTerm toggles the terminal; opening it starts the shell on first use.
func (m *Model) ShowTerm() {
	m.term.visible = !m.term.visible
	if m.term.visible {
		m.tool = "term"
	}
	if m.term.visible && m.term.id == 0 && !m.term.starting {
		m.SpawnTerm()
	}
	m.invalidate()
}

// SpawnTerm starts the user's login shell in the project root.
func (m *Model) SpawnTerm() {
	root := m.Root()
	if root == "" {
		m.term.err = "先打开一个项目，再启动终端"
		m.invalidate()
		return
	}
	if m.term.starting || m.term.id != 0 {
		return
	}
	ch := m.nextChannel()
	m.term.starting = true
	m.term.channel = ch
	m.term.ended = false
	m.term.err = ""
	m.term.exitCode = nil
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
			m.term.starting = false
			if m.Root() != root {
				if err == nil {
					// The project changed while the shell started: do not leak it.
					go func() {
						ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
						defer cancel()
						_, _ = m.mgr.Invoke(ctx, Window, "term_kill", map[string]any{"id": info.ID})
					}()
				}
				return
			}
			if err != nil {
				m.term.err = "启动终端失败：" + err.Error()
				m.invalidate()
				return
			}
			m.term.id = info.ID
			m.term.pid = info.PID
			if info.Title != "" {
				m.term.shell = info.Title
			}
			m.invalidate()
		})
	}()
}

// SubmitTermInput sends the input line to the shell, as Enter would.
func (m *Model) SubmitTermInput() {
	line := m.term.input
	if m.term.id == 0 {
		return
	}
	m.term.input = ""
	id := m.term.id
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

// KillTerm ends the shell; the pane keeps its output as a record.
func (m *Model) KillTerm() {
	id := m.term.id
	if id == 0 {
		return
	}
	m.term.id = 0
	m.term.pid = nil
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

// ClearTerm empties the scrollback.
func (m *Model) ClearTerm() {
	m.term.lines = nil
	m.term.truncated = false
	m.invalidate()
}

// write appends terminal output, honoring the line discipline a text pane can:
// newlines start lines, a carriage return rewrites the current one (progress
// bars), and everything else appends. ANSI colour codes are dropped.
func (p *TermPane) write(text string) {
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

func (m *Model) acceptTermChunk(data json.RawMessage) {
	var text string
	if json.Unmarshal(data, &text) != nil {
		return
	}
	if text == "" {
		return
	}
	m.term.write(text)
	m.invalidate()
}

func (m *Model) acceptTermExit(id uint32, code *int32) {
	if m.term.id == 0 || m.term.id != id {
		return
	}
	m.term.id = 0
	m.term.pid = nil
	m.term.ended = true
	m.term.exitCode = code
	m.invalidate()
}

// SmokeTerm starts a real shell, runs `echo` in it, waits for the text, and
// kills it: the offscreen check of the terminal path.
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
		m.tool = "term"
		m.term.channel = ch
		m.term.lines = nil
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
		m.term.id = info.ID
		m.term.pid = info.PID
		m.term.shell = info.Title
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
		m.post(func() { text = m.term.text() })
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
	if _, err := m.mgr.Invoke(ctx, Window, "term_kill", map[string]any{"id": info.ID}); err != nil {
		out["killError"] = err.Error()
	}
	m.post(func() {
		m.term.id = 0
		m.term.pid = nil
		m.invalidate()
	})
	return out, nil
}

// text is the scrollback, one line per entry.
func (p *TermPane) text() string {
	return strings.Join(p.lines, "\n")
}
