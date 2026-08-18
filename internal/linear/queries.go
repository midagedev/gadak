package linear

import "strconv"

// Every GraphQL string this package sends lives in this file, so a Linear
// schema change has exactly one place to fix. Each query was executed against
// the live API (2026-08-18) before being pinned here; the tests replay
// recorded responses against these exact strings.
//
// Filters are always passed as whole-object variables ($filter), never
// interpolated into the string — the query text has no caller-controlled
// bytes in it.
//
// Filters are always passed as whole-object variables ($filter), never
// interpolated into the string — the query text has no caller-controlled
// bytes in it.

// queryViewer is the smallest question that proves the credential works: the
// equivalent of Jira's /myself for `gadak init`. The response carries personal
// data (name, email); see User.
const queryViewer = `query Viewer {
  viewer {
    id
    name
    displayName
    email
  }
}`

// queryTeams lists the teams the credential can see. A workspace has few
// teams, but teams is a connection like everything else, so the client still
// follows the cursor.
const queryTeams = `query Teams($after: String) {
  teams(first: 50, after: $after) {
    pageInfo {
      hasNextPage
      endCursor
    }
    nodes {
      id
      key
      name
      private
    }
  }
}`

// queryWorkflowStates lists one team's status catalog. Filter shape verified
// live: workflowStates(filter: {team: {id: {eq: "<uuid>"}}}) returns exactly
// that team's states.
const queryWorkflowStates = `query WorkflowStates($filter: WorkflowStateFilter, $after: String) {
  workflowStates(filter: $filter, first: 100, after: $after) {
    pageInfo {
      hasNextPage
      endCursor
    }
    nodes {
      id
      name
      type
      position
    }
  }
}`

// CommentsPageSize is the inline comment page on every issue row. Issues with
// more comments than this set Comments.PageInfo.HasNextPage, which is the
// contract that makes the truncation visible (see Issue.Comments).
const CommentsPageSize = 50

// queryIssues pages issues oldest-updated-first. A var, not a const: the
// inline comment page size is spliced from CommentsPageSize so "50" has one
// owner. Filter carries the
// incremental-sync watermark (updatedAt.gte) and the team scope; both were
// verified live. orderBy: updatedAt has no descending direction argument on
// Query.issues (orderDirection does not exist there — verified by rejection),
// and ascending is what a watermark-driven sync wants anyway.
//
// includeArchived is passed as a variable so reconcile passes can see archived
// rows (archivedAt set) instead of mistaking them for deleted.
//
// Not requested here, deliberately: stateHistory / history (unbounded nested
// connections — the status_changed_at derivation is specified in MAPPING.md
// and belongs to the sync-wiring round) and attachments.
var queryIssues = `query Issues($first: Int, $after: String, $filter: IssueFilter, $includeArchived: Boolean) {
  issues(first: $first, after: $after, filter: $filter, includeArchived: $includeArchived, orderBy: updatedAt) {
    pageInfo {
      hasNextPage
      endCursor
    }
    nodes {
      id
      identifier
      number
      title
      description
      url
      createdAt
      updatedAt
      archivedAt
      priority
      priorityLabel
      dueDate
      state {
        id
        name
        type
        position
      }
      team {
        id
        key
        name
      }
      assignee {
        id
        name
        displayName
        email
      }
      creator {
        id
        name
        displayName
        email
      }
      labels {
        nodes {
          id
          name
        }
      }
      parent {
        id
        identifier
      }
      comments(first: ` + strconv.Itoa(CommentsPageSize) + `) {
        pageInfo {
          hasNextPage
          endCursor
        }
        nodes {
          id
          body
          createdAt
          updatedAt
          user {
            id
            name
            displayName
          }
        }
      }
    }
  }
}`
