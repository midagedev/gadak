# State of Play

**Read this first if you are picking this project up.** It is the bridge between
"what the docs describe" and "what actually exists right now", written so a fresh
session can start work without re-deriving anything.

Last updated: 2026-08-15 — the v0.13 wave: unified ⌘K search over issues *and*
documents, a `keys` axis so an agent can put an exact set on the window
(`views open --keys -`, MCP `gadak_show`), Confluence space scope that prunes
and backfills itself (schema v19), `author_id` on changelog and attachments
(v20), the account-id identity class closed across every surface, two security
fixes (profile path escape, unguarded top-level routes), and the macOS window
drag repair from issue #2. Last tagged: v0.13.0.

## In one paragraph

The tool works end to end on two surfaces over one SQLite mirror: the web UI
(`gadak serve`, or the desktop window around it) and the CLI that doubles as the
agent interface (`gadak issue/search/comment/transition/assign/sql`, plus an MCP
server for clients without a shell). `gadak demo` runs the whole thing against a
bundled snapshot with no Jira account. Sync (full, incremental, reconcile), the
read API, write-through, the settings UI, the enrichments plugin boundary with
working examples, i18n, single-binary packaging, and a Playwright suite are
implemented and tested. Identity is the stored Jira credential — there is no
account and no login.

## What is verified to work

Each claim here was executed against the real public demo site or the committed
snapshot, not assumed.

| Claim | Evidence |
| --- | --- |
| Full sync mirrors the demo site | 534 issues in ~5 s, `gadak sync --full` |
| Incremental sync is idempotent | Immediate re-run: fetched 193 (overlap window), changed 0, `version` unchanged |
| Derived fields match seeded ground truth | 224/144/166 per category and 95 reopen transitions — `examples/demo.db` counts |
| Live write-through | Comment and transition executed against real Jira; response carried the refreshed IssueLite (`comment_count` 0→1) |
| Settings round-trip without restart | `PUT settings/` → `config.json` reflects immediately; `groupRules` classified 39 issues on the next read |
| Plugin boundary | Two SQL statements (insert into `enrichments`, bump `sync_state.version`) surfaced a deploy badge and PR list in the API |
| Browser E2E | `playwright test --config e2e/playwright.config.ts` against `examples/demo.db` (`e2e/*.spec.ts` only; `demo/` and `hosted/` ignored) |
| Derived reopen_reason / cloned_from | Live demo-site sync: 87 reopened issues, 42 carrying a derived reason |
| Plugin examples end to end | `examples/plugins/github-prs` and `csv-import` run against a copy of the snapshot; the API then returns `linked_prs` and both the list badge (`deploy_status`) and detail `deploy` |
| Single binary | `go build` embeds `dist/app`; a fresh binary with no `--static` serves the UI and `/api/v1/issues/bootstrap/` returns 200 |
| Agent CLI | `gadak issue NMB-20`, `gadak search pagination`, `--json` shapes verified against the demo profile |
| JQL subset | `internal/jql` parse/emit/match tests; `gadak search --jql` and `POST /api/v1/issues/jql/` round-trip `project` + `statusCategory` against the fixture |
| Jira filter import | Sync `GET /filter/my?includeFavourites=true` → `source_queries`; sidebar **Jira filters**; partial JQL listed |
| Agent view focus | `gadak views open --jql '…'` writes `ui-focus.json`; the app / serve tab applies the hash; `--no-open` for tests |
| Secret scan | `scripts/scan-internal.sh` clean across the tracked tree and the demo snapshot |
| Release artifacts | `goreleaser release --snapshot` → six archives; the extracted darwin/arm64 binary serves the embedded UI and a 200 bootstrap with no `--static` |
| MCP server | stdio JSON-RPC round trip: initialize / tools/list / all five tools; write SQL rejected as a tool error; stdout carries frames only |
| Demo media | `make media` regenerates the hero GIF/MP4 and the agent-focus split (terminal + paper list) from the snapshot |
| Attachment cache | Fake-Jira test: one upstream fetch for two views, `immutable` validator on the second, and a cached image still served with the credential removed. Live: 0.6 ms from disk |
| Inline comment images | Live demo site: three uploads, a comment carrying two media nodes with real UUIDs and `alt` filenames, both rendering in a browser at full resolution |
| Offline attachments | `gadak demo` imports `examples/attachments/`; both inline images render with no Jira account |
| Onboarding | Live site: credential rejected vs verified, four projects listed, 409 on a concurrent sync start, progress 100 → 193 → done |
| Settings runtime panel | `GET settings/` carries profile, absolute DB and config paths, sizes, counts, watermark; a 5-second sync interval is refused with 400 and the credential survives a PUT |
| Whole test tree | `go test ./...` green across store/jira/sync/server/tools; `npm run typecheck` 0 errors |
| Gates | G1–G4 test evidence recorded per task in `../specs/000-product/tasks.md` |

## How the pieces fit

- `internal/store` — SQLite schema (documented in `data-model.md`, promised in
  part), migrations,
  FTS5, derived-field computation. Single writer, WAL.
- `internal/jira` — stdlib REST client: token-cursor search, changelog/comment
  paging, retry/backoff, id/category-only logic.
- `internal/sync` — full/incremental/reconcile passes, watermark with a 2-minute
  overlap, `SyncIssue` for post-write refresh, `Watch` loop.
- `internal/server` — the whole HTTP contract (`contracts/api.md`): bootstrap
  ETag, delta, detail (history carries `from_category`/`to_category`), search,
  attachment proxy, write-through, `settings/`, enrichments merge.
- `internal/config` — `~/.gadak/config.json` (0600). **Profiles**: `--profile x` /
  `GADAK_PROFILE` keep separate credentials and mirrors under
  `~/.gadak/profiles/x/` — this is how one machine points at a work site and the
  demo site at once.
- `web/` — the Svelte app. Feature flags (`feed/push/deploy/qa/teamGroups`)
  actually gate their surfaces; staleness comes from `status_changed_at`; i18n
  catalogs live in `web/src/lib/i18n/`.
- `tools/seed-demo` — Go port of the demo-site seeder (the Python original is
  gone).
- `internal/mcp` — stdio JSON-RPC server, five tools (four reads plus `gadak_show` for presentation), no writes to the mirror or to Jira, no SDK.
- `internal/attachcache` — attachment bytes on disk, content-addressed, single
  flight, LRU budget. Why it exists: proxying every image view contradicted the
  premise, and a cached image renders with no credential, which is what lets the
  offline snapshot show real screenshots.
- `internal/snapshot` — `gadak snapshot`. Builds a fresh schema and copies rows
  into it rather than duplicating the file, so personal tables leave no residue;
  optional timestamp spreading and issue cloning for fixtures.
- `internal/teamconfig` — `gadak team export/import`. Whitelist-only settings
  sharing; a reflection test refuses to compile past an unclassified `Config`
  field.
- `internal/secretscan` — the credential-shaped patterns every outbound
  artifact is checked against. Separate from both producers on purpose: a
  snapshot and a team file have nothing else in common, and whichever owned the
  patterns would have become the other's dependency.
- `examples/plugins/` — working enrichment plugins (GitHub PRs, deploy status
  from git tags, CSV import) with self-tests behind `make plugins-test`.
- `e2e/` — Playwright suite over `examples/demo.db`; runs in CI. `e2e/demo/` and
  `tools/tapes/` are the recording pipeline (`make media`), excluded from it.

## The plugin boundary (how company-specific surfaces come back)

The open-source core contains zero GitHub/CD/test-management code. An external
process — any language, any schedule — upserts rows into the `enrichments`
table and bumps `sync_state.version`; the server merges them into list rows
(`deploy_status`, `qa_impact_*`) and detail responses (`deploy`, `qa_context`,
`linked_prs`, `development_opinion`), and the UI surfaces appear once the
matching feature flag is on. Payload shapes: `docs/PLUGINS.md`. Enrichments can
never shadow mirrored fields.

## What remains before a public launch

| Item | Task | Note |
| --- | --- | --- |
| Live-site assignee display names | T6.8 | The committed snapshot is clean (fictional personas); the live site shows placeholder handles until each invitation is accepted. Affects live-site screenshots only |
| Zero-install hosted demo | v0.3 | Live at https://midagedev.github.io/gadak/ — static JSON + an in-page fetch adapter (not sqlite-wasm; the former demo-sw.js was retired for in-app browsers, decision 0004 addendum). `make hosted-demo` → `dist/hosted/`; Pages workflow deploys on `main` |
| Web push (VAPID) | v0.2 | Still deferred; `features.push` stays false; in-tab Notification only |
| Bootstrap payload cost at 10k | G5 | ≈61 ms/op on an M4 Pro — over the 50 ms product target, but it is a once-per-boot cost and the client caches it in IndexedDB. Streaming or a columnar payload is the lever if it matters |

Everything else on the original launch list is done: benchmarks (T6.7), the CI
secret scan (T7.4), Docker and the release pipeline (T7.5/T7.6), the MCP server
(T5.4), and the demo media pipeline.

## Operational notes

- All commits are authored `midagedev <midagedev@users.noreply.github.com>`.
- Everything in CI runs against `examples/demo.db`; no Jira credential is
  needed to build, test, or review a change. The snapshot's personas are
  fictional.
- Live-site verification uses a throwaway Atlassian site through a named profile
  (`gadak --profile demo …`). Credentials live outside the repo by construction —
  see [SECURITY.md](../SECURITY.md).

## Hard-won knowledge (do not rediscover these)

1. **Organization API keys are not product API keys.** `ATCTT` keys from
   admin.atlassian.com 401 on every product endpoint; you need a user token
   (`ATATT`) with Basic auth.
2. **Team-managed projects lack `priority`/`components`/`fixVersions`.**
   Company-managed only for demo data.
3. **Jira localizes type/status names per account language and ignores
   `Accept-Language`.** All logic keys on ids or `statusCategory`. The sync test
   suite runs the same fixture in English and Korean and asserts identical
   derived output.
4. **A reopen is a `done`-category → non-`done` transition.** Never a name match.
5. **Default workflows have a direct `Backlog -> Done` edge** that leaves a
   one-entry changelog; the seeder walks the category ladder instead.
6. **Changelog history cannot be backfilled.** Time spread is a snapshot-tool
   concern (T6.4).
7. **`search/jql` supports `expand=changelog`** with `nextPageToken` paging; when
   an issue reports a truncated changelog, page `GET /issue/{key}/changelog`.
8. **Issue deletion needs a permission the default scheme lacks.** Plan seeding
   runs assuming you cannot undo.
9. **The frontend build target rejects top-level `await`** — `web/src/main.ts`
   wraps its config load in an async IIFE.
10. **Admins cannot set display names on Jira Cloud** for accounts outside a
    verified domain; only the account holder can, after accepting the invite.
11. **Go's ServeMux panics at registration on intersecting patterns**
    (`watches/{key}/` vs `{key}/assignee/` under the same method). The server
    registers one `{key}/{action}` pattern and branches inside;
    `TestRoutesRegister` keeps it honest.
12. **goreleaser `--clean` empties its own output directory.** Pointing it at
    `dist/` deleted `dist/app`, the web build `go:embed` needs, so every tagged
    release would have failed its own before-hook. Output goes to `.release/`.
13. **The sync-health badge reads `sources.synced_at`, not the watermark.** A
    quiet project's watermark stays in the past forever and would read as
    permanently delayed; recordings freshen the former (`GADAK_FRESHEN=1`).
14. **`waitForLoadState('networkidle')` is a trap here.** The client polls for a
    delta every 15 s, so "no network for 500 ms" only becomes true after that
    poll fires. Wait on the boot payload instead.
15. **JQL `updated` comparisons are minute-granular and read in account time.**
    The watermark is stored verbatim and rendered into JQL with the offset Jira
    supplied; the 2-minute overlap makes the boundary loss-proof.

## Where to look for what

| Question | File |
| --- | --- |
| What is the product and why | `CONCEPT.md`, `../README.md` |
| What am I allowed to do | `../.specify/memory/constitution.md` |
| State of every task, with test evidence | `../specs/000-product/tasks.md` |
| Database contract | `../specs/000-product/data-model.md` |
| HTTP contract | `../specs/000-product/contracts/api.md` |
| Sync behavior | `../specs/000-product/contracts/sync.md` |
| Plugin payloads | `PLUGINS.md` |
| Agent access | `../specs/000-product/contracts/agent.md`, `AGENT_ACCESS.md` |
| Gate definitions | `../specs/000-product/gates.md` |
| Why it is shaped this way | `decisions/` |
| Running locally | `runbooks/local-dev.md` |
| Refilling demo data | `../tools/README.md` |
| Browser E2E | `../e2e/README.md` |
