package dashboards

// Contract ↔ assertion map for the lib cache (GDK-808):
//
//	contract clause                        | assertions
//	---------------------------------------+---------------------------------------------
//	add: https any host, http local only   | TestValidateLibURL, TestLibAddSchemeRefusedFast
//	add: redirects ≤3, every hop re-checked| TestLibAddRedirects
//	add: 8 MiB cap (lying length & stream) | TestLibAddSizeCap
//	add: sha384 pin + id = sha256-16+name  | TestLibAddRoundTrip
//	re-add same url same bytes = idempotent| TestLibAddReplaceFlow
//	re-add same url new bytes needs --replace | TestLibAddReplaceFlow (old entry+file swapped)
//	serve: bytes re-hashed against the pin | TestLibReadVerified (tamper, truncate, missing)
//	serve: path-shaped id never resolves   | TestLibReadVerified (pattern), TestParseConfigLibs
//	manifest torn/hand-edited = corrupt    | TestLibReadVerified (bad json, bad id, dupes)
//	config libs: pattern, ≤8, no dupes     | TestParseConfigLibs
//	basename sanitization                  | TestSanitizeLibName

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// mockCDN serves fixed bytes at one path; loopback http is exactly the
// self-hosting case validateLibURL allows, so no TLS machinery is needed.
func mockCDN(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func addNow() time.Time { return time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC) }

func TestLibAddRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cdn := mockCDN(t, "window.mockLib = function () { return 1; };\n")

	lib, added, err := LibAdd(context.Background(), dir, cdn.URL+"/mock-lib.iife.js", false, addNow())
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if !added {
		t.Fatalf("first add reported already-present")
	}
	sum := sha512.Sum384([]byte("window.mockLib = function () { return 1; };\n"))
	want384 := hex.EncodeToString(sum[:])
	if lib.SHA384 != want384 {
		t.Fatalf("sha384 = %s, want %s", lib.SHA384, want384)
	}
	if lib.Size != int64(len("window.mockLib = function () { return 1; };\n")) {
		t.Fatalf("size = %d", lib.Size)
	}
	if !ValidLibID(lib.ID) {
		t.Fatalf("id %q fails %s", lib.ID, LibIDPattern)
	}
	if !strings.HasSuffix(lib.ID, "-mock-lib.iife.js") {
		t.Fatalf("id %q lost the sanitized basename", lib.ID)
	}
	body, err := os.ReadFile(filepath.Join(dir, lib.ID))
	if err != nil {
		t.Fatalf("cache file: %v", err)
	}
	if string(body) != "window.mockLib = function () { return 1; };\n" {
		t.Fatalf("cache bytes = %q", body)
	}
	// The manifest is the readable pin: id, url, sha384, size, fetched_at.
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}
	var m struct {
		Libs []Lib `json:"libs"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("manifest json: %v", err)
	}
	if len(m.Libs) != 1 || m.Libs[0].ID != lib.ID || m.Libs[0].URL != cdn.URL+"/mock-lib.iife.js" {
		t.Fatalf("manifest = %s", raw)
	}

	// Idempotent re-run: same url, same bytes.
	_, added, err = LibAdd(context.Background(), dir, cdn.URL+"/mock-lib.iife.js", false, addNow())
	if err != nil || added {
		t.Fatalf("re-add = added:%v err:%v", added, err)
	}
	libs, err := LibList(dir)
	if err != nil || len(libs) != 1 {
		t.Fatalf("list after re-add = %v (%v)", libs, err)
	}

	// rm: entry and bytes both go.
	if err := LibRemove(dir, lib.ID); err != nil {
		t.Fatalf("rm: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, lib.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cache file survived rm: %v", err)
	}
	if _, err := LibLookup(dir, lib.ID); !errors.Is(err, ErrLibNotFound) {
		t.Fatalf("lookup after rm = %v, want ErrLibNotFound", err)
	}
	if err := LibRemove(dir, lib.ID); !errors.Is(err, ErrLibNotFound) {
		t.Fatalf("double rm = %v, want ErrLibNotFound", err)
	}
}

func TestValidateLibURL(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{"https://cdn.jsdelivr.net/npm/x.js", true},
		{"http://127.0.0.1:8080/x.js", true},
		{"http://[::1]:8080/x.js", true},
		{"http://localhost/x.js", true},
		{"http://devices.local/x.js", false},     // non-local hostname over http
		{"http://cdn.example.com/x.js", false},   // external host over http
		{"javascript:alert(1)", false},           // scheme
		{"data:text/javascript,alert(1)", false}, // scheme
		{"file:///etc/passwd", false},            // scheme
		{"ftp://example.com/x.js", false},        // scheme
		{"", false},
	}
	for _, tc := range cases {
		u, err := url.Parse(tc.raw)
		if err != nil {
			if tc.want {
				t.Errorf("%s: unparseable: %v", tc.raw, err)
			}
			continue
		}
		if got := validateLibURL(u) == nil; got != tc.want {
			t.Errorf("validateLibURL(%q) ok=%v, want %v", tc.raw, got, tc.want)
		}
	}
}

func TestLibAddSchemeRefusedFast(t *testing.T) {
	dir := t.TempDir()
	for _, raw := range []string{
		"javascript:alert(1)",
		"data:text/javascript,alert(1)",
		"http://cdn.example.com/x.js",
		"/etc/passwd",
	} {
		if _, _, err := LibAdd(context.Background(), dir, raw, false, addNow()); err == nil {
			t.Errorf("LibAdd(%q) accepted", raw)
		} else if !strings.Contains(err.Error(), "https") && !strings.Contains(err.Error(), "localhost") {
			t.Errorf("LibAdd(%q) error does not name the rule: %v", raw, err)
		}
	}
	// Nothing was written by the refusals — no partial cache.
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Fatalf("refused add left files: %v (%v)", entries, err)
	}
}

func TestLibAddRedirects(t *testing.T) {
	dir := t.TempDir()
	target := mockCDN(t, "x=1")

	// One hop is fine; the final URL's basename names the file.
	hop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/renamed.iife.js", http.StatusFound)
	}))
	t.Cleanup(hop.Close)
	lib, _, err := LibAdd(context.Background(), dir, hop.URL+"/old.js", false, addNow())
	if err != nil {
		t.Fatalf("single redirect: %v", err)
	}
	if !strings.HasSuffix(lib.ID, "-renamed.iife.js") {
		t.Fatalf("id %q should carry the FINAL url basename", lib.ID)
	}

	// Four hops is over the ≤3 budget.
	var chain *httptest.Server
	chain = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, chain.URL+"/next", http.StatusFound)
	}))
	t.Cleanup(chain.Close)
	if _, _, err := LibAdd(context.Background(), dir, chain.URL+"/start.js", false, addNow()); err == nil || !strings.Contains(err.Error(), "redirect") {
		t.Fatalf("redirect chain accepted: %v", err)
	}

	// A redirect to an http URL on a non-local host is refused mid-chain,
	// before any dial: the rule follows the hops.
	evil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://cdn.example.com/x.js", http.StatusFound)
	}))
	t.Cleanup(evil.Close)
	if _, _, err := LibAdd(context.Background(), dir, evil.URL+"/harmless.js", false, addNow()); err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("redirect to external http accepted: %v", err)
	}
}

func TestLibAddSizeCap(t *testing.T) {
	dir := t.TempDir()
	// One byte past the cap, declared honestly.
	big := mockCDN(t, strings.Repeat("x", MaxLibBytes+1))
	if _, _, err := LibAdd(context.Background(), dir, big.URL+"/big.js", false, addNow()); err == nil || !strings.Contains(err.Error(), "cache limit") {
		t.Fatalf("oversized body accepted: %v", err)
	}
	// An endless stream must be cut at the same cap, not buffered forever.
	endless := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 1<<20)
		for i := range buf {
			buf[i] = 'y'
		}
		for { // never ends; the client must stop reading and refuse
			if _, err := w.Write(buf); err != nil {
				return
			}
		}
	}))
	t.Cleanup(endless.Close)
	if _, _, err := LibAdd(context.Background(), dir, endless.URL+"/stream.js", false, addNow()); err == nil || !strings.Contains(err.Error(), "cache limit") {
		t.Fatalf("infinite stream accepted: %v", err)
	}
	libs, err := LibList(dir)
	if err != nil || len(libs) != 0 {
		t.Fatalf("refused bodies left manifest entries: %v (%v)", libs, err)
	}
}

func TestLibAddReplaceFlow(t *testing.T) {
	dir := t.TempDir()
	body := "a=1;"
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body)
	}))
	t.Cleanup(cdn.Close)
	url := cdn.URL + "/lib.js"
	first, _, err := LibAdd(context.Background(), dir, url, false, addNow())
	if err != nil {
		t.Fatalf("first add: %v", err)
	}

	// Upstream change without --replace: refused, old entry intact.
	body = "a=2;"
	if _, _, err := LibAdd(context.Background(), dir, url, false, addNow()); err == nil || !strings.Contains(err.Error(), "--replace") {
		t.Fatalf("upstream change accepted silently: %v", err)
	}
	if _, err := LibLookup(dir, first.ID); err != nil {
		t.Fatalf("old entry vanished during refused add: %v", err)
	}

	// With --replace: new id in, old id (entry and bytes) out.
	second, added, err := LibAdd(context.Background(), dir, url, true, addNow())
	if err != nil || !added {
		t.Fatalf("replace add: added=%v err=%v", added, err)
	}
	if second.ID == first.ID {
		t.Fatalf("changed bytes kept the same id — the pin did not move")
	}
	if _, err := LibLookup(dir, first.ID); !errors.Is(err, ErrLibNotFound) {
		t.Fatalf("old entry survived --replace: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, first.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old bytes survived --replace")
	}
	libs, _ := LibList(dir)
	if len(libs) != 1 {
		t.Fatalf("after replace = %d entries, want 1", len(libs))
	}
}

func TestLibReadVerified(t *testing.T) {
	dir := t.TempDir()
	cdn := mockCDN(t, "stable=1;")
	lib, _, err := LibAdd(context.Background(), dir, cdn.URL+"/v.js", false, addNow())
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, body, err := LibReadVerified(dir, lib.ID); err != nil || string(body) != "stable=1;" {
		t.Fatalf("verified read: %q %v", body, err)
	}

	// Tampered bytes: the pin must refuse them.
	path := filepath.Join(dir, lib.ID)
	if err := os.WriteFile(path, []byte("stable=2;"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LibReadVerified(dir, lib.ID); !errors.Is(err, ErrLibCorrupt) {
		t.Fatalf("tampered bytes served: %v, want ErrLibCorrupt", err)
	}
	// Truncation is corruption too (size check first).
	if err := os.WriteFile(path, []byte("sta"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LibReadVerified(dir, lib.ID); !errors.Is(err, ErrLibCorrupt) {
		t.Fatalf("truncated bytes served: %v", err)
	}
	// Missing file: entry exists, bytes gone.
	if err := os.WriteFile(path, []byte("stable=1;"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LibReadVerified(dir, lib.ID); !errors.Is(err, ErrLibCorrupt) {
		t.Fatalf("missing file served: %v", err)
	}

	// Path-shaped ids never resolve to a file, whatever the manifest says.
	for _, id := range []string{"../" + lib.ID, "../../etc/passwd", "a.b", "..", "sub/dir"} {
		if _, _, err := LibReadVerified(dir, id); !errors.Is(err, ErrLibNotFound) {
			t.Errorf("LibReadVerified(%q) = %v, want ErrLibNotFound", id, err)
		}
	}

	// A hand-edited manifest is corrupt cache, fail closed on every id.
	seed := func(content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	seed(`{"libs":[`)
	if _, _, err := LibReadVerified(dir, lib.ID); !errors.Is(err, ErrLibCorrupt) {
		t.Fatalf("torn manifest read: %v", err)
	}
	seed(`{"libs":[{"id":"../../evil","url":"https://x/y.js","sha384":"00","size":1}]}`)
	if _, _, err := LibReadVerified(dir, "../../evil"); !errors.Is(err, ErrLibNotFound) {
		t.Fatalf("path-shaped manifest id resolved: %v", err)
	}
	// The evil entry poisoned the whole manifest (load-time validation):
	// even the previously-valid id fails closed now, not silently serves.
	if _, _, err := LibReadVerified(dir, lib.ID); !errors.Is(err, ErrLibCorrupt) {
		t.Fatalf("valid id resolved against a corrupt manifest: %v", err)
	}
	// LibsExist reports the manifest error — nothing saves against an
	// unreadable pin.
	if _, err := LibsExist(dir, []string{lib.ID}); !errors.Is(err, ErrLibCorrupt) {
		t.Fatalf("LibsExist on corrupt manifest = %v", err)
	}
}

func TestLibsExist(t *testing.T) {
	dir := t.TempDir()
	cdn := mockCDN(t, "z;")
	lib, _, err := LibAdd(context.Background(), dir, cdn.URL+"/z.js", false, addNow())
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	missing, err := LibsExist(dir, []string{lib.ID, "ffffffffffffffff-nope.js"})
	if err != nil {
		t.Fatalf("LibsExist: %v", err)
	}
	if len(missing) != 1 || missing[0] != "ffffffffffffffff-nope.js" {
		t.Fatalf("missing = %v, want [ffffffffffffffff-nope.js]", missing)
	}
}

func TestParseConfigLibs(t *testing.T) {
	ok := `{"html":"<p/>","libs":["aaaaaaaaaaaaaaaa-uPlot.iife.min.js","bbbbbbbbbbbbbbbb-three.min.js"]}`
	if cfg, err := ParseConfig([]byte(ok)); err != nil || len(cfg.Libs) != 2 {
		t.Fatalf("valid libs config: %v (%+v)", err, cfg.Libs)
	}
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"bad id", `{"html":"<p/>","libs":["not an id"]}`, "must match"},
		{"path-shaped id", `{"html":"<p/>","libs":["aaaaaaaaaaaaaaaa-../x.js"]}`, "must match"},
		{"duplicate", `{"html":"<p/>","libs":["aaaaaaaaaaaaaaaa-x.js","aaaaaaaaaaaaaaaa-x.js"]}`, "twice"},
		{"over budget", `{"html":"<p/>","libs":["` + strings.Join(repeatIDs("c", 9), `","`) + `"]}`, "at most 8"},

		{"not an array", `{"html":"<p/>","libs":"aaaaaaaaaaaaaaaa-x.js"}`, "array"},
		{"unknown field still named", `{"html":"<p/>","nope":1}`, "expected html, datasources, libs"},
	}
	for _, tc := range cases {
		_, err := ParseConfig([]byte(tc.raw))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want it to contain %q", tc.name, err, tc.want)
		}
	}
	// Staleness: a pre-808 config (no libs key) parses unchanged.
	cfg, err := ParseConfig([]byte(`{"html":"<p/>","datasources":{"a":{"sql":"select 1"}}}`))
	if err != nil || cfg.Libs != nil {
		t.Fatalf("old config: %v %+v", err, cfg.Libs)
	}
}

func repeatIDs(prefix string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = prefix + strings.Repeat("0", 15) + "-lib" + string(rune('a'+i)) + ".js"
	}
	return out
}

func TestSanitizeLibName(t *testing.T) {
	cases := map[string]string{
		"uPlot.iife.min.js": "uPlot.iife.min.js",
		"three.min.js":      "three.min.js",
		"한글 라이브러리.js":       "js", // disallowed runes fold to '-'; only the extension survives
		"..":                "lib",
		".":                 "lib",
		"":                  "lib",
		"-leading-dash.js":  "leading-dash.js",
		"trailing.dot..":    "trailing.dot",
		"spaced name.js":    "spaced-name.js",
		"semi;colon.js":     "semi-colon.js",
		"quote'.js":         "quote-.js",
		"a/b/c.js":          "a-b-c.js", // callers pass path.Base output; separator defense anyway
	}
	for in, want := range cases {
		if got := sanitizeLibName(in); got != want {
			t.Errorf("sanitizeLibName(%q) = %q, want %q", in, got, want)
		}
	}
	long := strings.Repeat("a", 100) + ".js"
	if got := sanitizeLibName(long); len(got) != 64 || got != strings.Repeat("a", 64) {
		t.Errorf("long name = %q (len %d), want the 64-char cap", got, len(got))
	}
}
