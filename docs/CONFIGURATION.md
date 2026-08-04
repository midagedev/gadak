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
| `projects` | string[] | `[]` | Settings → Sync | Next sync / list scope; UI reload after save |
| `fieldMap` | map alias→field id | `{}` | Settings → Field mapping | Next sync ingest |
| `bodyFields` | string[] (field ids) | `[]` | Settings → Field mapping | Next sync (FTS body) |
| `editableFields` | map alias→field id | `{}` | Settings → Field mapping | Immediate (empty hides inline edit) |
| `members` | Member[] | `[]` | Settings → Members | Immediate (cached projection invalidated) |
| `groupRules` | GroupRule[] | `[]` | Settings → Teams | Immediate |
| `groupLabels` | map | `{}` | Settings → Teams | Immediate |
| `groupColors` | map | `{}` | Settings → Teams | Immediate |
| `productByGroup` | map → `{key,label}` | `{}` | Settings → Teams | Immediate |
| `features` | map of bool | all `false` | Settings → Features | Immediate after reload (client reads `config.json`) |
| `qaDashboardUrl` | string | _(empty)_ | Settings → Features | Immediate after reload |
| `staleThresholdHours` | int | `0` → client **72** | Settings → Sync | Immediate after reload |
| `syncIntervalSec` | int (seconds) | `0` → **60** | Settings → Sync (presets / custom) | **After restart** of `scry serve` |
| `reconcileIntervalSec` | int (seconds) | `0` → **3600** | Settings → Sync (presets / custom) | **After restart** of `scry serve` |
| `notify` | bool | **true** when absent | `config.json` (not on Settings UI) | Next watch-loop tick; OS desktop alerts for new personal-feed events |

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

## What you must still edit by hand (or outside Settings)

| Concern | How |
| --- | --- |
| Jira site / email / API token | Credential dialog, `PUT credential/`, or `scry init` — **not** settings PUT |
| Profile selection | CLI `--profile` / `SCRY_PROFILE` (separate home directory) |
| `SCRY_HOME` override | Environment variable |
| Binary version string in UI | `server.Version` package var — wire from `cmd/scry` ldflags (default `0.0.0-dev` until wired) |
| Sync loop process | Start/stop `scry serve` (default when credentialed; `--no-sync` opts out) or `scry sync --watch` |
| Keep serve across reboots | `scry install-service` (launchd / systemd user); `--uninstall` removes it |
| OS desktop notifications | `"notify": false` in `config.json` to disable (default on) |

There is no remaining day-to-day operational knob that only lives in the JSON
file: intervals, projects, features, field maps, teams, and members are all on
the Settings surface. Direct file edit still works for automation and recovery.

---

## Files and permissions

| Path | Role |
| --- | --- |
| `$SCRY_HOME/config.json` or `~/.scry/config.json` | Settings + credential (0600) |
| `$SCRY_HOME/scry.db` | SQLite mirror |
| `~/.scry/profiles/<name>/` | Isolated config + mirror per profile |

Never write issue rows into the DB by hand — the next sync overwrites them. The
supported external write table is `enrichments` (see [PLUGINS.md](PLUGINS.md)).
