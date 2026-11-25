package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

// logLimit is how many commits the Git log asks for at once, as OpenIDEA's
// first window does; the list is read-only, so a fixed window is enough.
const (
	logLimit                        = 200
	vcsContentConsole vcsContentTab = 6
)

// LogPane is the Git tool window's Log tab: the commit list, the selected
// commit's message, branches and signature, and the files that commit changed.
type LogPane struct {
	visible bool
	loading bool
	loaded  bool
	commits []openidea.Commit
	// commitRepos aligns with commits in the project-wide tool-window log.
	// The full Git Log remains repository-scoped when a path or branch filter
	// is active.
	commitRepos []string
	aggregate   bool
	selected    int
	detailFor   string
	detailRepo  string
	details     openidea.LogDetails
	changes     []openidea.RevisionChange
	err         string
	detailErr   string
	detailing   bool
	cancel      context.CancelFunc
	// stale asks for another commit-list read after the one in flight lands.
	stale bool
	// detailGeneration invalidates a detail read when the selected commit or
	// repository changes before the response arrives.
	detailGeneration uint64
	// The changed-files pane selection and IDEA's log display options.
	changeSelected   int
	groupByDirectory bool
	diffPreview      bool
	showBranches     bool
	collapseLinear   bool
	// The Branches pane and its UI-only favorites.
	branches         []openidea.Branch
	branchesLoading  bool
	branchesErr      string
	branchSelected   int
	favoriteBranches map[string]bool
	bookmarks        map[string]string
	// path and branch narrow the log for a file or branch tab. An empty path
	// is the main project log.
	path   string
	branch string
	// The native filter bar mirrors OpenIDEA's Branch, User, Date, Paths and
	// message filters. Text fields remain staged until Apply so a query is not
	// sent for every keystroke.
	filtersOpen     bool
	filterAuthors   string
	filterText      string
	filterPaths     string
	filterBranches  string
	filterSince     string
	filterUntil     string
	filterRegex     bool
	filterMatchCase bool
	filterErr       string
	// console is the Git window's command/output log. Operations may append
	// lines here without coupling the window to a particular RPC dialog.
	console []string
	// repo is an external Git log root; empty uses the open project's repository.
	repo string
}

type gitLogQuerySpec struct {
	authors     []string
	text        string
	paths       []string
	branches    []string
	since       any
	until       any
	regex       bool
	matchCase   bool
	branch      string
	path        string
	sort        string
	firstParent bool
	noMerges    bool
	limit       uint32
}

func (m *Model) resetLog() {
	if m.log.cancel != nil {
		m.log.cancel()
	}
	m.log = LogPane{}
}

// ShowLog opens the Git log panel and loads the commit list.
func (m *Model) ShowLog() {
	if m.Root() == "" {
		return
	}
	m.setGitToolWindowTab(vcsContentLog)
	root := m.vcs.current
	if root != "" && vcsRepositoryKey(m.log.repo) != vcsRepositoryKey(root) {
		if oldRoot := vcsRepositoryKey(m.log.repo); oldRoot != "" {
			m.rememberVCSLog(oldRoot)
		}
		visible := m.log.visible
		m.log = LogPane{}
		if parked, ok := m.vcs.logs[vcsRepositoryKey(root)]; ok {
			m.log = parked
		}
		m.log.repo = root
		m.log.visible = visible
	}
	m.log.path = ""
	m.log.branch = ""
	m.enableVCSAggregateLog(root)
	m.vcs.logGeneration++
	m.log.loading = false
	m.log.detailing = false
	m.applyGitLogPreferences()
	m.DockShow("log")
	if root != "" {
		m.ensureVCSLogForRepository(root)
	}
}

func (m *Model) setGitToolWindowTab(tab vcsContentTab) {
	root := vcsRepositoryKey(m.vcs.current)
	if root == "" {
		return
	}
	if m.vcs.content == nil {
		m.vcs.content = map[string]vcsContentTab{}
	}
	m.vcs.content[root] = tab
}

// CloseLog hides the Git log panel; the next open reloads it.
func (m *Model) CloseLog() { m.DockHide("log") }

func (m *Model) gitLogFiltersActive() bool {
	return m.log.filterAuthors != "" || m.log.filterText != "" ||
		m.log.filterPaths != "" || m.log.filterBranches != "" ||
		m.log.filterSince != "" || m.log.filterUntil != "" ||
		m.log.filterRegex || m.log.filterMatchCase
}

func (m *Model) ApplyGitLogFilters() {
	m.log.filterErr = ""
	if _, err := parseGitLogFilterTime(m.log.filterSince, false); err != nil {
		m.log.filterErr = "Since: " + err.Error()
		m.invalidate()
		return
	}
	if _, err := parseGitLogFilterTime(m.log.filterUntil, true); err != nil {
		m.log.filterErr = "Until: " + err.Error()
		m.invalidate()
		return
	}
	m.log.selected = 0
	m.RefreshLog()
}

func (m *Model) ClearGitLogFilters() {
	m.log.filterAuthors = ""
	m.log.filterText = ""
	m.log.filterPaths = ""
	m.log.filterBranches = ""
	m.log.filterSince = ""
	m.log.filterUntil = ""
	m.log.filterRegex = false
	m.log.filterMatchCase = false
	m.log.filterErr = ""
	m.log.selected = 0
	m.RefreshLog()
}

func (m *Model) gitLogFilterBar(c *ui.Context, p *ui.Theme) {
	l := &m.log
	ui.Column(c).Padding(6, 8).Gap(6).
		Background(p.Surface).BorderWidth(0, 0, 1, 0).BorderColor(p.Border).
		Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Text").FontSize(12).TextColor(p.TextMuted)
				if in := ui.TextInput(c, &l.filterText).Grow(1).
					Placeholder("Commit message or hash"); in.Submitted() {
					m.ApplyGitLogFilters()
				}
				if ui.Checkbox(c, &l.filterRegex, "Regex").Changed() {
					m.log.filterErr = ""
				}
				if ui.Checkbox(c, &l.filterMatchCase, "Match Case").Changed() {
					m.log.filterErr = ""
				}
				ui.Button(c, "Apply").OnClick(m.ApplyGitLogFilters)
				ui.Button(c, "Clear").Disabled(!m.gitLogFiltersActive()).
					OnClick(m.ClearGitLogFilters)
			})
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "User").FontSize(12).TextColor(p.TextMuted)
				if in := ui.TextInput(c, &l.filterAuthors).Width(120).
					Placeholder("author or me"); in.Submitted() {
					m.ApplyGitLogFilters()
				}
				ui.Text(c, "Branches").FontSize(12).TextColor(p.TextMuted)
				if in := ui.TextInput(c, &l.filterBranches).Width(120).
					Placeholder("main, feature/*"); in.Submitted() {
					m.ApplyGitLogFilters()
				}
				ui.Text(c, "Paths").FontSize(12).TextColor(p.TextMuted)
				if in := ui.TextInput(c, &l.filterPaths).Width(150).
					Placeholder("src, docs/*.md"); in.Submitted() {
					m.ApplyGitLogFilters()
				}
				ui.Text(c, "Since").FontSize(12).TextColor(p.TextMuted)
				if in := ui.TextInput(c, &l.filterSince).Width(96).
					Placeholder("YYYY-MM-DD"); in.Submitted() {
					m.ApplyGitLogFilters()
				}
				ui.Text(c, "Until").FontSize(12).TextColor(p.TextMuted)
				if in := ui.TextInput(c, &l.filterUntil).Width(96).
					Placeholder("YYYY-MM-DD"); in.Submitted() {
					m.ApplyGitLogFilters()
				}
			})
			if l.filterErr != "" {
				ui.Text(c, l.filterErr).FontSize(12).TextColor(p.Warning)
			}
		})
}

func splitGitLogFilterValues(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '\n'
	})
	out := make([]string, 0, len(parts))
	seen := make(map[string]bool, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || seen[part] {
			continue
		}
		seen[part] = true
		out = append(out, part)
	}
	return out
}

func gitLogFirstFilter(values []string) (string, []string) {
	if len(values) == 0 {
		return "", nil
	}
	return values[0], append([]string(nil), values[1:]...)
}

func parseGitLogFilterTime(value string, until bool) (any, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	if len(value) >= 11 {
		if millis, err := strconv.ParseInt(value, 10, 64); err == nil {
			return float64(millis), nil
		}
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return float64(parsed.UnixMilli()), nil
	}
	if parsed, err := time.ParseInLocation("2006-01-02 15:04", value, time.Local); err == nil {
		return float64(parsed.UnixMilli()), nil
	}
	if parsed, err := time.ParseInLocation("2006-01-02", value, time.Local); err == nil {
		if until {
			parsed = parsed.AddDate(0, 0, 1)
		}
		return float64(parsed.UnixMilli()), nil
	}
	return nil, fmt.Errorf("use YYYY-MM-DD, RFC 3339 or epoch milliseconds")
}

func (m *Model) gitLogQuerySpec(path, branch string, limit uint32) (gitLogQuerySpec, error) {
	spec := gitLogQuerySpec{
		authors:     splitGitLogFilterValues(m.log.filterAuthors),
		text:        strings.TrimSpace(m.log.filterText),
		paths:       splitGitLogFilterValues(m.log.filterPaths),
		branches:    splitGitLogFilterValues(m.log.filterBranches),
		regex:       m.log.filterRegex,
		matchCase:   m.log.filterMatchCase,
		sort:        m.prefs.VCS.Log.normalizedSort(),
		firstParent: prefBool(m.prefs.VCS.Log.FirstParent, false),
		noMerges:    prefBool(m.prefs.VCS.Log.NoMerges, false),
		limit:       limit,
	}
	var err error
	spec.since, err = parseGitLogFilterTime(m.log.filterSince, false)
	if err != nil {
		return gitLogQuerySpec{}, fmt.Errorf("Since: %w", err)
	}
	spec.until, err = parseGitLogFilterTime(m.log.filterUntil, true)
	if err != nil {
		return gitLogQuerySpec{}, fmt.Errorf("Until: %w", err)
	}
	if branch != "" {
		spec.branch = branch
	}
	if path != "" {
		spec.path = path
	}
	return spec, nil
}

func (m *Model) gitLogQuery(ctx context.Context, repo any, spec gitLogQuerySpec) (map[string]any, error) {
	authors := append([]string(nil), spec.authors...)
	for i, author := range authors {
		if !strings.EqualFold(author, "me") {
			continue
		}
		raw, err := m.invoke(ctx, "vcs_config_get", map[string]any{
			"repo": repo,
			"key":  "user.email",
		})
		if err != nil {
			return nil, fmt.Errorf("Could not read the Git user email: %w", err)
		}
		var email string
		if err := json.Unmarshal(raw, &email); err != nil {
			return nil, fmt.Errorf("Could not read the Git user email: %w", err)
		}
		email = strings.TrimSpace(email)
		if email == "" {
			return nil, fmt.Errorf("Git user.email is not configured")
		}
		authors[i] = email
	}
	author, moreAuthors := gitLogFirstFilter(authors)
	branch, moreBranches := gitLogFirstFilter(spec.branches)
	path, morePaths := gitLogFirstFilter(spec.paths)
	if spec.branch != "" {
		branch = spec.branch
	}
	if spec.path != "" {
		path = spec.path
	}
	var authorArg, textArg, branchArg, pathArg any
	if author != "" {
		authorArg = author
	}
	if spec.text != "" {
		textArg = spec.text
	}
	if branch != "" {
		branchArg = branch
	}
	if path != "" {
		pathArg = path
	}
	return map[string]any{
		"branch":        branchArg,
		"branches":      moreBranches,
		"path":          pathArg,
		"paths":         morePaths,
		"author":        authorArg,
		"authors":       moreAuthors,
		"authorsExact":  false,
		"text":          textArg,
		"textRegex":     spec.regex,
		"textMatchCase": spec.matchCase,
		"skip":          0,
		"limit":         spec.limit,
		"order":         spec.sort,
		"firstParent":   spec.firstParent,
		"noMerges":      spec.noMerges,
		"sinceMs":       spec.since,
		"untilMs":       spec.until,
	}, nil
}

// RefreshLog reloads the commit list in the background.
func (m *Model) RefreshLog() {
	root := m.Root()
	if root == "" || (m.mgr == nil && m.invokeHook == nil) {
		return
	}
	if m.log.aggregate && m.log.path == "" && m.log.branch == "" &&
		len(m.vcs.repositories) > 1 {
		m.refreshAggregateLog(root)
		return
	}
	if m.log.loading {
		m.log.stale = true
		return
	}
	path, branch := m.log.path, m.log.branch
	requestLimit := m.vcsLogRequestLimit(path)
	spec, specErr := m.gitLogQuerySpec(path, branch, uint32(requestLimit))
	if specErr != nil {
		m.log.filterErr = specErr.Error()
		m.invalidate()
		return
	}
	m.log.stale = false
	m.vcs.logGeneration++
	generation := m.vcs.logGeneration
	m.log.loading = true
	m.log.err = ""
	m.log.filterErr = ""
	m.invalidate()
	repoRoot := m.gitLogRepositoryRoot()
	repoArg := m.gitLogRepositoryArg()
	if m.log.cancel != nil {
		m.log.cancel()
	}
	ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
	m.log.cancel = cancel
	go func() {
		defer cancel()
		query, queryErr := m.gitLogQuery(ctx, repoArg, spec)
		if queryErr != nil {
			m.post(func() {
				if generation != m.vcs.logGeneration || m.Root() != root ||
					m.gitLogRepositoryRoot() != repoRoot {
					return
				}
				m.log.loading = false
				m.log.cancel = nil
				m.log.filterErr = queryErr.Error()
				m.invalidate()
			})
			return
		}
		raw, err := m.invoke(ctx, "vcs_log", map[string]any{
			"repo":  repoArg,
			"query": query,
		})
		var commits []openidea.Commit
		if err == nil {
			err = json.Unmarshal(raw, &commits)
		}
		m.post(func() {
			if generation != m.vcs.logGeneration || m.Root() != root ||
				m.gitLogRepositoryRoot() != repoRoot {
				return
			}
			m.log.loading = false
			m.log.loaded = true
			m.log.cancel = nil
			if err != nil {
				m.log.err = vcsErrorText(err)
				m.log.commits = nil
				m.invalidate()
				if m.log.stale {
					m.log.stale = false
					m.RefreshLog()
				}
				return
			}
			m.log.commits = commits
			m.log.commitRepos = nil
			m.log.changeSelected = 0
			m.loadGitLogBookmarks()
			if len(commits) == 0 {
				m.log.selected = 0
				m.log.detailFor = ""
				m.log.details = openidea.LogDetails{}
				m.log.changes = nil
			} else if m.log.selected >= len(commits) {
				m.log.selected = 0
			}
			m.invalidate()
			if len(commits) > 0 {
				m.selectCommit(m.log.selected)
			}
			if m.log.stale {
				m.log.stale = false
				m.RefreshLog()
			}
		})
	}()
}

// refreshAggregateLog reads the same commit window from every repository,
// then merges the rows by commit time. Each row keeps its owner so details,
// diffs and history actions can route back to the repository that supplied it.
func (m *Model) refreshAggregateLog(projectRoot string) {
	if m.log.loading {
		m.log.stale = true
		return
	}
	repositories := make([]string, 0, len(m.vcs.repositories))
	for _, repo := range m.vcs.repositories {
		if root := vcsRepositoryKey(repo.Root); root != "" {
			repositories = append(repositories, root)
		}
	}
	if len(repositories) < 2 {
		m.log.aggregate = false
		m.RefreshLog()
		return
	}
	spec, specErr := m.gitLogQuerySpec("", "", logLimit)
	if specErr != nil {
		m.log.filterErr = specErr.Error()
		m.invalidate()
		return
	}
	m.log.stale = false
	m.vcs.logGeneration++
	generation := m.vcs.logGeneration
	m.log.loading = true
	m.log.err = ""
	m.log.filterErr = ""
	m.invalidate()

	type result struct {
		commits []openidea.Commit
		err     error
	}
	results := make([]result, len(repositories))
	if m.log.cancel != nil {
		m.log.cancel()
	}
	ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
	m.log.cancel = cancel
	go func() {
		defer cancel()
		var wg sync.WaitGroup
		wg.Add(len(repositories))
		for i, repository := range repositories {
			i, repository := i, repository
			go func() {
				defer wg.Done()
				repoArg := vcsRepositoryArgFor(repository)
				query, err := m.gitLogQuery(ctx, repoArg, spec)
				if err != nil {
					results[i] = result{err: err}
					return
				}
				raw, err := m.invoke(ctx, "vcs_log", map[string]any{
					"repo":  repoArg,
					"query": query,
				})
				var commits []openidea.Commit
				if err == nil {
					err = json.Unmarshal(raw, &commits)
				}
				results[i] = result{commits: commits, err: err}
			}()
		}
		wg.Wait()
		m.post(func() {
			if generation != m.vcs.logGeneration || m.Root() != projectRoot ||
				!m.log.aggregate || !sameVCSRepositoryList(repositories, m.vcs.repositories) {
				return
			}
			m.log.loading = false
			m.log.loaded = true
			m.log.cancel = nil
			for i, value := range results {
				if value.err != nil {
					m.log.err = "Could not read the log for " +
						m.vcsRepositoryLabelForRoot(repositories[i]) + ": " + value.err.Error()
					m.log.commits = nil
					m.log.commitRepos = nil
					m.invalidate()
					if m.log.stale {
						m.log.stale = false
						m.RefreshLog()
					}
					return
				}
			}
			type aggregateCommit struct {
				commit openidea.Commit
				repo   string
			}
			merged := make([]aggregateCommit, 0)
			for i, value := range results {
				for _, commit := range value.commits {
					merged = append(merged, aggregateCommit{
						commit: commit,
						repo:   repositories[i],
					})
				}
			}
			sort.SliceStable(merged, func(i, j int) bool {
				if merged[i].commit.TimeMs != merged[j].commit.TimeMs {
					return merged[i].commit.TimeMs > merged[j].commit.TimeMs
				}
				if merged[i].repo != merged[j].repo {
					return merged[i].repo < merged[j].repo
				}
				return merged[i].commit.ID < merged[j].commit.ID
			})
			m.log.commits = m.log.commits[:0]
			m.log.commitRepos = m.log.commitRepos[:0]
			for _, entry := range merged {
				m.log.commits = append(m.log.commits, entry.commit)
				m.log.commitRepos = append(m.log.commitRepos, entry.repo)
			}
			m.log.changeSelected = 0
			m.loadGitLogBookmarks()
			if len(m.log.commits) == 0 {
				m.log.selected = 0
				m.log.detailFor = ""
				m.log.detailRepo = ""
				m.log.details = openidea.LogDetails{}
				m.log.changes = nil
			} else if m.log.selected >= len(m.log.commits) {
				m.log.selected = 0
			}
			m.invalidate()
			if len(m.log.commits) > 0 {
				m.selectCommit(m.log.selected)
			}
			if m.log.stale {
				m.log.stale = false
				m.RefreshLog()
			}
		})
	}()
}

func sameVCSRepositoryList(roots []string, repositories []vcsRepository) bool {
	if len(roots) != len(repositories) {
		return false
	}
	for i := range roots {
		if vcsRepositoryKey(roots[i]) != vcsRepositoryKey(repositories[i].Root) {
			return false
		}
	}
	return true
}

func vcsRepositoryRoots(repositories []vcsRepository) []string {
	roots := make([]string, 0, len(repositories))
	for _, repository := range repositories {
		if root := vcsRepositoryKey(repository.Root); root != "" {
			roots = append(roots, root)
		}
	}
	return roots
}

// selectCommit loads the selected commit's details and changed files.
func (m *Model) selectCommit(i int) {
	root := m.Root()
	if root == "" || i < 0 || i >= len(m.log.commits) {
		return
	}
	m.log.selected = i
	id := m.log.commits[i].ID
	repoRoot := m.gitLogRepositoryRoot()
	if i < len(m.log.commitRepos) && m.log.commitRepos[i] != "" {
		repoRoot = vcsRepositoryKey(m.log.commitRepos[i])
	}
	repoArg := vcsRepositoryArgFor(repoRoot)
	m.log.detailFor = id
	m.log.detailRepo = repoRoot
	m.log.detailGeneration++
	generation := m.log.detailGeneration
	m.log.details = openidea.LogDetails{}
	m.log.changes = nil
	m.log.changeSelected = 0
	m.log.detailErr = ""
	m.log.detailing = true
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
		defer cancel()
		detailsRaw, detailsErr := m.invoke(ctx, "vcs_log_details", map[string]any{
			"repo": repoArg, "revision": id,
		})
		var details openidea.LogDetails
		if detailsErr == nil {
			detailsErr = json.Unmarshal(detailsRaw, &details)
		}
		changesRaw, changesErr := m.invoke(ctx, "vcs_commits_changes", map[string]any{
			"repo": repoArg, "commits": []string{id},
		})
		var changes []openidea.RevisionChange
		if changesErr == nil {
			changesErr = json.Unmarshal(changesRaw, &changes)
		}
		m.post(func() {
			if m.Root() != root || generation != m.log.detailGeneration ||
				m.log.detailFor != id || m.log.detailRepo != repoRoot {
				return
			}
			m.log.detailing = false
			if detailsErr != nil {
				m.log.detailErr = "Could not read the commit: " + detailsErr.Error()
			} else {
				m.log.details = details
			}
			if changesErr != nil && m.log.detailErr == "" {
				m.log.detailErr = "Could not read the commit's files: " + changesErr.Error()
			}
			m.log.changes = changes
			m.log.changeSelected = clampSymbolSel(m.log.changeSelected, len(changes))
			m.invalidate()
		})
	}()
}

func (m *Model) logCommitRepository(index int) string {
	if index >= 0 && index < len(m.log.commitRepos) {
		if root := vcsRepositoryKey(m.log.commitRepos[index]); root != "" {
			return root
		}
	}
	return m.gitLogRepositoryRoot()
}

func (m *Model) logCommitTarget(index int) map[string]any {
	if index < 0 || index >= len(m.log.commits) {
		return nil
	}
	return map[string]any{
		"id":   m.log.commits[index].ID,
		"repo": m.logCommitRepository(index),
	}
}

// SmokeLog reports the Git log through the backend: the offscreen check of the
// log panel, its details and the files of its first commit.
func (m *Model) SmokeLog(ctx context.Context) (map[string]any, error) {
	raw, err := m.mgr.Invoke(ctx, Window, "vcs_log", map[string]any{
		"repo": nil,
		"query": map[string]any{
			"branch": nil, "path": nil, "author": nil, "text": nil,
			"skip": 0, "limit": 10,
		},
	})
	if err != nil {
		return nil, err
	}
	var commits []openidea.Commit
	if err := json.Unmarshal(raw, &commits); err != nil {
		return nil, err
	}
	out := map[string]any{"count": len(commits)}
	if len(commits) == 0 {
		return out, fmt.Errorf("vcs_log returned no commits")
	}
	first := commits[0]
	out["first"] = first.ShortID
	out["summary"] = first.Summary
	detailsRaw, err := m.mgr.Invoke(ctx, Window, "vcs_log_details", map[string]any{
		"repo": nil, "revision": first.ID,
	})
	if err != nil {
		return out, err
	}
	var details openidea.LogDetails
	if err := json.Unmarshal(detailsRaw, &details); err != nil {
		return out, err
	}
	out["message"] = details.Message
	out["branches"] = len(details.ContainingBranches)
	changesRaw, err := m.mgr.Invoke(ctx, Window, "vcs_commits_changes", map[string]any{
		"repo": nil, "commits": []string{first.ID},
	})
	if err != nil {
		return out, err
	}
	var changes []openidea.RevisionChange
	if err := json.Unmarshal(changesRaw, &changes); err != nil {
		return out, err
	}
	out["changes"] = len(changes)
	names := make([]string, 0, len(changes))
	for _, c := range changes {
		names = append(names, c.Path)
	}
	out["changedFiles"] = names
	return out, nil
}

// logPanel draws OpenIDEA's single bottom Git window. The dock ID stays "log"
// so every existing Show Git Log action keeps its route, while the content tabs
// share the Local Changes, Stash, Log, Console and Worktrees native surfaces.
func (m *Model) logPanel(c *ui.Context, p *ui.Theme) {
	if !m.log.visible {
		return
	}
	oi := oiPaletteOf(p)
	ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
		m.gitToolWindowTabs(c, p, oi)
		ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
			switch m.gitToolWindowTab() {
			case vcsContentChanges:
				m.vcsChangesContent(c, p)
			case vcsContentStash:
				m.vcsStashContent(c, p, oi)
			case vcsContentConsole:
				m.gitConsoleContent(c, p, oi)
			case vcsContentWorktrees:
				m.vcsWorktreeContent(c, p, oi)
			default:
				m.gitLogPanel(c, p)
			}
		})
	})
}

func (m *Model) gitToolWindowTab() vcsContentTab {
	switch tab := m.currentVCSContent(); tab {
	case vcsContentChanges, vcsContentStash, vcsContentLog, vcsContentConsole, vcsContentWorktrees:
		return tab
	default:
		return vcsContentLog
	}
}

func (m *Model) gitToolWindowTabs(c *ui.Context, p *ui.Theme, oi oiColors) {
	type gitTab struct {
		tab   vcsContentTab
		label string
		count int
	}
	tabs := []gitTab{
		{tab: vcsContentChanges, label: "Local Changes", count: len(m.currentVCSFiles())},
		{tab: vcsContentStash, label: "Stash", count: m.vcsStashCount()},
		{tab: vcsContentLog, label: "Log"},
		{tab: vcsContentConsole, label: "Console"},
		{tab: vcsContentWorktrees, label: "Worktrees"},
	}
	active := m.gitToolWindowTab()
	ui.Row(c).Height(32).Shrink(0).Padding(0, 6).Gap(2).AlignItems(ui.Center).
		BorderWidth(1, 0, 1, 0).BorderColor(p.Border).Children(func() {
		for _, item := range tabs {
			item := item
			row := ui.Row(c.Key("git-window:"+item.label)).Height(26).Padding(0, 9).
				Radius(5).AlignItems(ui.Center).Cursor(ui.CursorPointer)
			switch {
			case item.tab == active:
				row.Background(oi.stripeSelected)
			case row.Hovered():
				row.Background(oi.hoverTint)
			}
			row.Children(func() {
				ui.Text(c, item.label).FontSize(12).
					TextColor(boolColor(item.tab == active, oi.textStrong, oi.textSubtle))
				if item.count > 0 {
					ui.Text(c, fmt.Sprint(item.count)).FontSize(10).
						TextColor(oi.textSubtle)
				}
			})
			if row.Clicked() {
				m.SelectVCSTab(item.tab)
			}
		}
		ui.Spacer(c)
		if m.vcs.loading {
			ui.Text(c, "Refreshing…").FontSize(11).TextColor(oi.textSubtle)
		}
	})
}

func (m *Model) gitConsoleContent(c *ui.Context, p *ui.Theme, oi oiColors) {
	ui.Row(c).Height(32).Shrink(0).Padding(0, 6).Gap(3).AlignItems(ui.Center).
		BorderWidth(1, 0, 1, 0).BorderColor(p.Border).Children(func() {
		ui.Button(c, "Clear All").Size(72, 24).Disabled(len(m.log.console) == 0).
			OnClick(func() {
				m.log.console = nil
				m.invalidate()
			})
	})
	ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Padding(8, 10).Gap(4).Children(func() {
			if len(m.log.console) == 0 {
				ui.Text(c, "Git commands run by the IDE appear here.").FontSize(12).
					TextColor(oi.textMuted)
				return
			}
			for i, line := range m.log.console {
				ui.Text(c, line).Key(fmt.Sprintf("git-console:%d", i)).FontSize(12).
					Font("monospace").TextColor(oi.textStrong)
			}
		})
	})
}

func (m *Model) gitLogPanel(c *ui.Context, p *ui.Theme) {
	l := &m.log
	col := ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Background(m.toolWindowBackground(p)).
		BorderWidth(1, 0, 0, 0).BorderColor(p.Border)
	col.Children(func() {
		ui.Row(c).Height(30).Padding(0, 8).Gap(8).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
			ui.Text(c, "Git Log").FontSize(12).Bold().TextColor(p.Text)
			ui.Text(c, "Commit History").FontSize(12).TextColor(p.TextMuted)
			ui.Spacer(c)
			if l.loading {
				ui.Text(c, "Reading…").FontSize(12).TextColor(p.TextMuted)
			} else if l.loaded {
				ui.Text(c, fmt.Sprintf("The last %d commits", len(l.commits))).
					FontSize(12).TextColor(p.TextMuted)
			}
			ui.Button(c, "Cherry-Pick").Disabled(!l.loaded || len(l.commits) == 0).
				OnClick(m.CherryPickSelectedCommit)
			ui.Button(c, "Revert").Disabled(!l.loaded || len(l.commits) == 0).
				OnClick(m.RevertSelectedCommit)
			ui.Button(c, "Reset…").Disabled(!l.loaded || len(l.commits) == 0).
				OnClick(m.OpenGitResetAtSelectedCommit)
			ui.Button(c, "Branches").OnClick(m.ToggleGitLogBranches)
			ui.Button(c, "Bookmarks").Disabled(len(l.bookmarks) == 0).
				Menu(func(menu *ui.Menu) { m.gitLogBookmarksMenu(menu) })
			label := "Filters"
			if m.gitLogFiltersActive() {
				label += " •"
			}
			ui.Button(c, label).OnClick(func() {
				m.log.filtersOpen = !m.log.filtersOpen
				if !m.log.filtersOpen {
					m.log.filterErr = ""
				}
				m.invalidate()
			})
			ui.Button(c, "Options…").Menu(func(menu *ui.Menu) { m.gitLogOptionsMenu(menu) })
			ui.Button(c, "Refresh").OnClick(func() { m.RefreshLog() })
			ui.Button(c, "Close").OnClick(func() { m.CloseLog() })
		})
		if l.filtersOpen {
			m.gitLogFilterBar(c, p)
		}
		ui.Row(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			if l.showBranches {
				m.logBranchesPanel(c, p)
			}
			ui.Column(c).Width(420).Children(func() {
				m.logColumnHeader(c, p)
				ui.Scroll(c).Grow(1).Children(func() {
					ui.Column(c).Padding(4, 0).Children(func() { m.logRows(c, p) })
				})
			})
			ui.Column(c).Grow(1).MinWidth(0).Padding(8, 10).Children(func() {
				m.logDetails(c, p)
			})
		})
	})
}

func (m *Model) logBranchesPanel(c *ui.Context, p *ui.Theme) {
	l := &m.log
	ui.Column(c).Width(220).BorderWidth(0, 1, 0, 0).BorderColor(p.Border).Children(func() {
		ui.Row(c).Height(26).Padding(0, 8).Gap(4).AlignItems(ui.Center).
			Background(p.Background).BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
			ui.Text(c, "Branches").FontSize(12).Bold().TextColor(p.Text)
			ui.Spacer(c)
			ui.Button(c, "+").Size(22, 22).OnClick(m.OpenNewGitBranchAtLogSelection)
		})
		if l.branchesLoading {
			ui.Text(c, "Reading branches…").Padding(8).FontSize(12).TextColor(p.TextMuted)
			return
		}
		if l.branchesErr != "" {
			ui.Text(c, l.branchesErr).Padding(8).FontSize(12).TextColor(p.Warning)
			return
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(2, 0).Children(func() {
				for i, branch := range l.branches {
					i, branch := i, branch
					row := ui.Row(c.Key("log-branch:"+branch.Name)).Height(26).Padding(0, 8).
						Gap(6).AlignItems(ui.Center)
					if i == l.branchSelected {
						row.Background(p.Selection)
					} else if row.Hovered() {
						row.Background(p.SurfaceHover)
					}
					row.Children(func() {
						favorite := l.favoriteBranches != nil && l.favoriteBranches[branch.Name]
						if favorite {
							ui.Text(c, "★").FontSize(12).TextColor(p.Text)
						}
						ui.Text(c, branch.Name).FontSize(12).TextColor(p.Text).SingleLine().Ellipsis("…")
						ui.Spacer(c)
						if branch.Current {
							ui.Text(c, "HEAD").FontSize(11).TextColor(p.TextMuted)
						} else if branch.Remote {
							ui.Text(c, "remote").FontSize(11).TextColor(p.TextMuted)
						}
					})
					row.ContextMenu(func(menu *ui.Menu) { m.gitLogBranchMenu(menu, i) })
					if row.Clicked() {
						l.branchSelected = i
						m.invalidate()
					}
					if row.DoubleClicked() {
						m.NavigateGitLogToBranch(i)
					}
				}
			})
		})
	})
}

func (m *Model) logColumnHeader(c *ui.Context, p *ui.Theme) {
	ui.Row(c).Height(26).Padding(0, 8).Gap(8).AlignItems(ui.Center).
		Background(p.Background).BorderWidth(0, 0, 1, 0).BorderColor(p.Border).
		Children(func() {
			ui.Text(c, "Subject").FontSize(13).TextColor(p.TextMuted).SingleLine()
			ui.Spacer(c)
			ui.Text(c, "Author").FontSize(13).TextColor(p.TextMuted).SingleLine().Width(130)
			ui.Text(c, "Date").FontSize(13).TextColor(p.TextMuted).SingleLine().Width(120)
			ui.Text(c, "Hash").FontSize(13).TextColor(p.TextMuted).SingleLine().Width(72)
		})
}

func (m *Model) logRows(c *ui.Context, p *ui.Theme) {
	l := &m.log
	if l.err != "" {
		ui.Row(c).Padding(8, 6).Children(func() {
			ui.Text(c, l.err).FontSize(12).TextColor(p.Warning)
		})
		return
	}
	if !l.loaded && l.loading {
		ui.Text(c, "Reading the commit history…").FontSize(12).TextColor(p.TextMuted)
		return
	}
	if len(l.commits) == 0 {
		ui.Text(c, "No commits").FontSize(12).TextColor(p.TextMuted)
		return
	}
	for i, cm := range l.commits {
		i, cm := i, cm
		if l.collapseLinear && i != l.selected && len(cm.Refs) == 0 {
			continue
		}
		r := ui.Row(c.Key(fmt.Sprintf("log:%d", i))).Height(26).Padding(0, 8).Gap(8).
			AlignItems(ui.Center)
		if i == l.selected {
			r.Background(p.SurfaceHover)
		} else if r.Hovered() {
			r.Background(p.SurfaceHover)
		}
		r.Children(func() {
			if m.commitIsBookmarked(&cm) {
				ui.Text(c, "★").FontSize(12).TextColor(p.Text)
			}
			issueText(c, oiPaletteOf(p), cm.Summary, m.prefs.VCS.IssueNavigation).
				Grow(1).MinWidth(0)
			ui.Spacer(c)
			if len(cm.Refs) > 0 {
				ui.Text(c, strings.Join(cm.Refs, ", ")).FontSize(13).TextColor(p.TextMuted).
					SingleLine().Ellipsis("…")
			}
			ui.Text(c, cm.Author).FontSize(13).TextColor(p.TextMuted).SingleLine().Width(130)
			ui.Text(c, logTime(m.prefs.dateFormatPrefs(), c.Region(), cm.TimeMs)).FontSize(13).TextColor(p.TextMuted).SingleLine().Width(120)
			ui.Text(c, cm.ShortID).FontSize(13).Font("monospace").TextColor(p.TextMuted).
				SingleLine().Width(72)
		})
		if r.Clicked() && i != l.selected {
			m.selectCommit(i)
		}
		r.ContextMenu(func(menu *ui.Menu) {
			m.gitLogCommitMenu(menu, m.logCommitTarget(i))
		})
	}
}

func (m *Model) logDetails(c *ui.Context, p *ui.Theme) {
	l := &m.log
	if l.selected < 0 || l.selected >= len(l.commits) {
		ui.Text(c, "Select a commit to see its details").FontSize(12).TextColor(p.TextMuted)
		return
	}
	cm := l.commits[l.selected]
	oi := oiPaletteOf(p)
	ui.Column(c).Gap(6).Children(func() {
		issueSubjectText(c, oi, cm.Summary, m.prefs.VCS.IssueNavigation)
		ui.Text(c, cm.ShortID+" · "+cm.Author).FontSize(12).TextColor(p.TextMuted)
		if l.detailing {
			ui.Text(c, "Reading the commit…").FontSize(12).TextColor(p.TextMuted)
		}
		if l.detailErr != "" {
			ui.Text(c, l.detailErr).FontSize(12).TextColor(p.Warning)
		}
		message := l.details.Message
		if message == "" {
			message = cm.Message
		}
		if message != "" {
			ui.Scroll(c).Height(90).Children(func() {
				ui.Column(c).Gap(1).Children(func() {
					for _, line := range strings.Split(message, "\n") {
						issueText(c, oi, line, m.prefs.VCS.IssueNavigation)
					}
				})
			})
		}
		if len(l.details.ContainingBranches) > 0 {
			ui.Text(c, "Branches: "+strings.Join(l.details.ContainingBranches, ", ")).
				FontSize(12).TextColor(p.TextMuted).SingleLine().Ellipsis("…")
		}
		if l.details.Signature != "" {
			ui.Text(c, "Signed by: "+l.details.Signature).FontSize(12).TextColor(p.TextMuted)
		}
		if len(l.changes) == 0 {
			return
		}
		ui.Text(c, count(len(l.changes), "file", "files")).FontSize(12).TextColor(p.TextMuted)
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Children(func() {
				if l.groupByDirectory {
					groups := make(map[string][]int)
					var dirs []string
					for i, change := range l.changes {
						dir := filepath.ToSlash(filepath.Dir(change.Path))
						if dir == "." {
							dir = "/"
						}
						if _, ok := groups[dir]; !ok {
							dirs = append(dirs, dir)
						}
						groups[dir] = append(groups[dir], i)
					}
					sort.Strings(dirs)
					for _, dir := range dirs {
						ui.Text(c, dir).Padding(2, 8).FontSize(12).TextColor(p.TextMuted).
							SingleLine().Ellipsis("…")
						for _, i := range groups[dir] {
							m.logChangeRow(c, p, oi, i, 14)
						}
					}
				} else {
					for i := range l.changes {
						m.logChangeRow(c, p, oi, i, 0)
					}
				}
			})
		})
	})
}

func (m *Model) logChangeRow(c *ui.Context, p *ui.Theme, oi oiColors, i int, indent float32) {
	l := &m.log
	ch := l.changes[i]
	r := ui.Row(c.Key(fmt.Sprintf("logfile:%d", i))).Height(24).PaddingStart(indent).Gap(4).
		AlignItems(ui.Center)
	if i == l.changeSelected {
		r.Background(p.Selection)
	} else if r.Hovered() {
		r.Background(p.SurfaceHover)
	}
	r.Children(func() {
		fileIconEl(c, oi, ch.Path, 16, oi.textMuted)
		ui.Text(c, filepath.Base(ch.Path)).FontSize(13).
			TextColor(logChangeColor(ch.Kind, p)).SingleLine().Ellipsis("…")
		ui.Spacer(c)
		ui.Text(c, strings.ToUpper(ch.Kind)).FontSize(11).TextColor(p.TextMuted)
	})
	r.ContextMenu(func(menu *ui.Menu) { m.gitLogChangeMenu(menu, i) })
	if r.Clicked() {
		l.changeSelected = i
		if l.diffPreview {
			m.ShowSelectedLogChangeDiff()
		} else {
			m.invalidate()
		}
	}
	if r.DoubleClicked() {
		m.ShowSelectedLogChangeDiff()
	}
}

// logChangeColor is the colour of a changed file in a commit, as the VCS
// change list colours its names.
func logChangeColor(kind string, p *ui.Theme) ui.Color {
	oi := oiPaletteOf(p)
	switch kind {
	case "added":
		return oi.vcsAdded
	case "deleted":
		return oi.vcsDeleted
	case "untracked":
		return oi.vcsUntracked
	case "conflicted":
		return oi.vcsConflict
	}
	return oi.vcsModified
}

// logTime is a commit's time in Git Log, shared with commit pickers and the
// Local History sheet through Settings › System Settings › Date Formats.
func logTime(prefs DateFormatPrefs, region dateRegion, ms float64) string {
	return formatAppTimestamp(prefs, region, ms, time.Now())
}
