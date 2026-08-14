# Configuration reference

gadak stores its configuration in `~/.gadak/config.json` (mode `0600`). A profile
(`gadak --profile x` or `GADAK_PROFILE=x`) moves both the file and the mirror under
`~/.gadak/profiles/<name>/`.

A running `gadak serve` also mounts every sibling profile as a **workspace**
under `/w/<name>/` — full API, opened lazily on first request. The sidebar
shows a switcher when more than one exists. `GET /api/v1/workspaces` lists
them (name, site, projects — never credentials). Background sync, OS
notifications, and the update check stay on the profile `serve` was started
with; workspace mirrors sync on demand.

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
| `site` | string (URL) | _(empty)_ | Credential dialog / `PUT credential/` / `gadak init` | Immediate for writes & deep links |
| `email` | string | _(empty)_ | Credential dialog / `gadak init` | Immediate |
| `token` | string | _(empty)_ | Credential dialog / `gadak init` | Immediate; **never** returned by `settings/` or `config.json` |
| `tokenVerifiedAt` | string (RFC3339) | _(empty)_ | Set by successful credential verify | Read-only side effect |
| `tokenOwner` | string | _(empty)_ | Set by successful credential verify | Read-only side effect |
| `projects` | string[] | `[]` (empty = every project this account can see) | Settings → Sync / `gadak init` | Next sync / list scope; UI reload after save |
| `fields` | FieldSpec[] | `[]` | Auto on first full sync / `gadak fields --apply` (read-only on Settings GET as `fieldSpecs`) | Next sync ingest; `fieldUsage` on Settings is project→alias fill counts |
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
| `syncIntervalSec` | int (seconds) | `0` → **60** | Settings → Sync (presets / custom) | **After restart** of `gadak serve` |
| `reconcileIntervalSec` | int (seconds) | `0` → **3600** | Settings → Sync (presets / custom) | **After restart** of `gadak serve` |
| `notify` | bool | **true** when absent | `config.json` (not on Settings UI) | Next watch-loop tick; OS desktop alerts for new personal-feed events |
| `updateCheck` | bool | **true** when absent | `config.json` (not on Settings UI) | Next `sync` / `status` / `serve` start; once-per-day GitHub release lookup (cached under the profile dir). Set `false` to opt out — no outbound traffic beyond Jira |
| `confluence` | object or absent | absent = wiki mirror off | Settings → Sources | Next Confluence pass |
| `confluence.spaces` | string[] | `[]` = every *global* space; personal spaces only if named (`internal/config/config.go`) | Settings → Sources | Next Confluence pass |

The space list *is* the scope: drop a space and the next Confluence pass
removes it from the mirror; add one and that space is fetched from the start
(per-space watermark, schema v19 — see
[`runbooks/confluence-space-scope.md`](runbooks/confluence-space-scope.md)).
Most keys apply without restart; `syncIntervalSec` and
`reconcileIntervalSec` need a restart of `gadak serve`.

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
`0` always means “use default”.

### Why intervals need a restart

`internal/sync.Watch` creates its tickers once at loop start from the `Config`
passed in. `gadak serve` starts that loop by default when a credential is
configured (`--no-sync` opts out). `PUT settings/` updates the on-disk file and
the server’s atomic config for everything else (members, group rules,
features…), but it does **not** rebuild the Watch tickers. Restart `gadak serve`
to pick up new intervals.

### OS desktop notifications

When the watch loop runs (`gadak serve` with a credential, or `gadak sync
--watch`), each successful cycle may fire **one** bundled OS notification for
new personal-feed events since `sync_state.last_notified_at` (macOS
`osascript`, Linux `notify-send`; Windows is a quiet no-op). The body carries
the issue title only — never comment text. Set `"notify": false` in
`config.json` to opt out. Notifications never write `feed_reads`.

### Update check

Once a day, `gadak sync` (after a successful run), `gadak status`, and `gadak
serve` may query GitHub's public releases API for this project and cache the
answer under the profile directory (`update-check.json`). The request carries
no account identifiers. Dev builds (`0.0.0-dev`) never check. Network errors
and rate limits are silent. Set `"updateCheck": false` in `config.json` to
disable the lookup entirely (restores outbound traffic to Jira only).

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
views, field map, and group rules. Export is **whitelist-only**: new `Config`
fields never leave the machine until someone explicitly adds them to the share
list. Credentials and per-machine prefs are never included.

| Shared (export / import) | Not shared |
| --- | --- |
| `projects` | `site`, `email`, `token` |
| `fields`, `fieldMap`, `bodyFields`, `editableFields` | `account_id`, `tokenOwner`, `tokenVerifiedAt` |
| `groupRules`, `groupLabels`, `groupColors`, `productByGroup` | `syncIntervalSec`, `reconcileIntervalSec` |
| `features`, `qaDashboardUrl`, `staleThresholdHours` | `notify`, `updateCheck`, `attachmentCacheMB` |
| `members` only with `export --with-members` (emails; stderr warns) | personal machine intervals / notification prefs |
| saved views (`name` + `config`; new ids on import) | view `id` / timestamps |

Import **merges** by default (fill empty settings keys; add views only when the
name is new). `--overwrite` replaces conflicts. Prefer
`gadak team import FILE --dry-run` first. A file that contains credential keys
(`site` / `token` / …) is rejected — do not hand-edit secrets into a share file.

---

## What you must still edit by hand (or outside Settings)

| Concern | How |
| --- | --- |
| Jira site / email / API token | Credential dialog, `PUT credential/`, or `gadak init` — **not** settings PUT |
| Unattended setup (agents, CI, provisioning) | `gadak init` flags/env — see below |
| Profile selection | CLI `--profile` / `GADAK_PROFILE` (separate home directory) |
| `GADAK_HOME` override | Environment variable |
| Binary version string in UI | `server.Version`, wired from `cmd/gadak` ldflags by goreleaser (`main.version`); dev builds show `0.0.0-dev` |
| Team views / field map / group rules (between people) | `gadak team export` / `gadak team import` (see above) |
| Sync loop process | Start/stop `gadak serve` (default when credentialed; `--no-sync` opts out) or `gadak sync --watch` |
| Keep serve across reboots | `gadak install-service` (launchd / systemd user); `--uninstall` removes it |
| OS desktop notifications | `"notify": false` in `config.json` to disable (default on) |

There is no remaining day-to-day operational knob that only lives in the JSON
file: intervals, projects, features, field maps, teams, and members are all on
the Settings surface. Direct file edit still works for automation and recovery.

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
| `$GADAK_HOME/gadak.db` | SQLite mirror |
| `~/.gadak/profiles/<name>/` | Isolated config + mirror per profile |

Never write issue rows into the DB by hand — the next sync overwrites them. The
supported external write table is `enrichments` (see [PLUGINS.md](PLUGINS.md)).
