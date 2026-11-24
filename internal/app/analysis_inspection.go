package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

type analysisInspectSummary struct {
	Files     uint32 `json:"files"`
	Problems  uint32 `json:"problems"`
	Skipped   uint32 `json:"skipped"`
	Failed    uint32 `json:"failed"`
	Cancelled bool   `json:"cancelled"`
	ElapsedMs uint32 `json:"elapsedMs"`
}

type analysisInspectedFile struct {
	Path        string                `json:"path"`
	RelPath     string                `json:"relPath"`
	Diagnostics []openidea.Diagnostic `json:"diagnostics"`
}

type analysisInspectEvent struct {
	Type    string                 `json:"type"`
	Total   uint32                 `json:"total"`
	File    analysisInspectedFile  `json:"file"`
	Done    uint32                 `json:"done"`
	Summary analysisInspectSummary `json:"summary"`
}

type analysisInspectionState struct {
	visible         bool
	running         bool
	profileWait     bool
	request         uint32
	done            uint32
	total           uint32
	scope           analysisScopeState
	scopeLabel      string
	profile         string
	profileLabel    string
	rawFiles        map[string]analysisInspectedFile
	files           map[string]analysisInspectedFile
	summary         *analysisInspectSummary
	err             string
	filter          string
	severity        string
	groupBySeverity bool
	autoscroll      bool
	optionsLoaded   bool
	excluded        map[string]bool
	selected        string
	collapsed       map[string]bool
	step            int
}

type analysisInspectionProblem struct {
	File analysisInspectedFile
	Diag openidea.Diagnostic
	Kind string
}

type analysisInspectionFileGroup struct {
	Path     string
	RelPath  string
	Problems []analysisInspectionProblem
}

type analysisInspectionKindGroup struct {
	Kind     string
	Severity string
	Count    int
	Files    []analysisInspectionFileGroup
}

type analysisInspectionSeverityGroup struct {
	Severity string
	Count    int
	Kinds    []analysisInspectionKindGroup
}

type analysisInspectionNode struct {
	Kind     string
	Key      string
	Label    string
	Severity string
	Count    int
	Path     string
	Problem  *analysisInspectionProblem
	Children []analysisInspectionNode
}

// ShowAnalysisInspection opens the native Inspection Results window and runs
// the backend batch inspection with the requested initial scope.
func (m *Model) ShowAnalysisInspection(scope string) {
	state := &m.analysis.inspection
	state.scope.normalize(m.analysisCurrentFile(), m.Root())
	if scope != "" {
		state.scope.kind = scope
	}
	m.DockShow(analysisInspectionWindow)
	m.loadAnalysisInspectionOptions()
	if !m.inspectionsSettings.loaded && analysisInvokeAvailable(m) {
		state.profileWait = true
		state.err = ""
		m.loadInspectionsSettings()
		m.waitForAnalysisInspectionProfile()
		m.invalidate()
		return
	}
	m.RunAnalysisInspection()
}

func (m *Model) loadAnalysisInspectionOptions() {
	state := &m.analysis.inspection
	if state.optionsLoaded {
		return
	}
	state.optionsLoaded = true
	state.groupBySeverity = m.prefs.Analysis.Inspection.GroupBySeverity
	state.autoscroll = m.prefs.Analysis.Inspection.Autoscroll
}

func (m *Model) saveAnalysisInspectionOptions() {
	state := &m.analysis.inspection
	next := InspectionViewPrefs{
		GroupBySeverity: state.groupBySeverity,
		Autoscroll:      state.autoscroll,
	}
	if m.prefs.Analysis.Inspection == next {
		return
	}
	m.prefs.Analysis.Inspection = next
	m.savePreferences()
}

func (m *Model) setAnalysisInspectionProfile(profile string) {
	state := &m.analysis.inspection
	if profile == "" {
		profile = m.activeInspectionProfile()
	}
	combined := m.combinedInspectionDraft()
	if !inspectionProfileExists(combined, profile, m.Root() != "") {
		profile = "default"
	}
	state.profile = profile
	state.profileLabel = inspectionProfileLabel(combined, profile)
	state.selected = ""
	state.step = -1
	m.syncAnalysisInspectionResults()
	m.invalidate()
}

func (m *Model) analysisInspectionOptionsMenu(menu *ui.Menu) {
	state := &m.analysis.inspection
	if menu.Item("Group by Severity").Checked(state.groupBySeverity).Chosen() {
		state.groupBySeverity = !state.groupBySeverity
		m.saveAnalysisInspectionOptions()
		m.invalidate()
	}
	if menu.Item("Autoscroll to Source").Checked(state.autoscroll).Chosen() {
		state.autoscroll = !state.autoscroll
		m.saveAnalysisInspectionOptions()
		m.invalidate()
	}
}

func (m *Model) waitForAnalysisInspectionProfile() {
	generation := m.analysis.generation
	root := m.Root()
	ctx := m.ctx
	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			var loading, loaded bool
			var loadErr string
			var stale bool
			m.post(func() {
				loading = m.inspectionsSettings.loading
				loaded = m.inspectionsSettings.loaded
				loadErr = m.inspectionsSettings.err
				stale = m.analysis.generation != generation || m.Root() != root
			})
			if stale {
				return
			}
			if !loading {
				m.post(func() {
					state := &m.analysis.inspection
					if m.analysis.generation != generation || m.Root() != root {
						return
					}
					state.profileWait = false
					if !loaded {
						if loadErr == "" {
							loadErr = "The GoIDEA backend is not connected"
						}
						state.err = "Inspect Code: " + loadErr
						m.invalidate()
						return
					}
					m.RunAnalysisInspection()
				})
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

// RunAnalysisInspection starts a backend inspection, replacing the previous
// stream and request.
func (m *Model) RunAnalysisInspection() {
	state := &m.analysis.inspection
	m.loadAnalysisInspectionOptions()
	state.scope.normalize(m.analysisCurrentFile(), m.Root())
	scope, err := m.analysisScopePayload(state.scope.kind, state.scope.directory, state.scope.pattern)
	if err != nil {
		state.err = "Inspect Code: " + err.Error()
		m.invalidate()
		return
	}
	if !analysisInvokeAvailable(m) {
		state.err = "Inspect Code needs the GoIDEA backend"
		m.invalidate()
		return
	}
	if state.request != 0 {
		analysisCancelRequest(m, state.request)
	}
	if state.profile == "" || !inspectionProfileExists(
		m.combinedInspectionDraft(), state.profile, m.Root() != "",
	) {
		state.profile = m.activeInspectionProfile()
	}
	state.profileLabel = inspectionProfileLabel(m.combinedInspectionDraft(), state.profile)
	state.profileWait = false
	request := m.nextRequest()
	generation := m.analysis.generation
	state.request = request
	state.running = true
	state.done = 0
	state.total = 0
	state.rawFiles = map[string]analysisInspectedFile{}
	state.files = map[string]analysisInspectedFile{}
	state.summary = nil
	state.err = ""
	state.excluded = map[string]bool{}
	state.selected = ""
	state.collapsed = map[string]bool{}
	state.step = -1
	state.scopeLabel = m.analysisScopeLabel(state.scope.kind, state.scope.directory, state.scope.pattern)
	state.visible = true
	m.DockShow(analysisInspectionWindow)
	m.invalidate()

	var stop func()
	if m.mgr != nil {
		events, unlisten := m.mgr.Listen(2048)
		done := make(chan struct{})
		stop = func() {
			unlisten()
			close(done)
		}
		go func() {
			for {
				select {
				case <-done:
					return
				case note := <-events:
					if note.Method != "channel" {
						continue
					}
					var message openidea.ChannelMessage
					if analysisJSON(note.Params, &message) != nil || message.ID != request {
						continue
					}
					data := message.Data
					m.post(func() {
						if m.analysis.generation == generation {
							m.acceptAnalysisInspectEvent(request, data)
						}
					})
				}
			}
		}()
	}
	args := map[string]any{
		"request": request,
		"scope":   scope,
		"onEvent": map[string]any{"$channel": request},
	}
	go func() {
		if stop != nil {
			defer stop()
		}
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Minute)
		defer cancel()
		_, callErr := m.invoke(ctx, "inspect_code", args)
		m.post(func() {
			if m.analysis.generation != generation || state.request != request {
				return
			}
			if callErr != nil && state.err == "" {
				if openidea.ErrorKind(callErr) != "cancelled" {
					state.err = "Inspect Code failed: " + callErr.Error()
				}
			}
			state.running = false
			m.invalidate()
		})
	}()
}

func (m *Model) acceptAnalysisInspectEvent(request uint32, data json.RawMessage) {
	state := &m.analysis.inspection
	if state.request != request {
		return
	}
	var event analysisInspectEvent
	if analysisJSON(data, &event) != nil {
		return
	}
	switch event.Type {
	case "started":
		state.total = event.Total
	case "file":
		state.done = event.Done
		if state.rawFiles == nil {
			state.rawFiles = map[string]analysisInspectedFile{}
		}
		if event.File.Path != "" {
			state.rawFiles[event.File.Path] = event.File
			m.syncAnalysisInspectionResults()
		}
	case "done":
		state.running = false
		summary := event.Summary
		state.summary = &summary
		if state.done < event.Summary.Files+event.Summary.Skipped+event.Summary.Failed {
			state.done = event.Summary.Files + event.Summary.Skipped + event.Summary.Failed
		}
		m.syncAnalysisInspectionResults()
	}
	m.invalidate()
}

func (m *Model) CancelAnalysisInspection() {
	state := &m.analysis.inspection
	if !state.running || state.request == 0 {
		return
	}
	request := state.request
	state.request = 0
	analysisCancelRequest(m, request)
	state.running = false
	state.profileWait = false
	state.err = ""
	m.status = "Inspection stopped"
	m.invalidate()
}

func analysisSeverity(d openidea.Diagnostic) string {
	switch strings.ToLower(d.Severity) {
	case "error":
		return "error"
	case "warning", "serverproblem":
		return "warning"
	default:
		return "info"
	}
}

func analysisInspectionKind(d openidea.Diagnostic) string {
	source := analysisOptionalString(d.Source)
	if source == "" {
		source = "Language server"
	}
	if server := analysisOptionalString(d.Server); source == "Language server" && server != "" {
		source = server
	}
	if code := analysisOptionalString(d.Code); code != "" {
		return source + "(" + code + ")"
	}
	return source
}

func analysisLevelSeverity(level string) string {
	switch level {
	case "error":
		return "error"
	case "warning", "serverProblem":
		return "warning"
	default:
		return "info"
	}
}

func analysisProducer(d openidea.Diagnostic) string {
	if server := analysisOptionalString(d.Server); server != "" {
		return server
	}
	return analysisOptionalString(d.Source)
}

func analysisBuiltinInspection(producer string) (builtinInspectionSpec, bool) {
	for _, spec := range builtinInspectionCatalog {
		if spec.Producer == producer {
			return spec, true
		}
	}
	return builtinInspectionSpec{}, false
}

func analysisProducerInspectionID(producer string) string {
	if builtin, ok := analysisBuiltinInspection(producer); ok {
		return builtin.ID
	}
	return "lsp:" + producer
}

func analysisInspectionEnabledByDefault(id string) bool {
	for _, spec := range builtinInspectionCatalog {
		if spec.ID == id {
			return spec.DefaultEnabled == nil || *spec.DefaultEnabled
		}
	}
	return true
}

func analysisApplyInspectionProfile(
	overrides map[string]inspectionOverrideSettings,
	file analysisInspectedFile,
) []openidea.Diagnostic {
	byProducer := map[string][]openidea.Diagnostic{}
	producerOrder := make([]string, 0)
	for _, diagnostic := range file.Diagnostics {
		producer := analysisProducer(diagnostic)
		if _, ok := byProducer[producer]; !ok {
			producerOrder = append(producerOrder, producer)
		}
		byProducer[producer] = append(byProducer[producer], diagnostic)
	}
	out := make([]openidea.Diagnostic, 0, len(file.Diagnostics))
	for _, producer := range producerOrder {
		base := analysisProducerInspectionID(producer)
		own, hasOwn := overrides[base]
		enabled := analysisInspectionEnabledByDefault(base)
		if hasOwn && own.Enabled != nil {
			enabled = *own.Enabled
		}
		if !enabled {
			continue
		}
		builtin, hasBuiltin := analysisBuiltinInspection(producer)
		for _, diagnostic := range byProducer[producer] {
			severity := diagnostic.Severity
			var codeOverride inspectionOverrideSettings
			hasCodeOverride := false
			if code := analysisOptionalString(diagnostic.Code); code != "" {
				codeOverride, hasCodeOverride = overrides[base+":"+code]
			}
			if hasCodeOverride && codeOverride.Enabled != nil && !*codeOverride.Enabled {
				continue
			}
			level := ""
			if hasCodeOverride && codeOverride.Severity != "" {
				level = codeOverride.Severity
			} else if hasOwn && own.Severity != "" {
				level = own.Severity
			} else if hasBuiltin {
				level = builtin.DefaultLevel
			}
			if level == "none" {
				severity = "hint"
			} else if level != "" {
				severity = analysisLevelSeverity(level)
			}
			diagnostic.Severity = severity
			out = append(out, diagnostic)
		}
	}
	return out
}

func (m *Model) syncAnalysisInspectionResults() {
	state := &m.analysis.inspection
	source := state.rawFiles
	if source == nil {
		source = state.files
	}
	profile := state.profile
	if profile == "" {
		profile = m.activeInspectionProfile()
	}
	overrides := inspectionOverridesFor(m.combinedInspectionDraft(), profile)
	files := make(map[string]analysisInspectedFile, len(source))
	problems := uint32(0)
	for path, file := range source {
		diagnostics := analysisApplyInspectionProfile(overrides, file)
		if len(diagnostics) == 0 {
			continue
		}
		file.Diagnostics = diagnostics
		files[path] = file
		problems += uint32(len(diagnostics))
	}
	state.files = files
	if state.summary != nil {
		state.summary.Problems = problems
	}
}

func (m *Model) analysisInspectionProblems() []analysisInspectionProblem {
	state := &m.analysis.inspection
	m.syncAnalysisInspectionResults()
	query := strings.ToLower(strings.TrimSpace(state.filter))
	problems := make([]analysisInspectionProblem, 0)
	for _, file := range state.files {
		for _, diagnostic := range file.Diagnostics {
			if state.severity != "" && analysisSeverity(diagnostic) != state.severity {
				continue
			}
			kind := analysisInspectionKind(diagnostic)
			haystack := strings.ToLower(file.RelPath + " " + file.Path + " " + kind + " " + diagnostic.Message)
			if query != "" && !strings.Contains(haystack, query) {
				continue
			}
			problems = append(problems, analysisInspectionProblem{File: file, Diag: diagnostic, Kind: kind})
		}
	}
	sort.SliceStable(problems, func(i, j int) bool {
		a, b := problems[i], problems[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.File.Path != b.File.Path {
			return a.File.Path < b.File.Path
		}
		if a.Diag.Range.Start.Line != b.Diag.Range.Start.Line {
			return a.Diag.Range.Start.Line < b.Diag.Range.Start.Line
		}
		return a.Diag.Range.Start.Col < b.Diag.Range.Start.Col
	})
	return problems
}

func analysisInspectionProblemID(problem analysisInspectionProblem) string {
	return fmt.Sprintf("%s\x00%d:%d\x00%s\x00%s",
		problem.File.Path,
		problem.Diag.Range.Start.Line,
		problem.Diag.Range.Start.Col,
		problem.Kind,
		problem.Diag.Message,
	)
}

func analysisInspectionID(problem analysisInspectionProblem) string {
	id := analysisProducerInspectionID(analysisProducer(problem.Diag))
	if code := analysisOptionalString(problem.Diag.Code); code != "" {
		id += ":" + code
	}
	return id
}

func analysisInspectionSeverityRank(severity string) int {
	switch severity {
	case "error":
		return 0
	case "warning":
		return 1
	default:
		return 2
	}
}

func analysisInspectionSeverityGroups(
	problems []analysisInspectionProblem,
) []analysisInspectionSeverityGroup {
	type fileGroup struct {
		path     string
		relPath  string
		problems []analysisInspectionProblem
	}
	type kindGroup struct {
		count int
		files map[string]*fileGroup
	}
	severities := map[string]map[string]*kindGroup{}
	for _, problem := range problems {
		severity := analysisSeverity(problem.Diag)
		kinds := severities[severity]
		if kinds == nil {
			kinds = map[string]*kindGroup{}
			severities[severity] = kinds
		}
		kind := kinds[problem.Kind]
		if kind == nil {
			kind = &kindGroup{files: map[string]*fileGroup{}}
			kinds[problem.Kind] = kind
		}
		file := kind.files[problem.File.Path]
		if file == nil {
			file = &fileGroup{path: problem.File.Path, relPath: problem.File.RelPath}
			if file.relPath == "" {
				file.relPath = problem.File.Path
			}
			kind.files[problem.File.Path] = file
		}
		file.problems = append(file.problems, problem)
		kind.count++
	}
	out := make([]analysisInspectionSeverityGroup, 0, len(severities))
	for _, severity := range []string{"error", "warning", "info"} {
		kinds := severities[severity]
		if len(kinds) == 0 {
			continue
		}
		group := analysisInspectionSeverityGroup{Severity: severity}
		for name, source := range kinds {
			kind := analysisInspectionKindGroup{
				Kind: name, Severity: severity, Count: source.count,
			}
			for _, file := range source.files {
				sort.SliceStable(file.problems, func(i, j int) bool {
					a, b := file.problems[i], file.problems[j]
					if a.Diag.Range.Start.Line != b.Diag.Range.Start.Line {
						return a.Diag.Range.Start.Line < b.Diag.Range.Start.Line
					}
					return a.Diag.Range.Start.Col < b.Diag.Range.Start.Col
				})
				kind.Files = append(kind.Files, analysisInspectionFileGroup{
					Path: file.path, RelPath: file.relPath, Problems: file.problems,
				})
			}
			sort.SliceStable(kind.Files, func(i, j int) bool {
				if kind.Files[i].RelPath != kind.Files[j].RelPath {
					return kind.Files[i].RelPath < kind.Files[j].RelPath
				}
				return kind.Files[i].Path < kind.Files[j].Path
			})
			group.Count += kind.Count
			group.Kinds = append(group.Kinds, kind)
		}
		sort.SliceStable(group.Kinds, func(i, j int) bool {
			if group.Kinds[i].Count != group.Kinds[j].Count {
				return group.Kinds[i].Count > group.Kinds[j].Count
			}
			return group.Kinds[i].Kind < group.Kinds[j].Kind
		})
		out = append(out, group)
	}
	return out
}

func analysisMergedInspectionKinds(
	groups []analysisInspectionSeverityGroup,
) []analysisInspectionKindGroup {
	merged := map[string]analysisInspectionKindGroup{}
	for _, severity := range groups {
		for _, kind := range severity.Kinds {
			existing, ok := merged[kind.Kind]
			if !ok {
				existing = analysisInspectionKindGroup{
					Kind: kind.Kind, Severity: kind.Severity,
				}
			} else if analysisInspectionSeverityRank(kind.Severity) <
				analysisInspectionSeverityRank(existing.Severity) {
				existing.Severity = kind.Severity
			}
			existing.Count += kind.Count
			for _, file := range kind.Files {
				found := -1
				for i := range existing.Files {
					if existing.Files[i].Path == file.Path {
						found = i
						break
					}
				}
				if found < 0 {
					existing.Files = append(existing.Files, analysisInspectionFileGroup{
						Path: file.Path, RelPath: file.RelPath,
						Problems: append([]analysisInspectionProblem(nil), file.Problems...),
					})
				} else {
					existing.Files[found].Problems = append(
						existing.Files[found].Problems, file.Problems...)
				}
			}
			merged[kind.Kind] = existing
		}
	}
	out := make([]analysisInspectionKindGroup, 0, len(merged))
	for _, kind := range merged {
		for i := range kind.Files {
			sort.SliceStable(kind.Files[i].Problems, func(a, b int) bool {
				left, right := kind.Files[i].Problems[a], kind.Files[i].Problems[b]
				if left.Diag.Range.Start.Line != right.Diag.Range.Start.Line {
					return left.Diag.Range.Start.Line < right.Diag.Range.Start.Line
				}
				return left.Diag.Range.Start.Col < right.Diag.Range.Start.Col
			})
		}
		sort.SliceStable(kind.Files, func(i, j int) bool {
			if kind.Files[i].RelPath != kind.Files[j].RelPath {
				return kind.Files[i].RelPath < kind.Files[j].RelPath
			}
			return kind.Files[i].Path < kind.Files[j].Path
		})
		out = append(out, kind)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a := analysisInspectionSeverityRank(out[i].Severity)
		b := analysisInspectionSeverityRank(out[j].Severity)
		if a != b {
			return a < b
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

func analysisInspectionKindNode(kind analysisInspectionKindGroup, prefix string) analysisInspectionNode {
	key := prefix + "\x00" + kind.Kind
	node := analysisInspectionNode{
		Kind: "inspection", Key: key, Label: kind.Kind,
		Severity: kind.Severity, Count: kind.Count,
	}
	for _, file := range kind.Files {
		fileKey := key + "\x00file:" + file.Path
		fileNode := analysisInspectionNode{
			Kind: "file", Key: fileKey, Label: filepath.Base(file.Path),
			Path: file.Path, Count: len(file.Problems),
		}
		for _, problem := range file.Problems {
			value := problem
			fileNode.Children = append(fileNode.Children, analysisInspectionNode{
				Kind: "problem", Key: analysisInspectionProblemID(value),
				Label:    strings.Split(value.Diag.Message, "\n")[0],
				Severity: analysisSeverity(value.Diag), Problem: &value,
			})
		}
		node.Children = append(node.Children, fileNode)
	}
	return node
}

func analysisInspectionNodes(
	problems []analysisInspectionProblem,
	groupBySeverity bool,
) []analysisInspectionNode {
	groups := analysisInspectionSeverityGroups(problems)
	if !groupBySeverity {
		kinds := analysisMergedInspectionKinds(groups)
		out := make([]analysisInspectionNode, 0, len(kinds))
		for _, kind := range kinds {
			out = append(out, analysisInspectionKindNode(kind, "kind"))
		}
		return out
	}
	out := make([]analysisInspectionNode, 0, len(groups))
	for _, severity := range groups {
		node := analysisInspectionNode{
			Kind: "severity", Key: "severity:" + severity.Severity,
			Label:    analysisInspectionSeverityLabel(severity.Severity),
			Severity: severity.Severity, Count: severity.Count,
		}
		for _, kind := range severity.Kinds {
			node.Children = append(node.Children, analysisInspectionKindNode(
				kind, "severity:"+severity.Severity,
			))
		}
		out = append(out, node)
	}
	return out
}

func analysisInspectionSeverityLabel(severity string) string {
	switch severity {
	case "error":
		return "Errors"
	case "warning":
		return "Warnings"
	default:
		return "Weak warnings and hints"
	}
}

func analysisInspectionNodeProblems(node analysisInspectionNode) []analysisInspectionProblem {
	if node.Problem != nil {
		return []analysisInspectionProblem{*node.Problem}
	}
	var out []analysisInspectionProblem
	for _, child := range node.Children {
		out = append(out, analysisInspectionNodeProblems(child)...)
	}
	return out
}

func (m *Model) analysisInspectionNodeExcluded(node analysisInspectionNode) bool {
	problems := analysisInspectionNodeProblems(node)
	if len(problems) == 0 {
		return false
	}
	for _, problem := range problems {
		if !m.analysis.inspection.excluded[analysisInspectionProblemID(problem)] {
			return false
		}
	}
	return true
}

func analysisInspectionNodeID(node analysisInspectionNode) string {
	id := ""
	for _, problem := range analysisInspectionNodeProblems(node) {
		next := analysisInspectionID(problem)
		if id == "" {
			id = next
		} else if id != next {
			return ""
		}
	}
	return id
}

func (m *Model) toggleAnalysisInspectionProblems(problems []analysisInspectionProblem) {
	if len(problems) == 0 {
		return
	}
	state := &m.analysis.inspection
	if state.excluded == nil {
		state.excluded = map[string]bool{}
	}
	allExcluded := true
	for _, problem := range problems {
		if !state.excluded[analysisInspectionProblemID(problem)] {
			allExcluded = false
			break
		}
	}
	for _, problem := range problems {
		id := analysisInspectionProblemID(problem)
		if allExcluded {
			delete(state.excluded, id)
		} else {
			state.excluded[id] = true
		}
	}
	m.invalidate()
}

func (m *Model) analysisInspectionProblem(id string) (analysisInspectionProblem, bool) {
	for _, problem := range m.analysisInspectionProblems() {
		if analysisInspectionProblemID(problem) == id {
			return problem, true
		}
	}
	return analysisInspectionProblem{}, false
}

func (m *Model) selectAnalysisInspectionProblem(
	problem analysisInspectionProblem,
	focus bool,
) {
	state := &m.analysis.inspection
	state.selected = analysisInspectionProblemID(problem)
	if focus {
		analysisOpenResult(m, problem.File.Path,
			int(problem.Diag.Range.Start.Line), int(problem.Diag.Range.Start.Col),
			int(problem.Diag.Range.End.Line), int(problem.Diag.Range.End.Col))
	} else if state.autoscroll {
		m.previewAt(problem.File.Path,
			int(problem.Diag.Range.Start.Line), int(problem.Diag.Range.Start.Col))
	}
	m.invalidate()
}

func (m *Model) showAnalysisInspectionQuickFixes(problem analysisInspectionProblem) {
	m.selectAnalysisInspectionProblem(problem, true)
	m.revealAt(problem.File.Path,
		int(problem.Diag.Range.Start.Line), int(problem.Diag.Range.Start.Col))
	m.ShowIntentions()
}

func (m *Model) StepAnalysisInspection(direction int) {
	problems := m.analysisInspectionProblems()
	navigable := make([]analysisInspectionProblem, 0, len(problems))
	for _, problem := range problems {
		if !m.analysis.inspection.excluded[analysisInspectionProblemID(problem)] {
			navigable = append(navigable, problem)
		}
	}
	if len(navigable) == 0 {
		return
	}
	state := &m.analysis.inspection
	index := -1
	for i, problem := range navigable {
		if analysisInspectionProblemID(problem) == state.selected {
			index = i
			break
		}
	}
	if index < 0 {
		if direction < 0 {
			index = len(navigable) - 1
		} else {
			index = 0
		}
	} else {
		index = (index + direction + len(navigable)) % len(navigable)
	}
	state.step = index
	m.selectAnalysisInspectionProblem(navigable[index], true)
}

func analysisInspectionCSV(problems []analysisInspectionProblem) (string, error) {
	rows := [][]string{{"Severity", "Inspection", "Path", "Line", "Column", "Message"}}
	for _, problem := range problems {
		rows = append(rows, []string{
			analysisSeverity(problem.Diag),
			problem.Kind,
			problem.File.Path,
			fmt.Sprint(problem.Diag.Range.Start.Line + 1),
			fmt.Sprint(problem.Diag.Range.Start.Col + 1),
			problem.Diag.Message,
		})
	}
	return analysisCSV(rows)
}

func (m *Model) ExportAnalysisInspection() {
	text, err := analysisInspectionCSV(m.analysisInspectionProblems())
	if err != nil {
		m.status = "Export Inspection Results failed: " + err.Error()
		m.invalidate()
		return
	}
	path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
		Title:       "Export Inspection Results",
		DefaultPath: filepath.Join(m.Root(), "inspection-results.csv"),
		Filters: []mygo.FileFilter{{
			Name: "CSV", Extensions: []string{"csv"},
		}},
	})
	if err != nil || path == "" {
		return
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		m.status = "Export Inspection Results failed: " + err.Error()
	} else {
		m.status = "Exported inspection results to " + filepath.Base(path)
	}
	m.invalidate()
}

func (m *Model) configureAnalysisInspection() {
	m.OpenSettingsPage(settingsPageInspections)
}

func (m *Model) editAnalysisInspection(id string) {
	if id != "" {
		m.inspectionsSettings.query = id
	}
	m.OpenSettingsPage(settingsPageInspections)
}

func (m *Model) disableAnalysisInspection(id string) {
	if id == "" {
		return
	}
	state := &m.analysis.inspection
	profile := state.profile
	if profile == "" {
		profile = m.activeInspectionProfile()
	}
	m.setActiveInspectionProfile(profile)
	overrides := cloneInspectionOverrides(m.activeInspectionOverrides())
	override := overrides[id]
	override.Enabled = boolPointer(false)
	overrides[id] = override
	m.setActiveInspectionOverrides(overrides)
	m.syncAnalysisInspectionResults()
	m.status = "Disabled inspection " + id
	m.invalidate()
	m.saveAnalysisInspectionSettings()
}

// saveAnalysisInspectionSettings commits the inspection draft after an action
// in Inspection Results changes it. Settings › Inspections uses the broader
// staged-settings save so other pages apply together.
func (m *Model) saveAnalysisInspectionSettings() {
	state := &m.inspectionsSettings
	if !state.loaded || state.saving {
		return
	}
	var writes []settingsBackendWrite
	if !sameInspectionOverrides(state.globalApplied.Default, state.globalDraft.Default) {
		writes = append(writes,
			settingsBackendWrite{"global", "inspections.default", state.globalDraft.Default})
	}
	if !sameInspectionProfiles(state.globalApplied.IDEProfiles, state.globalDraft.IDEProfiles) {
		writes = append(writes,
			settingsBackendWrite{"global", "inspections.ideProfiles", state.globalDraft.IDEProfiles})
	}
	profileScope := "project"
	if m.Root() == "" {
		profileScope = "global"
	}
	appliedProfile := state.projectApplied.Profile
	draftProfile := state.projectDraft.Profile
	if profileScope == "global" {
		appliedProfile, draftProfile = state.globalApplied.Profile, state.globalDraft.Profile
	}
	if appliedProfile != draftProfile {
		writes = append(writes,
			settingsBackendWrite{profileScope, "inspections.profile", draftProfile})
	}
	if m.Root() != "" {
		if !sameInspectionOverrides(state.projectApplied.Project, state.projectDraft.Project) {
			writes = append(writes,
				settingsBackendWrite{"project", "inspections.project", state.projectDraft.Project})
		}
		if !sameInspectionProfiles(state.projectApplied.ProjectProfiles, state.projectDraft.ProjectProfiles) {
			writes = append(writes,
				settingsBackendWrite{"project", "inspections.projectProfiles", state.projectDraft.ProjectProfiles})
		}
	}
	if len(writes) == 0 {
		return
	}
	globalSnapshot := cloneInspectionSettings(state.globalDraft)
	projectSnapshot := cloneInspectionSettings(state.projectDraft)
	root := m.Root()
	state.saving = true
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		for _, write := range writes {
			encoded, err := json.Marshal(write.value)
			if err == nil {
				_, err = m.invoke(ctx, "settings_set", map[string]any{
					"scope": write.scope, "key": write.key, "value": string(encoded),
				})
			}
			if err != nil {
				m.post(func() {
					m.inspectionsSettings.saving = false
					m.settingsErr = fmt.Sprintf("Could not save %s %s: %v",
						write.scope, write.key, err)
					m.invalidate()
				})
				return
			}
		}
		m.post(func() {
			m.inspectionsSettings.saving = false
			if m.Root() != root {
				return
			}
			m.inspectionsSettings.globalApplied = globalSnapshot
			m.inspectionsSettings.projectApplied = projectSnapshot
			m.syncAnalysisInspectionResults()
			m.invalidate()
		})
	}()
}

func (m *Model) analysisInspectionPanel(c *ui.Context, p *ui.Theme) {
	state := &m.analysis.inspection
	state.scope.normalize(m.analysisCurrentFile(), m.Root())
	if state.profile == "" {
		state.profile = m.activeInspectionProfile()
	}
	combined := m.combinedInspectionDraft()
	if !inspectionProfileExists(combined, state.profile, m.Root() != "") {
		state.profile = "default"
	}
	state.profileLabel = inspectionProfileLabel(combined, state.profile)
	profiles := inspectionProfileEntries(combined, m.Root() != "")
	profileLabels := make([]string, len(profiles))
	for i, profile := range profiles {
		profileLabels[i] = profile.Label
		if parseInspectionProfile(profile.ID).Key != "" {
			if profile.Level == "project" {
				profileLabels[i] += " (Project)"
			} else {
				profileLabels[i] += " (IDE)"
			}
		}
	}
	profileLabel := state.profileLabel
	for i, profile := range profiles {
		if profile.ID == state.profile {
			profileLabel = profileLabels[i]
			break
		}
	}
	problems := m.analysisInspectionProblems()
	ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Background(m.toolWindowBackground(p)).
		BorderWidth(1, 0, 0, 0).BorderColor(p.Border).Children(func() {
		ui.Row(c).Height(34).Padding(0, 8).Gap(6).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
			analysisButton(c, "◀", len(problems) > 0, func() { m.StepAnalysisInspection(-1) })
			analysisButton(c, "▶", len(problems) > 0, func() { m.StepAnalysisInspection(1) })
			analysisButton(c, "Rerun", !state.running, m.RunAnalysisInspection)
			analysisButton(c, "Stop", state.running, m.CancelAnalysisInspection)
			analysisButton(c, "Export…", len(problems) > 0, m.ExportAnalysisInspection)
			ui.TextInput(c, &state.filter).Grow(1).Placeholder("Filter inspection results")
		})
		ui.Column(c).Padding(8, 10).Gap(6).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Profile").FontSize(11).TextColor(p.TextMuted).Width(44)
				if ui.Select(c, &profileLabel, profileLabels).Width(220).Changed() {
					for i, label := range profileLabels {
						if label == profileLabel {
							m.setAnalysisInspectionProfile(profiles[i].ID)
							break
						}
					}
				}
				analysisButton(c, "Configure Inspections…", true, m.configureAnalysisInspection)
				options := ui.Button(c, "Options")
				options.Menu(m.analysisInspectionOptionsMenu)
				if state.groupBySeverity {
					ui.Text(c, "By severity").FontSize(10).TextColor(p.TextMuted)
				}
				if state.autoscroll {
					ui.Text(c, "Autoscroll").FontSize(10).TextColor(p.TextMuted)
				}
			})
			analysisScopeBar(c, p, &state.scope, true)
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				for _, item := range []struct {
					ID    string
					Label string
				}{{"", "All"}, {"error", "Errors"}, {"warning", "Warnings"}, {"info", "Hints"}} {
					item := item
					label := item.Label
					if state.severity == item.ID {
						label = "[" + label + "]"
					}
					analysisButton(c, label, true, func() { state.severity = item.ID; m.invalidate() })
				}
				analysisButton(c, "Run Inspection", !state.running, m.RunAnalysisInspection)
			})
		})
		if status := analysisInspectionStatus(state); status != "" {
			analysisStatusRow(c, p, status, state.err != "")
		}
		if len(problems) == 0 {
			hint := "Run Code › Inspect Code… to analyse files"
			if state.running {
				hint = "Inspecting files…"
			} else if state.summary != nil {
				hint = "No problems found"
			}
			ui.Text(c, hint).Padding(8, 12).Grow(1).FontSize(12).TextColor(p.TextMuted)
			return
		}
		ui.Row(c).Grow(1).MinHeight(0).MinWidth(0).AlignItems(ui.Stretch).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
				m.analysisInspectionTreeNodes(c, p, problems)
			})
			ui.Column(c).Width(1).Shrink(0).Background(p.Border)
			ui.Column(c).Width(360).Shrink(0).MinWidth(0).MinHeight(0).Children(func() {
				m.analysisInspectionDetails(c, p)
			})
		})
	})
}

func analysisInspectionStatus(state *analysisInspectionState) string {
	if state.err != "" {
		return state.err
	}
	if state.running {
		total := fmt.Sprint(state.total)
		if state.total == 0 {
			total = "?"
		}
		return fmt.Sprintf("Inspecting %s… %d/%s files", state.scopeLabel, state.done, total)
	}
	if state.summary == nil {
		return ""
	}
	s := state.summary
	text := fmt.Sprintf("%s: %d problem(s) in %d file(s)", state.scopeLabel, s.Problems, s.Files)
	if s.Skipped > 0 {
		text += fmt.Sprintf(", %d skipped", s.Skipped)
	}
	if s.Failed > 0 {
		text += fmt.Sprintf(", %d failed", s.Failed)
	}
	if s.Cancelled {
		text += " (stopped)"
	}
	if len(state.excluded) > 0 {
		text += fmt.Sprintf(", %d excluded", len(state.excluded))
	}
	return text
}

func (m *Model) analysisInspectionTreeNodes(
	c *ui.Context,
	p *ui.Theme,
	problems []analysisInspectionProblem,
) {
	state := &m.analysis.inspection
	if state.collapsed == nil {
		state.collapsed = map[string]bool{}
	}
	nodes := analysisInspectionNodes(problems, state.groupBySeverity)
	ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Padding(3, 5).Children(func() {
			for _, node := range nodes {
				m.analysisInspectionTreeNode(c, p, node, 0)
			}
		})
	})
}

func (m *Model) analysisInspectionTreeNode(
	c *ui.Context,
	p *ui.Theme,
	node analysisInspectionNode,
	depth int,
) {
	state := &m.analysis.inspection
	excluded := m.analysisInspectionNodeExcluded(node)
	selected := node.Kind == "problem" && node.Key == state.selected
	row := ui.Row(c.Key(node.Key)).MinHeight(24).PaddingStart(float32(4 + depth*18)).
		PaddingEnd(8).Gap(5).Radius(5).AlignItems(ui.Center)
	switch {
	case selected:
		row.Background(p.Selection)
	case row.Hovered():
		row.Background(p.SurfaceHover)
	}
	row.Children(func() {
		switch node.Kind {
		case "severity", "inspection", "file":
			glyph := "▾"
			if state.collapsed[node.Key] {
				glyph = "▸"
			}
			ui.Text(c, glyph).Width(14).FontSize(11).TextColor(p.TextMuted)
		}
		color := p.Text
		if excluded {
			color = p.TextMuted
		}
		switch node.Kind {
		case "severity":
			ui.Text(c, "●").FontSize(10).TextColor(analysisInspectionSeverityColor(p, node.Severity)).
				Width(12)
			text := ui.Text(c, node.Label).FontSize(12).Bold().TextColor(color).SingleLine()
			if excluded {
				text.Strikethrough()
			}
			ui.Spacer(c)
			ui.Text(c, fmt.Sprintf("%d problem(s)", node.Count)).
				FontSize(11).TextColor(p.TextMuted).SingleLine()
		case "inspection":
			if !state.groupBySeverity {
				ui.Text(c, "●").FontSize(10).
					TextColor(analysisInspectionSeverityColor(p, node.Severity)).Width(12)
			}
			text := ui.Text(c, node.Label).FontSize(12).Bold().TextColor(color).
				SingleLine().Ellipsis("…")
			if excluded {
				text.Strikethrough()
			}
			ui.Spacer(c)
			ui.Text(c, fmt.Sprint(node.Count)).FontSize(11).TextColor(p.TextMuted)
		case "file":
			fileIconEl(c, oiPaletteOf(p), node.Path, 16, p.TextMuted).MarginEnd(2)
			text := ui.Text(c, node.Label).FontSize(12).TextColor(color).SingleLine()
			if excluded {
				text.Strikethrough()
			}
			ui.Text(c, analysisRelative(m.Root(), node.Path)).
				FontSize(10).TextColor(p.TextMuted).SingleLine().Ellipsis("…")
			ui.Spacer(c)
			ui.Text(c, fmt.Sprint(node.Count)).FontSize(11).TextColor(p.TextMuted)
		case "problem":
			if node.Problem == nil {
				return
			}
			ui.Text(c, "●").FontSize(9).
				TextColor(analysisInspectionSeverityColor(p, node.Severity)).Width(12)
			text := ui.Text(c, node.Label).FontSize(12).TextColor(color).
				SingleLine().Ellipsis("…")
			if excluded {
				text.Strikethrough()
			}
			ui.Spacer(c)
			ui.Text(c, fmt.Sprintf(":%d", node.Problem.Diag.Range.Start.Line+1)).
				FontSize(11).TextColor(p.TextMuted)
		}
	})
	if row.Clicked() {
		if node.Kind == "problem" && node.Problem != nil {
			m.selectAnalysisInspectionProblem(*node.Problem, !state.autoscroll)
		} else {
			state.collapsed[node.Key] = !state.collapsed[node.Key]
			m.invalidate()
		}
	}
	if row.DoubleClicked() && node.Kind == "problem" && node.Problem != nil {
		m.selectAnalysisInspectionProblem(*node.Problem, true)
	}
	row.ContextMenu(func(menu *ui.Menu) {
		m.analysisInspectionNodeMenu(menu, node)
	})
	if node.Kind == "problem" || state.collapsed[node.Key] {
		return
	}
	for _, child := range node.Children {
		m.analysisInspectionTreeNode(c, p, child, depth+1)
	}
}

func analysisInspectionSeverityColor(p *ui.Theme, severity string) ui.Color {
	switch severity {
	case "error":
		return p.Danger
	case "warning":
		return p.Warning
	default:
		return p.TextMuted
	}
}

func (m *Model) analysisInspectionNodeMenu(menu *ui.Menu, node analysisInspectionNode) {
	problems := analysisInspectionNodeProblems(node)
	excluded := m.analysisInspectionNodeExcluded(node)
	if node.Kind == "problem" && node.Problem != nil {
		if menu.Item("Jump to Source").Chosen() {
			m.selectAnalysisInspectionProblem(*node.Problem, true)
		}
		if menu.Item("Show Quick Fixes").Chosen() {
			m.showAnalysisInspectionQuickFixes(*node.Problem)
		}
		menu.Separator()
	}
	label := "Exclude"
	if excluded {
		label = "Include"
	}
	if menu.Item(label).Disabled(len(problems) == 0).Chosen() {
		wasSelectedProblem := node.Kind == "problem" && node.Key == m.analysis.inspection.selected
		m.toggleAnalysisInspectionProblems(problems)
		if wasSelectedProblem && !excluded {
			m.StepAnalysisInspection(1)
		}
	}
	id := analysisInspectionNodeID(node)
	if id == "" {
		return
	}
	menu.Separator()
	if menu.Item("Disable Inspection").Chosen() {
		m.disableAnalysisInspection(id)
	}
	if menu.Item("Edit Inspection Setting…").Chosen() {
		m.editAnalysisInspection(id)
	}
}

func (m *Model) analysisInspectionDetails(c *ui.Context, p *ui.Theme) {
	state := &m.analysis.inspection
	problem, ok := m.analysisInspectionProblem(state.selected)
	if !ok {
		ui.Text(c, "Select a problem to see its details").
			Padding(12, 14).FontSize(12).TextColor(p.TextMuted)
		return
	}
	source := analysisInspectionKind(problem.Diag)
	location := analysisRelative(m.Root(), problem.File.Path)
	ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Padding(12, 14).Gap(9).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "●").FontSize(10).
					TextColor(analysisInspectionSeverityColor(p, analysisSeverity(problem.Diag)))
				ui.Text(c, source).FontSize(12).Bold().TextColor(p.Text).
					SingleLine().Ellipsis("…")
			})
			ui.Text(c, problem.Diag.Message).FontSize(12).TextColor(p.Text)
			ui.Text(c, fmt.Sprintf("%s:%d:%d", location,
				problem.Diag.Range.Start.Line+1, problem.Diag.Range.Start.Col+1)).
				Font("monospace").FontSize(11).TextColor(p.TextMuted).
				SingleLine().Ellipsis("…")
			ui.Row(c).Gap(7).Children(func() {
				analysisButton(c, "Show Quick Fixes", true, func() {
					m.showAnalysisInspectionQuickFixes(problem)
				})
				analysisButton(c, "Jump to Source", true, func() {
					m.selectAnalysisInspectionProblem(problem, true)
				})
			})
			menuButton := ui.Button(c, "Inspection Actions")
			menuButton.Menu(func(menu *ui.Menu) {
				id := analysisInspectionID(problem)
				if menu.Item("Disable Inspection").Chosen() {
					m.disableAnalysisInspection(id)
				}
				if menu.Item("Edit Inspection Setting…").Chosen() {
					m.editAnalysisInspection(id)
				}
			})
		})
	})
}
