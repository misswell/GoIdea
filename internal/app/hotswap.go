package app

// HotSwap (IDEA: Run › Debugging Actions › Reload Changed Classes), the port
// of src/features/debugger/hotswap.ts through java-debug's hot code replace:
// compile with jdtls (`java_build_workspace`), then `debug_redefine_classes`
// in the running Java debug session. When jdtls finishes a build on its own
// (after a save) java-debug reports `BUILD_COMPLETE` and, like IDEA's default
// "Reload classes after compilation: Ask", the app offers the reload.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"mayoi/internal/openidea"
)

// hotSwapMessage renders the reload result (hotswap.ts hotSwapMessage, IDEA
// wording): level, title and body.
func hotSwapMessage(changed int, errors []string, bottomFrame bool) (string, string, string) {
	for _, e := range errors {
		if strings.TrimSpace(e) != "JVM contains obsolete methods" {
			return "error", "Hot Swap failed", strings.Join(errors, "\n")
		}
	}
	if len(errors) > 0 {
		body := "Active frames keep running the old code until they return; new calls use the reloaded code."
		if bottomFrame {
			body += " The lowest stack frame cannot be reset: re-enter the method or restart the debug session to run the new code."
		}
		return "warning", "Classes reloaded", body
	}
	if changed == 0 {
		return "info", "The loaded classes are current; nothing to reload.", ""
	}
	if changed == 1 {
		return "info", "Reloaded 1 class", ""
	}
	return "info", fmt.Sprintf("Reloaded %d classes", changed), ""
}

type hotSwapSession struct {
	id     uint32
	name   string
	module string
}

func (m *Model) hotSwapSessions(configID string) []hotSwapSession {
	m.debug.rememberDebugSession()
	out := make([]hotSwapSession, 0, len(m.debug.sessionOrder))
	for _, id := range m.debug.sessionOrder {
		state := m.debug.sessions[id]
		if state == nil || state.session == 0 || state.ended ||
			!state.capsKnown || !state.caps.HotCodeReplace {
			continue
		}
		if configID != "" && state.lastID != configID {
			continue
		}
		module := state.launchCwd
		if module == "" {
			module = m.Root()
		}
		name := state.tabName
		if name == "" {
			name = state.config
		}
		out = append(out, hotSwapSession{id: id, name: name, module: module})
	}
	return out
}

// HotSwapReload is debug.reloadChangedClasses: compile the changed classes
// and reload them into the running Java debug session. A failed compilation
// refuses the reload instead of applying stale classes.
func (m *Model) HotSwapReload() {
	m.hotSwapReload(true, "", nil)
}

// HotSwapReloadAfterBuild applies the classes jdtls compiled in response to a
// BUILD_COMPLETE event. Recompiling here would repeat the build that already
// completed and could replace the exact classes the adapter just announced.
func (m *Model) HotSwapReloadAfterBuild() {
	m.hotSwapReload(false, "", nil)
}

// HotSwapReloadForConfig applies Reload Changed Classes only to sessions
// belonging to configID and waits for the reload result. Update Application
// uses this so a Spring Boot or Tomcat update cannot reload another debug
// session, and so it can wait before deciding whether a Tomcat redeploy is
// needed.
func (m *Model) HotSwapReloadForConfig(configID string) bool {
	result := make(chan bool, 1)
	m.post(func() {
		m.hotSwapReload(true, configID, result)
	})
	return <-result
}

// HotSwapReloadForConfigAsync starts the scoped action without blocking the
// main thread. It is used by the action registry when an action argument
// carries a configuration id.
func (m *Model) HotSwapReloadForConfigAsync(configID string) bool {
	if len(m.hotSwapSessions(configID)) == 0 || m.debug.reloading {
		return false
	}
	m.hotSwapReload(true, configID, nil)
	return true
}

func hotSwapConfigIDArg(arg any) string {
	switch value := arg.(type) {
	case string:
		return strings.TrimSpace(value)
	case map[string]any:
		for _, key := range []string{"configId", "configID"} {
			if text, ok := value[key].(string); ok && strings.TrimSpace(text) != "" {
				return strings.TrimSpace(text)
			}
		}
	}
	return ""
}

// HotSwapReloadAfterBuildForConfig is the external-build variant of
// HotSwapReloadForConfig. It does not compile the workspace again.
func (m *Model) HotSwapReloadAfterBuildForConfig(configID string) bool {
	result := make(chan bool, 1)
	m.post(func() {
		m.hotSwapReload(false, configID, result)
	})
	return <-result
}

func (m *Model) HotSwapReloadAfterBuildForConfigAsync(configID string) bool {
	if len(m.hotSwapSessions(configID)) == 0 || m.debug.reloading {
		return false
	}
	m.hotSwapReload(false, configID, nil)
	return true
}

func (m *Model) hotSwapReload(build bool, configID string, result chan<- bool) {
	sessions := m.hotSwapSessions(configID)
	if len(sessions) == 0 {
		m.debug.err = "No live Java debug session supports Reload Changed Classes"
		m.invalidate()
		if result != nil {
			result <- false
		}
		return
	}
	if m.debug.reloading {
		if result != nil {
			result <- false
		}
		return
	}
	tabs := append([]*Tab(nil), m.tabs...)
	root := m.Root()
	cancelSignal := make(chan struct{})
	m.debug.reloading = true
	m.debug.hotSwapCancel = cancelSignal
	if build {
		m.debug.hotSwapQuietUntil = time.Now().Add(60 * time.Second)
		m.debug.hotSwapStage = "called"
		m.debug.status = "Compiling the changed classes…"
	} else {
		m.debug.hotSwapQuietUntil = time.Now().Add(2 * time.Second)
		m.debug.hotSwapStage = "reloading"
		m.debug.status = "Reloading changed classes…"
	}
	m.debug.err = ""
	for _, session := range sessions {
		m.updateDebugSession(session.id, func(state *debugSessionState) {
			if build {
				state.status = "Compiling the changed classes…"
			} else {
				state.status = "Reloading changed classes…"
			}
		})
	}
	m.invalidate()
	go func() {
		success := false
		finish := func() {
			m.debug.reloading = false
			m.debug.hotSwapRequest = 0
			m.debug.hotSwapCancel = nil
			m.debug.hotSwapQuietUntil = time.Now().Add(2 * time.Second)
			if result != nil {
				result <- success
			}
		}
		defer func() { m.post(finish) }()
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Minute)
		defer cancel()
		stage := func(name string) {
			m.post(func() { m.debug.hotSwapStage = name; m.invalidate() })
		}
		done := func(status, title, body string) {
			if status == "warning" || (status == "info" && !strings.Contains(title, "cancelled")) {
				success = true
			}
			m.post(func() {
				m.debug.hotSwapStage = "done: " + title
				m.debug.status = title
				switch status {
				case "error":
					m.notifyError("debug.hotswap", title, body)
					if body != "" {
						m.debug.err = title + "：" + body
					}
				case "warning":
					m.notifyWarning("debug.hotswap", title, body)
				default:
					m.notifyInfo("debug.hotswap", title, body)
				}
				m.invalidate()
			})
		}
		if build {
			// The build reads the files from disk.
			stage("saving")
			if err := m.saveOpenTabs(ctx, tabs); err != nil {
				done("error", "Hot Swap failed", err.Error())
				return
			}
			modules := make([]string, 0)
			seen := map[string]bool{}
			for _, session := range sessions {
				if session.module != "" && !seen[session.module] {
					seen[session.module] = true
					modules = append(modules, session.module)
				}
			}
			if len(modules) == 0 {
				modules = append(modules, root)
			}
			for _, module := range modules {
				request := m.nextRequest()
				requestSet := make(chan struct{})
				m.post(func() {
					m.debug.hotSwapRequest = request
					m.invalidate()
					close(requestSet)
				})
				<-requestSet
				// The reply is the build status itself: "failed" | "succeeded" |
				// "withErrors" | "cancelled" (bindings.ts JavaBuildStatus).
				stage("building " + module)
				raw, err := m.invoke(ctx, "java_build_workspace", map[string]any{
					"module": module, "request": request,
				})
				cancelled := hotSwapCancelled(cancelSignal)
				if err != nil {
					if ctx.Err() != nil || cancelled {
						m.markHotSwapRestartRequired(sessions)
						done("info", "Hot Swap build cancelled", "")
						return
					}
					done("error", "Hot Swap failed", "Compilation failed: fix the errors and reload. ("+err.Error()+")")
					return
				}
				var buildStatus string
				_ = json.Unmarshal(raw, &buildStatus)
				m.post(func() { m.debug.hotSwapBuild = buildStatus })
				if cancelled || buildStatus == "cancelled" {
					m.markHotSwapRestartRequired(sessions)
					done("info", "Hot Swap build cancelled", "")
					return
				}
				if buildStatus == "failed" || buildStatus == "withErrors" || buildStatus == "" {
					done("error", "Hot Swap failed", "Compilation failed: fix the errors and reload. (Compiler state: "+buildStatus+")")
					return
				}
			}
		}
		runtimeSessions := m.hotSwapRuntimeSessions(sessions)
		for _, session := range runtimeSessions {
			if session.needsRestart {
				done("error", "Hot Swap failed",
					"A cancelled build may have left classes out of the reload. Restart the debug session to apply all changes.")
				return
			}
		}
		// Reload every session captured before the build. A tab switch while
		// compiling cannot substitute a newer session.
		stage("redefining")
		changed := map[string]bool{}
		var reloadErrors []string
		obsolete := map[uint32]bool{}
		for _, session := range runtimeSessions {
			rawResult, err := m.invoke(ctx, "debug_redefine_classes", map[string]any{"session": session.session})
			if err != nil {
				reloadErrors = append(reloadErrors, session.name+": "+err.Error())
				continue
			}
			var result openidea.HotSwapResult
			if err := json.Unmarshal(rawResult, &result); err != nil {
				reloadErrors = append(reloadErrors, session.name+": "+err.Error())
				continue
			}
			for _, className := range result.ChangedClasses {
				changed[className] = true
			}
			if result.ErrorMessage != nil && *result.ErrorMessage != "" {
				reloadErrors = append(reloadErrors, *result.ErrorMessage)
				if strings.TrimSpace(*result.ErrorMessage) == "JVM contains obsolete methods" {
					obsolete[session.id] = true
				}
			}
		}
		bottom := false
		for id := range obsolete {
			for _, session := range runtimeSessions {
				if session.id == id && session.paused && session.frames <= 1 {
					bottom = true
					break
				}
			}
		}
		level, title, body := hotSwapMessage(len(changed), reloadErrors, bottom)
		done(level, title, body)
	}()
}

func hotSwapCancelled(signal <-chan struct{}) bool {
	select {
	case <-signal:
		return true
	default:
		return false
	}
}

type hotSwapRuntimeSession struct {
	id           uint32
	session      uint32
	name         string
	paused       bool
	frames       int
	needsRestart bool
}

// hotSwapRuntimeSessions captures mutable session fields on the UI thread.
// The HotSwap worker never reads DebugPane's session map directly.
func (m *Model) hotSwapRuntimeSessions(sessions []hotSwapSession) []hotSwapRuntimeSession {
	result := make(chan []hotSwapRuntimeSession, 1)
	m.post(func() {
		out := make([]hotSwapRuntimeSession, 0, len(sessions))
		for _, session := range sessions {
			state := m.debug.sessions[session.id]
			if state == nil || state.session == 0 || state.ended {
				continue
			}
			out = append(out, hotSwapRuntimeSession{
				id: session.id, session: state.session, name: session.name,
				paused: state.paused, frames: len(state.frames),
				needsRestart: state.hotSwapNeedsRestart,
			})
		}
		result <- out
	})
	return <-result
}

func (m *Model) markHotSwapRestartRequired(sessions []hotSwapSession) {
	m.post(func() {
		for _, session := range sessions {
			m.updateDebugSession(session.id, func(state *debugSessionState) {
				if state.session != 0 && !state.ended && state.caps.HotCodeReplace {
					state.hotSwapNeedsRestart = true
				}
			})
		}
		m.invalidate()
	})
}

// CancelHotSwap cancels the compile Reload Changed Classes is waiting on.
func (m *Model) CancelHotSwap() {
	if !m.debug.reloading || m.debug.hotSwapRequest == 0 {
		return
	}
	cancel := m.debug.hotSwapCancel
	if cancel == nil {
		return
	}
	select {
	case <-cancel:
		m.debug.hotSwapCancel = nil
		return
	default:
	}
	close(cancel)
	m.debug.hotSwapCancel = nil
	request := m.debug.hotSwapRequest
	m.debug.status = "Cancelling Hot Swap build…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		_, _ = m.invoke(ctx, "cancel_request", map[string]any{"id": request})
	}()
}

// offerHotSwapAfterBuild offers the reload when jdtls finished a build on its
// own while the session runs (hotswap.ts's BUILD_COMPLETE prompt, IDEA's
// "Reload classes after compilation: Ask").
func (m *Model) offerHotSwapAfterBuild(session uint32) {
	if m.debug.reloading || time.Now().Before(m.debug.hotSwapQuietUntil) {
		return
	}
	state := m.debugSessionState(session)
	if state == nil || state.session == 0 || state.ended ||
		!state.capsKnown || !state.caps.HotCodeReplace {
		return
	}
	m.notifyInfo("debug.hotswap", "jdtls finished compiling",
		"The changed classes can be reloaded into the debug session.",
		notifyAction{label: "Reload Changed Classes", run: func(m *Model) {
			m.HotSwapReloadAfterBuildForConfigAsync(state.lastID)
		}})
	m.invalidate()
}
