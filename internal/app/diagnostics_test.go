package app

import (
	"testing"

	"github.com/egoist/mygo/ui/editor"

	"goidea/internal/openidea"
)

func TestDiagSeverity(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want editor.Severity
	}{
		{"error", editor.SeverityError},
		{"warning", editor.SeverityWarning},
		{"info", editor.SeverityInfo},
		{"hint", editor.SeverityHint},
		{"Warning", editor.SeverityWarning},
		// A kind the protocol does not name is still a problem.
		{"", editor.SeverityError},
		{"fatal", editor.SeverityError},
	} {
		if got := diagSeverity(tc.in); got != tc.want {
			t.Errorf("diagSeverity(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestDiagSource(t *testing.T) {
	s := func(v string) *string { return &v }
	for _, tc := range []struct {
		name      string
		published string
		it        openidea.Diagnostic
		want      string
	}{
		{"server wins", "jdtls", openidea.Diagnostic{Server: s("jdtls"), Source: s("javac")}, "jdtls"},
		{"producer name next", "jdtls", openidea.Diagnostic{Source: s("javac")}, "javac"},
		{"publish source last", "jdtls", openidea.Diagnostic{}, "jdtls"},
		{"empty server falls through", "jdtls", openidea.Diagnostic{Server: s(""), Source: s("javac")}, "javac"},
	} {
		if got := diagSource(tc.published, tc.it); got != tc.want {
			t.Errorf("%s: diagSource = %q, want %q", tc.name, got, tc.want)
		}
	}
}
