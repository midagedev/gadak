package main

// gadak dev — the development-panel verbs (GDK-497). `dev link` records a
// pull request on an issue for standalone and paired workspaces: the origin
// (issuetap, or a paired home serve) keeps the link and serves it back in
// Jira's dev-status shape. On a plain connected Cloud workspace the panel
// belongs to Jira's GitHub app, so gadak refuses instead of pretending;
// mirroring that panel is `gadak config set devStatus true`. The skill
// guidance is one line: an agent that opens a PR runs
// `gadak dev link KEY --pr <url>` right there.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/store"
	"github.com/midagedev/gadak/internal/sync"
)

func cmdDev(args []string) error {
	if len(args) == 0 || wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("dev", nil))
		return nil
	}
	switch args[0] {
	case "link":
		return cmdDevLink(args[1:])
	case "scan":
		return cmdDevScan(args[1:])
	default:
		return fmt.Errorf("dev: unknown subcommand %q (try `gadak dev link` or `gadak dev scan`)", args[0])
	}
}

func cmdDevLink(args []string) error {
	fs := newFlagSet("dev link")
	prURL := fs.String("pr", "", "pull request URL (required)")
	name := fs.String("name", "", "display title (omitted = the URL tail)")
	status := fs.String("status", "open", "open | merged | declined")
	asJSON := fs.Bool("json", false, "emit JSON")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("dev", fs))
		return nil
	}
	rest, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 || *prURL == "" {
		return usageError("dev", "usage: gadak dev link <KEY> --pr <url> [--status open|merged|declined] [--name N]")
	}
	key := normalizeKey(rest[0])
	st, ok := jira.ParseDevPRStatus(*status)
	if !ok {
		return fmt.Errorf("dev link: --status must be open, merged or declined (got %q)", *status)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := refuseConnectedDevWrite(cfg, "dev link"); err != nil {
		return err
	}
	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	ctx := context.Background()

	issueID, err := db.ExternalID(ctx, key)
	if err != nil {
		return fmt.Errorf("dev link: %s is not in the mirror — run `gadak sync` first (%w)", key, err)
	}
	client, err := origin.Client(cfg)
	if err != nil {
		return err
	}
	created, err := client.LinkDevPR(ctx, issueID, *prURL, *name, st)
	if err != nil {
		return err
	}

	refreshDevLinks(ctx, db, client, key, issueID)

	if *asJSON {
		fmt.Printf("{\"key\":%q,\"url\":%q,\"status\":%q}\n", key, created.URL, created.Status)
		return nil
	}
	fmt.Printf("%s\tPR linked: %s (%s)\n", key, created.URL, created.Status.Stored())
	return nil
}

// refreshDevLinks re-reads the origin's dev-status answer for one issue and
// replaces the mirror's dev_links with it (write-through, never a direct
// mirror write of our own copy). A failed refresh is a warning, not an error
// — the origin already holds the link and the next sync heals the mirror.
func refreshDevLinks(ctx context.Context, db *store.DB, client *jira.Client, key, issueID string) {
	prs, err := client.DevStatusPRs(ctx, issueID)
	if err != nil {
		return
	}
	if err := db.ReplaceDevLinks(ctx, key, sync.DevLinksFromPRs(prs)); err != nil {
		fmt.Fprintf(os.Stderr, "gadak: warning: origin holds the link but the mirror refresh failed: %v\n", err)
	}
}

// issueKeyRe is deliberately broad — anything shaped like a key. Matches are
// filtered against the mirror before any write, so a CVE-2024 style false
// positive costs one lookup, never a bad link.
var issueKeyRe = regexp.MustCompile(`\b[A-Z][A-Z0-9]+-[0-9]+\b`)

// issueKeys extracts deduped issue-key candidates from free text
// (PR titles, branch names), in first-seen order.
func issueKeys(s string) []string {
	var keys []string
	seen := map[string]bool{}
	for _, k := range issueKeyRe.FindAllString(strings.ToUpper(s), -1) {
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	return keys
}

// devScanStatus maps gh's PR state onto the origin's dev-status vocabulary.
func devScanStatus(ghState string) jira.DevPRStatus {
	return jira.DevPRStatusFromGitHub(ghState)
}

// cmdDevScan reads the repo's pull requests via `gh pr list`, matches issue
// keys in titles and branch names, and records each match through the same
// write-through path as `dev link`. Idempotent: the origin upserts by URL,
// so a pre-push hook can run it on every push. Commits and bare branches are
// out of scope — the origin's dev-status store only holds pull requests.
func cmdDevScan(args []string) error {
	fs := newFlagSet("dev scan")
	dryRun := fs.Bool("dry-run", false, "list matches without writing")
	installHook := fs.Bool("install-hook", false, "add a pre-push hook that runs `gadak dev scan`")
	limit := fs.Int("limit", 200, "max pull requests to list from gh")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("dev", fs))
		return nil
	}
	if _, err := parseAround(fs, args); err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := refuseConnectedDevWrite(cfg, "dev scan"); err != nil {
		return err
	}
	if *installHook {
		return installDevScanHook()
	}
	if _, err := exec.LookPath("gh"); err != nil {
		return fmt.Errorf("dev scan reads pull requests via the `gh` CLI, which is not on PATH — install it, or record one PR with `gadak dev link`")
	}
	out, err := exec.Command("gh", "pr", "list", "--state", "all", "--limit", strconv.Itoa(*limit),
		"--json", "url,title,state,headRefName").Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return fmt.Errorf("gh pr list: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return fmt.Errorf("gh pr list: %w", err)
	}
	var prs []struct {
		URL         string `json:"url"`
		Title       string `json:"title"`
		State       string `json:"state"`
		HeadRefName string `json:"headRefName"`
	}
	if err := json.Unmarshal(out, &prs); err != nil {
		return fmt.Errorf("gh pr list: unexpected output: %w", err)
	}
	if notice := devScanHitLimitNotice(len(prs), *limit); notice != "" {
		fmt.Fprintln(os.Stderr, notice)
	}

	// Group PRs by candidate key so the mirror refresh runs once per issue.
	type match struct {
		url, title string
		status     jira.DevPRStatus
	}
	byKey := map[string][]match{}
	for _, pr := range prs {
		for _, k := range issueKeys(pr.Title + " " + pr.HeadRefName) {
			byKey[k] = append(byKey[k], match{pr.URL, pr.Title, devScanStatus(pr.State)})
		}
	}
	if len(byKey) == 0 {
		fmt.Println(devScanNoMatchMessage(len(prs)))
		return nil
	}
	keys := make([]string, 0, len(byKey))
	for k := range byKey {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	ctx := context.Background()
	var linked, skipped, issues int
	var writeFailed bool
	var client *jira.Client
	for _, key := range keys {
		issueID, err := db.ExternalID(ctx, key)
		if err != nil {
			skipped += len(byKey[key]) // key-shaped text that is not in the mirror
			continue
		}
		issues++
		for _, m := range byKey[key] {
			if *dryRun {
				fmt.Printf("%s\t%s (%s)\n", key, m.url, m.status.Stored())
				linked++
				continue
			}
			if client == nil {
				if client, err = origin.Client(cfg); err != nil {
					return err
				}
			}
			if _, err := client.LinkDevPR(ctx, issueID, m.url, m.title, m.status); err != nil {
				fmt.Fprintf(os.Stderr, "dev scan: %s: %v\n", key, err)
				writeFailed = true
				continue
			}
			linked++
		}
		if !*dryRun && client != nil {
			refreshDevLinks(ctx, db, client, key, issueID)
		}
	}
	verb := "linked"
	if *dryRun {
		verb = "would link"
	}
	fmt.Printf("dev scan: %s %d PR link(s) across %d issue(s)", verb, linked, issues)
	if skipped > 0 {
		fmt.Printf(" (%d match(es) skipped — key not in the mirror)", skipped)
	}
	fmt.Println()
	if writeFailed {
		return fmt.Errorf("dev scan: one or more links failed")
	}
	return nil
}

// installDevScanHook writes a pre-push hook that runs `gadak dev scan`.
// The hooks path is resolved through git so worktrees (where .git is a file)
// work; an existing hook is never overwritten.
func installDevScanHook() error {
	out, err := exec.Command("git", "rev-parse", "--git-path", "hooks/pre-push").Output()
	if err != nil {
		return fmt.Errorf("dev scan --install-hook: not inside a git repository")
	}
	hook := strings.TrimSpace(string(out))
	if _, err := os.Stat(hook); err == nil {
		return fmt.Errorf("dev scan --install-hook: %s already exists — add `gadak dev scan >/dev/null || echo \"gadak dev scan failed (push continues)\" >&2` to it yourself", hook)
	}
	if err := os.WriteFile(hook, []byte(devScanHookScript()), 0o755); err != nil {
		return err
	}
	fmt.Printf("dev scan: installed %s\n", hook)
	return nil
}

// refuseConnectedDevWrite allows standalone and paired workspaces (their
// origin implements the dev-status POST). A plain connected Cloud site is
// refused: the panel is Jira's GitHub app, and mirroring it is a config flag.
func refuseConnectedDevWrite(cfg *config.Config, verb string) error {
	if cfg.IsStandalone() {
		return nil
	}
	rem, err := origin.PairedStatus(cfg)
	if err != nil {
		return err
	}
	if rem != nil {
		return nil
	}
	return fmt.Errorf("%s is for standalone or paired workspaces — a connected Cloud workspace's development panel is linked by Jira's GitHub app; mirroring needs `gadak config set devStatus true`", verb)
}

func devScanNoMatchMessage(prCount int) string {
	if prCount == 0 {
		return "dev scan: no pull requests"
	}
	return "dev scan: no pull requests mention an issue key"
}

func devScanHitLimitNotice(n, limit int) string {
	if limit > 0 && n == limit {
		return fmt.Sprintf("first %d PRs scanned — raise --limit", limit)
	}
	return ""
}

type scanLink struct{ key, url string }

func linkScanMatches(matches []scanLink, link func(scanLink) error, errOut io.Writer) (linked int, failed bool) {
	for _, m := range matches {
		if err := link(m); err != nil {
			fmt.Fprintf(errOut, "dev scan: %s: %v\n", m.key, err)
			failed = true
			continue
		}
		linked++
	}
	return linked, failed
}

func devScanHookScript() string {
	cmd := "gadak"
	if name := config.Profile(); name != "" {
		cmd += " --workspace " + strconv.Quote(name)
	}
	cmd += " dev scan"
	return "#!/bin/sh\n# installed by gadak dev scan --install-hook\n" +
		cmd + " >/dev/null || echo \"gadak dev scan failed (push continues)\" >&2\n"
}
