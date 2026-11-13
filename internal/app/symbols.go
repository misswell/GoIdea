package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

const (
	symbolTimeout = 60 * time.Second
	// maxSymbolResults bounds the picker; JDT answers a broad query with every
	// declaration in the project.
	maxSymbolResults = 200

	// symbolDebounce keeps a query from reaching the language server on every
	// keystroke; a newer one replaces the request before it is sent.
	symbolDebounce = 180 * time.Millisecond
)

// symbolPicker is the Go to Symbol prompt: the typed query, the declarations
// the backend matched, and the row Enter opens.
type symbolPicker struct {
	open  bool
	query string
	items []openidea.WorkspaceSymbol
	sel   int
	busy  bool
	done  bool
	err   string

	truncated bool
	// seq marks the newest query; a reply to an older one updates nothing.
	seq     int
	request uint32
}

// OpenSymbols opens the symbol picker over the window.
func (m *Model) OpenSymbols() {
	if m.symbols != nil {
		return
	}
	m.symbols = &symbolPicker{open: true}
	m.invalidate()
}

// closeSymbols drops the picker and cancels a lookup still in flight.
func (m *Model) closeSymbols() {
	if m.symbols == nil {
		return
	}
	m.cancelSymbolQuery()
	m.symbols = nil
	m.invalidate()
}

// querySymbols asks the backend for the symbols matching the typed text. The
// answer to a query the user has typed past is dropped.
func (m *Model) querySymbols() {
	s := m.symbols
	if s == nil {
		return
	}
	s.seq++
	seq := s.seq
	query := strings.TrimSpace(s.query)
	m.cancelSymbolQuery()
	s.err = ""
	s.busy = false
	s.done = false
	if query == "" {
		s.items = nil
		s.sel = 0
		s.truncated = false
		m.invalidate()
		return
	}
	req := m.nextRequest()
	s.request = req
	s.busy = true
	m.invalidate()
	go func() {
		select {
		case <-m.ctx.Done():
			return
		case <-time.After(symbolDebounce):
		}
		var stale bool
		mygo.RunOnMain(func() { stale = m.symbols != s || s.seq != seq })
		if stale {
			return
		}
		ctx, cancel := context.WithTimeout(m.ctx, symbolTimeout)
		defer cancel()
		raw, err := m.mgr.Invoke(ctx, Window, "lsp_workspace_symbols", map[string]any{
			"request": req, "query": query,
		})
		var syms []openidea.WorkspaceSymbol
		if err == nil {
			err = json.Unmarshal(raw, &syms)
		}
		m.post(func() {
			if m.symbols != s || s.seq != seq {
				return
			}
			s.busy = false
			s.request = 0
			s.done = true
			if err != nil {
				// A cancelled lookup was replaced by a newer query, whose
				// answer is the one that lands.
				if openidea.ErrorKind(err) != "cancelled" {
					s.err = "查找符号失败：" + err.Error()
				}
				s.items = nil
				s.sel = 0
				m.invalidate()
				return
			}
			s.truncated = len(syms) > maxSymbolResults
			if s.truncated {
				syms = syms[:maxSymbolResults]
			}
			s.items = syms
			s.sel = 0
			m.invalidate()
		})
	}()
}

// cancelSymbolQuery stops the lookup in flight, whose answer then is ignored.
func (m *Model) cancelSymbolQuery() {
	s := m.symbols
	if s == nil || s.request == 0 {
		return
	}
	id := s.request
	s.request = 0
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
		defer cancel()
		_, _ = m.mgr.Invoke(ctx, Window, "cancel_request", map[string]any{"id": id})
	}()
}

// stepSymbol moves the chosen row by delta, stopping at the ends.
func (m *Model) stepSymbol(delta int) {
	s := m.symbols
	if s == nil || len(s.items) == 0 {
		return
	}
	s.sel = clampSymbolSel(s.sel+delta, len(s.items))
	m.invalidate()
}

func clampSymbolSel(sel, n int) int {
	if n <= 0 {
		return 0
	}
	if sel < 0 {
		return 0
	}
	if sel >= n {
		return n - 1
	}
	return sel
}

// openSelectedSymbol opens the chosen declaration.
func (m *Model) openSelectedSymbol() {
	s := m.symbols
	if s == nil {
		return
	}
	if len(s.items) == 0 {
		if strings.TrimSpace(s.query) == "" {
			s.err = "输入名称搜索项目里的符号"
		}
		m.invalidate()
		return
	}
	m.applySymbol(s.items[clampSymbolSel(s.sel, len(s.items))])
}

// applySymbol closes the picker and reveals a declaration's file and line.
func (m *Model) applySymbol(sym openidea.WorkspaceSymbol) {
	m.closeSymbols()
	loc := sym.Location
	if strings.Contains(loc.Path, "://") {
		m.status = "符号位于外部依赖，无法作为文件打开"
		m.invalidate()
		return
	}
	m.status = ""
	m.revealAt(loc.Path, int(loc.Range.Start.Line), int(loc.Range.Start.Col))
	m.invalidate()
}

// symbolKindLabel names a symbol kind the way IDEA's symbol chooser does.
func symbolKindLabel(kind string) string {
	switch kind {
	case "class":
		return "类"
	case "interface":
		return "接口"
	case "struct":
		return "结构"
	case "enum":
		return "枚举"
	case "enumMember":
		return "枚举项"
	case "method":
		return "方法"
	case "function":
		return "函数"
	case "constructor":
		return "构造器"
	case "field":
		return "字段"
	case "property":
		return "属性"
	case "variable":
		return "变量"
	case "constant":
		return "常量"
	case "package":
		return "包"
	case "module":
		return "模块"
	case "namespace":
		return "命名空间"
	case "trait":
		return "特征"
	case "typeParameter":
		return "类型参数"
	case "typeAlias":
		return "类型别名"
	case "file":
		return "文件"
	case "event":
		return "事件"
	case "operator":
		return "运算符"
	case "key":
		return "键"
	}
	return "符号"
}

// symbolWhere is a result's relative file and zero-based line, as the picker
// shows it.
func symbolWhere(root, path string, line int) string {
	rel := path
	if root != "" {
		if r, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(r, "..") {
			rel = r
		}
	}
	return fmt.Sprintf("%s:%d", rel, line+1)
}

// symbolsDialog draws the Go to Symbol prompt and its results.
func (m *Model) symbolsDialog(c *ui.Context, p *ui.Theme) {
	s := m.symbols
	if s == nil {
		return
	}
	open := s.open
	d := ui.Modal(c, &open, func() {
		ui.Column(c).Width(600).Gap(8).Children(func() {
			ui.Text(c, "转到符号").FontSize(13).Bold().TextColor(p.Text).SingleLine()
			in := ui.TextInput(c, &s.query).AutoFocus()
			in.OnChange(func() { m.querySymbols() })
			in.OnShortcut(0, ui.KeyDown, func() { m.stepSymbol(1) })
			in.OnShortcut(0, ui.KeyUp, func() { m.stepSymbol(-1) })
			if in.Submitted() {
				m.openSelectedSymbol()
			}
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				text := "输入类型或成员名称，Enter 打开"
				switch {
				case s.busy:
					text = "正在查找…"
				case s.err != "":
					text = ""
				case s.done && len(s.items) == 0:
					text = "没有匹配的符号"
				case len(s.items) > 0:
					text = fmt.Sprintf("%d 个结果", len(s.items))
					if s.truncated {
						text = fmt.Sprintf("结果过多，仅显示前 %d 个", maxSymbolResults)
					}
				}
				if text != "" {
					ui.Text(c, text).FontSize(10.5).TextColor(p.TextMuted).SingleLine()
				}
			})
			if s.err != "" {
				ui.Text(c, s.err).FontSize(11).TextColor(p.Danger).SingleLine().Ellipsis("…")
			}
			if len(s.items) == 0 {
				return
			}
			root := m.Root()
			ui.Scroll(c).Height(330).Children(func() {
				ui.Column(c).Children(func() {
					for i, sym := range s.items {
						i, sym := i, sym
						r := ui.Row(c.Key(fmt.Sprintf("symbol:%d", i))).Height(24).Padding(0, 8).
							Gap(8).AlignItems(ui.Center)
						if i == s.sel {
							r.Background(p.SurfaceHover)
						} else if r.Hovered() {
							r.Background(p.SurfaceHover)
						}
						r.Children(func() {
							ui.Text(c, sym.Name).FontSize(12).Bold().TextColor(p.Text).
								SingleLine()
							ui.Text(c, symbolKindLabel(sym.Kind)).FontSize(10.5).
								TextColor(p.TextMuted).SingleLine()
							if sym.Container != nil && *sym.Container != "" {
								ui.Text(c, *sym.Container).FontSize(10.5).
									TextColor(p.TextMuted).SingleLine().Ellipsis("…")
							}
							ui.Spacer(c)
							line := int(sym.Location.Range.Start.Line)
							ui.Text(c, symbolWhere(root, sym.Location.Path, line)).FontSize(10.5).
								TextColor(p.Accent).SingleLine().Ellipsis("…")
						})
						if r.Clicked() {
							m.applySymbol(sym)
						}
					}
				})
			})
		})
	})
	if d.Dismissed() || !open {
		m.closeSymbols()
	}
}
