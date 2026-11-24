package app

import (
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/width"
)

const vimDefaultTabWidth = 4

func vimTabWidth(t *Tab) int {
	if t != nil && t.state != nil && t.state.TabSize > 0 {
		return t.state.TabSize
	}
	return vimDefaultTabWidth
}

func vimRuneDisplayWidth(r rune, column, tabWidth int) int {
	if r == '\t' {
		return tabWidth - column%tabWidth
	}
	if unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) || unicode.Is(unicode.Cf, r) {
		return 0
	}
	switch width.LookupRune(r).Kind() {
	case width.EastAsianWide, width.EastAsianFullwidth:
		return 2
	default:
		return 1
	}
}

func vimTextDisplayWidth(text string, tabWidth int) int {
	column := 0
	for _, r := range text {
		column += vimRuneDisplayWidth(r, column, tabWidth)
	}
	return column
}

func vimDisplayColumnAt(text string, byteOffset, tabWidth int) int {
	byteOffset = max(0, min(byteOffset, len(text)))
	column := 0
	for offset := 0; offset < byteOffset; {
		r, size := utf8.DecodeRuneInString(text[offset:])
		if size <= 0 {
			break
		}
		column += vimRuneDisplayWidth(r, column, tabWidth)
		offset += size
	}
	return column
}

func vimDisplayByteOffset(text string, column, tabWidth int) int {
	if column <= 0 {
		return 0
	}
	current := 0
	for offset := 0; offset < len(text); {
		r, size := utf8.DecodeRuneInString(text[offset:])
		if size <= 0 {
			break
		}
		next := current + vimRuneDisplayWidth(r, current, tabWidth)
		if next > column {
			return offset
		}
		current = next
		offset += size
	}
	return len(text)
}

func vimDisplayCellSpan(text string, byteOffset, tabWidth int) (int, int) {
	byteOffset = max(0, min(byteOffset, len(text)))
	column := 0
	for offset := 0; offset < len(text); {
		r, size := utf8.DecodeRuneInString(text[offset:])
		if size <= 0 {
			break
		}
		next := column + vimRuneDisplayWidth(r, column, tabWidth)
		if byteOffset < offset+size {
			if r == '\t' {
				return column, next
			}
			if next == column {
				return column, column
			}
			return column, next
		}
		column = next
		offset += size
	}
	return column, column
}

func vimDisplayStep(text string, byteOffset, delta, tabWidth int) int {
	if delta >= 0 {
		for range delta {
			if byteOffset >= len(text) {
				return len(text)
			}
			_, size := utf8.DecodeRuneInString(text[byteOffset:])
			if size <= 0 {
				return len(text)
			}
			byteOffset += size
		}
		return byteOffset
	}
	for range -delta {
		if byteOffset <= 0 {
			return 0
		}
		_, size := utf8.DecodeLastRuneInString(text[:byteOffset])
		if size <= 0 {
			return 0
		}
		byteOffset -= size
	}
	return byteOffset
}
