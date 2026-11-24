package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"mayoi/internal/openidea"
)

func TestPreparedCommitMessageRemovesGitComments(t *testing.T) {
	raw := "Merge branch 'feature'\n\n# Conflicts:\n#\tsrc/App.java\n\nKeep this line\n"
	if got := preparedCommitMessage(raw); got != "Merge branch 'feature'\n\n\nKeep this line" {
		t.Fatalf("prepared message = %q", got)
	}
	root := filepath.Join(string(filepath.Separator), "repo")
	if got := commitTemplatePath("~/template.txt", root, "/Users/ada"); got != "/Users/ada/template.txt" {
		t.Fatalf("home template = %q", got)
	}
	if got := commitTemplatePath("~", root, "/Users/ada"); got != "/Users/ada" {
		t.Fatalf("bare home template = %q", got)
	}
	if got := commitTemplatePath(".gitmessage", root, "/Users/ada"); got != filepath.Join(root, ".gitmessage") {
		t.Fatalf("relative template = %q", got)
	}
}

func TestPreparedMergeMessageReadsMergeMsg(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, ".git", "MERGE_MSG"),
		[]byte("Merge branch 'main'\n\n# Conflicts:\n#\tsrc/App.java\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	m := &Model{
		ctx: context.Background(),
		invokeHook: func(_ context.Context, command string, args any) (json.RawMessage, error) {
			switch command {
			case "vcs_branch_info":
				operation := "merge"
				return json.Marshal(openidea.BranchInfo{Operation: &operation})
			case "fs_read_text":
				path := args.(map[string]any)["path"].(string)
				text, err := os.ReadFile(path)
				if err != nil {
					return nil, err
				}
				return json.Marshal(string(text))
			default:
				t.Fatalf("command = %q", command)
				return nil, nil
			}
		},
	}
	if got := m.commitPreparedMessage(context.Background(), root, root); got != "Merge branch 'main'" {
		t.Fatalf("prepared merge message = %q", got)
	}
}

func TestCommitMessageHistoryIsBoundedAndDeduplicated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	root := t.TempDir()
	m := &Model{
		cfg: Config{SettingsPath: path},
		prefs: Preferences{
			VCS: VCSPrefs{CommitMessages: map[string][]string{}},
		},
	}
	m.setRoot(root)
	for i := 0; i < maxCommitMessageHistory+3; i++ {
		m.rememberCommitMessage(root, " message "+string(rune('A'+i)))
	}
	m.rememberCommitMessage(root, "message "+string(rune('A'+maxCommitMessageHistory+2)))
	history := m.commitMessageHistory(root)
	if len(history) != maxCommitMessageHistory {
		t.Fatalf("history length = %d", len(history))
	}
	if history[0] != "message "+string(rune('A'+maxCommitMessageHistory+2)) {
		t.Fatalf("newest history = %q", history[0])
	}
	count := 0
	for _, message := range history {
		if message == history[0] {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("duplicate newest message count = %d", count)
	}
	stored := LoadPreferences(path).VCS.CommitMessages[filepath.Clean(root)]
	if !reflect.DeepEqual(stored, history) {
		t.Fatalf("stored history = %v, want %v", stored, history)
	}
}

func TestAmendLoadsHeadAndRestoresUntouchedDraft(t *testing.T) {
	root := t.TempDir()
	loaded := make(chan struct{}, 1)
	m := &Model{
		ctx: context.Background(),
		cfg: Config{SettingsPath: filepath.Join(t.TempDir(), "settings.json")},
		prefs: Preferences{
			VCS: VCSPrefs{CommitMessages: map[string][]string{}},
		},
		invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			if command != "vcs_log" {
				t.Fatalf("command = %q", command)
			}
			return json.Marshal([]openidea.Commit{{Message: "Last commit\n\nBody\n"}})
		},
		postHook: func(fn func()) {
			fn()
			loaded <- struct{}{}
		},
	}
	m.setRoot(root)
	m.commit = &commitState{message: "Uncommitted draft", repo: root}
	m.setCommitAmend(true)
	<-loaded
	if m.commit.message != "Last commit\n\nBody" {
		t.Fatalf("amend message = %q", m.commit.message)
	}
	if history := m.commitMessageHistory(root); len(history) != 1 || history[0] != "Uncommitted draft" {
		t.Fatalf("amend history = %v", history)
	}
	m.setCommitAmend(false)
	if m.commit.message != "Uncommitted draft" {
		t.Fatalf("restored draft = %q", m.commit.message)
	}
	m.setCommitAmend(true)
	<-loaded
	m.commit.message = "Edited while amending"
	m.setCommitAmend(false)
	if m.commit.message != "Edited while amending" {
		t.Fatalf("edited amend message was overwritten: %q", m.commit.message)
	}
}
