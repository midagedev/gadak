package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
)

// GDK-1973: the self-declared name. On a gadak origin, a request with no
// attested viewer may carry X-Gadak-Actor-Name — the person typed their
// name — and that name becomes this request's actor: slug derived from it,
// kind person. Precedence is withViewerActor's, written once there: a
// verified Tailscale viewer > the declared name > the serve process's
// actor > the origin's default user. The declaration buys attribution
// only — no gate reads it (terminal_test.go pins the terminal one).

// (a) A declared name on a gadak origin rides the request as a person. The
// client percent-encodes the value; the server decodes it. Any peer may
// declare — that is the documented --allow-remote sharp edge, not a bug.
func TestWithViewerActorHonoursDeclaredName(t *testing.T) {
	db, cfg := fixture(t)
	cfg.Kind = config.OriginGadak
	h := New(db, cfg)

	req := httptest.NewRequest(http.MethodPost, apiBase+"NMB-1/comment/", nil)
	req.RemoteAddr = "192.0.2.9:52100"
	req.Header.Set("X-Gadak-Actor-Name", "Kim%20Cheolsu")
	ctxReq := h.s.withViewerActor(req)
	slug, name, kind, ok := origin.ViewerActorFrom(ctxReq.Context())
	if !ok || slug != "person:kim-cheolsu" || name != "Kim Cheolsu" || kind != config.ActorKindPerson {
		t.Fatalf("declared actor = %q/%q/%q ok=%v, want person:kim-cheolsu / Kim Cheolsu / person", slug, name, kind, ok)
	}

	// A plain (unencoded) value is accepted too — encodeURIComponent leaves
	// these bytes alone.
	req = httptest.NewRequest(http.MethodPost, apiBase+"NMB-1/comment/", nil)
	req.Header.Set("X-Gadak-Actor-Name", "Dana")
	ctxReq = h.s.withViewerActor(req)
	if slug, _, _, ok := origin.ViewerActorFrom(ctxReq.Context()); !ok || slug != "person:dana" {
		t.Fatalf("unencoded name: slug %q ok=%v, want person:dana", slug, ok)
	}
}

// (b) An attested viewer outranks the declaration, even one that flatters.
func TestWithViewerActorVerifiedViewerWins(t *testing.T) {
	db, cfg := fixture(t)
	cfg.Kind = config.OriginGadak
	h := New(db, cfg)

	req := httptest.NewRequest(http.MethodPost, apiBase+"NMB-1/comment/", nil)
	req.RemoteAddr = "127.0.0.1:52100"
	req.Header.Set("Tailscale-User-Login", "kim@example.com")
	req.Header.Set("Tailscale-User-Name", "Kim")
	req.Header.Set("X-Gadak-Actor-Name", "Someone Else")
	ctxReq := h.s.withViewerActor(req)
	slug, name, kind, ok := origin.ViewerActorFrom(ctxReq.Context())
	if !ok || slug != "kim" || name != "Kim" || kind != config.ActorKindPerson {
		t.Fatalf("attested viewer = %q/%q/%q ok=%v, want kim / Kim / person (the verified login, not the declared name)", slug, name, kind, ok)
	}
}

// (c) A non-gadak origin ignores the header: X-Issuetap-* is an issuetap
// extension and would be noise on any other origin.
func TestWithViewerActorIgnoresDeclaredNameOnNonGadak(t *testing.T) {
	db, cfg := fixture(t)
	jiraCfg := *cfg
	jiraCfg.Kind = ""
	h := New(db, &jiraCfg)

	req := httptest.NewRequest(http.MethodPost, apiBase+"NMB-1/comment/", nil)
	req.RemoteAddr = "192.0.2.9:52100"
	req.Header.Set("X-Gadak-Actor-Name", "Kim%20Cheolsu")
	if got := h.s.withViewerActor(req); got != req {
		t.Fatal("withViewerActor attached an actor from a declared name on a non-gadak origin")
	}
}

// (d) A declaration that is absent, blank, over the rune cap, or not
// decodable is no declaration at all — the request stays untouched.
func TestWithViewerActorIgnoresUnusableDeclaredName(t *testing.T) {
	db, cfg := fixture(t)
	cfg.Kind = config.OriginGadak
	h := New(db, cfg)

	for _, v := range []string{"", "   ", "%zz", strings.Repeat("a", 65)} {
		req := httptest.NewRequest(http.MethodPost, apiBase+"NMB-1/comment/", nil)
		req.RemoteAddr = "192.0.2.9:52100"
		req.Header.Set("X-Gadak-Actor-Name", v)
		if got := h.s.withViewerActor(req); got != req {
			t.Fatalf("withViewerActor attached an actor for declared name %q", v)
		}
	}
}

// (e) GET viewer/ reports the declared name when this request's declaration
// was honoured, and the empty string otherwise — present, never omitted.
func TestViewerDocumentCarriesDeclaredName(t *testing.T) {
	db, cfg := fixture(t)
	cfg.Kind = config.OriginGadak
	h := New(db, cfg)

	req := httptest.NewRequest(http.MethodGet, viewerBase, nil)
	req.Host = "192.0.2.9:7777"
	req.RemoteAddr = "192.0.2.9:52100"
	req.Header.Set("X-Gadak-Actor-Name", "Kim%20Cheolsu")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if got := decode[Viewer](t, rec); got.Declared != "Kim Cheolsu" {
		t.Fatalf("viewer document declared = %q, want Kim Cheolsu", got.Declared)
	}

	// Without the header the field is the empty string, and an attested
	// viewer's document does not carry the declaration either.
	for _, tc := range []struct {
		name, declared, tailscale string
		host, remote              string
	}{
		{"no header", "", "", "192.0.2.9:7777", "192.0.2.9:52100"},
		// The declaration exists but is outranked by the attestation, so the
		// document reports none of it.
		{"attested viewer", "Someone%20Else", "kim@example.com", "127.0.0.1:7777", "127.0.0.1:52100"},
	} {
		req := httptest.NewRequest(http.MethodGet, viewerBase, nil)
		req.Host = tc.host
		req.RemoteAddr = tc.remote
		if tc.declared != "" {
			req.Header.Set("X-Gadak-Actor-Name", tc.declared)
		}
		if tc.tailscale != "" {
			req.Header.Set("Tailscale-User-Login", tc.tailscale)
			req.Header.Set("Tailscale-User-Name", "Kim")
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d: %s", tc.name, rec.Code, rec.Body.String())
		}
		if got := decode[Viewer](t, rec); got.Declared != "" {
			t.Fatalf("%s: viewer document declared = %q, want empty", tc.name, got.Declared)
		}
	}
}
