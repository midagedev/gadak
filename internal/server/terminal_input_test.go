package server

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/midagedev/gadak/internal/pairing"
)

// ⑰ GDK-1162: the input route places a command in a shell — it does not run
// one.
//
// The ruling this file pins is a trust boundary, not a UI habit: an issue
// body is written by whoever could edit the issue (a teammate, an agent), so
// one click may put a command *in front of a person* but may never execute
// it. A client that only promised to be careful would be undone by the next
// caller, so the promise is the server's: a payload carrying \n or \r is 400
// and nothing reaches the PTY. Enter stays a keystroke a human makes.
//
// FAIL-first (2026-08-30, before the route existed): every case below
// answered `404 {"error":"not_found"}` from the sub-mux catch-all —
// `place: 404 {"error":"not_found"}`.
func TestTerminalInputPlacesTextWithoutRunningIt(t *testing.T) {
	srv, _, _ := termServer(t)
	id := createSession(t, srv, "", "")

	c, _, err := dialTerminal(t, srv, id, "", "")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.CloseNow()

	// The rejected payload is a command with a newline in it — the exact
	// shape the ruling forbids. It carries its own marker so the assertion
	// below is "this never reached the shell", not "nothing happened".
	code, body := termRequest(t, srv, http.MethodPost, termBase+"sessions/"+id+"/input/",
		`{"text":"echo GDK1162REJECTED\n"}`, "", "")
	if code != http.StatusBadRequest || !strings.Contains(body, "input_not_a_line") {
		t.Fatalf("newline payload: %d %s; want 400 input_not_a_line", code, body)
	}
	// \r is the same door: it is the byte a terminal turns into Enter.
	code, body = termRequest(t, srv, http.MethodPost, termBase+"sessions/"+id+"/input/",
		`{"text":"echo GDK1162REJECTED\r"}`, "", "")
	if code != http.StatusBadRequest || !strings.Contains(body, "input_not_a_line") {
		t.Fatalf("carriage-return payload: %d %s; want 400 input_not_a_line", code, body)
	}

	// A clean line is placed, and the shell's own echo is the witness that
	// it arrived. printf with a %s argument keeps the *placed* text and the
	// text it would print distinct, so the two halves of the contract are
	// separable below.
	const placed = `printf 'GDK1162%s\n' -RAN`
	payload, err := json.Marshal(map[string]string{"text": placed})
	if err != nil {
		t.Fatal(err)
	}
	code, body = termRequest(t, srv, http.MethodPost, termBase+"sessions/"+id+"/input/", string(payload), "", "")
	if code != http.StatusOK {
		t.Fatalf("place: %d %s", code, body)
	}
	var res struct {
		Placed int `json:"placed"`
	}
	if err := json.Unmarshal([]byte(body), &res); err != nil {
		t.Fatalf("place body %s: %v", body, err)
	}
	if res.Placed != len(placed) {
		t.Fatalf("placed = %d, want %d (%s)", res.Placed, len(placed), body)
	}

	got := readUntilMarker(t, c, "-RAN")
	// Ordering assertion, not a bare absence: by the time the placed line has
	// echoed, a rejected payload that had reached the PTY would have echoed
	// too — it was sent first.
	if strings.Contains(got, "GDK1162REJECTED") {
		t.Fatalf("a rejected payload reached the shell: %q", got)
	}
	// And the line is only sitting at the prompt: printf's own output is the
	// proof of execution, and it must not be there.
	if strings.Contains(got, "GDK1162-RAN") {
		t.Fatalf("the placed line ran without an Enter: %q", got)
	}

	// The human presses Enter. Now — and only now — it runs. This is the
	// positive half: the placed text really was a pending command line, not
	// decoration.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := c.Write(ctx, websocket.MessageBinary, []byte("\r")); err != nil {
		t.Fatal(err)
	}
	readUntilMarker(t, c, "GDK1162-RAN")
}

// ⑱ The bounds and the doors, in the shape the neighbor routes use. Nothing
// here is a new permission axis: same gate, same ownership rule, same 404 for
// a session another credential opened.
//
// FAIL-first (2026-08-30): `over the cap: 404 {"error":"not_found"}`.
func TestTerminalInputBoundsAndDoors(t *testing.T) {
	srv, _, _ := termServer(t)
	id := createSession(t, srv, "", "")
	path := termBase + "sessions/" + id + "/input/"

	// Over the length cap: refused by size, before anything is placed.
	long, err := json.Marshal(map[string]string{"text": strings.Repeat("x", terminalInputMax+1)})
	if err != nil {
		t.Fatal(err)
	}
	code, body := termRequest(t, srv, http.MethodPost, path, string(long), "", "")
	if code != http.StatusBadRequest || !strings.Contains(body, "input_too_long") {
		t.Fatalf("over the cap: %d %s; want 400 input_too_long", code, body)
	}
	// Exactly at the cap is fine — the bound is a limit, not an off-by-one.
	atCap, err := json.Marshal(map[string]string{"text": strings.Repeat("x", terminalInputMax)})
	if err != nil {
		t.Fatal(err)
	}
	if code, body = termRequest(t, srv, http.MethodPost, path, string(atCap), "", ""); code != http.StatusOK {
		t.Fatalf("at the cap: %d %s; want 200", code, body)
	}

	// An empty text is not a command. This route exists to place one.
	if code, body = termRequest(t, srv, http.MethodPost, path, `{"text":""}`, "", ""); code != http.StatusBadRequest ||
		!strings.Contains(body, "invalid_body") {
		t.Fatalf("empty text: %d %s; want 400 invalid_body", code, body)
	}
	// A malformed body is a 400 before the session is consulted.
	if code, body = termRequest(t, srv, http.MethodPost, path, `not json`, "", ""); code != http.StatusBadRequest ||
		!strings.Contains(body, "invalid_body") {
		t.Fatalf("bad body: %d %s; want 400 invalid_body", code, body)
	}

	// An unknown session is 404, the same shape DELETE and the issue binding
	// answer — never a 403 that would confirm someone else's session exists.
	code, body = termRequest(t, srv, http.MethodPost, termBase+"sessions/deadbeef/input/", `{"text":"ls"}`, "", "")
	if code != http.StatusNotFound || !strings.Contains(body, "not_found") {
		t.Fatalf("unknown session: %d %s; want 404 not_found", code, body)
	}

	// Gate and ownership parity with the neighbors (⑬ and ⑥).
	srv2, _, dir := termServer(t)
	seedStore(t, dir, seedToken{"pane", pairing.ScopeTerminal})
	code, body = termRequest(t, srv2, http.MethodPost, termBase+"sessions/abc123/input/", `{"text":"ls"}`, remoteIPHost, "")
	if code != http.StatusUnauthorized || !strings.Contains(body, "pairing_rejected") {
		t.Fatalf("non-local without bearer: %d %s; want 401 pairing_rejected (the gate)", code, body)
	}
	toks := seedStore(t, dir,
		seedToken{"pane-a", pairing.ScopeTerminal},
		seedToken{"pane-b", pairing.ScopeTerminal})
	idA := createSession(t, srv2, mirrorHost, toks[0])
	if code, _ = termRequest(t, srv2, http.MethodPost, termBase+"sessions/"+idA+"/input/", `{"text":"ls"}`, mirrorHost, toks[1]); code != http.StatusNotFound {
		t.Fatalf("another token placed text in a session it did not open: %d; want 404", code)
	}
	if code, body = termRequest(t, srv2, http.MethodPost, termBase+"sessions/"+idA+"/input/", `{"text":"ls"}`, mirrorHost, toks[0]); code != http.StatusOK {
		t.Fatalf("own token place: %d %s; want 200", code, body)
	}
}

// ⑲ The structural half of GDK-1164: this surface reads liveness, and never
// writes it back.
//
// Detecting that no shell is on an issue is one layer; deciding the claim is
// dead is another, and gadak does not own it — the origin does. An automatic
// unclaim here would reproduce the very defect it was meant to fix: a laptop
// asleep for ten minutes is a serve that knows nothing, and a serve that
// wrote its ignorance to the origin would delete the claim of an agent still
// running on another machine.
//
// So the terminal surface is pinned as read-only with respect to the origin:
// its handlers may reach the session manager and nothing that writes. This is
// a source assertion because it is a claim about *absence*, and absence is
// exactly what a behavioural test cannot see — a future handler that grew an
// origin write would pass every case above.
func TestTerminalSurfaceNeverWritesToOrigin(t *testing.T) {
	raw, err := os.ReadFile("terminal.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	// The write verbs of the origin surface (internal/server/write.go) and
	// the mirror. Names, not shapes: this fails loudly on a rename, which is
	// the moment to re-read this comment rather than delete the line.
	for _, verb := range []string{
		"originWriter", "origin.Writer", "SetAssignee", "Transition(",
		"transitionIssue", "s.store.", "db.Exec", "UPDATE issues",
	} {
		if strings.Contains(src, verb) {
			t.Errorf("internal/server/terminal.go mentions %q — the terminal surface must not write to the origin or the mirror (GDK-1164: detecting death is not the same layer as recording it)", verb)
		}
	}
}
