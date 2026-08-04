# scry

**Your issue tracker, mirrored to a local SQLite file.** One binary: a browser UI
that filters 10,000 issues without a network round trip, a terminal UI for when
you never leave tmux, and a database your coding agent can query with plain SQL.

Jira is the first source.

<p align="center">
  <img src="docs/media/web-demo.gif" alt="Typing in the search box narrows 519 issues instantly, with matches highlighted; ⌘K jumps to an issue" width="900">
</p>

```bash
scry init && scry sync    # Jira -> ~/.scry/scry.db
scry serve                # http://localhost:7777
scry tui                  # same mirror, in your terminal
scry sql "select key, summary from issues where reopen_count > 1"
```

> **Status: working, pre-release.** Sync, the read API, write-through, the web UI,
> the TUI, the CLI, settings, the plugin boundary, and i18n are implemented and
> verified end to end against a live Jira site. `docs/STATE_OF_PLAY.md` is the
> honest inventory.

## Why

Three complaints about living in an issue tracker, one root cause.

**Search is slow.** Every filter change is a network round trip against a
multi-tenant service. Teams that live in the tracker feel this dozens of times an
hour. Once the data is on local disk, filtering is a memory operation: type a
character, see the result. No spinner, no debounce, no "loading issues…".

**Agents cannot read your tracker well.** A coding agent asked "what did we
already fix in the billing flow?" has to page through a REST API, guess at JQL,
and burn its context on JSON. Give it a SQLite file instead and it writes one
query with a join and an FTS match. No tool schema, no pagination, no rate limit.

**You cannot see your own team's shape.** "Which issues came back after we closed
them, and why?" is a join over the changelog. In Jira it is not a question you
can ask; here it is `where reopen_count > 0`.

All three fall out of the same move: mirror the data locally, then let the UI, the
terminal, and the agent read the same store.

## Three surfaces, one store

| | For | Looks like |
| --- | --- | --- |
| **Web UI** | all-day triage, mouse and keyboard | keyboard-driven list, saved views, ⌘K palette, full detail with rich text, comments, history, attachments |
| **TUI** | people who live in the terminal | [`scry tui`](docs/TUI.md) — list, filter, detail, and write actions over the same mirror |
| **CLI + SQL** | agents, scripts, one-off questions | `scry issue`, `scry search`, `scry sql`, plus the file itself |

<p align="center">
  <img src="docs/media/tui.gif" alt="scry tui: filtering and opening an issue in the terminal" width="800">
</p>

Writes go through in all three: status transitions, comments with mentions and
inline screenshots, assignee changes, field edits, issue creation. They hit Jira
and then refresh the mirror, so the list is correct a moment later without a full
sync.

Attachments are local too. The first view of an image caches its bytes next to
the mirror and every later view is a disk read, so a screenshot-heavy issue opens
at the speed of the rest of the app — and keeps rendering offline.

## Install

Jira Cloud only. You need an API token from
<https://id.atlassian.com/manage-profile/security/api-tokens>.

### 1. Homebrew (after the first release)

```bash
brew install midagedev/tap/scry
```

The `midagedev/homebrew-tap` formula is published by GoReleaser on each release
tag. Until the first public release exists, this install path is inactive.

### 2. Install script (after the first release)

```bash
curl -fsSL https://raw.githubusercontent.com/midagedev/scry/main/scripts/install.sh | sh
```

Downloads the latest GitHub Release for your OS/arch, verifies `checksums.txt`
(sha256), and installs to `~/.local/bin/scry` (override with `SCRY_INSTALL_DIR`).
Upgrades in place if a binary is already there. Inactive until a release is
published.

### 3. Release binary

Download the archive for your OS/arch from
[GitHub Releases](https://github.com/midagedev/scry/releases), verify against
`checksums.txt`, unpack, and put `scry` on your `PATH`.

### 4. Build from source

Requirements: Go 1.25+, Node.js 20+.

```bash
npm ci && npm run build       # build the web UI into dist/app
go build -o scry ./cmd/scry   # embeds it — the binary is the whole install
# or: make build  → bin/scry
```

### First run

```bash
scry serve                    # http://localhost:7777
```

The first run walks you through it in the browser: paste your site, email, and
token, pick projects from your site's own list, and watch the first sync fill the
mirror. If you would rather stay in the terminal, `scry init && scry sync`
does the same thing, and `scry serve --sync` keeps the mirror fresh in the
background afterwards.

Your API token lives in `~/.scry/config.json` at `0600` and never touches the
database, the repository, or a log line. There is no scry account, no server, and
no telemetry — the only outbound traffic is to your own Jira site.

Pointing one machine at two sites (work and a demo, say) is what profiles are
for: `scry --profile demo init` keeps a separate credential and mirror under
`~/.scry/profiles/demo/`.

### Try it with no Jira account at all

```bash
scry demo
# from a source checkout: ./scry demo
```

Opens the UI against `examples/demo.db` — 519 issues on fictional projects. It is
also what the test suite and the GIFs above run against, so what you see is what
CI checks.

### Run it in a container

```bash
docker build -t scry . && docker run --rm -p 7777:7777 -v scry-data:/data scry
```

The process has no authentication by design, so it refuses to bind a non-loopback
address without `--allow-remote` (the image passes it). Only put it on a network
you trust. Config and `scry.db` live under `/data`.

## For agents

This is half the reason scry exists, so it has its own reference: **[AGENTS.md](AGENTS.md)**.

<p align="center">
  <img src="docs/media/agent.gif" alt="scry search, scry sql aggregation, and scry issue in a terminal" width="800">
</p>

The interface is the database. Anything that can run a shell command can use it
at full power:

```bash
# What keeps coming back, and when did it last happen?
scry sql "select key, summary, reopen_count, reopened_at from issues
          where reopen_count > 0 order by reopened_at desc limit 20"

# Full-text across descriptions and comments
scry sql "select i.key, it.title from items_fts f
          join items it on it.rowid = f.rowid
          join issues i on i.item_id = it.id
          where items_fts match 'idempotency AND webhook' limit 20"

# One issue whole, or one write straight through to Jira
scry issue NMB-140 --json
scry comment NMB-140 -m "Reproduced on staging."
scry transition NMB-140 "In Review"
```

Every command takes `--json`, warns on stderr when the mirror is behind, and
keeps stdout clean enough to pipe. `scry sql` is read-only and statement-checked,
so an agent on a narrow command allowlist can be given mirror access without
being given arbitrary `sqlite3`.

The schema in `specs/000-product/data-model.md` is a public contract. Filter on
`status_category` and ids, never on display names — Jira translates those per
account, which is the one mistake that silently returns nothing.

## Making it yours

Two axes, no forking required — see **[docs/EXTENDING.md](docs/EXTENDING.md)**.

**Configuration** covers most of it, from the settings dialog or
`~/.scry/config.json`: map your custom fields (severity, environment, whatever
your site calls them), classify issues into teams by label or component, choose
which fields are inline-editable, set the staleness threshold and sync
intervals, toggle features. Most keys apply without restart; sync intervals
need a restart of `scry serve --sync`. Full key table:
[`docs/CONFIGURATION.md`](docs/CONFIGURATION.md).

**The plugin boundary** covers the rest. The core contains zero GitHub, CD, or
test-management code on purpose. Anything else you want beside an issue — linked
pull requests, deploy status, QA context — arrives by writing rows into the
`enrichments` table and bumping a version counter, from any language on any
schedule. The server merges them; the UI surfaces them. Working examples live in
[`examples/plugins/`](examples/plugins/), and the contract is
[`docs/PLUGINS.md`](docs/PLUGINS.md).

## How it works

```mermaid
flowchart LR
  Jira["Jira Cloud REST"] -->|"incremental sync"| DB["SQLite + FTS5<br/>~/.scry/scry.db"]
  DB --> Serve["scry serve"]
  Serve --> UI["Web UI<br/>(IndexedDB cache)"]
  DB --> TUI["scry tui"]
  DB --> Agent["Coding agent<br/>sqlite3 / scry sql"]
  UI -->|"writes"| Serve
  Serve -->|"writes"| Jira
```

Sync is incremental with a two-minute overlap on the watermark, plus a reconcile
pass so deletions do not linger. Derived fields the source does not provide —
reopen count and the reason it came back, last status change, resolution date,
clone origin — are computed during sync and keyed on `statusCategory`, never on a
localized status name.

### Why not a browser extension or a Forge app?

Jira Cloud deliberately sends no CORS headers on its REST API, so a static page
cannot call it. Both alternatives that avoid a local process were considered and
rejected because neither can hand a coding agent a queryable local database,
which is half the point. See `docs/decisions/0003-local-process.md`.

## Good fit / bad fit

| Use scry when… | Use Jira directly when… |
| --- | --- |
| You search and triage the same projects every day and the latency hurts. | You need boards, sprints, reports, automation, permissions. |
| You want an agent to reason over your tracker's history. | You need administration or workflow editing. |
| You want offline reading of everything you have access to. | A minute of staleness matters. |
| Your tracker holds tens of thousands of issues and Jira's UI struggles. | Your team is small enough that Jira already feels instant. |

**In scope:** issue fields, descriptions, comments, attachments, changelog, links,
status transitions, assignee, field edits, issue creation, full-text search, saved
views, watches.
**Out of scope:** boards and sprint mechanics, project administration, workflow
configuration, permission schemes, and anything requiring Jira's own UI.
**Not a sync engine:** Jira is the system of record. The mirror is disposable —
delete it and re-sync.

## More sources later

The storage schema and search layer are source-neutral so a second connector does
not reshape the database. Confluence is the intended next one: same local index,
same instant search, same agent access. Only Jira is implemented today, and no
source-specific work merges until the neutral layer stays neutral.

## Documentation

- [`AGENTS.md`](AGENTS.md) — the agent reference: SQL, CLI, REST
- [`docs/EXTENDING.md`](docs/EXTENDING.md) — fitting scry to your team
- [`docs/STATE_OF_PLAY.md`](docs/STATE_OF_PLAY.md) — what exists, what does not
- [`docs/CONCEPT.md`](docs/CONCEPT.md) — the product idea and the loop it optimizes
- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) — components and data flow
- [`docs/TUI.md`](docs/TUI.md) — terminal UI keys, and CJK font guidance
- [`docs/PLUGINS.md`](docs/PLUGINS.md) — the enrichment contract
- [`docs/decisions/`](docs/decisions/) — why it is shaped this way
- [`specs/000-product/`](specs/000-product/) — spec, data model, API and sync contracts

## Contributing

See [`CONTRIBUTING.md`](CONTRIBUTING.md). Bug reports should include your Jira
deployment type (Cloud), the scry commit, and the command you ran. Never paste
real issue data, tokens, or site URLs into a public issue.

## License

Apache-2.0. See `LICENSE` and `NOTICE`.
