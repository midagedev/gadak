package main

// gadak demo — serves the bundled snapshot from a throwaway home, so evaluating
// the UI needs no Jira account and cannot touch a real profile.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/midagedev/gadak/internal/attachcache"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
)

// cmdDemo serves the bundled snapshot from a throwaway home, so evaluating the
// UI needs no Jira account and cannot touch a real profile.
// importDemoAttachments loads <snapshotDir>/attachments/ into the demo cache.
// Demo config has no site, so Key collapses to the legacy id-only form.
func importDemoAttachments(snapshotDir, home string) error {
	stats, err := importAttachmentsInto(
		filepath.Join(snapshotDir, "attachments"),
		filepath.Join(home, "attachments"),
		"", "",
		filepath.Join(home, "gadak.db"),
	)
	logAttachmentImport("demo: attachment import", stats)
	return err
}

// importAttachmentDir seeds this profile's cache from a manifest directory. It is
// how a snapshot ships renderable images: bytes cannot be proxied without a
// credential, so a fixture (the demo, the test server, a shared snapshot) hands
// them over instead. site and profile must match the handler's
// attachmentCacheKey (config.Site, config.Profile()).
func importAttachmentDir(dir, site, profile, dbPath string) error {
	cacheDir, err := config.AttachmentDir()
	if err != nil {
		return err
	}
	stats, err := importAttachmentsInto(dir, cacheDir, site, profile, dbPath)
	logAttachmentImport("attachment import", stats)
	return err
}

// importAttachmentsInto is the shared body. A missing directory is not an error:
// the snapshot simply has no images. Every write goes through attachcache.Key;
// ids the mirror does not own are skipped (never stored under the raw id).
func importAttachmentsInto(dir, cacheDir, site, profile, dbPath string) (attachcache.ImportStats, error) {
	manifestPath := filepath.Join(dir, "manifest.json")
	if _, err := os.Stat(manifestPath); err != nil {
		if os.IsNotExist(err) {
			return attachcache.ImportStats{}, nil
		}
		return attachcache.ImportStats{}, err
	}
	byID, err := attachmentIssueKeys(dbPath)
	if err != nil {
		return attachcache.ImportStats{}, err
	}
	cache, err := attachcache.New(cacheDir, 0, 0)
	if err != nil {
		return attachcache.ImportStats{}, err
	}
	return cache.ImportManifest(dir, site, profile, func(id string) (string, bool) {
		k, ok := byID[id]
		return k, ok
	})
}

// attachmentIssueKeys maps attachments.external_id and attachments.id to the
// cache owner that holds them: an issue key, or "pages/"+page key for a page's
// attachment (GDK-1541 — the shared table, one owner convention with the
// server's pageOwner). There is no store helper for this lookup (searched
// before adding one); OpenReadOnly is the existing read-SQL surface.
func attachmentIssueKeys(dbPath string) (map[string]string, error) {
	if dbPath == "" {
		return nil, fmt.Errorf("attachment import: empty mirror path")
	}
	db, err := store.OpenReadOnly(dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(`
		SELECT COALESCE(a.external_id, ''), a.id,
		       COALESCE(i.key, 'pages/' || it.key), it.kind
		FROM attachments a
		JOIN items it ON it.id = a.item_id
		LEFT JOIN issues i ON i.item_id = a.item_id
		WHERE it.key IS NOT NULL AND it.key != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var ext, id, owner, kind string
		if err := rows.Scan(&ext, &id, &owner, &kind); err != nil {
			return nil, err
		}
		// Only issues and pages own attachment rows today; a future kind
		// has no cache owner yet and its ids are skipped, not guessed.
		if kind != "issue" && kind != "page" {
			continue
		}
		if ext != "" {
			out[ext] = owner
		}
		if id != "" {
			out[id] = owner
		}
	}
	return out, rows.Err()
}

// logAttachmentImport is the one-line import summary (serve/demo/export-static).
// A log line, not `gadak status`: import is a startup action, and status has no
// slot for a one-shot fixture seed.
func logAttachmentImport(prefix string, stats attachcache.ImportStats) {
	if stats.Seeded == 0 && len(stats.SkippedIDs) == 0 {
		return
	}
	if n := len(stats.SkippedIDs); n == 0 {
		log.Printf("%s: seeded %d", prefix, stats.Seeded)
		return
	}
	log.Printf("%s: seeded %d, skipped %d (not in mirror: %s)",
		prefix, stats.Seeded, len(stats.SkippedIDs), strings.Join(stats.SkippedIDs, ", "))
}

// freshenDemoClock stamps the throwaway demo copy as just-synced.
func freshenDemoClock(dbPath string) error {
	// dbPath is the temp-home copy cmdDemo wrote a moment above, never the
	// committed fixture and never a user workspace file, so the dev-lockout
	// policy does not apply: an explicit MigrateForward migrates the copy freely.
	db, err := store.OpenWith(dbPath, store.OpenOptions{ForwardMigration: store.MigrateForward})
	if err != nil {
		return err
	}
	defer db.Close()
	return db.FreshenSyncClock(context.Background())
}

func cmdDemo(args []string) error {
	fs := newFlagSet("demo")
	addr := fs.String("addr", "127.0.0.1:7878", "listen address")
	static := fs.String("static", "dist/app", "directory holding the built web UI")
	dbPath := fs.String("db", "examples/demo.db", "snapshot to serve")
	noOpen := fs.Bool("no-open", false, "do not open the browser after the server starts")
	if err := fs.Parse(args); err != nil {
		return err
	}
	src, err := os.ReadFile(*dbPath)
	if err != nil {
		return fmt.Errorf("demo snapshot %q not found — run from the repo root or pass --db: %w", *dbPath, err)
	}
	home, err := os.MkdirTemp("", "gadak-demo-")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(home, "gadak.db"), src, 0o600); err != nil {
		return err
	}
	// The identity the retro resume row needs (GDK-1729); see demoUserEmail.
	demoCfgMap := map[string]any{"projects": []string{"NMB", "NMA", "NMS"}, "email": demoUserEmail}
	if id := demoAccountID(filepath.Join(home, "gadak.db")); id != "" {
		demoCfgMap["account_id"] = id
	}
	demoCfg, err := json.Marshal(demoCfgMap)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(home, "config.json"), demoCfg, 0o600); err != nil {
		return err
	}
	os.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	// Attachment bytes cannot be proxied without a credential, so the snapshot
	// ships them and they are imported into the cache: the demo shows real
	// screenshots and inline comment images with no Jira account at all.
	if err := importDemoAttachments(filepath.Dir(*dbPath), home); err != nil {
		log.Printf("demo: attachment bytes unavailable (%v) — images will not render", err)
	}
	// The snapshot ages on the shelf, and a demo that opens with "Sync delayed"
	// reads as a defect rather than as the freshness guard it is. This is a
	// throwaway copy, so stamp its clock as current.
	if err := freshenDemoClock(filepath.Join(home, "gadak.db")); err != nil {
		log.Printf("demo: could not freshen the sync clock: %v", err)
	}
	log.Printf("demo mirror in %s (deleted on exit)", home)
	defer os.RemoveAll(home)
	serveArgs := []string{"--addr", *addr, "--static", *static}
	if *noOpen {
		serveArgs = append(serveArgs, "--no-open")
	}
	return cmdServe(serveArgs)
}

// The demo's own identity. GDK-1729: `gadak demo` and e2e/serve.sh both wrote
// a config with no account id, and store.IsSelfActor matches on AccountID (or
// a display name), so nothing in the fixture was ever "mine" — the retro
// resume cell, one of the four summary numbers, was a dash in the demo, in
// every recording and in every e2e run.
//
// The email is the constant; the account id is looked up in the mirror being
// served rather than written down a second time, so the two cannot drift and a
// regenerated fixture keeps working. e2e/serve.sh derives it the same way from
// the same email.
const demoUserEmail = "dana@example.com"

// demoAccountID is the account id the served mirror gives demoUserEmail, or ""
// when the fixture does not know that person — in which case the config keeps
// the email alone, which is what it carried before, rather than a guess.
func demoAccountID(dbPath string) string {
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		return ""
	}
	defer db.Close()
	var id string
	// Assignee and reporter both carry the pair; either answers, and the
	// fixture's users table is empty so this is the only place it lives.
	if err := db.QueryRow(`
		SELECT assignee_id FROM issues_full
		 WHERE assignee_email = ? AND assignee_id != '' LIMIT 1`, demoUserEmail).Scan(&id); err == nil && id != "" {
		return id
	}
	if err := db.QueryRow(`
		SELECT reporter_id FROM issues_full
		 WHERE reporter_email = ? AND reporter_id != '' LIMIT 1`, demoUserEmail).Scan(&id); err == nil {
		return id
	}
	return ""
}
