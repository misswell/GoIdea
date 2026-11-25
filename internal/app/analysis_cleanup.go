package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

const analysisCleanupLimit = 5000
const analysisCleanupConfirmAbove = 100

type analysisFileEntry struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Kind     string `json:"kind"`
	Ignored  bool   `json:"ignored"`
	Excluded bool   `json:"excluded"`
	Hidden   bool   `json:"hidden"`
}

type analysisCleanupPending struct {
	files   []string
	options analysisCleanupOptions
}

type analysisCleanupState struct {
	visible         bool
	optionsLoaded   bool
	running         bool
	cancelled       bool
	scope           analysisScopeState
	fixAll          bool
	optimizeImports bool
	reformat        bool
	total           int
	processed       int
	current         string
	err             string
	status          string
	pending         *analysisCleanupPending
}

type analysisCleanupOptions struct {
	fixAll          bool
	optimizeImports bool
	reformat        bool
}

type analysisCleanupLanguage struct {
	ID              string            `json:"id"`
	Extensions      []string          `json:"extensions"`
	Filenames       []string          `json:"filenames"`
	LanguageServers []json.RawMessage `json:"languageServers"`
}

func (m *Model) ShowAnalysisCleanup() {
	state := &m.analysis.cleanup
	m.loadAnalysisCleanupOptions()
	state.scope.normalize(m.analysisCurrentFile(), m.Root())
	state.visible = true
	m.DockShow(analysisCleanupWindow)
}

func (m *Model) loadAnalysisCleanupOptions() {
	state := &m.analysis.cleanup
	if state.optionsLoaded {
		return
	}
	state.optionsLoaded = true
	options := m.prefs.Analysis.Cleanup.options()
	state.fixAll = options.fixAll
	state.optimizeImports = options.optimizeImports
	state.reformat = options.reformat
}

func (m *Model) saveAnalysisCleanupOptions() {
	state := &m.analysis.cleanup
	options := analysisCleanupOptions{
		fixAll:          state.fixAll,
		optimizeImports: state.optimizeImports,
		reformat:        state.reformat,
	}
	if m.prefs.Analysis.Cleanup.options() == options && m.prefs.Analysis.Cleanup.Configured {
		return
	}
	m.prefs.Analysis.setCleanupOptions(options)
	m.savePreferences()
}

func (m *Model) RunAnalysisCleanup() {
	state := &m.analysis.cleanup
	state.scope.normalize(m.analysisCurrentFile(), m.Root())
	if state.running || !analysisInvokeAvailable(m) {
		return
	}
	if !state.fixAll && !state.optimizeImports && !state.reformat {
		state.err = "Choose at least one cleanup step"
		m.invalidate()
		return
	}
	state.running = true
	state.cancelled = false
	state.total = 0
	state.processed = 0
	state.current = ""
	state.err = ""
	state.status = "Finding files…"
	state.pending = nil
	options := analysisCleanupOptions{
		fixAll: state.fixAll, optimizeImports: state.optimizeImports, reformat: state.reformat,
	}
	m.saveAnalysisCleanupOptions()
	generation := m.analysis.generation
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		files, err := m.analysisCleanupFiles(ctx)
		m.post(func() {
			if m.analysis.generation != generation {
				return
			}
			if err != nil {
				state.running = false
				state.err = "Code Cleanup: " + err.Error()
				m.invalidate()
				return
			}
			state.total = len(files)
			if state.cancelled {
				state.running = false
				state.status = "Code Cleanup stopped"
				m.invalidate()
				return
			}
			if len(files) == 0 {
				state.running = false
				state.status = "No files with a language server in this scope"
				m.invalidate()
				return
			}
			if len(files) > analysisCleanupConfirmAbove {
				state.running = false
				state.pending = &analysisCleanupPending{files: files, options: options}
				state.status = fmt.Sprintf("Confirm cleanup of %d files", len(files))
				m.invalidate()
				return
			}
			m.analysisCleanupStart(files, options, generation)
		})
	}()
}

func (m *Model) CancelAnalysisCleanup() {
	state := &m.analysis.cleanup
	if !state.running {
		return
	}
	state.cancelled = true
	state.pending = nil
	state.status = "Stopping after the current file…"
	m.invalidate()
}

func (m *Model) ConfirmAnalysisCleanup() {
	state := &m.analysis.cleanup
	pending := state.pending
	if pending == nil {
		return
	}
	state.pending = nil
	m.analysisCleanupStart(pending.files, pending.options, m.analysis.generation)
}

func (m *Model) CancelAnalysisCleanupConfirmation() {
	state := &m.analysis.cleanup
	state.pending = nil
	state.status = "Code Cleanup cancelled"
	m.invalidate()
}

func (m *Model) analysisCleanupStart(
	files []string,
	options analysisCleanupOptions,
	generation uint64,
) {
	state := &m.analysis.cleanup
	state.pending = nil
	state.running = true
	state.cancelled = false
	state.total = len(files)
	state.processed = 0
	state.current = ""
	state.err = ""
	state.status = "Cleaning files…"
	m.analysisCleanupNext(files, 0, options, generation)
}

func (m *Model) analysisCleanupFiles(ctx context.Context) ([]string, error) {
	state := &m.analysis.cleanup
	current := m.analysisCurrentFile()
	if state.scope.kind == analysisScopeFile {
		if current == "" {
			return nil, fmt.Errorf("no file is open")
		}
		return m.analysisCleanupFilterServerFiles(ctx, []string{current})
	}
	if state.scope.kind == analysisScopeChanged {
		raw, err := m.invoke(ctx, "vcs_status", map[string]any{})
		var statuses []openidea.FileStatus
		if err == nil {
			err = analysisJSON(raw, &statuses)
		}
		if err != nil {
			return nil, err
		}
		files := make([]string, 0, len(statuses))
		for _, status := range statuses {
			if status.Unstaged != nil && *status.Unstaged == "deleted" ||
				status.Staged != nil && *status.Staged == "deleted" {
				continue
			}
			files = append(files, status.Path)
		}
		return m.analysisCleanupFilterServerFiles(ctx, files)
	}
	root := m.Root()
	if state.scope.kind == analysisScopeDirectory {
		root = strings.TrimSpace(state.scope.directory)
	}
	if root == "" {
		return nil, fmt.Errorf("choose a directory")
	}
	extensions, filenames, err := m.analysisCleanupAcceptance(ctx)
	if err != nil {
		return nil, err
	}
	accept := func(path string) bool {
		return analysisCleanupAccepts(path, extensions, filenames)
	}
	return m.analysisWalkCleanupFiles(ctx, root, accept)
}

func (m *Model) analysisCleanupFilterServerFiles(ctx context.Context, paths []string) ([]string, error) {
	extensions, filenames, err := m.analysisCleanupAcceptance(ctx)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		path = filepath.Clean(path)
		if _, ok := seen[path]; ok {
			continue
		}
		if analysisCleanupAccepts(path, extensions, filenames) {
			seen[path] = struct{}{}
			out = append(out, path)
			if len(out) == analysisCleanupLimit {
				break
			}
		}
	}
	sort.Strings(out)
	return out, nil
}

func (m *Model) analysisCleanupAcceptance(
	ctx context.Context,
) (map[string]bool, map[string]bool, error) {
	raw, err := m.invoke(ctx, "languages", map[string]any{})
	var languages []analysisCleanupLanguage
	if err == nil {
		err = analysisJSON(raw, &languages)
	}
	if err != nil {
		return nil, nil, err
	}
	extensions := map[string]bool{}
	filenames := map[string]bool{}
	for _, language := range languages {
		if len(language.LanguageServers) == 0 {
			continue
		}
		for _, ext := range language.Extensions {
			ext = strings.ToLower(strings.TrimPrefix(ext, "."))
			if ext != "" {
				extensions[ext] = true
			}
		}
		for _, name := range language.Filenames {
			filenames[name] = true
		}
	}
	return extensions, filenames, nil
}

func analysisCleanupAccepts(path string, extensions, filenames map[string]bool) bool {
	name := filepath.Base(path)
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	return extensions[ext] || filenames[name]
}

func (m *Model) analysisWalkCleanupFiles(
	ctx context.Context,
	root string,
	accept func(string) bool,
) ([]string, error) {
	queue := []string{filepath.Clean(root)}
	seen := map[string]struct{}{}
	out := make([]string, 0)
	for len(queue) > 0 && len(out) < analysisCleanupLimit {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		dir := queue[0]
		queue = queue[1:]
		raw, err := m.invoke(ctx, "fs_read_dir", map[string]any{"path": dir})
		if err != nil {
			continue
		}
		var entries []analysisFileEntry
		if analysisJSON(raw, &entries) != nil {
			continue
		}
		for _, entry := range entries {
			if entry.Ignored || entry.Excluded || entry.Hidden ||
				strings.EqualFold(entry.Kind, "symlink") {
				continue
			}
			path := entry.Path
			if path == "" {
				path = filepath.Join(dir, entry.Name)
			}
			path = filepath.Clean(path)
			if strings.EqualFold(entry.Kind, "dir") ||
				strings.EqualFold(entry.Kind, "directory") {
				if _, ok := seen[path]; !ok {
					seen[path] = struct{}{}
					queue = append(queue, path)
				}
				continue
			}
			if accept(path) {
				out = append(out, path)
				if len(out) >= analysisCleanupLimit {
					break
				}
			}
		}
	}
	sort.Strings(out)
	return out, nil
}

func (m *Model) analysisCleanupNext(
	files []string,
	index int,
	options analysisCleanupOptions,
	generation uint64,
) {
	state := &m.analysis.cleanup
	if m.analysis.generation != generation {
		return
	}
	if state.cancelled || index >= len(files) {
		state.running = false
		state.current = ""
		if state.cancelled {
			state.status = fmt.Sprintf("Code Cleanup stopped after %d of %d files", state.processed, len(files))
		} else if state.err == "" {
			state.status = fmt.Sprintf("Code Cleanup processed %d files", state.processed)
		}
		m.invalidate()
		return
	}
	path := files[index]
	wasOpen := m.tabForPath(path) >= 0
	state.current = path
	state.status = fmt.Sprintf("Cleaning %s (%d/%d)", filepath.Base(path), index+1, len(files))
	m.invalidate()
	m.openFileThen(path, func(tab *Tab) {
		go func() {
			ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
			defer cancel()
			err := m.analysisCleanupTab(ctx, tab, options)
			m.post(func() {
				if m.analysis.generation != generation {
					return
				}
				state.processed++
				if err != nil && state.err == "" {
					state.err = fmt.Sprintf("Cleanup %s failed: %v", filepath.Base(path), err)
				}
				next := func() { m.analysisCleanupNext(files, index+1, options, generation) }
				if wasOpen || !m.tabOpen(tab) {
					next()
					return
				}
				m.closeTabThen(tab, next)
			})
		}()
	})
}

func (m *Model) analysisCleanupTab(ctx context.Context, tab *Tab, options analysisCleanupOptions) error {
	if tab == nil || tab.doc == nil {
		return fmt.Errorf("file did not open")
	}
	if options.fixAll {
		if err := m.applyAnalysisSourceAction(ctx, tab, "source.fixAll"); err != nil {
			return err
		}
	}
	if options.optimizeImports {
		if err := m.applyAnalysisSourceAction(ctx, tab, "source.organizeImports"); err != nil {
			return err
		}
	}
	if options.reformat {
		version := tab.doc.Buffer().Version()
		_, stale, err := m.applyLanguageFormat(
			ctx, tab, tab.doc.ID(), version, false, m.formatGuardForDocument(),
		)
		if err != nil {
			return err
		}
		if stale {
			return fmt.Errorf("the file changed while it was being formatted")
		}
	}
	if err := tab.doc.Sync(ctx); err != nil {
		return err
	}
	return tab.doc.Save(ctx)
}

func (m *Model) applyAnalysisSourceAction(ctx context.Context, tab *Tab, kind string) error {
	if err := tab.doc.Sync(ctx); err != nil {
		return err
	}
	request := m.nextRequest()
	raw, err := m.invoke(ctx, "lsp_code_action_query", map[string]any{
		"request": request,
		"doc":     tab.doc.ID(),
		"range": map[string]any{
			"start": lspPos(openidea.OutlinePos{Line: 0, Col: 0}),
			"end":   lspPos(tab.doc.UTF16Pos(len(tab.doc.Text()))),
		},
		"only": []string{kind},
	})
	if err != nil {
		return err
	}
	var query openidea.CodeActionQuery
	if err := analysisJSON(raw, &query); err != nil {
		return err
	}
	action := chooseSourceAction(parseIntentActions(query.Actions), kind)
	if len(action) == 0 {
		return nil
	}
	version := tab.doc.Buffer().Version()
	if _, err := m.invoke(ctx, "lsp_apply_code_action", map[string]any{
		"doc": tab.doc.ID(), "action": action,
	}); err != nil {
		return err
	}
	// The backend applies the action to its document and streams the patch to
	// the native buffer. Let that patch land before asking its next server or
	// syncing the buffer back to the backend.
	deadline := time.Now().Add(2 * time.Second)
	for tab.doc.Buffer().Version() == version && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	return nil
}

type analysisCleanupFileArg struct {
	Path            string `json:"path"`
	FixAll          *bool  `json:"fixAll"`
	OptimizeImports *bool  `json:"optimizeImports"`
	Reformat        *bool  `json:"reformat"`
}

func decodeAnalysisCleanupFileArg(arg any) (string, analysisCleanupOptions, error) {
	options := analysisCleanupOptions{fixAll: true, optimizeImports: true, reformat: true}
	if arg == nil {
		return "", options, fmt.Errorf("choose a file")
	}
	raw, err := json.Marshal(arg)
	if err != nil {
		return "", options, err
	}
	var decoded analysisCleanupFileArg
	if err := analysisJSON(raw, &decoded); err != nil {
		return "", options, fmt.Errorf("invalid cleanup arguments: %w", err)
	}
	path := strings.TrimSpace(decoded.Path)
	if path == "" {
		return "", options, fmt.Errorf("choose a file")
	}
	if decoded.FixAll != nil {
		options.fixAll = *decoded.FixAll
	}
	if decoded.OptimizeImports != nil {
		options.optimizeImports = *decoded.OptimizeImports
	}
	if decoded.Reformat != nil {
		options.reformat = *decoded.Reformat
	}
	if !options.fixAll && !options.optimizeImports && !options.reformat {
		return "", options, fmt.Errorf("choose at least one cleanup step")
	}
	return path, options, nil
}

// CleanupAnalysisFile is the hidden code.cleanupFile action. It opens the
// requested file, cleans that document, and closes the tab only when it was
// closed before the action.
func (m *Model) CleanupAnalysisFile(arg any) {
	path, options, err := decodeAnalysisCleanupFileArg(arg)
	if err != nil {
		m.status = "Code Cleanup: " + err.Error()
		m.invalidate()
		return
	}
	if !analysisInvokeAvailable(m) {
		m.status = "Code Cleanup needs the GoIDEA backend"
		m.invalidate()
		return
	}
	wasOpen := m.tabForPath(path) >= 0
	generation := m.analysis.generation
	m.openFileThen(path, func(tab *Tab) {
		go func() {
			ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
			defer cancel()
			err := m.analysisCleanupTab(ctx, tab, options)
			m.post(func() {
				if m.analysis.generation != generation {
					return
				}
				if err != nil {
					m.status = fmt.Sprintf("Cleanup %s failed: %v", filepath.Base(path), err)
				} else {
					m.status = "Cleaned " + filepath.Base(path)
				}
				next := func() { m.invalidate() }
				if wasOpen || !m.tabOpen(tab) {
					next()
					return
				}
				m.closeTabThen(tab, next)
			})
		}()
	})
}

func (m *Model) analysisCleanupPanel(c *ui.Context, p *ui.Theme) {
	state := &m.analysis.cleanup
	m.loadAnalysisCleanupOptions()
	state.scope.normalize(m.analysisCurrentFile(), m.Root())
	ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Background(m.toolWindowBackground(p)).
		BorderWidth(1, 0, 0, 0).BorderColor(p.Border).Children(func() {
		ui.Row(c).Height(34).Padding(0, 8).Gap(6).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
			analysisButton(c, "Run Cleanup", !state.running, m.RunAnalysisCleanup)
			analysisButton(c, "Stop", state.running, m.CancelAnalysisCleanup)
			ui.Spacer(c)
			if state.running {
				ui.Text(c, fmt.Sprintf("%d/%d", state.processed, state.total)).FontSize(11).TextColor(p.TextMuted)
			}
		})
		ui.Column(c).Padding(10, 12).Gap(9).Children(func() {
			analysisScopeBar(c, p, &state.scope, false)
			ui.Checkbox(c, &state.fixAll, "Apply the language servers' automatic fixes")
			ui.Checkbox(c, &state.optimizeImports, "Optimize imports")
			ui.Checkbox(c, &state.reformat, "Reformat code")
			ui.Text(c, "Selected files are opened, cleaned by their language servers, saved, then closed when Cleanup opened them.").
				FontSize(11).TextColor(p.TextMuted).SingleLine().Ellipsis("…")
		})
		if status := analysisCleanupStatus(state); status != "" {
			analysisStatusRow(c, p, status, state.err != "")
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(10, 12).Children(func() {
				if state.current != "" {
					ui.Text(c, state.current).Font("monospace").FontSize(11).TextColor(p.TextMuted).
						SingleLine().Ellipsis("…")
				} else {
					ui.Text(c, "Code Cleanup applies the language server's fix-all and organize-imports actions, then reformats and saves each file in scope.").
						FontSize(12).TextColor(p.TextMuted)
				}
			})
		})
		m.analysisCleanupConfirmDialog(c, p)
	})
}

func (m *Model) analysisCleanupConfirmDialog(c *ui.Context, p *ui.Theme) {
	state := &m.analysis.cleanup
	pending := state.pending
	if pending == nil {
		return
	}
	open := true
	dlg := nativeModal(c.Key("analysis:cleanup:confirm"), &open, func() {
		ui.Column(c).Width(430).Padding(18).Gap(10).
			Background(p.Surface).Border(1, p.Border).Radius(6).Children(func() {
			ui.Text(c, "Code Cleanup").FontSize(14).Bold().TextColor(p.Text).SingleLine()
			ui.Text(c, fmt.Sprintf("Clean up %d files?", len(pending.files))).
				FontSize(12).TextColor(p.Text)
			ui.Text(c, "Each file will be opened, cleaned by its language servers, and saved.").
				FontSize(11).TextColor(p.TextMuted)
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				ui.Button(c, "Cancel").OnClick(func() {
					open = false
					m.CancelAnalysisCleanupConfirmation()
				})
				ui.Button(c, "Clean Up").OnClick(func() {
					open = false
					m.ConfirmAnalysisCleanup()
				})
			})
		})
	})
	if dlg.Dismissed() || !open {
		if state.pending == pending {
			m.CancelAnalysisCleanupConfirmation()
		}
	}
}

func analysisCleanupStatus(state *analysisCleanupState) string {
	if state.err != "" {
		return state.err
	}
	return state.status
}
