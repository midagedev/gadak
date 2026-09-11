// Package freshness owns the one judgment "how far behind is this mirror",
// shared by every surface that answers from it.
//
// It was born in package main (cmd/gadak warnIfStale, GDK-599): the CLI read
// verbs printed the verdict on stderr, and that was the only copy — so the
// MCP server, whose hosts cannot see stderr at all, had no way to know the
// answer it was serving was hours old. The judgment (SQL, thresholds,
// precedence) lives here exactly once; the sentence each surface prints from
// the Report is that surface's own, because stderr advice ("run `gadak sync
// --if-stale 1h`") and an in-result notice ("call gadak_status") correctly
// differ.
package freshness

import (
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/mattn/go-runewidth"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
	syncer "github.com/midagedev/gadak/internal/sync"
)

// Querier is the narrow read handle both callers already hold: the CLI read
// verbs pass whatever connection they opened (writable or read-only), and the
// MCP server passes its store or a read-only open. Deliberately not *sql.DB
// or *store.DB — every caller has some handle, and a second open here doubled
// any diagnostic the open path prints (GDK-314).
type Querier interface {
	QueryRow(query string, args ...any) *sql.Row
}

// StaleAfter is when a mirror stops being worth trusting silently. It is a
// warning, never a refusal: an old answer with a warning beats no answer.
const StaleAfter = time.Hour

// State is the kind of answer the freshness judgment returned. The names are
// the vocabulary every surface speaks; `gadak status` corroborates the same
// facts through its own reads (watermark, last_error, per-source synced_at).
type State int

const (
	// StateFresh means no notice: the mirror finished a sync, it did not
	// fail, and the oldest source is inside StaleAfter. A freshness read
	// error also lands here — silence, never a failed read (a mirror too
	// old to have the table must not break the verbs; doctor owns the
	// detailed verdict).
	StateFresh State = iota
	// StateFirstSync: a first full sync is running right now, so every
	// answer from this mirror is partial by design. It outranks everything
	// else — a live first sync explains the mirror's state better than any
	// staleness verdict can (every source row is empty or half-written).
	StateFirstSync
	// StateNeverSynced: no sync_state rows at all, or none of them ever
	// stored a parseable synced_at. The mirror may hold data (a half-done
	// first pass) or none; either way "fresh" would be a lie.
	StateNeverSynced
	// StateSyncFailed: the most recent sync of some source ended in an
	// error. Carries SourceID and LastError.
	StateSyncFailed
	// StateStaleSource: the oldest successfully-synced source is older than
	// StaleAfter. Carries SourceID, SyncedAt (the raw stored string) and
	// Age. One stale source does not indict the whole mirror — it is named
	// (GDK-810: "mirror last synced" made a stale confluence row look like
	// the whole mirror was that old).
	StateStaleSource
)

// String names the state for test failures and future telemetry.
func (s State) String() string {
	switch s {
	case StateFresh:
		return "fresh"
	case StateFirstSync:
		return "first-sync"
	case StateNeverSynced:
		return "never-synced"
	case StateSyncFailed:
		return "sync-failed"
	case StateStaleSource:
		return "stale-source"
	}
	return "unknown"
}

// Report is the mirror's freshness in one value. Only the fields the State
// documents are set; the others stay zero, so a Report can be compared in
// tests and logged without a sentence getting in the way.
type Report struct {
	State State

	// SourceID: StateFirstSync (the source currently syncing),
	// StateSyncFailed (the source whose sync failed), StateStaleSource
	// (the oldest source). Raw, unsanitized — FormatSourceID makes it safe
	// to interpolate.
	SourceID string

	// LastError: StateSyncFailed — sync_state.last_error as stored.
	LastError string

	// SyncedAt: StateStaleSource — the raw stored sources.synced_at string,
	// echoed verbatim so `gadak status` can corroborate the same string
	// (GDK-810).
	SyncedAt string

	// Age: StateStaleSource — now minus the parsed SyncedAt.
	Age time.Duration

	// Fetched, Total, WikiPending: StateFirstSync — the live sync_progress
	// heartbeat. Total.Valid is false when the origin gave no count; the
	// sentence must omit the denominator rather than invent one.
	Fetched     int
	Total       sql.NullInt64
	WikiPending bool
}

// Assess reads the mirror through q and returns its freshness judgment. now
// is the comparison clock (callers pass time.Now(); tests pin it).
// confluenceConfigured is lazy on purpose: it is consulted only while a
// first sync is live on the issues source, so a fresh or stale mirror never
// pays a config read — the CLI calls this on every read verb.
//
// Precedence (the same order warnIfStale has always printed): a live first
// sync stands everything else down; then a failed sync; then the oldest
// source's age. Read errors are StateFresh/silence for the same reason the
// CLI's warnings are best-effort.
func Assess(q Querier, now time.Time, confluenceConfigured func() bool) Report {
	if rep, ok := assessFirstSync(q, now, confluenceConfigured); ok {
		return rep
	}
	type staleRow struct {
		id        string
		syncedAt  *string
		lastError *string
	}
	var rows []staleRow
	for off := 0; ; off++ {
		var r staleRow
		err := q.QueryRow(`SELECT st.source_id, src.synced_at, st.last_error
			FROM sync_state st LEFT JOIN sources src ON src.id = st.source_id
			ORDER BY st.source_id LIMIT 1 OFFSET ?`, off).Scan(&r.id, &r.syncedAt, &r.lastError)
		if err != nil {
			if off == 0 && !errors.Is(err, sql.ErrNoRows) {
				return Report{State: StateFresh}
			}
			break
		}
		rows = append(rows, r)
	}
	if len(rows) == 0 {
		return Report{State: StateNeverSynced}
	}
	for _, r := range rows {
		if r.lastError != nil && *r.lastError != "" {
			return Report{State: StateSyncFailed, SourceID: r.id, LastError: *r.lastError}
		}
	}
	// Timestamps parse through config.ParseTimestamp, the single owner of
	// the layout table (GDK-1130). Unparseable values are skipped so a
	// corrupt row cannot crash a read, and cannot take the never-synced
	// branch while a sibling parsed.
	var oldest *time.Time
	var oldestID, oldestRaw string
	for _, r := range rows {
		if r.syncedAt == nil || *r.syncedAt == "" {
			continue
		}
		t, ok := config.ParseTimestamp(*r.syncedAt)
		if !ok {
			continue
		}
		if oldest == nil || t.Before(*oldest) {
			tt := t
			oldest = &tt
			oldestID = r.id
			oldestRaw = *r.syncedAt
		}
	}
	if oldest == nil {
		// Every source is empty. A leftover never-synced jira row next to
		// a fresh Linear source must not take this branch: that
		// is anyEmpty with oldest set from the Linear row.
		return Report{State: StateNeverSynced}
	}
	if age := now.Sub(*oldest); age > StaleAfter {
		return Report{State: StateStaleSource, SourceID: oldestID, SyncedAt: oldestRaw, Age: age}
	}
	return Report{State: StateFresh}
}

// assessFirstSync reads the live first-pass heartbeat (the same single row
// the CLI's warning read, over the same store.SyncProgressCutoff liveness
// window, so this and the store reader cannot disagree about what "live"
// means). ok is false when no first sync is in progress or the read failed —
// both mean "fall through to the staleness judgment".
func assessFirstSync(q Querier, now time.Time, confluenceConfigured func() bool) (Report, bool) {
	var sourceID string
	var fetched int
	var total sql.NullInt64
	err := q.QueryRow(`SELECT source_id, fetched, total FROM sync_progress
		WHERE first = 1 AND updated_at >= ?
		ORDER BY started_at DESC LIMIT 1`, store.SyncProgressCutoff(now)).
		Scan(&sourceID, &fetched, &total)
	if err != nil {
		return Report{}, false
	}
	wikiPending := false
	if sourceID != syncer.ConfluenceSourceID && confluenceConfigured != nil && confluenceConfigured() {
		var n int
		if qErr := q.QueryRow(`SELECT COUNT(*) FROM sync_progress
			WHERE source_id = ? AND updated_at >= ?`,
			syncer.ConfluenceSourceID, store.SyncProgressCutoff(now)).Scan(&n); qErr == nil && n == 0 {
			wikiPending = true
		}
	}
	return Report{
		State:       StateFirstSync,
		SourceID:    sourceID,
		Fetched:     fetched,
		Total:       total,
		WikiPending: wikiPending,
	}, true
}

// sourceIDDisplayCols is the one-line budget for a sync_state.source_id.
// jira / linear / confluence fit; a planted multi-kilobyte or control-laden
// id must not wrap the line (GDK-810).
const sourceIDDisplayCols = 32

// FormatSourceID makes a source_id safe to interpolate into one line.
// Control runes become a space; a width-aware clip collapses remaining
// whitespace and truncates (runewidth is this repo's width authority — a
// rune-counted cut renders twice as wide on CJK).
func FormatSourceID(id string) string {
	var b strings.Builder
	for _, r := range id {
		if unicode.IsControl(r) {
			b.WriteRune(' ')
			continue
		}
		b.WriteRune(r)
	}
	s := clipWidth(b.String(), sourceIDDisplayCols)
	if s == "" {
		return "?"
	}
	return s
}

// clipWidth flattens onto one line and cuts to a column budget. The same
// contract cmd/gadak's clip (agent_print.go) has; it lives here too because
// package main is not importable — kept byte-for-byte in step with it.
func clipWidth(s string, cols int) string {
	s = strings.Join(strings.Fields(s), " ")
	if runewidth.StringWidth(s) <= cols {
		return s
	}
	return runewidth.Truncate(s, cols, "…")
}

// noticePrefix is the fixed start of the in-result notice, so a host (or a
// test) can recognize the line without parsing the sentence.
const noticePrefix = "Mirror freshness: "

// Notice is the one-line notice the MCP read tools append to a successful
// result: the fact first, then the next move for an agent with no shell.
// StateFresh is the empty string — the notice exists only when it has
// something true to say, because a line that is always there is a line
// agents stop reading.
func (r Report) Notice() string {
	switch r.State {
	case StateFirstSync:
		return noticePrefix + "first sync in progress — results are partial. " +
			"Work with what has landed; gadak_status carries the progress."
	case StateNeverSynced:
		return noticePrefix + "the mirror has never finished a sync. " +
			"This answer may be empty because nothing has landed yet — " +
			"run `gadak sync` on a shell host, or call gadak_status."
	case StateSyncFailed:
		return noticePrefix + "last sync failed (" + FormatSourceID(r.SourceID) + "): " + r.LastError + ". " +
			"This answer may be out of date — run `gadak sync` on a shell host, or call gadak_status."
	case StateStaleSource:
		return noticePrefix + FormatSourceID(r.SourceID) + " last synced " + r.Age.Round(time.Minute).String() +
			" ago (synced_at " + r.SyncedAt + "). " +
			"This answer may be out of date — run `gadak sync` on a shell host, or call gadak_status."
	}
	return ""
}
