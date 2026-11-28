package app

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/egoist/mygo/ui"

	ghub "mayoi/internal/github"
)

const githubTokenURL = "https://github.com/settings/tokens/new?scopes=repo&description=GoIDEA"

func init() {
	registerToolWindow(&toolWindowSpec{
		id: "pullRequests", title: "Pull Requests",
		icon: "toolwindows/pullRequests", anchor: dockLeft, order: 5, stripe: true,
		badge: func(m *Model) int { return githubToolWindowBadge(&m.github) },
		open: func(m *Model) {
			m.github.visible = true
			m.leftHidden = false
			m.openGitHubContent()
		},
		close: func(m *Model) { m.github.visible = false },
		shown: func(m *Model) bool { return m.github.visible },
		render: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) {
			m.githubPanel(c, p, oi)
		},
	})
}

func (m *Model) githubPanel(c *ui.Context, p *ui.Theme, oi oiColors) {
	beforeRoot := m.github.root
	m.ensureGitHubState()
	if beforeRoot != "" && beforeRoot != m.github.root && m.github.visible {
		m.connectGitHub(true)
	}
	ui.Column(c.Key("github:panel")).Grow(1).MinWidth(0).MinHeight(0).
		Background(m.toolWindowBackground(p)).
		BorderWidth(1, 0, 0, 0).BorderColor(p.Border).Children(func() {
		m.githubHeader(c, p, oi)
		switch {
		case m.github.needsToken:
			m.githubTokenBody(c, p, oi)
		case m.github.connecting && !m.github.connected():
			githubLoading(c, oi, "Connecting to GitHub…")
		case m.github.connectErr != "" && !m.github.connected():
			m.githubConnectionError(c, p, oi)
		case m.github.view == "detail":
			m.githubDetailBody(c, p, oi)
		case m.github.view == "create":
			m.githubCreateBody(c, p, oi)
		default:
			m.githubListBody(c, p, oi)
		}
	})
}

func (m *Model) githubHeader(c *ui.Context, p *ui.Theme, oi oiColors) {
	ui.Row(c.Key("github:header")).Height(34).Shrink(0).Padding(0, 5).Gap(3).
		AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(oi.border).
		Children(func() {
			if m.github.view != "list" {
				m.httpToolbarButton(c, p, "general/chevronLeft", "Back to Pull Requests",
					false, m.BackToGitHubList)
			}
			if m.github.view == "list" && m.github.connected() {
				m.httpToolbarButton(c, p, "general/add", "Create Pull Request",
					!m.github.connected() || m.github.create.creating, m.OpenGitHubCreate)
			}
			if m.github.view != "create" {
				m.httpToolbarButton(c, p, "general/refresh", "Refresh",
					!m.github.connected() || m.github.connecting || m.github.listLoading ||
						m.github.detailLoading, m.refreshGitHubCurrentView)
			}
			ui.Spacer(c)
			title := "Pull Requests"
			if m.github.view == "detail" && m.github.selectedNumber > 0 {
				title = fmt.Sprintf("Pull Request #%d", m.github.selectedNumber)
			} else if m.github.view == "create" {
				title = "Create Pull Request"
			}
			ui.Text(c, title).FontSize(12).Bold().TextColor(oi.text).
				SingleLine().Ellipsis("…")
			if m.github.connected() {
				ui.Text(c, m.github.coordinates.String()).FontSize(11).
					TextColor(oi.textMuted).SingleLine().Ellipsis("…").
					Tooltip(m.github.coordinates.String() + " · " + m.github.viewer.Login)
			}
			if m.github.connected() || m.github.needsToken {
				ui.ButtonBase(c.Key("github:account")).Height(24).Padding(0, 6).
					Radius(4).Label("GitHub account").Tooltip("GitHub account").Children(func() {
					oiIconEl(c, "vcs/vendors/github", 16, oi, oi.textMuted)
				}).Menu(func(menu *ui.Menu) {
					if m.github.viewer.Login != "" {
						menu.Item("Signed in as " + m.github.viewer.Login).Disabled(true)
					}
					if len(m.github.remotes) > 1 {
						menu.Submenu("Repository", func(submenu *ui.Menu) {
							for _, remote := range m.github.remotes {
								label := remote.Coordinates.String()
								if remote.Remote != "" &&
									remote.Remote != remote.Coordinates.Name {
									label = remote.Remote + " · " + label
								}
								if submenu.Item(label).
									Checked(remote.Remote == m.github.selectedRemote).
									Chosen() {
									m.selectGitHubRepository(remote.Remote)
								}
							}
						})
					}
					if menu.Item("GitHub Token Settings…").Chosen() {
						c.OpenURL(githubTokenURL)
					}
					if menu.Item("Remove Saved Token").Chosen() {
						m.ClearGitHubToken()
					}
				})
			}
		})
}

func (m *Model) githubTokenBody(c *ui.Context, p *ui.Theme, oi oiColors) {
	ui.Scroll(c.Key("github:token-scroll")).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c.Key("github:token")).Grow(1).Padding(24).Gap(10).
			AlignItems(ui.Center).Justify(ui.Center).Children(func() {
			oiIconEl(c, "vcs/vendors/github", 40, oi, oi.textMuted)
			ui.Text(c, "Connect to GitHub").FontSize(15).Bold().TextColor(oi.text)
			ui.Text(c, "Use a personal access token with repository access. "+
				"GoIDEA stores it in the OS credential store.").
				FontSize(12).TextColor(oi.textMuted)
			if m.github.tokenErr != "" {
				ui.Text(c, m.github.tokenErr).FontSize(12).TextColor(oi.danger)
			}
			input := ui.TextInput(c.Key("github:token-input"), &m.github.tokenDraft).
				Width(300).AutoFocus().Password().
				Placeholder("github_pat_… or ghp_…")
			input.OnTextKey(func(key ui.TextKey) bool {
				if key.Key != ui.KeyEnter || key.Mods != 0 {
					return false
				}
				m.SaveGitHubToken()
				return true
			})
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Button(c, "Save Token").Disabled(m.github.tokenSaving ||
					strings.TrimSpace(m.github.tokenDraft) == "").OnClick(m.SaveGitHubToken)
				ui.Button(c, "Create a Token…").OnClick(func() {
					c.OpenURL(githubTokenURL)
				})
			})
			ui.Text(c, "The token is sent only to api.github.com and is never written to settings files.").
				FontSize(11).TextColor(oi.textFaint)
		})
	})
}

func (m *Model) githubConnectionError(c *ui.Context, p *ui.Theme, oi oiColors) {
	ui.Column(c.Key("github:connection-error")).Grow(1).Padding(24).Gap(9).
		AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		oiIconEl(c, "github/githubWarning", 32, oi, oi.danger)
		ui.Text(c, "Pull Requests Unavailable").FontSize(14).Bold().TextColor(oi.text)
		ui.Text(c, m.github.connectErr).FontSize(12).TextColor(oi.textMuted)
		ui.Button(c, "Retry").OnClick(func() { m.connectGitHub(true) })
	})
}

func githubLoading(c *ui.Context, oi oiColors, message string) {
	ui.Column(c.Key("github:loading")).Grow(1).AlignItems(ui.Center).
		Justify(ui.Center).Gap(7).Children(func() {
		oiIconEl(c, "toolwindows/pullRequests", 28, oi, oi.textMuted)
		ui.Text(c, message).FontSize(12).TextColor(oi.textMuted)
	})
}

func (m *Model) githubListBody(c *ui.Context, p *ui.Theme, oi oiColors) {
	ui.Row(c.Key("github:search-row")).Height(38).Shrink(0).Padding(6, 6).
		Gap(4).AlignItems(ui.Center).Children(func() {
		input := ui.TextInput(c.Key("github:search"), &m.github.filters.Text).
			Grow(1).MinWidth(80).Placeholder("Search pull requests")
		input.OnTextKey(func(key ui.TextKey) bool {
			if key.Key != ui.KeyEnter || key.Mods != 0 {
				return false
			}
			m.applyGitHubSearch()
			return true
		})
		filterLabel := "Filters"
		if count := m.github.filters.FilterCount(); count > 1 ||
			(count == 1 && m.github.filters.State == "") {
			filterLabel = fmt.Sprintf("Filters %d", count)
		}
		if ui.Button(c, filterLabel).Clicked() {
			m.github.showFilters = !m.github.showFilters
		}
	})
	ui.Row(c.Key("github:state-row")).Height(32).Shrink(0).Padding(0, 6).
		Gap(3).AlignItems(ui.Center).Children(func() {
		m.githubStateChip(c, p, "OPEN", "Open")
		m.githubStateChip(c, p, "CLOSED", "Closed")
		m.githubStateChip(c, p, "MERGED", "Merged")
		ui.Spacer(c)
		if m.github.listTotal > 0 {
			ui.Text(c, fmt.Sprintf("%d matching", m.github.listTotal)).
				FontSize(11).TextColor(oi.textMuted)
		}
	})
	if m.github.showFilters {
		m.githubFilters(c, p, oi)
	}
	if m.github.listErr != "" {
		githubInlineError(c, p, m.github.listErr, func() { m.loadGitHubList(true) })
	}
	if m.github.listLoading && !m.github.listLoaded {
		githubLoading(c, oi, "Loading pull requests…")
		return
	}
	if len(m.github.items) == 0 {
		ui.Column(c.Key("github:empty-list")).Grow(1).Padding(20).Gap(7).
			AlignItems(ui.Center).Justify(ui.Center).Children(func() {
			oiIconEl(c, "toolwindows/pullRequests", 32, oi, oi.textMuted)
			ui.Text(c, "No pull requests").FontSize(13).Bold().TextColor(oi.text)
			ui.Text(c, "Try another search or clear the filters.").
				FontSize(12).TextColor(oi.textMuted)
		})
		return
	}
	ui.Scroll(c.Key("github:list")).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Padding(3, 4).Gap(1).Children(func() {
			for _, pullRequest := range m.github.items {
				m.githubPullRequestRow(c, p, oi, pullRequest)
			}
			if m.github.listHasMore {
				button := ui.Button(c, "Load More")
				if m.github.listLoading {
					button = ui.Button(c, "Loading…").Disabled(true)
				}
				button.OnClick(m.loadMoreGitHub)
			}
		})
	})
}

func (m *Model) githubStateChip(c *ui.Context, p *ui.Theme, state, label string) {
	oi := oiPaletteOf(p)
	selected := m.github.filters.State == state
	button := ui.ButtonBase(c.Key("github:state:"+state)).Height(24).
		Padding(0, 9).Radius(5).Label(label).Cursor(ui.CursorPointer)
	if selected {
		button.Background(oi.selected).BorderColor(oi.borderStrong)
	} else if button.Hovered() {
		button.Background(oi.hoverTint)
	}
	button.OnClick(func() {
		if selected {
			m.github.filters.State = ""
		} else {
			m.github.filters.State = state
		}
		m.applyGitHubSearch()
	})
	button.Children(func() {
		color := oi.textMuted
		if selected {
			color = oi.text
		}
		ui.Text(c, label).FontSize(11).TextColor(color).SingleLine()
	})
}

func (m *Model) githubFilters(c *ui.Context, p *ui.Theme, oi oiColors) {
	ui.Column(c.Key("github:filters")).Shrink(0).Padding(7, 7).Gap(6).
		BorderWidth(0, 0, 1, 0).BorderColor(oi.border).Children(func() {
		author := ui.TextInput(c.Key("github:filter-author"), &m.github.filters.Author).
			Placeholder("Author login")
		author.OnTextKey(githubApplySearchKey(m))
		assignee := ui.TextInput(c.Key("github:filter-assignee"), &m.github.filters.Assignee).
			Placeholder("Assignee login")
		assignee.OnTextKey(githubApplySearchKey(m))
		label := ui.TextInput(c.Key("github:filter-label"), &m.github.filters.Label).
			Placeholder("Label")
		label.OnTextKey(githubApplySearchKey(m))
		ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
			ui.Button(c, githubReviewFilterLabel(m.github.filters.ReviewState)).
				Menu(func(menu *ui.Menu) { m.githubReviewMenu(menu) })
			ui.Button(c, githubSortLabel(m.github.filters.Sort)).
				Menu(func(menu *ui.Menu) { m.githubSortMenu(menu) })
			ui.Spacer(c)
			ui.Button(c, "Clear").OnClick(func() {
				m.github.filters = ghub.DefaultSearchFilters()
				m.applyGitHubSearch()
			})
			ui.Button(c, "Apply").OnClick(m.applyGitHubSearch)
		})
	})
}

func githubApplySearchKey(m *Model) func(ui.TextKey) bool {
	return func(key ui.TextKey) bool {
		if key.Key != ui.KeyEnter || key.Mods != 0 {
			return false
		}
		m.applyGitHubSearch()
		return true
	}
}

func (m *Model) githubReviewMenu(menu *ui.Menu) {
	options := []struct {
		id   string
		name string
	}{
		{"", "Any review state"},
		{"NO_REVIEW", "No review"},
		{"REQUIRED", "Review required"},
		{"APPROVED", "Approved"},
		{"CHANGES_REQUESTED", "Changes requested"},
		{"REVIEWED_BY_ME", "Reviewed by me"},
		{"NOT_REVIEWED_BY_ME", "Not reviewed by me"},
		{"AWAITING_REVIEW", "Awaiting my review"},
	}
	for _, option := range options {
		if menu.Item(option.name).Checked(m.github.filters.ReviewState == option.id).Chosen() {
			m.github.filters.ReviewState = option.id
			m.applyGitHubSearch()
		}
	}
}

func (m *Model) githubSortMenu(menu *ui.Menu) {
	options := []struct {
		id   string
		name string
	}{
		{"", "Default order"},
		{"NEWEST", "Newest"},
		{"OLDEST", "Oldest"},
		{"MOST_COMMENTED", "Most commented"},
		{"LEAST_COMMENTED", "Least commented"},
		{"RECENTLY_UPDATED", "Recently updated"},
		{"LEAST_RECENTLY_UPDATED", "Least recently updated"},
	}
	for _, option := range options {
		if menu.Item(option.name).Checked(m.github.filters.Sort == option.id).Chosen() {
			m.github.filters.Sort = option.id
			m.applyGitHubSearch()
		}
	}
}

func githubReviewFilterLabel(value string) string {
	switch value {
	case "NO_REVIEW":
		return "No review"
	case "REQUIRED":
		return "Review required"
	case "APPROVED":
		return "Approved"
	case "CHANGES_REQUESTED":
		return "Changes requested"
	case "REVIEWED_BY_ME":
		return "Reviewed by me"
	case "NOT_REVIEWED_BY_ME":
		return "Not reviewed by me"
	case "AWAITING_REVIEW":
		return "Awaiting my review"
	default:
		return "Any review"
	}
}

func githubSortLabel(value string) string {
	switch value {
	case "NEWEST":
		return "Newest"
	case "OLDEST":
		return "Oldest"
	case "MOST_COMMENTED":
		return "Most commented"
	case "LEAST_COMMENTED":
		return "Least commented"
	case "RECENTLY_UPDATED":
		return "Recently updated"
	case "LEAST_RECENTLY_UPDATED":
		return "Least recently updated"
	default:
		return "Default order"
	}
}

func (m *Model) githubPullRequestRow(
	c *ui.Context,
	p *ui.Theme,
	oi oiColors,
	pr ghub.PullRequestSummary,
) {
	selected := m.github.selectedNumber == pr.Number && m.github.view == "detail"
	row := ui.Column(c.Key(fmt.Sprintf("github:pr:%d", pr.Number))).
		Padding(7, 7).Gap(4).Radius(5).Cursor(ui.CursorPointer).
		Role(ui.RoleButton).Focusable().
		Label(fmt.Sprintf("#%d %s", pr.Number, pr.Title))
	if selected {
		row.Background(oi.selected)
	} else if row.Hovered() {
		row.Background(oi.hoverTint)
	}
	row.OnClick(func() { m.showGitHubPullRequestNumber(pr.Number) })
	row.Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			oiIconEl(c, githubPullRequestIcon(pr), 16, oi, githubPullRequestColor(pr, oi))
			ui.Text(c, fmt.Sprintf("#%d", pr.Number)).FontSize(11).
				TextColor(oi.textMuted).SingleLine()
			ui.Text(c, pr.Title).FontSize(12).TextColor(oi.text).
				SingleLine().Ellipsis("…").Grow(1)
			if pr.Draft {
				ui.Text(c, "Draft").FontSize(10).TextColor(oi.textMuted).
					Padding(1, 5).Radius(4).Background(oi.badge)
			}
			if pr.Unread {
				ui.Column(c).Size(6, 6).Radius(3).Background(oi.accent)
			}
		})
		ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
			if pr.Author != "" {
				ui.Text(c, pr.Author).FontSize(11).TextColor(oi.textMuted).SingleLine()
			}
			if decision := githubReviewDecisionLabel(pr.ReviewDecision); decision != "" {
				ui.Text(c, decision).FontSize(11).
					TextColor(githubReviewDecisionColor(pr.ReviewDecision, oi)).SingleLine()
			}
			if state := githubCheckStateLabel(pr.CheckState); state != "" {
				oiIconEl(c, githubCheckIcon(pr.CheckState), 12, oi, githubCheckColor(pr.CheckState, oi))
				ui.Text(c, state).FontSize(11).
					TextColor(githubCheckColor(pr.CheckState, oi)).SingleLine()
			}
			if pr.CommentCount > 0 {
				ui.Text(c, strconv.Itoa(pr.CommentCount)+" comments").
					FontSize(11).TextColor(oi.textMuted).SingleLine()
			}
			if pr.UnresolvedThreads > 0 {
				ui.Text(c, strconv.Itoa(pr.UnresolvedThreads)+" unresolved").
					FontSize(11).TextColor(oi.warning).SingleLine()
			}
		})
		if len(pr.Labels) > 0 {
			names := make([]string, 0, len(pr.Labels))
			for _, label := range pr.Labels {
				names = append(names, label.Name)
			}
			ui.Text(c, strings.Join(names, " · ")).FontSize(10).
				TextColor(oi.textSubtle).SingleLine().Ellipsis("…")
		}
	})
}

func githubPullRequestIcon(pr ghub.PullRequestSummary) string {
	if pr.Draft {
		return "github/pullRequestDraft"
	}
	switch strings.ToUpper(pr.State) {
	case "MERGED":
		return "github/pullRequestMerged"
	case "CLOSED":
		return "github/pullRequestClosed"
	default:
		return "github/pullRequestOpen"
	}
}

func githubPullRequestColor(pr ghub.PullRequestSummary, oi oiColors) ui.Color {
	switch strings.ToUpper(pr.State) {
	case "MERGED":
		return oi.accent
	case "CLOSED":
		return oi.danger
	default:
		if pr.Draft {
			return oi.textMuted
		}
		return oi.success
	}
}

func (m *Model) githubDetailBody(c *ui.Context, p *ui.Theme, oi oiColors) {
	if m.github.detailLoading && m.github.detail == nil {
		githubLoading(c, oi, fmt.Sprintf("Loading pull request #%d…", m.github.selectedNumber))
		return
	}
	if m.github.detailErr != "" {
		githubInlineError(c, p, m.github.detailErr, m.loadGitHubDetail)
	}
	if m.github.detail == nil {
		githubLoading(c, oi, "Pull request details are unavailable")
		return
	}
	detail := m.github.detail
	ui.Scroll(c.Key("github:detail")).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Padding(8, 8).Gap(10).Children(func() {
			m.githubDetailSummary(c, p, oi, detail)
			if m.github.connected() {
				m.githubReviewToolbar(c, p, oi)
			}
			m.githubMetadataEditor(c, p, oi)
			m.githubReviewSubmitForm(c, p, oi)
			if m.github.actionErr != "" {
				githubInlineError(c, p, m.github.actionErr, nil)
			}
			if m.github.notice != "" {
				ui.Text(c, m.github.notice).FontSize(11).TextColor(oi.success)
			}
			if len(detail.Checks) > 0 {
				m.githubChecks(c, p, oi, detail.Checks)
			}
			if len(detail.Files) > 0 {
				m.githubFiles(c, p, oi, detail.Files)
			}
			if m.github.selectedFile >= 0 {
				m.githubSelectedFileDiff(c, p, oi)
			}
			if m.github.review.mode == "review" {
				m.githubThreadsSection(c, p, oi)
			}
			if len(detail.Commits) > 0 {
				m.githubCommits(c, p, oi, detail.Commits)
			}
			if len(detail.Reviews) > 0 {
				m.githubReviews(c, p, oi, detail.Reviews)
			}
			if len(detail.Timeline) > 0 {
				m.githubTimeline(c, p, oi, detail.Timeline)
			}
			m.githubTimelineComposer(c, p, oi)
		})
	})
}

func (m *Model) githubDetailSummary(
	c *ui.Context,
	p *ui.Theme,
	oi oiColors,
	detail *ghub.PullRequestDetail,
) {
	ui.Column(c.Key("github:detail-summary")).Padding(9).Gap(7).Radius(6).
		Background(oi.elev).Border(1, oi.border).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			oiIconEl(c, githubPullRequestIcon(detail.PullRequestSummary), 20, oi,
				githubPullRequestColor(detail.PullRequestSummary, oi))
			ui.Text(c, detail.Title).FontSize(14).Bold().TextColor(oi.textStrong).
				SingleLine().Ellipsis("…").Grow(1)
			ui.Text(c, fmt.Sprintf("#%d", detail.Number)).FontSize(12).
				TextColor(oi.textMuted).SingleLine()
		})
		meta := []string{strings.ToLower(detail.State)}
		if detail.Author != "" {
			meta = append(meta, "by @"+detail.Author)
		}
		if detail.UpdatedAt != "" {
			meta = append(meta, "updated "+githubDate(detail.UpdatedAt))
		}
		ui.Text(c, strings.Join(meta, " · ")).FontSize(11).
			TextColor(oi.textMuted).SingleLine().Ellipsis("…")
		if detail.BaseRef != "" || detail.HeadRef != "" {
			ui.Text(c, detail.BaseRef+" ← "+detail.HeadRef).FontSize(11).
				TextColor(oi.textSecondary).SingleLine().Ellipsis("…")
		}
		if detail.Body != "" {
			ui.Text(c, detail.Body).FontSize(12).TextColor(oi.text)
		}
		ui.Row(c).Gap(6).AlignItems(ui.Center).Wrap().Children(func() {
			if decision := githubReviewDecisionLabel(detail.ReviewDecision); decision != "" {
				githubBadge(c, decision, githubReviewDecisionColor(detail.ReviewDecision, oi), oi)
			}
			if detail.Mergeable != "" {
				githubBadge(c, githubMergeableLabel(detail.Mergeable), oi.textMuted, oi)
			}
			if detail.UnresolvedThreads > 0 {
				githubBadge(c, fmt.Sprintf("%d unresolved", detail.UnresolvedThreads), oi.warning, oi)
			}
		})
		ui.Row(c).Gap(5).AlignItems(ui.Center).Wrap().Children(func() {
			ui.Button(c, "Open on GitHub").OnClick(func() {
				if detail.URL != "" {
					c.OpenURL(detail.URL)
				}
			})
			if strings.EqualFold(detail.MergeStateStatus, "BEHIND") {
				ui.Button(c, "Update Branch").Disabled(m.github.detailLoading).
					OnClick(m.UpdateGitHubBranch)
			}
		})
	})
}

func githubBadge(c *ui.Context, label string, color ui.Color, oi oiColors) {
	ui.Text(c, label).FontSize(10).TextColor(color).Padding(2, 6).
		Radius(5).Background(oi.badge)
}

func (m *Model) githubChecks(c *ui.Context, p *ui.Theme, oi oiColors, checks []ghub.Check) {
	githubSection(c, oi, "Checks", func() {
		for _, check := range checks {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				state := check.Conclusion
				if state == "" {
					state = check.State
				}
				if state == "" && !strings.EqualFold(check.Status, "COMPLETED") {
					state = "PENDING"
				}
				oiIconEl(c, githubCheckIcon(state), 13, oi, githubCheckColor(state, oi))
				name := check.Name
				if name == "" {
					name = "Check"
				}
				if check.Workflow != "" {
					name = check.Workflow + " / " + name
				}
				ui.Text(c, name).FontSize(11).TextColor(oi.text).
					SingleLine().Ellipsis("…").Grow(1)
				if check.URL != "" {
					ui.Button(c, "Details").OnClick(func() { c.OpenURL(check.URL) })
				}
			})
		}
	})
}

func (m *Model) githubFiles(c *ui.Context, p *ui.Theme, oi oiColors, files []ghub.FileChange) {
	githubSection(c, oi, fmt.Sprintf("Files (%d)", len(files)), func() {
		for index, file := range files {
			selected := index == m.github.selectedFile
			row := ui.Row(c.Key("github:file:"+file.Path)).MinHeight(28).
				Padding(3, 5).Gap(6).Radius(4).AlignItems(ui.Center).
				Cursor(ui.CursorPointer).Role(ui.RoleButton).Focusable().Label(file.Path)
			if selected {
				row.Background(oi.selected)
			} else if row.Hovered() {
				row.Background(oi.hoverTint)
			}
			row.OnClick(func() { m.SelectGitHubFile(index) })
			row.Children(func() {
				ui.Text(c, strings.ToUpper(file.ChangeType[:min(1, len(file.ChangeType))])).
					FontSize(10).TextColor(oi.textMuted).Width(10)
				ui.Text(c, file.Path).FontSize(11).TextColor(oi.text).
					SingleLine().Ellipsis("…").Grow(1)
				ui.Text(c, fmt.Sprintf("+%d −%d", file.Additions, file.Deletions)).
					FontSize(10).TextColor(oi.textMuted).SingleLine()
				label := "Mark viewed"
				if file.ViewerViewed {
					label = "Viewed"
				}
				ui.Button(c, label).FontSize(10).OnClick(func() {
					m.toggleGitHubFileViewedAt(index)
				})
			})
		}
	})
}

func (m *Model) githubCommits(c *ui.Context, p *ui.Theme, oi oiColors, commits []ghub.Commit) {
	githubSection(c, oi, fmt.Sprintf("Commits (%d)", len(commits)), func() {
		for _, commit := range commits {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, commit.ShortOID).FontSize(10).Font("monospace").
					TextColor(oi.link).Width(52)
				ui.Text(c, commit.Headline).FontSize(11).TextColor(oi.text).
					SingleLine().Ellipsis("…").Grow(1)
				if commit.Author != "" {
					ui.Text(c, commit.Author).FontSize(10).
						TextColor(oi.textMuted).SingleLine()
				}
			})
		}
	})
}

func (m *Model) githubReviews(c *ui.Context, p *ui.Theme, oi oiColors, reviews []ghub.Review) {
	githubSection(c, oi, fmt.Sprintf("Reviews (%d)", len(reviews)), func() {
		for _, review := range reviews {
			ui.Column(c).Padding(4, 5).Gap(2).Radius(4).
				Background(oi.elev).Children(func() {
				ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
					state := githubReviewDecisionLabel(review.State)
					if state == "" {
						state = strings.ToLower(review.State)
					}
					ui.Text(c, state).FontSize(11).
						TextColor(githubReviewDecisionColor(review.State, oi))
					if review.Author != "" {
						ui.Text(c, "@"+review.Author).FontSize(10).
							TextColor(oi.textMuted)
					}
				})
				if review.Body != "" {
					ui.Text(c, review.Body).FontSize(11).TextColor(oi.textSecondary)
				}
			})
		}
	})
}

func (m *Model) githubTimeline(c *ui.Context, p *ui.Theme, oi oiColors, items []ghub.TimelineItem) {
	githubSection(c, oi, "Activity", func() {
		order := append([]ghub.TimelineItem(nil), items...)
		sort.SliceStable(order, func(i, j int) bool {
			return order[i].CreatedAt < order[j].CreatedAt
		})
		for _, item := range order {
			ui.Column(c).PaddingStart(7).Gap(2).
				BorderWidth(0, 0, 0, 2).BorderColor(oi.border).Children(func() {
				ui.Text(c, githubTimelineLabel(item)).FontSize(10).
					TextColor(oi.textMuted).SingleLine().Ellipsis("…")
				if item.Body != "" {
					ui.Text(c, item.Body).FontSize(11).TextColor(oi.textSecondary)
				}
			})
		}
	})
}

func githubSection(c *ui.Context, oi oiColors, title string, body func()) {
	ui.Column(c).Gap(4).Children(func() {
		ui.Text(c, title).FontSize(12).Bold().TextColor(oi.text)
		body()
	})
}

func (m *Model) githubCreateBody(c *ui.Context, p *ui.Theme, oi oiColors) {
	ui.Scroll(c.Key("github:create")).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Padding(12).Gap(8).Children(func() {
			ui.Text(c, "New Pull Request").FontSize(14).Bold().TextColor(oi.text)
			ui.Text(c, "Choose branches already pushed to GitHub.").
				FontSize(11).TextColor(oi.textMuted)
			ui.TextInput(c.Key("github:create-title"), &m.github.create.title).
				Placeholder("Title")
			ui.TextArea(c.Key("github:create-body"), &m.github.create.body).
				MinHeight(120).Placeholder("Description")
			ui.Column(c).Gap(4).Children(func() {
				ui.Text(c, "Base branch").FontSize(11).TextColor(oi.textMuted)
				ui.Button(c, githubBranchLabel(m.github.create.base)).
					Disabled(m.github.create.loadingBranches).
					Menu(func(menu *ui.Menu) {
						m.githubBranchMenu(menu, true)
					})
			})
			ui.Column(c).Gap(4).Children(func() {
				ui.Text(c, "Head branch").FontSize(11).TextColor(oi.textMuted)
				ui.Button(c, githubBranchLabel(m.github.create.head)).
					Disabled(m.github.create.loadingBranches).
					Menu(func(menu *ui.Menu) {
						m.githubBranchMenu(menu, false)
					})
			})
			ui.Checkbox(c, &m.github.create.draft, "Create as draft")
			if m.github.create.loadingBranches {
				ui.Text(c, "Loading branches…").FontSize(11).TextColor(oi.textMuted)
			}
			if m.github.create.branchesErr != "" {
				githubInlineError(c, p, m.github.create.branchesErr, m.loadGitHubBranches)
			}
			if m.github.create.err != "" {
				ui.Text(c, m.github.create.err).FontSize(11).TextColor(oi.danger)
			}
			label := "Create Pull Request"
			if m.github.create.draft {
				label = "Create Draft Pull Request"
			}
			disabled := m.github.create.creating ||
				strings.TrimSpace(m.github.create.title) == "" ||
				m.github.create.base == "" || m.github.create.head == "" ||
				m.github.create.base == m.github.create.head
			ui.Button(c, label).Disabled(disabled).OnClick(m.CreateGitHubPullRequest)
		})
	})
}

func (m *Model) githubBranchMenu(menu *ui.Menu, base bool) {
	for _, branch := range m.github.create.branches {
		selected := branch.Name == m.github.create.head
		if base {
			selected = branch.Name == m.github.create.base
		}
		if menu.Item(branch.Name).Checked(selected).Chosen() {
			if base {
				m.github.create.base = branch.Name
			} else {
				m.github.create.head = branch.Name
			}
			m.invalidate()
		}
	}
}

func githubBranchLabel(name string) string {
	if name == "" {
		return "Choose branch"
	}
	return name
}

func githubInlineError(c *ui.Context, p *ui.Theme, message string, retry func()) {
	oi := oiPaletteOf(p)
	ui.Row(c.Key("github:error:"+message)).Shrink(0).Padding(6, 8).Gap(6).
		AlignItems(ui.Center).Background(p.Danger.Mix(p.Surface, 0.9)).Children(func() {
		oiIconEl(c, "status/error", 14, oi, oi.danger)
		ui.Text(c, message).FontSize(11).TextColor(oi.danger).Grow(1)
		if retry != nil {
			ui.Button(c, "Retry").OnClick(retry)
		}
	})
}

func githubReviewDecisionLabel(value string) string {
	switch strings.ToUpper(value) {
	case "APPROVED", "COMMENTED":
		return strings.ToLower(value)
	case "CHANGES_REQUESTED":
		return "Changes requested"
	case "REVIEW_REQUIRED", "REQUIRED":
		return "Review required"
	case "DISMISSED":
		return "Dismissed"
	default:
		return ""
	}
}

func githubReviewDecisionColor(value string, oi oiColors) ui.Color {
	switch strings.ToUpper(value) {
	case "APPROVED":
		return oi.success
	case "CHANGES_REQUESTED":
		return oi.danger
	case "REVIEW_REQUIRED", "REQUIRED":
		return oi.warning
	default:
		return oi.textMuted
	}
}

func githubCheckStateLabel(value string) string {
	switch strings.ToUpper(value) {
	case "SUCCESS":
		return "Checks passed"
	case "FAILURE", "ERROR", "CANCELLED", "TIMED_OUT", "ACTION_REQUIRED",
		"STARTUP_FAILURE", "STALE":
		return "Checks failed"
	case "PENDING", "EXPECTED":
		return "Checks running"
	default:
		return ""
	}
}

func githubCheckIcon(value string) string {
	switch strings.ToUpper(value) {
	case "SUCCESS", "NEUTRAL", "SKIPPED":
		return "status/check"
	case "FAILURE", "ERROR", "CANCELLED", "TIMED_OUT", "ACTION_REQUIRED",
		"STARTUP_FAILURE", "STALE":
		return "status/error"
	default:
		return "status/pending"
	}
}

func githubCheckColor(value string, oi oiColors) ui.Color {
	switch strings.ToUpper(value) {
	case "SUCCESS", "NEUTRAL", "SKIPPED":
		return oi.success
	case "FAILURE", "ERROR", "CANCELLED", "TIMED_OUT", "ACTION_REQUIRED",
		"STARTUP_FAILURE", "STALE":
		return oi.danger
	default:
		return oi.warning
	}
}

func githubMergeableLabel(value string) string {
	switch strings.ToUpper(value) {
	case "MERGEABLE":
		return "Mergeable"
	case "CONFLICTING":
		return "Conflicts"
	default:
		return "Mergeability unknown"
	}
}

func githubDate(value string) string {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return value
	}
	return parsed.Local().Format("Jan 2 15:04")
}

func githubTimelineLabel(item ghub.TimelineItem) string {
	actor := item.Actor
	if actor == "" {
		actor = "GitHub"
	}
	when := githubDate(item.CreatedAt)
	action := githubHumanEvent(item.Kind)
	if item.Kind == "commit" {
		action = "committed"
	}
	if item.State != "" && (item.Kind == "review" || strings.Contains(item.Kind, "Review")) {
		action += " " + strings.ToLower(strings.ReplaceAll(item.State, "_", " "))
	}
	if item.Label != "" {
		action += " label " + item.Label
	}
	return "@" + actor + " " + action + " · " + when
}

func githubHumanEvent(kind string) string {
	if kind == "comment" {
		return "commented"
	}
	var words []string
	for index, r := range kind {
		if index > 0 && unicode.IsUpper(r) {
			words = append(words, " ")
		}
		words = append(words, string(unicode.ToLower(r)))
	}
	return strings.TrimSpace(strings.Join(words, ""))
}
