package server

/*
 * GDK-804: a pairing token never reaches a log line, a response body, or a
 * response header — on any branch of any gate.
 *
 * The bearer is the phone's whole identity: a serve with `tailscale serve`
 * in front of it writes its log where an operator (and anything shipping
 * that log elsewhere) can read it, so one Printf with the credential in it
 * hands out the pairing. internal/pairing/offer.go already keeps the offer
 * payload out of its errors; this is the same rule one layer up, where the
 * token is actually presented.
 *
 * Structure of the guard, in two layers:
 *
 *  ① TestPairingTokenNeverReachesTheLog drives every gate branch that can
 *    log — accept, wrong scope, unknown token, tokens-gone, unpaired host,
 *    unreadable store, the terminal door — with a real plaintext bearer and
 *    asserts the plaintext is absent from the captured log, from every
 *    response body, and from every response header. Behavioral, so a new
 *    log line on an existing branch is caught by the branch's own case.
 *
 *  ② TestGateLogsNeverFormatTheBearer reads the package source and fails if
 *    any log call in a non-test file takes bearerToken(...) or the raw
 *    Authorization header as an argument. Behavioral coverage only reaches
 *    the branches this file drives; a brand-new branch with a brand-new
 *    Printf would sail past ① until someone thought to add a case. The
 *    source rule has no such hole, and it names the one extractor
 *    (bearerToken) that the gates agree is where a credential comes from.
 *
 * What the gates DO log is the token's *label* and *scope* (origin_rest.go,
 * mirror_gate.go) and, in terminal.go, the first eight characters of the
 * SHA-256 hash of a token — never the token. Those stay: an operator has to
 * be able to tell which pairing was denied. ① pins that distinction by
 * asserting the label survives on the scope-rejection branch while the
 * bearer does not, so a future "redact everything" round cannot quietly
 * make the log useless and still pass.
 */

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/pairing"
)

// loopbackHost is the Host shape the local web UI and the CLI present — the
// one allowedHost admits without any exemption, so a case using it exercises
// the gate on its loopback path rather than the tailnet one.
const loopbackHost = "127.0.0.1:7777"

// captureLog redirects the standard logger into a buffer for the duration of
// the test. The gates all write through the package-level log functions, so
// this is the same sink `gadak serve` prints to.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prevOut, prevFlags, prevPrefix := log.Writer(), log.Flags(), log.Prefix()
	log.SetOutput(&buf)
	t.Cleanup(func() {
		log.SetOutput(prevOut)
		log.SetFlags(prevFlags)
		log.SetPrefix(prevPrefix)
	})
	return &buf
}

// TestPairingTokenNeverReachesTheLog drives each gate branch with a real
// bearer and asserts the plaintext never surfaces.
func TestPairingTokenNeverReachesTheLog(t *testing.T) {
	h, cfg := builtInServer(t)
	toks := seedStore(t, cfg.Directory(), seedToken{"phone", "serve"}, seedToken{"laptop", "origin"})
	serve, laptop := toks[0], toks[1]
	// Never stored, so it exercises the "unknown token" reason path.
	unknown := "seed-token-ghost-serve-never-minted"

	cases := []struct {
		name   string
		method string
		path   string
		token  string
		host   string
	}{
		// Accept on the mirror REST: the branch that rewrites nothing and
		// logs nothing, driven so a future log line on the happy path is
		// caught here rather than in production.
		{"mirror accept", "GET", "/api/v1/issues/bootstrap/", serve, mirrorHost},
		// Wrong scope on the mirror REST: mirror_gate.go logs label+scope.
		{"mirror wrong scope", "GET", "/api/v1/issues/bootstrap/", laptop, mirrorHost},
		// Unknown token: pairing.Explain runs and failPairing answers with a
		// reason header — the branch most likely to echo what it was given.
		{"mirror unknown token", "GET", "/api/v1/issues/bootstrap/", unknown, mirrorHost},
		// The passthrough gate, both directions.
		{"passthrough accept", "GET", origin.RESTPrefix + "/rest/api/3/myself", laptop, ""},
		{"passthrough wrong scope", "GET", origin.RESTPrefix + "/rest/api/3/myself", serve, ""},
		{"passthrough unknown token", "GET", origin.RESTPrefix + "/rest/api/3/myself", unknown, ""},
		// The terminal door: a serve token must not open a shell, and the
		// refusal must not carry the token it refused.
		{"terminal refused", "GET", "/api/v1/terminal/sessions/", serve, mirrorHost},
		{"terminal refused loopback", "POST", "/api/v1/terminal/sessions/", serve, ""},
		// A path the guard never exempts, presented with a good bearer.
		{"non-api path", "GET", "/config.json", serve, mirrorHost},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			buf := captureLog(t)
			host := tc.host
			if host == "" {
				host = loopbackHost
			}
			rec := sendWithHost(t, h, tc.method, tc.path, "", bearer(tc.token), host)
			body, header := rec.Body.String(), rec.Result().Header
			logged := buf.String()
			for _, probe := range []struct{ where, text string }{
				{"log", logged},
				{"response body", body},
				{"response headers", headerDump(header)},
			} {
				if strings.Contains(probe.text, tc.token) {
					t.Errorf("%s: the bearer plaintext appears in the %s:\n%s",
						tc.name, probe.where, probe.text)
				}
			}
		})
	}

	// The counterweight: the wrong-scope branch must still say WHICH pairing
	// was denied, or the log stops being usable and nobody notices. Asserted
	// here so "redact more" can never be the whole answer.
	buf := captureLog(t)
	rec := sendWithHost(t, h, "GET", "/api/v1/issues/bootstrap/", "", bearer(laptop), mirrorHost)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("origin token on mirror REST: %d; want 403", rec.Code)
	}
	if logged := buf.String(); !strings.Contains(logged, `"laptop"`) || !strings.Contains(logged, `"origin"`) {
		t.Errorf("the scope refusal must name the label and scope it denied; got:\n%s", logged)
	}
}

// TestTerminalTokenLogIsHashedNotPlaintext pins the one place that prints a
// token-shaped identifier: terminal.go's session reaper logs `id[:8]` where
// id is the SHA-256 hex of the token. The comment there says so; this makes
// it measurable, because a later refactor that passes the plaintext into the
// same call site would keep the comment and change the meaning.
func TestTerminalTokenLogIsHashedNotPlaintext(t *testing.T) {
	token := "seed-token-phone-serve"
	sum := sha256.Sum256([]byte(token))
	id := hex.EncodeToString(sum[:])
	if strings.HasPrefix(token, id[:8]) || strings.Contains(token, id[:8]) {
		t.Fatalf("the hash prefix %q is a substring of the token; the reaper's log would leak", id[:8])
	}
	if len(id) != 64 {
		t.Fatalf("token id is %d hex chars, want 64 (SHA-256)", len(id))
	}
}

// logCall matches the opening of a log.Printf/Print/Println/Fatalf/… call.
// The scan below follows the argument list across line breaks: gofmt wraps a
// long Printf after a comma, so the leaking argument is usually NOT on the
// line that names log.Printf. Measured while building this file — the
// FAIL-first probe put `bearerToken(r)` on the continuation line and a
// single-line matcher stayed green while the behavioral test went red.
var logCall = regexp.MustCompile(`\blog\.(?:Printf|Print|Println|Fatalf|Fatal|Fatalln|Panicf|Panic)\(`)

// credentialArg matches the two ways a raw bearer can reach a format
// argument: the package's one extractor, and the header it comes from.
var credentialArg = regexp.MustCompile(`bearerToken\(|Header\.Get\("Authorization"\)|r\.Header\["Authorization"\]`)

// TestGateLogsNeverFormatTheBearer is the source-level half of the guard:
// no log call in a non-test file of this package may take the raw credential
// as an argument. Behavioral coverage above reaches only the branches it
// drives; this reaches every branch that exists.
func TestGateLogsNeverFormatTheBearer(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatal(err)
		}
		checked++
		// depth > 0 means we are inside the argument list of a log call that
		// started on an earlier line.
		depth, startLine, call := 0, 0, ""
		for i, line := range strings.Split(string(src), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") {
				continue
			}
			if depth == 0 {
				loc := logCall.FindStringIndex(line)
				if loc == nil {
					continue
				}
				// Start counting at the call's own open paren.
				depth, startLine, call = 0, i+1, ""
				line = line[loc[1]-1:]
			}
			call += trimmed + " "
			if credentialArg.MatchString(line) {
				t.Errorf("%s:%d formats a raw credential into a log line — log the label, the scope, or a hash of the token, never the token:\n\t%s",
					name, startLine, strings.TrimSpace(call))
				depth = 0
				continue
			}
			depth += strings.Count(line, "(") - strings.Count(line, ")")
			if depth <= 0 {
				depth = 0
			}
		}
	}
	// A glob that silently matched nothing would make this test a
	// permanently green no-op.
	if checked < 10 {
		t.Fatalf("only %d non-test .go files scanned; the source guard is not reading the package", checked)
	}
}

// TestPairedCredentialSurfaceIsMeasured characterizes what a serve-scope
// pairing can do to the workspace's *site* credential (`credential/`), so
// the answer is written down whichever way it is ruled.
//
// GDK-804 asked for a 403 here. It cannot be implemented in this round:
// TestMirrorGateCredentialHintedNoToken pins GET at 200 under an attributed
// product-owner ruling — "2026-08-25 — GDK-883 (product owner): serve scope
// opens the whole mirror REST; only the passthrough and non-API paths stay
// closed" — and TestMirrorAllowlistTable lists the same row. Flipping an
// attributed assertion is not a delegate's call, so this test asserts
// today's behavior instead of the requested behavior, and the report carries
// the conflict to the lead.
//
// The part worth seeing before ruling: GET only leaks a four-character hint,
// but PUT accepts a *plaintext Jira API token* for the workspace and DELETE
// erases the stored one. If the ruling stands for reads it does not
// obviously stand for those two.
func TestPairedCredentialSurfaceIsMeasured(t *testing.T) {
	_, h, cfg := mirrorWritableServer(t)
	token := seedStore(t, cfg.Directory(), seedToken{"phone", "serve"})[0]
	for _, tc := range []struct {
		method, body string
		want         int
		note         string
	}{
		{"GET", "", http.StatusOK, "hinted document (GDK-883 ruling)"},
		{"DELETE", "", http.StatusOK, "erases the workspace's stored site credential"},
		{"PUT", `{"jira_email":"","api_token":""}`, http.StatusBadRequest, "reached the handler: 400 is the body check, not a refusal by the gate"},
	} {
		rec := sendWithHost(t, h, tc.method, "/api/v1/issues/credential/", tc.body, bearer(token), mirrorHost)
		if rec.Code != tc.want {
			t.Errorf("%s credential/ through a serve pairing: %d %s; want %d (%s)",
				tc.method, rec.Code, rec.Body.String(), tc.want, tc.note)
		}
	}
	// Whatever the ruling, the stored token itself never crosses the wire.
	rec := sendWithHost(t, h, "GET", "/api/v1/issues/credential/", "", bearer(token), mirrorHost)
	if stored := cfg.Token; stored != "" && strings.Contains(rec.Body.String(), stored) {
		t.Error("GET credential/ echoed the stored site token")
	}
	_ = pairing.ScopeServe
}

// headerDump flattens a header map for substring probing.
func headerDump(h http.Header) string {
	var b strings.Builder
	for k, vs := range h {
		for _, v := range vs {
			b.WriteString(k)
			b.WriteString(": ")
			b.WriteString(v)
			b.WriteString("\n")
		}
	}
	return b.String()
}
