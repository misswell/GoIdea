package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"mayoi/internal/openidea"
)

const (
	remoteTunnelConcurrentConnections = 64
	remoteTunnelOutputGrace           = 5 * time.Second
)

type remotePortForwarder struct {
	backend *remoteBackend

	mu      sync.Mutex
	closed  bool
	tunnels []*localRemoteTunnel
}

type localRemoteTunnel struct {
	forward openidea.ForwardedPort
	listen  net.Listener
	cancel  context.CancelFunc
	done    chan struct{}
	relays  sync.WaitGroup

	mu        sync.Mutex
	conns     map[net.Conn]struct{}
	slots     chan struct{}
	closeOnce sync.Once
}

func newRemotePortForwarder(backend *remoteBackend) *remotePortForwarder {
	return &remotePortForwarder{backend: backend}
}

func (f *remotePortForwarder) Forward(remotePort uint16, host *string) (openidea.ForwardedPort, error) {
	if remotePort == 0 {
		return openidea.ForwardedPort{}, fmt.Errorf("Port must be between 1 and 65535")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return openidea.ForwardedPort{}, fmt.Errorf("the remote connection is closed")
	}
	for _, tunnel := range f.tunnels {
		if sameForward(tunnel.forward, remotePort, host) {
			return tunnel.forward, nil
		}
	}
	if f.backend.target.Kind == openidea.RemoteTargetWSL {
		return openidea.ForwardedPort{}, fmt.Errorf("Forwarding ports is not supported in a WSL project yet")
	}
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(int(remotePort))))
	if err != nil {
		listener, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		return openidea.ForwardedPort{}, err
	}
	localPort := listener.Addr().(*net.TCPAddr).Port
	ctx, cancel := context.WithCancel(context.Background())
	tunnel := &localRemoteTunnel{
		forward: openidea.ForwardedPort{
			RemotePort: remotePort,
			LocalPort:  uint16(localPort),
			Host:       cloneStringPtr(host),
		},
		listen: listener,
		cancel: cancel,
		done:   make(chan struct{}),
		conns:  map[net.Conn]struct{}{},
		slots:  make(chan struct{}, remoteTunnelConcurrentConnections),
	}
	f.tunnels = append(f.tunnels, tunnel)
	go tunnel.run(ctx, f.backend, remotePort, host)
	return tunnel.forward, nil
}

func (f *remotePortForwarder) Stop(remotePort uint16, host *string) {
	f.mu.Lock()
	index := -1
	var tunnel *localRemoteTunnel
	for i, candidate := range f.tunnels {
		if sameForward(candidate.forward, remotePort, host) {
			index = i
			tunnel = candidate
			break
		}
	}
	if index >= 0 {
		f.tunnels = append(f.tunnels[:index], f.tunnels[index+1:]...)
	}
	f.mu.Unlock()
	if tunnel != nil {
		tunnel.Close()
	}
}

func (f *remotePortForwarder) List() []openidea.ForwardedPort {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]openidea.ForwardedPort, 0, len(f.tunnels))
	for _, tunnel := range f.tunnels {
		port := tunnel.forward
		port.Host = cloneStringPtr(port.Host)
		out = append(out, port)
	}
	return out
}

func (f *remotePortForwarder) Close() {
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return
	}
	f.closed = true
	tunnels := append([]*localRemoteTunnel(nil), f.tunnels...)
	f.tunnels = nil
	f.mu.Unlock()
	for _, tunnel := range tunnels {
		tunnel.Close()
	}
}

func (t *localRemoteTunnel) run(ctx context.Context, backend *remoteBackend, remotePort uint16, host *string) {
	defer close(t.done)
	for {
		local, err := t.listen.Accept()
		if err != nil {
			return
		}
		select {
		case t.slots <- struct{}{}:
		default:
			_ = local.Close()
			continue
		}
		t.mu.Lock()
		t.conns[local] = struct{}{}
		t.mu.Unlock()
		t.relays.Add(1)
		go func() {
			defer t.relays.Done()
			defer func() { <-t.slots }()
			t.relay(ctx, backend, local, remotePort, host)
		}()
	}
}

func (t *localRemoteTunnel) relay(
	ctx context.Context,
	backend *remoteBackend,
	local net.Conn,
	remotePort uint16,
	host *string,
) {
	defer func() {
		_ = local.Close()
		t.mu.Lock()
		delete(t.conns, local)
		t.mu.Unlock()
	}()
	cmd, err := backend.forwardCommand(ctx, remotePort, host)
	if err != nil {
		return
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return
	}
	inputDone := make(chan struct{})
	outputDone := make(chan struct{})
	activity := make(chan struct{}, 1)
	go func() {
		defer close(inputDone)
		_, _ = io.Copy(stdin, local)
		_ = stdin.Close()
	}()
	go func() {
		defer close(outputDone)
		buffer := make([]byte, 16<<10)
		for {
			count, readErr := stdout.Read(buffer)
			if count > 0 {
				select {
				case activity <- struct{}{}:
				default:
				}
				if _, err := local.Write(buffer[:count]); err != nil {
					break
				}
			}
			if readErr != nil {
				break
			}
		}
		_ = local.Close()
	}()

	outputClosed := false
	grace := time.NewTimer(time.Hour)
	if !grace.Stop() {
		<-grace.C
	}
	select {
	case <-ctx.Done():
	case <-outputDone:
		outputClosed = true
	case <-inputDone:
		grace.Reset(remoteTunnelOutputGrace)
	waitOutput:
		for {
			select {
			case <-ctx.Done():
				break waitOutput
			case <-outputDone:
				outputClosed = true
				break waitOutput
			case <-activity:
				if !grace.Stop() {
					select {
					case <-grace.C:
					default:
					}
				}
				grace.Reset(remoteTunnelOutputGrace)
			case <-grace.C:
				break waitOutput
			}
		}
	}
	if !grace.Stop() {
		select {
		case <-grace.C:
		default:
		}
	}
	if !outputClosed {
		_ = local.Close()
	}
	_ = stdin.Close()
	_ = terminateRemoteCommand(cmd)
	_ = cmd.Wait()
	<-inputDone
	<-outputDone
}

func (t *localRemoteTunnel) Close() {
	t.closeOnce.Do(func() {
		t.cancel()
		_ = t.listen.Close()
		<-t.done
		t.mu.Lock()
		conns := make([]net.Conn, 0, len(t.conns))
		for conn := range t.conns {
			conns = append(conns, conn)
		}
		t.mu.Unlock()
		for _, conn := range conns {
			_ = conn.Close()
		}
		t.relays.Wait()
	})
}

func (b *remoteBackend) forwardCommand(
	ctx context.Context,
	remotePort uint16,
	host *string,
) (*exec.Cmd, error) {
	remoteHost := "127.0.0.1"
	if host != nil && *host != "" {
		remoteHost = *host
	}
	switch b.target.Kind {
	case openidea.RemoteTargetSSH:
		argv := b.sshArgs(ctx, false)
		argv = append(argv, "-W", net.JoinHostPort(remoteHost, strconv.Itoa(int(remotePort))))
		return b.command(ctx, "ssh", argv...), nil
	case openidea.RemoteTargetDevContainer:
		if b.container == nil {
			return nil, fmt.Errorf("dev container is not running")
		}
		return b.command(ctx, b.container.Docker,
			"exec", "-i", "-u", b.container.User, b.container.Container,
			b.remoteBin, "--connect", remoteHost, strconv.Itoa(int(remotePort))), nil
	default:
		return nil, fmt.Errorf("Forwarding ports is not supported in a WSL project yet")
	}
}

func sameForward(forward openidea.ForwardedPort, remotePort uint16, host *string) bool {
	if forward.RemotePort != remotePort {
		return false
	}
	if forward.Host == nil || host == nil {
		return forward.Host == nil && host == nil
	}
	return *forward.Host == *host
}

func cloneStringPtr(v *string) *string {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

func loadRemotePortStore(path string) map[string][]uint16 {
	raw, err := os.ReadFile(path)
	if err != nil {
		return map[string][]uint16{}
	}
	var stored map[string][]uint16
	if json.Unmarshal(raw, &stored) != nil {
		return map[string][]uint16{}
	}
	out := map[string][]uint16{}
	for key, ports := range stored {
		for _, port := range ports {
			if port > 0 {
				out[key] = append(out[key], port)
			}
		}
	}
	return out
}

func saveRemotePortStore(path string, ports map[string][]uint16) error {
	raw, err := json.MarshalIndent(ports, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, raw)
}
