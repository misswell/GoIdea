package app

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/width"
)

const (
	defaultTerminalCols = 80
	defaultTerminalRows = 24
)

// terminalHyperlink is one OSC 8 span on a rendered terminal row.
type terminalHyperlink struct {
	Row   int
	Start int
	End   int
	URI   string
}

// terminalCell is one display column in the alternate screen. A wide rune
// starts a cell of width two and marks the cell after it as a continuation.
// Blank cells have no text, which lets a terminal row drop trailing spaces
// without losing its fixed width.
type terminalCell struct {
	Text         string
	Width        int
	Continuation bool
	Hyperlink    string
	JoinNext     bool
}

// terminalRuneCellWidth returns the number of terminal columns a rune takes.
// Combining marks and format controls such as the zero-width joiner attach to
// the cell before them.
func terminalRuneCellWidth(r rune) int {
	if unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) ||
		unicode.Is(unicode.Cf, r) || r == '\u200d' {
		return 0
	}
	switch width.LookupRune(r).Kind() {
	case width.EastAsianWide, width.EastAsianFullwidth:
		return 2
	}
	// Some terminal pictographs are still "neutral" to x/text. Ghostty and
	// the PTY applications we support reserve two cells for these emoji.
	switch {
	case r >= 0x1f300 && r <= 0x1faff:
		return 2
	case r >= 0x1fc00 && r <= 0x1fffd:
		return 2
	default:
		return 1
	}
}

func terminalTextCellWidth(text string) int {
	column := 0
	for _, r := range text {
		column += terminalRuneCellWidth(r)
	}
	return column
}

func terminalDisplayColumnAt(text string, byteOffset int) int {
	byteOffset = max(0, min(byteOffset, len(text)))
	column := 0
	for offset := 0; offset < byteOffset; {
		r, size := utf8.DecodeRuneInString(text[offset:])
		if size <= 0 {
			break
		}
		column += terminalRuneCellWidth(r)
		offset += size
	}
	return column
}

func terminalDisplayByteOffset(text string, column int) int {
	if column <= 0 {
		return 0
	}
	current := 0
	for offset := 0; offset < len(text); {
		r, size := utf8.DecodeRuneInString(text[offset:])
		if size <= 0 {
			break
		}
		next := current + terminalRuneCellWidth(r)
		if next > column {
			return offset
		}
		current = next
		offset += size
	}
	return len(text)
}

func blankTerminalCells(cols int) []terminalCell {
	row := make([]terminalCell, max(cols, 1))
	for i := range row {
		row[i].Width = 1
	}
	return row
}

func cloneTerminalCells(row []terminalCell) []terminalCell {
	return append([]terminalCell(nil), row...)
}

// writeTerminal keeps the small amount of terminal state that can be split
// between streamed chunks. Shell output continues to use the scrollback. Once a
// program enters the alternate screen, printable text is drawn into a fixed
// cell grid until the program restores the primary screen.
func (p *termSession) writeTerminal(text string) {
	if p.escapeTail != "" {
		text = p.escapeTail + text
		p.escapeTail = ""
	}
	if p.terminalCols <= 0 {
		p.terminalCols = defaultTerminalCols
	}
	if p.terminalRows <= 0 {
		p.terminalRows = defaultTerminalRows
	}

	for len(text) > 0 {
		switch text[0] {
		case '\x1b':
			consumed, complete := p.applyEscape(text)
			if !complete {
				p.escapeTail = text
				return
			}
			text = text[consumed:]
		case '\a':
			text = text[1:]
		case '\b':
			if p.alternate && p.screenCol > 0 {
				p.screenCol--
			}
			text = text[1:]
		case '\r':
			if len(text) == 1 {
				p.escapeTail = text
				return
			}
			if text[1] == '\n' {
				if p.alternate {
					p.screenCol = 0
				}
				text = text[1:]
				continue
			}
			if p.alternate {
				p.screenCol = 0
			} else {
				p.writePrimary("\r")
			}
			text = text[1:]
		case '\n':
			if p.alternate {
				p.ensureAlternateScreen()
				if p.screenRow < p.terminalRows-1 {
					p.screenRow++
				} else {
					p.scrollAlternate(1)
				}
			} else {
				p.writePrimary("\n")
			}
			text = text[1:]
		case '\t':
			count := 8 - p.screenCol%8
			if p.alternate {
				for range count {
					p.writeAlternateRune(' ')
				}
			} else {
				p.writePrimary(strings.Repeat(" ", count))
			}
			text = text[1:]
		default:
			r, size := utf8.DecodeRuneInString(text)
			if r == utf8.RuneError && size == 1 {
				// A UTF-8 rune may also arrive in two PTY chunks. Keep the
				// incomplete tail until the next write rather than corrupting it.
				if !utf8.FullRuneInString(text) {
					p.escapeTail = text
					return
				}
			}
			if p.alternate {
				p.writeAlternateRune(r)
			} else {
				p.writePrimary(string(r))
			}
			text = text[size:]
		}
	}
}

// applyEscape consumes one complete escape sequence. incomplete means the
// stream ended in the middle of the sequence, so the caller keeps it for the
// next PTY chunk.
func (p *termSession) applyEscape(text string) (consumed int, complete bool) {
	if len(text) < 2 {
		return 0, false
	}
	switch text[1] {
	case '[':
		for i := 2; i < len(text); i++ {
			if text[i] >= 0x40 && text[i] <= 0x7e {
				p.applyCSI(text[2:i], text[i])
				return i + 1, true
			}
		}
		return 0, false
	case ']':
		for i := 2; i < len(text); i++ {
			switch text[i] {
			case '\a':
				p.applyOSC(text[2:i])
				return i + 1, true
			case '\x1b':
				if i+1 < len(text) && text[i+1] == '\\' {
					p.applyOSC(text[2:i])
					return i + 2, true
				}
			}
		}
		return 0, false
	case '7':
		p.savedRow, p.savedCol = p.screenRow, p.screenCol
		return 2, true
	case '8':
		p.screenRow, p.screenCol = p.savedRow, p.savedCol
		p.clampAlternateCursor()
		return 2, true
	case 'c':
		p.resetTerminalScreen()
		return 2, true
	case '(', ')', '*', '+':
		if len(text) < 3 {
			return 0, false
		}
		return 3, true
	default:
		return 2, true
	}
}

func (p *termSession) applyOSC(payload string) {
	switch {
	case strings.HasPrefix(payload, "0;"), strings.HasPrefix(payload, "2;"):
		p.terminalTitle = payload[2:]
	case strings.HasPrefix(payload, "8;"):
		parts := strings.SplitN(payload, ";", 3)
		if len(parts) == 3 {
			p.activeHyperlink = strings.TrimSpace(parts[2])
		}
	}
}

func (p *termSession) applyCSI(params string, final byte) {
	private := strings.HasPrefix(params, "?")
	params = strings.TrimPrefix(params, "?")
	values := terminalCSIValues(params)

	switch final {
	case 'h', 'l':
		if !private {
			return
		}
		for _, value := range values {
			switch value {
			case 47, 1047, 1049:
				if final == 'h' {
					p.enterAlternateScreen()
				} else {
					p.leaveAlternateScreen()
				}
			case 9, 1000, 1002, 1003:
				if final == 'h' {
					p.mouseMode = value
				} else {
					// xterm keeps one active tracking protocol. Resetting any
					// tracking mode returns that protocol to NONE.
					p.mouseMode = 0
				}
			case 1006:
				p.mouseSGR = final == 'h'
			case 1016:
				p.mousePixels = final == 'h'
			case 2004:
				p.bracketedPaste = final == 'h'
			case 1004:
				p.focusReporting = final == 'h'
				// Re-observe focus after a mode change so the child receives
				// the current state when it starts or restarts reporting.
				p.focusKnown = false
			case 25:
				p.cursorHidden = final == 'l'
			}
		}
	case 'H', 'f':
		row, col := terminalCSIArg(values, 0, 1), terminalCSIArg(values, 1, 1)
		p.setAlternateCursor(row-1, col-1)
	case 'A':
		p.moveAlternateCursor(-terminalCSIArg(values, 0, 1), 0)
	case 'B':
		p.moveAlternateCursor(terminalCSIArg(values, 0, 1), 0)
	case 'C':
		p.moveAlternateCursor(0, terminalCSIArg(values, 0, 1))
	case 'D':
		p.moveAlternateCursor(0, -terminalCSIArg(values, 0, 1))
	case 'E':
		p.moveAlternateCursor(terminalCSIArg(values, 0, 1), -p.screenCol)
	case 'F':
		p.moveAlternateCursor(-terminalCSIArg(values, 0, 1), -p.screenCol)
	case 'G', '`':
		p.setAlternateCursor(p.screenRow, terminalCSIArg(values, 0, 1)-1)
	case 'd':
		p.setAlternateCursor(terminalCSIArg(values, 0, 1)-1, p.screenCol)
	case 'J':
		p.eraseDisplay(terminalCSIArg(values, 0, 0))
	case 'K':
		p.eraseLine(terminalCSIArg(values, 0, 0))
	case 'P':
		p.deleteAlternateChars(terminalCSIArg(values, 0, 1))
	case '@':
		p.insertAlternateChars(terminalCSIArg(values, 0, 1))
	case 'X':
		p.eraseAlternateChars(terminalCSIArg(values, 0, 1))
	case 'L':
		p.insertAlternateLines(terminalCSIArg(values, 0, 1))
	case 'M':
		p.deleteAlternateLines(terminalCSIArg(values, 0, 1))
	case 'S':
		p.scrollAlternate(terminalCSIArg(values, 0, 1))
	case 'T':
		p.scrollAlternate(-terminalCSIArg(values, 0, 1))
	case 's':
		p.savedRow, p.savedCol = p.screenRow, p.screenCol
	case 'u':
		p.screenRow, p.screenCol = p.savedRow, p.savedCol
		p.clampAlternateCursor()
	}
}

func terminalCSIValues(params string) []int {
	if params == "" {
		return nil
	}
	parts := strings.Split(params, ";")
	values := make([]int, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			values = append(values, 0)
			continue
		}
		value, err := strconv.Atoi(part)
		if err != nil {
			value = 0
		}
		values = append(values, value)
	}
	return values
}

func terminalCSIArg(values []int, index, fallback int) int {
	if index >= len(values) || values[index] <= 0 {
		return fallback
	}
	return values[index]
}

func (p *termSession) enterAlternateScreen() {
	if p.alternate {
		return
	}
	p.primaryLines = append(p.primaryLines[:0], p.lines...)
	p.primaryTruncated = p.truncated
	p.primaryHyperlinks = append(p.primaryHyperlinks[:0], p.hyperlinks...)
	p.alternate = true
	p.ensureAlternateScreen()
	p.hyperlinks = nil
	p.screenRow = 0
	p.screenCol = 0
	p.truncated = false
	p.rebuildAlternateLines()
}

func (p *termSession) leaveAlternateScreen() {
	if !p.alternate {
		return
	}
	p.lines = append(p.lines[:0], p.primaryLines...)
	p.hyperlinks = append(p.hyperlinks[:0], p.primaryHyperlinks...)
	p.primaryLines = nil
	p.primaryHyperlinks = nil
	p.truncated = p.primaryTruncated
	p.primaryTruncated = false
	p.alternate = false
	p.screenCells = nil
	p.activeHyperlink = ""
	p.screenRow = 0
	p.screenCol = 0
}

func (p *termSession) ensureAlternateScreen() {
	if p.terminalCols <= 0 {
		p.terminalCols = defaultTerminalCols
	}
	if p.terminalRows <= 0 {
		p.terminalRows = defaultTerminalRows
	}
	if len(p.screenCells) == p.terminalRows {
		correct := true
		for i := range p.screenCells {
			if len(p.screenCells[i]) != p.terminalCols {
				correct = false
				break
			}
		}
		if correct {
			p.clampAlternateCursor()
			return
		}
	}
	rows := make([][]terminalCell, p.terminalRows)
	for i := range rows {
		rows[i] = blankTerminalCells(p.terminalCols)
	}
	p.screenCells = rows
	p.rebuildAlternateLines()
	p.clampAlternateCursor()
}

// applyTerminalSize adopts the PTY's acknowledged size. Full-screen programs
// repaint after SIGWINCH, so preserving the visible top-left grid across this
// short transition keeps the cursor and wide cells coherent until that frame
// arrives.
func (p *termSession) applyTerminalSize(cols, rows int) {
	cols = max(cols, 2)
	rows = max(rows, 1)
	if p.terminalCols == cols && p.terminalRows == rows {
		return
	}
	oldCols, oldRows := p.terminalCols, p.terminalRows
	oldCells := p.screenCells
	p.terminalCols, p.terminalRows = cols, rows
	if !p.alternate || len(oldCells) == 0 {
		p.clampAlternateCursor()
		return
	}
	cells := make([][]terminalCell, rows)
	for row := range cells {
		cells[row] = blankTerminalCells(cols)
	}
	for row := 0; row < min(oldRows, rows); row++ {
		source := oldCells[row]
		for col := 0; col < min(oldCols, cols); col++ {
			cell := source[col]
			if cell.Continuation {
				continue
			}
			if cell.Width == 2 && col+1 >= cols {
				continue
			}
			cells[row][col] = cell
			if cell.Width == 2 {
				cells[row][col+1] = source[col+1]
			}
		}
	}
	p.screenCells = cells
	p.rebuildAlternateLines()
	p.clampAlternateCursor()
}

func (p *termSession) clampAlternateCursor() {
	if p.screenRow < 0 {
		p.screenRow = 0
	}
	if p.screenRow >= p.terminalRows {
		p.screenRow = p.terminalRows - 1
	}
	if p.screenCol < 0 {
		p.screenCol = 0
	}
	if p.screenCol >= p.terminalCols {
		p.screenCol = p.terminalCols - 1
	}
}

func (p *termSession) setAlternateCursor(row, col int) {
	if !p.alternate {
		return
	}
	p.screenRow, p.screenCol = row, col
	p.ensureAlternateScreen()
}

func (p *termSession) moveAlternateCursor(rows, cols int) {
	if !p.alternate {
		return
	}
	p.setAlternateCursor(p.screenRow+rows, p.screenCol+cols)
}

func (p *termSession) previousAlternateCell() *terminalCell {
	if !p.alternate || p.screenCol <= 0 ||
		p.screenRow < 0 || p.screenRow >= len(p.screenCells) {
		return nil
	}
	col := p.screenCol - 1
	row := p.screenCells[p.screenRow]
	if col >= len(row) {
		col = len(row) - 1
	}
	if col >= 0 && row[col].Continuation && col > 0 {
		col--
	}
	return &row[col]
}

func (p *termSession) clearAlternateCell(row, col int) {
	if row < 0 || row >= len(p.screenCells) {
		return
	}
	cells := p.screenCells[row]
	if col < 0 || col >= len(cells) {
		return
	}
	if cells[col].Continuation && col > 0 {
		cells[col-1] = terminalCell{Width: 1}
	}
	if cells[col].Width == 2 && col+1 < len(cells) {
		cells[col+1] = terminalCell{Width: 1}
	}
	cells[col] = terminalCell{Width: 1}
}

func (p *termSession) writeAlternateCell(row, col int, r rune, cellWidth int) {
	p.clearAlternateCell(row, col)
	if cellWidth == 2 && col+1 < len(p.screenCells[row]) {
		p.clearAlternateCell(row, col+1)
	}
	p.screenCells[row][col] = terminalCell{
		Text:      string(r),
		Width:     cellWidth,
		Hyperlink: p.activeHyperlink,
	}
	if cellWidth == 2 && col+1 < len(p.screenCells[row]) {
		p.screenCells[row][col+1] = terminalCell{
			Width:        1,
			Continuation: true,
			Hyperlink:    p.activeHyperlink,
		}
	}
}

func (p *termSession) writeAlternateRune(r rune) {
	p.ensureAlternateScreen()
	if r == '\n' {
		if p.screenRow == p.terminalRows-1 {
			p.scrollAlternate(1)
		} else {
			p.screenRow++
		}
		p.screenCol = 0
		return
	}
	cellWidth := terminalRuneCellWidth(r)
	if unicode.IsControl(r) {
		return
	}
	if previous := p.previousAlternateCell(); previous != nil {
		if cellWidth == 0 || previous.JoinNext {
			previous.Text += string(r)
			previous.JoinNext = r == '\u200d'
			p.rebuildAlternateRow(p.screenRow)
			if p.activeHyperlink != "" || len(p.hyperlinks) > 0 {
				p.rebuildAlternateHyperlinks()
			}
			return
		}
	}
	if cellWidth == 0 {
		return
	}
	if p.screenCol >= p.terminalCols {
		p.screenCol = 0
		if p.screenRow == p.terminalRows-1 {
			p.scrollAlternate(1)
		} else {
			p.screenRow++
		}
	}
	if cellWidth == 2 && p.screenCol == p.terminalCols-1 {
		p.screenCol = 0
		if p.screenRow == p.terminalRows-1 {
			p.scrollAlternate(1)
		} else {
			p.screenRow++
		}
	}
	p.writeAlternateCell(p.screenRow, p.screenCol, r, cellWidth)
	p.screenCol += cellWidth
	if p.activeHyperlink != "" || len(p.hyperlinks) > 0 {
		p.rebuildAlternateRow(p.screenRow)
		p.rebuildAlternateHyperlinks()
	} else {
		p.rebuildAlternateRow(p.screenRow)
	}
	if p.screenCol >= p.terminalCols {
		p.screenCol = 0
		if p.screenRow == p.terminalRows-1 {
			p.scrollAlternate(1)
		} else {
			p.screenRow++
		}
	}
}

func (p *termSession) rebuildAlternateRow(row int) {
	if row < 0 || row >= len(p.screenCells) {
		return
	}
	for len(p.lines) <= row {
		p.lines = append(p.lines, "")
	}
	p.lines[row] = terminalCellsText(p.screenCells[row], p.terminalCols)
}

func (p *termSession) rebuildAlternateLines() {
	p.lines = make([]string, len(p.screenCells))
	for row := range p.screenCells {
		p.lines[row] = terminalCellsText(p.screenCells[row], p.terminalCols)
	}
	p.rebuildAlternateHyperlinks()
}

func terminalCellsText(cells []terminalCell, cols int) string {
	var text strings.Builder
	columns := 0
	for _, cell := range cells {
		if columns >= cols {
			break
		}
		if cell.Continuation {
			continue
		}
		if cell.Text == "" {
			text.WriteByte(' ')
			columns++
			continue
		}
		text.WriteString(cell.Text)
		cellWidth := cell.Width
		if cellWidth <= 0 {
			cellWidth = terminalTextCellWidth(cell.Text)
		}
		columns += cellWidth
	}
	if columns < cols {
		text.WriteString(strings.Repeat(" ", cols-columns))
	}
	return text.String()
}

func (p *termSession) rebuildAlternateHyperlinks() {
	p.hyperlinks = p.hyperlinks[:0]
	for row, cells := range p.screenCells {
		for col := 0; col < len(cells) && col < p.terminalCols; {
			link := cells[col].Hyperlink
			if link == "" || cells[col].Continuation {
				col++
				continue
			}
			end := col + max(cells[col].Width, 1)
			for end < len(cells) && cells[end].Hyperlink == link {
				end++
			}
			p.hyperlinks = append(p.hyperlinks, terminalHyperlink{
				Row: row, Start: col, End: end, URI: link,
			})
			col = end
		}
	}
}

// writePrimary retains the scrollback's line discipline for ordinary shell
// output: newlines add rows and carriage returns rewrite the current row.
func (p *termSession) writePrimary(text string) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if len(p.lines) == 0 {
		p.lines = []string{""}
	}
	for _, r := range text {
		switch r {
		case '\n':
			p.lines = append(p.lines, "")
		case '\r':
			p.lines[len(p.lines)-1] = ""
			p.removePrimaryHyperlinks(len(p.lines) - 1)
		default:
			row := len(p.lines) - 1
			start := terminalTextCellWidth(p.lines[row])
			p.lines[len(p.lines)-1] += string(r)
			if p.activeHyperlink != "" {
				p.addPrimaryHyperlink(row, start, start+terminalRuneCellWidth(r), p.activeHyperlink)
			}
		}
	}
	if over := len(p.lines) - maxTermLines; over > 0 {
		p.lines = append(p.lines[:0], p.lines[over:]...)
		p.truncated = true
		links := p.hyperlinks[:0]
		for _, link := range p.hyperlinks {
			if link.Row < over {
				continue
			}
			link.Row -= over
			links = append(links, link)
		}
		p.hyperlinks = links
	}
}

func (p *termSession) removePrimaryHyperlinks(row int) {
	links := p.hyperlinks[:0]
	for _, link := range p.hyperlinks {
		if link.Row != row {
			links = append(links, link)
		}
	}
	p.hyperlinks = links
}

func (p *termSession) addPrimaryHyperlink(row, start, end int, uri string) {
	if uri == "" || end <= start {
		return
	}
	for i := len(p.hyperlinks) - 1; i >= 0; i-- {
		link := &p.hyperlinks[i]
		if link.Row != row || link.URI != uri || link.End < start {
			continue
		}
		if link.End >= start {
			link.End = max(link.End, end)
			return
		}
	}
	p.hyperlinks = append(p.hyperlinks, terminalHyperlink{
		Row: row, Start: start, End: end, URI: uri,
	})
}

func (p *termSession) eraseDisplay(mode int) {
	if !p.alternate {
		return
	}
	p.ensureAlternateScreen()
	switch mode {
	case 0:
		p.eraseLine(0)
		for row := p.screenRow + 1; row < p.terminalRows; row++ {
			p.screenCells[row] = blankTerminalCells(p.terminalCols)
			p.rebuildAlternateRow(row)
		}
	case 1:
		for row := 0; row < p.screenRow; row++ {
			p.screenCells[row] = blankTerminalCells(p.terminalCols)
			p.rebuildAlternateRow(row)
		}
		p.eraseLine(1)
	case 2, 3:
		for row := range p.screenCells {
			p.screenCells[row] = blankTerminalCells(p.terminalCols)
			p.rebuildAlternateRow(row)
		}
	}
	p.rebuildAlternateHyperlinks()
}

func (p *termSession) eraseLine(mode int) {
	if !p.alternate {
		return
	}
	p.ensureAlternateScreen()
	start, end := 0, p.terminalCols
	switch mode {
	case 0:
		start = p.screenCol
	case 1:
		end = min(p.screenCol+1, p.terminalCols)
	case 2:
		start, end = 0, p.terminalCols
	}
	start, end = terminalWideCellRange(p.screenCells[p.screenRow], start, end)
	for i := start; i < end; i++ {
		p.screenCells[p.screenRow][i] = terminalCell{Width: 1}
	}
	p.rebuildAlternateRow(p.screenRow)
	p.rebuildAlternateHyperlinks()
}

func (p *termSession) deleteAlternateChars(count int) {
	if !p.alternate {
		return
	}
	p.ensureAlternateScreen()
	if p.screenCol >= p.terminalCols {
		return
	}
	count = max(1, min(count, p.terminalCols-p.screenCol))
	source := p.screenCells[p.screenRow]
	start := terminalWideCellStart(source, p.screenCol)
	start, end := terminalWideCellRange(source, start, p.screenCol+count)
	shifted := blankTerminalCells(p.terminalCols)
	copy(shifted[:start], source[:start])
	copyTerminalCellsLeft(shifted, start, source, end)
	p.screenCells[p.screenRow] = shifted
	p.screenCol = start
	p.rebuildAlternateRow(p.screenRow)
	p.rebuildAlternateHyperlinks()
}

func (p *termSession) insertAlternateChars(count int) {
	if !p.alternate {
		return
	}
	p.ensureAlternateScreen()
	if p.screenCol >= p.terminalCols {
		return
	}
	count = max(1, min(count, p.terminalCols-p.screenCol))
	source := p.screenCells[p.screenRow]
	start := terminalWideCellStart(source, p.screenCol)
	shifted := blankTerminalCells(p.terminalCols)
	copy(shifted[:start], source[:start])
	copyTerminalCellsRight(shifted, start+count, source, start)
	p.screenCells[p.screenRow] = shifted
	p.screenCol = start
	p.rebuildAlternateRow(p.screenRow)
	p.rebuildAlternateHyperlinks()
}

func (p *termSession) eraseAlternateChars(count int) {
	if !p.alternate {
		return
	}
	p.ensureAlternateScreen()
	if p.screenCol >= p.terminalCols {
		return
	}
	count = max(1, min(count, p.terminalCols-p.screenCol))
	start, end := terminalWideCellRange(
		p.screenCells[p.screenRow], p.screenCol, p.screenCol+count,
	)
	for i := start; i < end; i++ {
		p.screenCells[p.screenRow][i] = terminalCell{Width: 1}
	}
	p.rebuildAlternateRow(p.screenRow)
	p.rebuildAlternateHyperlinks()
}

func terminalWideCellStart(cells []terminalCell, col int) int {
	col = max(0, min(col, len(cells)))
	if col > 0 && col < len(cells) && cells[col].Continuation {
		return col - 1
	}
	return col
}

func terminalWideCellRange(cells []terminalCell, start, end int) (int, int) {
	start = max(0, min(start, len(cells)))
	end = max(0, min(end, len(cells)))
	if start >= end {
		return start, end
	}
	start = terminalWideCellStart(cells, start)
	if end < len(cells) && cells[end].Continuation {
		end++
	}
	return start, end
}

func copyTerminalCellsLeft(
	destination []terminalCell,
	destinationCol int,
	source []terminalCell,
	sourceCol int,
) {
	for sourceCol < len(source) && destinationCol < len(destination) {
		cell := source[sourceCol]
		if cell.Continuation {
			sourceCol++
			continue
		}
		if cell.Width == 2 {
			if sourceCol+1 >= len(source) || !source[sourceCol+1].Continuation ||
				destinationCol+1 >= len(destination) {
				sourceCol += 2
				continue
			}
			destination[destinationCol] = cell
			destination[destinationCol+1] = source[sourceCol+1]
			sourceCol += 2
			destinationCol += 2
			continue
		}
		destination[destinationCol] = cell
		sourceCol++
		destinationCol++
	}
}

func copyTerminalCellsRight(
	destination []terminalCell,
	destinationCol int,
	source []terminalCell,
	sourceCol int,
) {
	limit := min(len(source), len(destination)-destinationCol+sourceCol)
	for sourceCol < limit {
		cell := source[sourceCol]
		if cell.Continuation {
			sourceCol++
			continue
		}
		cellWidth := max(cell.Width, 1)
		if cellWidth == 2 {
			if sourceCol+1 >= limit || !source[sourceCol+1].Continuation ||
				destinationCol+1 >= len(destination) {
				sourceCol += 2
				continue
			}
			destination[destinationCol] = cell
			destination[destinationCol+1] = source[sourceCol+1]
		} else if destinationCol < len(destination) {
			destination[destinationCol] = cell
		}
		sourceCol += cellWidth
		destinationCol += cellWidth
	}
}

func (p *termSession) insertAlternateLines(count int) {
	if !p.alternate {
		return
	}
	p.ensureAlternateScreen()
	count = min(count, p.terminalRows-p.screenRow)
	blank := make([][]terminalCell, count)
	for i := range blank {
		blank[i] = blankTerminalCells(p.terminalCols)
	}
	p.screenCells = append(p.screenCells[:p.screenRow],
		append(blank, p.screenCells[p.screenRow:len(p.screenCells)-count]...)...)
	p.rebuildAlternateLines()
}

func (p *termSession) deleteAlternateLines(count int) {
	if !p.alternate {
		return
	}
	p.ensureAlternateScreen()
	count = min(count, p.terminalRows-p.screenRow)
	blank := make([][]terminalCell, count)
	for i := range blank {
		blank[i] = blankTerminalCells(p.terminalCols)
	}
	p.screenCells = append(p.screenCells[:p.screenRow],
		append(p.screenCells[p.screenRow+count:], blank...)...)
	p.rebuildAlternateLines()
}

func (p *termSession) scrollAlternate(count int) {
	if !p.alternate || count == 0 {
		return
	}
	p.ensureAlternateScreen()
	if count > 0 {
		count = min(count, p.terminalRows)
		p.screenCells = append(p.screenCells[count:], make([][]terminalCell, count)...)
		for i := p.terminalRows - count; i < p.terminalRows; i++ {
			p.screenCells[i] = blankTerminalCells(p.terminalCols)
		}
	} else {
		count = min(-count, p.terminalRows)
		prefix := make([][]terminalCell, count)
		for i := range prefix {
			prefix[i] = blankTerminalCells(p.terminalCols)
		}
		p.screenCells = append(prefix, p.screenCells[:p.terminalRows-count]...)
	}
	p.rebuildAlternateLines()
}

func (p *termSession) resetTerminalScreen() {
	p.lines = nil
	p.truncated = false
	p.alternate = false
	p.screenCells = nil
	p.primaryLines = nil
	p.primaryTruncated = false
	p.screenRow = 0
	p.screenCol = 0
	p.savedRow = 0
	p.savedCol = 0
	p.escapeTail = ""
	p.terminalTitle = ""
	p.activeHyperlink = ""
	p.hyperlinks = nil
	p.primaryHyperlinks = nil
	p.mouseMode = 0
	p.mouseSGR = false
	p.mousePixels = false
	p.bracketedPaste = false
	p.focusReporting = false
	p.focusKnown = false
	p.focused = false
	p.cursorHidden = false
}
