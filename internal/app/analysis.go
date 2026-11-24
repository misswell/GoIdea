package app

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

const (
	analysisInspectionWindow = "inspection"
	analysisDuplicatesWindow = "duplicates"
	analysisStructuralWindow = "analysis.structural"
	analysisCleanupWindow    = "analysis.cleanup"

	analysisScopeProject   = "project"
	analysisScopeDirectory = "directory"
	analysisScopeFile      = "file"
	analysisScopeChanged   = "changed"
	analysisScopeCustom    = "custom"
)

var analysisScopeLabels = []struct {
	ID    string
	Label string
}{
	{analysisScopeProject, "Whole project"},
	{analysisScopeDirectory, "Directory"},
	{analysisScopeFile, "Current file"},
	{analysisScopeChanged, "Uncommitted files"},
	{analysisScopeCustom, "Custom scope pattern"},
}

// analysisState is the four native surfaces that replace OpenIDEA's Analysis
// React feature. Results live with the model so docked windows are views over
// the same data rather than independent copies.
type analysisState struct {
	generation uint64
	inspection analysisInspectionState
	duplicates analysisDuplicatesState
	structural analysisStructuralState
	cleanup    analysisCleanupState
}

// resetAnalysisState invalidates every in-flight analysis operation and drops
// all project-bound results. Generation checking makes late RPC callbacks
// harmless even when the backend cannot acknowledge cancellation immediately.
func (m *Model) resetAnalysisState() {
	if m == nil {
		return
	}
	for _, request := range []uint32{
		m.analysis.inspection.request,
		m.analysis.duplicates.request,
		m.analysis.structural.request,
	} {
		analysisCancelRequest(m, request)
	}
	generation := m.analysis.generation + 1
	m.analysis = analysisState{generation: generation}
}

func (m *Model) analysisCurrentFile() string {
	t := m.tab()
	if t == nil || t.doc == nil || t.kind != "" {
		return ""
	}
	path := t.doc.Path()
	if !filepath.IsAbs(path) || strings.Contains(path, "://") {
		return ""
	}
	return path
}

func (m *Model) analysisScopePayload(kind, directory, pattern string) (map[string]any, error) {
	switch kind {
	case analysisScopeProject:
		return map[string]any{"kind": "project"}, nil
	case analysisScopeDirectory:
		directory = strings.TrimSpace(directory)
		if directory == "" {
			return nil, fmt.Errorf("choose a directory")
		}
		return map[string]any{"kind": "directory", "path": directory}, nil
	case analysisScopeFile:
		path := m.analysisCurrentFile()
		if path == "" {
			return nil, fmt.Errorf("no file is open")
		}
		return map[string]any{"kind": "files", "paths": []string{path}}, nil
	case analysisScopeChanged:
		paths := make([]string, 0, len(m.vcs.files))
		for _, file := range m.vcs.files {
			if file.Unstaged != nil && *file.Unstaged == "deleted" ||
				file.Staged != nil && *file.Staged == "deleted" {
				continue
			}
			paths = append(paths, file.Path)
		}
		if len(paths) == 0 {
			return nil, fmt.Errorf("there are no uncommitted files")
		}
		return map[string]any{"kind": "files", "paths": paths}, nil
	case analysisScopeCustom:
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			return nil, fmt.Errorf("choose or enter a custom scope pattern")
		}
		return map[string]any{"kind": "pattern", "pattern": pattern}, nil
	default:
		return nil, fmt.Errorf("unknown scope %q", kind)
	}
}

func (m *Model) analysisScopeLabel(kind, directory, pattern string) string {
	switch kind {
	case analysisScopeProject:
		return "Project"
	case analysisScopeDirectory:
		if directory = strings.TrimSpace(directory); directory != "" {
			if rel, err := filepath.Rel(m.Root(), directory); err == nil && !strings.HasPrefix(rel, "..") {
				directory = rel
			}
			return "Directory " + directory
		}
		return "Directory"
	case analysisScopeFile:
		if path := m.analysisCurrentFile(); path != "" {
			return "Current file " + filepath.Base(path)
		}
		return "Current file"
	case analysisScopeChanged:
		return fmt.Sprintf("%d uncommitted files", len(m.vcs.files))
	case analysisScopeCustom:
		return "Scope " + strings.TrimSpace(pattern)
	default:
		return kind
	}
}

func analysisCancelRequest(m *Model, request uint32) {
	if request == 0 || m == nil || m.ctx == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
		defer cancel()
		_, _ = m.invoke(ctx, "cancel_request", map[string]any{"id": request})
	}()
}

func analysisStatusRow(c *ui.Context, p *ui.Theme, text string, danger bool) {
	color := p.TextMuted
	if danger {
		color = p.Danger
	}
	ui.Row(c).MinHeight(26).Padding(2, 10).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
		ui.Text(c, text).FontSize(12).TextColor(color).SingleLine().Ellipsis("…")
	})
}

func analysisButton(c *ui.Context, label string, enabled bool, run func()) ui.Element {
	b := ui.Button(c, label)
	if !enabled {
		b.Disabled(true)
	}
	if enabled {
		b.OnClick(run)
	}
	return b
}

func analysisScopeBar(
	c *ui.Context,
	p *ui.Theme,
	state *analysisScopeState,
	includeCustom bool,
) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "Scope").FontSize(11).TextColor(p.TextMuted).Width(52)
		labels := make([]string, 0, len(analysisScopeLabels))
		current := "Whole project"
		for _, choice := range analysisScopeLabels {
			if choice.ID == analysisScopeCustom && !includeCustom {
				continue
			}
			labels = append(labels, choice.Label)
			if state.kind == choice.ID {
				current = choice.Label
			}
		}
		chosen := current
		ui.Select(c, &chosen, labels).Width(190)
		if chosen != current {
			for _, choice := range analysisScopeLabels {
				if choice.Label == chosen {
					state.kind = choice.ID
					break
				}
			}
		}
		if state.kind == analysisScopeDirectory {
			ui.TextInput(c, &state.directory).Grow(1).Placeholder("Directory path")
		} else if state.kind == analysisScopeCustom {
			ui.TextInput(c, &state.pattern).Grow(1).Placeholder("glob pattern")
		}
	})
}

type analysisScopeState struct {
	kind      string
	directory string
	pattern   string
}

func (s *analysisScopeState) normalize(currentFile, root string) {
	if s.kind == "" {
		s.kind = analysisScopeProject
	}
	if s.directory == "" && currentFile != "" {
		s.directory = filepath.Dir(currentFile)
	}
	if s.directory == "" {
		s.directory = root
	}
}

func analysisIncludedDefault(width, index int) bool {
	return index >= 0 && index < width
}

func analysisMatchKey(path string, index int) string {
	return fmt.Sprintf("%s\x00%d", path, index)
}

func analysisRelative(root, path string) string {
	if rel, err := filepath.Rel(root, path); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return path
}

func analysisSortedPaths(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func analysisDecode[T any](raw json.RawMessage) (T, error) {
	var value T
	err := json.Unmarshal(raw, &value)
	return value, err
}

func analysisCSV(rows [][]string) (string, error) {
	var b strings.Builder
	w := csv.NewWriter(&b)
	if err := w.WriteAll(rows); err != nil {
		return "", err
	}
	w.Flush()
	return b.String(), w.Error()
}

func analysisOptionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func analysisOpenResult(m *Model, path string, line, col, endLine, endCol int) {
	if endLine >= 0 && endCol >= 0 {
		m.revealRange(path, line, col, endLine, endCol)
		return
	}
	m.revealAt(path, line, col)
}

func init() {
	registerToolWindow(&toolWindowSpec{
		id: analysisInspectionWindow, title: "Inspection Results", icon: "toolwindows/problems",
		anchor: dockBottom, order: 36,
		open:  func(m *Model) { m.analysis.inspection.visible = true },
		close: func(m *Model) { m.analysis.inspection.visible = false },
		shown: func(m *Model) bool { return m.analysis.inspection.visible },
		render: func(m *Model, c *ui.Context, p *ui.Theme, _ oiColors) {
			m.analysisInspectionPanel(c, p)
		},
	})
	registerToolWindow(&toolWindowSpec{
		id: analysisDuplicatesWindow, title: "Duplicates", icon: "toolwindows/find",
		anchor: dockBottom, order: 37,
		open:  func(m *Model) { m.analysis.duplicates.visible = true },
		close: func(m *Model) { m.analysis.duplicates.visible = false },
		shown: func(m *Model) bool { return m.analysis.duplicates.visible },
		render: func(m *Model, c *ui.Context, p *ui.Theme, _ oiColors) {
			m.analysisDuplicatesPanel(c, p)
		},
	})
	registerToolWindow(&toolWindowSpec{
		id: analysisStructuralWindow, title: "Structural Search", icon: "toolwindows/find",
		anchor: dockBottom, order: 38,
		open:  func(m *Model) { m.analysis.structural.visible = true },
		close: func(m *Model) { m.analysis.structural.visible = false },
		shown: func(m *Model) bool { return m.analysis.structural.visible },
		render: func(m *Model, c *ui.Context, p *ui.Theme, _ oiColors) {
			m.analysisStructuralPanel(c, p)
		},
	})
	registerToolWindow(&toolWindowSpec{
		id: analysisCleanupWindow, title: "Code Cleanup", icon: "toolwindows/build",
		anchor: dockBottom, order: 39,
		open:  func(m *Model) { m.analysis.cleanup.visible = true },
		close: func(m *Model) { m.analysis.cleanup.visible = false },
		shown: func(m *Model) bool { return m.analysis.cleanup.visible },
		render: func(m *Model, c *ui.Context, p *ui.Theme, _ oiColors) {
			m.analysisCleanupPanel(c, p)
		},
	})

	for _, action := range []*Action{
		{
			ID: "analyze.inspectCode", Title: "Inspect Code…", Category: "Code",
			Keywords: "analysis inspection diagnostics problems scope",
			When:     func(m *Model) bool { return m.Root() != "" },
			Run:      func(m *Model) { m.ShowAnalysisInspection(analysisScopeProject) },
		},
		{
			ID: "analyze.inspectFile", Title: "Inspect Current File", Category: "Code",
			Keywords: "analysis inspection current file diagnostics",
			When: func(m *Model) bool {
				return m.Root() != "" && m.analysisCurrentFile() != ""
			},
			Run: func(m *Model) { m.ShowAnalysisInspection(analysisScopeFile) },
		},
		{
			ID: "analyze.locateDuplicates", Title: "Locate Duplicates…", Category: "Code",
			Keywords: "analysis duplicates clones copy paste cpd tokens",
			When:     func(m *Model) bool { return m.Root() != "" },
			Run:      func(m *Model) { m.ShowAnalysisDuplicates() },
		},
		{
			ID: "code.cleanup", Title: "Code Cleanup…", Category: "Code",
			Keywords: "analysis cleanup fix all organize imports reformat",
			When:     func(m *Model) bool { return m.Root() != "" },
			Run:      func(m *Model) { m.ShowAnalysisCleanup() },
		},
		{
			ID: "code.cleanupFile", Title: "Code Cleanup in File", Category: "Code",
			Keywords: "analysis cleanup file fix all organize imports reformat",
			When:     func(m *Model) bool { return m.Root() != "" },
			RunWithArg: func(m *Model, arg any) {
				m.CleanupAnalysisFile(arg)
			},
		},
		{
			ID: "find.structural", Title: "Search Structurally…", Category: "Find",
			Keywords: "analysis structural search pattern ast syntax",
			When:     func(m *Model) bool { return m.Root() != "" },
			Run:      func(m *Model) { m.ShowAnalysisStructural(false) },
		},
		{
			ID: "find.replaceStructural", Title: "Replace Structurally…", Category: "Find",
			Keywords: "analysis structural replace pattern ast syntax",
			When:     func(m *Model) bool { return m.Root() != "" },
			Run:      func(m *Model) { m.ShowAnalysisStructural(true) },
		},
	} {
		_ = registerAction(action)
	}
}

// The generic helpers live here so each window file stays independent of the
// test hook implementation in java_run.go.
func analysisJSON(raw json.RawMessage, out any) error {
	return json.Unmarshal(raw, out)
}

func analysisInvokeAvailable(m *Model) bool {
	return m != nil && m.ctx != nil && (m.mgr != nil || m.invokeHook != nil)
}

func analysisSyncOpenDocuments(ctx context.Context, tabs []*Tab) error {
	for _, tab := range tabs {
		if tab == nil || tab.doc == nil || tab.kind != "" {
			continue
		}
		if err := tab.doc.Sync(ctx); err != nil {
			return err
		}
	}
	return nil
}

func analysisRange(match openidea.OutlineRange) (int, int, int, int) {
	return int(match.Start.Line), int(match.Start.Col), int(match.End.Line), int(match.End.Col)
}
