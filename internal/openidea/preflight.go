package openidea

import (
	"context"
	"crypto/sha256"
	"debug/elf"
	"debug/macho"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// PreflightError is a server binary that cannot be used, with the part that
// failed: "path", "exec", "arch", "sha256", "version" or "home".
type PreflightError struct {
	Kind   string
	Path   string
	Detail string
}

func (e *PreflightError) Error() string {
	return fmt.Sprintf("openidea: server %s: %s", e.Kind, e.Detail)
}

// Report is what the preflight learned about the binary before it ran.
type Report struct {
	Path     string
	SHA256   string
	Version  string
	Protocol uint32
	Size     int64
}

var serverVersionRE = regexp.MustCompile(`openidea-server\s+(\S+)\s+\(protocol\s+(\d+)\)`)

// Preflight checks a server binary before Launch: it must exist, be a
// regular executable for this OS and CPU, match the expected checksum when one
// is pinned, answer --version with the configured protocol, and be at least
// the minimum version. Home is created when it is set.
func Preflight(cfg LaunchConfig) (Report, error) {
	r := Report{Path: cfg.ServerPath}
	if cfg.ServerPath == "" {
		return r, &PreflightError{Kind: "path", Detail: "no binary path"}
	}
	info, err := os.Stat(cfg.ServerPath)
	if err != nil {
		return r, &PreflightError{Kind: "path", Path: cfg.ServerPath, Detail: err.Error()}
	}
	if info.IsDir() {
		return r, &PreflightError{Kind: "path", Path: cfg.ServerPath, Detail: "is a directory"}
	}
	r.Size = info.Size()
	if err := executable(cfg.ServerPath, info); err != nil {
		return r, err
	}
	if err := checkArch(cfg.ServerPath); err != nil {
		return r, err
	}
	sum, err := fileSHA256(cfg.ServerPath)
	if err != nil {
		return r, &PreflightError{Kind: "path", Path: cfg.ServerPath, Detail: err.Error()}
	}
	r.SHA256 = sum
	if cfg.ExpectedSHA256 != "" && !strings.EqualFold(cfg.ExpectedSHA256, sum) {
		return r, &PreflightError{Kind: "sha256", Path: cfg.ServerPath,
			Detail: fmt.Sprintf("%s, want %s", sum, cfg.ExpectedSHA256)}
	}
	protocol := cfg.Protocol
	if protocol == 0 {
		protocol = 2
	}
	version, speaks, err := probeVersion(cfg.ServerPath)
	if err != nil {
		return r, &PreflightError{Kind: "version", Path: cfg.ServerPath, Detail: err.Error()}
	}
	r.Version, r.Protocol = version, speaks
	if speaks != protocol {
		return r, &PreflightError{Kind: "version", Path: cfg.ServerPath,
			Detail: fmt.Sprintf("speaks protocol %d, need %d", speaks, protocol)}
	}
	if cfg.MinVersion != "" && compareVersion(version, cfg.MinVersion) < 0 {
		return r, &PreflightError{Kind: "version", Path: cfg.ServerPath,
			Detail: fmt.Sprintf("%s is older than %s", version, cfg.MinVersion)}
	}
	if cfg.Home != "" {
		if err := os.MkdirAll(cfg.Home, 0o755); err != nil {
			return r, &PreflightError{Kind: "home", Path: cfg.Home, Detail: err.Error()}
		}
	}
	return r, nil
}

func executable(path string, info os.FileInfo) error {
	if runtime.GOOS == "windows" {
		if strings.EqualFold(filepath.Ext(path), ".exe") {
			return nil
		}
		return &PreflightError{Kind: "exec", Path: path, Detail: "not a .exe"}
	}
	if info.Mode().Perm()&0o111 == 0 {
		return &PreflightError{Kind: "exec", Path: path,
			Detail: "not executable (chmod +x " + path + ")"}
	}
	return nil
}

// checkArch reads the binary's CPU from its Mach-O or ELF header and refuses
// one built for another CPU, which would fail with a cryptic exec error.
func checkArch(path string) error {
	want := runtime.GOARCH
	switch runtime.GOOS {
	case "darwin":
		f, err := macho.Open(path)
		if err != nil {
			if strings.Contains(err.Error(), "not a Mach-O") {
				return &PreflightError{Kind: "arch", Path: path, Detail: "not a Mach-O binary"}
			}
			return &PreflightError{Kind: "arch", Path: path, Detail: err.Error()}
		}
		defer f.Close()
		var ok bool
		switch want {
		case "arm64":
			ok = f.Cpu == macho.CpuArm64
		case "amd64":
			ok = f.Cpu == macho.CpuAmd64
		}
		if !ok {
			return &PreflightError{Kind: "arch", Path: path,
				Detail: fmt.Sprintf("built for %v, this Mac runs %s", f.Cpu, want)}
		}
	case "linux":
		f, err := elf.Open(path)
		if err != nil {
			return &PreflightError{Kind: "arch", Path: path, Detail: err.Error()}
		}
		defer f.Close()
		var ok bool
		switch want {
		case "arm64":
			ok = f.Machine == elf.EM_AARCH64
		case "amd64":
			ok = f.Machine == elf.EM_X86_64
		}
		if !ok {
			return &PreflightError{Kind: "arch", Path: path,
				Detail: fmt.Sprintf("built for %v, this machine runs %s", f.Machine, want)}
		}
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// probeVersion asks the binary for its version, replaced in tests.
var probeVersion = serverVersion

// serverVersion asks the binary for the version and protocol it speaks.
func serverVersion(path string) (string, uint32, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--version").Output()
	if err != nil {
		return "", 0, fmt.Errorf("--version: %w", err)
	}
	m := serverVersionRE.FindStringSubmatch(string(out))
	if m == nil {
		return "", 0, fmt.Errorf("--version printed %q", strings.TrimSpace(string(out)))
	}
	p, err := strconv.ParseUint(m[2], 10, 32)
	if err != nil {
		return "", 0, fmt.Errorf("--version protocol %q", m[2])
	}
	return m[1], uint32(p), nil
}

// compareVersion compares numeric dot versions; a suffix such as -rc1 sorts
// before the release of the same numbers.
func compareVersion(a, b string) int {
	split := func(v string) ([]int, string) {
		nums, suffix := v, ""
		if i := strings.IndexFunc(v, func(r rune) bool {
			return !(r >= '0' && r <= '9' || r == '.')
		}); i >= 0 {
			nums, suffix = v[:i], v[i:]
		}
		var parts []int
		for _, s := range strings.Split(nums, ".") {
			n, _ := strconv.Atoi(s)
			parts = append(parts, n)
		}
		return parts, suffix
	}
	pa, sa := split(a)
	pb, sb := split(b)
	for i := 0; i < len(pa) || i < len(pb); i++ {
		na, nb := 0, 0
		if i < len(pa) {
			na = pa[i]
		}
		if i < len(pb) {
			nb = pb[i]
		}
		if na != nb {
			if na < nb {
				return -1
			}
			return 1
		}
	}
	switch {
	case sa == sb:
		return 0
	case sa == "":
		return 1
	case sb == "":
		return -1
	case sa < sb:
		return -1
	}
	return 1
}
