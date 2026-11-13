package app

import (
	"path/filepath"
	"sort"
	"strings"

	"goidea/internal/docbridge"
	"goidea/internal/openidea"

	"github.com/egoist/mygo/ui/editor"
)

// Diagnostics reach the editor: the backend publishes a file's problems per
// producer, each publish replacing that producer's earlier ones. The model
// keeps the newest ones per document and turns them into byte offsets of the
// text the tab shows, as the editor's ranges are.

// applyDiagnostics stores a publish and shows it in every tab of its file.
func (m *Model) applyDiagnostics(ev openidea.DiagnosticsEvent) {
	doc := m.documentFor(ev.Path)
	if doc == nil {
		return
	}
	per := m.diags[doc]
	if per == nil {
		per = make(map[string][]openidea.Diagnostic)
		if m.diags == nil {
			m.diags = make(map[*docbridge.Document]map[string][]openidea.Diagnostic)
		}
		m.diags[doc] = per
	}
	if len(ev.Items) == 0 {
		delete(per, ev.Source)
	} else {
		per[ev.Source] = ev.Items
	}
	if len(per) == 0 {
		delete(m.diags, doc)
	}
	for _, t := range m.tabs {
		if t.doc == doc {
			m.showDiagnostics(t)
		}
	}
	m.invalidate()
}

// showDiagnostics shows the stored diagnostics of a tab's document.
func (m *Model) showDiagnostics(t *Tab) {
	per := m.diags[t.doc]
	if len(per) == 0 {
		t.state.SetDiagnostics(t.doc.Buffer().Version(), nil)
		return
	}
	sources := make([]string, 0, len(per))
	for src := range per {
		sources = append(sources, src)
	}
	sort.Strings(sources)
	var ds []editor.Diagnostic
	for _, src := range sources {
		for _, it := range per[src] {
			start, end := t.doc.ByteRange(it.Range)
			ds = append(ds, editor.Diagnostic{
				Start:    start,
				End:      end,
				Severity: diagSeverity(it.Severity),
				Message:  it.Message,
				Source:   diagSource(src, it),
			})
		}
	}
	t.state.SetDiagnostics(t.doc.Buffer().Version(), ds)
}

// clearDiagnostics drops what was stored for the previous project.
func (m *Model) clearDiagnostics() {
	clear(m.diags)
}

// forgetDiagnostics drops a document's diagnostics once no tab shows it.
func (m *Model) forgetDiagnostics(doc *docbridge.Document) {
	if doc == nil {
		return
	}
	for _, t := range m.tabs {
		if t.doc == doc {
			return
		}
	}
	delete(m.diags, doc)
}

// documentFor finds the open document an event path names. The backend
// canonicalizes paths, so a direct miss falls back to resolving symlinks.
func (m *Model) documentFor(path string) *docbridge.Document {
	if doc := m.reg.Get(path); doc != nil {
		return doc
	}
	want := realPath(path)
	for _, doc := range m.reg.Documents() {
		if realPath(doc.Path()) == want {
			return doc
		}
	}
	return nil
}

func realPath(path string) string {
	if p, err := filepath.EvalSymlinks(path); err == nil {
		return p
	}
	if p, err := filepath.Abs(path); err == nil {
		return p
	}
	return filepath.Clean(path)
}

// diagSeverity maps a protocol severity to the editor's, an unknown one being
// an error so a problem is never silently invisible.
func diagSeverity(s string) editor.Severity {
	switch strings.ToLower(s) {
	case "warning":
		return editor.SeverityWarning
	case "info":
		return editor.SeverityInfo
	case "hint":
		return editor.SeverityHint
	}
	return editor.SeverityError
}

// diagSource names the tool behind a diagnostic: the language server when it
// says, else what its producer calls itself.
func diagSource(published string, it openidea.Diagnostic) string {
	if it.Server != nil && *it.Server != "" {
		return *it.Server
	}
	if it.Source != nil && *it.Source != "" {
		return *it.Source
	}
	return published
}
