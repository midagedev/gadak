# Giving your coding agent access to scry

One paste per tool. Each block teaches the agent that the mirror exists, how to
query it, and the one mistake that silently returns nothing. The full reference
is [`AGENTS.md`](../AGENTS.md).

## Let the agent set scry up for you

If a coding agent (Claude Code, Codex, …) already talks to Jira on this
machine — through environment variables, an MCP connector, or its own memory —
it already holds everything scry needs. Paste this prompt into the agent:

```text
Set up scry (https://github.com/midagedev/scry) against the Jira account this
machine already uses. Steps:

1. Find my Jira credentials wherever they already live: JIRA_URL /
   JIRA_EMAIL / JIRA_API_TOKEN environment variables (check ~/.zshrc or
   ~/.bashrc), an Atlassian MCP server entry in .mcp.json or the Claude
   config, or a .env file. Do not ask me for them unless nothing exists.
2. Install scry if missing: `brew install midagedev/tap/scry`
   (or the install script in the repo README).
3. Configure non-interactively — init never prompts when values are supplied:
   SCRY_TOKEN=<token> scry init --site <url> --email <email> --json
   Leave --projects off: an empty project list syncs every project I can see,
   and the first line of sync output shows the total so we can narrow later
   with `scry init --projects KEY1,KEY2` if it is huge.
4. Run `scry sync`. The first full sync auto-discovers the site's custom
   fields (labels, filter axes, editors) and prints what it configured.
5. Verify: `scry sql "select count(*) from issues_full"` returns a number,
   and `scry fields` lists the discovered mapping.
6. Add the scry usage block from docs/AGENT_SETUP.md to my agent config so
   future sessions query the mirror instead of the Jira REST API.
```

Everything the agent configures stays editable afterwards: field mapping in
the web UI under Settings → Fields (edits are pinned and survive
re-discovery), or `scry fields --apply` to re-run detection, or the `fields`
array in `~/.scry/config.json` directly.

## Claude Code — `CLAUDE.md` (project or user scope)

```markdown
## Jira (via scry local mirror)

Jira issues are mirrored to a local SQLite file. Prefer these over any Jira API:

- `scry issue <KEY>` — everything about one issue (fields, description,
  comments, history, links) in one call. Add `--json` for structure.
- `scry search "<text>" --json` — full-text over summaries, bodies, comments.
- `scry sql "<select …>"` — read-only SQL (SQLite mode=ro). Schema:
  specs/000-product/data-model.md in the scry repo (or
  `SELECT sql FROM sqlite_master WHERE type IN ('table','view')`).
- `scry comment <KEY> -m "…"`, `scry transition <KEY> "<status>"` — writes go
  through to Jira.

Rules: filter on `status_category` ('new'|'inprogress'|'done') and ids, never
on display names — Jira localizes those per account. Query the `issues_full`
view (it includes `summary`); the bare `issues` table has no title column. If
the mirror warns it is stale, mention that in your answer.
```

## Cursor — `.cursor/rules/scry.mdc`

```markdown
---
description: Query Jira through the local scry mirror instead of the REST API
alwaysApply: false
---

When the user asks about Jira issues, use the scry CLI against the local
mirror: `scry issue <KEY> --json`, `scry search "<text>" --json`, or
`scry sql "<select …>"` (read-only). Filter on status_category, not display
names. Use the `issues_full` view — it includes the issue title as `summary`.
```

## Codex — `AGENTS.md` (repo root)

```markdown
## Jira access

Use the scry local mirror, not the Jira REST API:
`scry issue <KEY> --json` · `scry search "<text>" --json` ·
`scry sql "<read-only select>"`. Filter on `status_category` and ids, never
display names. Use the `issues_full` view for titles.
```

## MCP (for hosts without a shell)

```json
{
  "mcpServers": {
    "scry": { "command": "scry", "args": ["mcp"] }
  }
}
```

Same store, tool-shaped: see [`docs/MCP.md`](MCP.md). If the agent can run
shell commands, the CLI is cheaper — no tool schemas in the context window.

## Why this is worth a paste

An agent paging the Jira REST API spends tokens on pagination, guesses at JQL,
and hits shared rate limits. Against the mirror the same question is one
process spawn and one SQL statement — and questions Jira cannot answer at all
(reopen history, cross-issue aggregates) become ordinary queries.
