# Release audit log

One section per minor-version audit cycle. The procedure is
[`../runbooks/release-audit.md`](../runbooks/release-audit.md); this file is
the record it produces, so a reader can answer "was this version audited,
and what came out of it?" without the session that ran it.

A cycle's section carries four things: the base SHA the census measured, the
thirteen axes with a verdict each (an axis nobody ran says so and why), the
baseline numbers against the previous cycle, and where the findings went.

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
