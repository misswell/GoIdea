package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

type debugMemoryView struct {
	loaded   bool
	loading  bool
	err      string
	snapshot debugMemorySnapshot
	filter   string
	list     ui.ListState
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
			memoryRow(name, fmt.Sprint(row.Instances), delta, fmt.Sprint(row.Bytes), p.Text)
		}).Grow(1).MinHeight(0)
	})
}
