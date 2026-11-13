package app

import "testing"

func TestEditorPathFlavours(t *testing.T) {
	got := editorPathFlavours("/work/demo", "/work/demo/src/Main.java", 7)
	want := []editorPathFlavour{
		{"Absolute Path", "/work/demo/src/Main.java"},
		{"File Name", "Main.java"},
		{"Path From Content Root", "src/Main.java"},
		{"Path with line number", "src/Main.java:7"},
	}
	if len(got) != len(want) {
		t.Fatalf("flavours = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("flavour %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestEditorPathFlavoursOutsideRootKeepAbsolute(t *testing.T) {
	got := editorPathFlavours("/work/demo", "/elsewhere/Lib.java", 3)
	if got[2].value != "/elsewhere/Lib.java" {
		t.Errorf("Path From Content Root = %q, want the absolute path", got[2].value)
	}
	if got[3].value != "/elsewhere/Lib.java:3" {
		t.Errorf("Path with line number = %q, want the absolute path and line", got[3].value)
	}
}
