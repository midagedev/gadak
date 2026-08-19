package atlhttp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const testAuth = "Basic test-auth-value-not-a-real-token"

func testCfg(t *testing.T, h http.Handler) (Config, *Meter) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	m := &Meter{}
	return Config{
		Base:      srv.URL,
		Auth:      testAuth,
		HTTP:      srv.Client(),
		Retries:   3,
		Backoff:   0, // Retry-After "0" and empty both fall through to this
		ErrPrefix: "atlhttp",
		Usage:     m,
	}, m
}

func do(t *testing.T, cfg Config, method, path string, payload []byte, hasBody, mutating bool) (int, []byte, error) {
	t.Helper()
	return DoRaw(context.Background(), cfg, method, path, payload, hasBody, mutating)
}

func TestDoRawSuccessIsSingleAttempt(t *testing.T) {
	// A retried POST would create two issues. Success must not re-send.
	for _, tc := range []struct {
		name     string
		method   string
		mutating bool
		hasBody  bool
		payload  []byte
		wantType bool
	}{
		{name: "GET read", method: http.MethodGet, mutating: false},
		{name: "POST write", method: http.MethodPost, mutating: true, hasBody: true, payload: []byte(`{"summary":"x"}`), wantType: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls int
			var bodies []string
			cfg, meter := testCfg(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != tc.method {
					t.Errorf("method = %s, want %s", r.Method, tc.method)
				}
				if got := r.Header.Get("Authorization"); got != testAuth {
					t.Errorf("Authorization = %q", got)
				}
				if got := r.Header.Get("Accept"); got != "application/json" {
					t.Errorf("Accept = %q", got)
				}
				ct := r.Header.Get("Content-Type")
				if tc.wantType && ct != "application/json" {
					t.Errorf("Content-Type = %q, want application/json", ct)
				}
				if !tc.wantType && ct != "" {
					t.Errorf("Content-Type = %q, want empty", ct)
				}
				b, _ := io.ReadAll(r.Body)
				bodies = append(bodies, string(b))
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"ok":true}`))
			}))
			status, body, err := do(t, cfg, tc.method, "/rest/api/3/ok", tc.payload, tc.hasBody, tc.mutating)
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if status != 200 || string(body) != `{"ok":true}` {
				t.Fatalf("status=%d body=%s", status, body)
			}
			if calls != 1 {
				t.Errorf("calls = %d, want 1", calls)
			}
			if tc.hasBody && (len(bodies) != 1 || bodies[0] != string(tc.payload)) {
				t.Errorf("bodies = %q", bodies)
			}
			if got := meter.Snapshot().Requests; got != 1 {
				t.Errorf("Requests = %d, want 1", got)
			}
			if got := meter.Snapshot().Retries; got != 0 {
				t.Errorf("Retries = %d, want 0", got)
			}
		})
	}
}

func TestDoRawRetryPolicyByStatus(t *testing.T) {
	// Non-mutating retries 429/500/502/503/504 only (not every 5xx).
	// Mutating retries 429/503 only — a 500 may mean the write applied.
	// The method is irrelevant; Search is POST with mutating=false.
	type row struct {
		code      int
		mutating  bool
		wantCalls int
	}
	retries := 3
	cases := []row{
		{200, false, 1},
		{201, true, 1},
		{400, false, 1},
		{401, false, 1},
		{403, false, 1},
		{404, false, 1},
		{422, false, 1},
		{429, false, retries},
		{429, true, retries},
		{500, false, retries},
		{500, true, 1},
		{501, false, 1},
		{501, true, 1},
		{502, false, retries},
		{502, true, 1},
		{503, false, retries},
		{503, true, retries},
		{504, false, retries},
		{504, true, 1},
		{505, false, 1},
	}
	for _, tc := range cases {
		name := http.StatusText(tc.code)
		if name == "" {
			name = "status"
		}
		if tc.mutating {
			name += " mutating"
		} else {
			name += " read"
		}
		t.Run(name, func(t *testing.T) {
			var calls int
			cfg, _ := testCfg(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(tc.code)
				_, _ = w.Write([]byte(`{"error":true}`))
			}))
			cfg.Retries = retries
			status, body, err := do(t, cfg, http.MethodPost, "/rest/api/3/x", []byte(`{}`), true, tc.mutating)
			if err != nil {
				t.Fatalf("completed HTTP must return err=nil; err=%v", err)
			}
			if status != tc.code {
				t.Errorf("status = %d, want %d", status, tc.code)
			}
			if string(body) != `{"error":true}` {
				t.Errorf("body = %s", body)
			}
			if calls != tc.wantCalls {
				t.Errorf("calls = %d, want %d", calls, tc.wantCalls)
			}
		})
	}
}

func TestDoRawRetriesThenSucceedsSameBody(t *testing.T) {
	var calls int
	var bodies []string
	payload := []byte(`{"summary":"once"}`)
	cfg, meter := testCfg(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		if calls < 3 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"1"}`))
	}))
	status, body, err := do(t, cfg, http.MethodPost, "/rest/api/3/issue", payload, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if status != 201 || string(body) != `{"id":"1"}` {
		t.Fatalf("status=%d body=%s", status, body)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
	for i, b := range bodies {
		if b != string(payload) {
			t.Errorf("attempt %d body = %q, want re-send of original payload", i+1, b)
		}
	}
	u := meter.Snapshot()
	if u.Requests != 3 {
		t.Errorf("Requests = %d, want 3", u.Requests)
	}
	if u.Retries != 2 {
		t.Errorf("Retries = %d, want 2", u.Retries)
	}
	if u.ServerErrors != 2 { // two 503s; the 201 is not 5xx
		t.Errorf("ServerErrors = %d, want 2", u.ServerErrors)
	}
}

func TestDoRawAttemptCapReturnsLastStatus(t *testing.T) {
	// A finished HTTP response is never an error, even after the attempt cap.
	// err names a status only at the jira/confluence call() layer, not here.
	var calls int
	cfg, meter := testCfg(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`last-body`))
	}))
	cfg.Retries = 4
	status, body, err := do(t, cfg, http.MethodGet, "/rest/api/3/status", nil, false, false)
	if err != nil {
		t.Fatalf("err = %v, want nil (status is returned, not wrapped)", err)
	}
	if status != 500 {
		t.Errorf("status = %d, want 500", status)
	}
	if string(body) != "last-body" {
		t.Errorf("body = %q", body)
	}
	if calls != 4 {
		t.Errorf("calls = %d, want 4 (Retries is total attempts)", calls)
	}
	if got := meter.Snapshot().Retries; got != 3 {
		t.Errorf("Retries = %d, want 3", got)
	}
}

func TestDoRawRetryAfterPositiveSecondsIsHonored(t *testing.T) {
	// Retry-After is integer seconds (Atoi). A 1s header with Backoff=0 would
	// sleep a full second if we waited it out. Instead: if the header is
	// honored, wait() blocks until the short context deadline; if it is
	// ignored, d<=0 and the loop finishes all attempts with err==nil.
	var calls int
	cfg, meter := testCfg(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`slow down`))
	}))
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	status, _, err := DoRaw(ctx, cfg, http.MethodGet, "/rest/api/3/status", nil, false, false)
	elapsed := time.Since(start)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v (status %d), want DeadlineExceeded — Retry-After:1 must beat Backoff=0", err, status)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (cancelled during the first wait)", calls)
	}
	if elapsed < 100*time.Millisecond {
		t.Errorf("elapsed %s: wait returned too fast to have used Retry-After:1", elapsed)
	}
	if elapsed > 2*time.Second {
		t.Errorf("elapsed %s: should have returned at the 200ms deadline, not slept the full second", elapsed)
	}
	u := meter.Snapshot()
	if u.Throttled != 1 {
		t.Errorf("Throttled = %d, want 1", u.Throttled)
	}
	if u.Retries != 0 {
		t.Errorf("Retries = %d, want 0 (wait was cancelled before noteRetry)", u.Retries)
	}
	if u.WaitMS <= 0 {
		t.Errorf("WaitMS = %d, want > 0 from the cancelled wait", u.WaitMS)
	}
}

func TestDoRawRetryAfterZeroUsesBackoff(t *testing.T) {
	// s > 0 is required; "0" and HTTP-date fall through to Backoff << attempt.
	var calls int
	cfg, _ := testCfg(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`ok`))
	}))
	cfg.Backoff = time.Millisecond
	start := time.Now()
	status, body, err := do(t, cfg, http.MethodGet, "/ok", nil, false, false)
	if err != nil || status != 200 || string(body) != "ok" {
		t.Fatalf("status=%d body=%s err=%v", status, body, err)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Errorf("Retry-After 0 must not sleep a whole second")
	}
}

func TestDoRawTransportErrorRetry(t *testing.T) {
	// Transport errors retry only when !mutating. The write path must not
	// replay a POST that may already have been applied.
	t.Run("read retries then succeeds", func(t *testing.T) {
		// atomic: ErrAbortHandler finishes on the server goroutine; the
		// test reads the counter after DoRaw returns (same shape as
		// TestDoRawContextCancelStopsRetryWait).
		var calls atomic.Int32
		cfg, meter := testCfg(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if calls.Add(1) == 1 {
				panic(http.ErrAbortHandler)
			}
			_, _ = w.Write([]byte(`recovered`))
		}))
		status, body, err := do(t, cfg, http.MethodGet, "/rest/api/3/myself", nil, false, false)
		if err != nil {
			t.Fatal(err)
		}
		if status != 200 || string(body) != "recovered" {
			t.Fatalf("status=%d body=%s", status, body)
		}
		if n := calls.Load(); n != 2 {
			t.Errorf("calls = %d, want 2", n)
		}
		if got := meter.Snapshot().Retries; got != 1 {
			t.Errorf("Retries = %d, want 1", got)
		}
	})
	t.Run("write does not retry", func(t *testing.T) {
		var calls atomic.Int32
		cfg, _ := testCfg(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			panic(http.ErrAbortHandler)
		}))
		_, _, err := do(t, cfg, http.MethodPost, "/rest/api/3/issue", []byte(`{}`), true, true)
		if err == nil {
			t.Fatal("want transport error")
		}
		if !strings.Contains(err.Error(), "POST") || !strings.Contains(err.Error(), "/rest/api/3/issue") {
			t.Errorf("err = %v, want method and path", err)
		}
		if strings.Contains(err.Error(), testAuth) {
			t.Error("error leaked Authorization")
		}
		if n := calls.Load(); n != 1 {
			t.Errorf("calls = %d, want 1", n)
		}
	})
	t.Run("read exhausts attempts", func(t *testing.T) {
		var calls atomic.Int32
		cfg, _ := testCfg(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			panic(http.ErrAbortHandler)
		}))
		cfg.Retries = 3
		_, _, err := do(t, cfg, http.MethodGet, "/rest/api/3/myself", nil, false, false)
		if err == nil {
			t.Fatal("want transport error after cap")
		}
		if !strings.Contains(err.Error(), "GET") || !strings.Contains(err.Error(), "/rest/api/3/myself") {
			t.Errorf("err = %v, want method and path", err)
		}
		if n := calls.Load(); n != 3 {
			t.Errorf("calls = %d, want 3", n)
		}
	})
}

func TestDoRawRejectsOffSitePaths(t *testing.T) {
	var calls int
	cfg, meter := testCfg(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		t.Errorf("must not send %s", r.URL)
	}))
	cases := []struct {
		path    string
		wantSub string
	}{
		{"https://evil.example/steal", "absolute"},
		{"http://evil.example/steal", "absolute"},
		{"HTTPS://evil.example/steal", "absolute"},
		{"//evil.example/steal", "absolute"},
		{"relative/path", "must start with /"},
		{"", "path is required"},
	}
	for _, tc := range cases {
		status, body, err := do(t, cfg, http.MethodGet, tc.path, nil, false, false)
		if err == nil {
			t.Errorf("%q: want error, got status=%d body=%s", tc.path, status, body)
			continue
		}
		if !strings.Contains(err.Error(), tc.wantSub) {
			t.Errorf("%q: err = %v, want substring %q", tc.path, err, tc.wantSub)
		}
		if strings.Contains(err.Error(), testAuth) {
			t.Errorf("%q: error leaked Authorization", tc.path)
		}
	}
	if calls != 0 {
		t.Errorf("rejected paths left the process: calls=%d", calls)
	}
	if got := meter.Snapshot().Requests; got != 0 {
		t.Errorf("Requests = %d, want 0", got)
	}
}

func TestDoRawRejectsUserinfoInResolvedURL(t *testing.T) {
	var calls int
	cfg, _ := testCfg(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
	}))
	u := strings.TrimPrefix(cfg.Base, "http://")
	cfg.Base = "http://user:pass@" + u
	_, _, err := do(t, cfg, http.MethodGet, "/ok", nil, false, false)
	if err == nil || !strings.Contains(err.Error(), "userinfo") {
		t.Fatalf("err = %v, want userinfo refusal", err)
	}
	if calls != 0 {
		t.Errorf("calls = %d, want 0", calls)
	}
}

func TestDoRawBadBaseUsesErrPrefix(t *testing.T) {
	cfg := Config{Base: "://not a url", HTTP: &http.Client{}, ErrPrefix: "jira"}
	_, _, err := do(t, cfg, http.MethodGet, "/ok", nil, false, false)
	if err == nil || !strings.Contains(err.Error(), "jira: bad site URL") {
		t.Fatalf("err = %v, want prefixed parse error", err)
	}
}

func TestDoRawContextCancelStopsRetryWait(t *testing.T) {
	var calls atomic.Int32
	started := make(chan struct{})
	cfg, _ := testCfg(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			close(started)
		}
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	cfg.Backoff = 2 * time.Second // would stall the suite if cancel is ignored
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-started
		cancel()
	}()
	start := time.Now()
	_, _, err := DoRaw(ctx, cfg, http.MethodGet, "/rest/api/3/status", nil, false, false)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want Canceled", err)
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("calls = %d, want 1", n)
	}
	if time.Since(start) > time.Second {
		t.Errorf("cancel did not stop the 2s backoff promptly")
	}
}

func TestDoRawContextCancelDuringRequest(t *testing.T) {
	entered := make(chan struct{})
	cfg, _ := testCfg(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done()
	}))
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-entered
		cancel()
	}()
	start := time.Now()
	_, _, err := DoRaw(ctx, cfg, http.MethodGet, "/hang", nil, false, false)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want Canceled", err)
	}
	if time.Since(start) > time.Second {
		t.Errorf("in-flight cancel took too long")
	}
}

func TestDoRawAlreadyCanceledContext(t *testing.T) {
	var calls int
	cfg, _ := testCfg(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := DoRaw(ctx, cfg, http.MethodGet, "/ok", nil, false, false)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want Canceled", err)
	}
	if calls != 0 {
		t.Errorf("calls = %d, want 0", calls)
	}
}

func TestDoRawNilUsageIsSafe(t *testing.T) {
	cfg, _ := testCfg(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`ok`))
	}))
	cfg.Usage = nil
	status, body, err := do(t, cfg, http.MethodGet, "/ok", nil, false, false)
	if err != nil || status != 200 || string(body) != "ok" {
		t.Fatalf("status=%d body=%s err=%v", status, body, err)
	}
}

func TestDoRawRetriesZeroStillSendsOnce(t *testing.T) {
	// Retries is "total attempts"; 0 still enters the loop once.
	var calls int
	cfg, _ := testCfg(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	cfg.Retries = 0
	status, _, err := do(t, cfg, http.MethodGet, "/ok", nil, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if status != 500 {
		t.Errorf("status = %d", status)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

func TestDoRawReadErrorOn2xx(t *testing.T) {
	cfg, _ := testCfg(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	cfg.HTTP = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(errReader{}),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})}
	_, _, err := do(t, cfg, http.MethodGet, "/ok", nil, false, false)
	if err == nil || !strings.Contains(err.Error(), "GET") {
		t.Fatalf("err = %v, want wrapped read error", err)
	}
}

func TestDoRawReadErrorOnNon2xxStillReturnsStatus(t *testing.T) {
	cfg, _ := testCfg(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	cfg.HTTP = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 500,
			Body:       io.NopCloser(errReader{}),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})}
	cfg.Retries = 1
	status, _, err := do(t, cfg, http.MethodGet, "/ok", nil, false, false)
	if err != nil {
		t.Fatalf("non-2xx read error must not become err; got %v", err)
	}
	if status != 500 {
		t.Errorf("status = %d", status)
	}
}

func TestSnippet(t *testing.T) {
	if got := Snippet([]byte("  hi\n")); got != "hi" {
		t.Errorf("trim = %q", got)
	}
	s := strings.Repeat("a", 400)
	if got := Snippet([]byte(s)); got != s {
		t.Errorf("400-char body must not grow an ellipsis")
	}
	if got := Snippet([]byte(s + "b")); got != s+"…" {
		t.Errorf("truncate = len %d", len(got))
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }
