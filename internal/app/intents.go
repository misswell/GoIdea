package app

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
)

// intentTimeout bounds one code action query or apply; a quickfix resolves
// against the whole project, which a cold index makes slow.
const intentTimeout = 60 * time.Second

// intentAction is one action the servers offered at the caret or selection.
// raw goes back to lsp_apply_code_action unchanged, so the resolve data the
// server sent with it is still there.
type intentAction struct {
	raw        json.RawMessage
	title      string
	kind       string
	preferred  bool
	disabled   string
	dependency *dependencyIntentEdit
	run        func(*Model)
	priority   int
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
	title string

	filter              func(intentAction) bool
	priority            func(intentAction) int
	clientItems         []intentAction
	emptyText           string
	only                []string
	autoApplySingle     bool
	dependenciesAllowed bool

	request uint32
	depReq  uint32

	// lspItems and dependencies arrive independently. items is their stable
	// merged view so a fast Cargo answer cannot erase the language server's.
	lspItems     []intentAction
	dependencies []intentAction
}

// ShowIntentions opens the intentions prompt for the caret or selection.
func (m *Model) ShowIntentions() {
	m.showIntentionsPane(intentionPaneOptions{
		title:               "Context Actions",
		dependenciesAllowed: true,
	})
}

type intentionPaneOptions struct {
	title               string
	emptyText           string
	only                []string
	filter              func(intentAction) bool
	priority            func(intentAction) int
	clientItems         []intentAction
	autoApplySingle     bool
	dependenciesAllowed bool
}

func (m *Model) showIntentionsPane(options intentionPaneOptions) {
	t := m.tab()
	if t == nil || !m.tabOpen(t) || m.intents != nil {
		return
	}
	if m.ctx == nil || m.mgr == nil && m.invokeHook == nil {
		m.status = "Context actions require the GoIDEA backend"
		m.invalidate()
		return
	}
	sel := t.state.Selection()
	start := t.doc.UTF16Pos(sel.Start())
	end := t.doc.UTF16Pos(sel.End())
	p := &intentPane{
		open: true, tab: t, busy: true, title: options.title,
		filter: options.filter, priority: options.priority,
		clientItems: append([]intentAction(nil), options.clientItems...),
		emptyText:   options.emptyText, only: append([]string(nil), options.only...),
		autoApplySingle:     options.autoApplySingle,
		dependenciesAllowed: options.dependenciesAllowed,
	}
	p.list.Selected = &p.sel
	m.intents = p
	m.status = "Finding context actions…"
	m.invalidate()
	m.queryIntentions(p, start, end)
	if p.dependenciesAllowed {
		m.queryDependencyIntentions(p, sel.Start())
	}
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
		only := p.only
		if only == nil {
			only = []string{}
		}
		raw, err := m.invoke(ctx, "lsp_code_action_query", map[string]any{
			"request": req, "doc": docID,
			"range": map[string]any{"start": lspPos(start), "end": lspPos(end)},
			"only":  only,
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
			p.err = "Finding context actions failed: " + err.Error()
		}
		m.invalidate()
		return
	}
	p.lspItems = parseIntentActions(q.Actions)
	if p.filter != nil {
		filtered := p.lspItems[:0]
		for _, action := range p.lspItems {
			if p.filter(action) {
				filtered = append(filtered, action)
			}
		}
		p.lspItems = filtered
	}
	p.lspItems = append(p.lspItems, p.clientItems...)
	if p.priority != nil {
		sort.SliceStable(p.lspItems, func(i, j int) bool {
			left, right := p.priority(p.lspItems[i]), p.priority(p.lspItems[j])
			if left != right {
				return left < right
			}
			return p.lspItems[i].priority < p.lspItems[j].priority
		})
	}
	m.rebuildIntentions(p)
	if len(p.items) == 0 && p.emptyText != "" {
		m.intents = nil
		m.status = p.emptyText
		m.invalidate()
		return
	}
	if p.autoApplySingle && len(p.items) == 1 {
		m.applyIntent(p.items[0])
		return
	}
	m.invalidate()
}

// queryDependencyIntentions adds OpenIDEA's registry-backed Cargo update to
// the language-server actions. Registry failures stay quiet, as they do in
// the upstream editor contribution.
func (m *Model) queryDependencyIntentions(p *intentPane, selection int) {
	t := p.tab
	if m.mgr == nil || !isCargoManifest(t.doc.Path()) || t.doc.ReadOnly() {
		return
	}
	doc := t.doc
	docID := doc.ID()
	req := m.nextRequest()
	p.depReq = req
	text := doc.Buffer().Snapshot().String()
	offset := byteToUTF16Offset(text, selection)
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, intentTimeout)
		defer cancel()
		if err := doc.Sync(ctx); err != nil {
			m.post(func() {
				if p == m.intents && p.depReq == req {
					p.depReq = 0
				}
			})
			return
		}
		found, err := m.mgr.DependencyVersions(ctx, Window, req, docID, &offset)
		m.post(func() {
			if p != m.intents || p.depReq != req {
				return
			}
			p.depReq = 0
			if err != nil {
				return
			}
			snapshot := doc.Buffer().Snapshot()
			if snapshot.String() != text {
				return
			}
			p.dependencies = dependencyIntentItems(
				text,
				selection,
				snapshot.Version(),
				found,
			)
			m.rebuildIntentions(p)
			m.invalidate()
		})
	}()
}

func (m *Model) rebuildIntentions(p *intentPane) {
	previous := ""
	if len(p.items) > 0 && p.sel >= 0 && p.sel < len(p.items) {
		previous = p.items[p.sel].title
	}
	p.items = append(append([]intentAction(nil), p.lspItems...), p.dependencies...)
	p.sel = 0
	if previous != "" {
		for i := range p.items {
			if p.items[i].title == previous && p.items[i].disabled == "" {
				p.sel = i
				break
			}
		}
	}
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
	if it.run != nil {
		m.intents = nil
		it.run(m)
		return
	}
	if it.dependency != nil {
		source := *it.dependency
		go func() {
			ctx, cancel := context.WithTimeout(m.ctx, intentTimeout)
			defer cancel()
			err := t.doc.ApplyEditsIfVersion(ctx, source.version, editor.Edit{
				Start: source.from, End: source.to, Text: source.replacement,
			})
			m.post(func() {
				if p != m.intents {
					return
				}
				p.busy = false
				if errors.Is(err, docbridge.ErrChanged) {
					p.err = "The dependency changed before the update was applied."
				} else if err != nil {
					p.err = "Applying the dependency update failed: " + err.Error()
				} else {
					m.intents = nil
					m.status = "Updated dependency to " + source.latest
				}
				m.invalidate()
			})
		}()
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, intentTimeout)
		defer cancel()
		_, err := m.invoke(ctx, "lsp_apply_code_action", map[string]any{
			"doc": docID, "action": it.raw,
		})
		m.post(func() {
			if p != m.intents {
				return
			}
			p.busy = false
			if err != nil {
				p.err = "Applying the context action failed: " + err.Error()
				m.invalidate()
				return
			}
			m.intents = nil
			m.status = "Applied: " + it.title
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
	if p == nil || p.request == 0 && p.depReq == 0 {
		return
	}
	ids := []uint32{p.request, p.depReq}
	p.request = 0
	p.depReq = 0
	if m.ctx == nil || m.mgr == nil && m.invokeHook == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
		defer cancel()
		for _, id := range ids {
			if id != 0 {
				_, _ = m.invoke(ctx, "cancel_request", map[string]any{"id": id})
			}
		}
	}()
}

// intentKindLabel names an action's kind the way IDEA groups intentions.
func intentKindLabel(kind string) string {
	switch {
	case kind == "":
		return "Action"
	case kind == "quickfix" || strings.HasPrefix(kind, "quickfix."):
		return "Quick Fix"
	case kind == "refactor" || strings.HasPrefix(kind, "refactor."):
		switch {
		case strings.HasPrefix(kind, "refactor.extract"):
			return "Extract"
		case strings.HasPrefix(kind, "refactor.inline"):
			return "Inline"
		case strings.HasPrefix(kind, "refactor.move"):
			return "Move"
		case strings.HasPrefix(kind, "refactor.rewrite"):
			return "Rewrite"
		}
		return "Refactor"
	case kind == "source" || strings.HasPrefix(kind, "source."):
		switch {
		case strings.HasPrefix(kind, "source.organizeImports"):
			return "Optimize Imports"
		case strings.HasPrefix(kind, "source.fixAll"):
			return "Fix All"
		}
		return "Source"
	}
	return "Action"
}

// intentionsDialog draws the actions offered at the caret.
func (m *Model) intentionsDialog(c *ui.Context, p *ui.Theme) {
	pn := m.intents
	if pn == nil {
		return
	}
	open := pn.open
	d := nativeModal(c, &open, func() {
		ui.Column(c).Width(460).Gap(8).Children(func() {
			title := pn.title
			if title == "" {
				title = "Context Actions"
			}
			ui.Text(c, title).FontSize(13).Bold().TextColor(p.Text).SingleLine()
			hint := count(len(pn.items), "action", "actions")
			switch {
			case pn.busy:
				hint = "Searching…"
			case pn.err != "":
				hint = ""
			case len(pn.items) == 0:
				hint = "No context actions are available here"
			}
			if hint != "" {
				ui.Text(c, hint).FontSize(12).TextColor(p.TextMuted).SingleLine()
			}
			if pn.err != "" {
				ui.Text(c, pn.err).FontSize(12).TextColor(p.Danger).SingleLine().Ellipsis("…")
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
						ui.Text(c, "Suggested").FontSize(12).TextColor(p.Accent).SingleLine()
					}
					ui.Spacer(c)
					right := intentKindLabel(it.kind)
					if it.disabled != "" {
						right = it.disabled
					}
					ui.Text(c, right).FontSize(12).TextColor(p.TextMuted).
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
