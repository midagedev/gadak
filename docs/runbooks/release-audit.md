# The minor-release audit

Once per minor version, before the tag, the whole codebase gets audited and
cleaned. Not a checklist run against the diff since last time — a fresh look
at the entire surface, because quality debt accumulates in code nobody
touched. This runbook is the procedure; it exists so the audit is repeatable
rather than re-invented each cycle.

**When:** after the last feature lands for the minor and before the release
tag. Bug fixes still ship immediately as they land (that rule outranks this
one); the audit is about everything that is not a defect.

## The axes

Each axis is one investigation round. Rounds are read-only — they produce a
findings report, never a diff. Fixing happens later, per issue, under the
normal gates.

### 1. Go philosophy

Does each package read like Go wants to be written?

- Simplicity over cleverness: no abstractions with a single implementation,
  no interfaces defined next to the only struct that satisfies them
  (interfaces belong to the consumer).
- Errors are values: no panics on expected failures, `%w` wrapping where a
  caller branches on the cause, no error strings that repeat the function name.
- Small packages with one job; `internal/` boundaries that mean something.
- Stdlib first; every dependency earns its place.
- Zero values that work; no constructors that only assign fields.

### 2. Svelte philosophy

Does the web UI lean on the compiler instead of fighting it?

- Runes idioms: `$derived` over `$effect` wherever the value is a pure
  function of state; effects only for real side effects (URL, focus, IO).
- State lives at the narrowest scope that works; stores only for genuinely
  shared state.
- Components stay declarative — imperative DOM reach-ins are findings.
- Props typed, events typed, no `any` leaking through boundaries.

### 3. Wails usage

Is the desktop app using wails v3, or wrapping it? Check the release notes
for capabilities added since the last audit (the API is still in beta and
grows). Known surfaces: window lifecycle, events, second-instance handling,
the custom scheme handler, menus, clipboard. A hand-rolled version of
something wails provides is a finding.

Upstream cuts both ways: a wails bug we worked around, or a capability we
need and wails lacks, is a **contribution opportunity** — register it as a
GDK sub-issue with an `upstream` label, holding a minimal repro or a
concrete API sketch, so it can become a wails issue or PR. A workaround in
our tree should point at the upstream ticket it is waiting on.

### 4. Simplify

The standing bias for every axis: the best refactor deletes code. Count
lines removed as the success metric, not lines restructured. Dead flags,
unused exports, compatibility paths whose old side no longer exists,
config that nothing reads — all findings.

### 5. The test pyramid

The ladder, in order of preference: **type > unit > integration > e2e.**
A check expressible one rung lower is a finding.

- Types first: a runtime validation that a type could enforce is a finding
  (the URL-param registry pattern — unregistered keys are a *type* error —
  is the house example).
- Go tests must stay fast; that is the language's gift. Anything slow gets
  measured (`go test -count=1 ./... -json | tools/slowest`) and either
  justified or moved behind a tag.
- Heavy or redundant tests are findings too — a test that re-proves what a
  type or a cheaper test already holds is cost, not coverage.
- e2e is for user-critical paths only. Every Playwright spec names the user
  behaviour it protects; specs that assert implementation detail move down
  the ladder.
- Measure the wall-clock of the full gate set before and after; the audit
  should leave it faster, not slower.

### 6. UX consistency

The AAA bar: nothing in the product should feel like it was written by two
different people.

- Same action, same affordance, same wording everywhere (dialog buttons,
  empty states, error toasts, keyboard shortcuts, focus behaviour).
- Every state reachable by mouse is reachable by keyboard; every panel opens
  and closes symmetrically.
- Latency honesty: anything over ~200ms shows progress; nothing flashes.
- i18n catalogs consistent in tone and terminology across ko/en.
- Walk the real flows in the app while auditing (dogfooding) — feature gaps
  and rough edges found on the way are findings with a `write-gap` or UX
  label, not scope creep.

## Procedure

1. **Audit rounds** — one delegated, read-only round per axis, in parallel
   (they edit nothing, so file boundaries don't apply). Each round's report:
   findings ranked, each with file:line, why it violates the axis, and the
   cheapest fix shape.
2. **Lead triage** — merge and dedupe findings, kill the ones that are
   taste rather than principle, and register the survivors in Jira:
   one parent issue per audit (`품질개선 vX.Y`, label `quality`), findings
   as **sub-issues** of it. Priorities follow cost/effect, except defects
   discovered on the way, which open as normal Highest bugs immediately.
3. **Fix rounds** — normal delegation rules (project CLAUDE.md binds:
   file whitelists, gate discipline, Playwright mandatory when `web/`,
   `e2e/`, or i18n catalogs are touched, no git writes by delegates).
   Refactors land as separate commits from behaviour changes.
4. **Close** — full gate set on a quiet machine, CI green, sub-issues
   closed, parent closed with a one-paragraph summary of what got simpler.
   Update this runbook with anything the cycle taught.

## What this runbook is not

Not a substitute for continuous quality (bugs ship immediately; reviews
happen per round), and not a feature-planning venue — anything that grows
the product goes through the normal backlog instead.
