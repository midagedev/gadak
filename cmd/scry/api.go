package main

// scry api — escape hatch: call Atlassian REST with the stored credential.
//
// The mirror only covers what sync models. Watchers, worklogs, sprints, user
// search, and similar endpoints are reached here as raw pass-through requests.
// Authorization is always the configured site; absolute URLs are refused so a
// prompt-injected path cannot exfiltrate the token (see SECURITY.md).

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/midagedev/scry/internal/config"
	"github.com/midagedev/scry/internal/confluence"
	"github.com/midagedev/scry/internal/jira"
	"github.com/midagedev/scry/internal/store"
)

// queryFlags collects repeated --query k=v options.
type queryFlags []string

func (q *queryFlags) String() string { return strings.Join(*q, ", ") }
func (q *queryFlags) Set(v string) error {
	*q = append(*q, v)
	return nil
}

func cmdAPI(args []string) error {
	pos, rest := leading(args, 2)
	fs := newFlagSet("api")
	var queries queryFlags
	fs.Var(&queries, "query", "query parameter k=v (repeatable; value is URL-encoded)")
	dataFlag := fs.String("data", "", "request body: literal, @file, or - for stdin")
	writeFlag := fs.Bool("write", false, "allow non-GET/HEAD methods (uses write retry policy)")
	statusFlag := fs.Bool("status", false, "print HTTP <code> to stderr in addition to the body")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	if len(fs.Args()) > 0 {
		return usageError("api", "usage: scry api [METHOD] <PATH> [--query k=v]... [--data <val|@file|->] [--write] [--status]")
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

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !cfg.HasCredential() {
		return errors.New("no Jira credential — run `scry init` first (scry api uses the stored token)")
	}

	ctx := context.Background()
	var (
		status int
		out    []byte
		client *jira.Client // for usage flush; nil when routed to Confluence
	)

	if isWikiPath(path) {
		if cfg.Confluence == nil {
			return errors.New("Confluence is not enabled for this profile — set confluence in config (or enable it in the web UI) before calling /wiki/ paths; scry will not use the wiki API against a source you left off")
		}
		// Client base is already …/wiki; strip the routing prefix.
		cc := confluence.New(cfg.Site, cfg.Email, cfg.Token)
		status, out, err = cc.Raw(ctx, method, stripWikiPrefix(path), body, mutating)
	} else {
		client = jira.New(cfg.Site, cfg.Email, cfg.Token)
		status, out, err = client.Raw(ctx, method, path, body, mutating)
	}
	// Flush rate-budget counters even when the call failed: the request left the
	// process. A missing/broken DB must not hide the API result.
	flushAPIUsageLocal(client)
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
		return "", "", usageError("api", "usage: scry api [METHOD] <PATH> [--query k=v]... [--data <val|@file|->] [--write] [--status]")
	}
	if len(pos) == 1 {
		if looksLikeMethod(pos[0]) {
			return "", "", usageError("api", "usage: scry api [METHOD] <PATH> … — path is required and must start with /")
		}
		return http.MethodGet, pos[0], nil
	}
	// Two positionals: METHOD PATH, or a mistaken second arg.
	if looksLikeMethod(pos[0]) {
		return pos[0], pos[1], nil
	}
	if strings.HasPrefix(pos[0], "/") {
		return "", "", fmt.Errorf("unexpected argument %q after path — put flags after the path (e.g. --query k=v)", pos[1])
	}
	return "", "", usageError("api", "usage: scry api [METHOD] <PATH> [--query k=v]... [--data <val|@file|->] [--write] [--status]")
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

// flushAPIUsageLocal mirrors sync.flushAPIUsage: accumulate the client's
// process-local counters into api_usage for the current UTC day. Open/write
// failures are swallowed so instrumentation never hides the API result.
func flushAPIUsageLocal(c *jira.Client) {
	if c == nil {
		return
	}
	u := c.TakeUsage()
	if u.Requests == 0 && u.Throttled == 0 && u.ServerErrors == 0 && u.Retries == 0 && u.WaitMS == 0 {
		return
	}
	db, err := openStore()
	if err != nil {
		return
	}
	defer db.Close()
	delta := store.APIUsageDelta{
		Requests:     u.Requests,
		Throttled:    u.Throttled,
		ServerErrors: u.ServerErrors,
		Retries:      u.Retries,
		WaitMS:       u.WaitMS,
	}
	if !u.LastThrottledAt.IsZero() {
		delta.LastThrottledAt = u.LastThrottledAt.UTC().Format("2006-01-02T15:04:05.000Z")
	}
	day := time.Now().UTC().Format("2006-01-02")
	_ = db.AddAPIUsage(day, delta)
}
