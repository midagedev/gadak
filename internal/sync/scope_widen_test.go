package sync

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

// GDK-2051 — adding one project to the scope must back-fill only that
// project. GDK-1400 made every scope change one full pass, which re-fetched
// every old project too: a 4 → 5 project widen on a real site spent ten
// minutes and 2,155 requests re-reading four projects that were current.

// fullHistoryProjects returns the projects the site was asked a full-history
// search for (`project = "X" ORDER BY updated DESC`), in request order.
func (s *honestSite) fullHistoryProjects() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for _, jql := range s.searchJQLs {
		if strings.HasPrefix(jql, `project = "`) && strings.HasSuffix(jql, "ORDER BY updated DESC") {
			out = append(out, strings.Split(jql, `"`)[1])
		}
	}
	return out
}

// scopeSite seeds A and B as current projects and C as an old project whose
// issues all predate the watermark the A/B pass earns.
func scopeSite(t *testing.T) (*honestSite, time.Time) {
	site := newHonestSite(t)
	now := time.Now().Truncate(time.Second)
	site.put("A-1", now.Add(-3*time.Hour))
	site.put("B-1", now.Add(-2*time.Hour))
	site.put("C-1", now.Add(-96*time.Hour))
	site.put("C-2", now.Add(-95*time.Hour))
	return site, now
}

func sortedMirrorKeys(t *testing.T, db *mirror) []string {
	k := mirrorKeys(t, db)
	sort.Strings(k)
	return k
}

func TestScopeWidenBackfillsOnlyTheAddedProject(t *testing.T) {
	ctx := context.Background()
	site, _ := scopeSite(t)
	db := newMirror(t)
	cfg := gdkConfig()
	cfg.Projects = []string{"A", "B"}
	client := site.start()

	if _, err := Run(ctx, cfg, db.DB, Options{Full: true, Client: client}); err != nil {
		t.Fatal(err)
	}
	if got, want := sortedMirrorKeys(t, db), []string{"A-1", "B-1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("first pass mirrored %v, want %v", got, want)
	}
	before := site.fullHistoryProjects()

	cfg.Projects = []string{"A", "B", "C"}
	var logs []string
	res, err := Run(ctx, cfg, db.DB, Options{Client: client, Log: func(s string) { logs = append(logs, s) }})
	if err != nil {
		t.Fatal(err)
	}
	got := site.fullHistoryProjects()[len(before):]
	if !reflect.DeepEqual(got, []string{"C"}) {
		t.Errorf("widen A,B → A,B,C sent full-history searches for %v, want only [C] (GDK-2051)", got)
	}
	if want := []string{"A-1", "B-1", "C-1", "C-2"}; !reflect.DeepEqual(sortedMirrorKeys(t, db), want) {
		t.Errorf("after the widen the mirror holds %v, want %v (C's backlog sits behind the watermark)", sortedMirrorKeys(t, db), want)
	}
	if res.Deleted != 0 {
		t.Errorf("a back-fill must tombstone nothing, deleted %d", res.Deleted)
	}
	if !reflect.DeepEqual(res.Backfilled, []string{"C"}) {
		t.Errorf("Result.Backfilled = %v, want [C]", res.Backfilled)
	}
	if !strings.Contains(strings.Join(logs, "\n"), "scope widened: + C") {
		t.Errorf("the pass did not name the added project; log:\n%s", strings.Join(logs, "\n"))
	}
	state, err := db.SyncState(ctx, SourceID)
	if err != nil {
		t.Fatal(err)
	}
	if state.ScopeHash != "A,B,C" {
		t.Errorf("scope signature after the widen = %q, want A,B,C", state.ScopeHash)
	}

	// Settles: the next run is a plain incremental with no scope line and no
	// full-history search at all.
	n := len(site.fullHistoryProjects())
	logs = nil
	res, err = Run(ctx, cfg, db.DB, Options{Client: client, Log: func(s string) { logs = append(logs, s) }})
	if err != nil {
		t.Fatal(err)
	}
	if res.Full || len(res.Backfilled) != 0 || len(site.fullHistoryProjects()) != n {
		t.Errorf("the run after a widen must be plain incremental: full=%v backfilled=%v", res.Full, res.Backfilled)
	}
	if j := strings.Join(logs, "\n"); strings.Contains(j, "scope changed") || strings.Contains(j, "scope widened") {
		t.Errorf("the settled run still logged a scope change:\n%s", j)
	}
}

// Every other scope change keeps GDK-1400's one full pass over the new scope.
func TestScopeChangeThatIsNotAWidenStaysOneFullPass(t *testing.T) {
	for _, tc := range []struct {
		name     string
		from, to []string
		wantFull []string
	}{
		{"narrow", []string{"A", "B", "C"}, []string{"A", "B"}, []string{"A", "B"}},
		{"replace", []string{"A", "B"}, []string{"A", "C"}, []string{"A", "C"}},
		{"list to all", []string{"A", "B"}, nil, nil},
		{"all to list", nil, []string{"A", "B"}, []string{"A", "B"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			site, _ := scopeSite(t)
			db := newMirror(t)
			cfg := gdkConfig()
			cfg.Projects = tc.from
			client := site.start()
			if _, err := Run(ctx, cfg, db.DB, Options{Full: true, Client: client}); err != nil {
				t.Fatal(err)
			}
			before := len(site.fullHistoryProjects())
			cfg.Projects = tc.to
			res, err := Run(ctx, cfg, db.DB, Options{Client: client})
			if err != nil {
				t.Fatal(err)
			}
			if !res.Full || len(res.Backfilled) != 0 {
				t.Errorf("full=%v backfilled=%v, want one full pass and no back-fill", res.Full, res.Backfilled)
			}
			if tc.to != nil {
				got := site.fullHistoryProjects()[before:]
				if !reflect.DeepEqual(got, tc.wantFull) {
					t.Errorf("full-history searches %v, want %v", got, tc.wantFull)
				}
			}
		})
	}
}

func TestWidenedProjects(t *testing.T) {
	for _, tc := range []struct {
		old, cfg string
		projects []string
		want     []string
		ok       bool
	}{
		{"A,B", "A,B,C", []string{"C", "B", "A"}, []string{"C"}, true},
		{"A", "A,B,C", []string{"A", "B", "C"}, []string{"B", "C"}, true},
		{"A,B", "A,B", []string{"A", "B"}, nil, false},
		{"A,B,C", "A,B", []string{"A", "B"}, nil, false},
		{"A,B", "A,C", []string{"A", "C"}, nil, false},
		{"*", "A,B", []string{"A", "B"}, nil, false},
		{"A,B", "*", nil, nil, false},
	} {
		got, ok := widenedProjects(tc.old, tc.cfg, tc.projects)
		if ok != tc.ok || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("widenedProjects(%q, %q, %v) = %v, %v; want %v, %v", tc.old, tc.cfg, tc.projects, got, ok, tc.want, tc.ok)
		}
	}
}
