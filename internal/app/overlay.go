package app

// The overlay manager after OpenIDEA's overlay layer: every sheet, popup and
// picker is registered once with an id, a layer, whether it owns the keyboard
// and a render function. The workbench asks the registry which overlays are
// open and draws them in priority order, so the View no longer keeps a list
// that drifts from the fields it reads.

import (
	"slices"
	"sort"

	"github.com/egoist/mygo/ui"
)

// Overlay layers, bottom to top.
const (
	overlayLayerMenu = iota
	overlayLayerPopup
	overlayLayerDialog
	overlayLayerNotification
)

// Overlay is one registered sheet.
type Overlay struct {
	ID string
	// Layer orders overlays drawn over the workbench; higher is nearer the
	// front.
	Layer int
	// Priority orders overlays inside a layer, and is what an equal layer
	// falls back to.
	Priority int
	// Modal overlays own the keyboard while open: the shortcut chain stops
	// at them.
	Modal bool
	// Open reports the business state that keeps this overlay up.
	Open func(m *Model) bool
	// Render draws it; the feature owns the dialog, the manager only calls
	// it in order.
	Render func(m *Model, c *ui.Context, p *ui.Theme)
}

type overlayRegistry struct {
	byID map[string]*Overlay
	list []*Overlay
}

var overlays = overlayRegistry{byID: map[string]*Overlay{}}

// registerOverlay adds an overlay; a duplicate id is a programming error.
func registerOverlay(o *Overlay) {
	if o == nil || o.ID == "" || o.Open == nil || o.Render == nil {
		return
	}
	if _, ok := overlays.byID[o.ID]; ok {
		return
	}
	overlays.byID[o.ID] = o
	overlays.list = append(overlays.list, o)
}

func unregisterOverlay(id string) bool {
	if _, ok := overlays.byID[id]; !ok {
		return false
	}
	delete(overlays.byID, id)
	for i, o := range overlays.list {
		if o.ID == id {
			overlays.list = append(overlays.list[:i], overlays.list[i+1:]...)
			break
		}
	}
	return true
}

// overlayOrder is the registry sorted back to front: layer, then priority,
// then registration order for ties (sort.SliceStable).
func overlayOrder() []*Overlay {
	list := append([]*Overlay(nil), overlays.list...)
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].Layer != list[j].Layer {
			return list[i].Layer < list[j].Layer
		}
		return list[i].Priority < list[j].Priority
	})
	return list
}

// openOverlays lists the overlays that are up, back to front.
func (m *Model) openOverlays() []*Overlay {
	var out []*Overlay
	for _, o := range overlayOrder() {
		if o.Open(m) {
			out = append(out, o)
		}
	}
	return out
}

// renderOverlays draws every open overlay in registry order and hands the
// keyboard back to the editor when the last one closes.
func (m *Model) renderOverlays(c *ui.Context, p *ui.Theme) {
	open := m.openOverlays()
	ids := make([]string, 0, len(open))
	modal := false
	for _, o := range open {
		ids = append(ids, o.ID)
		modal = modal || o.Modal
		o.Render(m, c, p)
	}
	m.overlayFocusSettle(ids, modal)
}

// overlayFocusSettle restores the editor's keyboard when the last overlay
// closed, as OpenIDEA does after a popup: the editor focuses itself on the
// next frame. A modal still up keeps the focus where it is.
func (m *Model) overlayFocusSettle(ids []string, modal bool) {
	closed := false
	for _, was := range m.overlayIDs {
		if !slices.Contains(ids, was) {
			closed = true
			break
		}
	}
	m.overlayIDs = append([]string(nil), ids...)
	if closed && !modal {
		m.focusApply = true
	}
}

// overlayOpen reports whether any overlay is up, which keeps the workbench's
// shortcuts from acting underneath one.
func (m *Model) overlayOpen() bool { return len(m.openOverlays()) > 0 }

// modalOverlayOpen reports whether an overlay that owns the keyboard is up.
func (m *Model) modalOverlayOpen() bool {
	for _, o := range m.openOverlays() {
		if o.Modal {
			return true
		}
	}
	return false
}
