package openidea

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPreflightRejectsBadBinaries(t *testing.T) {
	if _, err := Preflight(LaunchConfig{}); err == nil || !strings.Contains(err.Error(), "server path") {
		t.Fatalf("empty path: %v", err)
	}
	dir := t.TempDir()
	plain := filepath.Join(dir, "plain")
	if err := os.WriteFile(plain, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Preflight(LaunchConfig{ServerPath: plain}); err == nil || !strings.Contains(err.Error(), "server exec") {
		t.Fatalf("non-executable: %v", err)
	}
	if _, err := Preflight(LaunchConfig{ServerPath: dir}); err == nil || !strings.Contains(err.Error(), "is a directory") {
		t.Fatalf("directory: %v", err)
	}
}

func TestPreflightAcceptsThePinnedServer(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	old := probeVersion
	probeVersion = func(string) (string, uint32, error) { return "0.1.44", 2, nil }
	defer func() { probeVersion = old }()

	home := filepath.Join(t.TempDir(), "home")
	rep, err := Preflight(LaunchConfig{ServerPath: self, Home: home, MinVersion: "0.1.44"})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Version != "0.1.44" || rep.Protocol != 2 || len(rep.SHA256) != 64 || rep.Size == 0 {
		t.Fatalf("report = %+v", rep)
	}
	if _, err := os.Stat(home); err != nil {
		t.Fatalf("home not created: %v", err)
	}
	if _, err := Preflight(LaunchConfig{ServerPath: self, ExpectedSHA256: strings.Repeat("0", 64)}); err == nil ||
		!strings.Contains(err.Error(), "server sha256") {
		t.Fatalf("bad checksum: %v", err)
	}
	if _, err := Preflight(LaunchConfig{ServerPath: self, MinVersion: "0.1.45"}); err == nil ||
		!strings.Contains(err.Error(), "older than") {
		t.Fatalf("old version: %v", err)
	}
	probeVersion = func(string) (string, uint32, error) { return "0.1.44", 1, nil }
	if _, err := Preflight(LaunchConfig{ServerPath: self, Protocol: 2}); err == nil ||
		!strings.Contains(err.Error(), "protocol 1") {
		t.Fatalf("protocol: %v", err)
	}
}

func TestCompareVersion(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.1.44", "0.1.44", 0},
		{"0.1.44", "0.1.41", 1},
		{"0.1.9", "0.1.10", -1},
		{"1.0", "1.0.0", 0},
		{"1.0.0-rc1", "1.0.0", -1},
		{"1.0.0", "1.0.0-rc1", 1},
	}
	for _, c := range cases {
		if got := compareVersion(c.a, c.b); got != c.want {
			t.Fatalf("compareVersion(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestManagerReportsFailedPreflight(t *testing.T) {
	m := NewManager(LaunchConfig{ServerPath: filepath.Join(t.TempDir(), "missing")})
	defer m.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := m.Ensure(ctx); err == nil {
		t.Fatal("a missing server was accepted")
	}
	if st := m.State(); st.Phase != "failed" || st.Running {
		t.Fatalf("state after failed preflight = %+v", st)
	}
}
