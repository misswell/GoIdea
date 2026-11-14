package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestJavaMainClassFromArgs(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"-cp", "${classpath}", "demo.App"}, "demo.App"},
		{[]string{"-Xmx1g", "--add-opens", "java.base/java.lang=ALL-UNNAMED", "a.Main", "--port", "1"}, "a.Main"},
		{[]string{"-jar", "app.jar"}, ""},
		{nil, ""},
	}
	for _, c := range cases {
		if got := javaMainClass(c.args); got != c.want {
			t.Fatalf("javaMainClass(%v) = %q, want %q", c.args, got, c.want)
		}
	}
}

func TestJavaModuleOfFindsTheModule(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "server", "src", "main", "java", "demo")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "App.java"), []byte("package demo;\nclass App {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := javaModuleOf(root, "demo.App"); got != "server" {
		t.Fatalf("module = %q", got)
	}
	if got := javaModuleOf(root, "demo.Other"); got != "" {
		t.Fatalf("missing class found module %q", got)
	}
}
