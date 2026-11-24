package app

import (
	"context"
	"fmt"
	"strings"

	"mayoi/internal/openidea"
)

func (m *Model) undoCommitBranchInfo(
	ctx context.Context,
	repository string,
) (openidea.BranchInfo, error) {
	var info openidea.BranchInfo
	raw, err := m.invoke(ctx, "vcs_branch_info", map[string]any{
		"repo": vcsRepositoryArgFor(repository),
	})
	if err != nil {
		return info, err
	}
	if err := decodeOpenIDEAResponse(raw, &info); err != nil {
		return info, err
	}
	return info, nil
}

// undoCommitProblem is OpenIDEA's preflight for Undo Commit. GoIDEA also
// refuses every detached HEAD because there is no branch whose history can be
// safely rewritten.
func undoCommitProblem(commit openidea.Commit, branch openidea.BranchInfo) string {
	if branch.Operation != nil && strings.TrimSpace(*branch.Operation) != "" {
		return fmt.Sprintf("Cannot undo a commit while a %s is in progress", strings.TrimSpace(*branch.Operation))
	}
	if branch.Head == nil {
		return "Undo Commit is unavailable on a detached HEAD"
	}
	if branch.HeadCommit == nil || commit.ID != *branch.HeadCommit {
		return "Undo Commit is available only for the latest commit of the current branch"
	}
	if len(commit.Parents) > 1 {
		return "A merge commit cannot be undone; use Reset Current Branch to Here instead"
	}
	return ""
}

// undoCommitHeadPushed reports that HEAD itself is already on its upstream.
func undoCommitHeadPushed(branch openidea.BranchInfo) bool {
	return branch.Upstream != nil &&
		strings.TrimSpace(*branch.Upstream) != "" &&
		branch.Ahead == 0
}

// undoCommitPushedProtectedProblem refuses rewriting history already pushed to
// a branch configured as protected. Unpushed protected-branch commits remain
// editable because no remote history is rewritten.
func undoCommitPushedProtectedProblem(
	branch openidea.BranchInfo,
	protected func(string) bool,
) string {
	if !undoCommitHeadPushed(branch) || branch.Head == nil || protected == nil {
		return ""
	}
	name := strings.TrimSpace(*branch.Head)
	if name == "" || !protected(name) {
		return ""
	}
	return fmt.Sprintf("Pushed commits of the protected branch '%s' cannot be rewritten", name)
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
