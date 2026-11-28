package git

import "testing"

func TestRemoteURLForBranchUsesTrackedRemote(t *testing.T) {
	r, git := repo(t)
	git("remote", "add", "origin", "https://github.com/acme/origin.git")
	git("remote", "add", "upstream", "git@github.com:acme/upstream.git")
	git("config", "branch.main.remote", "upstream")
	git("config", "branch.main.merge", "refs/heads/main")

	url, err := r.RemoteURLForBranch("main")
	if err != nil || url != "git@github.com:acme/upstream.git" {
		t.Fatalf("tracked remote = %q, err=%v", url, err)
	}
	git("config", "--unset", "branch.main.remote")
	if _, err := r.RemoteURLForBranch("main"); err == nil {
		t.Fatal("an untracked branch returned a remote")
	}
}
