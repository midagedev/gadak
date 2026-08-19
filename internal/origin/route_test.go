package origin_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
		origin.SetInProcess(false)
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
	origin.SetInProcess(true)

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
	origin.SetInProcess(false)

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
	origin.SetInProcess(false)

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

func TestClientFallsBackOnProfileMismatch(t *testing.T) {
	cfg, _, _, _ := liveStandaloneServe(t)
	config.SetProfile("other")
	t.Cleanup(func() { config.SetProfile("") })

	before := origin.SessionsConstructed()
	c, err := origin.Client(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !origin.TransportIsEmbedded(c.HTTP.Transport) {
		t.Fatalf("Transport type %T after profile mismatch, want embedded", c.HTTP.Transport)
	}
	if got := origin.SessionsConstructed(); got != before+1 {
		t.Fatalf("profile mismatch should embed; constructed %d → %d", before, got)
	}
}

func TestSetInProcessSkipsRouting(t *testing.T) {
	cfg, _, _, _ := liveStandaloneServe(t)
	origin.SetInProcess(true)
	t.Cleanup(func() { origin.SetInProcess(false) })

	before := origin.SessionsConstructed()
	c, err := origin.Client(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !origin.TransportIsEmbedded(c.HTTP.Transport) {
		t.Fatalf("in-process Client Transport %T, want embedded", c.HTTP.Transport)
	}
	if got := origin.SessionsConstructed(); got != before+1 {
		t.Fatalf("in-process should embed; constructed %d → %d", before, got)
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
