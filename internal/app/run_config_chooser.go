package app

// The Select Run/Debug Configuration popup, the port of upstream
// ConfigChooser behind run.chooseConfig (Ctrl+Alt+R) and
// run.chooseDebugConfig (Ctrl+Alt+D): run a configuration (Shift+Enter to
// debug it instead), debug one, or — from the title bar widget — only select
// one. The chosen configuration runs as saved now, through the same
// stop-and-rerun confirm the Run action uses.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/adapter"
	"mayoi/internal/openidea"
)

// runChooser is the configuration chooser popup while it is open.
type runChooser struct {
	open      bool
	mode      string // "run" | "debug" | "select"
	query     string
	lastQuery string
	sel       int
	search    ui.Handle
}

type runChooserSection struct {
	label   string
	configs []*openidea.RunConfig
}

type runChooserItemKind string

const (
	runChooserSave    runChooserItemKind = "save"
	runChooserCurrent runChooserItemKind = "current"
	runChooserConfig  runChooserItemKind = "config"
	runChooserEdit    runChooserItemKind = "edit"
)

type runChooserItem struct {
	kind runChooserItemKind
	cfg  *openidea.RunConfig
}

type runChooserConfigSession struct {
	running  bool
	debug    bool
	debugTab uint32
}

// OpenRunChooser opens the chooser: "run" runs the chosen configuration
// (Shift+Enter debugs it), "debug" debugs it, "select" only selects it for
// the run pane, as the widget's chooser does.
func (m *Model) OpenRunChooser(mode string) {
	if m.chooser != nil && m.chooser.open {
		// A second press closes the open chooser, as upstream toggles it.
		m.chooser = nil
		m.invalidate()
		return
	}
	m.chooser = &runChooser{open: true, mode: mode}
	m.syncRunChooserSelection()
	if !m.run.loaded && !m.run.loading {
		m.loadRunConfigs()
	}
	m.invalidate()
}

func (m *Model) syncRunChooserSelection() {
	if m.chooser == nil {
		return
	}
	selectedID := m.run.selectedID()
	if selectedID == "current-file" {
		m.chooser.sel = m.chooserItemIndex(runChooserCurrent, "")
	} else {
		m.chooser.sel = m.chooserItemIndex(runChooserConfig, selectedID)
	}
	if m.chooser.sel < 0 {
		m.chooser.sel = m.chooserItemIndex(runChooserCurrent, "")
	}
	if m.chooser.sel < 0 {
		m.chooser.sel = 0
	}
}

// closeRunChooser drops the popup.
func (m *Model) closeRunChooser() {
	m.chooser = nil
	m.invalidate()
}

func (m *Model) runPrefsState() *RunPrefs {
	if m.prefs.Run == nil {
		m.prefs.Run = &RunPrefs{}
	}
	return m.prefs.Run
}

func (m *Model) runProjectPrefsState() *RunProjectPrefs {
	prefs := m.runPrefsState()
	if prefs.Projects == nil {
		prefs.Projects = map[string]*RunProjectPrefs{}
	}
	key := m.Root()
	project := prefs.Projects[key]
	if project == nil {
		project = &RunProjectPrefs{
			Pinned: append([]string(nil), prefs.Pinned...),
			Recent: append([]string(nil), prefs.Recent...),
		}
		prefs.Projects[key] = project
	}
	if len(prefs.Pinned) > 0 || len(prefs.Recent) > 0 {
		prefs.Pinned = nil
		prefs.Recent = nil
	}
	return project
}

func (m *Model) runChooserMatches(cfg *openidea.RunConfig, query string) bool {
	if cfg == nil {
		return false
	}
	if query == "" {
		return true
	}
	_, _, ok := fuzzySubsequence(query, cfg.Name)
	return ok
}

func (m *Model) runChooserByID(id string) *openidea.RunConfig {
	for i := range m.run.configs {
		if m.run.configs[i].ID == id {
			return &m.run.configs[i]
		}
	}
	if cfg := m.runTemporaryByID(id); cfg != nil {
		return cfg
	}
	if m.run.inline != nil && m.run.inline.ID == id {
		return m.run.inline
	}
	return nil
}

func (m *Model) runChooserHasSaveRow(query string) bool {
	return query == "" && m.run.inline != nil && m.isRunTemporaryID(m.run.inline.ID)
}

func runChooserMatchesCurrentFile(query string) bool {
	if query == "" {
		return true
	}
	_, _, ok := fuzzySubsequence(query, "Current File")
	return ok
}

func runChooserExecutor(mode string, modifiers ui.Modifiers) string {
	if mode == "select" {
		return "select"
	}
	if modifiers&ui.Shift == 0 {
		return mode
	}
	if mode == "run" {
		return "debug"
	}
	return "run"
}

func (m *Model) runConfigTargetLabel(cfg *openidea.RunConfig) string {
	if cfg == nil || cfg.Target == nil {
		return ""
	}
	target := strings.TrimSpace(*cfg.Target)
	if target == "" {
		return ""
	}
	return runTargetChoiceDisplay(m.remote.runTargets.choices(target), target)
}

func (m *Model) chooserConfigSession(id string) runChooserConfigSession {
	if id == "" {
		return runChooserConfigSession{}
	}
	if (m.run.running || m.run.awaiting) && m.run.lastID == id {
		return runChooserConfigSession{running: true}
	}
	if m.debug.active != 0 && !m.debug.ended && m.debug.lastID == id &&
		!m.debug.retiredSessions[m.debug.active] {
		return runChooserConfigSession{running: true, debug: true, debugTab: m.debug.active}
	}
	for _, tab := range m.debug.sessionOrder {
		state := m.debug.sessions[tab]
		if state != nil && !state.ended && state.lastID == id &&
			!m.debug.retiredSessions[tab] {
			return runChooserConfigSession{running: true, debug: true, debugTab: tab}
		}
	}
	return runChooserConfigSession{}
}

func (m *Model) activateChooserConfigSession(
	id string,
	session runChooserConfigSession,
	rerun bool,
) {
	m.closeRunChooser()
	m.selectRunConfig(id)
	if !session.debug {
		if rerun {
			m.Rerun()
		} else {
			m.StopRun()
		}
		return
	}
	m.selectDebugSession(session.debugTab)
	if rerun {
		m.DebugRerun()
	} else {
		m.StopDebug()
	}
}

func (m *Model) handleRunChooserInput(ev ui.InputEvent) bool {
	ch := m.chooser
	if ch == nil || ev.Kind != ui.InputKeyDown || ch.query != "" || ch.mode == "select" {
		return false
	}
	n := 0
	switch ev.Key {
	case ui.Key1:
		n = 1
	case ui.Key2:
		n = 2
	case ui.Key3:
		n = 3
	case ui.Key4:
		n = 4
	case ui.Key5:
		n = 5
	case ui.Key6:
		n = 6
	case ui.Key7:
		n = 7
	case ui.Key8:
		n = 8
	case ui.Key9:
		n = 9
	default:
		return false
	}
	if ev.Mods&^(ui.Shift) != 0 {
		return false
	}
	m.chooseNth(n, runChooserExecutor(ch.mode, ev.Mods))
	return true
}

// chooserSections ports ConfigChooser.buildRows: a temporary's Save row is
// rendered separately, then Pinned, Recent, and the remaining configurations
// grouped by kind.
func (m *Model) chooserSections() []runChooserSection {
	if m.chooser == nil {
		return nil
	}
	query := strings.ToLower(strings.TrimSpace(m.chooser.query))
	prefs := m.runProjectPrefsState()
	seen := map[string]bool{}
	var sections []runChooserSection
	pinned := map[string]bool{}
	for _, id := range prefs.Pinned {
		pinned[id] = true
	}
	appendIDs := func(label string, ids []string, skipPinned bool) {
		var rows []*openidea.RunConfig
		for _, id := range ids {
			if skipPinned && pinned[id] || seen[id] {
				continue
			}
			cfg := m.runChooserByID(id)
			if !m.runChooserMatches(cfg, query) {
				continue
			}
			seen[id] = true
			rows = append(rows, cfg)
		}
		if len(rows) > 0 {
			sections = append(sections, runChooserSection{label: label, configs: rows})
		}
	}
	appendIDs("Pinned", prefs.Pinned, false)
	appendIDs("Recent", prefs.Recent, true)

	var kinds []string
	seenKinds := map[string]bool{}
	addKind := func(cfg *openidea.RunConfig) {
		if cfg != nil && !seenKinds[cfg.Kind] {
			seenKinds[cfg.Kind] = true
			kinds = append(kinds, cfg.Kind)
		}
	}
	for i := range m.run.configs {
		addKind(&m.run.configs[i])
	}
	for _, cfg := range m.run.temporaries {
		addKind(cfg)
	}
	addKind(m.run.inline)
	addKind(m.run.pendingInline)
	for _, kind := range kinds {
		var rows []*openidea.RunConfig
		for i := range m.run.configs {
			cfg := &m.run.configs[i]
			if cfg.Kind == kind && !seen[cfg.ID] && m.runChooserMatches(cfg, query) {
				seen[cfg.ID] = true
				rows = append(rows, cfg)
			}
		}
		for _, cfg := range m.run.temporaries {
			if cfg != nil && cfg.Kind == kind && !seen[cfg.ID] &&
				m.runChooserMatches(cfg, query) {
				seen[cfg.ID] = true
				rows = append(rows, cfg)
			}
		}
		if cfg := m.run.inline; cfg != nil && cfg.Kind == kind && !seen[cfg.ID] &&
			m.runChooserMatches(cfg, query) {
			seen[cfg.ID] = true
			rows = append(rows, cfg)
		}
		if cfg := m.run.pendingInline; cfg != nil && cfg.Kind == kind && !seen[cfg.ID] &&
			m.runChooserMatches(cfg, query) {
			seen[cfg.ID] = true
			rows = append(rows, cfg)
		}
		if len(rows) > 0 {
			sections = append(sections, runChooserSection{label: adapter.KindLabel(kind), configs: rows})
		}
	}
	return sections
}

// chooserItems is the keyboard order of ConfigChooser.buildRows. Section
// headings are not active, while Save, Current File, every configuration and
// Edit Configurations are.
func (m *Model) chooserItems() []runChooserItem {
	if m.chooser == nil {
		return nil
	}
	query := strings.ToLower(strings.TrimSpace(m.chooser.query))
	if m.chooser.lastQuery != query {
		m.chooser.lastQuery = query
		m.chooser.sel = 0
	}
	var items []runChooserItem
	if m.runChooserHasSaveRow(query) {
		items = append(items, runChooserItem{kind: runChooserSave, cfg: m.run.inline})
	}
	if runChooserMatchesCurrentFile(query) {
		items = append(items, runChooserItem{kind: runChooserCurrent})
	}
	for _, section := range m.chooserSections() {
		for _, cfg := range section.configs {
			items = append(items, runChooserItem{kind: runChooserConfig, cfg: cfg})
		}
	}
	items = append(items, runChooserItem{kind: runChooserEdit})
	return items
}

func (m *Model) chooserItemIndex(kind runChooserItemKind, id string) int {
	for i, item := range m.chooserItems() {
		if item.kind != kind {
			continue
		}
		if kind != runChooserConfig || item.cfg != nil && item.cfg.ID == id {
			return i
		}
	}
	return -1
}

func (m *Model) chooserSelectedItem() (runChooserItem, bool) {
	if m.chooser == nil {
		return runChooserItem{}, false
	}
	items := m.chooserItems()
	if m.chooser.sel < 0 || m.chooser.sel >= len(items) {
		return runChooserItem{}, false
	}
	return items[m.chooser.sel], true
}

// chooserRows flattens the chooser's sections in keyboard order.
func (m *Model) chooserRows() []*openidea.RunConfig {
	var out []*openidea.RunConfig
	for _, item := range m.chooserItems() {
		if item.kind == runChooserConfig && item.cfg != nil {
			out = append(out, item.cfg)
		}
	}
	return out
}

func (m *Model) togglePinnedRunConfig(id string) {
	if m.runChooserByID(id) == nil {
		return
	}
	prefs := m.runProjectPrefsState()
	at := -1
	for i, pinned := range prefs.Pinned {
		if pinned == id {
			at = i
			break
		}
	}
	if at >= 0 {
		prefs.Pinned = append(prefs.Pinned[:at], prefs.Pinned[at+1:]...)
	} else {
		prefs.Pinned = append(prefs.Pinned, id)
	}
	m.savePreferences()
	m.invalidate()
}

// recordRecentRunConfig keeps five configurations per project, newest first.
func (m *Model) recordRecentRunConfig(id string) {
	if id == "" {
		return
	}
	prefs := m.runProjectPrefsState()
	if len(prefs.Recent) > 0 && prefs.Recent[0] == id {
		return
	}
	next := []string{id}
	for _, recent := range prefs.Recent {
		if recent != id && len(next) < 5 {
			next = append(next, recent)
		}
	}
	prefs.Recent = next
	m.savePreferences()
}

// stepChooser moves the selection within the filtered rows.
func (m *Model) stepChooser(delta int) {
	c := m.chooser
	if c == nil {
		return
	}
	n := len(m.chooserItems())
	if n == 0 {
		return
	}
	c.sel = ((c.sel+delta)%n + n) % n
	m.invalidate()
}

func (m *Model) selectRunConfig(id string) {
	if id == "current-file" {
		m.run.inline = nil
		m.run.currentFile = true
		m.persistRunSelection()
		return
	}
	for i := range m.run.configs {
		if m.run.configs[i].ID == id {
			m.run.sel = i
			m.run.inline = nil
			m.run.currentFile = false
			m.persistRunSelection()
			return
		}
	}
	if cfg := m.runTemporaryByID(id); cfg != nil {
		m.run.inline = cfg
		m.run.currentFile = false
		m.persistRunSelection()
	}
}

// chooseActivate applies the mode to the chosen row: how is "run", "debug"
// or "select".
func (m *Model) chooseActivate(how string) {
	c := m.chooser
	if c == nil {
		return
	}
	item, ok := m.chooserSelectedItem()
	if !ok {
		return
	}
	switch item.kind {
	case runChooserSave:
		m.editTemporaryRunConfig()
		return
	case runChooserCurrent:
		m.chooseCurrentFileWith(how)
		return
	case runChooserEdit:
		m.closeRunChooser()
		m.OpenRunConfigs()
		return
	}
	cfg := item.cfg
	if cfg == nil {
		return
	}
	m.selectRunConfig(cfg.ID)
	switch how {
	case "select":
		m.closeRunChooser()
	case "debug":
		m.closeRunChooser()
		m.DebugRunConfig(cfg)
	default:
		m.closeRunChooser()
		m.askOrStart(cfg.Raw(), cfg.Name)
	}
}

func (m *Model) chooseNth(n int, how string) {
	if n < 1 || n > 9 {
		return
	}
	number := 0
	index := -1
	for i, item := range m.chooserItems() {
		if item.kind != runChooserConfig {
			continue
		}
		number++
		if number == n {
			index = i
			break
		}
	}
	if index < 0 {
		return
	}
	m.chooser.sel = index
	m.chooseActivate(how)
}

// editChosenRunConfig is F4 in the chooser.
func (m *Model) editChosenRunConfig() {
	item, ok := m.chooserSelectedItem()
	if !ok || item.kind != runChooserConfig || item.cfg == nil {
		return
	}
	id := item.cfg.ID
	for i := range m.run.configs {
		if m.run.configs[i].ID == id {
			m.run.sel = i
			break
		}
	}
	m.closeRunChooser()
	m.OpenRunConfigs()
}

// deleteChosenRunConfig is Delete in the chooser. Removing a detected
// configuration records a suppression override, as OpenIDEA's deleteConfig.
func (m *Model) deleteChosenRunConfig() {
	c := m.chooser
	item, ok := m.chooserSelectedItem()
	if c == nil || !ok || item.kind != runChooserConfig || item.cfg == nil {
		return
	}
	cfg := item.cfg
	id, detected := cfg.ID, cfg.Detected
	temporary := m.isRunTemporaryID(id)
	filtered := adapter.RemoveConfig(runConfigPointers(m.run.configs), id)
	m.run.configs = make([]openidea.RunConfig, 0, len(filtered))
	for _, cfg := range filtered {
		if cfg != nil {
			m.run.configs = append(m.run.configs, *cfg)
		}
	}
	if temporary {
		m.removeRunTemporary(id)
	}
	prefs := m.runProjectPrefsState()
	prefs.Pinned = removeRunConfigID(prefs.Pinned, id)
	prefs.Recent = removeRunConfigID(prefs.Recent, id)
	m.savePreferences()
	if m.run.sel >= len(m.run.configs) {
		m.run.sel = len(m.run.configs) - 1
	}
	if c.sel >= len(m.chooserItems()) {
		c.sel = max(0, len(m.chooserItems())-1)
	}
	m.invalidate()
	if temporary {
		return
	}
	user := adapter.UserConfigs(filtered)
	var removed []string
	if detected {
		removed = []string{id}
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		err := adapter.SaveUserConfigs(ctx, javaBackend{m}, user, removed)
		m.post(func() {
			if err != nil {
				m.status = "Deleting run configuration failed: " + err.Error()
				m.loadRunConfigs()
				m.invalidate()
				return
			}
			m.status = "Run configuration deleted"
			m.loadRunConfigs()
			m.invalidate()
		})
	}()
}

func removeRunConfigID(ids []string, id string) []string {
	out := ids[:0]
	for _, value := range ids {
		if value != id {
			out = append(out, value)
		}
	}
	return out
}

// chooseCurrentFile runs or debugs the active file's own configuration, the
// Current File row OpenIDEA's chooser shows in run and debug modes.
func (m *Model) chooseCurrentFile() {
	mode := "run"
	if m.chooser != nil {
		mode = m.chooser.mode
	}
	m.chooseCurrentFileWith(mode)
}

func (m *Model) chooseCurrentFileWith(how string) {
	m.closeRunChooser()
	m.selectRunConfig("current-file")
	if how == "select" {
		m.invalidate()
		return
	}
	if how == "debug" {
		m.DebugContext()
		return
	}
	m.RunContextConfiguration()
}

// editTemporaryRunConfig opens the session-only configuration in Edit
// Configurations, where OK or Apply persists it. This is the Save row in
// OpenIDEA's chooser.
func (m *Model) editTemporaryRunConfig() {
	if m.run.inline == nil {
		return
	}
	m.closeRunChooser()
	m.OpenRunConfigs()
}

// runChooserDialog draws the configuration chooser.
func (m *Model) runChooserDialog(c *ui.Context, p *ui.Theme) {
	ch := m.chooser
	if ch == nil {
		return
	}
	open := ch.open
	items := m.chooserItems()
	if ch.sel >= len(items) {
		ch.sel = len(items) - 1
	}
	if ch.sel < 0 {
		ch.sel = 0
	}
	nativeModal(c, &open, func() {
		ui.Column(c).Width(560).Gap(8).Children(func() {
			if ch.mode != "select" {
				caption := "Run"
				if ch.mode == "debug" {
					caption = "Debug"
				}
				ui.Text(c, caption).FontSize(11).Bold().TextColor(p.TextMuted).SingleLine()
			}
			in := ui.TextInput(c, &ch.query).AutoFocus().Placeholder("Search configurations").
				Bind(&ch.search).HandleInput(m.handleRunChooserInput)
			in.OnShortcut(0, ui.KeyDown, func() { m.stepChooser(1) })
			in.OnShortcut(0, ui.KeyUp, func() { m.stepChooser(-1) })
			in.OnShortcut(0, ui.KeyF4, m.editChosenRunConfig)
			in.OnShortcut(0, ui.KeyDelete, func() {
				if ch.query == "" {
					m.deleteChosenRunConfig()
				}
			})
			if in.Submitted() {
				switch {
				case ch.mode == "select":
					m.chooseActivate("select")
				case ch.mode == "debug":
					m.chooseActivate("debug")
				case in.SubmittedMods()&ui.Shift != 0:
					m.chooseActivate("debug")
				default:
					m.chooseActivate("run")
				}
			}
			ui.Scroll(c).Height(320).Children(func() {
				ui.Column(c).Gap(1).Children(func() {
					saveIndex := m.chooserItemIndex(runChooserSave, "")
					if saveIndex >= 0 {
						r := ui.Row(c.Key("runcfg:save-temporary")).Height(26).Padding(0, 8).
							Gap(8).AlignItems(ui.Center).Cursor(ui.CursorPointer)
						if saveIndex == ch.sel || r.Hovered() {
							r.Background(p.SurfaceHover)
						}
						r.OnClick(func() {
							ch.sel = saveIndex
							m.chooseActivate(ch.mode)
						})
						r.Children(func() {
							ui.Text(c, "Save '"+m.run.inline.Name+"' Configuration").FontSize(12).
								TextColor(p.Text).SingleLine()
						})
					}
					currentIndex := m.chooserItemIndex(runChooserCurrent, "")
					if currentIndex >= 0 {
						r := ui.Row(c.Key("runcfg:current-file")).Height(26).Padding(0, 8).Gap(8).AlignItems(ui.Center)
						if currentIndex == ch.sel || r.Hovered() {
							r.Background(p.SurfaceHover)
						}
						r.OnClick(func() {
							ch.sel = currentIndex
							m.chooseActivate(runChooserExecutor(ch.mode, c.Modifiers()))
						})
						r.Children(func() {
							nameColor := p.Text
							if m.run.currentFile {
								nameColor = p.Accent
							}
							ui.Text(c, "Current File").FontSize(12).TextColor(nameColor).SingleLine()
							action := "Run Active File"
							if ch.mode == "debug" {
								action = "Debug Active File"
							}
							ui.Text(c, action).FontSize(12).TextColor(p.TextMuted).SingleLine()
						})
					}
					rowIndex := make(map[string]int, len(items))
					for index, item := range items {
						if item.kind == runChooserConfig && item.cfg != nil {
							rowIndex[item.cfg.ID] = index
						}
					}
					prefs := m.runProjectPrefsState()
					pinned := map[string]bool{}
					for _, id := range prefs.Pinned {
						pinned[id] = true
					}
					temporary := m.runTemporaryIDs()
					number := 0
					for _, section := range m.chooserSections() {
						ui.Text(c, section.label).FontSize(11).Bold().TextColor(p.TextMuted).
							Padding(8, 10).SingleLine()
						for _, cfg := range section.configs {
							cfgi := cfg
							i := rowIndex[cfg.ID]
							number++
							r := ui.Row(c.Key("runcfg:"+cfgi.ID)).Height(26).Padding(0, 8).
								Gap(8).AlignItems(ui.Center).Cursor(ui.CursorPointer)
							if i == ch.sel {
								r.Background(p.SurfaceHover)
							} else if r.Hovered() {
								r.Background(p.SurfaceHover)
							}
							r.OnClick(func() {
								ch.sel = i
								m.chooseActivate(runChooserExecutor(ch.mode, c.Modifiers()))
							})
							r.Children(func() {
								if ch.query == "" && ch.mode != "select" && number <= 9 {
									ui.Text(c, fmt.Sprintf("%d", number)).FontSize(10).
										TextColor(p.TextMuted).Width(14).SingleLine()
								}
								ui.Text(c, cfgi.Name).FontSize(12).Bold().TextColor(p.Text).
									Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
								if target := m.runConfigTargetLabel(cfgi); target != "" {
									ui.Text(c, "@ "+target).FontSize(10).TextColor(p.TextMuted).SingleLine()
								}
								ui.Text(c, adapter.KindLabel(cfgi.Kind)).FontSize(11).
									TextColor(p.TextMuted).SingleLine()
								if cfgi.IsTest {
									ui.Text(c, "test").FontSize(10).TextColor(p.TextMuted).SingleLine()
								}
								if !cfgi.Detected {
									ui.Text(c, "user").FontSize(10).TextColor(p.TextMuted).SingleLine()
								}
								if temporary[cfgi.ID] {
									ui.Text(c, "temporary").FontSize(10).TextColor(p.TextMuted).SingleLine()
								}
								if m.run.current() != nil && m.run.current().ID == cfgi.ID {
									ui.Text(c, "Current").FontSize(10).TextColor(p.Accent).SingleLine()
								}
								pinLabel := "Pin"
								if pinned[cfgi.ID] {
									pinLabel = "Unpin"
								}
								if i == ch.sel {
									session := m.chooserConfigSession(cfgi.ID)
									ui.Button(c, pinLabel).OnClick(func() {
										m.togglePinnedRunConfig(cfgi.ID)
									})
									if session.running {
										ui.Button(c, "Rerun").OnClick(func() {
											m.activateChooserConfigSession(cfgi.ID, session, true)
										})
										ui.Button(c, "Stop").OnClick(func() {
											m.activateChooserConfigSession(cfgi.ID, session, false)
										})
									} else {
										ui.Button(c, "Run").OnClick(func() {
											ch.sel = i
											m.chooseActivate("run")
										})
										ui.Button(c, "Debug").OnClick(func() {
											ch.sel = i
											m.chooseActivate("debug")
										})
									}
								}
							})
						}
					}
					editIndex := m.chooserItemIndex(runChooserEdit, "")
					edit := ui.Row(c.Key("runcfg:edit")).Height(26).Padding(0, 8).Gap(8).
						AlignItems(ui.Center).Cursor(ui.CursorPointer)
					if editIndex == ch.sel || edit.Hovered() {
						edit.Background(p.SurfaceHover)
					}
					edit.OnClick(func() {
						ch.sel = editIndex
						m.chooseActivate(ch.mode)
					})
					edit.Children(func() {
						ui.Text(c, "Edit Configurations…").FontSize(12).TextColor(p.Text).SingleLine()
					})
				})
			})
			if ch.mode != "select" {
				ui.Row(c).Children(func() {
					hint := "Hold Shift to Debug, F4 to edit, Delete to remove"
					if ch.mode == "debug" {
						hint = "Hold Shift to Run, F4 to edit, Delete to remove"
					}
					ui.Text(c, hint).FontSize(11).TextColor(p.TextMuted).SingleLine()
				})
			}
		})
	})
	if d := open; !d {
		m.closeRunChooser()
	}
}
