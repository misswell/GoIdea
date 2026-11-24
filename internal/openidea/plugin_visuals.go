package openidea

import (
	"context"
)

// PluginDecoration is one editor decoration published by a code plugin.
type PluginDecoration struct {
	Range OutlineRange `json:"range"`
	Style string       `json:"style"`
	Hover *string      `json:"hover"`
	After *string      `json:"after"`
}

// PluginFileDecoration is the project-tree part of a decoration set. GoIDEA
// currently consumes editor sets and retains the typed shape for diagnostics.
type PluginFileDecoration struct {
	Badge   *string `json:"badge"`
	Style   string  `json:"style"`
	Tooltip *string `json:"tooltip"`
	Note    *string `json:"note"`
}

// PluginDecorationSet is a plugin's complete decoration state for one file.
type PluginDecorationSet struct {
	Root  string                `json:"root"`
	Path  string                `json:"path"`
	SetID string                `json:"setId"`
	Tree  bool                  `json:"tree"`
	Items []PluginDecoration    `json:"items"`
	File  *PluginFileDecoration `json:"file"`
}

// PluginDecorationChange is the decorationsChanged runtime event.
type PluginDecorationChange struct {
	PluginID string                `json:"pluginId"`
	Sets     []PluginDecorationSet `json:"sets"`
}

// PluginLineMarker is one gutter marker from a line-markers provider.
type PluginLineMarker struct {
	PluginID string    `json:"pluginId"`
	Line     uint32    `json:"line"`
	Icon     *string   `json:"icon"`
	Tooltip  string    `json:"tooltip"`
	Target   *Location `json:"target"`
}

// PluginMarkers is the backend's resolved markers for one indexed file.
type PluginFileMarkers struct {
	Path    string                `json:"path"`
	Version *uint32               `json:"version"`
	Markers []PluginIndexedMarker `json:"markers"`
}

// PluginIndexedMarker is one resolved marker returned by a document indexer.
type PluginIndexedMarker struct {
	Range    OutlineRange `json:"range"`
	Targets  []Location   `json:"targets"`
	PluginID string       `json:"pluginId"`
	Icon     *string      `json:"icon"`
}

// PluginMarkerIconDecl is a validated contributes.markerIcons declaration.
// The pinned runtime exposes declarations and tooltips, but not plugin_assets
// over JSON-RPC, so GoIDEA uses this metadata with its native gutter dots.
type PluginMarkerIconDecl struct {
	ID       string   `json:"id"`
	Icon     string   `json:"icon"`
	Markers  []string `json:"markers"`
	Tooltip  *string  `json:"tooltip"`
	Revision string   `json:"revision"`
}

// PluginDecorationSets returns every live plugin's current project-scoped
// decoration snapshots.
func (m *Manager) PluginDecorationSets(ctx context.Context, window string) (map[string][]PluginDecorationSet, error) {
	raw, err := m.Invoke(ctx, window, "plugin_decoration_sets", nil)
	if err != nil {
		return nil, err
	}
	var sets map[string][]PluginDecorationSet
	if err := decodePluginJSON(raw, &sets); err != nil {
		return nil, err
	}
	return sets, nil
}

// PluginLineMarkers asks live line-markers providers about one open document.
func (m *Manager) PluginLineMarkers(ctx context.Context, window string, doc uint32) ([]PluginLineMarker, error) {
	raw, err := m.Invoke(ctx, window, "plugin_line_markers", map[string]any{"doc": doc})
	if err != nil {
		return nil, err
	}
	var markers []PluginLineMarker
	if err := decodePluginJSON(raw, &markers); err != nil {
		return nil, err
	}
	return markers, nil
}

// PluginNavigationMarkers resolves document-indexer markers for up to 64
// paths. The limit is enforced by the pinned backend.
func (m *Manager) PluginNavigationMarkers(
	ctx context.Context,
	window string,
	paths []string,
) ([]PluginFileMarkers, error) {
	raw, err := m.Invoke(ctx, window, "plugin_navigation_markers", map[string]any{"paths": paths})
	if err != nil {
		return nil, err
	}
	var markers []PluginFileMarkers
	if err := decodePluginJSON(raw, &markers); err != nil {
		return nil, err
	}
	return markers, nil
}

// PluginSetActiveFile tells the plugin host which editor has focus. An empty
// path clears it when no project editor is active.
func (m *Manager) PluginSetActiveFile(ctx context.Context, window, path string) error {
	var active any
	if path != "" {
		active = path
	}
	_, err := m.Invoke(ctx, window, "plugin_set_active_file", map[string]any{"path": active})
	return err
}
