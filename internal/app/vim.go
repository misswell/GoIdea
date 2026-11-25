package app

// Vim mode after OpenIDEA's IdeaVim integration. The mode machine is native
// to GoIDEA's MyGo editors: normal, insert, visual and visual-line modes share
// the editor's buffer and selections, while rc-file mappings can dispatch the
// same registered actions as menus and shortcuts.

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

// VimPrefs is Settings › Editor › Vim. Handlers stores only overrides of the
// conflict defaults, keyed by their normalized strokes.
type VimPrefs struct {
	Enabled  bool              `json:"enabled,omitempty"`
	Handlers map[string]string `json:"handlers,omitempty"`
}

type vimMode uint8

const (
	vimNormal vimMode = iota
	vimInsert
	vimVisual
	vimVisualLine
	vimVisualBlock
)

func (m vimMode) label() string {
	switch m {
	case vimInsert:
		return "INSERT"
	case vimVisual:
		return "VISUAL"
	case vimVisualLine:
		return "V-LINE"
	case vimVisualBlock:
		return "V-BLOCK"
	default:
		return "NORMAL"
	}
}

func (m vimMode) visual() bool {
	return m == vimVisual || m == vimVisualLine || m == vimVisualBlock
}

type vimRegister struct {
	text       string
	linewise   bool
	block      bool
	blockWidth int
	set        bool
}

type vimTabState struct {
	mode vimMode

	count   int
	pending string

	operator      rune
	operatorCount int
	operatorText  bool
	operatorInner bool

	visualAnchor int
	visualHead   int
	visualText   bool
	visualInner  bool

	mappingKeys  []string
	register     vimRegister
	registerName rune

	marks map[rune]int

	recording     rune
	macroTokens   []string
	macroPlayback int
	macroCount    int
	blockInsert   bool
}

// vimRuntime is the loaded configuration and each editor's mode state. The
// map is owned by the main thread, like the model and every editor state.
type vimRuntime struct {
	tabs     map[*Tab]*vimTabState
	rc       vimRcState
	rcLoaded bool

	mappings map[string][]vimMapping
	options  vimOptions
	command  *vimCommandState

	registers map[rune]vimRegister
	macros    map[rune][]string
	lastMacro rune
	search    vimSearchState
	lastEx    string
}

type vimCommandState struct {
	tab    *Tab
	prompt string
	input  string
	err    string
}

type vimSearchState struct {
	pattern     string
	replacement string
	forward     bool
	set         bool
}

type vimKey struct {
	stroke  string
	action  string
	handler string
}

var (
	vimClipboardRead  = mygo.Clipboard.ReadText
	vimClipboardWrite = mygo.Clipboard.WriteText
)

// vimKeys are IdeaVim's shortcut-conflict rows. The action column is derived
// from the effective GoIDEA keymap so user overrides show immediately.
var vimKeys = []vimKey{
	{"Escape", "Exit to Normal mode", "ide"},
	{"Ctrl+[", "Exit to Normal mode", "vim"},
	{"Ctrl+A", "Increment number", "vim"},
	{"Ctrl+B", "Page up", "vim"},
	{"Ctrl+C", "Exit to Normal mode", "ide"},
	{"Ctrl+D", "Scroll half page down / unindent", "vim"},
	{"Ctrl+E", "Scroll line down", "vim"},
	{"Ctrl+F", "Page down", "vim"},
	{"Ctrl+I", "Jump forward", "vim"},
	{"Ctrl+N", "Next line", "ide"},
	{"Ctrl+O", "Jump back / one Normal command", "vim"},
	{"Ctrl+P", "Previous line", "ide"},
	{"Ctrl+Q", "Visual block", "ide"},
	{"Ctrl+R", "Redo / insert register", "vim"},
	{"Ctrl+T", "Indent", "ide"},
	{"Ctrl+U", "Scroll half page up / delete to line start", "vim"},
	{"Ctrl+V", "Visual block", "vim"},
	{"Ctrl+W", "Delete word before caret", "vim"},
	{"Ctrl+X", "Decrement number", "vim"},
	{"Ctrl+Y", "Scroll line up", "vim"},
}

func init() {
	_ = registerAction(&Action{
		ID:       "vim.toggle",
		Title:    "Vim",
		Category: "Tools",
		Keywords: "vim ideavim emulator toggle vim mode",
		Toggled:  func(m *Model) bool { return m.prefs.Vim.Enabled },
		Run:      func(m *Model) { m.SetVimEnabled(!m.prefs.Vim.Enabled) },
	})
	_ = registerAction(&Action{
		ID:       "vim.reloadRc",
		Title:    "Reload Vim rc",
		Category: "Tools",
		Keywords: "vim vimrc goideavimrc ideavimrc",
		When:     func(m *Model) bool { return m.prefs.Vim.Enabled },
		Run:      func(m *Model) { m.reloadVimRc(false) },
	})
	_ = registerAction(&Action{
		ID:       "vim.settings",
		Title:    "Vim Settings…",
		Category: "Tools",
		Keywords: "vim shortcut conflicts",
		Run:      func(m *Model) { m.OpenSettingsPage(settingsPageVim) },
	})

	registerStatusItem(&StatusItem{
		ID:    "vim.mode",
		Title: "Vim",
		Side:  "right",
		Order: 45,
		Render: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) {
			m.vimStatus(c, p, oi)
		},
	})
}

// SetVimEnabled applies Vim to every open and future editor and persists the
// choice, as the status widget and Tools › Vim do.
func (m *Model) SetVimEnabled(enabled bool) {
	if m.prefs.Vim.Enabled == enabled {
		return
	}
	m.prefs.Vim.Enabled = enabled
	if enabled {
		m.ensureVimRc()
	}
	m.applyPreferences()
	m.savePreferences()
	m.invalidate()
}

func (m *Model) vimStatus(c *ui.Context, p *ui.Theme, oi oiColors) {
	enabled := m.prefs.Vim.Enabled
	label := "Vim"
	if enabled {
		if t := m.tab(); t != nil {
			label = m.vimStateFor(t).mode.label()
		} else {
			label = "NORMAL"
		}
	}
	tk := oiTokensFor(m, p)
	b := ui.ButtonBase(c.Key("status:vim")).
		Height(tk.StatusBarHeight-tk.SpaceL).
		Padding(0, tk.SpaceM).
		Radius(tk.RadiusS).
		AlignItems(ui.Center).
		Cursor(ui.CursorPointer).
		Tooltip(map[bool]string{true: "Vim mode", false: "Vim (disabled)"}[enabled])
	if b.Hovered() {
		b.Background(oi.toolbarHover)
	}
	b.Children(func() {
		ui.Text(c, label).FontSize(tk.FontSizeM).
			TextColor(map[bool]ui.Color{true: oi.text, false: oi.textMuted}[enabled]).
			SingleLine()
	})
	b.Menu(func(menu *ui.Menu) {
		if item := menu.Item("Enabled"); item.Checked(enabled).Chosen() {
			m.SetVimEnabled(!enabled)
		}
		menu.Separator()
		if item := menu.Item("Reload Vim rc").Disabled(!enabled); item.Chosen() {
			m.reloadVimRc(false)
		}
		if menu.Item("Settings…").Chosen() {
			m.OpenSettingsPage(settingsPageVim)
		}
	})
}

// applyVimPreferences brings every editor's Vim contribution in line with the
// persisted switch. Turning it off restores the Appearance page's cursor.
func (m *Model) applyVimPreferences() {
	if m.prefs.Vim.Enabled {
		m.ensureVimRc()
	}
	for _, t := range m.allTabs() {
		m.applyVimToTab(t)
	}
}

func (m *Model) applyVimToTab(t *Tab) {
	if t == nil || t.state == nil {
		return
	}
	if !m.prefs.Vim.Enabled {
		t.state.NoInputMethod = false
		t.state.CursorShape = editor.CursorBar
		if m.prefs.Editor.BlockCaret {
			t.state.CursorShape = editor.CursorBlock
		}
		return
	}
	st := m.vimStateFor(t)
	t.state.NoInputMethod = st.mode != vimInsert
	t.state.CursorShape = editor.CursorBlock
	if st.mode == vimInsert {
		t.state.CursorShape = editor.CursorBar
	}
}

func (m *Model) vimStateFor(t *Tab) *vimTabState {
	if m.vim.tabs == nil {
		m.vim.tabs = map[*Tab]*vimTabState{}
	}
	st := m.vim.tabs[t]
	if st == nil {
		st = &vimTabState{}
		m.vim.tabs[t] = st
	}
	return st
}

func (m *Model) vimTabClosed(t *Tab) {
	if t == nil {
		return
	}
	if st := m.vim.tabs[t]; st != nil {
		if st.recording != 0 {
			m.vimStopMacro(st)
		}
		st.mappingKeys = nil
		st.macroTokens = nil
		st.registerName = 0
		st.blockInsert = false
	}
	delete(m.vim.tabs, t)
	if m.vim.command != nil && m.vim.command.tab == t {
		m.vim.command = nil
	}
}

// vimInput is called before MyGo's editor input. Insert-mode text passes
// through, while Normal and Visual commands are consumed here.
func (m *Model) vimInput(t *Tab, ev ui.InputEvent) bool {
	if !m.prefs.Vim.Enabled || t == nil || t.state == nil {
		return false
	}
	st := m.vimStateFor(t)
	if ev.Kind == ui.InputText {
		if st.mode == vimInsert {
			handled := false
			for _, r := range ev.Text {
				if m.vimFeedToken(t, st, string(r), 0) {
					handled = true
				}
			}
			m.syncVimTab(t)
			return handled
		}
		handled := false
		for _, r := range ev.Text {
			if m.vimFeedToken(t, st, string(r), 0) {
				handled = true
			}
		}
		m.syncVimTab(t)
		return handled
	}
	if ev.Kind != ui.InputKeyDown {
		return false
	}
	token, printable := vimInputToken(ev)
	if token == "" {
		return false
	}
	if ev.Mods&(ui.Cmd|ui.Alt) != 0 && !m.vimMappingMayStart(st, token) {
		return false
	}
	if st.mode == vimInsert && token != "<Esc>" && token != "<Ctrl+C>" {
		if m.vimMappingMayStart(st, token) {
			return m.vimFeedToken(t, st, token, 0)
		}
		if !printable {
			m.vimRecordMacroToken(st, token)
		}
		return false
	}
	if printable {
		// The following InputText carries the layout's character.
		return true
	}
	if ev.Mods&ui.Ctrl != 0 && !m.vimHandlerSendsToVim(vimStroke(ev.Mods, ev.Key)) {
		return false
	}
	handled := m.vimFeedToken(t, st, token, 0)
	m.syncVimTab(t)
	return handled
}

func (m *Model) vimFeedToken(t *Tab, st *vimTabState, token string, depth int) bool {
	if depth > 12 {
		st.mappingKeys = nil
		m.status = "Vim: E223: Recursive mapping"
		return true
	}
	wasRecording := st.recording
	var handled bool
	if mapping, waiting, ok := m.takeVimMapping(st, token); waiting {
		handled = true
	} else if ok {
		handled = m.runVimMapping(t, st, mapping, depth+1)
	} else {
		handled = m.vimBuiltin(t, st, token, depth)
	}
	if wasRecording != 0 && st.recording == wasRecording &&
		(m.vim.command == nil || m.vim.command.tab != t) {
		m.vimRecordMacroToken(st, token)
	}
	return handled
}

func (m *Model) runVimMapping(t *Tab, st *vimTabState, mapping vimMapping, depth int) bool {
	if mapping.Ex != "" {
		m.runVimEx(t, mapping.Ex)
		m.syncVimTab(t)
		return true
	}
	for _, token := range mapping.RHS {
		m.vimFeedToken(t, st, token, depth)
	}
	m.syncVimTab(t)
	return true
}

func (m *Model) runVimEx(t *Tab, ex string) {
	if err := m.runVimExCommand(t, ex); err != nil {
		m.status = "Vim: " + err.Error()
		m.invalidate()
	}
}

func (m *Model) runVimExCommand(t *Tab, ex string) error {
	ex = strings.TrimSpace(ex)
	if strings.HasPrefix(ex, "action ") {
		id := strings.TrimSpace(strings.TrimPrefix(ex, "action "))
		if id == "" {
			return fmt.Errorf("usage: action <id>")
		}
		if !m.RunAction(id) {
			return fmt.Errorf("no action %q", id)
		}
		return nil
	}
	if handled, err := m.vimExecuteSubstitute(t, ex); handled {
		return err
	}
	switch ex {
	case "w", "write":
		if t == nil || t.doc == nil {
			return fmt.Errorf("no writable editor")
		}
		if t.kind == tabKindNotebook {
			m.saveNotebook(t)
		} else {
			m.saveTabAsyncMode(t, true, nil)
		}
		return nil
	case "wa", "wall":
		for _, tab := range m.allTabs() {
			if tab.kind == tabKindNotebook {
				m.saveNotebook(tab)
			} else if tab.doc != nil {
				m.saveTabAsync(tab, nil)
			}
		}
		return nil
	case "q", "quit":
		for i, candidate := range m.tabs {
			if candidate == t {
				m.closeTab(i)
				return nil
			}
		}
		return fmt.Errorf("no editor to close")
	case "qa", "qall":
		m.CloseAllTabs(false)
		return nil
	case "wq", "x":
		if t == nil || t.doc == nil {
			return fmt.Errorf("no writable editor")
		}
		if t.kind == tabKindNotebook {
			m.saveNotebook(t)
			for i, candidate := range m.tabs {
				if candidate == t {
					m.closeTab(i)
					break
				}
			}
			return nil
		}
		m.saveTabAsyncMode(t, true, func() {
			for i, candidate := range m.tabs {
				if candidate == t {
					m.closeTab(i)
					return
				}
			}
		})
		return nil
	case "":
		return nil
	default:
		return fmt.Errorf("unsupported command %q", ex)
	}
}

func (m *Model) vimBuiltin(t *Tab, st *vimTabState, token string, depth int) bool {
	switch st.mode {
	case vimInsert:
		if token == "<Esc>" || token == "<Ctrl+C>" || token == "<Ctrl-[>" {
			m.vimFinishInsert(t, st)
			return true
		}
		return false
	}

	if token == "<Esc>" {
		st.count = 0
		st.pending = ""
		st.operator = 0
		if st.mode.visual() {
			t.state.Select(st.visualHead, st.visualHead)
			st.mode = vimNormal
		}
		return true
	}
	if token == "<Ctrl+R>" {
		st.count = 0
		t.state.Run(editor.Redo)
		return true
	}
	if st.mode == vimVisualBlock {
		return m.vimVisualBlockCommand(t, st, token)
	}
	if st.mode.visual() {
		return m.vimVisualCommand(t, st, token)
	}
	return m.vimNormalCommand(t, st, token, depth)
}

func (m *Model) vimNormalCommand(t *Tab, st *vimTabState, token string, depth int) bool {
	if st.pending == "m" {
		st.pending = ""
		if !m.vimSetMark(t, st, token) && len(token) == 1 {
			m.status = "Vim: E191: Argument must be a letter"
		}
		return true
	}
	if st.pending == "'" {
		st.pending = ""
		return m.vimJumpMark(t, st, token, true)
	}
	if st.pending == "`" {
		st.pending = ""
		return m.vimJumpMark(t, st, token, false)
	}
	if st.pending == "q" {
		st.pending = ""
		return m.vimStartMacro(st, token)
	}
	if st.pending == "@" {
		st.pending = ""
		count := st.macroCount
		st.macroCount = 0
		return m.vimPlayMacro(t, st, token, max(1, count), depth)
	}
	if st.pending == "\"" {
		st.pending = ""
		if !m.vimSelectRegister(st, token) && len(token) == 1 {
			m.status = "Vim: E354: Invalid register name"
		}
		return true
	}
	if st.pending == "g" {
		st.pending = ""
		if token == "g" {
			t.state.Run(editor.MoveDocStart)
			return true
		}
		return true
	}
	if st.pending == "r" {
		st.pending = ""
		m.vimReplaceChar(t, token)
		return true
	}
	if st.recording != 0 && token == "q" {
		m.vimStopMacro(st)
		st.count = 0
		st.pending = ""
		st.operator = 0
		return true
	}
	if m.vimRejectReadOnlyRegister(st, token) {
		return true
	}
	if st.operator != 0 {
		return m.vimOperator(t, st, token)
	}
	if len(token) == 1 && token[0] >= '0' && token[0] <= '9' {
		if token == "0" && st.count == 0 {
			t.state.Run(editor.MoveLineStart)
			return true
		}
		st.count = st.count*10 + int(token[0]-'0')
		return true
	}

	count := st.takeCount()
	switch token {
	case "h", "<Left>", "<Backspace>":
		m.vimMove(t, "h", count)
	case "l", "<Right>", "<Space>":
		m.vimMove(t, "l", count)
	case "j", "<Down>":
		m.vimMove(t, "j", count)
	case "k", "<Up>":
		m.vimMove(t, "k", count)
	case "w", "W":
		m.vimMove(t, token, count)
	case "b", "B":
		m.vimMove(t, token, count)
	case "e", "E":
		m.vimMove(t, token, count)
	case "$", "<End>":
		t.state.Run(editor.MoveLineEnd)
	case "^":
		m.vimFirstNonBlank(t)
	case "0":
		t.state.Run(editor.MoveLineStart)
	case "G":
		if count > 1 {
			t.state.GoTo(editor.Pos{Line: min(count-1, t.state.Buffer.LineCount()-1)})
		} else {
			t.state.Run(editor.MoveDocEnd)
		}
	case "g":
		st.pending = "g"
	case "i":
		m.vimStartInsert(t, st, "i")
	case "a":
		m.vimStartInsert(t, st, "a")
	case "I":
		m.vimFirstNonBlank(t)
		m.vimStartInsert(t, st, "i")
	case "A":
		t.state.Run(editor.MoveLineEnd)
		m.vimStartInsert(t, st, "a")
	case "o":
		m.vimOpenLine(t, st, false)
	case "O":
		m.vimOpenLine(t, st, true)
	case "x", "<Delete>":
		m.vimDeleteChars(t, st, count, false)
	case "X":
		m.vimDeleteChars(t, st, count, true)
	case "d", "c", "y":
		st.operator = rune(token[0])
		st.operatorCount = count
	case "D":
		m.vimDeleteToLineEnd(t, true)
	case "C":
		m.vimDeleteToLineEnd(t, false)
		m.vimStartInsert(t, st, "i")
	case "Y":
		m.vimYankLines(t, st, count)
	case "p":
		m.vimPaste(t, st, true, count)
	case "P":
		m.vimPaste(t, st, false, count)
	case "u":
		t.state.Run(editor.Undo)
	case "J":
		m.vimJoinLines(t, count)
	case "~":
		m.vimToggleCaseAtCaret(t, count)
	case "r":
		st.pending = "r"
	case "s":
		m.vimDeleteChars(t, st, count, false)
		m.vimStartInsert(t, st, "i")
	case "S":
		m.vimChangeLines(t, st, count)
	case "v":
		m.vimStartVisual(t, st, false)
	case "V":
		m.vimStartVisual(t, st, true)
	case "<Ctrl+V>":
		m.vimStartVisualBlock(t, st)
	case ":":
		m.vimCommandLine(t, st, ":")
	case "/":
		m.vimCommandLine(t, st, "/")
	case "?":
		m.vimCommandLine(t, st, "?")
	case "n", "N":
		if err := m.vimRepeatSearch(t, st, count, token == "N"); err != nil {
			m.status = "Vim: " + err.Error()
			m.invalidate()
		}
	case "m":
		st.pending = "m"
	case "'":
		st.pending = "'"
	case "`":
		st.pending = "`"
	case "q":
		st.pending = "q"
	case "@":
		st.pending = "@"
		st.macroCount = count
	case "\"":
		st.pending = "\""
	case "<PageDown>", "<Ctrl+F>":
		t.state.Run(editor.MovePageDown)
	case "<PageUp>", "<Ctrl+B>":
		t.state.Run(editor.MovePageUp)
	case "<Ctrl+D>":
		t.state.Run(editor.MovePageDown)
	case "<Ctrl+U>":
		t.state.Run(editor.MovePageUp)
	default:
		if len(token) == 1 {
			// An unmapped printable key is consumed rather than typed.
			return true
		}
		return false
	}
	_ = depth
	return true
}

func (m *Model) vimOperator(t *Tab, st *vimTabState, token string) bool {
	op := st.operator
	count := st.operatorCount
	if count == 0 {
		count = 1
	}
	if st.operatorText {
		st.operatorText = false
		st.operator = 0
		st.operatorCount = 0
		start, end, linewise, ok := vimTextObjectRange(
			t.state.Buffer, t.state.Cursor(), token, st.operatorInner, count, nil)
		if !ok {
			return true
		}
		switch op {
		case 'd':
			m.vimYankKind(t, st, start, end, linewise, vimRegisterDelete)
			m.vimDeleteRange(t, start, end, linewise)
		case 'c':
			m.vimYankKind(t, st, start, end, linewise, vimRegisterDelete)
			m.vimDeleteRange(t, start, end, linewise)
			m.vimStartInsert(t, st, "i")
		case 'y':
			m.vimYank(t, st, start, end, linewise)
		}
		return true
	}
	if token == "i" || token == "a" {
		st.operatorText = true
		st.operatorInner = token == "i"
		return true
	}
	if token == string(op) {
		st.operator = 0
		st.operatorCount = 0
		switch op {
		case 'd':
			m.vimDeleteLines(t, count)
		case 'c':
			m.vimChangeLines(t, st, count)
		case 'y':
			m.vimYankLines(t, st, count)
		}
		return true
	}
	target, ok := vimMotionOffset(t, token, count)
	if !ok {
		st.operator = 0
		st.operatorCount = 0
		return true
	}
	start := min(t.state.Cursor(), target)
	end := max(t.state.Cursor(), target)
	if token == "e" || token == "E" || token == "$" {
		end = vimAfterRune(t.state.Buffer, end)
	}
	st.operator = 0
	st.operatorCount = 0
	switch op {
	case 'd':
		m.vimYankKind(t, st, start, end, false, vimRegisterDelete)
		m.vimDeleteRange(t, start, end, false)
	case 'c':
		m.vimYankKind(t, st, start, end, false, vimRegisterDelete)
		m.vimDeleteRange(t, start, end, false)
		m.vimStartInsert(t, st, "i")
	case 'y':
		m.vimYank(t, st, start, end, false)
	}
	return true
}

func (m *Model) vimStartVisual(t *Tab, st *vimTabState, linewise bool) {
	st.mode = vimVisual
	if linewise {
		st.mode = vimVisualLine
	}
	st.visualAnchor = t.state.Cursor()
	st.visualHead = st.visualAnchor
	m.vimUpdateVisual(t, st)
}

func (m *Model) vimVisualCommand(t *Tab, st *vimTabState, token string) bool {
	if st.visualText {
		st.visualText = false
		covered := &vimTextObjectCovered{
			start: min(st.visualAnchor, st.visualHead),
			end:   max(st.visualAnchor, st.visualHead) + 1,
		}
		start, end, linewise, ok := vimTextObjectRange(
			t.state.Buffer, st.visualHead, token, st.visualInner, 1, covered)
		if !ok {
			return true
		}
		if linewise {
			st.mode = vimVisualLine
		}
		st.visualAnchor = start
		st.visualHead = max(start, end)
		if !linewise {
			st.visualHead = max(start, vimRuneLeft(t.state.Buffer, end))
		}
		m.vimUpdateVisual(t, st)
		return true
	}
	if token == "i" || token == "a" {
		st.visualText = true
		st.visualInner = token == "i"
		return true
	}
	switch token {
	case "h", "<Left>":
		m.vimVisualMove(t, st, "h", 1)
	case "l", "<Right>":
		m.vimVisualMove(t, st, "l", 1)
	case "j", "<Down>":
		m.vimVisualMove(t, st, "j", 1)
	case "k", "<Up>":
		m.vimVisualMove(t, st, "k", 1)
	case "w", "W", "b", "B", "e", "E", "$", "^", "0", "G":
		m.vimVisualMove(t, st, token, 1)
	case "v":
		if st.mode == vimVisual {
			t.state.Select(st.visualHead, st.visualHead)
			st.mode = vimNormal
			return true
		}
		st.mode = vimVisual
		m.vimUpdateVisual(t, st)
	case "V":
		st.mode = vimVisualLine
		m.vimUpdateVisual(t, st)
	case "<Ctrl+V>":
		st.mode = vimVisualBlock
		m.vimUpdateVisualBlock(t, st)
	case "o":
		st.visualAnchor, st.visualHead = st.visualHead, st.visualAnchor
		m.vimUpdateVisual(t, st)
	case "d", "x":
		start, end := m.vimVisualRange(t, st)
		m.vimYankKind(t, st, start, end, st.mode == vimVisualLine, vimRegisterDelete)
		m.vimDeleteRange(t, start, end, st.mode == vimVisualLine)
		st.mode = vimNormal
	case "y":
		start, end := m.vimVisualRange(t, st)
		m.vimYank(t, st, start, end, st.mode == vimVisualLine)
		st.mode = vimNormal
	case "c":
		start, end := m.vimVisualRange(t, st)
		m.vimYankKind(t, st, start, end, st.mode == vimVisualLine, vimRegisterDelete)
		m.vimDeleteRange(t, start, end, st.mode == vimVisualLine)
		st.mode = vimNormal
		m.vimStartInsert(t, st, "i")
	case ">":
		t.state.Run(editor.Indent)
		st.mode = vimNormal
	case "<":
		t.state.Run(editor.Outdent)
		st.mode = vimNormal
	case "~":
		start, end := m.vimVisualRange(t, st)
		m.vimToggleCaseRange(t, start, end)
		st.mode = vimNormal
	default:
		if len(token) == 1 {
			return true
		}
		return false
	}
	return true
}

func (m *Model) vimVisualMove(t *Tab, st *vimTabState, token string, count int) {
	if target, ok := vimMotionOffsetAt(t, token, count, st.visualHead); ok {
		st.visualHead = target
		m.vimUpdateVisual(t, st)
	}
}

func (m *Model) vimUpdateVisual(t *Tab, st *vimTabState) {
	start, end := min(st.visualAnchor, st.visualHead), max(st.visualAnchor, st.visualHead)
	if st.mode == vimVisual {
		end = vimAfterRune(t.state.Buffer, end)
	} else {
		start, end = vimLineRange(t.state.Buffer, start, max(start, end-1))
	}
	t.state.Select(start, end)
}

func (m *Model) vimVisualRange(t *Tab, st *vimTabState) (int, int) {
	start, end := min(st.visualAnchor, st.visualHead), max(st.visualAnchor, st.visualHead)
	if st.mode == vimVisual {
		end = vimAfterRune(t.state.Buffer, end)
	} else {
		start, end = vimLineRange(t.state.Buffer, start, max(start, end-1))
	}
	return start, end
}

func (m *Model) vimMove(t *Tab, token string, count int) {
	if target, ok := vimMotionOffset(t, token, count); ok {
		t.state.Select(target, target)
	}
}

func vimMotionOffset(t *Tab, token string, count int) (int, bool) {
	if t == nil || t.state == nil || t.state.Buffer == nil {
		return 0, false
	}
	return vimMotionOffsetAt(t, token, count, t.state.Cursor())
}

func vimMotionOffsetAt(t *Tab, token string, count, pos int) (int, bool) {
	if t == nil || t.state == nil || t.state.Buffer == nil {
		return 0, false
	}
	if count < 1 {
		count = 1
	}
	b := t.state.Buffer
	switch token {
	case "h":
		for range count {
			pos = vimRuneLeft(b, pos)
		}
	case "l", "<Space>":
		for range count {
			pos = vimRuneRight(b, pos)
		}
	case "j", "k":
		p := b.Pos(pos)
		line := p.Line
		if token == "j" {
			line = min(b.LineCount()-1, line+count)
		} else {
			line = max(0, line-count)
		}
		pos = b.Offset(editor.Pos{Line: line, Col: p.Col})
	case "w", "W":
		for range count {
			pos = vimWordForward(b, pos, token == "W")
		}
	case "b", "B":
		for range count {
			pos = vimWordBack(b, pos, token == "B")
		}
	case "e", "E":
		for range count {
			pos = vimWordEnd(b, pos, token == "E")
		}
	case "0":
		pos = b.Offset(editor.Pos{Line: b.Pos(pos).Line})
	case "^":
		pos = vimFirstNonBlankOffset(b, b.Pos(pos).Line)
	case "$":
		line := b.Pos(pos).Line
		pos = b.Offset(editor.Pos{Line: line, Col: len(b.Line(line))})
	case "G":
		if count > 1 {
			pos = b.Offset(editor.Pos{Line: min(count-1, b.LineCount()-1)})
		} else {
			pos = b.Len()
		}
	default:
		return 0, false
	}
	return pos, true
}

func (m *Model) vimFirstNonBlank(t *Tab) {
	p := t.state.Buffer.Pos(t.state.Cursor())
	t.state.Select(vimFirstNonBlankOffset(t.state.Buffer, p.Line), vimFirstNonBlankOffset(t.state.Buffer, p.Line))
}

func (m *Model) vimStartInsert(t *Tab, st *vimTabState, at string) {
	if at == "a" {
		pos := vimRuneRight(t.state.Buffer, t.state.Cursor())
		t.state.Select(pos, pos)
	}
	st.mode = vimInsert
	st.count = 0
	st.pending = ""
	st.operator = 0
}

func (m *Model) vimFinishInsert(t *Tab, st *vimTabState) {
	st.mode = vimNormal
	p := t.state.Buffer.Pos(t.state.Cursor())
	lineStart := t.state.Buffer.Offset(editor.Pos{Line: p.Line})
	if t.state.Cursor() > lineStart {
		pos := vimRuneLeft(t.state.Buffer, t.state.Cursor())
		t.state.Select(pos, pos)
	}
	if st.blockInsert {
		pos := min(t.state.Cursor(), t.state.Buffer.Len())
		t.state.Select(pos, pos)
		st.blockInsert = false
	}
}

func (m *Model) vimOpenLine(t *Tab, st *vimTabState, above bool) {
	b := t.state.Buffer
	p := b.Pos(t.state.Cursor())
	offset := b.Offset(editor.Pos{Line: p.Line})
	if above {
		b.Edit(editor.Edit{Start: offset, End: offset, Text: "\n"})
		t.state.Select(offset, offset)
	} else {
		lineEnd := b.Offset(editor.Pos{Line: p.Line, Col: len(b.Line(p.Line))})
		b.Edit(editor.Edit{Start: lineEnd, End: lineEnd, Text: "\n"})
		t.state.Select(lineEnd+1, lineEnd+1)
	}
	st.mode = vimInsert
}

func (m *Model) vimDeleteChars(t *Tab, st *vimTabState, count int, before bool) {
	pos := t.state.Cursor()
	start, end := pos, pos
	if before {
		for range max(1, count) {
			start = vimRuneLeft(t.state.Buffer, start)
		}
	} else {
		for range max(1, count) {
			end = vimRuneRight(t.state.Buffer, end)
		}
	}
	m.vimYankKind(t, st, start, end, false, vimRegisterDelete)
	m.vimDeleteRange(t, start, end, false)
}

func (m *Model) vimDeleteToLineEnd(t *Tab, yank bool) {
	b := t.state.Buffer
	start := t.state.Cursor()
	end := b.Offset(editor.Pos{Line: b.Pos(start).Line, Col: len(b.Line(b.Pos(start).Line))})
	if end == start && end < b.Len() {
		end++
	}
	st := m.vimStateFor(t)
	m.vimYankKind(t, st, start, end, false, vimRegisterDelete)
	_ = yank
	m.vimDeleteRange(t, start, end, false)
}

func (m *Model) vimDeleteLines(t *Tab, count int) {
	st := m.vimStateFor(t)
	start, end := vimLinesRange(t.state.Buffer, t.state.Cursor(), count)
	m.vimYankKind(t, st, start, end, true, vimRegisterDelete)
	m.vimDeleteRange(t, start, end, true)
}

func (m *Model) vimChangeLines(t *Tab, st *vimTabState, count int) {
	start, end := vimLinesRange(t.state.Buffer, t.state.Cursor(), count)
	m.vimYankKind(t, st, start, end, true, vimRegisterDelete)
	m.vimDeleteRange(t, start, end, true)
	st.mode = vimInsert
}

func (m *Model) vimYankLines(t *Tab, st *vimTabState, count int) {
	start, end := vimLinesRange(t.state.Buffer, t.state.Cursor(), count)
	m.vimYank(t, st, start, end, true)
}

func (m *Model) vimYank(t *Tab, st *vimTabState, start, end int, linewise bool) {
	m.vimYankKind(t, st, start, end, linewise, vimRegisterYank)
}

func (m *Model) vimYankKind(t *Tab, st *vimTabState, start, end int, linewise bool, write vimRegisterWrite) {
	start, end = max(0, start), min(t.state.Buffer.Len(), end)
	if end < start {
		start, end = end, start
	}
	text := t.state.Buffer.Slice(start, end)
	m.vimStoreRegister(t, st, vimRegister{text: text, linewise: linewise, set: true}, write)
}

func (m *Model) vimDeleteRange(t *Tab, start, end int, linewise bool) {
	b := t.state.Buffer
	start, end = max(0, start), min(b.Len(), end)
	if end <= start {
		return
	}
	if linewise && end == b.Len() && start > 0 && b.Slice(end-1, end) != "\n" {
		start = max(0, start-1)
	}
	b.Edit(editor.Edit{Start: start, End: end})
	target := min(start, b.Len())
	p := b.Pos(target)
	lineStart := b.Offset(editor.Pos{Line: p.Line})
	if target > lineStart {
		target = vimRuneLeft(b, target)
	}
	t.state.Select(target, target)
}

func (m *Model) vimPaste(t *Tab, st *vimTabState, after bool, count int) {
	reg, ok := m.vimRegisterFor(t, st)
	if !ok {
		return
	}
	if reg.text == "" {
		return
	}
	if reg.block {
		m.vimPasteBlock(t, st, reg, after, count)
		return
	}
	text := strings.Repeat(reg.text, max(1, count))
	b := t.state.Buffer
	pos := t.state.Cursor()
	if reg.linewise {
		line := b.Pos(pos).Line
		if after {
			line++
		}
		offset := b.Offset(editor.Pos{Line: min(line, b.LineCount())})
		if offset == b.Len() && offset > 0 && b.Slice(offset-1, offset) != "\n" {
			text = "\n" + strings.TrimSuffix(text, "\n")
		}
		b.Edit(editor.Edit{Start: offset, End: offset, Text: text})
		t.state.Select(offset, offset)
		return
	}
	if after {
		pos = vimRuneRight(b, pos)
	}
	b.Edit(editor.Edit{Start: pos, End: pos, Text: text})
	target := vimRuneLeft(b, pos+len(text))
	t.state.Select(target, target)
}

// vimPasteBlock puts a visual-block register down each target line at the
// caret's display column. p starts after the current character and P starts
// before it; count repeats each register row horizontally, as Vim does.
func (m *Model) vimPasteBlock(t *Tab, st *vimTabState, reg vimRegister, after bool, count int) {
	b := t.state.Buffer
	cursor := t.state.Cursor()
	startLine := b.Pos(cursor).Line
	startText := b.Line(startLine)
	lineStart := b.Offset(editor.Pos{Line: startLine})
	column := vimDisplayColumnAt(startText, max(0, cursor-lineStart), 4)
	if after && column < vimDisplayColumnAt(startText, len(startText), 4) {
		start, end := vimDisplayCellSpan(startText, cursor-lineStart, 4)
		column += max(1, end-start)
	}

	rows := strings.Split(strings.TrimSuffix(reg.text, "\n"), "\n")
	count = max(1, count)
	edits := make([]editor.Edit, 0, len(rows)+2)
	first := -1
	lastExisting := b.LineCount() - 1
	for i, row := range rows {
		line := startLine + i
		insert := strings.Repeat(row, count)
		if line > lastExisting {
			continue
		}
		text := b.Line(line)
		targetColumn := min(column, vimDisplayColumnAt(text, len(text), 4))
		padding := max(0, column-targetColumn)
		offset := b.Offset(editor.Pos{
			Line: line,
			Col:  vimDisplayByteOffset(text, targetColumn, 4),
		})
		if first < 0 {
			first = offset + padding
		}
		edits = append(edits, editor.Edit{
			Start: offset, End: offset,
			Text: strings.Repeat(" ", padding) + insert,
		})
	}

	if missing := startLine + len(rows) - b.LineCount(); missing > 0 {
		var tail strings.Builder
		if b.Len() > 0 && b.Slice(b.Len()-1, b.Len()) != "\n" {
			tail.WriteByte('\n')
		}
		for i := len(rows) - missing; i < len(rows); i++ {
			if tail.Len() > 0 && i > len(rows)-missing {
				tail.WriteByte('\n')
			}
			tail.WriteString(strings.Repeat(" ", column))
			tail.WriteString(strings.Repeat(rows[i], count))
		}
		if first < 0 {
			first = b.Len()
			if tail.Len() > 0 && b.Len() > 0 && b.Slice(b.Len()-1, b.Len()) != "\n" {
				first++
			}
		}
		edits = append(edits, editor.Edit{Start: b.Len(), End: b.Len(), Text: tail.String()})
	}
	if len(edits) == 0 {
		return
	}
	sort.SliceStable(edits, func(i, j int) bool {
		if edits[i].Start == edits[j].Start {
			return i > j
		}
		return edits[i].Start > edits[j].Start
	})
	b.Edit(edits...)
	if first < 0 {
		first = b.Len()
	}
	t.state.Select(min(first, b.Len()), min(first, b.Len()))
}

func (m *Model) vimReplaceChar(t *Tab, token string) {
	r, size := utf8.DecodeRuneInString(token)
	if r == utf8.RuneError && size == 0 {
		return
	}
	pos := t.state.Cursor()
	end := vimRuneRight(t.state.Buffer, pos)
	if end == pos {
		return
	}
	t.state.Buffer.Edit(editor.Edit{Start: pos, End: end, Text: string(r)})
	t.state.Select(pos, pos)
}

func (m *Model) vimJoinLines(t *Tab, count int) {
	count = max(2, count)
	for range count - 1 {
		b := t.state.Buffer
		p := b.Pos(t.state.Cursor())
		if p.Line+1 >= b.LineCount() {
			break
		}
		lineEnd := b.Offset(editor.Pos{Line: p.Line, Col: len(b.Line(p.Line))})
		nextStart := lineEnd + 1
		nextText := strings.TrimLeft(b.Line(p.Line+1), " \t")
		insert := ""
		if len(nextText) > 0 {
			insert = " "
		}
		b.Edit(editor.Edit{Start: lineEnd, End: nextStart, Text: insert})
		t.state.Select(lineEnd, lineEnd)
	}
}

func (m *Model) vimToggleCaseAtCaret(t *Tab, count int) {
	start := t.state.Cursor()
	end := start
	for range max(1, count) {
		end = vimRuneRight(t.state.Buffer, end)
	}
	m.vimToggleCaseRange(t, start, end)
}

func (m *Model) vimToggleCaseRange(t *Tab, start, end int) {
	if end <= start {
		return
	}
	text := t.state.Buffer.Slice(start, end)
	toggled := []rune(text)
	for i, r := range toggled {
		if unicode.IsUpper(r) {
			toggled[i] = unicode.ToLower(r)
		} else if unicode.IsLower(r) {
			toggled[i] = unicode.ToUpper(r)
		}
	}
	t.state.Buffer.Edit(editor.Edit{Start: start, End: end, Text: string(toggled)})
	t.state.Select(start, start)
}

func (m *Model) vimCommandLine(t *Tab, st *vimTabState, prompt string) {
	st.count = 0
	st.pending = ""
	st.operator = 0
	if prompt == "" {
		prompt = ":"
	}
	m.vim.command = &vimCommandState{tab: t, prompt: prompt}
	m.invalidate()
}

func (m *Model) submitVimCommand() {
	command := m.vim.command
	if command == nil {
		return
	}
	command.err = ""
	var err error
	switch command.prompt {
	case "/", "?":
		err = m.vimExecuteSearch(command.tab, m.vimStateFor(command.tab), command.prompt, command.input)
	default:
		err = m.runVimExCommand(command.tab, command.input)
	}
	if err != nil {
		command.err = err.Error()
		if command.prompt == ":" && !strings.HasPrefix(command.err, "E") {
			command.err = "E492: " + command.err
		}
		m.invalidate()
		return
	}
	if st := m.vim.tabs[command.tab]; st != nil {
		m.vimRecordMacroCommand(st, command)
	}
	if command.prompt == ":" {
		m.vim.lastEx = command.input
	}
	m.vim.command = nil
	m.invalidate()
}

func (m *Model) cancelVimCommand() {
	if m.vim.command == nil {
		return
	}
	m.vim.command = nil
	m.invalidate()
}

func (m *Model) vimCommandDialog(c *ui.Context, p *ui.Theme) {
	command := m.vim.command
	if command == nil {
		return
	}
	open := true
	dlg := nativeModal(c.Key("vim:command"), &open, func() {
		ui.Column(c).Width(520).Gap(7).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				prompt := command.prompt
				if prompt == "" {
					prompt = ":"
				}
				ui.Text(c, prompt).Font(oiFontMono).FontSize(13).TextColor(p.Text)
				input := ui.TextInput(c, &command.input).AutoFocus()
				if input.Submitted() {
					m.submitVimCommand()
				}
			})
			if command.err != "" {
				ui.Text(c, command.err).FontSize(11).TextColor(p.Danger).
					SingleLine().Ellipsis("…")
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "Run").Clicked() {
					m.submitVimCommand()
				}
				if ui.Button(c, "Cancel").Clicked() {
					m.cancelVimCommand()
				}
			})
		})
	})
	if dlg.Dismissed() || !open {
		m.cancelVimCommand()
	}
}

func (m *Model) syncVimTab(t *Tab) {
	m.applyVimToTab(t)
	m.invalidate()
}

func (m *Model) vimHandlerFor(stroke string) string {
	stroke = keymapNormalize(stroke)
	if handler := m.prefs.Vim.Handlers[stroke]; handler == "vim" || handler == "ide" {
		return handler
	}
	for _, key := range vimKeys {
		if keymapNormalize(key.stroke) == stroke {
			return key.handler
		}
	}
	return ""
}

func (m *Model) vimHandlerSendsToVim(stroke string) bool {
	return m.prefs.Vim.Enabled && m.activeEditorAvailable() && m.vimHandlerFor(stroke) == "vim"
}

// vimClaimsShortcut keeps a conflict assigned to Vim out of the IDE shortcut
// chain while an editor owns the keyboard. The same key event then reaches the
// editor's OnInput callback, where vimInput consumes it.
func (m *Model) vimClaimsShortcut(c *ui.Context) bool {
	t := m.tab()
	if !m.prefs.Vim.Enabled || t == nil || !t.editorFocused {
		return false
	}
	for _, key := range vimKeys {
		if m.vimHandlerFor(key.stroke) != "vim" {
			continue
		}
		mods, code, ok := vimShortcutInput(key.stroke)
		if ok && c.Shortcut(mods, code) {
			return true
		}
	}
	return false
}

func vimShortcutInput(stroke string) (ui.Modifiers, ui.Key, bool) {
	parts := strings.Split(keymapNormalize(stroke), "+")
	if len(parts) == 0 {
		return 0, 0, false
	}
	var mods ui.Modifiers
	for _, part := range parts[:len(parts)-1] {
		switch part {
		case "Ctrl":
			mods |= ui.Ctrl
		case "Alt":
			mods |= ui.Alt
		case "Shift":
			mods |= ui.Shift
		case "Cmd":
			mods |= ui.Cmd
		default:
			return 0, 0, false
		}
	}
	name := parts[len(parts)-1]
	if len(name) == 1 && name[0] >= 'A' && name[0] <= 'Z' {
		return mods, ui.KeyA + ui.Key(name[0]-'A'), true
	}
	if len(name) == 1 && name[0] >= '0' && name[0] <= '9' {
		return mods, ui.Key0 + ui.Key(name[0]-'0'), true
	}
	keys := map[string]ui.Key{
		"Escape":    ui.KeyEscape,
		"Enter":     ui.KeyEnter,
		"Space":     ui.KeySpace,
		"Tab":       ui.KeyTab,
		"Backspace": ui.KeyBackspace,
		"Delete":    ui.KeyDelete,
		"Home":      ui.KeyHome,
		"End":       ui.KeyEnd,
		"PageUp":    ui.KeyPageUp,
		"PageDown":  ui.KeyPageDown,
		"Left":      ui.KeyLeft,
		"Right":     ui.KeyRight,
		"Up":        ui.KeyUp,
		"Down":      ui.KeyDown,
		"[":         ui.KeyBracketLeft,
		"]":         ui.KeyBracketRight,
		"/":         ui.KeySlash,
		`\`:         ui.KeyBackslash,
		";":         ui.KeySemicolon,
		"'":         ui.KeyQuote,
		"-":         ui.KeyMinus,
		"=":         ui.KeyEqual,
		",":         ui.KeyComma,
		".":         ui.KeyPeriod,
		"`":         ui.KeyBackquote,
	}
	key, ok := keys[name]
	return mods, key, ok
}

// shortcut is the Vim-aware replacement for Context.Shortcut in the global
// shortcut chain. A conflict row assigned to Vim never registers the IDE
// shortcut, so MyGo delivers the keystroke to editor.State.OnInput.
func (m *Model) shortcut(c *ui.Context, mods ui.Modifiers, key ui.Key) bool {
	if m.vimHandlerSendsToVim(vimStroke(mods, key)) {
		return false
	}
	return c.Shortcut(mods, key)
}

func vimStroke(mods ui.Modifiers, key ui.Key) string {
	parts := make([]string, 0, 4)
	if mods&ui.Ctrl != 0 {
		parts = append(parts, "Ctrl")
	}
	if mods&ui.Alt != 0 {
		parts = append(parts, "Alt")
	}
	if mods&ui.Shift != 0 {
		parts = append(parts, "Shift")
	}
	if mods&ui.Super != 0 {
		parts = append(parts, "Cmd")
	}
	parts = append(parts, vimKeyName(key))
	return keymapNormalize(strings.Join(parts, "+"))
}

func vimKeyName(key ui.Key) string {
	switch key {
	case ui.KeyEscape:
		return "Escape"
	case ui.KeyBackspace:
		return "Backspace"
	case ui.KeyDelete:
		return "Delete"
	case ui.KeyEnter:
		return "Enter"
	case ui.KeySpace:
		return "Space"
	case ui.KeyTab:
		return "Tab"
	case ui.KeyUp:
		return "Up"
	case ui.KeyDown:
		return "Down"
	case ui.KeyLeft:
		return "Left"
	case ui.KeyRight:
		return "Right"
	case ui.KeyHome:
		return "Home"
	case ui.KeyEnd:
		return "End"
	case ui.KeyPageUp:
		return "PageUp"
	case ui.KeyPageDown:
		return "PageDown"
	case ui.KeyBracketLeft:
		return "["
	case ui.KeyBracketRight:
		return "]"
	case ui.KeySlash:
		return "/"
	case ui.KeyBackslash:
		return "\\"
	case ui.KeySemicolon:
		return ";"
	case ui.KeyQuote:
		return "'"
	case ui.KeyMinus:
		return "-"
	case ui.KeyEqual:
		return "="
	case ui.KeyComma:
		return ","
	case ui.KeyPeriod:
		return "."
	case ui.KeyBackquote:
		return "`"
	}
	if key >= ui.KeyA && key <= ui.KeyZ {
		return string(rune('A' + key - ui.KeyA))
	}
	if key >= ui.Key0 && key <= ui.Key9 {
		return string(rune('0' + key - ui.Key0))
	}
	return fmt.Sprintf("Key%d", key)
}

func vimInputToken(ev ui.InputEvent) (string, bool) {
	if ev.Mods == 0 {
		switch ev.Key {
		case ui.KeySpace:
			return "<Space>", false
		case ui.KeyEscape:
			return "<Esc>", false
		case ui.KeyBackspace:
			return "<Backspace>", false
		case ui.KeyDelete:
			return "<Delete>", false
		case ui.KeyEnter:
			return "<Enter>", false
		case ui.KeyTab:
			return "<Tab>", false
		case ui.KeyLeft:
			return "<Left>", false
		case ui.KeyRight:
			return "<Right>", false
		case ui.KeyUp:
			return "<Up>", false
		case ui.KeyDown:
			return "<Down>", false
		case ui.KeyHome:
			return "<Home>", false
		case ui.KeyEnd:
			return "<End>", false
		case ui.KeyPageUp:
			return "<PageUp>", false
		case ui.KeyPageDown:
			return "<PageDown>", false
		}
	}
	if ev.Mods != 0 {
		if ev.Mods == ui.Shift {
			if name, special := vimShiftedSpecialKey(ev.Key); special {
				return "<Shift+" + name + ">", false
			}
			return "", false
		}
		if ev.Key == ui.KeyBracketLeft {
			if ev.Mods == ui.Ctrl {
				return "<Ctrl-[>", false
			}
		}
		stroke := vimStroke(ev.Mods, ev.Key)
		return "<" + stroke + ">", false
	}
	if ev.Mods == 0 {
		switch {
		case ev.Key >= ui.KeyA && ev.Key <= ui.KeyZ:
			return string(rune('a' + ev.Key - ui.KeyA)), true
		case ev.Key >= ui.Key0 && ev.Key <= ui.Key9:
			return string(rune('0' + ev.Key - ui.Key0)), true
		}
		switch ev.Key {
		case ui.KeySlash:
			return "/", true
		case ui.KeySemicolon:
			return ";", true
		case ui.KeyQuote:
			return "'", true
		case ui.KeyMinus:
			return "-", true
		case ui.KeyEqual:
			return "=", true
		case ui.KeyComma:
			return ",", true
		case ui.KeyPeriod:
			return ".", true
		case ui.KeyBackslash:
			return "\\", true
		case ui.KeyBackquote:
			return "`", true
		case ui.KeyBracketLeft:
			return "[", true
		case ui.KeyBracketRight:
			return "]", true
		}
	}
	return "", false
}

func vimShiftedSpecialKey(key ui.Key) (string, bool) {
	switch key {
	case ui.KeySpace:
		return "Space", true
	case ui.KeyEscape:
		return "Esc", true
	case ui.KeyBackspace:
		return "Backspace", true
	case ui.KeyDelete:
		return "Delete", true
	case ui.KeyEnter:
		return "Enter", true
	case ui.KeyTab:
		return "Tab", true
	case ui.KeyLeft:
		return "Left", true
	case ui.KeyRight:
		return "Right", true
	case ui.KeyUp:
		return "Up", true
	case ui.KeyDown:
		return "Down", true
	case ui.KeyHome:
		return "Home", true
	case ui.KeyEnd:
		return "End", true
	case ui.KeyPageUp:
		return "PageUp", true
	case ui.KeyPageDown:
		return "PageDown", true
	}
	return "", false
}

func vimRuneLeft(b *editor.Buffer, pos int) int {
	if pos <= 0 {
		return 0
	}
	for pos > 0 && !utf8.RuneStart(b.Slice(pos-1, pos)[0]) {
		pos--
	}
	if pos > 0 {
		pos--
	}
	return pos
}

func vimRuneRight(b *editor.Buffer, pos int) int {
	if pos >= b.Len() {
		return b.Len()
	}
	_, size := utf8.DecodeRuneInString(b.Slice(pos, b.Len()))
	if size <= 0 {
		size = 1
	}
	return min(b.Len(), pos+size)
}

func vimAfterRune(b *editor.Buffer, pos int) int {
	if pos >= b.Len() {
		return b.Len()
	}
	line := b.Pos(pos).Line
	lineEnd := b.Offset(editor.Pos{Line: line, Col: len(b.Line(line))})
	if pos >= lineEnd {
		return min(b.Len(), pos+1)
	}
	return vimRuneRight(b, pos)
}

func vimFirstNonBlankOffset(b *editor.Buffer, line int) int {
	text := b.Line(line)
	col := 0
	for col < len(text) {
		r, size := utf8.DecodeRuneInString(text[col:])
		if r != ' ' && r != '\t' {
			break
		}
		col += size
	}
	return b.Offset(editor.Pos{Line: line, Col: col})
}

func vimLinesRange(b *editor.Buffer, pos, count int) (int, int) {
	line := b.Pos(pos).Line
	endLine := min(b.LineCount()-1, line+max(1, count)-1)
	start := b.Offset(editor.Pos{Line: line})
	end := b.Offset(editor.Pos{Line: min(b.LineCount(), endLine+1)})
	if endLine == b.LineCount()-1 && end == start {
		end = b.Len()
	}
	return start, end
}

func vimLineRange(b *editor.Buffer, a, z int) (int, int) {
	first, last := b.Pos(min(a, z)).Line, b.Pos(max(a, z)).Line
	return b.Offset(editor.Pos{Line: first}), b.Offset(editor.Pos{Line: min(b.LineCount(), last+1)})
}

func vimWordForward(b *editor.Buffer, pos int, big bool) int {
	text := b.String()
	if pos >= len(text) {
		return len(text)
	}
	class := vimCharClassAt(text, pos, big)
	for pos < len(text) && vimCharClassAt(text, pos, big) == class && class != 0 {
		pos = vimRuneRight(b, pos)
	}
	for pos < len(text) && vimCharClassAt(text, pos, big) == 0 {
		pos = vimRuneRight(b, pos)
	}
	return pos
}

func vimWordBack(b *editor.Buffer, pos int, big bool) int {
	text := b.String()
	for pos > 0 && vimCharClassAt(text, vimRuneLeft(b, pos), big) == 0 {
		pos = vimRuneLeft(b, pos)
	}
	if pos == 0 {
		return 0
	}
	class := vimCharClassAt(text, vimRuneLeft(b, pos), big)
	for pos > 0 && vimCharClassAt(text, vimRuneLeft(b, pos), big) == class {
		pos = vimRuneLeft(b, pos)
	}
	return pos
}

func vimWordEnd(b *editor.Buffer, pos int, big bool) int {
	text := b.String()
	pos = vimRuneRight(b, pos)
	for pos < len(text) && vimCharClassAt(text, pos, big) == 0 {
		pos = vimRuneRight(b, pos)
	}
	if pos >= len(text) {
		return len(text)
	}
	class := vimCharClassAt(text, pos, big)
	for vimRuneRight(b, pos) < len(text) && vimCharClassAt(text, vimRuneRight(b, pos), big) == class {
		pos = vimRuneRight(b, pos)
	}
	return pos
}

func vimCharClassAt(text string, pos int, big bool) int {
	if pos < 0 || pos >= len(text) {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(text[pos:])
	switch {
	case unicode.IsSpace(r):
		return 0
	case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_':
		return 1
	case !big && !unicode.IsPrint(r):
		return 0
	default:
		return 2
	}
}

func (st *vimTabState) takeCount() int {
	count := st.count
	st.count = 0
	if count == 0 {
		return 1
	}
	return count
}
