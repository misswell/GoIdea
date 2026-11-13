package app

import "testing"

func TestCommandMatches(t *testing.T) {
	quick := appCommand{title: "快速打开…", group: "文件", keywords: "quick open"}
	cases := []struct {
		cmd   appCommand
		query string
		want  bool
	}{
		{quick, "", true},
		{quick, "   ", true},
		{quick, "快速", true},
		{quick, "quick", true},
		{quick, "qo", true},
		{quick, "打开快", false},
		{quick, "zzz", false},
		{appCommand{title: "显示 Git 更改", keywords: "git vcs"}, "git", true},
		{appCommand{title: "显示 Git 更改", keywords: "git vcs"}, "GIT", true},
	}
	for _, c := range cases {
		if got := commandMatches(c.cmd, c.query); got != c.want {
			t.Errorf("commandMatches(%q, %q) = %v, want %v", c.cmd.title, c.query, got, c.want)
		}
	}
}

func TestRefreshKeepsSelectionInRange(t *testing.T) {
	p := &commandPicker{sel: 100}
	p.refresh()
	if len(p.items) != len(commands()) {
		t.Fatalf("empty query listed %d commands, want %d", len(p.items), len(commands()))
	}
	if p.sel != len(p.items)-1 {
		t.Fatalf("selection = %d, want %d", p.sel, len(p.items)-1)
	}
	p.query = "zzzz"
	p.refresh()
	if len(p.items) != 0 {
		t.Fatalf("nonsense query listed %d commands", len(p.items))
	}
}

func TestRunSelectedCommandNeedsAnItem(t *testing.T) {
	m := &Model{palette: &commandPicker{open: true}}
	m.runSelectedCommand()
	if m.palette == nil {
		t.Fatalf("empty palette should stay open")
	}
}

func TestRunSelectedCommandRunsAndCloses(t *testing.T) {
	ran := false
	m := &Model{palette: &commandPicker{open: true, items: []appCommand{
		{title: "x", run: func(*Model) { ran = true }},
	}}}
	m.runSelectedCommand()
	if !ran {
		t.Fatalf("command did not run")
	}
	if m.palette != nil {
		t.Fatalf("palette stayed open")
	}
}

func TestStepCommandStopsAtEnds(t *testing.T) {
	m := &Model{palette: &commandPicker{open: true}}
	m.palette.refresh()
	m.stepCommand(-1)
	if m.palette.sel != 0 {
		t.Fatalf("sel = %d, want 0", m.palette.sel)
	}
	m.stepCommand(1)
	if m.palette.sel != 1 {
		t.Fatalf("sel = %d, want 1", m.palette.sel)
	}
}
