# Release audit log

One section per minor-version audit cycle. The procedure is
[`../runbooks/release-audit.md`](../runbooks/release-audit.md); this file is
the record it produces, so a reader can answer "was this version audited,
and what came out of it?" without the session that ran it.

A cycle's section carries four things: the base SHA the census measured, the
thirteen axes with a verdict each (an axis nobody ran says so and why), the
baseline numbers against the previous cycle, and where the findings went.

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
