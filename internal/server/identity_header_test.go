package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Identity headers identify a local scry process to another scry that finds
// its listen port busy (cmd/scry port fallback). They must appear on every
// response that goes through Handler.ServeHTTP — including 404 and guard
// failures — so a probe to any API path can classify the occupant.
func TestIdentityHeadersOnAPIResponses(t *testing.T) {
	db, cfg := fixture(t)
	h := NewWorkspace(db, cfg, nil, "work")

	cases := []struct {
		name string
		path string
	}{
		{"sync progress", apiBase + "sync/progress/"},
		{"bootstrap", apiBase + "bootstrap/"},
		{"not found", apiBase + "no-such-route/"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := get(t, h, tc.path, nil)
			if got := rec.Header().Get("X-Scry"); got != "1" {
				t.Errorf("X-Scry = %q, want %q", got, "1")
			}
			if got := rec.Header().Get("X-Scry-Profile"); got != "work" {
				t.Errorf("X-Scry-Profile = %q, want %q", got, "work")
			}
		})
	}
}

func TestIdentityHeadersEmptyProfile(t *testing.T) {
	db, cfg := fixture(t)
	h := NewWorkspace(db, cfg, nil, "")
	rec := get(t, h, apiBase+"sync/progress/", nil)
	if got := rec.Header().Get("X-Scry"); got != "1" {
		t.Fatalf("X-Scry = %q, want 1", got)
	}
	if got := rec.Header().Get("X-Scry-Profile"); got != "" {
		t.Fatalf("X-Scry-Profile = %q, want empty", got)
	}
}

func TestIdentityHeadersOnForbiddenHost(t *testing.T) {
	// Headers must be set before browserGuard so a probe still sees them
	// even when Host is rejected (defensive; real probes use 127.0.0.1).
	db, cfg := fixture(t)
	h := NewWorkspace(db, cfg, nil, "demo")
	req := testRequest(http.MethodGet, apiBase+"sync/progress/", nil)
	req.Host = "evil.example"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get("X-Scry"); got != "1" {
		t.Errorf("X-Scry on forbidden host = %q, want 1", got)
	}
	if got := rec.Header().Get("X-Scry-Profile"); got != "demo" {
		t.Errorf("X-Scry-Profile on forbidden host = %q, want demo", got)
	}
}
