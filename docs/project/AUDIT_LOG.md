# Release audit log

One section per minor-version audit cycle. The procedure is
[`../runbooks/release-audit.md`](../runbooks/release-audit.md); this file is
the record it produces, so a reader can answer "was this version audited,
and what came out of it?" without the session that ran it.

A cycle's section carries four things: the base SHA the census measured, the
fourteen axes with a verdict each (an axis nobody ran says so and why), the
baseline numbers against the previous cycle, and where the findings went.

## v0.24 cycle — base `a3beaa39`, census 2026-09-18

The v0.23 pass closed at `4c0d5e35` and **24 commits** landed after it,
carrying three Unreleased themes — the phone terminal, the phone's list
header, and Korean and Japanese reading whole. This cycle is the runbook's
full fourteen-axis run over that tree. Parent issue: GDK-2001.

The rounds ran on **agy (gemini-3.8-flash-high)** by the user's choice, until
the plan's individual quota returned 429 with a four-and-a-half-hour reset;
axes 4, 7, 11 and 12 finished on GLM-5.3 instead, and the substitution is
recorded here rather than left to be inferred from the logs. Axes 8 and 10
stayed with the lead, as every cycle, because the judgment in them is
narrative.

One backend trap came out of it and is now written down in the outsource
skill (`references/agy.md`, commit `0634a33` there): an agy round can start
its own background task and hand the turn back saying it will continue when
that finishes. There is no later turn — the launcher takes the turn as the
report. Only the absent `--done-marker` caught it (exit 72), which is the
whole reason that flag exists.

| # | axis | verdict |
|---|---|---|
| 1 | Go philosophy | **ran** — `warmAttachments` spawned five goroutines outside `jobsWG` and rooted at `context.Background()`, so `Shutdown` returned with a warm fetch in flight and the next mirror query ran against a closed handle (GDK-2000, **defect**, landed `48200d5c`). Findings: seven files over 1,500 lines and 30 commits (GDK-2010), and four small Go items — a pipe leak on an unreachable path, an unsynchronised package global, an unbounded `changelog` scan in `SprintBurnup`, a parameter named `id` that carries a cache key (GDK-2011) |
| 2 | Svelte philosophy | **ran — both defects rejected, on the record.** The `lastDetail` memo cell the round called an impure template mutation is a plain `let`, not `$state`, so Svelte's purity rule does not reach it and the write is idempotent (GDK-2022). The deep-link teardown race is real in shape but has no path: that `$effect` reads no rune state — `deepLinks` is a `const`, and everything before `bindOsDeepLinks`'s first `await` is `runtimeMode()` — so it never re-runs, and App is the root (GDK-2023, demoted to a note for whoever next edits the file) |
| 3 | App shells | **ran** — `transport.ts`'s comment still said the Rust pairing boundary "is filed, not built (GDK-897)" while `src-tauri/src/shell.rs` has carried it for a cycle; a security comment that says a boundary is open is the expensive direction to be wrong in (GDK-2007). Also: `desktop/pairing.go` classifies `pairflow` errors by substring, so rewording one sentence silently turns a 400 into a 500 (GDK-2008); and the phone's TypeScript dial scope admits loopback on packaged iOS where the capability file and `shell.rs` both refuse it, so the refusal reaches the user as "network error" (GDK-2009) |
| 4 | Simplify | **ran — measured zero, and that is the result.** Deletable lines 0 across every class: unread CLI flags 0 of 132, unread env names 0 of 29, config keys with no reader 0, declaration-only exports 0, compatibility paths whose other side is gone 0 of 5, byte-identical tracked duplicates 0. No growth backlog either — net +900 Go over the window, and the top four packages are all defect or feature work. What is left is surface, not lines: 228 Go and 27 TS identifiers exported past their only reader, 62 of the Go ones exported for tests alone (GDK-2018) |
| 5 | Test pyramid | **ran** — the web folds an unknown `status_category` to `inprogress` where Go's `internal/statuscat` — which the web file's own comment calls the single owner — folds it to `new` (GDK-2004). Findings: wall-clock `page.waitForTimeout` in CI specs, two specs whose header comment is a helper's JSDoc rather than the user behaviour they protect, a Playwright spec that parses markdown, redundant assertions (GDK-2019) |
| 6 | UX consistency | **ran — one defect rejected.** Keyboard cannot reach bulk selection: `IssueRow`'s `keydown` is a partial copy of `onRowClick` and skips its `bulk.count > 0` branch, so Enter on a focused row opens the detail while bulk selection is active (GDK-2005). Revoking a paired device is one unguarded click while the settings tab beside it arms a two-step for turning a source off (GDK-2006). Rejected: Escape not closing the retro, history and docs overlays — the round grepped the components, and the bindings are in `web/src/lib/commands.ts`'s keymap registry, which is the correct single owner (GDK-2021) |
| 7 | Agent surface | **ran** — SKILL.md taught that there is no stored current workspace, false since GDK-490 and contradicted twenty lines later by its own list of `workspace_source` values and by the MCP surface (GDK-2013, **defect**, landed `c105c223`). Four more gaps: `--layout` on `views save|open`, `gadak issue --link`, `gadak_search`'s description omitting labels (where SKILL contradicts itself at `:283` and `:475`), and `sync --source` (GDK-2017) |
| 8 | Changelog | **measured by the lead** — three themes, `doc-checks` #46 green, and theme 2's bold head does not describe its body: six keys under "the phone's list header says where to tap", of which four are the list's order, grouping, narrowing and a close-time crash. The subject those six share is that the phone's list is the desktop's list. GDK-1997 also arrives as "And … no longer breaks the screen", the narration shape the rule names (GDK-2020, one editing pass over three editions at tag time) |
| 9 | CI cost | **ran** — race shard 1 alone takes 543 s and decides the whole run's wall clock, against 307 and 482 for its siblings: 47 tests added since 2026-09-15 sit on median fallback and `WORKSPACE_PRELOAD=62.806` is a local number where CI takes ~133 s. 90 s off the deciding shard, 0 billable (GDK-2002). Three cheaper items: `cancel-in-progress` is unconditional on `main`, `Staticcheck` never moved to `tools/ci-filter.sh` and still uses an inline `^desktop/` regex, and `staticcheck` is compiled from source on every Go-touching run (GDK-2003). Flakiness census over 60 runs: 57 success, 2 failure (both legitimate gates), 1 cancellation — against v0.22's 0 failures and 13 cancellations |
| 10 | Leverage residue | **ran, by the lead** — GDK-1991 completes theme 2 and is the closest thing to a two-hour item (the groundwork landed this cycle in GDK-1993 and GDK-1996); GDK-1949 is under an hour; GDK-1995 fills the web-in-a-phone-browser half of theme 1 and costs about half a day, because sharing the phone's `keys.ts` means a root dependency on `glasskeys` and the lockfile-platform gate. Theme 3 has no product residue — its three open neighbours are demo-fixture quality and belong to the media round. Ranked list recorded on GDK-2001; the decision is the user's |
| 11 | i18n completeness | **ran** — `tools/ja-spacing.py` names eight files and none of them is a message catalogue, so the Japanese of every desk and phone screen was outside the gate written for exactly this: 774 ASCII spaces beside Japanese characters, the same class a reader reported on the site in GDK-1854 (GDK-2012, **defect**). One of them entered *this cycle*, after that incident. Measured zeros the other way: prose literals outside `t()` 0 (v0.22 had 27 on the phone and 1 on the web), and every date and duration formatter takes the active locale — the `'ko-KR'` hardcode is fixed. Tone half by the lead |
| 12 | Fact ledger | **ran** — copy-versus-ledger disagreements **0**, against v0.22's 10. But the census tool undercounts: `copies_of` matches URL literals against raw line text, so the percent-encoded Datasette Lite links made the repo's most-forward fact link read as 0 copies where there are 5 (GDK-2014 — a defect in our own instrument). Nine unguarded copies, the demo issue count and the "3.7 minutes" first-sync figure worst among them (GDK-2016); the ledger's own verified-at stamp is two minors stale (GDK-2015) |
| 13 | Invariants | **ran — measured zero.** Every outbound call site classified against `SECURITY.md`'s list with its caller named; writes all pass through origin; no display-name keying in Go writes or web logic. The two facts this cycle was asked to judge rather than assume — the sixth `items_fts` column and the new `type_id`/`direction` link path — both checked out |
| 14 | Schema | **ran** — the sixth `items_fts` column landed without a `schemaVNN` bump, and `schemaV25` and `schemaV50` are the same change with a `SELECT 1` entry each and a comment saying why. With two shapes both stamped 53 the version gate cannot tell them apart, so a v0.23.1 binary rebuilds the index back to five columns and GDK-1978's fix is silently undone (GDK-1999, **defect**, landed `9debcfd0`). `data-model.md` had already written `v50` for it, because there was no number of its own to write |

**Defects: four, all opened Highest and shipped inside the cycle** —
GDK-2000 (`48200d5c`), GDK-1999 (`9debcfd0`), GDK-2013 (`c105c223`) and
GDK-2012. Three cycles running had none; the difference is not that the tree
got worse but that axes 1, 7, 11 and 14 were pointed at lifecycle, at the
agent surface, at a gate's target list and at a stamp, rather than at shape.

**Findings**: GDK-2002 through GDK-2020, sub-issues of GDK-2001.

**`audit-rejected` ledger: no longer empty.** Three entries, each with the
evidence that killed it: GDK-2021 (Escape is bound, in the keymap registry),
GDK-2022 (`lastDetail` is not `$state`), GDK-2023 (the effect never re-runs).
Two of the three were delegate "defects"; one was found false by reading the
file the round had only grepped.

### Measures

| measure | v0.23 close (`4c0d5e35`) | v0.24 base (`a3beaa39`) |
|---|---|---|
| Go lines, non-test · test | 109,737 · 150,242 | 113,158 · 155,015 |
| Svelte · TS lines | 37,838 · 130,791 | 39,324 · 139,338 |
| mean non-test cyclomatic | 6.35 over 2,868 functions | 6.36 over 2,939 |
| functions at cyclomatic ≥ 60 · ≥ 69 | 4 · 3 | 4 · 3 |
| CI wall / billable | 438 / 3,358 (v0.22 base) | 543 / 3,725 |

Complexity did not move: +0.01 on the mean over 71 more functions, and the
tail is identical. That sentence needed a correction to write. The census
carried `5.35 over 3,011` forward as "the previous cycle" — it is this file's
own line 290, from three cycles back — and axis 1 built a whole narrative of
tail contraction against mean degradation on it, faithfully, from a wrong
input. The right neighbour was four lines above it. Two instrument failures
in one cycle (this one and GDK-2014) is the argument for the runbook's rule
that a census number is checked once against an independent path before a
finding stands on it.

### Install verification — the pre-tag half, run on `v0.23.1`'s artifacts

`install-verification.md` says this runs twice and the first moment is before
the tag, on the *previous* release's artifacts, to prove the install path is
unbroken. macOS host, 2026-09-18, over a shell rather than Finder — so the
install was `cp -R` plus `xattr -dr com.apple.quarantine`, which is what
Finder's drag would have done; `pgrep` showed no `AppTranslocation` path
afterwards.

The shipped `Gadak-0.23.1-arm64.dmg`, quarantined the way a browser download
is, is `accepted / source=Notarized Developer ID`, and so is the bundle inside
it: hardened runtime (`flags=0x10000(runtime)`), timestamped 2026-09-17,
`TeamIdentifier=XEF9KH7N43`. `checksums.txt` covers only the six goreleaser CLI
archives — the dmg, the two desktop zips and the `.mcpb` are not in it. That is
not a gap for the dmg, whose notarized signature is the stronger claim, but it
is worth knowing before anyone tells a user to verify a download by hash.

Three bundles claim the `gadak:` scheme on this machine and exactly one can
answer it: `/Applications/Gadak.app`. The other two are the phone app's iOS
device builds — an `.xcarchive` and an Xcode `DerivedData` copy, both
`requires-iphone-os`, both developer-machine artifacts a user never has. Cold
(no process, `open gadak://view?issue=GDK-2001` → exactly one process under
`/Applications`) and warm (a second link, same pid, count still 1) both pass.
The **destination** — whether the window went to the issue in the link — stays
unverified, the headless limit the runbook names.

Two findings the lead did not go looking for:

- **The CLI on `PATH` is two releases and one build class behind the bundle.**
  The bundle ships 0.23.1; `~/.local/bin/gadak` is `0.22.0-dev+bb40aac8` and
  shadows the 0.23.1 at `/opt/homebrew/bin/gadak`. This is the runbook's own
  "a `PATH` copy older than the bundle is a real finding", and it is the reason
  sessions in this repo call gadak by absolute path — a friction rule that
  turns out to have a cause rather than a superstition.
- **GDK-1999's scenario, observed live rather than argued.** One launch of the
  0.23.1 app migrated ten of twelve workspace mirrors 51 → 53, and what it
  wrote was a **five-column** `items_fts` stamped 53 — the shape GDK-1999 says
  no version gate could tell from the six-column one. On a WAL-consistent copy
  a HEAD build printed `rebuilt items_fts (2088 rows): DDL did not match this
  build's schema` and landed at 54 with `script_runs` present. The dev-build
  guard (GDK-1687, GDK-1967) refused that migration until `GADAK_DEV_MIGRATE=1`
  was set, which is the second thing this measured: the block works.

### Release readiness at the end of this cycle

**Asserted, for the tag.** The two halves the runbook asks for:

The **axis table above** — fourteen axes, fourteen verdicts, twelve delegated
read-only and axes 8 and 10 run by the lead. Four defects opened and all four
shipped; nineteen findings under GDK-2001; three delegate claims rejected with
evidence and filed in the `audit-rejected` ledger, which is non-empty for the
first time.

The **open Highest defects, with a decision per row**: there are none. GDK's
open-Highest count is zero (`WKS-34` belongs to another project). GDK-1192,
open at the v0.23 readiness block, closed in this cycle — `Desktop tests=success`
on all six main runs since the structural fix, with the evidence boundary
stated on the issue: six green runs corroborate, they do not prove, and no
theory of why darwin's ptmx reverts the size has been written.

What ships as known issues: one High — GDK-1095 (IME composition interrupted by
a shortcut sends twice; upstream xterm, needs a real-device repro) — and twelve
Medium and three Low, none of which blocks a user's first hour. The leverage
residue (GDK-1991, GDK-1949, GDK-1995) is ranked on GDK-2001 and is the user's
call, not a readiness condition.

## v0.23 cycle — base `4ff2195f`, census 2026-09-15, closed at `4c0d5e35`

The third v0.22 pass closed at `b6352471` and **87 commits** landed after it,
carrying three Unreleased themes — the phone rebuild, the Confluence cache
cleanup, and the agent page that renders on the issue. This cycle is the
runbook's full fourteen-axis run over that tree, the first since the census
tooling moved to `tools/audit/` (GDK-1707). Parent issue: GDK-1904.

Every axis got a verdict. Twelve ran as read-only delegated rounds off the
census pages; two (8 and 10) and the tone half of 11 stayed with the lead,
because the judgment in them is narrative and does not delegate.

| # | axis | verdict |
|---|---|---|
| 1 | Go philosophy | **ran** — `runConfluencePass` was the tree's worst function at cyclomatic 97 / cognitive 243, +36 this cycle (GDK-1920, landed); two duplications this cycle introduced (GDK-1921, landed); four structural trends that are each small and point the same way — server fan-out 24, settings helpers bypassed, eight files over 1,500 lines and 30 commits (GDK-1922, **open for the user**). Measured zeros: panics in product code 0, single-implementation interfaces 0 |
| 2 | Svelte philosophy | **ran** — the phone's `Detail.svelte` at 2,319 lines / 52 `$state`, +919 lines this cycle with no seam (GDK-1925: a size ratchet landed first, the extraction is landed the same day — 1,793 lines / 38 `$state`, the ratchet lowered in the same commit, six captures byte-identical); two blind spots in the rune gates — App.svelte's 500 ms poll and the effect-mirror leaking into the store (GDK-1926, landed); the relative-time ladder existed twice, web and phone, and had already diverged past seven days (GDK-1927, landed) |
| 3 | App shells | **ran** — the wails pin was five betas behind and the beta.18 Calloc-leak fix was not in the shipped binary (GDK-1916, landed via PR #106); the secure-storage plugin's 1.x pin against a 2.x toolchain re-confirmed as the intended disposition (GDK-1917) |
| 4 | Simplify | **ran** — four test-fixture families (~260 lines, GDK-1918) and ten product duplications (~230 lines, GDK-1919), both landed; dead code 0 |
| 5 | Test pyramid | **ran** — two clock waits in the browser tier, 6 s in mirror-instant and 10.5 s in terminal-fold, replaced by conditions (GDK-1924, landed) |
| 6 | UX consistency | **ran** — all three vocabulary gates read only `web/`, so the phone was structurally invisible to them (GDK-1928, landed) |
| 7 | Agent surface | **ran** — artifacts, the cycle's third theme, were on none of the three agent surfaces (SKILL, MCP, CLI) (GDK-1905, landed) |
| 8 | Changelog | **measured by the lead, compression deferred to the tag** — Unreleased is 3 themes / 5 paragraphs / 1,140 words / 20 keys; the phone theme alone is 730 words over 12 keys, 1.5× the whole of 0.22.0 after its cut, and paragraph 3 (353 words, 7 keys) has the one-key-per-sentence shape back. The lead compresses at tag time by the one test that survived last cycle: does someone opening gadak notice this |
| 9 | CI cost | **ran** — 27.6 runner-hours per cycle went to docs-only pushes re-running every tier (GDK-1912, a per-subject input filter, landed via PR #107); race shards were split by count so the wall clock was luck, 387/344/421 s (GDK-1913, measured deal); the e2e weight table was a cycle stale (GDK-1914, one command regenerates it from a run id); the partition check ran three times and Playwright downloaded four (GDK-1915). First live desktop-only push after the filter still ran every tier — the table's `suffix:.go` and `dir:examples/` rows reach files those tiers never build (GDK-1931, opened) |
| 10 | Leverage residue | **ran, by the lead** — surface matrix (20 Unreleased keys × 5 surfaces) read per theme against the open backlog; ranked list recorded on GDK-1904. Two of the four landed inside this cycle as findings (GDK-1905, GDK-1910); GDK-1827 (a phone sprint screen) and GDK-1901 (phone artifact rendering needs its own origin) are the user's call |
| 11 | i18n completeness | **ran** — 27 phone-only keys in the shared namespace that the dead-key gate could not see, the 2026-08-26 incident class (GDK-1923, landed: the gate now reads `mobile/src` too, with a reasoned allowlist). Tone half by the lead: 11 new ko/ja pairs read, one fixed in place (`detail.artifactExpand` ja used 広げる where the catalog says 開く) |
| 12 | Fact ledger | **ran** — seven ledger `path:line` citations pointed at unrelated code (GDK-1907), the ledger said 11 promises where the file has 8 and its minor version had stopped at 0.21 (GDK-1908), and the site's benchmark figures and Datasette link were outside every check (GDK-1909) — all landed, with a new doc-check (60) that resolves every cited line. That check found 32 more in SUPPORT_MATRIX the day it landed (GDK-1929, landed: 114 citations rewritten to claim-carrying lines) |
| 13 | Invariants | **ran** — `api_usage`, our own outbound counters, lived only in the disposable mirror: not regenerable from the origin, not exportable (GDK-1906, landed: copied to `local.db` by `schemaV53`, with two gates for the class — a CREATE-side scope test and a SQL-name qualification test) |
| 14 | Schema | **ran** — the healing `schemaV52`'s comment promised did not fire on the built-in wiki (GDK-1910, landed); the snapshot pipeline dropped the `parent_id` column the same cycle had added (GDK-1911, landed) |

**Defects: none opened.** Every finding was a quality gap, not behaviour
wrong today; the cycle's Highest bugs (GDK-1844, GDK-1095, GDK-1192) predate
it and are listed under readiness below.

**Findings**: GDK-1905 through GDK-1929, sub-issues of GDK-1904 — 25 opened,
24 landed in this cycle. Open at close: GDK-1922 (structural
trends, a user decision). Two follow-ups came out of the fix
rounds: GDK-1930 (drop the mirror-side `api_usage` table once the copy has
settled) and GDK-1931 (the CI filter's over-broad rows).

`audit-rejected` ledger: still empty.

### Measures

| measure | base (`4ff2195f`) | close (`4c0d5e35`) | command |
|---|---|---|---|
| Go lines, non-test · test | 109,495 · 149,980 | 109,737 · 150,242 | `tools/audit/complexity.sh` |
| Svelte · TS lines | 37,487 · 130,028 | 37,838 (+351: the extracted sheets each carry the scoped CSS they use) · 130,791 | same |
| mean non-test cyclomatic | 6.37 over 2,814 functions | 6.35 over 2,868 | same (`gocyclo -over 0`, non-test rows) |
| functions at cyclomatic ≥ 60 · ≥ 69 | 5 · 4 (worst `runConfluencePass` 97) | 4 · 3 (worst `cmdInit` 74) | same |
| `internal/server` fan-out · import cycles | 24 · 0 | 24 · 0 | same |
| CI wall / billable seconds (main run) | 580 / 3,763 | 481 / 4,059 | `tools/audit/ci-ledger.sh` — wall fell with the measured race deal (GDK-1913); billable rose because this close run is a desktop-only push that the filter did not skip (GDK-1931) |
| catalog keys · byte-equal without allowlist | 1,467 · 0 | 1,468 · 0 | `tools/audit/i18n-census.sh` |
| fact values · unguarded with copies | 30 · 19 | 30 · 16 | `tools/audit/fact-ledger.sh` |
| CHANGELOG Unreleased (en): paragraphs · words · keys | 5 · 1,140 · 20 | 5 · 1,142 · 20 | the axis-8 measurement; compression is a tag-time step |
| phone `Detail.svelte` lines · `$state` | 2,319 · 52 | 1,793 · 38 | `mobile/src/lib/screen-size.test.ts` ceilings |

### What this cycle taught

- **A citation check can pass on the wrong line.** Check 60 resolves every
  `path:line` and fails on a missing file, a line past EOF, a blank line or a
  comment. It cannot tell that `attachment.go:379` is real code about
  something else — and 32 of SUPPORT_MATRIX's anchors were exactly that the
  day the check landed. The structural half of "is this citation true" is a
  gate; the other half is review, and the log should say which half a gate
  covers.
- **Classification right, home wrong is its own class.** The scope table in
  `origin_scope.go` had `api_usage` under `scopeLocal` for a year while the
  table sat in the mirror. A table that names the rule is not a gate; the
  gate is the test that reads the table and the schema together. Two gates
  now do (CREATE side and SQL side), and the copy-migration pattern from
  GDK-105 is a list rather than one special case.
- **A filter's table is only as narrow as its evidence.** The CI filter
  landed with `suffix:.go` for every tier because "Go reaches the whole
  tree" — true of the root module, false of `desktop/`'s own. The first
  push that could have skipped did not. Every row in an input table should
  cite the build step that reads it, and a row that cites none is a guess.
- **Delegates do not commit, and a lead who forgets that reads empty
  diffs.** Three rounds in a row the branch diff was empty because the work
  was sitting uncommitted in the worktree. The commit is the lead's, so the
  first thing to read after a round is `git status` in the worktree, not
  `git diff main..branch`.
- **Per-round effort is a real dial.** Rounds ran at `medium` for spec-tight
  implementation and `high` for multi-file contracts and gate authoring;
  none needed `max`. The quota that used to go to one `max` round covered
  the whole census fan-out.

### Release readiness at the end of this cycle

Not asserted. Three Highest defects are open and none has a decision
recorded — GDK-1844 (a glyph in the folding terminal cell disappears; a
regression test exists and passes, the disposition is the user's),
GDK-1095 (IME composition interrupted by a shortcut sends twice, upstream
xterm), GDK-1192 (a desktop resize test times out intermittently in CI).
The mobile launch, the top priority, is blocked on GDK-958 (a review demo
path reachable from outside the tailnet), which is a product decision, not
a finding.

## v0.22 cycle, third pass — base `4e57a273..b6352471`, 2026-09-12

The second pass closed at `4e57a273` and **50 commits · 261 files ·
+17,651 / −1,460** landed after it with no audit at all, carrying one schema
migration (`schemaV51`), three new MCP tools and a new origin write verb.
This pass is a delta audit of that range, not a fourteen-axis re-run — the
precedent is GDK-980. Parent issue: GDK-1801.

The user asked three questions and this pass exists to answer them: **did
these commits damage the narrative, hurt the UX, or lower code quality?**

Mid-pass the scope was widened on the user's instruction: the delta names the
neighbourhoods, and inside a neighbourhood the whole thing is read, including
code the delta never touched. The finding class promoted to the top of every
round's ranking — *code that should have been unified with what already
existed and was placed beside it instead* — is what that widening was for,
and it produced the four highest-value findings of the pass.

| # | axis | verdict |
|---|---|---|
| 1·2·4 | Go · Svelte · Simplify | **ran** — quality did not fall (mean cyclomatic 6.37 → 6.34 over 9,407 more lines, ≥60 functions 5 → 5, fan-out 24 → 24, 0 cycles, 0 new dependencies) but the delta is additive: 53 files added, 0 deleted, 448 non-test Go lines removed against 3,735 added. 12 findings, the top four all unification (GDK-1810, GDK-1815, GDK-1811, GDK-1812). 0 defects |
| 3 | App shells | **not run** — `git diff --stat 4e57a273..b6352471 -- desktop mobile/src-tauri` is empty |
| 5 | Test pyramid | **not run** — outside the three questions asked; the rounds' own gates covered what they touched |
| 6 | UX consistency | **ran** — 17 findings. The delta did not regress the UX; it raised the bar on one surface and left the one beside it, which is what makes the terminal dock's 4px keyboard-dead grip read as a different author (GDK-1815, GDK-1816). 1 defect (GDK-1804) |
| 7 | Agent surface | **ran** — 224 raw gap rows triaged to 10 findings and 3 defects by opening the code. The MCP enum class came back clean, measured by dumping what the server returns rather than reading the descriptions. All three defects are SKILL.md saying something false (GDK-1803); findings GDK-1813, GDK-1814 |
| 8 | Changelog | **ran, by the lead** — narrative judgment is not delegated. Unreleased was 54 paragraphs / 13,373 words / 348 citations of 322 distinct keys, against a largest-ever-shipped section of 834 words; rewritten to 3 themes + 4 continuations in three editions, **zero keys lost in any edition** and no number added (GDK-1809). One paragraph was shipped twice in all three editions (GDK-1802) |
| 9 | CI cost | **not re-run** — recorded only that HEAD is green (`b6352471`, CI and Hosted demo both success) |
| 10 | Leverage residue | **ran, right before the tag** — `tools/audit/surface-coverage.sh` at `9a811377`, 333 keys × 5 surfaces, aggregated per theme. Theme ① reaches web on 2 of 15 keys and the phone on 0; theme ② reaches the phone on **0 of 45**; theme ③ reaches every surface. Two of the four MISSING cells triaged as census artefacts (SKILL.md:151 and `internal/mcp/tools.go:168` both carry `jira-server` under a different key). Three candidates ranked by cost with the surface each fills: GDK-1826 (MCP, an hour or two — `store.SprintBurnup` already has two callers), GDK-1827 (phone, half a day — the server routes exist), GDK-1828 (web, a round — the connect endpoint requires an email a Server workspace does not have). **The include/defer decision is the user's; the list is the question put to them.** |
| 11 | i18n completeness | **re-measured** — 1,459 catalog keys (1,430 at second-pass close); byte-equal without an allowlist entry 0; stale allowlist entries 0; English literals outside `t()` 0; missing locale values 0 |
| 12 | Fact ledger | **re-measured** — 30 values over 73 copy surfaces; **18 unguarded values with copies** (17 at second-pass close). Reading list for the next cycle |
| 13 | Invariants | **ran** — swept whole-tree per the widening: 35 Go HTTP call sites, 61 URL literals, 28 frontend fetches, 15 server-side store writes and all 48 mutating routes classified, zero exceptions, **zero re-introduction of the removed update check**. Four of five invariants pass; the fifth is defect GDK-1806. Three guard-weakness findings (GDK-1818, GDK-1819, GDK-1820) |
| 14 | Schema | **ran** — `schemaV51` answered against all seven questions on a 624 MB / 7,177-issue copy of a real mirror, plus `localSchemaV10`/`V11` which the spec had missed. Defect GDK-1805. The one two-file-commit migration is `schemaV26`, not v29 as the runbook said |

**Six defects, all opened Highest and all fixed in this pass**: GDK-1802
(paragraph shipped twice), GDK-1803 (SKILL.md × 3), GDK-1804 (`--field`
silent drop), GDK-1805 (`blocked_hours = 0` as a false claim), GDK-1806
(refused dev open migrates `local.db`), GDK-1807 (clear leaves two tables).

**Findings**: GDK-1808 through GDK-1820, sub-issues of GDK-1801. Nine landed
in this pass; GDK-1817, GDK-1818, GDK-1819, GDK-1820 remain open.

`audit-rejected` ledger: still empty — nothing was rejected this pass.

### Measures

| measure | second-pass close (`4e57a273`) | this pass (`b6352471`) |
|---|---|---|
| mean non-test cyclomatic | 6.37 over 2,771 functions | 6.34 over 2,803 |
| functions at cyclomatic ≥ 60 · ≥ 69 | 5 · 3 | 5 · 3 |
| `internal/server` fan-out · import cycles | 24 · 0 | 24 · 0 |
| staticcheck cross-platform findings | 0 | 0 |
| catalog keys · byte-equal without allowlist | 1,430 · 0 | 1,459 · 0 |
| fact values with copies and no guard | 17 | 18 |
| CHANGELOG Unreleased (en): paragraphs · words · keys | 54 · 13,373 · 322 | **7 · 3,958 · 331** (0 lost, 9 added by this pass's own fixes) |

### What this cycle taught

- **A gate can be green and measuring the wrong axis, and that is worse than
  a red one.** Two axes found the same shape independently: `doc-checks` #46
  counted bullets while the event log came back as unbolded paragraphs, and
  `palette-coverage.test.ts` read its axis list off a registry that did not
  contain the one axis with no keyboard door. Both gates would have stayed
  green forever. When a guard is written at the same time as the thing it
  guards, ask what it cannot see.
- **A delta-only audit misses the highest-value findings.** The first three
  rounds were specced to the delta and the user corrected it mid-round. Code
  that should have been unified with what already existed is invisible to a
  delta reader (it looks like clean new code) and invisible to a whole-tree
  reader (nothing changed recently); only a reader holding both halves sees
  it. Four of the top findings are that class.
- **Narrative judgment does not delegate, and the map does.** The axis-7
  round built the mechanical map — keys per paragraph, sentences carrying
  three or more keys, stand-alone test per key — and the lead read the map
  and decided. Asking a delegate for the verdict produces sentences that are
  not there.
- **A sack grows.** The third theme's head was "Elsewhere: …" — the word for
  everything unclassified — and it held 288 of 322 keys. The lead added a
  sentence to it the same day, without noticing. A theme needs a test that
  can reject a key.
- **A threshold fitted to one draft is not a contract.** The word cap was
  first set at 12 words per key — 3,972 for this section — and the draft in
  hand measured 3,958, fourteen words under a cap derived from itself. That
  is a number chosen to pass, not a contract. It is 15 now, from the
  tightest shipped precedent (18.4), which leaves the section 944 words of
  headroom instead of 14.

## v0.22 cycle, second pass — base `4c076f72`, census 2026-09-11, closed at `4e57a273`

The seven axes the first pass left open ran this time, plus the two the
runbook added since (invariants as 13, schema as 14). The reading rounds were
delegated per axis, every finding was registered under GDK-1766 before any
fix round started, and the fix rounds landed in four commits the same night
(`8efececc`, `b1a0eda8`, `61475394`, `4e57a273`).

| # | axis | verdict |
|---|------|---------|
| 1 | Go philosophy | **ran** — `cmd/gadak/agent.go` (3,452 lines) split by topic, `cmdInit` 100→74, `collectDoctor` 60→12, the two sync passes and the retro pass extracted (GDK-1771, GDK-1772, GDK-1775, GDK-1773) |
| 2 | Svelte philosophy | **ran** — one terminal socket driver for the web pane and the phone shell, the shell-drop seam moved to transport, the DOM sweeps now walk `mobile/src` (GDK-1767, GDK-1768, GDK-1776) |
| 3 | App shells | **ran** — two delegate session dirs a relative `--config-dir` had committed under `desktop/` removed and the shape ignored (GDK-1770); the two upstream tickets the shells wait for were already GDK-640 and GDK-1473 (GDK-1784 closed as their duplicate) |
| 4 | Simplify | **ran** — `migrate` to Jira/Linear as a staged pipeline (cyclomatic 93/86 → 9/11), one `UsageBox` for the two Atlassian clients, generic Linear paging, string-setting helpers, three store clones folded, `wiki` help derived from `page` (GDK-1774, GDK-1780, GDK-1779, GDK-1777, GDK-1781, GDK-1778) |
| 5 | Test pyramid | **ran** — the slowest Go test 14 s → 2.5 s under `-race`, headers and shard weights for every e2e spec, one catalog assertion moved off Playwright, the census's slowest row now completes (GDK-1782, GDK-1783, GDK-1764) |
| 6 | UX consistency | **ran** — the phone's relative time reads the catalog, one copy toast, editor-kind labels, one Korean word each for watch/history/active sprint, ja counters without a space (GDK-1765, GDK-1788, GDK-1787) |
| 11 | i18n completeness | **re-measured** — byte-equal keys without an allowlist entry: 0; stale entries: 0 |
| 12 | Fact ledger | **re-measured** — 17 values with copies and no guard, listed on the census for the next reading |
| 13 | Invariants | **ran** — `gadak workspace export` now carries the visit and search history the invariant excuses from the mirror rule; a paired serve's 501 is read as an answer rather than retried (GDK-1769, GDK-1762) |
| 14 | Schema | **ran** — every live table documented and cross-checked by a test; a seeded v44→head migration gate that fails on lost rows (GDK-1785, GDK-1786) |
| 7–10 | agent surface · changelog · CI cost · leverage | not re-run — the first pass closed them and nothing in this pass touched their inputs except the changelog, which the fix rounds wrote in the release's three theme paragraphs |

### Baseline

Both columns come from the same census scripts, run on the pass's base and
on its closing commit. Function counts move because the pass added
production files (`dispatch.go`, `usagebox.go`) and split others.

| measure | second-pass base (`4c076f72`, 2026-09-11) | after this pass (`4e57a273`) |
|---|---|---|
| Go lines, root module (non-test + test) | 244,274 | 245,120 |
| mean non-test cyclomatic | 6.42 over 2,701 functions | 6.37 over 2,771 |
| functions at cyclomatic ≥ 60 · ≥ 69 | 9 · 8 (worst `cmdInit` 100) | 5 · 3 (worst `cmdInit` 74) |
| `internal/server` fan-out · import cycles | 24 · 0 | 24 · 0 |
| catalog keys · byte-equal without allowlist | 1,427 · 0 | 1,430 · 0 |
| fact values with copies and no guard | — (first measured this pass) | 17 |
| CI wall / billable seconds | 440 / 2,988 (v0.21.0) | HEAD run not yet judged when this was written |

### Where the findings went

Twenty-seven issues were opened under GDK-1766; twenty-five are closed by the
four landing commits and two lead fixes, GDK-1784 closed as a duplicate of
GDK-640 and GDK-1473. Two came out of the landing itself rather than the
reading: GDK-1789 (two parallel e2e suites on neighbouring ports collide,
because one spec spawns its own serve on `GADAK_E2E_PORT+1`) stays open at
High; the `tools/sourcelint.sh` owner tables that keyed on the old
`cmd/gadak/agent.go` path went red in CI once and were re-pointed the same
hour, with the gate named in `CLAUDE.md` so a plain `go test` is no longer
mistaken for it.

### Release readiness at the end of this pass

Not asserted. GDK-1095 (an IME composition interrupted by a shortcut sends
its text twice, upstream xterm) is still open at Highest with no decision
recorded; GDK-1192 closed since the first pass. GDK-1789 is a test-infra
defect, not a product one, and does not block a tag on its own.

## v0.22 cycle — base `245ede99`, census 2026-09-09

Six axes ran, seven did not. The cycle's purpose was the runbook rewrite
itself (GDK-1698): the census was built and the six axes that had no
procedure before were run end to end, which is what produced the numbers the
remaining seven now open from.

| # | axis | verdict |
|---|------|---------|
| 1 | Go philosophy | **not run** — the census produced its input (hotspots, cyclomatic, coupling); the reading round is open |
| 2 | Svelte philosophy | **not run** — same, `$effect` and long-function lists exist |
| 3 | App shells (wails, Tauri) | **not run** |
| 4 | Simplify | **not run** — duplication pairs and the growth table exist |
| 5 | Test pyramid | **partial** — the per-spec and slowest tables were built; the only verdicts taken were the five checks axis 9 moved off Playwright |
| 6 | UX consistency | **not run** |
| 7 | Agent-surface coverage | **ran** — 49 keys × 5 surfaces, 5 findings, all closed (GDK-1700) |
| 8 | Changelog | **ran** — Unreleased compressed to a third in three editions (GDK-1701) |
| 9 | CI cost | **ran** — ledger built, three cuts landed (GDK-1702) |
| 10 | Leverage residue | **ran** — ranked list per theme, recorded on the issue for the user's call (GDK-1703) |
| 11 | i18n completeness | **ran** — 24 leaked strings, phone date formatting, byte-equality gate (GDK-1704) |
| 12 | Fact ledger | **ran** — 122 copies mapped, 10 disagreements fixed at the copy (GDK-1705) |
| 13 | Invariants | **not run** — the axis got its report form this cycle; nothing has been measured against it yet |

### Baseline

First cycle to record one, so there is nothing to compare against yet.

Every row names the command that produced it, because a later cycle that
re-derives a row with a different filter reads the difference as the
ledger being wrong (2026-09-15: the v0.23 axis-1 round measured three
other refs with its own filter, got 2,803 functions where this table says
3,011, and reported the baseline as irreproducible — re-run at this
table's own base, the command below gives 3,133 / 5.28 / 7, which is this
row).

| measure | v0.22 base (`245ede99`) | command |
|---|---|---|
| Go lines (test ≈ 1.3× production) | 219,183 | `tools/audit/complexity.sh` |
| mean non-test cyclomatic over 3,011 functions | 5.35 | `gocyclo -over 0 .` from the repo root, rows whose file is not `*_test.go` |
| functions at cyclomatic ≥ 69 | 7 (worst: `cmd/gadak/init.go` 100) | the same run, `$1 >= 69` |
| `internal/server` fan-out · import cycles | 23 · 0 | `go list -f '{{join .Imports "\n"}}' ./internal/server \| grep -c midagedev` |
| CI wall / billable seconds | 438 / 3,358 (v0.19.0: 317 / 2,241) | `tools/audit/ci-ledger.sh` |
| catalog keys · real ja fall-throughs | 1,303 · 2 | `tools/audit/i18n-census.sh` |
| fact copies · disagreeing | 122 · 10 | `tools/audit/fact-ledger.sh` |

### Where the findings went

Closed this cycle: GDK-1700, GDK-1701, GDK-1702, GDK-1703, GDK-1704,
GDK-1705, and the two that came out of the census as defects — GDK-1687 (a
dev build migrated a release-written cache) and GDK-1706 (the retro table
clipped). Opened for later: GDK-1707 promotes the census scripts to
`tools/audit/`; GDK-972 carries the invariants axis.

Not audited, and therefore not claimed: everything under axes 1–6 and 13.

### Release readiness at the end of this cycle

Not asserted. Two Highest defects were open when the cycle's fix rounds
finished — GDK-1095 (an IME composition interrupted by a shortcut sends its
text twice, upstream xterm) and GDK-1192 (a desktop terminal resize test
times out intermittently in CI) — and neither has a decision recorded yet.
