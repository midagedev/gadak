package main

// gadak api — escape hatch: call Atlassian REST with the stored credential.
//
// The mirror only covers what sync models. Watchers, worklogs, user search,
// and similar endpoints are reached here as raw pass-through requests. Sprints
// are projected onto issues.sprint_id / sprint_name / sprint_state (schema
// v30); prefer those columns, and never filter on sprint_name.
// Authorization is always the configured site; absolute URLs are refused so a
// prompt-injected path cannot exfiltrate the token (see SECURITY.md).

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
	syncer "github.com/midagedev/gadak/internal/sync"
)

// queryFlags collects repeated --query k=v options.
type queryFlags []string

func (q *queryFlags) String() string { return strings.Join(*q, ", ") }
func (q *queryFlags) Set(v string) error {
	*q = append(*q, v)
	return nil
}

const apiUsage = "usage: gadak api [METHOD] <PATH> [--query k=v]... [--data <val|@file|->] [--write] [--status]"

// apiBackoffOverride, when non-nil, replaces the Jira client's first retry
// wait after origin.Client. Tests pin a tiny value so a 429 retry is
// asserted without the production 1s Backoff. Nil in production.
var apiBackoffOverride *time.Duration

func cmdAPI(args []string) error {
	fs := newFlagSet("api")
	var queries queryFlags
	fs.Var(&queries, "query", "query parameter k=v (repeatable; value is URL-encoded)")
	dataFlag := fs.String("data", "", "request body: literal, @file, or - for stdin")
	writeFlag := fs.Bool("write", false, "allow non-GET/HEAD methods (uses write retry policy)")
	statusFlag := fs.Bool("status", false, "print HTTP <code> to stderr in addition to the body")
	// Every other subcommand takes --json, so hands type it here by reflex
	// (GDK-1072). The body is the origin's response printed unchanged —
	// already JSON — so the flag is accepted and changes nothing.
	_ = fs.Bool("json", false, "accepted for consistency; the response body is printed unchanged either way")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("api", fs))
		return nil
	}
	// Flags may sit before, between, or after METHOD/PATH. parseAround
	// rejects unknown dashes (GDK-41); the loop below is defense in depth.
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	for _, a := range pos {
		if a != "-" && strings.HasPrefix(a, "-") {
			return usageError("api", fmt.Sprintf("unknown flag %s", a))
		}
	}

	method, path, err := parseAPIMethodPath(pos)
	if err != nil {
		return err
	}

	// Security: absolute / scheme-relative paths never leave this process with
	// Authorization. Also enforced inside the clients; reject early for a clear CLI error.
	if err := rejectAPIPath(path); err != nil {
		return err
	}

	method = strings.ToUpper(method)
	mutating := method != http.MethodGet && method != http.MethodHead
	if mutating && !*writeFlag {
		return fmt.Errorf("%s requires --write (refusing to change remote state without an explicit flag; re-run with --write if intentional)", method)
	}

	body, err := readAPIData(*dataFlag)
	if err != nil {
		return err
	}

	path, err = appendQuery(path, queries)
	if err != nil {
		return err
	}

	if *writeFlag {
		warnWorkspaceIfEnv()
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !cfg.HasCredential() {
		return config.NotConfiguredWith("gadak api uses the stored token")
	}

	ctx := context.Background()
	var (
		status int
		out    []byte
	)

	// Flush rate-budget counters even when the call failed: the request left
	// the process. A missing/broken DB must not hide the API result. Failures
	// are logged (sync.FlushAPIUsage), never returned.
	if isWikiPath(path) {
		// Deliberately NOT gated on cfg.Confluence (GDK-1072): that setting
		// scopes the *mirror*, and this command is the escape hatch past the
		// mirror. The credential, the same-site check, and --write are the
		// security surface, and all three hold for /wiki exactly as for /rest.
		// origin.Wiki's own refusals (frozen workspace, no credential,
		// Linear-only profile) still apply.
		// Client base is already …/wiki; strip the routing prefix.
		cc, werr := origin.Wiki(cfg)
		if werr != nil {
			return werr
		}
		status, out, err = cc.Raw(ctx, method, stripWikiPrefix(path), body, mutating)
		if db, oerr := openStore(); oerr != nil {
			log.Printf("api usage flush: %v", oerr)
		} else {
			syncer.FlushAPIUsage(ctx, db, cc, log.Printf)
			_ = db.Close()
		}
	} else {
		client, oerr := origin.Client(cfg)
		if oerr != nil {
			return oerr
		}
		if apiBackoffOverride != nil {
			client.Backoff = *apiBackoffOverride
		}
		status, out, err = client.Raw(ctx, method, path, body, mutating)
		if db, oerr := openStore(); oerr != nil {
			log.Printf("api usage flush: %v", oerr)
		} else {
			syncer.FlushAPIUsage(ctx, db, client, log.Printf)
			_ = db.Close()
		}
	}
	if err != nil {
		return err
	}

	if *statusFlag {
		fmt.Fprintf(os.Stderr, "HTTP %d\n", status)
	}

	if len(out) > 0 {
		if _, werr := os.Stdout.Write(out); werr != nil {
			return werr
		}
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("HTTP %d: %s", status, httpStatusDetail(status, out))
	}
	return nil
}

func parseAPIMethodPath(pos []string) (method, path string, err error) {
	if len(pos) == 0 {
		return "", "", usageError("api", apiUsage)
	}
	if len(pos) > 2 {
		return "", "", fmt.Errorf("unexpected argument %q", pos[2])
	}
	if len(pos) == 1 {
		if looksLikeMethod(pos[0]) {
			return "", "", usageError("api", "usage: gadak api [METHOD] <PATH> … — path is required and must start with /")
		}
		if err := requireAPIPath(pos[0]); err != nil {
			return "", "", err
		}
		return http.MethodGet, pos[0], nil
	}
	// Two positionals: METHOD PATH, or a mistaken second arg.
	if looksLikeMethod(pos[0]) {
		if err := requireAPIPath(pos[1]); err != nil {
			return "", "", err
		}
		return pos[0], pos[1], nil
	}
	if strings.HasPrefix(pos[0], "/") {
		return "", "", fmt.Errorf("unexpected argument %q after path", pos[1])
	}
	return "", "", usageError("api", apiUsage)
}

// requireAPIPath is the cheap CLI shape check: a PATH always starts with /.
// rejectAPIPath then refuses absolute / scheme-relative URLs.
func requireAPIPath(path string) error {
	if path == "" || !strings.HasPrefix(path, "/") {
		return fmt.Errorf("path must start with / (got %q)", path)
	}
	return nil
}

func looksLikeMethod(s string) bool {
	switch strings.ToUpper(s) {
	case "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "TRACE":
		return true
	}
	return false
}

func isWikiPath(path string) bool {
	// Path may already carry a query string.
	p := path
	if i := strings.IndexByte(p, '?'); i >= 0 {
		p = p[:i]
	}
	return p == "/wiki" || strings.HasPrefix(p, "/wiki/")
}

// stripWikiPrefix turns a CLI path (/wiki/api/v2/…) into a confluence.Client
// path (/api/v2/…); the client base already ends in /wiki.
func stripWikiPrefix(path string) string {
	p, query, hasQuery := strings.Cut(path, "?")
	p = strings.TrimPrefix(p, "/wiki")
	if p == "" {
		p = "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if hasQuery {
		return p + "?" + query
	}
	return p
}

func rejectAPIPath(path string) error {
	// Strip query for the absolute-URL check; query is appended later or already present.
	p := path
	if i := strings.IndexByte(p, '?'); i >= 0 {
		p = p[:i]
	}
	if p == "" {
		return fmt.Errorf("path is required and must start with /")
	}
	lower := strings.ToLower(p)
	if strings.HasPrefix(p, "//") ||
		strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") {
		return fmt.Errorf("absolute URLs are not allowed — pass a path starting with / so the request stays on your configured site (Authorization is attached to every request)")
	}
	if !strings.HasPrefix(p, "/") {
		return fmt.Errorf("path must start with / (got %q)", p)
	}
	return nil
}

func readAPIData(flag string) ([]byte, error) {
	if flag == "" {
		return nil, nil
	}
	switch {
	case flag == "-":
		return io.ReadAll(os.Stdin)
	case strings.HasPrefix(flag, "@"):
		name := strings.TrimPrefix(flag, "@")
		if name == "" {
			return nil, errors.New("--data @file: empty file path")
		}
		b, err := os.ReadFile(name)
		if err != nil {
			return nil, fmt.Errorf("--data @%s: %w", name, err)
		}
		return b, nil
	default:
		return []byte(flag), nil
	}
}

func appendQuery(path string, pairs []string) (string, error) {
	if len(pairs) == 0 {
		return path, nil
	}
	q := url.Values{}
	for _, kv := range pairs {
		if kv == "" {
			return "", errors.New("--query requires k=v")
		}
		k, v, ok := strings.Cut(kv, "=")
		if !ok || k == "" {
			return "", fmt.Errorf("--query expects k=v (got %q)", kv)
		}
		q.Add(k, v)
	}
	enc := q.Encode()
	if enc == "" {
		return path, nil
	}
	if strings.Contains(path, "?") {
		return path + "&" + enc, nil
	}
	return path + "?" + enc, nil
}

func httpStatusDetail(status int, body []byte) string {
	s := strings.TrimSpace(string(body))
	if s == "" {
		if t := http.StatusText(status); t != "" {
			return t
		}
		return "request failed"
	}
	// One line for humans; body already went to stdout in full.
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 400 {
		return s[:400] + "…"
	}
	return s
}
