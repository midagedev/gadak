# Performance budget gates

Interaction latency budgets against a **~10,000-issue** fixture. Budgets are
not aspirations: they are **pinned from local p95** with CI headroom.

## Run

```bash
# Quiet machine recommended (no other browser harness / e2e in parallel).
npm run test:perf
```

What it does:

1. `e2e/perf/make-fixture.sh` — `examples/demo.db` → `scry snapshot --scale 10000
   --now 2026-08-06T00:00:00Z` → `e2e/perf/.tmp/fixture.db` (gitignored).
2. Playwright starts `e2e/perf/serve.sh` on `127.0.0.1:7878` (not the main
   suite's `:7877`).
3. Four metrics, each **1 warmup + 20 samples → p95** vs budget.

| Metric | What is timed |
| --- | --- |
| **coldBoot** | New browser context (empty IndexedDB) → `issue-layout` + issue count text |
| **warmBoot** | Second visit on a primed context (IndexedDB hydration path) |
| **searchKeystroke** | One client-search keystroke → list count DOM update; asserts zero `/api/` traffic |
| **paletteOpen** | ⌘K / Ctrl+K → command palette visible |

Main suite (`npm run test:e2e`) is untouched. This folder has its own
`playwright.config.ts`. Specs skip unless `SCRY_PERF=1` so accidental discovery
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

FAIL-first: budgets set to `1ms` → all four failed (see run log). Then re-pinned:

| Metric | p50 (ms) | p95 (ms) | Budget (ms) | Formula |
| --- | ---: | ---: | ---: | --- |
| coldBoot | 480 | 971 | **1943** | max(100, ceil(971.3×2)) |
| warmBoot | 2329 | 4453 | **8907** | max(100, ceil(4453.3×2)) |
| searchKeystroke | 21 | 27 | **100** | max(100, ceil(26.8×2)) |
| paletteOpen | 7 | 9 | **100** | max(100, ceil(8.7×2)) |

Warm boot can exceed cold on this fixture (10k IndexedDB hydrate + full navigation);
it remains a separate metric because cache boot is a product promise.

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
  serve.sh             # SCRY_HOME + scry serve :7878
  playwright.config.ts
  perf.spec.ts         # budgets + instrumentation
  README.md
  .tmp/                # gitignored (fixture, binary, home)
```
