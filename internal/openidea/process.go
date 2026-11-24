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
	// Launcher starts a transport other than a local process, such as an SSH
	// exec channel, WSL or docker exec. When nil, ServerPath is launched.
	Launcher TransportLauncher
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
	// MinVersion is the oldest server this client accepts, e.g. "0.1.44".
	MinVersion string
	// ExpectedSHA256 is the release checksum of the binary, when one is
	// shipped beside it; empty skips the comparison.
	ExpectedSHA256 string
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
	tr   Transport
	conn *Conn

	stdin  io.WriteCloser
	stderr io.Reader

	info     ServerInfo
	report   Report
	commands []string

	waitCh  chan error
	done    chan struct{}
	closeMu sync.Once
}

// Launch starts openidea-server, checks its protocol version, and fetches the
// commands it actually supports.
func Launch(ctx context.Context, cfg LaunchConfig) (*Server, error) {
	if cfg.Protocol == 0 {
		cfg.Protocol = 2
	}
	if cfg.StartupTimeout == 0 {
		cfg.StartupTimeout = 15 * time.Second
	}
	if cfg.Log == nil {
		cfg.Log = os.Stderr
	}
	if cfg.Launcher != nil {
		tr, err := cfg.Launcher(ctx, cfg)
		if err != nil {
			return nil, err
		}
		if tr == nil || tr.Stdin() == nil || tr.Stdout() == nil {
			if tr != nil {
				_ = tr.Close()
			}
			return nil, errors.New("goidea: remote transport has no stdin/stdout")
		}
		s := &Server{
			cfg:    cfg,
			tr:     tr,
			stdin:  tr.Stdin(),
			stderr: tr.Stderr(),
			waitCh: make(chan error, 1),
			done:   make(chan struct{}),
		}
		if cfg.Log != nil && s.stderr != nil {
			go func() { _, _ = io.Copy(cfg.Log, s.stderr) }()
		}
		go func() {
			s.waitCh <- tr.Wait()
			close(s.done)
		}()
		s.conn = NewConn(tr.Stdout(), s.stdin, Options{QueueSize: cfg.QueueSize, Handler: cfg.Handler})
		if err := s.initialize(ctx); err != nil {
			s.kill()
			return nil, err
		}
		return s, nil
	}
	if cfg.ServerPath == "" {
		return nil, errors.New("goidea: no server path")
	}
	report, err := Preflight(cfg)
	if err != nil {
		return nil, err
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
		return nil, fmt.Errorf("goidea: start %s: %w", cfg.ServerPath, err)
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
	if err := s.initialize(ctx); err != nil {
		s.kill()
		return nil, err
	}
	s.report = report
	return s, nil
}

func (s *Server) initialize(ctx context.Context) error {
	cfg := s.cfg
	startCtx, cancel := context.WithTimeout(ctx, cfg.StartupTimeout)
	defer cancel()
	raw, err := s.conn.Call(startCtx, "initialize", map[string]any{"version": cfg.Protocol})
	if err != nil {
		return fmt.Errorf("goidea: initialize: %w", err)
	}
	var info ServerInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		return fmt.Errorf("goidea: initialize reply: %w", err)
	}
	if info.Protocol != cfg.Protocol {
		return fmt.Errorf("goidea: server speaks protocol %d, need %d", info.Protocol, cfg.Protocol)
	}
	cmdsRaw, err := s.conn.Call(startCtx, "commands", nil)
	if err != nil {
		return fmt.Errorf("goidea: commands: %w", err)
	}
	var commands []string
	if err := json.Unmarshal(cmdsRaw, &commands); err != nil {
		return fmt.Errorf("goidea: commands reply: %w", err)
	}
	s.info = info
	s.commands = commands
	return nil
}

// Conn returns the JSON-RPC connection.
func (s *Server) Conn() *Conn { return s.conn }

// Info returns the server's initialize reply.
func (s *Server) Info() ServerInfo { return s.info }

// Report is what the preflight measured: checksum, version, protocol, size.
func (s *Server) Report() Report { return s.report }

// PID is the server process id, for logs and crash-recovery tests.
func (s *Server) PID() int {
	if s.cmd != nil && s.cmd.Process != nil {
		return s.cmd.Process.Pid
	}
	if s.tr != nil {
		return s.tr.PID()
	}
	return 0
}

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
		if s.cmd != nil {
			_ = terminateGroup(s.cmd)
		} else if s.tr != nil {
			_ = s.tr.Close()
		}
		select {
		case err = <-s.waitCh:
			return
		case <-time.After(2 * time.Second):
		}
		if s.cmd != nil {
			_ = killGroup(s.cmd)
		} else if s.tr != nil {
			_ = s.tr.Close()
		}
		err = <-s.waitCh
	})
	return err
}

// kill is Close without waiting: used when startup fails.
func (s *Server) kill() {
	if s.conn != nil {
		_ = s.conn.Close()
	}
	if s.stdin != nil {
		_ = s.stdin.Close()
	}
	if s.cmd != nil {
		_ = killGroup(s.cmd)
	} else if s.tr != nil {
		_ = s.tr.Close()
	}
	select {
	case <-s.waitCh:
	case <-time.After(2 * time.Second):
	}
}
