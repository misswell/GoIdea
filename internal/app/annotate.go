package app

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

// annotateState is the Annotate sheet of one file: the commit that last
// touched each of its lines.
type annotateState struct {
	open    bool
	path    string
	loading bool
	err     string
	rows    []annotateRow
}

// annotateRow is a run of consecutive lines from the same commit, as the
// annotate view groups them.
type annotateRow struct {
	Commit  string
	Author  string
	TimeMs  float64
	Summary string
	From    int
	To      int
}

// tabBlameState is the live annotation gutter of one editor tab, as
// OpenIDEA's blame compartment is per editor. request drops replies that a
// newer request superseded, and version is the buffer version the gutter
// describes.
type tabBlameState struct {
	lines            []openidea.BlameLine
	loading          bool
	ready            bool
	version          uint64
	scheduledVersion uint64
	request          uint64
	hoverCommit      string
	commits          map[string]blameCommitState
	layoutDateFormat DateFormatPrefs
	layoutDateWidth  float32
	layoutNameWidth  float32
	layoutReady      bool
}

// blameCommitState is the full commit behind a blame line. OpenIDEA loads it
// lazily when the gutter's rich popup opens, so the first hover can show the
// blame line immediately and fill in email and message when vcs_log replies.
type blameCommitState struct {
	loading bool
	commit  openidea.Commit
	err     string
}

// annotateRows groups blame lines by commit, numbering from 1.
func annotateRows(lines []openidea.BlameLine) []annotateRow {
	out := make([]annotateRow, 0, len(lines))
	for i, l := range lines {
		n := i + 1
		if len(out) > 0 {
			last := &out[len(out)-1]
			if last.Commit == l.Commit && last.Author == l.Author &&
				last.Summary == l.Summary && last.TimeMs == l.TimeMs {
				last.To = n
				continue
			}
		}
		out = append(out, annotateRow{
			Commit: l.Commit, Author: l.Author, TimeMs: l.TimeMs,
			Summary: l.Summary, From: n, To: n,
		})
	}
	return out
}

// annotateLines is one row's line range, as "12" or "12–18".
func annotateLines(r annotateRow) string {
	if r.To > r.From {
		return fmt.Sprintf("%d–%d", r.From, r.To)
	}
	return fmt.Sprintf("%d", r.From)
}

// annotateTime is a blame line's compact, local date.
func annotateTime(prefs DateFormatPrefs, region dateRegion, ms float64) string {
	return formatAppDate(prefs, region, ms, true)
}

// ShowAnnotate toggles OpenIDEA's live blame gutter for a file. A file that is
// not open yet opens first, then receives the annotations in its editor tab.
func (m *Model) ShowAnnotate(path string) {
	if path == "" || m.Root() == "" {
		return
	}
	path = realPath(path)
	m.openFileThen(path, func(tab *Tab) { m.toggleTabBlame(tab) })
}

// annotationsShown reports whether path's open editor currently has blame in
// its gutter. A tab is the annotation owner, matching OpenIDEA's per-editor
// compartment.
func (m *Model) annotationsShown(path string) bool {
	path = realPath(path)
	for _, tab := range m.allTabs() {
		if tab != nil && tab.blame != nil && samePath(tabPath(tab), path) {
			return true
		}
	}
	return false
}

func (m *Model) toggleTabBlame(tab *Tab) {
	if tab == nil || tab.doc == nil || tab.kind != "" {
		return
	}
	if tab.blame != nil {
		tab.blame = nil
		m.status = "Git annotations closed"
		m.invalidate()
		return
	}
	a := &tabBlameState{loading: true}
	tab.blame = a
	m.status = "Reading Git blame…"
	m.loadTabBlame(tab, a)
	m.invalidate()
}

func (m *Model) loadTabBlame(tab *Tab, a *tabBlameState) {
	if tab == nil || tab.doc == nil || a == nil {
		return
	}
	a.loading = true
	a.request++
	request := a.request
	version := tab.doc.Buffer().Version()
	ctx := m.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	go func() {
		callCtx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		raw, err := m.invoke(callCtx, "vcs_blame", map[string]any{
			"path": tab.doc.Path(), "text": nil, "revision": nil,
		})
		var lines []openidea.BlameLine
		if err == nil {
			err = json.Unmarshal(raw, &lines)
		}
		m.post(func() {
			if tab.blame != a || a.request != request || !m.tabOpen(tab) {
				return
			}
			a.loading = false
			a.scheduledVersion = 0
			if err != nil {
				tab.blame = nil
				m.status = "Could not annotate " + filepath.Base(tab.doc.Path())
				m.notifyUser("vcs.annotate", "error", "Cannot annotate", diffErrorText(err))
				m.invalidate()
				return
			}
			a.lines = lines
			a.version = version
			a.ready = true
			a.commits = nil
			m.status = "Git annotations shown for " + filepath.Base(tab.doc.Path())
			m.invalidate()
		})
	}()
}

// refreshTabBlame re-blames after the same 800 ms edit debounce OpenIDEA's
// BlameRefresher uses.
func (m *Model) refreshTabBlame(tab *Tab) {
	a := tab.blame
	if a == nil || a.loading || !a.ready || tab.doc == nil {
		return
	}
	version := tab.doc.Buffer().Version()
	if version == a.version || version == a.scheduledVersion {
		return
	}
	a.scheduledVersion = version
	go func() {
		timer := time.NewTimer(800 * time.Millisecond)
		defer timer.Stop()
		<-timer.C
		m.post(func() {
			if tab.blame != a || a.scheduledVersion != version || !m.tabOpen(tab) {
				return
			}
			a.scheduledVersion = 0
			if tab.doc.Buffer().Version() == version {
				m.loadTabBlame(tab, a)
			} else {
				m.invalidate()
			}
		})
	}()
}

func (m *Model) refreshOpenBlame() {
	for _, tab := range m.allTabs() {
		if tab == nil || tab.blame == nil || tab.blame.loading {
			continue
		}
		m.loadTabBlame(tab, tab.blame)
	}
}

func annotateUncommitted(commit string) bool {
	return commit != "" && strings.Trim(commit, "0") == ""
}

func annotateShortAuthor(name string) string {
	fields := strings.Fields(name)
	if len(fields) == 0 {
		return ""
	}
	runes := []rune(fields[0])
	if len(runes) > 12 {
		return string(runes[:11]) + "…"
	}
	return string(runes)
}

var annotatePalette = [...]ui.Color{
	projectColorPairs[4][0],
	projectColorPairs[6][0],
	projectColorPairs[3][0],
	projectColorPairs[7][0],
	projectColorPairs[5][0],
	projectColorPairs[0][0],
	projectColorPairs[1][0],
	projectColorPairs[8][0],
	projectColorPairs[2][0],
}

// annotateBackgrounds tints each commit from oldest to newest, as
// OpenIDEA's commitBackgrounds does.
func annotateBackgrounds(lines []openidea.BlameLine, background ui.Color) map[string]ui.Color {
	times := make(map[string]float64)
	for _, line := range lines {
		if annotateUncommitted(line.Commit) {
			continue
		}
		if previous, ok := times[line.Commit]; !ok || line.TimeMs > previous {
			times[line.Commit] = line.TimeMs
		}
	}
	commits := make([]string, 0, len(times))
	for commit := range times {
		commits = append(commits, commit)
	}
	sort.Slice(commits, func(i, j int) bool {
		if times[commits[i]] == times[commits[j]] {
			return commits[i] < commits[j]
		}
		return times[commits[i]] < times[commits[j]]
	})
	out := make(map[string]ui.Color, len(commits))
	for i, commit := range commits {
		slot := 0
		if len(commits) > 1 {
			slot = int(float64(i)/float64(len(commits)-1)*float64(len(annotatePalette)-1) + 0.5)
		}
		strength := float32(0.18)
		if len(commits) > len(annotatePalette) && i%2 == 1 {
			strength = 0.24
		}
		out[commit] = background.Mix(annotatePalette[slot], strength)
	}
	return out
}

// nearestGitRoot finds the repository that owns path, including submodules
// whose .git entry is a file. It keeps the commit popup on the right repository
// when the project contains nested repositories.
func nearestGitRoot(path, projectRoot string) string {
	dir := filepath.Dir(filepath.Clean(path))
	stop := filepath.Clean(projectRoot)
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir || (stop != "" && !strings.HasPrefix(dir, stop+string(filepath.Separator)) && dir != stop) {
			return ""
		}
		dir = parent
	}
}

func (m *Model) ensureBlameCommit(tab *Tab, a *tabBlameState, line openidea.BlameLine) {
	if tab == nil || tab.doc == nil || a == nil || annotateUncommitted(line.Commit) {
		return
	}
	if a.commits == nil {
		a.commits = map[string]blameCommitState{}
	}
	if _, exists := a.commits[line.Commit]; exists {
		return
	}
	a.commits[line.Commit] = blameCommitState{loading: true}
	repo := nearestGitRoot(tab.doc.Path(), m.Root())
	var repoArg any
	if repo != "" {
		repoArg = repo
	}
	go func() {
		base := m.ctx
		if base == nil {
			base = context.Background()
		}
		ctx, cancel := context.WithTimeout(base, time.Minute)
		defer cancel()
		raw, err := m.invoke(ctx, "vcs_log", map[string]any{
			"repo": repoArg,
			"query": map[string]any{
				"branch": line.Commit, "path": nil, "author": nil, "text": nil,
				"skip": 0, "limit": 1,
			},
		})
		var commits []openidea.Commit
		if err == nil {
			err = json.Unmarshal(raw, &commits)
		}
		m.post(func() {
			if tab.blame != a || !m.tabOpen(tab) {
				return
			}
			state := a.commits[line.Commit]
			state.loading = false
			if err != nil || len(commits) == 0 {
				if err != nil {
					state.err = diffErrorText(err)
				}
			} else {
				state.commit = commits[0]
			}
			a.commits[line.Commit] = state
			m.invalidate()
		})
	}()
}

// blameGutter draws OpenIDEA's author/date gutter immediately left of the
// editor's own line numbers. The panel follows the editor's line geometry and
// scroll position, including soft-wrapped and folded lines.
func (m *Model) blameGutter(c *ui.Context, p *ui.Theme, tab *Tab) {
	a := tab.blame
	if a == nil || !a.ready || tab.state == nil {
		return
	}
	oi := oiPaletteOf(p)
	backgrounds := annotateBackgrounds(a.lines, p.Background)
	datePrefs := m.prefs.dateFormatPrefs()
	m.blameGutterMetrics(c, a, datePrefs)
	panelWidth := a.layoutDateWidth + a.layoutNameWidth + 20
	panel := ui.Column(c.Key("vcs-blame:"+tab.doc.Path())).Width(panelWidth).Shrink(0).
		MinWidth(0).MinHeight(0).Clip().Background(p.Surface).
		BorderWidth(0, 1, 0, 0).BorderColor(oi.border)
	if !panel.Hovered() && a.hoverCommit != "" {
		a.hoverCommit = ""
		m.invalidate()
	}
	panel.Children(func() {
		first, last := tab.state.VisibleLines()
		first = max(0, first)
		last = min(last, len(a.lines)-1)
		lineHeight := float32(tab.state.Font.Size * tab.state.LineHeight)
		previousTop := float32(-1)
		for line := first; line <= last; line++ {
			top := float32(tab.state.LineTop(line) - tab.state.ScrollY())
			height := float32(tab.state.LineTop(line+1) - tab.state.LineTop(line))
			if height <= 0 {
				height = lineHeight
			}
			// Folded lines share the geometry of the first visible line; one
			// gutter label is enough for that row.
			if top == previousTop {
				continue
			}
			previousTop = top
			if top+height <= 0 {
				continue
			}
			blameLine := a.lines[line]
			uncommitted := annotateUncommitted(blameLine.Commit)
			row := ui.Row(c.Key(fmt.Sprintf("vcs-blame:%s:%d", tab.doc.Path(), line))).
				Absolute().Left(0).Right(0).Top(top).Height(height).
				Padding(0, 6).Gap(8).AlignItems(ui.Center)
			if color, ok := backgrounds[blameLine.Commit]; ok {
				row.Background(color)
			}
			hovered := row.Hovered()
			if hovered && a.hoverCommit != blameLine.Commit {
				a.hoverCommit = blameLine.Commit
				m.invalidate()
			}
			if hovered {
				m.ensureBlameCommit(tab, a, blameLine)
			}
			if hovered || (!uncommitted && a.hoverCommit != "" && a.hoverCommit == blameLine.Commit) {
				row.Background(oi.hoverTint.Mix(oi.textStrong, 0.08))
			}
			row.Children(func() {
				if uncommitted {
					ui.Spacer(c)
					return
				}
				ui.Text(c, annotateTime(datePrefs, c.Region(), blameLine.TimeMs)).Width(a.layoutDateWidth).Shrink(0).
					FontSize(11).TextColor(oi.textSecondary).SingleLine()
				ui.Text(c, annotateShortAuthor(blameLine.Author)).Width(a.layoutNameWidth).Shrink(0).
					FontSize(11).TextColor(oi.textSecondary).SingleLine().Ellipsis("…")
			})
			ui.TooltipBase(c, row, func(tip ui.Element) {
				tip.Width(340).MaxWidthPercent(80).Padding(8, 10).Gap(3).Radius(6).
					Background(oi.popup).Border(1, oi.popupBorder).
					Shadow(0, 8, 24, 0, ui.RGBA(0, 0, 0, 0.28))
				tip.Children(func() {
					if uncommitted {
						ui.Text(c, "Not committed yet").FontSize(12).TextColor(oi.textSecondary).
							SingleLine()
						return
					}
					detail := a.commits[blameLine.Commit]
					commit := detail.commit
					author := lineAuthor(blameLine, commit)
					when := formatAppTimestamp(m.prefs.dateFormatPrefs(), c.Region(), lineTime(blameLine, commit), time.Now())
					message := lineMessage(blameLine, commit)
					subject, body := splitCommitMessage(message)
					ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
						ui.Text(c, "commit").FontSize(12).TextColor(oi.textSecondary)
						hash := ui.Text(c, blameLine.Commit).FontSize(12).Font("monospace").
							TextColor(oi.link).SingleLine()
						hash.Cursor(ui.CursorPointer).OnClick(func() {
							m.ShowCommitInGitLog(blameLine.Commit)
						})
					})
					authorLine := "Author: " + author
					if commit.Email != "" {
						authorLine += " <" + commit.Email + ">"
					}
					ui.Text(c, authorLine).FontSize(12).TextColor(oi.text).Wrap()
					if when != "" {
						ui.Text(c, "Date: "+when).FontSize(12).TextColor(oi.text).
							SingleLine().Ellipsis("…")
					}
					if subject != "" {
						ui.Spacer(c).Height(3)
						issueSubjectText(c, oi, subject, m.prefs.VCS.IssueNavigation)
					}
					if body != "" {
						ui.Spacer(c).Height(3)
						ui.Column(c).Gap(1).Children(func() {
							for _, line := range strings.Split(body, "\n") {
								issueText(c, oi, line, m.prefs.VCS.IssueNavigation)
							}
						})
					}
				})
			})
			row.Cursor(ui.CursorPointer)
			if !uncommitted {
				row.OnClick(func() { m.ShowCommitInGitLog(blameLine.Commit) })
			}
			row.ContextMenu(func(menu *ui.Menu) {
				if !uncommitted {
					if menu.Item("Show Commit in Git Log").Chosen() {
						m.ShowCommitInGitLog(blameLine.Commit)
					}
					if menu.Item("Copy Revision Number").Chosen() {
						mygo.Clipboard.WriteText(blameLine.Commit)
					}
					menu.Separator()
				}
				if menu.Item("Close Annotations").Chosen() {
					tab.blame = nil
					m.invalidate()
				}
			})
		}
	})
}

func (m *Model) blameGutterMetrics(c *ui.Context, a *tabBlameState, datePrefs DateFormatPrefs) {
	if a.layoutReady && a.layoutDateFormat == datePrefs {
		return
	}
	dateWidth, nameWidth := float32(42), float32(54)
	dates, names := map[string]bool{}, map[string]bool{}
	for _, line := range a.lines {
		if annotateUncommitted(line.Commit) {
			continue
		}
		date, name := annotateTime(datePrefs, c.Region(), line.TimeMs), annotateShortAuthor(line.Author)
		if !dates[date] {
			dates[date] = true
			width, _ := c.MeasureText(0, ui.Span{Text: date, Size: 11})
			dateWidth = max(dateWidth, width+2)
		}
		if !names[name] {
			names[name] = true
			width, _ := c.MeasureText(0, ui.Span{Text: name, Size: 11})
			nameWidth = max(nameWidth, width+2)
		}
	}
	a.layoutDateFormat = datePrefs
	a.layoutDateWidth = float32(math.Ceil(float64(dateWidth)))
	a.layoutNameWidth = float32(math.Ceil(float64(nameWidth)))
	a.layoutReady = true
}

func lineAuthor(line openidea.BlameLine, commit openidea.Commit) string {
	if commit.Author != "" {
		return commit.Author
	}
	return line.Author
}

func lineTime(line openidea.BlameLine, commit openidea.Commit) float64 {
	if commit.TimeMs > 0 {
		return commit.TimeMs
	}
	return line.TimeMs
}

func lineMessage(line openidea.BlameLine, commit openidea.Commit) string {
	if strings.TrimSpace(commit.Message) != "" {
		return strings.TrimRight(commit.Message, "\r\n\t ")
	}
	return strings.TrimSpace(line.Summary)
}

func splitCommitMessage(message string) (subject, body string) {
	message = strings.ReplaceAll(message, "\r\n", "\n")
	subject, body, _ = strings.Cut(message, "\n")
	subject = strings.TrimSpace(subject)
	body = strings.TrimSpace(body)
	return subject, body
}

// ShowAnnotateVersion annotates a supplied version of path. The merge editor
// uses it for the Yours and Theirs panes, whose text is not necessarily the
// working-tree buffer.
func (m *Model) ShowAnnotateVersion(path, text, revision string) {
	if path == "" || m.Root() == "" {
		return
	}
	path = realPath(path)
	a := &annotateState{open: true, path: path, loading: true}
	m.annotate = a
	m.invalidate()
	go func() {
		base := m.ctx
		if base == nil {
			base = context.Background()
		}
		ctx, cancel := context.WithTimeout(base, time.Minute)
		defer cancel()
		var body *string
		if text != "" {
			body = &text
		}
		var rev *string
		if revision != "" {
			rev = &revision
		}
		raw, err := m.invoke(ctx, "vcs_blame", map[string]any{
			"path": path, "text": body, "revision": rev,
		})
		var lines []openidea.BlameLine
		if err == nil {
			err = json.Unmarshal(raw, &lines)
		}
		m.post(func() {
			if m.annotate != a {
				return
			}
			a.loading = false
			if err != nil {
				a.err = diffErrorText(err)
				m.invalidate()
				return
			}
			a.rows = annotateRows(lines)
			m.invalidate()
		})
	}()
}

// closeAnnotate closes the Annotate sheet.
func (m *Model) closeAnnotate() {
	if m.annotate == nil {
		return
	}
	m.annotate = nil
	m.invalidate()
}

// annotateDialog draws the Annotate sheet.
func (m *Model) annotateDialog(c *ui.Context, p *ui.Theme) {
	a := m.annotate
	if a == nil {
		return
	}
	open := a.open
	dlg := nativeModal(c, &open, func() {
		ui.Column(c).Width(820).Gap(8).Children(func() {
			title := "Annotate · " + filepath.Base(a.path)
			if a.loading {
				title = "Annotate · Reading…"
			}
			ui.Text(c, title).FontSize(13).Bold().TextColor(p.Text).SingleLine()
			if rel, err := filepath.Rel(m.Root(), a.path); err == nil && !strings.HasPrefix(rel, "..") {
				ui.Text(c, filepath.ToSlash(rel)).FontSize(12).TextColor(p.TextMuted).
					SingleLine().Ellipsis("…")
			}
			switch {
			case a.err != "":
				ui.Text(c, a.err).FontSize(12).TextColor(p.Danger).SingleLine().Ellipsis("…")
			case a.loading:
				ui.Text(c, "Reading the commit of each line…").FontSize(12).TextColor(p.TextMuted)
			case len(a.rows) == 0:
				ui.Text(c, "No lines to annotate").FontSize(12).TextColor(p.TextMuted)
			}
			if len(a.rows) > 0 {
				ui.Scroll(c).Height(460).Children(func() {
					ui.Column(c).Children(func() {
						for i, r := range a.rows {
							i, r := i, r
							row := ui.Row(c.Key(fmt.Sprintf("annotate:%d", i))).Height(22).
								Padding(0, 8).Gap(8).AlignItems(ui.Center)
							if row.Hovered() {
								row.Background(p.SurfaceHover)
							}
							row.Children(func() {
								ui.Row(c).Width(56).Shrink(0).Justify(ui.End).Children(func() {
									ui.Text(c, annotateLines(r)).FontSize(12).
										TextColor(p.TextMuted).SingleLine()
								})
								ui.Row(c).Width(72).Shrink(0).Children(func() {
									short := r.Commit
									if len(short) > 8 {
										short = short[:8]
									}
									ui.Text(c, short).FontSize(12).TextColor(p.Accent).SingleLine()
								})
								ui.Row(c).Width(120).Shrink(0).Children(func() {
									ui.Text(c, r.Author).FontSize(12).TextColor(p.Text).
										SingleLine().Ellipsis("…")
								})
								ui.Row(c).Width(86).Shrink(0).Children(func() {
									ui.Text(c, annotateTime(m.prefs.dateFormatPrefs(), c.Region(), r.TimeMs)).FontSize(12).
										TextColor(p.TextMuted).SingleLine()
								})
								ui.Text(c, r.Summary).FontSize(12).TextColor(p.Text).
									SingleLine().Ellipsis("…")
							})
						}
					})
				})
			}
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Spacer(c)
				ui.Button(c, "Close").OnClick(func() { m.closeAnnotate() })
			})
		})
	})
	if dlg.Dismissed() || !open {
		m.closeAnnotate()
	}
}
