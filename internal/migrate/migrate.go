// Package migrate exports a mirror into an issuetap fixture document — the
// seed a fresh local-origin workspace loads one-shot (origin/issuetap.yaml,
// GDK-1264). Reads are mirror-only; the single origin round-trip is the
// attachment byte download, and the caller owns that client.
//
// The document shapes mirror issuetap's internal/fixtures.Doc YAML contract
// (that package is internal, so the structs are declared here). Three rules
// came out of the GDK-1262 spike and must not regress:
//
//  1. priorities emit in id order — issuetap assigns priority_rank by
//     catalog position, so encounter order flips the ranking.
//  2. links emit each issue's own mirror rows verbatim, both directions —
//     the fixture load is single-sided (runtime AddIssueLink is what
//     materializes both projections), so a pair exists on both ends only
//     if both ends declare it.
//  3. the status catalog includes every status id the changelog references,
//     not just the ones issues currently sit in.
package migrate

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

// Doc is the fixture document. Key names follow issuetap's
// internal/fixtures.Doc (its json and yaml tags agree); only the parts a
// mirror can fill are declared. Emitted as JSON — see cmdMigrate for why
// the seed is never yaml.Marshal'd.
type Doc struct {
	Users      []User      `json:"users,omitempty"`
	Projects   []Project   `json:"projects,omitempty"`
	Statuses   []Status    `json:"statuses,omitempty"`
	Priorities []Priority  `json:"priorities,omitempty"`
	IssueTypes []IssueType `json:"issueTypes,omitempty"`
	Issues     []Issue     `json:"issues,omitempty"`
	Spaces     []Space     `json:"spaces,omitempty"`
	Pages      []Page      `json:"pages,omitempty"`
}

type User struct {
	AccountID   string `json:"accountId,omitempty"`
	DisplayName string `json:"displayName"`
	Email       string `json:"email,omitempty"`
	AccountType string `json:"accountType,omitempty"`
}

type Project struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

type Status struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"` // new | indeterminate | done
}

type Priority struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type IssueType struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	HierarchyLevel int    `json:"hierarchyLevel,omitempty"`
	Subtask        bool   `json:"subtask,omitempty"`
}

type Issue struct {
	Key         string       `json:"key"`
	Summary     string       `json:"summary"`
	Description string       `json:"description,omitempty"`
	Project     string       `json:"project,omitempty"`
	Type        string       `json:"type,omitempty"`   // id
	Status      string       `json:"status,omitempty"` // id
	Priority    string       `json:"priority,omitempty"`
	Assignee    string       `json:"assignee,omitempty"`
	Reporter    string       `json:"reporter,omitempty"`
	Parent      string       `json:"parent,omitempty"`
	Labels      []string     `json:"labels,omitempty"`
	Components  []string     `json:"components,omitempty"`
	FixVersions []string     `json:"fixVersions,omitempty"`
	Duedate     string       `json:"duedate,omitempty"`
	Resolution  string       `json:"resolution,omitempty"`
	Created     string       `json:"created,omitempty"`
	Updated     string       `json:"updated,omitempty"`
	Comments    []Comment    `json:"comments,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
	Links       []Link       `json:"links,omitempty"`
	History     []History    `json:"history,omitempty"`
	// StatusCategory (new|inprogress|done) and PriorityRank are the
	// mirror's contract axes — what a target with its own catalogs (Linear)
	// maps from, since ids mean nothing there. Never emitted: the fixture
	// path carries the ids and rebuilds both from its catalogs.
	StatusCategory string `json:"-"`
	PriorityRank   int    `json:"-"`
	// AssigneeEmail is the row's own email column — the fallback when the
	// users catalog has no row for the account (scrubbed fixtures).
	AssigneeEmail string `json:"-"`
}

type Comment struct {
	Author  string `json:"author,omitempty"`
	Body    string `json:"body"`
	Created string `json:"created,omitempty"`
}

type Attachment struct {
	Filename   string `json:"filename"`
	MimeType   string `json:"mimeType,omitempty"`
	Text       string `json:"text,omitempty"`
	DataBase64 string `json:"dataBase64,omitempty"`
	Author     string `json:"author,omitempty"`
	Created    string `json:"created,omitempty"`
	// ContentID is the origin content id for the byte download
	// (external_id when set, else the store row id — the same rule as
	// store.AttachmentOrigin). Never emitted.
	ContentID string `json:"-"`
	// Size is the mirror's byte count, used to skip oversized files
	// before spending the download. Never emitted.
	Size int64 `json:"-"`
	// SourceURL is the mirror's stored origin content URL (non-Jira
	// sources). Non-empty means the bytes do not live behind Jira's
	// /attachment/content/{id} and this pass skips them. Never emitted.
	SourceURL string `json:"-"`
}

type Link struct {
	Type    string `json:"type"`
	Inward  string `json:"inward,omitempty"`
	Outward string `json:"outward,omitempty"`
}

type History struct {
	At     string        `json:"at"`
	Author string        `json:"author,omitempty"`
	Items  []HistoryItem `json:"items"`
}

type HistoryItem struct {
	Field      string `json:"field"`
	From       string `json:"from,omitempty"`
	FromString string `json:"fromString,omitempty"`
	To         string `json:"to,omitempty"`
	ToString   string `json:"toString,omitempty"`
}

type Space struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

type Page struct {
	ID       string        `json:"id,omitempty"`
	Title    string        `json:"title"`
	Space    string        `json:"space"`
	Version  int           `json:"version,omitempty"`
	When     string        `json:"when,omitempty"`
	Author   string        `json:"author,omitempty"`
	Body     string        `json:"body,omitempty"`
	Labels   []string      `json:"labels,omitempty"`
	Parent   string        `json:"parent,omitempty"`
	Comments []PageComment `json:"comments,omitempty"`
}

type PageComment struct {
	Author string `json:"author,omitempty"`
	Body   string `json:"body"`
	When   string `json:"when,omitempty"`
}

// Options selects what leaves the mirror. Empty means everything mirrored.
type Options struct {
	Projects []string // issue project keys
	Spaces   []string // wiki space keys
}

// Stats is the honest half of the export: what was counted, what was
// flattened, what was dropped and why. The verification report is built
// from it — silent loss and reported loss are different products.
type Stats struct {
	Projects []string
	Spaces   []string

	Issues       int
	Comments     int
	Attachments  int
	Links        int
	History      int
	Pages        int
	PageComments int
	Users        int

	// Derived columns the target must reproduce from the migrated
	// changelog (they are never stored in the fixture).
	ReopenSum int
	EpicKeys  int

	// Formatting nodes flattened to plain text by the fixture path
	// (descriptions, comments and page bodies all load as adf.Doc(text)).
	LossCodeBlock int
	LossMedia     int
	LossTable     int

	// Dropped because the other end is outside the migrated set.
	DroppedLinks       int
	DroppedParents     []string
	DroppedPageParents int

	// Not migrated (the fixture has no slot, or out of scope).
	DevLinks     int
	CustomIssues int
	SprintIssues int

	// Attachment byte pass.
	AttachInlined  int
	AttachMissing  int // origin answered 404 — metadata kept
	AttachTooLarge int
	AttachSkipURL  int // stored origin URL (non-Jira source) — out of scope
	AttachErrors   []string

	MissingUsers    []string // referenced account ids absent from the users catalog
	UnnamedStatuses []string // history-only status ids with no display name
}

// MaxAttachmentBytes caps one inlined file. The fixture is a YAML document
// read into memory; a file past this stays metadata-only and is reported.
const MaxAttachmentBytes = 16 << 20

// Build reads the mirror and assembles the fixture document. db must be a
// mirror connection (read-only is fine); nothing is written.
func Build(ctx context.Context, db *sql.DB, opt Options) (*Doc, *Stats, error) {
	doc := &Doc{}
	st := &Stats{}

	projects, err := selectProjects(ctx, db, opt.Projects)
	if err != nil {
		return nil, nil, err
	}
	if len(projects) == 0 {
		return nil, nil, fmt.Errorf("migrate: the source mirror has no issues to export — sync it first")
	}
	st.Projects = projects
	for _, p := range projects {
		doc.Projects = append(doc.Projects, Project{Key: p, Name: p})
	}

	if err := buildIssues(ctx, db, doc, st); err != nil {
		return nil, nil, err
	}
	if err := buildCatalogs(ctx, db, doc, st); err != nil {
		return nil, nil, err
	}
	if err := buildPages(ctx, db, doc, st, opt.Spaces); err != nil {
		return nil, nil, err
	}
	if err := buildUsers(ctx, db, doc, st); err != nil {
		return nil, nil, err
	}
	return doc, st, nil
}

func selectProjects(ctx context.Context, db *sql.DB, want []string) ([]string, error) {
	if len(want) > 0 {
		out := append([]string{}, want...)
		sort.Strings(out)
		return out, nil
	}
	rows, err := db.QueryContext(ctx,
		`SELECT DISTINCT project_key FROM issues_full WHERE project_key != '' ORDER BY project_key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// issueRow is the per-issue working set Build carries between passes.
type issueRow struct {
	itemID string
	custom string
	sprint bool
}

func buildIssues(ctx context.Context, db *sql.DB, doc *Doc, st *Stats) error {
	marks, args := inClause(st.Projects)
	rows, err := db.QueryContext(ctx, `
		SELECT key, item_id, summary, COALESCE(description_text,''), COALESCE(description_adf,''),
		       project_key, issue_type_id, status_id, COALESCE(priority_id,''),
		       COALESCE(assignee_id,''), COALESCE(reporter_id,''), COALESCE(parent_key,''),
		       COALESCE(labels,'[]'), COALESCE(components,'[]'), COALESCE(fix_versions,'[]'),
		       COALESCE(duedate,''), COALESCE(resolution,''),
		       COALESCE(created_at,''), COALESCE(updated_at,''),
		       COALESCE(reopen_count,0), COALESCE(epic_key,''),
		       COALESCE(custom,''), COALESCE(sprint_id,0),
		       COALESCE(status_category,''), COALESCE(priority_rank,0), COALESCE(assignee_email,'')
		FROM issues_full WHERE project_key IN (`+marks+`) ORDER BY key`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	var items []issueRow
	keyset := map[string]bool{}
	for rows.Next() {
		var is Issue
		var itemID, descADF, labels, comps, fixes, custom string
		var reopen int
		var epic string
		var sprintID int64
		if err := rows.Scan(&is.Key, &itemID, &is.Summary, &is.Description, &descADF,
			&is.Project, &is.Type, &is.Status, &is.Priority,
			&is.Assignee, &is.Reporter, &is.Parent,
			&labels, &comps, &fixes, &is.Duedate, &is.Resolution,
			&is.Created, &is.Updated, &reopen, &epic, &custom, &sprintID,
			&is.StatusCategory, &is.PriorityRank, &is.AssigneeEmail); err != nil {
			return err
		}
		is.Labels = jsonStrings(labels)
		is.Components = jsonStrings(comps)
		is.FixVersions = jsonStrings(fixes)
		countLoss(descADF, st)
		st.ReopenSum += reopen
		if epic != "" {
			st.EpicKeys++
		}
		if custom != "" && custom != "{}" && custom != "null" {
			st.CustomIssues++
		}
		doc.Issues = append(doc.Issues, is)
		items = append(items, issueRow{itemID: itemID, custom: custom, sprint: sprintID != 0})
		if sprintID != 0 {
			st.SprintIssues++
		}
		keyset[is.Key] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	st.Issues = len(doc.Issues)

	for i := range doc.Issues {
		is := &doc.Issues[i]
		itemID := items[i].itemID
		if is.Parent != "" && !keyset[is.Parent] {
			st.DroppedParents = append(st.DroppedParents, is.Key+"→"+is.Parent)
			is.Parent = ""
		}
		if err := fillComments(ctx, db, itemID, is, st); err != nil {
			return err
		}
		if err := fillAttachmentMeta(ctx, db, itemID, is, st); err != nil {
			return err
		}
		if err := fillLinks(ctx, db, itemID, is, keyset, st); err != nil {
			return err
		}
		if err := fillHistory(ctx, db, itemID, is, st); err != nil {
			return err
		}
	}

	var devs int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM dev_links d JOIN issues_full i ON i.item_id = d.item_id
		WHERE i.project_key IN (`+marks+`)`, args...).Scan(&devs); err == nil {
		st.DevLinks = devs
	}
	return nil
}

func fillComments(ctx context.Context, db *sql.DB, itemID string, is *Issue, st *Stats) error {
	rows, err := db.QueryContext(ctx, `
		SELECT COALESCE(NULLIF(author_id,''), COALESCE(author,'')), COALESCE(body_text,''),
		       COALESCE(body_adf,''), COALESCE(created_at,'')
		FROM comments WHERE item_id = ? ORDER BY created_at, id`, itemID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var c Comment
		var adf string
		if err := rows.Scan(&c.Author, &c.Body, &adf, &c.Created); err != nil {
			return err
		}
		countLoss(adf, st)
		is.Comments = append(is.Comments, c)
	}
	st.Comments += len(is.Comments)
	return rows.Err()
}

func fillAttachmentMeta(ctx context.Context, db *sql.DB, itemID string, is *Issue, st *Stats) error {
	rows, err := db.QueryContext(ctx, `
		SELECT COALESCE(NULLIF(external_id,''), id), COALESCE(filename,''), COALESCE(mime_type,''),
		       COALESCE(size,0), COALESCE(NULLIF(author_id,''), COALESCE(author,'')),
		       COALESCE(created_at,''), COALESCE(url,'')
		FROM attachments WHERE item_id = ? ORDER BY created_at, id`, itemID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var a Attachment
		if err := rows.Scan(&a.ContentID, &a.Filename, &a.MimeType, &a.Size,
			&a.Author, &a.Created, &a.SourceURL); err != nil {
			return err
		}
		is.Attachments = append(is.Attachments, a)
	}
	st.Attachments += len(is.Attachments)
	return rows.Err()
}

func fillLinks(ctx context.Context, db *sql.DB, itemID string, is *Issue, keyset map[string]bool, st *Stats) error {
	rows, err := db.QueryContext(ctx, `
		SELECT type, direction, target_key FROM links WHERE item_id = ? ORDER BY type, direction, target_key`, itemID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var typ, dir, target string
		if err := rows.Scan(&typ, &dir, &target); err != nil {
			return err
		}
		if !keyset[target] {
			st.DroppedLinks++
			continue
		}
		l := Link{Type: typ}
		if dir == "inward" {
			l.Inward = target
		} else {
			l.Outward = target
		}
		is.Links = append(is.Links, l)
	}
	st.Links += len(is.Links)
	return rows.Err()
}

func fillHistory(ctx context.Context, db *sql.DB, itemID string, is *Issue, st *Stats) error {
	rows, err := db.QueryContext(ctx, `
		SELECT COALESCE(at,''), COALESCE(NULLIF(author_id,''), COALESCE(author,'')),
		       COALESCE(field,''), COALESCE(from_id,''), COALESCE(from_value,''),
		       COALESCE(to_id,''), COALESCE(to_value,'')
		FROM changelog WHERE item_id = ? ORDER BY at, id`, itemID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var h History
		var it HistoryItem
		if err := rows.Scan(&h.At, &h.Author, &it.Field,
			&it.From, &it.FromString, &it.To, &it.ToString); err != nil {
			return err
		}
		h.Items = []HistoryItem{it}
		is.History = append(is.History, h)
	}
	st.History += len(is.History)
	return rows.Err()
}

// buildCatalogs derives statuses / priorities / issue types from the rows
// already selected into doc.Issues, so the catalogs and the issues cannot
// disagree.
func buildCatalogs(ctx context.Context, db *sql.DB, doc *Doc, st *Stats) error {
	statusIDs := map[string]bool{}
	prioIDs := map[string]bool{}
	typeIDs := map[string]bool{}
	for i := range doc.Issues {
		is := &doc.Issues[i]
		statusIDs[is.Status] = true
		if is.Priority != "" {
			prioIDs[is.Priority] = true
		}
		typeIDs[is.Type] = true
		for _, h := range is.History {
			for _, it := range h.Items {
				if it.Field != "status" {
					continue
				}
				if it.From != "" {
					statusIDs[it.From] = true
				}
				if it.To != "" {
					statusIDs[it.To] = true
				}
			}
		}
	}

	// Display names for status ids: current issues first, then changelog
	// values for history-only ids, then the id itself (reported).
	name := map[string]string{}
	if err := scanPairs(ctx, db,
		`SELECT DISTINCT status_id, status FROM issues_full WHERE status_id != ''`, name); err != nil {
		return err
	}
	histName := map[string]string{}
	if err := scanPairs(ctx, db, `
		SELECT from_id, from_value FROM changelog WHERE field='status' AND from_id != '' AND from_value != ''
		UNION SELECT to_id, to_value FROM changelog WHERE field='status' AND to_id != '' AND to_value != ''`,
		histName); err != nil {
		return err
	}
	// Category: the issues' own status_category first — those rows are the
	// ones leaving, and the catalogs must agree with them — then the origin
	// status catalog for history-only ids. status_catalog alone is not
	// enough: it is written by sync passes, and a mirror that never ran one
	// (a snapshot, a fixture, examples/demo.db) has an empty table. Read
	// that way, every status fell to "new" and the migrated board opened
	// with Done and In Progress as open issues and no in-progress state for
	// `gadak claim` to land on (GDK-1361).
	cat := map[string]string{}
	if err := scanPairs(ctx, db, `SELECT status_id, category FROM status_catalog`, cat); err != nil {
		return err
	}
	for i := range doc.Issues {
		if is := &doc.Issues[i]; is.Status != "" && is.StatusCategory != "" {
			cat[is.Status] = is.StatusCategory
		}
	}

	// Mirror categories are new|inprogress|done; fixtures speak Cloud's
	// new|indeterminate|done.
	toFixtureCat := map[string]string{"new": "new", "inprogress": "indeterminate", "done": "done"}
	for _, id := range sortedKeys(statusIDs) {
		n := name[id]
		if n == "" {
			n = histName[id]
		}
		if n == "" {
			n = id
			st.UnnamedStatuses = append(st.UnnamedStatuses, id)
		}
		c := toFixtureCat[cat[id]]
		if c == "" {
			c = "new"
		}
		doc.Statuses = append(doc.Statuses, Status{ID: id, Name: n, Category: c})
	}

	// Rule 1: catalog order is priority_rank — emit in numeric id order,
	// never encounter order.
	prios := map[string]string{}
	if err := scanPairs(ctx, db,
		`SELECT DISTINCT priority_id, priority FROM issues_full WHERE priority_id != ''`, prios); err != nil {
		return err
	}
	ids := sortedKeys(prioIDs)
	sort.Slice(ids, func(i, j int) bool {
		a, b := ids[i], ids[j]
		if len(a) != len(b) {
			return len(a) < len(b) // numeric ids: shorter is smaller
		}
		return a < b
	})
	for _, id := range ids {
		n := prios[id]
		if n == "" {
			n = id
		}
		doc.Priorities = append(doc.Priorities, Priority{ID: id, Name: n})
	}

	types := map[string][2]string{}
	rows, err := db.QueryContext(ctx,
		`SELECT DISTINCT issue_type_id, issue_type, CAST(COALESCE(hierarchy_level,0) AS TEXT) FROM issues_full WHERE issue_type_id != ''`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, n, lvl string
		if err := rows.Scan(&id, &n, &lvl); err != nil {
			return err
		}
		types[id] = [2]string{n, lvl}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range sortedKeys(typeIDs) {
		t := IssueType{ID: id, Name: types[id][0]}
		if t.Name == "" {
			t.Name = id
		}
		fmt.Sscanf(types[id][1], "%d", &t.HierarchyLevel)
		t.Subtask = t.HierarchyLevel == -1
		doc.IssueTypes = append(doc.IssueTypes, t)
	}
	return nil
}

func buildPages(ctx context.Context, db *sql.DB, doc *Doc, st *Stats, want []string) error {
	spaceKeys := map[string]bool{}
	if len(want) > 0 {
		for _, s := range want {
			spaceKeys[s] = true
		}
	} else {
		rows, err := db.QueryContext(ctx, `SELECT DISTINCT space_key FROM pages`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var k string
			if err := rows.Scan(&k); err != nil {
				return err
			}
			spaceKeys[k] = true
		}
		if err := rows.Err(); err != nil {
			return err
		}
	}
	if len(spaceKeys) == 0 {
		return nil
	}
	st.Spaces = sortedKeys(spaceKeys)

	names := map[string]string{}
	if err := scanPairs(ctx, db, `SELECT key, name FROM spaces`, names); err != nil {
		return err
	}
	for _, k := range st.Spaces {
		n := names[k]
		if n == "" {
			n = k
		}
		doc.Spaces = append(doc.Spaces, Space{Key: k, Name: n})
	}

	marks, args := inClause(st.Spaces)
	rows, err := db.QueryContext(ctx, `
		SELECT it.external_id, it.title, COALESCE(it.body_text,''),
		       COALESCE(NULLIF(it.author_id,''), COALESCE(it.author,'')), COALESCE(it.updated_at,''),
		       p.space_key, p.parent_id, p.version, COALESCE(p.labels,'[]'), COALESCE(p.body_adf,''),
		       it.id
		FROM pages p JOIN items it ON it.id = p.item_id
		WHERE p.space_key IN (`+marks+`) AND p.status = 'current'
		ORDER BY it.external_id`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	inSet := map[string]bool{}
	var itemIDs []string
	for rows.Next() {
		var pg Page
		var labels, adf, itemID string
		if err := rows.Scan(&pg.ID, &pg.Title, &pg.Body, &pg.Author, &pg.When,
			&pg.Space, &pg.Parent, &pg.Version, &labels, &adf, &itemID); err != nil {
			return err
		}
		pg.Labels = jsonStrings(labels)
		countLoss(adf, st)
		doc.Pages = append(doc.Pages, pg)
		inSet[pg.ID] = true
		itemIDs = append(itemIDs, itemID)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	st.Pages = len(doc.Pages)

	for i := range doc.Pages {
		pg := &doc.Pages[i]
		if pg.Parent != "" && !inSet[pg.Parent] {
			st.DroppedPageParents++
			pg.Parent = ""
		}
		crows, err := db.QueryContext(ctx, `
			SELECT COALESCE(NULLIF(author_id,''), COALESCE(author,'')), COALESCE(body_text,''),
			       COALESCE(body_adf,''), COALESCE(created_at,'')
			FROM comments WHERE item_id = ? ORDER BY created_at, id`, itemIDs[i])
		if err != nil {
			return err
		}
		for crows.Next() {
			var c PageComment
			var adf string
			if err := crows.Scan(&c.Author, &c.Body, &adf, &c.When); err != nil {
				crows.Close()
				return err
			}
			countLoss(adf, st)
			pg.Comments = append(pg.Comments, c)
		}
		err = crows.Err()
		crows.Close()
		if err != nil {
			return err
		}
		st.PageComments += len(pg.Comments)
	}
	return nil
}

// buildUsers collects every account id the document references and emits a
// catalog row for each — a ghost row (displayName = id) when the mirror's
// users table no longer knows the account, so attribution survives departed
// members (reported, not silently invented).
func buildUsers(ctx context.Context, db *sql.DB, doc *Doc, st *Stats) error {
	ids := map[string]bool{}
	add := func(s string) {
		if s != "" {
			ids[s] = true
		}
	}
	for i := range doc.Issues {
		is := &doc.Issues[i]
		add(is.Assignee)
		add(is.Reporter)
		for _, c := range is.Comments {
			add(c.Author)
		}
		for _, a := range is.Attachments {
			add(a.Author)
		}
		for _, h := range is.History {
			add(h.Author)
		}
	}
	for i := range doc.Pages {
		add(doc.Pages[i].Author)
		for _, c := range doc.Pages[i].Comments {
			add(c.Author)
		}
	}

	for _, id := range sortedKeys(ids) {
		var u User
		u.AccountID = id
		var acctType string
		err := db.QueryRowContext(ctx, `
			SELECT COALESCE(name,''), COALESCE(email,''), COALESCE(account_type,'')
			FROM users WHERE account_id = ? LIMIT 1`, id).Scan(&u.DisplayName, &u.Email, &acctType)
		if err == sql.ErrNoRows {
			u.DisplayName = id
			st.MissingUsers = append(st.MissingUsers, id)
		} else if err != nil {
			return err
		}
		if acctType != "" && acctType != "atlassian" {
			u.AccountType = acctType
		}
		doc.Users = append(doc.Users, u)
	}
	st.Users = len(doc.Users)
	return nil
}

// Fetch downloads one attachment's bytes by content id. Implemented by the
// caller over the source origin client (Jira's and issuetap's
// /attachment/content/{id} are the same shape). A 404 returns status 404
// with err nil.
type Fetch func(ctx context.Context, contentID string) (status int, body []byte, err error)

// InlineAttachments downloads each attachment's bytes and inlines them into
// the document — printable text/* as Text, anything else as std base64. A
// missing file (404) or an oversized one keeps its metadata row and is
// counted, never fatal: a partial archive that says what is missing beats
// no archive.
func InlineAttachments(ctx context.Context, doc *Doc, fetch Fetch, st *Stats) {
	for i := range doc.Issues {
		for j := range doc.Issues[i].Attachments {
			a := &doc.Issues[i].Attachments[j]
			switch {
			case a.SourceURL != "":
				st.AttachSkipURL++
				continue
			case a.Size > MaxAttachmentBytes:
				st.AttachTooLarge++
				continue
			}
			status, body, err := fetch(ctx, a.ContentID)
			switch {
			case err != nil:
				st.AttachErrors = append(st.AttachErrors,
					fmt.Sprintf("%s (%s): %v", a.Filename, doc.Issues[i].Key, err))
			case status == 404:
				st.AttachMissing++
			case status != 200:
				st.AttachErrors = append(st.AttachErrors,
					fmt.Sprintf("%s (%s): origin status %d", a.Filename, doc.Issues[i].Key, status))
			case len(body) > MaxAttachmentBytes:
				st.AttachTooLarge++
			default:
				if isPrintableText(a.MimeType, body) {
					a.Text = string(body)
				} else {
					a.DataBase64 = base64.StdEncoding.EncodeToString(body)
				}
				st.AttachInlined++
			}
		}
	}
}

func isPrintableText(mime string, b []byte) bool {
	if !strings.HasPrefix(mime, "text/") || !utf8.Valid(b) {
		return false
	}
	for _, c := range b {
		if c < 0x20 && c != '\n' && c != '\r' && c != '\t' {
			return false
		}
	}
	return true
}

// countLoss walks one ADF document and counts the node kinds the fixture's
// plain-text bodies cannot carry.
func countLoss(adfJSON string, st *Stats) {
	if adfJSON == "" {
		return
	}
	var node any
	if err := json.Unmarshal([]byte(adfJSON), &node); err != nil {
		return
	}
	var walk func(any)
	walk = func(n any) {
		switch v := n.(type) {
		case map[string]any:
			switch v["type"] {
			case "codeBlock":
				st.LossCodeBlock++
			// Leaf media nodes only — mediaSingle/mediaGroup wrap a
			// media child and counting both doubles the figure.
			case "media", "mediaInline":
				st.LossMedia++
			case "table":
				st.LossTable++
			}
			for _, c := range v {
				walk(c)
			}
		case []any:
			for _, c := range v {
				walk(c)
			}
		}
	}
	walk(node)
}

func inClause(vals []string) (marks string, args []any) {
	qs := make([]string, len(vals))
	for i, v := range vals {
		qs[i] = "?"
		args = append(args, v)
	}
	return strings.Join(qs, ","), args
}

func jsonStrings(s string) []string {
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil || len(out) == 0 {
		return nil
	}
	return out
}

func scanPairs(ctx context.Context, db *sql.DB, q string, into map[string]string) error {
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return err
		}
		if _, seen := into[k]; !seen {
			into[k] = v
		}
	}
	return rows.Err()
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
