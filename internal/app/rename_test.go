package app

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"goidea/internal/openidea"
)

func TestRenameEditInfo(t *testing.T) {
	raw := json.RawMessage(`{"files":[` +
		`{"kind":"edit","path":"/p/A.java","edits":[]},` +
		`{"kind":"rename","from":"/p/B.java","to":"/p/C.java"},` +
		`{"kind":"create","path":"/p/D.java","overwrite":false}` +
		`]}`)
	info := renameEditInfo(raw)
	if info.Files != 3 || info.Moves != 2 {
		t.Fatalf("info = %+v, want 3 files and 2 moves", info)
	}
	if got := renameEditInfo(nil); got.Files != 0 || got.Moves != 0 {
		t.Fatalf("nil edit = %+v", got)
	}
	if got := renameEditInfo(json.RawMessage(`not json`)); got.Files != 0 {
		t.Fatalf("bad edit = %+v", got)
	}
	if got := renameEditInfo(json.RawMessage(`{"files":[]}`)); got.Files != 0 || got.Moves != 0 {
		t.Fatalf("empty edit = %+v", got)
	}
}

func TestRenameIncompleteText(t *testing.T) {
	cases := map[string]string{
		"indexing":     "索引",
		"tooManyFiles": "过多",
		"filesSkipped": "未检查",
		"serverFailed": "失败",
	}
	for kind, want := range cases {
		kind := kind
		if got := renameIncompleteText(&kind); !strings.Contains(got, want) {
			t.Fatalf("%s: %q does not contain %q", kind, got, want)
		}
	}
	if got := renameIncompleteText(nil); got != "" {
		t.Fatalf("complete rename warns %q", got)
	}
	odd := "somethingNew"
	if got := renameIncompleteText(&odd); !strings.Contains(got, odd) {
		t.Fatalf("unknown reason lost: %q", got)
	}
}

func TestRenameErrorText(t *testing.T) {
	stale := &openidea.Error{Kind: "outOfSync", Message: "document changed"}
	if got := renameErrorText(stale); !strings.Contains(got, "重新发起") {
		t.Fatalf("out of sync says %q", got)
	}
	missing := &openidea.Error{Kind: "notFound", Message: "no server"}
	if got := renameErrorText(missing); !strings.Contains(got, "语言服务器") {
		t.Fatalf("not found says %q", got)
	}
	plain := errors.New("boom")
	if got := renameErrorText(plain); got != "boom" {
		t.Fatalf("plain error says %q", got)
	}
}
