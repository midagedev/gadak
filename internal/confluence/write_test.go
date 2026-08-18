package confluence

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
)

func TestCreatePageRefetchesFromReadPath(t *testing.T) {
	var posts, gets int
	var posted map[string]any
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/wiki/rest/api/content":
			posts++
			body, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(body, &posted); err != nil {
				t.Errorf("create body: %v", err)
			}
			_ = json.NewEncoder(w).Encode(fullPage("77", "LOC", "from-write-response", `{"type":"doc","version":1,"content":[]}`, 1, "2026-08-18T00:00:00.000Z"))
		case r.Method == http.MethodGet && r.URL.Path == "/wiki/rest/api/content/77":
			gets++
			_ = json.NewEncoder(w).Encode(fullPage("77", "LOC", "from-origin", `{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"stored"}]}]}`, 1, "2026-08-18T00:00:00.000Z"))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))

	got, err := c.CreatePage(context.Background(), "LOC", "sent-title", `{"type":"doc","version":1,"content":[]}`, "12")
	if err != nil {
		t.Fatal(err)
	}
	if posts != 1 || gets != 1 {
		t.Fatalf("posts=%d gets=%d, want 1 write then 1 read", posts, gets)
	}
	if got.Title != "from-origin" {
		t.Fatalf("title = %q, want from-origin (must not keep the write response)", got.Title)
	}
	if posted["type"] != "page" {
		t.Errorf("type = %v, want page", posted["type"])
	}
	if posted["title"] != "sent-title" {
		t.Errorf("posted title = %v", posted["title"])
	}
	space, _ := posted["space"].(map[string]any)
	if space["key"] != "LOC" {
		t.Errorf("space.key = %v", posted["space"])
	}
	anc, _ := posted["ancestors"].([]any)
	if len(anc) != 1 {
		t.Fatalf("ancestors = %v, want [{id:12}]", posted["ancestors"])
	}
	first, _ := anc[0].(map[string]any)
	if first["id"] != "12" {
		t.Errorf("ancestors[0].id = %v", first["id"])
	}
}

func TestUpdatePageSendsNextVersionAndRefetches(t *testing.T) {
	var put map[string]any
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/wiki/rest/api/content/77":
			body, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(body, &put); err != nil {
				t.Errorf("update body: %v", err)
			}
			_ = json.NewEncoder(w).Encode(fullPage("77", "LOC", "write-resp", `{"type":"doc","version":1,"content":[]}`, 2, "2026-08-18T00:01:00.000Z"))
		case r.Method == http.MethodGet && r.URL.Path == "/wiki/rest/api/content/77":
			_ = json.NewEncoder(w).Encode(fullPage("77", "LOC", "from-origin-v2", `{"type":"doc","version":1,"content":[]}`, 2, "2026-08-18T00:01:00.000Z"))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))

	got, err := c.UpdatePage(context.Background(), "77", "next title", `{"type":"doc","version":1,"content":[]}`, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "from-origin-v2" || got.Version.Number != 2 {
		t.Fatalf("got title=%q version=%d", got.Title, got.Version.Number)
	}
	ver, _ := put["version"].(map[string]any)
	if ver["number"] != float64(2) {
		t.Errorf("version.number = %v, want 2", put["version"])
	}
}

func TestUpdatePageConflictIsDistinguishable(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"statusCode": 409,
			"message":    "Version must be incremented on update. Current version is: 1",
		})
	}))
	_, err := c.UpdatePage(context.Background(), "77", "x", `{"type":"doc","version":1,"content":[]}`, 1)
	if err == nil {
		t.Fatal("want conflict")
	}
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("errors.Is(err, ErrConflict) = false; %v", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("errors.As(APIError) = false; %v", err)
	}
	if apiErr.Status != http.StatusConflict {
		t.Fatalf("Status = %d, want 409", apiErr.Status)
	}
}

func TestAPIError400IsNotConflict(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"statusCode": 400, "message": "title is required"})
	}))
	_, err := c.CreatePage(context.Background(), "LOC", "", `{"type":"doc","version":1,"content":[]}`, "")
	if err == nil {
		t.Fatal("want error")
	}
	if errors.Is(err, ErrConflict) {
		t.Fatalf("400 must not be ErrConflict: %v", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusBadRequest {
		t.Fatalf("want APIError status 400, got %v / %v", apiErr, err)
	}
}
