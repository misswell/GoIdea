package app

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/egoist/mygo/ui"
)

const perfMemoryWidgetID = "perf.memory"

type perfState struct {
	memoryOpen  bool
	overlayOpen bool

	sampledAt      time.Time
	metrics        runtime.MemStats
	backendPID     int
	backendRunning bool
	lastAction     string
}

func init() {
	registerAction(&Action{
		ID:          "perf.clearCaches",
		Title:       "Clear Caches",
		Category:    "Help",
		Description: "Release unused Go memory and refresh the local workspace caches.",
		Keywords:    "memory cache garbage collect gc performance",
		Run:         func(m *Model) { m.ClearPerformanceCaches() },
	})
	registerAction(&Action{
		ID:          "perf.showMemory",
		Title:       "Show Memory Indicator Details",
		Category:    "Help",
		Description: "Show application and backend memory details.",
		Keywords:    "memory metrics performance heap rss",
		Run: func(m *Model) {
			m.SetStatusBarWidgetVisible(perfMemoryWidgetID, true)
			m.OpenPerformanceMemory()
		},
	})
	registerAction(&Action{
		ID:          "perf.toggleMemoryIndicator",
		Title:       "Memory Indicator",
		Category:    "View",
		Description: "Show or hide the memory indicator in the status bar.",
		Keywords:    "memory status bar widget performance",
		Toggled:     func(m *Model) bool { return m.performanceMemoryVisible() },
		Run: func(m *Model) {
			m.SetStatusBarWidgetVisible(perfMemoryWidgetID, !m.performanceMemoryVisible())
		},
	})
	registerAction(&Action{
		ID:          "perf.toggleOverlay",
		Title:       "Toggle Performance Overlay (Dev)",
		Category:    "Help",
		Description: "Show native runtime, frame, IPC and long-task diagnostics.",
		Keywords:    "latency fps diagnostics performance overlay memory",
		Toggled:     func(m *Model) bool { return m.perf.overlayOpen },
		Run: func(m *Model) {
			m.perf.overlayOpen = !m.perf.overlayOpen
			if m.perf.overlayOpen {
				m.samplePerformance(true)
			}
			m.invalidate()
		},
	})

	registerStatusItem(&StatusItem{
		ID:              perfMemoryWidgetID,
		Title:           "Memory Indicator",
		HiddenByDefault: true,
		Side:            "right",
		Order:           100,
		Render:          func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) { m.performanceMemoryStatus(c, p, oi) },
	})

	registerOverlay(&Overlay{
		ID:       "perf.memory",
		Layer:    overlayLayerDialog,
		Priority: 284,
		Modal:    true,
		Open:     func(m *Model) bool { return m.perf.memoryOpen },
		Render: func(m *Model, c *ui.Context, p *ui.Theme) {
			m.performanceMemoryDialog(c, p)
		},
	})
	registerOverlay(&Overlay{
		ID:       "perf.overlay",
		Layer:    overlayLayerDialog,
		Priority: 285,
		Modal:    true,
		Open:     func(m *Model) bool { return m.perf.overlayOpen },
		Render: func(m *Model, c *ui.Context, p *ui.Theme) {
			m.performanceOverlay(c, p)
		},
	})
}

func (m *Model) performanceMemoryVisible() bool {
	if visible, ok := m.prefs.Appearance.StatusBarWidgets[perfMemoryWidgetID]; ok {
		return visible
	}
	return false
}

func (m *Model) OpenPerformanceMemory() {
	m.samplePerformance(true)
	m.perf.memoryOpen = true
	m.invalidate()
}

func (m *Model) ClearPerformanceCaches() {
	runtime.GC()
	debug.FreeOSMemory()
	m.samplePerformance(true)
	m.perf.lastAction = "Released unused Go heap memory and refreshed performance metrics."
	m.notifyInfo("perf.clearCaches", "Caches cleared", m.perf.lastAction)
	m.invalidate()
}

func (m *Model) samplePerformance(force bool) {
	now := time.Now()
	if !force && !m.perf.sampledAt.IsZero() && now.Sub(m.perf.sampledAt) < 2*time.Second {
		return
	}
	runtime.ReadMemStats(&m.perf.metrics)
	m.perf.sampledAt = now
	m.perf.backendPID = 0
	m.perf.backendRunning = false
	if m.mgr != nil {
		state := m.mgr.State()
		m.perf.backendRunning = state.Running
		if server := m.mgr.Server(); server != nil {
			m.perf.backendPID = server.PID()
		}
	}
}

func (m *Model) performanceMemoryStatus(c *ui.Context, p *ui.Theme, oi oiColors) {
	m.samplePerformance(false)
	tk := oiTokensFor(m, p)
	label := formatMemorySize(m.perf.metrics.HeapAlloc)
	button := ui.ButtonBase(c.Key("status:perf-memory")).
		Height(tk.StatusBarHeight-tk.SpaceL).
		Padding(0, tk.SpaceM).
		Radius(tk.RadiusS).
		AlignItems(ui.Center).
		Cursor(ui.CursorPointer)
	if button.Hovered() {
		button.Background(oi.toolbarHover)
	}
	button.Tooltip("Memory Indicator")
	button.Children(func() {
		ui.Text(c, "Memory "+label).
			FontSize(tk.FontSizeM).
			TextColor(oi.textMuted).
			SingleLine()
	})
	if button.Clicked() {
		m.OpenPerformanceMemory()
	}
}

func (m *Model) performanceMemoryDialog(c *ui.Context, p *ui.Theme) {
	if !m.perf.memoryOpen {
		return
	}
	m.samplePerformance(false)
	open := m.perf.memoryOpen
	dialog := ui.Modal(c.Key("perf-memory-dialog"), &open, func() {
		ui.Column(c).Width(500).Padding(18).Gap(9).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Memory Indicator").FontSize(14).Bold().TextColor(p.Text)
				ui.Spacer(c)
				ui.Text(c, m.perf.sampledAt.Format("15:04:05")).FontSize(12).TextColor(p.TextMuted)
			})
			m.performanceMetric(c, p, "Go heap allocated", formatMemorySize(m.perf.metrics.HeapAlloc))
			m.performanceMetric(c, p, "Go heap in use", formatMemorySize(m.perf.metrics.HeapInuse))
			m.performanceMetric(c, p, "Go heap idle", formatMemorySize(m.perf.metrics.HeapIdle))
			m.performanceMetric(c, p, "Go runtime system", formatMemorySize(m.perf.metrics.Sys))
			m.performanceMetric(c, p, "Stack in use", formatMemorySize(m.perf.metrics.StackInuse))
			m.performanceMetric(c, p, "Next GC target", formatMemorySize(m.perf.metrics.NextGC))
			m.performanceMetric(c, p, "Goroutines", fmt.Sprint(runtime.NumGoroutine()))
			m.performanceMetric(c, p, "GC cycles", fmt.Sprint(m.perf.metrics.NumGC))
			backend := "Not connected"
			if m.perf.backendRunning {
				backend = fmt.Sprintf("running, PID %d", m.perf.backendPID)
			}
			m.performanceMetric(c, p, "openidea-server", backend)
			if m.perf.lastAction != "" {
				ui.Text(c, m.perf.lastAction).FontSize(12).TextColor(p.TextMuted)
			}
			ui.Row(c).Children(func() {
				ui.Button(c, "Clear Caches").OnClick(m.ClearPerformanceCaches)
				ui.Spacer(c)
				ui.Button(c, "Refresh").OnClick(func() {
					m.samplePerformance(true)
					m.invalidate()
				})
				ui.Button(c, "Close").OnClick(func() { m.perf.memoryOpen = false })
			})
		})
	})
	if !open || dialog.Dismissed() {
		m.perf.memoryOpen = false
	}
}

func (m *Model) performanceOverlay(c *ui.Context, p *ui.Theme) {
	if !m.perf.overlayOpen {
		return
	}
	m.samplePerformance(false)
	open := m.perf.overlayOpen
	dialog := ui.Modal(c.Key("perf-overlay-dialog"), &open, func() {
		ui.Column(c).Width(430).Padding(14).Gap(6).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Performance (dev diagnostics)").FontSize(14).Bold().TextColor(p.Text)
				ui.Spacer(c)
				ui.Text(c, "GoIDEA backend").FontSize(12).TextColor(p.TextMuted)
			})
			m.performanceMetric(c, p, "Frame renderer", "MyGo native")
			m.performanceMetric(c, p, "Keystroke → paint", "Unavailable")
			m.performanceMetric(c, p, "IPC round trip", "Unavailable")
			m.performanceMetric(c, p, "Long tasks", "Unavailable")
			m.performanceMetric(c, p, "Go heap", formatMemorySize(m.perf.metrics.HeapAlloc))
			m.performanceMetric(c, p, "Go runtime", formatMemorySize(m.perf.metrics.Sys))
			m.performanceMetric(c, p, "Goroutines", fmt.Sprint(runtime.NumGoroutine()))
			m.performanceMetric(c, p, "Backend", map[bool]string{true: "connected", false: "disconnected"}[m.perf.backendRunning])
			ui.Text(c, "MyGo exposes no frame-present timing. app_metrics is desktop-only in the pinned backend.").
				FontSize(11).TextColor(p.TextMuted).Wrap()
			ui.Row(c).Children(func() {
				ui.Spacer(c)
				ui.Button(c, "Close").OnClick(func() { m.perf.overlayOpen = false })
			})
		})
	})
	if !open || dialog.Dismissed() {
		m.perf.overlayOpen = false
	}
}

func (m *Model) performanceMetric(c *ui.Context, p *ui.Theme, label, value string) {
	ui.Row(c).Height(23).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).FontSize(12).TextColor(p.TextMuted).SingleLine()
		ui.Spacer(c)
		ui.Text(c, value).FontSize(12).TextColor(p.Text).SingleLine()
	})
}

func formatMemorySize(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	value := float64(bytes)
	for _, suffix := range []string{"KB", "MB", "GB", "TB"} {
		value /= unit
		if value < unit || suffix == "TB" {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%.1f TB", value)
}
