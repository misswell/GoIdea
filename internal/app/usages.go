package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

const (
	usageTimeout = 90 * time.Second
	maxUsages    = 2000

	// referenceRetryDelay is how long an empty answer waits before asking
	// again, long enough for the server to index the edit it just received.
	referenceRetryDelay = 300 * time.Millisecond
)

// UsagesPane is Find Usages: the symbol looked up, every place the backend says
// it is used, and whether the search is still streaming.
type UsagesPane struct {
	visible bool
	symbol  string
	channel uint32
	running bool
	done    bool
	// locs is the newest snapshot; streaming partials are cumulative.
	locs             []openidea.Location
	incomplete       string
	librariesSkipped bool
	truncated        bool
	err              string
}

func (m *Model) resetUsages() {
	m.cancelUsages()
	m.usages = UsagesPane{}
}

// showUsages opens the Find Usages window in the bottom side.
func (m *Model) showUsages() {
	m.usages.visible = true
	m.occupyBottom("usages")
	m.invalidate()
}

// FindUsages looks up every use of the symbol at the caret and shows the
// result in OpenIDEA's Find Usages window, which sits in the bottom side. The
// backend streams partial answers, so the list fills while JDT is still
// searching.
func (m *Model) FindUsages() {
	t := m.tab()
	if t == nil || !m.tabOpen(t) {
		return
	}
	off := t.state.Cursor()
	symbol := wordAt(t.doc.Text(), off)
	m.cancelUsages()
	ch := m.nextChannel()
	m.usages = UsagesPane{symbol: symbol, channel: ch, running: true}
	m.showUsages()
	m.status = "正在查找用法…"
	m.invalidate()
	docID := t.doc.ID()
	pos := t.doc.UTF16Pos(off)
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, usageTimeout)
		defer cancel()
		// The backend searches the text it was last given; typing since the
		// debounce sync would make the caret point at other text.
		if serr := t.doc.Sync(ctx); serr != nil {
			m.post(func() { m.finishUsages(ch, serr, nil) })
			return
		}
		ask := func() (openidea.References, error) {
			raw, err := m.mgr.Invoke(ctx, Window, "lsp_references", map[string]any{
				"request": ch, "doc": docID, "pos": lspPos(pos),
				"includeDeclaration": true, "projectWide": true, "libraries": false,
				"onPartial": map[string]any{"$channel": ch},
			})
			var refs openidea.References
			if err == nil {
				err = json.Unmarshal(raw, &refs)
			}
			return refs, err
		}
		refs, err := ask()
		// JDT applies the didChange just pushed in the background; its search
		// index catches up a moment later, so an empty answer right after an
		// edit can be premature.
		if err == nil && len(refs.Locations) == 0 {
			select {
			case <-ctx.Done():
			case <-time.After(referenceRetryDelay):
				refs, err = ask()
			}
		}
		m.post(func() { m.finishUsages(ch, err, &refs) })
	}()
}

// cancelUsages stops the running lookup; its final answer is then dropped.
func (m *Model) cancelUsages() {
	if !m.usages.running || m.usages.channel == 0 {
		return
	}
	id := m.usages.channel
	m.usages.running = false
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
		defer cancel()
		_, _ = m.mgr.Invoke(ctx, Window, "cancel_request", map[string]any{"id": id})
	}()
}

// acceptUsages consumes one streamed snapshot of the lookup.
func (m *Model) acceptUsages(ch uint32, data json.RawMessage) {
	if ch != m.usages.channel {
		return
	}
	var refs openidea.References
	if json.Unmarshal(data, &refs) != nil {
		return
	}
	m.setUsages(&refs)
	m.invalidate()
}

func (m *Model) finishUsages(ch uint32, err error, refs *openidea.References) {
	if ch != m.usages.channel {
		return
	}
	m.usages.running = false
	m.usages.done = true
	switch {
	case err != nil:
		if openidea.ErrorKind(err) == "cancelled" {
			m.status = "已停止查找用法"
			break
		}
		m.usages.err = "查找用法失败：" + err.Error()
		m.status = "查找用法失败"
	case refs != nil:
		m.setUsages(refs)
		switch {
		case len(m.usages.locs) == 0:
			m.status = "未找到用法"
		default:
			m.status = fmt.Sprintf("找到 %d 处用法", len(m.usages.locs))
		}
	}
	m.invalidate()
}

// setUsages replaces the list with a snapshot, keeping it bounded.
func (m *Model) setUsages(refs *openidea.References) {
	locs := refs.Locations
	m.usages.truncated = len(locs) > maxUsages
	if m.usages.truncated {
		locs = locs[:maxUsages]
	}
	m.usages.locs = locs
	m.usages.incomplete = ""
	if refs.Incomplete != nil {
		m.usages.incomplete = incompleteText(*refs.Incomplete)
	}
	m.usages.librariesSkipped = refs.LibrariesSkipped
}

// incompleteText names what a reference query may have missed.
func incompleteText(kind string) string {
	switch kind {
	case "indexing":
		return "语言服务器仍在索引，结果可能不完整"
	case "tooManyFiles":
		return "提及该名称的文件过多，只检查了一部分"
	case "filesSkipped":
		return "部分文件过大或无法读取，未检查"
	case "serverFailed":
		return "服务端查询失败，结果可能不完整"
	}
	if kind == "" {
		return ""
	}
	return "结果可能不完整（" + kind + "）"
}

// wordAt expands the identifier around off, the name the usages pane titles
// itself with; an offset on punctuation leaves it empty.
func wordAt(text string, off int) string {
	if off < 0 {
		off = 0
	}
	if off > len(text) {
		off = len(text)
	}
	isWord := func(b byte) bool {
		return b == '_' || b == '$' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' ||
			b >= 0x80
	}
	start, end := off, off
	for start > 0 && isWord(text[start-1]) {
		start--
	}
	for end < len(text) && isWord(text[end]) {
		end++
	}
	return text[start:end]
}

// usageFile groups the locations of one file, in the order the backend listed
// them.
type usageFile struct {
	path string
	locs []openidea.Location
}

func groupUsages(locs []openidea.Location) []usageFile {
	var files []usageFile
	index := map[string]int{}
	for _, loc := range locs {
		i, ok := index[loc.Path]
		if !ok {
			i = len(files)
			index[loc.Path] = i
			files = append(files, usageFile{path: loc.Path})
		}
		files[i].locs = append(files[i].locs, loc)
	}
	return files
}

// usagesRows draws the Find Usages pane: the symbol, warnings, then the
// locations grouped by file. Clicking a location opens it.
func (m *Model) usagesRows(c *ui.Context, p *ui.Theme) {
	u := &m.usages
	ui.Row(c).Padding(6, 8).Gap(6).AlignItems(ui.Center).Children(func() {
		name := u.symbol
		if name == "" {
			name = "当前符号"
		}
		ui.Text(c, name).FontSize(12).Bold().TextColor(p.Text).SingleLine().Ellipsis("…")
		ui.Spacer(c)
		text := fmt.Sprintf("%d 处", len(u.locs))
		if u.running {
			text = "查找中…"
		}
		if u.done && len(u.locs) == 0 && u.err == "" {
			text = "未找到"
		}
		ui.Text(c, text).FontSize(10.5).TextColor(p.TextMuted).SingleLine()
	})
	if u.err != "" {
		ui.Row(c).Padding(4, 8).Children(func() {
			ui.Text(c, u.err).FontSize(11).TextColor(p.Warning)
		})
	}
	if u.incomplete != "" {
		ui.Row(c).Padding(4, 8).Children(func() {
			ui.Text(c, u.incomplete).FontSize(10.5).TextColor(p.Warning)
		})
	}
	if u.librariesSkipped {
		ui.Row(c).Padding(4, 8).Children(func() {
			ui.Text(c, "未搜索依赖与 JDK，仅限项目源码").FontSize(10.5).TextColor(p.TextMuted)
		})
	}
	if u.truncated {
		ui.Row(c).Padding(4, 8).Children(func() {
			ui.Text(c, fmt.Sprintf("用法过多，仅显示前 %d 处", maxUsages)).FontSize(10.5).TextColor(p.TextMuted)
		})
	}
	if len(u.locs) == 0 {
		hint := "把光标放在符号上再查找"
		switch {
		case u.running:
			hint = "语言服务器正在查找…"
		case u.done:
			hint = "没有找到该符号的用法"
		}
		if u.err != "" {
			return
		}
		ui.Row(c).Padding(8, 8).Children(func() {
			ui.Text(c, hint).FontSize(11.5).TextColor(p.TextMuted)
		})
		return
	}
	root := m.Root()
	for fi, f := range groupUsages(u.locs) {
		rel := f.path
		if root != "" {
			if r, err := filepath.Rel(root, f.path); err == nil && !strings.HasPrefix(r, "..") {
				rel = r
			}
		}
		ui.Row(c).Height(22).Padding(0, 8).Gap(6).AlignItems(ui.Center).
			Children(func() {
				ui.Text(c, rel).FontSize(11.5).Bold().TextColor(p.Accent).
					SingleLine().Ellipsis("…")
				ui.Spacer(c)
				ui.Text(c, fmt.Sprintf("%d", len(f.locs))).FontSize(10.5).TextColor(p.TextMuted)
			})
		for li, loc := range f.locs {
			fi, li, loc := fi, li, loc
			r := ui.Row(c.Key(fmt.Sprintf("usage:%d:%d", fi, li))).Height(21).Padding(0, 8, 0, 18).
				Gap(6).AlignItems(ui.Center)
			if r.Hovered() {
				r.Background(p.SurfaceHover)
			}
			r.Children(func() {
				line := int(loc.Range.Start.Line) + 1
				col := int(loc.Range.Start.Col) + 1
				ui.Text(c, fmt.Sprintf("%d:%d", line, col)).FontSize(11).TextColor(p.Text).
					SingleLine()
				if strings.Contains(loc.Path, "://") {
					ui.Text(c, "外部依赖").FontSize(10.5).TextColor(p.TextMuted).SingleLine()
				}
			})
			if r.Clicked() {
				if strings.Contains(loc.Path, "://") {
					m.status = "用法在外部依赖中，无法作为文件打开"
					m.invalidate()
					continue
				}
				m.revealAt(loc.Path, int(loc.Range.Start.Line), int(loc.Range.Start.Col))
			}
		}
	}
}
