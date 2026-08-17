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
	fmt.Printf("imported %d views, %d watches, %d favorites, %d recents\n",
		len(doc.Views), len(doc.Watches), len(doc.Favorites), len(doc.Recents))
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
	if doc.Version != personalExportVersion {
		return personalExport{}, unknown, fmt.Errorf("unsupported gadak_export version %d (this gadak understands %d)", doc.Version, personalExportVersion)
	}
	return doc, unknown, nil
}

// applyPersonalExport upserts file rows. A same-named view or same-key
// watch/favorite is replaced by the file (file wins). Local-only rows stay.
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
	return db.ImportRecents(ctx, doc.Recents)
}

func newPersonalViewID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return store.Now()
	}
	return hex.EncodeToString(b)
}
