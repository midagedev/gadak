package store

import (
	"context"
	"errors"
	"testing"
)

// Conversion drops the old origin's mirror, and personal rows keyed into it
// (watches/favorites) go with it — kept, they would silently rebind to a new
// origin's issue that happens to share the key (GDK-344 F12). Rows keyed to
// another source's items survive.
func TestDropSourceMirrorTakesItsWatchesAndFavorites(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	for _, src := range []string{"jira", "other"} {
		if err := db.UpsertSource(ctx, Source{ID: src, Kind: "jira"}); err != nil {
			t.Fatal(err)
		}
	}
	put := func(src, id, key string) {
		if _, err := db.UpsertIssues(ctx, Batch{
			Categories: map[string]string{"1": "new"},
			Records: []IssueRecord{{
				Item: Item{ID: id, SourceID: src, Kind: "issue", ExternalID: id,
					Key: key, Title: key, CreatedAt: ago(1), UpdatedAt: ago(1)},
				Issue: Issue{ProjectKey: "STD", IssueType: "Task", IssueTypeID: "1",
					Status: "To Do", StatusID: "1", StatusCategory: "new"},
			}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	put("jira", "standalone-jira:10001", "STD-1")
	put("other", "other:1", "OTH-1")
	for _, key := range []string{"STD-1", "OTH-1"} {
		if err := db.SetWatch(ctx, key, true); err != nil {
			t.Fatal(err)
		}
		if err := db.SetFavorite(ctx, key, true); err != nil {
			t.Fatal(err)
		}
	}

	if err := db.DropSourceMirror(ctx, "jira"); err != nil {
		t.Fatal(err)
	}

	for name, list := range map[string]func(context.Context) ([]string, error){
		"watches": db.Watches, "favorites": db.Favorites,
	} {
		got, err := list(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0] != "OTH-1" {
			t.Errorf("%s after drop = %v, want [OTH-1]", name, got)
		}
	}
}

// A key can be minted by two sources at once (Jira project ENG and a Linear
// team key ENG both produce ENG-1). Write-through is a Jira surface: the jira
// row must win the lookup, or a legitimate Jira write gets refused on
// whichever row an unordered LIMIT 1 happened to return (GDK-263 review).
func TestKeySourceRefusesCollision(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	for _, src := range []string{"linear", "jira"} { // linear first: order must not matter
		if err := db.UpsertSource(ctx, Source{ID: src, Kind: src}); err != nil {
			t.Fatal(err)
		}
		if _, err := db.UpsertIssues(ctx, Batch{
			Categories: map[string]string{"1": "new"},
			Records: []IssueRecord{{
				Item: Item{ID: src + ":eng1", SourceID: src, Kind: "issue", ExternalID: "eng1",
					Key: "ENG-1", Title: "same key twice", CreatedAt: ago(1), UpdatedAt: ago(1)},
				Issue: Issue{ProjectKey: "ENG", IssueType: "Task", IssueTypeID: "1",
					Status: "To Do", StatusID: "1", StatusCategory: "new"},
			}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	// GDK-400 overturned the prefer-jira contract this test used to pin:
	// preferring one source routed a write to a tracker the screen was not
	// showing. A two-source key is now an explicit refusal.
	if _, err := db.KeySource(ctx, "ENG-1"); !errors.Is(err, ErrKeyAmbiguous) {
		t.Fatalf("KeySource(ENG-1) = %v, want ErrKeyAmbiguous", err)
	}
	if src, err := db.KeySource(ctx, "MISSING-1"); err != nil || src != "" {
		t.Fatalf("missing key = (%q, %v), want empty", src, err)
	}
}

func TestProjectSourceMirrorsKeySource(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	if err := db.UpsertSource(ctx, Source{ID: "linear", Kind: "linear"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertIssues(ctx, Batch{
		Categories: map[string]string{"1": "new"},
		Records: []IssueRecord{{
			Item: Item{ID: "linear:m1", SourceID: "linear", Kind: "issue", ExternalID: "m1",
				Key: "MID-1", Title: "linear row", CreatedAt: ago(1), UpdatedAt: ago(1)},
			Issue: Issue{ProjectKey: "MID", IssueType: "Issue", IssueTypeID: "issue",
				Status: "Todo", StatusID: "1", StatusCategory: "new"},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	src, err := db.ProjectSource(ctx, "MID")
	if err != nil || src != "linear" {
		t.Fatalf("ProjectSource(MID) = (%q, %v), want linear", src, err)
	}
	if src, err := db.ProjectSource(ctx, "NOPE"); err != nil || src != "" {
		t.Fatalf("missing project = (%q, %v), want empty", src, err)
	}
}
