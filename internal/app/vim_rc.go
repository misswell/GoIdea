package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

const (
	vimRcMaxSourcedFiles = 32
	vimRcSourceDepth     = 32
)

var vimRcNames = []string{".goideavimrc", ".openideavimrc", ".ideavimrc"}

type vimOptions struct {
	TextWidth                int
	PCRE                     bool
	InsertModeEscKeysTimeout int
	Langmap                  string
	Lmap                     string
}

type vimRcIgnored struct {
	File   string
	Line   int
	Text   string
	Reason string
}

type vimRcState struct {
	Path     string
	Commands int
	Mappings int
	Ignored  []vimRcIgnored
}

type vimMapping struct {
	Tokens []string
	RHS    []string
	Ex     string
}

type vimRcParse struct {
	Mappings []vimRcMapping
	Options  vimOptions
	Ignored  []vimRcIgnored
	Commands int
}

type vimRcMapping struct {
	Mode    string
	LHS     []string
	RHS     []string
	Ex      string
	Noremap bool
}

type vimRcLine struct {
	file string
	line int
	text string
}

var (
	vimMapCommandRE = regexp.MustCompile(`^(n|v|x|i|o)?(map|noremap)$`)
	vimSourceRE     = regexp.MustCompile(`^so(?:urce)?\s+(.+)$`)
	vimLetRE        = regexp.MustCompile(`^let\s+(?:g:)?mapleader\s*(=.*)$`)
	vimLeaderRE     = regexp.MustCompile(`(?i)<leader>`)
)

func (m *Model) ensureVimRc() {
	if m.vim.rcLoaded {
		return
	}
	m.reloadVimRc(true)
}

func (m *Model) reloadVimRc(quiet bool) {
	path, text, err := m.readVimRc()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		m.status = "Vim: rc file failed: " + err.Error()
		m.invalidate()
		return
	}
	parsed := m.parseVimRc(text, path)
	m.applyVimRc(parsed)
	m.vim.rc = vimRcState{
		Path:     path,
		Commands: parsed.Commands,
		Mappings: len(parsed.Mappings),
		Ignored:  append([]vimRcIgnored(nil), parsed.Ignored...),
	}
	m.vim.rcLoaded = true
	if !quiet {
		if parsed.Commands > 0 {
			m.status = "Vim: rc file reloaded"
		} else {
			m.status = "Vim: no Vim rc file found"
		}
	}
	m.invalidate()
}

func (m *Model) readVimRc() (string, string, error) {
	home := strings.TrimSpace(m.cfg.Home)
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return "", "", err
		}
	}
	var lastErr error
	for _, name := range vimRcNames {
		path := filepath.Join(home, name)
		raw, err := os.ReadFile(path)
		if err == nil {
			return path, string(raw), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			lastErr = err
		}
	}
	return "", "", lastErr
}

func (m *Model) parseVimRc(text, path string) vimRcParse {
	home := strings.TrimSpace(m.cfg.Home)
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	lines := make([]vimRcLine, 0)
	ignored := make([]vimRcIgnored, 0)
	sourced := 0
	active := map[string]bool{}
	loaded := map[string]bool{}
	if path != "" {
		active[filepath.Clean(path)] = true
	}
	var expand func(text, file, dir string, depth int)
	expand = func(text, file, dir string, depth int) {
		for i, raw := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
			line := strings.TrimSpace(raw)
			match := vimSourceRE.FindStringSubmatch(line)
			if match == nil {
				lines = append(lines, vimRcLine{file: file, line: i + 1, text: line})
				continue
			}
			sourcePath := strings.TrimSpace(match[1])
			if depth >= vimRcSourceDepth || sourced >= vimRcMaxSourcedFiles {
				ignored = append(ignored, vimRcIgnored{
					File: file, Line: i + 1, Text: line,
					Reason: "source limit reached",
				})
				continue
			}
			resolved := vimExpandHome(sourcePath, home)
			if !filepath.IsAbs(resolved) && dir != "" {
				resolved = filepath.Join(dir, resolved)
			}
			resolvedKey := filepath.Clean(resolved)
			if active[resolvedKey] {
				ignored = append(ignored, vimRcIgnored{
					File: file, Line: i + 1, Text: line,
					Reason: "file sources itself",
				})
				continue
			}
			if loaded[resolvedKey] {
				continue
			}
			rawSource, err := os.ReadFile(resolved)
			if err != nil {
				ignored = append(ignored, vimRcIgnored{
					File: file, Line: i + 1, Text: line,
					Reason: "cannot read file",
				})
				continue
			}
			sourced++
			loaded[resolvedKey] = true
			active[resolvedKey] = true
			expand(string(rawSource), sourcePath, filepath.Dir(resolved), depth+1)
			delete(active, resolvedKey)
		}
	}
	initialDir := ""
	if path != "" {
		initialDir = filepath.Dir(path)
	}
	expand(text, "", initialDir, 0)

	out := vimRcParse{Ignored: ignored}
	leader := "\\"
	for _, sourceLine := range lines {
		line := sourceLine.text
		if line == "" || strings.HasPrefix(line, `"`) {
			continue
		}
		ignore := func(reason string) {
			out.Ignored = append(out.Ignored, vimRcIgnored{
				File: sourceLine.file, Line: sourceLine.line, Text: line, Reason: reason,
			})
		}
		word, rest, _ := strings.Cut(line, " ")
		rest = strings.TrimSpace(rest)

		if word == "set" || word == "se" {
			fields := strings.Fields(rest)
			if len(fields) == 0 {
				ignore("set without options")
				continue
			}
			for _, field := range fields {
				name, value, ok := parseVimSetOption(field)
				if !ok {
					ignore("unsupported option syntax: " + field)
					continue
				}
				if !applyVimOption(&out.Options, name, value) {
					ignore("option not supported: " + name)
					continue
				}
				out.Commands++
			}
			continue
		}

		if word == "let" {
			match := vimLetRE.FindStringSubmatch(line)
			if match == nil {
				ignore("only `let mapleader` is supported")
				continue
			}
			value, ok := parseVimLetValue(match[1])
			if !ok || value == "" {
				ignore("only `let mapleader` is supported")
				continue
			}
			leader = normalizeVimLeader(value)
			out.Commands++
			continue
		}

		match := vimMapCommandRE.FindStringSubmatch(word)
		if match == nil {
			ignore("unsupported command: " + word)
			continue
		}
		fields := removeVimMapFlags(strings.Fields(rest))
		if len(fields) < 2 {
			ignore("mapping needs a left and right side")
			continue
		}
		lhsText := replaceVimLeader(fields[0], leader)
		rhsText := replaceVimLeader(strings.Join(fields[1:], " "), leader)
		lhs := tokenizeVimKeys(lhsText)
		if len(lhs) == 0 {
			ignore("mapping has an empty left side")
			continue
		}
		context := vimMappingContext(match[1])
		if ex := vimExOfRhs(rhsText); ex != "" {
			out.Mappings = append(out.Mappings, vimRcMapping{
				Mode: context, LHS: lhs, Ex: ex, Noremap: match[2] == "noremap",
			})
		} else {
			out.Mappings = append(out.Mappings, vimRcMapping{
				Mode: context, LHS: lhs, RHS: tokenizeVimKeys(rhsText), Noremap: match[2] == "noremap",
			})
		}
		out.Commands++
	}
	return out
}

func (m *Model) applyVimRc(parsed vimRcParse) {
	m.vim.mappings = map[string][]vimMapping{}
	m.vim.options = parsed.Options
	for _, state := range m.vim.tabs {
		state.mappingKeys = nil
	}
	for _, command := range parsed.Mappings {
		key := vimMappingKey(command.LHS)
		m.vim.mappings[command.Mode] = append(m.vim.mappings[command.Mode], vimMapping{
			Tokens: command.LHS,
			RHS:    command.RHS,
			Ex:     command.Ex,
		})
		_ = key
	}
}

func replaceVimLeader(text, leader string) string {
	return vimLeaderRE.ReplaceAllStringFunc(text, func(string) string {
		return leader
	})
}

func (m *Model) takeVimMapping(st *vimTabState, token string) (vimMapping, bool, bool) {
	st.mappingKeys = append(st.mappingKeys, token)
	contexts := vimMappingContexts(st)
	prefix := false
	for _, context := range contexts {
		for _, mapping := range m.vim.mappings[context] {
			if len(st.mappingKeys) > len(mapping.Tokens) {
				continue
			}
			if !sameVimTokens(st.mappingKeys, mapping.Tokens[:len(st.mappingKeys)]) {
				continue
			}
			if len(st.mappingKeys) == len(mapping.Tokens) {
				st.mappingKeys = nil
				return mapping, false, true
			}
			prefix = true
		}
	}
	if prefix {
		return vimMapping{}, true, false
	}
	st.mappingKeys = nil
	return vimMapping{}, false, false
}

func (m *Model) vimMappingMayStart(st *vimTabState, token string) bool {
	candidate := append(append([]string(nil), st.mappingKeys...), token)
	for _, context := range vimMappingContexts(st) {
		for _, mapping := range m.vim.mappings[context] {
			if len(candidate) <= len(mapping.Tokens) &&
				sameVimTokens(candidate, mapping.Tokens[:len(candidate)]) {
				return true
			}
		}
	}
	return false
}

func vimMappingContexts(st *vimTabState) []string {
	switch st.mode {
	case vimInsert:
		return []string{"insert"}
	case vimVisual, vimVisualLine:
		return []string{"visual", "all"}
	default:
		if st.operator != 0 {
			return []string{"operator", "all"}
		}
		return []string{"normal", "all"}
	}
}

func vimMappingContext(prefix string) string {
	switch prefix {
	case "n":
		return "normal"
	case "v", "x":
		return "visual"
	case "i":
		return "insert"
	case "o":
		return "operator"
	default:
		return "all"
	}
}

func vimMappingKey(tokens []string) string { return strings.Join(tokens, "\x1f") }

func sameVimTokens(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func parseVimSetOption(field string) (string, string, bool) {
	if name, value, ok := strings.Cut(field, "="); ok {
		if name == "" {
			return "", "", false
		}
		return name, value, true
	}
	if strings.HasPrefix(field, "no") && len(field) > 2 {
		return field[2:], "false", true
	}
	for _, r := range field {
		if !unicode.IsLetter(r) {
			return "", "", false
		}
	}
	return field, "true", field != ""
}

func applyVimOption(options *vimOptions, name, value string) bool {
	switch name {
	case "textwidth", "tw":
		n, err := strconv.Atoi(value)
		if err != nil {
			return false
		}
		options.TextWidth = max(0, n)
	case "pcre":
		options.PCRE = value != "false"
	case "insertModeEscKeysTimeout":
		n, err := strconv.Atoi(value)
		if err != nil {
			return false
		}
		options.InsertModeEscKeysTimeout = max(0, n)
	case "langmap":
		options.Langmap = value
	case "lmap":
		options.Lmap = value
	default:
		return false
	}
	return true
}

func parseVimLetValue(rest string) (string, bool) {
	rest = strings.TrimSpace(rest)
	if !strings.HasPrefix(rest, "=") {
		return "", false
	}
	rest = strings.TrimSpace(strings.TrimPrefix(rest, "="))
	if len(rest) < 2 || rest[0] != rest[len(rest)-1] || rest[0] != '"' && rest[0] != '\'' {
		return "", false
	}
	value := rest[1 : len(rest)-1]
	if rest[0] == '"' {
		value = strings.ReplaceAll(value, `\\`, `\`)
		value = strings.ReplaceAll(value, `\<`, `<`)
	}
	return value, true
}

func normalizeVimLeader(value string) string {
	if value == " " || strings.EqualFold(value, "<space>") || value == `\ <space>` {
		return "<Space>"
	}
	return value
}

func removeVimMapFlags(fields []string) []string {
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		switch strings.ToLower(field) {
		case "<silent>", "<unique>", "<nowait>", "<buffer>", "<expr>", "<script>":
			continue
		}
		out = append(out, field)
	}
	return out
}

func vimExOfRhs(rhs string) string {
	rhs = strings.TrimSpace(rhs)
	lower := strings.ToLower(rhs)
	if strings.HasPrefix(lower, "<action>(") && strings.HasSuffix(rhs, ")") {
		return "action " + strings.TrimSpace(rhs[len("<Action>("):len(rhs)-1])
	}
	for _, ending := range []string{"<CR>", "<Enter>", "<Return>"} {
		if !strings.HasSuffix(lower, strings.ToLower(ending)) {
			continue
		}
		command := strings.TrimSpace(rhs[:len(rhs)-len(ending)])
		command = strings.TrimPrefix(command, ":")
		if strings.HasPrefix(strings.ToLower(command), "<c-u>") {
			command = command[len("<C-u>"):]
		}
		return strings.TrimSpace(command)
	}
	return ""
}

func tokenizeVimKeys(keys string) []string {
	var out []string
	for i := 0; i < len(keys); {
		if keys[i] == '<' {
			if i+1 < len(keys) && keys[i+1] == '<' {
				out = append(out, "<")
				i++
				continue
			}
			if end := strings.IndexByte(keys[i+1:], '>'); end >= 0 {
				inner := keys[i+1 : i+1+end]
				if token := normalizeVimSpecialKey(inner); token != "" {
					out = append(out, token)
					i += end + 2
					continue
				}
			}
		}
		r, size := utf8DecodeRune(keys[i:])
		out = append(out, string(r))
		i += size
	}
	return out
}

func normalizeVimSpecialKey(inner string) string {
	parts := strings.Split(inner, "-")
	name := parts[len(parts)-1]
	ctrl, alt, shift, cmd := false, false, false, false
	for _, mod := range parts[:len(parts)-1] {
		switch strings.ToLower(mod) {
		case "c", "ctrl", "control":
			ctrl = true
		case "a", "m", "alt", "meta", "option":
			alt = true
		case "s", "shift":
			shift = true
		case "d", "cmd", "command", "super":
			cmd = true
		default:
			return "<" + inner + ">"
		}
	}
	switch strings.ToLower(name) {
	case "esc", "escape":
		name = "Esc"
	case "cr", "return", "enter":
		name = "Enter"
	case "bs", "backspace":
		name = "Backspace"
	case "del", "delete":
		name = "Delete"
	case "ins", "insert":
		name = "Insert"
	case "space":
		name = "Space"
	case "tab":
		name = "Tab"
	case "lt":
		name = "<"
	case "bar":
		name = "|"
	case "bslash":
		name = `\`
	case "left":
		name = "Left"
	case "right":
		name = "Right"
	case "up":
		name = "Up"
	case "down":
		name = "Down"
	case "home":
		name = "Home"
	case "end":
		name = "End"
	case "pageup":
		name = "PageUp"
	case "pagedown":
		name = "PageDown"
	default:
		lower := strings.ToLower(name)
		if len(lower) >= 2 && lower[0] == 'f' {
			if n, err := strconv.Atoi(lower[1:]); err == nil && n >= 1 && n <= 12 {
				name = "F" + strconv.Itoa(n)
				break
			}
		}
		if len([]rune(name)) != 1 {
			return "<" + inner + ">"
		}
		if !ctrl && !alt && !shift && !cmd {
			return name
		}
		name = strings.ToUpper(name)
	}
	if name == "<" || name == "|" || name == `\` {
		return name
	}
	if !ctrl && !alt && !shift && !cmd {
		return "<" + name + ">"
	}
	if !ctrl && !alt && !cmd && shift && len([]rune(name)) == 1 {
		return name
	}
	mods := make([]string, 0, 4)
	if ctrl {
		mods = append(mods, "Ctrl")
	}
	if alt {
		mods = append(mods, "Alt")
	}
	if shift {
		mods = append(mods, "Shift")
	}
	if cmd {
		mods = append(mods, "Cmd")
	}
	return "<" + strings.Join(append(mods, name), "+") + ">"
}

func vimExpandHome(path, home string) string {
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		return filepath.Join(home, path[2:])
	}
	return path
}

func utf8DecodeRune(s string) (rune, int) {
	for _, r := range s {
		return r, len(string(r))
	}
	return unicode.ReplacementChar, 1
}

func vimConflictActionNames(m *Model) []string {
	var out []string
	for _, action := range actionList() {
		out = append(out, action.Title)
	}
	return out
}

func vimConflictActions(m *Model, stroke string) []string {
	stroke = keymapNormalize(stroke)
	var out []string
	seen := map[string]bool{}
	for _, action := range actionList() {
		for _, binding := range m.KeymapBindings(action.ID) {
			first := strings.Split(binding, " ")[0]
			if first == stroke && !seen[action.Title] {
				seen[action.Title] = true
				out = append(out, action.Title)
			}
		}
	}
	return out
}

func (m *Model) setVimHandler(stroke, handler string) {
	stroke = keymapNormalize(stroke)
	handler = strings.ToLower(strings.TrimSpace(handler))
	if handler != "vim" && handler != "ide" {
		return
	}
	if m.settingsDraft.vimHandlers == nil {
		m.settingsDraft.vimHandlers = map[string]string{}
	}
	defaultHandler := ""
	for _, key := range vimKeys {
		if keymapNormalize(key.stroke) == stroke {
			defaultHandler = key.handler
			break
		}
	}
	if handler == defaultHandler {
		delete(m.settingsDraft.vimHandlers, stroke)
	} else {
		m.settingsDraft.vimHandlers[stroke] = handler
	}
	m.invalidate()
}

func cloneVimHandlers(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	out := make(map[string]string, len(source))
	for stroke, handler := range source {
		normalized := keymapNormalize(stroke)
		if normalized == "" || handler != "vim" && handler != "ide" {
			continue
		}
		out[normalized] = handler
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func sameVimHandlers(a, b map[string]string) bool {
	a, b = cloneVimHandlers(a), cloneVimHandlers(b)
	if len(a) != len(b) {
		return false
	}
	for stroke, handler := range a {
		if b[stroke] != handler {
			return false
		}
	}
	return true
}

func vimRcDescription(state vimRcState) string {
	if state.Path == "" {
		return "No Vim rc file found."
	}
	return fmt.Sprintf("%s · %d mappings", state.Path, state.Mappings)
}
