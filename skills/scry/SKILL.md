---
name: scry
description: >
  Answer questions about Jira issues and Confluence pages from a local SQLite
  mirror instead of the Atlassian API — with SQL, so you can join, aggregate,
  and read history that JQL cannot express. Use when the user asks what
  changed, who is working on what, why a ticket was reopened, what the wiki
  says about something, what is stuck or untriaged, how a release is shaped, or
  anything that would otherwise mean paging through Jira search. Also use
  before writing a comment, transition, or assignment, since those go through
  the same tool.
---

# Asking the mirror

scry keeps a local SQLite mirror of Jira (and optionally Confluence) at
`~/.scry/scry.db`. Reads never touch the network, so queries are free and fast:
prefer a query over asking the user to look something up.

First, check it is there and current:

```bash
scry status --json     # counts, watermark, last_error, schema_version
```

`last_error` means the last sync failed. An old `watermark` on a quiet project
is normal — treat it as "possibly behind", not broken. If `scry` is missing,
the mirror is not set up; say so rather than guessing at answers.

Other profiles: `scry --profile work sql "…"` (mirror under
`~/.scry/profiles/work/`).

## The one mistake that silently returns nothing

Filter on ids and categories, never on display names. Jira translates
`status.name` and `issuetype.name` per account, so a name that works on your
site returns zero rows on someone else's.

```sql
WHERE status = 'In Progress'          -- WRONG: empty on a localized account
WHERE status_category = 'inprogress'  -- RIGHT: stable everywhere
```

`status_category` is `new`, `inprogress`, or `done`. `status_id` and
`issue_type_id` are stable too.

## The schema in one paragraph

`items` is the source-neutral spine (title, `body_text`, timestamps). `issues`
is the Jira projection, joined on `issues.item_id = items.id`. **`issues_full`
is the view to reach for** — every `issues` column plus `summary`. `pages` is
the Confluence projection. `comments`, `attachments`, `changelog`, and `links`
hang off `items.id`. `items_fts` is one FTS5 index over titles, bodies, and
comment text — issues and wiki pages together. `labels`, `components`, and
`fix_versions` are JSON arrays; reach them with `json_each`.

Some columns exist only here, derived from the changelog while syncing:
`reopen_count`, `reopened_at`, `reopen_reason`, and `epic_key` (the nearest
level-1 ancestor). Jira cannot answer questions about these at all.

Output modes: `scry sql "…"` is tab-separated, `--json` gives one object per
row, `--csv` adds a header row.

## Queries that cover most questions

```sql
-- Someone's open work, most urgent first
SELECT key, status, priority, summary FROM issues_full
WHERE assignee_email = 'dana@example.com' AND status_category != 'done'
ORDER BY priority_rank, updated_at DESC;

-- What regressed (reopens are the highest-signal quality metric here)
SELECT key, summary, reopen_count, reopen_reason FROM issues_full
WHERE reopen_count > 0 ORDER BY reopen_count DESC, reopened_at DESC LIMIT 20;

-- What is stuck, and for how long
SELECT key, status, ROUND(julianday('now') - julianday(status_changed_at), 1) AS days
FROM issues WHERE status_category = 'inprogress' ORDER BY days DESC LIMIT 20;

-- Has anyone hit this before? (descriptions, comments AND wiki pages, one index)
SELECT it.kind, COALESCE(i.key, p.item_id) AS ref, it.title
FROM items_fts f JOIN items it ON it.rowid = f.rowid
LEFT JOIN issues i ON i.item_id = it.id
LEFT JOIN pages  p ON p.item_id = it.id
WHERE items_fts MATCH 'webhook AND retry' LIMIT 20;

-- What moved this week, and who moved it
SELECT c.at, c.author, c.field, c.from_value, c.to_value, i.key
FROM changelog c JOIN issues i ON i.item_id = c.item_id
WHERE c.at > datetime('now', '-7 days') ORDER BY c.at DESC LIMIT 50;

-- Untriaged: nobody on it, no priority set
SELECT key, created_at, summary FROM issues_full
WHERE status_category = 'new' AND assignee_id IS NULL AND priority_rank = 0
ORDER BY created_at LIMIT 30;

-- Everything one person wrote, across issues and pages
SELECT it.kind, it.title, substr(c.body_text, 1, 80), c.created_at
FROM comments c JOIN items it ON it.id = c.item_id
WHERE c.author = 'Dana Whitfield' ORDER BY c.created_at DESC LIMIT 20;
```

`scry search "…"` is the shortcut when a query is overkill; `--json` includes a
`pages` array, and every hit says which field matched.

## One issue, and writes

```bash
scry issue NMB-140 --json                    # fields, description, comments, history
scry comment NMB-140 -m "Reproduced on staging."
scry transition NMB-140 "In Review"
scry assign NMB-140 dana@example.com
```

Writes go to Jira and re-read the issue into the mirror afterwards. Confirm
with the user before writing — a comment or transition is visible to their
whole team.

## Rules that come with the file

- **Never write to the database.** Writes are Jira's job; a row written
  directly is destroyed by the next sync. No exception for "just a label".
- **Do not depend on `issues.raw`.** It is shaped by Jira's API, not by scry's
  contract. Use the projected columns.
- **Do not poll in a loop.** `sync_state.version` moves only when something
  changed — compare it instead.
- **Read the freshness warning.** `issue`, `search`, `comment`, `transition`,
  `assign`, and `fields` print one line to stderr when the last sync failed or
  is over an hour old. stdout stays clean and pipeable.

## When the mirror does not model it

Watchers, worklogs, sprints, user search, and anything else sync does not
project are reachable through the site itself with the stored credential:

```bash
scry api GET /rest/api/3/issue/NMB-140/watchers
```

Read-only unless `--write` is passed. Prefer the mirror when it can answer —
this path costs a network round trip and draws on the site's rate budget.

## More

`scry sql "select ..."` against `specs/000-product/data-model.md` covers every
column; `docs/RECIPES.md` in the scry repository has more worked questions.
`scry doctor` prints a redacted summary of the install when something looks
wrong.
