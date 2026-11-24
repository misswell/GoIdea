//go:build unix

package app

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"mayoi/internal/openidea"
)

func TestRemoteTunnelCloseTerminatesTheWholeProcessGroup(t *testing.T) {
	dir := t.TempDir()
	childPIDFile := filepath.Join(dir, "child.pid")
	docker := filepath.Join(dir, "docker")
	script := "#!/bin/sh\ncat >/dev/null\nsleep 30 &\nprintf '%s\\n' \"$!\" > " +
		strconv.Quote(childPIDFile) + "\nwait\n"
	if err := os.WriteFile(docker, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	backend := &remoteBackend{
		target: openidea.RemoteTarget{Kind: openidea.RemoteTargetDevContainer},
		runner: defaultRemoteRunner,
		container: &remoteContainerInfo{
			Docker:    docker,
			Container: "container-id",
			User:      "vscode",
		},
		remoteBin: "/home/vscode/.openidea/server/test/openidea-server",
	}
	forwarder := newRemotePortForwarder(backend)
	forwarded, err := forwarder.Forward(18081, nil)
	if err != nil {
		t.Fatal(err)
	}
	connection, err := net.DialTimeout(
		"tcp",
		net.JoinHostPort("127.0.0.1", strconv.Itoa(int(forwarded.LocalPort))),
		2*time.Second,
	)
	if err != nil {
		forwarder.Close()
		t.Fatal(err)
	}
	if _, err := connection.Write([]byte("start")); err != nil {
		_ = connection.Close()
		forwarder.Close()
		t.Fatal(err)
	}
	if err := connection.(*net.TCPConn).CloseWrite(); err != nil {
		_ = connection.Close()
		forwarder.Close()
		t.Fatal(err)
	}
	defer connection.Close()

	var childPID int
	deadline := time.Now().Add(3 * time.Second)
	for childPID == 0 {
		raw, err := os.ReadFile(childPIDFile)
		if err == nil {
			childPID, _ = strconv.Atoi(strings.TrimSpace(string(raw)))
		}
		if childPID > 0 {
			break
		}
		if time.Now().After(deadline) {
			forwarder.Close()
			t.Fatal("the tunnel's child process did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}

	closed := make(chan struct{})
	go func() {
		forwarder.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("closing the remote tunnel waited on a lingering child")
	}

	deadline = time.Now().Add(3 * time.Second)
	for {
		err := syscall.Kill(childPID, 0)
		if errors.Is(err, syscall.ESRCH) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("child process %d survived tunnel close: %v", childPID, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
