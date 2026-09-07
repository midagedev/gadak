package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"unsafe"

	gadak "github.com/midagedev/gadak"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/server"
	"github.com/midagedev/gadak/internal/store"
	"github.com/midagedev/gadak/internal/workspace"
)

// Exercises the exact seams the webview depends on: config.json, the API
// behind the browser guard (wails:// Origins, non-loopback Hosts), and the
// SPA fallback. Run with GADAK_PROFILE=demo — the profile is resolved from the
// environment at process start.
func TestFallbackHandler(t *testing.T) {
	if config.Profile() != "demo" {
		t.Skip("run with GADAK_PROFILE=demo — refusing to open the default profile's mirror")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	path, err := config.DBPath()
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ui, ok := gadak.WebUI()
	if !ok {
		t.Fatal("no embedded web UI — run `npm run build` at the repo root first")
	}
	reg := workspace.New()
	t.Cleanup(func() { reg.Close() })
	openedURLs := []string{}
	browse := newBrowseTabs()
	emb := &fakeEmbedder{}
	browse.bind(emb)
	h := fallbackHandler(server.New(db, cfg), ui, reg, func(u string) error {
		openedURLs = append(openedURLs, u)
		return nil
	}, browse)

	t.Run("config.json", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/config.json", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 200 || !strings.Contains(rec.Header().Get("Content-Type"), "json") {
			t.Fatalf("got %d %q", rec.Code, rec.Header().Get("Content-Type"))
		}
		// The UI reserves the window-controls corner off this flag; without it
		// the hidden title bar drops the traffic lights on the wordmark.
		var doc map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
			t.Fatal(err)
		}
		if doc["desktop"] != true {
			t.Fatalf("desktop flag missing: %v", doc["desktop"])
		}
		if doc["windowChrome"] != windowChrome() {
			t.Fatalf("windowChrome %v != owner %s", doc["windowChrome"], windowChrome())
		}
		// Same document `gadak serve` sends, plus the desktop keys — a dropped
		// field would switch off a surface in the app only.
		if _, ok := doc["apiBase"]; !ok {
			t.Fatalf("apiBase lost in the rewrite: %s", rec.Body.String())
		}
	})

	t.Run("api passes the browser guard with webview identity", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/issues/sync/progress/", nil)
		req.Host = "wails.localhost" // what the webview actually sends
		req.Header.Set("Origin", "wails://wails.localhost")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("spa fallback serves index.html for client routes", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/issues/NMA-1", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), "<!doctype html>") {
			t.Fatalf("got %d, body starts %q", rec.Code, rec.Body.String()[:min(80, rec.Body.Len())])
		}
	})

	// The webview has no new-window delegate, so the web bundle routes external
	// links here (lib/desktop-links.ts). http(s) with a host only — anything
	// else must be refused, not handed to the system browser.
	t.Run("desktop open routes http(s) to the browser and refuses the rest", func(t *testing.T) {
		post := func(body string) *httptest.ResponseRecorder {
			req := httptest.NewRequest("POST", "/desktop/open", strings.NewReader(body))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			return rec
		}
		if rec := post(`{"url":"https://example.atlassian.net/wiki/x"}`); rec.Code != 204 {
			t.Fatalf("https: got %d body %s", rec.Code, rec.Body.String())
		}
		if len(openedURLs) != 1 || openedURLs[0] != "https://example.atlassian.net/wiki/x" {
			t.Fatalf("opened = %v", openedURLs)
		}
		// mailto rides the same system-open path (GDK-339, About tab contact).
		if rec := post(`{"url":"mailto:midagedev@gmail.com"}`); rec.Code != 204 {
			t.Fatalf("mailto: got %d body %s", rec.Code, rec.Body.String())
		}
		openedURLs = openedURLs[:1]
		for _, bad := range []string{
			`{"url":"mailto:"}`,
			`{"url":"file:///etc/passwd"}`,
			`{"url":"javascript:alert(1)"}`,
			`{"url":"/api/v1/issues/"}`,
			`not json`,
		} {
			if rec := post(bad); rec.Code != 400 {
				t.Fatalf("%s: got %d, want 400", bad, rec.Code)
			}
		}
		if len(openedURLs) != 1 {
			t.Fatalf("refused URL leaked to the browser: %v", openedURLs)
		}
	})

	// The in-app browser pane at the handler seam: open two tabs (second takes
	// over as active), switch, close, and move the pane rect. URL validation
	// must match /desktop/open — an embedded tab is still a webview pointed at
	// the URL.
	t.Run("desktop browse pane lifecycle over the routes", func(t *testing.T) {
		post := func(path, body string) *httptest.ResponseRecorder {
			req := httptest.NewRequest("POST", path, strings.NewReader(body))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			return rec
		}
		state := func() (open []map[string]string, active string) {
			req := httptest.NewRequest("GET", "/desktop/browse/state", nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != 200 {
				t.Fatalf("state: got %d body %s", rec.Code, rec.Body.String())
			}
			var doc struct {
				Open   []map[string]string `json:"open"`
				Active string              `json:"active"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
				t.Fatal(err)
			}
			return doc.Open, doc.Active
		}

		// The SPA reports the pane rect before (or right after) the first tab.
		if rec := post("/desktop/browse/frame", `{"x":320,"y":48,"w":800,"h":600}`); rec.Code != 204 {
			t.Fatalf("frame: got %d body %s", rec.Code, rec.Body.String())
		}

		rec := post("/desktop/browse", `{"url":"https://example.atlassian.net/browse/NMA-1"}`)
		if rec.Code != 201 {
			t.Fatalf("browse: got %d body %s", rec.Code, rec.Body.String())
		}
		var first struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &first); err != nil || first.ID == "" {
			t.Fatalf("no id in %s (%v)", rec.Body.String(), err)
		}
		if got := emb.frameOf(first.ID); got != (frameRect{X: 320, Y: 48, W: 800, H: 600}) {
			t.Fatalf("tab created off the reported rect: %+v", got)
		}

		rec = post("/desktop/browse", `{"url":"https://example.atlassian.net/wiki/spaces/X/pages/42"}`)
		if rec.Code != 201 {
			t.Fatalf("second tab: got %d body %s", rec.Code, rec.Body.String())
		}
		var second struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &second)

		// Opening made the newest tab the visible one; the first hid.
		open, active := state()
		if len(open) != 2 || active != second.ID {
			t.Fatalf("open=%v active=%q want active %q", open, active, second.ID)
		}
		if !emb.hidden(first.ID) || emb.hidden(second.ID) {
			t.Fatalf("visibility: first hidden=%v second hidden=%v", emb.hidden(first.ID), emb.hidden(second.ID))
		}
		// Live titles come off the webview for the tab strip.
		if open[0]["title"] != "제목:"+emb.urlOf(first.ID) {
			t.Fatalf("state title = %q", open[0]["title"])
		}

		// Switch back, then hide everything (SPA overlay up).
		if rec := post("/desktop/browse/activate", `{"id":"`+first.ID+`"}`); rec.Code != 204 {
			t.Fatalf("activate: got %d", rec.Code)
		}
		if emb.hidden(first.ID) {
			t.Fatal("activate did not show the tab")
		}
		if rec := post("/desktop/browse/activate", `{"id":""}`); rec.Code != 204 {
			t.Fatalf("hide all: got %d", rec.Code)
		}
		if !emb.hidden(first.ID) || !emb.hidden(second.ID) {
			t.Fatal("hide-all left a native view over the SPA")
		}

		// A later frame report moves every tab, hidden ones included.
		if rec := post("/desktop/browse/frame", `{"x":0,"y":48,"w":1200,"h":700}`); rec.Code != 204 {
			t.Fatalf("re-frame: got %d", rec.Code)
		}
		if got := emb.frameOf(second.ID); got.W != 1200 {
			t.Fatalf("hidden tab not re-framed: %+v", got)
		}

		if rec := post("/desktop/browse/close", `{"id":"`+first.ID+`"}`); rec.Code != 204 {
			t.Fatalf("close: got %d", rec.Code)
		}
		open, _ = state()
		if len(open) != 1 || open[0]["id"] != second.ID {
			t.Fatalf("after close, open = %v", open)
		}
		if !emb.closed(first.ID) {
			t.Fatal("close route never reached the native layer")
		}

		for _, bad := range []string{
			`{"url":"file:///etc/passwd"}`,
			`{"url":"javascript:alert(1)"}`,
			`not json`,
		} {
			if rec := post("/desktop/browse", bad); rec.Code != 400 {
				t.Fatalf("%s: got %d, want 400", bad, rec.Code)
			}
		}
		if emb.created != 2 {
			t.Fatalf("refused URL created a tab: %d", emb.created)
		}
		if rec := post("/desktop/browse/close", `{"id":"999"}`); rec.Code != 400 {
			t.Fatalf("closing unknown tab: got %d, want 400", rec.Code)
		}

		// Close-all ("" id): what a freshly mounted SPA document sends on
		// boot, so a predecessor's webviews cannot outlive it (GDK-80).
		if rec := post("/desktop/browse/close", `{"id":""}`); rec.Code != 204 {
			t.Fatalf("close-all: got %d", rec.Code)
		}
		open, active = state()
		if len(open) != 0 || active != "" {
			t.Fatalf("after close-all, open=%v active=%q", open, active)
		}
		if !emb.closed(second.ID) {
			t.Fatal("close-all left a native view alive")
		}
	})
}

// fakeEmbedder stands in for the native WKWebView layer. browseTabs assigns
// ids "1","2",… in creation order, so helpers index by that same order.
type fakeTab struct {
	url    string
	frame  frameRect
	hid    bool
	closed bool
}

type fakeEmbedder struct {
	created int
	byOrder []*fakeTab
}

func (f *fakeEmbedder) Create(url string, fr frameRect) (unsafe.Pointer, error) {
	tab := &fakeTab{url: url, frame: fr, hid: true}
	f.created++
	f.byOrder = append(f.byOrder, tab)
	return unsafe.Pointer(tab), nil
}

func (f *fakeEmbedder) SetFrame(wv unsafe.Pointer, fr frameRect) { (*fakeTab)(wv).frame = fr }
func (f *fakeEmbedder) SetHidden(wv unsafe.Pointer, h bool)      { (*fakeTab)(wv).hid = h }
func (f *fakeEmbedder) Close(wv unsafe.Pointer)                  { (*fakeTab)(wv).closed = true }
func (f *fakeEmbedder) Info(wv unsafe.Pointer) (string, string) {
	t := (*fakeTab)(wv)
	return "제목:" + t.url, t.url
}

func (f *fakeEmbedder) tab(id string) *fakeTab {
	n, err := strconv.Atoi(id)
	if err != nil || n < 1 || n > len(f.byOrder) {
		return &fakeTab{}
	}
	return f.byOrder[n-1]
}

func (f *fakeEmbedder) frameOf(id string) frameRect { return f.tab(id).frame }
func (f *fakeEmbedder) hidden(id string) bool       { return f.tab(id).hid }
func (f *fakeEmbedder) closed(id string) bool       { return f.tab(id).closed }
func (f *fakeEmbedder) urlOf(id string) string      { return f.tab(id).url }

// seedDesktopProfile writes config.json + gadak.db under GADAK_HOME for tests that
// must not touch the real ~/.gadak tree.
func seedDesktopProfile(t *testing.T, name string, cfg *config.Config) {
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
	_ = db.Close()
}

func TestDesktopWorkspaceRoutes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Cleanup(func() { config.SetProfile("") })
	config.SetProfile("")

	seedDesktopProfile(t, "", &config.Config{
		Site: "http://127.0.0.1:1", Email: "a@example.invalid", Token: "test-token", Projects: []string{"AAA"},
	})
	seedDesktopProfile(t, "work", &config.Config{
		Site: "http://127.0.0.1:1", Email: "b@example.invalid", Token: "test-token", Projects: []string{"BBB"},
	})

	primaryCfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	dbPath, err := config.DBPath()
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	ui := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<!doctype html><html>spa</html>")},
	}
	reg := workspace.New()
	t.Cleanup(func() { reg.Close() })
	// Unbound browse: the routes exist but answer 503 until bind() — the same
	// state the app is in before application.New returns.
	h := fallbackHandler(server.New(db, primaryCfg), ui, reg, nil, newBrowseTabs())

	t.Run("workspace config.json is JSON", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/w/work/config.json", nil)
		req.Host = "wails.localhost"
		req.Header.Set("Origin", "wails://wails.localhost")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Header().Get("Content-Type"), "json") {
			t.Fatalf("Content-Type %q", rec.Header().Get("Content-Type"))
		}
		var doc map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
			t.Fatalf("not JSON: %v body %s", err, rec.Body.String())
		}
		if doc["apiBase"] != "/w/work/api/v1/issues/" {
			t.Fatalf("apiBase %v", doc["apiBase"])
		}
		// A workspace page in this webview is still the desktop app. Without
		// the stamp the SPA on /w/<name>/ used browser transports that are
		// dead in the webview (GDK-178: clipboard silently broken there).
		if doc["desktop"] != true {
			t.Fatalf("workspace config must carry desktop:true, got %v", doc["desktop"])
		}
		if doc["windowChrome"] != windowChrome() {
			t.Fatalf("workspace windowChrome %v != owner %s", doc["windowChrome"], windowChrome())
		}
		// Must not be SPA HTML.
		if strings.Contains(rec.Body.String(), "<!doctype html>") {
			t.Fatalf("got SPA HTML instead of config JSON")
		}
	})

	t.Run("workspace SPA index has one runtime.js", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/w/work/", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		if n := strings.Count(rec.Body.String(), "/wails/runtime.js"); n != 1 {
			t.Fatalf("runtime.js refs = %d\n%s", n, rec.Body.String())
		}
	})

	t.Run("workspaces list is JSON", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/workspaces", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Header().Get("Content-Type"), "json") {
			t.Fatalf("Content-Type %q", rec.Header().Get("Content-Type"))
		}
		var doc struct {
			Workspaces []json.RawMessage `json:"workspaces"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
			t.Fatalf("not JSON: %v body %s", err, rec.Body.String())
		}
		if doc.Workspaces == nil {
			t.Fatalf("workspaces not an array: %s", rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "<!doctype html>") {
			t.Fatalf("got SPA HTML instead of workspaces JSON")
		}
	})
}
