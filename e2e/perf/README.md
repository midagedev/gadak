# Performance budget gates

Interaction latency budgets against a **~10,000-issue, 5,000-document** fixture.
Budgets are not aspirations: they are **pinned from local p95** with CI headroom.

The document half of that fixture is new in 2026-08-07, and its absence is worth
recording: `gadak snapshot` copies the issue axis only (its table list has no
`pages` or `spaces`), so every fixture built from it held zero documents, and no
budget could see the document lists at all. They shipped rendering every row —
a 4.4s freeze in the desktop app on a 10,000-page mirror. A gate over a fixture
that cannot contain the data is not a gate.

## Run

```bash
# Quiet machine recommended (no other browser harness / e2e in parallel).
npm run test:perf
```

What it does:

1. `e2e/perf/make-fixture.sh` — `examples/demo.db` → `gadak snapshot --scale 10000
   --now 2026-08-06T00:00:00Z` → `e2e/perf/.tmp/fixture.db` (gitignored), then
   the source mirror's 71 pages are cloned in SQL to 5,000 (snapshot cannot
   carry them). Clones drop `parent_id`: a cloned hierarchy would nest N copies
   of one tree under a single root, which no real mirror looks like. FTS rows
   are not cloned either, so the fixture measures rendering, not search.
2. Playwright starts `e2e/perf/serve.sh` on `127.0.0.1:7878` (not the main
   suite's `:7877`).
3. Six metrics, each **1 warmup + 20 samples → p95** vs budget.

| Metric | What is timed |
| --- | --- |
| **coldBoot** | New browser context (empty IndexedDB) → `issue-layout` + issue count text |
| **warmBoot** | Second visit on a primed context (IndexedDB hydration path) |
| **searchKeystroke** | One client-search keystroke → list count DOM update; asserts zero `/api/` traffic |
| **paletteOpen** | ⌘K / Ctrl+K → command palette visible |
| **docsTabSwitch** | Documents → Updated ↔ By author; both tabs list the whole mirror, so this is the list rebuild |
| **docsFilterKeystroke** | One keystroke in the document filter (Updated, 5,000 pages) → header count becomes a fraction; asserts no request beyond the freshness pollers |

The document-filter metric excludes `sync/progress/`, `sync/runs/` and `meta/`
from its network assertion: those run on their own timer and will land inside
any window wide enough to catch one, so including them would let the sample
length decide the verdict. Anything that could actually answer a filter —
`pages/`, `search/`, `bootstrap/`, `delta/` — still fails it.

Main suite (`npm run test:e2e`) is untouched. This folder has its own
`playwright.config.ts`. Specs skip unless `GADAK_PERF=1` so accidental discovery
by the main config does not fail CI.

## Budget philosophy

- Measure on a quiet machine; concurrent harnesses invalidate numbers.
- Report p50 and p95; **gate on p95**.
- Pin: `budget = max(100, ceil(local_p95 * 2))` — 2× headroom for CI runner noise.
- Each budget constant carries an attribution comment:

  ```ts
  // pinned 2026-08-06: local p95=320ms, budget=ceil(320*2)=640
  coldBootMs: 640,
  ```

### Current pins (2026-08-06, local, quiet machine)

Original FAIL-first (all four at `1ms`): see `e2e/perf/.tmp/fail-first-run.log`.
warmBoot re-pin after priming fix (2026-08-06): FAIL-first at 0.5×
(`budget=66`) failed with p95≈138ms — `e2e/perf/.tmp/fail-first-warmboot-0.5x.log`.

Quiet-machine measure after product chunking + priming fix:

| Metric | p50 (ms) | p95 (ms) | Budget (ms) | Formula / note |
| --- | ---: | ---: | ---: | --- |
| coldBoot | 407 | 425 | **1943** | budget unchanged (no new basis); this-run p50/p95 recorded |
| warmBoot | 121 | 132 | **265** | max(100, ceil(132.1×2)); was 8907 (invalid cold+contention) |
| searchKeystroke | 22 | 27 | **100** | max(100, ceil(26.8×2)) — budget unchanged |
| paletteOpen | 7 | 14 | **100** | max(100, ceil(8.7×2)) — budget unchanged |

### docsTabSwitch (added 2026-08-07)

FAIL-first, on the unwindowed list this budget was written against:
`e2e/perf/.tmp/fail-first-docs-tab-switch.log`. The other four budgets were left
alone — adding 5,000 documents to the fixture did not move any of them.

| Metric | p50 (ms) | p95 (ms) | Budget (ms) | Formula / note |
| --- | ---: | ---: | ---: | --- |
| docsTabSwitch (before) | 777 | 1107 | — | every row rendered; 45,013 DOM nodes |
| docsTabSwitch (after) | 23 | 29 | **100** | max(100, ceil(29.2×2)) |

### docsFilterKeystroke (added 2026-08-07)

The document screens gained a filter, and with it a keystroke path over the
whole 5,000-page index. FAIL-first at `budget=1`:
`e2e/perf/.tmp/fail-first-docs-filter-keystroke.log`. The other five budgets
were left alone; the same run measured them all inside their pins.

| Metric | p50 (ms) | p95 (ms) | Budget (ms) | Formula / note |
| --- | ---: | ---: | ---: | --- |
| docsFilterKeystroke | 14.7 | 15.3 | **100** | max(100, ceil(15.3×2)) |

**Warm boot is faster than cold when measured correctly** (this run: warm p95 132ms
vs cold p95 425ms, ~3.2×). An earlier pin of warmBoot **8907ms** was invalid: the
harness navigated again before the bootstrap IndexedDB write committed, so every
"warm" sample was empty-cache cold plus contention with a still-open write. The
suite now polls IDB row count == 10,000 after interactive before sampling.

## Re-pin rules (all three required)

Do **not** loosen a budget without:

1. **Attribution comment** — date + measured p95 + derived budget formula.
2. **Justified derivation** — `max(100, ceil(p95 * 2))` (or a documented
   exception with reason).
3. **FAIL-first** — temporarily set the budget to ~0.5× measured p95, capture
   the failure output, then re-pin to 2×. Never raise a limit without seeing
   the gate go red first.

If the product truly got slower, prefer fixing the regression. Re-pin only when
the measurement is honest and the new frontier is accepted.

## Layout

```
e2e/perf/
  make-fixture.sh      # deterministic 10k DB
  serve.sh             # GADAK_HOME + gadak serve :7878
  playwright.config.ts
  perf.spec.ts         # budgets + instrumentation
  README.md
  .tmp/                # gitignored (fixture, binary, home)
```
