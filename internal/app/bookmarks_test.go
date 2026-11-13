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

func TestTogglePathBookmarkAddsAndRemoves(t *testing.T) {
	list, added := togglePathBookmark(nil, "/p/b.java", false)
	if !added || len(list) != 1 {
		t.Fatalf("add = %v, %v; want one file", list, added)
	}
	list, added = togglePathBookmark(list, "/p/a.java", false)
	if !added || len(list) != 2 || list[0].Path != "/p/a.java" {
		t.Fatalf("sorted = %v, want a.java first", list)
	}
	list, added = togglePathBookmark(list, "/p/b.java", false)
	if added || len(list) != 1 || list[0].Path != "/p/a.java" {
		t.Fatalf("remove = %v, %v; want only a.java", list, added)
	}
}

func TestTogglePathBookmarkKeepsTheFolderFlag(t *testing.T) {
	list, added := togglePathBookmark(nil, "/p/src", true)
	if !added || len(list) != 1 || !list[0].Dir {
		t.Fatalf("add = %v, %v; want one folder", list, added)
	}
}

func TestPathBookmarksInProjectDropsMissingAndOutside(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "A.java")
	if err := os.WriteFile(inside, []byte("class A {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "B.java")
	if err := os.WriteFile(outside, []byte("class B {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "src")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	list := []PathBookmark{
		{Path: inside}, {Path: outside}, {Path: dir, Dir: true},
		{Path: filepath.Join(root, "gone.java")}, {Path: inside},
		// The folder flag has to match what is on disk.
		{Path: dir}, {Path: inside, Dir: true},
	}
	got := pathBookmarksInProject(list, root)
	if len(got) != 2 || got[0].Path != inside || got[1].Path != dir || !got[1].Dir {
		t.Fatalf("pathBookmarksInProject = %v, want the file and the folder", got)
	}
}

func TestMoveBookmarkToList(t *testing.T) {
	list := []Bookmark{
		{Path: "/p/a.java", Line: 1},
		{Path: "/p/a.java", Line: 2, List: "list2"},
	}
	got := moveBookmarkToList(list, "/p/a.java", 2, "list3")
	if got[1].List != "list3" || got[0].List != "" {
		t.Fatalf("move = %v, want only line 2 in list3", got)
	}
	if list[1].List != "list2" {
		t.Fatal("the move changed the caller's list")
	}
	paths := []PathBookmark{{Path: "/p/a.java"}, {Path: "/p/dir", Dir: true}}
	gotPaths := movePathBookmarkToList(paths, "/p/dir", "list3")
	if gotPaths[1].List != "list3" || gotPaths[0].List != "" {
		t.Fatalf("path move = %v, want only the folder in list3", gotPaths)
	}
}

func TestAddBookmarkList(t *testing.T) {
	lists := normalizeBookmarkLists(nil)
	if len(lists) != 1 || lists[0].ID != bookmarkDefaultList {
		t.Fatalf("normalize(nil) = %v, want only the default list", lists)
	}
	lists, id, err := addBookmarkList(lists, " Work ")
	if err != "" || id != "list2" || len(lists) != 2 || lists[1].Name != "Work" {
		t.Fatalf("add = %v, %q, %q; want Work as list2", lists, id, err)
	}
	if _, _, err := addBookmarkList(lists, "Work"); err == "" {
		t.Fatal("a duplicate name was accepted")
	}
	if _, _, err := addBookmarkList(lists, "  "); err == "" {
		t.Fatal("an empty name was accepted")
	}
}

func TestBookmarksInList(t *testing.T) {
	list := []Bookmark{
		{Path: "/p/a.java", Line: 1},
		{Path: "/p/a.java", Line: 2, List: "list2"},
		{Path: "/p/a.java", Line: 3, List: bookmarkDefaultList},
	}
	if got := bookmarksInList(list, bookmarkDefaultList); len(got) != 2 {
		t.Fatalf("default list = %v, want the two unlisted bookmarks", got)
	}
	if got := bookmarksInList(list, "list2"); len(got) != 1 || got[0].Line != 2 {
		t.Fatalf("list2 = %v, want line 2", got)
	}
}

func TestRenameBookmarkList(t *testing.T) {
	lists, _, err := addBookmarkList(normalizeBookmarkLists(nil), "Work")
	if err != "" {
		t.Fatal(err)
	}
	got, name, err := renameBookmarkList(lists, "list2", " Home ")
	if err != "" || name != "Home" || got[1].Name != "Home" || got[0].Name != "书签" {
		t.Fatalf("rename = %v, %q, %q; want list2 as Home", got, name, err)
	}
	if _, _, err := renameBookmarkList(lists, "list2", "书签"); err == "" {
		t.Fatal("a duplicate name was accepted")
	}
	if _, _, err := renameBookmarkList(lists, "list2", " "); err == "" {
		t.Fatal("an empty name was accepted")
	}
	if _, _, err := renameBookmarkList(lists, "nope", "Home"); err == "" {
		t.Fatal("a missing list was renamed")
	}
}

func TestRemoveBookmarkList(t *testing.T) {
	lists, _, err := addBookmarkList(normalizeBookmarkLists(nil), "Work")
	if err != "" {
		t.Fatal(err)
	}
	marks := []Bookmark{
		{Path: "/p/a.java", Line: 1, List: "list2"},
		{Path: "/p/a.java", Line: 2},
	}
	paths := []PathBookmark{
		{Path: "/p/A.java", List: "list2"},
		{Path: "/p/B.java"},
	}
	got, keptMarks, keptPaths, def := removeBookmarkList(lists, marks, paths, "list2", "list2", "书签")
	if len(got) != 1 || got[0].ID != bookmarkDefaultList || def != bookmarkDefaultList {
		t.Fatalf("remove list2 = %v default %q, want the default list", got, def)
	}
	if len(keptMarks) != 1 || keptMarks[0].Line != 2 || len(keptPaths) != 1 || keptPaths[0].Path != "/p/B.java" {
		t.Fatalf("marks %v paths %v, want the default list's bookmarks", keptMarks, keptPaths)
	}
	// The last list cannot vanish: a fresh default replaces it.
	got, _, _, def = removeBookmarkList([]BookmarkList{{ID: bookmarkDefaultList, Name: "书签"}},
		marks[1:], nil, bookmarkDefaultList, bookmarkDefaultList, "书签")
	if len(got) != 1 || got[0].ID != bookmarkDefaultList || def != bookmarkDefaultList {
		t.Fatalf("remove the only list = %v default %q, want a fresh default", got, def)
	}
}

func TestBookmarkPopupRows(t *testing.T) {
	lists := []BookmarkList{{ID: bookmarkDefaultList, Name: "书签"}, {ID: "list2", Name: "Work"}}
	marks := []Bookmark{
		{Path: "/p/a.java", Line: 2, List: "list2"},
		{Path: "/p/a.java", Line: 1},
	}
	paths := []PathBookmark{{Path: "/p/A.java", List: "list2"}}
	rows := bookmarkPopupRows(lists, marks, paths)
	var headers int
	for _, r := range rows {
		if !r.IsItem {
			headers++
		}
	}
	if headers != 2 {
		t.Fatalf("headers = %d, want one per non-empty list", headers)
	}
	items := bookmarkPopupItems(rows)
	if len(items) != 3 {
		t.Fatalf("items = %v, want the file and both lines", items)
	}
	if !items[1].File || items[1].Path != "/p/A.java" {
		t.Fatalf("items = %v, want the file first inside Work", items)
	}
	// Empty lists are left out, and nothing is listed without bookmarks.
	if rows := bookmarkPopupRows([]BookmarkList{{ID: bookmarkDefaultList, Name: "书签"}}, nil, nil); len(rows) != 0 {
		t.Fatalf("empty rows = %v, want none", rows)
	}
}

func TestSetBookmarkDescription(t *testing.T) {
	list := []Bookmark{{Path: "/p/a.java", Line: 1, Text: "class A {"}}
	got, ok := setBookmarkDescription(list, "/p/a.java", 1, "  the class  ")
	if !ok || len(got) != 1 || got[0].Description != "the class" || got[0].Text != "class A {" {
		t.Fatalf("described = %v, %v; want the trimmed description", got, ok)
	}
	if list[0].Description != "" {
		t.Fatal("the original list was changed")
	}
	if _, ok := setBookmarkDescription(list, "/p/a.java", 2, "x"); ok {
		t.Fatal("a line without a bookmark was described")
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
	if err := (Session{Root: root, Bookmarks: []Bookmark{want},
		PathBookmarks: []PathBookmark{{Path: file}},
		BookmarkLists: []BookmarkList{{ID: "list2", Name: "Work"}}, DefaultList: "list2"}).Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got := LoadSession(path)
	if got.Root != root || len(got.Bookmarks) != 1 || got.Bookmarks[0] != want {
		t.Fatalf("LoadSession = %+v, want bookmark %+v", got, want)
	}
	if len(got.PathBookmarks) != 1 || got.PathBookmarks[0].Path != file {
		t.Fatalf("LoadSession path bookmarks = %+v, want %s", got.PathBookmarks, file)
	}
	if len(got.BookmarkLists) != 1 || got.BookmarkLists[0].Name != "Work" ||
		got.DefaultList != "list2" {
		t.Fatalf("LoadSession lists = %+v, default %q; want Work", got.BookmarkLists, got.DefaultList)
	}
}
