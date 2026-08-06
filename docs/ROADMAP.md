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

Ordered by one rule (review, 2026-08): everything here either
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
- ✅ **Zero-install hosted demo** — promoted from v0.5. Static JSON snapshot +
  service worker on GitHub Pages (not sqlite-wasm — see ADR 0004 addendum): no
  binary, no account, no trust decision. `make hosted-demo` → `dist/hosted/`;
  `.github/workflows/pages.yml` deploys on main once Pages is enabled (human
  step, README).
- ✅ **Query recipes** — `docs/RECIPES.md`: 13 questions JQL cannot ask, each
  verified against the demo snapshot. Doubles as launch content.
- ✅ **Agent setup doc** — `docs/AGENT_SETUP.md`: paste-ready blocks for Claude
  Code / Cursor / Codex / MCP.
- ✅ **TUI parity.** Feed focus tabs (`1`–`4`: all / assignee / reporter /
  mention, each with its unread badge) and saved-view `display.sort` / `dir` /
  `group_by`. Priority sorts on `priority_rank`, never the localized name, so
  the same view orders the same way here and in the web UI. `relevance` and
  group keys outside the supported four stay reported as unsupported — the TUI
  has no text ranking, and faking a group is worse than saying so.
- ✅ **UX and quality debt.** Per-command `--help` with real usage and
  examples; `scry team` for sharing views and field mappings; favorites moved
  from browser storage into the mirror; the dead `presence` client stack
  removed; duplicate `initials` merged; storage keys renamed off the project's
  old name.

## Later, research-backed (see docs/PAIN_POINTS.md)

- ✅ **Rate-limit visibility.** The Jira client counts outbound attempts,
  429s, 5xx, retries, and backoff wait time; each sync cycle flushes into
  `api_usage` (daily UTC rows). `scry status` / `status --json` and
  `GET settings/` `runtime.apiUsage` expose today plus a 7-day rollup. This is
  **our process's call volume**, not Jira's remaining shared point budget —
  the site still does not expose that.
- ✅ **Field-bloat report** (`scry fields`). Not a single SQL query: the mirror
  only stores custom fields listed in `fieldMap`, so the command lists fields
  from Jira (`GET /field`) and probes a stratified sample of mirrored issue
  keys with `fields=*all`. Rates are sample-based, not a site census.
- ✅ **Custom-field auto-discovery** (v0.3.0). The first full sync fetches
  `*all`, groups the site catalog by display name (one concept, several ids —
  measured: 57 of 353 names on a large site), classifies role/editor from the
  schema, saves `config.fields`, and backfills from stored raw with no
  re-download. Detail rows, filter axes (per-project via `field_usage`), and
  the multi-select editor all derive from the specs; Settings → Fields edits
  are pinned across re-discovery. Projects became optional the same release:
  empty means every project the account can see.
- **Offline write queue.** Deliberately deferred: optimistic write-through
  already covers short offline windows, and conflict resolution is a product
  of its own. Wait for observed demand.

## v0.4 — workspaces (multi-account)

Profiles (`--profile`) already isolate credential + mirror per site and cover
the two-site case today. Promote them to a first-class switcher:

- ✅ **One process, many mirrors** — settled and shipped: `scry serve` mounts
  every sibling profile under `/w/<name>/` (full API, reads and write-through),
  opened lazily on first request. The trust boundary is unchanged — same
  loopback listener, same single OS user; the workspace list endpoint carries
  site + projects only, never credentials (test-enforced). Background sync and
  the update check stay on the primary; workspaces sync on demand.
- ✅ **Web: workspace picker in the sidebar** — served from
  `GET /api/v1/workspaces`; the SPA detects its mount from the URL, fetches the
  per-workspace `config.json` (prefixed API bases), and keys IndexedDB and
  localStorage per workspace so two mirrors on one origin never share cache or
  favorites. Push notifications stay a primary-page feature.
- ✅ TUI/CLI: `scry --profile` stays; `scry profiles` lists them (predates this
  wave; `scry workspaces` as a separate verb was judged a duplicate).

Sequenced after v0.3 deliberately: retention loops for existing mirrors come
before conveniences for hypothetical second sites.

- **JQL → SQL bridge** also waits here: it removes a migration tax for
  arriving users, so it needs arriving users — and real JQL corpora to test
  against, or a half-working translator fails in trust-destroying ways.

## v0.6 — the second source, and structure

Confluence, as the proof that the neutral layer is actually neutral — plus the
hierarchy layer the data was missing.

- ✅ **Confluence connector** against the same `items` spine and the same FTS
  path (decision 0006). Same site credential, CQL incremental with a
  comments-only pass (comments don't bump page version), ADF bodies stored raw
  (`pages.body_adf`, v10).
- ✅ **Unified search across issues and pages** — one FTS index, one query;
  `search` returns `keys` + `pages`. CLI/MCP parity.
- ✅ **Docs in the UI** — sidebar DOCS tree (`parent_id` recursion),
  DocumentPanel with a clickable breadcrumb (`space › ancestors › title`),
  documents group in unified search. (Shipped in v0.6.0; the tree-first
  sidebar was later reworked toward recency-first views —
  `docs/UX_PRINCIPLES.md` §6.)
- ✅ **Epic hierarchy** — `epic_key` derived honestly on the write path
  (nearest hierarchy-level-1 ancestor via `parent_key`, schema v11), epic
  group headers with real summaries, epic chips, parent breadcrumb, children
  progress, and TUI `group_by=epic` support. (Shipped in v0.6.0.)
- **Confluence labels** as a pageLite filter axis (deferred from the connector
  round on purpose).
- **Demo snapshot refresh**: epics + page bodies, re-scrubbed
  (`scripts/scrub-demo-db.py` + `scan-internal.sh` before commit, as always).

This wave was deliberately sequenced after v0.3–v0.4: shipping the second
source earlier would have meant designing the neutral layer against one
imagined consumer instead of one real one.

## v0.7 — freshness, speed, and the agent wedge

The organizing ideas of this wave: the product is "an agent-aware memory of
your team's work" and the fast mirror is the wedge; the sync engine is the
product, speed is a feature, and trust in freshness is built in the UI.

- **Freshness as a feeling.** `sync-on-focus` (web regains focus + mirror older
  than N → sync now) and a visible freshness chip ("synced 12s ago") in the
  header. No webhooks, no server — the loopback model stays.
  ~~A cheap head-check between polls (one `updated >= -5m` count query gates
  whether a delta pull runs at all).~~ Dropped 2026-08-06 on measurement, and
  the original is kept because the argument comes back the moment the interval
  drops: the watch loop already runs at 60s (`DefaultSyncIntervalSec`, floor
  15s) and an idle incremental costs one or two Jira calls, so the gate query
  would spend about what it saves.
- **Performance budgets as gates.** Interaction budgets measured in e2e against
  a 10k-issue fixture (`tools/bench-fixture`), enforced in CI: cold boot →
  interactive, keystroke → search results, palette open, panel switch. Budgets
  pinned from real measurements first (FAIL-first), not aspirations.
- **Keyboard triage flow.** The palette exists; the flow doesn't. `j/k` move,
  `x` select, `s` status, `a` assign, `c` comment on the list itself — a sprint
  cleanup should never need the mouse. Every action also registered in the
  palette.
- **Agent wedge, front and center.** README leads with the agent story
  (the mirror is the substrate); one-command MCP setup per client
  (`claude mcp add` paste-blocks already exist in docs/AGENT_SETUP.md — promote
  to a `scry mcp install <client>` verb); a 90-second demo of an agent
  answering with issue+doc context no cloud API could assemble as fast.
  Launch content (Show HN) rides this, not the "fast Jira client" frame.
- **Offline write queue — revisit, don't build yet.** The v0.3-era deferral
  ("wait for observed demand") stands, but the 2026-08-06 review added a new
  argument: agents writing through MCP would benefit from a durable outbox
  more than humans do. Recorded here so the next demand signal reopens
  it with both arguments on the table.

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
