package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func githubReviewRequest(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	var request map[string]any
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	return request
}

func githubReviewRequestMap(t *testing.T, value any) map[string]any {
	t.Helper()
	item, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("value = %#v, want object", value)
	}
	return item
}

func githubReviewVariables(t *testing.T, request map[string]any) map[string]any {
	t.Helper()
	return githubReviewRequestMap(t, request["variables"])
}

func TestPullRequestFilesUsesBoundedRESTPagesAndMapsPatches(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer review-token" ||
			r.Header.Get("Accept") != "application/vnd.github+json" ||
			r.Header.Get("X-GitHub-Api-Version") != "2022-11-28" {
			t.Errorf("headers = %#v", r.Header)
		}
		var page int
		fmt.Sscan(r.URL.Query().Get("page"), &page)
		if r.URL.Path != "/repos/acme/goidea/pulls/42/files" ||
			r.URL.Query().Get("per_page") != "2" {
			t.Errorf("request = %s", r.URL.String())
		}
		mu.Lock()
		paths = append(paths, r.URL.RequestURI())
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch page {
		case 1:
			io.WriteString(w, `[
				{"sha":"one","filename":"src/Main.java","previous_filename":"src/Old.java",
				 "status":"renamed","additions":3,"deletions":1,
				 "patch":"@@ -1,1 +1,2 @@\n-old\n+new\n+more"},
				{"sha":"two","filename":"image.png","status":"modified","additions":0,"deletions":0}
			]`)
		case 2:
			io.WriteString(w, `[
				{"sha":"three","filename":"src/Other.java","status":"added",
				 "additions":1,"deletions":0,"patch":"@@ -0,0 +1 @@\n+other"}
			]`)
		default:
			t.Errorf("unexpected page %d", page)
			http.Error(w, "unexpected page", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	client := NewClient(staticCredentials("review-token"), ClientOptions{
		APIBase: server.URL, PageSize: 2, MaxPages: 4,
	})
	files, err := client.PullRequestFiles(
		context.Background(), Coordinates{Owner: "acme", Name: "goidea"}, 42,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Fatalf("files = %+v", files)
	}
	if files[0].Path != "src/Main.java" || files[0].PreviousPath != "src/Old.java" ||
		files[0].ChangeType != "RENAMED" || files[0].Additions != 3 ||
		files[0].Deletions != 1 || !strings.Contains(files[0].Patch, "+more") {
		t.Fatalf("first file = %+v", files[0])
	}
	if files[1].Patch != "" || files[2].ChangeType != "ADDED" {
		t.Fatalf("remaining files = %+v", files[1:])
	}
	mu.Lock()
	gotPaths := append([]string(nil), paths...)
	mu.Unlock()
	if len(gotPaths) != 2 ||
		!strings.Contains(gotPaths[0], "page=1&per_page=2") ||
		!strings.Contains(gotPaths[1], "page=2&per_page=2") {
		t.Fatalf("request paths = %v", gotPaths)
	}
}

func TestPullRequestFilesStopsAtMaxPages(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		io.WriteString(w, `[
			{"filename":"one.java","status":"modified"},
			{"filename":"two.java","status":"modified"}
		]`)
	}))
	defer server.Close()

	client := NewClient(staticCredentials("token"), ClientOptions{
		APIBase: server.URL, PageSize: 2, MaxPages: 2,
	})
	files, err := client.PullRequestFiles(
		context.Background(), Coordinates{Owner: "acme", Name: "goidea"}, 42,
	)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(files) != 4 {
		t.Fatalf("calls = %d, files = %+v", calls, files)
	}
}

func TestReviewThreadsPaginateAndMapCommentMetadata(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer thread-token" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		request := githubReviewRequest(t, r)
		query, _ := request["query"].(string)
		if !strings.Contains(query, "GoIDEAReviewThreads") {
			t.Fatalf("query = %s", query)
		}
		variables := githubReviewVariables(t, request)
		if variables["owner"] != "acme" || variables["name"] != "goidea" ||
			variables["number"] != float64(42) || variables["pageSize"] != float64(2) {
			t.Errorf("variables = %#v", variables)
		}
		switch calls {
		case 1:
			if _, ok := variables["cursor"]; ok {
				t.Errorf("first page cursor = %#v", variables["cursor"])
			}
			io.WriteString(w, `{"data":{"repository":{"pullRequest":{"reviewThreads":{
				"pageInfo":{"hasNextPage":true,"endCursor":"threads-2"},
				"nodes":[{
					"id":"THREAD_1","isResolved":false,"isOutdated":false,
					"path":"src/Main.java","subjectType":"LINE",
					"diffSide":"RIGHT","line":7,
					"originalLine":7,"startDiffSide":"RIGHT","startLine":6,
					"originalStartLine":6,"viewerCanReply":true,
					"viewerCanResolve":true,"viewerCanUnresolve":false,
					"comments":{"nodes":[{
						"id":"COMMENT_1","url":"https://comment/1",
						"author":{"login":"reviewer"},"body":"Use the new API",
						"state":"SUBMITTED","createdAt":"2026-11-02T08:00:00Z",
						"diffHunk":"@@ -1 +1 @@","viewerCanUpdate":false,
						"viewerCanDelete":false
					}]}
				}]}
			}}}}`)
		case 2:
			if variables["cursor"] != "threads-2" {
				t.Errorf("second page cursor = %#v", variables["cursor"])
			}
			io.WriteString(w, `{"data":{"repository":{"pullRequest":{"reviewThreads":{
				"pageInfo":{"hasNextPage":false,"endCursor":null},
				"nodes":[{
					"id":"THREAD_2","isResolved":true,"isOutdated":true,
					"path":"src/Other.java","subjectType":"FILE",
					"diffSide":"LEFT","line":3,
					"originalLine":2,"startDiffSide":"LEFT","startLine":3,
					"originalStartLine":2,"viewerCanReply":false,
					"viewerCanResolve":false,"viewerCanUnresolve":true,
					"comments":{"nodes":[]}
				}]}
			}}}}`)
		default:
			t.Errorf("unexpected call %d", calls)
		}
	}))
	defer server.Close()

	client := NewClient(staticCredentials("thread-token"), ClientOptions{
		GraphQLEndpoint: server.URL + "/graphql", PageSize: 2, MaxPages: 4,
	})
	threads, err := client.ReviewThreads(
		context.Background(), Coordinates{Owner: "acme", Name: "goidea"}, 42,
	)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(threads) != 2 {
		t.Fatalf("calls = %d, threads = %+v", calls, threads)
	}
	first := threads[0]
	if first.ID != "THREAD_1" || first.Path != "src/Main.java" ||
		first.SubjectType != "LINE" || first.DiffSide != "RIGHT" ||
		first.Line != 7 || first.StartLine != 6 ||
		!first.ViewerCanReply || !first.ViewerCanResolve ||
		len(first.Comments) != 1 || first.Comments[0].Author != "reviewer" ||
		first.Comments[0].URL != "https://comment/1" ||
		first.Comments[0].DiffHunk != "@@ -1 +1 @@" {
		t.Fatalf("first thread = %+v", first)
	}
	if threads[1].SubjectType != "FILE" ||
		!threads[1].IsResolved || !threads[1].IsOutdated ||
		!threads[1].ViewerCanUnresolve || threads[1].ViewerCanResolve {
		t.Fatalf("second thread = %+v", threads[1])
	}
}

func TestReviewThreadsRespectsMaxPages(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		io.WriteString(w, `{"data":{"repository":{"pullRequest":{"reviewThreads":{
			"pageInfo":{"hasNextPage":true,"endCursor":"more"},
			"nodes":[{"id":"THREAD_`+strconv.Itoa(calls)+`","path":"src/Main.java",
				"diffSide":"RIGHT","line":1,"comments":{"nodes":[]}}]
		}}}}}`)
	}))
	defer server.Close()

	client := NewClient(staticCredentials("token"), ClientOptions{
		GraphQLEndpoint: server.URL, PageSize: 1, MaxPages: 2,
	})
	threads, err := client.ReviewThreads(
		context.Background(), Coordinates{Owner: "acme", Name: "goidea"}, 42,
	)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(threads) != 2 {
		t.Fatalf("calls = %d, threads = %+v", calls, threads)
	}
}

func TestPendingReviewFindsOnlyTheViewerPendingReview(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		request := githubReviewRequest(t, r)
		variables := githubReviewVariables(t, request)
		if variables["pullRequestId"] != "PR_42" {
			t.Errorf("variables = %#v", variables)
		}
		if calls == 1 {
			io.WriteString(w, `{"data":{"node":{
				"reviews":{"nodes":[{"id":"REV_PENDING","state":"PENDING",
				"comments":{"totalCount":2}}]}}}}`)
			return
		}
		io.WriteString(w, `{"data":{"node":{"reviews":{"nodes":[]}}}}`)
	}))
	defer server.Close()

	client := NewClient(staticCredentials("token"), ClientOptions{GraphQLEndpoint: server.URL})
	pending, ok, err := client.PendingReview(context.Background(), "PR_42")
	if err != nil || !ok || pending.ID != "REV_PENDING" ||
		pending.State != "PENDING" || pending.CommentsCount != 2 {
		t.Fatalf("pending = %+v, ok=%v, err=%v", pending, ok, err)
	}
	pending, ok, err = client.PendingReview(context.Background(), "PR_42")
	if err != nil || ok || pending != (PendingReview{}) {
		t.Fatalf("empty pending = %+v, ok=%v, err=%v", pending, ok, err)
	}
}

func TestReviewRequestValidationDoesNotSendHTTP(t *testing.T) {
	client := NewClient(staticCredentials("token"), ClientOptions{
		GraphQLEndpoint: "http://127.0.0.1:1/graphql",
		APIBase:         "http://127.0.0.1:1",
	})
	repo := Coordinates{Owner: "acme", Name: "goidea"}
	tests := []struct {
		name string
		call func() error
	}{
		{"files number", func() error {
			_, err := client.PullRequestFiles(context.Background(), repo, 0)
			return err
		}},
		{"threads number", func() error {
			_, err := client.ReviewThreads(context.Background(), repo, 0)
			return err
		}},
		{"pending id", func() error {
			_, _, err := client.PendingReview(context.Background(), "")
			return err
		}},
		{"create review id", func() error {
			_, err := client.CreateReview(context.Background(), CreateReviewInput{})
			return err
		}},
		{"create review event", func() error {
			_, err := client.CreateReview(context.Background(), CreateReviewInput{
				PullRequestID: "PR_1", Event: "DISMISS",
			})
			return err
		}},
		{"create review body", func() error {
			_, err := client.CreateReview(context.Background(), CreateReviewInput{
				PullRequestID: "PR_1",
				Threads:       []ReviewThreadInput{{Path: "a.java", Line: 1, Body: " "}},
			})
			return err
		}},
		{"thread side", func() error {
			_, err := client.CreateReview(context.Background(), CreateReviewInput{
				PullRequestID: "PR_1",
				Threads: []ReviewThreadInput{{
					Path: "a.java", Line: 1, Body: "comment", Side: "MIDDLE",
				}},
			})
			return err
		}},
		{"thread range", func() error {
			_, err := client.CreateReview(context.Background(), CreateReviewInput{
				PullRequestID: "PR_1",
				Threads: []ReviewThreadInput{{
					Path: "a.java", Line: 2, StartLine: 3, Body: "comment",
				}},
			})
			return err
		}},
		{"submit review id", func() error {
			return client.SubmitReview(context.Background(), "", "APPROVE", "")
		}},
		{"submit review event", func() error {
			return client.SubmitReview(context.Background(), "REV_1", "DISMISS", "")
		}},
		{"add thread id", func() error {
			return client.AddReviewThread(context.Background(), "", ReviewThreadInput{
				Path: "a.java", Line: 1, Body: "comment",
			})
		}},
		{"resolve thread id", func() error {
			return client.SetReviewThreadResolved(context.Background(), "", true)
		}},
		{"metadata title", func() error {
			return client.UpdatePullRequestMetadata(context.Background(), "PR_1",
				PullRequestMetadataInput{Title: " "})
		}},
		{"base branch", func() error {
			return client.UpdatePullRequestBase(context.Background(), repo, 1, " ")
		}},
		{"timeline body", func() error {
			return client.CreateIssueComment(context.Background(), repo, 1, " ")
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); err == nil {
				t.Fatal("validation error = nil")
			}
		})
	}
}

func TestReviewMutationsSendExactGraphQLInputs(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost || r.URL.Path != "/graphql" {
			t.Errorf("request = %s %s", r.Method, r.URL)
		}
		if r.Header.Get("Authorization") != "Bearer mutation-token" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		request := githubReviewRequest(t, r)
		query, _ := request["query"].(string)
		variables := githubReviewVariables(t, request)
		switch {
		case strings.Contains(query, "GoIDEACreateReview"):
			if variables["pullRequestId"] != "PR_42" ||
				variables["event"] != "COMMENT" ||
				variables["body"] != "Review summary" ||
				variables["commitOid"] != "head-oid" {
				t.Errorf("create review variables = %#v", variables)
			}
			threads, ok := variables["threads"].([]any)
			if !ok || len(threads) != 2 {
				t.Fatalf("threads = %#v", variables["threads"])
			}
			single := githubReviewRequestMap(t, threads[0])
			if single["body"] != "Single line" || single["path"] != "src/Main.java" ||
				single["line"] != float64(7) || single["side"] != "RIGHT" ||
				single["startLine"] != nil || single["startSide"] != nil {
				t.Errorf("single thread = %#v", single)
			}
			multiline := githubReviewRequestMap(t, threads[1])
			if multiline["body"] != "Range" || multiline["line"] != float64(9) ||
				multiline["startLine"] != float64(5) ||
				multiline["startSide"] != "LEFT" ||
				multiline["side"] != "RIGHT" {
				t.Errorf("multiline thread = %#v", multiline)
			}
			io.WriteString(w, `{"data":{"addPullRequestReview":{"pullRequestReview":{
				"id":"REV_1","state":"COMMENTED","comments":{"totalCount":2}}}}}`)
		case strings.Contains(query, "GoIDEAAddReviewThread"):
			if variables["pullRequestReviewId"] != "REV_1" ||
				variables["body"] != "Pending comment" ||
				variables["path"] != "src/Main.java" ||
				variables["line"] != float64(12) ||
				variables["side"] != "RIGHT" ||
				variables["startLine"] != float64(10) ||
				variables["startSide"] != "RIGHT" {
				t.Errorf("add thread variables = %#v", variables)
			}
			io.WriteString(w, `{"data":{"addPullRequestReviewThread":{"thread":{
				"id":"THREAD_3","isResolved":false}}}}`)
		case strings.Contains(query, "GoIDEASubmitReview"):
			if variables["reviewId"] != "REV_1" ||
				variables["event"] != "REQUEST_CHANGES" ||
				variables["body"] != "Fix tests" {
				t.Errorf("submit variables = %#v", variables)
			}
			io.WriteString(w, `{"data":{"submitPullRequestReview":{
				"clientMutationId":null}}}`)
		case strings.Contains(query, "GoIDEAResolveReviewThread"):
			if variables["threadId"] != "THREAD_3" {
				t.Errorf("resolve variables = %#v", variables)
			}
			io.WriteString(w, `{"data":{"resolveReviewThread":{"thread":{
				"id":"THREAD_3","isResolved":true}}}}`)
		case strings.Contains(query, "GoIDEAUnresolveReviewThread"):
			if variables["threadId"] != "THREAD_3" {
				t.Errorf("unresolve variables = %#v", variables)
			}
			io.WriteString(w, `{"data":{"unresolveReviewThread":{"thread":{
				"id":"THREAD_3","isResolved":false}}}}`)
		case strings.Contains(query, "GoIDEAUpdatePullRequest"):
			if variables["pullRequestId"] != "PR_42" ||
				variables["title"] != "New title" ||
				variables["body"] != "New description" {
				t.Errorf("metadata variables = %#v", variables)
			}
			io.WriteString(w, `{"data":{"updatePullRequest":{"pullRequest":{
				"id":"PR_42","title":"New title","body":"New description"}}}}`)
		default:
			t.Errorf("unexpected query: %s", query)
			http.Error(w, "unexpected query", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	client := NewClient(staticCredentials("mutation-token"), ClientOptions{
		GraphQLEndpoint: server.URL + "/graphql",
	})
	review, err := client.CreateReview(context.Background(), CreateReviewInput{
		PullRequestID: "PR_42",
		Event:         "comment",
		Body:          " Review summary ",
		CommitOID:     "head-oid",
		Threads: []ReviewThreadInput{
			{Path: "src/Main.java", Line: 7, Body: "Single line", Side: "RIGHT"},
			{
				Path: "src/Main.java", Line: 9, StartLine: 5,
				StartSide: "LEFT", Side: "RIGHT", Body: "Range",
			},
		},
	})
	if err != nil || review.ID != "REV_1" || review.CommentsCount != 2 {
		t.Fatalf("review = %+v, err=%v", review, err)
	}
	if err := client.AddReviewThread(context.Background(), "REV_1", ReviewThreadInput{
		Path: "src/Main.java", Line: 12, StartLine: 10,
		StartSide: "RIGHT", Side: "RIGHT", Body: " Pending comment ",
	}); err != nil {
		t.Fatal(err)
	}
	if err := client.SubmitReview(context.Background(), "REV_1",
		"request_changes", " Fix tests "); err != nil {
		t.Fatal(err)
	}
	if err := client.SetReviewThreadResolved(context.Background(), "THREAD_3", true); err != nil {
		t.Fatal(err)
	}
	if err := client.SetReviewThreadResolved(context.Background(), "THREAD_3", false); err != nil {
		t.Fatal(err)
	}
	if err := client.UpdatePullRequestMetadata(context.Background(), "PR_42",
		PullRequestMetadataInput{Title: " New title ", Body: "New description"}); err != nil {
		t.Fatal(err)
	}
	if calls != 6 {
		t.Fatalf("calls = %d", calls)
	}
}

func TestReviewRESTOperationsUseExactPathsAndBodies(t *testing.T) {
	seen := map[string]bool{}
	var branchHeads []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer rest-token" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		switch r.Method + " " + r.URL.Path {
		case "PATCH /repos/acme/goidea/pulls/42":
			seen["base"] = true
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if len(body) != 1 || body["base"] != "develop" {
				t.Errorf("base body = %#v", body)
			}
			io.WriteString(w, `{"number":42,"base":{"ref":"develop"}}`)
		case "GET /repos/acme/goidea/pulls":
			seen["branch"] = true
			query := r.URL.Query()
			branchHeads = append(branchHeads, query.Get("head"))
			if query.Get("state") != "open" ||
				query.Get("base") != "" ||
				query.Get("per_page") != "1" {
				t.Errorf("branch query = %s", r.URL.RawQuery)
			}
			io.WriteString(w, `[{
				"number":42,"title":"Review flow",
				"html_url":"https://github.com/acme/goidea/pull/42",
				"head":{"sha":"head-oid"}
			}]`)
		case "POST /repos/acme/goidea/issues/42/comments":
			seen["comment"] = true
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if len(body) != 1 || body["body"] != "Timeline comment" {
				t.Errorf("comment body = %#v", body)
			}
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, `{"id":1,"body":"Timeline comment"}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewClient(staticCredentials("rest-token"), ClientOptions{APIBase: server.URL})
	repo := Coordinates{Owner: "acme", Name: "goidea"}
	if err := client.UpdatePullRequestBase(context.Background(), repo, 42, " develop "); err != nil {
		t.Fatal(err)
	}
	found, ok, err := client.OpenPullRequestForBranch(context.Background(), repo, " topic ")
	if err != nil || !ok || found.Number != 42 || found.Title != "Review flow" ||
		found.URL != "https://github.com/acme/goidea/pull/42" || found.HeadOID != "head-oid" {
		t.Fatalf("branch pull request = %+v, ok=%v, err=%v", found, ok, err)
	}
	found, ok, err = client.OpenPullRequestForHead(
		context.Background(), repo, "contributor", "topic",
	)
	if err != nil || !ok || found.Number != 42 {
		t.Fatalf("fork pull request = %+v, ok=%v, err=%v", found, ok, err)
	}
	if err := client.CreateIssueComment(
		context.Background(), repo, 42, " Timeline comment ",
	); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 3 || !seen["base"] || !seen["branch"] || !seen["comment"] {
		t.Fatalf("seen = %#v", seen)
	}
	if len(branchHeads) != 2 ||
		branchHeads[0] != "acme:topic" ||
		branchHeads[1] != "contributor:topic" {
		t.Fatalf("branch heads = %v", branchHeads)
	}
}

func TestReviewRequestErrorsAreBoundedRedactedAndCancelable(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch {
		case strings.HasSuffix(r.URL.Path, "/files"):
			w.WriteHeader(http.StatusUnauthorized)
			io.WriteString(w, `{"message":"bad secret-token"}`)
		case r.URL.Path == "/graphql":
			request := githubReviewRequest(t, r)
			query, _ := request["query"].(string)
			if strings.Contains(query, "GoIDEASubmitReview") {
				io.WriteString(w, `{"errors":[{"message":"rejected secret-token"}]}`)
				return
			}
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"message":"missing"}`)
		default:
			io.WriteString(w, `[]`)
		}
	}))
	defer server.Close()

	client := NewClient(staticCredentials("secret-token"), ClientOptions{
		GraphQLEndpoint: server.URL + "/graphql",
		APIBase:         server.URL,
	})
	_, err := client.PullRequestFiles(
		context.Background(), Coordinates{Owner: "acme", Name: "goidea"}, 42,
	)
	if !errors.Is(err, ErrUnauthorized) || strings.Contains(err.Error(), "secret-token") ||
		!strings.Contains(err.Error(), "[redacted token]") {
		t.Fatalf("unauthorized error = %v", err)
	}

	err = client.SubmitReview(context.Background(), "REV_1", "APPROVE", "")
	if err == nil || strings.Contains(err.Error(), "secret-token") ||
		!strings.Contains(err.Error(), "[redacted token]") {
		t.Fatalf("GraphQL error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.ReviewThreads(
		ctx, Coordinates{Owner: "acme", Name: "goidea"}, 42,
	)
	if err == nil || (!errors.Is(err, context.Canceled) &&
		!strings.Contains(strings.ToLower(err.Error()), "canceled")) {
		t.Fatalf("cancellation error = %v", err)
	}
	if calls != 2 {
		t.Fatalf("HTTP calls = %d, canceled request reached the server", calls)
	}
}
