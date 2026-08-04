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

| Check | Evidence |
| --- | --- |
| Schema matches `data-model.md` exactly | Test compares `PRAGMA table_info` against the documented columns |
| Migrations apply forward from empty and from each released version | Migration test over fixture databases |
| A newer schema version is refused, not silently used | Test asserts a clear error |
| Every documented example query runs | Test executes each query from `data-model.md` against a fixture |
| WAL enabled; a reader is not blocked by a writer | Test reads during an open write transaction |

## G2 — Sync Correctness

| Check | Evidence |
| --- | --- |
| Full sync of the demo site produces the expected issue count | Timed run against the demo snapshot fixture |
| Incremental sync is idempotent | Running it twice with no upstream change leaves row content and `version` unchanged |
| Watermark never regresses, and never advances past an uncommitted page | Test with an injected failure mid-page |
| A failed sync leaves the previous mirror readable and records `last_error` | Test with an injected transport error |
| Deletions propagate through the reconcile pass | Fixture where a key vanishes upstream |
| Derived fields match hand-computed values | Table test over changelog fixtures, including a done-to-todo reopen and a multi-reopen issue |
| No logic keys on a localized name | Test runs the same fixture with Korean and English display names and gets identical derived output |

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
| Each write endpoint calls Jira and refreshes the affected row before responding | Integration test against the demo site |
| Jira's error body is passed through, including field errors | Test with a deliberately invalid field edit |
| A missing credential yields `409 credential_required` | Handler test |
| The credential file is `0600` and the token never appears in a response, log, or the database | Test greps the database file and captured logs for the token |
| Field edits outside the configured allowlist are rejected | Handler test |

## G5 — Latency

| Check | Evidence |
| --- | --- |
| Filter, group, and sort stay under 50 ms at 10k issues | Bench over the synthetic 10k snapshot, recorded in the run log |
| Cold start to interactive under 1 s with a warm cache | Browser smoke measurement |
| No network request on a keystroke path | Smoke test records zero requests while typing into the search box |
| Server memory under 100 MB at 10k issues | Observed during the bench run |

## G6 — Demo and Public Readiness

| Check | Evidence |
| --- | --- |
| `scry demo` works with no configuration and no credentials | Fresh-container run |
| `examples/demo.db` contains no credential-shaped string and no real data | Scanner output in CI |
| Snapshot timestamps are spread realistically | Distribution check on the fixture |
| UI copy is English (with the original strings kept as a locale) | Manual review; blocks any public launch |
| README claims match `tasks.md` | Manual review, each claim traced to a passed gate |
