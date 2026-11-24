package app

import (
	"os"
	"testing"
)

// GoIDEA's native workbench is tested with OpenIDEA's macOS preset because
// macOS is the shipped client target, including when unit tests run on Linux.
func TestMain(m *testing.M) {
	keymapHostOS = "darwin"
	os.Exit(m.Run())
}
