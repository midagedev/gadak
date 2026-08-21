package store

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/midagedev/gadak/internal/config"
)

// CategoryInProgress pairs with CategoryDone for the lifecycle spans. Like
// every rule here it keys on the category, never a status name.
const CategoryInProgress = "inprogress"

// DurationsInput is everything the two lifecycle spans need. Changelog
// entries carry status ids only, so the id -> category map comes with them;
// Now is an input so the computation stays deterministic for tests and the
// server (GDK-590) can reuse it verbatim.
type DurationsInput struct {
	Created    string
	Changelog  []DetailChange
	Categories map[string]string // status id -> new | inprogress | done
	Now        time.Time
}

// Spans holds the two lifecycle spans `gadak issue` shows. nil means the
// changelog cannot answer: never entered progress (Wait), or no in-progress
// entry to measure from (Progress).
type Spans struct {
	Wait     *time.Duration // created -> first entry into in-progress
	Progress *time.Duration // latest entry into in-progress -> done, or Now while in progress
}

// Durations computes the spans from the changelog — the same walk Derive
// does, with two questions asked of it. Nothing is stored: data-model.md
// keeps time-in-status deliberately absent, so this stays a query-time
// computation (GDK-591).
//
// Progress measures from the latest in-progress entry, not the first: a
// reopened issue that re-enters progress restarts the clock, which pairs
// with "still in progress means until now" — first-entry would report the
// whole history of a reopened issue as one uninterrupted run.
func Durations(in DurationsInput) Spans {
	entries := make([]DetailChange, 0, len(in.Changelog))
	for _, h := range in.Changelog {
		if h.Field == "status" && h.At != "" {
			entries = append(entries, h)
		}
	}
	// ISO-8601 UTC sorts lexicographically (the connector normalizes to
	// ISOMilli), so oldest-first is a string sort, as Derive also relies on.
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].At < entries[j].At })

	first, last := -1, -1
	for i, e := range entries {
		if in.Categories[e.ToID] == CategoryInProgress {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	var out Spans
	if first >= 0 {
		if created, ok := parseStamp(in.Created); ok {
			if started, ok := parseStamp(entries[first].At); ok {
				out.Wait = span(started.Sub(created))
			}
		}
	}
	if last >= 0 {
		if started, ok := parseStamp(entries[last].At); ok {
			end := in.Now
			for _, e := range entries[last+1:] {
				if in.Categories[e.ToID] == CategoryDone {
					if done, ok := parseStamp(e.At); ok {
						end = done
					}
					break
				}
			}
			out.Progress = span(end.Sub(started))
		}
	}
	return out
}

// span clamps a negative difference to zero: mirror stamps can jitter by a
// millisecond across calls, and a negative wait is not information.
func span(d time.Duration) *time.Duration {
	if d < 0 {
		d = 0
	}
	return &d
}

// parseStamp reads the mirror's ISOMilli (the documented layout) and falls
// back to RFC3339 for stamps that predate the normalization.
func parseStamp(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(config.ISOMilli, s); err == nil {
		return t, true
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, true
	}
	return time.Time{}, false
}

// Line is the one-line form `gadak issue` prints: "wait 3d · progress 5h".
// Empty when neither span exists — the caller omits the line, not its parts.
func (d Spans) Line() string {
	var parts []string
	if d.Wait != nil {
		parts = append(parts, "wait "+FormatDuration(*d.Wait))
	}
	if d.Progress != nil {
		parts = append(parts, "progress "+FormatDuration(*d.Progress))
	}
	return strings.Join(parts, " · ")
}

// FormatDuration renders a span with its single largest unit — the scale
// the durations line reads at. Sub-minute spans keep seconds.
func FormatDuration(d time.Duration) string {
	switch {
	case d >= 24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d >= time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d >= time.Minute:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
}

// StatusCategories is the mirror's status id -> category map: the cached
// origin catalog (status_catalog, written by every sync pass) overlaid on
// the reconstruction from issue rows that carry one — the same query
// loadDeriveContext runs for `issue --derive`, kept so a mirror migrated to
// the catalog but not yet re-synced still resolves what it can. Durations
// needs this to walk the changelog, which stores bare ids.
func (db *DB) StatusCategories(ctx context.Context) (map[string]string, error) {
	out := map[string]string{}
	rows, err := db.sql.QueryContext(ctx,
		`SELECT DISTINCT status_id, status_category FROM issues
		 WHERE status_id IS NOT NULL AND status_id <> ''
		   AND status_category IS NOT NULL AND status_category <> ''`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, cat string
		if err := rows.Scan(&id, &cat); err != nil {
			rows.Close()
			return nil, err
		}
		out[id] = cat
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	cats, err := db.sql.QueryContext(ctx, `SELECT status_id, category FROM status_catalog`)
	if err != nil {
		return nil, err
	}
	defer cats.Close()
	for cats.Next() {
		var id, cat string
		if err := cats.Scan(&id, &cat); err != nil {
			return nil, err
		}
		out[id] = cat
	}
	return out, cats.Err()
}
