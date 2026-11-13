package app

// File › Invalidate Caches… after OpenIDEA's dialog: the symbol index and the
// syntax caches are always rebuilt at the next start; the file system cache
// with Local History, the language server workspaces and the build tool caches
// are optional. The choice is scheduled with the backend, which deletes the
// caches at the next start, and the app restarts. File › Restart IDE restarts
// without invalidating anything.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

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

// invalidationSchedule is the choice File › Invalidate Caches… wrote for the
// next start to apply.
type invalidationSchedule struct {
	LocalHistory    bool `json:"localHistory,omitempty"`
	LanguageServers bool `json:"languageServers,omitempty"`
	BuildCaches     bool `json:"buildCaches,omitempty"`
}

// invalidationMarker is the file in the data directory that tells the next
// start what to delete; the caches themselves are in use until then.
func invalidationMarker(dataDir string) string {
	return filepath.Join(dataDir, "invalidate.json")
}

// writeInvalidationSchedule records the choice for the next start. A nil
// options is Just Restart: nothing is deleted, but the marker still names the
// restart.
func writeInvalidationSchedule(dataDir string, opts *InvalidateOptions) error {
	if dataDir == "" {
		return errors.New("没有可用的数据目录")
	}
	plan := invalidationSchedule{}
	if opts != nil {
		plan = invalidationSchedule(*opts)
	}
	raw, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(invalidationMarker(dataDir), raw)
}

// ApplyPendingInvalidation deletes the caches a previous Invalidate Caches…
// scheduled, before the backend starts and takes them into use. The paths are
// the ones the openidea-server owns under its home: Local History, the
// language server workspaces and the build tool caches.
func ApplyPendingInvalidation(cfg Config) {
	dataDir := filepath.Dir(cfg.SettingsPath)
	if cfg.SettingsPath == "" {
		return
	}
	raw, err := os.ReadFile(invalidationMarker(dataDir))
	if err != nil {
		return
	}
	var plan invalidationSchedule
	if err := json.Unmarshal(raw, &plan); err != nil {
		return
	}
	caches := make([]string, 0, 3)
	if plan.LocalHistory {
		caches = append(caches, filepath.Join(cfg.Home, "data", "history"))
	}
	if plan.LanguageServers {
		caches = append(caches, filepath.Join(cfg.Home, "data", "lsp", "jdtls"))
	}
	if plan.BuildCaches {
		caches = append(caches, filepath.Join(cfg.Home, "data", "cache"))
	}
	for _, dir := range caches {
		if err := os.RemoveAll(dir); err != nil && cfg.Log != nil {
			fmt.Fprintf(cfg.Log, "goidea: invalidate %s: %v\n", dir, err)
		}
	}
	_ = os.Remove(invalidationMarker(dataDir))
}

// scheduleInvalidation records the invalidation for the next start and quits,
// which relaunches the app once the quit flow finishes.
func (m *Model) scheduleInvalidation(opts *InvalidateOptions) {
	if err := writeInvalidationSchedule(filepath.Dir(m.cfg.SettingsPath), opts); err != nil {
		if v := m.invalidateDlg; v != nil {
			v.err = "无法安排清除缓存：" + err.Error()
		}
		m.invalidate()
		return
	}
	m.invalidateDlg = nil
	m.restartPending = true
	m.invalidate()
	m.requestQuit(func() {})
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
				ui.Button(c, "清除并重启").OnClick(m.InvalidateAndRestart)
				ui.Button(c, "仅重启").OnClick(m.RestartIDE)
				ui.Button(c, "取消").OnClick(m.closeInvalidateCaches)
			})
		})
	})
	if d.Dismissed() || !open {
		m.closeInvalidateCaches()
	}
}
