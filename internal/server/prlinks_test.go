package server

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/midagedev/gadak/internal/store"
)

// GDK-495: a mirrored URL attachment pointing at a GitHub pull request surfaces
// as linked_prs, so a Linear issue whose PR the tracker already knows shows it
// here too. A plugin enrichment (kind='prs') still wins — it can carry state.
func TestDetailDerivesLinkedPRsFromAttachments(t *testing.T) {
	db, cfg, path := fixtureAt(t)
	h := New(db, cfg)

	if _, err := db.UpsertIssues(context.Background(), store.Batch{
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "jira:9001", SourceID: "jira", ExternalID: "9001", Key: "MID-7",
				Title: "sync drops the cursor",
			},
			Issue: store.Issue{ProjectKey: "MID", Status: "Todo", StatusID: "s1", StatusCategory: "new"},
			Attachments: []store.Attachment{
				{
					ID: "jira:at-1", ExternalID: "at-1", Filename: "midagedev/gadak#50",
					URL: "https://github.com/midagedev/gadak/pull/50",
				},
				{
					// A plain file attachment must not become a PR.
					ID: "jira:at-2", ExternalID: "at-2", Filename: "shot.png",
					MimeType: "image/png", URL: "https://uploads.example.com/shot.png",
				},
				{
					// An issue URL is not a pull request.
					ID: "jira:at-3", ExternalID: "at-3", Filename: "midagedev/gadak#12",
					URL: "https://github.com/midagedev/gadak/issues/12",
				},
			},
		}},
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	rec := get(t, h, apiBase+"MID-7/detail/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var d struct {
		LinkedPRs []struct {
			Number int     `json:"number"`
			Title  string  `json:"title"`
			URL    string  `json:"url"`
			State  string  `json:"state"`
			Repo   *string `json:"repo"`
		} `json:"linked_prs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(d.LinkedPRs) != 1 {
		t.Fatalf("linked_prs = %+v, want exactly the PR attachment", d.LinkedPRs)
	}
	pr := d.LinkedPRs[0]
	if pr.Number != 50 || pr.URL != "https://github.com/midagedev/gadak/pull/50" ||
		pr.Title != "midagedev/gadak#50" || pr.Repo == nil || *pr.Repo != "midagedev/gadak" {
		t.Fatalf("derived PR wrong: %+v", pr)
	}
	if pr.State != "" {
		t.Fatalf("state = %q — a bare URL cannot know the state", pr.State)
	}

	// The plugin enrichment stays the winner over the derived list.
	enrich(t, path, "MID-7", "prs",
		`[{"number":50,"title":"from plugin","url":"https://github.com/midagedev/gadak/pull/50","state":"merged","repo":"midagedev/gadak","author":"alice"}]`)
	var d2 struct {
		LinkedPRs []struct {
			Title string `json:"title"`
			State string `json:"state"`
		} `json:"linked_prs"`
	}
	if err := json.Unmarshal(get(t, h, apiBase+"MID-7/detail/", nil).Body.Bytes(), &d2); err != nil {
		t.Fatalf("decode 2: %v", err)
	}
	if len(d2.LinkedPRs) != 1 || d2.LinkedPRs[0].State != "merged" || d2.LinkedPRs[0].Title != "from plugin" {
		t.Fatalf("enrichment did not win: %+v", d2.LinkedPRs)
	}
}
