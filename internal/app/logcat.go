package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

const (
	maxLogcatLines   = 20_000
	maxLogcatHistory = 20
)

type logcatRow struct {
	openidea.LogcatLine
	id uint64
}

type logcatState struct {
	devices        []openidea.AndroidDevice
	devicesLoaded  bool
	devicesLoading bool
	devicesErr     string

	serial    string
	level     string
	query     string
	history   []string
	historyAt int
	draft     string
	matcher   logcatMatcher
	filterErr string

	paused bool
	follow bool
	status string
	err    string

	request uint32
	channel uint32
	seq     uint64
	cancel  context.CancelFunc

	lines    []logcatRow
	pending  []logcatRow
	filtered []logcatRow
	nextID   uint64
	list     ui.ListState
}

func (m *Model) ensureLogcat() {
	if m.logcat.level == "" {
		m.logcat.level = "verbose"
		m.logcat.follow = true
		m.logcat.status = "idle"
		m.logcat.historyAt = -1
		m.logcat.matcher, _ = compileLogcatQuery("")
	}
}

// OpenLogcat shows the Android log window and starts discovery the same way
// OpenIDEA's lazy tool-window view does.
func (m *Model) OpenLogcat() {
	m.ensureLogcat()
	m.DockShow("logcat")
	if !m.logcat.devicesLoaded && !m.logcat.devicesLoading {
		m.RefreshLogcatDevices()
	}
}

// RefreshLogcatDevices reads `adb devices -l`, keeping the watched device when
// it remains available.
func (m *Model) RefreshLogcatDevices() {
	m.ensureLogcat()
	if m.logcat.devicesLoading {
		return
	}
	if m.invokeHook == nil && m.mgr == nil {
		return
	}
	m.logcat.devicesLoading = true
	m.logcat.devicesErr = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.backgroundContext(), 20*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "android_devices", nil)
		var devices []openidea.AndroidDevice
		if err == nil {
			err = json.Unmarshal(raw, &devices)
		}
		m.post(func() {
			m.logcat.devicesLoading = false
			if err != nil {
				m.logcat.devicesErr = "Cannot list Android devices: " + err.Error()
				m.invalidate()
				return
			}
			m.logcat.devices = devices
			m.logcat.devicesLoaded = true
			keep := ""
			for _, device := range devices {
				if device.Serial == m.logcat.serial && device.State == "device" {
					keep = device.Serial
					break
				}
			}
			if keep == "" {
				for _, device := range devices {
					if device.State == "device" {
						keep = device.Serial
						break
					}
				}
			}
			if keep != "" {
				m.SelectLogcatDevice(keep)
			} else {
				m.StopLogcat()
				m.logcat.serial = ""
			}
			m.invalidate()
		})
	}()
}

// SelectLogcatDevice switches the stream. Offline and unauthorized devices
// remain selectable so the tool window can explain why no lines arrive.
func (m *Model) SelectLogcatDevice(serial string) {
	m.ensureLogcat()
	serial = strings.TrimSpace(serial)
	if serial == "" {
		m.StopLogcat()
		m.logcat.serial = ""
		m.invalidate()
		return
	}
	var state string
	for _, device := range m.logcat.devices {
		if device.Serial == serial {
			state = device.State
			break
		}
	}
	m.StopLogcat()
	m.logcat.serial = serial
	m.logcat.lines = nil
	m.logcat.pending = nil
	m.logcat.filtered = nil
	m.logcat.nextID = 0
	if state != "" && state != "device" {
		m.logcat.status = "idle"
		m.logcat.err = "Device " + serial + " is " + state
		m.invalidate()
		return
	}
	m.startLogcat(serial)
}

func (m *Model) startLogcat(serial string) {
	if m.invokeHook == nil && m.mgr == nil {
		return
	}
	ctx, cancel := context.WithCancel(m.backgroundContext())
	m.logcat.seq++
	seq := m.logcat.seq
	m.logcat.cancel = cancel
	m.logcat.request = m.nextRequest()
	m.logcat.channel = m.nextChannel()
	m.logcat.status = "running"
	m.logcat.err = ""
	request, channel := m.logcat.request, m.logcat.channel
	m.invalidate()
	go func() {
		raw, err := m.invoke(ctx, "android_logcat", map[string]any{
			"request": request,
			"serial":  serial,
			"clear":   false,
			"onLines": map[string]any{"$channel": channel},
		})
		_ = raw
		m.post(func() {
			if m.logcat.seq != seq {
				return
			}
			m.logcat.status = "idle"
			if err != nil && !errors.Is(err, context.Canceled) &&
				!strings.Contains(strings.ToLower(err.Error()), "cancel") {
				m.logcat.status = "error"
				m.logcat.err = "Logcat stopped: " + err.Error()
			}
			m.logcat.request = 0
			m.logcat.channel = 0
			m.logcat.cancel = nil
			m.invalidate()
		})
	}()
}

// StopLogcat cancels both the Go call and the backend request. The function is
// idempotent so closing the window or switching devices cannot leak a stream.
func (m *Model) StopLogcat() {
	m.ensureLogcat()
	m.logcat.seq++
	request := m.logcat.request
	if m.logcat.cancel != nil {
		m.logcat.cancel()
	}
	m.logcat.cancel = nil
	m.logcat.request = 0
	m.logcat.channel = 0
	if request != 0 && (m.invokeHook != nil || m.mgr != nil) {
		go func() {
			ctx, cancel := context.WithTimeout(m.backgroundContext(), 5*time.Second)
			defer cancel()
			_, _ = m.invoke(ctx, "cancel_request", map[string]any{"id": request})
		}()
	}
	if m.logcat.status == "running" {
		m.logcat.status = "idle"
		m.invalidate()
	}
}

func (m *Model) acceptLogcatChunk(channel uint32, data json.RawMessage) bool {
	m.ensureLogcat()
	if channel == 0 || channel != m.logcat.channel {
		return false
	}
	var lines []openidea.LogcatLine
	if err := json.Unmarshal(data, &lines); err != nil {
		m.logcat.err = "Cannot decode Logcat output: " + err.Error()
		m.invalidate()
		return true
	}
	batch := make([]logcatRow, 0, len(lines))
	for _, line := range lines {
		m.logcat.nextID++
		batch = append(batch, logcatRow{LogcatLine: line, id: m.logcat.nextID})
	}
	if m.logcat.paused {
		m.logcat.pending = appendLogcatRows(m.logcat.pending, batch)
	} else {
		m.logcat.lines = appendLogcatRows(m.logcat.lines, batch)
		m.rebuildLogcatView()
	}
	m.invalidate()
	return true
}

func appendLogcatRows(current, batch []logcatRow) []logcatRow {
	rows := append(current, batch...)
	if len(rows) > maxLogcatLines {
		trimmed := append([]logcatRow(nil), rows[len(rows)-maxLogcatLines:]...)
		return trimmed
	}
	return rows
}

func (m *Model) SetLogcatPaused(paused bool) {
	m.ensureLogcat()
	if m.logcat.paused == paused {
		return
	}
	m.logcat.paused = paused
	if !paused && len(m.logcat.pending) > 0 {
		m.logcat.lines = appendLogcatRows(m.logcat.lines, m.logcat.pending)
		m.logcat.pending = nil
		m.rebuildLogcatView()
	}
	m.invalidate()
}

func (m *Model) ToggleLogcatPaused() {
	m.ensureLogcat()
	m.SetLogcatPaused(!m.logcat.paused)
}

func (m *Model) ClearLogcat() {
	m.ensureLogcat()
	m.logcat.lines = nil
	m.logcat.pending = nil
	m.logcat.filtered = nil
	m.logcat.err = ""
	m.invalidate()
}

func (m *Model) ToggleLogcatFollow() {
	m.ensureLogcat()
	m.logcat.follow = !m.logcat.follow
	if m.logcat.follow {
		m.logcat.list.ScrollToEnd()
	}
	m.invalidate()
}

func (m *Model) setLogcatLevel(level string) {
	m.ensureLogcat()
	if level == "" {
		level = "verbose"
	}
	m.logcat.level = level
	m.rebuildLogcatView()
	m.invalidate()
}

func (m *Model) setLogcatQuery(query string) {
	m.ensureLogcat()
	m.logcat.query = query
	matcher, err := compileLogcatQuery(query)
	m.logcat.matcher = matcher
	if err != nil {
		m.logcat.filterErr = err.Error()
	} else {
		m.logcat.filterErr = ""
	}
	m.rebuildLogcatView()
	m.invalidate()
}

func (m *Model) rememberLogcatFilter(query string) {
	m.ensureLogcat()
	query = strings.TrimSpace(query)
	if query == "" {
		return
	}
	history := make([]string, 0, min(maxLogcatHistory, len(m.logcat.history)+1))
	history = append(history, query)
	for _, item := range m.logcat.history {
		if item != query && len(history) < maxLogcatHistory {
			history = append(history, item)
		}
	}
	m.logcat.history = history
	m.logcat.historyAt = -1
	m.logcat.draft = ""
	m.saveSession()
}

func (m *Model) recallLogcatFilter(step int) string {
	m.ensureLogcat()
	if len(m.logcat.history) == 0 {
		return m.logcat.query
	}
	if m.logcat.historyAt == -1 {
		m.logcat.draft = m.logcat.query
	}
	m.logcat.historyAt += step
	if m.logcat.historyAt < -1 {
		m.logcat.historyAt = -1
	}
	if m.logcat.historyAt >= len(m.logcat.history) {
		m.logcat.historyAt = len(m.logcat.history) - 1
	}
	if m.logcat.historyAt < 0 {
		return m.logcat.draft
	}
	return m.logcat.history[m.logcat.historyAt]
}

func (m *Model) rebuildLogcatView() {
	m.ensureLogcat()
	if m.logcat.matcher == nil {
		m.logcat.matcher, _ = compileLogcatQuery("")
	}
	floor := logcatLevelRank(m.logcat.level)
	out := make([]logcatRow, 0, len(m.logcat.lines))
	for _, line := range m.logcat.lines {
		rank := logcatLevelRank(line.Level)
		if rank < floor && line.Level != "unknown" {
			continue
		}
		if m.logcat.matcher.match(line.LogcatLine) {
			out = append(out, line)
		}
	}
	m.logcat.filtered = out
}

func logcatLevelRank(level string) int {
	switch level {
	case "verbose":
		return 0
	case "debug":
		return 1
	case "info":
		return 2
	case "warn":
		return 3
	case "error":
		return 4
	case "fatal":
		return 5
	default:
		return 6
	}
}

func (m *Model) backgroundContext() context.Context {
	if m.ctx != nil {
		return m.ctx
	}
	return context.Background()
}

func (m *Model) logcatPanel(c *ui.Context, p *ui.Theme, oi oiColors) {
	m.ensureLogcat()
	ui.Column(c.Key("logcat")).Grow(1).MinHeight(0).
		BorderWidth(0, 0, 0, 1).BorderColor(oi.border).Children(func() {
		m.logcatToolbar(c, p, oi)
		m.logcatBody(c, p, oi)
		m.logcatStatus(c, p, oi)
	})
}

func (m *Model) logcatToolbar(c *ui.Context, p *ui.Theme, oi oiColors) {
	ui.Row(c.Key("logcat-toolbar")).Shrink(0).Height(34).Padding(5, 7).Gap(6).
		AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(oi.border).
		Children(func() {
			deviceLabels := make([]string, 0, len(m.logcat.devices))
			deviceByLabel := make(map[string]string, len(m.logcat.devices))
			selectedDevice := ""
			for _, device := range m.logcat.devices {
				label := device.Serial
				if device.Model != nil && strings.TrimSpace(*device.Model) != "" {
					label = *device.Model + " (" + device.Serial + ")"
				}
				if device.State != "device" {
					label += " · " + device.State
				}
				deviceLabels = append(deviceLabels, label)
				deviceByLabel[label] = device.Serial
				if device.Serial == m.logcat.serial {
					selectedDevice = label
				}
			}
			if len(deviceLabels) == 0 {
				deviceLabels = []string{"No devices"}
				selectedDevice = "No devices"
			}
			if choice := ui.Select(c.Key("logcat-device"), &selectedDevice, deviceLabels).Width(230); choice.Changed() {
				if serial := deviceByLabel[selectedDevice]; serial != "" {
					m.SelectLogcatDevice(serial)
				}
			}
			levelLabels := map[string]string{
				"verbose": "Verbose",
				"debug":   "Debug",
				"info":    "Info",
				"warn":    "Warnings",
				"error":   "Errors",
				"fatal":   "Fatal",
			}
			levelValues := map[string]string{
				"Verbose":  "verbose",
				"Debug":    "debug",
				"Info":     "info",
				"Warnings": "warn",
				"Errors":   "error",
				"Fatal":    "fatal",
			}
			levels := []string{"Verbose", "Debug", "Info", "Warnings", "Errors", "Fatal"}
			selectedLevel := levelLabels[m.logcat.level]
			if choice := ui.Select(c.Key("logcat-level"), &selectedLevel, levels).Width(96); choice.Changed() {
				m.setLogcatLevel(levelValues[selectedLevel])
			}
			input := ui.TextInput(c.Key("logcat-filter"), &m.logcat.query).Grow(1).MinWidth(120).
				Placeholder("Filter: tag:glide -message:bitmap level:error")
			input.OnTextKey(func(key ui.TextKey) bool {
				if key.Mods != 0 {
					return false
				}
				switch key.Key {
				case ui.KeyUp:
					value := m.recallLogcatFilter(1)
					key.SetText(value)
					m.setLogcatQuery(value)
					return true
				case ui.KeyDown:
					value := m.recallLogcatFilter(-1)
					key.SetText(value)
					m.setLogcatQuery(value)
					return true
				case ui.KeyEnter:
					m.rememberLogcatFilter(key.Text())
					return true
				default:
					return false
				}
			})
			if input.Changed() {
				m.logcat.historyAt = -1
				m.setLogcatQuery(m.logcat.query)
			}
			history := ui.Button(c, "History")
			history.Disabled(len(m.logcat.history) == 0)
			history.Menu(func(menu *ui.Menu) {
				for _, query := range m.logcat.history {
					query := query
					if menu.Item(query).Chosen() {
						m.setLogcatQuery(query)
					}
				}
				menu.Separator()
				if menu.Item("Clear History").Chosen() {
					m.logcat.history = nil
					m.logcat.historyAt = -1
					m.saveSession()
					m.invalidate()
				}
			})
			pauseLabel := "Pause"
			if m.logcat.paused {
				pauseLabel = "Resume"
			}
			ui.Button(c, pauseLabel).OnClick(m.ToggleLogcatPaused)
			followLabel := "Follow"
			if !m.logcat.follow {
				followLabel = "Follow Off"
			}
			ui.Button(c, followLabel).OnClick(m.ToggleLogcatFollow)
			ui.Button(c, "Clear").OnClick(m.ClearLogcat)
			refresh := ui.Button(c, "Refresh")
			refresh.Disabled(m.logcat.devicesLoading)
			refresh.OnClick(m.RefreshLogcatDevices)
		})
}

func (m *Model) logcatBody(c *ui.Context, p *ui.Theme, oi oiColors) {
	if m.logcat.devicesErr != "" {
		ui.Column(c).Grow(1).AlignItems(ui.Center).Justify(ui.Center).Padding(20).Gap(5).
			Children(func() {
				ui.Text(c, "No Android devices").FontSize(13).Bold().TextColor(oi.text)
				ui.Text(c, m.logcat.devicesErr).FontSize(12).TextColor(oi.danger)
			})
		return
	}
	if len(m.logcat.devices) == 0 {
		message := "Connect a device over USB, or start an emulator."
		if m.logcat.devicesLoading {
			message = "Looking for Android devices…"
		}
		ui.Column(c).Grow(1).AlignItems(ui.Center).Justify(ui.Center).Padding(20).Gap(5).
			Children(func() {
				ui.Text(c, "No Android devices").FontSize(13).Bold().TextColor(oi.text)
				ui.Text(c, message).FontSize(12).TextColor(oi.textMuted)
			})
		return
	}
	if len(m.logcat.filtered) == 0 {
		message := "Waiting for output…"
		if len(m.logcat.lines) > 0 {
			message = "No lines match the filter"
		}
		ui.Column(c).Grow(1).MinHeight(0).AlignItems(ui.Center).Justify(ui.Center).
			Children(func() { ui.Text(c, message).FontSize(12).TextColor(oi.textMuted) })
		return
	}
	m.logcat.list.FollowEnd = m.logcat.follow
	m.logcat.list.FollowAnimation = &ui.Smooth
	m.logcat.list.Key = func(row int) any {
		if row < 0 || row >= len(m.logcat.filtered) {
			return row
		}
		return m.logcat.filtered[row].id
	}
	ui.List(c.Key("logcat-lines"), &m.logcat.list, len(m.logcat.filtered), func(row int) {
		if row < 0 || row >= len(m.logcat.filtered) {
			return
		}
		line := m.logcat.filtered[row]
		tone := logcatLevelColor(line.Level, oi)
		ui.Row(c.Key(fmt.Sprintf("logcat-line:%d", line.id))).Height(22).Padding(2, 7).
			Gap(7).AlignItems(ui.Center).Children(func() {
			ui.Text(c, shortLogcatTime(line.Time)).FontSize(11).Font("monospace").
				TextColor(oi.textMuted).Width(92).SingleLine()
			ui.Text(c, fmt.Sprintf("%d", line.PID)).FontSize(11).Font("monospace").
				TextColor(oi.textMuted).Width(48).SingleLine()
			ui.Text(c, "["+strings.ToUpper(logcatLevelLetter(line.Level))+"]").
				FontSize(11).Bold().Font("monospace").TextColor(tone).Width(26)
			ui.Text(c, line.Tag).FontSize(11).Bold().Font("monospace").TextColor(tone).
				Width(140).SingleLine().Ellipsis("…")
			ui.Text(c, line.Message).FontSize(11).Font("monospace").TextColor(oi.text).
				Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
		}).ContextMenu(func(menu *ui.Menu) {
			if menu.Item("Copy Line").Chosen() {
				copyLogcatLine(line.LogcatLine)
			}
			if menu.Item("Copy Tag").Chosen() {
				copyTextForLogcat(line.Tag)
			}
		})
	}).Grow(1).MinHeight(0)
}

func (m *Model) logcatStatus(c *ui.Context, p *ui.Theme, oi oiColors) {
	ui.Row(c.Key("logcat-status")).Shrink(0).Height(24).Padding(0, 7).Gap(9).
		AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(oi.border).
		Children(func() {
			status := "Idle"
			switch m.logcat.status {
			case "running":
				status = "Streaming"
			case "error":
				status = "Stopped"
			}
			ui.Text(c, status).FontSize(11).TextColor(oi.textSecondary)
			if m.logcat.paused {
				ui.Text(c, "Paused").FontSize(11).TextColor(oi.warning)
			}
			if m.logcat.filterErr != "" {
				ui.Text(c, "Filter ignored: "+m.logcat.filterErr).
					FontSize(11).TextColor(oi.danger).SingleLine().Ellipsis("…")
			}
			ui.Spacer(c)
			count := fmt.Sprintf("%d", len(m.logcat.lines))
			if len(m.logcat.filtered) != len(m.logcat.lines) {
				count = fmt.Sprintf("%d / %d", len(m.logcat.filtered), len(m.logcat.lines))
			}
			ui.Text(c, count).FontSize(11).TextColor(oi.textMuted)
			if m.logcat.err != "" {
				ui.Text(c, m.logcat.err).FontSize(11).TextColor(oi.danger).
					SingleLine().Ellipsis("…")
			}
		})
}

func logcatLevelLetter(level string) string {
	switch level {
	case "verbose":
		return "V"
	case "debug":
		return "D"
	case "info":
		return "I"
	case "warn":
		return "W"
	case "error":
		return "E"
	case "fatal":
		return "F"
	default:
		return "U"
	}
}

func logcatLevelColor(level string, oi oiColors) ui.Color {
	switch level {
	case "warn":
		return oi.warning
	case "error", "fatal":
		return oi.danger
	case "info":
		return oi.link
	case "debug":
		return oi.textSecondary
	default:
		return oi.textMuted
	}
}

func shortLogcatTime(value string) string {
	if len(value) > 6 {
		return value[6:]
	}
	return value
}

func copyLogcatLine(line openidea.LogcatLine) {
	mygo.Clipboard.WriteText(fmt.Sprintf(
		"%s %5d %5d %s %s: %s",
		line.Time, line.PID, line.TID, strings.ToUpper(line.Level), line.Tag, line.Message,
	))
}

func copyTextForLogcat(value string) {
	mygo.Clipboard.WriteText(value)
}
