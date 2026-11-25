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
	"unicode/utf8"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const (
	notifyInfoTimeout  = 10 * time.Second
	notifyStickyTime   = 5 * time.Minute
	notifyMaxBalloons  = 3
	notifyMaxHistory   = 200
	notifyBalloonWidth = 340
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
	// progressTask replaces the body with the live task's message and bar,
	// as OpenIDEA's UpdateBalloonBody does.
	progressTask string
	// verbatim keeps text supplied by a plugin as written. Native messages
	// already contain their final text, but the flag follows upstream's
	// notification model and survives history persistence.
	verbatim bool
	// group is Settings › Notifications' group, logged whether it is listed
	// in the tool window, and userInitiated the answer to a command the user
	// ran (it pops up even with balloons off).
	group         string
	logged        bool
	userInitiated bool
	// balloon is whether it still shows in the corner; sticky is whether it
	// may fade on its own at all, and permanent keeps it until dismissed.
	balloon   bool
	sticky    bool
	permanent bool
	hover     bool

	timer     *time.Timer
	deadline  time.Time
	remaining time.Duration
}

// notifier is the notification center's state.
type notifier struct {
	items []ideNotification
	seq   int
}

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

// notifyUser posts the answer to a command the user ran: it pops up even with
// balloon notifications turned off, as upstream's userInitiated does.
func (m *Model) notifyUser(key, level, title, body string, actions ...notifyAction) {
	m.postNotification(ideNotification{key: key, level: level, title: title, body: body,
		actions: actions, userInitiated: true})
}

// notifySticky posts a notification that stays until it is dismissed or
// replaced, as upstream's update announcements do.
func (m *Model) notifySticky(key, level, title, body string, userInitiated bool, actions ...notifyAction) {
	m.postNotification(ideNotification{key: key, level: level, title: title, body: body,
		actions: actions, userInitiated: userInitiated, permanent: true})
}

// notifyStickyProgress draws a background task's live message and progress
// bar in a sticky balloon.
func (m *Model) notifyStickyProgress(key, level, title string, taskID string,
	userInitiated bool, actions ...notifyAction) {
	m.postNotification(ideNotification{key: key, level: level, title: title,
		actions: actions, userInitiated: userInitiated, permanent: true,
		progressTask: taskID})
}

// notifyPlugin posts text supplied by a plugin. It is user-initiated because
// the plugin call comes from a command the user ran or one of its listeners,
// and belongs to the Plugins notification group.
func (m *Model) notifyPlugin(title, body string, level string, actions ...notifyAction) {
	m.postNotification(ideNotification{
		level: level, title: title, body: body, actions: actions,
		group: "plugins", verbatim: true, userInitiated: true,
	})
}

// updateNotificationBody changes the newest notification of a key in place,
// so a download's progress replaces its balloon's text instead of restarting
// the notification.
func (m *Model) updateNotificationBody(key, body string) {
	for i := range m.notify.items {
		if m.notify.items[i].key != key {
			continue
		}
		m.notify.items[i].body = body
		m.invalidate()
		return
	}
}

// notifyBeep plays the system alert; a test replaces it.
var notifyBeep = func() { mygo.Shell.Beep() }

// postNotification adds an entry and starts its balloon's clock.
func (m *Model) postNotification(n ideNotification) {
	n.id = m.notify.seq + 1
	m.notify.seq = n.id
	n.at = time.Now()
	n.group = notifyGroupOf(n)
	prefs := m.notifyPrefsFor(n.group)
	n.balloon = n.userInitiated || (!m.notifyOff() && prefs.Display != "none")
	n.sticky = n.sticky || prefs.Display == "sticky" || n.level == "error"
	n.logged = !prefs.NoLog
	if n.balloon && m.prefs.Notify != nil && m.prefs.Notify.Sound {
		notifyBeep()
	}
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
	m.saveSession()
}

// notificationHistory snapshots the logged entries for Session. Timers, hover
// state and action callbacks are runtime-only and are not written.
func (m *Model) notificationHistory() []StoredNotification {
	out := make([]StoredNotification, 0, len(m.notify.items))
	for _, n := range m.notify.items {
		if !n.logged {
			continue
		}
		out = append(out, StoredNotification{
			Key: n.key, Level: n.level, Title: n.title, Body: n.body,
			At: n.at.UnixMilli(), Read: n.read, Group: n.group, Logged: true,
			Verbatim: n.verbatim,
		})
	}
	return out
}

// restoreNotificationHistory fills the tool window from the previous session.
func (m *Model) restoreNotificationHistory(items []StoredNotification) {
	if len(items) == 0 {
		return
	}
	m.notify.items = make([]ideNotification, 0, min(len(items), notifyMaxHistory))
	for i, stored := range items {
		if i >= notifyMaxHistory {
			break
		}
		m.notify.seq++
		m.notify.items = append(m.notify.items, ideNotification{
			id: m.notify.seq, key: stored.Key, level: stored.Level,
			title: stored.Title, body: stored.Body, at: time.UnixMilli(stored.At),
			read: stored.Read, group: stored.Group, logged: stored.Logged,
			verbatim: stored.Verbatim,
		})
	}
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
		if it.permanent {
			// Stays until dismissed, as an update's announcement does.
			return
		}
		wait := notifyInfoTimeout
		if it.sticky {
			wait = notifyStickyTime
		}
		it.remaining = wait
		it.deadline = time.Now().Add(wait)
		it.timer = time.AfterFunc(wait, func() {
			m.post(func() { m.fadeBalloon(id) })
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

// dismissBalloon hides one balloon because the user asked: its close button or
// one of its actions. The hover pause holds back the fade clock, never the
// user, and the pointer must rest on a balloon to press it.
func (m *Model) dismissBalloon(id int) {
	m.hideBalloon(id, false)
}

// fadeBalloon hides one balloon whose clock ran out: a balloon the pointer
// rests on stays, as IDEA's smart fade-out does.
func (m *Model) fadeBalloon(id int) {
	m.hideBalloon(id, true)
}

// hideBalloon hides one balloon; the notification stays in the history.
func (m *Model) hideBalloon(id int, respectHover bool) {
	for i := range m.notify.items {
		it := &m.notify.items[i]
		if it.id != id {
			continue
		}
		if respectHover && it.hover {
			return
		}
		stopNotifyTimer(it)
		it.balloon = false
		m.invalidate()
		m.saveSession()
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
				m.post(func() { m.fadeBalloon(id) })
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
	changed := false
	for i := range m.notify.items {
		if !m.notify.items[i].read {
			changed = true
		}
		m.notify.items[i].read = true
	}
	if changed {
		m.saveSession()
	}
}

// ShowNotifications opens the Notifications tool window on the right.
func (m *Model) ShowNotifications() { m.DockShow("notifications") }

// CloseNotifications hides the right area without forgetting its window.
func (m *Model) CloseNotifications() {
	if m.dockWindowShown("notifications") {
		m.DockHide("notifications")
	}
}

// ToggleNotificationsWindow is the stripe button's action.
func (m *Model) ToggleNotificationsWindow() { m.DockToggle("notifications") }

// ClearNotifications empties the history, as the tool window's trash does.
func (m *Model) ClearNotifications() {
	for i := range m.notify.items {
		stopNotifyTimer(&m.notify.items[i])
	}
	m.notify.items = nil
	m.invalidate()
	m.saveSession()
}

// notificationsShown reports whether the right area currently shows the list.
func (m *Model) notificationsShown() bool {
	return m.dockWindowShown("notifications")
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
		total += m.notifyHeight(it, len(m.balloonActions(it))) + 8
	}
	x := w - notifyBalloonWidth - 16
	statusHeight := oiStatusBarHeight
	if !m.prefs.Appearance.HideStatusBar {
		statusHeight = oiTokensFor(m, p).StatusBarHeight
	}
	y := h - statusHeight - 12 - total
	ui.Overlay(c, func() {
		ui.Column(c).Absolute().Left(x).Top(y).Width(notifyBalloonWidth).Gap(8).
			Children(func() {
				for _, it := range items {
					m.balloonView(c, p, it)
				}
			})
	})
}

// notifyHeight is a balloon's height from its content, including the actions
// the balloon actually shows: the suppression balloonView appends counts, and
// leaving it out clipped the last action row of the bottom balloon.
func (m *Model) notifyHeight(n ideNotification, actions int) float32 {
	h := float32(46)
	if task, ok := m.notificationTask(n); ok {
		h += taskBarHeight + oiSpaceS
		if task.Message != "" {
			h += notificationBodyHeight(task.Message) + oiSpaceS
		}
	} else if n.body != "" {
		h += notificationBodyHeight(n.body)
	}
	if actions > 0 {
		h += 24
	}
	return h
}

// balloonActions is what a balloon offers: its own actions, then the
// suppression the notification's group allows.
func (m *Model) balloonActions(n ideNotification) []notifyAction {
	actions := n.actions
	if m.canSuppress(n) {
		actions = append(append([]notifyAction(nil), actions...),
			notifyAction{label: "Don't Show Again", run: func(m *Model) { m.SuppressGroup(n.group) }})
	}
	return actions
}

// balloonView draws one balloon as OpenIDEA's Toast does: the level icon, the
// title with a close button, the body and the action links, on the popup
// surface with the balloon shadow.
func (m *Model) balloonView(c *ui.Context, p *ui.Theme, n ideNotification) {
	oi := oiPaletteOf(p)
	actions := m.balloonActions(n)
	height := m.notifyHeight(n, len(actions))
	shade := float32(0.14)
	if oi.dark {
		shade = 0.45
	}
	r := ui.Row(c.Key(fmt.Sprintf("balloon:%d", n.id))).Height(height).
		Padding(14, 34, 14, 14).Gap(10).AlignItems(ui.Start).
		Radius(10).Background(oi.popup).BorderWidth(1).BorderColor(oi.popupBorder).
		Shadow(0, 8, 24, 0, ui.RGBA(0, 0, 0, shade))
	if r.Hovered() != n.hover {
		m.SetBalloonHover(n.id, r.Hovered())
	}
	icon := "status/info"
	switch n.level {
	case "error":
		icon = "status/error"
	case "warning":
		icon = "status/warning"
	case "success":
		icon = "status/success"
	}
	r.Children(func() {
		oiIconEl(c, icon, 16, oi, notifyLevelColor(p, n.level))
		ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, n.title).FontSize(13).Bold().TextColor(oi.textStrong).
					SingleLine().Ellipsis("…")
				ui.Spacer(c)
				close := ui.ButtonBase(c.Key(fmt.Sprintf("balloon:close:%d", n.id))).
					Size(20, 20).Radius(4).Justify(ui.Center).AlignItems(ui.Center).
					Cursor(ui.CursorPointer)
				if close.Hovered() {
					close.Background(oi.hoverTint)
				}
				close.Tooltip("Dismiss")
				close.Children(func() {
					oiIconEl(c, "general/close", 12, oi, oi.textSubtle)
				})
				close.OnClick(func() { m.dismissBalloon(n.id) })
			})
			if task, ok := m.notificationTask(n); ok {
				if task.Message != "" {
					body := ui.Text(c, task.Message).FontSize(13).TextColor(oi.textSecondary).Wrap()
					body.MaxHeight(notificationBodyHeight(task.Message))
				}
				m.taskBar(c, p, oi, task.Fraction, 0)
			} else if n.body != "" {
				body := ui.Text(c, n.body).FontSize(13).TextColor(oi.textSecondary).Wrap()
				body.MaxHeight(notificationBodyHeight(n.body))
			}
			if len(actions) > 0 {
				ui.Row(c).Gap(14).Margin(4, 0, 0, 0).Children(func() {
					for i, a := range actions {
						a := a
						b := ui.ButtonBase(c.Key(fmt.Sprintf("balloon:act:%d:%d", n.id, i))).
							Padding(0, 0).Cursor(ui.CursorPointer)
						b.Children(func() {
							ui.Text(c, a.label).FontSize(13).TextColor(oi.link).SingleLine()
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

func (m *Model) notificationTask(n ideNotification) (BackgroundTask, bool) {
	if n.progressTask == "" {
		return BackgroundTask{}, false
	}
	for _, task := range m.tasks {
		if task.ID == n.progressTask && !task.Done {
			return task, true
		}
	}
	return BackgroundTask{}, false
}

// notificationsView is the Notifications tool window: history newest first,
// each entry a balloon-like card with its time. OpenIDEA's content is the
// list alone; the window chrome carries the title.
func (m *Model) notificationsView(c *ui.Context, p *ui.Theme, oi oiColors) {
	ui.Column(c).Key("notifications:window").Grow(1).Children(func() {
		logged := m.loggedNotifications()
		if len(logged) == 0 {
			// OpenIDEA's empty text is centred grey lines without an
			// illustration, even though the call site names the bell icon.
			ui.Column(c).Grow(1).Justify(ui.Center).AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Text(c, "No notifications").FontSize(13).TextColor(p.TextMuted).SingleLine()
			})
			return
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Children(func() {
				for i, it := range logged {
					it := it
					last := i == len(logged)-1
					icon := "status/info"
					switch it.level {
					case "error":
						icon = "status/error"
					case "warning":
						icon = "status/warning"
					case "success":
						icon = "status/success"
					}
					// OpenIDEA's toast inside the tool window: a full-width
					// row separated by a bottom border, no card background.
					card := ui.Row(c.Key(fmt.Sprintf("notify:%d", it.id))).
						Padding(14, 34, 14, 14).Gap(10).AlignItems(ui.Start)
					if !last {
						card.BorderWidth(0, 0, 1, 0).BorderColor(oi.border)
					}
					card.Children(func() {
						oiIconEl(c, icon, 16, oi, notifyLevelColor(p, it.level))
						ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
							ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
								ui.Text(c, it.title).FontSize(13).Bold().TextColor(oi.textStrong).
									SingleLine().Ellipsis("…")
								ui.Spacer(c)
								ui.Text(c, it.at.Format("15:04")).FontSize(12).
									TextColor(oi.textSubtle).SingleLine()
								x := ui.ButtonBase(c.Key(fmt.Sprintf("notify:close:%d", it.id))).
									Size(20, 20).Radius(4).Justify(ui.Center).AlignItems(ui.Center).
									Cursor(ui.CursorPointer)
								if x.Hovered() {
									x.Background(oi.hoverTint)
								}
								x.Tooltip("Dismiss")
								x.Children(func() {
									oiIconEl(c, "general/close", 12, oi, oi.textSubtle)
								})
								x.OnClick(func() { m.removeNotification(it.id) })
							})
							if it.body != "" {
								ui.Text(c, it.body).FontSize(13).TextColor(oi.textSecondary).Wrap()
							}
							if len(it.actions) > 0 {
								ui.Row(c).Gap(14).Margin(4, 0, 0, 0).Children(func() {
									for i, a := range it.actions {
										a := a
										b := ui.ButtonBase(c.Key(fmt.Sprintf("notify:act:%d:%d", it.id, i))).
											Padding(0, 0).Cursor(ui.CursorPointer)
										b.Children(func() {
											ui.Text(c, a.label).FontSize(13).TextColor(oi.link).SingleLine()
										})
										b.OnClick(func() { a.run(m) })
									}
								})
							}
						})
					})
				}
			})
		})
	})
}

// notificationBodyHeight reserves room for explicit lines and wraps long
// lines at the balloon's text width. Balloons cap the body so three large
// plugin messages cannot push their close buttons beyond the window.
func notificationBodyHeight(body string) float32 {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	lines := 0
	for _, line := range strings.Split(body, "\n") {
		n := utf8.RuneCountInString(line)
		lines += max(1, (n+41)/42)
	}
	lines = max(1, min(lines, 8))
	return float32(lines * 20)
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
	return n.key != updateKey && n.level != "error" && m.notifyPrefsFor(n.group).Display != "none"
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
		if err := m.savePreferences(); err != nil {
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
