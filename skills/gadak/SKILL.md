---
name: gadak
description: >
  Answer questions about Jira issues and Confluence pages from a local SQLite
  mirror instead of the Atlassian API — with SQL, so you can join, aggregate,
  and read history that JQL cannot express. Use when the user asks what
  changed, who is working on what, why a ticket was reopened, what the wiki
  says about something, what is stuck or untriaged, how a release is shaped, or
  anything that would otherwise mean paging through Jira search. Also use
  before any write — creating an issue, attaching a file, editing a summary,
  label or priority, commenting, transitioning, assigning — since all of those
  go through the same tool rather than the Atlassian API. When the user wants to *see* issues — show them, put them on
  screen, open this list — do not render a markdown table; open them in the
  gadak app with `gadak views open`.
---

# Asking the mirror

gadak keeps a local SQLite mirror of Jira (and optionally Confluence) at
`~/.gadak/gadak.db`. Reads never touch the network, so queries are free and fast:
prefer a query over asking the user to look something up.

**Show, don't paste.** SQL answers; `gadak views open` presents. Render a
markdown table or other document artifact only when there is no UI to focus,
or when the user explicitly asked for a document. If they asked to *see* the
issues, put them on the app:

```bash
gadak views open --jql 'project = NMA AND statusCategory = "In Progress"'
gadak views open --keys 'NMA-1,NMA-2'
gadak views open NMB-140
gadak sql --no-header "select key from issues_full where status_category != 'done'" | gadak views open --keys -
```

`--keys` accepts comma or whitespace; `--keys -` reads stdin. First-seen order
is kept, so the SQL `ORDER BY` is what the list shows. `gadak sql` prints a
header row first — skip it with `--no-header` (or `tail -n +2`) or that header
becomes a key.
`--keys` cannot be combined with `--jql` or a view name.

`gadak open <KEY>` is the Jira escape hatch (system browser to `/browse/KEY`).
`gadak views open` is the "open in gadak" verb (focus the running app or serve
tab). The names collide; the verbs do not.

First, check it is there and current:

```bash
gadak status --json     # counts, watermark, last_error, schema_version
```

`last_error` means the last sync failed. An old `watermark` on a quiet project
is normal — treat it as "possibly behind", not broken. If `gadak` is missing,
the mirror is not set up; say so rather than guessing at answers.

## Which mirror you are reading

Someone may have several: one per Jira site, kept apart as **profiles**. Ask,
rather than assume there is one:

```bash
gadak profiles --json   # name, active, configured, site_host, issues, documents, last_sync_at
```

`active` is the profile the command you just ran used. Nothing else sets it —
there is no stored "current profile" to switch. It comes from the command line
or the environment, both of which are visible in what you ran:

```bash
gadak --profile work sql "…"    # this call only
```

Three rules follow, and they are the point of the design:

- **Name the profile in every command that matters.** Never rely on the
  ambient default when a question is about a specific site — a shell that was
  configured before you arrived can point anywhere, and your transcript will
  not record which mirror answered.
- **Say which mirror you read.** `gadak status --json` carries a `profile`
  field; quote it when the answer could differ per site.
- **Never write to one site while reading another.** `gadak comment`,
  `transition`, `assign` and `api --write` all take the same `--profile`. If
  the question came from data in one mirror, the write goes to that same one.

If a profile shows `configured: false` it has no credential: its mirror can be
read if it was ever synced, but it will not be refreshed and writes will fail.
If `site_host` is empty or the row says `unreadable`, do not guess what site it
is — report it.

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

Output modes: `gadak sql "…"` is tab-separated *with a header row*,
`--no-header` omits that row (TSV and `--csv`), `--json` gives one object per
row, `--csv` is header plus CSV.

## Queries that cover most questions

```sql
-- Someone's open work, most urgent first
-- assignee_email can be empty when the site hides emails; prefer assignee_id after looking it up by name.
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

`gadak search "…"` is the shortcut when a query is overkill; `--json` includes a
`pages` array, and every hit says which field matched. If the user pastes Jira
JQL or a navigator URL, use `gadak search --jql '…'` (or pass the URL as the
query). Clauses the subset cannot express are printed on stderr and must be
repeated to the user — do not pretend the list is what Jira would have shown.

To put the human on a view, do not describe the filters — set them:

```bash
gadak views                         # names after `gadak sync` (owned + starred Jira filters)
gadak views open "the name"         # focuses the running desktop app or serve tab
gadak views open --jql 'project = NMA AND statusCategory = "In Progress"'
gadak views open --keys 'NMA-1,NMA-2'
gadak sql --no-header "select key from issues_full where status_category != 'done'" | gadak views open --keys -
gadak views open NMB-140            # focus that issue's detail (a stored view with that name wins)
gadak views save "Night triage" --jql '…'   # keep a named view in the mirror
```

`views open` writes a one-shot hash the UI applies (`ks=` for `--keys`,
`issue=` for a positional key); it also opens a serve tab when one is
listening, and focuses Gadak.app on macOS (the `--profile` is passed through
so the window and the file match). `--no-open` writes the hash only. `--json`
prints the hash and where it was sent. Confirm you named `--profile` if the
user has more than one mirror. `gadak open` is the Jira-site escape hatch;
`gadak views open` is open-in-gadak.

## One issue, and writes

```bash
gadak issue NMB-140 --json                    # fields, description, comments, history
gadak issue NMB-140 --derive                  # why reopen_count / resolved_at / epic_key are what they are

gadak comment NMB-140 -m "Reproduced on staging."
gadak comment NMB-140 -m -                    # body from stdin, for anything multi-line
gadak transition NMB-140 "In Review"
gadak assign NMB-140 dana@example.com         # `-` unassigns

gadak create Batch worker drops the last page --project NMB --type Bug -m "repro on staging" --parent NMB-1
gadak create --batch -                        # one JSON object per line on stdin
gadak attach NMB-140 screenshot.png trace.log
gadak edit NMB-140 --summary "…" --label +regression --label -needs-triage --priority High --parent none
```

Writes go to Jira and re-read the issue into the mirror afterwards. Confirm
with the user before writing — a created issue, comment or transition is
visible to their whole team.

There is no separate write API to discover: everything gadak can change is one
of the verbs above. If a field is not covered, say so rather than reaching for
the REST API — `gadak api` exists for that, but it is an escape hatch, not the
path of least surprise.

## Rules that come with the file

- **Never write to the database.** Writes are Jira's job; a row written
  directly is destroyed by the next sync. No exception for "just a label".
- **Do not depend on `issues.raw`.** It is shaped by Jira's API, not by gadak's
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
gadak api GET /rest/api/3/issue/NMB-140/watchers
```

Read-only unless `--write` is passed. Prefer the mirror when it can answer —
this path costs a network round trip and draws on the site's rate budget.

## More

`gadak sql "select ..."` against `specs/000-product/data-model.md` covers every
column; `docs/RECIPES.md` in the gadak repository has more worked questions.
`gadak doctor` prints a redacted summary of the install when something looks
wrong.
