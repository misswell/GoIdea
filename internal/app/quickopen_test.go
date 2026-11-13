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
