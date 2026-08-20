package main

// gadak status — sync state and row counts.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/selfupdate"
	"github.com/midagedev/gadak/internal/store"
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

	st := map[string]any{"profile": config.Profile()}
	if ss, err := db.SyncState(context.Background(), "jira"); err == nil {
		st["watermark"] = ss.Watermark
		st["version"] = ss.Version
		st["schema_version"] = ss.SchemaVersion
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
	if n, err := db.TableCount(context.Background(), "issues"); err == nil {
		st["issues"] = n
	}
	if n, err := db.TableCount(context.Background(), "comments"); err == nil {
		st["comments"] = n
	}
	if n, err := db.TableCount(context.Background(), "pages"); err == nil {
		st["pages"] = n
	}
	usage, err := db.APIUsageSummary(context.Background())
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
	if rem, err := origin.PairedStatus(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "gadak: pairing: %v\n", err)
		st["pairing_error"] = err.Error()
	} else if rem != nil {
		st["pairing"] = map[string]string{
			"endpoint": rem.Endpoint,
			"label":    rem.Label,
		}
	}
	var tokenExpiry config.TokenExpiry
	if cfg != nil {
		tokenExpiry = cfg.TokenExpiryAt(time.Now().UTC())
		st["token_expiry"] = tokenExpiry
	}
	wiki := wikiPathStatus(cfg)
	if css, err := db.SyncState(context.Background(), "confluence"); err == nil {
		if css.LastError != nil && *css.LastError != "" {
			wiki["last_error"] = *css.LastError
		}
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
	for _, k := range []string{"profile", "kind", "issues", "comments", "pages", "watermark", "version", "last_full_sync_at", "last_error"} {
		if v, ok := st[k]; ok && v != "" {
			fmt.Printf("%-18s %v\n", k, v)
		}
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
	return nil
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
	if !cfg.IsStandalone() && (cfg.Site == "" || cfg.Email == "" || cfg.Token == "") {
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
