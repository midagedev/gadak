package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/attachcache"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/store"
)

// linearAttTestKey stands where a Linear API key would. It is not key-shaped
// (no lin_api_ prefix) so scanners have no opinion about it. Same string as
// internal/linear/client_test.go.
const linearAttTestKey = "linear-test-key-not-a-real-secret"

// attachmentFixture wires a fake Jira that serves one attachment and counts hits,
// so a test can prove the second view never leaves the process.
func attachmentFixture(t *testing.T) (http.Handler, *attachcache.Cache, *atomic.Int64, *store.DB, string) {
	t.Helper()
	db, cfg := fixture(t)

	var hits atomic.Int64
	jira := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/attachment/content/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		hits.Add(1)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("PNGBYTES"))
	}))
	t.Cleanup(jira.Close)

	cfg.Site = jira.URL
	cfg.Email = "dana@example.com"
	cfg.Token = "token"

	cache, err := attachcache.New(t.TempDir(), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	return NewWithCache(db, cfg, cache), cache, &hits, db, jira.URL
}

func TestAttachmentIsFetchedOnceThenServedFromDisk(t *testing.T) {
	h, cache, hits, _, site := attachmentFixture(t)
	path := apiBase + "NMB-1/attachments/10021/content/"

	first := get(t, h, path, nil)
	if first.Code != http.StatusOK || first.Body.String() != "PNGBYTES" {
		t.Fatalf("first view → %d %q", first.Code, first.Body.String())
	}
	if !cache.Has(attachcache.Key(site, "", "NMB-1", "10021")) {
		t.Fatal("first view did not cache the bytes")
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("upstream hit %d times on the first view", n)
	}

	second := get(t, h, path, nil)
	if second.Code != http.StatusOK || second.Body.String() != "PNGBYTES" {
		t.Fatalf("second view → %d %q", second.Code, second.Body.String())
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("upstream hit %d times — the second view should be local", n)
	}
	// The bytes behind an id never change, so the browser is told to keep them.
	if cc := second.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Fatalf("Cache-Control = %q, want an immutable directive", cc)
	}
	if second.Header().Get("ETag") == "" {
		t.Fatal("no ETag on a cached response")
	}
}

// The point of caching: a mirror with cached bytes keeps showing images after the
// credential is gone, which is what makes the offline demo snapshot work.
func TestCachedAttachmentSurvivesCredentialRemoval(t *testing.T) {
	h, cache, _, db, site := attachmentFixture(t)
	path := apiBase + "NMB-1/attachments/10021/content/"
	if rec := get(t, h, path, nil); rec.Code != http.StatusOK {
		t.Fatalf("warm-up → %d", rec.Code)
	}

	// Same cache, token gone, site kept — matches DELETE credential/ (site stays).
	offline := NewWithCache(db, &config.Config{Site: site}, cache)
	rec := get(t, offline, path, nil)
	if rec.Code != http.StatusOK || rec.Body.String() != "PNGBYTES" {
		t.Fatalf("offline view → %d %q, want the cached bytes", rec.Code, rec.Body.String())
	}
}

func TestUncachedAttachmentWithoutCredentialAsksForOne(t *testing.T) {
	db, _ := fixture(t)
	cache, err := attachcache.New(t.TempDir(), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	h := NewWithCache(db, &config.Config{}, cache)
	rec := get(t, h, apiBase+"NMB-1/attachments/10021/content/", nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("→ %d, want 409 credential_required", rec.Code)
	}
}

// SVG executes script, so it must never be offered for inline rendering even
// when it is served from the cache.
// D9: same attachment id cached on site A must not be served after the
// profile's site becomes B (cache key includes site identity).
func TestAttachmentCacheMissesAfterSiteSwitch(t *testing.T) {
	db, cfg := fixture(t)
	cache, err := attachcache.New(t.TempDir(), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	siteA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("SITE-A"))
	}))
	t.Cleanup(siteA.Close)
	siteB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("SITE-B"))
	}))
	t.Cleanup(siteB.Close)

	cfgA := *cfg
	cfgA.Site = siteA.URL
	hA := NewWithCache(db, &cfgA, cache)
	path := apiBase + "NMB-1/attachments/10021/content/"
	first := get(t, hA, path, nil)
	if first.Code != http.StatusOK || first.Body.String() != "SITE-A" {
		t.Fatalf("site A → %d %q", first.Code, first.Body.String())
	}

	cfgB := *cfg
	cfgB.Site = siteB.URL
	hB := NewWithCache(db, &cfgB, cache)
	second := get(t, hB, path, nil)
	if second.Code != http.StatusOK {
		t.Fatalf("site B → %d %s", second.Code, second.Body.String())
	}
	if second.Body.String() == "SITE-A" {
		t.Fatal("site B served site A's cached bytes")
	}
	if second.Body.String() != "SITE-B" {
		t.Fatalf("site B → %q, want SITE-B (miss + refetch)", second.Body.String())
	}
}

// An id the issue does not list is 404 — same as a foreign issue key.
func TestAttachmentUnknownIdIsNotFound(t *testing.T) {
	h, _, _, _, _ := attachmentFixture(t)
	rec := get(t, h, apiBase+"NMB-1/attachments/99999/content/", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown id → %d %q, want 404", rec.Code, rec.Body.String())
	}
}

// The URL id is the Jira external id when that column is set. The store row id
// must not open the bytes (old Detail loop used ExternalID first).
func TestAttachmentStoreIdDoesNotBypassExternalID(t *testing.T) {
	h, _, _, _, _ := attachmentFixture(t)
	rec := get(t, h, apiBase+"NMB-1/attachments/jira:a-1/content/", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("store id while external_id is set → %d %q, want 404", rec.Code, rec.Body.String())
	}
}

// When external_id is empty, the content URL uses the store id (handleDetail).
func TestAttachmentEmptyExternalIDFallsBackToStoreID(t *testing.T) {
	db, _ := fixture(t)
	if _, err := db.UpsertIssues(context.Background(), store.Batch{
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "jira:9090", SourceID: "jira", ExternalID: "9090", Key: "NMB-90",
				Title:     "local-only attachment",
				CreatedAt: "2026-08-01T00:00:00.000Z", UpdatedAt: "2026-08-01T00:00:00.000Z",
			},
			Issue: store.Issue{ProjectKey: "NMB", IssueType: "Bug", StatusCategory: "new"},
			Attachments: []store.Attachment{{
				ID: "jira:local-att", Filename: "note.txt", MimeType: "text/plain",
			}},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	// No credential: belongs → 409, not listed → 404. Distinguishes membership
	// from the fetch path.
	h := New(db, &config.Config{})
	owned := get(t, h, apiBase+"NMB-90/attachments/jira:local-att/content/", nil)
	if owned.Code != http.StatusConflict {
		t.Fatalf("empty external_id via store id → %d, want 409 (belongs, no credential)", owned.Code)
	}
	foreign := get(t, h, apiBase+"NMB-90/attachments/10021/content/", nil)
	if foreign.Code != http.StatusNotFound {
		t.Fatalf("other id on NMB-90 → %d, want 404", foreign.Code)
	}
}

// D9: a cached attachment id must not be readable under a different issue key.
func TestAttachmentCacheRejectsForeignIssueKey(t *testing.T) {
	h, _, _, _, _ := attachmentFixture(t)
	warm := apiBase + "NMB-1/attachments/10021/content/"
	if rec := get(t, h, warm, nil); rec.Code != http.StatusOK || rec.Body.String() != "PNGBYTES" {
		t.Fatalf("warm → %d %q", rec.Code, rec.Body.String())
	}
	foreign := get(t, h, apiBase+"NMB-2/attachments/10021/content/", nil)
	if foreign.Code != http.StatusNotFound {
		t.Fatalf("foreign issue key → %d %q, want 404", foreign.Code, foreign.Body.String())
	}
	if foreign.Body.String() == "PNGBYTES" {
		t.Fatal("served cached bytes for an issue that does not own the attachment")
	}
}

func TestLinearAttachmentFetchesStoredURLWithoutAuth(t *testing.T) {
	db, _ := fixture(t)
	var auth string
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		if r.URL.Path != "/file.png" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("LINBYTES"))
	}))
	t.Cleanup(origin.Close)
	dest, err := url.Parse(origin.URL)
	if err != nil {
		t.Fatal(err)
	}
	saved := proxyClient
	proxyClient = &http.Client{Timeout: 2 * time.Minute, CheckRedirect: saved.CheckRedirect, Transport: rewriteLinearUploads{dest: dest}}
	t.Cleanup(func() { proxyClient = saved })
	seedLinearAttachment(t, db, "FIX-L1", "att-lin", "https://uploads.linear.app/file.png")
	// No Jira credential and no Linear key: uploads.linear.app still proxies,
	// and the request must not carry Authorization.
	h := New(db, &config.Config{})
	rec := get(t, h, apiBase+"FIX-L1/attachments/att-lin/content/", nil)
	if rec.Code != http.StatusOK || rec.Body.String() != "LINBYTES" {
		t.Fatalf("linear proxy → %d %q", rec.Code, rec.Body.String())
	}
	if auth != "" {
		t.Fatalf("Authorization = %q, want empty", auth)
	}
}

func TestLinearAttachmentPassesThrough401(t *testing.T) {
	db, _ := fixture(t)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("no"))
	}))
	t.Cleanup(origin.Close)
	dest, err := url.Parse(origin.URL)
	if err != nil {
		t.Fatal(err)
	}
	saved := proxyClient
	proxyClient = &http.Client{Timeout: 2 * time.Minute, CheckRedirect: saved.CheckRedirect, Transport: rewriteLinearUploads{dest: dest}}
	t.Cleanup(func() { proxyClient = saved })
	seedLinearAttachment(t, db, "FIX-L2", "att-401", "https://uploads.linear.app/x.png")
	h := New(db, &config.Config{})
	rec := get(t, h, apiBase+"FIX-L2/attachments/att-401/content/", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("linear 401 → %d, want 401 passed through (not 409)", rec.Code)
	}
}

func TestJiraAttachmentStillUsesSiteBasicAuth(t *testing.T) {
	h, _, hits, _, _ := attachmentFixture(t)
	rec := get(t, h, apiBase+"NMB-1/attachments/10021/content/", nil)
	if rec.Code != http.StatusOK || rec.Body.String() != "PNGBYTES" {
		t.Fatalf("jira proxy → %d %q", rec.Code, rec.Body.String())
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("jira upstream hits = %d, want 1", n)
	}
}

func TestIsLinearUploadsURL(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"https://uploads.linear.app/abc/file.png", true},
		{"https://uploads.linear.app/", true},
		{"https://uploads.linear.app", true},
		{"https://uploads.linear.app/abc?x=1", true},
		{"https://uploads.linear.app/abc#frag", true},
		{"HTTPS://uploads.linear.app/abc", true}, // Parse lowercases the scheme
		{"http://uploads.linear.app/abc", false},
		{"https://cdn.uploads.linear.app/abc", false},
		{"https://uploads.linear.app.evil.example/abc", false},
		{"https://linear.app/abc", false},
		{"https://uploads.linear.app:443/abc", false},
		{"https://uploads.linear.app:443", false},
		{"https://example.com/abc", false},
		{"https://127.0.0.1/abc", false},
		{"https://Uploads.Linear.App/abc", false},
		{"https://uploads.linear.app.", false},
		{"uploads.linear.app/abc", false},
		{"", false},
		{"not a url", false},
	}
	for _, tt := range tests {
		if got := isLinearUploadsURL(tt.in); got != tt.want {
			t.Errorf("isLinearUploadsURL(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func seedLinearAttachment(t *testing.T, db *store.DB, issueKey, attExtID, contentURL string) {
	t.Helper()
	if err := db.UpsertSource(context.Background(), store.Source{ID: "linear", Kind: "linear", BaseURL: "https://linear.app"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertIssues(context.Background(), store.Batch{
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "linear:" + issueKey, SourceID: "linear", ExternalID: issueKey, Key: issueKey,
				Title: "linear att", CreatedAt: "2026-08-01T00:00:00.000Z", UpdatedAt: "2026-08-01T00:00:00.000Z",
			},
			Issue: store.Issue{ProjectKey: "FIX", StatusCategory: "new"},
			Attachments: []store.Attachment{{
				ID: "linear:" + attExtID, ExternalID: attExtID, Filename: "file.png",
				MimeType: "image/png", URL: contentURL,
			}},
		}},
	}); err != nil {
		t.Fatal(err)
	}
}

// rewriteLinearUploads is a test RoundTripper: https://uploads.linear.app is
// rewritten onto dest (an httptest server) as http, so isLinearUploadsURL can
// be true without contacting Linear. It is not a production bypass.
type rewriteLinearUploads struct {
	dest *url.URL
}

func (t rewriteLinearUploads) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "https" || req.URL.Host != linearUploadsHost {
		return nil, fmt.Errorf("unexpected attachment request %s://%s", req.URL.Scheme, req.URL.Host)
	}
	clone := req.Clone(req.Context())
	u := *req.URL
	u.Scheme = t.dest.Scheme
	u.Host = t.dest.Host
	clone.URL = &u
	clone.Host = t.dest.Host
	return http.DefaultTransport.RoundTrip(clone)
}

func TestLinearUploadsAttachmentSendsBareAPIKey(t *testing.T) {
	db, _ := fixture(t)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get("Authorization")
		if got == "" || strings.HasPrefix(got, "Bearer ") || got != linearAttTestKey {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("no"))
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("LINAUTH"))
	}))
	t.Cleanup(origin.Close)
	dest, err := url.Parse(origin.URL)
	if err != nil {
		t.Fatal(err)
	}
	saved := proxyClient
	proxyClient = &http.Client{Timeout: 2 * time.Minute, Transport: rewriteLinearUploads{dest: dest}}
	t.Cleanup(func() { proxyClient = saved })

	seedLinearAttachment(t, db, "FIX-AUTH", "att-auth", "https://uploads.linear.app/file.png")
	h := New(db, &config.Config{Linear: &config.LinearConfig{APIKey: linearAttTestKey}})
	rec := get(t, h, apiBase+"FIX-AUTH/attachments/att-auth/content/", nil)
	if rec.Code != http.StatusOK || rec.Body.String() != "LINAUTH" {
		t.Fatalf("linear uploads proxy → %d %q, want 200 LINAUTH", rec.Code, rec.Body.String())
	}
}

func TestLinearAttachmentOtherHostIsNotFetched(t *testing.T) {
	// GDK-560: a Linear content URL that is not uploads.linear.app must not
	// be fetched (SSRF). Zero requests on the httptest server is the proof.
	db, _ := fixture(t)
	var hits int
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("LINBYTES"))
	}))
	t.Cleanup(origin.Close)
	seedLinearAttachment(t, db, "FIX-OTHER", "att-other", origin.URL+"/file.png")
	h := New(db, &config.Config{Linear: &config.LinearConfig{APIKey: linearAttTestKey}})
	rec := get(t, h, apiBase+"FIX-OTHER/attachments/att-other/content/", nil)
	if hits != 0 {
		t.Fatalf("other-host URL was fetched %d times, want 0", hits)
	}
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusBadGateway {
		t.Fatalf("other-host proxy → %d %q, want 404 or 502", rec.Code, rec.Body.String())
	}
	if rec.Body.String() == "LINBYTES" {
		t.Fatal("other-host body was served — the URL was fetched")
	}
}

func TestFetchStoredURLStripsAuthorizationOnCrossHostRedirect(t *testing.T) {
	var destAuth string
	var destHits int
	dest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		destHits++
		destAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("DEST"))
	}))
	t.Cleanup(dest.Close)
	const destHost = "gadak-linear-redirect-dest.test"
	var srcAuth string
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srcAuth = r.Header.Get("Authorization")
		// Two httptest servers share 127.0.0.1, and Go treats that as the same
		// hostname (port is ignored by shouldCopyHeaderOnRedirect). Redirect
		// through a different hostname so the default Client actually strips
		// Authorization; DialContext maps that name onto dest.
		http.Redirect(w, r, "http://"+destHost+"/file.png", http.StatusFound)
	}))
	t.Cleanup(src.Close)
	destURL, err := url.Parse(dest.URL)
	if err != nil {
		t.Fatal(err)
	}
	saved := proxyClient
	proxyClient = &http.Client{
		Timeout: 2 * time.Minute,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				host, _, splitErr := net.SplitHostPort(addr)
				if splitErr == nil && host == destHost {
					addr = destURL.Host
				}
				var d net.Dialer
				return d.DialContext(ctx, network, addr)
			},
		},
	}
	t.Cleanup(func() { proxyClient = saved })

	res, err := fetchStoredURL(context.Background(), src.URL+"/file.png", linearAttTestKey)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if srcAuth != linearAttTestKey {
		t.Fatalf("first hop Authorization length %d, want bare key (otherwise dest empty is vacuous)", len(srcAuth))
	}
	if destHits != 1 {
		t.Fatalf("redirect dest hits %d, want 1", destHits)
	}
	if destAuth != "" {
		t.Fatalf("redirect dest Authorization length %d, want empty (Go strips on non-subdomain redirect)", len(destAuth))
	}
}

func TestFetchStoredURLRefusesUploadsSubdomainRedirect(t *testing.T) {
	// GDK-558: Go keeps Authorization on a redirect to a subdomain of the
	// original host. A hop from uploads.linear.app to *.uploads.linear.app
	// must not be followed.
	var destHits int
	var destAuth string
	dest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		destHits++
		destAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("DEST"))
	}))
	t.Cleanup(dest.Close)
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://cdn.uploads.linear.app/file.png", http.StatusFound)
	}))
	t.Cleanup(src.Close)
	srcURL, err := url.Parse(src.URL)
	if err != nil {
		t.Fatal(err)
	}
	destURL, err := url.Parse(dest.URL)
	if err != nil {
		t.Fatal(err)
	}
	saved := proxyClient
	proxyClient = &http.Client{
		Timeout:       2 * time.Minute,
		CheckRedirect: saved.CheckRedirect,
		Transport: linearSubdomainRedirect{
			src: srcURL, dest: destURL,
		},
	}
	t.Cleanup(func() { proxyClient = saved })

	res, err := fetchStoredURL(context.Background(), "https://uploads.linear.app/file.png", linearAttTestKey)
	if res != nil {
		res.Body.Close()
	}
	if destHits != 0 {
		t.Fatalf("subdomain redirect dest hits %d auth_len %d, want 0 (key must not leave uploads.linear.app)", destHits, len(destAuth))
	}
	if err == nil {
		t.Fatal("subdomain redirect was followed")
	}
}

// linearSubdomainRedirect rewrites https://uploads.linear.app onto src and
// https://cdn.uploads.linear.app onto dest so the redirect policy can be
// exercised without contacting Linear.
type linearSubdomainRedirect struct {
	src, dest *url.URL
}

func (t linearSubdomainRedirect) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	u := *req.URL
	switch req.URL.Host {
	case linearUploadsHost:
		u.Scheme, u.Host = t.src.Scheme, t.src.Host
		clone.Host = t.src.Host
	case "cdn.uploads.linear.app":
		u.Scheme, u.Host = t.dest.Scheme, t.dest.Host
		clone.Host = t.dest.Host
	default:
		return nil, fmt.Errorf("unexpected attachment request %s://%s", req.URL.Scheme, req.URL.Host)
	}
	clone.URL = &u
	return http.DefaultTransport.RoundTrip(clone)
}

func TestCachedSvgIsForcedToDownload(t *testing.T) {
	db, cfg := fixture(t)
	jira := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		_, _ = w.Write([]byte("<svg/>"))
	}))
	t.Cleanup(jira.Close)
	cfg.Site, cfg.Email, cfg.Token = jira.URL, "dana@example.com", "token"
	cache, err := attachcache.New(t.TempDir(), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	h := NewWithCache(db, cfg, cache)
	path := apiBase + "NMB-1/attachments/10021/content/"
	_ = get(t, h, path, nil)
	rec := get(t, h, path, nil) // cached path
	if got := rec.Header().Get("Content-Disposition"); got != "attachment" {
		t.Fatalf("Content-Disposition = %q, want attachment", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q", got)
	}
}

// GDK-1621: the cache key was the ETag, and the key is
// `<site>\x1f<profile>\x1f<issue>\x1f<id>` — so every cached attachment
// response published the Atlassian hostname and the workspace name in an
// HTTP header. Measured in a browser against a real workspace. A paired
// serve hands those headers to anything on the tailnet that can reach it,
// and an ETag is an opaque identity: it does not need to say what it is
// made of.
func TestCachedAttachmentETagCarriesNoSiteOrWorkspace(t *testing.T) {
	key := attachcache.Key("https://nimbus.example.com", "work", "NMB-1", "42")
	tag := attachcache.Tag(key)
	for _, leak := range []string{"nimbus.example.com", "work", "NMB-1", "\x1f"} {
		if strings.Contains(tag, leak) {
			t.Errorf("the tag still carries %q: %s", leak, tag)
		}
	}
	if tag == "" || tag == attachcache.Tag(key+"x") {
		t.Errorf("the tag is not a stable, distinguishing identity: %q", tag)
	}
	if attachcache.Tag(key) != tag {
		t.Error("the tag is not stable across calls, so a browser can never revalidate")
	}
}

/* ── GDK-1897: the artifact route — an HTML attachment served inline ── */

// artifactHTML is the document the artifact tests upload and serve. It
// carries a script tag on purpose: the route exists to make exactly this
// safe, and a document with no script would not distinguish the policies.
const artifactHTML = `<!doctype html><html><body><h1>artifact</h1><script>document.title = "ok"</script></body></html>`

// assertArtifactHeaders pins the artifact route's whole response contract in
// one place, so the cached path, the no-cache stream and the oversize stream
// are all measured against the same list (GDK-1897). testRequest sets Host
// 127.0.0.1, which is what the CSP's vendor source is composed from.
func assertArtifactHeaders(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q, want the forced text/html; charset=utf-8", got)
	}
	csp := rec.Header().Get("Content-Security-Policy")
	if want := artifactCSP(vendorCSPSource("127.0.0.1", false)); csp != want {
		t.Errorf("Content-Security-Policy = %q, want %q", csp, want)
	}
	if !strings.Contains(csp, "sandbox allow-scripts") {
		t.Errorf("CSP lost the sandbox directive: %q", csp)
	}
	// [GDK-1898] One owner: the directive now comes from dashboardCSP
	// itself, and this route no longer appends its own copy — a second
	// sandbox token would be ignored by the browser, silently dropping the
	// grants this route depends on. Count the directive ("; sandbox "), not
	// the substring: allow-popups-to-escape-sandbox contains "sandbox", so a
	// substring count reads 2 even on this correct policy (measured red
	// against a single-directive CSP before the count was shaped this way).
	if n := strings.Count(csp, "; sandbox "); n != 1 {
		t.Errorf("CSP carries %d sandbox directives, want exactly 1: %q", n, csp)
	}
	if got := rec.Header().Get("Referrer-Policy"); got != "no-referrer" {
		t.Errorf("Referrer-Policy = %q, want no-referrer", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if got := rec.Header().Get("Content-Disposition"); got != "" {
		t.Errorf("Content-Disposition = %q, want none — inline is the point of the route", got)
	}
}

// artifactFixture seeds NMB-77 with one html attachment (3001) and one png
// (3002) over a fake Jira that serves each id's bytes. The html bytes are
// deliberately labeled application/octet-stream upstream: the contract is
// that the mirror's mime row gates the route and the header is forced, so a
// passthrough implementation fails both the gate and the header assertion.
func artifactFixture(t *testing.T) (http.Handler, *attachcache.Cache, *atomic.Int64) {
	t.Helper()
	db, cfg := fixture(t)
	if _, err := db.UpsertIssues(context.Background(), store.Batch{
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "jira:3000", SourceID: "jira", ExternalID: "3000", Key: "NMB-77",
				Title: "artifact probe", CreatedAt: "2026-09-01T00:00:00.000Z", UpdatedAt: "2026-09-01T00:00:00.000Z",
			},
			Issue: store.Issue{ProjectKey: "NMB", IssueType: "Bug", StatusCategory: "new"},
			Attachments: []store.Attachment{
				{ID: "jira:a-3001", ExternalID: "3001", Filename: "report.html", MimeType: "text/html",
					Size: int64(len(artifactHTML)), CreatedAt: "2026-09-01T00:00:00.000Z"},
				{ID: "jira:a-3002", ExternalID: "3002", Filename: "shot.png", MimeType: "image/png",
					Size: 9, CreatedAt: "2026-09-01T00:00:00.000Z"},
			},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	var hits atomic.Int64
	jira := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/attachment/content/3001"):
			hits.Add(1)
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte(artifactHTML))
		case strings.HasSuffix(r.URL.Path, "/attachment/content/3002"):
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("PNG-BYTES"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(jira.Close)
	cfg.Site, cfg.Email, cfg.Token = jira.URL, "dana@example.com", "token"
	cache, err := attachcache.New(t.TempDir(), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	return NewWithCache(db, cfg, cache), cache, &hits
}

// The artifact route serves an html attachment as a document: forced
// Content-Type, the frame contract's CSP plus the sandbox directive (which
// is what makes it opaque-origin even opened top-level), and no
// Content-Disposition. First view fills the cache (upstream labeled the
// bytes octet-stream — the mirror's mime row decided, the header is forced),
// second view is pure cache; both carry the identical header set and the
// origin is paid once.
func TestArtifactRouteServesHTMLUnderSandboxCSP(t *testing.T) {
	h, _, hits := artifactFixture(t)
	path := apiBase + "NMB-77/attachments/3001/artifact/"

	first := get(t, h, path, nil)
	if first.Code != http.StatusOK {
		t.Fatalf("first view → %d %s", first.Code, first.Body.String())
	}
	if first.Body.String() != artifactHTML {
		t.Fatalf("body = %q, want the uploaded document", first.Body.String())
	}
	assertArtifactHeaders(t, first)

	second := get(t, h, path, nil)
	if second.Code != http.StatusOK || second.Body.String() != artifactHTML {
		t.Fatalf("cached view → %d %q", second.Code, second.Body.String())
	}
	assertArtifactHeaders(t, second)
	if n := hits.Load(); n != 1 {
		t.Fatalf("upstream hit %d times for two views — the second must be local", n)
	}
}

// The no-cache stream (a serve without an attachment cache, or any path the
// cache cannot take) carries the same policy: this is the header site that
// would drift if the artifact branch lived only in serveCached.
func TestArtifactStreamsSamePolicyWithoutCache(t *testing.T) {
	h, hits := noCacheArtifactServer(t)
	rec := get(t, h, apiBase+"NMB-77/attachments/3001/artifact/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("streamed view → %d %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != artifactHTML {
		t.Fatalf("body = %q, want the document", rec.Body.String())
	}
	assertArtifactHeaders(t, rec)
	if n := hits.Load(); n != 1 {
		t.Fatalf("upstream hit %d times", n)
	}
}

// The oversize stream (a file the cache refuses for its per-entry cap) is
// the third header site — same list, byte for byte.
func TestArtifactStreamsSamePolicyWhenTooLargeForCache(t *testing.T) {
	h, hits := tinyCacheArtifactServer(t)
	rec := get(t, h, apiBase+"NMB-77/attachments/3001/artifact/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("oversize stream → %d %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != artifactHTML {
		t.Fatalf("body = %q, want the document", rec.Body.String())
	}
	assertArtifactHeaders(t, rec)
	if n := hits.Load(); n != 1 {
		t.Fatalf("upstream hit %d times", n)
	}
}

// noCacheArtifactServer is artifactFixture's byte path with no cache at all:
// every view is the bottom streaming branch.
func noCacheArtifactServer(t *testing.T) (http.Handler, *atomic.Int64) {
	db, cfg := seedArtifactIssue(t)
	hits := serveArtifactBytes(t, cfg)
	return New(db, cfg), hits
}

// tinyCacheArtifactServer caps one cache entry at 8 bytes, so the document
// (larger) is streamed past the cache on its only fetch.
func tinyCacheArtifactServer(t *testing.T) (http.Handler, *atomic.Int64) {
	db, cfg := seedArtifactIssue(t)
	hits := serveArtifactBytes(t, cfg)
	cache, err := attachcache.New(t.TempDir(), 0, 8)
	if err != nil {
		t.Fatal(err)
	}
	return NewWithCache(db, cfg, cache), hits
}

// seedArtifactIssue is artifactFixture's mirror half, split out so the
// cache-shape variants can share it.
func seedArtifactIssue(t *testing.T) (*store.DB, *config.Config) {
	t.Helper()
	db, cfg := fixture(t)
	if _, err := db.UpsertIssues(context.Background(), store.Batch{
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "jira:3000", SourceID: "jira", ExternalID: "3000", Key: "NMB-77",
				Title: "artifact probe", CreatedAt: "2026-09-01T00:00:00.000Z", UpdatedAt: "2026-09-01T00:00:00.000Z",
			},
			Issue: store.Issue{ProjectKey: "NMB", IssueType: "Bug", StatusCategory: "new"},
			Attachments: []store.Attachment{{
				ID: "jira:a-3001", ExternalID: "3001", Filename: "report.html", MimeType: "text/html",
				Size: int64(len(artifactHTML)), CreatedAt: "2026-09-01T00:00:00.000Z",
			}},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	return db, cfg
}

// serveArtifactBytes points cfg at a fake Jira serving the document (mislabeled
// octet-stream, as artifactFixture does) and returns the hit counter.
func serveArtifactBytes(t *testing.T, cfg *config.Config) *atomic.Int64 {
	t.Helper()
	var hits atomic.Int64
	jira := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/attachment/content/3001") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		hits.Add(1)
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte(artifactHTML))
	}))
	t.Cleanup(jira.Close)
	cfg.Site, cfg.Email, cfg.Token = jira.URL, "dana@example.com", "token"
	return &hits
}

// The gate: only the mirror's mime row opens this route. A png id is a 404,
// an html id under a foreign key is a 404 (the membership the byte route
// keeps), and an unknown id is a 404 — all before any byte or credential is
// spent, which is what the no-credential config here proves.
func TestArtifactRouteRefusesNonHTMLAndForeignKeys(t *testing.T) {
	db, cfg := seedArtifactIssue(t)
	serveArtifactBytes(t, cfg)
	h := New(db, &config.Config{}) // no credential: a fetch would 409, so a 200 here could only come from the gate failing
	for _, tc := range []struct{ name, path string }{
		{"png id", apiBase + "NMB-77/attachments/3002/artifact/"},
		{"html id under a foreign key", apiBase + "NMB-1/attachments/3001/artifact/"},
		{"unknown id", apiBase + "NMB-77/attachments/9999/artifact/"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := get(t, h, tc.path, nil)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("→ %d %s, want 404", rec.Code, rec.Body.String())
			}
		})
	}
}

// The content route did not move: the same html attachment that renders on
// the artifact route still answers as a download there — html is not in
// inlineSafe's set, and this pins that the new mode did not leak into the
// old route.
func TestContentRouteStillDownloadsHTML(t *testing.T) {
	h, _, _ := artifactFixture(t)
	rec := get(t, h, apiBase+"NMB-77/attachments/3001/content/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("content view → %d %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Disposition"); got != "attachment" {
		t.Fatalf("Content-Disposition = %q, want attachment", got)
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Fatalf("Cache-Control = %q, want the byte route's long-lived policy", cc)
	}
}

// The detail tells the client which attachments are artifacts: the html row
// is flagged with its artifact route, the png row is not, and both keep
// content_url exactly as before.
func TestDetailFlagsHTMLAttachmentsAsArtifacts(t *testing.T) {
	h, _, _ := artifactFixture(t)
	detail := decode[struct {
		Attachments []struct {
			ID          string `json:"id"`
			IsArtifact  bool   `json:"is_artifact"`
			ArtifactURL string `json:"artifact_url"`
			ContentURL  string `json:"content_url"`
		} `json:"attachments"`
	}](t, get(t, h, apiBase+"NMB-77/detail/", nil))
	byID := map[string]int{}
	for i, a := range detail.Attachments {
		byID[a.ID] = i
	}
	html, png := byID["3001"], byID["3002"]
	if _, ok := byID["3001"]; !ok {
		t.Fatalf("detail attachments %+v — no 3001", detail.Attachments)
	}
	a := detail.Attachments[html]
	if !a.IsArtifact {
		t.Error("html attachment is_artifact = false")
	}
	if want := apiBase + "NMB-77/attachments/3001/artifact/"; a.ArtifactURL != want {
		t.Errorf("artifact_url = %q, want %q", a.ArtifactURL, want)
	}
	if want := apiBase + "NMB-77/attachments/3001/content/"; a.ContentURL != want {
		t.Errorf("content_url = %q, want %q (unchanged)", a.ContentURL, want)
	}
	p := detail.Attachments[png]
	if p.IsArtifact {
		t.Error("png attachment is_artifact = true")
	}
	if p.ArtifactURL != "" {
		t.Errorf("png artifact_url = %q, want empty", p.ArtifactURL)
	}
}

// The page route is the same serving core with the pages/ owner: an html
// page attachment served from the cache carries the artifact policy, and a
// png page attachment is refused by the same mime gate. The bytes come from
// an imported manifest (the offline-demo path) so no wiki origin is needed —
// the cache is a legitimate byte source for this route, exactly as for the
// byte route.
func TestPageArtifactRouteServesFromCacheAndRefusesNonHTML(t *testing.T) {
	db, cfg := fixturePages(t)
	adf := json.RawMessage(`{"type":"doc","version":1,"content":[]}`)
	if _, err := db.UpsertPages(context.Background(), []store.PageRecord{{
		Item: store.Item{
			ID: "confluence:100", SourceID: "confluence", Kind: "page", ExternalID: "100",
			Key: "100", Title: "빌링 품질 회의록", BodyText: "빌링 품질 논의",
			Author: "Dana", AuthorID: "acc-dana", URL: "https://x/wiki/spaces/PROD/pages/100",
			CreatedAt: "2026-07-01T00:00:00.000Z", UpdatedAt: "2026-08-01T00:00:00.000Z",
		},
		Page: store.Page{SpaceKey: "PROD", Version: 3, Status: "current", BodyADF: adf},
		Attachments: []store.Attachment{
			{ID: "confluence:a-9", ExternalID: "att-9", Filename: "notes.html", MimeType: "text/html",
				Size: int64(len(artifactHTML)), CreatedAt: "2026-09-01T00:00:00.000Z"},
			{ID: "confluence:a-8", ExternalID: "att-8", Filename: "diagram.png", MimeType: "image/png",
				Size: 777, CreatedAt: "2026-09-01T00:00:00.000Z"},
		},
	}}); err != nil {
		t.Fatal(err)
	}
	cache, err := attachcache.New(t.TempDir(), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	dir := writeManifestDir(t, "att-9", "notes.html", "application/octet-stream", artifactHTML)
	if _, err := cache.ImportManifest(dir, cfg.Site, config.Profile(), ownerOf("pages/100", "att-9")); err != nil {
		t.Fatal(err)
	}
	h := NewWithCache(db, cfg, cache)

	rec := get(t, h, apiBase+"pages/100/attachments/att-9/artifact/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("page artifact → %d %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != artifactHTML {
		t.Fatalf("body = %q, want the imported document", rec.Body.String())
	}
	assertArtifactHeaders(t, rec)

	if rec := get(t, h, apiBase+"pages/100/attachments/att-8/artifact/", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("png page artifact → %d, want 404", rec.Code)
	}
}

// GDK-1897's measured premise: an .html upload through the real upload
// handler on a built-in origin. The origin's own upload declares the part's
// Content-Type from the filename (attachmentPartHeader, GDK-1617), and
// issuetap keeps what it is told — so the mirror row the re-read writes
// must name text/html, or every artifact uploaded to a built-in workspace
// would 404 on the route that exists for it. Measured 2026-09-15: the row
// comes back "text/html; charset=utf-8" — the TypeByExtension branch, no
// fallback needed in gadak's handler.
func TestBuiltInUploadHTMLAttachmentMirrorsTextHTML(t *testing.T) {
	h, cfg, db := builtInServerDB(t)
	ctx := context.Background()
	c, err := origin.Client(cfg)
	if err != nil {
		t.Fatal(err)
	}
	key, err := c.CreateIssue(ctx, map[string]any{
		"project":   map[string]any{"key": origin.DefaultProjectKey},
		"summary":   "artifact upload probe",
		"issuetype": map[string]any{"name": "Task"},
	})
	if err != nil {
		t.Fatalf("create on the built-in origin: %v", err)
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, _ := mw.CreateFormFile("file", "report.html")
	_, _ = part.Write([]byte(artifactHTML))
	_ = mw.Close()
	req := testRequest(http.MethodPost, apiBase+key+"/attachments/", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload → %d: %s", rec.Code, rec.Body.String())
	}
	up := decode[struct {
		Attachments []struct {
			ID          string `json:"id"`
			MimeType    string `json:"mime_type"`
			IsArtifact  bool   `json:"is_artifact"`
			ArtifactURL string `json:"artifact_url"`
		} `json:"attachments"`
	}](t, rec)
	if len(up.Attachments) != 1 || up.Attachments[0].ID == "" {
		t.Fatalf("attachments %+v", up.Attachments)
	}
	id := up.Attachments[0].ID
	// The origin's echo and the detail must agree with the mirror row below;
	// an echo that could not see a mime is allowed to say so (empty).
	if m := up.Attachments[0].MimeType; m != "" && !isHTMLMediaType(m) {
		t.Errorf("upload echo mime_type = %q", m)
	}
	if m := up.Attachments[0].MimeType; m != "" && !up.Attachments[0].IsArtifact {
		t.Errorf("echo says is_artifact=false for mime %q", m)
	}

	// The measurement itself: the mirror row the write-through re-read left.
	ro, err := db.ReadOnly()
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	var mirrored string
	if err := ro.QueryRow(`
		SELECT COALESCE(a.mime_type, '') FROM attachments a
		WHERE COALESCE(NULLIF(a.external_id, ''), a.id) = ?`, id).Scan(&mirrored); err != nil {
		t.Fatalf("mirror row for %s: %v", id, err)
	}
	t.Logf("measured mirror mime_type for the uploaded .html: %q", mirrored)
	if !isHTMLMediaType(mirrored) {
		t.Fatalf("mirror mime_type = %q — the artifact route would 404 every .html upload on a built-in origin", mirrored)
	}

	// End to end: what was just uploaded renders on the artifact route.
	got := get(t, h, apiBase+key+"/attachments/"+id+"/artifact/", nil)
	if got.Code != http.StatusOK {
		t.Fatalf("artifact route after upload → %d %s", got.Code, got.Body.String())
	}
	if got.Body.String() != artifactHTML {
		t.Fatalf("body = %q, want the uploaded document", got.Body.String())
	}
	assertArtifactHeaders(t, got)
}
