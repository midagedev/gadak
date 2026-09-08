# Giving your coding agent access to gadak

One paste per tool. Each block teaches the agent that the mirror exists, how to
query it, and the one mistake that silently returns nothing. The full reference
is [`docs/MIRROR.md`](MIRROR.md).

## Let the agent set gadak up for you

If a coding agent (Claude Code, Codex, …) already talks to Jira on this
machine — through environment variables, an MCP connector, or its own memory —
it already holds everything gadak needs. Paste this prompt into the agent:

```text
Set up gadak (https://github.com/midagedev/gadak) against the Jira account this
machine already uses. Steps:

1. Use the Jira credentials this session already has: JIRA_URL / JIRA_EMAIL /
   JIRA_API_TOKEN environment variables, or an Atlassian MCP server entry in
   .mcp.json or the Claude config. Tell me which source you found before
   using it. Do not go hunting through shell profiles or dotfiles for
   tokens — if nothing is already exposed, ask me for the token instead.
2. Install gadak if missing: `brew install midagedev/tap/gadak-cli`
   (or the install script in the repo README).
3. Configure non-interactively — init never prompts when values are supplied:
   GADAK_TOKEN=<token> gadak init --site <url> --email <email> --json
   Leave --projects off: an empty project list syncs every project I can see,
   and the first line of sync output shows the total so we can narrow later
   with `gadak init --projects KEY1,KEY2` if it is huge. If `~/.claude`
   already exists, init installs the Claude Code skill into
   `~/.claude/skills/gadak/SKILL.md` (a file gadak did not write is left
   in place).
4. Run `gadak sync`. The first full sync auto-discovers the site's custom
   fields (labels, filter axes, editors) and prints what it configured.
5. Verify: `gadak sql "select count(*) from issues_full"` returns a number,
   and `gadak fields` lists the discovered mapping.
6. Add the gadak usage block from docs/AGENT_SETUP.md to my agent config so
   future sessions query the mirror instead of the Jira REST API.
```

## No Jira on this machine

If there is no Atlassian account and the user wants a backlog (this repo,
this machine, an agent-owned plan), that is a workspace on the **built-in tracker** — not
a missing Jira token. Do not invent `TODO.md` or a GitHub Issue when `gadak`
is on PATH. If this machine already has a Jira workspace, use a
dedicated `--workspace` so personal issues never land on the company site.

Paste this prompt:

```text
This machine has no Jira account. Keep a backlog for the work we do: create
a gadak workspace on the built-in tracker, file the first tickets, and show them to me.

1. Install gadak if missing: `brew install midagedev/tap/gadak-cli`
   (or the install script in the repo README).
2. Non-interactive, no token, no --site:
   gadak init --local --json
   If a Jira-site workspace already exists on this machine:
   gadak --workspace plan init --local --json
   and pass `--workspace plan` on every later command. If `~/.claude`
   already exists, init installs the Claude Code skill.
3. gadak sync
   then gadak create "first ticket title" -m "why this exists"
   Init seeds project STD and wiki space LOC and records the default
   type, so a summary is enough.
4. gadak views open <KEY>
5. The durable record is origin/issuetap.db under the workspace
   directory (`gadak doctor --json` → workspace.persist). Copy it while
   gadak is not running, or sqlite3 origin/issuetap.db ".backup dest.db".
   gadak.db is still a disposable cache.
6. Add the gadak usage block from docs/AGENT_SETUP.md to my agent config
   so future sessions query the mirror.
```

Everything the agent configures stays editable afterwards: field mapping in
the web UI under Settings → Fields (edits are pinned and survive
re-discovery), or `gadak fields --apply` to re-run detection, or the `fields`
array in `~/.gadak/config.json` directly.

## Claude Code — `CLAUDE.md` (project or user scope)

```markdown
## Jira (via gadak local mirror)

Jira issues are mirrored to a local SQLite file. Prefer these over any Jira API:

- `gadak issue <KEY>` — everything about one issue (fields, description,
  comments, history, links) in one call. Add `--json` for structure.
- `gadak search "<text>" --json` — full-text over summaries, bodies, comments.
  A Jira URL or `--jql "…"` applies the documented JQL subset instead;
  unsupported clauses are listed on stderr — do not hide them.
- `gadak sql "<select …>"` — read-only SQL (SQLite mode=ro). Schema:
  specs/000-product/data-model.md in the gadak repo (or
  `SELECT sql FROM sqlite_master WHERE type IN ('table','view')`).
- `gadak views` / `gadak views open --jql "…"` / `gadak views open --keys -` —
  list mirrored Jira filters and put the running app or serve tab on that
  view. Do not describe chips to the user; set them. When the user wants to
  *see* issues, do not paste a table — `gadak views open`. `gadak open` is
  the origin (Jira/Linear) escape hatch; `gadak views open` is open-in-gadak.
- `gadak comment <KEY> -m "…"`, `gadak transition <KEY> "<status>"` — writes go
  through the origin (the Jira site on a Jira workspace, the built-in tracker on
  the built-in tracker).

Rules: filter on `status_category` ('new'|'inprogress'|'done') and ids, never
on display names — Jira localizes those per account. Query the `issues_full`
view (it includes `summary`); the bare `issues` table has no title column. If
the mirror warns it is stale, mention that in your answer.
```

## Cursor — `.cursor/rules/gadak.mdc`

```markdown
---
description: Query Jira through the local gadak mirror instead of the REST API
alwaysApply: false
---

When the user asks about Jira issues, use the gadak CLI against the local
mirror: `gadak issue <KEY> --json`, `gadak search "<text>" --json`, or
`gadak sql "<select …>"` (read-only). Filter on status_category, not display
names. Use the `issues_full` view — it includes the issue title as `summary`.
`gadak views` / `gadak views open --jql "…"` / `gadak views open --keys -`
— list mirrored Jira filters and put the running app or serve tab on that
view. Do not describe chips to the user; set them.
```

## Codex — `AGENTS.md` (repo root)

Codex loads skills, so `gadak skill install codex` is the shorter route and the
one to prefer — see [The skill](#the-skill-preferred-when-the-agent-has-a-shell)
below. This block is for a repository that wants the instruction in the file
everyone on the team already reads.

```markdown
## Jira access

Use the gadak local mirror, not the Jira REST API:
`gadak issue <KEY> --json` · `gadak search "<text>" --json` ·
`gadak sql "<read-only select>"`. Filter on `status_category` and ids, never
display names. Use the `issues_full` view for titles.
`gadak views` / `gadak views open --jql "…"` / `gadak views open --keys -`
puts the running app on a view — do not describe chips; set them.
```

## Skill or MCP?

gadak's value for agents is **schema and query-pattern knowledge**, not a fixed
tool surface. Two install paths:

| | **Skill** (`gadak skill install`) | **MCP** (`gadak mcp install`) |
| --- | --- | --- |
| Best when | The agent has a shell and loads skills (Claude Code, Codex, Cursor, Gemini CLI, OpenCode, grok) | The host has **no** shell (Claude Desktop, some IDE hosts) |
| What you get | One folder of docs, loaded only when relevant | A stdio server + always-on tool schemas in context |
| Cost | Cheap on context; agent runs `gadak sql` / `gadak issue` itself | Tool definitions occupy context every turn |

If the agent has a shell, `gadak skill install <client>` is the path. MCP is
for hosts without a shell: `gadak mcp install claude-desktop` registers with
Claude Desktop, and `gadak mcp install claude` with Claude Code. They do not
conflict — the skill teaches SQL/CLI; MCP is a separate read-only tool
surface — but a shell agent does not need MCP.

## The skill (preferred when the agent has a shell)

The skill is one file, and it is the same file for every host. Codex, Cursor,
Gemini CLI, OpenCode and grok all read a `skills/<name>/SKILL.md` directory the
way Claude Code does — measured, not assumed: gadak's own `SKILL.md` was copied
byte-for-byte into Codex and read back out of `codex debug prompt-input`. So a
client selects a path and nothing else.

```bash
gadak skill install
```

That is Claude Code, the default. Name any other host to install it there —
`gadak skill install codex`, `gadak skill install cursor`, and so on:

| Client | Home (default) | `--project` | Confirm it loaded |
| --- | --- | --- | --- |
| `claude` | `~/.claude/skills/gadak/` | `.claude/skills/gadak/` | `/skills` in a new session |
| `codex` | `$CODEX_HOME/skills/gadak/`, else `~/.codex/skills/gadak/` | `.agents/skills/gadak/` | `codex debug prompt-input` lists gadak inside `<skills_instructions>` |
| `agents` | `~/.agents/skills/gadak/` | `.agents/skills/gadak/` | whatever the host provides; this is the shared convention, not one product |
| `cursor` | `~/.cursor/skills/gadak/` | — | the skill appears in Cursor's skill list |
| `gemini` | `~/.gemini/skills/gadak/` | — | the skill appears in the CLI's skill list |
| `opencode` | `~/.config/opencode/skills/gadak/` | — | the skill appears in OpenCode's skill list |
| `grok` | `~/.grok/skills/gadak/` | — | the skill appears in the CLI's skill list |

`agents` is the row that serves several hosts at once: `~/.agents/skills` is the
shared discovery root the [agentskills.io](https://agentskills.io) convention
describes, and Codex reads it too.

The four hosts with no `--project` entry read a *rules file* in a repository
(`.cursor/rules/gadak.mdc`, `gemini-extension.json`, a plugin under
`.opencode/`), not a skill directory, so `--project` refuses for them and says
so. Install those at the home scope, or write the file yourself with `--dir`.

Hosts that only ever load an always-on instruction file — Copilot, Windsurf,
Cline, Kiro, Amp, and the `AGENTS.md` family — are not installable this way at
all. Paste the block from the top of this page instead, or use
`gadak mcp install <client>`.

When `~/.claude` already exists, `gadak init` and `gadak install-cli` write
`~/.claude/skills/gadak/SKILL.md` themselves. A file gadak did not write is
left in place; `gadak skill install --force` overwrites it. If `~/.claude`
is absent, those commands skip the skill and (for `install-cli`) still print
`gadak skill install` as the next step. Auto-install and the once-a-day refresh
only ever touch the Claude Code copy — the other hosts are installed on request.

Flags:

| Flag | Effect |
| --- | --- |
| `--project` | install under the current directory instead of the home directory (see the table) |
| `--dir PATH` | install into `PATH/gadak/SKILL.md` (overrides the client's default and `--project`) |
| `--print` | print the install plan without writing |
| `--force` | overwrite a SKILL.md gadak did not write (hand-edited or your own) |

Restart the agent or open a new session so it picks up the skill. The skill
body is embedded in the binary (same as `skills/gadak/SKILL.md` in the repo),
so brew installs work without a checkout.

`gadak doctor` reports the skill per host — `skill: current (claude, codex) ·
missing (agents)` — so "did that install land?" is one command, and it names
the host it found rather than assuming Claude Code.

Building gadak from a checkout? That binary's embedded skill is whatever is in
your working tree, so the once-a-day refresh never runs from it: instead of
pushing an unreviewed draft into your own agent it prints
`skill: dev build — not syncing ~/.claude/skills/gadak`, once, and leaves the
installed copy alone. The installs that come along with another command —
`gadak init`, pairing, `gadak install-cli` — follow the same rule from a
checkout: they create the skill if you have none, and if one is already there
they leave it and say `skill: dev build — not replacing …` instead.
`gadak skill install` still works — that one you typed on
purpose — but the receipt beside the file then records `source: dev-tree` plus
the short git hash, and `gadak doctor` says
`skill: current (~/.claude/skills/gadak/SKILL.md, dev-tree 3f2ab1c)` so a draft
can never sit there looking like a release. A `+` on the hash means that tree
had uncommitted changes. `gadak doctor --json` carries the same facts per host
as `source`, `installed_by_version` and `revision`.

### Or install it as a Claude Code plugin

The repository is also a plugin marketplace, so the same skill installs
without the gadak binary at all:

```bash
claude plugin marketplace add midagedev/gadak
claude plugin install gadak@gadak
```

The plugin carries the identical `SKILL.md` (its source is the repo's
`skills/gadak/`). Pick one route — the plugin and `gadak skill install`
would each put a copy of the skill in front of the agent. The plugin
updates with `claude plugin marketplace update gadak`; the skill-install
copy updates with the binary.

## MCP (for hosts without a shell)

Shortest path — pins the **current** workspace into the registration so the host
cannot silently attach to the default mirror. The client names the app it
registers with:

```bash
gadak mcp install claude-desktop
```

for Claude Desktop, and

```bash
gadak mcp install claude
```

for Claude Code.

`claude` runs `claude mcp add` (Claude Code's own config). `claude-desktop`
writes the config Claude Desktop actually reads — it merges a `gadak` entry
into `claude_desktop_config.json` itself, because Claude Desktop has no shell
to run the CLI from. Restart Claude Desktop after installing; it reads the
file at startup.

`gadak doctor` reads that file too, and names the host it found the
registration in: `mcp: registered (~/Library/Application Support/Claude/claude_desktop_config.json, claude-desktop)`.
A registration that exists only there used to report `mcp: absent`.

To pin a named workspace:

```bash
gadak --workspace demo mcp install claude-desktop
```

The other clients print rather than register:

| Client | Command | What it does |
| --- | --- | --- |
| claude | `gadak mcp install claude` | runs `claude mcp add` (Claude Code) |
| claude-desktop | `gadak mcp install claude-desktop` | merges the entry into Claude Desktop's `claude_desktop_config.json` |
| cursor | `gadak mcp install cursor` | prints Cursor MCP config to paste (`.cursor/mcp.json`) |
| codex | `gadak mcp install codex` | prints Codex MCP config to paste (`~/.codex/config.toml`) |
| raycast | `gadak mcp install raycast` | prints values to fill into Raycast's Install New Server form |
| json | `gadak mcp install json` | prints a `mcpServers` JSON snippet only |

Raycast has no MCP config file to paste into, and its AI/MCP features may
require a paid plan.

The shortest manual path, verified end to end (this is the line in the README GIF):

```bash
claude mcp add gadak -- gadak mcp
```

Or the JSON form for hosts that take a config file:

```json
{
  "mcpServers": {
    "gadak": { "command": "gadak", "args": ["mcp"] }
  }
}
```

**Pin the workspace in the registration.** `gadak mcp` serves whatever mirror the
process environment resolves (`GADAK_HOME` / `--workspace`), and MCP hosts do not
inherit your shell exports. If the agent should see a non-default mirror, put
it in the command itself:

```bash
claude mcp add gadak -- gadak --workspace demo mcp
```

Same store, tool-shaped: see [`docs/MCP.md`](MCP.md). If the agent can run
shell commands, prefer the skill or raw CLI — no tool schemas in the context
window.

## Why this is worth a paste

An agent paging the Jira REST API spends tokens on pagination, guesses at JQL,
and hits shared rate limits. Against the mirror the same question is one
process spawn and one SQL statement — and questions Jira cannot answer at all
(reopen history, cross-issue aggregates) become ordinary queries.
