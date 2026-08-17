package server

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// TestRecentsRouteExists is FAIL-first for GDK-224: before the route existed
// this GET was 404 {"error":"not_found"} (captured 2026-08-17).
func TestRecentsRouteExists(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)
	rec := get(t, h, apiBase+"recents/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET recents/ = %d %s (want 200 — recency must be readable on the API)", rec.Code, rec.Body.String())
	}
	var body recentsDoc
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Items == nil {
		t.Fatal("items must be [] not null")
	}
}

// TestRecentsAPIParityWithSQL is the GDK-224 surface-parity gate: a recent
// type written through the API must be the same value gadak sql would read.
func TestRecentsAPIParityWithSQL(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)

	created := postJSON(t, h, apiBase+"recents/", `{"kind":"create-type:NMB","value":"10002"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("POST recents/ = %d %s", created.Code, created.Body.String())
	}

	got := get(t, h, apiBase+"recents/?kind=create-type:NMB", nil)
	if got.Code != http.StatusOK {
		t.Fatalf("GET recents/ = %d %s", got.Code, got.Body.String())
	}
	var body recentsDoc
	if err := json.Unmarshal(got.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 1 || body.Items[0].Value != "10002" {
		t.Fatalf("API items = %+v, want value 10002", body.Items)
	}

	viaStore, err := db.Recents(context.Background(), "create-type:NMB")
	if err != nil {
		t.Fatalf("store.Recents after API write: %v", err)
	}
	if len(viaStore) != 1 || viaStore[0].Value != body.Items[0].Value {
		t.Fatalf("API %q != store/SQL %v", body.Items[0].Value, viaStore)
	}
}

func TestAbsorbRecentsDoesNotDuplicate(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)
	postJSON(t, h, apiBase+"recents/", `{"kind":"assignee","value":"acct-1"}`)

	rec := postJSON(t, h, apiBase+"recents/absorb/", `{"kinds":{"assignee":["acct-2","acct-1"]}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("absorb = %d %s", rec.Code, rec.Body.String())
	}
	var body recentsDoc
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 2 {
		t.Fatalf("items = %+v", body.Items)
	}
	if body.Items[0].Value != "acct-1" || body.Items[1].Value != "acct-2" {
		t.Fatalf("order = %+v (server first, then LS fill)", body.Items)
	}
}

func TestPostRecentRejectsEmpty(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)
	rec := postJSON(t, h, apiBase+"recents/", `{"kind":"","value":"x"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty kind = %d", rec.Code)
	}
	rec = postJSON(t, h, apiBase+"recents/", `{"kind":"assignee","value":""}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty value = %d", rec.Code)
	}
}
