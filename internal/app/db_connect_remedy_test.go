package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

func TestDBConnectRemedyClassification(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		source map[string]any
		want   string
	}{
		{
			name: "host key",
			err:  &openidea.Error{Kind: "protocol", Message: "Unknown SSH host key for bastion:22"},
			want: "trustHostKey",
		},
		{
			name: "changed host key is never trusted",
			err:  &openidea.Error{Kind: "permissionDenied", Message: "SSH host key of bastion:22 has changed"},
			want: "",
		},
		{
			name: "ssh passphrase",
			err:  &openidea.Error{Kind: "permissionDenied", Message: "SSH key passphrase required for /key"},
			want: "sshSecret",
		},
		{
			name:   "open ssh agent failure has no prompt",
			err:    &openidea.Error{Kind: "permissionDenied", Message: "SSH authentication failed"},
			source: map[string]any{"ssh": map[string]any{"auth": "openSsh"}},
			want:   "",
		},
		{
			name: "database password",
			err:  &openidea.Error{Kind: "permissionDenied", Message: "password authentication failed"},
			want: "password",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := dbConnectRemedyKind(test.err, test.source); got != test.want {
				t.Fatalf("remedy = %q, want %q", got, test.want)
			}
		})
	}
}

func TestDBConnectRemedyDialogHidesPassword(t *testing.T) {
	remedy := &dbConnectRemedy{
		open: true, kind: "password", target: "pg", source: "pg",
		name: "PostgreSQL", user: "ada", driver: "postgres", secret: "test-secret",
	}
	m := &Model{db: &dbView{
		open: true, connectRemedy: remedy,
		sources: []openidea.DataSource{{ID: "pg", Name: "PostgreSQL", Driver: "postgres"}},
	}}
	tester := ui.NewTester(func(c *ui.Context) { m.dbConnectRemedyDialog(c, c.Theme()) }, 600, 400)
	tester.Frame()
	for _, label := range []string{
		"Connect to PostgreSQL", "User:", "ada", "Password:", "Save password", "Connect", "Cancel",
	} {
		if !tester.HasText(label) {
			t.Fatalf("missing %q from password prompt: %v", label, tester.Texts())
		}
	}
	if tester.HasText("test-secret") {
		t.Fatal("password was rendered as plain text")
	}
}
