package views

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/store"
)

// openViews opens a throwaway store — never the real ~/.gadak — with the
// given synced Jira filter and saved view already resolvable.
func openViews(t *testing.T, src []store.SourceQuery, saved []store.SavedView) *store.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "gadak.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if len(src) > 0 {
		// source_queries rows have a FK into sources (same shape as the CLI
		// tests' mirror() helper): upsert the owning source first.
		if err := db.UpsertSource(context.Background(), store.Source{ID: "jira", Kind: "jira", BaseURL: "https://nimbus.example.com"}); err != nil {
			t.Fatalf("source: %v", err)
		}
		if err := db.ReplaceSourceQueries(context.Background(), "jira", src); err != nil {
			t.Fatalf("source queries: %v", err)
		}
	}
	for _, s := range saved {
		if err := db.PutSavedView(context.Background(), s); err != nil {
			t.Fatalf("saved view %s: %v", s.ID, err)
		}
	}
	return db
}

func alphaSavedView() store.SavedView {
	return store.SavedView{
		ID:   "cli-alpha-queue",
		Name: "Alpha Queue",
		Config: json.RawMessage(`{"filters":{"jira_project":["NMA"]},"display":{},` +
			`"jql":"project = NMA","applied":["project"],"unsupported":["watchers > 1"]}`),
	}
}

// Exact wins over substring, and the id suffix (after the last ":") counts as
// an exact key — that suffix rule is why a Jira filter is reachable by its
// external id alone.
func TestFindViewExact(t *testing.T) {
	db := openViews(t,
		[]store.SourceQuery{{ID: "jira:10008", SourceID: "jira", ExternalID: "10008", Name: "gadak-test: NMA in progress",
			QueryText: "project = NMA", Config: json.RawMessage(`{"filters":{"jira_project":["NMA"]},"display":{}}`)}},
		[]store.SavedView{alphaSavedView()},
	)
	for _, name := range []string{"alpha queue", "  alpha queue  ", "cli-alpha-queue", "Alpha Queue"} {
		v, err := FindView(db, name)
		if err != nil {
			t.Fatalf("FindView(%q): %v", name, err)
		}
		if v.ID != "cli-alpha-queue" {
			t.Fatalf("FindView(%q) = %s, want cli-alpha-queue", name, v.ID)
		}
	}
	// Suffix of the synced filter's id, not a substring accident.
	v, err := FindView(db, "10008")
	if err != nil {
		t.Fatalf("id suffix: %v", err)
	}
	if v.ID != "jira:10008" {
		t.Fatalf("id suffix hit = %s, want jira:10008", v.ID)
	}
	// One substring hit resolves — prefixes stay usable.
	if _, err := FindView(db, "alpha"); err != nil {
		t.Fatalf("substring single hit: %v", err)
	}
}

func TestFindViewAmbiguous(t *testing.T) {
	db := openViews(t, nil, []store.SavedView{
		alphaSavedView(),
		{ID: "cli-beta-queue", Name: "Beta Queue", Config: json.RawMessage(`{"filters":{"jira_project":["NMB"]},"display":{}}`)},
	})
	_, err := FindView(db, "queue")
	if err == nil {
		t.Fatal("two substring hits must not resolve")
	}
	msg := err.Error()
	for _, want := range []string{`"queue" matches 2 views`, "Alpha Queue", "Beta Queue"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("ambiguous message %q missing %q", msg, want)
		}
	}
}

// The 0-hit message is the unified pair this package owns (GDK-612): an empty
// workspace explains itself; a non-empty one lists what is available. The CLI
// copy's "run `gadak views`" hint is the message that died in unification —
// both surfaces now draw these two, and only these.
func TestFindViewNone(t *testing.T) {
	empty := openViews(t, nil, nil)
	_, err := FindView(empty, "zzz")
	if err == nil || !strings.Contains(err.Error(),
		`no view matching "zzz" — no saved views or synced Jira filters in this workspace`) {
		t.Fatalf("empty-workspace message = %v", err)
	}

	db := openViews(t, nil, []store.SavedView{alphaSavedView()})
	_, err = FindView(db, "zzz")
	if err == nil || !strings.Contains(err.Error(), `no view matching "zzz" — available: Alpha Queue`) {
		t.Fatalf("available-list message = %v", err)
	}
}

// A saved view carries the applied/unsupported clauses its config recorded —
// the MCP copy used to drop them, so gadak_show could not report why a saved
// view applies less than its JQL.
func TestLoadViewsSavedViewFields(t *testing.T) {
	db := openViews(t, nil, []store.SavedView{alphaSavedView()})
	list, err := LoadViews(db)
	if err != nil {
		t.Fatalf("LoadViews: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("views = %d, want 1", len(list))
	}
	v := list[0]
	if v.Kind != "saved" || v.JQL != "project = NMA" ||
		len(v.Applied) != 1 || v.Applied[0] != "project" ||
		len(v.Unsupported) != 1 || v.Unsupported[0] != "watchers > 1" {
		t.Fatalf("saved view fields = %+v", v)
	}
	if !strings.Contains(v.Hash, "pj=NMA") {
		t.Fatalf("hash %q missing pj=NMA", v.Hash)
	}
}

func TestHashFromConfig(t *testing.T) {
	h := HashFromConfig(json.RawMessage(`{"filters":{"jira_project":["NMA"]},"display":{}}`))
	if !strings.Contains(h, "pj=NMA") {
		t.Fatalf("hash %q missing pj=NMA", h)
	}
	if got := HashFromConfig(nil); got != "" {
		t.Fatalf("empty config hash = %q, want \"\"", got)
	}
	if got := HashFromConfig(json.RawMessage(`{"filters":`)); got != "" {
		t.Fatalf("broken config hash = %q, want \"\"", got)
	}
	// Strict parse: metadata that does not unmarshal means no hash, not a
	// hash that silently ignores the broken half (the two pre-GDK-612 copies
	// disagreed here; this pins the stricter CLI behavior).
	if got := HashFromConfig(json.RawMessage(`{"filters":{"jira_project":["NMA"]},"display":{},"applied":"oops"}`)); got != "" {
		t.Fatalf("config with malformed applied must yield \"\", got %q", got)
	}
}
