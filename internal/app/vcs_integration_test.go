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

	"mayoi/internal/openidea"
)

// TestIntegrationVCSToolWindowLocalRemote drives the native Git operation
// path against a real openidea-server and a real bare repository used as the
// remote. It is skipped unless GOIDEA_SERVER points at the pinned server.
func TestIntegrationVCSToolWindowLocalRemote(t *testing.T) {
	server := os.Getenv("GOIDEA_SERVER")
	if server == "" {
		t.Skip("GOIDEA_SERVER is not set")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runGit := func(args ...string) {
		t.Helper()
		command := exec.Command("git", append([]string{"-C", root}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	runGit("init", "-q")
	runGit("config", "user.name", "GoIDEA integration")
	runGit("config", "user.email", "goidea-integration@invalid")
	if err := os.WriteFile(filepath.Join(root, "README.txt"), []byte("baseline\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "README.txt")
	runGit("commit", "-qm", "baseline")

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	manager := openidea.NewManager(openidea.LaunchConfig{
		ServerPath: server, Home: t.TempDir(), Log: io.Discard,
		StartupTimeout: 30 * time.Second,
	})
	if err := manager.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	if _, err := manager.Invoke(ctx, Window, "open_project", map[string]any{"path": root}); err != nil {
		t.Fatal(err)
	}
	m := &Model{
		ctx: ctx, mgr: manager,
		postHook: func(fn func()) { fn() },
	}
	m.setRoot(root)

	result, err := m.SmokeVCSOperations(ctx)
	if err != nil {
		t.Fatalf("native VCS local-remote flow: %v: %+v", err, result)
	}
	for _, key := range []string{
		"remoteListed", "worktreeCreated", "worktreeMoved", "worktreeRemoved",
		"fetched", "pushed", "pulled", "branchCreated", "rebased", "merged", "reset",
	} {
		if result[key] != true {
			t.Errorf("%s = %#v, want true (result %+v)", key, result[key], result)
		}
	}
}

// TestIntegrationVCSP1Safety covers the P1 commit, rollback and worktree
// branches against real Git repositories and the pinned openidea-server.
func TestIntegrationVCSP1Safety(t *testing.T) {
	server := os.Getenv("GOIDEA_SERVER")
	if server == "" {
		t.Skip("GOIDEA_SERVER is not set")
	}
	project, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first := filepath.Join(project, "first")
	second := filepath.Join(project, "second")
	initVCSIntegrationRepo(t, first)
	initVCSIntegrationRepo(t, second)
	writeVCSIntegrationFile(t, first, "multi.txt", "base first\n")
	writeVCSIntegrationFile(t, first, "lines.txt", "a\nb\nc\nd\ne\n")
	writeVCSIntegrationFile(t, first, "old.txt", "old\n")
	writeVCSIntegrationFile(t, first, "rollback.txt", "base\n")
	writeVCSIntegrationFile(t, second, "multi.txt", "base second\n")
	runVCSIntegrationGit(t, first, "add", "-A")
	runVCSIntegrationGit(t, first, "commit", "-qm", "baseline first")
	runVCSIntegrationGit(t, second, "add", "-A")
	runVCSIntegrationGit(t, second, "commit", "-qm", "baseline second")

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	t.Cleanup(cancel)
	manager := openidea.NewManager(openidea.LaunchConfig{
		ServerPath: server, Home: t.TempDir(), Log: io.Discard,
		StartupTimeout: 30 * time.Second,
	})
	if err := manager.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	openVCSIntegrationProject(t, ctx, manager, project)
	m := &Model{
		ctx: ctx, mgr: manager,
		postHook: func(fn func()) { fn() },
	}
	m.setRoot(project)

	writeVCSIntegrationFile(t, first, "multi.txt", "changed first\n")
	writeVCSIntegrationFile(t, second, "multi.txt", "changed second\n")
	refreshVCSIntegration(t, m)
	if len(m.vcs.repositories) != 2 {
		t.Fatalf("discovered repositories = %+v", m.vcs.repositories)
	}
	m.vcs.current = second
	m.OpenCommit()
	if m.commit == nil {
		t.Fatal("multi-repository Commit did not open")
	}
	m.commit.message = "Commit both repositories"
	m.submitCommit()
	waitVCSIntegrationCommit(t, m)
	for _, root := range []string{first, second} {
		if got := runVCSIntegrationGit(t, root, "status", "--porcelain"); got != "" {
			t.Fatalf("%s is not clean after multi-repository Commit: %q", root, got)
		}
		if got := strings.TrimSpace(runVCSIntegrationGit(
			t, root, "log", "-1", "--format=%s",
		)); got != "Commit both repositories" {
			t.Fatalf("%s latest commit = %q", root, got)
		}
	}

	writeVCSIntegrationFile(t, first, "lines.txt", "A\nb\nc\nd\nE\n")
	refreshVCSIntegration(t, m)
	m.vcs.current = first
	m.OpenCommit()
	if m.commit == nil {
		t.Fatal("partial Commit did not open")
	}
	linesPath := filepath.Join(first, "lines.txt")
	_, disk, hunks, err := m.commitHunks(ctx, linesPath)
	if err != nil {
		t.Fatalf("read partial hunks: %v", err)
	}
	if len(hunks) != 2 {
		t.Fatalf("line hunks = %#v, want two independent changes", hunks)
	}
	keys := commitHunkKeys(disk, hunks)
	state := &commitHunksState{
		hunks: hunks, keys: keys, selected: map[string]bool{keys[0]: true},
	}
	m.commit.message = "Commit only the first hunk"
	m.commit.checks[linesPath] = true
	m.commit.hunks[linesPath] = state
	m.submitCommit()
	waitVCSIntegrationCommit(t, m)
	if got := runVCSIntegrationGit(t, first, "show", "HEAD:lines.txt"); got != "A\nb\nc\nd\ne\n" {
		t.Fatalf("partial commit content = %q", got)
	}
	if got := readVCSIntegrationFile(t, linesPath); got != "A\nb\nc\nd\nE\n" {
		t.Fatalf("working tree after partial commit = %q", got)
	}

	runVCSIntegrationGit(t, first, "mv", "old.txt", "new.txt")
	writeVCSIntegrationFile(t, first, "new.txt", "renamed and changed\n")
	writeVCSIntegrationFile(t, first, "added.txt", "added\n")
	runVCSIntegrationGit(t, first, "add", "added.txt")
	writeVCSIntegrationFile(t, first, "rollback.txt", "changed\n")
	refreshVCSIntegration(t, m)
	m.AskRollback(openidea.FileStatus{Path: first, RelPath: "first"})
	if m.rollback == nil {
		t.Fatal("folder rollback did not include the changed files")
	}
	m.rollback.deleteAdded = true
	m.ConfirmRollback()
	waitVCSIntegrationRollback(t, m)
	if got := readVCSIntegrationFile(t, filepath.Join(first, "old.txt")); got != "old\n" {
		t.Fatalf("renamed source after rollback = %q", got)
	}
	if _, err := os.Stat(filepath.Join(first, "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("renamed destination still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(first, "added.txt")); !os.IsNotExist(err) {
		t.Fatalf("explicitly deleted addition still exists: %v", err)
	}
	if got := readVCSIntegrationFile(t, filepath.Join(first, "rollback.txt")); got != "base\n" {
		t.Fatalf("modified file after rollback = %q", got)
	}
	if got := runVCSIntegrationGit(t, first, "status", "--porcelain"); got != "" {
		t.Fatalf("rollback left changes: %q", got)
	}

	worktree, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runVCSIntegrationGit(t, first, "branch", "held")
	runVCSIntegrationGit(t, first, "worktree", "add", "-q", worktree, "held")
	m.vcs.current = first
	m.branch.open = true
	m.deleteLocalBranch(openidea.Branch{Name: "held"})
	waitVCSIntegration(t, "worktree deletion confirmation", 30*time.Second, func() bool {
		return m.branch.prompt == "delete-worktree" && !m.branch.busy
	})
	m.submitBranchPrompt()
	waitVCSIntegration(t, "worktree-held branch deletion", 2*time.Minute, func() bool {
		return m.branch.prompt == "" && !m.branch.busy
	})
	if got := runVCSIntegrationGit(t, first, "branch", "--list", "held"); got != "" {
		t.Fatalf("held branch still exists: %q", got)
	}
	if _, err := os.Stat(worktree); !os.IsNotExist(err) {
		t.Fatalf("linked worktree still exists: %v", err)
	}
}

func initVCSIntegrationRepo(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	runVCSIntegrationGit(t, root, "init", "-q")
	runVCSIntegrationGit(t, root, "config", "user.name", "GoIDEA integration")
	runVCSIntegrationGit(t, root, "config", "user.email", "goidea-integration@invalid")
}

func runVCSIntegrationGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	output, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
	return string(output)
}

func writeVCSIntegrationFile(t *testing.T, root, name, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readVCSIntegrationFile(t *testing.T, path string) string {
	t.Helper()
	text, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(text)
}

func openVCSIntegrationProject(
	t *testing.T,
	ctx context.Context,
	manager *openidea.Manager,
	root string,
) {
	t.Helper()
	if _, err := manager.Invoke(ctx, Window, "open_project", map[string]any{"path": root}); err != nil {
		t.Fatal(err)
	}
	waitVCSIntegration(t, "project open", 60*time.Second, func() bool {
		raw, err := manager.Invoke(ctx, Window, "project_current", nil)
		return err == nil && strings.TrimSpace(string(raw)) != "null"
	})
	if _, err := manager.Invoke(ctx, Window, "project_trust_set", map[string]any{
		"path": root, "trusted": true, "folder": true,
	}); err != nil {
		t.Fatal(err)
	}
}

func refreshVCSIntegration(t *testing.T, m *Model) {
	t.Helper()
	generation := m.vcs.refreshGeneration
	m.RefreshVCS()
	waitVCSIntegration(t, "VCS refresh", 30*time.Second, func() bool {
		return !m.vcs.loading && m.vcs.refreshGeneration > generation
	})
	if m.vcs.err != "" {
		t.Fatalf("VCS refresh: %s", m.vcs.err)
	}
}

func waitVCSIntegrationCommit(t *testing.T, m *Model) {
	t.Helper()
	waitVCSIntegration(t, "commit", 3*time.Minute, func() bool {
		if m.commit == nil {
			return true
		}
		if !m.commit.busy && m.commit.err != "" {
			t.Fatalf("commit: %s", m.commit.err)
		}
		return false
	})
}

func waitVCSIntegrationRollback(t *testing.T, m *Model) {
	t.Helper()
	waitVCSIntegration(t, "rollback", time.Minute, func() bool {
		if m.rollback == nil {
			return true
		}
		if !m.rollback.busy && m.rollback.err != "" {
			t.Fatalf("rollback: %s", m.rollback.err)
		}
		return false
	})
}

func waitVCSIntegration(
	t *testing.T,
	what string,
	timeout time.Duration,
	ready func() bool,
) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if ready() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(25 * time.Millisecond)
	}
}
