package app

// pom.xml dependency completion, after v0.1.44's pomCompletion.ts: inside a
// dependency's groupId/artifactId/version the backend
// (maven_dependency_completion) suggests the coordinates in the local Maven
// repository and Central's popular ones. GoIDEA asks the same RPC and replaces
// the backend's complete value span, including text after the caret.

import (
	"context"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

type pomCompletionOption struct {
	Label  string  `json:"label"`
	Detail *string `json:"detail"`
}

type pomCompletion struct {
	From    uint32                `json:"from"`
	To      uint32                `json:"to"`
	Kind    string                `json:"kind"`
	Options []pomCompletionOption `json:"options"`
}

func isPomPath(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), "/pom.xml") || strings.EqualFold(path, "pom.xml")
}

// requestPomCompletion asks the backend for the coordinates at the caret and
// shows them in the editor, replacing the value from its first character.
func (m *Model) requestPomCompletion(t *Tab, req editor.CompletionRequest) {
	if !m.tabOpen(t) || (m.mgr == nil && m.invokeHook == nil) || time.Now().Before(t.compOff) {
		return
	}
	doc := t.doc
	text := doc.Buffer().Snapshot().String()
	if req.Offset < 0 || req.Offset > len(text) {
		return
	}
	from := req.Offset
	if from < 0 || from > len(text) {
		return
	}
	if t.compReq != 0 {
		m.cancelRequest(t.compReq)
	}
	id := m.nextRequest()
	t.compReq = id
	t.compGen++
	gen := t.compGen
	docID := doc.ID()
	offset := uint32(byteToUTF16Offset(text, req.Offset))
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, lspTimeout)
		defer cancel()
		raw, err := json.RawMessage(nil), doc.Sync(ctx)
		if err == nil {
			raw, err = m.invoke(ctx, "maven_dependency_completion", map[string]any{
				"request": id, "doc": docID, "offset": offset,
			})
		}
		var found *pomCompletion
		if err == nil {
			if strings.TrimSpace(string(raw)) != "null" {
				err = json.Unmarshal(raw, &found)
			} else {
				found = nil
			}
		}
		items := make([]editor.Completion, 0, 32)
		infos := map[string]completionInfo{}
		if found != nil {
			for _, o := range found.Options {
				it := editor.Completion{
					Label: o.Label,
					Kind:  editor.CompletionConstant,
					// MyGo filters native completions against the typed word.
					// Prefixing the backend's full label keeps its installed-
					// first order without hiding coordinates that do not start
					// with that word.
					FilterText: req.Prefix + o.Label,
				}
				if o.Detail != nil {
					it.Detail = *o.Detail
				}
				infos[completionTrackKey(it)] = completionInfo{
					Replace: &completionReplacement{From: from, To: from},
				}
				items = append(items, it)
			}
		}
		start := from
		if found != nil {
			foundFrom := int(utf16ToByteOffset(text, int(found.From)))
			foundTo := int(utf16ToByteOffset(text, int(found.To)))
			if foundFrom < 0 || foundTo < foundFrom || foundTo > len(text) ||
				from < foundFrom || from > foundTo {
				return
			}
			start = foundFrom
			for _, item := range items {
				infos[completionTrackKey(item)] = completionInfo{
					Replace: &completionReplacement{From: foundFrom, To: foundTo},
				}
			}
		}
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
			if t.compGen != gen || !m.tabOpen(t) || t.doc.ID() != docID ||
				t.doc.Buffer().Version() != req.Version {
				return
			}
			m.showCompletionResults(t, req, start, items, infos)
		})
	}()
}

// utf16ToByteOffset maps a UTF-16 offset of text to its byte offset.
func utf16ToByteOffset(text string, off int) int {
	if off <= 0 {
		return 0
	}
	u, i := 0, 0
	for i < len(text) {
		if u >= off {
			return i
		}
		r, size := decodeRune(text[i:])
		if r > 0xFFFF {
			u += 2
		} else {
			u++
		}
		i += size
	}
	return len(text)
}

// byteToUTF16Offset maps a byte offset of text to its UTF-16 offset.
func byteToUTF16Offset(text string, byteOff int) int {
	if byteOff <= 0 {
		return 0
	}
	if byteOff > len(text) {
		byteOff = len(text)
	}
	u := 0
	for i := 0; i < byteOff; {
		r, size := decodeRune(text[i:])
		if i+size > byteOff {
			break
		}
		if r > 0xFFFF {
			u += 2
		} else {
			u++
		}
		i += size
	}
	return u
}

func decodeRune(s string) (rune, int) {
	return utf8.DecodeRuneInString(s)
}
