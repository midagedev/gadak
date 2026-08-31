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
  assigning, creating or editing a wiki page — since all of those go through
  the same tool rather than the Atlassian API. When the user wants to *see*
  issues — show them, put them on screen, open this list — do not render a
  markdown table; open them in the gadak app with `gadak views open`. Also use
  when changing gadak itself (theme, sync interval, feature flags, projects):
  that is `gadak config`, not an edit of config.json and not the settings
  dialog.
---

# Asking the mirror

gadak keeps a local SQLite mirror of Jira (and optionally Confluence) at
`~/.gadak/gadak.db`. The origin is a Jira site, or — with no Atlassian account —
an in-process tracker (`gadak init --standalone`), or another machine's
`gadak serve` bound with `gadak init --pairing-code-stdin`. Reads never touch
the network, so queries are free and fast: prefer a query over asking the user
to look something up.

If `gadak doctor` reports `skill: stale`, run `gadak skill install` so this
file matches the binary you are running. An upgrade of a previous gadak
install is in-place; a file gadak did not write needs `--force`.

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
gadak sync --if-stale 15m   # no-op when fresh; one incremental pass if older than 15m or last sync failed
gadak status --json     # counts, watermark, last_error, schema_version, custom_fields.mapped
```

`gadak sync --if-stale 15m` is the session preamble on a CLI-only host: it
returns immediately when every source is fresh, and runs one incremental
pass when a source is older than 15m or its last sync failed. A running
`gadak serve` keeps the mirror fresh on its own.

`last_error` means the last sync failed. An old `watermark` on a quiet project
is normal — treat it as "possibly behind", not broken. `watermark` /
`sync_count` / `last_error` are the issue-source row (Jira when it has run,
Linear when Linear is the only issue source that has; `sources.jira` /
`sources.linear` for per-source rows). If `gadak` is missing,
say so rather than guessing at answers. If `gadak` is on PATH but there is no
workspace yet and they asked for a backlog (or have no Jira), create a
standalone workspace — do not invent a `TODO.md` or a GitHub Issue.

After a context compaction, or when resuming someone else's session on an
existing workspace, the first command is `gadak recents`: the keys this
workspace read most recently, newest first. Reads record themselves —
`gadak issue` and `gadak search` append the visit/search rows to `local.db`
as they run, so the list is already there when you need it.

```bash
gadak recents                    # kind, key, viewed_at — TSV with a header
gadak recents --json --limit 50
```

## Which origin you are talking to

Two kinds. Same CLI verbs. Different record.

- **connected** — origin is an Atlassian Cloud site, *or* another machine's
  `gadak serve` (paired). Writes go to that origin, then the mirror re-reads.
  A Cloud site needs a stored credential. A paired workspace still reports
  `workspace.kind: connected`; `gadak status --json` adds a `pairing` object
  (`endpoint`, `label`) and the text form prints `paired with "…"`.
- **standalone** — origin is the in-process tracker that ships with gadak
  (`issuetap`). No Atlassian account. Writes go to that origin. The durable
  file is `origin/issuetap.yaml` under the workspace directory, not `gadak.db`.
  The SQLite file is still a cache you can delete.

Detect kind before you write:

```bash
gadak doctor --json     # workspace.kind is standalone | connected
gadak status --json     # freshness; pairing object when this workspace is bound to a remote serve
gadak workspaces --json # name, active, configured, site_host, issues, …
```

`gadak doctor --json` is the kind source that is always there
(`workspace.kind`; the same value is also top-level `workspace_kind`). If
`gadak status --json` includes `kind`, you may use that and skip doctor.
Paired is not a third kind value.

A standalone row is `configured: true` with an **empty** `site_host`. That
is not a broken workspace. Do not report "no site" and stop, and do not ask
for an Atlassian token.

## Which mirror you are reading

Someone may have several: one per Jira site, kept apart as **workspaces**. Ask,
rather than assume there is one:

```bash
gadak workspaces --json   # name, active, configured, site_host, issues, documents, last_sync_at
```

`active` is the workspace the command you just ran used. Nothing else sets it —
there is no stored "current workspace" to switch. It comes from the command line
or the environment, both of which are visible in what you ran:

```bash
gadak --workspace work sql "…"    # this call only
```

`--profile` is an alias of `--workspace`; existing scripts and MCP installs
that pass `--profile` keep working. `gadak profiles` is the same command as
`gadak workspaces`. To remove a workspace entirely:
`gadak workspaces rm <name> --yes` — a standalone workspace additionally
needs `--destroy-origin` (its persist is the only copy of that tracker; the
refusal names the file to copy out first).

Three rules follow, and they are the point of the design:

- **Name the workspace in every command that matters.** Never rely on the
  ambient default when a question is about a specific site — a shell that was
  configured before you arrived can point anywhere, and your transcript will
  not record which mirror answered. The one exception is gadak's own terminal:
  it sets `GADAK_WORKSPACE` to the workspace its window shows, so a bare
  `gadak` there is on that workspace by construction — but the transcript
  still records nothing, so name `--workspace` on writes.
- **Say which mirror you read.** `gadak status --json` carries a `profile`
  field (the workspace name; empty for the root); quote it when the answer
  could differ per site.
- **Never write to one origin while reading another.** `gadak comment`,
  `transition`, `assign` and `api --write` all take the same `--workspace`. If
  the question came from data in one mirror, the write goes to that same one.

If a workspace shows `configured: false` it has no credential: a **connected**
mirror can still be read if it was ever synced, but it will not be refreshed
and writes will fail. A **standalone** workspace is `configured: true` with
an empty `site_host` and no token — writes still work; they go to the local
origin, not Atlassian. If `site_host` is empty, read `workspace.kind` from
`gadak doctor --json` (or `kind` from `gadak status --json` when that field
is present) before assuming the workspace is broken. A row that says
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
is the view to reach for** — every `issues` column plus `summary` and
`description_text` (`items.body_text`, flattened). Sprint is three columns on
`issues` (`sprint_id`, `sprint_name`, `sprint_state` — filter on id or state,
never the name). `versions` is the project catalog; join it on
`fix_version_ids` (same-order ids next to the name array `fix_versions`).
`pages` is
the Confluence projection. `comments`, `attachments`, `changelog`, `links`,
and `dev_links` (development-panel PRs) hang off `items.id`. `items_fts` is
one FTS5 index over titles, bodies, and
comment text — issues and wiki pages together. `labels`, `components`, and
`fix_versions` are JSON arrays; reach them with `json_each`. Mapped custom
fields live in `issues.custom` (and `issues_full.custom`) keyed by alias —
only after `gadak fields --apply`. If `gadak status --json` shows
`custom_fields.mapped` of 0, an empty `json_extract(custom, '$.alias')` may
mean mapping has not run, not that the field is blank:

```sql
SELECT key, json_extract(custom, '$.story_points') AS sp
FROM issues_full WHERE json_extract(custom, '$.story_points') IS NOT NULL;
```

Personal state lives in `local.db` beside the mirror (ATTACHed as `local`;
you do not type ATTACH): `local.saved_views` (`gadak views save`),
`local.recipes` (`gadak recipes save`), `local.visits`, `local.searches`.
It survives deleting `gadak.db`.

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
-- gadak assign accepts that accountId (or the display name) when email is hidden; ambiguous names are refused with the candidates.
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
-- ref is the issue key, or for kind='page' the origin page id (items.key —
-- the id `gadak page get` prints and `gadak page edit` takes; NOT
-- pages.item_id, an internal join id).
SELECT it.kind, COALESCE(i.key, it.key) AS ref, it.title, p.space_key
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

-- Work in the active sprint (sprint_id / sprint_state are the stable keys;
-- sprint_name is a localized display name — never filter on it)
SELECT key, summary, sprint_id, sprint_name
FROM issues_full
WHERE sprint_state = 'active' AND status_category != 'done'
ORDER BY priority_rank, updated_at DESC;

-- What blocks one issue (links are stored from both ends: the blocked issue
-- carries the inward row, the blocker the outward one)
SELECT l.target_key AS blocked_by, t.status_category
FROM links l
JOIN issues_full i ON i.item_id = l.item_id
JOIN issues_full t ON t.key = l.target_key
WHERE i.key = 'NMA-24' AND l.type = 'Blocks' AND l.direction = 'inward';

-- Everything under one epic (epic_key is derived: nearest level-1 ancestor)
SELECT key, status_category, priority_rank, summary FROM issues_full
WHERE epic_key = 'NMB-194' ORDER BY status_category, priority_rank, key;
```

Blockers both ways, duplicate networks, open work per component or per epic,
overdue-not-done, comments keyed on `author_id`, and @-mentions read out of
the ADF body are worked queries in `docs/RECIPES.md` ("Blockers and
duplicates", "Epics, components, and due dates").

## Listing work: list, ready, next

`gadak list` is the default open-issues read — run it before writing any
query. Every issue with `status_category != 'done'`, highest
`priority_rank` first, `updated_at` newest first as the tiebreak, 30 rows.
Both `priority` and `priority_rank` are in the output so the ordering can
be checked without a second query. `--limit N`, `--json`, `--csv`,
`--no-header` behave like `gadak sql`; `--all` includes done issues.

`gadak ready` (or `gadak list --ready`) narrows that list to issues no
open blocker holds back: an inward Blocks link whose target issue is not
done disqualifies. The blocking link type resolves against the origin's
link-type catalog — the same vocabulary `gadak link --type` resolves —
never a hardcoded name. When no catalog can answer (no credential, offline,
Linear), a stderr notice says so and the plain open list is shown; an empty
"nothing ready" would be a stronger and wronger claim.

```bash
gadak list                          # open issues, priority rank first
gadak list --limit 5 --json
gadak list --all                    # include done
gadak ready                         # open issues nothing open blocks
```

A recipe is a name for a mirror SQL query, stored in `local.recipes`. It is
not a ranking engine — order comes from `priority_rank` / `status_category`.
`gadak next` (alias `gadak pick`) runs the recipe named next when one is
saved; with none saved it runs the `gadak list` default and prints the save
command on stderr. This is a report, not occupancy — claiming still goes
through the origin (`gadak claim`).

```bash
gadak recipes save next "select key, priority_rank, status, summary from issues_full where status_category != 'done' order by priority_rank, updated_at desc limit 10"
gadak next                          # saved recipe, or the built-in default
gadak pick                          # same command, the changelog's name
gadak recipes run next --json       # the runner: still an error when unsaved
gadak recipes show next             # SQL text; pipeable into save
gadak recipes show next | gadak recipes save next -m -
gadak recipes                       # name, updated_at, sql preview
gadak recipes rm next
```

`gadak search "…"` is the shortcut when a query is overkill; `--json` includes a
`pages` array, and every hit says which field matched. If the user pastes Jira
JQL or a navigator URL, use `gadak search --jql '…'` (or pass the URL as the
query). Clauses the subset cannot express are printed on stderr and must be
repeated to the user — do not pretend the list is what Jira would have shown.

CJK queries of two or more runes match inside a compound (`결제` hits
`간편결제`). English middles still miss (`ency` does not hit a title that is
only `idempotency`). JQL `project NOT IN (KEY, …)` applies; `status NOT IN`
does not (`cannot apply JQL — status not in … (only = and IN)`). Prefer
`status_category` / `status_id` in SQL — never `status = 'In Progress'`.

To put the human on a view, do not describe the filters — set them:

```bash
gadak views                         # Jira filters (after sync) + saved views
gadak views open "the name"         # focuses the running desktop app or serve tab
gadak views save "Night triage" --jql '…'   # keep a named view in local.db (survives deleting the mirror)
```

`views open` writes a one-shot hash the UI applies (`ks=` for `--keys`,
`issue=` for a positional key); it also opens a serve tab when one is
listening, and focuses Gadak.app on macOS (the `--workspace` is passed through
so the window and the file match). `--no-open` writes the hash only. `--json`
prints the hash and where it was sent. Confirm you named `--workspace` if the
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

Use a dedicated `--workspace` and name it in every command, so the backlog
never mixes with a connected mirror:

```bash
gadak --workspace plan init --standalone --json
gadak --workspace plan create "Ship the uploader" -m "found while refactoring"
gadak --workspace plan views open <KEY>
```

If the active workspace is already a connected Cloud site and the user asked
for a backlog of this repo (not of that site), do not file there. That write
is visible to their whole team. Init standalone under a new workspace instead.

Do not invent a `TODO.md`, a markdown checklist, or a GitHub Issue when
`gadak` is on PATH. The window is how the human sees the same backlog.

The durable record is `<workspace-dir>/origin/issuetap.yaml` — that file is
the backup target, not `gadak.db`. `gadak doctor --json` reports it as
`workspace.persist` (and top-level `origin`). Running `init` with a site on
a standalone workspace deletes locally originated issues; warn before
converting.

**Write freely here.** Confirm-before-writing applies to connected
workspaces because the team can see the issue. A standalone write is a file
on this machine: create, edit, comment, and transition without asking, then
report what you filed (keys and one-line summaries).

Everything else in this file works identically: `status_category`,
`issues_full`, `reopen_count`, `views open`.

## Pairing: the origin is another machine's serve

Home (standalone, with `gadak serve` running) mints one offer per device.
The remote binds a *fresh* workspace. After that, every verb on that workspace
uses the home serve as origin.

```bash
gadak pairing mint --label laptop                 # home: stdout is one offer line
gadak --workspace laptop init --pairing-code-stdin  # remote: paste the offer
gadak --workspace laptop status                     # kind is connected; prints paired with "laptop"
gadak pairing list                                # home: token table; remote: one status line
gadak pairing revoke laptop                       # home only
```

Do not combine `--pairing-code-stdin` with `--standalone` or a site token.
`_home` is this machine's routing token, not a device — `revoke` refuses it;
`gadak pairing mint --label _home` rotates it. If a command fails with a
`pairing:` prefix, show that error to the user. Do not invent a retry.

## Writing as yourself: the actor

On a standalone or paired workspace, every write records who made it. Set
your identity before the first write so comments and transitions attribute
to you, not the workspace's default user:

```bash
export GADAK_ACTOR="claude:354bff2b|Claude (build 1)"   # slug | display name
gadak status          # the actor row confirms recognition (--json: actor.slug, actor.source)
```

Claude Code is detected automatically — no export needed; each session
writes as `claude:<session prefix>`. A slug is a stable identity: pick one
per agent and keep it across sessions. The machine's fallback lives in
`gadak config set actor '{"slug":"grok:aa11","name":"Grok"}'` (the env
value wins over it). The actor reaches only the standalone/paired origin —
never a connected Cloud site or any other outbound request. Without one,
writes attribute to the workspace's default user, exactly as before.

The attribution is queryable — "what did the previous session leave" is
one query, not archaeology across four tables. Every write surface carries
`author_id` (`items` for created issues and pages, `comments`, `changelog`,
`page_versions`), so filter each on the actor slug and union the results;
the ready-made timeline query is in `docs/RECIPES.md` under "What a
session left". The current session's slug is `actor.slug` in
`gadak status --json`.

When several agents work one backlog, claiming is a write, not a comment
convention: `gadak claim <KEY>` takes an issue as yours — assignee plus the
in-progress transition in one step — and refuses (exit 75, the holder's
name in the error) while another actor holds it. Use it instead of a
"[claim]" comment; `--take-over` replaces the holder only when the human
says to. `gadak issue KEY` answers "how long has this sat?" with its
`durations` line (wait = created → first in-progress, progress = in-progress
→ done or now), computed from the changelog — never stored.

## One issue, and writes

On **standalone**, `init` seeds project `STD` and records a default issue type,
so a summary-only create is enough. On **connected**, use a key and project
that exist on that site (the `NMB-140` lines below are an example, not a
universal project). A paired workspace is `connected` with no default project
or type — `create` will ask for `--project` / `--type` until you set them.

```bash
# standalone (seed project STD)
gadak create "first ticket title" -m "why this exists"
gadak issue STD-1 --json
gadak comment STD-1 -m "Reproduced on staging."
gadak assign STD-1 you@example.com            # the one seeded user; example.com emails from connected docs do not exist here
gadak claim STD-1                             # take it as yours: assignee + in-progress transition; refuses while another actor holds it (exit 75)
gadak dev link STD-1 --pr https://github.com/org/app/pull/7   # opened a PR? record it right here
gadak dev scan                                                # or sweep the repo: keys in PR titles/branches → links

# connected: a key that exists on that site
gadak issue NMB-140 --json                    # fields, description, comments, history
gadak issue NMB-140 --editmeta                # which configured fields this issue can edit (origin GET; not stored)
gadak issue NMB-140 NMB-141 --json            # JSON array of the same documents; omit --json for text with --- KEY --- between them
gadak sql --no-header "select key from issues_full where parent_key='NMB-140'" | gadak issue --keys -
gadak issue NMB-140 --derive                  # why reopen_count / resolved_at / epic_key are what they are
gadak comment NMB-140 Reproduced on staging.  # positional body
gadak comment NMB-140 -m "Reproduced on staging."
gadak comment NMB-140 -m "thanks @Dana"       # @Name resolves to a site user; ambiguous names are refused
gadak comment NMB-140 -m -                    # body from stdin, for anything multi-line
gadak comment NMB-140 -m "done" --visibility role=Administrators
gadak comment NMB-140 -m "done" --internal    # JSM internal
gadak transition NMB-140                      # list tokens this credential can fire
gadak transition NMB-140 "In Review"
gadak transition NMB-140 done                 # status category: new | inprogress | done
gadak close NMB-140                           # same as transition KEY done; already done is a no-op
gadak transition NMB-140 done --resolution "Won't Do" -m "fixed in 1.2"
gadak assign NMB-140 dana@example.com         # email, display name, or accountId; `-` unassigns. Ambiguous names are refused with the candidates.
gadak claim NMB-140                            # Cloud has no atomic claim: assignee + transition run as two calls (warned on stderr); held issues refuse with exit 75
gadak claim NMB-140 --take-over                # replace the current holder
gadak create Batch worker drops the last page --project NMB --type Bug -m "repro on staging" --parent NMB-1
gadak create Severity required --project NMB --type Task --field severity=High
gadak attach NMB-140 screenshot.png trace.log
gadak edit NMB-140 --summary "…" --label +regression --label -needs-triage --priority High --parent none
gadak edit NMB-140 --type Task                # name, localized name, or id — same resolver as create --type
gadak edit NMB-140 -m "plain-text body"       # plain replace; a formatted description refuses without --force-plain
gadak edit NMB-140 --component +SDK --component -Docs
gadak edit NMB-140 --fix-version +v2.5 --fix-version -10012
gadak edit NMB-140 --field severity=High
gadak link NMB-140 NMB-141 --type blocks          # A blocks B; "is blocked by" means A is blocked by B
gadak unlink NMB-140 NMB-141 --type blocks        # removes that link (live id lookup; the mirror keeps no link ids)

gadak create --batch -                        # one JSON object per line on stdin (stops at the first failure)
gadak comment --batch -                       # JSON lines {"key","body"}; tries every line; one envelope row per key
gadak transition --batch -                    # JSON lines {"key","target"}; --dry-run writes nothing (--json adds transition_id)
gadak assign --batch -                        # JSON lines {"key","assignee"}; "-" unassigns
gadak edit --batch -                          # JSON lines {"key"} plus summary, labels (+x/-x), type, priority, due, parent, fields
gadak fields --apply                          # map in-use custom fields, then edit --field alias=value
gadak project create IDEA --name Ideas        # grow a standalone workspace by a project
gadak transition NMB-140 done --field environment=staging
gadak search NMB-140 --explain                # why each hit ranked: key-exact, key-prefix, or fts
```

`--batch -` on comment, transition, assign, and edit reads one JSON object per stdin line (at most 50). A line that fails does not stop the rest; stdout is one envelope row per line (`key`, `ok`, `changed`, `error`; `--json` is JSON lines). `create --batch -` still stops at the first failure.

Custom-field writes follow this order: `gadak fields --apply` (save aliases) →
`gadak issue KEY --editmeta` (which of those aliases this issue can edit) →
`gadak edit KEY --field alias=value` or `gadak create … --field alias=value`.

`gadak comment` resolves `@Name` to a site user (account id) before sending.
Ambiguous names are refused with the candidates and no comment is posted. A
name that matches nobody stays plain text and is named on stderr; stdout stays
pipeable.

Wiki pages — read from the local mirror (no network), write through the
origin (connected Confluence, or standalone issuetap). Standalone seed space
is `LOC`; connected: a space key that exists on that site. `gadak wiki` runs
every one of these (`gadak wiki get <ID>` = `gadak page get <ID>`):

```bash
gadak page list                                  # id, space, title, updated_at — newest first; where <ID> comes from
gadak page get <ID>                              # title, body, comments from the mirror
gadak page get <ID> --json
gadak page create --space LOC --title "Retention notes" -m "first draft"
gadak page edit <ID> --title "Renamed"
gadak page comment <ID> -m "a question"
```

Writes go to the origin, then the issue (or page) is re-read into the mirror.
On a **connected** Cloud workspace the origin is Jira — a create, comment,
or transition is visible to their whole team; confirm first.
On a **standalone** workspace the origin is this machine. File without
asking, then report the keys and one-line summaries.
On a **paired** workspace (`status --json` has `pairing`), writes go to the
home serve, not Atlassian.

Never write to the SQLite file. A row written directly is destroyed by the
next sync, on either kind.

Discover flags from the binary, not from memory: `gadak <verb> --help` lists
what that verb accepts, and `gadak issue KEY --editmeta` lists which configured
custom fields this issue can edit. If a field still is not there, say so rather
than reaching for the REST API — `gadak api` exists for that, but it is an
escape hatch, not the path of least surprise.

## Leaving and finding memory

A page is where a session leaves what the next one should not have to
rederive. Pages and issues share one index — `items_fts` covers both kinds —
so retrieval is the search this file already teaches: the unified query under
*Queries that cover most questions* above, or `gadak search '<keyword>'`,
which returns issues and pages together.

The loop, three commands:

```bash
gadak page create --space LOC --title "retry backoff — findings" -m - <<'EOF'
Base 250ms, factor 2, cap 8s, jitter mandatory. Measured on the upload
path; the flat 1s retry lost large uploads.
EOF
# → 20001  retry backoff — findings   (stdout is the page id, then the title)

gadak comment STD-1 -m "Findings: /wiki/spaces/LOC/pages/20001 — backoff measured"
gadak search "backoff"        # next session: the issue and the page both return
```

The comment is what ties page to issue: `item_refs` is rebuilt from comment
text on every write, and a comment links a page **only when it carries the
page's URL** — `/wiki/spaces/<KEY>/pages/<ID>` or `pageId=<ID>`, the two
shapes `item_refs` recognizes (the linked page then shows under `ref_pages`
in `gadak issue KEY --json`). A bare page id in a comment does not link —
unlike a bare issue key, which does.

**Agent memory has its own verb.** When the intent is "leave it so the next
session finds it", `gadak memory add '<note>'` is the correct call: it
writes a page into the memory space through the same origin path as
`page create`, derives the title from the note's first line, and reports
`id → title → space`. `gadak memory search '<text>'` scopes the search to
that space alone. The space is the `memory.space` setting: **standalone**
defaults to the seeded `LOC`; **connected refuses until it is set**
(`gadak config set memory.space KEY`) rather than guess a team-visible
space — ask the user which space before the first add. To extend an
existing note instead of starting a new page, `gadak page edit <ID>
--append -m '<text>'` grafts paragraphs onto the current body and keeps
whatever formatting is already there.

**Leave a page when the finding outruns the issue**: an investigation whose
result a future session would otherwise redo, a decision together with its
why, a map of something the mirror cannot derive from its own rows. What the
issue itself already records belongs in a comment — and so does a one-line
fact; a page that would hold one sentence is a comment wearing a title.

Confirm-first reaches pages exactly as it reaches issue writes: on a
**connected** workspace a page is visible to the whole team — confirm before
creating or editing one, and use a space key the user names, never a guess.
On **standalone** the seed space is `LOC` and writes are free (see
*Standalone* above).

## Workspace settings

An agent configures the workspace through the CLI. Do not hand-edit
`~/.gadak/config.json` and do not drive the Settings dialog.

```bash
gadak config list                         # every editable path, current value, one-line description
gadak config list --json
gadak config get appearance.theme
gadak config set appearance.theme dark
gadak config set syncIntervalSec 30
gadak config set features.feed true
gadak config set projects '["NMB","NMA"]'
gadak config set devStatus true
```

`--json` on `list` and `get` (and `set`, which prints the stored value).
Unknown paths exit 64 and print the valid list. `set` accepts JSON or a
bare scalar (`dark`, `true`, `30`); arrays and objects need JSON.

Credentials (site, email, token) stay on `gadak init` — they are not
`config set` paths. `gadak config list` says so.

`appearance.theme` is `system` (the default, not persisted), `light`,
`dark`, or a lowercase palette id (`[a-z0-9-]{1,32}`). Palette names
belong to the web; the CLI only checks the shape.

## UI colors (`ui.*`)

The user's color overrides are three `config set` paths. Discover the valid
token names first — the catalog ships with the binary:

```bash
gadak config get ui.tokens.catalog --json   # name, cssVar, tier, rules, per-palette values
gadak config set ui.tokens '{"colors":{"accent":"#7a4bd0"}}'
gadak config set ui.tokensByTheme '{"dark":{"colors":{"accent":"#9a6be0"}}}'
gadak config set ui.dataColors '{"label":{"urgent":"#c03030"},"type":{"10007":"#d07020"},"status":{"inprogress":"#7e5904"}}'
```

- **Warnings mean applied: only parsing refuses.** A value that
  cannot parse (`"red"`, `"90"` for a length) or a wrong shape refuses;
  everything else — locked tiers, contrast/ΔEok/deuteranopia floors,
  dimension ranges and relations — **warns on stderr and saves** (exit 0,
  value echoed). Do not retry or work around a warning: the user's look is
  theirs. Do surface the warning text — it carries the measured number,
  the floor, and the fix (contrast warnings name the failing palettes and
  the `ui.tokensByTheme.<palette>` scoping fix; type-step warnings list the
  four-rung ladder that moves together).
- **Tiers:** `locked` tokens warn and save (palette authoring — the build
  may re-derive them in an upgrade); `validated` tokens are judged in every
  palette they render in; `free` tokens need hex only. Values are `#rgb` or
  `#rrggbb`, nothing else.
- **`dataColors` keys are ids, never display names**: `label.*` is the label
  text itself, `type.*` is the Jira issue type id (digits — names localize per
  account), `status.*` is the status category `new` / `inprogress` / `done`.
  A display name is refused with the correct key kind in the message.
- **Unknown token names warn and are carried**, not refused — a newer gadak's
  config still loads. Do not "clean" them out on sight.
- Changes reach an already-open web tab within ~1s (the ui-focus poll carries
  `configVersion`); no reload, no server restart.

## Dashboards (agent-authored walls)

You can author a live dashboard yourself: one HTML document plus named
queries, saved like a view and rendered full-tab in the running web UI.
Write the HTML to a temp file, register it with its datasources, open it —
an already-open tab follows within ~1s, and every later `save` under the
same name live-replaces the frame:

```bash
gadak dashboards save triage --html /tmp/triage.html \
  --datasource "by_status=sql:select status_category, count(*) as n from issues_full group by 1 order by 1" \
  --datasource "mine=jql:assignee = currentUser() AND resolution is EMPTY"
gadak dashboards open triage        # focuses the running web tab
gadak dashboards list / show / rm   # lifecycle; same name on save = update
```

- **Your HTML never fetches.** It runs sandboxed (opaque origin, CSP closes
  the network); the host executes each registered datasource and pushes rows
  in as `postMessage` events `{type:'data', name, columns, rows}`. Listen
  for those and paint. **`rows` are positional arrays, not objects** —
  `columns` is `["label","n"]` and `rows` is `[["api",12], …]`, so read
  `row[columns.indexOf('n')]`. Reaching for `row.n` is `undefined` in every
  cell and throws nothing; a wall of `undefined`/`NaN` over correct SQL is
  always this. You may send back exactly two verbs, both throttled:
  `{type:'refresh'}` (re-run datasources) and
  `{type:'open', hash:'#/?issue=GDK-1'}` — navigate the app itself to one
  of its own hashes (issue detail `#/?issue=KEY`, filtered list
  `#/?sc=inprogress`, search `#/?q=…`; recipe table in `docs/DASHBOARDS.md`).
  To link off-app, use `<a target="_blank" rel="noopener">` — it opens a
  new tab and leaves the wall in place; never a plain external `href`,
  which navigates the frame away.
- **SQL datasources are arbitrary read-only SELECTs** over `issues_full` —
  CTEs and window functions included. Key on computed columns, never display
  names: `status_category` / `priority_rank` / `issue_type_id`.
- **Charts: use the vendored libraries, not a CDN** (CSP refuses external
  hosts). `<script src="/api/v1/dashboards/vendor/uPlot.iife.min.js">` (+ its
  CSS) — leading slash required. The frame inherits no app styling: set your
  own explicit palette.
**A complete one. Copy this shape — there is no example file to go find.**
Everything the contract requires is here: the listener, the positional read,
and a paint. Nothing is elided.

```html
<!doctype html><meta charset="utf-8">
<style>
  :root { color-scheme: light dark }
  body { margin:0; padding:24px; font:14px/1.5 ui-sans-serif,system-ui,sans-serif;
         background:#12141a; color:#e8eaf0 }
  h1 { font-size:15px; font-weight:600; margin:0 0 16px; letter-spacing:.01em }
  .row { display:flex; align-items:center; gap:12px; margin:6px 0 }
  .name { width:180px; color:#a8aec0; overflow:hidden; text-overflow:ellipsis;
          white-space:nowrap }
  .bar { height:18px; background:#5b8cff; border-radius:3px; min-width:2px }
  .n { color:#a8aec0; font-variant-numeric:tabular-nums }
</style>
<h1 id="t">…</h1>
<div id="out"></div>
<script>
  addEventListener('message', (e) => {
    const m = e.data
    if (!m || m.type !== 'data' || m.name !== 'by_label') return
    // rows are POSITIONAL arrays keyed by columns — never row.n
    const iL = m.columns.indexOf('label'), iN = m.columns.indexOf('n')
    const rows = m.rows.map((r) => [String(r[iL]), Number(r[iN])])
    const max = Math.max(1, ...rows.map((r) => r[1]))
    document.getElementById('t').textContent =
      `${rows.length} labels · ${rows.reduce((a, r) => a + r[1], 0)} issues`
    document.getElementById('out').replaceChildren(...rows.map(([label, n]) => {
      const row = document.createElement('div'); row.className = 'row'
      const nm = document.createElement('div'); nm.className = 'name'; nm.textContent = label
      const bar = document.createElement('div'); bar.className = 'bar'
      bar.style.width = `${(n / max) * 320}px`
      const num = document.createElement('div'); num.className = 'n'; num.textContent = n
      row.append(nm, bar, num); return row
    }))
  })
</script>
```

```bash
gadak dashboards save label_ratio --html /tmp/label_ratio.html \
  --datasource "by_label=sql:select json_each.value as label, count(*) as n \
from issues_full, json_each(issues_full.labels) group by 1 order by n desc limit 20"
gadak dashboards open label_ratio
```

- For a uPlot line chart and the full `open` recipe table, `docs/DASHBOARDS.md`
  lives in gadak's source tree — it is *not* installed beside this file, so
  fetch `https://github.com/midagedev/gadak/blob/main/docs/DASHBOARDS.md` if
  you want it. Do not search the filesystem for it: there is nothing to find,
  and the block above is already a working wall.

## Rules that come with the file

- **Never write to the database.** Writes go through the origin (Jira on
  connected, the local origin on standalone, the home serve on paired);
  a row written directly is destroyed by the next sync. No exception for
  "just a label". Saved views and visits live in `local.db`, not the mirror.
- **Do not depend on `issues.raw`.** It is shaped by Jira's API, not by gadak's
  contract. Use the projected columns.
- **Do not poll in a loop.** `sync_state.version` moves only when something
  changed — compare it instead.
- **Read the freshness warning.** `issue`, `search`, `comment`, `transition`,
  `assign`, and `fields` print one line to stderr when the last sync failed or
  is over an hour old. stdout stays clean and pipeable.
- **Do not quote a restricted or JSM-internal comment in a public channel.**
  Filter `visibility_type != '' OR jsd_public = 0` first (`jsd_public` NULL
  means the marker was absent, not internal).
- **Before quoting an issue in a public place** (commit message, public
  summary, chat), check `security_level_id`. NULL means unrestricted, or a
  row the next sync has not rewritten. Key on the id, never on
  `security_level` (names localize).

## Development-panel links (`dev_links`)

`dev_links` is the projected development panel (pull-request URL, title,
status). `gadak issue KEY --json` includes it; SQL joins `dev_links` on
`item_id`.

- **standalone:** `gadak dev link KEY --pr <url>` records a PR through the
  local origin; `gadak dev scan` execs `gh pr list` and links matches
  (`cmd/gadak/dev.go`). Both refuse on a connected workspace.
- **connected Cloud:** do not run `dev link` / `dev scan`. `gadak config set
  devStatus true` to *mirror* Jira's development panel into `dev_links` (read).
- **paired** is kind `connected`: same refusal as Cloud. Write on the
  standalone home, then sync.

## When the mirror does not model it

Watchers, worklogs, user search, and anything else sync does not
project are reachable through the origin with `gadak api`. Sprints *are*
projected (`issues.sprint_id`, `sprint_name`, `sprint_state`); filter on
`sprint_id` or `sprint_state='active'`, never on `sprint_name`.

```bash
gadak api GET /rest/api/3/issue/NMB-140/watchers
```

Read-only unless `--write` is passed. Prefer the mirror when it can answer.
On a connected workspace this is a network round trip against the site's
rate budget. On standalone it talks to the local origin (no network);
unimplemented paths return 501.

## When gadak itself is the problem

You cannot see the window, so a path is your whole diagnostic surface. In
order:

- `gadak doctor` — a redacted summary of this install. Safe to paste into a
  report; it never contains a credential.
- `~/.gadak/logs/gadak.log` — the log file. It is local, never uploaded, and
  token values are stripped before anything is written; read the tail of it.
  `gadak doctor` prints the real path, which is what to trust if the home was
  moved.
- The mirror is a disposable cache, so `gadak sync --full` is a safe first
  move and losing the mirror costs nothing. Do that before anything drastic.
- `skill: stale` means this file and the binary disagree — `gadak skill
  install`.

## Report the friction; do not route around it

If gadak made you work harder than the task deserved — a column you had to
parse by hand, an identifier a read gave you that a write refused, an error
that named a problem but not a next move — that is worth reporting, and
reporting it is the expected move rather than a favour. The workaround you
were about to write destroys the evidence: once you have parsed it yourself,
nobody can see that the column was missing.

Report it at `https://github.com/midagedev/gadak/issues`. What makes a report
actionable here:

- the exact command you ran and the exact output you got
- `gadak --version`, and whether the workspace is connected, standalone or
  Linear
- what you expected instead — one sentence

A patch is welcome too, and is often smaller than the report suggests: clone
`https://github.com/midagedev/gadak`, and `AGENTS.md` at the root is the
development contract, including the gate commands to run before you open a
pull request.

## More

`gadak sql "select ..."` against `specs/000-product/data-model.md` covers every
column; `docs/RECIPES.md` in the gadak repository has more worked questions.
