package main

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/linear"
)

// TestLinearFixtureMirror is the demo-linear fixture's contract gate: the
// generator must produce a mirror whose Linear shape is the one the real
// connector writes. It runs the same entry the Makefile target runs
// (seedLinearMirror) and reads the finished file read-only the way
// internal/store's committed-fixture gate does — never via Open, which
// migrates and would hide a lagging file.
//
// The assertions follow internal/linear/MAPPING.md rather than the
// generator: if the mapping changes, this test is what forces the fixture
// to be regenerated instead of silently drifting.
func TestLinearFixtureMirror(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gadak.db")
	fx, res, err := seedLinearMirror(path, 24, 20260910, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if res.Fetched != 24 || res.Changed != 24 || !res.Full {
		t.Fatalf("result = %+v, want 24 fetched/changed on a first (full) pass", res)
	}

	raw, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatalf("open mirror read-only: %v", err)
	}
	defer raw.Close()

	// Source row: the spec's original wording said base_url should carry
	// the workspace slug, but the production sync pins the display base
	// (internal/sync/linear.go, asserted in internal/sync/linear_test.go)
	// and the slug lives in each item's Linear-minted URL. The fixture
	// follows the code.
	var srcID, srcKind, srcBase string
	if err := raw.QueryRow(`SELECT id, kind, base_url FROM sources`).Scan(&srcID, &srcKind, &srcBase); err != nil {
		t.Fatalf("read sources: %v", err)
	}
	if srcID != "linear" || srcKind != "linear" || srcBase != "https://linear.app" {
		t.Fatalf("sources row = %q/%q/%q, want linear/linear/https://linear.app", srcID, srcKind, srcBase)
	}

	// Dataset↔mirror agreement, the property form: every generated issue
	// lands with the category its state TYPE maps to (never the name), and
	// with priority_rank == Linear's integer. A custom state name ("In
	// Review") must ride the standard started type.
	rows, err := raw.Query(`SELECT i.key, i.status, i.status_category, i.priority_rank, it.url, COALESCE(i.description_adf, '')
		FROM issues i JOIN items it ON it.key = i.key`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	byKey := map[string]linear.Issue{}
	for _, iss := range fx.issues {
		byKey[iss.Identifier] = iss
	}
	seen := 0
	for rows.Next() {
		var key, status, cat, url, adf string
		var rank int
		if err := rows.Scan(&key, &status, &cat, &rank, &url, &adf); err != nil {
			t.Fatal(err)
		}
		iss, ok := byKey[key]
		if !ok {
			t.Fatalf("mirror key %s not in the dataset", key)
		}
		wantCat, known := linear.StatusCategory(iss.State.Type)
		if !known {
			t.Fatalf("fixture state type %q is not one StatusCategory knows", iss.State.Type)
		}
		if cat != wantCat {
			t.Errorf("%s status_category = %q, want %q (state type %q)", key, cat, wantCat, iss.State.Type)
		}
		if status != iss.State.Name {
			t.Errorf("%s status = %q, want the state's display name %q", key, status, iss.State.Name)
		}
		if rank != iss.Priority {
			t.Errorf("%s priority_rank = %d, want Linear's integer %d", key, rank, iss.Priority)
		}
		wantURL := "https://linear.app/" + linearWorkspaceSlug + "/issue/" + key + "-" + iss.ID
		if url != wantURL {
			t.Errorf("%s items.url = %q, want %q", key, url, wantURL)
		}
		if adf != "" {
			t.Errorf("%s description_adf = %q, want empty — markdown must not pose as ADF", key, adf)
		}
		seen++
	}
	if seen != len(fx.issues) {
		t.Fatalf("mirrored %d issues, dataset has %d", seen, len(fx.issues))
	}

	// Relations → links, both sides: the owner gets the outward row, the
	// target the inward one, and the type names are the mirror's Jira-style
	// vocabulary (Blocks/Duplicate/Relates).
	link := func(from, typ, dir, to string) {
		var n int
		if err := raw.QueryRow(`SELECT count(*) FROM links l JOIN items i ON i.id = l.item_id
			WHERE i.key = ? AND l.type = ? AND l.direction = ? AND l.target_key = ?`,
			from, typ, dir, to).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Errorf("link %s %s %s %s: %d rows, want 1", from, typ, dir, to, n)
		}
	}
	link("LNX-1", "Blocks", "outward", "LNX-2")
	link("LNX-2", "Blocks", "inward", "LNX-1")
	link("LNX-1", "Relates", "outward", "LNX-3")
	link("LNX-2", "Duplicate", "outward", "LNX-4")
	link("LNX-4", "Duplicate", "inward", "LNX-2")

	// The one-row link-type catalog the sync attaches to every Linear
	// batch; the detail panel's phrases come from it.
	var ltName, ltIn, ltOut string
	if err := raw.QueryRow(`SELECT name, inward, outward FROM link_types WHERE source_id = 'linear'`).Scan(&ltName, &ltIn, &ltOut); err != nil {
		t.Fatalf("link_types: %v", err)
	}
	if ltName != "Blocks" || ltIn != "is blocked by" || ltOut != "blocks" {
		t.Fatalf("link_types = %q/%q/%q, want Blocks/is blocked by/blocks", ltName, ltIn, ltOut)
	}

	// Cycles → boards/sprints: one "cycles" board per team keyed by the
	// team key, and the unnamed cycle's display name is Linear's own
	// fallback. States are date-derived at the pass clock; with the
	// generator's windows (closed/active/future around now) all three must
	// be present.
	var boards int
	if err := raw.QueryRow(`SELECT count(*) FROM boards WHERE type = 'cycles'`).Scan(&boards); err != nil {
		t.Fatal(err)
	}
	if boards != len(fx.teams) {
		t.Fatalf("cycles boards = %d, want %d", boards, len(fx.teams))
	}
	for _, state := range []string{"closed", "active", "future"} {
		var n int
		if err := raw.QueryRow(`SELECT count(*) FROM sprints WHERE state = ?`, state).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			t.Errorf("no %q sprint — the fixture's three cycle windows must all be represented", state)
		}
	}
	var named int
	if err := raw.QueryRow(`SELECT count(*) FROM sprints WHERE name = 'Cycle 13'`).Scan(&named); err != nil {
		t.Fatal(err)
	}
	if named != len(fx.teams) {
		t.Errorf("sprints named 'Cycle 13' = %d, want %d (unnamed cycles take the fallback name)", named, len(fx.teams))
	}

	// Markdown bodies must land in body_text only. One probe of each is
	// enough here — internal/sync/linear_test.go owns the mapping suite.
	var nComments int
	if err := raw.QueryRow(`SELECT count(*) FROM comments`).Scan(&nComments); err != nil {
		t.Fatal(err)
	}
	if nComments == 0 {
		t.Error("no comments mirrored — the fixture's comments vanished on the way in")
	}
	var md string
	if err := raw.QueryRow(`SELECT body_text FROM items WHERE body_text LIKE '## %' LIMIT 1`).Scan(&md); err != nil {
		t.Errorf("no markdown body found: %v", err)
	} else if !strings.HasPrefix(md, "## ") {
		t.Errorf("body_text = %q, want a markdown heading", md)
	}

	// The e2e config identifies as Dana (e2e/serve.sh E2E_EMAIL); she must
	// have assigned work and an account id, or "mine" surfaces and the
	// account-id lookup arrive empty on the Linear fixture.
	var dana string
	if err := raw.QueryRow(`SELECT assignee_id FROM issues_full WHERE assignee_email = 'dana@example.com' AND assignee_id != '' LIMIT 1`).Scan(&dana); err != nil {
		t.Fatalf("Dana has no assigned issue with an account id: %v", err)
	}
	if !strings.HasPrefix(dana, "00000000-") {
		t.Errorf("Dana assignee_id = %q, want a fixture uuid", dana)
	}
}

// TestLinearFixtureDeterministic pins the generator's reproducibility at the
// dataset level: the same (now, count, seed) must rebuild the same world —
// same keys, URLs and titles, both relation sides intact.
func TestLinearFixtureDeterministic(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	a, err := buildLinearFixture(now, 24, 20260910)
	if err != nil {
		t.Fatal(err)
	}
	b, err := buildLinearFixture(now, 24, 20260910)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.issues) != len(b.issues) {
		t.Fatalf("issue counts differ: %d vs %d", len(a.issues), len(b.issues))
	}
	for i := range a.issues {
		x, y := a.issues[i], b.issues[i]
		if x.Identifier != y.Identifier || x.URL != y.URL || x.Title != y.Title {
			t.Errorf("issue %d differs: %s vs %s", i, x.Identifier, y.Identifier)
		}
		if len(x.Relations.Nodes) != len(y.Relations.Nodes) || len(x.InverseRelations.Nodes) != len(y.InverseRelations.Nodes) {
			t.Errorf("%s relation counts differ", x.Identifier)
		}
	}
}
