package app

import (
	"context"
	"fmt"
	"path/filepath"
	"time"
)

// SmokeCoverage imports a report through the native window's handler and
// uses a result row to open its source in the real editor.
func (m *Model) SmokeCoverage(ctx context.Context, reportPath string) (map[string]any, error) {
	root := m.Root()
	if root == "" || reportPath == "" {
		return nil, fmt.Errorf("coverage smoke needs a project and a report path")
	}
	reportPath = realPath(reportPath)
	m.post(func() { m.importCoveragePath(reportPath, false) })
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	var path string
	var out map[string]any
	var opened bool
	for {
		var failure string
		m.post(func() {
			s := &m.coverage
			if s.err != "" {
				failure = s.err
				return
			}
			if out == nil {
				suite := s.activeSuite()
				if suite == nil || suite.path != reportPath {
					return
				}
				leaf := firstCoverageFile(suite.tree)
				if leaf == nil {
					failure = "The imported report covers no source files"
					return
				}
				path = leaf.path
				rel, _ := filepath.Rel(root, path)
				out = map[string]any{
					"visible": s.visible, "open": s.open, "files": len(suite.lines), "runner": suite.runner,
					"hasBranches": suite.hasBranches, "linesCovered": suite.totals.LinesCovered,
					"linesTotal": suite.totals.LinesTotal, "branchesCovered": suite.totals.BranchesCovered,
					"branchesTotal": suite.totals.BranchesTotal, "firstFile": rel,
					"badge": m.coverageBadge(path), "gutterMarks": len(m.coverageGutterMarks(path)),
				}
				m.ManageCoverageReports()
				out["managed"] = s.manage && len(s.reportEntries()) > 0
				s.manage = false
				m.openCoverageNode(leaf)
			}
			if t := m.tab(); t != nil && t.doc != nil && t.doc.Path() == realPath(path) {
				out["opened"] = true
				out["editorMarks"] = len(t.state.GutterMarks)
				opened = true
			}
		})
		if failure != "" {
			return out, fmt.Errorf("coverage smoke: %s", failure)
		}
		if opened {
			return out, nil
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-ticker.C:
		}
	}
}

func firstCoverageFile(nodes []*coverageNode) *coverageNode {
	for _, node := range nodes {
		if node.children == nil {
			return node
		}
		if file := firstCoverageFile(node.children); file != nil {
			return file
		}
	}
	return nil
}
