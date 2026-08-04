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
| `syncIntervalSec` | int (seconds) | `0` → **60** | Settings → Sync (presets / custom) | **After restart** of `scry serve --sync` |
| `reconcileIntervalSec` | int (seconds) | `0` → **3600** | Settings → Sync (presets / custom) | **After restart** of `scry serve --sync` |

### Interval floors (settings `PUT`)

| Key | Minimum when non-zero |
| --- | --- |
| `syncIntervalSec` | 15 seconds |
| `reconcileIntervalSec` | 300 seconds (5 minutes) |

Below the floor → `400` with a clear `error` string; the file is not written.
`0` always means “use default”.

### Why intervals need a restart

`internal/sync.Watch` creates its tickers once at loop start from the `Config`
passed in. `scry serve --sync` starts that loop with the config loaded at process
boot. `PUT settings/` updates the on-disk file and the server’s atomic config for
everything else (members, group rules, features…), but it does **not** rebuild
the Watch tickers. Restart `scry serve --sync` to pick up new intervals.

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
| Sync loop process | Start/stop `scry serve --sync` or `scry sync` / `scry watch` |

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
