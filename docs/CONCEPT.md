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

That is the whole idea. Everything else is consequence.

## What follows from it

**Filtering must never touch the network.** Once data is local, latency is a bug,
not a fact of life. This constrains the architecture more than any other rule.

**The database is the agent API.** Given a documented SQLite schema, an agent
needs no tool definitions and no endpoint list. It writes a query. Designing a
REST or MCP surface instead would mean guessing which questions matter, and every
such guess is wrong at the edges. SQL is how the agent answers; `gadak views
open` is how it presents the same work in the window.

**Writes cannot be local.** Jira is the record. Writes call Jira and refresh the
affected rows. No queue, no offline write model, no reconciliation — those exist
to serve a different product than this one.

**The mirror is disposable.** If the schema is a cache of someone else's truth,
deleting it is always safe. That property is worth protecting, because the moment
gadak holds something irreplaceable it becomes infrastructure someone has to back
up.

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
4. Act — transition, comment, assign — and the write goes to Jira immediately.
5. Later, ask your agent a question that spans the whole backlog, and it answers
   with one query instead of forty API calls.
6. When the answer is something you should look at, the agent points the window
   (`gadak views open`) instead of pasting a table.

Steps 1 through 4 are what the extracted application already does in daily use.
Steps 5 and 6 are what make it worth publishing: SQL answers; views present.

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
refuses to model. Not a sync engine. Not an archive. Not multi-user. Not a
place to put anything you cannot afford to lose.
