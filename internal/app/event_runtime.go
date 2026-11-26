package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

const (
	backendTaskPrefix  = "goidea:task:"
	indexingTaskID     = "goidea:index"
	settingsEventDelay = 30 * time.Millisecond
)

var settingsEventSchedule = func(wait time.Duration, fn func()) func() {
	timer := time.AfterFunc(wait, fn)
	return func() { timer.Stop() }
}

type askpassPrompt struct {
	id       uint64
	prompt   string
	kind     string
	remember bool
}

// eventRuntimeState holds the state owned by backend events. It stays on the
// Model so a queued event remains tied to the project window that received it.
type eventRuntimeState struct {
	settingsGen     uint64
	settingsTimer   func()
	backendTasks    map[uint32]string
	indexing        openidea.IndexStateEvent
	indexingVisible bool
	askpass         []askpassPrompt
	unsupported     map[string]bool
}

func backendTaskID(id uint32) string { return backendTaskPrefix + fmt.Sprint(id) }
func backendIndexingID() string      { return indexingTaskID }

func (m *Model) runtimeEvents() *eventRuntimeState {
	state := &m.eventRuntime
	if state.backendTasks == nil {
		state.backendTasks = map[uint32]string{}
	}
	if state.unsupported == nil {
		state.unsupported = map[string]bool{}
	}
	return state
}

func (m *Model) resetEventRuntime() {
	if m == nil {
		return
	}
	if m.eventRuntime.settingsTimer != nil {
		m.eventRuntime.settingsTimer()
	}
	m.eventRuntime = eventRuntimeState{}
	kept := m.tasks[:0]
	for _, task := range m.tasks {
		if task.ID == indexingTaskID || strings.HasPrefix(task.ID, backendTaskPrefix) {
			continue
		}
		kept = append(kept, task)
	}
	m.tasks = kept
}

// acceptRuntimeEvent routes every backend event whose native consumer is
// owned by this file. It returns false for events handled elsewhere or with no
// event consumer, so the existing router stays explicit.
func (m *Model) acceptRuntimeEvent(eventType string, raw json.RawMessage) bool {
	if m == nil {
		return false
	}
	switch eventType {
	case "dbSourcesChanged":
		var event openidea.DBSourcesChangedEvent
		if json.Unmarshal(raw, &event) != nil {
			return true
		}
		m.acceptDBSourcesChanged(event)
		return true
	case "projectTrustChanged":
		var event openidea.ProjectTrustChangedEvent
		if json.Unmarshal(raw, &event) != nil {
			return true
		}
		m.acceptProjectTrustChanged(event)
		return true
	case "excludedDirsChanged":
		var event openidea.ExcludedDirsChangedEvent
		if json.Unmarshal(raw, &event) != nil {
			return true
		}
		m.acceptExcludedDirsChanged(event)
		return true
	case "task":
		var event openidea.TaskEvent
		if json.Unmarshal(raw, &event) != nil {
			return true
		}
		m.acceptBackendTask(event)
		return true
	case "indexState":
		var event openidea.IndexStateEvent
		if json.Unmarshal(raw, &event) != nil {
			return true
		}
		root := m.Root()
		m.post(func() {
			if root == "" || root != m.Root() {
				return
			}
			m.acceptIndexState(event)
		})
		return true
	case "vcsChanged":
		var event openidea.VCSChangedEvent
		if json.Unmarshal(raw, &event) != nil {
			return true
		}
		m.acceptVCSChanged(event)
		return true
	case "lspRefresh":
		var event openidea.LSPRefreshEvent
		if json.Unmarshal(raw, &event) != nil {
			return true
		}
		m.acceptLSPRefresh(event)
		return true
	case "notify":
		var event openidea.NotifyEvent
		if json.Unmarshal(raw, &event) != nil {
			return true
		}
		m.acceptNotify(event)
		return true
	case "settingsChanged":
		var event openidea.SettingsChangedEvent
		if json.Unmarshal(raw, &event) != nil {
			return true
		}
		m.acceptSettingsChanged(event)
		return true
	case "projectRootsChanged":
		var event openidea.ProjectRootsChangedEvent
		if json.Unmarshal(raw, &event) != nil {
			return true
		}
		m.acceptProjectRootsChanged(event)
		return true
	case "askpass":
		var event openidea.AskpassEvent
		if json.Unmarshal(raw, &event) != nil {
			return true
		}
		m.acceptAskpass(event)
		return true
	case "askpassDone":
		var event openidea.AskpassDoneEvent
		if json.Unmarshal(raw, &event) != nil {
			return true
		}
		m.dropAskpass(event.ID)
		return true
	default:
		return false
	}
}

func validEventRoot(root, current string) bool {
	if root == "" {
		return true
	}
	if current == "" {
		return false
	}
	return layoutKey(root) == layoutKey(current)
}

func (m *Model) acceptDBSourcesChanged(openidea.DBSourcesChangedEvent) {
	m.post(func() {
		if m.db == nil || !m.db.open {
			return
		}
		m.RefreshDBSources()
	})
}

func (m *Model) acceptProjectTrustChanged(event openidea.ProjectTrustChangedEvent) {
	m.post(func() {
		if !validEventRoot(event.Root, m.Root()) {
			return
		}
		wasTrusted := m.run.trusted
		m.run.trustErr = ""
		m.run.trusted = event.Trusted
		if event.Trusted {
			m.trustAsk = false
			m.trustFolder = false
			m.status = "This project is trusted; it may build and run"
			if !wasTrusted {
				if m.reg != nil && m.ctx != nil {
					go func() {
						ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
						defer cancel()
						if err := m.reg.ReopenAll(ctx); err != nil {
							m.post(func() {
								if m.Root() != event.Root {
									return
								}
								m.run.trustErr = "Reconnecting the language service failed: " + err.Error()
								m.invalidate()
							})
						}
					}()
				}
				m.loadRunConfigs()
				m.RefreshExternalLibraries()
			}
			m.RefreshProjectStructureSDKs()
		} else {
			m.clearTrustPrompt()
			m.run.loaded = false
			m.run.configs = nil
			m.run.sel = -1
			m.resetExternalLibraries()
			m.status = "Project trust revoked"
		}
		m.loadProjectTrust()
		m.invalidate()
	})
}

func (m *Model) acceptExcludedDirsChanged(event openidea.ExcludedDirsChangedEvent) {
	m.post(func() {
		root := m.Root()
		if root == "" || m.tree == nil {
			return
		}
		seen := map[string]bool{}
		for _, dir := range event.Dirs {
			dir = strings.TrimSpace(dir)
			if dir == "" || seen[dir] || !inProject(root, dir) {
				continue
			}
			seen[dir] = true
			m.tree.RefreshDir(dir)
		}
		if len(seen) == 0 {
			m.tree.Refresh()
		}
		m.invalidate()
	})
}

func (m *Model) acceptBackendTask(event openidea.TaskEvent) {
	root := m.Root()
	m.post(func() {
		if root == "" || root != m.Root() {
			return
		}
		state := m.runtimeEvents()
		id := backendTaskID(event.ID)
		if event.Done {
			delete(state.backendTasks, event.ID)
			m.upsertTask(BackgroundTask{ID: id, Done: true})
			m.invalidate()
			return
		}
		task := BackgroundTask{
			ID: id, Title: event.Title, Fraction: -1,
		}
		if event.Cancellable {
			task.Cancel = func() { m.cancelBackendTask(event.ID) }
		} else {
			task.Uncancellable = "This task can't be cancelled"
		}
		if event.Message != nil {
			task.Message = *event.Message
		}
		if event.Fraction != nil {
			task.Fraction = float64(*event.Fraction)
		}
		state.backendTasks[event.ID] = root
		m.upsertTask(task)
		m.invalidate()
	})
}

func (m *Model) cancelBackendTask(id uint32) {
	state := m.runtimeEvents()
	root, ok := state.backendTasks[id]
	if !ok || root != m.Root() {
		return
	}
	delete(state.backendTasks, id)
	m.markTaskCancelling(backendTaskID(id))
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 20*time.Second)
		defer cancel()
		_, _ = m.invoke(ctx, "cancel_task", map[string]any{"id": id})
	}()
}

func (m *Model) markTaskCancelling(id string) {
	for i := range m.tasks {
		task := &m.tasks[i]
		if task.ID != id || task.Done || task.Cancelling {
			continue
		}
		task.Cancelling = true
		task.Message = "Cancelling…"
		return
	}
}

func (m *Model) acceptIndexState(event openidea.IndexStateEvent) {
	m.post(func() {
		state := m.runtimeEvents()
		state.indexing = event
		if event.Indexing {
			state.indexingVisible = true
			m.upsertTask(BackgroundTask{
				ID: backendIndexingID(), Title: "Indexing",
				Message:  fmt.Sprintf("%d files · %d symbols", event.Files, event.Symbols),
				Fraction: -1, Uncancellable: "Code insight needs the index; it can't be stopped",
			})
		} else {
			state.indexingVisible = false
			m.upsertTask(BackgroundTask{ID: backendIndexingID(), Done: true})
		}
		m.refreshReadiness()
		m.invalidate()
	})
}

func (m *Model) acceptVCSChanged(event openidea.VCSChangedEvent) {
	m.post(func() {
		if !validEventRoot(event.Root, m.Root()) {
			return
		}
		m.RefreshVCS()
		m.RefreshBranch()
		if m.log.visible {
			m.RefreshLog()
		}
		m.invalidateCodeVision()
		m.clearVCSDecorations()
		m.invalidate()
	})
}

func (m *Model) clearVCSDecorations() {
	for _, t := range m.allTabs() {
		if t == nil || t.state == nil {
			continue
		}
		t.gitMarks = nil
		t.gitVersion = 0
		t.gitPath = ""
		t.gitOff = false
		t.inlay.inlineBlameLoading = false
		t.inlay.inlineBlameLoaded = false
		t.inlay.inlineBlameHasLine = false
		t.inlay.inlineBlameInlay = editor.Inlay{}
		t.inlay.inlineBlameLines = nil
		t.inlay.inlineBlameGeneration++
		applyEditorInlays(t)
		m.refreshTabDecorations(t)
	}
}

func (m *Model) acceptLSPRefresh(event openidea.LSPRefreshEvent) {
	m.post(func() {
		switch event.Kind {
		case "semanticTokens":
			m.invalidateSemanticTokens()
		case "inlayHints":
			for _, tab := range m.allTabs() {
				if tab == nil {
					continue
				}
				tab.inlay.docID = 0
				tab.inlay.version = 0
				tab.inlay.first = -1
				tab.inlay.last = -1
				tab.inlay.changed = time.Time{}
				tab.inlay.loading = false
				tab.inlay.loaded = false
				tab.inlay.retry = time.Time{}
				tab.inlay.count = 0
				tab.inlay.hints = nil
				applyEditorInlays(tab)
			}
			m.invalidate()
		case "codeLens":
			m.runtimeEvents().unsupported["codeLens"] = true
		}
	})
}

func (m *Model) acceptNotify(event openidea.NotifyEvent) {
	body := ""
	if event.Body != nil {
		body = *event.Body
	}
	actions, shownBody := backendNotificationRecovery(body)
	key := "backend-notify:" + event.Level + ":" + event.Title + ":" + body
	m.post(func() {
		switch event.Level {
		case "warning":
			m.notifyWarning(key, event.Title, shownBody, actions...)
		case "error":
			m.notifyError(key, event.Title, shownBody, actions...)
		default:
			m.notifyInfo(key, event.Title, shownBody, actions...)
		}
	})
}

func backendNotificationRecovery(body string) ([]notifyAction, string) {
	if strings.HasPrefix(body, "tsserver stopped because it reached its memory limit") {
		return []notifyAction{{
			label: "Language Servers…",
			run:   func(m *Model) { m.OpenSettingsPage(settingsPageLanguageServers) },
		}}, body
	}
	var actions []notifyAction
	if need, ok := requiredJDK(body); ok {
		version := need.version
		languageServer := need.languageServer
		actions = append(actions, notifyAction{
			label: fmt.Sprintf("Download JDK %d", version),
			run: func(m *Model) {
				m.StartJDKDownload(defaultJDKVendor, version, "", languageServer)
			},
		})
	}
	incompatible := strings.Contains(strings.ToLower(body), "can't use java") &&
		strings.Contains(strings.ToLower(body), "gradle") &&
		strings.Contains(strings.ToLower(body), "import gradle project")
	command := missingBuildToolCommand(body)
	tool := ""
	switch {
	case incompatible:
		tool = "gradle"
	case command == "mvn" || command == "mvnw" || command == "mvnd":
		tool = "maven"
	case command == "gradle" || command == "gradlew":
		tool = "gradle"
	}
	if tool == "" {
		return actions, body
	}
	label := "Open Maven Settings"
	page := settingsPageMaven
	hint := "Maven could not be started. Configure Maven home in Maven settings, install Maven on PATH, or use a project with a Maven wrapper."
	if tool == "gradle" {
		label = "Open Gradle Settings"
		page = settingsPageGradle
		hint = "Gradle could not be started. Configure a local installation in Gradle settings, install Gradle on PATH, or use a project with the Gradle wrapper."
		if incompatible {
			label = "Choose Gradle JVM"
			hint = "Choose a JDK compatible with this Gradle version in Gradle JVM. Download JDK is available in the JVM selector."
		}
	}
	actions = append(actions, notifyAction{
		label: label,
		run:   func(m *Model) { m.OpenSettingsPage(page) },
	})
	return actions, hint + "\n\n" + body
}

func missingBuildToolCommand(message string) string {
	const commandNotFound = "command not found:"
	lower := strings.ToLower(message)
	index := strings.Index(lower, commandNotFound)
	command := ""
	if index >= 0 {
		command = strings.TrimSpace(message[index+len(commandNotFound):])
		if newline := strings.IndexByte(command, '\n'); newline >= 0 {
			command = command[:newline]
		}
	}
	if command == "" {
		const cannotStart = "cannot start "
		index = strings.Index(lower, cannotStart)
		if index >= 0 {
			command = strings.TrimSpace(message[index+len(cannotStart):])
			if marker := strings.Index(command, ":"); marker >= 0 {
				command = command[:marker]
			}
		}
	}
	command = strings.Trim(command, `"'`+"`")
	command = strings.ReplaceAll(command, `\`, "/")
	return strings.ToLower(filepath.Base(command))
}

func (m *Model) acceptSettingsChanged(event openidea.SettingsChangedEvent) {
	root := m.Root()
	m.post(func() {
		if root != m.Root() {
			return
		}
		state := m.runtimeEvents()
		state.settingsGen++
		generation := state.settingsGen
		if state.settingsTimer != nil {
			state.settingsTimer()
		}
		stop := settingsEventSchedule(settingsEventDelay, func() {
			m.post(func() {
				current := m.runtimeEvents()
				if current.settingsGen != generation || m.Root() != root {
					return
				}
				current.settingsTimer = nil
				m.refreshRuntimeSettings(event.Key)
			})
		})
		state.settingsTimer = stop
	})
}

func (m *Model) refreshRuntimeSettings(key string) {
	if m.mgr == nil && m.invokeHook == nil {
		return
	}
	root := m.Root()
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 30*time.Second)
		defer cancel()
		_ = m.loadNativeRuntimeSettings(ctx)
		m.refreshFileTypeRuntime(ctx)
		if key == "" || strings.HasPrefix(key, "languages") || key == "cmake" ||
			strings.HasPrefix(key, "cmake.") || key == "maven" || strings.HasPrefix(key, "maven.") ||
			key == "gradle" || strings.HasPrefix(key, "gradle.") {
			m.post(func() {
				if m.Root() != root {
					return
				}
				m.loadBuildModel()
				m.refreshBuildToolRunConfigs()
			})
		}
		if key == "agents.mcpServers" {
			m.post(func() {
				if m.Root() == root {
					m.loadMCPServerStatus(true)
				}
			})
		}
		if key == "ssh.configs" {
			m.post(func() {
				if m.Root() != root {
					return
				}
				m.remote.configsLoaded = false
				m.loadRemoteChoices()
			})
		}
		if strings.HasPrefix(key, "editor.encodings") {
			m.post(func() {
				if m.Root() == root {
					m.invalidateFileEncodingSettings()
				}
			})
		}
		m.post(func() {
			if m.Root() == root {
				m.applyFileTypeRuntime()
			}
		})
	}()
}

func (m *Model) acceptProjectRootsChanged(event openidea.ProjectRootsChangedEvent) {
	root := m.Root()
	m.post(func() {
		if !validEventRoot(event.Root, root) || root != m.Root() {
			return
		}
		m.noteProjectRootsChanged(event)
		if m.tree != nil {
			m.tree.Refresh()
		}
		m.RefreshProjectStructureSDKs()
		m.loadBuildModel()
		m.refreshBuildToolRunConfigs()
		if m.run.trusted {
			m.loadRunConfigs()
		}
		m.invalidate()
	})
}

func (m *Model) acceptAskpass(event openidea.AskpassEvent) {
	m.post(func() {
		state := m.runtimeEvents()
		for _, pending := range state.askpass {
			if pending.id == event.ID {
				return
			}
		}
		state.askpass = append(state.askpass, askpassPrompt{
			id: event.ID, prompt: event.Prompt, kind: event.Kind, remember: event.Remember,
		})
		m.invalidate()
	})
}

func (m *Model) dropAskpass(id uint64) {
	m.post(func() {
		state := m.runtimeEvents()
		kept := state.askpass[:0]
		for _, prompt := range state.askpass {
			if prompt.id != id {
				kept = append(kept, prompt)
			}
		}
		state.askpass = kept
		m.invalidate()
	})
}

func (m *Model) currentAskpass() (askpassPrompt, bool) {
	state := m.runtimeEvents()
	if state == nil || len(state.askpass) == 0 {
		return askpassPrompt{}, false
	}
	return state.askpass[0], true
}

func init() {
	registerOverlay(&Overlay{
		ID: "ssh.askpass", Layer: overlayLayerDialog, Priority: 279, Modal: true,
		Open: func(m *Model) bool {
			_, ok := m.currentAskpass()
			return ok
		},
		Render: func(m *Model, c *ui.Context, p *ui.Theme) {
			m.askpassDialog(c, p)
		},
	})
}

func (m *Model) askpassDialog(c *ui.Context, p *ui.Theme) {
	prompt, ok := m.currentAskpass()
	if !ok {
		return
	}
	open := true
	dialog := nativeModal(c.Key("ssh-askpass-dialog"), &open, func() {
		ui.Column(c).Width(520).Gap(10).Children(func() {
			ui.Text(c, "SSH Authentication").FontSize(14).Bold().TextColor(p.Text)
			ui.Text(c, prompt.prompt).FontSize(12).TextColor(p.Text)
			kind := "Credential"
			if prompt.kind == "confirm" {
				kind = "Yes / No confirmation"
			}
			ui.Text(c, kind).FontSize(11).TextColor(p.TextMuted)
			if prompt.remember {
				ui.Text(c, "The requesting process offers to remember this answer.").
					FontSize(11).TextColor(p.TextMuted)
			}
			ui.Text(c, "This backend connection exposes credential prompts but does not expose the desktop-only answer command. No answer can be sent from this window.").
				FontSize(11).TextColor(p.TextMuted)
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Spacer(c)
				ui.Button(c, "Dismiss").OnClick(func() {
					m.dropAskpass(prompt.id)
				})
			})
		})
	})
	if dialog.Dismissed() || !open {
		m.dropAskpass(prompt.id)
	}
}
