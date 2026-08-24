# gadak Documentation

## Start here

A route, not a contributor reading list. Each line is the question that
document answers.

- [README.md](../README.md): what is gadak, and how do I try it?
- [INSTALL.md](INSTALL.md): how do I install the CLI or the app?
- [DESKTOP.md](DESKTOP.md): when do I want the desktop window instead of `gadak serve`?
- [AGENT_SETUP.md](AGENT_SETUP.md): how do I give a coding agent access (one paste per tool)?
- [MCP.md](MCP.md): how do I wire MCP on a host that has no shell?
- [FAQ.md](FAQ.md): what should I ask before pointing this at a company Jira?
- [BENCHMARKS.md](BENCHMARKS.md): how does the local mirror compare to the live REST API?

## Using gadak

- [MIRROR.md](MIRROR.md): SQL, CLI, REST, and MCP against the local mirror — the cookbook
- [AGENT_ACCESS.md](AGENT_ACCESS.md): which access layer to reach for, and what each costs
- [DASHBOARDS.md](DASHBOARDS.md): agent-authored dashboards — the frame contract, datasources, vendored charts, and the two residual leak channels
- [RECIPES.md](RECIPES.md): questions JQL cannot ask, as ready-to-run SQL
- [CONFIGURATION.md](CONFIGURATION.md): `config.json` keys, defaults, floors
- [NETWORK.md](NETWORK.md): what gadak does with the network — mirror reads stay local; writes go to the origin you configured
- [WINDOWS-SIGNING.md](WINDOWS-SIGNING.md): Windows showed a warning — which file you have, and why releases are unsigned
- [EXTENDING.md](EXTENDING.md): config, enrichments, and SQL extension axes
- [PLUGINS.md](PLUGINS.md): enrichment contract and payload shapes
- [PAIN_POINTS.md](PAIN_POINTS.md): sourced Jira complaints, and what gadak does about each

## Developing gadak

If you are changing this repository, not if you are using it.

- [STATE_OF_PLAY.md](STATE_OF_PLAY.md): what actually exists right now, the next task, and hard-won Jira behaviors
- [ROADMAP.md](ROADMAP.md): ordering and reasoning
- [ARCHITECTURE.md](ARCHITECTURE.md): components, module boundaries, data flow
- [CONCEPT.md](CONCEPT.md): the idea, the two surfaces, and good-fit / bad-fit
- [EXTRACTION.md](EXTRACTION.md): where this code came from, what was cut at extraction time
- [DERIVE.md](DERIVE.md): columns gadak computes at sync (reopens, resolution dates, priority rank, epic keys) — not source fields
- [UX_PRINCIPLES.md](UX_PRINCIPLES.md): the standard every UI wave is measured against
- [GOOD_FIRST_ISSUES.md](GOOD_FIRST_ISSUES.md): concrete starter work with code evidence
- [MEDIA.md](MEDIA.md): regenerating demo GIFs/MP4
- [../AGENTS.md](../AGENTS.md): contributor contract — required reading order, SQL/CLI/REST/MCP pointers

### Specs and contracts

- [../.specify/memory/constitution.md](../.specify/memory/constitution.md): rules that override preference
- [../specs/000-product/spec.md](../specs/000-product/spec.md): problem, scope, requirements, acceptance
- [../specs/000-product/tasks.md](../specs/000-product/tasks.md): honest state of every piece
- [../specs/000-product/data-model.md](../specs/000-product/data-model.md): the SQLite schema, and how much of it is promised
- [../specs/000-product/gates.md](../specs/000-product/gates.md): objective bars for calling a phase done
- [../specs/000-product/contracts/api.md](../specs/000-product/contracts/api.md): HTTP contract the UI already speaks
- [../specs/000-product/contracts/sync.md](../specs/000-product/contracts/sync.md): how the mirror stays correct
- [../specs/000-product/contracts/agent.md](../specs/000-product/contracts/agent.md): guarantees made to agents

### Runbooks

- [runbooks/local-dev.md](runbooks/local-dev.md): setup, frontend iteration, seeding a demo Jira
- [runbooks/install-verification.md](runbooks/install-verification.md): verifying the shipped bundle on each OS
- [runbooks/release-audit.md](runbooks/release-audit.md): per-minor full-codebase audit procedure
- [runbooks/upstream-pr.md](runbooks/upstream-pr.md): how this repository sends changes to upstream projects
- [runbooks/confluence-space-scope.md](runbooks/confluence-space-scope.md): Confluence space-scope residue and backfill
- [runbooks/omarchy-vm.md](runbooks/omarchy-vm.md): Omarchy QEMU guest used to verify install and the bar widget
- [runbooks/signpath-application.md](runbooks/signpath-application.md): SignPath Foundation application checklist (lead-only)

### Decisions

Two files on disk share the number `0007`
(`0007-jql-subset.md` and `0007-rename-to-gadak.md`). The filenames are left
as they are — renumbering a decision file is a revision, and decisions are
addendum-only. This index lists every file.

- [decisions/0001-project-shape.md](decisions/0001-project-shape.md): extracted client, new local server
- [decisions/0002-stack.md](decisions/0002-stack.md): Go + SQLite, Svelte client
- [decisions/0003-local-process.md](decisions/0003-local-process.md): why browser-only is impossible
- [decisions/0004-browser-sqlite.md](decisions/0004-browser-sqlite.md): SQLite in the browser, demo only
- [decisions/0005-three-surfaces.md](decisions/0005-three-surfaces.md): **superseded** (2026-08-13) — title is
  "Two surfaces over one store" (web + CLI; TUI retired); filename still says
  three-surfaces
- [decisions/0006-confluence-connector.md](decisions/0006-confluence-connector.md): Confluence as a second source, same spine
- [decisions/0007-jql-subset.md](decisions/0007-jql-subset.md): JQL is a filter interchange, not an engine
  (accepted, 2026-08-14)
- [decisions/0007-rename-to-gadak.md](decisions/0007-rename-to-gadak.md): product renamed from scry to gadak
  (2026-08-13; the file has no Status line)
- [decisions/0008-cli-first-parity.md](decisions/0008-cli-first-parity.md): CLI-first parity — every capability with user value is executable headless
- [decisions/0009-cjk-mid-compound-search.md](decisions/0009-cjk-mid-compound-search.md): CJK mid-compound search is app-layer bigrams, not a different tokenizer
- [decisions/0010-json-issue-key-alias.md](decisions/0010-json-issue-key-alias.md): JSON surfaces carry both `issue_key` and `key`

### Research

- [research/write-usecase-census.md](research/write-usecase-census.md): write-path census of the two surfaces; design input, no production code
