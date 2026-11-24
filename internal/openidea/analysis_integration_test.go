package openidea_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mayoi/internal/openidea"
)

type structuralTestSummary struct {
	Files   uint32 `json:"files"`
	Matches uint32 `json:"matches"`
}

type structuralTestEvent struct {
	Type    string                `json:"type"`
	Summary structuralTestSummary `json:"summary"`
}

func TestIntegrationStructuralSearchRunsTwice(t *testing.T) {
	m := launchForTest(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "src", "demo", "Structural.java")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	source := `package demo;

class Structural {
    void print() {
        System.out.println("first");
        System.out.println("second");
    }
}
`
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	const window = "goidea-structural-rerun"
	openProject(t, m, window, dir)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := m.Invoke(ctx, window, "project_trust_set", map[string]any{
		"path": dir, "trusted": true, "folder": false,
	}); err != nil {
		t.Fatalf("trust project: %v", err)
	}

	query := map[string]any{
		"pattern":     "System.out.println($x$);",
		"language":    "java",
		"replacement": "System.out.print($x$);",
		"maxMatches":  100,
	}
	if _, err := m.Invoke(ctx, window, "structural_validate", map[string]any{
		"query": query,
	}); err != nil {
		t.Fatalf("validate structural query: %v", err)
	}

	notes, stop := m.Listen(64)
	defer stop()
	for i, request := range []uint32{101, 102} {
		raw, err := m.Invoke(ctx, window, "structural_search", map[string]any{
			"request": request,
			"scope":   map[string]any{"kind": "project"},
			"query":   query,
			"onEvent": map[string]any{"$channel": request},
		})
		if err != nil {
			t.Fatalf("structural search %d: %v (reply %s)", i+1, err, raw)
		}
		summary := waitStructuralDone(t, ctx, notes, request)
		if summary.Matches != 2 {
			t.Fatalf("structural search %d matches = %d, want 2", i+1, summary.Matches)
		}
	}
}

func waitStructuralDone(
	t *testing.T,
	ctx context.Context,
	notes <-chan openidea.Notification,
	channel uint32,
) structuralTestSummary {
	t.Helper()
	for {
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case note := <-notes:
			if note.Method != "channel" {
				continue
			}
			var message openidea.ChannelMessage
			if err := json.Unmarshal(note.Params, &message); err != nil || message.ID != channel {
				continue
			}
			var event structuralTestEvent
			if err := json.Unmarshal(message.Data, &event); err != nil {
				t.Fatalf("decode structural event: %v", err)
			}
			if event.Type == "done" {
				return event.Summary
			}
		}
	}
}
