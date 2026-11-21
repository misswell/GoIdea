package app

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestOpenIDEATerminalActionsAreRegistered(t *testing.T) {
	for _, id := range []string{
		"terminal.newPredefinedSession",
		"terminal.openAt",
		"terminal.pluginSession",
		"terminal.runCommand",
		"terminal.splitRight",
		"terminal.splitDown",
		"terminal.splitAndMoveRight",
		"terminal.splitAndMoveDown",
		"terminal.closeSession",
		"terminal.unsplit",
	} {
		if _, ok := Command(id); !ok {
			t.Errorf("missing OpenIDEA terminal action %q", id)
		}
	}
}

func TestTerminalActionArgumentsMatchOpenIDEA(t *testing.T) {
	if got := terminalActionString("/bin/zsh"); got != "/bin/zsh" {
		t.Fatalf("shell argument = %q", got)
	}
	request := terminalActionObject(json.RawMessage(
		`{"program":"go","args":["test","./..."],"name":"tests","cwd":"/repo"}`,
	))
	if got := terminalActionStrings(request["args"]); !reflect.DeepEqual(got, []string{"test", "./..."}) {
		t.Fatalf("command arguments = %v", got)
	}
	if request["program"] != "go" || request["name"] != "tests" || request["cwd"] != "/repo" {
		t.Fatalf("command request = %#v", request)
	}
}

func TestMoveTerminalToSplitKeepsThePTY(t *testing.T) {
	m := &Model{}
	m.term.groups = []termGroup{{tabs: []termSession{
		{id: 1, channel: 10, cwd: "/repo"},
		{id: 2, channel: 11, cwd: "/repo/module"},
	}, active: 1}}
	m.term.active = 0
	m.MoveTermToSplit(&m.term.groups[0].tabs[1], "right")
	if len(m.term.groups) != 2 || m.term.split != "right" || m.term.active != 1 {
		t.Fatalf("split state = %+v", m.term)
	}
	if len(m.term.groups[0].tabs) != 1 || m.term.groups[0].tabs[0].id != 1 {
		t.Fatalf("left group = %+v", m.term.groups[0])
	}
	if len(m.term.groups[1].tabs) != 1 || m.term.groups[1].tabs[0].id != 2 {
		t.Fatalf("moved group = %+v", m.term.groups[1])
	}
}

func TestMoveTerminalToSplitRequiresATabLeftBehind(t *testing.T) {
	m := &Model{}
	m.term.groups = []termGroup{{tabs: []termSession{{id: 1}}, active: 0}}
	m.MoveTermToSplit(&m.term.groups[0].tabs[0], "down")
	if len(m.term.groups) != 1 || m.term.split != "" {
		t.Fatalf("single tab was split away: %+v", m.term)
	}
}

func TestTerminalFindStateBelongsToEachTab(t *testing.T) {
	m := &Model{}
	m.term = TermPane{
		visible: true,
		active:  0,
		groups: []termGroup{
			{
				tabs: []termSession{
					{channel: 1, lines: []string{"alpha", "needle"}, findAt: -1},
					{channel: 2, lines: []string{"other needle", "last"}, findAt: -1},
				},
				active: 0,
			},
			{tabs: []termSession{{channel: 3, lines: []string{"split needle"}, findAt: -1}}},
		},
	}

	m.ShowTermFind()
	m.term.current().find = "needle"
	m.FindTermStep(1)
	if !m.term.groups[0].tabs[0].finding || m.term.groups[0].tabs[0].findAt != 1 {
		t.Fatalf("first tab find state = %+v", m.term.groups[0].tabs[0])
	}

	if !m.ActivateTermTab(0, 1) {
		t.Fatal("second tab did not activate")
	}
	if m.term.groups[0].tabs[1].finding || m.term.groups[0].tabs[1].find != "" {
		t.Fatalf("find state leaked into the second tab: %+v", m.term.groups[0].tabs[1])
	}
	m.ShowTermFind()
	m.term.current().find = "other"
	m.FindTermStep(1)

	if !m.ActivateTermTab(0, 0) {
		t.Fatal("first tab did not reactivate")
	}
	if m.term.current().find != "needle" || m.term.current().findAt != 1 {
		t.Fatalf("first tab lost its find state: %+v", m.term.current())
	}
	if !m.ActivateTermGroup(1) || m.term.active != 1 {
		t.Fatalf("split group did not activate: %+v", m.term)
	}
}

func TestTerminalTabsCanReorderCloseOthersAndRename(t *testing.T) {
	m := &Model{}
	m.term = TermPane{
		active: 0,
		groups: []termGroup{{
			tabs: []termSession{
				{channel: 1, shell: "one"},
				{channel: 2, shell: "two"},
				{channel: 3, shell: "three"},
			},
			active: 1,
		}},
	}
	if !m.MoveTermTab(0, 0, 1) {
		t.Fatal("tab did not move right")
	}
	if m.term.groups[0].tabs[0].channel != 2 ||
		m.term.groups[0].tabs[1].channel != 1 ||
		m.term.groups[0].active != 0 {
		t.Fatalf("tab order after move = %+v", m.term.groups[0])
	}
	if !m.RenameTermSession(0, 1, "  Build shell  ") {
		t.Fatal("tab did not rename")
	}
	if got := termTabTitle(&m.term.groups[0].tabs[1], 1); got != "Build shell" {
		t.Fatalf("renamed tab title = %q", got)
	}
	if !m.CloseOtherTermTabs(0, 1) {
		t.Fatal("Close Other Tabs failed")
	}
	if len(m.term.groups[0].tabs) != 1 ||
		m.term.groups[0].tabs[0].customName != "Build shell" ||
		m.term.groups[0].active != 0 {
		t.Fatalf("remaining tab = %+v", m.term.groups[0])
	}
}
