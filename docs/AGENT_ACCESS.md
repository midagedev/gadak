# Agent Access

The short version: point your agent at `~/.scry/scry.db` and let it write SQL.
The contract is `../specs/000-product/contracts/agent.md`; this page is the
practical guide.

## Setup

None. If your agent can run a shell command, it can already query the mirror.

```bash
sqlite3 ~/.scry/scry.db "select key, status, summary from issues limit 5"
```

For agents with a command allowlist, grant `scry sql` instead — it opens the
database read-only and rejects anything that is not a `SELECT` or `WITH`.

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

## Questions the mirror answers well

```sql
-- What regressed? Reopens are the highest-signal quality metric available.
SELECT key, summary, reopen_count, reopened_at FROM issues
WHERE reopen_count > 0 ORDER BY reopened_at DESC LIMIT 20;

-- What is stuck, and for how long?
SELECT key, status, ROUND(julianday('now') - julianday(status_changed_at), 1) AS days
FROM issues WHERE status_category = 'inprogress' ORDER BY days DESC LIMIT 20;

-- Has anyone hit this before? (bodies and comments, one match)
SELECT i.key, it.title FROM items_fts f
JOIN items it ON it.rowid = f.rowid
JOIN issues i ON i.item_id = it.id
WHERE items_fts MATCH 'webhook AND idempotency' LIMIT 20;

-- Who is loaded, per project?
SELECT project_key, COALESCE(assignee,'(unassigned)') AS who, COUNT(*) n
FROM issues WHERE status_category != 'done'
GROUP BY project_key, who ORDER BY project_key, n DESC;

-- What shipped in a release?
SELECT key, summary, resolved_at FROM issues
WHERE fix_versions LIKE '%2026.8.0%' AND status_category = 'done'
ORDER BY resolved_at;

-- What did this issue's status actually do over time?
SELECT at, author, from_value, to_value FROM changelog
WHERE item_id = 'jira:10432' AND field = 'status' ORDER BY at;
```

## Check staleness before acting

```bash
scry status --json
# or
sqlite3 ~/.scry/scry.db "select watermark, last_error from sync_state"
```

An agent about to act on the answer should confirm `last_error IS NULL` and that
the watermark is recent. A mirror that silently stopped syncing looks exactly like
a quiet backlog.

## Rules

- **Never write to the database.** The next sync destroys direct writes. Changes
  go through Jira, which means through the UI or the write API.
- **Do not depend on `issues.raw`.** It follows Jira's payload shape, not scry's
  contract.
- **Do not poll.** `sync_state.version` changes only when something changed.
- **Remember it is a mirror, not an archive.** Deleted issues disappear from it.

## Why not an MCP server first

One is planned for agents without shell access, and it will stay a thin wrapper:
`scry_query`, `scry_search`, `scry_issue`, `scry_status`. Deliberately not one
tool per question — every extra tool is context an agent must read before it can
act, and `scry_query` plus the documented schema subsumes them all.
