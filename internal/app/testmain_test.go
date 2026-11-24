package app

import (
	"os"
	"runtime"
	"testing"
)

// GoIDEA's native workbench is tested with OpenIDEA's macOS preset because
// macOS is the shipped client target, including when unit tests run on Linux.
func TestMain(m *testing.M) {
	keymapHostOS = "darwin"
	os.Exit(m.Run())
}

func requireDarwinKeymap(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Skip("OpenIDEA's macOS Command and Control bindings are distinct only on macOS")
	}
}
