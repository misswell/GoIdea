package app

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

func TestValidateRemoteName(t *testing.T) {
	items := []openidea.Remote{{Name: "origin"}, {Name: "backup"}}
	for _, tc := range []struct {
		name, original string
		valid          bool
	}{
		{"", "", false}, {" bad name ", "", false}, {"-name", "", false},
		{"a..b", "", false}, {"a[b", "", false}, {"origin", "", false},
		{"origin", "origin", true}, {" backup ", "origin", false},
		{"new-remote", "", true}, {"foo/bar", "", true},
	} {
		err := validateRemoteName(tc.name, tc.original, items)
		if (err == "") != tc.valid {
			t.Errorf("validateRemoteName(%q, %q) = %q", tc.name, tc.original, err)
		}
	}
}

func TestGitRemotesDialogShowsListAndEdit(t *testing.T) {
	m := &Model{}
	m.setRoot(t.TempDir())
	state := &vcsRemotesPane{items: []openidea.Remote{{Name: "origin", URL: "https://example.test/repo"}}, selected: "origin"}
	m.vcsOps = VCSOperationPane{kind: "remotes", open: true, remotes: state}
	tester := ui.NewTester(func(c *ui.Context) { m.vcsOperationDialog(c, c.Theme()) }, 900, 600)
	tester.Frame()
	for _, label := range []string{"Git Remotes", "origin", "https://example.test/repo", "Add…", "Remove", "Edit…"} {
		if !tester.HasText(label) {
			t.Fatalf("missing %q from remote dialog: %v", label, tester.Texts())
		}
	}
	if err := tester.Click("Edit…"); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	if !tester.HasText("Edit Remote") || state.name != "origin" || state.url != "https://example.test/repo" {
		t.Fatalf("remote editor not populated: %+v", state)
	}
	state.mode = "remove"
	tester.Frame()
	if !tester.HasText("Remove remote 'origin'? Its remote-tracking branches are deleted.") {
		t.Fatal("remote removal confirmation is missing")
	}
}

// Set GOIDEA_SERVER to run the actual UI operation path against openidea-server.
func TestIntegrationGitRemotes(t *testing.T) {
	server := os.Getenv("GOIDEA_SERVER")
	if server == "" {
		t.Skip("GOIDEA_SERVER is not set")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	manager := openidea.NewManager(openidea.LaunchConfig{
		ServerPath: server, Home: t.TempDir(), Log: io.Discard, StartupTimeout: 30 * time.Second,
	})
	if err := manager.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	if _, err := manager.Invoke(ctx, Window, "open_project", map[string]any{"path": root}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Invoke(ctx, Window, "project_trust_set", map[string]any{
		"path": root, "trusted": true, "folder": true,
	}); err != nil {
		t.Fatal(err)
	}
	queue := make(chan func(), 32)
	m := &Model{ctx: ctx, mgr: manager}
	m.setRoot(root)
	m.postHook = func(fn func()) {
		select {
		case queue <- fn:
		case <-ctx.Done():
		}
	}
	wait := func(predicate func() bool) {
		t.Helper()
		for !predicate() {
			select {
			case fn := <-queue:
				fn()
			case <-ctx.Done():
				t.Fatal("timed out waiting for remote operation")
			}
		}
	}
	m.OpenGitRemotes()
	state := m.vcsOps.remotes
	wait(func() bool { return !state.loading })
	if state.err != "" || len(state.items) != 0 {
		t.Fatalf("initial remotes: %+v", state)
	}
	state.editRemote(nil)
	state.url = "https://example.test/one.git"
	m.submitGitRemote(state)
	wait(func() bool { return !state.busy && !state.loading })
	if state.err != "" || state.selected != "origin" || len(state.items) != 1 || state.items[0].URL != state.url {
		t.Fatalf("after adding remote: %+v", state)
	}
	state.editRemote(state.selectedRemote())
	state.name, state.url = "upstream", "https://example.test/two.git"
	m.submitGitRemote(state)
	wait(func() bool { return !state.busy && !state.loading })
	if state.err != "" || state.selected != "upstream" || len(state.items) != 1 || state.items[0].URL != state.url {
		t.Fatalf("after editing remote: %+v", state)
	}
	state.mode = "remove"
	m.submitGitRemote(state)
	wait(func() bool { return !state.busy && !state.loading })
	if state.err != "" || len(state.items) != 0 || state.selected != "" {
		t.Fatalf("after removing remote: %+v", state)
	}
	if out, err := exec.Command("git", "-C", root, "remote").CombinedOutput(); err != nil || strings.TrimSpace(string(out)) != "" {
		t.Fatalf("git remote = %q, %v", out, err)
	}
}
