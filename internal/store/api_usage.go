package store

import (
	"database/sql"
	"fmt"
	"time"
)

// APIUsageDelta is a non-negative increment to accumulate into one UTC day.
// Numbers only — this package must not import jira (layering).
type APIUsageDelta struct {
	Requests        int64
	Throttled       int64
	ServerErrors    int64
	Retries         int64
	WaitMS          int64
	LastThrottledAt string // RFC3339 UTC, empty if unknown this flush
}

// APIUsageDay is one row of daily accumulated outbound API volume.
type APIUsageDay struct {
	Day             string  `json:"day"`
	Requests        int64   `json:"requests"`
	Throttled       int64   `json:"throttled"`
	ServerErrors    int64   `json:"server_errors"`
	Retries         int64   `json:"retries"`
	WaitMS          int64   `json:"wait_ms"`
	LastThrottledAt *string `json:"last_throttled_at,omitempty"`
}

// AddAPIUsage UPSERTs delta into the given UTC day (YYYY-MM-DD), adding to any
// existing counters. last_throttled_at keeps the latest non-empty timestamp.
func (db *DB) AddAPIUsage(day string, u APIUsageDelta) error {
	if day == "" {
		return fmt.Errorf("store: api_usage day is required")
	}
	return db.write(func(tx *sql.Tx) error {
		var last any
		if u.LastThrottledAt != "" {
			last = u.LastThrottledAt
		}
		_, err := tx.Exec(`
			INSERT INTO api_usage (day, requests, throttled, server_errors, retries, wait_ms, last_throttled_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(day) DO UPDATE SET
				requests = api_usage.requests + excluded.requests,
				throttled = api_usage.throttled + excluded.throttled,
				server_errors = api_usage.server_errors + excluded.server_errors,
				retries = api_usage.retries + excluded.retries,
				wait_ms = api_usage.wait_ms + excluded.wait_ms,
				last_throttled_at = CASE
					WHEN excluded.last_throttled_at IS NULL OR excluded.last_throttled_at = ''
						THEN api_usage.last_throttled_at
					WHEN api_usage.last_throttled_at IS NULL OR api_usage.last_throttled_at = ''
						THEN excluded.last_throttled_at
					WHEN excluded.last_throttled_at > api_usage.last_throttled_at
						THEN excluded.last_throttled_at
					ELSE api_usage.last_throttled_at
				END`,
			day, u.Requests, u.Throttled, u.ServerErrors, u.Retries, u.WaitMS, last)
		return err
	})
}

// APIUsage returns up to the most recent days rows, newest day first.
// days <= 0 means all rows.
func (db *DB) APIUsage(days int) ([]APIUsageDay, error) {
	q := `SELECT day, requests, throttled, server_errors, retries, wait_ms, last_throttled_at
		FROM api_usage ORDER BY day DESC`
	var args []any
	if days > 0 {
		q += ` LIMIT ?`
		args = append(args, days)
	}
	rows, err := db.sql.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []APIUsageDay
	for rows.Next() {
		var d APIUsageDay
		var last sql.NullString
		if err := rows.Scan(&d.Day, &d.Requests, &d.Throttled, &d.ServerErrors,
			&d.Retries, &d.WaitMS, &last); err != nil {
			return nil, err
		}
		if last.Valid && last.String != "" {
			s := last.String
			d.LastThrottledAt = &s
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// APIUsageSummary is the shape status --json and GET settings/ runtime expose:
// today's row plus a 7-day rollup of our outbound call volume.
type APIUsageSummary struct {
	Today     APIUsageDay `json:"today"`
	Last7Days APIUsageSum `json:"last_7_days"`
}

// APIUsageSum is a multi-day total (no day key).
type APIUsageSum struct {
	Requests        int64   `json:"requests"`
	Throttled       int64   `json:"throttled"`
	ServerErrors    int64   `json:"server_errors"`
	Retries         int64   `json:"retries"`
	WaitMS          int64   `json:"wait_ms"`
	LastThrottledAt *string `json:"last_throttled_at,omitempty"`
}

// APIUsageSummary builds today + last 7 days from the mirror. Missing days are
// zeros. last_throttled_at on the rollup is the latest timestamp in the window.
func (db *DB) APIUsageSummary() (APIUsageSummary, error) {
	today := time.Now().UTC().Format("2006-01-02")
	out := APIUsageSummary{
		Today: APIUsageDay{Day: today},
	}
	days, err := db.APIUsage(7)
	if err != nil {
		return out, err
	}
	var latest string
	for _, d := range days {
		out.Last7Days.Requests += d.Requests
		out.Last7Days.Throttled += d.Throttled
		out.Last7Days.ServerErrors += d.ServerErrors
		out.Last7Days.Retries += d.Retries
		out.Last7Days.WaitMS += d.WaitMS
		if d.Day == today {
			out.Today = d
		}
		if d.LastThrottledAt != nil && *d.LastThrottledAt != "" {
			if latest == "" || *d.LastThrottledAt > latest {
				latest = *d.LastThrottledAt
			}
		}
	}
	if latest != "" {
		out.Last7Days.LastThrottledAt = &latest
	}
	return out, nil
}

// TableCount returns SELECT COUNT(*) for a fixed known table name used by status.
func (db *DB) TableCount(table string) (int, error) {
	switch table {
	case "issues", "comments":
	default:
		return 0, fmt.Errorf("store: TableCount: unknown table %q", table)
	}
	var n int
	err := db.sql.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n)
	return n, err
}
