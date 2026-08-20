package main

// gadak fields reports which Jira fields are actually populated on a stratified
// sample of mirrored issues. The mirror only stores mapped custom fields, so
// this command must ask Jira (field catalog + *all search) rather than run a
// pure SQL report over the local database.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/mattn/go-runewidth"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/fields"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/jirafields"
	"github.com/midagedev/gadak/internal/origin"
)

// mapSuggestMinRate is the filled-fraction threshold above which an unmapped
// custom field is suggested for fields. Sample-based, not a site census.
const mapSuggestMinRate = 0.10

// fieldsSampleBatch is how many issue keys go into one Search JQL `key in (...)`.
const fieldsSampleBatch = 100

// issueSampleRow is one mirrored issue used for stratified sampling.
type issueSampleRow struct {
	Key        string
	ProjectKey string
	CreatedAt  string
}

// fieldUsageRow is one field's fill statistics over the sample.
type fieldUsageRow struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Custom    bool    `json:"custom"`
	Type      string  `json:"type"`
	Filled    int     `json:"filled"`
	Sampled   int     `json:"sampled"`
	Rate      float64 `json:"rate"`
	Alias     string  `json:"alias,omitempty"`
	Suggested string  `json:"suggested_alias,omitempty"`
}

type fieldReport struct {
	rows         []fieldUsageRow
	unused       []fieldUsageRow
	suggestedMap map[string]string // alias -> field id
}

func cmdFields(args []string) error {
	fs := newFlagSet("fields")
	sampleN := fs.Int("sample", 200, "number of mirrored issues to sample")
	asJSON := fs.Bool("json", false, "emit JSON")
	showAll := fs.Bool("all", false, "include system fields (default: custom only)")
	project := fs.String("project", "", "limit the sample to one project key")
	apply := fs.Bool("apply", false, "discover custom fields from the mirror and save specs")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *apply {
		return cmdFieldsApply(*asJSON)
	}
	if *sampleN < 1 {
		return errors.New("--sample must be at least 1")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !cfg.HasCredential() {
		return config.NotConfiguredf("this command queries Jira, not only the mirror")
	}

	db, err := openReadOnly()
	if err != nil {
		return err
	}
	defer db.Close()
	warnIfStale()

	projectFilter := strings.ToUpper(strings.TrimSpace(*project))
	allRows, totalMirrored, allProjectCount, err := loadIssueSampleRows(db, "")
	if err != nil {
		return err
	}
	if totalMirrored == 0 {
		if *asJSON {
			return json.NewEncoder(os.Stdout).Encode(map[string]any{
				"sample":   0,
				"mirrored": 0,
				"projects": 0,
				"message":  "mirror is empty — run `gadak sync` first",
				"fields":   []fieldUsageRow{},
			})
		}
		fmt.Println("mirror is empty — run `gadak sync` first")
		return nil
	}

	samplePool := allRows
	if projectFilter != "" {
		samplePool = filterByProject(allRows, projectFilter)
		if len(samplePool) == 0 {
			msg := fmt.Sprintf("no mirrored issues match --project %s", projectFilter)
			if *asJSON {
				return json.NewEncoder(os.Stdout).Encode(map[string]any{
					"sample":   0,
					"mirrored": totalMirrored,
					"projects": allProjectCount,
					"message":  msg,
					"fields":   []fieldUsageRow{},
				})
			}
			fmt.Println(msg)
			return nil
		}
	}

	keys := sampleIssueKeys(samplePool, *sampleN)
	headerProjects := allProjectCount
	if projectFilter != "" {
		headerProjects = 1
	}

	ctx := context.Background()
	c, err := origin.Client(cfg)
	if err != nil {
		return err
	}

	catalog, err := c.Fields(ctx)
	if err != nil {
		return fmt.Errorf("list fields: %w", err)
	}

	filled, err := probeFieldFills(ctx, c, keys)
	if err != nil {
		return err
	}

	report := buildFieldReport(catalog, filled, len(keys), cfg.Fields, *showAll)
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"sample":             len(keys),
			"mirrored":           totalMirrored,
			"projects":           headerProjects,
			"sample_note":        "rates are over the stratified sample, not a site-wide census",
			"fields":             report.rows,
			"unused_custom":      report.unused,
			"suggested_fieldMap": report.suggestedMap,
		})
	}
	printFieldReport(report, len(keys), totalMirrored, headerProjects)
	return nil
}

func loadIssueSampleRows(db *sql.DB, projectFilter string) ([]issueSampleRow, int, int, error) {
	var total int
	if err := db.QueryRow(`SELECT COUNT(*) FROM issues`).Scan(&total); err != nil {
		return nil, 0, 0, err
	}
	var projectCount int
	if err := db.QueryRow(`SELECT COUNT(DISTINCT project_key) FROM issues`).Scan(&projectCount); err != nil {
		return nil, 0, 0, err
	}

	q := `SELECT key, project_key, COALESCE(created_at, '') FROM issues`
	var args []any
	if projectFilter != "" {
		q += ` WHERE project_key = ?`
		args = append(args, projectFilter)
	}
	q += ` ORDER BY project_key, created_at, key`
	rs, err := db.Query(q, args...)
	if err != nil {
		return nil, total, projectCount, err
	}
	defer rs.Close()

	var rows []issueSampleRow
	for rs.Next() {
		var r issueSampleRow
		if err := rs.Scan(&r.Key, &r.ProjectKey, &r.CreatedAt); err != nil {
			return nil, total, projectCount, err
		}
		rows = append(rows, r)
	}
	if err := rs.Err(); err != nil {
		return nil, total, projectCount, err
	}
	return rows, total, projectCount, nil
}

func filterByProject(rows []issueSampleRow, project string) []issueSampleRow {
	out := make([]issueSampleRow, 0, len(rows))
	for _, r := range rows {
		if r.ProjectKey == project {
			out = append(out, r)
		}
	}
	return out
}

// sampleIssueKeys picks up to n keys stratified by project, evenly spaced
// across each project's created_at-ordered list. Deterministic for the same
// input order (callers must pass rows ordered by project, created_at, key).
func sampleIssueKeys(rows []issueSampleRow, n int) []string {
	if n <= 0 || len(rows) == 0 {
		return nil
	}
	if n >= len(rows) {
		out := make([]string, len(rows))
		for i, r := range rows {
			out[i] = r.Key
		}
		return out
	}

	// Group while preserving created_at order within each project.
	byProj := map[string][]issueSampleRow{}
	var projects []string
	for _, r := range rows {
		if _, ok := byProj[r.ProjectKey]; !ok {
			projects = append(projects, r.ProjectKey)
		}
		byProj[r.ProjectKey] = append(byProj[r.ProjectKey], r)
	}
	sort.Strings(projects)

	counts := make(map[string]int, len(projects))
	total := 0
	for _, p := range projects {
		counts[p] = len(byProj[p])
		total += counts[p]
	}
	quotas := allocateSampleQuotas(projects, counts, n)

	var out []string
	for _, p := range projects {
		q := quotas[p]
		if q <= 0 {
			continue
		}
		group := byProj[p]
		// Within a project, order by created_at then key for deterministic spacing.
		sort.SliceStable(group, func(i, j int) bool {
			if group[i].CreatedAt != group[j].CreatedAt {
				return group[i].CreatedAt < group[j].CreatedAt
			}
			return group[i].Key < group[j].Key
		})
		for _, idx := range evenIndices(len(group), q) {
			out = append(out, group[idx].Key)
		}
	}
	// Stable overall order for reproducibility of JQL batches.
	sort.Strings(out)
	return out
}

// allocateSampleQuotas spreads n slots across projects proportionally to their
// sizes (Hamilton / largest-remainder). Deterministic: ties break by project key.
func allocateSampleQuotas(projects []string, counts map[string]int, n int) map[string]int {
	out := make(map[string]int, len(projects))
	if n <= 0 || len(projects) == 0 {
		return out
	}
	total := 0
	for _, p := range projects {
		total += counts[p]
	}
	if total == 0 {
		return out
	}
	if n >= total {
		for _, p := range projects {
			out[p] = counts[p]
		}
		return out
	}

	type rem struct {
		p string
		r int
	}
	remainders := make([]rem, 0, len(projects))
	assigned := 0
	for _, p := range projects {
		// floor(n * count / total)
		base := n * counts[p] / total
		if base > counts[p] {
			base = counts[p]
		}
		out[p] = base
		assigned += base
		remainders = append(remainders, rem{p: p, r: n*counts[p] - base*total})
	}
	sort.Slice(remainders, func(i, j int) bool {
		if remainders[i].r != remainders[j].r {
			return remainders[i].r > remainders[j].r
		}
		return remainders[i].p < remainders[j].p
	})
	for i := 0; assigned < n && i < len(remainders); i++ {
		p := remainders[i].p
		if out[p] < counts[p] {
			out[p]++
			assigned++
		}
	}
	// If some projects were full, keep walking until n is met.
	for assigned < n {
		progress := false
		for _, p := range projects {
			if assigned >= n {
				break
			}
			if out[p] < counts[p] {
				out[p]++
				assigned++
				progress = true
			}
		}
		if !progress {
			break
		}
	}
	return out
}

// evenIndices returns want distinct indices spread across [0, count).
func evenIndices(count, want int) []int {
	if want <= 0 || count <= 0 {
		return nil
	}
	if want >= count {
		out := make([]int, count)
		for i := range out {
			out[i] = i
		}
		return out
	}
	out := make([]int, want)
	if want == 1 {
		out[0] = count / 2
		return out
	}
	for i := 0; i < want; i++ {
		out[i] = i * (count - 1) / (want - 1)
	}
	return out
}

// probeFieldFills searches issues in batches with fields=*all and counts how
// many times each field id is filled.
func probeFieldFills(ctx context.Context, c *jira.Client, keys []string) (map[string]int, error) {
	filled := map[string]int{}
	for i := 0; i < len(keys); i += fieldsSampleBatch {
		end := i + fieldsSampleBatch
		if end > len(keys) {
			end = len(keys)
		}
		batch := keys[i:end]
		jql := "key in (" + quoteJiraKeys(batch) + ")"
		err := c.Search(ctx, jql, []string{"*all"}, false, func(issues []jira.Issue) error {
			for _, iss := range issues {
				for id, raw := range iss.Extra {
					if fields.IsFilled(raw) {
						filled[id]++
					}
				}
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("search sample batch: %w", err)
		}
	}
	return filled, nil
}

// cmdFieldsApply runs discovery against the mirror (catalog + raw fill stats),
// saves cfg.Fields, reingests custom/FTS, and refreshes field_usage.
func cmdFieldsApply(asJSON bool) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !cfg.HasCredential() {
		return config.ErrNotConfigured
	}
	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()

	ctx := context.Background()
	has, err := db.HasCustomFieldKeysInRaw(ctx)
	if err != nil {
		return err
	}
	if !has {
		msg := "mirror was synced without custom fields — run `gadak sync --full` first, then re-run"
		if asJSON {
			return json.NewEncoder(os.Stdout).Encode(map[string]any{"message": msg, "applied": 0})
		}
		fmt.Println(msg)
		return nil
	}

	c, err := origin.Client(cfg)
	if err != nil {
		return err
	}
	catalog, err := c.Fields(ctx)
	if err != nil {
		return fmt.Errorf("list fields: %w", err)
	}
	fill := map[string]int{}
	if err := db.ScanFieldFill(ctx, func(_ string, fieldVals map[string]json.RawMessage) error {
		for id, raw := range fieldVals {
			if fields.IsFilled(raw) {
				fill[id]++
			}
		}
		return nil
	}); err != nil {
		return err
	}

	specs := jirafields.Discover(catalog, fill, cfg.Fields)
	cfg.Fields = specs
	if err := cfg.Save(); err != nil {
		return err
	}
	bodyIDs := fields.BodyFieldIDs(cfg.BodyFields, specs)
	if _, err := db.ReingestCustom(ctx, fields.SpecIDsFrom(specs), bodyIDs); err != nil {
		return err
	}
	aliases := make([]string, 0, len(specs))
	for _, s := range specs {
		if s.Alias != "" {
			aliases = append(aliases, s.Alias)
		}
	}
	usage, err := db.ComputeFieldUsage(ctx, aliases)
	if err != nil {
		return err
	}
	if err := db.ReplaceFieldUsage(ctx, usage); err != nil {
		return err
	}

	if asJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"applied": len(specs),
			"fields":  specs,
		})
	}
	for _, s := range specs {
		kind := s.Role
		if s.Kind != "" {
			kind = s.Role + "/" + s.Kind
		}
		fmt.Printf("%s  %s  %s  ids=%d\n", s.Alias, s.Label, kind, len(s.IDs))
	}
	fmt.Printf("applied %d field specs — restart `gadak serve` to pick them up\n", len(specs))
	return nil
}

func quoteJiraKeys(keys []string) string {
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%q", k))
	}
	return strings.Join(parts, ", ")
}

// idsToAlias turns FieldSpec ids into id→alias (first alias wins if duplicates).
func idsToAlias(specs []config.FieldSpec) map[string]string {
	out := make(map[string]string)
	type pair struct{ id, alias string }
	var pairs []pair
	for _, s := range specs {
		if s.Alias == "" {
			continue
		}
		for _, id := range s.IDs {
			if id == "" {
				continue
			}
			pairs = append(pairs, pair{id: id, alias: s.Alias})
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].id != pairs[j].id {
			return pairs[i].id < pairs[j].id
		}
		return pairs[i].alias < pairs[j].alias
	})
	for _, p := range pairs {
		if _, ok := out[p.id]; !ok {
			out[p.id] = p.alias
		}
	}
	return out
}

func buildFieldReport(catalog []jira.FieldInfo, filled map[string]int, sampled int, specs []config.FieldSpec, showAll bool) fieldReport {
	idToAlias := idsToAlias(specs)
	usedAliases := map[string]bool{}
	for _, s := range specs {
		if s.Alias != "" {
			usedAliases[s.Alias] = true
		}
	}

	var rows []fieldUsageRow
	for _, f := range catalog {
		if !showAll && !f.Custom {
			continue
		}
		n := filled[f.ID]
		rate := 0.0
		if sampled > 0 {
			rate = float64(n) / float64(sampled)
		}
		row := fieldUsageRow{
			ID:      f.ID,
			Name:    f.Name,
			Custom:  f.Custom,
			Type:    f.Schema.Type,
			Filled:  n,
			Sampled: sampled,
			Rate:    rate,
			Alias:   idToAlias[f.ID],
		}
		if f.Custom && row.Alias == "" && rate >= mapSuggestMinRate {
			row.Suggested = fields.SuggestAlias(f.Name, f.ID, usedAliases)
			usedAliases[row.Suggested] = true
		}
		rows = append(rows, row)
	}

	// Sort by usage descending, then name, then id.
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Filled != rows[j].Filled {
			return rows[i].Filled > rows[j].Filled
		}
		if rows[i].Name != rows[j].Name {
			return rows[i].Name < rows[j].Name
		}
		return rows[i].ID < rows[j].ID
	})

	var used, unused []fieldUsageRow
	suggested := map[string]string{}
	for _, r := range rows {
		if r.Custom && r.Filled == 0 {
			unused = append(unused, r)
		} else {
			used = append(used, r)
		}
		if r.Suggested != "" {
			suggested[r.Suggested] = r.ID
		}
	}
	// When showAll, unused is custom-only bloat; used holds the rest.
	// When custom-only, used is non-zero custom fields.
	return fieldReport{rows: used, unused: unused, suggestedMap: suggested}
}

func printFieldReport(report fieldReport, sampleN, mirrored, projects int) {
	fmt.Printf("sample: %d issues of %d mirrored (%d projects)\n", sampleN, mirrored, projects)
	fmt.Println("(rates are over this stratified sample, not a site-wide census)")
	fmt.Println()

	printUsageTable(report.rows)

	if len(report.unused) > 0 {
		fmt.Println()
		fmt.Printf("Unused custom fields (0%% of sample) — field bloat candidates (%d):\n", len(report.unused))
		for _, r := range report.unused {
			fmt.Printf("  %s\t%s\t%s\n", r.Name, r.ID, r.Type)
		}
	}

	if len(report.suggestedMap) > 0 {
		fmt.Println()
		fmt.Printf("Suggested fields (≥%.0f%% filled, not in config):\n", mapSuggestMinRate*100)
		// Stable key order for paste-friendly output.
		aliases := make([]string, 0, len(report.suggestedMap))
		for a := range report.suggestedMap {
			aliases = append(aliases, a)
		}
		sort.Strings(aliases)
		fmt.Println("{")
		for i, a := range aliases {
			comma := ","
			if i == len(aliases)-1 {
				comma = ""
			}
			fmt.Printf("  %q: %q%s\n", a, report.suggestedMap[a], comma)
		}
		fmt.Println("}")
	}
}

func printUsageTable(rows []fieldUsageRow) {
	if len(rows) == 0 {
		fmt.Println("(no fields with non-zero fill in the sample)")
		return
	}
	// Column widths in terminal cells, not bytes or runes: Jira returns field
	// names in the account's language, so a Korean or Japanese name is routine
	// here and each of its characters occupies two cells.
	wName, wID, wType, wFilled, wAlias := 4, 2, 4, 6, 5 // header minima
	aliasOf := func(r fieldUsageRow) string {
		if r.Alias == "" && r.Suggested != "" {
			return r.Suggested + " *"
		}
		return r.Alias
	}
	for _, r := range rows {
		wName = max(wName, runewidth.StringWidth(r.Name))
		wID = max(wID, runewidth.StringWidth(r.ID))
		wType = max(wType, runewidth.StringWidth(r.Type))
		wFilled = max(wFilled, len(fmt.Sprintf("%d", r.Filled)))
		wAlias = max(wAlias, runewidth.StringWidth(aliasOf(r)))
	}
	pad := func(s string, w int) string { return runewidth.FillRight(s, w) }

	fmt.Printf("%s  %s  %s  %*s  %6s  %s\n",
		pad("NAME", wName), pad("ID", wID), pad("TYPE", wType), wFilled, "FILLED", "RATE", "ALIAS")
	for _, r := range rows {
		fmt.Printf("%s  %s  %s  %*d  %5.1f%%  %s\n",
			pad(r.Name, wName), pad(r.ID, wID), pad(r.Type, wType),
			wFilled, r.Filled, r.Rate*100, aliasOf(r))
	}
	fmt.Println("(* = alias suggestion; the field is not in fields yet)")
}
