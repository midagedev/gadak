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

// personalExportVersion is the only gadak_export value this build writes and
// accepts. Bump together with a documented format change.
const personalExportVersion = 1

// personalExport is the on-disk dump of the three personal tables in the
// mirror (saved_views, watches, favorites). It is not a team-config file
// (that is gadak_team_config) and it never carries credentials.
type personalExport struct {
	Version    int               `json:"gadak_export"`
	ExportedAt string            `json:"exported_at"`
	Views      []store.SavedView `json:"views"`
	Watches    []string          `json:"watches"`
	Favorites  []string          `json:"favorites"`
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

	doc := personalExport{
		Version:    personalExportVersion,
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Views:      views,
		Watches:    watches,
		Favorites:  favorites,
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
	fmt.Fprintf(os.Stderr, "exported %d views, %d watches, %d favorites to %s\n",
		len(doc.Views), len(doc.Watches), len(doc.Favorites), *outPath)
	return nil
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
