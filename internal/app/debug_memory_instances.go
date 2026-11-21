package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

const (
	debugMemoryInstanceLimit = 1000
	debugMemoryReferrerLimit = 50
)

type debugMemoryObjectRef struct {
	Generation uint64 `json:"generation"`
	ID         uint64 `json:"id"`
}

type debugMemoryValue struct {
	Kind     string  `json:"kind"`
	ID       uint64  `json:"id"`
	TypeName string  `json:"typeName"`
	Text     *string `json:"text"`
	Length   uint64  `json:"length"`
}

type debugMemoryField struct {
	Name  string           `json:"name"`
	Value debugMemoryValue `json:"value"`
}

type debugMemoryReferrer struct {
	Value debugMemoryValue `json:"value"`
	Via   []string         `json:"via"`
}

type debugMemoryInspectResult struct {
	Status string `json:"status"`
	Reason struct {
		Kind   string `json:"kind"`
		Detail string `json:"detail"`
	} `json:"reason"`
	Generation uint64                `json:"generation"`
	Instances  []debugMemoryValue    `json:"instances"`
	Fields     []debugMemoryField    `json:"fields"`
	Referrers  []debugMemoryReferrer `json:"referrers"`
	Truncated  bool                  `json:"truncated"`
}

type debugMemoryNode struct {
	ref                debugMemoryObjectRef
	expanded           bool
	showReferrers      bool
	fieldsLoading      bool
	fieldsLoaded       bool
	fieldsError        string
	fields             []debugMemoryField
	referrersLoading   bool
	referrersLoaded    bool
	referrersError     string
	referrers          []debugMemoryReferrer
	referrersTruncated bool
}

type debugMemoryInspection struct {
	className  string
	session    uint32
	loading    bool
	outdated   bool
	err        string
	generation uint64
	truncated  bool
	lead       debugMemoryObjectRef
	instances  []debugMemoryValue
	nodes      map[debugMemoryObjectRef]*debugMemoryNode
	list       ui.ListState
}

func (v *debugMemoryView) invalidateInspection() {
	if v.inspection != nil {
		v.inspection.outdated = true
		v.inspection.loading = false
	}
}

func (m *Model) showDebugMemoryInstances(className string) {
	d := &m.debug
	if !d.paused || d.ended || d.session == 0 || className == "" {
		return
	}
	inspection := &debugMemoryInspection{
		className: className,
		session:   d.session,
		loading:   true,
		nodes:     make(map[debugMemoryObjectRef]*debugMemoryNode),
	}
	d.memory.inspection = inspection
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "debug_jvm_memory_instances", map[string]any{
			"session": inspection.session, "className": className, "limit": debugMemoryInstanceLimit,
		})
		var result debugMemoryInspectResult
		if err == nil {
			err = json.Unmarshal(raw, &result)
		}
		m.post(func() {
			if !m.debugMemoryInspectionCurrent(inspection) {
				return
			}
			inspection.loading = false
			if err != nil {
				inspection.err = err.Error()
			} else if result.Status != "ready" {
				inspection.err = debugMemoryUnavailable(result.Reason.Kind, result.Reason.Detail)
			} else {
				inspection.generation = result.Generation
				inspection.instances = result.Instances
				inspection.truncated = result.Truncated
			}
			m.invalidate()
		})
	}()
}

func (m *Model) debugMemoryInspectionCurrent(inspection *debugMemoryInspection) bool {
	return inspection != nil && m.debug.session == inspection.session &&
		m.debug.memory.inspection == inspection && !inspection.outdated && m.debug.paused && !m.debug.ended
}

func debugMemoryUnavailable(kind, detail string) string {
	switch kind {
	case "notSuspended":
		return "Pause the program to inspect objects."
	case "stale":
		return "These objects are from an earlier stop. Refresh the instances."
	case "collected":
		return "The object has been garbage collected."
	case "classNotLoaded":
		return "The class is no longer loaded."
	case "vmNotFound":
		return "The JVM could not be found."
	case "timedOut":
		return "The request timed out. Try again."
	case "backendClosed":
		return "The JVM inspector connection closed."
	case "unsupported":
		if detail != "" {
			return detail
		}
		return "Object inspection is not supported by this debugger."
	default:
		return "Object inspection is not available for this session."
	}
}

func debugMemoryValueText(v debugMemoryValue) string {
	short := func(name string) string {
		if i := strings.LastIndexByte(name, '.'); i >= 0 {
			return name[i+1:]
		}
		return name
	}
	switch v.Kind {
	case "null":
		return "null"
	case "primitive", "boxed":
		if v.Text != nil {
			return *v.Text
		}
	case "string":
		if v.Text != nil {
			return strconv.Quote(*v.Text)
		}
		return fmt.Sprintf("String (%d chars)", v.Length)
	case "object":
		return fmt.Sprintf("%s@%d", short(v.TypeName), v.ID)
	case "array":
		return fmt.Sprintf("%s[%d]", short(strings.TrimSuffix(v.TypeName, "[]")), v.Length)
	}
	return ""
}

func debugMemoryValueRef(generation uint64, value debugMemoryValue) (debugMemoryObjectRef, bool) {
	if value.Kind == "null" || value.Kind == "primitive" || value.Kind == "" {
		return debugMemoryObjectRef{}, false
	}
	return debugMemoryObjectRef{Generation: generation, ID: value.ID}, true
}

func (m *Model) debugMemoryLoadNode(inspection *debugMemoryInspection, node *debugMemoryNode, referrers bool) {
	if !m.debugMemoryInspectionCurrent(inspection) {
		return
	}
	cmd := "debug_jvm_memory_fields"
	args := map[string]any{"session": inspection.session, "object": node.ref}
	if referrers {
		if node.referrersLoading {
			return
		}
		node.referrersLoading = true
		node.referrersError = ""
		cmd = "debug_jvm_memory_referrers"
		args["limit"] = debugMemoryReferrerLimit
	} else {
		if node.fieldsLoading {
			return
		}
		node.fieldsLoading = true
		node.fieldsError = ""
	}
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, cmd, args)
		var result debugMemoryInspectResult
		if err == nil {
			err = json.Unmarshal(raw, &result)
		}
		m.post(func() {
			if !m.debugMemoryInspectionCurrent(inspection) || inspection.nodes[node.ref] != node {
				return
			}
			errText := ""
			if err != nil {
				errText = err.Error()
			} else if result.Status != "ready" {
				errText = debugMemoryUnavailable(result.Reason.Kind, result.Reason.Detail)
			}
			if referrers {
				node.referrersLoading = false
				node.referrersLoaded = errText == ""
				node.referrersError = errText
				node.referrers = result.Referrers
				node.referrersTruncated = result.Truncated
			} else {
				node.fieldsLoading = false
				node.fieldsLoaded = errText == ""
				node.fieldsError = errText
				node.fields = result.Fields
			}
			m.invalidate()
		})
	}()
}

type debugMemoryDisplayRow struct {
	depth int
	name  string
	text  string
	node  *debugMemoryNode
	kind  string
}

func (inspection *debugMemoryInspection) displayRows() []debugMemoryDisplayRow {
	rows := make([]debugMemoryDisplayRow, 0, len(inspection.instances))
	var add func(string, debugMemoryValue, int, map[debugMemoryObjectRef]bool)
	add = func(name string, value debugMemoryValue, depth int, parents map[debugMemoryObjectRef]bool) {
		ref, hasRef := debugMemoryValueRef(inspection.generation, value)
		var node *debugMemoryNode
		if hasRef {
			node = inspection.nodes[ref]
			if node == nil {
				node = &debugMemoryNode{ref: ref}
				inspection.nodes[ref] = node
			}
		}
		rows = append(rows, debugMemoryDisplayRow{depth: depth, name: name, text: debugMemoryValueText(value), node: node})
		if node == nil || !node.expanded || parents[ref] || depth >= 32 {
			return
		}
		parents[ref] = true
		defer delete(parents, ref)
		switch {
		case node.fieldsLoading:
			rows = append(rows, debugMemoryDisplayRow{depth: depth + 1, text: "Loading fields…"})
		case node.fieldsError != "":
			rows = append(rows, debugMemoryDisplayRow{depth: depth + 1, text: node.fieldsError, node: node, kind: "retryFields"})
		default:
			for _, field := range node.fields {
				add(field.Name, field.Value, depth+1, parents)
			}
		}
		if node.showReferrers {
			rows = append(rows, debugMemoryDisplayRow{depth: depth + 1, text: "Referrers", node: node, kind: "referrers"})
			switch {
			case node.referrersLoading:
				rows = append(rows, debugMemoryDisplayRow{depth: depth + 2, text: "Loading referrers…"})
			case node.referrersError != "":
				rows = append(rows, debugMemoryDisplayRow{depth: depth + 2, text: node.referrersError, node: node, kind: "retryReferrers"})
			default:
				for _, referrer := range node.referrers {
					add(strings.Join(referrer.Via, ", "), referrer.Value, depth+2, parents)
				}
				if node.referrersTruncated {
					rows = append(rows, debugMemoryDisplayRow{depth: depth + 2, text: fmt.Sprintf("Showing the first %d referrers.", len(node.referrers))})
				}
			}
		}
	}
	for _, value := range inspection.instances {
		add("", value, 0, make(map[debugMemoryObjectRef]bool))
	}
	return rows
}

func (m *Model) debugMemoryInstances(c *ui.Context, p *ui.Theme, oi oiColors) {
	inspection := m.debug.memory.inspection
	ui.Column(c).Grow(1).MinWidth(260).MinHeight(0).
		BorderWidth(0, 0, 0, 1).BorderColor(p.Border).Children(func() {
		ui.Row(c).Height(30).Shrink(0).Padding(0, 8).Gap(4).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Instances of "+inspection.className).Grow(1).MinWidth(0).
				FontSize(12).TextColor(p.Text).SingleLine().Ellipsis("…")
			debugIcon(c, oi, "memory-reload-instances", "general/refresh", "Refresh Instances",
				m.debug.paused && !m.debug.ended && !inspection.loading, false,
				func() { m.showDebugMemoryInstances(inspection.className) })
			debugIcon(c, oi, "memory-close-instances", "general/close", "Close Instances", true, false, func() {
				m.debug.memory.inspection = nil
				m.invalidate()
			})
		})
		switch {
		case inspection.loading:
			ui.Text(c, "Loading…").Padding(4, 8).TextColor(p.TextMuted)
		case inspection.err != "":
			ui.Row(c).Padding(4, 8).Gap(6).Children(func() {
				ui.Text(c, inspection.err).Grow(1).TextColor(p.Danger)
				ui.Button(c, "Retry").Disabled(!m.debug.paused || m.debug.ended).
					OnClick(func() { m.showDebugMemoryInstances(inspection.className) })
			})
		case inspection.outdated:
			ui.Text(c, "Objects are from an earlier stop. Refresh to inspect them.").
				Padding(4, 8).TextColor(p.TextMuted)
		}
		if inspection.truncated {
			ui.Text(c, fmt.Sprintf("Showing the first %d instances.", len(inspection.instances))).
				Padding(4, 8).TextColor(p.TextMuted)
		}
		rows := inspection.displayRows()
		if !inspection.loading && inspection.err == "" && len(rows) == 0 {
			ui.Text(c, "Nothing to show").Padding(4, 8).TextColor(p.TextMuted)
		}
		ui.List(c.Key("debug-memory-instance-list"), &inspection.list, len(rows), func(i int) {
			row := rows[i]
			item := ui.Row(c.Key(fmt.Sprintf("memory-node:%d", i))).Height(24).
				Padding(0, 6+float32(row.depth)*16).Gap(4).AlignItems(ui.Center)
			item.Children(func() {
				if row.node != nil && row.kind == "" {
					glyph := "▸"
					if row.node.expanded {
						glyph = "▾"
					}
					ui.Text(c, glyph).FontSize(12).TextColor(p.TextMuted)
				}
				text := row.text
				if row.name != "" {
					text = row.name + " = " + text
				}
				ui.Text(c, text).Grow(1).MinWidth(0).FontSize(12).
					TextColor(p.Text).SingleLine().Ellipsis("…")
				if row.node != nil && row.kind == "" {
					debugIcon(c, oi, fmt.Sprintf("referrers:%d", i), "toolwindows/hierarchy",
						"Show Referrers", !inspection.outdated && m.debug.paused, false, func() {
							row.node.expanded = true
							row.node.showReferrers = true
							if !row.node.fieldsLoaded {
								m.debugMemoryLoadNode(inspection, row.node, false)
							}
							m.debugMemoryLoadNode(inspection, row.node, true)
						})
				}
			})
			if item.Clicked() && row.node != nil && !inspection.outdated && m.debug.paused {
				inspection.lead = row.node.ref
				switch row.kind {
				case "retryFields":
					m.debugMemoryLoadNode(inspection, row.node, false)
				case "retryReferrers":
					m.debugMemoryLoadNode(inspection, row.node, true)
				case "":
					row.node.expanded = !row.node.expanded
					if row.node.expanded && !row.node.fieldsLoaded {
						m.debugMemoryLoadNode(inspection, row.node, false)
					}
					m.invalidate()
				}
			}
		}).Grow(1).MinHeight(0)
	})
}
