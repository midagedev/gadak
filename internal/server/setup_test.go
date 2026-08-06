package server

// Sync-starter behaviour for first-run onboarding: when serve starts without a
// credential, SetSyncStarter registers a one-shot Watch kick; handleConnect
// fires it only on the first successful save.

import (
	"sync/atomic"
	"testing"
)

func TestConnectFiresSyncStarterOnce(t *testing.T) {
	f, hRaw, _ := onboarding(t)
	h, ok := hRaw.(*Handler)
	if !ok {
		t.Fatalf("onboarding handler type %T, want *Handler", hRaw)
	}

	var calls atomic.Int32
	h.SetSyncStarter(func() { calls.Add(1) })

	connect(t, h, f)
	if got := calls.Load(); got != 1 {
		t.Fatalf("after first connect: starter calls = %d, want 1", got)
	}

	// Second successful connect (token rotation / re-verify) must not re-fire.
	rec := send(t, h, "PUT", apiBase+"onboarding/connect/",
		`{"site":"`+f.URL+`","jira_email":"hc@example.com","api_token":"tok-rotated"}`)
	if rec.Code != 200 {
		t.Fatalf("second connect: %d %s", rec.Code, rec.Body.String())
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("after second connect: starter calls = %d, want 1", got)
	}
}

func TestConnectDoesNotFireWhenCredentialAlreadyPresent(t *testing.T) {
	f, hRaw, _ := onboarding(t)
	h := hRaw.(*Handler)

	// Pre-seed a credential on the live config (as if serve started with one).
	// Connect then becomes a rotation, not a first-run setup.
	cfg := h.s.config()
	cfg.Site, cfg.Email, cfg.Token = f.URL, "hc@example.com", "already-set"
	h.s.cfg.Store(cfg)

	var calls atomic.Int32
	h.SetSyncStarter(func() { calls.Add(1) })

	connect(t, h, f)
	if got := calls.Load(); got != 0 {
		t.Fatalf("starter calls = %d, want 0 when credential already present", got)
	}
}

func TestConnectSyncStarterUnregisteredIsNoop(t *testing.T) {
	f, hRaw, _ := onboarding(t)
	// No SetSyncStarter — first connect must still succeed.
	connect(t, hRaw, f)

	// And a later registration must not retroactively fire from a past connect.
	h := hRaw.(*Handler)
	var calls atomic.Int32
	h.SetSyncStarter(func() { calls.Add(1) })
	if got := calls.Load(); got != 0 {
		t.Fatalf("starter calls = %d after late registration, want 0", got)
	}
}
