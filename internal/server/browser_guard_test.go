package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/midagedev/scry/internal/config"
)

// browserGuard tests cover Host (DNS rebinding) and Origin (CSRF) checks
// wired into Handler.ServeHTTP before the mux. See browser_guard.go.

func TestBrowserGuardForbiddenOrigin(t *testing.T) {
	db, _ := fixture(t)
	h := New(db, &config.Config{}) // no credential → past the guard would be 409

	req := httptest.NewRequest(http.MethodPost, apiBase+"NMB-1/comment/", nil)
	req.Host = "127.0.0.1:7777"
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d, want 403; body %s", rec.Code, rec.Body.String())
	}
	if got := decode[map[string]string](t, rec)["error"]; got != "forbidden_origin" {
		t.Fatalf("error %q, want forbidden_origin", got)
	}
}

func TestBrowserGuardMatchingOriginAllowed(t *testing.T) {
	db, _ := fixture(t)
	h := New(db, &config.Config{})

	req := httptest.NewRequest(http.MethodPost, apiBase+"NMB-1/comment/", nil)
	req.Host = "127.0.0.1:7777"
	req.Header.Set("Origin", "http://127.0.0.1:7777")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code == http.StatusForbidden {
		t.Fatalf("matching Origin got 403: %s", rec.Body.String())
	}
	// No credential → existing write path answers 409, not a guard rejection.
	if rec.Code == http.StatusOK {
		t.Fatalf("unexpected 200 without credential: %s", rec.Body.String())
	}
}

func TestBrowserGuardNullOriginForbidden(t *testing.T) {
	db, _ := fixture(t)
	h := New(db, &config.Config{})

	req := httptest.NewRequest(http.MethodPost, apiBase+"NMB-1/comment/", nil)
	req.Host = "127.0.0.1:7777"
	req.Header.Set("Origin", "null")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d, want 403; body %s", rec.Code, rec.Body.String())
	}
	if got := decode[map[string]string](t, rec)["error"]; got != "forbidden_origin" {
		t.Fatalf("error %q, want forbidden_origin", got)
	}
}

func TestBrowserGuardForbiddenHost(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)

	req := httptest.NewRequest(http.MethodGet, apiBase+"bootstrap/", nil)
	req.Host = "attacker.example.com"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d, want 403; body %s", rec.Code, rec.Body.String())
	}
	if got := decode[map[string]string](t, rec)["error"]; got != "forbidden_host" {
		t.Fatalf("error %q, want forbidden_host", got)
	}
}

func TestBrowserGuardAllowedHosts(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)

	for _, host := range []string{
		"localhost:7777",
		"127.0.0.1:7777",
		"[::1]:7777",
		"192.168.0.5:7777",
	} {
		t.Run(host, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, apiBase+"bootstrap/", nil)
			req.Host = host
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code == http.StatusForbidden {
				t.Fatalf("Host %q got 403: %s", host, rec.Body.String())
			}
			if rec.Code < 200 || rec.Code >= 300 {
				// bootstrap is 200 on a healthy mirror; accept any 2xx.
				t.Fatalf("Host %q status %d (want 2xx): %s", host, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestBrowserGuardNoOriginAllowed(t *testing.T) {
	db, _ := fixture(t)
	h := New(db, &config.Config{})

	// curl / CLI / TUI: no Origin header.
	req := httptest.NewRequest(http.MethodPost, apiBase+"NMB-1/comment/", nil)
	req.Host = "127.0.0.1:7777"
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code == http.StatusForbidden {
		t.Fatalf("POST without Origin got 403: %s", rec.Body.String())
	}
	// Reach the real handler (credential_required without a token).
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v body %s", err, rec.Body.String())
	}
	if body["error"] == "forbidden_origin" || body["error"] == "forbidden_host" {
		t.Fatalf("guard rejected curl-like POST: %v", body)
	}
}
