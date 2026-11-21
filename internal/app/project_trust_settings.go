package app

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type projectTrustDecisions struct {
	Projects   []string `json:"projects"`
	Locations  []string `json:"locations"`
	Restricted []string `json:"restricted"`
}

type projectTrustSettingsState struct {
	decisions projectTrustDecisions
	loading   bool
	loaded    bool
	err       string
	seq       int
	busy      map[string]bool
}

// loadProjectTrust reads the user-owned decisions behind Settings › Security ›
// Project Trust. The response is a snapshot; every mutation asks for a new one.
func (m *Model) loadProjectTrust() {
	if m.mgr == nil {
		m.projectTrust.loaded = true
		return
	}
	state := &m.projectTrust
	state.seq++
	seq := state.seq
	state.loading = true
	state.err = ""
	ctx := m.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	go func() {
		callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		raw, err := m.mgr.Invoke(callCtx, Window, "project_trust_list", map[string]any{})
		var decisions projectTrustDecisions
		if err == nil {
			err = json.Unmarshal(raw, &decisions)
		}
		m.post(func() {
			if seq != state.seq {
				return
			}
			state.loading = false
			state.loaded = true
			if err != nil {
				state.err = "Couldn't load trust decisions: " + err.Error()
			} else {
				state.decisions = decisions
			}
			m.invalidate()
		})
	}()
}

// revokeProjectTrust returns one project to Restricted Mode.
func (m *Model) revokeProjectTrust(path string) {
	m.runProjectTrustAction("project:"+path, "project_trust_set", map[string]any{
		"path": path, "trusted": false, "folder": false,
	})
}

// removeTrustedLocation stops trusting every project beneath one folder.
func (m *Model) removeTrustedLocation(path string) {
	m.runProjectTrustAction("location:"+path, "project_trust_remove_location", map[string]any{
		"path": path,
	})
}

// addTrustedLocation is the backend half of Settings › Project Trust › Add
// folder…. The native picker chooses an existing folder.
func (m *Model) addTrustedLocation(path string) {
	m.runProjectTrustAction("add-location", "project_trust_add_location", map[string]any{
		"path": path,
	})
}

// BrowseTrustedLocation opens OpenIDEA's Add Trusted Location folder picker.
func (m *Model) BrowseTrustedLocation() {
	parent := m.win
	start := m.Root()
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: parent, Title: "Add Trusted Location", ButtonLabel: "Add",
			DefaultPath: start, Directory: true,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		m.post(func() { m.addTrustedLocation(paths[0]) })
	}()
}

func (m *Model) runProjectTrustAction(key, command string, args map[string]any) {
	if m.mgr == nil || m.projectTrust.busy[key] {
		return
	}
	if m.projectTrust.busy == nil {
		m.projectTrust.busy = map[string]bool{}
	}
	m.projectTrust.busy[key] = true
	m.projectTrust.err = ""
	m.invalidate()
	ctx := m.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	go func() {
		callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		_, err := m.mgr.Invoke(callCtx, Window, command, args)
		m.post(func() {
			delete(m.projectTrust.busy, key)
			if err != nil {
				m.projectTrust.err = fmt.Sprintf("Couldn't update trust: %v", err)
			} else {
				m.loadProjectTrust()
			}
			m.invalidate()
		})
	}()
}

func (m *Model) settingsProjectTrustBody(c *ui.Context, oi oiColors) {
	state := &m.projectTrust
	ui.Text(c, "Trust decisions are saved in your user data and take effect immediately.").
		FontSize(12).TextColor(oi.text).SingleLine().Ellipsis("…")
	if state.err != "" {
		ui.Row(c).Padding(0, 8).Children(func() {
			ui.Text(c, state.err).FontSize(12).TextColor(oi.danger)
		})
	}
	if state.loading && !state.loaded {
		ui.Text(c, "Loading trust decisions…").Padding(0, 10).
			FontSize(12).TextColor(oi.textMuted).SingleLine()
	}

	settingsSectionTitle(c, oi, "Trusted projects")
	for _, path := range state.decisions.Projects {
		path := path
		busy := state.busy["project:"+path]
		ui.Row(c.Key("projectTrust:project:"+path)).MinHeight(36).Gap(12).
			Padding(0, 8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, path).FontSize(12).TextColor(oi.text).
				SingleLine().Ellipsis("…").Grow(1)
			label := "Revoke Trust"
			if busy {
				label = "Revoking…"
			}
			ui.Button(c, label).Disabled(busy).OnClick(func() {
				m.revokeProjectTrust(path)
			})
		})
	}
	if state.loaded && len(state.decisions.Projects) == 0 {
		ui.Text(c, "No trusted projects.").Padding(0, 8).
			FontSize(12).TextColor(oi.textMuted).SingleLine()
	}

	ui.Row(c).MinHeight(34).Padding(0, 8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "Trusted locations").FontSize(12).TextColor(oi.textStrong).
			SingleLine().Grow(1)
		label := "Add folder…"
		if state.busy["add-location"] {
			label = "Adding…"
		}
		ui.Button(c, label).Disabled(state.busy["add-location"]).
			OnClick(m.BrowseTrustedLocation)
	})
	ui.Text(c, "Every project inside a trusted location is trusted without asking.").
		FontSize(12).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
	for _, path := range state.decisions.Locations {
		path := path
		busy := state.busy["location:"+path]
		ui.Row(c.Key("projectTrust:location:"+path)).MinHeight(36).Gap(12).
			Padding(0, 8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, path).FontSize(12).TextColor(oi.text).
				SingleLine().Ellipsis("…").Grow(1)
			label := "Remove"
			if busy {
				label = "Removing…"
			}
			ui.Button(c, label).Disabled(busy).OnClick(func() {
				m.removeTrustedLocation(path)
			})
		})
	}
	if state.loaded && len(state.decisions.Locations) == 0 {
		ui.Text(c, "No trusted locations.").Padding(0, 8).
			FontSize(12).TextColor(oi.textMuted).SingleLine()
	}
}
