package config

/*
 * GDK-852: the ui.tokens.dim-catalog read-only discovery path — the
 * dimension sibling of ui.tokens.catalog. All assertions go through the
 * setting's Get marshaled to JSON (exactly what the CLI prints), so the
 * tests compile against the pre-change source and FAIL-first is a real
 * run, not a compile error:
 *
 *	contract                              assertion
 *	────────────────────────────────────  ─────────────────────────────────────
 *	path, read-only refusal, state-free   TestSettingDimCatalogReadOnlyPath
 *	19+1 enumeration, axes, locked tier   TestDimCatalogEnumeration
 *	entry JSON shape (null vs omitted)    TestDimCatalogJSONShape
 *	relations ↔ ValidateDimensions sync   TestDimCatalogRelationsMatchValidation
 *
 * Ground truth comes from tokencheck/dim-catalog.json read off disk (the
 * file tokencheck embeds) and from tokencheck's exported lookups, never
 * from the production parse under test.
 */

import (
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config/tokencheck"
)

// diskDimCatalog parses the generated dim catalog tokencheck embeds.
func diskDimCatalog(t *testing.T) (axes []string, toks map[string]map[string]tokencheck.DimToken) {
	t.Helper()
	b, err := os.ReadFile("tokencheck/dim-catalog.json")
	if err != nil {
		t.Fatalf("read tokencheck/dim-catalog.json: %v", err)
	}
	var file struct {
		Axes []struct {
			ID     string                         `json:"id"`
			Tokens map[string]tokencheck.DimToken `json:"tokens"`
		} `json:"axes"`
	}
	if err := json.Unmarshal(b, &file); err != nil {
		t.Fatalf("parse dim catalog: %v", err)
	}
	toks = map[string]map[string]tokencheck.DimToken{}
	for _, ax := range file.Axes {
		axes = append(axes, ax.ID)
		toks[ax.ID] = ax.Tokens
	}
	return axes, toks
}

// dimCatalogJSON returns the discovery output exactly the way the CLI
// prints it: json.Marshal of the setting's Get on a zero config.
func dimCatalogJSON(t *testing.T) string {
	t.Helper()
	s, ok := SettingByPath("ui.tokens.dim-catalog")
	if !ok {
		t.Fatal("ui.tokens.dim-catalog missing from the settings catalog")
	}
	b, err := json.Marshal(s.Get(&Config{}))
	if err != nil {
		t.Fatalf("marshal dim catalog: %v", err)
	}
	return string(b)
}

func TestSettingDimCatalogReadOnlyPath(t *testing.T) {
	s, ok := SettingByPath("ui.tokens.dim-catalog")
	if !ok {
		t.Fatal("ui.tokens.dim-catalog missing from the settings catalog")
	}
	if s.Root != "ui" {
		t.Errorf("Root = %q, want ui", s.Root)
	}
	if !strings.Contains(s.Description, "dimension") || !strings.Contains(s.Description, "ui.tokens") {
		t.Errorf("Description must teach what it lists and the writable sibling: %q", s.Description)
	}
	// Config-state-free: user overrides must not change the catalog.
	plain, err := json.Marshal(s.Get(&Config{}))
	if err != nil {
		t.Fatal(err)
	}
	withOverrides, err := json.Marshal(s.Get(&Config{UI: &UIConfig{Tokens: &UITokens{
		Spacing: map[string]string{"row": "56px"},
	}}}))
	if err != nil {
		t.Fatal(err)
	}
	if string(plain) != string(withOverrides) {
		t.Errorf("catalog must not reflect stored overrides:\n%s\n%s", plain, withOverrides)
	}
	// Set refuses read-only, pointing at the writable sibling — same
	// shape as the color catalog's refusal.
	err = s.Set(&Config{}, json.RawMessage(`[]`))
	if err == nil || !strings.Contains(err.Error(), "read-only") || !strings.Contains(err.Error(), "set ui.tokens instead") {
		t.Fatalf("Set error = %v, want read-only refusal pointing at ui.tokens", err)
	}
}

func TestDimCatalogEnumeration(t *testing.T) {
	out := dimCatalogJSON(t)
	diskAxes, diskToks := diskDimCatalog(t)
	var got []map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not a JSON array of entries: %v", err)
	}
	// 19 recordable + 1 locked, pinned per axis — a catalog that grows
	// must update these counts consciously (and the doc table).
	wantAxisCounts := map[string]int{"spacing": 4, "layout": 8, "type": 8}
	if len(got) != 20 {
		t.Fatalf("catalog lists %d tokens, want 20 (19 recordable + locked docked-min)", len(got))
	}
	// The two embeds read one file: the disk axes are tokencheck's axes.
	if strings.Join(diskAxes, ",") != strings.Join(tokencheck.DimAxes(), ",") {
		t.Fatalf("disk catalog axes %v != DimAxes() %v", diskAxes, tokencheck.DimAxes())
	}
	axisSeen := map[string]int{}
	axisNames := map[string][]string{}
	lockedSeen := 0
	for i, e := range got {
		axis, _ := e["axis"].(string)
		name, _ := e["name"].(string)
		axisSeen[axis]++
		axisNames[axis] = append(axisNames[axis], name)
		disk, inDisk := diskToks[axis][name]
		if !inDisk {
			t.Fatalf("entry %d %s.%s is not in the catalog tokencheck embeds", i, axis, name)
		}
		// Every served field is tokencheck's canonical lookup, which in
		// turn equals the disk entry — the embedded name list contributed
		// names only.
		canon, ok := tokencheck.DimTokenOf(axis, name)
		if !ok {
			t.Fatalf("DimTokenOf(%q, %q) missing", axis, name)
		}
		if canon.CSSVar != disk.CSSVar || canon.Default != disk.Default || canon.Tier != disk.Tier ||
			canon.Unit != disk.Unit || !dimBoundEqual(canon.Min, disk.Min) || !dimBoundEqual(canon.Max, disk.Max) {
			t.Fatalf("%s.%s: DimTokenOf disagrees with the disk catalog: %+v vs %+v", axis, name, canon, disk)
		}
		if e["cssVar"] != canon.CSSVar || e["default"] != canon.Default || e["tier"] != canon.Tier || e["unit"] != canon.Unit {
			t.Errorf("%s.%s: entry %+v does not serve tokencheck's fields %+v", axis, name, e, canon)
		}
		if !dimJSONBoundEqual(t, e["min"], canon.Min) || !dimJSONBoundEqual(t, e["max"], canon.Max) {
			t.Errorf("%s.%s: bounds %v/%v do not match tokencheck %v/%v", axis, name, e["min"], e["max"], canon.Min, canon.Max)
		}
		switch tier := e["tier"]; tier {
		case "locked":
			// Same list, tier-marked — the color catalog's locked notation.
			lockedSeen++
			if name != "docked-min" {
				t.Errorf("unexpected locked token %s.%s", axis, name)
			}
		case "validated-range":
		default:
			t.Errorf("%s.%s: tier %q is neither locked nor validated-range", axis, name, tier)
		}
	}
	for ax, n := range wantAxisCounts {
		if axisSeen[ax] != n {
			t.Errorf("axis %s lists %d tokens, want %d", ax, axisSeen[ax], n)
		}
	}
	if lockedSeen != 1 {
		t.Errorf("catalog lists %d locked tokens, want exactly 1 (docked-min)", lockedSeen)
	}
	// Names sorted within each axis, axes in DimAxes() order: the first
	// entry is pinned so field order is part of the contract too.
	if !strings.HasPrefix(out, `[{"axis":"spacing","name":"control"`) {
		t.Errorf("output must start with the first spacing entry, got: %.120s", out)
	}
	for ax, names := range axisNames {
		sorted := append([]string(nil), names...)
		sort.Strings(sorted)
		if !reflect.DeepEqual(names, sorted) {
			t.Errorf("axis %s names not sorted: %v", ax, names)
		}
	}
}

func TestDimCatalogJSONShape(t *testing.T) {
	out := dimCatalogJSON(t)
	// One pinned entry per shape class: relation-free px with both
	// bounds, relation-owned lower bound (min null), the locked token
	// (both bounds null), and a unitless line-height. relations is always
	// present ([] when empty) like the color catalog's rules field;
	// absent bounds stay null, mirroring the embedded DimToken.
	pins := []string{
		`{"axis":"layout","name":"list-min","cssVar":"--layout-list-min","tier":"validated-range","unit":"px","default":"390px","min":320,"max":480,"relations":[]}`,
		`{"axis":"spacing","name":"row-excerpt","cssVar":"--spacing-row-excerpt","tier":"validated-range","unit":"px","default":"59px","min":null,"max":72,"relations":["--spacing-row-excerpt must be ≥ --spacing-row + 8px (a row carrying a preview line needs headroom)"]}`,
		`{"axis":"layout","name":"docked-min","cssVar":"--layout-docked-min","tier":"locked","unit":"px","default":"1100px","min":null,"max":null,"relations":[]}`,
		`{"axis":"type","name":"body-line-height","cssVar":"--text-body--line-height","tier":"validated-range","unit":"none","default":"1.4","min":1.15,"max":1.6,"relations":[]}`,
	}
	for _, p := range pins {
		if !strings.Contains(out, p) {
			t.Errorf("shape pin absent from output:\n  want substring: %s\n  got: %s", p, out)
		}
	}
}

// TestDimCatalogRelationsMatchValidation is the sync test between the
// discovery output's relation sentences and the rules tokencheck actually
// enforces. tokencheck keeps its relation table unexported, so the sync is
// behavioral: every unordered pair of recordable tokens is overridden at
// its range corners and every relation that CAN fire does. A relation
// added, removed, or re-paired in tokencheck moves the fired set and
// fails here; a sentence invented or dropped in the discovery output
// fails the sentence pins.
func TestDimCatalogRelationsMatchValidation(t *testing.T) {
	// The eight cross-token rules, pinned as axis → sorted "a|b" pairs.
	want := map[string]map[string]bool{
		"spacing": {
			"control|control-sm": true,
			"row|row-excerpt":    true,
		},
		"layout": {
			"detail-max|detail-min":  true,
			"overlay-max|shell-max":  true,
			"sidebar|sidebar-narrow": true,
		},
		"type": {
			"body|micro":    true,
			"body|title":    true,
			"heading|title": true,
		},
	}
	// overlay-max ≤ shell-max is the one pinned pair no valid input can
	// violate (their ranges 360–720 and 1600–2800 never overlap): it is
	// documented defense-in-depth, so its sentence stays while the fired
	// set stays at seven.
	const unfirable = "layout/overlay-max|shell-max"

	_, toks := diskDimCatalog(t)
	observed := map[string]map[string]bool{}
	for axis, axisToks := range toks {
		names := make([]string, 0, len(axisToks))
		for name, tok := range axisToks {
			if tok.Tier != "locked" { // locked writes never reach relations
				names = append(names, name)
			}
		}
		sort.Strings(names)
		for i := 0; i < len(names); i++ {
			for j := i + 1; j < len(names); j++ {
				x, y := names[i], names[j]
				lx, hx := dimCornerValues(t, axisToks[x])
				ly, hy := dimCornerValues(t, axisToks[y])
				for _, m := range []map[string]string{
					{x: lx, y: hy},
					{x: hx, y: ly},
				} {
					for _, v := range tokencheck.ValidateDimensions(map[string]map[string]string{axis: m}) {
						if v.Rule != "relation" {
							continue
						}
						// dimBlame joins both names with "/" only when
						// both sides are overridden — exactly the pair
						// under test. Single-name blames come from
						// relations whose other participant stayed at its
						// default; that relation's own pair combo covers
						// it below.
						parts := strings.Split(v.Token, "/")
						if len(parts) != 2 {
							continue
						}
						sort.Strings(parts)
						pair := parts[0] + "|" + parts[1]
						if observed[axis] == nil {
							observed[axis] = map[string]bool{}
						}
						observed[axis][pair] = true
					}
				}
			}
		}
	}
	fired := 0
	for _, pairs := range observed {
		fired += len(pairs)
	}
	if fired != 7 {
		t.Fatalf("brute force fired %d distinct relations, want 7 — tokencheck's rule set moved; re-pin consciously", fired)
	}
	for axis, pairs := range observed {
		for pair := range pairs {
			if !want[axis][pair] {
				t.Errorf("ValidateDimensions fires %s.%s, which is not pinned — add it or re-pin", axis, pair)
			}
		}
	}
	for axis, pairs := range want {
		for pair := range pairs {
			if axis+"/"+pair == unfirable {
				continue
			}
			if !observed[axis][pair] {
				t.Errorf("pinned %s.%s never fires — stale pin or a softened rule in tokencheck", axis, pair)
			}
		}
	}

	// The discovery output states exactly the pinned set: eight unique
	// sentences, each opening with one participant's cssVar and naming
	// the partner, with the operator and +add the rule uses. Prefix pins
	// keep operator, partner and add honest in one place.
	var entries []map[string]any
	if err := json.Unmarshal([]byte(dimCatalogJSON(t)), &entries); err != nil {
		t.Fatalf("catalog output: %v", err)
	}
	sentencePins := []string{
		"--spacing-control-sm must stay ≤ --spacing-control (",
		"--spacing-row-excerpt must be ≥ --spacing-row + 8px (",
		"--layout-detail-max must be ≥ --layout-detail-min (",
		"--layout-overlay-max must stay ≤ --layout-shell-max (",
		"--layout-sidebar-narrow must stay ≤ --layout-sidebar (",
		"--text-body must be ≥ --text-micro + 2px (",
		"--text-title must be ≥ --text-body + 2px (",
		"--text-heading must be ≥ --text-title + 2px (",
	}
	sentences := map[string]bool{}
	for _, e := range entries {
		rels, _ := e["relations"].([]any)
		for _, r := range rels {
			s, _ := r.(string)
			if s == "" {
				t.Fatalf("entry %+v carries a non-string relation %v", e, r)
			}
			sentences[s] = true
		}
	}
	if len(sentences) != len(sentencePins) {
		t.Fatalf("catalog carries %d unique relation sentences, want %d: %v", len(sentences), len(sentencePins), sentences)
	}
	for _, pin := range sentencePins {
		hit := false
		for s := range sentences {
			if strings.HasPrefix(s, pin) {
				hit = true
				break
			}
		}
		if !hit {
			t.Errorf("no sentence starts with the pinned rule %q — operator, partner or add drifted: %v", pin, sentences)
		}
	}
	// Every sentence names two known cssVars of one axis, and the pair it
	// names is pinned (no invented relations riding along).
	cssVarAxis := map[string]string{}
	for axis, axisToks := range toks {
		for _, tok := range axisToks {
			cssVarAxis[tok.CSSVar] = axis
		}
	}
	for s := range sentences {
		first := ""
		for _, v := range []string{"--spacing-", "--layout-", "--text-"} {
			if strings.HasPrefix(s, v) {
				// up to the space after the var name
				first = s[:strings.IndexByte(s, ' ')]
				break
			}
		}
		if first == "" {
			t.Errorf("sentence does not open with a known cssVar: %q", s)
			continue
		}
		rest := strings.TrimPrefix(s, first+" ")
		partner := ""
		for v := range cssVarAxis {
			if v != first && strings.HasPrefix(rest, "must ") && strings.Contains(rest, v) {
				partner = v
				break
			}
		}
		if partner == "" {
			t.Errorf("sentence names no partner cssVar: %q", s)
			continue
		}
		if cssVarAxis[first] != cssVarAxis[partner] {
			t.Errorf("relation crosses axes: %q", s)
		}
	}
}

// dimCornerValues returns range-valid low/high values for a token so a
// brute-forced pair can push a relation apart: low is the catalog min (1
// for the min-less px tokens, whose lower bound is relation-owned), high
// is the catalog max.
func dimCornerValues(t *testing.T, tok tokencheck.DimToken) (low, high string) {
	t.Helper()
	lowV, highV := 1.0, 0.0
	if tok.Min != nil {
		lowV = *tok.Min
	}
	if tok.Max != nil {
		highV = *tok.Max
	}
	if highV <= lowV {
		t.Fatalf("token %s has no usable range for corner values", tok.CSSVar)
	}
	return dimTestValue(tok.Unit, lowV), dimTestValue(tok.Unit, highV)
}

func dimTestValue(unit string, v float64) string {
	s := strconv.FormatFloat(v, 'f', -1, 64)
	if unit == "none" {
		return s
	}
	return s + "px"
}

func dimBoundEqual(a, b *float64) bool {
	if (a == nil) != (b == nil) {
		return false
	}
	return a == nil || *a == *b
}

func dimJSONBoundEqual(t *testing.T, got any, want *float64) bool {
	t.Helper()
	if want == nil {
		return got == nil
	}
	g, ok := got.(float64)
	return ok && g == *want
}
