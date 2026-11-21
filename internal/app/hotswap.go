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

// HotSwapReload is debug.reloadChangedClasses: compile the changed classes
// and reload them into the running Java debug session. A failed compilation
// refuses the reload instead of applying stale classes.
func (m *Model) HotSwapReload() {
	if m.debug.session == 0 || m.debug.ended {
		m.debug.err = "There is no debug session to reload classes into"
		m.invalidate()
		return
	}
	if m.debug.reloading {
		return
	}
	m.debug.reloading = true
	m.debug.hotSwapStage = "called"
	m.debug.hotSwapRequest = m.nextRequest()
	m.debug.status = "Compiling the changed classes…"
	m.invalidate()
	request := m.debug.hotSwapRequest
	go func() {
		defer func() { m.post(func() { m.debug.reloading = false }) }()
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Minute)
		defer cancel()
		stage := func(name string) {
			m.post(func() { m.debug.hotSwapStage = name; m.invalidate() })
		}
		done := func(status, title, body string) {
			m.post(func() {
				m.debug.reloading = false
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
		// The build reads the files from disk.
		stage("saving")
		tabs := append([]*Tab(nil), m.tabs...)
		if err := m.saveOpenTabs(ctx, tabs); err != nil {
			done("error", "Hot Swap failed", err.Error())
			return
		}
		// The compile covers the session's working directory (its module):
		// java_build_workspace is the incremental build of the jdtls
		// workspace serving it.
		module := m.Root()
		if m.debug.launchCwd != "" {
			module = m.debug.launchCwd
		}
		// The reply is the build status itself: "failed" | "succeeded" |
		// "withErrors" | "cancelled" (bindings.ts JavaBuildStatus).
		stage("building " + module)
		raw, err := m.invoke(ctx, "java_build_workspace", map[string]any{
			"module": module, "request": request,
		})
		if err != nil {
			if ctx.Err() != nil {
				done("info", "Hot Swap build cancelled", "")
				return
			}
			done("error", "Hot Swap failed", "Compilation failed: fix the errors and reload. ("+err.Error()+")")
			return
		}
		var buildStatus string
		_ = json.Unmarshal(raw, &buildStatus)
		m.post(func() { m.debug.hotSwapBuild = buildStatus })
		if buildStatus == "failed" || buildStatus == "withErrors" || buildStatus == "" {
			done("error", "Hot Swap failed", "Compilation failed: fix the errors and reload. (Compiler state: "+buildStatus+"）")
			return
		}
		// Reload into the session.
		stage("redefining")
		rawResult, err := m.invoke(ctx, "debug_redefine_classes", map[string]any{"session": m.debug.session})
		if err != nil {
			done("error", "Hot Swap failed", err.Error())
			return
		}
		var result openidea.HotSwapResult
		if err := json.Unmarshal(rawResult, &result); err != nil {
			done("error", "Hot Swap failed", err.Error())
			return
		}
		var errorsList []string
		obsolete := false
		if result.ErrorMessage != nil && *result.ErrorMessage != "" {
			errorsList = append(errorsList, *result.ErrorMessage)
			if strings.TrimSpace(*result.ErrorMessage) == "JVM contains obsolete methods" {
				obsolete = true
			}
		}
		level, title, body := hotSwapMessage(len(result.ChangedClasses), errorsList, obsolete)
		done(level, title, body)
	}()
}

// CancelHotSwap cancels the compile Reload Changed Classes is waiting on.
func (m *Model) CancelHotSwap() {
	if !m.debug.reloading || m.debug.hotSwapRequest == 0 {
		return
	}
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
func (m *Model) offerHotSwapAfterBuild() {
	if m.debug.session == 0 || m.debug.ended {
		return
	}
	m.notifyInfo("debug.hotswap", "jdtls finished compiling", "The changed classes can be reloaded into the debug session.")
	m.invalidate()
}
