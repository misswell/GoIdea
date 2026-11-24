package app

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

func TestLogcatFilterMatchesAndroidStudioExpressions(t *testing.T) {
	lines := []openidea.LogcatLine{
		{Time: "11-04 12:52:39.123", PID: 42, TID: 43, Level: "error", Tag: "GlideLoader", Message: "bitmap cache failed"},
		{Time: "11-04 12:52:40.123", PID: 7, TID: 7, Level: "info", Tag: "Chatty", Message: "beginning of main"},
		{Time: "11-04 12:52:41.123", PID: 42, TID: 99, Level: "debug", Tag: "Network", Message: "tile fetch"},
	}
	cases := []struct {
		query string
		want  []int
	}{
		{`tag:glide`, []int{0}},
		{`message:"cache failed"`, []int{0}},
		{`pid:43`, []int{0}},
		{`level:error,fatal`, []int{0}},
		{`tag:glide -message:works`, []int{0}},
		{`#42 (tag:Network|tag:GlideLoader)`, []int{0, 2}},
		{`message~:^tile.*`, []int{2}},
	}
	for _, tc := range cases {
		matcher, err := compileLogcatQuery(tc.query)
		if err != nil {
			t.Fatalf("%q: %v", tc.query, err)
		}
		var got []int
		for index, line := range lines {
			if matcher.match(line) {
				got = append(got, index)
			}
		}
		if len(got) != len(tc.want) {
			t.Fatalf("%q matched %v, want %v", tc.query, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("%q matched %v, want %v", tc.query, got, tc.want)
			}
		}
	}
	if _, err := compileLogcatQuery("level:loud"); err == nil {
		t.Fatal("unknown level was accepted")
	}
	if _, err := compileLogcatQuery("(tag:a"); err == nil {
		t.Fatal("missing parenthesis was accepted")
	}
}

func TestLogcatDevicesStreamingFiltersPauseAndClear(t *testing.T) {
	device := openidea.AndroidDevice{
		Serial: "emulator-5554", State: "device", Model: stringPointer("Pixel"),
	}
	executeDone := make(chan struct{})
	var mu sync.Mutex
	var calls []notebookTestCall
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			args, _ := input.(map[string]any)
			mu.Lock()
			calls = append(calls, notebookTestCall{command: command, args: args})
			mu.Unlock()
			switch command {
			case "android_devices":
				return json.Marshal([]openidea.AndroidDevice{device})
			case "android_logcat":
				<-executeDone
				return json.RawMessage(`null`), nil
			case "cancel_request":
				return json.RawMessage(`null`), nil
			default:
				return nil, nil
			}
		},
	}
	m.RefreshLogcatDevices()
	waitFor(t, "Android discovery and stream start", func() bool {
		return m.logcat.devicesLoaded && m.logcat.serial == device.Serial && m.logcat.channel != 0
	})
	channel := m.logcat.channel
	m.acceptLogcatChunk(channel, json.RawMessage(`[
		{"time":"11-04 12:52:39.123","pid":42,"tid":42,"level":"error","tag":"Glide","message":"boom"},
		{"time":"11-04 12:52:40.123","pid":7,"tid":7,"level":"info","tag":"System","message":"ready"}
	]`))
	if len(m.logcat.lines) != 2 || len(m.logcat.filtered) != 2 {
		t.Fatalf("lines = %d filtered = %d", len(m.logcat.lines), len(m.logcat.filtered))
	}

	m.setLogcatLevel("error")
	if len(m.logcat.filtered) != 1 || m.logcat.filtered[0].Tag != "Glide" {
		t.Fatalf("level filter = %+v", m.logcat.filtered)
	}
	m.setLogcatQuery("tag:glide")
	if len(m.logcat.filtered) != 1 || m.logcat.filterErr != "" {
		t.Fatalf("query filter = %+v, error = %q", m.logcat.filtered, m.logcat.filterErr)
	}
	m.setLogcatQuery("(")
	if m.logcat.filterErr == "" || len(m.logcat.filtered) != 1 {
		t.Fatalf("invalid query error = %q, lines = %d", m.logcat.filterErr, len(m.logcat.filtered))
	}
	m.setLogcatQuery("")
	m.setLogcatLevel("verbose")

	m.SetLogcatPaused(true)
	m.acceptLogcatChunk(channel, json.RawMessage(
		`[{"time":"11-04 12:52:41.123","pid":8,"tid":8,"level":"warn","tag":"Later","message":"queued"}]`,
	))
	if len(m.logcat.lines) != 2 || len(m.logcat.pending) != 1 {
		t.Fatalf("paused lines = %d pending = %d", len(m.logcat.lines), len(m.logcat.pending))
	}
	m.SetLogcatPaused(false)
	if len(m.logcat.lines) != 3 || len(m.logcat.pending) != 0 {
		t.Fatalf("resumed lines = %d pending = %d", len(m.logcat.lines), len(m.logcat.pending))
	}
	m.ClearLogcat()
	if len(m.logcat.lines) != 0 || len(m.logcat.filtered) != 0 {
		t.Fatalf("clear left lines = %d filtered = %d", len(m.logcat.lines), len(m.logcat.filtered))
	}

	m.StopLogcat()
	close(executeDone)
	waitFor(t, "backend cancellation", func() bool {
		mu.Lock()
		defer mu.Unlock()
		for _, call := range calls {
			if call.command == "cancel_request" {
				return true
			}
		}
		return false
	})
}

func TestLogcatToolWindowRendersNativeControlsAndRows(t *testing.T) {
	m := &Model{}
	m.ensureLogcat()
	m.logcat.devices = []openidea.AndroidDevice{{
		Serial: "emulator-5554", State: "device", Model: stringPointer("Pixel"),
	}}
	m.logcat.serial = "emulator-5554"
	m.logcat.status = "running"
	m.logcat.lines = []logcatRow{{
		id: 1,
		LogcatLine: openidea.LogcatLine{
			Time: "11-04 12:52:39.123", PID: 42, TID: 42, Level: "error",
			Tag: "Glide", Message: "bitmap cache failed",
		},
	}}
	m.rebuildLogcatView()

	tester := ui.NewTester(func(c *ui.Context) {
		m.logcatPanel(c, c.Theme(), openideaColors(true))
	}, 1100, 500)
	text := strings.Join(tester.Texts(), "\n")
	for _, want := range []string{"Pixel (emulator-5554)", "Glide", "bitmap cache failed", "Streaming", "1"} {
		if !strings.Contains(text, want) {
			t.Fatalf("Logcat view lacks %q: %q", want, tester.Texts())
		}
	}
}
