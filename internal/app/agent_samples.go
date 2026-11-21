package app

import (
	"fmt"
	"strings"
)

// hiddenActionIDs are registered for OpenIDEA compatibility but, like the
// upstream entries marked hidden, do not appear in the command palette.
var hiddenActionIDs = map[string]bool{
	"agents.openSampleConversations": true,
	"agents.retryConversation":       true,
	"agents.openTaskInEditor":        true,
	"agents.openTask":                true,
	"agents.dockTask":                true,
	"agents.undockTask":              true,
	"agents.renameTask":              true,
	"agents.forkTask":                true,
	"agents.archiveTasks":            true,
	"agents.unarchiveTasks":          true,
	"agents.deleteTasks":             true,
	"agents.pinTasks":                true,
	"agents.unpinTasks":              true,
	"agents.newTaskGroup":            true,
	"agents.renameTaskGroup":         true,
	"agents.removeTaskGroup":         true,
	"agents.copyReplyMarkdown":       true,
	"agents.jumpToLatest":            true,
	"agents.retractMessage":          true,
	"agents.quoteReply":              true,
	"agents.regenerateReply":         true,
	"agents.forkReply":               true,
	"code.optimizeImportsFile":       true,
	"code.reformatFile":              true,
	"edit.copy":                      true,
	"editor.addOrRemoveCaret":        true,
	"edit.cut":                       true,
	"edit.paste":                     true,
	"edit.redo":                      true,
	"edit.selectAll":                 true,
	"edit.undo":                      true,
	"file.new":                       true,
	"file.openPaths":                 true,
	"file.openProjectPath":           true,
	"file.openRecentProject":         true,
	"window.closeTab":                true,
	"window.reorderTabs":             true,
	"window.selectTab":               true,
	"view.toggleStatusBarWidget":     true,
	"java.downloadJdk":               true,
	"run.createConfig":               true,
	"tools.install":                  true,
	"tools.promptInstall":            true,
	"project.optimizeImports":        true,
	"debug.pluginStart":              true,
	"navigate.switcherBack":          true,
	"merge.change":                   true,
}

// OpenAgentSampleConversations creates the long in-memory conversations used
// by OpenIDEA's performance harness. They never call the backend or write the
// project, so switching and scrolling can be benchmarked in a release build.
func (m *Model) OpenAgentSampleConversations(count, turns int) {
	count = max(count, 1)
	turns = max(turns, 1)
	root := m.Root()
	if root == "" {
		root = "/"
	}
	a := &m.agents
	if a.tasks == nil {
		a.tasks = make(map[string]agentTask)
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
	a.loaded = true

	ids := make([]string, 0, count)
	for index := 1; index <= count; index++ {
		id := fmt.Sprintf("sample-%d", index)
		a.tasks[id] = agentTask{
			ID:        id,
			Title:     fmt.Sprintf("Sample task %d", index),
			Provider:  "claude",
			Status:    "ready",
			Isolation: "worktree",
			Cwd:       root,
		}
		a.entries[id] = agentSampleEntries(root, turns)
		a.entriesLoaded[id] = true
		ids = append(ids, id)
	}
	for _, id := range ids {
		m.openAgentTask(id)
	}
}

func agentSampleEntries(cwd string, turns int) []agentEntry {
	entries := make([]agentEntry, 0, turns*13)
	for turn := 0; turn < turns; turn++ {
		entries = append(entries, agentEntry{
			Type: "user",
			Text: fmt.Sprintf(
				"Turn %d: make the order service retry failed payments and cover it with tests",
				turn),
		})
		entries = append(entries, agentEntry{
			Type: "thought",
			Text: fmt.Sprintf(
				"The payment client throws on timeouts; retrying belongs in the service (turn %d).",
				turn),
		})
		for tool := 0; tool < 10; tool++ {
			path := fmt.Sprintf("%s/src/orders/Order%d.kt", cwd, (turn+tool)%40)
			title := fmt.Sprintf("grep -rn \"order%d\" src", turn)
			kind := "execute"
			switch tool % 3 {
			case 1:
				title, kind = "Read "+path, "read"
			case 2:
				title, kind = "Edit "+path, "edit"
			}
			var output strings.Builder
			for line := 0; line < 12; line++ {
				fmt.Fprintf(&output, "%s/src/orders/Order%d.kt:%d:  val order%d = repository.find(id%d)\n",
					cwd, line, line*7+3, turn, line)
			}
			text := output.String()
			entry := agentEntry{
				Type: "tool", Title: title, Kind: kind,
				Status: "completed", Output: &text,
			}
			if kind == "edit" {
				entry.Diffs = []agentToolDiff{{
					Path: path, Added: uint32(4 + tool%5), Removed: 2,
				}}
			}
			entries = append(entries, entry)
		}
		entries = append(entries, agentEntry{
			Type: "agent",
			Text: fmt.Sprintf(strings.Join([]string{
				"## Turn %d",
				"",
				"Payments now retry **three times** with backoff before the order fails. Changed `OrderService.kt` and added tests:",
				"",
				"- `retries` on timeouts, not on declined cards",
				"- waits 200 ms, then 400 ms, then 800 ms",
				"- logs each attempt with the order id",
				"",
				"```kotlin",
				"suspend fun pay%d(order: Order): Receipt =",
				"    retry(times = 3, initialDelay = 200.milliseconds) {",
				"        payments.charge(order.id, order.total)",
				"    }",
				"```",
				"",
				"| Case | Before | After |",
				"|---|---|---|",
				"| Timeout | failed | retried |",
				"| Declined | failed | failed |",
			}, "\n"), turn, turn),
		})
	}
	return entries
}
