package app

import "testing"

func TestFilePathParts(t *testing.T) {
	cases := []struct{ rel, name, dir string }{
		{"src/main/java/demo/Hello.java", "Hello.java", "src/main/java/demo"},
		{"Hello.java", "Hello.java", ""},
		{"./pom.xml", "pom.xml", ""},
		{"a/b/", "", "a/b"},
	}
	for _, c := range cases {
		name, dir := filePathParts(c.rel)
		if name != c.name || dir != c.dir {
			t.Fatalf("filePathParts(%q) = %q, %q; want %q, %q", c.rel, name, dir, c.name, c.dir)
		}
	}
}

func TestOpenQuickFilesNeedsAProject(t *testing.T) {
	m := &Model{}
	m.OpenQuickFiles()
	if m.files != nil {
		t.Fatalf("picker opened without a project")
	}
}

func TestCloseQuickFilesDropsPicker(t *testing.T) {
	m := &Model{files: &filePicker{open: true, items: nil}}
	m.closeQuickFiles()
	if m.files != nil {
		t.Fatalf("picker stayed open")
	}
}

func TestFileListHeightHugsRowsAndCaps(t *testing.T) {
	if got := fileListHeight(1); got != 24 {
		t.Fatalf("one match takes %v DIP, want 24", got)
	}
	if got := fileListHeight(5); got != 120 {
		t.Fatalf("five matches take %v DIP, want 120", got)
	}
	if got := fileListHeight(200); got != 330 {
		t.Fatalf("a long list takes %v DIP, want the 330 cap", got)
	}
}
