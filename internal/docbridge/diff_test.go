package docbridge

import (
	"math/rand/v2"
	"strings"
	"testing"
	"unicode/utf8"

	"mayoi/internal/position"
)

func TestDiffRebuildsNewText(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	alphabet := []string{"a", "b", "\n", "世界", "😀", "e\u0301", " ", "\t"}
	gen := func() string {
		n := rng.IntN(30)
		var b strings.Builder
		for i := 0; i < n; i++ {
			b.WriteString(alphabet[rng.IntN(len(alphabet))])
		}
		return b.String()
	}

	for iter := 0; iter < 2000; iter++ {
		oldText := gen()
		newText := gen()
		hunks := diffTexts(position.StringText(oldText), position.StringText(newText))

		prevOld, prevNew := 0, 0
		for _, h := range hunks {
			if h.OldStart < prevOld || h.NewStart < prevNew {
				t.Fatalf("hunks unordered or overlapping: %+v", hunks)
			}
			if !runeBoundary(oldText, h.OldStart) || !runeBoundary(oldText, h.OldEnd) {
				t.Fatalf("hunk splits a rune in old text %q: %+v", oldText, h)
			}
			if !runeBoundary(newText, h.NewStart) || !runeBoundary(newText, h.NewEnd) {
				t.Fatalf("hunk splits a rune in new text %q: %+v", newText, h)
			}
			prevOld, prevNew = h.OldEnd, h.NewEnd
		}

		var b strings.Builder
		prev := 0
		for _, h := range hunks {
			b.WriteString(oldText[prev:h.OldStart])
			b.WriteString(newText[h.NewStart:h.NewEnd])
			prev = h.OldEnd
		}
		b.WriteString(oldText[prev:])
		if got := b.String(); got != newText {
			t.Fatalf("iter %d: rebuild %q, want %q\nold %q hunks %+v", iter, got, newText, oldText, hunks)
		}
	}
}

func runeBoundary(s string, i int) bool {
	return i == 0 || i == len(s) || utf8.RuneStart(s[i])
}

func TestDiffIdenticalIsEmpty(t *testing.T) {
	for _, s := range []string{"", "x", "package demo;\n", "世界😀\n"} {
		if h := diffTexts(position.StringText(s), position.StringText(s)); len(h) != 0 {
			t.Fatalf("identical %q produced %+v", s, h)
		}
	}
}

func TestDiffNewlineAtLineStart(t *testing.T) {
	oldText := "class A {\n}\n"
	newText := "class A {\n// note\n}\n"
	hunks := diffTexts(position.StringText(oldText), position.StringText(newText))
	if len(hunks) != 1 {
		t.Fatalf("hunks %+v", hunks)
	}
	h := hunks[0]
	if h.OldStart != h.OldEnd || oldText[:h.OldStart] != "class A {\n" {
		t.Fatalf("insertion not at the line start: %+v", h)
	}
	if newText[h.NewStart:h.NewEnd] != "// note\n" {
		t.Fatalf("inserted %q", newText[h.NewStart:h.NewEnd])
	}
}

func TestDiffKeepsAstralRunesWhole(t *testing.T) {
	oldText := "a😀b"
	newText := "a😁b"
	hunks := diffTexts(position.StringText(oldText), position.StringText(newText))
	if len(hunks) != 1 {
		t.Fatalf("hunks %+v", hunks)
	}
	h := hunks[0]
	if h.OldEnd-h.OldStart != 4 || h.NewEnd-h.NewStart != 4 {
		t.Fatalf("hunk does not cover the whole run: %+v", h)
	}
	if oldText[h.OldStart:h.OldEnd] != "😀" || newText[h.NewStart:h.NewEnd] != "😁" {
		t.Fatalf("hunk %q -> %q", oldText[h.OldStart:h.OldEnd], newText[h.NewStart:h.NewEnd])
	}
}

func TestDiffLargeRewriteFallsBack(t *testing.T) {
	oldText := strings.Repeat("line\n", 4000)
	newText := strings.Repeat("other\n", 4000)
	hunks := diffTexts(position.StringText(oldText), position.StringText(newText))
	if len(hunks) != 1 {
		t.Fatalf("hunks %d", len(hunks))
	}
	// The one newline shared by both texts stays common; everything else differs.
	if hunks[0].OldStart != 0 || hunks[0].OldEnd != len(oldText)-1 ||
		hunks[0].NewStart != 0 || hunks[0].NewEnd != len(newText)-1 {
		t.Fatalf("fallback hunk %+v", hunks[0])
	}
}

func TestHunksConflict(t *testing.T) {
	at := func(o, n int) Hunk { return Hunk{OldStart: o, OldEnd: o, NewStart: n, NewEnd: n} }
	rangeH := func(o, e int) Hunk { return Hunk{OldStart: o, OldEnd: e, NewStart: o, NewEnd: e} }
	cases := []struct {
		name string
		a, b Hunk
		want bool
	}{
		{"same insert", at(5, 0), at(5, 1), true},
		{"different inserts", at(5, 0), at(6, 0), false},
		{"insert inside range", at(5, 0), rangeH(4, 7), true},
		{"insert at range start", at(4, 0), rangeH(4, 7), true},
		{"insert at range end", at(7, 0), rangeH(4, 7), false},
		{"overlapping ranges", rangeH(4, 8), rangeH(6, 9), true},
		{"adjacent ranges", rangeH(4, 6), rangeH(6, 9), false},
	}
	for _, c := range cases {
		if got := hunksConflict(c.a, c.b); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
		if got := hunksConflict(c.b, c.a); got != c.want {
			t.Errorf("%s reversed: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestConcatText(t *testing.T) {
	c := concatText{a: "hello", b: "世界"}
	if c.Len() != 11 {
		t.Fatalf("len %d", c.Len())
	}
	for _, tc := range []struct{ start, end int }{
		{0, 0}, {0, 5}, {0, 11}, {3, 8}, {5, 8}, {6, 11}, {-2, 3}, {8, 100},
	} {
		want := ("hello" + "世界")
		s, e := clamp(tc.start, 0, len(want)), clamp(tc.end, 0, len(want))
		if e < s {
			e = s
		}
		if got := c.Slice(tc.start, tc.end); got != want[s:e] {
			t.Errorf("slice %d:%d = %q, want %q", tc.start, tc.end, got, want[s:e])
		}
	}
}
