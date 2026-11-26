package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
	"mayoi/internal/highlight"
)

const (
	tabKindPluginContent = "pluginContent"

	pluginContentMaxMessageBytes = 2 << 20
	pluginContentMaxContentBytes = 1 << 20
	pluginContentMaxActions      = 16
	pluginContentMaxTabs         = 32
	pluginContentActionTimeout   = 15 * time.Second
)

// pluginContentAction is one action a plugin offers above its content tab.
type pluginContentAction struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Icon    string `json:"icon,omitempty"`
	Primary bool   `json:"primary,omitempty"`
	Save    bool   `json:"save,omitempty"`
}

// pluginContentData is the validated tab payload carried by OpenIDEA's
// WindowRequest and by an HTML view's open-content-tab message.
type pluginContentData struct {
	PluginID string                `json:"pluginId"`
	View     string                `json:"view"`
	Key      string                `json:"key"`
	Title    string                `json:"title"`
	Language string                `json:"language,omitempty"`
	FileName string                `json:"fileName,omitempty"`
	Content  string                `json:"content"`
	ReadOnly bool                  `json:"readOnly"`
	Actions  []pluginContentAction `json:"actions"`
}

// pluginContentDiff is the bounded two-text payload a plugin asks the native
// diff viewer to show.
type pluginContentDiff struct {
	Left       string `json:"left"`
	Right      string `json:"right"`
	LeftLabel  string `json:"leftLabel,omitempty"`
	RightLabel string `json:"rightLabel,omitempty"`
	Title      string `json:"title,omitempty"`
}

// pluginContentReply is validated before it changes a session.
type pluginContentReply struct {
	Content  *string            `json:"content,omitempty"`
	Saved    *bool              `json:"saved,omitempty"`
	ReadOnly *bool              `json:"readOnly,omitempty"`
	Status   *string            `json:"status,omitempty"`
	Error    *string            `json:"error,omitempty"`
	Diff     *pluginContentDiff `json:"diff,omitempty"`
}

type pluginContentCommandAnswer struct {
	Kind  string  `json:"kind"`
	Value *string `json:"value,omitempty"`
}

type pluginContentRequest struct {
	PluginID  string                     `json:"pluginId"`
	RequestID string                     `json:"requestId"`
	Request   pluginContentWindowRequest `json:"request"`
}

type pluginContentWindowRequest struct {
	Kind string          `json:"kind"`
	Tab  json.RawMessage `json:"tab"`
}

type pluginContentTabRequest struct {
	Key      string                `json:"key"`
	Title    string                `json:"title"`
	FileName *string               `json:"fileName"`
	Language *string               `json:"language"`
	Content  string                `json:"content"`
	ReadOnly bool                  `json:"readOnly"`
	Actions  []pluginContentAction `json:"actions"`
}

type pluginContentEvent struct {
	Type      string          `json:"type"`
	PluginID  string          `json:"pluginId"`
	RequestID string          `json:"requestId"`
	Reply     json.RawMessage `json:"reply"`
	Diff      json.RawMessage `json:"diff"`
	FileName  string          `json:"fileName"`
}

// pluginContentSession is the durable part of an editor tab. The virtual
// document is already read-only; edits update this session's editor buffer and
// the baseline records the last text the plugin acknowledged.
type pluginContentSession struct {
	key                 string
	data                pluginContentData
	baseline            string
	readOnly            bool
	closing             bool
	busy                bool
	status              string
	statusErr           bool
	tab                 *Tab
	findTab             *Tab
	project             string
	session             uint64
	pending             *pluginContentPending
	unregister          func()
	owner               *Model
	lifecycleUnregister func()
	closeThen           func()
}

type pluginContentModelState struct {
	suspended bool
}

type pluginContentPending struct {
	requestID string
	action    string
	snapshot  string
	done      chan pluginContentDispatchResult
	timer     *time.Timer
	once      sync.Once
	result    pluginContentDispatchResult
	hasResult bool
}

type pluginContentDispatchResult struct {
	reply    pluginContentReply
	err      error
	snapshot string
}

type pluginContentRegistry struct {
	mu        sync.Mutex
	next      uint64
	sessions  map[string]*pluginContentSession
	pending   map[string]*pluginContentSession
	listeners map[chan pluginContentDispatchResult]struct{}
}

var pluginContents pluginContentRegistry

type pluginContentLegacySurfaceState struct {
	mu       sync.Mutex
	sessions map[*pluginContentSession]bool
}

var pluginContentLegacySurfaces pluginContentLegacySurfaceState

func init() {
	pluginContents.sessions = map[string]*pluginContentSession{}
	pluginContents.pending = map[string]*pluginContentSession{}
	pluginContents.listeners = map[chan pluginContentDispatchResult]struct{}{}
	pluginContentLegacySurfaces.sessions = map[*pluginContentSession]bool{}
}

func pluginContentKey(pluginID, view, key string) string {
	raw, _ := json.Marshal([]string{pluginID, view, key})
	return tabKindPluginContent + ":" + string(raw)
}

func pluginContentText(value string, max int) bool {
	return value != "" && pluginContentUnits(value) <= max &&
		!strings.ContainsRune(value, '\x00') &&
		!strings.ContainsAny(value, "\r\n")
}

func pluginContentBody(value string) bool {
	return pluginContentUnits(value) <= pluginContentMaxContentBytes &&
		!strings.ContainsRune(value, '\x00')
}

func pluginContentLabel(value string, max int) bool {
	return value != "" && pluginContentUnits(value) <= max &&
		!strings.ContainsAny(value, "\x00\r\n")
}

func pluginContentUnits(value string) int {
	return len(utf16.Encode([]rune(value)))
}

func pluginContentAbsent(raw json.RawMessage) bool {
	return len(raw) == 0
}

func pluginContentNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func pluginContentDiffData(left, right string, leftLabel, rightLabel, title string) (pluginContentDiff, error) {
	if !pluginContentBody(left) || !pluginContentBody(right) ||
		(leftLabel != "" && !pluginContentLabel(leftLabel, 128)) ||
		(rightLabel != "" && !pluginContentLabel(rightLabel, 128)) ||
		(title != "" && !pluginContentLabel(title, 256)) {
		return pluginContentDiff{}, errors.New("Invalid content editor configuration")
	}
	return pluginContentDiff{
		Left: left, Right: right,
		LeftLabel: leftLabel, RightLabel: rightLabel, Title: title,
	}, nil
}

// validatePluginContentData applies the upstream webview limits and the
// action checks shared by HTML and WASM content tabs.
func validatePluginContentData(
	pluginID, view string,
	raw json.RawMessage,
	messageSize int,
	pluginEnabled func(string) (bool, error),
) (pluginContentData, error) {
	if messageSize > pluginContentMaxMessageBytes ||
		len(raw) > pluginContentMaxMessageBytes {
		return pluginContentData{}, errors.New("Invalid content editor configuration")
	}
	var value struct {
		Title    json.RawMessage `json:"title"`
		Key      json.RawMessage `json:"key"`
		Language json.RawMessage `json:"language"`
		FileName json.RawMessage `json:"fileName"`
		Content  json.RawMessage `json:"content"`
		ReadOnly json.RawMessage `json:"readOnly"`
		Actions  json.RawMessage `json:"actions"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		return pluginContentData{}, errors.New("Invalid content editor configuration")
	}
	var title string
	if json.Unmarshal(value.Title, &title) != nil || !pluginContentLabel(title, 256) {
		return pluginContentData{}, errors.New("Invalid content editor configuration")
	}
	key := title
	if len(value.Key) > 0 {
		if json.Unmarshal(value.Key, &key) != nil || !pluginContentLabel(key, 128) {
			return pluginContentData{}, errors.New("Invalid content editor configuration")
		}
	}
	language := ""
	if !pluginContentAbsent(value.Language) {
		if pluginContentNull(value.Language) {
			return pluginContentData{}, errors.New("Invalid content editor configuration")
		}
		if json.Unmarshal(value.Language, &language) != nil || !pluginContentLabel(language, 64) {
			return pluginContentData{}, errors.New("Invalid content editor configuration")
		}
	}
	fileName := ""
	if !pluginContentAbsent(value.FileName) {
		if pluginContentNull(value.FileName) {
			return pluginContentData{}, errors.New("Invalid content editor configuration")
		}
		if json.Unmarshal(value.FileName, &fileName) != nil ||
			!pluginContentLabel(fileName, 256) || strings.ContainsAny(fileName, `/\`) {
			return pluginContentData{}, errors.New("Invalid content editor configuration")
		}
	}
	var content string
	if pluginContentAbsent(value.Content) || pluginContentNull(value.Content) ||
		json.Unmarshal(value.Content, &content) != nil ||
		!pluginContentBody(content) {
		return pluginContentData{}, errors.New("Invalid content editor configuration")
	}
	readOnly := false
	if !pluginContentAbsent(value.ReadOnly) {
		if pluginContentNull(value.ReadOnly) {
			return pluginContentData{}, errors.New("Invalid content editor configuration")
		}
		if json.Unmarshal(value.ReadOnly, &readOnly) != nil {
			return pluginContentData{}, errors.New("Invalid content editor configuration")
		}
	}
	var actions []pluginContentAction
	if !pluginContentAbsent(value.Actions) {
		if pluginContentNull(value.Actions) {
			return pluginContentData{}, errors.New("Invalid content editor configuration")
		}
		if json.Unmarshal(value.Actions, &actions) != nil ||
			len(actions) > pluginContentMaxActions {
			return pluginContentData{}, errors.New("Invalid content editor configuration")
		}
	}
	ids := make(map[string]bool, len(actions))
	saveCount := 0
	for i := range actions {
		action := &actions[i]
		if !pluginContentLabel(action.ID, 64) || ids[action.ID] ||
			!pluginContentLabel(action.Label, 128) ||
			(action.Icon != "" && !validBuiltinIcon(action.Icon)) {
			return pluginContentData{}, errors.New("Invalid content editor configuration")
		}
		ids[action.ID] = true
		if action.Save {
			saveCount++
			if saveCount > 1 {
				return pluginContentData{}, errors.New("Invalid content editor configuration")
			}
		}
	}
	if pluginEnabled == nil {
		return pluginContentData{}, errors.New("GoIDEA backend is unavailable")
	}
	enabled, err := pluginEnabled(pluginID)
	if err != nil {
		return pluginContentData{}, err
	}
	if !enabled {
		return pluginContentData{}, errors.New("Invalid content editor configuration")
	}
	if actions == nil {
		actions = []pluginContentAction{}
	}
	return pluginContentData{
		PluginID: pluginID, View: view, Key: key, Title: title,
		Language: language, FileName: fileName, Content: content,
		ReadOnly: readOnly, Actions: actions,
	}, nil
}

func validBuiltinIcon(name string) bool {
	if name == "" || strings.ContainsAny(name, "\x00\r\n") {
		return false
	}
	return oiIconExists(name, 14)
}

func pluginContentSaveAction(data pluginContentData) (pluginContentAction, bool) {
	for _, action := range data.Actions {
		if action.Save {
			return action, true
		}
	}
	for _, action := range data.Actions {
		if action.ID == "save" {
			return action, true
		}
	}
	for _, action := range data.Actions {
		if action.Primary {
			return action, true
		}
	}
	return pluginContentAction{}, false
}

func pluginContentLanguage(data pluginContentData) *editor.Language {
	if data.Language != "" {
		if language := highlight.MyGoLanguage(highlight.LanguageByID(data.Language)); language != nil {
			return language
		}
	}
	name := data.FileName
	if name == "" {
		name = data.Title
	}
	if language := highlight.MyGoLanguage(highlight.LanguageFor(name)); language != nil {
		return language
	}
	return editor.LanguageFor(name)
}

func pluginContentDisplayPath(data pluginContentData) string {
	name := data.FileName
	if name == "" {
		name = data.Title
	}
	name = path.Base(strings.ReplaceAll(name, `\`, "/"))
	if name == "." || name == "/" || name == "" {
		name = "plugin-content"
	}
	return "plugin-content:/" + name
}

func pluginContentDiffPath(data pluginContentData) string {
	name := data.FileName
	if name == "" {
		name = data.Title
	}
	name = path.Base(strings.ReplaceAll(name, `\`, "/"))
	var b strings.Builder
	for _, r := range name {
		if r < 0x20 || r == ':' || r == '/' || r == '\\' {
			continue
		}
		b.WriteRune(r)
	}
	name = strings.TrimSpace(b.String())
	if name == "" {
		name = "diff"
	}
	return "plugin-diff:/" + name
}

func pluginContentOpenDiff(m *Model, diff pluginContentDiff, data pluginContentData) {
	m.OpenDiffArg(map[string]any{
		"path":  pluginContentDiffPath(data),
		"title": pluginContentFirst(diff.Title, data.Title),
		"left": map[string]any{
			"type": "text", "text": diff.Left,
			"label": pluginContentFirst(diff.LeftLabel, "Before"),
		},
		"right": map[string]any{
			"type": "text", "text": diff.Right,
			"label": pluginContentFirst(diff.RightLabel, "After"),
		},
	})
}

func pluginContentFirst(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func (r *pluginContentRegistry) find(key string) *pluginContentSession {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sessions[key]
}

func (r *pluginContentRegistry) add(s *pluginContentSession) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if old := r.sessions[s.key]; old != nil && old != s && old.unregister != nil {
		defer old.unregister()
	}
	r.sessions[s.key] = s
}

func (r *pluginContentRegistry) remove(s *pluginContentSession) {
	if s == nil {
		return
	}
	r.mu.Lock()
	var pending *pluginContentPending
	if r.sessions[s.key] == s {
		delete(r.sessions, s.key)
	}
	if s.pending != nil {
		delete(r.pending, s.pending.requestID)
		if s.pending.timer != nil {
			s.pending.timer.Stop()
		}
		pending = s.pending
		pending.once.Do(func() {
			pending.result = pluginContentDispatchResult{
				err: errors.New("Content editor action was cancelled"),
			}
			pending.hasResult = true
		})
		s.pending = nil
	}
	unregister := s.unregister
	s.unregister = nil
	lifecycleUnregister := s.lifecycleUnregister
	s.lifecycleUnregister = nil
	r.mu.Unlock()
	if unregister != nil {
		unregister()
	}
	if lifecycleUnregister != nil {
		lifecycleUnregister()
	}
	if pending != nil && pending.done != nil {
		select {
		case pending.done <- pending.result:
		default:
		}
	}
}

func (r *pluginContentRegistry) all() []*pluginContentSession {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*pluginContentSession, 0, len(r.sessions))
	for _, session := range r.sessions {
		out = append(out, session)
	}
	return out
}

func (r *pluginContentRegistry) countFor(pluginID, project string, session uint64) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for _, current := range r.sessions {
		if current.data.PluginID == pluginID &&
			current.project == project && current.session == session {
			count++
		}
	}
	return count
}

func (r *pluginContentRegistry) resetProject(root string, session uint64, m *Model) {
	for _, s := range r.all() {
		if s.project == root && s.session == session {
			continue
		}
		if s.tab != nil {
			m.removeTabNow(s.tab)
		}
		r.remove(s)
	}
}

func (r *pluginContentRegistry) reply(requestID, pluginID string, result pluginContentDispatchResult) bool {
	r.mu.Lock()
	s := r.pending[requestID]
	if s == nil || s.pending == nil || s.pending.requestID != requestID ||
		s.data.PluginID != pluginID {
		r.mu.Unlock()
		return false
	}
	delete(r.pending, requestID)
	pending := s.pending
	delivered := false
	pending.once.Do(func() {
		delivered = true
		pending.result = result
		pending.hasResult = true
	})
	if !delivered {
		r.mu.Unlock()
		return true
	}
	s.pending = nil
	if pending.timer != nil {
		pending.timer.Stop()
	}
	result.snapshot = pending.snapshot
	for listener := range r.listeners {
		select {
		case listener <- result:
		default:
		}
	}
	r.mu.Unlock()
	select {
	case pending.done <- result:
	default:
	}
	return true
}

func (r *pluginContentRegistry) finish(s *pluginContentSession, result pluginContentDispatchResult) {
	if s == nil {
		return
	}
	r.mu.Lock()
	pending := s.pending
	if pending == nil {
		r.mu.Unlock()
		return
	}
	var once bool
	pending.once.Do(func() {
		once = true
		pending.result = result
		pending.hasResult = true
	})
	if !once {
		r.mu.Unlock()
		return
	}
	s.pending = nil
	if r.pending[pending.requestID] == s {
		delete(r.pending, pending.requestID)
	}
	result.snapshot = pending.snapshot
	for listener := range r.listeners {
		select {
		case listener <- result:
		default:
		}
	}
	if pending.timer != nil {
		pending.timer.Stop()
	}
	r.mu.Unlock()
	select {
	case pending.done <- result:
	default:
	}
}

func (r *pluginContentRegistry) complete(s *pluginContentSession, result pluginContentDispatchResult) {
	if s == nil {
		return
	}
	r.mu.Lock()
	pending := s.pending
	if pending == nil {
		r.mu.Unlock()
		return
	}
	delivered := false
	pending.once.Do(func() {
		delivered = true
		pending.result = result
		pending.hasResult = true
	})
	if !delivered {
		r.mu.Unlock()
		return
	}
	s.pending = nil
	if r.pending[pending.requestID] == s {
		delete(r.pending, pending.requestID)
	}
	result.snapshot = pending.snapshot
	if pending.timer != nil {
		pending.timer.Stop()
	}
	for listener := range r.listeners {
		select {
		case listener <- result:
		default:
		}
	}
	r.mu.Unlock()
	select {
	case pending.done <- result:
	default:
	}
}

func (r *pluginContentRegistry) timeout(s *pluginContentSession, result pluginContentDispatchResult) {
	if s == nil {
		return
	}
	r.mu.Lock()
	if s.pending == nil {
		r.mu.Unlock()
		return
	}
	pending := s.pending
	delivered := false
	pending.once.Do(func() {
		delivered = true
		pending.result = result
		pending.hasResult = true
	})
	if !delivered {
		r.mu.Unlock()
		return
	}
	s.pending = nil
	if r.pending[pending.requestID] == s {
		delete(r.pending, pending.requestID)
	}
	result.snapshot = pending.snapshot
	for listener := range r.listeners {
		select {
		case listener <- result:
		default:
		}
	}
	r.mu.Unlock()
	select {
	case pending.done <- result:
	default:
	}
}

func (r *pluginContentRegistry) listen() (chan pluginContentDispatchResult, func()) {
	ch := make(chan pluginContentDispatchResult, 32)
	r.mu.Lock()
	r.listeners[ch] = struct{}{}
	r.next++
	generation := r.next
	r.mu.Unlock()
	var once sync.Once
	return ch, func() {
		once.Do(func() {
			r.mu.Lock()
			if r.next == generation {
				clear(r.listeners)
			} else {
				delete(r.listeners, ch)
			}
			r.mu.Unlock()
		})
	}
}

func (r *pluginContentRegistry) listenerRegistered(ch chan pluginContentDispatchResult) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.listeners[ch]
	return ok
}

func validatePluginContentReply(raw json.RawMessage) (pluginContentReply, error) {
	var reply pluginContentReply
	if len(raw) == 0 || string(raw) == "null" || json.Unmarshal(raw, &reply) != nil {
		return pluginContentReply{}, errors.New("Invalid content editor configuration")
	}
	if reply.Content != nil && !pluginContentBody(*reply.Content) {
		return pluginContentReply{}, errors.New("Invalid content editor configuration")
	}
	if reply.Status != nil && pluginContentUnits(*reply.Status) > 512 {
		return pluginContentReply{}, errors.New("Invalid content editor configuration")
	}
	if reply.Error != nil && pluginContentUnits(*reply.Error) > 1024 {
		return pluginContentReply{}, errors.New("Invalid content editor configuration")
	}
	if reply.Diff != nil {
		diff, err := pluginContentDiffData(
			reply.Diff.Left, reply.Diff.Right,
			reply.Diff.LeftLabel, reply.Diff.RightLabel, reply.Diff.Title,
		)
		if err != nil {
			return pluginContentReply{}, err
		}
		reply.Diff = &diff
	}
	return reply, nil
}

func (m *Model) pluginContentEnabled(pluginID string) (bool, error) {
	if m == nil {
		return false, errors.New("GoIDEA backend is unavailable")
	}
	ctx, cancel := context.WithTimeout(m.remoteBaseContext(), 30*time.Second)
	defer cancel()
	snapshot, err := m.pluginListSnapshot(ctx)
	if err != nil {
		return false, err
	}
	for _, plugin := range snapshot.Plugins {
		if plugin.ID == pluginID {
			return plugin.Enabled, nil
		}
	}
	return false, nil
}

func (m *Model) answerPluginContentWindowRequest(ctx context.Context, requestID string, answer pluginContentCommandAnswer) (bool, error) {
	if m == nil || requestID == "" {
		return false, nil
	}
	if m.pluginContentSuspended() {
		return false, errors.New("the project is changing")
	}
	raw, err := m.invoke(ctx, "plugin_answer_command", map[string]any{
		"requestId": requestID,
		"answer":    answer,
	})
	if err != nil {
		return false, err
	}
	if len(raw) == 0 || string(raw) == "null" {
		return false, nil
	}
	var ok bool
	if err := json.Unmarshal(raw, &ok); err != nil {
		return false, err
	}
	return ok, nil
}

func (m *Model) openPluginContentTab(pluginID, view string, raw json.RawMessage, messageSize int) (*Tab, error) {
	data, err := validatePluginContentData(pluginID, view, raw, messageSize, m.pluginContentEnabled)
	if err != nil {
		return nil, err
	}
	return m.openValidatedPluginContentTab(data)
}

func (m *Model) acceptPluginContentEvent(data json.RawMessage) bool {
	var header struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(data, &header) != nil {
		return false
	}
	switch header.Type {
	case "windowRequest":
		return m.acceptPluginContentWindowRequest(data)
	case "contentTabReply":
		var event struct {
			PluginID  string          `json:"pluginId"`
			RequestID string          `json:"requestId"`
			Reply     json.RawMessage `json:"reply"`
		}
		if json.Unmarshal(data, &event) != nil {
			return true
		}
		reply, err := validatePluginContentReply(event.Reply)
		pluginContents.reply(
			event.RequestID,
			event.PluginID,
			pluginContentDispatchResult{reply: reply, err: err},
		)
		return true
	case "openDiff":
		var event struct {
			Diff     json.RawMessage `json:"diff"`
			FileName string          `json:"fileName"`
		}
		if json.Unmarshal(data, &event) != nil {
			return true
		}
		diff, err := validatePluginContentReply(
			json.RawMessage(`{"diff":` + string(event.Diff) + `}`),
		)
		if err == nil && diff.Diff != nil {
			m.openPluginDiffPayload(*diff.Diff, event.FileName)
		}
		return true
	}
	return false
}

func (m *Model) acceptPluginContentWindowRequest(data json.RawMessage) bool {
	var event struct {
		PluginID  string                     `json:"pluginId"`
		RequestID string                     `json:"requestId"`
		Request   pluginContentWindowRequest `json:"request"`
	}
	if json.Unmarshal(data, &event) != nil ||
		event.RequestID == "" || event.Request.Kind != "contentTab" ||
		len(event.Request.Tab) == 0 {
		return false
	}
	if m.pluginContentSuspended() {
		_, _ = m.answerPluginContentWindowRequest(
			context.Background(), event.RequestID,
			pluginContentCommandAnswer{
				Kind: "failed", Value: pluginContentStringPtr("the project is changing"),
			},
		)
		return true
	}
	var tab pluginContentTabRequest
	if json.Unmarshal(event.Request.Tab, &tab) != nil {
		m.answerPluginContentWindowRequest(
			context.Background(), event.RequestID,
			pluginContentCommandAnswer{
				Kind:  "failed",
				Value: pluginContentStringPtr("Invalid content editor configuration"),
			},
		)
		return true
	}
	raw, err := json.Marshal(pluginContentData{
		Key: tab.Key, Title: tab.Title, FileName: pluginContentString(tab.FileName),
		Language: pluginContentString(tab.Language), Content: tab.Content,
		ReadOnly: tab.ReadOnly, Actions: tab.Actions,
	})
	if err == nil {
		_, err = m.openPluginContentTab(event.PluginID, "@wasm-content", raw, len(raw))
	}
	if err != nil {
		_, _ = m.answerPluginContentWindowRequest(
			context.Background(), event.RequestID,
			pluginContentCommandAnswer{
				Kind: "failed", Value: pluginContentStringPtr(err.Error()),
			},
		)
		return true
	}
	if _, err := m.answerPluginContentWindowRequest(
		context.Background(), event.RequestID,
		pluginContentCommandAnswer{Kind: "value"},
	); err != nil {
		m.status = "Could not acknowledge the plugin content tab: " + err.Error()
		m.invalidate()
	}
	return true
}

func (m *Model) openPluginDiffPayload(diff pluginContentDiff, fileName string) {
	m.OpenDiffArg(map[string]any{
		"path":  pluginContentDiffPath(pluginContentData{Title: fileName}),
		"title": pluginContentFirst(diff.Title, fileName, "Diff"),
		"left": map[string]any{
			"type": "text", "text": diff.Left,
			"label": pluginContentFirst(diff.LeftLabel, "Before"),
		},
		"right": map[string]any{
			"type": "text", "text": diff.Right,
			"label": pluginContentFirst(diff.RightLabel, "After"),
		},
	})
}

func pluginContentString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func pluginContentStringPtr(value string) *string { return &value }

func (m *Model) openValidatedPluginContentTab(data pluginContentData) (*Tab, error) {
	if m == nil {
		return nil, errors.New("GoIDEA backend is unavailable")
	}
	key := pluginContentKey(data.PluginID, data.View, data.Key)
	if existing := pluginContents.find(key); existing != nil && existing.tab != nil {
		if existing.project != m.Root() || existing.session != m.workspaceGeneration {
			_ = m.closePluginContentSession(existing)
		} else {
			existing.update(data)
			m.showPluginContentTab(existing.tab)
			return existing.tab, nil
		}
	}
	if pluginContents.countFor(data.PluginID, m.Root(), m.workspaceGeneration) >= pluginContentMaxTabs {
		return nil, errors.New("At most 32 plugin content tabs can be open")
	}
	doc := docbridge.Virtual(pluginContentDisplayPath(data), data.Content)
	t := &Tab{
		owner:     m,
		doc:       doc,
		state:     &editor.State{Buffer: doc.Buffer(), Bindings: m.editorBindings()},
		kind:      tabKindPluginContent,
		pageTitle: data.Title,
		pagePath:  key,
	}
	t.state.OnGutterClick = func(int) {}
	t.state.OnCompletion = nil
	t.state.OnHover = nil
	t.state.OnDefinition = nil
	t.state.ContextMenu = nil
	t.state.OnGutterClick = nil
	t.state.RenderCompletions = nil
	s := &pluginContentSession{
		key: key, data: data, baseline: data.Content,
		readOnly: data.ReadOnly, tab: t,
		project: m.Root(), session: m.workspaceGeneration, owner: m,
	}
	t.content = s
	pluginContents.add(s)
	existing := pluginContents.find(key)
	if existing != s {
		return nil, errors.New("The plugin content tab was replaced")
	}
	s.unregister = registerPluginContentController(key, s)
	s.lifecycleUnregister = registerPluginContentController(key+":view", s)
	m.configurePluginContentTab(t, s)
	if m.split != nil {
		m.focusGroup(groupFocused)
	}
	m.dropPreviewTabs()
	m.insertTab(t)
	m.showPluginContentTab(t)
	m.saveSession()
	return t, nil
}

func (m *Model) configurePluginContentTab(t *Tab, s *pluginContentSession) {
	t.state.LineHeight = editorLineHeight(m.editorFontSize())
	t.state.Font = ui.Font{
		Family: m.prefs.Font.fontFamily(), Size: m.editorFontSize(),
		Weight: m.prefs.Font.weight(), Features: m.prefs.Font.features(),
	}
	t.state.ReadOnly = s.readOnly
	t.state.Label = s.data.Title
	t.state.Buffer.SetLanguage(pluginContentLanguage(s.data))
	m.applyTabZenGutter(t)
	m.applyTabFoldingDefaults(t, m.prefs.Editor)
	t.state.OnInput = func(ev ui.InputEvent) bool {
		if m.editorCaretInput(t, ev) {
			return true
		}
		if s.busy || s.closing {
			return true
		}
		return false
	}
	t.synced = t.doc.Buffer().Version()
	t.seen = t.synced
}

func (m *Model) duplicatePluginContentTab(t *Tab, session *pluginContentSession) *Tab {
	if m == nil || t == nil || session == nil {
		return nil
	}
	dup := *t
	dup.owner = m
	dup.content = session
	dup.state = &editor.State{
		Buffer: t.state.Buffer, LineHeight: t.state.LineHeight,
		Font: t.state.Font, Bindings: m.editorBindings(),
		ReadOnly: session.readOnly, Label: session.data.Title,
	}
	dup.state.Buffer.SetLanguage(pluginContentLanguage(session.data))
	dup.synced = dup.state.Buffer.Version()
	dup.seen = dup.synced
	return &dup
}

func (m *Model) showPluginContentTab(t *Tab) {
	if t == nil {
		return
	}
	m.showingWelcome = false
	if g, ok := m.groupOf(t); !ok {
		return
	} else if g == groupSplit {
		m.focusGroup(groupSplit)
	}
	m.active = m.tabIndex(t)
	m.noteTabUse(t)
	m.revealEditor()
	m.applyActive()
	m.invalidate()
}

func (s *pluginContentSession) update(data pluginContentData) {
	if s == nil {
		return
	}
	wasDirty := s.dirty()
	s.data = data
	s.readOnly = data.ReadOnly
	if s.tab != nil {
		s.tab.pageTitle = data.Title
		s.tab.state.ReadOnly = data.ReadOnly
		s.tab.state.Label = data.Title
		s.tab.state.Buffer.SetLanguage(pluginContentLanguage(data))
	}
	if !wasDirty {
		s.baseline = data.Content
		if s.tab != nil && s.tab.state.Buffer.String() != data.Content {
			s.tab.state.Buffer.Set(data.Content)
		}
	}
}

func (s *pluginContentSession) text() string {
	if s == nil || s.tab == nil || s.tab.state == nil || s.tab.state.Buffer == nil {
		return ""
	}
	return s.tab.state.Buffer.String()
}

func (s *pluginContentSession) dirty() bool {
	return s != nil && s.text() != s.baseline
}

func (s *pluginContentSession) Active() bool {
	if s == nil {
		return false
	}
	if m := s.model(); m != nil && m.pluginContentVisible(s) {
		return m.tab() != nil && m.pluginContentTabFor(m.tab()) == s
	}
	return false
}

func (s *pluginContentSession) Find() {
	m := s.model()
	if m == nil {
		return
	}
	if m.pluginContentVisible(s) && s.tab != nil {
		m.showPluginContentTab(s.tab)
		m.projectTreeFocus = false
		m.focusApply = true
		s.tab.state.Run(editor.Find)
		m.invalidate()
	}
}

func (t *Tab) model() *Model {
	if t == nil {
		return nil
	}
	return t.owner
}

func (s *pluginContentSession) RunAction(id string) {
	if s == nil {
		return
	}
	m := s.model()
	if m == nil {
		return
	}
	if s.busy || s.closing {
		m.post(func() { m.finishPluginContentPending(s) })
		return
	}
	var declared bool
	for _, action := range s.data.Actions {
		if action.ID == id {
			declared = true
			break
		}
	}
	if !declared {
		s.setStatus("The plugin content action "+id+" is not available.", true)
		m.invalidate()
		return
	}
	if m.pluginContentVisible(s) && s.tab != nil {
		m.showPluginContentTab(s.tab)
	}
	enabled, err := m.pluginContentEnabled(s.data.PluginID)
	if err != nil {
		s.setStatus(err.Error(), true)
		m.invalidate()
		return
	}
	if !enabled {
		s.setStatus("Invalid content editor configuration", true)
		m.invalidate()
		return
	}
	s.startAction(id, false)
}

func (m *Model) pluginContentActive() bool {
	t := m.tab()
	return t != nil && t.kind == tabKindPluginContent
}

func (s *pluginContentSession) findTabForModel(m *Model) *Tab {
	if m == nil {
		return nil
	}
	for _, tab := range m.allTabs() {
		if tab != nil && tab.kind == tabKindPluginContent && tab.pagePath == s.key {
			return tab
		}
	}
	return nil
}

func (s *pluginContentSession) model() *Model {
	if s == nil {
		return nil
	}
	if s.owner != nil && !s.owner.pluginContentSuspended() {
		return s.owner
	}
	if s.tab != nil {
		return s.tab.model()
	}
	return nil
}

func (s *pluginContentSession) setStatus(text string, isError bool) {
	if s == nil {
		return
	}
	s.status = text
	s.statusErr = isError
}

func (s *pluginContentSession) startAction(actionID string, fromSave bool) {
	m := s.model()
	if m == nil {
		return
	}
	if s.pending != nil {
		m.post(func() { m.finishPluginContentPending(s) })
		return
	}
	if s.closing || s.busy {
		return
	}
	enabled, err := m.pluginContentEnabled(s.data.PluginID)
	if err != nil {
		s.setStatus(err.Error(), true)
		m.invalidate()
		return
	}
	if !enabled {
		s.setStatus("Invalid content editor configuration", true)
		m.invalidate()
		return
	}
	snapshot := s.text()
	s.busy = true
	s.setStatus("", false)
	m.invalidate()
	ctx, cancel := context.WithTimeout(m.remoteBaseContext(), pluginContentActionTimeout+time.Second)
	defer cancel()
	done := make(chan pluginContentDispatchResult, 1)
	requestID := pluginContentRequestID()
	s.pending = &pluginContentPending{
		requestID: requestID, action: actionID, snapshot: snapshot, done: done,
	}
	pluginContents.mu.Lock()
	pluginContents.pending[requestID] = s
	pluginContents.mu.Unlock()
	s.pending.timer = time.AfterFunc(pluginContentActionTimeout, func() {
		pluginContents.timeout(s, pluginContentDispatchResult{
			err: errors.New("Content editor action timed out"),
		})
	})
	go func() {
		if _, err := s.pluginContentAction(ctx, requestID, actionID, snapshot); err != nil {
			pluginContents.finish(s, pluginContentDispatchResult{err: err})
		}
		result := <-done
		m.post(func() { m.finishPluginContentAction(s, fromSave, result) })
	}()
}

func (m *Model) finishPluginContentPending(s *pluginContentSession) {
	if s == nil || s.pending == nil {
		return
	}
	done := s.pending.done
	go func() {
		result := <-done
		m.post(func() { m.finishPluginContentAction(s, false, result) })
	}()
}

func (m *Model) savePluginContentThen(session *pluginContentSession, closing bool, then func()) {
	if session == nil || m.tabForPluginContent(session) == nil {
		return
	}
	if closing && !session.closing {
		m.beginPluginContentClose(session)
	}
	if session.busy {
		return
	}
	action, ok := pluginContentSaveAction(session.data)
	if !ok {
		m.status = "No save action is configured for this tab"
		m.invalidate()
		return
	}
	if session.readOnly && !closing {
		m.status = "This plugin content is read-only"
		m.invalidate()
		return
	}
	enabled, err := m.pluginContentEnabled(session.data.PluginID)
	if err != nil {
		m.status = err.Error()
		m.invalidate()
		return
	}
	if !enabled {
		m.status = "Invalid content editor configuration"
		m.invalidate()
		return
	}
	session.closing = closing
	if closing {
		for _, tab := range m.allTabs() {
			if tab != nil && m.pluginContentTabFor(tab) == session {
				tab.state.ReadOnly = true
			}
		}
	}
	snapshot := session.text()
	session.busy = true
	session.setStatus("", false)
	m.invalidate()
	done := make(chan pluginContentDispatchResult, 1)
	requestID := pluginContentRequestID()
	session.pending = &pluginContentPending{
		requestID: requestID, action: action.ID, snapshot: snapshot, done: done,
	}
	pluginContents.mu.Lock()
	pluginContents.pending[requestID] = session
	pluginContents.mu.Unlock()
	session.pending.timer = time.AfterFunc(pluginContentActionTimeout, func() {
		pluginContents.timeout(session, pluginContentDispatchResult{
			err: errors.New("Content editor action timed out"),
		})
	})
	ctx, cancel := context.WithTimeout(m.remoteBaseContext(), pluginContentActionTimeout+time.Second)
	defer cancel()
	go func() {
		if _, err := session.pluginContentAction(ctx, requestID, action.ID, snapshot); err != nil {
			pluginContents.finish(session, pluginContentDispatchResult{err: err})
		}
		result := <-done
		m.post(func() {
			before := session.baseline
			m.finishPluginContentAction(session, true, result)
			ok := result.err == nil &&
				(result.reply.Error == nil || *result.reply.Error == "") &&
				(result.reply.Saved == nil || *result.reply.Saved) &&
				(session.baseline != before || !session.dirty())
			if closing && ok {
				m.closePluginContentSession(session)
			} else if closing {
				for _, tab := range m.allTabs() {
					if tab != nil && m.pluginContentTabFor(tab) == session {
						tab.state.ReadOnly = session.readOnly
					}
				}
				session.closing = false
			}
			if then != nil && ok {
				then()
			}
		})
	}()
}

func (m *Model) beginPluginContentClose(session *pluginContentSession) {
	if session == nil {
		return
	}
	session.closing = true
	for _, tab := range m.allTabs() {
		if tab != nil && m.pluginContentTabFor(tab) == session {
			tab.state.ReadOnly = true
		}
	}
}

func (m *Model) cancelPluginContentClose(session *pluginContentSession) {
	if session == nil {
		return
	}
	session.closing = false
	for _, tab := range m.allTabs() {
		if tab != nil && m.pluginContentTabFor(tab) == session {
			tab.state.ReadOnly = session.readOnly
		}
	}
}

func (m *Model) pluginContentSaveForLifecycle(ctx context.Context, tab *Tab) error {
	session := m.pluginContentTabFor(tab)
	if session == nil {
		return errors.New("The plugin content editor is no longer available")
	}
	if session.busy {
		return errors.New("The plugin content action is still running")
	}
	action, ok := pluginContentSaveAction(session.data)
	if !ok {
		return errors.New("No save action is configured for this tab")
	}
	enabled, err := m.pluginContentEnabled(session.data.PluginID)
	if err != nil {
		return err
	}
	if !enabled {
		return errors.New("Invalid content editor configuration")
	}
	if session.pending != nil {
		resultCh := session.pending.done
		if resultCh != nil {
			result := <-resultCh
			if result.err != nil {
				return result.err
			}
			if result.reply.Error != nil && *result.reply.Error != "" {
				return errors.New(*result.reply.Error)
			}
			if result.reply.Saved != nil && !*result.reply.Saved {
				return errors.New("The plugin did not save the content")
			}
		}
	}
	done := make(chan pluginContentDispatchResult, 1)
	requestID := pluginContentRequestID()
	snapshot := session.text()
	session.busy = true
	session.pending = &pluginContentPending{
		requestID: requestID, action: action.ID, snapshot: snapshot, done: done,
	}
	pluginContents.mu.Lock()
	pluginContents.pending[requestID] = session
	pluginContents.mu.Unlock()
	resultCh := make(chan pluginContentDispatchResult, 1)
	session.pending.done = resultCh
	go func() {
		if _, err := session.pluginContentAction(ctx, requestID, action.ID, snapshot); err != nil {
			pluginContents.finish(session, pluginContentDispatchResult{err: err})
		}
	}()
	result := <-resultCh
	session.busy = false
	if result.err != nil {
		return result.err
	}
	if result.reply.Error != nil && *result.reply.Error != "" {
		return errors.New(*result.reply.Error)
	}
	if result.reply.Saved != nil && !*result.reply.Saved {
		return errors.New("The plugin did not save the content")
	}
	if session.text() == snapshot {
		session.baseline = snapshot
	}
	if result.reply.Content != nil {
		session.baseline = *result.reply.Content
		if session.text() == snapshot {
			session.tab.state.Buffer.Set(*result.reply.Content)
		}
	}
	return nil
}

func (m *Model) pluginContentCloseTab(t *Tab) {
	if m == nil || t == nil || t.kind != tabKindPluginContent {
		return
	}
	if m.hasTab(t) {
		m.removeTabNow(t)
	}
	if session := pluginContents.find(t.pagePath); session != nil && session.tab == t {
		pluginContents.remove(session)
	}
}

var pluginContentRequestSeq struct {
	sync.Mutex
	value uint64
}

func pluginContentRequestID() string {
	pluginContentRequestSeq.Lock()
	defer pluginContentRequestSeq.Unlock()
	pluginContentRequestSeq.value++
	return fmt.Sprintf("ct-%d", pluginContentRequestSeq.value)
}

func (s *pluginContentSession) pluginContentAction(
	ctx context.Context,
	requestID, actionID, content string,
) (*pluginContentReply, error) {
	m := s.model()
	if s == nil || m == nil {
		return nil, errors.New("The plugin content editor is no longer available")
	}
	_, err := m.invoke(ctx, "plugin_content_tab_action", map[string]any{
		"pluginId": s.data.PluginID,
		"action": map[string]any{
			"requestId": requestID,
			"key":       s.data.Key,
			"action":    actionID,
			"content":   content,
		},
	})
	return nil, err
}

func (m *Model) finishPluginContentAction(s *pluginContentSession, fromSave bool, result pluginContentDispatchResult) {
	if s == nil || s.tab == nil || m.tabForPluginContent(s) == nil {
		return
	}
	s.busy = false
	if s.project != m.Root() || s.session != m.workspaceGeneration {
		pluginContents.remove(s)
		return
	}
	if result.err != nil {
		s.setStatus(result.err.Error(), true)
		m.projectTreeFocus = false
		if fromSave {
			m.status = result.err.Error()
		}
		m.invalidate()
		return
	}
	reply := result.reply
	if reply.Error != nil && *reply.Error != "" {
		s.setStatus(*reply.Error, true)
		m.projectTreeFocus = false
		if fromSave {
			m.status = *reply.Error
		}
		m.invalidate()
		return
	}
	if fromSave && reply.Saved != nil && !*reply.Saved {
		s.setStatus("The plugin did not save the content", true)
		m.projectTreeFocus = false
		m.status = "The plugin did not save the content"
		m.invalidate()
		return
	}
	snapshot := result.snapshot
	if reply.ReadOnly != nil {
		s.readOnly = *reply.ReadOnly
		s.tab.state.ReadOnly = *reply.ReadOnly
	}
	if reply.Content != nil && s.text() == snapshot {
		s.tab.state.Buffer.Set(*reply.Content)
		if fromSave || reply.Saved == nil || *reply.Saved {
			s.baseline = *reply.Content
		}
	} else if reply.Content != nil && result.snapshot != "" {
		if reply.Saved == nil || *reply.Saved {
			s.baseline = *reply.Content
		}
	} else if reply.Saved != nil && *reply.Saved {
		s.baseline = s.text()
	} else if fromSave && s.text() == result.snapshot {
		s.baseline = result.snapshot
	} else if fromSave {
		s.baseline = result.snapshot
	}
	if reply.Status != nil {
		s.setStatus(*reply.Status, false)
		m.projectTreeFocus = false
		m.status = *reply.Status
	} else if fromSave {
		s.setStatus("", false)
		m.status = "Saved " + s.data.Title
		m.projectTreeFocus = false
	}
	if reply.Diff != nil {
		pluginContentOpenDiff(m, *reply.Diff, s.data)
	}
	m.applyActive()
	m.invalidate()
	if s.closeThen != nil {
		then := s.closeThen
		s.closeThen = nil
		then()
	}
}

func (m *Model) tabForPluginContent(s *pluginContentSession) *Tab {
	if s == nil || s.tab == nil || m == nil {
		return nil
	}
	if _, ok := m.groupOf(s.tab); ok {
		return s.tab
	}
	return nil
}

func (m *Model) closePluginContentSession(s *pluginContentSession) error {
	if s == nil {
		return nil
	}
	if s.pending != nil {
		pluginContents.timeout(s, pluginContentDispatchResult{
			err: errors.New("The content editor action was cancelled"),
		})
	}
	if s.tab != nil && m.hasTab(s.tab) {
		m.removeTabNow(s.tab)
	}
	pluginContents.remove(s)
	return nil
}

func (m *Model) pluginContentTabFor(t *Tab) *pluginContentSession {
	if t == nil || t.kind != tabKindPluginContent {
		return nil
	}
	if t.content != nil {
		t.content.owner = m
		return t.content
	}
	if s := pluginContents.find(t.pagePath); s != nil && s.tab == t {
		return s
	}
	for _, s := range pluginContents.all() {
		if s.tab == t {
			return s
		}
	}
	return nil
}

func (m *Model) suspendPluginContentForModel() {
	if m == nil {
		return
	}
	m.pluginContentMu.Lock()
	defer m.pluginContentMu.Unlock()
	m.pluginContent.suspended = true
	for _, tab := range m.allTabs() {
		if tab != nil && tab.kind == tabKindPluginContent && tab.content != nil {
			tab.content.owner = m
		}
	}
}

func (m *Model) resumePluginContentForModel() {
	if m == nil {
		return
	}
	m.pluginContentMu.Lock()
	m.pluginContent.suspended = false
	m.pluginContentMu.Unlock()
}

func (m *Model) pluginContentSuspended() bool {
	if m == nil {
		return true
	}
	m.pluginContentMu.RLock()
	defer m.pluginContentMu.RUnlock()
	return m.pluginContent.suspended
}

func (m *Model) pluginContentVisible(s *pluginContentSession) bool {
	return s != nil && (s.owner == m || s.tab != nil && s.tab.owner == m)
}

func (m *Model) pluginContentDirty(t *Tab) bool {
	s := m.pluginContentTabFor(t)
	return s != nil && s.dirty()
}

func (m *Model) pluginContentTitle(t *Tab) string {
	if s := m.pluginContentTabFor(t); s != nil {
		return s.data.Title
	}
	if t != nil && t.pageTitle != "" {
		return t.pageTitle
	}
	return "Plugin Content"
}

func (m *Model) pluginContentPage(c *ui.Context, p *ui.Theme, t *Tab) {
	s := m.pluginContentTabFor(t)
	if s == nil || s.tab != t {
		ui.Row(c).Grow(1).Padding(16).Children(func() {
			ui.Text(c, "This plugin content tab is no longer available.").TextColor(p.TextMuted)
		})
		return
	}
	oi := oiPaletteOf(p)
	var findTarget string
	defer func() {
		if findTarget == "" {
			return
		}
		unregister := registerPluginContentController(findTarget, s)
		if s.lifecycleUnregister != nil {
			s.lifecycleUnregister()
		}
		s.lifecycleUnregister = unregister
	}()
	findTarget = t.pagePath + ":view"
	ui.Column(c.Key("plugin-content:" + s.key)).Grow(1).MinHeight(0).MinWidth(0).
		Background(p.Background).Children(func() {
		ui.Row(c).Height(oiHeaderHeight).PaddingStart(10).PaddingEnd(8).Gap(8).
			AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(p.Border).
			Shrink(0).Children(func() {
			ui.Text(c, s.data.Title).FontSize(12).TextColor(p.Text).SingleLine().
				Ellipsis("…").Grow(1)
			if s.dirty() {
				ui.Text(c, "• Modified").FontSize(11.5).TextColor(p.Warning).SingleLine()
			}
			if s.status != "" {
				color := p.TextMuted
				if s.statusErr {
					color = p.Danger
				}
				ui.Text(c, s.status).FontSize(11.5).TextColor(color).SingleLine().
					Ellipsis("…").MaxWidth(360).Tooltip(s.status)
			}
			ui.Row(c).Size(24, 24).Radius(4).AlignItems(ui.Center).Cursor(ui.CursorPointer).
				Justify(ui.Center).Tooltip("Find in Editor").Children(func() {
				oiIconEl(c, "general/search", 14, oi, oi.icon)
			}).OnClick(func() {
				m.RunActionWithArg(pluginServicesContentFindActionID, t.pagePath+":view")
			})
			for _, action := range s.data.Actions {
				label := action.Label
				button := ui.Button(c, label).Disabled(s.busy || s.closing)
				if action.Primary {
					button = ui.PrimaryButton(c, label).Disabled(s.busy || s.closing)
				}
				button.OnClick(func() {
					_ = m.RunActionWithArg(pluginServicesContentActionID, map[string]any{
						"target": t.pagePath + ":view", "id": action.ID,
					})
				})
			}
		})
		view := editor.View(c, t.state).Grow(1).MinHeight(0)
		t.editorFocused = view.Focused() || view.FocusWithin()
		if t.editorFocused {
			m.projectTreeFocus = false
		}
	})
}

// pluginContentProjectReset removes sessions whose project generation has
// ended. resetProjectState clears the tab strips before asking this to avoid
// touching a newly built strip.
func (m *Model) pluginContentProjectReset(root string, generation uint64) {
	for _, session := range pluginContents.all() {
		if session.project == root && session.session == generation {
			continue
		}
		pluginContents.remove(session)
	}
}
