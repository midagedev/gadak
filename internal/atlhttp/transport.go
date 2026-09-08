// Package atlhttp is the shared HTTP transport for Atlassian Cloud clients
// (Jira, Confluence): path safety, Authorization host pinning, and the
// Do/DoRaw loop. Retryable statuses, backoff plus Retry-After, the error
// snippet, the 64 MiB body cap, and Usage/Meter live in httppolicy so a
// non-Atlassian connector can share that policy without importing this
// package.
//
// The token lives only in the Authorization header. It is never put in an
// error, a log line or a URL (constitution article 8), which is why DoRaw
// reports the method and path but never the request itself.
package atlhttp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/midagedev/gadak/internal/httppolicy"
)

// Config is the per-request transport configuration. Callers pass live client
// fields so tests can mutate HTTP/Retries/Backoff after construction.
type Config struct {
	// Base is the site (or wiki) origin with no trailing slash.
	Base string
	// Auth is the full Authorization header value (e.g. "Basic …").
	Auth string
	// HTTP is the client used for each attempt; nil is a programming error.
	HTTP *http.Client
	// Retries is the total number of attempts per request; Backoff is the first
	// wait, doubling per attempt and capped at 30 s.
	Retries int
	Backoff time.Duration
	// ErrPrefix labels resolve errors ("jira" → "jira: bad site URL").
	ErrPrefix string
	// Usage, when non-nil, records every attempt that left the process.
	Usage *Meter
}

// DoRaw is the single HTTP path for JSON call helpers and Raw: retries,
// backoff, and optional usage. Path must be site-relative (leading "/");
// absolute URLs and scheme-relative paths are rejected so the Authorization
// header never leaves the configured site. mutating selects the write retry
// policy (429/503 only).
//
// A completed HTTP response always returns err == nil with the status and body
// (including non-2xx). err is reserved for transport failures and bad paths.
// JSON call helpers use Do, which classifies 401/403 as ErrAuth; Raw stays here.
func DoRaw(ctx context.Context, cfg Config, method, path string, payload []byte, hasBody, mutating bool) (int, []byte, error) {
	fullURL, err := resolveURL(cfg.Base, cfg.ErrPrefix, path)
	if err != nil {
		return 0, nil, err
	}
	retries := httppolicy.IsRetryable
	if mutating {
		retries = httppolicy.IsRetryableWrite
	}
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, fullURL, bytes.NewReader(payload))
		if err != nil {
			return 0, nil, err
		}
		if cfg.Auth != "" {
			req.Header.Set("Authorization", cfg.Auth)
		}
		req.Header.Set("Accept", "application/json")
		if hasBody {
			req.Header.Set("Content-Type", "application/json")
		}
		res, err := cfg.HTTP.Do(req)
		// Count every attempt that left the process; retries each draw rate budget.
		cfg.Usage.NoteRequest()
		if err != nil {
			if attempt < cfg.Retries-1 && !mutating {
				if werr := httppolicy.Wait(ctx, cfg.Backoff, attempt, "", cfg.Usage); werr != nil {
					return 0, nil, werr
				}
				cfg.Usage.NoteRetry()
				continue
			}
			return 0, nil, fmt.Errorf("%s %s: %w", method, path, err)
		}
		data, readErr := io.ReadAll(io.LimitReader(res.Body, httppolicy.MaxBody))
		res.Body.Close()
		cfg.Usage.NoteStatus(res.StatusCode)
		if retries(res.StatusCode) && attempt < cfg.Retries-1 {
			if werr := httppolicy.Wait(ctx, cfg.Backoff, attempt, res.Header.Get("Retry-After"), cfg.Usage); werr != nil {
				return 0, nil, werr
			}
			cfg.Usage.NoteRetry()
			continue
		}
		if res.StatusCode >= 200 && res.StatusCode < 300 && readErr != nil {
			return 0, nil, fmt.Errorf("%s %s: %w", method, path, readErr)
		}
		if err := refuseHTML(res, data); err != nil {
			return 0, nil, fmt.Errorf("%s %s: %w", method, path, err)
		}
		return res.StatusCode, data, nil
	}
}

// resolveURL joins path onto the configured base and refuses anything that
// would send the Authorization header off-host (absolute URL, scheme-relative
// "//host", or a ResolveReference host change).
func resolveURL(baseStr, errPrefix, path string) (string, error) {
	if err := rejectAbsolutePath(path); err != nil {
		return "", err
	}
	base, err := url.Parse(baseStr)
	if err != nil {
		return "", fmt.Errorf("%s: bad site URL: %w", errPrefix, err)
	}
	// Concatenate like the original call() did. ResolveReference would replace
	// any base path (and is unnecessary for a site-relative path).
	full := baseStr + path
	resolved, err := url.Parse(full)
	if err != nil {
		return "", fmt.Errorf("%s: bad path %q: %w", errPrefix, path, err)
	}
	if resolved.Scheme != base.Scheme || !strings.EqualFold(resolved.Host, base.Host) {
		return "", fmt.Errorf("refusing request: resolved host %q is not the configured site", resolved.Host)
	}
	if resolved.User != nil {
		return "", fmt.Errorf("refusing request: userinfo in URL is not allowed")
	}
	return full, nil
}

// rejectAbsolutePath blocks paths that would re-target the request before
// url.ResolveReference (https://…, http://…, //host/…).
func rejectAbsolutePath(path string) error {
	if path == "" {
		return fmt.Errorf("path is required and must start with /")
	}
	lower := strings.ToLower(path)
	// Order: absolute / scheme-relative first so the error names the real risk
	// (token on a foreign host), then require a site-relative leading slash.
	if strings.HasPrefix(path, "//") ||
		strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") {
		return fmt.Errorf("absolute URLs are not allowed — pass a path starting with / so the request stays on your configured site")
	}
	if !strings.HasPrefix(path, "/") {
		return fmt.Errorf("path must start with / (got %q)", path)
	}
	return nil
}

// Snippet trims and truncates a response body for error messages.
func Snippet(b []byte) string { return httppolicy.Snippet(b) }

// Stream is DoRaw for a response nobody wants in memory: it returns the live
// *http.Response with its Body unread, so the caller can copy it straight to
// a client or a file. The caller closes the Body.
//
// It exists because DoRaw reads through io.LimitReader(res.Body,
// httppolicy.MaxBody) — right for a JSON document, wrong for attachment
// bytes, where the cap that used to be 64 MiB is now the origin's
// configurable upload limit and a real workspace's largest measured file is
// 884 MiB (GDK-1617).
//
// Only transport failures retry. A response that has already begun cannot be
// replayed without reading it, which is the thing this function exists not
// to do; hdr carries request headers the caller needs passed through (Range,
// If-None-Match), and every status, including non-2xx, comes back as a
// response with err == nil.
func Stream(ctx context.Context, cfg Config, method, path string, hdr http.Header) (*http.Response, error) {
	fullURL, err := resolveURL(cfg.Base, cfg.ErrPrefix, path)
	if err != nil {
		return nil, err
	}
	// http.Client.Timeout covers reading the body, not just getting the
	// response — so the client's 60 s budget became a ceiling on transfer
	// size the moment bodies could be a gigabyte: the connection dies
	// mid-body under an already-sent Content-Length, which is a short file
	// nobody was told about. Streaming has no whole-exchange deadline; the
	// caller's context is the deadline, which for a served request is the
	// browser going away and for the CLI is the user pressing ^C.
	hc := cfg.HTTP
	if hc != nil && hc.Timeout != 0 {
		cp := *hc
		cp.Timeout = 0
		hc = &cp
	}
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, fullURL, nil)
		if err != nil {
			return nil, err
		}
		if cfg.Auth != "" {
			req.Header.Set("Authorization", cfg.Auth)
		}
		for k, vs := range hdr {
			for _, v := range vs {
				req.Header.Add(k, v)
			}
		}
		res, err := hc.Do(req)
		cfg.Usage.NoteRequest()
		if err != nil {
			if attempt < cfg.Retries-1 {
				if werr := httppolicy.Wait(ctx, cfg.Backoff, attempt, "", cfg.Usage); werr != nil {
					return nil, werr
				}
				cfg.Usage.NoteRetry()
				continue
			}
			return nil, fmt.Errorf("%s %s: %w", method, path, err)
		}
		cfg.Usage.NoteStatus(res.StatusCode)
		return res, nil
	}
}

// ErrNotAPI is a response that is a web page rather than an API answer
// (GDK-1648). Every call through DoRaw asks for JSON; a 2xx of HTML means
// the request was answered by something else — in the measured case a
// login page, reached because Go follows redirects and Jira Server sends a
// 302 to /login.jsp for a route the credential cannot reach.
//
// Without this the page's bytes are the API's answer: `gadak api` printed
// the login page's HTML, and a JSON decode failed with "invalid character
// '<'", which names the symptom and not the cause.
var ErrNotAPI = errors.New("the origin answered with a web page, not the API — the request was probably redirected to a login page; check the credential and the base URL")

// refuseHTML rejects a successful response whose body is a web page. Only
// 2xx: an origin is free to render an error page for a 4xx or 5xx, and the
// status already says what happened there.
func refuseHTML(res *http.Response, body []byte) error {
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil
	}
	// A page is a body. A bodyless success carries whatever Content-Type
	// the server likes — Jira Server answers 204 (sprint add) and 201
	// (issueLink) with text/html and nothing after the headers — and there
	// is no page in it to mistake for an answer. The status and
	// Content-Length heuristics this used to key on missed the 201
	// (GDK-1655, GDK-1662, both found against a live Server); the body
	// itself is the one thing every case agrees on.
	if len(bytes.TrimSpace(body)) == 0 {
		return nil
	}
	ct, _, err := mime.ParseMediaType(res.Header.Get("Content-Type"))
	if err != nil || ct != "text/html" {
		return nil
	}
	return ErrNotAPI
}
