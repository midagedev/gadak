# State of Play

**Read this first if you are picking this project up.** It is the bridge between
"what the docs describe" and "what actually exists right now", written so a fresh
session can start work without re-deriving anything.

Last updated: 2026-08-04, after the initial extraction.

## In one paragraph

The web application is real, mature, and in daily use in its original internal
deployment. Everything server-side is a skeleton: `scry serve` hosts the built UI
and a config document, and nothing else is implemented. There is no database, no
sync, no API. The next work is to build them, in the order given below. A populated
public demo Jira site exists to develop and test against.

## What is verified to work

Each claim here was executed, not assumed. Re-run any of them to confirm.

| Claim | Command |
| --- | --- |
| The UI builds | `npm ci && npm run build` -> `dist/app` |
| The UI typechecks clean | `npm run typecheck` -> 0 errors, 0 warnings |
| The binary builds without CGO | `CGO_ENABLED=0 go build -o /tmp/scry ./cmd/scry` |
| `serve` answers health, config, and the SPA | `/tmp/scry serve --static dist/app` then curl `/healthz`, `/config.json`, `/` |
| A non-loopback bind is refused | `/tmp/scry serve --addr 0.0.0.0:7789` exits non-zero |
| No internal strings remain | `grep -rn "redacted-org\|redacted-product\|/redacted-tool\|redacted-service" web/ tools/ docs/ cmd/` -> nothing |

## What does not exist yet

- No SQLite schema, no migrations, no store package.
- No Jira connector. Nothing has ever synced.
- No read API: `bootstrap`, `delta`, `detail`, `search` all return `404`.
- No write-through. The UI's write actions cannot succeed.
- No `scry init`, `sync`, `demo`, `snapshot`, `sql`, or `mcp` — they exit with a
  pointer to `../specs/000-product/tasks.md`.

Consequence: **launching the UI today shows an empty shell.** That is expected,
not a bug to chase.

## Start here: the next task

`T1.1` in `../specs/000-product/tasks.md` — create the schema.

1. Read `../specs/000-product/data-model.md` in full. It is a contract, so the
   schema must match it exactly, including column names.
2. Create `internal/store` with the schema, a migration runner, and WAL pragmas.
3. Write the test that G1 in `../specs/000-product/gates.md` demands: compare
   `PRAGMA table_info` output against the documented columns, and execute every
   example query from the data model doc against a fixture.

Then follow the critical path: `T1.1 -> T1.2 -> T2.1 -> T2.2 -> T2.7 -> T3.3 -> T3.4`.
At the end of it, the UI displays real mirrored data, which is the first point at
which the tool is worth installing.

Do not start with writes or search. They are cheap once the mirror exists and
worthless before.

## The demo Jira site

A personal Atlassian site holds fictional data for development, screenshots, and
fixtures. Nothing in it derives from any real backlog.

| | |
| --- | --- |
| Projects | `NMB` Nimbus Web, `NMA` Nimbus API, `NMS` Nimbus Support (all **company-managed**) |
| Volume | 519 issues; 209 todo / 144 in progress / 166 done |
| History | Status-change depth 0–7 per issue, 95 reopen transitions |
| Content | 339 issues with comments, 61 link edges, releases and components per project |
| Authored subset | 210 issues have hand-written bodies (`examples/demo-seed.json`); the other 309 are procedurally generated and noticeably more repetitive |
| Assignees | Four accounts (264 assigned / 255 unassigned). **Three of them still display as an email local part** — see the debt table |
| Extra project | `KAN` exists on the same site and is unrelated; leave it alone |

Credentials are **not** in this repository. To work against the site you need the
site URL, an account email, and a user API token from
<https://id.atlassian.com/manage-profile/security/api-tokens>, exported as
`JIRA_SITE`, `JIRA_EMAIL`, `JIRA_TOKEN`. See `runbooks/local-dev.md`.

## Hard-won knowledge (do not rediscover these)

Every one of these cost real debugging time. They are also encoded in the code and
the contracts, but they are worth reading once up front.

1. **Organization API keys are not product API keys.** A key from
   `admin.atlassian.com` (prefix `ATCTT`) authenticates against
   `/admin/v1/orgs/...` and returns `401` on every Jira product endpoint. You need
   a user API token (prefix `ATATT`) with Basic auth.

2. **Team-managed projects are unusable for this tool's demo data.** They do not
   expose `priority`, `components`, or `fixVersions`, which are exactly the axes
   the UI filters on. Company-managed only.

3. **Jira localizes names and ignores `Accept-Language`.**
   `issue/createmeta` and search responses translate issue type and status names
   into the *account's* display language. `project/{key}/statuses` leaves
   workflow-local statuses untranslated but still translates Jira's global ones.
   Therefore: **all logic keys on ids or `statusCategory`, never on names.** This is
   the single most common way to write code that works for you and silently
   returns nothing for someone else.

4. **Reopen and resolution detection must use `statusCategory`.** The internal
   original matched status names and broke everywhere else. A reopen is a
   transition from a `done`-category status to a non-`done` one.

5. **Default workflows offer a direct `Backlog -> Done` edge.** Taking it leaves a
   single changelog entry, so the history timeline has nothing to show. Walk the
   category ladder one rung at a time.

6. **Changelog history cannot be backfilled.** Jira assigns `created` at insert
   time with no backdating, and pushing an already-done issue backwards later
   registers as a reopen it should not have. Realistic time spread is therefore a
   `scry snapshot` concern, not a Jira one.

7. **`search/jql` does support `expand=changelog`** — verified. Use token
   pagination (`nextPageToken`), not the deprecated `startAt` search.

8. **Deleting issues needs a permission the default scheme lacks.** Plan seeding
   runs assuming you cannot undo them.

9. **The frontend build target rejects top-level `await`.** `web/src/main.ts`
   wraps its config load in an async IIFE for this reason. Do not "simplify" it.

10. **An admin cannot set a display name on Jira Cloud.**
    `POST /rest/api/3/user` accepts `displayName` and silently ignores it for
    accounts outside a verified domain; Jira uses the email local part instead.
    Only the account holder can set the real name, after accepting the invitation
    email. Plan demo personas accordingly — or use a domain you control.

## Deliberate debts

These are choices, not oversights. Each has a reason recorded.

| Debt | Why it was left | Where |
| --- | --- | --- |
| `PrList`, `DeployTimeline`, `QaImpact` still in the tree | Deleting them touches the detail panel and type surface; not worth bundling into the extraction | `EXTRACTION.md` |
| UI is Korean-only, including source comments | Translation is a separate pass, and it is a **release blocker** for a public launch | `tasks.md` T0.10 |
| `d1_group` field name survives in client types | Rename belongs with the first stable API release | `EXTRACTION.md` |
| 309 of 519 demo issues have templated bodies | Rewriting them is cheap later; the authored 210 cover screenshots | `tasks.md` T6.3 |
| Some demo issues have thin changelog history | Cannot be fixed retroactively without faking reopens | point 6 above |
| Three demo assignees display as an email local part, not a person's name | Jira Cloud will not let an admin set `displayName` for a non-managed account. `POST /rest/api/3/user` accepts the field and ignores it; the name is derived from the email instead, and only the account holder can change it after accepting the invitation. **Blocks public screenshots**, because the owner's alias pattern would be visible | point 10 below |

## Where to look for what

| Question | File |
| --- | --- |
| What is the product and why | `CONCEPT.md`, `../README.md` |
| What am I allowed to do | `../.specify/memory/constitution.md` |
| What is the state of every piece | `../specs/000-product/tasks.md` |
| What is the database contract | `../specs/000-product/data-model.md` |
| What does the UI expect from the server | `../specs/000-product/contracts/api.md` |
| How must sync behave | `../specs/000-product/contracts/sync.md` |
| What do agents get | `../specs/000-product/contracts/agent.md`, `AGENT_ACCESS.md` |
| When is a phase done | `../specs/000-product/gates.md` |
| Why is it shaped this way | `decisions/` |
| How do I run things | `runbooks/local-dev.md` |
| How do I refill demo data | `../tools/README.md` |
