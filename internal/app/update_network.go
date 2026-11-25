package app

import (
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// updateHTTP follows environment proxies as before, then falls back to the
// macOS system proxy. A Finder-launched app has no shell proxy variables, so
// without this fallback an update check can work while its GitHub asset
// download does not.
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
