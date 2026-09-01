# Concept

## One line

gadak mirrors your issue tracker to local SQLite, then serves it to a browser UI
and to your coding agent from the same file.

## The insight

Two complaints look unrelated:

- "Jira is slow to search."
- "My agent is bad at reasoning about our tickets."

They have one cause: the data lives only behind an API. Fix the location and both
improve at once. A local mirror makes filtering a memory operation and makes the
history queryable with SQL.

The wiki next door is the same problem twice. Half of what an agent needs is not
in the tracker; "what do we know about X?" is split across Jira search and
Confluence search, which do not talk to each other. One file holds both: one FTS
index, joins across sources, no token spent on pagination.

That is the whole idea. Everything else is consequence.

## What follows from it

**Filtering must never touch the network.** Once data is local, latency is a bug,
not a fact of life. This constrains the architecture more than any other rule.

**The database is the agent API.** Given a documented SQLite schema, an agent
needs no tool definitions and no endpoint list. It writes a query. Designing a
REST or MCP surface instead would mean guessing which questions matter, and every
such guess is wrong at the edges. SQL is how the agent answers; `gadak views
open` is how it presents the same work in the window.

**Writes go through the origin.** The mirror is a cache. The origin is the
record. Writes call that origin and refresh the affected rows. No queue, no
offline write model, no reconciliation — those exist to serve a different
product than this one.

**The mirror is disposable.** If the schema is a cache of the origin's truth,
deleting it is always safe. That property is worth protecting: the moment the
mirror holds something irreplaceable it becomes infrastructure someone has to
back up. On a gadak origin, the irreplaceable file is not `gadak.db` — it is the
origin persist file (below).

## Two origins

A workspace is bound to one origin.

- **Connected** — Atlassian Cloud (or Linear, when configured) is the record.
  The token talks to that site. Delete the profile directory and you have lost
  a cache.
- **A gadak origin** (from 0.16) — the origin is an in-process minimal Jira
  (`issuetap`). There is no Atlassian account. The durable file is
  `origin/issuetap.db` in the profile directory (`internal/origin/origin.go`
  `PersistRel`): a SQLite database (WAL). That is the file to back up —
  copy it while gadak is not running (include the `-wal`/`-shm` sidecars),
  or `sqlite3 origin/issuetap.db ".backup dest.db"`. A sibling
  `origin/issuetap.yaml`, if present, is a one-shot seed, not the record.
  `gadak.db` is still a cache; the next sync rebuilds it from the persist
  file.

## The browser it replaces

gadak is the dedicated browser for your issue tracker. Stop piling Jira tabs in
Chrome; this window is where Jira lives on the machine. What the mirror models —
lists, detail, search — is answered natively, instantly. What it deliberately
does not model — boards, dashboards, arbitrary pages — is contained: an in-app
tab on desktop, the system browser under `serve`. Reimplement nothing; contain
everything. The window has a second user: your coding agent points at it
(`gadak views open`) instead of pasting a markdown table, so you and the agent
look at the same work.

That is the same sentence as "not a Jira replacement" below, not a contradiction
of it. We do not rebuild Jira. We contain the pages we refuse to model, so the
window can hold them without becoming a second Jira.

What the product *is*, in order:

1. **The mirror is the body.** Speed, offline, SQL — the moat. Unchanged since
   the insight above.
2. **The browser feel is the packaging.** It is how the value is pitched
   ("stop having fifteen Jira tabs"), never a feature list of its own. The
   pitch word is "where your Jira lives", not "browser" — that word buys
   expectations an embedded WebView cannot honor.
3. **The agent handoff is the differentiator.** Agent points, human sees.

## The loop it optimizes

1. Open the app; it paints from cache before any request finishes.
2. Type; the list narrows as you type, over everything you have access to.
3. Open an issue; description, comments, history, and links are already local.
4. Act — transition, comment, assign — and the write goes to the origin immediately.
5. Later, ask your agent a question that spans the whole backlog, and it answers
   with one query instead of forty API calls.
6. When the answer is something you should look at, the agent points the window
   (`gadak views open`) instead of pasting a table.

Steps 1 through 4 are what the extracted application already does in daily use.
Steps 5 and 6 are what make it worth publishing: SQL answers; views present.

## Two surfaces

| | For | Looks like |
| --- | --- | --- |
| **App + Web UI** | all-day triage | the [desktop app](DESKTOP.md) — no port, no local server — or the same UI in a browser tab (`gadak serve`) |
| **CLI + SQL** | agents, scripts, one-off questions | `gadak issue`, `gadak search` (FTS, or `--jql` / a Jira URL), `gadak sql`, plus the file itself |

`j`/`k` walk the list, `x` selects, `s`/`a`/`l`/`c` change status, assignee,
labels, or leave a comment without leaving the list. Click an issue to rename
it, change priority, or edit labels. Paste a Jira filter URL or JQL into the
list box and the chips apply; **Copy JQL** is the way back
([decision 0007](decisions/0007-jql-subset.md)). Sync also pulls the filters
you own or have starred into the sidebar.

**Search ⌘K** in the toolbar queries titles, bodies, and comments across every
issue and every document, ignoring the chips on the list, and labels the field
that matched with a snippet. The box above the list only narrows this view.

Documents are first-class: recency lists, deep links (`?doc=`), and
cross-references both ways. Modeled issue and wiki links open the native
panel; the key in the header is the way out to Jira. Anything the mirror does
not model — a board, a workflow screen, a Confluence draft — opens in the
app's in-app tab (a system tab under `serve`); close it and the mirror
re-reads.

Writes go through the origin and then refresh the mirror. Comment, transition,
assign, labels, priority, and the title work from the app and the web UI; the
CLI covers those plus `create`, `attach`, and `edit` (values always come from
what the origin allows, never free text — an issue type or priority the CLI
cannot match is refused with the names that origin actually uses).
Wiki page create, edit, and comment go through the origin too — Confluence
Cloud on a connected workspace, the in-process origin on a gadak origin.

Hierarchy is first-class: `epic_key` is derived honestly (the nearest epic
*ancestor*, so a sub-task groups under its epic, not its story), group-by-epic
headers show the epic's actual title, an epic's detail rolls up its children
(`12 done / 14`), and both breadcrumbs — issue and document — are clickable.
"Which issues came back after we closed them, and why?" is `where reopen_count
> 0`. "Which epic is actually stuck?" is a group-by on `epic_key`. In Jira
neither is a question you can ask.

Jira and Confluence never tell each other what mentions what, but the text
does: gadak extracts issue keys from page bodies and wiki links from issue
text into an `item_refs` table while it syncs. That is why an issue can list
the design docs that cite it and a page can list the tickets it references.

Attachments are local too. The first view of an image caches its bytes next to
the mirror and every later view is a disk read, so a screenshot-heavy issue
opens at the speed of the rest of the app — and keeps rendering offline.

## Why not the alternatives

**A faster Jira client.** Any client that calls Jira per interaction inherits
Jira's latency. The only fix is to stop calling per interaction.

**A browser extension.** Genuinely lighter to install, and it reuses the existing
session. But it cannot hand an agent a queryable database, which is half the
value. Worth building as a companion, not as the product.

**An MCP server over the Jira API.** This is the common shape, and it inherits
every limitation of the API: pagination, rate limits, no joins, no aggregation, no
offline. An MCP server over a *local mirror* is a thin wrapper worth adding; an
MCP server over the network is a worse product.

**A hosted service.** Would need to hold everyone's issue data, which is both a
security liability and an approval barrier at exactly the companies that feel this
pain hardest. Local means nothing to approve.

## What it is not

Not a Jira replacement — we do not reimplement boards, dashboards, or the rest
of Jira's UI; we contain those pages so the window can hold what the mirror
refuses to model. Not a sync engine. Not an archive. Not multi-user. On a
connected workspace, not a place to put anything you cannot afford to lose
(the Atlassian site holds the record). On a gadak origin, `origin/issuetap.db`
*is* the record — losing that file loses the work.

## Good fit / bad fit

| Use gadak when… | Use Jira/Confluence directly when… |
| --- | --- |
| You search and triage the same projects every day and the latency hurts. | You need boards, sprints, reports, automation, permissions. |
| You want an agent to reason over your tracker's history *and* your wiki. | You need administration, workflow editing, or document authoring. |
| You want offline reading of everything you have access to. | A minute of staleness matters. |
| Your tracker holds tens of thousands of issues and Jira's UI struggles. | Your team is small enough that Jira already feels instant. |

**In scope:** issue fields, descriptions, comments, attachments, changelog,
links, epic hierarchy, status transitions, assignee, labels, priority, title,
wiki pages (bodies, comments, labels), full-text search across all of it,
saved views, watches; field edits and issue creation in the app and the web UI.
**Out of scope:** boards and sprint mechanics, project administration, workflow
configuration, permission schemes, a page editor in the UI (CLI and REST write
pages), grouping the list by label (filter the chips instead). Those stay in
Jira and Confluence; the macOS app contains them in the same window.
