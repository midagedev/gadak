package origin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/jira"
)

// skewServe answers the paired transport's myself path the way a home
// serve does since GDK-1273: X-Gadak-Version on every response. status
// selects between the 200 myself document and a 501 unimplemented-route
// answer (the shape an older serve gives a verb it predates). The
// returned counter is every request the serve saw — the answer-once
// gate below reads it.
func skewServe(t *testing.T, status int, serveVersion string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if serveVersion != "" {
			w.Header().Set("X-Gadak-Version", serveVersion)
		}
		if r.URL.Path != RESTPrefix+"/rest/api/3/myself" {
			http.NotFound(w, r)
			return
		}
		if status != http.StatusOK {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"errorMessages":["unsupported_endpoint"]}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"displayName":"Home User","accountId":"acc-pair"}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// TestPairedVerifyLearnsServeVersion is FAIL-first for GDK-1273's capture
// half: the one round trip every pairing already makes (VerifyPaired, the
// verify-before-save gate) learns the home serve's gadak version from the
// X-Gadak-Version header, and PairedServerVersion hands it to the pair-time
// record. Keyed by endpoint — a second workspace's serve must not answer
// for the first.
func TestPairedVerifyLearnsServeVersion(t *testing.T) {
	srv, _ := skewServe(t, http.StatusOK, "0.19.1")
	if _, err := VerifyPaired(context.Background(), srv.URL, "pair-token"); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got := PairedServerVersion(srv.URL); got != "0.19.1" {
		t.Fatalf("PairedServerVersion(%s) = %q, want 0.19.1", srv.URL, got)
	}
	if got := PairedServerVersion("http://192.0.2.9:7877"); got != "" {
		t.Fatalf("unrelated endpoint must not inherit a version, got %q", got)
	}
}

// TestPaired501FoldsIntoUpgradeHint is FAIL-first for GDK-1273's common
// 501 point: a paired workspace whose home serve is older than this client
// dies on new verbs with a bare "issuetap does not implement …" (measured
// 2026-09-01, GDK-1032 dogfooding) — true, and useless. The fold at the
// paired transport — the one place every remote-origin round trip passes —
// names the home serve, its version when it reports one, and the fix.
// It must stay a PairingError so callers that classify pairing failures
// keep working.
func TestPaired501FoldsIntoUpgradeHint(t *testing.T) {
	srv, _ := skewServe(t, http.StatusNotImplemented, "0.19.1")
	_, err := VerifyPaired(context.Background(), srv.URL, "pair-token")
	if err == nil {
		t.Fatal("501 verify must fail")
	}
	for _, want := range []string{"0.19.1", "upgrade gadak on the home machine"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("paired 501 must name %q, got: %v", want, err)
		}
	}
	var pe *PairingError
	if !errors.As(err, &pe) {
		t.Fatalf("paired 501 must fold into *PairingError, got %T: %v", err, err)
	}
}

// A serve with no version header (a gadak older than the header itself)
// still gets the fold — the sentence just cannot name a version.
func TestPaired501WithoutVersionStillFolds(t *testing.T) {
	srv, _ := skewServe(t, http.StatusNotImplemented, "")
	_, err := VerifyPaired(context.Background(), srv.URL, "pair-token")
	if err == nil {
		t.Fatal("501 verify must fail")
	}
	var pe *PairingError
	if !errors.As(err, &pe) || !strings.Contains(err.Error(), "upgrade gadak on the home machine") {
		t.Fatalf("headerless 501 must still fold with the fix named, got %T: %v", err, err)
	}
}

// TestPaired501KeepsTypedErrorForClassifiers pins the fold's non-negotiable
// property: the sync pass classifies a 501 as a degrade (boards → no agile,
// wiki attachments → skip the listing), and those classifiers key on the
// clients' own *APIError types. The fold wraps the typed error instead of
// swallowing it, so a paired sync against an older serve keeps degrading
// instead of hard-failing — errors.As must find both shapes through the
// PairingError.
func TestPaired501KeepsTypedErrorForClassifiers(t *testing.T) {
	srv, _ := skewServe(t, http.StatusNotImplemented, "0.19.1")
	_, err := VerifyPaired(context.Background(), srv.URL, "pair-token")
	if err == nil {
		t.Fatal("501 verify must fail")
	}
	var je *jira.APIError
	if !errors.As(err, &je) || je.Status != http.StatusNotImplemented {
		t.Fatalf("errors.As must find *jira.APIError 501 through the fold, got %T: %v", err, err)
	}
}

// TestPaired501AnswersOnce is FAIL-first for GDK-1762: the fold turns the
// serve's 501 into an error at RoundTrip level, and the client's retry
// ladder read that as a transport failure — five attempts and 1+2+4+8 s
// of backoff before the upgrade hint appeared (measured 15.01 s per verb,
// base 4c076f72). The folded error carries httppolicy's answer marker, so
// the ladder must return it on the first attempt. The backoff override is
// the documented jira seam: a regression's red run stays milliseconds
// instead of re-walking the very ladder this test exists to ban.
func TestPaired501AnswersOnce(t *testing.T) {
	orig := jira.DefaultBackoff
	jira.DefaultBackoff = time.Millisecond
	t.Cleanup(func() { jira.DefaultBackoff = orig })
	srv, hits := skewServe(t, http.StatusNotImplemented, "0.19.1")
	_, err := VerifyPaired(context.Background(), srv.URL, "pair-token")
	if err == nil {
		t.Fatal("501 verify must fail")
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("paired 501 is an answer, not a transient — want 1 attempt, got %d", got)
	}
}
