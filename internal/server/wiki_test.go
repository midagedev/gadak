package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
)

// serverWikiBody is the spec's sample: markup that is wiki, not markdown —
// "h2." and "{code}" and "* one" are literal characters on a Jira Server
// origin, and a markdown parse of them is a misreading (GDK-1637).
const serverWikiBody = "h2. Heading\n\n*bold* and {{code}}\n\n* one\n* two\n\n{code:java}\nSystem.out.println(1);\n{code}\n"

const serverWikiComment = "reproduced on *staging* — see {color:red}STEP 3{color}"

// wikiWorkspace is writable() pointed at a jira-server origin, with NMB-1
// rewritten to the mirror shape a v2 sync leaves: body_text holds the wiki
// string, description_adf the origin's own JSON string (sync copies the field
// as it came), and the comment the same pair.
func wikiWorkspace(t *testing.T) (*fakeJira, http.Handler) {
	t.Helper()
	f := newFakeJira(t)
	db, cfg := fixture(t)
	cfg.Kind = config.OriginJiraServer
	cfg.Site = f.URL
	cfg.Token = "pat"
	cfg.Email = "" // Server authenticates with the PAT alone (GDK-1640)
	descJSON, err := json.Marshal(serverWikiBody)
	if err != nil {
		t.Fatal(err)
	}
	commentJSON, err := json.Marshal(serverWikiComment)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertIssues(context.Background(), store.Batch{
		// Force: the fixture wrote this item at the same updated_at, and the
		// change detector would read the rewrite as "no new updated".
		Force: true,
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "jira:1001", SourceID: "jira", ExternalID: "1001", Key: "NMB-1",
				Title: "batch worker drops the last page", BodyText: serverWikiBody,
				CreatedAt: "2026-07-01T00:00:00.000Z", UpdatedAt: "2026-08-01T00:00:00.000Z",
			},
			Issue: store.Issue{
				ProjectKey: "NMB", IssueType: "Bug", IssueTypeID: "10004",
				Status: "진행 중", StatusID: "3", StatusCategory: "inprogress",
				Priority: "High", Assignee: "김현철", AssigneeID: "acc-hc",
				DescriptionADF: descJSON,
			},
			Comments: []store.Comment{{
				ID: "jira:c-1", ExternalID: "c-1", Author: "김현철", AuthorID: "acc-hc",
				BodyADF: commentJSON, BodyText: serverWikiComment,
				CreatedAt: "2026-07-02T00:00:00.000Z",
			}},
		}},
	}); err != nil {
		t.Fatalf("rewrite NMB-1 to the v2 mirror shape: %v", err)
	}
	return f, New(db, cfg)
}

// codeBlockText reads the one text child of the doc's first codeBlock, for
// asserting a verbatim display.
func codeBlockText(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var doc struct {
		Content []struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal display %s: %v", raw, err)
	}
	if len(doc.Content) != 1 || doc.Content[0].Type != "codeBlock" {
		t.Fatalf("display must be one codeBlock, got %s", raw)
	}
	if len(doc.Content[0].Content) != 1 {
		t.Fatalf("codeBlock must hold one text child, got %s", raw)
	}
	return doc.Content[0].Content[0].Text
}

// The reader's half: detail shows the wiki body as one codeBlock with the
// characters unchanged, and description_md — what the editor opens — is the
// string itself, so a save sends back exactly what was read.
func TestWikiServerDetailCarriesVerbatim(t *testing.T) {
	_, h := wikiWorkspace(t)
	rec := send(t, h, http.MethodGet, apiBase+"NMB-1/detail/", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("detail = %d: %s", rec.Code, rec.Body.String())
	}
	var d struct {
		DescriptionMD  json.RawMessage `json:"description_md"`
		DescriptionADF json.RawMessage `json:"description_adf"`
		Comments       []struct {
			Body    string          `json:"body"`
			RawBody json.RawMessage `json:"raw_body"`
		} `json:"comments"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	var md string
	if err := json.Unmarshal(d.DescriptionMD, &md); err != nil {
		t.Fatalf("description_md is not a string: %s", d.DescriptionMD)
	}
	if md != serverWikiBody {
		t.Fatalf("description_md = %q, want the wiki body verbatim", md)
	}
	if got := codeBlockText(t, d.DescriptionADF); got != serverWikiBody {
		t.Fatalf("description_adf codeBlock = %q, want %q", got, serverWikiBody)
	}
	if len(d.Comments) != 1 {
		t.Fatalf("comments = %d, want 1", len(d.Comments))
	}
	if d.Comments[0].Body != serverWikiComment {
		t.Fatalf("comment body = %q, want %q", d.Comments[0].Body, serverWikiComment)
	}
	if got := codeBlockText(t, d.Comments[0].RawBody); got != serverWikiComment {
		t.Fatalf("comment raw_body codeBlock = %q, want %q", got, serverWikiComment)
	}
}

// The writer's half: a description save and a comment send the typed
// characters as a JSON string — the field the markdown dialects fill with an
// ADF document — and neither the placeholder gate nor an origin read runs
// first (the fake would have recorded the read).
func TestWikiServerWritesSendVerbatimString(t *testing.T) {
	f, h := wikiWorkspace(t)
	b, _ := json.Marshal(map[string]any{"description": serverWikiBody})
	rec := send(t, h, http.MethodPut, apiBase+"NMB-1/description/", string(b))
	if rec.Code != http.StatusOK {
		t.Fatalf("description PUT = %d: %s", rec.Code, rec.Body.String())
	}
	// The handler trims the text it accepts (same trim every markdown
	// description save takes) — the carry is verbatim, not byte-untouched.
	wantDesc, _ := json.Marshal(map[string]any{"fields": map[string]any{"description": strings.TrimSpace(serverWikiBody)}})
	if got := string(f.bodies["PUT /issue/NMB-1"]); got != string(wantDesc) {
		t.Fatalf("description wire = %s, want %s", got, wantDesc)
	}
	for _, c := range f.calls {
		// No origin description read before the send: the verbatim carry has
		// no preserved nodes to put back and no format loss to weigh.
		if c == "GET /issue/NMB-1" {
			t.Fatalf("wiki description write read the origin issue first: %v", f.calls)
		}
	}

	cb, _ := json.Marshal(map[string]any{"text": serverWikiComment})
	rec = send(t, h, http.MethodPost, apiBase+"NMB-1/comment/", string(cb))
	if rec.Code != http.StatusOK {
		t.Fatalf("comment POST = %d: %s", rec.Code, rec.Body.String())
	}
	wantComment, _ := json.Marshal(map[string]any{"body": serverWikiComment})
	if got := string(f.bodies["POST /issue/NMB-1/comment"]); got != string(wantComment) {
		t.Fatalf("comment wire = %s, want %s", got, wantComment)
	}
}

// Preview previews a wiki draft the way its save stores it: one codeBlock,
// never a markdown parse — "h2." stays literal characters, "*" stays a bullet
// character, "{code}" stays a brace token.
func TestWikiServerPreviewIsCodeBlock(t *testing.T) {
	_, h := wikiWorkspace(t)
	b, _ := json.Marshal(map[string]any{"text": serverWikiBody})
	rec := send(t, h, http.MethodPost, apiBase+"preview/", string(b))
	if rec.Code != http.StatusOK {
		t.Fatalf("preview = %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		ADF json.RawMessage `json:"adf"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if got := codeBlockText(t, out.ADF); got != serverWikiBody {
		t.Fatalf("preview codeBlock = %q, want the draft verbatim", got)
	}
	if strings.Contains(string(out.ADF), `"em"`) {
		t.Fatalf("preview parsed wiki markup as markdown: %s", out.ADF)
	}
}
