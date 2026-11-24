package github

const repositoryQuery = `
query GoIDEARepository($owner: String!, $name: String!) {
  viewer {
    login
    name
    avatarUrl
  }
  repository(owner: $owner, name: $name) {
    id
    nameWithOwner
    viewerPermission
    mergeCommitAllowed
    squashMergeAllowed
    rebaseMergeAllowed
    defaultBranchRef {
      name
    }
    url
  }
}`

const searchPullRequestsQuery = `
query GoIDEASearchPullRequests($query: String!, $pageSize: Int!, $cursor: String) {
  search(first: $pageSize, after: $cursor, type: ISSUE_ADVANCED, query: $query) {
    issueCount
    pageInfo {
      hasNextPage
      endCursor
    }
    nodes {
      __typename
      ... on PullRequest {
        id
        url
        number
        title
        state
        isDraft
        author {
          login
          avatarUrl
        }
        createdAt
        updatedAt
        isReadByViewer
        reviewDecision
        mergeable
        labels(first: 20) {
          nodes {
            name
            color
          }
        }
        assignees(first: 20) {
          nodes {
            login
          }
        }
        comments(first: 0) {
          totalCount
        }
        reviewThreads(last: 100) {
          nodes {
            isResolved
          }
        }
        reviews(last: 20) {
          nodes {
            id
            url
            author {
              login
            }
            body
            state
            createdAt
          }
        }
        commits(last: 1) {
          nodes {
            commit {
              statusCheckRollup {
                state
              }
            }
          }
        }
      }
    }
  }
}`

const pullRequestQuery = `
query GoIDEAPullRequest($owner: String!, $name: String!, $number: Int!) {
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      id
      url
      number
      title
      state
      isDraft
      body
      author {
        login
        avatarUrl
      }
      createdAt
      updatedAt
      isReadByViewer
      reviewDecision
      mergeable
      mergeStateStatus
      canBeRebased
      viewerCanUpdate
      viewerDidAuthor
      viewerCanDeleteHeadRef
      baseRefName
      baseRefOid
      baseRepository {
        url
      }
      headRefName
      headRefOid
      headRepository {
        url
      }
      labels(first: 100) {
        nodes {
          name
          color
        }
      }
      assignees(first: 20) {
        nodes {
          login
        }
      }
      comments(first: 0) {
        totalCount
      }
      reviewThreads(last: 100) {
        nodes {
          isResolved
        }
      }
      reviews(last: 50) {
        nodes {
          id
          url
          author {
            login
          }
          body
          state
          createdAt
        }
      }
      files(first: 100) {
        nodes {
          path
          changeType
          additions
          deletions
          viewerViewedState
        }
      }
      commits(last: 20) {
        nodes {
          url
          commit {
            oid
            abbreviatedOid
            messageHeadline
            committedDate
            author {
              name
              user {
                login
              }
            }
            statusCheckRollup {
              state
              contexts(first: 100) {
                nodes {
                  __typename
                  ... on CheckRun {
                    name
                    status
                    conclusion
                    detailsUrl
                    checkSuite {
                      workflowRun {
                        workflow {
                          name
                        }
                      }
                    }
                  }
                  ... on StatusContext {
                    context
                    description
                    state
                    targetUrl
                  }
                }
              }
            }
          }
        }
      }
      timelineItems(first: 50) {
        nodes {
          __typename
          ... on PullRequestCommit {
            url
            commit {
              oid
              abbreviatedOid
              messageHeadline
              committedDate
              author {
                name
                user {
                  login
                }
              }
            }
          }
          ... on PullRequestReview {
            url
            body
            state
            createdAt
            author {
              login
            }
          }
          ... on IssueComment {
            url
            body
            createdAt
            author {
              login
            }
          }
          ... on MergedEvent {
            createdAt
            actor {
              login
            }
          }
          ... on ClosedEvent {
            createdAt
            actor {
              login
            }
          }
          ... on ReopenedEvent {
            createdAt
            actor {
              login
            }
          }
          ... on ReadyForReviewEvent {
            createdAt
            actor {
              login
            }
          }
          ... on ConvertToDraftEvent {
            createdAt
            actor {
              login
            }
          }
          ... on ReviewRequestedEvent {
            createdAt
            actor {
              login
            }
            requestedReviewer {
              ... on User {
                login
              }
              ... on Team {
                name
              }
            }
          }
          ... on ReviewRequestRemovedEvent {
            createdAt
            actor {
              login
            }
            requestedReviewer {
              ... on User {
                login
              }
              ... on Team {
                name
              }
            }
          }
          ... on LabeledEvent {
            createdAt
            actor {
              login
            }
            label {
              name
            }
          }
          ... on UnlabeledEvent {
            createdAt
            actor {
              login
            }
            label {
              name
            }
          }
          ... on ReviewDismissedEvent {
            createdAt
            actor {
              login
            }
            dismissalMessage
          }
        }
      }
    }
  }
}`

const markFileViewedMutation = `
mutation GoIDEAMarkFileViewed($pullRequestId: ID!, $path: String!) {
  markFileAsViewed(input: {pullRequestId: $pullRequestId, path: $path}) {
    clientMutationId
  }
}`

const unmarkFileViewedMutation = `
mutation GoIDEAUnmarkFileViewed($pullRequestId: ID!, $path: String!) {
  unmarkFileAsViewed(input: {pullRequestId: $pullRequestId, path: $path}) {
    clientMutationId
  }
}`

const pullRequestReviewThreadsQuery = `
query GoIDEAReviewThreads(
  $owner: String!
  $name: String!
  $number: Int!
  $pageSize: Int!
  $cursor: String
) {
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      reviewThreads(first: $pageSize, after: $cursor) {
        pageInfo {
          hasNextPage
          endCursor
        }
        nodes {
          id
          isResolved
          isOutdated
          path
          subjectType
          diffSide
          line
          originalLine
          startDiffSide
          startLine
          originalStartLine
          viewerCanReply
          viewerCanResolve
          viewerCanUnresolve
          comments(first: 100) {
            nodes {
              id
              url
              author {
                login
              }
              body
              state
              createdAt
              diffHunk
              viewerCanUpdate
              viewerCanDelete
            }
          }
        }
      }
    }
  }
}`

const pendingReviewQuery = `
query GoIDEAPendingReview($pullRequestId: ID!) {
  node(id: $pullRequestId) {
    ... on PullRequest {
      reviews(states: [PENDING], first: 1) {
        nodes {
          id
          state
          comments(first: 0) {
            totalCount
          }
        }
      }
    }
  }
}`

const createReviewMutation = `
mutation GoIDEACreateReview(
  $pullRequestId: ID!
  $event: PullRequestReviewEvent
  $body: String
  $commitOid: GitObjectID
  $threads: [DraftPullRequestReviewThread]
) {
  addPullRequestReview(input: {
    pullRequestId: $pullRequestId
    event: $event
    body: $body
    commitOID: $commitOid
    threads: $threads
  }) {
    pullRequestReview {
      id
      state
      comments(first: 0) {
        totalCount
      }
    }
  }
}`

const submitReviewMutation = `
mutation GoIDEASubmitReview(
  $reviewId: ID!
  $event: PullRequestReviewEvent!
  $body: String
) {
  submitPullRequestReview(input: {
    pullRequestReviewId: $reviewId
    event: $event
    body: $body
  }) {
    clientMutationId
  }
}`

const addReviewThreadMutation = `
mutation GoIDEAAddReviewThread(
  $pullRequestReviewId: ID
  $body: String!
  $path: String!
  $line: Int!
  $side: DiffSide
  $startLine: Int
  $startSide: DiffSide
) {
  addPullRequestReviewThread(input: {
    pullRequestReviewId: $pullRequestReviewId
    body: $body
    path: $path
    line: $line
    side: $side
    startLine: $startLine
    startSide: $startSide
  }) {
    thread {
      id
      isResolved
    }
  }
}`

const resolveReviewThreadMutation = `
mutation GoIDEAResolveReviewThread($threadId: ID!) {
  resolveReviewThread(input: {threadId: $threadId}) {
    thread {
      id
      isResolved
    }
  }
}`

const unresolveReviewThreadMutation = `
mutation GoIDEAUnresolveReviewThread($threadId: ID!) {
  unresolveReviewThread(input: {threadId: $threadId}) {
    thread {
      id
      isResolved
    }
  }
}`

const updatePullRequestMetadataMutation = `
mutation GoIDEAUpdatePullRequest(
  $pullRequestId: ID!
  $title: String
  $body: String
) {
  updatePullRequest(input: {
    pullRequestId: $pullRequestId
    title: $title
    body: $body
  }) {
    pullRequest {
      id
      title
      body
    }
  }
}`
