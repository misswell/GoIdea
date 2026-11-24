package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"

	"mayoi/internal/openidea"
)

type httpClientState struct {
	root string

	historyLoading bool
	historyErr     string
	history        []openidea.HTTPHistoryEntry
	selected       string
	exchange       *openidea.HTTPExchange
	exchangeBusy   bool
	exchangeErr    string
	detailTab      string
	rawBody        bool

	running *httpRequestRun

	environments map[string][]string
	envByFile    map[string]string

	blockGeneration uint64
	blocks          map[string]httpBlockCache

	routeLoading bool
	routeLoaded  bool
	routeErr     string
	routeSeq     uint64
	routes       []httpRouteEntry

	pluginEndpoints        []openidea.PluginEndpoint
	pluginEndpointsLoaded  bool
	pluginEndpointsLoading bool
	pluginEndpointSeq      uint64
}

type httpRequestRun struct {
	request uint32
	path    string
	index   uint32
	method  string
	url     string
	cancel  context.CancelFunc
}

func (m *Model) resetHTTPClient() {
	if m.http.running != nil && m.http.running.cancel != nil {
		m.http.running.cancel()
	}
	envByFile := make(map[string]string, len(m.prefs.HTTP.Environments))
	for path, environment := range m.prefs.HTTP.Environments {
		if path = cleanHTTPPath(path); path != "" && strings.TrimSpace(environment) != "" {
			envByFile[path] = environment
		}
	}
	m.http = httpClientState{
		root:         m.Root(),
		detailTab:    "response",
		environments: map[string][]string{},
		envByFile:    envByFile,
		blocks:       map[string]httpBlockCache{},
	}
}

func (m *Model) ensureHTTPClient() {
	root := m.Root()
	if m.http.root == root && m.http.envByFile != nil && m.http.blocks != nil {
		if m.http.detailTab == "" {
			m.http.detailTab = "response"
		}
		return
	}
	m.resetHTTPClient()
}

func (m *Model) showHTTPClient() {
	m.ensureHTTPClient()
	m.services.view = serviceHostHTTP
	m.ShowServices()
	m.loadHTTPHistory(false)
}

func (m *Model) loadHTTPHistory(force bool) {
	m.ensureHTTPClient()
	if m.http.historyLoading || (!force && m.http.history != nil) {
		return
	}
	if m.invokeHook == nil && m.mgr == nil {
		return
	}
	root := m.Root()
	m.http.historyLoading = true
	m.http.historyErr = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "http_history", nil)
		var history []openidea.HTTPHistoryEntry
		if err == nil {
			err = json.Unmarshal(raw, &history)
		}
		m.post(func() {
			if m.http.root != root {
				return
			}
			m.http.historyLoading = false
			if err != nil {
				m.http.historyErr = "Cannot read HTTP Client history: " + err.Error()
				m.invalidate()
				return
			}
			m.http.history = history
			if m.http.selected == "" && len(history) > 0 {
				m.selectHTTPHistory(history[0].ID)
			}
			m.invalidate()
		})
	}()
}

func (m *Model) selectHTTPHistory(id string) {
	m.ensureHTTPClient()
	if id == "" {
		return
	}
	m.http.selected = id
	m.http.exchangeBusy = true
	m.http.exchangeErr = ""
	m.http.exchange = nil
	root := m.Root()
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "http_history_get", map[string]any{"id": id})
		var exchange openidea.HTTPExchange
		if err == nil {
			err = json.Unmarshal(raw, &exchange)
		}
		m.post(func() {
			if m.http.root != root || m.http.selected != id {
				return
			}
			m.http.exchangeBusy = false
			if err != nil {
				m.http.exchangeErr = "Cannot open HTTP response: " + err.Error()
				m.invalidate()
				return
			}
			m.http.exchange = &exchange
			m.invalidate()
		})
	}()
}

func (m *Model) clearHTTPHistory() {
	m.ensureHTTPClient()
	if m.invokeHook == nil && m.mgr == nil {
		return
	}
	root := m.Root()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_, err := m.invoke(ctx, "http_history_clear", nil)
		m.post(func() {
			if m.http.root != root {
				return
			}
			if err != nil {
				m.notifyError("http-history-clear", "Cannot clear HTTP Client history", err.Error())
				m.invalidate()
				return
			}
			m.http.history = []openidea.HTTPHistoryEntry{}
			m.http.selected = ""
			m.http.exchange = nil
			m.invalidate()
		})
	}()
}

func (m *Model) RunHTTPRequestAtCaret() {
	t := m.tab()
	if t == nil || t.doc == nil || !isHTTPRequestFile(t.doc.Path()) {
		m.status = "Open a .http or .rest request file to send a request"
		m.invalidate()
		return
	}
	m.withHTTPRequestAtCaret(t, func(block openidea.HTTPRequestBlock, text string) {
		m.RunHTTPRequest(t.doc.Path(), text, block.Index)
	})
}

func (m *Model) RunHTTPRequest(path, text string, index uint32) {
	m.showHTTPClient()
	go m.runHTTPRequest(path, text, index)
}

func (m *Model) RunAllHTTPRequests() {
	t := m.tab()
	if t == nil || t.doc == nil || !isHTTPRequestFile(t.doc.Path()) {
		m.status = "Open a .http or .rest request file to run requests"
		m.invalidate()
		return
	}
	path, text := t.doc.Path(), t.doc.Text()
	m.showHTTPClient()
	go func() {
		blocks, err := m.parseHTTPRequests(text)
		if err != nil {
			m.post(func() {
				m.notifyError("http-run-all", "Cannot send the requests", err.Error())
				m.invalidate()
			})
			return
		}
		for _, block := range blocks {
			if !m.runHTTPRequest(path, text, block.Index) {
				return
			}
		}
	}()
}

func (m *Model) parseHTTPRequests(text string) ([]openidea.HTTPRequestBlock, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	raw, err := m.invoke(ctx, "http_requests", map[string]any{"text": text})
	if err != nil {
		return nil, err
	}
	var blocks []openidea.HTTPRequestBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, err
	}
	return blocks, nil
}

func (m *Model) runHTTPRequest(path, text string, index uint32) bool {
	m.ensureHTTPClient()
	if m.invokeHook == nil && m.mgr == nil {
		return false
	}
	if m.http.running != nil {
		m.cancelHTTPRequest(false)
	}
	request := m.nextRequest()
	ctx, cancel := context.WithCancel(context.Background())
	run := &httpRequestRun{
		request: request, path: path, index: index, cancel: cancel,
	}
	root := m.Root()
	m.post(func() {
		if m.http.root != root {
			return
		}
		m.http.running = run
		m.http.exchangeErr = ""
		m.services.view = serviceHostHTTP
		m.invalidate()
	})

	if blocks, err := m.parseHTTPRequests(text); err == nil {
		for _, block := range blocks {
			if block.Index != index {
				continue
			}
			m.post(func() {
				current := m.http.running
				if current == nil || current.request != request {
					return
				}
				current.method, current.url = block.Method, block.URL
				m.invalidate()
			})
			break
		}
	}

	env := m.effectiveHTTPEnvironment(path)
	var pathArg any
	if path != "" {
		pathArg = path
	}
	args := map[string]any{
		"request": request,
		"path":    pathArg,
		"text":    text,
		"index":   index,
		"env":     env,
	}
	raw, err := m.invoke(ctx, "http_execute", args)
	defer cancel()
	var exchange openidea.HTTPExchange
	if err == nil {
		err = json.Unmarshal(raw, &exchange)
	}
	cancelled := isCancelledError(err)
	m.post(func() {
		if m.http.root != root {
			return
		}
		if m.http.running != nil && m.http.running.request == request {
			m.http.running = nil
		}
		if err != nil {
			if !cancelled {
				m.notifyError("http-run", "Cannot send the request", err.Error())
			}
			m.invalidate()
			return
		}
		m.http.history = append(
			[]openidea.HTTPHistoryEntry{exchange.Entry},
			removeHTTPHistoryEntry(m.http.history, exchange.Entry.ID)...,
		)
		m.http.selected = exchange.Entry.ID
		m.http.exchange = &exchange
		m.http.exchangeBusy = false
		m.http.exchangeErr = ""
		m.invalidate()
	})
	return err == nil
}

func removeHTTPHistoryEntry(history []openidea.HTTPHistoryEntry, id string) []openidea.HTTPHistoryEntry {
	out := history[:0]
	for _, entry := range history {
		if entry.ID != id {
			out = append(out, entry)
		}
	}
	return out
}

func isCancelledError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "cancel")
}

func (m *Model) cancelHTTPRequest(notify bool) {
	m.ensureHTTPClient()
	run := m.http.running
	if run == nil {
		return
	}
	if run.cancel != nil {
		run.cancel()
	}
	m.http.running = nil
	if notify {
		m.invalidate()
	}
	request := run.request
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = m.invoke(ctx, "cancel_request", map[string]any{"id": request})
	}()
}

func (m *Model) loadHTTPEnvironments(path string) {
	m.ensureHTTPClient()
	path = cleanHTTPPath(path)
	if path == "" || m.http.environments[path] != nil || m.invokeHook == nil && m.mgr == nil {
		return
	}
	root := m.Root()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "http_environments", map[string]any{"path": path})
		var names []string
		if err == nil {
			err = json.Unmarshal(raw, &names)
		}
		m.post(func() {
			if m.http.root != root {
				return
			}
			if err != nil {
				m.http.environments[path] = []string{}
				m.notifyWarning("http-env", "HTTP Client environments", err.Error())
			} else {
				m.http.environments[path] = names
			}
			m.invalidate()
		})
	}()
}

func (m *Model) effectiveHTTPEnvironment(path string) any {
	m.ensureHTTPClient()
	path = cleanHTTPPath(path)
	chosen := m.http.envByFile[path]
	if chosen == "" {
		return nil
	}
	available := m.http.environments[path]
	if available != nil {
		found := false
		for _, name := range available {
			if name == chosen {
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}
	return chosen
}

func (m *Model) setHTTPEnvironment(path, environment string) {
	m.ensureHTTPClient()
	path = cleanHTTPPath(path)
	if path == "" {
		return
	}
	if environment == "" {
		delete(m.http.envByFile, path)
		delete(m.prefs.HTTP.Environments, path)
	} else {
		m.http.envByFile[path] = environment
		if m.prefs.HTTP.Environments == nil {
			m.prefs.HTTP.Environments = map[string]string{}
		}
		m.prefs.HTTP.Environments[path] = environment
	}
	m.savePreferences()
	m.invalidate()
}

func (m *Model) OpenHTTPEnvironmentFile(private bool) {
	t := m.tab()
	if t == nil || t.doc == nil || !isHTTPRequestFile(t.doc.Path()) {
		m.status = "Open a .http or .rest request file first"
		m.invalidate()
		return
	}
	path := cleanHTTPPath(t.doc.Path())
	root := m.Root()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "http_environment_file", map[string]any{
			"path": path, "private": private,
		})
		var environmentFile string
		if err == nil {
			err = json.Unmarshal(raw, &environmentFile)
		}
		m.post(func() {
			if m.http.root != root {
				return
			}
			if err != nil {
				m.notifyError("http-env-file", "Cannot create the environment file", err.Error())
				m.invalidate()
				return
			}
			m.openFile(environmentFile)
			delete(m.http.environments, path)
			m.loadHTTPEnvironments(path)
		})
	}()
}

func (m *Model) CopyHTTPRequestAsCurl() {
	t := m.tab()
	if t == nil || t.doc == nil || !isHTTPRequestFile(t.doc.Path()) {
		m.status = "Open a .http or .rest request file first"
		m.invalidate()
		return
	}
	m.withHTTPRequestAtCaret(t, func(block openidea.HTTPRequestBlock, text string) {
		path := cleanHTTPPath(t.doc.Path())
		env := m.effectiveHTTPEnvironment(path)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			raw, err := m.invoke(ctx, "http_curl", map[string]any{
				"path": path, "text": text, "index": block.Index, "env": env,
			})
			var command string
			if err == nil {
				err = json.Unmarshal(raw, &command)
			}
			m.post(func() {
				if err != nil {
					m.notifyError("http-curl", "Cannot convert the request to cURL", err.Error())
				} else {
					mygo.Clipboard.WriteText(command)
					m.status = "The request was copied as a cURL command"
				}
				m.invalidate()
			})
		}()
	})
}

func (m *Model) HTTPFromClipboard() {
	t := m.tab()
	if t == nil || t.doc == nil || !isHTTPRequestFile(t.doc.Path()) {
		m.status = "Open a .http or .rest request file to paste a cURL command"
		m.invalidate()
		return
	}
	if !m.pasteHTTPRequest(t) {
		m.status = "The clipboard does not contain a cURL command"
		m.invalidate()
	}
}

func isHTTPRequestFile(path string) bool {
	extension := strings.ToLower(filepath.Ext(path))
	return extension == ".http" || extension == ".rest"
}

func cleanHTTPPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	return filepath.Clean(path)
}

func (m *Model) selectHTTPDetailTab(tab string) {
	switch tab {
	case "response", "headers", "request":
		m.http.detailTab = tab
		m.invalidate()
	}
}

func httpHistoryTitle(entry openidea.HTTPHistoryEntry) string {
	if entry.Name != nil && strings.TrimSpace(*entry.Name) != "" {
		return strings.TrimSpace(*entry.Name)
	}
	return strings.TrimSpace(entry.Method + " " + entry.URL)
}

func httpHistoryError(entry openidea.HTTPHistoryEntry) string {
	if entry.Error == nil {
		return ""
	}
	return *entry.Error
}

func httpStatusLabel(entry openidea.HTTPHistoryEntry) string {
	if entry.Status != nil {
		return fmt.Sprintf("%d", *entry.Status)
	}
	return "ERR"
}
