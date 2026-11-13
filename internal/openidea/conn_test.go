package openidea

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

type testPeer struct {
	reader *bufio.Reader
	writer *frameWriter
}

func newConnPair(t *testing.T, opts Options) (*Conn, *testPeer) {
	t.Helper()
	clientR, serverW := io.Pipe()
	serverR, clientW := io.Pipe()
	c := NewConn(clientR, clientW, opts)
	t.Cleanup(func() {
		c.Close()
		_ = serverW.Close()
		_ = serverR.Close()
	})
	return c, &testPeer{reader: bufio.NewReader(serverR), writer: newFrameWriter(serverW)}
}

func (p *testPeer) read(t *testing.T) message {
	t.Helper()
	body, err := readFrame(p.reader, nil)
	if err != nil {
		t.Fatalf("server read: %v", err)
	}
	var m message
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("server decode: %v", err)
	}
	return m
}

func (p *testPeer) send(t *testing.T, m any) {
	t.Helper()
	body, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.writer.write(body); err != nil {
		t.Fatalf("server write: %v", err)
	}
}

func TestConnConcurrentCalls(t *testing.T) {
	c, peer := newConnPair(t, Options{})
	const n = 8
	var wg sync.WaitGroup
	results := make([]string, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			raw, err := c.Call(context.Background(), "invoke", map[string]any{"window": "w", "cmd": "echo", "args": i})
			if err != nil {
				t.Errorf("call %d: %v", i, err)
				return
			}
			results[i] = string(raw)
		}(i)
	}
	requests := make([]message, 0, n)
	for i := 0; i < n; i++ {
		requests = append(requests, peer.read(t))
	}
	// Answer in reverse to prove correlation is by id, not by arrival order.
	for i := len(requests) - 1; i >= 0; i-- {
		req := requests[i]
		var params struct {
			Args int `json:"args"`
		}
		_ = json.Unmarshal(req.Params, &params)
		peer.send(t, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{"echo": params.Args}})
	}
	wg.Wait()
	for i, got := range results {
		want := `{"echo":` + json.Number(rune('0'+i)).String() + `}`
		var decoded struct {
			Echo int `json:"echo"`
		}
		if err := json.Unmarshal([]byte(got), &decoded); err != nil || decoded.Echo != i {
			t.Fatalf("call %d = %q (want echo %d, err %v)", i, got, i, err)
		}
		_ = want
	}
}

func TestConnNotification(t *testing.T) {
	c, peer := newConnPair(t, Options{})
	peer.send(t, map[string]any{"jsonrpc": "2.0", "method": "event", "params": map[string]any{"window": "w", "event": map[string]any{"type": "docDirty"}}})
	select {
	case n := <-c.Notifications():
		var we WindowEvent
		if err := json.Unmarshal(n.Params, &we); err != nil || n.Method != "event" || EventType(we.Event) != "docDirty" {
			t.Fatalf("notification = %+v (err %v)", n, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no notification")
	}
}

func TestConnCancellation(t *testing.T) {
	c, peer := newConnPair(t, Options{})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		_, err := c.Call(ctx, "slow", nil)
		errCh <- err
	}()
	// The server reads the request but never answers before the context ends.
	_ = peer.read(t)
	select {
	case err := <-errCh:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("call did not return")
	}
}

func TestConnServerRequest(t *testing.T) {
	c, peer := newConnPair(t, Options{Handler: func(method string, params json.RawMessage) (any, *Error) {
		if method != "plugin_secret" {
			return nil, &Error{Code: CodeMethodNotFound, Message: "no"}
		}
		return "hunter2", nil
	}})
	peer.send(t, map[string]any{"jsonrpc": "2.0", "id": 99, "method": "plugin_secret", "params": map[string]any{"plugin": "p", "name": "n"}})
	reply := peer.read(t)
	if string(reply.ID) != "99" || string(reply.Result) != `"hunter2"` {
		t.Fatalf("reply = %+v result=%s", reply, reply.Result)
	}
	_ = c
}

func TestConnServerErrorMapped(t *testing.T) {
	c, peer := newConnPair(t, Options{})
	go func() {
		req := peer.read(t)
		peer.send(t, map[string]any{"jsonrpc": "2.0", "id": req.ID, "error": map[string]any{
			"code":    CodeInternalError,
			"message": `{"kind":"outOfSync","message":"stale","data":null}`,
		}})
	}()
	_, err := c.Call(context.Background(), "invoke", nil)
	if !IsOutOfSync(err) {
		t.Fatalf("err = %v, want outOfSync", err)
	}
	var e *Error
	if !errors.As(err, &e) || e.Code != CodeInternalError || e.Kind != "outOfSync" {
		t.Fatalf("error = %+v", err)
	}
}

func TestConnOverflowFailsFast(t *testing.T) {
	c, peer := newConnPair(t, Options{QueueSize: 1})
	peer.send(t, map[string]any{"jsonrpc": "2.0", "method": "event", "params": map[string]any{"type": "a"}})
	peer.send(t, map[string]any{"jsonrpc": "2.0", "method": "event", "params": map[string]any{"type": "b"}})
	select {
	case <-c.Done():
		if !errors.Is(c.Err(), ErrOverflow) {
			t.Fatalf("err = %v, want overflow", c.Err())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("connection did not close on overflow")
	}
}
