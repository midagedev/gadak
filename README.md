<p align="center">
  <img src="docs/media/wordmark-dark.svg#gh-dark-mode-only" width="380" alt="gadak">
  <img src="docs/media/wordmark-light.svg#gh-light-mode-only" width="380" alt="gadak">
</p>

<p align="center">
  <a href="https://github.com/midagedev/gadak/releases"><img src="https://img.shields.io/github/v/release/midagedev/gadak" alt="Latest Release"></a>
  <a href="https://github.com/midagedev/gadak/actions/workflows/ci.yml"><img src="https://github.com/midagedev/gadak/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache--2.0-blue" alt="License"></a>
</p>

<p align="center"><b>Find the thread in your backlog.</b></p>

<p align="center"><sub>English · <a href="README.ko.md">한국어</a> · <a href="README.ja.md">日本語</a></sub></p>

Query your Jira backlog with SQL.

gadak mirrors the Jira Cloud projects and Confluence spaces you select (issues,
comments, history, wiki pages) into one SQLite file on your machine, indexed
together and searchable with no network. Use the mirror through the
[desktop app](docs/DESKTOP.md), a browser tab (`gadak serve`), the CLI, or MCP.
Writes go to Jira first. One binary, no gadak account. It is 0.x software from
one maintainer; [status and promises](#status-compatibility-and-maintenance)
are below.

<p align="center">
  <a href="https://gadak.dev/demo/"><b>▶&nbsp; Open the live demo</b></a>
  &nbsp;·&nbsp; 534 issues, in your browser, right now.
  <br>
  <a href="CHANGELOG.md">Changelog</a>
  &nbsp;·&nbsp; what shipped.
</p>

## Query Jira with SQL

```bash
gadak sql "select epic_key, count(*) from issues_full where resolved_at is null
           and epic_key <> '' group by epic_key order by 2 desc"
```

JQL has no `GROUP BY`. This query counts unresolved issues in each epic; the
REST API returns the rows and leaves the counting to you. [Datasette Lite runs
this query on the demo snapshot in your
browser](<https://lite.datasette.io/?url=https%3A%2F%2Fgadak.dev%2Fdemo%2Fgadak-demo.db#/demo?sql=select+epic_key%2C+count(*)+from+issues_full+where+resolved_at+is+null+and+epic_key+%3C%3E+''+group+by+epic_key+order+by+2+desc>)
with nothing installed. More questions JQL cannot ask, as SQL:
[`docs/RECIPES.md`](docs/RECIPES.md).

Measured 2026-08-26 against a live Cloud site (3,296 issues; medians, CLI
startup included):

| Question | REST API | `gadak` | |
| --- | ---: | ---: | ---: |
| Simple filter, 100 issues | 583 ms | 19 ms | 31× |
| One issue with its full history | 710 ms | 28 ms | 25× |
| Free-text search | 543 ms | 41 ms | 13× |
| **Open issues per epic (`GROUP BY`)** | 4,761 ms — 8 API pages in this measurement, aggregated client-side | 22 ms — one query | **214×** |
| A count over the change history | no native aggregate — ≈ 28 min to crawl the history and count client-side | 14 ms | — |

The method, the re-measurement history, and the rows where gadak loses (the
first full sync, the watch tick on a quiet site, one sync interval of
staleness) are in [`docs/BENCHMARKS.md`](docs/BENCHMARKS.md).

<details>
<summary>▶ 20-second tour of the list (GIF)</summary>

<p align="center">
  <img src="docs/media/web-demo.gif" alt="The list narrows as you type; an issue opens with labels, priority and a reopen badge; documents and the board sit in the same window" width="900">
  <br>
  <sub>The window, in twenty seconds. Generated from <a href="e2e/demo/web-demo.spec.ts">e2e/demo/web-demo.spec.ts</a> against the demo snapshot.</sub>
</p>

</details>

## Install

macOS app, CLI included:

```bash
brew install --cask midagedev/tap/gadak
```

CLI only, on macOS or Linux. The same UI opens in a browser tab via
`gadak serve`:

```bash
brew install midagedev/tap/gadak-cli
```

Connect to Jira Cloud, then open the address `gadak serve` prints
(`http://gadak.localhost:7777`). The first sync runs inside `serve`, newest
issues first, and the list fills while it runs:

```bash
gadak init && gadak serve         # mirror a Jira Cloud site
gadak init --local && gadak serve # no Jira — start on the tracker gadak ships with
```

A site needs one [API token](https://id.atlassian.com/manage-profile/security/api-tokens),
a user token created with no scopes; it covers Jira and Confluence on the same
site, and the mirror sees what your account sees. **You pick what it mirrors**:
`--projects` for Jira, `--spaces` for the wiki, which stays off until you name
spaces. Jira Server and Data Center connect with `gadak init --server` and a
Personal Access Token; the wiki stays off there (no Confluence Server client).

```bash
gadak init --projects ENG,PROD --spaces ENG
```

**Windows:** the desktop app is on the [Microsoft Store](https://apps.microsoft.com/detail/9NZW91TXH36G).
The Store signs it, so neither SmartScreen nor Smart App Control objects, and a
Store install puts the `gadak` command on `PATH` as well (since 0.20.2). For
the CLI without the Store, take `gadak_<version>_windows_amd64.zip` (or
`arm64`) from the
[latest release](https://github.com/midagedev/gadak/releases/latest), unzip,
put `gadak.exe` on `PATH`. The release's desktop zip
(`Gadak-<version>-windows-x64.zip`) stays unsigned, and a SmartScreen block
there is a missing signature, not a virus finding ([why](docs/WINDOWS-SIGNING.md)).
If it blocks, install from the Store, and do not turn Smart App Control off.

The window follows the browser or OS language (English, Korean or Japanese;
Settings switches it). The signed dmg, the Linux tarball, Docker, upgrades:
[`docs/INSTALL.md`](docs/INSTALL.md).

**Other workspaces.** `gadak --workspace <new> migrate --from
<old>` moves a synced workspace onto another origin; add `--to linear` to
target a Linear team. A second machine pairs with a home `serve`:
`gadak --workspace laptop init --pairing-code-stdin`.

## Data and network

- **How much is copied.** The Cloud projects and spaces you named, as your
  account sees them; gadak adds no elevation and no service account.
- **What stays local, and how fresh.** One SQLite file. The first full sync is
  the slow part: 3.7 minutes for the benchmark site (3,514 issues and 462
  pages). After that `gadak serve` syncs every 60 seconds by default
  (`syncIntervalSec`), plus an hourly reconcile that removes issues you can no
  longer see. Reads are one interval behind Jira.
- **Where the token is.** `~/.gadak/config.json`, mode `0600`. Credentials
  never reach SQLite, a log, or a snapshot.
- **What leaves the machine.** No telemetry. gadak talks only to what you
  configured; [`SECURITY.md`](SECURITY.md) is the complete list and
  [`docs/NETWORK.md`](docs/NETWORK.md) walks every connection and its off
  switch. Four reads still ask Jira: viewing an attachment,
  `gadak issue --editmeta`, `gadak fields`, and `gadak api` (a passthrough).
- **What a write does.** It goes to Jira first; the mirror refreshes after
  Jira accepts. A write Jira rejects fails then and there. Nothing is queued.
- **What changes with an agent.** It sends what it reads to its model; see
  [Agents](#agents).

**The mirror is a cache you can throw away.** Delete the directory and nothing
is lost.

## Agents

Let your coding agent query the same mirror through the CLI or MCP. Reference:
**[docs/MIRROR.md](docs/MIRROR.md)**; one paste per host:
[`docs/AGENT_SETUP.md`](docs/AGENT_SETUP.md).

```bash
gadak skill install
```

That copies one `SKILL.md` for Claude Code and starts no process; the agent
runs short-lived `gadak` commands. Name another host to install the same file
there: `gadak skill install codex`, and the same for `agents`, `cursor`,
`gemini`, `opencode` and `grok`. For Claude Desktop, which has no shell,
register the same mirror as an MCP server (`gadak mcp install claude` is the
Claude Code registration; Claude Desktop never sees it):

```bash
gadak mcp install claude-desktop
```

<p align="center">
  <img src="docs/media/terminal-hero.gif" alt="gadak's own terminal pane under the list: gadak claim NMA-140 moves the row to In Progress and the shell's tab takes the key; claude starts in that shell, one prompt turns the list into Dana Whitfield's recently moved issues, and a second prompt saves and opens a label-ratio dashboard in the same window" width="900">
  <br>
  <sub>The shell is in the window (⌘K → Terminal, or Ctrl+`). <code>gadak claim</code> binds the pane to the issue, and a Claude Code session started in it filters the list, then saves and opens a dashboard in the same window. Nothing but the two prompts is scripted; the stretches where the agent is working are time-lapsed. The Korean and Japanese READMEs carry the same take recorded in their own language. Recorded from <a href="e2e/demo/terminal-claude-demo.spec.ts">e2e/demo/terminal-claude-demo.spec.ts</a> via <a href="e2e/demo/record-terminal-claude.sh">record-terminal-claude.sh</a>.</sub>
</p>

Two rules carry most of the value. Filter on `status_category` and
`priority_rank` rather than a display name: Jira translates those per account,
so `priority = High` is silently zero rows on a Korean-language site. And open
the issues a query returns in the gadak window:
`gadak sql --no-header "…" | gadak views open --keys -` puts the agent's answer
on your screen, and `gadak views open --jql '…'` lands pasted JQL as chips.

Writes (`create`, `edit`, `comment`, `transition`, `claim`, `link`, and the
wiki's `page` verbs) go through the origin before the mirror refreshes. On Jira and
Linear, an agent's comments and the issues it creates carry its name (off switch:
`gadak config set actor.trailer false`).

**An agent that reads the mirror sends what it reads to whatever model it talks
to.** gadak itself sends nothing. Scope the mirror to what the agent should see.

## Origins and limits

Four origins, one set of verbs: Atlassian Cloud, Jira Server / Data Center
(`gadak init --server` and a Personal Access Token), Linear (a `"linear"`
block in the workspace config and `gadak sync --source linear`), and the
built-in tracker that travels with the app. Reads, writes, hierarchy,
attachments, history and the board layout work on all four; the wiki on the
three that have one. What each origin refuses, with
the code citation behind every cell:
[`docs/SUPPORT_MATRIX.md`](docs/SUPPORT_MATRIX.md). Three things appear on no
origin at all and stay in Jira: sprints as a UI, Jira dashboards, and Jira's
notification inbox.

**Good fit / bad fit.** Yes to daily search latency, an agent over tracker
*and* wiki, and offline reads. Sprint planning, admin, a page editor in the UI,
or a minute of staleness that matters: keep those in Jira.
[`docs/CONCEPT.md`](docs/CONCEPT.md#good-fit--bad-fit).

**How it works.** One binary, one SQLite file; incremental sync plus a
reconcile pass: [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md). Why a local
process, not an extension or Forge app:
[`docs/decisions/0003-local-process.md`](docs/decisions/0003-local-process.md).
Config, enrichments and SQL make it yours without forking:
[`docs/EXTENDING.md`](docs/EXTENDING.md).

## Alternatives and prior work

jira-cli talks to the live API per command. Rovo MCP is hosted by Atlassian
and searches Jira and Confluence; it has write tools, but no native aggregation
tool and no offline reads. gadak runs SQL against a local mirror, and its cost
is a local binary and an initial sync. Earlier projects in this area include
Scrumdog, jira-offline and jira-cache; gadak combines a local Jira and
Confluence mirror with SQL, desktop and browser interfaces, a CLI, and MCP.
[`docs/FAQ.md`](docs/FAQ.md#how-it-compares).

## Status, compatibility and maintenance

**Status: 0.21, still 0.x.** Sync, read API, write-through, desktop, web, CLI
and MCP are verified against a live site. The project currently has one
maintainer. During 0.x, compatibility is promised for three things, listed in
[specs/000-product/data-model.md](specs/000-product/data-model.md): `issues_full` and the
RECIPES queries, `gadak sql` stdout, and `gadak views open --keys -`. The
license is Apache-2.0, and the mirror is ordinary SQLite. The name is Korean:
gadak (가닥) is a strand — a thread drawn out of a tangle. What you do not have
to take on trust, each with the command that checks it:
[`docs/PROMISES.md`](docs/PROMISES.md).

## Documentation

- [`CHANGELOG.md`](CHANGELOG.md): what shipped
- [`docs/INSTALL.md`](docs/INSTALL.md) · [`docs/DESKTOP.md`](docs/DESKTOP.md): install, first run, the desktop app
- [`SECURITY.md`](SECURITY.md) · [`docs/NETWORK.md`](docs/NETWORK.md): threat model, every connection and its off switch
- [`docs/MIRROR.md`](docs/MIRROR.md) · [`docs/MCP.md`](docs/MCP.md) · [`docs/AGENT_SETUP.md`](docs/AGENT_SETUP.md): SQL, CLI, REST, MCP, one paste per host
- [`docs/RECIPES.md`](docs/RECIPES.md) · [`docs/DASHBOARDS.md`](docs/DASHBOARDS.md): questions JQL cannot ask, as SQL; agent-authored dashboards
- [`docs/SHOWCASE.md`](docs/SHOWCASE.md): the window, the launcher, and agents driving both, on camera
- [`docs/FAQ.md`](docs/FAQ.md) · [`docs/MAINTENANCE.md`](docs/MAINTENANCE.md): site load, comparisons, who maintains this
- [`docs/README.md`](docs/README.md): the rest of the docs

## Feedback and contributing

If you used gadak on your own project, open a
[GitHub issue](https://github.com/midagedev/gadak/issues) or email
[midagedev@gmail.com](mailto:midagedev@gmail.com): what question did it
answer, and did you go back to it? Reports of everyday use are welcome before
any contribution.

Keep real issue data, tokens, and site URLs out of a public issue. A bug
report needs your Jira deployment type (Cloud or Server), the gadak commit,
and the command you ran. The maintainer mirrors issues to the
[public backlog](https://gadak.dev/backlog/), where commit `GDK-nnn` keys
resolve.

To contribute: [`.github/CONTRIBUTING.md`](.github/CONTRIBUTING.md) and
[`docs/project/GOOD_FIRST_ISSUES.md`](docs/project/GOOD_FIRST_ISSUES.md). Why
the next features are the ones they are, with sources:
[`docs/project/THEORY.md`](docs/project/THEORY.md).

## License

Apache-2.0. See `LICENSE` and `NOTICE`.
