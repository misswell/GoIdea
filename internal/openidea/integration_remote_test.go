package openidea_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"mayoi/internal/openidea"
)

type integrationStdioTransport struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    io.ReadCloser
	stderr    io.ReadCloser
	done      chan struct{}
	waitErr   error
	closeOnce sync.Once
}

func launchIntegrationStdioTransport(server, home string) (*integrationStdioTransport, error) {
	cmd := exec.Command(server, "--stdio")
	cmd.Env = append(os.Environ(), "OPENIDEA_SERVER_HOME="+home)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		return nil, err
	}
	transport := &integrationStdioTransport{
		cmd: cmd, stdin: stdin, stdout: stdout, stderr: stderr,
		done: make(chan struct{}),
	}
	go func() {
		transport.waitErr = cmd.Wait()
		close(transport.done)
	}()
	return transport, nil
}

func (t *integrationStdioTransport) Stdin() io.WriteCloser { return t.stdin }
func (t *integrationStdioTransport) Stdout() io.Reader     { return t.stdout }
func (t *integrationStdioTransport) Stderr() io.Reader     { return t.stderr }
func (t *integrationStdioTransport) PID() int {
	if t.cmd != nil && t.cmd.Process != nil {
		return t.cmd.Process.Pid
	}
	return 0
}

func (t *integrationStdioTransport) Wait() error {
	<-t.done
	return t.waitErr
}

func (t *integrationStdioTransport) Close() error {
	t.closeOnce.Do(func() {
		_ = t.stdin.Close()
		select {
		case <-t.done:
		case <-time.After(300 * time.Millisecond):
			if t.cmd != nil && t.cmd.Process != nil {
				_ = t.cmd.Process.Kill()
			}
			select {
			case <-t.done:
			case <-time.After(time.Second):
			}
		}
	})
	return nil
}

func TestIntegrationManagerSwitchesRealStdioBackends(t *testing.T) {
	server := os.Getenv("GOIDEA_SERVER")
	if server == "" {
		t.Skip("GOIDEA_SERVER is not set")
	}
	m := launchForTest(t)
	localProject := t.TempDir()
	remoteProject := t.TempDir()
	writeFixture(t, localProject)
	writeFixture(t, remoteProject)
	openProject(t, m, "goidea-local-it", localProject)

	remoteHome := t.TempDir()
	var remoteTransport *integrationStdioTransport
	launcher := func(context.Context, openidea.LaunchConfig) (openidea.Transport, error) {
		transport, err := launchIntegrationStdioTransport(server, remoteHome)
		if err != nil {
			return nil, err
		}
		remoteTransport = transport
		return transport, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := m.UseTransport(ctx, launcher); err != nil {
		t.Fatalf("switch to remote stdio backend: %v", err)
	}
	t.Cleanup(func() {
		if remoteTransport != nil {
			_ = remoteTransport.Close()
		}
	})
	openProject(t, m, "goidea-remote-it", remoteProject)
	raw, err := m.Invoke(ctx, "goidea-remote-it", "env_reload", nil)
	if err != nil {
		t.Fatalf("env_reload over remote stdio: %v", err)
	}
	var environment struct {
		Changed []string `json:"changed"`
	}
	if err := json.Unmarshal(raw, &environment); err != nil {
		t.Fatalf("env_reload response: %v", err)
	}

	if err := m.UseLocal(ctx); err != nil {
		t.Fatalf("switch back to the local stdio backend: %v", err)
	}
	openProject(t, m, "goidea-local-again", localProject)
	if remoteTransport == nil || remoteTransport.cmd.Process == nil {
		t.Fatal("remote backend process was not started")
	}
	select {
	case <-remoteTransport.done:
	default:
		t.Fatal("remote backend process stayed alive after switching back to local")
	}
}

func TestIntegrationPluginWorkspaceRPC(t *testing.T) {
	m := launchForTest(t)
	dir := t.TempDir()
	_, text := writeFixture(t, dir)
	const window = "goidea-plugin-workspace-it"

	for _, command := range []string{
		"plugin_workspace_find_files",
		"plugin_workspace_read_text",
		"plugin_workspace_stat",
		"plugin_workspace_write_text",
	} {
		if !m.Server().HasCommand(command) {
			t.Fatalf("server does not offer %s", command)
		}
	}
	openProject(t, m, window, dir)
	if _, err := m.Invoke(context.Background(), window, "project_trust_set", map[string]any{
		"path": dir, "trusted": true, "folder": false,
	}); err != nil {
		t.Fatalf("trust project: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	invoke := func(command string, args map[string]any) json.RawMessage {
		t.Helper()
		raw, err := m.Invoke(ctx, window, command, args)
		if err != nil {
			t.Fatalf("%s: %v (reply %s)", command, err, raw)
		}
		return raw
	}

	var got string
	if err := json.Unmarshal(invoke("plugin_workspace_read_text", map[string]any{
		"root": dir, "path": "src/demo/World.java", "request": uint32(810),
	}), &got); err != nil {
		t.Fatal(err)
	}
	if got != text {
		t.Fatalf("read text:\n%q\nwant\n%q", got, text)
	}

	var stat struct {
		IsDir bool  `json:"isDir"`
		Size  int64 `json:"size"`
		Mtime int64 `json:"mtime"`
	}
	if err := json.Unmarshal(invoke("plugin_workspace_stat", map[string]any{
		"root": dir, "path": "src/demo/World.java", "request": uint32(811),
	}), &stat); err != nil {
		t.Fatal(err)
	}
	if stat.IsDir || stat.Size != int64(len(text)) || stat.Mtime <= 0 {
		t.Fatalf("file stat = %+v", stat)
	}

	invoke("plugin_workspace_write_text", map[string]any{
		"root": dir, "path": "src/plugin/Generated.java", "text": "class Generated {}", "request": uint32(812),
	})
	generated, err := os.ReadFile(filepath.Join(dir, "src", "plugin", "Generated.java"))
	if err != nil {
		t.Fatal(err)
	}
	if string(generated) != "class Generated {}" {
		t.Fatalf("generated text = %q", generated)
	}

	var files []string
	deadline := time.Now().Add(15 * time.Second)
	for {
		if err := json.Unmarshal(invoke("plugin_workspace_find_files", map[string]any{
			"root": dir, "glob": "*.java", "limit": 100, "request": uint32(813),
		}), &files); err != nil {
			t.Fatal(err)
		}
		if len(files) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("plugin workspace index did not return World.java")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !containsString(files, "src/demo/World.java") {
		t.Fatalf("find files = %v", files)
	}

	_, err = m.Invoke(ctx, window, "plugin_workspace_write_text", map[string]any{
		"root": dir, "path": ".git/config", "text": "unsafe", "request": uint32(814),
	})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "invalid") {
		t.Fatalf("write inside .git error = %v", err)
	}

	if _, err := m.Invoke(ctx, window, "cancel_request", map[string]any{"id": uint32(815)}); err != nil {
		t.Fatal(err)
	}
	_, err = m.Invoke(ctx, window, "plugin_workspace_stat", map[string]any{
		"root": dir, "path": "src/demo/World.java", "request": uint32(815),
	})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "cancel") {
		t.Fatalf("canceled stat error = %v", err)
	}

	if runtime.GOOS != "windows" {
		outside := filepath.Join(t.TempDir(), "secret.txt")
		if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(dir, "src", "escape.txt")); err != nil {
			t.Fatal(err)
		}
		_, err = m.Invoke(ctx, window, "plugin_workspace_read_text", map[string]any{
			"root": dir, "path": "src/escape.txt", "request": uint32(816),
		})
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "outside") {
			t.Fatalf("symlink escape error = %v", err)
		}
	}

}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
