package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// AgentsPane mirrors OpenIDEA's Agents tool window and its editor surface.
// The backend remains the source of truth: tasks, providers and every timeline
// entry arrive from agent_* RPCs, and actions are sent back unchanged.
type AgentsPane struct {
	visible       bool
	connectedRoot string
	channel       uint32
	generation    uint64
	loading       bool
	loaded        bool
	err           string
	actionErr     string

	tasks  map[string]agentTask
	unread map[string]bool
	// limits are the account windows each provider reported on the limits
	// event, and timelineOpen remembers folds the reader opened.
	limits       map[string][]agentLimit
	timelineOpen map[string]bool

	providers        []agentProvider
	providersLoading bool
	providersErr     string

	selected           string
	selectedSubagents  []string
	selectedTasks      []string
	selectionAnchor    string
	taskDragID         string
	taskDragTasks      []string
	taskDragStart      ui.Point
	taskDragAt         ui.Point
	taskDropTarget     string
	conversation       bool
	filter             string
	findOpen           bool
	findText           string
	findMatch          int
	conversationScroll ui.ScrollState
	entries            map[string][]agentEntry
	entriesLoaded      map[string]bool
	entriesBusy        map[string]bool
	entriesErr         map[string]string
	composer           string
	composerFiles      []string
	composerImages     []agentImage
	composerDrafts     map[string]agentComposerDraft
	composerBusy       bool
	composerCaret      int
	commandsBusy       map[string]bool
	commandsLoaded     map[string]bool
	commandsErr        map[string]string
	commandsSeq        map[string]uint64
	completion         agentCompletionState
	completionCaret    int
	completionCaretSet bool
	historyIndex       int
	historyStash       string
	taskBusy           map[string]bool

	changesTask string
	changes     []agentChange
	changesBusy bool
	changesErr  string
	// changesByTask and groupedChanges keep the review data OpenIDEA loads as
	// soon as a task opens. The singular fields above remain the dialog's
	// current snapshot.
	changesByTask   map[string][]agentChange
	groupedChanges  map[string]agentChanges
	changesLoaded   map[string]bool
	changesLoading  map[string]bool
	changesErrors   map[string]string
	changesSequence map[string]uint64
	changesFold     map[string]bool
	changesTimers   map[string]*time.Timer

	externalOpen    bool
	externalLoading bool
	externalRequest uint64
	externalLoaded  bool
	externalBusy    string
	external        []agentExternalSession
	externalErr     string

	newOpen             bool
	newProvider         string
	newIsolation        string
	newPrompt           string
	newFiles            []string
	newImages           []agentImage
	newConfig           map[string]string
	newCommands         []agentCommand
	newCommandsLoading  bool
	newCommandsProvider string
	newCommandsErr      string
	newCommandsSeq      uint64
	newFileBusy         bool
	newBusy             bool
	newErr              string
	renameOpen          bool
	renameTask          string
	renameTitle         string
	groupOpen           bool
	groupMode           string
	groupID             string
	groupName           string
	groupTasks          []string
	groupErr            string
	removeGroupOpen     bool
	removeGroupID       string
	deleteOpen          bool
	deleteTasks         []string
	deleteTask          string
	deleteWorktree      bool
	changesOpen         bool
	editOpen            bool
	editTask            string
	editIndex           int
	editText            string
	editImages          []agentImage
	editBusy            bool
	editErr             string

	mcp        agentMCPSettingsState
	registry   agentRegistryState
	imageCache map[string]*ui.Bitmap
	imageOrder []string
	// markdownImages keeps decoded images from replies. Remote images are
	// absent until the reader consents to loading their address.
	markdownImages     map[string]*agentMarkdownImageState
	markdownImageOrder []string
	markdownAsked      map[string]bool
	// markdownDiagrams renders Mermaid away from the UI thread and keeps
	// the result across streamed chunks and frames.
	markdownDiagrams     map[string]*agentMarkdownDiagramState
	markdownDiagramOrder []string

	revealTexts map[string]*agentRevealState
	revealTimer *time.Timer
	meterTimer  *time.Timer
}

type agentTask struct {
	ProjectRoot  *string             `json:"projectRoot"`
	ID           string              `json:"id"`
	Title        string              `json:"title"`
	Provider     string              `json:"provider"`
	Model        *string             `json:"model"`
	Status       string              `json:"status"`
	Isolation    string              `json:"isolation"`
	Cwd          string              `json:"cwd"`
	Branch       *string             `json:"branch"`
	Base         *string             `json:"base"`
	BaseCommit   *string             `json:"baseCommit"`
	Error        *string             `json:"error"`
	CreatedMS    float64             `json:"createdMs"`
	UpdatedMS    float64             `json:"updatedMs"`
	Config       []agentConfigOption `json:"config"`
	Commands     []agentCommand      `json:"commands"`
	Usage        *agentUsage         `json:"usage"`
	Queue        []agentQueued       `json:"queue"`
	Capabilities agentCapabilities   `json:"capabilities"`
	ResetsMS     *float64            `json:"resetsMs"`
	Retry        *agentRetry         `json:"retry"`
	Background   []agentBackground   `json:"background"`
}

type agentCapabilities struct {
	Images          bool `json:"images"`
	Fork            bool `json:"fork"`
	Rollback        bool `json:"rollback"`
	Steer           bool `json:"steer"`
	BackgroundTasks bool `json:"backgroundTasks"`
}

type agentConfigOption struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Description *string            `json:"description"`
	Category    *string            `json:"category"`
	Current     *string            `json:"current"`
	Values      []agentConfigValue `json:"values"`
	Via         string             `json:"via"`
}

type agentConfigValue struct {
	Value       string  `json:"value"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

type agentConfigChoice struct {
	ID    string `json:"id"`
	Value string `json:"value"`
}

type agentCommand struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Hint        *string `json:"hint"`
	Source      string  `json:"source"`
}

type agentUsage struct {
	Used uint32     `json:"used"`
	Size uint32     `json:"size"`
	Cost *agentCost `json:"cost"`
}

type agentCost struct {
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
}

type agentLimit struct {
	Window      string   `json:"window"`
	UsedPercent *float64 `json:"usedPercent"`
	ResetsMS    *float64 `json:"resetsMs"`
}

type agentQueued struct {
	ID     string      `json:"id"`
	Prompt agentPrompt `json:"prompt"`
}

type agentPrompt struct {
	Text   string   `json:"text"`
	Files  []string `json:"files"`
	Images []any    `json:"images"`
}

type agentRetry struct {
	Attempt    uint32  `json:"attempt"`
	MaxRetries *uint32 `json:"maxRetries"`
	RetryAtMS  float64 `json:"retryAtMs"`
	Error      *string `json:"error"`
}

type agentBackground struct {
	ID          string  `json:"id"`
	Kind        string  `json:"kind"`
	Description string  `json:"description"`
	StartedMS   float64 `json:"startedMs"`
}

type agentProvider struct {
	ID                    string                         `json:"id"`
	Name                  string                         `json:"name"`
	Command               string                         `json:"command,omitempty"`
	Program               string                         `json:"program"`
	Args                  []string                       `json:"args,omitempty"`
	DefaultProgram        *string                        `json:"defaultProgram"`
	DefaultArgs           []string                       `json:"defaultArgs,omitempty"`
	Installed             bool                           `json:"installed"`
	LoginHint             *string                        `json:"loginHint"`
	Config                []agentConfigOption            `json:"config,omitempty"`
	ThoughtLevels         map[string][]agentConfigOption `json:"thoughtLevels,omitempty"`
	Commands              []agentCommand                 `json:"commands,omitempty"`
	Source                agentSource                    `json:"source,omitempty"`
	CommandEditable       bool                           `json:"commandEditable,omitempty"`
	CommandOverridden     bool                           `json:"commandOverridden,omitempty"`
	CommandReadOnlyReason *string                        `json:"commandReadOnlyReason"`
	Env                   map[string]string              `json:"env,omitempty"`
}

type agentSource struct {
	Kind    string  `json:"kind"`
	Version string  `json:"version,omitempty"`
	Update  *string `json:"update,omitempty"`
}

func (s *agentSource) UnmarshalJSON(data []byte) error {
	var kind string
	if json.Unmarshal(data, &kind) == nil {
		s.Kind = kind
		return nil
	}
	type plain agentSource
	var value plain
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*s = agentSource(value)
	return nil
}

type agentEntry struct {
	Type        string            `json:"type"`
	Text        string            `json:"text"`
	Attachments []string          `json:"attachments"`
	Images      []agentImage      `json:"images"`
	SentMS      *float64          `json:"sentMs"`
	DoneMS      *float64          `json:"doneMs"`
	Steered     bool              `json:"steered"`
	ID          any               `json:"id"`
	Title       string            `json:"title"`
	Kind        string            `json:"kind"`
	Status      string            `json:"status"`
	Locations   []string          `json:"locations"`
	Output      *string           `json:"output"`
	Command     *string           `json:"command"`
	Diffs       []agentToolDiff   `json:"diffs"`
	Items       []agentPlanItem   `json:"items"`
	Options     []agentPermission `json:"options"`
	Questions   []agentQuestion   `json:"questions"`
	Chosen      *string           `json:"chosen"`
	Message     string            `json:"message"`
	Incoming    bool              `json:"incoming"`
	Subagent    agentSubagent     `json:"-"`
}

func (e *agentEntry) UnmarshalJSON(data []byte) error {
	type plain agentEntry
	var value plain
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*e = agentEntry(value)
	if e.Type == "subagent" {
		if err := json.Unmarshal(data, &e.Subagent); err != nil {
			return err
		}
	}
	return nil
}

type agentImage struct {
	MimeType string  `json:"mimeType"`
	Data     string  `json:"data"`
	Name     *string `json:"name"`
}

type agentComposerDraft struct {
	Text           string
	Files          []string
	Images         []agentImage
	Caret          int
	HistoryIndex   int
	HistoryStash   string
	Completion     agentCompletionState
	CompletionPos  int
	CompletionSeat bool
}

type agentToolDiff struct {
	Path    string `json:"path"`
	Added   uint32 `json:"added"`
	Removed uint32 `json:"removed"`
}

type agentSubagent struct {
	ID          string   `json:"id"`
	Title       *string  `json:"title"`
	Description *string  `json:"description"`
	Status      string   `json:"status"`
	Error       *string  `json:"error"`
	Prompt      *string  `json:"prompt"`
	Reply       *string  `json:"reply"`
	Tools       uint32   `json:"tools"`
	Activity    *string  `json:"activity"`
	StartedMS   *float64 `json:"startedMs"`
	EndedMS     *float64 `json:"endedMs"`
	Cancellable bool     `json:"cancellable"`
}

func (s agentSubagent) TitleValue() string {
	if s.Title == nil {
		return ""
	}
	return *s.Title
}

func (s agentSubagent) DescriptionValue() string {
	if s.Description == nil {
		return ""
	}
	return *s.Description
}

func (s agentSubagent) PromptValue() string {
	if s.Prompt == nil {
		return ""
	}
	return *s.Prompt
}

func (s agentSubagent) ReplyValue() string {
	if s.Reply == nil {
		return ""
	}
	return *s.Reply
}

func (s agentSubagent) ActivityValue() string {
	if s.Activity == nil {
		return ""
	}
	return *s.Activity
}

func (s agentSubagent) ErrorValue() string {
	if s.Error == nil {
		return ""
	}
	return *s.Error
}

type agentPlanItem struct {
	Content string `json:"content"`
	Status  string `json:"status"`
}

type agentPermission struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type agentQuestion struct {
	AnswerKey   *string               `json:"answerKey"`
	Header      string                `json:"header"`
	Question    string                `json:"question"`
	MultiSelect bool                  `json:"multiSelect"`
	Options     []agentQuestionOption `json:"options"`
}

type agentQuestionOption struct {
	Label       string  `json:"label"`
	Description string  `json:"description"`
	Preview     *string `json:"preview"`
}

type agentChange struct {
	Path    string  `json:"path"`
	RelPath string  `json:"relPath"`
	Kind    string  `json:"kind"`
	Added   uint32  `json:"added"`
	Removed uint32  `json:"removed"`
	OldPath *string `json:"oldPath"`
}

type agentChanges struct {
	Uncommitted []agentChange `json:"uncommitted"`
	Commits     []agentCommit `json:"commits"`
}

type agentCommit struct {
	ID      string        `json:"id"`
	ShortID string        `json:"shortId"`
	Summary string        `json:"summary"`
	Message string        `json:"message"`
	TimeMS  *float64      `json:"timeMs"`
	Parent  *string       `json:"parent"`
	Files   []agentChange `json:"files"`
}

type agentExternalSession struct {
	Provider  string   `json:"provider"`
	SessionID string   `json:"sessionId"`
	Title     string   `json:"title"`
	Cwd       *string  `json:"cwd"`
	UpdatedMS *float64 `json:"updatedMs"`
}

// agentFindExpression is the case-insensitive expression behind Find in
// Conversation. An empty query has no expression, as the find bar shows no
// matches until the user types.
func agentFindExpression(query string) *regexp.Regexp {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	re, err := regexp.Compile(`(?i)` + regexp.QuoteMeta(query))
	if err != nil {
		return nil
	}
	return re
}

// agentEntrySearchText includes every part of an entry the timeline can show,
// so search behaves as if the reader could select that text.
func agentEntrySearchText(entry agentEntry) string {
	var parts []string
	for _, text := range []string{
		entry.Text, entry.Title, entry.Status, entry.Message,
	} {
		if text != "" {
			parts = append(parts, text)
		}
	}
	parts = append(parts, entry.Attachments...)
	parts = append(parts, entry.Locations...)
	if entry.Output != nil && *entry.Output != "" {
		parts = append(parts, *entry.Output)
	}
	for _, item := range entry.Items {
		parts = append(parts, item.Content, item.Status)
	}
	for _, option := range entry.Options {
		parts = append(parts, option.Name)
	}
	for _, question := range entry.Questions {
		parts = append(parts, question.Header, question.Question)
		for _, option := range question.Options {
			parts = append(parts, option.Label, option.Description)
		}
	}
	if entry.Type == "subagent" {
		for _, text := range []string{
			entry.Subagent.TitleValue(), entry.Subagent.DescriptionValue(),
			entry.Subagent.PromptValue(), entry.Subagent.ReplyValue(),
			entry.Subagent.ActivityValue(), entry.Subagent.ErrorValue(),
		} {
			if text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func agentFindMatchCount(re *regexp.Regexp, entries []agentEntry) int {
	if re == nil {
		return 0
	}
	count := 0
	for _, entry := range entries {
		count += len(re.FindAllStringIndex(agentEntrySearchText(entry), -1))
	}
	return count
}

type agentTaskGroup struct {
	Status string
	Label  string
	Tasks  []agentTask
	ID     string
	Custom bool
	Pinned bool
}

func agentEntriesKey(task, subagent string) string {
	if subagent == "" {
		return task
	}
	return task + "\x00" + subagent
}

func (a *AgentsPane) currentSubagent() string {
	if len(a.selectedSubagents) == 0 {
		return ""
	}
	return a.selectedSubagents[len(a.selectedSubagents)-1]
}

func (a *AgentsPane) currentEntriesKey() string {
	return agentEntriesKey(a.selected, a.currentSubagent())
}

func (a *AgentsPane) subagentPathCards(taskID string) []agentSubagent {
	cards := make([]agentSubagent, 0, len(a.selectedSubagents))
	parent := ""
	for _, id := range a.selectedSubagents {
		entries := a.entries[agentEntriesKey(taskID, parent)]
		found := false
		for _, entry := range entries {
			if entry.Type == "subagent" && entry.Subagent.ID == id {
				cards = append(cards, entry.Subagent)
				found = true
				break
			}
		}
		if !found {
			cards = append(cards, agentSubagent{ID: id})
		}
		parent = id
	}
	return cards
}

func (a *AgentsPane) currentSubagentCard(taskID string) *agentSubagent {
	if len(a.selectedSubagents) == 0 {
		return nil
	}
	cards := a.subagentPathCards(taskID)
	if len(cards) == 0 {
		return nil
	}
	return &cards[len(cards)-1]
}

func agentSubagentTitle(subagent agentSubagent) string {
	if title := subagent.TitleValue(); title != "" {
		return title
	}
	if description := subagent.DescriptionValue(); description != "" {
		return description
	}
	return "Subagent"
}

func agentSubagentStatusLabel(status string) string {
	switch status {
	case "running":
		return "Running"
	case "needsInput":
		return "Needs input"
	case "done":
		return "Done"
	case "cancelled":
		return "Stopped"
	case "failed":
		return "Failed"
	default:
		return status
	}
}

func (m *Model) resetAgents() {
	m.agents.generation++
	m.agents.visible = false
	m.agents.channel = 0
	m.agents.connectedRoot = ""
	m.agents.loading = false
	m.agents.loaded = false
	m.agents.err = ""
	m.agents.actionErr = ""
	m.agents.tasks = nil
	m.agents.unread = nil
	m.agents.limits = nil
	m.agents.timelineOpen = nil
	m.agents.providers = nil
	m.agents.providersLoading = false
	m.agents.providersErr = ""
	m.agents.selected = ""
	m.agents.selectedSubagents = nil
	m.agents.selectedTasks = nil
	m.agents.selectionAnchor = ""
	m.agents.taskDragID = ""
	m.agents.taskDragTasks = nil
	m.agents.taskDragStart = ui.Point{}
	m.agents.taskDragAt = ui.Point{}
	m.agents.taskDropTarget = ""
	m.agents.conversation = false
	m.agents.findOpen = false
	m.agents.findText = ""
	m.agents.findMatch = 0
	m.agents.entries = nil
	m.agents.entriesLoaded = nil
	m.agents.entriesBusy = nil
	m.agents.entriesErr = nil
	m.agents.composer = ""
	m.agents.composerFiles = nil
	m.agents.composerImages = nil
	m.agents.composerDrafts = nil
	m.agents.composerBusy = false
	m.agents.composerCaret = 0
	m.agents.commandsBusy = nil
	m.agents.commandsLoaded = nil
	m.agents.commandsErr = nil
	m.agents.commandsSeq = nil
	m.agents.completion = agentCompletionState{}
	m.agents.completionCaret = 0
	m.agents.completionCaretSet = false
	m.agents.historyIndex = -1
	m.agents.historyStash = ""
	m.agents.taskBusy = nil
	m.agents.changesTask = ""
	m.agents.changes = nil
	m.agents.changesBusy = false
	m.agents.changesErr = ""
	m.agents.changesByTask = nil
	m.agents.groupedChanges = nil
	m.agents.changesLoaded = nil
	m.agents.changesLoading = nil
	m.agents.changesErrors = nil
	m.agents.changesSequence = nil
	m.agents.changesFold = nil
	for _, timer := range m.agents.changesTimers {
		timer.Stop()
	}
	m.agents.changesTimers = nil
	m.agents.externalOpen = false
	m.agents.externalLoading = false
	m.agents.externalLoaded = false
	m.agents.externalBusy = ""
	m.agents.external = nil
	m.agents.externalErr = ""
	m.agents.newOpen = false
	m.agents.newFiles = nil
	m.agents.newImages = nil
	m.agents.newConfig = nil
	m.agents.newCommands = nil
	m.agents.newCommandsLoading = false
	m.agents.newCommandsProvider = ""
	m.agents.newCommandsErr = ""
	m.agents.newCommandsSeq = 0
	m.agents.newFileBusy = false
	m.agents.renameOpen = false
	m.agents.groupOpen = false
	m.agents.groupMode = ""
	m.agents.groupID = ""
	m.agents.groupName = ""
	m.agents.groupTasks = nil
	m.agents.groupErr = ""
	m.agents.removeGroupOpen = false
	m.agents.removeGroupID = ""
	m.agents.deleteOpen = false
	m.agents.deleteTasks = nil
	m.agents.changesOpen = false
	m.agents.editOpen = false
	m.agents.editTask = ""
	m.agents.editIndex = -1
	m.agents.editText = ""
	m.agents.editImages = nil
	m.agents.editBusy = false
	m.agents.editErr = ""
	m.agents.mcp = agentMCPSettingsState{}
	if m.agents.registry.PollTimer != nil {
		m.agents.registry.PollTimer.Stop()
	}
	m.agents.registry = agentRegistryState{}
	m.agents.imageCache = nil
	m.agents.imageOrder = nil
	m.agents.markdownImages = nil
	m.agents.markdownImageOrder = nil
	m.agents.markdownAsked = nil
	m.agents.markdownDiagrams = nil
	m.agents.markdownDiagramOrder = nil
	if m.agents.revealTimer != nil {
		m.agents.revealTimer.Stop()
	}
	if m.agents.meterTimer != nil {
		m.agents.meterTimer.Stop()
	}
	m.agents.revealTexts = nil
	m.agents.revealTimer = nil
	m.agents.meterTimer = nil
}

// OpenAgentsWindow is the OpenIDEA agents.showToolWindow action.
func (m *Model) OpenAgentsWindow() {
	m.agents.visible = true
	m.DockShow("agents")
	m.connectAgents(false)
}

// OpenAgentTask opens the Agents window and follows one task. It is the
// target of the Agents status item and of external task links.
func (m *Model) OpenAgentTask(id string) {
	m.openAgentTask(id)
}

// NewAgentTaskOpen opens the New Agent Task sheet after ensuring providers
// have been read from the backend.
func (m *Model) NewAgentTaskOpen() {
	m.connectAgents(false)
	m.agents.newOpen = true
	m.agents.newErr = ""
	if m.agents.newIsolation == "" {
		m.agents.newIsolation = "local"
	}
	m.chooseDefaultAgentProvider()
	m.prepareNewAgentConfig()
	m.refreshAgentProviders(false)
	if m.agentSessionLocation() == "editor" {
		m.agents.newOpen = false
		m.openAgentHomeInEditor()
		return
	}
	m.agents.visible = true
	m.DockShow("agents")
	m.invalidate()
}

func (m *Model) connectAgents(force bool) {
	root := m.Root()
	if root == "" || m.ctx == nil || (m.mgr == nil && m.invokeHook == nil) {
		return
	}
	m.loadAgentSettings()
	a := &m.agents
	if !force && a.connectedRoot == root && (a.channel != 0 || a.loading) {
		return
	}
	a.generation++
	gen := a.generation
	a.connectedRoot = root
	a.channel = m.nextChannel()
	channel := a.channel
	if a.tasks == nil {
		a.tasks = make(map[string]agentTask)
	}
	if a.unread == nil {
		a.unread = make(map[string]bool)
	}
	if a.entries == nil {
		a.entries = make(map[string][]agentEntry)
	}
	if a.entriesLoaded == nil {
		a.entriesLoaded = make(map[string]bool)
	}
	if a.entriesBusy == nil {
		a.entriesBusy = make(map[string]bool)
	}
	if a.entriesErr == nil {
		a.entriesErr = make(map[string]string)
	}
	if a.taskBusy == nil {
		a.taskBusy = make(map[string]bool)
	}
	a.loading = true
	a.loaded = false
	a.err = ""
	a.actionErr = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "agent_subscribe", map[string]any{
			"onEvent": map[string]any{"$channel": channel},
		})
		var tasks []agentTask
		if err == nil {
			err = json.Unmarshal(raw, &tasks)
		}
		m.post(func() {
			if a.generation != gen || m.Root() != root || a.channel != channel {
				return
			}
			a.loading = false
			if err != nil {
				a.channel = 0
				a.err = "Could not connect to Agents: " + err.Error()
				m.invalidate()
				return
			}
			if a.tasks == nil {
				a.tasks = make(map[string]agentTask)
			}
			for _, task := range tasks {
				a.tasks[task.ID] = task
				if task.Status == "needsInput" {
					a.unread[task.ID] = true
				}
			}
			a.loaded = true
			m.syncAgentTaskTabs()
			m.pruneAgentOrganize()
			m.savePreferences()
			m.invalidate()
		})
	}()
	m.refreshAgentProviders(false)
}

func (m *Model) refreshAgentProviders(force bool) {
	root := m.Root()
	if root == "" || m.ctx == nil || (m.mgr == nil && m.invokeHook == nil) {
		return
	}
	a := &m.agents
	if a.providersLoading && !force {
		return
	}
	gen := a.generation
	a.providersLoading = true
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "agent_providers", nil)
		var providers []agentProvider
		if err == nil {
			err = json.Unmarshal(raw, &providers)
		}
		m.post(func() {
			if a.generation != gen || m.Root() != root {
				return
			}
			a.providersLoading = false
			if err != nil {
				a.providersErr = err.Error()
			} else {
				a.providers = providers
				a.providersErr = ""
				m.chooseDefaultAgentProvider()
				m.prepareNewAgentConfig()
			}
			m.invalidate()
		})
	}()
}

func (m *Model) chooseDefaultAgentProvider() {
	a := &m.agents
	before := a.newProvider
	if a.newProvider != "" {
		for _, provider := range a.providers {
			if provider.ID == a.newProvider {
				return
			}
		}
	}
	for _, provider := range a.providers {
		if provider.Installed {
			a.newProvider = provider.ID
			return
		}
	}
	if len(a.providers) > 0 {
		a.newProvider = a.providers[0].ID
	}
	if a.newProvider != before {
		a.newConfig = nil
	}
}

// OpenAgentExternalSessions lists Claude Code, Codex and other provider
// sessions that exist on this machine but are not yet represented by a task.
func (m *Model) OpenAgentExternalSessions() {
	m.OpenAgentsWindow()
	m.agents.externalOpen = true
	m.agents.externalErr = ""
	m.loadAgentExternalSessions()
	m.invalidate()
}

func (m *Model) loadAgentExternalSessions() {
	if m.ctx == nil || (m.mgr == nil && m.invokeHook == nil) {
		return
	}
	if m.agents.externalLoading {
		return
	}
	root, gen := m.Root(), m.agents.generation
	m.agents.externalRequest++
	request := m.agents.externalRequest
	m.agents.externalLoading = true
	m.agents.externalErr = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 45*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "agent_external_sessions", nil)
		var sessions []agentExternalSession
		if err == nil {
			err = json.Unmarshal(raw, &sessions)
		}
		m.post(func() {
			if m.agents.externalRequest != request {
				return
			}
			m.agents.externalLoading = false
			if m.agents.generation != gen || m.Root() != root || !m.agents.externalOpen {
				m.invalidate()
				return
			}
			if err != nil {
				m.agents.externalErr = err.Error()
			} else {
				m.agents.external = sessions
				m.agents.externalLoaded = true
			}
			m.invalidate()
		})
	}()
}

func (m *Model) importAgentExternalSession(session agentExternalSession) {
	key := session.Provider + "\x00" + session.SessionID
	if session.Provider == "" || session.SessionID == "" || m.agents.externalBusy != "" {
		return
	}
	root, gen := m.Root(), m.agents.generation
	m.agents.externalBusy = key
	m.agents.externalErr = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 120*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "agent_import_session", map[string]any{
			"provider": session.Provider, "sessionId": session.SessionID,
		})
		var task agentTask
		if err == nil {
			err = json.Unmarshal(raw, &task)
		}
		m.post(func() {
			if m.agents.generation != gen || m.Root() != root {
				return
			}
			m.agents.externalBusy = ""
			if err != nil {
				m.agents.externalErr = err.Error()
				m.invalidate()
				return
			}
			if task.ID == "" {
				m.agents.externalErr = "The backend returned no Agent task"
				m.invalidate()
				return
			}
			if m.agents.tasks == nil {
				m.agents.tasks = make(map[string]agentTask)
			}
			if _, exists := m.agents.tasks[task.ID]; !exists {
				m.agents.tasks[task.ID] = task
			}
			filtered := m.agents.external[:0]
			for _, item := range m.agents.external {
				if item.Provider != session.Provider || item.SessionID != session.SessionID {
					filtered = append(filtered, item)
				}
			}
			m.agents.external = filtered
			m.agents.externalOpen = false
			m.openAgentTask(task.ID)
		})
	}()
}

func (m *Model) acceptAgentChunk(channel uint32, data json.RawMessage) {
	if channel == 0 || channel != m.agents.channel {
		return
	}
	var head struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(data, &head) != nil {
		return
	}
	a := &m.agents
	if a.tasks == nil {
		a.tasks = make(map[string]agentTask)
	}
	if a.unread == nil {
		a.unread = make(map[string]bool)
	}
	switch head.Type {
	case "task":
		var event struct {
			Task agentTask `json:"task"`
		}
		if json.Unmarshal(data, &event) != nil || event.Task.ID == "" {
			return
		}
		if a.tasks == nil {
			a.tasks = make(map[string]agentTask)
		}
		previous, existed := a.tasks[event.Task.ID]
		a.tasks[event.Task.ID] = event.Task
		m.updateAgentTaskTab(event.Task)
		if event.Task.Status == "archived" {
			delete(a.unread, event.Task.ID)
		} else if event.Task.ID != a.selected &&
			(!existed || previous.Status != event.Task.Status) &&
			agentNeedsAttention(event.Task.Status) {
			a.unread[event.Task.ID] = true
		}
	case "removed":
		var event struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(data, &event) != nil || event.ID == "" {
			return
		}
		m.removeAgentTaskLocal(event.ID)
	case "truncated":
		var event struct {
			Task   string `json:"task"`
			Length uint32 `json:"length"`
		}
		if json.Unmarshal(data, &event) != nil {
			return
		}
		entries := a.entries[agentEntriesKey(event.Task, "")]
		if int(event.Length) < len(entries) {
			a.entries[agentEntriesKey(event.Task, "")] = append([]agentEntry(nil), entries[:event.Length]...)
		}
	case "entry":
		var event struct {
			Task     string     `json:"task"`
			Subagent *string    `json:"subagent"`
			Index    uint32     `json:"index"`
			Entry    agentEntry `json:"entry"`
		}
		if json.Unmarshal(data, &event) != nil {
			return
		}
		subagent := ""
		if event.Subagent != nil {
			subagent = *event.Subagent
		}
		if event.Entry.Type == "tool" && event.Entry.Status == "completed" &&
			len(event.Entry.Diffs) > 0 {
			m.scheduleAgentChanges(event.Task)
		}
		key := agentEntriesKey(event.Task, subagent)
		entries, loaded := a.entries[key]
		if !loaded {
			if key == a.currentEntriesKey() {
				m.loadAgentConversation(event.Task, subagent, true)
			}
			break
		}
		if int(event.Index) > len(entries) {
			m.loadAgentConversation(event.Task, subagent, true)
			break
		}
		next := append([]agentEntry(nil), entries...)
		if int(event.Index) == len(next) {
			next = append(next, event.Entry)
		} else {
			next[event.Index] = event.Entry
		}
		a.entries[key] = next
		if (event.Task != a.selected || subagent != "") && event.Entry.Type == "permission" {
			a.unread[event.Task] = true
		}
	case "append":
		var event struct {
			Task     string  `json:"task"`
			Subagent *string `json:"subagent"`
			Index    uint32  `json:"index"`
			From     uint32  `json:"from"`
			Text     string  `json:"text"`
		}
		if json.Unmarshal(data, &event) != nil {
			return
		}
		subagent := ""
		if event.Subagent != nil {
			subagent = *event.Subagent
		}
		key := agentEntriesKey(event.Task, subagent)
		entries, loaded := a.entries[key]
		if !loaded || int(event.Index) >= len(entries) {
			if key == a.currentEntriesKey() {
				m.loadAgentConversation(event.Task, subagent, true)
			}
			return
		}
		entry := entries[event.Index]
		if entry.Type != "agent" && entry.Type != "thought" {
			m.loadAgentConversation(event.Task, subagent, true)
			return
		}
		entry.Text = appendAgentDelta(entry.Text, int(event.From), event.Text)
		entries[event.Index] = entry
		a.entries[key] = entries
	case "providers":
		m.refreshAgentProviders(true)
	case "limits":
		var event struct {
			Provider string       `json:"provider"`
			Limits   []agentLimit `json:"limits"`
		}
		if json.Unmarshal(data, &event) != nil || event.Provider == "" {
			return
		}
		if a.limits == nil {
			a.limits = make(map[string][]agentLimit)
		}
		a.limits[event.Provider] = append([]agentLimit(nil), event.Limits...)
		m.refreshAgentProviders(true)
	default:
		return
	}
	m.invalidate()
}

func agentNeedsAttention(status string) bool {
	switch status {
	case "needsInput", "failed", "limited", "ready":
		return true
	default:
		return false
	}
}

func appendAgentDelta(current string, from int, delta string) string {
	if delta == "" {
		return current
	}
	have := len(utf16.Encode([]rune(current)))
	if have < from {
		return current
	}
	overlap := have - from
	if overlap > 0 {
		delta = trimUTF16Prefix(delta, overlap)
	}
	return current + delta
}

func trimUTF16Prefix(text string, units int) string {
	if units <= 0 {
		return text
	}
	count := 0
	for i, r := range text {
		width := 1
		if r > 0xFFFF {
			width = 2
		}
		if count+width > units {
			return text[i:]
		}
		count += width
		if count == units {
			return text[i+len(string(r)):]
		}
	}
	return ""
}

func (m *Model) taskGroups() []agentTaskGroup {
	statusGroups := []agentTaskGroup{
		{Status: "needsInput", Label: "Needs Input"},
		{Status: "failed", Label: "Failed"},
		{Status: "limited", Label: "Usage Limit"},
		{Status: "running", Label: "Running"},
		{Status: "ready", Label: "Ready to Review"},
		{Status: "archived", Label: "Archived"},
	}
	needle := strings.ToLower(strings.TrimSpace(m.agents.filter))
	organize := m.agentOrganize()
	pinned := make(map[string]bool, len(organize.Pinned))
	for _, id := range organize.Pinned {
		pinned[id] = true
	}
	knownGroups := make(map[string]bool, len(organize.Groups))
	for _, group := range organize.Groups {
		knownGroups[group.ID] = true
	}
	filtered := make(map[string]agentTask, len(m.agents.tasks))
	for _, task := range m.agents.tasks {
		if needle != "" {
			haystack := strings.ToLower(strings.Join([]string{
				task.Title, task.Provider, task.Status, task.Cwd, agentBranchText(task.Branch),
			}, "\n"))
			if !strings.Contains(haystack, needle) {
				continue
			}
		}
		filtered[task.ID] = task
	}

	var out []agentTaskGroup
	var pinnedTasks []agentTask
	for _, id := range organize.Pinned {
		if task, ok := filtered[id]; ok {
			pinnedTasks = append(pinnedTasks, task)
		}
	}
	sort.SliceStable(pinnedTasks, func(i, j int) bool {
		if pinnedTasks[i].UpdatedMS != pinnedTasks[j].UpdatedMS {
			return pinnedTasks[i].UpdatedMS > pinnedTasks[j].UpdatedMS
		}
		return strings.ToLower(pinnedTasks[i].Title) < strings.ToLower(pinnedTasks[j].Title)
	})
	if len(pinnedTasks) > 0 {
		out = append(out, agentTaskGroup{
			ID: "pinned", Label: "Pinned", Tasks: pinnedTasks, Pinned: true,
		})
	}
	for _, group := range organize.Groups {
		var tasks []agentTask
		for _, task := range filtered {
			if !pinned[task.ID] && organize.GroupOf[task.ID] == group.ID {
				tasks = append(tasks, task)
			}
		}
		sortAgentTasksByAttention(tasks)
		if needle == "" || len(tasks) > 0 {
			out = append(out, agentTaskGroup{
				ID: group.ID, Label: group.Name, Tasks: tasks, Custom: true,
			})
		}
	}
	for _, task := range filtered {
		if pinned[task.ID] {
			continue
		}
		if group, ok := organize.GroupOf[task.ID]; ok && knownGroups[group] {
			continue
		}
		group := agentTaskGroupFor(task.Status)
		for i := range statusGroups {
			if statusGroups[i].Status == group {
				statusGroups[i].Tasks = append(statusGroups[i].Tasks, task)
				break
			}
		}
	}
	for i := range statusGroups {
		sort.SliceStable(statusGroups[i].Tasks, func(a, b int) bool {
			left, right := statusGroups[i].Tasks[a], statusGroups[i].Tasks[b]
			if left.UpdatedMS != right.UpdatedMS {
				return left.UpdatedMS > right.UpdatedMS
			}
			return strings.ToLower(left.Title) < strings.ToLower(right.Title)
		})
		if len(statusGroups[i].Tasks) > 0 {
			out = append(out, statusGroups[i])
		}
	}
	return out
}

func agentTaskGroupFor(status string) string {
	switch status {
	case "needsInput":
		return "needsInput"
	case "failed":
		return "failed"
	case "limited":
		return "limited"
	case "running", "starting":
		return "running"
	case "archived":
		return "archived"
	default:
		return "ready"
	}
}

func agentBranchText(branch *string) string {
	if branch == nil {
		return ""
	}
	return strings.TrimPrefix(*branch, "agent/")
}

func agentStatusLabel(status string) string {
	switch status {
	case "starting":
		return "Starting"
	case "running":
		return "Running"
	case "needsInput":
		return "Needs input"
	case "ready":
		return "Ready"
	case "failed":
		return "Failed"
	case "limited":
		return "Usage limit"
	case "archived":
		return "Archived"
	default:
		return status
	}
}

func (m *Model) openAgentConversation(id string) {
	if _, ok := m.agents.tasks[id]; !ok {
		return
	}
	m.switchAgentComposer(id)
	m.agents.selectedSubagents = nil
	m.agents.conversation = true
	delete(m.agents.unread, id)
	m.agents.actionErr = ""
	m.loadAgentEntries(id, false)
	m.refreshAgentChanges(id, false)
	m.invalidate()
}

func (m *Model) openAgentSubagent(taskID, subagent string) {
	a := &m.agents
	if taskID == "" || subagent == "" || taskID != a.selected {
		return
	}
	a.selectedSubagents = append(a.selectedSubagents, subagent)
	a.findOpen = false
	a.findText = ""
	a.findMatch = 0
	m.loadAgentConversation(taskID, subagent, false)
	m.invalidate()
}

func (m *Model) navigateAgentSubagents(taskID string, path []string) {
	a := &m.agents
	if taskID == "" || taskID != a.selected {
		return
	}
	a.selectedSubagents = append([]string(nil), path...)
	a.findOpen = false
	a.findText = ""
	a.findMatch = 0
	m.loadAgentConversation(taskID, a.currentSubagent(), false)
	m.invalidate()
}

func (m *Model) closeAgentConversation() {
	m.stashAgentComposer()
	m.agents.conversation = false
	m.agents.selected = ""
	m.agents.selectedSubagents = nil
	m.agents.composer = ""
	m.agents.composerFiles = nil
	m.agents.composerImages = nil
	m.agents.findOpen = false
	m.agents.findText = ""
	m.agents.findMatch = 0
	m.agents.actionErr = ""
	m.invalidate()
}

func (m *Model) quoteAgentText(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	lines := strings.Split(text, "\n")
	for index, line := range lines {
		if line == "" {
			lines[index] = ">"
		} else {
			lines[index] = "> " + line
		}
	}
	quote := strings.Join(lines, "\n") + "\n\n"
	if strings.TrimSpace(m.agents.composer) == "" {
		m.agents.composer = quote
	} else {
		m.agents.composer = strings.TrimRight(m.agents.composer, "\n") + "\n\n" + quote
	}
	m.invalidate()
}

// FindAgentConversation opens the conversation's search field.
func (m *Model) FindAgentConversation() {
	if m.activeAgentTaskTab() == nil &&
		(m.agentSessionLocation() != "toolWindow" || !m.agents.conversation) {
		return
	}
	if m.agents.selected == "" {
		return
	}
	m.agents.findOpen = true
	m.agents.findMatch = 0
	m.invalidate()
}

// CloseAgentTask closes the conversation surface while the task keeps running.
func (m *Model) CloseAgentTask() {
	if t := m.activeAgentTaskTab(); t != nil {
		m.removeTabNow(t)
		return
	}
	if m.agentSessionLocation() == "toolWindow" && m.agents.conversation {
		m.closeAgentConversation()
	}
}

// CloseAndNewAgentTask is OpenIDEA's close-then-new action.
func (m *Model) CloseAndNewAgentTask() {
	m.CloseAgentTask()
	m.NewAgentTaskOpen()
}

// SendEditorToAgent adds the active file to New Agent Task, which is
// OpenIDEA's agents.sendToAgent editor action.
func (m *Model) SendEditorToAgent() {
	t := m.tab()
	if t == nil || t.doc == nil {
		m.status = "Open a file to send it to an Agent"
		m.invalidate()
		return
	}
	m.SendPathsToAgent([]string{t.doc.Path()})
}

// SendPathsToAgent adds project files or folders to New Agent Task. The
// prompt stays as typed, as OpenIDEA keeps references separate from text.
func (m *Model) SendPathsToAgent(paths []string) {
	root := m.Root()
	text, files, added := agentAddPathReferences(
		m.agents.newPrompt, m.agents.newFiles, paths, root, false)
	if added == 0 {
		m.status = "The selection is not inside this project"
		m.invalidate()
		return
	}
	m.NewAgentTaskOpen()
	m.agents.newPrompt = text
	m.agents.newFiles = files
	m.status = fmt.Sprintf("Added %s to the Agent task",
		count(added, "file", "files"))
	m.invalidate()
}

// AddAgentTaskAttachments picks files or a folder for the new task. The
// native picker runs off the UI thread so the window keeps drawing.
func (m *Model) AddAgentTaskAttachments(directory bool) {
	a := &m.agents
	if a.newFileBusy || !m.agentNewTaskVisible() {
		return
	}
	root, gen := m.Root(), a.generation
	a.newFileBusy = true
	a.newErr = ""
	m.invalidate()
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:      m.win,
			Title:       "Add Files to Agent Task",
			DefaultPath: root,
			Directory:   directory,
			Multiple:    !directory,
		})
		m.post(func() {
			if a.generation != gen || m.Root() != root || !m.agentNewTaskVisible() {
				return
			}
			a.newFileBusy = false
			if err != nil {
				a.newErr = err.Error()
				m.invalidate()
				return
			}
			text, files, added := agentAddPathReferences(
				a.newPrompt, a.newFiles, paths, root, true)
			a.newPrompt = text
			a.newFiles = files
			if added == 0 && len(paths) > 0 {
				a.newErr = "Those files are already referenced"
			}
			m.invalidate()
		})
	}()
}

// AddAgentConversationAttachments picks files or a folder for the current
// conversation draft. It runs the native picker off the UI thread.
func (m *Model) AddAgentConversationAttachments(directory bool) {
	a := &m.agents
	if a.composerBusy || !a.conversation || a.selected == "" {
		return
	}
	root, gen := m.Root(), a.generation
	a.composerBusy = true
	a.actionErr = ""
	m.invalidate()
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:      m.win,
			Title:       "Add Files to Agent Message",
			DefaultPath: root,
			Directory:   directory,
			Multiple:    !directory,
		})
		m.post(func() {
			if a.generation != gen || m.Root() != root || !a.conversation {
				return
			}
			a.composerBusy = false
			if err != nil {
				a.actionErr = err.Error()
				m.invalidate()
				return
			}
			text, files, _ := agentAddPathReferences(
				a.composer, a.composerFiles, paths, root, true)
			a.composer = text
			a.composerFiles = files
			m.invalidate()
		})
	}()
}

const maxAgentImageBytes = 5 * 1024 * 1024

var agentImageExtensions = map[string]string{
	".gif":  "image/gif",
	".jpeg": "image/jpeg",
	".jpg":  "image/jpeg",
	".png":  "image/png",
	".webp": "image/webp",
}

func agentImagePresent(images []agentImage, want agentImage) bool {
	for _, image := range images {
		if image.MimeType == want.MimeType && image.Data == want.Data {
			return true
		}
	}
	return false
}

func agentImageFromAny(value any) (agentImage, bool) {
	switch image := value.(type) {
	case agentImage:
		return image, image.Data != "" && image.MimeType != ""
	case map[string]any:
		mimeType, _ := image["mimeType"].(string)
		data, _ := image["data"].(string)
		var name *string
		if text, ok := image["name"].(string); ok && text != "" {
			name = &text
		}
		return agentImage{MimeType: mimeType, Data: data, Name: name},
			mimeType != "" && data != ""
	default:
		return agentImage{}, false
	}
}

func agentImageName(image agentImage, index int) string {
	if image.Name != nil && strings.TrimSpace(*image.Name) != "" {
		return *image.Name
	}
	extension := strings.TrimPrefix(image.MimeType, "image/")
	if extension == "jpeg" {
		extension = "jpg"
	}
	return fmt.Sprintf("Image %d.%s", index+1, extension)
}

func (m *Model) addAgentImage(targetNew bool, image agentImage) bool {
	a := &m.agents
	if image.MimeType == "" || image.Data == "" ||
		len(image.Data)*3/4 > maxAgentImageBytes {
		return false
	}
	if targetNew {
		if agentImagePresent(a.newImages, image) {
			return false
		}
		a.newImages = append(a.newImages, image)
		return true
	}
	if agentImagePresent(a.composerImages, image) {
		return false
	}
	a.composerImages = append(a.composerImages, image)
	return true
}

// AddAgentTaskImageFromClipboard adds the image currently on the system
// clipboard to a New Agent Task, as OpenIDEA does on paste.
func (m *Model) AddAgentTaskImageFromClipboard() {
	m.addAgentImageFromClipboard(true)
}

// AddAgentConversationImageFromClipboard adds a clipboard image to the
// current conversation draft.
func (m *Model) AddAgentConversationImageFromClipboard() {
	m.addAgentImageFromClipboard(false)
}

func (m *Model) addAgentImageFromClipboard(targetNew bool) {
	a := &m.agents
	data := mygo.Clipboard.ReadImage()
	if len(data) == 0 {
		m.setAgentImageError(targetNew, "The clipboard has no image")
		return
	}
	image, err := agentImageFromBytes(data, "Clipboard image")
	if err != nil {
		m.setAgentImageError(targetNew, err.Error())
		return
	}
	if !m.addAgentImage(targetNew, image) {
		m.setAgentImageError(targetNew, "That image is already attached")
		return
	}
	if targetNew {
		a.newErr = ""
	} else {
		a.actionErr = ""
	}
	m.invalidate()
}

func (m *Model) setAgentImageError(targetNew bool, message string) {
	if targetNew {
		m.agents.newErr = message
	} else {
		m.agents.actionErr = message
	}
	m.invalidate()
}

func agentImageFromBytes(data []byte, name string) (agentImage, error) {
	if len(data) == 0 {
		return agentImage{}, fmt.Errorf("the image is empty")
	}
	if len(data) > maxAgentImageBytes {
		return agentImage{}, fmt.Errorf("images must be 5 MB or smaller")
	}
	mimeType := http.DetectContentType(data)
	if !strings.HasPrefix(mimeType, "image/") {
		return agentImage{}, fmt.Errorf("that file is not a supported image")
	}
	if _, ok := agentImageExtensions["."+strings.TrimPrefix(mimeType, "image/")]; !ok &&
		mimeType != "image/jpeg" && mimeType != "image/png" &&
		mimeType != "image/gif" && mimeType != "image/webp" {
		return agentImage{}, fmt.Errorf("that image type is not supported")
	}
	cleanName := filepath.Base(strings.TrimSpace(name))
	if cleanName == "" || cleanName == "." {
		cleanName = "Image"
	}
	return agentImage{
		MimeType: mimeType,
		Data:     base64.StdEncoding.EncodeToString(data),
		Name:     &cleanName,
	}, nil
}

// addAgentDroppedPaths turns image files into image attachments and adds the
// other files as @ references. Files are read off the UI thread.
func (m *Model) addAgentDroppedPaths(targetNew bool, paths []string) {
	if len(paths) == 0 {
		return
	}
	root, gen := m.Root(), m.agents.generation
	go func() {
		images := make([]agentImage, 0, len(paths))
		files := make([]string, 0, len(paths))
		var imageErr error
		for _, path := range paths {
			extension := strings.ToLower(filepath.Ext(path))
			if agentImageExtensions[extension] == "" {
				files = append(files, path)
				continue
			}
			info, err := os.Stat(path)
			if err != nil || !info.Mode().IsRegular() {
				imageErr = fmt.Errorf("could not read %s", filepath.Base(path))
				continue
			}
			if info.Size() > maxAgentImageBytes {
				imageErr = fmt.Errorf("%s is larger than 5 MB", filepath.Base(path))
				continue
			}
			data, err := os.ReadFile(path)
			if err != nil {
				imageErr = err
				continue
			}
			image, err := agentImageFromBytes(data, filepath.Base(path))
			if err != nil {
				imageErr = err
				continue
			}
			images = append(images, image)
		}
		m.post(func() {
			a := &m.agents
			if a.generation != gen || m.Root() != root {
				return
			}
			added := 0
			for _, image := range images {
				if m.addAgentImage(targetNew, image) {
					added++
				}
			}
			if len(files) > 0 {
				if targetNew {
					text, refs, count := agentAddPathReferences(
						a.newPrompt, a.newFiles, files, root, false)
					a.newPrompt, a.newFiles = text, refs
					added += count
				} else if a.conversation {
					text, refs, count := agentAddPathReferences(
						a.composer, a.composerFiles, files, root, false)
					a.composer, a.composerFiles = text, refs
					added += count
				}
			}
			if imageErr != nil {
				m.setAgentImageError(targetNew, imageErr.Error())
			} else if added == 0 {
				m.setAgentImageError(targetNew, "Those files are already attached")
			} else if targetNew {
				a.newErr = ""
			} else {
				a.actionErr = ""
			}
			m.invalidate()
		})
	}()
}

func (m *Model) RemoveAgentConversationImage(index int) {
	images := m.agents.composerImages
	if index < 0 || index >= len(images) {
		return
	}
	m.agents.composerImages = append(append([]agentImage(nil), images[:index]...), images[index+1:]...)
	m.invalidate()
}

func (m *Model) RemoveAgentTaskImage(index int) {
	images := m.agents.newImages
	if index < 0 || index >= len(images) {
		return
	}
	m.agents.newImages = append(append([]agentImage(nil), images[:index]...), images[index+1:]...)
	m.invalidate()
}

func (m *Model) RemoveAgentEditImage(index int) {
	images := m.agents.editImages
	if index < 0 || index >= len(images) {
		return
	}
	m.agents.editImages = append(append([]agentImage(nil), images[:index]...), images[index+1:]...)
	m.invalidate()
}

func (m *Model) addAgentEditDroppedPaths(paths []string) {
	if len(paths) == 0 {
		return
	}
	root, gen := m.Root(), m.agents.generation
	go func() {
		images := make([]agentImage, 0, len(paths))
		var readErr error
		for _, path := range paths {
			if agentImageExtensions[strings.ToLower(filepath.Ext(path))] == "" {
				readErr = fmt.Errorf("only images can be attached while editing a message")
				continue
			}
			data, err := os.ReadFile(path)
			if err != nil {
				readErr = err
				continue
			}
			image, err := agentImageFromBytes(data, filepath.Base(path))
			if err != nil {
				readErr = err
				continue
			}
			images = append(images, image)
		}
		m.post(func() {
			a := &m.agents
			if a.generation != gen || m.Root() != root || !a.editOpen {
				return
			}
			for _, image := range images {
				if !agentImagePresent(a.editImages, image) {
					a.editImages = append(a.editImages, image)
				}
			}
			if readErr != nil {
				a.editErr = readErr.Error()
			}
			m.invalidate()
		})
	}()
}

// RemoveAgentConversationFile detaches one path from the current message.
func (m *Model) RemoveAgentConversationFile(path string) {
	path = filepath.Clean(path)
	files := m.agents.composerFiles[:0]
	for _, file := range m.agents.composerFiles {
		if filepath.Clean(file) != path {
			files = append(files, file)
		}
	}
	m.agents.composerFiles = files
	m.invalidate()
}

// RemoveAgentTaskFile detaches one reference from the new task draft.
func (m *Model) RemoveAgentTaskFile(path string) {
	path = filepath.Clean(path)
	files := m.agents.newFiles[:0]
	for _, file := range m.agents.newFiles {
		if filepath.Clean(file) != path {
			files = append(files, file)
		}
	}
	m.agents.newFiles = files
	m.invalidate()
}

// AgentTaskFilePath is the compact path shown for an attachment.
func (m *Model) AgentTaskFilePath(path string) string {
	if rel := projectRelative(m.Root(), path); rel != path {
		return rel
	}
	return filepath.Base(path)
}

func (m *Model) loadAgentEntries(task string, force bool) {
	m.loadAgentConversation(task, "", force)
}

func (m *Model) loadAgentConversation(task, subagent string, force bool) {
	if task == "" || m.ctx == nil || (m.mgr == nil && m.invokeHook == nil) {
		return
	}
	a := &m.agents
	key := agentEntriesKey(task, subagent)
	if a.entriesBusy == nil {
		a.entriesBusy = make(map[string]bool)
	}
	if a.entriesErr == nil {
		a.entriesErr = make(map[string]string)
	}
	if a.entriesLoaded == nil {
		a.entriesLoaded = make(map[string]bool)
	}
	if a.entriesBusy[key] || (a.entriesLoaded[key] && !force) {
		return
	}
	root, gen := m.Root(), a.generation
	a.entriesBusy[key] = true
	delete(a.entriesErr, key)
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "agent_entries", map[string]any{
			"task":     task,
			"subagent": nullableAgentSubagent(subagent),
		})
		var entries []agentEntry
		if err == nil {
			err = json.Unmarshal(raw, &entries)
		}
		m.post(func() {
			if a.generation != gen || m.Root() != root {
				return
			}
			a.entriesBusy[key] = false
			if err != nil {
				a.entriesErr[key] = err.Error()
			} else {
				if a.entries == nil {
					a.entries = make(map[string][]agentEntry)
				}
				a.entries[key] = entries
				a.entriesLoaded[key] = true
				delete(a.entriesErr, key)
			}
			m.invalidate()
		})
	}()
}

func nullableAgentSubagent(subagent string) any {
	if subagent == "" {
		return nil
	}
	return subagent
}

func (m *Model) createAgentTask() {
	a := &m.agents
	if a.newBusy || m.Root() == "" {
		return
	}
	prompt := strings.TrimSpace(a.newPrompt)
	root := m.Root()
	files := agentPromptFiles(prompt, a.newFiles, root)
	if prompt == "" && len(files) == 0 && len(a.newImages) == 0 {
		a.newErr = "Describe what the agent should do or add files or images"
		m.invalidate()
		return
	}
	provider := m.selectedAgentProvider()
	if provider == nil {
		a.newErr = "Choose an agent provider"
		m.invalidate()
		return
	}
	if !provider.Installed {
		a.newErr = provider.Name + " is not installed"
		m.invalidate()
		return
	}
	isolation := a.newIsolation
	if isolation != "local" {
		isolation = "worktree"
	}
	config := m.newAgentConfigChoices()
	gen := a.generation
	a.newBusy = true
	a.newErr = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 90*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "agent_create", map[string]any{
			"request": map[string]any{
				"prompt":    map[string]any{"text": prompt, "files": files, "images": a.newImages},
				"provider":  provider.ID,
				"config":    config,
				"isolation": isolation,
				"base":      nil,
			},
		})
		var task agentTask
		if err == nil {
			err = json.Unmarshal(raw, &task)
		}
		m.post(func() {
			if a.generation != gen || m.Root() != root {
				return
			}
			a.newBusy = false
			if err != nil {
				a.newErr = err.Error()
				m.invalidate()
				return
			}
			if a.tasks == nil {
				a.tasks = make(map[string]agentTask)
			}
			if _, exists := a.tasks[task.ID]; !exists {
				a.tasks[task.ID] = task
			}
			a.newOpen = false
			a.newPrompt = ""
			a.newFiles = nil
			a.newImages = nil
			m.removeAgentHomeTab()
			m.openAgentTask(task.ID)
		})
	}()
}

func (m *Model) prepareNewAgentConfig() {
	a := &m.agents
	provider := m.selectedAgentProvider()
	if provider == nil {
		a.newConfig = nil
		return
	}
	if a.newConfig == nil {
		a.newConfig = make(map[string]string)
	}
	defaults := m.agentDefaultChoices(provider.ID)
	options := m.newAgentConfigOptions()
	for _, option := range options {
		if _, ok := a.newConfig[option.ID]; ok {
			continue
		}
		if value := defaults[option.ID]; value != "" && agentConfigValueExists(option, value) {
			a.newConfig[option.ID] = value
		} else if option.Current != nil && *option.Current != "" {
			a.newConfig[option.ID] = *option.Current
		} else if len(option.Values) > 0 {
			a.newConfig[option.ID] = option.Values[0].Value
		}
	}
	m.applyNewTaskApprovalDefaults(provider, options)
}

func (m *Model) newAgentConfigOptions() []agentConfigOption {
	provider := m.selectedAgentProvider()
	if provider == nil {
		return nil
	}
	options := append([]agentConfigOption(nil), provider.Config...)
	model := m.agentDefaultChoices(provider.ID)["model"]
	for _, option := range options {
		if model == "" && option.Category != nil && *option.Category == "model" &&
			option.Current != nil {
			model = *option.Current
			break
		}
	}
	if model == "" {
		model = m.agents.newConfig["model"]
	}
	options = append(options, provider.ThoughtLevels[model]...)
	return agentMainConfigOptions(options)
}

func (m *Model) applyNewTaskApprovalDefaults(provider *agentProvider,
	options []agentConfigOption) {
	if m.agentPrefs().newTaskApprovals() != "bypass" {
		return
	}
	for _, option := range options {
		category := ""
		if option.Category != nil {
			category = *option.Category
		}
		switch category {
		case "mode":
			for _, value := range option.Values {
				if agentModeIcon(value.Value) == "unlock" {
					m.agents.newConfig[option.ID] = value.Value
					break
				}
			}
		case "permissions":
			for _, value := range option.Values {
				if value.Value == "on" || value.Value == "true" {
					m.agents.newConfig[option.ID] = value.Value
					break
				}
			}
		case "approval":
			for _, value := range option.Values {
				if value.Value == "bypass" {
					m.agents.newConfig[option.ID] = value.Value
					break
				}
			}
		}
	}
	if provider.ID == "cursor" {
		for _, option := range options {
			if option.ID != "fast" {
				continue
			}
			for _, value := range option.Values {
				if value.Value == "false" {
					m.agents.newConfig[option.ID] = value.Value
					break
				}
			}
		}
	}
}

func (m *Model) newAgentConfigChoices() []agentConfigChoice {
	options := m.newAgentConfigOptions()
	choices := make([]agentConfigChoice, 0, len(options))
	for _, option := range options {
		value := m.agents.newConfig[option.ID]
		if value == "" && option.Current != nil {
			value = *option.Current
		}
		if value == "" && len(option.Values) > 0 {
			value = option.Values[0].Value
		}
		if value != "" {
			choices = append(choices, agentConfigChoice{ID: option.ID, Value: value})
		}
	}
	return choices
}

func agentModeIcon(mode string) string {
	mode = strings.ToLower(mode)
	if at := strings.LastIndexAny(mode, "#/"); at >= 0 {
		mode = mode[at+1:]
	}
	var letters strings.Builder
	for _, r := range mode {
		if r >= 'a' && r <= 'z' {
			letters.WriteRune(r)
		}
	}
	mode = letters.String()
	switch {
	case strings.Contains(mode, "plan"), mode == "architect":
		return "todo"
	case strings.Contains(mode, "bypass"), mode == "yolo",
		strings.Contains(mode, "fullaccess"), mode == "dangerous":
		return "unlock"
	case strings.Contains(mode, "readonly"), mode == "ask", mode == "chat":
		return "eye"
	case strings.Contains(mode, "edit"), mode == "code":
		return "edit"
	case mode == "auto", mode == "agent", strings.Contains(mode, "autopilot"):
		return "run"
	default:
		return "lock"
	}
}

func (m *Model) selectedAgentProvider() *agentProvider {
	for i := range m.agents.providers {
		if m.agents.providers[i].ID == m.agents.newProvider {
			return &m.agents.providers[i]
		}
	}
	return nil
}

func (m *Model) submitAgentPrompt() {
	a := &m.agents
	taskID := a.selected
	task, ok := a.tasks[taskID]
	text := strings.TrimSpace(a.composer)
	root := m.Root()
	promptRoot := root
	if task.Cwd != "" {
		promptRoot = task.Cwd
	}
	files := agentPromptFiles(text, a.composerFiles, promptRoot)
	if !ok || (text == "" && len(files) == 0 && len(a.composerImages) == 0) ||
		a.taskBusy[taskID] || task.Status == "archived" {
		return
	}
	if a.taskBusy == nil {
		a.taskBusy = make(map[string]bool)
	}
	gen := a.generation
	a.taskBusy[taskID] = true
	a.actionErr = ""
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 45*time.Second)
		defer cancel()
		_, err := m.invoke(ctx, "agent_prompt", map[string]any{
			"task":   taskID,
			"prompt": map[string]any{"text": text, "files": files, "images": a.composerImages},
			"now":    false,
		})
		m.post(func() {
			if a.generation != gen || m.Root() != root {
				return
			}
			a.taskBusy[taskID] = false
			if err != nil {
				a.actionErr = "Could not send the message: " + err.Error()
			} else {
				a.composer = ""
				a.composerFiles = nil
				a.composerImages = nil
				delete(a.composerDrafts, taskID)
			}
			m.invalidate()
		})
	}()
	m.invalidate()
}

func (m *Model) cancelAgentTask(taskID string) {
	m.cancelAgent(taskID, "")
}

func (m *Model) cancelAgentSubagent(taskID, subagent string) {
	m.cancelAgent(taskID, subagent)
}

func (m *Model) cancelAgent(taskID, subagent string) {
	if taskID == "" || m.ctx == nil {
		return
	}
	a := &m.agents
	if a.taskBusy == nil {
		a.taskBusy = make(map[string]bool)
	}
	if a.taskBusy[taskID] {
		return
	}
	root, gen := m.Root(), a.generation
	a.taskBusy[taskID] = true
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		_, err := m.invoke(ctx, "agent_cancel", map[string]any{
			"task": taskID, "subagent": nullableAgentSubagent(subagent),
		})
		m.post(func() {
			if a.generation != gen || m.Root() != root {
				return
			}
			a.taskBusy[taskID] = false
			if err != nil {
				if subagent == "" {
					a.actionErr = "Could not stop the task: " + err.Error()
				} else {
					a.actionErr = "Could not stop the subagent: " + err.Error()
				}
			}
			m.invalidate()
		})
	}()
	m.invalidate()
}

func (m *Model) sendAgentQueuedNow(taskID, queuedID string) {
	m.runAgentQueuedAction(taskID, queuedID, "agent_send_queued_now", false)
}

func (m *Model) editAgentQueued(taskID, queuedID string) {
	m.runAgentQueuedAction(taskID, queuedID, "agent_unqueue", true)
}

func (m *Model) removeAgentQueued(taskID, queuedID string) {
	m.runAgentQueuedAction(taskID, queuedID, "agent_unqueue", false)
}

func (m *Model) runAgentQueuedAction(taskID, queuedID, command string, restore bool) {
	if taskID == "" || queuedID == "" || m.ctx == nil {
		return
	}
	a := &m.agents
	if a.taskBusy == nil {
		a.taskBusy = make(map[string]bool)
	}
	if a.taskBusy[taskID] {
		return
	}
	root, gen := m.Root(), a.generation
	a.taskBusy[taskID] = true
	a.actionErr = ""
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, command, map[string]any{
			"task": taskID, "queued": queuedID,
		})
		var queued agentQueued
		if err == nil && command == "agent_unqueue" {
			err = json.Unmarshal(raw, &queued)
		}
		m.post(func() {
			if a.generation != gen || m.Root() != root {
				return
			}
			a.taskBusy[taskID] = false
			if err != nil {
				a.actionErr = "Could not update the queued message: " + err.Error()
				m.invalidate()
				return
			}
			if task, ok := a.tasks[taskID]; ok {
				queue := task.Queue[:0]
				for _, item := range task.Queue {
					if item.ID != queuedID {
						queue = append(queue, item)
					}
				}
				task.Queue = queue
				a.tasks[taskID] = task
			}
			if restore {
				text := strings.TrimSpace(queued.Prompt.Text)
				if text != "" {
					if strings.TrimSpace(a.composer) == "" {
						a.composer = queued.Prompt.Text
					} else {
						a.composer = strings.TrimRight(a.composer, "\n") + "\n\n" + queued.Prompt.Text
					}
				}
				seen := make(map[string]bool, len(a.composerFiles))
				for _, path := range a.composerFiles {
					seen[path] = true
				}
				for _, path := range queued.Prompt.Files {
					if path != "" && !seen[path] {
						a.composerFiles = append(a.composerFiles, path)
						seen[path] = true
					}
				}
				for _, image := range queued.Prompt.Images {
					value, ok := agentImageFromAny(image)
					if ok && !agentImagePresent(a.composerImages, value) {
						a.composerImages = append(a.composerImages, value)
					}
				}
			}
			m.invalidate()
		})
	}()
	m.invalidate()
}

func (m *Model) stopAgentBackground(taskID, backgroundID string) {
	if taskID == "" || backgroundID == "" || m.ctx == nil {
		return
	}
	a := &m.agents
	if a.taskBusy == nil {
		a.taskBusy = make(map[string]bool)
	}
	if a.taskBusy[taskID] {
		return
	}
	root, gen := m.Root(), a.generation
	a.taskBusy[taskID] = true
	a.actionErr = ""
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		_, err := m.invoke(ctx, "agent_stop_background", map[string]any{
			"task": taskID, "background": backgroundID,
		})
		m.post(func() {
			if a.generation != gen || m.Root() != root {
				return
			}
			a.taskBusy[taskID] = false
			if err != nil {
				a.actionErr = "Could not stop the background task: " + err.Error()
			} else if task, ok := a.tasks[taskID]; ok {
				remaining := task.Background[:0]
				for _, item := range task.Background {
					if item.ID != backgroundID {
						remaining = append(remaining, item)
					}
				}
				task.Background = remaining
				a.tasks[taskID] = task
			}
			m.invalidate()
		})
	}()
	m.invalidate()
}

func (m *Model) setAgentConfig(taskID, optionID, value string) {
	if taskID == "" || optionID == "" || m.ctx == nil {
		return
	}
	a := &m.agents
	if a.taskBusy == nil {
		a.taskBusy = make(map[string]bool)
	}
	if a.taskBusy[taskID] {
		return
	}
	root, gen := m.Root(), a.generation
	a.taskBusy[taskID] = true
	a.actionErr = ""
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		_, err := m.invoke(ctx, "agent_set_config", map[string]any{
			"task": taskID, "option": optionID, "value": value,
		})
		m.post(func() {
			if a.generation != gen || m.Root() != root {
				return
			}
			a.taskBusy[taskID] = false
			if err != nil {
				a.actionErr = "Could not change the agent setting: " + err.Error()
				m.invalidate()
				return
			}
			if task, ok := a.tasks[taskID]; ok {
				for i := range task.Config {
					if task.Config[i].ID != optionID {
						continue
					}
					chosen := value
					task.Config[i].Current = &chosen
					if task.Config[i].Category != nil && *task.Config[i].Category == "model" {
						task.Model = &chosen
					}
				}
				a.tasks[taskID] = task
			}
			m.invalidate()
		})
	}()
	m.invalidate()
}

func (m *Model) respondAgentPermission(taskID string, permission uint32, option string) {
	if taskID == "" || m.ctx == nil {
		return
	}
	a := &m.agents
	root, gen := m.Root(), a.generation
	a.actionErr = ""
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		_, err := m.invoke(ctx, "agent_respond", map[string]any{
			"task": taskID, "permission": permission, "option": option,
		})
		m.post(func() {
			if a.generation != gen || m.Root() != root {
				return
			}
			if err != nil {
				a.actionErr = "Could not answer the agent: " + err.Error()
			}
			m.invalidate()
		})
	}()
}

func (m *Model) setAgentArchived(taskID string, archived bool) {
	m.setAgentArchivedMany([]string{taskID}, archived)
}

func (m *Model) openAgentRename(taskID string) {
	task, ok := m.agents.tasks[taskID]
	if !ok {
		return
	}
	m.agents.renameTask = taskID
	m.agents.renameTitle = task.Title
	m.agents.renameOpen = true
	m.invalidate()
}

func (m *Model) submitAgentRename() {
	a := &m.agents
	title := strings.TrimSpace(a.renameTitle)
	if title == "" || a.renameTask == "" {
		return
	}
	taskID := a.renameTask
	root, gen := m.Root(), a.generation
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		_, err := m.invoke(ctx, "agent_rename", map[string]any{"task": taskID, "title": title})
		m.post(func() {
			if a.generation != gen || m.Root() != root {
				return
			}
			if err != nil {
				a.actionErr = "Could not rename the task: " + err.Error()
			} else {
				task := a.tasks[taskID]
				task.Title = title
				a.tasks[taskID] = task
				m.updateAgentTaskTab(task)
				a.renameOpen = false
			}
			m.invalidate()
		})
	}()
}

func (m *Model) openAgentDelete(taskID string) {
	m.openAgentDeleteTasks([]string{taskID})
}

func (m *Model) openAgentDeleteTasks(taskIDs []string) {
	taskIDs = uniqueAgentIDs(taskIDs)
	if len(taskIDs) == 0 {
		return
	}
	hasWorktree := false
	for _, taskID := range taskIDs {
		if task, ok := m.agents.tasks[taskID]; ok && task.Isolation == "worktree" {
			hasWorktree = true
			break
		}
	}
	m.agents.deleteTasks = taskIDs
	m.agents.deleteTask = taskIDs[0]
	m.agents.deleteWorktree = hasWorktree
	m.agents.deleteOpen = true
	m.invalidate()
}

func (m *Model) submitAgentDelete() {
	a := &m.agents
	taskIDs := append([]string(nil), a.deleteTasks...)
	if len(taskIDs) == 0 && a.deleteTask != "" {
		taskIDs = []string{a.deleteTask}
	}
	taskIDs = uniqueAgentIDs(taskIDs)
	if len(taskIDs) == 0 {
		return
	}
	removeWorktree := a.deleteWorktree
	root, gen := m.Root(), a.generation
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
		defer cancel()
		var deleted []string
		var firstErr error
		for _, taskID := range taskIDs {
			_, err := m.invoke(ctx, "agent_delete", map[string]any{
				"task": taskID, "removeWorktree": removeWorktree,
			})
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			deleted = append(deleted, taskID)
		}
		m.post(func() {
			if a.generation != gen || m.Root() != root {
				return
			}
			for _, taskID := range deleted {
				m.removeAgentTaskLocal(taskID)
			}
			if firstErr != nil {
				a.actionErr = "Could not delete the agent task: " + firstErr.Error()
			} else {
				a.deleteOpen = false
				a.deleteTasks = nil
				a.deleteTask = ""
			}
			m.invalidate()
		})
	}()
}

func (m *Model) removeAgentTaskLocal(taskID string) {
	a := &m.agents
	delete(a.tasks, taskID)
	delete(a.unread, taskID)
	delete(a.entries, taskID)
	delete(a.entriesLoaded, taskID)
	delete(a.entriesErr, taskID)
	delete(a.entriesBusy, taskID)
	delete(a.changesByTask, taskID)
	delete(a.groupedChanges, taskID)
	delete(a.changesLoaded, taskID)
	delete(a.changesLoading, taskID)
	delete(a.changesErrors, taskID)
	delete(a.changesSequence, taskID)
	if timer := a.changesTimers[taskID]; timer != nil {
		timer.Stop()
		delete(a.changesTimers, taskID)
	}
	a.selectedTasks = removeAgentID(a.selectedTasks, taskID)
	if a.selectionAnchor == taskID {
		a.selectionAnchor = ""
	}
	m.forgetAgentOrganizeTask(taskID)
	m.closeAgentTaskEditor(taskID)
	if a.selected == taskID {
		a.selected = ""
		a.selectedSubagents = nil
		a.conversation = false
	}
	m.clearAgentComposerDraft(taskID)
}

func (m *Model) loadAgentChanges(taskID string) {
	m.refreshAgentChanges(taskID, true)
}

func (m *Model) scheduleAgentChanges(taskID string) {
	if taskID == "" {
		return
	}
	a := &m.agents
	if a.changesTimers == nil {
		a.changesTimers = make(map[string]*time.Timer)
	}
	if a.changesTimers[taskID] != nil {
		return
	}
	root, gen := m.Root(), a.generation
	a.changesTimers[taskID] = time.AfterFunc(500*time.Millisecond, func() {
		m.post(func() {
			delete(m.agents.changesTimers, taskID)
			if m.Root() != root || m.agents.generation != gen {
				return
			}
			if _, ok := m.agents.tasks[taskID]; !ok {
				return
			}
			delete(m.agents.changesLoaded, taskID)
			delete(m.agents.changesErrors, taskID)
			m.refreshAgentChanges(taskID, false)
		})
	})
}

func (m *Model) refreshAgentChanges(taskID string, open bool) {
	task, ok := m.agents.tasks[taskID]
	if taskID == "" || !ok {
		return
	}
	a := &m.agents
	if a.changesByTask == nil {
		a.changesByTask = make(map[string][]agentChange)
	}
	if a.groupedChanges == nil {
		a.groupedChanges = make(map[string]agentChanges)
	}
	if a.changesLoaded == nil {
		a.changesLoaded = make(map[string]bool)
	}
	if a.changesLoading == nil {
		a.changesLoading = make(map[string]bool)
	}
	if a.changesErrors == nil {
		a.changesErrors = make(map[string]string)
	}
	if a.changesSequence == nil {
		a.changesSequence = make(map[string]uint64)
	}
	grouped := m.agentChangesGrouped(task)
	if open {
		a.changesOpen = true
		a.changesTask = taskID
		a.changesBusy = a.changesLoading[taskID]
		a.changesErr = a.changesErrors[taskID]
		if grouped {
			a.changes = append([]agentChange(nil), a.groupedChanges[taskID].Uncommitted...)
		} else {
			a.changes = append([]agentChange(nil), a.changesByTask[taskID]...)
		}
	}
	if a.changesLoading[taskID] || (!open && a.changesLoaded[taskID]) {
		m.invalidate()
		return
	}
	if m.ctx == nil || (m.mgr == nil && m.invokeHook == nil) {
		if open {
			a.changesErr = "The GoIDEA backend is not connected"
			m.invalidate()
		}
		return
	}
	root, gen := m.Root(), a.generation
	a.changesSequence[taskID]++
	sequence := a.changesSequence[taskID]
	a.changesLoading[taskID] = true
	a.changesErrors[taskID] = ""
	if open {
		a.changesBusy = true
		a.changesErr = ""
	}
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
		defer cancel()
		command := "agent_changes"
		if grouped {
			command = "agent_grouped_changes"
		}
		raw, err := m.invoke(ctx, command, map[string]any{"task": taskID})
		var flat []agentChange
		var review agentChanges
		if err == nil && grouped {
			err = json.Unmarshal(raw, &review)
		} else if err == nil {
			err = json.Unmarshal(raw, &flat)
		}
		m.post(func() {
			if a.generation != gen || m.Root() != root ||
				a.changesSequence[taskID] != sequence {
				return
			}
			delete(a.changesLoading, taskID)
			if err != nil {
				a.changesErrors[taskID] = err.Error()
			} else {
				a.changesErrors[taskID] = ""
				a.changesLoaded[taskID] = true
				if grouped {
					a.groupedChanges[taskID] = review
				} else {
					a.changesByTask[taskID] = append([]agentChange(nil), flat...)
					delete(a.groupedChanges, taskID)
				}
			}
			if open || a.changesOpen && a.changesTask == taskID {
				a.changesBusy = false
				a.changesErr = a.changesErrors[taskID]
				if grouped {
					a.changes = append([]agentChange(nil), a.groupedChanges[taskID].Uncommitted...)
				} else {
					a.changes = append([]agentChange(nil), a.changesByTask[taskID]...)
				}
			}
			m.invalidate()
		})
	}()
}

func (m *Model) agentChangesGrouped(task agentTask) bool {
	if task.Isolation != "worktree" {
		return false
	}
	return m.prefs.AgentWorkspace.changesGroupByCommit()
}

func (m *Model) agentReview(task agentTask) ([]agentChange, []agentCommit, bool, bool) {
	a := &m.agents
	grouped := m.agentChangesGrouped(task)
	if grouped {
		review := a.groupedChanges[task.ID]
		return review.Uncommitted, review.Commits, true, a.changesLoaded[task.ID]
	}
	return a.changesByTask[task.ID], nil, false, a.changesLoaded[task.ID]
}

func (m *Model) setAgentChangesGrouped(grouped bool) {
	m.prefs.AgentWorkspace.ChangesGroupByCommit = &grouped
	m.savePreferences()
	taskID := m.agents.changesTask
	if task, ok := m.agents.tasks[taskID]; ok && task.Isolation == "worktree" {
		m.refreshAgentChanges(taskID, true)
	}
	m.invalidate()
}

func (m *Model) acceptAgentChanges() {
	m.runAgentChangeOutcome("agent_accept_changes", "Changes were accepted")
}

func (m *Model) rollbackAgentChanges() {
	m.runAgentChangeOutcome("agent_rollback_changes", "Changes were rolled back")
}

func (m *Model) runAgentChangeOutcome(command, success string) {
	a := &m.agents
	taskID := a.changesTask
	if taskID == "" || a.changesBusy {
		return
	}
	root, gen := m.Root(), a.generation
	a.changesBusy = true
	a.changesErr = ""
	m.invalidate()
	args := map[string]any{"task": taskID, "paths": nil}
	if command == "agent_rollback_changes" {
		view := "sinceStart"
		if task, ok := m.agents.tasks[taskID]; ok && m.agentChangesGrouped(task) {
			view = "group"
		}
		args["view"] = view
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
		defer cancel()
		_, err := m.invoke(ctx, command, args)
		m.post(func() {
			if a.generation != gen || m.Root() != root || a.changesTask != taskID {
				return
			}
			a.changesBusy = false
			if err != nil {
				a.changesErr = err.Error()
			} else {
				m.status = success
				m.refreshAgentChanges(taskID, true)
			}
			m.invalidate()
		})
	}()
}

func (m *Model) forkAgentTask(taskID string) {
	m.forkAgentTaskFrom(taskID, nil)
}

func (m *Model) forkAgentTaskFrom(taskID string, turns *uint32) {
	task, ok := m.agents.tasks[taskID]
	if !ok || !task.Capabilities.Fork {
		return
	}
	a := &m.agents
	root, gen := m.Root(), a.generation
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
		defer cancel()
		var turnsArg any
		if turns != nil {
			turnsArg = *turns
		}
		raw, err := m.invoke(ctx, "agent_fork", map[string]any{"task": taskID, "turns": turnsArg})
		var forked agentTask
		if err == nil {
			err = json.Unmarshal(raw, &forked)
		}
		m.post(func() {
			if a.generation != gen || m.Root() != root {
				return
			}
			if err != nil {
				a.actionErr = "Could not fork the task: " + err.Error()
			} else {
				if a.tasks == nil {
					a.tasks = make(map[string]agentTask)
				}
				a.tasks[forked.ID] = forked
				m.openAgentTask(forked.ID)
			}
			m.invalidate()
		})
	}()
}

type agentTurnPosition struct {
	Number         uint32
	PromptIndex    int
	LastReplyIndex int
}

func agentTurnPositions(entries []agentEntry) []agentTurnPosition {
	turns := make([]agentTurnPosition, 0)
	current := -1
	for index, entry := range entries {
		if entry.Type == "user" && !entry.Steered {
			turns = append(turns, agentTurnPosition{
				Number: uint32(len(turns)), PromptIndex: index, LastReplyIndex: -1,
			})
			current = len(turns) - 1
			continue
		}
		if current >= 0 && entry.Type == "agent" && strings.TrimSpace(entry.Text) != "" {
			turns[current].LastReplyIndex = index
		}
	}
	return turns
}

func agentTurnAt(entries []agentEntry, index int) (agentTurnPosition, bool) {
	for _, turn := range agentTurnPositions(entries) {
		if turn.PromptIndex == index {
			return turn, true
		}
	}
	return agentTurnPosition{}, false
}

func agentTurnForReply(entries []agentEntry, index int) (agentTurnPosition, bool) {
	for _, turn := range agentTurnPositions(entries) {
		if turn.LastReplyIndex == index {
			return turn, true
		}
	}
	return agentTurnPosition{}, false
}

func (m *Model) openAgentMessageEditor(taskID string, entries []agentEntry, index int) {
	turn, ok := agentTurnAt(entries, index)
	task, taskOK := m.agents.tasks[taskID]
	if !ok || !taskOK || !task.Capabilities.Rollback || task.Status == "archived" ||
		agentTaskBusy(task.Status) {
		return
	}
	entry := entries[turn.PromptIndex]
	m.agents.editOpen = true
	m.agents.editTask = taskID
	m.agents.editIndex = turn.PromptIndex
	m.agents.editText = entry.Text
	m.agents.editImages = append([]agentImage(nil), entry.Images...)
	m.agents.editBusy = false
	m.agents.editErr = ""
	m.invalidate()
}

func (m *Model) cancelAgentMessageEditor() {
	m.agents.editOpen = false
	m.agents.editTask = ""
	m.agents.editIndex = -1
	m.agents.editText = ""
	m.agents.editImages = nil
	m.agents.editBusy = false
	m.agents.editErr = ""
	m.invalidate()
}

func (m *Model) submitAgentMessageEditor() {
	a := &m.agents
	task, ok := a.tasks[a.editTask]
	entries := a.entries[agentEntriesKey(a.editTask, "")]
	turn, turnOK := agentTurnAt(entries, a.editIndex)
	text := strings.TrimSpace(a.editText)
	if !ok || !turnOK || a.editBusy || (text == "" && len(a.editImages) == 0) ||
		!task.Capabilities.Rollback || agentTaskBusy(task.Status) {
		return
	}
	root, gen := m.Root(), a.generation
	taskID, numberOfTurn, promptIndex := task.ID, turn.Number, turn.PromptIndex
	images := append([]agentImage(nil), a.editImages...)
	a.editBusy = true
	a.editErr = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 90*time.Second)
		defer cancel()
		if _, err := m.invoke(ctx, "agent_rollback", map[string]any{
			"task": taskID, "turns": numberOfTurn,
		}); err != nil {
			m.post(func() {
				if a.generation != gen || m.Root() != root || !a.editOpen ||
					a.editTask != taskID || a.editIndex != promptIndex {
					return
				}
				a.editBusy = false
				a.editErr = "Could not edit the message: " + err.Error()
				m.invalidate()
			})
			return
		}
		_, err := m.invoke(ctx, "agent_prompt", map[string]any{
			"task": taskID,
			"prompt": map[string]any{
				"text": text, "files": []string{}, "images": images,
			},
			"now": false,
		})
		m.post(func() {
			if a.generation != gen || m.Root() != root {
				return
			}
			if err != nil {
				a.editBusy = false
				a.editErr = "Could not send the edited message: " + err.Error()
				m.invalidate()
				return
			}
			m.cancelAgentMessageEditor()
			m.loadAgentConversation(taskID, "", true)
		})
	}()
}

func (m *Model) retractAgentTurn(taskID string, entries []agentEntry, promptIndex int) {
	a := &m.agents
	task, ok := a.tasks[taskID]
	turn, turnOK := agentTurnAt(entries, promptIndex)
	if !ok || !turnOK || !task.Capabilities.Rollback ||
		task.Status == "archived" || agentTaskBusy(task.Status) {
		return
	}
	root, gen := m.Root(), a.generation
	original := entries[promptIndex]
	a.actionErr = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "agent_rollback", map[string]any{
			"task": taskID, "turns": turn.Number,
		})
		var prompt struct {
			Text   string       `json:"text"`
			Files  []string     `json:"files"`
			Images []agentImage `json:"images"`
		}
		if err == nil && len(raw) > 0 && string(raw) != "null" {
			err = json.Unmarshal(raw, &prompt)
		}
		m.post(func() {
			if a.generation != gen || m.Root() != root {
				return
			}
			if err != nil {
				a.actionErr = "Could not retract the message: " + err.Error()
				m.invalidate()
				return
			}
			if prompt.Text == "" {
				prompt.Text = original.Text
				prompt.Files = append([]string(nil), original.Attachments...)
				prompt.Images = append([]agentImage(nil), original.Images...)
			}
			m.restoreAgentComposer(prompt.Text, prompt.Files, prompt.Images)
			m.loadAgentConversation(taskID, "", true)
		})
	}()
}

func (m *Model) regenerateAgentTurn(taskID string, entries []agentEntry, promptIndex int) {
	a := &m.agents
	task, ok := a.tasks[taskID]
	turns := agentTurnPositions(entries)
	if !ok || len(turns) == 0 || !task.Capabilities.Rollback ||
		task.Status == "archived" || agentTaskBusy(task.Status) {
		return
	}
	turn, turnOK := agentTurnAt(entries, promptIndex)
	if !turnOK || turn.Number != uint32(len(turns)-1) {
		return
	}
	prompt := entries[promptIndex]
	root, gen := m.Root(), a.generation
	a.actionErr = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 90*time.Second)
		defer cancel()
		if _, err := m.invoke(ctx, "agent_rollback", map[string]any{
			"task": taskID, "turns": turn.Number,
		}); err != nil {
			m.post(func() {
				if a.generation != gen || m.Root() != root {
					return
				}
				a.actionErr = "Could not regenerate the response: " + err.Error()
				m.invalidate()
			})
			return
		}
		_, err := m.invoke(ctx, "agent_prompt", map[string]any{
			"task": taskID,
			"prompt": map[string]any{
				"text": prompt.Text, "files": []string{}, "images": prompt.Images,
			},
			"now": false,
		})
		m.post(func() {
			if a.generation != gen || m.Root() != root {
				return
			}
			if err != nil {
				a.actionErr = "Could not regenerate the response: " + err.Error()
			}
			m.loadAgentConversation(taskID, "", true)
		})
	}()
}

func (m *Model) sendAgentMessageAgain(taskID string, entry agentEntry) {
	if taskID == "" || strings.TrimSpace(entry.Text) == "" {
		return
	}
	a := &m.agents
	root, gen := m.Root(), a.generation
	a.actionErr = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, agentSendTimeout)
		defer cancel()
		_, err := m.invoke(ctx, "agent_prompt", map[string]any{
			"task": taskID,
			"prompt": map[string]any{
				"text": entry.Text, "files": []string{}, "images": entry.Images,
			},
			"now": false,
		})
		m.post(func() {
			if a.generation != gen || m.Root() != root {
				return
			}
			if err != nil {
				a.actionErr = "Could not send the message again: " + err.Error()
			}
			m.invalidate()
		})
	}()
}

func (m *Model) restoreAgentComposer(text string, files []string, images []agentImage) {
	a := &m.agents
	if strings.TrimSpace(text) != "" {
		if strings.TrimSpace(a.composer) == "" {
			a.composer = text
		} else {
			a.composer = strings.TrimRight(text, "\n") + "\n\n" + a.composer
		}
	}
	seenFiles := make(map[string]bool, len(a.composerFiles))
	nextFiles := make([]string, 0, len(files)+len(a.composerFiles))
	for _, path := range files {
		if path == "" || seenFiles[path] {
			continue
		}
		nextFiles = append(nextFiles, path)
		seenFiles[path] = true
	}
	for _, path := range a.composerFiles {
		if path != "" && !seenFiles[path] {
			nextFiles = append(nextFiles, path)
			seenFiles[path] = true
		}
	}
	a.composerFiles = nextFiles
	nextImages := append([]agentImage(nil), images...)
	for _, image := range a.composerImages {
		if !agentImagePresent(nextImages, image) {
			nextImages = append(nextImages, image)
		}
	}
	a.composerImages = nextImages
	m.invalidate()
}

func agentPermissionNumber(entry agentEntry) uint32 {
	switch value := entry.ID.(type) {
	case float64:
		return uint32(value)
	case json.Number:
		n, _ := value.Int64()
		return uint32(n)
	case int:
		return uint32(value)
	case uint32:
		return value
	}
	return 0
}

func agentEntryLabel(entry agentEntry) string {
	switch entry.Type {
	case "user":
		return "You"
	case "agent":
		return "Agent"
	case "thought":
		return "Thinking"
	case "tool":
		if entry.Title != "" {
			return entry.Title
		}
		return "Tool"
	case "permission":
		if entry.Title != "" {
			return entry.Title
		}
		return "Permission"
	case "error":
		return "Error"
	case "plan":
		return "Plan"
	case "subagent":
		return "Subagent"
	case "message":
		if entry.Incoming {
			return "Request from parent"
		}
		return "Reply to parent"
	}
	return strings.Title(entry.Type)
}

func agentUpdatedLabel(ms float64) string {
	if ms <= 0 {
		return ""
	}
	when := time.UnixMilli(int64(ms))
	now := time.Now()
	if when.Format("2006-01-02") == now.Format("2006-01-02") {
		return when.Format("15:04")
	}
	if now.Sub(when) < 7*24*time.Hour {
		return when.Format("Mon 15:04")
	}
	return when.Format("Jan 2")
}

func agentProviderName(providers []agentProvider, id string) string {
	for _, provider := range providers {
		if provider.ID == id {
			return provider.Name
		}
	}
	return fmt.Sprintf("%s", id)
}

// agentStatusSummary is the status bar's view of the task list: work waiting
// for the user, work running now, unread results, and the task its click
// follows.
func agentStatusSummary(a *AgentsPane) (input, running, unread int, target string) {
	tasks := make([]agentTask, 0, len(a.tasks))
	for _, task := range a.tasks {
		tasks = append(tasks, task)
		if task.Status == "needsInput" {
			input++
		}
		if task.Status == "running" || task.Status == "starting" {
			running++
		}
		if a.unread[task.ID] {
			unread++
		}
	}
	sort.SliceStable(tasks, func(i, j int) bool {
		return tasks[i].UpdatedMS > tasks[j].UpdatedMS
	})
	for _, task := range tasks {
		if task.Status == "needsInput" {
			return input, running, unread, task.ID
		}
	}
	for _, task := range tasks {
		if a.unread[task.ID] {
			return input, running, unread, task.ID
		}
	}
	for _, task := range tasks {
		if task.Status == "running" || task.Status == "starting" {
			return input, running, unread, task.ID
		}
	}
	return input, running, unread, ""
}
