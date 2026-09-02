package tokencheck

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
)

/*
 * Contract ↔ assertion map.
 *
 * Every clause below cites where it comes from — the gate that already
 * protects app.css (tools/theme-check.mjs, line numbers at the time of
 * writing) or the GDK-769 axis-3 input-defense contract. No floor exists
 * here that theme-check does not already assert; where theme-check is
 * silent, this package stays silent (hex validity only) and says so.
 *
 *	C1  Color math parity — all 297 golden vectors from
 *	    testdata/token-vectors.json (emitted by theme-check --emit-vectors
 *	    from the same formulas) agree with this package to 4 decimals.
 *	    Pins: sRGB transfer/gamma, OKLab coefficients, ΔEok, WCAG contrast,
 *	    and the Machado matrix orientation (a transposed copy fails the 30
 *	    deut vectors — gray-on-gray outputs coincide, colored ones don't).
 *	C2  locked tier warns and carries (GDK-858, user decision 2026-08-25:
 *	    "warnings, not refusals, for anything judgment-shaped") — the 10
 *	    tokens theme-check's palette-structure assertions are written
 *	    against (GROUNDS theme-check.mjs:107, FLOOR :108, search-match
 *	    isolation :420-423, shell ladder :222). Reject stays reserved for
 *	    what a machine can keep: parse/shape failures. Assert: warn
 *	    severity ×2 (a ground and a text token) + all 10 names warn (bulk).
 *	C3  Violations carry numbers — Measured and Floor are populated for
 *	    status-role-floor (theme-check.mjs:281/:447 `floor 3/4.5`), so an
 *	    agent can self-correct without a round trip. Assert: measured parses
 *	    as ≥0 float below floor value, floor string present, and the same
 *	    for the ΔEok pair rules (:353/:356 format `ΔEok 0.049 < 0.05`).
 *	C4  free tier accepts any valid hex (lozenge/avatar/dept chips, scrim,
 *	    scrollbar-hover — tokens theme-check asserts nothing about).
 *	    Assert: a valid odd hex passes with empty result ×2 different
 *	    names, and an invalid hex still rejects (free ≠ unvalidated).
 *	C5  Hex input defense (GDK-769 axis 3, class: corrupt/malicious
 *	    values). Assert: truncated/5/7/8-digit/mixed-garbage/`javascript:`
 *	    scheme/`rgb()` functional/giant 10k string/unicode digit forms all
 *	    REJECT as hex with no panic; 3-digit and case-insensitive 6-digit
 *	    ACCEPT; `#abc` ≡ `#aabbcc` numerically (3-digit expansion).
 *	C6  Partial-override context fill — a one-token status override is
 *	    judged against the FULL effective palette (base fill), matching how
 *	    it renders. Assert: status-stale := its own value on its own base
 *	    (light and dark both) → 0 violations; and the discriminating case —
 *	    the dark stale ink on the light base IS rejected (2.14 < 4.5 on
 *	    bg-base), proving the base is actually consulted, not decoration.
 *	C7  Unknown-token tolerance (GDK-769 axis 3, class: stale schema).
 *	    Assert: a name absent from the catalog yields exactly one WARN
 *	    "unknown-token", is otherwise ignored (no reject, no panic); the
 *	    `--color-` CSS variable spelling normalizes to the bare name (both
 *	    accepted, same tier lookup).
 *	C8  Catalog shape — the embedded catalog.json is the single tier
 *	    source. Assert: 60 tokens, tier counts 10/12/38 (44 → 60 when
 *	    GDK-1358 added the sixteen --color-ansi-* terminal tokens, all
 *	    free tier — no floor is written against them), exact locked and
 *	    validated name sets, tier ∈ {free,validated,locked}, every rules
 *	    entry ∈ implementedRules, palettes == [light dark ember ink],
 *	    cssVar == "--color-"+name, and every value parses as hex except the
 *	    alpha-form scrim (which rules never read).
 *	C9  Determinism — same inputs, same output, independent of map
 *	    insertion order. Assert: two runs equal; a differently-built map
 *	    (different insertion order) equal; sorted-name output order.
 *	C10 base-incomplete tolerance — group rules need the base palette;
 *	    a missing base must degrade to a WARN, never a panic or a silent
 *	    pass. Assert: status override + empty base → warn "base-incomplete"
 *	    and no reject for the valid hex; nil maps → nil result.
 *
 * Self-review of the three defect classes the spec names:
 *
 *	gamma handling   — lin() is pinned byte-for-byte by 191 contrast + 30
 *	                   oklab vectors (C1); a 2.2 shortcut instead of the
 *	                   sRGB curve moves black/white from 21.0 to ~18.6 and
 *	                   fails immediately.
 *	3-digit hex      — expanded by doubling each digit (C5 asserts #abc ≡
 *	                   #aabbcc through Contrast); the mjs parser and this
 *	                   gate share the rule.
 *	deut matrix      — orientation (row-vector vs transposed application)
 *	                   is pinned by the 30 deut vectors; the matrix also
 *	                   has ≈1.0 row sums so grays map to grays, which a
 *	                   transposed copy also satisfies — the colored
 *	                   vectors are what discriminate (C1).
 */

//go:embed testdata/token-vectors.json
var vectorsJSON []byte

type vectorsFile struct {
	Contrast []struct {
		Fg       string  `json:"fg"`
		Bg       string  `json:"bg"`
		Contrast float64 `json:"contrast"`
	} `json:"contrast"`
	DeltaEok []struct {
		A                    string  `json:"a"`
		B                    string  `json:"b"`
		DeltaEok             float64 `json:"deltaEok"`
		DeltaEokDeuteranopia float64 `json:"deltaEokDeuteranopia"`
	} `json:"deltaEok"`
	Oklab []struct {
		Hex string  `json:"hex"`
		L   float64 `json:"L"`
		A   float64 `json:"a"`
		B   float64 `json:"b"`
	} `json:"oklab"`
	Deut []struct {
		Hex          string `json:"hex"`
		Deuteranopia string `json:"deuteranopia"`
	} `json:"deut"`
}

func loadVectors(t *testing.T) vectorsFile {
	t.Helper()
	var v vectorsFile
	if err := json.Unmarshal(vectorsJSON, &v); err != nil {
		t.Fatalf("parse testdata/token-vectors.json: %v", err)
	}
	return v
}

const vecTol = 1e-4 // the vectors' own note says: compare to 4 decimals

func assertClose(t *testing.T, label string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > vecTol {
		t.Errorf("%s: got %.6f, want %.6f (diff %.2e > %.0e)", label, got, want, math.Abs(got-want), vecTol)
	}
}

// lightBase returns the light palette as a base map, the way a caller that
// read it from the embedded catalog would pass it.
func lightBase(t *testing.T) map[string]string {
	t.Helper()
	base := map[string]string{}
	for _, tok := range CatalogTokens() {
		if v, ok := tok.Values["light"]; ok {
			base[tok.Name] = v
		}
	}
	if len(base) < 44 {
		t.Fatalf("lightBase: only %d tokens with light values — catalog broken?", len(base))
	}
	return base
}

func hasViolation(vs []Violation, rule string) bool {
	for _, v := range vs {
		if v.Rule == rule {
			return true
		}
	}
	return false
}

// ── C1: golden vectors ────────────────────────────────────────────────────

func TestGoldenVectorsContrast(t *testing.T) {
	v := loadVectors(t)
	if len(v.Contrast) < 20 {
		t.Fatalf("contrast vectors: %d < 20", len(v.Contrast))
	}
	for i, c := range v.Contrast {
		assertClose(t, fmt.Sprintf("contrast[%d] %s/%s", i, c.Fg, c.Bg), Contrast(c.Fg, c.Bg), c.Contrast)
	}
}

func TestGoldenVectorsDeltaEok(t *testing.T) {
	v := loadVectors(t)
	if len(v.DeltaEok) < 20 {
		t.Fatalf("deltaEok vectors: %d < 20", len(v.DeltaEok))
	}
	for i, d := range v.DeltaEok {
		assertClose(t, fmt.Sprintf("dEok[%d] %s/%s", i, d.A, d.B), DEok(d.A, d.B), d.DeltaEok)
		assertClose(t, fmt.Sprintf("dEok-deut[%d] %s/%s", i, d.A, d.B), DEok(Deut(d.A), Deut(d.B)), d.DeltaEokDeuteranopia)
	}
}

func TestGoldenVectorsOklab(t *testing.T) {
	v := loadVectors(t)
	if len(v.Oklab) == 0 {
		t.Fatal("oklab vectors empty")
	}
	for i, o := range v.Oklab {
		lab := hex2oklab(o.Hex)
		assertClose(t, fmt.Sprintf("oklab[%d] %s L", i, o.Hex), lab[0], o.L)
		assertClose(t, fmt.Sprintf("oklab[%d] %s a", i, o.Hex), lab[1], o.A)
		assertClose(t, fmt.Sprintf("oklab[%d] %s b", i, o.Hex), lab[2], o.B)
	}
}

func TestGoldenVectorsDeut(t *testing.T) {
	v := loadVectors(t)
	if len(v.Deut) == 0 {
		t.Fatal("deut vectors empty")
	}
	for i, d := range v.Deut {
		if got := Deut(d.Hex); got != d.Deuteranopia {
			t.Errorf("deut[%d] %s: got %s, want %s — matrix substituted or transposed?", i, d.Hex, got, d.Deuteranopia)
		}
	}
}

// ── C2: locked tier ───────────────────────────────────────────────────────

// TestLockedTokensWarnNotRefuse and TestAllTenLockedNamesWarn are revised
// assertions, not relaxed ones — contract change 2026-08-25, user decision
// GDK-858 ("난 대비는 워닝만 떠야지 거절은 아니라고 생각해. 대비 뿐 아니라
// 전반적으로"): tier membership is a judgment about palette authoring, not a
// machine-checkable property of the value, so a locked override warns and is
// carried. The machine-checkable contract (hex shape, C5) still rejects.
// FAIL-first: both tests fail against the pre-GDK-858 source, which returned
// SeverityReject here (run output in the round report).
func TestLockedTokensWarnNotRefuse(t *testing.T) {
	base := lightBase(t)
	for _, name := range []string{"bg-base", "text-primary"} {
		vs := ValidateTokens(map[string]string{name: "#123456"}, base)
		if len(vs) == 0 {
			t.Fatalf("%s: override produced no verdict at all", name)
		}
		if vs[0].Rule != "locked" || vs[0].Severity != SeverityWarn {
			t.Errorf("%s: got %+v, want rule=locked warn", name, vs[0])
		}
	}
}

func TestAllTenLockedNamesWarn(t *testing.T) {
	base := lightBase(t)
	want := map[string]bool{
		"bg-base": true, "bg-panel": true, "bg-elevated": true, "bg-hover": true,
		"bg-active": true, "text-primary": true, "text-secondary": true,
		"text-muted": true, "search-match": true, "shell": true,
	}
	for name := range want {
		vs := ValidateTokens(map[string]string{name: "#a1b2c3"}, base)
		found := false
		for _, v := range vs {
			if v.Token == name && v.Rule == "locked" && v.Severity == SeverityWarn {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: no locked warn among %+v", name, vs)
		}
	}
}

// ── C2b: judgment rules warn; only parse rejects ─────────────────────────
//
// GDK-858 (user decision 2026-08-25): every rule that judges the LOOK —
// pair separation, its deuteranopia simulation, per-role contrast floors,
// accent-text contrast — warns and carries. The measured number and the
// floor stay in the warning (C3 unchanged), so the fix still needs no
// round trip. Reject survives only where a machine can keep the promise:
// the hex parse (C5). FAIL-first: every case below returned SeverityReject
// on the pre-GDK-858 source.
func TestJudgmentRulesWarnAndCarry(t *testing.T) {
	base := lightBase(t)
	cases := []struct {
		rule string
		in   map[string]string
	}{
		{"status-role-floor", map[string]string{"status-new": "#cfc0a4"}},
		{"status-pair", map[string]string{"status-done": base["status-new"]}},
		{"status-pair-deuteranopia", map[string]string{"status-done": base["status-new"]}},
		{"accent-text-contrast", map[string]string{"accent-text": "#ffffff"}},
	}
	for _, tc := range cases {
		vs := ValidateTokens(tc.in, base)
		found := false
		for _, v := range vs {
			if v.Rule == tc.rule {
				found = true
				if v.Severity != SeverityWarn {
					t.Errorf("%s: severity %q, want warn: %+v", tc.rule, v.Severity, v)
				}
			}
		}
		if !found {
			t.Errorf("%s never fired for %+v: %+v", tc.rule, tc.in, vs)
			continue
		}
		for _, v := range vs {
			if v.Severity == SeverityReject {
				t.Errorf("%s: judgment input produced a reject (%s) — reject is for parse/shape only: %+v", tc.rule, v.Rule, v)
			}
		}
	}
}

// ── C3: violations carry numbers ─────────────────────────────────────────

func TestViolationCarriesNumbers(t *testing.T) {
	base := lightBase(t)
	// status-new light ink fails the text floor on bg-base once dragged
	// toward the ground: a near-ground brown.
	vs := ValidateTokens(map[string]string{"status-new": "#cfc0a4"}, base)
	if !hasViolation(vs, "status-role-floor") {
		t.Fatalf("status-role-floor missing: %+v", vs)
	}
	for _, v := range vs {
		if v.Rule != "status-role-floor" {
			continue
		}
		if v.Measured == "" || v.Floor == "" {
			t.Errorf("role-floor violation without numbers: %+v", v)
		}
		if !strings.Contains(v.Message, v.Measured) {
			t.Errorf("message %q does not carry measured %q", v.Message, v.Measured)
		}
	}
}

func TestPairViolationCarriesNumbers(t *testing.T) {
	base := lightBase(t)
	// A clone of status-new next to the real one: pairwise ΔEok ≈ 0.
	vs := ValidateTokens(map[string]string{"status-done": base["status-new"]}, base)
	if !hasViolation(vs, "status-pair") {
		t.Fatalf("status-pair missing for clone ink: %+v", vs)
	}
	if !hasViolation(vs, "status-pair-deuteranopia") {
		t.Errorf("status-pair-deuteranopia missing for clone ink: %+v", vs)
	}
	for _, v := range vs {
		if v.Rule != "status-pair" && v.Rule != "status-pair-deuteranopia" {
			continue
		}
		if v.Measured == "" || v.Floor == "" {
			t.Errorf("%s violation without numbers: %+v", v.Rule, v)
		}
	}
}

// ── C4: free tier ────────────────────────────────────────────────────────

func TestFreeTierAcceptsAnyValidHex(t *testing.T) {
	base := lightBase(t)
	for _, name := range []string{"lozenge-green", "avatar-3"} {
		vs := ValidateTokens(map[string]string{name: "#123456"}, base)
		if len(vs) != 0 {
			t.Errorf("%s: free-tier override produced %+v", name, vs)
		}
	}
}

func TestFreeTierStillChecksHex(t *testing.T) {
	base := lightBase(t)
	vs := ValidateTokens(map[string]string{"dept-2": "nope"}, base)
	if len(vs) != 1 || vs[0].Rule != "hex" || vs[0].Severity != SeverityReject {
		t.Errorf("free invalid hex: got %+v, want one hex reject", vs)
	}
}

// ── C5: hex input defense ────────────────────────────────────────────────

func TestHexDefense(t *testing.T) {
	base := lightBase(t)
	hostile := []string{
		"",                          // empty
		"#12",                       // truncated
		"#1234",                     // 4 digits
		"#12345",                    // 5 digits
		"#1234567",                  // 7 digits
		"#12345678",                 // 8 digits (alpha — runtime takes none yet)
		"#GGGGGG",                   // hex letters out of range
		"#12 456",                   // inner space
		"123456",                    // missing #
		"javascript:alert(1)",       // scheme payload
		"rgb(1 2 3)",                // functional form
		"#١٢٣٤٥٦",                   // arabic-indic digits
		strings.Repeat("#ff", 4000), // giant string
	}
	for _, val := range hostile {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panic on value %q: %v", truncate(val), r)
				}
			}()
			vs := ValidateTokens(map[string]string{"status-done": val}, base)
			if len(vs) == 0 {
				t.Errorf("value %q accepted", truncate(val))
				return
			}
			if vs[0].Rule != "hex" || vs[0].Severity != SeverityReject {
				t.Errorf("value %q: got %+v, want hex reject", truncate(val), vs[0])
			}
			if len(vs[0].Message) > 200 {
				t.Errorf("message not truncated for hostile input: %d chars", len(vs[0].Message))
			}
		}()
	}
}

func truncate(s string) string {
	if len(s) > 40 {
		return s[:40] + "…"
	}
	return s
}

func TestHexAcceptsShortAndMixedCase(t *testing.T) {
	base := lightBase(t)
	for _, val := range []string{"#abc", "#ABC", "#aAbBcC", " #1c4c31 "} {
		vs := ValidateTokens(map[string]string{"status-done": val}, base)
		for _, v := range vs {
			if v.Rule == "hex" {
				t.Errorf("value %q rejected as hex: %+v", val, v)
			}
		}
	}
}

func TestThreeDigitExpansion(t *testing.T) {
	if !ValidHex("#abc") || !ValidHex("#aabbcc") {
		t.Fatal("baseline: both forms must be valid")
	}
	if Contrast("#abc", "#ffffff") != Contrast("#aabbcc", "#ffffff") {
		t.Error("#abc must expand to #aabbcc (digit doubling), not 0xabc padding")
	}
	if Deut("#f00") != Deut("#ff0000") {
		t.Error("deut: #abc-style expansion inconsistent")
	}
}

// ── C6: partial override with base fill ──────────────────────────────────

func TestPartialOverrideUsesBaseContext(t *testing.T) {
	light := lightBase(t)
	// Its own light value: the effective palette equals shipped light, which
	// theme-check holds green — a same-value override must be free.
	vs := ValidateTokens(map[string]string{"status-stale": light["status-stale"]}, light)
	if len(vs) != 0 {
		t.Errorf("same-value override flagged: %+v", vs)
	}
	// A dark ink judged in a dark base (its own palette): also green.
	dark := paletteBase(t, "dark")
	vs = ValidateTokens(map[string]string{"status-stale": dark["status-stale"]}, dark)
	if len(vs) != 0 {
		t.Errorf("same-value override on dark base flagged: %+v", vs)
	}
	// The discriminating case: that same dark ink on the LIGHT base must be
	// flagged — stale is #d19b5a, a warm mid tone that cannot carry text on
	// #f4efe4. If this passes, the base fill is not being used and every
	// override is being judged in a vacuum. (GDK-858: the flag is a warn
	// now; the firing is the assertion.)
	vs = ValidateTokens(map[string]string{"status-stale": dark["status-stale"]}, light)
	if !hasViolation(vs, "status-role-floor") {
		t.Errorf("dark stale ink accepted on light grounds — base fill ignored: %+v", vs)
	}
}

// paletteBase builds a base map from one catalog palette, like a caller that
// knows the user's active theme would.
func paletteBase(t *testing.T, palette string) map[string]string {
	t.Helper()
	base := map[string]string{}
	for _, tok := range CatalogTokens() {
		if v, ok := tok.Values[palette]; ok {
			base[tok.Name] = v
		}
	}
	return base
}

// ── C7: unknown tokens and name normalization ────────────────────────────

func TestUnknownTokenWarnsAndIsIgnored(t *testing.T) {
	base := lightBase(t)
	vs := ValidateTokens(map[string]string{"status-archived": "#123456", "accent": "#123456"}, base)
	var warns int
	for _, v := range vs {
		if v.Rule == "unknown-token" {
			warns++
			if v.Token != "status-archived" {
				t.Errorf("unknown-token names %q", v.Token)
			}
			if v.Severity != SeverityWarn {
				t.Errorf("unknown-token severity %q, want warn", v.Severity)
			}
		}
	}
	if warns != 1 {
		t.Errorf("unknown-token warnings = %d, want 1: %+v", warns, vs)
	}
	for _, v := range vs {
		if v.Token == "status-archived" && v.Severity == SeverityReject {
			t.Errorf("unknown token REJECTED (breaks stale schemas): %+v", v)
		}
	}
}

func TestCSSVarNameNormalizes(t *testing.T) {
	if tier, ok := TierOf("--color-accent"); !ok || tier != "validated" {
		t.Errorf("TierOf(--color-accent) = %q,%v — prefix not stripped in lookups", tier, ok)
	}
	base := lightBase(t)
	vs := ValidateTokens(map[string]string{"--color-shell": "#123456"}, base)
	if len(vs) == 0 || vs[0].Rule != "locked" {
		t.Errorf("--color-shell override not treated as locked shell: %+v", vs)
	}
}

// ── C8: catalog shape ────────────────────────────────────────────────────

func TestCatalogShape(t *testing.T) {
	toks := CatalogTokens()
	if len(toks) != 60 {
		t.Fatalf("catalog carries %d tokens, want 60", len(toks))
	}
	counts := map[string]int{}
	for _, tok := range toks {
		counts[tok.Tier]++
		if tok.Tier != "free" && tok.Tier != "validated" && tok.Tier != "locked" {
			t.Errorf("%s: tier %q outside whitelist", tok.Name, tok.Tier)
		}
		if tok.CSSVar != "--color-"+tok.Name {
			t.Errorf("%s: cssVar %q", tok.Name, tok.CSSVar)
		}
		for _, r := range tok.Rules {
			if !implementedRules[r] {
				t.Errorf("%s: rule %q not implemented by this package", tok.Name, r)
			}
		}
		for pal, v := range tok.Values {
			if tok.Name == "scrim" {
				continue // alpha form rgb(a/b/c) — rules never read it
			}
			if !ValidHex(v) {
				t.Errorf("%s[%s] = %q is not a hex color", tok.Name, pal, v)
			}
		}
	}
	// free 22 → 38: the sixteen ANSI terminal tokens (GDK-1358) carry no
	// floor of their own and land in the free tier.
	if counts["locked"] != 10 || counts["validated"] != 12 || counts["free"] != 38 {
		t.Errorf("tier counts = %v, want locked 10 / validated 12 / free 38", counts)
	}
	wantLocked := []string{"bg-base", "bg-panel", "bg-elevated", "bg-hover", "bg-active",
		"text-primary", "text-secondary", "text-muted", "search-match", "shell"}
	wantValidated := []string{"accent", "accent-hover", "accent-subtle", "accent-text",
		"border-subtle", "border-strong", "status-new", "status-inprogress", "status-done",
		"status-reopen", "status-stale", "focus-ring"}
	for _, name := range wantLocked {
		if tier, _ := TierOf(name); tier != "locked" {
			t.Errorf("%s tier = %q, want locked", name, tier)
		}
	}
	for _, name := range wantValidated {
		if tier, _ := TierOf(name); tier != "validated" {
			t.Errorf("%s tier = %q, want validated", name, tier)
		}
	}
}

func TestCatalogPalettes(t *testing.T) {
	if got := CatalogPalettes(); !reflect.DeepEqual(got, []string{"light", "dark", "ember", "ink"}) {
		t.Errorf("palettes = %v, want [light dark ember ink]", got)
	}
}

// ── C9: determinism ──────────────────────────────────────────────────────

func TestDeterminism(t *testing.T) {
	base := lightBase(t)
	overrides := map[string]string{
		"status-done": "#8f3530",
		"accent-text": "#734701",
		"dept-2":      "#ff0000",
	}
	a := ValidateTokens(overrides, base)
	b := ValidateTokens(overrides, base)
	if !reflect.DeepEqual(a, b) {
		t.Errorf("two runs differ:\n%+v\n%+v", a, b)
	}
	// Same content built in a different insertion order.
	shuffled := map[string]string{}
	shuffled["dept-2"] = overrides["dept-2"]
	shuffled["accent-text"] = overrides["accent-text"]
	shuffled["status-done"] = overrides["status-done"]
	c := ValidateTokens(shuffled, base)
	if !reflect.DeepEqual(a, c) {
		t.Errorf("insertion order changed the result:\n%+v\n%+v", a, c)
	}
}

// ── C10: incomplete base ─────────────────────────────────────────────────

func TestIncompleteBaseDegradesToWarn(t *testing.T) {
	vs := ValidateTokens(map[string]string{"status-done": "#8f3530"}, map[string]string{})
	if len(vs) == 0 {
		t.Fatal("empty base produced no output at all — group rules silently skipped")
	}
	if !hasViolation(vs, "base-incomplete") {
		t.Errorf("no base-incomplete warn: %+v", vs)
	}
	for _, v := range vs {
		if v.Severity == SeverityReject && v.Rule != "hex" {
			t.Errorf("empty base escalated to reject: %+v", v)
		}
	}
}

func TestNilInputs(t *testing.T) {
	if vs := ValidateTokens(nil, nil); vs != nil {
		t.Errorf("nil overrides = %+v, want nil", vs)
	}
}
