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
		body := "活动栈帧在返回前继续运行旧代码；新的调用使用重载后的代码。"
		if bottomFrame {
			body += " 最底层的栈帧无法重置：重新进入该方法或重启调试会话以运行新代码。"
		}
		return "warning", "类已重新加载", body
	}
	if changed == 0 {
		return "info", "已加载的类是最新的，没有需要重新加载的内容。", ""
	}
	if changed == 1 {
		return "info", "已重新加载 1 个类", ""
	}
	return "info", fmt.Sprintf("已重新加载 %d 个类", changed), ""
}

// HotSwapReload is debug.reloadChangedClasses: compile the changed classes
// and reload them into the running Java debug session. A failed compilation
// refuses the reload instead of applying stale classes.
func (m *Model) HotSwapReload() {
	if m.debug.session == 0 || m.debug.ended {
		m.debug.err = "没有可以重新加载类的调试会话"
		m.invalidate()
		return
	}
	if m.debug.reloading {
		return
	}
	m.debug.reloading = true
	m.debug.hotSwapStage = "called"
	m.debug.status = "正在编译变更的类…"
	m.invalidate()
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
		if err := saveAll(ctx, tabs, m.prefs.SaveFiles); err != nil {
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
			"module": module, "request": m.nextRequest(),
		})
		if err != nil {
			done("error", "Hot Swap failed", "编译失败：修复错误后重新加载。("+err.Error()+")")
			return
		}
		var buildStatus string
		_ = json.Unmarshal(raw, &buildStatus)
		m.post(func() { m.debug.hotSwapBuild = buildStatus })
		if buildStatus == "failed" || buildStatus == "withErrors" || buildStatus == "" {
			done("error", "Hot Swap failed", "编译失败：修复错误后重新加载。（编译状态："+buildStatus+"）")
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

// offerHotSwapAfterBuild offers the reload when jdtls finished a build on its
// own while the session runs (hotswap.ts's BUILD_COMPLETE prompt, IDEA's
// "Reload classes after compilation: Ask").
func (m *Model) offerHotSwapAfterBuild() {
	if m.debug.session == 0 || m.debug.ended {
		return
	}
	m.notifyInfo("debug.hotswap", "jdtls 已完成编译", "可以重新加载变更的类到调试会话。")
	m.invalidate()
}
