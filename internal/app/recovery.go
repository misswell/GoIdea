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
		return out, fmt.Errorf("The backend is not running")
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
		return out, fmt.Errorf("The backend did not restore the project within 90 seconds")
	}
	return out, nil
}

// rollbackSnapshot is the lifecycle state a failed switch must preserve.
type rollbackSnapshot struct {
	root        string
	tabs        int
	active      string
	split       bool
	splitDown   bool
	splitActive int
	document    string
	splitDoc    string
	recents     []RecentProject
	recentPaths []string
}

func (m *Model) rollbackSnapshot() rollbackSnapshot {
	var snapshot rollbackSnapshot
	m.post(func() {
		snapshot.root = realPath(m.Root())
		if snapshot.root == "." {
			snapshot.root = ""
		}
		snapshot.tabs = len(m.tabs)
		snapshot.active = tabPath(m.tab())
		if snapshot.active != "" {
			snapshot.active = realPath(snapshot.active)
		}
		if m.split != nil {
			snapshot.split = true
			snapshot.splitDown = m.splitDown
			snapshot.splitActive = m.split.active
			if i := m.split.active; i >= 0 && i < len(m.split.tabs) {
				snapshot.splitDoc = tabPath(m.split.tabs[i])
			}
		}
		if t := m.tab(); t != nil && t.kind == "" {
			snapshot.document = t.doc.Path()
		}
		if snapshot.document != "" {
			snapshot.document = realPath(snapshot.document)
		}
		if snapshot.splitDoc != "" {
			snapshot.splitDoc = realPath(snapshot.splitDoc)
		}
		snapshot.recents = append([]RecentProject(nil), m.recents...)
		snapshot.recentPaths = recentPaths(snapshot.recents)
	})
	return snapshot
}

func sameProjectRoot(a, b string) bool {
	if a == "" || b == "" {
		return a == b
	}
	return layoutKey(a) == layoutKey(b)
}

func sameRecentProjects(a, b []RecentProject) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sameStringOrder(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// SmokeRollback restores a rejected project and reports that the UI, backend,
// recents, active/split groups and saved session all remained on the open one.
// It is used by the real-server acceptance run.
func (m *Model) SmokeRollback(ctx context.Context) (map[string]any, error) {
	out := map[string]any{}
	m.post(func() {
		if m.split == nil {
			if t := m.tab(); t != nil {
				if duplicate := m.duplicateTab(t); duplicate != nil {
					m.split = &splitGroup{tabs: []*Tab{duplicate}}
					m.splitDown = true
				}
			}
		}
		m.saveSessionNow()
	})
	before := m.rollbackSnapshot()
	if before.root == "" {
		return out, fmt.Errorf("No project is open")
	}
	savedBefore := Session{}
	if m.cfg.SessionPath != "" {
		savedBefore = LoadSession(m.cfg.SessionPath)
	}
	savedRecentPathsBefore := recentPaths(sessionRecents(savedBefore))
	savedActiveBefore := savedBefore.Focus
	if savedActiveBefore != "" {
		savedActiveBefore = realPath(savedActiveBefore)
	}
	savedSplitDocBefore := ""
	if i := savedBefore.SplitActive; i >= 0 && i < len(savedBefore.SplitFiles) {
		savedSplitDocBefore = realPath(savedBefore.SplitFiles[i])
	}
	var current string
	raw, err := m.invoke(ctx, "project_current", nil)
	if err == nil {
		var info struct {
			Root string `json:"root"`
		}
		if json.Unmarshal(raw, &info) == nil {
			current = realPath(info.Root)
		}
	}
	out["rootBefore"] = before.root
	out["tabsBefore"] = before.tabs
	out["docBefore"] = before.document
	out["activeBefore"] = before.active
	out["splitBefore"] = before.split
	out["splitDownBefore"] = before.splitDown
	out["splitActiveBefore"] = before.splitActive
	out["backendRootBefore"] = current
	out["recentPathsBefore"] = before.recentPaths
	out["sessionRootBefore"] = realPath(savedBefore.Root)
	out["sessionActiveBefore"] = savedActiveBefore
	out["sessionSplitBefore"] = len(savedBefore.SplitFiles) > 0
	out["sessionSplitActiveBefore"] = savedBefore.SplitActive
	out["sessionRecentPathsBefore"] = savedRecentPathsBefore

	rollbackDir, err := os.MkdirTemp("", "goidea-rollback-")
	if err != nil {
		return out, err
	}
	defer os.RemoveAll(rollbackDir)
	bad := filepath.Join(rollbackDir, "not-a-project-directory")
	m.post(func() { m.openProject(bad, nil) })
	deadline := time.Now().Add(90 * time.Second)
	for {
		var busy bool
		var banner string
		after := m.rollbackSnapshot()
		m.post(func() {
			busy = m.busy
			banner = m.banner
		})
		if !busy {
			var currentAfter string
			raw, err := m.invoke(ctx, "project_current", nil)
			if err == nil {
				var info struct {
					Root string `json:"root"`
				}
				if json.Unmarshal(raw, &info) == nil {
					currentAfter = realPath(info.Root)
				}
			}
			savedAfter := Session{}
			if m.cfg.SessionPath != "" {
				savedAfter = LoadSession(m.cfg.SessionPath)
			}
			savedRecentPathsAfter := recentPaths(sessionRecents(savedAfter))
			savedActiveAfter := savedAfter.Focus
			if savedActiveAfter != "" {
				savedActiveAfter = realPath(savedActiveAfter)
			}
			savedSplitDocAfter := ""
			if i := savedAfter.SplitActive; i >= 0 && i < len(savedAfter.SplitFiles) {
				savedSplitDocAfter = realPath(savedAfter.SplitFiles[i])
			}
			out["rootAfter"] = after.root
			out["tabsAfter"] = after.tabs
			out["docAlive"] = after.document != "" && m.reg.Get(after.document) != nil
			out["banner"] = banner
			out["activeAfter"] = after.active
			out["splitAfter"] = after.split
			out["splitDownAfter"] = after.splitDown
			out["splitActiveAfter"] = after.splitActive
			out["backendRootAfter"] = currentAfter
			out["recentPathsAfter"] = after.recentPaths
			out["sessionRootAfter"] = realPath(savedAfter.Root)
			out["sessionActiveAfter"] = savedActiveAfter
			out["sessionSplitAfter"] = len(savedAfter.SplitFiles) > 0
			out["sessionSplitActiveAfter"] = savedAfter.SplitActive
			out["sessionDocumentAfter"] = savedSplitDocAfter
			out["sessionRecentPathsAfter"] = savedRecentPathsAfter
			recentsSame := sameRecentProjects(before.recents, after.recents)
			sessionRecentsSame := sameStringOrder(savedRecentPathsBefore, savedRecentPathsAfter)
			switch {
			case !sameProjectRoot(after.root, before.root):
				return out, fmt.Errorf("The failed switch left the original project")
			case banner == "":
				return out, fmt.Errorf("The failed switch reported no error")
			case after.tabs != before.tabs:
				return out, fmt.Errorf("The tabs were not restored: %d to %d", before.tabs, after.tabs)
			case after.active != before.active:
				return out, fmt.Errorf("The active tab changed from %s to %s", before.active, after.active)
			case after.split != before.split:
				return out, fmt.Errorf("The split group changed state")
			case after.splitDown != before.splitDown:
				return out, fmt.Errorf("The split group changed orientation")
			case after.split && after.splitActive != before.splitActive:
				return out, fmt.Errorf("The split active tab changed from %d to %d", before.splitActive, after.splitActive)
			case after.document != "" && !out["docAlive"].(bool):
				return out, fmt.Errorf("The original project's documents were closed")
			case !sameProjectRoot(current, before.root) || !sameProjectRoot(currentAfter, before.root):
				return out, fmt.Errorf("The backend did not remain on the original project")
			case !recentsSame || !sessionRecentsSame:
				return out, fmt.Errorf("The failed switch mutated the recent projects")
			case m.cfg.SessionPath != "" && !sameProjectRoot(savedAfter.Root, savedBefore.Root):
				return out, fmt.Errorf("The failed switch mutated the saved session root")
			case m.cfg.SessionPath != "" && savedActiveBefore != "" &&
				!sameProjectRoot(savedActiveAfter, savedActiveBefore):
				return out, fmt.Errorf("The failed switch mutated the saved active document")
			case m.cfg.SessionPath != "" && len(savedBefore.SplitFiles) > 0 &&
				(len(savedAfter.SplitFiles) == 0 ||
					savedAfter.SplitActive != savedBefore.SplitActive ||
					!sameProjectRoot(savedSplitDocAfter, savedSplitDocBefore)):
				return out, fmt.Errorf("The failed switch mutated the saved split group")
			}
			return out, nil
		}
		if time.Now().After(deadline) {
			return out, fmt.Errorf("The failed switch did not finish")
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
	// openProject resolves symlinks (/tmp on macOS), so the wait must compare
	// against the resolved path.
	other = realPath(other)
	if from == "" {
		return out, fmt.Errorf("No project is open")
	}
	out["from"], out["to"] = from, other
	var last string
	wait := func(dir string) error {
		deadline := time.Now().Add(90 * time.Second)
		for time.Now().Before(deadline) {
			var busy bool
			var root, status, phase string
			var current string
			m.post(func() {
				busy, root, status, phase = m.busy, m.Root(), m.status, m.Phase()
			})
			raw, err := m.mgr.Invoke(ctx, Window, "project_current", nil)
			if err == nil {
				var info struct {
					Root string `json:"root"`
				}
				_ = json.Unmarshal(raw, &info)
				current = info.Root
			}
			if !busy && layoutKey(root) == layoutKey(dir) && layoutKey(current) == layoutKey(dir) {
				return nil
			}
			last = fmt.Sprintf("busy=%v root=%q phase=%s current=%q status=%q", busy, root, phase, current, status)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(100 * time.Millisecond):
			}
		}
		return fmt.Errorf("Switching to %s timed out: %s", dir, last)
	}
	started := time.Now()
	m.post(func() { m.openProject(other, nil) })
	if err := wait(other); err != nil {
		out["switched"] = false
		out["elapsedMs"] = time.Since(started).Milliseconds()
		st := m.BackendState()
		out["restarts"] = st.Restarts
		out["backendPhase"] = st.Phase
		if st.Err != nil {
			out["backendErr"] = st.Err.Error()
		}
		m.post(func() {
			out["banner"] = m.banner
			var notes []string
			for _, n := range m.notify.items {
				notes = append(notes, n.title+": "+n.body)
			}
			out["notifications"] = notes
		})
		if m.cfg.Log != nil {
			fmt.Fprintln(m.cfg.Log, "=== smoke switch timeout: all goroutines ===")
			buf := make([]byte, 1<<20)
			n := runtime.Stack(buf, true)
			fmt.Fprint(m.cfg.Log, string(buf[:n]))
		}
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
