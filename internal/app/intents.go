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

// intentTimeout bounds one code action query or apply; a quickfix resolves
// against the whole project, which a cold index makes slow.
const intentTimeout = 60 * time.Second

// intentAction is one action the servers offered at the caret or selection.
// raw goes back to lsp_apply_code_action unchanged, so the resolve data the
// server sent with it is still there.
type intentAction struct {
	raw       json.RawMessage
	title     string
	kind      string
	preferred bool
	disabled  string
}

// intentPane is the intentions prompt: the actions offered where the caret
// was, and the row Enter applies.
type intentPane struct {
	open  bool
	tab   *Tab
	items []intentAction
	sel   int
	list  ui.ListState
	busy  bool
	err   string

	request uint32
}

// ShowIntentions opens the intentions prompt for the caret or selection.
func (m *Model) ShowIntentions() {
	t := m.tab()
	if t == nil || !m.tabOpen(t) || m.intents != nil {
		return
	}
	sel := t.state.Selection()
	start := t.doc.UTF16Pos(sel.Start())
	end := t.doc.UTF16Pos(sel.End())
	p := &intentPane{open: true, tab: t, busy: true}
	p.list.Selected = &p.sel
	m.intents = p
	m.status = "正在查找意图操作…"
	m.invalidate()
	m.queryIntentions(p, start, end)
}

// queryIntentions asks the servers what they offer at a range.
func (m *Model) queryIntentions(p *intentPane, start, end openidea.OutlinePos) {
	t := p.tab
	docID := t.doc.ID()
	req := m.nextRequest()
	p.request = req
	p.busy = true
	p.err = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, intentTimeout)
		defer cancel()
		// The servers answer for the text the backend was last given; typing
		// since the debounce sync would make the range point at other text.
		if serr := t.doc.Sync(ctx); serr != nil {
			m.post(func() { m.finishIntentions(p, req, nil, serr) })
			return
		}
		raw, err := m.mgr.Invoke(ctx, Window, "lsp_code_action_query", map[string]any{
			"request": req, "doc": docID,
			"range": map[string]any{"start": lspPos(start), "end": lspPos(end)},
			"only":  []string{},
		})
		var q openidea.CodeActionQuery
		if err == nil {
			err = json.Unmarshal(raw, &q)
		}
		m.post(func() { m.finishIntentions(p, req, &q, err) })
	}()
}

func (m *Model) finishIntentions(p *intentPane, req uint32, q *openidea.CodeActionQuery, err error) {
	if p != m.intents || p.request != req {
		return
	}
	p.request = 0
	p.busy = false
	m.status = ""
	if err != nil {
		// A cancelled query was closed or replaced, and nothing shows it.
		if openidea.ErrorKind(err) != "cancelled" {
			p.err = "查找意图操作失败：" + err.Error()
		}
		m.invalidate()
		return
	}
	p.items = parseIntentActions(q.Actions)
	p.sel = 0
	m.invalidate()
}

// parseIntentActions keeps the fields the prompt shows from each raw action.
func parseIntentActions(raw []json.RawMessage) []intentAction {
	items := make([]intentAction, 0, len(raw))
	for _, r := range raw {
		var a struct {
			Title          string  `json:"title"`
			Kind           *string `json:"kind"`
			IsPreferred    bool    `json:"isPreferred"`
			DisabledReason *string `json:"disabledReason"`
		}
		if json.Unmarshal(r, &a) != nil || strings.TrimSpace(a.Title) == "" {
			continue
		}
		it := intentAction{raw: r, title: a.Title, preferred: a.IsPreferred}
		if a.Kind != nil {
			it.kind = *a.Kind
		}
		if a.DisabledReason != nil {
			it.disabled = *a.DisabledReason
		}
		items = append(items, it)
	}
	return items
}

// applyIntent resolves and applies one action; its edits reach the buffer as
// backend changes.
func (m *Model) applyIntent(it intentAction) {
	p := m.intents
	if p == nil || p.busy || it.disabled != "" {
		return
	}
	t := p.tab
	if !m.tabOpen(t) {
		m.closeIntents()
		return
	}
	docID := t.doc.ID()
	p.busy = true
	p.err = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, intentTimeout)
		defer cancel()
		_, err := m.mgr.Invoke(ctx, Window, "lsp_apply_code_action", map[string]any{
			"doc": docID, "action": it.raw,
		})
		m.post(func() {
			if p != m.intents {
				return
			}
			p.busy = false
			if err != nil {
				p.err = "应用意图操作失败：" + err.Error()
				m.invalidate()
				return
			}
			m.intents = nil
			m.status = "已应用：" + it.title
			m.invalidate()
		})
	}()
}

// applySelectedIntent applies the row Enter chose.
func (m *Model) applySelectedIntent() {
	p := m.intents
	if p == nil || p.busy || len(p.items) == 0 {
		return
	}
	m.applyIntent(p.items[clampSymbolSel(p.sel, len(p.items))])
}

// closeIntents drops the prompt and cancels a query still in flight.
func (m *Model) closeIntents() {
	if m.intents == nil {
		return
	}
	m.cancelIntentQuery()
	m.intents = nil
	m.invalidate()
}

func (m *Model) cancelIntentQuery() {
	p := m.intents
	if p == nil || p.request == 0 {
		return
	}
	id := p.request
	p.request = 0
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
		defer cancel()
		_, _ = m.mgr.Invoke(ctx, Window, "cancel_request", map[string]any{"id": id})
	}()
}

// intentKindLabel names an action's kind the way IDEA groups intentions.
func intentKindLabel(kind string) string {
	switch {
	case kind == "":
		return "操作"
	case kind == "quickfix" || strings.HasPrefix(kind, "quickfix."):
		return "快速修复"
	case kind == "refactor" || strings.HasPrefix(kind, "refactor."):
		switch {
		case strings.HasPrefix(kind, "refactor.extract"):
			return "提取"
		case strings.HasPrefix(kind, "refactor.inline"):
			return "内联"
		case strings.HasPrefix(kind, "refactor.move"):
			return "移动"
		case strings.HasPrefix(kind, "refactor.rewrite"):
			return "改写"
		}
		return "重构"
	case kind == "source" || strings.HasPrefix(kind, "source."):
		switch {
		case strings.HasPrefix(kind, "source.organizeImports"):
			return "整理导入"
		case strings.HasPrefix(kind, "source.fixAll"):
			return "修复全部"
		}
		return "源码"
	}
	return "操作"
}

// intentionsDialog draws the actions offered at the caret.
func (m *Model) intentionsDialog(c *ui.Context, p *ui.Theme) {
	pn := m.intents
	if pn == nil {
		return
	}
	open := pn.open
	d := ui.Modal(c, &open, func() {
		ui.Column(c).Width(460).Gap(8).Children(func() {
			ui.Text(c, "意图操作").FontSize(13).Bold().TextColor(p.Text).SingleLine()
			hint := fmt.Sprintf("%d 个操作", len(pn.items))
			switch {
			case pn.busy:
				hint = "正在查找…"
			case pn.err != "":
				hint = ""
			case len(pn.items) == 0:
				hint = "此处没有可用的意图操作"
			}
			if hint != "" {
				ui.Text(c, hint).FontSize(10.5).TextColor(p.TextMuted).SingleLine()
			}
			if pn.err != "" {
				ui.Text(c, pn.err).FontSize(11).TextColor(p.Danger).SingleLine().Ellipsis("…")
			}
			if len(pn.items) == 0 {
				return
			}
			list := ui.List(c, &pn.list, len(pn.items), func(i int) {
				it := pn.items[i]
				r := ui.Row(c).Height(26).Padding(0, 10).Gap(8).AlignItems(ui.Center)
				r.Children(func() {
					color := p.Text
					if it.disabled != "" {
						color = p.TextMuted
					}
					ui.Text(c, it.title).FontSize(12).Bold().TextColor(color).
						SingleLine().Ellipsis("…")
					if it.preferred && it.disabled == "" {
						ui.Text(c, "建议").FontSize(10).TextColor(p.Accent).SingleLine()
					}
					ui.Spacer(c)
					right := intentKindLabel(it.kind)
					if it.disabled != "" {
						right = it.disabled
					}
					ui.Text(c, right).FontSize(10.5).TextColor(p.TextMuted).
						SingleLine().Ellipsis("…")
				})
				if r.Clicked() {
					m.applyIntent(it)
				}
			}).Height(300).AutoFocus()
			if list.Submitted() {
				m.applySelectedIntent()
			}
		})
	})
	if d.Dismissed() || !open {
		m.closeIntents()
	}
}
