package github

import (
	"context"
	"errors"
	"strings"
)

var (
	// ErrNoToken means the OS credential store has no GitHub token.
	ErrNoToken = errors.New("GitHub token is not configured")
	// ErrUnauthorized means GitHub rejected the configured token.
	ErrUnauthorized = errors.New("GitHub rejected the token")
	// ErrNotFound means the repository or pull request does not exist.
	ErrNotFound = errors.New("GitHub item not found")
)

// Credentials supplies the token for requests. The app backs it with the OS
// credential store so the token is never written to preferences or a file.
type Credentials interface {
	Token(context.Context) (string, error)
}

// Viewer is the signed-in GitHub account.
type Viewer struct {
	Login     string
	Name      string
	AvatarURL string
}

// Repository is the GitHub repository behind the project's preferred remote.
type Repository struct {
	ID            string
	Coordinates   Coordinates
	ViewerLogin   string
	Permission    string
	DefaultBranch string
	URL           string
	MergeCommits  bool
	SquashMerge   bool
	RebaseMerge   bool
}

// Label is a pull request label.
type Label struct {
	Name  string
	Color string
}

// Review is a submitted pull request review.
type Review struct {
	ID        string
	Author    string
	Body      string
	State     string
	CreatedAt string
	URL       string
}

// Check is one required check run or commit status on the pull request head.
type Check struct {
	Name       string
	Workflow   string
	State      string
	Status     string
	Conclusion string
	URL        string
}

// FileChange is one changed file in a pull request.
type FileChange struct {
	Path         string
	PreviousPath string
	ChangeType   string
	Additions    int
	Deletions    int
	Patch        string
	ViewedState  string
	ViewerViewed bool
}

// ReviewComment is one comment in a pull request review thread.
type ReviewComment struct {
	ID              string
	Author          string
	Body            string
	State           string
	CreatedAt       string
	URL             string
	DiffHunk        string
	ViewerCanEdit   bool
	ViewerCanDelete bool
}

// ReviewThread is a line or file conversation attached to a pull request diff.
type ReviewThread struct {
	ID                 string
	Path               string
	SubjectType        string
	IsResolved         bool
	IsOutdated         bool
	DiffSide           string
	Line               int
	OriginalLine       int
	StartDiffSide      string
	StartLine          int
	OriginalStartLine  int
	Comments           []ReviewComment
	ViewerCanReply     bool
	ViewerCanResolve   bool
	ViewerCanUnresolve bool
}

// PendingReview is the viewer's unsubmitted review.
type PendingReview struct {
	ID            string
	State         string
	CommentsCount int
}

// Commit is one pull request commit.
type Commit struct {
	OID         string
	ShortOID    string
	Headline    string
	Author      string
	CommittedAt string
	URL         string
}

// TimelineItem is one entry in the pull request conversation.
type TimelineItem struct {
	Kind      string
	Actor     string
	Body      string
	State     string
	CreatedAt string
	URL       string
	Label     string
}

// PullRequestSummary is a row in the pull request list.
type PullRequestSummary struct {
	ID                string
	Number            int
	Title             string
	URL               string
	State             string
	Draft             bool
	Author            string
	AuthorAvatarURL   string
	CreatedAt         string
	UpdatedAt         string
	ReviewDecision    string
	Mergeable         string
	Unread            bool
	Labels            []Label
	Assignees         []string
	CommentCount      int
	UnresolvedThreads int
	CheckState        string
	Reviews           []Review
}

// PullRequestDetail is everything the native detail page draws.
type PullRequestDetail struct {
	PullRequestSummary
	Body                string
	BaseRef             string
	BaseOID             string
	BaseRepositoryURL   string
	HeadRef             string
	HeadOID             string
	HeadRepositoryURL   string
	MergeStateStatus    string
	ViewerCanUpdate     bool
	ViewerDidAuthor     bool
	ViewerCanDeleteHead bool
	CanBeRebased        bool
	Files               []FileChange
	Commits             []Commit
	Checks              []Check
	Timeline            []TimelineItem
	Threads             []ReviewThread
	PendingReview       PendingReview
	DiffLoaded          bool
	ThreadsLoaded       bool
}

// SearchPage is one cursor-paged search response.
type SearchPage struct {
	PullRequests []PullRequestSummary
	Total        int
	EndCursor    string
	HasMore      bool
}

// SearchFilters mirrors OpenIDEA's pull request search value.
type SearchFilters struct {
	Text        string
	State       string
	Assignee    string
	ReviewState string
	Author      string
	Label       string
	Sort        string
}

// DefaultSearchFilters is OpenIDEA's initial "Open" quick filter.
func DefaultSearchFilters() SearchFilters {
	return SearchFilters{State: "OPEN"}
}

var searchStates = map[string]string{
	"OPEN":   "is:open",
	"CLOSED": "is:closed",
	"MERGED": "is:merged",
}

var reviewStates = map[string]string{
	"NO_REVIEW":          "review:none",
	"REQUIRED":           "review:required",
	"APPROVED":           "review:approved",
	"CHANGES_REQUESTED":  "review:changes-requested",
	"REVIEWED_BY_ME":     "reviewed-by:@me",
	"NOT_REVIEWED_BY_ME": "-reviewed-by:@me",
	"AWAITING_REVIEW":    "review-requested:@me",
}

var searchSorts = map[string]string{
	"NEWEST":                 "sort:created-desc",
	"OLDEST":                 "sort:created-asc",
	"MOST_COMMENTED":         "sort:comments-desc",
	"LEAST_COMMENTED":        "sort:comments-asc",
	"RECENTLY_UPDATED":       "sort:updated-desc",
	"LEAST_RECENTLY_UPDATED": "sort:updated-asc",
}

func (f SearchFilters) FilterCount() int {
	count := 0
	for _, value := range []string{
		f.Text, f.State, f.Assignee, f.ReviewState, f.Author, f.Label, f.Sort,
	} {
		if strings.TrimSpace(value) != "" {
			count++
		}
	}
	return count
}

// Terms returns the GitHub search terms in OpenIDEA's order.
func (f SearchFilters) Terms() string {
	var terms []string
	if text := strings.Join(strings.Fields(f.Text), " "); text != "" {
		terms = append(terms, "("+text+")")
	}
	if term := searchStates[f.State]; term != "" {
		terms = append(terms, term)
	}
	if login := cleanSearchLogin(f.Assignee); login != "" {
		terms = append(terms, "assignee:"+login)
	}
	if term := reviewStates[f.ReviewState]; term != "" {
		terms = append(terms, term)
	}
	if login := cleanSearchLogin(f.Author); login != "" {
		terms = append(terms, "author:"+login)
	}
	if label := cleanSearchLabel(f.Label); label != "" {
		label = strings.ReplaceAll(label, `\`, `\\`)
		label = strings.ReplaceAll(label, `"`, `\"`)
		terms = append(terms, `label:"`+label+`"`)
	}
	if term := searchSorts[f.Sort]; term != "" {
		terms = append(terms, term)
	}
	return strings.Join(terms, " ")
}

// Query prefixes the terms with the repository and item type.
func (f SearchFilters) Query(repo Coordinates) string {
	query := "type:pr repo:" + repo.String()
	if terms := f.Terms(); terms != "" {
		query += " " + terms
	}
	return query
}

func cleanSearchLogin(value string) string {
	return strings.Join(strings.Fields(value), "")
}

func cleanSearchLabel(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func (p PullRequestSummary) IsOpen() bool {
	return strings.EqualFold(p.State, "OPEN")
}

func (p PullRequestSummary) IsApproved() bool {
	return strings.EqualFold(p.ReviewDecision, "APPROVED")
}

// Branch is a branch that can be used as a pull request base or head.
type Branch struct {
	Name string
}

// CreatePullRequestInput is the create form's request.
type CreatePullRequestInput struct {
	Title string
	Body  string
	Base  string
	Head  string
	Draft bool
}

// BranchPullRequest identifies the open pull request whose head is a local
// branch, used by Review Mode.
type BranchPullRequest struct {
	Number int
	Title  string
	URL    string
}

// ReviewThreadInput is a single-line or multiline review comment.
type ReviewThreadInput struct {
	Path      string
	Body      string
	Side      string
	Line      int
	StartSide string
	StartLine int
}

// CreateReviewInput creates a pending review, a submitted general review, or
// a review containing one or more inline threads.
type CreateReviewInput struct {
	PullRequestID string
	Event         string
	Body          string
	CommitOID     string
	Threads       []ReviewThreadInput
}

// PullRequestMetadataInput changes the editable pull request fields.
type PullRequestMetadataInput struct {
	Title string
	Body  string
}
