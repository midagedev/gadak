// Package deeplink parses and builds the gadak:// URLs that hand a piece of
// gadak to someone — a link in chat or on a web page instead of a shell, a
// running serve, and a command with side effects.
//
// # The grammar
//
//	gadak://<action>[/w/<profile>][/<subject>][?<params>]
//
//	gadak://view?pj=GDK&sc=inprogress        a filtered list, primary mirror
//	gadak://view/w/oss?issue=GDK-119         the same, on the "oss" mirror
//
// The action is the host. The optional `/w/<profile>` segment is the same one
// the web UI uses for a workspace mount, in the same position, so a gadak://
// link and the http:// link `gadak views open` prints describe the same thing
// the same way. The optional subject that may follow is what the action acts
// on — an issue key, a page key — and the query is passed through verbatim.
//
// # Why the whole grammar exists for one action
//
// Only `view` is implemented today, and this package deliberately does not
// know that: it validates the *shape* and returns whichever action it read.
// Deciding which actions exist belongs to the app that handles them
// (desktop/deeplink.go), not to the parser.
//
// The split matters because these two halves ship separately. A link lives in
// a chat log forever and is opened by whatever version happens to be
// installed, so the grammar is the part that must not change — while the set
// of actions grows every time a surface becomes addressable. Keeping the
// action list out of the parser means growing it is a table entry in the
// handler, and an older app meeting a newer link produces "this link needs a
// newer Gadak" instead of a parse error.
//
// What `view` can say is decided on the web side: the hash carries the view
// params (filters, display) and the place params (which panel and screen —
// issue, doc, person, feed, settings tab…). The registry of place params is
// web/src/lib/url-state.ts; a param registered there is linkable from here
// the same moment, with no change in this package. A second action is for
// what a hash cannot express — a place with no URL, or a different kind of
// address entirely.
//
// # Security posture
//
// Any web page can embed a gadak:// link, so the worst one may achieve is
// that the user briefly looks at the wrong thing. The grammar carries no verb
// and no payload beyond an address: a link says *where to go*, never what to
// do. Handlers must keep that true — an action that writes, or that submits
// anything, does not belong in this scheme however convenient it would be.
//
// Validation here is shape and size only:
//
//   - a well-formed action, an optional profile that is a safe directory
//     name, an optional subject that cannot escape a path,
//   - length and arity limits that bound what a handler is asked to parse.
//
// There is deliberately no allowlist of view parameter keys. The keys the UI
// understands are owned by VIEW_PARAM_KEYS in web/src/lib/view-config; a
// second copy here would drift the moment a new axis is added there, and the
// drift fails silently in the direction that matters — a link would stop
// working with no error anywhere. Unknown keys pass through and are ignored
// by the UI, which is the "wrong thing" worst case already accepted above.
package deeplink

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// Callers must be able to tell these apart: a handler stays silent when the
// link simply is not ours (another app's scheme reaching a shared handler)
// and logs when it is ours but refused.
var (
	// ErrNotGadak reports a URL that is not a gadak:// link at all.
	ErrNotGadak = errors.New("deeplink: not a gadak:// URL")

	// ErrMalformed reports a gadak:// link this package refuses to parse.
	// Wrapping carries the specific rule that fired.
	ErrMalformed = errors.New("deeplink: malformed gadak:// URL")
)

// Scheme is the URL scheme, registered by the macOS bundle in
// desktop/build-app.sh. One owner, because the two are in a .go and a .sh
// file that nothing else connects and a disagreement is silent.
const Scheme = "gadak"

// ActionView shows a list of issues, filtered by the link's hash. The only
// action implemented today; see the package comment for why the parser does
// not enforce that.
const ActionView = "view"

// profileSegment is the path segment that introduces a profile, matching the
// web UI's /w/<name> workspace mount. Reserved: a subject may not be "w".
const profileSegment = "w"

// Size and shape limits. A deep link is typed or pasted by humans and handed
// over in chat, so real links are short; every constant here exists to bound
// what a handler can be asked to parse.
const (
	// maxInputLen bounds the whole link. URL handlers receive attacker-chosen
	// strings; 2048 bytes is far above any real view hash.
	maxInputLen = 2048

	// maxParams bounds how many parameters a link may carry. Real views use a
	// handful of axes (q, ks, pj, sc, a few f.<alias> filters); 32 leaves
	// headroom while capping the validation loop.
	maxParams = 32

	// maxValueLen bounds one parameter's value. Values are key lists and
	// search strings; 512 bytes is an order of magnitude above a real one.
	maxValueLen = 512

	// maxKeyLen bounds a parameter key. The static keys are two-letter
	// aliases, but a discovered-field axis is "f." + an ASCIISlug of a Jira
	// field's display name (internal/fields/slug.go), so its length is
	// whatever someone called a custom field. Measured across this machine's
	// profiles the longest real key is 19 bytes (f.requirement_11461), but
	// "Acceptance Criteria Verification Status" would slug to 41 — a
	// plausible field name whose link would silently do nothing. The cap
	// bounds the validation loop, and 64 bounds it as well as 32 does.
	maxKeyLen = 64
)

var (
	// actionPattern is the shape of an action. Lowercase words, hyphens
	// allowed for a future two-word action; no dots, so an action can never
	// be mistaken for a hostname.
	actionPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

	// profilePattern mirrors config's profile-name rule: profile names become
	// directory names, so the first character is alphanumeric and the rest
	// contain no path separator or shell metacharacter.
	profilePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

	// subjectPattern is what an action may act on: an issue key (GDK-119), a
	// page key, an email. Permissive in character, strict in what it cannot
	// be — no separators, and the traversal names are refused outright below.
	subjectPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.@+-]{0,127}$`)

	// keyPattern is the shape of a view-parameter key: the UI router's short
	// lowercase aliases (q, ks, pj, sc, f.<alias>). Shape only — keys are
	// never matched against the real key list; see the package comment. Built
	// from maxKeyLen so the limit has one owner and tests can derive it.
	keyPattern = regexp.MustCompile(fmt.Sprintf(`^[a-z][a-z0-9_.]{0,%d}$`, maxKeyLen-1))
)

// A Link is a parsed gadak:// URL.
type Link struct {
	// Action is what the link asks for, lowercased. Compare against the
	// Action* constants; an unrecognised one is a link from a newer gadak,
	// not an error.
	Action string

	// Profile is the mirror the link addresses. "" means the primary one;
	// "default" is accepted and means the same, because config does.
	Profile string

	// Subject is what the action acts on, or "" when it needs none. No action
	// uses it yet; the slot is reserved so adding one (gadak://issue/GDK-119)
	// does not have to change the grammar.
	Subject string

	// Hash is the view hash: the raw query string exactly as the UI's hash
	// router and uifocus want it, with no leading "#" or "?" and
	// percent-encoding untouched. Returned verbatim — never decoded,
	// re-encoded, or re-ordered — because the CLI's composeServeURL treats it
	// as opaque too, and any rewrite here would make the two paths disagree.
	// Empty when the link carries no query; whether that is acceptable is the
	// action's rule, not the grammar's.
	Hash string
}

// Parse turns a gadak:// URL into a Link.
//
// The host is compared case-insensitively and returned lowercased; the path
// is not, because profile and subject are identifiers. One trailing slash
// before the query is tolerated, because a human retyping a link produces
// one.
//
// err is ErrNotGadak when the URL is not a gadak:// link at all (the caller
// should stay silent) and wraps ErrMalformed when it is one this package
// rejects (the caller should say so).
func Parse(raw string) (Link, error) {
	u, err := url.Parse(raw)
	if err != nil {
		// Unparseable, but still ours if it claims our scheme prefix.
		if hasSchemePrefix(raw) {
			return Link{}, malformedf("unparseable URL: %v", err)
		}
		return Link{}, ErrNotGadak
	}
	if !strings.EqualFold(u.Scheme, Scheme) {
		return Link{}, ErrNotGadak
	}
	if len(raw) > maxInputLen {
		return Link{}, malformedf("input is %d bytes, over the %d-byte limit", len(raw), maxInputLen)
	}
	// The grammar carries no fragment. Checking the raw string also catches a
	// bare trailing "#", which url.Parse reports as an empty Fragment no
	// different from none (measured; see the tests).
	if strings.ContainsRune(raw, '#') {
		return Link{}, malformedf("fragment not allowed")
	}
	if u.User != nil {
		return Link{}, malformedf("userinfo not allowed")
	}
	// net/url preserves host case (measured) and keeps any port in Host, so
	// this fold is load-bearing and "view:7777" fails the pattern.
	action := strings.ToLower(u.Host)
	if !actionPattern.MatchString(action) {
		return Link{}, malformedf("action %q is not a plain lowercase word", u.Host)
	}
	profile, subject, err := splitPath(u.Path)
	if err != nil {
		return Link{}, err
	}
	if err := validateQuery(u.RawQuery); err != nil {
		return Link{}, err
	}
	return Link{Action: action, Profile: profile, Subject: subject, Hash: u.RawQuery}, nil
}

// Compose builds a gadak:// link: the inverse of Parse, in the package that
// owns the grammar, so the two cannot drift into disagreeing about the shape.
//
// prefix is the workspace segment — "" for the primary mirror, "/w/<name>"
// otherwise — and is taken as a parameter rather than derived here on
// purpose. The rule mapping a profile onto that segment belongs to
// internal/workspace, which pulls in the server and the sync engine; a parser
// any process might run should not drag those along to answer a question
// about a URL. Callers pass workspace.Prefix(profile, "").
//
// An empty hash yields "" for ActionView: there is no view to link to, and a
// link to nothing is worse than no link, because it looks like one that works.
func Compose(action, prefix, hash string) string {
	if action == ActionView && hash == "" {
		return ""
	}
	out := Scheme + "://" + action + prefix
	if hash != "" {
		out += "?" + hash
	}
	return out
}

// hasSchemePrefix reports whether raw begins with "gadak:", case-insensitively
// and without allocating — used to classify unparseable input.
func hasSchemePrefix(raw string) bool {
	const prefix = Scheme + ":"
	return len(raw) >= len(prefix) && strings.EqualFold(raw[:len(prefix)], prefix)
}

// splitPath reads the optional profile and subject out of the decoded path.
//
// Accepted: "", "/", "/w/<profile>", "/<subject>", "/w/<profile>/<subject>",
// each with an optional trailing slash. A leading "w" segment is reserved as
// the profile introducer, so a subject may not be the single letter "w" —
// no issue key, page key, or address is.
//
// The check runs on u.Path, the decoded form: an encoded separator (%2f) can
// only add segments, which the segment count and per-segment patterns then
// reject, so decoding cannot smuggle a separator into one identifier.
func splitPath(path string) (profile, subject string, err error) {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return "", "", nil
	}
	seg := strings.Split(trimmed, "/")
	if seg[0] == profileSegment {
		if len(seg) < 2 {
			return "", "", malformedf("empty profile after /%s/", profileSegment)
		}
		if len(seg) > 3 {
			return "", "", malformedf("path %q has more segments than the grammar allows", path)
		}
		if profile, err = checkIdent("profile", seg[1], profilePattern); err != nil {
			return "", "", err
		}
		if len(seg) == 3 {
			if subject, err = checkIdent("subject", seg[2], subjectPattern); err != nil {
				return "", "", err
			}
		}
		return profile, subject, nil
	}
	if len(seg) > 1 {
		return "", "", malformedf("path %q is not \"/w/<profile>\", \"/<subject>\", or both", path)
	}
	if subject, err = checkIdent("subject", seg[0], subjectPattern); err != nil {
		return "", "", err
	}
	return "", subject, nil
}

// checkIdent rejects the traversal names before the pattern. Both patterns
// already exclude them by requiring a leading alphanumeric; the explicit
// check states the invariant so a later pattern edit cannot quietly reopen
// the one input class here that could escape a directory.
func checkIdent(what, s string, pattern *regexp.Regexp) (string, error) {
	if s == "." || s == ".." {
		return "", malformedf("%s %q is a directory reference, not a name", what, s)
	}
	if !pattern.MatchString(s) {
		return "", malformedf("%s %q is not a plausible %s", what, s, what)
	}
	return s, nil
}

// validateQuery enforces the size limits on the raw query. It never inspects
// what a parameter means — there is no key allowlist; see the package
// comment. A parameter without "=" counts as one with an empty value, the
// same read the UI's URLSearchParams gives it.
//
// An empty query is legal here. Whether an action can act without one is that
// action's rule: ActionView cannot, and its handler says so.
func validateQuery(q string) error {
	if q == "" {
		return nil
	}
	// net/url already rejects control bytes anywhere in the URL (measured),
	// but this package's contract should not rest on the parser's internals:
	// the scan stays so a parser swap cannot reopen it.
	for i := 0; i < len(q); i++ {
		if q[i] < 0x20 || q[i] == 0x7f {
			return malformedf("control byte 0x%02x in query", q[i])
		}
	}
	parts := strings.Split(q, "&")
	if len(parts) > maxParams {
		return malformedf("%d parameters, over the limit of %d", len(parts), maxParams)
	}
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		key, value, _ := strings.Cut(part, "=")
		if !keyPattern.MatchString(key) {
			return malformedf("parameter key %q does not match the key shape", key)
		}
		if len(value) > maxValueLen {
			return malformedf("value for %q is %d bytes, over the %d-byte limit", key, len(value), maxValueLen)
		}
		// Two values for one axis is ambiguous, and every producer in this
		// repo emits each key once — reject rather than pick.
		if _, dup := seen[key]; dup {
			return malformedf("duplicate parameter key %q", key)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func malformedf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrMalformed, fmt.Sprintf(format, args...))
}
