package mcp

// Contract tests for the ui.tokens MCP pair (GDK-769). Two things are being
// pinned: that the tools route through the settings catalog rather than
// carrying their own copy of it, and that a description cannot teach an axis
// the server does not accept — internal/mcp/tools.go descriptions are the one
// surface in this repo with no gate, and the only reader is a shell-less agent.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/config/tokencheck"
)

// uiHome points config at an empty profile dir and returns it. No mirror is
// created: the ui tools must work without one.
func uiHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })
	return home
}

func uiServer(home string) *Server {
	return &Server{DBPath: filepath.Join(home, "gadak.db")}
}

// callUITool runs one tool and returns the decoded JSON payload, or the error.
func callUITool(t *testing.T, s *Server, name string, args map[string]any) (map[string]any, error) {
	t.Helper()
	var (
		items []contentItem
		err   error
	)
	switch name {
	case toolUITokens:
		items, err = s.toolUITokens(args)
	case toolUISet:
		items, err = s.toolUISet(args)
	default:
		t.Fatalf("callUITool: unknown tool %q", name)
	}
	if err != nil {
		return nil, err
	}
	if len(items) != 1 {
		t.Fatalf("%s returned %d content items, want 1", name, len(items))
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(items[0].Text), &out); err != nil {
		t.Fatalf("%s payload is not JSON: %v\n%s", name, err, items[0].Text)
	}
	return out, nil
}

// readConfigJSON is the on-disk check: a saved write has to be in the file,
// not only in the in-memory Config the tool answered from.
func readConfigJSON(t *testing.T, home string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(home, "config.json"))
	if err != nil {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("config.json is not JSON: %v", err)
	}
	return out
}

// storedAxis digs ui.tokens.<axis> out of a decoded config.json.
func storedAxis(doc map[string]any, axis string) map[string]any {
	ui, _ := doc["ui"].(map[string]any)
	tokens, _ := ui["tokens"].(map[string]any)
	ax, _ := tokens[axis].(map[string]any)
	return ax
}

// The description-vs-source check as an executable assertion. Every axis name
// the two tools publish — in the prose, in the JSON-Schema enum, and in the
// error the axis validator writes — must be a path the settings catalog
// actually carries, and the catalog's axes must all be published. This is the
// gate internal/mcp/tools.go has never had.
func TestUIToolDescriptionsMatchSettingsCatalog(t *testing.T) {
	uiHome(t)

	// The catalog's own answer, computed here the long way so the test does
	// not simply re-run the function it is checking.
	var fromCatalog []string
	for _, p := range config.SettingPaths() {
		if !strings.HasPrefix(p, "ui.tokens.") {
			continue
		}
		ax := strings.TrimPrefix(p, "ui.tokens.")
		if strings.Contains(ax, ".") {
			continue
		}
		if _, ok := config.SettingByPath("ui.tokens." + ax); !ok {
			continue
		}
		// A settable axis merges an object; the read-only discovery paths
		// (catalog, dim-catalog) refuse every write, including an empty merge.
		if s, _ := config.SettingByPath("ui.tokens." + ax); s.Set == nil {
			continue
		}
		c := &config.Config{}
		if err := mustSetting(t, "ui.tokens."+ax).Set(c, json.RawMessage(`{}`)); err != nil {
			continue
		}
		fromCatalog = append(fromCatalog, ax)
	}
	slices.Sort(fromCatalog)
	if len(fromCatalog) == 0 {
		t.Fatal("settings catalog exposes no ui.tokens axis — the derivation is broken, not the tools")
	}
	got := uiTokenAxes()
	if !slices.Equal(got, fromCatalog) {
		t.Fatalf("uiTokenAxes() = %v, settings catalog says %v", got, fromCatalog)
	}

	defs := uiToolDefinitions()
	if len(defs) != 2 {
		t.Fatalf("uiToolDefinitions() has %d tools, want 2", len(defs))
	}
	for _, tool := range defs {
		// The schema enum is the machine-readable half. The read tool publishes
		// the axes; the write tool publishes the axes plus dataColors, which is
		// a settings path of its own rather than a ui.tokens axis.
		want := fromCatalog
		if tool.Name == toolUISet {
			want = uiWriteTargets()
		}
		props, _ := tool.InputSchema["properties"].(map[string]any)
		axisProp, _ := props["axis"].(map[string]any)
		enum, _ := axisProp["enum"].([]any)
		if len(enum) != len(want) {
			t.Fatalf("%s axis enum = %v, want %v", tool.Name, enum, want)
		}
		for i, v := range enum {
			s, _ := v.(string)
			if s != want[i] {
				t.Fatalf("%s axis enum[%d] = %q, want %q", tool.Name, i, s, want[i])
			}
			path, err := uiTargetPath(s)
			if err != nil {
				t.Fatalf("%s teaches %q, which does not resolve: %v", tool.Name, s, err)
			}
			if _, ok := config.SettingByPath(path); !ok {
				t.Fatalf("%s teaches %q → %q, which is not a settings path", tool.Name, s, path)
			}
		}
		// The prose half: every catalog axis is named, and no word sitting in
		// an axis slot of the description is something the server would refuse.
		for _, ax := range fromCatalog {
			if !strings.Contains(tool.Description, `"`+ax+`"`) {
				t.Fatalf("%s description does not teach axis %q", tool.Name, ax)
			}
		}
		for _, quoted := range quotedAxisSlots(tool.Description) {
			if !slices.Contains(fromCatalog, quoted) {
				t.Fatalf("%s description teaches axis %q, which the server refuses (axes: %v)", tool.Name, quoted, fromCatalog)
			}
		}
	}

	// The refusal message is the third place an axis name is published.
	_, err := uiTokenSetting("bogus")
	if err == nil {
		t.Fatal("uiTokenSetting accepted a bogus axis")
	}
	for _, ax := range fromCatalog {
		if !strings.Contains(err.Error(), ax) {
			t.Fatalf("axis refusal %q does not list %q", err.Error(), ax)
		}
	}
}

func mustSetting(t *testing.T, path string) config.Setting {
	t.Helper()
	s, ok := config.SettingByPath(path)
	if !ok {
		t.Fatalf("settings catalog has no %s", path)
	}
	return s
}

// quotedAxisSlots pulls the axis names out of the generated `Axis: "a" | "b"`
// line the descriptions build, so a hand-typed extra alternative is caught.
// Both descriptions carry exactly one such line; a description that carries
// none fails the "teaches every axis" assertion above instead.
func quotedAxisSlots(desc string) []string {
	var out []string
	for _, line := range strings.Split(desc, "\n") {
		if !strings.HasPrefix(line, "Axis: ") {
			continue
		}
		parts := strings.Split(line, `"`)
		for i := 1; i < len(parts); i += 2 {
			if v := strings.TrimSpace(parts[i]); v != "" && !strings.Contains(v, " ") {
				out = append(out, v)
			}
		}
	}
	return out
}

// The read tool answers "what is set, and what can I set" from the same owners
// the CLI reads through — no second copy of either catalog.
func TestUITokensReadShape(t *testing.T) {
	home := uiHome(t)
	s := uiServer(home)

	out, err := callUITool(t, s, toolUITokens, map[string]any{})
	if err != nil {
		t.Fatalf("gadak_ui_tokens: %v", err)
	}
	axes, _ := out["axes"].([]any)
	if len(axes) != len(uiTokenAxes()) {
		t.Fatalf("axes = %v, want %v", axes, uiTokenAxes())
	}
	tokens, _ := out["tokens"].(map[string]any)
	for _, ax := range uiTokenAxes() {
		if _, ok := tokens[ax]; !ok {
			t.Fatalf("tokens has no %s axis: %v", ax, tokens)
		}
	}
	rules, _ := out["rules"].(map[string]any)
	colorRule, _ := rules[colorsAxis].(map[string]any)
	if colorRule["path"] != "ui.tokens.colors" {
		t.Fatalf("rules.colors.path = %v, want ui.tokens.colors", colorRule["path"])
	}
	if desc, _ := colorRule["description"].(string); desc != mustSetting(t, "ui.tokens.colors").Description {
		t.Fatal("rules.colors.description is not the settings catalog's own text")
	}

	// The catalogs come only with a named axis (the no-argument call is the
	// cheap one); TestUITokensNoArgumentOmitsCatalog pins the other half.
	colorOut, err := callUITool(t, s, toolUITokens, map[string]any{"axis": colorsAxis})
	if err != nil {
		t.Fatalf("gadak_ui_tokens axis=%s: %v", colorsAxis, err)
	}
	catalog, _ := colorOut["catalog"].(map[string]any)
	colors, _ := catalog["colors"].([]any)
	if want := len(tokencheck.CatalogTokens()); len(colors) != want {
		t.Fatalf("catalog.colors has %d rows, tokencheck.CatalogTokens() has %d", len(colors), want)
	}
	dimOut, err := callUITool(t, s, toolUITokens, map[string]any{"axis": "spacing"})
	if err != nil {
		t.Fatalf("gadak_ui_tokens axis=spacing: %v", err)
	}
	dimCatalog, _ := dimOut["catalog"].(map[string]any)
	dims, _ := dimCatalog["dimensions"].([]any)
	if len(dims) == 0 {
		t.Fatal("catalog.dimensions is empty — the dim-catalog owner was not consulted")
	}
	// Every dimension row must carry the clamp/tier fields the write rules use.
	first, _ := dims[0].(map[string]any)
	for _, field := range []string{"axis", "name", "cssVar", "tier", "unit", "default", "relations"} {
		if _, ok := first[field]; !ok {
			t.Fatalf("catalog.dimensions row is missing %q: %v", field, first)
		}
	}
	if _, ok := out["config_file"].(string); !ok {
		t.Fatalf("read tool does not say where the write would land: %v", out)
	}

	// Narrowing to one axis must drop the other catalog, not merely re-label it.
	only, err := callUITool(t, s, toolUITokens, map[string]any{"axis": "spacing"})
	if err != nil {
		t.Fatalf("gadak_ui_tokens axis=spacing: %v", err)
	}
	onlyCat, _ := only["catalog"].(map[string]any)
	if _, ok := onlyCat["colors"]; ok {
		t.Fatal("axis=spacing still carried the color catalog")
	}
	if _, ok := onlyCat["dimensions"]; !ok {
		t.Fatal("axis=spacing carried no dimension catalog")
	}
	if _, err := callUITool(t, s, toolUITokens, map[string]any{"axis": "nope"}); err == nil {
		t.Fatal("gadak_ui_tokens accepted an axis the catalog does not carry")
	}
}

// The whole-object ui.tokens set replaces; the axis merge does not. An agent
// that stores a color and then a spacing must still have the color (GDK-853).
func TestUISetMergesPerAxis(t *testing.T) {
	home := uiHome(t)
	s := uiServer(home)

	if _, err := callUITool(t, s, toolUISet, map[string]any{
		"axis": "colors", "values": map[string]any{"accent": "#7a4bd0"},
	}); err != nil {
		t.Fatalf("set colors: %v", err)
	}
	out, err := callUITool(t, s, toolUISet, map[string]any{
		"axis": "spacing", "values": map[string]any{"row": "44px"},
	})
	if err != nil {
		t.Fatalf("set spacing: %v", err)
	}
	if saved, _ := out["saved"].(bool); !saved {
		t.Fatalf("second write reports saved=false: %v", out)
	}
	doc := readConfigJSON(t, home)
	if got := storedAxis(doc, "colors")["accent"]; got != "#7a4bd0" {
		t.Fatalf("the spacing write dropped the stored color: colors = %v", storedAxis(doc, "colors"))
	}
	if got := storedAxis(doc, "spacing")["row"]; got != "44px" {
		t.Fatalf("spacing.row = %v, want 44px", got)
	}

	// null deletes one key and leaves the axis's other keys alone.
	if _, err := callUITool(t, s, toolUISet, map[string]any{
		"axis": "spacing", "values": map[string]any{"control": "36px"},
	}); err != nil {
		t.Fatalf("set spacing.control: %v", err)
	}
	if _, err := callUITool(t, s, toolUISet, map[string]any{
		"axis": "spacing", "values": map[string]any{"row": nil},
	}); err != nil {
		t.Fatalf("delete spacing.row: %v", err)
	}
	doc = readConfigJSON(t, home)
	sp := storedAxis(doc, "spacing")
	if _, still := sp["row"]; still {
		t.Fatalf("null did not delete spacing.row: %v", sp)
	}
	if sp["control"] != "36px" {
		t.Fatalf("the delete took spacing.control with it: %v", sp)
	}

	// {} is a no-op and writes nothing.
	before, _ := os.ReadFile(filepath.Join(home, "config.json"))
	noop, err := callUITool(t, s, toolUISet, map[string]any{"axis": "colors", "values": map[string]any{}})
	if err != nil {
		t.Fatalf("empty merge: %v", err)
	}
	if saved, _ := noop["saved"].(bool); saved {
		t.Fatalf("empty merge reports saved=true: %v", noop)
	}
	after, _ := os.ReadFile(filepath.Join(home, "config.json"))
	if string(before) != string(after) {
		t.Fatal("empty merge rewrote config.json")
	}
}

// A value that cannot render refuses, names the field, and writes nothing.
func TestUISetRefusesUnparseableValue(t *testing.T) {
	home := uiHome(t)
	s := uiServer(home)

	if _, err := callUITool(t, s, toolUISet, map[string]any{
		"axis": "colors", "values": map[string]any{"accent": "#7a4bd0"},
	}); err != nil {
		t.Fatalf("seed write: %v", err)
	}
	before, _ := os.ReadFile(filepath.Join(home, "config.json"))

	_, err := callUITool(t, s, toolUISet, map[string]any{
		"axis": "colors", "values": map[string]any{"accent": "not-a-color"},
	})
	if err == nil {
		t.Fatal("gadak_ui_set accepted a non-hex color")
	}
	if !strings.Contains(err.Error(), "accent") {
		t.Fatalf("refusal does not name the field: %v", err)
	}
	if !strings.Contains(err.Error(), "hex color") {
		t.Fatalf("refusal does not say what was wrong: %v", err)
	}
	after, _ := os.ReadFile(filepath.Join(home, "config.json"))
	if string(before) != string(after) {
		t.Fatal("a refused write still touched config.json")
	}

	// A non-string value is a shape refusal, not a silent coercion.
	if _, err := callUITool(t, s, toolUISet, map[string]any{
		"axis": "spacing", "values": map[string]any{"row": 44},
	}); err == nil {
		t.Fatal("gadak_ui_set accepted a numeric token value")
	}
	// values is required.
	if _, err := callUITool(t, s, toolUISet, map[string]any{"axis": "colors"}); err == nil {
		t.Fatal("gadak_ui_set accepted a call with no values")
	}
}

// A range violation warns and SAVES — the recorded user decision of
// 2026-08-25 ("대비는 워닝만 떠야지 거절은 아니라고 생각해"). The asymmetry with the
// refusal above is the product, so both halves are pinned. The warning has to
// ride the response: a shell-less host never sees the CLI's stderr.
func TestUISetWarnsAndSavesOutOfRange(t *testing.T) {
	home := uiHome(t)
	s := uiServer(home)

	out, err := callUITool(t, s, toolUISet, map[string]any{
		"axis": "spacing", "values": map[string]any{"row": "200px"},
	})
	if err != nil {
		t.Fatalf("an out-of-range dimension was refused, not warned: %v", err)
	}
	if saved, _ := out["saved"].(bool); !saved {
		t.Fatalf("warned write reports saved=false: %v", out)
	}
	warns, _ := out["warnings"].([]any)
	if len(warns) == 0 {
		t.Fatalf("no warning rode the response: %v", out)
	}
	var measured bool
	for _, w := range warns {
		m, _ := w.(map[string]any)
		if m["token"] == "row" && m["severity"] == "warn" {
			if msg, _ := m["message"].(string); strings.Contains(msg, "200") || m["measured"] != nil {
				measured = true
			}
		}
	}
	if !measured {
		t.Fatalf("the warning does not measure the stored value: %v", warns)
	}
	if got := storedAxis(readConfigJSON(t, home), "spacing")["row"]; got != "200px" {
		t.Fatalf("the warned value was not saved: spacing.row = %v", got)
	}

	// An unknown token name is carried with a warning, never refused
	// (forward-compat, GDK-769 axis 3) — the same downgrade the CLI does.
	out, err = callUITool(t, s, toolUISet, map[string]any{
		"axis": "colors", "values": map[string]any{"not-a-token": "#112233"},
	})
	if err != nil {
		t.Fatalf("an unknown token name was refused: %v", err)
	}
	if got := storedAxis(readConfigJSON(t, home), "colors")["not-a-token"]; got != "#112233" {
		t.Fatalf("the unknown token was not carried: %v", got)
	}
	var sawUnknown bool
	if ws, _ := out["warnings"].([]any); ws != nil {
		for _, w := range ws {
			m, _ := w.(map[string]any)
			if m["rule"] == "unknown-token" {
				sawUnknown = true
			}
		}
	}
	if !sawUnknown {
		t.Fatalf("no unknown-token advisory rode the response: %v", out["warnings"])
	}
}

// Neither tool reads the mirror, so neither may require one: a fresh Claude
// Desktop install has a config long before it has a synced gadak.db.
func TestUIToolsNeedNoMirror(t *testing.T) {
	home := uiHome(t)
	s := uiServer(home)
	if _, err := os.Stat(s.DBPath); err == nil {
		t.Fatal("the fixture created a mirror; this test needs none")
	}
	for _, call := range []struct {
		name string
		args map[string]any
	}{
		{toolUITokens, map[string]any{"axis": "colors"}},
		{toolUISet, map[string]any{"axis": "colors", "values": map[string]any{"accent": "#7a4bd0"}}},
	} {
		content, isErr := s.callTool(call.name, call.args)
		if isErr {
			t.Fatalf("%s failed with no mirror present: %s", call.name, content[0].Text)
		}
	}
}

// The read tool's full payload is the agent's context bill. 96 KiB is a
// budget, not a measurement of today's catalogs — it leaves room for both to
// grow while keeping the answer far under the 256 KiB response cap.
func TestUITokensPayloadBudget(t *testing.T) {
	home := uiHome(t)
	s := uiServer(home)
	items, err := s.toolUITokens(map[string]any{})
	if err != nil {
		t.Fatalf("gadak_ui_tokens: %v", err)
	}
	const budget = 96 * 1024
	if n := len(items[0].Text); n > budget {
		t.Fatalf("full gadak_ui_tokens payload is %d bytes, budget %d — narrow the answer or split the catalogs", n, budget)
	}
}

// Every tool tools/list advertises must be callable over the protocol. This is
// the assertion the round was missing: the ui.tokens pair reached tools/list
// and every Go-level test passed while handleToolsCall still refused both by
// name, because that layer kept its own hand-written list of tool names. A
// shell-less agent is the only caller who would ever have found out.
//
// The call goes through Serve, not through callTool, on purpose — the dispatch
// layer is exactly what was broken.
func TestEveryListedToolIsCallable(t *testing.T) {
	uiHome(t)
	s := &Server{DBPath: filepath.Join(t.TempDir(), "gadak.db")}

	var in strings.Builder
	for i, tool := range toolDefinitions() {
		req := map[string]any{
			"jsonrpc": "2.0", "id": i + 1, "method": "tools/call",
			"params": map[string]any{"name": tool.Name, "arguments": map[string]any{}},
		}
		b, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}
		in.Write(b)
		in.WriteByte('\n')
	}
	var out strings.Builder
	if err := s.Serve(strings.NewReader(in.String()), &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != len(toolDefinitions()) {
		t.Fatalf("got %d responses for %d tools", len(lines), len(toolDefinitions()))
	}
	for i, line := range lines {
		var resp struct {
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("response %d is not JSON: %v", i, err)
		}
		// A tool may answer isError (no mirror, missing argument) — that is a
		// working tool refusing a bad call. A JSON-RPC error is the dispatch
		// layer saying the advertised tool does not exist.
		if resp.Error != nil {
			t.Fatalf("tools/list advertises %s but tools/call answers a protocol error: %s",
				toolDefinitions()[i].Name, resp.Error.Message)
		}
	}
}

// ---------------------------------------------------------------------------
// The cheapest call must be the default call (GDK-w18).
//
// gadak_ui_tokens with no axis used to answer the entire colour catalog — its
// own description said "a few tens of KB". An agent almost always omits an
// optional argument, so the commonest call was the most expensive one. The
// no-argument answer now carries the cheap half for every axis (which axes
// exist, what is stored, the write rules, the standing warnings) and the
// catalog only when an axis is named.
func TestUITokensNoArgumentOmitsCatalog(t *testing.T) {
	home := uiHome(t)
	s := uiServer(home)

	all, err := callUITool(t, s, toolUITokens, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	cat, _ := all["catalog"].(map[string]any)
	if len(cat) != 0 {
		keys := make([]string, 0, len(cat))
		for k := range cat {
			keys = append(keys, k)
		}
		t.Errorf("no-argument answer carries the catalog (%v); it must not", keys)
	}
	// The cheap half is all still there — this is a payload change, not a
	// smaller tool.
	for _, key := range []string{"axes", "tokens", "rules", "warnings"} {
		if _, ok := all[key]; !ok {
			t.Errorf("no-argument answer dropped %q", key)
		}
	}
	if hint, _ := all["catalog_hint"].(string); !strings.Contains(hint, "axis") {
		t.Errorf("no-argument answer should say how to get the catalog, got %q", hint)
	}

	// Naming an axis is what buys the heavy half.
	one, err := callUITool(t, s, toolUITokens, map[string]any{"axis": "colors"})
	if err != nil {
		t.Fatal(err)
	}
	cat, _ = one["catalog"].(map[string]any)
	if _, ok := cat["colors"]; !ok {
		t.Error("axis=colors did not carry the colour catalog")
	}

	// And the description has to say exactly that.
	d := uiTokensDescription()
	if strings.Contains(d, "omit it for every axis (the color catalog is a") {
		t.Error("description still promises the catalog on a no-argument call")
	}
	if !strings.Contains(d, "catalog") || !strings.Contains(d, "axis") {
		t.Error("description should say the catalog comes only with an axis")
	}
}

// ---------------------------------------------------------------------------
// A change must be undoable (GDK-w18). gadak_ui_set returned the merged
// result and nothing else, so an agent asked to "undo that one" had nothing
// unless it still held the old value. The response now carries `previous`:
// the named keys as they were immediately before the merge, in the shape
// `values` takes, so handing it straight back undoes the change.
func TestUISetPreviousUndoesTheChange(t *testing.T) {
	home := uiHome(t)
	s := uiServer(home)

	// An existing key so previous has a real value to restore, and a new key
	// so previous has to carry a null for it.
	if _, err := callUITool(t, s, toolUISet, map[string]any{
		"axis": "colors", "values": map[string]any{"accent": "#3355cc"},
	}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(home, "config.json"))
	if err != nil {
		t.Fatal(err)
	}

	out, err := callUITool(t, s, toolUISet, map[string]any{
		"axis": "colors", "values": map[string]any{"accent": "#aa2211", "bg": "#101014"},
	})
	if err != nil {
		t.Fatal(err)
	}
	prev, ok := out["previous"].(map[string]any)
	if !ok {
		t.Fatalf("response has no previous: %v", out)
	}
	if prev["accent"] != "#3355cc" {
		t.Errorf("previous.accent = %v, want the pre-merge #3355cc", prev["accent"])
	}
	if v, present := prev["bg"]; !present || v != nil {
		t.Errorf("previous.bg = %v (present=%v), want an explicit null so the undo deletes it", v, present)
	}

	// The round trip: hand previous straight back.
	if _, err := callUITool(t, s, toolUISet, map[string]any{"axis": "colors", "values": prev}); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(filepath.Join(home, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Errorf("undo did not restore the config bytes:\nbefore %s\nafter  %s", before, after)
	}
	if !strings.Contains(uiSetDescription(), "previous") {
		t.Error("description does not mention previous")
	}
}

// ---------------------------------------------------------------------------
// Stop teaching what the tool will not do (GDK-w18). The description told the
// reader that ui.tokensByTheme and ui.dataColors are preserved by the merge,
// while neither was reachable over MCP at all. Both are now writable through
// the same settings-catalog paths the axes use.
func TestUISetWritesPerThemeColors(t *testing.T) {
	home := uiHome(t)
	s := uiServer(home)

	out, err := callUITool(t, s, toolUISet, map[string]any{
		"axis": "colors", "palette": "dark", "values": map[string]any{"accent": "#9a6be0"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["path"] != "ui.tokensByTheme" {
		t.Errorf("path = %v, want ui.tokensByTheme", out["path"])
	}
	doc := readConfigJSON(t, home)
	ui, _ := doc["ui"].(map[string]any)
	byTheme, _ := ui["tokensByTheme"].(map[string]any)
	dark, _ := byTheme["dark"].(map[string]any)
	colors, _ := dark["colors"].(map[string]any)
	if colors["accent"] != "#9a6be0" {
		t.Fatalf("ui.tokensByTheme.dark.colors.accent not stored: %v", doc)
	}

	// The catalog's own rule still owns the refusals: a palette-agnostic axis
	// per theme is refused there, not restated here.
	if _, err := callUITool(t, s, toolUISet, map[string]any{
		"axis": "spacing", "palette": "dark", "values": map[string]any{"gap": "8px"},
	}); err == nil {
		t.Error("per-theme spacing should be refused")
	}

	// And undo works the same way here.
	prev, _ := out["previous"].(map[string]any)
	if v, present := prev["accent"]; !present || v != nil {
		t.Errorf("previous.accent = %v (present=%v), want null", v, present)
	}
}

func TestUISetWritesDataColors(t *testing.T) {
	home := uiHome(t)
	s := uiServer(home)

	if _, err := callUITool(t, s, toolUISet, map[string]any{
		"axis": uiDataColorsTarget, "values": map[string]any{"label.urgent": "#c03030"},
	}); err != nil {
		t.Fatal(err)
	}
	doc := readConfigJSON(t, home)
	ui, _ := doc["ui"].(map[string]any)
	data, _ := ui["dataColors"].(map[string]any)
	label, _ := data["label"].(map[string]any)
	if label["urgent"] != "#c03030" {
		t.Fatalf("ui.dataColors.label.urgent not stored: %v", doc)
	}

	// The family rule is the catalog's: a display-name status key is refused
	// there, with the sentence that teaches the right key kind.
	if _, err := callUITool(t, s, toolUISet, map[string]any{
		"axis": uiDataColorsTarget, "values": map[string]any{"status.In Progress": "#7e5904"},
	}); err == nil {
		t.Error("a status display name should be refused")
	}
	// A key that is not family-qualified is refused by name.
	if _, err := callUITool(t, s, toolUISet, map[string]any{
		"axis": uiDataColorsTarget, "values": map[string]any{"urgent": "#c03030"},
	}); err == nil {
		t.Error("an unqualified dataColors key should be refused")
	}
}

// The read tool has to show what the write tool can now reach, or the pair
// teaches a capability with no way to discover its current value.
func TestUITokensReportsThemeAndDataColors(t *testing.T) {
	home := uiHome(t)
	s := uiServer(home)
	if _, err := callUITool(t, s, toolUISet, map[string]any{
		"axis": "colors", "palette": "dark", "values": map[string]any{"accent": "#9a6be0"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := callUITool(t, s, toolUISet, map[string]any{
		"axis": uiDataColorsTarget, "values": map[string]any{"label.urgent": "#c03030"},
	}); err != nil {
		t.Fatal(err)
	}
	out, err := callUITool(t, s, toolUITokens, nil)
	if err != nil {
		t.Fatal(err)
	}
	tokens, _ := out["tokens"].(map[string]any)
	if _, ok := tokens["tokensByTheme"]; !ok {
		t.Error("read tool does not report tokensByTheme")
	}
	if _, ok := tokens[uiDataColorsTarget]; !ok {
		t.Error("read tool does not report dataColors")
	}
}

// The write targets published in the schema enum and named in the prose must
// all be settings-catalog paths, and every one the tool accepts must be
// published — the same assertion the axis enum already carries.
func TestUIWriteTargetsComeFromSettingsCatalog(t *testing.T) {
	for _, target := range uiWriteTargets() {
		path, err := uiTargetPath(target)
		if err != nil {
			t.Errorf("published target %q does not resolve: %v", target, err)
			continue
		}
		if _, ok := config.SettingByPath(path); !ok {
			t.Errorf("target %q resolves to %q, which the settings catalog does not carry", target, path)
		}
	}
	if !slices.Contains(uiWriteTargets(), uiDataColorsTarget) {
		t.Errorf("dataColors is writable but not published")
	}
	// The description must not name a capability the reader cannot have.
	d := uiSetDescription()
	if strings.Contains(d, "ui.tokensByTheme and ui.dataColors are preserved") {
		t.Error("description still teaches the old unreachable-keys sentence")
	}
	if !strings.Contains(d, "palette") {
		t.Error("description does not say how to reach a per-palette colour")
	}
}
