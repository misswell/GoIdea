package openidea

import (
	"context"
	"errors"
	"sync"
	"time"
)

// State is the manager's view of the backend, for the status bar and for
// features that must wait for the backend to come back.
type State struct {
	Running  bool
	Info     ServerInfo
	Err      error
	Restarts int
}

// Manager owns one openidea-server for the application and survives its
// crashes: calls wait for a restart, and OnRestart reopens the projects and
// documents the UI had.
type Manager struct {
	cfg LaunchConfig

	mu       sync.Mutex
	srv      *Server
	closed   bool
	started  bool
	restarts int
	lastErr  error
	state    State

	notify   chan Notification
	status   chan State
	closedCh chan struct{}

	subMu sync.Mutex
	subs  map[chan Notification]struct{}

	startMu   sync.Mutex
	onRestart func(context.Context) error

	forwards sync.WaitGroup
}

// NewManager returns a manager for cfg; Start or the first Ensure launches the
// server.
func NewManager(cfg LaunchConfig) *Manager {
	if cfg.Protocol == 0 {
		cfg.Protocol = 2
	}
	return &Manager{
		cfg:      cfg,
		notify:   make(chan Notification, 8192),
		status:   make(chan State, 16),
		closedCh: make(chan struct{}),
	}
}

// OnRestart sets the callback that restores project and document state after
// the backend restarts. It runs after the new server answers initialize.
func (m *Manager) OnRestart(fn func(context.Context) error) {
	m.mu.Lock()
	m.onRestart = fn
	m.mu.Unlock()
}

// Start launches the server.
func (m *Manager) Start(ctx context.Context) error {
	_, err := m.Ensure(ctx)
	return err
}

// Server returns the running server, or nil.
func (m *Manager) Server() *Server {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.srv != nil && m.srv.Alive() {
		return m.srv
	}
	return nil
}

// State returns the latest state.
func (m *Manager) State() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

// StatusChanges delivers state transitions; slow consumers keep the latest.
func (m *Manager) StatusChanges() <-chan State { return m.status }

// Notifications returns the backend's event stream.
func (m *Manager) Notifications() <-chan Notification { return m.notify }

// Listen returns a second copy of the backend's event stream, for consumers
// that must not steal events from Notifications (run output channels, for
// example). Values are dropped rather than queued without bound when the
// consumer lags. cancel stops delivery and may be called more than once.
func (m *Manager) Listen(queue int) (<-chan Notification, func()) {
	if queue <= 0 {
		queue = 1024
	}
	ch := make(chan Notification, queue)
	m.subMu.Lock()
	if m.subs == nil {
		m.subs = make(map[chan Notification]struct{})
	}
	m.subs[ch] = struct{}{}
	m.subMu.Unlock()
	var once sync.Once
	cancel := func() {
		once.Do(func() {
			m.subMu.Lock()
			delete(m.subs, ch)
			m.subMu.Unlock()
		})
	}
	return ch, cancel
}

// deliver hands a notification to the main stream and every listener.
func (m *Manager) deliver(n Notification) {
	select {
	case m.notify <- n:
	case <-m.closedCh:
		return
	}
	m.subMu.Lock()
	for ch := range m.subs {
		select {
		case ch <- n:
		default:
		}
	}
	m.subMu.Unlock()
}

// Ensure returns a live server, launching one if the process is gone.
func (m *Manager) Ensure(ctx context.Context) (*Server, error) {
	if s := m.Server(); s != nil {
		return s, nil
	}
	m.startMu.Lock()
	defer m.startMu.Unlock()
	if s := m.Server(); s != nil {
		return s, nil
	}

	m.mu.Lock()
	closed := m.closed
	m.mu.Unlock()
	if closed {
		return nil, ErrClosed
	}

	var err error
	for attempt := 1; attempt <= 3; attempt++ {
		s, launchErr := Launch(ctx, m.cfg)
		if launchErr == nil {
			m.adopt(s)
			return s, nil
		}
		err = launchErr
		m.mu.Lock()
		m.lastErr = launchErr
		m.state = State{Err: launchErr, Restarts: m.restarts}
		m.mu.Unlock()
		m.publish()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(attempt) * 250 * time.Millisecond):
		}
	}
	return nil, err
}

func (m *Manager) adopt(s *Server) {
	m.mu.Lock()
	m.srv = s
	if m.started {
		m.restarts++
	}
	m.started = true
	m.lastErr = nil
	m.state = State{Running: true, Info: s.Info(), Restarts: m.restarts}
	restore := m.onRestart
	m.mu.Unlock()
	m.publish()
	m.forward(s)
	go m.watch(s)
	if restore != nil {
		if err := restore(context.Background()); err != nil {
			m.mu.Lock()
			m.lastErr = err
			m.mu.Unlock()
		}
	}
}

// forward relays a server's notifications to the manager's consumer.
func (m *Manager) forward(s *Server) {
	m.forwards.Add(1)
	go func() {
		defer m.forwards.Done()
		for {
			select {
			case <-s.conn.Done():
				return
			case <-m.closedCh:
				return
			case n := <-s.conn.Notifications():
				m.deliver(n)
			}
		}
	}()
}

// watch restarts the backend after an unexpected exit, with backoff, so an Fs
// watcher or tool crash does not leave the IDE without a project model.
func (m *Manager) watch(s *Server) {
	<-s.conn.Done()
	m.mu.Lock()
	wasCurrent := m.srv == s
	if wasCurrent {
		m.srv = nil
	}
	closed := m.closed
	err := s.conn.Err()
	m.state = State{Err: err, Restarts: m.restarts}
	m.mu.Unlock()
	if wasCurrent {
		m.publish()
	}
	if closed || !wasCurrent {
		return
	}
	for attempt := 1; attempt <= 5; attempt++ {
		select {
		case <-m.closedCh:
			return
		case <-time.After(time.Duration(attempt*attempt) * 200 * time.Millisecond):
		}
		if _, err := m.Ensure(context.Background()); err == nil {
			return
		}
	}
}

func (m *Manager) publish() {
	st := m.State()
	select {
	case m.status <- st:
	default:
		select {
		case <-m.status:
		default:
		}
		select {
		case m.status <- st:
		default:
		}
	}
}

// Invoke runs a backend command in a window, waiting for a restart if needed.
func (m *Manager) Invoke(ctx context.Context, window, cmd string, args any) ([]byte, error) {
	s, err := m.Ensure(ctx)
	if err != nil {
		return nil, err
	}
	raw, err := s.Invoke(ctx, window, cmd, args)
	if errors.Is(err, ErrClosed) {
		// The connection died mid-call; surface it so the caller can decide
		// whether replay is safe (documents resync; builds do not auto-replay).
		return nil, err
	}
	return raw, nil
}

// Close shuts the server down and stops restarts.
func (m *Manager) Close() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	s := m.srv
	m.srv = nil
	m.mu.Unlock()
	close(m.closedCh)
	if s != nil {
		return s.Close()
	}
	return nil
}
