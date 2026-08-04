# State of Play

**Read this first if you are picking this project up.** It is the bridge between
"what the docs describe" and "what actually exists right now", written so a fresh
session can start work without re-deriving anything.

Last updated: 2026-08-04, after the v0.1 implementation push.

## In one paragraph

The tool works end to end. `scry init && scry sync && scry serve` mirrors a real
Jira site into SQLite and serves the full UI from it; `scry demo` does the same
from a bundled snapshot with no account at all. Sync (full, incremental,
reconcile), the read API, write-through, the settings UI, the enrichments plugin
boundary, i18n (English default, Korean locale), and a Playwright E2E suite are
all implemented and tested. What remains before a public launch is release
polish, listed below.

## What is verified to work

Each claim here was executed against the real public demo site or the committed
snapshot, not assumed.

| Claim | Evidence |
| --- | --- |
| Full sync mirrors the demo site | 519 issues in ~5 s, `scry sync --full` |
| Incremental sync is idempotent | Immediate re-run: fetched 193 (overlap window), changed 0, `version` unchanged |
| Derived fields match seeded ground truth | 209/144/166 per category and 95 reopen transitions — the exact numbers the seeder wrote |
| Live write-through | Comment and transition executed against real Jira; response carried the refreshed IssueLite (`comment_count` 0→1) |
| Settings round-trip without restart | `PUT settings/` → `config.json` reflects immediately; `groupRules` classified 39 issues on the next read |
| Plugin boundary | Two SQL statements (insert into `enrichments`, bump `sync_state.version`) surfaced a deploy badge and PR list in the API |
| Browser E2E | `npx playwright test --config e2e/playwright.config.ts` → 8/8 against `examples/demo.db` |
| Whole test tree | `go test ./...` green across store/jira/sync/server/tools; `npm run typecheck` 0 errors |
| Gates | G1–G4 test evidence recorded per task in `../specs/000-product/tasks.md` |

## How the pieces fit

- `internal/store` — SQLite schema (public contract, `data-model.md`), migrations,
  FTS5, derived-field computation. Single writer, WAL.
- `internal/jira` — stdlib REST client: token-cursor search, changelog/comment
  paging, retry/backoff, id/category-only logic.
- `internal/sync` — full/incremental/reconcile passes, watermark with a 2-minute
  overlap, `SyncIssue` for post-write refresh, `Watch` loop.
- `internal/server` — the whole HTTP contract (`contracts/api.md`): bootstrap
  ETag, delta, detail (history carries `from_category`/`to_category`), search,
  attachment proxy, write-through, `settings/`, enrichments merge.
- `internal/config` — `~/.scry/config.json` (0600). **Profiles**: `--profile x` /
  `SCRY_PROFILE` keep separate credentials and mirrors under
  `~/.scry/profiles/x/` — this is how one machine points at a work site and the
  demo site at once.
- `web/` — the Svelte app. Feature flags (`presence/feed/push/deploy/qa/
  teamGroups`) actually gate their surfaces; staleness comes from
  `status_changed_at`; i18n catalogs live in `web/src/lib/i18n/`.
- `tools/seed-demo` — Go port of the demo-site seeder (the Python original is
  gone).
- `e2e/` — Playwright suite over `examples/demo.db`; runs in CI.

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
| `scry snapshot` (timestamp spreading, volume scaling) | T6.4 | `examples/demo.db` was scrubbed by hand this time; the command automates the next refresh |
| 10k-issue bench fixture + latency gate | T6.7 / G5 | The 50 ms / 10k target is still unmeasured |
| Secret & internal-string scan in CI | T7.4 | Manual scans passed; CI should pin them |
| Dockerfile, release process, signed binaries | T7.5 / T7.6 | |
| Live-site assignee display names | T6.8 | The committed snapshot is clean (fictional personas); the live site shows `midagedev+…` until each invitation is accepted. Blocks live-site screenshots only |
| Feed/push as a local watch-based design | v0.2 | Deferred endpoints return clean 404s today |
| MCP server | T5.4 | The SQLite file plus `scry sql` already cover shell-capable agents |

## Operational notes

- GitHub remote: `https://github.com/midagedev/scry` (private until launch).
  All commits are authored `midagedev <midagedev@users.noreply.github.com>`.
- The public demo site (a personal Atlassian site) is the live sync target;
  credentials are never in this repo. CI needs no secrets — everything runs
  against `examples/demo.db`.
- `scry --profile demo …` on the owner's machine holds the demo-site credential.

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
12. **JQL `updated` comparisons are minute-granular and read in account time.**
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
