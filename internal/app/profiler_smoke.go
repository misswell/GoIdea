package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"mayoi/internal/openidea"
)

// SmokeProfiler runs a real Node.js CPU profile through profiler_prepare,
// the normal run lifecycle, profiler_collect and the native snapshot view.
func (m *Model) SmokeProfiler(ctx context.Context) (map[string]any, error) {
	return m.SmokeProfilerMode(ctx, openidea.ProfileCPU)
}

// SmokeProfilerMode runs OpenIDEA's real profiler pipeline for CPU,
// allocation or live-object mode. The Java modes use an actual JVM so the
// async-profiler plan, GC-before-stop attach and collection path are exercised.
func (m *Model) SmokeProfilerMode(ctx context.Context, mode openidea.ProfileMode) (map[string]any, error) {
	root := m.Root()
	if root == "" {
		return nil, fmt.Errorf("profiler smoke needs an open project")
	}
	var command string
	var sourcePath string
	var sourceName string
	var programArgs []string
	cleanup := func() {}
	switch mode {
	case openidea.ProfileAllocations, openidea.ProfileLiveObjects:
		java, err := executableOnPath("java")
		if err != nil {
			return nil, fmt.Errorf("profiler smoke needs a JDK: %w", err)
		}
		sourceRoot := root
		mainJava := filepath.Join(root, "src", "main", "java")
		if info, statErr := os.Stat(mainJava); statErr == nil && info.IsDir() {
			sourceRoot = mainJava
		}
		className := fmt.Sprintf("GoideaProfilerSmoke%d", time.Now().UnixNano())
		sourcePath = filepath.Join(sourceRoot, className+".java")
		sourceName = filepath.Base(sourcePath)
		cleanup = func() { _ = os.Remove(sourcePath) }
		javaSource := fmt.Sprintf(`import java.util.ArrayList;
import java.util.List;

public class %s {
    static final List<byte[]> keep = new ArrayList<>();
    static volatile Object sink;

    static void retained() {
        for (int i = 0; i < 16384; i++) keep.add(new byte[4096]);
    }

    static void garbage() {
        for (int i = 0; i < 262144; i++) sink = new byte[1024];
    }

    public static void main(String[] args) throws Exception {
        retained();
        garbage();
        if (args.length > 0 && args[0].equals("wait")) {
            System.out.println("WAITING");
            System.out.flush();
            while (true) Thread.sleep(1000);
        }
    }
}
`, className)
		if err := os.WriteFile(sourcePath, []byte(javaSource), 0o600); err != nil {
			return nil, fmt.Errorf("write profiler smoke Java source: %w", err)
		}
		command = java
		programArgs = []string{sourcePath}
		if mode == openidea.ProfileLiveObjects {
			programArgs = append(programArgs, "wait")
		} else {
			programArgs = append(programArgs, "exit")
		}
	default:
		node, err := executableOnPath("node")
		if err != nil {
			return nil, fmt.Errorf("profiler smoke needs Node.js: %w", err)
		}
		script, err := os.CreateTemp(root, ".goidea-profiler-smoke-*.js")
		if err != nil {
			return nil, fmt.Errorf("create profiler smoke script: %w", err)
		}
		sourcePath = script.Name()
		sourceName = filepath.Base(sourcePath)
		cleanup = func() { _ = os.Remove(sourcePath) }
		const source = `function profileMe(depth) {
  let total = 0;
  for (let i = 0; i < 4000000; i++) total += (i % 17) * depth;
  return total;
}
let answer = 0;
for (let round = 0; round < 8; round++) answer += profileMe(round + 1);
console.log("profiler-smoke", answer);
`
		if _, err := script.WriteString(source); err != nil {
			_ = script.Close()
			cleanup()
			return nil, fmt.Errorf("write profiler smoke script: %w", err)
		}
		if err := script.Close(); err != nil {
			cleanup()
			return nil, fmt.Errorf("close profiler smoke script: %w", err)
		}
		command = node
		programArgs = []string{sourcePath}
	}
	defer cleanup()

	raw, err := json.Marshal(map[string]any{
		"id":               "profiler-smoke",
		"name":             "Profiler " + profileModeName(mode) + " Smoke",
		"kind":             "shell",
		"command":          command,
		"args":             programArgs,
		"cwd":              root,
		"env":              profilerSmokeEnvironment(),
		"detected":         true,
		"isTest":           false,
		"debugAdapter":     nil,
		"includeSystemEnv": true,
	})
	if err != nil {
		return nil, err
	}
	cfg, err := openidea.DecodeRunConfig(raw)
	if err != nil {
		return nil, err
	}
	if _, err := m.mgr.Invoke(ctx, Window, "project_trust_set", map[string]any{
		"path": root, "trusted": true, "folder": true,
	}); err != nil {
		return nil, fmt.Errorf("trust profiler smoke project: %w", err)
	}
	m.post(func() {
		m.run.trusted = true
		m.run.loaded = true
		m.run.configs = []openidea.RunConfig{*cfg}
		m.selectRunConfig(cfg.ID)
		m.profiler.err = ""
		m.profiler.notice = ""
	})
	m.post(func() { m.RunWithProfiler(mode) })

	stopRequested := false
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		var failure string
		var out map[string]any
		m.post(func() {
			if m.profiler.err != "" {
				failure = m.profiler.err
				return
			}
			if m.run.err != "" {
				failure = m.run.err
				return
			}
			if mode == openidea.ProfileLiveObjects && !stopRequested &&
				m.run.running && runOutputContains(m.run.output, "WAITING") {
				stopRequested = true
				m.StopRun()
			}
			snapshot := m.activeProfileSnapshot()
			if snapshot == nil || m.profiler.collecting {
				return
			}
			profile := snapshot.profile
			out = map[string]any{
				"profiler":      profile.Profiler,
				"unit":          profile.Unit,
				"mode":          profile.Mode,
				"total":         profileTotal(profile),
				"frames":        len(profile.Frames),
				"nodes":         len(profile.Nodes),
				"methods":       len(profile.Methods),
				"snapshots":     len(m.profiler.snapshots),
				"window":        m.dockWindowShown("profiler"),
				"runFinished":   !m.run.running && !m.run.awaiting,
				"source":        sourceName,
				"platform":      runtime.GOOS,
				"stopRequested": stopRequested,
				"notice":        snapshot.notice,
			}
		})
		if failure != "" {
			return out, fmt.Errorf("profiler smoke: %s", failure)
		}
		if out != nil {
			if mode == openidea.ProfileLiveObjects && !stopRequested {
				return out, fmt.Errorf("profiler smoke: live-object JVM never reached WAITING")
			}
			if mode == openidea.ProfileLiveObjects && out["notice"] != "" {
				return out, fmt.Errorf("profiler smoke: %v", out["notice"])
			}
			return out, nil
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-ticker.C:
		}
	}
}

func profilerSmokeEnvironment() map[string]string {
	env := map[string]string{}
	if home := os.Getenv("ASYNC_PROFILER_HOME"); home != "" {
		env["ASYNC_PROFILER_HOME"] = home
	}
	return env
}

func runOutputContains(lines []outputLine, wanted string) bool {
	for _, line := range lines {
		if strings.Contains(line.text, wanted) {
			return true
		}
	}
	return false
}

func executableOnPath(name string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", err
	}
	return filepath.Abs(path)
}
