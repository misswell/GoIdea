package git

import (
	"fmt"
	"strings"
)

// RemoteURLForBranch returns the fetch URL of the remote tracked by a local
// branch.
func (r *Repo) RemoteURLForBranch(branch string) (string, error) {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return "", fmt.Errorf("branch name is required")
	}
	ref := "refs/heads/" + branch
	if _, err := r.run(nil, "check-ref-format", ref); err != nil {
		return "", err
	}
	out, err := r.run(
		nil,
		"for-each-ref",
		"--format=%(upstream:remotename)",
		ref,
	)
	if err != nil {
		return "", err
	}
	remote := strings.TrimSpace(string(out))
	if remote == "" {
		return "", fmt.Errorf("branch %q has no remote upstream", branch)
	}
	return r.RemoteURL(remote)
}
