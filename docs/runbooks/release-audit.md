# The minor-release audit

Once per minor version, before the tag, the whole codebase gets audited and
cleaned. Not a checklist run against the diff since last time — a fresh look
at the entire surface, because quality debt accumulates in code nobody
touched. This runbook is the procedure; it exists so the audit is repeatable
rather than re-invented each cycle.

**When:** after the last feature lands for the minor and before the release
tag. Each cycle's record — axes covered, numbers measured, findings routed
— goes in [`../project/AUDIT_LOG.md`](../project/AUDIT_LOG.md). Bug fixes still ship immediately as they land (that rule outranks this
one); the audit is about everything that is not a defect.

**What changed in this edition (v0.22 cycle).** The axes used to be
principles without numbers, and five cycles of findings (116, classified in
GDK-972) showed what that costs: 21% of findings were plain bugs that got
audit priority instead of Highest, 12% were taste that nobody could reject
on the record, and the product invariants were never checked on purpose.
Now every cycle **starts with a census** — measured hotspots, a CI ledger,
a surface matrix, an i18n gap list, a fact map — and each axis opens from
those numbers. Six axes became twelve; the new ones are the agent-surface
check, the changelog, CI cost, leverage residue, i18n completeness, the fact
ledger, and the invariants. Defects and rejections get their own routing.

## Step 0 — the census

Run before any axis, on a quiet machine, in a worktree at the audit's base
SHA. Every number below is produced by a command, so the runbook carries
the command and the report carries the number. **Exclude
`.claude/worktrees/`, `node_modules`, `mobile/src-tauri/gen`** from every
count (an included agent worktree triples every duplicate). Read-only git
only; the outsource guard blocks `git tag -l` and `git ls-tree` — use
`git for-each-ref refs/tags` and `git show <tag>:<path>`.

| Census | Command | Feeds |
|---|---|---|
| Size | `git ls-files '*.go' '*.svelte' '*.ts' \| xargs wc -l \| sort -n` (split test/non-test; per-package totals for `internal/*`, `cmd/gadak`, `desktop`) | axes 1, 2, 4 |
| Function complexity | `go run github.com/fzipp/gocyclo/cmd/gocyclo@v0.6.0 -top 30 .` and `go run github.com/uudashr/gocognit/cmd/gocognit@v1.1.3 -top 30 .` (pass `.`, filter `desktop/` by path; tables with and without `_test.go`). Svelte/TS: functions over 60 lines, files with more than three `$effect(` | axes 1, 2 |
| Churn × complexity | `git log --name-only --format='' \| sort \| uniq -c \| sort -rn` joined with the two above; the top 20 is the hotspot list the axes open first | axes 1, 2, 4 |
| Coupling | `go list -f '{{.ImportPath}} {{.Imports}} {{.TestImports}}' ./...` → fan-in / fan-out per `internal/*` package, cycles, single-consumer packages. Always join `TestImports`: a gate package whose only importers are tests reads as dead otherwise (`internal/archlint`) | axis 1 |
| Duplication | `go run github.com/mibk/dupl@latest -t 40 .` (Go); a 5-gram token-shingle Jaccard over `.svelte` for the web (no Go-only tool covers it) | axis 4 |
| Trend | the per-package size totals at the last three tags via `git show <tag>:<path> \| wc -l`; one table package × tag | axis 4, close |
| CI ledger | `gh run view <id> --json jobs --jq '.jobs[] \| [.name, ((.completedAt\|fromdate)-(.startedAt\|fromdate))] \| @tsv'` for the run at each of the last three tags and the current head; wall = slowest shard, billable = sum. A `cancelled` run is not a data point for its cancelled jobs — say so in the table | axis 9 |
| E2E per spec | the E2E job logs (`gh run view <id> --log --job <job>`): Playwright prints per-test seconds; aggregate per file; the top 25 | axes 5, 9 |
| Go slowest | `go test -race -count=1 -json ./... > /tmp/gotest.json && go run ./tools/slowest /tmp/gotest.json` (the only heavy command; nothing else runs beside it) | axis 5 |
| Surface matrix | one row per `[GDK-nnnn]` cited in `CHANGELOG.md` Unreleased × {CLI, web, MCP description, SKILL.md, phone}; a `yes` cites `path:line`, a `MISSING` cites the grep. The Unreleased range is `## Unreleased` to the next `## v`-heading — `^## 0\.` matches nothing | axes 7, 10 |
| i18n gaps | a script over `web/src/lib/i18n/messages/*.ts` listing keys whose ko or ja is missing, empty or byte-equal to en (allowlist printed with reasons); a template scan of `web/src` and `mobile/src` for user-visible ASCII outside `t()`; the ko/ja site pages for three-or-more-word English runs; every `Intl.`/`toLocale` site for whether it takes the active locale | axis 11 |
| Fact map | for each clause of `docs/project/FACT_LEDGER.md`, every copy in the READMEs, `site/`, `docs/`, `skills/gadak/SKILL.md`, `site/public/llms.txt`, found by the fact's **values** (numbers, commands, hosts), with `agrees / DISAGREES` and whether `tools/doc-checks.sh` guards it | axis 12 |

The census is one read-only delegated round per row group (five rounds ran
in parallel in the v0.22 cycle, 38–70 minutes each). Its reports are inputs,
not findings: an axis round reads them and opens the code from there.

The baseline the v0.22 census set, so the next one can say which way things
moved: Go 219,183 lines (test 1.3× production), mean non-test cyclomatic
5.35 over 3,011 functions, seven functions at cyclomatic ≥ 69 (the worst:
`cmd/gadak/init.go` at 100, `internal/migrate/linear.go` at 95, three in
`internal/sync`), `internal/server` fan-out 23, no import cycles, CI wall
438 s and billable 3,358 s (up 50% on v0.19.0's 2,241 s), 1,303 catalog keys
with two real ja fall-throughs, 122 fact copies with 10 disagreeing.

## The axes

Each axis is one investigation round. Rounds are read-only — they produce a
findings report, never a diff. Fixing happens later, per issue, under the
normal gates. Every finding names `file:line`, the census number it stands
on where one exists, why it violates the axis, and the cheapest fix shape.
A round reports two lists, not one: **axis findings** and **defects** —
see *Procedure*.

### 1. Go philosophy — from the hotspots

Open the churn × complexity top 20 first, then the seven-or-so functions
above cyclomatic 60, then the package with the largest fan-out. The
questions stay the ones Go asks:

- Simplicity over cleverness: no abstractions with a single implementation,
  no interfaces defined next to the only struct that satisfies them
  (interfaces belong to the consumer).
- Errors are values: no panics on expected failures, `%w` wrapping where a
  caller branches on the cause, no error strings that repeat the function name.
- Small packages with one job; `internal/` boundaries that mean something.
  A package with one consumer is a boundary to justify or fold; a fan-out
  above 15 is a package that has become the place everything meets.
- Stdlib first; every dependency earns its place.
- Zero values that work; no constructors that only assign fields.

Numbers that are findings on sight: a non-test function at cyclomatic ≥ 30
(the threshold the v0.22 baseline puts seven functions over 69 above), a
production file over 1,500 lines with more than 30 commits, a `dupl` pair
inside one file (`internal/config/settings.go:988-1054` self-clones was the
v0.22 example). A number is where the reading starts, not the verdict — a
`switch` over a wire enum is legitimately wide.

### 2. Svelte philosophy — from the hotspots

Open the components with more than three `$effect(` blocks and the
functions over 60 lines first.

- Runes idioms: `$derived` over `$effect` wherever the value is a pure
  function of state; effects only for real side effects (URL, focus, IO).
- State lives at the narrowest scope that works; stores only for genuinely
  shared state.
- Components stay declarative — imperative DOM reach-ins are findings.
- Props typed, events typed, no `any` leaking through boundaries.

### 3. The app shells — wails (desktop) and Tauri (phone)

Two shells, one axis, because the question is the same on both: are we using
the framework, or wrapping it?

**wails v3** (`desktop/`, a separate go.mod). Check the release notes for
capabilities added since the last audit — the API is still in beta and
grows. Surfaces it does provide: window lifecycle
(<https://v3.wails.io/features/windows/events/>), events
(<https://v3.wails.io/features/events/system/>), single-instance
(<https://v3.wails.io/guides/single-instance/>), menus, clipboard, and the
asset scheme handler. A hand-rolled version of any of those is a finding.

Be exact about what "wails provides" covers, because the list above has been
read too widely. **OS protocol registration is not on it.** `gadak://` is
registered by `CFBundleURLTypes` that `desktop/build-app.sh` writes into
Info.plist on macOS, and by hand-rolled registry code in
`desktop/protocol_windows.go` — wails has no API for it, so that code is not
a finding, it is the only way. The scheme handler wails owns serves the app's
own assets, which is a different thing wearing a similar name.

**Tauri v2** (`mobile/`, its own Cargo workspace and lockfile). The audit
surface is bigger than "is the Rust idiomatic", because Tauri v2 moves two
product-critical things into config:

- `mobile/src-tauri/capabilities/*.json` is a **security artifact**, not
  build config. It declares what the webview may call, and a permission
  granted there outranks any guard written in TypeScript — JS can always
  call the plugin directly. Read it against the outbound invariant every
  cycle: an entry that widens what the app may dial, with the narrowing
  living only in frontend code, is a finding even when nothing currently
  misuses it. The file's own comment already names one (GDK-897).
- Plugin inventory and version drift. Every `tauri-plugin-*` is a dependency
  that earns its place like any other, and a plugin pinned to a different
  major than the rest of the toolchain is a finding on sight.

The installed-artifact rule below applies here too, with the `.ipa` as the
artifact: `mobile/scripts/testflight-upload.sh` verifies its contract before
upload, and `docs/runbooks/testflight-release.md` is the procedure.

Upstream cuts both ways on both shells: a framework bug we worked around, or
a capability we need and the framework lacks, is a **contribution
opportunity** — register it as a GDK sub-issue with an `upstream` label,
holding a minimal repro or a concrete API sketch, so it can become a wails or
Tauri issue or PR. A workaround in our tree should point at the upstream
ticket it is waiting on. `docs/runbooks/upstream-pr.md` is the pipeline, and
its second stage is the one that matters here: a defect confirmed by reading
the framework's code is a hypothesis until it is reproduced.

**Verify the shipped bundle, not the script that writes it.** Anything the
OS reads from `Info.plist` — the `gadak://` scheme above all — is invisible
to a source test: the repo can be right while the artifact is wrong, and the
symptom is a link that does nothing at all. Before tagging, install the built
app the way a user does (dmg → /Applications, not the repo's `build/`),
confirm `lsregister -dump | grep gadak:` binds to that copy, and open one
link cold (app not running) and one warm. A locally built copy registered
during development will out-claim the installed one — unregister it first,
or the test proves nothing about what ships.

The same rule generalizes: for any capability the OS or another app consumes
(scheme, file type, service), the release audit tests the installed artifact.

The step-by-step version of that, for all three platforms and with the traps
named, is [`install-verification.md`](install-verification.md). Two things it
adds that this paragraph does not: the check runs **twice** — once before the
tag on the previous release's artifacts, and once on the new artifacts after
the release publishes and before announcing, because signing happens in CI and
a locally built bundle proves nothing about Gatekeeper — and on macOS the
install *method* is part of the test, since a quarantined bundle runs from a
randomized App Translocation path where nothing that depends on the bundle's
location holds.

### 4. Simplify — count lines removed

The standing bias for every axis: the best refactor deletes code. Count
lines removed as the success metric, not lines restructured. Dead flags,
unused exports, compatibility paths whose old side no longer exists,
config that nothing reads — all findings. The census's duplication pairs and
the trend table are the starting list: a package that grew 40%+ in one
release (`httppolicy` +92%, `atlhttp` +47% in v0.22) is where the
compatibility paths pile up. `bash tools/staticcheck.sh` is a gate now (CI
`Staticcheck`, all three GOOS); a finding it reports is already red.

### 5. The test pyramid — from the per-spec and slowest tables

The ladder, in order of preference: **type > unit > integration > e2e.**
A check expressible one rung lower is a finding.

- Types first: a runtime validation that a type could enforce is a finding
  (the URL-param registry pattern — unregistered keys are a *type* error —
  is the house example).
- Go tests stay fast; that is the language's gift. The `tools/slowest` top
  20 is the list: a test that sleeps, polls wall-clock, or boots serve gets
  justified or moved behind a tag.
- Heavy or redundant tests are findings too — a test that re-proves what a
  type or a cheaper test already holds is cost, not coverage.
- e2e is for user-critical paths only. Every Playwright spec names the user
  behaviour it protects in its header; a spec in the per-file top 25 that
  asserts copy or implementation detail moves down the ladder (the
  precedent is `e2e/settings.spec.ts` source-scan cases), keeping one
  real-path case per spec.
- The gate set's wall-clock is measured before and after; the audit leaves
  it faster, not slower, and axis 9 carries the numbers.

This axis and axis 9 read the same tables and can reach the same test from
two directions — "move it down a rung" and "cut the seconds". **The split
is by verdict, not by file: axis 5 owns every proposal that changes where a
check lives, axis 9 owns every proposal that changes what CI runs or how it
is scheduled.** A test that moves rungs is axis 5's finding even when the
motive is minutes; a shard rebalance is axis 9's even when it lands in the
same commit. Say which axis a merged round is reporting under.

### 6. UX consistency

The AAA bar: nothing in the product should feel like it was written by two
different people.

- Same action, same affordance, same wording everywhere (dialog buttons,
  empty states, error toasts, keyboard shortcuts, focus behaviour).
- Every state reachable by mouse is reachable by keyboard; every panel opens
  and closes symmetrically.
- Latency honesty: anything over ~200ms shows progress; nothing flashes.
- i18n catalogs consistent in tone and terminology across ko/en/ja (axis 11
  measures completeness; this axis reads tone).
- Walk the real flows in the app while auditing (dogfooding) — feature gaps
  and rough edges found on the way are findings with a `write-gap` or UX
  label, not scope creep.

### 7. Agent-surface coverage (GDK-1700)

A feature that shipped on the CLI or the web and is absent from
`skills/gadak/SKILL.md` and the tool descriptions in `internal/mcp/tools.go`
does not exist for an agent — and the MCP descriptions have no gate, so a
description that teaches a value the server does not emit passes every
check. The surface matrix is the whole axis: every row where CLI or web is
`yes` and SKILL or MCP is `MISSING` is a finding, ranked new verb > new flag
> wording, each with the sentence that closes it and where it goes. The
v0.22 census found five (the `jira-server` origin invisible to the skill,
`comment edit`/`rm`, `sync --concurrency`, `api --headers`, and an MCP
`status` that lacked the `first_sync` object the CLI prints).

### 8. The changelog (GDK-1701)

`tools/doc-checks.sh` #46 measures the shape — at most three bold-led theme
paragraphs per release, no top-level bullets, three editions with the same
key sets — and this axis reads the content. Per theme paragraph: does it say
what changed against the previous version, or does it narrate the fixes in
the order they landed? A paragraph that carries more than five keys in one
sentence chain, or repeats "and then it was fixed", is a finding. The lead
compresses (ko and ja included — Korean and Japanese prose is never
delegated to GLM), and `bash tools/changelog-preserve.sh <file> <ref>` proves
the headings, keys, numbers, commands and links survived; a structural
rewrite legitimately turns the code-span and link axes red, and the report
says so.

### 9. CI cost (GDK-1702)

The ledger from the census is the axis, kept across releases so the trend
is visible:

| job (longest shard) | v0.19.0 | v0.20.0 | v0.21.0 | v0.22 base |
|---|---|---|---|---|
| Server race tests | 300 | 385 | 440 | 434 |
| Playwright E2E | 314 | 322 | 331 | 403 |
| Staticcheck | — | — | 219 | 309 |
| Build and check | 200 | 211 | 235 | 221 |
| wall / billable Σ | 317 / 2,241 | 389 / 2,620 | (cancelled) / 2,988 | 438 / 3,358 |

Findings are proposals with a number on them — seconds off the deciding
shard, and billable seconds — each naming the incident in CLAUDE.md the job
guards and how the guard survives. Shards are balanced by measured seconds,
not by test count (count-split gave 220/297/261 test-seconds across the
three E2E shards). A job that never fails and never gates is a candidate to
path-filter, not to delete. Flakiness is a census, not a feeling: over the
last 60 main runs, failures, reruns and cancellations by job; in v0.22 the
window held 0 failures and 13 `cancel-in-progress` casualties — the
cancellation policy, not flakiness, is what wastes runs here.

### 10. Leverage residue (GDK-1703)

Right before the tag, with the surface matrix and the open backlog in
hand, ask per Unreleased theme: did the theme reach every surface it
belongs on (a sprint that exists on the CLI and the board but not the
phone), and which open issue would complete it for an hour or two of work?
The output is a ranked list with cost and the surface each item fills; the
decision — this release or the next — is the user's, and the list is the
question put to them.

### 11. i18n completeness (GDK-1704)

Recording the ko and ja clips is the last place English leaks are seen; this
axis is the first. The census lists: catalog keys whose ko/ja is byte-equal
to en (two real fall-throughs in v0.22: `API token` on the ja connect
dialog), literals in templates outside `t()` (27 on the phone, 1 on the web),
English runs on the ko/ja site pages, and every date/duration formatter that
does not take the active locale (a `'ko-KR'` hardcode in `QaImpact.svelte`
leaked Korean timestamps to en/ja readers — the leak runs both ways). The
catalog test now holds byte-equality with an allowlist, so this axis reads
what the gate cannot: tone and terminology, and whether a rendered screen
still shows English.

### 12. The fact ledger (GDK-1705)

`docs/project/FACT_LEDGER.md` is the source; the READMEs, the site, the
docs and the skill are copies, and `tools/doc-checks.sh` guards the clauses
the ledger marks. The census maps every copy by the fact's values; the axis
reads the disagreements and the unguarded copies. The v0.22 map found 122
copies, 10 disagreeing, all in the unguarded set — five of them one stale
fact ("Server/DC untested") that outlived the measurement that made it
false. A copy that a check cannot reach is a finding even when it agrees
today.

### 13. The invariants (GDK-972)

The product invariants at the top of CLAUDE.md are checked on purpose, not
as by-products of other axes. Five cycles produced six invariant findings,
all incidental — this axis makes the zero a measured zero, which means it
reports the same shape whether or not it finds anything.

One row per invariant. A pass is the command's output **plus the sentence
that says what was read**, because every one of these greps returns hits
that are fine: the question is always whether each hit is on the allowed
list, never whether the count is zero.

| invariant | where to look | a pass reads like |
|---|---|---|
| No outbound beyond the configured origins | `grep -rn 'http\.\(Get\|Post\|NewRequest\|Client\)\|https://' --include='*.go' .` and the fetch/XHR sites in `web/src`, `mobile/src`, `site/src` | every host reached at runtime is one `SECURITY.md` lists (origin site, Linear, paired serve, user-run `gh`, loopback); each hit is classed origin / loopback / doc-string / test fixture, and the classification names the caller |
| The cache is disposable | every writer of `gadak.db` — `grep -rn 'INSERT\|UPDATE\|DELETE' --include='*.go' internal/store internal/sync internal/server` | no route or verb writes a row the origin cannot regenerate, and the exceptions (`local.db`, saved views) are the two the invariant names and are exportable |
| Writes pass the origin | the API surface — `internal/server` handlers that mutate, and `internal/origin/*writer.go` | every mutating handler reaches the origin before the cache; a handler that writes the cache directly is a finding even behind a flag |
| No secret reaches a log, an error or a report | `bash scripts/scan-internal.sh` plus the token/pairing values in `internal/config`, and one read of every `fmt.Errorf` that interpolates a config struct | the scanner is green, and no error path formats a struct whose fields include a token — the scanner is line-oriented and a one-line JSON dump defeats it (measured, `incident-line-oriented-leak-gates`) |
| A dev build does not decide for the release | `internal/store` open policy and its callers | plain `store.Open` callers inherit the process default; only commands opening a temp file they created opt out (GDK-1687) |

The report is that table with a verdict column and a `path:line` for each
class of hit. "No findings" without the table is not a pass — it is the
by-product this axis exists to replace.

### 14. Schema migration safety (GDK-1742)

Every `schemaVNN` added since the last tag is a change to a file the user
already has, made by a program they did not run on purpose — the first open
after an upgrade. Three incidents shaped this axis, none caught by the
axes above: a dev build migrated the author's real mirrors forward and
locked the installed release out (GDK-1687); a fixture regeneration
dropped derived tables because a head-schema file skips the backfill that
the migration path runs (`incident-fixture-regen-derived-tables`); and
schemaV40 turned CI red on e2e alone, because `e2e/serve.sh` refuses a
fixture whose `user_version` does not match the build (2026-08-31).

Start from the list: every migration between the last tag and head, one
row each, from `git diff <tag>..HEAD -- internal/store/schema.go` (the
`migrations` slice) — and the rows below are asked of **each** of them, not
of the set.

| question | where to look | a pass reads like |
|---|---|---|
| Upgrade from the last release's stamp | `internal/store/forward_migration_test.go` `mirrorAt` idiom — build a file at the previous tag's `user_version`, open it at head | the file reaches head's stamp, `SchemaAudit` reports nothing `Missing`, and the row count of every table the migration touches is the same before and after (a migration that drops rows is a finding even when the DDL is right) |
| Backfill, not just DDL | the migration body — does a new column or table get its existing rows in the **same transaction** (`backfillPageExcerpts`, `backfillItemRefs` are the pattern), or does it lean on "the next full sync"? | either the backfill is in the migration, or the comment names the sync path that fills it **and** the fixture was regenerated through `make demo-fixture` (the head-schema path does not run backfills) |
| The older binary against the newer file | `SchemaTooNewError` and its message; `internal/store/schema.go` user_version gate | an installed release opening a head-written mirror refuses with the message that names both stamps and the way out, and leaves the file untouched (byte-identical `user_version`) — never a partial open, never a silent downgrade |
| The dev-build policy still holds | `RefuseForward` at the open boundary, and the callers of `store.Open` added this cycle | every new `Open` caller inherits the process default; only a command opening a temp file it created opts out — GDK-1687's tests pin it, and this row confirms nobody added a bypass |
| Crash mid-migration | the migration runner — one transaction per migration, `user_version` written inside it | a kill between two migrations leaves the file at the previous stamp with nothing half-applied; a migration that does two file commits (the v29 shape, `schema.go` around line 483) is named in the report |
| Time on a real mirror | a copy of the largest local mirror (the `work` workspace, ~3,300 issues) opened at head, timed | the first open finishes in seconds, and the number is in the report — a migration that rewrites `items_fts` or every changelog row is the kind that turns a first launch into a hang |
| The fixtures moved with it | `PRAGMA user_version` of `examples/demo.db` and any other committed `.db` (`git ls-files '*.db'`) | every committed fixture is at head's stamp, regenerated through its `make` target, and the e2e that reads it is green — a fixture at head-1 is exactly the CI-only red of 2026-08-31 |

The report is the per-migration table. A cycle with no new migration
reports that in one line, with the tag-to-head diff that shows it — the
same measured zero as axis 13.

## Procedure

1. **Census** — the read-only rounds of Step 0, in parallel, one worktree
   each; reports to the session scratch, never the tree.
2. **Audit rounds** — one delegated, read-only round per axis, in parallel.
   Each round's report has two lists: **axis findings** (ranked, `file:line`,
   census number, why, cheapest fix) and **defects** (behaviour that is
   wrong today). Every round's spec carries the previous cycle's rejection
   list (below) so a rejected finding is not re-raised without new evidence.
3. **Lead triage** — merge and dedupe. **Defects open as normal Highest
   bugs immediately, never as audit sub-issues** (21% of past findings were
   defects that waited on audit priority). Findings the lead rejects get a
   `audit-rejected` label and a one-line reason in the issue, and closed —
   that ledger is what the next cycle's specs quote. Survivors go to Jira
   as sub-issues of one parent per audit (`품질개선 vX.Y`, label `quality`),
   priorities by cost/effect.

   The ledger has an address, so the next cycle quotes it instead of
   inventing the query:

   ```bash
   gadak --workspace gdk sql "select key, substr(summary,1,80) s, resolution
     from issues_full where labels like '%audit-rejected%' order by key"
   ```

   Paste that output into each axis round's spec under "already rejected —
   do not re-raise without new evidence".
4. **Fix rounds** — normal delegation rules (project CLAUDE.md binds:
   file whitelists, gate discipline, Playwright mandatory when `web/`,
   `e2e/`, or i18n catalogs are touched, no git writes by delegates).
   Refactors land as separate commits from behaviour changes. Large,
   disjoint, file-whitelisted chunks, one worktree each, merged the moment
   each finishes.
5. **Close** — full gate set on a quiet machine, CI green, sub-issues
   closed, parent closed with the census rerun: lines removed, the
   complexity and coupling numbers against the baseline, the CI ledger
   before/after. Update this runbook with anything the cycle taught, and
   write the cycle's row into [`../project/AUDIT_LOG.md`](../project/AUDIT_LOG.md)
   — see *Done* below.

## Done — what makes the audit finished

An audit is finished when a reader who was not here can answer "was this
version audited?" from the tree alone. Three things have to be true, and
the third is the one past cycles skipped:

1. **Every axis has a verdict** — reported, or deliberately not run with the
   reason written down. Fourteen rows, no blanks. An axis nobody ran is a
   legitimate outcome; an axis nobody can tell apart from one that ran clean
   is not.
2. **Every finding has an address** — opened as a defect (Highest), opened
   as a sub-issue of the audit parent, or closed with `audit-rejected` and a
   reason. Nothing lives only in a report.
3. **The cycle is recorded in the tree.** `docs/project/AUDIT_LOG.md` gets
   one section per cycle: the base SHA, the axis table with verdicts, the
   baseline numbers against the previous cycle's, and the defects that came
   out. The scratch reports are not the record — they are deleted with the
   session.

**Release readiness is a separate question, and this runbook answers only
half of it.** The audit says the code was looked at; it does not say the
product is shippable. Before the tag the lead states both, in one place:
the audit log's verdict table, and the open Highest defects with a decision
per row (fixed, or shipped as a named known issue in the release notes).
"Audit complete" over an open Highest defect is a statement about the audit,
never about the release.

What past cycles taught (keep this list short; delete a line once the
procedure above absorbs it):

- **A census must exclude `.claude/worktrees/`.** Agent worktrees are whole
  repo copies under the tree; a count that includes them triples every
  duplicate and invents "orphan" files (v0.21 second pass, axis 5's first
  numbers).
- **A read-only round that starts `gadak serve` in the foreground hangs
  forever**, and the harness reports "idle" — kill the serve child, not the
  round; the round resumes and reports (v0.21, axis 6, 19 minutes lost).
- Audit rounds read the tree while fix rounds may be writing to it; the
  `git status` paragraph in a report describes other rounds' work, not the
  audit's — read the findings, ignore that paragraph.
- Findings two axes deliver in duplicate are a good sign, not noise: merge
  them in triage and register once.
- **Review and merge a round the moment it finishes**, through
  `git -C <main-tree> merge --ff-only` — a merge run from the worktree's own
  cwd merges the branch into itself and pushes nothing (twice in v0.21).
- **A cancelled CI run is not a successful run.** The v0.21.0-tagged run
  itself was cancelled by `cancel-in-progress`; its E2E (3) figure is the
  time at which it died, not a duration (v0.22 census).
- **The outsource git guard blocks `git worktree`, `git tag -l` and
  `git ls-tree` in delegated rounds**; specs that need a historical tree
  say `git archive <ref> | tar -x -C /tmp` or `git show <ref>:<path>`, and
  say it before the round discovers it (v0.22, three rounds each lost a
  step to it).
- **A gate that says "`_test.go` excluded" excludes the gate packages too.**
  `internal/archlint`'s five consumers are all test imports; a fan-in table
  built from `Imports` alone calls it dead (v0.22 census).

## Version pins at tag time

The version owner is `git describe --tags --abbrev=0`. After a tag,
`tools/doc-checks.sh` check 6 (README status lines, `Last tagged:`) and
check 29 (Scoop / AUR) compare these files to it. Scoop and AUR also
carry hashes from that tag's `checksums.txt`, which does not exist until
the GitHub release is published — bump them with
`contrib/scoop/update.sh <tag>` and
`contrib/aur/gadak-bin/update.sh <tag>`, not by editing the version
alone.

**Order, measured on v0.20.0, v0.20.2 and v0.21.0.** The commit the tag
points at carries no pins. Check 6 fails while README says the new minor
and the latest tag is still the old one, check 29 fails the other way
round once the tag exists and the manifests still say the old patch, and
the machine-local pre-push hook runs doc-checks whenever an outgoing
commit adds a GDK key — so a pins commit cannot leave before the release
and a manifests commit cannot leave before checksums.txt. The sequence
that stays green at every step: (1) land the last content commit, tag it,
push branch then tag; (2) wait for Release and Desktop release; (3) run
the two update scripts, then `contrib/aur/gadak-bin/verify.sh` — it needs
a docker daemon (`orb start` on this machine) and is what regenerates
`.SRCINFO`, since makepkg is not on a mac; (4) bump README (en+ko) and
`Last tagged:`, run `tools/doc-checks.sh` and `contrib/scoop/verify.sh`,
and commit all six files as `release: <tag> manifests and state`. A push
during (4) cancels the previous commit's CI run (concurrency group), so
the tagged commit's verdict is the manifests commit's run.

In-repo pins to bump:

- `README.md` — status line, **minor** only (`v0.16.1` → `0.16`)
- `README.ko.md` — same minor
- `docs/project/STATE_OF_PLAY.md` — `Last tagged:` is the tag itself (`v0.16.1`)
- `contrib/scoop/gadak.json` — top-level `"version"`, full patch (`0.16.1`)
- `contrib/aur/gadak-bin/PKGBUILD` — `pkgver=`, full patch (`0.16.1`)

Not in-repo pins (the tag is the source; do not hand-edit):

- Homebrew `gadak-cli` — `.goreleaser.yaml` `brews` pushes
  `midagedev/homebrew-tap`
- Homebrew `gadak` cask — `.github/workflows/desktop-release.yml`
  renders from the tag
- `desktop/build-app.sh` — `CFBundleShortVersionString` from
  `git describe --tags --always`
- `contrib/omarchy/gadak/manifest.json` `"version"` — widget schema
  (`0.1.0`), not the product tag
- `contrib/raycast/` — no product-version field

## What this runbook is not

Not a substitute for continuous quality (bugs ship immediately; reviews
happen per round), and not a feature-planning venue — anything that grows
the product goes through the normal backlog instead.
