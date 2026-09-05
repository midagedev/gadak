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

// AttachmentsPageSize is the inline attachment page on every issue row.
const AttachmentsPageSize = 50

// RelationsPageSize is the inline page on Issue.relations and
// Issue.inverseRelations (GDK-1299). Same contract as the three above:
// HasNextPage marks the cut and the sync pass counts it. No follow-up
// fetch yet — an issue related to more than fifty others is a shape none of
// the measured workspaces have, and the count will say when one does.
const RelationsPageSize = 50

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

// labelSelection is the label node's field set, shared by the inline labels
// connection and the IssueLabels follow-up so CompleteLabels cannot drift
// from the issue query.
var labelSelection = `
          id
          name`

// attachmentSelection is the attachment node's field set, shared by the
// inline attachments connection and the IssueAttachments follow-up so
// CompleteAttachments cannot drift from the issue query.
var attachmentSelection = `
          id
          title
          url
          createdAt
          metadata`

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
        nodes {` + labelSelection + `
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
      }
      attachments(first: ` + strconv.Itoa(AttachmentsPageSize) + `) {
        pageInfo {
          hasNextPage
          endCursor
        }
        nodes {` + attachmentSelection + `
        }
      }
      relations(first: ` + strconv.Itoa(RelationsPageSize) + `) {
        pageInfo {
          hasNextPage
          endCursor
        }
        nodes {` + relationSelection + `
        }
      }
      inverseRelations(first: ` + strconv.Itoa(RelationsPageSize) + `) {
        pageInfo {
          hasNextPage
          endCursor
        }
        nodes {` + relationSelection + `
        }
      }`

// relationSelection is one IssueRelation node: the type and both ends by
// identifier — that is all the mirror's links table holds (GDK-1299). The
// two connections carry the same selection because Linear's relation node
// is the same type from either end; which end is "me" is the connection.
const relationSelection = `
          id
          type
          issue {
            id
            identifier
          }
          relatedIssue {
            id
            identifier
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
// and belongs to the sync-wiring round).
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

// queryIssue fetches one issue by id — Linear's issue(id:) accepts both the
// UUID and the human identifier ("MID-5"), which is what lets the write
// adapter resolve keys the way users type them. Same selection as the paged
// read so the mirror row shape is identical.
var queryIssue = `query Issue($id: String!) {
  issue(id: $id) {` + issueSelection + `}
}`

// queryIssueComments follows the comments cursor on one issue when the inline
// page (CommentsPageSize) set HasNextPage. Same node selection as the inline
// connection so the mirrored child list is one shape.
var queryIssueComments = `query IssueComments($id: String!, $after: String) {
  issue(id: $id) {
    comments(first: ` + strconv.Itoa(CommentsPageSize) + `, after: $after) {
      pageInfo {
        hasNextPage
        endCursor
      }
      nodes {` + commentSelection + `
      }
    }
  }
}`

// queryIssueLabels follows the labels cursor on one issue when the inline
// page (LabelsPageSize) set HasNextPage.
var queryIssueLabels = `query IssueLabels($id: String!, $after: String) {
  issue(id: $id) {
    labels(first: ` + strconv.Itoa(LabelsPageSize) + `, after: $after) {
      pageInfo {
        hasNextPage
        endCursor
      }
      nodes {` + labelSelection + `
      }
    }
  }
}`

// queryIssueAttachments follows the attachments cursor on one issue when the
// inline page (AttachmentsPageSize) set HasNextPage.
var queryIssueAttachments = `query IssueAttachments($id: String!, $after: String) {
  issue(id: $id) {
    attachments(first: ` + strconv.Itoa(AttachmentsPageSize) + `, after: $after) {
      pageInfo {
        hasNextPage
        endCursor
      }
      nodes {` + attachmentSelection + `
      }
    }
  }
}`

// queryUsers answers assignee search: workspace members whose display
// name or email contains the query, case-insensitively. The client
// builds filter.or so an email-shaped query still hits.
const queryUsers = `query Users($filter: UserFilter, $after: String) {
  users(filter: $filter, first: 50, after: $after) {
    pageInfo { hasNextPage endCursor }
    nodes { id name displayName email }
  }
}`

// The migrate verbs (GDK-1265). Input shapes were introspected live on
// 2026-09-02 (IssueRelationCreateInput, IssueLabelCreateInput,
// IssueCreateInput.parentId/createdAt, CommentCreateInput.createdAt).

// queryLabels lists every label the credential can see, unfiltered:
// workspace-level labels have team = null and a team filter would hide
// them, yet they apply to the team's issues and block same-named creates.
var queryLabels = `query Labels($after: String) {
  issueLabels(first: 250, after: $after) {
    pageInfo { hasNextPage endCursor }
    nodes {` + labelSelection + `
    }
  }
}`

var mutIssueLabelCreate = `mutation IssueLabelCreate($input: IssueLabelCreateInput!) {
  issueLabelCreate(input: $input) {
    success
    issueLabel {` + labelSelection + `
    }
  }
}`

const mutIssueRelationCreate = `mutation IssueRelationCreate($input: IssueRelationCreateInput!) {
  issueRelationCreate(input: $input) {
    success
    issueRelation { id }
  }
}`
