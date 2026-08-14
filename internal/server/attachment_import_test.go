package server

// Contract ↔ assertion (ATT-fix: snapshot import must use attachcache.Key):
//
//	C1  Site-set profile + manifest import → GET {key}/attachments/{id}/content/
//	    answers 200 from cache; fail-closed RoundTripper must not run.
//	    TestImportManifestServesFromCacheWithoutUpstream
//	C2  Empty site keeps the legacy id-only key (demo / export-static).
//	    TestImportManifestEmptySiteKeepsLegacyKey
//	C3  Manifest id absent from the mirror is skipped, not written under the raw id.
//	    TestImportManifestSkipsIDMissingFromMirror
//	C4  D9: a cached id is not readable under a foreign issue key (404, no bytes).
//	    TestImportManifestRejectsForeignIssueKey

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/midagedev/gadak/internal/attachcache"
	"github.com/midagedev/gadak/internal/config"
)

// failIfCalledTripper fails the test if the attachment path leaves this process.
type failIfCalledTripper struct {
	t    *testing.T
	hits *atomic.Int64
}

func (t *failIfCalledTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	t.hits.Add(1)
	t.t.Errorf("upstream called: %s %s", req.Method, req.URL)
	return nil, errors.New("upstream called")
}

func writeManifestDir(t *testing.T, id, filename, contentType, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	man := map[string]any{
		"attachments": []map[string]string{{
			"id": id, "file": filename, "filename": filename, "content_type": contentType,
		}},
	}
	raw, err := json.Marshal(man)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func ownerOf(issue string, known ...string) func(string) (string, bool) {
	okIDs := map[string]struct{}{}
	for _, id := range known {
		okIDs[id] = struct{}{}
	}
	return func(id string) (string, bool) {
		_, ok := okIDs[id]
		if !ok {
			return "", false
		}
		return issue, true
	}
}

func TestImportManifestServesFromCacheWithoutUpstream(t *testing.T) {
	db, cfg := fixture(t)
	config.SetProfile("")

	const (
		issue = "NMB-1"
		id    = "10021"
		body  = "IMPORTED-PNG"
	)
	cache, err := attachcache.New(t.TempDir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Site = "https://nimbus.example.com"
	dir := writeManifestDir(t, id, "shot.png", "image/png", body)
	stats, err := cache.ImportManifest(dir, cfg.Site, config.Profile(), ownerOf(issue, id))
	if err != nil {
		t.Fatal(err)
	}
	if stats.Seeded != 1 || len(stats.SkippedIDs) != 0 {
		t.Fatalf("import stats %+v", stats)
	}
	// No double write: the raw id must not be a second key (D9).
	if cache.Has(id) {
		t.Fatal("import also wrote the raw id — that reopens cross-site reads")
	}
	if !cache.Has(attachcache.Key(cfg.Site, config.Profile(), issue, id)) {
		t.Fatal("import did not write the scoped key the reader will ask for")
	}

	var hits atomic.Int64
	saved := proxyClient
	proxyClient = &http.Client{Transport: &failIfCalledTripper{t: t, hits: &hits}}
	t.Cleanup(func() { proxyClient = saved })

	cfg.Email = "dana@example.com"
	cfg.Token = "token"
	h := NewWithCache(db, cfg, cache)

	rec := get(t, h, apiBase+issue+"/attachments/"+id+"/content/", nil)
	if rec.Code != http.StatusOK || rec.Body.String() != body {
		t.Fatalf("imported attachment → %d %q (upstream called=%d); want 200 from cache (cache miss / upstream called)",
			rec.Code, rec.Body.String(), hits.Load())
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("upstream called %d times — imported bytes must be served from cache", n)
	}
}

func TestImportManifestEmptySiteKeepsLegacyKey(t *testing.T) {
	db, _ := fixture(t)
	config.SetProfile("")

	const (
		issue = "NMB-1"
		id    = "10021"
		body  = "DEMO-PNG"
	)
	cache, err := attachcache.New(t.TempDir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	dir := writeManifestDir(t, id, "shot.png", "image/png", body)
	if _, err := cache.ImportManifest(dir, "", "", ownerOf(issue, id)); err != nil {
		t.Fatal(err)
	}

	if !cache.Has(id) {
		t.Fatal("empty-site import must keep the legacy id-only key")
	}
	if cache.Has(attachcache.Key("https://nimbus.example.com", "", issue, id)) {
		t.Fatal("empty-site import wrote a scoped key")
	}

	// Demo has no credential and no site; the cached id-only entry must still
	// render. A miss would 409, which is how a scoped-key write would show up.
	h := NewWithCache(db, &config.Config{}, cache)
	rec := get(t, h, apiBase+issue+"/attachments/"+id+"/content/", nil)
	if rec.Code != http.StatusOK || rec.Body.String() != body {
		t.Fatalf("demo/legacy view → %d %q, want cached bytes", rec.Code, rec.Body.String())
	}
}

func TestImportManifestSkipsIDMissingFromMirror(t *testing.T) {
	cache, err := attachcache.New(t.TempDir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	dir := writeManifestDir(t, "99999", "ghost.png", "image/png", "GHOST")
	stats, err := cache.ImportManifest(dir, "https://nimbus.example.com", "", func(string) (string, bool) {
		return "", false
	})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Seeded != 0 || len(stats.SkippedIDs) != 1 || stats.SkippedIDs[0] != "99999" {
		t.Fatalf("stats %+v, want skipped 99999", stats)
	}
	if cache.Has("99999") {
		t.Fatal("imported an id that is not in the mirror (must skip, not write id-only)")
	}
	if cache.Has(attachcache.Key("https://nimbus.example.com", "", "NMB-1", "99999")) {
		t.Fatal("wrote a scoped key for an id that is not in the mirror")
	}
}

func TestImportManifestRejectsForeignIssueKey(t *testing.T) {
	db, cfg := fixture(t)
	config.SetProfile("")

	const (
		owner = "NMB-1"
		id    = "10021"
		body  = "OWNED-PNG"
	)
	cache, err := attachcache.New(t.TempDir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Site = "https://nimbus.example.com"
	dir := writeManifestDir(t, id, "shot.png", "image/png", body)
	if _, err := cache.ImportManifest(dir, cfg.Site, config.Profile(), ownerOf(owner, id)); err != nil {
		t.Fatal(err)
	}

	var hits atomic.Int64
	saved := proxyClient
	proxyClient = &http.Client{Transport: &failIfCalledTripper{t: t, hits: &hits}}
	t.Cleanup(func() { proxyClient = saved })

	cfg.Email = "dana@example.com"
	cfg.Token = "token"
	h := NewWithCache(db, cfg, cache)

	foreign := get(t, h, apiBase+"NMB-2/attachments/"+id+"/content/", nil)
	if foreign.Code != http.StatusNotFound {
		t.Fatalf("foreign issue key → %d %q, want 404", foreign.Code, foreign.Body.String())
	}
	if strings.Contains(foreign.Body.String(), body) {
		t.Fatal("served cached bytes for an issue that does not own the attachment")
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("foreign key hit upstream %d times; membership must reject first", n)
	}
}

func TestAttachmentMissReasonDistinguishesScopeMismatch(t *testing.T) {
	cache, err := attachcache.New(t.TempDir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	dir := writeManifestDir(t, "10021", "shot.png", "image/png", "X")
	// Simulate a leftover pre-D9 file: raw id on disk, reader asks for a scoped key.
	if err := cache.ImportFile("10021", filepath.Join(dir, "shot.png"), "image/png", "shot.png"); err != nil {
		t.Fatal(err)
	}
	scoped := attachcache.Key("https://nimbus.example.com", "", "NMB-1", "10021")
	if got := cache.MissReason(scoped, "10021"); got != "legacy id-only entry present (key scope mismatch)" {
		t.Fatalf("MissReason = %q", got)
	}
	if got := cache.MissReason(scoped, "nope"); got != "no cached bytes" {
		t.Fatalf("true miss = %q", got)
	}
}
