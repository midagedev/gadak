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
	authStatus int // non-zero: /myself answers with it
	// syncStatus, when non-zero, is what the sync trio (/status, /priority,
	// /search/jql) answers with — how a token that verified at connect time is
	// made dead (401) or the site flaky (5xx) before a sync runs.
	syncStatus int
	hold       chan struct{} // non-nil: first /search/jql waits on it
	held       chan struct{} // closed once that search is waiting
	projects   string        // /project/search body
}

// failSync answers the request with f.syncStatus when one is armed. A 5xx gets
// Retry-After: 1 so the transport's five attempts cost ~4s of fixed waits
// instead of 1+2+4+8s of exponential backoff — the error class is unchanged,
// only the wait between retries is pinned.
func (f *onboardJira) failSync(w http.ResponseWriter) bool {
	if f.syncStatus == 0 {
		return false
	}
	if f.syncStatus >= 500 {
		w.Header().Set("Retry-After", "1")
	}
	w.WriteHeader(f.syncStatus)
	return true
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
			if f.failSync(w) {
				return
			}
			_, _ = w.Write([]byte(`[{"id":"3","name":"진행 중","statusCategory":{"key":"indeterminate"}}]`))
		case path == "/priority":
			if f.failSync(w) {
				return
			}
			_, _ = w.Write([]byte(`[{"id":"2","name":"High"}]`))
		case path == "/search/jql":
			if f.failSync(w) {
				return
			}
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

// onboarding hands back a server whose configuration is saveable (GADAK_HOME in a
// temp dir). Each New() gets a fresh per-instance sync job/activity slot.
func onboarding(t *testing.T) (*onboardJira, http.Handler, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	f := newOnboardJira(t)
	db, cfg := fixture(t)
	cfg.Site, cfg.Email, cfg.Token = "", "", ""
	cfg.Projects = nil

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
	if saved["tokenExpirySource"] != "assumed" {
		t.Fatalf("skipped date should assume: %v", saved["tokenExpirySource"])
	}
	if saved["tokenExpiresAt"] == nil || saved["tokenExpiresAt"] == "" {
		t.Fatal("assumed tokenExpiresAt missing")
	}
}

func TestConnectStoresUserExpiry(t *testing.T) {
	f, h, home := onboarding(t)
	rec := send(t, h, http.MethodPut, apiBase+"onboarding/connect/",
		`{"site":"`+f.URL+`","jira_email":"hc@example.com","api_token":"tok-secret","token_expires_at":"2027-03-01"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	saved := savedConfig(t, home)
	if saved["tokenExpirySource"] != "user" {
		t.Fatalf("source %v, want user", saved["tokenExpirySource"])
	}
	if saved["tokenExpiresAt"] != "2027-03-01T00:00:00.000Z" {
		t.Fatalf("expires %v", saved["tokenExpiresAt"])
	}
}

func TestConnectRejectsInvalidExpiry(t *testing.T) {
	f, h, home := onboarding(t)
	rec := send(t, h, http.MethodPut, apiBase+"onboarding/connect/",
		`{"site":"`+f.URL+`","jira_email":"hc@example.com","api_token":"tok-secret","token_expires_at":"soon"}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_token_expires") {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(home, "config.json")); !os.IsNotExist(err) {
		t.Fatalf("invalid expiry must not write config: %v", err)
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

// TestConnectNamesTheTokenTrapItCanRecognise pins which of the three
// connect-time token traps the 401 tells apart. Jira answers all three
// identically, so the only evidence is what was pasted: an organization key
// gives itself away by its ATCTT prefix (STATE_OF_PLAY.md "hard-won knowledge"
// #1), while a scoped token carries no prefix of its own and therefore has to
// share the generic code with a plain wrong password — the copy behind that
// code is what names the scoped case. Equality, not Contains: the org-key code
// has the generic one as a substring, so Contains would pass without
// distinguishing anything.
func TestConnectNamesTheTokenTrapItCanRecognise(t *testing.T) {
	for _, tc := range []struct{ name, token, code string }{
		{"organization key", "ATCTT" + strings.Repeat("x", 30), "credential_rejected_org_key"},
		{"scoped token", "ATATT" + strings.Repeat("y", 30), "credential_rejected"},
		{"plain wrong token", "nope", "credential_rejected"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, h, _ := onboarding(t)
			f.authStatus = http.StatusUnauthorized

			rec := send(t, h, http.MethodPut, apiBase+"onboarding/connect/",
				`{"site":"`+f.URL+`","jira_email":"hc@example.com","api_token":"`+tc.token+`"}`)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
			if got := decode[map[string]string](t, rec)["error"]; got != tc.code {
				t.Fatalf("error code %q, want %q", got, tc.code)
			}
			if strings.Contains(rec.Body.String(), tc.token) {
				t.Fatal("the rejected token was echoed back to the client")
			}
		})
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
	// Scope change with a credential starts a full sync automatically — that is
	// the first flight. POST /sync/ while it runs must still 409.
	if rec := send(t, h, http.MethodPut, apiBase+"settings/", `{"projects":["NMB"]}`); rec.Code != http.StatusOK {
		t.Fatalf("settings: %d %s", rec.Code, rec.Body.String())
	}
	<-f.held // the auto-started run is inside its first search

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
	// Scope change auto-kicks a full sync (seeds the watermark); wait before
	// probing incremental mode so we do not 409 against that job.
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

// The polled progress document must classify a dead credential as a code, not
// leave it as prose. The two tests below decode the raw JSON with their own
// struct (not progressDoc) so the same assertion compiles — and fails — against
// a server that does not classify yet.
func TestSyncProgressClassifiesARejectedCredential(t *testing.T) {
	f, h, _ := onboarding(t)
	connect(t, h, f)
	// The token verified at connect, then died — the read-only user's situation.
	f.syncStatus = http.StatusUnauthorized

	if rec := send(t, h, http.MethodPost, apiBase+"sync/", ""); rec.Code != http.StatusAccepted {
		t.Fatalf("start: %d %s", rec.Code, rec.Body.String())
	}
	phase, errText, errCode := waitCodes(t, h)
	if phase != "error" || errText == "" {
		t.Fatalf("job should fail hard: phase %q error %q", phase, errText)
	}
	if errCode != "credential_rejected" {
		t.Fatalf("error_code %q, want credential_rejected — a dead token must be a code the client can act on", errCode)
	}
}

// A transport-class failure (here: the source answering 500) must NOT carry the
// credential code: the mirror must not tell the user to replace a working token.
func TestSyncProgressLeavesATransportFailureUnclassified(t *testing.T) {
	f, h, _ := onboarding(t)
	connect(t, h, f)
	f.syncStatus = http.StatusInternalServerError

	if rec := send(t, h, http.MethodPost, apiBase+"sync/", ""); rec.Code != http.StatusAccepted {
		t.Fatalf("start: %d %s", rec.Code, rec.Body.String())
	}
	phase, errText, errCode := waitCodes(t, h)
	if phase != "error" || errText == "" {
		t.Fatalf("job should fail hard: phase %q error %q", phase, errText)
	}
	if errCode != "" {
		t.Fatalf("error_code %q on a transport failure — a 500 is not a rejected credential", errCode)
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

// waitCodes is waitDone for the classification fields, decoded from raw JSON so
// the assertion does not depend on progressDoc growing the field first.
func waitCodes(t *testing.T, h http.Handler) (phase, errText, errCode string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		rec := get(t, h, apiBase+"sync/progress/", nil)
		var doc struct {
			Running   bool   `json:"running"`
			Phase     string `json:"phase"`
			Error     string `json:"error"`
			ErrorCode string `json:"error_code"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
			t.Fatalf("decode progress: %v", err)
		}
		if !doc.Running && doc.Phase != "syncing" {
			return doc.Phase, doc.Error, doc.ErrorCode
		}
		if time.Now().After(deadline) {
			t.Fatalf("sync did not finish: %+v", doc)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
