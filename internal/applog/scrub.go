package applog

import (
	"regexp"

	"github.com/midagedev/gadak/internal/pairing"
)

// Patterns match scripts/scan-internal.sh / internal/secretscan. The
// pairing-offer matcher is extra: an offer is not a repo-scan shape, but
// the product rule is never to print one — not a prefix, not a length, not
// a hash.
var (
	atlassianRe = regexp.MustCompile(`ATATT[A-Za-z0-9+/=_-]{20,}|ATCTT[A-Za-z0-9+/=_-]{20,}`)
	basicRe     = regexp.MustCompile(`(?i)Authorization:\s*Basic\s+[A-Za-z0-9+/=_-]{8,}`)
	bearerRe    = regexp.MustCompile(`(?i)(Bearer\s+)[A-Za-z0-9._-]{20,}`)
	slackRe     = regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{10,}`)
	githubRe    = regexp.MustCompile(`\b(?:ghp_|gho_|github_pat_)[A-Za-z0-9_]{20,}`)
	linearRe    = regexp.MustCompile(`lin_api_[A-Za-z0-9]{20,}`)
	pemRe       = regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)
	// Offers are base64url of a JSON document; DecodeOffer is the authority
	// for "this blob is a pairing code".
	offerRe = regexp.MustCompile(`[A-Za-z0-9_-]{80,}`)
)

func scrub(p []byte) []byte {
	s := string(p)
	s = atlassianRe.ReplaceAllString(s, "<redacted>")
	s = basicRe.ReplaceAllString(s, "<redacted>")
	s = bearerRe.ReplaceAllString(s, "${1}<redacted>")
	s = slackRe.ReplaceAllString(s, "<redacted>")
	s = githubRe.ReplaceAllString(s, "<redacted>")
	s = linearRe.ReplaceAllString(s, "<redacted>")
	s = pemRe.ReplaceAllString(s, "<redacted>")
	s = offerRe.ReplaceAllStringFunc(s, func(tok string) string {
		if _, err := pairing.DecodeOffer(tok); err == nil {
			return "<redacted>"
		}
		return tok
	})
	return []byte(s)
}
