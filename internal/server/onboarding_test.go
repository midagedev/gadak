package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// onboardJira is the slice of Jira the onboarding endpoints touch: /myself for
// step 1, /project/search for step 2, and the search/metadata calls a full sync
// makes for step 3. The first search can be held open, which is how the
// single-flight test keeps a run in progress deterministically.
type onboardJira struct {
	*httptest.Server
	authStatus int           // non-zero: /myself answers with it
	hold       chan struct{} // non-nil: first /search/jql waits on it
	held       chan struct{} // closed once that search is waiting
	projects   string        // /project/search body
}

func newOnboardJira(t *testing.T) *onboardJira {
	t.Helper()
	f := &onboardJira{projects: `{"values":[
		{"key":"NMB","name":"Numbers","projectTypeKey":"software"},
		{"key":"OPS","name":"Operations","projectTypeKey":"service_desk"}
	],"isLast":true,"total":2}`}
	searches := 0
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/rest/api/3")
		w.Header().Set("Content-Type", "application/json")
		switch {
		case path == "/myself":
			if f.authStatus != 0 {
				w.WriteHeader(f.authStatus)
				return
			}
			_, _ = w.Write([]byte(`{"accountId":"acc-hc","displayName":"김현철","emailAddress":"hc@example.com"}`))
		case strings.HasPrefix(path, "/project/search"):
			_, _ = w.Write([]byte(f.projects))
		case path == "/status":
			_, _ = w.Write([]byte(`[{"id":"3","name":"진행 중","statusCategory":{"key":"indeterminate"}}]`))
		case path == "/priority":
			_, _ = w.Write([]byte(`[{"id":"2","name":"High"}]`))
		case path == "/search/jql":
			searches++
			if searches == 1 && f.hold != nil {
				close(f.held)
				<-f.hold
			}
			_, _ = w.Write([]byte(`{"issues":[{"id":"1001","key":"NMB-1","fields":{
				"summary":"batch worker drops the last page",
				"status":{"id":"3","name":"진행 중","statusCategory":{"key":"indeterminate"}},
				"project":{"key":"NMB"},"issuetype":{"id":"10004","name":"Bug"},
				"created":"2026-07-01T00:00:00.000+0900","updated":"2026-08-04T12:00:00.000+0900"
			}}],"isLast":true}`))
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

// onboarding hands back a server whose configuration is saveable (SCRY_HOME in a
// temp dir) and a reset progress job, since the job is process-wide.
func onboarding(t *testing.T) (*onboardJira, http.Handler, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("SCRY_HOME", home)
	f := newOnboardJira(t)
	db, cfg := fixture(t)
	cfg.Site, cfg.Email, cfg.Token = "", "", ""
	cfg.Projects = nil

	syncMu.Lock()
	syncJob = progressDoc{}
	syncMu.Unlock()

	return f, New(db, cfg), home
}

func savedConfig(t *testing.T, home string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(home, "config.json"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	return out
}

func TestConnectVerifiesStoresSiteAndHidesTheToken(t *testing.T) {
	f, h, home := onboarding(t)

	// A trailing slash is what a pasted browser URL carries.
	rec := send(t, h, http.MethodPut, apiBase+"onboarding/connect/",
		`{"site":"`+f.URL+`/","jira_email":"hc@example.com","api_token":"tok-secret"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); strings.Contains(body, "tok-secret") {
		t.Fatalf("token leaked into the response: %s", body)
	}
	var doc credentialDoc
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !doc.Configured || doc.DisplayName != "김현철" || doc.JiraEmail != "hc@example.com" {
		t.Fatalf("credential doc %+v", doc)
	}

	saved := savedConfig(t, home)
	if got, want := saved["site"], f.URL; got != want {
		t.Fatalf("stored site %v, want %v", got, want)
	}
	if saved["token"] != "tok-secret" {
		t.Fatalf("token not stored in the config file")
	}
}

func TestNormalizeSiteAcceptsWhatPeoplePaste(t *testing.T) {
	for in, want := range map[string]string{
		"example.atlassian.net":          "https://example.atlassian.net",
		"https://example.atlassian.net/": "https://example.atlassian.net",
		" http://localhost:8080/x ":      "http://localhost:8080",
		"::nope":                         "",
		"ftp://example.atlassian.net":    "",
		"":                               "",
	} {
		if got := normalizeSite(in); got != want {
			t.Errorf("normalizeSite(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestConnectReportsARejectedCredential(t *testing.T) {
	f, h, _ := onboarding(t)
	f.authStatus = http.StatusUnauthorized

	rec := send(t, h, http.MethodPut, apiBase+"onboarding/connect/",
		`{"site":"`+f.URL+`","jira_email":"hc@example.com","api_token":"nope"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "credential_rejected") {
		t.Fatalf("body %s", rec.Body.String())
	}
}

func TestConnectRequiresSiteAndCredential(t *testing.T) {
	f, h, _ := onboarding(t)
	for _, tc := range []struct{ name, body, code string }{
		{"no site", `{"jira_email":"a@b.c","api_token":"t"}`, "site_required"},
		{"unparseable site", `{"site":"::nope","jira_email":"a@b.c","api_token":"t"}`, "site_required"},
		{"no token", `{"site":"` + f.URL + `","jira_email":"a@b.c"}`, "email_and_token_required"},
	} {
		rec := send(t, h, http.MethodPut, apiBase+"onboarding/connect/", tc.body)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), tc.code) {
			t.Errorf("%s: status %d body %s", tc.name, rec.Code, rec.Body.String())
		}
	}
}

func TestAvailableProjectsNeedsACredential(t *testing.T) {
	_, h, _ := onboarding(t)
	rec := get(t, h, apiBase+"projects/available/", nil)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "credential_required") {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestAvailableProjectsListsTheSite(t *testing.T) {
	f, h, _ := onboarding(t)
	connect(t, h, f)

	rec := get(t, h, apiBase+"projects/available/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Projects []struct {
			Key     string `json:"key"`
			Name    string `json:"name"`
			TypeKey string `json:"projectTypeKey"`
		} `json:"projects"`
		Truncated bool `json:"truncated"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Projects) != 2 || body.Projects[0].Key != "NMB" ||
		body.Projects[1].Name != "Operations" || body.Projects[0].TypeKey != "software" {
		t.Fatalf("projects %+v", body.Projects)
	}
	if body.Truncated {
		t.Errorf("a two-project site reported as truncated")
	}
}

func TestStartSyncRefusesAnIncompleteSetup(t *testing.T) {
	f, h, _ := onboarding(t)

	rec := send(t, h, http.MethodPost, apiBase+"sync/", "")
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "credential_required") {
		t.Fatalf("no credential: status %d body %s", rec.Code, rec.Body.String())
	}

	// A credential alone is a complete setup now: empty projects means "every
	// project this account can see", so the sync must start, not 400.
	connect(t, h, f)
	rec = send(t, h, http.MethodPost, apiBase+"sync/", "")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("no projects should still sync: status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestFirstSyncIsSingleFlightAndReportsProgress(t *testing.T) {
	f, h, _ := onboarding(t)
	f.hold, f.held = make(chan struct{}), make(chan struct{})
	connect(t, h, f)
	if rec := send(t, h, http.MethodPut, apiBase+"settings/", `{"projects":["NMB"]}`); rec.Code != http.StatusOK {
		t.Fatalf("settings: %d %s", rec.Code, rec.Body.String())
	}

	if rec := send(t, h, http.MethodPost, apiBase+"sync/", ""); rec.Code != http.StatusAccepted {
		t.Fatalf("start: %d %s", rec.Code, rec.Body.String())
	}
	<-f.held // the run is inside its first search

	if p := progress(t, h); !p.Running || p.Phase != "syncing" || p.StartedAt == "" {
		t.Fatalf("progress while running: %+v", p)
	}
	rec := send(t, h, http.MethodPost, apiBase+"sync/", "")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "sync_in_progress") {
		t.Fatalf("second start: %d %s", rec.Code, rec.Body.String())
	}

	close(f.hold)
	p := waitDone(t, h)
	if !p.Done || p.Phase != "done" || p.Running || p.Error != "" {
		t.Fatalf("final progress: %+v", p)
	}
	if p.Fetched != 1 || p.FinishedAt == "" {
		t.Fatalf("counters: %+v", p)
	}
}

func TestSyncProgressStartsIdle(t *testing.T) {
	_, h, _ := onboarding(t)
	if p := progress(t, h); p.Running || p.Phase != "idle" || p.Done || p.Fetched != 0 {
		t.Fatalf("progress %+v", p)
	}
}

func TestStartSyncAcceptsIncrementalMode(t *testing.T) {
	f, h, _ := onboarding(t)
	connect(t, h, f)
	if rec := send(t, h, http.MethodPut, apiBase+"settings/", `{"projects":["NMB"]}`); rec.Code != http.StatusOK {
		t.Fatalf("settings: %d %s", rec.Code, rec.Body.String())
	}
	// Seed a watermark so incremental is not forced full.
	if rec := send(t, h, http.MethodPost, apiBase+"sync/", `{"mode":"full"}`); rec.Code != http.StatusAccepted {
		t.Fatalf("full start: %d %s", rec.Code, rec.Body.String())
	}
	_ = waitDone(t, h)

	if rec := send(t, h, http.MethodPost, apiBase+"sync/", `{"mode":"incremental"}`); rec.Code != http.StatusAccepted {
		t.Fatalf("incremental start: %d %s", rec.Code, rec.Body.String())
	}
	p := waitDone(t, h)
	if p.Phase == "error" {
		t.Fatalf("incremental failed: %+v", p)
	}

	if rec := send(t, h, http.MethodPost, apiBase+"sync/", `{"mode":"bogus"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bogus mode: %d %s", rec.Code, rec.Body.String())
	}
}

func connect(t *testing.T, h http.Handler, f *onboardJira) {
	t.Helper()
	rec := send(t, h, http.MethodPut, apiBase+"onboarding/connect/",
		`{"site":"`+f.URL+`","jira_email":"hc@example.com","api_token":"tok"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("connect: %d %s", rec.Code, rec.Body.String())
	}
}

func progress(t *testing.T, h http.Handler) progressDoc {
	t.Helper()
	rec := get(t, h, apiBase+"sync/progress/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("progress: %d %s", rec.Code, rec.Body.String())
	}
	var p progressDoc
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode progress: %v", err)
	}
	return p
}

func waitDone(t *testing.T, h http.Handler) progressDoc {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		p := progress(t, h)
		if !p.Running && p.Phase != "syncing" {
			return p
		}
		if time.Now().After(deadline) {
			t.Fatalf("sync did not finish: %+v", p)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
