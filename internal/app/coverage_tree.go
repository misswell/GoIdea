package app

import (
	"cmp"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

type coverageNode struct {
	path, name string
	children   []*coverageNode
	coverageCounts
}

func makeCoverageSuite(root, name, path string, data coverageData, id uint64) *coverageSuite {
	suite := &coverageSuite{id: id, name: name, path: path, created: time.Now(), runner: data.Runner,
		hasBranches: data.HasBranches, badges: make(map[string]string), lines: make(map[string][]coverageLine, len(data.Files))}
	top := &coverageNode{}
	findDir := map[string]*coverageNode{root: top}
	var directory func(string) *coverageNode
	directory = func(path string) *coverageNode {
		if n := findDir[path]; n != nil {
			return n
		}
		parent := top
		if up := filepath.Dir(path); up != path {
			parent = directory(up)
		}
		n := &coverageNode{path: path, name: filepath.Base(path)}
		findDir[path] = n
		parent.children = append(parent.children, n)
		return n
	}
	for _, f := range data.Files {
		path := filepath.Clean(f.Path)
		parentDir := filepath.Dir(path)
		if root != "" && !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
			parentDir = filepath.Dir(path)
		}
		// A file outside the project starts at its first absolute directory.
		// The filesystem root is a common container for those entries.
		volumeRoot := filepath.VolumeName(path) + string(filepath.Separator)
		findDir[volumeRoot] = top
		parent := directory(parentDir)
		parent.children = append(parent.children, &coverageNode{path: path, name: filepath.Base(path), coverageCounts: f.coverageCounts})
		suite.lines[path] = f.Lines
	}
	var sum func(*coverageNode) coverageCounts
	sum = func(n *coverageNode) coverageCounts {
		for _, child := range n.children {
			n.coverageCounts.add(sum(child))
		}
		if n.path != "" && n.LinesTotal > 0 {
			suite.badges[n.path] = coveragePercentLabel(n.LinesCovered, n.LinesTotal) + "% lines"
		}
		return n.coverageCounts
	}
	suite.totals = sum(top)
	if root != "" && suite.totals.LinesTotal > 0 {
		suite.badges[root] = coveragePercentLabel(suite.totals.LinesCovered, suite.totals.LinesTotal) + "% lines"
	}
	var compact func(*coverageNode)
	compact = func(n *coverageNode) {
		for len(n.children) == 1 && len(n.children[0].children) > 0 {
			child := n.children[0]
			n.name += "/" + child.name
			n.path, n.children = child.path, child.children
		}
		for _, child := range n.children {
			compact(child)
		}
	}
	for _, child := range top.children {
		compact(child)
	}
	suite.tree = top.children
	return suite
}

func coveragePercentLabel(covered, total int) string {
	p := float64(covered) * 100 / float64(total)
	if p > 0 && p < 1 {
		return "<1"
	}
	return fmt.Sprint(int(p))
}

func coveragePercent(covered, total int) string {
	if total <= 0 {
		return "n/a"
	}
	return fmt.Sprintf("%s%% (%d/%d)", coveragePercentLabel(covered, total), covered, total)
}

func sortCoverageTree(nodes []*coverageNode, order ui.SortOrder) {
	slices.SortStableFunc(nodes, func(a, b *coverageNode) int {
		names := strings.Compare(strings.ToLower(a.name), strings.ToLower(b.name))
		cmpValue := names
		switch order.Column {
		case "lines", "branches":
			ac, at := a.LinesCovered, a.LinesTotal
			bc, bt := b.LinesCovered, b.LinesTotal
			if order.Column == "branches" {
				ac, at = a.BranchesCovered, a.BranchesTotal
				bc, bt = b.BranchesCovered, b.BranchesTotal
			}
			if at <= 0 || bt <= 0 {
				if at == bt {
					return names
				}
				return cmp.Compare(bt, at)
			}
			cmpValue = cmp.Compare(float64(ac)/float64(at), float64(bc)/float64(bt))
			if cmpValue == 0 {
				return names
			}
		default:
			if (a.children != nil) != (b.children != nil) {
				return cmp.Compare(len(b.children), len(a.children))
			}
		}
		if order.Descending {
			return -cmpValue
		}
		return cmpValue
	})
	for _, n := range nodes {
		sortCoverageTree(n.children, order)
	}
}

func (s *coverageState) resetCoverageTree() {
	s.outline = ui.OutlineState[*coverageNode]{Indent: 12}
	s.selected = -1
	if suite := s.activeSuite(); suite != nil {
		if s.sort.Column == "" || (s.sort.Column == "branches" && !suite.hasBranches) {
			s.sort = ui.SortOrder{Column: "name"}
		}
		sortCoverageTree(suite.tree, s.sort)
		s.appliedSort = s.sort
		for _, n := range suite.tree {
			if n.children != nil {
				s.outline.Open.Add(n)
			}
		}
	}
}
