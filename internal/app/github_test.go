package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	ghub "mayoi/internal/github"
)

type githubTestCredentials struct {
	mu     sync.Mutex
	token  string
	setErr error
	delErr error
	set    int
	delete int
}

func (c *githubTestCredentials) Token(context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token == "" {
		return "", ghub.ErrNoToken
	}
	return c.token, nil
}

func (c *githubTestCredentials) SetToken(token string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.set++
	if c.setErr != nil {
		return c.setErr
	}
	c.token = token
	return nil
}

func (c *githubTestCredentials) DeleteToken() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.delete++
	if c.delErr != nil {
		return c.delErr
	}
	c.token = ""
	return nil
}

func (c *githubTestCredentials) state() (string, int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.token, c.set, c.delete
}

func githubTestRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		command := append([]string{"-C", root}, args...)
		if out, err := exec.Command("git", command...).CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	run("init", "-q", "-b", "topic")
	run("remote", "add", "origin", "git@github.com:acme/goidea.git")
	return root
}

func githubTestModel(t *testing.T, root string, credentials githubTokenStore) (*Model, <-chan struct{}) {
	t.Helper()
	callbacks := make(chan struct{}, 16)
	m := &Model{ctx: context.Background()}
	m.setRoot(root)
	m.postHook = func(fn func()) {
		fn()
		callbacks <- struct{}{}
	}
	m.github = githubState{
		root: root, generation: 1, credentials: credentials,
		view: "list", filters: ghub.DefaultSearchFilters(), selectedFile: -1,
		review: newGitHubReviewState(),
	}
	return m, callbacks
}

func waitGitHubCallback(t *testing.T, callbacks <-chan struct{}) {
	t.Helper()
	select {
	case <-callbacks:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the GitHub main-thread callback")
	}
}

func testGitHubClient(server *httptest.Server, credentials ghub.Credentials) *ghub.Client {
	return ghub.NewClient(credentials, ghub.ClientOptions{
		GraphQLEndpoint: server.URL + "/graphql",
		APIBase:         server.URL,
	})
}

func githubRepositoryResponse() string {
	return `{"data":{"viewer":{"login":"me","name":"Me","avatarUrl":"https://avatar/me"},` +
		`"repository":{"id":"R_1","nameWithOwner":"acme/goidea","viewerPermission":"WRITE",` +
		`"mergeCommitAllowed":true,"squashMergeAllowed":true,"rebaseMergeAllowed":false,` +
		`"defaultBranchRef":{"name":"main"},"url":"https://github.com/acme/goidea"}}}`
}

func githubSearchResponse() string {
	return `{"data":{"search":{"issueCount":1,"pageInfo":{"hasNextPage":false,"endCursor":null},` +
		`"nodes":[{"__typename":"PullRequest","id":"PR_42",` +
		`"url":"https://github.com/acme/goidea/pull/42","number":42,` +
		`"title":"Improve review flow","state":"OPEN","isDraft":false,` +
		`"author":{"login":"octocat","avatarUrl":"https://avatar/octocat"},` +
		`"createdAt":"2026-11-01T08:00:00Z","updatedAt":"2026-11-02T08:00:00Z",` +
		`"isReadByViewer":false,"reviewDecision":"APPROVED","mergeable":"MERGEABLE",` +
		`"labels":{"nodes":[{"name":"review","color":"0e8a16"}]},` +
		`"assignees":{"nodes":[{"login":"me"}]},"comments":{"totalCount":3},` +
		`"reviewThreads":{"nodes":[{"isResolved":false}]},` +
		`"reviews":{"nodes":[]},` +
		`"commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"SUCCESS"}}}]}}]}}}`
}

func githubPullRequestResponse(title string) string {
	return `{"data":{"repository":{"pullRequest":{` +
		`"id":"PR_42","url":"https://github.com/acme/goidea/pull/42","number":42,` +
		`"title":` + strconvQuote(title) + `,"state":"OPEN","isDraft":false,"body":"Ready to merge",` +
		`"author":{"login":"octocat","avatarUrl":""},` +
		`"createdAt":"2026-11-01T08:00:00Z","updatedAt":"2026-11-02T08:00:00Z",` +
		`"isReadByViewer":true,"reviewDecision":"APPROVED","mergeable":"MERGEABLE",` +
		`"mergeStateStatus":"BEHIND","canBeRebased":false,"viewerCanUpdate":true,` +
		`"viewerDidAuthor":true,"viewerCanDeleteHeadRef":true,` +
		`"baseRefName":"main","baseRefOid":"base","baseRepository":{"url":"https://github.com/acme/goidea"},` +
		`"headRefName":"topic","headRefOid":"head","headRepository":{"url":"https://github.com/acme/goidea"},` +
		`"labels":{"nodes":[]},"assignees":{"nodes":[]},"comments":{"totalCount":3},` +
		`"reviewThreads":{"nodes":[]},` +
		`"reviews":{"nodes":[{"id":"REV_1","url":"https://review","author":{"login":"reviewer"},` +
		`"body":"Looks good","state":"APPROVED","createdAt":"2026-11-02T09:00:00Z"}]},` +
		`"files":{"nodes":[{"path":"src/Main.java","changeType":"MODIFIED","additions":4,` +
		`"deletions":2,"viewerViewedState":"UNVIEWED"}]},` +
		`"commits":{"nodes":[{"url":"https://commit","commit":{"oid":"head","abbreviatedOid":"head123",` +
		`"messageHeadline":"Improve review flow","committedDate":"2026-11-02T08:00:00Z",` +
		`"author":{"name":"Me","user":{"login":"me"}},` +
		`"statusCheckRollup":{"state":"FAILURE","contexts":{"nodes":[{"__typename":"CheckRun",` +
		`"name":"test","status":"COMPLETED","conclusion":"FAILURE","detailsUrl":"https://check",` +
		`"checkSuite":{"workflowRun":{"workflow":{"name":"CI"}}}}]}}}}]},` +
		`"timelineItems":{"nodes":[{"__typename":"PullRequestCommit","url":"https://commit",` +
		`"commit":{"oid":"head","abbreviatedOid":"head123","messageHeadline":"Improve review flow",` +
		`"committedDate":"2026-11-02T08:00:00Z","author":{"name":"Me","user":{"login":"me"}}}}]}` +
		`}}}}`
}

func strconvQuote(value string) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}

func TestGitHubActionsAndToolWindowAreRegistered(t *testing.T) {
	tests := []struct {
		id    string
		title string
	}{
		{"github.pullRequest.showInToolWindow", "Pull Requests"},
		{"github.refresh", "Refresh Pull Requests"},
		{"github.pullRequest.refresh", "Refresh Pull Request"},
		{"github.pullRequest.timeline.refresh", "Refresh Review Data"},
		{"github.pullRequest.diff.refreshReview", "Refresh Review Data"},
		{"github.pullRequest.diff.addComment", "Add Review Comment"},
		{"github.pullRequest.diff.addMultilineComment", "Add Multiline Review Comment"},
		{"github.pullRequest.diff.nextComment", "Next Comment"},
		{"github.pullRequest.diff.previousComment", "Previous Comment"},
		{"github.pullRequest.diff.resolveThread", "Resolve Review Thread"},
		{"github.pullRequest.diff.unresolveThread", "Reopen Review Thread"},
		{"github.pullRequest.reviewMode", "Review Mode"},
		{"github.pullRequest.show", "Show Current Branch Pull Request"},
		{"github.pullRequest.reviewMode.showDiff", "Show Review Diff"},
		{"github.pullRequest.submitReview", "Submit Review…"},
		{"github.pullRequest.editMetadata", "Edit Pull Request Fields…"},
		{"github.createPullRequest", "Create Pull Request…"},
		{"github.pullRequest.toggleViewed", "Toggle File Viewed"},
		{"github.pullRequest.updateBranch", "Update Branch"},
	}
	for _, test := range tests {
		action, ok := actionGet(test.id)
		if !ok || action.Title != test.title || action.Category != "GitHub" {
			t.Fatalf("%s = %#v, registered=%v", test.id, action, ok)
		}
	}
	spec := dockSpec("pullRequests")
	if spec == nil || spec.title != "Pull Requests" || spec.icon != "toolwindows/pullRequests" ||
		spec.anchor != dockLeft || !spec.stripe {
		t.Fatalf("Pull Requests tool window = %#v", spec)
	}
	foundMenu := false
	walkMenu(mainMenuGroups(), func(entry menuEntry) {
		if entry.label == "Pull Requests" &&
			entry.command == "github.pullRequest.showInToolWindow" {
			foundMenu = true
		}
	})
	if !foundMenu {
		t.Fatal("View › Tool Windows has no Pull Requests entry")
	}
}

func TestGitHubConnectLoadsRepositoryAndFirstSearchPage(t *testing.T) {
	var mu sync.Mutex
	var searchQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet &&
			r.URL.Path == "/repos/acme/goidea/pulls" {
			if r.URL.Query().Get("state") != "open" ||
				r.URL.Query().Get("head") != "acme:topic" {
				t.Errorf("branch query = %s", r.URL.RawQuery)
			}
			io.WriteString(w, `[{
				"number":42,"title":"Improve review flow",
				"html_url":"https://github.com/acme/goidea/pull/42"
			}]`)
			return
		}
		var request struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		switch {
		case strings.Contains(request.Query, "GoIDEARepository"):
			io.WriteString(w, githubRepositoryResponse())
		case strings.Contains(request.Query, "GoIDEASearchPullRequests"):
			mu.Lock()
			searchQuery, _ = request.Variables["query"].(string)
			mu.Unlock()
			io.WriteString(w, githubSearchResponse())
		default:
			http.Error(w, "unexpected query", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	root := githubTestRepo(t)
	credentials := &githubTestCredentials{token: "test-token"}
	m, callbacks := githubTestModel(t, root, credentials)
	m.github.client = testGitHubClient(server, credentials)
	m.connectGitHub(true)
	waitGitHubCallback(t, callbacks)

	if !m.github.connected() || m.github.repository.ID != "R_1" ||
		m.github.repository.DefaultBranch != "main" || m.github.viewer.Login != "me" {
		t.Fatalf("connection state = %+v repository=%+v viewer=%+v",
			m.github.coordinates, m.github.repository, m.github.viewer)
	}
	if m.github.connecting || m.github.listLoading || !m.github.listLoaded ||
		len(m.github.items) != 1 || m.github.listTotal != 1 {
		t.Fatalf("list state = %+v", m.github)
	}
	pr := m.github.items[0]
	if pr.Number != 42 || pr.Author != "octocat" || !pr.Unread ||
		pr.CommentCount != 3 || pr.UnresolvedThreads != 1 ||
		pr.CheckState != "SUCCESS" || m.github.currentBranch != "topic" ||
		m.github.review.branchNumber != 42 ||
		m.github.review.branchTitle != "Improve review flow" ||
		m.github.review.branchURL != "https://github.com/acme/goidea/pull/42" {
		t.Fatalf("pull request = %+v branch=%q", pr, m.github.currentBranch)
	}
	mu.Lock()
	gotSearchQuery := searchQuery
	mu.Unlock()
	if !strings.Contains(gotSearchQuery, "type:pr repo:acme/goidea") ||
		!strings.Contains(gotSearchQuery, "is:open") {
		t.Fatalf("search query = %q", gotSearchQuery)
	}
}

func TestGitHubStaleProjectResultIsIgnored(t *testing.T) {
	requestStarted := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Query string `json:"query"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if strings.Contains(request.Query, "GoIDEARepository") {
			select {
			case <-requestStarted:
			default:
				close(requestStarted)
			}
			<-release
			io.WriteString(w, githubRepositoryResponse())
			return
		}
		io.WriteString(w, githubSearchResponse())
	}))
	defer server.Close()

	root := githubTestRepo(t)
	credentials := &githubTestCredentials{token: "test-token"}
	m, callbacks := githubTestModel(t, root, credentials)
	m.github.client = testGitHubClient(server, credentials)
	m.connectGitHub(true)
	select {
	case <-requestStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("GitHub repository request did not start")
	}
	m.setRoot(t.TempDir())
	m.ensureGitHubState()
	close(release)
	waitGitHubCallback(t, callbacks)

	if m.github.connected() || m.github.repository.ID != "" || len(m.github.items) != 0 {
		t.Fatalf("stale project result was applied: %+v", m.github)
	}
	if m.github.root != m.Root() || m.github.generation < 2 {
		t.Fatalf("project generation = %d root=%q model root=%q",
			m.github.generation, m.github.root, m.Root())
	}
}

func TestGitHubTokenSaveClearsDraftAndReconnects(t *testing.T) {
	var mu sync.Mutex
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		authorization = r.Header.Get("Authorization")
		mu.Unlock()
		var request struct {
			Query string `json:"query"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if strings.Contains(request.Query, "GoIDEARepository") {
			io.WriteString(w, githubRepositoryResponse())
			return
		}
		io.WriteString(w, githubSearchResponse())
	}))
	defer server.Close()

	root := githubTestRepo(t)
	credentials := &githubTestCredentials{token: "old-token"}
	m, callbacks := githubTestModel(t, root, credentials)
	oldClient := testGitHubClient(server, credentials)
	m.github.client = oldClient
	m.github.coordinates = ghub.Coordinates{Owner: "old", Name: "repo"}
	m.github.repository = ghub.Repository{ID: "OLD"}
	m.github.tokenDraft = "new-token"
	m.github.needsToken = true
	m.SaveGitHubToken()
	waitGitHubCallback(t, callbacks)
	waitGitHubCallback(t, callbacks)

	token, setCalls, deleteCalls := credentials.state()
	if token != "new-token" || setCalls != 1 || m.github.tokenDraft != "" ||
		m.github.needsToken || !m.github.connected() ||
		m.github.repository.ID != "R_1" || m.github.client != oldClient {
		t.Fatalf("saved token state: token=%q set=%d github=%+v", token, setCalls, m.github)
	}
	mu.Lock()
	gotAuthorization := authorization
	mu.Unlock()
	if gotAuthorization != "Bearer new-token" {
		t.Fatalf("authorization = %q", gotAuthorization)
	}

	generation := m.github.generation
	m.ClearGitHubToken()
	waitGitHubCallback(t, callbacks)
	token, _, deleteCalls = credentials.state()
	if token != "" || deleteCalls != 1 || !m.github.needsToken || m.github.connected() ||
		m.github.repository.ID != "" || m.github.items != nil ||
		m.github.generation <= generation || m.github.selectedFile != -1 {
		t.Fatalf("cleared token state: token=%q delete=%d github=%+v",
			token, deleteCalls, m.github)
	}
}

func TestGitHubCreateViewedAndUpdateBranchFlows(t *testing.T) {
	var mu sync.Mutex
	var createBody map[string]any
	var mutations []string
	var updatePath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/repos/acme/goidea/pulls":
			mu.Lock()
			_ = json.NewDecoder(r.Body).Decode(&createBody)
			mu.Unlock()
			io.WriteString(w, `{"id":11,"node_id":"PR_42","html_url":"https://github.com/acme/goidea/pull/42",`+
				`"number":42,"title":"Improve review flow","state":"open","draft":false,`+
				`"created_at":"2026-11-01T08:00:00Z","updated_at":"2026-11-02T08:00:00Z",`+
				`"user":{"login":"me","avatar_url":"https://avatar/me"}}`)
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/update-branch"):
			mu.Lock()
			updatePath = r.URL.Path
			mu.Unlock()
			w.WriteHeader(http.StatusAccepted)
			io.WriteString(w, `{}`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/files"):
			io.WriteString(w, `[]`)
		case r.Method == http.MethodPost && r.URL.Path == "/graphql":
			var request struct {
				Query string `json:"query"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			switch {
			case strings.Contains(request.Query, "GoIDEAPullRequest"):
				io.WriteString(w, githubPullRequestResponse("Improve review flow"))
			case strings.Contains(request.Query, "GoIDEAMarkFileViewed"):
				mu.Lock()
				mutations = append(mutations, "mark")
				mu.Unlock()
				io.WriteString(w, `{"data":{"markFileAsViewed":{"clientMutationId":null}}}`)
			case strings.Contains(request.Query, "GoIDEAUnmarkFileViewed"):
				mu.Lock()
				mutations = append(mutations, "unmark")
				mu.Unlock()
				io.WriteString(w, `{"data":{"unmarkFileAsViewed":{"clientMutationId":null}}}`)
			case strings.Contains(request.Query, "GoIDEAReviewThreads"):
				io.WriteString(w, `{"data":{"repository":{"pullRequest":{"reviewThreads":{
					"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[]}}}}}`)
			case strings.Contains(request.Query, "GoIDEAPendingReview"):
				io.WriteString(w, `{"data":{"node":{"reviews":{"nodes":[]}}}}`)
			default:
				http.Error(w, "unexpected query", http.StatusBadRequest)
			}
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	root := githubTestRepo(t)
	credentials := &githubTestCredentials{token: "test-token"}
	m, callbacks := githubTestModel(t, root, credentials)
	m.github.coordinates = ghub.Coordinates{Owner: "acme", Name: "goidea"}
	m.github.repository = ghub.Repository{
		ID: "R_1", Coordinates: m.github.coordinates, DefaultBranch: "main",
	}
	m.github.viewer = ghub.Viewer{Login: "me"}
	m.github.client = testGitHubClient(server, credentials)
	m.github.create = githubCreateState{
		title: "Improve review flow", body: "Ready to merge", base: "main", head: "topic",
	}
	m.CreateGitHubPullRequest()
	waitGitHubCallback(t, callbacks)
	waitGitHubCallback(t, callbacks)
	waitGitHubCallback(t, callbacks)

	mu.Lock()
	created := createBody
	mu.Unlock()
	if created["title"] != "Improve review flow" || created["base"] != "main" ||
		created["head"] != "topic" || m.github.view != "detail" ||
		m.github.selectedNumber != 42 || m.github.detail == nil ||
		m.github.notice != "Created pull request #42" {
		t.Fatalf("create body=%#v detail=%+v", created, m.github.detail)
	}

	m.SelectGitHubFile(0)
	m.ToggleGitHubFileViewed()
	waitGitHubCallback(t, callbacks)
	if m.github.detail == nil || !m.github.detail.Files[0].ViewerViewed ||
		m.github.detail.Files[0].ViewedState != "VIEWED" {
		t.Fatalf("viewed file = %+v", m.github.detail)
	}

	m.UpdateGitHubBranch()
	waitGitHubCallback(t, callbacks)
	waitGitHubCallback(t, callbacks)
	waitGitHubCallback(t, callbacks)
	mu.Lock()
	gotMutations := append([]string(nil), mutations...)
	gotUpdatePath := updatePath
	mu.Unlock()
	if len(gotMutations) != 1 || gotMutations[0] != "mark" ||
		gotUpdatePath != "/repos/acme/goidea/pulls/42/update-branch" ||
		m.github.notice != "Updated pull request #42" {
		t.Fatalf("mutations=%v update=%q notice=%q",
			gotMutations, gotUpdatePath, m.github.notice)
	}
}

func TestGitHubStaleViewedResultDoesNotModifyNewerPullRequest(t *testing.T) {
	requestStarted := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Query string `json:"query"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if !strings.Contains(request.Query, "GoIDEAMarkFileViewed") {
			http.Error(w, "unexpected query", http.StatusBadRequest)
			return
		}
		close(requestStarted)
		<-release
		io.WriteString(w, `{"data":{"markFileAsViewed":{"clientMutationId":null}}}`)
	}))
	defer server.Close()

	root := githubTestRepo(t)
	credentials := &githubTestCredentials{token: "test-token"}
	m, callbacks := githubTestModel(t, root, credentials)
	m.github.coordinates = ghub.Coordinates{Owner: "acme", Name: "goidea"}
	m.github.repository = ghub.Repository{ID: "R_1", Coordinates: m.github.coordinates}
	m.github.client = testGitHubClient(server, credentials)
	m.github.selectedNumber = 42
	m.github.selectedFile = 0
	m.github.detail = &ghub.PullRequestDetail{
		PullRequestSummary: ghub.PullRequestSummary{ID: "PR_42", Number: 42},
		Files: []ghub.FileChange{{
			Path: "src/Main.java", ViewerViewed: false, ViewedState: "UNVIEWED",
		}},
	}
	m.ToggleGitHubFileViewed()
	select {
	case <-requestStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("file viewed mutation did not start")
	}
	m.github.selectedNumber = 43
	m.github.detail = &ghub.PullRequestDetail{
		PullRequestSummary: ghub.PullRequestSummary{ID: "PR_43", Number: 43},
		Files: []ghub.FileChange{{
			Path: "src/Main.java", ViewerViewed: false, ViewedState: "UNVIEWED",
		}},
	}
	close(release)
	waitGitHubCallback(t, callbacks)

	if m.github.detail == nil || m.github.detail.Number != 43 ||
		m.github.detail.ID != "PR_43" || m.github.detail.Files[0].ViewerViewed ||
		m.github.detail.Files[0].ViewedState != "UNVIEWED" || m.github.actionErr != "" {
		t.Fatalf("stale file result changed pull request #43: %+v", m.github.detail)
	}
}

func TestGitHubToolWindowRendersNativeStatesOffscreen(t *testing.T) {
	root := t.TempDir()
	m := &Model{ctx: context.Background()}
	m.setRoot(root)
	m.github = githubState{
		visible: true, root: root, generation: 1,
		coordinates: ghub.Coordinates{Owner: "acme", Name: "goidea"},
		repository:  ghub.Repository{ID: "R_1", DefaultBranch: "main"},
		viewer:      ghub.Viewer{Login: "me"},
		view:        "list",
		filters:     ghub.DefaultSearchFilters(),
		listLoaded:  true,
		listTotal:   1,
		items: []ghub.PullRequestSummary{{
			ID: "PR_42", Number: 42, Title: "Improve review flow",
			State: "OPEN", Author: "octocat", ReviewDecision: "APPROVED",
			CheckState: "FAILURE", CommentCount: 3, UnresolvedThreads: 1, Unread: true,
		}},
		selectedFile: -1,
		review:       newGitHubReviewState(),
	}
	render := func() *ui.Tester {
		return ui.NewTester(func(c *ui.Context) {
			m.githubPanel(c, c.Theme(), oiPaletteOf(c.Theme()))
		}, 430, 900)
	}

	list := render()
	list.Frame()
	for _, text := range []string{
		"Pull Requests", "Filters", "Open", "#42",
		"Improve review flow", "octocat", "approved", "Checks failed",
		"3 comments", "1 unresolved",
	} {
		if !list.HasText(text) {
			t.Fatalf("%q is missing from the pull request list: %q", text, list.Texts())
		}
	}

	m.github.view = "detail"
	m.github.selectedNumber = 42
	detailSummary := m.github.items[0]
	detailSummary.Mergeable = "MERGEABLE"
	detailSummary.Reviews = []ghub.Review{{
		Author: "reviewer", Body: "Looks good", State: "APPROVED",
	}}
	m.github.detail = &ghub.PullRequestDetail{
		PullRequestSummary: detailSummary,
		Body:               "Ready to merge",
		BaseRef:            "main",
		HeadRef:            "topic",
		MergeStateStatus:   "BEHIND",
		Files: []ghub.FileChange{{
			Path: "src/Main.java", ChangeType: "MODIFIED", Additions: 4, Deletions: 2,
		}},
		Commits: []ghub.Commit{{
			ShortOID: "head123", Headline: "Improve review flow", Author: "me",
		}},
		Checks: []ghub.Check{{
			Name: "test", Workflow: "CI", Conclusion: "FAILURE", URL: "https://check",
		}},
		Timeline: []ghub.TimelineItem{{
			Kind: "comment", Actor: "reviewer", Body: "Ready to merge",
			CreatedAt: "2026-11-02T08:00:00Z",
		}},
	}
	detail := render()
	detail.Frame()
	for _, text := range []string{
		"Pull Request #42", "Update Branch", "Checks", "CI / test",
		"Files (1)", "src/Main.java", "Commits (1)", "Reviews (1)",
		"Looks good", "Activity", "@reviewer commented",
	} {
		if !detail.HasText(text) {
			t.Fatalf("%q is missing from the pull request detail: %q", text, detail.Texts())
		}
	}

	m.github.view = "create"
	m.github.create = githubCreateState{
		base: "main", head: "topic",
		branches: []ghub.Branch{{Name: "main"}, {Name: "topic"}},
	}
	create := render()
	create.Frame()
	for _, text := range []string{
		"Create Pull Request", "New Pull Request", "Base branch",
		"Head branch", "Create as draft", "main", "topic",
	} {
		if !create.HasText(text) {
			t.Fatalf("%q is missing from the create form: %q", text, create.Texts())
		}
	}

	m.github.needsToken = true
	m.github.tokenErr = "GitHub rejected this token. Replace it to continue."
	token := render()
	token.Frame()
	for _, text := range []string{
		"Connect to GitHub", "Save Token", "Create a Token…",
		"GitHub rejected this token. Replace it to continue.",
	} {
		if !token.HasText(text) {
			t.Fatalf("%q is missing from the token form: %q", text, token.Texts())
		}
	}
}
