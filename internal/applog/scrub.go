package applog

import (
	"regexp"

	"github.com/midagedev/gadak/internal/pairing"
	"github.com/midagedev/gadak/internal/secretscan"
)

// Credential shapes are owned by internal/secretscan; this scrubber composes
// over its exported table, so a shape added there is erased here too. The
// corpus test in scrub_test.go fails until a fixture covers every pattern
// name. Two rules are log-specific and stay in this file.
var (
	// Wide bearer. The http_bearer_token row of the secretscan table
	// requires the Authorization: header, but a token in a log line can
	// follow any "Bearer " — a curl example, a debug line, a header dump —
	// so this matcher is wider. It must run BEFORE the secretscan table:
	// once the token bytes are replaced, the narrower pattern has nothing
	// left to match, while the reverse order would collapse
	// "Authorization: Bearer <token>" to a bare <redacted> and change what
	// logs record today. ${1} keeps the "Bearer " prefix in the output.
	bearerRe = regexp.MustCompile(`(?i)(Bearer\s+)[A-Za-z0-9._-]{20,}`)
	// The pairing-offer matcher is extra: an offer is not a repo-scan shape,
	// but the product rule is never to print one — not a prefix, not a
	// length, not a hash. Offers are base64url of a JSON document;
	// DecodeOffer is the authority for "this blob is a pairing code".
	offerRe = regexp.MustCompile(`[A-Za-z0-9_-]{80,}`)
)

// secretPatterns is bound once at init: scrub runs on every log write, and
// fetching the table per call would put a slice traversal on every line.
var secretPatterns = secretscan.Patterns()

func scrub(p []byte) []byte {
	s := string(p)
	s = bearerRe.ReplaceAllString(s, "${1}<redacted>")
	for _, pat := range secretPatterns {
		s = pat.Re.ReplaceAllString(s, "<redacted>")
	}
	s = offerRe.ReplaceAllStringFunc(s, func(tok string) string {
		if _, err := pairing.DecodeOffer(tok); err == nil {
			return "<redacted>"
		}
		return tok
	})
	return []byte(s)
}
