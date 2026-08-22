package linear

// The payload shapes below are only the parts a mirror needs. Field names and
// shapes were verified against the live GraphQL API (2026-08-18); anything not
// listed here is simply not requested by the queries in queries.go.

// User is a Linear account reference. Name/DisplayName/Email are personal
// data: this package never logs them, and callers must not either.
type User struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Email       string `json:"email"`
}

// Team is the scope unit a workspace divides issues by — the closest Linear
// counterpart to a Jira project (see MAPPING.md, "project_key").
type Team struct {
	ID      string `json:"id"`
	Key     string `json:"key"`
	Name    string `json:"name"`
	Private bool   `json:"private"`
}

// WorkflowState is one status in a team's workflow. Type is the stable axis:
// name is display-only and can be anything ("In Progress", "진행 중", a custom
// "In Review"); logic must key on Type or ID, never on Name — the same
// localization hazard Jira's status names carry.
//
// Verified Type values: backlog, unstarted, started, completed, canceled,
// duplicate (and triage on teams with triage enabled). The set is open:
// Linear added duplicate after the original six, so code consuming Type must
// tolerate values it does not know (see MAPPING.md, "status_category").
type WorkflowState struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Position int    `json:"position"`
}

// Label is an issue label. Unlike Jira labels (plain strings), Linear labels
// carry an id; the mirror stores names, so a label renamed upstream changes
// the stored key the same way a Jira label edit does.
type Label struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// LabelConn is the nested labels connection on an issue. PageInfo.HasNextPage
// set means the issue has more labels than LabelsPageSize — truncation is
// observable, never silent (same contract as CommentConn). CompleteLabels
// follows the cursor.
type LabelConn struct {
	PageInfo PageInfo `json:"pageInfo"`
	Nodes    []Label  `json:"nodes"`
}

// PageInfo is the cursor pagination envelope on every connection.
type PageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

// Comment is one issue comment. Body is markdown — not ADF. The Jira path
// stores body_adf; stuffing markdown into that column would be a false
// mapping, so Linear comments must stay markdown (body_text / raw), and the
// rendering surface is a post-connector decision. See MAPPING.md, "comments".
type Comment struct {
	ID        string `json:"id"`
	Body      string `json:"body"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
	User      *User  `json:"user"`
}

// CommentConn is the nested comments connection on an issue.
type CommentConn struct {
	PageInfo PageInfo  `json:"pageInfo"`
	Nodes    []Comment `json:"nodes"`
}

// IssueAttachment is one node of Issue.attachments. Title is the filename;
// URL is the origin content URL (store it verbatim — the proxy must not
// reconstruct a Jira path). Metadata may carry size/mimeType; those fields
// are not first-class on the Linear attachment type (MAPPING.md).
type IssueAttachment struct {
	ID        string         `json:"id"`
	Title     string         `json:"title"`
	URL       string         `json:"url"`
	CreatedAt string         `json:"createdAt"`
	Metadata  map[string]any `json:"metadata"`
}

// AttachmentConn is the nested attachments connection on an issue.
// PageInfo.HasNextPage means more than AttachmentsPageSize — truncation is
// observable. CompleteAttachments follows the cursor.
type AttachmentConn struct {
	PageInfo PageInfo          `json:"pageInfo"`
	Nodes    []IssueAttachment `json:"nodes"`
}

// ParentRef is the embedded parent issue. Linear parents are generic sub-issue
// nesting, not epics; see MAPPING.md, "epic / parent".
type ParentRef struct {
	ID         string `json:"id"`
	Identifier string `json:"identifier"`
}

// Issue is one row of the issues connection. Timestamps are Linear's verbatim
// ISO-8601 UTC strings with milliseconds (the format the mirror stores
// unmodified). ArchivedAt is empty for live issues.
type Issue struct {
	ID            string `json:"id"`
	Identifier    string `json:"identifier"`
	Number        int    `json:"number"`
	Title         string `json:"title"`
	Description   string `json:"description"`
	URL           string `json:"url"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
	ArchivedAt    string `json:"archivedAt"`
	Priority      int    `json:"priority"`
	PriorityLabel string `json:"priorityLabel"`
	DueDate       string `json:"dueDate"`

	State WorkflowState `json:"state"`
	Team  struct {
		ID   string `json:"id"`
		Key  string `json:"key"`
		Name string `json:"name"`
	} `json:"team"`
	Assignee *User      `json:"assignee"`
	Creator  *User      `json:"creator"`
	Labels   LabelConn  `json:"labels"`
	Parent   *ParentRef `json:"parent"`

	// Comments is the first inline page (CommentsPageSize). A page with
	// PageInfo.HasNextPage means the issue has more comments than that and
	// the rest need a follow-up fetch — the flag exists so truncation is
	// observable, never silent. CompleteComments follows the cursor.
	Comments CommentConn `json:"comments"`

	// Attachments is the first inline page (AttachmentsPageSize).
	// CompleteAttachments follows HasNextPage the same way comments do.
	Attachments AttachmentConn `json:"attachments"`
}

// IssueConnection and its siblings are the standard Linear connection shape.
type IssueConnection struct {
	PageInfo PageInfo `json:"pageInfo"`
	Nodes    []Issue  `json:"nodes"`
}

type TeamConnection struct {
	PageInfo PageInfo `json:"pageInfo"`
	Nodes    []Team   `json:"nodes"`
}

type WorkflowStateConnection struct {
	PageInfo PageInfo        `json:"pageInfo"`
	Nodes    []WorkflowState `json:"nodes"`
}
