package linear

import "strconv"

// Every GraphQL string this package sends lives in this file, so a Linear
// schema change has exactly one place to fix. Each query was executed against
// the live API (2026-08-18) before being pinned here; the tests replay
// recorded responses against these exact strings. The mutation documents
// (GDK-360) are the exception: no live mutation was allowed, so they were
// written against the documented schema (Apollo Studio reference, checked
// 2026-08-20) and their tests replay hand-built fixtures — testdata/README.md
// records which files are captures and which are built.
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

// LabelsPageSize is the inline label page on every issue row. Without an
// explicit first + pageInfo the connection was silently truncated at the
// server default; HasNextPage makes the cut observable, same contract as
// comments (GDK-263 audit, "labels 침묵 절단").
const LabelsPageSize = 50

// commentSelection is the comment node's field set, shared by the inline
// comments connection on issue queries and the commentCreate payload so the
// write path cannot drift from the read path on what a Comment is. The
// indentation is the nested queryIssues depth; GraphQL does not care.
var commentSelection = `
          id
          body
          createdAt
          updatedAt
          user {
            id
            name
            displayName
          }`

// issueSelection is the field set every Issue-returning document requests —
// the issue queries' nodes and the issueCreate/issueUpdate payloads share it,
// so a field added for the mirror appears on the write path too (the
// "same field set as the read query" contract, types.go's Issue). A var, not
// a const, for the same reason as queryIssues: the comment and label page
// sizes splice in from their named constants. The indentation is the
// queryIssues node depth; GraphQL does not care.
var issueSelection = `
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
      labels(first: ` + strconv.Itoa(LabelsPageSize) + `) {
        pageInfo {
          hasNextPage
          endCursor
        }
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
        nodes {` + commentSelection + `
        }
      }`

// queryIssues pages issues oldest-updated-first. Filter carries the
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
    nodes {` + issueSelection + `
    }
  }
}`

// The three mutations below (GDK-360) were not executed live — no mutation
// was allowed against the capture workspace. They follow Linear's mutation
// convention: the payload carries success plus the affected entity, and the
// entity uses the shared selection above so a write returns exactly what a
// read would (the mirror can commit it without a refetch). success=false is
// an application-level rejection and surfaces as an error (write.go).

// mutIssueCreate files one issue. teamId and title are the required fields of
// IssueCreateInput; everything else is optional and omitted from the input
// when unset, so Linear applies its own defaults.
var mutIssueCreate = `mutation IssueCreate($input: IssueCreateInput!) {
  issueCreate(input: $input) {
    success
    issue {` + issueSelection + `
    }
  }
}`

// mutIssueUpdate patches one issue by id. IssueUpdateInput is documented as
// "All fields are optional; only provided fields will be updated. Setting a
// field to null (where supported) will clear the field's value" — write.go
// encodes that three-way distinction (omitted / null / value) in the input it
// builds.
var mutIssueUpdate = `mutation IssueUpdate($id: String!, $input: IssueUpdateInput!) {
  issueUpdate(id: $id, input: $input) {
    success
    issue {` + issueSelection + `
    }
  }
}`

// mutCommentCreate posts one comment; body is markdown, the same format
// Comment.body carries on the read path.
var mutCommentCreate = `mutation CommentCreate($input: CommentCreateInput!) {
  commentCreate(input: $input) {
    success
    comment {` + commentSelection + `
    }
  }
}`

// mutFileUpload reserves workspace storage for one file: a signed PUT URL
// plus the headers that PUT must carry verbatim, and the stable assetUrl
// the bytes will live at (measured live 2026-08-20).
var mutFileUpload = `mutation FileUpload($contentType: String!, $filename: String!, $size: Int!) {
  fileUpload(contentType: $contentType, filename: $filename, size: $size) {
    success
    uploadFile { uploadUrl assetUrl headers { key value } }
  }
}`

// mutAttachmentCreate attaches a URL — for files, the assetUrl after the
// PUT — to an issue. Linear attachments are URL-first.
var mutAttachmentCreate = `mutation AttachmentCreate($input: AttachmentCreateInput!) {
  attachmentCreate(input: $input) {
    success
    attachment { id title url }
  }
}`
