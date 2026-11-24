package openidea

import "encoding/json"

// InlineCompletionItem is one lsp_inline_completion suggestion. Range is in
// the backend's UTF-16 coordinates; nil inserts at the requested caret.
type InlineCompletionItem struct {
	InsertText string          `json:"insertText"`
	Range      *OutlineRange   `json:"range"`
	FilterText *string         `json:"filterText"`
	Command    json.RawMessage `json:"command"`
	Server     uint32          `json:"server"`
}
