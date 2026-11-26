package app

// OpenIDEA's code plugins can declare views with `location = "services"`.
// The Rust/Wasm plugin runtime stays in openidea-server. This file projects its
// rendered tree/list, toolbar, detail panes and item menus into GoIDEA's native
// Services provider registry.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

const (
	pluginServicesSupervisorID    = "goidea.plugin-services"
	pluginServicesSupervisorOrder = 899
	pluginServicesProviderOrder   = 900

	pluginServicesEventQueue = 64

	pluginServicesLogFindActionID           = "plugins.log.find"
	pluginServicesLogClearActionID          = "plugins.log.clear"
	pluginServicesLogWrapActionID           = "plugins.log.wrap"
	pluginServicesLogToggleArgumentActionID = "plugins.log.toggleArgument"
	pluginServicesContentFindActionID       = "plugins.content.find"
	pluginServicesContentActionID           = "plugins.content.action"
)

func decodePluginJSON(raw []byte, out any) error {
	return json.NewDecoder(bytes.NewReader(raw)).Decode(out)
}

// pluginServicesAPI is the subset of openidea.Manager that this projection
// uses. Tests inject the same contract without starting a server.
type pluginServicesAPI interface {
	PluginsList(context.Context, string) (openidea.PluginsSnapshot, error)
	PluginContextKeys(context.Context, string) ([]openidea.PluginContextKey, error)
	PluginViewRender(context.Context, string, string, string) (openidea.PluginView, error)
	PluginViewEvent(context.Context, string, string, string, openidea.PluginViewEvent) error
	PluginServiceDetail(context.Context, string, string, string, string) ([]openidea.PluginServiceTab, error)
	PluginServiceLog(
		context.Context,
		string, string, string, string, string,
		uint32, uint32, []string,
	) (*int, error)
	PluginRunCommand(context.Context, string, string, string, string) error
	PluginCancel(context.Context, string, uint32) error
	Listen(int) (<-chan openidea.Notification, func())
}

type pluginServicesViewKey struct {
	pluginID string
	viewID   string
}

func (k pluginServicesViewKey) providerID() string {
	return "plugin." + k.pluginID + "." + k.viewID
}

func (k pluginServicesViewKey) keyPrefix() string {
	return "plugin:" + k.pluginID + "/" + k.viewID + "/"
}

type pluginServicesViewDecl struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Icon     *string `json:"icon"`
	Location string  `json:"location"`
	When     *string `json:"when"`
}

type pluginServicesCommandDecl struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	When     *string `json:"when"`
	Icon     *string `json:"icon"`
	Category *string `json:"category"`
}

type pluginServicesMenuDecl struct {
	Menu         string  `json:"menu"`
	View         *string `json:"view"`
	Command      string  `json:"command"`
	When         *string `json:"when"`
	Submenu      *string `json:"submenu"`
	CommandTitle *string `json:"commandTitle"`
}

type pluginServicesCatalog struct {
	pluginNames map[string]string
	views       []pluginServicesViewCatalog
	commands    map[string]pluginServicesCommandDecl
	menus       []pluginServicesMenuCatalog
}

type pluginServicesViewCatalog struct {
	key       pluginServicesViewKey
	decl      pluginServicesViewDecl
	rawDecl   string
	index     int
	signature string
}

type pluginServicesMenuCatalog struct {
	pluginID string
	decl     pluginServicesMenuDecl
}

// pluginServicesRuntime owns one Services-visible session. All catalog and
// view state is mutated on the main thread; its two goroutines only perform
// RPC work and post state back through Model.post.
type pluginServicesRuntime struct {
	m    *Model
	page *pluginPage
	api  pluginServicesAPI

	ctx    context.Context
	cancel context.CancelFunc

	listenCancel func()
	eventsDone   chan struct{}

	catalog           pluginServicesCatalog
	catalogLoaded     bool
	catalogLoading    bool
	catalogDirty      bool
	contextLoading    bool
	contextDirty      bool
	catalogGeneration int
	supervisorError   string

	contextValues map[string]any

	states map[pluginServicesViewKey]*pluginServicesViewState

	// seenExpanded mirrors OpenIDEA's tree store: an item asking to start
	// expanded does so once, and a later user collapse is not undone by a
	// render refresh.
	seenExpanded map[string]bool

	streamMu sync.RWMutex
	streams  map[uint32]*pluginServicesLogState
}

type pluginServicesViewState struct {
	runtime *pluginServicesRuntime
	key     pluginServicesViewKey
	decl    pluginServicesViewDecl
	icon    string

	ctx    context.Context
	cancel context.CancelFunc

	provider *serviceProvider
	dispose  func()

	generation int
	loading    bool
	err        string
	commandErr string
	toolbar    []openidea.PluginViewButton
	roots      []serviceTreeNode
	rawKeys    map[string]string

	renderCancel context.CancelFunc
	commandGen   int

	events chan pluginServicesEvent
	done   chan struct{}

	detail        *pluginServicesDetailState
	detailCancel  context.CancelFunc
	detailGen     int
	eventQueueErr string
}

type pluginServicesEvent struct {
	event openidea.PluginViewEvent
	done  chan struct{}
}

type pluginServicesDetailState struct {
	generation int
	fullKey    string
	rawKey     string
	loading    bool
	err        string
	tabs       []openidea.PluginServiceTab
	picked     string
	log        *pluginServicesLogState
}

type pluginServicesLogLine struct {
	stderr bool
	text   string
}

type pluginServicesLogState struct {
	runtime *pluginServicesRuntime
	view    *pluginServicesViewState
	detail  *pluginServicesDetailState
	tab     openidea.PluginServiceTab
	log     openidea.PluginServiceLog

	request uint32
	channel uint32
	cancel  context.CancelFunc

	on        map[string]bool
	wrap      bool
	findOpen  bool
	findText  string
	findMatch int
	loading   bool
	stopped   bool
	err       string
	exitCode  *int

	lines     []pluginServicesLogLine
	units     int
	truncated bool
}

// pluginContentController is the native seam used by OpenIDEA's content
// editor actions. The pinned GoIDEA surface has no plugin content editor yet;
// a future native editor can register this contract without duplicating the
// action routing.
type pluginContentController interface {
	Find()
	RunAction(id string)
	Active() bool
}

type pluginContentControllerBinding struct {
	id         uint64
	key        string
	controller pluginContentController
}

var pluginContentControllers = struct {
	sync.RWMutex
	nextID uint64
	items  []pluginContentControllerBinding
}{}

// registerPluginContentController binds OpenIDEA's target string to one native
// content editor. The returned function removes only this exact binding.
func registerPluginContentController(key string, controller pluginContentController) func() {
	if key == "" || controller == nil {
		return func() {}
	}
	pluginContentControllers.Lock()
	pluginContentControllers.nextID++
	id := pluginContentControllers.nextID
	pluginContentControllers.items = append(pluginContentControllers.items,
		pluginContentControllerBinding{id: id, key: key, controller: controller})
	pluginContentControllers.Unlock()
	return func() {
		pluginContentControllers.Lock()
		for i := range pluginContentControllers.items {
			item := pluginContentControllers.items[i]
			if item.id == id {
				pluginContentControllers.items =
					append(pluginContentControllers.items[:i], pluginContentControllers.items[i+1:]...)
				break
			}
		}
		pluginContentControllers.Unlock()
	}
}

func pluginContentControllerFor(target string) pluginContentController {
	items := pluginContentControllerSnapshot()
	if target != "" {
		for _, item := range items {
			if item.key == target {
				return item.controller
			}
		}
		return nil
	}
	for _, item := range items {
		if item.controller.Active() {
			return item.controller
		}
	}
	return nil
}

func pluginContentControllerSnapshot() []pluginContentControllerBinding {
	pluginContentControllers.RLock()
	defer pluginContentControllers.RUnlock()
	return append([]pluginContentControllerBinding(nil), pluginContentControllers.items...)
}

type pluginActionArgument struct {
	target     string
	id         string
	checked    bool
	checkedSet bool
}

func decodePluginActionArgument(arg any) pluginActionArgument {
	switch value := arg.(type) {
	case string:
		return pluginActionArgument{target: value}
	case map[string]any:
		var out pluginActionArgument
		out.target, _ = value["target"].(string)
		out.id, _ = value["id"].(string)
		out.checked, out.checkedSet = value["checked"].(bool)
		return out
	case map[string]string:
		return pluginActionArgument{target: value["target"], id: value["id"]}
	default:
		return pluginActionArgument{}
	}
}

func registerPluginServicesActions() {
	hiddenActionIDs[pluginServicesLogFindActionID] = true
	hiddenActionIDs[pluginServicesLogClearActionID] = true
	hiddenActionIDs[pluginServicesLogWrapActionID] = true
	hiddenActionIDs[pluginServicesLogToggleArgumentActionID] = true
	hiddenActionIDs[pluginServicesContentFindActionID] = true
	hiddenActionIDs[pluginServicesContentActionID] = true

	_ = registerAction(&Action{
		ID:       pluginServicesLogFindActionID,
		Title:    "Find in Logs",
		Category: "Plugins",
		RunWithArg: func(m *Model, arg any) {
			m.runPluginServicesLogAction(arg, pluginServicesLogFindActionID)
		},
	})
	_ = registerAction(&Action{
		ID:       pluginServicesLogClearActionID,
		Title:    "Clear",
		Category: "Plugins",
		RunWithArg: func(m *Model, arg any) {
			m.runPluginServicesLogAction(arg, pluginServicesLogClearActionID)
		},
	})
	_ = registerAction(&Action{
		ID:       pluginServicesLogWrapActionID,
		Title:    "Soft-wrap",
		Category: "Plugins",
		Toggled: func(m *Model) bool {
			log := m.activePluginServicesLog()
			return log != nil && log.wrap
		},
		RunWithArg: func(m *Model, arg any) {
			m.runPluginServicesLogAction(arg, pluginServicesLogWrapActionID)
		},
	})
	_ = registerAction(&Action{
		ID:       pluginServicesLogToggleArgumentActionID,
		Title:    "Toggle Log Option",
		Category: "Plugins",
		RunWithArg: func(m *Model, arg any) {
			m.runPluginServicesLogAction(arg, pluginServicesLogToggleArgumentActionID)
		},
	})
	_ = registerAction(&Action{
		ID:       pluginServicesContentFindActionID,
		Title:    "Find in Editor",
		Category: "Plugins",
		RunWithArg: func(m *Model, arg any) {
			m.runPluginContentAction(arg, pluginServicesContentFindActionID)
		},
	})
	_ = registerAction(&Action{
		ID:       pluginServicesContentActionID,
		Title:    "Run Content Editor Action",
		Category: "Plugins",
		RunWithArg: func(m *Model, arg any) {
			m.runPluginContentAction(arg, pluginServicesContentActionID)
		},
	})
}

func (l *pluginServicesLogState) targetKey() string {
	if l == nil || l.view == nil || l.detail == nil {
		return ""
	}
	parts := []string{
		"plugin.log",
		l.view.key.pluginID,
		l.view.key.viewID,
		l.detail.rawKey,
		l.tab.ID,
	}
	for i := range parts {
		parts[i] = strconv.Quote(parts[i])
	}
	return strings.Join(parts, ":")
}

func (r *pluginServicesRuntime) logForTarget(target string) *pluginServicesLogState {
	if r == nil || target == "" {
		return nil
	}
	for _, view := range r.states {
		if view == nil || view.detail == nil || view.detail.log == nil {
			continue
		}
		log := view.detail.log
		if !log.stopped && log.targetKey() == target {
			return log
		}
	}
	return nil
}

func (m *Model) activePluginServicesLog() *pluginServicesLogState {
	if m == nil || !m.toolVisible("services") ||
		m.services.view != serviceHostServices || m.services.selected == "" {
		return nil
	}
	runtime := m.pluginServicesRuntime()
	if runtime == nil {
		return nil
	}
	for _, view := range runtime.states {
		if view == nil || view.detail == nil || view.detail.log == nil ||
			view.detail.fullKey != m.services.selected {
			continue
		}
		if !view.detail.log.stopped {
			return view.detail.log
		}
	}
	return nil
}

func (m *Model) runPluginServicesLogAction(arg any, actionID string) {
	if m == nil {
		return
	}
	decoded := decodePluginActionArgument(arg)
	var log *pluginServicesLogState
	if decoded.target != "" {
		log = m.pluginServicesRuntime().logForTarget(decoded.target)
	} else {
		log = m.activePluginServicesLog()
	}
	if log == nil {
		if decoded.target != "" {
			m.status = "The plugin Services log is no longer available."
		} else {
			m.status = "No plugin Services log is active."
		}
		m.invalidate()
		return
	}

	switch actionID {
	case pluginServicesLogFindActionID:
		log.findOpen = true
	case pluginServicesLogClearActionID:
		log.clear()
	case pluginServicesLogWrapActionID:
		log.wrap = !log.wrap
	case pluginServicesLogToggleArgumentActionID:
		if decoded.id == "" || !decoded.checkedSet {
			m.status = "A log option id and checked state are required."
			m.invalidate()
			return
		}
		if !log.toggleArgument(decoded.id, decoded.checked) {
			m.status = "The log option " + strconv.Quote(decoded.id) + " is not available."
			m.invalidate()
			return
		}
	}
	m.invalidate()
}

func (m *Model) runPluginContentAction(arg any, actionID string) {
	if m == nil {
		return
	}
	decoded := decodePluginActionArgument(arg)
	controller := pluginContentControllerFor(decoded.target)
	if decoded.target == "" && m.pluginContentActive() {
		if session := m.pluginContentTabFor(m.tab()); session != nil {
			controller = session
		}
	}
	if controller == nil {
		if decoded.target != "" {
			m.status = "The plugin content editor is no longer available."
		} else {
			m.status = "No plugin content editor is active."
		}
		m.invalidate()
		return
	}
	switch actionID {
	case pluginServicesContentFindActionID:
		if m.pluginContentActive() {
			if session := m.pluginContentTabFor(m.tab()); session != nil {
				session.Find()
				break
			}
		}
		controller.Find()
	case pluginServicesContentActionID:
		if decoded.id == "" {
			m.status = "A plugin content editor action id is required."
			m.invalidate()
			return
		}
		controller.RunAction(decoded.id)
	default:
		return
	}
	m.invalidate()
}

const (
	pluginServiceLogMaxLines = 5000
	pluginServiceLogMaxUnits = 1 << 20
	pluginServiceLogMaxLine  = 16 << 10
)

func init() {
	registerServiceProvider(&serviceProvider{
		id:    pluginServicesSupervisorID,
		order: pluginServicesSupervisorOrder,
		watch: (*Model).watchOpenIDEAPluginServices,
		banner: func(m *Model, c *ui.Context, p *ui.Theme) {
			r := m.pluginServicesRuntime()
			if r == nil {
				return
			}
			r.drawSupervisorBanner(c, p)
		},
	})
	registerPluginServicesActions()
}

func newPluginServicesRuntime(m *Model, page *pluginPage, api pluginServicesAPI) *pluginServicesRuntime {
	parent := context.Background()
	if m != nil && m.ctx != nil {
		parent = m.ctx
	}
	ctx, cancel := context.WithCancel(parent)
	return &pluginServicesRuntime{
		m: m, page: page, api: api,
		ctx: ctx, cancel: cancel,
		eventsDone:    make(chan struct{}),
		contextValues: map[string]any{},
		states:        map[pluginServicesViewKey]*pluginServicesViewState{},
		seenExpanded:  map[string]bool{},
		streams:       map[uint32]*pluginServicesLogState{},
	}
}

func (m *Model) pluginServicesRuntime() *pluginServicesRuntime {
	if m == nil || m.plugins == nil {
		return nil
	}
	return m.plugins.openideaServices
}

func (m *Model) watchOpenIDEAPluginServices() func() {
	if m == nil || m.mgr == nil {
		return nil
	}
	page := m.pluginsPage()
	if page.openideaServices != nil {
		page.openideaServices.stop()
	}
	runtime := newPluginServicesRuntime(m, page, m.mgr)
	page.openideaServices = runtime
	runtime.start()
	return func() {
		runtime.stop()
		if page.openideaServices == runtime {
			page.openideaServices = nil
		}
	}
}

func (r *pluginServicesRuntime) start() {
	if r == nil || r.api == nil {
		return
	}
	ch, stop := r.api.Listen(256)
	r.listenCancel = stop
	go r.watchEvents(ch)
	r.refreshCatalog()
}

func (r *pluginServicesRuntime) stop() {
	if r == nil {
		return
	}
	for _, state := range r.states {
		state.close()
	}
	r.states = map[pluginServicesViewKey]*pluginServicesViewState{}
	r.cancel()
	if r.listenCancel != nil {
		r.listenCancel()
		r.listenCancel = nil
	}
	r.streamMu.Lock()
	r.streams = map[uint32]*pluginServicesLogState{}
	r.streamMu.Unlock()
	r.invalidate()
}

func (r *pluginServicesRuntime) invalidate() {
	if r != nil && r.m != nil {
		r.m.invalidate()
	}
}

func (r *pluginServicesRuntime) rootValues() map[string]any {
	root := ""
	if r != nil && r.m != nil {
		root = r.m.Root()
	}
	activeToolWindow := "services"
	servicesVisible := true
	if r != nil && r.m != nil {
		activeToolWindow = r.m.activeToolWindow()
		servicesVisible = r.m.toolVisible("services")
	}
	servicesFocus := activeToolWindow == "services"
	return map[string]any{
		"projectOpen":                 root != "",
		"projectLocal":                root != "",
		"projectOpenAsk":              false,
		"workspaceOpen":               false,
		"projectAttached":             false,
		"workspaceContainer":          false,
		"lightEdit":                   root == "",
		"activeToolWindow":            activeToolWindow,
		"toolWindowFocus":             servicesFocus,
		"servicesViewFocus":           servicesFocus,
		"toolWindow.services.visible": servicesVisible,
	}
}

// refreshCatalog coalesces commandsChanged bursts. A changed list replaces
// providers only where the view declaration or its declaration index changed,
// matching OpenIDEA's order-sensitive UI projection.
func (r *pluginServicesRuntime) refreshCatalog() {
	if r == nil || r.api == nil {
		return
	}
	if r.catalogLoading {
		r.catalogDirty = true
		return
	}
	r.catalogLoading = true
	r.catalogDirty = false
	r.catalogGeneration++
	generation := r.catalogGeneration
	ctx := r.ctx
	api := r.api
	window := Window
	go func() {
		snapshot, err := api.PluginsList(ctx, window)
		var keys []openidea.PluginContextKey
		var keyErr error
		if err == nil {
			keys, keyErr = api.PluginContextKeys(ctx, window)
		}
		r.m.post(func() {
			if r.ctx.Err() != nil || generation != r.catalogGeneration {
				return
			}
			r.catalogLoading = false
			if err != nil {
				r.supervisorError = "Could not load plugin services: " + err.Error()
				r.invalidate()
				if r.catalogDirty {
					r.catalogDirty = false
					r.refreshCatalog()
				}
				return
			}
			r.supervisorError = ""
			if keyErr == nil {
				r.applyContextKeys(keys)
			}
			if applyErr := r.applyCatalog(snapshot); applyErr != nil {
				r.supervisorError = "Could not project plugin services: " + applyErr.Error()
			}
			r.invalidate()
			if r.catalogDirty {
				r.catalogDirty = false
				r.refreshCatalog()
			}
		})
	}()
}

func (r *pluginServicesRuntime) refreshContext() {
	if r == nil || r.api == nil {
		return
	}
	if r.contextLoading {
		r.contextDirty = true
		return
	}
	r.contextLoading = true
	r.contextDirty = false
	ctx := r.ctx
	api := r.api
	window := Window
	go func() {
		keys, err := api.PluginContextKeys(ctx, window)
		r.m.post(func() {
			if r.ctx.Err() != nil {
				return
			}
			r.contextLoading = false
			if err == nil {
				r.applyContextKeys(keys)
				r.syncProviders()
			}
			r.invalidate()
			if r.contextDirty {
				r.contextDirty = false
				r.refreshContext()
			}
		})
	}()
}

func (r *pluginServicesRuntime) applyContextKeys(keys []openidea.PluginContextKey) {
	contextValues := r.rootValues()
	for _, key := range keys {
		value, ok := decodePluginContextValue(key.Value)
		if !ok {
			continue
		}
		contextValues["plugin."+key.PluginID+"."+key.Key] = value
	}
	r.contextValues = contextValues
}

func decodePluginContextValue(text string) (any, bool) {
	var value any
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	if err := dec.Decode(&value); err != nil {
		return nil, false
	}
	switch value.(type) {
	case nil, bool, string, json.Number:
		return value, true
	default:
		return value, true
	}
}

func (r *pluginServicesRuntime) applyCatalog(snapshot openidea.PluginsSnapshot) error {
	catalog := pluginServicesCatalog{
		pluginNames: map[string]string{},
		commands:    map[string]pluginServicesCommandDecl{},
	}
	for _, plugin := range snapshot.Plugins {
		name := plugin.Name
		if name == "" {
			name = plugin.ID
		}
		catalog.pluginNames[plugin.ID] = name
	}

	viewIndexes := map[string]int{}
	for _, contribution := range snapshot.UI {
		if contribution.PluginID == "" {
			continue
		}
		switch contribution.Point {
		case "views":
			var decl pluginServicesViewDecl
			if err := decodePluginJSON(contribution.Decl, &decl); err != nil {
				return fmt.Errorf("view %q: %w", contribution.Key, err)
			}
			if decl.ID == "" {
				decl.ID = contribution.Key
			}
			if decl.Location != "services" {
				continue
			}
			scope := contribution.PluginID + "\x00views"
			index := viewIndexes[scope]
			viewIndexes[scope]++
			raw := strings.TrimSpace(string(contribution.Decl))
			catalog.views = append(catalog.views, pluginServicesViewCatalog{
				key:       pluginServicesViewKey{pluginID: contribution.PluginID, viewID: decl.ID},
				decl:      decl,
				rawDecl:   raw,
				index:     index,
				signature: contribution.PluginID + "\x00" + decl.ID + "\x00" + raw + "\x00" + strconv.Itoa(index),
			})
		case "commands":
			var decl pluginServicesCommandDecl
			if err := decodePluginJSON(contribution.Decl, &decl); err != nil {
				return fmt.Errorf("command %q: %w", contribution.Key, err)
			}
			if decl.ID == "" {
				decl.ID = contribution.Key
			}
			catalog.commands[commandKey(contribution.PluginID, decl.ID)] = decl
		case "menus":
			var decl pluginServicesMenuDecl
			if err := decodePluginJSON(contribution.Decl, &decl); err != nil {
				return fmt.Errorf("menu %q: %w", contribution.Key, err)
			}
			catalog.menus = append(catalog.menus, pluginServicesMenuCatalog{
				pluginID: contribution.PluginID,
				decl:     decl,
			})
		}
	}
	r.catalog = catalog
	r.catalogLoaded = true
	r.syncProviders()
	return nil
}

func commandKey(pluginID, commandID string) string {
	return pluginID + "\x00" + commandID
}

func (r *pluginServicesRuntime) viewVisible(decl pluginServicesViewDecl) bool {
	return decl.When == nil || *decl.When == "" ||
		pluginServicesWhen(*decl.When, r.contextValues)
}

func (r *pluginServicesRuntime) syncProviders() {
	desired := make(map[pluginServicesViewKey]pluginServicesViewCatalog)
	for _, view := range r.catalog.views {
		if !r.viewVisible(view.decl) {
			continue
		}
		desired[view.key] = view
	}

	for key, state := range r.states {
		next, keep := desired[key]
		if keep && stateSignature(next) == state.declSignature() {
			continue
		}
		state.close()
		delete(r.states, key)
	}

	for _, view := range r.catalog.views {
		key := view.key
		if _, wanted := desired[key]; !wanted {
			continue
		}
		if state := r.states[key]; state != nil {
			continue
		}
		state := r.newViewState(view)
		r.states[key] = state
		state.register()
	}

	if r.m != nil && r.m.services.visible {
		r.m.ensureServiceSelection(r.m.serviceTree())
	}
	r.invalidate()
}

func stateSignature(view pluginServicesViewCatalog) string {
	return view.signature
}

func (s *pluginServicesViewState) declSignature() string {
	for _, view := range s.runtime.catalog.views {
		if view.key == s.key {
			return view.signature
		}
	}
	return ""
}

func (r *pluginServicesRuntime) newViewState(view pluginServicesViewCatalog) *pluginServicesViewState {
	ctx, cancel := context.WithCancel(r.ctx)
	icon := "nodes/plugin"
	if view.decl.Icon != nil && *view.decl.Icon != "" && !strings.HasSuffix(strings.ToLower(*view.decl.Icon), ".svg") {
		icon = *view.decl.Icon
	}
	state := &pluginServicesViewState{
		runtime: r,
		key:     view.key,
		decl:    view.decl,
		icon:    icon,
		ctx:     ctx,
		cancel:  cancel,
		rawKeys: map[string]string{},
		events:  make(chan pluginServicesEvent, pluginServicesEventQueue),
		done:    make(chan struct{}),
	}
	go state.watchEvents()
	state.loadRoots(false)
	return state
}

func (s *pluginServicesViewState) register() {
	if s == nil || s.provider != nil {
		return
	}
	s.provider = &serviceProvider{
		id:    s.key.providerID(),
		order: pluginServicesProviderOrder,
		roots: func(*Model) []serviceTreeNode {
			return s.roots
		},
		actions: func(_ *Model, node serviceTreeNode) []serviceItemAction {
			return s.actions(node)
		},
		menu: func(m *Model, menu *ui.Menu, node serviceTreeNode) {
			s.menu(m, menu, node)
		},
		detail: func(m *Model, c *ui.Context, p *ui.Theme, node serviceTreeNode) {
			s.drawDetail(m, c, p, node)
		},
		expand: func(_ *Model, node serviceTreeNode, open bool) {
			raw, ok := s.rawKeys[node.key]
			if !ok {
				return
			}
			kind := "collapse"
			if open {
				kind = "expand"
			}
			s.enqueue(openidea.PluginViewEvent{Kind: kind, Key: raw})
		},
		activate: func(_ *Model, node serviceTreeNode) {
			s.sendKeyEvent("activate", node.key)
		},
		selectFn: func(_ *Model, node serviceTreeNode) {
			s.sendKeyEvent("select", node.key)
		},
		banner: func(_ *Model, c *ui.Context, p *ui.Theme) {
			s.drawBanner(c, p)
		},
	}
	s.dispose = registerServiceProvider(s.provider)
}

func (s *pluginServicesViewState) close() {
	if s == nil {
		return
	}
	s.cancel()
	if s.dispose != nil {
		s.dispose()
		s.dispose = nil
	}
	if s.renderCancel != nil {
		s.renderCancel()
		s.renderCancel = nil
	}
	if s.detailCancel != nil {
		s.detailCancel()
		s.detailCancel = nil
	}
	s.stopLog()
	s.runtime.unregisterStreamsFor(s)
	s.provider = nil
}

func (s *pluginServicesViewState) sendKeyEvent(kind, fullKey string) {
	raw, ok := s.rawKeys[fullKey]
	if !ok {
		return
	}
	s.enqueue(openidea.PluginViewEvent{Kind: kind, Key: raw})
}

func (s *pluginServicesViewState) loadRoots(refresh bool) {
	if s == nil || s.runtime == nil || s.runtime.api == nil {
		return
	}
	if s.loading && !refresh {
		return
	}
	if s.renderCancel != nil {
		s.renderCancel()
	}
	ctx, cancel := context.WithCancel(s.ctx)
	s.renderCancel = cancel
	s.generation++
	generation := s.generation
	s.loading = true
	s.err = ""
	s.commandErr = ""
	s.eventQueueErr = ""
	s.runtime.invalidate()
	go func() {
		defer cancel()
		view, err := s.runtime.api.PluginViewRender(ctx, Window, s.key.pluginID, s.key.viewID)
		s.runtime.m.post(func() {
			if s.ctx.Err() != nil || generation != s.generation {
				return
			}
			s.loading = false
			s.renderCancel = nil
			if err != nil {
				s.err = err.Error()
				s.runtime.invalidate()
				return
			}
			if applyErr := s.applyRendered(view); applyErr != nil {
				s.err = applyErr.Error()
			}
			s.runtime.invalidate()
		})
	}()
}

func (s *pluginServicesViewState) applyRendered(view openidea.PluginView) error {
	s.toolbar = append([]openidea.PluginViewButton(nil), view.Toolbar...)
	var roots []serviceTreeNode
	rawKeys := map[string]string{}
	switch view.Body.Kind {
	case "tree":
		var items []openidea.PluginViewTreeItem
		if err := decodePluginJSON(view.Body.Value, &items); err != nil {
			return fmt.Errorf("decode plugin tree: %w", err)
		}
		roots = s.treeNodeItems(items, rawKeys)
	case "list":
		var items []openidea.PluginViewListItem
		if err := decodePluginJSON(view.Body.Value, &items); err != nil {
			return fmt.Errorf("decode plugin list: %w", err)
		}
		roots = s.listNodeItems(items, rawKeys)
	case "message":
		// OpenIDEA's services projection has no roots for other body kinds.
		roots = nil
	default:
		roots = nil
	}
	s.roots = roots
	s.rawKeys = rawKeys
	if s.runtime.m != nil && s.runtime.m.services.visible {
		s.runtime.m.ensureServiceSelection(s.runtime.m.serviceTree())
	}
	return nil
}

func (s *pluginServicesViewState) treeNodeItems(
	items []openidea.PluginViewTreeItem,
	rawKeys map[string]string,
) []serviceTreeNode {
	type pendingNode struct {
		item     openidea.PluginViewTreeItem
		children []*pendingNode
	}
	byKey := map[string]*pendingNode{}
	var pendingRoots []*pendingNode
	for _, item := range items {
		node := &pendingNode{item: item}
		if _, exists := byKey[item.Key]; !exists {
			byKey[item.Key] = node
		}
		if item.Parent != nil {
			if parent := byKey[*item.Parent]; parent != nil {
				parent.children = append(parent.children, node)
				continue
			}
		}
		pendingRoots = append(pendingRoots, node)
	}
	var convert func([]*pendingNode) []serviceTreeNode
	convert = func(nodes []*pendingNode) []serviceTreeNode {
		out := make([]serviceTreeNode, 0, len(nodes))
		for _, node := range nodes {
			item := node.item
			fullKey := s.key.keyPrefix() + item.Key
			rawKeys[fullKey] = item.Key
			detail := ""
			if item.Detail != nil {
				detail = *item.Detail
			}
			icon := s.icon
			if item.Icon != nil && *item.Icon != "" {
				icon = *item.Icon
			}
			serviceNode := serviceTreeNode{
				key:        fullKey,
				label:      item.Label,
				detail:     detail,
				nodeKind:   "plugin-service",
				expandable: item.Expandable || len(node.children) > 0,
				icon:       icon,
				children:   convert(node.children),
			}
			s.initializeExpanded(item.Key, fullKey, item.Expanded)
			out = append(out, serviceNode)
		}
		return out
	}
	return convert(pendingRoots)
}

func (s *pluginServicesViewState) listNodeItems(
	items []openidea.PluginViewListItem,
	rawKeys map[string]string,
) []serviceTreeNode {
	out := make([]serviceTreeNode, 0, len(items))
	for _, item := range items {
		fullKey := s.key.keyPrefix() + item.Key
		rawKeys[fullKey] = item.Key
		detail := ""
		if item.Detail != nil {
			detail = *item.Detail
		}
		icon := s.icon
		if item.Icon != nil && *item.Icon != "" {
			icon = *item.Icon
		}
		out = append(out, serviceTreeNode{
			key:      fullKey,
			label:    item.Label,
			detail:   detail,
			nodeKind: "plugin-service",
			icon:     icon,
		})
	}
	return out
}

func (s *pluginServicesViewState) initializeExpanded(rawKey, fullKey string, expanded bool) {
	if s.runtime.seenExpanded[fullKey] {
		return
	}
	s.runtime.seenExpanded[fullKey] = true
	if s.runtime.m.services.collapsed == nil {
		s.runtime.m.services.collapsed = map[string]bool{}
	}
	s.runtime.m.services.collapsed[fullKey] = !expanded
}

func (s *pluginServicesViewState) actions(node serviceTreeNode) []serviceItemAction {
	raw, ok := s.rawKeys[node.key]
	if !ok {
		return nil
	}
	out := make([]serviceItemAction, 0, len(s.toolbar))
	for _, button := range s.toolbar {
		button := button
		icon := "nodes/plugin"
		if button.Icon != nil && *button.Icon != "" && !strings.HasSuffix(strings.ToLower(*button.Icon), ".svg") {
			icon = *button.Icon
		}
		out = append(out, serviceItemAction{
			id:       node.key + "\x00" + button.ID,
			title:    button.Label,
			icon:     icon,
			disabled: !button.Enabled,
			run: func() {
				s.enqueue(openidea.PluginViewEvent{Kind: "select", Key: raw})
				s.enqueue(openidea.PluginViewEvent{Kind: "button", ID: button.ID})
			},
		})
	}
	return out
}

func (s *pluginServicesViewState) enqueue(event openidea.PluginViewEvent) {
	if s == nil || s.ctx.Err() != nil {
		return
	}
	select {
	case s.events <- pluginServicesEvent{event: event}:
	case <-s.ctx.Done():
	default:
		s.eventQueueErr = "A plugin Services action queue is full."
		s.runtime.m.status = s.eventQueueErr
		s.runtime.invalidate()
	}
}

func (s *pluginServicesViewState) waitEvents() error {
	done := make(chan struct{})
	item := pluginServicesEvent{done: done}
	select {
	case s.events <- item:
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
	select {
	case <-done:
		return nil
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}

func (s *pluginServicesViewState) watchEvents() {
	defer close(s.done)
	for {
		select {
		case <-s.ctx.Done():
			return
		case event := <-s.events:
			if event.done != nil {
				close(event.done)
				continue
			}
			err := s.runtime.api.PluginViewEvent(s.ctx, Window, s.key.pluginID, s.key.viewID, event.event)
			if err != nil && s.ctx.Err() == nil {
				s.runtime.m.post(func() {
					s.eventQueueErr = "Plugin view event failed: " + err.Error()
					s.runtime.m.status = s.eventQueueErr
					s.runtime.invalidate()
				})
			}
		}
	}
}

func (s *pluginServicesViewState) menu(m *Model, menu *ui.Menu, node serviceTreeNode) {
	raw, ok := s.rawKeys[node.key]
	if !ok {
		return
	}
	ctx := clonePluginContext(s.runtime.contextValues)
	ctx["viewItem"] = raw
	kind := raw
	if at := strings.IndexByte(raw, ':'); at >= 0 {
		kind = raw[:at]
	}
	ctx["viewItemKind"] = kind

	type menuGroup struct {
		label   string
		entries []pluginServicesMenuItem
	}
	type menuPart struct {
		item  *pluginServicesMenuItem
		group *menuGroup
	}
	var parts []menuPart
	groups := map[string]*menuGroup{}
	for _, contribution := range s.runtime.catalog.menus {
		decl := contribution.decl
		if contribution.pluginID != s.key.pluginID ||
			decl.Menu != "viewItem.context" ||
			decl.View == nil || *decl.View != s.key.viewID ||
			(decl.When != nil && !pluginServicesWhen(*decl.When, ctx)) {
			continue
		}
		command, ok := s.runtime.catalog.commands[commandKey(contribution.pluginID, decl.Command)]
		if !ok {
			continue
		}
		label := command.Title
		if decl.CommandTitle != nil && *decl.CommandTitle != "" {
			label = *decl.CommandTitle
		}
		if label == "" {
			name := s.runtime.catalog.pluginNames[contribution.pluginID]
			if name == "" {
				name = contribution.pluginID
			}
			label = name + ": " + decl.Command
		}
		item := pluginServicesMenuItem{
			label:    label,
			command:  decl.Command,
			rawKey:   raw,
			disabled: command.When != nil && !pluginServicesWhen(*command.When, s.runtime.contextValues),
		}
		if decl.Submenu == nil || *decl.Submenu == "" {
			copy := item
			parts = append(parts, menuPart{item: &copy})
			continue
		}
		group := groups[*decl.Submenu]
		if group == nil {
			group = &menuGroup{label: *decl.Submenu}
			groups[*decl.Submenu] = group
			parts = append(parts, menuPart{group: group})
		}
		group.entries = append(group.entries, item)
	}

	for _, part := range parts {
		if part.group != nil && len(part.group.entries) > 1 {
			group := part.group
			menu.Submenu(group.label, func(submenu *ui.Menu) {
				for _, item := range group.entries {
					item := item
					if submenu.Item(item.label).Disabled(item.disabled).Chosen() {
						s.runMenuCommand(m, item)
					}
				}
			})
			continue
		}
		var item pluginServicesMenuItem
		if part.item != nil {
			item = *part.item
		} else if len(part.group.entries) == 1 {
			item = part.group.entries[0]
		}
		if item.command != "" && menu.Item(item.label).Disabled(item.disabled).Chosen() {
			s.runMenuCommand(m, item)
		}
	}
}

type pluginServicesMenuItem struct {
	label    string
	command  string
	rawKey   string
	disabled bool
}

func (s *pluginServicesViewState) runMenuCommand(m *Model, item pluginServicesMenuItem) {
	s.commandGen++
	generation := s.commandGen
	s.commandErr = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(s.ctx, 60*time.Second)
		defer cancel()
		err := s.runtime.api.PluginRunCommand(ctx, Window, s.key.pluginID, item.command, item.rawKey)
		m.post(func() {
			if s.ctx.Err() != nil || generation != s.commandGen {
				return
			}
			if err == nil {
				s.commandErr = ""
				m.invalidate()
				return
			}
			s.commandErr = "Plugin command " + item.command + " failed: " + err.Error()
			m.status = s.commandErr
			m.invalidate()
		})
	}()
}

func clonePluginContext(src map[string]any) map[string]any {
	out := make(map[string]any, len(src)+2)
	for key, value := range src {
		out[key] = value
	}
	return out
}

func (s *pluginServicesViewState) ensureDetail(m *Model, fullKey, rawKey string, force bool) {
	if s == nil {
		return
	}
	if s.detail != nil && s.detail.fullKey == fullKey && !force {
		return
	}
	if s.detailCancel != nil {
		s.detailCancel()
		s.detailCancel = nil
	}
	s.stopLog()
	s.detailGen++
	generation := s.detailGen
	s.detail = &pluginServicesDetailState{
		generation: generation,
		fullKey:    fullKey,
		rawKey:     rawKey,
		loading:    true,
	}
	detail := s.detail
	ctx, cancel := context.WithCancel(s.ctx)
	s.detailCancel = cancel
	m.invalidate()
	go func() {
		defer cancel()
		if err := s.waitEvents(); err != nil {
			return
		}
		tabs, err := s.runtime.api.PluginServiceDetail(ctx, Window, s.key.pluginID, s.key.viewID, rawKey)
		m.post(func() {
			if s.ctx.Err() != nil || s.detail != detail || detail.generation != generation {
				return
			}
			s.detailCancel = nil
			detail.loading = false
			if err != nil {
				detail.err = err.Error()
				m.invalidate()
				return
			}
			detail.tabs = append([]openidea.PluginServiceTab(nil), tabs...)
			detail.err = ""
			detail.picked = ""
			if len(tabs) > 0 {
				detail.picked = tabs[0].ID
			}
			m.invalidate()
		})
	}()
}

func (s *pluginServicesViewState) drawDetail(
	m *Model,
	c *ui.Context,
	p *ui.Theme,
	node serviceTreeNode,
) {
	raw, ok := s.rawKeys[node.key]
	if !ok {
		ui.Text(c, "This plugin service no longer exists.").FontSize(12).TextColor(p.TextMuted)
		return
	}
	s.ensureDetail(m, node.key, raw, false)
	detail := s.detail
	if detail == nil {
		return
	}
	ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
		s.drawDetailHeader(m, c, p, detail)
		if detail.loading {
			ui.Row(c).Grow(1).Padding(20, 16).Children(func() {
				ui.Text(c, "Loading plugin service details…").FontSize(12).TextColor(p.TextMuted)
			})
			return
		}
		if detail.err != "" {
			ui.Column(c).Grow(1).Padding(20, 16).Gap(8).Children(func() {
				ui.Text(c, detail.err).FontSize(12).TextColor(p.Danger)
				if ui.Button(c, "Retry").Clicked() {
					s.detail = nil
					s.ensureDetail(m, node.key, raw, false)
				}
			})
			return
		}
		if len(detail.tabs) == 0 {
			ui.Row(c).Grow(1).Padding(20, 16).Children(func() {
				ui.Text(c, "Nothing to show").FontSize(12).TextColor(p.TextMuted)
			})
			return
		}
		tab := s.activeTab(detail)
		if tab == nil {
			ui.Text(c, "Nothing to show").FontSize(12).TextColor(p.TextMuted)
			return
		}
		s.drawServicePane(m, c, p, detail, *tab)
	})
}

func (s *pluginServicesViewState) drawDetailHeader(
	m *Model,
	c *ui.Context,
	p *ui.Theme,
	detail *pluginServicesDetailState,
) {
	if len(detail.tabs) == 0 {
		return
	}
	ui.Row(c).Height(32).Shrink(0).Padding(0, 6).Gap(4).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
		for _, tab := range detail.tabs {
			tab := tab
			label := tab.Label
			if label == "" {
				label = tab.ID
			}
			if ui.Button(c, label).Clicked() {
				if detail.picked != tab.ID {
					detail.picked = tab.ID
					s.stopLog()
					m.invalidate()
				}
			}
		}
	})
}

func (s *pluginServicesViewState) activeTab(detail *pluginServicesDetailState) *openidea.PluginServiceTab {
	if detail == nil || len(detail.tabs) == 0 {
		return nil
	}
	for i := range detail.tabs {
		if detail.tabs[i].ID == detail.picked {
			return &detail.tabs[i]
		}
	}
	detail.picked = detail.tabs[0].ID
	return &detail.tabs[0]
}

func (s *pluginServicesViewState) drawServicePane(
	m *Model,
	c *ui.Context,
	p *ui.Theme,
	detail *pluginServicesDetailState,
	tab openidea.PluginServiceTab,
) {
	switch tab.Body.Kind {
	case "message":
		var text string
		if err := decodePluginJSON(tab.Body.Value, &text); err != nil {
			ui.Text(c, err.Error()).FontSize(12).TextColor(p.Danger)
			return
		}
		ui.Row(c).Grow(1).Padding(20, 16).Children(func() {
			ui.Text(c, text).FontSize(12).TextColor(p.TextMuted)
		})
	case "text":
		var text string
		if err := decodePluginJSON(tab.Body.Value, &text); err != nil {
			ui.Text(c, err.Error()).FontSize(12).TextColor(p.Danger)
			return
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(10, 12).Children(func() {
				ui.Text(c, text).Font("monospace").FontSize(12).TextColor(p.Text)
			})
		})
	case "log":
		s.drawLogPane(m, c, p, detail, tab)
	default:
		ui.Text(c, "Unsupported plugin service pane: "+tab.Body.Kind).
			FontSize(12).TextColor(p.Danger)
	}
}

func (s *pluginServicesViewState) drawLogPane(
	m *Model,
	c *ui.Context,
	p *ui.Theme,
	detail *pluginServicesDetailState,
	tab openidea.PluginServiceTab,
) {
	var log openidea.PluginServiceLog
	if err := decodePluginJSON(tab.Body.Value, &log); err != nil {
		ui.Text(c, err.Error()).FontSize(12).TextColor(p.Danger)
		return
	}
	if detail.log == nil || detail.log.tab.ID != tab.ID || detail.log.detail != detail {
		s.startLog(m, detail, tab, log, nil)
	}
	logState := detail.log
	ui.Row(c).Height(34).Shrink(0).Padding(0, 8).Gap(10).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
		for _, toggle := range log.Toggles {
			toggle := toggle
			on := logState != nil && logState.on[toggle.ID]
			if ui.Checkbox(c, &on, toggle.Label).Changed() && logState != nil {
				logState.toggleArgument(toggle.ID, on)
			}
		}
		wrap := logState != nil && logState.wrap
		if ui.Checkbox(c, &wrap, "Soft-wrap").Changed() && logState != nil {
			logState.wrap = wrap
			m.invalidate()
		}
		ui.Spacer(c)
		if ui.Button(c, "Find").Clicked() && logState != nil {
			logState.findOpen = true
			m.invalidate()
		}
		if ui.Button(c, "Clear").Clicked() && logState != nil {
			logState.clear()
		}
		status := "Starting…"
		if logState != nil {
			switch {
			case logState.err != "":
				status = "Failed"
			case logState.exitCode != nil:
				status = fmt.Sprintf("Exit %d", *logState.exitCode)
			case !logState.loading:
				status = "Stopped"
			case len(logState.lines) > 0:
				status = "Running"
			}
		}
		ui.Text(c, status).FontSize(11).TextColor(p.TextMuted)
	})
	if logState == nil {
		return
	}
	queryBefore := logState.findText
	expression := pluginLogFindExpression(logState.findText)
	matches := pluginLogFindCount(logState.lines, expression)
	if ui.FindBar(c, &logState.findOpen, &logState.findText, matches, &logState.findMatch).Changed() {
		if logState.findText != queryBefore {
			logState.findMatch = 0
		}
		m.invalidate()
	}
	if !logState.findOpen && logState.findText != "" {
		logState.findText = ""
		logState.findMatch = 0
	}
	if logState.err != "" {
		ui.Row(c).Padding(8, 10).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, logState.err).FontSize(12).TextColor(p.Danger).Grow(1).
				SingleLine().Ellipsis("…")
			if ui.Button(c, "Retry").Clicked() {
				s.restartLog(m, logState)
			}
		})
	}
	ui.Scroll(c).Grow(1).Children(func() {
		render := func() {
			ui.Column(c).Padding(10, 12).Children(func() {
				if logState.truncated {
					ui.Text(c, "(Output truncated: earlier lines were omitted)").
						FontSize(11).TextColor(p.TextMuted)
				}
				if len(logState.lines) == 0 {
					ui.Text(c, "No output yet.").FontSize(12).TextColor(p.TextMuted)
					return
				}
				match := 0
				for _, line := range logState.lines {
					spans := pluginLogFindSpans(line.text, expression, match, logState.findMatch, p)
					match += pluginLogMatchCount(line.text, expression)
					element := ui.RichText(c, spans...).Font("monospace").FontSize(12)
					if !logState.wrap {
						element.NoWrap()
					}
					if line.stderr {
						element.TextColor(p.Warning)
					} else {
						element.TextColor(p.Text)
					}
				}
			})
		}
		if logState.wrap {
			render()
		} else {
			ui.ScrollHorizontal(c).MinWidth(0).Children(render)
		}
	})
}

func pluginLogFindExpression(query string) *regexp.Regexp {
	if query == "" {
		return nil
	}
	expression, err := regexp.Compile("(?i)" + regexp.QuoteMeta(query))
	if err != nil {
		return nil
	}
	return expression
}

func pluginLogFindCount(lines []pluginServicesLogLine, expression *regexp.Regexp) int {
	if expression == nil {
		return 0
	}
	count := 0
	for _, line := range lines {
		count += pluginLogMatchCount(line.text, expression)
	}
	return count
}

func pluginLogMatchCount(text string, expression *regexp.Regexp) int {
	if expression == nil {
		return 0
	}
	return len(expression.FindAllStringIndex(text, -1))
}

func pluginLogFindSpans(
	text string,
	expression *regexp.Regexp,
	firstMatch, currentMatch int,
	p *ui.Theme,
) []ui.Span {
	if expression == nil {
		return []ui.Span{{Text: text}}
	}
	ranges := expression.FindAllStringIndex(text, -1)
	if len(ranges) == 0 {
		return []ui.Span{{Text: text}}
	}
	spans := make([]ui.Span, 0, len(ranges)*2+1)
	start := 0
	for i, match := range ranges {
		if match[0] > start {
			spans = append(spans, ui.Span{Text: text[start:match[0]]})
		}
		background := p.Selection
		if firstMatch+i == currentMatch {
			background = p.Accent
		}
		spans = append(spans, ui.Span{
			Text: text[match[0]:match[1]], Background: background, Radius: 3,
		})
		start = match[1]
	}
	if start < len(text) {
		spans = append(spans, ui.Span{Text: text[start:]})
	}
	return spans
}

func (l *pluginServicesLogState) clear() {
	if l == nil {
		return
	}
	l.lines = nil
	l.units = 0
	l.truncated = false
	l.findMatch = 0
	if l.runtime != nil && l.runtime.m != nil {
		l.runtime.m.invalidate()
	}
}

func (l *pluginServicesLogState) toggleArgument(id string, checked bool) bool {
	if l == nil || l.view == nil || l.runtime == nil || l.runtime.m == nil {
		return false
	}
	if _, ok := l.on[id]; !ok {
		return false
	}
	if l.on[id] == checked {
		return true
	}
	l.on[id] = checked
	l.view.restartLog(l.runtime.m, l)
	return true
}

func (s *pluginServicesViewState) startLog(
	m *Model,
	detail *pluginServicesDetailState,
	tab openidea.PluginServiceTab,
	log openidea.PluginServiceLog,
	initialOn map[string]bool,
) {
	s.stopLog()
	ctx, cancel := context.WithCancel(s.ctx)
	onState := make(map[string]bool, len(log.Toggles))
	for _, toggle := range log.Toggles {
		onState[toggle.ID] = initialOn[toggle.ID]
	}
	state := &pluginServicesLogState{
		runtime: s.runtime,
		view:    s,
		detail:  detail,
		tab:     tab,
		log:     log,
		request: m.nextRequest(),
		channel: m.nextChannel(),
		cancel:  cancel,
		on:      onState,
		wrap:    true,
		loading: true,
	}
	detail.log = state
	s.runtime.registerStream(state)
	on := state.enabledToggles()
	go func() {
		code, err := s.runtime.api.PluginServiceLog(
			ctx, Window, s.key.pluginID, s.key.viewID, detail.rawKey, tab.ID,
			state.request, state.channel, on,
		)
		m.post(func() {
			if detail.log != state || state.stopped {
				return
			}
			request := state.request
			state.request = 0
			state.channel = 0
			state.cancel = nil
			state.loading = false
			if err != nil {
				if ctx.Err() == nil {
					state.err = err.Error()
				}
			} else {
				state.exitCode = code
			}
			s.runtime.unregisterStream(request)
			m.invalidate()
		})
	}()
	m.invalidate()
}

func (l *pluginServicesLogState) enabledToggles() []string {
	if l == nil {
		return nil
	}
	out := make([]string, 0, len(l.log.Toggles))
	for _, toggle := range l.log.Toggles {
		if l.on[toggle.ID] {
			out = append(out, toggle.ID)
		}
	}
	return out
}

func (s *pluginServicesViewState) restartLog(m *Model, log *pluginServicesLogState) {
	if log == nil {
		return
	}
	detail, tab := log.detail, log.tab
	decl := log.log
	wrap, findOpen, findText, findMatch := log.wrap, log.findOpen, log.findText, log.findMatch
	initialOn := make(map[string]bool, len(log.on))
	for id, on := range log.on {
		initialOn[id] = on
	}
	s.stopLog()
	s.startLog(m, detail, tab, decl, initialOn)
	if next := detail.log; next != nil {
		next.wrap = wrap
		next.findOpen = findOpen
		next.findText = findText
		next.findMatch = findMatch
	}
}

func (s *pluginServicesViewState) stopLog() {
	if s == nil || s.detail == nil || s.detail.log == nil {
		return
	}
	log := s.detail.log
	s.detail.log = nil
	log.stop()
}

func (l *pluginServicesLogState) stop() {
	if l == nil || l.stopped {
		return
	}
	l.stopped = true
	request := l.request
	l.request = 0
	l.channel = 0
	if l.cancel != nil {
		l.cancel()
		l.cancel = nil
	}
	if l.runtime != nil {
		l.runtime.unregisterStream(request)
		if request != 0 {
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				_ = l.runtime.api.PluginCancel(ctx, Window, request)
			}()
		}
	}
}

func (r *pluginServicesRuntime) registerStream(log *pluginServicesLogState) {
	if r == nil || log == nil || log.channel == 0 {
		return
	}
	r.streamMu.Lock()
	r.streams[log.channel] = log
	r.streamMu.Unlock()
}

func (r *pluginServicesRuntime) unregisterStream(request uint32) {
	if r == nil || request == 0 {
		return
	}
	r.streamMu.Lock()
	for channel, log := range r.streams {
		if log.request == request {
			delete(r.streams, channel)
		}
	}
	r.streamMu.Unlock()
}

func (r *pluginServicesRuntime) unregisterStreamsFor(view *pluginServicesViewState) {
	if r == nil || view == nil {
		return
	}
	r.streamMu.Lock()
	for channel, log := range r.streams {
		if log.view == view {
			delete(r.streams, channel)
		}
	}
	r.streamMu.Unlock()
}

func (l *pluginServicesLogState) append(line openidea.PluginOutputLine) {
	text := line.Text
	if pluginUTF16Units(text) > pluginServiceLogMaxLine {
		text = truncateUTF16(text, pluginServiceLogMaxLine) + " …"
	}
	l.lines = append(l.lines, pluginServicesLogLine{stderr: line.Stderr, text: text})
	l.units += pluginUTF16Units(text)
	for len(l.lines) > pluginServiceLogMaxLines || l.units > pluginServiceLogMaxUnits {
		l.truncated = true
		l.units -= pluginUTF16Units(l.lines[0].text)
		l.lines = l.lines[1:]
	}
}

func pluginUTF16Units(text string) int {
	return len(utf16.Encode([]rune(text)))
}

func truncateUTF16(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	units := 0
	for at, r := range text {
		size := 1
		if r > 0xffff {
			size = 2
		}
		if units+size > limit {
			return text[:at]
		}
		units += size
	}
	return text
}

func (r *pluginServicesRuntime) watchEvents(ch <-chan openidea.Notification) {
	defer close(r.eventsDone)
	for {
		select {
		case <-r.ctx.Done():
			return
		case notification, ok := <-ch:
			if !ok {
				return
			}
			switch notification.Method {
			case "plugin":
				var message openidea.PluginEventMessage
				if decodePluginJSON(notification.Params, &message) != nil {
					continue
				}
				if message.Window != nil && *message.Window != Window {
					continue
				}
				var event struct {
					Type     string `json:"type"`
					PluginID string `json:"pluginId"`
					View     string `json:"view"`
				}
				if decodePluginJSON(message.Event, &event) != nil {
					continue
				}
				r.m.post(func() { r.handlePluginEvent(event.Type, event.PluginID, event.View) })
			case "channel":
				var message openidea.ChannelMessage
				if decodePluginJSON(notification.Params, &message) != nil {
					continue
				}
				r.streamMu.RLock()
				stream := r.streams[message.ID]
				r.streamMu.RUnlock()
				if stream == nil {
					continue
				}
				var line openidea.PluginOutputLine
				if decodePluginJSON(message.Data, &line) != nil {
					continue
				}
				r.m.post(func() {
					if stream.stopped {
						return
					}
					stream.append(line)
					r.invalidate()
				})
			}
		}
	}
}

func (r *pluginServicesRuntime) handlePluginEvent(kind, pluginID, view string) {
	if r == nil || r.ctx.Err() != nil {
		return
	}
	switch kind {
	case "commandsChanged":
		r.refreshCatalog()
	case "contextChanged":
		r.refreshContext()
	case "viewChanged":
		state := r.states[pluginServicesViewKey{pluginID: pluginID, viewID: view}]
		if state == nil {
			return
		}
		state.loadRoots(true)
		if r.m != nil {
			node := r.m.serviceNodeByKey(r.m.serviceTree(), r.m.services.selected)
			if node != nil && node.provider == state.key.providerID() {
				if raw, ok := state.rawKeys[node.key]; ok {
					state.ensureDetail(r.m, node.key, raw, true)
				}
			}
		}
		r.invalidate()
	}
}

func (s *pluginServicesViewState) drawBanner(c *ui.Context, p *ui.Theme) {
	if s == nil {
		return
	}
	switch {
	case s.loading && len(s.roots) == 0:
		serviceProviderBanner(c, p, "Loading "+s.decl.Title+"…", p.TextMuted)
	case s.err != "":
		ui.Row(c).MinHeight(30).Shrink(0).Padding(6, 10).Gap(8).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(p.Border).
			Background(p.Warning.Mix(p.Surface, 0.9)).Children(func() {
			ui.Text(c, s.decl.Title+": "+s.err).FontSize(12).TextColor(p.Danger).Grow(1).
				SingleLine().Ellipsis("…")
			if ui.Button(c, "Retry").Clicked() {
				s.err = ""
				s.loadRoots(true)
			}
		})
	case s.commandErr != "":
		s.drawErrorBanner(c, p, s.commandErr, "Dismiss", func() {
			s.commandErr = ""
			s.runtime.invalidate()
		})
	case s.eventQueueErr != "":
		s.drawErrorBanner(c, p, s.eventQueueErr, "Dismiss", func() {
			s.eventQueueErr = ""
			s.runtime.invalidate()
		})
	}
}

func (s *pluginServicesViewState) drawErrorBanner(
	c *ui.Context,
	p *ui.Theme,
	message, action string,
	run func(),
) {
	ui.Row(c).MinHeight(30).Shrink(0).Padding(6, 10).Gap(8).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(p.Border).
		Background(p.Warning.Mix(p.Surface, 0.9)).Children(func() {
		ui.Text(c, s.decl.Title+": "+message).FontSize(12).TextColor(p.Danger).Grow(1).
			SingleLine().Ellipsis("…")
		if ui.Button(c, action).Clicked() {
			run()
		}
	})
}

func (r *pluginServicesRuntime) drawSupervisorBanner(c *ui.Context, p *ui.Theme) {
	if r == nil {
		return
	}
	switch {
	case r.supervisorError != "":
		ui.Row(c).MinHeight(30).Shrink(0).Padding(6, 10).Gap(8).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(p.Border).
			Background(p.Warning.Mix(p.Surface, 0.9)).Children(func() {
			ui.Text(c, r.supervisorError).FontSize(12).TextColor(p.Danger).Grow(1).
				SingleLine().Ellipsis("…")
			if ui.Button(c, "Retry").Clicked() {
				r.supervisorError = ""
				r.refreshCatalog()
			}
		})
	case r.catalogLoading && !r.catalogLoaded:
		serviceProviderBanner(c, p, "Loading plugin services…", p.TextMuted)
	}
}

// pluginServicesWhen implements OpenIDEA's small `when` expression grammar:
// ||, &&, !, parentheses, truthy keys and == / != comparisons.
func pluginServicesWhen(expression string, context map[string]any) bool {
	if strings.TrimSpace(expression) == "" {
		return true
	}
	tokens, err := tokenizePluginWhen(expression)
	if err != nil {
		return false
	}
	parser := pluginWhenParser{tokens: tokens, context: context}
	value, err := parser.parseOr()
	if err != nil || parser.pos != len(tokens) {
		return false
	}
	return value
}

type pluginWhenToken struct {
	kind  byte
	value string
}

func tokenizePluginWhen(source string) ([]pluginWhenToken, error) {
	var tokens []pluginWhenToken
	for at := 0; at < len(source); {
		r, size := utf8.DecodeRuneInString(source[at:])
		if unicode.IsSpace(r) {
			at += size
			continue
		}
		if strings.HasPrefix(source[at:], "&&") ||
			strings.HasPrefix(source[at:], "||") ||
			strings.HasPrefix(source[at:], "==") ||
			strings.HasPrefix(source[at:], "!=") {
			tokens = append(tokens, pluginWhenToken{kind: 'o', value: source[at : at+2]})
			at += 2
			continue
		}
		switch source[at] {
		case '!', '(', ')':
			tokens = append(tokens, pluginWhenToken{kind: 'o', value: source[at : at+1]})
			at++
			continue
		case '\'', '"':
			quote := source[at]
			end := strings.IndexByte(source[at+1:], quote)
			if end < 0 {
				return nil, fmt.Errorf("unterminated string in when clause")
			}
			tokens = append(tokens, pluginWhenToken{kind: 's', value: source[at+1 : at+1+end]})
			at += end + 2
			continue
		}
		start := at
		for at < len(source) {
			r, size := utf8.DecodeRuneInString(source[at:])
			if unicode.IsSpace(r) || strings.ContainsRune("!&|=()'\"", r) {
				break
			}
			if !(unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("_.-:/", r)) {
				return nil, fmt.Errorf("unexpected %q in when clause", r)
			}
			at += size
		}
		if start == at {
			return nil, fmt.Errorf("unexpected %q in when clause", source[at])
		}
		tokens = append(tokens, pluginWhenToken{kind: 'i', value: source[start:at]})
	}
	return tokens, nil
}

type pluginWhenParser struct {
	tokens  []pluginWhenToken
	pos     int
	context map[string]any
}

func (p *pluginWhenParser) peek(value string) bool {
	return p.pos < len(p.tokens) && p.tokens[p.pos].value == value
}

func (p *pluginWhenParser) eat(value string) bool {
	if !p.peek(value) {
		return false
	}
	p.pos++
	return true
}

func (p *pluginWhenParser) parseOr() (bool, error) {
	left, err := p.parseAnd()
	if err != nil {
		return false, err
	}
	for p.eat("||") {
		right, err := p.parseAnd()
		if err != nil {
			return false, err
		}
		left = left || right
	}
	return left, nil
}

func (p *pluginWhenParser) parseAnd() (bool, error) {
	left, err := p.parseUnary()
	if err != nil {
		return false, err
	}
	for p.eat("&&") {
		right, err := p.parseUnary()
		if err != nil {
			return false, err
		}
		left = left && right
	}
	return left, nil
}

func (p *pluginWhenParser) parseUnary() (bool, error) {
	if p.eat("!") {
		value, err := p.parseUnary()
		return !value, err
	}
	if p.eat("(") {
		value, err := p.parseOr()
		if err != nil {
			return false, err
		}
		if !p.eat(")") {
			return false, fmt.Errorf("missing ')' in when clause")
		}
		return value, nil
	}
	if p.pos >= len(p.tokens) || p.tokens[p.pos].kind == 'o' {
		return false, fmt.Errorf("expected key in when clause")
	}
	key := p.tokens[p.pos].value
	p.pos++
	if p.peek("==") || p.peek("!=") {
		negate := p.tokens[p.pos].value == "!="
		p.pos++
		if p.pos >= len(p.tokens) || p.tokens[p.pos].kind == 'o' {
			return false, fmt.Errorf("expected value in when clause")
		}
		value := pluginWhenLiteral(p.tokens[p.pos])
		p.pos++
		equal := pluginContextEqual(p.context[key], value)
		return equal != negate, nil
	}
	return pluginContextTruthy(p.context[key]), nil
}

func pluginWhenLiteral(token pluginWhenToken) any {
	if token.kind == 's' {
		return token.value
	}
	switch token.value {
	case "true":
		return true
	case "false":
		return false
	}
	if _, err := strconv.ParseFloat(token.value, 64); err == nil {
		return json.Number(token.value)
	}
	return token.value
}

func pluginContextEqual(left, right any) bool {
	leftNumber, leftOK := pluginContextNumber(left)
	rightNumber, rightOK := pluginContextNumber(right)
	if leftOK && rightOK {
		return leftNumber == rightNumber
	}
	switch left := left.(type) {
	case nil:
		return right == nil
	case string:
		value, ok := right.(string)
		return ok && left == value
	case bool:
		value, ok := right.(bool)
		return ok && left == value
	default:
		return false
	}
}

func pluginContextNumber(value any) (float64, bool) {
	switch value := value.(type) {
	case json.Number:
		number, err := value.Float64()
		return number, err == nil
	case float64:
		return value, true
	case float32:
		return float64(value), true
	case int:
		return float64(value), true
	case int64:
		return float64(value), true
	default:
		return 0, false
	}
}

func pluginContextTruthy(value any) bool {
	switch value := value.(type) {
	case nil:
		return false
	case bool:
		return value
	case string:
		return value != ""
	case json.Number:
		number, err := value.Float64()
		return err != nil || number != 0
	default:
		return true
	}
}
