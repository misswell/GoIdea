package app

import (
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

type profileFlameRect struct {
	node  int
	depth int
	x     float64
	width float64
}

type profileTreeRow struct {
	node  int
	depth int
}

func (m *Model) profilerPanel(c *ui.Context, p *ui.Theme, oi oiColors) {
	m.ensureProfiler()
	ui.Column(c.Key("profiler")).Grow(1).MinHeight(0).
		BorderWidth(0, 0, 0, 1).BorderColor(oi.border).Children(func() {
		if len(m.profiler.snapshots) > 0 {
			m.profilerSnapshotTabs(c, p, oi)
		}
		snapshot := m.activeProfileSnapshot()
		if snapshot == nil {
			m.profilerEmpty(c, p, oi)
			return
		}
		m.profilerToolbar(c, p, oi, snapshot)
		if snapshot.notice != "" {
			m.profilerNotice(c, p, oi, snapshot.notice, false)
		}
		if m.profiler.err != "" {
			m.profilerNotice(c, p, oi, m.profiler.err, true)
		}
		switch snapshot.tab {
		case "tree":
			m.profilerCallTree(c, p, oi, snapshot)
		case "methods":
			m.profilerMethods(c, p, oi, snapshot)
		default:
			m.profilerFlame(c, p, oi, snapshot)
		}
	})
}

func (m *Model) profilerSnapshotTabs(c *ui.Context, p *ui.Theme, oi oiColors) {
	ui.Row(c.Key("profiler-snapshots")).Shrink(0).Height(32).Padding(0, 5).Gap(2).
		AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(oi.border).
		Children(func() {
			for _, snapshot := range m.profiler.snapshots {
				snapshot := snapshot
				active := snapshot.id == m.profiler.activeID
				label := snapshot.name
				switch snapshot.profile.Mode {
				case openidea.ProfileAllocations:
					label += " · Allocations"
				case openidea.ProfileLiveObjects:
					label += " · Live Objects"
				}
				tab := ui.ButtonBase(c.Key("profiler-snapshot:"+snapshot.id)).
					Height(26).Padding(0, 7).Gap(4).Radius(4).AlignItems(ui.Center).
					Cursor(ui.CursorPointer)
				if active {
					tab.Background(oi.selected)
				} else if tab.Hovered() {
					tab.Background(oi.hoverTint)
				}
				tab.OnClick(func() { m.SelectProfileSnapshot(snapshot.id) })
				tab.Children(func() {
					ui.Text(c, label).FontSize(12).TextColor(oi.text).
						SingleLine().Ellipsis("…")
				})
				close := ui.ButtonBase(c.Key("profiler-snapshot-close:"+snapshot.id)).
					Size(18, 18).Radius(3).Justify(ui.Center).AlignItems(ui.Center).
					Cursor(ui.CursorPointer).Tooltip("Close Profile")
				if close.Hovered() {
					close.Background(oi.hoverTint)
				}
				close.OnClick(func() { m.CloseProfileSnapshot(snapshot.id) })
				close.Children(func() {
					ui.Text(c, "×").FontSize(13).TextColor(oi.textMuted)
				})
			}
			ui.Spacer(c)
			if m.profiler.preparing {
				ui.Text(c, "Preparing…").FontSize(11).TextColor(oi.textMuted)
			}
			if m.profiler.collecting {
				ui.Text(c, "Collecting…").FontSize(11).TextColor(oi.textMuted)
			}
		})
}

func (m *Model) profilerEmpty(c *ui.Context, p *ui.Theme, oi oiColors) {
	ui.Column(c).Grow(1).MinHeight(0).AlignItems(ui.Center).Justify(ui.Center).
		Padding(24).Gap(9).Children(func() {
		ui.Text(c, "No profiler snapshots").FontSize(14).Bold().TextColor(oi.textStrong)
		description := "Profile a run configuration to see where it spends its time."
		if m.profiler.preparing {
			description = "Preparing the selected run configuration…"
		} else if m.profiler.collecting {
			description = "Writing and reading the profile…"
		}
		ui.Text(c, description).FontSize(12).TextColor(oi.textMuted)
		if m.profiler.notice != "" {
			ui.Text(c, m.profiler.notice).FontSize(12).TextColor(oi.textSecondary)
		}
		if m.profiler.err != "" {
			ui.Text(c, m.profiler.err).FontSize(12).TextColor(oi.danger)
		}

		cfg := m.run.current()
		ui.Row(c).Gap(6).Children(func() {
			cpu := ui.Button(c, "Profile")
			cpu.Disabled(cfg == nil || !profilerSupports(*cfg) || m.profiler.preparing || m.profiler.collecting)
			cpu.OnClick(func() { m.RunWithProfiler(openidea.ProfileCPU) })
			allocations := ui.Button(c, "Profile Allocations")
			allocations.Disabled(cfg == nil || !allocationProfilerSupports(*cfg) ||
				m.profiler.preparing || m.profiler.collecting)
			allocations.OnClick(func() { m.RunWithProfiler(openidea.ProfileAllocations) })
			live := ui.Button(c, "Profile Live Objects")
			live.Disabled(cfg == nil || !allocationProfilerSupports(*cfg) ||
				m.profiler.preparing || m.profiler.collecting)
			live.OnClick(func() { m.RunWithProfiler(openidea.ProfileLiveObjects) })
		})
		if m.profiler.plan != nil && m.profiler.plan.Status == "missingTool" {
			ui.Row(c).Gap(6).Children(func() {
				if m.profiler.plan.Install != nil {
					ui.Button(c, "Install "+m.profiler.plan.Tool).OnClick(func() { m.InstallProfiler() })
				}
				if m.profiler.plan.Homepage != nil {
					ui.Button(c, "Download…").OnClick(func() { m.OpenProfilerHomepage() })
				}
			})
		}
	})
}

func (m *Model) profilerToolbar(c *ui.Context, p *ui.Theme, oi oiColors, snapshot *profileSnapshot) {
	ui.Row(c.Key("profiler-toolbar")).Shrink(0).Height(34).Gap(3).Padding(0, 6).
		AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(oi.border).
		Children(func() {
			for _, item := range []struct{ id, label string }{
				{"flame", "Flame Graph"},
				{"tree", "Call Tree"},
				{"methods", "Method List"},
			} {
				item := item
				tab := ui.ButtonBase(c.Key("profiler-view:"+item.id)).Height(26).Padding(0, 8).
					Radius(4).AlignItems(ui.Center).Cursor(ui.CursorPointer)
				if snapshot.tab == item.id {
					tab.Background(oi.selected)
				} else if tab.Hovered() {
					tab.Background(oi.hoverTint)
				}
				tab.OnClick(func() {
					snapshot.tab = item.id
					m.invalidate()
				})
				tab.Children(func() {
					ui.Text(c, item.label).FontSize(12).TextColor(oi.text)
				})
			}
			ui.Text(c, profileSummary(snapshot.profile)).FontSize(11).TextColor(oi.textMuted).
				SingleLine().Ellipsis("…")
			ui.Spacer(c)
			if snapshot.tab != "tree" {
				input := ui.TextInput(c.Key("profiler-search"), &m.profiler.query).
					Width(220).Placeholder("Search frames")
				if input.Changed() {
					m.invalidate()
				}
			}
		})
}

func (m *Model) profilerNotice(c *ui.Context, p *ui.Theme, oi oiColors, text string, errorText bool) {
	tone := oi.textSecondary
	if errorText {
		tone = oi.danger
	}
	ui.Row(c).Shrink(0).Padding(4, 9).Gap(6).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(oi.border).Children(func() {
		ui.Text(c, text).FontSize(11).TextColor(tone)
	})
}

func (m *Model) profilerFlame(c *ui.Context, p *ui.Theme, oi oiColors, snapshot *profileSnapshot) {
	profile := snapshot.profile
	if snapshot.focus >= len(profile.Nodes) {
		snapshot.focus = 0
	}
	rects, depth := profileFlameLayout(profile, snapshot.focus, 0.002)
	if snapshot.focus != 0 && snapshot.focus < len(profile.Nodes) {
		node := profile.Nodes[snapshot.focus]
		name := "all"
		if node.Frame >= 0 && node.Frame < len(profile.Frames) {
			name = profile.Frames[node.Frame].Name
		}
		ui.Row(c).Shrink(0).Height(27).Padding(0, 7).Gap(6).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(oi.border).Children(func() {
			ui.Button(c, "Reset Zoom").OnClick(func() {
				snapshot.focus = 0
				m.invalidate()
			})
			ui.Text(c, "Zoomed into "+name).FontSize(11).TextColor(oi.textMuted)
		})
	}
	matches := profileMatchingFrames(profile, m.profiler.query)
	height := float32(max(depth, 1) * 20)
	ui.Scroll(c.Key("profiler-flame-scroll")).Grow(1).MinHeight(0).Children(func() {
		ui.Box(c.Key("profiler-flame")).WidthPercent(100).Height(height).
			Children(func() {
				for _, rect := range rects {
					rect := rect
					if rect.node < 0 || rect.node >= len(profile.Nodes) {
						continue
					}
					node := profile.Nodes[rect.node]
					if node.Frame < 0 || node.Frame >= len(profile.Frames) {
						continue
					}
					frame := profile.Frames[node.Frame]
					background := profileFrameColor(frame, oi)
					button := ui.ButtonBase(c.Key(fmt.Sprintf("profiler-flame:%d", rect.node))).
						Absolute().LeftPercent(float32(rect.x*100)).
						Top(float32((depth-1-rect.depth)*20)).
						WidthPercent(float32(max(rect.width*100, 0.12))).Height(18).
						Padding(0, 3).Radius(1).Background(background).
						AlignItems(ui.Center).Cursor(ui.CursorPointer)
					if matches != nil {
						if matches[node.Frame] {
							button.BorderWidth(1).BorderColor(oi.textStrong)
						} else {
							button.Opacity(0.28)
						}
					}
					label := frame.Name
					if rect.node == 0 {
						label = "all (" + formatProfileWeight(profileTotal(profile), profile.Unit) + ")"
					}
					if rect.width >= 0.025 {
						button.Children(func() {
							ui.Text(c, label).FontSize(10).Font("monospace").
								TextColor(oi.textStrong).SingleLine().Ellipsis("…")
						})
					}
					button.Tooltip(profileFrameTooltip(profile, rect.node))
					button.OnClick(func() {
						snapshot.focus = rect.node
						m.invalidate()
					})
					if button.DoubleClicked() {
						m.openProfileFrame(snapshot, node.Frame)
					}
				}
			})
	})
}

func (m *Model) profilerCallTree(c *ui.Context, p *ui.Theme, oi oiColors, snapshot *profileSnapshot) {
	rows := profileTreeRows(snapshot.profile, snapshot.expanded)
	if len(rows) == 0 {
		ui.Column(c).Grow(1).AlignItems(ui.Center).Justify(ui.Center).
			Children(func() { ui.Text(c, "The profile has no call tree").FontSize(12).TextColor(oi.textMuted) })
		return
	}
	snapshot.treeList.Key = func(row int) any {
		if row < 0 || row >= len(rows) {
			return row
		}
		return rows[row].node
	}
	snapshot.treeList.Selected = &snapshot.treeSel
	ui.List(c.Key("profiler-tree"), &snapshot.treeList, len(rows), func(row int) {
		if row < 0 || row >= len(rows) {
			return
		}
		item := rows[row]
		if item.node < 0 || item.node >= len(snapshot.profile.Nodes) {
			return
		}
		node := snapshot.profile.Nodes[item.node]
		if node.Frame < 0 || node.Frame >= len(snapshot.profile.Frames) {
			return
		}
		frame := snapshot.profile.Frames[node.Frame]
		r := ui.Row(c.Key(fmt.Sprintf("profiler-tree-row:%d", item.node))).Height(23).
			Padding(2, 6).Gap(6).AlignItems(ui.Center)
		r.Children(func() {
			ui.Text(c, "").Width(float32(item.depth * 14))
			arrow := " "
			if len(node.Children) > 0 {
				arrow = "▸"
				if snapshot.expanded[item.node] {
					arrow = "▾"
				}
			}
			ui.Text(c, arrow).Width(12).FontSize(11).TextColor(oi.textMuted)
			ui.Text(c, shareProfileWeight(node.Total, profileTotal(snapshot.profile))).
				Width(48).FontSize(11).TextColor(oi.textMuted)
			ui.Text(c, frame.Name).FontSize(12).TextColor(oi.text).
				Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
			if frame.Kind == "allocatedType" {
				ui.Text(c, "allocated").FontSize(9).TextColor(oi.link)
			}
			if location := profileFrameLocation(frame); location != "" {
				ui.Text(c, location).FontSize(10).TextColor(oi.textMuted).
					Width(180).SingleLine().Ellipsis("…")
			}
			weight := formatProfileWeight(pointerFloat(node.Total), snapshot.profile.Unit)
			if pointerFloat(node.SelfWeight) > 0 {
				weight += " · self " + formatProfileWeight(pointerFloat(node.SelfWeight), snapshot.profile.Unit)
			}
			ui.Text(c, weight).FontSize(10).TextColor(oi.textMuted)
		})
		if r.Clicked() && len(node.Children) > 0 {
			snapshot.expanded[item.node] = !snapshot.expanded[item.node]
			m.invalidate()
		}
		if r.RowDoubleClicked() {
			m.openProfileFrame(snapshot, node.Frame)
		}
		r.ContextMenu(func(menu *ui.Menu) {
			if menu.Item("Jump to Source").Chosen() {
				m.openProfileFrame(snapshot, node.Frame)
			}
			if len(node.Children) > 0 {
				if menu.Item("Expand Children").Chosen() {
					for _, child := range node.Children {
						snapshot.expanded[item.node] = true
						snapshot.expanded[child] = true
					}
					m.invalidate()
				}
				if menu.Item("Collapse Children").Chosen() {
					snapshot.expanded[item.node] = false
					m.invalidate()
				}
			}
		})
	}).Grow(1).MinHeight(0)
}

func (m *Model) profilerMethods(c *ui.Context, p *ui.Theme, oi oiColors, snapshot *profileSnapshot) {
	rows := profileMethodRows(snapshot.profile, snapshot.methodSort, m.profiler.query)
	header := func(label, sortID string, width float32) {
		button := ui.ButtonBase(c.Key("profiler-method-sort:"+sortID)).Height(22).
			Padding(0, 4).Radius(3).AlignItems(ui.Center).Cursor(ui.CursorPointer)
		if snapshot.methodSort == sortID {
			button.Background(oi.selected)
		} else if button.Hovered() {
			button.Background(oi.hoverTint)
		}
		button.OnClick(func() {
			if snapshot.methodSort == sortID && sortID != "name" {
				snapshot.methodSort = "name"
			} else {
				snapshot.methodSort = sortID
			}
			m.invalidate()
		})
		button.Children(func() {
			ui.Text(c, label).FontSize(11).Bold().TextColor(oi.textSecondary).
				Width(width).SingleLine()
		})
	}
	ui.Row(c).Shrink(0).Height(25).Padding(0, 6).Gap(4).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(oi.border).Children(func() {
		header("Method", "name", 360)
		ui.Spacer(c)
		header("Self", "self", 100)
		header("Total", "total", 100)
	})
	if len(rows) == 0 {
		ui.Column(c).Grow(1).AlignItems(ui.Center).Justify(ui.Center).
			Children(func() { ui.Text(c, "No methods match the search").FontSize(12).TextColor(oi.textMuted) })
		return
	}
	snapshot.methodList.Key = func(row int) any {
		if row < 0 || row >= len(rows) {
			return row
		}
		return rows[row].Frame
	}
	snapshot.methodList.Selected = &snapshot.methodSel
	ui.List(c.Key("profiler-methods"), &snapshot.methodList, len(rows), func(row int) {
		if row < 0 || row >= len(rows) {
			return
		}
		item := rows[row]
		if item.Frame < 0 || item.Frame >= len(snapshot.profile.Frames) {
			return
		}
		frame := snapshot.profile.Frames[item.Frame]
		r := ui.Row(c.Key(fmt.Sprintf("profiler-method-row:%d", item.Frame))).Height(23).
			Padding(2, 6).Gap(5).AlignItems(ui.Center)
		r.Children(func() {
			ui.Text(c, frame.Name).FontSize(12).TextColor(oi.text).
				Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
			if frame.Kind == "allocatedType" {
				ui.Text(c, "allocated").FontSize(9).TextColor(oi.link)
			}
			if location := profileFrameLocation(frame); location != "" {
				ui.Text(c, location).FontSize(10).TextColor(oi.textMuted).
					Width(180).SingleLine().Ellipsis("…")
			}
			ui.Text(c, profileMethodWeight(item.SelfWeight, item.Total, snapshot.profile, true)).
				Width(100).FontSize(10).TextColor(oi.textSecondary).SingleLine()
			ui.Text(c, profileMethodWeight(item.SelfWeight, item.Total, snapshot.profile, false)).
				Width(100).FontSize(10).TextColor(oi.textSecondary).SingleLine()
		})
		if r.RowDoubleClicked() {
			m.openProfileFrame(snapshot, item.Frame)
		}
	}).Grow(1).MinHeight(0)
}

func profileFlameLayout(profile openidea.Profile, focus int, minWidth float64) ([]profileFlameRect, int) {
	if len(profile.Nodes) == 0 {
		return nil, 0
	}
	if focus < 0 || focus >= len(profile.Nodes) {
		focus = 0
	}
	parents := profileParents(profile)
	var ancestors []int
	for parent := parents[focus]; parent >= 0; parent = parents[parent] {
		ancestors = append([]int{parent}, ancestors...)
	}
	rects := make([]profileFlameRect, 0, len(profile.Nodes))
	for depth, node := range ancestors {
		rects = append(rects, profileFlameRect{node: node, depth: depth, x: 0, width: 1})
	}
	base := len(ancestors)
	total := pointerFloat(profile.Nodes[focus].Total)
	if total <= 0 {
		return rects, base
	}
	maxDepth := base
	type frame struct {
		node  int
		depth int
		x     float64
	}
	stack := []frame{{node: focus, depth: base, x: 0}}
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if current.node < 0 || current.node >= len(profile.Nodes) {
			continue
		}
		node := profile.Nodes[current.node]
		width := pointerFloat(node.Total) / total
		if width < minWidth {
			continue
		}
		rects = append(rects, profileFlameRect{
			node: current.node, depth: current.depth, x: current.x, width: width,
		})
		if current.depth > maxDepth {
			maxDepth = current.depth
		}
		childX := current.x
		for _, child := range node.Children {
			stack = append(stack, frame{node: child, depth: current.depth + 1, x: childX})
			if child >= 0 && child < len(profile.Nodes) {
				childX += pointerFloat(profile.Nodes[child].Total) / total
			}
		}
	}
	return rects, maxDepth + 1
}

func profileParents(profile openidea.Profile) []int {
	parents := make([]int, len(profile.Nodes))
	for index := range parents {
		parents[index] = -1
	}
	for index, node := range profile.Nodes {
		for _, child := range node.Children {
			if child >= 0 && child < len(parents) {
				parents[child] = index
			}
		}
	}
	return parents
}

func profileTreeRows(profile openidea.Profile, expanded map[int]bool) []profileTreeRow {
	if len(profile.Nodes) == 0 {
		return nil
	}
	rows := make([]profileTreeRow, 0, len(profile.Nodes))
	var visit func(int, int)
	visit = func(node, depth int) {
		if node < 0 || node >= len(profile.Nodes) {
			return
		}
		rows = append(rows, profileTreeRow{node: node, depth: depth})
		if !expanded[node] {
			return
		}
		for _, child := range profile.Nodes[node].Children {
			visit(child, depth+1)
		}
	}
	for _, child := range profile.Nodes[0].Children {
		visit(child, 0)
	}
	return rows
}

func profileMethodRows(profile openidea.Profile, mode, query string) []openidea.MethodStat {
	matches := profileMatchingFrames(profile, query)
	rows := make([]openidea.MethodStat, 0, min(len(profile.Methods), 2000))
	for _, method := range profile.Methods {
		if matches != nil && !matches[method.Frame] {
			continue
		}
		rows = append(rows, method)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		switch mode {
		case "name":
			return profileFrameName(profile, rows[i].Frame) < profileFrameName(profile, rows[j].Frame)
		case "total":
			return pointerFloat(rows[i].Total) > pointerFloat(rows[j].Total)
		default:
			return pointerFloat(rows[i].SelfWeight) > pointerFloat(rows[j].SelfWeight)
		}
	})
	if len(rows) > 2000 {
		rows = rows[:2000]
	}
	return rows
}

func profileMatchingFrames(profile openidea.Profile, query string) map[int]bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil
	}
	matches := make(map[int]bool)
	for index, frame := range profile.Frames {
		if strings.Contains(strings.ToLower(frame.Name), query) {
			matches[index] = true
		}
	}
	return matches
}

func profileSummary(profile openidea.Profile) string {
	total := profileTotal(profile)
	switch profile.Mode {
	case openidea.ProfileAllocations:
		return profile.Profiler + " · " + formatProfileWeight(total, profile.Unit) + " allocated"
	case openidea.ProfileLiveObjects:
		count := int(math.Round(total))
		label := fmt.Sprintf("%d live samples (≈ 512 KB each)", count)
		if count == 1 {
			label = "1 live sample (≈ 512 KB)"
		}
		return profile.Profiler + " · " + label
	default:
		return profile.Profiler + " · " + formatProfileWeight(total, profile.Unit)
	}
}

func formatProfileWeight(value float64, unit string) string {
	switch unit {
	case "bytes":
		units := []string{"B", "KB", "MB", "GB", "TB"}
		unitIndex := 0
		for math.Abs(roundProfile(value, 1)) >= 1024 && unitIndex < len(units)-1 {
			value /= 1024
			unitIndex++
		}
		if unitIndex == 0 {
			return fmt.Sprintf("%d B", int(math.Round(value)))
		}
		return fmt.Sprintf("%.1f %s", value, units[unitIndex])
	case "ms":
		if value >= 1000 {
			places := 2
			if value >= 10000 {
				places = 1
			}
			return fmt.Sprintf("%.*f s", places, value/1000)
		}
		if value >= 100 {
			return fmt.Sprintf("%.0f ms", value)
		}
		return fmt.Sprintf("%.1f ms", value)
	default:
		count := int(math.Round(value))
		if count == 1 {
			return "1 sample"
		}
		return fmt.Sprintf("%d samples", count)
	}
}

func roundProfile(value float64, places int) float64 {
	scale := math.Pow10(places)
	return math.Round(value*scale) / scale
}

func shareProfileWeight(value *float64, total float64) string {
	if total <= 0 {
		return "0%"
	}
	percent := pointerFloat(value) / total * 100
	if percent >= 10 {
		return fmt.Sprintf("%.0f%%", percent)
	}
	return fmt.Sprintf("%.1f%%", percent)
}

func profileTotal(profile openidea.Profile) float64 {
	return pointerFloat(profile.Total)
}

func pointerFloat(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func profileFrameLocation(frame openidea.ProfileFrame) string {
	if frame.File == nil || *frame.File == "" {
		return ""
	}
	name := filepath.Base(*frame.File)
	if frame.Line != nil {
		return fmt.Sprintf("%s:%d", name, *frame.Line+1)
	}
	return name
}

func profileFrameTooltip(profile openidea.Profile, nodeIndex int) string {
	if nodeIndex < 0 || nodeIndex >= len(profile.Nodes) {
		return ""
	}
	node := profile.Nodes[nodeIndex]
	if node.Frame < 0 || node.Frame >= len(profile.Frames) {
		return ""
	}
	frame := profile.Frames[node.Frame]
	kind := ""
	if frame.Kind == "allocatedType" {
		kind = "Allocated type\n"
	}
	location := profileFrameLocation(frame)
	if location != "" {
		location = " (" + location + ")"
	}
	return fmt.Sprintf(
		"%s%s%s\nTotal: %s (%s)\nSelf: %s",
		kind,
		frame.Name,
		location,
		formatProfileWeight(pointerFloat(node.Total), profile.Unit),
		shareProfileWeight(node.Total, profileTotal(profile)),
		formatProfileWeight(pointerFloat(node.SelfWeight), profile.Unit),
	)
}

func profileMethodWeight(self, total *float64, profile openidea.Profile, useSelf bool) string {
	weight := total
	if useSelf {
		weight = self
	}
	return formatProfileWeight(pointerFloat(weight), profile.Unit) + " " +
		shareProfileWeight(weight, profileTotal(profile))
}

func profileFrameName(profile openidea.Profile, frame int) string {
	if frame < 0 || frame >= len(profile.Frames) {
		return ""
	}
	return strings.ToLower(profile.Frames[frame].Name)
}

func profileFrameColor(frame openidea.ProfileFrame, oi oiColors) ui.Color {
	if frame.Kind == "allocatedType" {
		return oi.elev.Mix(oi.accent, 0.30)
	}
	hash := 0
	for _, r := range frame.Name {
		hash = hash*31 + int(r)
	}
	if hash < 0 {
		hash = -hash
	}
	switch hash % 4 {
	case 0:
		return oi.elev.Mix(oi.warning, 0.45)
	case 1:
		return oi.elev.Mix(oi.warning, 0.32)
	case 2:
		return oi.elev.Mix(oi.danger, 0.36)
	default:
		return oi.elev.Mix(oi.danger, 0.26)
	}
}
