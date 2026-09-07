package pairing

import (
	"encoding/base64"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestOfferRoundTrip(t *testing.T) {
	in := Offer{
		V:         OfferV1,
		Endpoint:  "https://home.example.com",
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
	// Decode normalizes the v1 token into the list; the expected value
	// gets the same treatment so the round trip compares whole structs.
	want := in
	want.Tokens = []OfferToken{{Scope: "", Token: in.Token}}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("round trip: %+v want %+v", out, want)
	}
	// The v1 payload itself must be untouched: the single-token shape,
	// no "tokens" key — a v1 pairer's promise (GDK-799).
	if doc := string(rawJSON(t, in)); !strings.Contains(doc, `"token":"abc-def_ghi"`) || strings.Contains(doc, "tokens") {
		t.Fatalf("v1 payload grew a tokens list: %s", doc)
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
	// GDK-433 test ⑦: an unknown v is an explicit refusal naming the
	// version — never a silent ignore, and never an echo of the payload.
	// v2 joined the known set with GDK-1498, so the probe climbs to 3.
	for _, v := range []int{0, 3, 99} {
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

// TestDecodeOfferV2Decodes (GDK-1498): a v2 offer carries one token per
// scope and no top-level token. The structural assertions live in
// TestOfferVectorsValid and TestDecodeOfferV2Shapes; this one is the
// FAIL-first core — the pre-change decoder refused v2 by version.
func TestDecodeOfferV2Decodes(t *testing.T) {
	line := mustEncodeVectorDoc(t, []byte(`{"v":2,"endpoint":"https://home.example.ts.net","expires_at":"2027-06-30T09:00:00Z","label":"phone","tokens":[{"scope":"serve","token":"offer-v2-serve"},{"scope":"terminal","token":"offer-v2-term"}]}`))
	if _, err := DecodeOffer(line); err != nil {
		t.Fatalf("v2 offer must decode: %v", err)
	}
}

// TestDecodeOfferV2Shapes pins the normalized list both versions expose:
// v2 keeps its scoped entries in payload order, v1 gains one entry whose
// scope the payload cannot name. Order is preserved — the mint emits
// canonical order, but a consumer must not depend on it.
func TestDecodeOfferV2Shapes(t *testing.T) {
	v2 := mustEncodeVectorDoc(t, []byte(`{"v":2,"endpoint":"https://home.example.ts.net","expires_at":"2027-06-30T09:00:00Z","label":"phone","tokens":[{"scope":"terminal","token":"offer-v2-term"},{"scope":"origin","token":"offer-v2-origin"}]}`))
	got, err := DecodeOffer(v2)
	if err != nil {
		t.Fatal(err)
	}
	wantList := []OfferToken{{Scope: "terminal", Token: "offer-v2-term"}, {Scope: "origin", Token: "offer-v2-origin"}}
	if !reflect.DeepEqual(got.Tokens, wantList) {
		t.Fatalf("v2 token list = %+v, want %+v", got.Tokens, wantList)
	}
	if got.Token != "" {
		t.Fatalf("a decoded v2 offer must not carry a top-level token: %q", got.Token)
	}
	v1 := mustB64(t, Offer{V: OfferV1, Endpoint: "https://home.example.com", Token: "v1-tok", Label: "laptop"})
	got, err = DecodeOffer(v1)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Tokens, []OfferToken{{Scope: "", Token: "v1-tok"}}) {
		t.Fatalf("v1 normalized list = %+v", got.Tokens)
	}
}

// TestOfferMalformedShapeRefusals pins every distinct shape error on both
// the encode and decode paths — one sentence per defect so a bad mint or
// a bad paste names what to redo, and none of them quotes the payload.
func TestOfferMalformedShapeRefusals(t *testing.T) {
	doc := func(body string) string { return mustEncodeVectorDoc(t, []byte(body)) }
	cases := []struct {
		name, line, errContains string
	}{
		{"v2 no tokens", doc(`{"v":2,"endpoint":"https://x","expires_at":"2027-01-01T00:00:00Z","label":"p","tokens":[]}`), "v2 carries no tokens"},
		{"v2 top-level token", doc(`{"v":2,"endpoint":"https://x","expires_at":"2027-01-01T00:00:00Z","label":"p","token":"secret-top","tokens":[{"scope":"serve","token":"s"}]}`), "v2 carries its tokens as a list"},
		{"v2 entry no scope", doc(`{"v":2,"endpoint":"https://x","expires_at":"2027-01-01T00:00:00Z","label":"p","tokens":[{"token":"s"}]}`), "entry has no scope"},
		{"v2 entry no token", doc(`{"v":2,"endpoint":"https://x","expires_at":"2027-01-01T00:00:00Z","label":"p","tokens":[{"scope":"serve"}]}`), "entry has no token"},
		{"v2 duplicate scope", doc(`{"v":2,"endpoint":"https://x","expires_at":"2027-01-01T00:00:00Z","label":"p","tokens":[{"scope":"serve","token":"a"},{"scope":"serve","token":"b"}]}`), `two tokens for scope "serve"`},
		{"v1 with a list", doc(`{"v":1,"endpoint":"https://x","expires_at":"2027-01-01T00:00:00Z","label":"p","token":"s","tokens":[{"scope":"serve","token":"s"}]}`), "v1 carries one token, not a list"},
	}
	for _, tc := range cases {
		_, err := DecodeOffer(tc.line)
		if err == nil || !strings.Contains(err.Error(), tc.errContains) {
			t.Fatalf("%s: decode error %v, want %q", tc.name, err, tc.errContains)
		}
		if strings.Contains(err.Error(), "secret-") {
			t.Fatalf("%s: error leaks a token value: %v", tc.name, err)
		}
	}
	// The encode side refuses the same shapes before a line exists.
	if _, err := EncodeOffer(Offer{V: OfferV1, Endpoint: "https://x", Tokens: []OfferToken{{Scope: "serve", Token: "t"}}}); err == nil || !strings.Contains(err.Error(), "not a list") {
		t.Fatalf("encode v1 with list: %v", err)
	}
	if _, err := EncodeOffer(Offer{V: OfferV2, Endpoint: "https://x", Token: "t", Tokens: []OfferToken{{Scope: "serve", Token: "t"}}}); err == nil || !strings.Contains(err.Error(), "v1 shape") {
		t.Fatalf("encode v2 with top-level token: %v", err)
	}
	if _, err := EncodeOffer(Offer{V: OfferV2, Endpoint: "https://x", Tokens: nil}); err == nil || !strings.Contains(err.Error(), "v2 carries no tokens") {
		t.Fatalf("encode v2 without tokens: %v", err)
	}
	if _, err := EncodeOffer(Offer{V: OfferV2, Endpoint: "https://x", Tokens: []OfferToken{{Scope: "serve", Token: "a"}, {Scope: "serve", Token: "b"}}}); err == nil || !strings.Contains(err.Error(), "two tokens for scope") {
		t.Fatalf("encode v2 duplicate scope: %v", err)
	}
}

// TestOfferConsumerToken pins the picker a pairing consumer uses: serve
// before origin, v1 as-is, terminal never — a terminal-only offer is
// refused naming the scope.
func TestOfferConsumerToken(t *testing.T) {
	v1, err := EncodeOffer(Offer{V: OfferV1, Endpoint: "https://x", Token: "v1-tok", Label: "laptop"})
	if err != nil {
		t.Fatal(err)
	}
	o, err := DecodeOffer(v1)
	if err != nil {
		t.Fatal(err)
	}
	if tok, err := o.ConsumerToken(); err != nil || tok != "v1-tok" {
		t.Fatalf("v1 consumer token = %q, %v", tok, err)
	}
	serve := mustEncodeVectorDoc(t, []byte(`{"v":2,"endpoint":"https://x","expires_at":"2027-01-01T00:00:00Z","label":"p","tokens":[{"scope":"origin","token":"o"},{"scope":"serve","token":"s"},{"scope":"terminal","token":"t"}]}`))
	o, err = DecodeOffer(serve)
	if err != nil {
		t.Fatal(err)
	}
	if tok, err := o.ConsumerToken(); err != nil || tok != "s" {
		t.Fatalf("v2 consumer token = %q, %v — serve wins", tok, err)
	}
	originOnly := mustEncodeVectorDoc(t, []byte(`{"v":2,"endpoint":"https://x","expires_at":"2027-01-01T00:00:00Z","label":"p","tokens":[{"scope":"origin","token":"o"}]}`))
	o, err = DecodeOffer(originOnly)
	if err != nil {
		t.Fatal(err)
	}
	if tok, err := o.ConsumerToken(); err != nil || tok != "o" {
		t.Fatalf("origin-only consumer token = %q, %v", tok, err)
	}
	termOnly := mustEncodeVectorDoc(t, []byte(`{"v":2,"endpoint":"https://x","expires_at":"2027-01-01T00:00:00Z","label":"p","tokens":[{"scope":"terminal","token":"t"}]}`))
	o, err = DecodeOffer(termOnly)
	if err != nil {
		t.Fatal(err)
	}
	_, err = o.ConsumerToken()
	if err == nil || !strings.Contains(err.Error(), "terminal") {
		t.Fatalf("terminal-only offer must be refused naming the scope: %v", err)
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
