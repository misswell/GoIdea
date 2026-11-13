package openidea

import (
	"encoding/json"
	"fmt"

	"mayoi/internal/position"
)

// ServerInfo is the reply to initialize.
type ServerInfo struct {
	Version  string `json:"version"`
	Protocol uint32 `json:"protocol"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	Home     string `json:"home"`
}

// DocInfo is the reply to doc_open and doc_snapshot.
type DocInfo struct {
	ID         uint32 `json:"id"`
	Path       string `json:"path"`
	Text       string `json:"text"`
	Version    uint32 `json:"version"`
	Language   string `json:"language"`
	Encoding   string `json:"encoding"`
	HasBOM     bool   `json:"hasBom"`
	LineEnding string `json:"lineEnding"`
	ReadOnly   bool   `json:"readOnly"`
	Dirty      bool   `json:"dirty"`
	Large      bool   `json:"large"`
	URI        string `json:"uri"`
}

// EventType returns the discriminant of an event payload.
func EventType(params json.RawMessage) string {
	var envelope struct {
		Type string `json:"type"`
	}
	_ = json.Unmarshal(params, &envelope)
	return envelope.Type
}

// DocPatchedEvent is the backend having changed a document (formatting,
// refactoring, reload). Changes are UTF-16 into baseVersion and yield version.
type DocPatchedEvent struct {
	Doc           uint32                `json:"doc"`
	BaseVersion   uint32                `json:"baseVersion"`
	Version       uint32                `json:"version"`
	Changes       []position.TextChange `json:"changes"`
	WorkspaceEdit bool                  `json:"workspaceEdit"`
}

// DecodeDocPatched decodes a docPatched event.
func DecodeDocPatched(params json.RawMessage) (DocPatchedEvent, error) {
	var e DocPatchedEvent
	err := json.Unmarshal(params, &e)
	return e, err
}

// DocDirtyEvent reports a document's dirty flag changing.
type DocDirtyEvent struct {
	Doc   uint32 `json:"doc"`
	Dirty bool   `json:"dirty"`
}

// FsChangedEvent reports files changed on disk, after ignore rules.
type FsChangedEvent struct {
	Created  []string `json:"created"`
	Modified []string `json:"modified"`
	Removed  []string `json:"removed"`
}

// RunExitedEvent reports a run configuration's process ending.
type RunExitedEvent struct {
	Run  uint32 `json:"run"`
	Code *int32 `json:"code"`
}

// BuildEvent is one step of a build streamed on build_run's onEvent channel.
// Kind selects the fields that are meaningful; Message is a BuildMessage for
// "message" and the status text for "progress".
type BuildEvent struct {
	Kind    string          `json:"kind"` // started | step | output | message | progress
	System  string          `json:"system"`
	Title   string          `json:"title"`
	Command string          `json:"command"`
	Cwd     string          `json:"cwd"`
	Text    string          `json:"text"`
	Stderr  bool            `json:"stderr"`
	Message json.RawMessage `json:"message"`
}

// BuildMessage is one compiler message of a build, in 0-based lines and
// UTF-16 columns.
type BuildMessage struct {
	Severity string        `json:"severity"`
	Text     string        `json:"text"`
	Path     *string       `json:"path"`
	Range    *OutlineRange `json:"range"`
	Code     *string       `json:"code"`
}

// BuildResult is build_model/build_run's answer: whether the build succeeded,
// how many messages it found and how long it took.
type BuildResult struct {
	System     string         `json:"system"`
	Title      string         `json:"title"`
	Success    bool           `json:"success"`
	Errors     uint32         `json:"errors"`
	Warnings   uint32         `json:"warnings"`
	DurationMS float64        `json:"durationMs"`
	Messages   []BuildMessage `json:"messages"`
}

// IndexStateEvent reports the project indexer's state.
type IndexStateEvent struct {
	Indexing bool   `json:"indexing"`
	Files    uint32 `json:"files"`
	Symbols  uint32 `json:"symbols"`
}

// WindowEvent is the params of an event notification.
type WindowEvent struct {
	Window *string         `json:"window"`
	Event  json.RawMessage `json:"event"`
}

// InvokeArgs is the params of invoke.
type InvokeArgs struct {
	Window string `json:"window"`
	Cmd    string `json:"cmd"`
	Args   any    `json:"args,omitempty"`
}

func (i InvokeArgs) String() string { return fmt.Sprintf("%s in %s", i.Cmd, i.Window) }

// ChannelMessage is a `channel` notification: one value of a streaming
// invoke argument, by the channel id the caller put in the arguments.
type ChannelMessage struct {
	ID   uint32          `json:"id"`
	Data json.RawMessage `json:"data"`
}

// OutputChunk is one piece of a run configuration's output.
type OutputChunk struct {
	Kind string `json:"kind"` // stdout | stderr | system
	Text string `json:"text"`
}

// RunConfig is a run configuration as run_configs returns it. Raw keeps the
// whole object so run_start gets back every field, including the ones GoIDEA
// does not model.
type RunConfig struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Kind         string  `json:"kind"`
	Command      string  `json:"command"`
	Detected     bool    `json:"detected"`
	IsTest       bool    `json:"isTest"`
	DebugAdapter *string `json:"debugAdapter"`

	raw json.RawMessage
}

// Raw is the configuration exactly as the backend sent it.
func (r RunConfig) Raw() json.RawMessage { return r.raw }

// DecodeRunConfigs decodes the run_configs reply.
func DecodeRunConfigs(raw json.RawMessage) ([]RunConfig, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	out := make([]RunConfig, 0, len(items))
	for _, item := range items {
		var cfg RunConfig
		if err := json.Unmarshal(item, &cfg); err != nil {
			return nil, err
		}
		cfg.raw = item
		out = append(out, cfg)
	}
	return out, nil
}

// RunInfo is the reply to run_start.
type RunInfo struct {
	ID       uint32 `json:"id"`
	ConfigID string `json:"configId"`
	Name     string `json:"name"`
	PID      *int32 `json:"pid"`
	Running  bool   `json:"running"`
	Pty      bool   `json:"pty"`
}

// TrustStatus is the reply to project_trust_status.
type TrustStatus struct {
	Decided bool   `json:"decided"`
	Root    string `json:"root"`
	Trusted bool   `json:"trusted"`
}

// DebugLaunchConfig is a debug adapter session start: what to run or attach to,
// and extraJson for the adapter's own fields (java-debug's debugServer port,
// main class, classpath).
type DebugLaunchConfig struct {
	Name      string            `json:"name"`
	Adapter   string            `json:"adapter"`
	Request   string            `json:"request"`
	Program   *string           `json:"program"`
	Args      []string          `json:"args"`
	Cwd       string            `json:"cwd"`
	Env       map[string]string `json:"env"`
	ExtraJSON *string           `json:"extraJson"`
}

// Breakpoint is one line breakpoint; Line is 0-based, as the editor's.
type Breakpoint struct {
	Line         int     `json:"line"`
	Condition    *string `json:"condition"`
	HitCondition *string `json:"hitCondition"`
	LogMessage   *string `json:"logMessage"`
	Enabled      bool    `json:"enabled"`
}

// BreakpointState is the adapter's verdict on a breakpoint.
type BreakpointState struct {
	Line     int     `json:"line"`
	Verified bool    `json:"verified"`
	Message  *string `json:"message"`
}

// JavaCompileError is one compile error the launch build found.
type JavaCompileError struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Message string `json:"message"`
}

// JavaLaunchInfo is the reply to java_resolve_launch: the built classpath and
// JDK of a module, plus what the build found.
type JavaLaunchInfo struct {
	Build              *string            `json:"build"`
	ProjectRoot        *string            `json:"projectRoot"`
	ClassPaths         []string           `json:"classPaths"`
	ModulePaths        []string           `json:"modulePaths"`
	PathListSeparator  string             `json:"pathListSeparator"`
	JavaHome           *string            `json:"javaHome"`
	JavaExec           *string            `json:"javaExec"`
	ShortenCommandLine *string            `json:"shortenCommandLine"`
	Errors             []JavaCompileError `json:"errors"`
	ErrorCount         int                `json:"errorCount"`
	Hints              []string           `json:"hints"`
}

// ThreadInfo is one thread of a debug session.
type ThreadInfo struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// StackFrame is one frame of a thread; Line and Column are 0-based.
type StackFrame struct {
	ID               int64   `json:"id"`
	Name             string  `json:"name"`
	Path             *string `json:"path"`
	Line             int     `json:"line"`
	Column           int     `json:"column"`
	PresentationHint *string `json:"presentationHint"`
}

// Scope is one variable scope of a frame, as Locals or "this".
type Scope struct {
	Name               string `json:"name"`
	VariablesReference int64  `json:"variablesReference"`
	Expensive          bool   `json:"expensive"`
}

// Variable is one variable, or a value with children when
// VariablesReference is positive.
type Variable struct {
	Name               string  `json:"name"`
	Value              string  `json:"value"`
	TypeName           *string `json:"typeName"`
	VariablesReference int64   `json:"variablesReference"`
	EvaluateName       *string `json:"evaluateName"`
}

// DebugEnvelope is the `debug` event's payload: the session it belongs to and
// the adapter event.
type DebugEnvelope struct {
	Session uint32          `json:"session"`
	Event   json.RawMessage `json:"event"`
}

// DebugEvent is one DAP event, flattened. Kind selects the fields that are
// meaningful: "stopped" has ThreadID/Reason/Description, "output" has
// Category/Output, "exited" has Code.
type DebugEvent struct {
	Kind              string `json:"kind"`
	Name              string `json:"name"`
	Adapter           string `json:"adapter"`
	ThreadID          int64  `json:"threadId"`
	AllThreadsStopped bool   `json:"allThreadsStopped"`
	Reason            string `json:"reason"`
	Description       string `json:"description"`
	Category          string `json:"category"`
	Output            string `json:"output"`
	Path              string `json:"path"`
	Code              *int32 `json:"code"`
	Message           string `json:"message"`
}

// LineMatch is one line a search matched: 0-based line, its text, and the
// UTF-16 column ranges within that text.
type LineMatch struct {
	Line       uint32      `json:"line"`
	Text       string      `json:"text"`
	Ranges     [][2]uint32 `json:"ranges"`
	TextOffset uint32      `json:"textOffset"`
}

// FileMatches is one file's search hits.
type FileMatches struct {
	Path    string      `json:"path"`
	RelPath string      `json:"relPath"`
	Matches []LineMatch `json:"matches"`
}

// SearchSummary ends a search: how much was covered and whether it stopped.
type SearchSummary struct {
	Files     uint32  `json:"files"`
	Matches   uint32  `json:"matches"`
	LimitHit  bool    `json:"limitHit"`
	Cancelled bool    `json:"cancelled"`
	ElapsedMs float64 `json:"elapsedMs"`
}

// SearchEvent is one channel message of search_start: a file's matches, then
// the summary.
type SearchEvent struct {
	Type    string         `json:"type"`
	File    *FileMatches   `json:"file"`
	Summary *SearchSummary `json:"summary"`
}

// OutlinePos is a 0-based line and UTF-16 column of an outline range.
type OutlinePos struct {
	Line uint32 `json:"line"`
	Col  uint32 `json:"col"`
}

// OutlineRange is a half-open range of an outline item.
type OutlineRange struct {
	Start OutlinePos `json:"start"`
	End   OutlinePos `json:"end"`
}

// OutlineItem is a node of a document's outline, for breadcrumbs and the
// structure view.
type OutlineItem struct {
	Name           string        `json:"name"`
	Detail         *string       `json:"detail"`
	Kind           string        `json:"kind"`
	Range          OutlineRange  `json:"range"`
	SelectionRange OutlineRange  `json:"selection_range"`
	Children       []OutlineItem `json:"children"`
}

// LineHunk is one line-level change between two texts (0-based lines): start
// and lines index the new text, oldStart and oldLines the original.
type LineHunk struct {
	Kind     string `json:"kind"`
	Start    uint32 `json:"start"`
	Lines    uint32 `json:"lines"`
	OldStart uint32 `json:"oldStart"`
	OldLines uint32 `json:"oldLines"`
	OldText  string `json:"oldText"`
}

// FileStatus is one changed file of a Git repository: the two change kinds
// are staged (index) and unstaged (working tree), either may be absent.
type FileStatus struct {
	Path        string  `json:"path"`
	RelPath     string  `json:"relPath"`
	Staged      *string `json:"staged"`
	Unstaged    *string `json:"unstaged"`
	RenamedFrom *string `json:"renamedFrom"`
}

// CommitOptions is what vcs_commit does beyond the message: amend rewrites the
// previous commit, signOff adds a Signed-off-by line, author overrides the
// configured user, runHooks runs the Git hooks, stage commits files with their
// working-tree content, and exclude keeps staged files out of the commit.
type CommitOptions struct {
	Amend    bool     `json:"amend"`
	SignOff  bool     `json:"signOff"`
	Author   *string  `json:"author"`
	RunHooks bool     `json:"runHooks"`
	Stage    []string `json:"stage,omitempty"`
	Exclude  []string `json:"exclude,omitempty"`
}

// TerminalInfo is an integrated terminal the server started: its id, title
// and process id (absent for shells without a direct child).
type TerminalInfo struct {
	ID    uint32 `json:"id"`
	Title string `json:"title"`
	PID   *int32 `json:"pid"`
}

// TerminalExitedEvent is the `terminalExited` window event: the shell ended
// with an exit code, or without one when it was killed.
type TerminalExitedEvent struct {
	Terminal uint32 `json:"terminal"`
	Code     *int32 `json:"code"`
}

// Markup is text a language server wrote: "markdown" or "plaintext".
type Markup struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// Hover is lsp_hover's answer: what a symbol is, and the range it covers.
type Hover struct {
	Contents Markup        `json:"contents"`
	Range    *OutlineRange `json:"range"`
}

// CompletionList is lsp_completion's answer.
type CompletionList struct {
	Items        []CompletionItem `json:"items"`
	IsIncomplete bool             `json:"isIncomplete"`
}

// CompletionItem is one suggestion of a completion list; only what the editor
// shows is kept.
type CompletionItem struct {
	Label            string  `json:"label"`
	LabelDetail      *string `json:"labelDetail"`
	LabelDescription *string `json:"labelDescription"`
	Detail           *string `json:"detail"`
	Kind             *string `json:"kind"`
	InsertText       string  `json:"insertText"`
	IsSnippet        bool    `json:"isSnippet"`
	FilterText       *string `json:"filterText"`
	Documentation    *Markup `json:"documentation"`
}

// Location is a place a language server points at: a file and a range of it.
// Virtual targets (an entry of a jar, for one) carry a URI instead of a path a
// tab can open.
type Location struct {
	Path  string       `json:"path"`
	Range OutlineRange `json:"range"`
	URI   *string      `json:"uri"`
}

// DiagnosticsEvent is the complete diagnostics of one file from one producer,
// replacing that producer's earlier ones.
type DiagnosticsEvent struct {
	Path   string       `json:"path"`
	Source string       `json:"source"`
	Items  []Diagnostic `json:"items"`
}

// Diagnostic is a problem with a range of a file: 0-based lines and UTF-16
// columns, as every backend range is.
type Diagnostic struct {
	Range    OutlineRange `json:"range"`
	Severity string       `json:"severity"`
	Message  string       `json:"message"`
	Source   *string      `json:"source"`
	Code     *string      `json:"code"`
	Server   *string      `json:"server"`
}

// TextEdit replaces a range of a file's text, in UTF-16 coordinates.
type TextEdit struct {
	Range   OutlineRange `json:"range"`
	NewText string       `json:"new_text"`
}

// Renamed is lsp_rename's answer: the edit that would rename the symbol, left
// unapplied so the UI can show what it touches. Preview asks for confirmation
// before it is applied, and Incomplete names what the rename may have missed.
type Renamed struct {
	Preview    bool            `json:"preview"`
	Edit       json.RawMessage `json:"edit"`
	Incomplete *string         `json:"incomplete"`
}

// WorkspaceEditAppliedEvent is the `workspaceEditApplied` window event: the id
// undo_workspace_edit takes, and the files the edit touched.
type WorkspaceEditAppliedEvent struct {
	ID    uint32   `json:"id"`
	Paths []string `json:"paths"`
}

// References is lsp_references' answer: where the symbol is used. A streaming
// reply replaces Locations as the search moves on, and Incomplete names what
// the query may have missed.
type References struct {
	Locations        []Location `json:"locations"`
	Incomplete       *string    `json:"incomplete"`
	LibrariesSkipped bool       `json:"librariesSkipped"`
}

// WorkspaceEditInfo is the part of a workspace edit the UI previews: which
// files it edits, and for renamed or deleted ones, which paths it moves.
type WorkspaceEditInfo struct {
	Files []FileEditInfo `json:"files"`
}

// FileEditInfo is one file of a workspace edit.
type FileEditInfo struct {
	Kind string `json:"kind"`
	Path string `json:"path"`
	From string `json:"from"`
	To   string `json:"to"`
}

// WorkspaceSymbol is one hit of lsp_workspace_symbols: a named declaration and
// where it lives.
type WorkspaceSymbol struct {
	Name      string   `json:"name"`
	Kind      string   `json:"kind"`
	Container *string  `json:"container"`
	Location  Location `json:"location"`
}

// CodeActionQuery is lsp_code_action_query's answer: the actions the servers
// offer at a range, kept raw for lsp_apply_code_action, and the kinds they
// advertise.
type CodeActionQuery struct {
	Actions         []json.RawMessage `json:"actions"`
	AdvertisedKinds *[]string         `json:"advertisedKinds"`
}

// FileMatch is one hit of find_files: an indexed file and how well its relative
// path matched, with the matched char indices for highlighting.
type FileMatch struct {
	Path    string   `json:"path"`
	RelPath string   `json:"relPath"`
	Score   uint32   `json:"score"`
	Indices []uint32 `json:"indices,omitempty"`
}
