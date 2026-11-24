package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/text/encoding/htmlindex"
	"golang.org/x/text/transform"
)

const textEncodingSniffBytes = 8 * 1024

type fileEncodingSettings struct {
	Global     string
	Project    string
	Properties string
}

func defaultFileEncodingSettings() fileEncodingSettings {
	return fileEncodingSettings{
		Global: "utf-8", Project: "utf-8", Properties: "windows-1252",
	}
}

type fileEncodingRuntime struct {
	mu      sync.RWMutex
	loaded  bool
	loading bool
	wait    chan struct{}
	values  fileEncodingSettings
}

func (m *Model) ensureFileEncodingSettings(ctx context.Context) {
	if m == nil || (m.mgr == nil && m.invokeHook == nil) {
		return
	}
	if m.invokeHook == nil && m.mgr != nil && !m.mgr.State().Running {
		return
	}
	m.fileEncodings.mu.Lock()
	if m.fileEncodings.loaded {
		m.fileEncodings.mu.Unlock()
		return
	}
	if m.fileEncodings.loading {
		wait := m.fileEncodings.wait
		m.fileEncodings.mu.Unlock()
		select {
		case <-wait:
		case <-ctx.Done():
		}
		return
	}
	m.fileEncodings.loading = true
	m.fileEncodings.wait = make(chan struct{})
	wait := m.fileEncodings.wait
	m.fileEncodings.mu.Unlock()

	raw, err := m.invoke(ctx, "settings_get_global", nil)
	values := defaultFileEncodingSettings()
	if err == nil {
		values = decodeFileEncodingSettings(raw)
	}

	m.fileEncodings.mu.Lock()
	if err == nil {
		m.fileEncodings.values = values
		m.fileEncodings.loaded = true
	}
	m.fileEncodings.loading = false
	close(wait)
	m.fileEncodings.mu.Unlock()
}

func (m *Model) invalidateFileEncodingSettings() {
	if m == nil {
		return
	}
	m.fileEncodings.mu.Lock()
	m.fileEncodings.loaded = false
	m.fileEncodings.mu.Unlock()
}

func (m *Model) standaloneEncodingFallback(path string) string {
	values := defaultFileEncodingSettings()
	if m != nil {
		m.fileEncodings.mu.RLock()
		if m.fileEncodings.loaded {
			values = m.fileEncodings.values
		}
		m.fileEncodings.mu.RUnlock()
	}
	if filepath.Ext(path) == ".properties" {
		return values.Properties
	}
	return values.Global
}

func decodeFileEncodingSettings(raw json.RawMessage) fileEncodingSettings {
	values := defaultFileEncodingSettings()
	var payload struct {
		Editor struct {
			Encodings struct {
				Global     string `json:"global"`
				Project    string `json:"project"`
				Properties string `json:"properties"`
			} `json:"encodings"`
		} `json:"editor"`
	}
	if json.Unmarshal(raw, &payload) != nil {
		return values
	}
	if label := normalizeTextEncoding(payload.Editor.Encodings.Global); label != "" {
		values.Global = label
	}
	if label := normalizeTextEncoding(payload.Editor.Encodings.Project); label != "" {
		values.Project = label
	}
	if label := normalizeTextEncoding(payload.Editor.Encodings.Properties); label != "" {
		values.Properties = label
	}
	return values
}

func normalizeTextEncoding(label string) string {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "":
		return ""
	case "utf8":
		return "utf-8"
	case "utf16le":
		return "utf-16le"
	case "utf16be":
		return "utf-16be"
	case "iso-8859-1", "iso8859-1", "latin1", "latin-1":
		return "windows-1252"
	case "shift-jis", "sjis", "ms_kanji":
		return "shift_jis"
	case "euckr":
		return "euc-kr"
	default:
		return strings.ToLower(strings.TrimSpace(label))
	}
}

func looksLikeBinaryText(head []byte) bool {
	if len(head) > textEncodingSniffBytes {
		head = head[:textEncodingSniffBytes]
	}
	if hasUnicodeBOM(head) {
		return false
	}
	if !containsZeroByte(head) {
		return false
	}
	if sniffTextUTF16(head) == "" {
		return true
	}
	var controls int
	for _, value := range head {
		if value < 0x20 && value != '\n' && value != '\r' && value != '\t' && value != '\f' && value != '\b' {
			controls++
		}
	}
	return controls*20 > len(head)
}

func decodeLocalText(data []byte, fallback string) (text, encoding string, hasBOM bool, lineEnding string) {
	switch {
	case hasBytesPrefix(data, 0xef, 0xbb, 0xbf):
		data = data[3:]
		text, encoding, hasBOM = string(data), "utf-8", true
	case hasBytesPrefix(data, 0xff, 0xfe):
		text, encoding, hasBOM = decodeUTF16(data[2:], false), "utf-16le", true
	case hasBytesPrefix(data, 0xfe, 0xff):
		text, encoding, hasBOM = decodeUTF16(data[2:], true), "utf-16be", true
	default:
		if detected := sniffTextUTF16(data); detected != "" {
			text, encoding = decodeUTF16(data, detected == "utf-16be"), detected
		} else if utf8.Valid(data) {
			text, encoding = string(data), "utf-8"
		} else {
			text, encoding = decodeWithTextEncoding(data, fallback)
		}
	}
	text, lineEnding = normalizeLocalLineEndings(text)
	return text, encoding, hasBOM, lineEnding
}

func encodeLocalText(text, encoding string, hasBOM bool, lineEnding string) ([]byte, error) {
	text = applyLocalLineEnding(text, lineEnding)
	encoding = normalizeTextEncoding(encoding)
	switch encoding {
	case "", "utf-8":
		data := []byte(text)
		if hasBOM {
			return append([]byte{0xef, 0xbb, 0xbf}, data...), nil
		}
		return data, nil
	case "utf-16le":
		return encodeUTF16(text, false, hasBOM), nil
	case "utf-16be":
		return encodeUTF16(text, true, hasBOM), nil
	}
	enc, err := htmlindex.Get(encoding)
	if err != nil {
		return nil, err
	}
	data, _, err := transform.Bytes(enc.NewEncoder(), []byte(text))
	return data, err
}

func decodeWithTextEncoding(data []byte, fallback string) (string, string) {
	fallback = normalizeTextEncoding(fallback)
	if fallback == "" {
		fallback = "utf-8"
	}
	enc, err := htmlindex.Get(fallback)
	if err == nil {
		decoded, _, decodeErr := transform.Bytes(enc.NewDecoder(), data)
		if decodeErr == nil {
			return string(decoded), fallback
		}
	}
	return strings.ToValidUTF8(string(data), "\uFFFD"), "utf-8"
}

func normalizeLocalLineEndings(text string) (string, string) {
	switch {
	case strings.Contains(text, "\r\n"):
		return strings.ReplaceAll(text, "\r\n", "\n"), "CRLF"
	case strings.Contains(text, "\r"):
		return strings.ReplaceAll(text, "\r", "\n"), "CR"
	default:
		return text, "LF"
	}
}

func applyLocalLineEnding(text, lineEnding string) string {
	switch lineEnding {
	case "CRLF":
		return strings.ReplaceAll(text, "\n", "\r\n")
	case "CR":
		return strings.ReplaceAll(text, "\n", "\r")
	default:
		return text
	}
}

func hasUnicodeBOM(data []byte) bool {
	return hasBytesPrefix(data, 0xef, 0xbb, 0xbf) ||
		hasBytesPrefix(data, 0xff, 0xfe) ||
		hasBytesPrefix(data, 0xfe, 0xff)
}

func hasBytesPrefix(data []byte, prefix ...byte) bool {
	if len(data) < len(prefix) {
		return false
	}
	for i, value := range prefix {
		if data[i] != value {
			return false
		}
	}
	return true
}

func containsZeroByte(data []byte) bool {
	for _, value := range data {
		if value == 0 {
			return true
		}
	}
	return false
}

func sniffTextUTF16(sample []byte) string {
	if len(sample) < 4 {
		return ""
	}
	pairs := len(sample) / 2
	var even, odd int
	for i, value := range sample[:pairs*2] {
		if value != 0 {
			continue
		}
		if i%2 == 0 {
			even++
		} else {
			odd++
		}
	}
	threshold := pairs * 3 / 5
	switch {
	case odd >= threshold && even*50 <= pairs:
		return "utf-16le"
	case even >= threshold && odd*50 <= pairs:
		return "utf-16be"
	default:
		return ""
	}
}

func decodeUTF16(data []byte, bigEndian bool) string {
	units := make([]uint16, 0, len(data)/2)
	for len(data) >= 2 {
		if bigEndian {
			units = append(units, uint16(data[0])<<8|uint16(data[1]))
		} else {
			units = append(units, uint16(data[1])<<8|uint16(data[0]))
		}
		data = data[2:]
	}
	return string(utf16.Decode(units))
}

func encodeUTF16(text string, bigEndian, hasBOM bool) []byte {
	units := utf16.Encode([]rune(text))
	data := make([]byte, 0, len(units)*2+2)
	if hasBOM {
		if bigEndian {
			data = append(data, 0xfe, 0xff)
		} else {
			data = append(data, 0xff, 0xfe)
		}
	}
	for _, unit := range units {
		if bigEndian {
			data = append(data, byte(unit>>8), byte(unit))
		} else {
			data = append(data, byte(unit), byte(unit>>8))
		}
	}
	return data
}
