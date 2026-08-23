package sync

import (
	"context"
	"errors"
	"time"

	"github.com/midagedev/gadak/internal/atlhttp"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/store"
)

// IsRejectedCredential reports whether err is a dead credential from any
// source. Transport errors (500, timeout, DNS) must stay false.
//
// Owner of the detection rule: atlhttp.ErrAuth (what Do returns on 401/403)
// or any error implementing atlhttp.RejectedCredential. A third source that
// uses atlhttp.Do is covered without a branch here. A source that is not
// built on atlhttp still implements the method. Exported for the server's
// sync progress document — every surface reads this one rule as a code
// instead of matching error prose.
func IsRejectedCredential(err error) bool {
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
			enabled: func(c *config.Config) bool { return c != nil && c.HasAtlassianCredential() },
			run:     Run,
			fatal:   true,
			notify:  true,
		},
		{
			id:      LinearSourceID,
			phase:   PhaseIssues,
			failLog: "linear sync failed: %v",
			enabled: func(c *config.Config) bool { return c != nil && c.Linear != nil },
			run:     RunLinear,
			// Like Confluence: Jira mirroring must keep ticking when only the
			// Linear key is rejected.
			fatal:  false,
			notify: false,
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
func applyWatchErr(ctx context.Context, cfg *config.Config, db *store.DB, src watchSource, err error, logf func(string, ...any), dead map[string]bool) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if logf != nil {
		logf(src.failLog, err)
	}
	if !IsRejectedCredential(err) {
		return nil
	}
	if db != nil {
		_ = record(ctx, cfg, db, src.id, err)
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

// Source kinds — the Kind column of sources / sync_state rows, one value per
// connector. Single owner (GDK-619): every writer of the column and every
// comparison against it goes through these, never a bare literal.
const (
	KindJira       = "jira"
	KindConfluence = "confluence"
	KindLinear     = "linear"
)

// sourceIdent names a connector in sources / sync_state / sync_runs.
type sourceIdent struct {
	ID   string // SourceID | ConfluenceSourceID | LinearSourceID
	Kind string // store Source.Kind — a Kind* constant above
}

// usageTaker is the client surface FlushAPIUsage needs. *jira.Client and
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
		if err != nil && store.IsBusy(err) {
			// GDK-754: AppendSyncRun is another write(). A holder that
			// just failed the pass will fail this too, stacking a third
			// busy_timeout cycle onto pass + last_error. Skip; the CLI
			// error already carries the holder hint.
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

	if src.Kind != KindLinear && (cfg == nil || !cfg.HasAtlassianCredential()) {
		// Per-source gate: the Linear pass authenticates with its own key
		// (checked in its setup); Jira-family sources need the Atlassian
		// credential, not a Linear API key. HasCredential now
		// counts Linear, so this check must stay the Jira-family half.
		return res, errors.New("sync: site, email and token are required")
	}
	baseURL, usage, err := setup()
	if err != nil {
		return res, err
	}
	// Flush call-volume counters into the mirror for every exit path of this
	// pass (success, transport failure, auth failure). Instrumentation must
	// never fail the sync itself — see FlushAPIUsage. Watch also lands here
	// once per cycle, so one-shot and watch share this single flush point.
	if usage != nil {
		defer FlushAPIUsage(ctx, db, usage, opts.logf)
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

	err = pass(state, &res)
	if err != nil && store.IsBusy(err) {
		// One pass-level retry after a short backoff. write() already
		// waited up to writeBusyAttempts × busy_timeout (~10s). last_error
		// and AppendSyncRun writes are skipped on BUSY, so a persistent
		// holder costs at most two write() cycles plus this backoff
		// (~20.05s) — it does not exceed the previous ~20s double-pay
		// (pass + last_error). A holder that drops during the backoff
		// succeeds on the retry.
		if werr := sleepSyncBusyBackoff(ctx); werr != nil {
			return res, err
		}
		err = pass(state, &res)
	}
	if err != nil {
		return res, store.WithBusyHint(err)
	}
	if res.Full && src.Kind != KindConfluence {
		// GDK-755: per-page upserts recompute only the batch and its
		// dependents. One full-table sweep at the end of a full issue
		// sync covers anything a page-scoped walk could miss (the v11
		// migration SQL is the other remaining full sweep). Confluence
		// has no issues.epic_key.
		if err = db.RecomputeEpicKeys(ctx); err != nil {
			return res, err
		}
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
// passes no watermark: a failed run must not advance it. FoldPairedError runs
// before the store so a paired failure's first line is the pairing sentence
// (GDK-485); connected/standalone errors pass through.
func record(ctx context.Context, cfg *config.Config, db *store.DB, sourceID string, err error) error {
	if store.IsBusy(err) {
		// GDK-754: last_error is itself a write(). A holder that just
		// failed the pass will fail this too, stacking a second
		// busy_timeout cycle. RecordSync also refuses BUSY last_error
		// writes; skip the call so we do not wait it out. The returned
		// error already names the holder (write() → WithBusyHint).
		return store.WithBusyHint(err)
	}
	_ = db.RecordSync(ctx, sourceID, store.SyncResult{Err: origin.FoldPairedError(cfg, err)})
	return err
}

// syncBusyPassBackoff is 1% of production busy_timeout(5000), matching
// store.writeBusyBackoff. Combined with one pass retry the persistent-holder
// failure is bounded by ~20s (see runSource).
const syncBusyPassBackoff = 50 * time.Millisecond

func sleepSyncBusyBackoff(ctx context.Context) error {
	timer := time.NewTimer(syncBusyPassBackoff)
	select {
	case <-ctx.Done():
		timer.Stop()
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// FlushAPIUsage takes the client's process-local counters and accumulates them
// into api_usage for the current UTC day. Jira and Confluence clients both
// satisfy usageTaker. One-shot sync, each Watch cycle, and `gadak api` go
// through this function so CLI and sync share one flush policy: a failure is
// logged and swallowed. api_usage is day-keyed with no source column — both
// connectors accumulate into the same daily row (schema contract).
func FlushAPIUsage(ctx context.Context, db *store.DB, c usageTaker, logf func(string, ...any)) {
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
		delta.LastThrottledAt = u.LastThrottledAt.UTC().Format(config.ISOMilli)
	}
	day := time.Now().UTC().Format("2006-01-02")
	if err := db.AddAPIUsage(ctx, day, delta); err != nil {
		if logf != nil {
			logf("api usage flush: %v", err)
		}
	}
}
