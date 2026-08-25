package config

// User token overrides for the web/desktop UI — colors (GDK-786/791 wave)
// and dimensions (the dim-token wave). The schema lives here; the color
// math, both tier catalogs and the length/range/relation rules live in
// internal/config/tokencheck (GDK-785/787) and this file is its only
// caller.
//
// Three blocks under `ui` in config.json:
//
//	ui.tokens         {colors: {accent: "#7a4bd0", …},        every palette
//	                   spacing: {row: "44px"},
//	                   layout: {sidebar: "280px"},
//	                   type: {body: "14px"}}                   dims: any palette
//	ui.tokensByTheme  {dark: {colors: {accent: "#9a6be0"}}}   colors only
//	ui.dataColors     {label: {urgent: "#c03030"},            per-data-key inks
//	                   type: {10007: "#d07020"},
//	                   status: {inprogress: "#7e5904"}}
//
// Write-time contract (CLI `config set` and PUT settings/ share it) — the
// Severity split, from the user decision of 2026-08-25 ("난 대비는
// 워닝만 떠야지 거절은 아니라고 생각해. 대비 뿐 아니라 전반적으로"):
// REFUSE what a machine can check without taste, WARN about everything
// judgment-shaped and save the value. Concretely:
//   - unparseable values (non-hex colors, non-length dimensions) and wrong
//     shapes (unknown wrapper axis) refuse — a value that cannot parse can
//     never render;
//   - derived tokens (layout.docked-min) refuse — a stored value would be
//     overwritten by the recomputation;
//   - everything else — locked tiers, contrast/ΔEok/deuteranopia floors,
//     dimension ranges and relations — WARNS and saves: the look is the
//     user's, warnings keep the measurements and teach the next move
//     (contrast warnings name the failing palettes and the
//     ui.tokensByTheme scoping fix; type-step warnings list the
//     ladder that moves together).
//   - dimension axes (spacing/layout/type) are palette-agnostic CSS lengths
//     judged by tokencheck.ValidateDimensions. They are refused inside
//     ui.tokensByTheme at the settings layer — a per-palette copy of a
//     palette-free value is dead weight, and docked-min (the derived dock
//     floor) is locked everywhere.
//   - dataColors keys: `label.*` is the label text itself (labels are their
//     own identifiers), `type.*` is a Jira issue_type_id (digits — display
//     names localize per account and are refused), `status.*` is a
//     status_category (new|inprogress|done — never a status display name).
//   - unknown token names and unknown tokensByTheme palettes are CARRIED with
//     a warning, never a panic and never a refused save: a config written by a
//     newer gadak must keep loading (GDK-769 axis 3). The load path
//     (json.Unmarshal into UIConfig) drops nothing it does not understand at
//     the block level; the expansion re-validates and turns drift into
//     advisories instead of a broken boot.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/midagedev/gadak/internal/config/tokencheck"
)

// UITokens is one token override set. Colors are palette-scoped hex
// overrides; the dimension axes (spacing, layout, type) are palette-agnostic
// CSS lengths that ride the same wrapper but never the per-theme overlay.
// Values are bare token names ("accent", "row") or CSS variable names
// ("--color-accent", "--spacing-row") — tokencheck normalizes both.
type UITokens struct {
	Colors  map[string]string `json:"colors,omitempty"`
	Spacing map[string]string `json:"spacing,omitempty"`
	Layout  map[string]string `json:"layout,omitempty"`
	Type    map[string]string `json:"type,omitempty"`
}

// UIConfig is the `ui` block of config.json. Nil means "nothing overridden"
// and is not written.
type UIConfig struct {
	Tokens *UITokens `json:"tokens,omitempty"`
	// TokensByTheme overlays Tokens for one palette. Keys are palette ids;
	// ids outside today's catalog are carried with a warning (theme ids are
	// an open set by design — ValidateTheme accepts any [a-z0-9-]+).
	TokensByTheme map[string]*UITokens `json:"tokensByTheme,omitempty"`
	// DataColors is family → key → hex. Families: label, type, status.
	DataColors map[string]map[string]string `json:"dataColors,omitempty"`
}

// Data-color families and the fixed key rule per family. This is the
// enforcement point of the repo-wide "never key on display names" trap: the
// error strings teach the caller the right key kind.
const (
	uiDataFamilyLabel  = "label"
	uiDataFamilyType   = "type"
	uiDataFamilyStatus = "status"
)

var uiDataFamilies = []string{uiDataFamilyLabel, uiDataFamilyType, uiDataFamilyStatus}

// statusCategoryValues is the closed key set of dataColors.status.
var statusCategoryValues = map[string]bool{"new": true, "inprogress": true, "done": true}

// maxDataColorKeyLen bounds one dataColors key. Labels are free strings, so
// the bound is the only defense against a pasted document turning config.json
// into a dump site; 256 is far above any real label.
const maxDataColorKeyLen = 256

// EffectiveTokenColors merges the palette-agnostic and palette-scoped
// overrides for one palette (theme wins). Nil maps are fine.
func (u *UIConfig) EffectiveTokenColors(palette string) map[string]string {
	if u == nil {
		return nil
	}
	var out map[string]string
	if u.Tokens != nil {
		for k, v := range u.Tokens.Colors {
			if out == nil {
				out = map[string]string{}
			}
			out[k] = v
		}
	}
	if byTheme := u.TokensByTheme[palette]; byTheme != nil {
		for k, v := range byTheme.Colors {
			if out == nil {
				out = map[string]string{}
			}
			out[k] = v
		}
	}
	return out
}

// knownPalettes returns the catalog palettes plus any palette named in
// TokensByTheme, sorted — the validation loop covers everything that could
// render.
func (u *UIConfig) knownPalettes() []string {
	set := map[string]bool{}
	for _, p := range tokencheck.CatalogPalettes() {
		set[p] = true
	}
	if u != nil {
		for p := range u.TokensByTheme {
			set[p] = true
		}
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// catalogBase is the catalog value set of one palette, shaped as the `base`
// ValidateTokens reads. A palette outside the catalog yields nil and the
// caller warns instead of validating.
func catalogBase(palette string) map[string]string {
	var out map[string]string
	for _, tok := range tokencheck.CatalogTokens() {
		v, ok := tokencheck.CatalogValue(tok.Name, palette)
		if !ok {
			return nil
		}
		if out == nil {
			out = map[string]string{}
		}
		out[tok.Name] = v
	}
	return out
}

// validateTokenColors judges one palette's effective overrides against that
// palette's catalog base. Rejects come back as error; warns (unknown token
// names, incomplete base) as violations for the caller to surface.
func validateTokenColors(palette string, overrides, base map[string]string) (warns []tokencheck.Violation, err error) {
	for _, v := range tokencheck.ValidateTokens(overrides, base) {
		if v.Severity == tokencheck.SeverityReject {
			return warns, fmt.Errorf("ui tokens (palette %s): %s", palette, v.Message)
		}
		warns = append(warns, v)
	}
	return warns, nil
}

// ValidateUIConfig is the write gate for the whole `ui` block. It returns the
// carry-along warnings (unknown names, unknown palettes) and an error that
// refuses the save. A nil config is valid and clears nothing.
func ValidateUIConfig(u *UIConfig) (warns []tokencheck.Violation, err error) {
	if u == nil {
		return nil, nil
	}
	if err := ValidateDataColors(u.DataColors); err != nil {
		return warns, err
	}
	catalogued := map[string]bool{}
	for _, p := range tokencheck.CatalogPalettes() {
		catalogued[p] = true
	}
	// Palette-agnostic warnings repeat once per palette the token renders in;
	// a CLI user should read one line, not four.
	warned := map[string]bool{}
	addWarn := func(v tokencheck.Violation) {
		k := v.Rule + "\x00" + v.Token
		if warned[k] {
			return
		}
		warned[k] = true
		warns = append(warns, v)
	}
	// Palette-JUDGED rules (the fold): the same (rule, token) can fire in
	// several palettes because an all-palette override is judged against each
	// palette's own base. One folded line names the failing palettes and
	// teaches the per-palette scoping fix. The palette
	// list is deterministic: knownPalettes is sorted.
	paletteJudged := map[string]bool{
		"status-pair":              true,
		"status-pair-deuteranopia": true,
		"status-role-floor":        true,
		"accent-text-contrast":     true,
	}
	paletteFails := map[string][]string{} // rule\x00token → failing palettes
	// Dimension axes are palette-agnostic: validate the one stored copy
	// before the per-palette color loop (a refused dimension stops the save
	// before any palette judgment runs).
	if u.Tokens != nil {
		for _, v := range tokencheck.ValidateDimensions(map[string]map[string]string{
			"spacing": u.Tokens.Spacing,
			"layout":  u.Tokens.Layout,
			"type":    u.Tokens.Type,
		}) {
			if v.Severity == tokencheck.SeverityReject {
				return warns, fmt.Errorf("ui tokens (dimensions): %s", v.Message)
			}
			addWarn(v)
		}
	}
	// A hand-edited or newer-build config can park dimension axes inside a
	// theme overlay (the struct has the fields); say so instead of
	// pretending they render.
	themeKeys := make([]string, 0, len(u.TokensByTheme))
	for p := range u.TokensByTheme {
		themeKeys = append(themeKeys, p)
	}
	sort.Strings(themeKeys)
	for _, palette := range themeKeys {
		t := u.TokensByTheme[palette]
		if t == nil || len(t.Spacing)+len(t.Layout)+len(t.Type) == 0 {
			continue
		}
		addWarn(tokencheck.Violation{
			Token:    palette,
			Rule:     "dimensions-not-per-theme",
			Severity: tokencheck.SeverityWarn,
			Message: fmt.Sprintf("ui.tokensByTheme[%q] carries dimension axes (spacing/layout/type); dimensions apply to every palette — kept for forward compatibility but not rendered; set them under ui.tokens",
				palette),
		})
	}
	for _, palette := range u.knownPalettes() {
		overrides := u.EffectiveTokenColors(palette)
		if len(overrides) == 0 {
			continue
		}
		if !catalogued[palette] {
			addWarn(tokencheck.Violation{
				Token:    palette,
				Rule:     "unknown-palette",
				Severity: tokencheck.SeverityWarn,
				Message: fmt.Sprintf("ui.tokensByTheme[%q] is not a palette this build ships (%s); kept for forward compatibility but not rendered",
					palette, strings.Join(tokencheck.CatalogPalettes(), ", ")),
			})
			continue
		}
		pw, err := validateTokenColors(palette, overrides, catalogBase(palette))
		if err != nil {
			return warns, err
		}
		for _, v := range pw {
			if paletteJudged[v.Rule] {
				// One (rule, token) can fire several times per palette (the
				// floors are per-ground); the fold keeps one line and one
				// mention per palette. Palettes arrive in sorted order, so
				// comparing the last entry dedupes.
				k := v.Rule + "\x00" + v.Token
				ps := paletteFails[k]
				if len(ps) == 0 {
					warns = append(warns, v) // first copy carries the message
				}
				if len(ps) == 0 || ps[len(ps)-1] != palette {
					paletteFails[k] = append(ps, palette)
				}
				continue
			}
			addWarn(v)
		}
	}
	// Fold the palette-judged warnings: append the failing-palette list (and
	// the scoping fix) to the first copy's message.
	for i := range warns {
		v := &warns[i]
		k := v.Rule + "\x00" + v.Token
		ps, ok := paletteFails[k]
		if !ok || len(ps) == 0 {
			continue
		}
		delete(paletteFails, k)
		noun, verb := "palette ", "to change one palette only, scope the override under "
		if len(ps) > 1 {
			noun, verb = "palettes ", "to change only those, scope the override per palette under "
		}
		v.Message += fmt.Sprintf(" — fails in %s%s; %sui.tokensByTheme.<palette>",
			noun, strings.Join(ps, ", "), verb)
	}
	return warns, nil
}

// ValidateDataColors enforces the family/key/value rules. The value rule is
// hex only — data inks are decorative (dots, chip text), not grounds, so no
// contrast floor applies; the token tiers carry the legibility contract.
func ValidateDataColors(dc map[string]map[string]string) error {
	if len(dc) == 0 {
		return nil
	}
	families := map[string]bool{}
	for _, f := range uiDataFamilies {
		families[f] = true
	}
	keys := make([]string, 0, len(dc))
	for f := range dc {
		keys = append(keys, f)
	}
	sort.Strings(keys)
	for _, family := range keys {
		if !families[family] {
			return fmt.Errorf("ui.dataColors: unknown family %q — valid families are %s", family, strings.Join(uiDataFamilies, ", "))
		}
		sub := make([]string, 0, len(dc[family]))
		for k := range dc[family] {
			sub = append(sub, k)
		}
		sort.Strings(sub)
		for _, k := range sub {
			key := strings.TrimSpace(k)
			if key == "" {
				return fmt.Errorf("ui.dataColors.%s: key must not be empty", family)
			}
			if len(key) > maxDataColorKeyLen {
				return fmt.Errorf("ui.dataColors.%s: key longer than %d characters", family, maxDataColorKeyLen)
			}
			switch family {
			case uiDataFamilyType:
				// Same rule as defaultIssueTypeId: ids, never display names.
				if !issueTypeIDRe.MatchString(key) {
					return fmt.Errorf("ui.dataColors.type keys must be Jira issue type ids (digits), not display names (got %q) — names localize per account", key)
				}
			case uiDataFamilyStatus:
				if !statusCategoryValues[key] {
					return fmt.Errorf("ui.dataColors.status keys must be a status_category: new, inprogress, or done (got %q) — status display names localize per account", key)
				}
			}
			v := dc[family][k]
			if !tokencheck.ValidHex(v) {
				return fmt.Errorf("ui.dataColors.%s[%q]: %q is not a #rgb or #rrggbb hex color", family, key, truncate(v, 40))
			}
		}
	}
	return nil
}

// truncate echoes a user value in an error, bounded (tokencheck.quote is the
// same rule; repeated here so the error strings stay self-contained).
func truncate(v string, max int) string {
	if max > 0 && len(v) > max {
		return v[:max] + "…"
	}
	return v
}

// ApplyUIConfig validates next and installs it on c. Warnings are printed to
// stderr here (LoadFor's migration notices set the precedent: the write
// succeeded and the warning is part of its output, not an API result).
func ApplyUIConfig(c *Config, next *UIConfig) error {
	_, err := ApplyUIConfigWithWarnings(c, next)
	return err
}

// ApplyUIConfigWithWarnings is ApplyUIConfig with the write-time warnings
// returned as well: under warn+save the warning IS the interesting
// part of the answer — why the saved look will render the way it does — and
// the settings PUT carries them on its response (uiWarnings) so a writer
// that never sees the CLI's stderr still reads them. The stderr print stays;
// server logs keep the same record a CLI session leaves.
func ApplyUIConfigWithWarnings(c *Config, next *UIConfig) ([]tokencheck.Violation, error) {
	if c == nil {
		return nil, errors.New("nil config")
	}
	warns, err := ValidateUIConfig(next)
	if err != nil {
		return nil, err
	}
	for _, w := range warns {
		fmt.Fprintf(os.Stderr, "gadak: ui: %s\n", w.Message)
	}
	c.UI = next
	return warns, nil
}

// UITokenVars expands the stored overrides into the final CSS variable maps
// the web consumes: palette → cssVar ("--color-accent") → hex. Only names
// the catalog knows and valid hex values are injected — anything else is
// reported as a load-time advisory so a config written by a newer build
// (renamed token) degrades to "override ignored" instead of an unbootable
// UI. Locked tiers RENDER (the write path warned and saved the
// value — filtering it here would turn that warning into a silent lie); the
// dimension sibling still filters its one locked token because a stored
// docked-min is genuinely dead (the runtime recomputes it).
func UITokenVars(u *UIConfig) (vars map[string]map[string]string, warns []tokencheck.Violation) {
	vars = map[string]map[string]string{}
	if u == nil {
		return vars, nil
	}
	for _, palette := range tokencheck.CatalogPalettes() {
		for name, value := range u.EffectiveTokenColors(palette) {
			_, known := tokencheck.TierOf(name)
			cssVar := "--color-" + strings.TrimPrefix(strings.TrimSpace(name), "--color-")
			switch {
			case !known:
				warns = append(warns, tokencheck.Violation{
					Token: name, Rule: "unknown-token", Severity: tokencheck.SeverityWarn,
					Message: fmt.Sprintf("%s is not in the color catalog; ignored (a newer catalog may have renamed it)", cssVar),
				})
			case !tokencheck.ValidHex(value):
				warns = append(warns, tokencheck.Violation{
					Token: name, Rule: "hex", Severity: tokencheck.SeverityWarn,
					Message: fmt.Sprintf("%s: %q is not a hex color; ignored", cssVar, value),
				})
			default:
				if vars[palette] == nil {
					vars[palette] = map[string]string{}
				}
				vars[palette][cssVar] = value
			}
		}
	}
	return vars, warns
}

// UIDimensionVars expands the stored dimension overrides into the single
// palette-agnostic CSS variable map the web consumes: cssVar ("--spacing-row")
// → value ("44px"). It is the sibling of UITokenVars with the palette axis
// removed — dimensions do not vary by theme. Mirrors its filter exactly:
// only names the dim catalog knows, tiers that may render, and values that
// parse are injected; everything else degrades to a load-time advisory
// (GDK-769 axis 3) so a config written by a newer build never blocks a boot.
// The map is overrides-only: base values keep coming from app.css and the JS
// layout constants.
func UIDimensionVars(u *UIConfig) (vars map[string]string, warns []tokencheck.Violation) {
	vars = map[string]string{}
	if u == nil || u.Tokens == nil {
		return vars, nil
	}
	axes := map[string]map[string]string{
		"spacing": u.Tokens.Spacing,
		"layout":  u.Tokens.Layout,
		"type":    u.Tokens.Type,
	}
	for _, axis := range tokencheck.DimAxes() {
		names := make([]string, 0, len(axes[axis]))
		for name := range axes[axis] {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			key := strings.TrimSpace(name)
			value := strings.TrimSpace(axes[axis][name])
			tok, known := tokencheck.DimTokenOf(axis, key)
			_, valueOK := tok.ParseValue(value) // false for the zero token too
			switch {
			case !known:
				warns = append(warns, tokencheck.Violation{
					Token: key, Rule: "unknown-token", Severity: tokencheck.SeverityWarn,
					Message: fmt.Sprintf("%s is not in the dimension catalog; ignored (a newer catalog may have renamed it)", key),
				})
			case tok.Tier == "locked":
				warns = append(warns, tokencheck.Violation{
					Token: key, Rule: "locked", Severity: tokencheck.SeverityWarn,
					Message: fmt.Sprintf("%s is locked in this build; override ignored (derived: sidebar + list-min + detail-min — set those three)", tok.CSSVar),
				})
			case !valueOK:
				warns = append(warns, tokencheck.Violation{
					Token: key, Rule: "length", Severity: tokencheck.SeverityWarn,
					Message: fmt.Sprintf("%s: %q is not a valid dimension value; ignored", tok.CSSVar, value),
				})
			default:
				vars[tok.CSSVar] = value
			}
		}
	}
	return vars, warns
}

// UIDataColors passes the stored data inks through the same value gate and
// returns what may render. Keys were validated at write time; this is the
// stale-schema defense for values that reached disk anyway.
func UIDataColors(u *UIConfig) map[string]map[string]string {
	out := map[string]map[string]string{}
	if u == nil {
		return out
	}
	for _, family := range uiDataFamilies {
		for k, v := range u.DataColors[family] {
			if !tokencheck.ValidHex(v) {
				continue
			}
			if out[family] == nil {
				out[family] = map[string]string{}
			}
			out[family][strings.TrimSpace(k)] = v
		}
	}
	return out
}

// cloneUIConfig copies the block header so a Set function can stage a mutation
// without aliasing the live config: a refused write must leave c.UI exactly as
// it was (the same guarantee ApplyAppearance gives appearance).
func cloneUIConfig(u *UIConfig) *UIConfig {
	if u == nil {
		return &UIConfig{}
	}
	return &UIConfig{
		Tokens:        u.Tokens,
		TokensByTheme: u.TokensByTheme,
		DataColors:    u.DataColors,
	}
}

// parseUITokens accepts the shapes a CLI user would type for the token
// block: the flat colors map {"accent": …} (string values are colors — the
// flat form stays colors-only, as before) and the axis wrapper
// {"colors": {…}, "spacing": {…}, "layout": {…}, "type": {…}}. Unknown
// wrapper axes refuse with the valid list so a typo cannot silently drop
// the payload.
func parseUITokens(path string, raw json.RawMessage) (*UITokens, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var flat map[string]string
	if err := json.Unmarshal(raw, &flat); err == nil {
		return &UITokens{Colors: flat}, nil
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, fmt.Errorf("%s must be an object of token→value, or {\"colors\": {…}, \"spacing\": {…}, \"layout\": {…}, \"type\": {…}}", path)
	}
	for k := range probe {
		switch k {
		case "colors", "spacing", "layout", "type":
		default:
			return nil, fmt.Errorf("%s: unknown axis %q — axes are colors, spacing, layout, type", path, k)
		}
	}
	var wrapped struct {
		Colors  map[string]string `json:"colors"`
		Spacing map[string]string `json:"spacing"`
		Layout  map[string]string `json:"layout"`
		Type    map[string]string `json:"type"`
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return nil, fmt.Errorf("%s axes must map token names to values", path)
	}
	if wrapped.Colors == nil {
		wrapped.Colors = map[string]string{}
	}
	return &UITokens{Colors: wrapped.Colors, Spacing: wrapped.Spacing, Layout: wrapped.Layout, Type: wrapped.Type}, nil
}

// parseThemeTokenOverlay parses one ui.tokensByTheme entry: colors only.
// Dimension axes are palette-agnostic, so a per-theme copy could never
// render; refusing here keeps the mistake next to the command that made it.
// (The write gate still warns for such axes that reach config.json by other
// roads — hand edits and newer builds.)
func parseThemeTokenOverlay(path string, raw json.RawMessage) (*UITokens, error) {
	t, err := parseUITokens(path, raw)
	if err != nil {
		return nil, err
	}
	if t != nil && len(t.Spacing)+len(t.Layout)+len(t.Type) > 0 {
		return nil, fmt.Errorf("%s: dimension axes (spacing/layout/type) apply to every palette — set them under ui.tokens, not per theme", path)
	}
	return t, nil
}

// ConfigVersionOfDir is the disk identity of a profile's config.json, for a
// directory already in hand (webConfig gets the loaded Config's Directory()).
// Every writer (CLI `config set`, PUT settings/, LoadFor's legacy-field
// rewrite) goes through the same atomic temp+rename, so mtime+size changes
// exactly when the content does. The ui-focus poll carries it; the web
// refetches config.json when it moves. A monotonic counter was rejected
// because it needed a second owner (local_meta) and a CLI↔SQLite coupling on
// a path that already had a single owner: the file.
func ConfigVersionOfDir(dir string) string {
	if dir == "" {
		return ""
	}
	return configFileVersion(filepath.Join(dir, "config.json"))
}

func configFileVersion(path string) string {
	fi, err := os.Stat(path)
	if err != nil {
		return "0"
	}
	return fmt.Sprintf("%d.%d", fi.ModTime().UnixNano(), fi.Size())
}
