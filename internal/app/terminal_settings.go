package app

import (
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/egoist/mygo/ui"
)

func (m *Model) terminalStartDirectory(root string) string {
	dir := strings.TrimSpace(m.nativeRuntimeString(settingsPageTerminal, "terminal.startDirectory"))
	if dir == "" {
		return root
	}
	if dir == "~" || strings.HasPrefix(dir, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			dir = filepath.Join(home, strings.TrimPrefix(dir, "~/"))
		}
	}
	if filepath.IsAbs(dir) {
		return filepath.Clean(dir)
	}
	return filepath.Clean(filepath.Join(root, strings.TrimPrefix(dir, "./")))
}

func (m *Model) terminalSpawnOptions(
	program string,
	args []string,
) map[string]any {
	if program == "" {
		program, args = splitTerminalShellCommand(
			m.nativeRuntimeString(settingsPageTerminal, "terminal.shellPath"),
		)
	}
	cols, rows := m.term.cols, m.term.rows
	if cols <= 0 {
		cols = defaultTerminalCols
	}
	if rows <= 0 {
		rows = defaultTerminalRows
	}
	opts := map[string]any{
		"cwd":  "",
		"args": append([]string{}, args...),
		"env":  m.nativeRuntimeStringMap(settingsPageTerminal, "terminal.env"),
		"cols": cols,
		"rows": rows,
		"shellIntegration": m.nativeRuntimeBool(
			settingsPageTerminal,
			"terminal.shellIntegration",
		),
	}
	if program != "" {
		opts["shell"] = program
	}
	return opts
}

func (m *Model) terminalDefaultTabName() string {
	name := strings.TrimSpace(m.nativeRuntimeString(settingsPageTerminal, "terminal.tabName"))
	if name == "" {
		return "Local"
	}
	return name
}

func (m *Model) terminalFont() (string, float32, float32) {
	family := strings.TrimSpace(m.nativeRuntimeString(settingsPageTerminal, "terminal.fontFamily"))
	size := float32(m.nativeRuntimeNumber(settingsPageTerminal, "terminal.fontSize"))
	lineHeight := float32(m.nativeRuntimeNumber(settingsPageTerminal, "terminal.lineHeight"))
	fallback := strings.TrimSpace(
		m.nativeRuntimeString(settingsPageTerminal, "terminal.fallbackFontFamily"),
	)
	if m.nativeRuntimeBool(settingsPageTerminal, "terminal.useEditorFont") {
		family = m.prefs.Font.fontFamily()
		size = m.editorFontSize()
	}
	if family == "" {
		family = oiFontMono
	}
	if fallback != "" {
		family += ", " + fallback
	}
	if size < 8 || size > 72 {
		size = 13
	}
	if lineHeight < 0.5 || lineHeight > 3 {
		lineHeight = 1
	}
	return family, size, lineHeight
}

func (m *Model) terminalTextStyle() ui.Font {
	family, size, lineHeight := m.terminalFont()
	return ui.Font{
		Family:     family,
		Size:       size,
		LineHeight: lineHeight,
	}
}

func (m *Model) terminalClosesOnExit() bool {
	return m.nativeRuntimeBool(settingsPageTerminal, "terminal.closeOnExit")
}

func (m *Model) terminalPlaysBell() bool {
	return m.nativeRuntimeBool(settingsPageTerminal, "terminal.audibleBell")
}

func (m *Model) terminalCopyOnSelection() bool {
	return m.nativeRuntimeBool(settingsPageTerminal, "terminal.copyOnSelection")
}

func (m *Model) terminalPasteOnMiddleClick() bool {
	return m.nativeRuntimeBool(settingsPageTerminal, "terminal.pasteOnMiddleClick")
}

func (m *Model) terminalOptionAsMeta() bool {
	return m.nativeRuntimeBool(settingsPageTerminal, "terminal.optionAsMeta")
}

func (m *Model) terminalEscapeToEditor() bool {
	return m.nativeRuntimeBool(settingsPageTerminal, "terminal.escapeToEditor")
}

func (m *Model) terminalMouseReporting() bool {
	return m.nativeRuntimeBool(settingsPageTerminal, "terminal.mouseReporting")
}

func (m *Model) terminalHighlightHyperlinks() bool {
	return m.nativeRuntimeBool(settingsPageTerminal, "terminal.highlightHyperlinks")
}

func (m *Model) terminalCursorBlink() bool {
	return m.nativeRuntimeBool(settingsPageTerminal, "terminal.cursorBlink")
}

func (m *Model) terminalCursorShape() string {
	shape := strings.TrimSpace(
		m.nativeRuntimeString(settingsPageTerminal, "terminal.cursorShape"),
	)
	switch shape {
	case "block", "underline", "bar":
		return shape
	default:
		return "block"
	}
}

func (m *Model) terminalColumnWidth() float32 {
	width := float32(m.nativeRuntimeNumber(settingsPageTerminal, "terminal.columnWidth"))
	if width < 0.5 || width > 3 {
		return 1
	}
	return width
}

func (m *Model) terminalColumnSpacing() float32 {
	_, size, _ := m.terminalFont()
	spacing := (m.terminalColumnWidth() - 1) * size * 0.6
	return float32(math.Round(float64(spacing)*10)) / 10
}

// splitTerminalShellCommand parses the shell command line accepted by
// Settings › Terminal. Single and double quotes group a path with spaces;
// backslash quotes the next character outside single quotes.
func splitTerminalShellCommand(command string) (string, []string) {
	var fields []string
	var field strings.Builder
	var quote rune
	escaped := false
	haveField := false
	flush := func() {
		if haveField {
			fields = append(fields, field.String())
			field.Reset()
			haveField = false
		}
	}
	for _, ch := range command {
		if escaped {
			field.WriteRune(ch)
			haveField = true
			escaped = false
			continue
		}
		if quote == '\'' {
			if ch == '\'' {
				quote = 0
			} else {
				field.WriteRune(ch)
			}
			haveField = true
			continue
		}
		if quote == '"' {
			switch ch {
			case '"':
				quote = 0
			case '\\':
				escaped = true
			default:
				field.WriteRune(ch)
			}
			haveField = true
			continue
		}
		switch {
		case ch == '\'' || ch == '"':
			quote = ch
			haveField = true
		case ch == '\\' && runtime.GOOS != "windows":
			escaped = true
			haveField = true
		case ch == ' ' || ch == '\t' || ch == '\n':
			flush()
		default:
			field.WriteRune(ch)
			haveField = true
		}
	}
	if escaped {
		field.WriteRune('\\')
	}
	flush()
	if len(fields) == 0 {
		return "", nil
	}
	return fields[0], fields[1:]
}
