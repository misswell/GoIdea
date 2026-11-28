package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	ghub "mayoi/internal/github"
)

func githubReviewTestModel(t *testing.T) *Model {
	t.Helper()
	m, _ := githubReviewTestModelWithCallbacks(t)
	return m
}

func githubReviewTestModelWithCallbacks(t *testing.T) (*Model, <-chan struct{}) {
	t.Helper()
	root := t.TempDir()
	credentials := &githubTestCredentials{token: "test-token"}
	m, callbacks := githubTestModel(t, root, credentials)
	m.github.coordinates = ghub.Coordinates{Owner: "acme", Name: "goidea"}
	m.github.repository = ghub.Repository{
		ID: "R_1", Coordinates: m.github.coordinates, DefaultBranch: "main",
	}
	m.github.viewer = ghub.Viewer{Login: "me"}
	m.github.currentBranch = "topic"
	m.github.view = "detail"
	m.github.selectedNumber = 42
	m.github.listLoaded = true
	return m, callbacks
}

func githubReviewTestDetail() *ghub.PullRequestDetail {
	return &ghub.PullRequestDetail{
		PullRequestSummary: ghub.PullRequestSummary{
			ID: "PR_42", Number: 42, Title: "Improve review flow",
			State: "OPEN", ReviewDecision: "REVIEW_REQUIRED",
			CommentCount: 3, UnresolvedThreads: 2,
		},
		Body:            "Ready to review",
		BaseRef:         "main",
		HeadRef:         "topic",
		HeadOID:         "head-oid",
		ViewerCanUpdate: true,
		ViewerDidAuthor: false,
		Files: []ghub.FileChange{{
			Path:       "src/Main.java",
			ChangeType: "MODIFIED",
			Additions:  2,
			Deletions:  1,
			Patch:      "@@ -1,2 +1,3 @@\n context\n-old\n+new\n+more",
		}},
		Threads: []ghub.ReviewThread{{
			ID: "THREAD_1", Path: "src/Main.java", DiffSide: "RIGHT",
			Line: 3, StartDiffSide: "RIGHT", StartLine: 3,
			ViewerCanReply: true, ViewerCanResolve: true,
			Comments: []ghub.ReviewComment{{
				ID: "COMMENT_1", Author: "reviewer",
				Body: "Use the new API", CreatedAt: "2026-11-02T08:00:00Z",
			}},
		}},
	}
}

func TestGitHubReviewBranchUsesBackendMergeBase(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		updateRequired bool
	}{
		{"head in local history", `"ABCDEF"`, false},
		{"older merge base", `"aaa111"`, true},
		{"missing head", `null`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, callbacks := githubReviewTestModelWithCallbacks(t)
			m.github.review.branchNumber = 42
			m.invokeHook = func(_ context.Context, method string, args any) (json.RawMessage, error) {
				params, ok := args.(map[string]any)
				if method != "vcs_merge_base" || !ok || params["a"] != "abcdef" || params["b"] != "HEAD" {
					t.Errorf("merge base request = %s %+v", method, args)
				}
				return json.RawMessage(tc.response), nil
			}
			m.updateGitHubReviewBranchSync(m.Root(), m.github.generation, 42, "abcdef")
			waitGitHubCallback(t, callbacks)
			if m.github.review.branchSyncLoading || m.github.review.branchUpdateRequired != tc.updateRequired {
				t.Fatalf("branch update required = %v, loading = %v", m.github.review.branchUpdateRequired, m.github.review.branchSyncLoading)
			}
		})
	}
}

func TestGitHubDiffRowRangeRequiresContiguousLinesOnSameSide(t *testing.T) {
	rows := parseGitHubPatch("@@ -1,2 +1,3 @@\n context\n-old\n+new\n+more\n@@ -9 +10 @@\n+later")
	start, end, ok := githubDiffRowRange(rows, 4, 3, true)
	if !ok || rows[start].CommentLine != 2 || rows[end].CommentLine != 3 {
		t.Fatalf("reverse selection: start=%d end=%d ok=%v", start, end, ok)
	}
	if _, _, ok := githubDiffRowRange(rows, 2, 3, true); ok {
		t.Fatal("accepted a range spanning different sides")
	}
	if _, _, ok := githubDiffRowRange(rows, 3, 6, true); ok {
		t.Fatal("accepted a range spanning hunks")
	}
}

func TestGitHubReviewSurfacesRenderOffscreen(t *testing.T) {
	m := githubReviewTestModel(t)
	m.github.detail = githubReviewTestDetail()
	m.github.selectedFile = 0
	m.github.review.branch = "topic"
	m.github.review.branchNumber = 42
	m.github.review.branchTitle = "Improve review flow"
	m.github.review.enabled = true
	m.github.review.mode = "review"
	m.github.review.threads = append([]ghub.ReviewThread(nil), m.github.detail.Threads...)
	m.github.review.threadsLoaded = true
	m.github.review.selectedThread = "THREAD_1"
	m.github.review.pending = ghub.PendingReview{
		ID: "REV_PENDING", State: "PENDING", CommentsCount: 2,
	}
	for index, row := range parseGitHubPatch(m.github.detail.Files[0].Patch) {
		if row.Kind == "add" {
			m.github.review.activeRow = index
			break
		}
	}

	render := func(width, height int, body func(*ui.Context, *ui.Theme, oiColors)) *ui.Tester {
		t.Helper()
		return ui.NewTester(func(c *ui.Context) {
			body(c, c.Theme(), oiPaletteOf(c.Theme()))
		}, width, height)
	}

	toolbar := render(980, 360, m.githubReviewToolbar)
	toolbar.Frame()
	for _, text := range []string{
		"Review Mode On", "#42 on topic", "Review diff", "Normal diff",
		"1 thread · 1 unresolved", "Previous", "Next", "Submit Review (2)…",
		"Edit fields…", "Selected line RIGHT:2", "Comment", "Multiline",
	} {
		if !toolbar.HasText(text) {
			t.Fatalf("%q is missing from the review toolbar: %q", text, toolbar.Texts())
		}
	}

	m.github.review.mode = "normal"
	normalToolbar := render(980, 360, m.githubReviewToolbar)
	normalToolbar.Frame()
	if normalToolbar.HasText("Multiline") || !normalToolbar.HasText("Normal diff") {
		t.Fatalf("normal toolbar = %q", normalToolbar.Texts())
	}

	m.github.review.mode = "review"
	m.github.review.comment = githubReviewCommentDraft{
		open: true, path: "src/Main.java", side: "RIGHT", startSide: "RIGHT",
		startLine: 2, line: 3, body: "Please cover this branch",
	}
	diff := render(980, 720, m.githubSelectedFileDiff)
	diff.Frame()
	for _, text := range []string{
		"src/Main.java", "Review comment · src/Main.java:2-3",
		"Add to Pending Review", "old", "new", "more",
	} {
		if !diff.HasText(text) {
			t.Fatalf("%q is missing from the review diff: %q", text, diff.Texts())
		}
	}
	m.github.review.mode = "normal"
	normalDiff := render(980, 720, m.githubSelectedFileDiff)
	normalDiff.Frame()
	if normalDiff.HasText("Start Review") || !normalDiff.HasText("new") {
		t.Fatalf("normal diff = %q", normalDiff.Texts())
	}

	m.github.review.mode = "review"
	threads := render(760, 500, m.githubThreadsSection)
	threads.Frame()
	for _, text := range []string{
		"Review Threads (1)", "src/Main.java:3", "@reviewer",
		"Use the new API", "Resolve",
	} {
		if !threads.HasText(text) {
			t.Fatalf("%q is missing from review threads: %q", text, threads.Texts())
		}
	}

	m.github.review.submit = githubReviewSubmitState{
		open: true, body: "One issue remains",
	}
	submit := render(760, 360, m.githubReviewSubmitForm)
	submit.Frame()
	for _, text := range []string{
		"Submit Review", "2 pending inline comments", "Approve",
		"Request Changes", "Comment", "Cancel",
	} {
		if !submit.HasText(text) {
			t.Fatalf("%q is missing from the review form: %q", text, submit.Texts())
		}
	}

	m.github.review.metadata = githubMetadataState{
		editing: true, title: "Improve review flow", body: "Ready to review", base: "main",
	}
	m.github.create.branches = []ghub.Branch{{Name: "main"}, {Name: "release"}}
	metadata := render(760, 520, m.githubMetadataEditor)
	metadata.Frame()
	for _, text := range []string{
		"Edit Pull Request", "Title", "Description", "Base branch",
		"main", "Save", "Cancel",
	} {
		if !metadata.HasText(text) {
			t.Fatalf("%q is missing from metadata editor: %q", text, metadata.Texts())
		}
	}

	m.github.review.timelineComment.body = "I checked the failing test"
	timeline := render(760, 260, m.githubTimelineComposer)
	timeline.Frame()
	for _, text := range []string{
		"Add Timeline Comment", "Comment",
	} {
		if !timeline.HasText(text) {
			t.Fatalf("%q is missing from timeline composer: %q", text, timeline.Texts())
		}
	}
}

func TestGitHubPatchLinesMultilineDraftsAndThreadNavigation(t *testing.T) {
	patch := "@@ -10,3 +20,4 @@\n context\n-old\n+new\n+extra\n" +
		"\\ No newline at end of file"
	rows := parseGitHubPatch(patch)
	if len(rows) != 5 {
		t.Fatalf("rows = %+v", rows)
	}
	if rows[0].Kind != "hunk" ||
		rows[1].OldLine != 10 || rows[1].NewLine != 20 ||
		rows[1].CommentSide != "RIGHT" || rows[1].CommentLine != 20 ||
		rows[2].Kind != "remove" || rows[2].OldLine != 11 ||
		rows[2].CommentSide != "LEFT" || rows[2].CommentLine != 11 ||
		rows[3].Kind != "add" || rows[3].NewLine != 21 ||
		rows[4].NewLine != 22 {
		t.Fatalf("parsed rows = %+v", rows)
	}

	m := githubReviewTestModel(t)
	m.github.detail = &ghub.PullRequestDetail{
		PullRequestSummary: ghub.PullRequestSummary{ID: "PR_42", Number: 42},
		Files: []ghub.FileChange{
			{Path: "src/Second.java", Patch: patch},
			{Path: "src/First.java", Patch: patch},
		},
	}
	m.github.selectedFile = 0
	m.SelectGitHubDiffRow(3)
	m.ExtendGitHubDiffSelection(4)
	m.BeginGitHubReviewComment(true)
	if !m.github.review.comment.open ||
		m.github.review.comment.path != "src/Second.java" ||
		m.github.review.comment.startLine != 21 ||
		m.github.review.comment.line != 22 ||
		m.github.review.comment.side != "RIGHT" {
		t.Fatalf("multiline draft = %+v", m.github.review.comment)
	}

	m.github.review.threads = []ghub.ReviewThread{
		{ID: "THREAD_3", Path: "src/Second.java", DiffSide: "RIGHT", Line: 22},
		{ID: "THREAD_1", Path: "src/First.java", DiffSide: "RIGHT", Line: 21},
		{ID: "THREAD_2", Path: "src/Second.java", DiffSide: "LEFT", Line: 11},
	}
	order := githubThreadOrder(m.github.detail.Files, m.github.review.threads)
	if len(order) != 3 || order[0] != 2 || order[1] != 0 || order[2] != 1 {
		t.Fatalf("thread order = %v", order)
	}
	m.StepGitHubReviewComment(1)
	if m.github.review.selectedThread != "THREAD_2" ||
		m.github.selectedFile != 0 || !m.githubThreadSelected() {
		t.Fatalf("first selection = thread %q, file %d",
			m.github.review.selectedThread, m.github.selectedFile)
	}
	m.StepGitHubReviewComment(1)
	if m.github.review.selectedThread != "THREAD_3" ||
		m.github.selectedFile != 0 {
		t.Fatalf("second selection = thread %q, file %d",
			m.github.review.selectedThread, m.github.selectedFile)
	}
	m.StepGitHubReviewComment(1)
	if m.github.review.selectedThread != "THREAD_1" ||
		m.github.selectedFile != 1 {
		t.Fatalf("third selection = %q", m.github.review.selectedThread)
	}
	m.StepGitHubReviewComment(1)
	if m.github.review.selectedThread != "THREAD_1" {
		t.Fatalf("navigation passed the last thread: %q", m.github.review.selectedThread)
	}

	m.SelectGitHubFile(1)
	if m.github.selectedFile != 1 || m.github.review.activeRow != -1 {
		t.Fatalf("file selection = file %d, row %d",
			m.github.selectedFile, m.github.review.activeRow)
	}
}

func TestGitHubFileReviewThreadOmitsAnEmptyLineNumber(t *testing.T) {
	m := githubReviewTestModel(t)
	m.github.review.threads = []ghub.ReviewThread{{
		ID: "THREAD_FILE", Path: "src/Main.java", SubjectType: "FILE",
		ViewerCanResolve: true,
	}}
	card := ui.NewTester(func(c *ui.Context) {
		m.githubReviewThreadCard(c, c.Theme(), oiPaletteOf(c.Theme()), 0, false)
	}, 520, 180)
	card.Frame()
	if !card.HasText("src/Main.java") || card.HasText("src/Main.java:0") {
		t.Fatalf("file thread card = %q", card.Texts())
	}
}

func TestGitHubPullRequestsDockMovesThroughTheLeftPrimaryArea(t *testing.T) {
	m := githubReviewTestModel(t)
	m.github.view = "list"
	m.github.listTotal = 1
	m.github.items = []ghub.PullRequestSummary{{
		ID: "PR_42", Number: 42, Title: "Improve review flow", State: "OPEN",
	}}

	renderLeft := func() *ui.Tester {
		return ui.NewTester(func(c *ui.Context) {
			m.leftSide(c, c.Theme(), oiPaletteOf(c.Theme()))
		}, 820, 720)
	}

	m.DockShow("pullRequests")
	if got := m.dockOccupant(dockLeft); got != "pullRequests" ||
		m.leftHidden || !m.github.visible {
		t.Fatalf("shown dock = %q, leftHidden=%v, visible=%v",
			got, m.leftHidden, m.github.visible)
	}
	shown := renderLeft()
	shown.Frame()
	if !shown.HasText("Pull Requests") || !shown.HasText("#42") {
		t.Fatalf("left dock = %q", shown.Texts())
	}

	m.DockHide("pullRequests")
	if m.dockOccupant(dockLeft) != "" || m.github.visible {
		t.Fatalf("hidden dock = %q, visible=%v", m.dockOccupant(dockLeft), m.github.visible)
	}
	hidden := renderLeft()
	hidden.Frame()
	if hidden.HasText("Pull Requests") {
		t.Fatalf("hidden dock still rendered: %q", hidden.Texts())
	}

	m.DockShow("pullRequests")
	m.DockMove("pullRequests", dockRight)
	if m.dockOccupant(dockLeft) != "" || m.dockOccupant(dockRight) != "pullRequests" ||
		!m.github.visible {
		t.Fatalf("moved dock left=%q right=%q visible=%v",
			m.dockOccupant(dockLeft), m.dockOccupant(dockRight), m.github.visible)
	}
	movedLeft := renderLeft()
	movedLeft.Frame()
	if movedLeft.HasText("Pull Requests") {
		t.Fatalf("right-docked Pull Requests rendered in the left area: %q", movedLeft.Texts())
	}

	m.DockMove("pullRequests", dockLeft)
	m.leftHidden = true
	hiddenAgain := renderLeft()
	hiddenAgain.Frame()
	if hiddenAgain.HasText("Pull Requests") {
		t.Fatalf("hidden primary area still rendered: %q", hiddenAgain.Texts())
	}
	m.leftHidden = false
	shownAgain := renderLeft()
	shownAgain.Frame()
	if !shownAgain.HasText("Pull Requests") {
		t.Fatalf("shown primary area did not return: %q", shownAgain.Texts())
	}
}

func TestGitHubDetailRefreshKeepsTheSelectedFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/files"):
			io.WriteString(w, `[{"filename":"src/Main.java","status":"modified",
				"additions":2,"deletions":1,
				"patch":"@@ -1 +1,2 @@\n-old\n+new\n+more"}]`)
		case r.URL.Path == "/graphql":
			var request struct {
				Query string `json:"query"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			switch {
			case strings.Contains(request.Query, "GoIDEAPullRequest"):
				io.WriteString(w, githubPullRequestResponse("Improved title"))
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

	m, callbacks := githubReviewTestModelWithCallbacks(t)
	m.github.client = testGitHubClient(server, m.github.credentials)
	m.github.detail = githubReviewTestDetail()
	m.github.selectedFile = 0
	m.loadGitHubDetail()
	waitGitHubCallback(t, callbacks)
	if m.github.detail == nil || m.github.selectedNumber != 42 ||
		m.github.selectedFile != 0 {
		t.Fatalf("detail refresh selection = %+v", m.github.detail)
	}
	waitGitHubCallback(t, callbacks)
	if m.github.detail == nil || m.github.detail.Title != "Improved title" ||
		m.github.selectedFile != 0 ||
		m.github.detail.Files[0].Path != "src/Main.java" ||
		!strings.Contains(m.github.detail.Files[0].Patch, "+more") {
		t.Fatalf("refreshed detail = %+v, selected=%d",
			m.github.detail, m.github.selectedFile)
	}
}

func TestGitHubReviewRefreshIgnoresAStalePullRequest(t *testing.T) {
	requestStarted := make(chan struct{})
	release := make(chan struct{})
	var startOnce sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/files"):
			startOnce.Do(func() { close(requestStarted) })
			<-release
			http.Error(w, "stale response", http.StatusInternalServerError)
		case r.URL.Path == "/graphql":
			var request struct {
				Query string `json:"query"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			switch {
			case strings.Contains(request.Query, "GoIDEAReviewThreads"):
				io.WriteString(w, `{"data":{"repository":{"pullRequest":{"reviewThreads":{
					"pageInfo":{"hasNextPage":false,"endCursor":null},
					"nodes":[{"id":"STALE_THREAD","path":"src/Main.java",
						"diffSide":"RIGHT","line":1,"comments":{"nodes":[]}}]
				}}}}}`)
			case strings.Contains(request.Query, "GoIDEAPendingReview"):
				io.WriteString(w, `{"data":{"node":{"reviews":{"nodes":[
					{"id":"STALE_REVIEW","state":"PENDING","comments":{"totalCount":1}}
				]}}}}`)
			default:
				http.Error(w, "unexpected query", http.StatusBadRequest)
			}
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	m, callbacks := githubReviewTestModelWithCallbacks(t)
	m.github.client = testGitHubClient(server, m.github.credentials)
	m.github.detail = githubReviewTestDetail()
	m.refreshGitHubReviewData()
	select {
	case <-requestStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("review data request did not start")
	}
	m.github.selectedNumber = 43
	m.github.detail = &ghub.PullRequestDetail{
		PullRequestSummary: ghub.PullRequestSummary{ID: "PR_43", Number: 43},
	}
	resetGitHubReviewData(&m.github.review)
	close(release)
	waitGitHubCallback(t, callbacks)

	if m.github.detail.ID != "PR_43" || m.github.review.diffLoaded ||
		m.github.review.threadsLoaded || len(m.github.review.threads) != 0 ||
		m.github.review.diffLoading || m.github.review.threadsLoading ||
		m.github.review.pendingLoading ||
		m.github.review.pending.ID != "" || m.github.review.diffErr != "" ||
		m.github.review.threadsErr != "" || m.github.review.pendingErr != "" {
		t.Fatalf("stale review data changed pull request #43: detail=%+v review=%+v",
			m.github.detail, m.github.review)
	}
}

func TestGitHubReviewMutationsIgnoreAStalePullRequest(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		setup    func(*Model)
		invoke   func(*Model)
		response string
	}{
		{
			name:  "comment",
			query: "GoIDEACreateReview",
			setup: func(m *Model) {
				m.github.review.comment = githubReviewCommentDraft{
					open: true, path: "src/Main.java", side: "RIGHT",
					startSide: "RIGHT", startLine: 2, line: 3, body: "Comment",
				}
			},
			invoke: func(m *Model) { m.SubmitGitHubReviewComment(false) },
			response: `{"data":{"addPullRequestReview":{"pullRequestReview":{
				"id":"REV_STALE","state":"PENDING","comments":{"totalCount":1}}}}}`,
		},
		{
			name:  "review submission",
			query: "GoIDEACreateReview",
			setup: func(m *Model) {
				m.github.review.submit = githubReviewSubmitState{
					open: true, body: "Looks good",
				}
			},
			invoke: func(m *Model) { m.SubmitGitHubReview("COMMENT") },
			response: `{"data":{"addPullRequestReview":{"pullRequestReview":{
				"id":"REV_STALE","state":"COMMENTED","comments":{"totalCount":0}}}}}`,
		},
		{
			name:  "metadata",
			query: "GoIDEAUpdatePullRequest",
			setup: func(m *Model) {
				m.github.review.metadata = githubMetadataState{
					editing: true, title: "Changed title",
					body: "Changed body", base: "main",
				}
			},
			invoke: func(m *Model) { m.SaveGitHubMetadata() },
			response: `{"data":{"updatePullRequest":{"pullRequest":{
				"id":"PR_42","title":"Changed title","body":"Changed body"}}}}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requestStarted := make(chan struct{})
			release := make(chan struct{})
			var startOnce sync.Once
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Query string `json:"query"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				if !strings.Contains(request.Query, test.query) {
					http.Error(w, "unexpected query", http.StatusBadRequest)
					return
				}
				startOnce.Do(func() { close(requestStarted) })
				<-release
				io.WriteString(w, test.response)
			}))
			defer server.Close()

			m, callbacks := githubReviewTestModelWithCallbacks(t)
			m.github.client = testGitHubClient(server, m.github.credentials)
			m.github.detail = githubReviewTestDetail()
			test.setup(m)
			test.invoke(m)
			select {
			case <-requestStarted:
			case <-time.After(3 * time.Second):
				t.Fatal("GitHub mutation did not start")
			}
			m.github.selectedNumber = 43
			m.github.detail = &ghub.PullRequestDetail{
				PullRequestSummary: ghub.PullRequestSummary{
					ID: "PR_43", Number: 43, Title: "Other pull request",
				},
				ViewerCanUpdate: true,
			}
			m.github.notice = ""
			resetGitHubReviewData(&m.github.review)
			close(release)
			waitGitHubCallback(t, callbacks)

			if m.github.detail.ID != "PR_43" || m.github.notice != "" ||
				m.github.review.comment.open || m.github.review.comment.submitting ||
				m.github.review.submit.open || m.github.review.submit.submitting ||
				m.github.review.metadata.editing || m.github.review.metadata.submitting {
				t.Fatalf("stale %s result changed pull request #43: detail=%+v review=%+v",
					test.name, m.github.detail, m.github.review)
			}
		})
	}
}

func TestGitHubReviewActionsSubmitResolveUpdateAndComment(t *testing.T) {
	var mu sync.Mutex
	var createInputs []map[string]any
	var addThreadInput map[string]any
	var submitInput map[string]any
	var metadataInput map[string]any
	var timelineBody map[string]any
	var resolved []bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		if r.Method == http.MethodPost &&
			strings.HasSuffix(r.URL.Path, "/issues/42/comments") {
			mu.Lock()
			_ = json.NewDecoder(r.Body).Decode(&timelineBody)
			mu.Unlock()
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, `{"id":1}`)
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
		case strings.Contains(request.Query, "GoIDEACreateReview"):
			mu.Lock()
			createInputs = append(createInputs, request.Variables)
			mu.Unlock()
			io.WriteString(w, `{"data":{"addPullRequestReview":{"pullRequestReview":{
				"id":"REV_PENDING","state":"PENDING","comments":{"totalCount":1}}}}}`)
		case strings.Contains(request.Query, "GoIDEAAddReviewThread"):
			mu.Lock()
			addThreadInput = request.Variables
			mu.Unlock()
			io.WriteString(w, `{"data":{"addPullRequestReviewThread":{"thread":{
				"id":"THREAD_1","isResolved":false}}}}`)
		case strings.Contains(request.Query, "GoIDEASubmitReview"):
			mu.Lock()
			submitInput = request.Variables
			mu.Unlock()
			io.WriteString(w, `{"data":{"submitPullRequestReview":{
				"clientMutationId":null}}}`)
		case strings.Contains(request.Query, "GoIDEAResolveReviewThread"):
			mu.Lock()
			resolved = append(resolved, true)
			mu.Unlock()
			io.WriteString(w, `{"data":{"resolveReviewThread":{"thread":{
				"id":"THREAD_1","isResolved":true}}}}`)
		case strings.Contains(request.Query, "GoIDEAUnresolveReviewThread"):
			mu.Lock()
			resolved = append(resolved, false)
			mu.Unlock()
			io.WriteString(w, `{"data":{"unresolveReviewThread":{"thread":{
				"id":"THREAD_1","isResolved":false}}}}`)
		case strings.Contains(request.Query, "GoIDEAUpdatePullRequest"):
			mu.Lock()
			metadataInput = request.Variables
			mu.Unlock()
			io.WriteString(w, `{"data":{"updatePullRequest":{"pullRequest":{
				"id":"PR_42","title":"New title","body":"New body"}}}}`)
		default:
			http.Error(w, "unexpected query", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	m, callbacks := githubReviewTestModelWithCallbacks(t)
	m.github.client = testGitHubClient(server, m.github.credentials)
	m.github.detail = githubReviewTestDetail()
	m.github.repository = ghub.Repository{}
	m.github.review.threads = []ghub.ReviewThread{{
		ID: "THREAD_1", Path: "src/Main.java", DiffSide: "RIGHT", Line: 3,
		ViewerCanResolve: true, ViewerCanUnresolve: true,
	}}
	m.github.review.selectedThread = "THREAD_1"

	m.github.review.comment = githubReviewCommentDraft{
		open: true, path: "src/Main.java", side: "RIGHT",
		startSide: "RIGHT", startLine: 2, line: 3, body: " Multiline comment ",
	}
	m.SubmitGitHubReviewComment(false)
	waitGitHubCallback(t, callbacks)
	if m.github.review.comment.open || m.github.notice != "Review comment added" {
		t.Fatalf("start review state = %+v notice=%q", m.github.review.comment, m.github.notice)
	}

	m.github.review.pending = ghub.PendingReview{
		ID: "REV_PENDING", State: "PENDING", CommentsCount: 1,
	}
	m.github.review.comment = githubReviewCommentDraft{
		open: true, path: "src/Main.java", side: "RIGHT",
		startSide: "RIGHT", startLine: 2, line: 3, body: " Another comment ",
	}
	m.SubmitGitHubReviewComment(false)
	waitGitHubCallback(t, callbacks)

	m.github.review.pending = ghub.PendingReview{}
	m.github.review.submit = githubReviewSubmitState{open: true, body: "Looks good"}
	m.SubmitGitHubReview("APPROVE")
	waitGitHubCallback(t, callbacks)
	if m.github.notice != "Review approved" || m.github.review.submit.open {
		t.Fatalf("approval state = %+v notice=%q", m.github.review.submit, m.github.notice)
	}

	m.github.review.pending = ghub.PendingReview{
		ID: "REV_PENDING", State: "PENDING", CommentsCount: 1,
	}
	m.github.review.submit = githubReviewSubmitState{open: true, body: "Fix the test"}
	m.SubmitGitHubReview("REQUEST_CHANGES")
	waitGitHubCallback(t, callbacks)

	m.SetSelectedGitHubReviewThreadResolved(true)
	waitGitHubCallback(t, callbacks)
	if !m.github.review.threads[0].IsResolved ||
		m.github.detail.UnresolvedThreads != 0 ||
		m.github.notice != "Conversation resolved" {
		t.Fatalf("resolved state = %+v detail=%+v", m.github.review.threads[0], m.github.detail)
	}
	m.SetSelectedGitHubReviewThreadResolved(false)
	waitGitHubCallback(t, callbacks)
	if m.github.review.threads[0].IsResolved ||
		m.github.detail.UnresolvedThreads != 1 ||
		m.github.notice != "Conversation reopened" {
		t.Fatalf("reopened state = %+v detail=%+v", m.github.review.threads[0], m.github.detail)
	}

	m.github.review.metadata = githubMetadataState{
		editing: true, title: "New title", body: "New body", base: "main",
	}
	m.SaveGitHubMetadata()
	waitGitHubCallback(t, callbacks)
	if m.github.review.metadata.editing || m.github.notice != "Pull request updated" {
		t.Fatalf("metadata state = %+v notice=%q", m.github.review.metadata, m.github.notice)
	}

	m.github.review.timelineComment.body = "Timeline comment"
	m.AddGitHubTimelineComment()
	waitGitHubCallback(t, callbacks)
	if m.github.review.timelineComment.body != "" || m.github.notice != "Comment added" {
		t.Fatalf("timeline state = %+v notice=%q",
			m.github.review.timelineComment, m.github.notice)
	}

	mu.Lock()
	gotCreateInputs := append([]map[string]any(nil), createInputs...)
	gotAddThread := addThreadInput
	gotSubmit := submitInput
	gotMetadata := metadataInput
	gotTimeline := timelineBody
	gotResolved := append([]bool(nil), resolved...)
	mu.Unlock()
	if len(gotCreateInputs) != 2 {
		t.Fatalf("create review inputs = %#v", gotCreateInputs)
	}
	firstThreads, _ := gotCreateInputs[0]["threads"].([]any)
	if gotCreateInputs[0]["event"] != nil || len(firstThreads) != 1 ||
		gotCreateInputs[1]["event"] != "APPROVE" {
		t.Fatalf("create review inputs = %#v", gotCreateInputs)
	}
	if gotAddThread["pullRequestReviewId"] != "REV_PENDING" ||
		gotAddThread["body"] != "Another comment" ||
		gotAddThread["startLine"] != float64(2) ||
		gotSubmit["reviewId"] != "REV_PENDING" ||
		gotSubmit["event"] != "REQUEST_CHANGES" ||
		gotSubmit["body"] != "Fix the test" {
		t.Fatalf("pending review inputs: add=%#v submit=%#v", gotAddThread, gotSubmit)
	}
	if gotMetadata["pullRequestId"] != "PR_42" ||
		gotMetadata["title"] != "New title" ||
		gotMetadata["body"] != "New body" ||
		gotTimeline["body"] != "Timeline comment" {
		t.Fatalf("metadata=%#v timeline=%#v", gotMetadata, gotTimeline)
	}
	if len(gotResolved) != 2 || !gotResolved[0] || gotResolved[1] {
		t.Fatalf("resolve calls = %v", gotResolved)
	}
}

func TestGitHubReviewSubmissionValidatesFeedbackAndAuthorship(t *testing.T) {
	m := githubReviewTestModel(t)
	m.github.detail = githubReviewTestDetail()

	m.SubmitGitHubReview("REQUEST_CHANGES")
	if m.github.review.submit.err !=
		"Add a review comment before requesting changes or commenting" {
		t.Fatalf("empty change request error = %q", m.github.review.submit.err)
	}
	m.SubmitGitHubReview("COMMENT")
	if m.github.review.submit.err !=
		"Add a review comment before requesting changes or commenting" {
		t.Fatalf("empty comment error = %q", m.github.review.submit.err)
	}

	m.github.detail.ViewerDidAuthor = true
	m.github.review.submit = githubReviewSubmitState{open: true}
	m.SubmitGitHubReview("APPROVE")
	if m.github.review.submit.err !=
		"The pull request author can submit a comment review" {
		t.Fatalf("author approval error = %q", m.github.review.submit.err)
	}

	m.github.review.pendingErr = "pending lookup failed"
	m.github.review.comment = githubReviewCommentDraft{
		open: true, path: "src/Main.java", side: "RIGHT",
		startSide: "RIGHT", startLine: 2, line: 3, body: "Comment",
	}
	m.SubmitGitHubReviewComment(false)
	if m.github.review.comment.err !=
		"Retry the pending review lookup before adding review comments" ||
		m.github.review.comment.submitting {
		t.Fatalf("pending comment guard = %+v", m.github.review.comment)
	}
	m.github.review.submit = githubReviewSubmitState{open: true, body: "Comment"}
	m.SubmitGitHubReview("COMMENT")
	if m.github.review.submit.err !=
		"Retry the pending review lookup before submitting a review" ||
		m.github.review.submit.submitting {
		t.Fatalf("pending submit guard = %+v", m.github.review.submit)
	}

	m.github.review.pendingErr = ""
	m.github.review.pendingLoading = true
	m.SubmitGitHubReviewComment(false)
	if m.github.review.comment.err !=
		"Wait for the pending review lookup before adding review comments" ||
		m.github.review.comment.submitting {
		t.Fatalf("pending comment loading guard = %+v", m.github.review.comment)
	}
	m.SubmitGitHubReview("COMMENT")
	if m.github.review.submit.err !=
		"Wait for the pending review lookup before submitting a review" ||
		m.github.review.submit.submitting {
		t.Fatalf("pending submit loading guard = %+v", m.github.review.submit)
	}
}

func TestGitHubStaleDetailDoesNotReplaceTheCurrentPullRequest(t *testing.T) {
	requestStarted := make(chan struct{})
	release := make(chan struct{})
	var startOnce sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startOnce.Do(func() { close(requestStarted) })
		<-release
		io.WriteString(w, githubPullRequestResponse("Stale title"))
	}))
	defer server.Close()

	m, callbacks := githubReviewTestModelWithCallbacks(t)
	m.github.client = testGitHubClient(server, m.github.credentials)
	m.loadGitHubDetail()
	select {
	case <-requestStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("detail request did not start")
	}
	m.github.selectedNumber = 43
	m.github.detail = &ghub.PullRequestDetail{
		PullRequestSummary: ghub.PullRequestSummary{
			ID: "PR_43", Number: 43, Title: "Other pull request",
		},
	}
	close(release)
	waitGitHubCallback(t, callbacks)

	if m.github.detail.ID != "PR_43" || m.github.detail.Title != "Other pull request" ||
		m.github.selectedNumber != 43 {
		t.Fatalf("stale detail replaced the current pull request: %+v", m.github.detail)
	}
}
