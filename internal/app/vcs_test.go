package app

import (
	"testing"

	"mayoi/internal/openidea"
)

func str(s string) *string { return &s }

func TestVCSBadge(t *testing.T) {
	cases := []struct {
		name         string
		staged       *string
		unstaged     *string
		letter, word string
	}{
		{"modified in worktree", nil, str("modified"), "M", "已修改"},
		{"staged addition", str("added"), nil, "A", "新增"},
		{"staged and edited", str("added"), str("modified"), "M", "已修改"},
		{"deleted", nil, str("deleted"), "D", "已删除"},
		{"renamed", str("renamed"), nil, "R", "已重命名"},
		{"conflicted", nil, str("conflicted"), "!", "冲突"},
		{"untracked", nil, str("untracked"), "U", "未跟踪"},
		{"ignored", nil, str("ignored"), "I", "已忽略"},
		{"no kind", nil, nil, "?", "已更改"},
	}
	for _, c := range cases {
		f := openidea.FileStatus{RelPath: "a.java", Staged: c.staged, Unstaged: c.unstaged}
		letter, word, tone := vcsBadge(f)
		if letter != c.letter || word != c.word || tone == "" {
			t.Errorf("%s: got %q %q %q, want %q %q", c.name, letter, word, tone, c.letter, c.word)
		}
	}
}
