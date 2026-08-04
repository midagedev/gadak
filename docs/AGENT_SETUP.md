# Giving your coding agent access to scry

One paste per tool. Each block teaches the agent that the mirror exists, how to
query it, and the one mistake that silently returns nothing. The full reference
is [`AGENTS.md`](../AGENTS.md).

## Claude Code — `CLAUDE.md` (project or user scope)

```markdown
## Jira (via scry local mirror)

Jira issues are mirrored to a local SQLite file. Prefer these over any Jira API:

- `scry issue <KEY>` — everything about one issue (fields, description,
  comments, history, links) in one call. Add `--json` for structure.
- `scry search "<text>" --json` — full-text over summaries, bodies, comments.
- `scry sql "<select …>"` — read-only SQL. Schema: `scry sql ".schema"` or
  specs/000-product/data-model.md in the scry repo.
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
