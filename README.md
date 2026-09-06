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

gadak mirrors Jira *and* Confluence — issues, comments, history, wiki pages —
into one SQLite file on your machine, indexed together and searchable with no
network. Triage it in the [desktop app](docs/DESKTOP.md) or a browser tab, or
let a coding agent ask in plain SQL and point the same window at the answer.
One binary, no gadak account.

**The mirror is a cache you can throw away.** If this project stops tomorrow,
you delete a directory and have lost nothing: Jira stays the source of truth.

<p align="center">
  <a href="https://gadak.dev/demo/"><b>▶&nbsp; Open the live demo</b></a>
  &nbsp;—&nbsp; 534 issues, in your browser, right now.
  <br>
  <a href="CHANGELOG.md">Changelog</a>
  &nbsp;—&nbsp; what shipped.
</p>

## Install

macOS app, CLI included:

```bash
brew install --cask midagedev/tap/gadak
```

CLI only — the same UI in a browser tab via `gadak serve`:

```bash
brew install midagedev/tap/gadak-cli
```

Connect to Jira, then open the address `gadak serve` prints
(`http://gadak.localhost:7777`):

```bash
gadak init && gadak sync && gadak serve
```

A Jira site needs one [API token](https://id.atlassian.com/manage-profile/security/api-tokens);
it covers Jira and Confluence on the same site. **You pick what it mirrors**:
`--projects` for Jira, `--spaces` for the wiki, which stays off until you name
them. No Atlassian account? `gadak init --local` starts a workspace on the
built-in tracker, and `gadak --workspace <new> migrate --from <old>` later carries a
synced mirror onto it — or into a Linear team with `--to linear`.

**Windows:** the desktop app is on the [Microsoft Store](https://apps.microsoft.com/detail/9NZW91TXH36G) — the
Store signs it, so neither SmartScreen nor Smart App Control objects. For the
CLI, take `gadak_<version>_windows_amd64.zip` (or `arm64`) from the
[latest release](https://github.com/midagedev/gadak/releases/latest), unzip,
put `gadak.exe` on `PATH`. The release's desktop zip
(`Gadak-<version>-windows-x64.zip`) stays unsigned — a SmartScreen block is a
missing signature, not a virus finding ([why](docs/WINDOWS-SIGNING.md)); if it
blocks, install from the Store, and do not turn Smart App Control off.

The window is in English, Korean or Japanese — it follows the browser or OS
language, and Settings switches it.

The signed dmg, the Linux tarball, pairing a second machine
(`gadak --workspace laptop init --pairing-code-stdin`), Docker, upgrades:
[`docs/INSTALL.md`](docs/INSTALL.md).

## The point

```bash
gadak sql "select epic_key, count(*) from issues_full where resolved_at is null
           and epic_key <> '' group by epic_key order by 2 desc"
```

JQL has no `GROUP BY`. "Which epic is actually stuck?" is not a hard question —
it is an unaskable one, until the data is a file. [`docs/RECIPES.md`](docs/RECIPES.md)
has the rest, and [Datasette Lite runs this query on the demo snapshot in your
browser](<https://lite.datasette.io/?url=https%3A%2F%2Fraw.githubusercontent.com%2Fmidagedev%2Fgadak%2Fmain%2Fexamples%2Fdemo.db#/demo?sql=select+epic_key%2C+count(*)+from+issues_full+where+resolved_at+is+null+and+epic_key+%3C%3E+''+group+by+epic_key+order+by+2+desc>)
with nothing installed.

Measured 2026-08-26 against a live Cloud site (3,296 issues; medians, CLI
startup included):

| Question | REST API | `gadak` | |
| --- | ---: | ---: | ---: |
| Simple filter, 100 issues | 583 ms | 19 ms | 31× |
| One issue with its full history | 710 ms | 28 ms | 25× |
| Free-text search | 543 ms | 41 ms | 13× |
| **Open issues per epic (`GROUP BY`)** | 4,761 ms — 8 API pages, aggregated client-side | 22 ms — one query | **214×** |
| A count over the change history | not expressible — ≈ 28 min of crawling | 14 ms | — |

Past a page size, JQL answers stop being slow and start being unaskable: the
API hands you rows, never the aggregate. The method, the re-measurement
history, and the rows where gadak loses — the first full sync, the watch tick
on a quiet site, one sync interval of staleness — are in
[`docs/BENCHMARKS.md`](docs/BENCHMARKS.md).

<details>
<summary>▶ 20-second tour of the paper list (GIF)</summary>

<p align="center">
  <img src="docs/media/web-demo.gif" alt="The paper list narrows as you type; an issue opens with labels, priority and a reopen badge; documents and the board sit in the same window" width="900">
  <br>
  <sub>The window, in twenty seconds. Generated from <a href="e2e/demo/web-demo.spec.ts">e2e/demo/web-demo.spec.ts</a> against the demo snapshot.</sub>
</p>

</details>

> **Status: 0.20, still 0.x.** Sync, read API, write-through, desktop, web, CLI, and MCP are verified against a live site. [`CHANGELOG.md`](CHANGELOG.md).

## For agents

This is half the reason gadak exists. Reference: **[docs/MIRROR.md](docs/MIRROR.md)**;
one paste per host: [`docs/AGENT_SETUP.md`](docs/AGENT_SETUP.md).

```bash
gadak skill install
```

Schema and query patterns, no extra process. For hosts without a shell
(Claude Desktop), the same mirror is an MCP server:

```bash
gadak mcp install claude
```

<p align="center">
  <img src="docs/media/terminal-hero.gif" alt="gadak's own terminal pane under the list: gadak claim NMA-140 moves the row to In Progress and the shell's tab takes the key; claude starts in that shell, a Korean prompt turns the list into Dana Whitfield's recently moved issues, and a second prompt saves and opens a label-ratio dashboard in the same window" width="900">
  <br>
  <sub>The shell is in the window (⌘K → Terminal, or Ctrl+`). <code>gadak claim</code> binds it to the issue — the tab is named by the key — and a live Claude Code session started in it drives the board beside it: one Korean sentence becomes the list, the next one paints a dashboard. Nothing but the two prompts is scripted; the stretches where the agent is working are time-lapsed. Recorded from <a href="e2e/demo/terminal-claude-demo.spec.ts">e2e/demo/terminal-claude-demo.spec.ts</a> via <a href="e2e/demo/record-terminal-claude.sh">record-terminal-claude.sh</a>.</sub>
</p>

Two rules carry most of the value. Filter on `status_category` and
`priority_rank`, never on a display name — Jira translates those per account,
so `priority = High` is silently zero rows on a Korean-language site. And SQL
answers while the window presents: `gadak sql --no-header "…" | gadak views
open --keys -` puts an agent's answer on your screen, and `gadak views open
--jql '…'` lands pasted JQL as chips. Writes — `create`, `edit`, `comment`,
`transition`, `claim`, `link`, and the wiki's `page` verbs — go through the
origin before the mirror refreshes, and every agent write carries the agent's
name.

What agents have built on it — dashboards, a team theme, a launcher, a live
MCP session — is a gallery of recordings: [`docs/SHOWCASE.md`](docs/SHOWCASE.md).

**An agent that reads your mirror sends what it reads to whatever model it
talks to.** gadak itself sends nothing ([`SECURITY.md`](SECURITY.md)); scope
the mirror to what the agent should see. Where gadak *does* touch the network —
sync, writes, pairing — [`docs/NETWORK.md`](docs/NETWORK.md) walks every
connection and its off switch.

## What's covered

Three origins, one set of verbs: Atlassian Cloud, Linear (a `"linear"` block
in the workspace config and `gadak sync --source linear`), and the built-in
tracker that travels with the app. Reads, writes, hierarchy, wiki, attachments,
history and the board layout work on all three; what each origin refuses, with
the code citation behind every cell, is one table:
[`docs/SUPPORT_MATRIX.md`](docs/SUPPORT_MATRIX.md). Three things appear on no
origin at all: sprints as a UI, Jira dashboards, and Jira's notification inbox
— those stay in Jira.

## The rest

**Good fit / bad fit.** Daily search latency, an agent over tracker *and* wiki,
offline reads — yes. Sprint planning, admin, a page editor in the UI, or a
minute of staleness — stay in Jira. [`docs/CONCEPT.md`](docs/CONCEPT.md#good-fit-bad-fit).

**How it works.** One binary, one SQLite file; incremental sync plus a
reconcile pass. [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md). Why not an
extension or Forge app: [`docs/decisions/0003-local-process.md`](docs/decisions/0003-local-process.md).

**How it compares.** jira-cli talks to the live API per command. Rovo MCP
searches both sources too, but it is hosted: no aggregate, no offline, and
every call spends tokens. [`docs/FAQ.md`](docs/FAQ.md#how-it-compares).

**Making it yours.** Config, enrichments, SQL — two axes, no forking:
[`docs/EXTENDING.md`](docs/EXTENDING.md).

## Documentation

- [`CHANGELOG.md`](CHANGELOG.md) — what shipped
- [`docs/INSTALL.md`](docs/INSTALL.md) · [`docs/DESKTOP.md`](docs/DESKTOP.md) — install, first run, the desktop app
- [`docs/SHOWCASE.md`](docs/SHOWCASE.md) — the window, the launcher, and agents driving both, on camera
- [`docs/MIRROR.md`](docs/MIRROR.md) · [`docs/MCP.md`](docs/MCP.md) · [`docs/AGENT_SETUP.md`](docs/AGENT_SETUP.md) — SQL, CLI, REST, MCP, one paste per host
- [`docs/RECIPES.md`](docs/RECIPES.md) · [`docs/DASHBOARDS.md`](docs/DASHBOARDS.md) — questions JQL cannot ask, as SQL; agent-authored dashboards
- [`SECURITY.md`](SECURITY.md) · [`docs/FAQ.md`](docs/FAQ.md) · [`MAINTENANCE.md`](docs/MAINTENANCE.md) — threat model, site load, who maintains this
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

[`CONTRIBUTING.md`](.github/CONTRIBUTING.md), and
[`docs/project/GOOD_FIRST_ISSUES.md`](docs/project/GOOD_FIRST_ISSUES.md) to start.
Why the next features are the ones they are, with sources:
[`docs/project/THEORY.md`](docs/project/THEORY.md).
Bug reports need your Jira deployment type (Cloud), the gadak commit, and the
command you ran. Never paste real issue data, tokens, or site URLs into a
public issue. Commit `GDK-nnn` keys resolve on the
[public backlog](https://gadak.dev/backlog/); to file something, open a
[GitHub issue](https://github.com/midagedev/gadak/issues) and the maintainer
mirrors it there. Using gadak with an agent and hitting friction? Open an issue
with the question you asked and what the agent did.

## License

Apache-2.0. See `LICENSE` and `NOTICE`.
