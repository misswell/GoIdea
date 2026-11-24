package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"

	"mayoi/internal/docbridge"
)

func TestFileEncodingSettingsUseTheGlobalBackendSettings(t *testing.T) {
	var command string
	var args any = "unset"
	m := &Model{
		ctx: context.Background(),
		invokeHook: func(_ context.Context, got string, input any) (json.RawMessage, error) {
			command, args = got, input
			return json.RawMessage(`{
				"editor":{"encodings":{
					"global":"GBK",
					"project":"UTF-16LE",
					"properties":"ISO-8859-1"
				}}
			}`), nil
		},
	}
	m.ensureFileEncodingSettings(context.Background())
	if command != "settings_get_global" || args != nil {
		t.Fatalf("settings RPC = %q %#v, want settings_get_global with no arguments", command, args)
	}
	if got := m.standaloneEncodingFallback(filepath.Join("tmp", "notes.txt")); got != "gbk" {
		t.Fatalf("text fallback = %q, want gbk", got)
	}
	if got := m.standaloneEncodingFallback(filepath.Join("tmp", "messages.properties")); got != "windows-1252" {
		t.Fatalf("properties fallback = %q, want windows-1252", got)
	}
}

func TestFileEncodingSettingsRefreshAfterSettingsWrites(t *testing.T) {
	var loads int
	m := &Model{
		ctx: context.Background(),
		invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			if command != "settings_get_global" {
				t.Fatalf("settings RPC = %q", command)
			}
			loads++
			if loads == 1 {
				return json.RawMessage(`{"editor":{"encodings":{"global":"GBK"}}}`), nil
			}
			return json.RawMessage(`{"editor":{"encodings":{"global":"windows-1252"}}}`), nil
		},
	}
	path := filepath.Join("tmp", "notes.txt")
	m.ensureFileEncodingSettings(context.Background())
	if got := m.standaloneEncodingFallback(path); got != "gbk" {
		t.Fatalf("initial fallback = %q, want gbk", got)
	}
	m.invalidateFileEncodingSettings()
	m.ensureFileEncodingSettings(context.Background())
	if got := m.standaloneEncodingFallback(path); got != "windows-1252" {
		t.Fatalf("refreshed fallback = %q, want windows-1252", got)
	}
	if loads != 2 {
		t.Fatalf("settings loads = %d, want 2", loads)
	}
}

func TestStandaloneDocumentsDecodeGlobalAndPropertiesBytes(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "project")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	gbkText := "name=中文\n"
	gbkBytes, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(gbkText))
	if err != nil {
		t.Fatal(err)
	}
	gbkPath := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(gbkPath, gbkBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	propertiesPath := filepath.Join(dir, "messages.properties")
	propertiesBytes := []byte("greeting=caf\xe9\n")
	if err := os.WriteFile(propertiesPath, propertiesBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	m := &Model{
		invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			t.Fatalf("unexpected backend RPC %q", command)
			return nil, nil
		},
	}
	m.fileEncodings.loaded = true
	m.fileEncodings.values = fileEncodingSettings{
		Global: "gbk", Project: "utf-8", Properties: "windows-1252",
	}
	docs := newHybridDocs(&recordingDocBackend{}, func() string { return root })
	docs.SetEncodingRuntime(m.ensureFileEncodingSettings, m.standaloneEncodingFallback)

	info, err := docs.DocOpen(context.Background(), Window, gbkPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Text != gbkText || info.Encoding != "gbk" {
		t.Fatalf("GBK document = %+v", info)
	}
	if err := docs.DocSave(context.Background(), Window, info.ID); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(gbkPath); err != nil || !bytes.Equal(got, gbkBytes) {
		t.Fatalf("GBK saved bytes = %q, err %v", got, err)
	}

	properties, err := docs.DocOpen(context.Background(), Window, propertiesPath)
	if err != nil {
		t.Fatal(err)
	}
	if properties.Text != "greeting=café\n" || properties.Encoding != "windows-1252" {
		t.Fatalf("properties document = %+v", properties)
	}
	if err := docs.DocSave(context.Background(), Window, properties.ID); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(propertiesPath); err != nil || !bytes.Equal(got, propertiesBytes) {
		t.Fatalf("properties saved bytes = %q, err %v", got, err)
	}
}

func TestStandaloneDocumentsSniffAndPreserveBOMlessUTF16(t *testing.T) {
	for _, test := range []struct {
		name      string
		bigEndian bool
	}{
		{name: "little-endian"},
		{name: "big-endian", bigEndian: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			const text = "plain ascii\n"
			data := encodeUTF16(text, test.bigEndian, false)
			if !utf8.Valid(data) {
				t.Fatal("fixture must also be syntactically valid UTF-8")
			}
			path := filepath.Join(t.TempDir(), "notes.txt")
			if err := os.WriteFile(path, data, 0o644); err != nil {
				t.Fatal(err)
			}
			docs := newLocalDocsBackend()
			info, err := docs.docOpen(context.Background(), Window, path, "windows-1252")
			if err != nil {
				t.Fatal(err)
			}
			wantEncoding := "utf-16le"
			if test.bigEndian {
				wantEncoding = "utf-16be"
			}
			if info.Text != text || info.Encoding != wantEncoding || info.HasBOM {
				t.Fatalf("UTF-16 document = %+v", info)
			}
			if err := docs.DocSave(context.Background(), Window, info.ID); err != nil {
				t.Fatal(err)
			}
			if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, data) {
				t.Fatalf("UTF-16 saved bytes = %x, err %v", got, err)
			}
		})
	}
}

func TestEncodedProjectTextOpensThroughDocOpenInsteadOfHex(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "src", "Encoded.txt")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte("// 中文\n"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	m := New(Config{SettingsPath: filepath.Join(root, "settings.json")})
	t.Cleanup(m.off)
	m.setRoot(root)
	remote := &recordingDocBackend{}
	m.reg = docbridge.NewRegistry(remote, Window)
	if hex, _ := wantsHexViewer(path); hex {
		t.Fatal("encoded project text was routed to the hex viewer")
	}
	opened := make(chan struct{})
	m.postHook = func(fn func()) {
		fn()
		if m.tab() != nil {
			select {
			case <-opened:
			default:
				close(opened)
			}
		}
	}
	m.openFile(path)
	select {
	case <-opened:
	case <-time.After(5 * time.Second):
		t.Fatalf("encoded project file did not open: busy=%v tabs=%d status=%q banner=%q queue=%+v",
			m.busy, len(m.tabs), m.status, m.banner, m.openQueue)
	}
	if tab := m.tab(); tab == nil || tab.kind != "" || tab.doc.Text() != "remote\n" {
		t.Fatalf("encoded project file opened as %+v", tab)
	}
	if got, want := remote.opened(), []string{realPath(path)}; !reflect.DeepEqual(got, want) {
		t.Fatalf("project backend opens = %v, want %v", got, want)
	}
}

func TestBinaryRoutingStillStopsBeforeDocumentDecoding(t *testing.T) {
	if !isBinaryViewerData([]byte{0x7f, 'E', 'L', 'F', 0, 0, 1, 0}) {
		t.Fatal("NUL-bearing binary data was not routed to the hex viewer")
	}
	if isBinaryViewerData([]byte{0xd6, 0xd0, 0xce, 0xc4}) {
		t.Fatal("invalid UTF-8 text was treated as binary instead of a configured encoding")
	}
}
