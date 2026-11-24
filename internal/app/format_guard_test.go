package app

import (
	"testing"

	"mayoi/internal/openidea"
	"mayoi/internal/position"
)

func TestFormatPathExcludedMatchesOpenIDEAGlobs(t *testing.T) {
	root := "/p"
	globs := []string{"*.min.js", "generated", "src/vendor/**", "docs/"}
	cases := []struct {
		path string
		want bool
	}{
		{"/p/web/app.min.js", true},
		{"/p/a/generated/x.ts", true},
		{"/p/src/vendor/lib/x.ts", true},
		{"/p/docs/a.md", true},
		{"/p/src/app.js", false},
		{"/p/other/docs.md", false},
	}
	for _, test := range cases {
		if got := formatPathExcluded(test.path, root, globs); got != test.want {
			t.Errorf("formatPathExcluded(%q) = %v, want %v", test.path, got, test.want)
		}
	}
}

func TestFormatterMarkersProtectOffOnLines(t *testing.T) {
	text := "first\n// @formatter:off\nkeep  this\n// @formatter:on\nlast\n"
	guard := formatGuard{
		formatterTagsEnabled: true,
		formatterOffTag:      "@formatter:off",
		formatterOnTag:       "@formatter:on",
	}
	changes := []formatTextChange{
		{from: 0, to: 0, insert: "X"},
		{from: len("first\n// @formatter:off\n"), to: len("first\n// @formatter:off\nkeep"), insert: "changed"},
		{from: len(text) - 2, to: len(text) - 2, insert: "Y"},
	}
	guarded := guardFormatChanges(text, changes, guard)
	if len(guarded) != 2 {
		t.Fatalf("guarded changes = %#v, want the protected insertion removed", guarded)
	}
	if guarded[0].insert != "X" || guarded[1].insert != "Y" {
		t.Fatalf("guarded changes = %#v", guarded)
	}
}

func TestFormatterRegexpMarkersAndUTF16Edits(t *testing.T) {
	text := "😀\n// fmt: off\npreserve\n// fmt: on\noutside\n"
	guard := formatGuard{
		formatterTagsEnabled: true,
		formatterOffTag:      `fmt:\s*off`,
		formatterOnTag:       `fmt:\s*on`,
		formatterTagsRegexp:  true,
	}
	mapper := position.NewMapper(text)
	inside := mapper.ByteOffsetToUTF16Position(
		position.StringText(text), len("😀\n// fmt: off\n"),
	)
	outside := mapper.ByteOffsetToUTF16Position(
		position.StringText(text), len(text)-len("outside\n"),
	)
	edits := []openidea.TextEdit{
		{
			Range: openidea.OutlineRange{
				Start: openidea.OutlinePos{Line: inside.Line, Col: inside.Character},
				End:   openidea.OutlinePos{Line: inside.Line, Col: inside.Character + 1},
			},
			NewText: "P",
		},
		{
			Range: openidea.OutlineRange{
				Start: openidea.OutlinePos{Line: outside.Line, Col: outside.Character},
				End:   openidea.OutlinePos{Line: outside.Line, Col: outside.Character + 1},
			},
			NewText: "O",
		},
	}
	guarded := guardFormatTextEdits(text, edits, guard)
	if len(guarded) != 1 || guarded[0].NewText != "O" {
		t.Fatalf("guarded edits = %#v, want only the outside edit", guarded)
	}
}
