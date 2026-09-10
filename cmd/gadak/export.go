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
const personalExportVersion = 2

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
type personalExport struct {
	Version    int               `json:"gadak_export"`
	ExportedAt string            `json:"exported_at"`
	Views      []store.SavedView `json:"views"`
	Watches    []string          `json:"watches"`
	Favorites  []string          `json:"favorites"`
	Recents    []store.Recent    `json:"recents"`
	Visits     []store.Visit     `json:"visits"`
	Searches   []store.Search    `json:"searches"`
	Recipes    []store.Recipe    `json:"recipes"`
	Dashboards []store.Dashboard `json:"dashboards"`
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

	doc := personalExport{
		Version:    personalExportVersion,
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Views:      views,
		Watches:    watches,
		Favorites:  favorites,
		Recents:    recents,
		Visits:     visits,
		Searches:   searches,
		Recipes:    recipes,
		Dashboards: dashboards,
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
	fmt.Fprintf(os.Stderr, "exported %d views, %d watches, %d favorites, %d recents, %d visits, %d searches, %d recipes, %d dashboards to %s\n",
		len(doc.Views), len(doc.Watches), len(doc.Favorites), len(doc.Recents),
		len(doc.Visits), len(doc.Searches), len(doc.Recipes), len(doc.Dashboards), *outPath)
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
