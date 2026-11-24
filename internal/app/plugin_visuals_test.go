package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
	"mayoi/internal/position"
)

func newPluginVisualTestModel(t *testing.T, text string) (*Model, *Tab) {
	t.Helper()
	m, _, tab := newPluginEditorFixture(t, text)
	tab.synced = tab.doc.Buffer().Version()
	m.plugins = &pluginPage{visual: newPluginVisualState(m.Root())}
	return m, tab
}

func newPluginVisualState(root string) pluginVisualState {
	return pluginVisualState{
		root:           root,
		decorations:    map[string][]openidea.PluginDecorationSet{},
		icons:          map[string]pluginMarkerIcon{},
		lineLoading:    map[uint32]bool{},
		lineSeen:       map[uint32]uint64{},
		lineResults:    map[uint32]pluginLineMarkerResult{},
		indexedSeen:    map[string]uint64{},
		indexedResults: map[string]pluginIndexedMarkerResult{},
	}
}

type pluginVisualRPC struct {
	command string
	args    any
}

func TestPluginVisualsUsePinnedProviderRPCs(t *testing.T) {
	tests := []struct {
		name   string
		call   func(*Model, *Tab)
		want   string
		reply  string
		verify func(*testing.T, any)
	}{
		{
			name:  "decorations",
			call:  func(m *Model, _ *Tab) { m.loadPluginDecorations() },
			want:  "plugin_decoration_sets",
			reply: `{}`,
			verify: func(t *testing.T, args any) {
				t.Helper()
				if args != nil {
					t.Fatalf("decoration args = %#v, want nil", args)
				}
			},
		},
		{
			name: "marker icons",
			call: func(m *Model, _ *Tab) { m.loadPluginMarkerIcons() },
			want: "plugin_list",
			reply: `{"plugins":[],"ui":[{"pluginId":"demo","point":"markerIcons",` +
				`"decl":{"id":"run","icon":"run.svg","tooltip":"Run application"}}]}`,
			verify: func(t *testing.T, args any) {
				t.Helper()
				if args != nil {
					t.Fatalf("plugin list args = %#v, want nil", args)
				}
			},
		},
		{
			name:  "line markers",
			call:  func(m *Model, tab *Tab) { m.startPluginLineMarkers(tab) },
			want:  "plugin_line_markers",
			reply: `[]`,
			verify: func(t *testing.T, args any) {
				t.Helper()
				got := args.(map[string]any)
				if got["doc"] != uint32(41) {
					t.Fatalf("line marker args = %#v", got)
				}
			},
		},
		{
			name:  "indexed markers",
			call:  func(m *Model, _ *Tab) { m.startPluginIndexedMarkers() },
			want:  "plugin_navigation_markers",
			reply: `[]`,
			verify: func(t *testing.T, args any) {
				t.Helper()
				got := args.(map[string]any)
				paths, ok := got["paths"].([]string)
				if !ok || len(paths) != 1 || !strings.HasSuffix(paths[0], "Unicode.java") {
					t.Fatalf("indexed marker args = %#v", got)
				}
			},
		},
		{
			name:  "active file",
			call:  func(m *Model, _ *Tab) { m.syncPluginActiveFile() },
			want:  "plugin_set_active_file",
			reply: `null`,
			verify: func(t *testing.T, args any) {
				t.Helper()
				got := args.(map[string]any)
				if path, _ := got["path"].(string); !strings.HasSuffix(path, "Unicode.java") {
					t.Fatalf("active file args = %#v", got)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, tab := newPluginVisualTestModel(t, "a😀b\nsecond")
			requests := make(chan pluginVisualRPC, 1)
			posts := make(chan struct{}, 1)
			m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
				requests <- pluginVisualRPC{command: command, args: args}
				return json.RawMessage(tc.reply), nil
			}
			m.postHook = func(fn func()) {
				fn()
				posts <- struct{}{}
			}

			tc.call(m, tab)
			request := receivePluginVisual(t, requests, "provider request")
			if request.command != tc.want {
				t.Fatalf("command = %q, want %q", request.command, tc.want)
			}
			tc.verify(t, request.args)
			receivePluginVisual(t, posts, "provider reply")

			if tc.want == "plugin_list" && m.plugins.visual.icons["demo\x00run"].Tooltip != "Run application" {
				t.Fatalf("marker icons = %#v", m.plugins.visual.icons)
			}
			if tc.want == "plugin_set_active_file" {
				m.active = -1
				tc.call(m, tab)
				cleared := receivePluginVisual(t, requests, "clear active file request")
				if args := cleared.args.(map[string]any); args["path"] != nil {
					t.Fatalf("cleared active file args = %#v", args)
				}
				receivePluginVisual(t, posts, "clear active file reply")
			}
		})
	}
}

func receivePluginVisual[T any](t *testing.T, ch <-chan T, what string) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
		var zero T
		return zero
	}
}

func TestPluginDecorationsConvertUTF16RangesAndShowNativeHover(t *testing.T) {
	m, tab := newPluginVisualTestModel(t, "a😀b\nsecond")
	path := tab.doc.Path()
	hover, after := "Plugin detail", "after text"
	m.plugins.visual.decorations["demo"] = []openidea.PluginDecorationSet{{
		Root: m.Root(),
		Path: path,
		Items: []openidea.PluginDecoration{
			{
				Range: openidea.OutlineRange{
					Start: openidea.OutlinePos{Line: 0, Col: 1},
					End:   openidea.OutlinePos{Line: 0, Col: 3},
				},
				Style: "highlight",
				Hover: &hover,
				After: &after,
			},
			{
				Range: openidea.OutlineRange{
					Start: openidea.OutlinePos{Line: 1},
					End:   openidea.OutlinePos{Line: 1},
				},
				Style: "warning",
				After: strPtr("warning after"),
			},
		},
	}}

	lines, text := m.pluginDecorationBackgrounds(tab)
	if len(text) != 1 || text[0].Start != 1 || text[0].End != len("a😀") {
		t.Fatalf("text backgrounds = %#v", text)
	}
	if len(lines) != 1 || lines[0].First != 1 || lines[0].Last != 1 {
		t.Fatalf("line backgrounds = %#v", lines)
	}
	if !m.pluginDecorationHover(tab, editor.HoverRequest{Offset: 2}) {
		t.Fatal("plugin decoration hover was not handled")
	}

	tester := ui.NewTester(func(c *ui.Context) {
		editor.View(c, tab.state).Fill()
	}, 600, 180)
	tester.Frame()
	if !tester.HasText("Plugin detail") || !tester.HasText("after text") {
		t.Fatalf("native hover did not show decoration detail")
	}
}

func TestPluginMarkerGroupsMergeIconsTargetsAndChooser(t *testing.T) {
	m, tab := newPluginVisualTestModel(t, "first\nsecond\nthird")
	path := tab.doc.Path()
	local, backend := tab.doc.Buffer().Version(), tab.doc.Version()
	targetA := openidea.Location{
		Path: path,
		Range: openidea.OutlineRange{
			Start: openidea.OutlinePos{Line: 1},
			End:   openidea.OutlinePos{Line: 1, Col: 3},
		},
	}
	targetB := openidea.Location{
		Path: filepath.Join(m.Root(), "Other.java"),
		Range: openidea.OutlineRange{
			Start: openidea.OutlinePos{Line: 4},
			End:   openidea.OutlinePos{Line: 4, Col: 2},
		},
	}
	externalURI := "jar:file:///deps/demo.jar!/Demo.class"
	targetC := openidea.Location{
		Path:  externalURI,
		URI:   &externalURI,
		Range: openidea.OutlineRange{},
	}
	lineIcon, indexedIcon := "implemented", "run"
	m.plugins.visual.lineResults[tab.doc.ID()] = pluginLineMarkerResult{
		LocalVersion: local, BackendVersion: backend,
		Markers: []openidea.PluginLineMarker{
			{Line: 1, Icon: &lineIcon, Tooltip: "Implementation available", Target: &targetA},
			{Line: 1, Tooltip: "Also available here", Target: &targetB},
		},
	}
	m.plugins.visual.indexedResults[path] = pluginIndexedMarkerResult{
		LocalVersion: local, BackendVersion: backend,
		Markers: []openidea.PluginIndexedMarker{{
			Range:    targetA.Range,
			Targets:  []openidea.Location{targetA, targetC},
			PluginID: "demo",
			Icon:     &indexedIcon,
		}},
	}
	m.plugins.visual.icons[pluginMarkerIconKey("demo", "run")] = pluginMarkerIcon{
		PluginID: "demo",
		ID:       "run",
		Icon:     "run.svg",
		Tooltip:  "Run application",
	}

	group := m.pluginMarkerGroups(tab)[1]
	if group == nil {
		t.Fatal("line 1 has no marker group")
	}
	if len(group.Markers) != 3 || len(group.Targets) != 3 {
		t.Fatalf("group markers = %#v, targets = %#v", group.Markers, group.Targets)
	}
	if group.IconDecl == nil || group.IconDecl.ID != "run" {
		t.Fatalf("icon declaration = %#v", group.IconDecl)
	}
	if got := pluginMarkerColor(group, oiPaletteOf(m.theme)); got != oiPaletteOf(m.theme).success {
		t.Fatalf("run icon color = %#v, want success", got)
	}
	marks := m.pluginGutterMarks(tab)
	if len(marks) != 1 || marks[0].Line != 1 {
		t.Fatalf("gutter marks = %#v", marks)
	}
	tooltip := joinPluginMarkerTooltip(m.pluginMarkerTooltip(tab, 1))
	for _, want := range []string{
		"Related targets (3)",
		"Implementation available",
		"Run application",
		"Unicode.java:2",
		"Other.java:5",
	} {
		if !strings.Contains(tooltip, want) {
			t.Fatalf("tooltip %q does not contain %q", tooltip, want)
		}
	}

	if !m.pluginMarkerClick(tab, 1) {
		t.Fatal("multi-target marker click was not handled")
	}
	chooser := m.plugins.visual.markerChooser
	if !chooser.Open || len(chooser.Items) != 3 {
		t.Fatalf("marker chooser = %#v", chooser)
	}
	m.stepPluginMarkerChooser(1)
	if m.plugins.visual.markerChooser.Sel != 1 {
		t.Fatalf("marker chooser selection = %d", m.plugins.visual.markerChooser.Sel)
	}
	m.choosePluginMarker(2)
	if m.plugins.visual.markerChooser.Open {
		t.Fatal("marker chooser stayed open after a target was chosen")
	}
	if !strings.Contains(m.status, "external dependency") {
		t.Fatalf("external target status = %q", m.status)
	}
}

func TestPluginMarkersHideOnEditAndIgnoreStaleResponses(t *testing.T) {
	m, tab := newPluginVisualTestModel(t, "first\nsecond")
	path := tab.doc.Path()
	local, backend := tab.doc.Buffer().Version(), tab.doc.Version()
	m.plugins.visual.lineResults[tab.doc.ID()] = pluginLineMarkerResult{
		LocalVersion: local, BackendVersion: backend,
		Markers: []openidea.PluginLineMarker{{Line: 1, Tooltip: "Before edit"}},
	}
	m.plugins.visual.indexedResults[path] = pluginIndexedMarkerResult{
		LocalVersion: local, BackendVersion: backend,
		Markers: []openidea.PluginIndexedMarker{{
			Range:    openidea.OutlineRange{Start: openidea.OutlinePos{Line: 1}},
			Targets:  []openidea.Location{{Path: path}},
			PluginID: "demo",
		}},
	}
	if group := m.pluginMarkerGroups(tab)[1]; group == nil {
		t.Fatal("seed marker was not visible")
	}
	if err := tab.doc.ApplyEdits(context.Background(), editor.Edit{
		Start: 0, End: 0, Text: "// edit\n",
	}); err != nil {
		t.Fatal(err)
	}
	m.pluginEditorChanged(tab, time.Now())
	if groups := m.pluginMarkerGroups(tab); len(groups) != 0 {
		t.Fatalf("edited document kept stored markers: %#v", groups)
	}

	tab.synced = tab.doc.Buffer().Version()
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	posted := make(chan struct{}, 1)
	m.invokeHook = func(context.Context, string, any) (json.RawMessage, error) {
		started <- struct{}{}
		<-release
		return json.RawMessage(`[{"line":0,"tooltip":"Stale response"}]`), nil
	}
	m.postHook = func(fn func()) {
		fn()
		posted <- struct{}{}
	}
	m.startPluginLineMarkers(tab)
	receivePluginVisual(t, started, "line-marker request")
	if err := tab.doc.ApplyEdits(context.Background(), editor.Edit{
		Start: 0, End: 0, Text: "// newer edit\n",
	}); err != nil {
		t.Fatal(err)
	}
	close(release)
	receivePluginVisual(t, posted, "stale marker reply")
	if groups := m.pluginMarkerGroups(tab); len(groups) != 0 {
		t.Fatalf("stale marker response became visible: %#v", groups)
	}

	tab.synced = tab.doc.Buffer().Version()
	m.invokeHook = func(context.Context, string, any) (json.RawMessage, error) {
		return json.RawMessage(`[{"line":0,"tooltip":"Refreshed marker"}]`), nil
	}
	m.startPluginLineMarkers(tab)
	receivePluginVisual(t, posted, "refreshed marker reply")
	group := m.pluginMarkerGroups(tab)[0]
	if group == nil || len(group.Markers) != 1 || group.Markers[0] != "Refreshed marker" {
		t.Fatalf("refreshed marker group = %#v", group)
	}
}

func TestPluginIndexedMarkersRequestAtMostSixtyFourPaths(t *testing.T) {
	root := t.TempDir()
	backend := &pluginVisualBatchBackend{docs: map[string]openidea.DocInfo{}}
	reg := docbridge.NewRegistry(backend, Window)
	t.Cleanup(func() {
		for path := range backend.docs {
			_ = reg.Close(context.Background(), path)
		}
	})
	m := &Model{
		postHook: func(fn func()) { fn() },
		reg:      reg,
	}
	m.setRoot(root)
	for i := 0; i < pluginVisualBatchLimit+1; i++ {
		path := filepath.Join(root, fmt.Sprintf("File%d.java", i))
		backend.docs[path] = openidea.DocInfo{
			ID: uint32(i + 1), Path: path, Text: "class File {}\n", Version: 1,
		}
		doc, err := reg.Open(context.Background(), path)
		if err != nil {
			t.Fatal(err)
		}
		m.tabs = append(m.tabs, &Tab{
			doc:    doc,
			state:  &editor.State{Buffer: doc.Buffer()},
			synced: doc.Buffer().Version(),
		})
	}
	m.plugins = &pluginPage{visual: newPluginVisualState(root)}
	requests := make(chan pluginVisualRPC, 1)
	posted := make(chan struct{}, 1)
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		requests <- pluginVisualRPC{command: command, args: args}
		return json.RawMessage(`[]`), nil
	}
	m.postHook = func(fn func()) {
		fn()
		posted <- struct{}{}
	}
	m.startPluginIndexedMarkers()
	request := receivePluginVisual(t, requests, "indexed-marker batch")
	if request.command != "plugin_navigation_markers" {
		t.Fatalf("command = %q", request.command)
	}
	paths := request.args.(map[string]any)["paths"].([]string)
	if len(paths) != pluginVisualBatchLimit {
		t.Fatalf("indexed marker path count = %d, want %d", len(paths), pluginVisualBatchLimit)
	}
	seen := map[string]bool{}
	for _, path := range paths {
		if seen[path] {
			t.Fatalf("duplicate indexed marker path %q", path)
		}
		seen[path] = true
	}
	receivePluginVisual(t, posted, "indexed-marker reply")
}

func TestPluginVisualEventsReplaceAndInvalidateNativeState(t *testing.T) {
	m, tab := newPluginVisualTestModel(t, "first\nsecond")
	path := tab.doc.Path()
	m.plugins.visual.lineResults[tab.doc.ID()] = pluginLineMarkerResult{
		LocalVersion: tab.doc.Buffer().Version(), BackendVersion: tab.doc.Version(),
		Markers: []openidea.PluginLineMarker{{Line: 0, Tooltip: "Old"}},
	}
	m.plugins.visual.indexedResults[path] = pluginIndexedMarkerResult{
		LocalVersion: tab.doc.Buffer().Version(), BackendVersion: tab.doc.Version(),
	}

	m.acceptPluginEvent(json.RawMessage(`{"type":"decorationsChanged","pluginId":"demo","sets":[` +
		`{"root":"` + strings.ReplaceAll(m.Root(), `\`, `\\`) + `","path":"` +
		strings.ReplaceAll(path, `\`, `\\`) + `","items":[]}]}`))
	if sets := m.plugins.visual.decorations["demo"]; len(sets) != 1 {
		t.Fatalf("decoration event sets = %#v", sets)
	}
	m.acceptPluginEvent(json.RawMessage(`{"type":"decorationsChanged","pluginId":"demo"}`))
	if _, ok := m.plugins.visual.decorations["demo"]; ok {
		t.Fatal("null decoration event did not remove the plugin's sets")
	}

	m.acceptPluginEvent(json.RawMessage(`{"type":"documentIndexesChanged"}`))
	if len(m.plugins.visual.lineResults) != 0 || len(m.plugins.visual.indexedResults) != 0 {
		t.Fatalf("document index event kept marker results: %#v %#v",
			m.plugins.visual.lineResults, m.plugins.visual.indexedResults)
	}
	m.plugins.visual.lineResults[tab.doc.ID()] = pluginLineMarkerResult{}
	m.acceptPluginEvent(json.RawMessage(`{"type":"commandsChanged"}`))
	if len(m.plugins.visual.lineResults) != 0 {
		t.Fatalf("commands event kept marker results: %#v", m.plugins.visual.lineResults)
	}
}

func TestPluginDecorationEventsInitializeStateAndRestartPreservesGenerations(t *testing.T) {
	m := &Model{
		postHook: func(fn func()) { fn() },
		plugins:  &pluginPage{},
	}
	m.setRoot(t.TempDir())
	m.acceptPluginEvent(json.RawMessage(`{"type":"decorationsChanged","pluginId":"early","sets":[]}`))
	if sets, ok := m.plugins.visual.decorations["early"]; !ok || sets == nil {
		t.Fatalf("early decoration event sets = %#v, present = %v", sets, ok)
	}

	m.plugins.visual.decorationGen = 4
	m.plugins.visual.iconGen = 5
	m.plugins.visual.markerGen = 7
	m.pluginVisualBackendRestarted()
	visual := m.plugins.visual
	if visual.decorationGen != 4 || visual.iconGen != 5 || visual.markerGen != 8 {
		t.Fatalf("restart generations = %#v", visual)
	}
}

type pluginVisualBatchBackend struct {
	docs map[string]openidea.DocInfo
}

func (b *pluginVisualBatchBackend) DocOpen(_ context.Context, _, path string) (openidea.DocInfo, error) {
	info, ok := b.docs[path]
	if !ok {
		return openidea.DocInfo{}, fmt.Errorf("unknown document %s", path)
	}
	return info, nil
}

func (b *pluginVisualBatchBackend) DocEdit(
	_ context.Context,
	path string,
	_ uint32,
	_ uint32,
	_ position.ChangeSet,
) (uint32, error) {
	return b.docs[path].Version + 1, nil
}

func (b *pluginVisualBatchBackend) DocSnapshot(
	_ context.Context,
	path string,
	_ uint32,
) (openidea.DocInfo, error) {
	info, ok := b.docs[path]
	if !ok {
		return openidea.DocInfo{}, fmt.Errorf("unknown document %s", path)
	}
	return info, nil
}

func (*pluginVisualBatchBackend) DocSave(context.Context, string, uint32) error {
	return nil
}

func (*pluginVisualBatchBackend) DocClose(context.Context, string, uint32) error {
	return nil
}
