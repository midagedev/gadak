package sync

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/atlhttp"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/confluence"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/linear"
)

// Watch tests inject Tick so they do not sit on EffectiveSyncIntervalSec's
// 1-second integer floor. testSleepAfterStop scales with it (1.5×); the
// contracts they protect are unchanged. testNextTickWait is a success-polling
// deadline — every user breaks early on success, so it is generous (20×)
// rather than tight: at 2× a loaded CI runner missed the second tick and
// failed a run whose diff never touched this package (GDK-534).
const (
	testTick           = 100 * time.Millisecond
	testSleepAfterStop = 150 * time.Millisecond
	testNextTickWait   = 20 * testTick
)

// Clause coverage (each clause has a happy path and a violation/boundary):
//
//	ErrAuth stops the source
//	  happy:    TestWatchStopsOnErrAuth (Jira ends Watch)
//	  boundary: TestWatchStopsOnConfluenceErrAuth (Confluence hits freeze; Watch stays up)
//	            TestWatchStopsOnConfluenceForbidden (403 is also ErrAuth)
//	transport retries
//	  happy:    TestWatchRetriesTransportError (Jira 500)
//	  boundary: TestWatchRetriesConfluenceTransportError (Confluence 500; only ErrAuth stops)
//	last_error recorded
//	  happy:    TestWatchStopsOnErrAuth (jira last_error)
//	  boundary: TestWatchStopsOnConfluenceErrAuth (confluence last_error; jira last_error stays empty)
//	last_error distinguishes the source
//	  happy:    TestWatchLastErrorDistinguishesSource (jira: vs confluence:)
//	  boundary: TestWatchLastErrorDistinguishesSource (the two strings are not equal)
//	no further requests after stop
//	  happy:    TestWatchStopsOnErrAuth (Jira hits freeze)
//	  boundary: TestWatchStopsOnConfluenceErrAuth (Confluence hits freeze while Jira keeps ticking)
//	            TestRunConfluenceNamedSpaceErrAuthAbortsPass (path ② Space 401 does not SearchPages)
//	Layer-1 general rule
//	  happy:    TestIsRejectedCredential (third-source implementer is detected)
//	  boundary: TestIsRejectedCredential (transport error is not); TestEveryWatchSourceHasAuthCoverage
//	            TestApplyWatchErrThirdSource (fatal vs skip, by construction)
//	            TestEveryOriginClientAuthSentinelIsRejectedCredential (every var ErrAuth, including linear)
//	            TestApplyWatchErrLinearSentinel (a dead Linear token must mark the source dead)
//	skip is per-credential
//	  happy:    TestWatchConfluenceResumesAfterCredentialReload (new token retries)
//	  boundary: same test, hits stay frozen while Reload keeps the old token

// TestWatchStopsOnErrAuth: a revoked token must end the loop, record last_error
// (status / sync_health read that column), log once, and not schedule another
// request. A later one-shot Run with a working credential must still succeed.
func TestWatchStopsOnErrAuth(t *testing.T) {
	site := newSite(t, "en")
	site.authStatus = http.StatusUnauthorized
	client := site.start()
	db := newMirror(t)
	cfg := testConfig()
	cfg.SyncIntervalSec, cfg.ReconcileIntervalSec = 1, 3600

	var mu sync.Mutex
	var logs []string
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- Watch(ctx, cfg, db.DB, Options{
			Client: client,
			Tick:   testTick,
			Log: func(line string) {
				mu.Lock()
				logs = append(logs, line)
				mu.Unlock()
			},
		})
	}()

	var err error
	select {
	case err = <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Watch did not stop after ErrAuth")
	}
	if !errors.Is(err, jira.ErrAuth) {
		t.Fatalf("Watch returned %v, want ErrAuth", err)
	}

	site.mu.Lock()
	hitsAfterStop := site.hits
	site.mu.Unlock()
	if hitsAfterStop == 0 {
		t.Fatal("Watch returned ErrAuth without requesting Jira")
	}

	time.Sleep(testSleepAfterStop)
	site.mu.Lock()
	hitsLater := site.hits
	site.mu.Unlock()
	if hitsLater != hitsAfterStop {
		t.Fatalf("Watch requested Jira %d more times after ErrAuth (hits %d → %d)",
			hitsLater-hitsAfterStop, hitsAfterStop, hitsLater)
	}

	st, err := db.SyncState(context.Background(), SourceID)
	if err != nil {
		t.Fatal(err)
	}
	if st.LastError == nil || !strings.Contains(*st.LastError, "credential rejected") {
		t.Fatalf("last_error = %v, want the ErrAuth text status/sync_health already surface", st.LastError)
	}

	mu.Lock()
	got := append([]string{}, logs...)
	mu.Unlock()
	var failed int
	for _, line := range got {
		if strings.Contains(line, "sync failed") {
			failed++
		}
	}
	if failed != 1 {
		t.Fatalf("sync-failed log lines = %d, want 1 (once, not per tick); logs = %q", failed, got)
	}
	if !strings.Contains(strings.Join(got, "\n"), "credential rejected") {
		t.Fatalf("log did not carry the existing ErrAuth text: %q", got)
	}

	// Manual `gadak sync` after a new token: one-shot Run must resume and clear
	// last_error. Stopping Watch is process-lifetime; it must not poison the row.
	site.mu.Lock()
	site.authStatus = 0
	site.mu.Unlock()
	if _, err := Run(context.Background(), cfg, db.DB, Options{Full: true, Client: client}); err != nil {
		t.Fatalf("one-shot Run after new token: %v", err)
	}
	st, err = db.SyncState(context.Background(), SourceID)
	if err != nil {
		t.Fatal(err)
	}
	if st.LastError != nil {
		t.Fatalf("last_error = %q after a successful Run — must not be permanent", *st.LastError)
	}
}

// TestWatchRetriesTransportError: a 500 is not ErrAuth — the loop must keep
// requesting on the next tick instead of exiting.
func TestWatchRetriesTransportError(t *testing.T) {
	site := newSite(t, "en")
	site.authStatus = http.StatusInternalServerError
	client := site.start()
	db := newMirror(t)
	cfg := testConfig()
	cfg.SyncIntervalSec, cfg.ReconcileIntervalSec = 1, 3600

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- Watch(ctx, cfg, db.DB, Options{Client: client, Tick: testTick})
	}()

	deadline := time.Now().Add(testNextTickWait)
	for {
		site.mu.Lock()
		n := site.hits
		site.mu.Unlock()
		if n >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("Watch hits = %d after a 500, want >= 2 (retry on the next tick)", n)
		}
		select {
		case err := <-done:
			t.Fatalf("Watch returned %v on a transport error; only ErrAuth may stop the loop", err)
		case <-time.After(50 * time.Millisecond):
		}
	}
	cancel()
	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Watch returned %v, want context.Canceled", err)
	}
}

// confAuthStub is a Confluence origin that answers every request with a fixed
// status. Watch tests use 401/403 (ErrAuth) and 500 (transport).
type confAuthStub struct {
	mu     sync.Mutex
	hits   int
	status int
}

func (s *confAuthStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.hits++
	status := s.status
	s.mu.Unlock()
	if status != 0 {
		http.Error(w, `{"message":"Client must be authenticated"}`, status)
		return
	}
	// status 0 is the post-fix credential: empty space listing is a successful
	// RunConfluence (no spaces in scope) and clears last_error.
	if r.URL.Path == "/wiki/rest/api/space" {
		_, _ = w.Write([]byte(`{"results":[],"size":0,"limit":100,"start":0}`))
		return
	}
	http.NotFound(w, r)
}

func (s *confAuthStub) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hits
}

func startConfAuth(t *testing.T, status int) (*confluence.Client, *confAuthStub) {
	t.Helper()
	stub := &confAuthStub{status: status}
	srv := httptest.NewServer(stub)
	t.Cleanup(srv.Close)
	c := confluence.New(srv.URL, "user@example.invalid", "secret-token")
	c.Retries, c.Backoff, c.PauseBetween = 1, 0, 0
	return c, stub
}

func watchBoth(t *testing.T, jiraAuth, confAuth int) (*fakeSite, *confAuthStub, *mirror, context.CancelFunc, <-chan error, *[]string, *sync.Mutex) {
	t.Helper()
	site := newSite(t, "en")
	site.authStatus = jiraAuth
	jclient := site.start()
	cclient, stub := startConfAuth(t, confAuth)
	db := newMirror(t)
	cfg := testConfig()
	cfg.SyncIntervalSec, cfg.ReconcileIntervalSec = 1, 3600
	cfg.Confluence = &config.ConfluenceConfig{}

	var mu sync.Mutex
	var logs []string
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Watch(ctx, cfg, db.DB, Options{
			Client:           jclient,
			ConfluenceClient: cclient,
			Tick:             testTick,
			Log: func(line string) {
				mu.Lock()
				logs = append(logs, line)
				mu.Unlock()
			},
		})
	}()
	return site, stub, db, cancel, done, &logs, &mu
}

func waitConfluenceLastError(t *testing.T, db *mirror, done <-chan error) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		st, err := db.SyncState(context.Background(), ConfluenceSourceID)
		if err == nil && st.LastError != nil && strings.Contains(*st.LastError, "credential rejected") {
			return *st.LastError
		}
		select {
		case err := <-done:
			t.Fatalf("Watch returned %v before confluence last_error was recorded (state %v)", err, st.LastError)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("confluence last_error not recorded within 2s: %v", st.LastError)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestWatchStopsOnConfluenceErrAuth: a revoked Confluence credential must stop
// *Confluence* retries (hits freeze), record last_error on the confluence
// source, log once, and leave Jira mirroring running. A later one-shot
// RunConfluence with a working credential must still clear last_error.
func TestWatchStopsOnConfluenceErrAuth(t *testing.T) {
	site, stub, db, cancel, done, logs, mu := watchBoth(t, 0, http.StatusUnauthorized)
	defer cancel()

	confErr := waitConfluenceLastError(t, db, done)

	select {
	case err := <-done:
		t.Fatalf("Watch returned %v after Confluence ErrAuth; Jira mirroring must continue", err)
	default:
	}

	hitsAfterStop := stub.count()
	if hitsAfterStop == 0 {
		t.Fatal("Watch recorded confluence last_error without requesting Confluence")
	}

	site.mu.Lock()
	jiraAtStop := site.hits
	site.mu.Unlock()
	if jiraAtStop == 0 {
		t.Fatal("Watch never requested Jira — confluence-dead must not skip the Jira pass")
	}

	time.Sleep(testSleepAfterStop)
	hitsLater := stub.count()
	if hitsLater != hitsAfterStop {
		t.Fatalf("Watch requested Confluence %d more times after ErrAuth (hits %d → %d)",
			hitsLater-hitsAfterStop, hitsAfterStop, hitsLater)
	}

	// Jira must keep ticking on the next interval.
	deadline := time.Now().Add(testNextTickWait)
	for {
		site.mu.Lock()
		n := site.hits
		site.mu.Unlock()
		if n > jiraAtStop {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("Watch returned %v while waiting for another Jira tick; confluence-dead must not stop Jira", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("Jira hits stayed at %d after Confluence ErrAuth, want another tick", n)
		}
		time.Sleep(50 * time.Millisecond)
	}

	stJira, err := db.SyncState(context.Background(), SourceID)
	if err != nil {
		t.Fatal(err)
	}
	if stJira.LastError != nil {
		t.Fatalf("jira last_error = %q after only Confluence died — must stay empty so doctor/sync_health do not blame Jira", *stJira.LastError)
	}
	if !strings.Contains(confErr, "confluence") {
		t.Fatalf("confluence last_error = %q, want the source named so doctor/sql can tell which credential died", confErr)
	}
	if strings.Contains(confErr, "jira:") {
		t.Fatalf("confluence last_error = %q, must not carry the Jira prefix", confErr)
	}

	mu.Lock()
	got := append([]string{}, *logs...)
	mu.Unlock()
	var failed int
	for _, line := range got {
		if strings.Contains(line, "confluence sync failed") {
			failed++
		}
	}
	if failed != 1 {
		t.Fatalf("confluence-sync-failed log lines = %d, want 1 (once, not per tick); logs = %q", failed, got)
	}
	if !strings.Contains(strings.Join(got, "\n"), "credential rejected") {
		t.Fatalf("log did not carry the existing ErrAuth text: %q", got)
	}

	// Manual `gadak sync` (one-shot RunConfluence) after a new token: must
	// resume and clear last_error. Skipping Confluence in Watch is
	// process-lifetime; it must not poison the row.
	stub.mu.Lock()
	stub.status = 0
	stub.mu.Unlock()
	okClient := clientOnStub(t, stub)
	cfg := testConfig()
	cfg.Confluence = &config.ConfluenceConfig{}
	if _, err := RunConfluence(context.Background(), cfg, db.DB, Options{Full: true, ConfluenceClient: okClient}); err != nil {
		t.Fatalf("one-shot RunConfluence after new token: %v", err)
	}
	st, err := db.SyncState(context.Background(), ConfluenceSourceID)
	if err != nil {
		t.Fatal(err)
	}
	if st.LastError != nil {
		t.Fatalf("confluence last_error = %q after a successful RunConfluence — must not be permanent", *st.LastError)
	}

	cancel()
	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Watch returned %v, want context.Canceled", err)
	}
}

// clientOnStub wraps an existing confAuthStub in a new httptest server so a
// one-shot RunConfluence can reuse the same handler (status already flipped).
func clientOnStub(t *testing.T, stub *confAuthStub) *confluence.Client {
	t.Helper()
	srv := httptest.NewServer(stub)
	t.Cleanup(srv.Close)
	c := confluence.New(srv.URL, "user@example.invalid", "secret-token")
	c.Retries, c.Backoff, c.PauseBetween = 1, 0, 0
	return c
}

// TestWatchStopsOnConfluenceForbidden: 403 unwraps to the same ErrAuth as 401.
func TestWatchStopsOnConfluenceForbidden(t *testing.T) {
	_, stub, db, cancel, done, _, _ := watchBoth(t, 0, http.StatusForbidden)
	defer cancel()

	_ = waitConfluenceLastError(t, db, done)
	hitsAfter := stub.count()
	if hitsAfter == 0 {
		t.Fatal("Watch recorded confluence last_error without requesting Confluence")
	}
	time.Sleep(testSleepAfterStop)
	if got := stub.count(); got != hitsAfter {
		t.Fatalf("Watch requested Confluence %d more times after 403 (hits %d → %d)",
			got-hitsAfter, hitsAfter, got)
	}
	select {
	case err := <-done:
		t.Fatalf("Watch returned %v after Confluence 403; Jira mirroring must continue", err)
	default:
	}
	cancel()
	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Watch returned %v, want context.Canceled", err)
	}
}

// TestWatchRetriesConfluenceTransportError: a 500 is not ErrAuth — the loop
// must keep requesting Confluence on the next tick instead of skipping it.
func TestWatchRetriesConfluenceTransportError(t *testing.T) {
	_, stub, db, cancel, done, _, _ := watchBoth(t, 0, http.StatusInternalServerError)
	defer cancel()

	deadline := time.Now().Add(testNextTickWait)
	for {
		if stub.count() >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("Confluence hits = %d after a 500, want >= 2 (retry on the next tick)", stub.count())
		}
		select {
		case err := <-done:
			t.Fatalf("Watch returned %v on a Confluence transport error; only ErrAuth may stop the source", err)
		case <-time.After(50 * time.Millisecond):
		}
	}

	st, err := db.SyncState(context.Background(), ConfluenceSourceID)
	if err != nil {
		t.Fatal(err)
	}
	if st.LastError == nil {
		t.Fatal("confluence last_error empty after a 500 — record() must still land the transport error")
	}
	if strings.Contains(*st.LastError, "credential rejected") {
		t.Fatalf("confluence last_error = %q after a 500, must not be ErrAuth", *st.LastError)
	}

	cancel()
	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Watch returned %v, want context.Canceled", err)
	}
}

// TestWatchLastErrorDistinguishesSource: a user reading last_error (doctor,
// sql, sync_health) must be able to tell which credential died.
// TestRunConfluenceNamedSpaceErrAuthAbortsPass: path ② used to log a Space()
// 401 and continue into SearchPages (a second guaranteed 401). A rejected
// credential must abort the pass on the first 401.
func TestRunConfluenceNamedSpaceErrAuthAbortsPass(t *testing.T) {
	client, stub := startConfAuth(t, http.StatusUnauthorized)
	db := newMirror(t)
	cfg := testConfig()
	cfg.Confluence = &config.ConfluenceConfig{Spaces: []string{"AAA"}}

	_, err := RunConfluence(context.Background(), cfg, db.DB, Options{Full: true, ConfluenceClient: client})
	if !errors.Is(err, confluence.ErrAuth) {
		t.Fatalf("RunConfluence returned %v, want ErrAuth", err)
	}
	if got := stub.count(); got != 1 {
		t.Fatalf("Confluence hits = %d, want 1 (Space GET 401 must not continue to SearchPages)", got)
	}
	st, err := db.SyncState(context.Background(), ConfluenceSourceID)
	if err != nil {
		t.Fatal(err)
	}
	if st.LastError == nil || !strings.Contains(*st.LastError, "credential rejected") {
		t.Fatalf("last_error = %v, want ErrAuth recorded", st.LastError)
	}
}

func TestWatchLastErrorDistinguishesSource(t *testing.T) {
	jiraText := lastErrorFromWatch(t, http.StatusUnauthorized, 0, SourceID)
	confText := lastErrorFromWatch(t, 0, http.StatusUnauthorized, ConfluenceSourceID)
	if jiraText == "" || confText == "" {
		t.Fatalf("missing last_error jira=%q confluence=%q", jiraText, confText)
	}
	if jiraText == confText {
		t.Fatalf("jira and confluence last_error are identical %q — doctor cannot tell which credential died", jiraText)
	}
	if !strings.Contains(jiraText, "jira:") {
		t.Fatalf("jira last_error = %q, want the jira: prefix", jiraText)
	}
	if !strings.Contains(confText, "confluence:") {
		t.Fatalf("confluence last_error = %q, want the confluence: prefix", confText)
	}
	if strings.Contains(jiraText, "confluence:") {
		t.Fatalf("jira last_error leaked the confluence prefix: %q", jiraText)
	}
	if strings.Contains(confText, "jira:") {
		t.Fatalf("confluence last_error leaked the jira prefix: %q", confText)
	}
}

func lastErrorFromWatch(t *testing.T, jiraAuth, confAuth int, sourceID string) string {
	t.Helper()
	if confAuth == 0 {
		// Jira-only: match TestWatchStopsOnErrAuth's shape (no Confluence config).
		site := newSite(t, "en")
		site.authStatus = jiraAuth
		client := site.start()
		db := newMirror(t)
		cfg := testConfig()
		cfg.SyncIntervalSec, cfg.ReconcileIntervalSec = 1, 3600
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan error, 1)
		go func() {
			done <- Watch(ctx, cfg, db.DB, Options{Client: client, Tick: testTick})
		}()
		select {
		case err := <-done:
			if !errors.Is(err, jira.ErrAuth) {
				t.Fatalf("Watch returned %v, want jira.ErrAuth", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Watch did not stop after Jira ErrAuth")
		}
		st, err := db.SyncState(context.Background(), sourceID)
		if err != nil {
			t.Fatal(err)
		}
		if st.LastError == nil {
			t.Fatalf("%s last_error is nil", sourceID)
		}
		return *st.LastError
	}
	_, _, db, cancel, done, _, _ := watchBoth(t, jiraAuth, confAuth)
	defer cancel()
	text := waitConfluenceLastError(t, db, done)
	cancel()
	<-done
	if sourceID != ConfluenceSourceID {
		st, err := db.SyncState(context.Background(), sourceID)
		if err != nil {
			t.Fatal(err)
		}
		if st.LastError == nil {
			return ""
		}
		return *st.LastError
	}
	return text
}

// thirdAuth is a fake third-source sentinel. It is neither jira.ErrAuth nor
// confluence.ErrAuth. The Layer-1 rule must detect it by the interface
// alone so a future source does not need a new branch.
type thirdAuth struct{}

func (thirdAuth) Error() string       { return "third: credential rejected" }
func (thirdAuth) RejectedCredential() {}

func TestIsRejectedCredential(t *testing.T) {
	if !IsRejectedCredential(jira.ErrAuth) {
		t.Fatal("jira.ErrAuth not detected")
	}
	if !IsRejectedCredential(fmtWrap(jira.ErrAuth)) {
		t.Fatal("wrapped jira.ErrAuth not detected")
	}
	if !IsRejectedCredential(confluence.ErrAuth) {
		t.Fatal("confluence.ErrAuth not detected — a third source that only implements RejectedCredential() would also be missed")
	}
	if !IsRejectedCredential(fmtWrap(confluence.ErrAuth)) {
		t.Fatal("wrapped confluence.ErrAuth not detected")
	}
	if !IsRejectedCredential(atlhttp.ErrAuth) {
		t.Fatal("bare atlhttp.ErrAuth not detected")
	}
	if !IsRejectedCredential(fmtWrap(atlhttp.ErrAuth)) {
		t.Fatal("wrapped atlhttp.ErrAuth not detected")
	}
	if !IsRejectedCredential(linear.ErrAuth) {
		t.Fatal("linear.ErrAuth not detected — a future Watch wiring will retry a dead Linear token forever")
	}
	if !IsRejectedCredential(fmtWrap(linear.ErrAuth)) {
		t.Fatal("wrapped linear.ErrAuth not detected")
	}
	if errors.Is(linear.ErrAuth, atlhttp.ErrAuth) {
		t.Fatal("linear.ErrAuth must not unwrap to atlhttp.ErrAuth — that sentinel's identity is Atlassian")
	}
	if !errors.Is(fmtWrap(linear.ErrAuth), linear.ErrAuth) {
		t.Fatal("wrapped linear.ErrAuth must still match errors.Is(..., linear.ErrAuth)")
	}
	if !IsRejectedCredential(thirdAuth{}) {
		t.Fatal("third-source RejectedCredential implementer not detected — the class is still open")
	}
	if !IsRejectedCredential(fmtWrap(thirdAuth{})) {
		t.Fatal("wrapped third-source implementer not detected")
	}
	if IsRejectedCredential(nil) {
		t.Fatal("nil must not be a rejected credential")
	}
	if IsRejectedCredential(errors.New("GET /x: 500 Internal Server Error")) {
		t.Fatal("transport error must not be a rejected credential")
	}
	if IsRejectedCredential(errors.New("confluence: 500 from the source")) {
		t.Fatal("confluence transport text must not be a rejected credential")
	}
	if errors.Is(jira.ErrAuth, confluence.ErrAuth) {
		t.Fatal("jira.ErrAuth must not match confluence.ErrAuth — failJira vs settings Spaces would mis-label")
	}
	if !errors.Is(jira.ErrAuth, atlhttp.ErrAuth) {
		t.Fatal("jira.ErrAuth must unwrap to atlhttp.ErrAuth")
	}
	if !errors.Is(confluence.ErrAuth, atlhttp.ErrAuth) {
		t.Fatal("confluence.ErrAuth must unwrap to atlhttp.ErrAuth")
	}
}

func fmtWrap(err error) error {
	return fmt.Errorf("GET /rest/x: %w (401 Unauthorized)", err)
}

// TestNewAtlhttpClientRejectedWithoutSyncRegistration: a third connector
// that only uses the transport — no package ErrAuth, not in
// defaultWatchSources — must still be IsRejectedCredential on 401.
func TestNewAtlhttpClientRejectedWithoutSyncRegistration(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Client must be authenticated"}`, http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)
	cfg := atlhttp.Config{
		Base:      srv.URL,
		Auth:      "Basic x",
		HTTP:      srv.Client(),
		Retries:   1,
		Backoff:   0,
		ErrPrefix: "third",
	}
	_, _, err := atlhttp.Do(context.Background(), cfg, http.MethodGet, "/rest/ping", nil, false, false)
	if err == nil {
		t.Fatal("Do on 401 must return an error (the shared identity)")
	}
	if !IsRejectedCredential(err) {
		t.Fatalf("IsRejectedCredential(%v) = false — a new atlhttp client must be detected without a sync branch", err)
	}
	if !errors.Is(err, atlhttp.ErrAuth) {
		t.Fatalf("errors.Is(%v, atlhttp.ErrAuth) = false", err)
	}
	if !strings.Contains(err.Error(), "third:") {
		t.Fatalf("err = %q, want third: so last_error names the credential", err.Error())
	}
	if strings.Contains(err.Error(), "jira:") || strings.Contains(err.Error(), "confluence:") {
		t.Fatalf("err = %q, must not borrow another source's prefix", err.Error())
	}
}

func TestEveryWatchSourceHasAuthCoverage(t *testing.T) {
	// Adding a source to Watch without an auth-sentinel test must fail here.
	//
	// Relaxation (GDK-263 wiring): the atlhttp.ErrAuth unwrap assertion is
	// now per-family instead of unconditional. Atlassian sources (Do-based)
	// must unwrap to atlhttp.ErrAuth; Linear must NOT — its sentinel is a
	// deliberate non-unwrap (internal/linear client.go package comment: not
	// an Atlassian host family; it registers via the RejectedCredential
	// duck-type instead). Same split originClientAuthSentinels already
	// carries. FAIL-first: adding the linear watch source turned this test
	// red ("no auth sentinel in this table") before this table grew a row.
	want := map[string]struct {
		err       error
		atlassian bool
	}{
		SourceID:           {jira.ErrAuth, true},
		ConfluenceSourceID: {confluence.ErrAuth, true},
		LinearSourceID:     {linear.ErrAuth, false},
	}
	seen := map[string]bool{}
	for _, src := range defaultWatchSources() {
		row, ok := want[src.id]
		if !ok {
			t.Errorf("watch source %q has no auth sentinel in this table — add one so a forgotten ErrAuth cannot ship", src.id)
			continue
		}
		sent := row.err
		seen[src.id] = true
		if !IsRejectedCredential(sent) {
			t.Errorf("watch source %q sentinel %v is not IsRejectedCredential — the source will retry a dead credential forever", src.id, sent)
		}
		if !IsRejectedCredential(fmtWrap(sent)) {
			t.Errorf("watch source %q wrapped sentinel not detected", src.id)
		}
		if unwraps := errors.Is(sent, atlhttp.ErrAuth); row.atlassian && !unwraps {
			t.Errorf("watch source %q sentinel does not unwrap to atlhttp.ErrAuth — register via atlhttp.Auth so a new client is detected without a sync branch", src.id)
		} else if !row.atlassian && unwraps {
			t.Errorf("watch source %q sentinel unwraps to atlhttp.ErrAuth — that sentinel's identity is Atlassian and this source is not", src.id)
		}
		if !strings.Contains(sent.Error(), src.id+":") {
			t.Errorf("watch source %q sentinel = %q, want %s: so last_error names the credential", src.id, sent, src.id)
		}
	}
	for id := range want {
		if !seen[id] {
			t.Errorf("auth table lists %q but defaultWatchSources does not", id)
		}
	}
}

// originClientAuthSentinels is every origin-client var ErrAuth in this
// repository, keyed by the internal/ directory that declares it.
//
// TestEveryWatchSourceHasAuthCoverage cannot hold this list: it also
// requires unwrap to atlhttp.ErrAuth, which is correct for Do-based
// Atlassian clients and forbidden for Linear (package comment on
// internal/linear: not an Atlassian host family). A hand list of values
// is unavoidable — you cannot construct linear.ErrAuth without importing
// linear — so originClientErrAuthDirs walks internal/ and fails if a new
// var ErrAuth appears without a row here.
var originClientAuthSentinels = []struct {
	dir       string
	err       error
	atlassian bool
}{
	{dir: "atlhttp", err: atlhttp.ErrAuth, atlassian: true},
	{dir: "jira", err: jira.ErrAuth, atlassian: true},
	{dir: "confluence", err: confluence.ErrAuth, atlassian: true},
	{dir: "linear", err: linear.ErrAuth, atlassian: false},
}

func TestEveryOriginClientAuthSentinelIsRejectedCredential(t *testing.T) {
	known := map[string]bool{}
	for _, row := range originClientAuthSentinels {
		known[row.dir] = true
		if row.err == nil {
			t.Errorf("internal/%s ErrAuth is nil", row.dir)
			continue
		}
		if !IsRejectedCredential(row.err) {
			t.Errorf("internal/%s ErrAuth %v is not IsRejectedCredential — a dead credential will be retried forever", row.dir, row.err)
		}
		if !IsRejectedCredential(fmtWrap(row.err)) {
			t.Errorf("wrapped internal/%s ErrAuth not detected", row.dir)
		}
		unwraps := errors.Is(row.err, atlhttp.ErrAuth)
		if row.atlassian && !unwraps {
			t.Errorf("internal/%s ErrAuth must unwrap to atlhttp.ErrAuth", row.dir)
		}
		if !row.atlassian && unwraps {
			t.Errorf("internal/%s ErrAuth must not unwrap to atlhttp.ErrAuth — that sentinel's identity is Atlassian", row.dir)
		}
	}
	found := originClientErrAuthDirs(t)
	for dir := range found {
		if !known[dir] {
			t.Errorf("internal/%s declares var ErrAuth but is not in originClientAuthSentinels — add it so a wrongly-built sentinel goes red before anyone wires it", dir)
		}
	}
	for dir := range known {
		if !found[dir] {
			t.Errorf("originClientAuthSentinels lists %s but no var ErrAuth was found under internal/%s", dir, dir)
		}
	}
}

func originClientErrAuthDirs(t *testing.T) map[string]bool {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	internal := filepath.Join(root, "internal")
	found := map[string]bool{}
	err := filepath.WalkDir(internal, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "vendor", "node_modules", "dist", "testdata", "scratch":
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !srcDeclaresVarErrAuth(string(data)) {
			return nil
		}
		rel, err := filepath.Rel(internal, filepath.Dir(path))
		if err != nil {
			return err
		}
		found[filepath.ToSlash(rel)] = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

func srcDeclaresVarErrAuth(src string) bool {
	for _, line := range strings.Split(src, "\n") {
		s := strings.TrimSpace(line)
		if s == "" || strings.HasPrefix(s, "//") {
			continue
		}
		if strings.HasPrefix(s, "var ErrAuth ") || strings.HasPrefix(s, "var ErrAuth=") {
			return true
		}
		// var ( ErrAuth = ... ) block form
		if strings.HasPrefix(s, "ErrAuth =") || strings.HasPrefix(s, "ErrAuth=") {
			return true
		}
	}
	return false
}

// TestApplyWatchErrLinearSentinel: Linear is not a Watch source yet, but the
// moment it is appended to defaultWatchSources the same applyWatchErr path
// must already treat its sentinel as a dead credential. Otherwise Watch
// retries the token forever and last_error stays empty.
func TestApplyWatchErrLinearSentinel(t *testing.T) {
	db := newMirror(t)
	ctx := context.Background()
	dead := map[string]bool{}
	src := watchSource{id: "linear", failLog: "linear sync failed: %v", fatal: false}
	if err := applyWatchErr(ctx, nil, db.DB, src, linear.ErrAuth, func(string, ...any) {}, dead); err != nil {
		t.Fatalf("non-fatal linear source returned %v, want continue", err)
	}
	if !dead["linear"] {
		t.Fatal("linear.ErrAuth not marked dead — Watch would retry a dead Linear token forever")
	}
	st, err := db.SyncState(ctx, "linear")
	if err != nil {
		t.Fatal(err)
	}
	if st.LastError == nil || !strings.Contains(*st.LastError, "linear:") {
		t.Fatalf("last_error = %v, want linear: so doctor/sql can name the source", st.LastError)
	}
}

func TestApplyWatchErrThirdSource(t *testing.T) {
	db := newMirror(t)
	ctx := context.Background()

	// happy: non-fatal third source is skipped, last_error recorded, Watch continues
	dead := map[string]bool{}
	src := watchSource{id: "third", failLog: "third sync failed: %v", fatal: false}
	if err := applyWatchErr(ctx, nil, db.DB, src, thirdAuth{}, func(string, ...any) {}, dead); err != nil {
		t.Fatalf("non-fatal third source returned %v, want continue (Jira must keep running)", err)
	}
	if !dead["third"] {
		t.Fatal("non-fatal third source not marked dead — next tick would retry the 401")
	}
	st, err := db.SyncState(ctx, "third")
	if err != nil {
		t.Fatal(err)
	}
	if st.LastError == nil || !strings.Contains(*st.LastError, "third:") {
		t.Fatalf("last_error = %v, want third: so doctor/sql can name the source", st.LastError)
	}

	// boundary: fatal third source ends Watch
	fatal := watchSource{id: "third-fatal", failLog: "third sync failed: %v", fatal: true}
	err = applyWatchErr(ctx, nil, db.DB, fatal, thirdAuth{}, func(string, ...any) {}, map[string]bool{})
	if err == nil {
		t.Fatal("fatal third source returned nil, want the auth error so Watch stops")
	}
	if !errors.As(err, new(thirdAuth)) && !errors.Is(err, thirdAuth{}) {
		// thirdAuth is a value type; errors.Is uses ==
		if err.Error() != (thirdAuth{}).Error() {
			t.Fatalf("fatal third source returned %v, want thirdAuth", err)
		}
	}

	// boundary: transport on a third source must not mark it dead
	dead2 := map[string]bool{}
	trans := watchSource{id: "third", failLog: "third sync failed: %v", fatal: false}
	if err := applyWatchErr(ctx, nil, db.DB, trans, errors.New("500 boom"), func(string, ...any) {}, dead2); err != nil {
		t.Fatalf("transport on third source returned %v, want continue", err)
	}
	if dead2["third"] {
		t.Fatal("transport error marked the source dead — only ErrAuth may skip later ticks")
	}
}

// TestWatchConfluenceResumesAfterCredentialReload: skip is "this credential
// is dead", not "this source is cursed". A settings token change (Reload)
// must retry Confluence; otherwise a user who rotates the token in the
// running process never gets the wiki back without a restart.
func TestWatchConfluenceResumesAfterCredentialReload(t *testing.T) {
	site := newSite(t, "en")
	jclient := site.start()
	cclient, stub := startConfAuth(t, http.StatusUnauthorized)
	db := newMirror(t)
	cfg := testConfig()
	cfg.SyncIntervalSec, cfg.ReconcileIntervalSec = 1, 3600
	cfg.Confluence = &config.ConfluenceConfig{}

	var mu sync.Mutex
	token := "old-token"
	reload := func() (*config.Config, error) {
		mu.Lock()
		defer mu.Unlock()
		c := *cfg
		c.Token = token
		c.Confluence = &config.ConfluenceConfig{}
		c.SyncIntervalSec, c.ReconcileIntervalSec = 1, 3600
		return &c, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- Watch(ctx, cfg, db.DB, Options{
			Client: jclient, ConfluenceClient: cclient, Reload: reload, Tick: testTick,
		})
	}()

	_ = waitConfluenceLastError(t, db, done)
	hitsDead := stub.count()
	if hitsDead == 0 {
		t.Fatal("no Confluence request before reload")
	}
	time.Sleep(testSleepAfterStop)
	if stub.count() != hitsDead {
		t.Fatalf("Confluence hits moved %d → %d before the token changed", hitsDead, stub.count())
	}

	stub.mu.Lock()
	stub.status = 0
	stub.mu.Unlock()
	mu.Lock()
	token = "new-token"
	mu.Unlock()

	deadline := time.Now().Add(testNextTickWait)
	for stub.count() <= hitsDead {
		if time.Now().After(deadline) {
			t.Fatalf("Confluence hits stayed at %d after token reload — skip must be per-credential, not process-lifetime", hitsDead)
		}
		select {
		case err := <-done:
			t.Fatalf("Watch returned %v while waiting for Confluence to resume", err)
		case <-time.After(50 * time.Millisecond):
		}
	}
	cancel()
	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Watch returned %v, want context.Canceled", err)
	}
}

func TestSourceLastErrorsAnswersWhichCredential(t *testing.T) {
	_, _, db, cancel, done, _, _ := watchBoth(t, 0, http.StatusUnauthorized)
	defer cancel()
	_ = waitConfluenceLastError(t, db, done)

	got, err := sourceLastErrors(context.Background(), db.DB)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got[SourceID]; ok {
		t.Fatalf("sourceLastErrors includes jira = %q; only Confluence died", got[SourceID])
	}
	if !strings.Contains(got[ConfluenceSourceID], "confluence:") || !strings.Contains(got[ConfluenceSourceID], "credential rejected") {
		t.Fatalf("sourceLastErrors = %v, want confluence credential rejected", got)
	}
	cancel()
	<-done
}
