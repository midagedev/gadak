# Agent Access

**The cookbook, the CLI reference, and the REST examples live in
[`MIRROR.md`](MIRROR.md).** This page is the map: which of the three access
layers to reach for, and what each one costs. The contract behind them is
`../specs/000-product/contracts/agent.md`.

## Three layers, lowest one wins

| | What it is | Reach for it when | Setup |
| --- | --- | --- | --- |
| **SQL** | the mirror file itself, `~/.gadak/gadak.db` | the question is relational, aggregated, or historical — "reopened twice and last touched by QA" is one query | none; `sqlite3` or `gadak sql` |
| **CLI** | `gadak issue`, `search`, `comment`, `transition`, `assign` | one issue read whole, or one write to Jira | the binary, and `gadak init` for writes |
| **REST** | the read and write API `gadak serve` exposes on loopback | the caller is not a shell — an editor extension, a script in another language, a browser | `gadak serve` running |

SQL is the widest and the cheapest: no tool schema to read, no pagination, no
rate limit, and only the columns asked for come back. The CLI exists because
"read this issue with its comments and history" and "leave a comment" are the two
things every agent needs and neither is a pleasant query. REST exists because not
every caller has a shell.

**SQL answers; `gadak views open` presents.** The three rows above are how an
agent *answers*. Presentation is a different axis:

| | What it is | Reach for it when | Setup |
| --- | --- | --- | --- |
| **Views** | `gadak views open` (`--jql`, `--keys`, `--keys -`, or a KEY) | the human should *see* the set in gadak, not in a pasted table | a running Gadak.app or `gadak serve` tab |

The database stays the answer interface. `gadak open` is the Jira escape hatch
(system browser to `/browse/KEY`); `gadak views open` is the "open in gadak"
verb. The names collide; the verbs do not.

There is no fourth layer for writes: the CLI and REST write paths both call Jira
and then re-read the issue into the mirror. Writing to SQLite directly is not a
shortcut, it is data loss on the next sync.

## Setup

None, for reading. If your agent can run a shell command, it can already query
the mirror:

```bash
sqlite3 ~/.gadak/gadak.db "select key, status, status_category from issues limit 5"
```

For an agent on a command allowlist, grant `gadak sql` instead of `sqlite3`: it
opens the database with SQLite's `mode=ro`, so a mistyped `UPDATE` fails on the
connection rather than corrupting the mirror. `gadak sql` ATTACHes `local.db`
as `local`, so `local.visits` (one row per view: `kind` `issue`|`page`, `key`,
`viewed_at`) and `local.searches` (`query`, `searched_at`, `result_count`,
optional `opened_kind`/`opened_key`) are ordinary SELECTs — do not ATTACH
yourself. Counts are `count(*)`; the file is next to the mirror and survives
deleting `gadak.db`. Search query text is personal and never leaves the machine.

## The one mistake to avoid

Filter on ids and categories, never on display names:

```sql
-- WRONG: returns nothing on a non-English Jira account
WHERE status = 'In Progress'

-- RIGHT: stable across every site and language
WHERE status_category = 'inprogress'
```

Jira translates `status.name` and `issuetype.name` into the account's display
language and ignores `Accept-Language`. `status_category`, `status_id`, and
`issue_type_id` do not move.

## Check staleness before acting

```bash
gadak status --json
# or
sqlite3 ~/.gadak/gadak.db "select source_id, watermark, last_error from sync_state"
```

Confirm `last_error IS NULL` and that the watermark is recent before acting on an
answer: a mirror that silently stopped syncing looks exactly like a quiet
backlog. `sql`, `issue`, `search`, `fields`, and the write verbs (`comment`,
`transition`, `assign`, …) also print a one-line warning to stderr when the
last sync failed or is over an hour old, so a stale answer says so without
being asked. (`status` reports instead of warning; use `gadak status --json`
when you need an explicit freshness check.)

## Rules

- **Never write to the database.** The next sync destroys direct writes. Changes
  go through Jira — the web UI, the write API, or the write commands.
- **Do not depend on `issues.raw`.** It follows Jira's payload shape, not gadak's
  contract.
- **Do not poll.** `sync_state.version` changes only when something changed.
- **Remember it is a mirror, not an archive.** Deleted issues disappear from it.

## Escape hatch: `gadak api`

When the mirror does not model what you need — watchers, worklogs, sprint
boards, user search, label bulk reads, Confluence REST that is not in the page
mirror — use **`gadak api`**. It sends the request with the workspace's stored
credential and prints the response body unchanged on stdout so an agent can
parse it. It is not a second product surface; it is a deliberate hole in the
fence for endpoints gadak has not chosen to model.

```bash
# Who am I (credential check / account id)
gadak api /rest/api/3/myself

# Watchers on one issue (not in the mirror)
gadak api GET /rest/api/3/issue/ABC-1/watchers

# Confluence spaces (path prefix /wiki/ → Confluence client; needs confluence enabled)
gadak api GET /wiki/api/v2/spaces --query limit=5

# Worklog write — requires --write; uses write retry policy (429/503 only)
gadak api POST /rest/api/3/issue/ABC-1/worklog --data @wl.json --write
```

**Read by default.** `GET` and `HEAD` run as-is. Any other method is refused
unless you pass **`--write`**, which also switches to the write retry policy
(no blind retry on 500 — a lost response may already have applied). Absolute
URLs (`https://…`, `//host/…`) are rejected so a prompt-injected path cannot
walk the token off your site.

**Not on MCP.** `gadak api` is CLI-only. MCP stays five tools
(`gadak_query`, `gadak_search`, `gadak_issue`, `gadak_status`, `gadak_show`)
with no writes to the mirror or to Jira and no raw proxy. `gadak_show` is
presentation (local ui-focus), ranked below SQL. A host without a shell is
not given a full-credential REST tunnel: that would expand the blast radius of
a compromised or confused agent beyond what the mirror contracts describe. If
you need the escape hatch, grant the agent a shell and `gadak api`.

## Why MCP is last, not first

MCP is shipped (`gadak mcp`) for hosts without a shell — Claude Desktop and the
like. Prefer the CLI or SQL when the agent can spawn a process: no tool schemas
in the context window, and the same five capabilities. The MCP surface stays a
thin wrapper: `gadak_query`, `gadak_search`, `gadak_issue`, `gadak_status`,
`gadak_show`. Deliberately not one tool per question — every extra tool is
context an agent must read before it can act, and `gadak_query` plus the
documented schema subsumes the reads. `gadak_show` is how a shell-less host
presents (SQL answers; show presents). Setup: [`MCP.md`](MCP.md).
