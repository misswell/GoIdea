package app

import (
	"fmt"
	"slices"

	"github.com/egoist/mygo/ui"
)

func (m *Model) coveragePanel(c *ui.Context, p *ui.Theme) {
	s := &m.coverage
	oi := oiPaletteOf(p)
	ui.Column(c.Key("coverage:panel")).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
		ui.Row(c).Height(28).Padding(0, 6).Gap(3).AlignItems(ui.Center).Children(func() {
			if len(s.suites) == 0 {
				ui.Text(c, "Coverage").FontSize(12).Bold().TextColor(oi.textStrong)
				ui.Spacer(c)
			} else {
				m.coverageTabs(c, p, oi)
			}
			m.headerIcon(c, p, oi, "coverage-import", "Import Coverage Report…", "nodes/folder", m.ImportCoverageReport)
			m.headerIcon(c, p, oi, "coverage-manage", "Manage Coverage Reports…", "toolwindows/coverage", m.ManageCoverageReports)
			if s.active != 0 && s.visible {
				m.headerIcon(c, p, oi, "coverage-hide", "Hide Coverage", "general/hide", func() { m.SetCoverageVisible(false) })
			} else if s.active != 0 {
				m.headerIcon(c, p, oi, "coverage-show", "Show Coverage", "general/show", func() { m.SetCoverageVisible(true) })
			} else {
				m.headerIconDisabled(c, p, oi, "coverage-show", "Show Coverage", "general/show", nil)
			}
			m.headerIcon(c, p, oi, "coverage-close", "Hide Coverage Tool Window", "general/close", func() { m.DockHide("coverage") })
		})
		m.coverageStatus(c, oi)
		suite := s.activeSuite()
		switch {
		case suite == nil:
			ui.Column(c).Grow(1).Padding(14, 10).Gap(8).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				ui.Text(c, "No coverage data").FontSize(13).TextColor(oi.textSecondary)
				ui.Text(c, "Run a configuration with coverage to see which lines its tests execute.").
					FontSize(12).TextColor(oi.textMuted)
				ui.Button(c, "Run with Coverage").OnClick(m.RunWithCoverage)
				ui.Button(c, "Import a report collected in CI from disk").OnClick(m.ImportCoverageReport)
			})
		case len(suite.tree) == 0:
			ui.Column(c).Padding(10).Children(func() {
				ui.Text(c, "The report covers no files").FontSize(12).TextColor(oi.textMuted)
			})
		default:
			m.coverageTable(c, oi, suite)
		}
	})
}

func (m *Model) coverageTabs(c *ui.Context, p *ui.Theme, oi oiColors) {
	s := &m.coverage
	ui.ScrollHorizontal(c.Key("coverage:tabs")).Grow(1).MinWidth(0).Gap(2).Children(func() {
		for _, suite := range s.suites {
			ui.Row(c.Key(fmt.Sprintf("coverage:tab:%d", suite.id))).Height(23).Gap(2).AlignItems(ui.Center).Children(func() {
				button := ui.ButtonBase(c.Key("select")).Height(22).MaxWidth(150).Padding(0, 6).Radius(4).
					AlignItems(ui.Center).OnClick(func() { m.selectCoverageSuite(suite.id) })
				if suite.id == s.active {
					button.Background(oi.tabSelected)
				} else if button.Hovered() {
					button.Background(oi.hover)
				}
				button.Tooltip(suite.runner + ", " + suite.created.Local().Format("3:04:05 PM"))
				button.Children(func() {
					ui.Text(c, suite.name).FontSize(12).TextColor(oi.text).SingleLine().Ellipsis("…")
				})
				m.headerIcon(c, p, oi, fmt.Sprintf("coverage-suite:%d", suite.id), "Close "+suite.name, "general/close", func() { m.closeCoverageSuite(suite.id) })
			})
		}
	})
}

func (m *Model) coverageStatus(c *ui.Context, oi oiColors) {
	s := &m.coverage
	msg, color := s.message, oi.textMuted
	if s.err != "" {
		msg, color = s.err, oi.danger
	} else if msg == "" && s.active != 0 && !s.visible {
		msg = "Coverage is hidden in editors and the project view."
	}
	if msg != "" {
		ui.Row(c).Padding(4, 8).Children(func() {
			ui.Text(c, msg).FontSize(12).TextColor(color)
		})
	}
}

func (m *Model) coverageTable(c *ui.Context, oi oiColors, suite *coverageSuite) {
	s := &m.coverage
	if s.sort != s.appliedSort {
		if s.sort.Column != s.appliedSort.Column && s.sort.Column != "name" {
			s.sort.Descending = true
		}
		sortCoverageTree(suite.tree, s.sort)
		s.appliedSort = s.sort
	}
	s.outline.List.Selected = &s.selected
	s.outline.List.Sort = &s.sort
	columns := []ui.TableColumn{
		{Title: "Element", ID: "name", Sortable: true, MinWidth: 104},
		{Title: "Line, %", ID: "lines", Sortable: true, Width: 108},
	}
	if suite.hasBranches {
		columns = append(columns, ui.TableColumn{Title: "Branch, %", ID: "branches", Sortable: true, Width: 108})
	}
	table := ui.OutlineTable(c.Key("coverage:table"), &s.outline, columns, suite.tree,
		func(n *coverageNode) []*coverageNode { return n.children },
		func(n *coverageNode, col int) {
			switch col {
			case 0:
				ui.Row(c).Gap(4).MinWidth(0).AlignItems(ui.Center).Children(func() {
					if n.children != nil {
						oiIconEl(c, "nodes/folder", 14, oi, oi.icon)
					} else {
						fileIconEl(c, oi, n.name, 14, oi.icon)
					}
					ui.Text(c, n.name).Tooltip(n.path).FontSize(12).TextColor(oi.text).SingleLine().Ellipsis("…")
				})
			case 1:
				coverageMetric(c, oi, n.LinesCovered, n.LinesTotal)
			case 2:
				coverageMetric(c, oi, n.BranchesCovered, n.BranchesTotal)
			}
		}).Grow(1).MinWidth(0).MinHeight(0)
	if table.Submitted() && 0 <= s.selected && s.selected < s.outline.Rows() {
		m.openCoverageNode(s.outline.Item(s.selected))
	}
}

func (m *Model) openCoverageNode(n *coverageNode) {
	if n.children == nil {
		m.openFile(n.path)
	}
}

func coverageMetric(c *ui.Context, oi oiColors, covered, total int) {
	ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
		if total > 0 {
			const barWidth = 20
			fill := barWidth * float32(covered) / float32(total)
			ui.Row(c).Size(barWidth, 5).Radius(1).Clip().Background(oi.danger).Children(func() {
				ui.Row(c).Size(fill, 5).Background(gutterAddedColor)
			})
		}
		ui.Text(c, coveragePercent(covered, total)).FontFeatures("tnum").FontSize(12).
			TextColor(oi.textSecondary).SingleLine().Ellipsis("…")
	})
}

func (m *Model) coverageManageDialog(c *ui.Context, p *ui.Theme) {
	s := &m.coverage
	oi := oiPaletteOf(p)
	open := s.manage
	ui.Modal(c, &open, func() {
		ui.Column(c.Key("coverage:reports")).Width(560).Gap(8).Children(func() {
			ui.Text(c, "Coverage Reports").Bold().FontSize(13).TextColor(oi.textStrong)
			m.coverageStatus(c, oi)
			entries := s.reportEntries()
			s.reportIndex = slices.IndexFunc(entries, func(entry coverageReportEntry) bool { return entry.key == s.reportSelection })
			s.reportIndex = max(0, s.reportIndex)
			if len(entries) == 0 {
				ui.Text(c, "No coverage reports. Run with coverage or import a report.").FontSize(12).TextColor(oi.textMuted)
			} else {
				s.reports.Selected = &s.reportIndex
				s.reports.Key = func(i int) any { return entries[i].key }
				list := ui.List(c.Key("coverage:report-list"), &s.reports, len(entries), func(i int) {
					m.coverageReportRow(c, oi, entries[i])
				}).Height(216).MinHeight(0)
				if list.Changed() {
					s.reportSelection = entries[s.reportIndex].key
				}
				if list.Submitted() {
					m.showCoverageReport(entries[s.reportIndex])
				}
			}
			ui.Row(c).Gap(8).Children(func() {
				busy := s.request != nil
				ui.Button(c, "Import…").Disabled(busy).OnClick(m.ImportCoverageReport)
				ui.Button(c, "Delete").Disabled(len(entries) == 0 || busy).OnClick(func() { m.deleteCoverageReport(entries[s.reportIndex]) })
				ui.Spacer(c)
				ui.Button(c, "Cancel").OnClick(func() { s.manage = false; m.invalidate() })
				ui.Button(c, "Show Selected").Disabled(len(entries) == 0 || busy).OnClick(func() { m.showCoverageReport(entries[s.reportIndex]) })
			})
		})
	})
	if !open {
		s.manage = false
	}
}

func (m *Model) coverageReportRow(c *ui.Context, oi oiColors, entry coverageReportEntry) {
	ui.Row(c).Height(36).Padding(0, 6).Gap(8).AlignItems(ui.Center).Children(func() {
		icon := "run/runWithCoverage"
		if entry.path != "" {
			icon = "nodes/folder"
		}
		oiIconEl(c, icon, 16, oi, oi.icon)
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			ui.Text(c, entry.name).FontSize(12).TextColor(oi.textStrong).SingleLine().Ellipsis("…")
			ui.Text(c, entry.detail).FontSize(11).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
		})
		status := entry.created.Format("Jan 2 · 15:04")
		if entry.suite == nil {
			status = "not loaded " + entry.created.Local().Format("1/2/2006, 3:04:05 PM")
		} else if m.coverage.active == entry.suite.id {
			status = "shown " + entry.created.Local().Format("1/2/2006, 3:04:05 PM")
		}
		ui.Text(c, status).FontSize(11).TextColor(oi.textMuted).SingleLine()
	})
}
