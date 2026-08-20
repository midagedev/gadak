package store

import (
	"context"
	"encoding/json"
	"testing"
)

// AbsorbViews (GDK-437): the localStorage→server one-shot. Three rules under
// test — an empty server takes everything, a name the server already owns is
// not overwritten, and re-absorbing the same browser rows changes nothing.

func absorbableView(id, name, cfg string) SavedView {
	return SavedView{ID: id, Name: name, Config: json.RawMessage(cfg), CreatedAt: "2026-08-20T00:00:00.000Z"}
}

func TestAbsorbViewsFillsEmptyServer(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	in := []SavedView{
		absorbableView("p-beta", "Beta", `{"filters":{"labels":["api"]}}`),
		absorbableView("p-alpha", "Alpha", `{"filters":{"stale":true}}`),
	}
	if err := db.AbsorbViews(ctx, in); err != nil {
		t.Fatal(err)
	}
	got, err := db.SavedViews(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 absorbed views, got %+v", got)
	}
	// Reads order by name; ids stay stable so the client can hide its
	// absorbed localStorage rows by id.
	if got[0].Name != "Alpha" || got[0].ID != "p-alpha" {
		t.Fatalf("Alpha first with its own id, got %+v", got[0])
	}
	if got[1].Name != "Beta" || got[1].ID != "p-beta" {
		t.Fatalf("Beta second with its own id, got %+v", got[1])
	}
	if got[0].CreatedAt != "2026-08-20T00:00:00.000Z" {
		t.Fatalf("created_at must survive the move, got %+v", got[0])
	}
}

func TestAbsorbViewsServerRowWinsOnName(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	if err := db.PutSavedView(ctx, SavedView{
		ID: "server-1", Name: "Alpha", Config: json.RawMessage(`{"filters":{"stale":true}}`),
	}); err != nil {
		t.Fatal(err)
	}
	// Same name, different id and config: the server row must survive as-is.
	if err := db.AbsorbViews(ctx, []SavedView{
		absorbableView("p-alpha", "Alpha", `{"filters":{"labels":["api"]}}`),
		absorbableView("p-beta", "Beta", `{"filters":{}}`),
	}); err != nil {
		t.Fatal(err)
	}
	got, err := db.SavedViews(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("name conflict drops the incoming row, got %+v", got)
	}
	if got[0].ID != "server-1" || string(got[0].Config) != `{"filters":{"stale":true}}` {
		t.Fatalf("server row must win unchanged, got %+v", got[0])
	}
	if got[1].ID != "p-beta" {
		t.Fatalf("non-conflicting row fills in, got %+v", got[1])
	}
}

func TestAbsorbViewsTwiceKeepsNoDuplicates(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	in := []SavedView{
		absorbableView("p-beta", "Beta", `{"filters":{}}`),
		absorbableView("p-alpha", "Alpha", `{"filters":{}}`),
	}
	for i := 1; i <= 2; i++ {
		if err := db.AbsorbViews(ctx, in); err != nil {
			t.Fatalf("pass %d: %v", i, err)
		}
	}
	got, err := db.SavedViews(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("re-absorb must be a no-op, got %+v", got)
	}
}

// The id-collision guard: an incoming row whose id is taken (but whose name is
// new) must be inserted under a fresh id, never upserted over the server row.
func TestAbsorbViewsIdCollisionGetsFreshID(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	if err := db.PutSavedView(ctx, SavedView{
		ID: "dup", Name: "One", Config: json.RawMessage(`{"filters":{"stale":true}}`),
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.AbsorbViews(ctx, []SavedView{
		absorbableView("dup", "Two", `{"filters":{"labels":["api"]}}`),
	}); err != nil {
		t.Fatal(err)
	}
	got, err := db.SavedViews(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want the server row plus a fresh-id insert, got %+v", got)
	}
	if got[0].ID != "dup" || string(got[0].Config) != `{"filters":{"stale":true}}` {
		t.Fatalf("server row 'One' must survive the id collision, got %+v", got[0])
	}
	if got[1].ID == "dup" || got[1].Name != "Two" {
		t.Fatalf("colliding row needs a fresh id, got %+v", got[1])
	}
}
