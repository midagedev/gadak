package linear

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// This file is the mutating half of the client (GDK-360): the three verbs the
// origin.Writer adapter (GDK-361) routes through Linear. It is this package's
// instance of the discipline internal/jira/write.go documents: every call
// here is a user action, nothing writes on a schedule, and a failed write is
// reported to the person who asked for it. Writes pass through the origin —
// these methods are that origin path; the mirror is never written directly.
//
// Field semantics come from the documented schema (Apollo Studio reference,
// checked 2026-08-20; no live mutation was allowed to verify them): of
// IssueUpdateInput it says "All fields are optional; only provided fields
// will be updated. Setting a field to null (where supported) will clear the
// field's value." The input maps below encode exactly that three-way
// distinction — omitted key = unchanged, explicit null = cleared, value =
// set. The tests replay hand-built fixtures (testdata/README.md).
//
// ids, never display names: stateId is a WorkflowState UUID, assigneeId a
// user UUID, label ids label UUIDs — the same rule the read path's filters
// follow (CLAUDE.md; a state *name* is display text and localizable).

// IssueCreate is the input to CreateIssue. Zero-value strings and slices are
// omitted from the wire input so Linear applies its own defaults.
type IssueCreate struct {
	// TeamID and Title are the required fields of IssueCreateInput; the
	// client checks them first so the error names the field instead of the
	// server's generic rejection.
	TeamID string
	Title  string
	// Description is markdown, the same format Issue.description carries.
	Description string
	// StateID is a workflow-state UUID from WorkflowStates.
	StateID string
	// Priority is Linear's 0-4 scale (0 = No priority … 4 = Low; MAPPING.md
	// "priority"). Nil omits the field.
	Priority *int
	// AssigneeID is a user UUID; empty omits (a create has nothing to clear).
	AssigneeID string
	// LabelIDs becomes the issue's whole label set; empty omits.
	LabelIDs []string
	// DueDate is "YYYY-MM-DD" exactly; empty omits.
	DueDate string
	// ParentID nests the new issue under an existing one (sub-issue);
	// empty omits.
	ParentID string
	// CreatedAt backdates the issue (ISO-8601 UTC) — the import case, where
	// the source's timestamp is the truth. Empty omits and Linear stamps
	// now. Whether the server honors it is the caller's to verify against
	// the returned Issue.CreatedAt.
	CreatedAt string
}

// IssueUpdate is the patch for UpdateIssue. A nil field is omitted from the
// wire input — unchanged. Set fields are sent as values; the one special
// case is AssigneeID, where the empty string means the explicit null that
// unassigns.
type IssueUpdate struct {
	Title       *string
	Description *string
	// StateID is a workflow-state UUID from WorkflowStates.
	StateID *string
	// Priority is Linear's 0-4 scale (MAPPING.md "priority").
	Priority *int
	// AssigneeID: a pointer to a user UUID assigns; a pointer to the empty
	// string sends assigneeId: null, which the documented schema defines as
	// clearing (unassigning); nil leaves the assignee unchanged.
	AssigneeID *string
	// LabelIDs: a pointer to a slice replaces the whole label set (an empty
	// slice clears labels); nil leaves them unchanged.
	LabelIDs *[]string
	// DueDate: a pointer to "YYYY-MM-DD" sets; nil is unchanged. Clearing a
	// due date (explicit null) is not expressible in this shape — extend it
	// with the AssigneeID convention if the adapter ever needs unsetting.
	DueDate *string
	// ParentID: a pointer to an issue UUID nests under it; nil is unchanged.
	ParentID *string
}

// CreateIssue files one issue and returns it with the full read-path field
// set, so the mirror can commit the row without a refetch.
func (c *Client) CreateIssue(ctx context.Context, in IssueCreate) (Issue, error) {
	if in.TeamID == "" {
		return Issue{}, errors.New("linear: teamId is required")
	}
	if in.Title == "" {
		return Issue{}, errors.New("linear: title is required")
	}
	input := map[string]any{
		"teamId": in.TeamID,
		"title":  in.Title,
	}
	if in.Description != "" {
		input["description"] = in.Description
	}
	if in.StateID != "" {
		input["stateId"] = in.StateID
	}
	if in.Priority != nil {
		if err := validatePriority(*in.Priority); err != nil {
			return Issue{}, err
		}
		input["priority"] = *in.Priority
	}
	if in.AssigneeID != "" {
		input["assigneeId"] = in.AssigneeID
	}
	if len(in.LabelIDs) > 0 {
		input["labelIds"] = in.LabelIDs
	}
	if in.DueDate != "" {
		if err := validateDueDate(in.DueDate); err != nil {
			return Issue{}, err
		}
		input["dueDate"] = in.DueDate
	}
	if in.ParentID != "" {
		input["parentId"] = in.ParentID
	}
	if in.CreatedAt != "" {
		input["createdAt"] = in.CreatedAt
	}

	var res struct {
		IssueCreate struct {
			Success bool  `json:"success"`
			Issue   Issue `json:"issue"`
		} `json:"issueCreate"`
	}
	if err := c.gqlWrite(ctx, mutIssueCreate, map[string]any{"input": input}, &res); err != nil {
		return Issue{}, err
	}
	if !res.IssueCreate.Success {
		return Issue{}, fmt.Errorf("POST /graphql: linear: issueCreate returned success=false")
	}
	return res.IssueCreate.Issue, nil
}

// UpdateIssue patches one issue by id and returns the updated issue with the
// full read-path field set. An all-nil IssueUpdate is a legal wire no-op —
// the input travels as {} and Linear answers success with the unchanged
// issue, the same tolerance jira's EditIssue gives empty maps.
func (c *Client) UpdateIssue(ctx context.Context, id string, in IssueUpdate) (Issue, error) {
	if id == "" {
		return Issue{}, errors.New("linear: id is required")
	}
	input := map[string]any{}
	if in.Title != nil {
		input["title"] = *in.Title
	}
	if in.Description != nil {
		input["description"] = *in.Description
	}
	if in.StateID != nil {
		input["stateId"] = *in.StateID
	}
	if in.Priority != nil {
		if err := validatePriority(*in.Priority); err != nil {
			return Issue{}, err
		}
		input["priority"] = *in.Priority
	}
	if in.AssigneeID != nil {
		if *in.AssigneeID == "" {
			// The explicit null: the documented schema clears the field.
			input["assigneeId"] = nil
		} else {
			input["assigneeId"] = *in.AssigneeID
		}
	}
	if in.LabelIDs != nil {
		input["labelIds"] = *in.LabelIDs
	}
	if in.DueDate != nil {
		if err := validateDueDate(*in.DueDate); err != nil {
			return Issue{}, err
		}
		input["dueDate"] = *in.DueDate
	}
	if in.ParentID != nil {
		input["parentId"] = *in.ParentID
	}

	var res struct {
		IssueUpdate struct {
			Success bool  `json:"success"`
			Issue   Issue `json:"issue"`
		} `json:"issueUpdate"`
	}
	if err := c.gqlWrite(ctx, mutIssueUpdate, map[string]any{"id": id, "input": input}, &res); err != nil {
		return Issue{}, err
	}
	if !res.IssueUpdate.Success {
		return Issue{}, fmt.Errorf("POST /graphql: linear: issueUpdate returned success=false")
	}
	return res.IssueUpdate.Issue, nil
}

// CreateComment posts one comment; body is markdown, the same format
// Comment.body carries on the read path (never ADF — see MAPPING.md
// "comments"). Whether an empty body is acceptable is Linear's call; the
// client does not invent a rule for it.
func (c *Client) CreateComment(ctx context.Context, issueID string, body string) (Comment, error) {
	return c.CreateCommentAt(ctx, issueID, body, "")
}

// CreateCommentAt is CreateComment with a backdated createdAt (ISO-8601
// UTC) — the import case. Empty createdAt omits the field.
func (c *Client) CreateCommentAt(ctx context.Context, issueID, body, createdAt string) (Comment, error) {
	if issueID == "" {
		return Comment{}, errors.New("linear: issueId is required")
	}
	input := map[string]any{
		"issueId": issueID,
		"body":    body,
	}
	if createdAt != "" {
		input["createdAt"] = createdAt
	}

	var res struct {
		CommentCreate struct {
			Success bool    `json:"success"`
			Comment Comment `json:"comment"`
		} `json:"commentCreate"`
	}
	if err := c.gqlWrite(ctx, mutCommentCreate, map[string]any{"input": input}, &res); err != nil {
		return Comment{}, err
	}
	if !res.CommentCreate.Success {
		return Comment{}, fmt.Errorf("POST /graphql: linear: commentCreate returned success=false")
	}
	return res.CommentCreate.Comment, nil
}

// validatePriority rejects anything off Linear's 0-4 scale before it leaves
// the process — a Jira-style rank or a 1-based slip is a caller bug, and a
// named error beats a server round-trip. If Linear ever widens the scale,
// this is the one place to change (MAPPING.md "priority").
func validatePriority(p int) error {
	if p < 0 || p > 4 {
		return fmt.Errorf("linear: priority %d is outside Linear's 0-4 scale", p)
	}
	return nil
}

// validateDueDate enforces the wire format of a Linear Date: "YYYY-MM-DD"
// exactly (zero-padded, no time, no zone).
func validateDueDate(d string) error {
	if _, err := time.Parse("2006-01-02", d); err != nil {
		return fmt.Errorf("linear: dueDate %q is not YYYY-MM-DD", d)
	}
	return nil
}

// Attachment is the record attachmentCreate returns. Linear attachments are
// URL-first: a file becomes an attachment only after it is uploaded to the
// workspace's storage and its assetUrl is attached.
type Attachment struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

// UploadTarget is what fileUpload hands back: a signed PUT destination and
// the stable assetUrl the uploaded bytes will live at. Headers must be sent
// verbatim on the PUT (measured 2026-08-20: the signed URL rejects the write
// without them).
type UploadTarget struct {
	UploadURL string
	AssetURL  string
	Headers   map[string]string
}

// UploadFile reserves storage for one file and returns where to PUT it.
// The PUT itself is the caller's (it goes to storage, not the GraphQL
// endpoint, and must not carry the API key).
func (c *Client) UploadFile(ctx context.Context, filename, contentType string, size int) (UploadTarget, error) {
	if filename == "" || contentType == "" || size <= 0 {
		return UploadTarget{}, errors.New("linear: filename, contentType and a positive size are required")
	}
	var res struct {
		FileUpload struct {
			Success    bool `json:"success"`
			UploadFile struct {
				UploadURL string `json:"uploadUrl"`
				AssetURL  string `json:"assetUrl"`
				Headers   []struct {
					Key   string `json:"key"`
					Value string `json:"value"`
				} `json:"headers"`
			} `json:"uploadFile"`
		} `json:"fileUpload"`
	}
	if err := c.gqlWrite(ctx, mutFileUpload, map[string]any{
		"contentType": contentType, "filename": filename, "size": size,
	}, &res); err != nil {
		return UploadTarget{}, err
	}
	if !res.FileUpload.Success {
		return UploadTarget{}, fmt.Errorf("POST /graphql: linear: fileUpload returned success=false")
	}
	t := UploadTarget{
		UploadURL: res.FileUpload.UploadFile.UploadURL,
		AssetURL:  res.FileUpload.UploadFile.AssetURL,
		Headers:   map[string]string{},
	}
	for _, h := range res.FileUpload.UploadFile.Headers {
		t.Headers[h.Key] = h.Value
	}
	return t, nil
}

// CreateAttachment attaches an already-uploaded (or external) URL to an
// issue. For files, the url is the assetUrl UploadFile returned after the
// caller's PUT succeeded.
func (c *Client) CreateAttachment(ctx context.Context, issueID, url, title string) (Attachment, error) {
	if issueID == "" || url == "" {
		return Attachment{}, errors.New("linear: issueId and url are required")
	}
	input := map[string]any{"issueId": issueID, "url": url}
	if title != "" {
		input["title"] = title
	}
	var res struct {
		AttachmentCreate struct {
			Success    bool       `json:"success"`
			Attachment Attachment `json:"attachment"`
		} `json:"attachmentCreate"`
	}
	if err := c.gqlWrite(ctx, mutAttachmentCreate, map[string]any{"input": input}, &res); err != nil {
		return Attachment{}, err
	}
	if !res.AttachmentCreate.Success {
		return Attachment{}, fmt.Errorf("POST /graphql: linear: attachmentCreate returned success=false")
	}
	return res.AttachmentCreate.Attachment, nil
}
