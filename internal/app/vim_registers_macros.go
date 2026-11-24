package app

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui/editor"
)

type vimRegisterWrite uint8

const (
	vimRegisterYank vimRegisterWrite = iota
	vimRegisterDelete
)

func vimRegisterName(token string) (rune, bool) {
	if len(token) != 1 {
		return 0, false
	}
	name := rune(token[0])
	if name >= 'a' && name <= 'z' || name >= 'A' && name <= 'Z' {
		return name, true
	}
	switch name {
	case '"', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9',
		'-', '_', '+', '*', '/', ':', '.', '%', '#':
		return name, true
	default:
		return 0, false
	}
}

func (m *Model) vimSelectRegister(st *vimTabState, token string) bool {
	name, ok := vimRegisterName(token)
	if !ok {
		return false
	}
	st.registerName = name
	return true
}

func vimRegisterReadOnly(name rune) bool {
	return name >= '0' && name <= '9' || name == '-' || name == '/' ||
		name == ':' || name == '.' || name == '%' || name == '#'
}

func vimRegisterWrites(name rune, token string) bool {
	if vimRegisterReadOnly(name) {
		switch token {
		case "d", "c", "y", "x", "X", "D", "C", "Y", "s", "S", "q":
			return true
		}
	}
	return false
}

func (m *Model) vimRejectReadOnlyRegister(st *vimTabState, token string) bool {
	if st.registerName == 0 || !vimRegisterWrites(st.registerName, token) {
		return false
	}
	m.status = fmt.Sprintf("Vim: E46: Cannot change read-only register %c", st.registerName)
	st.registerName = 0
	return true
}

func vimLowerRegister(name rune) rune {
	if name >= 'A' && name <= 'Z' {
		return name - 'A' + 'a'
	}
	return name
}

func (m *Model) vimNamedRegister(st *vimTabState, name rune) (vimRegister, bool) {
	if m.vim.registers == nil {
		return vimRegister{}, false
	}
	name = vimLowerRegister(name)
	reg, ok := m.vim.registers[name]
	return reg, ok && reg.set
}

func (m *Model) vimStoreRegister(t *Tab, st *vimTabState, reg vimRegister, write vimRegisterWrite) bool {
	if !reg.set {
		return false
	}
	name := st.registerName
	st.registerName = 0
	if vimRegisterReadOnly(name) {
		m.status = fmt.Sprintf("Vim: E46: Cannot change read-only register %c", name)
		return false
	}
	if name == '_' {
		return true
	}
	if m.vim.registers == nil {
		m.vim.registers = make(map[rune]vimRegister)
	}
	st.register = reg
	m.vim.registers['"'] = reg
	switch write {
	case vimRegisterYank:
		m.vim.registers['0'] = reg
	case vimRegisterDelete:
		m.vim.registers['1'] = reg
		if !reg.linewise && !reg.block {
			m.vim.registers['-'] = reg
		}
	}
	switch {
	case name == 0 || name == '"':
	case name >= 'a' && name <= 'z':
		m.vim.registers[name] = reg
	case name >= 'A' && name <= 'Z':
		lower := vimLowerRegister(name)
		m.vim.registers[lower] = vimAppendRegister(m.vim.registers[lower], reg)
	case name == '+' || name == '*':
		m.vim.registers[name] = reg
	}
	vimClipboardWrite(reg.text)
	_ = t
	return true
}

func vimAppendRegister(existing, addition vimRegister) vimRegister {
	if !existing.set {
		return addition
	}
	combined := addition
	switch {
	case existing.linewise && addition.linewise:
		combined.linewise = true
	case existing.block && addition.block:
		combined.block = true
		combined.blockWidth = addition.blockWidth
	}
	combined.text = existing.text + addition.text
	combined.set = true
	return combined
}

func (m *Model) vimRegisterFor(t *Tab, st *vimTabState) (vimRegister, bool) {
	name := st.registerName
	st.registerName = 0
	switch {
	case name == 0 || name == '"':
		if reg, ok := m.vimNamedRegister(st, '"'); ok {
			return reg, true
		}
		if st.register.set {
			return st.register, true
		}
	case name >= 'a' && name <= 'z' || name >= 'A' && name <= 'Z':
		return m.vimNamedRegister(st, name)
	case name >= '0' && name <= '9' || name == '-':
		return m.vimNamedRegister(st, name)
	case name == '+' || name == '*':
		if reg, ok := m.vimNamedRegister(st, name); ok && reg.text != "" {
			return reg, true
		}
		text := vimClipboardRead()
		return vimRegister{text: text, set: text != ""}, text != ""
	case name == '/':
		if m.vim.search.set {
			return vimRegister{text: m.vim.search.pattern, set: true}, true
		}
		return vimRegister{}, false
	case name == ':':
		if m.vim.lastEx != "" {
			return vimRegister{text: m.vim.lastEx, set: true}, true
		}
		return vimRegister{}, false
	case name == '%':
		if t != nil && t.doc != nil {
			return vimRegister{text: t.doc.Path(), set: true}, true
		}
		return vimRegister{}, false
	case name == '_' || name == '.' || name == '#':
		return vimRegister{}, false
	}
	if st.register.set {
		return st.register, true
	}
	text := vimClipboardRead()
	return vimRegister{text: text, set: text != ""}, text != ""
}

func (m *Model) vimStartMacro(st *vimTabState, token string) bool {
	name, ok := vimRegisterName(token)
	if !ok || vimRegisterReadOnly(name) {
		return false
	}
	lower := vimLowerRegister(name)
	if m.vim.macros != nil && name >= 'A' && name <= 'Z' {
		st.macroTokens = append([]string(nil), m.vim.macros[lower]...)
	} else {
		st.macroTokens = nil
	}
	st.recording = name
	return true
}

func (m *Model) vimStopMacro(st *vimTabState) {
	if st.recording == 0 {
		return
	}
	name := vimLowerRegister(st.recording)
	tokens := append([]string(nil), st.macroTokens...)
	if m.vim.macros == nil {
		m.vim.macros = make(map[rune][]string)
	}
	if m.vim.registers == nil {
		m.vim.registers = make(map[rune]vimRegister)
	}
	m.vim.macros[name] = tokens
	m.vim.registers[name] = vimRegister{
		text: vimMacroRegisterText(tokens),
		set:  true,
	}
	st.recording = 0
	st.macroTokens = nil
}

func (m *Model) vimRecordMacroToken(st *vimTabState, token string) {
	if st.recording == 0 || st.macroPlayback != 0 {
		return
	}
	st.macroTokens = append(st.macroTokens, token)
}

func (m *Model) vimRecordMacroCommand(st *vimTabState, command *vimCommandState) {
	if st == nil || command == nil || st.recording == 0 || st.macroPlayback != 0 {
		return
	}
	st.macroTokens = append(st.macroTokens, command.prompt)
	for _, r := range command.input {
		st.macroTokens = append(st.macroTokens, string(r))
	}
	st.macroTokens = append(st.macroTokens, "<Enter>")
}

func vimMacroRegisterText(tokens []string) string {
	var out strings.Builder
	for _, token := range tokens {
		switch token {
		case "<Enter>":
			out.WriteByte('\n')
		case "<Tab>":
			out.WriteByte('\t')
		default:
			out.WriteString(token)
		}
	}
	return out.String()
}

func (m *Model) vimPlayMacro(t *Tab, st *vimTabState, token string, count, depth int) bool {
	if !m.vimSearchReady(t) {
		m.status = "Vim: E158: Invalid buffer name"
		return true
	}
	var name rune
	if token == "@" {
		name = m.vim.lastMacro
	} else {
		var ok bool
		name, ok = vimRegisterName(token)
		if !ok || vimRegisterReadOnly(name) {
			return false
		}
		name = vimLowerRegister(name)
	}
	if name == 0 {
		m.status = "Vim: E748: No previously used register"
		return true
	}
	tokens := append([]string(nil), m.vim.macros[name]...)
	if len(tokens) == 0 {
		if reg, ok := m.vimNamedRegister(st, name); ok && reg.text != "" {
			tokens = tokenizeVimKeys(reg.text)
		}
	}
	if len(tokens) == 0 {
		m.status = fmt.Sprintf("Vim: E748: Register %c is empty", name)
		return true
	}
	m.vim.lastMacro = name
	st.macroPlayback++
	defer func() { st.macroPlayback-- }()
	b := t.state.Buffer
	b.Transact(func() {
		for range max(1, count) {
			if !m.hasTab(t) {
				return
			}
			for _, recorded := range tokens {
				if command := m.vim.command; command != nil && command.tab == t {
					m.vimPlayCommandToken(command, recorded)
					if m.vim.command != nil && recorded == "<Enter>" {
						return
					}
					continue
				}
				handled := m.vimFeedToken(t, st, recorded, depth+1)
				if !handled {
					m.vimPlaybackToken(t, st, recorded)
				}
			}
		}
	})
	return true
}

func (m *Model) vimPlayCommandToken(command *vimCommandState, token string) {
	if command == nil {
		return
	}
	switch token {
	case "<Enter>":
		m.submitVimCommand()
	case "<Esc>", "<Ctrl+C>", "<Ctrl-[>":
		m.cancelVimCommand()
	case "<Backspace>":
		if command.input == "" {
			return
		}
		_, size := utf8.DecodeLastRuneInString(command.input)
		command.input = command.input[:len(command.input)-size]
	case "<Space>", " ":
		command.input += " "
	case "<Tab>", "\t":
		command.input += "\t"
	default:
		if utf8RuneCount(token) == 1 {
			command.input += token
		}
	}
}

func (m *Model) vimPlaybackToken(t *Tab, st *vimTabState, token string) {
	if st.mode != vimInsert || t == nil || t.state == nil || t.state.Buffer == nil {
		return
	}
	b := t.state.Buffer
	switch token {
	case "<Enter>":
		t.state.Insert("\n")
	case "<Tab>":
		t.state.Insert("\t")
	case "<Backspace>":
		edits := make([]editor.Edit, 0, len(t.state.Selections()))
		for _, sel := range t.state.Selections() {
			head := sel.Head
			start := head
			if sel.Anchor != sel.Head {
				start, head = min(sel.Anchor, sel.Head), max(sel.Anchor, sel.Head)
			} else {
				start = vimRuneLeft(b, head)
			}
			if start < head {
				edits = append(edits, editor.Edit{Start: start, End: head})
			}
		}
		if len(edits) > 0 {
			b.Edit(edits...)
		}
	case "<Delete>":
		edits := make([]editor.Edit, 0, len(t.state.Selections()))
		for _, sel := range t.state.Selections() {
			start, end := min(sel.Anchor, sel.Head), max(sel.Anchor, sel.Head)
			if start == end {
				end = vimRuneRight(b, end)
			}
			if end > start {
				edits = append(edits, editor.Edit{Start: start, End: end})
			}
		}
		if len(edits) > 0 {
			b.Edit(edits...)
		}
	default:
		if utf8RuneCount(token) == 1 {
			t.state.Insert(token)
		}
	}
}

func utf8RuneCount(text string) int {
	count := 0
	for range text {
		count++
	}
	return count
}

func (m *Model) vimSetMark(t *Tab, st *vimTabState, token string) bool {
	name, ok := vimMarkName(token)
	if !ok || t == nil || t.state == nil || t.state.Buffer == nil {
		return false
	}
	if st.marks == nil {
		st.marks = make(map[rune]int)
	}
	st.marks[name] = max(0, min(t.state.Cursor(), t.state.Buffer.Len()))
	return true
}

func (m *Model) vimJumpMark(t *Tab, st *vimTabState, token string, linewise bool) bool {
	name, ok := vimMarkName(token)
	if !ok || t == nil || t.state == nil || t.state.Buffer == nil {
		return false
	}
	offset, found := st.marks[name]
	if !found {
		m.status = fmt.Sprintf("Vim: E20: Mark not set: %c", name)
		return true
	}
	offset = max(0, min(offset, t.state.Buffer.Len()))
	if linewise {
		line := t.state.Buffer.Pos(offset).Line
		offset = vimFirstNonBlankOffset(t.state.Buffer, line)
	}
	t.state.Select(offset, offset)
	return true
}

func vimMarkName(token string) (rune, bool) {
	if len(token) != 1 || token[0] < 'a' || token[0] > 'z' {
		return 0, false
	}
	return rune(token[0]), true
}
