package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/midagedev/gadak/internal/secretscan"
	"github.com/midagedev/gadak/internal/store"
)

// personalExportVersion is the gadak_export value this build writes. The
// parse side still imports every version back to minPersonalExportVersion
// — a format bump that refuses the old files strands every backup a
// person already has, which is a deletion wearing a version number.
const personalExportVersion = 3

// minPersonalExportVersion is the oldest gadak_export this build still
// imports. 1 is the pre-history format: its files simply carry no visits,
// searches, recipes, or dashboards sections.
const minPersonalExportVersion = 1

// personalExport is the on-disk dump of personal tables (saved_views,
// watches, favorites, and local.recents). It is not a team-config file
// (that is gadak_team_config) and it never carries credentials.
//
// v2 (GDK-1769) grows the document to the rest of the personal data the
// product invariant names: the visit/search history local.db keeps and the
// recipes/dashboards a person authored. Import applies the sections that
// have faithful writers (recipes, dashboards); history rides in the file
// until a store-level import exists (see cmdImport's note).
//
// v3 (GDK-1439/1440) adds the two history summaries local.db now keeps:
// the person-session rows and the write ledger. They are derived from the
// visits (and, for the ledger, the writes) already in the file — they ride
// along for the same reason the raw events do, and import ignores them the
// same way.
type personalExport struct {
	Version     int                `json:"gadak_export"`
	ExportedAt  string             `json:"exported_at"`
	Views       []store.SavedView  `json:"views"`
	Watches     []string           `json:"watches"`
	Favorites   []string           `json:"favorites"`
	Recents     []store.Recent     `json:"recents"`
	Visits      []store.Visit      `json:"visits"`
	Searches    []store.Search     `json:"searches"`
	Recipes     []store.Recipe     `json:"recipes"`
	Dashboards  []store.Dashboard  `json:"dashboards"`
	Sessions    []store.Session    `json:"sessions"`
	AgentWrites []store.AgentWrite `json:"agent_writes"`
}

func cmdExport(args []string) error {
	fs := newFlagSet("export")
	outPath := fs.String("out", "", "write to this file instead of stdout")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return usageError("export", "usage: gadak export [--out FILE]")
	}

	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	ctx := context.Background()

	views, err := db.SavedViews(ctx)
	if err != nil {
		return err
	}
	watches, err := db.Watches(ctx)
	if err != nil {
		return err
	}
	favorites, err := db.Favorites(ctx)
	if err != nil {
		return err
	}
	recents, err := db.Recents(ctx, "")
	if err != nil {
		return err
	}
	recipes, err := db.Recipes(ctx)
	if err != nil {
		return err
	}
	dashboards, err := db.Dashboards(ctx)
	if err != nil {
		return err
	}
	visits, searches, err := exportHistory()
	if err != nil {
		return err
	}
	sessions, writes, err := exportHistorySummaries()
	if err != nil {
		return err
	}

	doc := personalExport{
		Version:     personalExportVersion,
		ExportedAt:  time.Now().UTC().Format(time.RFC3339),
		Views:       views,
		Watches:     watches,
		Favorites:   favorites,
		Recents:     recents,
		Visits:      visits,
		Searches:    searches,
		Recipes:     recipes,
		Dashboards:  dashboards,
		Sessions:    sessions,
		AgentWrites: writes,
	}
	raw, err := marshalPersonalExport(doc)
	if err != nil {
		return err
	}

	if *outPath == "" {
		_, err = os.Stdout.Write(raw)
		return err
	}
	if err := os.WriteFile(*outPath, raw, 0o600); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "exported %d views, %d watches, %d favorites, %d recents, %d visits, %d searches, %d recipes, %d dashboards, %d sessions, %d agent writes to %s\n",
		len(doc.Views), len(doc.Watches), len(doc.Favorites), len(doc.Recents),
		len(doc.Visits), len(doc.Searches), len(doc.Recipes), len(doc.Dashboards),
		len(doc.Sessions), len(doc.AgentWrites), *outPath)
	return nil
}

// exportHistory reads the whole visit and search timeline through the same
// read-only surface `gadak sql` uses (store.OpenReadOnly ATTACHes local.db;
// GDK-753 keeps local reads on that path). Raw rows rather than the History
// reader, on purpose: History paginates, orders for the feed, and filters to
// the current origin epoch — and a file that is a person's whole record must
// not drop the epochs a workspace retired (localSchemaV3 kept those rows
// readable for exactly this). IDs ride along as stable ordering; the epoch
// number itself is workspace-local bookkeeping and stays out of the file.
func exportHistory() ([]store.Visit, []store.Search, error) {
	ro, err := openReadOnly()
	if err != nil {
		return nil, nil, err
	}
	defer ro.Close()

	vrows, err := ro.Query(`SELECT id, kind, key, viewed_at, source, seen_updated_at FROM local.visits ORDER BY id`)
	if err != nil {
		return nil, nil, err
	}
	defer vrows.Close()
	var visits []store.Visit
	for vrows.Next() {
		var v store.Visit
		if err := vrows.Scan(&v.ID, &v.Kind, &v.Key, &v.ViewedAt, &v.Source, &v.SeenUpdatedAt); err != nil {
			return nil, nil, err
		}
		visits = append(visits, v)
	}
	if err := vrows.Err(); err != nil {
		return nil, nil, err
	}

	srows, err := ro.Query(`SELECT id, query, searched_at, result_count, opened_kind, opened_key FROM local.searches ORDER BY id`)
	if err != nil {
		return nil, nil, err
	}
	defer srows.Close()
	var searches []store.Search
	for srows.Next() {
		var s store.Search
		if err := srows.Scan(&s.ID, &s.Query, &s.SearchedAt, &s.ResultCount, &s.OpenedKind, &s.OpenedKey); err != nil {
			return nil, nil, err
		}
		searches = append(searches, s)
	}
	if err := srows.Err(); err != nil {
		return nil, nil, err
	}
	return visits, searches, nil
}

// exportHistorySummaries reads the two history summaries local.db keeps —
// the person-session rows (GDK-1439) and the write ledger (GDK-1440) —
// through the same read-only surface exportHistory uses. All epochs, same
// as the raw events: a replaced origin's sessions are still that person's
// evenings. The ledger's epoch column says which origin a write landed on
// and stays out of the file for the same reason the visits' does.
func exportHistorySummaries() ([]store.Session, []store.AgentWrite, error) {
	ro, err := openReadOnly()
	if err != nil {
		return nil, nil, err
	}
	defer ro.Close()

	srows, err := ro.Query(`SELECT started_at, ended_at, first_write_at, visits FROM local.sessions ORDER BY started_at`)
	if err != nil {
		return nil, nil, err
	}
	defer srows.Close()
	var sessions []store.Session
	for srows.Next() {
		var s store.Session
		if err := srows.Scan(&s.StartedAt, &s.EndedAt, &s.FirstWriteAt, &s.Visits); err != nil {
			return nil, nil, err
		}
		sessions = append(sessions, s)
	}
	if err := srows.Err(); err != nil {
		return nil, nil, err
	}

	wrows, err := ro.Query(`SELECT id, key, at, verb, source FROM local.agent_writes ORDER BY id`)
	if err != nil {
		return nil, nil, err
	}
	defer wrows.Close()
	var writes []store.AgentWrite
	for wrows.Next() {
		var w store.AgentWrite
		if err := wrows.Scan(&w.ID, &w.Key, &w.At, &w.Verb, &w.Source); err != nil {
			return nil, nil, err
		}
		writes = append(writes, w)
	}
	if err := wrows.Err(); err != nil {
		return nil, nil, err
	}
	return sessions, writes, nil
}

func marshalPersonalExport(doc personalExport) ([]byte, error) {
	if doc.Views == nil {
		doc.Views = []store.SavedView{}
	}
	if doc.Watches == nil {
		doc.Watches = []string{}
	}
	if doc.Favorites == nil {
		doc.Favorites = []string{}
	}
	if doc.Recents == nil {
		doc.Recents = []store.Recent{}
	}
	if doc.Visits == nil {
		doc.Visits = []store.Visit{}
	}
	if doc.Searches == nil {
		doc.Searches = []store.Search{}
	}
	if doc.Recipes == nil {
		doc.Recipes = []store.Recipe{}
	}
	if doc.Dashboards == nil {
		doc.Dashboards = []store.Dashboard{}
	}
	if doc.Sessions == nil {
		doc.Sessions = []store.Session{}
	}
	if doc.AgentWrites == nil {
		doc.AgentWrites = []store.AgentWrite{}
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	raw = append(raw, '\n')
	if name := secretscan.Match(string(raw)); name != "" {
		return nil, fmt.Errorf("refusing to write export: credential-shaped string detected (pattern=%s)", name)
	}
	return raw, nil
}
