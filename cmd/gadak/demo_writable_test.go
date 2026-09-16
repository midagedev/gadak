package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
)

// TestWritableDemoServesAWritableOrigin is GDK-1959's gate.
//
// The property under test is the one bit every client reads before it paints
// a write control: `GET credential/`'s `configured`, which is
// config.HasCredential. The read-only demo is a connected workspace with no
// credential, so that bit is false and the web and the phone ship every
// comment box, transition chip and attachment picker disabled — measured on
// the phone at mobile/src/lib/store.svelte.ts (the writability probe) and in
// mobile/e2e/attach.spec.ts, which has to stub this very endpoint before it
// can tap anything. `--writable` migrates the same snapshot onto the built-in
// tracker, where the bit is true by construction.
//
// Both halves are asserted here, because the assertion that matters is the
// difference: a test that only checked the writable side would still pass if
// the read-only demo silently became writable too, and that is a different
// product (the demo must not pretend to reach a Jira it has no account for).
//
// FAIL-first: the writable half fails on the pre-change tree with
// "makeDemoWritable undefined" — and, more usefully, the same assertion run
// against a home prepared the read-only way reports configured=false, which
// is the defect itself.
func TestWritableDemoServesAWritableOrigin(t *testing.T) {
	fixture := filepath.Join("..", "..", "examples", "demo.db")
	if _, err := os.Stat(fixture); err != nil {
		t.Skipf("demo fixture absent: %v", err)
	}

	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	clearCredentialEnv(t)
	src, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "gadak.db"), src, 0o600); err != nil {
		t.Fatal(err)
	}
	// The read-only demo's own config, spelled the way cmdDemo spells it:
	// projects and an email, no site and no token.
	if err := os.WriteFile(filepath.Join(home, "config.json"),
		[]byte(`{"projects":["NMB","NMA","NMS"],"email":"`+demoUserEmail+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	// The bytes, seeded the way cmdDemo seeds them — the writable path's
	// export refuses rather than migrate attachments as empty metadata, so
	// this ordering is a real dependency, not test scaffolding.
	if err := importDemoAttachments(filepath.Dir(fixture), home); err != nil {
		t.Fatalf("seed the demo attachment cache: %v", err)
	}
	config.SetProfile("")
	t.Cleanup(func() {
		allowProfileCreate = false
		_ = origin.Close()
		config.SetProfile("")
	})

	// ── the defect, stated as a measurement ──────────────────────────────
	roCfg, err := config.LoadFor("")
	if err != nil {
		t.Fatal(err)
	}
	if roCfg.HasCredential() {
		t.Fatalf("the read-only demo reports a credential; it has no site and no token, and a demo must not claim to reach a Jira it cannot")
	}

	// ── --writable's half ────────────────────────────────────────────────
	report, err := capture(t, func() error { return makeDemoWritable(home, true, demoAccountID(filepath.Join(home, "gadak.db"))) })
	if err != nil {
		t.Fatalf("makeDemoWritable: %v\n%s", err, report)
	}
	wCfg, err := config.LoadFor(writableDemoWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	if !wCfg.HasCredential() {
		t.Fatalf("the writable demo workspace reports no credential — its clients would paint every write control disabled")
	}
	if wCfg.OriginType() != config.OriginGadak {
		t.Errorf("writable demo origin_type = %q, want %q — writes must reach the built-in tracker, not a Jira", wCfg.OriginType(), config.OriginGadak)
	}

	// The demo's two corrections to a raw snapshot have to reach the new
	// workspace too, or --writable serves a demo that is subtly worse than
	// the read-only one: nothing is "mine" (GDK-1729) and it opens on a
	// stale-sync warning.
	if wCfg.Email != demoUserEmail {
		t.Errorf("writable demo email = %q, want %q", wCfg.Email, demoUserEmail)
	}
	if wCfg.AccountID == "" {
		t.Error("writable demo carries no account id — nothing in the fixture would be the reader's own")
	}

	dbPath, err := config.DBPathFor(writableDemoWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	n, err := demoIssueCount(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatalf("the writable demo mirrored no issues:\n%s", report)
	}

	// What the two fixes before this one bought, asserted here because the
	// writable demo is their first real user: the attachment bytes came
	// from the cache with no origin to ask (GDK-1960), and the sprints
	// travelled (GDK-1961), which is why the demo still opens on a sprint
	// line. Both are read from the report the migration printed.
	if !strings.Contains(report, "local cache") {
		t.Errorf("the report does not say the attachment bytes came from the cache:\n%s", report)
	}
	if !strings.Contains(report, "sprints") {
		t.Errorf("the report has no sprints row — a writable demo with no sprint line:\n%s", report)
	}
}
