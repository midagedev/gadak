package mcp

// gadak_retro — the retrospective numbers for a host without a shell.
//
// `gadak retro` is the instrument the release asks the user to quote when
// they say whether their work got better, and until now every shell-less
// client (Claude Desktop and everything `gadak mcp install` targets) could
// not reach it at all: the mirror was queryable with gadak_query, but the
// report is not a query — it is sessions, resume medians, cycle percentiles
// and the reopen surfaces, computed together.
//
// Thin on purpose, like the ui pair next door: retro.Compute and
// retro.Report.JSON() are the owners the CLI's --json and
// GET /api/v1/issues/retro/ already share, and nothing here recomputes,
// reformats or copies a query. The argument names are the CLI's flag names
// (since, session-gap, by-sprint, board, open, week) and the metric enum is
// GENERATED from retro.OpenMetrics — internal/mcp/tools.go descriptions are
// the one surface in this repo with no gate, so the vocabulary must not be
// typed here twice.

import (
	"context"
	gosql "database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/retro"
	"github.com/midagedev/gadak/internal/store"
)

const toolRetro = "gadak_retro"

// retroNow is the one clock this tool reads; a test can pin it.
var retroNow = time.Now

// retroMCPDefaultSince is this surface's window when the call names none.
// Four ISO weeks — the same choice GET /api/v1/issues/retro/ made for a
// reader that is looking at a document rather than typing a flag; the CLI's
// 14d stays the CLI's, and a caller that wants it says since: "14d".
const retroMCPDefaultSince = "4w"

// retroArgNames is the accepted argument set, in help order. It is also the
// refusal message's list: an argument the CLI does not have is named rather
// than dropped, because a silently ignored window is a wrong number the
// reader cannot see.
var retroArgNames = []string{"since", "session-gap", "by-sprint", "board", "open", "week"}

func retroDescription() string {
	return `The user's retrospective numbers — the same document ` + "`gadak retro --json`" + ` prints.

One column per ISO week (or per sprint), and for each: sessions, resume
(median minutes back to where you were), WIP age p85/max, in progress, closed,
cycle p50/p85, mismatch — plus the materials under the table (events,
surprises, closed by type and by epic, unplanned, seen-not-moved,
moved-not-seen) and the report-level aging tail and retro actions. Definitions
for every row come back with the numbers, so a figure is never quoted without
what it means.

Arguments (all optional; the names are the CLI's flags):
- since: "14d", "30d", "8w" — how far back the columns reach, 1 to 365 days
  (default "` + retroMCPDefaultSince + `"). Not accepted beside by-sprint: they choose the columns
  two different ways.
- session-gap: a Go duration, 5m to 24h — split sessions where the gap to the
  previous read exceeds this. Unset means the workspace's retro.sessionGap.
- by-sprint: true for one column per sprint instead of per week.
- board: with by-sprint, which board's sprints are the columns (only needed
  when more than one board has sprints).
- open: name one cell and get the issue keys behind it instead of the table:
  "` + strings.Join(retro.OpenMetrics, `" | "`) + `".
- week: which column open reads — 0 is the current partial column, 1 the last
  full one. "aging" is measured at now and takes no week.

The table answer carries the numbers, the definitions, the surprises, the
per-type and per-epic closes, the cycle points, the aging tail and the retro
actions. It leaves out each column's event log and issue-key lists, because
those are what open returns and keeping them would make the plainest call the
largest one; the response says so in "omitted".

Read-only: this reads the local cache and writes nothing, here or at the
origin. Follow the keys with gadak_issue, or put them on screen with
gadak_show.`
}

func retroToolDefinition() Tool {
	metricEnum := make([]any, 0, len(retro.OpenMetrics))
	for _, m := range retro.OpenMetrics {
		metricEnum = append(metricEnum, m)
	}
	return Tool{
		Name:        toolRetro,
		Description: retroDescription(),
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"since": map[string]any{
					"type":        "string",
					"description": `How far back the columns reach: <N>d or <N>w, 1 to 365 days. Default "` + retroMCPDefaultSince + `".`,
				},
				"session-gap": map[string]any{
					"type":        "string",
					"description": "Go duration, 5m to 24h: split sessions where the gap to the previous read exceeds this. Unset uses the workspace's retro.sessionGap.",
				},
				"by-sprint": map[string]any{
					"type":        "boolean",
					"description": "One column per sprint instead of per ISO week. Not accepted beside since.",
				},
				"board": map[string]any{
					"type":        "integer",
					"description": "With by-sprint: which board's sprints are the columns.",
				},
				"open": map[string]any{
					"type":        "string",
					"enum":        metricEnum,
					"description": "Answer the issue keys behind one cell instead of the table.",
				},
				"week": map[string]any{
					"type":        "integer",
					"description": "With open: which column to read — 0 is the current partial column, 1 the last full one.",
					"minimum":     0,
				},
			},
			"required":             []string{},
			"additionalProperties": false,
		},
	}
}

// retroUnknownArgs names every key the CLI does not have. additionalProperties
// is already false in the schema, but a host that does not validate would
// otherwise get a report for a window it did not ask for.
func retroUnknownArgs(args map[string]any) []string {
	var bad []string
	for k := range args {
		if !containsString(retroArgNames, k) {
			bad = append(bad, k)
		}
	}
	sort.Strings(bad)
	return bad
}

func containsString(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func boolArg(args map[string]any, key string) (bool, bool) {
	if args == nil {
		return false, false
	}
	v, ok := args[key]
	if !ok || v == nil {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}

func (s *Server) toolRetro(args map[string]any) ([]contentItem, error) {
	if bad := retroUnknownArgs(args); len(bad) > 0 {
		return nil, fmt.Errorf("%s does not take %s — it takes %s (the flags of `gadak retro`)",
			toolRetro, strings.Join(bad, ", "), strings.Join(retroArgNames, ", "))
	}

	sinceRaw, sinceGiven := stringArg(args, "since")
	sinceRaw = strings.TrimSpace(sinceRaw)
	sinceGiven = sinceGiven && sinceRaw != ""
	bySprint, _ := boolArg(args, "by-sprint")
	// The same refusal the CLI makes: honouring both would silently drop one —
	// since would name a window the sprints do not fill.
	if bySprint && sinceGiven {
		return nil, fmt.Errorf("since and by-sprint choose the columns two different ways; pass one")
	}
	if !sinceGiven {
		sinceRaw = retroMCPDefaultSince
	}
	since, err := retro.ParseSince(sinceRaw)
	if err != nil {
		return nil, err
	}

	boardID := int64(intArg(args, "board", 0))
	if boardID != 0 && !bySprint {
		return nil, fmt.Errorf("board only applies to by-sprint")
	}

	cfg, cfgErr := config.LoadFor(s.Profile)
	if cfgErr != nil {
		return nil, cfgErr
	}
	// The session gap's default is config-owned (retro.sessionGap); a given
	// argument still wins. Same precedence as the CLI flag and ?session_gap=.
	gapRaw, gapGiven := stringArg(args, "session-gap")
	gapRaw = strings.TrimSpace(gapRaw)
	if !gapGiven || gapRaw == "" {
		gapRaw = cfg.EffectiveRetroSessionGap()
	}
	sessionGap, err := retro.ParseSessionGap(gapRaw)
	if err != nil {
		if !gapGiven {
			return nil, fmt.Errorf("config retro.sessionGap: %w", err)
		}
		return nil, err
	}

	metric, metricGiven := stringArg(args, "open")
	metric = strings.TrimSpace(metric)
	if metricGiven && metric != "" && !retro.IsOpenMetric(metric) {
		return nil, fmt.Errorf("open wants %s (got %q)", retro.JoinOpenMetrics(), metric)
	}
	week := intArg(args, "week", 0)
	_, weekGiven := args["week"]
	if weekGiven && (metric == "") {
		return nil, fmt.Errorf("week only applies to open")
	}

	// The built-in tracker attributes a write to the actor slug rather than to
	// a credential, so an actor is an identity here even with no account
	// (GDK-1427) — the same pair cmd/gadak/retro.go passes.
	actor := ""
	if a, ok := config.ResolveActor(cfg); ok {
		actor = a.Slug
	}

	db, err := s.readOnlyMirror()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rep, err := retro.Compute(context.Background(), db, store.FeedIdentityOf(cfg), since, retroNow(), retro.Options{
		SessionGap: sessionGap,
		BySprint:   bySprint,
		BoardID:    boardID,
		Actor:      actor,
		Origin:     cfg.OriginType(),
	})
	if err != nil {
		// A workspace with no sprints, or several boards and no choice, is the
		// caller asking for a report that cannot be built: the error already
		// carries the sentence, and an ambiguous board carries the list.
		var amb *retro.ErrAmbiguousBoard
		if errors.As(err, &amb) || errors.Is(err, retro.ErrNoSprints) {
			return nil, err
		}
		return nil, err
	}

	if metric == "" {
		return s.marshalResult(retroTableDocument(rep))
	}

	// A report-level list has no column: aging reads now, so a week beside it
	// is a request the report cannot honour, said rather than dropped.
	if keys, ok := retro.ReportKeysFor(rep, metric); ok {
		if weekGiven {
			return nil, fmt.Errorf("open %s is measured now, not per %s; drop week", metric, rep.BucketNoun())
		}
		return s.marshalResult(map[string]any{"metric": metric, "keys": keys})
	}
	if week < 0 || week >= len(rep.Buckets) {
		return nil, fmt.Errorf("week %d is out of range 0..%d for since %s (0 is the current partial %s)",
			week, len(rep.Buckets)-1, sinceRaw, rep.BucketNoun())
	}
	b := rep.Buckets[len(rep.Buckets)-1-week]
	keys := retro.KeysFor(b, metric)
	if keys == nil {
		keys = []string{}
	}
	return s.marshalResult(map[string]any{
		"metric": metric,
		"week":   week,
		"bucket": b.Label(),
		"keys":   keys,
	})
}

// retroHeavyBucketFields are the per-bucket arrays the table answer leaves
// out, and the reason it can: every one of them is a list of issue keys the
// `open` argument returns on demand, so keeping them in the default answer
// would make the cheapest call the most expensive one. Measured on
// examples/demo.db: the full document is 238 KB over four weeks and 447 KB
// over a year — the second one does not fit the 256 KiB result cap at all.
// Without them the same two are 74 KB and 141 KB, and the numbers, the
// definitions, the surprises, the per-type/per-epic closes, the cycle points,
// the aging tail and the actions are all still there.
var retroHeavyBucketFields = []string{"events", "keys"}

// retroHeavyKeySets are the material objects whose `keys` array is dropped;
// their counts stay.
var retroHeavyKeySets = []string{"unplanned", "seen_not_moved", "moved_not_seen"}

// retroTableDocument is retro.Report.JSON() with the key lists taken out and
// an `omitted` note saying how to get them back. Nothing is recomputed: the
// document is the owner's, and this only decides what fits in an answer.
func retroTableDocument(rep retro.Report) any {
	raw, err := json.Marshal(rep.JSON())
	if err != nil {
		return rep.JSON()
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return rep.JSON()
	}
	buckets, _ := doc["buckets"].([]any)
	for _, entry := range buckets {
		bucket, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		for _, f := range retroHeavyBucketFields {
			delete(bucket, f)
		}
		for _, name := range retroHeavyKeySets {
			if sub, ok := bucket[name].(map[string]any); ok {
				delete(sub, "keys")
			}
		}
	}
	omitted := append([]string{}, retroHeavyBucketFields...)
	for _, name := range retroHeavyKeySets {
		omitted = append(omitted, name+".keys")
	}
	doc["omitted"] = map[string]any{
		"fields": omitted,
		"why": "per-bucket event log and issue-key lists are left out so the default call stays small; " +
			"ask for one cell's keys with open (" + retro.JoinOpenMetrics() + ") and week",
	}
	return doc
}

// readOnlyMirror hands retro.Compute the same mode=ro handle with local.db
// attached that `gadak retro` and the REST endpoint compute against, so this
// tool cannot take the mirror's write lock either.
func (s *Server) readOnlyMirror() (*gosql.DB, error) {
	if err := s.ensureDB(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	db := s.db
	s.mu.Unlock()
	if db == nil {
		return nil, fmt.Errorf("mirror is not open")
	}
	return db.ReadOnly()
}
