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

// Workspace is the project grouping stored in .openidea/workspace.json.
// Projects uses the first entry as the primary attachment when it names the
// folder containing the manifest.
type Workspace struct {
	Version  uint32   `json:"version"`
	Name     string   `json:"name"`
	Projects []string `json:"projects"`
}

// DiscoveredProject is one build-root found by workspace_discover_projects.
// RelativePath is relative to the folder the user selected.
type DiscoveredProject struct {
	Path         string `json:"path"`
	Root         string `json:"root"`
	RelativePath string `json:"relativePath"`
}

// ProjectDiscovery is the reply to workspace_discover_projects. Direct keeps
// selected folders that are already projects or contain no discovered project.
type ProjectDiscovery struct {
	Direct    []string            `json:"direct"`
	Projects  []DiscoveredProject `json:"projects"`
	Truncated bool                `json:"truncated"`
}

// ProjectInfo is the part of project_current the native frontend consumes.
// Settings retain their raw JSON so future fields do not require a protocol
// change here.
type ProjectInfo struct {
	Trusted     bool            `json:"trusted"`
	Root        string          `json:"root"`
	Name        string          `json:"name"`
	DisplayName string          `json:"displayName"`
	IsGit       bool            `json:"isGit"`
	Workspace   *Workspace      `json:"workspace,omitempty"`
	Settings    json.RawMessage `json:"settings,omitempty"`
}

// DecodeProjectInfo reads project_current's reply.
func DecodeProjectInfo(raw json.RawMessage) (ProjectInfo, error) {
	var info ProjectInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		return ProjectInfo{}, err
	}
	return info, nil
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

// LanguageServerEvent reports one language server's lifecycle state.
type LanguageServerEvent struct {
	Server  uint32  `json:"server"`
	Name    string  `json:"name"`
	Label   *string `json:"label"`
	Status  string  `json:"status"`
	Message *string `json:"message"`
}

// LanguageServerWarningEvent reports a running server that lacks a capability
// the managed tool can supply.
type LanguageServerWarningEvent struct {
	Name        string `json:"name"`
	Message     string `json:"message"`
	InstallTool string `json:"installTool"`
}

// LanguageServerRemovedEvent reports a server leaving the project's list.
type LanguageServerRemovedEvent struct {
	Server uint32 `json:"server"`
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
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
	// Cwd is the working directory relative to the project root; nil is the
	// root, "" the same.
	Cwd          *string           `json:"cwd"`
	Env          map[string]string `json:"env"`
	Detected     bool              `json:"detected"`
	IsTest       bool              `json:"isTest"`
	DebugAdapter *string           `json:"debugAdapter"`
	// Params carries the kind-specific run configuration fields, such as a Go
	// module's scope. GoIDEA edits only the fields it draws, but a derived
	// context configuration must preserve the fields OpenIDEA launches with.
	Params map[string]any `json:"params,omitempty"`
	// Module is the owning module, independent of the working directory; it
	// also supplies a JVM application's classpath. nil keeps it unset; ""
	// means the project root.
	Module      *string  `json:"module"`
	BuildPolicy *string  `json:"buildPolicy"`
	EnvFiles    []string `json:"envFiles"`
	// AllowParallel lets Run on a running configuration start another
	// instance instead of restarting it.
	AllowParallel bool `json:"allowParallel,omitempty"`

	raw json.RawMessage
}

// CwdOr returns the configuration's working directory, def for an unset one.
func (r RunConfig) CwdOr(def string) string {
	if r.Cwd != nil && *r.Cwd != "" {
		return *r.Cwd
	}
	return def
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

// DecodeRunConfig decodes one run configuration object.
func DecodeRunConfig(raw json.RawMessage) (*RunConfig, error) {
	var cfg RunConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}
	cfg.raw = raw
	return &cfg, nil
}

// RunImportSource is another IDE's project file that run configurations can be
// imported from.
type RunImportSource string

const (
	RunImportVSCode    RunImportSource = "vscode"
	RunImportJetBrains RunImportSource = "jetbrains"
)

// ImportedRunConfig is one configuration read from another IDE's project file.
type ImportedRunConfig struct {
	Config  json.RawMessage `json:"config"`
	Ignored []string        `json:"ignored"`
}

// SkippedRunConfig is one source entry the backend could not translate.
type SkippedRunConfig struct {
	Name   string         `json:"name"`
	Reason map[string]any `json:"reason"`
}

// RunImport is the backend's translation of an external run configuration
// file; imported configurations remain a draft until Run/Debug Configurations
// saves them.
type RunImport struct {
	Source   RunImportSource     `json:"source"`
	File     string              `json:"file"`
	Imported []ImportedRunConfig `json:"imported"`
	Skipped  []SkippedRunConfig  `json:"skipped"`
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

// FunctionBreakpoint is one method breakpoint sent to the adapter.
type FunctionBreakpoint struct {
	Name         string  `json:"name"`
	Condition    *string `json:"condition"`
	HitCondition *string `json:"hitCondition"`
	Enabled      bool    `json:"enabled"`
}

// DataBreakpoint is one field watchpoint sent to the adapter.
type DataBreakpoint struct {
	DataID       string  `json:"dataId"`
	AccessType   *string `json:"accessType"`
	Condition    *string `json:"condition"`
	HitCondition *string `json:"hitCondition"`
	Enabled      bool    `json:"enabled"`
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

// StepInTarget is one call on the current line that Smart Step Into can enter.
type StepInTarget struct {
	ID        int64  `json:"id"`
	Label     string `json:"label"`
	Line      *int   `json:"line"`
	Column    *int   `json:"column"`
	EndColumn *int   `json:"endColumn"`
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
	// HotSwap ("Reload Changed Classes"): java-debug's `redefineClasses`
	// request.
	ChangeType string `json:"changeType"`
}

// HotSwapResult is the reply to debug_redefine_classes: the classes reloaded
// (empty: nothing had changed) and java-debug's error, if any.
type HotSwapResult struct {
	ChangedClasses []string `json:"changedClasses"`
	ErrorMessage   *string  `json:"errorMessage"`
}

// AttachableProcess is one entry of debug_attachable_processes: a process a
// debugger can attach to.
type AttachableProcess struct {
	PID         int     `json:"pid"`
	Name        string  `json:"name"`
	CommandLine string  `json:"commandLine"`
	Kind        string  `json:"kind"` // java | node | python | go | native
	Adapter     string  `json:"adapter"`
	Host        *string `json:"host"`
	Port        *int    `json:"port"`
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

// SearchPosition is one 0-based line and UTF-16 column used as a replace
// target.
type SearchPosition struct {
	Line uint32 `json:"line"`
	Col  uint32 `json:"col"`
}

// ReplaceTarget asks replace_in_files to change one file. A nil Only list
// replaces every remaining match in the file.
type ReplaceTarget struct {
	Path string           `json:"path"`
	Only []SearchPosition `json:"only,omitempty"`
}

// ReplaceSummary is the final result of replace_in_files.
type ReplaceSummary struct {
	Files        uint32  `json:"files"`
	Replacements uint32  `json:"replacements"`
	Failed       uint32  `json:"failed"`
	Cancelled    bool    `json:"cancelled"`
	ElapsedMs    float64 `json:"elapsedMs"`
}

// ReplaceEvent is one streamed message from replace_in_files.
type ReplaceEvent struct {
	Type         string          `json:"type"`
	Path         string          `json:"path"`
	Replacements uint32          `json:"replacements"`
	Done         uint32          `json:"done"`
	Total        uint32          `json:"total"`
	Message      string          `json:"message"`
	Summary      *ReplaceSummary `json:"summary"`
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

// BlameLine is one line of vcs_blame: the commit that last touched it, its
// author, the commit time, and its summary. An all-zero commit means the line
// is not committed yet.
type BlameLine struct {
	Commit  string  `json:"commit"`
	Author  string  `json:"author"`
	TimeMs  float64 `json:"timeMs"`
	Summary string  `json:"summary"`
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

// Commit is one row of vcs_log: the id and summary the log list shows, the
// refs pointing at it, and what the commit details view needs.
type Commit struct {
	ID        string   `json:"id"`
	ShortID   string   `json:"shortId"`
	Parents   []string `json:"parents"`
	Author    string   `json:"author"`
	Email     string   `json:"email"`
	TimeMs    float64  `json:"timeMs"`
	Summary   string   `json:"summary"`
	Message   string   `json:"message"`
	Refs      []string `json:"refs"`
	OnCurrent *bool    `json:"onCurrentBranch"`
}

// LogDetails is the selected commit's full message, the branches that contain
// it, and Git's signature verification of it.
type LogDetails struct {
	Message            string   `json:"message"`
	ContainingBranches []string `json:"containingBranches"`
	Signature          string   `json:"signature"`
	Signer             string   `json:"signer"`
	Key                string   `json:"key"`
	Fingerprint        string   `json:"fingerprint"`
}

// RevisionChange is one file a commit changed: Kind is added, modified,
// deleted, renamed, untracked, ignored or conflicted.
type RevisionChange struct {
	Path    string  `json:"path"`
	Kind    string  `json:"kind"`
	OldPath *string `json:"oldPath"`
}

// BranchInfo is the current head `vcs_branch_info` reports: the branch name
// (absent on a detached head), its tip, its upstream and how far it is from
// it, and a merge, rebase or cherry-pick in progress.
type BranchInfo struct {
	Head       *string `json:"head"`
	HeadCommit *string `json:"headCommit"`
	Upstream   *string `json:"upstream"`
	Ahead      int     `json:"ahead"`
	Behind     int     `json:"behind"`
	Operation  *string `json:"operation"`
}

// Branch is one row of `vcs_branches`: a local or remote branch, whether it
// is the head, and how far it is from its upstream.
type Branch struct {
	Name         string  `json:"name"`
	Remote       bool    `json:"remote"`
	Current      bool    `json:"current"`
	Upstream     *string `json:"upstream"`
	Ahead        int     `json:"ahead"`
	Behind       int     `json:"behind"`
	LastCommitMs float64 `json:"lastCommitMs"`
	Commit       *string `json:"commit"`
}

// DeletedBranch is the restorable branch payload vcs_delete_branch returns.
// OpenIDEA's branch-deleted notification uses it to recreate the exact tip and
// upstream after an unmerged branch was removed.
type DeletedBranch struct {
	Name     string  `json:"name"`
	Tip      string  `json:"tip"`
	Upstream *string `json:"upstream"`
	Unmerged bool    `json:"unmerged"`
}

// Remote is one Git remote `vcs_remotes` lists.
type Remote struct {
	Name        string  `json:"name"`
	URL         string  `json:"url"`
	ResolvedURL *string `json:"resolvedUrl"`
}

// Stash is one entry of `vcs_stashes`.
type Stash struct {
	Index   uint32  `json:"index"`
	Message string  `json:"message"`
	TimeMs  float64 `json:"timeMs"`
}

// Worktree is one Git worktree `vcs_worktrees` lists.
type Worktree struct {
	Path     string  `json:"path"`
	Head     *string `json:"head"`
	Branch   *string `json:"branch"`
	Main     bool    `json:"main"`
	Locked   bool    `json:"locked"`
	Prunable bool    `json:"prunable"`
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
	Locations        []Location            `json:"locations"`
	Classified       []ClassifiedReference `json:"classified"`
	Incomplete       *string               `json:"incomplete"`
	LibrariesSkipped bool                  `json:"librariesSkipped"`
}

// ClassifiedReference is the semantic metadata a Java extension attaches to
// one References location. Older servers omit it, and unknown locations stay
// visible under either access filter.
type ClassifiedReference struct {
	Location Location `json:"location"`
	Kind     string   `json:"kind"`
	Access   string   `json:"access"`
	Text     *string  `json:"text"`
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
