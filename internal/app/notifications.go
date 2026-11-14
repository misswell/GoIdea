package app

// Notifications after OpenIDEA's workbench: a bottom-right balloon stack, a
// Notifications tool window on the right stripe that holds the history, and a
// notification store whose levels, keys and sticky rules follow
// notificationStore.ts. Balloons fade after 10 s of hover-free time; errors
// stay until dismissed (5 minutes when they are listed).

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

const (
	notifyInfoTimeout  = 10 * time.Second
	notifyStickyTime   = 5 * time.Minute
	notifyMaxBalloons  = 3
	notifyMaxHistory   = 200
	notifyBalloonWidth = 376
)

// notifyAction is one button on a notification.
type notifyAction struct {
	label string
	run   func(m *Model)
}

// ideNotification is one entry of the notification center.
type ideNotification struct {
	id      int
	key     string
	level   string // "info", "warning" or "error"
	title   string
	body    string
	actions []notifyAction
	at      time.Time
	read    bool
	// group is Settings › Notifications' group, logged whether it is listed
	// in the tool window, and userInitiated the answer to a command the user
	// ran (it pops up even with balloons off).
	group         string
	logged        bool
	userInitiated bool
	// balloon is whether it still shows in the corner; sticky is whether it
	// may fade on its own at all.
	balloon bool
	sticky  bool
	hover   bool

	timer     *time.Timer
	deadline  time.Time
	remaining time.Duration
}

// notifier is the notification center's state.
type notifier struct {
	items []ideNotification
	seq   int
}

// notifyIcon is OpenIDEA's bell for the stripe and the empty state.
var notifyIcon = welcomeIcon(`<path d="M18 8a6 6 0 0 0-12 0c0 7-3 9-3 9h18s-3-2-3-9"/><path d="M13.7 21a2 2 0 0 1-3.4 0"/>`)

// notifyInfo, notifyWarning and notifyError post a notification. A key
// replaces the earlier notification with the same key, as OpenIDEA's
// NotificationService does; errors are sticky.
func (m *Model) notifyInfo(key, title, body string, actions ...notifyAction) {
	m.postNotification(ideNotification{key: key, level: "info", title: title, body: body, actions: actions})
}

func (m *Model) notifyWarning(key, title, body string, actions ...notifyAction) {
	m.postNotification(ideNotification{key: key, level: "warning", title: title, body: body, actions: actions})
}

func (m *Model) notifyError(key, title, body string, actions ...notifyAction) {
	m.postNotification(ideNotification{key: key, level: "error", title: title, body: body, actions: actions})
}

// postNotification adds an entry and starts its balloon's clock.
func (m *Model) postNotification(n ideNotification) {
	n.id = m.notify.seq + 1
	m.notify.seq = n.id
	n.at = time.Now()
	n.group = notifyGroupOf(n)
	prefs := m.notifyPrefsFor(n.group)
	n.balloon = n.userInitiated || (!m.notifyOff() && prefs.Display != "none")
	n.sticky = prefs.Display == "sticky" || n.level == "error"
	n.logged = !prefs.NoLog
	// Neither popped nor listed: dropped, as IDEA's "No popup" and no log.
	if !n.balloon && !n.logged {
		m.invalidate()
		return
	}
	if n.key != "" {
		kept := m.notify.items[:0]
		for _, old := range m.notify.items {
			if old.key != n.key {
				kept = append(kept, old)
			} else {
				stopNotifyTimer(&old)
			}
		}
		m.notify.items = kept
	}
	m.notify.items = append([]ideNotification{n}, m.notify.items...)
	if len(m.notify.items) > notifyMaxHistory {
		for _, old := range m.notify.items[notifyMaxHistory:] {
			stopNotifyTimer(&old)
		}
		m.notify.items = m.notify.items[:notifyMaxHistory]
	}
	m.startNotifyTimer(n.id)
	m.invalidate()
}

// startNotifyTimer arms the balloon's fade-out: 10 s for info and warnings,
// 5 minutes for a listed sticky error, and never for an error the tool window
// would not list (the store lists everything, so sticky errors fade at 5 min).
func (m *Model) startNotifyTimer(id int) {
	for i := range m.notify.items {
		it := &m.notify.items[i]
		if it.id != id {
			continue
		}
		wait := notifyInfoTimeout
		if it.sticky {
			wait = notifyStickyTime
		}
		it.remaining = wait
		it.deadline = time.Now().Add(wait)
		it.timer = time.AfterFunc(wait, func() {
			m.post(func() { m.dismissBalloon(id) })
		})
		return
	}
}

// stopNotifyTimer drops a timer whose entry is gone.
func stopNotifyTimer(n *ideNotification) {
	if n.timer != nil {
		n.timer.Stop()
		n.timer = nil
	}
}

// DismissBalloon hides one balloon; the notification stays in the history.
func (m *Model) dismissBalloon(id int) {
	for i := range m.notify.items {
		it := &m.notify.items[i]
		if it.id != id {
			continue
		}
		if it.hover {
			return
		}
		stopNotifyTimer(it)
		it.balloon = false
		m.invalidate()
		return
	}
}

// SetBalloonHover pauses a balloon's clock while the pointer rests on it, as
// IDEA's smart fade-out does.
func (m *Model) SetBalloonHover(id int, hover bool) {
	for i := range m.notify.items {
		it := &m.notify.items[i]
		if it.id != id || it.hover == hover {
			continue
		}
		it.hover = hover
		if hover {
			if it.timer != nil {
				it.timer.Stop()
				it.timer = nil
				it.remaining = max(time.Duration(1), it.deadline.Sub(time.Now()))
			}
			continue
		}
		if it.balloon {
			wait := it.remaining
			if wait <= 0 {
				wait = notifyInfoTimeout
			}
			it.deadline = time.Now().Add(wait)
			it.timer = time.AfterFunc(wait, func() {
				m.post(func() { m.dismissBalloon(id) })
			})
		}
		return
	}
}

// unreadNotifications is the stripe's badge count.
func (m *Model) unreadNotifications() int {
	n := 0
	for _, it := range m.notify.items {
		if !it.read && it.logged {
			n++
		}
	}
	return n
}

// markNotificationsRead is the tool window opening: everything in the list
// counts as seen.
func (m *Model) markNotificationsRead() {
	for i := range m.notify.items {
		m.notify.items[i].read = true
	}
}

// ShowNotifications opens the Notifications tool window on the right.
func (m *Model) ShowNotifications() {
	m.rightTool = "notifications"
	m.rightHidden = false
	m.markNotificationsRead()
	m.rememberLayout()
	m.invalidate()
}

// CloseNotifications hides the right area without forgetting its window.
func (m *Model) CloseNotifications() {
	if m.rightTool != "notifications" {
		return
	}
	m.rightHidden = true
	m.rememberLayout()
	m.invalidate()
}

// ToggleNotificationsWindow is the stripe button's action.
func (m *Model) ToggleNotificationsWindow() {
	if m.rightTool == "notifications" && !m.rightHidden {
		m.CloseNotifications()
		return
	}
	m.ShowNotifications()
}

// ClearNotifications empties the history, as the tool window's trash does.
func (m *Model) ClearNotifications() {
	for i := range m.notify.items {
		stopNotifyTimer(&m.notify.items[i])
	}
	m.notify.items = nil
	m.invalidate()
}

// notificationsShown reports whether the right area currently shows the list.
func (m *Model) notificationsShown() bool {
	return m.rightTool == "notifications" && !m.rightHidden
}

// balloonItems is the corner stack: the newest three notifications still
// showing as balloons, newest at the bottom, as Balloons() draws them.
func (m *Model) balloonItems() []ideNotification {
	var newest []ideNotification
	for _, it := range m.notify.items {
		if !it.balloon {
			continue
		}
		newest = append(newest, it)
		if len(newest) == notifyMaxBalloons {
			break
		}
	}
	// Newest at the bottom of the stack, as Balloons() reverses its slice.
	out := make([]ideNotification, len(newest))
	for i, it := range newest {
		out[len(newest)-1-i] = it
	}
	return out
}

// notifyLevelColor is the level's accent.
func notifyLevelColor(p *ui.Theme, level string) ui.Color {
	switch level {
	case "error":
		return p.Danger
	case "warning":
		return p.Warning
	}
	return p.Accent
}

// balloons draws the bottom-right stack above the rest of the window.
func (m *Model) balloons(c *ui.Context, p *ui.Theme) {
	items := m.balloonItems()
	if len(items) == 0 {
		return
	}
	w, h := c.Size()
	total := float32(0)
	for _, it := range items {
		total += notifyHeight(it) + 8
	}
	x := w - notifyBalloonWidth - 16
	y := h - oiStatusBarHeight - 12 - total
	ui.Overlay(c, func() {
		ui.Column(c).Absolute().Left(x).Top(y).Width(notifyBalloonWidth).Gap(8).
			Children(func() {
				for _, it := range items {
					m.balloonView(c, p, it)
				}
			})
	})
}

// notifyHeight is a balloon's height from its content.
func notifyHeight(n ideNotification) float32 {
	h := float32(50)
	if n.body != "" {
		h += 16
	}
	if len(n.actions) > 0 {
		h += 24
	}
	return h
}

// balloonView draws one balloon, as OpenIDEA's Toast: accent bar, title, body,
// actions, and a close button on the corner.
func (m *Model) balloonView(c *ui.Context, p *ui.Theme, n ideNotification) {
	r := ui.Row(c.Key(fmt.Sprintf("balloon:%d", n.id))).Height(notifyHeight(n)).
		Radius(10).Background(p.Surface).BorderWidth(1).BorderColor(p.Border).
		Shadow(0, 8, 24, 0, ui.RGBA(0, 0, 0, 0.35))
	if r.Hovered() != n.hover {
		m.SetBalloonHover(n.id, r.Hovered())
	}
	r.Children(func() {
		ui.Row(c).Width(3).Height(notifyHeight(n)).Radius(10).
			Background(notifyLevelColor(p, n.level))
		ui.Column(c).Grow(1).Padding(10, 12).Gap(3).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, n.title).FontSize(12.5).Bold().TextColor(p.Text).SingleLine()
				ui.Spacer(c)
				close := ui.ButtonBase(c.Key(fmt.Sprintf("balloon:close:%d", n.id))).
					Size(18, 18).Radius(4).Justify(ui.Center).AlignItems(ui.Center).
					Cursor(ui.CursorPointer)
				if close.Hovered() {
					close.Background(p.SurfaceHover)
				}
				close.Children(func() {
					ui.Text(c, "×").FontSize(13).TextColor(p.TextMuted).SingleLine()
				})
				close.OnClick(func() { m.dismissBalloon(n.id) })
			})
			if n.body != "" {
				ui.Text(c, n.body).FontSize(11.5).TextColor(p.TextMuted).
					SingleLine().Ellipsis("…")
			}
			actions := n.actions
			if m.canSuppress(n) {
				actions = append(append([]notifyAction(nil), actions...),
					notifyAction{label: "Don't Show Again", run: func(m *Model) { m.SuppressGroup(n.group) }})
			}
			if len(actions) > 0 {
				ui.Row(c).Gap(6).Children(func() {
					for i, a := range actions {
						a := a
						b := ui.ButtonBase(c.Key(fmt.Sprintf("balloon:act:%d:%d", n.id, i))).
							Height(20).Padding(0, 6).Radius(4).AlignItems(ui.Center).
							Cursor(ui.CursorPointer)
						if b.Hovered() {
							b.Background(p.SurfaceHover)
						}
						b.Children(func() {
							ui.Text(c, a.label).FontSize(11.5).TextColor(p.Accent).SingleLine()
						})
						b.OnClick(func() {
							m.dismissBalloon(n.id)
							a.run(m)
						})
					}
				})
			}
		})
	})
}

// notificationsView is the Notifications tool window: history newest first,
// each entry a balloon-like card with its time.
func (m *Model) notificationsView(c *ui.Context, p *ui.Theme, oi oiColors) {
	ui.Column(c).Key("notifications:window").Grow(1).Children(func() {
		ui.Row(c).Height(oiHeaderHeight).Padding(12, 10).Gap(8).AlignItems(ui.Center).
			Children(func() {
				ui.Text(c, "Notifications").FontSize(12.5).Bold().TextColor(p.Text).SingleLine()
				ui.Spacer(c)
				clear := ui.ButtonBase(c.Key("notifications:clear")).Size(22, 22).
					Radius(4).Justify(ui.Center).AlignItems(ui.Center).Cursor(ui.CursorPointer)
				if clear.Hovered() {
					clear.Background(p.SurfaceHover)
				}
				clear.Tooltip("Clear All")
				clear.Children(func() {
					ui.Text(c, "⌫").FontSize(12).TextColor(p.TextMuted).SingleLine()
				})
				clear.OnClick(func() { m.ClearNotifications() })
			})
		logged := m.loggedNotifications()
		if len(logged) == 0 {
			ui.Column(c).Grow(1).Justify(ui.Center).AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Icon(c, notifyIcon).Size(24, 24).TextColor(p.TextMuted)
				ui.Text(c, "No notifications").FontSize(11.5).TextColor(p.TextMuted).SingleLine()
			})
			return
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(8, 8).Gap(6).Children(func() {
				for _, it := range logged {
					it := it
					card := ui.Column(c.Key(fmt.Sprintf("notify:%d", it.id))).Padding(8, 10).
						Gap(3).Radius(8).Background(p.Surface).
						BorderWidth(1).BorderColor(notifyLevelColor(p, it.level))
					card.Children(func() {
						ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
							ui.Text(c, it.title).FontSize(12).Bold().TextColor(p.Text).SingleLine()
							ui.Spacer(c)
							ui.Text(c, it.at.Format("15:04")).FontSize(10.5).TextColor(p.TextMuted).SingleLine()
							x := ui.ButtonBase(c.Key(fmt.Sprintf("notify:close:%d", it.id))).
								Size(18, 18).Radius(4).Justify(ui.Center).AlignItems(ui.Center).
								Cursor(ui.CursorPointer)
							if x.Hovered() {
								x.Background(p.SurfaceHover)
							}
							x.Children(func() {
								ui.Text(c, "×").FontSize(12).TextColor(p.TextMuted).SingleLine()
							})
							x.OnClick(func() { m.removeNotification(it.id) })
						})
						if it.body != "" {
							ui.Text(c, it.body).FontSize(11).TextColor(p.TextMuted).SingleLine().Ellipsis("…")
						}
						if len(it.actions) > 0 {
							ui.Row(c).Gap(6).Children(func() {
								for i, a := range it.actions {
									a := a
									b := ui.ButtonBase(c.Key(fmt.Sprintf("notify:act:%d:%d", it.id, i))).
										Height(20).Padding(0, 6).Radius(4).AlignItems(ui.Center).
										Cursor(ui.CursorPointer)
									if b.Hovered() {
										b.Background(p.SurfaceHover)
									}
									b.Children(func() {
										ui.Text(c, a.label).FontSize(11).TextColor(p.Accent).SingleLine()
									})
									b.OnClick(func() { a.run(m) })
								}
							})
						}
					})
				}
			})
		})
	})
}

// removeNotification drops one entry from the history.
func (m *Model) removeNotification(id int) {
	kept := m.notify.items[:0]
	for _, it := range m.notify.items {
		if it.id == id {
			stopNotifyTimer(&it)
			continue
		}
		kept = append(kept, it)
	}
	m.notify.items = kept
	m.invalidate()
}

// notifyGroupSample seeds the visual baseline with one of each level.
func (m *Model) notifyGroupSample() {
	m.notifyInfo("shot.info", "Build finished", "hello:package in 1.2 s")
	m.notifyWarning("shot.warn", "Unused import", "App.java imports java.util.List")
	m.notifyError("shot.error", "Build failed", "2 errors, 1 warning")
}

// notifySuffix trims an error message for a balloon's one-line body.
func notifySuffix(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if len(s) > 120 {
		s = s[:117] + "…"
	}
	return s
}

// SmokeNotifications drives the notification center and reports what it did,
// for the base e2e.
func (m *Model) SmokeNotifications() map[string]any {
	out := map[string]any{}
	m.ClearNotifications()
	m.notifyInfo("smoke:info", "Build finished", "one")
	m.notifyWarning("smoke:warn", "Unused import", "two")
	m.notifyError("smoke:error", "Build failed", "three",
		notifyAction{label: "Show Build Output", run: func(m *Model) {}})
	out["count"] = len(m.notify.items)
	out["unread"] = m.unreadNotifications()
	out["balloons"] = len(m.balloonItems())
	sticky := 0
	for _, it := range m.notify.items {
		if it.sticky {
			sticky++
		}
	}
	out["sticky"] = sticky
	m.notifyInfo("smoke:info", "Build finished again", "replaced")
	out["afterReplace"] = len(m.notify.items)
	m.ShowNotifications()
	out["window"] = m.notificationsShown() && m.rightTool == "notifications"
	out["unreadAfterOpen"] = m.unreadNotifications()
	m.CloseNotifications()
	m.ClearNotifications()
	out["cleared"] = len(m.notify.items)
	return out
}

// notifyGroupOf maps a notification to Settings' group, as groupOf does: the
// key's prefix picks the group unless it was set.
func notifyGroupOf(n ideNotification) string {
	if n.group != "" {
		return n.group
	}
	switch {
	case strings.HasPrefix(n.key, "build"), strings.HasPrefix(n.key, "run"):
		return "run"
	case strings.HasPrefix(n.key, "lsp"):
		return "lsp"
	case strings.HasPrefix(n.key, "vcs"):
		return "vcs"
	}
	return "system"
}

// notifyPrefsFor is one group's Settings choice.
func (m *Model) notifyPrefsFor(group string) NotifyGroupPrefs {
	if p := m.prefs.Notify; p != nil {
		return p.Groups[group]
	}
	return NotifyGroupPrefs{}
}

// notifyOff is Settings' global "Display balloon notifications" turned off.
func (m *Model) notifyOff() bool {
	return m.prefs.Notify != nil && m.prefs.Notify.NoBalloons
}

// loggedNotifications is the tool window's list: the entries whose group logs.
func (m *Model) loggedNotifications() []ideNotification {
	out := make([]ideNotification, 0, len(m.notify.items))
	for _, it := range m.notify.items {
		if it.logged {
			out = append(out, it)
		}
	}
	return out
}

// canSuppress reports whether a balloon offers "Don't Show Again": errors
// always show, and a group already silenced has no balloon to offer it on.
func (m *Model) canSuppress(n ideNotification) bool {
	return n.level != "error" && m.notifyPrefsFor(n.group).Display != "none"
}

// SuppressGroup is "Don't Show Again": the group stops popping up but keeps
// logging, as Settings › Notifications › No popup does.
func (m *Model) SuppressGroup(group string) {
	p := NotifyPrefs{}
	if m.prefs.Notify != nil {
		p.NoBalloons = m.prefs.Notify.NoBalloons
		if m.prefs.Notify.Groups != nil {
			p.Groups = make(map[string]NotifyGroupPrefs, len(m.prefs.Notify.Groups))
			for k, v := range m.prefs.Notify.Groups {
				p.Groups[k] = v
			}
		}
	}
	if p.Groups == nil {
		p.Groups = map[string]NotifyGroupPrefs{}
	}
	g := p.Groups[group]
	g.Display = "none"
	p.Groups[group] = g
	m.prefs.Notify = &p
	if path := m.settingsWritePath(); path != "" {
		if err := m.prefs.Save(path); err != nil {
			m.notifyError("notify:save", "Couldn't save notification settings", notifySuffix(err.Error()))
		}
	}
	kept := m.notify.items[:0]
	for _, it := range m.notify.items {
		if it.group != group {
			kept = append(kept, it)
			continue
		}
		stopNotifyTimer(&it)
		if it.logged {
			it.balloon = false
			kept = append(kept, it)
		}
	}
	m.notify.items = kept
	m.invalidate()
}
