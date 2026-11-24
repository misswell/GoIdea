package openidea_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIntegrationGlobalSettingsAndJetBrainsImport(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ideDir := filepath.Join(home, "Library", "Application Support", "JetBrains", "IntelliJIdea")
	if err := os.MkdirAll(filepath.Join(ideDir, "options"), 0o755); err != nil {
		t.Fatal(err)
	}
	editorXML := `<application><component name="EditorSettings"><option name="IS_WIDE" value="true"/></component></application>`
	if err := os.WriteFile(filepath.Join(ideDir, "options", "editor.xml"), []byte(editorXML), 0o644); err != nil {
		t.Fatal(err)
	}

	m := launchForTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	raw, err := m.Invoke(ctx, "settings-window", "settings_get_global", nil)
	if err != nil {
		t.Fatalf("settings_get_global: %v", err)
	}
	var global map[string]any
	if err := json.Unmarshal(raw, &global); err != nil || global == nil {
		t.Fatalf("global settings: err=%v raw=%s", err, raw)
	}

	bundle := filepath.Join(home, "goidea-settings.json")
	if _, err := m.Invoke(ctx, "settings-window", "settings_export", map[string]any{
		"path": bundle,
		"storage": map[string]string{
			"goidea.preferences": `{"theme":"dark"}`,
		},
	}); err != nil {
		t.Fatalf("settings_export: %v", err)
	}
	var exported struct {
		Format  string            `json:"format"`
		Storage map[string]string `json:"storage"`
	}
	exportedRaw, err := os.ReadFile(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(exportedRaw, &exported); err != nil {
		t.Fatal(err)
	}
	if exported.Format != "openidea-settings" || exported.Storage["goidea.preferences"] != `{"theme":"dark"}` {
		t.Fatalf("exported bundle = %+v", exported)
	}

	if _, err := m.Invoke(ctx, "settings-window", "settings_import", map[string]any{
		"path": bundle,
	}); err != nil {
		t.Fatalf("settings_import: %v", err)
	}

	choicesRaw, err := m.Invoke(ctx, "settings-window", "settings_list_jetbrains_ides", nil)
	if err != nil {
		t.Fatalf("settings_list_jetbrains_ides: %v", err)
	}
	var choices []struct {
		IDE  string `json:"ide"`
		Path string `json:"path"`
	}
	if err := json.Unmarshal(choicesRaw, &choices); err != nil {
		t.Fatal(err)
	}
	if len(choices) != 1 || choices[0].IDE != "IntelliJIdea" || choices[0].Path != ideDir {
		t.Fatalf("JetBrains choices = %+v", choices)
	}

	readRaw, err := m.Invoke(ctx, "settings-window", "settings_read_jetbrains", map[string]any{"path": ideDir})
	if err != nil {
		t.Fatalf("settings_read_jetbrains: %v", err)
	}
	var read struct {
		Entries []string          `json:"entries"`
		Files   map[string]string `json:"files"`
	}
	if err := json.Unmarshal(readRaw, &read); err != nil {
		t.Fatal(err)
	}
	if read.Files["options/editor.xml"] != editorXML {
		t.Fatalf("JetBrains settings = %+v", read)
	}

	var archive bytes.Buffer
	zipWriter := zip.NewWriter(&archive)
	for name, contents := range map[string]string{
		"IntelliJ IDEA Global Settings": "",
		"options/editor.xml":            editorXML,
	} {
		entry, err := zipWriter.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(contents)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	zipPath := filepath.Join(home, "settings.zip")
	if err := os.WriteFile(zipPath, archive.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	archiveRaw, err := m.Invoke(ctx, "settings-window", "settings_read_jetbrains", map[string]any{
		"path": zipPath,
	})
	if err != nil {
		t.Fatalf("settings_read_jetbrains archive: %v", err)
	}
	if err := json.Unmarshal(archiveRaw, &read); err != nil {
		t.Fatal(err)
	}
	if read.Files["options/editor.xml"] != editorXML {
		t.Fatalf("JetBrains archive settings = %+v", read)
	}
}
