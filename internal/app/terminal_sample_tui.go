package app

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

const sampleTUISpinner = "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"

const (
	sampleTUIStart = "\x1b[?1049h"
	sampleTUIEnd   = "\x1b[?1049l"
)

// sampleTUIFrame mirrors OpenIDEA's performance fixture: every frame repaints
// the whole alternate screen, updates the window title and changes the spinner.
func sampleTUIFrame(n, cols, rows int) string {
	if cols < 7 {
		cols = 7
	}
	if rows < 8 {
		rows = 8
	}
	inner := max(cols-6, 1)
	spinnerRunes := []rune(sampleTUISpinner)
	spinner := spinnerRunes[n%len(spinnerRunes)]
	var out strings.Builder
	fmt.Fprintf(&out, "\x1b]0;%c Thinking - sample\a\x1b[?25l\x1b[H", spinner)
	line := func(row int, text string) {
		fmt.Fprintf(&out, "\x1b[%d;1H\x1b[2K%s\x1b[0m", row, text)
	}
	rgb := func(r, g, b int) string {
		return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, b)
	}
	line(1, rgb(130, 130, 140)+"  ~/projects/sample")
	for row := 2; row < rows-6; row++ {
		text := fillSampleTUI(fmt.Sprintf("Line %d of the answer, with words and `code` in it. ", row-1), inner)
		line(row, rgb(200, 200, 205)+"  "+text)
	}
	line(rows-6, fmt.Sprintf("%s  %c Thinking… %.1fs %s[stop]",
		rgb(220, 180, 90), spinner, float64(n)/10, rgb(110, 110, 120)))
	line(rows-4, fmt.Sprintf("%s  ╭%s╮", rgb(90, 90, 100), strings.Repeat("─", inner)))
	line(rows-3, fmt.Sprintf("%s  │ %s❯%s%s│",
		rgb(90, 90, 100), rgb(200, 200, 205), strings.Repeat(" ", max(inner-2, 0)), rgb(90, 90, 100)))
	line(rows-2, fmt.Sprintf("%s  ╰%s╯", rgb(90, 90, 100), strings.Repeat("─", inner)))
	line(rows, rgb(150, 150, 160)+"  Enter:send  │  Shift+Tab:mode  │  Ctrl+c:cancel")
	fmt.Fprintf(&out, "\x1b[%d;6H\x1b[?25h", rows-3)
	return out.String()
}

func fillSampleTUI(text string, width int) string {
	if text == "" || width <= 0 {
		return ""
	}
	filled := strings.Repeat(text, width/len([]rune(text))+1)
	runes := []rune(filled)
	return string(runes[:width])
}

// RunTerminalSampleTUI starts the hidden performance action on the active tab,
// opening a real terminal when none is running. Arguments accept the pinned
// fixture's `{frames, fps}` shape while keeping both loops bounded.
func (m *Model) RunTerminalSampleTUI(arg any) {
	options := terminalActionObject(arg)
	frames := terminalSampleInt(options["frames"], 40)
	fps := terminalSampleInt(options["fps"], 10)
	frames = min(max(frames, 0), 1000)
	fps = min(max(fps, 1), 60)

	if m.Root() == "" {
		m.term.err = "Open a project before starting the terminal"
		m.invalidate()
		return
	}
	m.post(func() {
		if m.term.current() == nil ||
			(m.term.current().id == 0 && !m.term.current().starting) {
			m.OpenTermTab()
		}
		m.dockActivate("term")
	})
	go m.streamTerminalSampleTUI(frames, fps)
}

func terminalSampleInt(value any, fallback int) int {
	switch number := value.(type) {
	case float64:
		return int(number)
	case float32:
		return int(number)
	case int:
		return number
	case int64:
		return int(number)
	case json.Number:
		parsed, err := number.Int64()
		if err == nil {
			return int(parsed)
		}
	case string:
		parsed, err := parseTerminalSampleInt(number)
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func parseTerminalSampleInt(value string) (int, error) {
	var parsed int
	_, err := fmt.Sscanf(value, "%d", &parsed)
	return parsed, err
}

func (m *Model) streamTerminalSampleTUI(frames, fps int) {
	ctx, cancel := context.WithTimeout(m.ctx, 20*time.Second)
	defer cancel()

	var channel uint32
	deadline := time.Now().Add(5 * time.Second)
	for channel == 0 && time.Now().Before(deadline) {
		m.post(func() {
			if session := m.term.current(); session != nil &&
				session.channel != 0 && !session.starting && session.id != 0 {
				channel = session.channel
			}
		})
		if channel != 0 {
			break
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(50 * time.Millisecond):
		}
	}
	if channel == 0 {
		m.post(func() {
			m.term.err = "Starting the sample TUI failed: the terminal did not open"
			m.invalidate()
		})
		return
	}

	if !m.postTerminalWrite(channel, sampleTUIStart) {
		return
	}
	interval := time.Second / time.Duration(fps)
	for frame := 0; frame < frames; frame++ {
		cols, rows := defaultTerminalCols, defaultTerminalRows
		m.post(func() {
			if session := m.term.byChannel(channel); session != nil {
				if session.terminalCols > 0 {
					cols = session.terminalCols
				}
				if session.terminalRows > 0 {
					rows = session.terminalRows
				}
			}
		})
		if !m.postTerminalWrite(channel, sampleTUIFrame(frame, cols, rows)) {
			return
		}
		select {
		case <-ctx.Done():
			m.postTerminalWrite(channel, sampleTUIEnd)
			return
		case <-time.After(interval):
		}
	}
	m.postTerminalWrite(channel, sampleTUIEnd)
}

func (m *Model) postTerminalWrite(channel uint32, text string) bool {
	wrote := false
	m.post(func() {
		session := m.term.byChannel(channel)
		if session == nil {
			return
		}
		session.write(text)
		session.scroll.Y = math.MaxFloat32
		wrote = true
		m.invalidate()
	})
	return wrote
}
