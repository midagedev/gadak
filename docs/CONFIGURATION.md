# Configuration reference

scry stores its configuration in `~/.scry/config.json` (mode `0600`). A profile
(`scry --profile x` or `SCRY_PROFILE=x`) moves both the file and the mirror under
`~/.scry/profiles/<name>/`.

Most day-to-day keys are editable from the web **Settings** dialog
(`GET` / `PUT /api/v1/issues/settings/`). Credentials use a separate endpoint
(`credential/`). A few operational facts are read-only on the settings response
under `runtime`.

Full field mapping and plugin axes: [EXTENDING.md](EXTENDING.md). HTTP shapes:
[specs/000-product/contracts/api.md](../specs/000-product/contracts/api.md)
(settings section).

---

## Keys

| Key | Type | Default | Where to edit | Applies |
| --- | --- | --- | --- | --- |
| `site` | string (URL) | _(empty)_ | Credential dialog / `PUT credential/` / `scry init` | Immediate for writes & deep links |
| `email` | string | _(empty)_ | Credential dialog / `scry init` | Immediate |
| `token` | string | _(empty)_ | Credential dialog / `scry init` | Immediate; **never** returned by `settings/` or `config.json` |
| `tokenVerifiedAt` | string (RFC3339) | _(empty)_ | Set by successful credential verify | Read-only side effect |
| `tokenOwner` | string | _(empty)_ | Set by successful credential verify | Read-only side effect |
| `projects` | string[] | `[]` (empty = every project this account can see) | Settings → Sync / `scry init` | Next sync / list scope; UI reload after save |
| `fields` | FieldSpec[] | `[]` | Auto on first full sync / `scry fields --apply` (read-only on Settings GET as `fieldSpecs`) | Next sync ingest; `fieldUsage` on Settings is project→alias fill counts |
| `fieldMap` | map alias→field id | `{}` | Settings → Field mapping (legacy; synthesized into `fields` when `fields` is empty) | Next sync ingest |
| `bodyFields` | string[] (field ids) | `[]` | Settings → Field mapping | Next sync (FTS body); additive with role=body specs |
| `editableFields` | map alias→field id | `{}` | Settings → Field mapping (legacy; Kind-bearing specs also enable inline edit) | Immediate |
| `members` | Member[] | `[]` | Settings → Members | Immediate (cached projection invalidated) |
| `groupRules` | GroupRule[] | `[]` | Settings → Teams | Immediate |
| `groupLabels` | map | `{}` | Settings → Teams | Immediate |
| `groupColors` | map | `{}` | Settings → Teams | Immediate |
| `productByGroup` | map → `{key,label}` | `{}` | Settings → Teams | Immediate |
| `features` | map of bool | keys: `feed`, `push`, `deploy`, `qa`, `teamGroups`; **`feed` defaults true when omitted**, others false | Settings → Features | Immediate after reload (client reads `config.json`) |
| `qaDashboardUrl` | string | _(empty)_ | Settings → Features | Immediate after reload |
| `staleThresholdHours` | int | `0` → client **72** | Settings → Sync | Immediate after reload |
| `syncIntervalSec` | int (seconds) | `0` → **60** | Settings → Sync (presets / custom) | **After restart** of `scry serve` |
| `reconcileIntervalSec` | int (seconds) | `0` → **3600** | Settings → Sync (presets / custom) | **After restart** of `scry serve` |
| `notify` | bool | **true** when absent | `config.json` (not on Settings UI) | Next watch-loop tick; OS desktop alerts for new personal-feed events |

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
| `auto` | Discovery-owned; regenerated on `scry fields --apply` / re-discovery |

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
`0` always means “use default”.

### Why intervals need a restart

`internal/sync.Watch` creates its tickers once at loop start from the `Config`
passed in. `scry serve` starts that loop by default when a credential is
configured (`--no-sync` opts out). `PUT settings/` updates the on-disk file and
the server’s atomic config for everything else (members, group rules,
features…), but it does **not** rebuild the Watch tickers. Restart `scry serve`
to pick up new intervals.

### OS desktop notifications

When the watch loop runs (`scry serve` with a credential, or `scry sync
--watch`), each successful cycle may fire **one** bundled OS notification for
new personal-feed events since `sync_state.last_notified_at` (macOS
`osascript`, Linux `notify-send`; Windows is a quiet no-op). The body carries
the issue title only — never comment text. Set `"notify": false` in
`config.json` to opt out. Notifications never write `feed_reads`.

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
| `scryVersion` | Build/version string (`server.Version`) |
| `defaultSyncIntervalSec` / `defaultReconcileIntervalSec` | Placeholder defaults for the UI |

Secrets never appear here. Paths have copy buttons in the Settings UI (including
a ready-to-paste `sqlite3 <dbPath>`).

---

## Sharing team settings (`scry team export` / `import`)

Teams can commit a single file (for example `scry-team.json` in a repo) so a new
member runs `scry init` then `scry team import scry-team.json` and gets the same
views, field map, and group rules. Export is **whitelist-only**: new `Config`
fields never leave the machine until someone explicitly adds them to the share
list. Credentials and per-machine prefs are never included.

| Shared (export / import) | Not shared |
| --- | --- |
| `projects` | `site`, `email`, `token` |
| `fields`, `fieldMap`, `bodyFields`, `editableFields` | `account_id`, `tokenOwner`, `tokenVerifiedAt` |
| `groupRules`, `groupLabels`, `groupColors`, `productByGroup` | `syncIntervalSec`, `reconcileIntervalSec` |
| `features`, `qaDashboardUrl`, `staleThresholdHours` | `notify`, `attachmentCacheMB` |
| `members` only with `export --with-members` (emails; stderr warns) | personal machine intervals / notification prefs |
| saved views (`name` + `config`; new ids on import) | view `id` / timestamps |

Import **merges** by default (fill empty settings keys; add views only when the
name is new). `--overwrite` replaces conflicts. Prefer
`scry team import FILE --dry-run` first. A file that contains credential keys
(`site` / `token` / …) is rejected — do not hand-edit secrets into a share file.

---

## What you must still edit by hand (or outside Settings)

| Concern | How |
| --- | --- |
| Jira site / email / API token | Credential dialog, `PUT credential/`, or `scry init` — **not** settings PUT |
| Unattended setup (agents, CI, provisioning) | `scry init` flags/env — see below |
| Profile selection | CLI `--profile` / `SCRY_PROFILE` (separate home directory) |
| `SCRY_HOME` override | Environment variable |
| Binary version string in UI | `server.Version` package var — wire from `cmd/scry` ldflags (default `0.0.0-dev` until wired) |
| Team views / field map / group rules (between people) | `scry team export` / `scry team import` (see above) |
| Sync loop process | Start/stop `scry serve` (default when credentialed; `--no-sync` opts out) or `scry sync --watch` |
| Keep serve across reboots | `scry install-service` (launchd / systemd user); `--uninstall` removes it |
| OS desktop notifications | `"notify": false` in `config.json` to disable (default on) |

There is no remaining day-to-day operational knob that only lives in the JSON
file: intervals, projects, features, field maps, teams, and members are all on
the Settings surface. Direct file edit still works for automation and recovery.

### Unattended `scry init`

A prompt an agent cannot answer is a hang, so init is non-interactive whenever
anything is supplied:

| Value | Flag | Environment |
| --- | --- | --- |
| site | `--site <url>` | `SCRY_SITE` |
| email | `--email <addr>` | `SCRY_EMAIL` |
| projects | `--projects <A,B>` | `SCRY_PROJECTS` |
| token | `--token-file <path>` / `--token-stdin` | `SCRY_TOKEN` |

Per value: flag beats environment beats the saved config. **There is no
`--token` flag** — argv shows up in `ps` and shell history; passing one fails
with a pointer to the three accepted paths.

Using *any* flag or `SCRY_*` value turns prompting off entirely, so a missing
value errors with the list of what is missing rather than waiting on stdin.
`--json` prints one object (`profile`, `account`, `site`, `projects`, `path`;
never the token) and also forbids prompting, since a prompt would corrupt the
document a caller is parsing.

Running `scry init` bare in a terminal keeps the original behavior: all four
values are re-asked with the current one shown, and an empty answer keeps it —
which is how an expired token gets replaced. The credential is always verified
against `/myself` before anything is written.

---

## Files and permissions

| Path | Role |
| --- | --- |
| `$SCRY_HOME/config.json` or `~/.scry/config.json` | Settings + credential (0600) |
| `$SCRY_HOME/scry.db` | SQLite mirror |
| `~/.scry/profiles/<name>/` | Isolated config + mirror per profile |

Never write issue rows into the DB by hand — the next sync overwrites them. The
supported external write table is `enrichments` (see [PLUGINS.md](PLUGINS.md)).
