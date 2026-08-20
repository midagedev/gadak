// Package pairing owns the device tokens that gate a serve's origin
// passthrough once it is exposed beyond loopback (GDK-433).
//
// Model: each paired device — a remote gadak binding this serve as its
// workspace origin — gets an opaque random token, presented as
// `Authorization: Bearer <token>` (the Jira DC PAT shape; the Cloud
// email:token Basic shape is deliberately not reused). The server stores
// only the SHA-256 hash plus metadata, in `pairing.json` inside the profile
// directory: next to config.json and its 0600 atomic-write convention, and
// never inside the mirror (gadak.db is a disposable cache of the origin,
// not a place for originals) or any export. The plaintext exists once, in
// `gadak pairing mint` output, inside a one-line offer.
//
// Gate semantics (single owner: server.handleOriginREST): while at least
// one active — unrevoked, unexpired — token exists, every request under
// origin.RESTPrefix must present a valid Bearer token. With no active
// token the passthrough behaves exactly as before (implicit loopback
// trust, decision 0003). There is no loopback bypass: a tailnet proxy
// reaches the serve as loopback, so the token is the only identity the
// server can distinguish.
package pairing

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ScopeOrigin is full origin passthrough — the device-token scope.
// It exists in the stored metadata so a narrower scope (read-only,
// wiki-only, local-routing) can be told apart from tokens minted before
// scopes mattered.
const ScopeOrigin = "origin"

// ScopeLocalRouting is the home machine's own routing token. pairing list
// uses this so the `_home` row is not mistaken for a paired device.
const ScopeLocalRouting = "local-routing"

// HomeLabel is the reserved label of the home routing token. It is not a
// device: revoking it locks local writes while a serve is running.
const HomeLabel = "_home"

// StoreRel is the profile-relative path of the token store. A separate
// file, not a config.json key: config.json is settings the UI edits and
// exports surface; this is a credential-adjacent secret list.
const StoreRel = "pairing.json"

// tokenBytes is the entropy of a minted token: 32 random bytes, base64url
// (43 characters). Opaque — no structure for a remote party to parse.
const tokenBytes = 32

// StorePath is the absolute token-store path inside a profile directory.
func StorePath(dir string) string {
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, StoreRel)
}

// Meta is one stored token. Hash is the hex SHA-256 of the plaintext; the
// plaintext is never persisted anywhere.
type Meta struct {
	Label      string     `json:"label"`
	Scope      string     `json:"scope"`
	Hash       string     `json:"hash"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

// Active reports whether this token admits requests at now. The home
// origin's clock is authoritative (GDK-369): expiry is judged where the
// store lives, never by a client-supplied timestamp.
func (m Meta) Active(now time.Time) bool {
	return m.RevokedAt == nil && !now.After(m.ExpiresAt)
}

type storeDoc struct {
	Tokens []Meta `json:"tokens"`
}

// Verdict is Authorize's answer.
type Verdict int

const (
	// VerdictOff: no active token exists, so the gate does not apply and
	// the passthrough keeps its pre-pairing behavior.
	VerdictOff Verdict = iota
	// VerdictAccept: a valid Bearer token was presented.
	VerdictAccept
	// VerdictReject: active tokens exist but the request carried no valid
	// one. Missing and wrong are the same answer — do not reveal which.
	VerdictReject
)

// Mint creates a token for label, persists its hash, and returns the
// plaintext. The plaintext appears exactly here; callers print it once (as
// an offer) and never store it. A duplicate *active* label is refused so
// `pairing revoke <label>` stays unambiguous; a revoked label may be reused.
func Mint(dir, label string, ttl time.Duration, now time.Time) (string, Meta, error) {
	token, meta, err := prepareMint(label, ttl, now)
	if err != nil {
		return "", Meta{}, err
	}
	err = mutateStore(dir, func(doc *storeDoc) error {
		for _, m := range doc.Tokens {
			if m.Label == meta.Label && m.Active(now) {
				return fmt.Errorf("pairing: an active token labeled %q already exists; revoke it first or pick another label", meta.Label)
			}
		}
		doc.Tokens = append(doc.Tokens, meta)
		return nil
	})
	if err != nil {
		return "", Meta{}, err
	}
	return token, meta, nil
}

// Rotate replaces every live token with label by a newly minted one in a
// single store write, so pairing list never shows two live rows with the
// same name. Used for the home routing token: mint + revoke of the previous
// `_home` must not be two verbs a crash can split.
func Rotate(dir, label string, ttl time.Duration, now time.Time) (string, Meta, error) {
	token, meta, err := prepareMint(label, ttl, now)
	if err != nil {
		return "", Meta{}, err
	}
	err = mutateStore(dir, func(doc *storeDoc) error {
		t := now.UTC()
		for i := range doc.Tokens {
			if doc.Tokens[i].Label == meta.Label && doc.Tokens[i].RevokedAt == nil {
				doc.Tokens[i].RevokedAt = &t
			}
		}
		doc.Tokens = append(doc.Tokens, meta)
		return nil
	})
	if err != nil {
		return "", Meta{}, err
	}
	return token, meta, nil
}

func prepareMint(label string, ttl time.Duration, now time.Time) (string, Meta, error) {
	label = strings.TrimSpace(label)
	if label == "" {
		return "", Meta{}, errors.New("pairing: label is required")
	}
	if len(label) > 64 {
		return "", Meta{}, errors.New("pairing: label is longer than 64 characters")
	}
	if ttl <= 0 {
		return "", Meta{}, errors.New("pairing: ttl must be positive")
	}
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", Meta{}, fmt.Errorf("pairing: mint: %w", err)
	}
	token := encodeToken(raw)
	meta := Meta{
		Label:     label,
		Scope:     scopeForLabel(label),
		Hash:      hashToken(token),
		CreatedAt: now.UTC(),
		ExpiresAt: now.UTC().Add(ttl),
	}
	return token, meta, nil
}

func scopeForLabel(label string) string {
	if label == HomeLabel {
		return ScopeLocalRouting
	}
	return ScopeOrigin
}

// List returns every stored token, earliest created first, including
// revoked and expired ones — revocation is audit history, not noise.
func List(dir string) ([]Meta, error) {
	doc, err := readStore(dir)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, nil
	}
	out := append([]Meta(nil), doc.Tokens...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// Revoke revokes the token selected by exact label or hash prefix and
// returns it. Refuses ambiguities instead of guessing, and refuses an
// already-revoked token rather than no-op'ing silently. Revoked entries
// stay in the store as audit history but never stand as candidates: a
// re-minted label revokes by label again, pointed at the live mint.
func Revoke(dir, selector string, now time.Time) (Meta, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return Meta{}, errors.New("pairing: revoke needs a label or hash prefix")
	}
	if selector == HomeLabel {
		return Meta{}, revokeHomeError()
	}
	var out Meta
	err := mutateStore(dir, func(doc *storeDoc) error {
		var byLabel, byPrefix []int
		revoked := false
		for i, m := range doc.Tokens {
			if m.RevokedAt != nil {
				if m.Label == selector {
					revoked = true
				}
				continue
			}
			if m.Label == selector {
				byLabel = append(byLabel, i)
			}
			if len(selector) >= minPrefix && strings.HasPrefix(strings.ToLower(m.Hash), strings.ToLower(selector)) {
				byPrefix = append(byPrefix, i)
			}
		}
		cands := byLabel
		if len(cands) == 0 {
			cands = byPrefix
		}
		switch len(cands) {
		case 0:
			if revoked {
				return fmt.Errorf("pairing: token %q is already revoked", selector)
			}
			return fmt.Errorf("pairing: no token matches %q", selector)
		case 1:
		default:
			return fmt.Errorf("pairing: %q matches %d tokens; revoke by a longer hash prefix", selector, len(cands))
		}
		i := cands[0]
		if doc.Tokens[i].Label == HomeLabel {
			return revokeHomeError()
		}
		t := now.UTC()
		doc.Tokens[i].RevokedAt = &t
		out = doc.Tokens[i]
		return nil
	})
	return out, err
}

func revokeHomeError() error {
	return fmt.Errorf("%s is this machine's own routing key — revoking it locks local writes while a serve is running. to rotate it: gadak pairing mint --label %s", HomeLabel, HomeLabel)
}

// minPrefix is the shortest hash prefix revoke accepts. `pairing list`
// shows 8 characters, so 8 is the natural floor.
const minPrefix = 8

// Authorize decides whether bearer may pass the gate at now. It fails
// closed: a store that cannot be read rejects, because tokens may exist;
// only a proven-absent store is VerdictOff. On accept it records
// last_used_at (throttled to one disk write per token per interval).
func Authorize(dir, bearer string, now time.Time) (Verdict, error) {
	doc, err := loadCached(dir)
	if err != nil {
		return VerdictReject, err
	}
	if doc == nil {
		return VerdictOff, nil
	}
	digest, _ := digestOf(bearer)
	anyActive := false
	for _, m := range doc.Tokens {
		if !m.Active(now) {
			continue
		}
		anyActive = true
		if digest != nil && digestMatches(m.Hash, digest) {
			touchLastUsed(dir, m.Hash, now)
			return VerdictAccept, nil
		}
	}
	if !anyActive {
		return VerdictOff, nil
	}
	return VerdictReject, nil
}

// lastUsedInterval throttles last_used_at persistence: one write per token
// per interval. last_used is telemetry for `pairing list`, not an audit
// log — a per-request write would put disk IO on the passthrough hot path
// for no decision anyone makes from it.
const lastUsedInterval = 30 * time.Second

func touchLastUsed(dir, hash string, now time.Time) {
	cacheMu.Lock()
	entry := cache[dir]
	cacheMu.Unlock()
	if entry != nil {
		entry.luMu.Lock()
		last, wrote := entry.lastPersisted[hash]
		if wrote && now.Sub(last) < lastUsedInterval {
			entry.luMu.Unlock()
			return
		}
		entry.lastPersisted[hash] = now
		entry.luMu.Unlock()
	}
	// Re-read before writing: mint/revoke from another process (the usual
	// way tokens change) must not be clobbered by this stale copy.
	_ = mutateStore(dir, func(doc *storeDoc) error {
		t := now.UTC()
		for i := range doc.Tokens {
			if doc.Tokens[i].Hash == hash && doc.Tokens[i].RevokedAt == nil {
				doc.Tokens[i].LastUsedAt = &t
			}
		}
		return nil
	})
}

var (
	// cacheMu guards cache. The serve process calls Authorize per
	// passthrough request; stat-then-maybe-read keeps that at one stat.
	cacheMu sync.Mutex
	cache   = map[string]*cacheEntry{}
)

type cacheEntry struct {
	mu            sync.Mutex
	mod           time.Time
	size          int64
	doc           *storeDoc
	luMu          sync.Mutex
	lastPersisted map[string]time.Time
}

// loadCached returns the store for dir, re-reading when the file changed
// since the last load. The store is shared with `gadak pairing` running
// as a separate process, so freshness is decided by stat, not by trust.
func loadCached(dir string) (*storeDoc, error) {
	p := StorePath(dir)
	if p == "" {
		return nil, nil
	}
	fi, err := os.Stat(p)
	if os.IsNotExist(err) {
		cacheMu.Lock()
		delete(cache, dir)
		cacheMu.Unlock()
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	cacheMu.Lock()
	entry := cache[dir]
	cacheMu.Unlock()
	if entry == nil {
		entry = &cacheEntry{lastPersisted: map[string]time.Time{}}
		cacheMu.Lock()
		cache[dir] = entry
		cacheMu.Unlock()
	}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if entry.doc != nil && entry.mod.Equal(fi.ModTime()) && entry.size == fi.Size() {
		return entry.doc, nil
	}
	doc, err := readStore(dir)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, nil
	}
	entry.mod, entry.size, entry.doc = fi.ModTime(), fi.Size(), doc
	return doc, nil
}

// readStore reads and parses the store file. Missing file is (nil, nil).
func readStore(dir string) (*storeDoc, error) {
	p := StorePath(dir)
	if p == "" {
		return nil, nil
	}
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var doc storeDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("pairing: %s: %w", StoreRel, err)
	}
	return &doc, nil
}

// mutateStore applies fn to a freshly read store and writes it back
// atomically (temp + rename, 0600, like config.Save). Read-modify-write
// under a per-path lock so two local mutators cannot interleave.
func mutateStore(dir string, fn func(*storeDoc) error) error {
	p := StorePath(dir)
	if p == "" {
		return errors.New("pairing: no profile directory")
	}
	cacheMu.Lock()
	entry := cache[dir]
	cacheMu.Unlock()
	if entry == nil {
		entry = &cacheEntry{lastPersisted: map[string]time.Time{}}
		cacheMu.Lock()
		cache[dir] = entry
		cacheMu.Unlock()
	}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	doc, err := readStore(dir)
	if err != nil {
		return err
	}
	if doc == nil {
		doc = &storeDoc{}
	}
	if err := fn(doc); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("pairing: profile dir: %w", err)
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if fi, err := os.Stat(p); err == nil {
		entry.mod, entry.size, entry.doc = fi.ModTime(), fi.Size(), doc
	}
	return nil
}

func encodeToken(raw []byte) string {
	return base64.RawURLEncoding.EncodeToString(raw)
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// digestOf hashes a presented bearer string for comparison. An empty
// bearer has no digest: missing is reject, not a hash of "".
func digestOf(bearer string) ([]byte, bool) {
	if bearer == "" {
		return nil, false
	}
	sum := sha256.Sum256([]byte(bearer))
	return sum[:], true
}

// digestMatches compares a stored hex hash against a digest in constant
// time. The inputs are digests, so timing exposure cannot reveal token
// bytes, but constant-time keeps the comparison shape uniform anyway.
func digestMatches(storedHex string, digest []byte) bool {
	raw, err := hex.DecodeString(storedHex)
	if err != nil || len(raw) != len(digest) {
		return false
	}
	return subtle.ConstantTimeCompare(raw, digest) == 1
}
