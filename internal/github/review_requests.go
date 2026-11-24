package github

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type restPullFileJSON struct {
	SHA              string `json:"sha"`
	Filename         string `json:"filename"`
	PreviousFilename string `json:"previous_filename"`
	Status           string `json:"status"`
	Additions        int    `json:"additions"`
	Deletions        int    `json:"deletions"`
	Patch            string `json:"patch"`
}

type reviewCommentJSON struct {
	ID              string     `json:"id"`
	URL             string     `json:"url"`
	Author          *actorJSON `json:"author"`
	Body            string     `json:"body"`
	State           string     `json:"state"`
	CreatedAt       string     `json:"createdAt"`
	DiffHunk        string     `json:"diffHunk"`
	ViewerCanUpdate bool       `json:"viewerCanUpdate"`
	ViewerCanDelete bool       `json:"viewerCanDelete"`
}

type reviewThreadJSON struct {
	ID                 string `json:"id"`
	IsResolved         bool   `json:"isResolved"`
	IsOutdated         bool   `json:"isOutdated"`
	Path               string `json:"path"`
	SubjectType        string `json:"subjectType"`
	DiffSide           string `json:"diffSide"`
	Line               int    `json:"line"`
	OriginalLine       int    `json:"originalLine"`
	StartDiffSide      string `json:"startDiffSide"`
	StartLine          int    `json:"startLine"`
	OriginalStartLine  int    `json:"originalStartLine"`
	ViewerCanReply     bool   `json:"viewerCanReply"`
	ViewerCanResolve   bool   `json:"viewerCanResolve"`
	ViewerCanUnresolve bool   `json:"viewerCanUnresolve"`
	Comments           struct {
		Nodes []reviewCommentJSON `json:"nodes"`
	} `json:"comments"`
}

func repoRESTPath(repo Coordinates) string {
	return fmt.Sprintf("repos/%s/%s", url.PathEscape(repo.Owner), url.PathEscape(repo.Name))
}

// PullRequestFiles loads REST patches for the pull request's changed files.
// The page count is bounded by the client's MaxPages.
func (c *Client) PullRequestFiles(
	ctx context.Context,
	repo Coordinates,
	number int,
) ([]FileChange, error) {
	if number <= 0 {
		return nil, fmt.Errorf("pull request number is required")
	}
	files := []FileChange{}
	for page := 1; page <= c.maxPages; page++ {
		query := url.Values{
			"per_page": {fmt.Sprintf("%d", c.pageSize)},
			"page":     {fmt.Sprintf("%d", page)},
		}
		endpoint := fmt.Sprintf(
			"%s/pulls/%d/files?%s",
			repoRESTPath(repo), number, query.Encode(),
		)
		var response []restPullFileJSON
		if err := c.REST(ctx, http.MethodGet, endpoint, nil, &response); err != nil {
			return nil, err
		}
		for _, raw := range response {
			if raw.Filename == "" {
				continue
			}
			files = append(files, FileChange{
				Path:         raw.Filename,
				PreviousPath: raw.PreviousFilename,
				ChangeType:   strings.ToUpper(raw.Status),
				Additions:    raw.Additions,
				Deletions:    raw.Deletions,
				Patch:        raw.Patch,
			})
		}
		if len(response) < c.pageSize {
			break
		}
	}
	return files, nil
}

// ReviewThreads loads every bounded page of rich line threads and comments.
func (c *Client) ReviewThreads(
	ctx context.Context,
	repo Coordinates,
	number int,
) ([]ReviewThread, error) {
	if number <= 0 {
		return nil, fmt.Errorf("pull request number is required")
	}
	threads := []ReviewThread{}
	var cursor string
	for page := 0; page < c.maxPages; page++ {
		variables := map[string]any{
			"owner": repo.Owner, "name": repo.Name,
			"number": number, "pageSize": c.pageSize,
		}
		if cursor != "" {
			variables["cursor"] = cursor
		}
		var response struct {
			Repository *struct {
				PullRequest *struct {
					ReviewThreads struct {
						PageInfo pageInfoJSON       `json:"pageInfo"`
						Nodes    []reviewThreadJSON `json:"nodes"`
					} `json:"reviewThreads"`
				} `json:"pullRequest"`
			} `json:"repository"`
		}
		if err := c.GraphQL(ctx, pullRequestReviewThreadsQuery, variables, &response); err != nil {
			return nil, err
		}
		if response.Repository == nil || response.Repository.PullRequest == nil {
			return nil, fmt.Errorf("%w: pull request #%d", ErrNotFound, number)
		}
		connection := response.Repository.PullRequest.ReviewThreads
		for _, raw := range connection.Nodes {
			threads = append(threads, reviewThreadModel(raw))
		}
		if !connection.PageInfo.HasNextPage || connection.PageInfo.EndCursor == nil ||
			*connection.PageInfo.EndCursor == "" {
			break
		}
		cursor = *connection.PageInfo.EndCursor
	}
	return threads, nil
}

func reviewThreadModel(raw reviewThreadJSON) ReviewThread {
	thread := ReviewThread{
		ID: raw.ID, Path: raw.Path, SubjectType: raw.SubjectType,
		IsResolved: raw.IsResolved,
		IsOutdated: raw.IsOutdated, DiffSide: raw.DiffSide,
		Line: raw.Line, OriginalLine: raw.OriginalLine,
		StartDiffSide: raw.StartDiffSide, StartLine: raw.StartLine,
		OriginalStartLine:  raw.OriginalStartLine,
		ViewerCanReply:     raw.ViewerCanReply,
		ViewerCanResolve:   raw.ViewerCanResolve,
		ViewerCanUnresolve: raw.ViewerCanUnresolve,
	}
	for _, comment := range raw.Comments.Nodes {
		item := ReviewComment{
			ID: comment.ID, URL: comment.URL, Body: comment.Body,
			State: comment.State, CreatedAt: comment.CreatedAt,
			DiffHunk:        comment.DiffHunk,
			ViewerCanEdit:   comment.ViewerCanUpdate,
			ViewerCanDelete: comment.ViewerCanDelete,
		}
		if comment.Author != nil {
			item.Author = comment.Author.Login
		}
		thread.Comments = append(thread.Comments, item)
	}
	return thread
}

// PendingReview returns the viewer's unsubmitted review when one exists.
func (c *Client) PendingReview(
	ctx context.Context,
	pullRequestID string,
) (PendingReview, bool, error) {
	if pullRequestID == "" {
		return PendingReview{}, false, fmt.Errorf("pull request id is required")
	}
	var response struct {
		Node *struct {
			Reviews struct {
				Nodes []struct {
					ID       string `json:"id"`
					State    string `json:"state"`
					Comments struct {
						TotalCount int `json:"totalCount"`
					} `json:"comments"`
				} `json:"nodes"`
			} `json:"reviews"`
		} `json:"node"`
	}
	if err := c.GraphQL(ctx, pendingReviewQuery, map[string]any{
		"pullRequestId": pullRequestID,
	}, &response); err != nil {
		return PendingReview{}, false, err
	}
	if response.Node == nil || len(response.Node.Reviews.Nodes) == 0 {
		return PendingReview{}, false, nil
	}
	raw := response.Node.Reviews.Nodes[0]
	return PendingReview{
		ID: raw.ID, State: raw.State, CommentsCount: raw.Comments.TotalCount,
	}, true, nil
}

// CreateReview starts a pending review, submits a general review, or creates
// inline threads in one request, matching GitHub's addPullRequestReview input.
func (c *Client) CreateReview(
	ctx context.Context,
	input CreateReviewInput,
) (PendingReview, error) {
	if input.PullRequestID == "" {
		return PendingReview{}, fmt.Errorf("pull request id is required")
	}
	event := strings.ToUpper(strings.TrimSpace(input.Event))
	switch event {
	case "", "APPROVE", "REQUEST_CHANGES", "COMMENT":
	default:
		return PendingReview{}, fmt.Errorf("invalid review event %q", event)
	}
	threads, err := draftThreads(input.Threads)
	if err != nil {
		return PendingReview{}, err
	}
	variables := map[string]any{
		"pullRequestId": input.PullRequestID,
		"event":         nil,
		"body":          strings.TrimSpace(input.Body),
		"commitOid":     nil,
		"threads":       nil,
	}
	if event != "" {
		variables["event"] = event
	}
	if input.CommitOID != "" {
		variables["commitOid"] = input.CommitOID
	}
	if len(threads) > 0 {
		variables["threads"] = threads
	}
	var response struct {
		AddPullRequestReview struct {
			PullRequestReview struct {
				ID       string `json:"id"`
				State    string `json:"state"`
				Comments struct {
					TotalCount int `json:"totalCount"`
				} `json:"comments"`
			} `json:"pullRequestReview"`
		} `json:"addPullRequestReview"`
	}
	if err := c.GraphQL(ctx, createReviewMutation, variables, &response); err != nil {
		return PendingReview{}, err
	}
	raw := response.AddPullRequestReview.PullRequestReview
	return PendingReview{
		ID: raw.ID, State: raw.State, CommentsCount: raw.Comments.TotalCount,
	}, nil
}

// SubmitReview submits an existing pending review.
func (c *Client) SubmitReview(
	ctx context.Context,
	reviewID, event, body string,
) error {
	if reviewID == "" {
		return fmt.Errorf("review id is required")
	}
	event = strings.ToUpper(strings.TrimSpace(event))
	switch event {
	case "APPROVE", "REQUEST_CHANGES", "COMMENT":
	default:
		return fmt.Errorf("invalid review event %q", event)
	}
	var response map[string]any
	return c.GraphQL(ctx, submitReviewMutation, map[string]any{
		"reviewId": reviewID,
		"event":    event,
		"body":     strings.TrimSpace(body),
	}, &response)
}

// AddReviewThread adds a line comment to an existing pending review.
func (c *Client) AddReviewThread(
	ctx context.Context,
	reviewID string,
	input ReviewThreadInput,
) error {
	if reviewID == "" {
		return fmt.Errorf("review id is required")
	}
	thread, err := draftThread(input)
	if err != nil {
		return err
	}
	var response map[string]any
	return c.GraphQL(ctx, addReviewThreadMutation, map[string]any{
		"pullRequestReviewId": reviewID,
		"body":                thread["body"],
		"path":                thread["path"],
		"line":                thread["line"],
		"side":                thread["side"],
		"startLine":           thread["startLine"],
		"startSide":           thread["startSide"],
	}, &response)
}

func draftThreads(inputs []ReviewThreadInput) ([]map[string]any, error) {
	threads := make([]map[string]any, 0, len(inputs))
	for _, input := range inputs {
		thread, err := draftThread(input)
		if err != nil {
			return nil, err
		}
		threads = append(threads, thread)
	}
	return threads, nil
}

func draftThread(input ReviewThreadInput) (map[string]any, error) {
	if strings.TrimSpace(input.Body) == "" {
		return nil, fmt.Errorf("review comment body is required")
	}
	if strings.TrimSpace(input.Path) == "" || input.Line <= 0 {
		return nil, fmt.Errorf("review comment path and line are required")
	}
	side := strings.ToUpper(strings.TrimSpace(input.Side))
	if side == "" {
		side = "RIGHT"
	}
	if side != "LEFT" && side != "RIGHT" {
		return nil, fmt.Errorf("invalid diff side %q", side)
	}
	startSide := strings.ToUpper(strings.TrimSpace(input.StartSide))
	if startSide == "" {
		startSide = side
	}
	if startSide != "LEFT" && startSide != "RIGHT" {
		return nil, fmt.Errorf("invalid start diff side %q", startSide)
	}
	thread := map[string]any{
		"body":      strings.TrimSpace(input.Body),
		"path":      input.Path,
		"line":      input.Line,
		"side":      side,
		"startLine": nil,
		"startSide": nil,
	}
	if input.StartLine > 0 && input.StartLine != input.Line {
		if input.StartLine > input.Line {
			return nil, fmt.Errorf("start line must not follow the end line")
		}
		thread["startLine"] = input.StartLine
		thread["startSide"] = startSide
	}
	return thread, nil
}

// SetReviewThreadResolved resolves or reopens one review thread.
func (c *Client) SetReviewThreadResolved(
	ctx context.Context,
	threadID string,
	resolved bool,
) error {
	if threadID == "" {
		return fmt.Errorf("review thread id is required")
	}
	query := unresolveReviewThreadMutation
	if resolved {
		query = resolveReviewThreadMutation
	}
	var response map[string]any
	return c.GraphQL(ctx, query, map[string]any{"threadId": threadID}, &response)
}

// UpdatePullRequestMetadata changes the title and description.
func (c *Client) UpdatePullRequestMetadata(
	ctx context.Context,
	pullRequestID string,
	input PullRequestMetadataInput,
) error {
	if pullRequestID == "" || strings.TrimSpace(input.Title) == "" {
		return fmt.Errorf("pull request id and title are required")
	}
	var response map[string]any
	return c.GraphQL(ctx, updatePullRequestMetadataMutation, map[string]any{
		"pullRequestId": pullRequestID,
		"title":         strings.TrimSpace(input.Title),
		"body":          input.Body,
	}, &response)
}

// UpdatePullRequestBase changes the base branch through GitHub's REST API.
// GitHub has no API operation that changes an existing pull request head.
func (c *Client) UpdatePullRequestBase(
	ctx context.Context,
	repo Coordinates,
	number int,
	base string,
) error {
	base = strings.TrimSpace(base)
	if number <= 0 || base == "" {
		return fmt.Errorf("pull request number and base branch are required")
	}
	endpoint := fmt.Sprintf("%s/pulls/%d", repoRESTPath(repo), number)
	return c.REST(ctx, http.MethodPatch, endpoint, map[string]any{"base": base}, nil)
}

// OpenPullRequestForBranch finds the open pull request whose head is branch.
func (c *Client) OpenPullRequestForBranch(
	ctx context.Context,
	repo Coordinates,
	branch string,
) (BranchPullRequest, bool, error) {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return BranchPullRequest{}, false, nil
	}
	query := url.Values{
		"state":    {"open"},
		"head":     {repo.Owner + ":" + branch},
		"per_page": {"1"},
	}
	endpoint := repoRESTPath(repo) + "/pulls?" + query.Encode()
	var response []struct {
		Number  int    `json:"number"`
		Title   string `json:"title"`
		HTMLURL string `json:"html_url"`
	}
	if err := c.REST(ctx, http.MethodGet, endpoint, nil, &response); err != nil {
		return BranchPullRequest{}, false, err
	}
	if len(response) == 0 || response[0].Number <= 0 {
		return BranchPullRequest{}, false, nil
	}
	return BranchPullRequest{
		Number: response[0].Number,
		Title:  response[0].Title,
		URL:    response[0].HTMLURL,
	}, true, nil
}

// CreateIssueComment adds a conversation comment to the pull request timeline.
func (c *Client) CreateIssueComment(
	ctx context.Context,
	repo Coordinates,
	number int,
	body string,
) error {
	if number <= 0 || strings.TrimSpace(body) == "" {
		return fmt.Errorf("pull request number and comment body are required")
	}
	endpoint := fmt.Sprintf("%s/issues/%d/comments", repoRESTPath(repo), number)
	return c.REST(ctx, http.MethodPost, endpoint, map[string]any{
		"body": strings.TrimSpace(body),
	}, nil)
}
