// Package snapshot builds shareable mirror copies for demos and benchmarks.
// See specs/000-product/contracts/sync.md "Snapshot generation".
package snapshot

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/midagedev/gadak/internal/store"

	_ "modernc.org/sqlite"
)

// Options controls snapshot generation. Now is injectable for deterministic tests.
type Options struct {
	From   string
	Out    string
	Spread time.Duration // 0 = keep source timestamps
	Scale  int           // target issue count; ≤ source count means no cloning
	Seed   int64         // reserved for determinism; algorithm itself is seed-stable
	Force  bool
	Now    time.Time
}

// Result is the human-readable summary of a successful Build.
type Result struct {
	Path      string
	Issues    int
	Comments  int
	Changelog int
	Spread    time.Duration
	Scale     int
	Bytes     int64
}

// Build creates a new shareable database at opts.Out from opts.From.
// Output is written via a temp file and renamed only after a credential scan.
func Build(opts Options) (Result, error) {
	var zero Result
	if opts.Out == "" {
		return zero, fmt.Errorf("output path is required")
	}
	if opts.From == "" {
		return zero, fmt.Errorf("source path is required")
	}
	if !opts.Now.IsZero() {
		opts.Now = opts.Now.UTC()
	}
	if opts.Seed == 0 {
		opts.Seed = 1
	}
	_ = opts.Seed // algorithm is deterministic without RNG; seed is part of the contract

	if _, err := os.Stat(opts.From); err != nil {
		return zero, fmt.Errorf("source %q: %w", opts.From, err)
	}
	if _, err := os.Stat(opts.Out); err == nil && !opts.Force {
		return zero, fmt.Errorf("output %q already exists (pass --force to overwrite)", opts.Out)
	}

	// "now" is the wall clock unless the caller pins it. Anchoring to the
	// source's own latest timestamp would make every build reproducible for
	// free, but it also freezes the snapshot in the past: sources.synced_at
	// would land back when the mirror was last synced, and the sync-health
	// badge reads that field, so a fresh snapshot would show as permanently
	// delayed (docs/project/STATE_OF_PLAY.md, hard-won knowledge #13). Reproducible
	// builds pass --now instead.
	if opts.Now.IsZero() {
		opts.Now = time.Now().UTC()
	}

	tmp := fmt.Sprintf("%s.tmp-%d", opts.Out, os.Getpid())
	_ = os.Remove(tmp)
	_ = os.Remove(tmp + "-wal")
	_ = os.Remove(tmp + "-shm")
	cleanupTmp := true
	defer func() {
		if !cleanupTmp {
			return
		}
		_ = os.Remove(tmp)
		_ = os.Remove(tmp + "-wal")
		_ = os.Remove(tmp + "-shm")
	}()

	if err := buildInto(tmp, opts); err != nil {
		return zero, err
	}

	// The column-bag copier drops the flow columns by design: issueColumns is
	// the destination's static list, and spread moves the very stamps a
	// copied started_at / cycle_hours / last_activity_at would disagree with.
	// They land at their DEFAULT and are re-derived from the destination's
	// own spread rows here (store.BackfillFlow, the v43 migration hook), so
	// the shipped flow columns always agree with the timestamps the snapshot
	// actually carries. open_blockers rides the same call's full sweep.
	sdb, err := store.Open(tmp)
	if err != nil {
		return zero, err
	}
	if err := sdb.BackfillFlow(context.Background()); err != nil {
		_ = sdb.Close()
		return zero, fmt.Errorf("backfill flow columns: %w", err)
	}
	if err := sdb.Close(); err != nil {
		return zero, err
	}

	// Credential scan before publish.
	scanDB, err := openSQLite(tmp, false)
	if err != nil {
		return zero, err
	}
	scanErr := scanCredentials(scanDB)
	_ = scanDB.Close()
	if scanErr != nil {
		return zero, scanErr
	}

	// Publish: remove destination if force, then rename.
	if opts.Force {
		_ = os.Remove(opts.Out)
		_ = os.Remove(opts.Out + "-wal")
		_ = os.Remove(opts.Out + "-shm")
	}
	if err := os.Rename(tmp, opts.Out); err != nil {
		// Cross-device rename can fail; fall back to copy+remove.
		if err := copyFile(tmp, opts.Out); err != nil {
			return zero, fmt.Errorf("publish snapshot: %w", err)
		}
		_ = os.Remove(tmp)
	}
	cleanupTmp = false

	info, err := os.Stat(opts.Out)
	if err != nil {
		return zero, err
	}
	counts, err := countMain(opts.Out)
	if err != nil {
		return zero, err
	}
	return Result{
		Path:      opts.Out,
		Issues:    counts.issues,
		Comments:  counts.comments,
		Changelog: counts.changelog,
		Spread:    opts.Spread,
		Scale:     opts.Scale,
		Bytes:     info.Size(),
	}, nil
}

type counts struct {
	issues, comments, changelog int
}

func countMain(path string) (counts, error) {
	var c counts
	db, err := openSQLite(path, true)
	if err != nil {
		return c, err
	}
	defer db.Close()
	if err := db.QueryRow(`SELECT COUNT(*) FROM issues`).Scan(&c.issues); err != nil {
		return c, err
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM comments`).Scan(&c.comments); err != nil {
		return c, err
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM changelog`).Scan(&c.changelog); err != nil {
		return c, err
	}
	return c, nil
}

// plannedIssue is one output issue (original or clone) with optional remapped times.
type plannedIssue struct {
	src      issueRow
	cloneSeq int // 0 = original
	// remapped timestamps (empty strings mean keep source)
	createdAt, updatedAt          string
	statusChangedAt, resolvedAt   string
	reopenedAt, assigneeChangedAt string
	itemCreatedAt, itemUpdatedAt  string
	itemSyncedAt                  string
	// for linear mapping of children
	srcLo, srcHi, dstLo, dstHi time.Time
	useMap                     bool
	zeroSpan                   bool
}

type issueRow struct {
	itemID, key, projectKey string
	createdAt, updatedAt    string
	// full column bags for insert
	itemCols  map[string]any
	issueCols map[string]any
}

type children struct {
	comments    []map[string]any
	attachments []map[string]any
	changelog   []map[string]any
	// by item_id
	commentsBy    map[string][]map[string]any
	attachmentsBy map[string][]map[string]any
	changelogBy   map[string][]map[string]any
}

// pageRow is one document (items row + pages projection) for snapshot copy.
type pageRow struct {
	itemID, key        string
	createdAt          string
	itemCols, pageCols map[string]any
}

func openSQLite(path string, readOnly bool) (*sql.DB, error) {
	mode := "rwc"
	if readOnly {
		mode = "ro"
	}
	// Single-writer one-shot. busy_timeout is already set. Do not add WAL:
	// the snapshot is a shareable artifact and build.go forces journal_mode
	// DELETE on the dest so the output is a single file. synchronous is left
	// at SQLite's default for the same reason — it is part of what ships.
	dsn := "file:" + path + "?mode=" + mode +
		"&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func copyFile(src, dst string) error {
	in, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	return os.WriteFile(dst, in, 0o600)
}
