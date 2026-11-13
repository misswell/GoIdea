// Package openidea speaks the OpenIDEA desktop protocol: JSON-RPC 2.0 with
// Content-Length framing over `openidea-server --stdio`.
//
// The server is the Java IDE backend (project model, JDT language server,
// Maven/Gradle builds, run and debug). GoIDEA never reimplements it; this
// package is the transport and process lifecycle around it.
package openidea

import (
	"encoding/json"
	"errors"
	"fmt"
)

// JSON-RPC error codes used by the protocol.
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
)

// Error is a failure reported by openidea-server. The server wraps its
// IdeError JSON in the message of an INTERNAL_ERROR reply; Kind exposes that
// structure so callers can react to outOfSync without string matching.
type Error struct {
	Code    int64
	Kind    string
	Message string
	Data    json.RawMessage
}

func (e *Error) Error() string {
	if e.Kind != "" {
		return fmt.Sprintf("openidea: %s: %s", e.Kind, e.Message)
	}
	return fmt.Sprintf("openidea: %s", e.Message)
}

// IsOutOfSync reports whether err is the document version mismatch the bridge
// recovers from with doc_snapshot.
func IsOutOfSync(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Kind == "outOfSync"
}

// ErrorKind returns the server's IdeError kind, or "" for other errors.
func ErrorKind(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return ""
}

// internalError is the reply the server gives for every invoke failure.
func parseServerError(code int64, message string) *Error {
	var payload struct {
		Kind    string          `json:"kind"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(message), &payload); err == nil && payload.Kind != "" {
		return &Error{Code: code, Kind: payload.Kind, Message: payload.Message, Data: payload.Data}
	}
	return &Error{Code: code, Message: message}
}

// ErrClosed is returned to calls when the connection or process is gone.
var ErrClosed = errors.New("openidea: connection closed")

// ErrOverflow is returned when the notification consumer falls too far behind:
// the connection is closed rather than letting an unbounded queue grow.
var ErrOverflow = errors.New("openidea: notification queue overflow")
