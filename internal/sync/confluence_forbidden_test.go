package sync

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

// GDK-2049: one page whose body GET answers 403 is a per-page restriction,
// not a rejected credential — unless it is the only answer the pass got.

func forbidBodies(f *confFixture, ids ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bodyStatus = map[string]int{}
	for _, id := range ids {
		f.bodyStatus[id] = http.StatusForbidden
	}
}

func pageTitle(t *testing.T, m *mirror, key string) (string, bool) {
	t.Helper()
	var title string
	err := m.raw(t).QueryRow(`SELECT i.title FROM items i JOIN pages p ON p.item_id = i.id
		WHERE i.source_id = 'confluence' AND i.key = ?`, key).Scan(&title)
	if err != nil {
		return "", false
	}
	return title, true
}

func TestConfluenceBodyForbiddenSkipsThatPageOnly(t *testing.T) {
	f := newConfFixture(t)
	cclient := f.start()
	db := newMirror(t)
	cfg := confCfg([]string{"AAA", "BBB"})
	ctx := context.Background()
	if _, err := RunConfluence(ctx, cfg, db.DB, Options{Full: true, ConfluenceClient: cclient}); err != nil {
		t.Fatal(err)
	}
	before, err := db.SyncState(ctx, ConfluenceSourceID)
	if err != nil {
		t.Fatal(err)
	}
	oldTitle, ok := pageTitle(t, db, "1002")
	if !ok {
		t.Fatal("seed pass did not cache page 1002")
	}

	f.mu.Lock()
	f.pages["1002"].Title = "Release notes (renamed)"
	f.mu.Unlock()
	forbidBodies(f, "1002")

	var log []string
	_, err = RunConfluence(ctx, cfg, db.DB, Options{Full: true, ConfluenceClient: cclient,
		Log: func(l string) { log = append(log, l) }})
	if err != nil {
		t.Fatalf("a 403 on one page body must not fail the pass: %v", err)
	}
	st, err := db.SyncState(ctx, ConfluenceSourceID)
	if err != nil {
		t.Fatal(err)
	}
	if st.LastError != nil || st.SyncCount <= before.SyncCount {
		t.Fatalf("pass must record success: before=%+v after=%+v", before, st)
	}
	for _, key := range []string{"1001", "2001"} {
		if _, ok := pageTitle(t, db, key); !ok {
			t.Errorf("page %s must stay cached", key)
		}
	}
	if got, ok := pageTitle(t, db, "1002"); !ok || got != oldTitle {
		t.Errorf("forbidden page row must be kept unchanged: got %q ok=%v, want %q", got, ok, oldTitle)
	}
	joined := strings.Join(log, "\n")
	if !strings.Contains(joined, "skip 1002 (forbidden, 403") {
		t.Errorf("log must name the page id and 403:\n%s", joined)
	}
}

func TestConfluenceEveryBodyForbiddenStillFailsAsRejectedCredential(t *testing.T) {
	f := newConfFixture(t)
	cclient := f.start()
	db := newMirror(t)
	cfg := confCfg([]string{"AAA", "BBB"})
	ctx := context.Background()
	if _, err := RunConfluence(ctx, cfg, db.DB, Options{Full: true, ConfluenceClient: cclient}); err != nil {
		t.Fatal(err)
	}
	forbidBodies(f, "1001", "1002", "2001")
	_, err := RunConfluence(ctx, cfg, db.DB, Options{Full: true, ConfluenceClient: cclient})
	if err == nil || !IsRejectedCredential(err) {
		t.Fatalf("all-403 pass must fail as a rejected credential, got %v", err)
	}
	st, serr := db.SyncState(ctx, ConfluenceSourceID)
	if serr != nil {
		t.Fatal(serr)
	}
	if st.LastError == nil {
		t.Fatal("all-403 pass must record last_error")
	}
}

func TestWatchContinuesAfterConfluenceBodyForbidden(t *testing.T) {
	f := newConfFixture(t)
	cclient := f.start()
	db := newMirror(t)
	cfg := confCfg([]string{"AAA", "BBB"})
	if _, err := RunConfluence(context.Background(), cfg, db.DB, Options{Full: true, ConfluenceClient: cclient}); err != nil {
		t.Fatal(err)
	}
	forbidBodies(f, "1002")
	f.mu.Lock()
	f.pages["1002"].Version++
	f.pages["1002"].When = "2026-09-01T09:00:00.000Z"
	// A second changed page that reads fine: without one, the lone 403 would
	// be the pass's only answer and the guard would (rightly) fail it.
	f.pages["2001"].Version++
	f.pages["2001"].When = "2026-09-01T10:00:00.000Z"
	f.mu.Unlock()

	site := newSite(t, "en")
	jclient := site.start()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	completed := make(chan struct{}, 4)
	go func() {
		done <- Watch(ctx, cfg, db.DB, Options{
			Client: jclient, ConfluenceClient: cclient, Tick: testTick,
			Log: func(line string) {
				if strings.HasPrefix(line, "confluence done:") {
					select {
					case completed <- struct{}{}:
					default:
					}
				}
			},
		})
	}()
	defer func() {
		cancel()
		<-done
	}()
	timer := time.NewTimer(testNextTickWait)
	defer timer.Stop()
	for range 2 {
		select {
		case <-completed:
		case <-timer.C:
			t.Fatal("Watch must keep syncing the wiki after a per-page body 403")
		}
	}
	state, err := db.SyncState(context.Background(), ConfluenceSourceID)
	if err != nil {
		t.Fatal(err)
	}
	if state.LastError != nil || state.SyncCount < 3 {
		t.Fatalf("wiki must keep syncing: %+v", state)
	}
}

// An incremental pass whose only changed page is the restricted one has no
// page that read fine, yet the listing it just ran proves the credential.
// It must not fail as a rejected credential: that pass would repeat on every
// tick, because the watermark never moves past the page (GDK-2049).
func TestConfluenceIncrementalOnlyForbiddenPageSucceeds(t *testing.T) {
	f := newConfFixture(t)
	cclient := f.start()
	db := newMirror(t)
	cfg := confCfg([]string{"AAA", "BBB"})
	ctx := context.Background()
	if _, err := RunConfluence(ctx, cfg, db.DB, Options{Full: true, ConfluenceClient: cclient}); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.pages["1002"].Version = 3
	f.pages["1002"].When = "2026-08-03T12:00:00.000Z"
	f.mu.Unlock()
	forbidBodies(f, "1002")

	res, err := RunConfluence(ctx, cfg, db.DB, Options{ConfluenceClient: cclient})
	if err != nil {
		t.Fatalf("an incremental pass whose only change is a restricted page must not fail: %v", err)
	}
	if res.Full {
		t.Fatal("second run must be incremental")
	}
	if _, ok := pageTitle(t, db, "1002"); !ok {
		t.Error("forbidden page row must be kept")
	}
	st, err := db.SyncState(ctx, ConfluenceSourceID)
	if err != nil {
		t.Fatal(err)
	}
	if st.LastError != nil {
		t.Fatalf("pass must record success, last_error = %q", *st.LastError)
	}
}
