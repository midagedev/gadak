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
5. `gadak sql` and `gadak status --json` for agents.
6. Demo snapshot and `gadak demo`.

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
- ✅ **`gadak install-service`.** launchd plist / systemd user unit so the mirror
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
  verified against the demo snapshot.
- ✅ **Agent setup doc** — `docs/AGENT_SETUP.md`: paste-ready blocks for Claude
  Code / Cursor / Codex / MCP.
- ✅ **TUI parity (retired 2026-08-13).** Shipped, then dropped — a second
  human surface split attention. See `docs/decisions/0005-three-surfaces.md`.
- ✅ **UX and quality debt.** Per-command `--help` with real usage and
  examples; `gadak team` for sharing views and field mappings; favorites moved
  from browser storage into the mirror; the dead `presence` client stack
  removed; duplicate `initials` merged; storage keys renamed off the project's
  old name.

## Later, research-backed (see docs/PAIN_POINTS.md)

- ✅ **Rate-limit visibility.** The Jira client counts outbound attempts,
  429s, 5xx, retries, and backoff wait time; each sync cycle flushes into
  `api_usage` (daily UTC rows). `gadak status` / `status --json` and
  `GET settings/` `runtime.apiUsage` expose today plus a 7-day rollup. This is
  **our process's call volume**, not Jira's remaining shared point budget —
  the site still does not expose that.
- ✅ **Field-bloat report** (`gadak fields`). Not a single SQL query: the mirror
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

- ✅ **One process, many mirrors** — settled and shipped: `gadak serve` mounts
  every sibling profile under `/w/<name>/` (full API, reads and write-through),
  opened lazily on first request. The trust boundary is unchanged — same
  loopback listener, same single OS user; the workspace list endpoint carries
  site + projects only, never credentials (test-enforced). HTTP mounts are
  lazy; every credentialed profile gets a watch loop at boot. The update
  check stays on the primary.
- ✅ **Web: workspace picker in the sidebar** — served from
  `GET /api/v1/workspaces`; the SPA detects its mount from the URL, fetches the
  per-workspace `config.json` (prefixed API bases), and keys IndexedDB and
  localStorage per workspace so two mirrors on one origin never share cache or
  favorites. Push notifications stay a primary-page feature.
- ✅ TUI/CLI: `gadak --profile` stays; `gadak profiles` lists them (predates this
  wave; `gadak workspaces` as a separate verb was judged a duplicate).

Sequenced after v0.3 deliberately: retention loops for existing mirrors come
before conveniences for hypothetical second sites.

- ✅ **JQL ↔ filter** (2026-08-14) — not a SQL bridge. A documented subset
  maps to the in-memory ViewFilters the UI already applies, both directions,
  on the search box and `gadak search --jql`. Unsupported clauses are listed.
  Sync imports owned/starred Jira filters into `source_queries` (sidebar +
  `gadak views`). `gadak views open` focuses the running app or serve tab.
  A JQL → SQL compiler for questions the filter cannot express is still
  waiting on a real corpus; until then `gadak sql` is the way to ask them.

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

- ✅ **Freshness as a feeling.** `sync-on-focus` (web regains focus + mirror older
  than N → incremental pull; `issues.svelte.ts`) and a visible freshness chip
  (`FreshnessChip` in the list header). No webhooks, no server — the loopback
  model stays.
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
- ✅ **Keyboard triage flow.** `j/k` move, `x` select, `s` status, `a` assign,
  `l` labels, `c` comment on the list (`web/src/App.svelte`,
  `stores/triage.svelte.ts`, `e2e/triage.spec.ts`). Every action also
  registered in the palette.
- **Agent wedge, front and center.** README leads with the agent story
  (the mirror is the substrate); `gadak mcp install <client>` shipped
  (`docs/MCP.md`); a 90-second demo of an agent
  answering with issue+doc context no cloud API could assemble as fast.
- **Offline write queue — revisit, don't build yet.** The v0.3-era deferral
  ("wait for observed demand") stands, but the 2026-08-06 review added a new
  argument: agents writing through MCP would benefit from a durable outbox
  more than humans do. Recorded here so the next demand signal reopens
  it with both arguments on the table.

## v0.8 — the desktop app

The port and the terminal were the last pieces of accidental UX between a
person and the mirror. Shipped 2026-08-06:

- ✅ **Gadak.app** — the web UI in a native macOS window with no local server
  at all (in-process handler; no port, no conflicts, single-instance).
  Signed + notarized `Gadak-<ver>-arm64.dmg` attached to every release.
- ✅ **First run in the window.** The existing in-app onboarding carries the
  desktop case; finishing setup now also starts the background sync without
  a restart (previously boot-time only).
- ✅ **The CLI rides inside the bundle** (`Contents/Resources/bin/gadak`), so
  a desktop-only install still graduates to agent use — one symlink, then
  `gadak mcp install claude`. See `docs/DESKTOP.md`.

Not in this wave: workspace switching in the app (one profile per window),
Intel/universal builds, Windows/Linux shells.

## v0.9 — people, evidence, and a real grid

The mirror had answers it could not show, and a surface that undersold them.
Shipped 2026-08-06:

- ✅ **The people axis.** A person is a thing you can open: their comments
  across issues and pages, plus assigned/reported/authored as one click each.
  Web-only this version; agents reach the same
  axis through `gadak_query` — see the recipe in `docs/RECIPES.md`.
- ✅ **Search says why it matched**, everywhere the API reaches (CLI, MCP, web).
  Comment search always worked; now it looks like it.
- ✅ **Page excerpts (schema v15)** on activity lists, web and TUI.
- ✅ **A visual foundation, then its geometry.** A type scale, accent
  discipline, one icon family; then a two-step control-height grid, radius by
  nesting depth, panel headers pinned by structure, and grouped comments.
- ✅ **The bundled demo has four people in it**, so the people axis is
  explorable before you connect anything.

## v0.10 — the two sources finally meet

The wiki mirror existed but lived like a guest: reachable, searchable in
principle, connected to nothing. This wave came out of using it for real on a
work mirror. Shipped 2026-08-07:

- ✅ **Search reaches every screen.** The ⌘K palette matches page titles
  above the issues; `/` focuses whichever narrowing field
  the screen in front of you has; the document screens gained a local filter
  whose Enter hands the query to the one unified results surface.
- ✅ **Document screens carry their own weight.** Deep links (`?doc=`,
  `?space=`) that restore the sidebar and background identically to a
  click-through; label chips that narrow on click; every filtered row marks
  what kept it; the tree separates answers from the path to them.
- ✅ **Cross-references (schema v16).** Page bodies mention issue keys and
  issue text mentions wiki pages; `item_refs` derives both directions at sync
  and backfills on migration. Issue detail lists its documents, page detail
  lists its issues — web and TUI in the same version.
- ✅ **A reading aid nobody asks for until they drag-read a long doc**: the
  block under the cursor lifts, in issue descriptions, page bodies, and
  comments alike.
- ✅ **`gadak snapshot` carries documents** — a shared mirror no longer arrives
  with an empty DOCS section.
- ✅ **A sixth perf axis** (`docsFilterKeystrokeMs`, pinned FAIL-first on the
  10k fixture) so the document filter can never quietly leave memory.

## v0.13 — the dedicated browser, and the agent that drives it

Owner decision, 2026-08-14 (`specs/001-dedicated-browser/spec.md`). The window
is where Jira lives on the machine. What the mirror models is answered
natively; what it does not is contained, not reimplemented. The window has a
second user: the coding agent points at it (`gadak views open`) instead of
pasting a table. SQL answers; views present.

Hierarchy, in order of what the product is: the mirror is the body; the
browser feel is the packaging; the agent handoff is the differentiator. A
feature that grows the shell but not the mirror or the handoff is
default-rejected. The in-app pane is an escape hatch, not a floor — tabs, a
rectangle, and post-close resync, nothing else. In-app tabs are
session-scoped; retrieval stays the mirror's job (`docs/UX_PRINCIPLES.md`
§11–§13).

The contained-browser half exists only in Gadak.app (WKWebView; Atlassian
forbids iframes). `gadak serve` users get native surfaces plus system tabs.
`gadak open` stays the Jira escape hatch (system browser to `/browse/KEY`);
`gadak views open` is the "open in gadak" verb.

Work in this wave (see `specs/001-dedicated-browser/tasks.md`): a `keys`
filter axis so an agent can show a result set; paste and body-link routing
so modeled URLs open native; `views open` can focus one issue, take a key
list, and print the link; retrieval finishes the anti-tab thesis (recents,
palette filters); agent docs teach showing, not only answering.

## Next — arrival started; questions still run in parallel

The 2026-08-07 stance was no feature waves until a user showed up. That
stance is amended, not dropped. PR #1 (external user; person filters keyed
on Jira account ids; opened 2026-08-10, merged 2026-08-14) is the first
arrival signal. v0.13 is aimed at the two audiences that arrival proved
exist — agent-driven use and daily in-app living.

Collecting questions and watching an install continue in parallel; they are
not displaced by this wave:

1. **Collect questions, not installs.** Ask people who live in Jira for one
   question they want to ask it and cannot, and answer it with SQL against a
   mirror that already exists. Demand for the answer has to show up before the
   binary is worth anyone's evening.
2. **Watch someone install it without help**, and write down where they stop.
3. **Prepare for arrival rather than polish for absence** — `gadak doctor`,
   [`MAINTENANCE.md`](../MAINTENANCE.md), and the narrowed schema contract in
   `specs/000-product/data-model.md` all exist so that a next real user costs
   hours instead of weeks.

Deliberately **not** now: Show HN (a card that can be played once, and not
before the install friction is known), a 1.0 (0.x is the accurate label and
the better shield), new sources, new surfaces, and UI polish rounds. The
type-scale sweep that used to sit here was the clearest example of the trap
— a hundred-odd class literals to tidy, for nobody.

### Held, with the bar written down

- **Desktop notifications** — the watch feed has the events; a native
  notification is the natural surface. Quiet by default: this is a mirror,
  not another thing that interrupts you. Worth doing once someone other than
  the author leaves the app running all day.
- **Windows and Linux shells** — **not until 10 people have asked.** Wails
  builds all three and the no-listener architecture ports unchanged, so this
  is not a technical wall; it is WebView2 bootstrap and an installer on
  Windows (plus a code-signing decision, or SmartScreen greets every
  download), webkit2gtk and AppImage/.deb on Linux, and splitting the
  macOS-only menu code — three platforms of packaging and bug surface for a
  maintainer whose macOS build has not yet carried a real user. Linux first
  when the bar is met; it overlaps the agent audience most.
- **Jira-less workspace (a local-only source).** The appeal is real — no
  token, no approval, an agent-readable local tracker; the demo already
  proves the read surface without a live site. But every write path gadak
  has goes *through* Jira (the app creates issues against Jira, never
  locally), and the moment
  gadak holds the only copy, "delete a directory and have lost nothing"
  stops being true — a durability promise a 0.x schema should not make.
  Note the "Writing to the mirror" objection below is about conflict
  resolution against a synced source; a source-less workspace has no sync,
  so that argument does not close this one — these three bars do. Revisit
  when **all three** hold: (1) `gadak export`/`import` and the pipe/schema
  promises have shipped, (2) five independent asks naming a concrete use,
  (3) a local write-model design note answers workflow, durability, and
  backup.

## More sources later

Confluence was the proof: the second connector merged against the same spine,
the same FTS index, and the same read contracts without reshaping the database
(decision 0006). The pattern — mirror, project, index — is what the next
source rides too. Candidates are ranked by user demand, not by roadmap
romance; the ordered work above is what is actually next. New sources are
deliberately not now (see the arrival stance in **Next**).

## Considered and not planned

- **Bidirectional sync engines** (PowerSync, Electric, Zero). Wrong shape: they
  assume a remote server with a Postgres upstream, and writes here cannot leave
  the browser directly anyway. See `decisions/0004-browser-sqlite.md`.
- **Jira Server / Data Center.** Not until someone with an instance can test it.
  Guessing at DC behavior is worse than declining to support it.
- **Multi-user or hosted deployment.** Contradicts the security model, which is
  "one user, loopback only, no auth".
- **Boards, sprints, reports.** Jira's own UI does these. We contain that
  page (in-app tab on desktop, system browser under `serve`); we do not
  reimplement it.
- **Writing to the mirror.** Jira is the record. Any local write model would need
  conflict resolution, which is a different product.
