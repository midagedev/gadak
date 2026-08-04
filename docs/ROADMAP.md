# Roadmap

`../specs/000-product/tasks.md` holds the task-level state. This is the ordering
and the reasoning.

## v0.1 — the mirror works

Goal: someone can install this, point it at their Jira, and stop waiting on
search.

1. Store: schema, migrations, WAL, FTS5.
2. Jira connector: full sync, incremental sync, changelog and comment paging,
   derived fields.
3. Read API from SQLite: `bootstrap`, `delta`, `detail`, `search`, attachment
   proxy.
4. Write-through: credentials, transitions, comments, attachments, assignee,
   field edits, creation.
5. `scry sql` and `scry status --json` for agents.
6. Demo snapshot and `scry demo`.

~~**Release blocker: the UI is Korean-only.**~~ Done — English-first with Korean
kept as a locale.

## v0.2 — reach and polish

Versioning policy from here: one minor per meaningful improvement wave. No
number-jumping; each minor is earned by shipped, verified work.

- ✅ **MCP server** for agents without shell access.
- ✅ **Local watch feed.** Notify on changes to watched issues, computed from the
  mirror (changelog + comments) rather than a server-side event stream. Replaces
  the cut internal feed. In-tab browser notifications; web push deliberately not.
- ✅ **Plugin boundary** for the leftover internal surfaces (deploy/QA/PR arrive
  via `enrichments`).
- ✅ **Packaging**: Homebrew tap, install script, container image, `embed.FS` so
  the binary carries its own UI. (Signing: later, when there is a user to ask.)
- Moved out to keep v0.2 shippable: the zero-install hosted demo (below).

## v0.3 — TUI parity and daily-driver polish

- **TUI catches up to the web UI**: feed, saved views, watch toggle, in-app help.
  From here, new surfaces land on web and TUI together.
- UX debt from the pre-launch audit (P2 tier): comment drafts, offline badge,
  keyboard write actions.
- **JQL → SQL bridge.** Paste a JQL query, get the roughly equivalent SQL.
  Common clauses only (`project`, `status`, `assignee`, relative dates,
  `ORDER BY`); anything else is honestly marked untranslatable. Users arrive
  with JQL assets; this removes the migration tax (`docs/PAIN_POINTS.md` §6).
- **Query recipes.** Ship the questions JQL cannot ask as named, documented
  queries: stalled N days, reopened, version ranges, comment-history search.
  Demonstrates SQL > JQL instead of asserting it.

## Later, research-backed (see docs/PAIN_POINTS.md)

- **Rate-limit visibility.** Jira's shared 65k-point pool has no user-facing
  dashboard and 429s are invisible until they hit. The client already honors
  `Retry-After` with backoff; counting and showing our own call volume would be
  a dashboard Jira does not offer.
- **Field-bloat report.** "Which custom fields are actually used" is one SQL
  query over a mirror and impossible to see in Jira itself.
- **Offline write queue.** Deliberately deferred: optimistic write-through
  already covers short offline windows, and conflict resolution is a product
  of its own. Wait for observed demand.

## v0.4 — workspaces (multi-account)

Profiles (`--profile`) already isolate credential + mirror per site. Promote
them to a first-class workspace switcher:

- Web: workspace picker in the sidebar (server enumerates profiles; switch
  restarts the serve target or proxies per-profile handlers).
- TUI/CLI: `scry --profile` stays; add `scry workspaces` to list.
- One process, many mirrors is the design question to settle first — see the
  security model (loopback, one user) before letting one server open several
  credentials.

## v0.5 — zero-install hosted demo

`sqlite-wasm` reading a static snapshot over HTTP range requests, published on
static hosting. No binary, no account, no trust decision — the strongest
adoption lever available (`decisions/0004-browser-sqlite.md`).

## Later — the second source

Confluence, as the proof that the neutral layer is actually neutral.

- Confluence connector against the same `items` spine and the same FTS path.
- Unified search across issues and pages, since the questions people ask ("what
  do we know about X") do not respect the boundary.
- A generic detail view for document-shaped items.

This is deliberately last. Shipping it earlier would mean designing the neutral
layer against one imagined consumer instead of one real one.

## Considered and not planned

- **Bidirectional sync engines** (PowerSync, Electric, Zero). Wrong shape: they
  assume a remote server with a Postgres upstream, and writes here cannot leave
  the browser directly anyway. See `decisions/0004-browser-sqlite.md`.
- **Jira Server / Data Center.** Not until someone with an instance can test it.
  Guessing at DC behavior is worse than declining to support it.
- **Multi-user or hosted deployment.** Contradicts the security model, which is
  "one user, loopback only, no auth".
- **Boards, sprints, reports.** Jira's own UI does these, and a partial
  reimplementation is worse than a link.
- **Writing to the mirror.** Jira is the record. Any local write model would need
  conflict resolution, which is a different product.
