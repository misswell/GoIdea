package app

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf16"

	"mayoi/internal/keymap"
	"mayoi/internal/openidea"
)

const remoteProbeMarker = "__GOIDEA_REMOTE_PROBE__"
const remoteServerDirectory = ".openidea/server"
const remoteServerReleaseRepository = "misswell/openidea"

type remoteProcessRunner struct {
	lookPath func(string) (string, error)
	command  func(context.Context, string, ...string) *exec.Cmd
	http     *http.Client
}

var defaultRemoteRunner = remoteProcessRunner{
	lookPath: exec.LookPath,
	command:  remoteCommandContext,
	http:     &http.Client{Timeout: 4 * time.Minute},
}

type remoteTail struct {
	mu    sync.Mutex
	lines []string
}

func (t *remoteTail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, line := range strings.Split(strings.TrimRight(string(p), "\r\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		t.lines = append(t.lines, line)
		if len(t.lines) > 20 {
			t.lines = append(t.lines[:0], t.lines[len(t.lines)-20:]...)
		}
	}
	return len(p), nil
}

func (t *remoteTail) Text() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.Join(t.lines, "\n")
}

type commandTransport struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser
	done   chan error
	once   sync.Once
}

func startCommandTransport(ctx context.Context, cmd *exec.Cmd) (*commandTransport, error) {
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		return nil, err
	}
	t := &commandTransport{cmd: cmd, stdin: stdin, stdout: stdout, stderr: stderr, done: make(chan error, 1)}
	go func() { t.done <- cmd.Wait() }()
	if done := ctx.Done(); done != nil {
		go func() {
			<-done
			_ = t.Close()
		}()
	}
	return t, nil
}

func (t *commandTransport) Stdin() io.WriteCloser { return t.stdin }
func (t *commandTransport) Stdout() io.Reader     { return t.stdout }
func (t *commandTransport) Stderr() io.Reader     { return t.stderr }
func (t *commandTransport) Wait() error           { return <-t.done }
func (t *commandTransport) PID() int {
	if t.cmd.Process == nil {
		return 0
	}
	return t.cmd.Process.Pid
}
func (t *commandTransport) Close() error {
	var closeErr error
	t.once.Do(func() {
		_ = t.stdin.Close()
		_ = terminateRemoteCommand(t.cmd)
		_ = t.stdout.Close()
		_ = t.stderr.Close()
		closeErr = nil
	})
	return closeErr
}

type remoteBackend struct {
	target openidea.RemoteTarget
	ssh    openidea.SSHConfiguration
	runner remoteProcessRunner

	dataDir            string
	localServer        string
	serverVersion      string
	log                io.Writer
	secret             string
	remoteBin          string
	home               string
	container          *remoteContainerInfo
	devContainerConfig string
	devContainerCLI    string
	tail               remoteTail

	mu           sync.Mutex
	forwarder    *remotePortForwarder
	webForwarder *remotePortForwarder
	configured   map[string]bool
}

type remoteContainerInfo struct {
	Container string `json:"containerId"`
	User      string `json:"remoteUser"`
	Folder    string `json:"remoteWorkspaceFolder"`
	Docker    string `json:"-"`
}

type remoteProbe struct {
	Home      string
	Uname     string
	Installed []string
}

func newRemoteBackend(
	target openidea.RemoteTarget,
	ssh openidea.SSHConfiguration,
	serverVersion, dataDir, localServer string,
	log io.Writer,
) *remoteBackend {
	return &remoteBackend{
		target:        target,
		ssh:           ssh,
		runner:        defaultRemoteRunner,
		dataDir:       dataDir,
		localServer:   localServer,
		serverVersion: serverVersion,
		log:           log,
	}
}

func (b *remoteBackend) prepare(ctx context.Context, progress func(openidea.RemoteProgress)) error {
	if err := b.target.Validate(); err != nil {
		return err
	}
	switch b.target.Kind {
	case openidea.RemoteTargetSSH, openidea.RemoteTargetWSL:
		return b.prepareShell(ctx, progress)
	case openidea.RemoteTargetDevContainer:
		return b.prepareContainer(ctx, progress)
	default:
		return fmt.Errorf("unsupported remote target %q", b.target.Kind)
	}
}

func reportProgress(progress func(openidea.RemoteProgress), stage openidea.RemoteStage, fraction *float64) {
	if progress != nil {
		progress(openidea.RemoteProgress{Stage: stage, Fraction: fraction})
	}
}

func (b *remoteBackend) prepareShell(ctx context.Context, progress func(openidea.RemoteProgress)) error {
	reportProgress(progress, openidea.RemoteStageChecking, nil)
	probe, err := b.probe(ctx)
	if err != nil {
		return err
	}
	osName, arch := remotePlatform(probe.Uname)
	if osName != "linux" && osName != "macos" {
		return fmt.Errorf("the remote host reports unsupported platform %s", probe.Uname)
	}
	if !supportedRemotePlatform(osName, arch) {
		return fmt.Errorf("remote development does not support %s-%s hosts", osName, arch)
	}
	bin, err := b.ensureServer(ctx, osName, arch, probe, progress)
	if err != nil {
		return err
	}
	b.home = probe.Home
	b.remoteBin = bin
	return nil
}

func (b *remoteBackend) probe(ctx context.Context) (remoteProbe, error) {
	script := `printf '` + remoteProbeMarker + `\n'; echo "$HOME"; uname -sm; ls -d "$HOME/` +
		remoteServerDirectory + `/` + b.serverVersion + `"/*/openidea-server 2>/dev/null || true`
	out, err := b.runRemote(ctx, nil, 60*time.Second, script, nil)
	if err != nil {
		return remoteProbe{}, err
	}
	return parseRemoteProbe(string(out))
}

func parseRemoteProbe(text string) (remoteProbe, error) {
	_, rest, ok := strings.Cut(text, remoteProbeMarker)
	if !ok {
		return remoteProbe{}, fmt.Errorf("the remote host did not answer the platform probe")
	}
	lines := strings.Split(strings.TrimSpace(rest), "\n")
	if len(lines) < 2 {
		return remoteProbe{}, fmt.Errorf("the remote host did not report its home directory and platform")
	}
	home := strings.TrimSpace(lines[0])
	if !strings.HasPrefix(home, "/") || strings.ContainsAny(home, "\x00\r\n") {
		return remoteProbe{}, fmt.Errorf("the remote host reported an invalid home directory")
	}
	p := remoteProbe{Home: home, Uname: strings.TrimSpace(lines[1])}
	for _, line := range lines[2:] {
		line = strings.TrimSpace(line)
		if line != "" {
			p.Installed = append(p.Installed, line)
		}
	}
	return p, nil
}

func remotePlatform(uname string) (string, string) {
	parts := strings.Fields(strings.ToLower(uname))
	osName := ""
	if len(parts) > 0 {
		osName = parts[0]
	}
	if osName == "darwin" {
		osName = "macos"
	}
	arch := ""
	if len(parts) > 1 {
		arch = parts[1]
		switch arch {
		case "arm64":
			arch = "aarch64"
		case "amd64":
			arch = "x86_64"
		}
	}
	return osName, arch
}

func supportedRemotePlatform(osName, arch string) bool {
	switch osName + "-" + arch {
	case "linux-x86_64", "linux-aarch64", "macos-x86_64", "macos-aarch64":
		return true
	default:
		return false
	}
}

func (b *remoteBackend) ensureServer(
	ctx context.Context,
	osName, arch string,
	probe remoteProbe,
	progress func(openidea.RemoteProgress),
) (string, error) {
	local, err := b.serverBinary(ctx, osName, arch, progress)
	if err != nil {
		if len(probe.Installed) > 0 {
			return probe.Installed[0], nil
		}
		return "", err
	}
	data, err := os.ReadFile(local)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	build := hex.EncodeToString(sum[:])[:16]
	for _, installed := range probe.Installed {
		if strings.HasSuffix(installed, "/"+build+"/openidea-server") {
			return installed, nil
		}
	}
	versionDir := probe.Home + "/" + remoteServerDirectory + "/" + b.serverVersion
	buildDir := versionDir + "/" + build
	remoteBin := buildDir + "/openidea-server"
	reportProgress(progress, openidea.RemoteStageUploading, floatPtr(0))
	reader := &progressReader{
		reader: bytes.NewReader(data),
		total:  int64(len(data)),
		report: func(done int64) {
			reportProgress(progress, openidea.RemoteStageUploading, floatPtr(float64(done)/float64(len(data))))
		},
	}
	script := `set -e
mkdir -p "$1"
p="$1/openidea-server.part.$$"
trap 'rm -f "$p"' EXIT
cat > "$p"
chmod 755 "$p"
mv -f "$p" "$1/openidea-server"
find "$2" -mindepth 1 -maxdepth 1 ! -name "$3" -exec rm -rf {} +
`
	if _, err := b.runRemoteReader(ctx, reader, 10*time.Minute, script, []string{buildDir, versionDir, build}); err != nil {
		return "", err
	}
	return remoteBin, nil
}

func (b *remoteBackend) serverBinary(
	ctx context.Context,
	osName, arch string,
	progress func(openidea.RemoteProgress),
) (string, error) {
	if p := os.Getenv("OPENIDEA_SERVER_BINARY"); p != "" {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	hostOS, hostArch := runtime.GOOS, runtime.GOARCH
	if hostArch == "arm64" {
		hostArch = "aarch64"
	}
	if hostArch == "amd64" {
		hostArch = "x86_64"
	}
	if osName == hostOS && arch == hostArch && b.localServer != "" {
		if st, err := os.Stat(b.localServer); err == nil && !st.IsDir() {
			return b.localServer, nil
		}
	}
	cacheDir := filepath.Join(b.dataDir, "server", b.serverVersion, osName+"-"+arch)
	cache := filepath.Join(cacheDir, "openidea-server")
	if st, err := os.Stat(cache); err == nil && !st.IsDir() {
		return cache, nil
	}
	reportProgress(progress, openidea.RemoteStageDownloading, floatPtr(0))
	if err := b.downloadServer(ctx, cache, osName, arch, progress); err != nil {
		return "", err
	}
	return cache, nil
}

func (b *remoteBackend) downloadServer(
	ctx context.Context,
	cache, osName, arch string,
	progress func(openidea.RemoteProgress),
) error {
	base := os.Getenv("GOIDEA_REMOTE_SERVER_URL")
	if base == "" {
		base = remoteServerReleaseBaseURL(b.serverVersion)
	}
	asset := "openidea-server-" + osName + "-" + arch + ".gz"
	url := strings.TrimRight(base, "/") + "/" + asset
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := b.runner.http.Do(req)
	if err != nil {
		return fmt.Errorf("download GoIDEA backend for %s-%s: %w", osName, arch, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download GoIDEA backend for %s-%s: HTTP %s", osName, arch, resp.Status)
	}
	if err := os.MkdirAll(filepath.Dir(cache), 0o755); err != nil {
		return err
	}
	archive, err := os.CreateTemp(filepath.Dir(cache), ".openidea-server-*.gz")
	if err != nil {
		return err
	}
	archiveName := archive.Name()
	defer os.Remove(archiveName)
	defer archive.Close()
	hasher := sha256.New()
	reader := &progressReader{
		reader: resp.Body,
		total:  resp.ContentLength,
		report: func(done int64) {
			fraction := float64(0)
			if resp.ContentLength > 0 {
				fraction = float64(done) / float64(resp.ContentLength)
			}
			reportProgress(progress, openidea.RemoteStageDownloading, floatPtr(min(1, fraction)))
		},
	}
	if _, err := io.Copy(io.MultiWriter(archive, hasher), reader); err != nil {
		return fmt.Errorf("download GoIDEA backend: %w", err)
	}
	if err := archive.Close(); err != nil {
		return err
	}
	expected, err := b.remoteServerChecksum(ctx, url+".sha256", asset)
	if err != nil {
		return err
	}
	actual := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(expected, actual) {
		return fmt.Errorf("GoIDEA backend checksum mismatch: expected %s, got %s", expected, actual)
	}
	archiveReader, err := os.Open(archiveName)
	if err != nil {
		return err
	}
	defer archiveReader.Close()
	gz, err := gzip.NewReader(archiveReader)
	if err != nil {
		return fmt.Errorf("download GoIDEA backend: %w", err)
	}
	defer gz.Close()
	binary, err := os.CreateTemp(filepath.Dir(cache), ".openidea-server-*")
	if err != nil {
		return err
	}
	binaryName := binary.Name()
	defer os.Remove(binaryName)
	if _, err := io.Copy(binary, gz); err != nil {
		_ = binary.Close()
		return fmt.Errorf("download GoIDEA backend: %w", err)
	}
	if err := binary.Chmod(0o755); err != nil {
		_ = binary.Close()
		return err
	}
	if err := binary.Close(); err != nil {
		return err
	}
	if err := os.Rename(binaryName, cache); err != nil {
		return err
	}
	reportProgress(progress, openidea.RemoteStageDownloading, floatPtr(1))
	return nil
}

func remoteServerReleaseBaseURL(version string) string {
	return "https://github.com/" + remoteServerReleaseRepository + "/releases/download/v" + version
}

func (b *remoteBackend) remoteServerChecksum(ctx context.Context, checksumURL, asset string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, checksumURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := b.runner.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("download the GoIDEA backend checksum: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download the GoIDEA backend checksum: HTTP %s", resp.Status)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("download the GoIDEA backend checksum: %w", err)
	}
	expected := parseRemoteChecksum(string(raw), asset)
	if expected == "" {
		return "", fmt.Errorf("no SHA-256 for %s at %s", asset, checksumURL)
	}
	return expected, nil
}

func parseRemoteChecksum(text, asset string) string {
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 1 && len(fields[0]) == sha256.Size*2 {
			if _, err := hex.DecodeString(fields[0]); err == nil {
				return strings.ToLower(fields[0])
			}
		}
		if len(fields) >= 2 && len(fields[0]) == sha256.Size*2 {
			name := strings.TrimPrefix(fields[1], "*")
			if filepath.Base(name) != asset {
				continue
			}
			if _, err := hex.DecodeString(fields[0]); err == nil {
				return strings.ToLower(fields[0])
			}
		}
	}
	return ""
}

func (b *remoteBackend) prepareContainer(ctx context.Context, progress func(openidea.RemoteProgress)) error {
	folder := filepath.Clean(b.target.Folder)
	info, err := os.Stat(folder)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("dev-container folder %s is not a directory", folder)
	}
	config := strings.TrimSpace(b.target.Config)
	configPath, err := findDevContainerConfig(folder, config)
	if err != nil {
		return err
	}
	docker, err := b.runner.lookPath("docker")
	if err != nil {
		return fmt.Errorf("Docker is not installed")
	}
	localDocker, err := b.localDockerEndpoint(ctx, docker)
	if err != nil {
		return err
	}
	if !localDocker {
		return fmt.Errorf("The Docker daemon of the current context is not on this machine; dev containers need a local one")
	}
	cli := strings.TrimSpace(b.devContainerCLI)
	if cli == "" {
		cli = os.Getenv("DEV_CONTAINER_CLI")
	}
	if cli == "" {
		cli, err = b.runner.lookPath("devcontainer")
		if err != nil {
			return fmt.Errorf("The Dev Containers CLI is not installed. Install it in Settings › Languages & Frameworks › Language Servers.")
		}
	}
	reportProgress(progress, openidea.RemoteStageContainer, nil)
	args := []string{"up", "--workspace-folder", folder}
	if config != "" {
		args = append(args, "--config", configPath)
	}
	args = append(args, "--docker-path", docker)
	upCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	cmd := b.runner.command(upCtx, cli, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = io.MultiWriter(&stderr, remoteLineWriter(b.log, "devcontainer"))
	err = cmd.Run()
	container, parseErr := parseDevContainerUp(stdout.Bytes())
	if err != nil {
		text := strings.TrimSpace(stderr.String())
		if text == "" && parseErr != nil {
			text = parseErr.Error()
		}
		if text != "" {
			return fmt.Errorf("devcontainer up failed: %s", tailText(text, 600))
		}
		return fmt.Errorf("devcontainer up failed: %w", err)
	}
	if parseErr != nil {
		return parseErr
	}
	container.Docker = docker
	b.container = &container
	b.devContainerConfig = configPath
	probe, err := b.probeContainer(ctx)
	if err != nil {
		return err
	}
	osName, arch := remotePlatform(probe.Uname)
	if osName != "linux" {
		return fmt.Errorf("the dev container reports %s instead of linux", osName)
	}
	bin, err := b.ensureContainerServer(ctx, osName, arch, probe, progress)
	if err != nil {
		return err
	}
	b.home = probe.Home
	b.remoteBin = bin
	return nil
}

func (b *remoteBackend) localDockerEndpoint(ctx context.Context, docker string) (bool, error) {
	if host := strings.TrimSpace(os.Getenv("DOCKER_HOST")); host != "" &&
		strings.TrimSpace(os.Getenv("DOCKER_CONTEXT")) == "" {
		return dockerDaemonURLLocal(host), nil
	}
	args := []string{"context", "inspect", "--format", "{{.Endpoints.docker.Host}}"}
	if contextName := strings.TrimSpace(os.Getenv("DOCKER_CONTEXT")); contextName != "" {
		args = append(args, contextName)
	}
	raw, err := b.runner.command(ctx, docker, args...).Output()
	if err != nil {
		return false, fmt.Errorf("Could not inspect the Docker context: %w", err)
	}
	return dockerDaemonURLLocal(string(raw)), nil
}

func dockerDaemonURLLocal(value string) bool {
	value = strings.TrimSpace(value)
	scheme, rest, split := strings.Cut(value, "://")
	if !split {
		return true
	}
	switch scheme {
	case "unix", "npipe":
		return true
	case "tcp", "http", "https":
		host := strings.SplitN(rest, "/", 2)[0]
		if strings.HasPrefix(host, "[") {
			host = strings.SplitN(strings.TrimPrefix(host, "["), "]", 2)[0]
		} else if index := strings.LastIndex(host, ":"); index >= 0 {
			host = host[:index]
		}
		return host == "localhost" || host == "127.0.0.1" || host == "::1"
	default:
		return false
	}
}

func findDevContainerConfig(folder, config string) (string, error) {
	folder = filepath.Clean(folder)
	if config = strings.TrimSpace(config); config != "" {
		configPath := filepath.Clean(filepath.Join(folder, config))
		st, err := os.Stat(configPath)
		if err != nil || st.IsDir() {
			return "", fmt.Errorf("dev-container configuration %s was not found", config)
		}
		return configPath, nil
	}
	for _, relative := range []string{
		filepath.Join(".devcontainer", "devcontainer.json"),
		".devcontainer.json",
	} {
		configPath := filepath.Join(folder, relative)
		if st, err := os.Stat(configPath); err == nil && !st.IsDir() {
			return configPath, nil
		}
	}
	return "", fmt.Errorf("%s has no devcontainer.json", folder)
}

func hasDevContainerFile(folder string) bool {
	_, err := findDevContainerConfig(folder, "")
	return err == nil
}

type devContainerForwardPort struct {
	Host *string
	Port uint16
}

func loadDevContainerForwardPorts(configPath string) ([]devContainerForwardPort, error) {
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	var document struct {
		ForwardPorts json.RawMessage `json:"forwardPorts"`
	}
	if err := json.Unmarshal(keymap.StripJSONC(raw), &document); err != nil {
		return nil, err
	}
	if len(document.ForwardPorts) == 0 || bytes.Equal(bytes.TrimSpace(document.ForwardPorts), []byte("null")) {
		return nil, nil
	}
	var entries []any
	if err := json.Unmarshal(document.ForwardPorts, &entries); err != nil {
		return nil, fmt.Errorf("forwardPorts: %w", err)
	}
	seen := map[string]bool{}
	out := make([]devContainerForwardPort, 0, len(entries))
	for _, entry := range entries {
		host, port, ok := parseDevContainerForwardPort(entry)
		if !ok {
			continue
		}
		key := strconv.Itoa(int(port))
		if host != nil {
			key = *host + ":" + key
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, devContainerForwardPort{Host: host, Port: port})
	}
	return out, nil
}

func parseDevContainerForwardPort(entry any) (*string, uint16, bool) {
	var (
		host  *string
		value string
	)
	switch entry := entry.(type) {
	case json.Number:
		value = entry.String()
	case float64:
		if entry < 1 || entry > 65535 || math.Trunc(entry) != entry {
			return nil, 0, false
		}
		value = strconv.FormatUint(uint64(entry), 10)
	case string:
		value = strings.TrimSpace(entry)
		if name, number, split := strings.Cut(value, ":"); split {
			if strings.Contains(number, ":") || !validDevContainerHost(name) {
				return nil, 0, false
			}
			switch name {
			case "localhost", "127.0.0.1":
			default:
				host = &name
			}
			value = number
		}
	default:
		return nil, 0, false
	}
	port, err := strconv.ParseUint(strings.TrimSpace(value), 10, 16)
	if err != nil || port == 0 {
		return nil, 0, false
	}
	return host, uint16(port), true
}

func validDevContainerHost(host string) bool {
	if host == "" || len(host) > 253 || host[0] == '-' || host[0] == '.' {
		return false
	}
	for _, r := range host {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func parseDevContainerUp(raw []byte) (remoteContainerInfo, error) {
	var result remoteContainerInfo
	lines := bytes.Split(bytes.TrimSpace(raw), []byte{'\n'})
	for i := len(lines) - 1; i >= 0; i-- {
		var candidate remoteContainerInfo
		if json.Unmarshal(bytes.TrimSpace(lines[i]), &candidate) == nil &&
			candidate.Container != "" && candidate.User != "" && candidate.Folder != "" {
			return candidate, nil
		}
	}
	return result, fmt.Errorf("devcontainer up did not return a container id, user and workspace folder")
}

func (b *remoteBackend) probeContainer(ctx context.Context) (remoteProbe, error) {
	script := `printf '` + remoteProbeMarker + `\n'; echo "$HOME"; uname -sm; ls -d "$HOME/` +
		remoteServerDirectory + `/` + b.serverVersion + `"/*/openidea-server 2>/dev/null || true`
	out, err := b.runContainer(ctx, nil, 60*time.Second, script, nil)
	if err != nil {
		return remoteProbe{}, err
	}
	return parseRemoteProbe(string(out))
}

func (b *remoteBackend) ensureContainerServer(
	ctx context.Context,
	osName, arch string,
	probe remoteProbe,
	progress func(openidea.RemoteProgress),
) (string, error) {
	local, err := b.serverBinary(ctx, osName, arch, progress)
	if err != nil {
		if len(probe.Installed) > 0 {
			return probe.Installed[0], nil
		}
		return "", err
	}
	data, err := os.ReadFile(local)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	build := hex.EncodeToString(sum[:])[:16]
	for _, installed := range probe.Installed {
		if strings.HasSuffix(installed, "/"+build+"/openidea-server") {
			return installed, nil
		}
	}
	versionDir := probe.Home + "/" + remoteServerDirectory + "/" + b.serverVersion
	buildDir := versionDir + "/" + build
	script := `set -e
mkdir -p "$1"
p="$1/openidea-server.part.$$"
trap 'rm -f "$p"' EXIT
cat > "$p"
chmod 755 "$p"
mv -f "$p" "$1/openidea-server"
find "$2" -mindepth 1 -maxdepth 1 ! -name "$3" -exec rm -rf {} +
`
	reportProgress(progress, openidea.RemoteStageUploading, floatPtr(0))
	reader := &progressReader{
		reader: bytes.NewReader(data),
		total:  int64(len(data)),
		report: func(done int64) {
			reportProgress(progress, openidea.RemoteStageUploading, floatPtr(float64(done)/float64(len(data))))
		},
	}
	if _, err := b.runContainerReader(ctx, reader, 10*time.Minute, script, []string{buildDir, versionDir, build}); err != nil {
		return "", err
	}
	return buildDir + "/openidea-server", nil
}

func (b *remoteBackend) launcher() openidea.TransportLauncher {
	return func(ctx context.Context, _ openidea.LaunchConfig) (openidea.Transport, error) {
		if b.remoteBin == "" {
			return nil, fmt.Errorf("remote GoIDEA backend is not installed")
		}
		reportProgress(nil, openidea.RemoteStageStarting, nil)
		// The setup timeout ends when the server is ready; the transport stays
		// alive until Manager.Close or a backend switch closes it.
		lifetime := context.WithoutCancel(ctx)
		cmd, err := b.serverCommand(lifetime)
		if err != nil {
			return nil, err
		}
		return startCommandTransport(lifetime, cmd)
	}
}

func (b *remoteBackend) serverCommand(ctx context.Context) (*exec.Cmd, error) {
	switch b.target.Kind {
	case openidea.RemoteTargetSSH:
		args := b.sshArgs(ctx, false)
		args = append(args, remoteShellInvocation(b.remoteBin+" --stdio", nil)...)
		return b.command(ctx, "ssh", args...), nil
	case openidea.RemoteTargetWSL:
		if err := (openidea.RemoteTarget{Kind: openidea.RemoteTargetWSL, Distro: b.target.Distro}).Validate(); err != nil {
			return nil, err
		}
		return b.command(ctx, "wsl.exe", "-d", b.target.Distro, "--exec", b.remoteBin, "--stdio"), nil
	case openidea.RemoteTargetDevContainer:
		if b.container == nil {
			return nil, fmt.Errorf("dev container is not running")
		}
		return b.command(ctx, b.container.Docker,
			"exec", "-i", "-u", b.container.User, "-w", b.container.Folder,
			"-e", "OPENIDEA_TRUSTED_ROOT="+b.container.Folder,
			"-e", "OPENIDEA_LIFECYCLE_DONE=1",
			b.container.Container, b.remoteBin, "--stdio"), nil
	default:
		return nil, fmt.Errorf("unsupported remote target %q", b.target.Kind)
	}
}

func (b *remoteBackend) runRemote(
	ctx context.Context,
	input io.Reader,
	timeout time.Duration,
	script string,
	args []string,
) ([]byte, error) {
	switch b.target.Kind {
	case openidea.RemoteTargetSSH, openidea.RemoteTargetWSL:
		return b.runRemoteReader(ctx, input, timeout, script, args)
	case openidea.RemoteTargetDevContainer:
		return b.runContainerReader(ctx, input, timeout, script, args)
	default:
		return nil, fmt.Errorf("unsupported remote target %q", b.target.Kind)
	}
}

func (b *remoteBackend) runRemoteReader(
	ctx context.Context,
	input io.Reader,
	timeout time.Duration,
	script string,
	args []string,
) ([]byte, error) {
	if b.target.Kind == openidea.RemoteTargetDevContainer {
		return b.runContainerReader(ctx, input, timeout, script, args)
	}
	cmd, err := b.shellCommand(ctx, true, script, args)
	if err != nil {
		return nil, err
	}
	return b.runCaptured(ctx, cmd, input, timeout)
}

func (b *remoteBackend) runContainer(
	ctx context.Context,
	input io.Reader,
	timeout time.Duration,
	script string,
	args []string,
) ([]byte, error) {
	return b.runContainerReader(ctx, input, timeout, script, args)
}

func (b *remoteBackend) runContainerReader(
	ctx context.Context,
	input io.Reader,
	timeout time.Duration,
	script string,
	args []string,
) ([]byte, error) {
	if b.container == nil {
		return nil, fmt.Errorf("dev container is not running")
	}
	argv := []string{"exec", "-i", "-u", b.container.User, b.container.Container, "sh", "-c", script, "sh"}
	argv = append(argv, args...)
	cmd := b.command(ctx, b.container.Docker, argv...)
	return b.runCaptured(ctx, cmd, input, timeout)
}

func (b *remoteBackend) shellCommand(
	ctx context.Context,
	input bool,
	script string,
	args []string,
) (*exec.Cmd, error) {
	switch b.target.Kind {
	case openidea.RemoteTargetSSH:
		argv := b.sshArgs(ctx, input)
		argv = append(argv, remoteShellInvocation(script, args)...)
		return b.command(ctx, "ssh", argv...), nil
	case openidea.RemoteTargetWSL:
		if err := (openidea.RemoteTarget{Kind: openidea.RemoteTargetWSL, Distro: b.target.Distro}).Validate(); err != nil {
			return nil, err
		}
		argv := []string{"-d", b.target.Distro, "--exec", "sh", "-c", script, "sh"}
		argv = append(argv, args...)
		return b.command(ctx, "wsl.exe", argv...), nil
	default:
		return nil, fmt.Errorf("target %s does not run shell commands", b.target.Kind)
	}
}

func (b *remoteBackend) runCaptured(
	ctx context.Context,
	cmd *exec.Cmd,
	input io.Reader,
	timeout time.Duration,
) ([]byte, error) {
	if input != nil {
		cmd.Stdin = input
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, b.classifyError(ctx, err, "")
	}
	var timedOut atomic.Bool
	timer := time.AfterFunc(timeout, func() {
		timedOut.Store(true)
		_ = terminateRemoteCommand(cmd)
	})
	err := cmd.Wait()
	timer.Stop()
	if err != nil {
		if timedOut.Load() {
			return nil, fmt.Errorf("the remote command did not finish in time")
		}
		text := strings.TrimSpace(stderr.String())
		if text == "" {
			text = strings.TrimSpace(stdout.String())
		}
		return nil, b.classifyError(ctx, err, text)
	}
	return stdout.Bytes(), nil
}

func (b *remoteBackend) command(ctx context.Context, program string, args ...string) *exec.Cmd {
	cmd := b.runner.command(ctx, program, args...)
	if b.target.Kind == openidea.RemoteTargetSSH && b.secret != "" {
		askpass := sshAskpassProgram()
		if askpass != "" {
			cmd.Env = append(os.Environ(),
				"SSH_ASKPASS="+askpass,
				"SSH_ASKPASS_REQUIRE=force",
				"GOIDEA_SSH_ASKPASS=1",
				"GOIDEA_SSH_ASKPASS_SECRET="+b.secret,
			)
		}
	}
	return cmd
}

func (b *remoteBackend) sshArgs(ctx context.Context, interactive bool) []string {
	_ = ctx
	args := []string{
		"-T",
		"-o", "ConnectTimeout=15",
		"-o", "ServerAliveInterval=30",
		"-o", "ServerAliveCountMax=3",
		"-o", "StrictHostKeyChecking=yes",
		"-o", "ClearAllForwardings=yes",
	}
	if b.secret == "" {
		args = append(args, "-o", "BatchMode=yes")
	}
	host := strings.TrimSpace(b.ssh.Host)
	if host == "" || strings.HasPrefix(host, "-") || strings.ContainsAny(host, " \t\r\n\x00/\\") {
		args = append(args, "__invalid_goidea_ssh_host__")
		return args
	}
	if b.ssh.Auth == "openSsh" {
		args = append(args, host)
		return args
	}
	args = append(args, "-o", "IdentitiesOnly=yes")
	if b.ssh.Port != nil && *b.ssh.Port > 0 {
		args = append(args, "-p", strconv.Itoa(*b.ssh.Port))
	}
	if user := strings.TrimSpace(b.ssh.User); user != "" {
		args = append(args, "-l", user)
	}
	if b.ssh.Auth == "password" {
		args = append(args, "-o", "PreferredAuthentications=password,keyboard-interactive", "-o", "PubkeyAuthentication=no")
	} else if key := strings.TrimSpace(b.ssh.KeyFile); key != "" {
		args = append(args, "-i", expandUserPath(key))
	}
	args = append(args, host)
	return args
}

func remoteShellInvocation(script string, args []string) []string {
	out := []string{"sh", "-c", script, "sh"}
	for _, arg := range args {
		out = append(out, arg)
	}
	for i := range out {
		out[i] = shellQuote(out[i])
	}
	return []string{strings.Join(out, " ")}
}

func expandUserPath(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			if path == "~" {
				return home
			}
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func (b *remoteBackend) classifyError(ctx context.Context, err error, text string) error {
	lower := strings.ToLower(text)
	if b.target.Kind == openidea.RemoteTargetSSH &&
		(strings.Contains(lower, "remote host identification has changed") ||
			strings.Contains(lower, "host key verification failed") ||
			strings.Contains(lower, "unknown host key")) {
		if strings.Contains(lower, "identification has changed") {
			return fmt.Errorf("The SSH host key changed. Remove the old entry from known_hosts only after verifying the host. %s", tailText(text, 500))
		}
		fingerprint, scanErr := b.hostKeyFingerprint(ctx)
		if scanErr != nil {
			return fmt.Errorf("Unknown SSH host key for %s. %s", b.ssh.Label(), tailText(text, 500))
		}
		return &remoteHostTrustError{ConfigID: b.ssh.ID, Host: b.ssh.Label(), Fingerprint: fingerprint}
	}
	secretProblem := strings.Contains(lower, "permission denied") ||
		strings.Contains(lower, "authentication failed") ||
		strings.Contains(lower, "incorrect passphrase") ||
		strings.Contains(lower, "load key") ||
		strings.Contains(lower, "no such identity")
	if b.secret == "" && b.sshAuthCanPrompt() && secretProblem {
		return &remoteSecretRequiredError{ConfigID: b.ssh.ID, Host: b.ssh.Label(), Auth: b.ssh.Auth}
	}
	if text == "" {
		return err
	}
	return fmt.Errorf("%s", tailText(text, 800))
}

func (b *remoteBackend) sshAuthCanPrompt() bool {
	return b.target.Kind == openidea.RemoteTargetSSH && (b.ssh.Auth == "password" || b.ssh.Auth == "keyPair")
}

type remoteHostTrustError struct {
	ConfigID    string
	Host        string
	Fingerprint string
}

func (e *remoteHostTrustError) Error() string {
	return fmt.Sprintf("Unknown SSH host key for %s (%s)", e.Host, e.Fingerprint)
}

type remoteSecretRequiredError struct {
	ConfigID string
	Host     string
	Auth     string
}

func (e *remoteSecretRequiredError) Error() string {
	if e.Auth == "keyPair" {
		return "SSH key passphrase required for " + e.Host
	}
	return "SSH password required for " + e.Host
}

func (b *remoteBackend) hostKeyFingerprint(ctx context.Context) (string, error) {
	raw, err := b.scanHostKey(ctx)
	if err != nil {
		return "", err
	}
	return b.fingerprintHostKey(ctx, raw)
}

func (b *remoteBackend) fingerprintHostKey(ctx context.Context, raw []byte) (string, error) {
	keygen, err := b.runner.lookPath("ssh-keygen")
	if err != nil {
		return "", err
	}
	fp := b.runner.command(ctx, keygen, "-lf", "-")
	fp.Stdin = bytes.NewReader(raw)
	out, err := fp.Output()
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(out))
	if len(fields) < 2 {
		return "", fmt.Errorf("cannot fingerprint the host key")
	}
	return fields[1], nil
}

func (b *remoteBackend) trustHost(ctx context.Context) error {
	return b.trustHostMatching(ctx, "")
}

// trustHostMatching scans once and writes only that exact key. The expected
// fingerprint closes the gap between the prompt and the user's confirmation:
// a concurrent reconnect cannot replace the key after a separate display scan.
func (b *remoteBackend) trustHostMatching(ctx context.Context, expectedFingerprint string) error {
	raw, err := b.scanHostKey(ctx)
	if err != nil {
		return err
	}
	if expected := strings.TrimSpace(expectedFingerprint); expected != "" {
		fingerprint, err := b.fingerprintHostKey(ctx, raw)
		if err != nil {
			return err
		}
		if fingerprint != expected {
			return fmt.Errorf("The SSH host key changed while connecting. The connection was not trusted.")
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		return err
	}
	known := filepath.Join(home, ".ssh", "known_hosts")
	f, err := os.OpenFile(known, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(raw)
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

// scanHostKey returns exactly the key whose fingerprint the confirmation
// shows. Trusting every algorithm returned by ssh-keyscan would silently trust
// keys the user never reviewed.
func (b *remoteBackend) scanHostKey(ctx context.Context) ([]byte, error) {
	scan, err := b.runner.lookPath("ssh-keyscan")
	if err != nil {
		return nil, fmt.Errorf("ssh-keyscan is not installed")
	}
	host, port, err := b.sshEndpoint(ctx)
	if err != nil {
		return nil, err
	}
	for _, algorithm := range []string{"ed25519", "ecdsa", "rsa"} {
		args := []string{"-T", "10", "-t", algorithm}
		if port != "22" {
			args = append(args, "-p", port)
		}
		args = append(args, host)
		raw, scanErr := b.runner.command(ctx, scan, args...).Output()
		if scanErr == nil {
			if line := firstScannedHostKey(raw); len(line) > 0 {
				return append(line, '\n'), nil
			}
		}
	}
	return nil, fmt.Errorf("cannot read the SSH host key")
}

func firstScannedHostKey(raw []byte) []byte {
	for _, line := range bytes.Split(raw, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 || line[0] == '#' {
			continue
		}
		fields := bytes.Fields(line)
		if len(fields) >= 3 && fields[1][0] != '!' {
			return append([]byte(nil), line...)
		}
	}
	return nil
}

func (b *remoteBackend) sshEndpoint(ctx context.Context) (string, string, error) {
	host := strings.TrimSpace(b.ssh.Host)
	port := "22"
	if b.ssh.Port != nil && *b.ssh.Port > 0 {
		port = strconv.Itoa(*b.ssh.Port)
	}
	if b.ssh.Auth != "openSsh" || host == "" {
		return host, port, nil
	}
	cmd := b.runner.command(ctx, "ssh", "-G", "-o", "BatchMode=yes", host)
	out, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("cannot resolve the SSH configuration %s: %w", host, err)
	}
	resolvedHost, resolvedPort, ok := parseSSHConfigTarget(string(out))
	if !ok {
		return "", "", fmt.Errorf("cannot resolve the SSH configuration %s", host)
	}
	return resolvedHost, resolvedPort, nil
}

func parseSSHConfigTarget(output string) (string, string, bool) {
	host := ""
	port := "22"
	for _, line := range strings.Split(output, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok {
			key, value, ok = strings.Cut(strings.TrimSpace(line), "\t")
		}
		if !ok {
			continue
		}
		switch key {
		case "hostname":
			host = strings.TrimSpace(value)
		case "port":
			port = strings.TrimSpace(value)
		}
	}
	number, err := strconv.Atoi(port)
	return host, port, host != "" && err == nil && number > 0 && number <= 65535
}

func (b *remoteBackend) browse(ctx context.Context, dir string) ([]openidea.RemoteDirectoryEntry, error) {
	if b.target.Kind != openidea.RemoteTargetSSH {
		return nil, fmt.Errorf("folder browsing requires an SSH connection")
	}
	script := `case "$1" in
  "~") cd "$HOME" ;;
  "~/"*) cd "$HOME/${1#~/}" ;;
  *) cd "$1" ;;
esac
base="$(pwd -P)" || exit
printf '%s\0' "$base"
for item in "$base"/* "$base"/.[!.]* "$base"/..?*; do
  [ -d "$item" ] || continue
  printf '%s\0' "${item##*/}"
done
`
	raw, err := b.runRemote(ctx, nil, 30*time.Second, script, []string{dir})
	if err != nil {
		return nil, err
	}
	parts := bytes.Split(raw, []byte{0})
	if len(parts) == 0 || len(bytes.TrimSpace(parts[0])) == 0 {
		return nil, fmt.Errorf("the remote host returned an empty folder listing")
	}
	base := strings.TrimSpace(string(parts[0]))
	out := make([]openidea.RemoteDirectoryEntry, 0, len(parts)-1)
	for _, part := range parts[1:] {
		name := strings.TrimSpace(string(part))
		if name == "" {
			continue
		}
		out = append(out, openidea.RemoteDirectoryEntry{
			Name:  name,
			Path:  path.Join(base, name),
			IsDir: true,
		})
	}
	return out, nil
}

func (b *remoteBackend) remoteHome() string { return b.home }

func (b *remoteBackend) forwardConfiguredPorts() error {
	if b.target.Kind != openidea.RemoteTargetDevContainer || b.devContainerConfig == "" {
		return nil
	}
	ports, err := loadDevContainerForwardPorts(b.devContainerConfig)
	if err != nil {
		return fmt.Errorf("read dev-container forwardPorts: %w", err)
	}
	var failures []string
	for _, configured := range ports {
		if _, err := b.startConfiguredForward(configured.Port, configured.Host); err != nil {
			host := "the container"
			if configured.Host != nil {
				host = *configured.Host
			}
			failures = append(failures, fmt.Sprintf("%s:%d: %v", host, configured.Port, err))
			if b.log != nil {
				_, _ = fmt.Fprintf(b.log, "remote: cannot forward %s:%d: %v\n", host, configured.Port, err)
			}
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("%s", strings.Join(failures, ", "))
	}
	return nil
}

func (b *remoteBackend) startForward(remotePort uint16, host *string) (openidea.ForwardedPort, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.configured != nil {
		delete(b.configured, remoteForwardKey(remotePort, host))
	}
	return b.startForwardLocked(remotePort, host)
}

func (b *remoteBackend) startConfiguredForward(remotePort uint16, host *string) (openidea.ForwardedPort, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	forwarded, err := b.startForwardLocked(remotePort, host)
	if err == nil {
		if b.configured == nil {
			b.configured = map[string]bool{}
		}
		b.configured[remoteForwardKey(remotePort, host)] = true
	}
	return forwarded, err
}

func (b *remoteBackend) startForwardLocked(remotePort uint16, host *string) (openidea.ForwardedPort, error) {
	if b.forwarder == nil {
		b.forwarder = newRemotePortForwarder(b)
	}
	return b.forwarder.Forward(remotePort, host)
}

func (b *remoteBackend) forwardIsConfigured(remotePort uint16, host *string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.configured[remoteForwardKey(remotePort, host)]
}

func remoteForwardKey(remotePort uint16, host *string) string {
	if host == nil || *host == "" {
		return strconv.Itoa(int(remotePort))
	}
	return net.JoinHostPort(*host, strconv.Itoa(int(remotePort)))
}

func (b *remoteBackend) stopForward(remotePort uint16, host *string) {
	b.mu.Lock()
	f := b.forwarder
	if b.configured != nil {
		delete(b.configured, remoteForwardKey(remotePort, host))
	}
	b.mu.Unlock()
	if f != nil {
		f.Stop(remotePort, host)
	}
}

func (b *remoteBackend) listForwards() []openidea.ForwardedPort {
	b.mu.Lock()
	f := b.forwarder
	b.mu.Unlock()
	if f == nil {
		return nil
	}
	return f.List()
}

func (b *remoteBackend) forwardWebURL(rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "http" || parsed.Hostname() != "127.0.0.1" {
		return rawURL, nil
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil || port <= 0 || port > 65535 {
		return rawURL, nil
	}
	if b.target.Kind == openidea.RemoteTargetWSL {
		return rawURL, nil
	}
	b.mu.Lock()
	if b.webForwarder == nil {
		b.webForwarder = newRemotePortForwarder(b)
	}
	forwarder := b.webForwarder
	b.mu.Unlock()
	forwarded, err := forwarder.Forward(uint16(port), nil)
	if err != nil {
		return "", err
	}
	parsed.Host = "127.0.0.1:" + strconv.Itoa(int(forwarded.LocalPort))
	return parsed.String(), nil
}

func (b *remoteBackend) closeForwards() {
	b.mu.Lock()
	f := b.forwarder
	web := b.webForwarder
	b.forwarder = nil
	b.webForwarder = nil
	b.mu.Unlock()
	if f != nil {
		f.Close()
	}
	if web != nil {
		web.Close()
	}
}

func remoteLineWriter(w io.Writer, prefix string) io.Writer {
	if w == nil {
		return io.Discard
	}
	return &prefixLineWriter{w: w, prefix: prefix}
}

type prefixLineWriter struct {
	mu     sync.Mutex
	w      io.Writer
	prefix string
	line   []byte
}

func (w *prefixLineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.line = append(w.line, p...)
	for {
		i := bytes.IndexByte(w.line, '\n')
		if i < 0 {
			break
		}
		_, _ = fmt.Fprintf(w.w, "%s: %s\n", w.prefix, strings.TrimRight(string(w.line[:i]), "\r"))
		w.line = append(w.line[:0], w.line[i+1:]...)
	}
	return len(p), nil
}

func tailText(text string, limit int) string {
	text = strings.TrimSpace(text)
	if len(text) <= limit {
		return text
	}
	start := len(text) - limit
	for start < len(text) && text[start]&0xc0 == 0x80 {
		start++
	}
	return text[start:]
}

type progressReader struct {
	reader io.Reader
	total  int64
	read   int64
	report func(int64)
}

func (r *progressReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.read += int64(n)
	if r.report != nil {
		r.report(r.read)
	}
	return n, err
}

func floatPtr(v float64) *float64 { return &v }

func decodeWSLOutput(out []byte) string {
	if len(out) >= 2 && out[0] == 0xff && out[1] == 0xfe {
		u := make([]uint16, 0, (len(out)-2)/2)
		for i := 2; i+1 < len(out); i += 2 {
			u = append(u, uint16(out[i])|uint16(out[i+1])<<8)
		}
		return string(utf16.Decode(u))
	}
	if len(out) >= 2 && out[1] == 0 {
		u := make([]uint16, 0, len(out)/2)
		for i := 0; i+1 < len(out); i += 2 {
			u = append(u, uint16(out[i])|uint16(out[i+1])<<8)
		}
		return string(utf16.Decode(u))
	}
	return strings.TrimRight(string(out), "\r\n\x00")
}

func listWSLDistros(ctx context.Context, runner remoteProcessRunner) ([]string, error) {
	if runtime.GOOS != "windows" {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	out, err := runner.command(ctx, "wsl.exe", "-l", "-q").Output()
	if err != nil {
		return nil, fmt.Errorf("wsl.exe did not answer in time")
	}
	text := decodeWSLOutput(out)
	var outNames []string
	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		name := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "\ufeff"))
		if name != "" {
			outNames = append(outNames, name)
		}
	}
	return outNames, scanner.Err()
}

var errRemoteCancelled = errors.New("remote development cancelled")
