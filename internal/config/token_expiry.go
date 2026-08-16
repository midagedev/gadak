package config

import (
	"fmt"
	"strings"
	"time"
)

// Token expiry assessment. One owner: AssessTokenExpiry. Surfaces (status,
// sync_health, the freshness chip) render what this returns; they do not
// re-derive state.
const (
	TokenExpiryOK       = "ok"
	TokenExpiryExpiring = "expiring"
	TokenExpiryExpired  = "expired"
	TokenExpiryUnknown  = "unknown"

	TokenExpirySourceUser    = "user"
	TokenExpirySourceAssumed = "assumed"

	// TokenDefaultLifetimeDays is Atlassian's default API-token lifetime
	// when the user skipped the date from the create dialog.
	TokenDefaultLifetimeDays = 365
	// TokenExpiryWarnDays is the first day the warning surfaces (inclusive).
	TokenExpiryWarnDays = 14
	// TokenExpiryUrgentDays is the first day the warning is urgent (inclusive).
	TokenExpiryUrgentDays = 3

	// TokenTimeFormat is ISOMilli so assumed dates line up with
	// tokenVerifiedAt and every other store timestamp.
	TokenTimeFormat = ISOMilli
)

// TokenExpiry is the computed warning state for a stored API token.
// DaysLeft is nil when State is unknown.
type TokenExpiry struct {
	State     string `json:"state"`
	DaysLeft  *int   `json:"days_left,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
	Source    string `json:"source,omitempty"`
	Urgent    bool   `json:"urgent,omitempty"`
	// Message is the one English warning line. Empty when there is nothing
	// to say (ok / unknown).
	Message string `json:"message,omitempty"`
}

// TokenExpiryAt is AssessTokenExpiry against this config and now.
func (c *Config) TokenExpiryAt(now time.Time) TokenExpiry {
	if c == nil {
		return TokenExpiry{State: TokenExpiryUnknown}
	}
	return AssessTokenExpiry(now, c.TokenExpiresAt, c.TokenExpirySource)
}

// AssessTokenExpiry maps (now, stored expiry, source) onto a warning state.
// Missing or unparseable dates are unknown — there is nothing to warn from.
//
// DaysLeft is remaining/elapsed time in whole 24h periods, truncated toward
// zero. now >= expiresAt is expired (including a 0-day remaining of exactly
// now). 15 days is still ok; 14 days is the first warning; 3 days is urgent.
func AssessTokenExpiry(now time.Time, expiresAt, source string) TokenExpiry {
	now = now.UTC()
	raw := strings.TrimSpace(expiresAt)
	if raw == "" {
		return TokenExpiry{State: TokenExpiryUnknown}
	}
	exp, err := ParseTokenExpiresAt(raw)
	if err != nil {
		return TokenExpiry{State: TokenExpiryUnknown}
	}
	src := strings.TrimSpace(source)
	out := TokenExpiry{
		ExpiresAt: FormatTokenTime(exp),
		Source:    src,
	}
	remaining := exp.Sub(now)
	days := int(remaining / (24 * time.Hour))
	out.DaysLeft = &days
	switch {
	case remaining <= 0:
		out.State = TokenExpiryExpired
	case days <= TokenExpiryWarnDays:
		out.State = TokenExpiryExpiring
		out.Urgent = days <= TokenExpiryUrgentDays
	default:
		out.State = TokenExpiryOK
	}
	out.Message = out.WarningLine()
	return out
}

// WarningLine is the English sentence status and sync_health surface.
// Empty when there is nothing to warn about.
func (e TokenExpiry) WarningLine() string {
	switch e.State {
	case TokenExpiryExpiring:
		return "API token " + expiringWhen(e.DaysLeft) + e.hedge() + tokenExpiryRemedy
	case TokenExpiryExpired:
		return "API token " + expiredWhen(e.DaysLeft) + e.hedge() + tokenExpiryRemedy
	default:
		return ""
	}
}

// tokenExpiryRemedy names the two paths that actually write a new token
// (cmd/gadak/init.go, PUT credential/). There is no `gadak connect`.
const tokenExpiryRemedy = " — create a new one and run gadak init, or replace the token in Settings."

func (e TokenExpiry) hedge() string {
	if e.Source == TokenExpirySourceAssumed {
		return " (assumed from the default lifetime)"
	}
	return ""
}

func expiringWhen(days *int) string {
	if days == nil {
		return "expires soon"
	}
	switch *days {
	case 0:
		return "expires today"
	case 1:
		return "expires in 1 day"
	default:
		return fmt.Sprintf("expires in %d days", *days)
	}
}

func expiredWhen(days *int) string {
	if days == nil {
		return "expired"
	}
	switch *days {
	case 0:
		return "expired"
	case -1:
		return "expired 1 day ago"
	default:
		if *days < 0 {
			return fmt.Sprintf("expired %d days ago", -*days)
		}
		return "expired"
	}
}

// ParseTokenExpiresAt accepts a calendar date (YYYY-MM-DD, from an HTML date
// input or Atlassian's create dialog) or an RFC3339 timestamp. Date-only
// values are midnight UTC that day.
func ParseTokenExpiresAt(raw string) (time.Time, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty expiry date")
	}
	for _, layout := range []string{
		time.DateOnly,
		TokenTimeFormat,
		time.RFC3339Nano,
		time.RFC3339,
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid expiry date %q (want YYYY-MM-DD or RFC3339)", s)
}

// FormatTokenTime writes the on-disk / JSON form (UTC, millisecond).
func FormatTokenTime(t time.Time) string {
	return t.UTC().Format(TokenTimeFormat)
}

// ApplyTokenExpiry writes TokenExpiresAt and TokenExpirySource.
// A non-empty userRaw is source "user". An empty userRaw with a parseable
// verifiedAt assumes verifiedAt + 365 days. Empty userRaw and no verifiedAt
// leaves the fields untouched (offline init must not invent a date).
func (c *Config) ApplyTokenExpiry(userRaw, verifiedAt string) error {
	if c == nil {
		return nil
	}
	userRaw = strings.TrimSpace(userRaw)
	if userRaw != "" {
		t, err := ParseTokenExpiresAt(userRaw)
		if err != nil {
			return err
		}
		c.TokenExpiresAt = FormatTokenTime(t)
		c.TokenExpirySource = TokenExpirySourceUser
		return nil
	}
	if strings.TrimSpace(verifiedAt) == "" {
		return nil
	}
	base, err := ParseTokenExpiresAt(verifiedAt)
	if err != nil {
		return err
	}
	c.TokenExpiresAt = FormatTokenTime(base.AddDate(0, 0, TokenDefaultLifetimeDays))
	c.TokenExpirySource = TokenExpirySourceAssumed
	return nil
}

// ApplyTokenExpiryIfNeeded is the init path: do not reset an existing date
// when the token was kept and the user did not supply a new one. Connect
// and replace-token always call ApplyTokenExpiry (they always store a token).
func (c *Config) ApplyTokenExpiryIfNeeded(userRaw, verifiedAt string, tokenReplaced bool) error {
	if c == nil {
		return nil
	}
	if strings.TrimSpace(userRaw) != "" || tokenReplaced || c.TokenExpiresAt == "" {
		return c.ApplyTokenExpiry(userRaw, verifiedAt)
	}
	return nil
}

// ClearTokenExpiry drops the stored date. Called when the credential is deleted.
func (c *Config) ClearTokenExpiry() {
	if c == nil {
		return
	}
	c.TokenExpiresAt = ""
	c.TokenExpirySource = ""
}
