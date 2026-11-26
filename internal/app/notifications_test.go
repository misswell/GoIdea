package app

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestNotificationTimeoutMatchesOpenIDEA(t *testing.T) {
	tests := []struct {
		name    string
		post    func(*Model)
		want    time.Duration
		hasTime bool
	}{
		{
			name:    "info",
			post:    func(m *Model) { m.notifyInfo("info", "Info", "") },
			want:    notifyInfoTimeout,
			hasTime: true,
		},
		{
			name:    "warning",
			post:    func(m *Model) { m.notifyWarning("warning", "Warning", "") },
			want:    notifyInfoTimeout,
			hasTime: true,
		},
		{
			name:    "default error",
			post:    func(m *Model) { m.notifyError("error", "Error", "") },
			want:    notifyStickyTime,
			hasTime: true,
		},
		{
			name: "explicit non-sticky error",
			post: func(m *Model) {
				m.postNotification(ideNotification{
					key: "error", level: "error", title: "Formatting failed",
					sticky: false, stickySet: true,
				})
			},
			want:    notifyInfoTimeout,
			hasTime: true,
		},
		{
			name: "sticky group",
			post: func(m *Model) {
				m.prefs.Notify = &NotifyPrefs{Groups: map[string]NotifyGroupPrefs{
					"system": {Display: "sticky"},
				}}
				m.notifyInfo("sticky", "Sticky", "")
			},
			want:    notifyStickyTime,
			hasTime: true,
		},
		{
			name: "unlogged sticky",
			post: func(m *Model) {
				m.prefs.Notify = &NotifyPrefs{Groups: map[string]NotifyGroupPrefs{
					"system": {NoLog: true},
				}}
				m.notifySticky("sticky-unlogged", "warning", "Sync failed", "", false)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := &Model{}
			test.post(m)
			if len(m.notify.items) != 1 {
				t.Fatalf("notifications = %+v", m.notify.items)
			}
			got, hasTime := notificationTimeout(m.notify.items[0])
			if hasTime != test.hasTime || got != test.want {
				t.Fatalf("timeout = %v, %v, want %v, %v", got, hasTime, test.want, test.hasTime)
			}
		})
	}
}

func TestNotificationClockPausesOutsideTheActiveWindow(t *testing.T) {
	oldNow, oldSchedule := notifyNow, notifySchedule
	defer func() {
		notifyNow, notifySchedule = oldNow, oldSchedule
	}()
	now := time.Unix(1_000, 0)
	var waits []time.Duration
	var callbacks []func()
	notifyNow = func() time.Time { return now }
	notifySchedule = func(wait time.Duration, fn func()) func() {
		waits = append(waits, wait)
		callbacks = append(callbacks, fn)
		return func() {}
	}
	m := &Model{postHook: func(fn func()) { fn() }}
	m.notify.active = true
	m.notify.activeKnown = true
	m.notifyInfo("indexed", "Indexed", "")
	if len(waits) != 1 || waits[0] != notifyInfoTimeout {
		t.Fatalf("initial wait = %v", waits)
	}
	now = now.Add(notifyInfoTimeout - time.Second)
	m.SetWindowActive(false)
	if got := m.notify.items[0].remaining; got != time.Second {
		t.Fatalf("remaining asleep = %v, want 1s", got)
	}
	now = now.Add(time.Minute)
	m.SetWindowActive(true)
	if len(waits) != 2 || waits[1] != time.Second {
		t.Fatalf("resumed wait = %v, want second 1s", waits)
	}
	callbacks[len(callbacks)-1]()
	if m.notify.items[0].balloon {
		t.Fatal("the balloon stayed after its active-window clock elapsed")
	}
}

func TestNotificationHoverResumesTheRemainingTime(t *testing.T) {
	oldNow, oldSchedule := notifyNow, notifySchedule
	defer func() {
		notifyNow, notifySchedule = oldNow, oldSchedule
	}()
	now := time.Unix(2_000, 0)
	var waits []time.Duration
	var callbacks []func()
	notifyNow = func() time.Time { return now }
	notifySchedule = func(wait time.Duration, fn func()) func() {
		waits = append(waits, wait)
		callbacks = append(callbacks, fn)
		return func() {}
	}
	m := &Model{postHook: func(fn func()) { fn() }}
	m.notifyInfo("hover", "Hover", "")
	now = now.Add(notifyInfoTimeout - time.Second)
	m.SetBalloonHover(m.notify.items[0].id, true)
	if got := m.notify.items[0].remaining; got != time.Second {
		t.Fatalf("remaining on hover = %v, want 1s", got)
	}
	m.SetBalloonHover(m.notify.items[0].id, false)
	if len(waits) != 2 || waits[1] != time.Second {
		t.Fatalf("resumed hover wait = %v, want second 1s", waits)
	}
	callbacks[len(callbacks)-1]()
	if m.notify.items[0].balloon {
		t.Fatal("the balloon stayed after its resumed hover clock elapsed")
	}
}

func TestNotificationKeyReplacesAndErrorsStick(t *testing.T) {
	m := &Model{}
	m.notifyInfo("k", "First", "one")
	first := m.notify.items[0].id
	m.notifyInfo("k", "Second", "two")
	if len(m.notify.items) != 1 {
		t.Fatalf("the keyed notification was not replaced: %d entries", len(m.notify.items))
	}
	if m.notify.items[0].title != "Second" || m.notify.items[0].id == first {
		t.Fatalf("the newer notification did not win: %+v", m.notify.items[0])
	}
	m.notifyError("e", "Failed", "")
	if !m.notify.items[0].sticky {
		t.Fatalf("an error is not sticky")
	}
	m.notifyWarning("w", "Careful", "")
	if m.notify.items[0].sticky {
		t.Fatalf("a warning is sticky")
	}
	m.ClearNotifications()
	if len(m.notify.items) != 0 {
		t.Fatalf("ClearNotifications left %d entries", len(m.notify.items))
	}
}

func TestBalloonHeightCountsTheSuppressionAction(t *testing.T) {
	m := &Model{}
	n := ideNotification{group: "shot.info", level: "info", title: "Build finished", body: "one"}
	// A balloon the user can suppress shows "Don't Show Again", and the
	// height must leave room for it: a height that counts only n.actions
	// clipped that row off the bottom of the balloon.
	if actions := m.balloonActions(n); len(actions) != 1 || actions[0].label != "Don't Show Again" {
		t.Fatalf("actions = %+v, want the suppression", actions)
	}
	if plain, with := m.notifyHeight(n, 0), m.notifyHeight(n, len(m.balloonActions(n))); with-plain != 24 {
		t.Fatalf("suppression adds %v to the balloon height, want 24", with-plain)
	}
	err := ideNotification{group: "shot.error", level: "error", title: "Build failed", body: "two"}
	if got := len(m.balloonActions(err)); got != 0 {
		t.Fatalf("an error offers %d actions, want none", got)
	}
}

func TestUpdateBalloonUsesTheLiveTaskProgress(t *testing.T) {
	m := &Model{}
	m.tasks = []BackgroundTask{{
		ID: updateInstallTaskID, Title: "Downloading GoIDEA 0.1.42",
		Message: "5.0 MB of 60.0 MB", Fraction: 0.5,
	}}
	m.notifyStickyProgress(updateKey, "info", "Downloading GoIDEA 0.1.42",
		updateInstallTaskID, true)
	if len(m.notify.items) != 1 {
		t.Fatalf("notifications = %+v", m.notify.items)
	}
	n := m.notify.items[0]
	if n.progressTask != updateInstallTaskID || n.body != "" {
		t.Fatalf("progress notification = %+v", n)
	}
	task, ok := m.notificationTask(n)
	if !ok || task.Fraction != 0.5 || task.Message != "5.0 MB of 60.0 MB" {
		t.Fatalf("notification task = %+v, %v", task, ok)
	}
	withoutProgress := m.notifyHeight(ideNotification{
		group: "shot.info", level: "info", title: n.title,
	}, 0)
	if withProgress := m.notifyHeight(n, 0); withProgress <= withoutProgress {
		t.Fatalf("progress height = %v, plain height = %v", withProgress, withoutProgress)
	}
	if timeout, ok := notificationTimeout(n); ok || timeout != 0 {
		t.Fatalf("download timeout = %v, %v; want no timer", timeout, ok)
	}
	m.tasks[0].Message = "9.0 MB of 60.0 MB"
	m.tasks[0].Fraction = 0.9
	if task, ok := m.notificationTask(n); !ok ||
		task.Fraction != 0.9 || task.Message != "9.0 MB of 60.0 MB" {
		t.Fatalf("live notification task = %+v, %v", task, ok)
	}
}

func TestUpdateDownloadBalloonStaysUntilDismissedAndOffersSuppression(t *testing.T) {
	m := &Model{}
	m.notifyStickyProgress(updateKey, "info", "Downloading GoIDEA 0.1.44",
		updateInstallTaskID, true,
		notifyAction{label: "Cancel", run: func(*Model) {}})
	if len(m.notify.items) != 1 || !m.notify.items[0].balloon {
		t.Fatalf("download notification = %+v", m.notify.items)
	}
	n := m.notify.items[0]
	if timeout, ok := notificationTimeout(n); ok || timeout != 0 {
		t.Fatalf("download timeout = %v, %v; want no timer", timeout, ok)
	}
	actions := m.balloonActions(n)
	if len(actions) != 2 || actions[0].label != "Cancel" ||
		actions[1].label != "Don't Show Again" {
		t.Fatalf("download actions = %+v", actions)
	}
	m.dismissBalloon(n.id)
	if len(m.notify.items) != 1 || m.notify.items[0].balloon {
		t.Fatalf("manual close left %+v", m.notify.items)
	}
}

func TestNotificationBodyHeightPreservesMultilineText(t *testing.T) {
	if got := notificationBodyHeight("one\ntwo\nthree"); got != 60 {
		t.Fatalf("three-line body height = %v, want 60", got)
	}
	if got := notificationBodyHeight(strings.Repeat("x", 85)); got != 60 {
		t.Fatalf("wrapped body height = %v, want 60", got)
	}
	if got := notificationBodyHeight(strings.Repeat("line\n", 20)); got != 160 {
		t.Fatalf("unbounded body height = %v, want the eight-line cap", got)
	}
}

func TestBalloonsKeepThreeNewestAtTheBottom(t *testing.T) {
	m := &Model{}
	m.notifyInfo("a", "A", "")
	m.notifyInfo("b", "B", "")
	m.notifyInfo("c", "C", "")
	m.notifyInfo("d", "D", "")
	items := m.balloonItems()
	if len(items) != notifyMaxBalloons {
		t.Fatalf("the stack holds %d balloons, want %d", len(items), notifyMaxBalloons)
	}
	if items[len(items)-1].title != "D" {
		t.Fatalf("the newest balloon is not at the bottom: %+v", items)
	}
	m.ClearNotifications()
}

func TestHoverPausesBalloonDismissal(t *testing.T) {
	m := &Model{}
	m.notifyInfo("h", "Hover", "")
	id := m.notify.items[0].id
	m.SetBalloonHover(id, true)
	m.fadeBalloon(id)
	if !m.notify.items[0].balloon {
		t.Fatalf("a hovered balloon was dismissed")
	}
	m.SetBalloonHover(id, false)
	m.fadeBalloon(id)
	if m.notify.items[0].balloon {
		t.Fatalf("the balloon stayed after the pointer left")
	}
	m.ClearNotifications()
}

// TestCloseButtonDismissesAHoveredBalloon: the pointer must rest on a balloon
// to press its close button, so the hover pause cannot hold that press back.
func TestCloseButtonDismissesAHoveredBalloon(t *testing.T) {
	m := &Model{postHook: func(fn func()) { fn() }}
	m.setRoot(t.TempDir())
	m.notifyUser("kn", "info", "GoIDEA 0.1.26 is up to date", "")
	tt := workbenchTester(t, m)
	title, ok := tt.Find("GoIDEA 0.1.26 is up to date")
	if !ok {
		t.Fatalf("the balloon is not on the window: %q", tt.Texts())
	}
	// The close button sits at the balloon's right edge, level with its title:
	// 16 DIPs from the window edge and 34 of the balloon's padding, with the
	// button 20 wide.
	x, y := float32(1440-16-34-10), title.Y+title.H/2
	tt.Move(x, y)
	tt.Frame()
	if !m.notify.items[0].hover {
		t.Fatalf("the pointer on the balloon did not pause it: %+v", m.notify.items[0])
	}
	tt.ClickAt(x, y)
	if m.notify.items[0].balloon {
		t.Fatalf("the close button left the balloon up")
	}
	m.ClearNotifications()
}

func TestNotificationsWindowMarksRead(t *testing.T) {
	m := &Model{}
	m.notifyInfo("r", "Unread", "")
	if m.unreadNotifications() != 1 {
		t.Fatalf("unread = %d, want 1", m.unreadNotifications())
	}
	m.ShowNotifications()
	if !m.notificationsShown() || m.rightTool != "notifications" {
		t.Fatalf("the window did not open: shown=%v tool=%q", m.notificationsShown(), m.rightTool)
	}
	if m.unreadNotifications() != 0 {
		t.Fatalf("opening the window left %d unread", m.unreadNotifications())
	}
	if m.notify.items[0].balloon || m.notify.items[0].timer != nil {
		t.Fatalf("the listed balloon did not defer to the tool window: %+v", m.notify.items[0])
	}
	m.CloseNotifications()
	if m.notificationsShown() || !m.rightHidden {
		t.Fatalf("closing left shown=%v hidden=%v", m.notificationsShown(), m.rightHidden)
	}
	m.ClearNotifications()
}

func TestUnloggedBalloonStaysVisibleBesideTheNotificationsWindow(t *testing.T) {
	m := &Model{prefs: Preferences{Notify: &NotifyPrefs{Groups: map[string]NotifyGroupPrefs{
		"system": {NoLog: true},
	}}}}
	m.notifyInfo("unlogged", "No history", "")
	m.ShowNotifications()
	items := m.balloonItems()
	if len(items) != 1 || items[0].title != "No history" || m.notify.items[0].logged {
		t.Fatalf("unlogged balloon = %+v, items = %+v", m.notify.items, items)
	}
	m.dismissBalloon(m.notify.items[0].id)
	if len(m.notify.items) != 0 {
		t.Fatalf("dismissing an unlogged balloon kept %+v", m.notify.items)
	}
}

func TestNotificationsWindowRendersOpenIDEAHeader(t *testing.T) {
	m := &Model{}
	m.notifyInfo("header", "Build finished", "ready")
	tester := ui.NewTester(func(c *ui.Context) {
		m.notificationsView(c, c.Theme(), oiPaletteOf(c.Theme()))
	}, 420, 480)
	tester.Frame()
	for _, text := range []string{"Notifications", "Build finished"} {
		if !tester.HasText(text) {
			t.Fatalf("%q is missing from the Notifications window: %q", text, tester.Texts())
		}
	}
	m.ClearNotifications()
}

func TestNotificationHistorySurvivesARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	m := &Model{cfg: Config{SessionPath: path}}
	m.notifyInfo("saved", "Saved notification", "Its text is restored.")
	m.notify.items[0].read = true
	m.saveSessionNow()

	session := LoadSession(path)
	if len(session.Notifications) != 1 {
		t.Fatalf("session notifications = %+v", session.Notifications)
	}
	restored := &Model{}
	restored.restoreNotificationHistory(session.Notifications)
	if len(restored.notify.items) != 1 {
		t.Fatalf("restored notifications = %+v", restored.notify.items)
	}
	got := restored.notify.items[0]
	m.notifyPlugin("Demo Plugin", "first\nsecond", "info")
	m.notify.items[0].read = true
	m.saveSessionNow()
	session = LoadSession(path)
	if len(session.Notifications) != 2 || !session.Notifications[0].Verbatim {
		t.Fatalf("plugin notification history = %+v", session.Notifications)
	}
	restored = &Model{}
	restored.restoreNotificationHistory(session.Notifications)
	got = restored.notify.items[0]
	if got.title != "Demo Plugin" || got.body != "first\nsecond" ||
		!got.read || !got.logged || got.balloon || !got.verbatim {
		t.Fatalf("restored notification = %+v", got)
	}
	m.ClearNotifications()
}

func TestNotificationLayoutRoundTrips(t *testing.T) {
	m := &Model{rightTool: "notifications", rightHidden: false, rightWidth: 320}
	l := m.currentLayout()
	if l.Right != "notifications" || l.RightHidden {
		t.Fatalf("the layout stored right=%q hidden=%v", l.Right, l.RightHidden)
	}
	n := normalizeLayout(l)
	if n.Right != "notifications" || n.RightHidden || n.RightWidth != 320 {
		t.Fatalf("normalizeLayout changed the right window: %+v", n)
	}
}

func TestNotificationGroupPrefsSilenceAndLog(t *testing.T) {
	m := &Model{prefs: Preferences{Notify: &NotifyPrefs{Groups: map[string]NotifyGroupPrefs{
		"system": {Display: "none"},
	}}}}
	m.notifyInfo("sys:a", "Silenced", "")
	if len(m.notify.items) != 1 {
		t.Fatalf("a silenced group was dropped from the log: %d entries", len(m.notify.items))
	}
	if m.notify.items[0].balloon {
		t.Fatalf("a silenced group still popped up")
	}
	if !m.notify.items[0].logged || m.unreadNotifications() != 1 {
		t.Fatalf("a silenced group lost its log entry or unread count")
	}
	m.notifyInfo("sys:b", "Sticky", "")
	m.prefs.Notify.Groups["system"] = NotifyGroupPrefs{Display: "sticky"}
	m.notifyInfo("sys:c", "Sticky again", "")
	if !m.notify.items[0].balloon || !m.notify.items[0].sticky {
		t.Fatalf("a sticky group did not pop as sticky: %+v", m.notify.items[0])
	}
	m.ClearNotifications()

	m.prefs.Notify = &NotifyPrefs{NoBalloons: true}
	m.notifyWarning("run:w", "Off", "")
	if m.notify.items[0].balloon {
		t.Fatalf("balloons stayed on with the global switch off")
	}
	m.ClearNotifications()
}

func TestProjectSwitchKeepsOnlyApplicationUpdateNotifications(t *testing.T) {
	m := &Model{}
	m.notifyInfo("build", "Build finished", "")
	m.notifyWarning("vcs:push", "Push failed", "")
	m.notifySticky(updateKey, "info", "GoIDEA is available", "", false)
	m.notifyInfo("lsp:warning", "Language server stopped", "")
	m.clearProjectNotifications()
	if len(m.notify.items) != 1 {
		t.Fatalf("project switch kept %+v", m.notify.items)
	}
	if got := m.notify.items[0]; got.key != updateKey || got.group != "updates" {
		t.Fatalf("update notification = %+v", got)
	}
}

func TestSuppressGroupKeepsHistory(t *testing.T) {
	m := &Model{}
	m.notifyWarning("lsp:w", "Language server", "")
	m.SuppressGroup("lsp")
	if got := m.notifyPrefsFor("lsp").Display; got != "none" {
		t.Fatalf("SuppressGroup stored display %q", got)
	}
	if len(m.notify.items) != 1 || m.notify.items[0].balloon || !m.notify.items[0].logged {
		t.Fatalf("suppression dropped or kept the balloon: %+v", m.notify.items)
	}
	m.ClearNotifications()
}

func TestNotifyPrefsDraftRoundTrip(t *testing.T) {
	m := &Model{}
	d := m.settingsDraftFromPrefs()
	if !d.notifyBalloons || len(d.notifyGroups) != 0 {
		t.Fatalf("the default draft is %+v", d)
	}
	if d.notifySound {
		t.Fatal("the default draft plays sound")
	}
	if p := d.notifyPrefs(); p != nil {
		t.Fatalf("the default draft stores %+v, want nil", p)
	}
	d.notifySound = true
	if p := d.notifyPrefs(); p == nil || !p.Sound {
		t.Fatalf("the staged sound is %+v", p)
	}
	d.notifySound = false
	d.notifyBalloons = false
	d.notifyGroups["lsp"] = NotifyGroupPrefs{Display: "none", NoLog: true}
	p := d.notifyPrefs()
	if p == nil || !p.NoBalloons || p.Groups["lsp"] != (NotifyGroupPrefs{Display: "none", NoLog: true}) {
		t.Fatalf("the draft stored %+v", p)
	}
	if !sameNotifyGroups(d.notifyGroups, p.Groups) {
		t.Fatalf("the staged groups do not compare equal to the stored ones")
	}
}

func TestNotificationSoundPlaysForBalloons(t *testing.T) {
	old := notifyBeep
	played := 0
	notifyBeep = func() { played++ }
	defer func() { notifyBeep = old }()

	m := &Model{}
	m.notifyInfo("quiet", "Quiet", "")
	if played != 0 {
		t.Fatalf("sound played while off: %d", played)
	}
	m.prefs.Notify = &NotifyPrefs{Sound: true}
	m.notifyInfo("loud", "Loud", "")
	if played != 1 {
		t.Fatalf("the balloon did not play the sound: %d", played)
	}
	m.prefs.Notify.Groups = map[string]NotifyGroupPrefs{"lsp": {Display: "none"}}
	m.notifyInfo("lsp:silent", "Silent", "")
	if played != 1 {
		t.Fatalf("a suppressed balloon played the sound: %d", played)
	}
	m.ClearNotifications()
}
