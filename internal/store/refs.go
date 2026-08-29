package store

import (
	"database/sql"
	"regexp"
	"sort"
	"strings"

	"github.com/midagedev/gadak/internal/fields"
)

// item_refs holds text-derived cross-references between mirrored items
// (page body → issue keys, issue body/comments → page IDs). Targets need not
// exist in the mirror; readers join items and only surface live rows.

// ItemRef is one outgoing reference from an item. Pure extraction returns these;
// the write path persists them into item_refs.
type ItemRef struct {
	TargetKind string // "issue" | "page"
	TargetKey  string // issue key or page id (numeric string)
	Via        string // "url" | "text"
}

// Package-level regexes: compiled once for sync and backfill throughput.
var (
	// /browse/ABC-123 in ADF / URL strings (via=url). Key body assembled from
	// fields.IssueKeyBare (single owner, GDK-1103).
	reBrowseIssue = regexp.MustCompile(`/browse/(` + fields.IssueKeyBare + `)`)
	// Bare issue key in plain text; must be filtered by known project keys
	// (via=text). Key body assembled from fields.IssueKeyBare (single owner,
	// GDK-1103); the capture and \b anchors are this scan's.
	reBareIssue = regexp.MustCompile(`\b(` + fields.IssueKeyBare + `)\b`)
	// Confluence pretty URL …/wiki/spaces/…/pages/123…
	reWikiPage = regexp.MustCompile(`/wiki/spaces/[^/\s]+/pages/(\d+)`)
	// Query-style pageId=123
	rePageIDParam = regexp.MustCompile(`pageId=(\d+)`)
)

// ExtractIssueRefsFromPage finds issue keys in a page's ADF (URL paths) and
// plain body_text (bare keys filtered by knownProjects). Same key from both
// sources yields one row with via=url. knownProjects may be empty (no text hits).
func ExtractIssueRefsFromPage(bodyADF, bodyText string, knownProjects map[string]bool) []ItemRef {
	urlKeys := reBrowseIssue.FindAllStringSubmatch(bodyADF, -1)
	urlSet := map[string]struct{}{}
	for _, m := range urlKeys {
		if len(m) >= 2 && m[1] != "" {
			urlSet[m[1]] = struct{}{}
		}
	}

	// via ranking: url wins over text when both match the same key.
	best := map[string]string{}
	for k := range urlSet {
		best[k] = "url"
	}
	if len(knownProjects) > 0 && bodyText != "" {
		for _, m := range reBareIssue.FindAllStringSubmatch(bodyText, -1) {
			if len(m) < 2 || m[1] == "" {
				continue
			}
			// The owner body matches the whole key; the project half is what
			// knownProjects filters on (its charset has no hyphen, so the
			// first one separates key halves).
			proj, _, _ := strings.Cut(m[1], "-")
			if !knownProjects[proj] {
				continue
			}
			if _, ok := best[m[1]]; !ok {
				best[m[1]] = "text"
			}
		}
	}

	return refsFromMap(best, "issue")
}

// ExtractPageRefsFromIssue finds Confluence page IDs in an issue description
// (raw ADF + flattened text) and its comment bodies. Scanning ADF is what
// picks up link marks and inlineCards that PlainText drops — the same raw
// scan ExtractIssueRefsFromPage already does in the other direction.
// Both URL shapes use via=url. Duplicates collapse to one.
func ExtractPageRefsFromIssue(bodyADF, bodyText string, commentBodies []string) []ItemRef {
	seen := map[string]struct{}{}
	scan := func(s string) {
		if s == "" {
			return
		}
		for _, m := range reWikiPage.FindAllStringSubmatch(s, -1) {
			if len(m) >= 2 && m[1] != "" {
				seen[m[1]] = struct{}{}
			}
		}
		for _, m := range rePageIDParam.FindAllStringSubmatch(s, -1) {
			if len(m) >= 2 && m[1] != "" {
				seen[m[1]] = struct{}{}
			}
		}
	}
	scan(bodyADF)
	scan(bodyText)
	for _, c := range commentBodies {
		scan(c)
	}
	best := make(map[string]string, len(seen))
	for k := range seen {
		best[k] = "url"
	}
	return refsFromMap(best, "page")
}

func refsFromMap(best map[string]string, kind string) []ItemRef {
	if len(best) == 0 {
		return nil
	}
	keys := make([]string, 0, len(best))
	for k := range best {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]ItemRef, 0, len(keys))
	for _, k := range keys {
		out = append(out, ItemRef{TargetKind: kind, TargetKey: k, Via: best[k]})
	}
	return out
}

// filterSelfRef drops a ref whose target_key equals the source item's key.
func filterSelfRef(refs []ItemRef, selfKey string) []ItemRef {
	if selfKey == "" || len(refs) == 0 {
		return refs
	}
	out := refs[:0:0]
	for _, r := range refs {
		if r.TargetKey == selfKey {
			continue
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func loadKnownProjectKeys(tx *sql.Tx) (map[string]bool, error) {
	rows, err := tx.Query(`
		SELECT DISTINCT project_key FROM issues
		WHERE project_key IS NOT NULL AND project_key != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out[k] = true
	}
	return out, rows.Err()
}

// replaceItemRefs deletes every outgoing ref for itemID and inserts refs.
// Called inside the same upsert transaction as the item write.
func replaceItemRefs(tx *sql.Tx, itemID string, refs []ItemRef) error {
	if _, err := tx.Exec(`DELETE FROM item_refs WHERE item_id = ?`, itemID); err != nil {
		return err
	}
	for _, r := range refs {
		if r.TargetKind == "" || r.TargetKey == "" || r.Via == "" {
			continue
		}
		if _, err := tx.Exec(`
			INSERT INTO item_refs (item_id, target_kind, target_key, via)
			VALUES (?,?,?,?)`,
			itemID, r.TargetKind, r.TargetKey, r.Via,
		); err != nil {
			return err
		}
	}
	return nil
}

// backfillItemRefs builds item_refs for every existing page and issue.
// Called from the v16 migration in the same transaction as CREATE TABLE.
func backfillItemRefs(tx *sql.Tx) error {
	known, err := loadKnownProjectKeys(tx)
	if err != nil {
		return err
	}

	// Pages: ADF + body_text → issue keys.
	type pageRow struct {
		id, key, adf, body string
	}
	var pages []pageRow
	prows, err := tx.Query(`
		SELECT it.id, COALESCE(it.key, ''), COALESCE(p.body_adf, ''), COALESCE(it.body_text, '')
		FROM pages p JOIN items it ON it.id = p.item_id`)
	if err != nil {
		return err
	}
	for prows.Next() {
		var r pageRow
		if err := prows.Scan(&r.id, &r.key, &r.adf, &r.body); err != nil {
			prows.Close()
			return err
		}
		pages = append(pages, r)
	}
	if err := prows.Err(); err != nil {
		prows.Close()
		return err
	}
	prows.Close()
	for _, r := range pages {
		refs := filterSelfRef(ExtractIssueRefsFromPage(r.adf, r.body, known), r.key)
		if err := replaceItemRefs(tx, r.id, refs); err != nil {
			return err
		}
	}

	// Issues: ADF + body_text + comments → page IDs.
	type issueRow struct {
		id, key, body, adf string
	}
	var issues []issueRow
	irows, err := tx.Query(`
		SELECT it.id, COALESCE(it.key, ''), COALESCE(it.body_text, ''), COALESCE(i.description_adf, '')
		FROM items it JOIN issues i ON i.item_id = it.id
		WHERE it.kind = 'issue'`)
	if err != nil {
		return err
	}
	for irows.Next() {
		var r issueRow
		if err := irows.Scan(&r.id, &r.key, &r.body, &r.adf); err != nil {
			irows.Close()
			return err
		}
		issues = append(issues, r)
	}
	if err := irows.Err(); err != nil {
		irows.Close()
		return err
	}
	irows.Close()

	// Load all comment bodies keyed by item_id in one pass.
	commentsByItem := map[string][]string{}
	crows, err := tx.Query(`
		SELECT c.item_id, COALESCE(c.body_text, ''), COALESCE(c.body_adf, '')
		FROM comments c
		JOIN items it ON it.id = c.item_id AND it.kind = 'issue'`)
	if err != nil {
		return err
	}
	for crows.Next() {
		var id, body, adf string
		if err := crows.Scan(&id, &body, &adf); err != nil {
			crows.Close()
			return err
		}
		if body != "" {
			commentsByItem[id] = append(commentsByItem[id], body)
		}
		if adf != "" {
			commentsByItem[id] = append(commentsByItem[id], adf)
		}
	}
	if err := crows.Err(); err != nil {
		crows.Close()
		return err
	}
	crows.Close()

	for _, r := range issues {
		refs := filterSelfRef(ExtractPageRefsFromIssue(r.adf, r.body, commentsByItem[r.id]), r.key)
		if err := replaceItemRefs(tx, r.id, refs); err != nil {
			return err
		}
	}
	return nil
}
