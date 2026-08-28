package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
)

// apiMirror is mirror() plus optional Confluence config for wiki-path tests.
func apiMirror(t *testing.T, site string, withConfluence bool) *config.Config {
	t.Helper()
	cfg := mirror(t, site)
	if withConfluence {
		cfg.Confluence = &config.ConfluenceConfig{}
		if err := cfg.Save(); err != nil {
			t.Fatalf("save confluence: %v", err)
		}
	}
	return cfg
}

func captureErr(t *testing.T, fn func() error) (stdout, stderr string, err error) {
	t.Helper()
	rOut, wOut, e := os.Pipe()
	if e != nil {
		t.Fatal(e)
	}
	rErr, wErr, e := os.Pipe()
	if e != nil {
		t.Fatal(e)
	}
	savedOut, savedErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = wOut, wErr
	err = fn()
	os.Stdout, os.Stderr = savedOut, savedErr
	_ = wOut.Close()
	_ = wErr.Close()
	outB, _ := io.ReadAll(rOut)
	errB, _ := io.ReadAll(rErr)
	return string(outB), string(errB), err
}

func TestAPI_GETBodyPassthrough(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.Method != http.MethodGet || r.URL.Path != "/rest/api/3/myself" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.Write([]byte(`{"accountId":"a1","displayName":"Ada"}`))
	}))
	t.Cleanup(srv.Close)
	apiMirror(t, srv.URL, false)

	out, _, err := captureErr(t, func() error {
		return cmdAPI([]string{"/rest/api/3/myself"})
	})
	if err != nil {
		t.Fatalf("cmdAPI: %v", err)
	}
	if out != `{"accountId":"a1","displayName":"Ada"}` {
		t.Errorf("stdout = %q", out)
	}
	if gotAuth == "" {
		t.Error("Authorization header missing")
	}
	if strings.Contains(out, "token") {
		t.Error("token must not appear in output")
	}
}

func TestAPI_RejectsAbsoluteURL(t *testing.T) {
	hits := int32(0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
	}))
	t.Cleanup(srv.Close)
	apiMirror(t, srv.URL, false)

	for _, path := range []string{
		"https://evil.example/steal",
		"//evil.example/steal",
	} {
		_, _, err := captureErr(t, func() error {
			return cmdAPI([]string{path})
		})
		if err == nil {
			t.Errorf("%q: want error", path)
			continue
		}
		if !strings.Contains(err.Error(), "absolute") && !strings.Contains(err.Error(), "must start with /") {
			t.Errorf("%q: err = %v", path, err)
		}
		if strings.Contains(err.Error(), "token") {
			t.Errorf("%q: token leaked", path)
		}
	}
	if atomic.LoadInt32(&hits) != 0 {
		t.Errorf("absolute URL must not hit the server: hits=%d", hits)
	}
}

func TestAPI_POSTRequiresWrite(t *testing.T) {
	hits := int32(0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
	}))
	t.Cleanup(srv.Close)
	apiMirror(t, srv.URL, false)

	_, _, err := captureErr(t, func() error {
		return cmdAPI([]string{"POST", "/rest/api/3/issue/A-1/worklog", "--data", `{"timeSpent":"1h"}`})
	})
	if err == nil || !strings.Contains(err.Error(), "--write") {
		t.Fatalf("want --write error, got %v", err)
	}
	if atomic.LoadInt32(&hits) != 0 {
		t.Error("POST without --write must not leave the process")
	}
}

func TestAPI_POSTWithWriteAndRetryPolicy(t *testing.T) {
	// 500 must not be retried on write; body still returned.
	t.Run("500 once", func(t *testing.T) {
		calls := int32(0)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&calls, 1)
			if r.Method != http.MethodPost {
				t.Errorf("method %s", r.Method)
			}
			body, _ := io.ReadAll(r.Body)
			if string(body) != `{"timeSpent":"1h"}` {
				t.Errorf("body = %s", body)
			}
			if r.Header.Get("Content-Type") != "application/json" {
				t.Errorf("Content-Type = %q", r.Header.Get("Content-Type"))
			}
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"errorMessages":["server error"]}`))
		}))
		t.Cleanup(srv.Close)
		apiMirror(t, srv.URL, false)

		out, _, err := captureErr(t, func() error {
			return cmdAPI([]string{"POST", "/rest/api/3/issue/A-1/worklog",
				"--data", `{"timeSpent":"1h"}`, "--write"})
		})
		if err == nil || !strings.Contains(err.Error(), "HTTP 500") {
			t.Fatalf("err = %v", err)
		}
		if !strings.Contains(out, "server error") {
			t.Errorf("stdout should carry body: %q", out)
		}
		if atomic.LoadInt32(&calls) != 1 {
			t.Errorf("calls = %d, want 1", calls)
		}
	})

	t.Run("429 retried", func(t *testing.T) {
		// Retry-After:0 falls through to the client's Backoff (1s in
		// production). Tests own a zero wait; the retry itself is still asserted.
		zero := time.Duration(0)
		apiBackoffOverride = &zero
		t.Cleanup(func() { apiBackoffOverride = nil })

		calls := int32(0)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n := atomic.AddInt32(&calls, 1)
			if n == 1 {
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id":"wl-1"}`))
		}))
		t.Cleanup(srv.Close)
		apiMirror(t, srv.URL, false)

		out, _, err := captureErr(t, func() error {
			return cmdAPI([]string{"POST", "/rest/api/3/issue/A-1/worklog",
				"--data", `{"timeSpent":"1h"}`, "--write"})
		})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if out != `{"id":"wl-1"}` {
			t.Errorf("stdout = %q", out)
		}
		if atomic.LoadInt32(&calls) != 2 {
			t.Errorf("calls = %d, want 2", calls)
		}
	})
}

func TestAPI_WikiRoutesToConfluence(t *testing.T) {
	var sawPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawPath = r.URL.Path
		if r.URL.Query().Get("limit") != "5" {
			t.Errorf("limit = %q", r.URL.Query().Get("limit"))
		}
		w.Write([]byte(`{"results":[{"id":"1"}]}`))
	}))
	t.Cleanup(srv.Close)
	apiMirror(t, srv.URL, true)

	out, _, err := captureErr(t, func() error {
		return cmdAPI([]string{"GET", "/wiki/api/v2/spaces", "--query", "limit=5"})
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	// confluence.New appends /wiki; CLI strips the routing /wiki prefix, so the
	// wire path is /wiki/api/v2/spaces on the httptest host.
	if sawPath != "/wiki/api/v2/spaces" {
		t.Errorf("path = %q, want /wiki/api/v2/spaces (site/wiki + /api/v2/spaces)", sawPath)
	}
	if !strings.Contains(out, `"results"`) {
		t.Errorf("stdout = %q", out)
	}
}

// TestAPI_WikiPassthroughIgnoresMirrorScope (GDK-1072): confluence in config
// scopes the mirror; `api` is the escape hatch past the mirror, so a /wiki
// path goes through on the credential alone. FAIL-first: this used to refuse
// with "Confluence is not enabled for this profile" and 0 network calls.
func TestAPI_WikiPassthroughIgnoresMirrorScope(t *testing.T) {
	hits := int32(0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Write([]byte(`{"results":[]}`))
	}))
	t.Cleanup(srv.Close)
	apiMirror(t, srv.URL, false) // no Confluence in config — mirror scope only

	out, _, err := captureErr(t, func() error {
		return cmdAPI([]string{"/wiki/api/v2/spaces"})
	})
	if err != nil {
		t.Fatalf("wiki passthrough with confluence off: %v", err)
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Errorf("hits = %d, want 1 (request must reach the configured site)", hits)
	}
	if !strings.Contains(out, `"results"`) {
		t.Errorf("stdout = %q", out)
	}
}

// --json is accepted (every sibling subcommand takes it, so hands type it by
// reflex — GDK-1072); the body is printed unchanged either way.
func TestAPI_JSONFlagAcceptedAsNoOp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":"1"}`))
	}))
	t.Cleanup(srv.Close)
	apiMirror(t, srv.URL, false)

	out, _, err := captureErr(t, func() error {
		return cmdAPI([]string{"GET", "/rest/api/3/myself", "--json"})
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if out != `{"id":"1"}` {
		t.Errorf("stdout = %q", out)
	}
}

func TestAPI_Non2xxExitAndStderr(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"errorMessages":["Issue does not exist"]}`))
	}))
	t.Cleanup(srv.Close)
	apiMirror(t, srv.URL, false)

	out, _, err := captureErr(t, func() error {
		return cmdAPI([]string{"/rest/api/3/issue/NOPE-1"})
	})
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "HTTP 404") {
		t.Errorf("err = %v", err)
	}
	if !strings.Contains(out, "Issue does not exist") {
		t.Errorf("stdout body = %q", out)
	}
}

func TestAPI_QueryEncoding(t *testing.T) {
	var rawQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawQuery = r.URL.RawQuery
		w.Write([]byte(`[]`))
	}))
	t.Cleanup(srv.Close)
	apiMirror(t, srv.URL, false)

	_, _, err := captureErr(t, func() error {
		return cmdAPI([]string{"/rest/api/3/user/search",
			"--query", "query=a b",
			"--query", "maxResults=10",
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	// Space must be percent-encoded (or +).
	if !strings.Contains(rawQuery, "query=a") || (!strings.Contains(rawQuery, "b") && !strings.Contains(rawQuery, "%20") && !strings.Contains(rawQuery, "+")) {
		t.Errorf("rawQuery = %q, want encoded space in query=", rawQuery)
	}
	vals := map[string]string{}
	for _, part := range strings.Split(rawQuery, "&") {
		k, v, _ := strings.Cut(part, "=")
		vals[k] = v
	}
	if vals["maxResults"] != "10" {
		t.Errorf("maxResults = %q", vals["maxResults"])
	}
	// "a b" → a+b or a%20b
	if vals["query"] != "a+b" && vals["query"] != "a%20b" {
		t.Errorf("query encoding = %q", vals["query"])
	}
}

func TestAPI_DataFileAndStdin(t *testing.T) {
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)
	apiMirror(t, srv.URL, false)

	dir := t.TempDir()
	path := filepath.Join(dir, "wl.json")
	if err := os.WriteFile(path, []byte(`{"from":"file"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	_, _, err := captureErr(t, func() error {
		return cmdAPI([]string{"POST", "/rest/api/3/issue/A-1/worklog",
			"--data", "@" + path, "--write"})
	})
	if err != nil {
		t.Fatalf("file: %v", err)
	}

	// stdin via temporarily replacing os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stdin
	os.Stdin = r
	go func() {
		_, _ = w.Write([]byte(`{"from":"stdin"}`))
		_ = w.Close()
	}()
	_, _, err = captureErr(t, func() error {
		return cmdAPI([]string{"POST", "/rest/api/3/issue/A-1/worklog",
			"--data", "-", "--write"})
	})
	os.Stdin = saved
	if err != nil {
		t.Fatalf("stdin: %v", err)
	}

	if len(bodies) != 2 || bodies[0] != `{"from":"file"}` || bodies[1] != `{"from":"stdin"}` {
		t.Errorf("bodies = %v", bodies)
	}
}

func TestAPI_UsageFlushed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)
	apiMirror(t, srv.URL, false)

	_, _, err := captureErr(t, func() error {
		return cmdAPI([]string{"/rest/api/3/myself"})
	})
	if err != nil {
		t.Fatal(err)
	}

	db, err := openStore()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	day := time.Now().UTC().Format("2006-01-02")
	rows, err := db.APIUsage(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	var found store.APIUsageDay
	for _, d := range rows {
		if d.Day == day {
			found = d
			break
		}
	}
	if found.Requests < 1 {
		// Also accept summary path if day row shape differs
		sum, err := db.APIUsageSummary(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if sum.Today.Requests < 1 {
			t.Fatalf("api_usage not accumulated: rows=%+v summary=%+v", rows, sum)
		}
	}
}

func TestAPI_StatusFlag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":1}`))
	}))
	t.Cleanup(srv.Close)
	apiMirror(t, srv.URL, false)

	out, errOut, err := captureErr(t, func() error {
		return cmdAPI([]string{"/rest/api/3/myself", "--status"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if out != `{"ok":1}` {
		t.Errorf("stdout = %q", out)
	}
	if !strings.Contains(errOut, "HTTP 200") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestAPI_DefaultMethodGET(t *testing.T) {
	var method string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	apiMirror(t, srv.URL, false)

	_, _, err := captureErr(t, func() error {
		return cmdAPI([]string{"/rest/api/3/myself"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if method != http.MethodGet {
		t.Errorf("method = %q", method)
	}
}

func TestAPI_FlagsAnyPosition(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)
	apiMirror(t, srv.URL, false)

	const (
		body = `{"timeSpent":"1h"}`
		path = "/rest/api/3/issue/A-1/worklog"
	)
	cases := []struct {
		name     string
		args     []string
		wantBody string
	}{
		{"after", []string{"PUT", path, "--write", "--data", body}, body},
		// Field incident 2026-08-15: flags before METHOD/PATH were a usage error.
		{"before", []string{"--write", "PUT", path, "--data", body}, body},
		{"between", []string{"PUT", "--write", path, "--data", body}, body},
		{"split", []string{"--write", "PUT", "--data", body, path}, body},
		// A PATH always starts with /; --data's value must not be taken as PATH.
		{"dataLooksLikePath", []string{"--data", "/not-the-path", "PUT", path, "--write"}, "/not-the-path"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotMethod, gotPath, gotBody = "", "", ""
			out, _, err := captureErr(t, func() error { return cmdAPI(tc.args) })
			if err != nil {
				t.Fatalf("cmdAPI %v: %v\n%s", tc.args, err, out)
			}
			if gotMethod != http.MethodPut || gotPath != path || gotBody != tc.wantBody {
				t.Fatalf("got %s %s body=%q want PUT %s body=%q", gotMethod, gotPath, gotBody, path, tc.wantBody)
			}
		})
	}
}

func TestAPI_DataStdinFlagBeforeMethod(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)
	apiMirror(t, srv.URL, false)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stdin
	os.Stdin = r
	go func() {
		_, _ = w.Write([]byte(`{"from":"stdin-before"}`))
		_ = w.Close()
	}()
	_, _, err = captureErr(t, func() error {
		return cmdAPI([]string{"--data", "-", "PUT", "/rest/api/3/issue/A-1/worklog", "--write"})
	})
	os.Stdin = saved
	if err != nil {
		t.Fatalf("stdin before METHOD: %v", err)
	}
	if gotBody != `{"from":"stdin-before"}` {
		t.Fatalf("body = %q", gotBody)
	}
}

func TestAPI_UnknownFlagErrors(t *testing.T) {
	// parseAround rejects unknown dashes and returns (not os.Exit) so a typo
	// is a usage error in any position.
	for _, args := range [][]string{
		{"--pretty", "GET", "/rest/api/3/myself"},
		{"GET", "--pretty", "/rest/api/3/myself"},
		{"GET", "/rest/api/3/myself", "--pretty"},
	} {
		_, _, err := captureErr(t, func() error { return cmdAPI(args) })
		if err == nil || !strings.Contains(err.Error(), "--pretty") {
			t.Fatalf("%v: want unknown --pretty, got %v", args, err)
		}
		if !strings.Contains(err.Error(), `run "gadak api --help"`) {
			t.Fatalf("%v: want usageError help pointer, got %v", args, err)
		}
	}
}

func TestParseAPIMethodPath(t *testing.T) {
	t.Parallel()
	cases := []struct {
		pos        []string
		wantMethod string
		wantPath   string
		wantErr    string
	}{
		{[]string{"/rest/api/3/myself"}, http.MethodGet, "/rest/api/3/myself", ""},
		{[]string{"PUT", "/rest/api/3/issue/A-1"}, "PUT", "/rest/api/3/issue/A-1", ""},
		{[]string{"put", "/x"}, "put", "/x", ""},
		{[]string{}, "", "", "usage:"},
		{[]string{"PUT"}, "", "", "path is required"},
		{[]string{"PUT", "rest/no-slash"}, "", "", "must start with /"},
		{[]string{"/already", "extra"}, "", "", "unexpected argument"},
		{[]string{"PUT", "/x", "extra"}, "", "", "unexpected argument"},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.pos, " "), func(t *testing.T) {
			t.Parallel()
			method, path, err := parseAPIMethodPath(tc.pos)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if method != tc.wantMethod || path != tc.wantPath {
				t.Fatalf("got %s %s want %s %s", method, path, tc.wantMethod, tc.wantPath)
			}
		})
	}
}
