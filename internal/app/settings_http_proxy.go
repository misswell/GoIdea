package app

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

const nativeHTTPProxyCheckURL = "http://"

func nativeHTTPProxyMode(m *Model) string {
	switch strings.ToLower(strings.TrimSpace(
		nativeSettingsString(m, settingsPageHTTPProxy, "proxy.mode"),
	)) {
	case "none", "manual":
		return strings.ToLower(strings.TrimSpace(
			nativeSettingsString(m, settingsPageHTTPProxy, "proxy.mode"),
		))
	default:
		return "auto"
	}
}

func nativeHTTPProxyAuthenticationEnabled(m *Model) bool {
	return nativeHTTPProxyMode(m) == "manual" &&
		nativeSettingsBool(m, settingsPageHTTPProxy, "proxy.authenticate")
}

func nativeHTTPProxyKind(m *Model) string {
	if strings.EqualFold(
		strings.TrimSpace(nativeSettingsString(m, settingsPageHTTPProxy, "proxy.kind")),
		"socks",
	) {
		return "socks"
	}
	return "http"
}

func (m *Model) setNativeHTTPProxyKind(kind string) {
	previous := nativeHTTPProxyKind(m)
	if kind != "socks" {
		kind = "http"
	}
	if previous == kind {
		return
	}
	m.nativeSettingsMu.Lock()
	m.nativeSettingsMu.Lock()
	state := m.nativeSettings.pages[settingsPageHTTPProxy]
	m.nativeSettingsMu.Unlock()
	if state.draft == nil {
		state.draft = map[string]any{}
	}
	port := 0
	switch typed := state.draft["proxy.port"].(type) {
	case float64:
		port = int(typed)
	case int:
		port = typed
	}
	state.draft["proxy.kind"] = kind
	switch {
	case previous == "http" && port == 80:
		state.draft["proxy.port"] = float64(1080)
	case previous == "socks" && port == 1080:
		state.draft["proxy.port"] = float64(80)
	}
	m.nativeSettings.pages[settingsPageHTTPProxy] = state
	m.nativeSettingsMu.Unlock()
	m.invalidate()
}

func nativeHTTPProxyPort(m *Model) int {
	port := int(nativeSettingsNumberValue(m, settingsPageHTTPProxy, "proxy.port"))
	if port >= 1 && port <= 65535 {
		return port
	}
	if strings.EqualFold(
		nativeSettingsString(m, settingsPageHTTPProxy, "proxy.kind"),
		"socks",
	) {
		return 1080
	}
	return 80
}

func nativeHTTPProxyProblem(m *Model) string {
	if nativeHTTPProxyMode(m) != "manual" {
		return ""
	}
	host := strings.TrimSpace(nativeSettingsString(m, settingsPageHTTPProxy, "proxy.host"))
	switch {
	case host == "":
		return "Proxy host is empty"
	case strings.ContainsAny(host, "/@ "):
		return "Invalid proxy host: " + host
	case nativeHTTPProxyPort(m) < 1 || nativeHTTPProxyPort(m) > 65535:
		return "Proxy port must be between 1 and 65535"
	case nativeSettingsBool(m, settingsPageHTTPProxy, "proxy.authenticate") &&
		strings.TrimSpace(nativeSettingsString(m, settingsPageHTTPProxy, "proxy.login")) == "":
		return "Proxy login is empty"
	default:
		return ""
	}
}

func (m *Model) renderHTTPProxyCheck(c *ui.Context, oi oiColors) {
	m.nativeSettingsMu.Lock()
	state := m.nativeSettings.pages[settingsPageHTTPProxy]
	m.nativeSettingsMu.Unlock()
	if state.aux == nil {
		state.aux = map[string]string{}
		nativeSettingsStateSet(m, settingsPageHTTPProxy, state)
	}
	problem := nativeHTTPProxyProblem(m)
	if problem != "" {
		ui.Text(c, problem).FontSize(11).TextColor(oi.warning).
			PaddingStart(20).SingleLine().Ellipsis("…")
	}
	urlText := state.aux["proxy.checkURL"]
	if urlText == "" {
		urlText = nativeHTTPProxyCheckURL
	}
	status := state.aux["proxy.checkStatus"]
	running := state.aux["proxy.checkRunning"] == "1"
	ui.Row(c).MinHeight(32).Gap(6).AlignItems(ui.Center).PaddingStart(20).Children(func() {
		settingsFormLabel(c, oi, "Check connection", 190)
		input := ui.TextInput(c, &urlText).Grow(1).Placeholder(nativeHTTPProxyCheckURL)
		if input.Changed() {
			m.nativeSettingsMu.Lock()
			updated := m.nativeSettings.pages[settingsPageHTTPProxy]
			if updated.aux == nil {
				updated.aux = map[string]string{}
			}
			updated.aux["proxy.checkURL"] = urlText
			if updated.draft == nil {
				updated.draft = map[string]any{}
			}
			m.nativeSettingsMu.Unlock()
			nativeSettingsStateSet(m, settingsPageHTTPProxy, updated)
		}
		button := ui.Button(c, "Check")
		if running {
			button = ui.Button(c, "Checking…")
		}
		button.Width(96).Disabled(problem != "" || running ||
			strings.TrimSpace(urlText) == "").OnClick(func() {
			m.checkHTTPProxyConnection(strings.TrimSpace(urlText))
		})
	})
	if status != "" {
		ui.Text(c, status).FontSize(11).TextColor(oi.textMuted).
			PaddingStart(20).SingleLine().Ellipsis("…")
	}
}

func nativeSettingsStateSet(m *Model, page string, state nativeSettingsPageState) {
	m.nativeSettingsMu.Lock()
	defer m.nativeSettingsMu.Unlock()
	if m.nativeSettings.pages == nil {
		m.nativeSettings.pages = map[string]nativeSettingsPageState{}
	}
	m.nativeSettings.pages[page] = state
}

func (m *Model) checkHTTPProxyConnection(rawURL string) {
	rawURL = withHTTPProxyScheme(rawURL)
	if rawURL == "" {
		m.settingsErr = "Enter a URL to check the proxy connection"
		m.invalidate()
		return
	}
	if _, err := url.ParseRequestURI(rawURL); err != nil {
		m.settingsErr = "Invalid proxy check URL: " + err.Error()
		m.invalidate()
		return
	}
	state := m.nativeSettings.pages[settingsPageHTTPProxy]
	if state.aux == nil {
		state.aux = map[string]string{}
	}
	state.aux["proxy.checkRunning"] = "1"
	state.aux["proxy.checkStatus"] = "Connecting…"
	nativeSettingsStateSet(m, settingsPageHTTPProxy, state)
	m.invalidate()
	proxy := m.proxySettings()
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 31*time.Second)
		defer cancel()
		status, err := proxyCheckConnection(ctx, proxy, rawURL)
		m.post(func() {
			m.nativeSettingsMu.Lock()
			state := m.nativeSettings.pages[settingsPageHTTPProxy]
			if state.aux == nil {
				state.aux = map[string]string{}
			}
			delete(state.aux, "proxy.checkRunning")
			if err != nil {
				state.aux["proxy.checkStatus"] = "Problem with connection: " + err.Error()
			} else {
				state.aux["proxy.checkStatus"] = "Connection successful: " + status
			}
			m.nativeSettingsMu.Unlock()
			nativeSettingsStateSet(m, settingsPageHTTPProxy, state)
			m.invalidate()
		})
	}()
}

func proxyCheckConnection(ctx context.Context, settings nativeProxySettings, rawURL string) (string, error) {
	if problem := settings.problem(); problem != "" {
		return "", errors.New(problem)
	}
	parsed, err := url.ParseRequestURI(withHTTPProxyScheme(rawURL))
	if err != nil {
		return "", err
	}
	proxy, err := settings.proxyFor(parsed)
	if err != nil {
		return "", err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = func(*http.Request) (*url.URL, error) {
		return proxy, nil
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			return nil
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return "", err
	}
	response, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
	if response.StatusCode == http.StatusProxyAuthRequired {
		return "", errors.New("Proxy authentication required (407)")
	}
	return response.Status, nil
}

func withHTTPProxyScheme(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if parsed, err := url.Parse(raw); err == nil && parsed.Scheme != "" &&
		parsed.Host != "" {
		return raw
	}
	return "http://" + raw
}

func (p nativeProxySettings) problem() string {
	if strings.ToLower(strings.TrimSpace(p.mode)) != "manual" {
		return ""
	}
	host := strings.TrimSpace(p.host)
	switch {
	case host == "":
		return "Proxy host is empty"
	case strings.ContainsAny(host, "/@ "):
		return "Invalid proxy host: " + host
	case p.port < 1 || p.port > 65535:
		return "Proxy port must be between 1 and 65535"
	case p.authenticate && strings.TrimSpace(p.login) == "":
		return "Proxy login is empty"
	default:
		return ""
	}
}
