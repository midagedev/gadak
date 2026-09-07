package atlhttp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestAuthErrorUnwrapsAndNamesSource(t *testing.T) {
	jiraErr := Auth("jira")
	confErr := Auth("confluence")
	if jiraErr.Error() != "jira: credential rejected" {
		t.Fatalf("jira Auth = %q", jiraErr)
	}
	if confErr.Error() != "confluence: credential rejected" {
		t.Fatalf("confluence Auth = %q", confErr)
	}
	if !errors.Is(jiraErr, ErrAuth) {
		t.Fatal("Auth(jira) must unwrap to ErrAuth")
	}
	if !errors.Is(confErr, ErrAuth) {
		t.Fatal("Auth(confluence) must unwrap to ErrAuth")
	}
	if errors.Is(jiraErr, confErr) {
		t.Fatal("Auth(jira) must not match Auth(confluence)")
	}
	wrapped := fmt.Errorf("GET /x: %w (401 Unauthorized)", jiraErr)
	if !errors.Is(wrapped, jiraErr) {
		t.Fatal("wrapped Auth(jira) must match Auth(jira)")
	}
	if !errors.Is(wrapped, ErrAuth) {
		t.Fatal("wrapped Auth(jira) must match ErrAuth")
	}
	if errors.Is(wrapped, confErr) {
		t.Fatal("wrapped Auth(jira) must not match Auth(confluence)")
	}
	var rc RejectedCredential
	if !errors.As(jiraErr, &rc) {
		t.Fatal("AuthError must implement RejectedCredential")
	}
	if Auth("").Error() != ErrAuth.Error() {
		t.Fatalf("empty prefix = %q, want %q", Auth(""), ErrAuth)
	}
}

func TestAuthFromStatus(t *testing.T) {
	for _, code := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		err := authFromStatus(code, "jira")
		if err == nil {
			t.Fatalf("status %d: want Auth", code)
		}
		if !errors.Is(err, ErrAuth) {
			t.Fatalf("status %d: %v must unwrap to ErrAuth", code, err)
		}
		if err.Error() != "jira: credential rejected" {
			t.Fatalf("status %d: %q", code, err)
		}
	}
	for _, code := range []int{200, 400, 404, 429, 500} {
		if err := authFromStatus(code, "jira"); err != nil {
			t.Fatalf("status %d: got %v, want nil", code, err)
		}
	}
}

func TestDoClassifies401And403(t *testing.T) {
	for _, code := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			cfg, _ := testCfg(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, `{"message":"Client must be authenticated"}`, code)
			}))
			cfg.ErrPrefix = "third"
			cfg.Retries = 1
			status, _, err := Do(context.Background(), cfg, http.MethodGet, "/rest/ping", nil, false, false)
			if err == nil {
				t.Fatal("Do must return ErrAuth on 401/403")
			}
			if status != code {
				t.Errorf("status = %d, want %d", status, code)
			}
			if !errors.Is(err, ErrAuth) {
				t.Fatalf("err = %v, want ErrAuth", err)
			}
			if !strings.Contains(err.Error(), "third:") {
				t.Fatalf("err = %q, want third:", err)
			}
			if !strings.Contains(err.Error(), "GET") || !strings.Contains(err.Error(), "/rest/ping") {
				t.Fatalf("err = %q, want method and path", err)
			}
			if strings.Contains(err.Error(), testAuth) {
				t.Error("error leaked Authorization")
			}
		})
	}
}

func TestDoLeavesNonAuthStatusToCaller(t *testing.T) {
	cfg, _ := testCfg(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`boom`))
	}))
	cfg.Retries = 1
	status, body, err := Do(context.Background(), cfg, http.MethodGet, "/rest/ping", nil, false, false)
	if err != nil {
		t.Fatalf("500 must stay a status, not ErrAuth; err=%v", err)
	}
	if status != 500 || string(body) != "boom" {
		t.Fatalf("status=%d body=%s", status, body)
	}
}

func TestDoRawStillNilOn401(t *testing.T) {
	// Raw / gadak api inspect status themselves. DoRaw must not start
	// returning ErrAuth or those callers lose the body.
	cfg, _ := testCfg(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":true}`))
	}))
	cfg.Retries = 1
	status, body, err := DoRaw(context.Background(), cfg, http.MethodGet, "/rest/ping", nil, false, false)
	if err != nil {
		t.Fatalf("DoRaw on 401 must stay err=nil; err=%v", err)
	}
	if status != 401 || string(body) != `{"error":true}` {
		t.Fatalf("status=%d body=%s", status, body)
	}
}
