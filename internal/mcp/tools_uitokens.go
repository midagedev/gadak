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

func uiTokensDescription() string {
	return `Read the user's UI design tokens: what is set now, and what may be set.

Answers both halves in one call — the stored ui.tokens overrides per axis, the
read-only token catalogs (color tokens with tier, rules and per-palette values;
dimension tokens with axis, cssVar, tier, unit, default, min/max and the
cross-token relations), and the write rules for each axis as the settings
catalog states them. Warnings list what the CURRENTLY STORED values already
trip; a value can be saved and warned about at the same time.

Argument: {axis?: <one axis>} — omit it for every axis (the color catalog is a
few tens of KB; pass an axis to narrow).
Axis: "` + strings.Join(uiTokenAxes(), `" | "`) + `"

Change a token with gadak_ui_set. This tool reads config.json only; it does not
touch the mirror or the origin.`
}

func uiSetDescription() string {
	return `Merge token overrides into one axis of the user's ui.tokens, and save.

Argument: {axis: <one axis>, values: {<token name>: <value string>, or null to delete}}.
Axis: "` + strings.Join(uiTokenAxes(), `" | "`) + `"
The merge is key-wise: named keys update, every other key of the axis, the
other axes, ui.tokensByTheme and ui.dataColors are preserved, a null value
deletes its key, and {} changes nothing. Discover token names with
gadak_ui_tokens.

Two outcomes, and they are not the same (a deliberate product decision):
- REFUSED — nothing is written and the error names the field and what was
  wrong. What refuses: a value that cannot parse (a non-hex color, a
  non-length dimension, a malformed font stack), a wrong shape, and the
  derived layout.docked-min.
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
	axes := uiTokenAxes()
	axisEnum := make([]any, 0, len(axes))
	for _, a := range axes {
		axisEnum = append(axisEnum, a)
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
						"enum":        axisEnum,
						"description": "Limit the answer to one token axis. Omit for all of them.",
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
						"enum":        axisEnum,
						"description": "The token axis to merge into.",
					},
					"values": map[string]any{
						"type":        "object",
						"description": "token name → value string, or null to delete that key. {} is a no-op.",
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
	if raw, ok := stringArg(args, "axis"); ok && strings.TrimSpace(raw) != "" {
		if _, err := uiTokenSetting(raw); err != nil {
			return nil, err
		}
		want = []string{strings.TrimSpace(raw)}
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

	catalog := map[string]any{}
	if wantColors {
		if st, ok := config.SettingByPath("ui.tokens.catalog"); ok {
			catalog["colors"] = st.Get(cfg)
		}
	}
	if wantDims {
		if st, ok := config.SettingByPath("ui.tokens.dim-catalog"); ok {
			catalog["dimensions"] = st.Get(cfg)
		}
	}

	// Advisories on what is stored right now — the same judgment a write
	// re-runs, so "why does my UI look like this" is answerable without a write.
	warns, verr := config.ValidateUIConfig(cfg.UI)
	out := map[string]any{
		"axes":     axes,
		"tokens":   tokens,
		"rules":    rules,
		"catalog":  catalog,
		"warnings": warns,
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

func (s *Server) toolUISet(args map[string]any) ([]contentItem, error) {
	axisRaw, _ := stringArg(args, "axis")
	st, err := uiTokenSetting(axisRaw)
	if err != nil {
		return nil, err
	}
	axis := strings.TrimSpace(axisRaw)

	rawValues, present := args["values"]
	if !present || rawValues == nil {
		return nil, fmt.Errorf("%s requires {axis: string, values: object} — values maps a token name to a string, or to null to delete it (sent: [%s])", toolUISet, receivedArgKeys(args))
	}
	values, ok := rawValues.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s values must be an object of token→value (string, or null to delete)", toolUISet)
	}
	patch, err := json.Marshal(values)
	if err != nil {
		return nil, err
	}

	cfg, err := config.LoadFor(s.Profile)
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
		"axis":     axis,
		"path":     st.Path,
		"saved":    saved,
		"tokens":   st.Get(cfg),
		"warnings": warns,
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
