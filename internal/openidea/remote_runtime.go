package openidea

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// UseTransport replaces the current backend with a transport such as SSH,
// WSL or docker exec. Existing RPC callers keep using this Manager.
func (m *Manager) UseTransport(ctx context.Context, launcher TransportLauncher) error {
	m.startMu.Lock()
	defer m.startMu.Unlock()
	return m.switchLocked(ctx, launcher)
}

// UseLocal returns from a remote backend to the local server configured at
// construction. It deliberately skips the restart callback: the app controls
// the project transition explicitly.
func (m *Manager) UseLocal(ctx context.Context) error {
	m.startMu.Lock()
	defer m.startMu.Unlock()
	return m.switchLocked(ctx, m.localCfg.Launcher)
}

func (m *Manager) switchLocked(ctx context.Context, launcher TransportLauncher) error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return ErrClosed
	}
	old := m.srv
	m.srv = nil
	m.cfg.Launcher = launcher
	m.mu.Unlock()

	if old != nil {
		_ = old.Close()
	}

	m.setPhase("starting")
	var (
		s   *Server
		err error
	)
	for attempt := 1; attempt <= 3; attempt++ {
		m.mu.Lock()
		cfg := m.cfg
		m.mu.Unlock()
		s, err = Launch(ctx, cfg)
		if err == nil {
			if !m.adopt(s, false) {
				return ErrClosed
			}
			return nil
		}
		var preflight *PreflightError
		if errors.As(err, &preflight) {
			break
		}
		if attempt < 3 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt) * 250 * time.Millisecond):
			}
		}
	}
	m.mu.Lock()
	m.state = State{Err: err, Restarts: m.restarts, Phase: "failed"}
	m.mu.Unlock()
	m.publish()
	if err == nil {
		err = fmt.Errorf("goidea: backend launcher returned no server")
	}
	return err
}
