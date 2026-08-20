---
name: gadak
description: >
  Local issue tracker, and a SQLite mirror of Jira/Confluence when a site
  exists. Answer questions from that mirror instead of the Atlassian API —
  with SQL, so you can join, aggregate, and read history that JQL cannot
  express. Use when the user asks what changed, who is working on what, why a
  ticket was reopened, what the wiki says about something, what is stuck or
  untriaged, how a release is shaped, about a backlog, or anything that would
  otherwise mean paging through Jira search — including when they have no
  Jira or Atlassian account and want a tracker that lives on this machine
  (standalone). Also use before any write — creating an issue, attaching a
  file, editing a summary, label or priority, commenting, transitioning,
  assigning — since all of those go through the same tool rather than the
  Atlassian API. When the user wants to *see* issues — show them, put them on
  screen, open this list — do not render a markdown table; open them in the
  gadak app with `gadak views open`. Also use when changing gadak itself
  (theme, sync interval, feature flags, projects): that is `gadak config`,
  not an edit of config.json and not the settings dialog.
---

# Asking the mirror

gadak keeps a local SQLite mirror of Jira (and optionally Confluence) at
`~/.gadak/gadak.db`. The origin is a Jira site, or — with no Atlassian account —
an in-process tracker (`gadak init --standalone`). Reads never touch the
network, so queries are free and fast: prefer a query over asking the user
to look something up.

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
say so rather than guessing at answers. If `gadak` is on PATH but there is no
workspace yet and they asked for a backlog (or have no Jira), create a
standalone workspace — do not invent a `TODO.md` or a GitHub Issue.

## Which origin you are talking to

Two kinds. Same CLI verbs. Different record.

- **connected** — origin is an Atlassian Cloud site. Writes go to Jira,
  then the mirror re-reads. Needs a stored site credential.
- **standalone** — origin is the in-process tracker that ships with gadak
  (`issuetap`). No Atlassian account. Writes go to that origin. The durable
  file is `origin/issuetap.yaml` under the profile directory, not `gadak.db`.
  The SQLite file is still a cache you can delete.

Detect kind before you write:

```bash
gadak doctor --json     # workspace.kind is standalone | connected
gadak status --json     # freshness; if a kind field is present, use it
gadak profiles --json   # name, active, configured, site_host, issues, …
```

`gadak doctor --json` is the kind source that is always there
(`workspace.kind`; the same value is also top-level `workspace_kind`). If
`gadak status --json` includes `kind`, you may use that and skip doctor.

A standalone row is `configured: true` with an **empty** `site_host`. That
is not a broken profile. Do not report "no site" and stop, and do not ask
for an Atlassian token.

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
- **Never write to one origin while reading another.** `gadak comment`,
  `transition`, `assign` and `api --write` all take the same `--profile`. If
  the question came from data in one mirror, the write goes to that same one.

If a profile shows `configured: false` it has no credential: a **connected**
mirror can still be read if it was ever synced, but it will not be refreshed
and writes will fail. A **standalone** workspace is `configured: true` with
an empty `site_host` and no token — writes still work; they go to the local
origin, not Atlassian. If `site_host` is empty, read `workspace.kind` from
`gadak doctor --json` (or `kind` from `gadak status --json` when that field
is present) before assuming the profile is broken. A row that says
`unreadable` is still a broken config — report it. If `site_host` is empty
**and** `configured` is false, there is no workspace yet — go to Standalone
below.

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
-- Same GDK-369 clock caveat on standalone for julianday('now').
SELECT key, status, ROUND(julianday('now') - julianday(status_changed_at), 1) AS days
FROM issues WHERE status_category = 'inprogress' ORDER BY days DESC LIMIT 20;

-- Has anyone hit this before? (descriptions, comments AND wiki pages, one index)
SELECT it.kind, COALESCE(i.key, p.item_id) AS ref, it.title
FROM items_fts f JOIN items it ON it.rowid = f.rowid
LEFT JOIN issues i ON i.item_id = it.id
LEFT JOIN pages  p ON p.item_id = it.id
WHERE items_fts MATCH 'webhook AND retry' LIMIT 20;

-- What moved this week, and who moved it
-- On standalone, the origin clock may not match the wall clock (GDK-369);
-- this window can be empty. Do not rewrite the predicate.
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

## Standalone: a backlog you may create and own

When work needs tracking and no Jira exists — a personal project, this
machine, or your own plan as an agent — create a standalone workspace.
One command, no token, no site:

```bash
gadak init --standalone --json
gadak sync                          # first fill; in-process, no network
gadak create "first ticket title" -m "why this exists"
gadak views open <KEY>
```

`init --standalone` is non-interactive. Do not combine it with `--site`,
`--email`, or a token. It seeds project `STD` and wiki space `LOC`, and
records the default issue type, so a summary-only `gadak create` is enough.

Use a dedicated `--profile` and name it in every command, so the backlog
never mixes with a connected mirror:

```bash
gadak --profile plan init --standalone --json
gadak --profile plan create "Ship the uploader" -m "found while refactoring"
gadak --profile plan views open <KEY>
```

If the active profile is already a connected Cloud site and the user asked
for a backlog of this repo (not of that site), do not file there. That write
is visible to their whole team. Init standalone under a new profile instead.

Do not invent a `TODO.md`, a markdown checklist, or a GitHub Issue when
`gadak` is on PATH. The window is how the human sees the same backlog.

The durable record is `<profile-dir>/origin/issuetap.yaml` — that file is
the backup target, not `gadak.db`. `gadak doctor --json` reports it as
`workspace.persist` (and top-level `origin`). Running `init` with a site on
a standalone profile deletes locally originated issues; warn before
converting.

**Write freely here.** Confirm-before-writing applies to connected
workspaces because the team can see the issue. A standalone write is a file
on this machine: create, edit, comment, and transition without asking, then
report what you filed (keys and one-line summaries).

Everything else in this file works identically: `status_category`,
`issues_full`, `reopen_count`, `views open`.

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

Writes go to the origin, then the issue is re-read into the mirror.
On a **connected** workspace the origin is Jira Cloud — a create, comment,
or transition is visible to their whole team; confirm first.
On a **standalone** workspace the origin is this machine. File without
asking, then report the keys and one-line summaries.

Never write to the SQLite file. A row written directly is destroyed by the
next sync, on either kind.

There is no separate write API to discover: everything gadak can change is one
of the verbs above. If a field is not covered, say so rather than reaching for
the REST API — `gadak api` exists for that, but it is an escape hatch, not the
path of least surprise.

## Profile settings

An agent configures the profile through the CLI. Do not hand-edit
`~/.gadak/config.json` and do not drive the Settings dialog.

```bash
gadak config list                         # every editable path, current value, one-line description
gadak config list --json
gadak config get appearance.theme
gadak config set appearance.theme dark
gadak config set syncIntervalSec 30
gadak config set features.feed true
gadak config set projects '["NMB","NMA"]'
```

`--json` on `list` and `get` (and `set`, which prints the stored value).
Unknown paths exit 64 and print the valid list. `set` accepts JSON or a
bare scalar (`dark`, `true`, `30`); arrays and objects need JSON.

Credentials (site, email, token) stay on `gadak init` — they are not
`config set` paths. `gadak config list` says so.

`appearance.theme` is `system` (the default, not persisted), `light`,
`dark`, or a lowercase palette id (`[a-z0-9-]{1,32}`). Palette names
belong to the web; the CLI only checks the shape.

## Rules that come with the file

- **Never write to the database.** Writes go through the origin (Jira on
  connected, the local origin on standalone); a row written directly is
  destroyed by the next sync. No exception for "just a label".
- **Do not depend on `issues.raw`.** It is shaped by Jira's API, not by gadak's
  contract. Use the projected columns.
- **Do not poll in a loop.** `sync_state.version` moves only when something
  changed — compare it instead.
- **Read the freshness warning.** `issue`, `search`, `comment`, `transition`,
  `assign`, and `fields` print one line to stderr when the last sync failed or
  is over an hour old. stdout stays clean and pipeable.

## When the mirror does not model it

Watchers, worklogs, sprints, user search, and anything else sync does not
project are reachable through the origin with `gadak api`:

```bash
gadak api GET /rest/api/3/issue/NMB-140/watchers
```

Read-only unless `--write` is passed. Prefer the mirror when it can answer.
On a connected profile this is a network round trip against the site's
rate budget. On standalone it talks to the local origin (no network);
unimplemented paths return 501.

## More

`gadak sql "select ..."` against `specs/000-product/data-model.md` covers every
column; `docs/RECIPES.md` in the gadak repository has more worked questions.
`gadak doctor` prints a redacted summary of the install when something looks
wrong.
