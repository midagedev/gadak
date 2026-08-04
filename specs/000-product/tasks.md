# Tasks

Current state of every piece of v0.1. This file is the honest inventory — if
something is marked done here, it works; if it is not, no amount of README
enthusiasm changes that.

Legend: **done** / **partial** / **todo**

## T0 Extraction and repository shape

| # | Task | State | Notes |
| --- | --- | --- | --- |
| T0.1 | Move the web application into `web/` | done | 73 source files, Svelte 5 + Vite + Tailwind 4 |
| T0.2 | Runtime config layer replacing hardcoded internals | done | `web/src/lib/config.ts`, fetched before mount |
| T0.3 | Strip every internal string from the app source | done | Verified: no site URL, company name, project key, team label, or internal path in `web/` |
| T0.4 | Generalize built-in views to tenant-neutral axes | done | Presets now use only `status_category`, `unassigned`, `reopened`, `stale`, `updated_from` |
| T0.5 | Generalize the media-URL allowlist to the configured API base | done | Security-relevant: still an exact-shape whitelist |
| T0.6 | Build config at repo root (`package.json`, `vite.config.ts`, `tsconfig.json`) | done | Vite root is `web/`, output `dist/app` |
| T0.7 | Governance and license files | done | Apache-2.0 |
| T0.8 | Specs, contracts, and architecture docs | done | This directory plus `docs/` |
| T0.9 | Remove or plugin-ize the cut internal surfaces still present in the UI | done | Resolved by feature-flag gating, not deletion: `PrList` and `DeployTimeline` sit behind `features.deploy`, `QaImpact` and `QaFieldEditor` behind `features.qa`. The components stay in the tree so a tenant that has the data can switch them on |
| T0.10 | Translate the UI to English with the current copy as a locale | done | `web/src/lib/i18n/` — thin `t()` + `en.ts`/`ko.ts` catalogs (~520 keys, key sets typechecked against each other), locale from `scry_locale` in localStorage > `navigator.language` > `en`. All three relative-time implementations unified; composed sentences rewritten as whole message keys. Source comments stay Korean by design |
| T0.11 | Wire the `features` flags to actual consumers | done | The flags were declared in `config.ts` and read by nobody, so `presence` still opened a WebSocket retry loop against a missing endpoint and the deploy/QA/team-group columns stayed in the catalog. Gating now runs through `feature()` in `config.ts` |
| T0.12 | Compute staleness from `status_changed_at` | done | The UI read `working_hours_in_status`, a field no server populates (see `data-model.md`), so the "stale" view was always empty. Threshold is `staleThresholdHours` in config, default 72 |

## T1 Storage

| # | Task | State | Notes |
| --- | --- | --- | --- |
| T1.1 | Schema and migrations per `data-model.md` | done | `internal/store`, driver `modernc.org/sqlite` (no cgo). `TestSchemaMatchesDataModel` compares every table against the documented column list; `TestDocumentedExampleQueries` runs all four example queries |
| T1.2 | WAL mode, sane pragmas, single-writer discipline | done | WAL + `busy_timeout=5000` + `foreign_keys=ON` + `synchronous=NORMAL` in the DSN so every pooled connection gets them; writes serialized by one mutex. `TestWALReaderNotBlockedByWriter` |
| T1.3 | FTS5 table and rebuild-on-sync path | done | Contentless with `contentless_delete=1`; a changed item's row is deleted and re-inserted inside the same transaction as the upsert. Rebuild-on-sync is exercised by `TestChangedIssueReplacesChildrenAndDerivedFields` |
| T1.4 | `saved_views` / `watches` / `favorites` plus `scry export` | partial | Tables and CRUD done (`TestPersonalState`); `scry export` not written |
| T1.5 | Schema-version check on open, refusing a newer database | done | `PRAGMA user_version` is the level, mirrored into `sync_state.schema_version`. `TestOpenRefusesNewerSchema` |

## T2 Jira connector

| # | Task | State | Notes |
| --- | --- | --- | --- |
| T2.1 | REST client with Basic auth, retry, and backoff | done | `internal/jira`, stdlib only. 429/5xx exponential backoff capped at 30 s, `Retry-After` honoured, 401/403 aborts as `jira.ErrAuth`. `TestRetriesThenSucceeds`, `TestAuthFailureAbortsWithoutRetry` (which also asserts the token never reaches an error string) |
| T2.2 | Full sync with token pagination and explicit field lists | done | `sync.Run` with `Options{Full:true}`: `project = KEY ORDER BY created ASC`, `POST /search/jql` with `nextPageToken`, explicit field list, one store transaction per page. `TestFullSyncMapsEverything` over a two-page fixture, `TestSearchFollowsPageTokens` for the cursor |
| T2.3 | Incremental sync with watermark and overlap window | done | Watermark advances only past a committed page and is stored verbatim, so the JQL floor can be rendered in the offset Jira stamps on `updated` (a bare JQL timestamp is read in account time). `TestIncrementalRerunIsANoOp` pins the two-minute overlap and asserts `version` does not move; `TestFailedPageKeepsMirrorAndWatermark` injects a mid-run 500 and asserts the committed page survives, the watermark stays put and `last_error` is recorded |
| T2.4 | Changelog paging for long histories | done | `changelog.total > len(histories)` triggers `GET /issue/{key}/changelog` paging. `TestTruncatedChildrenArePaged` derives a reopen that exists only in the paged history; `TestChangelogAndCommentsPageToTotal` covers the client's paging |
| T2.5 | Comment paging | done | Same shape for `comment.total`. Covered by the two tests above (`comment_count` = 3 from a fixture that inlines one) |
| T2.6 | Reconcile pass for deletion detection | done | Key-only pass after every full sync and on `Options{Reconcile:true}`, diffed against the mirror's in-scope keys. Refuses to delete when upstream reports nothing at all, which is a scoping failure rather than a mass deletion. `TestReconcileDeletesVanishedKeys` covers both |
| T2.7 | Derived field computation | done | The connector supplies `GET /status` (id → category) and `GET /priority` (rank order) per batch, plus each issue's own status id so a status missing from the site list still resolves. `TestDerivedFieldsIgnoreDisplayLanguage` runs the whole sync against an English and a Korean site and asserts identical derived output |
| T2.8 | Configurable field mapping and body fields | done | `fieldMap` aliases land in `issues.custom`, `bodyFields` are flattened into `items.body_text`, and both are appended to the requested field list. `TestFullSyncMapsEverything` asserts the alias in `custom` and finds a term that exists only in a body field (and one only in a comment) through FTS |

## T3 Server

| # | Task | State | Notes |
| --- | --- | --- | --- |
| T3.1 | `scry serve` with static UI, `config.json`, `/healthz` | done | `cmd/scry/main.go` |
| T3.2 | Loopback-only bind with an explicit `--allow-remote` escape | done | The mirror has no auth; this is the only thing protecting it |
| T3.3 | `bootstrap` and `delta` from SQLite, with ETag | done | `internal/server`. `"sv-<version>"` ETag with a 304 path that also accepts the client's own `"in-<version>"` hydration tag; `delta` drops the member payload when `mv` matches. `d1_group` is injected from `groupRules` (assignee's configured group as the fallback) and configured field aliases are spread from `issues.custom` into the row. `TestBootstrapShapeAndETag`, `TestDeltaUpsertedAndDeleted`, `TestIssueLiteFieldNames` |
| T3.4 | `detail` assembly | done | Status history carries `from_category`/`to_category` resolved through the mirror's own status id → category map, so a reopen is never inferred from a localized name. `TestDetailAssembly` |
| T3.5 | `search` over FTS5 | done | `{keys, total}` straight from `store.Search`. `TestSearchHitsCommentText` |
| T3.6 | Attachment content proxy | done | Streams from Jira with Basic auth, no credential → `409 credential_required`, a rejected token → the same so the UI reopens its dialog. `nosniff` always, and anything scriptable (SVG included) is forced to download. `TestAttachmentProxyStreamsFromJira` |
| T3.7 | Saved views and watches endpoints | done | Local only, never 401/403. `TestPersonalStateRoundtrip` |
| T3.8 | Deferred endpoints returning a clean `404` | done | One catch-all under both bases, with an `{"error": …}` body. `TestDeferredEndpointsAre404` |
| T3.9 | `settings/` read and write for the settings UI | done | Credential-free config projection; a write preserves the credential block and invalidates the cached member/group projection. `TestSettingsRoundtripPreservesCredential`, `TestWebConfigHidesCredential` |
| T3.10 | Merge plugin `enrichments` into the list and detail responses | done | `docs/PLUGINS.md` is the payload contract. Wrapped or bare payloads both work, invalid JSON is dropped instead of corrupting the document, and an enrichment can never shadow a mirrored field. `TestEnrichmentsMerge`, `TestEnrichmentCannotShadowMirroredFields` |
| T3.11 | Runnable example plugins + extending guide | done | `examples/plugins/{github-prs,deploy-status,csv-import}/` (Python 3 stdlib; `--db`/`--dry-run`/idempotent upsert + `sync_state.version` bump; `--from-json` offline path for github-prs). `docs/EXTENDING.md` (config / enrichments / SQL axes + recipes). `docs/PLUGINS.md` payload tables verified against `internal/server/read.go` + `web/src/lib/types.ts`. `make plugins-test` runs all three self-tests. Evidence: copy `examples/demo.db` → temp, run github-prs `--from-json` + csv-import twice → row counts stable, version +2 per plugin pair; deploy-status `--self-test` builds a temp git repo |
| T3.12 | First-run onboarding in the browser (`serve` only, no CLI step) | done | `internal/server/onboarding.go`: `PUT onboarding/connect/` (verifies via `/myself` and is the only writer of `site`), `GET projects/available/` (proxies `/project/search`, 500-project cap with `truncated`), `POST sync/` (single-flight background full sync, `409 sync_in_progress`) and `GET sync/progress/` (counters only, fed by the new `sync.Options.Progress` per-page callback). UI: `web/src/components/shell/Onboarding.svelte` is a 3-step wizard — connect, live project picker, first sync whose count climbs and then lands on the filled list via `issues.refresh()`, no reload. `internal/server/onboarding_test.go`, `e2e/onboarding.spec.ts` |

## T4 Write-through

| # | Task | State | Notes |
| --- | --- | --- | --- |
| T4.1 | Credential store at `0600`, verified against `/myself` | done | Verified before it is stored, so a typo never becomes the stored credential. `GET` answers with a `…last4` hint and never the token. `TestCredentialLifecycle` asserts the file mode, `TestRejectedCredentialIsNotStored` the rejection path |
| T4.2 | Transitions (list and execute) | done | `to_category` is Jira's own category key, which is what the client's type documents. `TestTransitionWritesThroughToTheMirror`, `TestTransitionsAndUsersAndCreateMeta` |
| T4.3 | Comment with mentions and attachments | done | `@Display Name` plus the resolved account ids become ADF mention nodes — a mention left as plain text notifies nobody. Longest name wins so `@김현` cannot shadow `@김현철` (`jira.TestDocTurnsMentionsIntoNodes`). `attachment_ids` is accepted and ignored: the files are already on the issue, and inlining them needs a media id Jira only exposes through the attachment redirect |
| T4.4 | Attachment upload | done | Multipart pass-through with Jira's required `X-Atlassian-Token`, capped at 64 MB. `TestUploadProxiesAndReturnsContentURL` |
| T4.5 | Assignee change | done | `TestAssigneeSetAndClear`, which also covers the PUT route it shares with `watches/{key}/` |
| T4.6 | Field edit against a configured allowlist | done | Refused server-side whatever the UI offered, and refused again when Jira says the field is not editable on that issue. The value shape per kind (`option` / `user` / `version_array`) comes from Jira's editmeta schema, so no field id is hardcoded. `TestFieldEditAllowlistAndShapes`, `TestEditMetaOnlyExposesAllowlistedFields` |
| T4.7 | Issue creation and create-meta | done | Filing outside the mirrored projects is refused up front: the re-read would never find the issue. `TestCreateIssue` |
| T4.8 | Re-read and mirror refresh after every write | done | `sync.SyncIssue` — the same mapping and derived-field code a scheduled sync uses, with `Force` so the rewrite moves `synced_at` and the version, which is what makes the next delta and the ETag agree. A write that lands but fails to re-read reports `write_applied_mirror_stale` rather than a failure the user would retry |
| T4.9 | `meta/write/` boot cache | partial | `create_meta` is real; the transition map is empty because the client already falls back to fetching an issue's transitions when the menu opens. No credential answers 200 with empty rather than blocking the boot |

## T5 Agent access

| # | Task | State |
| --- | --- | --- |
| T5.1 | Documented schema | done (`data-model.md`) |
| T5.2 | `scry sql` read-only query path | done — `mode=ro` connection, tab-separated, `--json`, or `--csv`. NULL prints as empty in the row formats, not as Go's `<nil>` |
| T5.3 | `scry status --json` | done |
| T5.4 | MCP server | done — `scry mcp` stdio JSON-RPC (`internal/mcp`, no SDK). Tools: `scry_query` (SELECT/WITH only, row/byte caps), `scry_search`, `scry_issue`, `scry_status`. Protocol round-trip + write-SQL rejection tests in `internal/mcp/*_test.go`; docs in `docs/MCP.md` |
| T5.5 | `scry issue <KEY> [--json]` | done — `store.Detail` plus the list row; `--json` is the `GET <key>/detail/` document with `issue` added. `TestIssueAndSearchReadTheMirror` |
| T5.6 | `scry search <text> [--limit] [--json]` | done — FTS over titles, bodies and comments, best match first. Text output is `key\tstatus\tassignee\tsummary`, so `cut -f1` gives keys |
| T5.7 | `scry comment <KEY> -m <text\|->` | done — write-through via `jira.AddComment` + `sync.SyncIssue`, same order as the server. Body is plain text: an `@Name` notifies nobody, unlike the UI's mention picker. `TestCommentSendsADFAndRefusesAnEmptyBody` |
| T5.8 | `scry transition <KEY> <status-or-id>` | done — matches a transition id, its name, or its **target status** name (case-insensitive), because a caller knows the status it wants, not the verb the workflow uses. A miss lists what is available. `TestTransitionMatchesByNameAndReportsAlternatives` |
| T5.9 | `scry assign <KEY> <email\|->` | done — configured member directory first, then Jira's user search; a bare `-` unassigns without asking Jira anything. Ambiguous matches are refused with the candidates, never guessed. `TestAssignResolvesEmailAndUnassigns` |
| T5.10 | Staleness warning on every issue command | done — one stderr line when the last sync failed or is over an hour old, so stdout stays pipeable. `contracts/agent.md`, "Staleness" |
| T5.11 | `AGENTS.md` as the agent-facing reference | done — SQL cookbook, CLI reference, REST examples, staleness; `docs/AGENT_ACCESS.md` is the three-layer map that points at it |

Writes with no stored credential fail before any Jira call
(`TestWritesRefuseToRunWithoutACredential`), and a write that lands but fails to
re-read says so rather than reporting a failure the caller would retry.

CLI wiring beyond T5: `init` (interactive, verifies against `/myself` before saving),
`sync --full/--watch`, `serve --sync`, `demo`, `profiles`, and a global
`--profile <name>` / `SCRY_PROFILE` that keeps separate credentials and mirrors
under `~/.scry/profiles/<name>/` — the work/demo dual-account setup.

## T6 Demo and fixtures

| # | Task | State | Notes |
| --- | --- | --- | --- |
| T6.1 | Jira seeding tool | done | `tools/seed-demo`: dataset-driven or generated, plus `--repair-states` |
| T6.2 | Public demo site populated | done | 519 issues across three fictional products. Categories: 209 todo / 144 in progress / 166 done. Status-change depth 0–7 per issue. 95 reopen transitions, 339 issues with comments, 264 assigned, 61 link edges |
| T6.3 | Authored (non-templated) issue bodies | done | 210 of the 519 are hand-authored: 210/210 unique summaries, 642/642 unique paragraphs, 339/339 unique comments. The other 309 are procedurally generated and visibly more repetitive in the detail panel |
| T6.8 | Demo assignee display names | partial | The **committed snapshot** (`examples/demo.db`) is clean: emails and display names rewritten to fictional personas (Dana Whitfield, Marco Reyes, Priya Sharma, Alex Kim) and the site URL to `nimbus.example.com`, so `scry demo` and screenshots of it are safe. The **live site** still shows `midagedev+…` until each invitation is accepted; live-site screenshots stay blocked |
| T6.4 | `scry snapshot` with timestamp spreading and volume scaling | todo |
| T6.5 | `examples/demo.db` committed, credential-scanned | done | 519 issues mirrored from the demo site, then scrubbed (see T6.8). Scan: zero `ATATT`/`ATCTT`/real emails/real names |
| T6.6 | `scry demo` serving the bundled snapshot | done | Copies the snapshot into a throwaway temp home, serves it, deletes on exit. No Jira account, no config |
| T6.7 | 10k-issue benchmark fixture for the latency target | done | `tools/bench-fixture` builds a deterministic 10k `scry.db` via `store.UpsertIssues` (`go run ./tools/bench-fixture -out /tmp/bench.db -issues 10000`). `internal/server/bench_test.go`: `TestBenchSmoke1k` + `BenchmarkBootstrap10k` / `BenchmarkSearch10k`. `make bench` records timings; not a CI fail gate (machine variance). Evidence in `gates.md` G5. |

## T7 CI and release

| # | Task | State |
| --- | --- | --- |
| T7.1 | CI: Go build and vet, frontend typecheck and build | done |
| T7.2 | Go tests once there is Go logic to test | done |
| T7.3 | Browser smoke test against the demo snapshot | done | Superseded by the Playwright suite (`e2e/`, 15 specs over `examples/demo.db`, CI job `e2e`) — it covers boot, search, detail, palette, settings, enrichment, locale, onboarding, and console hygiene. |
| T7.4 | Secret and internal-string scan in CI | done | `scripts/scan-internal.sh` greps `git ls-files` + `strings examples/demo.db` for token-shaped API-token prefixes, a former company name, and non-allowlisted tenant hosts. CI job `scan` in `.github/workflows/ci.yml`. Real-name patterns skipped. Local: `make scan`. |
| T7.5 | Dockerfile and container build | done | Multi-stage `Dockerfile` (node:20 → golang:1.25 CGO=0 → distroless/static). Volume `/data` as `SCRY_HOME`, `EXPOSE 7777`, `ENTRYPOINT ["scry"]` + `CMD serve --addr 0.0.0.0:7777 --allow-remote` (the UI is embedded, so no `--static`). `.dockerignore` present. README documents `docker run`. **Verified**: `docker build` → 24.1 MB image; `docker run -p 7941:7777` answers `healthz` 200, serves the embedded `index.html`, and `bootstrap/` returns 200. |
| T7.6 | Release process and signed binaries | done | `.goreleaser.yaml`: linux/darwin/windows × amd64/arm64, `CGO_ENABLED=0`, archives include `dist/app` (no `go:embed` yet — serve with `--static dist/app`). Checksums only (cosign deferred, commented). `.github/workflows/release.yml` on `v*` tags: npm build then goreleaser-action. |

## T8 Keyboard and command surface

| # | Task | State | Notes |
| --- | --- | --- | --- |
| T8.1 | Command palette (⌘K/Ctrl+K) | done | `web/src/components/palette/CommandPalette.svelte`, mounted and key-bound in `App.svelte`. Three sections off one input: issue jump (reuses `filterIssues` + relevance sort, so key shortforms and 초성 queries work), views (built-in + personal + team), and actions (new issue, settings, reset filters, reopened/unassigned/stale toggles, locale switch, sync status toast). Empty query lists recently viewed issues. Every keystroke is a memory-pool computation — `e2e/palette.spec.ts` asserts zero `/api/` requests while typing, and that ⌘K opens even with a text input focused |

## Critical path to something usable

**Done.** The full loop works end to end against the public demo site: full sync
(519 issues in ~5 s), idempotent incremental sync, derived fields matching the
seeded ground truth (95 reopen transitions), FTS, bootstrap/delta/detail/search,
live write-through (comment + transition verified against real Jira), settings
round-trip without a restart, and the enrichments plugin boundary.

Since then the surfaces widened: a terminal UI (`scry tui`), agent-facing CLI
commands plus an MCP server, working plugin examples, single-binary packaging via
`go:embed`, a verified release pipeline (six archives, container image), and a
scripted demo-media pipeline.

**Still open**: T6.4 `scry snapshot`; T6.8 live-site display names (waiting on
invitation acceptance); the feed / push / presence surfaces, which are deliberate
404s until a local watch-based design lands in v0.2 — the one place a user
migrating from the internal predecessor loses a feature they had.
