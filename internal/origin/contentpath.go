package origin

import (
	"fmt"
	"net/url"
	"strings"
)

// SiteRelative turns an origin-stated absolute URL into the site-relative
// path a jira.Client may request, and refuses one that points anywhere else
// (GDK-1639).
//
// Jira Server states an attachment's bytes as an absolute URL and serves
// them nowhere else. The transport refuses absolute URLs on purpose — the
// Authorization header must never leave the configured site (atlhttp
// resolveURL) — so the address has to be reduced to a path before it can be
// asked for, and the reduction is exactly where that guarantee would be
// lost if it were skipped.
//
// A URL on another scheme or host is refused rather than fetched
// credential-less: an origin that answers with someone else's address is
// either misconfigured or hostile, and neither is something to follow.
func SiteRelative(base, raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("this attachment has no content URL in the mirror — run `gadak sync`")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("attachment content URL is not a URL: %w", err)
	}
	if !u.IsAbs() {
		// Already a path. Keep it, but only in the rooted form the
		// transport accepts.
		if !strings.HasPrefix(raw, "/") {
			return "", fmt.Errorf("attachment content URL %q is neither absolute nor rooted", raw)
		}
		return raw, nil
	}
	b, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("site URL is not a URL: %w", err)
	}
	if u.Scheme != b.Scheme || !strings.EqualFold(u.Host, b.Host) {
		return "", fmt.Errorf("refusing to fetch attachment bytes from %q: it is not this workspace's site", u.Scheme+"://"+u.Host)
	}
	// Relative to the base, not to the host root: atlhttp concatenates the
	// path onto the configured base rather than resolving it, so a Server
	// deployed under a context path would otherwise be asked for
	// /jira/jira/secure/... — which Jira answers with its own HTML at 200,
	// the exact shape GDK-1644 refuses. Measured on 11.3.11.
	p := u.EscapedPath()
	if prefix := strings.TrimRight(b.EscapedPath(), "/"); prefix != "" {
		if !strings.HasPrefix(p, prefix+"/") && p != prefix {
			return "", fmt.Errorf("refusing to fetch attachment bytes from %q: it is outside this workspace's site path %q", p, prefix)
		}
		p = strings.TrimPrefix(p, prefix)
	}
	if u.RawQuery != "" {
		p += "?" + u.RawQuery
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p, nil
}
