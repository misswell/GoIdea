package app

// Run Anything, ported from OpenIDEA's features/run-anything: one command
// line over the project's recent commands and its run configurations. Enter
// runs the chosen row or the line itself, Tab completes the line from the
// chosen row, and a line no configuration claims runs in the platform shell
// from the project folder, its output in the Run tool window.

import (
	"encoding/json"
	"fmt"
	"runtime"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

// runAnythingGroupLimit is how many rows each section shows, as OpenIDEA's
// GROUP_LIMIT.
const runAnythingGroupLimit = 8

// runAnythingState is the open Run Anything popup.
type runAnythingState struct {
	open  bool
	query string
	rows  []runAnythingRow
	// sel is the chosen row among the item rows, -1 while Enter runs the line.
	sel int
}

// runAnythingRow is a section header or one entry the popup offers.
type runAnythingRow struct {
	section string // the header's label; empty on item rows
	isItem  bool
	label   string
	detail  string
	line    string // the command line a Recent row runs
	config  int    // index into m.run.configs, -1 when the row is not one
}

// OpenRunAnything opens the popup over the window, as runAnything.show does.
func (m *Model) OpenRunAnything() {
	if m.Root() == "" || m.runAnything != nil {
		return
	}
	m.runAnything = &runAnythingState{open: true, sel: -1}
	m.refreshRunAnything()
	if m.run.trusted && !m.run.loaded {
		m.loadRunConfigs()
	}
	m.invalidate()
}

// closeRunAnything drops the popup.
func (m *Model) closeRunAnything() {
	if m.runAnything == nil {
		return
	}
	m.runAnything = nil
	m.invalidate()
}

// refreshRunAnything rebuilds the rows for the typed text and forgets the
// chosen row, as changing the input does upstream.
func (m *Model) refreshRunAnything() {
	v := m.runAnything
	if v == nil {
		return
	}
	v.rows = buildRunAnythingRows(strings.TrimSpace(v.query), m.runAnythingHistory(), m.run.configs)
	v.sel = -1
}

// runAnythingHistory is the open project's remembered commands.
func (m *Model) runAnythingHistory() []string {
	root := m.Root()
	if root == "" {
		return nil
	}
	return append([]string(nil), m.runHistory[layoutKey(root)]...)
}

// pushRunAnythingHistory remembers line for the open project, once, and writes
// the session so the history survives a quit.
func (m *Model) pushRunAnythingHistory(line string) {
	root := m.Root()
	if root == "" {
		return
	}
	if m.runHistory == nil {
		m.runHistory = map[string][]string{}
	}
	m.runHistory[layoutKey(root)] = pushRunHistory(m.runHistory, root, line)
	m.saveSession()
}

// clearRunAnythingHistory forgets the open project's commands.
func (m *Model) clearRunAnythingHistory() {
	root := m.Root()
	if root == "" {
		return
	}
	if _, ok := m.runHistory[layoutKey(root)]; ok {
		delete(m.runHistory, layoutKey(root))
		m.saveSession()
	}
	m.refreshRunAnything()
	m.invalidate()
}

// runAnythingMatches reports whether every word of query occurs in text,
// case-insensitive.
func runAnythingMatches(text, query string) bool {
	t := strings.ToLower(text)
	for _, w := range strings.Fields(strings.ToLower(query)) {
		if !strings.Contains(t, w) {
			return false
		}
	}
	return true
}

// runAnythingConfigs are the indices of the configurations whose name contains
// every word of query, prefix matches first.
func runAnythingConfigs(configs []openidea.RunConfig, query string) []int {
	words := strings.Fields(strings.ToLower(query))
	q := strings.ToLower(strings.TrimSpace(query))
	out := make([]int, 0, len(configs))
	for i, c := range configs {
		name := strings.ToLower(c.Name)
		ok := true
		for _, w := range words {
			if !strings.Contains(name, w) {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, i)
		}
	}
	if q != "" {
		sort.SliceStable(out, func(a, b int) bool {
			pa := strings.HasPrefix(strings.ToLower(configs[out[a]].Name), q)
			pb := strings.HasPrefix(strings.ToLower(configs[out[b]].Name), q)
			return pa && !pb
		})
	}
	return out
}

// buildRunAnythingRows lists the matching recent commands, then the matching
// configurations under their group, as OpenIDEA's buildRows does.
func buildRunAnythingRows(query string, history []string, configs []openidea.RunConfig) []runAnythingRow {
	var rows, recent, found []runAnythingRow
	for _, h := range history {
		if len(recent) == runAnythingGroupLimit {
			break
		}
		if !runAnythingMatches(h, query) {
			continue
		}
		recent = append(recent, runAnythingRow{isItem: true, label: h, line: h, config: -1})
	}
	if len(recent) > 0 {
		rows = append(rows, runAnythingRow{section: "Recent"})
		rows = append(rows, recent...)
	}
	for _, i := range runAnythingConfigs(configs, query) {
		if len(found) == runAnythingGroupLimit {
			break
		}
		c := configs[i]
		found = append(found, runAnythingRow{isItem: true, label: c.Name, detail: c.Kind, config: i})
	}
	if len(found) > 0 {
		rows = append(rows, runAnythingRow{section: "Run Configurations"})
		rows = append(rows, found...)
	}
	return rows
}

// itemCount is how many rows the popup can choose.
func (v *runAnythingState) itemCount() int {
	n := 0
	for _, r := range v.rows {
		if r.isItem {
			n++
		}
	}
	return n
}

// stepRunAnything moves the chosen row by delta, wrapping at the ends.
func (m *Model) stepRunAnything(delta int) {
	v := m.runAnything
	if v == nil {
		return
	}
	n := v.itemCount()
	if n == 0 {
		return
	}
	if v.sel < 0 {
		if delta > 0 {
			v.sel = 0
		} else {
			v.sel = n - 1
		}
	} else {
		v.sel = (v.sel + delta%n + n) % n
	}
	m.invalidate()
}

// selectedRunAnythingRow is the chosen item row, or nil when the line itself
// is Enter's target.
func (m *Model) selectedRunAnythingRow() *runAnythingRow {
	v := m.runAnything
	if v == nil || v.sel < 0 {
		return nil
	}
	i := 0
	for ri := range v.rows {
		if !v.rows[ri].isItem {
			continue
		}
		if i == v.sel {
			return &v.rows[ri]
		}
		i++
	}
	return nil
}

// completeRunAnything puts the chosen row's command into the input.
func (m *Model) completeRunAnything() {
	v := m.runAnything
	if v == nil {
		return
	}
	row := m.selectedRunAnythingRow()
	if row == nil {
		for ri := range v.rows {
			if v.rows[ri].isItem {
				row = &v.rows[ri]
				break
			}
		}
	}
	if row == nil {
		return
	}
	v.query = row.label
	m.refreshRunAnything()
	m.invalidate()
}

// runSelectedRunAnything runs the chosen row, or the typed line when no row is
// chosen.
func (m *Model) runSelectedRunAnything() {
	v := m.runAnything
	if v == nil {
		return
	}
	if row := m.selectedRunAnythingRow(); row != nil {
		m.runAnythingRow(*row)
		return
	}
	line := v.query
	m.closeRunAnything()
	m.runAnythingLine(line)
}

// runAnythingRow runs one row: a configuration starts as the selected one, a
// recent command runs as a typed line.
func (m *Model) runAnythingRow(r runAnythingRow) {
	if r.config >= 0 {
		if r.config >= len(m.run.configs) {
			return
		}
		name := m.run.configs[r.config].Name
		m.closeRunAnything()
		m.pushRunAnythingHistory(name)
		m.run.inline = nil
		m.run.sel = r.config
		m.startRun()
		return
	}
	m.closeRunAnything()
	m.runAnythingLine(r.line)
}

// runAnythingLine runs a command line: a configuration of exactly that name,
// else the platform shell in the project folder.
func (m *Model) runAnythingLine(line string) {
	line = strings.TrimSpace(line)
	root := m.Root()
	if line == "" || root == "" {
		return
	}
	m.pushRunAnythingHistory(line)
	m.closeRunAnything()
	if !m.run.trusted {
		m.run.visible = true
		m.occupyBottom("run")
		m.status = "未信任的项目不会构建或运行"
		m.invalidate()
		return
	}
	for i, c := range m.run.configs {
		if c.Name == line {
			m.run.inline = nil
			m.run.sel = i
			m.startRun()
			return
		}
	}
	cfg, raw, err := runAnythingShellConfig(line, root)
	if err != nil {
		m.run.visible = true
		m.occupyBottom("run")
		m.run.err = "启动失败：" + err.Error()
		m.invalidate()
		return
	}
	if m.startRunWith(raw, cfg.Name) {
		m.run.inline = &cfg
		m.invalidate()
	}
}

// runAnythingShellConfig is the temporary configuration a command line runs
// as, as OpenIDEA's shellRunConfig: it goes through run_start like any other
// configuration and its output lands in the Run tool window.
func runAnythingShellConfig(line, cwd string) (openidea.RunConfig, json.RawMessage, error) {
	name := line
	if r := []rune(line); len(r) > 60 {
		name = string(r[:57]) + "…"
	}
	command, args := "/bin/sh", []string{"-c", line}
	if runtime.GOOS == "windows" {
		command, args = "cmd.exe", []string{"/d", "/s", "/c", line}
	}
	id := "runAnything:" + line
	raw, err := json.Marshal(map[string]any{
		"id": id, "name": name, "kind": "external",
		"command": command, "args": args, "cwd": cwd,
		"env": map[string]string{}, "detected": false,
		"isTest": false, "debugAdapter": nil,
	})
	if err != nil {
		return openidea.RunConfig{}, nil, err
	}
	return openidea.RunConfig{ID: id, Name: name, Kind: "external", Command: command}, raw, nil
}

// runAnythingDialog draws the popup: the title with Clear History, the command
// line, the rows, and the footer OpenIDEA shows.
func (m *Model) runAnythingDialog(c *ui.Context, p *ui.Theme) {
	v := m.runAnything
	if v == nil {
		return
	}
	// The configurations arrive after trust; refresh so their rows appear.
	v.rows = buildRunAnythingRows(strings.TrimSpace(v.query), m.runAnythingHistory(), m.run.configs)
	if n := v.itemCount(); v.sel >= n {
		v.sel = -1
	}
	history := m.runAnythingHistory()
	open := v.open
	d := ui.Modal(c, &open, func() {
		ui.Column(c).Width(620).Gap(0).Children(func() {
			ui.Row(c).Height(26).Padding(4, 10).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Run Anything").FontSize(11.5).Bold().TextColor(p.Text).SingleLine().Grow(1)
				if len(history) > 0 {
					link := ui.ButtonBase(c.Key("runanything:clear")).Height(20).Padding(0, 2).
						Radius(4).AlignItems(ui.Center).Cursor(ui.CursorPointer)
					if link.Hovered() {
						link.Background(p.SurfaceHover)
					}
					link.Children(func() {
						ui.Text(c, "Clear History").FontSize(10.5).TextColor(p.Accent).SingleLine()
					})
					if link.Clicked() {
						m.clearRunAnythingHistory()
					}
				}
			})
			ui.Row(c).Padding(6, 10).Gap(6).AlignItems(ui.Center).
				BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
				ui.Icon(c, iconRun).Size(16, 16).TextColor(p.TextMuted)
				in := ui.TextInput(c, &v.query).Grow(1).AutoFocus().
					Placeholder("Run a command, configuration or goal")
				in.OnChange(func() { m.refreshRunAnything() })
				in.OnShortcut(0, ui.KeyDown, func() { m.stepRunAnything(1) })
				in.OnShortcut(0, ui.KeyUp, func() { m.stepRunAnything(-1) })
				in.OnShortcut(0, ui.KeyPageDown, func() { m.stepRunAnything(5) })
				in.OnShortcut(0, ui.KeyPageUp, func() { m.stepRunAnything(-5) })
				in.OnShortcut(0, ui.KeyTab, func() { m.completeRunAnything() })
				if in.Submitted() {
					m.runSelectedRunAnything()
				}
			})
			if v.itemCount() == 0 {
				text := "Type a command"
				if strings.TrimSpace(v.query) != "" {
					text = "Press Enter to run the command in the project directory"
				}
				ui.Row(c).Padding(6, 10).Children(func() {
					ui.Text(c, text).FontSize(10.5).TextColor(p.TextMuted).SingleLine().Ellipsis("…")
				})
			} else {
				ui.Scroll(c).Height(360).Children(func() {
					ui.Column(c).Children(func() {
						i := 0
						for ri := range v.rows {
							r := v.rows[ri]
							if !r.isItem {
								ui.Row(c).Height(22).Padding(0, 10).Gap(10).AlignItems(ui.Center).Children(func() {
									ui.Text(c, r.section).FontSize(10.5).TextColor(p.TextMuted).SingleLine()
									ui.Row(c).Height(1).Grow(1).Background(p.Border)
								})
								continue
							}
							idx := i
							i++
							row := ui.Row(c.Key(fmt.Sprintf("runanything:%d", idx))).Height(24).
								Padding(0, 10).Gap(8).AlignItems(ui.Center)
							if idx == v.sel || row.Hovered() {
								row.Background(p.SurfaceHover)
							}
							row.Children(func() {
								ui.Icon(c, iconRun).Size(16, 16).TextColor(p.TextMuted)
								ui.Text(c, r.label).FontSize(12).TextColor(p.Text).
									SingleLine().Ellipsis("…").Grow(1)
								if r.detail != "" {
									ui.Text(c, r.detail).FontSize(10.5).TextColor(p.TextMuted).SingleLine()
								}
							})
							if row.Clicked() {
								v.sel = idx
								m.runSelectedRunAnything()
							}
						}
					})
				})
			}
			ui.Row(c).Padding(6, 10).BorderWidth(1, 0, 0, 0).BorderColor(p.Border).Children(func() {
				ui.Text(c, "Enter to run, Tab to complete. Other commands run in the project directory.").
					FontSize(10.5).TextColor(p.TextMuted).SingleLine().Ellipsis("…")
			})
		})
	})
	if d.Dismissed() || !open {
		m.closeRunAnything()
	}
}
