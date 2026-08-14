package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/store"
)

func TestHistoryVisitAppendAndList(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)

	for i := 0; i < 2; i++ {
		rec := postJSON(t, h, apiBase+"history/visits/", `{"kind":"issue","key":"NMB-1"}`)
		if rec.Code != http.StatusCreated {
			t.Fatalf("visit %d: %d %s", i, rec.Code, rec.Body.String())
		}
	}
	var created store.Visit
	if err := json.Unmarshal(postJSON(t, h, apiBase+"history/visits/", `{"kind":"page","key":"622723"}`).Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Kind != "page" || created.Key != "622723" || created.ID == 0 || created.ViewedAt == "" {
		t.Fatalf("created = %+v", created)
	}

	body := decode[store.HistoryPage](t, get(t, h, apiBase+"history/", nil))
	if len(body.Items) != 3 {
		t.Fatalf("items = %d, want 3", len(body.Items))
	}
	var issueN int
	for _, it := range body.Items {
		if it.Type == "visit" && it.Kind == "issue" && it.Key == "NMB-1" {
			issueN++
		}
	}
	if issueN != 2 {
		t.Fatalf("issue visits = %d, want 2 (append-only)", issueN)
	}

	issues := decode[store.HistoryPage](t, get(t, h, apiBase+"history/?kind=issue", nil))
	if len(issues.Items) != 2 {
		t.Fatalf("kind=issue items = %d, want 2", len(issues.Items))
	}
}

func TestHistorySearchAndOpened(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)

	rec := postJSON(t, h, apiBase+"history/searches/", `{"query":"flaky upload","result_count":5}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("search: %d %s", rec.Code, rec.Body.String())
	}
	var created store.Search
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == 0 || created.Query != "flaky upload" || created.ResultCount != 5 || created.OpenedKey != nil {
		t.Fatalf("created = %+v", created)
	}

	patch := patchJSON(t, h, apiBase+"history/searches/"+strconv.FormatInt(created.ID, 10)+"/", `{"opened_kind":"issue","opened_key":"NMB-1"}`)
	if patch.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", patch.Code, patch.Body.String())
	}
	var updated store.Search
	if err := json.Unmarshal(patch.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.OpenedKind == nil || *updated.OpenedKind != "issue" || updated.OpenedKey == nil || *updated.OpenedKey != "NMB-1" {
		t.Fatalf("updated = %+v", updated)
	}

	page := decode[store.HistoryPage](t, get(t, h, apiBase+"history/?kind=search", nil))
	if len(page.Items) != 1 || page.Items[0].Query != "flaky upload" {
		t.Fatalf("search list = %+v", page.Items)
	}
	if page.Items[0].OpenedKey == nil || *page.Items[0].OpenedKey != "NMB-1" {
		t.Fatalf("opened_key missing: %+v", page.Items[0])
	}
}

func TestHistoryCursorLimit(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)
	postJSON(t, h, apiBase+"history/visits/", `{"kind":"issue","key":"NMB-1"}`)
	postJSON(t, h, apiBase+"history/visits/", `{"kind":"issue","key":"NMB-2"}`)
	postJSON(t, h, apiBase+"history/visits/", `{"kind":"issue","key":"NMA-9"}`)

	page1 := decode[store.HistoryPage](t, get(t, h, apiBase+"history/?limit=2", nil))
	if len(page1.Items) != 2 || page1.NextCursor == "" {
		t.Fatalf("page1 = %+v", page1)
	}
	page2 := decode[store.HistoryPage](t, get(t, h, apiBase+"history/?limit=2&cursor="+page1.NextCursor, nil))
	if len(page2.Items) != 1 {
		t.Fatalf("page2 items = %d, want 1 (%+v)", len(page2.Items), page2.Items)
	}
	seen := map[string]bool{}
	for _, it := range append(page1.Items, page2.Items...) {
		seen[it.Key] = true
	}
	if !seen["NMB-1"] || !seen["NMB-2"] || !seen["NMA-9"] {
		t.Fatalf("keys across pages = %v", seen)
	}
}

func TestHistoryRejectsBadKind(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)
	rec := postJSON(t, h, apiBase+"history/visits/", `{"kind":"ticket","key":"NMB-1"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "invalid_kind") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestHistoryPatchMissingSearch(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)
	rec := patchJSON(t, h, apiBase+"history/searches/99/", `{"opened_kind":"issue","opened_key":"NMB-1"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d %s", rec.Code, rec.Body.String())
	}
}

func postJSON(t *testing.T, h http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := testRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func patchJSON(t *testing.T, h http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := testRequest(http.MethodPatch, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}
