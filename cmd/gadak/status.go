package main

// gadak status — sync state and row counts.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/selfupdate"
	"github.com/midagedev/gadak/internal/store"
	syncer "github.com/midagedev/gadak/internal/sync"
)

func cmdStatus(args []string) error {
	fs := newFlagSet("status")
	asJSON := fs.Bool("json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	// store.Open migrates the mirror (schema v7 field_usage). status only reads
	// issue rows; it may create an empty mirror when none exists yet.
	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()

	st := map[string]any{
		"profile":          config.Profile(),
		"workspace":        workspaceJSONName(),
		"workspace_source": workspaceJSONSource(),
	}
	ctx := context.Background()
	jiraSS, jiraErr := db.SyncState(ctx, "jira")
	linearSS, linearErr := db.SyncState(ctx, "linear")
	confSS, confErr := db.SyncState(ctx, "confluence")
	if ss, err := db.IssueSyncState(ctx); err == nil {
		st["watermark"] = ss.Watermark
		st["version"] = ss.Version
		st["schema_version"] = ss.SchemaVersion // live PRAGMA; SyncState is the owner (GDK-526)
		st["sync_count"] = ss.SyncCount
		if ss.LastFullSyncAt != nil && *ss.LastFullSyncAt != "" {
			st["last_full_sync_at"] = *ss.LastFullSyncAt
		}
		if ss.LastError != nil && *ss.LastError != "" {
			st["last_error"] = *ss.LastError
		}
		if ss.FirstSyncAt != nil && *ss.FirstSyncAt != "" {
			st["first_sync_at"] = *ss.FirstSyncAt
		}
	}
	if jiraErr == nil && linearErr == nil {
		sources := map[string]any{
			"jira":   syncStateJSON(jiraSS),
			"linear": syncStateJSON(linearSS),
		}
		// GDK-810: confluence is the source warnIfStale most often names
		// when watermark is fresh; keep it on the same JSON map so
		// `status --json` `.sources.<id>.synced_at` matches the sql warning.
		if confErr == nil {
			sources["confluence"] = syncStateJSON(confSS)
		}
		st["sources"] = sources
	}
	if n, err := db.TableCount(ctx, "issues"); err == nil {
		st["issues"] = n
	}
	// GDK-628: the comments table holds issue and page comments alike, so a
	// raw TableCount("comments") row was a mixed figure under an
	// issue-sounding label. Split it by meaning: issue_comments reuses the
	// settings runtime's owner (IssueCommentCount), page_comments is its
	// counterpart — the two surfaces cannot disagree by definition.
	if n, err := db.IssueCommentCount(ctx); err == nil {
		st["issue_comments"] = n
	}
	if n, err := db.PageCommentCount(ctx); err == nil {
		st["page_comments"] = n
	}
	if n, err := db.TableCount(ctx, "pages"); err == nil {
		st["pages"] = n
	}
	usage, err := db.APIUsageSummary(ctx)
	if err != nil {
		usage = store.APIUsageSummary{Today: store.APIUsageDay{Day: time.Now().UTC().Format("2006-01-02")}}
	}
	st["api_usage"] = usage

	cfg, err := config.Load()
	if err != nil {
		// Soft: the mirror is already open. Name the config problem on
		// stderr (and in --json) so `gadak status` can diagnose a locked
		// or corrupt file instead of swallowing the error.
		fmt.Fprintf(os.Stderr, "gadak: config: %v\n", err)
		st["config_error"] = err.Error()
	}
	kind, _ := origin.Describe(cfg)
	st["kind"] = kind
	if cfg != nil {
		// The two axes kind used to conflate (GDK-1278). kind stays.
		st["origin_type"] = cfg.OriginType()
		st["transport"] = cfg.Transport()
	}
	if rem, err := origin.PairedStatus(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "gadak: pairing: %v\n", err)
		st["pairing_error"] = err.Error()
	} else if rem != nil {
		st["pairing"] = map[string]string{
			"endpoint": rem.Endpoint,
			"label":    rem.Label,
		}
	}
	// Diagnostic only: status stays exit 0. The sentence is the same one
	// sync/writes/open/api print (GDK-454). Paired workspaces have a
	// credential, so they do not take this branch (GDK-449).
	if cfg != nil && !cfg.HasCredential() {
		fmt.Fprintf(os.Stderr, "gadak: %v\n", config.ErrNotConfigured)
	}
	if cfg != nil {
		st["frozen"] = cfg.SyncFrozen()
	}
	// The resolved actor (GDK-586): the one place an agent can check that
	// its identity was recognized (env > config > Claude Code detection)
	// before writing to a local-origin or paired origin. Absent when the
	// ladder resolves nothing — writes then use the origin's default user.
	if actor, ok := config.ResolveActor(cfg); ok {
		st["actor"] = actor
	}
	// The origin's language (GDK-597), next to the actor row — the two things
	// an agent reads to know who it writes as and what the origin will call
	// things back. Local-origin only: a connected workspace's language is the
	// Atlassian account's, not ours.
	if cfg != nil && cfg.HasLocalOrigin() {
		st["locale"] = cfg.EffectiveLocale()
	}
	st["custom_fields"] = cfg.CustomFieldsStatus()
	notMirrored, notConfigured := projectScopeMismatch(cfg, db)
	st["projects_configured_not_in_mirror"] = notMirrored
	st["projects_in_mirror_not_configured"] = notConfigured
	var tokenExpiry config.TokenExpiry
	if cfg != nil {
		tokenExpiry = cfg.TokenExpiryAt(time.Now().UTC())
		st["token_expiry"] = tokenExpiry
	}
	wiki := wikiPathStatus(cfg)
	if confErr == nil && confSS.LastError != nil && *confSS.LastError != "" {
		wiki["last_error"] = *confSS.LastError
	}
	st["wiki"] = wiki
	var updateInfo selfupdate.Info
	var updateOK bool
	if cfg != nil && cfg.UpdateCheckEnabled() {
		if dir, err := config.Dir(); err == nil {
			updateInfo, updateOK = selfupdate.Check(context.Background(), dir, version, true)
			if updateOK && selfupdate.Newer(version, updateInfo.Latest) {
				st["update"] = map[string]string{
					"latest": updateInfo.Latest,
					"url":    updateInfo.URL,
				}
			}
		}
	}

	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(st)
	}
	for _, k := range []string{"profile", "kind", "issues", "issue_comments", "page_comments", "pages", "watermark", "version", "last_full_sync_at", "last_error"} {
		if v, ok := st[k]; ok && v != "" {
			fmt.Printf("%-18s %v\n", k, v)
		}
	}
	// GDK-810: the same sources.synced_at strings warnIfStale prints.
	// Compare `<id>.synced_at` here with `(synced_at …)` on the sql warning.
	printSourceSyncedAt("jira", jiraSS, jiraErr)
	printSourceSyncedAt("linear", linearSS, linearErr)
	printSourceSyncedAt("confluence", confSS, confErr)
	if frozen, ok := st["frozen"].(bool); ok && frozen {
		fmt.Printf("%-18s %v\n", "frozen", true)
	}
	if actor, ok := st["actor"].(config.ResolvedActor); ok {
		line := actor.Slug
		if actor.Name != "" && actor.Name != actor.Slug {
			line += " — " + actor.Name
		}
		// The actor.trailer switch beside the identity, so a person can see
		// whether Jira/Linear writes will carry the attribution line (the
		// built-in origin records the actor as the author instead).
		trailer := "off"
		if cfg.ActorTrailerEnabled() {
			trailer = "on"
		}
		fmt.Printf("%-18s %s (%s) · trailer %s\n", "actor", line, actor.Source, trailer)
	}
	if loc, ok := st["locale"].(string); ok {
		fmt.Printf("%-18s %s\n", "locale", loc)
	}
	if p, ok := st["pairing"].(map[string]string); ok {
		fmt.Printf("paired with %q (%s)\n", p["label"], p["endpoint"])
	}
	if line := formatWikiStatusLine(wiki); line != "" {
		fmt.Printf("%-18s %s\n", "wiki", line)
	}
	if line := formatAPIUsageLine(usage); line != "" {
		fmt.Printf("%-18s %s\n", "api (today)", line)
	}
	if tokenExpiry.Message != "" {
		fmt.Println(tokenExpiry.Message)
	}
	if updateOK && selfupdate.Newer(version, updateInfo.Latest) {
		fmt.Printf("update: v%s available (running v%s) — brew upgrade midagedev/tap/gadak\n",
			updateInfo.Latest, version)
		if updateInfo.URL != "" {
			fmt.Println(updateInfo.URL)
		}
	}
	if len(notMirrored) > 0 {
		fmt.Printf("%-18s %s\n", "configured, not in the mirror", strings.Join(notMirrored, ", "))
	}
	if len(notConfigured) > 0 {
		fmt.Printf("%-18s %s\n", "in the mirror, not configured", strings.Join(notConfigured, ", "))
	}
	return nil
}

// printSourceSyncedAt writes one text-mode row for a source that has a
// stored sources.synced_at. The key is `<id>.synced_at` so it lines up
// with the other 18-column status labels and with the sql warning's
// `(synced_at …)` (GDK-810). Empty / missing / fetch error: skip.
func printSourceSyncedAt(id string, ss store.SyncState, err error) {
	if err != nil {
		return
	}
	if ss.SyncedAt == nil || *ss.SyncedAt == "" {
		return
	}
	fmt.Printf("%-18s %s\n", id+".synced_at", *ss.SyncedAt)
}

// projectScopeMismatch compares config.Projects with distinct project_key
// values already in the mirror (GDK-809). Empty Projects means "every
// project" — that is not a mismatch. Does not call the origin.
// projectScopeMismatch reads the sides through sync.ProjectScopeDiff — the
// one owner of the config↔mirror comparison (GDK-973), scoped to Jira keys so
// a Linear team key never reads as "in the mirror, not configured" (the
// unscoped DISTINCT this used to run had that false positive latent). Empty
// slices, not nil: the status JSON always carries both arrays.
func projectScopeMismatch(cfg *config.Config, db *store.DB) (notMirrored, notConfigured []string) {
	notMirrored, notConfigured = []string{}, []string{}
	if cfg == nil || db == nil || len(cfg.Projects) == 0 {
		return notMirrored, notConfigured
	}
	counts, err := db.ProjectIssueCounts(context.Background(), syncer.SourceID)
	if err != nil {
		return notMirrored, notConfigured
	}
	empty, extra := syncer.ProjectScopeDiff(cfg, counts)
	notMirrored = append(notMirrored, empty...)
	notConfigured = append(notConfigured, extra...)
	return notMirrored, notConfigured
}

func mirrorProjectKeys(db *store.DB) ([]string, error) {
	if db == nil {
		return nil, fmt.Errorf("nil db")
	}
	rows, err := db.Query(`SELECT DISTINCT project_key FROM issues WHERE project_key IS NOT NULL AND project_key != '' ORDER BY 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		k = strings.TrimSpace(k)
		if k != "" {
			out = append(out, k)
		}
	}
	return out, rows.Err()
}

func printProjectScopeWarnings(cfg *config.Config, db *store.DB) {
	notMirrored, notConfigured := projectScopeMismatch(cfg, db)
	if len(notMirrored) > 0 {
		fmt.Fprintf(os.Stderr, "warning: configured, not in the mirror: %s\n", strings.Join(notMirrored, ", "))
	}
	if len(notConfigured) > 0 {
		fmt.Fprintf(os.Stderr, "warning: in the mirror, not configured: %s\n", strings.Join(notConfigured, ", "))
	}
}

func syncStateJSON(ss store.SyncState) map[string]any {
	m := map[string]any{
		"watermark":  ss.Watermark,
		"version":    ss.Version,
		"sync_count": ss.SyncCount,
	}
	if ss.SyncedAt != nil && *ss.SyncedAt != "" {
		m["synced_at"] = *ss.SyncedAt
	}
	if ss.LastFullSyncAt != nil && *ss.LastFullSyncAt != "" {
		m["last_full_sync_at"] = *ss.LastFullSyncAt
	}
	if ss.LastError != nil && *ss.LastError != "" {
		m["last_error"] = *ss.LastError
	}
	if ss.FirstSyncAt != nil && *ss.FirstSyncAt != "" {
		m["first_sync_at"] = *ss.FirstSyncAt
	}
	return m
}

// wikiPathStatus reports whether the wiki sync pass will run for this
// workspace. Reasons reuse the refusal strings the pass itself returns:
// "sync: confluence is not configured" (internal/sync/confluence.go) and
// "origin: site, email and token are required" (internal/origin.errNeedCredential).
// No site, email, or token is copied into the map.
func wikiPathStatus(cfg *config.Config) map[string]any {
	out := map[string]any{}
	if cfg == nil || cfg.Confluence == nil {
		out["path"] = "skipped"
		out["reason"] = "sync: confluence is not configured"
		return out
	}
	// HasAtlassianCredential, not the site/email/token triple: a paired
	// workspace keeps its credential in remote-origin.json (GDK-1276).
	if !cfg.HasAtlassianCredential() {
		out["path"] = "skipped"
		out["reason"] = "origin: site, email and token are required"
		return out
	}
	out["path"] = "on"
	return out
}

// formatWikiStatusLine is the text-mode wiki row. Empty only if wiki is missing.
func formatWikiStatusLine(wiki map[string]any) string {
	if wiki == nil {
		return ""
	}
	path, _ := wiki["path"].(string)
	if path == "" {
		return ""
	}
	line := path
	if r, ok := wiki["reason"].(string); ok && r != "" {
		line += " (" + r + ")"
	}
	if le, ok := wiki["last_error"].(string); ok && le != "" {
		line += "; last_error " + le
	}
	return line
}

// formatAPIUsageLine returns "" when nothing has been counted in the last week,
// so a fresh or credential-less profile does not carry a line that only ever
// says zero. The value is aligned with the other status rows by the caller.
func formatAPIUsageLine(u store.APIUsageSummary) string {
	if u.Today.Requests == 0 && u.Last7Days.Requests == 0 {
		return ""
	}
	line := fmt.Sprintf("%d requests", u.Today.Requests)
	if u.Last7Days.Requests != u.Today.Requests {
		line += fmt.Sprintf(" (%d in 7 days)", u.Last7Days.Requests)
	}
	if u.Today.Throttled > 0 {
		line += fmt.Sprintf(", %d throttled", u.Today.Throttled)
		at := u.Today.LastThrottledAt
		if at == nil {
			at = u.Last7Days.LastThrottledAt
		}
		if at != nil && *at != "" {
			if t, err := time.Parse(config.ISOMilli, *at); err == nil {
				line += fmt.Sprintf(" (last %s)", t.UTC().Format("15:04Z"))
			} else if t, err := time.Parse(time.RFC3339, *at); err == nil {
				line += fmt.Sprintf(" (last %s)", t.UTC().Format("15:04Z"))
			}
		}
	}
	return line
}
