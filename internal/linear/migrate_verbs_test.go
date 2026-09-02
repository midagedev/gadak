package linear

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// The migrate verbs (GDK-1265): parent / createdAt on create, relation and
// label creation, and the label catalog. Fixtures are hand-built like the
// GDK-360 ones (testdata/README.md).

func decodeQueryInput(t *testing.T, r *http.Request, input *map[string]json.RawMessage) string {
	t.Helper()
	var body struct {
		Query     string `json:"query"`
		Variables struct {
			Input json.RawMessage `json:"input"`
		} `json:"variables"`
	}
	decode(t, r, &body)
	if len(body.Variables.Input) > 0 {
		if err := json.Unmarshal(body.Variables.Input, input); err != nil {
			t.Fatalf("variables.input: %v", err)
		}
	}
	return body.Query
}

func TestCreateIssueSendsParentAndCreatedAt(t *testing.T) {
	var input map[string]json.RawMessage
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decodeInput(t, r, &input)
		writeFixture(w, t, "issue_create.json")
	}))
	_, err := c.CreateIssue(context.Background(), IssueCreate{
		TeamID: "00000000-0000-4000-8000-000000000003", Title: "child",
		ParentID: "00000000-0000-4000-8000-000000000012", CreatedAt: "2026-01-02T03:04:05.000Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(input["parentId"]) != `"00000000-0000-4000-8000-000000000012"` || string(input["createdAt"]) != `"2026-01-02T03:04:05.000Z"` {
		t.Fatalf("input %v", input)
	}
}

func TestUpdateIssueParent(t *testing.T) {
	var input map[string]json.RawMessage
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decodeInput(t, r, &input)
		writeFixture(w, t, "issue_update.json")
	}))
	if _, err := c.UpdateIssue(context.Background(), "x", IssueUpdate{ParentID: ptr("p-1")}); err != nil {
		t.Fatal(err)
	}
	if len(input) != 1 || string(input["parentId"]) != `"p-1"` {
		t.Fatalf("input %v", input)
	}
}

func TestCreateCommentAtSendsCreatedAt(t *testing.T) {
	var input map[string]json.RawMessage
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decodeInput(t, r, &input)
		writeFixture(w, t, "comment_create.json")
	}))
	if _, err := c.CreateCommentAt(context.Background(), "i-1", "hi", "2026-01-02T03:04:05.000Z"); err != nil {
		t.Fatal(err)
	}
	if string(input["createdAt"]) != `"2026-01-02T03:04:05.000Z"` || string(input["body"]) != `"hi"` {
		t.Fatalf("input %v", input)
	}
	// The plain verb sends no createdAt at all.
	input = nil
	if _, err := c.CreateComment(context.Background(), "i-1", "hi"); err != nil {
		t.Fatal(err)
	}
	if _, has := input["createdAt"]; has {
		t.Fatalf("CreateComment must omit createdAt: %v", input)
	}
}

func TestCreateRelation(t *testing.T) {
	var query string
	var input map[string]json.RawMessage
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = decodeQueryInput(t, r, &input)
		writeFixture(w, t, "relation_create.json")
	}))
	if err := c.CreateRelation(context.Background(), "a", "b", "blocks"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(query, "issueRelationCreate") {
		t.Fatalf("query %q", query)
	}
	if string(input["issueId"]) != `"a"` || string(input["relatedIssueId"]) != `"b"` || string(input["type"]) != `"blocks"` {
		t.Fatalf("input %v", input)
	}
	if err := c.CreateRelation(context.Background(), "a", "b", "clones"); err == nil {
		t.Fatal("unknown relation type must be refused before the wire")
	}
}

func TestLabelsAndCreateLabel(t *testing.T) {
	var input map[string]json.RawMessage
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := decodeQueryInput(t, r, &input)
		if strings.Contains(q, "issueLabelCreate") {
			writeFixture(w, t, "label_create.json")
			return
		}
		writeFixture(w, t, "labels.json")
	}))
	labels, err := c.Labels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(labels) != 2 || labels[0].Name != "Bug" {
		t.Fatalf("labels %+v", labels)
	}
	l, err := c.CreateLabel(context.Background(), "00000000-0000-4000-8000-000000000003", "gadak-migrate")
	if err != nil {
		t.Fatal(err)
	}
	if l.ID == "" || string(input["name"]) != `"gadak-migrate"` || string(input["teamId"]) != `"00000000-0000-4000-8000-000000000003"` {
		t.Fatalf("label %+v input %v", l, input)
	}
}
