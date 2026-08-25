package pairing

import (
	"testing"
	"time"
)

// The terminal scope (GDK-863) is the third one-way door, and the one with
// the worst failure mode: a leaked serve token leaks the data in the
// mirror, a leaked terminal token leaks the machine. These tests pin every
// wrong pairing of scope and door, in both directions — the mistake this
// package exists to make impossible is a scope that silently widens.

func TestAdmitsTerminalIsTerminalOnly(t *testing.T) {
	for scope, want := range map[string]bool{
		ScopeTerminal:     true,
		ScopeServe:        false,
		ScopeOrigin:       false,
		ScopeLocalRouting: false,
		"":                false, // minted before the terminal existed
		"anything-else":   false,
	} {
		if got := AdmitsTerminal(scope); got != want {
			t.Errorf("AdmitsTerminal(%q) = %v, want %v", scope, got, want)
		}
	}
}

// The doors are one-way in both directions: a terminal token opens no
// other surface either.
func TestTerminalScopeOpensNothingElse(t *testing.T) {
	if AdmitsOrigin(ScopeTerminal) {
		t.Error("a terminal token may not ride the origin passthrough")
	}
	// The mirror gate keys on ScopeServe exactly (internal/server), so the
	// statement here is that terminal is not that value.
	if ScopeTerminal == ScopeServe || ScopeTerminal == ScopeOrigin || ScopeTerminal == ScopeLocalRouting {
		t.Error("the terminal scope must be a distinct value")
	}
}

func TestMintScopedTerminal(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	token, meta, err := MintScoped(dir, "pane", ScopeTerminal, 12*time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Scope != ScopeTerminal {
		t.Fatalf("scope %q, want %q", meta.Scope, ScopeTerminal)
	}
	v, m, err := AuthorizeMeta(dir, token, now.Add(time.Minute))
	if err != nil || v != VerdictAccept {
		t.Fatalf("authorize terminal token: %v %v", v, err)
	}
	if !AdmitsTerminal(m.Scope) {
		t.Fatalf("matched meta %+v — the gate cannot read the scope it must enforce", m)
	}
	// Never a default: an unnamed scope is still origin.
	_, def, err := MintScoped(dir, "laptop", "", time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	if def.Scope != ScopeOrigin {
		t.Fatalf("default scope %q — terminal must be typed, never inferred", def.Scope)
	}
	// And the reserved routing label still cannot become one.
	_, home, err := MintScoped(dir, HomeLabel, ScopeTerminal, time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	if home.Scope != ScopeLocalRouting {
		t.Fatalf("_home scope %q — a routing token must not acquire a shell", home.Scope)
	}
}

// TokenActive is what the serve's revoke watchdog asks on an interval: the
// CLI edits pairing.json in another process, so a live shell can only
// learn about a revoke by re-reading the store.
func TestTokenActiveFollowsRevoke(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	_, meta, err := MintScoped(dir, "pane", ScopeTerminal, time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	if !TokenActive(dir, meta.Hash, now.Add(time.Minute)) {
		t.Fatal("freshly minted token reads as inactive")
	}
	// Expiry is judged where the store lives.
	if TokenActive(dir, meta.Hash, now.Add(2*time.Hour)) {
		t.Fatal("expired token reads as active")
	}
	if _, err := Revoke(dir, "pane", now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if TokenActive(dir, meta.Hash, now.Add(3*time.Minute)) {
		t.Fatal("revoked token still reads as active — a live shell would survive its revoke")
	}
	// Unknown ids and a missing store fail closed.
	if TokenActive(dir, "0123456789abcdef", now) {
		t.Fatal("unknown hash reads as active")
	}
	if TokenActive(dir, "", now) {
		t.Fatal("empty hash reads as active")
	}
	if TokenActive(t.TempDir(), meta.Hash, now) {
		t.Fatal("a store with no file reads a token as active")
	}
}
