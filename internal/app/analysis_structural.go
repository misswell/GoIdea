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

type analysisStructuralLanguage struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type analysisStructuralMatch struct {
	Range       openidea.OutlineRange `json:"range"`
	Text        string                `json:"text"`
	LineText    string                `json:"lineText"`
	Replacement *string               `json:"replacement"`
}

type analysisStructuralFile struct {
	Path    string                    `json:"path"`
	RelPath string                    `json:"relPath"`
	Matches []analysisStructuralMatch `json:"matches"`
}

type analysisStructuralSummary struct {
	Files     uint32 `json:"files"`
	Matches   uint32 `json:"matches"`
	LimitHit  bool   `json:"limitHit"`
	Cancelled bool   `json:"cancelled"`
	ElapsedMs uint32 `json:"elapsedMs"`
}

type analysisStructuralEvent struct {
	Type    string                    `json:"type"`
	File    analysisStructuralFile    `json:"file"`
	Summary analysisStructuralSummary `json:"summary"`
}

type analysisStructuralState struct {
	visible          bool
	optionsLoaded    bool
	replace          bool
	running          bool
	validating       bool
	applying         bool
	request          uint32
	scope            analysisScopeState
	scopeLabel       string
	pattern          string
	replacement      string
	language         string
	languages        []analysisStructuralLanguage
	languagesLoading bool
	languagesErr     string
	validationErr    string
	files            map[string]analysisStructuralFile
	summary          *analysisStructuralSummary
	err              string
	applyStatus      string
	collapsed        map[string]bool
	included         map[string]bool
	prepared         *analysisWorkspaceEdit
	resultsShown     bool
	prepareStarted   bool
}

type analysisWorkspaceEdit struct {
	Files         []analysisWorkspaceFile         `json:"files"`
	Preconditions []analysisWorkspacePrecondition `json:"preconditions,omitempty"`
}

type analysisWorkspaceFile struct {
	Kind  string              `json:"kind"`
	Path  string              `json:"path"`
	Edits []openidea.TextEdit `json:"edits"`
}

type analysisWorkspacePrecondition struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
}

func (m *Model) ShowAnalysisStructural(replace bool) {
	state := &m.analysis.structural
	m.loadAnalysisStructuralOptions()
	state.replace = replace
	state.scope.normalize(m.analysisCurrentFile(), m.Root())
	state.visible = true
	m.DockShow(analysisStructuralWindow)
	m.loadAnalysisStructuralLanguages()
}

func (m *Model) loadAnalysisStructuralOptions() {
	state := &m.analysis.structural
	if state.optionsLoaded {
		return
	}
	state.optionsLoaded = true
	prefs := m.prefs.Analysis.Structural
	state.pattern = prefs.Pattern
	state.replacement = prefs.Replacement
}

func (m *Model) saveAnalysisStructuralOptions() {
	state := &m.analysis.structural
	next := StructuralFormPrefs{
		Pattern:     state.pattern,
		Replacement: state.replacement,
		Language:    state.language,
	}
	if m.prefs.Analysis.Structural == next {
		return
	}
	m.prefs.Analysis.Structural = next
	m.savePreferences()
}

func (m *Model) loadAnalysisStructuralLanguages() {
	state := &m.analysis.structural
	if len(state.languages) > 0 || state.languagesLoading || !analysisInvokeAvailable(m) {
		return
	}
	generation := m.analysis.generation
	root := m.Root()
	state.languagesLoading = true
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "structural_languages", map[string]any{})
		var languages []analysisStructuralLanguage
		if err == nil {
			err = analysisJSON(raw, &languages)
		}
		m.post(func() {
			if m.analysis.generation != generation || m.Root() != root {
				return
			}
			state.languagesLoading = false
			if err != nil {
				state.languagesErr = "Structural Search languages failed: " + err.Error()
				m.invalidate()
				return
			}
			state.languages = languages
			state.languagesErr = ""
			if state.language == "" {
				state.language = m.analysisLanguageForCurrentFile(languages)
			}
			if state.language == "" {
				for _, language := range languages {
					if language.ID == m.prefs.Analysis.Structural.Language {
						state.language = language.ID
						break
					}
				}
			}
			if state.language == "" && len(languages) > 0 {
				state.language = languages[0].ID
			}
			m.invalidate()
		})
	}()
}

func (m *Model) analysisLanguageForCurrentFile(languages []analysisStructuralLanguage) string {
	path := m.analysisCurrentFile()
	if path == "" {
		return ""
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	aliases := map[string][]string{
		"js":   {"javascript"},
		"jsx":  {"javascript", "jsx"},
		"ts":   {"typescript"},
		"tsx":  {"tsx", "typescript"},
		"kt":   {"kotlin"},
		"kts":  {"kotlin"},
		"py":   {"python"},
		"rb":   {"ruby"},
		"rs":   {"rust"},
		"go":   {"go"},
		"java": {"java"},
		"c":    {"c"},
		"h":    {"c", "cpp"},
		"cc":   {"cpp"},
		"cpp":  {"cpp"},
		"hpp":  {"cpp"},
	}
	candidates := append([]string{ext}, aliases[ext]...)
	for _, candidate := range candidates {
		for _, language := range languages {
			if strings.EqualFold(language.ID, candidate) {
				return language.ID
			}
		}
	}
	return ""
}

func analysisStructuralQuery(pattern, language string, replacement *string) map[string]any {
	return map[string]any{
		"pattern":     pattern,
		"language":    language,
		"replacement": replacement,
		"maxMatches":  0,
	}
}

// ValidateAnalysisStructural checks the current template through the pinned
// backend and keeps its error beside the form.
func (m *Model) ValidateAnalysisStructural() {
	state := &m.analysis.structural
	query := analysisStructuralQuery(state.pattern, state.language, nil)
	if strings.TrimSpace(state.pattern) == "" || state.language == "" {
		state.validationErr = "Choose a language and enter a search template"
		m.invalidate()
		return
	}
	if !analysisInvokeAvailable(m) {
		state.validationErr = "Structural validation needs the GoIDEA backend"
		m.invalidate()
		return
	}
	state.validating = true
	state.validationErr = ""
	generation := m.analysis.generation
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		_, err := m.invoke(ctx, "structural_validate", map[string]any{"query": query})
		m.post(func() {
			if m.analysis.generation != generation {
				return
			}
			state.validating = false
			if err != nil {
				state.validationErr = err.Error()
			} else {
				m.saveAnalysisStructuralOptions()
			}
			m.invalidate()
		})
	}()
}

// RunAnalysisStructural validates and then streams a structural search. A
// replace run keeps replacement text and prepares a workspace edit preview.
func (m *Model) RunAnalysisStructural() {
	state := &m.analysis.structural
	state.scope.normalize(m.analysisCurrentFile(), m.Root())
	if state.running || state.validating || !analysisInvokeAvailable(m) {
		return
	}
	if strings.TrimSpace(state.pattern) == "" || state.language == "" {
		state.validationErr = "Choose a language and enter a search template"
		m.invalidate()
		return
	}
	scope, err := m.analysisScopePayload(state.scope.kind, state.scope.directory, state.scope.pattern)
	if err != nil {
		state.err = "Structural Search: " + err.Error()
		m.invalidate()
		return
	}
	if state.request != 0 {
		analysisCancelRequest(m, state.request)
	}
	m.saveAnalysisStructuralOptions()
	generation := m.analysis.generation
	request := m.nextRequest()
	state.request = request
	state.running = true
	state.validating = true
	state.validationErr = ""
	state.err = ""
	state.applyStatus = ""
	state.files = map[string]analysisStructuralFile{}
	state.summary = nil
	state.collapsed = map[string]bool{}
	state.included = map[string]bool{}
	state.prepared = nil
	state.resultsShown = false
	state.prepareStarted = false
	state.scopeLabel = m.analysisScopeLabel(state.scope.kind, state.scope.directory, state.scope.pattern)
	pattern, language := state.pattern, state.language
	var replacement *string
	if state.replace {
		value := state.replacement
		replacement = &value
	}
	query := analysisStructuralQuery(pattern, language, replacement)
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
							m.acceptAnalysisStructuralEvent(request, data)
						}
					})
				}
			}
		}()
	}
	args := map[string]any{
		"request": request,
		"scope":   scope,
		"query":   query,
		"onEvent": map[string]any{"$channel": request},
	}
	openTabs := m.allTabs()
	go func() {
		if stop != nil {
			defer stop()
		}
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Minute)
		defer cancel()
		if err := analysisSyncOpenDocuments(ctx, openTabs); err != nil {
			m.post(func() {
				if m.analysis.generation == generation && state.request == request {
					state.running = false
					state.validating = false
					state.err = "Structural Search: " + err.Error()
					m.invalidate()
				}
			})
			return
		}
		if _, err := m.invoke(ctx, "structural_validate", map[string]any{"query": query}); err != nil {
			m.post(func() {
				if m.analysis.generation == generation && state.request == request {
					state.running = false
					state.validating = false
					state.validationErr = err.Error()
					m.invalidate()
				}
			})
			return
		}
		m.post(func() {
			if m.analysis.generation == generation && state.request == request {
				state.validating = false
				m.invalidate()
			}
		})
		_, callErr := m.invoke(ctx, "structural_search", args)
		m.post(func() {
			if m.analysis.generation != generation || state.request != request {
				return
			}
			if callErr != nil && state.err == "" {
				state.err = "Structural Search failed: " + callErr.Error()
			}
			state.running = false
			if callErr == nil && !state.replace {
				m.showAnalysisStructuralResults()
			}
			if callErr == nil && state.replace {
				m.prepareAnalysisStructuralEdit(request)
			}
			m.invalidate()
		})
	}()
}

func (m *Model) acceptAnalysisStructuralEvent(request uint32, data json.RawMessage) {
	state := &m.analysis.structural
	if state.request != request {
		return
	}
	var event analysisStructuralEvent
	if analysisJSON(data, &event) != nil {
		return
	}
	switch event.Type {
	case "file":
		if state.files == nil {
			state.files = map[string]analysisStructuralFile{}
		}
		if event.File.Path != "" {
			state.files[event.File.Path] = event.File
		}
	case "done":
		summary := event.Summary
		state.summary = &summary
		state.running = false
		if !state.replace {
			m.showAnalysisStructuralResults()
		} else {
			m.prepareAnalysisStructuralEdit(request)
		}
	}
	m.invalidate()
}

func (m *Model) CancelAnalysisStructural() {
	state := &m.analysis.structural
	if (!state.running && !state.validating) || state.request == 0 {
		return
	}
	analysisCancelRequest(m, state.request)
	state.request = 0
	state.running = false
	state.validating = false
	state.err = ""
	m.status = "Structural search stopped"
	m.invalidate()
}

func analysisStructuralSymbol(pattern string) string {
	pattern = strings.Join(strings.Fields(pattern), " ")
	runes := []rune(pattern)
	if len(runes) <= 60 {
		return pattern
	}
	return string(runes[:59]) + "…"
}

func (m *Model) showAnalysisStructuralResults() {
	state := &m.analysis.structural
	if state.replace || state.resultsShown || state.summary == nil ||
		state.summary.Cancelled || state.summary.Matches == 0 {
		return
	}
	locations := make([]openidea.Location, 0, state.summary.Matches)
	for _, file := range m.analysisStructuralFiles() {
		for _, match := range file.Matches {
			locations = append(locations, openidea.Location{Path: file.Path, Range: match.Range})
		}
	}
	if len(locations) == 0 {
		state.applyStatus = "No occurrences found"
		return
	}
	note := ""
	if state.summary.LimitHit {
		note = fmt.Sprintf("Stopped after %d matches", state.summary.Matches)
	}
	state.resultsShown = true
	m.showUsageResults(analysisStructuralSymbol(state.pattern), locations, nil, note)
}

func (m *Model) analysisStructuralFiles() []analysisStructuralFile {
	files := make([]analysisStructuralFile, 0, len(m.analysis.structural.files))
	for _, file := range m.analysis.structural.files {
		files = append(files, file)
	}
	sort.SliceStable(files, func(i, j int) bool { return files[i].RelPath < files[j].RelPath })
	return files
}

func (m *Model) analysisStructuralIncluded(path string, index int) bool {
	state := &m.analysis.structural
	key := analysisMatchKey(path, index)
	if state.included == nil {
		return true
	}
	value, ok := state.included[key]
	if !ok {
		return true
	}
	return value
}

func (m *Model) ToggleAnalysisStructuralMatch(path string, index int) {
	state := &m.analysis.structural
	if state.included == nil {
		state.included = map[string]bool{}
	}
	key := analysisMatchKey(path, index)
	state.included[key] = !m.analysisStructuralIncluded(path, index)
	m.invalidate()
}

func (m *Model) analysisStructuralEdit() analysisWorkspaceEdit {
	state := &m.analysis.structural
	edit := analysisWorkspaceEdit{}
	if state.prepared != nil {
		edit.Preconditions = append(edit.Preconditions, state.prepared.Preconditions...)
	}
	for _, file := range m.analysisStructuralFiles() {
		var edits []openidea.TextEdit
		for i, match := range file.Matches {
			if match.Replacement == nil || !m.analysisStructuralIncluded(file.Path, i) {
				continue
			}
			edits = append(edits, openidea.TextEdit{Range: match.Range, NewText: *match.Replacement})
		}
		if len(edits) > 0 {
			edit.Files = append(edit.Files, analysisWorkspaceFile{
				Kind: "edit", Path: file.Path, Edits: edits,
			})
		}
	}
	return edit
}

func (m *Model) prepareAnalysisStructuralEdit(request uint32) {
	state := &m.analysis.structural
	if !state.replace || state.request != request || state.prepareStarted ||
		state.summary == nil || state.summary.Cancelled {
		return
	}
	state.prepareStarted = true
	generation := m.analysis.generation
	edit := m.analysisStructuralEdit()
	if len(edit.Files) == 0 {
		state.applyStatus = "No replacements to preview"
		m.invalidate()
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
		defer cancel()
		openTabs := m.allTabs()
		if err := analysisSyncOpenDocuments(ctx, openTabs); err != nil {
			m.post(func() {
				if m.analysis.generation == generation && state.request == request {
					state.err = "Replace Structurally: " + err.Error()
					m.invalidate()
				}
			})
			return
		}
		raw, err := m.invoke(ctx, "prepare_workspace_edit", map[string]any{"edit": edit})
		var prepared analysisWorkspaceEdit
		if err == nil {
			err = analysisJSON(raw, &prepared)
		}
		m.post(func() {
			if m.analysis.generation != generation || state.request != request {
				return
			}
			if err != nil {
				state.err = "Replace Structurally preview failed: " + err.Error()
			} else {
				state.prepared = &prepared
				state.applyStatus = fmt.Sprintf("Ready to replace %d occurrence(s)",
					analysisStructuralReplacementCount(edit))
			}
			m.invalidate()
		})
	}()
}

func analysisStructuralReplacementCount(edit analysisWorkspaceEdit) int {
	count := 0
	for _, file := range edit.Files {
		count += len(file.Edits)
	}
	return count
}

func (m *Model) ApplyAnalysisStructuralReplace() {
	state := &m.analysis.structural
	if !state.replace || state.applying || state.running || state.prepared == nil {
		return
	}
	edit := m.analysisStructuralEdit()
	if len(edit.Files) == 0 {
		return
	}
	state.applying = true
	state.err = ""
	generation := m.analysis.generation
	m.invalidate()
	openTabs := m.allTabs()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
		defer cancel()
		err := analysisSyncOpenDocuments(ctx, openTabs)
		if err == nil {
			_, err = m.invoke(ctx, "apply_workspace_edit", map[string]any{"edit": edit})
		}
		m.post(func() {
			if m.analysis.generation != generation {
				return
			}
			if err == nil {
				state.applyStatus = fmt.Sprintf("Replaced %d occurrence(s)", analysisStructuralReplacementCount(edit))
			} else if openidea.IsOutOfSync(err) {
				state.err = "Replace Structurally: the files changed since the preview. Run the search again before replacing."
				state.applyStatus = ""
				state.prepared = nil
			} else {
				state.err = "Replace Structurally: " + err.Error()
			}
			state.applying = false
			m.invalidate()
		})
	}()
}

func (m *Model) analysisStructuralPanel(c *ui.Context, p *ui.Theme) {
	state := &m.analysis.structural
	m.loadAnalysisStructuralOptions()
	state.scope.normalize(m.analysisCurrentFile(), m.Root())
	files := m.analysisStructuralFiles()
	ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Background(m.toolWindowBackground(p)).
		BorderWidth(1, 0, 0, 0).BorderColor(p.Border).Children(func() {
		ui.Row(c).Height(34).Padding(0, 8).Gap(6).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
			analysisButton(c, "Validate", !state.validating && !state.running, m.ValidateAnalysisStructural)
			analysisButton(c, "Find", !state.validating && !state.running, m.RunAnalysisStructural)
			analysisButton(c, "Stop", state.running || state.validating, m.CancelAnalysisStructural)
			analysisButton(c, "Replace", state.replace && state.prepared != nil && !state.applying,
				m.ApplyAnalysisStructuralReplace)
			ui.Spacer(c)
			ui.Text(c, fmt.Sprintf("%d match(es)", analysisStructuralMatchCount(files))).
				FontSize(11).TextColor(p.TextMuted)
		})
		ui.Column(c).Padding(8, 10).Gap(6).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Language").FontSize(11).TextColor(p.TextMuted).Width(60)
				labels := make([]string, 0, len(state.languages))
				current := ""
				for _, language := range state.languages {
					labels = append(labels, language.Name)
					if language.ID == state.language {
						current = language.Name
					}
				}
				chosen := current
				ui.Select(c, &chosen, labels).Width(190)
				if chosen != current {
					for _, language := range state.languages {
						if language.Name == chosen {
							state.language = language.ID
							break
						}
					}
				}
				ui.Text(c, "Pattern").FontSize(11).TextColor(p.TextMuted)
				ui.TextInput(c, &state.pattern).Grow(1).Placeholder("$a$.equals($b$)")
			})
			if state.replace {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "Replace with").FontSize(11).TextColor(p.TextMuted).Width(92)
					ui.TextInput(c, &state.replacement).Grow(1).Placeholder("Objects.equals($a$, $b$)")
				})
			}
			analysisScopeBar(c, p, &state.scope, true)
			ui.Text(c, "Placeholders: $name$ or $NAME$ match one node; $$$NAME matches many. Reused placeholders must match equal code.").
				FontSize(11).TextColor(p.TextMuted).SingleLine().Ellipsis("…")
			if state.validationErr != "" {
				ui.Text(c, state.validationErr).FontSize(11).TextColor(p.Danger)
			}
			if state.languagesErr != "" {
				ui.Text(c, state.languagesErr).FontSize(11).TextColor(p.Danger)
			}
		})
		if status := analysisStructuralStatus(state, files); status != "" {
			analysisStatusRow(c, p, status, state.err != "")
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(0, 0, 6, 0).Children(func() {
				if len(files) == 0 {
					hint := "Enter a structural pattern, validate it, then run a search"
					if state.running {
						hint = "Searching syntax trees…"
					} else if state.summary != nil {
						hint = "No matching code found"
					}
					ui.Text(c, hint).Padding(8, 12).FontSize(12).TextColor(p.TextMuted)
					return
				}
				m.analysisStructuralResults(c, p, files)
			})
		})
	})
}

func analysisStructuralMatchCount(files []analysisStructuralFile) int {
	count := 0
	for _, file := range files {
		count += len(file.Matches)
	}
	return count
}

func analysisStructuralStatus(state *analysisStructuralState, files []analysisStructuralFile) string {
	if state.err != "" {
		return state.err
	}
	if state.applyStatus != "" {
		return state.applyStatus
	}
	if state.validating {
		return "Validating structural template…"
	}
	if state.running {
		return "Searching " + state.scopeLabel + " for structural matches…"
	}
	if state.summary == nil {
		return ""
	}
	s := state.summary
	text := fmt.Sprintf("%s: %d match(es) in %d file(s)", state.scopeLabel, s.Matches, s.Files)
	if s.LimitHit {
		text += " (match limit reached)"
	}
	if s.Cancelled {
		text += " (stopped)"
	}
	return text
}

func (m *Model) analysisStructuralResults(c *ui.Context, p *ui.Theme, files []analysisStructuralFile) {
	state := &m.analysis.structural
	for _, file := range files {
		file := file
		collapsed := state.collapsed[file.Path]
		r := ui.Row(c.Key("structural:file:" + file.Path)).Height(25).PaddingStart(4).PaddingEnd(8).
			Gap(5).Radius(5).AlignItems(ui.Center)
		if r.Hovered() {
			r.Background(p.SurfaceHover)
		}
		r.Children(func() {
			glyph := "▾"
			if collapsed {
				glyph = "▸"
			}
			ui.Text(c, glyph).Width(14).FontSize(11).TextColor(p.TextMuted)
			fileIconEl(c, oiPaletteOf(p), file.Path, 16, p.TextMuted).MarginEnd(2)
			ui.Text(c, filepath.Base(file.Path)).Bold().FontSize(12).TextColor(p.Text).SingleLine()
			ui.Text(c, file.RelPath).FontSize(11).TextColor(p.TextMuted).SingleLine().Ellipsis("…")
			ui.Spacer(c)
			ui.Text(c, fmt.Sprint(len(file.Matches))).FontSize(11).TextColor(p.TextMuted)
		})
		if r.Clicked() {
			state.collapsed[file.Path] = !collapsed
			m.invalidate()
		}
		if collapsed {
			continue
		}
		for i, match := range file.Matches {
			i, match := i, match
			r := ui.Row(c.Key(fmt.Sprintf("structural:%s:%d", file.Path, i))).Height(38).
				PaddingStart(24).PaddingEnd(8).Gap(7).Radius(5).AlignItems(ui.Center)
			if r.Hovered() {
				r.Background(p.SurfaceHover)
			}
			r.Children(func() {
				if state.replace {
					included := m.analysisStructuralIncluded(file.Path, i)
					label := "☐"
					if included {
						label = "☑"
					}
					ui.Text(c, label).FontSize(14).TextColor(p.Accent)
				}
				ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
					ui.Text(c, fmt.Sprintf("Line %d · %s", match.Range.Start.Line+1,
						strings.TrimSpace(match.LineText))).FontSize(11).TextColor(p.TextMuted).
						SingleLine().Ellipsis("…")
					ui.Text(c, strings.TrimSpace(match.Text)).Font("monospace").FontSize(11).
						TextColor(p.Text).SingleLine().Ellipsis("…")
					if match.Replacement != nil {
						ui.Text(c, "→ "+strings.TrimSpace(*match.Replacement)).Font("monospace").
							FontSize(11).TextColor(p.Success).SingleLine().Ellipsis("…")
					}
				})
			})
			if r.Clicked() {
				if state.replace {
					m.ToggleAnalysisStructuralMatch(file.Path, i)
				}
			}
			if r.DoubleClicked() {
				analysisOpenResult(m, file.Path,
					int(match.Range.Start.Line), int(match.Range.Start.Col),
					int(match.Range.End.Line), int(match.Range.End.Col))
			}
		}
	}
}
