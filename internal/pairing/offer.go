package pairing

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// OfferV1 is the only offer format this build understands. A version bump
// means capability change, not app version: mixed-version clients syncing
// against one serve is the normal state (GDK-433 prior-art survey), so an
// unknown version is an explicit refusal, never a silent best-effort parse.
const OfferV1 = 1

// Offer is the one-line pairing secret `gadak pairing mint` prints and a
// remote gadak consumes via `gadak init --pairing-code`. Base64url of a
// JSON document: one pasteable line, no shell-hostile characters.
//
// The offer carries the token itself, so it is treated like a credential:
// never echoed into a log line, an error message, or argv more than the
// one flag the user already typed. ExpiresAt is advisory for the human
// (RFC3339, home origin's clock); the gate judges expiry from the store.
type Offer struct {
	V         int    `json:"v"`
	Endpoint  string `json:"endpoint"`
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
	Label     string `json:"label"`
}

// EncodeOffer renders o as the single-line base64url form.
func EncodeOffer(o Offer) (string, error) {
	data, err := json.Marshal(o)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

// DecodeOffer parses an offer line. Errors describe the problem without
// quoting the payload — the token inside must not leak through an error
// path. Unknown version, missing endpoint, and missing token are distinct
// errors so `init --pairing-code` can tell the user what to redo.
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
	if o.V != OfferV1 {
		return Offer{}, fmt.Errorf("pairing offer: version %d is not supported (this gadak speaks v%d)", o.V, OfferV1)
	}
	if strings.TrimSpace(o.Endpoint) == "" {
		return Offer{}, errors.New("pairing offer: no endpoint")
	}
	if o.Token == "" {
		return Offer{}, errors.New("pairing offer: no token")
	}
	return o, nil
}

// FormatExpiry renders an offer's ExpiresAt for mint output; "" when unset.
func FormatExpiry(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
