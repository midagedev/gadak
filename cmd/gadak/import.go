package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/midagedev/gadak/internal/store"
)

func cmdImport(args []string) error {
	if wantsHelp(args) {
		printHelp("import")
		return nil
	}
	if len(args) != 1 || strings.HasPrefix(args[0], "-") {
		return usageError("import", "usage: gadak import <FILE>")
	}

	raw, err := os.ReadFile(args[0])
	if err != nil {
		return err
	}
	doc, unknown, err := parsePersonalExport(raw)
	if err != nil {
		return err
	}
	for _, k := range unknown {
		fmt.Fprintf(os.Stderr, "warning: ignoring unknown export key %q\n", k)
	}

	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	if err := applyPersonalExport(db, doc); err != nil {
		return err
	}
	fmt.Printf("imported %d views, %d watches, %d favorites, %d recents, %d recipes, %d dashboards\n",
		len(doc.Views), len(doc.Watches), len(doc.Favorites), len(doc.Recents), len(doc.Recipes), len(doc.Dashboards))
	// History is carried, not restored (GDK-1769): a faithful import needs a
	// store-level writer that preserves viewed_at/searched_at, and the only
	// writers that exist re-stamp rows as if they happened now — importing a
	// year of reads as one blob at import time would corrupt the timeline it
	// claims to restore. Say so instead of half-doing it; export keeps every
	// row readable for the version that gains the writer.
	if len(doc.Visits) > 0 || len(doc.Searches) > 0 {
		fmt.Fprintf(os.Stderr, "note: the file carries %d visits and %d searches; this build does not restore history yet\n",
			len(doc.Visits), len(doc.Searches))
	}
	return nil
}

func parsePersonalExport(raw []byte) (personalExport, []string, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return personalExport{}, nil, fmt.Errorf("invalid export JSON: %w", err)
	}
	known := map[string]bool{
		"gadak_export": true,
		"exported_at":  true,
		"views":        true,
		"watches":      true,
		"favorites":    true,
		"recents":      true,
		"visits":       true,
		"searches":     true,
		"recipes":      true,
		"dashboards":   true,
	}
	var unknown []string
	for k := range top {
		if !known[k] {
			unknown = append(unknown, k)
		}
	}
	sort.Strings(unknown)

	var doc personalExport
	if err := json.Unmarshal(raw, &doc); err != nil {
		return personalExport{}, unknown, fmt.Errorf("invalid export JSON: %w", err)
	}
	if doc.Version == 0 {
		return personalExport{}, unknown, fmt.Errorf("missing required field gadak_export (version)")
	}
	if doc.Version < minPersonalExportVersion || doc.Version > personalExportVersion {
		return personalExport{}, unknown, fmt.Errorf("unsupported gadak_export version %d (this gadak understands %d through %d)", doc.Version, minPersonalExportVersion, personalExportVersion)
	}
	return doc, unknown, nil
}

// applyPersonalExport upserts file rows. A same-named view or same-key
// watch/favorite is replaced by the file (file wins). Local-only rows stay.
// Recipes follow the file-wins rule too (PutRecipe upserts by name); a
// dashboard keeps its rule from the merge helper it rides — AbsorbDashboards
// was written for exactly this adoption shape and lets a stored name win, so
// a dashboard being viewed on this machine is never silently reconfigured.
// Visits and searches are deliberately absent: see cmdImport's note.
func applyPersonalExport(db *store.DB, doc personalExport) error {
	ctx := context.Background()
	existing, err := db.SavedViews(ctx)
	if err != nil {
		return err
	}
	byName := make(map[string]store.SavedView, len(existing))
	for _, v := range existing {
		byName[v.Name] = v
	}
	for i, v := range doc.Views {
		if strings.TrimSpace(v.Name) == "" {
			return fmt.Errorf("views[%d]: name is required", i)
		}
		if v.ID == "" {
			v.ID = newPersonalViewID()
		}
		if other, ok := byName[v.Name]; ok && other.ID != v.ID {
			if err := db.DeleteSavedView(ctx, other.ID); err != nil {
				return err
			}
		}
		if err := db.PutSavedView(ctx, v); err != nil {
			return err
		}
		byName[v.Name] = v
	}
	for _, key := range doc.Watches {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if err := db.SetWatch(ctx, key, true); err != nil {
			return err
		}
	}
	for _, key := range doc.Favorites {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if err := db.SetFavorite(ctx, key, true); err != nil {
			return err
		}
	}
	if err := db.ImportRecents(ctx, doc.Recents); err != nil {
		return err
	}
	for i, r := range doc.Recipes {
		// Same failure contract as views above: one bad row names itself and
		// stops the import rather than half-applying a person's file.
		if _, err := db.PutRecipe(ctx, r.Name, r.SQL); err != nil {
			return fmt.Errorf("recipes[%d] (%s): %w", i, r.Name, err)
		}
	}
	return db.AbsorbDashboards(ctx, doc.Dashboards)
}

func newPersonalViewID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return store.Now()
	}
	return hex.EncodeToString(b)
}
