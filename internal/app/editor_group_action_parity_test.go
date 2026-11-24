package app

import (
	"testing"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
)

func editorGroupTab(path, text string) *Tab {
	return &Tab{
		doc: docbridge.Virtual(path, text),
	}
}

func editorGroupModel() *Model {
	m := toolModel()
	m.reg = docbridge.NewRegistry(&fileActionBackend{info: openidea.DocInfo{
		ID: 73, Path: "/project/Main.java", Text: "class Main {}\n", Version: 1,
	}}, Window)
	m.tabs = []*Tab{
		editorGroupTab("/project/Main.java", "class Main {}\n"),
		editorGroupTab("/project/Other.java", "class Other {}\n"),
	}
	m.active = 0
	return m
}

func TestOpenIDEAEditorGroupActionsUseTheNativeCommandPath(t *testing.T) {
	t.Run("split right and orientation", func(t *testing.T) {
		m := editorGroupModel()
		if !m.RunAction("editor.splitRight") {
			t.Fatal("editor.splitRight was unavailable")
		}
		if m.split == nil || m.splitDown {
			t.Fatalf("right split state: split=%v down=%v", m.split != nil, m.splitDown)
		}
		if !m.RunAction("editor.changeSplitOrientation") {
			t.Fatal("editor.changeSplitOrientation was unavailable")
		}
		if !m.splitDown {
			t.Fatal("orientation did not flip the split")
		}
	})

	t.Run("split down", func(t *testing.T) {
		m := editorGroupModel()
		if !m.RunAction("editor.splitDown") {
			t.Fatal("editor.splitDown was unavailable")
		}
		if m.split == nil || !m.splitDown {
			t.Fatalf("down split state: split=%v down=%v", m.split != nil, m.splitDown)
		}
	})

	t.Run("open and move between groups", func(t *testing.T) {
		m := editorGroupModel()
		if !m.RunAction("editor.openInOppositeGroup") {
			t.Fatal("editor.openInOppositeGroup was unavailable")
		}
		if m.split == nil || len(m.allTabs()) != 3 {
			t.Fatalf("open opposite: split=%v tabs=%d", m.split != nil, len(m.allTabs()))
		}

		m = editorGroupModel()
		m.split = &splitGroup{tabs: []*Tab{editorGroupTab("/project/Side.java", "class Side {}\n")}}
		if !m.RunAction("editor.moveToOppositeGroup") {
			t.Fatal("editor.moveToOppositeGroup was unavailable")
		}
		if m.split == nil || len(m.allTabs()) != 3 {
			t.Fatalf("move opposite: split=%v tabs=%d", m.split != nil, len(m.allTabs()))
		}
	})

	t.Run("next and previous splitter", func(t *testing.T) {
		m := editorGroupModel()
		m.split = &splitGroup{tabs: []*Tab{editorGroupTab("/project/Side.java", "class Side {}\n")}}
		before := m.splitFirst
		if !m.RunAction("editor.nextSplitter") {
			t.Fatal("editor.nextSplitter was unavailable")
		}
		if m.splitFirst == before {
			t.Fatal("next splitter did not focus the other group")
		}
		if !m.RunAction("editor.prevSplitter") {
			t.Fatal("editor.prevSplitter was unavailable")
		}
		if m.splitFirst != before {
			t.Fatal("previous splitter did not return to the first group")
		}
	})

	t.Run("unsplit focused group", func(t *testing.T) {
		m := editorGroupModel()
		m.split = &splitGroup{tabs: []*Tab{editorGroupTab("/project/Side.java", "class Side {}\n")}}
		if !m.RunAction("editor.unsplit") {
			t.Fatal("editor.unsplit was unavailable")
		}
		if m.split != nil || len(m.tabs) != 1 || tabPath(m.tabs[0]) != "/project/Side.java" {
			t.Fatalf("unsplit: split=%v tabs=%v", m.split != nil, tabPaths(m.tabs))
		}
	})

	t.Run("unsplit all", func(t *testing.T) {
		m := editorGroupModel()
		m.split = &splitGroup{tabs: []*Tab{editorGroupTab("/project/Side.java", "class Side {}\n")}}
		if !m.RunAction("editor.unsplitAll") {
			t.Fatal("editor.unsplitAll was unavailable")
		}
		if m.split != nil || len(m.allTabs()) != 2 {
			t.Fatalf("unsplit all: split=%v tabs=%d", m.split != nil, len(m.allTabs()))
		}
	})
}

func tabPaths(tabs []*Tab) []string {
	out := make([]string, 0, len(tabs))
	for _, tab := range tabs {
		out = append(out, tabPath(tab))
	}
	return out
}
