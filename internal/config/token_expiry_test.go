package config

import (
	"strings"
	"testing"
	"time"
)

func TestAssessTokenExpiryBoundaries(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	at := func(d time.Duration) string { return formatTokenTime(now.Add(d)) }
	day := 24 * time.Hour

	cases := []struct {
		name       string
		expires    string
		source     string
		wantState  string
		wantDays   *int
		wantUrgent bool
		wantHedge  bool
		wantPhrase string
	}{
		{name: "missing date", wantState: TokenExpiryUnknown},
		{name: "unparseable", expires: "not-a-date", source: TokenExpirySourceUser, wantState: TokenExpiryUnknown},
		{name: "15 days ok", expires: at(15 * day), source: TokenExpirySourceUser, wantState: TokenExpiryOK, wantDays: intp(15)},
		{name: "14 days expiring", expires: at(14 * day), source: TokenExpirySourceUser, wantState: TokenExpiryExpiring, wantDays: intp(14), wantPhrase: "expires in 14 days"},
		{name: "14 days assumed hedges", expires: at(14 * day), source: TokenExpirySourceAssumed, wantState: TokenExpiryExpiring, wantDays: intp(14), wantHedge: true, wantPhrase: "expires in 14 days"},
		{name: "4 days", expires: at(4 * day), source: TokenExpirySourceUser, wantState: TokenExpiryExpiring, wantDays: intp(4), wantPhrase: "expires in 4 days"},
		{name: "3 days urgent", expires: at(3 * day), source: TokenExpirySourceUser, wantState: TokenExpiryExpiring, wantDays: intp(3), wantUrgent: true, wantPhrase: "expires in 3 days"},
		{name: "0 days remaining (1h left)", expires: at(time.Hour), source: TokenExpirySourceUser, wantState: TokenExpiryExpiring, wantDays: intp(0), wantUrgent: true, wantPhrase: "expires today"},
		{name: "0 days expired (exactly now)", expires: formatTokenTime(now), source: TokenExpirySourceUser, wantState: TokenExpiryExpired, wantDays: intp(0), wantPhrase: "API token expired"},
		{name: "-1 day", expires: at(-1 * day), source: TokenExpirySourceUser, wantState: TokenExpiryExpired, wantDays: intp(-1), wantPhrase: "expired 1 day ago"},
		{name: "-1 day assumed hedges", expires: at(-1 * day), source: TokenExpirySourceAssumed, wantState: TokenExpiryExpired, wantDays: intp(-1), wantHedge: true, wantPhrase: "expired 1 day ago"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := AssessTokenExpiry(now, tc.expires, tc.source)
			if got.State != tc.wantState {
				t.Errorf("state = %q, want %q", got.State, tc.wantState)
			}
			if !intpEq(got.DaysLeft, tc.wantDays) {
				t.Errorf("days_left = %v, want %v", deref(got.DaysLeft), deref(tc.wantDays))
			}
			if got.Urgent != tc.wantUrgent {
				t.Errorf("urgent = %v, want %v", got.Urgent, tc.wantUrgent)
			}
			if tc.wantState == TokenExpiryOK || tc.wantState == TokenExpiryUnknown {
				if got.Message != "" {
					t.Errorf("message should be empty for %s, got %q", tc.wantState, got.Message)
				}
				return
			}
			if got.Message == "" {
				t.Fatal("warning message is empty")
			}
			if tc.wantPhrase != "" && !strings.Contains(got.Message, tc.wantPhrase) {
				t.Errorf("message %q, want to contain %q", got.Message, tc.wantPhrase)
			}
			if !strings.Contains(got.Message, "gadak init") {
				t.Errorf("message must name gadak init, got %q", got.Message)
			}
			if strings.Contains(got.Message, "gadak connect") {
				t.Errorf("message invents gadak connect: %q", got.Message)
			}
			hedged := strings.Contains(got.Message, "assumed from the default lifetime")
			if hedged != tc.wantHedge {
				t.Errorf("hedge = %v, want %v; message %q", hedged, tc.wantHedge, got.Message)
			}
		})
	}
}

func TestParseTokenExpiresAt(t *testing.T) {
	t.Parallel()
	got, err := parseTokenExpiresAt("2026-12-31")
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("date-only = %s, want %s", got, want)
	}
	rfc := "2026-08-20T09:15:00.000Z"
	got, err = parseTokenExpiresAt(rfc)
	if err != nil {
		t.Fatal(err)
	}
	if formatTokenTime(got) != rfc {
		t.Fatalf("round-trip %q → %q", rfc, formatTokenTime(got))
	}
	if _, err := parseTokenExpiresAt("31/12/2026"); err == nil {
		t.Fatal("expected error for slash date")
	}
}

func TestApplyTokenExpiry(t *testing.T) {
	t.Parallel()
	var c Config
	if err := c.ApplyTokenExpiry("2027-01-15", "2026-08-15T12:00:00.000Z"); err != nil {
		t.Fatal(err)
	}
	if c.TokenExpirySource != TokenExpirySourceUser {
		t.Fatalf("source = %q, want user", c.TokenExpirySource)
	}
	if c.TokenExpiresAt != "2027-01-15T00:00:00.000Z" {
		t.Fatalf("expires = %q", c.TokenExpiresAt)
	}

	var assumed Config
	if err := assumed.ApplyTokenExpiry("", "2026-08-15T12:00:00.000Z"); err != nil {
		t.Fatal(err)
	}
	if assumed.TokenExpirySource != TokenExpirySourceAssumed {
		t.Fatalf("source = %q, want assumed", assumed.TokenExpirySource)
	}
	if assumed.TokenExpiresAt != "2027-08-15T12:00:00.000Z" {
		t.Fatalf("assumed expires = %q, want verifiedAt+365d", assumed.TokenExpiresAt)
	}

	var offline Config
	if err := offline.ApplyTokenExpiry("", ""); err != nil {
		t.Fatal(err)
	}
	if offline.TokenExpiresAt != "" || offline.TokenExpirySource != "" {
		t.Fatalf("offline must not invent expiry: %+v", offline)
	}

	var bad Config
	if err := bad.ApplyTokenExpiry("whenever", "2026-08-15T12:00:00.000Z"); err == nil {
		t.Fatal("expected error for invalid user date")
	}

	var keep Config
	keep.TokenExpiresAt = "2027-01-01T00:00:00.000Z"
	keep.TokenExpirySource = TokenExpirySourceUser
	if err := keep.ApplyTokenExpiryIfNeeded("", "2026-08-15T12:00:00.000Z", false); err != nil {
		t.Fatal(err)
	}
	if keep.TokenExpiresAt != "2027-01-01T00:00:00.000Z" {
		t.Fatalf("kept expiry overwritten: %q", keep.TokenExpiresAt)
	}
	if err := keep.ApplyTokenExpiryIfNeeded("", "2026-08-15T12:00:00.000Z", true); err != nil {
		t.Fatal(err)
	}
	if keep.TokenExpirySource != TokenExpirySourceAssumed {
		t.Fatalf("token replace should assume: %q", keep.TokenExpirySource)
	}

	var n *Config
	n.ApplyTokenExpiry("2027-01-01", "")
	n.ApplyTokenExpiryIfNeeded("", "", false)
	n.ClearTokenExpiry()
	n.TokenExpiryAt(time.Now())
}

func TestConfigTokenExpiryAtNil(t *testing.T) {
	t.Parallel()
	var n *Config
	got := n.TokenExpiryAt(time.Now())
	if got.State != TokenExpiryUnknown {
		t.Fatalf("nil config: %+v", got)
	}
}

func intp(n int) *int { return &n }

func intpEq(a, b *int) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func deref(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}
