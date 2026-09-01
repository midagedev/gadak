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

`gadak config list` is the live catalog (48 paths). Five of them are not
on the Settings PUT document and are not on the Settings form: `notify`,
`updateCheck`, `attachmentCacheMB`, and the read-only discovery surfaces
`ui.tokens.catalog` / `ui.tokens.dim-catalog` (see below). PUT leaves
those untouched (it copies the live file, then overwrites the fields it
knows). The `ui.tokens.<axis>` and `ui.tokens.<axis>.<name>` paths are
also CLI-only — PUT's `ui` key is a whole-block replace with no per-axis
or per-token merge. The listing carries one `ui.tokens.<axis>.<name>`
row per axis, not one row per token.

### `appearance.theme`

Validation is **shape only**: empty or `system` (stored as the zero value),
or a lowercase identifier `[a-z0-9-]{1,32}`. Palette names belong to the
web so a new palette does not need a server deploy. The picker today
iterates `system`, `light`, `dark`, `ink`, `ember`
(`web/src/lib/theme.ts`).

### `ui.*` — user token overrides (colors + dimensions)

Catalog paths under one `ui` block in `config.json` — the three whole
blocks plus one merge path per axis and one scalar leaf
(`ui.tokens.<axis>.<name>`) per axis. They re-skin the
web/desktop UI without touching `app.css`: the server merges them into the
final CSS-variable map, the browser injects one unlayered `<style>` whose
rules mirror the palette cascade, and a blocking boot script replays a
localStorage cache so a customized user never sees the default palette
flash. Colors are hex-only (`#rgb` or `#rrggbb`); dimension axes are CSS
lengths — see [Dimension axes](#dimension-axes-spacing-layout-type)
below.

| Path | Shape | Meaning |
| --- | --- | --- |
| `ui.tokens` | `{\"colors\": {\"accent\": \"#7a4bd0\"}, \"spacing\": {\"row\": \"44px\"}, \"layout\": {\"sidebar\": \"280px\"}, \"type\": {\"heading\": \"24px\"}}` (colors also accept the flat `{\"accent\": …}`) | Overrides for **every** palette. Colors and the three dimension axes (`spacing`, `layout`, `type`) coexist here. A set **replaces the whole object** — see below. |
| `ui.tokens.colors` | `{\"accent\": \"#7a4bd0\"}` | The colors axis alone, as a **key-wise merge** — the recommended way to update one axis. |
| `ui.tokens.spacing` | `{\"row\": \"44px\"}` | The spacing axis alone, key-wise merge (rules judge the merged set). |
| `ui.tokens.layout` | `{\"sidebar\": \"280px\"}` | The layout axis alone, key-wise merge. |
| `ui.tokens.type` | `{\"heading\": \"24px\"}` | The type axis alone, key-wise merge. |
| `ui.tokens.<axis>.<name>` | `15px` / `#7a4bd0` / `44px` | One token as a **bare scalar** — the same key-wise merge as the axis path with a one-key object. `null` deletes that key. Unknown names save with a warning naming the catalog they missed (discover them with `ui.tokens.catalog` / `ui.tokens.dim-catalog`). |
| `ui.tokensByTheme` | `{\"dark\": {\"colors\": {\"accent\": \"#9a6be0\"}}}` | Overlay for one palette only (theme wins over `ui.tokens`). **Colors only** — dimension axes are refused here (see below). |
| `ui.dataColors` | `{\"label\": {\"urgent\": \"#c03030\"}, \"type\": {\"10007\": \"#d07020\"}, \"status\": {\"inprogress\": \"#7e5904\"}}` | Per-data inks: label chips, issue-type chips, status dots. |

Token tiers for **colors** (the color catalog is the single source — `gadak
config get ui.tokens.catalog` lists all 44 with per-palette values; the
dimension axes have their own table below). **Only parsing refuses**
(user decision 2026-08-25 — judgment warns and saves; the look is
yours): tier and contrast verdicts are warnings that carry the measured
number and the floor, so the fix does not need a round trip:

| Tier | Count | Rule on write |
| --- | --- | --- |
| locked | 10 | Warns and saves. Grounds and shell structure are palette authoring (GDK-789) — the warning says the build may re-derive them in an upgrade. |
| validated | 12 | Judged in **every palette the value can render in** — a `tokensByTheme` overlay is judged in its own palette only. Contrast/ΔEok/deuteranopia violations warn and save; the warning names the failing palettes and the `ui.tokensByTheme` scoping fix. |
| free | 22 | Hex shape only (lozenges, avatars, departments). |

What still **refuses**: a value that cannot parse (`"red"`, `"90"`) or a
wrong shape (unknown wrapper axis) — a value that cannot parse can never
render — and the derived `layout.docked-min` (below). Everything else
saves with its warning on stderr, and the write echoes the stored value:

```console
$ gadak config set ui.tokens '{"colors":{"bg-base":"#000000"}}'
gadak: ui: --color-bg-base is locked (page ground (lightest surface of the paper
ladder); light orange family (#f4efe4)) — applied, but locked tokens are palette
authoring: the build may re-derive them in an upgrade; per-palette opening is the
custom-palette scope (GDK-789)
{"colors":{"bg-base":"#000000"}}
$ echo $?
0

$ gadak config set ui.tokensByTheme '{"light":{"colors":{"status-reopen":"#f4efe4"}}}'
gadak: ui: status-reopen #f4efe4 as text on bg-base #f4efe4: contrast 1.00 < 4.5 —
applied, but it will read thin; pick a darker (text) or higher-chroma ink — fails
in palette light; to change one palette only, scope the override under
ui.tokensByTheme.<palette>
{"light":{"colors":{"status-reopen":"#f4efe4"}}}

$ gadak config set ui.tokens.colors '{"accent":"red"}'
gadak: ui tokens (palette dark): --color-accent: "red" is not a #rgb or #rrggbb hex color
$ echo $?
1
```

`dataColors` keys follow the repo-wide display-name ban, and the refusals
teach the right key kind: `label.*` is the label text itself, `type.*` is a
Jira **issue type id** (digits), `status.*` is a **status_category**
(`new` \| `inprogress` \| `done`).

```console
$ gadak config set ui.dataColors '{"status":{"In Progress":"#7e5904"}}'
gadak: ui.dataColors.status keys must be a status_category: new, inprogress, or done
(got "In Progress") — status display names localize per account
```

**Recovery.** A saved-but-regretted override is one command away:
`gadak config set ui.tokens '{}'` clears the whole block, and a `null`
value on an axis subpath deletes one key (`gadak config set
ui.tokens.colors '{"accent":null}'`). The leaf spelling of that delete is
`gadak config set ui.tokens.colors.accent null`. All three always work
from the CLI.

Unknown token names (and unknown `tokensByTheme` palettes) are **carried
with a warning, never refused** on every path — object
(`ui.tokens`, `ui.tokens.<axis>`, `ui.tokensByTheme`) and leaf
(`ui.tokens.<axis>.<name>`) alike — because a config written by a newer
gadak must keep loading; on this build the entry is simply not rendered.
The warning names the catalog it missed, so a typo is stored but never
silent.

```console
$ gadak config set ui.tokens '{"colors":{"accent":"#7a4bd0","future-name":"#111111"}}'
gadak: ui: --color-future-name is not in the color catalog; ignored (a newer
catalog may have renamed it)
{"colors":{"accent":"#7a4bd0","future-name":"#111111"}}
```

**Live reflection (no reload).** Every settings write — CLI `config set`,
the Settings dialog, another tab — moves `configVersion` (the disk identity
of `config.json`). The web's 500 ms ui-focus poll carries it; when it
moves, an open tab refetches `config.json` and re-applies the colors in
place. One honest limit: a scrim override would be hex-only and therefore
opaque — the shipped scrims are `rgb(...)` strings, so they stay locked to
palette authoring.

### Dimension axes (`spacing`, `layout`, `type`)

The `ui.tokens` wrapper carries three further axes (GDK-842): **CSS
lengths, not colors** — palette-agnostic by construction, so one value
renders in every palette and they never live under `ui.tokensByTheme`.
Nineteen tokens are recordable; one is locked (below). Values are px
lengths (`"44px"`, integer or one decimal) or, for the `*-line-height`
pairs, unitless one-or-two-decimal numbers (`"1.4"`).

| Axis | Tokens (default) | CSS var |
| --- | --- | --- |
| `spacing` | `row` (42px), `row-excerpt` (59px), `control` (32px), `control-sm` (24px) | `--spacing-*` |
| `layout` | `sidebar` (272px), `sidebar-narrow` (208px), `list-min` (390px), `detail-min` (438px), `detail-max` (720px), `overlay-max` (560px), `shell-max` (2200px) | `--layout-*` |
| `type` | `micro` (11px), `body` (13px), `title` (15px), `heading` (22px), each with a matching `…-line-height` (1.3 / 1.4 / 1.35 / 1.22), plus `terminal` (13px, range 9–24) | `--text-*` |

The dim catalog is the single source — `gadak config get
ui.tokens.dim-catalog` lists all 21 with tier, default, range and
relations as JSON (`gadak config list` carries the same pointer on the
`ui.tokens` row):

```console
$ gadak config get ui.tokens.dim-catalog | jq . | head -13
[
  {
    "axis": "spacing",
    "name": "control",
    "cssVar": "--spacing-control",
    "tier": "validated-range",
    "unit": "px",
    "default": "32px",
    "min": 28,
    "max": 40,
    "relations": [
      "--spacing-control-sm must stay ≤ --spacing-control (the small control rides inside the regular one)"
    ]
```

```console
$ gadak config set ui.tokens '{"spacing":{"row":"44px"}}'
{"spacing":{"row":"44px"}}
```

**Update one axis at a time with `ui.tokens.<axis>`** — the recommended
recipe. A subpath write is a **key-wise merge**: it updates the named keys
only and preserves every other key, the other axes (colors included), and
`ui.tokensByTheme`; a `null` value deletes its key, and `{}` changes
nothing. Validation judges the merged object, so a refused write (parse,
shape, docked-min) leaves the config exactly as it was. One token is that
merge as a scalar: `gadak config set ui.tokens.type.terminal 15px`
(hex values usually need shell quotes: `ui.tokens.colors.accent '#7a4bd0'`).

```console
$ gadak config set ui.tokens '{"colors":{"accent":"#7a4bd0"}}'
{"colors":{"accent":"#7a4bd0"}}
$ gadak config set ui.tokens.spacing '{"row":"44px"}'
{"row":"44px"}
$ gadak config get ui.tokens
{"colors":{"accent":"#7a4bd0"},"spacing":{"row":"44px"}}
$ gadak config set ui.tokens.spacing '{"row":null}'
{}
$ gadak config get ui.tokens
{"colors":{"accent":"#7a4bd0"}}
$ gadak config set ui.tokens.spacing '{"row":"wide"}'
gadak: ui tokens (dimensions): --spacing-row: "wide" is not a px length (integer or
one decimal, e.g. "44px")
```

The one-token spelling of that merge is the leaf path — a bare scalar, no
JSON object. `get` prints that one override, or `null` when there is none
(it does not invent the catalog default):

```console
$ gadak config set ui.tokens.type.terminal 15px
"15px"
$ gadak config set ui.tokens.spacing.row 44px
"44px"
$ gadak config get ui.tokens
{"spacing":{"row":"44px"},"type":{"terminal":"15px"}}
$ gadak config get ui.tokens.type.terminal
"15px"
$ gadak config set ui.tokens.type.terminal null
null
$ gadak config set ui.tokens.type.not-a-token 15px
gadak: ui.tokens.type.not-a-token is not a type token — discover names with `gadak config get ui.tokens.dim-catalog`
```

A write to `ui.tokens` itself **still replaces the whole object** — include
your colors in the same write
(`{\"colors\":{\"accent\":\"#7a4bd0\"},\"spacing\":{…}}`) or they are
dropped. That was already true for colors; with five axes it bites more
often, which is why the axis paths above are the default recipe.

**Validation: range + relations — warn and save.** Every
recordable token carries an inclusive min/max, and cross-token relations
judge the **effective** set (defaults fill whatever you did not override) —
`control-sm ≤ control`, `row-excerpt ≥ row + 8px`, `detail-max ≥
detail-min`, `sidebar-narrow ≤ sidebar`, and the type steps (neighboring
sizes ≥ 2px apart). These are judgments, not shapes: the value **saves**
and the warning carries the measured value and the bound, like the color
rules. An out-of-range value still participates in the relations, so one
write can land both warnings:

```console
$ gadak config set ui.tokens '{"spacing":{"row":"90px"}}'
gadak: ui: --spacing-row: 90px is outside its range 36px–56px — applied, but only
the range is tested
gadak: ui: --spacing-row-excerpt 59px breaks ≥ 90px + 8px (a row carrying a preview
line needs headroom) — applied, but the relation no longer holds
{"spacing":{"row":"90px"}}

$ gadak config set ui.tokens '{"spacing":{"control":"28px","control-sm":"30px"}}'
gadak: ui: --spacing-control-sm 30px breaks ≤ 28px (the small control rides inside
the regular one) — applied, but the relation no longer holds
{"spacing":{"control":"28px","control-sm":"30px"}}
```

Type-step warnings add the ladder teaching — the four size rungs are one
ladder, listed as it now renders, so the set that must move together is in
the message:

```console
$ gadak config set ui.tokens.type '{"micro":"14px"}'
gadak: ui: --text-micro: 14px is outside its range 11px–13px — applied, but only
the range is tested
gadak: ui: --text-body 13px breaks ≥ 14px + 2px (type steps closer than 2px read as
noise, not hierarchy) — applied, but the relation no longer holds; the ladder moves
together: micro 14px → body 13px → title 15px → heading 22px
{"type":{"micro":"14px"}}
```

What still refuses here: the **length grammar** (`"44"` without the unit,
`"1.4px"` for a line-height) and the derived `docked-min` — see below.

**`layout.docked-min` is derived and locked — the one dimension that still
refuses.** It is the dock floor `sidebar + list-min + detail-min`. A
stored value would be overwritten by the recomputation whenever any of the
three track tokens is overridden, so refusing is the honest answer; the
message keeps teaching the three tokens to set instead — the grid and the
dock/overlay regime switch stay one sum, so they cannot drift apart:

```console
$ gadak config set ui.tokens '{"layout":{"docked-min":"1200px"}}'
gadak: ui tokens (dimensions): --layout-docked-min is locked (derived: sidebar +
list-min + detail-min): a derived sum, not a runtime override — set those
three and it is recomputed
```

**Not per palette.** The palette-agnostic axes — dimensions and fonts —
under `ui.tokensByTheme` are refused at the same command: a per-palette
copy of a palette-free value could never render:

```console
$ gadak config set ui.tokensByTheme '{"dark":{"spacing":{"row":"44px"}}}'
gadak: ui.tokensByTheme.dark: spacing/layout/type/fonts apply to every
palette — set them under ui.tokens, not per theme
```

Unknown **axes** are refused up front (`axes are colors, spacing, layout,
type, fonts`); unknown token **names** — object and leaf paths alike — are
carried with a warning, never a refused save (same contract as colors; see
above).

**The fonts axis (GDK-896 R4) — `mono-terminal`, the terminal pane
stack.** `ui.tokens.fonts` carries font stacks where the other axes carry
lengths. One token today: `mono-terminal`, the CSS variable
`--font-mono-terminal` the xterm renderer reads when a terminal opens:

```console
$ gadak config set ui.tokens.fonts.mono-terminal "'JetBrains Mono', Menlo, monospace"
"'JetBrains Mono', Menlo, monospace"
```

The value grammar is the one axis with **no warn tier**: a comma-separated
list of 1–8 families, at most 256 characters in total, each family either a
bare identifier (`Menlo`, `ui-monospace`) or a quoted name (`'JetBrains
Mono'` — spaces only inside quotes). Anything else refuses, because the
stored value is spliced into a `<style>` element as text and the grammar
excludes every character that could carry CSS structure. Whether a family
exists on your machine stays the CSS fallback chain's business — there is
nothing to warn about. The change rides the same 500 ms ui-focus poll as
the other axes, but an **open** terminal does not re-read font CSS (canvas
renderer): the new stack applies from the next terminal open. The mobile
app does not inherit font tokens.

**Live reflection** rides the same 500 ms ui-focus poll as colors, and the
dimensions recouple the JS geometry that used to own them as constants:
the virtual list reads the row tokens, so a new `spacing.row` moves the
rows and the scroll window together, without a reload.

The mobile app does **not** inherit the dimension tokens — its touch
targets deliberately keep their own 44pt floor instead of the web row
heights.

### `PUT /api/v1/issues/settings/` is a replace

The path is `/api/v1/issues/settings/` (`apiBase` + `settings/`), not
`/api/v1/settings/`. A successful PUT assigns the writable fields from the
body and preserves credentials. Omitted assigned fields become empty
(projects, maps, members, groups, intervals, …). Omitted `features` is
normalized (`feed` defaults on; the other flags default off). Partial
edits are GET then PUT — the dialog's `save` and the theme picker both do
that.

Four keys are omit-to-preserve, so an older client cannot wipe them:
`fields` (discovery output), `confluence`, `appearance`, `ui` (color
overrides). `runtime`, `site`, and `hasCredential` on the body are ignored.
A present `ui` is still a whole-block replace — the per-axis merge paths
(`ui.tokens.<axis>`) and the scalar leaves (`ui.tokens.<axis>.<name>`) are
CLI-only. Because judgment violations save
(see above), a PUT carrying a `ui` block answers with the write-time
warnings of **that** write as a response-only `uiWarnings` array (same
violation objects: `token`, `rule`, `severity`, `measured`, `floor`,
`message`) — a client that never sees the CLI's stderr still learns why
the saved look will render the way it does. The key is absent when there
is nothing to say, and a client-supplied value is ignored.

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
| `ui.tokens` | `{colors: {token: hex}, spacing\|layout\|type: {token: length}, fonts: {token: stack}}` | _(absent)_ | `gadak config set ui.tokens` / Settings PUT `ui` (no form editor yet) | Live, no reload — color-token overrides for every palette plus the palette-agnostic dimension and font axes (see above). Only parse/shape/derived refuse; tiers, contrast, ranges and relations warn and save. A set replaces the whole object |
| `ui.tokens.<axis>` | `{token: hex, length, or font stack}` (axis = `colors`, `spacing`, `layout`, `type`, `fonts`) | _(absent)_ | `gadak config set ui.tokens.<axis>` (CLI only) | Live, no reload — key-wise merge into one axis: named keys update, other keys/axes survive; `null` deletes a key, `{}` is a no-op |
| `ui.tokens.<axis>.<name>` | hex, length, or font-stack scalar | _(absent)_ | `gadak config set ui.tokens.<axis>.<name>` (CLI only) | Live, no reload — one token into its axis as a bare scalar (`15px`, `#7a4bd0`, `Menlo, monospace`); `null` deletes that key; unknown names save with a warning |
| `ui.tokensByTheme` | `{palette: {colors: {token: hex}}}` | _(absent)_ | `gadak config set ui.tokensByTheme` / Settings PUT `ui` | Live, no reload — per-palette overlay, judged in that palette; colors only (dimension and font axes are refused) |
| `ui.dataColors` | `{label\|type\|status: {key: hex}}` | _(absent)_ | `gadak config set ui.dataColors` / Settings PUT `ui` | Live, no reload — per-data inks; `type` keys are issue type ids, `status` keys are status categories |
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
| `confluence` | object or absent | absent = wiki mirror off. `gadak init --local` writes the block scoped to `LOC` | Settings → Sources / `gadak config set confluence` | Next Confluence pass |
| `linear` | object or absent | absent = Linear source off. `apiKey` (personal API key, sent bare in the Authorization header) turns the source on; writes to Linear-owned keys route through it | edit `config.json` (no Settings surface yet) | Next `sync --source linear` |
| `linear.teamIds` | string[] | `[]` = every team the key can see; team UUIDs restrict the mirror scope | edit `config.json` | Next Linear pass |
| `devStatus` | bool | **false** | `gadak config set devStatus true` / `config.json` (not on Settings UI or Settings PUT) | Next sync; connected Cloud: mirror Jira's development-status API into `dev_links` (one extra request per issue). A gadak origin always fetches; `gadak dev link` / `dev scan` write the same table |
| `actor` | object or absent | absent = no acting identity; env `GADAK_ACTOR` (`slug\|display name`) overrides, and Claude Code sessions are auto-detected when both are unset | `gadak config set actor 'slug\|display name'` / `config.json` (not on Settings UI or Settings PUT; never team-exported) | Next origin session; writes to a gadak origin (local or paired) carry `X-Issuetap-Actor` and attribute to that agent account. Never sent to connected Jira/Linear |
| `locale` | string | _(empty)_ = English; `en` \| `ko` \| `ja` \| `de` | `gadak config set locale ko` / `config.json` (not on Settings UI or Settings PUT) | A gadak origin only: the origin's display-name language — status / issue-type / field names and agent aliases follow it; priority names stay English, like a live Cloud site. Changing it rebuilds the mirror on the next sync (display names are cached). A connected workspace ignores it: its language is the Atlassian account's |
| `confluence.spaces` | string[] | `[]` = every *global* space; personal spaces only if named (`internal/config/config.go`) | Settings → Sources / `gadak config set confluence.spaces` | Next Confluence pass |
| `terminal` | `{shell, workingDir, scrollback, cursorBlink}` | absent = all defaults (see below) | `gadak config set terminal` or `terminal.<leaf>` (not on Settings UI or Settings PUT; never team-exported — shell and workingDir are this machine's paths) | Next terminal session create; a block set replaces the whole object, a leaf set merges |

The space list *is* the scope: drop a space and the next Confluence pass
removes it from the mirror; add one and that space is fetched from the start
(per-space watermark, schema v19 — see
[`runbooks/confluence-space-scope.md`](runbooks/confluence-space-scope.md)).
Most keys apply without restart; `syncIntervalSec` and
`reconcileIntervalSec` are picked up on the next watch tick.

### `terminal` (behavior block)

Terminal behavior lives in the `terminal` block; terminal style (size,
family) lives in the design tokens. Both reach through the one `gadak
config` verb, so an agent can set every terminal preference from the shell —
the same promise a
Ghostty config file makes. For Ghostty users, the mapping:

| Ghostty (`~/.config/ghostty/config`) | gadak |
| --- | --- |
| `font-size` | `gadak config set ui.tokens.type.terminal 15px` (9–24px) |
| `font-family` | `gadak config set ui.tokens.fonts.mono-terminal "'JetBrains Mono', Menlo, monospace"` (1–8 comma-separated families, at most 256 characters, each a bare identifier or a quoted name; an open terminal picks the change up on its next open) |
| `scrollback-limit` (bytes) | `gadak config set terminal.scrollback 20000` (**lines**, 200–100000; `0` = 5000) |
| `cursor-style-blink` | `gadak config set terminal.cursorBlink true` |
| `command` | `gadak config set terminal.shell /bin/zsh` (absolute path; empty = `$SHELL`, else `/bin/sh`) |
| `working-directory` | `gadak config set terminal.workingDir /path` (empty = the workspace dir) |

`shell` and `workingDir` are consumed server-side at session create; they are
never carried in any API response, so a paired remote client cannot learn this
machine's shell paths. `scrollback`/`cursorBlink` ride the create response to
the pane — new sessions pick a change up immediately; a kept session that is
reattached inside the grace window keeps the defaults until its next fresh
session. A missing `workingDir` falls back to the default with one log line
naming the path. There is no renderer choice: the pane has one renderer, so
nothing is stored.

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
settings live in that other command) and it does not include the gadak-origin
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
| `$GADAK_HOME/origin/issuetap.db` | A gadak origin's persist file (`internal/origin/origin.go` `PersistRel`). SQLite (WAL); this is the record on a workspace whose origin is gadak's own tracker. Copy while gadak is not running (include `-wal`/`-shm`), or `sqlite3 <db> ".backup"`. Absent on a connected workspace. A sibling `origin/issuetap.yaml` is a one-shot seed if the db is missing. |
| `~/.gadak/profiles/<name>/` | Isolated config + mirror (and, on a gadak origin, persist) per profile |

Never write issue rows into the DB by hand — the next sync overwrites them. The
supported external write table is `enrichments` (see [PLUGINS.md](PLUGINS.md)).
