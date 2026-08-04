# Agent Contract

How a coding agent is meant to read the mirror. This is half the reason scry
exists, so it gets a contract rather than a paragraph in the README.

## The interface is the database

```bash
sqlite3 ~/.scry/scry.db "select key, summary from issues where status_category != 'done'"
```

No tool schema, no endpoint list, no rate limit, no pagination. An agent that can
run a shell command can already use scry at full power, and the schema in
`../data-model.md` is the only documentation it needs.

This is a deliberate rejection of the alternative: designing a REST or MCP
surface that exposes N pre-baked questions. Every such surface is a guess about
what will be asked. SQL is not a guess.

## Why this beats calling Jira directly

| | Jira REST | scry mirror |
| --- | --- | --- |
| Relational questions ("issues reopened twice with a comment from QA") | Multiple calls plus client-side joins | One query |
| Aggregation | Not supported; fetch and count | `GROUP BY` |
| Full-text over comments | Weak, and separate from issue search | One FTS match |
| Rate limits | Yes, and shared with the human's session | None |
| Context cost | Large JSON payloads | Exactly the columns asked for |
| Works offline | No | Yes |
| Determinism for tests | No | Yes, with a snapshot |

## Guarantees

1. **The schema is versioned.** Released columns are never repurposed. A rename
   keeps the old name readable for one minor version.
2. **The example queries in `../data-model.md` keep working** across minor
   versions. They are the executable part of this contract.
3. **`status_category`, `issue_type_id`, and `status_id` are stable across sites
   and account languages.** Display names are not — an agent that filters on
   `status = 'In Progress'` will silently return nothing on a Korean account.
   This is the single most likely mistake and the schema is shaped to make the
   right choice available.
4. **Reads never block writes.** The database runs in WAL mode, so an agent
   querying during a sync sees a consistent earlier snapshot rather than a lock
   error.

## Staleness

An agent must be able to tell how old its answer is:

```sql
SELECT watermark, version, last_full_sync_at, last_error FROM sync_state;
```

Agents doing anything consequential should check `last_error IS NULL` and that
`watermark` is recent. `scry status --json` prints the same information for
agents that prefer not to write SQL for it.

## Read-only convenience

```bash
scry sql "select count(*) from issues where reopen_count > 0"
scry sql --json "select key, summary from issues limit 5"
```

Opens the database read-only and rejects anything that is not a `SELECT` or
`WITH`. It exists so an agent that has been given a narrow command allowlist can
still be granted mirror access without granting arbitrary `sqlite3`.

## MCP server (planned, not in v0.1)

For agents without shell access. It stays a thin wrapper over the same schema:

| Tool | Shape |
| --- | --- |
| `scry_query` | `{sql}` -> rows. Read-only, statement-type checked, row-capped |
| `scry_search` | `{text, limit}` -> keys and titles, via FTS |
| `scry_issue` | `{key}` -> full detail including comments and history |
| `scry_status` | `{}` -> sync state |

Deliberately not planned: one tool per question. `scry_query` plus the documented
schema subsumes them, and every extra tool is context an agent has to read before
it can act.

## Anti-patterns

- **Do not write to the database.** Writes are Jira's job; a row written directly
  is destroyed by the next sync. There is no exception for "just a label".
- **Do not treat the mirror as an archive.** It reflects current Jira state.
  Deleted issues disappear. If history matters, snapshot it.
- **Do not depend on `issues.raw`.** It is an escape hatch for fields not yet
  mapped, and its shape follows Jira's API, not scry's contract.
- **Do not poll in a loop.** Check `sync_state.version`; it only changes when
  something changed.
