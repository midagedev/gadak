# scry Documentation

## Start here

- `STATE_OF_PLAY.md`: **picking this up? read this first.** What exists, what does
  not, the next task, and hard-won Jira behaviors
- `../README.md`: what scry is, quick start, scope
- `CONCEPT.md`: the idea and what follows from it
- `ARCHITECTURE.md`: components, module boundaries, data flow
- `AGENT_ACCESS.md`: practical guide to querying the mirror
- `EXTRACTION.md`: where this code came from, what was cut, what remains
- `ROADMAP.md`: ordering and reasoning

## Specs and contracts

- `../.specify/memory/constitution.md`: rules that override preference
- `../specs/000-product/spec.md`: problem, scope, requirements, acceptance
- `../specs/000-product/tasks.md`: honest state of every piece
- `../specs/000-product/data-model.md`: the SQLite schema, a public contract
- `../specs/000-product/contracts/api.md`: HTTP contract the UI already speaks
- `../specs/000-product/contracts/sync.md`: how the mirror stays correct
- `../specs/000-product/contracts/agent.md`: guarantees made to agents

## Operations

- `runbooks/local-dev.md`: setup, frontend iteration, seeding a demo Jira

## Decisions

- `decisions/0001-project-shape.md`: extracted client, new local server
- `decisions/0002-stack.md`: Go + SQLite, Svelte client
- `decisions/0003-local-process.md`: why browser-only is impossible
- `decisions/0004-browser-sqlite.md`: SQLite in the browser, demo only
