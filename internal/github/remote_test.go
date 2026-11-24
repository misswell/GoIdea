package github

import "testing"

func TestParseGitHubRemotes(t *testing.T) {
	tests := []struct {
		remote string
		owner  string
		name   string
	}{
		{"https://github.com/acme/goidea.git", "acme", "goidea"},
		{"ssh://git@github.com/acme/goidea.git", "acme", "goidea"},
		{"git@github.com:acme/goidea.git", "acme", "goidea"},
		{"github.com:acme/goidea.git", "acme", "goidea"},
	}
	for _, test := range tests {
		got, err := ParseRemote(test.remote)
		if err != nil || got.Owner != test.owner || got.Name != test.name {
			t.Errorf("ParseRemote(%q) = %+v, %v", test.remote, got, err)
		}
	}
}

func TestParseGitHubRemoteRejectsUnsafeAddresses(t *testing.T) {
	for _, remote := range []string{
		"",
		"file:///tmp/repo",
		"https://token@github.com/acme/goidea.git",
		"https://github.com:8443/acme/goidea.git",
		"https://github.com/acme/../goidea.git",
		"https://github.com/acme/goidea?token=secret",
		"git@example.com:acme/goidea.git",
	} {
		if got, err := ParseRemote(remote); err == nil {
			t.Errorf("ParseRemote(%q) = %+v, want error", remote, got)
		}
	}
}

func TestGitHubSearchQueryMatchesOpenIDEAFilters(t *testing.T) {
	repo := Coordinates{Owner: "acme", Name: "goidea"}
	filters := SearchFilters{
		Text:        "fix login\n timeout",
		State:       "OPEN",
		Assignee:    "me",
		ReviewState: "AWAITING_REVIEW",
		Author:      "octocat",
		Label:       `needs "review"`,
		Sort:        "RECENTLY_UPDATED",
	}
	want := `type:pr repo:acme/goidea (fix login timeout) is:open assignee:me review-requested:@me author:octocat label:"needs \"review\"" sort:updated-desc`
	if got := filters.Query(repo); got != want {
		t.Fatalf("Query() = %q, want %q", got, want)
	}
}
