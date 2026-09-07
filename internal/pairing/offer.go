package pairing

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// OfferV1 and OfferV2 are the offer formats this build understands. A
// version bump means capability change, not app version: mixed-version
// clients syncing against one serve is the normal state (GDK-433
// prior-art survey), so an unknown version is an explicit refusal, never a
// silent best-effort parse. v2 (GDK-1498) carries one token per scope in a
// list — one scan pairs a phone's mirror and its shell — where v1 carries
// a single token whose scope lives server-side. A v1 offer for one scope
// stays byte-identical v1 (GDK-799: the v1 line is a promise to existing
// pairers).
const (
	OfferV1 = 1
	OfferV2 = 2
)

// Offer is the one-line pairing secret `gadak pairing mint` prints and a
// remote gadak consumes via `gadak init --pairing-code`. Base64url of a
// JSON document: one pasteable line, no shell-hostile characters.
//
// The offer carries tokens, so it is treated like a credential: never
// echoed into a log line, an error message, or argv more than the one
// flag the user already typed. ExpiresAt is advisory for the human
// (RFC3339, home origin's clock); the gate judges expiry from the store.
type Offer struct {
	V         int          `json:"v"`
	Endpoint  string       `json:"endpoint"`
	Token     string       `json:"token,omitempty"` // v1's single token; never set in v2
	ExpiresAt string       `json:"expires_at"`
	Label     string       `json:"label"`
	Tokens    []OfferToken `json:"tokens,omitempty"` // v2's scoped list; never set in v1
}

// OfferToken is one scoped credential inside an offer. v2 entries name
// their scope; the normalized v1 entry carries Scope "" — v1's scope is
// not in the payload (it lives in the server's store), so callers that
// need it decide from context, exactly as they did before v2 existed.
type OfferToken struct {
	Scope string `json:"scope"`
	Token string `json:"token"`
}

// EncodeOffer renders o as the single-line base64url form. It refuses the
// shapes a version cannot carry — a v1 with a token list, a v2 with a
// top-level token, no tokens, a duplicate scope, or an entry missing its
// scope or token — so a malformed offer cannot leave a mint path. Errors
// name the defect without quoting the payload.
func EncodeOffer(o Offer) (string, error) {
	if err := validateOfferShape(o); err != nil {
		return "", err
	}
	data, err := json.Marshal(o)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

// DecodeOffer parses an offer line of either version and returns it with
// the normalized token list: one entry per scope for v2, one unscoped
// entry for v1. Errors describe the problem without quoting the payload —
// the tokens inside must not leak through an error path. Unknown version,
// missing endpoint, and each malformed-token shape are distinct errors so
// `init --pairing-code` can tell the user what to redo.
func DecodeOffer(s string) (Offer, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Offer{}, errors.New("pairing offer: empty")
	}
	// Tolerate padded input from a copy that wrapped the std encoding.
	enc := base64.RawURLEncoding
	data, err := enc.DecodeString(strings.TrimRight(s, "="))
	if err != nil {
		padded, perr := base64.URLEncoding.DecodeString(s)
		if perr != nil {
			return Offer{}, errors.New("pairing offer: not base64url")
		}
		data = padded
	}
	var o Offer
	if err := json.Unmarshal(data, &o); err != nil {
		return Offer{}, errors.New("pairing offer: malformed document")
	}
	switch o.V {
	case OfferV1, OfferV2:
	default:
		return Offer{}, fmt.Errorf("pairing offer: version %d is not supported (this gadak speaks v1 and v2)", o.V)
	}
	if strings.TrimSpace(o.Endpoint) == "" {
		return Offer{}, errors.New("pairing offer: no endpoint")
	}
	switch o.V {
	case OfferV1:
		if len(o.Tokens) > 0 {
			return Offer{}, errors.New("pairing offer: v1 carries one token, not a list")
		}
		if o.Token == "" {
			return Offer{}, errors.New("pairing offer: no token")
		}
		o.Tokens = []OfferToken{{Scope: "", Token: o.Token}}
	case OfferV2:
		if err := validateOfferShape(o); err != nil {
			return Offer{}, err
		}
	}
	return o, nil
}

// ConsumerToken picks the token a pairing consumer — a second gadak
// binding this serve as its workspace origin (`init --pairing-code`) —
// presents: the serve token when the offer carries one, else the origin
// one. A v1 offer has a single token whose scope the payload cannot name;
// it is returned as-is and the serve's gate judges it, exactly as every
// pre-v2 consumer behaved. A terminal token is never returned — it opens
// a shell, not a workspace — and an offer carrying nothing else is
// refused naming that scope.
func (o Offer) ConsumerToken() (string, error) {
	if o.V == OfferV1 {
		return o.Token, nil
	}
	var origin string
	for _, e := range o.Tokens {
		switch e.Scope {
		case ScopeServe:
			return e.Token, nil
		case ScopeOrigin:
			origin = e.Token
		}
	}
	if origin != "" {
		return origin, nil
	}
	return "", fmt.Errorf("pairing offer: this offer's only token is %s-scoped, which opens a shell on that machine — not a workspace; ask the home machine for an offer with the serve or origin scope", ScopeTerminal)
}

// validateOfferShape enforces the per-version document shape. Scope
// values are deliberately not checked against the three known scopes:
// unknown keys ride along in v1 today, and a newer home minting a newer
// scope must not make an older consumer refuse the whole offer — consumers
// pick the scopes they know by name.
func validateOfferShape(o Offer) error {
	switch o.V {
	case OfferV1:
		if len(o.Tokens) > 0 {
			return errors.New("pairing offer: v1 carries one token, not a list")
		}
		if o.Token == "" {
			return errors.New("pairing offer: no token")
		}
	case OfferV2:
		if o.Token != "" {
			return errors.New("pairing offer: v2 carries its tokens as a list — a top-level token is the v1 shape")
		}
		if len(o.Tokens) == 0 {
			return errors.New("pairing offer: v2 carries no tokens")
		}
		return validateOfferTokens(o.Tokens)
	default:
		return fmt.Errorf("pairing offer: version %d is not supported (this gadak speaks v1 and v2)", o.V)
	}
	return nil
}

func validateOfferTokens(tokens []OfferToken) error {
	seen := make(map[string]bool, len(tokens))
	for _, e := range tokens {
		if e.Scope == "" {
			return errors.New("pairing offer: a v2 token entry has no scope")
		}
		if e.Token == "" {
			return errors.New("pairing offer: a v2 token entry has no token")
		}
		if seen[e.Scope] {
			return fmt.Errorf("pairing offer: v2 carries two tokens for scope %q", e.Scope)
		}
		seen[e.Scope] = true
	}
	return nil
}

// FormatExpiry renders an offer's ExpiresAt for mint output; "" when unset.
func FormatExpiry(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
