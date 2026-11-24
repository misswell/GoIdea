package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"mayoi/internal/openidea"
)

var referenceIdentifier = regexp.MustCompile(`^[\pL_$][\pL\pN_$]*$`)

type referenceInput struct {
	language string
	path     string
	root     string
	text     string
	chain    []string
	line     int
}

type editorReferenceResult struct {
	text string
	err  error
}

// CopyActiveEditorPath is Edit › Copy Path.
func (m *Model) CopyActiveEditorPath() {
	t := m.tab()
	if t == nil || t.doc == nil || t.doc.Path() == "" {
		return
	}
	m.copyEditorText(t.doc.Path())
}

// CopyActiveEditorPathWithLineNumber is Edit › Copy Path with Line Number.
func (m *Model) CopyActiveEditorPathWithLineNumber() {
	t := m.tab()
	if t == nil || t.doc == nil || t.state == nil || t.doc.Path() == "" {
		return
	}
	line := t.state.CursorPos().Line + 1
	m.copyEditorText(fmt.Sprintf("%s:%d", projectRelative(m.Root(), t.doc.Path()), line))
}

// CopyActiveEditorReference ports OpenIDEA's Copy Reference. A declaration
// under the caret uses the outline and the language's module convention. If
// the caret is on a usage, the definition is resolved first. Without an
// unambiguous declaration, the project-relative path and line are copied.
func (m *Model) CopyActiveEditorReference() {
	t := m.tab()
	if t == nil || t.doc == nil || t.state == nil {
		return
	}
	result := m.resolveEditorReference(t)
	go func() {
		resolved := <-result
		m.post(func() {
			if resolved.err != nil {
				m.status = "Cannot copy reference"
				m.invalidate()
				return
			}
			m.copyReferenceText(resolved.text)
		})
	}()
}

// resolveEditorReference is the reusable asynchronous reference resolver used
// by both Copy Reference and the Copy Path/Reference chooser. It snapshots the
// invocation's document, caret and fallback before doing any backend work, so
// a later caret move cannot change the value that gets copied.
func (m *Model) resolveEditorReference(t *Tab) <-chan editorReferenceResult {
	result := make(chan editorReferenceResult, 1)
	if t == nil || t.doc == nil || t.state == nil {
		result <- editorReferenceResult{err: fmt.Errorf("no active editor")}
		close(result)
		return result
	}
	text := t.doc.Buffer().Snapshot().String()
	selection := t.state.Selection()
	from, to := selection.Anchor, selection.Head
	if from > to {
		from, to = to, from
	}
	pos := t.doc.UTF16Pos(from)
	root := m.Root()
	fallback := qualifiedReference(referenceInput{
		language: t.doc.Info().Language,
		path:     t.doc.Path(),
		root:     root,
		line:     int(pos.Line),
	})
	offset, ok := referenceOffsetInText(text, from, to)
	if !ok {
		result <- editorReferenceResult{text: fallback}
		close(result)
		return result
	}
	at := t.doc.UTF16Pos(offset)
	items := referenceDeclarationChain(t.outline, int(at.Line), int(at.Col))
	chain := referenceChainNames(items)
	if len(chain) > 0 {
		result <- editorReferenceResult{text: qualifiedReference(referenceInput{
			language: t.doc.Info().Language,
			path:     t.doc.Path(),
			root:     root,
			text:     text,
			chain:    chain,
			line:     int(at.Line),
		})}
		close(result)
		return result
	}
	if m.ctx == nil || m.reg == nil || m.mgr == nil {
		result <- editorReferenceResult{text: fallback}
		close(result)
		return result
	}
	docID := t.doc.ID()
	go func() {
		defer close(result)
		ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "lsp_goto", map[string]any{
			"doc": docID, "pos": lspPos(pos), "kind": "definition",
		})
		var targets []openidea.Location
		if err == nil {
			err = json.Unmarshal(raw, &targets)
		}
		if err != nil || len(targets) != 1 || strings.Contains(targets[0].Path, "://") {
			result <- editorReferenceResult{text: fallback}
			return
		}

		target := targets[0]
		targetDoc, err := m.reg.Open(ctx, target.Path)
		if err != nil {
			result <- editorReferenceResult{text: fallback}
			return
		}
		defer func() {
			closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer closeCancel()
			_ = m.reg.Close(closeCtx, target.Path)
		}()

		raw, err = m.invoke(ctx, "doc_outline", map[string]any{"doc": targetDoc.ID()})
		var outline []openidea.OutlineItem
		if err == nil {
			err = json.Unmarshal(raw, &outline)
		}
		if err != nil {
			result <- editorReferenceResult{text: fallback}
			return
		}
		chain := referenceDeclarationChain(outline,
			int(target.Range.Start.Line), int(target.Range.Start.Col))
		if len(chain) == 0 {
			result <- editorReferenceResult{text: fallback}
			return
		}
		last := chain[len(chain)-1]
		if last.Kind != "function" && last.Kind != "method" && last.Kind != "constructor" {
			result <- editorReferenceResult{text: fallback}
			return
		}
		reference := qualifiedReference(referenceInput{
			language: targetDoc.Info().Language,
			path:     target.Path,
			root:     root,
			text:     targetDoc.Text(),
			chain:    referenceChainNames(chain),
			line:     int(target.Range.Start.Line),
		})
		result <- editorReferenceResult{text: reference}
	}()
	return result
}

func (m *Model) copyReferenceText(text string) {
	if text == "" {
		return
	}
	clipboardWriteText(text)
	m.status = "Copied reference: " + text
	m.invalidate()
}

// referenceOffsetInText is OpenIDEA's rule for the identifier a reference
// uses: the whole selection when it is exactly an identifier, otherwise the
// identifier rune under a caret.
func referenceOffsetInText(text string, from, to int) (int, bool) {
	if from < 0 || to < from || to > len(text) {
		return 0, false
	}
	if from != to {
		if referenceIdentifier.MatchString(text[from:to]) {
			return from, true
		}
		return 0, false
	}
	if from >= len(text) {
		return 0, false
	}
	r, _ := utf8.DecodeRuneInString(text[from:])
	if r == utf8.RuneError || unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' || r == '$' {
		return from, true
	}
	return 0, false
}

// referenceDeclarationChain is OpenIDEA's declarationChain: the outline items
// enclosing the position, ending at the innermost item whose declared name
// contains it.
func referenceDeclarationChain(items []openidea.OutlineItem, line, col int) []openidea.OutlineItem {
	var chain []openidea.OutlineItem
	level := items
	for {
		hit := -1
		for i, item := range level {
			if outlinePositionContains(item.Range, line, col) {
				hit = i
				break
			}
		}
		if hit < 0 {
			break
		}
		item := level[hit]
		chain = append(chain, item)
		level = item.Children
	}
	named := -1
	for i, item := range chain {
		if outlinePositionContains(item.SelectionRange, line, col) {
			named = i
		}
	}
	if named < 0 {
		return nil
	}
	return chain[:named+1]
}

func outlinePositionContains(r openidea.OutlineRange, line, col int) bool {
	afterStart := int(r.Start.Line) < line ||
		int(r.Start.Line) == line && int(r.Start.Col) <= col
	beforeEnd := line < int(r.End.Line) ||
		line == int(r.End.Line) && col < int(r.End.Col)
	return afterStart && beforeEnd
}

func referenceChainNames(items []openidea.OutlineItem) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item.Kind == "package" || item.Name == "" {
			continue
		}
		out = append(out, referenceDeclaredName(item.Name, item.Kind))
	}
	return out
}

// referenceDeclaredName drops a language server's callable signature from a
// function, method or constructor name.
func referenceDeclaredName(name, kind string) string {
	if kind != "function" && kind != "method" && kind != "constructor" {
		return name
	}
	if open := strings.IndexByte(name, '('); open > 0 {
		return strings.TrimSpace(name[:open])
	}
	return name
}

func qualifiedReference(input referenceInput) string {
	fallback := referenceRelativePath(input.path, input.root) + ":" + strconv.Itoa(input.line+1)
	chain := make([]string, 0, len(input.chain))
	for _, name := range input.chain {
		if name != "" {
			chain = append(chain, name)
		}
	}
	if len(chain) == 0 {
		return fallback
	}
	switch strings.ToLower(input.language) {
	case "java", "groovy":
		return jvmReference(referenceJavaPackage(input.text), chain)
	case "kotlin", "scala":
		return jvmReference(referenceKotlinPackage(input.text), chain)
	case "csharp", "c#":
		return strings.Join(chain, ".")
	case "rust":
		names := make([]string, len(chain))
		for i, name := range chain {
			names[i] = referenceRustImplType(name)
		}
		return strings.Join(append([]string{referenceRustModulePath(input.path)}, names...), "::")
	case "python":
		return strings.Join(append([]string{referencePythonModulePath(input.path, input.root)}, chain...), ".")
	case "go":
		module := referenceGoPackage(input.text)
		if module == "" {
			module = referenceFileStem(input.path)
		}
		return strings.Join(append([]string{module}, chain...), ".")
	case "c", "cpp", "objective-cpp", "php":
		return strings.Join(chain, "::")
	default:
		return referenceRelativePath(input.path, input.root) + ":" + strings.Join(chain, ".")
	}
}

func referenceRelativePath(path, root string) string {
	if root == "" {
		return path
	}
	rel, err := filepath.Rel(root, path)
	if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(rel)
	}
	return path
}

func referenceFileStem(path string) string {
	name := filepath.Base(path)
	if dot := strings.LastIndexByte(name, '.'); dot > 0 {
		return name[:dot]
	}
	return name
}

func referenceRustImplType(name string) string {
	if strings.HasPrefix(name, "impl") {
		trimmed := strings.TrimSpace(strings.TrimPrefix(name, "impl"))
		if strings.HasPrefix(trimmed, "<") {
			if close := strings.IndexByte(trimmed, '>'); close >= 0 {
				trimmed = strings.TrimSpace(trimmed[close+1:])
			}
		}
		if at := strings.LastIndex(trimmed, " for "); at >= 0 {
			trimmed = strings.TrimSpace(trimmed[at+5:])
		}
		if open := strings.IndexByte(trimmed, '<'); open >= 0 {
			trimmed = trimmed[:open]
		}
		trimmed = strings.TrimPrefix(strings.TrimPrefix(trimmed, "&mut "), "&")
		if trimmed != "" {
			return trimmed
		}
	}
	return name
}

func referenceRustModulePath(path string) string {
	path = filepath.ToSlash(path)
	rel := filepath.Base(path)
	if at := strings.LastIndex(path, "/src/"); at >= 0 {
		rel = path[at+5:]
	}
	rel = strings.TrimSuffix(rel, ".rs")
	parts := strings.Split(rel, "/")
	if len(parts) > 0 {
		last := parts[len(parts)-1]
		if last == "mod" || len(parts) == 1 && (last == "lib" || last == "main") {
			parts = parts[:len(parts)-1]
		}
	}
	return strings.Join(append([]string{"crate"}, parts...), "::")
}

func referencePythonModulePath(path, root string) string {
	parts := strings.Split(strings.TrimSuffix(strings.TrimSuffix(
		referenceRelativePath(path, root), ".py"), ".pyi"), "/")
	for i, part := range parts {
		if part == "src" && i < len(parts)-1 {
			parts = parts[i+1:]
			break
		}
	}
	if len(parts) > 0 && parts[len(parts)-1] == "__init__" {
		parts = parts[:len(parts)-1]
	}
	return strings.Join(parts, ".")
}

func jvmReference(pkg string, chain []string) string {
	typePath := chain[0]
	if pkg != "" {
		typePath = pkg + "." + typePath
	}
	if len(chain) == 1 {
		return typePath
	}
	base := append([]string{typePath}, chain[1:len(chain)-1]...)
	return strings.Join(base, ".") + "#" + chain[len(chain)-1]
}

var (
	referenceJavaPackagePattern   = regexp.MustCompile(`(?m)^\s*package\s+([A-Za-z_$][\w$.]*)\s*;?`)
	referenceKotlinPackagePattern = regexp.MustCompile(`(?m)^\s*package\s+([A-Za-z_$][\w$.]*)`)
	referenceGoPackagePattern     = regexp.MustCompile(`(?m)^\s*package\s+([A-Za-z_]\w*)`)
)

func referenceJavaPackage(text string) string {
	if match := referenceJavaPackagePattern.FindStringSubmatch(text); len(match) == 2 {
		return match[1]
	}
	return ""
}

func referenceKotlinPackage(text string) string {
	if match := referenceKotlinPackagePattern.FindStringSubmatch(text); len(match) == 2 {
		return match[1]
	}
	return ""
}

func referenceGoPackage(text string) string {
	if match := referenceGoPackagePattern.FindStringSubmatch(text); len(match) == 2 {
		return match[1]
	}
	return ""
}
