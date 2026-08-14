# gadak Documentation

## Start here

- `STATE_OF_PLAY.md`: **picking this up? read this first.** What exists, what does
  not, the next task, and hard-won Jira behaviors
- `../README.md`: what gadak is, quick start, scope
- `ROADMAP.md`: ordering and reasoning
- `CONCEPT.md`: the idea, the two surfaces, and good-fit / bad-fit
- `ARCHITECTURE.md`: components, module boundaries, data flow
- `FAQ.md`: hard questions, and how gadak compares to jira-cli / Linear / Rovo
- `EXTRACTION.md`: where this code came from, what was cut at extraction time

## Using gadak (humans and agents)

- `../AGENTS.md`: agent reference — SQL cookbook, CLI, REST, MCP
- `AGENT_SETUP.md`: one paste per agent (Claude Code, Cursor, Codex, MCP)
- `AGENT_ACCESS.md`: which access layer to reach for
- `RECIPES.md`: questions JQL cannot ask, as ready-to-run SQL
- `decisions/0007-jql-subset.md`: JQL is a filter interchange, not an engine
- `MCP.md`: MCP server setup for hosts without a shell
- `CONFIGURATION.md`: `config.json` keys, defaults, floors
- `EXTENDING.md`: config, enrichments, and SQL extension axes
- `PLUGINS.md`: enrichment contract and payload shapes

## Contributing and ops

- `GOOD_FIRST_ISSUES.md`: concrete starter work with code evidence
- `MEDIA.md`: regenerating demo GIFs/MP4
- `runbooks/local-dev.md`: setup, frontend iteration, seeding a demo Jira

## Specs and contracts

- `../.specify/memory/constitution.md`: rules that override preference
- `../specs/000-product/spec.md`: problem, scope, requirements, acceptance
- `../specs/000-product/tasks.md`: honest state of every piece
- `../specs/000-product/data-model.md`: the SQLite schema, and how much of it is promised
- `../specs/000-product/contracts/api.md`: HTTP contract the UI already speaks
- `../specs/000-product/contracts/sync.md`: how the mirror stays correct
- `../specs/000-product/contracts/agent.md`: guarantees made to agents

## Decisions

- `decisions/0001-project-shape.md`: extracted client, new local server
- `decisions/0002-stack.md`: Go + SQLite, Svelte client
- `decisions/0003-local-process.md`: why browser-only is impossible
- `decisions/0004-browser-sqlite.md`: SQLite in the browser, demo only
- `decisions/0005-three-surfaces.md`: two surfaces (web + CLI); TUI retired
