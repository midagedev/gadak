package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/adf"
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
	// GDK-1396: the panel stands in the source as a placeholder pair.
	if !strings.HasPrefix(d.DescriptionMD, "<!-- adf:1:") || !strings.HasSuffix(d.DescriptionMD, " panel info -->\n\nnote\n\n<!-- /adf:1 -->") {
		t.Fatalf("rich source: %q", d.DescriptionMD)
	}
}

// GDK-1396: a description PUT whose text carries the detail's placeholders
// puts the preserved nodes back; one that carries none over a body that has
// them is 409 format_loss unless forced; a placeholder the body cannot
// honour is 409 placeholder with the reason; a deleted placeholder is named
// in `dropped`. Preview resolves placeholders against the base it is sent.
func TestDescriptionPlaceholdersRoundTrip(t *testing.T) {
	f, h, _ := writable(t)
	rich := `{"type":"doc","version":1,"content":[` +
		`{"type":"panel","attrs":{"panelType":"info"},"content":[{"type":"paragraph","content":[{"type":"text","text":"note"}]}]},` +
		`{"type":"paragraph","content":[{"type":"text","text":"cc "},{"type":"mention","attrs":{"id":"acc","text":"@Dana"}}]}]}`
	f.description = rich
	src := adf.Source(json.RawMessage(rich))
	kept := adf.Preserved(json.RawMessage(rich))
	if len(kept) != 2 {
		t.Fatalf("fixture: %d kept", len(kept))
	}

	put := func(body string) *httptest.ResponseRecorder {
		b, _ := json.Marshal(map[string]any{"description": body})
		return send(t, h, http.MethodPut, apiBase+"NMB-1/description/", string(b))
	}
	// No placeholders: the plain replace, refused.
	if rec := put("rewritten"); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "format_loss") {
		t.Fatalf("plain replace over preserved nodes → %d %s", rec.Code, rec.Body.String())
	}
	// Placeholders kept, interior edited.
	edited := strings.Replace(src, "note", "edited note", 1)
	rec := put(edited)
	if rec.Code != http.StatusOK {
		t.Fatalf("placeholder save → %d: %s", rec.Code, rec.Body.String())
	}
	sent := string(f.bodies["PUT /issue/NMB-1"])
	for _, want := range []string{`"panelType":"info"`, `"text":"edited note"`, `"id":"acc"`} {
		if !strings.Contains(sent, want) {
			t.Fatalf("PUT lacks %s: %s", want, sent)
		}
	}
	if strings.Contains(rec.Body.String(), `"dropped"`) {
		t.Fatalf("nothing was dropped: %s", rec.Body.String())
	}
	// The mention's placeholder deleted: the node goes, and the response says so.
	rec = put(strings.Replace(src, "cc <!-- adf:2:"+kept[1].Hash+" mention @Dana -->", "cc nobody", 1))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"dropped":["mention #2"]`) {
		t.Fatalf("dropped mention → %d %s", rec.Code, rec.Body.String())
	}
	if sent := string(f.bodies["PUT /issue/NMB-1"]); strings.Contains(sent, `"mention"`) {
		t.Fatalf("deleted mention came back: %s", sent)
	}
	// A stale placeholder: the body changed since it was read.
	f.description = strings.Replace(rich, `"panelType":"info"`, `"panelType":"error"`, 1)
	rec = put(src)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"placeholder"`) || !strings.Contains(rec.Body.String(), "changed since") {
		t.Fatalf("stale placeholder → %d %s", rec.Code, rec.Body.String())
	}
	// force: the plain replace, allowed.
	b, _ := json.Marshal(map[string]any{"description": "rewritten", "force": true})
	if rec := send(t, h, http.MethodPut, apiBase+"NMB-1/description/", string(b)); rec.Code != http.StatusOK {
		t.Fatalf("forced replace → %d %s", rec.Code, rec.Body.String())
	}

	// Preview with the base resolves; without it, a placeholder is refused.
	pb, _ := json.Marshal(map[string]any{"text": src, "base": json.RawMessage(rich)})
	req := testRequest(http.MethodPost, apiBase+"preview/", strings.NewReader(string(pb)))
	req.Header.Set("Content-Type", "application/json")
	prec := httptest.NewRecorder()
	h.ServeHTTP(prec, req)
	if prec.Code != http.StatusOK || !strings.Contains(prec.Body.String(), `"panelType":"info"`) {
		t.Fatalf("preview with base → %d %s", prec.Code, prec.Body.String())
	}
	pb, _ = json.Marshal(map[string]any{"text": src})
	prec = httptest.NewRecorder()
	h.ServeHTTP(prec, testRequest(http.MethodPost, apiBase+"preview/", strings.NewReader(string(pb))))
	if prec.Code != http.StatusConflict {
		t.Fatalf("preview without base → %d %s", prec.Code, prec.Body.String())
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

// GDK-1396: a comment or a new issue has no body to substitute from, so a
// marker in its text is 409 placeholder, never stored as visible text.
func TestCommentAndCreateRefuseStrayPlaceholders(t *testing.T) {
	f, h, _ := writable(t)
	marker := "<!-- adf:1:0badf00d mention @Dana --> look"
	b, _ := json.Marshal(map[string]any{"text": marker})
	if rec := send(t, h, http.MethodPost, apiBase+"NMB-1/comment/", string(b)); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"placeholder"`) {
		t.Fatalf("comment with a marker → %d %s", rec.Code, rec.Body.String())
	}
	if f.called("POST /issue/NMB-1/comment") {
		t.Fatalf("refused comment still reached the origin: %v", f.calls)
	}
}
