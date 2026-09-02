package linear

import (
	"context"
	"errors"
	"fmt"
)

// The verbs `gadak migrate --to linear` needs beyond write.go (GDK-1265):
// relations and labels. Same discipline as write.go — every call is a user
// action routed through the origin, ids never display names.

// RelationTypes is Linear's IssueRelationType enum (introspected
// 2026-09-02). CreateRelation refuses anything else before the wire.
var RelationTypes = map[string]bool{"blocks": true, "duplicate": true, "related": true, "similar": true}

// CreateRelation links two issues: issueID <typ> relatedID (for "blocks",
// issueID blocks relatedID). Linear materializes the inverse itself.
func (c *Client) CreateRelation(ctx context.Context, issueID, relatedID, typ string) error {
	if issueID == "" || relatedID == "" {
		return errors.New("linear: issueId and relatedIssueId are required")
	}
	if !RelationTypes[typ] {
		return fmt.Errorf("linear: relation type %q is not one of blocks|duplicate|related|similar", typ)
	}
	var res struct {
		IssueRelationCreate struct {
			Success bool `json:"success"`
		} `json:"issueRelationCreate"`
	}
	input := map[string]any{"issueId": issueID, "relatedIssueId": relatedID, "type": typ}
	if err := c.gqlWrite(ctx, mutIssueRelationCreate, map[string]any{"input": input}, &res); err != nil {
		return err
	}
	if !res.IssueRelationCreate.Success {
		return fmt.Errorf("POST /graphql: linear: issueRelationCreate returned success=false")
	}
	return nil
}

// Labels lists every label the credential can see — workspace-level ones
// (team null) and team ones alike, since both apply to an issue and a
// team label cannot reuse a workspace label's name.
func (c *Client) Labels(ctx context.Context) ([]Label, error) {
	out := []Label{}
	after := ""
	for {
		vars := map[string]any{}
		if after != "" {
			vars["after"] = after
		}
		var page struct {
			IssueLabels LabelConn `json:"issueLabels"`
		}
		if err := c.gql(ctx, queryLabels, vars, &page); err != nil {
			return nil, err
		}
		out = append(out, page.IssueLabels.Nodes...)
		if !page.IssueLabels.PageInfo.HasNextPage || page.IssueLabels.PageInfo.EndCursor == "" {
			return out, nil
		}
		after = page.IssueLabels.PageInfo.EndCursor
	}
}

// CreateLabel creates a team-scoped label.
func (c *Client) CreateLabel(ctx context.Context, teamID, name string) (Label, error) {
	if teamID == "" || name == "" {
		return Label{}, errors.New("linear: teamId and name are required")
	}
	var res struct {
		IssueLabelCreate struct {
			Success    bool  `json:"success"`
			IssueLabel Label `json:"issueLabel"`
		} `json:"issueLabelCreate"`
	}
	input := map[string]any{"teamId": teamID, "name": name}
	if err := c.gqlWrite(ctx, mutIssueLabelCreate, map[string]any{"input": input}, &res); err != nil {
		return Label{}, err
	}
	if !res.IssueLabelCreate.Success {
		return Label{}, fmt.Errorf("POST /graphql: linear: issueLabelCreate returned success=false")
	}
	return res.IssueLabelCreate.IssueLabel, nil
}
