package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"mayoi/internal/openidea"
)

// javaToolNames are the tools Java editing and debugging need: the language
// server and the java-debug bundle that jdtls loads.
var javaToolNames = []string{"jdtls", "java-debug"}

// javaTool is one row of the settings sheet: a tool with its state, and why
// it cannot be installed when that is the case.
type javaTool struct {
	Name       string
	Display    string
	Status     string
	CanInstall bool
	Path       string
	Version    string
	Reason     string
	Homepage   string
}

const toolsLogLimit = 400

// LoadJavaTools reads the backend's tool list so the settings sheet can show
// whether Java language support is installed.
func (m *Model) LoadJavaTools() {
	if m.Root() == "" || m.toolsLoading {
		return
	}
	m.toolsLoading = true
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		list, err := m.mgr.ToolsList(ctx, Window)
		m.post(func() {
			m.toolsLoading = false
			if err != nil {
				m.toolsErr = "读取 Java 工具状态失败：" + err.Error()
				m.invalidate()
				return
			}
			m.javaTools = javaToolRows(list)
			m.toolsLoaded = true
			m.toolsErr = ""
			m.invalidate()
		})
	}()
}

// javaToolRows keeps the Java tools out of the full registry, in the order
// jdtls then java-debug.
func javaToolRows(list []openidea.ToolInfo) []javaTool {
	rows := make([]javaTool, 0, len(javaToolNames))
	for _, want := range javaToolNames {
		for _, t := range list {
			if t.Name != want {
				continue
			}
			row := javaTool{
				Name:       t.Name,
				Display:    t.DisplayName,
				Status:     t.Status,
				CanInstall: t.CanInstall && t.Status == "missing",
			}
			if t.Path != nil {
				row.Path = *t.Path
			}
			if t.Version != nil {
				row.Version = *t.Version
			}
			if t.UnavailableReason != nil {
				row.Reason = *t.UnavailableReason
			}
			if t.Homepage != nil {
				row.Homepage = *t.Homepage
			}
			rows = append(rows, row)
		}
	}
	return rows
}

// javaToolStatus is the sheet's label for a backend ToolStatus.
func javaToolStatus(status string) string {
	switch status {
	case "installed":
		return "已由 GoIDEA 安装"
	case "onPath":
		return "已在 PATH 中"
	case "custom":
		return "已在设置中指定"
	case "missing":
		return "未安装"
	}
	return status
}

// javaToolsMissing reports whether every Java tool the debugger needs is
// installed, and names the missing ones.
func javaToolsMissing(rows []javaTool) []javaTool {
	var missing []javaTool
	for _, r := range rows {
		if r.Status == "missing" {
			missing = append(missing, r)
		}
	}
	return missing
}

// InstallJavaToolchain installs the missing Java tools into the backend's own
// data directory, streaming progress into the settings sheet. Safe to call
// when everything is already present: it then only reloads the list.
func (m *Model) InstallJavaToolchain() {
	if m.toolsBusy != "" {
		return
	}
	if m.Root() == "" {
		m.status = "先打开一个项目，再安装 Java 工具"
		m.invalidate()
		return
	}
	m.toolsErr = ""
	m.toolsLog = nil
	m.toolsProgress = ""
	m.toolsAbort.Store(false)
	m.toolsBusy = "正在检查 Java 工具…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Minute)
		defer cancel()
		list, err := m.mgr.ToolsList(ctx, Window)
		if err != nil {
			m.finishToolInstall("读取工具列表失败：" + err.Error())
			return
		}
		pending := javaToolsMissing(javaToolRows(list))
		if len(pending) == 0 {
			m.finishToolInstall("")
			return
		}
		for _, tool := range pending {
			ch := m.nextChannel()
			var installErr error
			// The Eclipse download is a plain HTTPS transfer and the CDN
			// occasionally drops it mid-stream; one retry turns that into a
			// slow install instead of a failure the user must chase.
			for attempt := 0; attempt < 2; attempt++ {
				req := m.nextRequest()
				first := attempt == 0
				m.post(func() {
					m.toolsChannel = ch
					m.toolsRequest = req
					m.toolsProgress = ""
					if first {
						m.toolsBusy = "正在安装 " + tool.Display
					} else {
						m.toolsBusy = "正在重试 " + tool.Display
						m.toolsLog = append(m.toolsLog, "下载中断，重试…")
					}
					m.invalidate()
				})
				_, installErr = m.mgr.ToolInstall(ctx, Window, tool.Name, req, ch)
				if installErr == nil || m.toolsAbort.Load() || ctx.Err() != nil {
					break
				}
				select {
				case <-time.After(2 * time.Second):
				case <-ctx.Done():
				}
			}
			if installErr != nil {
				if m.toolsAbort.Load() {
					m.finishToolInstall("Java 工具安装已取消")
				} else {
					m.finishToolInstall(fmt.Sprintf("%s 安装失败：%v", tool.Display, installErr))
				}
				return
			}
			m.post(func() {
				m.toolsLog = append(m.toolsLog, "已安装 "+tool.Display)
				m.trimToolsLog()
			})
		}
		reloaded, err := m.mgr.ToolsList(ctx, Window)
		if err != nil {
			m.finishToolInstall("安装完成，但重新读取状态失败：" + err.Error())
			return
		}
		m.post(func() {
			m.javaTools = javaToolRows(reloaded)
			m.toolsLoaded = true
		})
		// jdtls starts with the new server binary and the java-debug bundle;
		// re-attaching documents makes the language services pick it up.
		if err := m.reg.ReopenAll(ctx); err != nil {
			m.finishToolInstall("安装完成，但重新打开文件失败：" + err.Error())
			return
		}
		m.finishToolInstall("")
	}()
}

// finishToolInstall clears the busy state and reports err, if any, in the
// sheet and the status line.
func (m *Model) finishToolInstall(errText string) {
	m.post(func() {
		m.toolsBusy = ""
		m.toolsChannel = 0
		m.toolsRequest = 0
		m.toolsProgress = ""
		if errText != "" {
			m.toolsErr = errText
			m.status = errText
			m.invalidate()
			return
		}
		missing := javaToolsMissing(m.javaTools)
		if len(missing) == 0 {
			m.status = "Java 语言服务与调试器已就绪"
		} else {
			m.status = "Java 工具仍缺少：" + javaToolNamesOf(missing)
		}
		m.invalidate()
	})
}

func javaToolNamesOf(tools []javaTool) string {
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		names = append(names, t.Display)
	}
	return strings.Join(names, "、")
}

// CancelToolInstall stops the running install; the backend discards the
// partial download.
func (m *Model) CancelToolInstall() {
	req := m.toolsRequest
	if m.toolsBusy == "" || req == 0 {
		return
	}
	m.toolsAbort.Store(true)
	m.toolsBusy = "正在取消安装…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
		defer cancel()
		if _, err := m.mgr.Invoke(ctx, Window, "cancel_request", map[string]any{"id": req}); err != nil {
			m.post(func() {
				m.toolsErr = "取消安装失败：" + err.Error()
				m.invalidate()
			})
		}
	}()
}

// acceptToolEvent turns an install event into the sheet's progress line and
// log; it reports whether the channel belongs to a tool install.
func (m *Model) acceptToolEvent(id uint32, data json.RawMessage) bool {
	if id == 0 || id != m.toolsChannel {
		return false
	}
	var ev openidea.InstallEvent
	if json.Unmarshal(data, &ev) != nil {
		return true
	}
	switch ev.Type {
	case "progress":
		if ev.Message != "" {
			m.toolsProgress = ev.Message
		}
	case "line":
		for _, line := range strings.Split(ev.Text, "\n") {
			if line = strings.TrimSpace(line); line != "" {
				m.toolsLog = append(m.toolsLog, line)
			}
		}
		m.trimToolsLog()
	}
	m.invalidate()
	return true
}

func (m *Model) trimToolsLog() {
	if over := len(m.toolsLog) - toolsLogLimit; over > 0 {
		m.toolsLog = append(m.toolsLog[:0], m.toolsLog[over:]...)
	}
}

// SmokeTools lists the Java tools and, with install, installs the missing
// ones: the offscreen check of the toolchain path against the real backend.
func (m *Model) SmokeTools(ctx context.Context, install bool) (map[string]any, error) {
	out := map[string]any{}
	root := m.Root()
	if root == "" {
		return out, errors.New("没有打开项目")
	}
	if _, err := m.mgr.Invoke(ctx, Window, "project_trust_set", map[string]any{
		"path": root, "trusted": true, "folder": true,
	}); err != nil {
		return out, err
	}
	list, err := m.mgr.ToolsList(ctx, Window)
	if err != nil {
		return out, err
	}
	before := map[string]string{}
	for _, t := range list {
		if slices.Contains(javaToolNames, t.Name) {
			before[t.Name] = t.Status
		}
	}
	out["before"] = before
	if !install {
		return out, nil
	}
	installed := []string{}
	for _, row := range javaToolsMissing(javaToolRows(list)) {
		if !row.CanInstall {
			out[row.Name+"Error"] = "不可安装：" + row.Reason
			continue
		}
		ch := m.nextChannel()
		var info openidea.ToolInfo
		var installErr error
		for attempt := 0; attempt < 2; attempt++ {
			info, installErr = m.mgr.ToolInstall(ctx, Window, row.Name, m.nextRequest(), ch)
			if installErr == nil || ctx.Err() != nil {
				break
			}
			select {
			case <-time.After(2 * time.Second):
			case <-ctx.Done():
			}
		}
		if installErr != nil {
			out[row.Name+"Error"] = installErr.Error()
			continue
		}
		installed = append(installed, info.Name)
	}
	out["installed"] = installed
	after, err := m.mgr.ToolsList(ctx, Window)
	if err != nil {
		return out, err
	}
	state := map[string]string{}
	for _, t := range after {
		if slices.Contains(javaToolNames, t.Name) {
			state[t.Name] = t.Status
		}
	}
	out["after"] = state
	for _, name := range javaToolNames {
		if state[name] == "missing" {
			return out, fmt.Errorf("%s 仍未安装", name)
		}
	}
	if err := m.reg.ReopenAll(ctx); err != nil {
		out["reopenError"] = err.Error()
	}
	return out, nil
}
