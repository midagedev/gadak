package store

import (
	"bytes"
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureLog swaps the package's log output for the duration of fn and returns
// what fn wrote. The repair path logs through the standard logger like the
// rest of the package ("store: ..."), so this is the observability hook the
// no-rebuild assertions use.
func captureLog(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	oldWriter := log.Writer()
	oldFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(oldWriter)
		log.SetFlags(oldFlags)
	}()
	fn()
	return buf.String()
}

// GDK-112: the committed demo snapshot's items_fts was rebuilt without
// contentless_delete=1 (GDK-101, Datasette Lite portability), but writeFTS
// replaces rows with DELETE FROM items_fts — which a contentless table without
// that option rejects. Open must detect the diverged DDL and rebuild the index
// from items/comments before the first write dies with "SQL logic error".
//
// The failing write has to UPDATE an existing item: a brand-new item's DELETE
// targets a rowid that is not in the index yet, matches nothing, and is
// silently legal on a contentless table. The first sync that changes an
// already-mirrored issue is what dies in production.
func TestOpenRepairsSnapshotFTSBeforeWrite(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "examples", "demo.db"))
	if err != nil {
		t.Fatalf("examples/demo.db is part of the tree: %v", err)
	}
	path := filepath.Join(t.TempDir(), "gadak.db")
	if err := os.WriteFile(path, src, 0o600); err != nil {
		t.Fatal(err)
	}

	db, err := Open(path)
	if err != nil {
		t.Fatalf("open snapshot copy: %v", err)
	}

	// One batch that goes through writeFTS both ways: an update of an existing
	// issue (the DELETE half — the half the scrubbed snapshot breaks) and a
	// fresh issue (the INSERT half).
	var id, sourceID, key string
	if err := db.sql.QueryRow(`
		SELECT it.id, it.source_id, it.key
		FROM items it JOIN issues i ON i.item_id = it.id
		ORDER BY it.id LIMIT 1`).Scan(&id, &sourceID, &key); err != nil {
		db.Close()
		t.Fatalf("snapshot has no issue to update: %v", err)
	}
	update := IssueRecord{
		Item: Item{
			ID: id, SourceID: sourceID, Kind: "issue", Key: key,
			Title:     "FTS repair update probe axolotl",
			BodyText:  "probe body",
			CreatedAt: "2026-08-15T00:00:00.000Z", UpdatedAt: "2026-08-15T00:00:01.000Z",
		},
		Issue: Issue{StatusCategory: "new"},
	}
	insert := IssueRecord{
		Item: Item{
			ID: sourceID + ":gdk112", SourceID: sourceID, Kind: "issue",
			ExternalID: "gdk112", Key: "GDK-112",
			Title:     "FTS repair insert probe quokka",
			BodyText:  "probe body",
			CreatedAt: "2026-08-15T00:00:00.000Z", UpdatedAt: "2026-08-15T00:00:00.000Z",
		},
		Issue: Issue{StatusCategory: "new"},
	}
	if _, err := db.UpsertIssues(context.Background(), Batch{Force: true, Records: []IssueRecord{update, insert}}); err != nil {
		db.Close()
		t.Fatalf("first write after Open must pass through a repaired items_fts: %v", err)
	}

	// The rebuilt index still answers for the snapshot's own content and for
	// both rows the batch just wrote. 'upload' is the scrub script's own probe
	// term; measured match count in examples/demo.db is 4.
	for q, want := range map[string]int{"upload": 1, "axolotl": 1, "quokka": 1} {
		var hits int
		if err := db.sql.QueryRow(`SELECT count(*) FROM items_fts WHERE items_fts MATCH ?`, q).Scan(&hits); err != nil {
			db.Close()
			t.Fatalf("MATCH %q after repair: %v", q, err)
		}
		if hits < want {
			db.Close()
			t.Errorf("MATCH %q after repair: %d hits, want at least %d", q, hits, want)
		}
	}
	var ftsRows, itemRows int
	if err := db.sql.QueryRow(`SELECT count(*) FROM items_fts`).Scan(&ftsRows); err != nil {
		t.Fatal(err)
	}
	if err := db.sql.QueryRow(`SELECT count(*) FROM items`).Scan(&itemRows); err != nil {
		t.Fatal(err)
	}
	if ftsRows != itemRows {
		db.Close()
		t.Errorf("items_fts has %d rows, items has %d — rebuild lost or duplicated rows", ftsRows, itemRows)
	}
	var ddl string
	if err := db.sql.QueryRow(`SELECT sql FROM sqlite_master WHERE name='items_fts'`).Scan(&ddl); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if got, want := normalizeFTSDDL(ddl), normalizeFTSDDL(itemsFTSCreate); got != want {
		db.Close()
		t.Errorf("repaired items_fts DDL is not the canonical shape\n got: %q\nwant: %q", got, want)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopening the repaired copy must not rebuild again.
	var reopened *DB
	logged := captureLog(t, func() {
		reopened, err = Open(path)
	})
	if err != nil {
		t.Fatalf("reopen repaired copy: %v", err)
	}
	defer reopened.Close()
	if strings.Contains(logged, "items_fts") {
		t.Errorf("second Open rebuilt items_fts again; log: %s", logged)
	}
}

// A mirror this build created must open without any FTS repair: the DDL check
// costs one sqlite_master row read on the happy path and must stay quiet.
func TestOpenLeavesCanonicalFTSAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gadak.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var before string
	if err := db.sql.QueryRow(`SELECT sql FROM sqlite_master WHERE name='items_fts'`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	var reopened *DB
	logged := captureLog(t, func() {
		reopened, err = Open(path)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	var after string
	if err := reopened.sql.QueryRow(`SELECT sql FROM sqlite_master WHERE name='items_fts'`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Errorf("items_fts DDL changed across a no-op Open\n got: %q\nwant: %q", after, before)
	}
	if strings.Contains(logged, "items_fts") {
		t.Errorf("fresh mirror Open rebuilt items_fts; log: %s", logged)
	}
}

// itemsFTSCreate is a verbatim quote of the statement schemaV1 executes (the
// migration is append-only, so it cannot reference the constant). If the two
// drift, every healthy mirror looks diverged and gets rebuilt on every Open.
func TestItemsFTSCreateMatchesSchemaV1(t *testing.T) {
	if !strings.Contains(schemaV1, itemsFTSCreate) {
		t.Fatal("itemsFTSCreate is no longer verbatim inside schemaV1 — make the constant and the migration agree")
	}
}

// The reload must reproduce what writeFTS indexed. A contentless table
// exposes no stored text, so parity is row count plus MATCH hits per probe,
// before vs after a forced rebuild. The NULL-title/body row covers the
// COALESCEs external data can hit.
func TestRebuildItemsFTSParity(t *testing.T) {
	db := openTemp(t)
	seed(t, db)
	if _, err := db.sql.Exec(`INSERT INTO items (id, source_id, kind, key)
		VALUES ('jira:nullrow', 'jira', 'issue', 'NMB-NULL')`); err != nil {
		t.Fatal(err)
	}
	probes := []string{"charges", "idempotency", "sandbox", "budget", "retry"}
	matchCounts := func() map[string]int {
		t.Helper()
		out := map[string]int{}
		for _, q := range probes {
			var n int
			if err := db.sql.QueryRow(`SELECT count(*) FROM items_fts WHERE items_fts MATCH ?`, q).Scan(&n); err != nil {
				t.Fatalf("MATCH %q: %v", q, err)
			}
			out[q] = n
		}
		return out
	}
	before := matchCounts()
	var items int
	if err := db.sql.QueryRow(`SELECT count(*) FROM items`).Scan(&items); err != nil {
		t.Fatal(err)
	}

	rows, err := db.rebuildItemsFTS(context.Background())
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if rows != int64(items) {
		t.Errorf("rebuild loaded %d rows, items has %d", rows, items)
	}
	if after := matchCounts(); !mapsEqual(after, before) {
		t.Errorf("rebuild changed search behavior: before=%v after=%v", before, after)
	}
	var ddl string
	if err := db.sql.QueryRow(`SELECT sql FROM sqlite_master WHERE name='items_fts'`).Scan(&ddl); err != nil {
		t.Fatal(err)
	}
	if got, want := normalizeFTSDDL(ddl), normalizeFTSDDL(itemsFTSCreate); got != want {
		t.Errorf("rebuilt items_fts DDL is not the canonical shape\n got: %q\nwant: %q", got, want)
	}
}

func mapsEqual(a, b map[string]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
