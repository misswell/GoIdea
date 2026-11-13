package position

// Position is an LSP/OpenIDEA position: a line and a UTF-16 code-unit column.
type Position struct {
	Line      uint32 `json:"line"`
	Character uint32 `json:"character"`
}

// Range is a range between two positions.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// TextChange replaces the UTF-16 range [From, To) of a document with Insert.
// As in CodeMirror, the offsets are into the document before the change set is
// applied, and the changes of a set are ordered and do not overlap.
type TextChange struct {
	From   uint32 `json:"from"`
	To     uint32 `json:"to"`
	Insert string `json:"insert"`
}

// ChangeSet is the payload of doc_edit.
type ChangeSet struct {
	Changes []TextChange `json:"changes"`
}

// TextEdit is an LSP edit: a range and its replacement.
type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}
