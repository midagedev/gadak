// Package atlhttp is the shared HTTP transport for Atlassian Cloud clients
// (Jira, Confluence): retries, backoff, path safety, and optional usage meters.
//
// The token lives only in the Authorization header. It is never put in an
// error, a log line or a URL (constitution article 8), which is why DoRaw
// reports the method and path but never the request itself.
package atlhttp

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
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
	retries := isRetryable
	if mutating {
		retries = func(code int) bool { return code == 429 || code == 503 }
	}
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, fullURL, bytes.NewReader(payload))
		if err != nil {
			return 0, nil, err
		}
		req.Header.Set("Authorization", cfg.Auth)
		req.Header.Set("Accept", "application/json")
		if hasBody {
			req.Header.Set("Content-Type", "application/json")
		}
		res, err := cfg.HTTP.Do(req)
		// Count every attempt that left the process; retries each draw rate budget.
		cfg.Usage.noteRequest()
		if err != nil {
			if attempt < cfg.Retries-1 && !mutating {
				if werr := wait(ctx, cfg, attempt, ""); werr != nil {
					return 0, nil, werr
				}
				cfg.Usage.noteRetry()
				continue
			}
			return 0, nil, fmt.Errorf("%s %s: %w", method, path, err)
		}
		data, readErr := io.ReadAll(io.LimitReader(res.Body, 64<<20))
		res.Body.Close()
		cfg.Usage.noteStatus(res.StatusCode)
		if retries(res.StatusCode) && attempt < cfg.Retries-1 {
			if werr := wait(ctx, cfg, attempt, res.Header.Get("Retry-After")); werr != nil {
				return 0, nil, werr
			}
			cfg.Usage.noteRetry()
			continue
		}
		if res.StatusCode >= 200 && res.StatusCode < 300 && readErr != nil {
			return 0, nil, fmt.Errorf("%s %s: %w", method, path, readErr)
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

func isRetryable(code int) bool {
	switch code {
	case 429, 500, 502, 503, 504:
		return true
	}
	return false
}

func wait(ctx context.Context, cfg Config, attempt int, retryAfter string) error {
	d := cfg.Backoff << attempt
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	if s, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && s > 0 {
		d = time.Duration(s) * time.Second
	}
	if d <= 0 {
		return ctx.Err()
	}
	start := time.Now()
	select {
	case <-ctx.Done():
		cfg.Usage.noteWait(time.Since(start))
		return ctx.Err()
	case <-time.After(d):
		cfg.Usage.noteWait(time.Since(start))
		return nil
	}
}

// Snippet trims and truncates a response body for error messages.
func Snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 400 {
		return s[:400] + "…"
	}
	return s
}
