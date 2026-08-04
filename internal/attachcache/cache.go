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
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// DefaultMaxBytes bounds the cache. Attachments are usually screenshots, so a
// few hundred megabytes holds a working set of thousands.
const DefaultMaxBytes int64 = 512 << 20

// maxEntryBytes skips files too large to be worth caching for a UI that only
// renders images, PDFs, and short clips inline.
const maxEntryBytes int64 = 64 << 20

// Meta is the sidecar recorded next to each cached file. Content-Type has to
// survive a restart, and guessing it back from bytes is worse than storing it.
type Meta struct {
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	Filename    string `json:"filename,omitempty"`
}

// Cache is a content-addressed store under one directory. Safe for concurrent
// use; a miss for the same id collapses into a single fetch.
type Cache struct {
	dir      string
	maxBytes int64

	mu     sync.Mutex
	flight map[string]*sync.WaitGroup
}

// New opens (and creates) a cache directory. maxBytes <= 0 means DefaultMaxBytes.
func New(dir string, maxBytes int64) (*Cache, error) {
	if dir == "" {
		return nil, errors.New("attachcache: empty directory")
	}
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Cache{dir: dir, maxBytes: maxBytes, flight: map[string]*sync.WaitGroup{}}, nil
}

// Dir is the directory the cache owns.
func (c *Cache) Dir() string { return c.dir }

// path derives a filename from the attachment id by hashing it. Jira ids are
// numeric today, but hashing keeps a hostile id from escaping the directory.
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

// Fill is the single-flight write path: fetch runs at most once per id even if
// ten renders miss at the same moment. It returns after the bytes are on disk.
//
// fetch must return the body, its content type, and its length (0 if unknown).
func (c *Cache) Fill(id string, fetch func() (io.ReadCloser, Meta, error)) error {
	c.mu.Lock()
	if wg, busy := c.flight[id]; busy {
		c.mu.Unlock()
		wg.Wait()
		if c.Has(id) {
			return nil
		}
		return errors.New("attachcache: concurrent fill failed")
	}
	wg := &sync.WaitGroup{}
	wg.Add(1)
	c.flight[id] = wg
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.flight, id)
		c.mu.Unlock()
		wg.Done()
	}()

	body, meta, err := fetch()
	if err != nil {
		return err
	}
	defer body.Close()
	if meta.Size > maxEntryBytes {
		return errTooLarge
	}

	p := c.path(id)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	// Write to a temp file and rename so a crashed download never becomes a
	// half-cached image.
	tmp, err := os.CreateTemp(filepath.Dir(p), ".part-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(tmpName)
	}()

	written, err := io.Copy(tmp, io.LimitReader(body, maxEntryBytes+1))
	if err != nil {
		return err
	}
	if written > maxEntryBytes {
		return errTooLarge
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	meta.Size = written
	if err := os.Rename(tmpName, p); err != nil {
		return err
	}
	if err := writeMeta(p+".json", meta); err != nil {
		os.Remove(p)
		return err
	}
	c.evict()
	return nil
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

// Stats reports the cache footprint, for the settings panel and `scry status`.
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

// ImportFile seeds an entry from a local file. It exists for fixtures: the
// bundled demo snapshot ships attachment bytes so `scry demo` shows real images
// with no Jira account, and a test can prime the cache without a fake server.
func (c *Cache) ImportFile(id, path, contentType, filename string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	return c.Fill(id, func() (io.ReadCloser, Meta, error) {
		return io.NopCloser(f), Meta{ContentType: contentType, Size: info.Size(), Filename: filename}, nil
	})
}
