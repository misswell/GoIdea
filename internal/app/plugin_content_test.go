package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui/editor"
)

func resetPluginContentState(t *testing.T) {
	t.Helper()
	clearPluginContentState()
	t.Cleanup(clearPluginContentState)
}

func clearPluginContentState() {
	pluginContents.mu.Lock()
	sessions := make([]*pluginContentSession, 0, len(pluginContents.sessions))
	for _, session := range pluginContents.sessions {
		sessions = append(sessions, session)
	}
	pluginContents.mu.Unlock()
	for _, session := range sessions {
		pluginContents.remove(session)
	}
	pluginContents.mu.Lock()
	pluginContents.sessions = map[string]*pluginContentSession{}
	pluginContents.pending = map[string]*pluginContentSession{}
	pluginContents.listeners = map[chan pluginContentDispatchResult]struct{}{}
	pluginContents.mu.Unlock()
	pluginContentControllers.Lock()
	pluginContentControllers.items = nil
	pluginContentControllers.Unlock()
}

func testPluginContentModel(t *testing.T, plugins ...string) *Model {
	t.Helper()
	enabled := make(map[string]bool, len(plugins))
	for _, plugin := range plugins {
		enabled[plugin] = true
	}
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
	}
	m.setRoot(t.TempDir())
	m.invokeHook = func(ctx context.Context, cmd string, args any) (json.RawMessage, error) {
		switch cmd {
		case "plugin_list":
			items := make([]map[string]any, 0, len(enabled))
			for id, on := range enabled {
				items = append(items, map[string]any{"id": id, "enabled": on})
			}
			return json.Marshal(map[string]any{"plugins": items})
		case "plugin_content_tab_action":
			return json.RawMessage("null"), nil
		case "plugin_answer_command":
			return json.RawMessage("true"), nil
		default:
			return nil, errors.New("unexpected RPC " + cmd)
		}
	}
	return m
}

func testPluginContentData(t *testing.T) pluginContentData {
	t.Helper()
	return pluginContentData{
		PluginID: "probe",
		View:     "inspector",
		Key:      "service-config",
		Title:    "Service",
		FileName: "service.toml",
		Language: "toml",
		Content:  "port = 8080\n",
		Actions: []pluginContentAction{
			{ID: "reload", Label: "Reload", Primary: true},
			{ID: "save", Label: "Save", Save: true},
		},
	}
}

func TestPluginContentValidationMatchesPublishedLimits(t *testing.T) {
	resetPluginContentState(t)
	enabled := func(string) (bool, error) { return true, nil }
	raw := func(value map[string]any) json.RawMessage {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	base := map[string]any{
		"title":   "service.toml",
		"content": "port = 8080\n",
		"actions": []map[string]any{
			{"id": "save", "label": "Save", "save": true},
		},
	}
	data, err := validatePluginContentData("probe", "view", raw(base), 100, enabled)
	if err != nil {
		t.Fatalf("valid data: %v", err)
	}
	if data.Key != "service.toml" || data.ReadOnly || len(data.Actions) != 1 ||
		data.Actions[0].ID != "save" {
		t.Fatalf("normalized data = %+v", data)
	}

	for name, mutate := range map[string]func(map[string]any){
		"empty title":      func(v map[string]any) { v["title"] = "" },
		"long title":       func(v map[string]any) { v["title"] = strings.Repeat("x", 257) },
		"long key":         func(v map[string]any) { v["key"] = strings.Repeat("x", 129) },
		"path separator":   func(v map[string]any) { v["fileName"] = "sub/service.toml" },
		"backslash":        func(v map[string]any) { v["fileName"] = `sub\service.toml` },
		"null language":    func(v map[string]any) { v["language"] = nil },
		"number content":   func(v map[string]any) { v["content"] = 3 },
		"nul content":      func(v map[string]any) { v["content"] = "a\x00b" },
		"string read only": func(v map[string]any) { v["readOnly"] = "true" },
		"duplicate action": func(v map[string]any) { v["actions"] = []any{base["actions"], base["actions"]} },
		"two save actions": func(v map[string]any) {
			v["actions"] = []any{
				map[string]any{"id": "s1", "label": "S1", "save": true},
				map[string]any{"id": "s2", "label": "S2", "save": true},
			}
		},
		"unknown icon": func(v map[string]any) {
			v["actions"] = []any{
				map[string]any{"id": "s1", "label": "S1", "icon": "not-a-builtin-icon"},
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			value := make(map[string]any, len(base))
			for key, current := range base {
				value[key] = current
			}
			mutate(value)
			if _, err := validatePluginContentData("probe", "view", raw(value), 100, enabled); err == nil {
				t.Fatal("invalid data was accepted")
			}
		})
	}
	if _, err := validatePluginContentData(
		"probe", "view", raw(base), pluginContentMaxMessageBytes+1, enabled,
	); err == nil {
		t.Fatal("oversized message was accepted")
	}
	oversizedMessage := make(map[string]any, len(base)+1)
	for key, value := range base {
		oversizedMessage[key] = value
	}
	oversizedMessage["padding"] = strings.Repeat("x", pluginContentMaxMessageBytes)
	oversizedPayload := raw(oversizedMessage)
	if len(oversizedPayload) <= pluginContentMaxMessageBytes {
		t.Fatalf("oversized fixture is only %d bytes", len(oversizedPayload))
	}
	if _, err := validatePluginContentData(
		"probe", "view", oversizedPayload, len(oversizedPayload), enabled,
	); err == nil {
		t.Fatal("oversized payload was accepted")
	}
	if _, err := validatePluginContentData(
		"probe", "view",
		raw(map[string]any{
			"title": "x", "content": strings.Repeat("a", pluginContentMaxContentBytes+1),
		}),
		100,
		enabled,
	); err == nil {
		t.Fatal("oversized content was accepted")
	}
	if _, err := validatePluginContentData(
		"probe", "view", raw(base), 100,
		func(string) (bool, error) { return false, nil },
	); err == nil {
		t.Fatal("disabled plugin was accepted")
	}
}

func TestPluginContentSaveActionFallbackOrder(t *testing.T) {
	data := pluginContentData{Actions: []pluginContentAction{
		{ID: "primary", Label: "Primary", Primary: true},
		{ID: "save", Label: "Save"},
		{ID: "persist", Label: "Persist", Save: true},
	}}
	action, ok := pluginContentSaveAction(data)
	if !ok || action.ID != "persist" {
		t.Fatalf("explicit Save = %+v, %v", action, ok)
	}
	data.Actions = data.Actions[:2]
	action, ok = pluginContentSaveAction(data)
	if !ok || action.ID != "save" {
		t.Fatalf("named Save = %+v, %v", action, ok)
	}
	data.Actions = data.Actions[:1]
	action, ok = pluginContentSaveAction(data)
	if !ok || action.ID != "primary" {
		t.Fatalf("primary fallback = %+v, %v", action, ok)
	}
}

func TestPluginContentTabReusesKeyAndPreservesDirtyText(t *testing.T) {
	resetPluginContentState(t)
	m := testPluginContentModel(t, "probe")
	data := testPluginContentData(t)
	tab, err := m.openValidatedPluginContentTab(data)
	if err != nil {
		t.Fatal(err)
	}
	if tab.kind != tabKindPluginContent || m.tab() != tab ||
		tab.state.Buffer.Language() == nil ||
		tab.state.Buffer.Language().Name != "TOML" {
		t.Fatalf("opened tab = %#v, language %v", tab, tab.state.Buffer.Language())
	}
	tab.state.Buffer.Set("port = 9090\n")
	if !m.pluginContentDirty(tab) {
		t.Fatal("edit did not make the tab dirty")
	}
	next := data
	next.Title = "Service config"
	next.Content = "port = 7070\n"
	reopened, err := m.openValidatedPluginContentTab(next)
	if err != nil {
		t.Fatal(err)
	}
	if reopened != tab || tab.state.Buffer.String() != "port = 9090\n" ||
		m.pluginContentTitle(tab) != "Service config" {
		t.Fatalf("dirty reopen replaced text or tab: %q %q", tab.state.Buffer.String(), m.pluginContentTitle(tab))
	}
	pluginContents.find(pluginContentKey("probe", "inspector", data.Key)).baseline = "port = 9090\n"
	if m.pluginContentDirty(tab) {
		t.Fatal("baseline update did not clean the tab")
	}
}

func TestPluginContentReplyRoutingAndStaleProjectProtection(t *testing.T) {
	resetPluginContentState(t)
	m := testPluginContentModel(t, "probe")
	data := testPluginContentData(t)
	tab, err := m.openValidatedPluginContentTab(data)
	if err != nil {
		t.Fatal(err)
	}
	session := m.pluginContentTabFor(tab)
	tab.state.Buffer.Set("port = 9090\n")
	done := make(chan pluginContentDispatchResult, 1)
	session.pending = &pluginContentPending{
		requestID: "ct-test", action: "save", snapshot: "port = 9090\n", done: done,
	}
	pluginContents.pending["ct-test"] = session
	reply := json.RawMessage(`{"saved":true,"readOnly":false,"status":"Saved","content":"port = 9090\n"}`)
	if !m.acceptPluginContentEvent(json.RawMessage(`{"type":"contentTabReply","pluginId":"probe","requestId":"ct-test","reply":` + string(reply) + `}`)) {
		t.Fatal("content reply was not recognized")
	}
	result := <-done
	if result.snapshot != "port = 9090\n" || result.reply.Status == nil ||
		*result.reply.Status != "Saved" {
		t.Fatalf("routed result = %+v", result)
	}
	m.finishPluginContentAction(session, true, result)
	if m.pluginContentDirty(tab) {
		t.Fatal("successful reply did not establish the clean baseline")
	}

	tab.state.Buffer.Set("port = 10000\n")
	staleDone := make(chan pluginContentDispatchResult, 1)
	session.pending = &pluginContentPending{
		requestID: "ct-stale", action: "save", snapshot: "port = 10000\n",
		done: staleDone,
	}
	pluginContents.pending["ct-stale"] = session
	m.workspaceGeneration++
	pluginContents.reply("ct-stale", "probe", pluginContentDispatchResult{
		reply: pluginContentReply{}, snapshot: "port = 10000\n",
	})
	result = <-staleDone
	m.finishPluginContentAction(session, true, result)
	if pluginContents.find(session.key) != nil {
		t.Fatal("stale project reply left its session routable")
	}
}

func TestPluginContentReplyRejectsInvalidFieldsAndPluginDisable(t *testing.T) {
	for name, raw := range map[string]string{
		"content NUL":   `{"content":"a\u0000b"}`,
		"long status":   `{"status":"` + strings.Repeat("x", 513) + `"}`,
		"bad diff body": `{"diff":{"left":3,"right":"b"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := validatePluginContentReply(json.RawMessage(raw)); err == nil {
				t.Fatal("invalid reply was accepted")
			}
		})
	}

	resetPluginContentState(t)
	m := testPluginContentModel(t, "probe")
	data := testPluginContentData(t)
	tab, err := m.openValidatedPluginContentTab(data)
	if err != nil {
		t.Fatal(err)
	}
	session := m.pluginContentTabFor(tab)
	m.invokeHook = func(ctx context.Context, cmd string, args any) (json.RawMessage, error) {
		if cmd != "plugin_list" {
			return nil, errors.New("unexpected RPC " + cmd)
		}
		return json.RawMessage(`{"plugins":[{"id":"probe","enabled":false}]}`), nil
	}
	session.RunAction("reload")
	if session.busy || !strings.Contains(session.status, "Invalid content editor configuration") {
		t.Fatalf("disabled plugin action state = busy %v status %q", session.busy, session.status)
	}
}

func TestPluginContentOpenDiffUsesNativeViewer(t *testing.T) {
	resetPluginContentState(t)
	m := testPluginContentModel(t, "probe")
	data := testPluginContentData(t)
	tab, err := m.openValidatedPluginContentTab(data)
	if err != nil {
		t.Fatal(err)
	}
	session := m.pluginContentTabFor(tab)
	done := make(chan pluginContentDispatchResult, 1)
	session.pending = &pluginContentPending{
		requestID: "ct-diff", action: "reload", snapshot: data.Content, done: done,
	}
	pluginContents.pending["ct-diff"] = session
	payload := `{"type":"contentTabReply","pluginId":"probe","requestId":"ct-diff","reply":{"diff":{"left":"a\n","right":"b\n"}}}`
	if !m.acceptPluginContentEvent(json.RawMessage(payload)) {
		t.Fatal("diff reply was not recognized")
	}
	result := <-done
	m.finishPluginContentAction(session, false, result)
	if m.diff == nil || m.diff.path != "plugin-diff:/service.toml" ||
		m.diff.leftLabel != "Before" || m.diff.rightLabel != "After" {
		t.Fatalf("native diff = %+v", m.diff)
	}
}

func TestPluginContentFindTargetsAddressedTab(t *testing.T) {
	resetPluginContentState(t)
	m := testPluginContentModel(t, "probe")
	first := testPluginContentData(t)
	first.Key = "one"
	firstTab, err := m.openValidatedPluginContentTab(first)
	if err != nil {
		t.Fatal(err)
	}
	second := testPluginContentData(t)
	second.Key = "two"
	secondTab, err := m.openValidatedPluginContentTab(second)
	if err != nil {
		t.Fatal(err)
	}
	target := secondTab.pagePath + ":view"
	if controller := pluginContentControllerFor(target); controller == nil {
		t.Fatal("addressed Find controller was not registered")
	} else {
		controller.Find()
	}
	if m.tab() != secondTab || secondTab == firstTab {
		t.Fatalf("Find selected %p, want addressed %p", m.tab(), secondTab)
	}
	if session := m.pluginContentTabFor(secondTab); session == nil {
		t.Fatal("second session disappeared")
	}
}

func TestPluginContentRegistryUsesBoundedListeners(t *testing.T) {
	resetPluginContentState(t)
	first, stopFirst := pluginContents.listen()
	second, stopSecond := pluginContents.listen()
	stopFirst()
	if pluginContents.listenerRegistered(first) {
		t.Fatal("first listener was retained")
	}
	if !pluginContents.listenerRegistered(second) {
		t.Fatal("second listener was removed by the first cleanup")
	}
	stopSecond()
	if len(pluginContents.listeners) != 0 {
		t.Fatal("listener cleanup leaked delivery channels")
	}
}

func TestPluginContentRemovalReleasesPendingAction(t *testing.T) {
	resetPluginContentState(t)
	done := make(chan pluginContentDispatchResult, 1)
	session := &pluginContentSession{
		key: pluginContentKey("probe", "view", "pending"),
		pending: &pluginContentPending{
			requestID: "ct-pending",
			done:      done,
			timer:     time.NewTimer(time.Hour),
		},
	}
	pluginContents.add(session)
	pluginContents.mu.Lock()
	pluginContents.pending[session.pending.requestID] = session
	pluginContents.mu.Unlock()
	pluginContents.remove(session)
	select {
	case result := <-done:
		if result.err == nil || !strings.Contains(result.err.Error(), "cancelled") {
			t.Fatalf("cancellation result = %+v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("session removal stranded the pending action waiter")
	}
}

func TestPluginContentConcurrentProjectResetDoesNotLeakSessions(t *testing.T) {
	resetPluginContentState(t)
	m := testPluginContentModel(t, "probe")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			data := testPluginContentData(t)
			data.Key = "tab-" + string(rune('a'+i))
			_, _ = m.openValidatedPluginContentTab(data)
		}(i)
	}
	wg.Wait()
	m.resetProjectState(t.TempDir())
	if got := len(pluginContents.all()); got != 0 {
		t.Fatalf("project reset left %d plugin content sessions", got)
	}
	if len(m.tabs) != 0 {
		t.Fatalf("project reset left %d tabs", len(m.tabs))
	}
}

func TestPluginContentEditorUndoRedoAndReadOnly(t *testing.T) {
	resetPluginContentState(t)
	m := testPluginContentModel(t, "probe")
	data := testPluginContentData(t)
	data.ReadOnly = true
	tab, err := m.openValidatedPluginContentTab(data)
	if err != nil {
		t.Fatal(err)
	}
	if !tab.state.ReadOnly {
		t.Fatal("readOnly payload did not reach the editor")
	}
	tab.state.ReadOnly = false
	tab.state.SetSelections(editor.Selection{Anchor: tab.state.Buffer.Len(), Head: tab.state.Buffer.Len()})
	tab.state.Buffer.Edit(editor.Edit{Start: tab.state.Buffer.Len(), End: tab.state.Buffer.Len(), Text: "x"})
	if !tab.state.Buffer.CanUndo() || !m.pluginContentDirty(tab) {
		t.Fatal("edit did not create undo state and dirty text")
	}
	tab.state.Buffer.Undo()
	if m.pluginContentDirty(tab) {
		t.Fatal("undo did not return to the clean baseline")
	}
	tab.state.Buffer.Redo()
	if !m.pluginContentDirty(tab) {
		t.Fatal("redo did not restore the edit")
	}
}
