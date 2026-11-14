package app

// The dock after OpenIDEA's dock manager: six areas that each hold at most one
// tool window, a registry that describes every window instead of a list of
// names with switches, and a versioned layout record. A window's slot is the
// only authority on where it shows; the old per-window visibility flags are
// mirrors kept for the code that still reads them.

import (
	"github.com/egoist/mygo/ui"
)

type dockSlot int

const (
	dockLeft dockSlot = iota
	dockLeftSplit
	dockBottom
	dockBottomSplit
	dockRight
	dockRightSplit
	dockSlotCount
)

var dockSlotNames = [dockSlotCount]string{
	"left", "leftSplit", "bottom", "bottomSplit", "right", "rightSplit",
}

func (s dockSlot) valid() bool { return s >= 0 && s < dockSlotCount }

// dockSlotOfName is the layout key of a slot, and the reverse.
func dockSlotOfName(name string) (dockSlot, bool) {
	for i, n := range dockSlotNames {
		if n == name {
			return dockSlot(i), true
		}
	}
	return 0, false
}

// toolWindowSpec is one registered tool window: what OpenIDEA's
// registerToolWindow takes, minus the parts GoIDEA has no host for (header
// menu contributions arrive with the features that need them).
type toolWindowSpec struct {
	id     string
	title  string
	anchor dockSlot
	order  int
	stripe bool
	// open puts the window's content up (loading included); close takes it
	// down but keeps its data, as a hidden terminal keeps its shell.
	open   func(m *Model)
	close  func(m *Model)
	render func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors)
	shown  func(m *Model) bool
}

// toolWindowRegistry is every registered window by id.
var toolWindowRegistry = map[string]*toolWindowSpec{}

// toolWindowOrder keeps registration order for strays and tests.
var toolWindowOrder []string

// registerToolWindow adds a window; a second registration of the same id is a
// programming error and wins, the way upstream's registry replaces entries.
func registerToolWindow(spec *toolWindowSpec) {
	if _, ok := toolWindowRegistry[spec.id]; !ok {
		toolWindowOrder = append(toolWindowOrder, spec.id)
	}
	toolWindowRegistry[spec.id] = spec
}

// dockState is the six slots and what the workbench remembers between runs.
type dockState struct {
	slots [dockSlotCount]string
	// active is the window shortcuts act on, last is the use order (newest
	// last) for F12 and Hide All's restore.
	active string
	last   []string
	// hidden holds the slots Hide All emptied, so a second press restores
	// exactly that arrangement.
	hidden       [dockSlotCount]string
	hiddenActive string
	hiddenMax    bool
	hasHidden    bool
	max          bool
}

// dockSpec returns a registered window.
func dockSpec(id string) *toolWindowSpec { return toolWindowRegistry[id] }

// dockWindowShown reports whether a registered window sits in a slot and its
// content is up.
func (m *Model) dockWindowShown(id string) bool {
	if _, ok := m.dockSlotOf(id); !ok {
		return false
	}
	spec := dockSpec(id)
	return spec != nil && (spec.shown == nil || spec.shown(m))
}

// dockSlotOf is the slot holding id, for registered windows only.
func (m *Model) dockSlotOf(id string) (dockSlot, bool) {
	for s := dockSlot(0); s < dockSlotCount; s++ {
		if m.dock.slots[s] == id {
			return s, true
		}
	}
	return 0, false
}

// dockOccupant is the window a slot shows, "" when empty.
func (m *Model) dockOccupant(s dockSlot) string {
	if !s.valid() {
		return ""
	}
	return m.dock.slots[s]
}

// dockVisibleSlot reports whether a side draws at all: the left side draws
// while either of its slots is occupied or the primary area is shown, the
// right side while either right slot is occupied.
func (m *Model) dockRightShown() bool {
	return m.dock.slots[dockRight] != "" || m.dock.slots[dockRightSplit] != ""
}

// dockTouch records a window as the active and most recently used one.
func (m *Model) dockTouch(id string) {
	m.dock.active = id
	for i, used := range m.dock.last {
		if used == id {
			m.dock.last = append(m.dock.last[:i], m.dock.last[i+1:]...)
			break
		}
	}
	m.dock.last = append(m.dock.last, id)
	if len(m.dock.last) > 12 {
		m.dock.last = m.dock.last[len(m.dock.last)-12:]
	}
}

// dockOpenContent opens a window's content through its registered opener.
func (m *Model) dockOpenContent(id string) {
	if spec := dockSpec(id); spec != nil && spec.open != nil {
		spec.open(m)
	}
}

// dockCloseContent closes a window's content, keeping its data.
func (m *Model) dockCloseContent(id string) {
	if spec := dockSpec(id); spec != nil && spec.close != nil {
		spec.close(m)
	}
}

// dockArea is the side an area's slot lives on: "left", "bottom" or "right".
func dockArea(s dockSlot) string {
	switch s {
	case dockLeft, dockLeftSplit:
		return "left"
	case dockBottom, dockBottomSplit:
		return "bottom"
	}
	return "right"
}

// dockEvictArea makes room in one slot: its registered occupant closes and the
// legacy windows that lived in the same area of the bottom side close too. The
// other slot of the side keeps its window, as OpenIDEA's six slots allow two
// windows per side. keep is the window moving in, which is left alone.
func (m *Model) dockEvictArea(s dockSlot, keep string) {
	if id := m.dockOccupant(s); id != "" && id != keep {
		m.dockCloseContent(id)
		m.dock.slots[s] = ""
	}
	if dockArea(s) != "bottom" {
		return
	}
	for _, name := range toolWindows {
		if dockSpec(name) != nil || name == keep {
			continue
		}
		if m.toolVisible(name) && m.toolArea(name) == areaName(s) {
			m.hideTool(name)
		}
	}
}

// areaName is the legacy "primary"/"split" name of a bottom slot.
func areaName(s dockSlot) string {
	if s == dockBottomSplit {
		return "split"
	}
	return "primary"
}

// dockCloseOccupant empties a slot, closing what it held.
func (m *Model) dockCloseOccupant(s dockSlot) {
	if id := m.dockOccupant(s); id != "" {
		m.dockCloseContent(id)
		m.dock.slots[s] = ""
	}
}

// DockShow shows a registered window in its home slot (or where it already
// sits), taking that slot from whatever held it. The content opens even when
// it moves areas, so no slot is ever blank.
func (m *Model) DockShow(id string) {
	spec := dockSpec(id)
	if spec == nil {
		return
	}
	if _, ok := m.dockSlotOf(id); ok {
		m.dockOpenContent(id)
		m.dockTouch(id)
		m.dockSyncMirrors()
		m.rememberLayout()
		m.invalidate()
		return
	}
	release := m.holdLayout()
	m.dockEvictArea(spec.anchor, id)
	m.dockOpenContent(id)
	m.dock.slots[spec.anchor] = id
	m.dockTouch(id)
	release()
	m.dockSyncMirrors()
	m.rememberLayout()
	m.invalidate()
}

// DockHide takes a registered window down and empties its slot.
func (m *Model) DockHide(id string) {
	s, ok := m.dockSlotOf(id)
	if !ok {
		m.dockCloseContent(id)
		m.dockSyncMirrors()
		m.invalidate()
		return
	}
	m.dockCloseContent(id)
	m.dock.slots[s] = ""
	if m.maximized && m.visibleTool() == "" {
		m.maximized = false
	}
	if m.dock.active == id {
		m.dock.active = m.dockNewestShown("")
	}
	m.dockSyncMirrors()
	m.rememberLayout()
	m.invalidate()
}

// DockToggle shows a window or hides it when it already shows.
func (m *Model) DockToggle(id string) {
	if m.dockWindowShown(id) {
		m.DockHide(id)
		return
	}
	m.DockShow(id)
}

// DockMove moves a shown window into another slot, opening its content there
// and closing the slot's previous window. Moving a window that is not shown
// shows it in the target slot.
func (m *Model) DockMove(id string, target dockSlot) {
	if !target.valid() || dockSpec(id) == nil {
		return
	}
	from, shown := m.dockSlotOf(id)
	if shown && from == target {
		return
	}
	release := m.holdLayout()
	m.dockEvictArea(target, id)
	if shown {
		// The content stays up: only its slot changes.
		m.dock.slots[from] = ""
	} else {
		m.dockOpenContent(id)
	}
	m.dock.slots[target] = id
	m.dockTouch(id)
	release()
	m.dockSyncMirrors()
	m.rememberLayout()
	m.invalidate()
}

// dockNewestShown is the most recently used window still shown, ignoring id.
func (m *Model) dockNewestShown(ignore string) string {
	for i := len(m.dock.last) - 1; i >= 0; i-- {
		id := m.dock.last[i]
		if id != ignore && m.dockWindowShown(id) {
			return id
		}
	}
	return ""
}

// dockSyncMirrors copies the slot placement into the legacy fields the rest of
// the app still reads: rightTool/rightSplitTool, the bottom's splitNames, and
// each window's own visible flag is set by its open/close adapters.
func (m *Model) dockSyncMirrors() {
	m.rightTool = m.dock.slots[dockRight]
	m.rightSplitTool = m.dock.slots[dockRightSplit]
	m.rightHidden = !m.dockRightShown()
	if id := m.dock.slots[dockLeftSplit]; id != "" {
		m.leftSplit = id
	}
	if m.splitNames == nil {
		m.splitNames = map[string]bool{}
	}
	for id := range m.splitNames {
		if dockSpec(id) != nil {
			delete(m.splitNames, id)
		}
	}
	if id := m.dock.slots[dockBottomSplit]; id != "" {
		m.splitNames[id] = true
	}
	if id := m.dock.slots[dockBottom]; id != "" {
		m.tool = id
		delete(m.splitNames, id)
	} else if id := m.dock.slots[dockBottomSplit]; id != "" && m.tool != id {
		m.tool = id
	}
	// tool is the last-used mark F12 reopens; a hidden window keeps it, as
	// OpenIDEA's lastUsed does.
}

// dockHideAll stashes every occupied slot, closes the windows and remembers
// the arrangement.
func (m *Model) dockHideAll() {
	m.dock.hidden = m.dock.slots
	m.dock.hiddenActive = m.dock.active
	m.dock.hiddenMax = m.dock.max
	m.dock.hasHidden = true
	release := m.holdLayout()
	for s := dockSlot(0); s < dockSlotCount; s++ {
		m.dockCloseOccupant(s)
	}
	release()
	m.dockSyncMirrors()
	m.rememberLayout()
	m.invalidate()
}

// dockRestoreAll reopens the stashed arrangement, maximized again if it was.
func (m *Model) dockRestoreAll() {
	stash := m.dock.hidden
	release := m.holdLayout()
	for s := dockSlot(0); s < dockSlotCount; s++ {
		id := stash[s]
		if id == "" {
			continue
		}
		m.dockCloseOccupant(s)
		m.dockOpenContent(id)
		m.dock.slots[s] = id
	}
	m.dock.active = m.dock.hiddenActive
	m.dock.max = m.dock.hiddenMax
	m.dock.hidden = [dockSlotCount]string{}
	m.dock.hiddenActive = ""
	m.dock.hasHidden = false
	release()
	m.dockSyncMirrors()
	m.rememberLayout()
	m.invalidate()
}

// DockLayout is the versioned dock record a project keeps. Slots are window
// ids; the missing ones are empty. Hidden holds the arrangement Hide All put
// away.
type DockLayout struct {
	Version int               `json:"version"`
	Slots   map[string]string `json:"slots,omitempty"`
	Active  string            `json:"active,omitempty"`
	Last    []string          `json:"last,omitempty"`
	Hidden  map[string]string `json:"hidden,omitempty"`
	Max     bool              `json:"maximized,omitempty"`
}

const dockLayoutVersion = 2

// currentDockLayout reads the dock for saving, including slots that legacy
// windows still fill.
func (m *Model) currentDockLayout() *DockLayout {
	l := &DockLayout{Version: dockLayoutVersion, Active: m.dock.active,
		Last: append([]string(nil), m.dock.last...), Max: m.dock.max}
	put := func(name, id string) {
		if id == "" {
			return
		}
		if l.Slots == nil {
			l.Slots = map[string]string{}
		}
		l.Slots[name] = id
	}
	put("left", m.sideMode)
	put("leftSplit", m.leftSplit)
	put("bottom", m.primaryTool())
	put("bottomSplit", m.splitTool())
	put("right", m.rightTool)
	put("rightSplit", m.rightSplitTool)
	if m.dock.hasHidden {
		l.Hidden = map[string]string{}
		for s := dockSlot(0); s < dockSlotCount; s++ {
			if id := m.dock.hidden[s]; id != "" {
				l.Hidden[dockSlotNames[s]] = id
			}
		}
	}
	if len(l.Slots) == 0 && len(l.Last) == 0 && !l.Max {
		l.Slots = map[string]string{}
	}
	return l
}

// applyDockLayout restores a versioned record: every slot's window opens its
// content and lands where the record says. Legacy windows keep their old
// fields, which the caller fills from the same record.
func (m *Model) applyDockLayout(d *DockLayout) {
	m.dock = dockState{}
	if d == nil {
		return
	}
	for name, id := range d.Slots {
		s, ok := dockSlotOfName(name)
		if !ok || id == "" {
			continue
		}
		if s == dockLeft {
			m.sideMode = id
			continue
		}
		if s == dockLeftSplit && dockSpec(id) == nil {
			m.leftSplit = id
			continue
		}
		if dockSpec(id) == nil {
			continue
		}
		m.dock.slots[s] = id
		m.dockOpenContent(id)
	}
	m.dock.active = d.Active
	m.dock.last = append([]string(nil), d.Last...)
	m.dock.max = d.Max && m.visibleTool() != ""
	for name, id := range d.Hidden {
		s, ok := dockSlotOfName(name)
		if !ok || id == "" || dockSpec(id) == nil {
			continue
		}
		m.dock.hidden[s] = id
		m.dock.hasHidden = true
	}
	m.dockSyncMirrors()
}
