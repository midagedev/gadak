import { test, expect, type Page } from '@playwright/test'
import { attachConsoleErrors, forceLocale, DEMO_ISSUE_COUNT_EN_RE } from './helpers'

/*
 * GDK-1046: with the detail panel open, no trailing slot may paint past the
 * list scroller's right edge.
 *
 * Why nothing caught this before: list-row-columns.spec.ts proves slots
 * SHARE an x (GDK-128) — a column set can be perfectly aligned and entirely
 * outside the box; narrow-clip.spec.ts proves the 740-closed and the
 * docked-1100-open cells — at 1100 the viewport itself sits below the xl/lg
 * column breaks, so the panel-open 1280/1440 cells (a 570-678px row under a
 * viewport that still says "show everything", updated painting 190px past
 * the scroller, measured 2026-08-27) were covered by no gate. Column
 * visibility is row-width-keyed since GDK-1046 (@container issuerow), so
 * this assertion holds whatever made the row narrow.
 *
 * GDK-1050: inside the labels slot the same squeeze cut the +N counter at
 * the digit (+2 read as "+" and a stroke, measured 6px at the 64px slot
 * step, 2026-08-27). The slot clips with overflow:hidden, so the scroller
 * axis above stays green while the count is unreadable — the counter's own
 * rect vs the slot's rect is the only measurement that sees it (the
 * row-width probe's chip rows measure the same axis).
 *
 * qa_impact is a default column behind the `qa` feature; e2e/serve.sh's
 * config.json enables only deploy + teamGroups, so without the route
 * override the painted set is one slot short and this gate under-covers
 * (narrow-clip.spec.ts's config.json route idiom).
 *
 * GDK-1049: the same axis as a column-set matrix. The 12 option columns
 * (severity … due) were still viewport-keyed (`sm:/md:/lg:`), so a 1280-wide
 * viewport with the detail panel open left a 570px row carrying every slot
 * the viewport asked for — measured pre-fix: updated painted 1029px past the
 * scroller at 1280-open, 1001px at 1440-open, and the full catalog overflowed
 * CLOSED rows too (811px past at a 900 viewport, 437px at 1600). Since
 * GDK-1049 every optional column breaks on the row's own width
 * (@container issuerow, the threshold table in web/src/app.css), so this
 * assertion must hold for any on-set at any row width. The ladder also has
 * a capacity ceiling this matrix pins: the row caps at 1360px and the
 * fold-managed defaults + epic + qa_impact already spend most of it, so the
 * six sm-group columns are all that can ever paint together — at 1360 the
 * md/lg groups stay display:none (their thresholds sit above the cap).
 */

const COLUMN_SET = 'assignee,updated,labels,reopen,stale,qa_impact,deploy'
// Every catalog key (view-config COLUMN_KEYS_ALL order). Gated keys (qa_impact)
// survive the route override below; deploy/teamGroups are on in serve.sh's
// config.json.
const FULL_CATALOG =
  'assignee,updated,labels,reopen,stale,qa_impact,deploy,severity,issue_type,status,reporter,comment_count,fix_versions,components,created,due,environment,team_group,dev_test_result'
const PANEL_ISSUE = 'NMB-110'
// Float slack for deviceScaleFactor subpixel. narrow-clip.spec.ts uses 0.5
// for the same axis; GDK-1046's contract granted 1px.
const PAST_TOLERANCE_PX = 1

async function enableQaColumn(page: Page): Promise<void> {
  await page.route('**/config.json', async (route) => {
    const resp = await route.fetch()
    const json = (await resp.json()) as { features?: Record<string, unknown> }
    json.features = { ...(json.features ?? {}), qa: true }
    await route.fulfill({ json })
  })
}

/** Boot onto an open panel — the shared-link path (cross-links.spec.ts
 *  gotoParams idiom) — and wait for the resting docked grid track
 *  (narrow-clip.spec.ts idiom: the panel width locks immediately, the slide
 *  is 160ms, so wait on the resting track, never a duration). */
async function gotoPanelOpen(page: Page, cl: string = COLUMN_SET): Promise<void> {
  await page.goto('about:blank')
  await forceLocale(page, 'en')
  await page.goto(`/#/?cl=${cl}&issue=${PANEL_ISSUE}`)
  await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
  await expect(page.getByText(DEMO_ISSUE_COUNT_EN_RE).first()).toBeVisible({ timeout: 30_000 })
  await expect(
    page.getByTestId('issue-list-scroller').locator('[data-issue-key]').first(),
  ).toBeVisible({ timeout: 30_000 })
  await expect(
    page.locator('[data-testid="issue-layout"][data-detail-open="true"]'),
  ).toBeVisible()
  await expect
    .poll(async () =>
      page
        .getByTestId('issue-detail-panel')
        .evaluate((el) => Math.round(el.getBoundingClientRect().width)),
    )
    .toBeGreaterThan(400)
}

/** Per data-col, how many in-view rows paint that slot (display:not-none,
 * width ≥ 1). The matrix's liveness: a no-overflow pass with every optional
 * column hidden would be vacuous, so the ladder's visible/hidden rungs are
 * asserted by name at each driven width. */
async function visibleColCounts(page: Page): Promise<Record<string, number>> {
  return page.evaluate(() => {
    const scroller = document.querySelector<HTMLElement>('[data-testid="issue-list-scroller"]')
    const counts: Record<string, number> = {}
    if (!scroller) return counts
    for (const row of scroller.querySelectorAll<HTMLElement>('[data-issue-key]')) {
      const r = row.getBoundingClientRect()
      if (r.bottom < 0 || r.top > innerHeight) continue
      for (const slot of row.querySelectorAll<HTMLElement>('[data-col]')) {
        const s = getComputedStyle(slot)
        if (s.display === 'none' || s.visibility === 'hidden') continue
        if (slot.getBoundingClientRect().width < 1) continue
        const col = slot.dataset.col ?? '?'
        counts[col] = (counts[col] ?? 0) + 1
      }
    }
    return counts
  })
}

type PastProbe = {
  rows: number
  rowW: number | null
  past: { key: string; col: string; right: number; scrollerRight: number }[]
}

/** Every visible trailing slot of every in-view row that paints past the
 *  scroller's right edge (beyond the tolerance), plus row liveness counts.
 *  An empty `past` list is the contract. */
async function trailPastScroller(page: Page): Promise<PastProbe> {
  return page.evaluate((tol) => {
    const round = (n: number) => Math.round(n * 10) / 10
    const scroller = document.querySelector<HTMLElement>('[data-testid="issue-list-scroller"]')
    if (!scroller) return { rows: 0, rowW: null, past: [] }
    const sRight = scroller.getBoundingClientRect().right
    const past: { key: string; col: string; right: number; scrollerRight: number }[] = []
    let rows = 0
    let rowW: number | null = null
    for (const row of scroller.querySelectorAll<HTMLElement>('[data-issue-key]')) {
      const r = row.getBoundingClientRect()
      if (r.bottom < 0 || r.top > innerHeight) continue
      rows++
      if (rowW === null) rowW = round(r.width)
      const trail = row.querySelector<HTMLElement>('[data-testid="issue-row-trail"]')
      if (!trail) continue
      for (const slot of [...trail.children] as HTMLElement[]) {
        const s = getComputedStyle(slot)
        if (s.display === 'none' || s.visibility === 'hidden') continue
        const b = slot.getBoundingClientRect()
        if (b.width < 1) continue
        if (b.right > sRight + tol) {
          past.push({
            key: row.dataset.issueKey ?? '',
            col: slot.dataset.col ?? slot.className.toString().slice(0, 40),
            right: round(b.right),
            scrollerRight: round(sRight),
          })
        }
      }
    }
    return { rows, rowW, past }
  }, PAST_TOLERANCE_PX)
}

/** Closed-panel boot at the same pinned column set. 1440 closed is the 76px
 *  labels-slot step (row 1168, counters visible); 1280 closed is row 1008,
 *  where the labelfold ≤71 hide leaves no counter rendered to measure. */
async function gotoPanelClosed(page: Page, cl: string = COLUMN_SET): Promise<void> {
  await page.goto('about:blank')
  await forceLocale(page, 'en')
  await page.goto(`/#/?cl=${cl}`)
  await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
  await expect(page.getByText(DEMO_ISSUE_COUNT_EN_RE).first()).toBeVisible({ timeout: 30_000 })
  await expect(
    page.getByTestId('issue-list-scroller').locator('[data-issue-key]').first(),
  ).toBeVisible({ timeout: 30_000 })
}

type CounterProbe = {
  rows: number
  counted: number
  clipped: { key: string; kind: string; over: number; counterRight: number; slotRight: number }[]
}

/** Every visible +N counter whose box escapes its own .chipfold-labels slot
 *  (GDK-1050). The slot clips with overflow:hidden — `scrollWidth` on the
 *  slot cannot see this cut, only rect-vs-rect can. With `counterText`, each
 *  visible counter's text is swapped before measuring and restored after:
 *  the fixture's counts stop at 3 labels (+2), so the two-digit contract
 *  (a "+12" on a real account) is gated by widening a live counter, which
 *  exercises exactly the CSS that must give the width back. */
async function countersPastSlot(page: Page, counterText?: string): Promise<CounterProbe> {
  return page.evaluate(
    ({ text, tol }) => {
      const round = (n: number) => Math.round(n * 10) / 10
      const scroller = document.querySelector<HTMLElement>('[data-testid="issue-list-scroller"]')
      if (!scroller) return { rows: 0, counted: 0, clipped: [] }
      const clipped: CounterProbe['clipped'] = []
      const mutations: [HTMLElement, string | null][] = []
      let rows = 0
      let counted = 0
      for (const row of scroller.querySelectorAll<HTMLElement>('[data-issue-key]')) {
        const r = row.getBoundingClientRect()
        if (r.bottom < 0 || r.top > innerHeight) continue
        rows++
        const slot = row.querySelector<HTMLElement>('.chipfold-labels')
        if (!slot) continue
        const ss = getComputedStyle(slot)
        if (ss.display === 'none' || ss.visibility === 'hidden') continue
        const visible = [...slot.querySelectorAll<HTMLElement>('.chipfold-n-extra, .chipfold-n-rest, .chipfold-n-all')].filter(
          (el) => {
            const st = getComputedStyle(el)
            return st.display !== 'none' && st.visibility !== 'hidden' && el.getBoundingClientRect().width >= 1
          },
        )
        if (text) for (const el of visible) mutations.push([el, el.textContent])
        // One gBCR read after the (possible) text swap forces layout, so the
        // measured slot right edge and counter boxes are from the same frame.
        const slotRight = slot.getBoundingClientRect().right
        for (const el of visible) {
          const b = el.getBoundingClientRect()
          const kind = (el.className.match(/chipfold-n-\w+/) ?? [''])[0]
          if (text) el.textContent = text
          const w = el.getBoundingClientRect()
          counted++
          const over = w.right - slotRight
          if (over > tol) {
            clipped.push({
              key: row.dataset.issueKey ?? '',
              kind,
              over: round(over),
              counterRight: round(w.right),
              slotRight: round(slotRight),
            })
          }
        }
      }
      for (const [el, text0] of mutations) el.textContent = text0
      return { rows, counted, clipped }
    },
    { text: counterText ?? '', tol: 0.5 },
  )
}

async function expectCountersInsideSlot(page: Page, width: number, label: string): Promise<void> {
  const probe = await countersPastSlot(page)
  // Liveness: with no counter rendered this would pass vacuously — the
  // fixture's 2-3-label rows must be in view at every driven width.
  expect(probe.rows, 'need several rendered rows in view').toBeGreaterThan(4)
  expect(probe.counted, 'need at least one rendered +N counter').toBeGreaterThan(0)
  expect(
    probe.clipped,
    `${label}: +N counters past their labels slot at ${width}: ${JSON.stringify(probe.clipped)}`,
  ).toEqual([])

  // Two-digit contract (GDK-1050): widen every live counter to "+99" — the
  // chip must hand the extra width back, at every slot step.
  const twoDigit = await countersPastSlot(page, '+99')
  expect(twoDigit.counted, 'need at least one widened counter').toBeGreaterThan(0)
  expect(
    twoDigit.clipped,
    `${label}: two-digit (+99) counters past their labels slot at ${width}: ${JSON.stringify(twoDigit.clipped)}`,
  ).toEqual([])
}

for (const width of [1280, 1440]) {
  test.describe(`detail panel open @${width}`, () => {
    test.use({ viewport: { width, height: 900 } })

    test('no trailing slot paints past the list scroller', async ({ page }) => {
      const errors = attachConsoleErrors(page)
      await enableQaColumn(page)
      await gotoPanelOpen(page)

      const probe = await trailPastScroller(page)
      // Liveness: the measurement ran over a painted list, not an empty one —
      // otherwise the empty `past` below would pass vacuously.
      expect(probe.rows, 'need several rendered rows in view').toBeGreaterThan(4)

      expect(
        probe.past,
        `rowW=${probe.rowW}: trail slots past the scroller at ${width}: ${JSON.stringify(probe.past)}`,
      ).toEqual([])

      expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
    })

    test('every +N counter fits inside the labels slot', async ({ page }) => {
      const errors = attachConsoleErrors(page)
      await enableQaColumn(page)
      await gotoPanelOpen(page)

      await expectCountersInsideSlot(page, width, 'detail panel open')

      expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
    })
  })
}

test.describe('detail panel closed @1440', () => {
  test.use({ viewport: { width: 1440, height: 900 } })

  test('every +N counter fits inside the labels slot', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await enableQaColumn(page)
    await gotoPanelClosed(page)

    await expectCountersInsideSlot(page, 1440, 'detail panel closed')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})

/* ── GDK-1049 column-set matrix ──
 * Same axis as above (no trailing slot past the scroller) over the worst
 * on-set: the whole catalog. Row widths the driven viewports produce:
 * 1280-open ≈ 570, 1440-open ≈ 678, 1440-closed = 1168, 1700-closed = 1360
 * (the layout cap). Ladder rungs (app.css threshold table): severity 770,
 * issue_type 860, status 1060, comment_count 1130, created 1180, due 1340;
 * md/lg groups above 1360 = never paint. */
for (const width of [1280, 1440]) {
  test.describe(`full catalog, detail panel open @${width}`, () => {
    test.use({ viewport: { width, height: 900 } })

    test('no trailing slot paints past the list scroller', async ({ page }) => {
      const errors = attachConsoleErrors(page)
      await enableQaColumn(page)
      await gotoPanelOpen(page, FULL_CATALOG)

      const probe = await trailPastScroller(page)
      expect(probe.rows, 'need several rendered rows in view').toBeGreaterThan(4)
      expect(
        probe.past,
        `rowW=${probe.rowW}: full-catalog trail slots past the scroller at ${width}: ${JSON.stringify(probe.past)}`,
      ).toEqual([])

      // Liveness: the row narrowed by hiding the optional columns (the
      // GDK-1049 mechanism), not by rendering nothing. Fold-managed defaults
      // stay: stale/labels paint at both widths; the lowest option rung
      // (severity, 770) is above both rows (570/678).
      const cols = await visibleColCounts(page)
      expect(cols.stale, 'stale is fold-managed and must paint').toBeGreaterThan(0)
      expect(cols.labels, 'labels are fold-managed and must paint').toBeGreaterThan(0)
      expect(cols.severity, `severity (rung 770) must be hidden on a ${probe.rowW}px row`).toBeUndefined()
      expect(cols.components, 'components (rung 1960) must be hidden').toBeUndefined()

      expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
    })
  })
}

test.describe('full catalog, detail panel closed @1440', () => {
  test.use({ viewport: { width: 1440, height: 900 } })

  test('no trailing slot paints past the list scroller', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await enableQaColumn(page)
    await gotoPanelClosed(page, FULL_CATALOG)

    const probe = await trailPastScroller(page)
    expect(probe.rows, 'need several rendered rows in view').toBeGreaterThan(4)
    expect(
      probe.past,
      `rowW=${probe.rowW}: full-catalog trail slots past the scroller at 1440 closed: ${JSON.stringify(probe.past)}`,
    ).toEqual([])

    // Row 1168: the ladder paints its ≤1130 rungs and cuts above them.
    const cols = await visibleColCounts(page)
    expect(cols.severity, 'severity (770) must paint on a 1168px row').toBeGreaterThan(0)
    expect(cols.issue_type, 'issue_type (860) must paint on a 1168px row').toBeGreaterThan(0)
    expect(cols.status, 'status (1060) must paint on a 1168px row').toBeGreaterThan(0)
    expect(cols.comment_count, 'comment_count (1130) must paint on a 1168px row').toBeGreaterThan(0)
    expect(cols.epic, 'epic (750, GDK-1046) must paint on a 1168px row').toBeGreaterThan(0)
    expect(cols.updated, 'updated is fold-managed and must paint').toBeGreaterThan(0)
    expect(cols.created, 'created (rung 1180) must be hidden on a 1168px row').toBeUndefined()
    expect(cols.fix_versions, 'fix_versions (rung 1840) must be hidden').toBeUndefined()
    expect(cols.components, 'components (rung 1960) must be hidden').toBeUndefined()
    expect(cols.reporter, 'reporter (rung 1720) must be hidden').toBeUndefined()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})

test.describe('full catalog, detail panel closed @1700', () => {
  test.use({ viewport: { width: 1700, height: 900 } })

  test('no trailing slot paints past the list scroller at the row cap', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await enableQaColumn(page)
    await gotoPanelClosed(page, FULL_CATALOG)

    const probe = await trailPastScroller(page)
    expect(probe.rows, 'need several rendered rows in view').toBeGreaterThan(4)
    expect(probe.rowW, '1700 closed must produce the 1360px capped row').toBe(1360)
    expect(
      probe.past,
      `rowW=${probe.rowW}: full-catalog trail slots past the scroller at the cap: ${JSON.stringify(probe.past)}`,
    ).toEqual([])

    // Capacity ceiling: the cap row spends its option budget on the sm six
    // (due is the top rung that still paints); every md/lg slot stays
    // display:none above the cap it cannot fit.
    const cols = await visibleColCounts(page)
    expect(cols.due, 'due (rung 1340) must paint on the 1360px cap row').toBeGreaterThan(0)
    expect(cols.dev_test_result, 'dev_test_result (rung 1440 > cap) must be hidden').toBeUndefined()
    expect(cols.environment, 'environment (rung 1530 > cap) must be hidden').toBeUndefined()
    expect(cols.team_group, 'team_group (rung 1620 > cap) must be hidden').toBeUndefined()
    expect(cols.reporter, 'reporter (rung 1720 > cap) must be hidden').toBeUndefined()
    expect(cols.fix_versions, 'fix_versions (rung 1840 > cap) must be hidden').toBeUndefined()
    expect(cols.components, 'components (rung 1960 > cap) must be hidden').toBeUndefined()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
