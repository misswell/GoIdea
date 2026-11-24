package github

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type staticCredentials string

func (c staticCredentials) Token(context.Context) (string, error) {
	if c == "" {
		return "", ErrNoToken
	}
	return string(c), nil
}

func TestGitHubClientLoadsRepositoryAndSearchPage(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer test-token" ||
			r.Header.Get("X-GitHub-Api-Version") != "2022-11-28" {
			t.Errorf("headers = %#v", r.Header)
		}
		var request struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(request.Query, "GoIDEARepository"):
			io.WriteString(w, `{"data":{"viewer":{"login":"me","name":"Me","avatarUrl":"https://avatar/me"},
				"repository":{"id":"R_1","nameWithOwner":"acme/goidea","viewerPermission":"WRITE",
				"mergeCommitAllowed":true,"squashMergeAllowed":true,"rebaseMergeAllowed":false,
				"defaultBranchRef":{"name":"main"},"url":"https://github.com/acme/goidea"}}}`)
		case strings.Contains(request.Query, "GoIDEASearchPullRequests"):
			if request.Variables["cursor"] != "next-page" {
				t.Errorf("cursor = %#v", request.Variables["cursor"])
			}
			if !strings.Contains(request.Variables["query"].(string), "is:open") {
				t.Errorf("search query = %#v", request.Variables["query"])
			}
			io.WriteString(w, `{"data":{"search":{"issueCount":12,
				"pageInfo":{"hasNextPage":true,"endCursor":"cursor-2"},
				"nodes":[{"__typename":"PullRequest","id":"PR_1","url":"https://github.com/acme/goidea/pull/7",
				"number":7,"title":"Fix login","state":"OPEN","isDraft":false,
				"author":{"login":"octocat","avatarUrl":"https://avatar/octocat"},"createdAt":"2026-01-01T00:00:00Z",
				"updatedAt":"2026-01-02T00:00:00Z","isReadByViewer":false,"reviewDecision":"APPROVED",
				"mergeable":"MERGEABLE","labels":{"nodes":[{"name":"bug","color":"d73a4a"}]},
				"assignees":{"nodes":[{"login":"me"}]},"comments":{"totalCount":3},
				"reviewThreads":{"nodes":[{"isResolved":false},{"isResolved":true}]},
				"reviews":{"nodes":[{"id":"REV_1","url":"https://review","author":{"login":"reviewer"},
				"body":"Looks good","state":"APPROVED","createdAt":"2026-01-03T00:00:00Z"}]},
				"commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"SUCCESS"}}}]}}]}}}`)
		default:
			t.Fatalf("unexpected GraphQL query: %s", request.Query)
		}
	}))
	defer server.Close()

	client := NewClient(staticCredentials("test-token"), ClientOptions{
		GraphQLEndpoint: server.URL + "/graphql", APIBase: server.URL,
	})
	repo := Coordinates{Owner: "acme", Name: "goidea"}
	repository, viewer, err := client.Repository(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if repository.ID != "R_1" || repository.DefaultBranch != "main" || viewer.Login != "me" {
		t.Fatalf("repository = %+v viewer = %+v", repository, viewer)
	}
	page, err := client.SearchPullRequests(
		context.Background(), repo, DefaultSearchFilters(), "next-page",
	)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || page.Total != 12 || !page.HasMore || page.EndCursor != "cursor-2" ||
		len(page.PullRequests) != 1 {
		t.Fatalf("page = %+v, calls=%d", page, calls)
	}
	pr := page.PullRequests[0]
	if pr.Number != 7 || pr.Author != "octocat" || pr.CheckState != "SUCCESS" ||
		pr.UnresolvedThreads != 1 || pr.CommentCount != 3 || len(pr.Labels) != 1 {
		t.Fatalf("pull request = %+v", pr)
	}
}

func TestGitHubClientLoadsPullRequestDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"data":{"repository":{"pullRequest":{
			"id":"PR_9","url":"https://github.com/acme/goidea/pull/9","number":9,"title":"Ship it",
			"state":"OPEN","isDraft":false,"body":"Body","author":{"login":"me","avatarUrl":""},
			"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-02T00:00:00Z",
			"isReadByViewer":true,"reviewDecision":"REVIEW_REQUIRED","mergeable":"MERGEABLE",
			"mergeStateStatus":"BEHIND","canBeRebased":true,"viewerCanUpdate":true,"viewerDidAuthor":true,
			"viewerCanDeleteHeadRef":true,"baseRefName":"main","baseRefOid":"base",
			"baseRepository":{"url":"https://github.com/acme/goidea"},"headRefName":"topic","headRefOid":"head",
			"headRepository":{"url":"https://github.com/me/goidea"},"labels":{"nodes":[]},
			"assignees":{"nodes":[]},"comments":{"totalCount":0},
			"reviewThreads":{"nodes":[{"isResolved":false}]},
			"reviews":{"nodes":[]},
			"files":{"nodes":[{"path":"src/App.java","changeType":"MODIFIED","additions":3,
				"deletions":1,"viewerViewedState":"UNVIEWED"}]},
			"commits":{"nodes":[{"url":"https://commit","commit":{"oid":"head","abbreviatedOid":"head",
				"messageHeadline":"Ship it","committedDate":"2026-01-02T00:00:00Z",
				"author":{"name":"Me","user":{"login":"me"}},
				"statusCheckRollup":{"state":"FAILURE","contexts":{"nodes":[
					{"__typename":"CheckRun","name":"test","status":"COMPLETED","conclusion":"FAILURE",
					"detailsUrl":"https://check","checkSuite":{"workflowRun":{"workflow":{"name":"CI"}}}},
					{"__typename":"StatusContext","context":"lint","state":"SUCCESS",
					"targetUrl":"https://status"}
				]}}}}]},
			"timelineItems":{"nodes":[
				{"__typename":"PullRequestCommit","url":"https://commit","commit":{"oid":"head",
				"abbreviatedOid":"head","messageHeadline":"Ship it","committedDate":"2026-01-02T00:00:00Z",
				"author":{"name":"Me","user":{"login":"me"}}}},
				{"__typename":"IssueComment","url":"https://comment","body":"Ready","createdAt":"2026-01-03T00:00:00Z",
				"author":{"login":"reviewer"}}
			]}
		}}}}`)
	}))
	defer server.Close()
	client := NewClient(staticCredentials("token"), ClientOptions{
		GraphQLEndpoint: server.URL, APIBase: server.URL,
	})
	detail, err := client.PullRequest(
		context.Background(), Coordinates{Owner: "acme", Name: "goidea"}, 9,
	)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Number != 9 || detail.HeadRef != "topic" || detail.Files[0].Path != "src/App.java" ||
		len(detail.Checks) != 2 || detail.Checks[0].Workflow != "CI" ||
		len(detail.Timeline) != 2 || detail.UnresolvedThreads != 1 {
		t.Fatalf("detail = %+v", detail)
	}
}

func TestGitHubClientCreatesAndUpdatesPullRequest(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/branches"):
			io.WriteString(w, `[{"name":"main"},{"name":"topic"}]`)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/pulls"):
			io.WriteString(w, `{"id":1,"node_id":"PR_1","html_url":"https://github.com/acme/goidea/pull/1",
				"number":1,"title":"Change","state":"open","draft":true,
				"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z",
				"user":{"login":"me","avatar_url":"https://avatar"}}`)
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/update-branch"):
			w.WriteHeader(http.StatusAccepted)
			io.WriteString(w, `{}`)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL)
		}
	}))
	defer server.Close()
	client := NewClient(staticCredentials("token"), ClientOptions{
		GraphQLEndpoint: server.URL + "/graphql", APIBase: server.URL,
	})
	repo := Coordinates{Owner: "acme", Name: "goidea"}
	branches, err := client.Branches(context.Background(), repo)
	if err != nil || len(branches) != 2 {
		t.Fatalf("branches = %+v, %v", branches, err)
	}
	pr, err := client.CreatePullRequest(context.Background(), repo, CreatePullRequestInput{
		Title: "Change", Body: "Details", Base: "main", Head: "topic", Draft: true,
	})
	if err != nil || pr.Number != 1 || !pr.Draft || pr.ID != "PR_1" {
		t.Fatalf("created = %+v, %v", pr, err)
	}
	if err := client.UpdateBranch(context.Background(), repo, 1, "head"); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 3 {
		t.Fatalf("requests = %v", requests)
	}
}

func TestGitHubClientRedactsTokenAndHonorsCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{"message":"bad secret-token"}`)
	}))
	defer server.Close()
	client := NewClient(staticCredentials("secret-token"), ClientOptions{
		GraphQLEndpoint: server.URL, APIBase: server.URL,
	})
	var out map[string]any
	err := client.GraphQL(context.Background(), `query { viewer { login } }`, nil, &out)
	if err == nil || strings.Contains(err.Error(), "secret-token") ||
		!strings.Contains(err.Error(), "[redacted token]") {
		t.Fatalf("error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = client.GraphQL(ctx, `query { viewer { login } }`, nil, &out)
	if err == nil || (!errors.Is(err, context.Canceled) &&
		!strings.Contains(strings.ToLower(err.Error()), "canceled")) {
		t.Fatalf("canceled request error = %v", err)
	}
}

func TestGitHubClientRedactsGraphQLError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"errors":[{"message":"bad secret-token"}]}`)
	}))
	defer server.Close()
	client := NewClient(staticCredentials("secret-token"), ClientOptions{
		GraphQLEndpoint: server.URL, APIBase: server.URL,
	})
	var out map[string]any
	err := client.GraphQL(context.Background(), `query { viewer { login } }`, nil, &out)
	if err == nil || strings.Contains(err.Error(), "secret-token") ||
		!strings.Contains(err.Error(), "[redacted token]") {
		t.Fatalf("error = %v", err)
	}
}

func TestGitHubClientMissingTokenDoesNotCallHTTP(t *testing.T) {
	client := NewClient(staticCredentials(""), ClientOptions{
		GraphQLEndpoint: "http://127.0.0.1:1", APIBase: "http://127.0.0.1:1",
	})
	var out map[string]any
	err := client.GraphQL(context.Background(), `query { viewer { login } }`, nil, &out)
	if !errors.Is(err, ErrNoToken) {
		t.Fatalf("error = %v, want ErrNoToken", err)
	}
}
