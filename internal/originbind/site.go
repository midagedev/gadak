package originbind

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/midagedev/gadak/internal/config"
)

// SiteBoundError is returned when a connected workspace is asked to bind a
// different site. Issue keys are not globally unique; changing origin means
// a new workspace (package comment). Error() is the CLI/HTTP sentence.
type SiteBoundError struct {
	Bound string
}

func (e *SiteBoundError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("this workspace is bound to %s — create a new workspace: gadak --workspace <name> init", e.Bound)
}

// CanonicalSite is the compare key for site rebind: scheme + host, no path,
// no trailing slash. Scheme-less input is treated as https, matching
// internal/server.normalizeSite.
func CanonicalSite(raw string) string {
	v := strings.TrimRight(strings.TrimSpace(raw), "/")
	if v == "" {
		return ""
	}
	if !strings.Contains(v, "://") {
		v = "https://" + v
	}
	u, err := url.Parse(v)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// RefuseSiteRebind stops a connected init / onboarding connect from silently
// re-pointing a workspace at a different site. Empty cfg.Site is first bind
// (or standalone/paired with no site) and is allowed. Same site after
// CanonicalSite is token rotation and is allowed.
func RefuseSiteRebind(cfg *config.Config, newSite string) error {
	if cfg == nil {
		return nil
	}
	bound := CanonicalSite(cfg.Site)
	if bound == "" {
		return nil
	}
	next := CanonicalSite(newSite)
	if next == "" {
		return nil
	}
	if bound != next {
		return &SiteBoundError{Bound: bound}
	}
	return nil
}
