package jira

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

// Contract: Claim POSTs /issue/{key}/claim with the two-option body and
// parses the origin's answer; the 409 arrives as an APIError carrying the
// holder's sentence (internal/claim turns that into its refusal).
func TestClaimWire(t *testing.T) {
	var gotPath, gotBody string
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.Method + " " + r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		if r.URL.Path == "/rest/api/3/issue/NMB-1/claim" {
			w.Write([]byte(`{"key":"NMB-1","assignee":{"accountId":"a1","displayName":"Agent A"},
				"status":{"id":"20","name":"In Progress","statusCategory":{"key":"indeterminate"}},
				"claimedAt":"2026-08-22T10:00:00.000Z"}`))
			return
		}
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"errorMessages":["NMB-2 is already claimed by Agent B"],"errors":{}}`))
	}))
	ctx := context.Background()

	res, err := c.Claim(ctx, "NMB-1", "", false)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if gotPath != "POST /rest/api/3/issue/NMB-1/claim" {
		t.Fatalf("path: %s", gotPath)
	}
	if gotBody != `{"transitionId":"","takeOver":false}` {
		t.Fatalf("body: %s", gotBody)
	}
	if res.Key != "NMB-1" || res.Assignee.DisplayName != "Agent A" ||
		res.Status.ID != "20" || res.ClaimedAt == "" {
		t.Fatalf("answer: %+v", res)
	}

	_, err = c.Claim(ctx, "NMB-2", "31", true)
	var api *APIError
	if !errors.As(err, &api) || api.Status != http.StatusConflict {
		t.Fatalf("want 409 APIError, got %v", err)
	}
	if !strings.Contains(api.Message(), "NMB-2 is already claimed by Agent B") {
		t.Fatalf("message: %q", api.Message())
	}
}
