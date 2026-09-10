package reflink

import (
	"testing"

	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/store"
)

func TestParse(t *testing.T) {
	cases := []struct {
		in       string
		ws, key  string
		expectOK bool
	}{
		{"gadak://work/NMA-9", "work", "NMA-9", true},
		{"gadak://work/", "", "", false},
		{"gadak:///NMA-9", "", "", false},
		{"https://example.com/x", "", "", false},
		{"", "", "", false},
	}
	for _, c := range cases {
		ws, key, ok := Parse(c.in)
		if ok != c.expectOK || ws != c.ws || key != c.key {
			t.Errorf("Parse(%q) = %q,%q,%v want %q,%q,%v", c.in, ws, key, ok, c.ws, c.key, c.expectOK)
		}
	}
}

// TestComposeParseRoundTrip: what Compose writes, Parse reads back — the
// two halves of the grammar must not drift apart.
func TestComposeParseRoundTrip(t *testing.T) {
	url := Compose("team", "NMA-9")
	if url != "gadak://team/NMA-9" {
		t.Fatalf("Compose = %q", url)
	}
	ws, key, ok := Parse(url)
	if !ok || ws != "team" || key != "NMA-9" {
		t.Fatalf("Parse(Compose(...)) = %q,%q,%v", ws, key, ok)
	}
}

// TestStoreLink is the six-field mapping as a contract: the CLI
// write-through and the sync rewrite both build mirror rows through this
// one function, so a dropped field fails here instead of emptying a column
// on one surface only.
func TestStoreLink(t *testing.T) {
	got := StoreLink(jira.RemoteLink{
		ID: "60001", GlobalID: "gadak://team/NMA-9", Relationship: "blocked by",
		URL: "gadak://team/NMA-9", Title: "NMA-9 — the team's issue", Summary: "the team's issue",
	})
	want := store.RemoteLink{
		ID: "60001", GlobalID: "gadak://team/NMA-9", Relationship: "blocked by",
		URL: "gadak://team/NMA-9", Title: "NMA-9 — the team's issue", Summary: "the team's issue",
	}
	if got != want {
		t.Fatalf("StoreLink round trip:\n got %+v\nwant %+v", got, want)
	}
}
