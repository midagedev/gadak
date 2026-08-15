package main

// gadak status — sync state and row counts.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/midagedev/gadak/internal/config"
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
	usage, err := db.APIUsageSummary(context.Background())
	if err != nil {
		usage = store.APIUsageSummary{Today: store.APIUsageDay{Day: time.Now().UTC().Format("2006-01-02")}}
	}
	st["api_usage"] = usage

	cfg, _ := config.Load()
	var tokenExpiry config.TokenExpiry
	if cfg != nil {
		tokenExpiry = cfg.TokenExpiryAt(time.Now().UTC())
		st["token_expiry"] = tokenExpiry
	}
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
	for _, k := range []string{"profile", "issues", "comments", "watermark", "version", "last_full_sync_at", "last_error"} {
		if v, ok := st[k]; ok && v != "" {
			fmt.Printf("%-18s %v\n", k, v)
		}
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
			if t, err := time.Parse("2006-01-02T15:04:05.000Z", *at); err == nil {
				line += fmt.Sprintf(" (last %s)", t.UTC().Format("15:04Z"))
			} else if t, err := time.Parse(time.RFC3339, *at); err == nil {
				line += fmt.Sprintf(" (last %s)", t.UTC().Format("15:04Z"))
			}
		}
	}
	return line
}
