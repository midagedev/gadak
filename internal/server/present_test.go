package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/store"
)

// GDK-1385: the detail response is the single owner of the rendered body.
// A Linear body (markdown text, no ADF) and a migrated wall (one text node
// with the markdown inside) both come back as blocks; a rich Jira body comes
// back as it is with the loss a markdown edit would cause named. The mirror
// rows are not touched — only the response is derived.
func TestDetailPresentsBodiesAsMarkdown(t *testing.T) {
	db, cfg := fixture(t)
	ctx := context.Background()
	if _, err := db.UpsertIssues(ctx, store.Batch{
		Categories: map[string]string{"1": "new", "3": "inprogress", "10001": "done"},
		Records: []store.IssueRecord{
			{ // Linear shape: markdown in body_text, ADF column empty.
				Item: store.Item{ID: "jira:2001", SourceID: "jira", ExternalID: "2001", Key: "NMB-91",
					Title: "linear-shaped", BodyText: "## Repro\n\n- one\n- two",
					CreatedAt: "2026-07-01T00:00:00.000Z", UpdatedAt: "2026-08-01T00:00:00.000Z"},
				Issue: store.Issue{ProjectKey: "NMB", IssueType: "Bug", Status: "진행 중", StatusID: "3", StatusCategory: "inprogress"},
				Comments: []store.Comment{{ID: "jira:c-91", ExternalID: "c-91", Author: "x", AuthorID: "acc-x",
					BodyText: "**bold** reply", CreatedAt: "2026-07-02T00:00:00.000Z"}},
			},
			{ // The migrated wall: one paragraph, one text node, newlines inside.
				Item: store.Item{ID: "jira:2002", SourceID: "jira", ExternalID: "2002", Key: "NMB-92",
					Title: "wall", BodyText: "## Symptom\n\nfirst\n\n- a",
					CreatedAt: "2026-07-01T00:00:00.000Z", UpdatedAt: "2026-08-01T00:00:00.000Z"},
				Issue: store.Issue{ProjectKey: "NMB", IssueType: "Bug", Status: "진행 중", StatusID: "3", StatusCategory: "inprogress",
					DescriptionADF: json.RawMessage(`{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"## Symptom\n\nfirst\n\n- a"}]}]}`)},
			},
			{ // Rich: a panel, which markdown cannot carry.
				Item: store.Item{ID: "jira:2003", SourceID: "jira", ExternalID: "2003", Key: "NMB-93",
					Title: "rich", BodyText: "note",
					CreatedAt: "2026-07-01T00:00:00.000Z", UpdatedAt: "2026-08-01T00:00:00.000Z"},
				Issue: store.Issue{ProjectKey: "NMB", IssueType: "Bug", Status: "진행 중", StatusID: "3", StatusCategory: "inprogress",
					DescriptionADF: json.RawMessage(`{"type":"doc","version":1,"content":[{"type":"panel","attrs":{"panelType":"info"},"content":[{"type":"paragraph","content":[{"type":"text","text":"note"}]}]}]}`)},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	h := New(db, cfg)

	kinds := func(raw json.RawMessage) string {
		var n struct {
			Content []struct {
				Type string `json:"type"`
			} `json:"content"`
		}
		_ = json.Unmarshal(raw, &n)
		var out []string
		for _, c := range n.Content {
			out = append(out, c.Type)
		}
		return strings.Join(out, ",")
	}

	d := decode[detailResponse](t, get(t, h, apiBase+"NMB-91/detail/", nil))
	if got := kinds(d.DescriptionADF); got != "heading,bulletList" {
		t.Fatalf("linear body rendered as %q, want heading,bulletList", got)
	}
	if d.DescriptionMD != "## Repro\n\n- one\n- two" || len(d.FormatLoss) != 0 {
		t.Fatalf("editor source/loss: %q %v", d.DescriptionMD, d.FormatLoss)
	}
	if len(d.Comments) != 1 || !strings.Contains(string(d.Comments[0].RawBody), `"strong"`) {
		t.Fatalf("linear comment markdown must render its marks: %+v", d.Comments)
	}

	d = decode[detailResponse](t, get(t, h, apiBase+"NMB-92/detail/", nil))
	if got := kinds(d.DescriptionADF); got != "heading,paragraph,bulletList" {
		t.Fatalf("the wall rendered as %q, want heading,paragraph,bulletList", got)
	}
	if d.DescriptionMD != "## Symptom\n\nfirst\n\n- a" {
		t.Fatalf("wall source: %q", d.DescriptionMD)
	}
	// The mirror still holds the wall — the derivation is response-only.
	row, err := db.Detail(ctx, "NMB-92")
	if err != nil {
		t.Fatal(err)
	}
	if got := kinds(row.DescriptionADF); got != "paragraph" {
		t.Fatalf("mirror row must be untouched, got %q", got)
	}

	d = decode[detailResponse](t, get(t, h, apiBase+"NMB-93/detail/", nil))
	if got := kinds(d.DescriptionADF); got != "panel" {
		t.Fatalf("rich body must be displayed verbatim, got %q", got)
	}
	if len(d.FormatLoss) != 1 || d.FormatLoss[0] != "panel" {
		t.Fatalf("format_loss = %v, want [panel]", d.FormatLoss)
	}
	if d.DescriptionMD != "note" {
		t.Fatalf("rich source: %q", d.DescriptionMD)
	}
}

func TestPreviewRendersMarkdownAsADF(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)
	req := testRequest(http.MethodPost, apiBase+"preview/", strings.NewReader(`{"text":"# T\n\n- a"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		ADF json.RawMessage `json:"adf"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out.ADF), `"heading"`) || !strings.Contains(string(out.ADF), `"bulletList"`) {
		t.Fatalf("preview did not parse the markdown: %s", out.ADF)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, testRequest(http.MethodPost, apiBase+"preview/", strings.NewReader(`not json`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("garbage body: status %d", rec.Code)
	}
}
