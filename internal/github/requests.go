package github

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type actorJSON struct {
	Login     string `json:"login"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatarUrl"`
}

type pageInfoJSON struct {
	HasNextPage bool    `json:"hasNextPage"`
	EndCursor   *string `json:"endCursor"`
}

type reviewJSON struct {
	ID        string     `json:"id"`
	URL       string     `json:"url"`
	Author    *actorJSON `json:"author"`
	Body      string     `json:"body"`
	State     string     `json:"state"`
	CreatedAt string     `json:"createdAt"`
}

type labelJSON struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

type statusRollupJSON struct {
	State    string `json:"state"`
	Contexts struct {
		Nodes []struct {
			TypeName   string `json:"__typename"`
			Name       string `json:"name"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
			DetailsURL string `json:"detailsUrl"`
			CheckSuite *struct {
				WorkflowRun *struct {
					Workflow *struct {
						Name string `json:"name"`
					} `json:"workflow"`
				} `json:"workflowRun"`
			} `json:"checkSuite"`
			Context     string `json:"context"`
			Description string `json:"description"`
			State       string `json:"state"`
			TargetURL   string `json:"targetUrl"`
		} `json:"nodes"`
	} `json:"contexts"`
}

type summaryJSON struct {
	ID             string     `json:"id"`
	URL            string     `json:"url"`
	Number         int        `json:"number"`
	Title          string     `json:"title"`
	State          string     `json:"state"`
	IsDraft        bool       `json:"isDraft"`
	Author         *actorJSON `json:"author"`
	CreatedAt      string     `json:"createdAt"`
	UpdatedAt      string     `json:"updatedAt"`
	IsReadByViewer bool       `json:"isReadByViewer"`
	ReviewDecision string     `json:"reviewDecision"`
	Mergeable      string     `json:"mergeable"`
	Labels         struct {
		Nodes []labelJSON `json:"nodes"`
	} `json:"labels"`
	Assignees struct {
		Nodes []actorJSON `json:"nodes"`
	} `json:"assignees"`
	Comments struct {
		TotalCount int `json:"totalCount"`
	} `json:"comments"`
	ReviewThreads struct {
		Nodes []struct {
			IsResolved bool `json:"isResolved"`
		} `json:"nodes"`
	} `json:"reviewThreads"`
	Reviews struct {
		Nodes []reviewJSON `json:"nodes"`
	} `json:"reviews"`
	Commits struct {
		Nodes []struct {
			Commit struct {
				StatusCheckRollup *statusRollupJSON `json:"statusCheckRollup"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
}

func (v summaryJSON) model() PullRequestSummary {
	pr := PullRequestSummary{
		ID: v.ID, URL: v.URL, Number: v.Number, Title: v.Title,
		State: v.State, Draft: v.IsDraft, CreatedAt: v.CreatedAt,
		UpdatedAt: v.UpdatedAt, ReviewDecision: v.ReviewDecision,
		Mergeable: v.Mergeable, Unread: !v.IsReadByViewer,
		CommentCount: v.Comments.TotalCount,
	}
	if v.Author != nil {
		pr.Author = v.Author.Login
		pr.AuthorAvatarURL = v.Author.AvatarURL
	}
	for _, label := range v.Labels.Nodes {
		pr.Labels = append(pr.Labels, Label{Name: label.Name, Color: label.Color})
	}
	for _, assignee := range v.Assignees.Nodes {
		if assignee.Login != "" {
			pr.Assignees = append(pr.Assignees, assignee.Login)
		}
	}
	for _, thread := range v.ReviewThreads.Nodes {
		if !thread.IsResolved {
			pr.UnresolvedThreads++
		}
	}
	for _, review := range v.Reviews.Nodes {
		pr.Reviews = append(pr.Reviews, review.model())
	}
	if len(v.Commits.Nodes) > 0 {
		if rollup := v.Commits.Nodes[0].Commit.StatusCheckRollup; rollup != nil {
			pr.CheckState = rollup.State
		}
	}
	return pr
}

func (v reviewJSON) model() Review {
	review := Review{
		ID: v.ID, URL: v.URL, Body: v.Body, State: v.State,
		CreatedAt: v.CreatedAt,
	}
	if v.Author != nil {
		review.Author = v.Author.Login
	}
	return review
}

// Repository loads repository permissions, the default branch and the
// signed-in account in one GraphQL request.
func (c *Client) Repository(ctx context.Context, repo Coordinates) (Repository, Viewer, error) {
	var response struct {
		Viewer struct {
			Login     string `json:"login"`
			Name      string `json:"name"`
			AvatarURL string `json:"avatarUrl"`
		} `json:"viewer"`
		Repository *struct {
			ID               string `json:"id"`
			NameWithOwner    string `json:"nameWithOwner"`
			ViewerPermission string `json:"viewerPermission"`
			MergeCommit      bool   `json:"mergeCommitAllowed"`
			SquashMerge      bool   `json:"squashMergeAllowed"`
			RebaseMerge      bool   `json:"rebaseMergeAllowed"`
			DefaultBranchRef *struct {
				Name string `json:"name"`
			} `json:"defaultBranchRef"`
			URL string `json:"url"`
		} `json:"repository"`
	}
	if err := c.GraphQL(ctx, repositoryQuery, map[string]any{
		"owner": repo.Owner, "name": repo.Name,
	}, &response); err != nil {
		return Repository{}, Viewer{}, err
	}
	if response.Repository == nil {
		return Repository{}, Viewer{}, fmt.Errorf("%w: repository %s", ErrNotFound, repo)
	}
	viewer := Viewer{
		Login: response.Viewer.Login, Name: response.Viewer.Name,
		AvatarURL: response.Viewer.AvatarURL,
	}
	repository := Repository{
		ID: response.Repository.ID, Coordinates: repo,
		ViewerLogin: viewer.Login, Permission: response.Repository.ViewerPermission,
		URL: response.Repository.URL, MergeCommits: response.Repository.MergeCommit,
		SquashMerge: response.Repository.SquashMerge,
		RebaseMerge: response.Repository.RebaseMerge,
	}
	if response.Repository.DefaultBranchRef != nil {
		repository.DefaultBranch = response.Repository.DefaultBranchRef.Name
	}
	return repository, viewer, nil
}

// SearchPullRequests loads one bounded cursor page from GitHub search.
func (c *Client) SearchPullRequests(
	ctx context.Context,
	repo Coordinates,
	filters SearchFilters,
	cursor string,
) (SearchPage, error) {
	var response struct {
		Search struct {
			IssueCount int          `json:"issueCount"`
			PageInfo   pageInfoJSON `json:"pageInfo"`
			Nodes      []struct {
				TypeName string `json:"__typename"`
				summaryJSON
			} `json:"nodes"`
		} `json:"search"`
	}
	variables := map[string]any{
		"query": filters.Query(repo), "pageSize": c.pageSize,
	}
	if cursor != "" {
		variables["cursor"] = cursor
	}
	if err := c.GraphQL(ctx, searchPullRequestsQuery, variables, &response); err != nil {
		return SearchPage{}, err
	}
	page := SearchPage{Total: response.Search.IssueCount}
	if response.Search.PageInfo.EndCursor != nil {
		page.EndCursor = *response.Search.PageInfo.EndCursor
	}
	page.HasMore = response.Search.PageInfo.HasNextPage
	for _, node := range response.Search.Nodes {
		if node.TypeName == "PullRequest" && node.Number > 0 {
			page.PullRequests = append(page.PullRequests, node.summaryJSON.model())
		}
	}
	return page, nil
}

// PullRequest loads details, checks, files, reviews and the first timeline
// page for one pull request.
func (c *Client) PullRequest(
	ctx context.Context,
	repo Coordinates,
	number int,
) (PullRequestDetail, error) {
	var response struct {
		Repository *struct {
			PullRequest *struct {
				summaryJSON
				Body                string `json:"body"`
				MergeStateStatus    string `json:"mergeStateStatus"`
				CanBeRebased        bool   `json:"canBeRebased"`
				ViewerCanUpdate     bool   `json:"viewerCanUpdate"`
				ViewerDidAuthor     bool   `json:"viewerDidAuthor"`
				ViewerCanDeleteHead bool   `json:"viewerCanDeleteHeadRef"`
				BaseRefName         string `json:"baseRefName"`
				BaseRefOID          string `json:"baseRefOid"`
				BaseRepository      *struct {
					URL string `json:"url"`
				} `json:"baseRepository"`
				HeadRefName    string `json:"headRefName"`
				HeadRefOID     string `json:"headRefOid"`
				HeadRepository *struct {
					URL string `json:"url"`
				} `json:"headRepository"`
				Files struct {
					Nodes []struct {
						Path              string `json:"path"`
						ChangeType        string `json:"changeType"`
						Additions         int    `json:"additions"`
						Deletions         int    `json:"deletions"`
						ViewerViewedState string `json:"viewerViewedState"`
					} `json:"nodes"`
				} `json:"files"`
				Commits struct {
					Nodes []struct {
						URL    string `json:"url"`
						Commit struct {
							OID            string `json:"oid"`
							AbbreviatedOID string `json:"abbreviatedOid"`
							Headline       string `json:"messageHeadline"`
							CommittedDate  string `json:"committedDate"`
							Author         *struct {
								Name string     `json:"name"`
								User *actorJSON `json:"user"`
							} `json:"author"`
							StatusCheckRollup *statusRollupJSON `json:"statusCheckRollup"`
						} `json:"commit"`
					} `json:"nodes"`
				} `json:"commits"`
				TimelineItems struct {
					Nodes []struct {
						TypeName          string     `json:"__typename"`
						URL               string     `json:"url"`
						Body              string     `json:"body"`
						State             string     `json:"state"`
						CreatedAt         string     `json:"createdAt"`
						DismissalMessage  string     `json:"dismissalMessage"`
						Actor             *actorJSON `json:"actor"`
						Author            *actorJSON `json:"author"`
						RequestedReviewer *actorJSON `json:"requestedReviewer"`
						Label             *struct {
							Name string `json:"name"`
						} `json:"label"`
						Commit *struct {
							OID            string `json:"oid"`
							AbbreviatedOID string `json:"abbreviatedOid"`
							Headline       string `json:"messageHeadline"`
							CommittedDate  string `json:"committedDate"`
							Author         *struct {
								Name string     `json:"name"`
								User *actorJSON `json:"user"`
							} `json:"author"`
						} `json:"commit"`
					} `json:"nodes"`
				} `json:"timelineItems"`
			} `json:"pullRequest"`
		} `json:"repository"`
	}
	if err := c.GraphQL(ctx, pullRequestQuery, map[string]any{
		"owner": repo.Owner, "name": repo.Name, "number": number,
	}, &response); err != nil {
		return PullRequestDetail{}, err
	}
	if response.Repository == nil || response.Repository.PullRequest == nil {
		return PullRequestDetail{}, fmt.Errorf("%w: pull request #%d", ErrNotFound, number)
	}
	raw := response.Repository.PullRequest
	detail := PullRequestDetail{
		PullRequestSummary: raw.summaryJSON.model(),
		Body:               raw.Body, BaseRef: raw.BaseRefName, BaseOID: raw.BaseRefOID,
		HeadRef: raw.HeadRefName, HeadOID: raw.HeadRefOID,
		MergeStateStatus: raw.MergeStateStatus, CanBeRebased: raw.CanBeRebased,
		ViewerCanUpdate: raw.ViewerCanUpdate, ViewerDidAuthor: raw.ViewerDidAuthor,
		ViewerCanDeleteHead: raw.ViewerCanDeleteHead,
	}
	if raw.BaseRepository != nil {
		detail.BaseRepositoryURL = raw.BaseRepository.URL
	}
	if raw.HeadRepository != nil {
		detail.HeadRepositoryURL = raw.HeadRepository.URL
	}
	for _, file := range raw.Files.Nodes {
		detail.Files = append(detail.Files, FileChange{
			Path: file.Path, ChangeType: file.ChangeType,
			Additions: file.Additions, Deletions: file.Deletions,
			ViewedState:  file.ViewerViewedState,
			ViewerViewed: file.ViewerViewedState == "VIEWED",
		})
	}
	for _, node := range raw.Commits.Nodes {
		commit := Commit{
			OID: node.Commit.OID, ShortOID: node.Commit.AbbreviatedOID,
			Headline: node.Commit.Headline, CommittedAt: node.Commit.CommittedDate,
			URL: node.URL,
		}
		if node.Commit.Author != nil {
			commit.Author = node.Commit.Author.Name
			if node.Commit.Author.User != nil {
				commit.Author = node.Commit.Author.User.Login
			}
		}
		detail.Commits = append(detail.Commits, commit)
		if node.Commit.StatusCheckRollup != nil {
			for _, context := range node.Commit.StatusCheckRollup.Contexts.Nodes {
				check := Check{
					State: context.State, Status: context.Status,
					Conclusion: context.Conclusion,
				}
				if context.TypeName == "CheckRun" {
					check.Name = context.Name
					check.URL = context.DetailsURL
					if context.CheckSuite != nil && context.CheckSuite.WorkflowRun != nil &&
						context.CheckSuite.WorkflowRun.Workflow != nil {
						check.Workflow = context.CheckSuite.WorkflowRun.Workflow.Name
					}
				} else {
					check.Name = context.Context
					check.URL = context.TargetURL
				}
				detail.Checks = append(detail.Checks, check)
			}
		}
	}
	for _, node := range raw.TimelineItems.Nodes {
		item := timelineItem(node)
		if item.Kind != "" {
			detail.Timeline = append(detail.Timeline, item)
		}
	}
	return detail, nil
}

func timelineItem(node struct {
	TypeName          string     `json:"__typename"`
	URL               string     `json:"url"`
	Body              string     `json:"body"`
	State             string     `json:"state"`
	CreatedAt         string     `json:"createdAt"`
	DismissalMessage  string     `json:"dismissalMessage"`
	Actor             *actorJSON `json:"actor"`
	Author            *actorJSON `json:"author"`
	RequestedReviewer *actorJSON `json:"requestedReviewer"`
	Label             *struct {
		Name string `json:"name"`
	} `json:"label"`
	Commit *struct {
		OID            string `json:"oid"`
		AbbreviatedOID string `json:"abbreviatedOid"`
		Headline       string `json:"messageHeadline"`
		CommittedDate  string `json:"committedDate"`
		Author         *struct {
			Name string     `json:"name"`
			User *actorJSON `json:"user"`
		} `json:"author"`
	} `json:"commit"`
}) TimelineItem {
	item := TimelineItem{
		Kind: node.TypeName, CreatedAt: node.CreatedAt, URL: node.URL,
		Body: node.Body, State: node.State,
	}
	switch {
	case node.Actor != nil:
		item.Actor = node.Actor.Login
	case node.Author != nil:
		item.Actor = node.Author.Login
	}
	switch node.TypeName {
	case "PullRequestCommit":
		if node.Commit != nil {
			item.Kind = "commit"
			item.Body = node.Commit.Headline
			item.CreatedAt = node.Commit.CommittedDate
			if node.Commit.Author != nil {
				item.Actor = node.Commit.Author.Name
				if node.Commit.Author.User != nil {
					item.Actor = node.Commit.Author.User.Login
				}
			}
		}
	case "PullRequestReview":
		item.Kind = "review"
	case "IssueComment":
		item.Kind = "comment"
	case "ReviewRequestedEvent", "ReviewRequestRemovedEvent":
		if node.RequestedReviewer != nil {
			item.Actor = node.RequestedReviewer.Login
			if item.Actor == "" {
				item.Actor = node.RequestedReviewer.Name
			}
		}
	case "LabeledEvent", "UnlabeledEvent":
		if node.Label != nil {
			item.Label = node.Label.Name
		}
	case "ReviewDismissedEvent":
		item.Body = node.DismissalMessage
	}
	return item
}

// Branches returns the first 100 branches, enough for an IDE create form and
// below the API's response bound.
func (c *Client) Branches(ctx context.Context, repo Coordinates) ([]Branch, error) {
	endpoint := fmt.Sprintf(
		"repos/%s/%s/branches?per_page=100",
		url.PathEscape(repo.Owner), url.PathEscape(repo.Name),
	)
	var response []struct {
		Name string `json:"name"`
	}
	if err := c.REST(ctx, http.MethodGet, endpoint, nil, &response); err != nil {
		return nil, err
	}
	branches := make([]Branch, 0, len(response))
	for _, item := range response {
		if item.Name != "" {
			branches = append(branches, Branch{Name: item.Name})
		}
	}
	return branches, nil
}

// CreatePullRequest opens a pull request for branches already pushed to
// GitHub. The returned summary is the new REST resource.
func (c *Client) CreatePullRequest(
	ctx context.Context,
	repo Coordinates,
	input CreatePullRequestInput,
) (PullRequestSummary, error) {
	if strings.TrimSpace(input.Title) == "" || input.Base == "" || input.Head == "" {
		return PullRequestSummary{}, fmt.Errorf("title, base branch and head branch are required")
	}
	if input.Base == input.Head {
		return PullRequestSummary{}, fmt.Errorf("base and head branches must differ")
	}
	endpoint := fmt.Sprintf(
		"repos/%s/%s/pulls",
		url.PathEscape(repo.Owner), url.PathEscape(repo.Name),
	)
	var response struct {
		ID        int64  `json:"id"`
		NodeID    string `json:"node_id"`
		HTMLURL   string `json:"html_url"`
		Number    int    `json:"number"`
		Title     string `json:"title"`
		State     string `json:"state"`
		Draft     bool   `json:"draft"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
		User      *struct {
			Login     string `json:"login"`
			AvatarURL string `json:"avatar_url"`
		} `json:"user"`
	}
	err := c.REST(ctx, http.MethodPost, endpoint, map[string]any{
		"title": input.Title,
		"body":  input.Body,
		"base":  input.Base,
		"head":  input.Head,
		"draft": input.Draft,
	}, &response)
	if err != nil {
		return PullRequestSummary{}, err
	}
	summary := PullRequestSummary{
		ID: response.NodeID, Number: response.Number, Title: response.Title,
		URL: response.HTMLURL, State: strings.ToUpper(response.State),
		Draft: response.Draft, CreatedAt: response.CreatedAt,
		UpdatedAt: response.UpdatedAt,
	}
	if response.User != nil {
		summary.Author = response.User.Login
		summary.AuthorAvatarURL = response.User.AvatarURL
	}
	return summary, nil
}

// UpdateBranch merges the base branch into the pull request head when GitHub
// allows it.
func (c *Client) UpdateBranch(ctx context.Context, repo Coordinates, number int, headOID string) error {
	endpoint := fmt.Sprintf(
		"repos/%s/%s/pulls/%d/update-branch",
		url.PathEscape(repo.Owner), url.PathEscape(repo.Name), number,
	)
	body := map[string]any{}
	if headOID != "" {
		body["expected_head_sha"] = headOID
	}
	return c.REST(ctx, http.MethodPut, endpoint, body, nil)
}

// SetFileViewed changes one file's viewed state through GitHub's GraphQL
// mutation.
func (c *Client) SetFileViewed(
	ctx context.Context,
	pullRequestID, path string,
	viewed bool,
) error {
	if pullRequestID == "" || path == "" {
		return fmt.Errorf("pull request id and file path are required")
	}
	query := unmarkFileViewedMutation
	if viewed {
		query = markFileViewedMutation
	}
	var response map[string]any
	return c.GraphQL(ctx, query, map[string]any{
		"pullRequestId": pullRequestID,
		"path":          path,
	}, &response)
}
