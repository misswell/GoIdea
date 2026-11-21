package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

type sourceActionSpec struct {
	kind     string
	label    string
	progress string
	done     string
	none     string
}

var (
	optimizeImportsAction = sourceActionSpec{
		kind:     "source.organizeImports",
		label:    "Optimize Imports",
		progress: "Optimizing imports",
		done:     "Optimize Imports done",
		none:     "Optimize imports is not available",
	}
	fixESLintProblemsAction = sourceActionSpec{
		kind:     "source.fixAll.eslint",
		label:    "Fix ESLint Problems",
		progress: "Fixing ESLint problems",
		done:     "Fix ESLint Problems done",
		none:     "No ESLint problems to fix",
	}
)

// OptimizeImports applies the language server's source.organizeImports action
// to the active document.
func (m *Model) OptimizeImports() {
	if tab := m.tab(); tab != nil {
		m.optimizeImportsTab(tab)
	}
}

func (m *Model) optimizeImportsTab(tab *Tab) {
	m.runSourceAction(tab, optimizeImportsAction)
}

// FixESLintProblems applies vscode-eslint's whole-file source.fixAll.eslint
// action to the active document.
func (m *Model) FixESLintProblems() {
	if tab := m.tab(); tab != nil {
		m.runSourceAction(tab, fixESLintProblemsAction)
	}
}

// runSourceAction asks the server for one action kind over the whole file,
// then applies the preferred enabled result. The backend's workspace edit
// reaches every open buffer and its applied event enables Undo Refactoring.
func (m *Model) runSourceAction(tab *Tab, spec sourceActionSpec) {
	if !m.tabOpen(tab) || tab.formatting {
		return
	}
	tab.formatting = true
	m.status = spec.progress + " in " + filepath.Base(tab.doc.Path()) + "…"
	m.invalidate()
	docID := tab.doc.ID()
	request := m.nextRequest()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, intentTimeout)
		defer cancel()
		if err := tab.doc.Sync(ctx); err != nil {
			m.finishSourceAction(tab, spec.progress, "", err)
			return
		}
		raw, err := m.invoke(ctx, "lsp_code_action_query", map[string]any{
			"request": request,
			"doc":     docID,
			"range": map[string]any{
				"start": lspPos(openidea.OutlinePos{Line: 0, Col: 0}),
				"end":   lspPos(tab.doc.UTF16Pos(len(tab.doc.Text()))),
			},
			"only": []string{spec.kind},
		})
		var query openidea.CodeActionQuery
		if err == nil {
			err = json.Unmarshal(raw, &query)
		}
		var chosen json.RawMessage
		if err == nil {
			chosen = chooseSourceAction(parseIntentActions(query.Actions), spec.kind)
			if len(chosen) == 0 {
				m.finishSourceAction(tab, spec.progress, spec.none, nil)
				return
			}
		}
		if err == nil {
			_, err = m.invoke(ctx, "lsp_apply_code_action", map[string]any{
				"doc": docID, "action": chosen,
			})
		}
		m.finishSourceAction(tab, spec.progress, spec.done, err)
	}()
}

// chooseSourceAction picks the first enabled action of the requested kind,
// preferring the server's suggested row.
func chooseSourceAction(actions []intentAction, kind string) json.RawMessage {
	var fallback json.RawMessage
	for _, action := range actions {
		if action.disabled != "" || !strings.HasPrefix(action.kind, kind) {
			continue
		}
		if action.preferred {
			return action.raw
		}
		if len(fallback) == 0 {
			fallback = action.raw
		}
	}
	return fallback
}

func (m *Model) finishSourceAction(tab *Tab, progress, result string, err error) {
	m.post(func() {
		tab.formatting = false
		if !m.tabOpen(tab) {
			return
		}
		if err != nil {
			m.status = progress + " failed"
			m.fail(tab.doc.Path(), err)
		} else {
			m.status = result
		}
		m.invalidate()
	})
}

// fixESLintProblemsAvailable is OpenIDEA's eslintCovers menu condition. The
// server must be running, list the editor's language, and be rooted at or
// above the file. An empty root is a project-wide server.
func (m *Model) fixESLintProblemsAvailable(tab *Tab) bool {
	if tab == nil || tab.doc == nil || tab.state == nil || tab.state.ReadOnly ||
		tab.doc.ReadOnly() || !m.tabOpen(tab) {
		return false
	}
	language := editor.LanguageFor(tab.doc.Path())
	if language == nil {
		return false
	}
	file := normalizeLanguageServerPath(tab.doc.Path())
	for _, server := range m.languageServers {
		if !strings.EqualFold(server.Name, "eslint") ||
			(server.Status != "ready" && server.Status != "busy") ||
			!containsFold(server.Languages, language.Name) {
			continue
		}
		root := normalizeLanguageServerPath(server.Root)
		if root == "" || strings.HasPrefix(file, root+"/") {
			return true
		}
	}
	return false
}

func normalizeLanguageServerPath(value string) string {
	value = strings.ReplaceAll(value, `\`, "/")
	for len(value) > 1 && strings.HasSuffix(value, "/") {
		value = strings.TrimSuffix(value, "/")
	}
	return value
}

func containsFold(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(value, want) {
			return true
		}
	}
	return false
}
