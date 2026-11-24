package app

import (
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"mayoi/internal/openidea"
)

func TestRemoteContainerTunnelKeepsTheReplyAfterClientHalfClose(t *testing.T) {
	dir := t.TempDir()
	docker := filepath.Join(dir, "docker")
	script := "#!/bin/sh\ncat >/dev/null\nsleep 0.1\nprintf final-reply\n"
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
	defer forwarder.Close()
	forwarded, err := forwarder.Forward(18080, nil)
	if err != nil {
		t.Fatal(err)
	}
	connection, err := net.DialTimeout(
		"tcp",
		net.JoinHostPort("127.0.0.1", strconv.Itoa(int(forwarded.LocalPort))),
		2*time.Second,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if _, err := connection.Write([]byte("request")); err != nil {
		t.Fatal(err)
	}
	tcp := connection.(*net.TCPConn)
	if err := tcp.CloseWrite(); err != nil {
		t.Fatal(err)
	}
	if err := connection.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	reply, err := io.ReadAll(connection)
	if err != nil {
		t.Fatal(err)
	}
	if string(reply) != "final-reply" {
		t.Fatalf("forwarded reply = %q, want the complete response after input EOF", reply)
	}
}
