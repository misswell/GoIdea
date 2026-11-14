package app

import "testing"

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
	m.dismissBalloon(id)
	if !m.notify.items[0].balloon {
		t.Fatalf("a hovered balloon was dismissed")
	}
	m.SetBalloonHover(id, false)
	m.dismissBalloon(id)
	if m.notify.items[0].balloon {
		t.Fatalf("the balloon stayed after the pointer left")
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
	m.CloseNotifications()
	if m.notificationsShown() || !m.rightHidden {
		t.Fatalf("closing left shown=%v hidden=%v", m.notificationsShown(), m.rightHidden)
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
