# AGENTS.md

Two audiences. Pick your half:

- **[Using the mirror](#using-the-mirror)** — you want answers about issues, or
  you want to comment, transition, or assign one. Most agents stop here.
- **[Developing scry](#developing-scry)** — you are changing this repository.

## Using the mirror

scry keeps a local SQLite mirror of Jira at `~/.scry/scry.db` (`--profile x` puts
it under `~/.scry/profiles/x/`). Reads never touch the network. Writes go to Jira
and re-read the issue into the mirror afterwards.

Four layers. Use the lowest one that answers the question:

| Layer | Use it for | Needs |
| --- | --- | --- |
| **SQL** | anything relational, aggregated, or historical | the file, or `scry sql` |
| **CLI** | one issue, one search, one write | the `scry` binary |
| **REST** | the same data from something that is not a shell | `scry serve` running |
| **MCP** | shell-less clients only (Claude Desktop, etc.) | `scry mcp` — see [docs/MCP.md](docs/MCP.md) |

### Check freshness before you answer

Every CLI command prints one line to **stderr** when the mirror is behind or the
last sync failed; stdout stays clean and pipeable. To check explicitly:

```bash
scry status --json
# {"profile":"","issues":519,"comments":339,"watermark":"2026-08-04T09:15:00.000Z",
#  "version":41,"schema_version":5,"sync_count":12,"first_sync_at":"2026-07-01T…"}
```

A `last_error` field means the last sync failed. A quiet project's `watermark`
stalls on its own, so treat an old watermark as "possibly behind", not "broken".
`first_sync_at` / `sync_count` are retention instrumentation (successful syncs
only).

### The one mistake that silently returns nothing

Filter on ids and categories, never on display names. Jira translates
`status.name` and `issuetype.name` per account and ignores `Accept-Language`.

```sql
WHERE status = 'In Progress'          -- WRONG: empty on a Korean account
WHERE status_category = 'inprogress'  -- RIGHT: stable on every site
```

`status_category` is one of `new`, `inprogress`, `done`. `status_id` and
`issue_type_id` are stable too.

### SQL cookbook

The schema in one paragraph: `items` is the source-neutral spine (title,
`body_text`, timestamps); `issues` is the Jira projection, joined on
`issues.item_id = items.id`; **`issues_full` is the agent convenience view**
(`summary` + every `issues` column — prefer it when you need a title);
`comments`, `attachments`, `changelog`, and `links` hang off `items.id`;
`items_fts` is the FTS5 index over titles, bodies, and comment text;
`sync_state` holds freshness. `labels`, `components`, and `fix_versions` are
JSON arrays — reach them with `json_each`. Every column is listed in
`specs/000-product/data-model.md`.

```bash
scry sql "…"          # tab-separated, read-only
scry sql --json "…"   # one JSON object per row
scry sql --csv "…"    # header row plus CSV
```

```sql
-- 1. Someone's open work, most urgent first.
-- Prefer issues_full for titles (summary comes from items.title).
SELECT key, status, priority, summary
FROM issues_full
WHERE assignee_email = 'dana@example.com' AND status_category != 'done'
ORDER BY priority_rank, updated_at DESC;

-- 2. What regressed — reopens are the highest-signal quality metric available
SELECT key, summary, reopen_count, reopened_at
FROM issues_full
WHERE reopen_count > 0 ORDER BY reopen_count DESC, reopened_at DESC LIMIT 20;

-- 3. What is stuck, and for how long
SELECT key, status, ROUND(julianday('now') - julianday(status_changed_at), 1) AS days
FROM issues WHERE status_category = 'inprogress' ORDER BY days DESC LIMIT 20;

-- 4. Has anyone hit this before? (descriptions and comments, one index)
SELECT i.key, it.title FROM items_fts f
JOIN items it ON it.rowid = f.rowid
JOIN issues i ON i.item_id = it.id
WHERE items_fts MATCH 'webhook AND retry' LIMIT 20;

-- 5. Who is loaded, per project
SELECT project_key, COALESCE(assignee, '(unassigned)') AS who, COUNT(*) AS n
FROM issues WHERE status_category != 'done'
GROUP BY project_key, who ORDER BY project_key, n DESC;

-- 6. What is in a release (JSON array column)
SELECT key, status, summary
FROM issues_full, json_each(fix_versions) v
WHERE v.value = '2026.8.0' ORDER BY resolved_at;

-- 7. What moved this week, and who moved it
SELECT c.at, c.author, c.field, c.from_value, c.to_value, i.key
FROM changelog c JOIN issues i ON i.item_id = c.item_id
WHERE c.at > datetime('now', '-7 days') ORDER BY c.at DESC LIMIT 50;

-- 8. Untriaged: nobody on it, no priority set
SELECT key, created_at, summary
FROM issues_full
WHERE status_category = 'new' AND assignee_id IS NULL AND priority_rank = 0
ORDER BY created_at LIMIT 30;
```

Rules that come with the file:

- **Never write to the database.** Writes are Jira's job; a row written directly
  is destroyed by the next sync. There is no exception for "just a label".
- **Do not depend on `issues.raw`.** It is an escape hatch shaped by Jira's API,
  not by scry's contract.
- **Do not poll in a loop.** `sync_state.version` only moves when something
  changed; compare it instead.

### CLI reference

```bash
scry issue NMB-140                   # fields, description, comments, history, links
scry issue NMB-140 --json            # the `GET <key>/detail/` document plus the list row
# `scry issue` is the context pack: one call returns everything an LLM needs
# about an issue — no follow-up requests, no pagination.

scry open NMB-140                    # jump to the issue on the Jira site (boards, admin)

scry search "flaky upload" --limit 5
scry search "idempotency" --json     # matching IssueLite rows, best match first

scry comment NMB-140 -m "Reproduced on staging; trace attached."
scry comment NMB-140 -m -            # body from stdin, for anything multi-line
scry transition NMB-140 "In Review"  # transition name, target status name, or id
scry transition NMB-140 31
scry assign NMB-140 dana@example.com
scry assign NMB-140 -                # unassign

scry fields                          # custom-field usage on a sample (needs credential)
scry fields --sample 100 --project NMB --json

scry sync                            # incremental; --full re-fetches everything
scry status --json
scry sql --json "select count(*) from issues where reopen_count > 0"
scry --profile demo status           # separate credential and mirror per profile
```

Text output for a search result or a write is one tab-separated line —
`key`, `status`, `assignee`, `summary` — so `cut -f1` gives you keys. `--json` on
a write answers `{"issue": {…IssueLite}}`, plus `"comment"` for `comment`.

Writes need a credential (`scry init`) and fail before calling Jira without one.
A body written by `scry comment` is plain text: an `@Name` in it notifies nobody,
unlike the web UI's mention autocomplete.

`scry transition` reports what is available when the name does not match, so a
failed guess tells you what to guess next:

```
scry: no transition matching "Done" on NMB-140 — available: Start work (id 21, → 진행 중); Close (id 31, → 완료)
```

### REST

While `scry serve` is running (loopback only, no auth by design):

```bash
# Everything, once. Send the ETag back and an unchanged mirror answers 304.
curl -s localhost:7777/api/v1/issues/bootstrap/ | jq '.issues | length'

# Only what changed since a cursor — the timestamp the previous response carried.
curl -s 'localhost:7777/api/v1/issues/delta/?since=2026-08-04T09:15:00.000Z' | jq '.deleted_keys'

# One issue, fully hydrated.
curl -s localhost:7777/api/v1/issues/NMB-140/detail/ | jq '.comments[-1].body'

# Full-text search.
curl -s 'localhost:7777/api/v1/issues/search/?q=idempotency&limit=10' | jq '.keys'

# Writes — each answers with the refreshed IssueLite under "issue".
curl -s -X POST localhost:7777/api/v1/issues/NMB-140/comment/ \
  -H 'Content-Type: application/json' -d '{"text":"Reproduced on staging."}'

curl -s localhost:7777/api/v1/issues/NMB-140/transitions/ | jq '.transitions'
curl -s -X POST localhost:7777/api/v1/issues/NMB-140/transition/ \
  -H 'Content-Type: application/json' -d '{"transition_id":"31"}'

# account_id comes from `GET users/?q=<email>`; null unassigns.
curl -s -X PUT localhost:7777/api/v1/issues/NMB-140/assignee/ \
  -H 'Content-Type: application/json' -d '{"account_id":"5b10a2…"}'
```

A write with no stored credential answers `409 {"error":"credential_required"}`.
The full endpoint list, response shapes, and error bodies are in
`specs/000-product/contracts/api.md`.

### MCP (for clients without a shell)

If you can run shell commands, **stop here** — use SQL or the CLI above. MCP is
only for hosts that cannot spawn `scry sql` / `scry issue` as one-shot processes.

```bash
scry mcp                          # stdio JSON-RPC; logs go to stderr only
scry --profile demo mcp
```

Four tools, no more: `scry_query` (read-only SQL), `scry_search`, `scry_issue`,
`scry_status`. There are no write tools on MCP. Setup examples (Claude Desktop
config, profiles, troubleshooting) live in **[docs/MCP.md](docs/MCP.md)**.

## Developing scry

### Required reading order

0. **`docs/STATE_OF_PLAY.md`** — what actually exists right now, the next task,
   and the Jira behaviors that already cost debugging time. Start here.
1. `.specify/memory/constitution.md`
2. `specs/000-product/spec.md`
3. `specs/000-product/tasks.md` — the honest state of every piece
4. `specs/000-product/data-model.md` — the schema is a public contract
5. `specs/000-product/contracts/` — HTTP, sync, and agent contracts
6. `docs/ARCHITECTURE.md` and `docs/EXTRACTION.md`

### Development rules

- The mirror is disposable and Jira is the record. Never add state that only
  lives in scry, except local personal data, which must stay exportable.
- Nothing installation-specific goes in code or in a built artifact. No site URL,
  project key, custom field id, status name, team label, or person.
- Logic keys on ids and `statusCategory`, never on localized display names. Jira
  translates type and status names per account and ignores `Accept-Language`.
- `internal/store` must not import Jira-shaped code; `internal/connector/jira`
  must not write SQL.
- No network call on a keystroke path. Filtering stays in memory.
- Schema changes are contract changes: update `data-model.md`, add a migration,
  note it in `CHANGELOG.md`, and keep the documented example queries working.
- Credentials never reach SQLite, a log, or a snapshot.
- Derived fields are recomputed from the changelog, never carried forward.

### Before sending changes

```bash
go build ./... && go vet ./... && gofmt -l .
go test ./...
npm run typecheck
npm run build
```

Add a test with any non-trivial logic — parsing, derived fields, sync cursors,
or anything touching the schema.

### Handoff format

```
Summary
- What changed

Files changed
- path

Verification
- command or evidence

Open risks
- risk and next step
```
