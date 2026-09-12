# Release audit log

One section per minor-version audit cycle. The procedure is
[`../runbooks/release-audit.md`](../runbooks/release-audit.md); this file is
the record it produces, so a reader can answer "was this version audited,
and what came out of it?" without the session that ran it.

A cycle's section carries four things: the base SHA the census measured, the
fourteen axes with a verdict each (an axis nobody ran says so and why), the
baseline numbers against the previous cycle, and where the findings went.

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

| measure | v0.22 base |
|---|---|
| Go lines (test ≈ 1.3× production) | 219,183 |
| mean non-test cyclomatic over 3,011 functions | 5.35 |
| functions at cyclomatic ≥ 69 | 7 (worst: `cmd/gadak/init.go` 100) |
| `internal/server` fan-out · import cycles | 23 · 0 |
| CI wall / billable seconds | 438 / 3,358 (v0.19.0: 317 / 2,241) |
| catalog keys · real ja fall-throughs | 1,303 · 2 |
| fact copies · disagreeing | 122 · 10 |

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
