# AGENTS.md

Two audiences. Pick your half:

- **[Using the mirror](#using-the-mirror)** — you want answers about issues, or
  you want to comment, transition, or assign one. Most agents stop here.
- **[Developing gadak](#developing-gadak)** — you are changing this repository.

## Using the mirror

gadak keeps a local SQLite mirror of Jira at `~/.gadak/gadak.db` (`--profile x` puts
it under `~/.gadak/profiles/x/`). Reads never touch the network. Writes go to Jira
and re-read the issue into the mirror afterwards.

Four layers. Use the lowest one that answers the question:

| Layer | Use it for | Needs |
| --- | --- | --- |
| **SQL** | anything relational, aggregated, or historical | the file, or `gadak sql` |
| **CLI** | one issue, one search, one write | the `gadak` binary |
| **REST** | the same data from something that is not a shell | `gadak serve` running |
| **MCP** | shell-less clients only (Claude Desktop, etc.) | `gadak mcp` — see [docs/MCP.md](docs/MCP.md) |

### Check freshness before you answer

`issue`, `search`, `comment`, `transition`, `assign`, and `fields` print one line
to **stderr** when the last sync failed or is over an hour old; stdout stays
clean and pipeable. (`sql`, `status`, `open`, `sync`, and others do not.) To
check explicitly:

```bash
gadak status --json
# {"profile":"…","issues":534,"comments":634,"watermark":"…",
#  "version":6,"schema_version":20,"sync_count":1,"first_sync_at":"…"}
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
(`summary` + every `issues` column + `description_text` from `items.body_text` — prefer it when you need a title or the description as plain text);
`comments`, `attachments`, `changelog`, and `links` hang off `items.id`;
`items_fts` is the FTS5 index over titles, bodies, and comment text;
`sync_state` holds freshness. `labels`, `components`, and `fix_versions` are
JSON arrays — reach them with `json_each`. Every column is listed in
`specs/000-product/data-model.md`.

Personal history lives in a second file next to the mirror (`local.db`),
ATTACHed as `local` when gadak opens the mirror — you do not type ATTACH.
`local.visits` is one row per view (`kind` is `issue` or `page`; `key` is the
issue key or page id; `viewed_at`). `local.searches` is one row per executed
search (`query`, `searched_at`, `result_count`, and `opened_kind`/`opened_key`
when something was opened from it). There is no stored counter: `count(*)`
per key is the visit count. Rows older than 180 days are pruned. This file
survives `rm gadak.db` and is never sent to Jira. Do not write search-query
text into logs or tickets.

```bash
gadak sql "…"              # header row plus TSV, read-only
gadak sql --no-header "…"  # TSV/CSV without the header row
gadak sql --json "…"       # one JSON object per row
gadak sql --csv "…"        # header row plus CSV
```

```sql
-- Find a person's ids first; email may be empty if the site hides it.
SELECT DISTINCT assignee, assignee_id, assignee_email FROM issues
WHERE assignee LIKE '%dana%' OR assignee_email LIKE '%dana%';

-- 1. Someone's open work, most urgent first.
-- Prefer issues_full for titles (summary comes from items.title).
-- assignee_id is the value from the lookup above.
SELECT key, status, priority, summary
FROM issues_full
WHERE assignee_id = '<id from the lookup above>' AND status_category != 'done'
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
SELECT i.key, i.status, i.summary
FROM issues_full i, json_each(i.fix_versions) v
WHERE v.value = '2026.8.0' ORDER BY i.resolved_at;

-- 7. What moved this week, and who moved it
SELECT c.at, c.author, c.field, c.from_value, c.to_value, i.key
FROM changelog c JOIN issues i ON i.item_id = c.item_id
WHERE c.at > datetime('now', '-7 days') ORDER BY c.at DESC LIMIT 50;

-- 8. Untriaged: nobody on it, no priority set
SELECT key, created_at, summary
FROM issues_full
WHERE status_category = 'new' AND assignee_id IS NULL AND priority_rank = 0
ORDER BY created_at LIMIT 30;

-- 9. Recently viewed issues (then present: gadak views open --keys -)
SELECT key, MAX(viewed_at) AS last_seen, COUNT(*) AS n
FROM local.visits
WHERE kind = 'issue'
GROUP BY key
ORDER BY last_seen DESC
LIMIT 20;

-- 10. A search you ran, and what you opened from it
SELECT query, opened_key, searched_at, result_count
FROM local.searches
WHERE query LIKE '%upload%'
ORDER BY searched_at DESC
LIMIT 20;

-- 11. Read an issue description as plain text (no ADF parser)
SELECT key, substr(description_text,1,200) FROM issues_full WHERE key='...';
```

Pipe keys from (9) into the running UI: `gadak sql --no-header "select key from local.visits where kind='issue' group by key order by max(viewed_at) desc" | gadak views open --keys -` (or keep the header and `tail -n +2`).

Rules that come with the file:

- **Never write to the database.** Writes are Jira's job; a row written directly
  is destroyed by the next sync. There is no exception for "just a label".
- **Do not depend on `issues.raw`.** It is an escape hatch shaped by Jira's API,
  not by gadak's contract.
- **Do not poll in a loop.** `sync_state.version` only moves when something
  changed; compare it instead.

### CLI reference

`gadak open` is the Jira escape hatch (system browser to `/browse/KEY`);
`gadak views open` is the "open in gadak" verb (focus the running app or
serve tab). The names collide; the verbs do not.

**Handing a view to a human.** `gadak views open` acts — it pulls the window
forward now. When you would rather offer than act, or you are on a host with
no shell, use the link instead: every `views open` prints a `deeplink` line
(and a `"deeplink"` field under `--json`) of the form
`gadak://view/w/<profile>?<hash>`. Put that in your message and the person
decides when to click. Unlike the `web` field beside it, it is always there —
`web` needs a `serve` already listening to know which port to name, and is
empty otherwise. Opening one needs the macOS desktop app installed.

```bash
gadak issue NMB-140                   # fields, description, comments, history, links
gadak issue NMB-140 --json            # the `GET <key>/detail/` document plus the list row
# `gadak issue` is the context pack: one call returns everything an LLM needs
# about an issue — no follow-up requests, no pagination.

gadak open NMB-140                    # Jira escape hatch: system browser to /browse/KEY

gadak search "flaky upload" --limit 5
gadak search "idempotency" --json     # matching IssueLite rows, best match first
gadak search --jql 'project = NMA AND statusCategory = "In Progress"'
gadak search 'https://your-site.atlassian.net/issues/?jql=project%20%3D%20NMA'

gadak views                              # Jira filters (after sync) + saved views
gadak views open "NMA in progress"       # open in gadak: focus the running app or serve tab
gadak views open --jql 'project = NMA AND statusCategory = "In Progress"'
gadak views save "Night triage" --jql 'assignee = currentUser() AND resolution is EMPTY'

gadak create Batch worker drops the last page --project NMB --type Bug -m "repro on staging" --parent NMB-1
gadak create --batch -                # one JSON object per line on stdin
gadak attach NMB-140 screenshot.png trace.log
gadak edit NMB-140 --summary "…" --label +regression --label -needs-triage --priority High --parent none

gadak comment NMB-140 -m "Reproduced on staging; trace attached."
gadak comment NMB-140 -m -            # body from stdin, for anything multi-line
gadak transition NMB-140 "In Review"  # transition name, target status name, or id
gadak transition NMB-140 31
gadak assign NMB-140 dana@example.com
gadak assign NMB-140 -                # unassign

gadak fields                          # custom-field usage on a sample (needs credential)
gadak fields --sample 100 --project NMB --json

gadak team export --out gadak-team.json   # share views, fieldMap, group rules (no credentials)
gadak team import gadak-team.json         # merge into this profile; try --dry-run first

gadak sync                            # incremental; --full re-fetches everything
gadak status --json
gadak sql --json "select count(*) from issues where reopen_count > 0"
gadak --profile demo status           # separate credential and mirror per profile
```

Keystroke-driven clients (a typeahead UI, Raycast) should not send every IME
intermediate as a query: Korean composition produces jamo states that match
nothing and flash the result list empty. If the host exposes composition
events, commit the query only when composition ends. If it does not (Raycast's
input has no composition visibility), keep the last non-empty result set
visible when a query returns 0 rows.

Text output for a search result or a write is one tab-separated line —
`key`, `status`, `assignee`, `summary` — so `cut -f1` gives you keys. `--json` on
a write answers `{"issue": {…IssueLite}}`, plus `"comment"` for `comment`.

Writes need a credential and fail before calling Jira without one. `gadak init`
takes the whole setup non-interactively, so an agent never has to drive a
prompt — it only falls back to asking when stdin is a terminal *and* nothing was
supplied:

```
GADAK_TOKEN=$(cat token) gadak init \
  --site https://your-site.atlassian.net --email you@example.com --projects ABC --json
```

There is deliberately no `--token` flag: argv is visible in `ps` and lands in
shell history. Pass the token as `GADAK_TOKEN`, `--token-file <path>`, or
`--token-stdin`. Any flag or `GADAK_*` value switches init fully non-interactive,
so a missing value fails immediately — listing what is missing — instead of
blocking on a prompt no one is there to answer.
A body written by `gadak comment` is plain text: an `@Name` in it notifies nobody,
unlike the web UI's mention autocomplete.

`gadak transition` reports what is available when the name does not match, so a
failed guess tells you what to guess next:

```
gadak: no transition matching "Done" on NMB-140 — available: Start work (id 21, → 진행 중); Close (id 31, → 완료)
```

### REST

While `gadak serve` is running (loopback only, no auth by design):

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

# Full replace. Empty array clears. Trim and de-dupe happen server-side.
curl -s -X PUT localhost:7777/api/v1/issues/NMB-140/labels/ \
  -H 'Content-Type: application/json' -d '{"labels":["batch","tech-debt"]}'

# priority_id comes from `GET priorities/`; null clears. Do not send the name.
curl -s -X PUT localhost:7777/api/v1/issues/NMB-140/priority/ \
  -H 'Content-Type: application/json' -d '{"priority_id":"2"}'

curl -s -X PUT localhost:7777/api/v1/issues/NMB-140/summary/ \
  -H 'Content-Type: application/json' -d '{"summary":"Rename without opening Jira"}'
```

A write with no stored credential answers `409 {"error":"credential_required"}`.
The full endpoint list, response shapes, and error bodies are in
`specs/000-product/contracts/api.md`.

### MCP (for clients without a shell)

If you can run shell commands, **stop here** — use SQL or the CLI above. MCP is
only for hosts that cannot spawn `gadak sql` / `gadak issue` as one-shot processes.

```bash
gadak mcp                          # stdio JSON-RPC; logs go to stderr only
gadak --profile demo mcp
```

Five tools: `gadak_query` (read-only SQL), `gadak_search`, `gadak_issue`,
`gadak_status`, `gadak_show`. MCP does not write to the mirror or to Jira;
`gadak_show` writes a local ui-focus file so the running app presents the set
(SQL answers; show presents). Setup examples (Claude Desktop config, profiles,
troubleshooting) live in **[docs/MCP.md](docs/MCP.md)**.

## Developing gadak

### Required reading order

0. **`docs/STATE_OF_PLAY.md`** — what actually exists right now, the next task,
   and the Jira behaviors that already cost debugging time. Start here.
1. `.specify/memory/constitution.md`
2. `specs/000-product/spec.md`
3. `specs/000-product/tasks.md` — the honest state of every piece
4. `specs/000-product/data-model.md` — the schema, and how much of it is promised
5. `specs/000-product/contracts/` — HTTP, sync, and agent contracts
6. `docs/ARCHITECTURE.md` and `docs/EXTRACTION.md`

### Development rules

- The mirror is disposable and Jira is the record. Never add state that only
  lives in gadak, except local personal data, which must stay exportable.
- Nothing installation-specific goes in code or in a built artifact. No site URL,
  project key, custom field id, status name, team label, or person.
- Logic keys on ids and `statusCategory`, never on localized display names. Jira
  translates type and status names per account and ignores `Accept-Language`.
- `internal/store` must not import Jira-shaped code; `internal/jira`
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

The E2E fixture server (`e2e/serve.sh`) serves the **built** UI from `dist/app`,
not your source tree. After editing anything under `web/`, rebuild before you
run the suite or screenshot it — otherwise you are testing the last build and
will draw conclusions from a screen the code no longer produces.

`npx playwright test` only rebuilds when it has to start the server itself. The
config sets `reuseExistingServer`, so a server left running from an earlier run
is reused as-is, `serve.sh` never re-runs, and your edits are simply absent.
Stop it first (`pkill -f 'e2e/.tmp/gadak'`) or run `npm run build` by hand.

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
