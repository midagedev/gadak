package origin_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/server"
	"github.com/midagedev/gadak/internal/store"
)

func standaloneHome(t *testing.T) (*config.Config, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() {
		_ = origin.Close()
		origin.ResetInProcess()
		config.SetProfile("")
	})
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Kind = config.KindStandalone
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	cfg, err = config.Load()
	if err != nil {
		t.Fatal(err)
	}
	return cfg, home
}

func searchKey(t *testing.T, c *jira.Client, key string) bool {
	t.Helper()
	found := false
	err := c.Search(context.Background(), `key = "`+key+`"`, []string{"summary"}, false, func(issues []jira.Issue) error {
		for _, iss := range issues {
			if iss.Key == key {
				found = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	return found
}

// liveStandaloneServe is a real server.Handler + standalone issuetap
// session advertised at serve-origin.json. The session is evicted from
// origin.live so Client() cannot reuse it in-process (the CLI process
// shape). Passthrough is pinned on the handler.
func liveStandaloneServe(t *testing.T) (cfg *config.Config, serveClient *jira.Client, h *server.Handler, ts *httptest.Server) {
	t.Helper()
	cfg, _ = standaloneHome(t)
	origin.SetInProcess(cfg, true)

	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "mirror.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	serveClient, err = origin.Client(cfg)
	if err != nil {
		t.Fatal(err)
	}
	hOrigin, err := origin.StandaloneHandler(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if c, ok := hOrigin.(interface{ Close() error }); ok {
			_ = c.Close()
		}
	})

	h = server.New(db, cfg)
	h.BindOriginHandler(hOrigin)
	t.Cleanup(func() { _ = h.Close() })

	ts = httptest.NewServer(h)
	t.Cleanup(ts.Close)

	origin.ForgetLive()
	origin.SetInProcess(cfg, false)

	if err := origin.WriteAdvertise(cfg.Directory(), ts.Listener.Addr().String()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = origin.RemoveAdvertise(cfg.Directory()) })
	return cfg, serveClient, h, ts
}

func TestClientRoutesToLiveServe(t *testing.T) {
	cfg, serveClient, _, ts := liveStandaloneServe(t)

	before := origin.SessionsConstructed()
	c, err := origin.Client(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if origin.TransportIsEmbedded(c.HTTP.Transport) {
		t.Fatal("Client constructed an embedded session; want a transport aimed at the live serve")
	}
	if !origin.TransportIsServe(c.HTTP.Transport) {
		t.Fatalf("Transport type %T, want serve passthrough", c.HTTP.Transport)
	}
	if got := origin.SessionsConstructed(); got != before {
		t.Fatalf("SessionsConstructed %d → %d; routed Client must not constructStandalone", before, got)
	}
	if c.BaseURL() != "" {
		t.Fatalf("BaseURL = %q, want empty", c.BaseURL())
	}

	ctx := context.Background()
	key, err := c.CreateIssue(ctx, map[string]any{
		"project":   map[string]any{"key": origin.DefaultProjectKey},
		"summary":   "routed through live serve",
		"issuetype": map[string]any{"name": "Task"},
	})
	if err != nil {
		t.Fatalf("CreateIssue via routed client: %v", err)
	}
	if !searchKey(t, serveClient, key) {
		t.Fatalf("serve session cannot see %s — create did not land on the pinned origin", key)
	}

	req, err := http.NewRequest(http.MethodGet, ts.URL+origin.RESTPrefix+"/rest/api/3/issue/"+key, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("standalone:standalone")))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET origin issue: %d %s", res.StatusCode, body)
	}
	var got struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode issue: %v\n%s", err, body)
	}
	if got.Key != key {
		t.Fatalf("origin issue key %q, want %q", got.Key, key)
	}
}

func TestClientFallsBackWhenProbeFails(t *testing.T) {
	cfg, home := standaloneHome(t)
	origin.SetInProcess(cfg, false)

	if err := origin.WriteAdvertise(home, "127.0.0.1:1"); err != nil {
		t.Fatal(err)
	}
	before := origin.SessionsConstructed()
	c, err := origin.Client(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !origin.TransportIsEmbedded(c.HTTP.Transport) {
		t.Fatalf("Transport type %T, want embedded fallback", c.HTTP.Transport)
	}
	if got := origin.SessionsConstructed(); got != before+1 {
		t.Fatalf("SessionsConstructed %d → %d, want +1 embedded construct", before, got)
	}
}

// GDK-677 (GitHub #52): the routing decision compared the probe answer
// against the process-global profile, so a Config loaded for a mounted
// workspace — whose stale advertise pointed at the primary's serve — was
// approved for routing and its origin calls landed on the PRIMARY profile's
// origin route (not_found). The comparison now uses the profile the Config
// belongs to: the probe answers the primary's name, the cfg says the
// mount's, mismatch, no routing — the mount constructs its own embedded
// session (its persist has no live owner) and create metadata works.
func TestMountedWorkspaceIgnoresPrimaryOwnersAdvertise(t *testing.T) {
	_, _, _, ts := liveStandaloneServe(t)

	jt, err := config.LoadFor("jt")
	if err != nil {
		t.Fatal(err)
	}
	jt.Kind = config.KindStandalone
	if err := jt.Save(); err != nil {
		t.Fatal(err)
	}
	jt, err = config.LoadFor("jt")
	if err != nil {
		t.Fatal(err)
	}
	// The reporter's residue: serving jt as primary once (their workaround)
	// leaves jt's advertise pointing at the same default port the primary
	// serve now listens on.
	if err := origin.WriteAdvertise(jt.Directory(), ts.Listener.Addr().String()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = origin.RemoveAdvertise(jt.Directory()) })

	before := origin.SessionsConstructed()
	c, err := origin.Client(jt)
	if err != nil {
		t.Fatal(err)
	}
	if origin.TransportIsServe(c.HTTP.Transport) {
		t.Fatal("mounted workspace routed to the primary's serve — its origin calls land on the wrong profile's route (GDK-677)")
	}
	if !origin.TransportIsEmbedded(c.HTTP.Transport) {
		t.Fatalf("Transport type %T, want the mount's own embedded session", c.HTTP.Transport)
	}
	if got := origin.SessionsConstructed(); got != before+1 {
		t.Fatalf("SessionsConstructed %d → %d, want +1 (jt's own graph)", before, got)
	}
}

// Pre-GDK-343 this fell back to a second embedded graph over the live
// owner's persist (the F5 double-write hazard) and the persist lock made
// that fallback fail loud instead: the owner is alive, so busy.
//
// Revised 2026-08-23 (GDK-677): the routing decision now compares the probe
// answer against the profile the Config BELONGS TO, not the process-global
// one. This cfg is the root profile and the live serve owns the root
// profile, so the mismatch that used to force the busy refusal is gone —
// the client routes to the legitimate owner. The double-write hazard stays
// closed the stronger way: no embedded graph is constructed at all.
func TestClientRoutesToOwnerOnGlobalProfileMismatch(t *testing.T) {
	cfg, _, _, _ := liveStandaloneServe(t)
	config.SetProfile("other")
	t.Cleanup(func() { config.SetProfile("") })

	before := origin.SessionsConstructed()
	c, err := origin.Client(cfg)
	if err != nil {
		t.Fatalf("cfg with a live owner: err = %v, want a routed client", err)
	}
	if !origin.TransportIsServe(c.HTTP.Transport) {
		t.Fatalf("Transport type %T, want the live owner's serve passthrough", c.HTTP.Transport)
	}
	if got := origin.SessionsConstructed(); got != before {
		t.Fatalf("SessionsConstructed %d → %d; routing must not construct a second graph over the live persist", before, got)
	}
}

// GDK-484: the busy assertion above flaked on CI because ForgetLive used
// to drop the only reference to the live owner's session — the persist
// lock is a flock on an os.File, and a GC in the window between ForgetLive
// and Client ran the file's finalizer, closed the fd, and released the
// lock. Forcing GC here reproduced err = <nil> five out of five times
// before ForgetLive learned to keep forgotten sessions reachable.
func TestForgetLiveKeepsPersistLockAcrossGC(t *testing.T) {
	cfg, _, _, _ := liveStandaloneServe(t)
	// Force the embedded path by removing the advertise file. This test used
	// to force it with a global-profile mismatch, but since GDK-677 a
	// mismatch with the cfg's own profile no longer exists here (the cfg
	// carries its identity), so routing would succeed and never touch the
	// lock this test exists to check.
	if err := origin.RemoveAdvertise(cfg.Directory()); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 3; i++ {
		runtime.GC()
	}

	_, err := origin.Client(cfg)
	if !errors.Is(err, origin.ErrWorkspaceBusy) {
		t.Fatalf("after GC: err = %v, want ErrWorkspaceBusy — the forgotten owner's lock was released", err)
	}
}

// In-process (this process is the serve owner) must never route to its own
// advertise file. With the live owner's lock held by the still-open forgotten
// session, the proof is the busy error: routing would have returned a serve
// transport, and embedding would have opened a second graph (GDK-343).
func TestSetInProcessSkipsRouting(t *testing.T) {
	cfg, _, _, _ := liveStandaloneServe(t)
	origin.SetInProcess(cfg, true)
	t.Cleanup(func() { origin.SetInProcess(cfg, false) })

	_, err := origin.Client(cfg)
	if !errors.Is(err, origin.ErrWorkspaceBusy) {
		t.Fatalf("in-process Client err = %v, want ErrWorkspaceBusy (routing to self is the bug this guards)", err)
	}
}

func TestSetInProcessDoesNotSkipOtherWorkspaceRouting(t *testing.T) {
	cfgA, _, _, _ := liveStandaloneServe(t)

	cfgB, err := config.LoadFor("other")
	if err != nil {
		t.Fatal(err)
	}
	cfgB.Kind = config.KindStandalone
	if err := cfgB.Save(); err != nil {
		t.Fatal(err)
	}
	cfgB, err = config.LoadFor("other")
	if err != nil {
		t.Fatal(err)
	}

	origin.SetInProcess(cfgB, true)
	t.Cleanup(func() { origin.SetInProcess(cfgB, false) })
	hOrigin, err := origin.StandaloneHandler(cfgB)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "mirror.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	h := server.NewWorkspace(db, cfgB, nil, "other")
	h.BindOriginHandler(hOrigin)
	t.Cleanup(func() { _ = h.Close() })
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)

	origin.ForgetLive()
	origin.SetInProcess(cfgB, false)
	if err := origin.WriteAdvertise(cfgB.Directory(), ts.Listener.Addr().String()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = origin.RemoveAdvertise(cfgB.Directory()) })

	origin.SetInProcess(cfgA, true)
	t.Cleanup(func() { origin.SetInProcess(cfgA, false) })
	if _, err := origin.Client(cfgA); !errors.Is(err, origin.ErrWorkspaceBusy) {
		t.Fatalf("owned workspace Client err = %v, want ErrWorkspaceBusy (routing to self is the bug this guards)", err)
	}

	c, err := origin.Client(cfgB)
	if err != nil {
		t.Fatalf("other workspace Client: %v", err)
	}
	if !origin.TransportIsServe(c.HTTP.Transport) {
		t.Fatalf("other workspace transport %T, want serve passthrough", c.HTTP.Transport)
	}
	if _, err := c.CreateIssue(context.Background(), map[string]any{
		"project":   map[string]any{"key": origin.DefaultProjectKey},
		"summary":   "in-process key isolation",
		"issuetype": map[string]any{"name": "Task"},
	}); err != nil {
		t.Fatalf("other workspace routed CreateIssue: %v", err)
	}
}

func TestOwnerStatusEmbeddedAndServe(t *testing.T) {
	cfg, home := standaloneHome(t)
	if got := origin.OwnerStatus(cfg); got != "embedded (no live serve)" {
		t.Fatalf("no file: %q", got)
	}
	if err := origin.WriteAdvertise(home, "127.0.0.1:1"); err != nil {
		t.Fatal(err)
	}
	if got := origin.OwnerStatus(cfg); got != "embedded (no live serve)" {
		t.Fatalf("dead probe: %q", got)
	}

	cfg2, _, _, ts := liveStandaloneServe(t)
	got := origin.OwnerStatus(cfg2)
	if !strings.HasPrefix(got, "serve pid=") || !strings.Contains(got, "addr=") {
		t.Fatalf("live serve OwnerStatus %q", got)
	}
	if !strings.Contains(got, ts.Listener.Addr().String()) {
		t.Fatalf("OwnerStatus %q missing %s", got, ts.Listener.Addr().String())
	}
}

func TestWriteRemoveAdvertise(t *testing.T) {
	dir := t.TempDir()
	if err := origin.WriteAdvertise(dir, "127.0.0.1:7998"); err != nil {
		t.Fatal(err)
	}
	p := origin.AdvertisePath(dir)
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var adv origin.Advertise
	if err := json.Unmarshal(raw, &adv); err != nil {
		t.Fatal(err)
	}
	if adv.Addr != "127.0.0.1:7998" || adv.PID != os.Getpid() || adv.StartedAt == "" {
		t.Fatalf("advertise = %+v", adv)
	}
	if err := origin.RemoveAdvertise(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("file still there: %v", err)
	}
	if err := origin.RemoveAdvertise(dir); err != nil {
		t.Fatalf("remove missing: %v", err)
	}
}

func TestConnectedOwnerStatusEmpty(t *testing.T) {
	if got := origin.OwnerStatus(&config.Config{}); got != "" {
		t.Fatalf("connected OwnerStatus = %q", got)
	}
	if got := origin.OwnerStatus(nil); got != "" {
		t.Fatalf("nil OwnerStatus = %q", got)
	}
}
