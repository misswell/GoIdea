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

const agentCompletionLimit = 12

type agentCompletionState struct {
	Open   bool
	Busy   bool
	Target string
	Kind   string
	Query  string
	From   int
	To     int
	Seq    uint64
	Index  int
	Items  []agentCompletionItem
}

type agentCompletionItem struct {
	Kind    string
	Label   string
	Detail  string
	Value   string
	Path    string
	Rel     string
	Dir     bool
	Section bool
	Score   uint32
}

type agentCompletionRef struct {
	Kind  string
	Query string
	From  int
	To    int
}

type agentPathCompletion struct {
	Name string `json:"name"`
	Dir  bool   `json:"dir"`
}

// agentCompletionAt finds the @ or / token at the rune caret. A token only
// starts at the beginning of the prompt or after whitespace.
func agentCompletionAt(text string, caret int) (agentCompletionRef, bool) {
	runes := []rune(text)
	caret = max(0, min(caret, len(runes)))
	start := caret
	for start > 0 && !isAgentPromptSpace(runes[start-1]) {
		start--
	}
	if start >= caret || (runes[start] != '@' && runes[start] != '/') {
		return agentCompletionRef{}, false
	}
	if start > 0 && !isAgentPromptSpace(runes[start-1]) {
		return agentCompletionRef{}, false
	}
	kind := "file"
	if runes[start] == '/' {
		kind = "command"
	}
	return agentCompletionRef{
		Kind: kind, Query: string(runes[start+1 : caret]), From: start, To: caret,
	}, true
}

func isAgentPromptSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}

func agentApplyCompletion(text string, completion agentCompletionRef, insert string) (string, int) {
	return agentApplyCompletionGlue(text, completion, insert, true)
}

func agentApplyCompletionGlue(text string, completion agentCompletionRef, insert string,
	appendGlue bool) (string, int) {
	runes := []rune(text)
	from := max(0, min(completion.From, len(runes)))
	to := max(from, min(completion.To, len(runes)))
	after := runes[to:]
	glue := ""
	if appendGlue {
		glue = " "
		if len(after) > 0 && after[0] == ' ' {
			glue = ""
		}
	}
	replacement := []rune(insert + glue)
	next := make([]rune, 0, len(runes)-(to-from)+len(replacement))
	next = append(next, runes[:from]...)
	next = append(next, replacement...)
	next = append(next, after...)
	return string(next), from + len([]rune(insert)) + len([]rune(glue))
}

func agentMatchCommands(commands []agentCommand, query string) []agentCompletionItem {
	query = strings.ToLower(query)
	type ranked struct {
		command agentCommand
		rank    int
	}
	matches := make([]ranked, 0, len(commands))
	for _, command := range commands {
		name := strings.ToLower(command.Name)
		rank := -1
		switch {
		case name == "":
			continue
		case strings.HasPrefix(name, query):
			rank = 0
		case strings.HasPrefix(name[strings.LastIndex(name, ":")+1:], query):
			rank = 0
		case strings.Contains(name, query):
			rank = 1
		case agentLettersInOrder(name, query):
			rank = 2
		}
		if rank >= 0 {
			matches = append(matches, ranked{command: command, rank: rank})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		leftSkill := matches[i].command.Source == "skill"
		rightSkill := matches[j].command.Source == "skill"
		if leftSkill != rightSkill {
			return !leftSkill
		}
		return matches[i].rank < matches[j].rank
	})
	items := make([]agentCompletionItem, 0, len(matches)+2)
	source := ""
	for _, match := range matches {
		nextSource := match.command.Source
		if nextSource == "" {
			nextSource = "command"
		}
		if nextSource != source {
			source = nextSource
			label := "Commands"
			if source == "skill" {
				label = "Skills"
			}
			items = append(items, agentCompletionItem{Kind: "section", Label: label, Section: true})
		}
		detail := match.command.Description
		if match.command.Hint != nil && *match.command.Hint != "" {
			detail = strings.TrimSpace(detail + " · " + *match.command.Hint)
		}
		items = append(items, agentCompletionItem{
			Kind: "command", Label: "/" + match.command.Name,
			Detail: detail, Value: match.command.Name,
		})
	}
	return items
}

func agentLettersInOrder(text, query string) bool {
	if query == "" {
		return true
	}
	queryRunes := []rune(query)
	index := 0
	for _, ch := range text {
		if ch == queryRunes[index] {
			index++
			if index == len(queryRunes) {
				return true
			}
		}
	}
	return len(queryRunes) == 0
}

// SyncAgentCompletion follows the composer text and caret. The target is
// "new" for the New Agent Task page or a task id in a conversation.
func (m *Model) SyncAgentCompletion(target, text string, caret int, root string, commands []agentCommand) {
	a := &m.agents
	reference, ok := agentCompletionAt(text, caret)
	if !ok {
		if a.completion.Open || a.completion.Busy {
			a.completion = agentCompletionState{}
			a.completionCaretSet = false
			m.invalidate()
		}
		return
	}
	current := a.completion
	if current.Target == target && current.Kind == reference.Kind &&
		current.Query == reference.Query && current.From == reference.From &&
		current.To == reference.To && (current.Open || current.Busy) {
		return
	}
	a.completion.Seq++
	seq := a.completion.Seq
	a.completion = agentCompletionState{
		Target: target, Kind: reference.Kind, Query: reference.Query,
		From: reference.From, To: reference.To, Seq: seq,
	}
	a.completionCaretSet = false
	if reference.Kind == "command" {
		a.completion.Items = agentMatchCommands(commands, reference.Query)
		a.completion.Index = firstSelectableAgentCompletion(a.completion.Items)
		a.completion.Open = len(a.completion.Items) > 0
		m.invalidate()
		return
	}
	if m.ctx == nil || (m.mgr == nil && m.invokeHook == nil) {
		m.invalidate()
		return
	}
	a.completion.Busy = true
	m.invalidate()
	go m.loadAgentFileCompletions(seq, target, root, reference)
}

func (m *Model) loadAgentFileCompletions(seq uint64, target, root string,
	reference agentCompletionRef) {
	if m.ctx == nil || (m.mgr == nil && m.invokeHook == nil) {
		return
	}
	rootAtStart, gen := m.Root(), m.agents.generation
	ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
	defer cancel()
	var items []agentCompletionItem
	var err error
	if agentPathQuery(reference.Query) {
		items, err = m.agentPathCompletions(ctx, root, reference.Query)
	} else {
		items, err = m.agentIndexedCompletions(ctx, root, reference.Query)
	}
	m.post(func() {
		if m.agents.generation != gen || m.Root() != rootAtStart ||
			m.agents.completion.Seq != seq || m.agents.completion.Target != target {
			return
		}
		m.agents.completion.Busy = false
		if err != nil {
			m.agents.completion.Items = nil
			m.agents.completion.Open = false
			m.agents.actionErr = "Could not complete the file reference: " + err.Error()
		} else {
			m.agents.completion.Items = limitAgentCompletionItems(items)
			m.agents.completion.Index = firstSelectableAgentCompletion(
				m.agents.completion.Items)
			m.agents.completion.Open = len(items) > 0
		}
		m.invalidate()
	})
}

func agentPathQuery(query string) bool {
	return query == "." || query == ".." || strings.HasPrefix(query, "/") ||
		strings.HasPrefix(query, "./") || strings.HasPrefix(query, "../")
}

func (m *Model) agentPathCompletions(ctx context.Context, root, query string) ([]agentCompletionItem, error) {
	typed := query
	if typed == "." || typed == ".." ||
		strings.HasSuffix(typed, "/.") || strings.HasSuffix(typed, "/..") {
		typed += "/"
	}
	from := filepath.Join(root, "_")
	if strings.HasPrefix(typed, "/") {
		from = string(filepath.Separator) + "_"
	}
	raw, err := m.invoke(ctx, "path_completions", map[string]any{
		"from": from, "prefix": typed,
	})
	if err != nil {
		return nil, err
	}
	var completions []agentPathCompletion
	if err := json.Unmarshal(raw, &completions); err != nil {
		return nil, err
	}
	dirPart := typed
	if index := strings.LastIndexAny(dirPart, `/\`); index >= 0 {
		dirPart = dirPart[:index+1]
	} else {
		dirPart = ""
	}
	last := typed[len(dirPart):]
	items := make([]agentCompletionItem, 0, len(completions))
	for _, completion := range completions {
		if !strings.HasPrefix(strings.ToLower(completion.Name), strings.ToLower(last)) {
			continue
		}
		rel := dirPart + completion.Name
		path := agentResolvePromptPath(root, rel)
		label := "@" + rel
		if completion.Dir {
			label += "/"
			rel += "/"
		}
		items = append(items, agentCompletionItem{
			Kind: "file", Label: label, Detail: path, Path: path,
			Rel: rel, Dir: completion.Dir,
		})
	}
	return items, nil
}

func (m *Model) agentIndexedCompletions(ctx context.Context, root, query string) ([]agentCompletionItem, error) {
	rawFiles, err := m.invoke(ctx, "find_files", map[string]any{
		"query": query, "limit": agentCompletionLimit,
	})
	if err != nil {
		return nil, err
	}
	var files []openidea.FileMatch
	if err := json.Unmarshal(rawFiles, &files); err != nil {
		return nil, err
	}
	rawDirs, err := m.invoke(ctx, "find_dirs", map[string]any{
		"query": query, "limit": agentCompletionLimit,
	})
	if err != nil {
		return nil, err
	}
	var dirs []openidea.FileMatch
	if err := json.Unmarshal(rawDirs, &dirs); err != nil {
		return nil, err
	}
	if len(files)+len(dirs) == 0 {
		return m.agentPathCompletions(ctx, root, "")
	}
	items := make([]agentCompletionItem, 0, len(files)+len(dirs))
	for _, file := range files {
		path := agentResolvePromptPath(root, file.RelPath)
		items = append(items, agentCompletionItem{
			Kind: "file", Label: "@" + file.RelPath, Detail: file.RelPath,
			Path: path, Rel: file.RelPath, Score: file.Score,
		})
	}
	for _, dir := range dirs {
		rel := strings.TrimSuffix(dir.RelPath, "/")
		path := agentResolvePromptPath(root, rel)
		items = append(items, agentCompletionItem{
			Kind: "file", Label: "@" + rel + "/", Detail: rel + "/",
			Path: path, Rel: rel + "/", Dir: true, Score: dir.Score,
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Score != items[j].Score {
			return items[i].Score > items[j].Score
		}
		if items[i].Dir != items[j].Dir {
			return !items[i].Dir
		}
		return items[i].Rel < items[j].Rel
	})
	return items, nil
}

func limitAgentCompletionItems(items []agentCompletionItem) []agentCompletionItem {
	selectable := 0
	kept := 0
	for _, item := range items {
		if kept >= agentCompletionLimit {
			break
		}
		if !item.Section {
			selectable++
		}
		kept++
	}
	if selectable == 0 {
		return nil
	}
	return append([]agentCompletionItem(nil), items[:kept]...)
}

func agentResolvePromptPath(root, rel string) string {
	rel = filepath.FromSlash(rel)
	if filepath.IsAbs(rel) {
		return filepath.Clean(rel)
	}
	return filepath.Clean(filepath.Join(root, rel))
}

// agentPromptFiles is toPrompt's file list: attached files from outside the
// project, plus project files whose @mention remains in the text.
func agentPromptFiles(text string, attached []string, root string) []string {
	out := make([]string, 0, len(attached)+2)
	seen := make(map[string]bool, len(attached)+2)
	for _, path := range attached {
		path = filepath.Clean(path)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		out = append(out, path)
	}
	for _, field := range strings.Fields(text) {
		if !strings.HasPrefix(field, "@") || len(field) == 1 {
			continue
		}
		mention := strings.TrimPrefix(field, "@")
		if at := strings.IndexRune(mention, '@'); at >= 0 {
			mention = mention[:at]
		}
		mention = agentMentionWithoutLines(mention)
		if mention == "" {
			continue
		}
		path := agentResolvePromptPath(root, mention)
		if seen[path] {
			continue
		}
		seen[path] = true
		out = append(out, path)
	}
	return out
}

func agentMentionWithoutLines(mention string) string {
	at := strings.LastIndex(mention, ":")
	if at <= 0 || at == len(mention)-1 {
		return mention
	}
	suffix := mention[at+1:]
	from, to, _ := strings.Cut(suffix, "-")
	if !agentDecimal(from) || to != "" && !agentDecimal(to) {
		return mention
	}
	return mention[:at]
}

func agentDecimal(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// agentAddPathReferences follows Composer's addPaths: files inside a project
// become @mentions, while outside paths become attachment chips when the
// caller picked or dropped them from the OS.
func agentAddPathReferences(text string, attached, paths []string, root string,
	attachOutside bool) (string, []string, int) {
	files := append([]string(nil), attached...)
	seen := make(map[string]bool, len(files))
	for _, path := range files {
		seen[filepath.Clean(path)] = true
	}
	existing := make(map[string]bool)
	for _, path := range agentPromptFiles(text, nil, root) {
		existing[filepath.Clean(path)] = true
	}
	added := 0
	for _, raw := range paths {
		path := filepath.Clean(strings.TrimSpace(raw))
		if path == "" || seen[path] || existing[path] {
			continue
		}
		rel, err := filepath.Rel(root, path)
		inside := err == nil && rel != ".." &&
			!strings.HasPrefix(rel, ".."+string(filepath.Separator))
		if inside {
			rel = filepath.ToSlash(rel)
			if rel == "." {
				rel = "."
			}
			glue := ""
			if text != "" && !strings.HasSuffix(text, " ") &&
				!strings.HasSuffix(text, "\n") && !strings.HasSuffix(text, "\t") {
				glue = " "
			}
			text += glue + "@" + rel + " "
			existing[path] = true
			added++
			continue
		}
		if !attachOutside {
			continue
		}
		files = append(files, path)
		seen[path] = true
		existing[path] = true
		added++
	}
	return text, files, added
}

func (m *Model) HandleAgentComposerInput(ev ui.InputEvent) bool {
	if ev.Kind == ui.InputText {
		m.agents.historyIndex = -1
		m.agents.historyStash = ""
		return false
	}
	if ev.Kind != ui.InputKeyDown {
		return false
	}
	if m.agents.completion.Open {
		st := &m.agents.completion
		switch ev.Key {
		case ui.KeyDown:
			if next, ok := agentNextCompletionItem(st.Items, st.Index, 1); ok {
				st.Index = next
				m.invalidate()
			}
			return true
		case ui.KeyUp:
			if next, ok := agentNextCompletionItem(st.Items, st.Index, -1); ok {
				st.Index = next
				m.invalidate()
			}
			return true
		case ui.KeyEnter:
			return m.AcceptAgentCompletion(firstSelectableAgentCompletion(st.Items), false)
		case ui.KeyTab:
			return m.AcceptAgentCompletion(firstSelectableAgentCompletion(st.Items), true)
		case ui.KeyEscape:
			st.Open = false
			st.Items = nil
			st.Busy = false
			m.invalidate()
			return true
		}
	}
	if ev.Key == ui.KeyTab && ev.Mods&ui.Shift != 0 {
		return m.cycleAgentComposerMode()
	}
	if ev.Key == ui.KeyUp || ev.Key == ui.KeyDown {
		if m.recallAgentComposerHistory(ev.Key == ui.KeyUp) {
			return true
		}
	}
	if ev.Key == ui.KeyEnter {
		now, ok := agentSendIntent(ev.Mods, m.agentPrefs())
		if !ok {
			return false
		}
		if m.agentNewTaskFocused() {
			m.createAgentTask()
		} else if now {
			task := m.agents.tasks[m.agents.selected]
			steer := agentCanSteer(task, m.agents.composerImages)
			if m.agentPrefs().whileRunning() == "steer" && !steer {
				m.submitAgentPrompt()
			} else {
				m.submitAgentPromptNow()
			}
		} else {
			m.submitAgentPrompt()
		}
		m.agents.historyIndex = -1
		m.agents.historyStash = ""
		return true
	}
	return false
}

func agentCanSteer(task agentTask, images []agentImage) bool {
	if !task.Capabilities.Steer || task.Status != "running" {
		return false
	}
	return len(images) == 0 || task.Capabilities.Images
}

func firstSelectableAgentCompletion(items []agentCompletionItem) int {
	for index, item := range items {
		if !item.Section {
			return index
		}
	}
	return -1
}

// agentSendIntent follows OpenIDEA's sendKeyAction.
func agentSendIntent(mods ui.Modifiers, prefs AgentPrefs) (now, ok bool) {
	if mods&ui.Alt != 0 {
		return false, false
	}
	mod := mods&(ui.Cmd|ui.Ctrl) != 0
	other := false
	switch prefs.sendWith() {
	case "modEnter":
		if !mod {
			return false, false
		}
		other = mods&ui.Shift != 0
	default:
		if mods&ui.Shift != 0 {
			return false, false
		}
		other = mod
	}
	return (prefs.whileRunning() == "steer") != other, true
}

func (m *Model) recallAgentComposerHistory(up bool) bool {
	a := &m.agents
	if m.agentNewTaskFocused() || !a.conversation || a.selected == "" {
		return false
	}
	history := agentComposerHistory(a.entries[a.selected])
	if len(history) == 0 {
		return false
	}
	index := a.historyIndex
	if index < -1 || index >= len(history) {
		index = -1
	}
	fresh := a.composer == ""
	if index >= 0 && index < len(history) {
		fresh = a.composer == history[index]
	}
	if !fresh {
		return false
	}
	direction := 1
	if !up {
		direction = -1
	}
	if up && !agentHistoryAtEdge(a.composer, a.composerCaret, -1) ||
		!up && !agentHistoryAtEdge(a.composer, a.composerCaret, 1) {
		return false
	}
	next := index + direction
	if next < -1 || next >= len(history) {
		return false
	}
	if index == -1 {
		a.historyStash = a.composer
	}
	a.historyIndex = next
	value := a.historyStash
	if next >= 0 {
		value = history[next]
	}
	a.composer = value
	a.completion = agentCompletionState{}
	a.completionCaret = len([]rune(value))
	a.completionCaretSet = true
	m.invalidate()
	return true
}

func agentComposerHistory(entries []agentEntry) []string {
	out := make([]string, 0)
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		if entry.Type == "user" && entry.Text != "" {
			out = append(out, entry.Text)
		}
	}
	return out
}

func agentHistoryAtEdge(text string, caret, direction int) bool {
	runes := []rune(text)
	caret = max(0, min(caret, len(runes)))
	if direction < 0 {
		return !strings.ContainsRune(string(runes[:caret]), '\n')
	}
	return !strings.ContainsRune(string(runes[caret:]), '\n')
}

func (m *Model) cycleAgentComposerMode() bool {
	a := &m.agents
	if m.agentNewTaskFocused() {
		for _, option := range m.newAgentConfigOptions() {
			if option.Category == nil || *option.Category != "mode" {
				continue
			}
			next := agentNextConfigValue(option, a.newConfig[option.ID])
			if next == "" {
				return false
			}
			a.newConfig[option.ID] = next
			m.invalidate()
			return true
		}
		return false
	}
	task, ok := a.tasks[a.selected]
	if !ok || task.Status == "archived" {
		return false
	}
	for _, option := range task.Config {
		if option.Category == nil || *option.Category != "mode" {
			continue
		}
		current := ""
		if option.Current != nil {
			current = *option.Current
		}
		next := agentNextConfigValue(option, current)
		if next == "" {
			return false
		}
		m.setAgentConfig(task.ID, option.ID, next)
		return true
	}
	return false
}

func agentNextConfigValue(option agentConfigOption, current string) string {
	if len(option.Values) == 0 {
		return ""
	}
	index := -1
	for i, value := range option.Values {
		if value.Value == current {
			index = i
			break
		}
	}
	return option.Values[(index+1)%len(option.Values)].Value
}

func agentNextCompletionItem(items []agentCompletionItem, current, direction int) (int, bool) {
	if len(items) == 0 {
		return 0, false
	}
	current = max(0, min(current, len(items)-1))
	for step := 1; step <= len(items); step++ {
		next := (current + direction*step + len(items)) % len(items)
		if !items[next].Section {
			return next, true
		}
	}
	return current, false
}

// AcceptAgentCompletion inserts the selected file or command in place.
func (m *Model) AcceptAgentCompletion(index int, descend bool) bool {
	a := &m.agents
	st := &a.completion
	if !st.Open || index < 0 || index >= len(st.Items) || st.Items[index].Section {
		return false
	}
	item := st.Items[index]
	text := a.composer
	if st.Target == "new" {
		text = a.newPrompt
	}
	insert := item.Label
	if item.Kind == "command" {
		insert = "/" + item.Value
	}
	next, caret := agentApplyCompletionGlue(text, agentCompletionRef{
		Kind: st.Kind, Query: st.Query, From: st.From, To: st.To,
	}, insert, !(item.Dir && descend))
	if st.Target == "new" {
		a.newPrompt = next
	} else {
		a.composer = next
	}
	a.completionCaret = caret
	a.completionCaretSet = true
	keepOpen := item.Dir && descend
	st.Open = keepOpen
	st.Busy = false
	st.Items = nil
	st.Index = 0
	m.invalidate()
	return true
}

func (m *Model) agentCompletionsView(c *ui.Context, p *ui.Theme, oi oiColors) {
	st := &m.agents.completion
	if !st.Open && !st.Busy {
		return
	}
	ui.Column(c).Key("agents-completions").Shrink(0).MaxHeight(210).
		Radius(5).BorderWidth(1, 1, 1, 1).BorderColor(oi.border).
		Background(oi.island).Padding(3, 4).Children(func() {
		if st.Busy && len(st.Items) == 0 {
			ui.Text(c, "Searching files…").FontSize(11).TextColor(oi.textMuted).
				Padding(4, 5).SingleLine()
			return
		}
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).Gap(1).Children(func() {
				for index, item := range st.Items {
					index, item := index, item
					row := ui.Row(c.Key(fmt.Sprintf("agents-completion:%d:%s", index, item.Label))).
						MinHeight(26).Padding(2, 5).Gap(7).AlignItems(ui.Center).Radius(4)
					if item.Section {
						ui.Text(c, item.Label).FontSize(10).Bold().TextColor(oi.textMuted).
							SingleLine()
						continue
					}
					if index == st.Index || row.Hovered() {
						row.Background(oi.hoverTint)
					}
					row.Cursor(ui.CursorPointer)
					row.Children(func() {
						icon := "@"
						if item.Kind == "command" {
							icon = "›"
						}
						ui.Text(c, icon).Width(16).FontSize(12).TextColor(oi.accent)
						ui.Text(c, item.Label).FontSize(12).TextColor(oi.text).
							SingleLine().Ellipsis("…")
						if item.Detail != "" {
							ui.Text(c, item.Detail).Grow(1).MinWidth(0).FontSize(10).
								TextColor(oi.textMuted).SingleLine().Ellipsis("…").
								Tooltip(item.Detail)
						} else {
							ui.Spacer(c)
						}
					})
					if row.Clicked() {
						m.AcceptAgentCompletion(index, false)
					}
				}
			})
		})
	})
}

// RefreshNewAgentCommands reads `/` commands and skills from the selected
// provider for the New Agent Task composer.
func (m *Model) RefreshNewAgentCommands(force bool) {
	a := &m.agents
	provider := a.newProvider
	if !m.agentNewTaskVisible() || provider == "" || m.ctx == nil ||
		(m.mgr == nil && m.invokeHook == nil) {
		return
	}
	if !force && (a.newCommandsLoading ||
		(a.newCommandsProvider == provider && a.newCommands != nil)) {
		return
	}
	root, gen := m.Root(), a.generation
	a.newCommandsSeq++
	seq := a.newCommandsSeq
	a.newCommandsLoading = true
	a.newCommandsProvider = provider
	a.newCommandsErr = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "agent_commands", map[string]any{
			"provider": provider, "task": nil,
		})
		var commands []agentCommand
		if err == nil {
			err = json.Unmarshal(raw, &commands)
		}
		m.post(func() {
			if a.generation != gen || m.Root() != root ||
				!m.agentNewTaskVisible() || a.newProvider != provider ||
				a.newCommandsSeq != seq {
				return
			}
			a.newCommandsLoading = false
			if err != nil {
				a.newCommandsErr = err.Error()
			} else {
				a.newCommands = commands
			}
			m.invalidate()
		})
	}()
}

// RefreshTaskAgentCommands reads the slash commands and skills available to
// an existing task. The task's directory can differ from the project root for
// a worktree, so the backend resolves them with the task id.
func (m *Model) refreshTaskAgentCommands(taskID string, force bool) {
	a := &m.agents
	task, ok := a.tasks[taskID]
	if !ok || task.Provider == "" || m.ctx == nil ||
		(m.mgr == nil && m.invokeHook == nil) {
		return
	}
	if a.commandsBusy == nil {
		a.commandsBusy = make(map[string]bool)
	}
	if a.commandsLoaded == nil {
		a.commandsLoaded = make(map[string]bool)
	}
	if a.commandsErr == nil {
		a.commandsErr = make(map[string]string)
	}
	if a.commandsSeq == nil {
		a.commandsSeq = make(map[string]uint64)
	}
	if !force && (a.commandsBusy[taskID] || a.commandsLoaded[taskID] ||
		task.Commands != nil || a.commandsErr[taskID] != "") {
		return
	}
	root, gen := m.Root(), a.generation
	a.commandsSeq[taskID]++
	seq := a.commandsSeq[taskID]
	a.commandsBusy[taskID] = true
	delete(a.commandsErr, taskID)
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "agent_commands", map[string]any{
			"provider": task.Provider, "task": taskID,
		})
		var commands []agentCommand
		if err == nil {
			err = json.Unmarshal(raw, &commands)
		}
		m.post(func() {
			if a.generation != gen || m.Root() != root ||
				a.commandsSeq[taskID] != seq {
				return
			}
			if _, exists := a.tasks[taskID]; !exists {
				return
			}
			a.commandsBusy[taskID] = false
			if err != nil {
				a.commandsErr[taskID] = err.Error()
			} else {
				current := a.tasks[taskID]
				current.Commands = commands
				a.tasks[taskID] = current
				a.commandsLoaded[taskID] = true
				delete(a.commandsErr, taskID)
			}
			m.invalidate()
		})
	}()
}

func (m *Model) resetNewAgentCommands() {
	a := &m.agents
	a.newCommandsSeq++
	a.newCommands = nil
	a.newCommandsLoading = false
	a.newCommandsProvider = ""
	a.newCommandsErr = ""
}
