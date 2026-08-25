<p align="center">
  <img src="docs/media/wordmark-dark.svg#gh-dark-mode-only" width="380" alt="gadak">
  <img src="docs/media/wordmark-light.svg#gh-light-mode-only" width="380" alt="gadak">
</p>

<p align="center">
  <a href="https://github.com/midagedev/gadak/releases"><img src="https://img.shields.io/github/v/release/midagedev/gadak" alt="Latest Release"></a>
  <a href="https://github.com/midagedev/gadak/actions/workflows/ci.yml"><img src="https://github.com/midagedev/gadak/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache--2.0-blue" alt="License"></a>
</p>

<p align="center"><b>Follow the thread.</b></p>

<p align="center"><sub>English · <a href="README.ko.md">한국어</a></sub></p>

A local SQLite file of your Jira — so "which epic is stuck?" is one query, not an unaskable one.

gadak mirrors Jira *and* Confluence into one local SQLite file — issues,
comments, history, wiki pages — indexed together and searchable locally.
This window is where that work lives on your machine: triage
it in the [desktop app](docs/DESKTOP.md) or a browser tab, or let a coding
agent ask in plain SQL and point the same window at the answer. One binary,
one app, no gadak account.

**The mirror is a cache you can throw away.** On a connected workspace, if this
project stops tomorrow, you delete a directory and have lost nothing: Jira
stays the source of truth.

<p align="center">
  <a href="https://gadak.dev/demo/"><b>▶&nbsp; Open the live demo</b></a>
  &nbsp;—&nbsp; 534 issues, in your browser, right now.
  <br>
  <a href="CHANGELOG.md">Changelog</a>
  &nbsp;—&nbsp; what shipped.
</p>

A connected site needs one [API token](https://id.atlassian.com/manage-profile/security/api-tokens)
— it covers Jira and Confluence on the same site. A standalone workspace
needs no Atlassian account at all.

macOS app, CLI included:

```bash
brew install --cask midagedev/tap/gadak
```

CLI only — same UI in a browser tab via `gadak serve`:

```bash
brew install midagedev/tap/gadak-cli
```

Windows: from the [latest release](https://github.com/midagedev/gadak/releases/latest),
download `gadak_<version>_windows_amd64.zip` (or `windows_arm64`), unzip, put
`gadak.exe` on `PATH`. The desktop zip (`Gadak-<version>-windows-x64.zip`) is
unsigned — if SmartScreen or Smart App Control blocks it, that is a missing
signature, not a virus finding; use the CLI zip. Do not turn Smart App Control
off.

Connect to Jira:

```bash
gadak init && gadak sync && gadak serve
```

Or start with no tracker:

```bash
gadak init --standalone
gadak create "the thing I just noticed"
gadak serve
```

`gadak serve` prints the address — open `http://gadak.localhost:7777` and you
should see your issues. Linux tarball, pairing, the signed macOS dmg:
[Install](#install).

```bash
gadak sql "select epic_key, count(*) from issues_full where resolved_at is null
           and epic_key <> '' group by epic_key order by 2 desc"
```

That last query is the point: JQL has no `GROUP BY`. "Which epic is actually
stuck?" is not a hard question — it is an unaskable one, until the data is a
file. [`docs/RECIPES.md`](docs/RECIPES.md) has the rest.

Run that query now, nothing installed: [Datasette Lite loads the demo
snapshot in this
tab](<https://lite.datasette.io/?url=https%3A%2F%2Fraw.githubusercontent.com%2Fmidagedev%2Fgadak%2Fmain%2Fexamples%2Fdemo.db#/demo?sql=select+epic_key%2C+count(*)+from+issues_full+where+resolved_at+is+null+and+epic_key+%3C%3E+''+group+by+epic_key+order+by+2+desc>)
and the SQL runs client-side.

Measured 2026-08-23 against a live Cloud site (7,166 issues; medians, CLI
startup included — [method, the re-measurement history, and the losing
rows](docs/BENCHMARKS.md)):

| Question | REST API | `gadak` | |
| --- | ---: | ---: | ---: |
| Simple filter, 100 issues | 374 ms | 17 ms | 23× |
| One issue with its full history | 687 ms | 29 ms | 24× |
| Free-text search | 504 ms | 22 ms | 23× |
| A count over the change history | not expressible — ≈ 109 min of crawling | 14 ms | — |

The last row is the point: past a page size, JQL answers stop being slow and
start being unaskable. `GROUP BY` is the same story — on the 2026-08-15 corpus
the epic rollup measured 3,924 ms across 7 API pages against one 24 ms query;
this run the REST aggregation matched no rows at all, because the project's
epic shape had changed, so that comparison stands on the August 15 numbers.

And the other side: the first full sync measured 26.4 s for 534 issues and
7.2 min for 2,865 ([method and the losing rows](docs/BENCHMARKS.md)), every
watch tick costs ~4.7 s on a quiet site even when nothing changed, and the
mirror trails Jira by one sync interval.

<details>
<summary>▶ 90-second tour of the paper list (GIF, 7 MB)</summary>

<p align="center">
  <img src="docs/media/web-demo.gif" alt="The paper list narrows as you type; an issue opens with labels, priority and a reopen badge; documents and epics sit in the same window" width="900">
  <br>
  <sub>Generated from <a href="e2e/demo/web-demo.spec.ts">e2e/demo/web-demo.spec.ts</a> against the demo snapshot.</sub>
</p>

</details>

> **Status: 0.17, still 0.x.** Sync, read API, write-through, desktop, web, CLI, and MCP are verified against a live site. [`CHANGELOG.md`](CHANGELOG.md).

## Why

Jira search is a network round trip, and the wiki is a second search. An
agent asked "what did we already fix, and what did we decide?" pages two
REST APIs. Same cause: the data is not a file.
[`docs/CONCEPT.md`](docs/CONCEPT.md) · [`docs/PAIN_POINTS.md`](docs/PAIN_POINTS.md).

⌘K is the one index — titles, bodies, comments, issues and pages. The chips
on the list do not apply. That is why a comment-only word still finds the row.

<p align="center">
  <img src="docs/media/search.gif" alt="A Project chip is on the list; ⌘K opens the palette, a comment-only word is typed, and All search fills with rows from other projects, each labelled Comment match with a snippet" width="900">
  <br>
  <sub>Generated from <a href="e2e/demo/search-demo.spec.ts">e2e/demo/search-demo.spec.ts</a> against the demo snapshot.</sub>
</p>

## Two surfaces, one store

| | For | Looks like |
| --- | --- | --- |
| **App + Web UI** | all-day triage | [desktop app](docs/DESKTOP.md) (no port) or `gadak serve`. `j`/`k` walk, `x` selects, `s`/`a`/`l`/`c` change status, assignee, labels, or comment from the list. |
| **CLI + SQL** | agents, scripts | `gadak issue`, `gadak search` (FTS, `--jql`, a Jira URL, `--explain`), `gadak sql`, plus the file |

Writes go through the origin, then the mirror refreshes. App and web: comment,
transition, assign, labels, priority, title. CLI: `create` (single or
`--batch`), `attach`, `edit`, `comment`, `transition` (`--resolution`),
`assign`, `link`, `dev link` / `dev scan`, `fields --apply`,
`issue --editmeta`, `project create`, and `page create` / `page edit` /
`page comment` for the wiki (pages, titles, bodies and comments all through
the origin). Hierarchy, `item_refs`, attachments: [`docs/CONCEPT.md`](docs/CONCEPT.md#two-surfaces).
The window keeps one paper metaphor across four palettes — `light`, a
neutral-cool `dark`, blue-black `ink`, and warm `ember`. The theme follows
the system unless you pick one, and it belongs to the workspace, not the
browser: `gadak config set appearance.theme ink`.

<p align="center">
  <img src="docs/media/tokens.gif" alt="A terminal sets ui.tokens and ui.dataColors and the open tab retints live — accent, chips and breakdown colors change with no reload; a write to the locked bg-base is refused with the reason" width="900">
  <br>
  <sub>Colors are config: <code>ui.tokens</code> / <code>ui.dataColors</code> flow from the CLI into an open tab with no reload, and the keys a palette owns refuse an override instead of silently breaking the paper. Generated from <a href="e2e/demo/tokens-demo.spec.ts">e2e/demo/tokens-demo.spec.ts</a>.</sub>
</p>

And two surfaces is not a closed list. Reading the mirror is one binary
call (`gadak search --json`, ~20 ms), and opening anything in the app is
one URL (`gadak://view?issue=…` — [the scheme](docs/DESKTOP.md)), so
whatever can do those two things becomes a surface. A launcher, say:

<p align="center">
  <img src="docs/media/raycast.gif" alt="Raycast searches the local gadak mirror as you type — a text query shows the matched snippet in bold with a field tag, then typing the bare issue key finds that issue, and Enter opens it in the Gadak app through a gadak:// deep link" width="800">
  <br>
  <sub>Each keystroke is one <code>gadak search --json</code>; Enter is the deep link. A saved view travels the same way — <code>gadak views open</code> prints its link.</sub>
</p>

That launcher exists: a Raycast extension that searches issues and wiki
documents as you type, [submitted to the Raycast Store](https://github.com/raycast/extensions/pull/30297).
Until the review lands, one command installs it from the binary you already
have (embedded, no checkout):

```sh
gadak raycast install
```

The macOS app has the same install as a button — **Settings → Integrations**
lists Raycast, the agent skill and MCP, shows what is already installed, and
runs the exact command it prints. Building on the extension itself:
[`contrib/raycast/`](contrib/raycast/). And with no extension at all, a
Raycast Quicklink pointed at `gadak://view?issue={argument}` covers the
open-by-key half.

## What's covered

Connected talks to Atlassian Cloud. Standalone (from 0.16) is a workspace
with no Atlassian account — a minimal Jira origin that travels with the
app. The mirror is a cache either way; every write goes through the origin.
On standalone the durable file is the origin's persist file — issuetap.db
in the workspace's origin folder (SQLite, WAL). Copy it while gadak is not
running (include the `-wal`/`-shm` sidecars), or
`sqlite3 origin/issuetap.db ".backup dest.db"`.

| | Connected (Atlassian Cloud) | Standalone (from 0.16) |
| --- | :---: | :---: |
| Issue read and search (FTS, JQL, SQL) | ✅¹ | ✅¹ |
| Create, comment, transition, assignee, labels, priority | ✅ | ✅ |
| Due date, description, custom-field edits (from 0.16) | ✅² | ✅² |
| Hierarchy | ✅³ | ✅³ |
| Wiki documents | ✅⁴ | ✅⁵ |
| Attachments | ✅ | ✅ |
| History / time in status | ✅⁶ | ✅⁶ |
| Agent surfaces (skill, MCP, SQL) | ✅ | ✅ |
| Boards and sprints | —⁸ | —⁸ |
| Dashboards | — | — |
| Jira notifications | —⁷ | —⁷ |

1. SQL and FTS are local. `--jql` / a Jira URL maps a documented subset onto the in-memory filter; clauses gadak cannot express are listed, never dropped. Sprint-by-name, `WAS`, cross-field `OR`, and custom fields are among the refusals; numeric `sprint =` / `sprint in` and `sprint in openSprints()` are in the subset ([decision 0007](docs/decisions/0007-jql-subset.md)).
2. Dedicated endpoints for due date and description. Custom fields: kinds `text`, `number`, `date`, `option`, `user`, `multi_option` / `version_array`, gated by the issue's editmeta and the configured field allowlist. Cascading selects and textarea custom fields have no editor.
3. Epic grouping (`epic_key`, nearest hierarchy-level-1 ancestor) is first-class. Setting a parent is CLI `create --parent` / `edit --parent` only — there is no REST `PUT {key}/parent`. Sub-task create-meta flags are not surfaced, so create cannot tell that a type requires a parent.
4. Confluence Cloud is mirrored; page create, edit (title/body) and page comments write through it — `gadak page create|edit|comment`, `POST pages/`, `PUT pages/{id}/edit`, `POST pages/{id}/comment`.
5. Pages sync from the in-process origin. `gadak page create|edit|comment` and the REST verbs work here too; the UI has a page comment composer but no page editor yet.
6. Changelog is mirrored. Time in status is computed from `status_changed_at`, not stored as a column.
7. Jira's notification inbox, rules, and email are not mirrored. gadak has its own watch-feed OS alerts on macOS and Linux.
8. No board UI and no sprint column on the list. Sprint fields (`sprint_id` / `sprint_name` / `sprint_state`) are in the mirror; SQL and JQL (`sprint =` / `sprint in openSprints()`) can query them. The `versions` catalog and `fix_version_ids` join the same way.

**Linear.** A Linear workspace mirrors and writes through the same
verbs: add a `"linear"` block (`apiKey`, optional `teamIds`) to the
workspace's `config.json` and run `gadak sync --source linear`. Writes
route by the mirror's source for the key — comment, transition (the
team's workflow states, id-keyed), summary/priority/due-date edits,
assign/unassign, and file attachments all pass through Linear's API and
refresh the mirror row. Not yet: label edits, clearing a due date, and
state history (`status_changed_at` stays NULL) each refuse honestly
rather than half-applying; inline comment media attaches the file and
drops only the body embed. Field mapping:
[`internal/linear/MAPPING.md`](internal/linear/MAPPING.md).

## For agents

This is half the reason gadak exists. Reference: **[docs/MIRROR.md](docs/MIRROR.md)**.
One paste per host: [`docs/AGENT_SETUP.md`](docs/AGENT_SETUP.md).

```bash
gadak skill install
```

Schema and query patterns, no extra process. For hosts without a shell
(Claude Desktop):

```bash
gadak mcp install claude
```

Pins this binary and workspace into the registration.

`gadak init` and `gadak install-cli` install that skill automatically when
`~/.claude` already exists. A file gadak did not write is left in place.

<p align="center">
  <img src="docs/media/claude-drive.gif" alt="A live Claude Code session beside the app: asked to retint the workspace and build a chart dashboard, it runs gadak config and dashboards save, and the open tab picks up the new colors and renders the dashboard" width="900">
  <br>
  <sub>What the skill buys: a live Claude Code session drives the same workspace you are looking at — colors and a chart dashboard land in the open tab, no reload. Recorded from <a href="tools/tapes/claude-drive.tape">tools/tapes/claude-drive.tape</a>.</sub>
</p>

Both installs (and the Raycast one) are also buttons in the macOS app,
with install state shown honestly: **Settings → Integrations**.

Setup is not a screen an agent has to click, either. Every field the
settings dialog edits is a CLI verb over the same validation:

```bash
gadak config list
```

Every editable path and its value.

```bash
gadak config set appearance.theme ink
```

Per workspace, applied live.

SQL answers; the window presents. Filter on `status_category` /
`priority_rank` (1 = most urgent, 0 = unset), never on a display name —
Jira translates those per account, so `priority = High` is silently zero
rows on a Korean-language site. `--jql` is Jira's own language and stores
the literal, so prefer `gadak sql` when the filter is a rank or category:

```bash
gadak sql --no-header "select key from issues_full where status_category = 'inprogress'
                       order by status_changed_at asc limit 5" | gadak views open --keys -
```

If you already have JQL, the clauses land as chips. This one keys on a
project key and emptiness, not a localized name:

```bash
gadak views open --jql 'project = NMA AND resolution is EMPTY'
```

<p align="center">
  <img src="docs/media/agent.gif" alt="A terminal pipes gadak sql into gadak views open --keys - and the running app snaps to those five keys; then gadak views open --jql lands the same window on project, priority and unresolved chips" width="800">
  <br>
  <sub><code>gadak views open</code> writes a one-shot hash; the running app or serve tab applies it. The recording adds a priority clause — in <code>--jql</code> a priority or status name is matched as the literal string your Jira stores, which is localized, so the example above leaves it out. Generated from <a href="e2e/demo/agent-demo.spec.ts">e2e/demo/agent-demo.spec.ts</a>.</sub>
</p>

When the answer is a wall rather than a list, author a dashboard — one HTML
document plus registered datasources, rendered sandboxed in the web tab:
**[docs/DASHBOARDS.md](docs/DASHBOARDS.md)**.

<p align="center">
  <img src="docs/media/dashboards.gif" alt="A terminal saves a dashboard — one HTML file plus four datasources over the mirror — and the web tab renders the triage wall: status counters and the top open issues by priority; a second save swaps the open frame in place" width="900">
  <br>
  <sub><code>gadak dashboards save</code> registers the document and its datasources; the host runs the queries and pushes rows in, and a re-save swaps an open frame in about a second. Charts come from a locally served uPlot — no CDN, no CSP widening. Generated from <a href="e2e/demo/dashboards-demo.spec.ts">e2e/demo/dashboards-demo.spec.ts</a>.</sub>
</p>

For hosts without a shell (Claude Desktop), the same mirror is an MCP
server. Ask the thing Jira cannot answer at all, because the wiki is a
second search: "what do we know about X?" One index holds both, so the
answer can put a ticket and the design doc that drove it in the same
sentence.

<p align="center">
  <img src="docs/media/mcp.gif" alt="Claude Code registers gadak as an MCP server, is asked to search Jira and the wiki for idempotency, calls gadak, and answers with an issue and the Confluence brief that drove it" width="800">
  <br>
  <sub>Five tools; no writes to the mirror or to Jira. A host with a shell can use <code>gadak sql</code> instead. Setup: <a href="docs/MCP.md">docs/MCP.md</a>.</sub>
</p>

`gadak views open` is the "open in gadak" verb; `gadak open KEY` leaves for
Jira. The list box takes the same JQL paste as `gadak search --jql`; clauses
gadak cannot express are listed, never dropped. What JQL still cannot ask
stays in `gadak sql` and [`docs/RECIPES.md`](docs/RECIPES.md). `gadak sql`
opens the file `mode=ro`; MCP's `gadak_query` rejects anything that is not a
SELECT. [`gadak api`](docs/AGENT_ACCESS.md) is the pass-through for endpoints
the mirror does not model — read-only unless `--write`, never on MCP.

**An agent that reads your mirror sends what it reads to whatever model it
talks to.** gadak itself sends nothing ([`SECURITY.md`](SECURITY.md)). Scope
the mirror to what the agent should see. When gadak *does* touch the network
— sync, writes, the pairing model that lets a tailnet or a whole team share
one workspace — [`docs/NETWORK.md`](docs/NETWORK.md) walks every connection
and its off switch.

## Install

Atlassian Cloud, or (from 0.16) a standalone workspace with no Atlassian account.
A connected site needs one [API token](https://id.atlassian.com/manage-profile/security/api-tokens)
— it covers Jira and Confluence on the same site.

**1. The [desktop app](docs/DESKTOP.md).**

macOS: download `Gadak-<version>-arm64.dmg` from the
[latest release](https://github.com/midagedev/gadak/releases/latest), drag to
Applications. Signed and notarized. First launch walks through site, email,
token, and projects. The CLI is inside the bundle; macOS does not put an app
on your `PATH`:

```bash
/Applications/Gadak.app/Contents/Resources/bin/gadak install-cli
```

**2. The CLI**, on Linux, Windows, or for the same UI in a browser tab.

macOS + Linux:

```bash
brew install midagedev/tap/gadak-cli
```

Windows: from the [latest release](https://github.com/midagedev/gadak/releases/latest),
download `gadak_<version>_windows_amd64.zip` (or `windows_arm64`) and
`checksums.txt`. Unzip, put `gadak.exe` on `PATH`. This is the reliable
Windows route. How to check the sha256:
[`docs/WINDOWS-SIGNING.md`](docs/WINDOWS-SIGNING.md).

The desktop zip (`Gadak-<version>-windows-x64.zip`, or `windows-arm64`) is
unsigned (signing is [GDK-211]). If Windows shows **Windows protected your PC**
or **Smart App Control blocked an app that may be unsafe**, that is a missing
signature, not a virus finding — use the CLI zip above. Do not turn Smart App
Control off.
Install: [`docs/INSTALL.md`](docs/INSTALL.md#desktop-app-windows).
Code signing policy (why the warning, SHA256):
[`docs/WINDOWS-SIGNING.md`](docs/WINDOWS-SIGNING.md).

No Atlassian account:

```bash
gadak init --standalone
gadak create "the thing I just noticed"
gadak serve
```

`gadak serve` prints the address — open `http://gadak.localhost:7777` and you
should see your issues.

Already have Jira:

```bash
gadak init && gadak sync && gadak serve
```

`gadak serve` prints the address — open `http://gadak.localhost:7777` and you
should see your issues.

**Pair another machine.** Home `gadak serve` is the origin. On the home
machine, mint an offer (stdout is one offer line):

```bash
gadak pairing mint --label laptop
```

On the remote, paste the offer:

```bash
gadak --workspace laptop init --pairing-code-stdin
```

Confirm with `gadak --workspace laptop status` (paired with "laptop").
`gadak pairing list` is the token table on home and one status line on the
remote. `gadak pairing revoke laptop` is home only.

`_home` is this machine's routing token, not a device (`revoke` refuses it; `mint --label _home` rotates). Same verbs on the remote; a `pairing:` error is the whole message. `--profile` is an alias of `--workspace`. The gate is in [`SECURITY.md`](SECURITY.md).

A Scoop manifest lives in [`contrib/scoop`](contrib/scoop). The bucket
is not published and `scoop install` has not been run on a Windows machine
([`docs/INSTALL.md`](docs/INSTALL.md#scoop-windows-cli)).

Linux without Homebrew: from the
[latest release](https://github.com/midagedev/gadak/releases/latest),
download `gadak_<version>_linux_amd64.tar.gz` (or `linux_arm64`) and
`checksums.txt`. One archive is the whole install — the web UI is inside
the binary. Replace `<version>` with the tag from that release, without
the leading `v`. Verify, unpack, and put `gadak` on `PATH`:

```bash
sha256sum --ignore-missing -c checksums.txt
tar -xzf gadak_<version>_linux_amd64.tar.gz
```

```bash
gadak serve
```

`gadak serve` prints the address — open `http://gadak.localhost:7777` and you
should see your issues.

Optional, to survive reboot (`systemd --user`):

```bash
gadak install-service
```

Arch Linux: a checked `PKGBUILD` lives in
[`contrib/aur/gadak-bin`](contrib/aur/gadak-bin) — `makepkg -si` there. Not in
the AUR yet; upstream registration is closed
([`docs/INSTALL.md`](docs/INSTALL.md#arch-linux)).

On [Omarchy](https://omarchy.org), the bar can answer the one question no
cloud plugin can — what changed in *your* mirror. A shell-plugin widget in
[`contrib/omarchy`](contrib/omarchy) shows `open·stuck` straight from the
local mirror (no token, no network) and click-opens the app. Run once on a
real guest.

<details>
<summary>▶ The widget on a real Omarchy guest (PNG) — the bar badge is <code>gadak sql</code>'s own numbers</summary>

<p align="center">
  <img src="docs/media/omarchy-widget.png" alt="An Omarchy desktop: the Waybar badge reads 368·201, the terminal below shows gadak sql --json returning open 368 and stuck 201 for the same mirror, and the gadak web app is open after clicking the badge" width="900">
  <br>
  <sub>Captured on the Arch + Hyprland verification guest (<a href="contrib/omarchy/README.md">contrib/omarchy/README.md</a>).</sub>
</p>

</details>

Install script, release archive, source, Docker, wiki mirroring, workspaces,
upgrades: **[`docs/INSTALL.md`](docs/INSTALL.md)**.

## The rest

**Making it yours.** Two axes, no forking: [`docs/EXTENDING.md`](docs/EXTENDING.md).
Config: [`docs/CONFIGURATION.md`](docs/CONFIGURATION.md). Enrichments:
[`docs/PLUGINS.md`](docs/PLUGINS.md).

**How it works.** One binary, one SQLite file; incremental sync plus a
reconcile pass. [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md). Why not an
extension or Forge app: [`docs/decisions/0003-local-process.md`](docs/decisions/0003-local-process.md).

**Good fit / bad fit.** Daily search latency, an agent over tracker *and* wiki,
offline reads — yes. Boards, admin, a page editor in the UI, or a minute of
staleness — stay in Jira. CLI and REST already write wiki pages.
[`docs/CONCEPT.md`](docs/CONCEPT.md#good-fit-bad-fit).

**How it compares.** jira-cli talks to the live API per command. Linear is a
different tracker, and also a gadak source (see above). Rovo MCP searches both sources too, but it is hosted: no
aggregate, no offline, and every call spends tokens.
[`docs/FAQ.md`](docs/FAQ.md#how-it-compares).

**More sources later.** Confluence proved the spine is neutral. Next source,
ranked by demand: [`docs/project/ROADMAP.md`](docs/project/ROADMAP.md#more-sources-later).

## Documentation

- [`CHANGELOG.md`](CHANGELOG.md) — what shipped
- [`docs/INSTALL.md`](docs/INSTALL.md) · [`docs/DESKTOP.md`](docs/DESKTOP.md) — install, first run, the desktop app
- [`docs/MIRROR.md`](docs/MIRROR.md) · [`docs/MCP.md`](docs/MCP.md) · [`docs/AGENT_SETUP.md`](docs/AGENT_SETUP.md) — SQL, CLI, REST, MCP, one paste per host
- [`docs/RECIPES.md`](docs/RECIPES.md) — questions JQL cannot ask, as SQL
- [`SECURITY.md`](SECURITY.md) · [`docs/FAQ.md`](docs/FAQ.md) · [`MAINTENANCE.md`](docs/MAINTENANCE.md) — threat model, site load, who maintains this
- [`docs/EXTENDING.md`](docs/EXTENDING.md) · [`docs/PLUGINS.md`](docs/PLUGINS.md) — fitting gadak to your team
- [`docs/project/STATE_OF_PLAY.md`](docs/project/STATE_OF_PLAY.md) · [`docs/CONCEPT.md`](docs/CONCEPT.md) · [`docs/PAIN_POINTS.md`](docs/PAIN_POINTS.md)
- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) · [`docs/project/UX_PRINCIPLES.md`](docs/project/UX_PRINCIPLES.md)
- [`docs/README.md`](docs/README.md) — the rest of the docs

## Who makes this

One person, currently. Weigh that — and the other side: the mirror is a
disposable cache of your own Jira, the 0.x contract is the three promises
in [data-model.md](specs/000-product/data-model.md) (`issues_full` and the
RECIPES queries, `gadak sql` stdout, and `gadak views open --keys -`), the
license is Apache-2.0, and the file is plain SQLite. Hard questions:
[`docs/FAQ.md`](docs/FAQ.md). What you do not have to take on trust, each with
the command that checks it: [`PROMISES.md`](docs/PROMISES.md).

## Contributing and feedback

[`CONTRIBUTING.md`](.github/CONTRIBUTING.md) — and
[`docs/project/GOOD_FIRST_ISSUES.md`](docs/project/GOOD_FIRST_ISSUES.md) to start. Bug reports
need your Jira deployment type (Cloud), the gadak commit, and the command you
ran. Never paste real issue data, tokens, or site URLs into a public issue.

Commit `GDK-nnn` keys resolve on the [public backlog](https://gadak.dev/backlog/).
To file something, open a [GitHub issue](https://github.com/midagedev/gadak/issues)
— the maintainer mirrors it into the backlog.

Using gadak with an agent and hitting friction? Open an issue
with the question you asked and what the agent did.

## License

Apache-2.0. See `LICENSE` and `NOTICE`.

[GDK-211]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-211
