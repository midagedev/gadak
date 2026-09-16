package server

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"testing/fstest"

	"github.com/midagedev/gadak/internal/config"
)

// The phone bundle at /m/ (GDK-1966): the phone opens the serve in its
// browser — same origin, same guard — and gets a small SPA built from
// mobile/. These gates pin the serving shape and the phone_urls the web
// Devices tab reads.

const phoneShell = "<!doctype html><html><title>phone</title></html>"

func fakePhoneFS() fs.FS {
	return fstest.MapFS{
		"index.html":    &fstest.MapFile{Data: []byte(phoneShell)},
		"assets/app.js": &fstest.MapFile{Data: []byte("console.log('phone')")},
	}
}

func TestPhoneUIHandlerServesIndexAssetsAndFallback(t *testing.T) {
	h := PhoneUIHandler(func() (fs.FS, bool) { return fakePhoneFS(), true })

	get := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	// The mount root answers the SPA shell.
	rec := get("/m/")
	if rec.Code != http.StatusOK {
		t.Fatalf("/m/ status %d, want 200; body %s", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); got != phoneShell {
		t.Fatalf("/m/ body %q, want the index shell", got)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("/m/ Content-Type %q, want text/html; charset=utf-8", ct)
	}

	// An extension-less route (the SPA's own issue view) falls back to the
	// shell so a reload mid-view survives.
	rec = get("/m/issues/NMB-1")
	if rec.Code != http.StatusOK || rec.Body.String() != phoneShell {
		t.Fatalf("/m/issues/NMB-1 status %d body %q, want the index shell", rec.Code, rec.Body.String())
	}

	// A real asset is served as bytes, not the shell.
	rec = get("/m/assets/app.js")
	if rec.Code != http.StatusOK {
		t.Fatalf("/m/assets/app.js status %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != "console.log('phone')" {
		t.Fatalf("/m/assets/app.js body %q, want the asset bytes", got)
	}

	// A missing file WITH an extension is a 404, not the shell — a typo'd
	// asset URL must fail loudly, not boot the app.
	rec = get("/m/assets/missing.js")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("/m/assets/missing.js status %d, want 404; body %s", rec.Code, rec.Body.String())
	}

	// Directory traversal stays inside the bundle.
	rec = get("/m/../config.json")
	if rec.Code != http.StatusOK || rec.Body.String() != phoneShell {
		t.Fatalf("traversal-adjacent path status %d body %q, want the shell (cleaned to a route)", rec.Code, rec.Body.String())
	}
}

func TestPhoneUIHandlerMissingBundleIs503(t *testing.T) {
	h := PhoneUIHandler(func() (fs.FS, bool) { return nil, false })
	for _, path := range []string{"/m/", "/m/issues/NMB-1"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s status %d, want 503; body %s", path, rec.Code, rec.Body.String())
		}
		got := decode[map[string]string](t, rec)
		if got["error"] != "phone_ui_missing" || got["hint"] != "make phone" {
			t.Fatalf("%s body = %v, want phone_ui_missing / make phone", path, got)
		}
	}
}

// PhoneURLs is the address list config.json carries as phone_urls: every
// --public-url origin always; the listen address only when a phone could
// actually dial it (non-loopback bind, and a name rather than a wildcard
// unless the Tailscale probe supplied one).
func TestPhoneURLs(t *testing.T) {
	cases := []struct {
		name       string
		listenAddr string
		publicURLs []string
		tsName     string
		want       []string
	}{
		{"loopback only, nothing to dial", "127.0.0.1:7777", nil, "", []string{}},
		{"empty listen addr is the loopback default", "", nil, "vps.example.ts.net", []string{}},
		{"public urls always listed", "127.0.0.1:7777",
			[]string{"https://gadak.example.com", "http://alt.example.com:8443"}, "",
			[]string{"https://gadak.example.com/m/", "http://alt.example.com:8443/m/"}},
		{"non-loopback IP listen is dialable", "192.0.2.10:7777", nil, "",
			[]string{"http://192.0.2.10:7777/m/"}},
		{"tailscale name beats the wildcard", "0.0.0.0:7777", nil, "vps.example.ts.net",
			[]string{"http://vps.example.ts.net:7777/m/"}},
		{"tailscale name beats the bare IP too", "192.0.2.10:7777", nil, "vps.example.ts.net",
			[]string{"http://vps.example.ts.net:7777/m/"}},
		{"wildcard without a name dials nothing", "0.0.0.0:7777", nil, "", []string{}},
		{"both sources together", "0.0.0.0:7777", []string{"https://gadak.example.com"}, "vps.example.ts.net",
			[]string{"https://gadak.example.com/m/", "http://vps.example.ts.net:7777/m/"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := PhoneURLs(c.listenAddr, c.publicURLs, c.tsName)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("PhoneURLs(%q, %v, %q) = %v, want %v", c.listenAddr, c.publicURLs, c.tsName, got, c.want)
			}
		})
	}
}

// The document contract: WebConfigPhone injects phone_urls pre-marshal, and
// every other config.json shape carries the key as [] (never absent, never
// null) so a web build that predates the field binds without guards.
func TestWebConfigCarriesPhoneURLs(t *testing.T) {
	doc, err := WebConfigPhone(&config.Config{}, []string{"https://gadak.example.com/m/"})
	if err != nil {
		t.Fatal(err)
	}
	var with struct {
		PhoneURLs []string `json:"phone_urls"`
	}
	if err := json.Unmarshal(doc, &with); err != nil {
		t.Fatalf("decode WebConfigPhone: %v (%s)", err, doc)
	}
	if !reflect.DeepEqual(with.PhoneURLs, []string{"https://gadak.example.com/m/"}) {
		t.Fatalf("phone_urls = %v, want the injected list", with.PhoneURLs)
	}

	base, err := WebConfig(&config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	var plain struct {
		PhoneURLs []string `json:"phone_urls"`
	}
	if err := json.Unmarshal(base, &plain); err != nil {
		t.Fatalf("decode WebConfig: %v (%s)", err, base)
	}
	if !reflect.DeepEqual(plain.PhoneURLs, []string{}) {
		t.Fatalf("plain WebConfig phone_urls = %v, want [] (present, empty)", plain.PhoneURLs)
	}
}
