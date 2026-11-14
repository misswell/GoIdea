package app

import "testing"

func TestDockProblemsRightAndBack(t *testing.T) {
	m := &Model{}
	m.ShowProblems()
	if !m.problems.visible {
		t.Fatalf("Problems did not open at the bottom")
	}
	m.MoveToolToRight()
	if !m.problems.visible {
		t.Fatalf("Problems lost its content when it moved")
	}
	if m.rightTool != "problems" || m.rightHidden || !m.rightShown() {
		t.Fatalf("the right area shows tool=%q hidden=%v shown=%v", m.rightTool, m.rightHidden, m.rightShown())
	}
	m.MoveToolToBottom()
	if !m.problems.visible || m.rightShown() {
		t.Fatalf("Problems did not return to the bottom: visible=%v right=%v", m.problems.visible, m.rightShown())
	}
	m.ClearNotifications()
}

func TestDockTodoRightAndBack(t *testing.T) {
	m := &Model{}
	m.ShowTodo()
	m.MoveToolToRight()
	if !m.todo.visible || m.rightTool != "todo" {
		t.Fatalf("TODO did not dock to the right: visible=%v tool=%q", m.todo.visible, m.rightTool)
	}
	m.MoveToolToBottom()
	if !m.todo.visible || m.tool != "todo" {
		t.Fatalf("TODO did not return to the bottom: visible=%v tool=%q", m.todo.visible, m.tool)
	}
}

func TestDockMovesBuildAndKeepsItsContent(t *testing.T) {
	m := &Model{}
	m.ShowBuild()
	m.MoveToolToRight()
	if m.rightTool != "build" || !m.build.visible {
		t.Fatalf("Build did not dock right: right=%q visible=%v", m.rightTool, m.build.visible)
	}
	if m.primaryTool() == "build" {
		t.Fatalf("Build stayed in the bottom area")
	}
	m.MoveToolToBottom()
	if m.rightTool != "" || m.primaryTool() != "build" || !m.build.visible {
		t.Fatalf("Build did not return: right=%q primary=%q", m.rightTool, m.primaryTool())
	}
}
