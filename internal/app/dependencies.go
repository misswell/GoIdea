package app

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

// isDependencyManifest reports the three manifests OpenIDEA parses for
// dependency version completion.
func isDependencyManifest(path string) bool {
	path = strings.ReplaceAll(path, `\`, "/")
	switch strings.ToLower(filepath.Base(path)) {
	case "cargo.toml", "go.mod", "package.json":
		return true
	default:
		return false
	}
}

func isCargoManifest(path string) bool {
	path = strings.ReplaceAll(path, `\`, "/")
	return strings.EqualFold(filepath.Base(path), "Cargo.toml")
}

// requestDependencyCompletion asks OpenIDEA for versions at the manifest
// dependency containing the caret. The backend owns parsing each ecosystem;
// the editor only converts its UTF-16 dependency range to native byte offsets.
func (m *Model) requestDependencyCompletion(t *Tab, req editor.CompletionRequest) bool {
	if !m.tabOpen(t) || m.mgr == nil || !isDependencyManifest(t.doc.Path()) {
		return false
	}
	doc := t.doc
	if t.doc.ReadOnly() {
		return true
	}
	text := doc.Buffer().Snapshot().String()
	if req.Offset < 0 || req.Offset > len(text) {
		return true
	}
	if t.compReq != 0 {
		m.cancelRequest(t.compReq)
	}
	id := m.nextRequest()
	t.compReq = id
	t.compGen++
	gen := t.compGen
	docID := doc.ID()
	offset := byteToUTF16Offset(text, req.Offset)
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, lspTimeout)
		defer cancel()
		if err := doc.Sync(ctx); err != nil {
			m.post(func() {
				if t.compReq == id {
					t.compReq = 0
				}
			})
			return
		}
		found, err := m.mgr.DependencyVersions(ctx, Window, id, docID, &offset)
		m.post(func() {
			if t.compReq == id {
				t.compReq = 0
			}
			if err != nil {
				// Offline registries and cancelled checks never interrupt editing.
				return
			}
			if t.compGen != gen || !m.tabOpen(t) || t.doc.Buffer().Snapshot().String() != text {
				return
			}
			start, items := dependencyCompletionItems(text, offset, req.Explicit, found)
			if start < 0 || len(items) == 0 {
				return
			}
			m.showCompletionResults(t, req, start, items, nil)
		})
	}()
	return true
}

// dependencyCompletionItems converts the backend's dependency range and
// versions into native completion rows. The replacement starts after a Cargo
// operator prefix, so selecting a version leaves ^, ~, >= and similar text
// intact.
func dependencyCompletionItems(
	text string,
	offset int,
	explicit bool,
	found []openidea.DependencyVersions,
) (int, []editor.Completion) {
	for _, item := range found {
		dependency := item.Dependency
		if offset < dependency.From || offset > dependency.To {
			continue
		}
		from := utf16ToByteOffset(text, dependency.From)
		to := utf16ToByteOffset(text, dependency.To)
		if from < 0 || to < from || to > len(text) || offset > len(text) {
			return -1, nil
		}
		cursor := utf16ToByteOffset(text, offset)
		if cursor < from || cursor > to {
			return -1, nil
		}
		typed := text[from:cursor]
		operator := dependencyOperatorPrefix(typed)
		start := from + len(operator)
		filter := typed[len(operator):]
		items := make([]editor.Completion, 0, len(item.Versions))
		for _, version := range item.Versions {
			if !explicit && !strings.HasPrefix(version, filter) {
				continue
			}
			filterText := version
			if explicit {
				// MyGo filters completion rows as the user types. Upstream's
				// explicit request shows every version, including non-prefix
				// matches, without changing their order.
				filterText = typed + version
			}
			items = append(items, editor.Completion{
				Label:      version,
				Detail:     dependency.Package,
				Kind:       editor.CompletionConstant,
				Insert:     version,
				FilterText: filterText,
			})
		}
		return start, items
	}
	return -1, nil
}

// dependencyOperatorPrefix is the Cargo requirement operator before the
// version text, which completion and quick fixes preserve.
func dependencyOperatorPrefix(typed string) string {
	for i, r := range typed {
		if r != ' ' && r != '\t' && r != '\r' && r != '\n' &&
			r != '^' && r != '~' && r != '<' && r != '>' && r != '=' {
			return typed[:i]
		}
	}
	return typed
}

type dependencyIntentEdit struct {
	from        int
	to          int
	replacement string
	latest      string
	version     uint64
}

// dependencyIntentItems turns Cargo's newer-version answer into a native
// quick fix. The replacement range and document version make a stale action
// harmless if the manifest changed while the prompt was open.
func dependencyIntentItems(
	text string,
	selection int,
	version uint64,
	found []openidea.DependencyVersions,
) []intentAction {
	items := make([]intentAction, 0)
	for _, item := range found {
		if item.Update == nil || strings.TrimSpace(item.Update.Replacement) == "" {
			continue
		}
		from := utf16ToByteOffset(text, item.Dependency.From)
		to := utf16ToByteOffset(text, item.Dependency.To)
		if from < 0 || to < from || to > len(text) ||
			selection < from || selection > to {
			continue
		}
		items = append(items, intentAction{
			title: "Update to " + item.Update.Replacement,
			kind:  "quickfix",
			dependency: &dependencyIntentEdit{
				from:        from,
				to:          to,
				replacement: item.Update.Replacement,
				latest:      item.Update.Latest,
				version:     version,
			},
		})
	}
	return items
}

// DependencyCompletionDelay is the quiet period before a Cargo inspection
// queries the registry, matching OpenIDEA's editor contribution.
const dependencyCompletionDelay = 500 * time.Millisecond
