package store

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCommittedDemoDBMatchesCurrentSchema is the GDK-671 lockstep gate.
// examples/demo.db is opened read-only on a throwaway byte copy — never via
// Open — because Open migrates and would hide a lagging committed file
// (e2e/serve.sh did that, and go tests that copy-then-Open still do).
//
// Schema owner is len(migrations) / SchemaVersion of a fresh Open, not a
// literal. Derived-table counts are measured on the same unread-migrated
// copy so a snapshot regen that drops item_refs (GDK-114) fails here
// instead of in CI Playwright.
func TestCommittedDemoDBMatchesCurrentSchema(t *testing.T) {
	src := filepath.Join("..", "..", "examples", "demo.db")
	in, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("examples/demo.db is part of the tree: %v", err)
	}
	path := filepath.Join(t.TempDir(), "gadak.db")
	if err := os.WriteFile(path, in, 0o600); err != nil {
		t.Fatal(err)
	}

	// mode=ro: no WAL sidecars, no migrate, no FTS repair.
	raw, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatalf("open demo copy read-only: %v", err)
	}
	defer raw.Close()

	var have int
	if err := raw.QueryRow(`PRAGMA user_version`).Scan(&have); err != nil {
		t.Fatalf("PRAGMA user_version: %v", err)
	}

	fresh, err := Open(filepath.Join(t.TempDir(), "level.db"))
	if err != nil {
		t.Fatalf("open empty mirror for current schema: %v", err)
	}
	want := fresh.SchemaVersion()
	_ = fresh.Close()
	if want != len(migrations) {
		t.Fatalf("SchemaVersion() = %d, len(migrations) = %d — they are the same owner", want, len(migrations))
	}

	if have != want {
		t.Errorf("examples/demo.db PRAGMA user_version = %d, want %d (this binary's migration level). Open() on a copy hides the lag; rebaseline the committed file", have, want)
	}

	// GDK-114 class: derived tables/columns filled by migrate-time backfill
	// or by the snapshot copy set. Zero rows means the fixture lost them.
	for _, table := range []string{"item_refs"} {
		var n int
		if err := raw.QueryRow(`SELECT count(*) FROM ` + table).Scan(&n); err != nil {
			t.Errorf("%s: %v", table, err)
			continue
		}
		if n == 0 {
			t.Errorf("%s has 0 rows on examples/demo.db — derived backfill/copy was lost (GDK-114)", table)
		}
	}
	var excerpts int
	if err := raw.QueryRow(`SELECT count(*) FROM pages WHERE excerpt IS NOT NULL AND excerpt != ''`).Scan(&excerpts); err != nil {
		t.Errorf("pages.excerpt: %v", err)
	} else if excerpts == 0 {
		t.Errorf("pages.excerpt is empty on every page — v15 backfill was lost")
	}
}

// TestCommittedDemoDBMediaNodesCarryAlt is the GDK-1517 recurrence gate.
//
// Live Jira Cloud carries attrs.alt = the attachment's filename on every
// media node (measured 2026-09-10: five issues, 24/24 nodes), and the
// mirror's attachments table has no media id — so alt is the only join the
// renderer has (web/src/lib/adf.ts findAttachment). A fixture media node
// whose alt does not name one of its own issue's attachment filenames
// renders as an "Attachment" chip instead of the image; the NMB-110 comment
// shipped exactly that. Normalization (trim + lowercase) mirrors
// findAttachment's.
func TestCommittedDemoDBMediaNodesCarryAlt(t *testing.T) {
	src := filepath.Join("..", "..", "examples", "demo.db")
	in, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("examples/demo.db is part of the tree: %v", err)
	}
	path := filepath.Join(t.TempDir(), "gadak.db")
	if err := os.WriteFile(path, in, 0o600); err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatalf("open demo copy read-only: %v", err)
	}
	defer raw.Close()

	files := map[string]map[string]bool{}
	attRows, err := raw.Query(`SELECT item_id, filename FROM attachments`)
	if err != nil {
		t.Fatalf("attachments: %v", err)
	}
	for attRows.Next() {
		var item, filename sql.NullString
		if err := attRows.Scan(&item, &filename); err != nil {
			t.Fatal(err)
		}
		if files[item.String] == nil {
			files[item.String] = map[string]bool{}
		}
		files[item.String][normalizeFilename(filename.String)] = true
	}
	attRows.Close()

	type mediaNode struct {
		item string
		at   string
		alt  string
		id   string
	}
	var nodes []mediaNode
	collect := func(item, at, body string) {
		if body == "" {
			return
		}
		var doc any
		if err := json.Unmarshal([]byte(body), &doc); err != nil {
			t.Errorf("%s %s: body_adf is not JSON: %v", item, at, err)
			return
		}
		var walk func(n any)
		walk = func(n any) {
			m, ok := n.(map[string]any)
			if !ok {
				return
			}
			if kind, _ := m["type"].(string); kind == "media" || kind == "mediaInline" {
				attrs, _ := m["attrs"].(map[string]any)
				alt, _ := attrs["alt"].(string)
				id, _ := attrs["id"].(string)
				nodes = append(nodes, mediaNode{item: item, at: at, alt: alt, id: id})
			}
			children, _ := m["content"].([]any)
			for _, c := range children {
				walk(c)
			}
		}
		walk(doc)
	}

	descRows, err := raw.Query(`SELECT item_id, description_adf FROM issues WHERE description_adf IS NOT NULL AND description_adf != ''`)
	if err != nil {
		t.Fatalf("issues.description_adf: %v", err)
	}
	for descRows.Next() {
		var item, body sql.NullString
		if err := descRows.Scan(&item, &body); err != nil {
			t.Fatal(err)
		}
		collect(item.String, "description", body.String)
	}
	descRows.Close()

	comRows, err := raw.Query(`SELECT item_id, id, body_adf FROM comments WHERE body_adf IS NOT NULL AND body_adf != ''`)
	if err != nil {
		t.Fatalf("comments.body_adf: %v", err)
	}
	for comRows.Next() {
		var item, id, body sql.NullString
		if err := comRows.Scan(&item, &id, &body); err != nil {
			t.Fatal(err)
		}
		collect(item.String, "comment "+id.String, body.String)
	}
	comRows.Close()

	if len(nodes) == 0 {
		t.Errorf("no media nodes in the committed fixture — this gate went vacuous (GDK-1517)")
	}
	for _, n := range nodes {
		if strings.TrimSpace(n.alt) == "" {
			t.Errorf("%s %s: media node id=%s has no alt — renders as the Attachment chip, not the image (GDK-1517)", n.item, n.at, n.id)
			continue
		}
		if !files[n.item][normalizeFilename(n.alt)] {
			t.Errorf("%s %s: media node alt %q names no attachment filename of this issue (GDK-1517)", n.item, n.at, n.alt)
		}
	}
	t.Logf("media nodes checked: %d", len(nodes))
}

func normalizeFilename(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// TestCommittedLinearDemoDBMatchesCurrentSchema is the same lockstep gate for
// the Linear fixture (GDK-1298): examples/demo-linear.db serves on the
// suite's second port, and a schema bump that regenerates demo.db but not
// this file would leave e2e/serve.sh refusing it only at serve time — this
// test fails it in `go test ./...` where the round sees it. Same read-only
// copy discipline: never Open, which migrates and hides the lag.
//
// Shape invariants here are only the ones the file exists to hold (a linear
// source, the relation set, comments); the mapping contract itself lives in
// tools/seed-demo/linear_seed_test.go, which runs the generator.
func TestCommittedLinearDemoDBMatchesCurrentSchema(t *testing.T) {
	src := filepath.Join("..", "..", "examples", "demo-linear.db")
	in, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("examples/demo-linear.db is part of the tree: %v", err)
	}
	path := filepath.Join(t.TempDir(), "gadak.db")
	if err := os.WriteFile(path, in, 0o600); err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatalf("open linear demo copy read-only: %v", err)
	}
	defer raw.Close()

	var have int
	if err := raw.QueryRow(`PRAGMA user_version`).Scan(&have); err != nil {
		t.Fatalf("PRAGMA user_version: %v", err)
	}
	fresh, err := Open(filepath.Join(t.TempDir(), "level.db"))
	if err != nil {
		t.Fatalf("open empty mirror for current schema: %v", err)
	}
	want := fresh.SchemaVersion()
	_ = fresh.Close()

	if have != want {
		t.Errorf("examples/demo-linear.db PRAGMA user_version = %d, want %d — regenerate with `make demo-linear-fixture`", have, want)
	}

	for _, q := range []struct {
		what string
		sql  string
	}{
		{"one linear source", `SELECT count(*) FROM sources WHERE kind = 'linear'`},
		{"the fixed relation set", `SELECT count(*) FROM links`},
		{"mirrored comments", `SELECT count(*) FROM comments`},
		{"the cycles boards", `SELECT count(*) FROM boards WHERE type = 'cycles'`},
	} {
		var n int
		if err := raw.QueryRow(q.sql).Scan(&n); err != nil {
			t.Errorf("%s: %v", q.what, err)
		} else if n == 0 {
			t.Errorf("%s: 0 rows on examples/demo-linear.db — the fixture was emptied or truncated", q.what)
		}
	}
}
