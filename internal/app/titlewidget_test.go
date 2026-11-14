package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestTitleWidgetsAreRegisteredOnce(t *testing.T) {
	for _, id := range []string{toolbarProject, toolbarVCS, toolbarRun, toolbarSearch, toolbarTerminal} {
		w := titleWidgetFor(id)
		if w == nil {
			t.Fatalf("%s is not registered", id)
		}
		if w.Title == "" || w.Render == nil {
			t.Fatalf("%s has no title or render", id)
		}
	}
	if got := titleWidgetSide(toolbarVCS); got != "left" {
		t.Fatalf("the VCS widget's side is %q", got)
	}
	if got := titleWidgetSide(toolbarRun); got != "right" {
		t.Fatalf("the run widget's side is %q", got)
	}
	if got := titleWidgetSide(toolbarActionPrefix + "view.services"); got != "right" {
		t.Fatalf("an action entry's side is %q", got)
	}
	if toolbarTitle(toolbarProject) != "Project Widget" || toolbarTitle(toolbarVCS) != "VCS Widget" {
		t.Fatal("the customization does not list the widget titles")
	}
}

func TestTitleWidgetRegistrationLifecycle(t *testing.T) {
	registerTitleWidget(nil)
	registerTitleWidget(&TitleWidget{ID: ""})
	registerTitleWidget(&TitleWidget{ID: "test.title.nilrender"})
	if unregisterTitleWidget("test.title.nilrender") {
		t.Fatal("a widget without render was registered")
	}
	registerTitleWidget(&TitleWidget{ID: "test.title", Title: "Test", Side: "right",
		Render: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) {}})
	if titleWidgetFor("test.title") == nil {
		t.Fatal("the widget was not registered")
	}
	// A duplicate id must not replace the original contribution.
	registerTitleWidget(&TitleWidget{ID: "test.title", Title: "Again", Render: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) {}})
	if got := titleWidgetFor("test.title").Title; got != "Test" {
		t.Fatalf("a duplicate id replaced the original: %q", got)
	}
	if !unregisterTitleWidget("test.title") {
		t.Fatal("unregister reported nothing removed")
	}
	if titleWidgetFor("test.title") != nil {
		t.Fatal("the widget survived unregister")
	}
	if unregisterTitleWidget("test.title") {
		t.Fatal("a second unregister reported a removal")
	}
}

func TestToolbarDrawsAnAddedAction(t *testing.T) {
	m := &Model{}
	m.prefs.MainToolbar = &toolbarLayout{Right: []string{toolbarActionPrefix + "view.services"}}
	tt := workbenchTester(t, m)
	r, ok := tt.Find("Services")
	if !ok || r.Y >= oiTitleBarHeight {
		t.Fatalf("the added action is not in the title bar: %+v, %v", r, ok)
	}
	m.prefs.MainToolbar = nil
}

func TestTokensMatchTheWorkbenchConstants(t *testing.T) {
	tk := oiTokensOf(nil)
	if tk.TitleBarHeight != oiTitleBarHeight || tk.StatusBarHeight != oiStatusBarHeight ||
		tk.StripeWidth != oiStripeWidth || tk.IslandGap != oiIslandGap ||
		tk.TabHeight != oiTabHeight || tk.HeaderHeight != oiHeaderHeight {
		t.Fatalf("the tokens drifted from the workbench: %+v", tk)
	}
	if tk != oiTokensOf(&ui.Theme{Dark: true}) {
		t.Fatal("dark and light have different sizing tokens")
	}
	if tk.SpaceM != oiSpaceM || tk.RadiusM != oiRadiusM || tk.ButtonHeight != oiButtonHeight {
		t.Fatalf("a token is not the named constant: %+v", tk)
	}
}
