package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	"mayoi/internal/adapter"
	"mayoi/internal/openidea"
)

type stackTraceTab struct {
	title string
	lines []string
	list  ui.ListState
}
type stackTraceState struct {
	open   bool
	text   string
	tabs   []stackTraceTab
	active int // -1 is the Run output
}

type traceLink struct {
	start, end int
	location   adapter.TestLocation
	url        string
}

var (
	tracePython    = regexp.MustCompile(`File "([^"]+)", line ([0-9]+)`)
	traceJVM       = regexp.MustCompile(`(?:([\w$]+(?:\.[\w$]+)*)\.[\w$<>-]+)?\(([\w$.-]+\.(?:java|kt|kts|scala|groovy|clj)):([0-9]+)\)`)
	tracePath      = regexp.MustCompile(`(?:[A-Za-z]:[\\/]|~?/|\.{1,2}/)?(?:[\w.@+\-]+[\\/])*[\w@+\-][\w.@+\-]*\.[A-Za-z][\w]{0,9}` + `(?::\[?([0-9]+)(?::([0-9]+)|,([0-9]+)\])?|\(([0-9]+)(?:,([0-9]+))?\))`)
	traceURL       = regexp.MustCompile(`\bhttps?://[^\s<>"'` + "`" + `]+`)
	traceException = regexp.MustCompile(`^(?:Exception in thread "[^"]*"\s+)?(?:[\w$]+\.)*([\w$]+(?:Exception|Error))\b`)
)

func normalizeStackTrace(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimRightFunc(lines[i], unicode.IsSpace)
	}
	return strings.Trim(strings.Join(lines, "\n"), "\n")
}

func stackTraceTitle(s string) string {
	lines := strings.Split(s, "\n")
	head := ""
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if head == "" {
			head = line
		}
		if !strings.HasPrefix(line, "at ") && (strings.Contains(line, "Exception") || strings.Contains(line, "Error") || strings.Contains(line, "panicked") || strings.Contains(line, "Traceback") || strings.HasPrefix(line, "Full thread dump")) {
			head = line
			break
		}
	}
	if head == "" {
		return "Stack trace"
	}
	if match := traceException.FindStringSubmatch(head); match != nil {
		head = match[1]
	}
	runes := []rune(head)
	if len(runes) > 40 {
		return string(runes[:39]) + "…"
	}
	return head
}

func trimTraceURL(value string) string {
	for value != "" {
		last := value[len(value)-1]
		if strings.ContainsRune(".,;:!?'\"", rune(last)) {
			value = value[:len(value)-1]
			continue
		}
		if last == ')' && strings.Count(value, "(") < strings.Count(value, ")") {
			value = value[:len(value)-1]
			continue
		}
		break
	}
	return value
}

// traceLinks recognizes the same source locations and URLs as OpenIDEA's run
// console. More specific file patterns win over the generic path pattern.
func traceLinks(line string) []traceLink {
	if len(line) > 4000 {
		return nil
	}
	var links []traceLink
	add := func(start, end int, loc adapter.TestLocation, url string) {
		for _, l := range links {
			if start < l.end && end > l.start {
				return
			}
		}
		links = append(links, traceLink{start: start, end: end, location: loc, url: url})
	}
	num := func(s string) int { n, _ := strconv.Atoi(s); return n }
	for _, m := range traceURL.FindAllStringIndex(line, -1) {
		url := trimTraceURL(line[m[0]:m[1]])
		if url != "" {
			add(m[0], m[0]+len(url), adapter.TestLocation{}, url)
		}
	}
	for _, m := range tracePython.FindAllStringSubmatchIndex(line, -1) {
		add(m[0], m[1], adapter.TestLocation{Path: line[m[2]:m[3]], Line: num(line[m[4]:m[5]]), Resolve: "path"}, "")
	}
	for _, m := range traceJVM.FindAllStringSubmatchIndex(line, -1) {
		class := ""
		if m[2] >= 0 {
			class = line[m[2]:m[3]]
		}
		file := line[m[4]:m[5]]
		path := file
		if i := strings.LastIndex(class, "."); i >= 0 {
			path = strings.ReplaceAll(class[:i], ".", "/") + "/" + file
		}
		add(m[4], m[7], adapter.TestLocation{Path: path, Line: num(line[m[6]:m[7]]), Resolve: "name", ClassName: class}, "")
	}
	for _, m := range tracePath.FindAllStringSubmatchIndex(line, -1) {
		if m[0] > 0 && strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_/.~-", rune(line[m[0]-1])) {
			continue
		}
		part := func(i int) string {
			if m[i] < 0 {
				return ""
			}
			return line[m[i]:m[i+1]]
		}
		// The position immediately before the first number starts the suffix.
		numberStart := m[2]
		if numberStart < 0 {
			numberStart = m[8]
		}
		pathEnd := numberStart - 1
		if line[pathEnd] == '[' {
			pathEnd--
		}
		path := line[m[0]:pathEnd]
		lineNo, col := part(2), part(4)
		if lineNo == "" {
			lineNo = part(8)
		}
		if col == "" {
			col = part(6)
		}
		if col == "" {
			col = part(10)
		}
		add(m[0], m[1], adapter.TestLocation{Path: path, Line: num(lineNo), Col: num(col), Resolve: "path"}, "")
	}
	for i := 1; i < len(links); i++ {
		for j := i; j > 0 && links[j].start < links[j-1].start; j-- {
			links[j], links[j-1] = links[j-1], links[j]
		}
	}
	return links
}

func countStackFrames(s string) int {
	n := 0
	for _, line := range strings.Split(s, "\n") {
		for _, link := range traceLinks(line) {
			if link.url == "" && link.location.Path != "" {
				n++
				break
			}
		}
	}
	return n
}

func (m *Model) openTraceLink(link traceLink, c *ui.Context) {
	if link.url != "" {
		c.OpenURL(link.url)
		return
	}
	m.openTraceSource(link.location)
}

func (m *Model) OpenStackTrace() {
	if m.Root() == "" {
		return
	}
	if m.stackTrace == nil {
		m.stackTrace = &stackTraceState{active: -1}
	}
	s := m.stackTrace
	s.open, s.text = true, ""
	if data, err := mygo.Clipboard.Read(transfer.Text); err == nil {
		if raw, err := data.Read(transfer.Text); err == nil && countStackFrames(normalizeStackTrace(string(raw))) > 0 {
			s.text = string(raw)
		}
	}
	m.invalidate()
}

func (m *Model) analyzeStackTrace() bool {
	s := m.stackTrace
	if s == nil {
		return false
	}
	text := normalizeStackTrace(s.text)
	if strings.TrimSpace(text) == "" {
		return false
	}
	s.tabs = append(s.tabs, stackTraceTab{title: stackTraceTitle(text), lines: strings.Split(text, "\n")})
	s.active = len(s.tabs) - 1
	s.open = false
	s.text = ""
	m.DockShow("run")
	m.invalidate()
	return true
}

func (m *Model) closeStackTraceTab(index int) {
	s := m.stackTrace
	if s == nil || index < 0 || index >= len(s.tabs) {
		return
	}
	s.tabs = slices.Delete(s.tabs, index, index+1)
	if s.active == index {
		s.active = -1
	} else if s.active > index {
		s.active--
	}
	m.invalidate()
}

func (m *Model) stackTraceDialog(c *ui.Context, p *ui.Theme) {
	s := m.stackTrace
	if s == nil || !s.open {
		return
	}
	open := s.open
	dlg := ui.Modal(c, &open, func() {
		ui.Column(c).Width(720).Gap(8).Children(func() {
			ui.Text(c, "Analyze Stack Trace").FontSize(13).Bold().TextColor(p.Text)
			ui.Text(c, "Put a stack trace or a complete thread dump here:").FontSize(12).TextColor(p.TextMuted)
			ui.TextArea(c, &s.text).Height(300).Font("monospace").FontSize(12).AutoFocus()
			ui.Row(c).Gap(8).Children(func() {
				ui.Button(c, "Cancel").OnClick(func() { s.open = false; m.invalidate() })
				ui.Button(c, "Analyze").Disabled(strings.TrimSpace(s.text) == "").OnClick(func() { m.analyzeStackTrace() })
			})
		})
	})
	if dlg.Dismissed() || !open {
		s.open = false
		m.invalidate()
	}
}

func (m *Model) openTraceSource(loc adapter.TestLocation) {
	root, state := m.Root(), m.stackTrace
	if root == "" || loc.Line < 1 || m.ctx == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 5*time.Second)
		defer cancel()
		path := m.traceSourcePath(ctx, root, loc)
		m.post(func() {
			if m.Root() != root || m.stackTrace != state {
				return
			}
			if path == "" {
				m.notifyWarning("stack-trace-source", "Cannot find source file", loc.Path)
				return
			}
			m.revealAt(realPath(path), loc.Line-1, max(loc.Col-1, 0))
		})
	}()
}

func (m *Model) traceSourcePath(ctx context.Context, root string, loc adapter.TestLocation) string {
	path := filepath.FromSlash(loc.Path)
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[2:])
		}
	}
	direct := path
	if !filepath.IsAbs(direct) {
		direct = filepath.Join(root, path)
	}
	if info, err := os.Stat(direct); err == nil && info.Mode().IsRegular() {
		return direct
	}
	base := filepath.Base(path)
	raw, err := m.invoke(ctx, "find_files", map[string]any{"query": base, "limit": 60})
	var files []openidea.FileMatch
	if err != nil || json.Unmarshal(raw, &files) != nil {
		return ""
	}
	first := ""
	for _, file := range files {
		if filepath.Base(file.Path) != base {
			continue
		}
		if file.RelPath == path || strings.HasSuffix(filepath.ToSlash(file.Path), "/"+filepath.ToSlash(path)) {
			return file.Path
		}
		if first == "" {
			first = file.Path
		}
	}
	return first
}

// renderStackTraces draws the Run tabs and, when selected, the trace console.
func (m *Model) renderStackTraces(c *ui.Context, p *ui.Theme) bool {
	s := m.stackTrace
	if s == nil || len(s.tabs) == 0 {
		return false
	}
	ui.Row(c).Gap(4).Children(func() {
		ui.Button(c, "Run Output").OnClick(func() { s.active = -1; m.invalidate() })
		for i, tab := range s.tabs {
			i, tab := i, tab
			ui.Button(c.Key(fmt.Sprintf("trace-tab:%d", i)), tab.title).OnClick(func() { s.active = i; m.invalidate() })
			ui.Button(c.Key(fmt.Sprintf("trace-close:%d", i)), "×").OnClick(func() { m.closeStackTraceTab(i) })
		}
	})
	if s.active < 0 || s.active >= len(s.tabs) {
		return false
	}
	tab := &s.tabs[s.active]
	ui.List(c, &tab.list, len(tab.lines), func(i int) {
		line := tab.lines[i]
		ui.Row(c).Height(18).Children(func() {
			pos := 0
			for _, link := range traceLinks(line) {
				if link.start > pos {
					ui.Text(c, line[pos:link.start]).FontSize(12).Font(oiFontMono).TextColor(p.Text).SingleLine()
				}
				ui.Text(c, line[link.start:link.end]).FontSize(12).Font(oiFontMono).TextColor(p.Accent).SingleLine().
					OnClick(func() { m.openTraceLink(link, c) })
				pos = link.end
			}
			if pos < len(line) {
				ui.Text(c, line[pos:]).FontSize(12).Font(oiFontMono).TextColor(p.Text).SingleLine()
			}
		})
	}).Grow(1).MinHeight(0).Padding(8, 10)
	return true
}
