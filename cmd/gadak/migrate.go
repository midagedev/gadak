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
	"github.com/midagedev/gadak/internal/migrate"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/originbind"
	"github.com/midagedev/gadak/internal/store"
)

const migrateUsage = "usage: gadak --workspace <new name> migrate --from <workspace> [--projects A,B] [--spaces X,Y] [--skip-attachments] [--json]"

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
			fmt.Fprintf(os.Stderr, "warning: source origin unavailable (%v) — attachments stay metadata-only\n", cerr)
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
	if st.LossCodeBlock+st.LossMedia+st.LossTable > 0 {
		fmt.Fprintf(w, "formatting flattened to plain text: code blocks %d, media %d, tables %d (bodies migrate as text)\n",
			st.LossCodeBlock, st.LossMedia, st.LossTable)
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
