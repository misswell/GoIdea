package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestToggleBookmarkAddsAndRemoves(t *testing.T) {
	list, added := toggleBookmark(nil, Bookmark{Path: "/p/b.java", Line: 4})
	if !added || len(list) != 1 {
		t.Fatalf("add = %v, %v; want one bookmark", list, added)
	}
	list, added = toggleBookmark(list, Bookmark{Path: "/p/a.java", Line: 9})
	if !added || len(list) != 2 || list[0].Path != "/p/a.java" || list[1].Path != "/p/b.java" {
		t.Fatalf("sorted list = %v, want a.java before b.java", list)
	}
	list, added = toggleBookmark(list, Bookmark{Path: "/p/b.java", Line: 4})
	if added || len(list) != 1 || list[0].Path != "/p/a.java" {
		t.Fatalf("remove = %v, %v; want only a.java", list, added)
	}
}

func TestStepBookmarkWraps(t *testing.T) {
	list := []Bookmark{
		{Path: "/p/b.java", Line: 1},
		{Path: "/p/a.java", Line: 9},
		{Path: "/p/a.java", Line: 2},
	}
	b, ok := stepBookmark(list, "/p/a.java", 0, 1)
	if !ok || b.Path != "/p/a.java" || b.Line != 2 {
		t.Fatalf("next = %+v, %v; want a.java:2", b, ok)
	}
	b, ok = stepBookmark(list, "/p/a.java", 2, 1)
	if !ok || b.Path != "/p/a.java" || b.Line != 9 {
		t.Fatalf("next from 2 = %+v, %v; want a.java:9", b, ok)
	}
	b, ok = stepBookmark(list, "/p/b.java", 1, 1)
	if !ok || b.Path != "/p/a.java" || b.Line != 2 {
		t.Fatalf("next wraps = %+v, %v; want a.java:2", b, ok)
	}
	b, ok = stepBookmark(list, "/p/a.java", 2, -1)
	if !ok || b.Path != "/p/b.java" || b.Line != 1 {
		t.Fatalf("prev wraps = %+v, %v; want b.java:1", b, ok)
	}
	if _, ok := stepBookmark(nil, "", -1, 1); ok {
		t.Fatal("stepping an empty list found a bookmark")
	}
}

func TestBookmarkTextIsClipped(t *testing.T) {
	if got := bookmarkLineText("one\n  two words  \nthree", 1); got != "two words" {
		t.Fatalf("line text = %q, want %q", got, "two words")
	}
	if got := bookmarkLineText("one", 5); got != "" {
		t.Fatalf("out of range line = %q, want empty", got)
	}
	long := make([]rune, maxBookmarkText+20)
	for i := range long {
		long[i] = 'x'
	}
	if got := clipBookmarkText(string(long)); len([]rune(got)) != maxBookmarkText {
		t.Fatalf("clipped to %d runes, want %d", len([]rune(got)), maxBookmarkText)
	}
}

func TestBookmarksInProjectDropsMissingAndOutside(t *testing.T) {
	root := t.TempDir()
	keep := filepath.Join(root, "keep.java")
	if err := os.WriteFile(keep, []byte("class Keep {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.java")
	if err := os.WriteFile(outside, []byte("class Out {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	list := bookmarksInProject([]Bookmark{
		{Path: keep, Line: 3},
		{Path: keep, Line: 1},
		{Path: keep, Line: 3},
		{Path: filepath.Join(root, "gone.java"), Line: 1},
		{Path: outside, Line: 1},
		{Path: "", Line: 1},
		{Path: keep, Line: -2},
	}, root)
	if len(list) != 2 || list[0].Line != 1 || list[1].Line != 3 {
		t.Fatalf("kept %+v, want the two in-project lines sorted", list)
	}
}

func TestAssignBookmarkMnemonicTakesTheKey(t *testing.T) {
	first := Bookmark{Path: "/p/a.java", Line: 1, Text: "one"}
	second := Bookmark{Path: "/p/a.java", Line: 2, Text: "two"}
	list := assignBookmarkMnemonic(nil, first, "1")
	if len(list) != 1 || list[0].Mnemonic != "1" || list[0].Text != "one" {
		t.Fatalf("assign created %+v, want a.java:1 with key 1", list)
	}
	list = assignBookmarkMnemonic(list, second, "1")
	if len(list) != 2 || list[0].Mnemonic != "" || list[1].Mnemonic != "1" {
		t.Fatalf("moving key = %+v, want only a.java:2 to keep it", list)
	}
	list = assignBookmarkMnemonic(list, second, "")
	if len(list) != 2 || list[0].Mnemonic != "" || list[1].Mnemonic != "" {
		t.Fatalf("clear = %+v, want both bookmarks without keys", list)
	}
}

func TestToggleBookmarkMnemonic(t *testing.T) {
	b := Bookmark{Path: "/p/a.java", Line: 3, Text: "three"}
	list, added := toggleBookmarkMnemonic(nil, b, "A")
	if !added || len(list) != 1 || list[0].Mnemonic != "A" {
		t.Fatalf("toggle add = %+v, %v; want a.java:3 with key A", list, added)
	}
	list, added = toggleBookmarkMnemonic(list, b, "A")
	if added || len(list) != 0 {
		t.Fatalf("toggle same key = %+v, %v; want the bookmark gone", list, added)
	}
	list = assignBookmarkMnemonic(list, b, "A")
	list, added = toggleBookmarkMnemonic(list, b, "B")
	if !added || len(list) != 1 || list[0].Mnemonic != "B" {
		t.Fatalf("toggle other key = %+v, %v; want key B", list, added)
	}
}

func TestBookmarkByMnemonic(t *testing.T) {
	list := []Bookmark{
		{Path: "/p/a.java", Line: 1},
		{Path: "/p/b.java", Line: 2, Mnemonic: "9"},
	}
	if b, ok := bookmarkByMnemonic(list, "9"); !ok || b.Path != "/p/b.java" {
		t.Fatalf("lookup 9 = %+v, %v; want b.java:2", b, ok)
	}
	for _, key := range []string{"", "1", "Z"} {
		if _, ok := bookmarkByMnemonic(list, key); ok {
			t.Fatalf("lookup %q found a bookmark", key)
		}
	}
}

func TestSessionRoundTripsBookmarks(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "A.java")
	if err := os.WriteFile(file, []byte("class A {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "session.json")
	want := Bookmark{Path: file, Line: 0, Text: "class A {}", Description: "the class"}
	if err := (Session{Root: root, Bookmarks: []Bookmark{want}}).Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got := LoadSession(path)
	if got.Root != root || len(got.Bookmarks) != 1 || got.Bookmarks[0] != want {
		t.Fatalf("LoadSession = %+v, want bookmark %+v", got, want)
	}
}
