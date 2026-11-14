package app

// Go to Implementation(s) after v0.1.44's navigation.ts: the backend
// (lsp_goto_implementation) streams the implementations it finds, the
// project's first and the dependencies' later. One target opens at once,
// none says so, several open a chooser that fills in as the search goes on,
// and closing the chooser cancels the search.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

type implTarget struct {
	path string
	rng  openidea.OutlineRange
}

// implChooser is the open implementations chooser.
type implChooser struct {
	open    bool
	title   string
	items   []implTarget
	sel     int
	busy    bool
	request uint32
	gen     uint64
	name    string
	// done, found and landed record the last search's outcome for the
	// headless acceptance run.
	done   bool
	found  int
	landed string
}

// GoToImplementation is Navigate › Go to Implementation(s).
func (m *Model) GoToImplementation() { m.gotoImplementation() }

// gotoImplementation asks the backend for the implementations at the caret,
// replacing any search already running.
func (m *Model) gotoImplementation() {
	t := m.tab()
	if t == nil || m.mgr == nil || m.ctx == nil {
		return
	}
	off := t.state.Cursor()
	text := t.doc.Buffer().Snapshot().String()
	m.closeImplementation()
	m.impl = implChooser{name: identifierAt(text, off)}
	m.impl.gen++
	gen := m.impl.gen
	ch := m.nextChannel()
	m.impl.request = ch
	docID := t.doc.ID()
	pos := t.doc.UTF16Pos(off)
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
		defer cancel()
		raw, err := m.mgr.Invoke(ctx, Window, "lsp_goto_implementation", map[string]any{
			"request": ch, "doc": docID, "pos": lspPos(pos),
			"onPartial": map[string]any{"$channel": ch},
		})
		var locs []openidea.Location
		if err == nil {
			err = json.Unmarshal(raw, &locs)
		}
		m.post(func() {
			if m.impl.gen != gen {
				return
			}
			if m.impl.request == ch {
				m.impl.request = 0
			}
			m.finishImplementations(locs, err)
		})
	}()
}

// acceptImplementationChunk applies one streamed batch: the first opens the
// chooser, later ones fill it in.
func (m *Model) acceptImplementationChunk(data json.RawMessage) {
	var locs []openidea.Location
	if json.Unmarshal(data, &locs) != nil || m.impl.request == 0 || len(locs) == 0 {
		return
	}
	if !m.impl.open {
		m.openImplementationChooser(locs)
		m.invalidate()
		return
	}
	n := len(m.impl.items)
	m.addImplTargets(locs)
	if len(m.impl.items) != n {
		m.impl.title = m.implementationTitle()
		m.invalidate()
	}
}

// finishImplementations shows what the search ended with, or fills in the
// chooser the stream opened.
func (m *Model) finishImplementations(locs []openidea.Location, err error) {
	m.impl.done = true
	if m.impl.open {
		m.impl.busy = false
		m.addImplTargets(locs)
		m.impl.found = len(m.impl.items)
		m.impl.title = m.implementationTitle()
		m.invalidate()
		return
	}
	switch {
	case err != nil:
		m.status = "查找实现失败：" + err.Error()
	case len(locs) == 0:
		m.status = "未找到实现"
	case len(locs) == 1:
		m.impl.found = 1
		m.openImplTarget(implTargetOf(locs[0]))
		return
	default:
		m.impl.found = len(locs)
		m.openImplementationChooser(locs)
	}
	m.invalidate()
}

func (m *Model) openImplementationChooser(locs []openidea.Location) {
	m.impl.open = true
	m.impl.busy = true
	m.impl.sel = 0
	m.impl.items = nil
	m.addImplTargets(locs)
	m.impl.title = m.implementationTitle()
}

func (m *Model) addImplTargets(locs []openidea.Location) {
	for _, loc := range locs {
		t := implTargetOf(loc)
		if !m.implHas(t) {
			m.impl.items = append(m.impl.items, t)
		}
	}
}

func (m *Model) implHas(t implTarget) bool {
	for _, x := range m.impl.items {
		if x.path == t.path && x.rng == t.rng {
			return true
		}
	}
	return false
}

func (m *Model) implementationTitle() string {
	if m.impl.name == "" {
		return fmt.Sprintf("选择实现（%d）", len(m.impl.items))
	}
	return fmt.Sprintf("选择实现：%s（%d）", m.impl.name, len(m.impl.items))
}

// stepImplementation moves the chooser's selection, wrapping around.
func (m *Model) stepImplementation(delta int) {
	n := len(m.impl.items)
	if n == 0 {
		return
	}
	m.impl.sel = (m.impl.sel + delta + n) % n
	m.invalidate()
}

// chooseImplementation opens the chooser's selected row.
func (m *Model) chooseImplementation(i int) {
	if i < 0 || i >= len(m.impl.items) {
		return
	}
	t := m.impl.items[i]
	m.impl.open = false
	m.impl.gen++
	if m.impl.request != 0 {
		m.cancelRequest(m.impl.request)
		m.impl.request = 0
	}
	m.openImplTarget(t)
}

// closeImplementation drops the chooser and cancels a running search.
func (m *Model) closeImplementation() {
	if m.impl.request != 0 {
		m.cancelRequest(m.impl.request)
	}
	m.impl = implChooser{gen: m.impl.gen}
}

// openImplTarget reveals the implementation a row points at; a target inside
// a jar or jrt has no file to open and says so.
func (m *Model) openImplTarget(t implTarget) {
	m.impl.landed = t.path
	m.impl.done = true
	if strings.Contains(t.path, "://") {
		m.status = "实现在外部依赖中，无法作为文件打开"
		m.invalidate()
		return
	}
	m.revealRange(t.path,
		int(t.rng.Start.Line), int(t.rng.Start.Col),
		int(t.rng.End.Line), int(t.rng.End.Col))
}

func implTargetOf(loc openidea.Location) implTarget {
	return implTarget{path: loc.Path, rng: loc.Range}
}

// identifierAt is the word around a byte offset, for the chooser's title.
func identifierAt(text string, off int) string {
	if off < 0 || off > len(text) {
		return ""
	}
	ident := func(b byte) bool {
		return b == '_' || b == '$' || b >= '0' && b <= '9' ||
			b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
	}
	start := off
	for start > 0 && ident(text[start-1]) {
		start--
	}
	end := off
	for end < len(text) && ident(text[end]) {
		end++
	}
	return text[start:end]
}

// implementationDialog draws the chooser.
func (m *Model) implementationDialog(c *ui.Context, p *ui.Theme) {
	if !m.impl.open {
		return
	}
	items := m.impl.items
	sel := m.impl.sel
	title := m.impl.title
	busy := m.impl.busy
	open := m.impl.open
	jump := -1
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(560).Gap(8).Children(func() {
			ui.Text(c, title).FontSize(13).Bold().TextColor(p.Text).SingleLine().Ellipsis("…")
			hint := fmt.Sprintf("%d 个实现 · Enter 打开 · Esc 取消", len(items))
			if busy {
				hint = fmt.Sprintf("%d 个实现 · 继续查找中… · Esc 取消", len(items))
			}
			ui.Text(c, hint).FontSize(10.5).TextColor(p.TextMuted).SingleLine()
			root := m.Root()
			ui.Scroll(c).Height(320).Children(func() {
				ui.Column(c).Children(func() {
					for i, t := range items {
						t := t
						i := i
						row := ui.Row(c.Key(fmt.Sprintf("impl:%d", i))).Height(24).Padding(0, 8).
							Gap(8).AlignItems(ui.Center)
						if i == sel || row.Hovered() {
							row.Background(p.SurfaceHover)
						}
						row.Children(func() {
							ui.Text(c, implTargetLabel(root, t)).FontSize(12).TextColor(p.Text).
								SingleLine().Ellipsis("…")
						})
						if row.Clicked() {
							jump = i
							m.impl.sel = i
						}
					}
				})
			})
		})
	})
	if !open && m.impl.open {
		m.closeImplementation()
	}
	if jump >= 0 {
		m.chooseImplementation(jump)
	}
}

// implTargetLabel is a target as the chooser names it: project-relative path
// and 1-based line, or the whole path when outside the project.
func implTargetLabel(root string, t implTarget) string {
	path := t.path
	if root != "" {
		if rel, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(rel, "..") {
			path = rel
		}
	}
	return fmt.Sprintf("%s:%d", path, int(t.rng.Start.Line)+1)
}

// SmokeImplementation runs Go to Implementation at a needle in a project file
// and reports what the chooser got, for the real-server acceptance run.
func (m *Model) SmokeImplementation(ctx context.Context, relFile, needle string) (map[string]any, error) {
	out := map[string]any{}
	root := m.Root()
	if root == "" {
		return out, fmt.Errorf("没有打开项目")
	}
	if _, err := m.mgr.Invoke(ctx, Window, "project_trust_set", map[string]any{
		"path": root, "trusted": true, "folder": true,
	}); err != nil {
		return out, err
	}
	if err := m.reg.ReopenAll(ctx); err != nil {
		out["reopenError"] = err.Error()
	}
	path := filepath.Join(root, relFile)
	src, err := os.ReadFile(path)
	if err != nil {
		return out, err
	}
	off := strings.Index(string(src), needle)
	if off < 0 {
		return out, fmt.Errorf("%s 里没有 %q", relFile, needle)
	}
	out["file"], out["needle"] = relFile, needle
	m.post(func() { m.openFile(path) })
	deadline := time.Now().Add(2 * time.Minute)
	for {
		var open bool
		m.post(func() { open = m.tab() != nil && m.tab().doc.Path() == path })
		if open {
			break
		}
		if time.Now().After(deadline) {
			return out, fmt.Errorf("打开 %s 超时", relFile)
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	started := false
	for {
		var done, chooser, busy bool
		var n, found int
		var labels []string
		var status, landed, selected string
		m.post(func() {
			if !started {
				if t := m.tab(); t != nil && t.doc.Path() == path {
					t.state.Select(off, off)
					m.GoToImplementation()
					started = true
				}
				return
			}
			done, chooser, busy = m.impl.done, m.impl.open, m.impl.busy
			n, found = len(m.impl.items), m.impl.found
			for i, it := range m.impl.items {
				if i == 3 {
					break
				}
				labels = append(labels, implTargetLabel(root, it))
			}
			landed = m.impl.landed
			status = m.status
			if t := m.tab(); t != nil {
				selected = t.state.SelectedText()
			}
		})
		if started && done && (!chooser || !busy) {
			if landed != "" && !strings.Contains(landed, "://") {
				if rel, err := filepath.Rel(root, landed); err == nil {
					landed = rel
				}
			}
			out["chooser"], out["count"], out["items"] = chooser, n, labels
			out["found"] = found
			out["target"] = landed
			out["selected"] = strings.TrimSpace(selected)
			out["status"] = status
			return out, nil
		}
		if time.Now().After(deadline) {
			return out, fmt.Errorf("查找实现没有返回")
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}
