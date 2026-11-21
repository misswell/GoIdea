package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// agentDiffTarget is one file in an agent review. Grouped targets compare
// HEAD with the working tree; commit targets compare a commit with its parent.
type agentDiffTarget struct {
	Change  agentChange
	Commit  *agentCommit
	Grouped bool
}

func (m *Model) showAgentChangeDiff(taskID string, change agentChange, commit *agentCommit) {
	task, ok := m.agents.tasks[taskID]
	if !ok {
		return
	}
	var targets []agentDiffTarget
	grouped := commit == nil && m.agentChangesGrouped(task)
	if commit != nil {
		for index := range commit.Files {
			targets = append(targets, agentDiffTarget{Change: commit.Files[index], Commit: commit})
		}
	} else if grouped {
		for _, item := range m.agents.groupedChanges[taskID].Uncommitted {
			targets = append(targets, agentDiffTarget{Change: item, Grouped: true})
		}
	} else {
		for _, item := range m.agents.changesByTask[taskID] {
			targets = append(targets, agentDiffTarget{Change: item})
		}
	}
	index := agentDiffTargetIndex(targets, change.Path)
	if len(targets) == 0 {
		targets = []agentDiffTarget{{Change: change, Grouped: grouped}}
		index = 0
	}
	m.loadAgentDiff(taskID, targets, index)
}

func agentDiffTargetIndex(targets []agentDiffTarget, path string) int {
	for index := range targets {
		if targets[index].Change.Path == path {
			return index
		}
	}
	for index := range targets {
		if targets[index].Change.RelPath != "" && targets[index].Change.RelPath == path {
			return index
		}
	}
	return 0
}

func agentChangePaths(change agentChange) (string, string) {
	path := change.Path
	if path == "" {
		path = change.RelPath
	}
	oldPath := path
	if change.OldPath != nil && *change.OldPath != "" {
		oldPath = *change.OldPath
	}
	return path, oldPath
}

func agentChangeKind(change agentChange) string {
	return strings.ToLower(strings.TrimSpace(change.Kind))
}

func agentChangeAbsent(change agentChange) bool {
	switch agentChangeKind(change) {
	case "added", "untracked":
		return true
	}
	return false
}

func agentChangeDeleted(change agentChange) bool {
	return agentChangeKind(change) == "deleted"
}

func agentChangeAbsolutePath(task agentTask, change agentChange) string {
	path, _ := agentChangePaths(change)
	return agentPathForTask(task, path)
}

func agentPathForTask(task agentTask, path string) string {
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	if task.Cwd == "" {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(task.Cwd, path))
}

func (m *Model) loadAgentDiff(taskID string, targets []agentDiffTarget, index int) {
	task, ok := m.agents.tasks[taskID]
	if !ok || len(targets) == 0 {
		return
	}
	if index < 0 {
		index = 0
	}
	if index >= len(targets) {
		index = len(targets) - 1
	}
	target := targets[index]
	path, oldPath := agentChangePaths(target.Change)
	absolute := agentChangeAbsolutePath(task, target.Change)
	d := &diffView{
		open:       true,
		path:       absolute,
		loading:    true,
		options:    m.diffPrefs(),
		agentTask:  taskID,
		agentChain: append([]agentDiffTarget(nil), targets...),
		agentIndex: index,
	}
	d.leftLabel, d.rightLabel = agentDiffLabels(task, target)
	m.diff = d
	m.invalidate()

	rightText, fromTab := m.bufferText(absolute)
	root, gen := m.Root(), m.agents.generation
	go func() {
		left, right, err := m.agentDiffTexts(task, target, path, oldPath, absolute, rightText, fromTab)
		_, added, gone := diffRows(left, right)
		m.post(func() {
			if d.isStale(root, gen, m) {
				return
			}
			d.loading = false
			if err != nil {
				d.err = diffErrorText(err)
				m.invalidate()
				return
			}
			d.leftText, d.rightText = left, right
			d.added, d.gone = added, gone
			if added == 0 && gone == 0 {
				d.message = d.leftLabel + " is identical to " + d.rightLabel
			} else {
				d.setDiffRows(left, right)
			}
			m.invalidate()
		})
	}()
}

func (d *diffView) isStale(root string, generation uint64, m *Model) bool {
	return m.diff != d || m.Root() != root || m.agents.generation != generation
}

func (m *Model) agentDiffTexts(task agentTask, target agentDiffTarget,
	path, oldPath, absolute, buffered string, fromTab bool) (string, string, error) {
	if m.ctx == nil || (m.mgr == nil && m.invokeHook == nil) {
		return "", "", errors.New("The GoIDEA backend is not connected")
	}
	ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
	defer cancel()
	if task.Isolation == "worktree" && task.Cwd != "" {
		_, _ = m.invoke(ctx, "vcs_open_external_log", map[string]any{"path": task.Cwd})
	}
	left, right := "", ""
	switch {
	case target.Commit != nil:
		commit := target.Commit
		if commit.Parent != nil && *commit.Parent != "" && !agentChangeAbsent(target.Change) {
			value, err := m.agentVCSFileAt(ctx, task, agentPathForTask(task, oldPath), *commit.Parent)
			if err != nil {
				return "", "", err
			}
			left = value
		}
		if !agentChangeDeleted(target.Change) {
			value, err := m.agentVCSFileAt(ctx, task, absolute, commit.ID)
			if err != nil {
				return "", "", err
			}
			right = value
		}
	case target.Grouped:
		if !agentChangeAbsent(target.Change) {
			value, err := m.agentVCSFileAt(ctx, task, agentPathForTask(task, oldPath), "HEAD")
			if err != nil {
				return "", "", err
			}
			left = value
		}
		if !agentChangeDeleted(target.Change) {
			if fromTab {
				right = buffered
			} else {
				data, err := os.ReadFile(absolute)
				if err != nil && !errors.Is(err, os.ErrNotExist) {
					return "", "", err
				}
				right = string(data)
			}
		}
	default:
		raw, err := m.invoke(ctx, "agent_base_text", map[string]any{
			"task": task.ID, "path": absolute,
		})
		if err != nil {
			return "", "", err
		}
		var base *string
		if len(raw) > 0 && string(raw) != "null" {
			if err := json.Unmarshal(raw, &base); err != nil {
				return "", "", err
			}
		}
		if base != nil {
			left = *base
		}
		if !agentChangeDeleted(target.Change) {
			if fromTab {
				right = buffered
			} else {
				data, err := os.ReadFile(absolute)
				if err != nil && !errors.Is(err, os.ErrNotExist) {
					return "", "", err
				}
				right = string(data)
			}
		}
	}
	return left, right, nil
}

func (m *Model) agentVCSFileAt(ctx context.Context, task agentTask,
	path, revision string) (string, error) {
	if path == "" {
		return "", nil
	}
	raw, err := m.invoke(ctx, "vcs_file_at", map[string]any{
		"path": path, "rev": revision,
	})
	if err != nil {
		return "", err
	}
	var value *string
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", err
		}
	}
	if value == nil {
		return "", nil
	}
	return *value, nil
}

func agentDiffLabels(task agentTask, target agentDiffTarget) (string, string) {
	if target.Commit != nil {
		commit := target.Commit
		right := commit.ShortID
		if right == "" && len(commit.ID) >= 7 {
			right = commit.ID[:7]
		}
		if right == "" {
			right = "Commit"
		}
		left := "(does not exist)"
		if commit.Parent != nil && *commit.Parent != "" {
			left = *commit.Parent
			if len(left) >= 7 {
				left = left[:7]
			}
		}
		return left, right
	}
	if target.Grouped {
		left, right := "HEAD", "Working tree"
		if agentChangeAbsent(target.Change) {
			left = "(does not exist)"
		}
		if agentChangeDeleted(target.Change) {
			right = "(deleted)"
		} else if branch := agentBranchText(task.Branch); branch != "" {
			right = branch
		}
		return left, right
	}
	left := "Base"
	if task.Base != nil && *task.Base != "" {
		left = *task.Base
	}
	if agentChangeAbsent(target.Change) {
		left = "(does not exist)"
	}
	right := "Working tree"
	if branch := agentBranchText(task.Branch); branch != "" {
		right = branch
	}
	if agentChangeDeleted(target.Change) {
		right = "(deleted)"
	}
	return left, right
}

func agentDiffSummary(changes []agentChange) (int, int) {
	added, removed := 0, 0
	for _, change := range changes {
		added += int(change.Added)
		removed += int(change.Removed)
	}
	return added, removed
}

func agentDiffCommitStats(commit agentCommit) (int, int) {
	return agentDiffSummary(commit.Files)
}

func agentDiffCommitLabel(commit agentCommit) string {
	short := commit.ShortID
	if short == "" && len(commit.ID) >= 7 {
		short = commit.ID[:7]
	}
	summary := strings.TrimSpace(commit.Summary)
	if summary == "" {
		summary = strings.TrimSpace(commit.Message)
	}
	if short == "" {
		return summary
	}
	if summary == "" {
		return short
	}
	return short + " " + summary
}

func agentDiffTimeLabel(ms *float64) string {
	if ms == nil {
		return ""
	}
	value := int64(*ms)
	if value <= 0 {
		return ""
	}
	return time.UnixMilli(value).Format("Jan 2 15:04")
}

func agentDiffCountLabel(count int, singular, plural string) string {
	if count == 1 {
		return "1 " + singular
	}
	return fmt.Sprintf("%d %s", count, plural)
}
