package app

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
)

const treeDragThresholdSquared = 16

// treeDragState is Project View's pointer-driven internal drag. The gesture is
// followed from the row that was pressed, while every visible row can claim
// the pointer as the current drop target.
type treeDragState struct {
	source      string
	paths       []string
	x           float32
	y           float32
	distanceSq  float32
	active      bool
	copy        bool
	targetPath  string
	targetFrame uint64
}

func (m *Model) beginTreeDragFrame() {
	m.treeDragFrame++
}

func (m *Model) endTreeDragFrame() {
	d := m.treeDrag
	if d == nil || !d.active || d.targetFrame == m.treeDragFrame {
		return
	}
	if d.targetPath != "" {
		d.targetPath = ""
		m.invalidate()
	}
}

func pointInsideRect(r ui.Rect, x, y float32) bool {
	return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H
}

func (m *Model) treeRowIsRoot(row *treeNode) bool {
	return row == nil || row.path == m.Root() || m.workspaceContains(row.path)
}

func (m *Model) draggableTreePaths(source string) []string {
	var paths []string
	if _, selected := m.tree.sels[source]; selected {
		paths = m.selectedMutableTreePaths()
	}
	if len(paths) == 0 {
		paths = []string{source}
	}
	readOnly := map[string]bool{}
	for _, row := range m.tree.Rows() {
		if row.readOnly {
			readOnly[row.path] = true
		}
	}
	out := paths[:0]
	remote := m.projectTreeUsesRemotePaths()
	for _, path := range paths {
		if readOnly[path] || path == m.Root() || m.workspaceContains(path) {
			continue
		}
		if remote {
			if m.projectTreeEntryKnown(path) {
				out = append(out, path)
			}
			continue
		}
		if _, err := os.Stat(path); err == nil {
			out = append(out, path)
		}
	}
	return m.topLevelTreePaths(out)
}

// handleTreeDrag follows OpenIDEA's pointer gesture: a four-DIP drag starts a
// Move dialog when dropped on a folder, or a Copy dialog with Alt held.
func (m *Model) handleTreeDrag(c *ui.Context, row *treeNode, r ui.Element) {
	dx, dy, pressed := r.Dragged()
	d := m.treeDrag
	if d == nil {
		if !pressed || row.member || row.readOnly || m.treeRowIsRoot(row) {
			return
		}
		lx, ly, _ := r.PointerPosition()
		bounds := r.Bounds()
		d = &treeDragState{
			source: row.path,
			x:      bounds.X + lx,
			y:      bounds.Y + ly,
		}
		m.treeDrag = d
	}
	if d.source != row.path {
		return
	}
	if !pressed {
		if d.active {
			m.finishTreeDrag()
		} else {
			m.treeDrag = nil
		}
		return
	}

	d.distanceSq += dx*dx + dy*dy
	lx, ly, _ := r.PointerPosition()
	bounds := r.Bounds()
	d.x, d.y = bounds.X+lx, bounds.Y+ly
	d.copy = c.Modifiers()&ui.Alt != 0
	if d.active || d.distanceSq < treeDragThresholdSquared {
		return
	}
	d.paths = m.draggableTreePaths(row.path)
	if len(d.paths) == 0 {
		m.treeDrag = nil
		return
	}
	d.active = true
	m.projectTreeFocus = true
	m.invalidate()
}

func (m *Model) updateTreeDragTarget(row *treeNode, r ui.Element) {
	d := m.treeDrag
	if d == nil || !d.active || row.member || row.readOnly {
		return
	}
	if !pointInsideRect(r.Bounds(), d.x, d.y) {
		return
	}
	target := row.path
	if !row.dir {
		target = filepath.Dir(row.path)
	}
	if target == "" {
		return
	}
	d.targetPath = target
	d.targetFrame = m.treeDragFrame
}

func sameTreePath(left, right string) bool {
	left, right = filepath.Clean(left), filepath.Clean(right)
	if caseInsensitiveFileSystem() {
		return strings.EqualFold(left, right)
	}
	return left == right
}

// treeDropPlan returns the movable paths OpenIDEA would pass to its Move or
// Copy dialog. A folder cannot land in itself, and a Move into the current
// parent is a no-op.
func treeDropPlan(paths []string, target string, copy bool) []string {
	target = filepath.Clean(target)
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		path = filepath.Clean(path)
		if sameTreePath(target, path) || underTreePath(target, path) {
			continue
		}
		if !copy && sameTreePath(filepath.Dir(path), target) {
			continue
		}
		out = append(out, path)
	}
	return out
}

func (m *Model) projectTreeDropPlan(paths []string, target string, copy bool) []string {
	target = m.projectTreePathClean(target)
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		path = m.projectTreePathClean(path)
		if m.projectTreeSamePath(target, path) || m.projectTreeUnderPath(target, path) {
			continue
		}
		if !copy && m.projectTreeSamePath(m.projectTreePathDir(path), target) {
			continue
		}
		out = append(out, path)
	}
	return out
}

func (m *Model) finishTreeDrag() {
	d := m.treeDrag
	m.treeDrag = nil
	if d == nil || !d.active || d.targetPath == "" {
		m.invalidate()
		return
	}
	paths := m.projectTreeDropPlan(d.paths, d.targetPath, d.copy)
	if len(paths) == 0 {
		m.invalidate()
		return
	}
	if d.copy {
		m.OpenFileCopyDialog(paths, d.targetPath)
		return
	}
	m.OpenFileMoveDialogFor(paths, d.targetPath, false)
}
