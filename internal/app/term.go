package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

// TermPane is the integrated terminal: one tab per PTY session that
// openidea-server owns, whose output arrives on a streaming channel. A pane may
// split into a second tab group, as OpenIDEA splits its terminal; each group
// keeps its own tabs and active one. It renders the text stream with the ANSI
// cursor and alternate-screen operations used by command-line and full-screen
// programs.
type TermPane struct {
	visible bool
	groups  []termGroup
	// active is the group that takes typing; split is "right" or "down" while
	// a second group shows, "" with one group.
	active int
	split  string
	input  string
	err    string
	rename *termRenameState

	// cols and rows are the last measured terminal size. New tabs start at
	// this size so a pane does not jump when the first child repaints.
	cols int
	rows int
}

// termGroup is one pane's tabs: the primary group is 0, the split group 1.
type termGroup struct {
	tabs   []termSession
	active int
	focus  ui.Handle
}

// termSession is one terminal tab: its PTY, shell and scrollback.
type termSession struct {
	// starting covers the spawn round trip; id is the PTY, 0 before it exists.
	starting bool
	id       uint32
	pid      *int32
	channel  uint32
	shell    string
	// customName wins over the shell title after Rename Session.
	customName string
	cwd        string

	lines     []string
	truncated bool
	ended     bool
	exitCode  *int32

	// Full-screen programs draw into a cell grid instead of appending to the
	// shell's scrollback. The primary buffer is restored when they leave the
	// alternate screen, as an xterm does.
	alternate        bool
	primaryLines     []string
	primaryTruncated bool
	terminalCols     int
	terminalRows     int
	screenCells      [][]terminalCell
	screenRow        int
	screenCol        int
	savedRow         int
	savedCol         int
	escapeTail       string
	terminalTitle    string
	cursorHidden     bool

	mouseMode      int
	mouseSGR       bool
	mousePixels    bool
	bracketedPaste bool
	focusReporting bool
	focusKnown     bool
	focused        bool

	activeHyperlink   string
	hyperlinks        []terminalHyperlink
	primaryHyperlinks []terminalHyperlink

	copiedSelection string
	optionMetaKey   ui.Key

	desiredCols int
	desiredRows int
	ptyCols     int
	ptyRows     int
	resizeGen   uint64
	resizing    bool
	writeQueue  []byte
	writing     bool

	// scroll, finding and find stay with this tab when another tab becomes
	// active. findAt is the line the find bar last jumped to, -1 when the query
	// matched nothing, and reveal asks the next frame to scroll it into view.
	scroll  ui.ScrollState
	finding bool
	find    string
	findAt  int
	reveal  bool
}

type termRenameState struct {
	group  int
	tab    int
	text   string
	anchor ui.Rect
	focus  ui.Handle
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

// ActivateTermTab selects a tab in its group and makes that group active.
func (m *Model) ActivateTermTab(group, tab int) bool {
	if group < 0 || group >= len(m.term.groups) {
		return false
	}
	g := &m.term.groups[group]
	if tab < 0 || tab >= len(g.tabs) {
		return false
	}
	m.term.active = group
	g.active = tab
	m.invalidate()
	return true
}

// ActivateTermGroup focuses the selected tab of group.
func (m *Model) ActivateTermGroup(group int) bool {
	if group < 0 || group >= len(m.term.groups) {
		return false
	}
	g := &m.term.groups[group]
	if len(g.tabs) == 0 {
		return false
	}
	if g.active < 0 || g.active >= len(g.tabs) {
		g.active = 0
	}
	m.term.active = group
	m.invalidate()
	return true
}

// MoveTermTab swaps a tab with its left or right neighbour while keeping the
// same session active.
func (m *Model) MoveTermTab(group, tab, delta int) bool {
	if group < 0 || group >= len(m.term.groups) || delta == 0 {
		return false
	}
	g := &m.term.groups[group]
	target := tab + delta
	if tab < 0 || tab >= len(g.tabs) || target < 0 || target >= len(g.tabs) {
		return false
	}
	active := g.active
	if active < 0 || active >= len(g.tabs) {
		active = 0
	}
	g.tabs[tab], g.tabs[target] = g.tabs[target], g.tabs[tab]
	switch active {
	case tab:
		active = target
	case target:
		active = tab
	}
	g.active = active
	m.invalidate()
	return true
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

// maxTerminalInputQueue bounds writes waiting for one slow PTY. A terminal
// must either apply backpressure or report that input is not moving.
const maxTerminalInputQueue = 1 << 20

func (m *Model) resetTerm() { m.term = TermPane{}; m.termShell = nil }

// ShowTerm toggles the terminal; opening it starts a shell on first use.
func (m *Model) ShowTerm() {
	if !m.term.visible {
		m.autoSaveOnSwitch()
	}
	m.dockActivate("term")
}

// SpawnTerm starts the user's login shell in a tab, when the active one has no
// shell left to use.
func (m *Model) SpawnTerm() {
	root := m.Root()
	if root == "" {
		m.term.err = "Open a project before starting the terminal"
		m.invalidate()
		return
	}
	if s := m.term.current(); s != nil && (s.starting || s.id != 0) {
		return
	}
	m.spawnTermSession(root, m.terminalStartDirectory(root), m.term.active)
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
		m.term.err = "Open a project before starting the terminal"
		m.invalidate()
		return
	}
	if !m.term.visible {
		m.autoSaveOnSwitch()
		m.term.visible = true
		m.occupyBottom("term")
	}
	m.spawnTermSession(root, m.terminalStartDirectory(root), g)
}

// OpenTerminalAt opens a tab whose shell starts in dir, as OpenIDEA's Open in
// Terminal does for a project-tree folder.
func (m *Model) OpenTerminalAt(dir string) {
	root := m.Root()
	if root == "" || dir == "" {
		m.term.err = "Open a project before starting the terminal"
		m.invalidate()
		return
	}
	if !m.term.visible {
		m.autoSaveOnSwitch()
		m.term.visible = true
		m.occupyBottom("term")
	}
	m.spawnTermSession(root, dir, m.term.active)
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
		m.term.err = "Open a project before starting the terminal"
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
	m.spawnTermSession(root, m.terminalStartDirectory(root), 1)
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

// spawnTermSession adds a tab to group g and starts its login shell in cwd,
// which belongs to the project at root.
func (m *Model) spawnTermSession(root, cwd string, g int) {
	m.spawnTermSessionShell(root, cwd, g, "")
}

// spawnTermSessionShell starts a login shell (or shell) in cwd as a new tab.
func (m *Model) spawnTermSessionShell(root, cwd string, g int, shell string) {
	m.spawnTermProcess(root, cwd, g, shell, nil, "")
}

// spawnTermProcess opens one PTY tab. program is empty for the user's login
// shell, and name overrides the tab title until the backend reports one.
func (m *Model) spawnTermProcess(root, cwd string, g int, program string, args []string, name string) {
	opts := m.terminalSpawnOptions(program, args)
	opts["cwd"] = cwd
	if name == "" {
		name = m.terminalDefaultTabName()
	}
	m.spawnTerminalRequest(root, cwd, g, name, func(ctx context.Context, channel uint32) (json.RawMessage, error) {
		return m.mgr.Invoke(ctx, Window, "term_spawn", map[string]any{
			"options":  opts,
			"onOutput": map[string]any{"$channel": channel},
		})
	})
}

// spawnTerminalRequest owns the native terminal tab and its cleanup while a
// backend-specific start command creates the PTY.
func (m *Model) spawnTerminalRequest(
	root string,
	cwd string,
	g int,
	name string,
	spawn func(context.Context, uint32) (json.RawMessage, error),
) {
	ch := m.nextChannel()
	s := m.term.add(g)
	s.starting = true
	s.channel = ch
	s.cwd = cwd
	s.shell = name
	s.terminalCols = max(m.term.cols, 2)
	s.terminalRows = max(m.term.rows, 1)
	if m.term.cols <= 0 {
		s.terminalCols = defaultTerminalCols
	}
	if m.term.rows <= 0 {
		s.terminalRows = defaultTerminalRows
	}
	s.desiredCols, s.desiredRows = s.terminalCols, s.terminalRows
	s.ptyCols, s.ptyRows = s.desiredCols, s.desiredRows
	m.term.err = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := spawn(ctx, ch)
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
				m.term.err = "Starting the terminal failed: " + err.Error()
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
	if s == nil || s.id == 0 || !m.queueTerminalInput(s, line+"\r") {
		return
	}
	m.term.input = ""
	m.invalidate()
}

// queueTerminalInput serializes writes for one PTY. At most one RPC is in
// flight, so key repeat and paste cannot flood the backend with goroutines or
// overtake one another.
func (m *Model) queueTerminalInput(s *termSession, text string) bool {
	if s == nil || s.id == 0 || text == "" {
		return false
	}
	if len(s.writeQueue)+len(text) > maxTerminalInputQueue {
		m.term.err = "Terminal input is paused because the shell is not accepting writes"
		m.invalidate()
		return false
	}
	s.writeQueue = append(s.writeQueue, text...)
	m.flushTerminalInput(s)
	return true
}

func (m *Model) flushTerminalInput(s *termSession) {
	if s == nil || s.id == 0 || s.writing || len(s.writeQueue) == 0 {
		return
	}
	if m.mgr == nil && m.invokeHook == nil {
		return
	}
	const maxWrite = 64 << 10
	count := min(len(s.writeQueue), maxWrite)
	if count < len(s.writeQueue) {
		count = utf8RuneBoundary(s.writeQueue, count)
	}
	data := string(s.writeQueue[:count])
	s.writeQueue = append(s.writeQueue[:0], s.writeQueue[count:]...)
	id, channel := s.id, s.channel
	s.writing = true
	base := m.ctx
	if base == nil {
		base = context.Background()
	}
	go func() {
		ctx, cancel := context.WithTimeout(base, 10*time.Second)
		defer cancel()
		_, err := m.invoke(ctx, "term_write", map[string]any{
			"id": id, "data": data,
		})
		m.post(func() {
			session := m.term.byChannel(channel)
			if session == nil || session.id != id {
				return
			}
			session.writing = false
			if err != nil {
				m.term.err = "Writing to the terminal failed: " + err.Error()
			} else {
				m.flushTerminalInput(session)
			}
			m.invalidate()
		})
	}()
}

func utf8RuneBoundary(data []byte, at int) int {
	if at <= 0 || at >= len(data) {
		return at
	}
	for at > 0 && data[at]&0xc0 == 0x80 {
		at--
	}
	return at
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
	m.killTerminalSession(id, true)
}

func (m *Model) killTerminalSession(id uint32, report bool) {
	if id == 0 || m.mgr == nil {
		return
	}
	base := m.ctx
	if base == nil {
		base = context.Background()
	}
	go func() {
		ctx, cancel := context.WithTimeout(base, 10*time.Second)
		defer cancel()
		if _, err := m.mgr.Invoke(ctx, Window, "term_kill", map[string]any{"id": id}); err != nil && report {
			m.post(func() {
				m.term.err = "Killing the terminal failed: " + err.Error()
				m.invalidate()
			})
		}
	}()
}

// requestTerminalResize applies the session's latest measured size to its PTY.
// The call is asynchronous because the backend owns every terminal write.
func (m *Model) requestTerminalResize(s *termSession) {
	if s == nil || s.id == 0 || s.resizing {
		return
	}
	cols, rows := s.desiredCols, s.desiredRows
	if cols <= 0 {
		cols = max(s.terminalCols, defaultTerminalCols)
	}
	if rows <= 0 {
		rows = max(s.terminalRows, defaultTerminalRows)
	}
	if s.ptyCols == cols && s.ptyRows == rows {
		return
	}
	if m.mgr == nil && m.invokeHook == nil {
		s.applyTerminalSize(cols, rows)
		s.ptyCols, s.ptyRows = cols, rows
		return
	}
	id, channel, generation := s.id, s.channel, s.resizeGen
	s.resizing = true
	m.term.cols, m.term.rows = cols, rows
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
		defer cancel()
		_, err := m.invoke(ctx, "term_resize", map[string]any{
			"id": id, "cols": cols, "rows": rows,
		})
		m.post(func() {
			s := m.term.byChannel(channel)
			if s == nil || s.id != id {
				return
			}
			s.resizing = false
			if err != nil {
				m.term.err = "Resizing the terminal failed: " + err.Error()
			} else {
				s.ptyCols, s.ptyRows = cols, rows
				s.applyTerminalSize(cols, rows)
				if s.resizeGen != generation ||
					s.desiredCols != cols || s.desiredRows != rows {
					m.requestTerminalResize(s)
				}
			}
			m.invalidate()
		})
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
	m.killTerminalSession(tabs[ti].id, false)
	m.dropTermTab(g, ti)
}

func (m *Model) dropTermTab(g, ti int) {
	if g < 0 || g >= len(m.term.groups) {
		return
	}
	if ti < 0 || ti >= len(m.term.groups[g].tabs) {
		return
	}
	m.term.remove(g, ti)
	if m.term.total() == 0 {
		if dockSpec("term") != nil {
			m.DockHide("term")
		} else {
			m.hideTool("term")
		}
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

// CloseOtherTermTabs keeps one tab in its group and ends every sibling shell.
func (m *Model) CloseOtherTermTabs(group, keep int) bool {
	if group < 0 || group >= len(m.term.groups) {
		return false
	}
	g := &m.term.groups[group]
	if keep < 0 || keep >= len(g.tabs) {
		return false
	}
	if len(g.tabs) == 1 {
		return true
	}
	kept := g.tabs[keep]
	for index, session := range g.tabs {
		if index != keep {
			m.killTerminalSession(session.id, false)
		}
	}
	g.tabs = []termSession{kept}
	g.active = 0
	m.term.active = group
	m.invalidate()
	return true
}

// ClearTerm empties the active tab's scrollback.
func (m *Model) ClearTerm() {
	if s := m.term.current(); s != nil {
		s.lines = nil
		s.truncated = false
	}
	m.invalidate()
}

// ShowTermFind opens the scrollback search bar over the terminal, as OpenIDEA's
// Find in Terminal does.
func (m *Model) ShowTermFind() {
	if !m.term.visible {
		m.term.visible = true
		m.occupyBottom("term")
	}
	s := m.term.current()
	if s == nil {
		m.invalidate()
		return
	}
	s.finding = true
	m.invalidate()
}

// HideTermFind closes the bar and drops its matches.
func (m *Model) HideTermFind() {
	if s := m.term.current(); s != nil {
		s.finding = false
		s.find = ""
		s.findAt = -1
		s.reveal = false
	}
	m.invalidate()
}

// FindTermStep jumps to the next (dir 1) or previous (dir -1) line holding the
// query, wrapping at both ends; the first step starts at the newest end's first
// match, or the top when going forward.
func (m *Model) FindTermStep(dir int) {
	s := m.term.current()
	if s == nil || s.find == "" {
		return
	}
	hits := termHits(s.lines, s.find)
	if len(hits) == 0 {
		s.findAt = -1
		s.reveal = false
		m.invalidate()
		return
	}
	at := -1
	for i, line := range hits {
		if line == s.findAt {
			at = i
			break
		}
	}
	switch {
	case at < 0 && dir < 0:
		at = len(hits) - 1
	case at < 0:
		at = 0
	default:
		at = (at + dir + len(hits)) % len(hits)
	}
	s.findAt = hits[at]
	s.reveal = true
	m.invalidate()
}

// termFindLabel is the bar's match position, "" before a query.
func (m *Model) termFindLabel() string {
	s := m.term.current()
	if s == nil || s.find == "" {
		return ""
	}
	hits := termHits(s.lines, s.find)
	if len(hits) == 0 {
		return "No matches"
	}
	at := 0
	for i, line := range hits {
		if line == s.findAt {
			at = i + 1
		}
	}
	if at == 0 {
		return fmt.Sprintf("%d", len(hits))
	}
	return fmt.Sprintf("%d/%d", at, len(hits))
}

// termHits lists the lines containing q, ignoring case.
func termHits(lines []string, q string) []int {
	if q == "" {
		return nil
	}
	needle := strings.ToLower(q)
	var hits []int
	for i, line := range lines {
		if strings.Contains(strings.ToLower(line), needle) {
			hits = append(hits, i)
		}
	}
	return hits
}

// write appends terminal output and applies the cursor and alternate-screen
// operations used by progress displays and full-screen programs.
func (p *termSession) write(text string) {
	p.writeTerminal(text)
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
	if strings.Contains(text, "\a") && m.terminalPlaysBell() {
		mygo.Shell.Beep()
	}
	s.write(text)
	// A pane that showed the end follows new output; one scrolled up keeps
	// its place until the user comes back to the end.
	if s.scroll.Y >= s.scroll.MaxY {
		s.scroll.Y = math.MaxFloat32
	}
	m.invalidate()
}

func (m *Model) acceptTermExit(id uint32, code *int32) {
	s := m.term.byID(id)
	if s == nil {
		return
	}
	if m.terminalClosesOnExit() {
		for gi := range m.term.groups {
			for ti := range m.term.groups[gi].tabs {
				if m.term.groups[gi].tabs[ti].id == id {
					m.dropTermTab(gi, ti)
					return
				}
			}
		}
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
	initialCols, initialRows := defaultTerminalCols, defaultTerminalRows
	if m.term.cols > 0 {
		initialCols = m.term.cols
	}
	if m.term.rows > 0 {
		initialRows = m.term.rows
	}
	root := m.Root()
	if root == "" {
		return out, errors.New("No project is open")
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
	// The shells the New Terminal Tab picker offers; the smoke starts one of
	// them through the same spawn options the picker uses.
	var chosen string
	var shells []string
	if rawShells, err := m.mgr.Invoke(ctx, Window, "term_available_shells", nil); err != nil {
		out["shellsError"] = err.Error()
	} else if json.Unmarshal(rawShells, &shells) != nil {
		out["shellsError"] = "The shell list has an unexpected format"
	}
	out["shells"] = len(shells)
	if len(shells) > 0 {
		chosen = shells[0]
		out["chosenShell"] = filepath.Base(chosen)
	}
	m.post(func() {
		m.term = TermPane{visible: true}
		// The smoke supplies its own shell: register it before the window
		// opens, so the dock's opener has nothing to spawn.
		s := m.term.add(0)
		s.starting = true
		s.channel = ch
		s.terminalCols, s.terminalRows = initialCols, initialRows
		s.desiredCols, s.desiredRows = initialCols, initialRows
		s.ptyCols, s.ptyRows = initialCols, initialRows
		m.DockShow("term")
		m.invalidate()
	})
	spawnOpts := map[string]any{
		"cwd": root, "args": []string{}, "env": map[string]string{},
		"cols": initialCols, "rows": initialRows, "shellIntegration": false,
	}
	if chosen != "" {
		spawnOpts["shell"] = chosen
	}
	raw, err := m.mgr.Invoke(ctx, Window, "term_spawn", map[string]any{
		"options":  spawnOpts,
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
		s.terminalCols, s.terminalRows = initialCols, initialRows
		s.ptyCols, s.ptyRows = initialCols, initialRows
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
	// Resizing the real PTY changes stty's rows and columns. The unique
	// markers keep prompt text from looking like a successful size.
	sizeCommand := "printf 'GOIDEA_SIZE=%s\\n' \"$(stty size)\"\r"
	if _, err := m.mgr.Invoke(ctx, Window, "term_write", map[string]any{
		"id": info.ID, "data": sizeCommand,
	}); err != nil {
		return out, err
	}
	expectedInitial := fmt.Sprintf("GOIDEA_SIZE=%d %d", initialRows, initialCols)
	_, initialSizeOK, err := m.waitTermSmokeText(
		ctx, ch, expectedInitial, 10*time.Second,
	)
	if err != nil {
		return out, err
	}
	out["initialSize"] = expectedInitial
	out["initialSizeObserved"] = initialSizeOK
	resizeCols, resizeRows := 42, 17
	if _, err := m.mgr.Invoke(ctx, Window, "term_resize", map[string]any{
		"id": info.ID, "cols": resizeCols, "rows": resizeRows,
	}); err != nil {
		return out, err
	}
	m.post(func() {
		if session := m.term.byChannel(ch); session != nil {
			session.applyTerminalSize(resizeCols, resizeRows)
			session.ptyCols, session.ptyRows = resizeCols, resizeRows
			session.desiredCols, session.desiredRows = resizeCols, resizeRows
		}
	})
	if _, err := m.mgr.Invoke(ctx, Window, "term_write", map[string]any{
		"id":   info.ID,
		"data": "printf 'GOIDEA_SIZE_AFTER=%s\\n' \"$(stty size)\"\r",
	}); err != nil {
		return out, err
	}
	expectedResized := fmt.Sprintf("GOIDEA_SIZE_AFTER=%d %d", resizeRows, resizeCols)
	_, resizedOK, err := m.waitTermSmokeText(
		ctx, ch, expectedResized, 10*time.Second,
	)
	if err != nil {
		return out, err
	}
	out["resizedSize"] = expectedResized
	out["resize"] = resizedOK

	// The full-screen fixture enters the alternate screen through the real
	// PTY. Its final spinner proves repeated frame output reached the parser.
	const tuiFrames = 12
	var tui strings.Builder
	tui.WriteString(sampleTUIStart)
	for frame := 0; frame < tuiFrames; frame++ {
		tui.WriteString(sampleTUIFrame(frame, resizeCols, resizeRows))
	}
	lastSpinner := []rune(sampleTUISpinner)[(tuiFrames-1)%len([]rune(sampleTUISpinner))]
	if _, err := m.mgr.Invoke(ctx, Window, "term_write", map[string]any{
		"id": info.ID, "data": "printf %s " + terminalShellQuote(tui.String()) + "\r",
	}); err != nil {
		return out, err
	}
	var alternate bool
	var tuiText string
	tuiDeadline := time.Now().Add(10 * time.Second)
	for {
		m.post(func() {
			if session := m.term.byChannel(ch); session != nil {
				alternate = session.alternate
				tuiText = session.text()
			}
		})
		if alternate &&
			strings.Contains(tuiText, string(lastSpinner)+" Thinking") &&
			strings.Contains(tuiText, "Enter:send") {
			break
		}
		if time.Now().After(tuiDeadline) {
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	out["tuiFrames"] = tuiFrames
	out["tuiAlternate"] = alternate
	out["tuiComplete"] = alternate &&
		strings.Contains(tuiText, string(lastSpinner)+" Thinking") &&
		strings.Contains(tuiText, "Enter:send")
	if _, err := m.mgr.Invoke(ctx, Window, "term_write", map[string]any{
		"id": info.ID, "data": "printf %s " + terminalShellQuote(sampleTUIEnd) + "\r",
	}); err != nil {
		return out, err
	}
	// Find in Terminal marks the line that holds the query.
	m.post(func() {
		m.ShowTermFind()
		if s := m.term.current(); s != nil {
			s.find = "GOIDEA_TERM_OK"
		}
		m.FindTermStep(1)
	})
	var findAt, findHits int
	var findLabel string
	m.post(func() {
		if s := m.term.current(); s != nil {
			findAt = s.findAt
			findHits = len(termHits(s.lines, "GOIDEA_TERM_OK"))
			findLabel = m.termFindLabel()
		}
	})
	out["findAt"] = findAt
	out["findHits"] = findHits
	out["findLabel"] = findLabel
	m.post(func() { m.HideTermFind() })
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

func (m *Model) waitTermSmokeText(
	ctx context.Context,
	channel uint32,
	marker string,
	timeout time.Duration,
) (string, bool, error) {
	deadline := time.Now().Add(timeout)
	var text string
	for {
		m.post(func() {
			if session := m.term.byChannel(channel); session != nil {
				text = session.text()
			}
		})
		if strings.Contains(text, marker) {
			return text, true, nil
		}
		if time.Now().After(deadline) {
			return text, false, nil
		}
		select {
		case <-ctx.Done():
			return text, false, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func terminalShellQuote(text string) string {
	return "'" + strings.ReplaceAll(text, "'", "'\\''") + "'"
}

// text is the scrollback, one line per entry.
func (p *termSession) text() string {
	return strings.Join(p.lines, "\n")
}

// termTabTitle is the label shown in a terminal tab strip.
func termTabTitle(session *termSession, index int) string {
	if session == nil {
		return ""
	}
	label := strings.TrimSpace(session.customName)
	if label == "" {
		label = strings.TrimSpace(session.shell)
	}
	if label == "" {
		label = fmt.Sprintf("Terminal %d", index+1)
	}
	switch {
	case session.starting:
		return "Starting…"
	case session.id == 0 && session.ended:
		return label + " (ended)"
	default:
		return label
	}
}

// termShellPicker is the open shell list of New Terminal Tab.
type termShellPicker struct {
	open    bool
	loading bool
	err     string
	shells  []string
	sel     int
	gen     uint64
}

// OpenTermShellPicker lists the shells a new terminal tab can run.
func (m *Model) OpenTermShellPicker() {
	if m.termShell == nil {
		m.termShell = &termShellPicker{open: true}
		m.loadTermShells()
		return
	}
	m.termShell.open = true
	if len(m.termShell.shells) == 0 && !m.termShell.loading {
		m.loadTermShells()
	}
	m.invalidate()
}

// loadTermShells asks the backend for the installed login shells.
func (m *Model) loadTermShells() {
	p := m.termShell
	if p == nil || p.loading {
		return
	}
	p.loading = true
	p.err = ""
	p.gen++
	gen := p.gen
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 20*time.Second)
		defer cancel()
		raw, err := m.mgr.Invoke(ctx, Window, "term_available_shells", nil)
		var shells []string
		if err == nil {
			err = json.Unmarshal(raw, &shells)
		}
		m.post(func() {
			if m.termShell != p || p.gen != gen {
				return
			}
			p.loading = false
			if err != nil {
				p.err = "Could not read the shell list: " + err.Error()
			} else {
				p.shells = shells
			}
			m.invalidate()
		})
	}()
}

// StepTermShell moves the shell selection, wrapping around.
func (m *Model) StepTermShell(delta int) {
	p := m.termShell
	if p == nil || len(p.shells) == 0 {
		return
	}
	p.sel = (p.sel + delta + len(p.shells)) % len(p.shells)
	m.invalidate()
}

// ChooseTermShell opens a tab running the selected shell.
func (m *Model) ChooseTermShell(i int) {
	p := m.termShell
	if p == nil || i < 0 || i >= len(p.shells) {
		return
	}
	shell := p.shells[i]
	m.closeTermShell()
	root := m.Root()
	m.spawnTermSessionShell(root, root, m.term.active, shell)
}

// closeTermShell drops the shell list.
func (m *Model) closeTermShell() { m.termShell = nil; m.invalidate() }

// termShellLabel names a shell in the list; duplicate names carry their path.
func termShellLabel(shells []string, i int) string {
	base := filepath.Base(shells[i])
	dup := false
	for j, s := range shells {
		if j != i && filepath.Base(s) == base {
			dup = true
			break
		}
	}
	if dup {
		return base + " (" + shells[i] + ")"
	}
	return base
}

// termShellDialog draws the shell list.
func (m *Model) termShellDialog(c *ui.Context, p *ui.Theme) {
	pk := m.termShell
	if pk == nil {
		return
	}
	open := pk.open
	jump := -1
	nativeModal(c, &open, func() {
		ui.Column(c).Width(480).Gap(8).Children(func() {
			ui.Text(c, "New Terminal Tab").FontSize(13).Bold().TextColor(p.Text).SingleLine()
			switch {
			case pk.loading:
				ui.Text(c, "Reading the shells…").FontSize(12).TextColor(p.TextMuted).SingleLine()
			case pk.err != "":
				ui.Text(c, pk.err).FontSize(12).TextColor(p.Danger).SingleLine().Ellipsis("…")
			case len(pk.shells) == 0:
				ui.Text(c, "No usable shell was found.").FontSize(12).TextColor(p.TextMuted).SingleLine()
			}
			for i := range pk.shells {
				i := i
				row := ui.Row(c.Key(fmt.Sprintf("termshell:%d", i))).Height(24).Padding(0, 8).
					AlignItems(ui.Center)
				if i == pk.sel || row.Hovered() {
					row.Background(p.SurfaceHover)
				}
				row.Children(func() {
					ui.Text(c, termShellLabel(pk.shells, i)).FontSize(12).TextColor(p.Text).
						SingleLine().Ellipsis("…")
				})
				if row.Clicked() {
					jump = i
					pk.sel = i
				}
			}
		})
	})
	if !open && m.termShell != nil {
		m.closeTermShell()
	}
	if jump >= 0 {
		m.ChooseTermShell(jump)
	}
}
