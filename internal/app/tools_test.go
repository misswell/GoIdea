package app

import (
	"encoding/json"
	"strings"
	"testing"

	"mayoi/internal/openidea"
)

func TestJavaToolRowsKeepsTheJavaToolsInOrder(t *testing.T) {
	list := []openidea.ToolInfo{
		{Name: "gopls", DisplayName: "gopls", Status: "onPath"},
		{Name: "java-debug", DisplayName: "Java Debugger", Status: "missing", CanInstall: true},
		{Name: "jdtls", DisplayName: "Eclipse JDT LS", Status: "installed", Path: strp("/x/bin/jdtls"), Version: strp("1.63.0")},
	}
	rows := javaToolRows(list)
	if len(rows) != 2 || rows[0].Name != "jdtls" || rows[1].Name != "java-debug" {
		t.Fatalf("rows %+v", rows)
	}
	if rows[0].CanInstall || rows[0].Version != "1.63.0" || rows[0].Path == "" {
		t.Fatalf("installed row %+v", rows[0])
	}
	if !rows[1].CanInstall {
		t.Fatalf("missing row cannot be installed: %+v", rows[1])
	}
	missing := javaToolsMissing(rows)
	if len(missing) != 1 || missing[0].Name != "java-debug" {
		t.Fatalf("missing %+v", missing)
	}
}

func TestAcceptToolEventKeepsOnlyItsChannel(t *testing.T) {
	m := &Model{toolsChannel: 9}
	m.acceptChunk(8, json.RawMessage(`{"type":"line","text":"stolen"}`))
	if len(m.toolsLog) != 0 {
		t.Fatal("another channel's install output was kept")
	}
	m.acceptChunk(9, json.RawMessage(`{"type":"line","text":"Downloading\njdtls"}`))
	m.acceptChunk(9, json.RawMessage(`{"type":"progress","fraction":0.5,"message":"Downloaded 1 of 2 MB"}`))
	if len(m.toolsLog) != 2 || m.toolsLog[1] != "jdtls" {
		t.Fatalf("log %q", m.toolsLog)
	}
	if m.toolsProgress != "Downloaded 1 of 2 MB" {
		t.Fatalf("progress %q", m.toolsProgress)
	}
}

func TestToolsLogIsBounded(t *testing.T) {
	m := &Model{toolsChannel: 1}
	var lines strings.Builder
	for i := 0; i < toolsLogLimit+20; i++ {
		lines.WriteString("line\n")
	}
	ev := mustJSON(t, map[string]any{"type": "line", "text": lines.String()})
	m.acceptChunk(1, ev)
	if len(m.toolsLog) != toolsLogLimit {
		t.Fatalf("kept %d lines", len(m.toolsLog))
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
