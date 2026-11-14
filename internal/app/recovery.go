package app

// SmokeRecovery kills the backend process and records whether GoIDEA brings a
// new one up, reopens the same project and keeps the open documents, for the
// real-server acceptance run.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	var tabs int
	m.post(func() { tabs = len(m.tabs) })
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
	var after int
	m.post(func() { after = len(m.tabs) })
	out["tabsAfter"] = after
	out["docsKept"] = after == tabs
	out["elapsedMs"] = time.Since(started).Milliseconds()
	if !restored {
		return out, fmt.Errorf("后端 90 秒内没有恢复项目")
	}
	return out, nil
}

// SmokeRollback opens a folder that cannot be a project and reports that the
// open project's tabs and documents survived the failed switch, for the
// real-server acceptance run.
func (m *Model) SmokeRollback(ctx context.Context) (map[string]any, error) {
	out := map[string]any{}
	root := m.Root()
	if root == "" {
		return out, fmt.Errorf("没有打开项目")
	}
	var tabs int
	var firstDoc string
	m.post(func() {
		tabs = len(m.tabs)
		if t := m.tab(); t != nil {
			firstDoc = t.doc.Path()
		}
	})
	out["rootBefore"], out["tabsBefore"], out["docBefore"] = root, tabs, firstDoc
	bad := filepath.Join(os.TempDir(), "goidea-no-such-project-4d2c", "missing")
	m.post(func() { m.openProject(bad, nil) })
	deadline := time.Now().Add(90 * time.Second)
	for {
		var busy bool
		var after int
		var rootAfter, banner string
		var docAlive bool
		m.post(func() {
			busy, after, rootAfter = m.busy, len(m.tabs), m.Root()
			banner = m.banner
			docAlive = firstDoc != "" && m.reg.Get(firstDoc) != nil
		})
		if !busy {
			out["rootAfter"], out["tabsAfter"], out["docAlive"], out["banner"] = rootAfter, after, docAlive, banner
			switch {
			case layoutKey(rootAfter) != layoutKey(root):
				return out, fmt.Errorf("失败切换后不在原项目")
			case banner == "":
				return out, fmt.Errorf("失败切换没有报错")
			case after != tabs:
				return out, fmt.Errorf("标签页没有恢复：%d → %d", tabs, after)
			case firstDoc != "" && !docAlive:
				return out, fmt.Errorf("原项目的文档已关闭")
			}
			return out, nil
		}
		if time.Now().After(deadline) {
			return out, fmt.Errorf("失败切换没有结束")
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// SmokeSwitch opens another project through the normal transactional path and
// switches back, recording both roots and the backend's project_current, for
// the real-server acceptance run.
func (m *Model) SmokeSwitch(ctx context.Context, other string) (map[string]any, error) {
	out := map[string]any{}
	from := m.Root()
	if from == "" {
		return out, fmt.Errorf("没有打开项目")
	}
	out["from"], out["to"] = from, other
	current := func() string {
		raw, err := m.mgr.Invoke(ctx, Window, "project_current", nil)
		if err != nil {
			return ""
		}
		var info struct {
			Root string `json:"root"`
		}
		_ = json.Unmarshal(raw, &info)
		return info.Root
	}
	wait := func(dir string) error {
		deadline := time.Now().Add(90 * time.Second)
		for time.Now().Before(deadline) {
			var busy bool
			var root string
			m.post(func() { busy, root = m.busy, m.Root() })
			if !busy && layoutKey(root) == layoutKey(dir) && layoutKey(current()) == layoutKey(dir) {
				return nil
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(100 * time.Millisecond):
			}
		}
		return fmt.Errorf("切换到 %s 超时", dir)
	}
	started := time.Now()
	m.post(func() { m.openProject(other, nil) })
	if err := wait(other); err != nil {
		out["switched"] = false
		out["elapsedMs"] = time.Since(started).Milliseconds()
		return out, err
	}
	out["switched"] = true
	m.post(func() { m.openProject(from, nil) })
	if err := wait(from); err != nil {
		out["back"] = false
		return out, err
	}
	out["back"] = true
	out["elapsedMs"] = time.Since(started).Milliseconds()
	return out, nil
}
