package app

// The status bar after OpenIDEA's StatusItemContribution: every item on the
// bar is registered once with an id, a side, an order and a render function,
// and the bar draws whatever the registry currently holds. A feature or plugin
// contributes an item instead of the status bar growing another branch.

import (
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"
)

// StatusItem is one contribution to the status bar.
type StatusItem struct {
	ID string
	// Title names a widget in View › Appearance › Status Bar Widgets and in
	// the status bar's context menu. Untitled transient items stay visible and
	// cannot be switched off, as OpenIDEA's untitled contributions do.
	Title string
	// HiddenByDefault omits the widget from a fresh settings file. OpenIDEA's
	// defaultVisible=false contributions use it.
	HiddenByDefault bool
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

// updateStatusItemTitle follows a contribution whose displayed title changes
// while it stays registered, as a plugin status item does when its text or
// tooltip changes.
func updateStatusItemTitle(id, title string) bool {
	title = strings.TrimSpace(title)
	if id == "" || title == "" {
		return false
	}
	for _, it := range statusItems.items {
		if it.ID == id {
			it.Title = title
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
		if !m.statusItemEnabled(it) {
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

// statusItemEnabled applies the user's appearance.statusBarWidgets override,
// or the contribution's default when the widget has not been changed.
func (m *Model) statusItemEnabled(it *StatusItem) bool {
	if it == nil {
		return false
	}
	if visible, ok := m.prefs.Appearance.StatusBarWidgets[it.ID]; ok {
		return visible
	}
	return !it.HiddenByDefault
}

// statusWidgetRows lists the titled widgets in bar order. It includes widgets
// that are contextually hidden so users can set their preference before the
// matching file, run or VCS state appears.
func (m *Model) statusWidgetRows() []*StatusItem {
	var out []*StatusItem
	seen := map[string]bool{}
	add := func(items []*StatusItem) {
		for _, it := range items {
			if it == nil || it.Title == "" || seen[it.ID] {
				continue
			}
			seen[it.ID] = true
			out = append(out, it)
		}
	}
	add(m.statusSideWithoutOverrides("left"))
	add(m.statusSideWithoutOverrides("right"))
	return out
}

func (m *Model) statusSideWithoutOverrides(side string) []*StatusItem {
	var out []*StatusItem
	for _, it := range statusItems.items {
		if it.Side == side {
			out = append(out, it)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Order < out[j].Order })
	return out
}

// toggleStatusBarWidget flips one named widget and persists the whole map.
func (m *Model) toggleStatusBarWidget(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	var target *StatusItem
	for _, it := range statusItems.items {
		if it.ID == id && it.Title != "" {
			target = it
			break
		}
	}
	if target == nil {
		return false
	}
	visible := m.statusItemEnabled(target)
	m.SetStatusBarWidgetVisible(id, !visible)
	return true
}

// SetStatusBarWidgetVisible stores one status bar widget choice. The map keeps
// ids with dots intact because it is stored as a JSON object rather than as
// nested settings keys.
func (m *Model) SetStatusBarWidgetVisible(id string, visible bool) {
	if strings.TrimSpace(id) == "" {
		return
	}
	if m.prefs.Appearance.StatusBarWidgets == nil {
		m.prefs.Appearance.StatusBarWidgets = map[string]bool{}
	}
	m.prefs.Appearance.StatusBarWidgets[id] = visible
	m.savePreferences()
	m.invalidate()
}
