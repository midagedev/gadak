# Changelog

## Unreleased

- **Page list excerpt (schema v15).** `pages.excerpt` — a one-line body preview
  (≤200 runes from ADF plain text) on every `PageLite` for document list UIs;
  backfilled from existing `body_adf` on migrate.

## v0.8.0 — 2026-08-06

- **Scry.app — the macOS desktop app.** The web UI in its own signed,
  notarized window (`Scry-<version>-arm64.dmg` on every release), with **no
  local server at all**: the window reaches the mirror in-process, so ports,
  addresses, and conflicts stop existing as UX. First launch runs the same
  in-window setup as the browser; a second launch focuses the running window.
  The bundle carries the CLI (`Contents/Resources/bin/scry`) so a
  desktop-only install can still wire up an agent — see `docs/DESKTOP.md`.
- **Sync starts after in-app onboarding.** `serve` (and the app) began the
  background watch loop only when a credential existed at boot; finishing
  first-run setup now kicks it off without a restart.

## v0.7.0 — 2026-08-06

- **`scry mcp install <client>`.** Pins the current profile and absolute binary
  path into an MCP host registration so clients that do not inherit shell env
  cannot silently attach to the default mirror. `claude` runs
  `claude mcp add` (or prints a manual command if the binary is missing);
  `cursor` / `codex` / `json` print paste-ready config; `--dry-run` prints
  without registering. See `docs/MCP.md` and `docs/AGENT_SETUP.md`.
- **Browser guard on the local API.** Reject cross-origin writes and
  DNS-rebinding reads so a page opened in the browser cannot drive the
  loopback mirror as an open proxy.
- **Space names (schema v14).** `spaces` table and `PageLite.space_name`;
  settings APIs to list Confluence spaces and persist `confluence.spaces`.
- **Docs UX wave.** Space names in the UI, unified recents, scope pickers, and
  a Recently edited view for mirrored pages — landing in a final recency-first
  shape: Viewed / Updated / By author tabs replace the sidebar space tree, in
  both the web UI and the TUI docs mode.
- **Epics built-in view.** The open backlog grouped by epic, one click from
  the sidebar.
- **Mirror file permissions.** The database and its WAL/SHM sidecars are
  chmodded to `0600` and data directories to `0700` on open; older installs
  are tightened the next time scry opens them.
- **A face.** Wordmark, logo, and a favicon the app never had; the README
  leads with the live demo and a hero clip.
- **Demo speaks English.** The bundled snapshot's statuses, types, titles, and
  space homes read as English product data (Korean narrative pages remain for
  CJK search); page authors spread across five personas.
- **`docs/FAQ.md`.** The hard questions answered with receipts — site load
  math, admin visibility, single-maintainer risk, agent data exposure.
- **`scry.localhost`.** `serve` opens `http://scry.localhost` when the resolver
  maps it to loopback.
- **Port-conflict handling.** On a busy listen port, hand off to a running
  scry or fall back to a free port instead of failing opaquely.
- **Keyboard triage.** Sprint cleanup from the keyboard without touching the
  mouse; TUI `s` aliases `t` for transition (parity with the web flow).
- **Freshness chip.** Surface the server↔Jira leg and pull the mirror on focus.
- **Warm-boot cache.** Chunked IndexedDB writes and an honest warm-boot metric
  for durable bootstrap.
- **Interaction performance gate.** Budget tests against a 10k-issue fixture.
- **TUI page-scroll keys.** Register page-scroll bindings in `keyMap` so help
  and docs match what the navigator actually does.
- Confluence sync hardened for real sites (team spaces by default, chunked
  CQL, quoted space keys, tolerated 404s).

## v0.6.0 — 2026-08-06

- **Confluence page labels (schema v13).** `pages.labels` (JSON array,
  alphabetical) collected via `expand=metadata.labels` on the page fetch and
  exposed on `PageLite` everywhere pages appear (list, detail, search). First
  label page only (≤25) — real pages carry single-digit label counts.
- **Epic hierarchy (schema v11/v12).** `issues.hierarchy_level` (source tree
  rank, backfilled from raw) and a derived `issues.epic_key` — the nearest
  level-1 ancestor via `parent_key`, recomputed after every upsert batch, so a
  sub-task groups under its epic rather than its story. `IssueLite` now carries
  `epic_key` and `parent_key` separately; the TUI supports `group_by=epic`
  (label `KEY summary`, `(no epic)` bucket); `issues_full` is rebuilt (v12) to
  expose the new columns; snapshots carry them.
- **Confluence page mirror.** Second source on the items spine — sync, pages
  API, unified search, and a demo snapshot that carries the Nimbus wiki beside
  the issue backlog.
- **Docs in the web UI.** Mirrored wiki pages as a sidebar tree, document
  panel, and unified search.
- **TUI docs navigator.** `D` toggles a space-grouped page tree.
- **Epic hierarchy in the web UI.** Group labels, row chips, breadcrumb, and
  rollup over the honest `epic_key`.
- **Mobile viewport.** Phones render the desktop layout instead of a squeezed
  column.

## v0.5.0 — 2026-08-05

- **Workspaces.** `serve` mounts every profile under `/w/<name>/`; the web UI
  workspace picker switches between mounted profile mirrors.
- **TUI neon look.** Ambient animation, mouse support, palette, and match
  highlight.
- **Search prefix match.** Bare terms prefix-match so inflected Korean (and
  similar morphology) is found.

## v0.4.0 — 2026-08-05

- **TUI custom-field edit.** Edit discovered custom fields with Jira-allowed
  values only; detail shows the discovered set.
- **Update notice.** Daily anonymous check on every surface, with opt-out.
- **Hosted-demo service-worker handshake.** Time out cleanly and say so when
  the browser cannot run the demo.

## v0.3.0 — 2026-08-05

- **Field auto-discovery.** The first full sync discovers and configures
  custom fields itself.
- **Filter axes from discovered fields.** Per-project scope; the detail panel
  renders whatever fields the site actually has, including multi-select
  `array<option>` editors.
- **Sync progress denominator.** Projects are optional on sync, and progress
  lines carry a real total.
- **Sync history.** Activity behind the sidebar timestamp.

## v0.2.1 — 2026-08-05

- Sign and notarize the macOS release binaries.
- Hosted demo: local write simulation that says the change was not saved, and
  copy that identifies the surface as a demo (no token prompt).

## v0.2.0 — 2026-08-05

- **Team config sharing.** `scry team export` writes the views, field map,
  group rules and thresholds a team agrees on into one file to commit next to
  the code; `scry team import` merges it into a profile (`--dry-run` prints the
  same plan the apply path runs, `--overwrite` replaces conflicts). Export is
  whitelist-only and a reflection test forces every new `Config` field to be
  classified as shareable or private. Credentials, account identity and
  per-machine preferences never travel; `members` ships only with
  `--with-members` because it carries email addresses. A file containing
  credential keys is refused on import rather than silently ignored.
- **Rate-limit visibility (schema v6).** The Jira client counts outbound
  attempts, 429s, 5xx, retries and backoff wait; each sync pass flushes them
  into `api_usage` (one row per UTC day). Shown in `scry status`, `status
  --json`, `GET settings/` and the settings runtime panel — hidden while the
  count is zero. This is our own call volume, not Jira's remaining point
  budget, which the site does not expose. The retry policy itself is unchanged.
- **`scry fields`.** Reports which custom fields are actually populated, by
  listing the site's fields and probing a stratified, deterministic sample of
  mirrored issues with `fields=*all`. Fields with real usage that are missing
  from `fieldMap` come with a paste-ready fragment; fields at zero are listed
  as the bloat. Not one SQL query over the mirror — the mirror only stores what
  `fieldMap` already names.
- **`scry snapshot` (T6.4).** Builds a shareable copy by creating a fresh
  schema and copying rows into it, so dropped tables leave no residue.
  Personal state and `sync_state` counters stay behind. `--spread` restates
  timestamps across a window while preserving every issue's internal ordering,
  `--scale` clones issues onto new keys for benchmark fixtures, `--now` pins
  the clock for reproducible builds, and a credential scan runs before the file
  is published.
- **Per-command help.** Every subcommand answers `--help` with a summary, the
  real call shape including positional arguments, its flags, working examples
  and related commands, exiting 0. Flag lines are generated from the FlagSet so
  they cannot drift from the registration site.
- **TUI parity.** Feed focus tabs (`1`–`4`) with per-tab unread badges, and
  saved-view `sort` / `dir` / `group_by` support. Priority sorting keys on
  `priority_rank` rather than the localized priority name.
- Favorites live in the mirror (`GET/PUT/DELETE favorites/`) instead of only in
  browser storage, so `scry sql` and agents can see them; the hosted demo,
  which has no writable API, falls back to local storage.
- Removed the `presence` client stack and its feature flag: the server has
  answered those endpoints with a permanent 404 since extraction, and the
  security model (one user, loopback) has no room for it.

- **Zero-install hosted demo (v0.3).** Static snapshot of `examples/demo.db`
  (bootstrap + 519 detail JSON + attachment bytes) served by a demo-only service
  worker on GitHub Pages — no binary, no account. `scry export-static`,
  `make hosted-demo`, `e2e/hosted/`, `.github/workflows/pages.yml`. ADR 0004
  addendum: static JSON+SW instead of sqlite-wasm (client already boots from
  bootstrap JSON; FTS is client-side typing search for the demo).
- **Retention loop (v0.3).** `scry serve` starts the sync watch loop by default
  when a credential is configured (`--no-sync` opts out; `--sync` kept as a
  deprecated alias). `scry install-service` writes a launchd agent or systemd
  user unit for `serve --no-open`. After each successful watch cycle, one OS
  desktop notification may fire for new personal-feed events (macOS
  `osascript`, Linux `notify-send`; config `notify`, default true; body is the
  issue title only). Schema v5: `sync_state.first_sync_at`, `sync_count`,
  `last_notified_at`, and the `issues_full` view (`summary` + issues columns).
- Personal watch feed: `GET /api/v1/issues/feed/` and `POST …/feed/read/` compute
  activity from the mirror at query time (status/assignee/fields changelog,
  comments, attachments, issue creates) over a 30-day window, with local
  `feed_reads` receipts (schema v4). Relevance is watched · assignee · reporter ·
  mention; self-actions are excluded. `account_id` is stored on credential
  verify. `features.feed` defaults on. In-tab browser Notifications fire when
  unread grows (no VAPID/push).
- Ported the demo Jira seeder from Python to Go (`go run ./tools/seed-demo`),
  removing the last Python dependency. Flag contract, category-ladder
  transitions, and repair idempotency are unchanged.
- Extracted the web application from an internal deployment into this
  repository, replacing every hardcoded internal value with a runtime
  configuration document (`config.json`) fetched before mount.
- Generalized built-in views to axes that mean the same thing on every Jira site
  (`status_category`, `unassigned`, `reopened`, `stale`, `updated_from`),
  replacing presets that referenced internal project keys, status names, and team
  groupings.
- Replaced name-matching rules for resolution and reopen detection with status
  *category* rules, which are stable across sites and account languages. Dropped
  the internal `working_hours_in_status` field, which no code ever populated.
- Added `scry serve`: serves the built UI, the runtime config document, and
  `/healthz`. Refuses to bind a non-loopback address without `--allow-remote`,
  because the mirror has no authentication.
- Added a Jira seeding tool for populating a throwaway Jira site with releases,
  components, issues, transition history, comments, and links — either generated
  or projected from a dataset file (`tools/seed-demo`).
- Specified the storage schema as a public contract, plus the HTTP, sync, and
  agent contracts under `specs/000-product/`.
- Implemented that schema in `internal/store`: SQLite (pure-Go driver, so the
  binary needs no cgo) with WAL, a migration runner keyed on `PRAGMA
  user_version` that refuses a database written by a newer scry, an FTS5 index
  over titles, bodies and comment text, and the derived-field calculator
  (`status_changed_at`, `resolved_at`, `reopen_count`, `reopened_at`,
  `assignee_changed_at`, `comment_count`, `priority_rank`).
- Added `issues.reporter_email`, which the client filters and groups on but the
  first schema draft only had for the assignee.
- Schema additions over the first draft of `data-model.md`, all documented there:
  a `deleted_items` tombstone table so `delta` can report deletions,
  `contentless_delete=1` on `items_fts` so one row can be replaced,
  `items(synced_at)` and `issues(key)` indexes, and `ON DELETE CASCADE` from
  every child table to `items`. Corrected the first example query, which joined
  on a column the spine does not have.
