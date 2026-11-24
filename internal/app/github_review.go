package app

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	ghub "mayoi/internal/github"
)

type githubReviewCommentDraft struct {
	open       bool
	path       string
	side       string
	startSide  string
	startLine  int
	line       int
	body       string
	submitting bool
	err        string
}

type githubReviewSubmitState struct {
	open       bool
	body       string
	submitting bool
	err        string
}

type githubMetadataState struct {
	editing    bool
	title      string
	body       string
	base       string
	submitting bool
	err        string
}

type githubTimelineCommentState struct {
	body       string
	submitting bool
	err        string
}

type githubReviewState struct {
	mode              string
	enabled           bool
	branch            string
	branchNumber      int
	branchTitle       string
	branchURL         string
	branchLoading     bool
	branchErr         string
	enableAfterLookup bool
	branchSeq         uint64
	actionSeq         uint64
	refreshSeq        uint64

	diffLoading     bool
	diffLoaded      bool
	diffErr         string
	threadsLoading  bool
	threadsLoaded   bool
	threadsErr      string
	pendingLoading  bool
	pendingErr      string
	threads         []ghub.ReviewThread
	pending         ghub.PendingReview
	selectedThread  string
	resolvingThread string
	activeRow       int

	comment         githubReviewCommentDraft
	submit          githubReviewSubmitState
	metadata        githubMetadataState
	timelineComment githubTimelineCommentState
}

func newGitHubReviewState() githubReviewState {
	return githubReviewState{
		mode: "review", selectedThread: "", activeRow: -1,
		comment: githubReviewCommentDraft{startLine: -1, line: -1},
	}
}

func resetGitHubReviewData(state *githubReviewState) {
	state.mode = "review"
	state.actionSeq++
	state.refreshSeq++
	state.diffLoading = false
	state.diffLoaded = false
	state.diffErr = ""
	state.threadsLoading = false
	state.threadsLoaded = false
	state.threadsErr = ""
	state.pendingLoading = false
	state.pendingErr = ""
	state.threads = nil
	state.pending = ghub.PendingReview{}
	state.selectedThread = ""
	state.resolvingThread = ""
	state.activeRow = -1
	state.comment = githubReviewCommentDraft{startLine: -1, line: -1}
	state.submit = githubReviewSubmitState{}
	state.metadata = githubMetadataState{}
	state.timelineComment = githubTimelineCommentState{}
}

func (m *Model) loadGitHubReviewBranch() {
	m.ensureGitHubState()
	if !m.github.connected() {
		if !m.github.connecting && !m.github.needsToken {
			m.connectGitHub(true)
		}
		return
	}
	if m.github.review.branchLoading {
		return
	}
	client := m.githubClient()
	coordinates := m.github.coordinates
	branch := m.github.currentBranch
	root := m.github.root
	gen := m.github.generation
	m.github.review.branchSeq++
	seq := m.github.review.branchSeq
	m.github.review.branchLoading = true
	m.github.review.branchErr = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 45*time.Second)
		defer cancel()
		found, ok, err := client.OpenPullRequestForBranch(ctx, coordinates, branch)
		m.post(func() {
			if !m.githubResultCurrent(root, gen) || m.github.review.branchSeq != seq {
				return
			}
			m.github.review.branchLoading = false
			m.github.review.branch = branch
			if err != nil {
				m.github.review.branchErr = githubErrorMessage(err)
			} else if ok {
				m.github.review.branchNumber = found.Number
				m.github.review.branchTitle = found.Title
				m.github.review.branchURL = found.URL
			} else {
				m.github.review.branchNumber = 0
				m.github.review.branchTitle = ""
				m.github.review.branchURL = ""
			}
			if m.github.review.enableAfterLookup {
				m.github.review.enableAfterLookup = false
				if ok {
					m.github.review.enabled = true
					m.github.review.mode = "review"
					m.ShowGitHubPullRequest(found.Number)
				} else if err == nil {
					m.github.notice = "No open pull request uses the current branch"
				}
			}
			m.invalidate()
		})
	}()
}

// ToggleGitHubReviewMode turns the current-branch review surface on or off.
func (m *Model) ToggleGitHubReviewMode() {
	m.ensureGitHubState()
	if m.github.review.enabled {
		m.github.review.enabled = false
		m.github.notice = "Review Mode is off"
		m.invalidate()
		return
	}
	m.github.review.mode = "review"
	if !m.github.connected() {
		m.github.review.enableAfterLookup = true
		m.openGitHubContent()
		return
	}
	if m.github.review.branchNumber > 0 {
		m.github.review.enabled = true
		m.ShowGitHubPullRequest(m.github.review.branchNumber)
		m.github.notice = fmt.Sprintf("Review Mode: #%d on %s",
			m.github.review.branchNumber, m.github.currentBranch)
		m.invalidate()
		return
	}
	m.github.review.enableAfterLookup = true
	m.loadGitHubReviewBranch()
}

// OpenCurrentGitHubPullRequest opens the pull request checked out by Review Mode.
func (m *Model) OpenCurrentGitHubPullRequest() {
	m.ensureGitHubState()
	if m.github.review.branchNumber <= 0 {
		m.loadGitHubReviewBranch()
		return
	}
	m.github.review.mode = "review"
	m.ShowGitHubPullRequest(m.github.review.branchNumber)
}

// SetGitHubDiffMode switches the native diff between normal and review views.
func (m *Model) SetGitHubDiffMode(mode string) {
	m.ensureGitHubState()
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "normal", "review":
		m.github.review.mode = strings.ToLower(strings.TrimSpace(mode))
		m.github.review.activeRow = -1
		m.invalidate()
	}
}

func mergeGitHubDiffFiles(base, diff []ghub.FileChange) []ghub.FileChange {
	if len(base) == 0 {
		return append([]ghub.FileChange(nil), diff...)
	}
	position := make(map[string]int, len(base))
	for index, file := range base {
		position[file.Path] = index
	}
	seen := make(map[string]bool, len(diff))
	for _, patch := range diff {
		seen[patch.Path] = true
		if index, ok := position[patch.Path]; ok {
			base[index].PreviousPath = patch.PreviousPath
			base[index].ChangeType = patch.ChangeType
			base[index].Additions = patch.Additions
			base[index].Deletions = patch.Deletions
			base[index].Patch = patch.Patch
			continue
		}
		base = append(base, patch)
		position[patch.Path] = len(base) - 1
	}
	for index := range base {
		if !seen[base[index].Path] {
			base[index].Patch = ""
		}
	}
	return base
}

func githubUnresolvedThreads(threads []ghub.ReviewThread) int {
	count := 0
	for _, thread := range threads {
		if !thread.IsResolved {
			count++
		}
	}
	return count
}

// refreshGitHubReviewData loads REST patches, GraphQL threads and the viewer's
// pending review for the selected pull request.
func (m *Model) refreshGitHubReviewData() {
	m.ensureGitHubState()
	if !m.github.connected() || m.github.selectedNumber <= 0 ||
		m.github.detail == nil || m.github.detail.ID == "" {
		return
	}
	number := m.github.selectedNumber
	pullRequestID := m.github.detail.ID
	client := m.githubClient()
	coordinates := m.github.coordinates
	root := m.github.root
	gen := m.github.generation
	m.github.review.refreshSeq++
	seq := m.github.review.refreshSeq
	m.github.review.diffLoading = true
	m.github.review.threadsLoading = true
	m.github.review.pendingLoading = true
	m.github.review.diffErr = ""
	m.github.review.threadsErr = ""
	m.github.review.pendingErr = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 60*time.Second)
		defer cancel()
		files, filesErr := client.PullRequestFiles(ctx, coordinates, number)
		threads, threadsErr := client.ReviewThreads(ctx, coordinates, number)
		pending, pendingOK, pendingErr := client.PendingReview(ctx, pullRequestID)
		m.post(func() {
			if !m.githubResultCurrent(root, gen) || m.github.review.refreshSeq != seq ||
				m.github.selectedNumber != number || m.github.detail == nil ||
				m.github.detail.ID != pullRequestID {
				return
			}
			m.github.review.diffLoading = false
			m.github.review.threadsLoading = false
			m.github.review.pendingLoading = false
			if filesErr != nil {
				m.github.review.diffErr = githubErrorMessage(filesErr)
				m.markGitHubUnauthorized(filesErr)
			} else {
				m.github.detail.Files = mergeGitHubDiffFiles(m.github.detail.Files, files)
				m.github.detail.DiffLoaded = true
				m.github.review.diffLoaded = true
			}
			if threadsErr != nil {
				m.github.review.threadsErr = githubErrorMessage(threadsErr)
				m.markGitHubUnauthorized(threadsErr)
			} else {
				m.github.review.threads = threads
				m.github.review.threadsLoaded = true
				m.github.detail.Threads = append([]ghub.ReviewThread(nil), threads...)
				m.github.detail.ThreadsLoaded = true
				m.github.detail.UnresolvedThreads = githubUnresolvedThreads(threads)
				if !m.githubThreadSelected() {
					m.github.review.selectedThread = ""
				}
			}
			if pendingErr != nil {
				m.github.review.pendingErr = githubErrorMessage(pendingErr)
				m.markGitHubUnauthorized(pendingErr)
			} else if pendingOK {
				m.github.review.pending = pending
			} else {
				m.github.review.pending = ghub.PendingReview{}
			}
			m.invalidate()
		})
	}()
}

func (m *Model) githubThreadSelected() bool {
	if m.github.review.selectedThread == "" {
		return false
	}
	for _, thread := range m.github.review.threads {
		if thread.ID == m.github.review.selectedThread {
			return true
		}
	}
	return false
}

func (m *Model) selectedGitHubDiffRow() (githubDiffRow, bool) {
	if m.github.detail == nil || m.github.selectedFile < 0 ||
		m.github.selectedFile >= len(m.github.detail.Files) {
		return githubDiffRow{}, false
	}
	rows := parseGitHubPatch(m.github.detail.Files[m.github.selectedFile].Patch)
	if m.github.review.activeRow < 0 || m.github.review.activeRow >= len(rows) {
		return githubDiffRow{}, false
	}
	row := rows[m.github.review.activeRow]
	return row, row.CommentLine > 0
}

// SelectGitHubDiffRow selects a line for comment actions.
func (m *Model) SelectGitHubDiffRow(index int) {
	m.ensureGitHubState()
	rows := []githubDiffRow(nil)
	if m.github.detail != nil && m.github.selectedFile >= 0 &&
		m.github.selectedFile < len(m.github.detail.Files) {
		rows = parseGitHubPatch(m.github.detail.Files[m.github.selectedFile].Patch)
	}
	if index < 0 || index >= len(rows) || rows[index].CommentLine <= 0 {
		m.github.review.activeRow = -1
	} else {
		m.github.review.activeRow = index
	}
	m.invalidate()
}

// BeginGitHubReviewComment opens a comment editor at the selected diff line.
func (m *Model) BeginGitHubReviewComment(multiline bool) {
	m.ensureGitHubState()
	row, ok := m.selectedGitHubDiffRow()
	if !ok || m.github.detail == nil || m.github.selectedFile < 0 {
		m.github.actionErr = "Select a changed line before adding a review comment"
		m.invalidate()
		return
	}
	file := m.github.detail.Files[m.github.selectedFile]
	start := row.CommentLine
	if multiline {
		rows := parseGitHubPatch(file.Patch)
		for index := m.github.review.activeRow - 1; index >= 0; index-- {
			if rows[index].CommentSide == row.CommentSide && rows[index].CommentLine > 0 {
				start = rows[index].CommentLine
				break
			}
		}
	}
	m.github.review.comment = githubReviewCommentDraft{
		open: true, path: file.Path, side: row.CommentSide,
		startSide: row.CommentSide, startLine: start, line: row.CommentLine,
	}
	m.github.review.submit.open = false
	m.github.review.metadata.editing = false
	m.invalidate()
}

func (m *Model) closeGitHubReviewComment() {
	m.github.review.comment = githubReviewCommentDraft{startLine: -1, line: -1}
	m.invalidate()
}

// SubmitGitHubReviewComment creates a pending review thread. single posts a
// standalone COMMENT review when no pending review already exists.
func (m *Model) SubmitGitHubReviewComment(single bool) {
	m.ensureGitHubState()
	if m.github.detail == nil || m.github.review.comment.submitting {
		return
	}
	if m.github.review.pendingErr != "" {
		m.github.review.comment.err =
			"Retry the pending review lookup before adding review comments"
		m.invalidate()
		return
	}
	if m.github.review.pendingLoading {
		m.github.review.comment.err =
			"Wait for the pending review lookup before adding review comments"
		m.invalidate()
		return
	}
	draft := m.github.review.comment
	body := strings.TrimSpace(draft.body)
	if body == "" {
		m.github.review.comment.err = "Write a review comment"
		m.invalidate()
		return
	}
	if draft.path == "" || draft.line <= 0 || draft.startLine <= 0 ||
		draft.line < draft.startLine {
		m.github.review.comment.err = "Choose a valid single-line or multiline range"
		m.invalidate()
		return
	}
	input := ghub.ReviewThreadInput{
		Path: draft.path, Body: body, Side: draft.side, Line: draft.line,
		StartSide: draft.startSide, StartLine: draft.startLine,
	}
	client := m.githubClient()
	pullRequestID := m.github.detail.ID
	headOID := m.github.detail.HeadOID
	number := m.github.selectedNumber
	pendingID := m.github.review.pending.ID
	root := m.github.root
	gen := m.github.generation
	m.github.review.actionSeq++
	seq := m.github.review.actionSeq
	m.github.review.comment.submitting = true
	m.github.review.comment.err = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 60*time.Second)
		defer cancel()
		var err error
		if pendingID != "" {
			err = client.AddReviewThread(ctx, pendingID, input)
		} else {
			event := ""
			if single {
				event = "COMMENT"
			}
			_, err = client.CreateReview(ctx, ghub.CreateReviewInput{
				PullRequestID: pullRequestID,
				Event:         event,
				CommitOID:     headOID,
				Threads:       []ghub.ReviewThreadInput{input},
			})
		}
		m.post(func() {
			if !m.githubResultCurrent(root, gen) || m.github.review.actionSeq != seq ||
				m.github.selectedNumber != number || m.github.detail == nil ||
				m.github.detail.ID != pullRequestID {
				return
			}
			m.github.review.comment.submitting = false
			if err != nil {
				m.github.review.comment.err = githubErrorMessage(err)
				m.markGitHubUnauthorized(err)
				m.invalidate()
				return
			}
			m.github.review.comment = githubReviewCommentDraft{startLine: -1, line: -1}
			m.github.notice = "Review comment added"
			m.refreshGitHubReviewData()
			m.invalidate()
		})
	}()
}

func githubThreadOrder(files []ghub.FileChange, threads []ghub.ReviewThread) []int {
	order := make(map[string]int, len(files))
	for index, file := range files {
		order[file.Path] = index
	}
	indices := make([]int, len(threads))
	for index := range threads {
		indices[index] = index
	}
	sort.SliceStable(indices, func(i, j int) bool {
		left, right := threads[indices[i]], threads[indices[j]]
		leftFile, leftOK := order[left.Path]
		rightFile, rightOK := order[right.Path]
		if !leftOK {
			leftFile = len(order) + 1
		}
		if !rightOK {
			rightFile = len(order) + 1
		}
		if leftFile != rightFile {
			return leftFile < rightFile
		}
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		if left.Line != right.Line {
			return left.Line < right.Line
		}
		return left.ID < right.ID
	})
	return indices
}

// SelectGitHubReviewThread reveals one thread, its file and diff line.
func (m *Model) SelectGitHubReviewThread(index int) {
	m.ensureGitHubState()
	if index < 0 || index >= len(m.github.review.threads) {
		return
	}
	thread := m.github.review.threads[index]
	m.github.review.selectedThread = thread.ID
	m.github.review.mode = "review"
	m.github.review.comment.open = false
	m.github.review.activeRow = -1
	if m.github.detail != nil {
		for fileIndex, file := range m.github.detail.Files {
			if file.Path != thread.Path {
				continue
			}
			m.github.selectedFile = fileIndex
			rows := parseGitHubPatch(file.Patch)
			for rowIndex, row := range rows {
				if row.CommentSide == thread.DiffSide && row.CommentLine == thread.Line {
					m.github.review.activeRow = rowIndex
					break
				}
			}
			break
		}
	}
	m.invalidate()
}

// StepGitHubReviewComment moves across comment threads in file and line order.
func (m *Model) StepGitHubReviewComment(delta int) {
	m.ensureGitHubState()
	if len(m.github.review.threads) == 0 || delta == 0 {
		return
	}
	var files []ghub.FileChange
	if m.github.detail != nil {
		files = m.github.detail.Files
	}
	order := githubThreadOrder(files, m.github.review.threads)
	position := -1
	for index, threadIndex := range order {
		if m.github.review.threads[threadIndex].ID == m.github.review.selectedThread {
			position = index
			break
		}
	}
	next := position + delta
	if position < 0 {
		if delta < 0 {
			next = len(order) - 1
		} else {
			next = 0
		}
	}
	if next < 0 || next >= len(order) {
		return
	}
	m.SelectGitHubReviewThread(order[next])
}

func (m *Model) toggleGitHubReviewThread(index int, resolved bool) {
	m.ensureGitHubState()
	if index < 0 || index >= len(m.github.review.threads) ||
		m.github.review.resolvingThread != "" {
		return
	}
	thread := m.github.review.threads[index]
	if resolved && !thread.ViewerCanResolve || !resolved && !thread.ViewerCanUnresolve {
		return
	}
	client := m.githubClient()
	threadID := thread.ID
	number := m.github.selectedNumber
	detailID := ""
	if m.github.detail != nil {
		detailID = m.github.detail.ID
	}
	root := m.github.root
	gen := m.github.generation
	m.github.review.actionSeq++
	seq := m.github.review.actionSeq
	m.github.review.resolvingThread = threadID
	m.github.actionErr = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 45*time.Second)
		defer cancel()
		err := client.SetReviewThreadResolved(ctx, threadID, resolved)
		m.post(func() {
			if !m.githubResultCurrent(root, gen) || m.github.review.actionSeq != seq ||
				m.github.selectedNumber != number || m.github.detail == nil ||
				m.github.detail.ID != detailID {
				return
			}
			m.github.review.resolvingThread = ""
			if err != nil {
				m.github.actionErr = githubErrorMessage(err)
				m.markGitHubUnauthorized(err)
				m.invalidate()
				return
			}
			for threadIndex := range m.github.review.threads {
				if m.github.review.threads[threadIndex].ID == threadID {
					m.github.review.threads[threadIndex].IsResolved = resolved
				}
			}
			m.github.detail.Threads = append([]ghub.ReviewThread(nil), m.github.review.threads...)
			m.github.detail.UnresolvedThreads = githubUnresolvedThreads(m.github.review.threads)
			if resolved {
				m.github.notice = "Conversation resolved"
			} else {
				m.github.notice = "Conversation reopened"
			}
			m.invalidate()
		})
	}()
}

// SetSelectedGitHubReviewThreadResolved resolves or reopens the selected
// thread, if GitHub grants that operation.
func (m *Model) SetSelectedGitHubReviewThreadResolved(resolved bool) {
	m.ensureGitHubState()
	for index, thread := range m.github.review.threads {
		if thread.ID == m.github.review.selectedThread {
			m.toggleGitHubReviewThread(index, resolved)
			return
		}
	}
}

// OpenGitHubReviewSubmit opens or closes the review submission form.
func (m *Model) OpenGitHubReviewSubmit() {
	m.ensureGitHubState()
	if !m.github.review.submit.open &&
		(m.github.review.pendingLoading || m.github.review.pendingErr != "") {
		return
	}
	m.github.review.submit.open = !m.github.review.submit.open
	if m.github.review.submit.open {
		m.github.review.comment.open = false
		m.github.review.metadata.editing = false
	}
	m.github.review.submit.err = ""
	m.invalidate()
}

// SubmitGitHubReview approves, requests changes or comments on a pull request.
func (m *Model) SubmitGitHubReview(event string) {
	m.ensureGitHubState()
	if m.github.detail == nil || m.github.review.submit.submitting {
		return
	}
	if m.github.review.pendingErr != "" {
		m.github.review.submit.err =
			"Retry the pending review lookup before submitting a review"
		m.invalidate()
		return
	}
	if m.github.review.pendingLoading {
		m.github.review.submit.err =
			"Wait for the pending review lookup before submitting a review"
		m.invalidate()
		return
	}
	event = strings.ToUpper(strings.TrimSpace(event))
	switch event {
	case "APPROVE", "REQUEST_CHANGES", "COMMENT":
	default:
		m.github.review.submit.err = "Choose Approve, Request Changes or Comment"
		m.invalidate()
		return
	}
	body := strings.TrimSpace(m.github.review.submit.body)
	pending := m.github.review.pending
	if event != "APPROVE" && body == "" && pending.CommentsCount == 0 {
		m.github.review.submit.err = "Add a review comment before requesting changes or commenting"
		m.invalidate()
		return
	}
	if m.github.detail.ViewerDidAuthor && event != "COMMENT" {
		m.github.review.submit.err = "The pull request author can submit a comment review"
		m.invalidate()
		return
	}
	client := m.githubClient()
	pullRequestID := m.github.detail.ID
	headOID := m.github.detail.HeadOID
	number := m.github.selectedNumber
	pendingID := pending.ID
	root := m.github.root
	gen := m.github.generation
	m.github.review.actionSeq++
	seq := m.github.review.actionSeq
	m.github.review.submit.submitting = true
	m.github.review.submit.err = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 60*time.Second)
		defer cancel()
		var err error
		if pendingID != "" {
			err = client.SubmitReview(ctx, pendingID, event, body)
		} else {
			_, err = client.CreateReview(ctx, ghub.CreateReviewInput{
				PullRequestID: pullRequestID, Event: event, Body: body,
				CommitOID: headOID,
			})
		}
		m.post(func() {
			if !m.githubResultCurrent(root, gen) || m.github.review.actionSeq != seq ||
				m.github.selectedNumber != number || m.github.detail == nil ||
				m.github.detail.ID != pullRequestID {
				return
			}
			m.github.review.submit.submitting = false
			if err != nil {
				m.github.review.submit.err = githubErrorMessage(err)
				m.markGitHubUnauthorized(err)
				m.invalidate()
				return
			}
			m.github.review.submit = githubReviewSubmitState{}
			m.github.review.pending = ghub.PendingReview{}
			m.github.notice = githubReviewSubmittedNotice(event)
			m.loadGitHubDetail()
			m.invalidate()
		})
	}()
}

func githubReviewSubmittedNotice(event string) string {
	switch event {
	case "APPROVE":
		return "Review approved"
	case "REQUEST_CHANGES":
		return "Changes requested"
	default:
		return "Review comment submitted"
	}
}

// OpenGitHubMetadataEditor stages title, description and base branch changes.
func (m *Model) OpenGitHubMetadataEditor() {
	m.ensureGitHubState()
	if m.github.detail == nil || !m.github.detail.ViewerCanUpdate {
		return
	}
	m.github.review.metadata = githubMetadataState{
		editing: true,
		title:   m.github.detail.Title,
		body:    m.github.detail.Body,
		base:    m.github.detail.BaseRef,
	}
	m.github.review.comment.open = false
	m.github.review.submit.open = false
	if len(m.github.create.branches) == 0 {
		m.loadGitHubBranches()
	}
	m.invalidate()
}

// SaveGitHubMetadata updates editable pull request metadata and its base
// branch. GitHub does not expose a head-branch update operation.
func (m *Model) SaveGitHubMetadata() {
	m.ensureGitHubState()
	metadata := m.github.review.metadata
	if m.github.detail == nil || !metadata.editing || metadata.submitting {
		return
	}
	title := strings.TrimSpace(metadata.title)
	if title == "" {
		m.github.review.metadata.err = "Title is required"
		m.invalidate()
		return
	}
	if metadata.base == "" {
		m.github.review.metadata.err = "Choose a base branch"
		m.invalidate()
		return
	}
	client := m.githubClient()
	coordinates := m.github.coordinates
	pullRequestID := m.github.detail.ID
	number := m.github.selectedNumber
	titleChanged := title != m.github.detail.Title
	bodyChanged := metadata.body != m.github.detail.Body
	baseChanged := metadata.base != m.github.detail.BaseRef
	if !titleChanged && !bodyChanged && !baseChanged {
		m.github.review.metadata.editing = false
		m.invalidate()
		return
	}
	root := m.github.root
	gen := m.github.generation
	m.github.review.actionSeq++
	seq := m.github.review.actionSeq
	m.github.review.metadata.submitting = true
	m.github.review.metadata.err = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 60*time.Second)
		defer cancel()
		wrote := false
		var err error
		if titleChanged || bodyChanged {
			err = client.UpdatePullRequestMetadata(ctx, pullRequestID, ghub.PullRequestMetadataInput{
				Title: title, Body: metadata.body,
			})
			if err == nil {
				wrote = true
			}
		}
		if err == nil && baseChanged {
			err = client.UpdatePullRequestBase(ctx, coordinates, number, metadata.base)
			if err == nil {
				wrote = true
			}
		}
		m.post(func() {
			if !m.githubResultCurrent(root, gen) || m.github.review.actionSeq != seq ||
				m.github.selectedNumber != number || m.github.detail == nil ||
				m.github.detail.ID != pullRequestID {
				return
			}
			m.github.review.metadata.submitting = false
			if err != nil {
				m.github.review.metadata.err = githubErrorMessage(err)
				m.markGitHubUnauthorized(err)
				if wrote {
					m.loadGitHubDetail()
				}
				m.invalidate()
				return
			}
			m.github.review.metadata = githubMetadataState{}
			m.github.notice = "Pull request updated"
			m.loadGitHubDetail()
			m.invalidate()
		})
	}()
}

// AddGitHubTimelineComment posts a conversation comment to the pull request.
func (m *Model) AddGitHubTimelineComment() {
	m.ensureGitHubState()
	if m.github.detail == nil || m.github.review.timelineComment.submitting {
		return
	}
	body := strings.TrimSpace(m.github.review.timelineComment.body)
	if body == "" {
		m.github.review.timelineComment.err = "Write a comment"
		m.invalidate()
		return
	}
	client := m.githubClient()
	coordinates := m.github.coordinates
	number := m.github.selectedNumber
	pullRequestID := m.github.detail.ID
	root := m.github.root
	gen := m.github.generation
	m.github.review.actionSeq++
	seq := m.github.review.actionSeq
	m.github.review.timelineComment.submitting = true
	m.github.review.timelineComment.err = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 45*time.Second)
		defer cancel()
		err := client.CreateIssueComment(ctx, coordinates, number, body)
		m.post(func() {
			if !m.githubResultCurrent(root, gen) || m.github.review.actionSeq != seq ||
				m.github.selectedNumber != number || m.github.detail == nil ||
				m.github.detail.ID != pullRequestID {
				return
			}
			m.github.review.timelineComment.submitting = false
			if err != nil {
				m.github.review.timelineComment.err = githubErrorMessage(err)
				m.markGitHubUnauthorized(err)
				m.invalidate()
				return
			}
			m.github.review.timelineComment.body = ""
			m.github.notice = "Comment added"
			m.loadGitHubDetail()
			m.invalidate()
		})
	}()
}

func (m *Model) markGitHubUnauthorized(err error) {
	if !errors.Is(err, ghub.ErrUnauthorized) {
		return
	}
	m.github.needsToken = true
	m.github.tokenErr = "GitHub rejected this token. Replace it to continue."
}
