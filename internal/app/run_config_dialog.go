package app

// Run › Edit Configurations… and the "Process is running" rerun confirm,
// ports of OpenIDEA's Run/Debug Configurations dialog (ConfigForm.tsx) and
// confirmRerun (dialogs.tsx): the same fields, validation, override semantics
// (editing a detected config produces a user copy with its id) and save flow
// (run_save_configs with the removed detected ids).

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/adapter"
	"mayoi/internal/openidea"
)

// runConfigDlg is Run › Edit Configurations… while it is open.
type runConfigDlg struct {
	open bool
	// items are the working copies of the project's configurations, in the
	// merged order the backend sent.
	items []*openidea.RunConfig
	sel   int
	// removedDetected are the ids of detected configurations the user deleted;
	// run_save_configs stops the backend from regenerating them.
	removedDetected []string

	// The form's fields, as the editor shows them.
	name   string
	args   string
	cwd    string
	env    string
	module string
	policy string
	// selLabel and policyLabel are the Select bindings; both write back on
	// Changed().
	selLabel    string
	policyLabel string

	errs    adapter.ConfigErrors
	saveErr string
}

// rerunAsk is the "Process 'x' Is Running" confirm of a rerun.
type rerunAsk struct {
	open    bool
	name    string
	dontAsk bool
	raw     json.RawMessage
}

// OpenRunConfigs opens Run › Edit Configurations… with the project's current
// configurations.
func (m *Model) OpenRunConfigs() {
	items := make([]*openidea.RunConfig, len(m.run.configs))
	for i := range m.run.configs {
		items[i] = &m.run.configs[i]
	}
	d := &runConfigDlg{
		items: items,
		sel:   m.run.sel,
	}
	if d.sel < 0 || d.sel >= len(d.items) {
		d.sel = 0
	}
	if d.sel < len(d.items) {
		d.selLabel = adapter.KindLabel(d.items[d.sel].Kind) + " · " + d.items[d.sel].Name
	}
	m.runCfgDlg = d
	d.loadForm()
	m.invalidate()
}

// closeRunConfigs drops the dialog without saving.
func (m *Model) closeRunConfigs() {
	m.runCfgDlg = nil
	m.invalidate()
}

// selected returns the configuration the form edits.
func (d *runConfigDlg) selected() *openidea.RunConfig {
	if d.sel < 0 || d.sel >= len(d.items) {
		return nil
	}
	return d.items[d.sel]
}

// loadForm fills the fields from the selected configuration.
func (d *runConfigDlg) loadForm() {
	cfg := d.selected()
	if cfg == nil {
		d.name, d.args, d.cwd, d.env, d.module, d.policy = "", "", "", "", "", "automatic"
		d.selLabel, d.policyLabel = "", ""
		return
	}
	d.name = cfg.Name
	d.args = adapter.FormatArgs(cfg.Args)
	d.cwd = cfg.CwdOr("")
	d.env = adapter.FormatEnv(cfg.Env)
	d.module = ""
	if cfg.Module != nil {
		d.module = *cfg.Module
	}
	d.policy = "automatic"
	if cfg.BuildPolicy != nil && *cfg.BuildPolicy != "" {
		d.policy = *cfg.BuildPolicy
	}
	d.policyLabel = policyLabelOf(d.policy, cfg.Kind)
	d.selLabel = adapter.KindLabel(cfg.Kind) + " · " + cfg.Name
	if cfg.Detected {
		d.selLabel += "（检测）"
	}
	d.errs = nil
	d.saveErr = ""
}

// policyLabelOf is the label of a configuration's build policy as the Build
// select shows it.
func policyLabelOf(policy, kind string) string {
	for _, o := range adapter.BuildPolicyOptions(kind) {
		if o.Value == policy {
			return o.Label
		}
	}
	return adapter.BuildPolicyOptions(kind)[0].Label
}

// applyForm writes the fields into the selected configuration, as an edit of
// it: a detected config becomes a user copy with its id.
func (d *runConfigDlg) applyForm() {
	cfg := d.selected()
	if cfg == nil {
		return
	}
	edited := *cfg
	edited.Name = d.name
	edited.Args = adapter.ParseArgs(d.args)
	if c := strings.TrimSpace(d.cwd); c == "" {
		edited.Cwd = nil
	} else {
		edited.Cwd = &c
	}
	edited.Env = adapter.ParseEnv(d.env)
	if mod := strings.TrimSpace(d.module); mod == "" {
		edited.Module = nil
	} else {
		edited.Module = &mod
	}
	if adapter.BuildPolicyApplies(edited.Kind) && d.policy != "" && d.policy != "automatic" {
		edited.BuildPolicy = &d.policy
	} else {
		edited.BuildPolicy = nil
	}
	d.items = adapter.UpsertConfig(d.items, &edited)
	for i, c := range d.items {
		if c.ID == edited.ID {
			d.sel = i
			break
		}
	}
}

// AddRunConfig creates an empty Shell configuration, as upstream newConfig.
func (m *Model) AddRunConfig() {
	d := m.runCfgDlg
	if d == nil {
		return
	}
	d.applyForm()
	cfg, err := adapter.NewConfig(d.items)
	if err != nil {
		d.saveErr = err.Error()
		m.invalidate()
		return
	}
	d.items = append(d.items, cfg)
	d.sel = len(d.items) - 1
	d.loadForm()
	m.invalidate()
}

// DuplicateRunConfig copies the selected configuration with a fresh id.
func (m *Model) DuplicateRunConfig() {
	d := m.runCfgDlg
	if d == nil || d.selected() == nil {
		return
	}
	d.applyForm()
	cfg, err := adapter.DuplicateConfig(d.selected(), d.items)
	if err != nil {
		d.saveErr = err.Error()
		m.invalidate()
		return
	}
	d.items = append(d.items, cfg)
	d.sel = len(d.items) - 1
	d.loadForm()
	m.invalidate()
}

// RemoveRunConfig deletes the selected configuration; a detected one is
// remembered so the backend stops regenerating it.
func (m *Model) RemoveRunConfig() {
	d := m.runCfgDlg
	if d == nil || d.selected() == nil {
		return
	}
	cfg := d.selected()
	if cfg.Detected {
		d.removedDetected = append(d.removedDetected, cfg.ID)
	}
	id := cfg.ID
	d.items = adapter.RemoveConfig(d.items, id)
	if d.sel >= len(d.items) {
		d.sel = len(d.items) - 1
	}
	d.loadForm()
	m.invalidate()
}

// SaveRunConfigs validates the edited configuration, persists the user
// configurations and reloads the merged list from the backend.
func (m *Model) SaveRunConfigs() {
	d := m.runCfgDlg
	if d == nil {
		return
	}
	d.applyForm()
	if cfg := d.selected(); cfg != nil {
		if errs := adapter.ValidateConfig(cfg, d.items); errs.HasErrors() {
			d.errs = errs
			m.invalidate()
			return
		}
	}
	d.errs = nil
	d.saveErr = ""
	user := adapter.UserConfigs(d.items)
	removed := d.removedDetected
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		err := adapter.SaveUserConfigs(ctx, javaBackend{m}, user, removed)
		m.post(func() {
			if m.runCfgDlg != d {
				return
			}
			if err != nil {
				d.saveErr = err.Error()
				m.invalidate()
				return
			}
			m.closeRunConfigs()
			m.loadRunConfigs()
		})
	}()
	m.invalidate()
}

// runConfigsDialog draws Run › Edit Configurations….
func (m *Model) runConfigsDialog(c *ui.Context, p *ui.Theme) {
	d := m.runCfgDlg
	if d == nil {
		return
	}
	open := d.open
	modal := ui.Modal(c, &open, func() {
		ui.Column(c).Width(640).Gap(8).Children(func() {
			ui.Text(c, "运行/调试配置").FontSize(13).Bold().TextColor(p.Text).SingleLine()
			if len(d.items) == 0 {
				ui.Text(c, "项目还没有运行配置。").FontSize(11).TextColor(p.TextMuted).SingleLine()
			}
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				list := make([]string, 0, len(d.items))
				for _, cfg := range d.items {
					label := adapter.KindLabel(cfg.Kind) + " · " + cfg.Name
					if cfg.Detected {
						label += "（检测）"
					}
					list = append(list, label)
				}
				if sel := ui.Select(c, &d.selLabel, list); sel.Changed() {
					d.applyForm()
					for i, cfg := range d.items {
						label := adapter.KindLabel(cfg.Kind) + " · " + cfg.Name
						if cfg.Detected {
							label += "（检测）"
						}
						if label == d.selLabel {
							d.sel = i
							break
						}
					}
					d.loadForm()
				}
				ui.Button(c, "新建").OnClick(m.AddRunConfig)
				ui.Button(c, "复制").OnClick(m.DuplicateRunConfig)
				ui.Button(c, "删除").OnClick(m.RemoveRunConfig)
			})
			if cfg := d.selected(); cfg != nil {
				ui.Column(c).Gap(6).Children(func() {
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Text(c, "名称").FontSize(11).TextColor(p.TextMuted).Width(96).SingleLine()
						ui.TextInput(c, &d.name).Width(300)
						ui.Text(c, adapter.KindLabel(cfg.Kind)).FontSize(11).TextColor(p.TextMuted).SingleLine()
					})
					if e := d.errs["name"]; e != "" {
						ui.Text(c, e).FontSize(10.5).TextColor(p.Danger).SingleLine()
					}
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Text(c, "命令").FontSize(11).TextColor(p.TextMuted).Width(96).SingleLine()
						ui.TextInput(c, &cfg.Command).Width(430)
					})
					if e := d.errs["command"]; e != "" {
						ui.Text(c, e).FontSize(10.5).TextColor(p.Danger).SingleLine()
					}
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Text(c, "程序实参").FontSize(11).TextColor(p.TextMuted).Width(96).SingleLine()
						ui.TextInput(c, &d.args).Width(430)
					})
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Text(c, "工作目录").FontSize(11).TextColor(p.TextMuted).Width(96).SingleLine()
						ui.Text(c, m.Root()+"/").FontSize(10.5).TextColor(p.TextMuted).SingleLine()
						ui.TextInput(c, &d.cwd).Width(240)
					})
					if e := d.errs["cwd"]; e != "" {
						ui.Text(c, e).FontSize(10.5).TextColor(p.Danger).SingleLine()
					}
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Text(c, "环境变量").FontSize(11).TextColor(p.TextMuted).Width(96).SingleLine()
						ui.TextInput(c, &d.env).Width(430)
					})
					if e := d.errs["env"]; e != "" {
						ui.Text(c, e).FontSize(10.5).TextColor(p.Danger).SingleLine()
					}
					if adapter.IsJVMApplication(cfg.Kind) {
						ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
							ui.Text(c, "模块").FontSize(11).TextColor(p.TextMuted).Width(96).SingleLine()
							ui.TextInput(c, &d.module).Width(300)
							ui.Text(c, "留空则在保存时自动识别").FontSize(10).TextColor(p.TextMuted).SingleLine()
						})
					}
					if adapter.BuildPolicyApplies(cfg.Kind) {
						opts := adapter.BuildPolicyOptions(cfg.Kind)
						labels := make([]string, 0, len(opts))
						for _, o := range opts {
							labels = append(labels, o.Label)
						}
						ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
							ui.Text(c, "启动前构建").FontSize(11).TextColor(p.TextMuted).Width(96).SingleLine()
							if sel := ui.Select(c, &d.policyLabel, labels); sel.Changed() {
								for _, o := range opts {
									if o.Label == d.policyLabel {
										d.policy = o.Value
									}
								}
							}
						})
					}
				})
			}
			if d.saveErr != "" {
				ui.Text(c, d.saveErr).FontSize(10.5).TextColor(p.Danger).SingleLine()
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Button(c, "保存").OnClick(m.SaveRunConfigs)
				ui.Button(c, "取消").OnClick(m.closeRunConfigs)
			})
		})
	})
	if modal.Dismissed() || !open {
		m.closeRunConfigs()
	}
}

// rerunTarget is what a rerun launches: the configuration the pane started,
// as saved now (upstream rerunConfig), or the one it ran with when it was
// deleted or never saved (a Run Anything line).
func (m *Model) rerunTarget() (json.RawMessage, string) {
	if id := m.run.lastID; id != "" {
		for i := range m.run.configs {
			if m.run.configs[i].ID == id {
				return m.run.configs[i].Raw(), m.run.configs[i].Name
			}
		}
	}
	if m.run.lastRaw != nil {
		return m.run.lastRaw, m.run.lastName
	}
	return nil, ""
}

// Rerun starts the current configuration again, as OpenIDEA's run.rerun does:
// the configuration as saved now, and — unless it allows parallel runs — a
// confirm before its running instance is stopped and waited for.
func (m *Model) Rerun() {
	if m.run.busy || m.run.loading {
		return
	}
	raw, name := m.rerunTarget()
	if raw == nil {
		return
	}
	live := m.run.running || m.run.awaiting
	if !live {
		m.startRunWith(raw, name)
		return
	}
	if m.confirmRerunEnabled() {
		m.rerunAsk = &rerunAsk{open: true, name: name, raw: append(json.RawMessage(nil), raw...)}
		m.invalidate()
		return
	}
	m.stopAndRerun(raw, name)
}

// stopAndRerun stops the running process, waits for its exit, and starts the
// configuration again; a still-busy session gets a second stop, which kills.
func (m *Model) stopAndRerun(raw json.RawMessage, name string) {
	m.StopRun()
	go func() {
		deadline := time.Now().Add(30 * time.Second)
		for {
			var exit *int32
			m.post(func() { exit = m.run.exitCode })
			if exit != nil {
				break
			}
			if time.Now().After(deadline) {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		m.post(func() {
			if m.run.busy || m.run.running || m.run.awaiting {
				// The first stop did not finish in time: the second one kills.
				m.StopRun()
			}
			m.startRunWith(raw, name)
		})
	}()
}

// ConfirmRerun answers the "Process is running" dialog with Stop and Rerun.
func (m *Model) ConfirmRerun() {
	ask := m.rerunAsk
	if ask == nil {
		return
	}
	if ask.dontAsk {
		off := false
		if m.prefs.Run == nil {
			m.prefs.Run = &RunPrefs{}
		}
		m.prefs.Run.ConfirmRerun = &off
		m.savePreferences()
	}
	raw, name := ask.raw, ask.name
	m.rerunAsk = nil
	m.invalidate()
	m.stopAndRerun(raw, name)
}

// CancelRerun answers the confirm with Cancel: nothing is stopped.
func (m *Model) CancelRerun() {
	m.rerunAsk = nil
	m.invalidate()
}

// rerunConfirmDialog draws the "Process 'x' Is Running" confirm.
func (m *Model) rerunConfirmDialog(c *ui.Context, p *ui.Theme) {
	ask := m.rerunAsk
	if ask == nil {
		return
	}
	open := ask.open
	modal := ui.Modal(c, &open, func() {
		ui.Column(c).Width(440).Gap(10).Children(func() {
			ui.Text(c, "进程 '"+ask.name+"' 正在运行").FontSize(13).Bold().TextColor(p.Text).SingleLine()
			ui.Text(c, "'"+ask.name+"' 不允许并行运行。\n要停止正在运行的进程并重新运行吗？").
				FontSize(11).TextColor(p.Text)
			dontAsk := ask.dontAsk
			if ui.Checkbox(c, &dontAsk, "不再询问").Changed() {
				ask.dontAsk = dontAsk
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Button(c, "停止并重新运行").OnClick(m.ConfirmRerun)
				ui.Button(c, "取消").OnClick(m.CancelRerun)
			})
		})
	})
	if modal.Dismissed() || !open {
		m.CancelRerun()
	}
}
