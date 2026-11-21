package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
)

// editorSafeDeleteTimeout bounds declaration, usage and syntax queries. JDT
// may validate the whole workspace before it can answer references.
const editorSafeDeleteTimeout = 60 * time.Second

type editorSafeDeletePhase int

const (
	editorSafeDeleteResolving editorSafeDeletePhase = iota
	editorSafeDeleteConfirm
	editorSafeDeleteSearching
	editorSafeDeleteUsages
	editorSafeDeleteTextPreview
	editorSafeDeleteApplying
)

// editorSafeDeleteState is OpenIDEA's editor Safe Delete flow. The target is
// resolved before the first dialog, then references and text mentions are
// searched after the user confirms. Byte offsets are tied to sourceVersion,
// so a concurrent edit cancels the deletion instead of removing other text.
type editorSafeDeleteState struct {
	phase         editorSafeDeletePhase
	tab           *Tab
	sourceVersion uint64
	selection     editor.Selection
	target        editorSafeDeleteTarget
	deleteStart   int
	deleteEnd     int
	searchText    bool
	usages        []openidea.Location
	textHits      []safeDeleteTextHit
	err           string

	usageRequest atomic.Uint32
	textRequest  atomic.Uint32
	cancelSearch context.CancelFunc
}

type editorSafeDeleteTarget struct {
	name   string
	kind   string
	at     openidea.OutlinePos
	rng    openidea.OutlineRange
	local  *safeDeleteLocalSymbol
	source string
}

type safeDeleteResolution struct {
	target  editorSafeDeleteTarget
	reason  string
	comment safeDeleteCommentSyntax
}

type safeDeleteCommentSyntax struct {
	line       string
	blockStart string
	blockEnd   string
}

type safeDeleteTextHit struct {
	offset int
	end    int
	line   int
	text   string
}

type safeDeleteLocalSymbol struct {
	Name       string                  `json:"name"`
	Kind       string                  `json:"kind"`
	Definition openidea.OutlineRange   `json:"definition"`
	Scope      openidea.OutlineRange   `json:"scope"`
	References []openidea.OutlineRange `json:"references"`
	Certain    bool                    `json:"certain"`
	Conflicts  []openidea.OutlineRange `json:"conflicts"`
}

type safeDeleteLocals struct {
	Status string                 `json:"status"`
	Symbol *safeDeleteLocalSymbol `json:"symbol"`
	Name   string                 `json:"name"`
}

type safeDeleteSyntaxNode struct {
	Kind  string                `json:"kind"`
	Range openidea.OutlineRange `json:"range"`
}

type safeDeleteStructuralRanges struct {
	Statement *safeDeleteSyntaxNode `json:"statement"`
}

type safeDeleteIdentifier struct {
	Range openidea.OutlineRange `json:"range"`
}

type safeDeleteLanguage struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	LineComment  *string  `json:"lineComment"`
	BlockComment []string `json:"blockComment"`
}

var (
	safeDeleteAnnotationRE = regexp.MustCompile(
		`^(?:@[A-Za-z_][A-Za-z0-9_.]*|#\[)`,
	)
	safeDeleteDeclarationRE = regexp.MustCompile(
		`declaration|assignment|^let|local|^var|^const|import|^use|expression_statement|property|short_var|definition$`,
	)
	safeDeleteCompoundRE = regexp.MustCompile(
		`^(?:for|while|if|with|try|catch|switch|match|do|loop|foreach|enhanced_for|case)`,
	)
	safeDeleteFunctionRE = regexp.MustCompile(
		`function|method|^def|^fn|func|procedure`,
	)
	safeDeleteClosingLineRE = regexp.MustCompile(`^(?:[}\])]|end\b)`)
	safeDeleteGenericRE     = regexp.MustCompile(`<[^<>]*>`)
	safeDeleteOtherNamesRE  = regexp.MustCompile(`[,(\[{]`)
	safeDeletePunctuationRE = regexp.MustCompile(`^[;,]\s*`)
)

// SafeDeleteActive runs Refactor › Safe Delete for the declaration at the
// editor caret.
func (m *Model) SafeDeleteActive() {
	t := m.tab()
	if t == nil || t.doc == nil || t.state == nil || !m.tabOpen(t) ||
		m.safeDelete != nil {
		return
	}
	if m.ctx == nil {
		m.status = "Safe Delete is unavailable"
		m.invalidate()
		return
	}
	if t.doc.ReadOnly() || t.state.ReadOnly {
		m.status = "The file is read-only"
		m.invalidate()
		return
	}
	state := &editorSafeDeleteState{
		phase:         editorSafeDeleteResolving,
		tab:           t,
		sourceVersion: t.doc.Buffer().Version(),
		selection:     t.state.Selection(),
		searchText:    m.safeDeleteSearchText,
	}
	m.safeDelete = state
	m.status = "Finding the symbol to delete…"
	m.invalidate()

	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, editorSafeDeleteTimeout)
		defer cancel()
		resolved, err := m.resolveEditorSafeDelete(ctx, state, t)
		m.post(func() {
			if m.safeDelete != state {
				return
			}
			if !m.tabOpen(t) || t.doc.Buffer().Version() != state.sourceVersion {
				m.closeEditorSafeDelete(state, "The file changed; Safe Delete cancelled")
				return
			}
			switch {
			case err != nil:
				m.safeDelete = nil
				m.status = "Safe Delete failed: " + editorSafeDeleteErrorText(err)
			case resolved.reason != "":
				m.safeDelete = nil
				m.status = "Cannot perform refactoring. " + resolved.reason
			default:
				state.target = resolved.target
				state.phase = editorSafeDeleteConfirm
				state.deleteStart, state.deleteEnd = t.doc.ByteRange(resolved.target.rng)
				deletion := safeDeleteDeletionRange(
					t.doc.Text(), state.deleteStart, state.deleteEnd, resolved.comment,
				)
				if deletion == nil {
					m.safeDelete = nil
					m.status = fmt.Sprintf(
						"Cannot perform refactoring. '%s' shares its line with other code",
						resolved.target.name,
					)
					break
				}
				state.deleteStart, state.deleteEnd = deletion.start, deletion.end
				state.target.source = t.doc.Text()
				m.status = ""
			}
			m.invalidate()
		})
	}()
}

func (m *Model) resolveEditorSafeDelete(
	ctx context.Context,
	state *editorSafeDeleteState,
	t *Tab,
) (safeDeleteResolution, error) {
	if err := t.doc.Sync(ctx); err != nil {
		return safeDeleteResolution{}, err
	}
	text := t.doc.Text()
	wordStart, wordEnd := safeDeleteWordRange(text, state.selection.Head)
	if wordStart == wordEnd {
		return safeDeleteResolution{reason: "Caret should be on a symbol"}, nil
	}
	name := text[wordStart:wordEnd]

	info := t.doc.Info()
	language, _ := m.editorSafeDeleteLanguage(ctx, info.Language)
	comments := language.commentSyntax(info.Language)

	defPos := t.doc.UTF16Pos(state.selection.Head)
	lspAvailable := true
	raw, err := m.invoke(ctx, "lsp_goto", map[string]any{
		"doc": t.doc.ID(), "pos": lspPos(defPos), "kind": "definition",
	})
	if err != nil {
		if ctx.Err() != nil || openidea.ErrorKind(err) == "cancelled" {
			return safeDeleteResolution{}, err
		}
		lspAvailable = false
	} else {
		var locations []openidea.Location
		if json.Unmarshal(raw, &locations) != nil {
			locations = nil
		}
		var here *openidea.Location
		for i := range locations {
			if samePath(locations[i].Path, t.doc.Path()) {
				here = &locations[i]
				break
			}
		}
		if here != nil {
			defPos = here.Range.Start
		} else if len(locations) > 0 {
			return safeDeleteResolution{
				reason: fmt.Sprintf(
					"'%s' is declared in %s; Safe Delete it there",
					name, filepath.Base(locations[0].Path),
				),
				comment: comments,
			}, nil
		}
	}

	var outline []openidea.OutlineItem
	if raw, err = m.invoke(ctx, "doc_outline", map[string]any{"doc": t.doc.ID()}); err == nil {
		_ = json.Unmarshal(raw, &outline)
	}
	item := safeDeleteOutlineItemAt(outline, defPos)
	if item != nil && !safeDeleteUnDeletable(item.Kind) && lspAvailable {
		return safeDeleteResolution{
			target: editorSafeDeleteTarget{
				name: item.Name,
				kind: safeDeleteKindLabel(item.Kind),
				at:   item.SelectionRange.Start,
				rng:  item.Range,
			},
			comment: comments,
		}, nil
	}

	raw, localErr := m.invoke(ctx, "syntax_locals", map[string]any{
		"doc": t.doc.ID(), "pos": lspPos(defPos),
	})
	if localErr == nil {
		var locals safeDeleteLocals
		if json.Unmarshal(raw, &locals) == nil && locals.Status == "local" && locals.Symbol != nil {
			raw, rangeErr := m.invoke(ctx, "syntax_structural_ranges", map[string]any{
				"doc": t.doc.ID(), "pos": lspPos(locals.Symbol.Definition.Start),
			})
			if rangeErr == nil {
				var ranges safeDeleteStructuralRanges
				if json.Unmarshal(raw, &ranges) == nil {
					declaration, reason := safeDeleteLocalDeclaration(
						t.doc, text, *locals.Symbol, ranges.Statement,
					)
					if reason != "" {
						return safeDeleteResolution{reason: reason, comment: comments}, nil
					}
					symbol := locals.Symbol
					targetLocal := symbol
					if lspAvailable {
						targetLocal = nil
					}
					return safeDeleteResolution{
						target: editorSafeDeleteTarget{
							name: symbol.Name,
							kind: safeDeleteKindLabel(
								safeDeleteLocalKind(symbol.Kind),
							),
							at:    symbol.Definition.Start,
							rng:   declaration,
							local: targetLocal,
						},
						comment: comments,
					}, nil
				}
			}
		}
	}

	if !lspAvailable {
		languageName := language.Name
		if languageName == "" {
			languageName = safeDeleteLanguageName(info.Language)
		}
		return safeDeleteResolution{
			reason: fmt.Sprintf(
				"The language server for %s is not available; only local declarations can be deleted safely without it",
				languageName,
			),
			comment: comments,
		}, nil
	}
	if item != nil {
		return safeDeleteResolution{
			reason:  fmt.Sprintf("'%s' can't be deleted", name),
			comment: comments,
		}, nil
	}
	return safeDeleteResolution{
		reason:  "Caret should be on the name of a declaration",
		comment: comments,
	}, nil
}

func (m *Model) editorSafeDeleteLanguage(
	ctx context.Context,
	id string,
) (safeDeleteLanguage, bool) {
	raw, err := m.invoke(ctx, "languages", nil)
	if err == nil {
		var all []safeDeleteLanguage
		if json.Unmarshal(raw, &all) == nil {
			for _, language := range all {
				if language.ID == id {
					return language, true
				}
			}
		}
	}
	return safeDeleteLanguage{ID: id, Name: safeDeleteLanguageName(id)}, false
}

func (l safeDeleteLanguage) commentSyntax(languageID string) safeDeleteCommentSyntax {
	comments := safeDeleteCommentSyntax{
		line:       "//",
		blockStart: "/*",
		blockEnd:   "*/",
	}
	switch languageID {
	case "python", "ruby", "shellscript", "bash", "r", "perl", "yaml", "toml":
		comments.line = "#"
		comments.blockStart, comments.blockEnd = "", ""
	}
	if l.LineComment != nil {
		comments.line = *l.LineComment
	}
	if len(l.BlockComment) == 2 {
		comments.blockStart, comments.blockEnd = l.BlockComment[0], l.BlockComment[1]
	}
	return comments
}

// confirmEditorSafeDelete starts the references and comments/string searches
// after IDEA's first confirmation.
func (m *Model) confirmEditorSafeDelete() {
	state := m.safeDelete
	if state == nil || state.phase != editorSafeDeleteConfirm ||
		!m.tabOpen(state.tab) {
		return
	}
	if state.tab.doc.Buffer().Version() != state.sourceVersion ||
		state.tab.state.Selection() != state.selection {
		m.closeEditorSafeDelete(state, "The file changed; Safe Delete cancelled")
		return
	}
	m.safeDeleteSearchText = state.searchText
	state.phase = editorSafeDeleteSearching
	state.err = ""
	ctx, cancel := context.WithTimeout(m.ctx, editorSafeDeleteTimeout)
	state.cancelSearch = cancel
	m.status = "Looking for usages…"
	m.invalidate()

	go func() {
		type result struct {
			usages []openidea.Location
			hits   []safeDeleteTextHit
			err    error
		}
		usageCh := make(chan result, 1)
		textCh := make(chan result, 1)
		go func() {
			usages, err := m.editorSafeDeleteUsages(ctx, state)
			usageCh <- result{usages: usages, err: err}
		}()
		if state.searchText {
			go func() {
				hits, err := m.editorSafeDeleteTextHits(ctx, state)
				textCh <- result{hits: hits, err: err}
			}()
		} else {
			textCh <- result{}
		}
		usageResult := <-usageCh
		textResult := <-textCh
		err := usageResult.err
		if err == nil {
			err = textResult.err
		}
		m.post(func() {
			if state.cancelSearch != nil {
				state.cancelSearch()
				state.cancelSearch = nil
			}
			if m.safeDelete != state {
				return
			}
			if !m.tabOpen(state.tab) ||
				state.tab.doc.Buffer().Version() != state.sourceVersion {
				m.closeEditorSafeDelete(state, "The file changed; Safe Delete cancelled")
				return
			}
			if err != nil {
				if ctx.Err() != nil ||
					openidea.ErrorKind(err) == "cancelled" {
					m.closeEditorSafeDelete(state, "Safe Delete cancelled")
					return
				}
				m.safeDelete = nil
				m.status = "Safe Delete search failed: " + editorSafeDeleteErrorText(err)
				m.invalidate()
				return
			}
			state.usages = append([]openidea.Location(nil), usageResult.usages...)
			state.textHits = append([]safeDeleteTextHit(nil), textResult.hits...)
			switch {
			case len(state.usages) > 0:
				state.phase = editorSafeDeleteUsages
				m.status = ""
				m.invalidate()
			case len(state.textHits) > 0:
				state.phase = editorSafeDeleteTextPreview
				m.status = ""
				m.invalidate()
			default:
				m.applyEditorSafeDelete(state)
			}
		})
	}()
}

func (m *Model) editorSafeDeleteUsages(
	ctx context.Context,
	state *editorSafeDeleteState,
) ([]openidea.Location, error) {
	if state.target.local != nil {
		out := make([]openidea.Location, 0, len(state.target.local.References))
		for _, rng := range state.target.local.References {
			if safeDeleteRangeOutside(rng, state.target.rng) {
				out = append(out, openidea.Location{
					Path: state.tab.doc.Path(), Range: rng,
				})
			}
		}
		return out, nil
	}
	request := m.nextRequest()
	channel := m.nextChannel()
	state.usageRequest.Store(request)
	defer state.usageRequest.CompareAndSwap(request, 0)
	raw, err := m.invoke(ctx, "lsp_references", map[string]any{
		"request": request, "doc": state.tab.doc.ID(),
		"pos":                lspPos(state.target.at),
		"includeDeclaration": false, "projectWide": true, "libraries": false,
		"onPartial": map[string]any{"$channel": channel},
	})
	if err != nil {
		return nil, err
	}
	var refs openidea.References
	if err := json.Unmarshal(raw, &refs); err != nil {
		return nil, err
	}
	out := make([]openidea.Location, 0, len(refs.Locations))
	seen := map[string]bool{}
	for _, location := range refs.Locations {
		if !safeDeleteUsageOutside(
			state.tab.doc.Path(), location, state.target.rng,
		) {
			continue
		}
		key := fmt.Sprintf("%s:%d:%d", filepath.Clean(location.Path),
			location.Range.Start.Line, location.Range.Start.Col)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, location)
	}
	return out, nil
}

func (m *Model) editorSafeDeleteTextHits(
	ctx context.Context,
	state *editorSafeDeleteState,
) ([]safeDeleteTextHit, error) {
	request := m.nextRequest()
	state.textRequest.Store(request)
	defer state.textRequest.CompareAndSwap(request, 0)
	raw, err := m.invoke(ctx, "syntax_identifiers", map[string]any{
		"doc": state.tab.doc.ID(), "name": state.target.name,
		"request": request,
	})
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		// OpenIDEA's comment preview is optional: no syntax tree means no
		// comment/string mentions can be distinguished from code.
		return nil, nil
	}
	var identifiers []safeDeleteIdentifier
	if json.Unmarshal(raw, &identifiers) != nil {
		return nil, nil
	}
	ranges := make([]openidea.OutlineRange, 0, len(identifiers))
	for _, identifier := range identifiers {
		ranges = append(ranges, identifier.Range)
	}
	start, end := state.tab.doc.ByteRange(state.target.rng)
	return safeDeleteTextOccurrences(
		state.target.source, state.target.name, ranges, start, end,
	), nil
}

// SafeDeleteViewUsages sends an already-finished Safe Delete search to the
// Find Usages tool window.
func (m *Model) SafeDeleteViewUsages() {
	state := m.safeDelete
	if state == nil || state.phase != editorSafeDeleteUsages {
		return
	}
	locations := append([]openidea.Location(nil), state.usages...)
	name := state.target.name
	m.safeDelete = nil
	m.showUsageResults(name, locations, nil, "")
	m.status = ""
	m.invalidate()
}

// SafeDeleteDeleteAnyway deletes a declaration that still has references.
func (m *Model) SafeDeleteDeleteAnyway() {
	state := m.safeDelete
	if state == nil || state.phase != editorSafeDeleteUsages {
		return
	}
	m.applyEditorSafeDelete(state)
}

// SafeDeletePreviewDelete confirms deleting a declaration whose name still
// appears in comments or strings.
func (m *Model) SafeDeletePreviewDelete() {
	state := m.safeDelete
	if state == nil || state.phase != editorSafeDeleteTextPreview {
		return
	}
	m.applyEditorSafeDelete(state)
}

// CancelSafeDelete closes the active editor Safe Delete flow.
func (m *Model) CancelSafeDelete() {
	if state := m.safeDelete; state != nil {
		if state.phase == editorSafeDeleteApplying {
			return
		}
		m.closeEditorSafeDelete(state, "Safe Delete cancelled")
	}
}

func (m *Model) applyEditorSafeDelete(state *editorSafeDeleteState) {
	if m.safeDelete != state || !m.tabOpen(state.tab) {
		return
	}
	if state.tab.doc.Buffer().Version() != state.sourceVersion {
		m.closeEditorSafeDelete(state, "The file changed; Safe Delete cancelled")
		return
	}
	state.phase = editorSafeDeleteApplying
	state.err = ""
	m.status = "Deleting…"
	m.invalidate()
	t := state.tab
	start, end := state.deleteStart, state.deleteEnd
	name, kind := state.target.name, state.target.kind
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, editorSafeDeleteTimeout)
		defer cancel()
		err := t.doc.ApplyEditsIfVersion(ctx, state.sourceVersion, editor.Edit{
			Start: start, End: end,
		})
		m.post(func() {
			if m.safeDelete != state {
				return
			}
			switch {
			case errors.Is(err, docbridge.ErrChanged):
				m.closeEditorSafeDelete(state, "The file changed; Safe Delete cancelled")
			case err != nil:
				m.safeDelete = nil
				m.status = "Safe Delete failed: " + editorSafeDeleteErrorText(err)
				m.fail(t.doc.Path(), fmt.Errorf("Safe Delete %s '%s': %w", kind, name, err))
			default:
				if m.tabOpen(t) {
					t.state.Select(start, start)
				}
				m.closeEditorSafeDelete(state,
					fmt.Sprintf("Deleted %s '%s'", strings.ToLower(kind), name))
			}
		})
	}()
}

func (m *Model) closeEditorSafeDelete(state *editorSafeDeleteState, status string) {
	if state == nil || m.safeDelete != state {
		return
	}
	if state.cancelSearch != nil {
		state.cancelSearch()
		state.cancelSearch = nil
	}
	if request := state.usageRequest.Swap(0); request != 0 {
		m.cancelRequest(request)
	}
	if request := state.textRequest.Swap(0); request != 0 {
		m.cancelRequest(request)
	}
	m.safeDelete = nil
	if status != "" {
		m.status = status
	}
	m.invalidate()
}

// safeDeleteDialog draws the confirmation, progress and result sheets in the
// same order OpenIDEA presents them.
func (m *Model) safeDeleteDialog(c *ui.Context, p *ui.Theme) {
	state := m.safeDelete
	if state == nil || state.phase == editorSafeDeleteResolving {
		return
	}
	open := true
	dialog := ui.Modal(c.Key(fmt.Sprintf("editor-safe-delete:%d", state.phase)),
		&open, func() {
			ui.Column(c).Width(500).Gap(10).Children(func() {
				switch state.phase {
				case editorSafeDeleteConfirm:
					ui.Text(c, "Safe Delete").FontSize(14).Bold().TextColor(p.Text)
					ui.Text(c, fmt.Sprintf(
						"Delete %s '%s'?",
						strings.ToLower(state.target.kind), state.target.name,
					)).FontSize(13).TextColor(p.Text)
					ui.Checkbox(c, &state.searchText, "Search in comments and strings")
					ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
						ui.Button(c, "Cancel").OnClick(m.CancelSafeDelete)
						ui.Button(c, "OK").OnClick(m.confirmEditorSafeDelete)
					})
				case editorSafeDeleteSearching:
					ui.Text(c, "Safe Delete").FontSize(14).Bold().TextColor(p.Text)
					ui.Text(c, "Looking for usages…").FontSize(13).TextColor(p.TextMuted)
					ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
						ui.Button(c, "Cancel").OnClick(m.CancelSafeDelete)
					})
				case editorSafeDeleteUsages:
					message, detail := safeDeleteUsagesMessage(
						state.target.kind, state.target.name, state.usages,
					)
					ui.Text(c, "Usages Detected").FontSize(14).Bold().TextColor(p.Warning)
					ui.Text(c, message).FontSize(13).Bold().TextColor(p.Text)
					ui.Text(c, detail).FontSize(12).TextColor(p.TextMuted)
					ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
						ui.Button(c, "Cancel").OnClick(m.CancelSafeDelete)
						ui.Button(c, "Delete Anyway").OnClick(m.SafeDeleteDeleteAnyway)
						ui.Button(c, "View Usages").OnClick(m.SafeDeleteViewUsages)
					})
				case editorSafeDeleteTextPreview:
					ui.Text(c, fmt.Sprintf(
						"Safe Delete %s '%s'",
						state.target.kind, state.target.name,
					)).FontSize(14).Bold().TextColor(p.Text)
					ui.Text(c, fmt.Sprintf(
						"'%s' is mentioned in comments or strings. The declaration will be deleted; these mentions stay.",
						state.target.name,
					)).FontSize(12).TextColor(p.Warning)
					ui.Scroll(c.Key("safe-delete-text-hits")).MaxHeight(300).Children(func() {
						for i, hit := range state.textHits {
							ui.Row(c.Key(fmt.Sprintf("safe-delete-text-hit:%d", i))).
								Height(24).Padding(0, 6).Children(func() {
								ui.Text(c, fmt.Sprintf("%d", hit.line+1)).
									Width(48).FontSize(12).TextColor(p.TextMuted)
								ui.Text(c, hit.text).FontSize(12).
									TextColor(p.Text).SingleLine().Ellipsis("…")
							})
						}
					})
					ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
						ui.Button(c, "Cancel").OnClick(m.CancelSafeDelete)
						ui.Button(c, "Delete").OnClick(m.SafeDeletePreviewDelete)
					})
				case editorSafeDeleteApplying:
					ui.Text(c, "Safe Delete").FontSize(14).Bold().TextColor(p.Text)
					ui.Text(c, "Deleting…").FontSize(13).TextColor(p.TextMuted)
				}
			})
		})
	if (dialog.Dismissed() || !open) &&
		state.phase != editorSafeDeleteApplying {
		m.CancelSafeDelete()
	}
}

func safeDeleteOutlineItemAt(
	items []openidea.OutlineItem,
	pos openidea.OutlinePos,
) *openidea.OutlineItem {
	for i := range items {
		item := &items[i]
		if !safeDeleteRangeContains(item.Range, pos) {
			continue
		}
		if inner := safeDeleteOutlineItemAt(item.Children, pos); inner != nil {
			return inner
		}
		if safeDeleteRangeContains(item.SelectionRange, pos) {
			return item
		}
	}
	return nil
}

func safeDeleteUnDeletable(kind string) bool {
	return kind == "file" || kind == "package" || kind == "namespace"
}

func safeDeleteKindLabel(kind string) string {
	labels := map[string]string{
		"enumMember":    "Enum constant",
		"enummember":    "Enum constant",
		"typeParameter": "Type parameter",
		"typeparameter": "Type parameter",
		"typeAlias":     "Type alias",
		"typealias":     "Type alias",
		"var":           "Variable",
		"function":      "Function",
		"import":        "Import",
	}
	if label := labels[kind]; label != "" {
		return label
	}
	if kind == "" {
		return "Symbol"
	}
	r, size := utf8.DecodeRuneInString(kind)
	if r == utf8.RuneError {
		return kind
	}
	return strings.ToUpper(string(r)) + kind[size:]
}

func safeDeleteLocalKind(kind string) string {
	if kind == "other" || kind == "" {
		return "variable"
	}
	return kind
}

func safeDeleteLanguageName(id string) string {
	switch id {
	case "java":
		return "Java"
	case "kotlin":
		return "Kotlin"
	case "typescript":
		return "TypeScript"
	case "javascript":
		return "JavaScript"
	case "python":
		return "Python"
	case "go":
		return "Go"
	case "rust":
		return "Rust"
	case "csharp":
		return "C#"
	case "cpp":
		return "C++"
	}
	return safeDeleteKindLabel(id)
}

func safeDeleteLocalDeclaration(
	doc *docbridge.Document,
	text string,
	symbol safeDeleteLocalSymbol,
	statement *safeDeleteSyntaxNode,
) (openidea.OutlineRange, string) {
	if symbol.Kind == "parameter" {
		return openidea.OutlineRange{},
			"Deleting a parameter changes its call sites; use Change Signature"
	}
	if symbol.Kind == "label" || symbol.Kind == "type" || symbol.Kind == "macro" {
		return openidea.OutlineRange{}, "Caret should be on a variable or function declaration"
	}
	if !symbol.Certain {
		return openidea.OutlineRange{},
			fmt.Sprintf("'%s' is declared more than once in this scope", symbol.Name)
	}
	if statement == nil || !safeDeleteRangeContains(statement.Range, symbol.Definition.Start) {
		return openidea.OutlineRange{}, "The declaration could not be determined"
	}
	if symbol.Kind == "function" {
		if safeDeleteFunctionRE.MatchString(statement.Kind) {
			return statement.Range, ""
		}
		return openidea.OutlineRange{}, "The declaration could not be determined"
	}
	if safeDeleteCompoundRE.MatchString(statement.Kind) ||
		!safeDeleteDeclarationRE.MatchString(statement.Kind) {
		return openidea.OutlineRange{},
			fmt.Sprintf("'%s' is not declared by a separate statement", symbol.Name)
	}

	from, to := doc.ByteRange(statement.Range)
	defStart, defEnd := doc.ByteRange(symbol.Definition)
	if from < 0 || to < from || defStart < from || defEnd < defStart || to > len(text) {
		return openidea.OutlineRange{}, "The declaration could not be determined"
	}
	after := text[defEnd:to]
	cut := len(after)
	if at := strings.IndexAny(after, "=;"); at >= 0 {
		cut = at
	}
	declarator := text[from:defEnd] + after[:cut]
	for i := 0; i < 5; i++ {
		next := safeDeleteGenericRE.ReplaceAllString(declarator, "")
		if next == declarator {
			break
		}
		declarator = next
	}
	if safeDeleteOtherNamesRE.MatchString(declarator) {
		return openidea.OutlineRange{}, "The declaration declares other names too"
	}
	return statement.Range, ""
}

// safeDeleteDeletionRange mirrors OpenIDEA's deletionRange: it removes whole
// lines, attached comments/annotations and one surrounding blank line.
func safeDeleteDeletionRange(
	text string,
	from int,
	to int,
	comments safeDeleteCommentSyntax,
) *safeDeleteRange {
	lines := strings.Split(text, "\n")
	if len(lines) == 0 {
		return nil
	}
	starts := make([]int, len(lines))
	ends := make([]int, len(lines))
	offset := 0
	for i, line := range lines {
		starts[i] = offset
		ends[i] = offset + len(line)
		offset = ends[i] + 1
	}
	from = min(max(from, 0), len(text))
	to = min(max(to, from), len(text))
	first := safeDeleteLineIndexAt(starts, ends, from)
	last := safeDeleteLineIndexAt(starts, ends, to)
	if strings.TrimSpace(text[starts[first]:from]) != "" {
		return nil
	}
	rest := strings.TrimSpace(text[to:ends[last]])
	restOK := rest == "" || rest == ";" || rest == ","
	if !restOK && comments.line != "" {
		restOK = strings.HasPrefix(rest, comments.line) ||
			(safeDeletePunctuationRE.ReplaceAllString(rest, "") != rest &&
				strings.HasPrefix(
					safeDeletePunctuationRE.ReplaceAllString(rest, ""),
					comments.line,
				))
	}
	if !restOK {
		return nil
	}

	startLine := first
	for n := first - 1; n >= 0; n-- {
		line := strings.TrimSpace(lines[n])
		if line == "" {
			break
		}
		if comments.blockEnd != "" && strings.HasSuffix(line, comments.blockEnd) {
			open := n
			for open >= 0 && !strings.Contains(lines[open], comments.blockStart) {
				open--
			}
			if open < 0 ||
				!strings.HasPrefix(strings.TrimSpace(lines[open]), comments.blockStart) {
				break
			}
			startLine = open
			n = open
			continue
		}
		if comments.line != "" && strings.HasPrefix(line, comments.line) ||
			safeDeleteAnnotationRE.MatchString(line) {
			startLine = n
			continue
		}
		break
	}

	start := starts[startLine]
	end := ends[last]
	blank := func(n int) bool {
		return n >= 0 && n < len(lines) && strings.TrimSpace(lines[n]) == ""
	}
	if last+1 < len(lines) {
		end = ends[last] + 1
		next := last + 1
		switch {
		case blank(next) && next+1 < len(lines):
			end = ends[next] + 1
		case blank(next):
			end = ends[next]
		case blank(startLine-1) &&
			safeDeleteClosingLineRE.MatchString(strings.TrimSpace(lines[next])):
			start = starts[startLine-1]
		}
	} else if startLine > 0 {
		chosen := startLine - 1
		if blank(startLine-1) && startLine > 1 {
			chosen = startLine - 2
		}
		start = ends[chosen]
	}
	return &safeDeleteRange{start: start, end: end}
}

type safeDeleteRange struct {
	start int
	end   int
}

func safeDeleteLineIndexAt(starts, ends []int, offset int) int {
	line := 0
	for line+1 < len(starts) && starts[line+1] <= offset {
		line++
	}
	if offset > ends[line] && line+1 < len(starts) {
		return line + 1
	}
	return line
}

// safeDeleteTextOccurrences finds whole-word mentions outside syntax
// identifiers and the declaration.
func safeDeleteTextOccurrences(
	text string,
	name string,
	identifiers []openidea.OutlineRange,
	excludeStart int,
	excludeEnd int,
) []safeDeleteTextHit {
	if name == "" {
		return nil
	}
	identifierOffsets := make(map[int]bool, len(identifiers))
	for _, rng := range identifiers {
		off := safeDeleteDocumentOffset(text, rng.Start)
		identifierOffsets[off] = true
	}
	var hits []safeDeleteTextHit
	for from := 0; from <= len(text)-len(name); {
		at := strings.Index(text[from:], name)
		if at < 0 {
			break
		}
		at += from
		end := at + len(name)
		from = at + 1
		if at >= excludeStart && at < excludeEnd {
			continue
		}
		if identifierOffsets[at] {
			continue
		}
		if safeDeleteWordByteBefore(text, at) ||
			safeDeleteWordByteAfter(text, end) {
			continue
		}
		line, lineText := safeDeleteLineTextAt(text, at)
		hits = append(hits, safeDeleteTextHit{
			offset: at, end: end, line: line, text: lineText,
		})
	}
	return hits
}

func safeDeleteWordRange(text string, off int) (int, int) {
	off = min(max(off, 0), len(text))
	start, end := off, off
	for start > 0 {
		r, size := utf8.DecodeLastRuneInString(text[:start])
		if !safeDeleteWordRune(r) {
			break
		}
		start -= size
	}
	for end < len(text) {
		r, size := utf8.DecodeRuneInString(text[end:])
		if !safeDeleteWordRune(r) {
			break
		}
		end += size
	}
	return start, end
}

func safeDeleteWordByteBefore(text string, at int) bool {
	if at <= 0 {
		return false
	}
	r, _ := utf8.DecodeLastRuneInString(text[:at])
	return safeDeleteWordRune(r)
}

func safeDeleteWordByteAfter(text string, at int) bool {
	if at >= len(text) {
		return false
	}
	r, _ := utf8.DecodeRuneInString(text[at:])
	return safeDeleteWordRune(r)
}

func safeDeleteWordRune(r rune) bool {
	return r == '_' || r == '$' ||
		unicode.IsLetter(r) || unicode.IsNumber(r)
}

func safeDeleteLineTextAt(text string, offset int) (int, string) {
	line := strings.Count(text[:min(max(offset, 0), len(text))], "\n")
	start := strings.LastIndex(text[:min(max(offset, 0), len(text))], "\n") + 1
	end := strings.IndexByte(text[start:], '\n')
	if end < 0 {
		end = len(text)
	} else {
		end += start
	}
	return line, text[start:end]
}

func safeDeleteDocumentOffset(text string, pos openidea.OutlinePos) int {
	lines := strings.Split(text, "\n")
	line := int(pos.Line)
	if line >= len(lines) {
		return len(text)
	}
	start := 0
	for i := 0; i < line; i++ {
		start += len(lines[i]) + 1
	}
	units := uint32(0)
	offset := start
	for offset < start+len(lines[line]) {
		r, size := utf8.DecodeRuneInString(text[offset:])
		if r == utf8.RuneError && size == 0 {
			break
		}
		next := units + uint32(utf16.RuneLen(r))
		if next > pos.Col {
			break
		}
		units = next
		offset += size
	}
	return offset
}

func safeDeleteUsageOutside(
	targetPath string,
	location openidea.Location,
	target openidea.OutlineRange,
) bool {
	if !samePath(location.Path, targetPath) {
		return true
	}
	return safeDeleteRangeOutside(location.Range, target)
}

func safeDeleteRangeOutside(
	rng openidea.OutlineRange,
	target openidea.OutlineRange,
) bool {
	return safeDeleteComparePos(rng.Start, target.Start) < 0 ||
		safeDeleteComparePos(rng.Start, target.End) > 0
}

func safeDeleteComparePos(a, b openidea.OutlinePos) int {
	switch {
	case a.Line < b.Line:
		return -1
	case a.Line > b.Line:
		return 1
	case a.Col < b.Col:
		return -1
	case a.Col > b.Col:
		return 1
	}
	return 0
}

func safeDeleteRangeContains(rng openidea.OutlineRange, pos openidea.OutlinePos) bool {
	return safeDeleteComparePos(pos, rng.Start) >= 0 &&
		safeDeleteComparePos(pos, rng.End) <= 0
}

func safeDeleteUsagesMessage(
	kind string,
	name string,
	usages []openidea.Location,
) (string, string) {
	count := len(usages)
	message := fmt.Sprintf("%d usages found", count)
	if count == 1 {
		message = "1 usage found"
	}
	files := map[string]bool{}
	for _, usage := range usages {
		files[filepath.Clean(usage.Path)] = true
	}
	where := ""
	if len(files) > 1 {
		where = fmt.Sprintf(" (in %d files)", len(files))
	}
	subject := fmt.Sprintf("%s '%s'", kind, name)
	if count == 1 {
		return message, fmt.Sprintf(
			"%s has 1 usage that is not safe to delete%s.",
			subject, where,
		)
	}
	return message, fmt.Sprintf(
		"%s has %d usages that are not safe to delete%s.",
		subject, count, where,
	)
}

func editorSafeDeleteErrorText(err error) string {
	switch {
	case openidea.IsOutOfSync(err):
		return "The file changed; start Safe Delete again"
	case openidea.ErrorKind(err) == "unavailable":
		return "No language server is available"
	case openidea.ErrorKind(err) == "notFound":
		return "The language server for this symbol was not found"
	}
	return err.Error()
}
