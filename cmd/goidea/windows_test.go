package main

import (
	"slices"
	"testing"

	"github.com/egoist/mygo"

	"mayoi/internal/app"
)

func TestGoideaWindowUsesOpenIDEAChrome(t *testing.T) {
	options := goideaWindowOptions("main", true, nil)
	if options.TitleBarStyle != mygo.TitleBarHidden || options.TitleBarHeight != 40 {
		t.Fatalf("window title bar = %q height %d", options.TitleBarStyle, options.TitleBarHeight)
	}
	if options.TrafficLightPosition == nil ||
		options.TrafficLightPosition.X != 14 || options.TrafficLightPosition.Y != 13 {
		t.Fatalf("traffic light position = %+v", options.TrafficLightPosition)
	}
	if options.Title != "GoIDEA" || options.Width != 1280 || options.Height != 820 {
		t.Fatalf("window defaults = %+v", options)
	}
}

func TestWorkspaceWindowsOnlyIncludesTheModelsGroup(t *testing.T) {
	first, second, other, standalone := &app.Model{}, &app.Model{}, &app.Model{}, &app.Model{}
	host := &nativeWindowHost{
		windows: []*nativeProjectWindow{
			{id: "first", group: 1, model: first},
			{id: "second", group: 1, model: second},
			{id: "other", group: 2, model: other},
			{id: "standalone", model: standalone},
		},
		selectedWindow: map[int]string{1: "first", 2: "other"},
	}
	got := host.WorkspaceWindows(second)
	if len(got) != 2 || got[0].ID != "first" || !got[0].Selected || got[1].ID != "second" || got[1].Selected {
		t.Fatalf("group windows = %+v", got)
	}
	if got := host.WorkspaceWindows(standalone); len(got) != 0 {
		t.Fatalf("standalone window has group tabs: %+v", got)
	}
}

func TestWindowGroupActionsOnNativeHost(t *testing.T) {
	h := &nativeWindowHost{headless: true, selectedWindow: map[int]string{1: "b"}}
	for _, id := range []string{"a", "outside", "b", "c"} {
		m := &app.Model{}
		m.SetWorkspaceHost(h)
		g := 1
		if id == "outside" {
			g = 0
		}
		h.windows = append(h.windows, &nativeProjectWindow{id: id, group: g, model: m})
	}
	b := h.windowByID("b")
	h.active = b.model
	order := func() []string {
		var ids []string
		for _, w := range h.windows {
			ids = append(ids, w.id)
		}
		return ids
	}
	if !b.model.RunActionWithArg("window.reorderTabs", []any{"c", "a", "b"}) || !slices.Equal(order(), []string{"c", "outside", "a", "b"}) {
		t.Fatalf("group reorder changed the wrong slots: %v", order())
	}
	for _, bad := range []any{[]string{"outside", "a", "b"}, []string{"c", "a", "a"}, []string{"b"}, []any{"a", 7, "b", "c"}} {
		b.model.RunActionWithArg("window.reorderTabs", bad)
		if !slices.Equal(order(), []string{"c", "outside", "a", "b"}) {
			t.Fatalf("accepted invalid order %v", bad)
		}
	}
	b.model.RunActionWithArg("window.selectTab", "outside")
	if h.active != b.model {
		t.Fatal("selected another group")
	}
	b.model.RunAction("window.nextTab")
	if h.active != h.windowByID("c").model {
		t.Fatal("cycle did not wrap in group order")
	}
	h.active.RunAction("window.previousTab")
	if h.active != b.model {
		t.Fatal("reverse cycle failed")
	}
	b.model.RunActionWithArg("window.selectTab", "a")
	a := h.windowByID("a")
	if h.active != a.model {
		t.Fatal("did not select the project tab")
	}
	if !a.model.RunAction("window.detachTab") || a.group != 0 || h.active != a.model || len(h.windows) != 4 || len(h.groupedWindows(1)) != 2 {
		t.Fatal("detach did not preserve the original native windows")
	}
	if a.model.ActionEnabled("window.closeTab") || a.model.ActionEnabled("window.detachTab") {
		t.Fatal("standalone project still has group actions")
	}
}
