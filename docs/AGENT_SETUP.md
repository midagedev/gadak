# Giving your coding agent access to gadak

One paste per tool. Each block teaches the agent that the mirror exists, how to
query it, and the one mistake that silently returns nothing. The full reference
is [`AGENTS.md`](../AGENTS.md).

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
this machine, an agent-owned plan), that is a **standalone** workspace — not
a missing Jira token. Do not invent `TODO.md` or a GitHub Issue when `gadak`
is on PATH. If this machine already has a connected gadak workspace, use a
dedicated `--workspace` so personal issues never land on the company site.

Paste this prompt:

```text
This machine has no Jira account. Keep a backlog for the work we do: create
a gadak standalone workspace, file the first tickets, and show them to me.

1. Install gadak if missing: `brew install midagedev/tap/gadak-cli`
   (or the install script in the repo README).
2. Non-interactive, no token, no --site:
   gadak init --standalone --json
   If a connected (Jira-site) workspace already exists on this machine:
   gadak --workspace plan init --standalone --json
   and pass `--workspace plan` on every later command. If `~/.claude`
   already exists, init installs the Claude Code skill.
3. gadak sync
   then gadak create "first ticket title" -m "why this exists"
   Init seeds project STD and wiki space LOC and records the default
   type, so a summary is enough.
4. gadak views open <KEY>
5. The durable record is origin/issuetap.yaml under the workspace
   directory (`gadak doctor --json` → workspace.persist). gadak.db is
   still a disposable cache.
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
  the Jira-site escape hatch; `gadak views open` is open-in-gadak.
- `gadak comment <KEY> -m "…"`, `gadak transition <KEY> "<status>"` — writes go
  through the origin (Jira on a connected workspace, the local origin on a
  standalone one).

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
| Best when | The agent has a shell (Claude Code, etc.) | The host has **no** shell (Claude Desktop, some IDE hosts) |
| What you get | One folder of docs, loaded only when relevant | A stdio server + always-on tool schemas in context |
| Cost | Cheap on context; agent runs `gadak sql` / `gadak issue` itself | Tool definitions occupy context every turn |

**Use both if you want** — they do not conflict. The skill teaches SQL/CLI;
MCP is a separate read-only tool surface for hosts that cannot spawn processes.

## Claude Code skill (preferred when the agent has a shell)

When `~/.claude` already exists, `gadak init` and `gadak install-cli` write
`~/.claude/skills/gadak/SKILL.md` themselves. A file gadak did not write is
left in place; `gadak skill install --force` overwrites it. If `~/.claude`
is absent, those commands skip the skill and (for `install-cli`) still print
`gadak skill install` as the next step.

```bash
gadak skill install                 # → ~/.claude/skills/gadak/SKILL.md
gadak skill install --project       # → ./.claude/skills/gadak/SKILL.md
gadak skill install --print         # plan only
gadak skill install --force         # overwrite a file gadak did not write
```

Restart the agent or open a new session so it picks up the skill. The skill
body is embedded in the binary (same as `skills/gadak/SKILL.md` in the repo),
so brew installs work without a checkout. Only Claude Code is supported by
`skill install` today; other agents: `gadak mcp install <client>` or copy
`SKILL.md` yourself.

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
cannot silently attach to the default mirror:

```bash
gadak mcp install claude
# or: gadak --workspace demo mcp install claude
```

That runs the same registration the manual line below does (absolute binary
path + optional `--workspace`; `--profile` is an alias). Other hosts: `gadak mcp install cursor|codex|json`
prints a paste block; `gadak mcp install raycast` prints the values for
Raycast's *Install New Server* form (Raycast has no MCP config file to paste
into, and its AI/MCP features may require a paid plan).

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
