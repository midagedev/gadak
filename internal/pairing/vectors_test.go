package pairing

// Golden vectors for the one-line pairing offer, shared with the mobile app
// TypeScript decoder (mobile/src/lib/offer.ts). Both decoders read the exact
// same file — internal/pairing/testdata/offer-vectors.json — so a contract
// change on either side turns both suites red.
//
// Regenerate after changing the vector set (never hand-edit the JSON):
//
//	go test ./internal/pairing/ -run TestRegenerateOfferVectors -update
//
// The strings are produced by EncodeOffer itself, so the file can never drift
// from what the encoder emits.

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type offerVector struct {
	Name string `json:"name"`
	// Offer is the one-line encoded form under test.
	Offer string `json:"offer"`
	// Want is the expected decode for valid cases (nil for invalid ones).
	Want *Offer `json:"want,omitempty"`
	// ErrorContains is the substring the decode error must carry for invalid
	// cases. Error text is a contract here: the mobile decoder mirrors the
	// exact phrasing so the same substring matches on both sides.
	ErrorContains string `json:"error_contains,omitempty"`
	// Note names the defense class the case belongs to.
	Note string `json:"note,omitempty"`
}

type offerVectors struct {
	Comment string        `json:"comment"`
	Valid   []offerVector `json:"valid"`
	Invalid []offerVector `json:"invalid"`
}

var vectorUpdate = flag.Bool("update", false, "rewrite testdata/offer-vectors.json")

func TestOfferVectorsValid(t *testing.T) {
	for _, c := range loadOfferVectors(t).Valid {
		t.Run(c.Name, func(t *testing.T) {
			got, err := DecodeOffer(c.Offer)
			if err != nil {
				t.Fatalf("valid case %s failed to decode: %v", c.Name, err)
			}
			if c.Want == nil {
				t.Fatalf("valid case %s has no want value", c.Name)
			}
			if !reflect.DeepEqual(got, normalizeVectorWant(*c.Want)) {
				t.Fatalf("case %s decoded %+v, want %+v", c.Name, got, normalizeVectorWant(*c.Want))
			}
		})
	}
}

func TestOfferVectorsInvalid(t *testing.T) {
	for _, c := range loadOfferVectors(t).Invalid {
		t.Run(c.Name, func(t *testing.T) {
			_, err := DecodeOffer(c.Offer)
			if err == nil {
				t.Fatalf("invalid case %s decoded without error", c.Name)
			}
			if !strings.Contains(err.Error(), c.ErrorContains) {
				t.Fatalf("case %s error %q does not contain %q", c.Name, err.Error(), c.ErrorContains)
			}
		})
	}
}

// TestOfferVectorsHostileHugeLabel covers the hostile class that stays inside
// the contract: an enormous but well-formed payload decodes on both sides
// (the Go parser has no cap). The paste surface bounds input length at the UI
// layer, not in the decoder — the two decoders must agree here.
func TestOfferVectorsHostileHugeLabel(t *testing.T) {
	line, err := EncodeOffer(Offer{
		V:        OfferV1,
		Endpoint: "https://home.example.ts.net",
		Token:    "t",
		Label:    strings.Repeat("x", 100*1024),
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeOffer(line)
	if err != nil {
		t.Fatalf("huge label must decode: %v", err)
	}
	if len(got.Label) != 100*1024 {
		t.Fatalf("huge label decode lost bytes: %d", len(got.Label))
	}
}

// normalizeVectorWant gives a want value the same normalization a decode
// applies: a v1 want's single token becomes its one-entry list, so the
// stored v1 wants keep expressing the payload (one token, no list) while
// comparing equal to the decoded offer.
func normalizeVectorWant(w Offer) Offer {
	if w.V == OfferV1 && w.Token != "" && w.Tokens == nil {
		w.Tokens = []OfferToken{{Scope: "", Token: w.Token}}
	}
	return w
}

func loadOfferVectors(t *testing.T) offerVectors {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "offer-vectors.json"))
	if err != nil {
		t.Fatalf("read offer vectors: %v", err)
	}
	var v offerVectors
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("parse offer vectors: %v", err)
	}
	if len(v.Valid) == 0 || len(v.Invalid) == 0 {
		t.Fatal("offer vectors file is empty — regenerate it (see file comment)")
	}
	return v
}

// TestRegenerateOfferVectors rewrites testdata/offer-vectors.json from the
// case table below. Skipped unless -update is passed so a plain `go test`
// only consumes the file.
func TestRegenerateOfferVectors(t *testing.T) {
	if !*vectorUpdate {
		t.Skip("pass -update to regenerate testdata/offer-vectors.json")
	}
	type extra struct {
		Offer
		Scope  string `json:"scope,omitempty"`
		Future bool   `json:"future,omitempty"`
	}
	full := Offer{
		V:         OfferV1,
		Endpoint:  "https://home.example.ts.net",
		Token:     "offer-vector-token-full",
		ExpiresAt: "2027-06-30T09:00:00Z",
		Label:     "laptop",
	}
	minimal := Offer{
		V:        OfferV1,
		Endpoint: "http://127.0.0.1:7899",
		Token:    "offer-vector-token-minimal",
	}
	aging := Offer{
		V:        OfferV1,
		Endpoint: "https://home.example.ts.net",
		Token:    "offer-vector-token-aging",
	}
	// Unknown keys ride along in the document; the decoder must ignore them
	// so a newer home can hand an older phone an offer it still understands.
	agingDoc, err := json.Marshal(extra{Offer: aging, Scope: "serve", Future: true})
	if err != nil {
		t.Fatal(err)
	}
	versioned, err := json.Marshal(struct {
		V        int    `json:"v"`
		Endpoint string `json:"endpoint"`
		Token    string `json:"token"`
	}{V: 3, Endpoint: "https://home.example.ts.net", Token: "offer-vector-token-v3"})
	if err != nil {
		t.Fatal(err)
	}
	// A document cut mid-string: base64 survives, the JSON does not.
	truncated := `{"v":1,"endpoint":"https://home.example.ts.net","token":"offer-vec`
	// v2 (GDK-1498): one token per scope, no top-level token. Two scopes
	// is the phone's shape (serve + terminal); three adds origin in
	// canonical order. The invalid documents are raw JSON because the
	// struct+encoder refuse to produce them — that refusal is the product
	// behavior; these vectors prove the decoder says the same thing when
	// handed one anyway.
	v2Two := Offer{
		V:         OfferV2,
		Endpoint:  "https://home.example.ts.net",
		ExpiresAt: "2027-06-30T09:00:00Z",
		Label:     "phone",
		Tokens: []OfferToken{
			{Scope: ScopeServe, Token: "offer-v2-serve"},
			{Scope: ScopeTerminal, Token: "offer-v2-terminal"},
		},
	}
	v2Three := Offer{
		V:         OfferV2,
		Endpoint:  "https://home.example.ts.net",
		ExpiresAt: "2027-06-30T09:00:00Z",
		Label:     "laptop",
		Tokens: []OfferToken{
			{Scope: ScopeOrigin, Token: "offer-v2-origin"},
			{Scope: ScopeServe, Token: "offer-v2-serve"},
			{Scope: ScopeTerminal, Token: "offer-v2-terminal"},
		},
	}

	out := offerVectors{
		Comment: "Golden vectors for the pairing offer line, shared by internal/pairing (Go) and mobile/src/lib/offer.ts (TypeScript). Regenerate with: go test ./internal/pairing/ -run TestRegenerateOfferVectors -update",
		Valid: []offerVector{
			{Name: "full", Offer: mustEncodeVector(t, full), Want: &full, Note: "all five fields round-trip"},
			{Name: "minimal", Offer: mustEncodeVector(t, minimal), Want: &minimal, Note: "expires_at and label are empty strings"},
			{Name: "aging_unknown_fields", Offer: mustEncodeVectorDoc(t, agingDoc), Want: &aging, Note: "aging: unknown keys from a newer minter are ignored"},
			{Name: "v2_two_scopes", Offer: mustEncodeVector(t, v2Two), Want: &v2Two, Note: "v2: one scan carries the phone's serve and terminal tokens"},
			{Name: "v2_three_scopes", Offer: mustEncodeVector(t, v2Three), Want: &v2Three, Note: "v2: all three scopes, canonical order"},
		},
		Invalid: []offerVector{
			{Name: "whitespace_only", Offer: "  \n\t ", ErrorContains: "pairing offer: empty", Note: "hostile: blank paste"},
			{Name: "forged_base64", Offer: "### not base64 at all ###", ErrorContains: "pairing offer: not base64url", Note: "hostile: characters outside the base64url alphabet"},
			{Name: "truncated_document", Offer: mustEncodeVectorDoc(t, []byte(truncated)), ErrorContains: "pairing offer: malformed document", Note: "corruption: the line was cut mid-document"},
			{Name: "version_3", Offer: mustEncodeVectorDoc(t, versioned), ErrorContains: "pairing offer: version 3 is not supported", Note: "hostile: explicit refusal naming the version, never a best-effort parse"},
			{Name: "missing_endpoint", Offer: mustEncodeVectorDoc(t, []byte(`{"v":1,"token":"offer-vector-token-noep"}`)), ErrorContains: "pairing offer: no endpoint", Note: "corruption: required field absent"},
			{Name: "v2_no_tokens", Offer: mustEncodeVectorDoc(t, []byte(`{"v":2,"endpoint":"https://home.example.ts.net","tokens":[]}`)), ErrorContains: "pairing offer: v2 carries no tokens", Note: "v2 corruption: the list exists but is empty"},
			{Name: "v2_top_level_token", Offer: mustEncodeVectorDoc(t, []byte(`{"v":2,"endpoint":"https://home.example.ts.net","token":"offer-vector-token-stray","tokens":[{"scope":"serve","token":"offer-v2-serve"}]}`)), ErrorContains: "pairing offer: v2 carries its tokens as a list", Note: "v2 corruption: a top-level token is the v1 shape"},
			{Name: "v2_entry_no_token", Offer: mustEncodeVectorDoc(t, []byte(`{"v":2,"endpoint":"https://home.example.ts.net","tokens":[{"scope":"serve"}]}`)), ErrorContains: "pairing offer: a v2 token entry has no token", Note: "v2 corruption: an entry names its scope but carries no credential"},
			{Name: "v2_duplicate_scope", Offer: mustEncodeVectorDoc(t, []byte(`{"v":2,"endpoint":"https://home.example.ts.net","tokens":[{"scope":"serve","token":"offer-v2-serve"},{"scope":"serve","token":"offer-v2-serve-2"}]}`)), ErrorContains: "pairing offer: v2 carries two tokens for scope \"serve\"", Note: "v2 corruption: one scope, two credentials — which one wins must never be a question"},
		},
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	path := filepath.Join("testdata", "offer-vectors.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	t.Logf("rewrote %s", path)
}

func mustEncodeVector(t *testing.T, o Offer) string {
	t.Helper()
	line, err := EncodeOffer(o)
	if err != nil {
		t.Fatal(err)
	}
	return line
}

// mustEncodeVectorDoc encodes a raw JSON document so cases with extra keys or
// truncated bodies can be expressed — the Offer struct alone cannot produce
// either shape.
func mustEncodeVectorDoc(t *testing.T, doc []byte) string {
	t.Helper()
	return base64.RawURLEncoding.EncodeToString(doc)
}
