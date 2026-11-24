package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
)

const tabKindDockerLog = "dockerLog"

func dockerLogEditorKey(connectionID, containerID string) string {
	return connectionID + "/" + containerID
}

func dockerLogEditorPath(connectionID, containerID string) string {
	return "docker-log:" + dockerLogEditorKey(connectionID, containerID)
}

func (m *Model) dockerLogEditor(key string) *Tab {
	tab := m.services.docker.logEditors[key]
	if tab == nil {
		return nil
	}
	for _, candidate := range m.allTabs() {
		if candidate == tab {
			return tab
		}
	}
	delete(m.services.docker.logEditors, key)
	return nil
}

func (m *Model) newDockerLogTab(path, title, text string) *Tab {
	doc := docbridge.Virtual(path, text)
	size := m.prefs.fontSize()
	font := m.prefs.Font
	tab := &Tab{
		doc: doc,
		state: &editor.State{
			Buffer:     doc.Buffer(),
			LineHeight: font.lineHeightPixels(size) / size,
			Bindings:   m.editorBindings(),
			ReadOnly:   true,
		},
		kind:      tabKindDockerLog,
		pageTitle: title,
	}
	tab.state.Font = ui.Font{
		Family: font.fontFamily(), Size: size,
		Weight: font.weight(), Features: font.features(),
	}
	tab.synced = doc.Buffer().Version()
	tab.seen = tab.synced
	return tab
}

type dockerLogHighlighter struct {
	stderr []bool
}

func (h *dockerLogHighlighter) Edit(editor.Snapshot, editor.Snapshot, []editor.Change) {}

func (h *dockerLogHighlighter) Highlight(int, time.Duration) bool { return true }

func (h *dockerLogHighlighter) Tokens(
	line int,
	text string,
	dst []editor.Token,
) ([]editor.Token, bool) {
	if line >= 0 && line < len(h.stderr) && h.stderr[line] && text != "" {
		dst = append(dst, editor.Token{End: len(text), Kind: editor.TokenInvalid})
	}
	return dst, true
}

func dockerLogLanguage(lines []openidea.DockerLogLine, hasError bool) *editor.Language {
	stderr := make([]bool, len(lines), len(lines)+1)
	for i, line := range lines {
		stderr[i] = line.Stderr
	}
	if hasError {
		stderr = append(stderr, true)
	}
	return &editor.Language{
		Name: "Docker Log",
		Highlighter: func(editor.Snapshot, func()) editor.Highlighter {
			return &dockerLogHighlighter{stderr: stderr}
		},
	}
}

func dockerLogText(lines []openidea.DockerLogLine) string {
	text := make([]string, len(lines))
	for i, line := range lines {
		text[i] = line.Text
	}
	return strings.Join(text, "\n")
}

func dockerLogSpans(lines []openidea.DockerLogLine, p *ui.Theme) []ui.Span {
	spans := make([]ui.Span, 0, len(lines))
	for i, line := range lines {
		text := line.Text
		if text == "" {
			text = " "
		}
		if i+1 < len(lines) {
			text += "\n"
		}
		color := p.TextMuted
		if line.Stderr {
			color = p.Danger
		}
		spans = append(spans, ui.Span{Text: text, Color: color})
	}
	return spans
}

func (m *Model) openDockerLogEditor(
	connection openidea.DockerConnection,
	container openidea.DockerContainer,
) {
	d := &m.services.docker
	key := dockerLogEditorKey(connection.ID, container.ID)
	title := container.Name + " (Log)"
	if d.logEditors == nil {
		d.logEditors = map[string]*Tab{}
	}
	if existing := m.dockerLogEditor(key); existing != nil {
		existing.pageTitle = title
		for i, candidate := range m.tabs {
			if candidate == existing {
				m.activate(i)
				break
			}
		}
		if stream := d.logs[dockerLogEditorPath(connection.ID, container.ID)]; stream != nil {
			stream.editor = existing
			m.syncDockerLogEditor(stream)
		}
		m.invalidate()
		return
	}

	node := serviceTreeNode{
		key:        dockerLogEditorPath(connection.ID, container.ID),
		dockerConn: connection.ID,
		dockerID:   container.ID,
		live:       dockerContainerRunning(container),
	}
	tab := m.newDockerLogTab(node.key, title, "")
	d.logEditors[key] = tab
	if m.split != nil {
		m.focusGroup(groupFocused)
	}
	m.insertTab(tab)
	m.active = m.tabIndex(tab)
	m.noteTabUse(tab)
	m.showingWelcome = false
	m.revealEditor()
	m.applyActive()

	stream := d.logs[node.key]
	if stream == nil {
		m.startDockerLogsWithOptions(connection, node, node.live, false)
		stream = d.logs[node.key]
	}
	if stream != nil {
		stream.editor = tab
		m.syncDockerLogEditor(stream)
	}
	m.invalidate()
}

func (m *Model) syncDockerLogEditor(stream *dockerLogStream) {
	if stream == nil || stream.editor == nil || stream.editor.state == nil {
		return
	}
	open := false
	for _, tab := range m.allTabs() {
		if tab == stream.editor {
			open = true
			break
		}
	}
	if !open {
		stream.editor = nil
		return
	}
	text := dockerLogText(stream.lines)
	if stream.err != "" {
		if text != "" {
			text += "\n"
		}
		text += "Docker log stream ended: " + stream.err
	}
	buffer := stream.editor.state.Buffer
	if buffer == nil {
		return
	}
	current := buffer.Snapshot().String()
	if current == text {
		return
	}
	buffer.Edit(editor.Edit{Start: 0, End: len(current), Text: text})
	buffer.SetLanguage(dockerLogLanguage(stream.lines, stream.err != ""))
	version := buffer.Version()
	stream.editor.synced = version
	stream.editor.seen = version
}

func dockerLogEditorStatus(stream *dockerLogStream) string {
	if stream == nil {
		return "Connecting…"
	}
	if stream.err != "" {
		return stream.err
	}
	if stream.loading {
		if stream.follow {
			return fmt.Sprintf("Following · %d lines", len(stream.lines))
		}
		return fmt.Sprintf("Loading · %d lines", len(stream.lines))
	}
	return fmt.Sprintf("%d lines", len(stream.lines))
}

func (m *Model) releaseDockerLogTab(tab *Tab) {
	if tab == nil || tab.kind != tabKindDockerLog {
		return
	}
	d := &m.services.docker
	for key, candidate := range d.logEditors {
		if candidate == tab {
			delete(d.logEditors, key)
		}
	}
	for key, stream := range d.logs {
		if stream == nil || stream.editor != tab {
			continue
		}
		delete(d.logs, key)
		m.stopDockerLogStream(stream)
	}
}
