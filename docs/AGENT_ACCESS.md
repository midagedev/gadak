# Agent Access

**The cookbook, the CLI reference, and the REST examples live in
[`../AGENTS.md`](../AGENTS.md).** This page is the map: which of the three access
layers to reach for, and what each one costs. The contract behind them is
`../specs/000-product/contracts/agent.md`.

## Three layers, lowest one wins

| | What it is | Reach for it when | Setup |
| --- | --- | --- | --- |
| **SQL** | the mirror file itself, `~/.scry/scry.db` | the question is relational, aggregated, or historical — "reopened twice and last touched by QA" is one query | none; `sqlite3` or `scry sql` |
| **CLI** | `scry issue`, `search`, `comment`, `transition`, `assign` | one issue read whole, or one write to Jira | the binary, and `scry init` for writes |
| **REST** | the read and write API `scry serve` exposes on loopback | the caller is not a shell — an editor extension, a script in another language, a browser | `scry serve` running |

SQL is the widest and the cheapest: no tool schema to read, no pagination, no
rate limit, and only the columns asked for come back. The CLI exists because
"read this issue with its comments and history" and "leave a comment" are the two
things every agent needs and neither is a pleasant query. REST exists because not
every caller has a shell.

There is no fourth layer for writes: the CLI and REST write paths both call Jira
and then re-read the issue into the mirror. Writing to SQLite directly is not a
shortcut, it is data loss on the next sync.

## Setup

None, for reading. If your agent can run a shell command, it can already query
the mirror:

```bash
sqlite3 ~/.scry/scry.db "select key, status, status_category from issues limit 5"
```

For an agent on a command allowlist, grant `scry sql` instead of `sqlite3`: it
opens the database with SQLite's `mode=ro`, so a mistyped `UPDATE` fails on the
connection rather than corrupting the mirror.

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
scry status --json
# or
sqlite3 ~/.scry/scry.db "select watermark, last_error from sync_state"
```

Confirm `last_error IS NULL` and that the watermark is recent before acting on an
answer: a mirror that silently stopped syncing looks exactly like a quiet
backlog. `issue`, `search`, `comment`, `transition`, `assign`, and `fields` also
print a one-line warning to stderr when the last sync failed or is over an hour
old, so a stale answer says so without being asked. (`sql` and `status` do not;
use `scry status --json` when you need an explicit freshness check.)

## Rules

- **Never write to the database.** The next sync destroys direct writes. Changes
  go through Jira — the web UI, the write API, or the write commands.
- **Do not depend on `issues.raw`.** It follows Jira's payload shape, not scry's
  contract.
- **Do not poll.** `sync_state.version` changes only when something changed.
- **Remember it is a mirror, not an archive.** Deleted issues disappear from it.

## Why MCP is last, not first

MCP is shipped (`scry mcp`) for hosts without a shell — Claude Desktop and the
like. Prefer the CLI or SQL when the agent can spawn a process: no tool schemas
in the context window, and the same four capabilities. The MCP surface stays a
thin wrapper: `scry_query`, `scry_search`, `scry_issue`, `scry_status`.
Deliberately not one tool per question — every extra tool is context an agent
must read before it can act, and `scry_query` plus the documented schema subsumes
them all. Setup: [`MCP.md`](MCP.md).
