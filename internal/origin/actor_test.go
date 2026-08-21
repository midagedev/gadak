package origin

// X-Issuetap-Actor (GDK-586, issuetap side GDK-588): a workspace shared by
// several agents attributes each write to the agent that made it. gadak
// resolves an actor (env GADAK_ACTOR > config.json actor > Claude Code
// auto-detection) and stamps it on requests that head for an issuetap
// origin — never on connected-workspace traffic (atlhttp stays clean).
//
// | Contract | Test |
// | --- | --- |
// | a standalone comment/transition is authored by the actor slug as an agent account | TestStandaloneWriteAttributesToActor |
// | no actor resolved → the seed user authors, byte-identical to before | TestStandaloneWriteWithoutActorKeepsSeedUser |
// | a connected client never carries the header, GADAK_ACTOR set or not | TestConnectedClientNeverCarriesActor |
// | the serve-routing transport stamps the same headers | TestServeTransportCarriesActor |

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
)

// standaloneActorWorkspace is origin_test.go's standalone fixture shape,
// with the ambient actor environment pinned so the ladder's outcome is the
// test's, not the invoking agent's (go test under Claude Code inherits
// CLAUDECODE=1; CI does not).
func standaloneActorWorkspace(t *testing.T) *config.Config {
	t.Helper()
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() {
		_ = Close()
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
	return cfg
}

// actorAuthorOf fetches the last comment's author through the origin GET
// path as raw JSON, so the assertion sees exactly what issuetap stored
// (accountId, accountType) rather than the jira.Client's projection.
func actorAuthorOf(t *testing.T, c *jira.Client, key string) map[string]any {
	t.Helper()
	status, body, err := c.Raw(context.Background(), http.MethodGet,
		fmt.Sprintf("/rest/api/3/issue/%s/comment", key), nil, false)
	if err != nil {
		t.Fatalf("GET comments: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("GET comments: status %d: %s", status, body)
	}
	var page struct {
		Comments []struct {
			Author map[string]any `json:"author"`
		} `json:"comments"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatalf("decode comments: %v", err)
	}
	if len(page.Comments) == 0 {
		t.Fatal("no comments stored")
	}
	return page.Comments[len(page.Comments)-1].Author
}

func TestStandaloneWriteAttributesToActor(t *testing.T) {
	t.Setenv("GADAK_ACTOR", "claude:354bff2b|Claude (build 1)")
	t.Setenv("CLAUDECODE", "") // env wins, but keep the ladder single-rung
	cfg := standaloneActorWorkspace(t)

	c, err := Client(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	key, err := c.CreateIssue(ctx, map[string]any{
		"project":   map[string]any{"key": DefaultProjectKey},
		"summary":   "actor attribution probe",
		"issuetype": map[string]any{"name": "Task"},
	})
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if _, err := c.AddComment(ctx, key, jira.Doc("agent note", nil), nil, false); err != nil {
		t.Fatalf("AddComment: %v", err)
	}
	author := actorAuthorOf(t, c, key)
	if got := author["accountId"]; got != "claude:354bff2b" {
		t.Fatalf("comment author accountId=%v, want the actor slug claude:354bff2b", got)
	}
	if got := author["displayName"]; got != "Claude (build 1)" {
		t.Fatalf("comment author displayName=%v, want X-Issuetap-Actor-Name", got)
	}
	if got := author["accountType"]; got != "agent" {
		t.Fatalf("comment author accountType=%v, want agent", got)
	}

	// A transition changelog entry carries the same actor.
	tts, err := c.Transitions(ctx, key)
	if err != nil {
		t.Fatalf("Transitions: %v", err)
	}
	if len(tts) == 0 {
		t.Fatal("no transitions available")
	}
	if err := c.Transition(ctx, key, tts[0].ID, nil, nil); err != nil {
		t.Fatalf("Transition: %v", err)
	}
	status, body, err := c.Raw(ctx, http.MethodGet,
		fmt.Sprintf("/rest/api/3/issue/%s?expand=changelog", key), nil, false)
	if err != nil {
		t.Fatalf("GET issue: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("GET issue: status %d: %s", status, body)
	}
	var doc struct {
		Changelog struct {
			Histories []struct {
				Author map[string]any `json:"author"`
			} `json:"histories"`
		} `json:"changelog"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("decode issue: %v", err)
	}
	if len(doc.Changelog.Histories) == 0 {
		t.Fatal("transition wrote no changelog")
	}
	last := doc.Changelog.Histories[len(doc.Changelog.Histories)-1]
	if got := last.Author["accountId"]; got != "claude:354bff2b" {
		t.Fatalf("changelog author accountId=%v, want the actor slug", got)
	}
}

func TestStandaloneWriteWithoutActorKeepsSeedUser(t *testing.T) {
	// Pin the whole ladder off: no env, no config, no Claude Code marker.
	// This is the byte-identical disabled path — the seed user authors.
	t.Setenv("GADAK_ACTOR", "")
	t.Setenv("CLAUDECODE", "")
	cfg := standaloneActorWorkspace(t)

	c, err := Client(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	key, err := c.CreateIssue(ctx, map[string]any{
		"project":   map[string]any{"key": DefaultProjectKey},
		"summary":   "seed user attribution probe",
		"issuetype": map[string]any{"name": "Task"},
	})
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if _, err := c.AddComment(ctx, key, jira.Doc("human note", nil), nil, false); err != nil {
		t.Fatalf("AddComment: %v", err)
	}
	author := actorAuthorOf(t, c, key)
	// issuetap's seed fixture user (store.go DefaultUser): Ada Lovelace,
	// accountId 5b10a2844c20165700ede21g. That is who authored before
	// GDK-586 and who must still author when the ladder resolves nothing.
	if got := author["accountId"]; got != "5b10a2844c20165700ede21g" {
		t.Fatalf("comment author accountId=%v, want the seed user 5b10a2844c20165700ede21g (no actor in play)", got)
	}
	if got := author["accountType"]; got == "agent" {
		t.Fatalf("comment author accountType=agent with no actor resolved; want the seed user")
	}
}

func TestConnectedClientNeverCarriesActor(t *testing.T) {
	// The outbound-cleanliness pin: a connected workspace's requests go
	// through atlhttp, which never sees the actor. GADAK_ACTOR set and
	// Claude Code detected must not leak the header to a real site.
	t.Setenv("GADAK_ACTOR", "claude:354bff2b")
	t.Setenv("CLAUDECODE", "1")
	t.Setenv("CLAUDE_CODE_SESSION_ID", "e3e6a49a-1382-4502-9381-2c89d3234d74")

	var mu sync.Mutex
	var sawActor, sawName bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		if r.Header.Get("X-Issuetap-Actor") != "" {
			sawActor = true
		}
		if r.Header.Get("X-Issuetap-Actor-Name") != "" {
			sawName = true
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accountId":"5b10a2844c20165700ede21g","displayName":"Ada Lovelace"}`))
	}))
	defer ts.Close()

	home := t.TempDir()
	t.Setenv("GADAK_HOME", filepath.Join(home, "profile"))
	config.SetProfile("")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Site, cfg.Email, cfg.Token = ts.URL, "you@example.com", "token"
	c, err := Client(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Myself(context.Background()); err != nil {
		t.Fatalf("Myself: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if sawActor || sawName {
		t.Fatal("connected workspace request carried X-Issuetap-Actor(-Name) — the header must never leave for a real site")
	}
}

func TestServeTransportCarriesActor(t *testing.T) {
	// The serve-routing transport (a live serve owns the persist) stamps
	// the same headers; the bearer rewrite and the actor are independent.
	t.Setenv("GADAK_ACTOR", "claude:354bff2b")
	var mu sync.Mutex
	var actor, actorName string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		actor = r.Header.Get("X-Issuetap-Actor")
		actorName = r.Header.Get("X-Issuetap-Actor-Name")
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer ts.Close()

	tr := newServeOriginTransport(ts.Listener.Addr().String())
	tr.bearer = "dev-token"
	tr.actor, tr.actorName = "claude:354bff2b", "Claude (build 1)"
	req, _ := http.NewRequest(http.MethodGet, "http://origin/rest/api/3/myself", nil)
	if _, err := tr.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if actor != "claude:354bff2b" {
		t.Fatalf("serve transport actor header = %q, want the slug", actor)
	}
	if actorName != "Claude (build 1)" {
		t.Fatalf("serve transport actor-name header = %q", actorName)
	}
}
