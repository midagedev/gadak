# Configuration reference

**Glossary.** A *workspace* is one origin and one mirror, kept apart from
every other. `gadak --workspace x` (or `GADAK_WORKSPACE=x`) moves both
`config.json` and the mirror under `~/.gadak/profiles/<name>/`, and the same
workspace is what `gadak serve` mounts at `/w/<name>/` and what the sidebar
switcher lists. `gadak workspaces` lists them; `--profile`, `GADAK_PROFILE`
and `gadak profiles` still work as aliases, and the on-disk directory keeps
its `profiles/` name.

gadak stores its configuration in `~/.gadak/config.json` (mode `0600`). A
named workspace moves both the file and the mirror under
`~/.gadak/profiles/<name>/`.

A running `gadak serve` also mounts every sibling workspace under
`/w/<name>/` — full API, opened lazily on first request. The sidebar shows a
switcher when more than one exists. `GET /api/v1/workspaces` lists them
(name, site, projects — never credentials). HTTP mounts are lazy; **every
credentialed workspace gets a watch loop at boot** (same as the desktop
app). OS notifications and the update check stay on the workspace `serve`
was started with (the primary one).

Most day-to-day keys are editable from the web **Settings** dialog
(`GET` / `PUT /api/v1/issues/settings/`) and from `gadak config`. Credentials
use a separate endpoint (`credential/`) or `gadak init`. A few operational
facts are read-only on the settings response under `runtime`.

Full field mapping and plugin axes: [EXTENDING.md](EXTENDING.md). HTTP shapes:
[specs/000-product/contracts/api.md](../specs/000-product/contracts/api.md)
(settings section).

### Naming layers

`config.json`, the HTTP issue contract, and SQL do not share one identifier
style. Existing names stay; do not rename a field to "fix" the mix.

| Layer | Where | Convention | Examples |
| --- | --- | --- | --- |
| On-disk config | `~/.gadak/config.json` (`internal/config/config.go`) | New keys follow the file's existing camelCase. A few keys are already snake_case; leave them. | camelCase: `tokenVerifiedAt`, `fields`. Existing snake_case: `account_id`, `display_name`. |
| Issue/read HTTP | `specs/000-product/contracts/api.md` (`IssueLite`, detail, feed) | New fields are snake_case. | Wire `issue_key`; the SQL column is `key`. |
| SQL | `gadak.db` (`specs/000-product/data-model.md`) | Documented column names. | `issues.key`, `items.key` |

Settings `GET`/`PUT` (`/api/v1/issues/settings/`) and `config.json` served to
the browser reuse the on-disk key names (camelCase) rather than translating
them into the issue-contract snake_case. That is the config layer on an HTTP
path, not a third style.

Never introduce a third style (kebab-case, PascalCase, a new prefix). Do not
rename an existing field.

---

## `gadak config`

`gadak config` reads and writes the same profile `config.json` the Settings
dialog uses. The path catalog and the validators live in
`internal/config/settings.go` — Settings PUT, the dialog, and this command
do not keep separate rules for a field they share.

```
gadak config list
gadak config list --json
gadak config get <path>
gadak config get <path> --json
gadak config set <path> <value>
gadak config set <path> <value> --json
```

`--profile` / `GADAK_PROFILE` selects the file (`~/.gadak/config.json`, or
`~/.gadak/profiles/<name>/config.json`). Theme and every other catalog
path are therefore per workspace. See [Files and permissions](#files-and-permissions).

`list` prints one TSV row per catalog path (`path`, JSON value, description)
and ends with a credentials reminder. `--json` is one object
(`settings` + `note`). `get` prints the JSON value; `--json` wraps
`{"path","value"}`. `set` prints the stored value the same way. `set`
accepts JSON, or a bare scalar (`dark`, `true`, `30`); arrays and objects
need JSON. A value of `-` reads stdin.

An unknown path exits **64** and prints every valid path. Credentials
(`site`, `email`, `token`) are not catalog paths — use `gadak init`.

`gadak config list` is the live catalog (28 paths). Three of them are not
on the Settings PUT document and are not on the Settings form:
`notify`, `updateCheck`, `attachmentCacheMB`. PUT leaves those three
untouched (it copies the live file, then overwrites the fields it knows).

### `appearance.theme`

Validation is **shape only**: empty or `system` (stored as the zero value),
or a lowercase identifier `[a-z0-9-]{1,32}`. Palette names belong to the
web so a new palette does not need a server deploy. The picker today
iterates `system`, `light`, `dark`, `ink`, `ember`
(`web/src/lib/theme.ts`).

### `PUT /api/v1/issues/settings/` is a replace

The path is `/api/v1/issues/settings/` (`apiBase` + `settings/`), not
`/api/v1/settings/`. A successful PUT assigns the writable fields from the
body and preserves credentials. Omitted assigned fields become empty
(projects, maps, members, groups, intervals, …). Omitted `features` is
normalized (`feed` defaults on; the other flags default off). Partial
edits are GET then PUT — the dialog's `save` and the theme picker both do
that.

Three keys are omit-to-preserve, so an older client cannot wipe them:
`fields` (discovery output), `confluence`, `appearance`. `runtime`, `site`,
and `hasCredential` on the body are ignored.

---

## Keys

| Key | Type | Default | Where to edit | Applies |
| --- | --- | --- | --- | --- |
| `site` | string (URL) | _(empty)_ | Credential dialog / `PUT credential/` / `gadak init` (not `gadak config`) | Immediate for writes & deep links |
| `email` | string | _(empty)_ | Credential dialog / `gadak init` (not `gadak config`) | Immediate |
| `token` | string | _(empty)_ | Credential dialog / `gadak init` (not `gadak config`) | Immediate; **never** returned by `settings/` or `config.json` |
| `tokenVerifiedAt` | string (RFC3339) | _(empty)_ | Set by successful credential verify | Read-only side effect |
| `tokenOwner` | string | _(empty)_ | Set by successful credential verify | Read-only side effect |
| `appearance.theme` | string | empty → **system** | Settings theme picker / `gadak config set appearance.theme` | Immediate after reload; shape `[a-z0-9-]{1,32}` (see above) |
| `projects` | string[] | `[]` (empty = every project this account can see) | Settings → Sources / `gadak init` / `gadak config set projects` | Next sync / list scope; UI reload after save |
| `fields` | FieldSpec[] | `[]` | Auto on first full sync / `gadak fields --apply` / `gadak config set fields` (read-only on Settings GET as `fieldSpecs`) | Next sync ingest; `fieldUsage` on Settings is project→alias fill counts |
| `fieldMap` | map alias→field id | `{}` | leftover; LoadFor migrates into `fields` and clears it. Not settable. | migrated on load |
| `bodyFields` | string[] (field ids) | `[]` | Settings → Fields / `gadak config` | Next sync (FTS body); additive with role=body specs |
| `editableFields` | map alias→field id | `{}` | leftover; LoadFor overlays onto `fields` (legacy wins per alias) and clears it. Not settable. | migrated on load |
| `members` | Member[] | `[]` | Settings → Members / `gadak config` | Immediate (cached projection invalidated) |
| `groupRules` | GroupRule[] | `[]` | Settings → Teams / `gadak config` | Immediate |
| `groupQuery` | string (SQL) | `""` | Settings → Teams / `gadak config set groupQuery` | Immediate (derived view rebuild) |
| `groupLabels` | map | `{}` | Settings → Teams / `gadak config` | Immediate |
| `groupColors` | map | `{}` | Settings → Teams / `gadak config` | Immediate |
| `productByGroup` | map → `{key,label}` | `{}` | Settings → Teams / `gadak config` | Immediate |
| `features` | map of bool | keys: `feed`, `push`, `deploy`, `qa`, `teamGroups`; **`feed` defaults true when omitted**, others false | Settings → Features / `gadak config set features` or `features.<name>` | Immediate after reload (client reads `config.json`) |
| `qaDashboardUrl` | string | _(empty)_ | Settings → Features / `gadak config` | Immediate after reload |
| `staleThresholdHours` | int | `0` → client **72** | Settings → Sync / `gadak config` | Immediate after reload |
| `syncIntervalSec` | int (seconds) | `0` → **60** | Settings → Sync (presets / custom) / `gadak config` | Next watch tick; no process restart |
| `reconcileIntervalSec` | int (seconds) | `0` → **3600** | Settings → Sync (presets / custom) / `gadak config` | Next watch tick; no process restart |
| `notify` | bool | **true** when absent | `gadak config set notify` / `config.json` (not on Settings UI or Settings PUT) | Next watch-loop tick; OS desktop alerts for new personal-feed events |
| `updateCheck` | bool | **true** when absent | `gadak config set updateCheck` / `config.json` (not on Settings UI or Settings PUT) | Next `sync` / `status` / `serve` / desktop start; once-per-day GitHub release lookup (cached under the profile directory on disk). Set `false` to opt out — no GitHub version check; outbound stays your configured origins |
| `attachmentCacheMB` | int | `0` → package **512** | `gadak config set attachmentCacheMB` / `config.json` (not on Settings UI or Settings PUT) | Cap (MB) for the on-disk cache opened when a workspace mounts; `0` becomes the package default |
| `confluence` | object or absent | absent = wiki mirror off. `gadak init --standalone` writes the block scoped to `LOC` | Settings → Sources / `gadak config set confluence` | Next Confluence pass |
| `linear` | object or absent | absent = Linear source off. `apiKey` (personal API key, sent bare in the Authorization header) turns the source on; writes to Linear-owned keys route through it | edit `config.json` (no Settings surface yet) | Next `sync --source linear` |
| `linear.teamIds` | string[] | `[]` = every team the key can see; team UUIDs restrict the mirror scope | edit `config.json` | Next Linear pass |
| `devStatus` | bool | **false** | `gadak config set devStatus true` / `config.json` (not on Settings UI or Settings PUT) | Next sync; connected Cloud: mirror Jira's development-status API into `dev_links` (one extra request per issue). Standalone always fetches; `gadak dev link` / `dev scan` write the same table |
| `actor` | object or absent | absent = no acting identity; env `GADAK_ACTOR` (`slug\|display name`) overrides, and Claude Code sessions are auto-detected when both are unset | `gadak config set actor 'slug\|display name'` / `config.json` (not on Settings UI or Settings PUT; never team-exported) | Next origin session; writes to an issuetap origin (standalone/paired) carry `X-Issuetap-Actor` and attribute to that agent account. Never sent to connected Jira/Linear |
| `locale` | string | _(empty)_ = English; `en` \| `ko` \| `ja` \| `de` | `gadak config set locale ko` / `config.json` (not on Settings UI or Settings PUT) | Standalone only: the origin's display-name language — status / issue-type / field names and agent aliases follow it; priority names stay English, like a live Cloud site. Changing it rebuilds the mirror on the next sync (display names are cached). A connected workspace ignores it: its language is the Atlassian account's |
| `confluence.spaces` | string[] | `[]` = every *global* space; personal spaces only if named (`internal/config/config.go`) | Settings → Sources / `gadak config set confluence.spaces` | Next Confluence pass |

The space list *is* the scope: drop a space and the next Confluence pass
removes it from the mirror; add one and that space is fetched from the start
(per-space watermark, schema v19 — see
[`runbooks/confluence-space-scope.md`](runbooks/confluence-space-scope.md)).
Most keys apply without restart; `syncIntervalSec` and
`reconcileIntervalSec` are picked up on the next watch tick.

### `groupQuery`

Optional classifier for team groups that do not fit `groupRules` (three
AND-lists). One `SELECT` or `WITH` returning **two columns**: issue key, group.
It runs when the derived view is rebuilt (config save or sync version), never
on a keystroke. `REGEXP` is available (`col REGEXP '(?i)invoice'`).

| Second column | Meaning |
| --- | --- |
| `'billing'` | assign that group |
| `''` | unclassified — stop, do not fall through |
| `NULL` or a key the query omits | fall through to `groupRules`, then the assignee's member `group` |

Site-specific `CASE` belongs in this string (and in `gadak team export`), not
in gadak source. Writes, `PRAGMA`, `ATTACH`, and multiple statements are
rejected on save.

```sql
SELECT i.key,
  CASE
    WHEN EXISTS (SELECT 1 FROM json_each(i.labels) e WHERE e.value = 'skip-triage') THEN ''
    WHEN i.components REGEXP '(?i)invoice|payment'
      OR coalesce(json_extract(i.custom, '$.billing_code'), '') REGEXP '(?i)invoice'
      THEN 'billing'
    WHEN EXISTS (SELECT 1 FROM json_each(i.labels) e WHERE e.value = 'backend') THEN 'platform'
    ELSE NULL
  END
FROM issues_full i
```

### `fields` (FieldSpec)

One logical custom field. Jira often creates a separate field id per board
template for the same display name, so a spec can list several ids; sync
coalesces the first filled value.

| Field | Meaning |
| --- | --- |
| `alias` | Stable key (ASCII slug of the name, else `cf_<id>`) |
| `label` | Jira display name (account language) |
| `ids` | Field ids sharing that name, most-filled first |
| `role` | `body` \| `facet` \| `user` \| `plain` |
| `kind` | Editor: `option` \| `multi_option` \| `user` \| `version_array` \| empty |
| `auto` | Discovery-owned; regenerated on `gadak fields --apply` / re-discovery |

When `fields` is empty, legacy `fieldMap` / `editableFields` are synthesized into
specs for consumers. Discovery only writes `fields` (never rewrites `fieldMap`).
The first full sync with neither map configured uses `fields=*all`, discovers
in-use custom fields, saves specs, and backfills `issues.custom` from raw without
re-download. Settings GET exposes `fieldSpecs` and `fieldUsage` as read-only;
PUT ignores both so it cannot wipe discovery.

### Interval floors (settings `PUT`)

| Key | Minimum when non-zero |
| --- | --- |
| `syncIntervalSec` | 15 seconds |
| `reconcileIntervalSec` | 300 seconds (5 minutes) |

Below the floor → `400` with a clear `error` string; the file is not written.
`gadak config set` uses the same `ValidateIntervals` and refuses the write
the same way. `0` always means “use default”.

### Interval changes (no process restart)

`internal/sync.Watch` reloads config each cycle when `opts.Reload` is set
(`gadak serve` passes `config.Load`). A new `syncIntervalSec` /
`reconcileIntervalSec` is applied with `tick.Reset` on that cycle — picked up
on the next watch tick, no process restart. `PUT settings/` updates the
on-disk file and the server’s atomic config immediately; the watch loop
reads it on the next cycle. `gadak serve` starts the loop by default when a
credential is configured (`--no-sync` opts out).

### OS desktop notifications

When the watch loop runs (`gadak serve` with a credential, or `gadak sync
--watch`), each successful cycle may fire **one** bundled OS notification for
new personal-feed events since `sync_state.last_notified_at` (macOS
`osascript`, Linux `notify-send`). On Windows and other unsupported platforms
the notifier is a no-op and **does not advance** `last_notified_at`, so a later
toast implementation can still deliver those events. The body carries
the issue title only — never comment text. Set `"notify": false` in
`config.json`, or `gadak config set notify false`, to opt out. Notifications
never write `feed_reads`. `GET settings/` reports the capability as
`runtime.osNotifySupported` (always present; false is meaningful).

### Update check

Once a day, `gadak sync` (after a successful run), `gadak status`, `gadak
serve`, and the desktop app may query GitHub's public releases API for this
project and cache the answer under that profile's directory on disk
(`update-check.json`). The request carries no account identifiers. Dev
builds (`0.0.0-dev`) never check. Network errors and rate limits are silent.
Set `"updateCheck": false` in `config.json`, or `gadak config set updateCheck
false`, to disable the lookup entirely (no GitHub version check; outbound
stays your configured origins — Atlassian, Linear if enabled, pairing home,
user-invoked `gh`). The lookup only feeds the sidebar
banner; installing an update is `brew upgrade --cask gadak`, a new dmg, or
replacing the Windows portable-zip directory with a newer zip.

---

## Read-only `runtime` on `GET settings/`

| Field | Meaning |
| --- | --- |
| `profile` | Active profile name, or `"default"` |
| `dbPath` | Absolute path of the mirror SQLite file |
| `dbSizeBytes` / `dbSizeHuman` | File size |
| `dbModifiedAt` | mtime (RFC3339), when the file exists |
| `configPath` | Absolute path of `config.json` |
| `issueCount` / `commentCount` | Row counts in the mirror |
| `schemaVersion` | Migration level |
| `watermark` / `syncVersion` / `lastFullSyncAt` / `lastError` | From `sync_state` |
| `gadakVersion` | Build/version string (`server.Version`) |
| `defaultSyncIntervalSec` / `defaultReconcileIntervalSec` | Placeholder defaults for the UI |

Secrets never appear here. Paths have copy buttons in the Settings UI (including
a ready-to-paste `sqlite3 <dbPath>`).

---

## Sharing team settings (`gadak team export` / `import`)

Teams can commit a single file (for example `gadak-team.json` in a repo) so a new
member runs `gadak init` then `gadak team import gadak-team.json` and gets the same
views, fields, and group rules. Export is **whitelist-only**: new `Config`
fields never leave the machine until someone explicitly adds them to the share
list. Credentials and per-machine prefs are never included.

| Shared (export / import) | Not shared |
| --- | --- |
| `projects` | `site`, `email`, `token` |
| `fields`, `bodyFields` | `account_id`, `tokenOwner`, `tokenVerifiedAt` |
| `groupRules`, `groupQuery`, `groupLabels`, `groupColors`, `productByGroup` | `syncIntervalSec`, `reconcileIntervalSec` |
| `features`, `qaDashboardUrl`, `staleThresholdHours` | `notify`, `updateCheck`, `attachmentCacheMB` |
| `members` only with `export --with-members` (emails; stderr warns) | personal machine intervals / notification prefs |
| saved views (`name` + `config`; new ids on import) | view `id` / timestamps |

Import **merges** by default (fill empty settings keys; add views only when the
name is new). `--overwrite` replaces conflicts. Prefer
`gadak team import FILE --dry-run` first. A file that contains credential keys
(`site` / `token` / …) is rejected — do not hand-edit secrets into a share file.
Import still converts leftover `fieldMap` / `editableFields` in old team files
into `fields`.

---

## Personal state (`gadak export` / `import`)

`gadak export [--out FILE]` dumps this profile's personal tables: saved views,
watches, favorites, and recents (`cmd/gadak/export.go`; help in
`cmd/gadak/help.go`). Credentials never appear in the file — a credential-shaped
string is refused (`secretscan`). It is not a `gadak team export` file (team
settings live in that other command) and it does not include the standalone
persist file (`origin/issuetap.db`) or the issue rows in `gadak.db`.

`gadak import <FILE>` restores those four lists. On a name/key conflict the
file wins; local-only rows stay (`cmd/gadak/import.go` `applyPersonalExport`).

---

## What you must still edit by hand (or outside Settings)

| Concern | How |
| --- | --- |
| Jira site / email / API token | Credential dialog, `PUT credential/`, or `gadak init` — **not** settings PUT |
| Unattended setup (agents, CI, provisioning) | `gadak init` flags/env — see below |
| Profile selection (the disk directory; serve mounts it as a workspace) | CLI `--profile` / `GADAK_PROFILE` (separate home directory) |
| `GADAK_HOME` override | Environment variable |
| Binary version string in UI | `server.Version`, wired from `cmd/gadak` ldflags by goreleaser (`main.version`); dev builds show `0.0.0-dev` |
| Team views / field map / group rules (between people) | `gadak team export` / `gadak team import` (see above) |
| Sync loop process | Start/stop `gadak serve` (default when credentialed; `--no-sync` opts out) or `gadak sync --watch` |
| Keep serve across reboots | `gadak install-service` (launchd / systemd user); `--uninstall` removes it |
| OS desktop notifications | `gadak config set notify false` (default on) |
| GitHub release lookup | `gadak config set updateCheck false` (default on) |
| Attachment cache cap | `gadak config set attachmentCacheMB` (`0` = package default 512) |

There is no remaining day-to-day operational knob that only lives in a hand
edit of the JSON file. Settings and `gadak config` share
`internal/config/settings.go`; `notify`, `updateCheck`, and
`attachmentCacheMB` are on `gadak config` only (not on Settings PUT). Direct
file edit still works for automation and recovery.

### Unattended `gadak init`

A prompt an agent cannot answer is a hang, so init is non-interactive whenever
anything is supplied:

| Value | Flag | Environment |
| --- | --- | --- |
| site | `--site <url>` | `GADAK_SITE` |
| email | `--email <addr>` | `GADAK_EMAIL` |
| projects | `--projects <A,B>` | `GADAK_PROJECTS` |
| token | `--token-file <path>` / `--token-stdin` | `GADAK_TOKEN` |

Per value: flag beats environment beats the saved config. **There is no
`--token` flag** — argv shows up in `ps` and shell history; passing one fails
with a pointer to the three accepted paths.

Using *any* flag or `GADAK_*` value turns prompting off entirely, so a missing
value errors with the list of what is missing rather than waiting on stdin.
`--json` prints one object (`profile`, `account`, `site`, `projects`, `path`;
never the token) and also forbids prompting, since a prompt would corrupt the
document a caller is parsing.

Running `gadak init` bare in a terminal keeps the original behavior: all four
values are re-asked with the current one shown, and an empty answer keeps it —
which is how an expired token gets replaced. The credential is always verified
against `/myself` before anything is written.

---

## Files and permissions

| Path | Role |
| --- | --- |
| `$GADAK_HOME/config.json` or `~/.gadak/config.json` | Settings + credential (0600) |
| `$GADAK_HOME/gadak.db` | SQLite mirror (a cache; the next sync rebuilds it from the origin) |
| `$GADAK_HOME/origin/issuetap.db` | Standalone origin persist (`internal/origin/origin.go` `PersistRel`). SQLite (WAL); this is the record on a standalone workspace. Copy while gadak is not running (include `-wal`/`-shm`), or `sqlite3 <db> ".backup"`. Absent on a connected workspace. A sibling `origin/issuetap.yaml` is a one-shot seed if the db is missing. |
| `~/.gadak/profiles/<name>/` | Isolated config + mirror (and, on standalone, persist) per profile |

Never write issue rows into the DB by hand — the next sync overwrites them. The
supported external write table is `enrichments` (see [PLUGINS.md](PLUGINS.md)).
