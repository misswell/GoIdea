package app

// SmokeRecovery kills the backend process and records whether GoIDEA brings a
// new one up, reopens the same project and keeps the open documents, for the
// real-server acceptance run.

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// SmokeRecovery crashes the backend on purpose and waits for recovery.
func (m *Model) SmokeRecovery(ctx context.Context) (map[string]any, error) {
	out := map[string]any{}
	s := m.mgr.Server()
	if s == nil {
		return out, fmt.Errorf("后端未运行")
	}
	root := m.Root()
	tabs := len(m.tabs)
	restarts := m.BackendState().Restarts
	out["pid"] = s.PID()
	out["rootBefore"] = root
	out["tabsBefore"] = tabs
	out["restartsBefore"] = restarts

	var killErr error
	if runtime.GOOS == "windows" {
		killErr = exec.Command("taskkill", "/PID", strconv.Itoa(s.PID()), "/F").Run()
	} else {
		killErr = exec.Command("kill", "-9", strconv.Itoa(s.PID())).Run()
	}
	if killErr != nil {
		return out, fmt.Errorf("kill server: %w", killErr)
	}
	out["killed"] = true

	started := time.Now()
	deadline := started.Add(90 * time.Second)
	var restored bool
	var restoredRoot string
	for time.Now().Before(deadline) {
		st := m.BackendState()
		if st.Running && st.Restarts > restarts {
			raw, err := m.mgr.Invoke(ctx, Window, "project_current", nil)
			if err == nil {
				var info struct {
					Root string `json:"root"`
				}
				if json.Unmarshal(raw, &info) == nil && strings.TrimSpace(info.Root) != "" {
					restoredRoot = info.Root
					if layoutKey(restoredRoot) == layoutKey(root) {
						restored = true
						break
					}
				}
			}
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	out["restartsAfter"] = m.BackendState().Restarts
	out["rootAfter"] = restoredRoot
	out["rootRestored"] = restored
	out["tabsAfter"] = len(m.tabs)
	out["docsKept"] = len(m.tabs) == tabs
	out["elapsedMs"] = time.Since(started).Milliseconds()
	if !restored {
		return out, fmt.Errorf("后端 90 秒内没有恢复项目")
	}
	return out, nil
}
