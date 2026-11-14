package app

// The status bar after OpenIDEA's StatusItemContribution: every item on the
// bar is registered once with an id, a side, an order and a render function,
// and the bar draws whatever the registry currently holds. A feature or plugin
// contributes an item instead of the status bar growing another branch.

import (
	"sort"

	"github.com/egoist/mygo/ui"
)

// StatusItem is one contribution to the status bar.
type StatusItem struct {
	ID string
	// Side is "left" or "right".
	Side string
	// Order sorts items inside the side; ties keep registration order.
	Order int
	// Visible hides the item in contexts where it has nothing to say.
	Visible func(m *Model) bool
	Render  func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors)
}

type statusRegistry struct {
	items []*StatusItem
}

var statusItems = statusRegistry{}

// registerStatusItem adds an item; a duplicate id is a programming error and
// is ignored, as the action registry rejects one.
func registerStatusItem(it *StatusItem) {
	if it == nil || it.ID == "" || it.Render == nil {
		return
	}
	for _, old := range statusItems.items {
		if old.ID == it.ID {
			return
		}
	}
	statusItems.items = append(statusItems.items, it)
}

// unregisterStatusItem removes an item, as a plugin's teardown does.
func unregisterStatusItem(id string) bool {
	for i, it := range statusItems.items {
		if it.ID == id {
			statusItems.items = append(statusItems.items[:i], statusItems.items[i+1:]...)
			return true
		}
	}
	return false
}

// statusSide is the visible items of one side, back to front: order, then
// registration order for ties.
func (m *Model) statusSide(side string) []*StatusItem {
	var out []*StatusItem
	for _, it := range statusItems.items {
		if it.Side != side {
			continue
		}
		if it.Visible != nil && !it.Visible(m) {
			continue
		}
		out = append(out, it)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Order < out[j].Order })
	return out
}
