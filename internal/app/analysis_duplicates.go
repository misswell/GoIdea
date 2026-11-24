package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

type analysisDuplicateOptions struct {
	MinTokens            uint32 `json:"minTokens"`
	AnonymizeIdentifiers bool   `json:"anonymizeIdentifiers"`
	AnonymizeLiterals    bool   `json:"anonymizeLiterals"`
}

type analysisDuplicateFragment struct {
	Path    string                `json:"path"`
	RelPath string                `json:"relPath"`
	Range   openidea.OutlineRange `json:"range"`
	Text    string                `json:"text"`
}

type analysisDuplicateGroup struct {
	Tokens    uint32                      `json:"tokens"`
	Lines     uint32                      `json:"lines"`
	Language  string                      `json:"language"`
	Fragments []analysisDuplicateFragment `json:"fragments"`
}

type analysisDuplicatesReport struct {
	Groups    []analysisDuplicateGroup `json:"groups"`
	Files     uint32                   `json:"files"`
	Tokens    uint32                   `json:"tokens"`
	Truncated bool                     `json:"truncated"`
	Cancelled bool                     `json:"cancelled"`
	ElapsedMs uint32                   `json:"elapsedMs"`
}

type analysisDuplicatesState struct {
	visible              bool
	optionsLoaded        bool
	running              bool
	request              uint32
	scope                analysisScopeState
	scopeLabel           string
	minText              string
	anonymizeIdentifiers bool
	anonymizeLiterals    bool
	report               *analysisDuplicatesReport
	err                  string
	collapsed            map[int]bool
	selected             int
}

func (m *Model) ShowAnalysisDuplicates() {
	state := &m.analysis.duplicates
	m.loadAnalysisDuplicateOptions()
	state.scope.normalize(m.analysisCurrentFile(), m.Root())
	state.visible = true
	m.DockShow(analysisDuplicatesWindow)
	m.RunAnalysisDuplicates()
}

func (m *Model) loadAnalysisDuplicateOptions() {
	state := &m.analysis.duplicates
	if state.optionsLoaded {
		return
	}
	state.optionsLoaded = true
	prefs := m.prefs.Analysis.Duplicates
	state.minText = strconv.Itoa(prefs.minTokens())
	state.anonymizeIdentifiers = prefs.anonymizeIdentifiers()
	state.anonymizeLiterals = prefs.anonymizeLiterals()
}

func (m *Model) saveAnalysisDuplicateOptions() {
	state := &m.analysis.duplicates
	parsed, err := strconv.ParseInt(strings.TrimSpace(state.minText), 10, 32)
	minimum := int(parsed)
	if err != nil {
		minimum = 60
	}
	next := DuplicateSearchPrefs{
		MinTokens:              minimum,
		NoAnonymizeIdentifiers: !state.anonymizeIdentifiers,
		NoAnonymizeLiterals:    !state.anonymizeLiterals,
	}
	if m.prefs.Analysis.Duplicates == next {
		return
	}
	m.prefs.Analysis.Duplicates = next
	m.savePreferences()
}

func (m *Model) RunAnalysisDuplicates() {
	state := &m.analysis.duplicates
	state.scope.normalize(m.analysisCurrentFile(), m.Root())
	minimum, err := strconv.ParseUint(strings.TrimSpace(state.minText), 10, 32)
	if err != nil || minimum < 10 || minimum > 10_000 {
		state.err = "Minimum size must be a whole number from 10 to 10000 tokens"
		m.invalidate()
		return
	}
	scope, err := m.analysisScopePayload(state.scope.kind, state.scope.directory, state.scope.pattern)
	if err != nil {
		state.err = "Locate Duplicates: " + err.Error()
		m.invalidate()
		return
	}
	if !analysisInvokeAvailable(m) {
		state.err = "Locate Duplicates needs the GoIDEA backend"
		m.invalidate()
		return
	}
	if state.request != 0 {
		analysisCancelRequest(m, state.request)
	}
	m.saveAnalysisDuplicateOptions()
	generation := m.analysis.generation
	request := m.nextRequest()
	state.request = request
	state.running = true
	state.report = nil
	state.err = ""
	state.collapsed = map[int]bool{}
	state.selected = -1
	state.scopeLabel = m.analysisScopeLabel(state.scope.kind, state.scope.directory, state.scope.pattern)
	state.visible = true
	m.DockShow(analysisDuplicatesWindow)
	m.invalidate()
	options := analysisDuplicateOptions{
		MinTokens:            uint32(minimum),
		AnonymizeIdentifiers: state.anonymizeIdentifiers,
		AnonymizeLiterals:    state.anonymizeLiterals,
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Minute)
		defer cancel()
		raw, callErr := m.invoke(ctx, "analysis_locate_duplicates", map[string]any{
			"request": request,
			"scope":   scope,
			"options": options,
		})
		var report analysisDuplicatesReport
		if callErr == nil {
			callErr = analysisJSON(raw, &report)
		}
		m.post(func() {
			if m.analysis.generation != generation || state.request != request {
				return
			}
			state.running = false
			if callErr != nil {
				state.err = "Locate Duplicates failed: " + callErr.Error()
			} else {
				state.report = &report
				if len(report.Groups) > 0 {
					state.selected = 0
				}
			}
			m.invalidate()
		})
	}()
}

func (m *Model) CancelAnalysisDuplicates() {
	state := &m.analysis.duplicates
	if !state.running || state.request == 0 {
		return
	}
	analysisCancelRequest(m, state.request)
	state.request = 0
	state.running = false
	state.err = ""
	m.status = "Duplicate search stopped"
	m.invalidate()
}

func (m *Model) analysisDuplicatesPanel(c *ui.Context, p *ui.Theme) {
	state := &m.analysis.duplicates
	m.loadAnalysisDuplicateOptions()
	state.scope.normalize(m.analysisCurrentFile(), m.Root())
	ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Background(m.toolWindowBackground(p)).
		BorderWidth(1, 0, 0, 0).BorderColor(p.Border).Children(func() {
		ui.Row(c).Height(34).Padding(0, 8).Gap(6).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
			analysisButton(c, "Rerun", !state.running, m.RunAnalysisDuplicates)
			analysisButton(c, "Stop", state.running, m.CancelAnalysisDuplicates)
			ui.Spacer(c)
			ui.Text(c, "Minimum").FontSize(11).TextColor(p.TextMuted)
			field := ui.TextInput(c, &state.minText).Width(70).Placeholder("60")
			if field.Submitted() {
				m.RunAnalysisDuplicates()
			}
			ui.Text(c, "tokens").FontSize(11).TextColor(p.TextMuted)
		})
		ui.Column(c).Padding(8, 10).Gap(6).Children(func() {
			analysisScopeBar(c, p, &state.scope, true)
			ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() {
				ui.Checkbox(c, &state.anonymizeIdentifiers, "Anonymize identifiers")
				ui.Checkbox(c, &state.anonymizeLiterals, "Anonymize literals")
				analysisButton(c, "Find Duplicates", !state.running, m.RunAnalysisDuplicates)
			})
		})
		if status := analysisDuplicateStatus(state); status != "" {
			analysisStatusRow(c, p, status, state.err != "")
		}
		ui.Row(c).Grow(1).MinHeight(0).MinWidth(0).AlignItems(ui.Stretch).Children(func() {
			ui.Column(c).Width(360).Shrink(0).MinWidth(0).MinHeight(0).Children(func() {
				ui.Scroll(c).Grow(1).Children(func() {
					ui.Column(c).Padding(0, 0, 6, 0).Children(func() {
						if state.report == nil || len(state.report.Groups) == 0 {
							hint := "Run Code › Analyze Code › Locate Duplicates… to find copied code"
							if state.running {
								hint = "Looking for duplicate code…"
							} else if state.report != nil {
								hint = "No duplicates found"
							}
							ui.Text(c, hint).Padding(8, 12).FontSize(12).TextColor(p.TextMuted)
							return
						}
						m.analysisDuplicateGroups(c, p)
					})
				})
			})
			ui.Column(c).Width(1).Shrink(0).Background(p.Border)
			ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
				m.analysisDuplicateFragments(c, p)
			})
		})
	})
}

func analysisDuplicateStatus(state *analysisDuplicatesState) string {
	if state.err != "" {
		return state.err
	}
	if state.running {
		return "Looking for duplicates in " + state.scopeLabel + "…"
	}
	if state.report == nil {
		return ""
	}
	r := state.report
	plus := ""
	if r.Truncated {
		plus = "+"
	}
	text := fmt.Sprintf("%s: %d%s duplicate group(s) in %d files (%d tokens)",
		state.scopeLabel, len(r.Groups), plus, r.Files, r.Tokens)
	if r.Cancelled {
		text += " (stopped)"
	}
	return text
}

func (m *Model) analysisDuplicateGroups(c *ui.Context, p *ui.Theme) {
	state := &m.analysis.duplicates
	if state.collapsed == nil {
		state.collapsed = map[int]bool{}
	}
	report := state.report
	for i, group := range report.Groups {
		i, group := i, group
		collapsed := state.collapsed[i]
		r := ui.Row(c.Key(fmt.Sprintf("duplicates:group:%d", i))).Height(27).
			PaddingStart(4).PaddingEnd(8).Gap(6).Radius(5).AlignItems(ui.Center)
		if state.selected == i || r.Hovered() {
			r.Background(p.SurfaceHover)
		}
		r.Children(func() {
			glyph := "▾"
			if collapsed {
				glyph = "▸"
			}
			ui.Text(c, glyph).Width(14).FontSize(11).TextColor(p.TextMuted)
			ui.Text(c, fmt.Sprintf("%d occurrences · %d lines", len(group.Fragments), group.Lines)).
				Bold().FontSize(12).TextColor(p.Text).SingleLine()
			ui.Text(c, group.Language).FontSize(11).TextColor(p.TextMuted)
			ui.Spacer(c)
			ui.Text(c, fmt.Sprintf("%d tokens", group.Tokens)).FontSize(11).TextColor(p.TextMuted)
		})
		if r.Clicked() {
			state.selected = i
			state.collapsed[i] = !collapsed
			m.invalidate()
		}
		if collapsed {
			continue
		}
		for j, fragment := range group.Fragments {
			j, fragment := j, fragment
			r := ui.Row(c.Key(fmt.Sprintf("duplicates:%d:fragment:%d", i, j))).Height(25).
				PaddingStart(24).PaddingEnd(8).Gap(6).Radius(5).AlignItems(ui.Center)
			if r.Hovered() {
				r.Background(p.SurfaceHover)
			}
			r.Children(func() {
				fileIconEl(c, oiPaletteOf(p), fragment.Path, 16, p.TextMuted).MarginEnd(2)
				ui.Text(c, filepath.Base(fragment.Path)).FontSize(12).TextColor(p.Text).SingleLine()
				ui.Text(c, fmt.Sprintf(":%d-%d", fragment.Range.Start.Line+1, fragment.Range.End.Line+1)).
					FontSize(11).TextColor(p.TextMuted)
				ui.Text(c, fragment.RelPath).FontSize(11).TextColor(p.TextMuted).
					SingleLine().Ellipsis("…")
			})
			if r.Clicked() {
				state.selected = i
				analysisOpenResult(m, fragment.Path,
					int(fragment.Range.Start.Line), int(fragment.Range.Start.Col),
					int(fragment.Range.End.Line), int(fragment.Range.End.Col))
			}
		}
	}
}

func (m *Model) analysisDuplicateFragments(c *ui.Context, p *ui.Theme) {
	state := &m.analysis.duplicates
	report := state.report
	if report == nil || state.selected < 0 || state.selected >= len(report.Groups) {
		ui.Text(c, "Select a duplicate group to compare its fragments").
			Padding(12, 14).FontSize(12).TextColor(p.TextMuted)
		return
	}
	group := report.Groups[state.selected]
	ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		ui.ScrollHorizontal(c).Grow(1).MinWidth(0).Children(func() {
			ui.Row(c).Padding(8, 10).Gap(8).AlignItems(ui.Stretch).Children(func() {
				for i, fragment := range group.Fragments {
					i, fragment := i, fragment
					ui.Column(c).Key(fmt.Sprintf("duplicates:preview:%d:%d", state.selected, i)).
						Width(360).Shrink(0).Gap(5).Children(func() {
						header := ui.Row(c).Height(26).Gap(5).AlignItems(ui.Center).Radius(4).
							Padding(0, 5).Tooltip("Open in editor")
						if header.Hovered() {
							header.Background(p.SurfaceHover)
						}
						header.Children(func() {
							fileIconEl(c, oiPaletteOf(p), fragment.Path, 16, p.TextMuted).MarginEnd(2)
							ui.Text(c, filepath.Base(fragment.Path)).Bold().FontSize(12).
								TextColor(p.Text).SingleLine()
							ui.Text(c, fmt.Sprintf("%d–%d",
								fragment.Range.Start.Line+1, fragment.Range.End.Line+1)).
								FontSize(11).TextColor(p.TextMuted).SingleLine()
							ui.Spacer(c)
							ui.Text(c, fragment.RelPath).FontSize(10).TextColor(p.TextMuted).
								SingleLine().Ellipsis("…")
						})
						if header.Clicked() {
							analysisOpenResult(m, fragment.Path,
								int(fragment.Range.Start.Line), int(fragment.Range.Start.Col),
								int(fragment.Range.End.Line), int(fragment.Range.End.Col))
						}
						ui.Column(c).Padding(6, 7).Gap(1).Radius(4).Background(p.Surface).
							Border(1, p.Border).Children(func() {
							lines := strings.Split(fragment.Text, "\n")
							for line, text := range lines {
								ui.Text(c, fmt.Sprintf("%4d  %s",
									int(fragment.Range.Start.Line)+line+1, text)).
									Font("monospace").FontSize(11).TextColor(p.Text).
									SingleLine().Ellipsis("…")
							}
						})
					})
				}
			})
		})
	})
}
