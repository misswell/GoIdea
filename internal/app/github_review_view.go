package app

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	ghub "mayoi/internal/github"
)

func (m *Model) githubReviewToolbar(c *ui.Context, p *ui.Theme, oi oiColors) {
	ui.Column(c.Key("github:review-toolbar")).Gap(5).Padding(7).Radius(6).
		Background(oi.elev).Border(1, oi.border).Children(func() {
		ui.Row(c).Gap(5).AlignItems(ui.Center).Wrap().Children(func() {
			label := "Review Mode"
			if m.github.review.enabled {
				label = "Review Mode On"
			}
			ui.Button(c, label).Disabled(m.github.connecting ||
				m.github.review.branchLoading).OnClick(m.ToggleGitHubReviewMode)
			if m.github.review.branchNumber > 0 {
				ui.Text(c, fmt.Sprintf("#%d on %s", m.github.review.branchNumber,
					m.github.currentBranch)).FontSize(11).TextColor(oi.textMuted).
					SingleLine().Ellipsis("…")
			} else if m.github.review.branchErr != "" {
				ui.Text(c, m.github.review.branchErr).FontSize(11).
					TextColor(oi.danger).SingleLine().Ellipsis("…")
			}
		})
		ui.Row(c).Gap(5).AlignItems(ui.Center).Wrap().Children(func() {
			reviewLabel := "Review diff"
			normalLabel := "Normal diff"
			if m.github.review.mode == "review" {
				reviewLabel = "✓ Review diff"
			} else {
				normalLabel = "✓ Normal diff"
			}
			ui.Button(c, reviewLabel).OnClick(func() { m.SetGitHubDiffMode("review") })
			ui.Button(c, normalLabel).OnClick(func() { m.SetGitHubDiffMode("normal") })
			count := len(m.github.review.threads)
			threadWord := "threads"
			if count == 1 {
				threadWord = "thread"
			}
			ui.Text(c, fmt.Sprintf("%d %s · %d unresolved", count, threadWord,
				githubUnresolvedThreads(m.github.review.threads))).
				FontSize(11).TextColor(oi.textMuted)
			ui.Button(c, "Previous").Disabled(count == 0).
				OnClick(func() { m.StepGitHubReviewComment(-1) })
			ui.Button(c, "Next").Disabled(count == 0).
				OnClick(func() { m.StepGitHubReviewComment(1) })
			submitLabel := "Submit Review…"
			if m.github.review.pending.CommentsCount > 0 {
				submitLabel = fmt.Sprintf("Submit Review (%d)…",
					m.github.review.pending.CommentsCount)
			}
			ui.Button(c, submitLabel).Disabled(m.github.review.submit.submitting ||
				m.github.review.pendingLoading || m.github.review.pendingErr != "").
				OnClick(m.OpenGitHubReviewSubmit)
			if m.github.detail != nil && m.github.detail.ViewerCanUpdate {
				ui.Button(c, "Edit fields…").OnClick(m.OpenGitHubMetadataEditor)
			}
		})
		if row, ok := m.selectedGitHubDiffRow(); ok && m.github.review.mode == "review" {
			ui.Row(c).Gap(5).AlignItems(ui.Center).Wrap().Children(func() {
				ui.Text(c, fmt.Sprintf("Selected line %s:%d", row.CommentSide,
					row.CommentLine)).FontSize(11).TextColor(oi.textMuted)
				ui.Button(c, "Comment").OnClick(func() {
					m.BeginGitHubReviewComment(false)
				})
				ui.Button(c, "Multiline").OnClick(func() {
					m.BeginGitHubReviewComment(true)
				})
			})
		}
		if m.github.review.pendingErr != "" {
			ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Pending review unavailable · "+
					m.github.review.pendingErr).FontSize(11).
					TextColor(oi.danger).SingleLine().Ellipsis("…").Grow(1)
				ui.Button(c, "Retry").OnClick(m.refreshGitHubReviewData)
			})
		}
	})
}

func (m *Model) githubMetadataEditor(c *ui.Context, p *ui.Theme, oi oiColors) {
	metadata := &m.github.review.metadata
	if !metadata.editing {
		return
	}
	ui.Column(c.Key("github:metadata")).Padding(9).Gap(7).Radius(6).
		Background(oi.elev).Border(1, oi.border).Children(func() {
		ui.Text(c, "Edit Pull Request").FontSize(13).Bold().TextColor(oi.text)
		ui.Text(c, "Title and description use GitHub GraphQL. The base branch uses REST. "+
			"GitHub has no operation for changing the head branch.").
			FontSize(11).TextColor(oi.textMuted)
		ui.Text(c, "Title").FontSize(11).TextColor(oi.textMuted)
		ui.TextInput(c.Key("github:metadata-title"), &metadata.title).
			Placeholder("Title")
		ui.Text(c, "Description").FontSize(11).TextColor(oi.textMuted)
		ui.TextArea(c.Key("github:metadata-body"), &metadata.body).
			MinHeight(110).Placeholder("Description")
		ui.Column(c).Gap(4).Children(func() {
			ui.Text(c, "Base branch").FontSize(11).TextColor(oi.textMuted)
			ui.Button(c, githubBranchLabel(metadata.base)).
				Disabled(m.github.create.loadingBranches).
				Menu(func(menu *ui.Menu) {
					for _, branch := range m.github.create.branches {
						if menu.Item(branch.Name).
							Checked(branch.Name == metadata.base).Chosen() {
							metadata.base = branch.Name
							m.invalidate()
						}
					}
				})
		})
		if m.github.create.loadingBranches {
			ui.Text(c, "Loading branches…").FontSize(11).TextColor(oi.textMuted)
		}
		if m.github.create.branchesErr != "" {
			ui.Text(c, m.github.create.branchesErr).FontSize(11).TextColor(oi.danger)
		}
		if metadata.err != "" {
			ui.Text(c, metadata.err).FontSize(11).TextColor(oi.danger)
		}
		ui.Row(c).Gap(6).Children(func() {
			ui.Button(c, "Save").Disabled(metadata.submitting ||
				strings.TrimSpace(metadata.title) == "" || metadata.base == "").
				OnClick(m.SaveGitHubMetadata)
			ui.Button(c, "Cancel").Disabled(metadata.submitting).OnClick(func() {
				metadata.editing = false
				metadata.err = ""
				m.invalidate()
			})
		})
	})
}

func (m *Model) githubSelectedFileDiff(c *ui.Context, p *ui.Theme, oi oiColors) {
	if m.github.detail == nil || m.github.selectedFile < 0 ||
		m.github.selectedFile >= len(m.github.detail.Files) {
		return
	}
	file := m.github.detail.Files[m.github.selectedFile]
	githubSection(c, oi, file.Path, func() {
		switch {
		case m.github.review.diffLoading && !m.github.review.diffLoaded:
			ui.Text(c, "Loading diff…").FontSize(11).TextColor(oi.textMuted)
		case m.github.review.diffErr != "":
			githubInlineError(c, p, m.github.review.diffErr, m.refreshGitHubReviewData)
		case file.Patch == "":
			ui.Text(c, "This file has no textual patch. Binary and very large files "+
				"can still be marked viewed.").FontSize(11).TextColor(oi.textMuted)
		default:
			m.githubPatchRows(c, p, oi, file)
		}
		if m.github.review.mode == "review" && m.github.review.comment.open &&
			m.github.review.comment.path == file.Path {
			m.githubReviewCommentEditor(c, p, oi)
		}
	})
}

func (m *Model) githubPatchRows(
	c *ui.Context,
	p *ui.Theme,
	oi oiColors,
	file ghub.FileChange,
) {
	rows := parseGitHubPatch(file.Patch)
	threadsByLine := map[string][]int{}
	for index, thread := range m.github.review.threads {
		if thread.Path != file.Path || thread.Line <= 0 {
			continue
		}
		key := thread.DiffSide + ":" + fmt.Sprint(thread.Line)
		threadsByLine[key] = append(threadsByLine[key], index)
	}
	ui.Column(c.Key("github:diff:" + file.Path)).Gap(1).Children(func() {
		for index, row := range rows {
			if row.Kind == "hunk" {
				ui.Text(c, row.Text).FontSize(10).Font("monospace").
					TextColor(oi.link).Padding(2, 4).Background(oi.badge).
					SingleLine().Ellipsis("…")
				continue
			}
			selected := index == m.github.review.activeRow
			line := ui.Row(c.Key(fmt.Sprintf("github:diff-row:%s:%d", file.Path, index))).
				MinHeight(19).Padding(0, 2).Gap(4).AlignItems(ui.Center)
			if row.CommentLine > 0 {
				line.Cursor(ui.CursorPointer)
			}
			if selected {
				line.Background(oi.selected)
			} else if line.Hovered() && row.CommentLine > 0 {
				line.Background(oi.hoverTint)
			}
			switch row.Kind {
			case "add":
				line.Background(oi.success.Mix(oi.elev, 0.88))
			case "remove":
				line.Background(oi.danger.Mix(oi.elev, 0.9))
			}
			if row.CommentLine > 0 {
				line.OnClick(func() { m.SelectGitHubDiffRow(index) })
			}
			line.Children(func() {
				oldLabel, newLabel := "", ""
				if row.OldLine > 0 {
					oldLabel = fmt.Sprint(row.OldLine)
				}
				if row.NewLine > 0 {
					newLabel = fmt.Sprint(row.NewLine)
				}
				ui.Text(c, oldLabel).Width(34).FontSize(10).Font("monospace").
					TextColor(oi.textFaint).SingleLine()
				ui.Text(c, newLabel).Width(34).FontSize(10).Font("monospace").
					TextColor(oi.textFaint).SingleLine()
				marker := " "
				switch row.Kind {
				case "add":
					marker = "+"
				case "remove":
					marker = "−"
				}
				ui.Text(c, marker).Width(10).FontSize(10).Font("monospace").
					TextColor(oi.textMuted)
				ui.Text(c, row.Text).FontSize(10).Font("monospace").
					TextColor(oi.text).SingleLine().Ellipsis("…").Grow(1)
				if m.github.review.mode == "review" && row.CommentLine > 0 {
					ui.Button(c, "+").FontSize(10).Tooltip("Comment on this line").
						OnClick(func() {
							m.SelectGitHubDiffRow(index)
							m.BeginGitHubReviewComment(false)
						})
				}
			})
			key := row.CommentSide + ":" + fmt.Sprint(row.CommentLine)
			for _, threadIndex := range threadsByLine[key] {
				if m.github.review.mode == "review" {
					m.githubReviewThreadCard(c, p, oi, threadIndex, true)
				}
			}
		}
	})
}

func (m *Model) githubReviewCommentEditor(c *ui.Context, p *ui.Theme, oi oiColors) {
	comment := &m.github.review.comment
	ui.Column(c.Key("github:new-review-comment")).Padding(7).Gap(5).Radius(5).
		Background(oi.badge).Children(func() {
		location := fmt.Sprintf("%s:%d", comment.path, comment.line)
		if comment.startLine > 0 && comment.startLine != comment.line {
			location = fmt.Sprintf("%s:%d-%d", comment.path, comment.startLine, comment.line)
		}
		ui.Text(c, "Review comment · "+location).
			FontSize(11).Bold().TextColor(oi.text).SingleLine().Ellipsis("…")
		ui.Row(c).Gap(5).AlignItems(ui.Center).Wrap().Children(func() {
			ui.Text(c, "Side").FontSize(11).TextColor(oi.textMuted)
			if comment.side == "LEFT" {
				ui.Button(c, "✓ Old").OnClick(func() {
					comment.side, comment.startSide = "LEFT", "LEFT"
				})
				ui.Button(c, "New").OnClick(func() {
					comment.side, comment.startSide = "RIGHT", "RIGHT"
				})
			} else {
				ui.Button(c, "Old").OnClick(func() {
					comment.side, comment.startSide = "LEFT", "LEFT"
				})
				ui.Button(c, "✓ New").OnClick(func() {
					comment.side, comment.startSide = "RIGHT", "RIGHT"
				})
			}
			ui.Text(c, "Start").FontSize(11).TextColor(oi.textMuted)
			startLine := float64(comment.startLine)
			if ui.NumberInput(c, &startLine, 1, 100000, 1).Width(74).Changed() {
				comment.startLine = int(startLine)
			}
			ui.Text(c, "End").FontSize(11).TextColor(oi.textMuted)
			line := float64(comment.line)
			if ui.NumberInput(c, &line, 1, 100000, 1).Width(74).Changed() {
				comment.line = int(line)
			}
		})
		ui.TextArea(c.Key("github:review-comment-body"), &comment.body).
			MinHeight(72).Placeholder("Leave a review comment")
		if comment.err != "" {
			ui.Text(c, comment.err).FontSize(11).TextColor(oi.danger)
		}
		ui.Row(c).Gap(5).Children(func() {
			if m.github.review.pending.ID == "" {
				ui.Button(c, "Start Review").
					Disabled(comment.submitting || m.github.review.pendingLoading ||
						m.github.review.pendingErr != "").
					OnClick(func() { m.SubmitGitHubReviewComment(false) })
				ui.Button(c, "Add Single Comment").
					Disabled(comment.submitting || m.github.review.pendingLoading ||
						m.github.review.pendingErr != "").
					OnClick(func() { m.SubmitGitHubReviewComment(true) })
			} else {
				ui.Button(c, "Add to Pending Review").
					Disabled(comment.submitting || m.github.review.pendingLoading ||
						m.github.review.pendingErr != "").
					OnClick(func() { m.SubmitGitHubReviewComment(false) })
			}
			ui.Button(c, "Cancel").Disabled(comment.submitting).
				OnClick(m.closeGitHubReviewComment)
		})
	})
}

func (m *Model) githubThreadsSection(c *ui.Context, p *ui.Theme, oi oiColors) {
	if m.github.review.threadsLoading && !m.github.review.threadsLoaded {
		githubSection(c, oi, "Review Threads", func() {
			ui.Text(c, "Loading review threads…").FontSize(11).TextColor(oi.textMuted)
		})
		return
	}
	if m.github.review.threadsErr != "" {
		githubSection(c, oi, "Review Threads", func() {
			githubInlineError(c, p, m.github.review.threadsErr, m.refreshGitHubReviewData)
		})
		return
	}
	githubSection(c, oi, fmt.Sprintf("Review Threads (%d)", len(m.github.review.threads)), func() {
		if len(m.github.review.threads) == 0 {
			ui.Text(c, "No review comments yet. Select a changed line to add one.").
				FontSize(11).TextColor(oi.textMuted)
			return
		}
		for _, index := range githubThreadOrder(m.github.detail.Files, m.github.review.threads) {
			m.githubReviewThreadCard(c, p, oi, index, false)
		}
	})
}

func (m *Model) githubReviewThreadCard(
	c *ui.Context,
	p *ui.Theme,
	oi oiColors,
	index int,
	compact bool,
) {
	if index < 0 || index >= len(m.github.review.threads) {
		return
	}
	thread := m.github.review.threads[index]
	selected := thread.ID == m.github.review.selectedThread
	card := ui.Column(c.Key("github:thread:"+thread.ID)).Padding(6).Gap(3).
		Radius(5).Border(1, oi.border).Cursor(ui.CursorPointer).
		Role(ui.RoleButton).Focusable().Background(oi.elev)
	if selected {
		card.Background(oi.selected)
	} else if card.Hovered() {
		card.Background(oi.hoverTint)
	}
	card.OnClick(func() { m.SelectGitHubReviewThread(index) })
	card.Children(func() {
		ui.Row(c).Gap(5).AlignItems(ui.Center).Wrap().Children(func() {
			location := thread.Path
			if !strings.EqualFold(thread.SubjectType, "FILE") && thread.Line > 0 {
				location = fmt.Sprintf("%s:%d", thread.Path, thread.Line)
			}
			ui.Text(c, location).
				FontSize(10).Font("monospace").TextColor(oi.link).
				SingleLine().Ellipsis("…").Grow(1)
			if thread.IsResolved {
				githubBadge(c, "Resolved", oi.success, oi)
			}
			if thread.IsOutdated {
				githubBadge(c, "Outdated", oi.warning, oi)
			}
		})
		comments := thread.Comments
		if compact && len(comments) > 3 {
			comments = append([]ghub.ReviewComment{comments[0]},
				comments[len(comments)-1])
		}
		for _, comment := range comments {
			author := comment.Author
			if author == "" {
				author = "GitHub"
			}
			ui.Text(c, "@"+author+" · "+githubDate(comment.CreatedAt)).
				FontSize(10).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
			if comment.Body != "" {
				ui.Text(c, comment.Body).FontSize(11).TextColor(oi.text)
			}
		}
		ui.Row(c).Gap(5).Children(func() {
			button := "Resolve"
			if thread.IsResolved {
				button = "Unresolve"
			}
			canToggle := thread.ViewerCanResolve
			if thread.IsResolved {
				canToggle = thread.ViewerCanUnresolve
			}
			ui.Button(c, button).
				Disabled(m.github.review.resolvingThread != "" || !canToggle).
				OnClick(func() {
					m.toggleGitHubReviewThread(index, !thread.IsResolved)
				})
		})
	})
}

func (m *Model) githubReviewSubmitForm(c *ui.Context, p *ui.Theme, oi oiColors) {
	submit := &m.github.review.submit
	if !submit.open {
		return
	}
	ui.Column(c.Key("github:submit-review")).Padding(9).Gap(7).Radius(6).
		Background(oi.elev).Border(1, oi.border).Children(func() {
		ui.Text(c, "Submit Review").FontSize(13).Bold().TextColor(oi.text)
		if m.github.review.pending.CommentsCount > 0 {
			commentWord := "comments"
			if m.github.review.pending.CommentsCount == 1 {
				commentWord = "comment"
			}
			ui.Text(c, fmt.Sprintf("%d pending inline %s",
				m.github.review.pending.CommentsCount, commentWord)).FontSize(11).
				TextColor(oi.textMuted)
		}
		ui.TextArea(c.Key("github:submit-review-body"), &submit.body).
			MinHeight(90).Placeholder("Review comment")
		if submit.err != "" {
			ui.Text(c, submit.err).FontSize(11).TextColor(oi.danger)
		}
		hasFeedback := strings.TrimSpace(submit.body) != "" ||
			m.github.review.pending.CommentsCount > 0
		isAuthor := m.github.detail != nil && m.github.detail.ViewerDidAuthor
		pendingUnavailable := m.github.review.pendingLoading ||
			m.github.review.pendingErr != ""
		ui.Row(c).Gap(5).Wrap().Children(func() {
			ui.Button(c, "Approve").
				Disabled(submit.submitting || pendingUnavailable || isAuthor).
				OnClick(func() { m.SubmitGitHubReview("APPROVE") })
			ui.Button(c, "Request Changes").
				Disabled(submit.submitting || pendingUnavailable || isAuthor || !hasFeedback).
				OnClick(func() { m.SubmitGitHubReview("REQUEST_CHANGES") })
			ui.Button(c, "Comment").
				Disabled(submit.submitting || pendingUnavailable || !hasFeedback).
				OnClick(func() { m.SubmitGitHubReview("COMMENT") })
			ui.Button(c, "Cancel").Disabled(submit.submitting).
				OnClick(m.OpenGitHubReviewSubmit)
		})
	})
}

func (m *Model) githubTimelineComposer(c *ui.Context, p *ui.Theme, oi oiColors) {
	comment := &m.github.review.timelineComment
	githubSection(c, oi, "Add Timeline Comment", func() {
		ui.TextArea(c.Key("github:timeline-comment"), &comment.body).
			MinHeight(70).Placeholder("Comment on this pull request")
		if comment.err != "" {
			ui.Text(c, comment.err).FontSize(11).TextColor(oi.danger)
		}
		ui.Button(c, "Comment").Disabled(comment.submitting ||
			strings.TrimSpace(comment.body) == "").OnClick(m.AddGitHubTimelineComment)
	})
}
