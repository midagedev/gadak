# Changelog

## Unreleased

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
