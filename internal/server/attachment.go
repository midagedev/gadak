package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/midagedev/gadak/internal/attachcache"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/linear"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/store"
)

/* ── attachment bytes: local cache first, Jira only on a miss ── */

// ponytail: a 2-minute ceiling on one attachment download, no resumption.
// CheckRedirect refuses hops off uploads.linear.app (GDK-558): Go's default
// policy keeps Authorization on a subdomain redirect, which would send the
// Linear API key to *.uploads.linear.app. Jira fetches start on the site
// host, so their media redirects are unchanged.
var proxyClient = &http.Client{
	Timeout:       2 * time.Minute,
	CheckRedirect: proxyCheckRedirect,
}

// handleAttachment serves attachment bytes from the on-disk cache, falling back
// to Jira on a miss and caching what it fetches. Bytes for an attachment id are
// immutable in Jira, so a hit is served with a long-lived validator and a cached
// attachment keeps working with no credential at all — which is how the bundled
// demo snapshot shows real images offline.
func (s *server) handleAttachment(w http.ResponseWriter, r *http.Request) {
	issueKey := r.PathValue("key")
	id := r.PathValue("id")
	// Membership first: a cached id must not be readable under another issue
	// key, and a site switch cannot serve leftover bytes for an issue the new
	// mirror does not own.
	if !s.attachmentBelongs(r.Context(), issueKey, id) {
		fail(w, http.StatusNotFound, "not_found")
		return
	}
	ck := s.attachmentCacheKey(issueKey, id)
	if s.cache != nil {
		if served := s.serveCached(w, r, ck); served {
			return
		}
	}

	cfg := s.config()
	sourceID, _, originErr := s.db.AttachmentOrigin(r.Context(), issueKey, id)
	linear := originErr == nil && sourceID == "linear"
	if !linear && !cfg.HasCredential() {
		fail(w, http.StatusConflict, "credential_required")
		return
	}

	// One request, one fetch (GDK-1616). The counter is the structure, not a
	// comment: every path below goes through this closure, so a second call
	// in the same request cannot reach the origin at all — it errors. The
	// double fetch it closes was real: the cache filled, refused the entry
	// for its size, and the handler fell through and fetched the same large
	// video again to stream it. In-process that is waste; on a paired serve
	// it is the file crossing the tailnet twice for one <video>.
	fetches := 0
	fetchOnce := func(hdr http.Header) (*http.Response, error) {
		fetches++
		if fetches > 1 {
			return nil, errDoubleFetch
		}
		return s.fetchAttachment(r.Context(), cfg, issueKey, id, hdr)
	}

	// A partial or conditional request still fills the cache when the object
	// could fit it, and only streams past the cache when it could not.
	//
	// Skipping the cache for every ranged request was the shape this round
	// first took, and it was wrong in the common case: a <video> opens with
	// `Range: bytes=0-` on Safari, so the first play of a cacheable file
	// would never cache — and if the warm pass (warmAttachments, detached and
	// capped at eight per open) was still filling that same file, the request
	// fetched it a second time in parallel. On a paired serve that is the
	// file crossing the network twice for one play. Filling first costs the
	// whole object once, and http.ServeContent then answers this range, and
	// every later seek, off the local file.
	//
	// The size is the origin's claim from the mirror, and the only thing it
	// is trusted for is this either/or: a file the cache could never keep is
	// streamed with the browser's Range passed through, which is the one
	// shape that cannot be served locally at all.
	pass := rangeHeaders(r)
	oversize := false
	if s.cache != nil && pass != nil {
		if cap := s.cache.MaxEntry(); cap > 0 {
			if size, err := s.db.AttachmentSize(r.Context(), issueKey, id); err == nil && size > cap {
				oversize = true
			}
		}
	}
	if s.cache != nil && !(pass != nil && oversize) {
		log.Printf("server: attachment cache miss id=%s issue=%s: %s", id, issueKey, s.cache.MissReason(ck, id))
		var upstream http.Header
		body, meta, err := s.cache.FillOrStream(ck, func() (io.ReadCloser, attachcache.Meta, error) {
			res, err := fetchOnce(nil)
			if err != nil {
				return nil, attachcache.Meta{}, err
			}
			upstream = res.Header
			return res.Body, attachcache.Meta{
				ContentType: contentTypeOf(res),
				Size:        res.ContentLength,
			}, nil
		})
		switch {
		case err == nil && body == nil:
			// On disk now: every later view is local, and a Range — this
			// request's included — is answered by http.ServeContent off the
			// file rather than by the origin.
			if s.serveCached(w, r, ck) {
				return
			}
			// The bytes were written and then could not be read back. This
			// request has spent its one fetch, and inventing a second is
			// the bug this round closed — say so instead.
			log.Printf("server: attachment cached but unreadable id=%s issue=%s", id, issueKey)
			fail(w, http.StatusBadGateway, "attachment_unavailable")
			return
		case err == nil:
			// Too large to keep, and the bytes came back with the verdict.
			// Logged with the size: this is the one request shape that
			// re-reads the origin on every view, so it should be visible
			// when someone asks why a large video is slow twice.
			// A ranged request that got here asked for part of a file the
			// mirror said would fit and the origin then overran. The whole
			// object is in hand and the fetch is spent, so it is answered
			// whole: ignoring Range is a 200, which is allowed, and the next
			// request takes the pass-through path above once the size is
			// recorded.
			log.Printf("server: attachment too large to cache, streamed once id=%s issue=%s bytes=%d ranged=%v", id, issueKey, meta.Size, pass != nil)
			defer body.Close()
			ct := meta.ContentType
			if ct == "" {
				ct = "application/octet-stream"
			}
			w.Header().Set("Content-Type", ct)
			// The origin's own validator and range advertisement, so the
			// browser's next request can seek and revalidate.
			for _, h := range []string{"Accept-Ranges", "ETag"} {
				if v := upstream.Get(h); v != "" {
					w.Header().Set(h, v)
				}
			}
			if meta.Size > 0 {
				w.Header().Set("Content-Length", strconv.FormatInt(meta.Size, 10))
			}
			w.Header().Set("Cache-Control", "private, max-age=300")
			setAttachmentGuards(w, ct)
			if _, err := io.Copy(w, body); err != nil {
				log.Printf("server: attachment stream: %v", err)
			}
			return
		case errors.Is(err, errAttachmentAuth):
			fail(w, http.StatusConflict, "credential_rejected")
			return
		case errors.Is(err, errAttachmentMissing):
			fail(w, http.StatusNotFound, "not_found")
			return
		default:
			var denied *originDeniedError
			if errors.As(err, &denied) {
				writeOriginDenied(w, denied)
				return
			}
			log.Printf("server: attachment cache fill: %v", err)
			fail(w, http.StatusBadGateway, "attachment_unavailable")
			return
		}
	}

	// No cache at all, or a ranged request for a file too large to cache:
	// stream from the origin and pass the browser's Range through so seeking
	// in a video works here too (GDK-1617).
	res, err := fetchOnce(pass)
	switch {
	case errors.Is(err, errAttachmentAuth):
		fail(w, http.StatusConflict, "credential_rejected")
		return
	case errors.Is(err, errAttachmentMissing):
		fail(w, http.StatusNotFound, "not_found")
		return
	case err != nil:
		var denied *originDeniedError
		if errors.As(err, &denied) {
			writeOriginDenied(w, denied)
			return
		}
		log.Printf("server: attachment proxy: %v", err)
		fail(w, http.StatusBadGateway, "attachment_unavailable")
		return
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusNotModified {
		// 304 carries no body, and RFC 9110 says not to send
		// Content-Length or Content-Type with one.
		if et := res.Header.Get("ETag"); et != "" {
			w.Header().Set("ETag", et)
		}
		w.Header().Set("Cache-Control", "private, max-age=300")
		w.WriteHeader(http.StatusNotModified)
		return
	}
	ct := contentTypeOf(res)
	w.Header().Set("Content-Type", ct)
	for _, h := range []string{"Content-Length", "Content-Range", "Accept-Ranges", "ETag"} {
		if v := res.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	w.Header().Set("Cache-Control", "private, max-age=300")
	setAttachmentGuards(w, ct)
	if res.StatusCode != http.StatusOK {
		w.WriteHeader(res.StatusCode)
	}
	if _, err := io.Copy(w, res.Body); err != nil {
		log.Printf("server: attachment stream: %v", err)
	}
}

// rangeHeaders is the subset of a client's request the upstream may act on.
// Nothing else is forwarded: this is a proxy for bytes, not for the request.
func rangeHeaders(r *http.Request) http.Header {
	var out http.Header
	for _, h := range []string{"Range", "If-Range", "If-None-Match"} {
		if v := r.Header.Get(h); v != "" {
			if out == nil {
				out = http.Header{}
			}
			out.Set(h, v)
		}
	}
	return out
}

// serveCached answers from disk. Reports whether it wrote a response.
func (s *server) serveCached(w http.ResponseWriter, r *http.Request, id string) bool {
	f, meta, err := s.cache.Get(id)
	if err != nil {
		return false
	}
	defer f.Close()
	w.Header().Set("Content-Type", meta.ContentType)
	// The bytes behind an attachment id never change, so the browser may keep
	// them for as long as it likes. This is what makes a second view instant.
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	// The cache key, not the tag, used to be the ETag — so every response
	// carried the site hostname and the workspace name (GDK-1621). An
	// ETag is an opaque identity; it does not need to say what it is made
	// of.
	w.Header().Set("ETag", fmt.Sprintf("%q", "att-"+attachcache.Tag(id)))
	setAttachmentGuards(w, meta.ContentType)
	http.ServeContent(w, r, "", time.Time{}, f)
	return true
}

// cacheStatus is what the client shows next to an attachment: "ready" once the
// bytes are local, "pending" while they still have to come from Jira.
func (s *server) cacheStatus(issueKey, id string) string {
	if s.cache == nil || !s.cache.Has(s.attachmentCacheKey(issueKey, id)) {
		return "pending"
	}
	return "ready"
}

// attachmentCacheKey is the on-disk identity: site + profile + issue + id.
func (s *server) attachmentCacheKey(issueKey, id string) string {
	return attachcache.Key(s.config().Site, s.profile, issueKey, id)
}

// attachmentBelongs reports whether the mirror lists id on issueKey. Used to
// refuse a cached (or upstream) fetch under a foreign issue key. One store
// query (not Detail): comments/history/links/page-refs are not membership.
func (s *server) attachmentBelongs(ctx context.Context, issueKey, id string) bool {
	if issueKey == "" || id == "" {
		return false
	}
	ok, err := s.db.AttachmentBelongs(ctx, issueKey, id)
	return err == nil && ok
}

// warmAttachments pre-downloads the inline-renderable attachments of an issue the
// user just opened, so the images are local before the browser asks for them.
// Bounded and fire-and-forget: a failure only means the proxy path handles it.
func (s *server) warmAttachments(cfg *config.Config, issueKey string, atts []detailAttachment) {
	if s.cache == nil || !cfg.HasCredential() {
		return
	}
	var pending []detailAttachment
	for _, a := range atts {
		if (a.IsImage || a.IsVideo) && !s.cache.Has(s.attachmentCacheKey(issueKey, a.ID)) {
			pending = append(pending, a)
		}
	}
	if len(pending) == 0 {
		return
	}
	// ponytail: at most four in flight and eight per open; raise it when someone
	// has an issue with dozens of screenshots and complains.
	const maxWarm, workers = 8, 4
	if len(pending) > maxWarm {
		pending = pending[:maxWarm]
	}
	jobs := make(chan detailAttachment)
	for i := 0; i < workers; i++ {
		go func() {
			for a := range jobs {
				// Detached from the request: the browser may have moved on already.
				id := a.ID
				ck := s.attachmentCacheKey(issueKey, id)
				log.Printf("server: attachment warm miss id=%s issue=%s: %s", id, issueKey, s.cache.MissReason(ck, id))
				if err := s.cache.Fill(ck, func() (io.ReadCloser, attachcache.Meta, error) {
					res, err := s.fetchAttachment(context.Background(), cfg, issueKey, id, nil)
					if err != nil {
						return nil, attachcache.Meta{}, err
					}
					return res.Body, attachcache.Meta{
						ContentType: contentTypeOf(res),
						Size:        res.ContentLength,
						Filename:    a.Filename,
					}, nil
				}); err != nil && !attachcache.TooLarge(err) {
					log.Printf("server: attachment warm %s: %v", id, err)
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, a := range pending {
			jobs <- a
		}
	}()
}

var (
	// errDoubleFetch is never expected: it is what a request gets if it asks
	// the origin for the same attachment twice (GDK-1616). A 502 with this in
	// the log is a regression report, not a user's problem.
	errDoubleFetch       = errors.New("attachment: refusing a second origin fetch for one request")
	errAttachmentAuth    = errors.New("attachment: credential rejected")
	errAttachmentMissing = errors.New("attachment: not found")
)

// originDeniedError is a Linear (or other non-Jira) 401/403: pass the status
// through instead of mapping it onto gadak's credential_rejected 409.
type originDeniedError struct {
	code int
	ct   string
	body io.ReadCloser
}

func (e *originDeniedError) Error() string {
	return fmt.Sprintf("attachment: origin status %d", e.code)
}

func writeOriginDenied(w http.ResponseWriter, e *originDeniedError) {
	if e.body != nil {
		defer e.body.Close()
	}
	if e.ct != "" {
		w.Header().Set("Content-Type", e.ct)
	}
	w.WriteHeader(e.code)
	if e.body != nil {
		_, _ = io.Copy(w, e.body)
	}
}

// fetchAttachment performs the one call that leaves this process.
func (s *server) fetchAttachment(ctx context.Context, cfg *config.Config, issueKey, id string, hdr http.Header) (*http.Response, error) {
	sourceID, contentURL, err := s.db.AttachmentOrigin(ctx, issueKey, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, errAttachmentMissing
		}
		return nil, err
	}
	if sourceID == "linear" {
		_ = hdr // Linear content URLs are pre-signed; Range is not passed on.
		if !isLinearUploadsURL(contentURL) {
			// GDK-560: do not fetch an arbitrary stored URL (SSRF).
			return nil, errAttachmentMissing
		}
		var key string
		if cfg != nil && cfg.Linear != nil {
			key = cfg.Linear.APIKey
		}
		return fetchStoredURL(ctx, contentURL, key)
	}
	// A built-in origin has no site — in-process because the origin is this
	// process, paired because the endpoint lives in remote-origin.json — so
	// concatenating cfg.Site produced a relative URL and every view answered
	// 502 (GDK-1613). origin.Client is the seam that answers on all of them;
	// the streaming path below stays for a connected site, where the bytes
	// really are remote and can be large.
	if cfg.OriginType() == config.OriginGadak {
		return s.fetchBuiltInAttachment(ctx, cfg, id, hdr)
	}
	// Jira Server states the bytes' address and serves them nowhere else,
	// so the stored URL is the request — reduced to a site-relative path,
	// which is what keeps the credential on this workspace's site
	// (GDK-1639). It goes through origin.Client for the same reason the
	// built-in path does: that client already holds the right credential
	// shape, and Server's is a bearer, not the basic auth below.
	if cfg.OriginType() == config.OriginJiraServer {
		return s.fetchServerAttachment(ctx, cfg, contentURL, hdr)
	}
	target := strings.TrimRight(cfg.Site, "/") + "/rest/api/3/attachment/content/" + url.PathEscape(id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	// Jira answers with a redirect to a pre-signed media URL. Go drops the
	// Authorization header on a cross-host redirect, which is exactly right: the
	// token must not travel to the media host.
	req.SetBasicAuth(cfg.Email, cfg.Token)
	for k, vs := range hdr {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	res, err := proxyClient.Do(req)
	if err != nil {
		return nil, err
	}
	return mapAttachmentStatus(res, false)
}

// isLinearUploadsURL is internal/linear's rule (GDK-1610: the CLI download
// needs the same one, so the host and the test live in that package). The
// alias stays because this file's redirect policy and the test read it.
func isLinearUploadsURL(target string) bool { return linear.IsUploadsURL(target) }

const linearUploadsHost = linear.UploadsHost

// proxyCheckRedirect is proxyClient's redirect policy. Linear downloads
// start on uploads.linear.app (GDK-560); a hop off that host is refused so
// Authorization cannot follow a subdomain redirect (GDK-558). Other origins
// (Jira site → media host) follow with the default 10-hop cap.
func proxyCheckRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	if len(via) == 0 || via[0].URL == nil {
		return nil
	}
	if via[0].URL.Host != linearUploadsHost {
		return nil
	}
	host := ""
	if req != nil && req.URL != nil {
		host = req.URL.Host
	}
	if host != linearUploadsHost {
		return fmt.Errorf("attachment: refusing redirect from %s to %s", linearUploadsHost, host)
	}
	return nil
}

// fetchStoredURL GETs an origin content URL. apiKey, when non-empty, is sent
// as a bare Authorization value (Linear rejects the "Bearer " prefix —
// internal/linear/client.go).
//
// Callers attach the Linear API key only for https://uploads.linear.app
// downloads (isLinearUploadsURL). Linear documents that file downloads from
// that host accept the API key in Authorization. That is a different path
// from the upload PUT to a signed URL, which must not carry the key
// (internal/linear/write.go UploadFile). Other Linear content URLs are
// refused before this function (GDK-560).
//
// proxyClient.CheckRedirect refuses any hop whose host is not exactly
// uploads.linear.app when the original host was that (GDK-558).
func fetchStoredURL(ctx context.Context, target, apiKey string) (*http.Response, error) {
	if target == "" {
		return nil, errAttachmentMissing
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", apiKey)
	}
	res, err := proxyClient.Do(req)
	if err != nil {
		return nil, err
	}
	return mapAttachmentStatus(res, true)
}

func mapAttachmentStatus(res *http.Response, passDenied bool) (*http.Response, error) {
	switch {
	case res.StatusCode == http.StatusUnauthorized, res.StatusCode == http.StatusForbidden:
		if passDenied {
			return nil, &originDeniedError{code: res.StatusCode, ct: res.Header.Get("Content-Type"), body: res.Body}
		}
		res.Body.Close()
		return nil, errAttachmentAuth
	case res.StatusCode == http.StatusNotFound:
		res.Body.Close()
		return nil, errAttachmentMissing
	case res.StatusCode == http.StatusNotModified:
		// The browser revalidated and the origin said the bytes are
		// unchanged. This proxy forwards If-None-Match now, so 304 is a
		// normal answer on the streaming path — and mapping it to an
		// error turned every second view of a large video into a 502
		// (GDK-1617). Not an error, and it has no body.
		return res, nil
	case res.StatusCode != http.StatusOK && res.StatusCode != http.StatusPartialContent:
		// 206 is a success too: it is what a Range request asks for.
		res.Body.Close()
		return nil, fmt.Errorf("attachment: upstream status %d", res.StatusCode)
	}
	return res, nil
}

func contentTypeOf(res *http.Response) string {
	if ct := res.Header.Get("Content-Type"); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

// setAttachmentGuards keeps attacker-controlled bytes from executing on this
// origin: nothing scriptable renders inline.
func setAttachmentGuards(w http.ResponseWriter, contentType string) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if !inlineSafe(contentType) {
		w.Header().Set("Content-Disposition", "attachment")
	}
}

// inlineSafe reports whether a type may render in the page. SVG is an image that
// executes script, so it is deliberately excluded.
func inlineSafe(contentType string) bool {
	mime := strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0])
	if mime == "image/svg+xml" {
		return false
	}
	return strings.HasPrefix(mime, "image/") || strings.HasPrefix(mime, "video/") ||
		strings.HasPrefix(mime, "audio/") || mime == "application/pdf"
}

// fetchServerAttachment streams a Jira Server attachment from the URL the
// origin stated (GDK-1639). hdr is passed through: the measured instance
// answers Range on this route, so seeking in a video works the same way it
// does on the other origins.
func (s *server) fetchServerAttachment(ctx context.Context, cfg *config.Config, contentURL string, hdr http.Header) (*http.Response, error) {
	c, err := origin.Client(cfg)
	if err != nil {
		return nil, err
	}
	path, err := origin.SiteRelative(c.BaseURL(), contentURL)
	if err != nil {
		return nil, err
	}
	res, err := c.Stream(ctx, http.MethodGet, path, hdr)
	if err != nil {
		return nil, err
	}
	return mapAttachmentStatus(res, false)
}

// fetchBuiltInAttachment reads bytes from gadak's own tracker through
// origin.Client, which resolves to the in-process origin or the paired home
// serve without this file knowing which.
//
// Streamed, not buffered. The origin holds files a real workspace's size
// (measured: 22% of 19,076 attachments over 8 MiB, the largest 884 MiB) now
// that its bytes live on disk rather than in a BLOB (GDK-1617), and the
// origin labels them properly and answers Range — so hdr is passed through
// and the response is handed back exactly as it came, 206 included.
func (s *server) fetchBuiltInAttachment(ctx context.Context, cfg *config.Config, id string, hdr http.Header) (*http.Response, error) {
	c, err := origin.Client(cfg)
	if err != nil {
		return nil, err
	}
	res, err := c.Stream(ctx, http.MethodGet,
		"/rest/api/3/attachment/content/"+url.PathEscape(id), hdr)
	if err != nil {
		return nil, err
	}
	return mapAttachmentStatus(res, false)
}
