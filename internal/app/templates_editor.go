package app

import (
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

type templateCompletionAction struct {
	TemplateID string
	From, To   int
	Expansion  templateExpansion
	Postfix    bool
}

type completionInfo struct {
	Template *templateCompletionAction
	LSP      *openidea.CompletionItem
	Replace  *completionReplacement
}

// completionReplacement is a completion that replaces the backend's complete
// value span, even when the caret is in the middle of it. MyGo can replace the
// typed prefix to the caret itself; this path applies the wider XML value.
type completionReplacement struct {
	From int
	To   int
}

type completionTracking struct {
	version          uint64
	cursor           int
	start            int
	infos            map[string]completionInfo
	selected         *editor.Completion
	open             bool
	generation       uint64
	explicit         bool
	focused          bool
	itemCount        int
	selectedIndex    int
	lastSelected     int
	autoTimer        *time.Timer
	autoKey          string
	documentation    *time.Timer
	documentationKey string
	consumeText      string
}

func completionTrackKey(item editor.Completion) string {
	return item.Label + "\x00" + item.Insert
}

func (m *Model) templateContext(t *Tab, selection string) liveTemplateContext {
	path := ""
	if t != nil && t.doc != nil {
		path = t.doc.Path()
	}
	ctx := liveTemplateContext{
		Path: path, Project: m.Root(), FileName: path,
		User: projectNewUserName(m.Root()), Now: time.Now(), Selection: selection,
	}
	ctx.ClassName = projectNewStyleName(strings.TrimSuffix(templateFileBase(path), templateFileExt(path)), "pascal")
	return ctx
}

func templateFileBase(path string) string {
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		return path[i+1:]
	}
	return path
}

func templateFileExt(path string) string {
	base := templateFileBase(path)
	if i := strings.LastIndexByte(base, '.'); i >= 0 {
		return base[i:]
	}
	return ""
}

func (m *Model) liveTemplateCompletionItems(
	t *Tab,
	req editor.CompletionRequest,
) (int, []editor.Completion, map[string]completionInfo) {
	if t == nil || t.doc == nil {
		return req.Offset, nil, nil
	}
	prefs := m.templates()
	if !prefs.Live.Enabled || !prefs.Live.ShowInCompletion {
		return req.Offset, nil, nil
	}
	language := templateLanguage(t.doc.Path())
	typed := req.Prefix
	start := max(0, req.Offset-len(typed))
	var items []editor.Completion
	infos := map[string]completionInfo{}
	for _, template := range prefs.liveTemplates() {
		if !templateContextApplies(template, language) {
			continue
		}
		if !req.Explicit && typed != "" &&
			!templatePrefixMatches(template.Abbreviation, typed, prefs.Live.CaseSensitive) {
			continue
		}
		expansion := expandTemplate(template.Body, m.templateContext(t, ""))
		item := editor.Completion{
			Label:      template.Abbreviation,
			Detail:     template.Name,
			Insert:     expansion.Text,
			FilterText: template.Abbreviation,
			Kind:       editor.CompletionSnippet,
		}
		items = append(items, item)
		infos[completionTrackKey(item)] = completionInfo{Template: &templateCompletionAction{
			TemplateID: template.ID, From: start, To: req.Offset, Expansion: expansion,
		}}
		if len(items) >= 8 && !req.Explicit {
			break
		}
	}
	return start, items, infos
}

func templatePrefixMatches(value, prefix string, caseSensitive bool) bool {
	if !caseSensitive {
		value = strings.ToLower(value)
		prefix = strings.ToLower(prefix)
	}
	return strings.HasPrefix(value, prefix)
}

func (m *Model) emmetCompletionItems(
	t *Tab,
	req editor.CompletionRequest,
) (int, []editor.Completion, map[string]completionInfo) {
	if t == nil || t.doc == nil || !m.templates().Emmet.Enabled {
		return req.Offset, nil, nil
	}
	language := templateLanguage(t.doc.Path())
	if language != "html" && language != "xml" && language != "css" {
		return req.Offset, nil, nil
	}
	text := t.doc.Text()
	if req.Offset < 0 || req.Offset > len(text) {
		return req.Offset, nil, nil
	}
	start := emmetAbbreviationStart(text, req.Offset)
	if start >= req.Offset {
		return req.Offset, nil, nil
	}
	abbreviation := strings.TrimSpace(text[start:req.Offset])
	expanded, ok := expandEmmet(abbreviation)
	if !ok {
		return req.Offset, nil, nil
	}
	item := editor.Completion{
		Label: abbreviation, Detail: "Emmet abbreviation", Insert: expanded,
		FilterText: req.Prefix, Kind: editor.CompletionSnippet,
	}
	return start, []editor.Completion{item}, map[string]completionInfo{
		completionTrackKey(item): {Template: &templateCompletionAction{
			TemplateID: "emmet", From: start, To: req.Offset,
			Expansion: templateExpansion{Text: expanded, Caret: len(expanded)},
		}},
	}
}

func emmetAbbreviationStart(text string, offset int) int {
	if offset <= 0 || offset > len(text) {
		return offset
	}
	i := offset
	for i > 0 {
		r := rune(text[i-1])
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '<' || r == '=' || r == '"' || r == '\'' {
			break
		}
		if strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789.#>+*[]{}:-_$", r) {
			i--
			continue
		}
		break
	}
	return i
}

func (m *Model) requestPostfixCompletion(t *Tab, req editor.CompletionRequest) bool {
	if t == nil || t.doc == nil {
		return false
	}
	text := t.doc.Text()
	prefs := m.templates().Postfix
	if !prefs.Enabled {
		return false
	}
	language := templateLanguage(t.doc.Path())
	candidates := postfixExpansionCandidates(
		text, req.Offset, language, prefs, m.templateContext(t, ""),
	)
	if len(candidates) == 0 {
		return false
	}
	start := candidates[0].From
	for _, candidate := range candidates[1:] {
		start = min(start, candidate.From)
	}
	typed := text[start:req.Offset]
	items := make([]editor.Completion, 0, len(candidates))
	infos := map[string]completionInfo{}
	for _, candidate := range candidates {
		item := editor.Completion{
			Label: candidate.Expression, Detail: "." + strings.TrimPrefix(candidate.TemplateID, "postfix."),
			Insert: candidate.Text.Text, FilterText: typed, Kind: editor.CompletionSnippet,
		}
		items = append(items, item)
		infos[completionTrackKey(item)] = completionInfo{Template: &templateCompletionAction{
			TemplateID: candidate.TemplateID, From: candidate.From, To: candidate.To,
			Expansion: candidate.Text, Postfix: true,
		}}
	}
	m.showTrackedCompletions(t, req, start, items, infos)
	return true
}

func (m *Model) showTrackedCompletions(
	t *Tab,
	req editor.CompletionRequest,
	start int,
	items []editor.Completion,
	infos map[string]completionInfo,
) {
	if t == nil || t.state == nil || len(items) == 0 {
		return
	}
	old := t.completion
	if old.autoTimer != nil {
		old.autoTimer.Stop()
	}
	if old.documentation != nil {
		old.documentation.Stop()
	}
	generation := old.generation + 1
	explicit := req.Explicit
	if old.version == req.Version && old.cursor == req.Offset && old.generation > 0 &&
		old.itemCount == 0 && old.explicit {
		explicit = true
	}
	t.completion = completionTracking{
		version: req.Version, cursor: req.Offset, start: start,
		infos: infos, open: true, generation: generation,
		explicit: explicit, focused: explicit, lastSelected: -1,
	}
	t.state.ShowCompletions(start, items)
	m.invalidate()
}

func (m *Model) mergeTemplateCompletions(
	t *Tab,
	req editor.CompletionRequest,
	start int,
	items []editor.Completion,
	infos map[string]completionInfo,
) (int, []editor.Completion, map[string]completionInfo) {
	liveStart, live, liveInfo := m.liveTemplateCompletionItems(t, req)
	if liveStart == start && len(live) > 0 {
		items = append(live, items...)
		if infos == nil {
			infos = map[string]completionInfo{}
		}
		for key, info := range liveInfo {
			infos[key] = info
		}
	}
	if len(items) == 0 {
		emmetStart, emmet, emmetInfo := m.emmetCompletionItems(t, req)
		if len(emmet) > 0 {
			return emmetStart, emmet, emmetInfo
		}
	}
	return start, items, infos
}

func (m *Model) showCompletionResults(
	t *Tab,
	req editor.CompletionRequest,
	start int,
	items []editor.Completion,
	infos map[string]completionInfo,
) {
	start, items, infos = m.mergeTemplateCompletions(t, req, start, items, infos)
	items = m.applyCompletionSettings(req.Prefix, items)
	if len(items) == 0 {
		t.completion.open = false
		return
	}
	m.showTrackedCompletions(t, req, start, items, infos)
}

func (m *Model) renderedCompletion(c *ui.Context, panel ui.Element, menu *editor.CompletionMenu) {
	t := m.tab()
	if t == nil {
		editor.DefaultCompletions(c, panel, menu)
		return
	}
	track := &t.completion
	track.open = true
	if t.doc != nil && t.state != nil {
		track.version = t.doc.Buffer().Version()
		track.cursor = t.state.Cursor()
	}
	items := menu.Items()
	track.itemCount = len(items)
	track.selectedIndex = menu.Selected()
	if selected := menu.Selected(); selected >= 0 && selected < len(items) {
		item := items[selected]
		track.selected = &item
		if track.lastSelected >= 0 && track.lastSelected != selected {
			track.focused = true
		}
		track.lastSelected = selected
	} else {
		track.selected = nil
		track.lastSelected = -1
	}
	m.scheduleCompletionDocumentation(t)
	if track.itemCount == 1 && track.selectedIndex == 0 {
		m.scheduleSingleCompletion(t)
	} else {
		m.stopSingleCompletion(t)
	}
	editor.DefaultCompletions(c, panel, menu)
}

func (m *Model) selectedCompletion(t *Tab) (editor.Completion, completionInfo, bool) {
	item, ok := m.currentSelectedCompletion(t)
	if !ok {
		return editor.Completion{}, completionInfo{}, false
	}
	info, tracked := t.completion.infos[completionTrackKey(item)]
	return item, info, tracked
}

func (m *Model) currentSelectedCompletion(t *Tab) (editor.Completion, bool) {
	if t == nil || t.doc == nil || t.state == nil || t.completion.selected == nil ||
		!t.completion.open {
		return editor.Completion{}, false
	}
	if t.completion.version != t.doc.Buffer().Version() || t.completion.cursor != t.state.Cursor() {
		return editor.Completion{}, false
	}
	return *t.completion.selected, true
}

func (m *Model) tabTemplateInput(t *Tab, ev ui.InputEvent) bool {
	if t == nil || t.state == nil {
		return false
	}
	if ev.Kind == ui.InputKeyDown && ev.Key == ui.KeyEscape {
		m.cancelCompletionTimers(t)
		t.completion.open = false
		return false
	}
	if ev.Kind != ui.InputKeyDown || ev.Key != ui.KeyTab || ev.Mods != 0 {
		return false
	}
	if _, ok := m.currentSelectedCompletion(t); ok {
		return m.acceptCurrentCompletion(t, "")
	}
	if !m.templates().Live.ExpandWithTab {
		return false
	}
	return m.expandTemplateAtCaret(t, false)
}

func (m *Model) expandTemplateAtCaret(t *Tab, postfix bool) bool {
	if t == nil || t.doc == nil || t.state == nil {
		return false
	}
	text := t.doc.Text()
	cursor := t.state.Cursor()
	if cursor < 0 || cursor > len(text) {
		return false
	}
	language := templateLanguage(t.doc.Path())
	if postfix {
		candidates := postfixExpansionCandidates(
			text, cursor, language, m.templates().Postfix, m.templateContext(t, ""),
		)
		if len(candidates) == 0 {
			return false
		}
		start := cursor
		for _, candidate := range candidates {
			start = min(start, candidate.From)
		}
		chosen := candidates[0]
		for _, candidate := range candidates {
			if candidate.From == start {
				chosen = candidate
				break
			}
		}
		m.applyTemplateAction(t, templateCompletionAction{
			TemplateID: chosen.TemplateID, From: chosen.From, To: cursor,
			Expansion: chosen.Text, Postfix: true,
		})
		return true
	}
	prefs := m.templates()
	if !prefs.Live.Enabled {
		return false
	}
	var matches []LiveTemplate
	for _, template := range prefs.liveTemplates() {
		if !templateContextApplies(template, language) {
			continue
		}
		start := cursor - len(template.Abbreviation)
		if start < 0 || !templatePrefixMatches(
			template.Abbreviation, text[start:cursor], prefs.Live.CaseSensitive,
		) {
			continue
		}
		if start > 0 && isTemplateIdentifierByte(text[start-1]) {
			continue
		}
		if start != cursor-len(template.Abbreviation) {
			continue
		}
		matches = append(matches, template)
	}
	if len(matches) != 1 {
		return false
	}
	template := matches[0]
	from := cursor - len(template.Abbreviation)
	m.applyTemplateAction(t, templateCompletionAction{
		TemplateID: template.ID, From: from, To: cursor,
		Expansion: expandTemplate(template.Body, m.templateContext(t, "")),
	})
	return true
}

func isTemplateIdentifierByte(value byte) bool {
	return value == '_' || value >= 'a' && value <= 'z' ||
		value >= 'A' && value <= 'Z' || value >= '0' && value <= '9'
}

func (m *Model) applyTemplateAction(t *Tab, action templateCompletionAction) {
	if t == nil || t.doc == nil || t.state == nil {
		return
	}
	text := t.doc.Text()
	if action.From < 0 || action.To < action.From || action.To > len(text) {
		return
	}
	t.state.Buffer.Edit(editor.Edit{Start: action.From, End: action.To, Text: action.Expansion.Text})
	field := action.Expansion.Caret
	if len(action.Expansion.Fields) > 0 {
		field = action.Expansion.Fields[0].Start
	}
	from := action.From + max(0, min(field, len(action.Expansion.Text)))
	end := from
	if len(action.Expansion.Fields) > 0 {
		first := action.Expansion.Fields[0]
		end = action.From + min(first.End, len(action.Expansion.Text))
	}
	t.state.Select(from, end)
	m.cancelCompletionTimers(t)
	t.completion = completionTracking{generation: t.completion.generation, consumeText: t.completion.consumeText}
	m.status = "Expanded template"
	m.invalidate()
}

func (m *Model) cancelCompletionTimers(t *Tab) {
	if t == nil {
		return
	}
	if t.completion.autoTimer != nil {
		t.completion.autoTimer.Stop()
		t.completion.autoTimer = nil
	}
	if t.completion.documentation != nil {
		t.completion.documentation.Stop()
		t.completion.documentation = nil
	}
}

func (m *Model) stopSingleCompletion(t *Tab) {
	if t != nil && t.completion.autoTimer != nil {
		t.completion.autoTimer.Stop()
		t.completion.autoTimer = nil
		t.completion.autoKey = ""
	}
}

func (m *Model) scheduleSingleCompletion(t *Tab) {
	if t == nil || t.completion.autoTimer != nil || !t.completion.explicit ||
		!m.completionAutoInsertSingle() || t.completion.selected == nil ||
		t.completion.itemCount != 1 || t.completion.selectedIndex != 0 {
		return
	}
	item := *t.completion.selected
	key := completionTrackKey(item)
	generation := t.completion.generation
	version := t.completion.version
	cursor := t.completion.cursor
	t.completion.autoKey = key
	t.completion.autoTimer = time.AfterFunc(80*time.Millisecond, func() {
		m.post(func() {
			if !m.tabOpen(t) || t.completion.generation != generation ||
				t.completion.version != version || t.completion.cursor != cursor ||
				t.completion.itemCount != 1 || t.completion.selectedIndex != 0 ||
				t.completion.autoKey != key {
				return
			}
			current, ok := m.currentSelectedCompletion(t)
			if !ok || completionTrackKey(current) != key {
				return
			}
			m.acceptCurrentCompletion(t, "")
		})
	})
}

func (m *Model) completionHasDocumentation(t *Tab, item editor.Completion) bool {
	if t == nil || strings.TrimSpace(item.Documentation) != "" {
		return strings.TrimSpace(item.Documentation) != ""
	}
	info, ok := t.completion.infos[completionTrackKey(item)]
	return ok && info.LSP != nil && info.LSP.Documentation != nil
}

func (m *Model) stopCompletionDocumentation(t *Tab) {
	if t != nil && t.completion.documentation != nil {
		t.completion.documentation.Stop()
		t.completion.documentation = nil
		t.completion.documentationKey = ""
	}
}

func (m *Model) scheduleCompletionDocumentation(t *Tab) {
	if t == nil || !m.completionShowDocumentation() || t.completion.selected == nil {
		m.stopCompletionDocumentation(t)
		return
	}
	item := *t.completion.selected
	key := completionTrackKey(item)
	if t.completion.documentation != nil && t.completion.documentationKey == key {
		return
	}
	m.stopCompletionDocumentation(t)
	if !m.completionHasDocumentation(t, item) {
		return
	}
	generation := t.completion.generation
	version := t.completion.version
	cursor := t.completion.cursor
	t.completion.documentationKey = key
	t.completion.documentation = time.AfterFunc(m.completionDocumentationDelay(), func() {
		m.post(func() {
			if !m.tabOpen(t) || t.completion.generation != generation ||
				t.completion.version != version || t.completion.cursor != cursor ||
				t.completion.documentationKey != key {
				return
			}
			current, info, ok := m.selectedCompletion(t)
			if !ok || completionTrackKey(current) != key ||
				strings.TrimSpace(current.Documentation) == "" &&
					(info.LSP == nil || info.LSP.Documentation == nil) {
				return
			}
			m.quickCompletionDocumentation(t, current, info)
		})
	})
}

func (m *Model) acceptCurrentCompletion(t *Tab, typed string) bool {
	item, info, selected := m.selectedCompletion(t)
	current, currentOK := m.currentSelectedCompletion(t)
	if !currentOK || !selected {
		// Generated completions without resolve data still have an item we can
		// accept; templates and LSP items take the tracked path above.
		if !currentOK {
			return false
		}
		item = current
	}
	parens := ""
	if info.Template == nil {
		parens = m.completionCallParens(t, item, info)
	}
	if info.Template != nil {
		action := info.Template
		if action.From < 0 || action.To < action.From || action.To > len(t.doc.Text()) ||
			item.Insert != action.Expansion.Text {
			return false
		}
		m.applyTemplateAction(t, *action)
	} else if info.Replace != nil {
		if !m.applyCompletionReplacement(t, item, *info.Replace) {
			return false
		}
	} else if !t.state.Run(editor.Tab) {
		return false
	}
	if parens != "" {
		at := t.state.Cursor()
		t.state.Buffer.Edit(editor.Edit{Start: at, End: at, Text: "()"})
		caret := at + 1
		if parens == "after" {
			caret = at + 2
		}
		t.state.Select(caret, caret)
		if typed == "(" {
			t.completion.consumeText = "("
		}
	}
	m.cancelCompletionTimers(t)
	t.completion.open = false
	t.completion.selected = nil
	m.invalidate()
	return true
}

func (m *Model) applyCompletionReplacement(
	t *Tab,
	item editor.Completion,
	replacement completionReplacement,
) bool {
	if t == nil || t.doc == nil || t.state == nil {
		return false
	}
	text := t.doc.Text()
	if replacement.From < 0 || replacement.To < replacement.From ||
		replacement.To > len(text) {
		return false
	}
	insert := item.Insert
	if insert == "" {
		insert = item.Label
	}
	t.state.Buffer.Edit(editor.Edit{
		Start: replacement.From,
		End:   replacement.To,
		Text:  insert,
	})
	caret := replacement.From + len(insert)
	t.state.Select(caret, caret)
	return true
}

var completionImportLine = regexp.MustCompile(
	`(?i)^\s*(?:import|from|use|pub\s+use|export\s*\{|#include|require)\b`,
)

func (m *Model) completionCallParens(
	t *Tab,
	item editor.Completion,
	info completionInfo,
) string {
	if t == nil || t.doc == nil || info.LSP == nil || !m.completionInsertParentheses() ||
		info.LSP.IsSnippet || info.LSP.Kind == nil ||
		strings.ContainsAny(item.Insert, "(<[{") || len(t.state.Selections()) != 1 {
		return ""
	}
	switch *info.LSP.Kind {
	case "function", "method", "constructor":
	default:
		return ""
	}
	text := t.doc.Text()
	cursor := t.state.Cursor()
	if cursor < 0 || cursor > len(text) {
		return ""
	}
	if cursor < len(text) {
		next, _ := utf8.DecodeRuneInString(text[cursor:])
		if unicode.IsLetter(next) || unicode.IsNumber(next) ||
			strings.ContainsRune("([{<_$'\"`", next) {
			return ""
		}
	}
	if t.completion.start < 0 || t.completion.start > len(text) {
		return ""
	}
	lineStart := strings.LastIndexByte(text[:t.completion.start], '\n') + 1
	if completionImportLine.MatchString(text[lineStart:t.completion.start]) {
		return ""
	}
	signature := item.Detail
	if info.LSP.LabelDetail != nil {
		signature = *info.LSP.LabelDetail
	} else if info.LSP.Detail != nil {
		signature = *info.LSP.Detail
	}
	if completionSignatureHasNoParameters(singleLine(signature)) {
		return "after"
	}
	return "inside"
}

func completionSignatureHasNoParameters(signature string) bool {
	open := strings.IndexByte(signature, '(')
	if open < 0 {
		return false
	}
	close := strings.IndexByte(signature[open+1:], ')')
	return close >= 0 && strings.TrimSpace(signature[open+1:open+1+close]) == ""
}

func (m *Model) InsertLiveTemplate() {
	t := m.tab()
	if t == nil || t.doc == nil || m.templates() == nil {
		return
	}
	m.openTemplateChooser(false)
}

func (m *Model) SurroundWithLiveTemplate() {
	t := m.tab()
	if t == nil || t.doc == nil {
		return
	}
	m.openTemplateChooser(true)
}

// SurroundWith applies the highest-priority native surround for a selection.
// The chooser remains available for choosing another form.
func (m *Model) SurroundWith() {
	m.openTemplateChooser(true)
}

func (m *Model) applySurroundTemplate(t *Tab, from, to int, template LiveTemplate) {
	if t == nil || t.doc == nil || from < 0 || to < from || to > len(t.doc.Text()) {
		return
	}
	selection := t.doc.Text()[from:to]
	expansion := expandTemplate(template.Body, m.templateContext(t, selection))
	t.state.Buffer.Edit(editor.Edit{Start: from, End: to, Text: expansion.Text})
	field := from + expansion.Caret
	if len(expansion.Fields) > 0 {
		field = from + expansion.Fields[0].Start
	}
	t.state.Select(field, field)
	m.status = "Surrounded with " + template.Name
	m.invalidate()
}

func (m *Model) UpdateTag() {
	t := m.tab()
	if t == nil || t.doc == nil {
		return
	}
	selection := t.state.Selection()
	from, to := min(selection.Anchor, selection.Head), max(selection.Anchor, selection.Head)
	text := t.doc.Text()
	if from == to {
		from, to = htmlTagRangeAt(text, t.state.Cursor())
	}
	if from == to {
		m.status = "Place the caret inside an HTML tag"
		m.invalidate()
		return
	}
	m.openTagUpdateChooser(from, to)
}

func htmlTagRangeAt(text string, offset int) (int, int) {
	if offset < 0 || offset > len(text) {
		return 0, 0
	}
	start := strings.LastIndexByte(text[:offset], '<')
	end := strings.IndexByte(text[offset:], '>')
	if start < 0 || end < 0 {
		return 0, 0
	}
	end += offset + 1
	for _, close := range []string{"</" + tagNameAt(text[start:end]) + ">", "</" + tagNameAt(text[start:end]) + " >"} {
		if next := strings.Index(text[end:], close); next >= 0 {
			return start, end + next + len(close)
		}
	}
	return start, end
}

var htmlTagNameRE = regexp.MustCompile(`^</?\s*([A-Za-z][A-Za-z0-9:-]*)`)

func tagNameAt(raw string) string {
	match := htmlTagNameRE.FindStringSubmatch(raw)
	if len(match) < 2 {
		return ""
	}
	return match[1]
}

func (m *Model) templateActions() []*Action {
	return []*Action{
		{
			ID: "code.insertLiveTemplate", Title: "Insert Live Template…", Category: "Code",
			Keywords: "snippet live template expansion",
			When:     func(m *Model) bool { return m.activeEditorAvailable() },
			Run:      func(m *Model) { m.InsertLiveTemplate() },
		},
		{
			ID: "code.surroundWith", Title: "Surround With…", Category: "Code",
			Keywords: "surround wrap selection",
			When:     func(m *Model) bool { return m.activeEditorEditable() },
			Run:      func(m *Model) { m.SurroundWith() },
		},
		{
			ID: "code.surroundWithLiveTemplate", Title: "Surround with Live Template…", Category: "Code",
			Keywords: "surround live template selection",
			When:     func(m *Model) bool { return m.activeEditorEditable() },
			Run:      func(m *Model) { m.SurroundWithLiveTemplate() },
		},
		{
			ID: "emmet.updateTag", Title: "Update Tag with Emmet", Category: "Code",
			Keywords: "emmet html tag rename attributes",
			When:     func(m *Model) bool { return m.activeEditorEditable() },
			Run:      func(m *Model) { m.UpdateTag() },
		},
	}
}

func init() {
	for _, action := range (&Model{}).templateActions() {
		_ = registerAction(action)
	}
	_ = registerAction(&Action{
		ID: "templates.newFromTemplate", Title: "New File from Template…", Category: "File",
		Keywords: "file template new class kind",
		When:     func(m *Model) bool { return m.Root() != "" && !m.showingWelcome },
		Run:      func(m *Model) { m.NewFileAtProjectSelection() },
	})
}

type templateChooserState struct {
	kind          string
	query         string
	selected      int
	anchor        ui.Rect
	t             *Tab
	from, to      int
	caseSensitive bool
	templates     []LiveTemplate
}

func (m *Model) openTemplateChooser(surround bool) {
	t := m.tab()
	if t == nil || t.doc == nil {
		return
	}
	var templates []LiveTemplate
	kind := "live"
	from, to := t.state.Cursor(), t.state.Cursor()
	if surround {
		selection := t.state.Selection()
		from, to = min(selection.Anchor, selection.Head), max(selection.Anchor, selection.Head)
		if from == to {
			m.status = "Select text to surround"
			m.invalidate()
			return
		}
		kind = "surround"
		templates = m.templates().surroundTemplates()
	} else {
		language := templateLanguage(t.doc.Path())
		for _, candidate := range m.templates().liveTemplates() {
			if templateContextApplies(candidate, language) {
				templates = append(templates, candidate)
			}
		}
	}
	sort.SliceStable(templates, func(i, j int) bool {
		if templates[i].Abbreviation == templates[j].Abbreviation {
			return templates[i].Name < templates[j].Name
		}
		return templates[i].Abbreviation < templates[j].Abbreviation
	})
	m.templateChooser = &templateChooserState{
		kind: kind, anchor: m.usagePopupAnchor, t: t, from: from, to: to,
		caseSensitive: m.templates().Live.CaseSensitive, templates: templates,
	}
	m.invalidate()
}

func (m *Model) openTagUpdateChooser(from, to int) {
	t := m.tab()
	if t == nil {
		return
	}
	m.templateChooser = &templateChooserState{
		kind: "updateTag", anchor: m.usagePopupAnchor, t: t, from: from, to: to,
	}
	m.invalidate()
}

func (m *Model) closeTemplateChooser() {
	m.templateChooser = nil
	m.invalidate()
}

func templateChooserMatches(state *templateChooserState) []LiveTemplate {
	if state == nil {
		return nil
	}
	query := strings.TrimSpace(state.query)
	if query == "" {
		return state.templates
	}
	if !state.caseSensitive {
		query = strings.ToLower(query)
	}
	out := make([]LiveTemplate, 0, len(state.templates))
	for _, template := range state.templates {
		haystack := template.Abbreviation + " " + template.Name + " " + template.Description
		if !state.caseSensitive {
			haystack = strings.ToLower(haystack)
		}
		if strings.Contains(haystack, query) {
			out = append(out, template)
		}
	}
	return out
}

func (m *Model) chooseTemplate(state *templateChooserState, template LiveTemplate) {
	if m.templateChooser != state || state.t == nil || state.t.doc == nil {
		return
	}
	m.templateChooser = nil
	if state.kind == "surround" {
		m.applySurroundTemplate(state.t, state.from, state.to, template)
		return
	}
	action := templateCompletionAction{
		TemplateID: template.ID,
		From:       state.from,
		To:         state.to,
		Expansion:  expandTemplate(template.Body, m.templateContext(state.t, "")),
	}
	m.applyTemplateAction(state.t, action)
}

func (m *Model) applyTagUpdate(state *templateChooserState, abbreviation string) {
	if m.templateChooser != state || state.t == nil || state.t.doc == nil {
		return
	}
	text := state.t.doc.Text()
	if state.from < 0 || state.to < state.from || state.to > len(text) {
		return
	}
	updated, ok := emmetUpdateTag(text[state.from:state.to], abbreviation)
	if !ok {
		m.status = "Enter an Emmet tag abbreviation"
		m.invalidate()
		return
	}
	state.t.state.Buffer.Edit(editor.Edit{Start: state.from, End: state.to, Text: updated})
	state.t.state.Select(state.from, state.from+min(len(updated), len(tagNameAt(updated))+1))
	m.templateChooser = nil
	m.status = "Updated tag"
	m.invalidate()
}

func (m *Model) templateChooserDialog(c *ui.Context, p *ui.Theme) {
	state := m.templateChooser
	if state == nil || state.t == nil || m.tab() != state.t {
		return
	}
	oi := oiPaletteOf(p)
	width, height := c.Size()
	w := min(float32(420), width-16)
	h := float32(320)
	x, y := state.anchor.X, state.anchor.Y+state.anchor.H+4
	if state.anchor.W == 0 {
		x, y = width/2-w/2, height/4
	}
	x = max(8, min(x, width-w-8))
	if y+h > height-8 {
		y = max(8, state.anchor.Y-h-4)
	}
	ui.Overlay(c, func() {
		box := ui.Column(c.Key("templates:chooser")).Role(ui.RolePopup).
			Label("Insert Live Template").Absolute().Left(x).Top(y).Width(w).Height(h).
			Background(oi.popup).Border(1, oi.borderStrong).Radius(7).
			Shadow(0, 10, 24, 0, ui.RGBA(0, 0, 0, 0.28)).Clip()
		box.Children(func() {
			ui.Row(c).Height(38).Shrink(0).Padding(0, 10).Gap(6).AlignItems(ui.Center).
				BorderWidth(0, 0, 1, 0).BorderColor(oi.border).Children(func() {
				oiIconEl(c, "general/search", 14, oi, oi.textMuted)
				input := ui.TextInput(c, &state.query).Grow(1).AutoFocus().
					Placeholder("Search templates")
				input.OnShortcut(0, ui.KeyUp, func() {
					matches := templateChooserMatches(state)
					if len(matches) > 0 {
						state.selected = (state.selected - 1 + len(matches)) % len(matches)
					}
				})
				input.OnShortcut(0, ui.KeyDown, func() {
					matches := templateChooserMatches(state)
					if len(matches) > 0 {
						state.selected = (state.selected + 1) % len(matches)
					}
				})
				input.OnShortcut(0, ui.KeyEscape, m.closeTemplateChooser)
				if state.kind == "updateTag" {
					input.Placeholder("Emmet tag, for example div.card#main")
					input.OnShortcut(0, ui.KeyEnter, func() {
						m.applyTagUpdate(state, state.query)
					})
					if input.Submitted() {
						m.applyTagUpdate(state, state.query)
					}
					return
				}
				if input.Submitted() {
					matches := templateChooserMatches(state)
					if len(matches) > 0 {
						m.chooseTemplate(state, matches[min(state.selected, len(matches)-1)])
					}
				}
			})
			if state.kind == "updateTag" {
				ui.Text(c, "Updates the selected tag and merges its classes and attributes.").
					FontSize(11).TextColor(oi.textMuted).Padding(10, 12).Wrap()
				return
			}
			matches := templateChooserMatches(state)
			if len(matches) == 0 {
				ui.Text(c, "Nothing found").FontSize(12).TextColor(oi.textFaint).
					Padding(10, 12)
				return
			}
			state.selected = min(state.selected, len(matches)-1)
			ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
				ui.Column(c).Padding(0, 4).Children(func() {
					for index, template := range matches {
						index, template := index, template
						row := ui.Row(c.Key("template:"+template.ID)).Height(42).
							Padding(0, 10).Gap(8).Radius(4).AlignItems(ui.Center).
							Cursor(ui.CursorPointer)
						if index == state.selected || row.Hovered() {
							row.Background(oi.selected)
						}
						row.Children(func() {
							ui.Text(c, template.Abbreviation).Font("monospace").
								FontSize(12).Bold().TextColor(oi.textStrong).Width(110)
							ui.Column(c).Grow(1).MinWidth(0).Children(func() {
								ui.Text(c, template.Name).FontSize(12).TextColor(oi.text).
									SingleLine().Ellipsis("…")
								if template.Description != "" {
									ui.Text(c, template.Description).FontSize(10.5).
										TextColor(oi.textMuted).SingleLine().Ellipsis("…")
								}
							})
						})
						if row.Hovered() {
							state.selected = index
						}
						if row.Clicked() {
							m.chooseTemplate(state, template)
						}
					}
				})
			})
		})
		if box.PressedOutside() {
			m.closeTemplateChooser()
		}
	})
}
