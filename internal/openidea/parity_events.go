package openidea

import (
	"encoding/json"
	"fmt"
)

type ProjectTrustChangedEvent struct {
	Root    string `json:"root"`
	Trusted bool   `json:"trusted"`
}
type DBSourcesChangedEvent struct{}
type ExcludedDirsChangedEvent struct {
	Dirs []string `json:"dirs"`
}
type DocLanguageEvent struct {
	Doc      uint32  `json:"doc"`
	Language string  `json:"language"`
	UIMode   *string `json:"uiMode"`
}
type TaskEvent struct {
	ID          uint32   `json:"id"`
	Title       string   `json:"title"`
	Message     *string  `json:"message"`
	Fraction    *float32 `json:"fraction"`
	Done        bool     `json:"done"`
	Cancellable bool     `json:"cancellable"`
}
type VCSChangedEvent struct {
	Root string `json:"root"`
}
type LSPRefreshEvent struct {
	Server uint32 `json:"server"`
	Kind   string `json:"kind"`
}
type DebugSessionEvent struct {
	Session uint32          `json:"session"`
	Event   json.RawMessage `json:"event"`
}
type NotifyEvent struct {
	Level string  `json:"level"`
	Title string  `json:"title"`
	Body  *string `json:"body"`
}
type SettingsChangedEvent struct {
	Key string `json:"key"`
}
type UpdateStatusChangedEvent struct {
	Status json.RawMessage `json:"status"`
}
type UpdateAvailableEvent struct {
	Update json.RawMessage `json:"update"`
}
type AskpassEvent struct {
	ID       uint64 `json:"id"`
	Prompt   string `json:"prompt"`
	Kind     string `json:"kind"`
	Remember bool   `json:"remember"`
}
type AskpassDoneEvent struct {
	ID uint64 `json:"id"`
}

// DecodeIDEEvent decodes an event payload into its corresponding event type.
// Unrecognized event types remain raw so newer server events are not lost.
// Decoding does not imply that the app handles an event.
func DecodeIDEEvent(raw json.RawMessage) (string, any, error) {
	var header struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &header); err != nil {
		return "", nil, err
	}
	if header.Type == "" {
		return "", nil, fmt.Errorf("event has no type")
	}
	var event any
	switch header.Type {
	case "mcpRequest":
		event = &McpRequestEvent{}
	case "mcpRequestClosed":
		event = &McpRequestClosedEvent{}
	case "projectTrustChanged":
		event = &ProjectTrustChangedEvent{}
	case "dbSourcesChanged":
		event = &DBSourcesChangedEvent{}
	case "fsChanged":
		event = &FsChangedEvent{}
	case "excludedDirsChanged":
		event = &ExcludedDirsChangedEvent{}
	case "docPatched":
		event = &DocPatchedEvent{}
	case "docDirty":
		event = &DocDirtyEvent{}
	case "workspaceEditApplied":
		event = &WorkspaceEditAppliedEvent{}
	case "docLanguage":
		event = &DocLanguageEvent{}
	case "diagnostics":
		event = &DiagnosticsEvent{}
	case "task":
		event = &TaskEvent{}
	case "indexState":
		event = &IndexStateEvent{}
	case "vcsChanged":
		event = &VCSChangedEvent{}
	case "languageServer":
		event = &LanguageServerEvent{}
	case "languageServerWarning":
		event = &LanguageServerWarningEvent{}
	case "languageServerRemoved":
		event = &LanguageServerRemovedEvent{}
	case "lspRefresh":
		event = &LSPRefreshEvent{}
	case "terminalExited":
		event = &TerminalExitedEvent{}
	case "runExited":
		event = &RunExitedEvent{}
	case "debug":
		event = &DebugSessionEvent{}
	case "notify":
		event = &NotifyEvent{}
	case "settingsChanged":
		event = &SettingsChangedEvent{}
	case "updateStatusChanged":
		event = &UpdateStatusChangedEvent{}
	case "updateAvailable":
		event = &UpdateAvailableEvent{}
	case "projectRootsChanged":
		event = &ProjectRootsChangedEvent{}
	case "askpass":
		event = &AskpassEvent{}
	case "askpassDone":
		event = &AskpassDoneEvent{}
	case "notebookKernel":
		event = &NotebookKernelEvent{}
	default:
		return header.Type, append(json.RawMessage(nil), raw...), nil
	}
	if err := json.Unmarshal(raw, event); err != nil {
		return header.Type, nil, err
	}
	return header.Type, event, nil
}
