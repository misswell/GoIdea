package app

// File › Invalidate Caches… after OpenIDEA's dialog: the symbol index and the
// syntax caches are always rebuilt at the next start; the file system cache
// with Local History, the language server workspaces and the build tool caches
// are optional. The choice is scheduled with the backend, which deletes the
// caches at the next start, and the app restarts. File › Restart IDE restarts
// without invalidating anything.

import (
	"context"
	"time"

	"github.com/egoist/mygo/ui"
)

// InvalidateOptions are the optional caches the dialog clears; the symbol
// index of every project and the syntax caches are always rebuilt.
type InvalidateOptions struct {
	// LocalHistory clears the file system cache and the Local History of every
	// project.
	LocalHistory bool `json:"localHistory"`
	// LanguageServers clears the language server workspaces, which re-import
	// their projects at the next start.
	LanguageServers bool `json:"languageServers"`
	// BuildCaches clears the build tool sync results and other per-project
	// caches.
	BuildCaches bool `json:"buildCaches"`
}

// invalidatePane is the dialog's state: its optional switches and the error a
// failed schedule shows without leaving the dialog.
type invalidatePane struct {
	open            bool
	localHistory    bool
	languageServers bool
	buildCaches     bool
	busy            bool
	err             string
}

// OpenInvalidateCaches opens File › Invalidate Caches….
func (m *Model) OpenInvalidateCaches() {
	if m.invalidateDlg == nil {
		m.invalidateDlg = &invalidatePane{}
	}
	m.invalidateDlg.open = true
	m.invalidateDlg.err = ""
	m.invalidate()
}

// closeInvalidateCaches drops the dialog without scheduling anything.
func (m *Model) closeInvalidateCaches() {
	m.invalidateDlg = nil
	m.invalidate()
}

// InvalidateAndRestart schedules the staged invalidation and restarts.
func (m *Model) InvalidateAndRestart() {
	v := m.invalidateDlg
	opts := &InvalidateOptions{}
	if v != nil {
		opts = &InvalidateOptions{
			LocalHistory:    v.localHistory,
			LanguageServers: v.languageServers,
			BuildCaches:     v.buildCaches,
		}
	}
	m.scheduleInvalidation(opts)
}

// RestartIDE is File › Restart IDE: it schedules no invalidation and restarts.
func (m *Model) RestartIDE() { m.scheduleInvalidation(nil) }

// invalidateParams is the payload caches_schedule_invalidation takes: the
// options, or nil for a plain restart.
func invalidateParams(opts *InvalidateOptions) map[string]any {
	if opts == nil {
		return map[string]any{"options": nil}
	}
	return map[string]any{"options": *opts}
}

// scheduleInvalidation asks the backend to delete the caches at the next start
// and then quits, which relaunches the app once the quit flow finishes. A
// failed schedule keeps the dialog open, as OpenIDEA reports it instead of
// restarting.
func (m *Model) scheduleInvalidation(opts *InvalidateOptions) {
	v := m.invalidateDlg
	if v != nil {
		if v.busy {
			return
		}
		v.busy = true
		v.err = ""
	}
	m.invalidate()
	params := invalidateParams(opts)
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		_, err := m.mgr.Invoke(ctx, Window, "caches_schedule_invalidation", params)
		m.post(func() {
			if err != nil {
				if v != nil {
					v.busy = false
					v.err = "清除缓存失败：" + err.Error()
				}
				m.invalidate()
				return
			}
			m.invalidateDlg = nil
			m.restartPending = true
			m.invalidate()
			m.requestQuit(func() {})
		})
	}()
}

// invalidateCachesDialog draws the dialog: what is always rebuilt, the
// optional caches, and Invalidate and Restart / Just Restart / Cancel.
func (m *Model) invalidateCachesDialog(c *ui.Context, p *ui.Theme) {
	v := m.invalidateDlg
	if v == nil {
		return
	}
	open := v.open
	d := ui.Modal(c, &open, func() {
		ui.Column(c).Width(480).Gap(10).Children(func() {
			ui.Text(c, "清除缓存").FontSize(13).Bold().TextColor(p.Text).SingleLine()
			ui.Text(c, "下一次启动会重建缓存：每个项目的符号索引和语法缓存。").
				FontSize(11).TextColor(p.TextMuted)
			ui.Text(c, "可选：").FontSize(11).TextColor(p.Text).SingleLine()
			local := v.localHistory
			if ui.Checkbox(c, &local, "清除文件系统缓存和本地历史").Changed() {
				v.localHistory = local
			}
			servers := v.languageServers
			if ui.Checkbox(c, &servers, "清除语言服务工作区（Java 导入数据和索引）").Changed() {
				v.languageServers = servers
			}
			build := v.buildCaches
			if ui.Checkbox(c, &build, "清除构建工具缓存（Maven / Gradle 同步结果）").Changed() {
				v.buildCaches = build
			}
			if v.localHistory {
				ui.Text(c, "所有项目的本地历史都会被删除。").
					FontSize(10.5).TextColor(p.Danger).SingleLine()
			}
			if v.err != "" {
				ui.Text(c, v.err).FontSize(10.5).TextColor(p.Danger).
					SingleLine().Ellipsis("…")
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Button(c, "清除并重启").Disabled(v.busy).OnClick(m.InvalidateAndRestart)
				ui.Button(c, "仅重启").Disabled(v.busy).OnClick(m.RestartIDE)
				ui.Button(c, "取消").Disabled(v.busy).OnClick(m.closeInvalidateCaches)
			})
		})
	})
	if (d.Dismissed() || !open) && !v.busy {
		m.closeInvalidateCaches()
	}
}
