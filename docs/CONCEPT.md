# Concept

## One line

scry mirrors your issue tracker to local SQLite, then serves it to a browser UI
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
such guess is wrong at the edges.

**Writes cannot be local.** Jira is the record. Writes call Jira and refresh the
affected rows. No queue, no offline write model, no reconciliation — those exist
to serve a different product than this one.

**The mirror is disposable.** If the schema is a cache of someone else's truth,
deleting it is always safe. That property is worth protecting, because the moment
scry holds something irreplaceable it becomes infrastructure someone has to back
up.

## The loop it optimizes

1. Open the app; it paints from cache before any request finishes.
2. Type; the list narrows as you type, over everything you have access to.
3. Open an issue; description, comments, history, and links are already local.
4. Act — transition, comment, assign — and the write goes to Jira immediately.
5. Later, ask your agent a question that spans the whole backlog, and it answers
   with one query instead of forty API calls.

Steps 1 through 4 are what the extracted application already does in daily use.
Step 5 is what makes it worth publishing.

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

Not a Jira replacement. Not a sync engine. Not an archive. Not multi-user. Not a
place to put anything you cannot afford to lose.
