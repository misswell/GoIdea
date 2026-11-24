package openidea

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"sync"
	"testing"
	"time"
)

type runtimeTestTransport struct {
	reader *bufio.Reader
	writer *frameWriter

	stdin  *io.PipeWriter
	stdout *io.PipeReader
	stderr *io.PipeReader

	closeOnce sync.Once
	done      chan struct{}
}

func newRuntimeTestTransport(t *testing.T) *runtimeTestTransport {
	t.Helper()
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	stderrR, stderrW := io.Pipe()
	tr := &runtimeTestTransport{
		reader: bufio.NewReader(stdinR),
		writer: newFrameWriter(stdoutW),
		stdin:  stdinW,
		stdout: stdoutR,
		stderr: stderrR,
		done:   make(chan struct{}),
	}
	go func() {
		defer close(tr.done)
		defer stdoutW.Close()
		defer stderrW.Close()
		for {
			body, err := readFrame(tr.reader, nil)
			if err != nil {
				return
			}
			var request message
			if err := json.Unmarshal(body, &request); err != nil {
				return
			}
			var result any
			switch request.Method {
			case "initialize":
				result = ServerInfo{Version: "test", Protocol: 2, OS: "test", Arch: "test"}
			case "commands":
				result = []string{}
			default:
				continue
			}
			response, err := json.Marshal(map[string]any{
				"jsonrpc": "2.0", "id": request.ID, "result": result,
			})
			if err != nil || tr.writer.write(response) != nil {
				return
			}
		}
	}()
	return tr
}

func (t *runtimeTestTransport) Stdin() io.WriteCloser { return t.stdin }
func (t *runtimeTestTransport) Stdout() io.Reader     { return t.stdout }
func (t *runtimeTestTransport) Stderr() io.Reader     { return t.stderr }
func (t *runtimeTestTransport) PID() int              { return 0 }
func (t *runtimeTestTransport) Wait() error {
	<-t.done
	return nil
}
func (t *runtimeTestTransport) Close() error {
	t.closeOnce.Do(func() {
		_ = t.stdin.Close()
		_ = t.stdout.Close()
		_ = t.stderr.Close()
	})
	return nil
}

func TestManagerUseLocalClosesRemoteTransport(t *testing.T) {
	localLauncher := func(context.Context, LaunchConfig) (Transport, error) {
		return newRuntimeTestTransport(t), nil
	}
	var remote *runtimeTestTransport
	remoteLauncher := func(context.Context, LaunchConfig) (Transport, error) {
		remote = newRuntimeTestTransport(t)
		return remote, nil
	}
	m := NewManager(LaunchConfig{Launcher: localLauncher, StartupTimeout: 2 * time.Second})
	t.Cleanup(func() { _ = m.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := m.UseTransport(ctx, remoteLauncher); err != nil {
		t.Fatalf("use remote transport: %v", err)
	}
	if remote == nil || m.Server() == nil {
		t.Fatal("remote backend did not start")
	}
	remoteServer := m.Server()

	if err := m.UseLocal(ctx); err != nil {
		t.Fatalf("return to local backend: %v", err)
	}
	select {
	case <-remote.done:
	case <-time.After(2 * time.Second):
		t.Fatal("remote transport stayed open after returning to the local backend")
	}
	if m.Server() == nil || m.Server() == remoteServer {
		t.Fatal("manager did not adopt the new local backend")
	}
}

func TestManagerReconnectsAnInterruptedRemoteTransport(t *testing.T) {
	localLauncher := func(context.Context, LaunchConfig) (Transport, error) {
		return newRuntimeTestTransport(t), nil
	}
	var mu sync.Mutex
	var transports []*runtimeTestTransport
	remoteLauncher := func(context.Context, LaunchConfig) (Transport, error) {
		transport := newRuntimeTestTransport(t)
		mu.Lock()
		transports = append(transports, transport)
		mu.Unlock()
		return transport, nil
	}
	m := NewManager(LaunchConfig{Launcher: localLauncher, StartupTimeout: 2 * time.Second})
	t.Cleanup(func() { _ = m.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	if err := m.UseTransport(ctx, remoteLauncher); err != nil {
		t.Fatalf("use remote transport: %v", err)
	}
	mu.Lock()
	first := transports[0]
	mu.Unlock()
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(6 * time.Second)
	for {
		mu.Lock()
		reconnected := len(transports) >= 2
		if reconnected {
			first = transports[len(transports)-1]
		}
		mu.Unlock()
		if reconnected && m.Server() != nil && m.State().Running {
			select {
			case <-first.done:
				t.Fatal("manager adopted a transport that had already disconnected")
			default:
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("manager did not reconnect: %+v", m.State())
		}
		time.Sleep(25 * time.Millisecond)
	}
}
