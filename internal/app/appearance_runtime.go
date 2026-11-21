package app

import "fmt"

// ideZoomSteps are OpenIDEA's View › Appearance › Zoom IDE stops.
var ideZoomSteps = [...]int{50, 67, 75, 80, 90, 100, 110, 125, 150, 175, 200, 250, 300}

func (m *Model) applyIDEOZoom(c interface{ SetZoom(float32) }) {
	c.SetZoom(float32(m.ideZoomPercent()) / 100)
}

// ideZoomPercent returns the stored IDE zoom, with 100 standing in for the
// unset value and OpenIDEA's 50..300 limit applied to older settings.
func (m *Model) ideZoomPercent() int {
	zoom := m.prefs.Appearance.IDEZoom
	if zoom == 0 {
		return 100
	}
	return max(ideZoomSteps[0], min(zoom, ideZoomSteps[len(ideZoomSteps)-1]))
}

func nextIDEOZoom(current int, direction int) int {
	if direction > 0 {
		for _, zoom := range ideZoomSteps {
			if zoom > current {
				return zoom
			}
		}
		return ideZoomSteps[len(ideZoomSteps)-1]
	}
	for i := len(ideZoomSteps) - 1; i >= 0; i-- {
		if ideZoomSteps[i] < current {
			return ideZoomSteps[i]
		}
	}
	return ideZoomSteps[0]
}

func (m *Model) setIDEOZoom(percent int) {
	percent = max(ideZoomSteps[0], min(percent, ideZoomSteps[len(ideZoomSteps)-1]))
	if percent == m.ideZoomPercent() {
		return
	}
	m.prefs.Appearance.IDEZoom = percent
	m.savePreferences()
	m.status = fmt.Sprintf("IDE zoom %d%%", percent)
	m.invalidate()
}

// StepIDEOZoom changes the whole native UI scale by one OpenIDEA zoom stop.
func (m *Model) StepIDEOZoom(direction int) {
	if direction == 0 {
		return
	}
	m.setIDEOZoom(nextIDEOZoom(m.ideZoomPercent(), direction))
}

// ResetIDEOZoom returns the native UI to 100%.
func (m *Model) ResetIDEOZoom() { m.setIDEOZoom(100) }

func (m *Model) toggleAppearanceFlag(name string, value *bool) {
	*value = !*value
	m.savePreferences()
	state := "shown"
	if *value {
		state = "hidden"
	}
	m.status = name + " " + state
	m.invalidate()
}

// ToggleCompactMode applies OpenIDEA's :root[data-compact] sizing tokens to
// the native chrome and persists the choice in global appearance settings.
func (m *Model) ToggleCompactMode() {
	m.prefs.Appearance.CompactMode = !m.prefs.Appearance.CompactMode
	m.savePreferences()
	state := "on"
	if !m.prefs.Appearance.CompactMode {
		state = "off"
	}
	m.status = "Compact mode " + state
	m.invalidate()
}

// ToggleMainToolbar hides or shows the title bar's main toolbar.
func (m *Model) ToggleMainToolbar() {
	m.toggleAppearanceFlag("Toolbar", &m.prefs.Appearance.HideMainToolbar)
}

// ToggleToolWindowBars hides or shows the left and right tool window stripes.
func (m *Model) ToggleToolWindowBars() {
	m.toggleAppearanceFlag("Tool window bars", &m.prefs.Appearance.HideToolWindowBars)
}

// ToggleStatusBar hides or shows the status bar.
func (m *Model) ToggleStatusBar() {
	m.toggleAppearanceFlag("Status bar", &m.prefs.Appearance.HideStatusBar)
}
