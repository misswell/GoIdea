package openidea

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"sync/atomic"
)

// Notification is a message the server sent without an id: an event, a channel
// value, or a plugin event.
type Notification struct {
	Method string
	Params json.RawMessage
}

// RequestHandler answers a server-to-client request (for example
// plugin_secret) and runs off the reader goroutine.
type RequestHandler func(method string, params json.RawMessage) (any, *Error)

// Options tune a connection.
type Options struct {
	// QueueSize bounds the notification queue; when it fills, the connection is
	// closed with ErrOverflow rather than growing without limit.
	QueueSize int
	OnNoise   func(line string)
	Handler   RequestHandler
}

type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *remoteError    `json:"error,omitempty"`
}

type remoteError struct {
	Code    int64           `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Conn is a JSON-RPC 2.0 endpoint over a framed byte stream. Calls may run
// concurrently; writes are serialized; the reader never blocks on consumers.
type Conn struct {
	out     *frameWriter
	pending map[string]chan *message
	mu      sync.Mutex
	nextID  atomic.Int64

	notifications chan Notification
	done          chan struct{}
	closeOnce     sync.Once
	err           atomic.Pointer[error]

	handler RequestHandler
}

// NewConn reads and writes framed messages on r and w, then returns at once;
// the reader runs until the stream closes or the connection is closed.
func NewConn(r io.Reader, w io.Writer, opts Options) *Conn {
	queue := opts.QueueSize
	if queue <= 0 {
		queue = 8192
	}
	c := &Conn{
		out:           newFrameWriter(w),
		pending:       make(map[string]chan *message),
		notifications: make(chan Notification, queue),
		done:          make(chan struct{}),
		handler:       opts.Handler,
	}
	go c.readLoop(bufio.NewReaderSize(r, 64<<10), opts.OnNoise)
	return c
}

func (c *Conn) Notifications() <-chan Notification { return c.notifications }

// Done is closed when the connection closes; Err then says why.
func (c *Conn) Done() <-chan struct{} { return c.done }

// Err returns why the connection closed, nil while it is open.
func (c *Conn) Err() error {
	if p := c.err.Load(); p != nil {
		return *p
	}
	return nil
}

func (c *Conn) readLoop(r *bufio.Reader, onNoise func(string)) {
	for {
		body, err := readFrame(r, onNoise)
		if err != nil {
			if errors.Is(err, io.EOF) {
				err = ErrClosed
			}
			c.shutdown(err)
			return
		}
		var m message
		if err := json.Unmarshal(body, &m); err != nil {
			c.shutdown(fmt.Errorf("openidea: invalid message: %w", err))
			return
		}
		switch {
		case m.Method != "" && len(m.ID) > 0:
			go c.answerRequest(m)
		case m.Method != "":
			c.deliver(Notification{Method: m.Method, Params: m.Params})
		default:
			if os.Getenv("GOIDEA_RPC_TRACE") == "1" {
				log.Printf("openidea: response %s: %s", m.ID, body)
			}
			if len(m.Result) == 0 && m.Error == nil {
				// A reply with neither result nor error hides backend bugs: keep
				// the raw body so the caller's "empty result" has a cause in the log.
				log.Printf("openidea: response %s has neither result nor error: %s", m.ID, body)
			}
			c.complete(&m)
		}
	}
}

func (c *Conn) deliver(n Notification) {
	select {
	case c.notifications <- n:
	case <-c.done:
	default:
		// A consumer this far behind would lose editor state; fail loudly so the
		// supervisor restarts the backend and documents resync from snapshots.
		c.shutdown(ErrOverflow)
	}
}

func (c *Conn) answerRequest(m message) {
	var (
		result any
		rerr   *Error
	)
	if c.handler == nil {
		rerr = &Error{Code: CodeMethodNotFound, Message: fmt.Sprintf("unhandled method %s", m.Method)}
	} else {
		result, rerr = c.handler(m.Method, m.Params)
	}
	reply := message{JSONRPC: "2.0", ID: m.ID}
	if rerr != nil {
		reply.Error = &remoteError{Code: rerr.Code, Message: rerr.Message}
	} else {
		raw, err := json.Marshal(result)
		if err != nil {
			reply.Error = &remoteError{Code: CodeInternalError, Message: err.Error()}
		} else {
			reply.Result = raw
		}
	}
	body, err := json.Marshal(reply)
	if err != nil {
		return
	}
	_ = c.out.write(body)
}

func (c *Conn) complete(m *message) {
	key := string(m.ID)
	c.mu.Lock()
	ch := c.pending[key]
	delete(c.pending, key)
	c.mu.Unlock()
	if ch != nil {
		ch <- m
	}
}

// Call sends a request and waits for its response, the context ending, or the
// connection closing.
func (c *Conn) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := c.nextID.Add(1)
	key := fmt.Sprintf("%d", id)
	ch := make(chan *message, 1)
	c.mu.Lock()
	select {
	case <-c.done:
		c.mu.Unlock()
		if err := c.Err(); err != nil {
			return nil, err
		}
		return nil, ErrClosed
	default:
	}
	c.pending[key] = ch
	c.mu.Unlock()

	req := map[string]any{"jsonrpc": "2.0", "id": id, "method": method}
	if params != nil {
		req["params"] = params
	}
	body, err := json.Marshal(req)
	if err != nil {
		c.removePending(key)
		return nil, err
	}
	if err := c.out.write(body); err != nil {
		c.removePending(key)
		return nil, err
	}

	select {
	case m := <-ch:
		if m.Error != nil {
			return nil, parseServerError(m.Error.Code, m.Error.Message)
		}
		return m.Result, nil
	case <-ctx.Done():
		c.removePending(key)
		return nil, ctx.Err()
	case <-c.done:
		if err := c.Err(); err != nil {
			return nil, err
		}
		return nil, ErrClosed
	}
}

func (c *Conn) removePending(key string) {
	c.mu.Lock()
	delete(c.pending, key)
	c.mu.Unlock()
}

// Notify sends a notification.
func (c *Conn) Notify(method string, params any) error {
	msg := map[string]any{"jsonrpc": "2.0", "method": method}
	if params != nil {
		msg["params"] = params
	}
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return c.out.write(body)
}

// Close stops the connection; in-flight calls fail with ErrClosed.
func (c *Conn) Close() error { c.shutdown(ErrClosed); return nil }

func (c *Conn) shutdown(err error) {
	c.closeOnce.Do(func() {
		if err == nil {
			err = ErrClosed
		}
		c.err.Store(&err)
		close(c.done)
		c.mu.Lock()
		pending := c.pending
		c.pending = make(map[string]chan *message)
		c.mu.Unlock()
		for _, ch := range pending {
			select {
			case ch <- &message{Error: &remoteError{Code: CodeInternalError, Message: err.Error()}}:
			default:
			}
		}
	})
}
