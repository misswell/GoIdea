package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"

	"mayoi/internal/app"
)

type smokeProjectWindow struct {
	Root    string `json:"root"`
	Tabs    int    `json:"tabs"`
	Backend bool   `json:"backend"`
	PID     int    `json:"pid"`
	Phase   string `json:"phase"`
}

func smokeProjectWindows(host *nativeWindowHost, started time.Time) {
	fields := map[string]any{}
	var first *nativeProjectWindow
	var second *nativeProjectWindow
	var empty *nativeProjectWindow
	defer func() {
		if raw, err := json.MarshalIndent(fields, "", "  "); err == nil {
			if path := os.Getenv("GOIDEA_SMOKE_OUT"); path != "" {
				_ = os.WriteFile(path, raw, 0o644)
			}
		}
		mygo.RunOnMain(func() {
			if first != nil {
				first.model.QuitImmediately()
			}
		})
	}()

	mygo.RunOnMain(func() {
		if len(host.windows) > 0 {
			first = host.windows[0]
		}
	})
	if first == nil {
		fields["windowsError"] = "the initial native project window is absent"
		return
	}
	if !waitProjectWindow(first.model, 50*time.Second) {
		fields["windowsError"] = "the initial project window did not become ready"
		return
	}

	secondRoot := os.Getenv("GOIDEA_SMOKE_WINDOW_PROJECT")
	if secondRoot == "" {
		secondRoot = os.Getenv("GOIDEA_SMOKE_SWITCH_TO")
	}
	if secondRoot == "" {
		fields["windowsError"] = "GOIDEA_SMOKE_WINDOW_PROJECT is empty"
		return
	}
	var secondFiles []string
	if raw := os.Getenv("GOIDEA_SMOKE_WINDOW_FILES"); raw != "" {
		secondFiles = filepath.SplitList(raw)
	}
	mygo.RunOnMain(func() {
		before := len(host.windows)
		first.model.RunActionWithArg("file.openInNewWindow", map[string]any{
			"path":  filepath.Clean(secondRoot),
			"files": secondFiles,
		})
		if len(host.windows) == before+1 {
			second = host.windows[len(host.windows)-1]
		}
	})
	if second == nil {
		fields["windowsError"] = "file.openInNewWindow did not create a native project window"
		return
	}
	if !waitProjectWindow(second.model, 60*time.Second) {
		fields["windowsError"] = "the second project window did not become ready"
		return
	}
	var focusBefore, focusAfter *app.Model
	mygo.RunOnMain(func() {
		focusBefore = host.active
		first.model.RunAction("window.next")
		focusAfter = host.active
	})
	fields["windowNext"] = focusAfter != nil && focusAfter != focusBefore
	mygo.RunOnMain(func() {
		before := len(host.windows)
		first.model.RunAction("window.new")
		if len(host.windows) == before+1 {
			empty = host.windows[len(host.windows)-1]
		}
	})
	if empty == nil {
		fields["windowsError"] = "window.new did not create a native Welcome window"
		return
	}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) && empty.model.BackendPID() == 0 {
		time.Sleep(100 * time.Millisecond)
	}
	emptySnapshot := captureSmokeWindow(empty.model)
	var countWithEmpty int
	mygo.RunOnMain(func() {
		countWithEmpty = host.WindowCount()
		empty.win.Close()
	})
	fields["newWindow"] = map[string]any{
		"windows": countWithEmpty,
		"root":    emptySnapshot.Root,
		"backend": emptySnapshot.Backend,
		"pid":     emptySnapshot.PID,
	}
	deadline = time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		var count int
		mygo.RunOnMain(func() { count = host.WindowCount() })
		if count == 2 && empty.model.BackendPID() == 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	firstBefore := captureSmokeWindow(first.model)
	secondBefore := captureSmokeWindow(second.model)
	var countBefore int
	mygo.RunOnMain(func() { countBefore = host.WindowCount() })
	fields["beforeClose"] = map[string]any{
		"windows": countBefore,
		"first":   firstBefore,
		"second":  secondBefore,
	}
	fields["distinctRoots"] = firstBefore.Root != "" &&
		secondBefore.Root != "" &&
		firstBefore.Root != secondBefore.Root
	fields["distinctBackends"] = firstBefore.PID > 0 &&
		secondBefore.PID > 0 &&
		firstBefore.PID != secondBefore.PID

	mygo.RunOnMain(second.win.Close)
	deadline = time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		var count int
		mygo.RunOnMain(func() { count = host.WindowCount() })
		if count == 1 && second.model.BackendPID() == 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	firstAfter := captureSmokeWindow(first.model)
	secondAfter := captureSmokeWindow(second.model)
	var countAfter int
	mygo.RunOnMain(func() { countAfter = host.WindowCount() })
	fields["afterClose"] = map[string]any{
		"windows": countAfter,
		"first":   firstAfter,
		"second":  secondAfter,
	}
	fields["onlyClosedWindowStopped"] = countAfter == 1 &&
		secondAfter.PID == 0 &&
		firstAfter.Backend &&
		firstAfter.PID == firstBefore.PID
	fields["windowPIDs"] = []int{firstBefore.PID, secondBefore.PID, emptySnapshot.PID}
	fields["allProjectWindowPIDs"] = []int{firstBefore.PID, secondBefore.PID, emptySnapshot.PID}
	fields["elapsedMs"] = time.Since(started).Milliseconds()
}

func waitProjectWindow(model *app.Model, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		root, tabs, backend := model.Snapshot()
		if root != "" && tabs > 0 && backend && model.BackendPID() > 0 {
			return true
		}
		time.Sleep(150 * time.Millisecond)
	}
	return false
}

func captureSmokeWindow(model *app.Model) smokeProjectWindow {
	root, tabs, backend := model.Snapshot()
	return smokeProjectWindow{
		Root:    root,
		Tabs:    tabs,
		Backend: backend,
		PID:     model.BackendPID(),
		Phase:   model.Phase(),
	}
}
