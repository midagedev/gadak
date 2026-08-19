package workspace

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
	"github.com/midagedev/gadak/internal/uifocus"
)

// seedProfile writes config.json and an empty migrated gadak.db under GADAK_HOME
// for the named profile ("" = default root).
func seedProfile(t *testing.T, name string, cfg *config.Config) {
	t.Helper()
	dir, err := config.DirFor(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.LoadFor(name)
	if err != nil {
		t.Fatal(err)
	}
	loaded.Site = cfg.Site
	loaded.Email = cfg.Email
	loaded.Token = cfg.Token
	loaded.Projects = cfg.Projects
	if err := loaded.Save(); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dir, "gadak.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	key := "AAA-1"
	if name != "" && name != "default" {
		key = "BBB-1"
	}
	if err := db.UpsertSource(context.Background(), store.Source{ID: "jira", Kind: "jira", BaseURL: cfg.Site}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertIssues(context.Background(), store.Batch{
		Categories: map[string]string{"1": "new"},
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "jira:" + key, SourceID: "jira", ExternalID: key, Key: key,
				Title: "fixture " + key, CreatedAt: "2026-07-01T00:00:00.000Z", UpdatedAt: "2026-07-01T00:00:00.000Z",
			},
			Issue: store.Issue{ProjectKey: strings.Split(key, "-")[0], Status: "To Do", StatusID: "1", StatusCategory: "new"},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
}

func setupHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Cleanup(func() { config.SetProfile("") })
	config.SetProfile("")
}

func TestHandlerRoutesToProfileAPI(t *testing.T) {
	setupHome(t)
	seedProfile(t, "", &config.Config{
		Site: "http://127.0.0.1:1", Email: "a@example.invalid", Token: "test-token", Projects: []string{"AAA"},
	})
	seedProfile(t, "work", &config.Config{
		Site: "http://127.0.0.1:1", Email: "b@example.invalid", Token: "test-token", Projects: []string{"BBB"},
	})

	reg := New()
	t.Cleanup(func() { reg.Close() })
	spa := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html>spa</html>"))
	})
	h := reg.Handler(spa, "test-ver")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/w/work/api/v1/issues/bootstrap/", nil)
	req.Host = "127.0.0.1:7777"
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var boot struct {
		Issues []struct {
			IssueKey string `json:"issue_key"`
		} `json:"issues"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &boot); err != nil {
		t.Fatalf("decode: %v body %s", err, rec.Body.String())
	}
	if len(boot.Issues) != 1 || boot.Issues[0].IssueKey != "BBB-1" {
		t.Fatalf("want BBB-1 from work mirror, got %+v", boot.Issues)
	}
}

func TestHandlerUIFocusReadsProfileFile(t *testing.T) {
	setupHome(t)
	seedProfile(t, "", &config.Config{
		Site: "http://127.0.0.1:1", Email: "a@example.invalid", Token: "test-token", Projects: []string{"AAA"},
	})
	seedProfile(t, "work", &config.Config{
		Site: "http://127.0.0.1:1", Email: "b@example.invalid", Token: "test-token", Projects: []string{"BBB"},
	})
	if err := uifocus.WriteFor("work", "ks=BBB-1"); err != nil {
		t.Fatal(err)
	}
	if err := uifocus.WriteFor("", "ks=AAA-1"); err != nil {
		t.Fatal(err)
	}

	reg := New()
	t.Cleanup(func() { reg.Close() })
	h := reg.Handler(http.NotFoundHandler(), "test-ver")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/w/work/api/v1/issues/ui-focus/", nil)
	req.Host = "127.0.0.1:7777"
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Hash string `json:"hash"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Hash != "ks=BBB-1" {
		t.Fatalf("workspace handler consumed %q, want ks=BBB-1", body.Hash)
	}
	hash, ok, err := uifocus.TakeFor("")
	if err != nil || !ok || hash != "ks=AAA-1" {
		t.Fatalf("default file should be untouched: %q ok=%v err=%v", hash, ok, err)
	}
}

func TestHandlerBadName404(t *testing.T) {
	setupHome(t)
	seedProfile(t, "work", &config.Config{
		Site: "http://127.0.0.1:1", Email: "b@example.invalid", Token: "test-token",
	})

	reg := New()
	t.Cleanup(func() { reg.Close() })
	h := reg.Handler(http.NotFoundHandler(), "test-ver")

	for _, path := range []string{
		"/w/../etc/passwd",
		"/w/a%2Fb",
		"/w/no-such-profile/api/v1/issues/bootstrap/",
		"/w/bad..name/config.json",
	} {
		rec := httptest.NewRecorder()
		// httptest.NewRequest parses the URL; for encoded slash use a raw URL path.
		req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1"+path, nil)
		// Preserve the path segment as written for path-escape cases.
		if path == "/w/a%2Fb" {
			req.URL.Path = "/w/a%2Fb"
			req.URL.RawPath = "/w/a%2Fb"
		}
		if path == "/w/../etc/passwd" {
			// After URL cleaning this may become /etc/passwd; the handler must
			// still not open a workspace. Force the path the router would see.
			req.URL.Path = "/w/../etc/passwd"
		}
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s → %d, want 404", path, rec.Code)
		}
	}
}

func TestListHandlerNoSecrets(t *testing.T) {
	setupHome(t)
	seedProfile(t, "", &config.Config{
		Site: "http://127.0.0.1:1", Email: "user@example.invalid", Token: "test-token-aaa", Projects: []string{"AAA"},
	})
	seedProfile(t, "work", &config.Config{
		Site: "http://127.0.0.1:1", Email: "other@example.invalid", Token: "test-token-bbb", Projects: []string{"BBB"},
	})

	rec := httptest.NewRecorder()
	ListHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/workspaces", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	// Spec: response bytes must not contain the substrings "token" or "email"
	// (case-sensitive check for the forbidden credential field names/values).
	for _, banned := range []string{"token", "email", "Token", "Email", "test-token-aaa", "test-token-bbb", "user@example.invalid", "other@example.invalid"} {
		if strings.Contains(body, banned) {
			t.Fatalf("ListHandler response contains %q: %s", banned, body)
		}
	}
}

func TestListHandlerPrimaryFirstActive(t *testing.T) {
	setupHome(t)
	seedProfile(t, "", &config.Config{
		Site: "http://127.0.0.1:1", Email: "a@example.invalid", Token: "test-token", Projects: []string{"AAA"},
	})
	seedProfile(t, "work", &config.Config{
		Site: "http://127.0.0.1:1", Email: "b@example.invalid", Token: "test-token", Projects: []string{"BBB"},
	})

	rec := httptest.NewRecorder()
	ListHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/workspaces", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var doc struct {
		Workspaces []struct {
			Name   string `json:"name"`
			Active bool   `json:"active"`
		} `json:"workspaces"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Workspaces) < 1 {
		t.Fatalf("empty workspaces: %+v", doc.Workspaces)
	}
	if doc.Workspaces[0].Name != "default" || !doc.Workspaces[0].Active {
		t.Fatalf("first entry want default active, got %+v", doc.Workspaces[0])
	}
	// Named profiles after primary are not active.
	for i, w := range doc.Workspaces[1:] {
		if w.Active {
			t.Fatalf("workspace[%d] %q should not be active", i+1, w.Name)
		}
	}
}

func TestWatchAllSkipsPrimary(t *testing.T) {
	setupHome(t)
	// Primary is named "work" for this process.
	config.SetProfile("work")
	seedProfile(t, "work", &config.Config{
		Site: "http://127.0.0.1:1", Email: "a@example.invalid", Token: "test-token",
	})
	seedProfile(t, "side", &config.Config{
		Site: "http://127.0.0.1:1", Email: "b@example.invalid", Token: "test-token",
	})

	reg := New()
	t.Cleanup(func() { reg.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	started := reg.WatchAll(ctx, "work", func(string) {})
	for _, name := range started {
		if name == "work" || name == "default" || name == "" {
			t.Fatalf("WatchAll returned primary %q in %v", name, started)
		}
	}
	// side should be started.
	found := false
	for _, name := range started {
		if name == "side" {
			found = true
		}
	}
	if !found {
		t.Fatalf("want side in started, got %v", started)
	}
	cancel()
}

func TestWatchAllSkipsNoCredential(t *testing.T) {
	setupHome(t)
	seedProfile(t, "cred", &config.Config{
		Site: "http://127.0.0.1:1", Email: "a@example.invalid", Token: "test-token",
	})
	// Profile with no credential (empty token).
	seedProfile(t, "nocred", &config.Config{
		Site: "http://127.0.0.1:1", Email: "b@example.invalid", Token: "",
	})

	reg := New()
	t.Cleanup(func() { reg.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	started := reg.WatchAll(ctx, "", func(string) {})
	for _, name := range started {
		if name == "nocred" {
			t.Fatalf("WatchAll started no-credential profile: %v", started)
		}
	}
	found := false
	for _, name := range started {
		if name == "cred" {
			found = true
		}
	}
	if !found {
		t.Fatalf("want cred in started, got %v", started)
	}
	cancel()
}

func TestWatchAllStopsOnCancel(t *testing.T) {
	setupHome(t)
	seedProfile(t, "loop", &config.Config{
		// Connection refused is fast; requests use ctx so cancel aborts quickly.
		Site: "http://127.0.0.1:1", Email: "a@example.invalid", Token: "test-token",
	})

	reg := New()
	t.Cleanup(func() { reg.Close() })

	baseline := runtime.NumGoroutine()
	ctx, cancel := context.WithCancel(context.Background())
	started := reg.WatchAll(ctx, "", func(string) {})
	if len(started) != 1 || started[0] != "loop" {
		cancel()
		t.Fatalf("started %v", started)
	}
	// Let the loop enter Watch / first Run.
	time.Sleep(30 * time.Millisecond)
	cancel()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= baseline+2 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("goroutine leak after cancel: now %d baseline %d", runtime.NumGoroutine(), baseline)
}

func TestSameProfile(t *testing.T) {
	if !sameProfile("", "default") || !sameProfile("default", "") {
		t.Fatal("empty and default must match")
	}
	if !sameProfile("work", "work") {
		t.Fatal("same name")
	}
	if sameProfile("work", "side") {
		t.Fatal("different names")
	}
}

func watchingHas(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}

// D8: a profile that gains a credential after WatchAll must get a loop when
// EnsureWatches runs (the single "credential appeared" scan). Today WatchAll
// is boot-only, so this fails until the rescan owner exists.
func TestWatchStartsWhenCredentialAppearsAfterBoot(t *testing.T) {
	setupHome(t)
	seedProfile(t, "late", &config.Config{
		Site: "http://127.0.0.1:1", Email: "a@example.invalid", Token: "",
	})

	reg := New()
	t.Cleanup(func() { reg.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	started := reg.WatchAll(ctx, "", func(string) {})
	if watchingHas(started, "late") {
		t.Fatalf("WatchAll started no-credential profile: %v", started)
	}

	seedProfile(t, "late", &config.Config{
		Site: "http://127.0.0.1:1", Email: "a@example.invalid", Token: "test-token",
	})
	got := reg.EnsureWatches()
	if !watchingHas(got, "late") {
		t.Fatalf("EnsureWatches after credential appeared: started %v, watching %v", got, reg.Watching())
	}
	if !watchingHas(reg.Watching(), "late") {
		t.Fatalf("Watching() missing late after EnsureWatches: %v", reg.Watching())
	}
	// Second scan must not start another loop.
	if again := reg.EnsureWatches(); len(again) != 0 {
		t.Fatalf("second EnsureWatches started %v", again)
	}
}

// D8 onboarding path: PUT onboarding/connect/ on a workspace must start that
// profile's Watch (SetSyncStarter on the workspace handler).
func TestWorkspaceConnectStartsWatch(t *testing.T) {
	setupHome(t)
	seedProfile(t, "late", &config.Config{
		Site: "", Email: "", Token: "",
	})

	jira := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/myself") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"accountId":"acc-1","displayName":"Pat","emailAddress":"a@example.invalid"}`))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(jira.Close)

	reg := New()
	t.Cleanup(func() { reg.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	_ = reg.WatchAll(ctx, "", func(string) {})

	h := reg.Handler(http.NotFoundHandler(), "test-ver")
	body := `{"site":"` + jira.URL + `","jira_email":"a@example.invalid","api_token":"tok"}`
	req := httptest.NewRequest(http.MethodPut, "/w/late/api/v1/issues/onboarding/connect/", strings.NewReader(body))
	req.Host = "127.0.0.1:7777"
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("connect → %d %s", rec.Code, rec.Body.String())
	}
	if !watchingHas(reg.Watching(), "late") {
		t.Fatalf("workspace connect did not start Watch: watching %v", reg.Watching())
	}
}

// TestCloseStopsWorkspaceSyncBeforeTheMirror is the production half of
// GDK-270. The server learned to own its background sync, but a Registry that
// closed only e.DB would still leave a workspace's job writing into a mirror
// that had just been closed — the same leak, one layer up. Driving a real
// scope-changing settings PUT is what starts that job; the profile's Site is a
// dead loopback port, so the sync fails fast without leaving this machine.
func TestCloseStopsWorkspaceSyncBeforeTheMirror(t *testing.T) {
	setupHome(t)
	seedProfile(t, "", &config.Config{
		Site: "http://127.0.0.1:1", Email: "a@example.invalid", Token: "test-token", Projects: []string{"AAA"},
	})
	seedProfile(t, "work", &config.Config{
		Site: "http://127.0.0.1:1", Email: "b@example.invalid", Token: "test-token", Projects: []string{"BBB"},
	})

	reg := New()
	e, err := reg.Get("work")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	db := e.DB

	spa := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := reg.Handler(spa, "test-ver")
	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"projects":["BBB","CCC"],"staleThresholdHours":72}`)
	req := httptest.NewRequest(http.MethodPut, "/w/work/api/v1/issues/settings/", body)
	req.Host = "127.0.0.1:7777"
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT settings → %d %s", rec.Code, rec.Body.String())
	}

	reg.Close()

	if n := db.PoolStats().InUse; n != 0 {
		t.Fatalf("%d connection(s) still checked out after Registry.Close — the workspace's sync outlived its mirror (GDK-270)", n)
	}
}
