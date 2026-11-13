package app

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui/editor"

	"goidea/internal/openidea"
)

// lspTimeout bounds one language-server request; JDT LS may take seconds on a
// cold project.
const lspTimeout = 15 * time.Second

// lspBackoff is how long completion stops asking after the backend reported
// the language server missing or not ready yet.
const lspBackoff = 5 * time.Second

// requestCompletion asks the language server for completions at the caret.
// The editor filters the list against what is typed meanwhile, so a reply that
// arrives a keystroke late still helps.
func (m *Model) requestCompletion(t *Tab, req editor.CompletionRequest) {
	if !m.tabOpen(t) || time.Now().Before(t.compOff) {
		return
	}
	doc := t.doc
	docID := doc.ID()
	pos := doc.UTF16Pos(req.Offset)
	start := max(0, req.Offset-len(req.Prefix))
	var trigger any
	if req.Offset > 0 {
		if last := doc.Buffer().Snapshot().Slice(req.Offset-1, req.Offset); strings.ContainsAny(last, ".:>") {
			trigger = last
		}
	}
	// A newer request supersedes the one in flight, as the typing that made it
	// did; without this a slow JDT LS queues a request per keystroke.
	if t.compReq != 0 {
		m.cancelRequest(t.compReq)
	}
	id := m.nextRequest()
	t.compReq = id
	t.compGen++
	gen := t.compGen
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, lspTimeout)
		defer cancel()
		raw, err := m.mgr.Invoke(ctx, Window, "lsp_completion", map[string]any{
			"request": id, "doc": docID, "pos": lspPos(pos), "trigger": trigger,
		})
		var list openidea.CompletionList
		if err == nil {
			err = json.Unmarshal(raw, &list)
		}
		items := lspCompletions(list.Items)
		m.post(func() {
			if t.compReq == id {
				t.compReq = 0
			}
			if err != nil {
				if k := openidea.ErrorKind(err); k == "unavailable" || k == "notFound" {
					t.compOff = time.Now().Add(lspBackoff)
				}
				return
			}
			if t.compGen != gen || !m.tabOpen(t) || len(items) == 0 {
				return
			}
			t.state.ShowCompletions(start, items)
			m.invalidate()
		})
	}()
}

// requestHover asks what the text at the caret is. The popover only shows for
// the text and version the server answered about.
func (m *Model) requestHover(t *Tab, req editor.HoverRequest) {
	if !m.tabOpen(t) {
		return
	}
	doc := t.doc
	docID := doc.ID()
	pos := doc.UTF16Pos(req.Offset)
	id := m.nextRequest()
	t.hoverGen++
	gen := t.hoverGen
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, lspTimeout)
		defer cancel()
		raw, err := m.mgr.Invoke(ctx, Window, "lsp_hover", map[string]any{
			"request": id, "doc": docID, "pos": lspPos(pos),
		})
		if err != nil {
			return
		}
		var hover *openidea.Hover
		if err := json.Unmarshal(raw, &hover); err != nil || hover == nil {
			return
		}
		text := hoverText(hover.Contents)
		if text == "" {
			return
		}
		m.post(func() {
			if t.hoverGen != gen || !m.tabOpen(t) || t.doc.ID() != docID ||
				t.doc.Buffer().Version() != req.Version || t.state.Cursor() != req.Offset {
				return
			}
			start, end := req.Offset, req.Offset
			if hover.Range != nil {
				start, end = t.doc.ByteRange(*hover.Range)
			}
			t.state.ShowHover(start, end, text, nil)
			m.invalidate()
		})
	}()
}

// gotoDefinition jumps to the first definition of the symbol at off. Results
// inside a jar or jrt have no file to open and say so instead of failing to
// open a URI as a path.
func (m *Model) gotoDefinition(t *Tab, off int) {
	if !m.tabOpen(t) {
		return
	}
	docID := t.doc.ID()
	pos := t.doc.UTF16Pos(off)
	t.defGen++
	gen := t.defGen
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, lspTimeout)
		defer cancel()
		raw, err := m.mgr.Invoke(ctx, Window, "lsp_goto", map[string]any{
			"doc": docID, "pos": lspPos(pos), "kind": "definition",
		})
		var locs []openidea.Location
		if err == nil {
			err = json.Unmarshal(raw, &locs)
		}
		m.post(func() {
			if t.defGen != gen || !m.tabOpen(t) {
				return
			}
			switch {
			case err != nil:
				m.status = "跳转失败：" + err.Error()
			case len(locs) == 0:
				m.status = "未找到定义"
			case strings.Contains(locs[0].Path, "://"):
				m.status = "定义在外部依赖中，无法作为文件打开"
			default:
				loc := locs[0]
				m.revealAt(loc.Path, int(loc.Range.Start.Line), int(loc.Range.Start.Col))
				return
			}
			m.invalidate()
		})
	}()
}

// cancelRequest cancels a backend request whose answer is no longer wanted.
func (m *Model) cancelRequest(id uint32) {
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 2*time.Second)
		defer cancel()
		_, _ = m.mgr.Invoke(ctx, Window, "cancel_request", map[string]any{"id": id})
	}()
}

// tabOpen reports whether t is still one of the model's tabs, on the main
// thread; a reply for a closed tab is dropped.
func (m *Model) tabOpen(t *Tab) bool {
	for _, x := range m.tabs {
		if x == t {
			return true
		}
	}
	return false
}

// lspPos is a protocol position, as the backend decodes it.
func lspPos(p openidea.OutlinePos) map[string]any {
	return map[string]any{"line": p.Line, "col": p.Col}
}

// lspCompletions converts language-server items to the editor's, stripping
// snippet syntax the editor cannot expand.
func lspCompletions(items []openidea.CompletionItem) []editor.Completion {
	out := make([]editor.Completion, 0, len(items))
	for _, it := range items {
		if it.Label == "" {
			continue
		}
		c := editor.Completion{
			Label: it.Label,
			Kind:  lspCompletionKind(it.Kind),
		}
		if it.FilterText != nil {
			c.FilterText = *it.FilterText
		}
		// The label detail is the parameter list JDT puts beside the label;
		// the detail alone is often the whole signature, which would repeat it.
		if it.LabelDetail != nil {
			c.Detail = singleLine(*it.LabelDetail)
		} else if it.Detail != nil {
			c.Detail = singleLine(*it.Detail)
		}
		c.Insert = it.InsertText
		if it.IsSnippet {
			c.Insert = stripSnippet(c.Insert)
		}
		if c.Insert == "" {
			c.Insert = c.Label
		}
		if it.Documentation != nil {
			c.Documentation = hoverText(*it.Documentation)
		}
		out = append(out, c)
	}
	return out
}

// lspCompletionKind maps the protocol's completion kind to the editor's icon.
func lspCompletionKind(kind *string) editor.CompletionKind {
	if kind == nil {
		return editor.CompletionText
	}
	switch *kind {
	case "method":
		return editor.CompletionMethod
	case "function", "constructor":
		return editor.CompletionFunction
	case "field", "property", "enumMember":
		return editor.CompletionField
	case "variable", "localVariable":
		return editor.CompletionVariable
	case "class", "interface", "enum", "struct", "typeParameter", "unit":
		return editor.CompletionType
	case "module", "file", "folder", "reference":
		return editor.CompletionModule
	case "constant", "value":
		return editor.CompletionConstant
	case "keyword", "operator":
		return editor.CompletionKeyword
	case "snippet":
		return editor.CompletionSnippet
	}
	return editor.CompletionText
}

// hoverText is markup as the editor's hover shows it: its text in the editor's
// font, with the code fence and emphasis markers taken out.
func hoverText(m openidea.Markup) string {
	if m.Kind != "markdown" {
		return strings.TrimSpace(m.Value)
	}
	var lines []string
	inFence := false
	for _, line := range strings.Split(m.Value, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "```") {
			inFence = !inFence
			continue
		}
		if !inFence && strings.HasPrefix(t, "#") {
			t = strings.TrimSpace(strings.TrimLeft(t, "#"))
		}
		if !inFence && strings.HasPrefix(t, "> ") {
			t = strings.TrimSpace(t[2:])
		}
		lines = append(lines, t)
	}
	text := strings.Join(lines, "\n")
	text = strings.ReplaceAll(text, "`", "")
	text = strings.ReplaceAll(text, "**", "")
	text = strings.ReplaceAll(text, "__", "")
	// The fences and the paragraphs around them leave runs of blank lines.
	for strings.Contains(text, "\n\n\n") {
		text = strings.ReplaceAll(text, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(text)
}

// singleLine keeps a signature's first line for the completion list.
func singleLine(s string) string {
	first, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(first)
}

// stripSnippet turns LSP snippet syntax into the literal text it would insert
// expanded without placeholders, which the editor, without snippet support,
// can type as is: `${1:name}` and `${1|a,b|}` become their first text, `$1`
// repeats the first text of step 1, and steps left empty vanish.
func stripSnippet(s string) string {
	var b strings.Builder
	stops := map[int]string{}
	stripSnippetInto(&b, s, stops)
	return b.String()
}

func stripSnippetInto(b *strings.Builder, s string, stops map[int]string) {
	for i := 0; i < len(s); {
		c := s[i]
		if c == '\\' && i+1 < len(s) && (s[i+1] == '$' || s[i+1] == '}' || s[i+1] == '\\' || s[i+1] == ',') {
			b.WriteByte(s[i+1])
			i += 2
			continue
		}
		if c != '$' || i+1 == len(s) {
			b.WriteByte(c)
			i++
			continue
		}
		if s[i+1] == '{' {
			depth, j := 0, i+1
			for ; j < len(s); j++ {
				switch s[j] {
				case '{':
					depth++
				case '}':
					depth--
				}
				if depth == 0 {
					break
				}
			}
			if j == len(s) {
				b.WriteString(s[i:])
				return
			}
			inner := s[i+2 : j]
			k := 0
			for k < len(inner) && inner[k] >= '0' && inner[k] <= '9' {
				k++
			}
			step, _ := strconv.Atoi(inner[:k])
			var text string
			switch {
			case strings.HasPrefix(inner[k:], ":"):
				var d strings.Builder
				stripSnippetInto(&d, inner[k+1:], stops)
				text = d.String()
			case strings.HasPrefix(inner[k:], "|"):
				text = snippetChoice(inner[k+1:])
			}
			if step != 0 {
				stops[step] = text
			}
			b.WriteString(text)
			i = j + 1
			continue
		}
		if s[i+1] >= '0' && s[i+1] <= '9' {
			j := i + 1
			for j < len(s) && s[j] >= '0' && s[j] <= '9' {
				j++
			}
			if step, err := strconv.Atoi(s[i+1 : j]); err == nil {
				b.WriteString(stops[step])
			}
			i = j
			continue
		}
		b.WriteByte(c)
		i++
	}
}

// snippetChoice is the first alternative of a `${1|a,b|}` step.
func snippetChoice(s string) string {
	s = strings.TrimSuffix(s, "|")
	first := s
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' {
			i++
			continue
		}
		if s[i] == ',' {
			first = s[:i]
			break
		}
	}
	return strings.ReplaceAll(first, "\\,", ",")
}

// SmokeLSP runs real language-server requests against an open Java file and
// reports what came back: the offscreen check of completion, hover and
// go-to-definition.
func (m *Model) SmokeLSP(ctx context.Context) (map[string]any, error) {
	out := map[string]any{}
	root := m.Root()
	if root == "" {
		return out, errors.New("没有打开项目")
	}
	// The language server executes JDT LS, which restricted mode forbids; the
	// smoke is an explicit check of a project the developer chose to run.
	if _, err := m.mgr.Invoke(ctx, Window, "project_trust_set", map[string]any{
		"path": root, "trusted": true, "folder": true,
	}); err != nil {
		return out, err
	}
	// A document opened before the grant has no server attached; re-opening it
	// is what the product does in TrustProject.
	if err := m.reg.ReopenAll(ctx); err != nil {
		out["reopenError"] = err.Error()
	}
	need := false
	mygo.RunOnMain(func() { need = m.tab() == nil })
	if need {
		candidate := findSmokeFile(root)
		if !strings.HasSuffix(candidate, ".java") {
			return out, errors.New("项目里没有 Java 文件可做语言服务检查")
		}
		mygo.RunOnMain(func() { m.openFile(candidate) })
		if !m.waitDebug(ctx, func(*DebugPane) bool { return m.tab() != nil }, 60*time.Second) {
			return out, errors.New("打开 " + candidate + " 超时")
		}
	}
	var t *Tab
	mygo.RunOnMain(func() { t = m.tab() })
	if t == nil {
		return out, errors.New("没有可检查的文件")
	}
	if !strings.HasSuffix(t.doc.Path(), ".java") {
		return out, errors.New("当前文件不是 Java 文件，JDT 语言服务不处理它")
	}
	out["path"] = t.doc.Path()
	text := t.doc.Text()
	compOff, symbolOff := smokeLSPOffsets(text)
	pos := t.doc.UTF16Pos(compOff)
	symbol := t.doc.UTF16Pos(symbolOff)
	out["pos"] = map[string]any{"line": pos.Line, "col": pos.Col}

	// A missing or misconfigured JDT LS never answers: say so at once instead
	// of asking for three minutes.
	servers := m.smokeLSPServers(ctx)
	out["servers"] = servers
	if !lspHasJavaServer(servers) {
		out["completionError"] = "没有配置 Java 语言服务器（jdtls）"
		return out, nil
	}
	if lspJavaStalled(servers) {
		out["completionError"] = "Java 语言服务器无法启动：" + lspJavaStates(servers)
		return out, nil
	}

	// JDT LS indexes the project on its first start; ask until it answers.
	deadline := time.Now().Add(3 * time.Minute)
	var completionErr string
	var items int
	var first string
	for {
		var list openidea.CompletionList
		if err := m.smokeLSPCall(ctx, "lsp_completion", map[string]any{
			"request": m.nextRequest(), "doc": t.doc.ID(), "pos": lspPos(pos), "trigger": nil,
		}, &list); err != nil {
			completionErr = err.Error()
		} else if len(list.Items) > 0 {
			items, first = len(list.Items), list.Items[0].Label
		}
		servers = m.smokeLSPServers(ctx)
		if lspJavaStalled(servers) {
			out["servers"] = servers
			out["completionError"] = "Java 语言服务器无法启动：" + lspJavaStates(servers)
			return out, nil
		}
		if items > 0 || time.Now().After(deadline) {
			out["servers"] = servers
			if items > 0 {
				out["completionItems"] = items
				out["completionFirst"] = first
			} else {
				out["completionError"] = completionErr
				out["timeout"] = true
			}
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}

	var hover *openidea.Hover
	if err := m.smokeLSPCall(ctx, "lsp_hover", map[string]any{
		"request": m.nextRequest(), "doc": t.doc.ID(), "pos": lspPos(symbol),
	}, &hover); err != nil {
		out["hoverError"] = err.Error()
	} else if hover != nil {
		out["hoverText"] = hoverText(hover.Contents)
	}

	var locs []openidea.Location
	if err := m.smokeLSPCall(ctx, "lsp_goto", map[string]any{
		"doc": t.doc.ID(), "pos": lspPos(symbol), "kind": "definition",
	}, &locs); err != nil {
		out["definitionError"] = err.Error()
	} else if len(locs) > 0 {
		out["definitionPath"] = locs[0].Path
		out["definitionLine"] = locs[0].Range.Start.Line
	}

	// What the server finds wrong with the text must reach the editor as a
	// squiggle. Breaking the text in memory is not saved.
	out["diagnostics"] = m.smokeDiagnostics(ctx, t)

	// Reformatting goes the other way: the server's edits reach the buffer.
	out["format"] = m.smokeFormat(ctx, t)

	// A rename goes through the backend's workspace edit, which reaches open
	// documents as backend changes.
	out["rename"] = m.smokeRename(ctx, t)
	return out, nil
}

// smokeFormat adds a misformatted snippet to the tab's text without saving it,
// asks the model to reformat, and waits for the formatted text.
func (m *Model) smokeFormat(ctx context.Context, t *Tab) any {
	text := t.doc.Text()
	const ugly = "\nclass SmokeFormat{int    x=1;}\n"
	if err := t.doc.ApplyEdits(ctx, editor.Edit{Start: len(text), End: len(text), Text: ugly}); err != nil {
		return map[string]any{"error": err.Error()}
	}
	defer func() {
		_ = t.doc.ApplyEdits(context.Background(), editor.Edit{Start: 0, End: len(t.doc.Text()), Text: text})
	}()
	start := time.Now()
	mygo.RunOnMain(func() { m.formatTab(t) })
	deadline := time.Now().Add(90 * time.Second)
	for {
		cur := t.doc.Text()
		if !strings.Contains(cur, "int    x") {
			tail := cur
			if len(tail) > 120 {
				tail = tail[len(tail)-120:]
			}
			return map[string]any{
				"changed":       cur != text+ugly,
				"spacedEquals":  strings.Contains(cur, "x = 1"),
				"tail":          tail,
				"elapsedMillis": time.Since(start).Milliseconds(),
			}
		}
		if time.Now().After(deadline) {
			return map[string]any{"timeout": true, "elapsedMillis": time.Since(start).Milliseconds()}
		}
		select {
		case <-ctx.Done():
			return map[string]any{"error": ctx.Err().Error()}
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// smokeRename adds a class with a method to the tab's text without saving it,
// renames the method through the prompt's path, and waits for the backend's
// workspace edit to change the buffer.
func (m *Model) smokeRename(ctx context.Context, t *Tab) any {
	text := t.doc.Text()
	const helper = "\nclass SmokeRenameHelper { static int smokeValue() { return 1; } }\n"
	if err := t.doc.ApplyEdits(ctx, editor.Edit{Start: len(text), End: len(text), Text: helper}); err != nil {
		return map[string]any{"error": err.Error()}
	}
	defer func() {
		_ = t.doc.ApplyEdits(context.Background(), editor.Edit{Start: 0, End: len(t.doc.Text()), Text: text})
	}()
	at := strings.Index(t.doc.Text(), "smokeValue")
	if at < 0 {
		return map[string]any{"error": "fixture"}
	}
	start := time.Now()
	mygo.RunOnMain(func() {
		t.state.Select(at+2, at+2)
		m.RenameActive()
	})
	deadline := time.Now().Add(150 * time.Second)
	var old string
	for {
		var opened bool
		mygo.RunOnMain(func() {
			opened = m.rename != nil
			if opened {
				old = m.rename.old
			}
		})
		if opened {
			break
		}
		if time.Now().After(deadline) {
			return map[string]any{"timeout": "prompt", "status": m.smokeStatus()}
		}
		select {
		case <-ctx.Done():
			return map[string]any{"error": ctx.Err().Error()}
		case <-time.After(200 * time.Millisecond):
		}
	}
	if old != "smokeValue" {
		return map[string]any{"error": "prompt did not find smokeValue", "old": old}
	}
	mygo.RunOnMain(func() {
		m.rename.name = "smokeNumber"
		m.submitRename()
	})
	for {
		cur := t.doc.Text()
		if strings.Contains(cur, "smokeNumber") {
			tail := cur
			if len(tail) > 120 {
				tail = tail[len(tail)-120:]
			}
			return map[string]any{
				"old":           old,
				"status":        m.smokeStatus(),
				"tail":          tail,
				"elapsedMillis": time.Since(start).Milliseconds(),
			}
		}
		// An edit upstream marks for preview waits for the confirmation the
		// prompt's button sends; the smoke confirms it as a user would.
		var pending bool
		mygo.RunOnMain(func() {
			pending = m.rename != nil && len(m.rename.pending) > 0
			if pending {
				m.applyRename()
			}
		})
		if time.Now().After(deadline) {
			var errText string
			var busy bool
			var files int
			mygo.RunOnMain(func() {
				if m.rename != nil {
					errText, busy, files = m.rename.err, m.rename.busy, m.rename.files
				}
			})
			return map[string]any{
				"timeout": "apply", "status": m.smokeStatus(), "err": errText,
				"busy": busy, "files": files, "elapsedMillis": time.Since(start).Milliseconds(),
			}
		}
		select {
		case <-ctx.Done():
			return map[string]any{"error": ctx.Err().Error()}
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// smokeStatus reads the status line on the main thread, where it is written.
func (m *Model) smokeStatus() string {
	var s string
	mygo.RunOnMain(func() { s = m.status })
	return s
}

// smokeDiagnostic is one diagnostic the smoke found shown in the editor.
type smokeDiagnostic struct {
	Start    int    `json:"start"`
	End      int    `json:"end"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Source   string `json:"source,omitempty"`
}

// smokeDiagnostics adds an undefined call to the tab's text without saving it,
// then waits for the language server's publish to show up as editor
// diagnostics, and puts the text back.
func (m *Model) smokeDiagnostics(ctx context.Context, t *Tab) any {
	text := t.doc.Text()
	const broken = "\nclass SmokeBroken { void run() { smokeUndefinedSymbol(); } }\n"
	if err := t.doc.ApplyEdits(ctx, editor.Edit{Start: len(text), End: len(text), Text: broken}); err != nil {
		return map[string]any{"error": err.Error()}
	}
	defer func() {
		_ = t.doc.ApplyEdits(ctx, editor.Edit{Start: len(text), End: len(text) + len(broken)})
	}()
	deadline := time.Now().Add(90 * time.Second)
	for {
		var ds []editor.Diagnostic
		mygo.RunOnMain(func() { ds = t.state.Diagnostics() })
		if len(ds) > 0 {
			list := make([]smokeDiagnostic, 0, len(ds))
			for i, d := range ds {
				if i == 20 {
					break
				}
				list = append(list, smokeDiagnostic{
					Start: d.Start, End: d.End, Severity: severityName(d.Severity),
					Message: d.Message, Source: d.Source,
				})
			}
			return list
		}
		if time.Now().After(deadline) {
			return map[string]any{"timeout": true}
		}
		select {
		case <-ctx.Done():
			return map[string]any{"error": ctx.Err().Error()}
		case <-time.After(time.Second):
		}
	}
}

func severityName(s editor.Severity) string {
	switch s {
	case editor.SeverityWarning:
		return "warning"
	case editor.SeverityInfo:
		return "info"
	case editor.SeverityHint:
		return "hint"
	}
	return "error"
}

// smokeLSPCall is one language-server request, bounded on its own so a stuck
// server cannot hold the whole smoke.
func (m *Model) smokeLSPCall(ctx context.Context, cmd string, args map[string]any, out any) error {
	c, cancel := context.WithTimeout(ctx, lspTimeout)
	defer cancel()
	raw, err := m.mgr.Invoke(c, Window, cmd, args)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

// lspServerState is a configured language server as the smoke reports it.
type lspServerState struct {
	Name      string          `json:"name"`
	Status    string          `json:"status"`
	Languages []string        `json:"languages"`
	Root      json.RawMessage `json:"root,omitempty"`
	PID       *uint32         `json:"pid,omitempty"`
}

func (m *Model) smokeLSPServers(ctx context.Context) []lspServerState {
	var list []lspServerState
	_ = m.smokeLSPCall(ctx, "lsp_servers", nil, &list)
	return list
}

func lspHasJavaServer(servers []lspServerState) bool {
	for _, s := range servers {
		if slices.Contains(s.Languages, "java") {
			return true
		}
	}
	return false
}

// lspJavaStalled reports whether every Java server failed to launch (missing
// binary or a configuration that cannot run), rather than being on its way up.
func lspJavaStalled(servers []lspServerState) bool {
	stalled := false
	for _, s := range servers {
		if !slices.Contains(s.Languages, "java") {
			continue
		}
		switch s.Status {
		case "missing", "misconfigured":
			stalled = true
		default:
			return false
		}
	}
	return stalled
}

func lspJavaStates(servers []lspServerState) string {
	var parts []string
	for _, s := range servers {
		if slices.Contains(s.Languages, "java") {
			parts = append(parts, s.Name+"="+s.Status)
		}
	}
	if len(parts) == 0 {
		return "无"
	}
	return strings.Join(parts, ", ")
}

// smokeLSPOffsets picks what the smoke asks about: the caret after the first
// dot, for completion, and the declared class name, for hover and definition,
// so the answer is in the file itself.
func smokeLSPOffsets(text string) (compOff, symbolOff int) {
	if i := strings.Index(text, "."); i >= 0 && i+1 < len(text) {
		compOff = i + 1
	}
	for _, decl := range []string{"class ", "interface ", "enum ", "record "} {
		if i := strings.Index(text, decl); i >= 0 {
			return compOff, i + len(decl)
		}
	}
	for i := 0; i < len(text); i++ {
		if c := text[i]; c == '_' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' {
			return compOff, i
		}
	}
	return compOff, 0
}
