package openidea

import "testing"

func TestNormalizeBrandRenamesTheProduct(t *testing.T) {
	cases := []struct{ in, want string }{
		{"OpenIDEA does not read jrt:// entries", "GoIDEA does not read jrt:// entries"},
		{"update OpenIDEA", "update GoIDEA"},
		{"OpenIDEA's answer ended early", "GoIDEA's answer ended early"},
		{"a project open in openidea", "a project open in GoIDEA"},
		{"move OpenIDEA to the Applications folder", "move GoIDEA to the Applications folder"},
		{"open in OpenIDEA.", "open in GoIDEA."},
		// Names that exist on disk or in the protocol keep their spelling.
		{"openidea-server exited with status 1", "openidea-server exited with status 1"},
		{"see .openidea/project.json", "see .openidea/project.json"},
		{"openidea-api 2 is required", "openidea-api 2 is required"},
		{"openidea.repositories could not be written", "openidea.repositories could not be written"},
		{"cannot read /Users/me/openidea/src/App.java", "cannot read /Users/me/openidea/src/App.java"},
		{"the openidea/bin folder", "the openidea/bin folder"},
	}
	for _, c := range cases {
		if got := NormalizeBrand(c.in); got != c.want {
			t.Fatalf("NormalizeBrand(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestServerErrorIsBrandedAsGoIDEA(t *testing.T) {
	// The server wraps its IdeError in the JSON-RPC message; the message the
	// status bar shows must not call the product OpenIDEA.
	err := parseServerError(CodeInternalError, `{"kind":"plugin","message":"is not an OpenIDEA plugin"}`)
	if err.Message != "is not a GoIDEA plugin" {
		t.Fatalf("message = %q", err.Message)
	}
	if got := err.Error(); got != "goidea: plugin: is not a GoIDEA plugin" {
		t.Fatalf("Error() = %q", got)
	}
	plain := parseServerError(CodeInternalError, "cannot start OpenIDEA")
	if plain.Message != "cannot start GoIDEA" {
		t.Fatalf("plain message = %q", plain.Message)
	}
}
