package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mayoi/internal/openidea"
)

// TestIntegrationDevContainerAcceptance is intentionally opt-in because it
// starts a real Dev Containers CLI, Docker container and remote server.
func TestIntegrationDevContainerAcceptance(t *testing.T) {
	if os.Getenv("GOIDEA_DEVCONTAINER_ACCEPTANCE") != "1" {
		t.Skip("set GOIDEA_DEVCONTAINER_ACCEPTANCE=1 to run the real container gate")
	}
	cli := strings.TrimSpace(os.Getenv("GOIDEA_DEVCONTAINER_CLI"))
	if cli == "" {
		t.Fatal("GOIDEA_DEVCONTAINER_CLI must name the Dev Containers CLI")
	}
	docker, err := exec.LookPath("docker")
	if err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(t.TempDir(), "devcontainer-acceptance")
	configDir := filepath.Join(folder, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	port := freeTCPPort(t)
	image := strings.TrimSpace(os.Getenv("GOIDEA_DEVCONTAINER_IMAGE"))
	if image == "" {
		image = "golang:1.27-bookworm"
	}
	config, err := json.Marshal(map[string]any{
		"image":        image,
		"remoteUser":   "root",
		"forwardPorts": []uint16{port},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.json"), config, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	version := strings.TrimSpace(os.Getenv("GOIDEA_SERVER_VERSION"))
	if version == "" {
		version = "0.1.54"
	}
	localServer := strings.TrimSpace(os.Getenv("GOIDEA_DEVCONTAINER_SERVER_BINARY"))
	if localServer == "" {
		localServer = strings.TrimSpace(os.Getenv("OPENIDEA_SERVER_BINARY"))
	}
	if localServer != "" {
		info, err := os.Stat(localServer)
		if err != nil {
			t.Fatalf("dev container server binary: %v", err)
		}
		if !info.Mode().IsRegular() {
			t.Fatalf("dev container server binary is not a regular file: %s", localServer)
		}
		// The container target asks serverBinary for linux-aarch64 even when the
		// host is macOS. This explicit override is how a fork-built Linux binary
		// reaches the container acceptance test.
		t.Setenv("OPENIDEA_SERVER_BINARY", localServer)
	}
	backend := newRemoteBackend(
		openidea.RemoteTarget{
			Kind:   openidea.RemoteTargetDevContainer,
			Folder: folder,
			Config: ".devcontainer/devcontainer.json",
		},
		openidea.SSHConfiguration{},
		version,
		t.TempDir(),
		"",
		os.Stderr,
	)
	backend.devContainerCLI = cli
	t.Cleanup(func() {
		backend.closeForwards()
		if backend.container != nil && backend.container.Container != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_ = exec.CommandContext(ctx, docker, "rm", "-f", backend.container.Container).Run()
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	if err := backend.prepare(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if backend.container == nil {
		t.Fatal("devcontainer up returned no container")
	}

	const echoSource = `package main

import (
	"io"
	"net"
	"os"
)

func main() {
	listener, err := net.Listen("tcp", "127.0.0.1:"+os.Args[1])
	if err != nil {
		panic(err)
	}
	for {
		connection, err := listener.Accept()
		if err != nil {
			panic(err)
		}
		go func() {
			defer connection.Close()
			_, _ = io.Copy(connection, connection)
		}()
	}
}
`
	script := `set -e
cat > "$1"
go build -o /tmp/goidea-echo "$1"
nohup /tmp/goidea-echo "$2" >/tmp/goidea-echo.log 2>&1 </dev/null &
`
	if _, err := backend.runRemote(
		ctx,
		bytes.NewBufferString(echoSource),
		2*time.Minute,
		script,
		[]string{"/tmp/goidea-echo.go", fmt.Sprint(port)},
	); err != nil {
		t.Fatal(err)
	}
	if err := backend.forwardConfiguredPorts(); err != nil {
		t.Fatal(err)
	}
	forwards := backend.listForwards()
	if len(forwards) != 1 || forwards[0].RemotePort != port {
		t.Fatalf("configured forwards = %+v", forwards)
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		connection, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", fmt.Sprint(forwards[0].LocalPort)), 2*time.Second)
		if err == nil {
			_ = connection.SetDeadline(time.Now().Add(5 * time.Second))
			_, writeErr := io.WriteString(connection, "goidea-devcontainer")
			reply := make([]byte, len("goidea-devcontainer"))
			_, readErr := io.ReadFull(connection, reply)
			_ = connection.Close()
			if writeErr == nil && readErr == nil && string(reply) == "goidea-devcontainer" {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("container tunnel did not echo: %v", err)
		}
		time.Sleep(200 * time.Millisecond)
	}

	server, err := openidea.Launch(ctx, openidea.LaunchConfig{
		Launcher:       backend.launcher(),
		Home:           t.TempDir(),
		Log:            os.Stderr,
		StartupTimeout: 90 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
	raw, err := server.Invoke(ctx, "main", "open_project", map[string]any{"path": backend.container.Folder})
	if err != nil {
		t.Fatalf("open remote project: %v (reply %s)", err, raw)
	}
	if !server.HasCommand("plugin_workspace_read_text") {
		t.Fatal("the dev container server does not expose plugin_workspace_read_text")
	}
	var text string
	if err := server.InvokeInto(ctx, "main", "plugin_workspace_read_text", map[string]any{
		"root": backend.container.Folder, "path": "main.go", "request": uint32(901),
	}, &text); err != nil {
		t.Fatalf("remote plugin workspace read: %v", err)
	}
	if text != "package main\n" {
		t.Fatalf("remote plugin workspace text = %q", text)
	}
}
