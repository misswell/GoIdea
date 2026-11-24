package app

import (
	"path/filepath"
	"regexp"
	"strings"
)

const dockerComposeScanLimit = 64 * 1024

type dockerTarget struct {
	kind    string
	line    int
	stage   string
	last    bool
	service string
}

var dockerFileName = regexp.MustCompile(`(?i)^(dockerfile|containerfile)(\.[\w.-]+)?$|\.(dockerfile|containerfile)$`)
var dockerFrom = regexp.MustCompile(`(?i)^\s*FROM\s+(?:--\S+\s+)*\S+(?:\s+AS\s+([\w.-]+))?\s*$`)
var composeKey = regexp.MustCompile(`^(\s*)(?:"([^"]+)"|'([^']+)'|([A-Za-z0-9._-]+))\s*:\s*(?:[&*{#].*)?$`)
var composeAnyKey = regexp.MustCompile(`^(\s*)(?:"([^"]+)"|'([^']+)'|([A-Za-z0-9._-]+))\s*:`)

func dockerBase(path string) string { return filepath.Base(strings.ReplaceAll(path, `\`, "/")) }
func isDockerfile(path string) bool { return dockerFileName.MatchString(dockerBase(path)) }
func isYAMLFile(path string) bool {
	p := strings.ToLower(dockerBase(path))
	return strings.HasSuffix(p, ".yaml") || strings.HasSuffix(p, ".yml")
}
func isNamedComposeFile(path string) bool {
	return isYAMLFile(path) && strings.Contains(strings.ToLower(dockerBase(path)), "compose")
}
func composeKeyName(m []string) string {
	if len(m) == 0 {
		return ""
	}
	for _, s := range m[2:] {
		if s != "" {
			return s
		}
	}
	return ""
}
func isComposeServiceName(s string) bool { return s != "" && !strings.HasPrefix(s, "x-") }

var composeTopKeys = map[string]bool{"version": true, "name": true, "networks": true, "volumes": true, "configs": true, "secrets": true, "include": true}
var composeServiceKeys = func() map[string]bool {
	m := map[string]bool{}
	for _, s := range strings.Fields("image build container_name hostname domainname ports expose environment env_file volumes volumes_from networks network_mode depends_on command entrypoint restart pull_policy healthcheck deploy develop labels annotations platform profiles user working_dir privileged read_only stdin_open tty init cap_add cap_drop cgroup cgroup_parent configs secrets cpus cpu_count cpu_percent cpu_shares cpu_period cpu_quota cpuset devices dns dns_opt dns_search extends external_links extra_hosts group_add ipc isolation links logging mac_address mem_limit mem_reservation oom_kill_disable oom_score_adj pid pids_limit post_start pre_stop runtime scale security_opt shm_size stop_grace_period stop_signal storage_opt sysctls tmpfs ulimits userns_mode uts") {
		m[s] = true
	}
	return m
}()

func isComposeContent(text string) bool {
	if len(text) > dockerComposeScanLimit {
		text = text[:dockerComposeScanLimit]
	}
	lines := strings.Split(text, "\n")
	if len(lines) > 600 {
		lines = lines[:600]
	}
	start := -1
	for i, l := range lines {
		l = strings.TrimSuffix(l, "\r")
		if strings.TrimSpace(l) == "" || strings.HasPrefix(strings.TrimSpace(l), "#") {
			continue
		}
		if l[0] == ' ' || l[0] == '\t' {
			continue
		}
		m := composeAnyKey.FindStringSubmatch(l)
		if m == nil {
			if l != "---" && l != "..." {
				return false
			}
			continue
		}
		k := composeKeyName(m)
		if k == "services" {
			if start < 0 {
				start = i
			}
		} else if !composeTopKeys[k] && !strings.HasPrefix(k, "x-") {
			return false
		}
	}
	if start < 0 {
		return false
	}
	serviceIndent, propIndent := -1, -1
	service := false
	for _, l := range lines[start+1:] {
		if strings.TrimSpace(l) == "" || strings.HasPrefix(strings.TrimSpace(l), "#") {
			continue
		}
		lead := len(l) - len(strings.TrimLeft(l, " \t"))
		if lead == 0 {
			break
		}
		if serviceIndent < 0 {
			serviceIndent = lead
		}
		if lead == serviceIndent {
			propIndent = -1
			service = isComposeServiceName(composeKeyName(composeKey.FindStringSubmatch(strings.TrimSuffix(l, "\r"))))
			continue
		}
		if service && lead > serviceIndent {
			if propIndent < 0 {
				propIndent = lead
			}
			if lead == propIndent {
				key := composeKeyName(composeAnyKey.FindStringSubmatch(l))
				if composeServiceKeys[key] || strings.HasPrefix(key, "x-") {
					return true
				}
			}
		}
	}
	return false
}
func isComposeFile(path, text string) bool {
	return isYAMLFile(path) && (isNamedComposeFile(path) || isComposeContent(text))
}
func dockerfileTargets(text string) []dockerTarget {
	var out []dockerTarget
	for i, l := range strings.Split(text, "\n") {
		if m := dockerFrom.FindStringSubmatch(strings.TrimSuffix(l, "\r")); m != nil {
			out = append(out, dockerTarget{kind: "build", line: i, stage: m[1]})
		}
	}
	if len(out) > 0 {
		out[len(out)-1].last = true
	}
	return out
}
func composeTargets(text string) []dockerTarget {
	var out []dockerTarget
	lines := strings.Split(text, "\n")
	start := -1
	for i, l := range lines {
		if strings.HasPrefix(l, "services:") && composeKeyName(composeKey.FindStringSubmatch(strings.TrimSuffix(l, "\r"))) == "services" {
			start = i
			break
		}
	}
	if start < 0 {
		return out
	}
	out = append(out, dockerTarget{kind: "composeAll", line: start})
	indent := -1
	for i := start + 1; i < len(lines); i++ {
		l := lines[i]
		if strings.TrimSpace(l) == "" || strings.HasPrefix(strings.TrimSpace(l), "#") {
			continue
		}
		lead := len(l) - len(strings.TrimLeft(l, " \t"))
		if lead == 0 {
			break
		}
		if indent < 0 {
			indent = lead
		}
		if lead == indent {
			name := composeKeyName(composeKey.FindStringSubmatch(strings.TrimSuffix(l, "\r")))
			if isComposeServiceName(name) {
				out = append(out, dockerTarget{kind: "composeService", line: i, service: name})
			}
		}
	}
	return out
}

func dockerFileTargets(path, text string) []dockerTarget {
	if isDockerfile(path) {
		return dockerfileTargets(text)
	}
	if isComposeFile(path, text) {
		return composeTargets(text)
	}
	return nil
}

type composeDeclaration struct {
	name           string
	line, from, to int
}

func composeDeclarations(text string) map[string]map[string]composeDeclaration {
	result := map[string]map[string]composeDeclaration{}
	for _, s := range []string{"services", "networks", "volumes", "configs", "secrets"} {
		result[s] = map[string]composeDeclaration{}
	}
	section := ""
	childIndent := -1
	offset := 0
	for i, raw := range strings.Split(text, "\n") {
		line := strings.TrimSuffix(raw, "\r")
		if strings.TrimSpace(line) != "" && !strings.HasPrefix(strings.TrimSpace(line), "#") {
			m := composeAnyKey.FindStringSubmatchIndex(line)
			if m != nil {
				lead := m[3] - m[2]
				match := composeAnyKey.FindStringSubmatch(line)
				key := composeKeyName(match)
				if lead == 0 {
					if _, ok := result[key]; ok {
						section = key
						childIndent = -1
					} else {
						section = ""
					}
				} else if section != "" {
					if childIndent < 0 {
						childIndent = lead
					}
					if lead == childIndent {
						from := strings.Index(line, key)
						result[section][key] = composeDeclaration{name: key, line: i, from: offset + from, to: offset + from + len(key)}
					}
				}
			}
		}
		offset += len(raw) + 1
	}
	return result
}
