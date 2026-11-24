package openidea_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIntegrationMCPWindowReports(t *testing.T) {
	m := launchForTest(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "src", "Main.java")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("class Main {\n  String value = \"😀\";\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	const window = "goidea-mcp-it"
	openProject(t, m, window, dir)

	for _, command := range []string{
		"mcp_set_open_editors",
		"mcp_set_focused_editor",
		"mcp_report_problems",
	} {
		if !m.Server().HasCommand(command) {
			t.Fatalf("server does not offer %s", command)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := m.Invoke(ctx, window, "mcp_set_open_editors", map[string]any{
		"paths": []string{path},
	}); err != nil {
		t.Fatalf("mcp_set_open_editors: %v", err)
	}
	if _, err := m.Invoke(ctx, window, "mcp_set_focused_editor", map[string]any{
		"editor": map[string]any{
			"path": path,
			"selection": map[string]any{
				"start": map[string]any{"line": 1, "col": 2},
				"end":   map[string]any{"line": 1, "col": 7},
			},
			"selectedText": "value",
		},
		"focused": true,
	}); err != nil {
		t.Fatalf("mcp_set_focused_editor: %v", err)
	}
	if _, err := m.Invoke(ctx, window, "mcp_report_problems", map[string]any{
		"reset": true,
		"files": []map[string]any{{
			"path": path,
			"items": []map[string]any{{
				"range": map[string]any{
					"start": map[string]any{"line": 1, "col": 2},
					"end":   map[string]any{"line": 1, "col": 7},
				},
				"severity": "warning",
				"message":  "integration warning",
			}},
		}},
	}); err != nil {
		t.Fatalf("mcp_report_problems reset: %v", err)
	}
	if _, err := m.Invoke(ctx, window, "mcp_report_problems", map[string]any{
		"reset": false,
		"files": []map[string]any{{"path": path, "items": []any{}}},
	}); err != nil {
		t.Fatalf("mcp_report_problems removal: %v", err)
	}
}
