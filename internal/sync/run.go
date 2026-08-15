package sync

import (
	"context"
	"errors"
	"time"

	"github.com/midagedev/gadak/internal/atlhttp"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
)

// isRejectedCredential reports whether err is a dead credential from any
// source. Transport errors (500, timeout, DNS) must stay false.
//
// Owner of the detection rule: atlhttp.ErrAuth (what Do returns on 401/403)
// or any error implementing atlhttp.RejectedCredential. A third source that
// uses atlhttp.Do is covered without a branch here. A source that is not
// built on atlhttp still implements the method.
func isRejectedCredential(err error) bool {
	if err == nil {
		return false
	}
	var rc atlhttp.RejectedCredential
	if errors.As(err, &rc) {
		return true
	}
	return errors.Is(err, atlhttp.ErrAuth)
}

// watchSource is one connector in the Watch cycle. Adding a third source
// means appending a row; applyWatchErr owns the auth rule for every row.
type watchSource struct {
	id      string
	phase   string
	failLog string // existing log format: "sync failed: %v" / "confluence sync failed: %v"
	enabled func(*config.Config) bool
	run     func(context.Context, *config.Config, *store.DB, Options) (Result, error)
	// fatal: a rejected credential ends Watch. Jira is fatal (the product
	// cannot mirror issues without it). Confluence is not: Jira must keep
	// ticking when only the wiki side is rejected.
	fatal  bool
	notify bool // fire notifyAfterSync after a successful pass (Jira)
}

func defaultWatchSources() []watchSource {
	return []watchSource{
		{
			id:      SourceID,
			phase:   PhaseIssues,
			failLog: "sync failed: %v",
			enabled: func(*config.Config) bool { return true },
			run:     Run,
			fatal:   true,
			notify:  true,
		},
		{
			id:      ConfluenceSourceID,
			phase:   PhaseDocuments,
			failLog: "confluence sync failed: %v",
			enabled: func(c *config.Config) bool { return c != nil && c.Confluence != nil },
			run:     RunConfluence,
			fatal:   false,
			notify:  false,
		},
	}
}

// applyWatchErr is the single owner of Watch's "rejected credential stops
// retrying this source" rule. Every source in defaultWatchSources goes
// through here. A third source is covered by construction.
//
// Transport errors are logged and the cycle continues. A rejected credential
// is re-recorded on last_error (status / doctor / sync_health read that
// column) and either ends Watch (src.fatal) or marks the source dead so
// later ticks skip it (non-fatal).
func applyWatchErr(ctx context.Context, db *store.DB, src watchSource, err error, logf func(string, ...any), dead map[string]bool) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if logf != nil {
		logf(src.failLog, err)
	}
	if !isRejectedCredential(err) {
		return nil
	}
	if db != nil {
		_ = db.RecordSync(ctx, src.id, store.SyncResult{Err: err})
	}
	if src.fatal {
		return err
	}
	if dead != nil {
		dead[src.id] = true
	}
	return nil
}

// sourceLastErrors is the one-call answer to "why did watch stop, and which
// credential was it?". It reads last_error for every Watch source. Same
// columns `gadak doctor` classifies as sync.<id>.last_error and
// `gadak sql --json "select source_id, last_error from sync_state where last_error is not null"`
// prints.
func sourceLastErrors(ctx context.Context, db *store.DB) (map[string]string, error) {
	out := map[string]string{}
	if db == nil {
		return out, nil
	}
	for _, src := range defaultWatchSources() {
		st, err := db.SyncState(ctx, src.id)
		if err != nil {
			return nil, err
		}
		if st.LastError != nil && *st.LastError != "" {
			out[src.id] = *st.LastError
		}
	}
	return out, nil
}

// sourceIdent names a connector in sources / sync_state / sync_runs.
type sourceIdent struct {
	ID   string // "jira" | "confluence"
	Kind string // store Source.Kind
}

// usageTaker is the client surface flushAPIUsage needs. *jira.Client and
// *confluence.Client both implement it (TakeUsage returns atlhttp.Usage).
type usageTaker interface {
	TakeUsage() atlhttp.Usage
}

// runSource is the shared skeleton for Run and RunConfluence: SyncRun history,
// credential check, usage flush, UpsertSource, SyncState, res.Full, the
// source-specific pass, and the elapsed done line.
//
// setup runs only after the credential check and supplies BaseURL + the client
// whose usage counters are flushed on every exit path. pass owns watermark
// accumulation, RecordSync, and any source-specific work (reconcile, discovery).
//
// SupportsReconcile controls the +reconcile kind suffix. Jira sets true (full
// and opts.Reconcile both trigger reconcile). Confluence sets true once space
// prune ships; the flag is the kind label only — each source still owns its
// reconcile body.
func runSource(
	ctx context.Context,
	cfg *config.Config,
	db *store.DB,
	opts Options,
	src sourceIdent,
	supportsReconcile bool,
	donePrefix string,
	setup func() (baseURL string, usage usageTaker, err error),
	pass func(state store.SyncState, res *Result) error,
) (res Result, err error) {
	started := time.Now()
	// History: keep only runs worth remembering — a change, a full pass, or a
	// failure. The watch loop's no-op incrementals would drown everything else.
	defer func() {
		if err == nil && !res.Full && res.Changed == 0 && res.Deleted == 0 {
			return
		}
		kind := syncRunKind(res.Full, supportsReconcile && (opts.Reconcile || res.Full))
		run := store.SyncRun{
			Kind:       kind,
			StartedAt:  started.UTC().Format(time.RFC3339),
			FinishedAt: time.Now().UTC().Format(time.RFC3339),
			Fetched:    res.Fetched,
			Changed:    res.Changed,
			Deleted:    res.Deleted,
		}
		if err != nil {
			run.Error = err.Error()
		}
		// Bookkeeping must never fail the sync that produced it.
		_ = db.AppendSyncRun(ctx, src.ID, run)
	}()

	if cfg == nil || !cfg.HasCredential() {
		return res, errors.New("sync: site, email and token are required")
	}
	baseURL, usage, err := setup()
	if err != nil {
		return res, err
	}
	// Flush call-volume counters into the mirror for every exit path of this
	// pass (success, transport failure, auth failure). Instrumentation must
	// never fail the sync itself — see flushAPIUsage. Watch also lands here
	// once per cycle, so one-shot and watch share this single flush point.
	if usage != nil {
		defer flushAPIUsage(ctx, db, usage, opts.logf)
	}
	if err := db.UpsertSource(ctx, store.Source{ID: src.ID, Kind: src.Kind, BaseURL: baseURL}); err != nil {
		return res, err
	}
	state, err := db.SyncState(ctx, src.ID)
	if err != nil {
		return res, err
	}
	// No watermark means nothing has been mirrored yet, so incremental has no
	// floor to start from.
	res.Full = opts.Full || state.Watermark == ""

	if err := pass(state, &res); err != nil {
		return res, err
	}

	elapsed := time.Since(started).Round(time.Second)
	opts.logf("%sdone: %s fetched, %s changed, %s deleted in %s",
		donePrefix,
		formatCount(res.Fetched), formatCount(res.Changed), formatCount(res.Deleted), elapsed)
	return res, nil
}

// syncRunKind is the SyncRun.Kind string: "full" | "incremental", optionally
// suffixed with "+reconcile" when the pass is reconcile-class.
func syncRunKind(full, reconcile bool) string {
	kind := "incremental"
	if full {
		kind = "full"
	}
	if reconcile {
		kind += "+reconcile"
	}
	return kind
}

// record stores last_error for sourceID and returns the error unchanged. It
// passes no watermark: a failed run must not advance it.
func record(ctx context.Context, db *store.DB, sourceID string, err error) error {
	_ = db.RecordSync(ctx, sourceID, store.SyncResult{Err: err})
	return err
}

// flushAPIUsage takes the client's process-local counters and accumulates them
// into api_usage for the current UTC day. Jira and Confluence clients both
// satisfy usageTaker. One-shot sync and each Watch cycle go through runSource,
// so this is the single flush point per source pass.
//
// A flush failure is logged and swallowed: rate-limit visibility must not break
// the sync that produced the traffic. api_usage is day-keyed with no source
// column — both connectors accumulate into the same daily row (schema contract).
func flushAPIUsage(ctx context.Context, db *store.DB, c usageTaker, logf func(string, ...any)) {
	if db == nil || c == nil {
		return
	}
	u := c.TakeUsage()
	if u.Requests == 0 && u.Throttled == 0 && u.ServerErrors == 0 && u.Retries == 0 && u.WaitMS == 0 {
		return
	}
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
	if err := db.AddAPIUsage(ctx, day, delta); err != nil {
		if logf != nil {
			logf("api usage flush: %v", err)
		}
	}
}
