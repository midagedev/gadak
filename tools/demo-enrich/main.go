// Command demo-enrich seeds the fixture-only rows examples/demo-source.db
// carries that never existed on the live site the mirror was cut from
// (GDK-1755, GDK-114). demo-fixture.sh's contract is "edit the source, run
// make demo-fixture" — this tool is the reviewable, rerunnable form of that
// edit: fixed content, keyed upserts, and a verify pass that fails loudly.
// Everything it writes is deterministic, so `make demo-fixture-check`'s
// byte-identity gate holds across reruns.
//
//	go run ./tools/demo-enrich && make demo-fixture
//
// Three relations, each named for the surface that was starved:
//
//   - dev_links: three pullrequest rows on NMB-5 covering the whole
//     jira.DevPRStatus vocabulary (open|merged|declined). The committed
//     fixture had 0 rows — e2e/serve.sh injects one 'open' row into its
//     serve-time copy (GDK-590), so the merged/declined chip classes in
//     PrList.svelte were exercised by no test and shown by no demo.
//   - Cloners: NMB-3 cloned from NMB-2 — an outward clone-type link plus the
//     issues_raw.cloned_from it derives to (same predicate as schemaV40 /
//     store's clonedFrom). The fixture's links held only
//     Blocks/Duplicate/Relates, so cloned_from was empty on every row.
//   - a wiki-URL comment on NMA-100 whose /wiki/spaces/…/pages/N link is
//     extracted into item_refs by store.ExtractPageRefsFromIssue — the same
//     exported grammar sync uses, so the fixture row cannot drift from what
//     a live mirror would write. item_refs held only issue|text rows.
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	_ "modernc.org/sqlite"

	"github.com/midagedev/gadak/internal/store"
)

// prLink is one development-panel pull request on NMB-5 ("dark mode contrast
// throws 500 when the workspace has no members" — In Progress, so open,
// merged and declined PRs all read naturally on one detail panel).
type prLink struct {
	url    string
	title  string
	status string // stored form: jira.DevPRStatus.Stored()
	at     string
	author string
	branch string
}

var prLinks = []prLink{
	{
		url:    "https://github.com/nimbus-dev/board/pull/479",
		title:  "Hotfix: fall back to the default palette",
		status: "declined",
		at:     "2026-08-12T16:40:00.000Z",
		author: "Dana Whitfield",
		branch: "hotfix/dark-mode-fallback",
	},
	{
		url:    "https://github.com/nimbus-dev/board/pull/482",
		title:  "Fix contrast token lookup for empty workspaces",
		status: "merged",
		at:     "2026-08-18T09:14:00.000Z",
		author: "Marco Reyes",
		branch: "fix/contrast-empty-workspace",
	},
	{
		url:    "https://github.com/nimbus-dev/board/pull/517",
		title:  "Add regression coverage for dark-mode contrast",
		status: "open",
		at:     "2026-08-21T13:02:00.000Z",
		author: "Alex Kim",
		branch: "test/dark-mode-contrast",
	},
}

// The three fixed targets. Item ids, not keys, are the storage keys; keys are
// asserted at run time so a source reshuffle fails the seed instead of
// quietly writing onto the wrong issue.
const (
	prIssueKey    = "NMB-5"
	cloneIssueKey = "NMB-3" // the clone; cloned_from points at the original
	cloneFromKey  = "NMB-2"
	refIssueKey   = "NMA-100"

	// Fixed ids beyond the source's current jira:* ranges so the seed is
	// stable no matter what else the corpus grows.
	refCommentID    = "jira:10961"
	refCommentExtID = "10961"
	// Component Map — Platform API (ENG). The numeric page id is what
	// reWikiPage captures and what item_refs.target_key stores.
	refPageID  = "131106"
	refPageURL = "https://nimbus.example.com/wiki/spaces/ENG/pages/" + refPageID
	refComment = "Root-caused against the component map — the refresh path crosses two services " +
		"before the token cache answers: " + refPageURL
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("demo-enrich", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dbPath := fs.String("db", "examples/demo-source.db", "fixture source mirror to enrich (the committed content original)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	db, err := sql.Open("sqlite", "file:"+*dbPath+"?mode=rw")
	if err != nil {
		fmt.Fprintf(os.Stderr, "open %s: %v\n", *dbPath, err)
		return 1
	}
	defer db.Close()

	for _, step := range []struct {
		name string
		fn   func(*sql.Tx) error
	}{
		{"dev_links", seedDevLinks},
		{"clone relation", seedCloneRelation},
		{"page ref comment", seedPageRefComment},
	} {
		if err := inTx(db, step.fn); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", step.name, err)
			return 1
		}
	}

	if err := verify(db); err != nil {
		fmt.Fprintf(os.Stderr, "verify: %v\n", err)
		return 1
	}
	return 0
}

func inTx(db *sql.DB, fn func(*sql.Tx) error) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// itemID resolves a key the way every mirror read does (issues view), and
// fails instead of seeding the wrong row when the key is missing.
func itemID(tx *sql.Tx, key string) (string, error) {
	var id string
	if err := tx.QueryRow(`SELECT item_id FROM issues WHERE key = ?`, key).Scan(&id); err != nil {
		return "", fmt.Errorf("resolve %s: %w", key, err)
	}
	return id, nil
}

func seedDevLinks(tx *sql.Tx) error {
	id, err := itemID(tx, prIssueKey)
	if err != nil {
		return err
	}
	for _, p := range prLinks {
		if _, err := tx.Exec(`
			INSERT INTO dev_links (item_id, kind, external_id, url, title, status,
			                       updated_at, author, actor, actor_name, branch, environment)
			VALUES (?, 'pullrequest', '', ?, ?, ?, ?, ?, 'demo-priya', 'Priya Sharma', ?, '')
			ON CONFLICT (item_id, url) DO UPDATE SET
			  kind='pullrequest', external_id='', title=excluded.title, status=excluded.status,
			  updated_at=excluded.updated_at, author=excluded.author,
			  actor='demo-priya', actor_name='Priya Sharma', branch=excluded.branch, environment=''`,
			id, p.url, p.title, p.status, p.at, p.author, p.branch,
		); err != nil {
			return err
		}
	}
	fmt.Printf("dev_links: %d pullrequest rows on %s (open/merged/declined)\n", len(prLinks), prIssueKey)
	return nil
}

func seedCloneRelation(tx *sql.Tx) error {
	id, err := itemID(tx, cloneIssueKey)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO links (item_id, type, direction, target_key)
		VALUES (?, 'Cloners', 'outward', ?)
		ON CONFLICT (item_id, type, direction, target_key) DO NOTHING`,
		id, cloneFromKey,
	); err != nil {
		return err
	}
	// cloned_from is the outward clone link, derived: same predicate as
	// schemaV40's recompute and store.clonedFrom (derive.go). 'Cloners' is
	// Jira's own link-type name for the relation.
	if _, err := tx.Exec(`UPDATE issues_raw SET cloned_from = ? WHERE item_id = ?`, cloneFromKey, id); err != nil {
		return err
	}
	fmt.Printf("clone: %s cloned_from %s (Cloners outward)\n", cloneIssueKey, cloneFromKey)
	return nil
}

func seedPageRefComment(tx *sql.Tx) error {
	id, err := itemID(tx, refIssueKey)
	if err != nil {
		return err
	}

	adf, err := commentADF(refComment)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO comments (id, item_id, external_id, author, author_id,
		                      body_adf, body_text, created_at, updated_at)
		VALUES (?, ?, ?, 'Dana Whitfield', 'demo-dana', ?, ?, '2026-07-15T10:20:30.000Z', '')
		ON CONFLICT (item_id, id) DO UPDATE SET
		  item_id=excluded.item_id, external_id=excluded.external_id,
		  author='Dana Whitfield', author_id='demo-dana',
		  body_adf=excluded.body_adf, body_text=excluded.body_text,
		  created_at=excluded.created_at, updated_at=''`,
		refCommentID, id, refCommentExtID, adf, refComment,
	); err != nil {
		return err
	}
	// The column the detail header and api_usage sum read counts the row.
	if _, err := tx.Exec(
		`UPDATE issues_raw SET comment_count = (SELECT count(*) FROM comments WHERE item_id = ?) WHERE item_id = ?`,
		id, id,
	); err != nil {
		return err
	}

	// Recompute this issue's page refs through the exported grammar sync
	// itself uses — issue|text refs from bare keys are left untouched.
	var bodyADF, bodyText string
	if err := tx.QueryRow(
		`SELECT r.description_adf, i.body_text FROM issues_raw r JOIN items i ON i.id = r.item_id WHERE r.item_id = ?`,
		id,
	).Scan(&bodyADF, &bodyText); err != nil {
		return err
	}
	crows, err := tx.Query(`SELECT body_text FROM comments WHERE item_id = ? ORDER BY id`, id)
	if err != nil {
		return err
	}
	var bodies []string
	for crows.Next() {
		var b string
		if err := crows.Scan(&b); err != nil {
			crows.Close()
			return err
		}
		bodies = append(bodies, b)
	}
	crows.Close()
	if err := crows.Err(); err != nil {
		return err
	}

	refs := store.ExtractPageRefsFromIssue(bodyADF, bodyText, bodies)
	if _, err := tx.Exec(`DELETE FROM item_refs WHERE item_id = ? AND target_kind = 'page'`, id); err != nil {
		return err
	}
	for _, r := range refs {
		if _, err := tx.Exec(
			`INSERT OR REPLACE INTO item_refs (item_id, target_kind, target_key, via) VALUES (?,?,?,?)`,
			id, r.TargetKind, r.TargetKey, r.Via,
		); err != nil {
			return err
		}
	}
	fmt.Printf("page ref: %s comment → page %s (%d item_refs page rows)\n", refIssueKey, refPageID, len(refs))
	return nil
}

// verify re-reads what was seeded — the same probes the committed-fixture
// gate runs (internal/store TestCommittedDemoDBCarriesDevPanelCloneAndPageRefs)
// — so a seed that silently wrote nothing fails here, at the source, before
// the snapshot can launder it into a green regen.
func verify(db *sql.DB) error {
	for _, status := range []string{"open", "merged", "declined"} {
		var n int
		if err := db.QueryRow(
			`SELECT count(*) FROM dev_links WHERE kind='pullrequest' AND status=?`, status,
		).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("no %q dev_links row survived the seed", status)
		}
	}

	// schemaV40's predicate, verbatim: cloned_from must be re-derivable.
	var bad int
	if err := db.QueryRow(`
		SELECT count(*) FROM issues_raw r
		 WHERE r.cloned_from != ''
		   AND COALESCE((SELECT l.target_key FROM links l
		                 WHERE l.item_id = r.item_id AND l.direction = 'outward'
		                   AND lower(l.type) LIKE '%clone%'
		                 ORDER BY l.target_key LIMIT 1), '') != r.cloned_from`,
	).Scan(&bad); err != nil {
		return err
	}
	if bad != 0 {
		return fmt.Errorf("%d cloned_from rows disagree with the outward-Cloners predicate", bad)
	}

	var refs int
	if err := db.QueryRow(`
		SELECT count(*) FROM item_refs r
		 WHERE r.target_kind = 'page'
		   AND EXISTS (SELECT 1 FROM pages p WHERE p.item_id = 'confluence:' || r.target_key)`,
	).Scan(&refs); err != nil {
		return err
	}
	if refs == 0 {
		return fmt.Errorf("no resolvable page item_refs row survived the seed")
	}
	fmt.Printf("verify: 3 statuses, clone predicate clean, %d resolvable page ref(s)\n", refs)
	return nil
}

// commentADF renders the same compact doc→paragraph→text shape the rest of
// the fixture's comments carry (typed structs so field order is stable).
func commentADF(text string) (string, error) {
	type adfText struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	type adfPara struct {
		Type    string    `json:"type"`
		Content []adfText `json:"content"`
	}
	type adfDoc struct {
		Type    string    `json:"type"`
		Version int       `json:"version"`
		Content []adfPara `json:"content"`
	}
	b, err := json.Marshal(adfDoc{
		Type: "doc", Version: 1,
		Content: []adfPara{{Type: "paragraph", Content: []adfText{{Type: "text", Text: text}}}},
	})
	return string(b), err
}
