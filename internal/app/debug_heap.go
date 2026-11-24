package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

type debugHeapState struct {
	busy       bool
	request    uint32
	channel    uint32
	progress   string
	snapshots  []debugHeapSnapshotEntry
	selected   uint64
	info       *debugHeapSnapshot
	classes    []debugHeapClass
	classesErr string
	filter     string
	list       ui.ListState
	err        string

	mode           string
	selectedClass  uint32
	objects        []debugHeapObjectRow
	objectsTotal   uint32
	objectsLoading bool
	objectsError   string
	objectsRequest uint64
	objectParent   uint32
	objectClass    uint32
	objectClassSet bool
	objectTitle    string
	objectList     ui.ListState
	selectedObject uint32

	path         *debugHeapRootPath
	pathLoading  bool
	pathError    string
	pathMissing  bool
	pathRequest  uint64
	pathSnapshot uint64
	pathObject   uint32
	pathList     ui.ListState

	compare        []debugHeapClassDiff
	compareBase    uint64
	compareLoading bool
	compareError   string
	compareRequest uint64
	compareCurrent uint64
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
	Status   string                 `json:"status"`
	Snapshot *debugHeapSnapshot     `json:"snapshot"`
	Reason   debugHeapCaptureReason `json:"reason"`
}

type debugHeapCaptureReason struct {
	Kind      string `json:"kind"`
	Needed    uint64 `json:"needed"`
	Available uint64 `json:"available"`
	Message   string `json:"message"`
	Seconds   uint64 `json:"seconds"`
	JavaMajor uint32 `json:"javaMajor"`
	Reason    struct {
		Kind      string `json:"kind"`
		JavaMajor uint32 `json:"javaMajor"`
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

func (m *Model) releaseDebugHeapChannels(owner uint32) {
	for channel, channelOwner := range m.debug.heapChannels {
		if owner == 0 || channelOwner == owner {
			delete(m.debug.heapChannels, channel)
		}
	}
}

func (m *Model) debugHeapOwner() uint32 {
	if m.debug.active != 0 {
		return m.debug.active
	}
	return m.debug.session
}

func (m *Model) updateDebugHeap(owner uint32, update func(*debugHeapState)) bool {
	if owner == 0 {
		update(&m.debug.heap)
		return true
	}
	return m.updateDebugSession(owner, func(state *debugSessionState) {
		update(&state.heap)
	})
}

func (m *Model) registerDebugHeapChannel(owner, channel uint32) {
	if channel == 0 {
		return
	}
	if m.debug.heapChannels == nil {
		m.debug.heapChannels = map[uint32]uint32{}
	}
	m.debug.heapChannels[channel] = owner
}

func (m *Model) releaseDebugHeapChannel(channel uint32) {
	delete(m.debug.heapChannels, channel)
}

// invalidateDebugHeapOperation detaches a capture or open request from its
// progress channel and returns whether the backend request should be
// cancelled. A late reply can no longer match either the request or channel.
func invalidateDebugHeapOperation(heap *debugHeapState) (uint32, bool) {
	request := heap.request
	cancel := heap.busy && request != 0
	heap.request = 0
	heap.channel = 0
	heap.busy = false
	heap.progress = ""
	return request, cancel
}

// endDebugHeapOperation releases progress ownership for one session and
// cancels only a capture or open that is still in flight.
func (m *Model) endDebugHeapOperation(owner uint32) {
	m.releaseDebugHeapChannels(owner)
	var request uint32
	var cancel bool
	if !m.updateDebugHeap(owner, func(heap *debugHeapState) {
		request, cancel = invalidateDebugHeapOperation(heap)
	}) && owner != 0 {
		if m.debug.active == owner || m.debug.session == owner {
			request, cancel = invalidateDebugHeapOperation(&m.debug.heap)
		}
	}
	if cancel {
		m.cancelDebugRequest(request)
	}
}

func (m *Model) cancelDebugRequest(request uint32) {
	if request == 0 {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		_, _ = m.invoke(ctx, "cancel_request", map[string]any{"id": request})
	}()
}

// DebugCaptureHeap captures a JVM heap dump for the active session and opens
// its native class table.
func (m *Model) DebugCaptureHeap() {
	d := &m.debug
	if d.session == 0 || d.ended || d.starting {
		d.err = "Start and pause a JVM debug session before capturing a heap"
		m.invalidate()
		return
	}
	owner := m.debugHeapOwner()
	if !m.debugSessionSupports(owner, func(caps openidea.DebugCapabilities) bool {
		return caps.JVMHeap
	}) {
		d.err = "This debugger does not support capturing a JVM heap"
		m.invalidate()
		return
	}
	h := &d.heap
	if h.busy {
		return
	}
	h.busy = true
	h.err = ""
	h.progress = "Dumping the JVM heap…"
	h.request = m.nextRequest()
	h.channel = m.nextChannel()
	m.registerDebugHeapChannel(owner, h.channel)
	if owner == d.active {
		d.view = debugViewHeap
	}
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
			m.releaseDebugHeapChannel(channel)
			var ready *debugHeapSnapshot
			m.updateDebugHeap(owner, func(heap *debugHeapState) {
				if heap.request != request || heap.channel != channel {
					return
				}
				heap.busy = false
				switch {
				case err != nil:
					heap.err = debugHeapActionError(err)
				case result.Status == "ready" && result.Snapshot != nil:
					ready = result.Snapshot
				case result.Status == "unavailable":
					heap.err = debugHeapUnavailable(result.Reason)
				case result.Status == "cancelled":
					heap.progress = "Heap capture cancelled"
				default:
					heap.err = "The backend did not return a heap snapshot"
				}
			})
			if ready != nil {
				m.adoptDebugHeap(owner, request, channel, ready)
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
	owner := m.debugHeapOwner()
	request := m.nextRequest()
	channel := m.nextChannel()
	m.updateDebugHeap(owner, func(heap *debugHeapState) {
		heap.busy = true
		heap.err = ""
		heap.request = request
		heap.channel = channel
	})
	m.registerDebugHeapChannel(owner, channel)
	if owner == m.debug.active {
		m.debug.view = debugViewHeap
	}
	parent := m.win
	m.invalidate()
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: parent, Title: "Open Heap Dump", ButtonLabel: "Open",
			DefaultPath: m.Root(),
			Filters:     []mygo.FileFilter{{Name: "Heap dump", Extensions: []string{"hprof"}}},
		})
		if err != nil || len(paths) == 0 {
			m.post(func() {
				m.releaseDebugHeapChannel(channel)
				m.updateDebugHeap(owner, func(heap *debugHeapState) {
					if heap.request != request || heap.channel != channel {
						return
					}
					heap.busy = false
					if err != nil {
						heap.err = err.Error()
					}
				})
				m.invalidate()
			})
			return
		}
		m.analyzeDebugHeapDump(owner, paths[0], request, channel)
	}()
}

// analyzeDebugHeapDump is the RPC and state transition behind Open Heap Dump.
// Keeping it separate from the native file dialog lets the action be tested
// without opening UI or touching the owner's files.
func (m *Model) analyzeDebugHeapDump(owner uint32, path string, request, channel uint32) {
	m.post(func() {
		m.updateDebugHeap(owner, func(heap *debugHeapState) {
			if heap.request == request && heap.channel == channel {
				heap.progress = "Analyzing " + filepath.Base(path) + "…"
			}
		})
		m.invalidate()
	})
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Minute)
		defer cancel()
		raw, invokeErr := m.invoke(ctx, "debug_heap_open", map[string]any{
			"path": path, "request": request,
			"onProgress": map[string]any{"$channel": channel},
		})
		var result debugHeapOpenResult
		if invokeErr == nil {
			invokeErr = json.Unmarshal(raw, &result)
		}
		m.post(func() {
			m.releaseDebugHeapChannel(channel)
			var ready *debugHeapSnapshot
			m.updateDebugHeap(owner, func(heap *debugHeapState) {
				if heap.request != request || heap.channel != channel {
					return
				}
				heap.busy = false
				switch {
				case invokeErr != nil:
					heap.err = debugHeapActionError(invokeErr)
				case result.Status == "ready" && result.Snapshot != nil:
					ready = result.Snapshot
				case result.Status == "notEnoughDiskSpace":
					heap.err = fmt.Sprintf(
						"Not enough disk space for the heap index: %s needed, %s free",
						debugBytes(result.Needed), debugBytes(result.Available),
					)
				case result.Status == "cancelled":
					heap.progress = "Heap analysis cancelled"
				}
			})
			if ready != nil {
				m.adoptDebugHeap(owner, request, channel, ready)
			}
			m.invalidate()
		})
	}()
}

func (m *Model) adoptDebugHeap(owner uint32, request, channel uint32, snapshot *debugHeapSnapshot) {
	info := *snapshot
	adopted := false
	var evicted []uint64
	m.updateDebugHeap(owner, func(heap *debugHeapState) {
		if heap.request != request || heap.channel != channel {
			return
		}
		evicted = debugHeapAdoptSnapshot(heap, info)
		heap.progress = "Reading the heap classes…"
		adopted = true
	})
	if !adopted {
		m.closeDebugHeapSnapshotIDs([]uint64{info.ID})
		return
	}
	m.closeDebugHeapSnapshotIDs(evicted)
	if owner == m.debug.active {
		m.debug.view = debugViewHeap
	}
	m.invalidate()
	m.loadDebugHeapClasses(owner, info.ID)
}

// DebugSaveHeapDump writes the open snapshot to an .hprof file.
func (m *Model) DebugSaveHeapDump() {
	h := &m.debug.heap
	if h.info == nil || h.busy || m.win == nil {
		return
	}
	owner := m.debugHeapOwner()
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
			err = m.saveDebugHeapDump(owner, id, path)
		}
		m.post(func() {
			m.updateDebugHeap(owner, func(heap *debugHeapState) {
				if heap.info == nil || heap.info.ID != id {
					return
				}
				heap.busy = false
				if err != nil {
					heap.err = "Could not save the heap dump: " + err.Error()
				}
			})
			if owner == m.debug.active && err == nil && path != "" {
				m.debug.status = "Saved heap dump to " + path
			}
			m.invalidate()
		})
	}()
}

// saveDebugHeapDump is the backend call behind Save Heap Dump As. The caller
// owns the busy/error transition around the native Save dialog.
func (m *Model) saveDebugHeapDump(_ uint32, id uint64, path string) error {
	ctx, cancel := context.WithTimeout(m.ctx, 5*time.Minute)
	defer cancel()
	_, err := m.invoke(ctx, "debug_heap_save", map[string]any{"snapshot": id, "path": path})
	return err
}

// DebugCancelHeap cancels the dump currently being captured or opened.
func (m *Model) DebugCancelHeap() {
	h := &m.debug.heap
	if !h.busy || h.request == 0 {
		return
	}
	owner := m.debugHeapOwner()
	request := h.request
	h.progress = "Cancelling…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		_, _ = m.invoke(ctx, "cancel_request", map[string]any{"id": request})
		m.post(func() {
			m.updateDebugHeap(owner, func(heap *debugHeapState) {
				if heap.request != request {
					return
				}
				heap.progress = "Heap operation cancelled"
				heap.busy = false
			})
			m.invalidate()
		})
	}()
}

func (m *Model) closeDebugHeap() {
	h := &m.debug.heap
	owner := m.debugHeapOwner()
	if h.busy {
		m.DebugCancelHeap()
	}
	id := uint64(0)
	if h.info != nil {
		id = h.info.ID
	}
	if id != 0 {
		m.updateDebugHeap(owner, func(heap *debugHeapState) {
			debugHeapRemoveSnapshot(heap, id)
		})
	}
	h.err = ""
	h.progress = ""
	m.releaseDebugHeapChannels(owner)
	m.invalidate()
	if id != 0 {
		m.closeDebugHeapSnapshotIDs([]uint64{id})
	}
}

func (m *Model) acceptDebugHeapProgress(channel uint32, data json.RawMessage) bool {
	if channel == 0 {
		return false
	}
	owner, owned := m.debug.heapChannels[channel]
	if !owned && channel == m.debug.heap.channel {
		owner, owned = m.debugHeapOwner(), true
	}
	if !owned {
		return false
	}
	var progress debugHeapProgress
	if json.Unmarshal(data, &progress) != nil {
		return true
	}
	updated := m.updateDebugHeap(owner, func(heap *debugHeapState) {
		if heap.channel != channel {
			return
		}
		switch progress.Type {
		case "dumping":
			heap.progress = "Dumping the JVM heap…"
		case "analyzing":
			phase := strings.ReplaceAll(progress.Phase, "_", " ")
			if phase == "" {
				phase = "analyzing"
			}
			percent := float64(0)
			if progress.Total > 0 {
				percent = float64(progress.Done) * 100 / float64(progress.Total)
			}
			heap.progress = fmt.Sprintf("Heap %s · %.0f%%", phase, percent)
		}
	})
	if !updated {
		return false
	}
	m.invalidate()
	return true
}

func (m *Model) debugHeap(c *ui.Context, p *ui.Theme, oi oiColors) {
	h := &m.debug.heap
	owner := m.debugHeapOwner()
	ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
		ui.Row(c).Height(34).Shrink(0).Padding(0, 8).Gap(6).AlignItems(ui.Center).Children(func() {
			debugIcon(c, oi, "heap-capture", "memory/heap", "Capture Heap Snapshot",
				m.debug.session != 0 && !m.debug.ended && !m.debug.starting && !h.busy &&
					m.debugSessionSupports(m.debug.session, func(caps openidea.DebugCapabilities) bool {
						return caps.JVMHeap
					}), false, m.DebugCaptureHeap)
			debugIcon(c, oi, "heap-open", "run/open", "Open Heap Dump…", !h.busy, false, m.DebugOpenHeapDump)
			debugIcon(c, oi, "heap-save", "run/save", "Save Heap Dump As…", h.info != nil && !h.busy, false, m.DebugSaveHeapDump)
			if h.busy {
				ui.Button(c, "Cancel").OnClick(m.DebugCancelHeap)
			}
			if h.info != nil {
				snapshots := append([]debugHeapSnapshotEntry(nil), h.snapshots...)
				snapshotButton := ui.Button(c, h.info.Label).Height(25).Disabled(len(snapshots) < 2)
				snapshotButton.Menu(func(menu *ui.Menu) {
					for i := len(snapshots) - 1; i >= 0; i-- {
						entry := snapshots[i]
						item := menu.Item(entry.info.Label)
						if entry.info.ID == h.info.ID {
							item.Disabled(true)
						} else if item.Chosen() {
							m.selectDebugHeapSnapshot(owner, entry.info.ID)
						}
					}
				})
				ui.Spacer(c)
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
		if h.classesErr != "" {
			ui.Text(c, h.classesErr).Padding(4, 9).FontSize(12).TextColor(p.Danger)
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
		ui.Row(c).Height(30).Shrink(0).Padding(0, 9).Gap(6).AlignItems(ui.Center).Children(func() {
			modeButton := func(id, label string) {
				b := ui.ButtonBase(c.Key("debug-heap-view:"+id)).Height(24).Padding(0, 8).Radius(4)
				if h.mode == id || h.mode == "" && id == "classes" {
					b.Background(oi.selected)
				} else if b.Hovered() {
					b.Background(oi.toolbarHover)
				}
				b.Children(func() { ui.Text(c, label).FontSize(12).TextColor(p.Text) })
				if b.Clicked() {
					m.updateDebugHeap(owner, func(state *debugHeapState) { state.mode = id })
					if id == "dominators" {
						m.debugHeapShowAllObjects()
					}
					m.invalidate()
				}
			}
			modeButton("classes", "Classes")
			modeButton("dominators", "Dominators")
			if h.mode == "dominators" && (h.objectClassSet || h.objectParent != 0 || len(h.objects) > 0) {
				ui.Button(c, "Show All Dominators").Height(24).OnClick(m.debugHeapShowAllObjects)
			}
			ui.Spacer(c)
			candidates := m.debugHeapCompareCandidates(owner, h.info.ID)
			compareLabel := "Compare with…"
			if h.compareBase != 0 {
				compareLabel = "Compared with #" + fmt.Sprint(h.compareBase)
			}
			compare := ui.Button(c, compareLabel).Height(24).Disabled(len(candidates) == 0 && h.compareBase == 0)
			compare.Menu(func(menu *ui.Menu) {
				if len(candidates) == 0 {
					menu.Item("No other heap snapshot").Disabled(true)
				}
				for _, candidate := range candidates {
					if menu.Item(candidate.label).Chosen() {
						m.loadDebugHeapCompare(candidate.id)
					}
				}
				if h.compareBase != 0 {
					menu.Separator()
					if menu.Item("Do Not Compare").Chosen() {
						m.loadDebugHeapCompare(0)
					}
				}
			})
		})
		if h.compareLoading {
			ui.Text(c, "Comparing heap snapshots…").Padding(4, 9).FontSize(12).TextColor(p.TextMuted)
		}
		if h.compareError != "" {
			ui.Text(c, h.compareError).Padding(4, 9).FontSize(12).TextColor(p.Danger)
		}
		if h.mode == "dominators" {
			m.debugHeapObjects(c, p, oi, h)
			return
		}
		ui.TextInput(c.Key("debug-heap-filter"), &h.filter).Placeholder("Filter heap classes")
		classes := append([]debugHeapClass(nil), h.classes...)
		query := strings.ToLower(strings.TrimSpace(h.filter))
		if query != "" {
			filtered := classes[:0]
			for _, row := range classes {
				if strings.Contains(strings.ToLower(row.Name), query) {
					filtered = append(filtered, row)
				}
			}
			classes = filtered
		}
		sort.SliceStable(classes, func(i, j int) bool {
			if classes[i].Retained == classes[j].Retained {
				return classes[i].Name < classes[j].Name
			}
			return classes[i].Retained > classes[j].Retained
		})
		if len(classes) == 0 {
			ui.Text(c, "No heap classes to show.").Padding(4, 9).FontSize(12).TextColor(p.TextMuted)
			return
		}
		changes := debugHeapChangesByClass(h.compare)
		ui.Row(c).Height(24).Padding(0, 9).Gap(8).Children(func() {
			ui.Text(c, "Class").Grow(1).FontSize(12).TextColor(p.TextMuted)
			ui.Text(c, "Instances").Width(80).FontSize(12).TextColor(p.TextMuted)
			ui.Text(c, "Shallow").Width(80).FontSize(12).TextColor(p.TextMuted)
			ui.Text(c, "Retained").Width(80).FontSize(12).TextColor(p.TextMuted)
			if h.compareBase != 0 {
				ui.Text(c, "Count Δ").Width(70).FontSize(12).TextColor(p.TextMuted)
				ui.Text(c, "Retained Δ").Width(95).FontSize(12).TextColor(p.TextMuted)
			}
		})
		ui.List(c.Key("debug-heap-classes"), &h.list, len(classes), func(i int) {
			row := classes[i]
			name := row.Name
			if row.Ambiguous {
				name += " · same name in several class loaders"
			}
			item := ui.Row(c.Key(fmt.Sprintf("heap-class:%d", row.ClassIndex))).Height(24).
				Padding(0, 9).Gap(8).AlignItems(ui.Center)
			if row.ClassIndex == uint64(h.selectedClass) {
				item.Background(oi.selected)
			} else if item.Hovered() {
				item.Background(oi.toolbarHover)
			}
			item.Children(func() {
				ui.Text(c, name).Grow(1).MinWidth(0).FontSize(12).TextColor(p.Text).
					SingleLine().Ellipsis("…")
				ui.Text(c, fmt.Sprint(row.Count)).Width(80).FontSize(12).TextColor(p.Text)
				ui.Text(c, debugBytes(row.Shallow)).Width(80).FontSize(12).TextColor(p.Text)
				ui.Text(c, debugBytes(row.Retained)).Width(80).FontSize(12).TextColor(p.Text)
				if h.compareBase != 0 {
					if diff, found := changes[row.Name]; found {
						ui.Text(c, debugHeapClassDiffText(diff, "count")).Width(70).
							FontSize(12).TextColor(p.TextMuted)
						ui.Text(c, debugHeapClassDiffText(diff, "retained")).Width(95).
							FontSize(12).TextColor(p.TextMuted)
					} else {
						ui.Text(c, "—").Width(70).FontSize(12).TextColor(p.TextMuted)
						ui.Text(c, "—").Width(95).FontSize(12).TextColor(p.TextMuted)
					}
				}
			})
			if item.Clicked() {
				m.debugHeapShowClassObjects(uint32(row.ClassIndex), row.Name)
			}
		}).Grow(1).MinHeight(0)
	})
}

type debugHeapCompareCandidate struct {
	id    uint64
	label string
}

func (m *Model) debugHeapCompareCandidates(owner uint32, current uint64) []debugHeapCompareCandidate {
	var out []debugHeapCompareCandidate
	appendSession := func(session uint32) {
		state := m.debug.sessions[session]
		if state == nil {
			return
		}
		for _, entry := range state.heap.snapshots {
			if entry.info.ID == current {
				continue
			}
			name := state.tabName
			if name == "" {
				name = "Session " + fmt.Sprint(session)
			}
			out = append(out, debugHeapCompareCandidate{
				id: entry.info.ID, label: name + " · " + entry.info.Label,
			})
		}
	}
	for _, session := range m.debug.sessionOrder {
		appendSession(session)
	}
	if owner == 0 {
		for _, entry := range m.debug.heap.snapshots {
			if entry.info.ID != current {
				out = append(out, debugHeapCompareCandidate{
					id: entry.info.ID, label: entry.info.Label,
				})
			}
		}
	}
	return out
}

func (m *Model) debugHeapObjects(c *ui.Context, p *ui.Theme, oi oiColors, h *debugHeapState) {
	ui.Row(c).Height(28).Shrink(0).Padding(0, 9).Gap(6).AlignItems(ui.Center).Children(func() {
		title := h.objectTitle
		if title == "" {
			title = "Dominators"
		}
		ui.Text(c, title).Grow(1).MinWidth(0).FontSize(12).TextColor(p.Text).
			SingleLine().Ellipsis("…")
		if h.objectsTotal > uint32(len(h.objects)) && len(h.objects) < debugHeapMaxLoadedObjects {
			ui.Button(c, "Show More").Height(24).Disabled(h.objectsLoading).
				OnClick(m.debugHeapShowMoreObjects)
		}
	})
	if h.objectsLoading {
		ui.Text(c, "Reading heap objects…").Padding(4, 9).FontSize(12).TextColor(p.TextMuted)
	}
	if h.objectsError != "" {
		ui.Text(c, h.objectsError).Padding(4, 9).FontSize(12).TextColor(p.Danger)
	}
	if len(h.objects) == 0 && !h.objectsLoading && h.objectsError == "" {
		ui.Text(c, "Nothing to show.").Padding(4, 9).FontSize(12).TextColor(p.TextMuted)
	}
	ui.Row(c).Height(24).Padding(0, 9).Gap(8).Children(func() {
		ui.Text(c, "Object").Grow(1).FontSize(12).TextColor(p.TextMuted)
		ui.Text(c, "Shallow").Width(85).FontSize(12).TextColor(p.TextMuted)
		ui.Text(c, "Retained").Width(90).FontSize(12).TextColor(p.TextMuted)
		ui.Text(c, "%").Width(55).FontSize(12).TextColor(p.TextMuted)
	})
	ui.List(c.Key("debug-heap-objects"), &h.objectList, len(h.objects), func(i int) {
		row := h.objects[i]
		item := ui.Row(c.Key(fmt.Sprintf("heap-object:%d", row.Object))).Height(25).
			Padding(0, 9).Gap(8).AlignItems(ui.Center)
		if row.Object == h.selectedObject {
			item.Background(oi.selected)
		} else if item.Hovered() {
			item.Background(oi.toolbarHover)
		}
		item.Children(func() {
			if row.HasChildren {
				debugIcon(c, oi, fmt.Sprintf("heap-object-children:%d", row.Object),
					"toolwindows/hierarchy", "Open dominated objects", true, false, func() {
						m.debugHeapShowObjectChildren(row)
					})
			}
			ui.Text(c, debugHeapObjectText(row)).Grow(1).MinWidth(0).FontSize(12).
				TextColor(p.Text).SingleLine().Ellipsis("…")
			ui.Text(c, debugBytes(row.Shallow)).Width(85).FontSize(12).TextColor(p.Text)
			ui.Text(c, debugBytes(row.Retained)).Width(90).FontSize(12).TextColor(p.Text)
			share := "—"
			if h.info.Summary.ReachableBytes > 0 {
				share = fmt.Sprintf("%.1f%%", 100*float64(row.Retained)/
					float64(h.info.Summary.ReachableBytes))
			}
			ui.Text(c, share).Width(55).FontSize(12).TextColor(p.Text)
		})
		if item.Clicked() {
			m.loadDebugHeapPath(row)
		}
	}).Height(190).Shrink(0)
	m.debugHeapPath(c, p, h)
}

func (m *Model) debugHeapPath(c *ui.Context, p *ui.Theme, h *debugHeapState) {
	if h.pathLoading {
		ui.Text(c, "Finding the path to a GC root…").Padding(4, 9).
			FontSize(12).TextColor(p.TextMuted)
		return
	}
	if h.pathError != "" {
		ui.Text(c, h.pathError).Padding(4, 9).FontSize(12).TextColor(p.Danger)
		return
	}
	if h.pathMissing {
		ui.Text(c, "This object is not reachable from a GC root.").Padding(4, 9).
			FontSize(12).TextColor(p.TextMuted)
		return
	}
	rows := debugHeapPathRows(h.path)
	if len(rows) == 0 {
		return
	}
	ui.Text(c, "Path to GC Root").Padding(4, 9).FontSize(12).TextColor(p.TextMuted)
	ui.List(c.Key("debug-heap-root-path"), &h.pathList, len(rows), func(i int) {
		row := rows[i]
		if row.omitted > 0 {
			ui.Text(c, fmt.Sprintf("Path truncated: %d steps not shown", row.omitted)).
				Height(22).Padding(0, 9).FontSize(12).TextColor(p.TextMuted)
			return
		}
		ui.Row(c.Key(fmt.Sprintf("heap-path:%d:%d", i, row.step.Object.Object))).Height(36).
			Padding(0, 9).Gap(8).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				ui.Text(c, debugHeapObjectText(row.step.Object)).FontSize(12).TextColor(p.Text).
					SingleLine().Ellipsis("…")
				ui.Text(c, debugHeapEdgeText(row.step.Via)).FontSize(11).TextColor(p.TextMuted).
					SingleLine().Ellipsis("…")
			})
		})
	}).Grow(1).MinHeight(80)
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

func debugHeapUnavailable(reason debugHeapCaptureReason) string {
	switch reason.Kind {
	case "memory":
		switch reason.Reason.Kind {
		case "attachedJvmNotFound":
			return "Heap capture cannot find this JVM's attach listener. The debugged program must allow attach requests."
		case "listenModeNotSupported":
			return "Heap capture is not available when GoIDEA listens for the JVM to connect."
		case "unsupportedPlatform":
			return "Heap capture is not available on this platform yet."
		case "noJvmProcess":
			return "Could not find the debugged program's JVM process."
		case "listenerNotRunning":
			return "The JVM does not accept attach requests. Restart the debug session without -XX:+DisableAttachMechanism."
		case "unsafeWhileSuspended":
			version := reason.Reason.JavaMajor
			if version == 0 {
				version = reason.JavaMajor
			}
			if version > 0 {
				return fmt.Sprintf(
					"JDK %d cannot capture a heap while suspended. Resume the program, then capture the heap.",
					version,
				)
			}
			return "This JDK cannot capture a heap while suspended. Resume the program, then capture the heap."
		case "programExited":
			return "The program exited before the heap capture completed."
		case "timedOut":
			return "The JVM did not answer the heap capture request within 10 seconds."
		default:
			return "Heap capture is unavailable for this session."
		}
	case "notEnoughDiskSpace":
		return fmt.Sprintf(
			"Not enough disk space for the heap snapshot: %s needed, %s free",
			debugBytes(reason.Needed),
			debugBytes(reason.Available),
		)
	case "dumpFailed":
		if reason.Message != "" {
			return "Heap dump failed: " + reason.Message
		}
		return "Heap dump failed"
	case "dumpTimedOut":
		if reason.Seconds > 0 {
			return fmt.Sprintf("The heap dump timed out after %d seconds", reason.Seconds)
		}
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
