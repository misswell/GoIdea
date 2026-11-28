package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"

	"mayoi/internal/git"
	ghub "mayoi/internal/github"
)

const githubTokenSecretKey = "github.token"

type githubTokenStore interface {
	ghub.Credentials
	SetToken(string) error
	DeleteToken() error
}

type secretGitHubTokenStore struct{}

func (secretGitHubTokenStore) Token(context.Context) (string, error) {
	value, err := mygo.Secrets.Get(githubTokenSecretKey)
	if errors.Is(err, mygo.ErrSecretNotFound) {
		return "", ghub.ErrNoToken
	}
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(value))
	if token == "" {
		return "", ghub.ErrNoToken
	}
	return token, nil
}

func (secretGitHubTokenStore) SetToken(token string) error {
	return mygo.Secrets.Set(githubTokenSecretKey, []byte(token))
}

func (secretGitHubTokenStore) DeleteToken() error {
	err := mygo.Secrets.Delete(githubTokenSecretKey)
	if errors.Is(err, mygo.ErrSecretNotFound) {
		return nil
	}
	return err
}

type githubCreateState struct {
	title           string
	body            string
	base            string
	head            string
	draft           bool
	branches        []ghub.Branch
	loadingBranches bool
	branchesErr     string
	creating        bool
	err             string
}

type githubState struct {
	visible bool
	root    string
	// generation changes when the project or connection changes. Every
	// asynchronous result is discarded when it no longer matches.
	generation  uint64
	credentials githubTokenStore
	// client is an injection point for focused tests.
	client *ghub.Client

	coordinates    ghub.Coordinates
	remotes        []githubRemoteChoice
	selectedRemote string
	repository     ghub.Repository
	viewer         ghub.Viewer
	currentBranch  string
	connecting     bool
	connectErr     string
	needsToken     bool
	tokenErr       string
	tokenDraft     string
	tokenSaving    bool

	view string
	// showFilters expands the author, assignee and label fields below the
	// quick state chips.
	showFilters bool
	// listSeq, detailSeq, branchSeq and actionSeq keep slower replies from
	// replacing a newer request in the same view.
	listSeq   uint64
	detailSeq uint64
	branchSeq uint64
	actionSeq uint64

	filters     ghub.SearchFilters
	items       []ghub.PullRequestSummary
	listTotal   int
	listCursor  string
	listHasMore bool
	listLoading bool
	listLoaded  bool
	listErr     string

	selectedNumber   int
	openPullRequests []int
	detail           *ghub.PullRequestDetail
	detailLoading    bool
	detailErr        string
	selectedFile     int

	create    githubCreateState
	actionErr string
	notice    string

	review githubReviewState
}

type githubBranchIdentity struct {
	remotes     []githubRemoteChoice
	remoteName  string
	coordinates ghub.Coordinates
	localBranch string
	headOwner   string
	headBranch  string
	pullRequest int
	tracked     bool
}

type githubRemoteChoice struct {
	Remote      string
	Coordinates ghub.Coordinates
}

func (s githubState) connected() bool {
	return s.repository.ID != "" && s.coordinates.Owner != ""
}

func (m *Model) githubResultCurrent(root string, generation uint64) bool {
	return m.Root() == root && m.github.root == root && m.github.generation == generation
}

func (m *Model) ensureGitHubState() {
	root := m.Root()
	if m.github.root == root {
		if m.github.credentials == nil {
			m.github.credentials = secretGitHubTokenStore{}
		}
		return
	}
	visible := m.github.visible
	credentials := m.github.credentials
	if credentials == nil {
		credentials = secretGitHubTokenStore{}
	}
	client := m.github.client
	generation := m.github.generation + 1
	m.github = githubState{
		visible: visible, root: root, generation: generation,
		credentials: credentials, client: client,
		view: "list", filters: ghub.DefaultSearchFilters(),
		selectedFile: -1,
		review:       newGitHubReviewState(),
	}
}

func (m *Model) githubClient() *ghub.Client {
	m.ensureGitHubState()
	if m.github.client != nil {
		return m.github.client
	}
	m.github.client = ghub.NewClient(m.github.credentials, ghub.ClientOptions{
		UserAgent: "GoIDEA/" + mygo.App.Version(),
	})
	return m.github.client
}

func githubBranchIdentityForRoot(root string) (githubBranchIdentity, error) {
	repo, err := git.Open(root)
	if err != nil {
		return githubBranchIdentity{}, fmt.Errorf("this project is not a Git work tree")
	}
	names, namesErr := repo.Remotes()
	if namesErr != nil {
		return githubBranchIdentity{}, namesErr
	}
	raw, _, preferredErr := repo.PreferredRemoteURL()
	preferredRemote := ""
	var remotes []githubRemoteChoice
	seen := map[ghub.Coordinates]bool{}
	for _, name := range names {
		remoteURL, err := repo.RemoteURL(name)
		if err != nil {
			continue
		}
		coordinates, err := ghub.ParseRemote(remoteURL)
		if err != nil || seen[coordinates] {
			continue
		}
		seen[coordinates] = true
		remotes = append(remotes, githubRemoteChoice{
			Remote: name, Coordinates: coordinates,
		})
		if preferredErr == nil && remoteURL == raw {
			preferredRemote = name
		}
	}
	if len(remotes) == 0 {
		if preferredErr != nil {
			return githubBranchIdentity{}, preferredErr
		}
		return githubBranchIdentity{}, fmt.Errorf("add a github.com remote to use Pull Requests")
	}
	if preferredRemote == "" {
		preferredRemote = remotes[0].Remote
	}
	for _, remote := range remotes {
		if remote.Remote == preferredRemote {
			return githubBranchIdentityFromRemote(
				repo, remotes, remote,
			), nil
		}
	}
	return githubBranchIdentity{}, fmt.Errorf("add a github.com remote to use Pull Requests")
}

func githubBranchIdentityFromRemote(
	repo *git.Repo,
	remotes []githubRemoteChoice,
	remote githubRemoteChoice,
) githubBranchIdentity {
	local := repo.BranchName()
	identity := githubBranchIdentity{
		remotes:     append([]githubRemoteChoice(nil), remotes...),
		remoteName:  remote.Remote,
		coordinates: remote.Coordinates,
		localBranch: local,
		headOwner:   remote.Coordinates.Owner,
		headBranch:  local,
	}
	_, upstream, err := repo.PreferredRemoteURL()
	if err != nil || upstream == "" {
		return identity
	}
	identity.tracked = true
	if number, ok := githubPullRequestRef(upstream); ok {
		identity.pullRequest = number
		identity.headBranch = ""
		return identity
	}
	if trackedRemote, remoteErr := repo.RemoteURLForBranch(local); remoteErr == nil {
		if trackedCoordinates, parseErr := ghub.ParseRemote(trackedRemote); parseErr == nil {
			identity.headOwner = trackedCoordinates.Owner
		}
	}
	identity.headBranch = strings.TrimPrefix(upstream, "refs/heads/")
	if identity.headBranch == "" {
		identity.headBranch = local
	}
	return identity
}

func githubPullRequestRef(ref string) (int, bool) {
	ref = strings.TrimSpace(ref)
	if !strings.HasPrefix(ref, "refs/pull/") || !strings.HasSuffix(ref, "/head") {
		return 0, false
	}
	value := strings.TrimSuffix(strings.TrimPrefix(ref, "refs/pull/"), "/head")
	number, err := strconv.Atoi(value)
	return number, err == nil && number > 0
}

func githubCoordinatesForRoot(root string) (ghub.Coordinates, string, error) {
	identity, err := githubBranchIdentityForRoot(root)
	if err != nil {
		return ghub.Coordinates{}, "", err
	}
	return identity.coordinates, identity.localBranch, nil
}

// OpenGitHubPullRequests shows the Pull Requests dock window.
func (m *Model) OpenGitHubPullRequests() {
	m.DockShow("pullRequests")
}

func (m *Model) openGitHubContent() {
	m.ensureGitHubState()
	m.github.visible = true
	if m.github.root == "" {
		m.github.connectErr = "Open a project with a github.com remote first"
		return
	}
	if !m.github.connected() && !m.github.connecting && !m.github.needsToken {
		m.connectGitHub(true)
	}
}

func (m *Model) connectGitHub(force bool) {
	m.ensureGitHubState()
	if m.github.root == "" {
		m.github.connectErr = "Open a project with a github.com remote first"
		return
	}
	if m.github.connecting {
		return
	}
	if m.github.connected() && !force {
		return
	}
	identity, err := githubBranchIdentityForRoot(m.github.root)
	if err != nil {
		m.github.connectErr = err.Error()
		m.github.needsToken = false
		m.invalidate()
		return
	}
	selectedRemote := identity.remoteName
	if m.github.selectedRemote != "" {
		for _, remote := range identity.remotes {
			if remote.Remote == m.github.selectedRemote {
				selectedRemote = remote.Remote
				identity.coordinates = remote.Coordinates
				break
			}
		}
	}
	coordinates, branch := identity.coordinates, identity.localBranch
	client := m.githubClient()
	filters := m.github.filters
	root := m.github.root
	gen := m.github.generation
	m.github.coordinates = coordinates
	m.github.remotes = append([]githubRemoteChoice(nil), identity.remotes...)
	m.github.selectedRemote = selectedRemote
	m.github.currentBranch = branch
	m.github.connecting = true
	m.github.connectErr = ""
	m.github.needsToken = false
	m.github.tokenErr = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 45*time.Second)
		defer cancel()
		repository, viewer, err := client.Repository(ctx, coordinates)
		if err == nil {
			var branchPullRequest ghub.BranchPullRequest
			var branchPullRequestOK bool
			var branchPullRequestErr error
			if identity.tracked && identity.pullRequest > 0 {
				branchPullRequest = ghub.BranchPullRequest{Number: identity.pullRequest}
				branchPullRequestOK = true
			} else if identity.tracked && identity.headBranch != "" {
				branchPullRequest, branchPullRequestOK, branchPullRequestErr =
					client.OpenPullRequestForHead(
						ctx, coordinates, identity.headOwner, identity.headBranch,
					)
			}
			var page ghub.SearchPage
			page, err = client.SearchPullRequests(ctx, coordinates, filters, "")
			if err == nil {
				m.post(func() {
					if !m.githubResultCurrent(root, gen) {
						return
					}
					m.github.connecting = false
					m.github.repository = repository
					m.github.viewer = viewer
					m.github.listApply(page, true)
					m.github.review.branch = branch
					m.github.review.branchTracked = identity.tracked
					m.github.review.headOwner = identity.headOwner
					m.github.review.headBranch = identity.headBranch
					m.github.review.branchLoading = false
					m.github.review.branchErr = ""
					if branchPullRequestErr != nil {
						m.github.review.branchErr = githubErrorMessage(branchPullRequestErr)
					} else if branchPullRequestOK {
						m.github.review.branchNumber = branchPullRequest.Number
						m.github.review.branchTitle = branchPullRequest.Title
						m.github.review.branchURL = branchPullRequest.URL
					} else {
						m.github.review.branchNumber = 0
						m.github.review.branchTitle = ""
						m.github.review.branchURL = ""
					}
					if branchPullRequestOK {
						m.updateGitHubReviewBranchSync(
							root, gen, branchPullRequest.Number, branchPullRequest.HeadOID,
						)
					} else {
						m.github.review.branchUpdateRequired = false
					}
					if m.github.pendingNumber() > 0 {
						number := m.github.pendingNumber()
						if m.github.detail == nil {
							m.showGitHubPullRequestNumber(number)
						}
					} else if branchPullRequestOK &&
						(m.github.review.enableAfterLookup || m.github.review.enabled) {
						if branchPullRequest.HeadOID == "" {
							m.github.review.enableAfterLookup = false
							m.github.review.enabled = true
							m.github.review.mode = "review"
							m.showGitHubPullRequestNumber(branchPullRequest.Number)
						}
					} else if !identity.tracked && m.github.review.enableAfterLookup {
						m.github.review.enableAfterLookup = false
						m.github.notice = "The current branch has no remote upstream"
					}
					m.invalidate()
				})
				return
			}
		}
		m.post(func() {
			if !m.githubResultCurrent(root, gen) {
				return
			}
			m.github.connecting = false
			switch {
			case errors.Is(err, ghub.ErrNoToken):
				m.github.needsToken = true
				m.github.tokenErr = ""
			case errors.Is(err, ghub.ErrUnauthorized):
				m.github.needsToken = true
				m.github.tokenErr = "GitHub rejected this token. Replace it to continue."
			default:
				m.github.connectErr = githubErrorMessage(err)
			}
			m.invalidate()
		})
	}()
}

func (m *Model) selectGitHubRepository(remoteName string) {
	m.ensureGitHubState()
	if remoteName == "" || remoteName == m.github.selectedRemote {
		return
	}
	identity, err := githubBranchIdentityForRoot(m.github.root)
	if err != nil {
		m.github.connectErr = err.Error()
		m.invalidate()
		return
	}
	var chosen *githubRemoteChoice
	for index := range identity.remotes {
		if identity.remotes[index].Remote == remoteName {
			chosen = &identity.remotes[index]
			break
		}
	}
	if chosen == nil {
		return
	}
	visible := m.github.visible
	credentials, client := m.github.credentials, m.github.client
	reviewEnabled := m.github.review.enabled
	m.github = githubState{
		visible: visible, root: m.github.root, generation: m.github.generation + 1,
		credentials: credentials, client: client,
		remotes:        append([]githubRemoteChoice(nil), identity.remotes...),
		selectedRemote: remoteName, coordinates: chosen.Coordinates,
		view: "list", filters: ghub.DefaultSearchFilters(), selectedFile: -1,
		review: newGitHubReviewState(),
	}
	m.github.review.enabled = reviewEnabled
	m.connectGitHub(false)
}

func (s *githubState) pendingNumber() int {
	if s.selectedNumber > 0 && s.view == "detail" {
		return s.selectedNumber
	}
	return 0
}

func (s *githubState) listApply(page ghub.SearchPage, reset bool) {
	if reset {
		s.items = append([]ghub.PullRequestSummary(nil), page.PullRequests...)
	} else {
		seen := make(map[string]bool, len(s.items))
		for _, item := range s.items {
			seen[item.ID] = true
		}
		for _, item := range page.PullRequests {
			if !seen[item.ID] {
				s.items = append(s.items, item)
				seen[item.ID] = true
			}
		}
	}
	s.listTotal = page.Total
	s.listCursor = page.EndCursor
	s.listHasMore = page.HasMore
	s.listLoading = false
	s.listLoaded = true
	s.listErr = ""
}

func (m *Model) refreshGitHub() {
	m.ensureGitHubState()
	if m.github.root == "" {
		return
	}
	if m.github.needsToken && !m.github.connected() {
		return
	}
	m.connectGitHub(true)
}

func (m *Model) refreshGitHubCurrentView() {
	m.ensureGitHubState()
	if !m.github.connected() {
		m.refreshGitHub()
		return
	}
	switch m.github.view {
	case "detail":
		m.loadGitHubDetail()
	case "create":
		if len(m.github.create.branches) == 0 {
			m.loadGitHubBranches()
		}
	default:
		m.loadGitHubList(true)
	}
}

func (m *Model) refreshGitHubList() {
	m.ensureGitHubState()
	if !m.github.connected() {
		m.connectGitHub(true)
		return
	}
	m.loadGitHubList(true)
}

func (m *Model) loadGitHubList(reset bool) {
	m.ensureGitHubState()
	if !m.github.connected() || m.github.listLoading {
		return
	}
	if !reset && (!m.github.listHasMore || m.github.listCursor == "") {
		return
	}
	client := m.githubClient()
	coordinates := m.github.coordinates
	root := m.github.root
	gen := m.github.generation
	filters := m.github.filters
	cursor := ""
	if !reset {
		cursor = m.github.listCursor
	}
	m.github.listSeq++
	seq := m.github.listSeq
	m.github.listLoading = true
	if reset {
		m.github.listErr = ""
	}
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 45*time.Second)
		defer cancel()
		page, err := client.SearchPullRequests(ctx, coordinates, filters, cursor)
		m.post(func() {
			if !m.githubResultCurrent(root, gen) || m.github.listSeq != seq {
				return
			}
			m.github.listLoading = false
			if err != nil {
				if reset {
					m.github.items = nil
					m.github.listLoaded = true
				}
				m.github.listErr = githubErrorMessage(err)
				if errors.Is(err, ghub.ErrUnauthorized) {
					m.github.needsToken = true
					m.github.tokenErr = "GitHub rejected this token. Replace it to continue."
				}
			} else {
				m.github.listApply(page, reset)
			}
			m.invalidate()
		})
	}()
}

func (m *Model) loadMoreGitHub() {
	if m.github.listLoading || !m.github.listHasMore {
		return
	}
	m.loadGitHubList(false)
}

func (m *Model) applyGitHubSearch() {
	m.ensureGitHubState()
	m.github.view = "list"
	m.github.selectedNumber = 0
	m.github.detail = nil
	m.github.selectedFile = -1
	m.loadGitHubList(true)
}

func (m *Model) showGitHubPullRequestNumber(number int) {
	m.ensureGitHubState()
	if number <= 0 {
		return
	}
	samePullRequest := m.github.view == "detail" &&
		m.github.selectedNumber == number
	m.github.view = "detail"
	m.github.selectedNumber = number
	if !slices.Contains(m.github.openPullRequests, number) {
		m.github.openPullRequests = append(m.github.openPullRequests, number)
	}
	if !samePullRequest {
		m.github.detail = nil
		m.github.detailErr = ""
		m.github.selectedFile = -1
		resetGitHubReviewData(&m.github.review)
	}
	if !m.github.connected() {
		if !m.github.connecting && !m.github.needsToken {
			m.connectGitHub(true)
		}
		m.invalidate()
		return
	}
	m.loadGitHubDetail()
}

func (m *Model) ShowGitHubPullRequest(number int) {
	m.OpenGitHubPullRequests()
	m.showGitHubPullRequestNumber(number)
	m.invalidate()
}

func (m *Model) loadGitHubDetail() {
	m.ensureGitHubState()
	if !m.github.connected() || m.github.selectedNumber <= 0 {
		return
	}
	number := m.github.selectedNumber
	selectedPath := ""
	if m.github.detail != nil && m.github.selectedNumber == number &&
		m.github.selectedFile >= 0 && m.github.selectedFile < len(m.github.detail.Files) {
		selectedPath = m.github.detail.Files[m.github.selectedFile].Path
	}
	client := m.githubClient()
	coordinates := m.github.coordinates
	root := m.github.root
	gen := m.github.generation
	m.github.detailSeq++
	seq := m.github.detailSeq
	m.github.detailLoading = true
	m.github.detailErr = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 45*time.Second)
		defer cancel()
		detail, err := client.PullRequest(ctx, coordinates, number)
		m.post(func() {
			if !m.githubResultCurrent(root, gen) ||
				m.github.detailSeq != seq || m.github.selectedNumber != number {
				return
			}
			m.github.detailLoading = false
			if err != nil {
				m.github.detailErr = githubErrorMessage(err)
				if errors.Is(err, ghub.ErrUnauthorized) {
					m.github.needsToken = true
					m.github.tokenErr = "GitHub rejected this token. Replace it to continue."
				}
			} else {
				detail.Unread = false
				m.github.detail = &detail
				m.github.detailErr = ""
				m.github.selectedFile = -1
				if selectedPath != "" {
					for index, file := range detail.Files {
						if file.Path == selectedPath {
							m.github.selectedFile = index
							break
						}
					}
				}
				m.refreshGitHubReviewData()
				if m.github.review.branchNumber == number && detail.HeadOID != "" {
					m.updateGitHubReviewBranchSync(root, gen, number, detail.HeadOID)
				}
				for i := range m.github.items {
					if m.github.items[i].Number == number {
						m.github.items[i] = detail.PullRequestSummary
						break
					}
				}
			}
			m.invalidate()
		})
	}()
}

func (m *Model) BackToGitHubList() {
	m.ensureGitHubState()
	m.github.view = "list"
	m.github.detailLoading = false
	m.github.detailErr = ""
	m.github.selectedFile = -1
	if m.github.connected() {
		m.loadGitHubList(true)
	}
	m.invalidate()
}

func (m *Model) closeGitHubPullRequest(number int) {
	m.ensureGitHubState()
	index := slices.Index(m.github.openPullRequests, number)
	if index < 0 {
		return
	}
	m.github.openPullRequests = append(
		m.github.openPullRequests[:index],
		m.github.openPullRequests[index+1:]...,
	)
	if m.github.view != "detail" || m.github.selectedNumber != number {
		m.invalidate()
		return
	}
	if index > 0 {
		m.showGitHubPullRequestNumber(m.github.openPullRequests[index-1])
		return
	}
	m.BackToGitHubList()
}

func (m *Model) OpenGitHubCreate() {
	m.ensureGitHubState()
	if !m.github.connected() {
		if m.github.needsToken {
			return
		}
		m.connectGitHub(true)
		return
	}
	m.github.view = "create"
	m.github.create.err = ""
	if len(m.github.create.branches) == 0 && !m.github.create.loadingBranches {
		m.loadGitHubBranches()
	}
	m.invalidate()
}

func (m *Model) loadGitHubBranches() {
	m.ensureGitHubState()
	if !m.github.connected() || m.github.create.loadingBranches {
		return
	}
	client := m.githubClient()
	coordinates := m.github.coordinates
	root := m.github.root
	gen := m.github.generation
	m.github.branchSeq++
	seq := m.github.branchSeq
	m.github.create.loadingBranches = true
	m.github.create.branchesErr = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 45*time.Second)
		defer cancel()
		branches, err := client.Branches(ctx, coordinates)
		m.post(func() {
			if !m.githubResultCurrent(root, gen) || m.github.branchSeq != seq {
				return
			}
			m.github.create.loadingBranches = false
			if err != nil {
				m.github.create.branchesErr = githubErrorMessage(err)
				if errors.Is(err, ghub.ErrUnauthorized) {
					m.github.needsToken = true
					m.github.tokenErr = "GitHub rejected this token. Replace it to continue."
				}
			} else {
				m.github.create.branches = branches
				m.prefillGitHubCreateBranches()
			}
			m.invalidate()
		})
	}()
}

func (m *Model) prefillGitHubCreateBranches() {
	s := &m.github
	if s.create.base == "" {
		s.create.base = s.repository.DefaultBranch
	}
	if s.create.base == "" && len(s.create.branches) > 0 {
		s.create.base = s.create.branches[0].Name
	}
	if s.create.head == "" {
		s.create.head = s.currentBranch
	}
	if s.create.head == "" {
		for _, branch := range s.create.branches {
			if branch.Name != s.create.base {
				s.create.head = branch.Name
				break
			}
		}
	}
}

func (m *Model) CreateGitHubPullRequest() {
	m.ensureGitHubState()
	if !m.github.connected() || m.github.create.creating {
		return
	}
	input := ghub.CreatePullRequestInput{
		Title: strings.TrimSpace(m.github.create.title),
		Body:  m.github.create.body,
		Base:  m.github.create.base,
		Head:  m.github.create.head,
		Draft: m.github.create.draft,
	}
	if input.Title == "" {
		m.github.create.err = "Enter a title"
		m.invalidate()
		return
	}
	if input.Base == "" || input.Head == "" {
		m.github.create.err = "Choose a base and head branch"
		m.invalidate()
		return
	}
	if input.Base == input.Head {
		m.github.create.err = "The base and head branch must differ"
		m.invalidate()
		return
	}
	client := m.githubClient()
	coordinates := m.github.coordinates
	root := m.github.root
	gen := m.github.generation
	m.github.create.creating = true
	m.github.create.err = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 60*time.Second)
		defer cancel()
		created, err := client.CreatePullRequest(ctx, coordinates, input)
		m.post(func() {
			if !m.githubResultCurrent(root, gen) {
				return
			}
			m.github.create.creating = false
			if err != nil {
				m.github.create.err = githubErrorMessage(err)
				if errors.Is(err, ghub.ErrUnauthorized) {
					m.github.needsToken = true
					m.github.tokenErr = "GitHub rejected this token. Replace it to continue."
				}
				m.invalidate()
				return
			}
			m.github.notice = fmt.Sprintf("Created pull request #%d", created.Number)
			m.github.listLoaded = false
			m.github.view = "detail"
			m.github.selectedNumber = created.Number
			m.github.detail = nil
			m.loadGitHubDetail()
			m.invalidate()
		})
	}()
}

func (m *Model) SelectGitHubFile(index int) {
	m.ensureGitHubState()
	if m.github.detail == nil || index < 0 || index >= len(m.github.detail.Files) {
		m.github.selectedFile = -1
	} else {
		m.github.selectedFile = index
		if m.prefs.VCS.GitHub.AutoMarkViewed &&
			!m.github.detail.Files[index].ViewerViewed {
			m.toggleGitHubFileViewedAt(index)
			return
		}
	}
	m.github.review.activeRow = -1
	m.github.review.selectionAnchor = -1
	m.invalidate()
}

func (m *Model) ToggleGitHubFileViewed() {
	m.ensureGitHubState()
	if !m.github.connected() || m.github.detail == nil || m.github.selectedFile < 0 ||
		m.github.selectedFile >= len(m.github.detail.Files) {
		return
	}
	m.toggleGitHubFileViewedAt(m.github.selectedFile)
}

func (m *Model) toggleGitHubFileViewedAt(index int) {
	m.ensureGitHubState()
	if !m.github.connected() || m.github.detail == nil ||
		index < 0 || index >= len(m.github.detail.Files) {
		return
	}
	m.github.selectedFile = index
	file := m.github.detail.Files[index]
	viewed := !file.ViewerViewed
	client := m.githubClient()
	pullRequestID := m.github.detail.ID
	number := m.github.selectedNumber
	root := m.github.root
	gen := m.github.generation
	m.github.actionSeq++
	seq := m.github.actionSeq
	m.github.actionErr = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 45*time.Second)
		defer cancel()
		err := client.SetFileViewed(ctx, pullRequestID, file.Path, viewed)
		m.post(func() {
			if !m.githubResultCurrent(root, gen) ||
				m.github.actionSeq != seq || m.github.selectedNumber != number ||
				m.github.detail == nil || m.github.detail.ID != pullRequestID {
				return
			}
			if err != nil {
				m.github.actionErr = githubErrorMessage(err)
				if errors.Is(err, ghub.ErrUnauthorized) {
					m.github.needsToken = true
					m.github.tokenErr = "GitHub rejected this token. Replace it to continue."
				}
			} else if m.github.detail != nil &&
				index < len(m.github.detail.Files) &&
				m.github.detail.Files[index].Path == file.Path {
				m.github.detail.Files[index].ViewerViewed = viewed
				if viewed {
					m.github.detail.Files[index].ViewedState = "VIEWED"
				} else {
					m.github.detail.Files[index].ViewedState = "UNVIEWED"
				}
			}
			m.invalidate()
		})
	}()
}

func (m *Model) UpdateGitHubBranch() {
	m.ensureGitHubState()
	if !m.github.connected() || m.github.detail == nil || m.github.detail.HeadOID == "" {
		return
	}
	detail := *m.github.detail
	pullRequestID := detail.ID
	client := m.githubClient()
	coordinates := m.github.coordinates
	root := m.github.root
	gen := m.github.generation
	m.github.actionSeq++
	seq := m.github.actionSeq
	m.github.actionErr = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 60*time.Second)
		defer cancel()
		err := client.UpdateBranch(ctx, coordinates, detail.Number, detail.HeadOID)
		m.post(func() {
			if !m.githubResultCurrent(root, gen) ||
				m.github.actionSeq != seq || m.github.selectedNumber != detail.Number ||
				m.github.detail == nil || m.github.detail.ID != pullRequestID {
				return
			}
			if err != nil {
				m.github.actionErr = githubErrorMessage(err)
				if errors.Is(err, ghub.ErrUnauthorized) {
					m.github.needsToken = true
					m.github.tokenErr = "GitHub rejected this token. Replace it to continue."
				}
			} else {
				m.github.notice = fmt.Sprintf("Updated pull request #%d", detail.Number)
				m.loadGitHubDetail()
			}
			m.invalidate()
		})
	}()
}

func (m *Model) SaveGitHubToken() {
	m.ensureGitHubState()
	token := strings.TrimSpace(m.github.tokenDraft)
	if token == "" || strings.ContainsAny(token, "\r\n\t ") {
		m.github.tokenErr = "Enter a GitHub token without spaces"
		m.invalidate()
		return
	}
	store := m.github.credentials
	root := m.github.root
	gen := m.github.generation
	m.github.tokenSaving = true
	m.github.tokenErr = ""
	m.invalidate()
	go func() {
		err := store.SetToken(token)
		m.post(func() {
			if !m.githubResultCurrent(root, gen) {
				return
			}
			m.github.tokenSaving = false
			if err != nil {
				m.github.tokenErr = "Could not save the token in the OS credential store"
				m.invalidate()
				return
			}
			m.github.tokenDraft = ""
			m.github.needsToken = false
			m.connectGitHub(true)
		})
	}()
}

func (m *Model) ClearGitHubToken() {
	m.ensureGitHubState()
	store := m.github.credentials
	root := m.github.root
	gen := m.github.generation
	m.github.tokenSaving = true
	m.github.tokenErr = ""
	m.invalidate()
	go func() {
		err := store.DeleteToken()
		m.post(func() {
			if !m.githubResultCurrent(root, gen) {
				return
			}
			m.github.tokenSaving = false
			if err != nil {
				m.github.tokenErr = "Could not remove the token from the OS credential store"
			} else {
				m.github.generation++
				m.github.connecting = false
				m.github.repository = ghub.Repository{}
				m.github.items = nil
				m.github.detail = nil
				m.github.detailLoading = false
				m.github.listLoading = false
				m.github.create.loadingBranches = false
				m.github.create.creating = false
				m.github.selectedFile = -1
				m.github.review.enabled = false
				m.github.review.branchNumber = 0
				m.github.review.branchTitle = ""
				m.github.review.branchURL = ""
				m.github.review.branchErr = ""
				m.github.review.enableAfterLookup = false
				resetGitHubReviewData(&m.github.review)
				m.github.needsToken = true
				m.github.tokenDraft = ""
				m.github.listLoaded = false
			}
			m.invalidate()
		})
	}()
}

func githubErrorMessage(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ghub.ErrNoToken):
		return "Add a GitHub token to load pull requests"
	case errors.Is(err, ghub.ErrUnauthorized):
		return "GitHub rejected this token. Replace it to continue."
	case errors.Is(err, ghub.ErrNotFound):
		return "GitHub could not find this repository or pull request"
	case errors.Is(err, context.Canceled):
		return "The GitHub request was canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "GitHub did not respond in time"
	default:
		return err.Error()
	}
}

func githubToolWindowBadge(s *githubState) int {
	if s == nil {
		return 0
	}
	count := 0
	for _, item := range s.items {
		if item.Unread {
			count++
		}
	}
	return count
}

func init() {
	registerGitHubActions()
}

func registerGitHubActions() {
	available := func(m *Model) bool {
		return m.Root() != "" && !m.showingWelcome
	}
	actions := []*Action{
		{
			ID: "github.pullRequest.showInToolWindow", Title: "Pull Requests",
			Category: "GitHub", Keywords: "github pull request review",
			When: available,
			RunWithArg: func(m *Model, arg any) {
				number, _ := actionInt(arg)
				if number > 0 {
					m.ShowGitHubPullRequest(number)
				} else {
					m.OpenGitHubPullRequests()
				}
			},
		},
		{
			ID: "github.refresh", Title: "Refresh Pull Requests List",
			Category: "GitHub", Keywords: "github pull request reload",
			When:    available,
			Enabled: func(m *Model) bool { return !m.github.connecting && !m.github.listLoading },
			Running: func(m *Model) bool { return m.github.connecting || m.github.listLoading },
			Run:     func(m *Model) { m.refreshGitHub() },
		},
		{
			ID: "github.pullRequest.refresh", Title: "Refresh Pull Request",
			Category: "GitHub", Keywords: "github pull request review reload",
			When:    func(m *Model) bool { return available(m) && m.github.view == "detail" },
			Enabled: func(m *Model) bool { return m.github.connected() && !m.github.detailLoading },
			Running: func(m *Model) bool { return m.github.detailLoading },
			Run:     func(m *Model) { m.loadGitHubDetail() },
		},
		{
			ID: "github.pullRequest.timeline.refresh", Title: "Refresh Timeline",
			Category: "GitHub", Keywords: "github timeline comments review",
			When:    func(m *Model) bool { return available(m) && m.github.view == "detail" },
			Enabled: func(m *Model) bool { return m.github.connected() && !m.github.detailLoading },
			Running: func(m *Model) bool { return m.github.detailLoading },
			Run:     func(m *Model) { m.loadGitHubDetail() },
		},
		{
			ID: "github.pullRequest.diff.refreshReview", Title: "Refresh Review Data",
			Category: "GitHub", Keywords: "github diff review comments refresh",
			When: func(m *Model) bool {
				return available(m) && m.github.view == "detail" && m.github.detail != nil
			},
			Enabled: func(m *Model) bool {
				return m.github.connected() && !githubReviewLoading(&m.github.review)
			},
			Running: func(m *Model) bool { return githubReviewLoading(&m.github.review) },
			Run:     func(m *Model) { m.refreshGitHubReviewData() },
		},
		{
			ID: "github.createPullRequest", Title: "Create Pull Request…",
			Category: "GitHub", Keywords: "github pull request new",
			When: available,
			Enabled: func(m *Model) bool {
				return !m.github.create.creating
			},
			Run: func(m *Model) {
				if !m.github.connected() {
					m.OpenGitHubPullRequests()
					return
				}
				m.OpenGitHubCreate()
			},
		},
		{
			ID: "github.pullRequest.toggleViewed", Title: "Toggle File Viewed",
			Category: "GitHub", Keywords: "github pull request viewed file",
			When: func(m *Model) bool {
				return available(m) && m.github.detail != nil && m.github.selectedFile >= 0
			},
			Enabled: func(m *Model) bool {
				return m.github.connected() && !m.github.detailLoading
			},
			Run: func(m *Model) { m.ToggleGitHubFileViewed() },
		},
		{
			ID: "github.pullRequest.updateBranch", Title: "Update Branch",
			Category: "GitHub", Keywords: "github pull request behind merge base",
			When: func(m *Model) bool {
				return available(m) && m.github.detail != nil &&
					strings.EqualFold(m.github.detail.MergeStateStatus, "BEHIND")
			},
			Enabled: func(m *Model) bool {
				return m.github.connected() && !m.github.detailLoading
			},
			Run: func(m *Model) { m.UpdateGitHubBranch() },
		},
	}
	for _, action := range actions {
		_ = registerAction(action)
	}
}
