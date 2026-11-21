package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommandLineHelpAndVersion(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--help"}, "Usage:"},
		{[]string{"--cli", "--version"}, "GoIDEA 1.2.3"},
	} {
		var stdout, stderr bytes.Buffer
		handled, code := handleCommandLine(tc.args, &stdout, &stderr, "1.2.3")
		if !handled || code != 0 || !strings.Contains(stdout.String(), tc.want) {
			t.Fatalf("%v: handled=%v code=%d stdout=%q stderr=%q", tc.args, handled, code, stdout.String(), stderr.String())
		}
	}
}

func TestCommandLineTargetsSeparateFolderAndFiles(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "Main.java")
	if err := os.WriteFile(file, []byte("class Main {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir, files, errs := commandLineTargets([]string{"--cli", root, file})
	if len(errs) != 0 || dir != root || len(files) != 1 || files[0] != file {
		t.Fatalf("dir=%q files=%v errs=%v", dir, files, errs)
	}
}
