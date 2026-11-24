package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

const maxCommitMessageHistory = 20

func commitHistoryKey(repo, projectRoot string) string {
	if strings.TrimSpace(repo) != "" {
		return filepath.Clean(repo)
	}
	if strings.TrimSpace(projectRoot) != "" {
		return filepath.Clean(projectRoot)
	}
	return ""
}

func (m *Model) commitMessageHistory(repo string) []string {
	key := commitHistoryKey(repo, m.Root())
	if key == "" {
		return nil
	}
	return append([]string(nil), m.prefs.VCS.CommitMessages[key]...)
}

func (m *Model) rememberCommitMessage(repo, message string) {
	key := commitHistoryKey(repo, m.Root())
	message = strings.TrimSpace(message)
	if key == "" || message == "" {
		return
	}
	if m.prefs.VCS.CommitMessages == nil {
		m.prefs.VCS.CommitMessages = map[string][]string{}
	}
	next := []string{message}
	for _, previous := range m.prefs.VCS.CommitMessages[key] {
		if previous == message || len(next) == maxCommitMessageHistory {
			continue
		}
		next = append(next, previous)
	}
	m.prefs.VCS.CommitMessages[key] = next
	m.savePreferences()
}

func (m *Model) commitMessageHistoryButton(c *ui.Context, s *commitState) {
	history := m.commitMessageHistory(s.repo)
	button := ui.Button(c, "History")
	button.Tooltip("Commit Message History")
	button.Disabled(len(history) == 0)
	button.Menu(func(menu *ui.Menu) {
		for _, message := range history {
			message := message
			label := firstLine(message)
			if len([]rune(label)) > 80 {
				label = string([]rune(label)[:80])
			}
			if menu.Item(label).Chosen() {
				s.message = message
				s.problems = nil
				s.problemsOverride = false
				s.err = ""
				m.invalidate()
			}
		}
	})
}

// preparedCommitMessage removes the comments Git writes into MERGE_MSG and
// commit templates, as OpenIDEA does before putting the text in the field.
func preparedCommitMessage(raw string) string {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	kept := lines[:0]
	for _, line := range lines {
		if !strings.HasPrefix(line, "#") {
			kept = append(kept, line)
		}
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

func commitTemplatePath(value, repo, home string) string {
	value = strings.TrimSpace(value)
	if value == "~" {
		return filepath.Clean(home)
	}
	if strings.HasPrefix(value, "~/") {
		return filepath.Join(home, strings.TrimPrefix(value, "~/"))
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	return filepath.Join(repo, value)
}

func (m *Model) loadPreparedCommitMessage(c *commitState) {
	if c == nil || strings.TrimSpace(c.message) != "" || c.amend {
		return
	}
	if m.mgr == nil && m.invokeHook == nil {
		return
	}
	root := m.Root()
	repo := c.repo
	if repo == "" {
		repo = root
	}
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 15*time.Second)
		defer cancel()
		message := m.commitPreparedMessage(ctx, root, repo)
		if message == "" {
			return
		}
		m.post(func() {
			if c != m.commit || c.amend || strings.TrimSpace(c.message) != "" {
				return
			}
			c.message = message
			m.invalidate()
		})
	}()
}

func (m *Model) setCommitAmend(on bool) {
	c := m.commit
	if c == nil {
		return
	}
	c.problems = nil
	c.problemsOverride = false
	c.err = ""
	if !on {
		c.amend = false
		c.amendGeneration++
		if c.amendMessage != "" && c.message == c.amendMessage {
			c.message = c.amendBefore
		}
		c.amendBefore = ""
		c.amendMessage = ""
		m.invalidate()
		return
	}
	if c.amend {
		return
	}
	c.amend = true
	before := c.message
	c.amendGeneration++
	generation := c.amendGeneration
	if m.mgr == nil && m.invokeHook == nil {
		m.invalidate()
		return
	}
	repo := c.repo
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 20*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "vcs_log", map[string]any{
			"repo": vcsRepositoryArgFor(repo),
			"query": map[string]any{
				"branch": "HEAD", "path": nil, "author": nil, "text": nil,
				"skip": 0, "limit": 1,
			},
		})
		var commits []openidea.Commit
		if err == nil {
			err = decodeOpenIDEAResponse(raw, &commits)
		}
		m.post(func() {
			if c != m.commit || !c.amend || c.amendGeneration != generation ||
				err != nil || len(commits) == 0 {
				return
			}
			last := strings.TrimRightFunc(commits[0].Message, unicode.IsSpace)
			if last == "" || strings.TrimSpace(before) == strings.TrimSpace(last) {
				return
			}
			m.rememberCommitMessage(repo, before)
			c.amendBefore = before
			c.amendMessage = last
			c.message = last
			m.invalidate()
		})
	}()
}

func (m *Model) commitPreparedMessage(ctx context.Context, root, repo string) string {
	repoArg := vcsRepositoryArgFor(repo)
	infoRaw, infoErr := m.invoke(ctx, "vcs_branch_info", map[string]any{"repo": repoArg})
	var info openidea.BranchInfo
	if infoErr == nil {
		infoErr = decodeOpenIDEAResponse(infoRaw, &info)
	}
	if infoErr == nil && info.Operation != nil {
		switch *info.Operation {
		case "merge", "cherry-pick", "revert":
			gitDir := m.commitGitDir(ctx, root)
			if text, err := m.readCommitText(ctx, filepath.Join(gitDir, "MERGE_MSG")); err == nil {
				if prepared := preparedCommitMessage(text); prepared != "" {
					return prepared
				}
			}
		}
	}
	raw, err := m.invoke(ctx, "vcs_config_get", map[string]any{
		"repo": repoArg, "key": "commit.template",
	})
	var value *string
	if err != nil {
		return ""
	}
	if decodeOpenIDEAResponse(raw, &value) != nil || value == nil || strings.TrimSpace(*value) == "" {
		return ""
	}
	home, _ := os.UserHomeDir()
	path := commitTemplatePath(*value, root, home)
	text, err := m.readCommitText(ctx, path)
	if err != nil {
		return ""
	}
	return preparedCommitMessage(text)
}

func (m *Model) commitGitDir(ctx context.Context, root string) string {
	dotGit := filepath.Join(root, ".git")
	text, err := m.readCommitText(ctx, dotGit)
	if err == nil {
		if marker := strings.TrimSpace(strings.TrimPrefix(text, "gitdir:")); marker != text {
			if !filepath.IsAbs(marker) {
				marker = filepath.Join(root, marker)
			}
			return filepath.Clean(marker)
		}
	}
	return dotGit
}

func (m *Model) readCommitText(ctx context.Context, path string) (string, error) {
	raw, err := m.invoke(ctx, "fs_read_text", map[string]any{"path": path})
	if err != nil {
		return "", err
	}
	var text string
	if err := decodeOpenIDEAResponse(raw, &text); err != nil {
		return "", err
	}
	return text, nil
}

func decodeOpenIDEAResponse(raw json.RawMessage, out any) error {
	return json.Unmarshal(raw, out)
}
