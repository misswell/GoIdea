package app

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

func (m *Model) httpClientPanel(c *ui.Context, p *ui.Theme) {
	m.ensureHTTPClient()
	oi := oiPaletteOf(p)
	ui.Column(c.Key("http-client:view")).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
		m.httpClientToolbar(c, p)
		if m.http.historyErr != "" {
			ui.Row(c).Shrink(0).Padding(6, 10).Gap(8).AlignItems(ui.Center).
				Background(p.Danger.Mix(p.Surface, 0.9)).Children(func() {
				oiIconEl(c, "status/error", 16, oi, p.Danger)
				ui.Text(c, m.http.historyErr).FontSize(12).TextColor(p.Danger).Grow(1).SingleLine().Ellipsis("…")
				ui.Button(c, "Retry").OnClick(func() {
					m.http.historyErr = ""
					m.loadHTTPHistory(true)
				})
			})
		}
		if m.http.running != nil {
			m.httpClientRunning(c, p)
			return
		}
		if m.http.historyLoading && m.http.history == nil {
			ui.Column(c).Grow(1).AlignItems(ui.Center).Justify(ui.Center).Gap(8).Children(func() {
				ui.Text(c, "Loading HTTP Client history…").FontSize(13).TextColor(oi.textMuted)
			})
			return
		}
		if len(m.http.history) == 0 {
			ui.Column(c).Grow(1).AlignItems(ui.Center).Justify(ui.Center).Gap(8).Children(func() {
				ui.Text(c, "No requests yet").FontSize(13).Bold().TextColor(oi.text)
				ui.Text(c, "Run a request from an .http file with the editor shortcut.").
					FontSize(12).TextColor(oi.textMuted)
			})
			return
		}
		ui.Row(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
			ui.Column(c).Width(390).Shrink(0).MinWidth(240).MinHeight(0).
				BorderWidth(0, 0, 0, 1).BorderColor(oi.border).Children(func() {
				m.httpHistoryList(c, p)
			})
			ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
				m.httpExchange(c, p)
			})
		})
	})
}

func (m *Model) httpClientToolbar(c *ui.Context, p *ui.Theme) {
	oi := oiPaletteOf(p)
	tab := m.tab()
	httpTab := tab != nil && tab.doc != nil && isHTTPRequestFile(tab.doc.Path())
	ui.Row(c.Key("http-client:toolbar")).Height(32).Shrink(0).Padding(0, 8).Gap(7).
		AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(oi.border).
		Children(func() {
			if httpTab {
				m.loadHTTPEnvironments(tab.doc.Path())
				path := cleanHTTPPath(tab.doc.Path())
				selected := m.http.envByFile[path]
				label := "No Environment"
				if selected != "" {
					label = "Environment: " + selected
				}
				ui.MenuButton(c, label, func(menu *ui.Menu) {
					if menu.Item("No Environment").Chosen() {
						m.setHTTPEnvironment(path, "")
					}
					for _, environment := range m.http.environments[path] {
						environment := environment
						item := menu.Item(environment)
						if environment == selected {
							item.Checked(true)
						}
						if item.Chosen() {
							m.setHTTPEnvironment(path, environment)
						}
					}
					menu.Separator()
					if menu.Item("Add Environment to Public File…").Chosen() {
						m.OpenHTTPEnvironmentFile(false)
					}
					if menu.Item("Add Environment to Private File…").Chosen() {
						m.OpenHTTPEnvironmentFile(true)
					}
				})
				ui.Button(c, "Run All").OnClick(m.RunAllHTTPRequests)
				ui.Button(c, "Copy as cURL").OnClick(m.CopyHTTPRequestAsCurl)
			}
			ui.Button(c, "Run Request").Disabled(!httpTab).OnClick(m.RunHTTPRequestAtCaret)
			ui.Spacer(c)
			stop := ui.Button(c, "Stop")
			stop.Disabled(m.http.running == nil)
			stop.OnClick(func() { m.cancelHTTPRequest(true) })
			ui.Button(c, "Refresh").OnClick(func() { m.loadHTTPHistory(true) })
			clear := ui.Button(c, "Clear All")
			clear.Disabled(len(m.http.history) == 0)
			clear.OnClick(m.clearHTTPHistory)
		})
}

func (m *Model) httpClientRunning(c *ui.Context, p *ui.Theme) {
	run := m.http.running
	oi := oiPaletteOf(p)
	ui.Column(c).Grow(1).AlignItems(ui.Center).Justify(ui.Center).Gap(9).Children(func() {
		ui.Text(c, fmt.Sprintf("Sending request #%d…", run.index+1)).
			FontSize(13).Bold().TextColor(oi.text)
		if run.method != "" || run.url != "" {
			ui.Text(c, strings.TrimSpace(run.method+" "+run.url)).FontSize(12).
				TextColor(oi.textSecondary).SingleLine().Ellipsis("…")
		}
		ui.Button(c, "Stop Request").OnClick(func() { m.cancelHTTPRequest(true) })
	})
}

func (m *Model) httpHistoryList(c *ui.Context, p *ui.Theme) {
	oi := oiPaletteOf(p)
	ui.Row(c).Height(28).Shrink(0).Padding(0, 8).Gap(6).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(oi.border).Children(func() {
		ui.Text(c, "Response History").FontSize(12).Bold().TextColor(oi.text)
		ui.Spacer(c)
		ui.Text(c, fmt.Sprintf("%d", len(m.http.history))).FontSize(11).TextColor(oi.textMuted)
	})
	ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Padding(3, 4).Gap(1).Children(func() {
			for _, entry := range m.http.history {
				entry := entry
				selected := entry.ID == m.http.selected
				row := ui.Row(c.Key("http-history:"+entry.ID)).Height(28).Padding(5, 7).
					Gap(7).Radius(5).AlignItems(ui.Center).Cursor(ui.CursorPointer)
				if selected {
					row.Background(oi.selected)
				} else if row.Hovered() {
					row.Background(oi.hoverTint)
				}
				if row.Clicked() {
					m.selectHTTPHistory(entry.ID)
				}
				row.ContextMenu(func(menu *ui.Menu) {
					if entry.File != nil && menu.Item("Jump to Source").Chosen() {
						m.openFile(*entry.File)
					}
					if menu.Item("Copy URL").Chosen() {
						mygo.Clipboard.WriteText(entry.URL)
					}
				})
				row.Children(func() {
					tone := oi.success
					if entry.Status == nil || *entry.Status >= 400 {
						tone = oi.danger
					} else if *entry.Status >= 300 {
						tone = oi.warning
					}
					ui.Text(c, httpStatusLabel(entry)).FontSize(11).Bold().TextColor(tone).Width(32)
					ui.Text(c, entry.Method).FontSize(11).Bold().TextColor(oi.textSecondary).Width(44)
					ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
						ui.Text(c, httpHistoryTitle(entry)).FontSize(12).TextColor(oi.text).
							SingleLine().Ellipsis("…")
						if line := httpHistoryError(entry); line != "" {
							ui.Text(c, line).FontSize(10).TextColor(oi.danger).SingleLine().Ellipsis("…")
						} else {
							ui.Text(c, entry.URL).FontSize(10).TextColor(oi.textMuted).
								SingleLine().Ellipsis("…")
						}
					})
					ui.Text(c, httpHistoryTime(entry.TimeMs)).FontSize(10).TextColor(oi.textMuted)
				})
			}
		})
	})
}

func (m *Model) httpExchange(c *ui.Context, p *ui.Theme) {
	oi := oiPaletteOf(p)
	if m.http.exchangeBusy {
		ui.Column(c).Grow(1).AlignItems(ui.Center).Justify(ui.Center).
			Children(func() { ui.Text(c, "Loading response…").FontSize(12).TextColor(oi.textMuted) })
		return
	}
	if m.http.exchangeErr != "" {
		ui.Column(c).Grow(1).Padding(16).Gap(6).Children(func() {
			ui.Text(c, "Cannot open the response").FontSize(13).Bold().TextColor(oi.danger)
			ui.Text(c, m.http.exchangeErr).FontSize(12).TextColor(oi.textMuted)
		})
		return
	}
	ex := m.http.exchange
	if ex == nil {
		ui.Column(c).Grow(1).AlignItems(ui.Center).Justify(ui.Center).
			Children(func() { ui.Text(c, "No response selected").FontSize(12).TextColor(oi.textMuted) })
		return
	}
	status := "No response"
	color := oi.danger
	meta := []string{}
	if response := ex.Response; response != nil {
		status = fmt.Sprintf("%d %s", response.Status, response.StatusText)
		if response.Status < 300 {
			color = oi.success
		} else if response.Status < 400 {
			color = oi.warning
		}
		meta = append(meta, response.Version, formatHTTPDuration(response.TotalMs), formatHTTPBytes(response.Size))
	} else if ex.Entry.Error != nil {
		status = *ex.Entry.Error
	}
	ui.Row(c.Key("http-exchange:status")).Shrink(0).Padding(7, 10).Gap(9).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(oi.border).Children(func() {
		ui.Text(c, status).FontSize(12).Bold().TextColor(color).SingleLine().Ellipsis("…")
		for _, value := range meta {
			ui.Text(c, value).FontSize(11).TextColor(oi.textMuted)
		}
		ui.Spacer(c)
		ui.Text(c, ex.Request.Method+" "+ex.Request.URL).FontSize(11).TextColor(oi.textSecondary).
			SingleLine().Ellipsis("…")
	})
	ui.Row(c).Height(28).Shrink(0).Padding(0, 6).Gap(2).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(oi.border).Children(func() {
		for _, item := range []struct {
			id, label string
		}{
			{"response", "Response"},
			{"headers", fmt.Sprintf("Headers (%d)", len(httpResponseHeaders(ex)))},
			{"request", "Request"},
		} {
			item := item
			button := ui.ButtonBase(c.Key("http-detail-tab:"+item.id)).Height(24).
				Padding(0, 8).Radius(4).Cursor(ui.CursorPointer)
			if m.http.detailTab == item.id {
				button.Background(oi.selected)
			} else if button.Hovered() {
				button.Background(oi.hoverTint)
			}
			button.OnClick(func() { m.selectHTTPDetailTab(item.id) })
			button.Children(func() {
				ui.Text(c, item.label).FontSize(12).TextColor(oi.text).SingleLine()
			})
		}
		ui.Spacer(c)
		if m.http.detailTab == "response" && ex.Response != nil && ex.Response.Body != nil {
			label := "Format"
			if m.http.rawBody {
				label = "Raw"
			}
			button := ui.Button(c, label)
			button.OnClick(func() {
				m.http.rawBody = !m.http.rawBody
				m.invalidate()
			})
		}
	})
	ui.Scroll(c).Grow(1).MinHeight(0).Padding(8, 10).Children(func() {
		switch m.http.detailTab {
		case "headers":
			m.httpHeaders(c, p, httpResponseHeaders(ex))
		case "request":
			m.httpRequestDetails(c, p, ex.Request)
		default:
			m.httpResponseBody(c, p, ex)
		}
	})
}

func (m *Model) httpResponseBody(c *ui.Context, p *ui.Theme, ex *openidea.HTTPExchange) {
	oi := oiPaletteOf(p)
	if ex.Response == nil {
		text := "No response"
		if ex.Entry.Error != nil {
			text = *ex.Entry.Error
		}
		ui.Text(c, text).FontSize(12).TextColor(oi.danger)
		return
	}
	if ex.Response.Body == nil {
		contentType := "unknown type"
		if ex.Response.ContentType != nil {
			contentType = *ex.Response.ContentType
		}
		ui.Column(c).Gap(5).Children(func() {
			ui.Text(c, "Binary response").FontSize(13).Bold().TextColor(oi.text)
			ui.Text(c, contentType+", "+formatHTTPBytes(ex.Response.Size)).
				FontSize(12).TextColor(oi.textMuted)
		})
		return
	}
	body := *ex.Response.Body
	if !m.http.rawBody {
		body = prettyHTTPBody(body, ex.Response.ContentType)
	}
	if ex.Response.Truncated {
		body += fmt.Sprintf("\n… truncated (%s received)", formatHTTPBytes(ex.Response.Size))
	}
	ui.Text(c, body).FontSize(12).Font("monospace").TextColor(oi.text)
}

func (m *Model) httpHeaders(c *ui.Context, p *ui.Theme, headers [][2]string) {
	oi := oiPaletteOf(p)
	if len(headers) == 0 {
		ui.Text(c, "No headers").FontSize(12).TextColor(oi.textMuted)
		return
	}
	ui.Column(c).Gap(2).Children(func() {
		for i, header := range headers {
			i, header := i, header
			ui.Row(c.Key(fmt.Sprintf("http-header:%d", i))).MinHeight(24).Padding(3, 5).
				Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Text(c, header[0]).FontSize(12).Bold().TextColor(oi.textSecondary).Width(180)
				ui.Text(c, header[1]).FontSize(12).Font("monospace").TextColor(oi.text).Grow(1)
			})
		}
	})
}

func (m *Model) httpRequestDetails(c *ui.Context, p *ui.Theme, request openidea.HTTPPreparedRequest) {
	oi := oiPaletteOf(p)
	var text strings.Builder
	text.WriteString(request.Method)
	text.WriteByte(' ')
	text.WriteString(request.URL)
	text.WriteByte('\n')
	for _, header := range request.Headers {
		text.WriteString(header[0])
		text.WriteString(": ")
		text.WriteString(header[1])
		text.WriteByte('\n')
	}
	if request.Body != nil {
		text.WriteByte('\n')
		text.WriteString(*request.Body)
	}
	ui.Text(c, text.String()).FontSize(12).Font("monospace").TextColor(oi.text)
}

func httpResponseHeaders(ex *openidea.HTTPExchange) [][2]string {
	if ex == nil || ex.Response == nil {
		return nil
	}
	return ex.Response.Headers
}

func prettyHTTPBody(body string, contentType *string) string {
	return httpPrettyBody(httpBodyKind(contentType, body), body)
}

func formatHTTPDuration(ms float64) string {
	if ms < 1000 {
		return fmt.Sprintf("%.0f ms", ms)
	}
	return fmt.Sprintf("%.2f s", ms/1000)
}

func formatHTTPBytes(size float64) string {
	if size < 1024 {
		return fmt.Sprintf("%.0f B", size)
	}
	if size < 1024*1024 {
		return fmt.Sprintf("%.1f KB", size/1024)
	}
	return fmt.Sprintf("%.1f MB", size/(1024*1024))
}

func httpHistoryTime(ms float64) string {
	if ms <= 0 {
		return ""
	}
	return time.UnixMilli(int64(ms)).Local().Format("15:04:05")
}

func httpHistoryFileName(entry openidea.HTTPHistoryEntry) string {
	if entry.File == nil {
		return ""
	}
	return filepath.Base(*entry.File)
}
