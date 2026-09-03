package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/midagedev/gadak/internal/atomicfile"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/fsperm"
	"github.com/midagedev/gadak/internal/linear"
	"github.com/midagedev/gadak/internal/migrate"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/originbind"
	"github.com/midagedev/gadak/internal/store"
)

const migrateUsage = "usage: gadak --workspace <new name> migrate --from <workspace> [--projects A,B] [--spaces X,Y] [--skip-attachments] [--json]\n" +
	"       gadak --workspace <linear workspace> migrate --from <workspace> --to linear --team <KEY> [--projects A,B] [--limit N] [--dry-run] [--json]"

// cmdMigrate exports a workspace's mirror into a brand-new local-origin
// workspace (GDK-1264): mirror → issuetap fixture YAML → one-shot seed →
// first fill → verification report. The source is only read — its mirror,
// plus one origin round-trip per attachment for the bytes. The target must
// not exist yet: changing an existing workspace's origin is a new
// workspace, never an edit (product invariant).
func cmdMigrate(args []string) error {
	fs := newFlagSet("migrate")
	from := fs.String("from", "", "source workspace whose mirror is exported (read-only; the source keeps working)")
	projectsFlag := fs.String("projects", "", "comma-separated project keys (default: every project in the source mirror)")
	spacesFlag := fs.String("spaces", "", "comma-separated wiki space keys (default: every mirrored space)")
	skipAttach := fs.Bool("skip-attachments", false, "keep attachment metadata only; skip the byte download")
	to := fs.String("to", "", "destination: the built-in tracker (default, a new workspace) or `linear` (the Linear workspace this command runs in)")
	team := fs.String("team", "", "Linear team key that receives the issues (--to linear)")
	limit := fs.Int("limit", 0, "migrate only the first N issues by key (--to linear)")
	dryRun := fs.Bool("dry-run", false, "print the mapping and counts without writing anything (--to linear)")
	jsonOut := fs.Bool("json", false, "emit JSON")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("migrate", fs))
		return nil
	}
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 0 || *from == "" {
		return usageError("migrate", migrateUsage)
	}

	target := config.Profile()
	switch *to {
	case "linear":
		return migrateToLinear(target, *from, *team, *projectsFlag, *limit, *dryRun, *jsonOut)
	case "":
	default:
		return usageError("migrate", migrateUsage)
	}
	if target == "" || target == "default" {
		return fmt.Errorf("migrate creates a new local-origin workspace — name it: gadak --workspace <new name> migrate --from %s", *from)
	}
	if target == *from {
		return fmt.Errorf("--from %s names the target workspace itself; migrate exports into a different, new workspace", *from)
	}
	targetDir, err := config.DirFor(target)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(targetDir, "config.json")); err == nil {
		return fmt.Errorf("workspace %q already exists — migrate only fills a new one (changing an existing workspace's origin is a new workspace)", target)
	}

	srcCfg, err := config.LoadFor(*from)
	if err != nil {
		return err
	}
	srcDBPath, err := config.DBPathFor(*from)
	if err != nil {
		return err
	}
	if _, err := os.Stat(srcDBPath); err != nil {
		return fmt.Errorf("source workspace %q has no mirror yet — run `gadak --workspace %s sync` first", *from, *from)
	}
	srcDB, err := store.OpenReadOnly(srcDBPath)
	if err != nil {
		return err
	}
	defer srcDB.Close()

	ctx := context.Background()
	doc, stats, err := migrate.Build(ctx, srcDB, migrate.Options{
		Projects: originbind.ParseProjectKeys(*projectsFlag),
		Spaces:   splitCSV(*spacesFlag),
	})
	if err != nil {
		return err
	}

	if !*skipAttach && stats.Attachments > 0 {
		client, cerr := origin.Client(srcCfg)
		if cerr != nil {
			// A warning used to be enough here, and it was not: the run
			// still "succeeded", and the verify table still read 26/26,
			// because that row counts rows and the bytes are what went
			// missing (GDK-1275). The cutover procedure — freeze the
			// source, then migrate — walks into exactly this, so the
			// refusal has to come before the export, not after.
			return fmt.Errorf("cannot read attachment bytes from %q: %w\n"+
				"  %d attachments would migrate as empty metadata, and the count table would not say so\n"+
				"  to bring the bytes: make the source reachable (a frozen workspace: `gadak --workspace %s config set frozen false`)\n"+
				"  to migrate without them on purpose: --skip-attachments",
				*from, cerr, stats.Attachments, *from)
		} else {
			migrate.InlineAttachments(ctx, doc, func(ctx context.Context, id string) (int, []byte, error) {
				return client.Raw(ctx, "GET", "/rest/api/3/attachment/content/"+url.PathEscape(id), nil, false)
			}, stats)
		}
	}

	// JSON, not YAML, even though the seed file keeps issuetap's legacy
	// name: yaml.v3's emitter produces block scalars its own parser
	// rejects on real Jira bodies (leading-space/blank-line combinations —
	// measured on the first full GDK export, "did not find expected key").
	// JSON escaping has no such class, and JSON is valid YAML, so
	// issuetap's yaml.Unmarshal reads it unchanged.
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	originDir := filepath.Join(targetDir, filepath.Dir(filepath.FromSlash(origin.LegacyYAMLRel)))
	if err := fsperm.EnsurePrivateDir(targetDir); err != nil {
		return err
	}
	if err := fsperm.EnsurePrivateDir(originDir); err != nil {
		return err
	}
	yamlPath := filepath.Join(targetDir, filepath.FromSlash(origin.LegacyYAMLRel))
	if err := atomicfile.WriteFile(yamlPath, "issuetap-*.yaml", data); err != nil {
		return err
	}

	// The YAML is in place before the first origin.Client call inside
	// SeedLocalOrigin, so issuetap's one-shot legacy seed picks it up —
	// reversed, the workspace would silently seed the default STD project.
	tcfg, err := config.LoadFor(target)
	if err != nil {
		return err
	}
	fillErr, err := originbind.SeedLocalOrigin(tcfg, strings.Join(stats.Projects, ","), stats.Spaces,
		func() (*store.DB, func() error, error) {
			p, err := config.DBPathFor(target)
			if err != nil {
				return nil, nil, err
			}
			db, err := store.Open(p)
			if err != nil {
				return nil, nil, err
			}
			return db, db.Close, nil
		})
	if err != nil {
		return err
	}
	if err := origin.Close(); err != nil {
		return fmt.Errorf("flush origin persist: %w", err)
	}

	var verify []migrate.VerifyRow
	if fillErr != nil {
		fmt.Fprintf(os.Stderr, "warning: could not fill the new mirror yet (%v) — run `gadak --workspace %s sync`, then compare counts by hand\n", fillErr, target)
	} else {
		tdbPath, err := config.DBPathFor(target)
		if err != nil {
			return err
		}
		tdb, err := store.OpenReadOnly(tdbPath)
		if err != nil {
			return err
		}
		verify, err = migrate.VerifyMirror(ctx, tdb, stats)
		_ = tdb.Close()
		if err != nil {
			return err
		}
	}

	if *jsonOut {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"workspace": target,
			"from":      *from,
			"persist":   origin.PersistPath(targetDir),
			"stats":     stats,
			"verify":    verify,
		})
	}
	printMigrateReport(os.Stdout, target, *from, stats, verify)
	return nil
}

// migrateToLinear is the second destination (GDK-1265): the source mirror
// leaves through the Linear write verbs into team `team` of the Linear
// workspace this command runs in. No workspace is created — Linear is the
// origin already, and `gadak sync` fills its mirror afterwards. --dry-run
// makes no network call.
func migrateToLinear(target, from, team, projects string, limit int, dryRun, jsonOut bool) error {
	if team == "" {
		return usageError("migrate", migrateUsage)
	}
	if target == from {
		return fmt.Errorf("--from %s names the target workspace itself", from)
	}
	tcfg, err := config.LoadFor(target)
	if err != nil {
		return err
	}
	if tcfg.OriginType() != config.OriginLinear {
		return fmt.Errorf("workspace %q is not a Linear workspace (origin: %s) — --to linear writes through the Linear credential of the workspace it runs in", target, tcfg.OriginType())
	}
	srcCfg, err := config.LoadFor(from)
	if err != nil {
		return err
	}
	srcDBPath, err := config.DBPathFor(from)
	if err != nil {
		return err
	}
	if _, err := os.Stat(srcDBPath); err != nil {
		return fmt.Errorf("source workspace %q has no mirror yet — run `gadak --workspace %s sync` first", from, from)
	}
	srcDB, err := store.OpenReadOnly(srcDBPath)
	if err != nil {
		return err
	}
	defer srcDB.Close()

	ctx := context.Background()
	doc, stats, err := migrate.Build(ctx, srcDB, migrate.Options{Projects: originbind.ParseProjectKeys(projects)})
	if err != nil {
		return err
	}
	opt := migrate.LinearOptions{TeamKey: team, Limit: limit, DryRun: dryRun, Progress: os.Stderr}
	if srcCfg.Site != "" {
		site := strings.TrimRight(srcCfg.Site, "/")
		opt.AttachmentURL = func(id string) string {
			return site + "/rest/api/3/attachment/content/" + url.PathEscape(id)
		}
	}
	var client *linear.Client
	if !dryRun {
		if client, err = origin.Linear(tcfg); err != nil {
			return err
		}
	}
	rep, err := migrate.ToLinear(ctx, client, doc, stats, opt)
	if err != nil {
		return err
	}
	if jsonOut {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"workspace": target, "from": from, "to": "linear", "stats": stats, "report": rep,
		})
	}
	w := os.Stdout
	verb := "migrated"
	if rep.DryRun {
		verb = "dry-run:"
	}
	fmt.Fprintf(w, "%s %s → Linear team %s (workspace %q)\n", verb, from, rep.Team, target)
	fmt.Fprintf(w, "projects: %s\n\nmapping:\n", strings.Join(stats.Projects, ", "))
	for _, m := range rep.Mapping {
		fmt.Fprintf(w, "  %s\n", m)
	}
	fmt.Fprintf(w, "\n%-12s %8s %8s %16s\n", "metric", "source", "migrated", "already there")
	for _, r := range rep.Counts {
		mark := ""
		if !rep.DryRun && r.Source != r.Migrated {
			mark = "  MISMATCH"
		}
		fmt.Fprintf(w, "%-12s %8d %8d %16d%s\n", r.Metric, r.Source, r.Migrated, r.Skipped, mark)
	}
	fmt.Fprintln(w, "\nnot migrated:")
	for _, n := range rep.NotMigrated {
		fmt.Fprintf(w, "  %s\n", n)
	}
	for _, wn := range rep.Warnings {
		fmt.Fprintf(w, "warning: %s\n", wn)
	}
	if !rep.DryRun {
		fmt.Fprintf(w, "\nnext: gadak --workspace %s sync\n", target)
	}
	return nil
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func printMigrateReport(w *os.File, target, from string, st *migrate.Stats, verify []migrate.VerifyRow) {
	fmt.Fprintf(w, "migrated %s → local-origin workspace %q\n", from, target)
	fmt.Fprintf(w, "projects: %s", strings.Join(st.Projects, ", "))
	if len(st.Spaces) > 0 {
		fmt.Fprintf(w, "  spaces: %s", strings.Join(st.Spaces, ", "))
	}
	fmt.Fprintln(w)

	if len(verify) > 0 {
		fmt.Fprintf(w, "\n%-16s %8s %8s\n", "metric", "source", "migrated")
		allOK := true
		for _, r := range verify {
			mark := ""
			if r.Source != r.Migrated {
				mark = "  MISMATCH"
				allOK = false
			}
			fmt.Fprintf(w, "%-16s %8d %8d%s\n", r.Metric, r.Source, r.Migrated, mark)
		}
		if !allOK {
			fmt.Fprintln(w, "some counts differ — the lines above say which; the source workspace is untouched")
		}
	}

	if st.Attachments > 0 {
		fmt.Fprintf(w, "attachments: %d inlined", st.AttachInlined)
		if n := st.Attachments - st.AttachInlined; n > 0 {
			fmt.Fprintf(w, ", %d metadata-only (missing at origin %d, over size cap %d, non-Jira source %d, errors %d)",
				n, st.AttachMissing, st.AttachTooLarge, st.AttachSkipURL, len(st.AttachErrors))
		}
		fmt.Fprintln(w)
		for _, e := range st.AttachErrors {
			fmt.Fprintf(w, "  ! %s\n", e)
		}
	}
	if st.FmtCodeBlock+st.FmtMedia+st.FmtTable > 0 {
		fmt.Fprintf(w, "formatting carried as ADF: code blocks %d, tables %d, inline media %d (an image resolves by filename against its migrated attachment)\n",
			st.FmtCodeBlock, st.FmtTable, st.FmtMedia)
	}
	if st.DevLinks > 0 || st.CustomIssues > 0 || st.SprintIssues > 0 {
		fmt.Fprintf(w, "not migrated: dev links %d, issues with custom fields %d, issues with sprints %d\n",
			st.DevLinks, st.CustomIssues, st.SprintIssues)
	}
	if st.DroppedLinks > 0 {
		fmt.Fprintf(w, "links to issues outside the migrated set: %d dropped\n", st.DroppedLinks)
	}
	if len(st.DroppedParents) > 0 {
		fmt.Fprintf(w, "parents outside the migrated set: %s\n", strings.Join(st.DroppedParents, ", "))
	}
	if st.DroppedPageParents > 0 {
		fmt.Fprintf(w, "page parents outside the migrated set: %d dropped\n", st.DroppedPageParents)
	}
	if len(st.MissingUsers) > 0 {
		fmt.Fprintf(w, "accounts no longer in the user catalog (kept as ghost users): %s\n", strings.Join(st.MissingUsers, ", "))
	}
	fmt.Fprintf(w, "\nnext: gadak --workspace %s status\n", target)
}
