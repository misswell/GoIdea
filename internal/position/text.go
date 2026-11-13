// Package position converts between the byte offsets MyGo editors use and the
// UTF-16 code-unit offsets the OpenIDEA protocol uses.
//
// Offsets are counted in bytes of UTF-8 for the editor side and in UTF-16 code
// units for the protocol side. Neither count is a grapheme count: a CJK
// character is one UTF-16 unit, an emoji outside the BMP is two (a surrogate
// pair), a combining mark is a unit of its own.
package position

import "unicode/utf8"

// Text is the read side of a text a Mapper describes. *editor.Snapshot
// satisfies it, as does StringText; a Mapper never copies the whole text.
type Text interface {
	Len() int
	Slice(start, end int) string
}

// StringText adapts a string to Text.
type StringText string

func (s StringText) Len() int { return len(s) }

// Slice returns bytes start to end, clamped to the text.
func (s StringText) Slice(start, end int) string {
	n := len(s)
	start = clamp(start, 0, n)
	end = clamp(end, 0, n)
	if end < start {
		end = start
	}
	return string(s[start:end])
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// UTF16Length returns the UTF-16 code units of s. Invalid UTF-8 bytes count
// one unit each, as utf8.DecodeRuneInString reports each as RuneError of size 1.
func UTF16Length(s string) int { return utf16Len(s) }

// utf16Len returns the UTF-16 code units of s. Invalid UTF-8 bytes count one
// unit each, as utf8.DecodeRuneInString reports each as RuneError of size 1.
func utf16Len(s string) int {
	n := 0
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		if r > 0xFFFF {
			n += 2
		} else {
			n++
		}
		s = s[size:]
	}
	return n
}

// byteAtUTF16Col maps a UTF-16 column inside s to a byte offset, snapping a
// column inside a surrogate pair to the start of the rune.
func byteAtUTF16Col(s string, col int) int {
	if col <= 0 {
		return 0
	}
	units := 0
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		u := 1
		if r > 0xFFFF {
			u = 2
		}
		if units+u > col {
			return i
		}
		units += u
		i += size
	}
	return len(s)
}
