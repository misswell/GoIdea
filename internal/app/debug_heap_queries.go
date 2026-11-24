package app

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

const (
	debugHeapMaxSnapshots     = 8
	debugHeapPageSize         = 100
	debugHeapMaxLoadedObjects = 500
	debugHeapClassObjectLimit = 100
)

type debugHeapSnapshotEntry struct {
	info       debugHeapSnapshot
	classes    []debugHeapClass
	classesErr string
}

type debugHeapObjectRow struct {
	Object      uint32  `json:"object"`
	ClassName   string  `json:"className"`
	Address     string  `json:"address"`
	Shallow     uint64  `json:"shallow"`
	Retained    uint64  `json:"retained"`
	HasChildren bool    `json:"hasChildren"`
	Preview     *string `json:"preview"`
}

type debugHeapObjectPage struct {
	Rows  []debugHeapObjectRow `json:"rows"`
	Total uint32               `json:"total"`
}

type debugHeapEdge struct {
	Kind   string  `json:"kind"`
	Name   string  `json:"name"`
	Index  uint32  `json:"index"`
	Root   string  `json:"root"`
	Thread *string `json:"thread"`
}

type debugHeapPathStep struct {
	Object debugHeapObjectRow `json:"object"`
	Via    debugHeapEdge      `json:"via"`
}

type debugHeapRootPath struct {
	Steps        []debugHeapPathStep `json:"steps"`
	Omitted      uint32              `json:"omitted"`
	OmittedAfter uint32              `json:"omittedAfter"`
}

type debugHeapClassDiff struct {
	Name         string `json:"name"`
	BaseCount    uint64 `json:"baseCount"`
	Count        uint64 `json:"count"`
	BaseShallow  uint64 `json:"baseShallow"`
	Shallow      uint64 `json:"shallow"`
	BaseRetained uint64 `json:"baseRetained"`
	Retained     uint64 `json:"retained"`
	Ambiguous    bool   `json:"ambiguous"`
}

func (h *debugHeapState) selectedSnapshot() *debugHeapSnapshotEntry {
	for i := range h.snapshots {
		if h.snapshots[i].info.ID == h.selected {
			return &h.snapshots[i]
		}
	}
	return nil
}

func (h *debugHeapState) findSnapshot(id uint64) *debugHeapSnapshotEntry {
	for i := range h.snapshots {
		if h.snapshots[i].info.ID == id {
			return &h.snapshots[i]
		}
	}
	return nil
}

func (h *debugHeapState) syncSelectedSnapshot() {
	entry := h.selectedSnapshot()
	if entry == nil {
		h.selected = 0
		h.info = nil
		h.classes = nil
		h.classesErr = ""
		h.clearSelectedQueries()
		return
	}
	info := entry.info
	h.info = &info
	h.classes = entry.classes
	h.classesErr = entry.classesErr
}

func (h *debugHeapState) clearSelectedQueries() {
	h.mode = "classes"
	h.selectedClass = 0
	h.objects = nil
	h.objectsTotal = 0
	h.objectsLoading = false
	h.objectsError = ""
	h.objectsRequest++
	h.objectParent = 0
	h.objectClass = 0
	h.objectClassSet = false
	h.objectTitle = ""
	h.path = nil
	h.pathLoading = false
	h.pathError = ""
	h.pathMissing = false
	h.pathRequest++
	h.pathSnapshot = 0
	h.pathObject = 0
	h.compare = nil
	h.compareBase = 0
	h.compareLoading = false
	h.compareError = ""
	h.compareRequest++
	h.compareCurrent = 0
}

func debugHeapAdoptSnapshot(h *debugHeapState, info debugHeapSnapshot) []uint64 {
	for i := range h.snapshots {
		if h.snapshots[i].info.ID == info.ID {
			h.snapshots[i] = debugHeapSnapshotEntry{info: info}
			h.selected = info.ID
			h.syncSelectedSnapshot()
			return nil
		}
	}
	h.snapshots = append(h.snapshots, debugHeapSnapshotEntry{info: info})
	var evicted []uint64
	for len(h.snapshots) > debugHeapMaxSnapshots {
		id := h.snapshots[0].info.ID
		evicted = append(evicted, id)
		if h.compareBase == id {
			h.compare = nil
			h.compareBase = 0
			h.compareError = ""
			h.compareLoading = false
			h.compareRequest++
		}
		h.snapshots = h.snapshots[1:]
	}
	h.selected = info.ID
	h.syncSelectedSnapshot()
	return evicted
}

func debugHeapRemoveSnapshot(h *debugHeapState, id uint64) {
	for i := range h.snapshots {
		if h.snapshots[i].info.ID != id {
			continue
		}
		h.snapshots = append(h.snapshots[:i], h.snapshots[i+1:]...)
		if h.compareBase == id {
			h.compare = nil
			h.compareBase = 0
			h.compareError = ""
			h.compareLoading = false
			h.compareRequest++
		}
		if h.selected == id {
			h.selected = 0
			if len(h.snapshots) > 0 {
				h.selected = h.snapshots[len(h.snapshots)-1].info.ID
			}
		}
		h.syncSelectedSnapshot()
		return
	}
}

func (m *Model) selectDebugHeapSnapshot(owner uint32, id uint64) {
	m.updateDebugHeap(owner, func(h *debugHeapState) {
		if h.findSnapshot(id) == nil {
			return
		}
		h.selected = id
		h.syncSelectedSnapshot()
	})
	m.invalidate()
	if state := m.debugSessionState(owner); state != nil {
		if entry := state.heap.selectedSnapshot(); entry != nil && len(entry.classes) == 0 && entry.classesErr == "" {
			m.loadDebugHeapClasses(owner, id)
		}
	} else {
		h := &m.debug.heap
		if entry := h.selectedSnapshot(); entry != nil && len(entry.classes) == 0 && entry.classesErr == "" {
			m.loadDebugHeapClasses(owner, id)
		}
	}
}

func (m *Model) loadDebugHeapClasses(owner uint32, snapshot uint64) {
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.debugHeapContext(), 2*time.Minute)
		defer cancel()
		raw, err := m.invoke(ctx, "debug_heap_classes", map[string]any{"snapshot": snapshot})
		var classes []debugHeapClass
		if err == nil {
			err = json.Unmarshal(raw, &classes)
		}
		m.post(func() {
			m.updateDebugHeap(owner, func(h *debugHeapState) {
				entry := h.findSnapshot(snapshot)
				if entry == nil {
					return
				}
				entry.classes = classes
				entry.classesErr = ""
				if err != nil {
					entry.classes = nil
					entry.classesErr = "Could not read the heap classes: " + err.Error()
				}
				if h.selected == snapshot {
					h.syncSelectedSnapshot()
					h.progress = ""
				}
			})
			m.invalidate()
		})
	}()
}

func (m *Model) loadDebugHeapObjects(
	owner uint32,
	snapshot uint64,
	parent uint32,
	classIndex uint32,
	classSet bool,
	offset uint32,
	title string,
) {
	var request uint64
	ok := m.updateDebugHeap(owner, func(h *debugHeapState) {
		if h.selected != snapshot || h.info == nil || h.info.ID != snapshot {
			return
		}
		h.objectsRequest++
		request = h.objectsRequest
		h.objectsLoading = true
		h.objectsError = ""
		h.objectParent = parent
		h.objectClass = classIndex
		h.objectClassSet = classSet
		h.objectTitle = title
		if offset == 0 {
			h.objects = nil
			h.objectsTotal = 0
		}
	})
	if !ok || request == 0 {
		return
	}
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.debugHeapContext(), 2*time.Minute)
		defer cancel()
		var parentArg, classArg any
		if parent != 0 {
			parentArg = parent
		}
		if classSet {
			classArg = classIndex
		}
		raw, err := m.invoke(ctx, "debug_heap_dominators", map[string]any{
			"snapshot":   snapshot,
			"parent":     parentArg,
			"classIndex": classArg,
			"offset":     offset,
			"limit":      debugHeapPageSize,
		})
		var page debugHeapObjectPage
		if err == nil {
			err = json.Unmarshal(raw, &page)
		}
		m.post(func() {
			m.updateDebugHeap(owner, func(h *debugHeapState) {
				if h.selected != snapshot || h.objectsRequest != request {
					return
				}
				h.objectsLoading = false
				if err != nil {
					h.objectsError = "Could not read heap objects: " + err.Error()
					return
				}
				if offset == 0 {
					h.objects = append([]debugHeapObjectRow(nil), page.Rows...)
				} else {
					h.objects = append(h.objects, page.Rows...)
				}
				if len(h.objects) > debugHeapMaxLoadedObjects {
					h.objects = h.objects[:debugHeapMaxLoadedObjects]
				}
				h.objectsTotal = page.Total
				if page.Total < uint32(len(h.objects)) {
					h.objectsTotal = uint32(len(h.objects))
				}
			})
			m.invalidate()
		})
	}()
}

func (m *Model) debugHeapShowClassObjects(classIndex uint32, className string) {
	h := &m.debug.heap
	if h.info == nil {
		return
	}
	owner := m.debugHeapOwner()
	m.updateDebugHeap(owner, func(state *debugHeapState) {
		state.mode = "dominators"
		state.selectedClass = classIndex
		state.selectedObject = 0
		state.path = nil
		state.pathMissing = false
		state.pathError = ""
	})
	m.loadDebugHeapObjects(owner, h.info.ID, 0, classIndex, true, 0, "Biggest objects of "+className)
}

func (m *Model) debugHeapShowAllObjects() {
	h := &m.debug.heap
	if h.info == nil {
		return
	}
	owner := m.debugHeapOwner()
	m.updateDebugHeap(owner, func(state *debugHeapState) {
		state.mode = "dominators"
		state.selectedClass = 0
		state.selectedObject = 0
		state.path = nil
		state.pathMissing = false
		state.pathError = ""
	})
	m.loadDebugHeapObjects(owner, h.info.ID, 0, 0, false, 0, "Dominators")
}

func (m *Model) debugHeapShowMoreObjects() {
	h := &m.debug.heap
	if h.info == nil || h.objectsLoading ||
		h.objectsTotal <= uint32(len(h.objects)) ||
		len(h.objects) >= debugHeapMaxLoadedObjects {
		return
	}
	m.loadDebugHeapObjects(
		m.debugHeapOwner(), h.info.ID, h.objectParent, h.objectClass, h.objectClassSet,
		uint32(len(h.objects)), h.objectTitle,
	)
}

func (m *Model) debugHeapShowObjectChildren(row debugHeapObjectRow) {
	h := &m.debug.heap
	if h.info == nil || !row.HasChildren || len(h.objects) >= debugHeapMaxLoadedObjects {
		return
	}
	m.updateDebugHeap(m.debugHeapOwner(), func(state *debugHeapState) {
		state.selectedObject = row.Object
		state.path = nil
		state.pathMissing = false
		state.pathError = ""
	})
	m.loadDebugHeapObjects(
		m.debugHeapOwner(), h.info.ID, row.Object, 0, false, 0,
		row.ClassName+"@"+row.Address,
	)
}

func (m *Model) loadDebugHeapPath(row debugHeapObjectRow) {
	h := &m.debug.heap
	if h.info == nil {
		return
	}
	snapshot := h.info.ID
	owner := m.debugHeapOwner()
	var request uint64
	ok := m.updateDebugHeap(owner, func(state *debugHeapState) {
		if state.selected != snapshot {
			return
		}
		state.pathRequest++
		request = state.pathRequest
		state.pathLoading = true
		state.path = nil
		state.pathMissing = false
		state.pathError = ""
		state.pathSnapshot = snapshot
		state.pathObject = row.Object
		state.selectedObject = row.Object
	})
	if !ok || request == 0 {
		return
	}
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.debugHeapContext(), 2*time.Minute)
		defer cancel()
		raw, err := m.invoke(ctx, "debug_heap_path", map[string]any{
			"snapshot": snapshot, "object": row.Object,
		})
		var path *debugHeapRootPath
		if err == nil {
			err = json.Unmarshal(raw, &path)
		}
		m.post(func() {
			m.updateDebugHeap(owner, func(state *debugHeapState) {
				if state.selected != snapshot || state.pathRequest != request {
					return
				}
				state.pathLoading = false
				if err != nil {
					state.pathError = "Could not read the path to a GC root: " + err.Error()
					return
				}
				state.path = path
				state.pathMissing = path == nil
			})
			m.invalidate()
		})
	}()
}

func (m *Model) loadDebugHeapCompare(base uint64) {
	h := &m.debug.heap
	if h.info == nil {
		return
	}
	current := h.info.ID
	owner := m.debugHeapOwner()
	var request uint64
	ok := m.updateDebugHeap(owner, func(state *debugHeapState) {
		if state.selected != current {
			return
		}
		state.compareRequest++
		request = state.compareRequest
		state.compareBase = base
		state.compareCurrent = current
		state.compare = nil
		state.compareError = ""
		state.compareLoading = base != 0 && base != current
	})
	if !ok || request == 0 || base == 0 || base == current {
		m.invalidate()
		return
	}
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.debugHeapContext(), 2*time.Minute)
		defer cancel()
		raw, err := m.invoke(ctx, "debug_heap_compare", map[string]any{
			"base": base, "current": current,
		})
		var rows []debugHeapClassDiff
		if err == nil {
			err = json.Unmarshal(raw, &rows)
		}
		m.post(func() {
			m.updateDebugHeap(owner, func(state *debugHeapState) {
				if state.selected != current || state.compareRequest != request ||
					state.compareBase != base {
					return
				}
				state.compareLoading = false
				if err != nil {
					state.compareError = "Could not compare heap snapshots: " + err.Error()
					return
				}
				state.compare = rows
			})
			m.invalidate()
		})
	}()
}

func (m *Model) closeDebugHeapSnapshotIDs(ids []uint64) {
	if len(ids) == 0 || (m.invokeHook == nil && m.mgr == nil) {
		return
	}
	go func() {
		for _, id := range ids {
			if id == 0 {
				continue
			}
			ctx, cancel := context.WithTimeout(m.debugHeapContext(), 30*time.Second)
			_, _ = m.invoke(ctx, "debug_heap_close", map[string]any{"snapshot": id})
			cancel()
		}
	}()
}

func (m *Model) debugHeapContext() context.Context {
	if m.ctx != nil {
		return m.ctx
	}
	return context.Background()
}

func (m *Model) debugHeapCloseSessionSnapshots(id uint32) {
	state := m.debug.sessions[id]
	if state == nil {
		return
	}
	ids := make([]uint64, 0, len(state.heap.snapshots)+1)
	for _, entry := range state.heap.snapshots {
		ids = append(ids, entry.info.ID)
	}
	if state.heap.info != nil && len(ids) == 0 {
		ids = append(ids, state.heap.info.ID)
	}
	m.releaseDebugHeapChannels(id)
	m.closeDebugHeapSnapshotIDs(ids)
}

func debugHeapObjectText(row debugHeapObjectRow) string {
	text := row.ClassName + "@" + row.Address
	if row.Preview != nil && *row.Preview != "" {
		text += " · " + *row.Preview
	}
	return text
}

func debugHeapEdgeText(edge debugHeapEdge) string {
	switch edge.Kind {
	case "field":
		if edge.Name == "" {
			return ".?"
		}
		return "." + edge.Name
	case "element":
		return fmt.Sprintf("[%d]", edge.Index)
	case "static":
		return "static " + edge.Name
	case "classOf":
		return "class"
	case "superClass":
		return "superclass"
	case "classLoader":
		return "class loader"
	case "root":
		thread := ""
		if edge.Thread != nil {
			thread = *edge.Thread
		}
		switch edge.Root {
		case "jniGlobal":
			return "JNI global"
		case "jniLocal":
			return "JNI local"
		case "javaFrame":
			if thread != "" {
				return "Local variable in " + thread
			}
			return "Local variable"
		case "nativeStack":
			return "Native stack"
		case "stickyClass":
			return "System class"
		case "threadBlock":
			return "Thread block"
		case "monitorUsed":
			return "Monitor"
		case "threadObject":
			if thread != "" {
				return "Thread " + thread
			}
			return "Thread"
		default:
			return "Unknown root"
		}
	}
	return ""
}

func debugHeapClassDiffText(diff debugHeapClassDiff, field string) string {
	if diff.Ambiguous {
		return "—"
	}
	if field == "count" {
		if diff.Count == diff.BaseCount {
			return "0"
		}
		if diff.Count > diff.BaseCount {
			return fmt.Sprintf("+%d", diff.Count-diff.BaseCount)
		}
		return fmt.Sprintf("−%d", diff.BaseCount-diff.Count)
	}
	if diff.Retained == diff.BaseRetained {
		return debugBytes(0)
	}
	if diff.Retained > diff.BaseRetained {
		return "+" + debugBytes(diff.Retained-diff.BaseRetained)
	}
	return "−" + debugBytes(diff.BaseRetained-diff.Retained)
}

func debugHeapChangesByClass(rows []debugHeapClassDiff) map[string]debugHeapClassDiff {
	changes := make(map[string]debugHeapClassDiff, len(rows))
	for _, row := range rows {
		changes[row.Name] = row
	}
	return changes
}

func debugHeapPathRows(path *debugHeapRootPath) []debugHeapRootPathRow {
	if path == nil {
		return nil
	}
	rows := make([]debugHeapRootPathRow, 0, len(path.Steps)+1)
	for i, step := range path.Steps {
		if path.Omitted > 0 && uint32(i) == path.OmittedAfter {
			rows = append(rows, debugHeapRootPathRow{omitted: path.Omitted})
		}
		rows = append(rows, debugHeapRootPathRow{step: step})
	}
	if path.Omitted > 0 && path.OmittedAfter >= uint32(len(path.Steps)) {
		rows = append(rows, debugHeapRootPathRow{omitted: path.Omitted})
	}
	return rows
}

type debugHeapRootPathRow struct {
	step    debugHeapPathStep
	omitted uint32
}
