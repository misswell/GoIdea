package docbridge

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"goidea/internal/openidea"
	"goidea/internal/position"
)

func eventNotification(t *testing.T, window string, event any) openidea.Notification {
	t.Helper()
	params, err := json.Marshal(map[string]any{"window": window, "event": event})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	return openidea.Notification{Method: "event", Params: params}
}

func TestRegistrySharesOneDocumentPerPath(t *testing.T) {
	b := newFakeBackend()
	disk := b.add("/tmp/demo/Shared.java", sampleJava)
	r := NewRegistry(b, "w1")
	ctx := context.Background()

	d1, err := r.Open(ctx, disk.path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	d2, err := r.Open(ctx, disk.path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if d1 != d2 {
		t.Fatal("two tabs of one file got different documents")
	}
	if got := b.opens; got != 1 {
		t.Fatalf("backend opened the file %d times, want 1", got)
	}
	if got := r.Get(disk.path); got != d1 {
		t.Fatal("Get did not return the open document")
	}
	if got := r.ByID(d1.ID()); got != d1 {
		t.Fatal("ByID did not return the open document")
	}
	if got := len(r.Documents()); got != 1 {
		t.Fatalf("Documents has %d entries, want 1", got)
	}

	if err := r.Close(ctx, disk.path); err != nil {
		t.Fatalf("close first ref: %v", err)
	}
	if b.closes != 0 {
		t.Fatal("the backend document closed while a tab still shows it")
	}
	if r.Get(disk.path) != d1 {
		t.Fatal("document vanished with a tab still open")
	}
	if err := r.Close(ctx, disk.path); err != nil {
		t.Fatalf("close last ref: %v", err)
	}
	if b.closes != 1 {
		t.Fatalf("backend closes %d, want 1", b.closes)
	}
	if r.Get(disk.path) != nil || r.ByID(d1.ID()) != nil {
		t.Fatal("registry still lists a closed document")
	}
}

func TestRegistryAppliesDocPatched(t *testing.T) {
	b := newFakeBackend()
	disk := b.add("/tmp/demo/Patched.java", sampleJava)
	r := NewRegistry(b, "w1")
	ctx := context.Background()
	doc, err := r.Open(ctx, disk.path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	text := doc.Text()
	at := strings.Index(text, "世界")
	from := position.UTF16Length(text[:at])
	p := b.edit(disk.id, from, from+2, "地球")
	n := eventNotification(t, "w1", map[string]any{
		"type":        "docPatched",
		"doc":         disk.id,
		"baseVersion": p.BaseVersion,
		"version":     p.Version,
		"changes":     p.Changes,
	})
	if !r.HandleNotification(ctx, n) {
		t.Fatal("docPatched was not recognised")
	}
	if !strings.Contains(doc.Text(), "地球") || doc.Text() != b.text(disk.id) {
		t.Fatalf("patch not applied: buffer %q backend %q", doc.Text(), b.text(disk.id))
	}
	if doc.Version() != p.Version {
		t.Fatalf("version %d, want %d", doc.Version(), p.Version)
	}
	requireMapper(t, doc)

	// Events for another window must not touch this window's documents.
	doc.SetDirty(false)
	other := eventNotification(t, "w2", map[string]any{
		"type": "docDirty", "doc": disk.id, "dirty": true,
	})
	r.HandleNotification(ctx, other)
	if doc.Dirty() {
		t.Fatal("an event from another window changed the document")
	}

	dirty := eventNotification(t, "w1", map[string]any{
		"type": "docDirty", "doc": disk.id, "dirty": true,
	})
	r.HandleNotification(ctx, dirty)
	if !doc.Dirty() {
		t.Fatal("docDirty was not applied")
	}

	if r.HandleNotification(ctx, openidea.Notification{Method: "channel"}) {
		t.Fatal("a non-event notification was reported as a document event")
	}
}

func TestRegistryReopenAllKeepsUnsavedText(t *testing.T) {
	b := newFakeBackend()
	disk := b.add("/tmp/demo/Restart.java", sampleJava)
	r := NewRegistry(b, "w1")
	ctx := context.Background()
	doc, err := r.Open(ctx, disk.path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	text := doc.Text()
	at := strings.Index(text, "}\n")
	if err := doc.ApplyEdits(ctx, editorEdit(at, at, "// unsaved\n")); err != nil {
		t.Fatalf("edit: %v", err)
	}
	want := doc.Text()
	oldID := doc.ID()

	b.restart()
	if err := r.ReopenAll(ctx); err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if doc.ID() == oldID {
		t.Fatal("document id was not refreshed after the restart")
	}
	if got := doc.Text(); got != want || !strings.Contains(got, "// unsaved\n") {
		t.Fatalf("unsaved text lost: %q", got)
	}
	if got := b.text(doc.ID()); got != want {
		t.Fatalf("backend %q, want %q", got, want)
	}
	if r.ByID(doc.ID()) != doc || r.ByID(oldID) != nil {
		t.Fatal("registry id index not updated")
	}
	requireMapper(t, doc)

	r.CloseAll(ctx)
	if got := len(r.Documents()); got != 0 {
		t.Fatalf("%d documents left after CloseAll", got)
	}
}
