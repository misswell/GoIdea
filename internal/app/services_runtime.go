package app

// Services owns independent run sessions. Each session is the same runChild
// used by Compound configurations, but its output is routed to Services and
// does not replace the Run window's primary process.

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/adapter"
	"mayoi/internal/openidea"
)

var serviceStartedPortPattern = regexp.MustCompile(
	`(?i)\b(?:tomcat|netty|undertow|jetty)(?:\s+web\s+server)?\s+started\s+on\s+port(?:\(s\)|s)?(?:\s*[:=]\s*|\s+)(\d{1,5})`,
)

func (p *servicesPane) contains(child *runChild) bool {
	for _, candidate := range p.children {
		if candidate == child {
			return true
		}
	}
	return false
}

func (m *Model) addServiceChild(child *runChild) {
	if child == nil || child.root != m.Root() {
		return
	}
	// Keep one current console per configuration. A finished session is
	// replaced by its rerun, while live independent sessions stay separate.
	for i, previous := range m.services.children {
		if previous != nil && previous.config.ID == child.config.ID && !previous.live() {
			m.services.children[i] = child
			m.invalidate()
			return
		}
	}
	m.services.children = append(m.services.children, child)
	m.invalidate()
}

func (m *Model) serviceChildForConfig(id string) *runChild {
	var latest *runChild
	for i := len(m.services.children) - 1; i >= 0; i-- {
		child := m.services.children[i]
		if child == nil || child.config.ID != id {
			continue
		}
		if latest == nil {
			latest = child
		}
		if child.live() {
			return child
		}
	}
	return latest
}

func (m *Model) syncServiceActivity() {
	if running := m.runningServiceCount(); running > 0 {
		m.status = fmt.Sprintf("%d services running", running)
	}
	m.invalidate()
}

func serviceExecutionDetail(child *runChild) string {
	if child == nil {
		return ""
	}
	if child.live() {
		if port := servicePort(child); port != 0 {
			return fmt.Sprintf(":%d", port)
		}
		if child.starting {
			return "Starting"
		}
		if child.stopped {
			return "Stopping"
		}
		return "Running"
	}
	if child.stopped {
		return "Stopped"
	}
	if child.err != "" {
		return "Failed"
	}
	if child.exitCode != nil && *child.exitCode != 0 {
		return fmt.Sprintf("Failed (exit %d)", *child.exitCode)
	}
	return "Finished"
}

func servicePort(child *runChild) int {
	if child == nil {
		return 0
	}
	port := 0
	for _, line := range child.output {
		matches := serviceStartedPortPattern.FindAllStringSubmatch(line.text, -1)
		for _, match := range matches {
			if len(match) != 2 {
				continue
			}
			value, err := strconv.Atoi(match[1])
			if err == nil && value > 0 && value < 65536 {
				port = value
			}
		}
	}
	return port
}

func (m *Model) debugServiceActive(configID string) bool {
	return configID != "" && m.debug.lastID == configID &&
		(m.debug.starting || (m.debug.session != 0 && !m.debug.ended))
}

func (m *Model) debugServiceHasOutput(configID string) bool {
	return configID != "" && m.debug.lastID == configID && len(m.debug.output) > 0
}

func (m *Model) runServiceRunningCount() int {
	m.ensureServicesProject()
	count := 0
	for _, kind := range m.services.types {
		for _, cfg := range m.serviceConfigsForType(kind) {
			if m.serviceConfigHidden(kind, cfg.ID) {
				continue
			}
			if child := m.serviceChildForConfig(cfg.ID); child != nil && child.live() {
				count++
				continue
			}
			if m.debugServiceActive(cfg.ID) {
				count++
			}
		}
	}
	return count
}

func (m *Model) runningServiceCount() int {
	if !m.services.visible {
		m.disposeServiceProviderWatches()
	}
	count := 0
	for _, provider := range registeredServiceProviders() {
		if provider.badge != nil {
			count += max(0, provider.badge(m))
		}
	}
	return count
}

func (m *Model) runningServiceCountIn(configs []openidea.RunConfig) int {
	seen := map[string]bool{}
	for _, cfg := range configs {
		if seen[cfg.ID] {
			continue
		}
		child := m.serviceChildForConfig(cfg.ID)
		if (child != nil && child.live()) || m.debugServiceActive(cfg.ID) {
			seen[cfg.ID] = true
		}
	}
	return len(seen)
}

func (m *Model) serviceVisibleConfigsForType(kind string) []openidea.RunConfig {
	configs := m.serviceConfigsForType(kind)
	visible := make([]openidea.RunConfig, 0, len(configs))
	for _, cfg := range configs {
		if !m.serviceConfigHidden(kind, cfg.ID) {
			visible = append(visible, cfg)
		}
	}
	return orderServiceConfigs(visible, m.services.order[kind])
}

func (m *Model) serviceScopeConfigs(node *serviceTreeNode) []openidea.RunConfig {
	if node == nil {
		return nil
	}
	switch node.nodeKind {
	case "config":
		if cfg := m.serviceConfigByID(node.configID); cfg != nil {
			return []openidea.RunConfig{*cfg}
		}
	case "type":
		return m.serviceVisibleConfigsForType(node.kind)
	case "folder":
		var out []openidea.RunConfig
		for _, cfg := range m.serviceVisibleConfigsForType(node.kind) {
			if serviceFolder(cfg) == node.folder {
				out = append(out, cfg)
			}
		}
		return out
	}
	return nil
}

func (m *Model) serviceScopeAllRunning(configs []openidea.RunConfig) bool {
	if len(configs) == 0 {
		return false
	}
	for _, cfg := range configs {
		child := m.serviceChildForConfig(cfg.ID)
		if !(child != nil && child.live()) && !m.debugServiceActive(cfg.ID) {
			return false
		}
	}
	return true
}

func (m *Model) serviceScopeAnyRunning(configs []openidea.RunConfig) bool {
	return m.runningServiceCountIn(configs) > 0
}

func (m *Model) serviceScopePorts(node *serviceTreeNode) map[string]string {
	urls := map[string]string{}
	for _, cfg := range m.serviceScopeConfigs(node) {
		child := m.serviceChildForConfig(cfg.ID)
		if child == nil || !child.live() {
			continue
		}
		if port := servicePort(child); port != 0 {
			urls[fmt.Sprintf("http://localhost:%d", port)] = cfg.Name
		}
	}
	return urls
}

func (m *Model) openServiceBrowser(urls map[string]string) {
	if len(urls) == 0 {
		return
	}
	keys := make([]string, 0, len(urls))
	for url := range urls {
		keys = append(keys, url)
	}
	sort.Strings(keys)
	openReleasePage(keys[0])
}

func (m *Model) startServiceConfig(configID string, restart bool) {
	m.ensureServicesProject()
	if m.Root() == "" {
		return
	}
	if !m.run.trusted {
		m.run.pendingTrust = true
		m.trustAsk = true
		m.trustFolder = false
		m.status = "The project is not trusted: accept the prompt before running"
		m.invalidate()
		return
	}
	if !m.run.loaded {
		m.loadRunConfigs()
		m.status = "Loading run configurations…"
		m.invalidate()
		return
	}
	cfg := m.serviceConfigByID(configID)
	if cfg == nil {
		return
	}
	if serviceDebugOnlyKind(cfg.Kind) {
		m.status = adapter.KindLabel(cfg.Kind) + " can only be started with Debug"
		m.invalidate()
		return
	}
	child := m.serviceChildForConfig(configID)
	if child != nil && child.live() {
		restart = true
	}
	m.runServiceConfigs([]openidea.RunConfig{*cfg}, restart)
}

func (m *Model) runServiceConfigs(configs []openidea.RunConfig, restart bool) {
	if m.ctx == nil || (m.mgr == nil && m.invokeHook == nil) {
		m.setServiceProviderError(runServiceProviderID, "Services needs a connected backend to run")
		return
	}
	m.ensureServicesProject()
	root := m.Root()
	if root == "" || !m.run.trusted || !m.run.loaded {
		return
	}

	copies := serviceRunnableConfigs(configs)
	var toStart []openidea.RunConfig
	var toStop []*runChild
	for _, cfg := range copies {
		child := m.serviceChildForConfig(cfg.ID)
		if child != nil && child.live() {
			if !restart {
				continue
			}
			toStop = append(toStop, child)
		}
		if m.services.starting[cfg.ID] {
			continue
		}
		toStart = append(toStart, cfg)
	}
	if len(toStart) == 0 {
		return
	}
	for _, child := range toStop {
		m.stopRunChild(child)
	}
	for _, cfg := range toStart {
		m.services.starting[cfg.ID] = true
	}
	m.clearServiceProviderError(runServiceProviderID)
	m.status = "Starting services…"
	m.invalidate()

	tabs := append([]*Tab(nil), m.tabs...)
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Minute)
		defer cancel()
		for _, child := range toStop {
			if err := waitServiceChild(ctx, child); err != nil {
				m.post(func() {
					m.setServiceProviderError(
						runServiceProviderID,
						"Stopping a service failed: "+err.Error(),
					)
				})
				return
			}
		}
		if err := m.saveOpenTabs(ctx, tabs); err != nil {
			m.post(func() {
				for _, cfg := range toStart {
					delete(m.services.starting, cfg.ID)
				}
				m.setServiceProviderError(
					runServiceProviderID,
					"Saving before running services failed: "+err.Error(),
				)
			})
			return
		}
		for _, cfg := range toStart {
			if m.Root() != root {
				return
			}
			cfg := cfg
			groupID := "service:" + cfg.ID
			if err := m.prepareBeforeLaunch(ctx, &cfg, []string{cfg.ID}, false, groupID); err != nil {
				m.post(func() {
					delete(m.services.starting, cfg.ID)
					m.setServiceProviderError(
						runServiceProviderID,
						"'"+cfg.Name+"' was not started: Before launch: "+err.Error(),
					)
				})
				continue
			}
			if _, err := m.startServiceChildRun(ctx, &cfg, groupID); err != nil {
				m.post(func() {
					delete(m.services.starting, cfg.ID)
					m.setServiceProviderError(
						runServiceProviderID,
						"Cannot run '"+cfg.Name+"': "+err.Error(),
					)
				})
				continue
			}
			m.post(func() {
				delete(m.services.starting, cfg.ID)
				m.status = "Running: " + cfg.Name
				m.invalidate()
			})
		}
	}()
}

func serviceRunnableConfigs(configs []openidea.RunConfig) []openidea.RunConfig {
	out := make([]openidea.RunConfig, 0, len(configs))
	for _, cfg := range configs {
		if !serviceDebugOnlyKind(cfg.Kind) {
			out = append(out, cfg)
		}
	}
	return out
}

func waitServiceChild(ctx context.Context, child *runChild) error {
	if child == nil || child.finished {
		return nil
	}
	if child.done == nil {
		return fmt.Errorf("the service has no completion signal")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-child.done:
		return err
	}
}

func (m *Model) stopServiceConfigs(configs []openidea.RunConfig) {
	for _, cfg := range configs {
		m.removeQueuedServiceDebug(cfg.ID)
		if child := m.serviceChildForConfig(cfg.ID); child != nil && child.live() {
			m.stopRunChild(child)
		}
		if m.debugServiceActive(cfg.ID) {
			m.StopDebug()
		}
	}
}

func (m *Model) debugServiceConfig(configID string) {
	cfg := m.serviceConfigByID(configID)
	if cfg == nil {
		return
	}
	if m.debugServiceActive(configID) {
		return
	}
	if m.debug.starting || (m.debug.session != 0 && !m.debug.ended) {
		if m.debug.lastID != configID && !slicesContains(m.services.debugQueue, configID) {
			m.services.debugQueue = append(m.services.debugQueue, configID)
			m.status = "Debug queued: " + cfg.Name
		}
		m.invalidate()
		return
	}
	m.startServiceDebugNow(configID)
}

func (m *Model) startServiceDebugNow(configID string) {
	cfg := m.serviceConfigByID(configID)
	if cfg == nil {
		return
	}
	m.selectOnlyServiceKey(serviceConfigKey(configID))
	m.debugRunConfig(cfg, false)
	if m.debug.err != "" {
		m.setServiceProviderError(runServiceProviderID, "Cannot debug '"+cfg.Name+"': "+m.debug.err)
	} else {
		m.clearServiceProviderError(runServiceProviderID)
	}
	m.invalidate()
}

func (m *Model) removeQueuedServiceDebug(configID string) {
	m.services.debugQueue = filterServiceSelection(m.services.debugQueue, func(candidate string) bool {
		return candidate != configID
	})
}

func (m *Model) advanceServiceDebugQueue() {
	if len(m.services.debugQueue) == 0 || m.debug.starting ||
		(m.debug.session != 0 && !m.debug.ended) {
		return
	}
	next := m.services.debugQueue[0]
	m.services.debugQueue = append([]string(nil), m.services.debugQueue[1:]...)
	m.startServiceDebugNow(next)
}

func (m *Model) showServiceInRun(child *runChild) {
	if child == nil {
		return
	}
	m.run.viewChild = child
	m.run.visible = true
	m.occupyBottom("run")
	m.invalidate()
}

func (m *Model) showServiceInDebug(configID string) {
	if m.debugServiceActive(configID) {
		m.DockShow("debug")
	}
}

func (m *Model) editServiceConfig(configID string) {
	for i := range m.run.configs {
		if m.run.configs[i].ID != configID {
			continue
		}
		m.selectRunConfig(m.run.configs[i].ID)
		m.OpenRunConfigs()
		return
	}
}

func (m *Model) runViewedServicePanel(c *ui.Context, p *ui.Theme) {
	child := m.run.viewChild
	if child == nil {
		return
	}
	ui.Row(c).Height(30).Shrink(0).Padding(0, 8).Gap(8).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
		ui.Text(c, "Run").FontSize(12).Bold().TextColor(p.Text)
		ui.Text(c, child.name).FontSize(12).TextColor(p.Text).
			SingleLine().Ellipsis("…").Grow(1)
		ui.Text(c, serviceExecutionDetail(child)).FontSize(12).
			TextColor(serviceStatusColor(p, child)).SingleLine()
		if child.live() {
			if ui.Button(c, "Rerun").Clicked() {
				m.startServiceConfig(child.config.ID, true)
			}
			if ui.Button(c, "Stop").Clicked() {
				m.stopRunChild(child)
			}
		} else if ui.Button(c, "Run").Clicked() {
			m.startServiceConfig(child.config.ID, false)
		}
		if ui.Button(c, "Show in Services").Clicked() {
			m.run.viewChild = nil
			m.selectOnlyServiceKey(serviceConfigKey(child.config.ID))
			m.ShowServices()
		}
	})
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).Padding(10, 12).Children(func() {
			if child.truncated {
				ui.Text(c, "(Output truncated: earlier lines were omitted)").
					FontSize(12).TextColor(p.TextMuted)
			}
			if len(child.output) == 0 {
				ui.Text(c, "No output yet.").FontSize(12).TextColor(p.TextMuted)
			} else {
				ui.Text(c, child.renderText()).FontSize(12).TextColor(p.Text)
			}
		})
	})
}

func serviceURLsFromPorts(ports map[string]string) []string {
	urls := make([]string, 0, len(ports))
	for url := range ports {
		urls = append(urls, url)
	}
	sort.Strings(urls)
	return urls
}

func servicePortLabel(url string) string {
	return strings.TrimPrefix(url, "http://")
}
