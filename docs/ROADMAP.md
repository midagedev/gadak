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

## v0.3 — retention and reach

Ordered by one rule (post-mentor-review, 2026-08): everything here either
keeps an installed mirror alive or removes a reason not to try one.

- ✅ **Close the notification loop.** The watch feed is computed (`GET feed/`);
  OS notifications fire from the sync loop (macOS `osascript`, Linux
  `notify-send`) for new personal-feed events. Config: `notify` (default true).
- ✅ **`scry install-service`.** launchd plist / systemd user unit so the mirror
  survives reboot (`serve --no-open`). `--uninstall` removes it.
- ✅ **`serve` syncs by default** when a credential is configured (`--no-sync`
  opts out). A stale mirror is the fastest way to lose a habit.
- ✅ **Retention counters + `issues_full`.** `sync_state.first_sync_at` /
  `sync_count` on successful syncs; SQL view gives agents a title without a join.
- **Zero-install hosted demo** — promoted from v0.5. `sqlite-wasm` over the
  demo snapshot on static hosting: no binary, no account, no trust decision.
  The strongest adoption lever available (`decisions/0004-browser-sqlite.md`).
- **Query recipes.** The questions JQL cannot ask, as named, documented
  queries: stalled N days, reopened, version ranges, comment-history search.
  Doubles as launch content.
- **Agent setup doc** — paste-ready blocks for Claude Code / Cursor / Codex.
  The 2026 distribution channel is the developer's agent, not their browser.
- TUI keeps parity with new surfaces (feed focus tabs, saved-view sort) and
  the remaining UX-audit P2 debt lands here.

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

Profiles (`--profile`) already isolate credential + mirror per site and cover
the two-site case today. Promote them to a first-class switcher:

- Web: workspace picker in the sidebar (server enumerates profiles; switch
  restarts the serve target or proxies per-profile handlers).
- TUI/CLI: `scry --profile` stays; add `scry workspaces` to list.
- One process, many mirrors is the design question to settle first — see the
  security model (loopback, one user) before letting one server open several
  credentials.

Sequenced after v0.3 deliberately: retention loops for existing mirrors come
before conveniences for hypothetical second sites.

- **JQL → SQL bridge** also waits here: it removes a migration tax for
  arriving users, so it needs arriving users — and real JQL corpora to test
  against, or a half-working translator fails in trust-destroying ways.

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
