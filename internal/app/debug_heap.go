package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type debugHeapState struct {
	busy     bool
	request  uint32
	channel  uint32
	progress string
	info     *debugHeapSnapshot
	classes  []debugHeapClass
	filter   string
	list     ui.ListState
	err      string
}

type debugHeapSnapshot struct {
	ID      uint64           `json:"id"`
	Label   string           `json:"label"`
	Origin  debugHeapOrigin  `json:"origin"`
	Summary debugHeapSummary `json:"summary"`
}

type debugHeapOrigin struct {
	Kind string `json:"kind"`
	Path string `json:"path"`
}

type debugHeapSummary struct {
	Objects          uint64  `json:"objects"`
	ReachableObjects uint64  `json:"reachableObjects"`
	ReachableBytes   uint64  `json:"reachableBytes"`
	UnreachableBytes uint64  `json:"unreachableBytes"`
	Classes          uint64  `json:"classes"`
	AnalysisMillis   float64 `json:"analysisMs"`
}

type debugHeapClass struct {
	ClassIndex       uint64 `json:"classIndex"`
	Name             string `json:"name"`
	Count            uint64 `json:"count"`
	Shallow          uint64 `json:"shallow"`
	Retained         uint64 `json:"retained"`
	UnreachableCount uint64 `json:"unreachableCount"`
	Ambiguous        bool   `json:"ambiguous"`
}

type debugHeapCaptureResult struct {
	Status   string             `json:"status"`
	Snapshot *debugHeapSnapshot `json:"snapshot"`
	Reason   struct {
		Kind      string `json:"kind"`
		Needed    uint64 `json:"needed"`
		Available uint64 `json:"available"`
		Message   string `json:"message"`
		Seconds   uint64 `json:"seconds"`
		Reason    struct {
			Kind string `json:"kind"`
		} `json:"reason"`
	} `json:"reason"`
}

type debugHeapOpenResult struct {
	Status    string             `json:"status"`
	Snapshot  *debugHeapSnapshot `json:"snapshot"`
	Needed    uint64             `json:"needed"`
	Available uint64             `json:"available"`
}

type debugHeapProgress struct {
	Type  string `json:"type"`
	Phase string `json:"phase"`
	Done  uint64 `json:"done"`
	Total uint64 `json:"total"`
}

// DebugCaptureHeap captures a JVM heap dump for the active session and opens
// its native class table.
func (m *Model) DebugCaptureHeap() {
	d := &m.debug
	h := &d.heap
	if d.session == 0 || d.ended || d.starting {
		d.err = "Start and pause a JVM debug session before capturing a heap"
		m.invalidate()
		return
	}
	if h.busy {
		return
	}
	h.busy = true
	h.err = ""
	h.progress = "Dumping the JVM heap…"
	h.request = m.nextRequest()
	h.channel = m.nextChannel()
	d.view = debugViewHeap
	session, request, channel := d.session, h.request, h.channel
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Minute)
		defer cancel()
		raw, err := m.invoke(ctx, "debug_heap_capture", map[string]any{
			"session":    session,
			"request":    request,
			"onProgress": map[string]any{"$channel": channel},
		})
		var result debugHeapCaptureResult
		if err == nil {
			err = json.Unmarshal(raw, &result)
		}
		m.post(func() {
			if m.debug.heap.request != request {
				return
			}
			m.debug.heap.busy = false
			switch {
			case err != nil:
				m.debug.heap.err = debugHeapActionError(err)
			case result.Status == "ready" && result.Snapshot != nil:
				m.adoptDebugHeap(result.Snapshot)
			case result.Status == "unavailable":
				m.debug.heap.err = debugHeapUnavailable(result.Reason.Kind, result.Reason.Message)
			case result.Status == "cancelled":
				m.debug.heap.progress = "Heap capture cancelled"
			default:
				m.debug.heap.err = "The backend did not return a heap snapshot"
			}
			m.invalidate()
		})
	}()
}

// DebugOpenHeapDump asks for an .hprof file and analyzes it in the backend.
func (m *Model) DebugOpenHeapDump() {
	if m.debug.heap.busy || m.win == nil {
		return
	}
	h := &m.debug.heap
	h.busy = true
	h.err = ""
	h.request = m.nextRequest()
	h.channel = m.nextChannel()
	m.debug.view = debugViewHeap
	request, channel, parent := h.request, h.channel, m.win
	m.invalidate()
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: parent, Title: "Open Heap Dump", ButtonLabel: "Open",
			DefaultPath: m.Root(),
			Filters:     []mygo.FileFilter{{Name: "Heap dump", Extensions: []string{"hprof"}}},
		})
		if err != nil || len(paths) == 0 {
			m.post(func() {
				if m.debug.heap.request != request {
					return
				}
				m.debug.heap.busy = false
				if err != nil {
					m.debug.heap.err = err.Error()
				}
				m.invalidate()
			})
			return
		}
		m.post(func() {
			if m.debug.heap.request == request {
				m.debug.heap.progress = "Analyzing " + filepath.Base(paths[0]) + "…"
				m.invalidate()
			}
		})
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Minute)
		defer cancel()
		raw, invokeErr := m.invoke(ctx, "debug_heap_open", map[string]any{
			"path": paths[0], "request": request,
			"onProgress": map[string]any{"$channel": channel},
		})
		var result debugHeapOpenResult
		if invokeErr == nil {
			invokeErr = json.Unmarshal(raw, &result)
		}
		m.post(func() {
			if m.debug.heap.request != request {
				return
			}
			m.debug.heap.busy = false
			switch {
			case invokeErr != nil:
				m.debug.heap.err = debugHeapActionError(invokeErr)
			case result.Status == "ready" && result.Snapshot != nil:
				m.adoptDebugHeap(result.Snapshot)
			case result.Status == "notEnoughDiskSpace":
				m.debug.heap.err = fmt.Sprintf(
					"Not enough disk space for the heap index: %s needed, %s free",
					debugBytes(result.Needed), debugBytes(result.Available),
				)
			case result.Status == "cancelled":
				m.debug.heap.progress = "Heap analysis cancelled"
			}
			m.invalidate()
		})
	}()
}

func (m *Model) adoptDebugHeap(snapshot *debugHeapSnapshot) {
	info := *snapshot
	m.debug.heap.info = &info
	m.debug.heap.classes = nil
	m.debug.heap.progress = "Reading the heap classes…"
	m.debug.view = debugViewHeap
	request := m.debug.heap.request
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
		defer cancel()
		raw, err := m.invoke(ctx, "debug_heap_classes", map[string]any{"snapshot": info.ID})
		var classes []debugHeapClass
		if err == nil {
			err = json.Unmarshal(raw, &classes)
		}
		m.post(func() {
			if m.debug.heap.info == nil || m.debug.heap.info.ID != info.ID {
				return
			}
			m.debug.heap.classes = classes
			m.debug.heap.progress = ""
			if err != nil {
				m.debug.heap.err = "Could not read the heap classes: " + err.Error()
			}
			_ = request
			m.invalidate()
		})
	}()
}

// DebugSaveHeapDump writes the open snapshot to an .hprof file.
func (m *Model) DebugSaveHeapDump() {
	h := &m.debug.heap
	if h.info == nil || h.busy || m.win == nil {
		return
	}
	h.busy = true
	h.err = ""
	id, parent := h.info.ID, m.win
	name := h.info.Label
	if name == "" {
		name = fmt.Sprintf("heap-%d.hprof", id)
	}
	if !strings.HasSuffix(strings.ToLower(name), ".hprof") {
		name += ".hprof"
	}
	m.invalidate()
	go func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent: parent, Title: "Save Heap Dump As",
			DefaultPath: filepath.Join(m.Root(), name), CreateDirectories: true,
			Filters: []mygo.FileFilter{{Name: "Heap dump", Extensions: []string{"hprof"}}},
		})
		if err == nil && path != "" {
			ctx, cancel := context.WithTimeout(m.ctx, 5*time.Minute)
			defer cancel()
			_, err = m.invoke(ctx, "debug_heap_save", map[string]any{"snapshot": id, "path": path})
		}
		m.post(func() {
			if m.debug.heap.info == nil || m.debug.heap.info.ID != id {
				return
			}
			m.debug.heap.busy = false
			if err != nil {
				m.debug.heap.err = "Could not save the heap dump: " + err.Error()
			} else if path != "" {
				m.debug.status = "Saved heap dump to " + path
			}
			m.invalidate()
		})
	}()
}

// DebugCancelHeap cancels the dump currently being captured or opened.
func (m *Model) DebugCancelHeap() {
	h := &m.debug.heap
	if !h.busy || h.request == 0 {
		return
	}
	request := h.request
	h.progress = "Cancelling…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		_, _ = m.invoke(ctx, "cancel_request", map[string]any{"id": request})
	}()
}

func (m *Model) closeDebugHeap() {
	h := &m.debug.heap
	if h.busy {
		m.DebugCancelHeap()
	}
	id := uint64(0)
	if h.info != nil {
		id = h.info.ID
	}
	h.info = nil
	h.classes = nil
	h.err = ""
	h.progress = ""
	m.invalidate()
	if id != 0 {
		go func() {
			ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
			defer cancel()
			_, _ = m.invoke(ctx, "debug_heap_close", map[string]any{"snapshot": id})
		}()
	}
}

func (m *Model) acceptDebugHeapProgress(channel uint32, data json.RawMessage) {
	if channel == 0 || channel != m.debug.heap.channel {
		return
	}
	var progress debugHeapProgress
	if json.Unmarshal(data, &progress) != nil {
		return
	}
	switch progress.Type {
	case "dumping":
		m.debug.heap.progress = "Dumping the JVM heap…"
	case "analyzing":
		phase := strings.ReplaceAll(progress.Phase, "_", " ")
		if phase == "" {
			phase = "analyzing"
		}
		percent := float64(0)
		if progress.Total > 0 {
			percent = float64(progress.Done) * 100 / float64(progress.Total)
		}
		m.debug.heap.progress = fmt.Sprintf("Heap %s · %.0f%%", phase, percent)
	}
	m.invalidate()
}

func (m *Model) debugHeap(c *ui.Context, p *ui.Theme, oi oiColors) {
	h := &m.debug.heap
	ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
		ui.Row(c).Height(34).Shrink(0).Padding(0, 8).Gap(6).AlignItems(ui.Center).Children(func() {
			debugIcon(c, oi, "heap-capture", "memory/heap", "Capture Heap Snapshot",
				m.debug.session != 0 && !m.debug.ended && !m.debug.starting && !h.busy, false, m.DebugCaptureHeap)
			debugIcon(c, oi, "heap-open", "run/open", "Open Heap Dump…", !h.busy, false, m.DebugOpenHeapDump)
			debugIcon(c, oi, "heap-save", "run/save", "Save Heap Dump As…", h.info != nil && !h.busy, false, m.DebugSaveHeapDump)
			if h.busy {
				ui.Button(c, "Cancel").OnClick(m.DebugCancelHeap)
			}
			if h.info != nil {
				ui.Text(c, h.info.Label).Grow(1).MinWidth(0).FontSize(12).TextColor(p.Text)
				debugIcon(c, oi, "heap-close", "general/close", "Close Heap Snapshot", true, false, m.closeDebugHeap)
			} else {
				ui.Spacer(c)
			}
		})
		if h.err != "" {
			ui.Text(c, h.err).Padding(4, 9).FontSize(12).TextColor(p.Danger)
		}
		if h.progress != "" {
			ui.Text(c, h.progress).Padding(4, 9).FontSize(12).TextColor(p.TextMuted)
		}
		if h.info == nil {
			ui.Column(c).Grow(1).AlignItems(ui.Center).Justify(ui.Center).Gap(6).Children(func() {
				ui.Text(c, "Capture a live JVM heap or open an .hprof file.").FontSize(12).TextColor(p.TextMuted)
			})
			return
		}
		ui.Row(c).Shrink(0).Padding(4, 9).Gap(16).Children(func() {
			ui.Text(c, fmt.Sprintf(
				"%d objects · %s reachable · %d classes",
				h.info.Summary.Objects, debugBytes(h.info.Summary.ReachableBytes), h.info.Summary.Classes,
			)).FontSize(12).TextColor(p.TextMuted)
		})
		ui.TextInput(c.Key("debug-heap-filter"), &h.filter).Placeholder("Filter heap classes")
		classes := h.classes
		query := strings.ToLower(strings.TrimSpace(h.filter))
		if query != "" {
			classes = make([]debugHeapClass, 0, len(h.classes))
			for _, row := range h.classes {
				if strings.Contains(strings.ToLower(row.Name), query) {
					classes = append(classes, row)
				}
			}
		}
		if len(classes) == 0 {
			ui.Text(c, "No heap classes to show.").Padding(4, 9).FontSize(12).TextColor(p.TextMuted)
			return
		}
		ui.Row(c).Height(24).Padding(0, 9).Gap(8).Children(func() {
			ui.Text(c, "Class").Grow(1).FontSize(12).TextColor(p.TextMuted)
			ui.Text(c, "Instances").Width(90).FontSize(12).TextColor(p.TextMuted)
			ui.Text(c, "Shallow").Width(90).FontSize(12).TextColor(p.TextMuted)
			ui.Text(c, "Retained").Width(90).FontSize(12).TextColor(p.TextMuted)
		})
		ui.List(c.Key("debug-heap-classes"), &h.list, len(classes), func(i int) {
			row := classes[i]
			name := row.Name
			if row.Ambiguous {
				name += " · same name in several class loaders"
			}
			ui.Row(c.Key(fmt.Sprintf("heap-class:%d", row.ClassIndex))).Height(24).
				Padding(0, 9).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, name).Grow(1).MinWidth(0).FontSize(12).TextColor(p.Text).
					SingleLine().Ellipsis("…")
				ui.Text(c, fmt.Sprint(row.Count)).Width(90).FontSize(12).TextColor(p.Text)
				ui.Text(c, debugBytes(row.Shallow)).Width(90).FontSize(12).TextColor(p.Text)
				ui.Text(c, debugBytes(row.Retained)).Width(90).FontSize(12).TextColor(p.Text)
			})
		}).Grow(1).MinHeight(0)
	})
}

func debugHeapActionError(err error) string {
	if err == nil {
		return ""
	}
	if strings.Contains(strings.ToLower(err.Error()), "cancel") {
		return "Heap operation cancelled"
	}
	return err.Error()
}

func debugHeapUnavailable(kind, message string) string {
	switch kind {
	case "memory":
		return "JVM memory inspection is unavailable for this session"
	case "notEnoughDiskSpace":
		return "There is not enough disk space to dump the heap"
	case "dumpFailed":
		if message != "" {
			return "Heap dump failed: " + message
		}
		return "Heap dump failed"
	case "dumpTimedOut":
		return "The heap dump timed out"
	default:
		return "The JVM could not capture a heap snapshot"
	}
}

func debugBytes(value uint64) string {
	const unit = 1024
	if value < unit {
		return fmt.Sprintf("%d B", value)
	}
	div, exp := uint64(unit), 0
	for n := value / unit; n >= unit && exp < 5; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(value)/float64(div), "KMGTPE"[exp])
}
