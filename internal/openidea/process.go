package openidea

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// LaunchConfig describes one openidea-server process.
type LaunchConfig struct {
	// ServerPath is the openidea-server binary; it is run with --stdio.
	ServerPath string
	// Home becomes OPENIDEA_SERVER_HOME, the server's data root. GoIDEA keeps
	// its own, so the desktop app's data is never touched.
	Home string
	// Env appends to the inherited environment.
	Env []string
	// Log receives the server's stderr (logs), which is never protocol data.
	Log io.Writer
	// Protocol is the version required from the server; 0 means 2.
	Protocol uint32
	// StartupTimeout bounds initialize plus commands; 0 means 15s.
	StartupTimeout time.Duration
	// QueueSize bounds the notification queue.
	QueueSize int
	// Handler answers server-to-client requests such as plugin_secret.
	Handler RequestHandler
}

// Server is one running openidea-server with its JSON-RPC connection.
type Server struct {
	cfg  LaunchConfig
	cmd  *exec.Cmd
	conn *Conn

	stdin  io.WriteCloser
	stderr io.ReadCloser

	info     ServerInfo
	commands []string

	waitCh  chan error
	done    chan struct{}
	closeMu sync.Once
}

// Launch starts openidea-server, checks its protocol version, and fetches the
// commands it actually supports.
func Launch(ctx context.Context, cfg LaunchConfig) (*Server, error) {
	if cfg.ServerPath == "" {
		return nil, errors.New("openidea: no server path")
	}
	if cfg.Protocol == 0 {
		cfg.Protocol = 2
	}
	if cfg.StartupTimeout == 0 {
		cfg.StartupTimeout = 15 * time.Second
	}
	if cfg.Log == nil {
		cfg.Log = os.Stderr
	}
	if _, err := os.Stat(cfg.ServerPath); err != nil {
		return nil, fmt.Errorf("openidea: server binary: %w", err)
	}

	cmd := exec.Command(cfg.ServerPath, "--stdio")
	cmd.Env = os.Environ()
	if cfg.Home != "" {
		cmd.Env = append(cmd.Env, "OPENIDEA_SERVER_HOME="+cfg.Home)
	}
	if len(cfg.Env) > 0 {
		cmd.Env = append(cmd.Env, cfg.Env...)
	}
	setProcessGroup(cmd)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("openidea: start %s: %w", cfg.ServerPath, err)
	}

	s := &Server{
		cfg:    cfg,
		cmd:    cmd,
		stdin:  stdin,
		stderr: stderr,
		waitCh: make(chan error, 1),
		done:   make(chan struct{}),
	}
	go func() {
		_, _ = io.Copy(cfg.Log, stderr)
	}()
	go func() {
		s.waitCh <- cmd.Wait()
		close(s.done)
	}()

	s.conn = NewConn(stdout, stdin, Options{QueueSize: cfg.QueueSize, Handler: cfg.Handler})

	startCtx, cancel := context.WithTimeout(ctx, cfg.StartupTimeout)
	defer cancel()
	raw, err := s.conn.Call(startCtx, "initialize", map[string]any{"version": cfg.Protocol})
	if err != nil {
		s.kill()
		return nil, fmt.Errorf("openidea: initialize: %w", err)
	}
	var info ServerInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		s.kill()
		return nil, fmt.Errorf("openidea: initialize reply: %w", err)
	}
	if info.Protocol != cfg.Protocol {
		s.kill()
		return nil, fmt.Errorf("openidea: server speaks protocol %d, need %d", info.Protocol, cfg.Protocol)
	}
	cmdsRaw, err := s.conn.Call(startCtx, "commands", nil)
	if err != nil {
		s.kill()
		return nil, fmt.Errorf("openidea: commands: %w", err)
	}
	var commands []string
	if err := json.Unmarshal(cmdsRaw, &commands); err != nil {
		s.kill()
		return nil, fmt.Errorf("openidea: commands reply: %w", err)
	}
	s.info = info
	s.commands = commands
	return s, nil
}

// Conn returns the JSON-RPC connection.
func (s *Server) Conn() *Conn { return s.conn }

// Info returns the server's initialize reply.
func (s *Server) Info() ServerInfo { return s.info }

// PID is the server process id, for logs and crash-recovery tests.
func (s *Server) PID() int { return s.cmd.Process.Pid }

// Commands returns the command names this server supports. Features must be
// gated on this list: a command is not guaranteed to exist in every version.
func (s *Server) Commands() []string { return append([]string(nil), s.commands...) }

// HasCommand reports whether cmd is available.
func (s *Server) HasCommand(cmd string) bool {
	for _, c := range s.commands {
		if c == cmd {
			return true
		}
	}
	return false
}

// Done is closed when the process exits.
func (s *Server) Done() <-chan struct{} { return s.done }

// Alive reports whether the process and connection are still up.
func (s *Server) Alive() bool {
	if s == nil {
		return false
	}
	select {
	case <-s.done:
		return false
	default:
	}
	select {
	case <-s.conn.Done():
		return false
	default:
	}
	return true
}

// Invoke runs a backend command in a window and returns its result.
func (s *Server) Invoke(ctx context.Context, window, cmd string, args any) ([]byte, error) {
	return s.conn.Call(ctx, "invoke", InvokeArgs{Window: window, Cmd: cmd, Args: args})
}

// InvokeInto runs a backend command and decodes its result into out.
func (s *Server) InvokeInto(ctx context.Context, window, cmd string, args, out any) error {
	raw, err := s.Invoke(ctx, window, cmd, args)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

// Close stops the server: stdin closes first for a graceful exit, then the
// whole process group is terminated if it lingers.
func (s *Server) Close() error {
	var err error
	s.closeMu.Do(func() {
		_ = s.stdin.Close()
		_ = s.conn.Close()
		select {
		case err = <-s.waitCh:
			return
		case <-time.After(3 * time.Second):
		}
		_ = terminateGroup(s.cmd)
		select {
		case err = <-s.waitCh:
			return
		case <-time.After(2 * time.Second):
		}
		_ = killGroup(s.cmd)
		err = <-s.waitCh
	})
	return err
}

// kill is Close without waiting: used when startup fails.
func (s *Server) kill() {
	_ = s.conn.Close()
	_ = s.stdin.Close()
	_ = killGroup(s.cmd)
	select {
	case <-s.waitCh:
	case <-time.After(2 * time.Second):
	}
}
