package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/adapter"
	"mayoi/internal/openidea"
)

// runConsoleState is the per-tab state OpenIDEA keeps in SessionPanel. The
// state is keyed by the run tab so switching tabs does not carry one console's
// find query or follow setting into another.
type runConsoleState struct {
	key         string
	searching   bool
	query       string
	activeMatch int
	lastQuery   string
	lastMatches []int
	noFollow    bool
	noSoftWrap  bool
	input       string
	list        ui.ListState
}

type runSessionKind uint8

const (
	runSessionPrimary runSessionKind = iota
	runSessionChild
	runSessionAgent
	runSessionTrace
	runSessionService
)

type runSessionView struct {
	kind        runSessionKind
	key         string
	name        string
	config      *openidea.RunConfig
	live        bool
	failed      bool
	stopped     bool
	finished    bool
	stdinClosed bool
	runID       uint32
	channel     uint32
	agentID     uint64
	exitCode    *int32
	started     time.Time
	ended       time.Time
	err         string
	output      []outputLine
	truncated   bool
	dropped     int
	tests       *adapter.TestResults
	selected    string
}

type runTab struct {
	key      string
	label    string
	icon     string
	live     bool
	failed   bool
	finished bool
	closable bool
	dimmed   bool
	tooltip  string
	kind     runSessionKind
	session  *runSessionView
	trace    int
}

func runPrimaryTabKey() string { return "run:primary" }

func runChildTabKey(channel uint32) string {
	return fmt.Sprintf("run:child:%d", channel)
}

func runAgentTabKey(id uint64) string {
	return fmt.Sprintf("run:agent:%d", id)
}

func runTraceTabKey(index int) string {
	return fmt.Sprintf("run:trace:%d", index)
}

func (p *RunPane) consoleState(key string) *runConsoleState {
	if p.console == nil {
		p.console = map[string]*runConsoleState{}
	}
	state := p.console[key]
	if state == nil {
		state = &runConsoleState{key: key, list: ui.ListState{}}
		p.console[key] = state
	}
	return state
}

func (p *RunPane) hasPrimarySession() bool {
	return p.channel != 0 || p.runID != 0 || p.running || p.awaiting ||
		p.lastName != "" || p.exitCode != nil || len(p.output) > 0
}

func (p *RunPane) primaryConfig() *openidea.RunConfig {
	if p.lastID != "" {
		for i := range p.configs {
			if p.configs[i].ID == p.lastID {
				return &p.configs[i]
			}
		}
		for _, cfg := range p.temporaries {
			if cfg != nil && cfg.ID == p.lastID {
				return cfg
			}
		}
	}
	if p.inline != nil {
		return p.inline
	}
	if p.sel >= 0 && p.sel < len(p.configs) {
		return &p.configs[p.sel]
	}
	return nil
}

func (p *RunPane) primaryName() string {
	if p.lastName != "" {
		return p.lastName
	}
	if cfg := p.primaryConfig(); cfg != nil {
		return cfg.Name
	}
	return "Run"
}

func (m *Model) primaryRunSession() *runSessionView {
	p := &m.run
	if !p.hasPrimarySession() {
		return nil
	}
	cfg := p.primaryConfig()
	live := p.running || p.awaiting || p.busy
	failed := p.err != "" || p.exitCode != nil && *p.exitCode != 0 && !p.stopped
	return &runSessionView{
		kind: runSessionPrimary, key: runPrimaryTabKey(), name: m.runTabName(p.primaryName(), cfg),
		config: cfg, live: live, failed: failed, stopped: p.stopped,
		finished: !live, runID: p.runID, channel: p.channel,
		stdinClosed: p.stdinClosed,
		exitCode:    p.exitCode, started: p.started, ended: p.ended,
		err: p.err, output: p.output, truncated: p.truncated, dropped: p.dropped,
		tests: p.tests, selected: p.selTest,
	}
}

func (m *Model) childRunSession(child *runChild) *runSessionView {
	if child == nil {
		return nil
	}
	failed := child.err != "" ||
		child.exitCode != nil && *child.exitCode != 0 && !child.stopped
	return &runSessionView{
		kind: runSessionChild, key: runChildTabKey(child.channel),
		name:   m.runTabName(child.name, &child.config),
		config: &child.config, live: child.live(), failed: failed, stopped: child.stopped,
		finished: child.finished, runID: child.runID, channel: child.channel,
		stdinClosed: child.stdinClosed,
		exitCode:    child.exitCode, started: child.started, ended: child.ended,
		err: child.err, output: child.output, truncated: child.truncated,
		dropped: child.dropped, tests: child.tests, selected: child.selTest,
	}
}

func (m *Model) serviceRunSession(child *runChild) *runSessionView {
	if child == nil {
		return nil
	}
	failed := child.err != "" ||
		child.exitCode != nil && *child.exitCode != 0 && !child.stopped
	return &runSessionView{
		kind: runSessionService, key: runChildTabKey(child.channel),
		name:   m.runTabName(child.name, &child.config),
		config: &child.config, live: child.live(), failed: failed, stopped: child.stopped,
		finished: child.finished, runID: child.runID, channel: child.channel,
		stdinClosed: child.stdinClosed, exitCode: child.exitCode,
		started: child.started, ended: child.ended, err: child.err,
		output: child.output, truncated: child.truncated, dropped: child.dropped,
		tests: child.tests, selected: child.selTest,
	}
}

func (m *Model) runTabName(name string, config *openidea.RunConfig) string {
	if config == nil || config.Target == nil || strings.TrimSpace(*config.Target) == "" {
		return name
	}
	target := strings.TrimSpace(*config.Target)
	for _, item := range m.remote.runTargets.settings.Targets {
		if item.ID == target {
			if label := item.Label(); label != "" {
				target = label
			}
			break
		}
	}
	return name + " @ " + target
}

func (m *Model) agentRunSessionView(run *agentRunSession) *runSessionView {
	if run == nil {
		return nil
	}
	failed := run.err != "" || run.exitCode != nil && *run.exitCode != 0 && !run.stopped
	return &runSessionView{
		kind: runSessionAgent, key: runAgentTabKey(run.id), name: run.request.Name,
		live: run.live(), failed: failed, stopped: run.stopped, finished: !run.live(),
		runID: run.runID, channel: run.channel, agentID: run.id,
		stdinClosed: run.stdinClosed,
		exitCode:    run.exitCode, started: run.started, ended: run.ended,
		err: run.err, output: run.output, truncated: run.truncated, dropped: run.dropped,
	}
}

func (m *Model) activeRunSession() *runSessionView {
	if m.stackTrace != nil && m.stackTrace.active >= 0 {
		return nil
	}
	if agent := m.activeAgentRun(); agent != nil {
		return m.agentRunSessionView(agent)
	}
	if child := m.activeChild(); child != nil {
		return m.childRunSession(child)
	}
	if primary := m.primaryRunSession(); primary != nil {
		return primary
	}
	visible := m.visibleAgentRuns()
	if len(visible) > 0 {
		return m.agentRunSessionView(visible[len(visible)-1])
	}
	if len(m.run.children) > 0 {
		return m.childRunSession(m.run.children[len(m.run.children)-1])
	}
	return nil
}

func (m *Model) runTabs() []runTab {
	var tabs []runTab
	if session := m.primaryRunSession(); session != nil {
		tabs = append(tabs, m.runTabForSession(session))
	}
	for _, child := range m.run.children {
		if child == nil {
			continue
		}
		if session := m.childRunSession(child); session != nil {
			tabs = append(tabs, m.runTabForSession(session))
		}
	}
	for _, run := range m.visibleAgentRuns() {
		if session := m.agentRunSessionView(run); session != nil {
			tabs = append(tabs, m.runTabForSession(session))
		}
	}
	if m.stackTrace != nil {
		for i, trace := range m.stackTrace.tabs {
			tabs = append(tabs, runTab{
				key: runTraceTabKey(i), label: trace.title, icon: "fileTypes/text",
				closable: true, kind: runSessionTrace, trace: i,
			})
		}
	}
	return tabs
}

func (m *Model) runTabForSession(session *runSessionView) runTab {
	icon := "run/run"
	switch {
	case session.live && session.config != nil && session.config.IsTest:
		icon = "general/greenCheckmark"
	case session.live:
		icon = "run/run"
	case session.failed:
		icon = "run/testFailed"
	case session.kind == runSessionAgent:
		icon = "toolwindows/terminal"
	default:
		icon = runConfigIconPath(session.config)
	}
	tooltip := ""
	if session.finished {
		tooltip = runFinishedTooltip(session)
	}
	return runTab{
		key: session.key, label: session.name, icon: icon, live: session.live,
		failed: session.failed, finished: session.finished, closable: true,
		dimmed:  session.kind == runSessionAgent && session.finished,
		tooltip: tooltip, kind: session.kind, session: session,
	}
}

func runConfigIconPath(config *openidea.RunConfig) string {
	if config == nil {
		return "run/run"
	}
	if config.IsTest {
		return "general/greenCheckmark"
	}
	fileIcon := "file-unknown"
	switch config.Kind {
	case "compound":
		return "run/run"
	case "program", "jar-application", "native":
		fileIcon = "file-binary"
	case "spring-boot":
		return "openidea/runConfigurations/springBoot"
	case "tomcat":
		return "openidea/runConfigurations/tomcat"
	case "npm":
		fileIcon = "file-npm"
	case "deno":
		fileIcon = "file-ts"
	case "cargo":
		fileIcon = "file-cargo"
	case "gradle":
		fileIcon = "file-gradle"
	case "maven":
		fileIcon = "file-maven"
	case "dotnet", "dotnet-test":
		fileIcon = "file-csharp"
	case "go":
		fileIcon = "file-go"
	case "node":
		fileIcon = "file-js"
	case "application":
		return "fileTypes/java"
	case "java-file":
		fileIcon = "file-java"
	case "rustc":
		fileIcon = "file-rust"
	case "cc", "cmake-application":
		fileIcon = "file-cpp"
	case "shell":
		fileIcon = "file-shell"
	case "ruby":
		fileIcon = "file-ruby"
	case "php", "phpunit", "composer":
		fileIcon = "file-php"
	case "perl":
		fileIcon = "file-perl"
	case "lua":
		fileIcon = "file-lua"
	case "r":
		fileIcon = "file-r"
	case "swift":
		fileIcon = "file-swift"
	case "dart":
		fileIcon = "file-dart"
	case "elixir":
		fileIcon = "file-elixir"
	case "scala-cli":
		fileIcon = "file-scala"
	case "kotlinc":
		fileIcon = "file-kotlin"
	case "kotlin":
		fileIcon = "file-kotlin-script"
	case "python", "pytest":
		fileIcon = "file-python"
	default:
		return "toolwindows/terminal"
	}
	if path := fileIconPaths[fileIcon]; path != "" {
		return path
	}
	return "toolwindows/terminal"
}

func runFinishedTooltip(session *runSessionView) string {
	if session.started.IsZero() || session.ended.IsZero() || session.ended.Before(session.started) {
		if session.exitCode != nil {
			return fmt.Sprintf("Finished, exit code %d", *session.exitCode)
		}
		return "Finished, exit code ?"
	}
	exit := "?"
	if session.exitCode != nil {
		exit = fmt.Sprint(*session.exitCode)
	}
	return fmt.Sprintf("Finished in %s, exit code %s",
		formatRunDuration(session.ended.Sub(session.started)), exit)
}

func formatRunDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%d ms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1f s", d.Seconds())
	}
	return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
}

func (m *Model) runSessionTabs(c *ui.Context, p *ui.Theme) {
	tabs := m.runTabs()
	if len(tabs) == 0 && len(m.archivedAgentRuns()) == 0 {
		return
	}
	oi := oiPaletteOf(p)
	activeKey := m.activeRunTabKey()
	ui.Row(c).Height(30).Shrink(0).Padding(0, 6).Gap(2).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(oi.border).Children(func() {
		ui.ScrollHorizontal(c).Grow(1).MinWidth(0).Shrink(1).Children(func() {
			ui.Row(c).Height(28).Gap(2).AlignItems(ui.Center).Children(func() {
				for i := range tabs {
					m.runSessionTab(c, p, oi, &tabs[i], tabs[i].key == activeKey)
				}
			})
		})
		if archive := m.archivedAgentRuns(); len(archive) > 0 {
			m.runAgentArchive(c, p, archive)
		}
		m.runTabsMore(c, p, oi)
		b := ui.ButtonBase(c.Key("run:hide")).Size(24, 24).Radius(4).
			AlignItems(ui.Center).Justify(ui.Center).Tooltip("Hide Run Tool Window").
			Label("Hide Run Tool Window")
		if b.Hovered() {
			b.Background(oi.toolbarHover)
		}
		b.Children(func() {
			oiIconEl(c, "general/hide", 16, oi, oi.icon)
		})
		if b.Clicked() {
			m.DockHide("run")
		}
	})
}

func (m *Model) runViewedServiceTabs(c *ui.Context, p *ui.Theme, session *runSessionView) {
	if session == nil {
		return
	}
	oi := oiPaletteOf(p)
	tab := m.runTabForSession(session)
	if child := m.run.viewChild; child != nil {
		tab.tooltip = serviceExecutionDetail(child)
	}
	ui.Row(c).Height(30).Shrink(0).Padding(0, 6).Gap(2).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(oi.border).Children(func() {
		ui.ScrollHorizontal(c).Grow(1).MinWidth(0).Shrink(1).Children(func() {
			ui.Row(c).Height(28).Children(func() {
				m.runSessionTab(c, p, oi, &tab, true)
			})
		})
		button := ui.ButtonBase(c.Key("run:show-service-in-services")).Height(24).
			Padding(0, 8).Radius(4).Tooltip("Show in Services").
			Label("Show in Services")
		if button.Hovered() {
			button.Background(oi.toolbarHover)
		}
		button.Children(func() {
			ui.Text(c, "Show in Services").FontSize(12).TextColor(oi.textSecondary)
		})
		if button.Clicked() {
			m.showViewedServiceInServices()
		}
		b := ui.ButtonBase(c.Key("run:hide")).Size(24, 24).Radius(4).
			AlignItems(ui.Center).Justify(ui.Center).Tooltip("Hide Run Tool Window").
			Label("Hide Run Tool Window")
		if b.Hovered() {
			b.Background(oi.toolbarHover)
		}
		b.Children(func() { oiIconEl(c, "general/hide", 16, oi, oi.icon) })
		if b.Clicked() {
			m.DockHide("run")
		}
	})
}

func (m *Model) showViewedServiceInServices() {
	child := m.run.viewChild
	if child == nil {
		return
	}
	m.run.viewChild = nil
	m.selectOnlyServiceKey(serviceConfigKey(child.config.ID))
	m.ShowServices()
}

func (m *Model) runAgentArchive(c *ui.Context, p *ui.Theme, archive []*agentRunSession) {
	oi := oiPaletteOf(p)
	button := ui.ButtonBase(c.Key("run-agent-archive")).Height(24).
		Padding(0, 6).Gap(4).Radius(4).
		AlignItems(ui.Center).Cursor(ui.CursorPointer).
		Tooltip("Archived agent tabs").Label("Archived agent tabs")
	if button.Hovered() {
		button.Background(oi.toolbarHover)
	}
	button.Children(func() {
		oiIconEl(c, "nodes/folder", 14, oi, oi.textMuted)
		ui.Text(c, fmt.Sprintf("Archived (%d)", len(archive))).
			FontSize(12).TextColor(oi.textMuted).SingleLine()
	})
	button.Menu(func(menu *ui.Menu) {
		for _, run := range archive {
			item := menu.Item(run.request.Name)
			item.Checked(run.id == m.run.agentActive)
			if item.Chosen() {
				m.selectAgentRun(run.id)
			}
		}
		menu.Separator()
		if menu.Item("Close All Archived Tabs").Chosen() {
			m.closeAgentArchive()
			m.hideRunWhenEmpty()
		}
	})
}

func (m *Model) runSessionTab(c *ui.Context, p *ui.Theme, oi oiColors, tab *runTab, active bool) {
	row := ui.Row(c.Key("run-tab:"+tab.key)).Height(26).Padding(0, 7).Gap(6).
		Radius(5).AlignItems(ui.Center).Cursor(ui.CursorPointer).
		Label(tab.label + " Run Tab")
	if tab.tooltip != "" {
		row.Tooltip(tab.tooltip)
	}
	if active {
		row.Background(oi.tabSelected).BorderWidth(1, 1, 1, 1).
			BorderColor(oi.tabSelectedBorder)
	} else if row.Hovered() {
		row.Background(oi.toolbarHover)
	}
	row.OnClick(func() { m.selectRunTab(tab) })
	row.Children(func() {
		icon := oiIconEl(c, tab.icon, 15, oi, oi.icon)
		if tab.failed {
			icon = oiIconEl(c, "run/testFailed", 15, oi, oi.danger)
		}
		if tab.dimmed {
			icon.Opacity(0.7)
		}
		textColor := oi.text
		if tab.dimmed {
			textColor = oi.textMuted
		}
		if tab.failed {
			textColor = oi.danger
		}
		ui.Text(c, tab.label).FontSize(12).TextColor(textColor).SingleLine().
			Ellipsis("…").MaxWidth(230)
		if tab.closable {
			closeButton := ui.ButtonBase(c.Key("run-tab-close:"+tab.key)).
				Size(18, 18).Justify(ui.Center).AlignItems(ui.Center).Radius(4).
				Cursor(ui.CursorPointer).Tooltip("Close " + tab.label).
				Label("Close " + tab.label + " Run Tab")
			if closeButton.Hovered() {
				closeButton.Background(oi.hover)
			}
			closeButton.Children(func() {
				ui.Text(c, "×").FontSize(12).TextColor(oi.textMuted)
			})
			if closeButton.Clicked() {
				m.closeRunTab(tab)
			}
		}
	})
}

func (m *Model) activeRunTabKey() string {
	if m.stackTrace != nil && m.stackTrace.active >= 0 {
		return runTraceTabKey(m.stackTrace.active)
	}
	if run := m.activeAgentRun(); run != nil {
		return runAgentTabKey(run.id)
	}
	if child := m.activeChild(); child != nil {
		return runChildTabKey(child.channel)
	}
	if m.run.hasPrimarySession() {
		return runPrimaryTabKey()
	}
	if visible := m.visibleAgentRuns(); len(visible) > 0 {
		return runAgentTabKey(visible[len(visible)-1].id)
	}
	if len(m.run.children) > 0 {
		return runChildTabKey(m.run.children[len(m.run.children)-1].channel)
	}
	return ""
}

func (m *Model) selectRunTab(tab *runTab) {
	if m.stackTrace != nil {
		m.stackTrace.active = -1
	}
	switch {
	case tab.kind == runSessionTrace:
		if m.stackTrace != nil {
			m.stackTrace.active = tab.trace
		}
	case tab.kind == runSessionAgent && tab.session != nil:
		m.selectAgentRun(tab.session.agentID)
	case tab.kind == runSessionChild && tab.session != nil:
		m.selectRunChild(tab.session.channel)
	case tab.kind == runSessionPrimary:
		m.run.activeChild = 0
		m.run.agentActive = 0
	}
	m.invalidate()
}

func (m *Model) closeRunTab(tab *runTab) {
	switch {
	case tab.kind == runSessionTrace:
		m.closeStackTraceTab(tab.trace)
		m.hideRunWhenEmpty()
	case tab.kind == runSessionAgent && tab.session != nil:
		m.requestCloseAgentRun(tab.session.agentID)
	case tab.kind == runSessionChild && tab.session != nil:
		m.requestCloseRunChild(tab.session.channel)
	case tab.kind == runSessionPrimary:
		m.requestClosePrimaryRun()
	}
	m.invalidate()
}

func (m *Model) requestClosePrimaryRun() {
	session := m.primaryRunSession()
	if session == nil {
		return
	}
	if !session.live {
		m.dropPrimaryRunSession()
		m.hideRunWhenEmpty()
		return
	}
	m.requestRunTermination(session.name, func() {
		m.StopRun()
		m.dropPrimaryRunSession()
		m.hideRunWhenEmpty()
	})
}

func (m *Model) dropPrimaryRunSession() {
	p := &m.run
	p.busy = false
	p.running = false
	p.awaiting = false
	p.stopped = false
	p.runID = 0
	p.channel = 0
	p.pid = nil
	p.exitCode = nil
	p.err = ""
	p.started = time.Time{}
	p.ended = time.Time{}
	p.lastID = ""
	p.lastRaw = nil
	p.lastName = ""
	p.output = nil
	p.truncated = false
	p.dropped = 0
	p.tests = nil
	p.selTest = ""
	delete(p.console, runPrimaryTabKey())
	m.invalidate()
}

func (m *Model) hideRunWhenEmpty() {
	hasRun := m.run.hasPrimarySession() || len(m.run.children) > 0 || len(m.run.agentRuns) > 0
	hasTrace := m.stackTrace != nil && len(m.stackTrace.tabs) > 0
	if !hasRun && !hasTrace {
		m.run.visible = false
		m.DockHide("run")
	}
}

func (m *Model) hasRunContent() bool {
	return m.run.hasPrimarySession() || len(m.run.children) > 0 || len(m.run.agentRuns) > 0 ||
		m.stackTrace != nil && len(m.stackTrace.tabs) > 0
}

func (m *Model) runTabsMore(c *ui.Context, p *ui.Theme, oi oiColors) {
	b := ui.ButtonBase(c.Key("run:tabs-more")).Size(24, 24).Radius(4).
		AlignItems(ui.Center).Justify(ui.Center).Tooltip("More Run Actions").
		Label("More Run Actions")
	if b.Hovered() {
		b.Background(oi.toolbarHover)
	}
	b.Children(func() { oiIconEl(c, "general/moreHorizontal", 16, oi, oi.icon) })
	b.Menu(func(menu *ui.Menu) {
		activeKey := m.activeRunTabKey()
		for _, tab := range m.runTabs() {
			tab := tab
			item := menu.Item(tab.label)
			item.Checked(tab.key == activeKey)
			if item.Chosen() {
				m.selectRunTab(&tab)
			}
		}
		menu.Separator()
		if menu.Item("Choose Run/Debug Configuration…").Chosen() {
			m.RunAction("run.chooseConfig")
		}
		if menu.Item("Edit Configurations…").Chosen() {
			m.RunAction("run.editConfigs")
		}
		if menu.Item("Reload Run Configurations").Chosen() {
			m.RunAction("run.reloadConfigs")
		}
		menu.Separator()
		if menu.Item("Close Active Tab").Chosen() {
			for _, tab := range m.runTabs() {
				if tab.key == m.activeRunTabKey() {
					m.closeRunTab(&tab)
					break
				}
			}
		}
		if menu.Item("Close Finished Run Tabs").Chosen() {
			m.closeFinishedRunTabs()
		}
	})
}

func (m *Model) closeFinishedRunTabs() {
	if session := m.primaryRunSession(); session != nil && !session.live {
		m.dropPrimaryRunSession()
	}
	for _, child := range append([]*runChild(nil), m.run.children...) {
		if child != nil && !child.live() {
			m.closeRunChild(child.channel)
		}
	}
	for _, run := range append([]*agentRunSession(nil), m.run.agentRuns...) {
		if run != nil && !run.live() {
			m.closeAgentRun(run.id)
		}
	}
	m.hideRunWhenEmpty()
}

func (m *Model) runEmptyState(c *ui.Context, p *ui.Theme) {
	oi := oiPaletteOf(p)
	ui.Column(c).Grow(1).MinHeight(0).AlignItems(ui.Center).Justify(ui.Center).
		Padding(24).Children(func() {
		ui.Column(c).MaxWidth(520).Gap(6).Children(func() {
			ui.Text(c, "To run your code, do one of the following:").
				FontSize(12).TextColor(oi.textSubtle)
			ui.Text(c, "— Click the Run icon in the editor gutter").
				FontSize(12).TextColor(oi.textSubtle)
			ui.Text(c, `— Select "Run…" in the editor context menu`).
				FontSize(12).TextColor(oi.textSubtle)
			ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "— Launch a").FontSize(12).TextColor(oi.textSubtle)
				link := ui.ButtonBase(c.Key("run:empty-edit-configs")).Padding(0).
					Radius(0).Cursor(ui.CursorPointer).Label("run configuration")
				link.Children(func() {
					ui.Text(c, "run configuration").FontSize(12).
						TextColor(oi.link).Underline()
				})
				if link.Clicked() {
					m.RunAction("run.editConfigs")
				}
			})
			if !m.run.trusted {
				ui.Text(c, "This project is not trusted, so Run is unavailable.").
					FontSize(12).TextColor(oi.textMuted).Margin(0, 8, 0, 0)
			}
		})
	})
}

func (m *Model) runSessionPanel(c *ui.Context, p *ui.Theme, session *runSessionView) {
	oi := oiPaletteOf(p)
	ui.Row(c).Grow(1).MinWidth(0).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
		m.runSessionRail(c, p, oi, session)
		ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
			if session.err != "" {
				ui.Row(c).Height(26).Shrink(0).Padding(0, 10).AlignItems(ui.Center).
					Background(oi.elev).Children(func() {
					ui.Text(c, session.err).FontSize(12).TextColor(oi.warning).
						SingleLine().Ellipsis("…").Grow(1)
				})
			}
			m.runSessionBody(c, p, session)
		})
	})
}

func (m *Model) runSessionRail(c *ui.Context, p *ui.Theme, oi oiColors, session *runSessionView) {
	ui.Column(c).Width(32).Shrink(0).Padding(6, 3).Gap(2).
		BorderWidth(0, 1, 0, 0).BorderColor(oi.border).Children(func() {
		m.runActionIcon(c, p, oi, "rerun", "run/rerun", "Rerun", true, false,
			func() { m.rerunRunSession(session) })
		if session.kind != runSessionAgent {
			m.runActionIcon(c, p, oi, "debug", "run/debug", "Debug", session.config != nil, false,
				func() { m.debugRunSession(session) })
		}
		if session.tests != nil {
			failed := len(adapter.FailedTestPaths(session.tests))
			m.runActionIcon(c, p, oi, "rerun-failed", "run/rerun",
				"Rerun Failed Tests", session.kind == runSessionPrimary && !session.live && failed > 0,
				false, m.RerunFailedTests)
		}
		m.runActionIconDanger(c, p, oi, "stop", "run/stop", "Stop", session.live,
			func() { m.stopRunSession(session) })
		if session.config != nil && applicationConfigUpdatable(session.config) {
			m.runActionIcon(c, p, oi, "update", "general/refresh", "Update Application",
				session.live, false, func() { m.updateRunSession(session) })
		}
		m.runRailSeparator(c, oi)
		state := m.run.consoleState(session.key)
		m.runActionIcon(c, p, oi, "find", "general/search", "Find…", true,
			state.searching, func() {
				state.searching = !state.searching
				if !state.searching {
					state.query = ""
				}
				m.invalidate()
			})
		m.runActionIcon(c, p, oi, "follow", "run/stepInto", "Scroll to End", true,
			!state.noFollow, func() {
				state.noFollow = !state.noFollow
				if !state.noFollow {
					state.list.ScrollToEnd()
				}
				m.invalidate()
			})
		m.runActionIcon(c, p, oi, "wrap", "general/softWrap", "Soft-Wrap", true,
			!state.noSoftWrap, func() {
				state.noSoftWrap = !state.noSoftWrap
				m.invalidate()
			})
		m.runActionIcon(c, p, oi, "clear", "general/delete", "Clear All", true, false,
			func() { m.clearRunSession(session) })
		m.runRailSeparator(c, oi)
		m.runActionIcon(c, p, oi, "edit", "general/settings", "Edit Configurations…",
			session.kind != runSessionAgent && session.config != nil, false,
			func() { m.editRunSession(session) })
		m.runActionIcon(c, p, oi, "more", "general/moreVertical", "More Run Actions", true, false, nil).
			Menu(func(menu *ui.Menu) { m.runSessionMoreMenu(menu, session) })
	})
}

func (m *Model) runRailSeparator(c *ui.Context, oi oiColors) {
	ui.Box(c).Width(16).Height(1).Shrink(0).Margin(3, 0, 3, 0).
		Background(oi.borderStrong)
}

func (m *Model) runActionIcon(
	c *ui.Context,
	p *ui.Theme,
	oi oiColors,
	key, icon, label string,
	enabled, pressed bool,
	run func(),
) ui.Element {
	b := ui.ButtonBase(c.Key("run-action:"+key)).Size(25, 24).Shrink(0).Radius(4).
		Disabled(!enabled).AlignItems(ui.Center).Justify(ui.Center).
		Tooltip(label).Label(label)
	switch {
	case pressed:
		b.Background(oi.selected)
	case enabled && b.Hovered():
		b.Background(oi.toolbarHover)
	}
	b.Children(func() {
		icon := oiIconEl(c, icon, 16, oi, oi.icon)
		if !enabled {
			icon.Opacity(0.38)
		}
	})
	if run != nil && enabled && b.Clicked() {
		run()
	}
	return b
}

func (m *Model) runActionIconDanger(
	c *ui.Context,
	p *ui.Theme,
	oi oiColors,
	key, icon, label string,
	enabled bool,
	run func(),
) {
	b := ui.ButtonBase(c.Key("run-action:"+key)).Size(25, 24).Shrink(0).Radius(4).
		Disabled(!enabled).AlignItems(ui.Center).Justify(ui.Center).
		Tooltip(label).Label(label)
	if enabled && b.Hovered() {
		b.Background(oi.danger.Mix(oi.island, 0.82))
	}
	b.Children(func() {
		icon := oiIconEl(c, icon, 16, oi, oi.danger)
		if !enabled {
			icon.Opacity(0.38)
		}
	})
	if enabled && b.Clicked() {
		run()
	}
}

func (m *Model) runSessionBody(c *ui.Context, p *ui.Theme, session *runSessionView) {
	oi := oiPaletteOf(p)
	if session.tests == nil {
		ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
			m.runConsoleColumn(c, p, session)
		})
		return
	}
	ui.Row(c).Grow(1).MinWidth(0).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
		m.runTestPane(c, p, session, 34)
		ui.Box(c).Width(1).Shrink(0).Background(oi.border)
		ui.Column(c).Grow(66).MinWidth(0).MinHeight(0).Children(func() {
			m.runConsoleColumn(c, p, session)
		})
	})
}

func (m *Model) runTestPane(c *ui.Context, p *ui.Theme, session *runSessionView, grow float32) {
	oi := oiPaletteOf(p)
	ui.Column(c).Grow(grow).MinWidth(140).MinHeight(0).Children(func() {
		counts := session.tests.Counts()
		statusIcon := "run/testNotRunYet"
		statusColor := oi.textMuted
		switch {
		case session.live:
			statusIcon = "run/testPassed"
			statusColor = oi.success
		case counts.Failed > 0:
			statusIcon = "run/testFailed"
			statusColor = oi.danger
		case counts.Total > 0:
			statusIcon = "run/testPassed"
			statusColor = oi.success
		}
		ui.Row(c).Height(28).Shrink(0).Padding(0, 4, 0, 10).Gap(8).
			AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(oi.border).
			Children(func() {
				if session.live {
					ui.Spinner(c).Size(16, 16).TextColor(oi.textMuted).Label("Running tests")
				} else {
					oiIconEl(c, statusIcon, 16, oi, statusColor)
				}
				if counts.Failed > 0 {
					ui.Text(c, fmt.Sprintf("%d failing", counts.Failed)).
						FontSize(12).TextColor(oi.danger)
				}
				if counts.Passed > 0 {
					ui.Text(c, fmt.Sprintf("%d passed", counts.Passed)).
						FontSize(12).TextColor(oi.textSecondary)
				}
				if counts.Skipped > 0 {
					ui.Text(c, fmt.Sprintf("%d ignored", counts.Skipped)).
						FontSize(12).TextColor(oi.textSubtle)
				}
				if counts.Total == 0 {
					label := "No tests"
					if session.live {
						label = "Running…"
					}
					ui.Text(c, label).FontSize(12).TextColor(oi.textSubtle)
				}
				if duration, ok := testResultsDuration(session.tests); ok {
					ui.Text(c, formatTestDuration(duration)).
						FontSize(11).TextColor(oi.textSubtle)
				}
				ui.Spacer(c)
				m.runActionIcon(c, p, oi, "show-passed", "run/showPassed",
					"Show Passed", true, m.run.showPassedTests, func() {
						m.run.showPassedTests = !m.run.showPassedTests
						m.invalidate()
					})
				m.runActionIcon(c, p, oi, "show-ignored", "run/showIgnored",
					"Show Ignored", true, !m.run.hideIgnoredTests, func() {
						m.run.hideIgnoredTests = !m.run.hideIgnoredTests
						m.invalidate()
					})
			})

		tree := testTreeLines(session.tests, m.run.showPassedTests, !m.run.hideIgnoredTests)
		if len(tree) == 0 {
			ui.Text(c, "No tests").FontSize(12).TextColor(oi.textSubtle).Padding(12)
			return
		}
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).Padding(4, 6).Children(func() {
				for _, line := range tree {
					line := line
					row := ui.Row(c.Key("run-test:"+line.id)).Height(20).
						Padding(0, 4).Gap(6).AlignItems(ui.Center)
					if line.id == session.selected {
						row.Background(oi.selectedInactive)
					} else if row.Hovered() {
						row.Background(oi.toolbarHover)
					}
					row.OnClick(func() {
						m.setRunSelectedTest(session, line.id)
					})
					if row.DoubleClicked() {
						m.openTestLocation(line.loc)
					}
					row.Children(func() {
						ui.Text(c, line.mark+" "+line.text).FontSize(12).
							TextColor(oi.text).SingleLine().Ellipsis("…")
					})
				}
			})
		})
	})
}

func testResultsDuration(results *adapter.TestResults) (float64, bool) {
	if results == nil {
		return 0, false
	}
	var total float64
	any := false
	var visit func(*adapter.TestNode)
	visit = func(node *adapter.TestNode) {
		if node.DurationMs != nil {
			total += *node.DurationMs
			any = true
			return
		}
		if len(node.Children) == 0 {
			return
		}
		for _, child := range node.Children {
			visit(child)
		}
	}
	for _, root := range results.Roots {
		visit(root)
	}
	return total, any
}

func formatTestDuration(milliseconds float64) string {
	switch {
	case milliseconds < 1000:
		return fmt.Sprintf("%.0f ms", milliseconds)
	case milliseconds < 60_000:
		value := milliseconds / 1000
		if value < 10 {
			return fmt.Sprintf("%.2f s", value)
		}
		return fmt.Sprintf("%.1f s", value)
	default:
		minutes := int(milliseconds / 60_000)
		seconds := int((milliseconds - float64(minutes)*60_000) / 1000)
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
}

func (m *Model) setRunSelectedTest(session *runSessionView, selected string) {
	if session == nil {
		return
	}
	switch session.kind {
	case runSessionPrimary:
		m.run.selTest = selected
	case runSessionChild, runSessionService:
		if child := m.childByChannel(session.channel); child != nil {
			child.selTest = selected
		}
	}
	m.invalidate()
}

func (m *Model) clearRunSelectedTest(session *runSessionView) {
	if session == nil {
		return
	}
	switch session.kind {
	case runSessionPrimary:
		m.run.selTest = ""
	case runSessionChild, runSessionService:
		if child := m.childByChannel(session.channel); child != nil {
			child.selTest = ""
		}
	}
	m.invalidate()
}

func testTreeLines(results *adapter.TestResults, showPassed, showIgnored bool) []testTreeLine {
	if results == nil || results.IsEmpty() {
		return nil
	}
	mark := func(n *adapter.TestNode) string {
		switch adapter.TestNodeStatusOf(n) {
		case adapter.TestPassed:
			return "✓"
		case adapter.TestFailed:
			return "✗"
		case adapter.TestSkipped:
			return "⊘"
		case adapter.TestRunning:
			return "⟳"
		}
		return "·"
	}
	var out []testTreeLine
	var walk func(n *adapter.TestNode, depth int) bool
	walk = func(n *adapter.TestNode, depth int) bool {
		start := len(out)
		row := testTreeLine{
			id: n.ID, mark: mark(n),
			text: strings.Repeat("    ", depth) + n.Name,
			loc:  n.Location,
		}
		if n.DurationMs != nil {
			row.text += fmt.Sprintf(" (%.1f ms)", *n.DurationMs)
		}
		out = append(out, row)
		visible := len(n.Children) == 0 &&
			(showPassed || adapter.TestNodeStatusOf(n) != adapter.TestPassed) &&
			(showIgnored || adapter.TestNodeStatusOf(n) != adapter.TestSkipped)
		for _, child := range n.Children {
			if walk(child, depth+1) {
				visible = true
			}
		}
		if !visible {
			out = out[:start]
		}
		return visible
	}
	for _, root := range results.Roots {
		walk(root, 0)
	}
	return out
}

func (m *Model) runConsoleColumn(c *ui.Context, p *ui.Theme, session *runSessionView) {
	oi := oiPaletteOf(p)
	state := m.run.consoleState(session.key)
	ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
		selectedLines, selectedName := m.selectedTestOutput(session)
		lines := session.output
		if selectedLines != nil {
			lines = selectedLines
		}
		if session.dropped > 0 && selectedLines == nil {
			ui.Row(c).Height(24).Shrink(0).Padding(0, 12).AlignItems(ui.Center).
				BorderWidth(0, 0, 1, 0).BorderColor(oi.border).Children(func() {
				ui.Text(c, fmt.Sprintf("%d earlier lines were discarded", session.dropped)).
					FontSize(11).TextColor(oi.textSubtle).SingleLine()
			})
		}
		if selectedLines != nil {
			ui.Row(c).Height(24).Shrink(0).Padding(0, 12).AlignItems(ui.Center).
				BorderWidth(0, 0, 1, 0).BorderColor(oi.border).Children(func() {
				link := ui.ButtonBase(c.Key("run:show-all-output")).Padding(0).
					Radius(0).Cursor(ui.CursorPointer).Label("Show all output")
				link.Children(func() {
					ui.Text(c, "Show all output").FontSize(11).TextColor(oi.link).
						Underline()
				})
				if link.Clicked() {
					m.clearRunSelectedTest(session)
				}
				if selectedName != "" {
					ui.Text(c, " · "+selectedName).FontSize(11).TextColor(oi.textSubtle).
						SingleLine().Ellipsis("…")
				}
			})
		}
		if state.searching {
			m.runConsoleFindBar(c, p, session, state, lines)
		}
		m.runConsoleList(c, p, state, lines, selectedLines != nil)
		if session.live && session.runID != 0 && !session.stdinClosed && selectedLines == nil {
			m.runConsoleInput(c, p, session, state)
		}
	})
}

func (m *Model) runConsoleInput(c *ui.Context, p *ui.Theme, session *runSessionView, state *runConsoleState) {
	oi := oiPaletteOf(p)
	ui.Row(c).Height(30).Shrink(0).Padding(0, 8).Gap(6).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(oi.border).Children(func() {
		ui.Text(c, "›").Font(oiFontMono).FontSize(12).TextColor(oi.success)
		input := ui.TextInput(c.Key("run-console-input:"+session.key), &state.input).
			Height(24).Grow(1).Font(oiFontMono).Placeholder("Type input and press Enter").
			Label("Run console input")
		if input.Submitted() {
			line := state.input
			state.input = ""
			m.sendRunConsoleInput(session, line+"\n")
			m.invalidate()
		}
		m.runActionIcon(c, p, oi, "send-eof", "general/close", "Send EOF", true, false,
			func() { m.closeRunConsoleInput(session) })
	})
}

func (m *Model) sendRunConsoleInput(session *runSessionView, data string) {
	if session == nil || session.runID == 0 || !session.live {
		return
	}
	m.appendRunConsoleLine(session, "input", strings.TrimSuffix(data, "\n"))
	id := session.runID
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 15*time.Second)
		defer cancel()
		if _, err := m.invoke(ctx, "run_stdin", map[string]any{"id": id, "data": data}); err != nil {
			m.post(func() {
				m.appendRunConsoleLine(session, "system", "Cannot write to stdin: "+err.Error())
				m.invalidate()
			})
		}
	}()
}

func (m *Model) closeRunConsoleInput(session *runSessionView) {
	if session == nil || session.runID == 0 || !session.live {
		return
	}
	session.stdinClosed = true
	m.markRunStdinClosed(session)
	id := session.runID
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 15*time.Second)
		defer cancel()
		_, _ = m.invoke(ctx, "run_close_stdin", map[string]any{"id": id})
	}()
	m.invalidate()
}

func (m *Model) markRunStdinClosed(session *runSessionView) {
	switch session.kind {
	case runSessionPrimary:
		m.run.stdinClosed = true
	case runSessionChild, runSessionService:
		if child := m.childByChannel(session.channel); child != nil {
			child.stdinClosed = true
		}
	case runSessionAgent:
		if run := m.agentRunByID(session.agentID); run != nil {
			run.stdinClosed = true
		}
	}
}

func (m *Model) appendRunConsoleLine(session *runSessionView, kind, text string) {
	if session == nil || text == "" {
		return
	}
	switch session.kind {
	case runSessionPrimary:
		if strings.TrimSpace(m.run.lastName) != "" || m.run.hasPrimarySession() {
			m.run.append(openidea.OutputChunk{Kind: kind, Text: text})
		}
	case runSessionChild, runSessionService:
		if child := m.childByChannel(session.channel); child != nil {
			child.append(openidea.OutputChunk{Kind: kind, Text: text})
		}
	case runSessionAgent:
		if run := m.agentRunByID(session.agentID); run != nil {
			run.append(openidea.OutputChunk{Kind: kind, Text: text})
		}
	}
}

func (m *Model) selectedTestOutput(session *runSessionView) ([]outputLine, string) {
	if session == nil || session.tests == nil || session.selected == "" {
		return nil, ""
	}
	node := session.tests.Get(session.selected)
	if node == nil || len(node.Output) == 0 {
		return nil, ""
	}
	text := adapter.NodeOutputText(node)
	rows := make([]outputLine, 0, strings.Count(text, "\n")+1)
	for _, line := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
		rows = append(rows, outputLine{kind: "stdout", text: line})
	}
	return rows, node.Name
}

func (m *Model) runConsoleFindBar(
	c *ui.Context,
	p *ui.Theme,
	session *runSessionView,
	state *runConsoleState,
	lines []outputLine,
) {
	oi := oiPaletteOf(p)
	matches := runFindLines(lines, state.query)
	ui.Row(c).Height(30).Shrink(0).Padding(0, 8).Gap(6).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(oi.border).Children(func() {
		input := ui.TextInput(c.Key("run-console-find:"+session.key), &state.query).
			Height(24).Grow(1).AutoFocus().Placeholder("Find in Run Console").
			Label("Find in Run Console")
		if input.Changed() {
			state.activeMatch = 0
			state.lastQuery = ""
			if len(matches) > 0 {
				state.list.ScrollTo(matches[0], ui.Start)
			}
			m.invalidate()
		}
		count := "No matches"
		if len(matches) > 0 {
			index := state.activeMatch
			if index < 0 || index >= len(matches) {
				index = 0
			}
			count = fmt.Sprintf("%d of %d", index+1, len(matches))
		}
		ui.Text(c, count).FontSize(11).TextColor(oi.textSubtle)
		m.runActionIcon(c, p, oi, "find-prev", "general/chevronUp",
			"Previous Match", len(matches) > 0, false, func() {
				state.activeMatch = (state.activeMatch - 1 + len(matches)) % len(matches)
				state.list.ScrollTo(matches[state.activeMatch], ui.Center)
				m.invalidate()
			})
		m.runActionIcon(c, p, oi, "find-next", "general/chevronDown",
			"Next Match", len(matches) > 0, false, func() {
				state.activeMatch = (state.activeMatch + 1) % len(matches)
				state.list.ScrollTo(matches[state.activeMatch], ui.Center)
				m.invalidate()
			})
		m.runActionIcon(c, p, oi, "find-close", "general/close",
			"Close Find", true, false, func() {
				state.searching = false
				state.query = ""
				state.lastQuery = ""
				m.invalidate()
			})
	})
	state.lastMatches = matches
}

func runFindLines(lines []outputLine, query string) []int {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	lower := strings.ToLower(query)
	var matches []int
	for i, line := range lines {
		if strings.Contains(strings.ToLower(line.text), lower) {
			matches = append(matches, i)
		}
	}
	return matches
}

func (m *Model) runConsoleList(c *ui.Context, p *ui.Theme, state *runConsoleState, lines []outputLine, testOutput bool) {
	oi := oiPaletteOf(p)
	if len(lines) == 0 {
		ui.Box(c).Grow(1).MinHeight(0).Background(oi.island)
		return
	}
	state.list.FollowEnd = !state.noFollow && !state.searching && !testOutput
	ui.List(c.Key("run-console:"+state.key), &state.list, len(lines), func(i int) {
		if i < 0 || i >= len(lines) {
			return
		}
		line := lines[i]
		m.runConsoleLine(c, p, line, state.query, !state.noSoftWrap)
	}).Grow(1).MinWidth(0).MinHeight(0).Background(oi.island)
}

func (m *Model) runConsoleLine(c *ui.Context, p *ui.Theme, line outputLine, query string, wrap bool) {
	oi := oiPaletteOf(p)
	color := oi.text
	switch line.kind {
	case "stderr":
		color = oi.danger
	case "system", "console":
		color = oi.textSubtle
	case "input":
		color = oi.success
	}
	text := line.text
	if text == "" {
		text = " "
	}
	row := ui.Row(c).MinHeight(18).Padding(0, 12).AlignItems(ui.Start)
	if wrap {
		row.Wrap()
	} else {
		row.NoWrap()
	}
	row.Children(func() {
		for _, segment := range runLineSegments(text, traceLinks(text), query) {
			segment := segment
			t := ui.Text(c, segment.text).Font(oiFontMono).FontSize(12).
				TextColor(color)
			if !wrap {
				t.NoWrap()
			}
			if line.kind == "system" || line.kind == "console" {
				t.Italic()
			}
			if segment.match {
				t.Background(oi.accent.Mix(oi.island, 0.72))
			}
			if segment.link != nil {
				t.TextColor(oi.link).Underline().Cursor(ui.CursorPointer)
				link := *segment.link
				t.OnClick(func() { m.openTraceLink(link, c) })
			}
		}
	})
}

type runLineSegment struct {
	text  string
	match bool
	link  *traceLink
}

func runLineSegments(line string, links []traceLink, query string) []runLineSegment {
	if line == "" {
		return []runLineSegment{{text: " "}}
	}
	var out []runLineSegment
	appendPlain := func(text string, match bool) {
		if text == "" {
			return
		}
		parts := splitRunQuery(text, query)
		for _, part := range parts {
			if part.text != "" {
				out = append(out, runLineSegment{text: part.text, match: match || part.match})
			}
		}
	}
	pos := 0
	for i := range links {
		link := &links[i]
		if link.start < pos || link.start > len(line) || link.end < link.start {
			continue
		}
		appendPlain(line[pos:link.start], false)
		label := line[link.start:link.end]
		appendPlain(label, false)
		out[len(out)-1].link = link
		pos = link.end
	}
	appendPlain(line[pos:], false)
	return out
}

type runQueryPart struct {
	text  string
	match bool
}

func splitRunQuery(text, query string) []runQueryPart {
	query = strings.TrimSpace(query)
	if query == "" {
		return []runQueryPart{{text: text}}
	}
	lowerText, lowerQuery := strings.ToLower(text), strings.ToLower(query)
	var parts []runQueryPart
	for pos := 0; pos < len(text); {
		index := strings.Index(lowerText[pos:], lowerQuery)
		if index < 0 {
			parts = append(parts, runQueryPart{text: text[pos:]})
			break
		}
		start := pos + index
		if start > pos {
			parts = append(parts, runQueryPart{text: text[pos:start]})
		}
		end := start + len(query)
		parts = append(parts, runQueryPart{text: text[start:end], match: true})
		pos = end
	}
	return parts
}

func (m *Model) runSessionMoreMenu(menu *ui.Menu, session *runSessionView) {
	if session.live {
		if menu.Item("Stop").Chosen() {
			m.stopRunSession(session)
		}
	} else if menu.Item("Rerun").Chosen() {
		m.rerunRunSession(session)
	}
	if session.kind != runSessionAgent && session.config != nil {
		if menu.Item("Debug").Chosen() {
			m.debugRunSession(session)
		}
	}
	if session.tests != nil && session.kind == runSessionPrimary && !session.live &&
		len(adapter.FailedTestPaths(session.tests)) > 0 {
		if menu.Item("Rerun Failed Tests").Chosen() {
			m.RerunFailedTests()
		}
	}
	menu.Separator()
	if session.kind != runSessionAgent {
		if menu.Item("Find…").Chosen() {
			state := m.run.consoleState(session.key)
			state.searching = true
			m.invalidate()
		}
		if menu.Item("Clear All").Chosen() {
			m.clearRunSession(session)
		}
	}
	if session.kind != runSessionAgent && session.config != nil {
		menu.Separator()
		if menu.Item("Edit Configurations…").Chosen() {
			m.editRunSession(session)
		}
	}
	menu.Separator()
	if menu.Item("Choose Run/Debug Configuration…").Chosen() {
		m.RunAction("run.chooseConfig")
	}
	if menu.Item("Reload Run Configurations").Chosen() {
		m.RunAction("run.reloadConfigs")
	}
	menu.Separator()
	if menu.Item("Close Tab").Chosen() {
		m.closeRunSession(session)
	}
}

func (m *Model) rerunRunSession(session *runSessionView) {
	if session == nil {
		return
	}
	switch session.kind {
	case runSessionPrimary:
		m.Rerun()
	case runSessionAgent:
		if run := m.agentRunByID(session.agentID); run != nil {
			m.rerunActiveAgentRun()
		}
	case runSessionChild:
		child := m.childByChannel(session.channel)
		if child == nil {
			return
		}
		cfg, groupID := child.config, child.groupID
		m.closeRunChild(child.channel)
		go func() {
			ctx, cancel := context.WithTimeout(modelContext(m), 30*time.Minute)
			defer cancel()
			_, _ = m.startOwnedChildRun(ctx, &cfg, groupID, true, false)
		}()
	case runSessionService:
		if child := m.childByChannel(session.channel); child != nil {
			m.startServiceConfig(child.config.ID, true)
		}
	}
}

func (m *Model) debugRunSession(session *runSessionView) {
	if session == nil {
		return
	}
	switch session.kind {
	case runSessionChild, runSessionService:
		if child := m.childByChannel(session.channel); child != nil {
			m.DebugRunConfig(&child.config)
		}
	case runSessionPrimary:
		raw, _ := m.rerunTarget()
		if len(raw) == 0 {
			return
		}
		cfg, err := openidea.DecodeRunConfig(raw)
		if err == nil {
			m.DebugRunConfig(cfg)
		}
	}
}

func (m *Model) stopRunSession(session *runSessionView) {
	if session == nil || !session.live {
		return
	}
	switch session.kind {
	case runSessionPrimary:
		m.StopRun()
	case runSessionChild, runSessionService:
		if child := m.childByChannel(session.channel); child != nil {
			m.stopRunChild(child)
		}
	case runSessionAgent:
		m.stopActiveAgentRun()
	}
}

func (m *Model) updateRunSession(session *runSessionView) {
	if session == nil || !session.live {
		return
	}
	switch session.kind {
	case runSessionPrimary:
		m.UpdateApplication(nil)
	case runSessionChild, runSessionService:
		if session.runID != 0 {
			m.UpdateApplication(map[string]any{"run": "run:" + fmt.Sprint(session.runID)})
		}
	}
}

func (m *Model) editRunSession(session *runSessionView) {
	if session == nil || session.config == nil {
		m.RunAction("run.editConfigs")
		return
	}
	m.OpenRunConfigsFor(session.config.ID)
}

func (m *Model) clearRunSession(session *runSessionView) {
	if session == nil {
		return
	}
	switch session.kind {
	case runSessionPrimary:
		m.ClearRun()
	case runSessionChild, runSessionService:
		if child := m.childByChannel(session.channel); child != nil {
			child.output = nil
			child.truncated = false
			child.dropped = 0
			m.invalidate()
		}
	case runSessionAgent:
		m.clearActiveAgentRun()
	}
}

func (m *Model) closeRunSession(session *runSessionView) {
	if session == nil {
		return
	}
	switch session.kind {
	case runSessionPrimary:
		m.requestClosePrimaryRun()
	case runSessionChild:
		m.requestCloseRunChild(session.channel)
	case runSessionAgent:
		m.requestCloseAgentRun(session.agentID)
	case runSessionService:
		m.run.viewChild = nil
		m.run.visible = false
		m.DockHide("run")
	}
}
