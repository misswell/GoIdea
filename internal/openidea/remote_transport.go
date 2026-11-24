package openidea

import (
	"context"
	"io"
)

// Transport is one bidirectional openidea-server protocol stream. SSH exec,
// WSL and docker exec all fit this seam, so the JSON-RPC framing and command
// routing stay identical to a local backend.
type Transport interface {
	Stdin() io.WriteCloser
	Stdout() io.Reader
	Stderr() io.Reader
	Wait() error
	Close() error
	PID() int
}

// TransportLauncher starts a transport for one openidea-server connection.
type TransportLauncher func(context.Context, LaunchConfig) (Transport, error)
