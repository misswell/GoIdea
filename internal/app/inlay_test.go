package app

import (
	"testing"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
)

func TestEditorInlaysConvertsUTF16AndPadding(t *testing.T) {
	doc := docbridge.Virtual("/demo.java", "😀method(1)\n")
	hints := []inlayHint{
		{Position: openidea.OutlinePos{Line: 0, Col: 9}, Label: "value:", PaddingLeft: true, PaddingRight: true},
		{Position: openidea.OutlinePos{Line: 0, Col: 2}, Label: "type"},
		{Position: openidea.OutlinePos{Line: 1, Col: 0}},
	}
	got := editorInlays(doc, hints)
	want := []editor.Inlay{{Offset: len("😀method("), Text: " value: "}, {Offset: len("😀"), Text: "type"}}
	if len(got) != len(want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("hint %d = %#v, want %#v", i, got[i], want[i])
		}
	}
}
