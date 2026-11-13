package app

import (
	"encoding/json"
	"testing"
)

func TestParseIntentActions(t *testing.T) {
	raw := []json.RawMessage{
		json.RawMessage(`{"title":"Create method 'smoke()'","kind":"quickfix","isPreferred":true,"disabledReason":null}`),
		json.RawMessage(`{"title":"Inline variable","kind":"refactor.inline","isPreferred":false,"disabledReason":"Cannot inline"}`),
		json.RawMessage(`{"title":"","kind":"quickfix","isPreferred":false}`),
		json.RawMessage(`not json`),
	}
	items := parseIntentActions(raw)
	if len(items) != 2 {
		t.Fatalf("actions = %d, want 2", len(items))
	}
	first := items[0]
	if first.title != "Create method 'smoke()'" || first.kind != "quickfix" || !first.preferred || first.disabled != "" {
		t.Errorf("first = %+v", first)
	}
	if string(first.raw) != string(raw[0]) {
		t.Errorf("raw = %s, want %s", first.raw, raw[0])
	}
	second := items[1]
	if second.preferred || second.disabled != "Cannot inline" || second.kind != "refactor.inline" {
		t.Errorf("second = %+v", second)
	}
}

func TestParseIntentActionsWithoutKind(t *testing.T) {
	items := parseIntentActions([]json.RawMessage{
		json.RawMessage(`{"title":"Organize imports","isPreferred":false}`),
	})
	if len(items) != 1 || items[0].kind != "" {
		t.Fatalf("items = %+v", items)
	}
}

func TestIntentKindLabel(t *testing.T) {
	cases := map[string]string{
		"":                       "操作",
		"quickfix":               "快速修复",
		"refactor":               "重构",
		"refactor.extract":       "提取",
		"refactor.inline":        "内联",
		"refactor.move":          "移动",
		"refactor.rewrite":       "改写",
		"refactor.rewrite.var":   "改写",
		"source":                 "源码",
		"source.organizeImports": "整理导入",
		"source.fixAll":          "修复全部",
		"other.kind":             "操作",
	}
	for kind, want := range cases {
		if got := intentKindLabel(kind); got != want {
			t.Errorf("intentKindLabel(%q) = %q, want %q", kind, got, want)
		}
	}
}
