package app

import (
	"errors"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/adapter"
	"mayoi/internal/openidea"
)

type externalMacroContext struct {
	projectRoot  string
	projectName  string
	filePath     string
	line         int
	column       int
	selectedText string
}

var externalMacroPattern = regexp.MustCompile(`\$([A-Za-z]+)\$`)

type externalToolPromptState struct {
	tool   string
	value  string
	result chan externalToolPromptResult
}

type externalToolPromptResult struct {
	value string
	ok    bool
}

var externalToolPromptMu sync.Mutex

func (m *Model) externalTool(id string) (ExternalToolPrefs, bool) {
	var tool ExternalToolPrefs
	found := false
	m.post(func() {
		for _, candidate := range m.prefs.ExternalTools {
			if candidate.ID == id {
				tool = candidate
				found = true
				break
			}
		}
	})
	return tool, found
}

func (m *Model) externalMacroContext() externalMacroContext {
	root := m.Root()
	ctx := externalMacroContext{
		projectRoot: root,
		projectName: filepath.Base(root),
	}
	if root == "" || root == "." {
		ctx.projectName = ""
	}
	if tab := m.tab(); tab != nil && tab.doc != nil {
		ctx.filePath = tab.doc.Path()
		pos := tab.state.CursorPos()
		ctx.line = pos.Line + 1
		ctx.column = pos.Col + 1
		ctx.selectedText = tab.state.SelectedText()
	}
	return ctx
}

func externalMacroValue(name string, ctx externalMacroContext, prompt string) (string, bool) {
	if name == "Prompt" {
		return prompt, true
	}
	file := ctx.filePath
	dir := ""
	base := ""
	if file != "" {
		dir = filepath.Dir(file)
		base = filepath.Base(file)
	}
	relative := func(path string) string {
		if path == "" || ctx.projectRoot == "" {
			return ""
		}
		rel, err := filepath.Rel(ctx.projectRoot, path)
		if err != nil {
			return path
		}
		if rel == "." {
			return ""
		}
		return filepath.ToSlash(rel)
	}
	switch name {
	case "FilePath":
		return file, true
	case "FileDir":
		return dir, true
	case "FileName":
		return base, true
	case "FileNameWithoutExtension":
		return strings.TrimSuffix(base, filepath.Ext(base)), true
	case "FileExt":
		return strings.TrimPrefix(filepath.Ext(base), "."), true
	case "FileRelativePath":
		return relative(file), true
	case "FileDirRelativeToProjectRoot":
		return relative(dir), true
	case "ProjectFileDir":
		return ctx.projectRoot, true
	case "ProjectName":
		return ctx.projectName, true
	case "LineNumber":
		if ctx.line == 0 {
			return "", true
		}
		return strconv.Itoa(ctx.line), true
	case "ColumnNumber":
		if ctx.column == 0 {
			return "", true
		}
		return strconv.Itoa(ctx.column), true
	case "SelectedText":
		return ctx.selectedText, true
	default:
		return "", false
	}
}

func expandExternalMacros(text string, ctx externalMacroContext, prompt string) string {
	return externalMacroPattern.ReplaceAllStringFunc(text, func(match string) string {
		parts := externalMacroPattern.FindStringSubmatch(match)
		if len(parts) != 2 {
			return match
		}
		value, ok := externalMacroValue(parts[1], ctx, prompt)
		if !ok {
			return match
		}
		return value
	})
}

func externalToolUsesFileMacro(text string) bool {
	for _, name := range []string{
		"$FilePath$", "$FileDir$", "$FileName$",
		"$FileRelativePath$", "$FileDirRelativeToProjectRoot$",
	} {
		if strings.Contains(text, name) {
			return true
		}
	}
	return false
}

func (m *Model) expandExternalTool(tool ExternalToolPrefs) (*openidea.RunConfig, error) {
	if strings.TrimSpace(tool.Program) == "" {
		return nil, errors.New("has no program")
	}
	var ctx externalMacroContext
	m.post(func() { ctx = m.externalMacroContext() })
	if ctx.filePath == "" && externalToolUsesFileMacro(tool.Program+tool.Arguments+tool.WorkingDirectory) {
		return nil, errors.New("needs an open file")
	}
	prompt := ""
	if strings.Contains(tool.Program+tool.Arguments+tool.WorkingDirectory, "$Prompt$") {
		value, ok := m.promptExternalToolValue(tool.Name)
		if !ok {
			return nil, errors.New("was cancelled")
		}
		prompt = value
	}
	program := strings.TrimSpace(expandExternalMacros(tool.Program, ctx, prompt))
	if program == "" {
		return nil, errors.New("has no program")
	}
	args := adapter.ParseArgs(tool.Arguments)
	for i := range args {
		args[i] = expandExternalMacros(args[i], ctx, prompt)
	}
	cwd := expandExternalMacros(strings.TrimSpace(tool.WorkingDirectory), ctx, prompt)
	if cwd == "" {
		cwd = ctx.projectRoot
	}
	return &openidea.RunConfig{
		ID:       "external:" + tool.ID,
		Name:     tool.Name,
		Kind:     "external",
		Command:  program,
		Args:     args,
		Cwd:      &cwd,
		Env:      map[string]string{},
		Detected: false,
	}, nil
}

func (m *Model) promptExternalToolValue(tool string) (string, bool) {
	externalToolPromptMu.Lock()
	defer externalToolPromptMu.Unlock()
	state := &externalToolPromptState{
		tool:   tool,
		result: make(chan externalToolPromptResult, 1),
	}
	m.post(func() {
		m.externalToolPrompt = state
		m.invalidate()
	})
	select {
	case <-m.ctx.Done():
		m.post(func() {
			if m.externalToolPrompt == state {
				m.externalToolPrompt = nil
			}
		})
		return "", false
	case result := <-state.result:
		return result.value, result.ok
	}
}

func (m *Model) finishExternalToolPrompt(ok bool) {
	state := m.externalToolPrompt
	if state == nil {
		return
	}
	m.externalToolPrompt = nil
	select {
	case state.result <- externalToolPromptResult{value: state.value, ok: ok}:
	default:
	}
	m.invalidate()
}

func (m *Model) externalToolPromptDialog(c *ui.Context, p *ui.Theme) {
	state := m.externalToolPrompt
	if state == nil {
		return
	}
	open := true
	dlg := ui.Modal(c.Key("tools:external-tool-prompt"), &open, func() {
		ui.Column(c).Width(440).Gap(10).Children(func() {
			ui.Text(c, state.tool).FontSize(13).Bold().TextColor(p.Text).
				SingleLine().Ellipsis("…")
			ui.Text(c, "Enter the value for $Prompt$:").FontSize(12).TextColor(p.TextMuted)
			input := ui.TextInput(c, &state.value).AutoFocus()
			if input.Submitted() {
				m.finishExternalToolPrompt(true)
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "OK").Clicked() {
					m.finishExternalToolPrompt(true)
				}
				if ui.Button(c, "Cancel").Clicked() {
					m.finishExternalToolPrompt(false)
				}
			})
		})
	})
	if dlg.Dismissed() || !open {
		m.finishExternalToolPrompt(false)
	}
}
