package app

// Attach to Process (⌥⇧F5), the port of src/features/debugger/attach.ts and
// java.ts's javaAttachConfig: the backend's attachable process list, the
// filter and the Java (JDWP) group, and the attach request java-debug needs.
// GoIDEA ships the java-debug adapter only, so the popup lists the Java
// group — a kind without an adapter is not offered instead of being attached
// with a placeholder.

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

// attachDlg is the Attach to Process popup while it is open.
type attachDlg struct {
	open  bool
	query string
	sel   int
	list  []openidea.AttachableProcess
	err   string
	busy  bool
}

// OpenAttach opens Attach to Process and loads the process list.
func (m *Model) OpenAttach() {
	m.attach = &attachDlg{open: true}
	m.invalidate()
	m.refreshAttachList()
}

// closeAttach drops the popup.
func (m *Model) closeAttach() {
	m.attach = nil
	m.invalidate()
}

// refreshAttachList asks the backend for the attachable processes.
func (m *Model) refreshAttachList() {
	if m.attach == nil {
		return
	}
	m.attach.busy = true
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "debug_attachable_processes", nil)
		var list []openidea.AttachableProcess
		if err == nil {
			err = json.Unmarshal(raw, &list)
		}
		m.post(func() {
			if m.attach == nil {
				return
			}
			m.attach.busy = false
			if err != nil {
				m.attach.err = err.Error()
			} else {
				m.attach.err = ""
				// Java (JDWP) only: the adapter GoIDEA ships. Listening
				// processes first, by pid.
				var java []openidea.AttachableProcess
				for _, p := range list {
					if p.Kind == "java" {
						java = append(java, p)
					}
				}
				sort.SliceStable(java, func(i, j int) bool {
					pi, pj := java[i].Port, java[j].Port
					if (pi != nil) != (pj != nil) {
						return pi != nil
					}
					return java[i].PID < java[j].PID
				})
				m.attach.list = java
				if m.attach.sel >= len(java) {
					m.attach.sel = 0
				}
			}
			m.invalidate()
		})
	}()
}

// attachFilterRows applies the filter to the Java processes: every word must
// match pid, name or command line (attach.ts matchesFilter).
func (d *attachDlg) filterRows() []openidea.AttachableProcess {
	words := strings.Fields(strings.ToLower(d.query))
	if len(words) == 0 {
		return d.list
	}
	var out []openidea.AttachableProcess
	for _, p := range d.list {
		hay := strings.ToLower(fmt.Sprintf("%d %s %s", p.PID, p.Name, p.CommandLine))
		ok := true
		for _, w := range words {
			if !strings.Contains(hay, w) {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, p)
		}
	}
	return out
}

// AttachToProcess ports java.ts's javaAttachConfig: attach the JDWP process
// through java-debug, naming the imported project so Evaluate resolves.
func (m *Model) AttachToProcess(p openidea.AttachableProcess) {
	root := m.Root()
	m.DockShow("debug")
	if root == "" {
		m.debug.err = "先打开一个项目：源码与被调试进程同属一个工作区"
		m.invalidate()
		return
	}
	if p.Port == nil {
		m.debug.err = fmt.Sprintf("进程 %d 没有 JDWP 调试端口", p.PID)
		m.invalidate()
		return
	}
	if m.debug.starting {
		return
	}
	if m.debug.session != 0 && !m.debug.ended {
		m.debug.err = "已有调试会话在运行：先停止"
		m.invalidate()
		return
	}
	m.attach = nil
	m.debug.starting = true
	m.debug.session = 0
	m.debug.ended = false
	m.debug.paused = false
	m.debug.stopLine = -1
	m.debug.stopFile = ""
	m.debug.frames = nil
	m.debug.scopes = nil
	m.debug.vars = nil
	m.debug.threads = nil
	m.debug.output = nil
	m.debug.truncated = false
	m.debug.err = ""
	m.debug.status = "正在连接 " + fmt.Sprintf("%s (%d)…", p.Name, p.PID)
	m.invalidate()

	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 5*time.Minute)
		defer cancel()
		fail := func(msg string, err error) {
			text := msg
			if err != nil {
				text += "：" + err.Error()
			}
			m.post(func() {
				m.debug.starting = false
				m.debug.err = text
				m.debug.status = ""
				m.invalidate()
			})
		}
		// java.ts javaAttachConfig: the module directory whose jdtls resolves
		// sources is the project root (attach targets carry no module).
		cwd := root
		rawPort, err := m.invoke(ctx, "java_debug_server", map[string]any{"module": ""})
		if err != nil {
			// java-debug missing from jdtls: offer the one-click install.
			if openidea.ErrorKind(err) == "unavailable" && javaDebugMissing.MatchString(err.Error()) {
				m.post(func() {
					m.notifyWarning("debug.java-debug-missing", "Java 调试器未安装",
						"调试 Java 需要 jdtls 内的 Microsoft java-debug。安装并等 jdtls 重启后再调试。",
						notifyAction{label: "安装", run: func(m *Model) { m.InstallJavaToolchain() }})
				})
			}
			fail("启动 java-debug 失败", err)
			return
		}
		var port uint16
		if err := json.Unmarshal(rawPort, &port); err != nil {
			fail("读取 java-debug 端口失败", err)
			return
		}
		project := javaProjectName(root)
		host := "localhost"
		if p.Host != nil && *p.Host != "" {
			host = *p.Host
		}
		extra := map[string]any{
			"debugServer": int(port),
			"hostName":    host,
			"port":        *p.Port,
			"timeout":     30000,
			"projectName": project,
		}
		rawExtra, err := json.Marshal(extra)
		if err != nil {
			fail("构造附加配置失败", err)
			return
		}
		s := string(rawExtra)
		launch := openidea.DebugLaunchConfig{
			Name:      fmt.Sprintf("%s (%d)", p.Name, p.PID),
			Adapter:   "java-debug",
			Request:   "attach",
			Program:   nil,
			Args:      []string{},
			Cwd:       cwd,
			Env:       map[string]string{},
			ExtraJSON: &s,
		}
		rawSession, err := m.mgr.Invoke(ctx, Window, "debug_start", map[string]any{"config": launch})
		if err != nil {
			fail("附加失败", err)
			return
		}
		var session uint32
		if err := json.Unmarshal(rawSession, &session); err != nil {
			fail("读取调试会话失败", err)
			return
		}
		_ = filepath.Clean(cwd)
		m.post(func() {
			if m.Root() != root {
				go func(id uint32) {
					ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
					defer cancel()
					_, _ = m.mgr.Invoke(ctx, Window, "debug_stop", map[string]any{"session": id})
				}(session)
				return
			}
			m.debug.starting = false
			if m.debug.session == 0 {
				m.debug.session = session
			}
			m.debug.status = "已附加：" + fmt.Sprintf("%s (%d)", p.Name, p.PID)
			m.invalidate()
			// A VM launched with suspend=y waits for the debugger: IDEA's
			// attach resumes it once attached. The main thread may be
			// suspended without a stopped event, so the resume is explicit.
			go func(id uint32) {
				rctx, rcancel := context.WithTimeout(m.ctx, 30*time.Second)
				defer rcancel()
				rawThreads, err := m.invoke(rctx, "debug_threads", map[string]any{"session": id})
				if err != nil {
					return
				}
				var threads []openidea.ThreadInfo
				if json.Unmarshal(rawThreads, &threads) != nil || len(threads) == 0 {
					return
				}
				_, _ = m.invoke(rctx, "debug_resume", map[string]any{"session": id, "threadId": threads[0].ID})
			}(session)
		})
	}()
}

// attachDialog draws the Attach to Process popup: the filter, the Java
// (JDWP) group and its listening processes.
func (m *Model) attachDialog(c *ui.Context, p *ui.Theme) {
	d := m.attach
	if d == nil {
		return
	}
	open := d.open
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(560).Gap(8).Children(func() {
			ui.Text(c, "附加到进程").FontSize(13).Bold().TextColor(p.Text).SingleLine()
			in := ui.TextInput(c, &d.query).AutoFocus()
			in.OnShortcut(0, ui.KeyDown, func() { m.stepAttach(1) })
			in.OnShortcut(0, ui.KeyUp, func() { m.stepAttach(-1) })
			rows := d.filterRows()
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				switch {
				case d.busy:
					ui.Text(c, "正在读取进程…").FontSize(10.5).TextColor(p.TextMuted).SingleLine()
				case d.err != "":
					ui.Text(c, d.err).FontSize(10.5).TextColor(p.Danger).SingleLine().Ellipsis("…")
				case len(rows) == 0:
					ui.Text(c, "没有可附加的 Java（JDWP）进程：以 -agentlib:jdwp=transport=dt_socket,server=y 启动的进程会出现在这里").
						FontSize(10.5).TextColor(p.TextMuted)
				default:
					ui.Text(c, fmt.Sprintf("Java (JDWP) · %d 个进程，Enter 附加", len(rows))).
						FontSize(10.5).TextColor(p.TextMuted).SingleLine()
				}
			})
			if len(rows) > 0 {
				ui.Scroll(c).Height(280).Children(func() {
					ui.Column(c).Children(func() {
						for i, proc := range rows {
							i, proc := i, proc
							r := ui.Row(c.Key(fmt.Sprintf("attach:%d", proc.PID))).Height(26).Padding(0, 8).Gap(8).AlignItems(ui.Center)
							if i == d.sel {
								r.Background(p.SurfaceHover)
							} else if r.Hovered() {
								r.Background(p.SurfaceHover)
							}
							r.OnClick(func() {
								d.sel = i
								m.AttachToProcess(rows[i])
							})
							r.Children(func() {
								detail := fmt.Sprintf("pid %d", proc.PID)
								if proc.Port != nil {
									host := "localhost"
									if proc.Host != nil && *proc.Host != "" {
										host = *proc.Host
									}
									detail = fmt.Sprintf("%s:%d", host, *proc.Port)
								}
								ui.Text(c, proc.Name).FontSize(12).TextColor(p.Text).SingleLine().Ellipsis("…")
								ui.Text(c, detail).FontSize(10.5).TextColor(p.TextMuted).SingleLine()
							})
						}
					})
				})
			}
			ui.Row(c).Gap(8).Children(func() {
				attach := ui.Button(c, "附加")
				attach.OnClick(func() { m.attachSelected(rows) })
				ui.Button(c, "刷新").OnClick(m.refreshAttachList)
				ui.Button(c, "取消").OnClick(m.closeAttach)
			})
		})
	})
	if !open {
		m.closeAttach()
	}
}

func (m *Model) stepAttach(delta int) {
	d := m.attach
	if d == nil {
		return
	}
	n := len(d.filterRows())
	if n == 0 {
		return
	}
	d.sel = ((d.sel+delta)%n + n) % n
	m.invalidate()
}

func (m *Model) attachSelected(rows []openidea.AttachableProcess) {
	d := m.attach
	if d == nil || d.sel < 0 || d.sel >= len(rows) {
		return
	}
	m.AttachToProcess(rows[d.sel])
}
