package pairing

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestOfferRoundTrip(t *testing.T) {
	in := Offer{
		V:         OfferV1,
		Endpoint:  "https://home.tail1234.ts.net",
		Token:     "abc-def_ghi",
		ExpiresAt: "2026-11-18T12:00:00Z",
		Label:     "laptop",
	}
	line, err := EncodeOffer(in)
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(line, " \n\r\t") {
		t.Fatalf("offer must be one line with no whitespace: %q", line)
	}
	out, err := DecodeOffer(line)
	if err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Fatalf("round trip: %+v want %+v", out, in)
	}
	// Padded input still decodes (a copy through a wrapping terminal).
	padded := base64.URLEncoding.EncodeToString(rawJSON(t, in))
	if _, err := DecodeOffer(padded); err != nil {
		t.Fatalf("padded offer: %v", err)
	}
	// Surrounding whitespace is tolerated (trailing newline from $()).
	if _, err := DecodeOffer("  " + line + "\n"); err != nil {
		t.Fatalf("whitespace-wrapped offer: %v", err)
	}
}

func TestDecodeOfferRejectsBadVersionsExplicitly(t *testing.T) {
	// GDK-433 test ⑦: v != 1 is an explicit refusal naming the version —
	// never a silent ignore, and never an echo of the payload.
	for _, v := range []int{0, 2, 99} {
		line := mustB64(t, Offer{V: v, Endpoint: "https://x", Token: "t"})
		_, err := DecodeOffer(line)
		if err == nil {
			t.Fatalf("v=%d decoded without error", v)
		}
		if !strings.Contains(err.Error(), "version") {
			t.Fatalf("v=%d error must name the version problem: %v", v, err)
		}
	}
}

func TestDecodeOfferRejectsIncomplete(t *testing.T) {
	if _, err := DecodeOffer(mustB64(t, Offer{V: 1, Token: "t"})); err == nil || !strings.Contains(err.Error(), "endpoint") {
		t.Fatalf("missing endpoint: %v", err)
	}
	if _, err := DecodeOffer(mustB64(t, Offer{V: 1, Endpoint: "https://x"})); err == nil || !strings.Contains(err.Error(), "token") {
		t.Fatalf("missing token: %v", err)
	}
	if _, err := DecodeOffer("!!!not-base64!!!"); err == nil {
		t.Fatal("garbage must be refused")
	}
	if _, err := DecodeOffer(""); err == nil {
		t.Fatal("empty must be refused")
	}
}

func TestDecodeOfferErrorsNeverEchoPayload(t *testing.T) {
	// The offer carries the token; an error that quoted the input would
	// leak it into logs. Feed token-bearing garbage and assert silence.
	secret := "SUPERSECRET-TOKEN-VALUE"
	cases := []string{
		mustB64(t, Offer{V: 7, Endpoint: "https://x", Token: secret}),
		mustB64(t, Offer{V: 1, Token: secret}),
		secret,
		"  \n ",
	}
	for _, c := range cases {
		_, err := DecodeOffer(c)
		if err == nil {
			t.Fatalf("case %q unexpectedly decoded", c[:min(len(c), 6)])
		}
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("error leaks the offer payload: %v", err)
		}
	}
}

func TestFormatExpiry(t *testing.T) {
	want := "2026-11-18T12:00:00Z"
	got := FormatExpiry(time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC))
	if got != want {
		t.Fatalf("FormatExpiry = %q, want %q", got, want)
	}
	if FormatExpiry(time.Time{}) != "" {
		t.Fatal("zero time must format as empty")
	}
}

func mustB64(t *testing.T, o Offer) string {
	t.Helper()
	return base64.RawURLEncoding.EncodeToString(rawJSON(t, o))
}

func rawJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
