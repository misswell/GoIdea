package app

// The title bar after OpenIDEA's widget contributions: the project, VCS and run
// widgets and the toolbar actions are registered once with an id, a default
// side and a render function, and the toolbar draws what the registry holds.
// A plugin adds a widget instead of the title bar growing another branch.

import "github.com/egoist/mygo/ui"

// TitleWidget is one contribution to the title bar. Action entries use the
// toolbar's "action:<id>" ids so the saved layout stays OpenIDEA's shape.
type TitleWidget struct {
	ID string
	// Title names the entry in Customize Main Toolbar.
	Title string
	// Side is the default side, "left" or "right"; empty means right.
	Side string
	// Visible hides the widget in contexts where it has nothing to say.
	Visible func(m *Model) bool
	Render  func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors)
}

type titleRegistry struct {
	items []*TitleWidget
}

var titleWidgets = titleRegistry{}

// registerTitleWidget adds a widget; a duplicate id is a programming error and
// is ignored, as the action and status registries reject one.
func registerTitleWidget(w *TitleWidget) {
	if w == nil || w.ID == "" || w.Render == nil {
		return
	}
	for _, old := range titleWidgets.items {
		if old.ID == w.ID {
			return
		}
	}
	titleWidgets.items = append(titleWidgets.items, w)
}

// unregisterTitleWidget removes a widget, as a plugin's teardown does.
func unregisterTitleWidget(id string) bool {
	for i, w := range titleWidgets.items {
		if w.ID == id {
			titleWidgets.items = append(titleWidgets.items[:i], titleWidgets.items[i+1:]...)
			return true
		}
	}
	return false
}

func titleWidgetFor(id string) *TitleWidget {
	for _, w := range titleWidgets.items {
		if w.ID == id {
			return w
		}
	}
	return nil
}

// titleWidgetSide is an entry's default side; unknown entries, including the
// "action:<id>" entries the customization adds, default to the right.
func titleWidgetSide(id string) string {
	if w := titleWidgetFor(id); w != nil && w.Side != "" {
		return w.Side
	}
	return "right"
}
