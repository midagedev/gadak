package jira

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Server's board/{id}/sprint answers no `total` — only isLast — so the pager
// must not depend on it (GDK-1654). Measured on Jira Software 11.3.11.
func TestSprintsPagesOnIsLastAlone(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Get("startAt") == "0" {
			_, _ = w.Write([]byte(`{"maxResults":1,"startAt":0,"isLast":false,"values":[{"id":1,"name":"one","state":"active"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"maxResults":1,"startAt":1,"isLast":true,"values":[{"id":2,"name":"two","state":"future"}]}`))
	}))
	defer srv.Close()

	got, err := NewServer(srv.URL, "tok").Sprints(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != 1 || got[1].State != "future" {
		t.Fatalf("got %+v after %d calls", got, calls)
	}
}

// A site with no Jira Software has no Agile API. That is not a sync failure.
func TestBoardsOnASiteWithoutSoftware(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"errorMessages":["no"]}`, http.StatusNotFound)
	}))
	defer srv.Close()
	if _, err := NewServer(srv.URL, "tok").Boards(context.Background()); !errors.Is(err, ErrNoAgile) {
		t.Fatalf("want ErrNoAgile, got %v", err)
	}
}

// A 501 is a different fact from a missing Agile API (GDK-1691): a gadak
// origin built before its sprints (GDK-1666) answers 501 on the route. The
// caller has to be able to tell the two apart — one skips quietly, the
// other owes the user the upgrade sentence — so the status is its own
// sentinel, not a status line to grep.
func TestBoardsOnAnOriginThatPredatesTheAgileAPI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not implemented", http.StatusNotImplemented)
	}))
	defer srv.Close()
	if _, err := NewServer(srv.URL, "tok").Boards(context.Background()); !errors.Is(err, ErrAgileUnimplemented) {
		t.Fatalf("want ErrAgileUnimplemented, got %v", err)
	}
}

// The move body is a value, not pre-marshalled bytes: handing write() bytes
// sends them base64-encoded and Jira 400s (GDK-1655, measured).
func TestMoveToSprintSendsAJSONObject(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	if err := NewServer(srv.URL, "tok").MoveToSprint(context.Background(), 7, []string{"A-1"}); err != nil {
		t.Fatal(err)
	}
	issues, ok := body["issues"].([]any)
	if !ok || len(issues) != 1 || issues[0] != "A-1" {
		t.Fatalf("body was %#v", body)
	}
}
