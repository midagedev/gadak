package server

// GDK-1215: the mirror stores a link's direction as the wire token
// (`inward`/`outward`), and the detail response used to hand that token to
// the client raw. The client then had to resolve the human sentence itself
// (a catalog fetch that only runs on some workspaces), and a surface that
// skipped the lookup read "Blocks outward NMB-2" — tracker wire vocabulary
// on a human line. The backend owns the phrase now: it renders the side's
// sentence through origin.LinkPhrase from the same mirror catalog
// (link_types, schemaV43) the CLI's human line reads (GDK-1734), so every
// client of the detail document gets the phrase without a second fetch.

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/midagedev/gadak/internal/store"
)

// TestDetailLinkPhraseFromBackend plants the Blocks catalog row the way a
// sync run caches it (Batch.LinkTypes → link_types) and reads one issue's
// detail. Both directions of the same type must answer the type's own
// sentence, never the token; a type the catalog does not know keeps the
// field absent, which is the client's fallback signal, not an error.
func TestDetailLinkPhraseFromBackend(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)

	if _, err := db.UpsertIssues(context.Background(), store.Batch{
		Force: true,
		LinkTypes: []store.LinkType{{
			ID: "10000", Name: "Blocks", Inward: "is blocked by", Outward: "blocks",
		}},
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "jira:1001", SourceID: "jira", ExternalID: "1001", Key: "NMB-1",
				Title: "batch worker drops the last page",
			},
			Issue: store.Issue{
				ProjectKey: "NMB", Status: "진행 중", StatusID: "3", StatusCategory: "inprogress",
			},
			Links: []store.Link{
				{Type: "Blocks", Direction: "outward", TargetKey: "NMB-2"},
				{Type: "Blocks", Direction: "inward", TargetKey: "NMA-9"},
				// A type the catalog does not carry: the phrase field must
				// stay absent, and the row must still be served.
				{Type: "Relates", Direction: "outward", TargetKey: "NMA-9"},
			},
		}},
	}); err != nil {
		t.Fatalf("re-upsert: %v", err)
	}

	rec := get(t, h, apiBase+"NMB-1/detail/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var doc struct {
		LinkedIssues []struct {
			Key       string  `json:"key"`
			Type      string  `json:"type"`
			Direction string  `json:"direction"`
			Phrase    *string `json:"phrase"`
		} `json:"linked_issues"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	if len(doc.LinkedIssues) != 3 {
		t.Fatalf("linked_issues %+v", doc.LinkedIssues)
	}
	for _, l := range doc.LinkedIssues {
		var want string
		switch {
		case l.Type == "Blocks" && l.Direction == "outward":
			want = "blocks"
		case l.Type == "Blocks" && l.Direction == "inward":
			want = "is blocked by"
		case l.Type == "Relates":
			if l.Phrase != nil {
				t.Fatalf("Relates is not in the catalog: phrase must be absent, got %q", *l.Phrase)
			}
			continue
		default:
			t.Fatalf("unexpected linked row %+v", l)
		}
		if l.Phrase == nil || *l.Phrase != want {
			t.Fatalf("%s %s %s: phrase = %v, want %q — the type's own sentence, not the token", l.Type, l.Direction, l.Key, l.Phrase, want)
		}
		if *l.Phrase == l.Direction {
			t.Fatalf("%s %s: the wire token leaked into the human phrase", l.Type, l.Direction)
		}
	}
}
