package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInvalidationScheduleAppliesAtStart(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "server")
	mk := func(rel string) {
		p := filepath.Join(home, rel)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p, "keep"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, rel := range []string{"data/history", "data/cache", "data/lsp/jdtls", "data/tools/jdtls"} {
		mk(rel)
	}
	cfg := Config{SettingsPath: filepath.Join(dir, "settings.json"), Home: home}
	if err := writeInvalidationSchedule(dir, &InvalidateOptions{LocalHistory: true, LanguageServers: true}); err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if _, err := os.Stat(invalidationMarker(dir)); err != nil {
		t.Fatalf("marker missing: %v", err)
	}
	ApplyPendingInvalidation(cfg)
	for _, rel := range []string{"data/history", "data/lsp/jdtls"} {
		if _, err := os.Stat(filepath.Join(home, rel)); !os.IsNotExist(err) {
			t.Fatalf("%s survived (err %v)", rel, err)
		}
	}
	for _, rel := range []string{"data/cache", "data/tools/jdtls"} {
		if _, err := os.Stat(filepath.Join(home, rel)); err != nil {
			t.Fatalf("%s should stay: %v", rel, err)
		}
	}
	if _, err := os.Stat(invalidationMarker(dir)); !os.IsNotExist(err) {
		t.Fatalf("marker stayed after the start applied it")
	}
}

func TestRestartScheduleDeletesNothing(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "server")
	cache := filepath.Join(home, "data", "cache")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeInvalidationSchedule(dir, nil); err != nil {
		t.Fatalf("schedule: %v", err)
	}
	ApplyPendingInvalidation(Config{SettingsPath: filepath.Join(dir, "settings.json"), Home: home})
	if _, err := os.Stat(cache); err != nil {
		t.Fatalf("Just Restart deleted a cache: %v", err)
	}
	if _, err := os.Stat(invalidationMarker(dir)); !os.IsNotExist(err) {
		t.Fatalf("marker stayed")
	}
}

func TestInvalidateDialogStagesAndCloses(t *testing.T) {
	m := &Model{}
	m.OpenInvalidateCaches()
	v := m.invalidateDlg
	if v == nil || !v.open {
		t.Fatalf("dialog = %+v, want open", v)
	}
	if v.localHistory || v.languageServers || v.buildCaches {
		t.Fatalf("defaults = %+v, want every optional cache off", v)
	}
	v.localHistory = true
	v.buildCaches = true
	if !v.open {
		t.Fatalf("staging closed the dialog")
	}
	m.closeInvalidateCaches()
	if m.invalidateDlg != nil {
		t.Fatalf("dialog stayed after Cancel: %+v", m.invalidateDlg)
	}
}
