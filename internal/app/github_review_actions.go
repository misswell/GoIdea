package app

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

func init() {
	registerStatusItem(&StatusItem{
		ID: "status.githubReview", Title: "GitHub Review", Side: "right", Order: 4,
		Visible: func(m *Model) bool {
			return m.github.review.branchNumber > 0 && m.Root() != ""
		},
		Render: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) {
			number := m.github.review.branchNumber
			label := fmt.Sprintf("#%d Review", number)
			if m.github.review.enabled {
				label = fmt.Sprintf("#%d Review On", number)
			}
			button := ui.ButtonBase(c.Key("status:github-review")).Height(22).
				Padding(0, 6).Radius(4).Gap(4).AlignItems(ui.Center).
				Cursor(ui.CursorPointer)
			if button.Hovered() {
				button.Background(oi.toolbarHover)
			}
			title := m.github.review.branchTitle
			if title == "" {
				title = fmt.Sprintf("Pull request #%d", number)
			}
			button.Tooltip(title + " on " + m.github.currentBranch)
			button.Children(func() {
				oiIconEl(c, "vcs/vendors/github", 14, oi, oi.textMuted)
				ui.Text(c, label).FontSize(12).TextColor(oi.textMuted).SingleLine()
			})
			if button.Clicked() {
				m.OpenCurrentGitHubPullRequest()
			}
		},
	})

	available := func(m *Model) bool {
		return m.Root() != "" && !m.showingWelcome
	}
	detailAvailable := func(m *Model) bool {
		return available(m) && m.github.detail != nil
	}
	register := func(action *Action) {
		_ = registerAction(action)
	}
	register(&Action{
		ID: "github.pullRequest.reviewMode", Title: "Review Mode", Category: "GitHub",
		Keywords: "github review current branch pull request",
		When:     available, Enabled: func(m *Model) bool {
			return !m.github.connecting && !m.github.review.branchLoading
		},
		Toggled: func(m *Model) bool { return m.github.review.enabled },
		Running: func(m *Model) bool { return m.github.review.branchLoading },
		Run:     func(m *Model) { m.ToggleGitHubReviewMode() },
	})
	register(&Action{
		ID: "github.pullRequest.show", Title: "Show Current Branch Pull Request",
		Category: "GitHub", Keywords: "github review current branch show pull request",
		When: func(m *Model) bool { return available(m) && m.github.review.branchNumber > 0 },
		Run:  func(m *Model) { m.OpenCurrentGitHubPullRequest() },
	})
	register(&Action{
		ID: "github.pullRequest.submitReview", Title: "Submit Review…", Category: "GitHub",
		Keywords: "github approve request changes comment review",
		When:     detailAvailable,
		Enabled: func(m *Model) bool {
			return m.github.connected() && !m.github.review.submit.submitting &&
				!m.github.review.pendingLoading && m.github.review.pendingErr == ""
		},
		Running: func(m *Model) bool { return m.github.review.submit.submitting },
		Run:     func(m *Model) { m.OpenGitHubReviewSubmit() },
	})
	register(&Action{
		ID: "github.pullRequest.diff.addComment", Title: "Add Review Comment",
		Category: "GitHub", Keywords: "github diff line single comment",
		When: detailAvailable,
		Enabled: func(m *Model) bool {
			_, ok := m.selectedGitHubDiffRow()
			return ok
		},
		Run: func(m *Model) { m.BeginGitHubReviewComment(false) },
	})
	register(&Action{
		ID:    "github.pullRequest.diff.addMultilineComment",
		Title: "Add Multiline Review Comment", Category: "GitHub",
		Keywords: "github diff range lines multiline comment",
		When:     detailAvailable,
		Enabled: func(m *Model) bool {
			_, ok := m.selectedGitHubDiffRow()
			return ok
		},
		Run: func(m *Model) { m.BeginGitHubReviewComment(true) },
	})
	register(&Action{
		ID: "github.pullRequest.diff.nextComment", Title: "Next Comment",
		Category: "GitHub", Keywords: "github review next thread comment",
		When: detailAvailable,
		Enabled: func(m *Model) bool {
			return len(m.github.review.threads) > 0
		},
		Run: func(m *Model) { m.StepGitHubReviewComment(1) },
	})
	register(&Action{
		ID: "github.pullRequest.diff.previousComment", Title: "Previous Comment",
		Category: "GitHub", Keywords: "github review previous thread comment",
		When: detailAvailable,
		Enabled: func(m *Model) bool {
			return len(m.github.review.threads) > 0
		},
		Run: func(m *Model) { m.StepGitHubReviewComment(-1) },
	})
	register(&Action{
		ID: "github.pullRequest.diff.resolveThread", Title: "Resolve Review Thread",
		Category: "GitHub", Keywords: "github review conversation resolve",
		When: detailAvailable,
		RunWithArg: func(m *Model, arg any) {
			index, ok := actionInt(arg)
			if !ok {
				m.SetSelectedGitHubReviewThreadResolved(true)
				return
			}
			m.toggleGitHubReviewThread(index, true)
		},
	})
	register(&Action{
		ID: "github.pullRequest.diff.unresolveThread", Title: "Reopen Review Thread",
		Category: "GitHub", Keywords: "github review conversation reopen unresolve",
		When: detailAvailable,
		RunWithArg: func(m *Model, arg any) {
			index, ok := actionInt(arg)
			if !ok {
				m.SetSelectedGitHubReviewThreadResolved(false)
				return
			}
			m.toggleGitHubReviewThread(index, false)
		},
	})
	register(&Action{
		ID: "github.pullRequest.editMetadata", Title: "Edit Pull Request Fields…",
		Category: "GitHub", Keywords: "github edit title description base branch",
		When: func(m *Model) bool {
			return detailAvailable(m) && m.github.detail.ViewerCanUpdate
		},
		Run: func(m *Model) { m.OpenGitHubMetadataEditor() },
	})
	register(&Action{
		ID: "github.pullRequest.reviewMode.showDiff", Title: "Show Review Diff",
		Category: "GitHub", Keywords: "github review mode diff file",
		When: detailAvailable,
		Run: func(m *Model) {
			m.github.review.mode = "review"
			if m.github.selectedFile < 0 && m.github.detail != nil && len(m.github.detail.Files) > 0 {
				m.github.selectedFile = 0
			}
			m.OpenGitHubPullRequests()
			m.invalidate()
		},
	})
}
