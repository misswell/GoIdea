package openidea

import (
	"bytes"
	"context"
	"encoding/json"
)

func decodePluginJSON(raw []byte, out any) error {
	return json.NewDecoder(bytes.NewReader(raw)).Decode(out)
}

// PluginUIContribution is one validated declaration in OpenIDEA's UI store.
// Decl keeps the contribution-shaped JSON so newer extension points can be
// ignored without changing the list response.
type PluginUIContribution struct {
	PluginID string          `json:"pluginId"`
	Point    string          `json:"point"`
	Key      string          `json:"key"`
	Decl     json.RawMessage `json:"decl"`
}

// PluginInfo is the part of OpenIDEA's installed plugin record the native
// workbench needs to decide whether its live contributions can be queried.
type PluginInfo struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Authors     []string `json:"authors"`
	Enabled     bool     `json:"enabled"`
	Status      string   `json:"status"`
	HasCode     bool     `json:"hasCode"`
	Error       *string  `json:"error"`
}

// PluginsSnapshot is OpenIDEA's installed/linked/project plugin list plus the
// UI contributions projected from the live plugins.
type PluginsSnapshot struct {
	Plugins []PluginInfo           `json:"plugins"`
	UI      []PluginUIContribution `json:"ui"`
}

// PluginEventMessage is a server-side plugin notification addressed to one
// window, or to every window when Window is nil.
type PluginEventMessage struct {
	Window *string         `json:"window"`
	Event  json.RawMessage `json:"event"`
}

// PluginContextKey is a plugin context value used by `when` clauses. Value is
// JSON text, as OpenIDEA stores it.
type PluginContextKey struct {
	PluginID string `json:"pluginId"`
	Key      string `json:"key"`
	Value    string `json:"value"`
}

// PluginViewButton is one toolbar button of a rendered plugin view.
type PluginViewButton struct {
	ID      string  `json:"id"`
	Label   string  `json:"label"`
	Icon    *string `json:"icon"`
	Enabled bool    `json:"enabled"`
}

// PluginViewTreeItem is one item of a rendered plugin tree. Parent keys refer
// to an earlier item in the flat list.
type PluginViewTreeItem struct {
	Key        string  `json:"key"`
	Parent     *string `json:"parent"`
	Label      string  `json:"label"`
	Icon       *string `json:"icon"`
	Detail     *string `json:"detail"`
	Expandable bool    `json:"expandable"`
	Expanded   bool    `json:"expanded"`
}

// PluginViewListItem is one item of a rendered plugin list.
type PluginViewListItem struct {
	Key    string  `json:"key"`
	Label  string  `json:"label"`
	Icon   *string `json:"icon"`
	Detail *string `json:"detail"`
}

// PluginViewEvent is a user action sent back to a rendered plugin view.
type PluginViewEvent struct {
	Kind string `json:"kind"`
	Key  string `json:"key,omitempty"`
	ID   string `json:"id,omitempty"`
}

// PluginView is what OpenIDEA's plugin returned for one view render. Value in
// Body is shaped by Kind.
type PluginView struct {
	Toolbar []PluginViewButton `json:"toolbar"`
	Body    struct {
		Kind  string          `json:"kind"`
		Value json.RawMessage `json:"value"`
	} `json:"body"`
	Status *string `json:"status"`
}

// PluginServiceTab is one tab in a Services item's detail pane.
type PluginServiceTab struct {
	ID    string            `json:"id"`
	Label string            `json:"label"`
	Body  PluginServicePane `json:"body"`
}

// PluginServicePane is a tagged message, text or process-log pane.
type PluginServicePane struct {
	Kind  string          `json:"kind"`
	Value json.RawMessage `json:"value"`
}

// PluginServiceLog is the process a Services log tab runs. Toggle arguments
// are appended in declaration order.
type PluginServiceLog struct {
	Program string                   `json:"program"`
	Args    []string                 `json:"args"`
	Toggles []PluginServiceLogToggle `json:"toggles"`
}

// PluginServiceLogToggle is one optional switch above a Services log.
type PluginServiceLogToggle struct {
	ID    string   `json:"id"`
	Label string   `json:"label"`
	Args  []string `json:"args"`
}

// PluginOutputLine is one line streamed by a Services log.
type PluginOutputLine struct {
	Stderr bool   `json:"stderr"`
	Text   string `json:"text"`
}

// PluginEndpoint is one HTTP route a code plugin reported for a project.
type PluginEndpoint struct {
	Method    *string  `json:"method"`
	Path      string   `json:"path"`
	Framework string   `json:"framework"`
	Handler   *string  `json:"handler"`
	Location  Location `json:"location"`
}

// PluginTestEvent is one incremental update from a plugin-owned test parser.
// Only the fields for Type are set, mirroring OpenIDEA's tagged TestEvent
// union.
type PluginTestEvent struct {
	Type       string    `json:"type"`
	ID         string    `json:"id,omitempty"`
	Parent     *string   `json:"parent,omitempty"`
	Name       string    `json:"name,omitempty"`
	Location   *Location `json:"location,omitempty"`
	DurationMs *float64  `json:"durationMs,omitempty"`
	Message    string    `json:"message,omitempty"`
	Expected   *string   `json:"expected,omitempty"`
	Actual     *string   `json:"actual,omitempty"`
	Reason     *string   `json:"reason,omitempty"`
	Text       string    `json:"text,omitempty"`
}

// PluginsList asks the server-side OpenIDEA plugin host for this window's
// installed, linked and project plugins.
func (m *Manager) PluginsList(ctx context.Context, window string) (PluginsSnapshot, error) {
	raw, err := m.Invoke(ctx, window, "plugin_list", nil)
	if err != nil {
		return PluginsSnapshot{}, err
	}
	var snapshot PluginsSnapshot
	if err := decodePluginJSON(raw, &snapshot); err != nil {
		return PluginsSnapshot{}, err
	}
	return snapshot, nil
}

// PluginContextKeys returns the plugin context keys in effect for window.
func (m *Manager) PluginContextKeys(ctx context.Context, window string) ([]PluginContextKey, error) {
	raw, err := m.Invoke(ctx, window, "plugin_context_keys", nil)
	if err != nil {
		return nil, err
	}
	var keys []PluginContextKey
	if err := decodePluginJSON(raw, &keys); err != nil {
		return nil, err
	}
	return keys, nil
}

// PluginViewRender asks a live code plugin to render a view. Services views
// pass no document, editor or table viewport.
func (m *Manager) PluginViewRender(ctx context.Context, window, pluginID, view string) (PluginView, error) {
	raw, err := m.Invoke(ctx, window, "plugin_view_render", map[string]any{
		"pluginId": pluginID,
		"view":     view,
		"viewport": nil,
		"doc":      nil,
		"editor":   nil,
	})
	if err != nil {
		return PluginView{}, err
	}
	var rendered PluginView
	if err := decodePluginJSON(raw, &rendered); err != nil {
		return PluginView{}, err
	}
	return rendered, nil
}

// PluginViewEvent sends one user action to a rendered view.
func (m *Manager) PluginViewEvent(
	ctx context.Context,
	window, pluginID, view string,
	event PluginViewEvent,
) error {
	_, err := m.Invoke(ctx, window, "plugin_view_event", map[string]any{
		"pluginId": pluginID,
		"view":     view,
		"event":    event,
		"doc":      nil,
		"editor":   nil,
	})
	return err
}

// PluginServiceDetail returns the selected Services item's detail tabs.
func (m *Manager) PluginServiceDetail(
	ctx context.Context,
	window, pluginID, view, key string,
) ([]PluginServiceTab, error) {
	raw, err := m.Invoke(ctx, window, "plugin_service_detail", map[string]any{
		"pluginId": pluginID,
		"view":     view,
		"key":      key,
	})
	if err != nil {
		return nil, err
	}
	var tabs []PluginServiceTab
	if err := decodePluginJSON(raw, &tabs); err != nil {
		return nil, err
	}
	return tabs, nil
}

// PluginServiceLog starts a cancellable Services log stream. Lines arrive as
// `channel` notifications carrying PluginOutputLine values with ID channel.
// The result is the process exit code, or nil when the program was stopped.
func (m *Manager) PluginServiceLog(
	ctx context.Context,
	window, pluginID, view, key, tab string,
	request, channel uint32,
	on []string,
) (*int, error) {
	raw, err := m.Invoke(ctx, window, "plugin_service_log", map[string]any{
		"request":  request,
		"pluginId": pluginID,
		"view":     view,
		"key":      key,
		"tab":      tab,
		"on":       on,
		"onLine":   map[string]any{"$channel": channel},
	})
	if err != nil {
		return nil, err
	}
	var code *int
	if err := decodePluginJSON(raw, &code); err != nil {
		return nil, err
	}
	return code, nil
}

// PluginRunCommand runs a command declared by a code plugin. Services item
// menus pass the raw item key as arg.
func (m *Manager) PluginRunCommand(
	ctx context.Context,
	window, pluginID, commandID, arg string,
) error {
	_, err := m.Invoke(ctx, window, "plugin_run_command", map[string]any{
		"pluginId":      pluginID,
		"commandId":     commandID,
		"arg":           arg,
		"editor":        nil,
		"paths":         nil,
		"settingsScope": nil,
		"commandChain":  nil,
		"request":       nil,
	})
	return err
}

// PluginCancel requests cancellation of a cancellable plugin operation.
func (m *Manager) PluginCancel(ctx context.Context, window string, request uint32) error {
	_, err := m.Invoke(ctx, window, "cancel_request", map[string]any{"id": request})
	return err
}

// PluginEndpoints asks one live code plugin for the project's HTTP routes.
func (m *Manager) PluginEndpoints(ctx context.Context, window, pluginID string) ([]PluginEndpoint, error) {
	raw, err := m.Invoke(ctx, window, "plugin_endpoints", map[string]any{"pluginId": pluginID})
	if err != nil {
		return nil, err
	}
	var endpoints []PluginEndpoint
	if err := decodePluginJSON(raw, &endpoints); err != nil {
		return nil, err
	}
	return endpoints, nil
}

// PluginSelectTests asks a plugin test provider to turn selected test-tree
// paths into argv for its configuration. A null reply means it offers no
// selector.
func (m *Manager) PluginSelectTests(
	ctx context.Context,
	window, config string,
	tests [][]string,
) ([]string, error) {
	raw, err := m.Invoke(ctx, window, "plugin_select_tests", map[string]any{
		"config": config,
		"tests":  tests,
	})
	if err != nil {
		return nil, err
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}
	var args []string
	if err := decodePluginJSON(raw, &args); err != nil {
		return nil, err
	}
	return args, nil
}

// PluginParseTestResults sends one bounded output chunk to a plugin test
// provider. A null reply means the provider has no test parser.
func (m *Manager) PluginParseTestResults(
	ctx context.Context,
	window, config, chunk string,
) ([]PluginTestEvent, error) {
	raw, err := m.Invoke(ctx, window, "plugin_parse_test_results", map[string]any{
		"config": config,
		"chunk":  chunk,
	})
	if err != nil {
		return nil, err
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}
	var events []PluginTestEvent
	if err := decodePluginJSON(raw, &events); err != nil {
		return nil, err
	}
	return events, nil
}
