package mcp

// The ui.tokens surface for shell-less hosts (GDK-769). A host with a shell
// discovers and edits design tokens with `gadak config get ui.tokens.catalog`
// / `gadak config set ui.tokens.<axis> …`; Claude Desktop and its siblings
// had no path to either — the MCP server carried five read tools and no
// config surface at all.
//
// Both tools here are thin: every fact and every rule comes from the settings
// catalog (config.SettingByPath / config.SettingPaths), which is the same
// owner `gadak config` and PUT /api/settings read and write through. That is
// deliberate and is the structural point of the round — a second copy of the
// axis list, of the token catalog, or of the merge semantics is exactly the
// drift internal/mcp/tools.go descriptions have no gate against. The axis
// enum in the schema and in the prose below is GENERATED from the catalog
// for the same reason: a description cannot teach an axis the server does
// not accept, because it does not contain a typed axis name at all.
//
// Writes here touch config.json for this process's workspace only — never
// the mirror, never the origin. That is the same boundary gadak_show already
// draws (it writes a local ui-focus file), and the web repaints from the
// configVersion poll without a reload.

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/config/tokencheck"
)

const (
	toolUITokens = "gadak_ui_tokens"
	toolUISet    = "gadak_ui_set"
)

// uiTokenAxes returns the ui.tokens axis names, derived from the settings
// catalog rather than restated: an axis is a `ui.tokens.<ax>` path that also
// has the `ui.tokens.<ax>.<name>` scalar template beside it. The read-only
// discovery paths (ui.tokens.catalog, ui.tokens.dim-catalog) have no such
// template and so are not axes — which is the fact that matters, not their
// spelling.
func uiTokenAxes() []string {
	const prefix = "ui.tokens."
	var out []string
	for _, p := range config.SettingPaths() {
		ax, ok := strings.CutPrefix(p, prefix)
		if !ok || ax == "" || strings.Contains(ax, ".") {
			continue
		}
		if _, hasLeaf := config.SettingByPath(prefix + ax + ".<name>"); !hasLeaf {
			continue
		}
		out = append(out, ax)
	}
	sort.Strings(out)
	return out
}

// uiAxisList renders the axes for prose. Generated, never typed.
func uiAxisList() string { return strings.Join(uiTokenAxes(), ", ") }

// uiTokenSetting looks up one axis merge entry, refusing an axis the catalog
// does not carry with the list it does.
func uiTokenSetting(axis string) (config.Setting, error) {
	axis = strings.TrimSpace(axis)
	if axis == "" {
		return config.Setting{}, fmt.Errorf("axis is required — one of: %s", uiAxisList())
	}
	for _, ax := range uiTokenAxes() {
		if ax == axis {
			s, ok := config.SettingByPath("ui.tokens." + ax)
			if !ok {
				return config.Setting{}, fmt.Errorf("ui.tokens.%s is not in the settings catalog", ax)
			}
			return s, nil
		}
	}
	return config.Setting{}, fmt.Errorf("axis %q is not a ui.tokens axis — one of: %s", axis, uiAxisList())
}

// uiDataColorsTarget is the one write target that is not a ui.tokens axis:
// ui.dataColors, the per-data inks (label / type / status). It is spelled the
// way its settings path is, so a reader who sees it in the enum can find it
// with `gadak config get ui.dataColors`.
const uiDataColorsTarget = "dataColors"

// uiWriteTargets is everything gadak_ui_set accepts in `axis`: the ui.tokens
// axes, plus dataColors. Generated, never typed — the enum, the prose and the
// refusal message all read this one list, so the description cannot name a
// target the server does not accept.
func uiWriteTargets() []string {
	return append(uiTokenAxes(), uiDataColorsTarget)
}

// uiTargetPath maps a published target to the settings-catalog path that owns
// it. Every write in this file ends in config.SettingByPath(...).Set, so the
// parse rules, the refusals and the warnings have exactly one owner — the one
// `gadak config set` and PUT /api/settings already share.
func uiTargetPath(target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == uiDataColorsTarget {
		return "ui.dataColors", nil
	}
	if _, err := uiTokenSetting(target); err != nil {
		return "", err
	}
	return "ui.tokens." + target, nil
}

// uiTargetSetting resolves one write target, with the palette in hand: a
// per-palette colour lives under ui.tokensByTheme, which is a different
// catalog entry from the axis of the same name. Returns the Setting and the
// path so the response can name where the write landed.
func uiTargetSetting(target, palette string) (config.Setting, error) {
	if strings.TrimSpace(palette) != "" {
		if target == uiDataColorsTarget {
			return config.Setting{}, fmt.Errorf("palette does not apply to %s — data inks are not per palette", uiDataColorsTarget)
		}
		// The axis still has to be a real axis; whether it may be set per
		// theme is the catalog's judgment (parseThemeTokenOverlay refuses the
		// palette-agnostic axes), not a rule restated here.
		if _, err := uiTokenSetting(target); err != nil {
			return config.Setting{}, err
		}
		st, ok := config.SettingByPath("ui.tokensByTheme")
		if !ok {
			return config.Setting{}, fmt.Errorf("ui.tokensByTheme is not in the settings catalog")
		}
		return st, nil
	}
	path, err := uiTargetPath(target)
	if err != nil {
		return config.Setting{}, err
	}
	st, ok := config.SettingByPath(path)
	if !ok {
		return config.Setting{}, fmt.Errorf("%s is not in the settings catalog", path)
	}
	return st, nil
}

// uiTargetList renders the write targets for prose. Generated, never typed.
func uiTargetList() string { return strings.Join(uiWriteTargets(), ", ") }

func uiTokensDescription() string {
	return `Read the user's UI design tokens: what is set now, and what may be set.

With NO argument this is the cheap call: the axis list, the stored ui.tokens
overrides per axis (plus ui.tokensByTheme and ui.dataColors), the write rules
as the settings catalog states them, and the warnings the CURRENTLY STORED
values already trip — a value can be saved and warned about at the same time.
The read-only token catalogs are NOT in that answer.

Pass an axis to add its catalog: for "` + colorsAxis + `" the colour tokens with tier,
rules and per-palette values; for a dimension axis the tokens with cssVar,
tier, unit, default, min/max and the cross-token relations. That half is tens
of KB, which is why naming the axis is what buys it.

Argument: {axis?: <one axis>}
Axis: "` + strings.Join(uiTokenAxes(), `" | "`) + `"

Change a token with gadak_ui_set. This tool reads config.json only; it does not
touch the mirror or the origin.`
}

func uiSetDescription() string {
	return `Merge token overrides into one axis of the user's UI settings, and save.

Argument: {axis: <one target>, values: {<token name>: <value string>, or null to delete}, palette?: <palette id>}.
axis: "` + strings.Join(uiWriteTargets(), `" | "`) + `"
The merge is key-wise: named keys update, every other key of the target and
every other target are preserved, a null value deletes its key, and {} changes
nothing. Discover token names with gadak_ui_tokens.

Three things are writable, all through the settings catalog ` + "`gadak config set`" + `
uses:
- a ui.tokens axis — the default, when palette is absent.
- a per-palette colour — pass palette ("dark", "light", any palette id the
  build ships) and the merge lands in ui.tokensByTheme.<palette>. Colours only:
  spacing/layout/type/fonts apply to every palette and are refused per theme.
- the per-data inks — axis "` + uiDataColorsTarget + `", where each key is
  <family>.<key>. Families: ` + strings.Join(config.UIDataFamilies(), ", ") + `. A type key is an
  issue type id, not a name, and a status key is a status_category
  (new|inprogress|done), not a display name — names localize per account.
  For example label.urgent, type.10007, status.inprogress.

The response carries ` + "`previous`" + `: the keys you named as they were immediately
before the merge, in the same shape values takes (a key that did not exist
comes back as null). Pass it straight back as values to undo the change.

Two outcomes, and they are not the same (a deliberate product decision):
- REFUSED — nothing is written and the error names the field and what was
  wrong. What refuses: a value that cannot parse (a non-hex color, a
  non-length dimension, a malformed font stack), a wrong shape, a display name
  where an id belongs, and the derived layout.docked-min.
- SAVED WITH WARNINGS — the value is stored and the response carries
  warnings[] measuring what it trips: locked tiers, contrast / ΔEok /
  deuteranopia floors, dimension ranges and cross-token relations, and
  unknown token names (carried forward on purpose so a config written by a
  newer gadak keeps loading). The look is the user's; the warning is the
  measurement, not a rejection.

Writes config.json for this workspace only — never the mirror, never Jira.
A running gadak window repaints without a reload.`
}

func uiToolDefinitions() []Tool {
	enumOf := func(vs []string) []any {
		out := make([]any, 0, len(vs))
		for _, v := range vs {
			out = append(out, v)
		}
		return out
	}
	return []Tool{
		{
			Name:        toolUITokens,
			Description: uiTokensDescription(),
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"axis": map[string]any{
						"type":        "string",
						"enum":        enumOf(uiTokenAxes()),
						"description": "Limit the answer to one token axis, and add that axis's read-only catalog. Omit for the cheap answer over every axis.",
					},
				},
				"required":             []string{},
				"additionalProperties": false,
			},
		},
		{
			Name:        toolUISet,
			Description: uiSetDescription(),
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"axis": map[string]any{
						"type":        "string",
						"enum":        enumOf(uiWriteTargets()),
						"description": "The token axis to merge into, or \"" + uiDataColorsTarget + "\" for the per-data inks.",
					},
					"values": map[string]any{
						"type":        "object",
						"description": "token name → value string, or null to delete that key. {} is a no-op. For " + uiDataColorsTarget + " the key is <family>.<key>.",
					},
					"palette": map[string]any{
						"type":        "string",
						"description": "Merge into ui.tokensByTheme.<palette> instead of ui.tokens — a colour for one palette only (\"dark\", \"light\").",
					},
				},
				"required":             []string{"axis", "values"},
				"additionalProperties": false,
			},
		},
	}
}

// uiConfigLocation reports where the write lands and the disk identity the
// web's configVersion poll compares — the two facts a shell-less host cannot
// otherwise see.
func uiConfigLocation(cfg *config.Config) (file, version string) {
	dir := cfg.Directory()
	if dir == "" {
		// An in-memory Config answers "" and DirFor then resolves the root
		// profile — the same fallback internal/server/settings.go uses.
		dir, _ = config.DirFor(cfg.ProfileName())
	}
	if dir == "" {
		return "", ""
	}
	return dir + "/config.json", config.ConfigVersionOfDir(dir)
}

// colorsAxis is the one axis name this file spells, and it is checked against
// the settings catalog by TestUIToolAxesComeFromSettingsCatalog: the color
// catalog is a different owner (tokencheck.CatalogTokens) from the dimension
// one, so the read tool has to tell them apart. Every other axis name here is
// generated.
const colorsAxis = "colors"

// isDimAxis asks tokencheck which axes its dimension catalog covers, rather
// than listing the ones it does not.
func isDimAxis(axis string) bool {
	return slices.Contains(tokencheck.DimAxes(), axis)
}

func (s *Server) toolUITokens(args map[string]any) ([]contentItem, error) {
	axes := uiTokenAxes()
	want := axes
	// Naming an axis is what buys the catalog. With no axis the answer is the
	// cheap half for every axis: an agent almost always omits an optional
	// argument, so the commonest call must not be the most expensive one.
	named := false
	if raw, ok := stringArg(args, "axis"); ok && strings.TrimSpace(raw) != "" {
		if _, err := uiTokenSetting(raw); err != nil {
			return nil, err
		}
		want = []string{strings.TrimSpace(raw)}
		named = true
	}
	cfg, err := config.LoadFor(s.Profile)
	if err != nil {
		return nil, err
	}

	tokens := map[string]any{}
	rules := map[string]any{}
	wantColors, wantDims := false, false
	for _, ax := range want {
		st, err := uiTokenSetting(ax)
		if err != nil {
			return nil, err
		}
		tokens[ax] = st.Get(cfg)
		rules[ax] = map[string]any{"path": st.Path, "description": st.Description}
		if isDimAxis(ax) {
			wantDims = true
		} else if ax == colorsAxis {
			wantColors = true
		}
	}
	// The two non-axis targets gadak_ui_set can now write. They are small
	// (what the user actually overrode), so they ride along on every call —
	// a write target with no way to read its current value would be the same
	// gap this round closed in the other direction.
	for _, path := range []string{"ui.tokensByTheme", "ui.dataColors"} {
		st, ok := config.SettingByPath(path)
		if !ok {
			continue
		}
		name := strings.TrimPrefix(path, "ui.")
		tokens[name] = st.Get(cfg)
		rules[name] = map[string]any{"path": st.Path, "description": st.Description}
	}

	catalog := map[string]any{}
	if named && wantColors {
		if st, ok := config.SettingByPath("ui.tokens.catalog"); ok {
			catalog["colors"] = st.Get(cfg)
		}
	}
	if named && wantDims {
		if st, ok := config.SettingByPath("ui.tokens.dim-catalog"); ok {
			catalog["dimensions"] = st.Get(cfg)
		}
	}

	// Advisories on what is stored right now — the same judgment a write
	// re-runs, so "why does my UI look like this" is answerable without a write.
	warns, verr := config.ValidateUIConfig(cfg.UI)
	out := map[string]any{
		"axes":     axes,
		"targets":  uiWriteTargets(),
		"tokens":   tokens,
		"rules":    rules,
		"catalog":  catalog,
		"warnings": warns,
	}
	if !named {
		out["catalog_hint"] = "the read-only token catalogs are tens of KB and are left out here; " +
			"pass axis (one of: " + uiAxisList() + ") to get that axis's catalog"
	}
	if verr != nil {
		// A stored config that no longer validates is a fact the reader needs;
		// it is not this tool's failure.
		out["invalid"] = verr.Error()
	}
	if file, version := uiConfigLocation(cfg); file != "" {
		out["config_file"] = file
		out["config_version"] = version
	}
	return s.marshalResult(out)
}

// uiPreviousOf is the undo patch for one merge: for every key the caller
// named, its value immediately before the write, or JSON null where the key
// did not exist. Handing it back as `values` restores the target exactly —
// the prior value alone would not, because it cannot delete a key the merge
// added.
func uiPreviousOf(current map[string]string, values map[string]any) map[string]any {
	prev := make(map[string]any, len(values))
	for k := range values {
		if v, ok := current[k]; ok {
			prev[k] = v
		} else {
			prev[k] = nil
		}
	}
	return prev
}

// uiCurrentMap reads a target's stored keys as a flat name→value map, through
// the catalog Get rather than off the Config struct: one owner, and the
// per-palette and per-family shapes flatten the same way their write patch
// does.
func uiCurrentMap(st config.Setting, cfg *config.Config, target, palette string) (map[string]string, error) {
	raw, err := json.Marshal(st.Get(cfg))
	if err != nil {
		return nil, err
	}
	switch {
	case strings.TrimSpace(palette) != "":
		var byTheme map[string]map[string]map[string]string
		if err := json.Unmarshal(raw, &byTheme); err != nil {
			return nil, err
		}
		return byTheme[palette][target], nil
	case target == uiDataColorsTarget:
		var families map[string]map[string]string
		if err := json.Unmarshal(raw, &families); err != nil {
			return nil, err
		}
		flat := map[string]string{}
		for family, keys := range families {
			for k, v := range keys {
				flat[family+"."+k] = v
			}
		}
		return flat, nil
	default:
		var axis map[string]string
		if err := json.Unmarshal(raw, &axis); err != nil {
			return nil, err
		}
		return axis, nil
	}
}

// uiPatchFor turns the flat {name: value|null} the caller sent into the body
// the target's catalog Setting takes. The axis settings merge key-wise
// themselves, so their body is the patch verbatim; ui.tokensByTheme and
// ui.dataColors replace wholesale, so the merge happens here — over the
// current value read through the same Setting — and the RESULT is handed to
// Set, which still owns every parse rule, refusal and warning.
func uiPatchFor(st config.Setting, cfg *config.Config, target, palette string, values map[string]any) (json.RawMessage, error) {
	merge := func(into map[string]any) {
		for k, v := range values {
			if v == nil {
				delete(into, k)
				continue
			}
			into[k] = v
		}
	}
	switch {
	case strings.TrimSpace(palette) != "":
		raw, err := json.Marshal(st.Get(cfg))
		if err != nil {
			return nil, err
		}
		byTheme := map[string]map[string]map[string]any{}
		if err := json.Unmarshal(raw, &byTheme); err != nil {
			return nil, err
		}
		if byTheme[palette] == nil {
			byTheme[palette] = map[string]map[string]any{}
		}
		if byTheme[palette][target] == nil {
			byTheme[palette][target] = map[string]any{}
		}
		merge(byTheme[palette][target])
		return json.Marshal(byTheme)
	case target == uiDataColorsTarget:
		raw, err := json.Marshal(st.Get(cfg))
		if err != nil {
			return nil, err
		}
		families := map[string]map[string]any{}
		if err := json.Unmarshal(raw, &families); err != nil {
			return nil, err
		}
		for k, v := range values {
			family, key, ok := strings.Cut(k, ".")
			if !ok || family == "" || key == "" {
				return nil, fmt.Errorf("%s key %q must be <family>.<key> — families are %s (for example label.urgent, status.inprogress)",
					uiDataColorsTarget, k, strings.Join(config.UIDataFamilies(), ", "))
			}
			if families[family] == nil {
				families[family] = map[string]any{}
			}
			if v == nil {
				delete(families[family], key)
				continue
			}
			families[family][key] = v
		}
		for family, keys := range families {
			if len(keys) == 0 {
				delete(families, family)
			}
		}
		return json.Marshal(families)
	default:
		return json.Marshal(values)
	}
}

func (s *Server) toolUISet(args map[string]any) ([]contentItem, error) {
	targetRaw, _ := stringArg(args, "axis")
	target := strings.TrimSpace(targetRaw)
	palette, _ := stringArg(args, "palette")
	palette = strings.TrimSpace(palette)
	if target == "" {
		return nil, fmt.Errorf("axis is required — one of: %s", uiTargetList())
	}
	if target != uiDataColorsTarget {
		if _, err := uiTokenSetting(target); err != nil {
			return nil, err
		}
	}
	st, err := uiTargetSetting(target, palette)
	if err != nil {
		return nil, err
	}

	rawValues, present := args["values"]
	if !present || rawValues == nil {
		return nil, fmt.Errorf("%s requires {axis: string, values: object} — values maps a token name to a string, or to null to delete it (sent: [%s])", toolUISet, receivedArgKeys(args))
	}
	values, ok := rawValues.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s values must be an object of token→value (string, or null to delete)", toolUISet)
	}

	cfg, err := config.LoadFor(s.Profile)
	if err != nil {
		return nil, err
	}
	// Read the pre-merge values BEFORE the write, so the response can hand the
	// caller an undo patch rather than leaving it to whatever it still holds in
	// context.
	current, err := uiCurrentMap(st, cfg, target, palette)
	if err != nil {
		return nil, err
	}
	previous := uiPreviousOf(current, values)

	patch, err := uiPatchFor(st, cfg, target, palette, values)
	if err != nil {
		return nil, err
	}
	// The one gate. A refusal here leaves cfg untouched exactly as it does for
	// `gadak config set` and for PUT /api/settings — nothing is saved below.
	if err := st.Set(cfg, patch); err != nil {
		return nil, err
	}
	// The warnings this write leaves standing: the same ValidateUIConfig verdict
	// ApplyUIConfigWithWarnings returned inside Set, re-read off the merged
	// whole so the response can carry it (the CLI prints it to stderr, which a
	// shell-less host never sees).
	warns, verr := config.ValidateUIConfig(cfg.UI)
	if verr != nil {
		return nil, verr
	}

	saved := len(values) > 0
	if saved {
		if err := cfg.Save(); err != nil {
			return nil, err
		}
	}
	out := map[string]any{
		"axis":     target,
		"path":     st.Path,
		"saved":    saved,
		"tokens":   st.Get(cfg),
		"previous": previous,
		"warnings": warns,
		"undo": fmt.Sprintf("call %s again with the same axis%s and values = the previous object above",
			toolUISet, map[bool]string{true: "/palette", false: ""}[palette != ""]),
	}
	if palette != "" {
		out["palette"] = palette
	}
	if !saved {
		out["note"] = "values was empty: nothing merged, nothing written"
	}
	if file, version := uiConfigLocation(cfg); file != "" {
		out["config_file"] = file
		out["config_version"] = version
	}
	return s.marshalResult(out)
}
