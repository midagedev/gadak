import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { expect, test } from './helpers'
import { gotoApp } from './helpers'

/*
 * GDK-1745: a dashboard list must never end in a row cut through its glyphs.
 *
 * The ja hero take opened a 12-colour label_ratio wall whose last legend row,
 * "flaky", stopped half-drawn at the panel's bottom edge with no scrollbar
 * and no fade. The host is not the clipper — measured on this harness, the
 * frame scrolls (scrollHeight 1616 against clientHeight 852), so the rows
 * were reachable and nothing said so. The sandbox has no allow-same-origin,
 * so the host cannot see inside the document to draw an affordance for it;
 * the document is the only layer that can, and
 * examples/dashboards/label-ratio.html is the shape SKILL.md now teaches.
 *
 * What this measures is the contract, not the styling: at both window sizes
 * in the issue, and with the panel forced short the way a terminal strip
 * makes it, the scroll container holds a whole number of rows (zero cut
 * rows) and says "+N more" exactly while rows are hidden.
 */

const E2E_DIR = dirname(fileURLToPath(import.meta.url))

/** The hero's datasource, verbatim from the SKILL.md recipe. */
const BY_LABEL =
  'select json_each.value as label, count(*) as n ' +
  'from issues_full, json_each(issues_full.labels) group by 1 order by n desc limit 20'

type Metrics = {
  rows: number
  rowH: number
  clientH: number
  scrollH: number
  /** Rows whose box is cut by the clipping edge at rest — the defect. */
  clipped: number
  /** Rows that begin below the clipping edge: hidden, but not cut. */
  belowFold: number
  pageOverflow: number
  moreVisible: boolean
  moreText: string
}

/**
 * Measure one wall's list against its own clipping edge. Both walls under
 * test name their scroller and their "+N more" line; only the selectors
 * differ, so the arithmetic lives here once.
 */
async function measureRows(
  page: import('@playwright/test').Page,
  rowSel: string,
  scrollSel: string,
): Promise<Metrics> {
  const body = page.frameLocator('[data-testid="dashboard-frame"]').locator('body')
  await expect(body.locator(rowSel).first()).toBeVisible()
  return await body.evaluate(
    (_body, [rowSel, scrollSel]) => {
      const legend = document.querySelector(scrollSel) as HTMLElement
      // A pre-fix wall has no affordance element at all — that reads as
      // "not visible", so the failure names the cut row rather than a null.
      const more = document.getElementById('more')
      const rows = [...document.querySelectorAll<HTMLElement>(rowSel)]
      const box = legend.getBoundingClientRect()
      const rowH = rows.length ? rows[0].getBoundingClientRect().height : 0
      // A row is "clipped" when the edge that actually cuts the drawing falls
      // strictly inside its box — the half-glyph the issue is about. That edge
      // is the scroll container's bottom or the frame's own viewport bottom,
      // whichever comes first: a document with no scroller of its own is cut
      // by the viewport, which is exactly the pre-fix shape. Rows entirely
      // below the fold are not clipped, they are scrolled away.
      const edge = Math.min(box.bottom, window.innerHeight)
      const clipped = rows.filter((r) => {
        const b = r.getBoundingClientRect()
        return b.top < edge - 0.5 && b.bottom > edge + 0.5
      }).length
      const belowFold = rows.filter((r) => r.getBoundingClientRect().top >= edge - 0.5).length
      const de = document.scrollingElement as HTMLElement
      return {
        rows: rows.length,
        rowH,
        clientH: legend.clientHeight,
        scrollH: legend.scrollHeight,
        clipped,
        belowFold,
        pageOverflow: de.scrollHeight - de.clientHeight,
        moreVisible: !!more && !more.hidden && more.offsetHeight > 0,
        moreText: more?.textContent ?? '',
      }
    },
    [rowSel, scrollSel],
  )
}

function assertContract(m: Metrics, where: string): void {
  expect(m.rows, `${where}: the legend painted no rows`).toBeGreaterThan(1)
  expect(m.clipped, `${where}: ${m.clipped} legend row(s) cut through by the panel edge`).toBe(0)
  // The page itself never scrolls: the header and the pie stay put and the
  // one growable region owns the overflow.
  expect(m.pageOverflow, `${where}: the document body overflowed the frame`).toBeLessThanOrEqual(1)
  const hidden = m.belowFold > 0
  expect(
    m.moreVisible,
    `${where}: ${m.belowFold} row(s) below the fold but the "+N more" line visible=${m.moreVisible}`,
  ).toBe(hidden)
  if (hidden) expect(m.moreText).toMatch(/\+\d+ more/)
}

test('a tall dashboard legend folds on a whole row and says there is more', async ({
  page,
  request,
}) => {
  const html = readFileSync(
    join(E2E_DIR, '..', 'examples', 'dashboards', 'label-ratio.html'),
    'utf8',
  )
  const name = `gdk1745-${Date.now()}`
  const created = await request.post('/api/v1/dashboards/', {
    data: {
      name,
      config: { html, datasources: { by_label: { sql: BY_LABEL } } },
    },
  })
  expect(created.ok(), await created.text()).toBeTruthy()
  const saved = (await created.json()) as { id: string }

  try {
    for (const size of [
      { width: 1440, height: 900 },
      { width: 1280, height: 800 },
    ]) {
      const where = `${size.width}x${size.height}`
      await page.setViewportSize(size)
      await gotoApp(page)
      await page.locator(`[data-dashboard-id="${saved.id}"]`).click()
      await expect(page.getByTestId('dashboard-frame')).toBeVisible()

      assertContract(await measureRows(page, '[data-legend-row]', '#legend'), `${where} full panel`)

      // ...and with the panel halved, the way the hero's terminal strip
      // halves it. The frame element is host DOM, so the host can do this
      // without reaching into the sandboxed document.
      await page.getByTestId('dashboard-frame').evaluate((el) => {
        ;(el as HTMLElement).style.flex = '0 0 380px'
      })
      const short = await measureRows(page, '[data-legend-row]', '#legend')
      // Contract first, precondition second: a pre-fix document fails on the
      // cut row, not on the scaffolding that arranges for one.
      assertContract(short, `${where} short panel`)
      expect(
        short.belowFold,
        'a 380px panel must actually put rows below the fold',
      ).toBeGreaterThan(0)
    }
  } finally {
    await request.delete(`/api/v1/dashboards/${saved.id}/`)
  }
})

/*
 * ...and the shape SKILL.md hands an agent is held to the same contract.
 * The example file above is what a person reads; the fenced block is what a
 * model copies, and only one of the two used to be tested. The block is
 * extracted from the file rather than duplicated here so the two cannot
 * drift: SKILL.md carries exactly one ```html fence.
 */
test('the copyable SKILL.md wall folds on a whole row too', async ({ page, request }) => {
  const md = readFileSync(join(E2E_DIR, '..', 'skills', 'gadak', 'SKILL.md'), 'utf8')
  const fences = md.split('```html')
  expect(fences.length, 'SKILL.md must carry exactly one html fence').toBe(2)
  const html = fences[1].split('```')[0]
  expect(html, 'the fence is the dashboard example').toContain("m.name !== 'by_label'")

  const created = await request.post('/api/v1/dashboards/', {
    data: {
      name: `gdk1745-skill-${Date.now()}`,
      config: { html, datasources: { by_label: { sql: BY_LABEL } } },
    },
  })
  expect(created.ok(), await created.text()).toBeTruthy()
  const saved = (await created.json()) as { id: string }

  try {
    for (const size of [
      { width: 1440, height: 900 },
      { width: 1280, height: 800 },
    ]) {
      const where = `${size.width}x${size.height} SKILL.md wall`
      await page.setViewportSize(size)
      await gotoApp(page)
      await page.locator(`[data-dashboard-id="${saved.id}"]`).click()
      await expect(page.getByTestId('dashboard-frame')).toBeVisible()
      await page.getByTestId('dashboard-frame').evaluate((el) => {
        ;(el as HTMLElement).style.flex = '0 0 380px'
      })
      const m = await measureRows(page, '.row', '#out')
      assertContract(m, where)
      expect(m.belowFold, `${where}: a 380px panel must put rows below the fold`).toBeGreaterThan(0)
    }
  } finally {
    await request.delete(`/api/v1/dashboards/${saved.id}/`)
  }
})
