package server

/*
 * GDK-803: golden fixtures for the REST responses the phone app decodes.
 *
 * The phone's wire types (mobile/src/lib/types.ts) are Picks of the desk's
 * web/src/lib/types.ts, but until this file there was nothing proving the
 * *server* still emits those shapes: the two clients share a repository, so
 * a renamed field should turn the same commit red on both sides — the
 * orca/protocol-version drift this test class exists for. offer-vectors.json
 * (internal/pairing) is the precedent: the Go side emits the golden from a
 * real handler, the TS side consumes the same bytes.
 *
 * Each golden is the exact body of a real handler answer over the shared
 * fixture (fixtureAt in server_test.go), with the clock-derived fields
 * normalized (see scrubVolatile) so the bytes are stable across runs. The
 * phone's consumer is mobile/src/lib/contract.test.ts, which imports these
 * files directly — a field deleted or retyped on the server regenerates the
 * golden here and breaks the phone's compile/test there. One commit, both
 * suites red; that is the whole point.
 *
 * Regenerate after an intentional contract change (never hand-edit):
 *
 *	go test ./internal/server/ -run TestRESTResponseGoldens -update
 *
 * A missing golden is a failure, not an empty pass: a moved file would
 * silently unhook the lockstep (the same rule offer-vectors carries).
 */

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
)

var goldenUpdate = flag.Bool("update", false, "rewrite testdata/contract/*.json from the live handlers")

// contractCase is one golden: a fixed request against the fixture, the
// response body it must keep producing, and the clock-derived fields in that
// body (per case — sync_health rides only bootstrap/delta, the lifecycle
// spans only detail). The phone consumers named here are the types the TS
// side pins the same bytes to.
type contractCase struct {
	name string
	// path includes the query string, exactly as the phone sends it.
	path string
	// phone names the mobile type that consumes this body in
	// mobile/src/lib/contract.test.ts (documentation only — the TS import
	// is the enforcement).
	phone string
	// volatile lists the fields scrubVolatile pins. Everything else must be
	// byte-stable: a new clock-derived field that lands here unnamed shows
	// up as a golden diff on the next run and gets named, never ignored.
	volatile []volatileField
}

var contractCases = []contractCase{
	// BootstrapResponse (mobile/src/lib/types.ts): issues rows + server_time
	// + sync_version + flow. The fixture's status names are Korean on
	// purpose: the golden pins that display names localize while
	// status_category stays the stable axis the phone keys on. `flow` is
	// deterministically absent for this fixture (flowFields: the normalized
	// stale threshold is set and the done-issue count is under
	// CycleP85MinSamples), so the golden locks its absence.
	{
		name:  "bootstrap",
		path:  apiBase + "bootstrap/",
		phone: "BootstrapResponse",
		volatile: []volatileField{
			{path: "server_time", kind: "iso", fixed: "2026-09-10T00:00:00.000Z"},
			{path: "sync_health.checked_at", kind: "iso", fixed: "2026-09-10T00:00:00.000Z"},
			{path: "sync_health.sources[].synced_at", kind: "iso", fixed: "2026-09-10T00:00:00.000Z"},
			// NMA-9's only comes from seedMentionComment (max of the row's
			// activity sources); the fixture rows' own values are fixed.
			{path: "issues[].last_activity_at", kind: "iso", fixed: "2026-09-10T00:00:00.000Z"},
		},
	},
	// DeltaResponse: same IssueLite rows, `since` fixed to a stamp that
	// selects exactly NMB-1 (updated 2026-08-01) from the fixture.
	{
		name:  "delta",
		path:  apiBase + "delta/?since=2026-07-15T00:00:00.000Z",
		phone: "IssueLite[] via delta.upserted",
		volatile: []volatileField{
			{path: "server_time", kind: "iso", fixed: "2026-09-10T00:00:00.000Z"},
			{path: "sync_health.checked_at", kind: "iso", fixed: "2026-09-10T00:00:00.000Z"},
			{path: "sync_health.sources[].synced_at", kind: "iso", fixed: "2026-09-10T00:00:00.000Z"},
			{path: "upserted[].last_activity_at", kind: "iso", fixed: "2026-09-10T00:00:00.000Z"},
		},
	},
	// DetailResponse for the one fixture issue that has everything: ADF
	// description, comment, attachment, changelog, link. wait/progress are
	// Durations measured against time.Now() in handleDetail.
	{
		name:  "detail",
		path:  apiBase + "NMB-1/detail/",
		phone: "DetailResponse",
		volatile: []volatileField{
			{path: "wait_ms", kind: "ms", fixed: 60000},
			{path: "progress_ms", kind: "ms", fixed: 60000},
		},
	},
	// FeedResponse (mobile/src/lib/domain.ts): changelog/comment-derived
	// items plus unread_counts, with the query the phone's sync() sends.
	// The fixture's own events are self-authored and outside the 30-day
	// feed window, so seedMentionComment adds one non-self mention —
	// otherwise the golden would lock an empty items array and pin none of
	// the item row the phone's hand-written FeedItem must keep matching.
	{
		name:  "feed",
		path:  apiBase + "feed/?focus=all&limit=20",
		phone: "FeedResponse",
		volatile: []volatileField{
			// The seeded comment rides the real clock so the event stays
			// inside the feed window on any regeneration date.
			{path: "items[].occurred_at", kind: "iso", fixed: "2026-09-10T00:00:00.000Z"},
		},
	},
	// Me: identity from the stored credential. Fully deterministic.
	{name: "me", path: authBase + "me/", phone: "Me"},
}

// volatileField is one clock-derived value in a response body: asserted for
// presence and kind, then pinned to a constant so the golden bytes are
// reproducible.
type volatileField struct {
	// path is dotted; a `[]` segment iterates every array element.
	path string
	// kind: "iso" stamps must be non-empty ISO-8601 strings; "ms" must be
	// non-negative numbers (durations in milliseconds).
	kind string
	fixed any
}

func TestRESTResponseGoldens(t *testing.T) {
	db, cfg, _ := fixtureAt(t)
	// The fixture leaves AccountID unset; mention events need it (mentionHit
	// is disabled on an empty id, and relevance is what admits a feed item).
	// acc-hc is the same account /me/ already resolves for this credential.
	cfg.AccountID = "acc-hc"
	seedMentionComment(t, db)
	h := New(db, cfg)
	goldenDir := filepath.Join("testdata", "contract")
	for _, tc := range contractCases {
		t.Run(tc.name, func(t *testing.T) {
			rec := get(t, h, tc.path, nil)
			if rec.Code != http.StatusOK {
				t.Fatalf("GET %s: status %d, body %s", tc.path, rec.Code, rec.Body.String())
			}
			var body any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("GET %s: decode: %v", tc.path, err)
			}
			scrubVolatile(t, tc.name, body, tc.volatile)
			want, err := json.MarshalIndent(body, "", "  ")
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			want = append(want, '\n')
			golden := filepath.Join(goldenDir, tc.name+".json")
			if *goldenUpdate {
				if err := os.MkdirAll(goldenDir, 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				if err := os.WriteFile(golden, want, 0o644); err != nil {
					t.Fatalf("write golden: %v", err)
				}
				return
			}
			have, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("golden missing (%v). The phone imports these files; regenerate with\n\n\tgo test ./internal/server/ -run TestRESTResponseGoldens -update", err)
			}
			if !bytes.Equal(have, want) {
				// Diff-style reporting without a diff dependency: the
				// normalized JSON is small enough to show whole.
				t.Fatalf("REST contract drifted for %s (%s — %s consumes this).\nIf the change is intentional, regenerate:\n\n\tgo test ./internal/server/ -run TestRESTResponseGoldens -update\n\ncommitted:\n%s\nhandler now emits:\n%s",
					tc.name, tc.path, tc.phone, have, want)
			}
		})
	}
}

// seedMentionComment adds the one feed event the golden needs: a comment by
// the reporter (박보고, acc-rp — not the local user, so isSelfActor keeps it)
// mentioning acc-hc on NMA-9, so relevance yields ["mention"]. Re-sends the
// NMA-9 record verbatim from fixtureAt plus the comment; the timestamp rides
// now−2h so the event sits inside the 30-day feed window on every
// regeneration date, and items[].occurred_at scrubs it back out.
//
// The fixture's own events cannot serve: its comment and changelog are
// 김현철's (self, dropped) and dated July (outside the window — FeedWindowDays
// is 30, so those dates only age further out).
func seedMentionComment(t *testing.T, db *store.DB) {
	t.Helper()
	at := time.Now().UTC().Add(-2 * time.Hour).Format(config.ISOMilli)
	if _, err := db.UpsertIssues(context.Background(), store.Batch{
		// Same catalogs as fixtureAt: the re-upsert must not disturb them.
		Categories: map[string]string{"1": "new", "3": "inprogress", "10001": "done"},
		Priorities: []string{"Highest", "High", "Medium"},
		// The item row is byte-identical to fixtureAt's on purpose (the
		// bootstrap/delta goldens keep their dates), and upsertRecord's
		// conditional DO UPDATE treats an unchanged updated_at as
		// "nothing new" and skips the child rows entirely — Force is the
		// documented bypass (SyncIssue write-through) that lands the
		// comment anyway.
		Force: true,
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "jira:2001", SourceID: "jira", ExternalID: "2001", Key: "NMA-9",
				Title: "modeler crash on import",
				CreatedAt: "2026-07-05T00:00:00.000Z", UpdatedAt: "2026-07-06T00:00:00.000Z",
			},
			Issue: store.Issue{
				ProjectKey: "NMA", IssueType: "Task", Status: "할 일", StatusID: "1",
				StatusCategory: "new",
			},
			Comments: []store.Comment{{
				ID: "jira:c-9", ExternalID: "c-9", Author: "박보고", AuthorID: "acc-rp",
				BodyADF: json.RawMessage(`{"type":"doc","version":1,"content":[{"type":"paragraph","content":[` +
					`{"type":"mention","attrs":{"id":"acc-hc"}},` +
					`{"type":"text","text":" the import crash reproduces with the new build"}]}]}`),
				BodyText: "the import crash reproduces with the new build", CreatedAt: at,
			}},
		}},
	}); err != nil {
		t.Fatalf("seed mention comment: %v", err)
	}
}

// scrubVolatile asserts each listed volatile field exists with the right
// kind and pins it to its constant. Absence where the container exists is a
// failure — a listed field going missing means the handler changed shape and
// the list is stale, which should surface here rather than pass silently
// (except `synced_at`, where a source that never synced legitimately omits
// the key; absence passes there).
func scrubVolatile(t *testing.T, name string, body any, list []volatileField) {
	t.Helper()
	for _, vf := range list {
		optional := vf.path == "sync_health.sources[].synced_at"
		for _, holder := range atPath(t, body, parentPath(vf.path), name, vf.path, optional) {
			key := lastSegment(vf.path)
			m, ok := holder.(map[string]any)
			if !ok {
				t.Fatalf("%s: %s: parent of %s is not an object", name, vf.path, vf.path)
			}
			raw, present := m[key]
			if !present || raw == nil {
				continue
			}
			switch vf.kind {
			case "iso":
				s, ok := raw.(string)
				if !ok || s == "" {
					t.Fatalf("%s: %s must be a non-empty ISO stamp, got %#v", name, vf.path, raw)
				}
			case "ms":
				f, ok := raw.(float64)
				if !ok || f < 0 {
					t.Fatalf("%s: %s must be a non-negative number, got %#v", name, vf.path, raw)
				}
			}
			m[key] = vf.fixed
		}
	}
}

// parentPath drops the last segment ("a.b[].c" → "a.b[]").
func parentPath(p string) string {
	i := len(p) - 1
	for i >= 0 && p[i] != '.' {
		i--
	}
	if i < 0 {
		return ""
	}
	return p[:i]
}

// lastSegment returns the final dotted segment ("a.b[].c" → "c").
func lastSegment(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '.' {
			return p[i+1:]
		}
	}
	return p
}

// atPath resolves a dotted path against a decoded JSON body, expanding `[]`
// segments over every array element. The result is the list of containers
// (objects) the final write targets. An empty path returns the root.
func atPath(t *testing.T, body any, path, caseName, whole string, optional bool) []any {
	t.Helper()
	cur := []any{body}
	if path == "" {
		return cur
	}
	seg := ""
	for i := 0; i < len(path); i++ {
		if path[i] != '.' {
			seg += string(path[i])
			continue
		}
		cur = step(t, cur, seg, caseName, whole, optional)
		seg = ""
	}
	return step(t, cur, seg, caseName, whole, optional)
}

// step advances one segment, mapping arrays when the segment ends in `[]`.
func step(t *testing.T, cur []any, seg, caseName, whole string, optional bool) []any {
	t.Helper()
	if seg == "" {
		t.Fatalf("%s: empty segment in volatile path %s", caseName, whole)
	}
	mapAll := false
	key := seg
	if len(seg) > 2 && seg[len(seg)-2:] == "[]" {
		mapAll, key = true, seg[:len(seg)-2]
	}
	var out []any
	for _, v := range cur {
		m, ok := v.(map[string]any)
		if !ok {
			t.Fatalf("%s: %s walks through a non-object in %s", caseName, whole, seg)
		}
		next, present := m[key]
		if !present {
			if optional {
				continue
			}
			t.Fatalf("%s: volatile path %s has no %q where the container exists", caseName, whole, key)
		}
		if mapAll {
			arr, ok := next.([]any)
			if !ok {
				t.Fatalf("%s: %s: %q is not an array", caseName, whole, key)
			}
			out = append(out, arr...)
			continue
		}
		out = append(out, next)
	}
	return out
}

// TestRESTContractGoldensListed guards the coupling itself: the phone test
// reads these five files by name, so a rename here is a contract change for
// mobile/src/lib/contract.test.ts and must travel with it.
func TestRESTContractGoldensListed(t *testing.T) {
	if len(contractCases) != 5 {
		t.Fatalf("phone contract test imports %d goldens by name; adding or removing one here must travel with mobile/src/lib/contract.test.ts", len(contractCases))
	}
	for _, tc := range contractCases {
		if tc.name == "" || tc.phone == "" {
			t.Fatalf("contract case %+v needs a name and a phone consumer", tc)
		}
	}
}
