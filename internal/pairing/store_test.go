package pairing

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMintListRevokeRoundTrip(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)

	token, meta, err := Mint(dir, "laptop", 24*time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(token) != 43 { // 32 bytes, base64url, unpadded
		t.Fatalf("token length %d, want 43", len(token))
	}
	if meta.Hash == "" || strings.Contains(meta.Hash, token) {
		t.Fatalf("meta hash must be a digest, not the token")
	}
	if meta.Scope != ScopeOrigin {
		t.Fatalf("scope %q, want %q", meta.Scope, ScopeOrigin)
	}

	// Plaintext never touches the store file.
	data, err := os.ReadFile(StorePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), token) {
		t.Fatal("store file contains the plaintext token")
	}
	fi, err := os.Stat(StorePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("store mode %v, want 0600", fi.Mode().Perm())
	}

	got, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Label != "laptop" || got[0].Hash != meta.Hash {
		t.Fatalf("list = %+v", got)
	}

	// Duplicate active label is refused; a revoked one frees the name.
	if _, _, err := Mint(dir, "laptop", time.Hour, now); err == nil {
		t.Fatal("duplicate active label must be refused")
	}
	revoked, err := Revoke(dir, "laptop", now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if revoked.Label != "laptop" || revoked.RevokedAt == nil {
		t.Fatalf("revoke returned %+v", revoked)
	}
	if _, err := Revoke(dir, "laptop", now.Add(2*time.Minute)); err == nil {
		t.Fatal("revoking an already-revoked token must be an explicit error")
	}
	if _, _, err := Mint(dir, "laptop", time.Hour, now.Add(3*time.Minute)); err != nil {
		t.Fatalf("label reuse after revoke: %v", err)
	}
	// The revived label must revoke by label again: the revoked namesake is
	// audit history, not a live candidate (found live — `revoke laptop`
	// after re-minting refused with a 2-token ambiguity).
	again, err := Revoke(dir, "laptop", now.Add(4*time.Minute))
	if err != nil {
		t.Fatalf("revoking the re-minted label: %v", err)
	}
	if again.RevokedAt == nil || !again.CreatedAt.Equal(now.Add(3*time.Minute)) {
		t.Fatalf("revoked %+v, want the newer mint", again)
	}
}

func TestRevokeByHashPrefix(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	_, a, err := Mint(dir, "aaa", time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	// Short selector (< 8 chars) matches nothing by prefix.
	if _, err := Revoke(dir, a.Hash[:6], now); err == nil {
		t.Fatal("prefix shorter than 8 must not match")
	}
	// A distinguishing prefix works, case-insensitively.
	sel := strings.ToUpper(a.Hash[:10])
	got, err := Revoke(dir, sel, now)
	if err != nil {
		t.Fatal(err)
	}
	if got.Hash != a.Hash {
		t.Fatalf("revoked %+v, want %s", got, a.Hash)
	}
}

func TestRevokeAmbiguousPrefixRefused(t *testing.T) {
	// Random tokens never share an 8-char hash prefix, so the ambiguous
	// case is constructed directly: two entries with colliding prefixes.
	dir := t.TempDir()
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	doc := storeDoc{Tokens: []Meta{
		{Label: "one", Scope: ScopeOrigin, Hash: "abcdef0100000000000000000000000000000000000000000000000000000000",
			CreatedAt: now, ExpiresAt: now.Add(time.Hour)},
		{Label: "two", Scope: ScopeOrigin, Hash: "abcdef01ffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			CreatedAt: now, ExpiresAt: now.Add(time.Hour)},
	}}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(StorePath(dir), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Revoke(dir, "abcdef01", now); err == nil || !strings.Contains(err.Error(), "2 tokens") {
		t.Fatalf("colliding prefix must be refused with the count, got %v", err)
	}
	// A longer prefix still separates them.
	got, err := Revoke(dir, "abcdef010000", now)
	if err != nil || got.Label != "one" {
		t.Fatalf("longer prefix: %+v, %v", got, err)
	}
}

func TestAuthorizeVerdicts(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)

	// No store file: gate off (implicit loopback trust, decision 0003).
	v, err := Authorize(dir, "", now)
	if err != nil || v != VerdictOff {
		t.Fatalf("absent store: %v, %v; want Off, nil", v, err)
	}

	long, _, err := Mint(dir, "long-lived", 24*time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	short, _, err := Mint(dir, "short-lived", time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}

	// Active tokens exist: missing and wrong bearers both reject, with no
	// distinction between them.
	for _, bearer := range []string{"", "not-the-token", short + "x"} {
		if v, _ := Authorize(dir, bearer, now.Add(time.Minute)); v != VerdictReject {
			t.Fatalf("bearer %q: %v, want Reject", bearer[:min(len(bearer), 8)], v)
		}
	}
	if v, _ := Authorize(dir, long, now.Add(time.Minute)); v != VerdictAccept {
		t.Fatalf("valid token: %v, want Accept", v)
	}

	// An expired token is not valid while another active token keeps the
	// gate on (GDK-433 test ④).
	if v, _ := Authorize(dir, short, now.Add(2*time.Hour)); v != VerdictReject {
		t.Fatalf("expired token: %v, want Reject", v)
	}
	// When every token is expired there is no active token: gate off.
	if v, _ := Authorize(dir, "", now.Add(48*time.Hour)); v != VerdictOff {
		t.Fatalf("all expired: %v, want Off", v)
	}
}

func TestAuthorizeRecordsLastUsed(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	token, _, err := Mint(dir, "laptop", 24*time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Authorize(dir, token, now); err != nil {
		t.Fatal(err)
	}
	got, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].LastUsedAt == nil || !got[0].LastUsedAt.Equal(now) {
		t.Fatalf("last_used_at not recorded as %v: %+v", now, got)
	}
}

func TestAuthorizeReloadsAfterExternalChange(t *testing.T) {
	// The serve process caches the store; `gadak pairing revoke` runs as a
	// different process. Simulate that by rewriting the file behind the
	// package's back — the next Authorize must re-stat, re-read, and obey.
	dir := t.TempDir()
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	token, _, err := Mint(dir, "laptop", 24*time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := Authorize(dir, token, now); v != VerdictAccept {
		t.Fatalf("before external revoke: %v", v)
	}
	doc := storeDoc{} // empty store, as if every token were revoked elsewhere
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(StorePath(dir), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if v, _ := Authorize(dir, token, now.Add(2*time.Second)); v != VerdictOff {
		t.Fatalf("after external revoke: %v, want Off (no active tokens)", v)
	}
}

func TestAuthorizeFailsClosedOnCorruptStore(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(StorePath(dir), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	v, err := Authorize(dir, "anything", time.Now())
	if err == nil || v != VerdictReject {
		t.Fatalf("corrupt store: %v, %v; want Reject + error (fail closed)", v, err)
	}
}

func TestRemoteRoundTrip(t *testing.T) {
	dir := t.TempDir()
	r, err := LoadRemote(dir)
	if err != nil || r != nil {
		t.Fatalf("absent remote: %+v, %v; want nil, nil", r, err)
	}
	in := Remote{Endpoint: "https://home.tail1234.ts.net", Token: "tok", Label: "laptop"}
	if err := SaveRemote(dir, in); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(filepath.Join(dir, RemoteRel))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("remote mode %v, want 0600", fi.Mode().Perm())
	}
	out, err := LoadRemote(dir)
	if err != nil {
		t.Fatal(err)
	}
	if out.Endpoint != in.Endpoint || out.Token != in.Token || out.Label != in.Label {
		t.Fatalf("round trip: %+v", out)
	}
	if out.PairedAt == "" {
		t.Fatal("PairedAt should be stamped on save")
	}
}
