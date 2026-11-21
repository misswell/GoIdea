package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

type debugMemoryView struct {
	loaded     bool
	loading    bool
	err        string
	snapshot   debugMemorySnapshot
	filter     string
	selected   string
	list       ui.ListState
	inspection *debugMemoryInspection
}

type debugMemorySnapshot struct {
	Rows           []debugMemoryClass
	TotalInstances uint64
	TotalBytes     uint64
	Source         string
}

type debugMemoryClass struct {
	Name           string
	Module         string
	Instances      uint64
	Bytes          uint64
	DeltaInstances *int64
}

type debugMemoryResult struct {
	Status             string
	Snapshot           debugMemorySnapshot
	Reason             struct{ Kind string }
	CanCountWhenPaused bool
}

func (m *Model) refreshDebugMemory() {
	d := &m.debug
	if d.memory.loading || d.session == 0 || d.ended || d.starting {
		return
	}
	session := d.session
	d.memory.loading = true
	d.memory.err = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "debug_jvm_memory_classes", map[string]any{"session": session})
		var result debugMemoryResult
		if err == nil {
			err = json.Unmarshal(raw, &result)
		}
		m.post(func() {
			if m.debug.session != session {
				return
			}
			v := &m.debug.memory
			v.loading = false
			v.loaded = true
			switch {
			case err != nil:
				v.err = err.Error()
			case result.Status == "ready":
				v.snapshot = result.Snapshot
			case result.CanCountWhenPaused:
				v.err = "Pause the program and refresh to count objects."
			case result.Reason.Kind == "unsafeWhileSuspended":
				v.err = "Resume the program and refresh to capture JVM memory."
			case result.Reason.Kind == "programExited":
				v.err = "The program exited before the snapshot completed."
			case result.Reason.Kind == "timedOut":
				v.err = "The JVM memory request timed out."
			default:
				v.err = "JVM memory is unavailable for this session."
			}
			m.invalidate()
		})
	}()
}

// DebugShowMemoryInstances opens the instances pane for the selected memory
// class, or the first class when nothing is selected.
func (m *Model) DebugShowMemoryInstances() {
	v := &m.debug.memory
	name := v.selected
	if name == "" && len(v.snapshot.Rows) > 0 {
		name = v.snapshot.Rows[0].Name
	}
	if name == "" {
		m.debug.err = "Select a JVM class first"
		m.invalidate()
		return
	}
	m.showDebugMemoryInstances(name)
}

// DebugShowMemoryReferrers opens the incoming-reference tree for the lead
// object in the instances pane.
func (m *Model) DebugShowMemoryReferrers() {
	inspection := m.debug.memory.inspection
	if inspection == nil || len(inspection.instances) == 0 || !m.debug.paused {
		return
	}
	ref := inspection.lead
	if ref.ID == 0 {
		ref, _ = debugMemoryValueRef(inspection.generation, inspection.instances[0])
	}
	node := inspection.nodes[ref]
	if node == nil {
		node = &debugMemoryNode{ref: ref}
		inspection.nodes[ref] = node
	}
	node.expanded = true
	node.showReferrers = true
	if !node.fieldsLoaded {
		m.debugMemoryLoadNode(inspection, node, false)
	}
	m.debugMemoryLoadNode(inspection, node, true)
	m.invalidate()
}

func (m *Model) DebugCloseMemoryInstances() {
	m.debug.memory.inspection = nil
	m.invalidate()
}

// DebugMemoryJumpToSource opens the declaration of the selected JVM class.
// It mirrors OpenIDEA's jumpToClass: query the outer class name, require the
// package-qualified container, and reveal the declaration's exact position.
func (m *Model) DebugMemoryJumpToSource(className string) {
	if className == "" {
		className = m.debug.memory.selected
	}
	if className == "" && m.debug.memory.inspection != nil {
		className = m.debug.memory.inspection.className
	}
	if className == "" {
		return
	}
	outer := debugMemoryOuterClassName(className)
	if outer == "" {
		return
	}
	dot := strings.LastIndexByte(outer, '.')
	simple, pkg := outer, ""
	if dot >= 0 {
		simple, pkg = outer[dot+1:], outer[:dot]
	}
	ctx := m.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	go func() {
		defer cancel()
		raw, err := m.invoke(ctx, "lsp_workspace_symbols", map[string]any{
			"request": m.nextRequest(), "query": simple,
		})
		var symbols []openidea.WorkspaceSymbol
		if err == nil {
			err = json.Unmarshal(raw, &symbols)
		}
		m.post(func() {
			if err != nil {
				m.status = "Could not find the source of " + className + "."
				m.invalidate()
				return
			}
			for _, symbol := range symbols {
				if symbol.Name != simple || !debugMemoryTypeSymbol(symbol.Kind) ||
					!debugMemoryInPackage(symbol.Container, pkg) {
					continue
				}
				m.revealAt(symbol.Location.Path,
					int(symbol.Location.Range.Start.Line),
					int(symbol.Location.Range.Start.Col))
				m.invalidate()
				return
			}
			m.status = "Could not find the source of " + className + "."
			m.invalidate()
		})
	}()
}

func debugMemoryOuterClassName(className string) string {
	className = strings.TrimSpace(className)
	for strings.HasSuffix(className, "[]") {
		className = strings.TrimSuffix(className, "[]")
	}
	if i := strings.IndexByte(className, '$'); i >= 0 {
		className = className[:i]
	}
	return className
}

func debugMemoryTypeSymbol(kind string) bool {
	switch strings.ToLower(kind) {
	case "class", "interface", "enum", "struct":
		return true
	default:
		return false
	}
}

func debugMemoryInPackage(container *string, pkg string) bool {
	if container == nil {
		return pkg == ""
	}
	value := *container
	if value == pkg {
		return true
	}
	if pkg == "" || !strings.HasSuffix(value, pkg) {
		return false
	}
	suffix := strings.TrimSuffix(value, pkg)
	if suffix == "" {
		return true
	}
	last := suffix[len(suffix)-1]
	return !(last == '_' || last == '$' || last == '.' ||
		last >= '0' && last <= '9' ||
		last >= 'A' && last <= 'Z' ||
		last >= 'a' && last <= 'z')
}

func (m *Model) debugMemory(c *ui.Context, p *ui.Theme, oi oiColors) {
	d, v := &m.debug, &m.debug.memory
	ui.Column(c).Grow(1).MinHeight(0).MinWidth(0).Children(func() {
		ui.Row(c).Height(34).Shrink(0).Padding(0, 8).Gap(6).AlignItems(ui.Center).Children(func() {
			debugIcon(c, oi, "memory-refresh", "general/refresh", "Refresh Classes",
				d.session != 0 && !d.ended && !d.starting && !v.loading, false, m.refreshDebugMemory)
			if v.loading {
				ui.Text(c, "Reading JVM memory…").TextColor(p.TextMuted)
			}
			ui.TextInput(c.Key("debug-memory-filter"), &v.filter).Grow(1).Placeholder("Filter classes")
		})
		switch {
		case v.err != "":
			ui.Text(c, v.err).Padding(4, 10).TextColor(p.TextMuted)
		case d.starting:
			ui.Text(c, "Waiting for the JVM to start…").Padding(4, 10).TextColor(p.TextMuted)
		case !v.loading && !v.loaded:
			ui.Text(c, "Refresh to read JVM memory.").Padding(4, 10).TextColor(p.TextMuted)
		}
		snapshot := &v.snapshot
		if len(snapshot.Rows) == 0 {
			return
		}
		query := strings.ToLower(strings.TrimSpace(v.filter))
		rows := snapshot.Rows
		if query != "" {
			rows = make([]debugMemoryClass, 0)
			for _, row := range snapshot.Rows {
				if strings.Contains(strings.ToLower(row.Name), query) {
					rows = append(rows, row)
				}
			}
		}
		showBytes := snapshot.Source != "debugger"
		memoryRow := func(name, count, delta, bytes string, color ui.Color) {
			ui.Row(c).Height(24).Padding(0, 10).Gap(5).AlignItems(ui.Center).Children(func() {
				ui.Text(c, name).Grow(1).MinWidth(0).FontSize(12).TextColor(color).SingleLine().Ellipsis("…")
				ui.Text(c, count).Width(90).FontSize(12).TextColor(color).SingleLine()
				ui.Text(c, delta).Width(75).FontSize(12).TextColor(color).SingleLine()
				if showBytes {
					ui.Text(c, bytes).Width(90).FontSize(12).TextColor(color).SingleLine()
				}
			})
		}
		ui.Row(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
				memoryRow("Class", "Instances", "Δ Instances", "Bytes", p.TextMuted)
				ui.List(c.Key("debug-memory-classes"), &v.list, len(rows), func(i int) {
					row := rows[i]
					name := row.Name
					if row.Module != "" {
						name += "  ·  " + row.Module
					}
					delta := ""
					if row.DeltaInstances != nil {
						delta = fmt.Sprintf("%+d", *row.DeltaInstances)
					}
					item := ui.Row(c.Key("memory-class:" + row.Name)).Height(24).AlignItems(ui.Center)
					if d.paused {
						item.Cursor(ui.CursorPointer)
					}
					item.Children(func() {
						ui.Row(c).Grow(1).MinWidth(0).Children(func() {
							memoryRow(name, fmt.Sprint(row.Instances), delta, fmt.Sprint(row.Bytes), p.Text)
						})
						debugIcon(c, oi, "memory-instances:"+row.Name, "toolwindows/hierarchy",
							"Show Instances", d.paused && !d.ended, false,
							func() { m.showDebugMemoryInstances(row.Name) }).Label("Show Instances")
					})
					if item.RowDoubleClicked() && d.paused {
						m.showDebugMemoryInstances(row.Name)
					}
					if item.Clicked() {
						v.selected = row.Name
					}
					item.Menu(func(menu *ui.Menu) {
						if menu.Item("Show Instances").Disabled(!d.paused).Chosen() {
							m.showDebugMemoryInstances(row.Name)
						}
						if menu.Item("Jump to Source").Chosen() {
							m.DebugMemoryJumpToSource(row.Name)
						}
					})
				}).Grow(1).MinWidth(0).MinHeight(0)
			})
			if v.inspection != nil {
				m.debugMemoryInstances(c, p, oi)
			}
		})
	})
}
