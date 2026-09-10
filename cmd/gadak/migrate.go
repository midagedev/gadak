package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/midagedev/gadak/internal/atomicfile"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/fsperm"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/linear"
	"github.com/midagedev/gadak/internal/migrate"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/originbind"
	"github.com/midagedev/gadak/internal/store"
)

const migrateUsage = "usage: gadak --workspace <new name> migrate --from <workspace> [--projects A,B] [--spaces X,Y] [--skip-attachments] [--json]\n" +
	"       gadak --workspace <linear workspace> migrate --from <workspace> --to linear --team <KEY> [--projects A,B] [--limit N] [--dry-run] [--json]\n" +
	"       gadak --workspace <jira workspace> migrate --from <workspace> --to jira --project <KEY> [--projects A,B] [--limit N] [--skip-attachments] [--dry-run] [--json]"

// cmdMigrate exports a workspace's mirror into a brand-new built-in
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
	to := fs.String("to", "", "destination: the built-in tracker (default, a new workspace), `linear`, or `jira` (the workspace this command runs in)")
	team := fs.String("team", "", "Linear team key that receives the issues (--to linear)")
	project := fs.String("project", "", "Jira project key that receives the issues (--to jira)")
	limit := fs.Int("limit", 0, "migrate only the first N issues by key (--to linear, --to jira)")
	dryRun := fs.Bool("dry-run", false, "print the mapping and counts without writing anything (--to linear, --to jira)")
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
	// --team and --project name the destination's own unit; each belongs to
	// exactly one --to, and saying which is the fix beats a usage dump.
	switch *to {
	case "linear":
		if *project != "" {
			return fmt.Errorf("--project is the Jira destination's flag; --to linear takes --team <KEY>")
		}
		return migrateToLinear(target, *from, *team, *projectsFlag, *limit, *dryRun, *jsonOut)
	case "jira":
		if *team != "" {
			return fmt.Errorf("--team is the Linear destination's flag; --to jira takes --project <KEY>")
		}
		if *project == "" {
			return fmt.Errorf("--to jira needs the project that receives the issues: --project <KEY>")
		}
		return migrateToJira(target, *from, *project, *projectsFlag, *limit, *skipAttach, *dryRun, *jsonOut)
	case "":
		if *team != "" || *project != "" {
			return fmt.Errorf("--team and --project name a destination's unit — add --to linear or --to jira (the default destination is a new built-in workspace)")
		}
	default:
		return usageError("migrate", migrateUsage)
	}
	if target == "" || target == "default" {
		return fmt.Errorf("migrate creates a new built-in workspace — name it: gadak --workspace <new name> migrate --from %s", *from)
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

	// The attachment byte source. nil means the seed carries metadata only
	// (--skip-attachments, or a source with no attachments at all).
	var fetch migrate.StreamFetch
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
		}
		fetch = func(ctx context.Context, id string) (int, int64, io.ReadCloser, error) {
			// Stream, not Raw: Raw reads through a 64 MiB io.LimitReader,
			// which on a large attachment returns a truncated prefix with no
			// error — the same silent loss GDK-1614 fixed on the upload
			// side, on the migrate side (GDK-1617). The body is handed on
			// unread: migrate.WriteDoc base64-encodes it straight into the
			// seed file, so the bytes never accumulate in memory (GDK-1618).
			res, err := client.Stream(ctx, "GET", "/rest/api/3/attachment/content/"+url.PathEscape(id), nil)
			if err != nil {
				return 0, 0, nil, err
			}
			if res.StatusCode != 200 {
				_ = res.Body.Close()
				return res.StatusCode, 0, nil, nil
			}
			return res.StatusCode, res.ContentLength, res.Body, nil
		}
	}

	originDir := filepath.Join(targetDir, filepath.Dir(filepath.FromSlash(origin.LegacyYAMLRel)))
	if err := fsperm.EnsurePrivateDir(targetDir); err != nil {
		return err
	}
	if err := fsperm.EnsurePrivateDir(originDir); err != nil {
		return err
	}
	yamlPath := filepath.Join(targetDir, filepath.FromSlash(origin.LegacyYAMLRel))
	if err := atomicfile.WriteStream(yamlPath, "issuetap-*.yaml", func(w io.Writer) error {
		return migrate.WriteDoc(ctx, w, doc, fetch, stats)
	}); err != nil {
		return err
	}

	// The YAML is in place before the first origin.Client call inside
	// SeedBuiltIn, so issuetap's one-shot legacy seed picks it up —
	// reversed, the workspace would silently seed the default STD project.
	tcfg, err := config.LoadFor(target)
	if err != nil {
		return err
	}
	// GDK-1561: the display-name language is part of what migrate carries.
	// issuetap stores ids and overlays names by the workspace locale, so a
	// target left at the default (en) shows English status and type chips
	// under Korean prose — the seed moves the rows, but the overlay owns the
	// names the origin knows the ids for. The source's locale setting is the
	// one owner of "what language this data reads in" that the source has; a
	// connected source carries none (the account's language is not ours to
	// read) and inherits nothing. Set before SeedBuiltIn so the workspace is
	// born speaking it — construction, not a rebuild after the fact.
	tcfg.Locale = srcCfg.Locale
	// GDK-1484: the migrated workspace's wiki scope comes from this export,
	// never from the built-in default. An export with no wiki page hands
	// over an empty list — "every space this origin has" — because the LOC
	// key the default would leave behind belongs to the source origin and is
	// not in the one this import just seeded: the pass would 404 it and
	// mirror zero pages on every run, silently before GDK-1484.
	wiki := &config.ConfluenceConfig{Spaces: stats.Spaces}
	fillErr, err := originbind.SeedBuiltIn(tcfg, strings.Join(stats.Projects, ","), wiki,
		func() (*store.DB, func() error, error) {
			p, err := config.DBPathFor(target)
			if err != nil {
				return nil, nil, err
			}
			// The target is a workspace this command just minted (a
			// pre-existing one was refused above), so its mirror is a
			// brand-new file the command owns: plain Open may create and
			// migrate it freely (the dev-lockout policy is for
			// release-written mirrors).
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
			"locale":    tcfg.Locale,
			"stats":     stats,
			"verify":    verify,
		})
	}
	printMigrateReport(os.Stdout, target, *from, tcfg.Locale, stats, verify)
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
	printMigrateToReport(os.Stdout, from, target,
		fmt.Sprintf("Linear team %s", rep.Team), rep.DryRun,
		stats.Projects, rep.Mapping, rep.Counts, rep.NotMigrated, rep.Warnings)
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

func printMigrateReport(w *os.File, target, from, locale string, st *migrate.Stats, verify []migrate.VerifyRow) {
	fmt.Fprintf(w, "migrated %s → built-in workspace %q\n", from, target)
	fmt.Fprintf(w, "projects: %s", strings.Join(st.Projects, ", "))
	if len(st.Spaces) > 0 {
		fmt.Fprintf(w, "  spaces: %s", strings.Join(st.Spaces, ", "))
	}
	fmt.Fprintln(w)
	// GDK-1561: say it when the language came along, because the strip of
	// English chips under translated prose is what a silent default looked
	// like. Absent means English, the same nothing the source had.
	if locale != "" {
		fmt.Fprintf(w, "locale: %s (inherited from %s)\n", locale, from)
	}

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
		fmt.Fprintf(w, "attachments: %d inlined (%d bytes)", st.AttachInlined, st.AttachBytes)
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
	// GDK-1491: where the priorities came from. An id-less mirror used to
	// land every issue on the target's default with no line here at all.
	if st.PriorityIDsDerived {
		fmt.Fprintf(w, "priorities: the source mirror carries no priority ids — catalog derived from priority_rank, issues keyed by rank\n")
	}
	if st.PriorityDefaulted > 0 {
		fmt.Fprintf(w, "issues with no priority in the source (land on the target default): %d\n", st.PriorityDefaulted)
	}
	if len(st.MissingUsers) > 0 {
		fmt.Fprintf(w, "accounts no longer in the user catalog (kept as ghost users): %s\n", strings.Join(st.MissingUsers, ", "))
	}
	fmt.Fprintf(w, "\nnext: gadak --workspace %s status\n", target)
}

// migrateToJira is the third destination (GDK-378): the source mirror
// leaves through the Jira write verbs into project `project` of the Jira
// workspace this command runs in. It is the exit `init --replace-local`
// never had — that verb deletes locally originated issues from the mirror,
// this one carries them out first. No workspace is created; Jira is the
// origin already, and `gadak sync` fills its mirror afterwards. --dry-run
// makes no network call.
func migrateToJira(target, from, project, projects string, limit int, skipAttach, dryRun, jsonOut bool) error {
	if target == from {
		return fmt.Errorf("--from %s names the target workspace itself", from)
	}
	tcfg, err := config.LoadFor(target)
	if err != nil {
		return err
	}
	if !config.JiraFamily(tcfg.OriginType()) {
		return fmt.Errorf("workspace %q is not a Jira workspace (origin: %s) — --to jira writes through the Jira credential of the workspace it runs in", target, tcfg.OriginType())
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

	var client *jira.Client
	if !dryRun {
		if client, err = origin.Client(tcfg); err != nil {
			return err
		}
	}

	// Attachment bytes come from the source origin, streamed into each
	// upload as the writer reaches it (the GDK-1618 shape; GDK-1275: a
	// warning here let a run report success with empty files, because the
	// count table counts rows and the bytes are what went missing).
	var fetch migrate.StreamFetch
	if !dryRun && !skipAttach && stats.Attachments > 0 {
		srcClient, cerr := origin.Client(srcCfg)
		if cerr != nil {
			return fmt.Errorf("cannot read attachment bytes from %q: %w\n"+
				"  %d attachments would migrate as empty metadata, and the count table would not say so\n"+
				"  to bring the bytes: make the source reachable (a frozen workspace: `gadak --workspace %s config set frozen false`)\n"+
				"  to migrate without them on purpose: --skip-attachments",
				from, cerr, stats.Attachments, from)
		}
		fetch = func(ctx context.Context, id string) (int, int64, io.ReadCloser, error) {
			res, err := srcClient.Stream(ctx, "GET", "/rest/api/3/attachment/content/"+url.PathEscape(id), nil)
			if err != nil {
				return 0, 0, nil, err
			}
			if res.StatusCode != 200 {
				_ = res.Body.Close()
				return res.StatusCode, 0, nil, nil
			}
			return res.StatusCode, res.ContentLength, res.Body, nil
		}
	}

	rep, err := migrate.ToJira(ctx, client, doc, stats, migrate.JiraOptions{
		ProjectKey: project, Limit: limit, DryRun: dryRun,
		SkipAttachments: skipAttach, Progress: os.Stderr, Fetch: fetch,
	})
	if err != nil {
		return err
	}
	if jsonOut {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"workspace": target, "from": from, "to": "jira", "stats": stats, "report": rep,
		})
	}
	printMigrateToReport(os.Stdout, from, target,
		fmt.Sprintf("Jira project %s", rep.Project), rep.DryRun,
		stats.Projects, rep.Mapping, rep.Counts, rep.NotMigrated, rep.Warnings)
	return nil
}

// printMigrateToReport is the human report both write destinations share:
// where it went, the mapping, the source-vs-migrated count table, and what
// stayed behind. A row whose migrated count differs from the source is
// marked, because that is the line a reader has to act on.
func printMigrateToReport(w *os.File, from, target, dest string, dryRun bool,
	projects, mapping []string, counts []migrate.VerifyRow, notMigrated, warnings []string) {
	verb := "migrated"
	if dryRun {
		verb = "dry-run:"
	}
	fmt.Fprintf(w, "%s %s → %s (workspace %q)\n", verb, from, dest, target)
	fmt.Fprintf(w, "projects: %s\n\nmapping:\n", strings.Join(projects, ", "))
	for _, m := range mapping {
		fmt.Fprintf(w, "  %s\n", m)
	}
	fmt.Fprintf(w, "\n%-12s %8s %8s %16s\n", "metric", "source", "migrated", "already there")
	for _, r := range counts {
		mark := ""
		if !dryRun && r.Source != r.Migrated {
			mark = "  MISMATCH"
		}
		fmt.Fprintf(w, "%-12s %8d %8d %16d%s\n", r.Metric, r.Source, r.Migrated, r.Skipped, mark)
	}
	fmt.Fprintln(w, "\nnot migrated:")
	for _, n := range notMigrated {
		fmt.Fprintf(w, "  %s\n", n)
	}
	for _, wn := range warnings {
		fmt.Fprintf(w, "warning: %s\n", wn)
	}
	if !dryRun {
		fmt.Fprintf(w, "\nnext: gadak --workspace %s sync\n", target)
	}
}
