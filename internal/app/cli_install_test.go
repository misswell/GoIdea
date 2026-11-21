package app

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCLIInstallUninstallPreservesForeignCommands(t *testing.T) {
	home := t.TempDir()
	exe := filepath.Join(home, "GoIDEA")
	if err := os.WriteFile(exe, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	goos := runtime.GOOS
	result, err := installCLI(exe, home, "", goos)
	if err != nil {
		t.Fatal(err)
	}
	if result.PathDirectory == "" {
		t.Fatal("a bin directory absent from PATH was not reported")
	}
	data, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), goideaCLIMarker) || !strings.Contains(string(data), exe) {
		t.Fatalf("wrapper = %q", data)
	}
	stat, err := os.Stat(result.Path)
	if err != nil {
		t.Fatal(err)
	}
	if got := stat.Mode().Perm(); got&0o111 == 0 {
		t.Fatalf("wrapper mode = %o, want executable", got)
	}
	if _, err := installCLI(exe, home, result.PathDirectory, goos); err != nil {
		t.Fatalf("reinstalling an owned wrapper failed: %v", err)
	}

	removed, err := uninstallCLI(home, goos)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) == 0 || removed[0] != result.Path {
		t.Fatalf("removed = %v, want %s", removed, result.Path)
	}
	if _, err := os.Stat(result.Path); !os.IsNotExist(err) {
		t.Fatalf("wrapper still exists: %v", err)
	}

	foreign := goideaCLIWrapperPath(filepath.Dir(result.Path), goos)
	if err := os.WriteFile(foreign, []byte("#!/bin/sh\necho foreign\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := installCLI(exe, home, "", goos); err == nil {
		t.Fatal("a foreign command was overwritten")
	}
	got, err := os.ReadFile(foreign)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "#!/bin/sh\necho foreign\n" {
		t.Fatalf("foreign command changed to %q", got)
	}
	removed, err = uninstallCLI(home, goos)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 0 {
		t.Fatalf("uninstall removed a foreign command: %v", removed)
	}
}

func TestCLIWrapperDetectsPATHAndRejectsMissingExecutable(t *testing.T) {
	home := t.TempDir()
	exe := filepath.Join(home, "GoIDEA")
	if err := os.WriteFile(exe, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, ".local", "bin")
	result, err := installCLI(exe, home, dir, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	if result.PathDirectory != "" {
		t.Fatalf("PATH directory = %q, want empty", result.PathDirectory)
	}
	if _, err := installCLI(filepath.Join(home, "missing"), home, "", runtime.GOOS); err == nil {
		t.Fatal("installing the CLI for a missing executable succeeded")
	}
}
