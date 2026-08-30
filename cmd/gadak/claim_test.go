package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
)

// TestClaimStandaloneTwoActors is the GDK-591 round-trip on a standalone
// workspace, against the real embedded origin: actor A claims, actor B's
// claim is refused with a distinguishable exit code and A's display name,
// B takes over only when asking for it, A finishes the issue, and
// `gadak issue` then shows the wait/progress spans computed from the
// changelog. No network, no real site.
func TestClaimStandaloneTwoActors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	// The actor ladder would otherwise pick up this very Claude Code session
	// (CLAUDECODE=1 under `go test` run from an agent) and both "actors"
	// would be the same account — the exact parallel-work hazard claim
	// exists for. GADAK_ACTOR wins, but keep the ladder single-rung.
	t.Setenv("GADAK_ACTOR", "agent-a|Agent A")
	t.Setenv("CLAUDECODE", "")
	t.Setenv("CLAUDE_CODE_SESSION_ID", "")
	clearCredentialEnv(t)
	config.SetProfile("")
	t.Cleanup(func() {
		_ = origin.Close()
		config.SetProfile("")
	})

	if _, err := capture(t, func() error { return cmdInit([]string{"--standalone", "--json"}) }); err != nil {
		t.Fatalf("init --standalone: %v", err)
	}
	created, err := capture(t, func() error { return cmdCreate([]string{"claim roundtrip"}) })
	if err != nil {
		t.Fatalf("create: %v\n%s", err, created)
	}
	key := strings.Split(strings.TrimSpace(strings.Split(created, "\n")[0]), "\t")[0]

	// A claims: one atomic call, and the write-through confirmation line
	// carries the new assignee.
	out, err := capture(t, func() error { return cmdClaim([]string{key}) })
	if err != nil {
		t.Fatalf("agent A claim: %v\n%s", err, out)
	}
	if !strings.Contains(out, key) || !strings.Contains(out, "Agent A") {
		t.Fatalf("claim line lacks key or actor: %q", strings.TrimSpace(out))
	}

	// A claims again: idempotent on the origin, still a success — and the
	// --json form carries the claim answer next to the refreshed issue.
	if _, err := capture(t, func() error { return cmdClaim([]string{key}) }); err != nil {
		t.Fatalf("agent A re-claim (idempotent): %v", err)
	}
	claimJSON, err := capture(t, func() error { return cmdClaim([]string{key, "--json"}) })
	if err != nil {
		t.Fatalf("claim --json: %v\n%s", err, claimJSON)
	}
	if !strings.Contains(claimJSON, `"claim"`) || !strings.Contains(claimJSON, `"atomic":true`) {
		t.Fatalf("claim --json lacks the claim answer:\n%s", claimJSON)
	}

	// B claims: refused — exit 75, not 1, and A's display name is in the
	// message so the human (and the agent) sees who holds it.
	asActor(t, "agent-b|Agent B")
	_, err = capture(t, func() error { return cmdClaim([]string{key}) })
	if err == nil {
		t.Fatal("agent B claim over A's hold succeeded without --take-over")
	}
	if code := exitStatus(err); code != exitClaimConflict {
		t.Fatalf("agent B claim exit = %d, want %d (%v)", code, exitClaimConflict, err)
	}
	if !strings.Contains(err.Error(), "already claimed by Agent A") {
		t.Fatalf("refusal lacks the holder's name: %v", err)
	}

	// B takes over: succeeds, and the confirmation line carries B.
	out, err = capture(t, func() error { return cmdClaim([]string{key, "--take-over"}) })
	if err != nil {
		t.Fatalf("agent B take-over: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Agent B") {
		t.Fatalf("take-over line lacks actor B: %q", strings.TrimSpace(out))
	}

	// A finishes the issue; `gadak issue` then shows both spans. The wait
	// was created -> A's claim; the progress runs to A's done transition.
	asActor(t, "agent-a|Agent A")
	if _, err := capture(t, func() error { return cmdTransition([]string{key, "done"}) }); err != nil {
		t.Fatalf("agent A done: %v", err)
	}
	issue, err := capture(t, func() error { return cmdIssue([]string{key}) })
	if err != nil {
		t.Fatalf("issue after done: %v\n%s", err, issue)
	}
	var line string
	for _, l := range strings.Split(issue, "\n") {
		if strings.HasPrefix(l, "durations ") {
			line = l
		}
	}
	if line == "" {
		t.Fatalf("issue output lacks a durations line:\n%s", issue)
	}
	if !strings.Contains(line, "wait ") || !strings.Contains(line, "progress ") {
		t.Fatalf("durations line lacks a span: %q", line)
	}
}

// asActor switches the acting account between origin sessions. The actor is
// resolved when a session is constructed, so switching means flushing the
// cached session first — the same thing a second CLI process gets for free.
func asActor(t *testing.T, actor string) {
	t.Helper()
	if err := origin.Close(); err != nil {
		t.Fatalf("origin.Close between actors: %v", err)
	}
	t.Setenv("GADAK_ACTOR", actor)
}

// TestClaimBindsToTerminalSession is GDK-1158: a claim typed inside a gadak
// pane reflects itself into the session the pane's shell runs in, so the
// serve can say which issue a terminal is for. The pane side is emulated
// with the two things the real one provides — GADAK_TERMINAL_SESSION in the
// environment (Manager.Create sets it) and a loopback serve answering the
// binding route — plus the three rules that make the reflection safe: it
// follows the confirmation, never fails the claim, and never happens for a
// claim the origin refused.
func TestClaimBindsToTerminalSession(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	t.Setenv("GADAK_ACTOR", "agent-a|Agent A")
	t.Setenv("CLAUDECODE", "")
	t.Setenv("CLAUDE_CODE_SESSION_ID", "")
	clearCredentialEnv(t)
	config.SetProfile("")
	t.Cleanup(func() {
		_ = origin.Close()
		config.SetProfile("")
	})

	if _, err := capture(t, func() error { return cmdInit([]string{"--standalone", "--json"}) }); err != nil {
		t.Fatalf("init --standalone: %v", err)
	}
	created, err := capture(t, func() error { return cmdCreate([]string{"bind roundtrip"}) })
	if err != nil {
		t.Fatalf("create: %v\n%s", err, created)
	}
	key := strings.Split(strings.TrimSpace(strings.Split(created, "\n")[0]), "\t")[0]

	// The fake serve: it answers the binding route and records what reached
	// it, which is the whole assertion surface for the POST's shape.
	const session = "4f9c1a2b3c4d5e6f7a8b9c0d1e2f3a4b"
	var posts []string
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		posts = append(posts, r.Method+" "+r.URL.Path+" "+string(body))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(fake.Close)

	savedDiscover := discoverServes
	t.Cleanup(func() { discoverServes = savedDiscover })
	discoverServes = func() []serveHit {
		return []serveHit{{base: fake.URL, profile: "", port: "7877", source: serveSourceRun}}
	}
	t.Setenv("GADAK_TERMINAL_SESSION", session)

	// A landed claim binds: one POST to this session's route, a line after
	// the confirmation row, and the full id in --json beside the claim.
	out, err := capture(t, func() error { return cmdClaim([]string{key}) })
	if err != nil {
		t.Fatalf("claim with a session env: %v\n%s", err, out)
	}
	if want := "POST /api/v1/terminal/sessions/" + session + "/issue/ " + `{"issue_key":"` + key + `"}`; len(posts) != 1 || posts[0] != want {
		t.Fatalf("binding POSTs: %v; want [%s]", posts, want)
	}
	bound := "bound to session " + session[:8] + "…"
	if !strings.Contains(out, bound) {
		t.Fatalf("claim output lacks %q:\n%s", bound, out)
	}
	if i, j := strings.Index(out, key), strings.Index(out, bound); i < 0 || j < i {
		t.Fatalf("the bound line must follow the confirmation row:\n%s", out)
	}
	claimJSON, err := capture(t, func() error { return cmdClaim([]string{key, "--json"}) })
	if err != nil {
		t.Fatalf("claim --json with a session env: %v\n%s", err, claimJSON)
	}
	var jsonBody struct {
		Session string         `json:"session"`
		Claim   map[string]any `json:"claim"`
	}
	if err := json.Unmarshal([]byte(claimJSON), &jsonBody); err != nil {
		t.Fatalf("claim --json: %v\n%s", err, claimJSON)
	}
	if jsonBody.Session != session {
		t.Fatalf("claim --json session = %q; want %s\n%s", jsonBody.Session, session, claimJSON)
	}
	if jsonBody.Claim == nil {
		t.Fatalf("claim --json lost the claim answer:\n%s", claimJSON)
	}

	// Rule 3: a claim survives a serve that is not there. The origin write
	// already landed; the binding is best-effort and quiet.
	discoverServes = func() []serveHit { return nil }
	out, err = capture(t, func() error { return cmdClaim([]string{key}) })
	if err != nil {
		t.Fatalf("claim must survive a missing serve: %v\n%s", err, out)
	}
	if strings.Contains(out, "bound to session") {
		t.Fatalf("no serve, yet a bound line printed:\n%s", out)
	}

	// Rule 5: a claim the origin refused never binds. B's claim over A's
	// hold is exit 75, and the serve that is right there sees no POST.
	discoverServes = func() []serveHit {
		return []serveHit{{base: fake.URL, profile: "", port: "7877", source: serveSourceRun}}
	}
	asActor(t, "agent-b|Agent B")
	before := len(posts)
	out, err = capture(t, func() error { return cmdClaim([]string{key}) })
	if code := exitStatus(err); code != exitClaimConflict {
		t.Fatalf("agent B claim exit = %d, want %d (%v)", code, exitClaimConflict, err)
	}
	if len(posts) != before {
		t.Fatalf("a refused claim still bound: %v", posts[before:])
	}
	if strings.Contains(out, "bound to session") {
		t.Fatalf("a refused claim printed a bound line:\n%s", out)
	}
}

// TestClaimConnectedFallback is the Cloud shape: the fake answers the claim
// route with 404 (Atlassian has none), so the CLI judges locally and writes
// the two calls the atomic route fuses — and says on stderr that the claim
// was not atomic. The mirror fixture holds NMB-1 in progress as Dana, so a
// bare claim is refused with Dana's name; --take-over replaces her without
// re-transitioning; a not-yet-in-progress issue transitions first.
func TestClaimConnectedFallback(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	// Held by Dana, in progress: refused locally — same exit 75, same
	// sentence shape as the origin's own 409.
	_, _, err := captureBoth(t, func() error { return cmdClaim([]string{"NMB-1"}) })
	if err == nil {
		t.Fatal("claim over Dana's hold succeeded without --take-over")
	}
	if code := exitStatus(err); code != exitClaimConflict {
		t.Fatalf("refused exit = %d, want %d (%v)", code, exitClaimConflict, err)
	}
	if !strings.Contains(err.Error(), "already claimed by Dana Whitfield") {
		t.Fatalf("refusal lacks the holder's name: %v", err)
	}

	// Take-over: assignee PUT only (already in progress), and the
	// no-atomicity warning lands on stderr, not in the pipeable stdout.
	_, stderr, err := captureBoth(t, func() error { return cmdClaim([]string{"NMB-1", "--take-over"}) })
	if err != nil {
		t.Fatalf("take-over: %v (stderr %s)", err, stderr)
	}
	if !strings.Contains(stderr, "no atomic claim") {
		t.Fatalf("stderr lacks the atomicity warning: %q", stderr)
	}
	if !f.called("PUT /issue/NMB-1/assignee") {
		t.Fatal("take-over never wrote the assignee")
	}
	if f.called("POST /issue/NMB-1/transitions") {
		t.Fatal("take-over of an in-progress issue re-transitioned")
	}

	// A not-yet-in-progress issue: transition first, then assign — the
	// order the atomic route applies, so a rejected transition leaves the
	// assignee untouched.
	f.issueStatusJSON = `{"fields":{"status":{"id":"1","name":"To Do","statusCategory":{"key":"new"}},"assignee":null}}`
	_, stderr, err = captureBoth(t, func() error { return cmdClaim([]string{"NMB-1"}) })
	if err != nil {
		t.Fatalf("fresh claim: %v (stderr %s)", err, stderr)
	}
	if !strings.Contains(stderr, "no atomic claim") {
		t.Fatalf("stderr lacks the atomicity warning: %q", stderr)
	}
	ti, ai, lastClaim := -1, -1, -1
	for i, c := range f.calls {
		if c == "POST /issue/NMB-1/claim" {
			lastClaim = i
		}
	}
	for i := lastClaim + 1; i < len(f.calls); i++ {
		if f.calls[i] == "POST /issue/NMB-1/transitions" && ti < 0 {
			ti = i
		}
		if f.calls[i] == "PUT /issue/NMB-1/assignee" && ai < 0 {
			ai = i
		}
	}
	if ti < 0 || ai < 0 || ti > ai {
		t.Fatalf("fresh claim must transition before assignee; calls: %v", f.calls)
	}
}
