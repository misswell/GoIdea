package app

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestPerformanceActionsStatusAndOverlaysAreRegistered(t *testing.T) {
	for _, id := range []string{
		"perf.clearCaches",
		"perf.showMemory",
		"perf.toggleMemoryIndicator",
		"perf.toggleOverlay",
	} {
		if _, ok := actionGet(id); !ok {
			t.Errorf("performance action %s is not registered", id)
		}
	}
	var memory *StatusItem
	for _, item := range statusItems.items {
		if item.ID == perfMemoryWidgetID {
			memory = item
			break
		}
	}
	if memory == nil || !memory.HiddenByDefault || memory.Title != "Memory Indicator" {
		t.Fatalf("memory status item = %+v", memory)
	}
	for _, id := range []string{"perf.memory", "perf.overlay"} {
		if overlays.byID[id] == nil {
			t.Errorf("performance overlay %s is not registered", id)
		}
	}
	for _, id := range []string{
		"perf.clearCaches",
		"perf.showMemory",
		"perf.toggleOverlay",
	} {
		if !perfMenuHasCommand(mainMenuGroups(), id) {
			t.Errorf("Help menu lacks %s", id)
		}
	}
}

func TestPerformanceMemoryDialogRendersMetricsOffscreen(t *testing.T) {
	m := &Model{}
	m.perf.memoryOpen = true
	m.perf.lastAction = "Released unused Go heap memory."
	m.samplePerformance(true)
	tester := ui.NewTester(func(c *ui.Context) {
		m.performanceMemoryDialog(c, c.Theme())
	}, 640, 520)
	tester.Frame()
	for _, text := range []string{
		"Memory Indicator",
		"Go heap allocated",
		"Go runtime system",
		"Goroutines",
		"openidea-server",
		"Clear Caches",
		"Refresh",
	} {
		if !tester.HasText(text) {
			t.Errorf("memory dialog lacks %q: %q", text, tester.Texts())
		}
	}
}

func TestPerformanceOverlayRendersUnavailableDiagnosticsOffscreen(t *testing.T) {
	m := &Model{}
	m.perf.overlayOpen = true
	m.samplePerformance(true)
	tester := ui.NewTester(func(c *ui.Context) {
		m.performanceOverlay(c, c.Theme())
	}, 720, 560)
	tester.Frame()
	for _, text := range []string{
		"Performance (dev diagnostics)",
		"Keystroke → paint",
		"IPC round trip",
		"Long tasks",
		"Unavailable",
		"app_metrics is desktop-only",
	} {
		if !tester.HasText(text) {
			t.Errorf("performance overlay lacks %q: %q", text, tester.Texts())
		}
	}
}

func TestPerformanceMemoryFormatting(t *testing.T) {
	for _, tc := range []struct {
		bytes uint64
		want  string
	}{
		{0, "0 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{5 * 1024 * 1024, "5.0 MB"},
	} {
		if got := formatMemorySize(tc.bytes); got != tc.want {
			t.Errorf("formatMemorySize(%d) = %q, want %q", tc.bytes, got, tc.want)
		}
	}
	if got := formatMemorySize(1024); strings.TrimSpace(got) == "" {
		t.Fatal("formatted memory is empty")
	}
}

func perfMenuHasCommand(entries []menuEntry, command string) bool {
	for _, entry := range entries {
		if entry.command == command || perfMenuHasCommand(entry.sub, command) {
			return true
		}
	}
	return false
}
