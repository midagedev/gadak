# Gates

Objective bars for calling a phase done. A gate is passed by evidence — a command
and its output — not by judgment. Work may proceed past a failed gate only for
independent research.

Ordering and task detail live in `tasks.md` and `../../docs/ROADMAP.md`.

## G0 — Extraction Readiness

**Passed.**

| Check | Evidence |
| --- | --- |
| No installation-specific value in shipped code | `grep -rn` for site URL, company name, project keys, team labels, internal paths across `web/`, `tools/`, `cmd/` returns nothing outside example values |
| The UI boots with no server | `npm run build` succeeds; opening `dist/app/index.html` paints the shell and falls back to config defaults |
| Contracts written down | `contracts/api.md`, `contracts/sync.md`, `contracts/agent.md`, `data-model.md` exist and describe what the client actually sends |
| `scry serve` serves the built UI and config | `curl /healthz`, `/config.json`, and `/` all succeed in CI |
| Non-loopback bind refused | CI asserts `scry serve --addr 0.0.0.0:7778` exits non-zero |

## G1 — Store Contract

**Passed.** `CGO_ENABLED=0 go test ./internal/store/`.

| Check | Evidence |
| --- | --- |
| Schema matches `data-model.md` exactly | `TestSchemaMatchesDataModel` compares `PRAGMA table_info` for all fourteen tables against the documented column lists, in document order |
| Migrations apply forward from empty and from each released version | `TestMigrateForwardIsIdempotent`: empty file to current level, then reopening an already-migrated database applies nothing and preserves its rows. Only one version is released so far, so "each released version" is one case |
| A newer schema version is refused, not silently used | `TestOpenRefusesNewerSchema` sets `user_version` past the binary's level and asserts `Open` fails with an error naming the mismatch |
| Every documented example query runs | `TestDocumentedExampleQueries` executes all four queries from `data-model.md` verbatim against a fixture and asserts each returns the rows it should |
| WAL enabled; a reader is not blocked by a writer | `TestPragmas` for the pragmas; `TestWALReaderNotBlockedByWriter` opens a second connection and reads while a write transaction is held open, failing on a 2 s timeout (`busy_timeout` is 5 s, so a blocked reader hangs rather than erroring) |

## G2 — Sync Correctness

| Check | Evidence |
| --- | --- |
| Full sync of the demo site produces the expected issue count | Timed run against the demo snapshot fixture |
| Incremental sync is idempotent | `TestIncrementalRerunIsANoOp`: a second run fetches every issue over the overlap window, changes none, and leaves `sync_state.version` and the watermark where they were |
| Watermark never regresses, and never advances past an uncommitted page | `TestFailedPageKeepsMirrorAndWatermark` injects a 500 on the second page: the watermark is still empty afterwards, and the store's `RecordSync` refuses a lower value by construction |
| A failed sync leaves the previous mirror readable and records `last_error` | Same test: the first page's two issues are still readable and `last_error` names the 500. `TestAuthFailureAbortsWithoutRetry` covers the credential case, which aborts on the first response |
| Deletions propagate through the reconcile pass | `TestReconcileDeletesVanishedKeys`: two keys vanish upstream, both leave the mirror and both appear in `deleted_items`. The same test asserts an upstream that returns nothing at all is refused rather than treated as a mass deletion |
| Derived fields match hand-computed values | `TestDerive` for the calculator; `TestFullSyncMapsEverything` for the same over a sync, asserting `reopen_count`, `reopened_at`, `status_changed_at`, a `resolved_at` cleared by a reopen, and `priority_rank` from the site's priority order |
| No logic keys on a localized name | `TestDerive` runs every case in both languages; `TestDerivedFieldsIgnoreDisplayLanguage` syncs a whole English and a whole Korean fixture site and asserts every derived column is identical, with only the display name differing |

## G3 — Read API

| Check | Evidence |
| --- | --- |
| `bootstrap` returns the documented shape and an `ETag`; repeat with `If-None-Match` gives `304` | Handler test |
| `delta` reports `upserted` and `deleted_keys` correctly across a window | Handler test against a mutated fixture |
| `detail` assembles description, comments, history, links, attachments | Handler test |
| `search` finds text present only in a comment body | Handler test |
| Deferred endpoints return `404`, never `500` | Handler test over the deferred list |
| The UI works end to end against a real mirror with Jira unreachable | Browser smoke test with outbound blocked |

## G4 — Write-through

| Check | Evidence |
| --- | --- |
| Each write endpoint calls Jira and refreshes the affected row before responding | `TestTransitionWritesThroughToTheMirror`: the fake Jira returns a *different* status on re-read, and the response carries it — as does the next `bootstrap`, with a moved ETag. Every other write asserts the same round trip |
| Jira's error body is passed through, including field errors | `TestJiraErrorsPassThrough`: `error` from `errorMessages`, `jira_errors` from `errors`, status preserved for 4xx |
| A missing credential yields `409 credential_required` | `TestWritesRequireACredential` over all nine credentialed endpoints, plus the same code when Jira rejects a stored token |
| The credential file is `0600` and the token never appears in a response, log, or the database | `TestCredentialLifecycle` stats the file; `TestTokenNeverReachesResponsesOrLogs` greps five response bodies, the config document, captured logs, and the mirror (`.db` and `-wal`) |
| Field edits outside the configured allowlist are rejected | `TestFieldEditAllowlistAndShapes`: refused with `403 field_not_editable` and, crucially, no request reaches Jira |

## G5 — Latency

Measured on Apple M4 Pro (darwin/arm64) with `make bench` (2026-08-04).
Fixture: synthetic 10k issues via `tools/bench-fixture` / `seedBenchDB` (seed 42).
These are not CI fail gates — wall times vary by machine; re-run `make bench` to refresh.

| Check | Evidence |
| --- | --- |
| Filter, group, and sort stay under 50 ms at 10k issues | Client-side filter/group/sort is in-memory over the bootstrap payload (no network on keystroke — constitution). Server-side measurements for the payload itself: **bootstrap** `BenchmarkBootstrap10k` ≈ **61.5 ms/op** (66.9 MB/op, 600k allocs); **FTS search** `BenchmarkSearch10k` ≈ **0.063 ms/op** (3.0 KB/op). Bootstrap is slightly over the 50 ms product target on this machine; search is well under. Re-measure with `make bench`. |
| Cold start to interactive under 1 s with a warm cache | Browser smoke measurement (not re-run this pass) |
| No network request on a keystroke path | Smoke test records zero requests while typing into the search box |
| Server memory under 100 MB at 10k issues | Bootstrap bench allocs ≈ 67 MB/op on the response path; process RSS not separately profiled. Fixture DB ≈ 10 MB on disk (`go run ./tools/bench-fixture -out /tmp/bench.db -issues 10000`) |

## G6 — Demo and Public Readiness

| Check | Evidence |
| --- | --- |
| `scry demo` works with no configuration and no credentials | Fresh-container run |
| `examples/demo.db` contains no credential-shaped string and no real data | Scanner output in CI |
| Snapshot timestamps are spread realistically | Distribution check on the fixture |
| UI copy is English (with the original strings kept as a locale) | Manual review; blocks any public launch |
| README claims match `tasks.md` | Manual review, each claim traced to a passed gate |
