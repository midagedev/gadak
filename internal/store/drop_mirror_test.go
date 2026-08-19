package store

import (
	"context"
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
