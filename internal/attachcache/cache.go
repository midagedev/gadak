// Package attachcache stores attachment bytes on local disk.
//
// Attachment bytes were proxied from Jira on every view, which contradicts the
// premise of the tool: everything else answers from local disk, and an issue with
// three screenshots re-downloaded them on every open. Bytes for a given
// attachment id are immutable in Jira, so they cache indefinitely and the only
// bound needed is total size.
//
// A cached attachment also survives having no credential at all, which is what
// lets the bundled demo snapshot show real images offline.
package attachcache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// defaultMaxBytes bounds the cache. Attachments are usually screenshots, so a
// few hundred megabytes holds a working set of thousands.
const defaultMaxBytes int64 = 512 << 20

// defaultMaxEntryBytes skips files too large to be worth caching for a UI
// that only renders images, PDFs, and short clips inline. It is a default,
// not a ceiling: an origin's own upload cap is configurable now, and a
// workspace whose largest file is 884 MiB should be able to say so
// (GDK-1617).
const defaultMaxEntryBytes int64 = 64 << 20

// Meta is the sidecar recorded next to each cached file. Content-Type has to
// survive a restart, and guessing it back from bytes is worse than storing it.
type Meta struct {
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	Filename    string `json:"filename,omitempty"`
}

// Key is the on-disk identity for an attachment. Site (and issue) are mixed in
// so a profile that changes sites cannot serve the previous site's bytes, and a
// request for the wrong issue cannot read a cached id.
//
// An empty site keeps the legacy id-only form so snapshot imports for
// `gadak demo` / export-static (no site) stay reachable. Existing id-only
// files on a real site miss after this change (the safe invalidation).
func Key(site, profile, issue, id string) string {
	site = strings.TrimRight(strings.TrimSpace(site), "/")
	if site == "" {
		return id
	}
	var b strings.Builder
	b.Grow(len(site) + len(profile) + len(issue) + len(id) + 3)
	b.WriteString(site)
	b.WriteByte('\x1f')
	b.WriteString(profile)
	b.WriteByte('\x1f')
	b.WriteString(issue)
	b.WriteByte('\x1f')
	b.WriteString(id)
	return b.String()
}

// Cache is a content-addressed store under one directory. Safe for concurrent
// use; a miss for the same id collapses into a single fetch.
type Cache struct {
	dir      string
	maxBytes int64
	maxEntry int64

	mu     sync.Mutex
	flight map[string]*fill
}

// fill is one in-flight Fill. err carries the owner's outcome to every
// waiter — without it, a waiter saw a fresh errors.New and the caller's
// typed branches (auth, too-large) all fell through to default (GDK-1237).
type fill struct {
	wg  sync.WaitGroup
	err error
}

// New opens (and creates) a cache directory. maxBytes <= 0 means defaultMaxBytes.
// New opens the cache. maxBytes bounds the directory, maxEntry the largest
// single file it will keep; either 0 takes the package default.
func New(dir string, maxBytes, maxEntry int64) (*Cache, error) {
	if dir == "" {
		return nil, errors.New("attachcache: empty directory")
	}
	if maxBytes <= 0 {
		maxBytes = defaultMaxBytes
	}
	switch {
	case maxEntry < 0:
		// Negative means the user turned the ceiling off — the same shape
		// the origin's own upload cap uses. MaxInt64 rather than a branch
		// at each of the three places the limit is applied.
		maxEntry = math.MaxInt64
	case maxEntry == 0:
		maxEntry = defaultMaxEntryBytes
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Cache{dir: dir, maxBytes: maxBytes, maxEntry: maxEntry, flight: map[string]*fill{}}, nil
}

// Dir is the directory the cache owns.
func (c *Cache) Dir() string { return c.dir }

// path derives a filename from the cache key by hashing it. The key is
// attachcache.Key(site, profile, issue, id). Hashing keeps a hostile key
// from escaping the directory.
// Tag is an opaque, stable identity for a cache key — the same hash the
// filename uses. A caller that needs a validator wants this, not the key
// itself: the key is `<site>\x1f<profile>\x1f<issue>\x1f<id>`, and putting
// that in an ETag published the Atlassian hostname and the workspace name
// in an HTTP response header, which a paired serve hands to anything on
// the tailnet that can reach it (GDK-1621).
func Tag(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])[:32]
}

// MaxEntry is the largest object this cache will keep, in bytes. A caller
// with a recorded size can ask before fetching whether an object could ever
// be cached — which is how a ranged request decides between filling the
// cache and streaming past it (GDK-1616).
func (c *Cache) MaxEntry() int64 { return c.maxEntry }

func (c *Cache) path(id string) string {
	sum := sha256.Sum256([]byte(id))
	name := hex.EncodeToString(sum[:])
	// One level of fan-out keeps directory listings small.
	return filepath.Join(c.dir, name[:2], name)
}

// Get returns an open reader for a cached attachment. The caller closes it.
// A miss returns os.ErrNotExist.
func (c *Cache) Get(id string) (io.ReadSeekCloser, Meta, error) {
	p := c.path(id)
	meta, err := readMeta(p + ".json")
	if err != nil {
		return nil, Meta{}, err
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, Meta{}, err
	}
	// Touch so eviction sees recent use. A failure here only costs accuracy.
	now := time.Now()
	_ = os.Chtimes(p, now, now)
	return f, meta, nil
}

// Has reports whether an attachment is cached, without opening it.
func (c *Cache) Has(id string) bool {
	if _, err := os.Stat(c.path(id)); err != nil {
		return false
	}
	_, err := readMeta(c.path(id) + ".json")
	return err == nil
}

// Fill is FillOrStream for callers with nowhere to stream to (the warm
// path): an entry too large to keep is reported as TooLarge and its body
// discarded.
func (c *Cache) Fill(id string, fetch func() (io.ReadCloser, Meta, error)) error {
	rc, _, err := c.FillOrStream(id, fetch)
	if rc != nil {
		rc.Close()
		return errTooLarge
	}
	return err
}

// FillOrStream is the single-flight write path, and it never asks a caller to
// fetch the same bytes twice (GDK-1616). fetch runs at most once per call —
// at most once per id when ten renders miss at the same moment — and the
// three outcomes are distinct:
//
//   - (nil, meta, nil): the bytes are on disk. Serve them with Get.
//   - (rc, meta, nil): too large to keep. rc is the whole object, exactly as
//     the origin sent it, including any bytes this call had already staged.
//     The caller streams it and closes it.
//   - (nil, _, err): the fetch or the write failed.
//
// The returned body is why a request can no longer fetch twice: the size
// verdict arrives with the bytes attached, so there is nothing left to go
// back to the origin for.
//
// fetch must return the body, its content type, and its length (0 if unknown).
func (c *Cache) FillOrStream(id string, fetch func() (io.ReadCloser, Meta, error)) (io.ReadCloser, Meta, error) {
	c.mu.Lock()
	if f, busy := c.flight[id]; busy {
		c.mu.Unlock()
		f.wg.Wait()
		switch {
		case f.err == nil:
			return nil, Meta{}, nil
		case TooLarge(f.err):
			// The owner threw the bytes away because they will not fit.
			// This caller still has to answer its request, so it fetches
			// once — the same single fetch the owner made, not a second
			// one on top of its own.
			body, meta, err := fetch()
			if err != nil {
				return nil, Meta{}, err
			}
			return body, meta, nil
		default:
			// %w keeps the owner's typed cause (auth) reaching every
			// waiter's errors.Is branches (GDK-1237).
			return nil, Meta{}, fmt.Errorf("attachcache: concurrent fill: %w", f.err)
		}
	}
	// Cached means no fetch — owned here, not by callers. Without this, a
	// caller arriving between a flight's completion and its own Has check
	// refetches the same id (GDK-177, caught as a CI flake in the
	// single-flight test).
	if c.Has(id) {
		c.mu.Unlock()
		return nil, Meta{}, nil
	}
	f := &fill{}
	f.wg.Add(1)
	c.flight[id] = f
	c.mu.Unlock()

	// f.err is assigned before the deferred Done, so waiters woken by Wait
	// always observe the outcome (WaitGroup gives the happens-before).
	defer func() {
		c.mu.Lock()
		delete(c.flight, id)
		c.mu.Unlock()
		f.wg.Done()
	}()
	rc, meta, err := c.fill(id, fetch)
	f.err = err
	if rc != nil {
		// A too-large entry is not a failed flight for the waiters — they
		// must not inherit a body this caller owns, so they are told
		// TooLarge and fetch for themselves.
		f.err = errTooLarge
		return rc, meta, nil
	}
	return nil, meta, err
}

// fill is FillOrStream's owner half: fetch, stage, rename. Split out so the
// single-flight bookkeeping above can record its error for the waiters.
//
// A non-nil first return means "not cached, here are the bytes": the caller
// owns and closes it. It is never returned together with an error.
func (c *Cache) fill(id string, fetch func() (io.ReadCloser, Meta, error)) (io.ReadCloser, Meta, error) {
	body, meta, err := fetch()
	if err != nil {
		return nil, Meta{}, err
	}
	if meta.Size > c.maxEntry {
		// Known too large before a byte is read: hand the untouched body
		// back rather than making the caller ask the origin again.
		return body, meta, nil
	}
	// The body is closed here unless it is handed back to the caller: the
	// spillover path below returns a reader that still needs it open.
	handOff := false
	defer func() {
		if !handOff {
			body.Close()
		}
	}()

	p := c.path(id)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return nil, Meta{}, err
	}
	// Write to a temp file and rename so a crashed download never becomes a
	// half-cached image.
	tmp, err := os.CreateTemp(filepath.Dir(p), ".part-*")
	if err != nil {
		return nil, Meta{}, err
	}
	tmpName := tmp.Name()
	keepTmp := false
	defer func() {
		if keepTmp {
			return
		}
		tmp.Close()
		os.Remove(tmpName)
	}()

	written, err := io.Copy(tmp, io.LimitReader(body, c.maxEntry+1))
	if err != nil {
		return nil, Meta{}, err
	}
	if written > c.maxEntry {
		// The size the origin stated was wrong (or absent) and the entry
		// overran mid-copy. The bytes already read are in tmp, so the
		// caller gets tmp followed by the rest of the body — one fetch,
		// whole object, nothing to ask the origin for again.
		if _, err := tmp.Seek(0, io.SeekStart); err != nil {
			return nil, Meta{}, err
		}
		keepTmp, handOff = true, true
		return &spilloverBody{tmp: tmp, name: tmpName, r: io.MultiReader(tmp, body), body: body}, meta, nil
	}
	if err := tmp.Close(); err != nil {
		return nil, Meta{}, err
	}
	meta.Size = written
	if err := os.Rename(tmpName, p); err != nil {
		return nil, Meta{}, err
	}
	if err := writeMeta(p+".json", meta); err != nil {
		os.Remove(p)
		return nil, Meta{}, err
	}
	c.evict()
	return nil, meta, nil
}

// spilloverBody is the staged prefix plus the unread remainder of one fetch,
// presented as a single body. Closing it closes the origin body and removes
// the staging file.
type spilloverBody struct {
	tmp  *os.File
	name string
	r    io.Reader
	body io.ReadCloser
}

func (s *spilloverBody) Read(p []byte) (int, error) { return s.r.Read(p) }

func (s *spilloverBody) Close() error {
	err := s.body.Close()
	s.tmp.Close()
	os.Remove(s.name)
	return err
}

// errTooLarge is not exported: callers only need to know the fill failed, and
// the server falls back to streaming straight through.
var errTooLarge = errors.New("attachcache: entry exceeds the per-file limit")

// TooLarge reports whether err means the entry was skipped for its size.
func TooLarge(err error) bool { return errors.Is(err, errTooLarge) }

// evict drops least-recently-used entries until the cache fits its budget.
// Cheap because it only runs after a successful fill.
func (c *Cache) evict() {
	type entry struct {
		path string
		mod  time.Time
		size int64
	}
	var entries []entry
	var total int64
	err := filepath.WalkDir(c.dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasSuffix(p, ".json") || strings.Contains(d.Name(), ".part-") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		entries = append(entries, entry{p, info.ModTime(), info.Size()})
		total += info.Size()
		return nil
	})
	if err != nil || total <= c.maxBytes {
		return
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].mod.Before(entries[j].mod) })
	for _, e := range entries {
		if total <= c.maxBytes {
			return
		}
		if os.Remove(e.path) == nil {
			os.Remove(e.path + ".json")
			total -= e.size
		}
	}
}

// Stats reports the cache footprint, for the settings panel and `gadak status`.
func (c *Cache) Stats() (files int, bytes int64) {
	_ = filepath.WalkDir(c.dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasSuffix(p, ".json") {
			return nil
		}
		if info, err := d.Info(); err == nil {
			files++
			bytes += info.Size()
		}
		return nil
	})
	return files, bytes
}

func readMeta(p string) (Meta, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return Meta{}, err
	}
	var m Meta
	if err := json.Unmarshal(b, &m); err != nil {
		return Meta{}, err
	}
	return m, nil
}

func writeMeta(p string, m Meta) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o600)
}

// ImportFile seeds an entry from a local file under the given cache key.
// Callers that know site/profile/issue must pass Key(...); do not pass a raw
// id when the site is set — that is the snapshot-import miss that D9 closed.
func (c *Cache) ImportFile(key, path, contentType, filename string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	return c.Fill(key, func() (io.ReadCloser, Meta, error) {
		return io.NopCloser(f), Meta{ContentType: contentType, Size: info.Size(), Filename: filename}, nil
	})
}

// ImportStats is what a snapshot import reports: how many files landed, and
// which manifest ids were skipped (not in the mirror).
type ImportStats struct {
	Seeded     int
	SkippedIDs []string
}

// ImportManifest seeds the cache from a fixture directory's manifest.json.
// Each file is stored under Key(site, profile, issue, id). issueForID must
// return the issue key that owns id; ids it rejects are skipped — they are
// never written under the raw id (that would reopen the D9 cross-site hole).
func (c *Cache) ImportManifest(dir, site, profile string, issueForID func(id string) (issue string, ok bool)) (ImportStats, error) {
	var stats ImportStats
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return stats, nil
		}
		return stats, err
	}
	var manifest struct {
		Attachments []struct {
			ID          string `json:"id"`
			File        string `json:"file"`
			Filename    string `json:"filename"`
			ContentType string `json:"content_type"`
		} `json:"attachments"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return stats, err
	}
	if issueForID == nil {
		issueForID = func(string) (string, bool) { return "", false }
	}
	for _, a := range manifest.Attachments {
		if a.ID == "" {
			stats.SkippedIDs = append(stats.SkippedIDs, "(empty id)")
			continue
		}
		issue, ok := issueForID(a.ID)
		if !ok || issue == "" {
			stats.SkippedIDs = append(stats.SkippedIDs, a.ID)
			continue
		}
		ck := Key(site, profile, issue, a.ID)
		if err := c.ImportFile(ck, filepath.Join(dir, a.File), a.ContentType, a.Filename); err != nil {
			return stats, fmt.Errorf("import %s: %w", a.File, err)
		}
		stats.Seeded++
	}
	return stats, nil
}

// MissReason explains a cache miss so a log can tell a key-scope mismatch
// (legacy id-only file still on disk) from a true absence. It never serves
// the legacy entry.
func (c *Cache) MissReason(key, id string) string {
	if c.Has(key) {
		return "entry present"
	}
	if id != "" && key != id && c.Has(id) {
		return "legacy id-only entry present (key scope mismatch)"
	}
	return "no cached bytes"
}
