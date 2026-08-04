package main

// The agent-facing commands: one issue, one search, and the three writes, each
// self-contained enough for a coding agent to run from a shell with no session
// and no state beyond the mirror and the stored credential
// (specs/000-product/contracts/agent.md).
//
// Reads come from the mirror and never call Jira. Writes go to Jira first and
// re-read the issue into the mirror afterwards, in that order — the same
// write-through shape internal/server/write.go implements for the UI, because a
// write Jira rejected must not leave a trace locally.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/midagedev/scry/internal/config"
	"github.com/midagedev/scry/internal/jira"
	"github.com/midagedev/scry/internal/store"
	syncer "github.com/midagedev/scry/internal/sync"
)

// staleAfter is when a mirror stops being worth trusting silently. It is a
// warning, never a refusal: an old answer with a warning beats no answer.
const staleAfter = time.Hour

// warnIfStale prints one stderr line when the last sync failed or is old, so a
// caller reading stdout knows how far behind the answer may be. stdout stays
// clean, which is what makes the output pipeable.
func warnIfStale() {
	db, err := openReadOnly()
	if err != nil {
		return
	}
	defer db.Close()
	var syncedAt, lastErr *string
	if err := db.QueryRow(`SELECT src.synced_at, st.last_error
		FROM sync_state st LEFT JOIN sources src ON src.id = st.source_id
		WHERE st.source_id = 'jira'`).Scan(&syncedAt, &lastErr); err != nil {
		return
	}
	warn := func(format string, a ...any) { fmt.Fprintf(os.Stderr, "warning: "+format+"\n", a...) }
	switch {
	case lastErr != nil && *lastErr != "":
		warn("last sync failed: %s", *lastErr)
	case syncedAt == nil || *syncedAt == "":
		warn("the mirror has never finished a sync — run `scry sync`")
	default:
		t, err := time.Parse(time.RFC3339, *syncedAt)
		if err == nil && time.Since(t) > staleAfter {
			warn("mirror last synced %s ago — run `scry sync`", time.Since(t).Round(time.Minute))
		}
	}
}

// normalizeKey accepts a key in any case; Jira's are uppercase.
func normalizeKey(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }

// leading peels up to n non-flag arguments off the front so flags may follow
// them: Go's flag package stops parsing at the first non-flag, which would make
// `scry comment NMB-1 -m ok` silently drop the -m. A bare `-` is a value, not a
// flag — it is how `assign` unassigns.
func leading(args []string, n int) (positional, rest []string) {
	for len(args) > 0 && len(positional) < n && (args[0] == "-" || !strings.HasPrefix(args[0], "-")) {
		positional = append(positional, args[0])
		args = args[1:]
	}
	return positional, args
}

// lookup returns the IssueLite rows for the given keys, in the order asked, and
// skips keys the mirror does not have. The store exposes no single-key read, so
// this filters the full list — which is what the server's write path does too,
// and cheap enough at mirror scale.
func lookup(db *store.DB, keys []string) ([]store.IssueLite, error) {
	all, err := db.IssueLites()
	if err != nil {
		return nil, err
	}
	byKey := make(map[string]store.IssueLite, len(all))
	for _, l := range all {
		byKey[l.IssueKey] = l
	}
	out := make([]store.IssueLite, 0, len(keys))
	for _, k := range keys {
		if l, ok := byKey[k]; ok {
			out = append(out, l)
		}
	}
	return out, nil
}

// summaryLine is the one-line rendering shared by search results and the
// confirmation a write prints. Tab-separated for the same reason `sql` is:
// `cut -f1` has to work.
func summaryLine(l store.IssueLite) string {
	return strings.Join([]string{l.IssueKey, l.Status, deref(l.Assignee, "(unassigned)"), l.Summary}, "\t")
}

func deref(s *string, fallback string) string {
	if s == nil || *s == "" {
		return fallback
	}
	return *s
}

/* ── issue ── */

func cmdIssue(args []string) error {
	pos, rest := leading(args, 1)
	fs := flag.NewFlagSet("issue", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "emit the detail document as JSON")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	if len(pos) == 0 {
		return errors.New("usage: scry issue <KEY> [--json]")
	}
	key := normalizeKey(pos[0])

	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	warnIfStale()

	d, err := db.Detail(key)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%s is not in the mirror — check the key, or run `scry sync`", key)
	}
	if err != nil {
		return err
	}
	lites, err := lookup(db, []string{key})
	if err != nil {
		return err
	}
	if len(lites) == 0 {
		return fmt.Errorf("%s has a detail row but no issue row — the mirror is inconsistent, re-sync", key)
	}

	if *asJSON {
		// The detail fields are flattened alongside `issue` so the document reads
		// like `GET <key>/detail/` with the list row included.
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(struct {
			Issue store.IssueLite `json:"issue"`
			*store.Detail
		}{lites[0], d})
	}
	printIssue(lites[0], d)
	return nil
}

func printIssue(l store.IssueLite, d *store.Detail) {
	fmt.Printf("%s\t%s\n", l.IssueKey, l.Summary)
	kv := func(label, value string) {
		if value != "" {
			fmt.Printf("%-13s %s\n", label, value)
		}
	}
	kv("project", l.ProjectKey)
	kv("type", l.IssueType)
	kv("status", fmt.Sprintf("%s (%s)", l.Status, l.StatusCategory))
	kv("priority", deref(l.Priority, ""))
	kv("assignee", deref(l.Assignee, "(unassigned)"))
	kv("reporter", deref(l.Reporter, ""))
	kv("parent", deref(l.EpicKey, ""))
	kv("labels", strings.Join(l.Labels, ", "))
	kv("components", strings.Join(l.Components, ", "))
	kv("fix versions", strings.Join(l.FixVersions, ", "))
	kv("duedate", deref(l.Duedate, ""))
	kv("resolution", deref(l.Resolution, ""))
	kv("created", deref(l.CreatedAt, ""))
	kv("updated", deref(l.UpdatedAt, ""))
	kv("status since", deref(l.StatusChangedAt, ""))
	kv("resolved", deref(l.ResolvedAt, ""))
	if l.ReopenCount > 0 {
		kv("reopens", fmt.Sprintf("%d (last %s)", l.ReopenCount, deref(l.ReopenedAt, "?")))
	}
	// Sorted: map order would make two runs on the same issue differ.
	aliases := make([]string, 0, len(l.Custom))
	for alias := range l.Custom {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	for _, alias := range aliases {
		kv(alias, fmt.Sprint(l.Custom[alias]))
	}

	if body := strings.TrimSpace(jira.PlainText(d.DescriptionADF)); body != "" {
		fmt.Printf("\ndescription\n%s\n", indent(body))
	}
	if len(d.Comments) > 0 {
		fmt.Printf("\ncomments (%d)\n", len(d.Comments))
		for _, c := range d.Comments {
			body := strings.TrimSpace(c.Body)
			if body == "" {
				body = strings.TrimSpace(jira.PlainText(c.BodyADF))
			}
			fmt.Printf("  %s  %s\n%s\n", c.CreatedAt, c.Author, indent(body))
		}
	}
	if len(d.Attachments) > 0 {
		fmt.Printf("\nattachments (%d)\n", len(d.Attachments))
		for _, a := range d.Attachments {
			fmt.Printf("  %s\t%s\t%d bytes\n", a.Filename, a.MimeType, a.Size)
		}
	}
	if len(d.LinkedIssues) > 0 {
		fmt.Printf("\nlinks (%d)\n", len(d.LinkedIssues))
		for _, k := range d.LinkedIssues {
			fmt.Printf("  %s %s\t%s\t%s\n", k.Type, k.Direction, k.Key, k.Summary)
		}
	}
	if len(d.History) > 0 {
		fmt.Printf("\nhistory (%d)\n", len(d.History))
		for _, h := range d.History {
			fmt.Printf("  %s  %s\t%s: %s → %s\n", h.At, h.Author, h.Field, h.FromValue, h.ToValue)
		}
	}
}

func indent(s string) string {
	return "    " + strings.ReplaceAll(strings.TrimRight(s, "\n"), "\n", "\n    ")
}

/* ── search ── */

func cmdSearch(args []string) error {
	// The query is peeled off first so flags may follow it, which is how anyone
	// actually types this: `scry search "flaky upload" --limit 5`.
	pos, rest := leading(args, 1)
	fs := flag.NewFlagSet("search", flag.ExitOnError)
	limit := fs.Int("limit", 20, "maximum matches")
	asJSON := fs.Bool("json", false, "emit matching IssueLite rows as JSON")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	query := strings.TrimSpace(strings.Join(append(pos, fs.Args()...), " "))
	if query == "" {
		return errors.New(`usage: scry search [--limit N] [--json] "text"`)
	}
	// An unquoted multi-word query swallows the flags that follow it, and FTS would
	// then quietly match nothing rather than fail. Say so instead.
	if strings.Contains(query, " -") {
		return fmt.Errorf("quote the search text: %q reads a flag as part of the query", query)
	}
	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	warnIfStale()

	keys, err := db.Search(query, *limit)
	if err != nil {
		return err
	}
	// Best match first: lookup preserves the order FTS ranked the keys in.
	lites, err := lookup(db, keys)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"total": len(lites), "issues": lites})
	}
	for _, l := range lites {
		fmt.Println(summaryLine(l))
	}
	return nil
}

/* ── writes ── */

// mutate is the whole write-through shape: call Jira, re-read the issue into the
// mirror, then print the refreshed row. A failure between the two is reported as
// such, because retrying it would repeat the write Jira already accepted.
func mutate(key string, asJSON bool, fn func(context.Context, *jira.Client) (map[string]any, error)) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !cfg.HasCredential() {
		return errors.New("no Jira credential — run `scry init` first (writes go to Jira, not to the mirror)")
	}
	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	warnIfStale()

	ctx := context.Background()
	c := jira.New(cfg.Site, cfg.Email, cfg.Token)
	extra, err := fn(ctx, c)
	if err != nil {
		return err
	}
	if err := syncer.SyncIssue(ctx, cfg, db, key, syncer.Options{Client: c}); err != nil {
		return fmt.Errorf("write applied to %s, but the mirror did not refresh (run `scry sync`): %w", key, err)
	}
	lites, err := lookup(db, []string{key})
	if err != nil {
		return err
	}
	if len(lites) == 0 {
		return fmt.Errorf("write applied to %s, but it is not in the mirror — is it outside the configured projects?", key)
	}
	if asJSON {
		body := map[string]any{"issue": lites[0]}
		for k, v := range extra {
			body[k] = v
		}
		return json.NewEncoder(os.Stdout).Encode(body)
	}
	fmt.Println(summaryLine(lites[0]))
	return nil
}

func cmdComment(args []string) error {
	pos, rest := leading(args, 1)
	fs := flag.NewFlagSet("comment", flag.ExitOnError)
	text := fs.String("m", "", "comment body; `-` reads it from stdin")
	asJSON := fs.Bool("json", false, "emit JSON")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	if len(pos) == 0 {
		return errors.New("usage: scry comment <KEY> -m <text> [--json]")
	}
	key := normalizeKey(pos[0])
	body := *text
	if body == "-" {
		buf, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		body = string(buf)
	}
	if strings.TrimSpace(body) == "" {
		return errors.New("empty comment — pass -m <text>, or -m - to read stdin")
	}
	return mutate(key, *asJSON, func(ctx context.Context, c *jira.Client) (map[string]any, error) {
		// No mention resolution: `@Name` in a CLI body stays plain text and notifies
		// nobody. ponytail: add it when someone asks, via the users endpoint the UI uses.
		created, err := c.AddComment(ctx, key, jira.Doc(body, nil))
		if err != nil {
			return nil, err
		}
		return map[string]any{"comment": map[string]any{
			"comment_id": created.ID,
			"author":     created.Author.DisplayName,
			"body":       jira.PlainText(created.Body),
		}}, nil
	})
}

func cmdTransition(args []string) error {
	pos, rest := leading(args, 2)
	fs := flag.NewFlagSet("transition", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "emit JSON")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	if len(pos) < 2 {
		return errors.New("usage: scry transition <KEY> <status-or-id> [--json]")
	}
	key := normalizeKey(pos[0])
	// Trailing words join the target so an unquoted `In Review` still works.
	want := strings.TrimSpace(strings.Join(append(pos[1:], fs.Args()...), " "))

	return mutate(key, *asJSON, func(ctx context.Context, c *jira.Client) (map[string]any, error) {
		list, err := c.Transitions(ctx, key)
		if err != nil {
			return nil, err
		}
		// Matching the target status name as well as the transition's own name: sites
		// name transitions arbitrarily ("Start work" → In Progress), and the caller
		// knows the status it wants, not the verb this workflow uses for it.
		for _, t := range list {
			if t.ID == want || strings.EqualFold(t.Name, want) || strings.EqualFold(t.To.Name, want) {
				return nil, c.Transition(ctx, key, t.ID)
			}
		}
		available := make([]string, 0, len(list))
		for _, t := range list {
			available = append(available, fmt.Sprintf("%s (id %s, → %s)", t.Name, t.ID, t.To.Name))
		}
		if len(available) == 0 {
			return nil, fmt.Errorf("%s has no available transitions for this credential", key)
		}
		return nil, fmt.Errorf("no transition matching %q on %s — available: %s",
			want, key, strings.Join(available, "; "))
	})
}

func cmdAssign(args []string) error {
	pos, rest := leading(args, 2)
	fs := flag.NewFlagSet("assign", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "emit JSON")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	if len(pos) < 2 {
		return errors.New("usage: scry assign <KEY> <email|-> [--json]")
	}
	key, who := normalizeKey(pos[0]), strings.TrimSpace(pos[1])

	return mutate(key, *asJSON, func(ctx context.Context, c *jira.Client) (map[string]any, error) {
		id, err := resolveAccount(ctx, c, who)
		if err != nil {
			return nil, err
		}
		return nil, c.SetAssignee(ctx, key, id)
	})
}

// resolveAccount turns an email into a Jira account id: `-` unassigns, the
// configured member directory answers without a network call, and anything else
// goes to Jira's own user search — the same source the UI's picker uses, because
// there is no local user table to search.
func resolveAccount(ctx context.Context, c *jira.Client, who string) (string, error) {
	if who == "-" {
		return "", nil
	}
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	for _, m := range cfg.Members {
		if strings.EqualFold(m.Email, who) && m.JiraAccountID != "" {
			return m.JiraAccountID, nil
		}
	}
	users, err := c.SearchUsers(ctx, who)
	if err != nil {
		return "", err
	}
	for _, u := range users {
		if strings.EqualFold(u.Email, who) {
			return u.AccountID, nil
		}
	}
	// A site that hides emails answers with no email to match on, so a single hit
	// is taken at its word and an ambiguous one is refused rather than guessed.
	if len(users) == 1 {
		return users[0].AccountID, nil
	}
	if len(users) == 0 {
		return "", fmt.Errorf("no Jira user matches %q", who)
	}
	names := make([]string, 0, len(users))
	for _, u := range users {
		names = append(names, fmt.Sprintf("%s <%s>", u.DisplayName, u.Email))
	}
	return "", fmt.Errorf("%q matches %d users — be more specific: %s", who, len(users), strings.Join(names, "; "))
}

// cmdOpen jumps from a key in the terminal to the issue on the Jira site.
// scry is the fast path for reading; this is the escape hatch for everything
// the mirror deliberately does not do (boards, admin, workflow).
func cmdOpen(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: scry open <KEY>")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.Site == "" {
		return errors.New("no Jira site configured — run `scry init` first")
	}
	u := strings.TrimRight(cfg.Site, "/") + "/browse/" + url.PathEscape(normalizeKey(args[0]))
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", u)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	default:
		cmd = exec.Command("xdg-open", u)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("could not open a browser (%v) — the URL is %s", err, u)
	}
	fmt.Println(u)
	return nil
}
