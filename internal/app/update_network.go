package app

import (
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
)

// updateHTTP follows environment proxies as before, then falls back to the
// Settings › HTTP Proxy, then the macOS system proxy. A Finder-launched app has
// no shell proxy variables, so without these fallbacks an update check can work
// while its GitHub asset download does not.
var updateHTTP = newUpdateHTTPClient()

func newUpdateHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = updateRequestProxy
	return &http.Client{Transport: transport}
}

func updateRequestProxy(req *http.Request) (*url.URL, error) {
	if req == nil || req.URL == nil {
		return nil, nil
	}
	if runtime := proxyRuntime.Load(); runtime != nil && runtime.proxyPageLoaded() {
		return runtime.applicationRequestProxy(req)
	}
	if proxy, err := http.ProxyFromEnvironment(req); proxy != nil || err != nil {
		return proxy, err
	}
	if runtime.GOOS != "darwin" || localRequestHost(req.URL) {
		return nil, nil
	}
	snapshot, err := readMacProxySnapshot()
	if err != nil {
		return nil, nil
	}
	return snapshot.proxyFor(req.URL), nil
}

type nativeProxySettings struct {
	mode         string
	kind         string
	host         string
	port         int
	noProxy      string
	authenticate bool
	login        string
	password     string
}

// proxyRuntime is the active window whose Settings page supplies the native
// proxy. The update transport is package-level for compatibility with its
// callers; requests are still resolved against the current window each time.
var proxyRuntime atomic.Pointer[Model]

func setProxyRuntime(m *Model) {
	proxyRuntime.Store(m)
}

func (p nativeProxySettings) proxyFor(u *url.URL) (*url.URL, error) {
	if u == nil || localRequestHost(u) {
		return nil, nil
	}
	switch strings.ToLower(strings.TrimSpace(p.mode)) {
	case "none":
		return nil, nil
	case "manual":
		if proxyHTTPHostMatchesNoProxy(p.noProxy, u.Hostname()) {
			return nil, nil
		}
		host := strings.TrimSpace(p.host)
		if host == "" {
			return nil, nil
		}
		scheme := "http"
		if strings.EqualFold(strings.TrimSpace(p.kind), "socks") {
			scheme = "socks5h"
		}
		proxy := &url.URL{
			Scheme: scheme,
			Host:   net.JoinHostPort(host, strconv.Itoa(p.port)),
		}
		if p.authenticate && strings.TrimSpace(p.login) != "" {
			proxy.User = url.UserPassword(strings.TrimSpace(p.login), p.password)
		}
		return proxy, nil
	default:
		if requestProxy := updateSystemRequestProxy(u); requestProxy != nil {
			return requestProxy, nil
		}
		return nil, nil
	}
}

func (m *Model) proxySettings() nativeProxySettings {
	return m.proxySnapshot()
}

func (m *Model) applicationRequestProxy(req *http.Request) (*url.URL, error) {
	if req == nil || req.URL == nil {
		return nil, nil
	}
	return m.proxySettings().proxyFor(req.URL)
}

func updateSystemRequestProxy(u *url.URL) *url.URL {
	if u == nil {
		return nil
	}
	req := &http.Request{URL: u}
	if proxy, err := http.ProxyFromEnvironment(req); proxy != nil || err != nil {
		return proxy
	}
	if runtime.GOOS != "darwin" || localRequestHost(u) {
		return nil
	}
	snapshot, err := readMacProxySnapshot()
	if err != nil {
		return nil
	}
	return snapshot.proxyFor(u)
}

func proxyHTTPHostMatchesNoProxy(noProxy, host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return false
	}
	for _, raw := range strings.FieldsFunc(noProxy, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n'
	}) {
		pattern := strings.ToLower(strings.TrimSpace(raw))
		if pattern == "" {
			continue
		}
		if proxyHTTPGlobMatch(pattern, host) {
			return true
		}
		if strings.HasPrefix(pattern, ".") && proxyHTTPGlobMatch("*"+pattern, host) {
			return true
		}
	}
	return false
}

func proxyHTTPGlobMatch(pattern, value string) bool {
	patternBytes, valueBytes := []byte(pattern), []byte(value)
	patternIndex, valueIndex := 0, 0
	starIndex, starValueIndex := -1, 0
	for valueIndex < len(valueBytes) {
		switch {
		case patternIndex < len(patternBytes) && patternBytes[patternIndex] == byte('*'):
			starIndex = patternIndex
			starValueIndex = valueIndex
			patternIndex++
		case patternIndex < len(patternBytes) && patternBytes[patternIndex] == valueBytes[valueIndex]:
			patternIndex++
			valueIndex++
		case starIndex >= 0:
			patternIndex = starIndex + 1
			starValueIndex++
			valueIndex = starValueIndex
		default:
			return false
		}
	}
	for patternIndex < len(patternBytes) && patternBytes[patternIndex] == byte('*') {
		patternIndex++
	}
	return patternIndex == len(patternBytes)
}

func localRequestHost(u *url.URL) bool {
	host := strings.TrimSpace(u.Hostname())
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func readMacProxySnapshot() (systemProxySnapshot, error) {
	out, err := exec.Command("/usr/sbin/scutil", "--proxy").Output()
	if err != nil {
		return systemProxySnapshot{}, err
	}
	return parseMacProxySnapshot(string(out)), nil
}

type systemProxySnapshot struct {
	http       proxySetting
	https      proxySetting
	socks      proxySetting
	exceptions []string
}

type proxySetting struct {
	enabled bool
	host    string
	port    string
}

func parseMacProxySnapshot(text string) systemProxySnapshot {
	values := make(map[string]string)
	var snapshot systemProxySnapshot
	inExceptions := false
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if inExceptions && line == "}" {
			inExceptions = false
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(strings.Trim(key, `"`))
		value = strings.TrimSpace(strings.Trim(value, `"`))
		if key == "ExceptionsList" {
			inExceptions = true
			continue
		}
		if inExceptions {
			if _, numbered := parseExceptionIndex(key); numbered {
				snapshot.exceptions = append(snapshot.exceptions, value)
			}
			continue
		}
		values[key] = value
	}
	snapshot.http = proxySetting{
		enabled: values["HTTPEnable"] == "1",
		host:    values["HTTPProxy"],
		port:    values["HTTPPort"],
	}
	snapshot.https = proxySetting{
		enabled: values["HTTPSEnable"] == "1",
		host:    values["HTTPSProxy"],
		port:    values["HTTPSPort"],
	}
	snapshot.socks = proxySetting{
		enabled: values["SOCKSEnable"] == "1",
		host:    values["SOCKSProxy"],
		port:    values["SOCKSPort"],
	}
	return snapshot
}

func parseExceptionIndex(key string) (int, bool) {
	if key == "" {
		return 0, false
	}
	n, err := strconv.Atoi(key)
	return n, err == nil
}

func (s systemProxySnapshot) proxyFor(u *url.URL) *url.URL {
	if u == nil || s.bypass(u.Hostname()) {
		return nil
	}
	setting := s.http
	kind := "http"
	if u.Scheme == "https" && s.https.enabled {
		setting = s.https
	} else if !setting.enabled && s.https.enabled {
		setting = s.https
	}
	if !setting.enabled && s.socks.enabled {
		setting = s.socks
		kind = "socks5"
	}
	if !setting.enabled || setting.host == "" || setting.port == "" {
		return nil
	}
	return &url.URL{Scheme: kind, Host: net.JoinHostPort(setting.host, setting.port)}
}

func (s systemProxySnapshot) bypass(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	ip := net.ParseIP(host)
	for _, raw := range s.exceptions {
		entry := strings.ToLower(strings.TrimSpace(raw))
		switch {
		case entry == "<local>":
			if !strings.Contains(host, ".") {
				return true
			}
		case strings.Contains(entry, "/"):
			if ip != nil {
				if _, network, err := net.ParseCIDR(entry); err == nil && network.Contains(ip) {
					return true
				}
			}
		case strings.HasPrefix(entry, "*."):
			suffix := strings.TrimPrefix(entry, "*.")
			if strings.HasSuffix(host, "."+suffix) {
				return true
			}
		default:
			if host == entry || strings.HasSuffix(host, "."+strings.TrimPrefix(entry, ".")) {
				return true
			}
		}
	}
	return false
}
