package dashboards

// The dashboard library cache (GDK-808): the one place external JavaScript
// enters this product, and only through the front door. `gadak dashboards
// lib add <url>` downloads a library once (a user-invoked fetch to a URL the
// user typed), pins its sha384 in a manifest next to the bytes, and from then
// on dashboards load that library from the local route
// /api/v1/dashboards/libs/{id} — never from the network. Render never
// fetches; the CSP never names an external host; a tampered cache file is
// refused at serve time by the pinned hash, so what executed is what the
// user hashed or nothing.
//
// The cache is disposable by the same rule as the mirror: delete
// <profile>/dashboards/libs/ and re-run lib add. That is why the manifest is
// a file in this directory and not a local.db table — local.db holds state
// that must survive (views, dashboards, history), the lib cache holds bytes
// that can always be re-fetched, and a manifest row whose file did not
// travel with it would be a dangling reference pretending to be exportable.
// Writers are CLI-only and serialized by the user (one add at a time); the
// server only reads.

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/midagedev/gadak/internal/fsperm"
	"github.com/midagedev/gadak/internal/httppolicy"
)

// Cache limits. MaxLibBytes bounds both a lying Content-Length and an
// infinite stream (the reader is capped one byte above, so a body at the
// limit is read to that point and refused, never buffered unbounded).
// Redirects are bounded because a redirect chain is attacker-controlled
// routing, not content. The timeout reuses the origin clients' budget
// (httppolicy.DefaultTimeout): one user-invoked fetch, no retries — a 5xx is
// an answer, and retrying would re-download bytes the user pays for. The
// rest of httppolicy (retryable statuses, the 64 MiB MaxBody) belongs to
// API clients; this is not one.
const (
	// 50 MiB (GDK-808, user call 2026-08-24): the cap is a mistake guard on a
	// user-invoked download, not a security boundary — sized so a whole built
	// app bundle can ride as one lib.
	MaxLibBytes     = 50 << 20
	MaxLibRedirects = 3
	MaxLibs         = 8
)

// LibIDPattern is the lib id rule: 16 hex of the content's sha256, a dash,
// then the sanitized original basename. The id is also the cache filename,
// so the pattern doubles as the path-safety contract — no separators, no
// "..", nothing that can escape libs/. Quoted in violation messages like
// NamePattern so an author can fix the config without a second round-trip.
const LibIDPattern = `[a-f0-9]{16}-[A-Za-z0-9](?:[A-Za-z0-9._-]{0,62}[A-Za-z0-9_-])?`

var libIDRe = regexp.MustCompile(`^` + LibIDPattern + `$`)

// ValidLibID is the lib id rule used by ParseConfig and by the manifest
// loader (a hand-edited manifest must not smuggle a path through the id).
func ValidLibID(id string) bool { return libIDRe.MatchString(id) }

// ErrLibNotFound is returned by libLookup/LibRemove for an id the manifest
// does not carry. The serve route answers 404 on it, like the vendor route.
var ErrLibNotFound = errors.New("lib not found")

// ErrLibCorrupt means bytes exist but do not match their pinned hash (or the
// manifest itself does not parse) — an operator problem, surfaced as a 500
// by the serve route, never as a silently-served file.
var ErrLibCorrupt = errors.New("lib cache corrupt")

// Lib is one manifest entry. ID is also the filename inside the cache dir,
// so entry and bytes cannot drift apart.
type Lib struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	SHA384    string `json:"sha384"`
	Size      int64  `json:"size"`
	FetchedAt string `json:"fetched_at"`
}

// LibsDir is the cache directory under a profile directory. Single owner of
// the path shape: the CLI passes config.Dir(), the server config.DirFor of
// its profile — both get "dashboards/libs" from here, never spelled twice.
func LibsDir(profileDir string) string {
	return filepath.Join(profileDir, "dashboards", "libs")
}

const manifestFile = "manifest.json"

// loadManifest reads the cache manifest. A missing file is an empty cache
// (first run, or the user deleted the cache — both legal). Anything else
// that fails to parse is ErrLibCorrupt: fail closed, because every serve
// decision below trusts this file's hash.
func loadManifest(dir string) ([]Lib, error) {
	raw, err := os.ReadFile(filepath.Join(dir, manifestFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%w: reading %s: %w", ErrLibCorrupt, manifestFile, err)
	}
	var m struct {
		Libs []Lib `json:"libs"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("%w: %s is not valid JSON: %w", ErrLibCorrupt, manifestFile, err)
	}
	seen := make(map[string]bool, len(m.Libs))
	for _, l := range m.Libs {
		if !ValidLibID(l.ID) {
			return nil, fmt.Errorf("%w: manifest entry id %q fails %s", ErrLibCorrupt, l.ID, LibIDPattern)
		}
		if l.SHA384 == "" || l.Size < 0 || l.URL == "" {
			return nil, fmt.Errorf("%w: manifest entry %q is incomplete", ErrLibCorrupt, l.ID)
		}
		if seen[l.ID] {
			return nil, fmt.Errorf("%w: manifest lists %q twice", ErrLibCorrupt, l.ID)
		}
		seen[l.ID] = true
	}
	sort.Slice(m.Libs, func(i, j int) bool { return m.Libs[i].ID < m.Libs[j].ID })
	return m.Libs, nil
}

// writeManifest persists the manifest atomically at 0600 (same discipline as
// config.json: temp file in the target directory, then rename).
func writeManifest(dir string, libs []Lib) error {
	sort.Slice(libs, func(i, j int) bool { return libs[i].ID < libs[j].ID })
	body, err := json.MarshalIndent(struct {
		Libs []Lib `json:"libs"`
	}{Libs: libs}, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return writeFileAtomic(filepath.Join(dir, manifestFile), body)
}

// writeFileAtomic creates-replaces one cache file: temp in the same
// directory (same filesystem, so the rename is atomic), 0600, then rename.
func writeFileAtomic(dst string, body []byte) error {
	f, err := os.CreateTemp(filepath.Dir(dst), "."+filepath.Base(dst)+".tmp*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp) // no-op after a successful rename
	if _, err := f.Write(body); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

// LibList returns every cached lib, sorted by id.
func LibList(dir string) ([]Lib, error) {
	return loadManifest(dir)
}

// libLookup resolves one id to its manifest entry.
func libLookup(dir, id string) (Lib, error) {
	libs, err := loadManifest(dir)
	if err != nil {
		return Lib{}, err
	}
	for _, l := range libs {
		if l.ID == id {
			return l, nil
		}
	}
	return Lib{}, fmt.Errorf("%w: %s", ErrLibNotFound, id)
}

// validateLibURL is the outbound rule for lib add: https to any host, http
// only to a host this machine plausibly is (IP literal or localhost — a
// self-hosted mirror), everything else refused. javascript:, data:, file:
// and friends fail the scheme test. Every redirect hop is re-validated
// against the same rule; there is no "trusted after the first hop".
func validateLibURL(u *url.URL) error {
	if u == nil {
		return errors.New("empty url")
	}
	host := u.Hostname()
	switch u.Scheme {
	case "https":
		if host == "" {
			return fmt.Errorf("https url has no host: %s", u)
		}
		return nil
	case "http":
		if host == "localhost" || strings.HasSuffix(host, ".localhost") {
			return nil
		}
		// Same judgment the browser guard makes of a Host header: a bare IP
		// literal is this machine's own address space.
		if net.ParseIP(host) != nil {
			return nil
		}
		return fmt.Errorf("http is allowed only to localhost/IP hosts (self-hosted libraries), not %q — use https", host)
	default:
		return fmt.Errorf("url scheme must be https (or http to localhost): got %q", u.Scheme)
	}
}

// sanitizeLibName reduces a URL basename to the id-safe alphabet: keep
// [A-Za-z0-9._-], anything else becomes '-', edges trimmed to alnum (a
// trailing dot or a leading dash would weaken the filename), capped at 64.
// Empty after all that (a data: style path, a unicode name) becomes "lib".
func sanitizeLibName(base string) string {
	var b strings.Builder
	for _, r := range base {
		switch {
		case r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	s := b.String()
	s = strings.Trim(s, ".-")
	if len(s) > 64 {
		s = s[:64]
		s = strings.TrimRight(s, ".-")
	}
	if s == "" {
		return "lib"
	}
	return s
}

// libHTTPClient is the one-shot downloader: bounded redirects, each hop
// re-validated, whole-request timeout from the shared policy.
func libHTTPClient() *http.Client {
	return &http.Client{
		Timeout: httppolicy.DefaultTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > MaxLibRedirects {
				return fmt.Errorf("more than %d redirects", MaxLibRedirects)
			}
			if err := validateLibURL(req.URL); err != nil {
				return fmt.Errorf("redirect to %s refused: %w", req.URL, err)
			}
			return nil
		},
	}
}

// LibAdd downloads rawURL once and caches it. added is false when the exact
// bytes are already cached (same URL and sha384 — the idempotent re-run).
// Same URL with a different hash is an upstream change: refused unless
// replace, because a dashboard's config pins the old id and a silent swap
// would change what its next render executes.
func LibAdd(ctx context.Context, dir, rawURL string, replace bool, now time.Time) (lib Lib, added bool, err error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return Lib{}, false, fmt.Errorf("lib add: %w", err)
	}
	if err := validateLibURL(u); err != nil {
		return Lib{}, false, fmt.Errorf("lib add: %w", err)
	}
	if err := fsperm.EnsurePrivateDir(dir); err != nil {
		return Lib{}, false, err
	}
	libs, err := loadManifest(dir)
	if err != nil {
		return Lib{}, false, err
	}
	var prior *Lib
	for i := range libs {
		if libs[i].URL == rawURL {
			prior = &libs[i]
			break
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Lib{}, false, fmt.Errorf("lib add: %w", err)
	}
	resp, err := libHTTPClient().Do(req)
	if err != nil {
		return Lib{}, false, fmt.Errorf("lib add: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Lib{}, false, fmt.Errorf("lib add: %s: %s", rawURL, resp.Status)
	}
	// One byte past the cap: a body larger than MaxLibBytes reads long
	// enough to be recognized and refused, never buffered whole.
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxLibBytes+1))
	if err != nil {
		return Lib{}, false, fmt.Errorf("lib add: reading %s: %w", rawURL, err)
	}
	if len(body) > MaxLibBytes {
		return Lib{}, false, fmt.Errorf("lib add: %s exceeds the %d MiB cache limit", rawURL, MaxLibBytes>>20)
	}
	sum256 := sha256.Sum256(body)
	sum384 := sha512.Sum384(body)
	id := hex.EncodeToString(sum256[:8]) + "-" + sanitizeLibName(path.Base(resp.Request.URL.Path))
	entry := Lib{
		ID:        id,
		URL:       rawURL,
		SHA384:    hex.EncodeToString(sum384[:]),
		Size:      int64(len(body)),
		FetchedAt: now.UTC().Format(time.RFC3339),
	}

	// Same URL, changed bytes: an upstream swap the user must confirm — a
	// dashboard's config pins the old id, and a silent swap would change
	// what its next render executes.
	if prior != nil && prior.SHA384 != entry.SHA384 {
		if !replace {
			return Lib{}, false, fmt.Errorf(
				"lib add: %s changed since it was cached (sha384 %s… → %s…) — pass --replace to swap the cache entry; dashboards referencing the old id %q keep working until you re-save them",
				rawURL, prior.SHA384[:16], entry.SHA384[:16], prior.ID)
		}
		libs = removeLib(libs, prior.ID)
		if err := os.Remove(filepath.Join(dir, prior.ID)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return Lib{}, false, err
		}
	}
	// added is false when these exact bytes (this id) were already cached —
	// same URL re-run, or another URL serving identical bytes under the same
	// basename. Provenance refreshes either way.
	added = true
	for i := range libs {
		if libs[i].ID == entry.ID {
			libs[i] = entry
			added = false
			break
		}
	}
	if added {
		libs = append(libs, entry)
	}
	if err := persistLib(dir, libs, entry, body); err != nil {
		return Lib{}, false, err
	}
	return entry, added, nil
}

// persistLib writes the bytes then the manifest: a crash between the two
// leaves an unreferenced file (harmless, invisible), never a manifest entry
// without its bytes.
func persistLib(dir string, libs []Lib, entry Lib, body []byte) error {
	if err := writeFileAtomic(filepath.Join(dir, entry.ID), body); err != nil {
		return err
	}
	return writeManifest(dir, libs)
}

// LibRemove drops one entry and its bytes. Unknown id is ErrLibNotFound.
func LibRemove(dir, id string) error {
	libs, err := loadManifest(dir)
	if err != nil {
		return err
	}
	next := removeLib(libs, id)
	if len(next) == len(libs) {
		return fmt.Errorf("%w: %s", ErrLibNotFound, id)
	}
	if err := writeManifest(dir, next); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(dir, id)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func removeLib(libs []Lib, id string) []Lib {
	out := libs[:0:0]
	for _, l := range libs {
		if l.ID != id {
			out = append(out, l)
		}
	}
	return out
}

// LibReadVerified is the serve path's whole trust decision: read the bytes,
// re-hash them, and hand them over only when they still match the pin. The
// manifest itself not parsing is ErrLibCorrupt here too — a cache whose
// pin cannot be read serves nothing. Size is checked first because it is
// free and catches truncation without hashing.
func LibReadVerified(dir, id string) (Lib, []byte, error) {
	if !ValidLibID(id) {
		return Lib{}, nil, fmt.Errorf("%w: %s", ErrLibNotFound, id)
	}
	entry, err := libLookup(dir, id)
	if err != nil {
		return Lib{}, nil, err
	}
	body, err := os.ReadFile(filepath.Join(dir, entry.ID))
	if errors.Is(err, os.ErrNotExist) {
		return Lib{}, nil, fmt.Errorf("%w: cache file %s is gone — re-run `gadak dashboards lib add %s`", ErrLibCorrupt, entry.ID, entry.URL)
	}
	if err != nil {
		return Lib{}, nil, fmt.Errorf("%w: reading %s: %w", ErrLibCorrupt, entry.ID, err)
	}
	if int64(len(body)) != entry.Size {
		return Lib{}, nil, fmt.Errorf("%w: %s no longer matches its pinned size — the cache was modified after lib add; re-run `gadak dashboards lib add %s` to restore it", ErrLibCorrupt, entry.ID, entry.URL)
	}
	sum := sha512.Sum384(body)
	if hex.EncodeToString(sum[:]) != entry.SHA384 {
		return Lib{}, nil, fmt.Errorf("%w: %s no longer matches its pinned sha384 — the cache was modified after lib add; re-run `gadak dashboards lib add %s` to restore it", ErrLibCorrupt, entry.ID, entry.URL)
	}
	return entry, body, nil
}

// LibsExist reports which of ids the cache does not carry — the save paths'
// existence check. A manifest that fails to parse names no lib (every id is
// "missing"), which is fail-closed: nothing saves against an unreadable pin.
func LibsExist(dir string, ids []string) (missing []string, err error) {
	libs, mErr := loadManifest(dir)
	have := make(map[string]bool, len(libs))
	for _, l := range libs {
		have[l.ID] = true
	}
	for _, id := range ids {
		if !have[id] {
			missing = append(missing, id)
		}
	}
	return missing, mErr
}
